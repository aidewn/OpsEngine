package clients

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewLinuxFileHandleValidatesInput(t *testing.T) {
	ssh := NewLinuxSshClient(nil, "10.0.0.1", 22, "root")

	cases := map[string]struct {
		client    *LinuxSshClient
		path      string
		wantErr   bool
		wantMatch string
	}{
		"missing client":  {nil, "/tmp/a", true, "未提供"},
		"empty path":      {ssh, "", true, "路径为空"},
		"newline in path": {ssh, "/tmp/a\nb", true, "换行"},
		"valid":           {ssh, "/etc/hosts", false, ""},
	}
	for name, tc := range cases {
		h, err := NewLinuxFileHandle(tc.client, tc.path)
		if tc.wantErr {
			if err == nil || !strings.Contains(err.Error(), tc.wantMatch) {
				t.Fatalf("%s: 期望错误包含 %q，实际: %v", name, tc.wantMatch, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: 不应报错: %v", name, err)
		}
		if h.Path != tc.path {
			t.Fatalf("%s: path 不匹配", name)
		}
	}
}

func TestLinuxFileHandleMarshalJSONIsSafe(t *testing.T) {
	ssh := NewLinuxSshClient(nil, "10.0.0.1", 22, "root")
	h, err := NewLinuxFileHandle(ssh, "/etc/hosts")
	if err != nil {
		t.Fatalf("构造 handle 失败: %v", err)
	}

	data, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if got["type"] != "LinuxFileHandle" {
		t.Fatalf("type 不匹配: %v", got["type"])
	}
	if got["path"] != "/etc/hosts" {
		t.Fatalf("path 不匹配: %v", got["path"])
	}
	if got["host"] != "10.0.0.1" || got["user"] != "root" {
		t.Fatalf("host/user 不匹配: %+v", got)
	}
	for _, leaked := range []string{"Client", "client", "password", "sftp"} {
		if _, ok := got[leaked]; ok {
			t.Fatalf("不应序列化 %s", leaked)
		}
	}
}
