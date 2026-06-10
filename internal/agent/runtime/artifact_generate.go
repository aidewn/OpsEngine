// artifact 生成重试：解析/校验失败时将错误反馈给模型并多轮修正，避免一次失败就退出。

package runtime

import (
	"fmt"
	"strings"

	"OpsEngine/internal/agent/workflow"
	"OpsEngine/internal/clients"
)

// defaultArtifactGenRetries 是 artifact JSON 生成默认最大尝试次数（含首次）。
const defaultArtifactGenRetries = 3

// isExecutionFailureReport 判断用户消息是否为工作流/集合执行失败日志。
func isExecutionFailureReport(message string) bool {
	text := strings.ToLower(strings.TrimSpace(message))
	if text == "" {
		return false
	}
	signals := []string{
		"exit_code", "stderr", "stdout",
		"执行失败", "脚本执行失败", "命令执行失败",
		"execution failed", "failed with exit",
	}
	for _, s := range signals {
		if strings.Contains(text, s) {
			return true
		}
	}
	return false
}

// buildExecutionFixUserPrompt 构造「根据执行日志修复 artifact」的用户提示。
func buildExecutionFixUserPrompt(message string) string {
	return fmt.Sprintf(`集合/工作流执行失败，请根据以下日志修复脚本或节点配置（重点检查 sed/awk 转义、变量引用、shell 语法），输出完整修正后的 JSON。

失败日志：
%s

修复要求：
- 先定位 stderr/exit_code 根因，再改脚本，不要只改注释
- sed 表达式含特殊字符时，改用 heredoc、python 或更稳妥写法
- 保持 params/returns 接口不变，除非日志表明必须调整
- 只输出 JSON，禁止 Markdown 与解释文字`, message)
}

// requestArtifactDraft 向 LLM 请求 artifact 草案；解析或校验失败时自动反馈并重试。
func (r *Runtime) requestArtifactDraft(
	req Request,
	sessionID string,
	progress *[]string,
	systemPrompt string,
	userPrompt string,
	validate func(draft workflow.Draft) error,
) (workflow.Draft, error) {
	messages := []clients.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	maxRetries := defaultArtifactGenRetries

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			r.emitProgress(req.RequestID, sessionID,
				fmt.Sprintf("第 %d 次尝试修正…", attempt+1), progress)
		}

		r.emitProgress(req.RequestID, sessionID, "正在请求大模型", progress)
		reply, err := r.LLM.Chat(messages)
		if err != nil {
			if attempt < maxRetries-1 && isRetryableLLMError(err) {
				r.emitProgress(req.RequestID, sessionID, "模型调用失败，正在重试…", progress)
				continue
			}
			return workflow.Draft{}, err
		}

		r.emitProgress(req.RequestID, sessionID, "正在解析模型返回", progress)
		draft, err := workflow.ParseDraft(reply)
		if err != nil {
			r.emitProgress(req.RequestID, sessionID, "解析失败："+err.Error(), progress)
			messages = append(messages,
				clients.ChatMessage{Role: "assistant", Content: reply},
				clients.ChatMessage{Role: "user", Content: "JSON 解析失败：" + err.Error() + "。请只输出修正后的完整 JSON，禁止 Markdown。"},
			)
			continue
		}

		r.emitProgress(req.RequestID, sessionID, "正在校验结构", progress)
		if err := validate(draft); err != nil {
			r.emitProgress(req.RequestID, sessionID, "校验失败："+err.Error(), progress)
			messages = append(messages,
				clients.ChatMessage{Role: "assistant", Content: reply},
				clients.ChatMessage{Role: "user", Content: "结构校验失败：" + err.Error() + "。请修正后输出完整 JSON。"},
			)
			continue
		}
		return draft, nil
	}
	return workflow.Draft{}, fmt.Errorf("已尝试 %d 次仍无法生成有效 JSON，请补充更具体的需求或手动修改", maxRetries)
}

// isRetryableLLMError 判断 LLM 调用错误是否值得自动重试（网络/超时类）。
func isRetryableLLMError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	retryable := []string{
		"timeout", "timed out", "connection", "network", "eof",
		"429", "502", "503", "504", "rate limit",
	}
	for _, s := range retryable {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}
