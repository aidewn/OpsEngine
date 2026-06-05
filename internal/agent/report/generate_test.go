// GenerateInspection 编排测试：覆盖纯事实 + LLM 注入 + LLM 失败兜底三条路径。

package report

import (
	"errors"
	"strings"
	"testing"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
)

// TestGenerateInspectionWithoutLLM 验证不传 Summarizer 时仍能产出 OpsDoc，body 走纯事实分支。
func TestGenerateInspectionWithoutLLM(t *testing.T) {
	env, wf, rec := sampleData()
	doc, err := GenerateInspection(env, wf, rec, core.AISession{}, nil)
	if err != nil {
		t.Fatalf("GenerateInspection error: %v", err)
	}
	if doc.Kind != core.OpsDocKindInspection {
		t.Fatalf("kind = %q", doc.Kind)
	}
	if doc.Source.ExecutionID != "exec-1" || doc.Source.WorkflowID != "wf-1" {
		t.Fatalf("source 未带入: %#v", doc.Source)
	}
	if !strings.Contains(doc.Body, "未启用 LLM 风险分析") {
		t.Fatal("纯事实报告应保留 LLM 占位提示")
	}
}

// TestGenerateInspectionWithLLM 验证 Summarizer 输出被注入到风险段。
func TestGenerateInspectionWithLLM(t *testing.T) {
	env, wf, rec := sampleData()
	called := 0
	sum := Summarizer(func(messages []clients.ChatMessage) (string, error) {
		called++
		// 校验 prompt 包含事实摘要片段。
		if !strings.Contains(messages[0].Content, "环境=生产环境") {
			t.Fatalf("LLM prompt 缺少事实摘要: %s", messages[0].Content)
		}
		return "- 磁盘检查失败，建议立即扩容根分区", nil
	})
	doc, err := GenerateInspection(env, wf, rec, core.AISession{ID: "sess-1"}, sum)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if called != 1 {
		t.Fatalf("Summarizer should be called once, got %d", called)
	}
	if !strings.Contains(doc.Body, "立即扩容根分区") {
		t.Fatal("LLM 输出未注入正文")
	}
	if doc.Source.SessionID != "sess-1" {
		t.Fatalf("SessionID 未带入: %s", doc.Source.SessionID)
	}
}

// TestGenerateInspectionLLMFailureIsNotFatal 验证 LLM 失败时报告仍生成，错误被写入风险段。
func TestGenerateInspectionLLMFailureIsNotFatal(t *testing.T) {
	env, wf, rec := sampleData()
	sum := Summarizer(func(_ []clients.ChatMessage) (string, error) {
		return "", errors.New("timeout")
	})
	doc, err := GenerateInspection(env, wf, rec, core.AISession{}, sum)
	if err != nil {
		t.Fatalf("LLM 失败不应抛错: %v", err)
	}
	if !strings.Contains(doc.Body, "LLM 风险分析失败") || !strings.Contains(doc.Body, "timeout") {
		t.Fatalf("失败原因未回显: %s", doc.Body)
	}
}
