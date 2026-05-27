// env_probe_k8s_workloads 单元测试
// 重点：flattenContainers 把多容器 workload 展平为多项；TypeDef 关键字段
// Probe 主流程依赖真实 K8s API，留给集成测试

package env_probe_k8s_workloads

import (
	"strings"
	"testing"

	"OpsEngine/internal/core"

	corev1 "k8s.io/api/core/v1"
)

func TestTypeDef(t *testing.T) {
	def := (Node{}).TypeDef()
	if def.TypeID != "env_probe_k8s_workloads" {
		t.Fatalf("TypeID 不匹配: %s", def.TypeID)
	}
	if def.NodeKind != core.NodeKindPure {
		t.Fatalf("NodeKind 应为 Pure，得到: %s", def.NodeKind)
	}
	// 输出 selected_workload / workloads / count 三个端口
	wantOut := map[string]core.PortType{
		"selected_workload": core.PortTypeString,
		"workloads":         core.PortTypeAny,
		"count":             core.PortTypeInt,
	}
	got := map[string]core.PortType{}
	for _, p := range def.OutputPorts {
		got[p.ID] = p.PortType
	}
	for id, pt := range wantOut {
		if got[id] != pt {
			t.Fatalf("输出端口 %s 类型不匹配: got %s want %s", id, got[id], pt)
		}
	}
}

// flattenContainers：每个 container 都展开为一项，key 编码 kind/name/container
func TestFlattenContainers(t *testing.T) {
	cs := []corev1.Container{
		{Name: "api", Image: "nginx:1.19"},
		{Name: "sidecar", Image: "envoy:1.30"},
	}
	items := flattenContainers("Deployment", "default", "myapp", cs, 3, 2)
	if len(items) != 2 {
		t.Fatalf("应展平为 2 项，得到 %d", len(items))
	}
	wantKeys := []string{"Deployment/myapp/api", "Deployment/myapp/sidecar"}
	for i, w := range wantKeys {
		if items[i].Key != w {
			t.Fatalf("items[%d].Key = %s want %s", i, items[i].Key, w)
		}
	}
	// label 应包含 image，便于用户在 picker 里直接看到
	if !strings.Contains(items[0].Label, "nginx:1.19") {
		t.Fatalf("label 应含 image，得到: %s", items[0].Label)
	}
	// meta 必须含 container + current_image + replicas + ready_replicas
	meta := items[0].Meta
	if meta["container"] != "api" {
		t.Fatalf("meta.container 错误: %v", meta["container"])
	}
	if meta["current_image"] != "nginx:1.19" {
		t.Fatalf("meta.current_image 错误: %v", meta["current_image"])
	}
	if meta["replicas"] != int64(3) || meta["ready_replicas"] != int64(2) {
		t.Fatalf("replicas/ready_replicas 错误: %+v", meta)
	}
}

// 空容器列表 → 空 items
func TestFlattenContainersEmpty(t *testing.T) {
	items := flattenContainers("StatefulSet", "default", "x", nil, 0, 0)
	if len(items) != 0 {
		t.Fatalf("空容器应返回空 items，得到 %d", len(items))
	}
}
