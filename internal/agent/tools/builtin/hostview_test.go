// host_status 解析单测：完整段、缺段降级、CPU 差值计算。
package builtin

import (
	"encoding/json"
	"strings"
	"testing"
)

const sampleInspectOut = `## 系统信息
Linux web-1 5.15.0 x86_64
## 磁盘
/dev/sda1 ...
## __VIEW__
hostname=web-1
os=Ubuntu 22.04.3 LTS
uptime_sec=86400
cores=4
loadavg=0.52 0.48 0.40
stat1=cpu 100 0 100 800 0 0 0 0
stat2=cpu 150 0 150 900 0 0 0 0
mem=8000000000,3000000000
__DISK__
/|50000000000|30000000000
/data|100000000000|95000000000
__PROC__
1234|root|12.5|3.1|nginx
5678|app|8.0|10.2|java
`

// TestSplitHostStatusView 验证文本/视图拆分与字段解析。
func TestSplitHostStatusView(t *testing.T) {
	text, view := splitHostStatusView(sampleInspectOut, "prod")
	if strings.Contains(text, "__VIEW__") || !strings.Contains(text, "## 系统信息") {
		t.Fatalf("文本段拆分异常: %q", text)
	}
	if view == nil || view.Kind != "host_status" || !strings.Contains(view.Title, "web-1") {
		t.Fatalf("视图载荷异常: %#v", view)
	}
	var data hostStatusData
	if err := json.Unmarshal([]byte(view.Data), &data); err != nil {
		t.Fatalf("Data 不是合法 JSON: %v", err)
	}
	if data.Hostname != "web-1" || data.Cores != 4 || data.Load1 != 0.52 {
		t.Fatalf("基础字段异常: %#v", data)
	}
	// CPU: 总差值 200，空闲差值 100 → 50%
	if data.CPUPercent < 49.9 || data.CPUPercent > 50.1 {
		t.Fatalf("CPU 差值计算异常: %v", data.CPUPercent)
	}
	if data.MemTotal != 8000000000 || data.MemUsed != 3000000000 {
		t.Fatalf("内存解析异常: %#v", data)
	}
	if len(data.Disks) != 2 || data.Disks[1].Mount != "/data" {
		t.Fatalf("磁盘解析异常: %#v", data.Disks)
	}
	if len(data.Procs) != 2 || data.Procs[0].Cmd != "nginx" {
		t.Fatalf("进程解析异常: %#v", data.Procs)
	}
}

// TestSplitHostStatusViewNoMarker 验证缺机器段时静默降级。
func TestSplitHostStatusViewNoMarker(t *testing.T) {
	text, view := splitHostStatusView("## 系统信息\nLinux ...", "prod")
	if view != nil || !strings.Contains(text, "Linux") {
		t.Fatalf("缺标记时应降级: %q %#v", text, view)
	}
}
