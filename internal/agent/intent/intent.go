// Intent Router：把用户消息解析成结构化意图，替代 ai.go 里基于关键词的二分判断。
// 当前为规则版（P3 第一阶段），后续可扩展为"规则 + LLM 兜底"双层。
// 设计原则：先识别用户的"否定意图"（不要生成工作流 / 只想问问），再识别强信号关键词。

package intent

import "strings"

// Kind 是意图枚举。后续新增 inspect_server / troubleshoot / generate_report 时直接加常量即可。
type Kind string

const (
	// KindChat 普通运维问答，不落工作流。
	KindChat Kind = "chat"
	// KindGenerateWorkflow 用户明确要求生成可执行工作流。
	KindGenerateWorkflow Kind = "generate_workflow"
	// KindInspectServer 用户要求做服务器巡检：走两阶段生成（LLM 出 Plan，后端拼工作流）。
	// 比 KindGenerateWorkflow 更具体、更稳定，所以路由时应优先命中。
	KindInspectServer Kind = "inspect_server"
)

// Result 包含意图本身和触发原因，供前端进度展示和后续诊断使用。
type Result struct {
	Kind   Kind
	Reason string
}

// inspectionKeywords 触发 inspect_server。优先于通用 workflow 命中——巡检走更稳定的两阶段生成。
var inspectionKeywords = []string{
	"巡检", "体检", "健康检查",
	"inspect", "inspection", "health check",
}

// workflowKeywords 是触发 generate_workflow 的强信号词。
// 注意：扩展时优先添加"动作 + 对象"的组合，避免单字误命中（例如只写"流"会误判）。
var workflowKeywords = []string{
	"工作流", "流程",
	"workflow", "pipeline",
}

// negativeKeywords 命中其一时强制回退到 chat，覆盖"解释一下"/"不要生成"等场景。
// 即使消息里出现"工作流"，也按用户更明确的否定意图处理。
var negativeKeywords = []string{
	"不要生成", "不用生成", "别生成", "不需要工作流",
	"解释", "说明", "为什么", "怎么排查", "如何排查",
	"do not generate", "don't generate", "explain",
}

// Resolve 根据外部传入的 operation 和用户消息推断意图。
// operation 为空或 "auto" 时走规则判断；否则尊重前端显式指定。
func Resolve(operation, message string) Result {
	operation = strings.TrimSpace(operation)
	switch operation {
	case "":
		// 兼容旧调用方未设置 operation 的情况。
	case "auto":
		// 显式要求自动判定。
	default:
		return Result{Kind: Kind(operation), Reason: "由调用方显式指定"}
	}

	text := strings.ToLower(strings.TrimSpace(message))
	if text == "" {
		return Result{Kind: KindChat, Reason: "空消息走默认对话"}
	}

	for _, kw := range negativeKeywords {
		if strings.Contains(text, kw) {
			return Result{Kind: KindChat, Reason: "命中否定关键词：" + kw}
		}
	}
	// 巡检优先于通用工作流：用户说"巡检"几乎一定是要做巡检，而不是"生成一个叫巡检的通用工作流"。
	for _, kw := range inspectionKeywords {
		if strings.Contains(text, kw) {
			return Result{Kind: KindInspectServer, Reason: "命中巡检关键词：" + kw}
		}
	}
	for _, kw := range workflowKeywords {
		if strings.Contains(text, kw) {
			return Result{Kind: KindGenerateWorkflow, Reason: "命中工作流关键词：" + kw}
		}
	}
	return Result{Kind: KindChat, Reason: "未命中关键词，按普通问答处理"}
}

// String 让 Kind 可直接拼接到日志/事件文本中。
func (k Kind) String() string { return string(k) }
