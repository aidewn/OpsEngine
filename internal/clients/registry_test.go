// registry helper 单元测试
// 覆盖 NormalizeRegistryHost + BuildRegistryAuth + PingRegistry 的关键状态码分支
// PingRegistry 用 httptest.NewServer 模拟 200 / 401 / 500 三种响应

package clients

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNormalizeRegistryHost(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://reg.example.com:5000", "reg.example.com:5000"},
		{"http://reg.example.com", "reg.example.com"},
		{"reg.example.com", "reg.example.com"},
		{"reg.example.com/", "reg.example.com"},
	}
	for _, c := range cases {
		got, err := NormalizeRegistryHost(c.in)
		if err != nil {
			t.Fatalf("NormalizeRegistryHost(%q) 失败: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("NormalizeRegistryHost(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeRegistryHostRejectsEmpty(t *testing.T) {
	if _, err := NormalizeRegistryHost(""); err == nil {
		t.Fatal("空 url 应报错")
	}
}

// BuildRegistryAuth：匿名情况返回空串；带凭据时是合法 base64(json)
func TestBuildRegistryAuth(t *testing.T) {
	anon, err := BuildRegistryAuth("", "", "reg.example.com")
	if err != nil {
		t.Fatalf("匿名 BuildRegistryAuth 失败: %v", err)
	}
	if anon != "" {
		t.Fatalf("匿名 BuildRegistryAuth 应返回空串，得到 %q", anon)
	}

	token, err := BuildRegistryAuth("alice", "p@ss", "reg.example.com")
	if err != nil {
		t.Fatalf("带凭据 BuildRegistryAuth 失败: %v", err)
	}
	raw, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("token 不是合法 base64: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("token 不是合法 JSON: %v", err)
	}
	if cfg["username"] != "alice" || cfg["password"] != "p@ss" {
		t.Fatalf("token 字段不一致: %+v", cfg)
	}
}

// PingRegistry 200 → 成功
func TestPingRegistryOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/" {
			t.Fatalf("path 不是 /v2/: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	if err := PingRegistry(srv.URL, "", "", 2*time.Second); err != nil {
		t.Fatalf("200 应成功: %v", err)
	}
}

// 401 + 无凭据 → 返回「需要认证」错误
func TestPingRegistry401AnonymousReportsAuthRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	err := PingRegistry(srv.URL, "", "", 2*time.Second)
	if err == nil || !strings.Contains(err.Error(), "要求认证") {
		t.Fatalf("401 匿名应提示要求认证，得到: %v", err)
	}
}

// 401 + 带凭据 → 返回「认证失败」错误
func TestPingRegistry401WithCredentialsReportsAuthFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	err := PingRegistry(srv.URL, "alice", "wrong", 2*time.Second)
	if err == nil || !strings.Contains(err.Error(), "认证失败") {
		t.Fatalf("401 带凭据应提示认证失败，得到: %v", err)
	}
}

// 500 → 直接报错
func TestPingRegistry500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	err := PingRegistry(srv.URL, "", "", 2*time.Second)
	if err == nil {
		t.Fatal("500 应报错")
	}
}

// user/password 只填一个 → 由 testRegistryConfig 处的校验把关，这里不重复
