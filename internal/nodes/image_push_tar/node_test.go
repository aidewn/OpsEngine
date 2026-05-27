// image_push_tar 单元测试
// 重点：parseLoadedRef 与 drainPushResponse 的纯解析逻辑
// （Execute 依赖真实 docker daemon，留给集成测试）

package image_push_tar

import (
	"strings"
	"testing"

	"OpsEngine/internal/core"
)

func TestTypeDef(t *testing.T) {
	def := (Node{}).TypeDef()
	if def.TypeID != "image_push_tar" {
		t.Fatalf("TypeID 不匹配: %s", def.TypeID)
	}
	if def.NodeKind != core.NodeKindAction {
		t.Fatalf("NodeKind 不匹配: %s", def.NodeKind)
	}
	// 必须有 docker 输入和 image 输出
	hasDocker := false
	for _, p := range def.InputPorts {
		if p.ID == "docker" && p.PortType == core.PortTypeDockerContext {
			hasDocker = true
		}
	}
	if !hasDocker {
		t.Fatal("缺少 docker 输入端口（DockerContext）")
	}
	hasImage := false
	for _, p := range def.OutputPorts {
		if p.ID == "image" && p.PortType == core.PortTypeString {
			hasImage = true
		}
	}
	if !hasImage {
		t.Fatal("缺少 image 输出端口（String）")
	}
}

// parseLoadedRef：典型 docker load 输出 → 抽取 repo:tag
func TestParseLoadedRefRepoTag(t *testing.T) {
	body := strings.NewReader(`{"stream":"Loaded image: nginx:1.19\n"}` + "\n")
	ref, err := parseLoadedRef(body, nil)
	if err != nil {
		t.Fatalf("parseLoadedRef 失败: %v", err)
	}
	if ref != "nginx:1.19" {
		t.Fatalf("ref 不匹配: %s", ref)
	}
}

// 只有 ID 时退化使用 ID
func TestParseLoadedRefIDOnly(t *testing.T) {
	body := strings.NewReader(`{"stream":"Loaded image ID: sha256:abcdef\n"}` + "\n")
	ref, err := parseLoadedRef(body, nil)
	if err != nil {
		t.Fatalf("parseLoadedRef 失败: %v", err)
	}
	if ref != "sha256:abcdef" {
		t.Fatalf("ref 不匹配: %s", ref)
	}
}

// 两条都有时 repo:tag 优先（无论先后顺序）
func TestParseLoadedRefPrefersRepoTag(t *testing.T) {
	body := strings.NewReader(
		`{"stream":"Loaded image ID: sha256:abcdef\n"}` + "\n" +
			`{"stream":"Loaded image: nginx:1.19\n"}` + "\n",
	)
	ref, err := parseLoadedRef(body, nil)
	if err != nil {
		t.Fatalf("parseLoadedRef 失败: %v", err)
	}
	if ref != "nginx:1.19" {
		t.Fatalf("期望 repo:tag 优先，得到: %s", ref)
	}
}

// daemon 报错路径 → 直接 surface 出来
func TestParseLoadedRefSurfacesError(t *testing.T) {
	body := strings.NewReader(`{"error":"open xxx: no such file"}` + "\n")
	_, err := parseLoadedRef(body, nil)
	if err == nil || !strings.Contains(err.Error(), "docker load 报错") {
		t.Fatalf("应 surface docker load 报错，得到: %v", err)
	}
}

// drainPushResponse：error 字段必须中断
func TestDrainPushResponseSurfacesError(t *testing.T) {
	body := strings.NewReader(`{"error":"denied: requested access to the resource is denied"}` + "\n")
	err := drainPushResponse(body, nil)
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("应 surface push 错误，得到: %v", err)
	}
}

// 正常 progress + status 输出 → 无错
func TestDrainPushResponseHappy(t *testing.T) {
	body := strings.NewReader(
		`{"status":"The push refers to repository [reg.example.com/foo]"}` + "\n" +
			`{"status":"Pushed","progressDetail":{"current":1,"total":1},"id":"abc"}` + "\n" +
			`{"status":"latest: digest: sha256:abc size: 100"}` + "\n",
	)
	if err := drainPushResponse(body, nil); err != nil {
		t.Fatalf("正常路径应无错: %v", err)
	}
}
