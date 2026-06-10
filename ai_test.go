// AI 助手 Wails 适配层的薄壳测试。
// 业务逻辑（意图、prompt、上下文、handler、会话标题）均在 internal/agent/* 各子包内测试。

package main

import (
	"strings"
	"testing"
)

// TestNormalizeAISettingsTrimsAndFillsDefaults 验证 trim + 默认值补齐。
func TestNormalizeAISettingsTrimsAndFillsDefaults(t *testing.T) {
	got := normalizeAISettings(AISettings{
		DeepSeekAPIKey:  "  sk-test  ",
		DeepSeekBaseURL: "",
		DeepSeekModel:   "   ",
		TimeoutSeconds:  0,
	})
	if got.DeepSeekAPIKey != "sk-test" {
		t.Fatalf("API Key trim 失败: %q", got.DeepSeekAPIKey)
	}
	if got.DeepSeekBaseURL == "" || got.DeepSeekModel == "" {
		t.Fatalf("空字段未补默认: %#v", got)
	}
	if got.TimeoutSeconds != 120 {
		t.Fatalf("Timeout 未补默认: %d", got.TimeoutSeconds)
	}
}

// TestDefaultAISettings 验证默认配置补齐 BaseURL / Model / Timeout。
func TestDefaultAISettings(t *testing.T) {
	got := defaultAISettings()
	if got.DeepSeekBaseURL == "" || got.DeepSeekModel == "" || got.TimeoutSeconds <= 0 {
		t.Fatalf("默认配置不完整: %#v", got)
	}
}

// TestUpdateAISettingsRequiresAPIKey 验证空 API Key 被拒绝。
func TestUpdateAISettingsRequiresAPIKey(t *testing.T) {
	app := &App{}
	err := app.UpdateAISettings(AISettings{DeepSeekAPIKey: "   "})
	if err == nil || !strings.Contains(err.Error(), "API Key") {
		t.Fatalf("expected API Key error, got %v", err)
	}
}
