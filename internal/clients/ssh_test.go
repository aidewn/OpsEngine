package clients

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLinuxSshClientMarshalJSONIsSafe(t *testing.T) {
	client := NewLinuxSshClient(nil, "10.0.0.1", 22, "root")

	data, err := json.Marshal(client)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}

	if got["type"] != "LinuxSshConnection" {
		t.Fatalf("类型不匹配: %v", got["type"])
	}
	if got["host"] != "10.0.0.1" {
		t.Fatalf("host 不匹配: %v", got["host"])
	}
	if got["user"] != "root" {
		t.Fatalf("user 不匹配: %v", got["user"])
	}
	if _, ok := got["password"]; ok {
		t.Fatalf("不应序列化 password 字段")
	}
}

func TestParseLinuxSshDialConfigDefaultsToPassword(t *testing.T) {
	cfg, err := ParseLinuxSshDialConfig(map[string]any{
		"host":            "10.0.0.1",
		"port":            float64(2222),
		"user":            "root",
		"password":        "secret",
		"timeout_seconds": "15",
	})
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if cfg.AuthType != SSHAuthPassword {
		t.Fatalf("认证方式不匹配: %s", cfg.AuthType)
	}
	if cfg.Port != 2222 || cfg.TimeoutSeconds != 15 {
		t.Fatalf("端口或超时不匹配: %#v", cfg)
	}
}

func TestParseLinuxSshDialConfigPrivateKeyRequiresPath(t *testing.T) {
	_, err := ParseLinuxSshDialConfig(map[string]any{
		"host":      "10.0.0.1",
		"user":      "root",
		"auth_type": SSHAuthPrivateKey,
	})
	if err == nil || !strings.Contains(err.Error(), "private_key_path") {
		t.Fatalf("期望 private_key_path 错误，实际: %v", err)
	}
}
