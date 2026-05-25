package clients

import (
	"encoding/json"
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
