// 数据源单测：覆盖 host 解析（/proc、df）、docker/k8s 的 ProbeItem 映射、http 探活。
// SSH/Docker/K8s 的真实拨号依赖外部环境，不在单测内；这里只测可纯函数化的解析与映射。

package monitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"OpsEngine/internal/probe"
)

func TestParseHostBasic(t *testing.T) {
	out := `__LOADAVG__
0.52 0.48 0.40 1/234 5678
__UPTIME__
123456.78 100000.00
__MEMINFO__
MemTotal:        8000000 kB
MemFree:         1000000 kB
MemAvailable:    2000000 kB
__STAT1__
cpu  100 0 100 800 0 0 0 0 0 0
__STAT2__
cpu  150 0 150 900 0 0 0 0 0 0`

	res, err := parseHostBasic(out)
	if err != nil {
		t.Fatalf("parseHostBasic 失败: %v", err)
	}
	if res.Load1 != 0.52 || res.Load5 != 0.48 || res.Load15 != 0.40 {
		t.Fatalf("loadavg 解析错误: %+v", res)
	}
	if res.UptimeSeconds != 123456.78 {
		t.Fatalf("uptime 解析错误: %v", res.UptimeSeconds)
	}
	if res.MemTotalKB != 8000000 || res.MemAvailableKB != 2000000 {
		t.Fatalf("meminfo 解析错误: %+v", res)
	}
	// 内存使用率 = (8000000-2000000)/8000000 = 75%
	if res.MemUsagePercent != 75 {
		t.Fatalf("内存使用率错误: %v", res.MemUsagePercent)
	}
	// CPU：dTotal = (150+150+900)-(100+100+800)=1200-1000=200; dIdle=900-800=100; usage=(1-100/200)*100=50
	if res.CPUUsagePercent != 50 {
		t.Fatalf("CPU 使用率错误: %v", res.CPUUsagePercent)
	}
}

func TestParseDF(t *testing.T) {
	out := `Filesystem     1024-blocks     Used Available Capacity Mounted on
/dev/sda1         51474044 12868511  35982696      27% /
tmpfs              4096000        0   4096000       0% /dev/shm`

	fs := parseDF(out)
	if len(fs) != 2 {
		t.Fatalf("期望 2 个挂载点，实际 %d", len(fs))
	}
	root := fs[0]
	if root.Mount != "/" || root.UsePercent != 27 || root.SizeKB != 51474044 || root.UsedKB != 12868511 {
		t.Fatalf("根分区解析错误: %+v", root)
	}
}

func TestParseDF_MountWithSpace(t *testing.T) {
	out := `Filesystem 1024-blocks Used Available Capacity Mounted on
/dev/sdb1 1000 500 500 50% /mnt/my data`
	fs := parseDF(out)
	if len(fs) != 1 || fs[0].Mount != "/mnt/my data" {
		t.Fatalf("含空格挂载点解析错误: %+v", fs)
	}
}

func TestMapDockerItems(t *testing.T) {
	items := []probe.ProbeItem{
		{Key: "abc123", Label: "nginx", Meta: map[string]any{
			"image": "nginx:1.25", "state": "running", "status": "Up 3 hours",
		}},
		{Key: "def456", Label: "redis"}, // 无 Meta，应安全降级
	}
	cs := mapDockerItems(items)
	if len(cs) != 2 {
		t.Fatalf("期望 2 个容器，实际 %d", len(cs))
	}
	if cs[0].Image != "nginx:1.25" || cs[0].State != "running" {
		t.Fatalf("容器映射错误: %+v", cs[0])
	}
	if cs[1].Name != "redis" || cs[1].State != "" {
		t.Fatalf("无 Meta 容器降级错误: %+v", cs[1])
	}
}

func TestMapK8sItems_UnavailableReplicas(t *testing.T) {
	items := []probe.ProbeItem{
		{Meta: map[string]any{
			"kind": "Deployment", "namespace": "default", "name": "web",
			"container": "app", "current_image": "web:v1",
			"replicas": int64(3), "ready_replicas": int64(1),
		}},
		{Meta: map[string]any{ // float64 兼容（经 JSON 往返的情形）
			"kind": "Deployment", "name": "ok", "replicas": float64(2), "ready_replicas": float64(2),
		}},
	}
	ws := mapK8sItems(items)
	if ws[0].UnavailableReplicas != 2 {
		t.Fatalf("不可用副本计算错误: %+v", ws[0])
	}
	if ws[1].UnavailableReplicas != 0 {
		t.Fatalf("全就绪不应有不可用副本: %+v", ws[1])
	}
}

func TestHTTPHealthSource_Collect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	src := newHTTPHealthSource()
	data, err := src.Collect(context.Background(), CollectContext{
		Task: CollectionTask{Kind: KindHTTPHealth, Params: map[string]any{"url": srv.URL}},
	})
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}
	res := data.(HTTPHealthResult)
	if res.StatusCode != 200 || !res.OK {
		t.Fatalf("探活结果错误: %+v", res)
	}
}

func TestHTTPHealthSource_ExpectStatusMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	src := newHTTPHealthSource()
	data, err := src.Collect(context.Background(), CollectContext{
		Task: CollectionTask{Kind: KindHTTPHealth, Params: map[string]any{"url": srv.URL, "expect_status": 200}},
	})
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}
	res := data.(HTTPHealthResult)
	if res.StatusCode != 500 || res.OK {
		t.Fatalf("期望 OK=false（500≠200）: %+v", res)
	}
}

func TestHTTPHealthSource_MissingURL(t *testing.T) {
	src := newHTTPHealthSource()
	_, err := src.Collect(context.Background(), CollectContext{Task: CollectionTask{Kind: KindHTTPHealth}})
	if err == nil {
		t.Fatal("缺少 url 应报错")
	}
}

func TestRegisterDefaults(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry 失败: %v", err)
	}
	for _, kind := range []string{
		KindHostBasic, KindHostDisk, KindDockerContainers, KindK8sWorkloads, KindHTTPHealth,
	} {
		if _, ok := reg.Get(kind); !ok {
			t.Fatalf("默认注册表缺少数据源: %s", kind)
		}
	}
}
