// 服务器巡检的"运维计划"中间表示。
//
// 设计动机（文档 P6 / 路线 B）：
//   - 模型直接拼工作流 JSON 不稳定（节点 id / 端口 / 字段易编造）。
//   - 两阶段生成：模型只产 Plan，后端把 Plan 翻译成 core.WorkflowDef。
//   - v2 引入 ItemKind：模型按"动作类型"声明意图，Materializer 翻译成具体 shell 命令。
//     这样 LLM 不用记 docker ps / kubectl 等各种命令的精确格式，安全性也由后端把关。

package inspection

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ItemKind 是巡检项的动作类型。
//
// 全部动作最终都走 linux_exec_command 节点执行，但 Materializer 按 Kind 选择命令模板：
//   - shell：调用方提供任意命令（保底，需走 dangerousTokens 黑名单）
//   - 其它 Kind：Materializer 自行拼接命令，参数走 shellQuote 防注入
type ItemKind string

const (
	// ItemKindShell 自定义 shell 命令（向后兼容旧 Plan）。
	ItemKindShell ItemKind = "shell"
	// ItemKindReadFile 读文件全文（head -c 限制 64KB）。
	ItemKindReadFile ItemKind = "read_file"
	// ItemKindReadLog 读文件尾部（tail -n）。
	ItemKindReadLog ItemKind = "read_log"
	// ItemKindFindFiles find 文件，深度 5、上限 200。
	ItemKindFindFiles ItemKind = "find_files"
	// ItemKindDockerList docker ps，含 stopped。
	ItemKindDockerList ItemKind = "docker_list"
	// ItemKindDockerLogs docker logs --tail。
	ItemKindDockerLogs ItemKind = "docker_logs"
	// ItemKindK8sPods kubectl get pods。
	ItemKindK8sPods ItemKind = "k8s_pods"
	// ItemKindK8sDescribe kubectl describe workload。
	ItemKindK8sDescribe ItemKind = "k8s_describe"
	// ItemKindSystemd systemctl status --no-pager。
	ItemKindSystemd ItemKind = "systemd"
	// ItemKindPortListen ss -tlnp / netstat -tlnp。
	ItemKindPortListen ItemKind = "port_listen"
)

// AllItemKinds 暴露给 Prompt 渲染时引用（保持顺序稳定）。
var AllItemKinds = []ItemKind{
	ItemKindShell, ItemKindReadFile, ItemKindReadLog, ItemKindFindFiles,
	ItemKindDockerList, ItemKindDockerLogs,
	ItemKindK8sPods, ItemKindK8sDescribe,
	ItemKindSystemd, ItemKindPortListen,
}

// Plan 是一份巡检计划，由 LLM 输出，后端消费。
type Plan struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Items       []Item `json:"items"`
}

// Item 是一条巡检项。
// 字段按 Kind 解读；Validate 会针对每种 Kind 检查必填项。
// JSON 字段平铺方便 LLM 输出，多余字段忽略。
type Item struct {
	Title          string   `json:"title"`
	Kind           ItemKind `json:"kind,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`

	// 自定义 shell（仅 Kind=shell 用）
	Command string `json:"command,omitempty"`

	// 文件/日志类
	Path      string `json:"path,omitempty"`
	TailLines int    `json:"tail_lines,omitempty"`

	// 查找
	Root    string `json:"root,omitempty"`
	Pattern string `json:"pattern,omitempty"`

	// 容器
	Container string `json:"container,omitempty"`

	// K8s
	Namespace     string `json:"namespace,omitempty"`
	LabelSelector string `json:"label_selector,omitempty"`
	Workload      string `json:"workload,omitempty"` // 形如 "Deployment/nginx"

	// systemd
	Service string `json:"service,omitempty"`
}

// MinItems / MaxItems 限制单次巡检的项数。
const (
	MinItems = 1
	MaxItems = 20
)

// ParsePlan 从模型回复中提取并解析 JSON 巡检计划。
// 兼容模型常见的带 Markdown 围栏 / 前后解释输出。
func ParsePlan(reply string) (Plan, error) {
	text := strings.TrimSpace(reply)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return Plan{}, errors.New("AI 返回内容不是 JSON")
	}
	var plan Plan
	if err := json.Unmarshal([]byte(text[start:end+1]), &plan); err != nil {
		return Plan{}, fmt.Errorf("解析巡检计划失败: %w", err)
	}
	if err := plan.Validate(); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

// Validate 校验 Plan 的基础合法性 + 每个 Item 按 Kind 检查必填字段。
func (p Plan) Validate() error {
	if len(p.Items) < MinItems {
		return fmt.Errorf("巡检计划至少需要 %d 项，实际 %d", MinItems, len(p.Items))
	}
	if len(p.Items) > MaxItems {
		return fmt.Errorf("巡检计划最多 %d 项，实际 %d", MaxItems, len(p.Items))
	}
	for i, it := range p.Items {
		if strings.TrimSpace(it.Title) == "" {
			return fmt.Errorf("巡检项 #%d 缺少 title", i+1)
		}
		if err := it.validateForKind(); err != nil {
			return fmt.Errorf("巡检项 %q: %w", it.Title, err)
		}
	}
	return nil
}

// resolveKind 缺省 Kind 时回退到 shell（向后兼容旧 Plan）。
func (it Item) resolveKind() ItemKind {
	if it.Kind == "" {
		return ItemKindShell
	}
	return it.Kind
}

// validateForKind 按 Kind 检查必填字段。
func (it Item) validateForKind() error {
	kind := it.resolveKind()
	requireNonEmpty := func(field, value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("kind=%s 时 %s 必填", kind, field)
		}
		return nil
	}
	switch kind {
	case ItemKindShell:
		return requireNonEmpty("command", it.Command)
	case ItemKindReadFile:
		return requireNonEmpty("path", it.Path)
	case ItemKindReadLog:
		return requireNonEmpty("path", it.Path)
	case ItemKindFindFiles:
		return requireNonEmpty("root", it.Root)
	case ItemKindDockerList, ItemKindK8sPods, ItemKindPortListen:
		// 这三个无必填字段
		return nil
	case ItemKindDockerLogs:
		return requireNonEmpty("container", it.Container)
	case ItemKindK8sDescribe:
		if err := requireNonEmpty("workload", it.Workload); err != nil {
			return err
		}
		if !strings.Contains(it.Workload, "/") {
			return fmt.Errorf("workload 必须是 Kind/Name 形式，例如 Deployment/nginx，实际 %q", it.Workload)
		}
		return nil
	case ItemKindSystemd:
		return requireNonEmpty("service", it.Service)
	default:
		return fmt.Errorf("未知 Kind: %s", kind)
	}
}
