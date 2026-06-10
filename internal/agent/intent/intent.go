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
	// KindCreateAssemble 用户明确要求生成可复用集合。
	KindCreateAssemble Kind = "create_assemble"
	// KindUpdateAssemble 用户要求修改当前或指定集合。
	KindUpdateAssemble Kind = "update_assemble"
	// KindUpdateWorkflow 用户要求修改当前或指定工作流。
	KindUpdateWorkflow Kind = "update_workflow"
	// KindInspectServer 用户要求做服务器巡检：走两阶段生成（LLM 出 Plan，后端拼工作流）。
	// 比 KindGenerateWorkflow 更具体、更稳定，所以路由时应优先命中。
	KindInspectServer Kind = "inspect_server"
	// KindTroubleshoot 用户描述故障现象，要求 Agent 排查：走"工具循环 + 事实/判断/建议"路径。
	// 与 KindChat 区别在于系统提示词强制结构化输出，便于后续沉淀为排障文档。
	KindTroubleshoot Kind = "troubleshoot"
	// KindAnalyzeArchitecture 用户要求分析环境架构：触发 SSH 拓扑采集 + Mermaid 渲染 + LLM 说明。
	KindAnalyzeArchitecture Kind = "analyze_architecture"
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

// architectureKeywords 触发 analyze_architecture。包含明确的"架构/拓扑/服务关系"信号词。
// 优先级低于 inspection / troubleshoot（这两类是更具体的诉求）。
var architectureKeywords = []string{
	"架构", "拓扑", "服务关系", "组件关系", "依赖关系",
	"architecture", "topology",
}

// troubleshootKeywords 触发 troubleshoot。包含典型故障现象词。
// 优先于普通 chat，但低于 inspection（"排查巡检脚本"应走巡检而不是排障）。
var troubleshootKeywords = []string{
	"排查", "排障", "故障", "诊断", "为什么挂", "为什么慢", "为什么不通",
	"cpu 高", "cpu高", "内存高", "内存满", "磁盘满", "磁盘 100", "服务挂", "起不来", "无响应",
	"troubleshoot", "diagnose", "investigate", "high cpu", "out of memory", "disk full",
}

// workflowKeywords 是触发 generate_workflow 的强信号词。
// 注意：扩展时优先添加"动作 + 对象"的组合，避免单字误命中（例如只写"流"会误判）。
var workflowKeywords = []string{
	"工作流", "流程",
	"workflow", "pipeline",
}

// assembleKeywords 触发 create_assemble。集合是可复用资产，优先于工作流判定。
var assembleKeywords = []string{
	"集合", "可复用节点", "复用模块", "assemble", "module",
}

// updateKeywords 触发当前资产迭代，需结合 operation 或前端传入的上下文使用。
var updateKeywords = []string{
	"修改", "调整", "更新", "改成", "再加", "加一个", "删除", "移除",
	"update", "modify", "change", "add", "remove",
}

// negativeKeywords 命中其一时强制回退到 chat，覆盖"解释一下"/"不要生成"等场景。
// 注意：这些词只用来抑制"生成"类操作（workflow / inspection）；
// "怎么排查 / 为什么挂"这类原本被当作否定的词已经迁移到 troubleshootKeywords，
// 因为它们实际上是排障信号而非"不要做任何事"。
var negativeKeywords = []string{
	"不要生成", "不用生成", "别生成", "不需要工作流",
	"解释", "说明",
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
	// 排障优先于通用 workflow：用户说"排查 CPU 高"应直接进排障路径，而不是被识别为"生成排查工作流"。
	for _, kw := range troubleshootKeywords {
		if strings.Contains(text, kw) {
			return Result{Kind: KindTroubleshoot, Reason: "命中排障关键词：" + kw}
		}
	}
	// 架构分析在排障/巡检之后判断：避免"排查架构问题"被错路由到这里（troubleshoot 已先命中）。
	for _, kw := range architectureKeywords {
		if strings.Contains(text, kw) {
			return Result{Kind: KindAnalyzeArchitecture, Reason: "命中架构关键词：" + kw}
		}
	}
	for _, kw := range assembleKeywords {
		if strings.Contains(text, kw) {
			if containsAny(text, updateKeywords) {
				return Result{Kind: KindUpdateAssemble, Reason: "命中集合修改关键词：" + kw}
			}
			return Result{Kind: KindCreateAssemble, Reason: "命中集合关键词：" + kw}
		}
	}
	for _, kw := range workflowKeywords {
		if strings.Contains(text, kw) {
			if containsAny(text, updateKeywords) {
				return Result{Kind: KindUpdateWorkflow, Reason: "命中工作流修改关键词：" + kw}
			}
			return Result{Kind: KindGenerateWorkflow, Reason: "命中工作流关键词：" + kw}
		}
	}
	return Result{Kind: KindChat, Reason: "未命中关键词，按普通问答处理"}
}

// String 让 Kind 可直接拼接到日志/事件文本中。
func (k Kind) String() string { return string(k) }

func containsAny(text string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}
