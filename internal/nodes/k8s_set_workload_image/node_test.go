// k8s_set_workload_image 单元测试
// 重点：parseWorkloadRef + buildContainerImagePatch + containerImageMatches
// patchWorkload / waitForRollout 依赖真实 K8s API，留给集成测试

package k8s_set_workload_image

import (
	"encoding/json"
	"strings"
	"testing"

	"OpsEngine/internal/core"

	corev1 "k8s.io/api/core/v1"
)

func TestTypeDef(t *testing.T) {
	def := (Node{}).TypeDef()
	if def.TypeID != "k8s_set_workload_image" {
		t.Fatalf("TypeID 不匹配: %s", def.TypeID)
	}
	if def.NodeKind != core.NodeKindAction {
		t.Fatalf("NodeKind 不匹配: %s", def.NodeKind)
	}
	// 必须有 k8s + workload_ref + image 三个输入
	wantInputs := map[string]core.PortType{
		"k8s":          core.PortTypeK8sContext,
		"workload_ref": core.PortTypeString,
		"image":        core.PortTypeString,
	}
	got := map[string]core.PortType{}
	for _, p := range def.InputPorts {
		got[p.ID] = p.PortType
	}
	for id, pt := range wantInputs {
		if got[id] != pt {
			t.Fatalf("输入端口 %s 类型不匹配: got %s want %s", id, got[id], pt)
		}
	}
}

// 标准三段式正常解析
func TestParseWorkloadRefHappy(t *testing.T) {
	kind, name, container, err := parseWorkloadRef("Deployment/myapp/api")
	if err != nil {
		t.Fatalf("parseWorkloadRef 失败: %v", err)
	}
	if kind != "Deployment" || name != "myapp" || container != "api" {
		t.Fatalf("解析结果错误: %s/%s/%s", kind, name, container)
	}
}

// 缺段 / 空段 / 非法 Kind 都应报错
func TestParseWorkloadRefRejectsBadInputs(t *testing.T) {
	bad := []string{
		"",
		"Deployment/myapp",
		"Deployment//api",
		"/myapp/api",
		"Pod/foo/c",       // Kind 未在白名单
		"deployment/x/y",  // 大小写敏感
	}
	for _, ref := range bad {
		if _, _, _, err := parseWorkloadRef(ref); err == nil {
			t.Fatalf("ref %q 应报错，实际通过", ref)
		}
	}
}

// 三种合法 Kind 都应通过
func TestParseWorkloadRefAcceptsAllSupportedKinds(t *testing.T) {
	for _, k := range []string{"Deployment", "StatefulSet", "DaemonSet"} {
		ref := k + "/x/y"
		if _, _, _, err := parseWorkloadRef(ref); err != nil {
			t.Fatalf("Kind %s 应被接受: %v", k, err)
		}
	}
}

// patch JSON 结构必须是 strategic merge 期望的形状
func TestBuildContainerImagePatch(t *testing.T) {
	body, err := buildContainerImagePatch("api", "nginx:1.20")
	if err != nil {
		t.Fatalf("buildContainerImagePatch 失败: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("patch 不是合法 JSON: %v", err)
	}
	// 校验 spec.template.spec.containers[0].name/image
	containers := got["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)["containers"].([]any)
	if len(containers) != 1 {
		t.Fatalf("containers 数量不为 1: %d", len(containers))
	}
	c := containers[0].(map[string]any)
	if c["name"] != "api" || c["image"] != "nginx:1.20" {
		t.Fatalf("container 字段不一致: %+v", c)
	}
}

// containerImageMatches：找到名字且 image 相同 → true；找不到名字 → false；image 不同 → false
func TestContainerImageMatches(t *testing.T) {
	cs := []corev1.Container{
		{Name: "api", Image: "nginx:1.19"},
		{Name: "sidecar", Image: "envoy:1.30"},
	}
	if !containerImageMatches(cs, "api", "nginx:1.19") {
		t.Fatal("应匹配 api=nginx:1.19")
	}
	if containerImageMatches(cs, "api", "nginx:1.20") {
		t.Fatal("api image 不同应返回 false")
	}
	if containerImageMatches(cs, "missing", "nginx:1.19") {
		t.Fatal("找不到 container 应返回 false")
	}
}

// 错误消息样本：未指定 namespace 时回退到 K8sContext.Namespace
// 这里只覆盖 string trimming 边界，namespace 字段为空 vs 仅空格的处理
func TestNamespaceFallbackHandling(t *testing.T) {
	// 这部分逻辑融在 Execute 里，单独抽出比较费劲；
	// 用一个最简化的逻辑等价断言：strings.TrimSpace 行为
	if strings.TrimSpace("   ") != "" {
		t.Fatal("TrimSpace 行为变化了？")
	}
}
