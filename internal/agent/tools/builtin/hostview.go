// host_status 视图构造：解析 ssh_inspect 脚本尾部的 __VIEW__ 机器可读段。
// 解析全程宽容——任何字段缺失/格式异常都只是该字段缺省，绝不让视图问题影响工具本身。

package builtin

import (
	"encoding/json"
	"strconv"
	"strings"

	"OpsEngine/internal/core"
)

// hostStatusData 是 host_status 视图的 Data JSON 结构（与前端 HostStatusCard 对齐）。
type hostStatusData struct {
	Hostname   string          `json:"hostname,omitempty"`
	OS         string          `json:"os,omitempty"`
	UptimeSec  int64           `json:"uptime_sec,omitempty"`
	Cores      int             `json:"cores,omitempty"`
	Load1      float64         `json:"load1,omitempty"`
	CPUPercent float64         `json:"cpu_percent,omitempty"` // -1 表示未采集到
	MemTotal   int64           `json:"mem_total,omitempty"`
	MemUsed    int64           `json:"mem_used,omitempty"`
	Disks      []hostDiskEntry `json:"disks,omitempty"`
	Procs      []hostProcEntry `json:"procs,omitempty"`
}

type hostDiskEntry struct {
	Mount string `json:"mount"`
	Total int64  `json:"total"`
	Used  int64  `json:"used"`
}

type hostProcEntry struct {
	PID  string  `json:"pid"`
	User string  `json:"user"`
	CPU  float64 `json:"cpu"`
	Mem  float64 `json:"mem"`
	Cmd  string  `json:"cmd"`
}

// viewMarker 是脚本输出中人读段与机器段的分隔标记。
const viewMarker = "## __VIEW__"

// splitHostStatusView 把脚本输出拆为「给模型的文本」与「给前端的视图」。
// 没有 __VIEW__ 段（老系统/精简发行版命令缺失）时视图为 nil，文本原样返回。
func splitHostStatusView(out, envName string) (string, *core.AIViewPayload) {
	idx := strings.Index(out, viewMarker)
	if idx < 0 {
		return out, nil
	}
	text := strings.TrimRight(out[:idx], "\n")
	data := parseHostStatus(out[idx+len(viewMarker):])
	raw, err := json.Marshal(data)
	if err != nil {
		return text, nil
	}
	title := data.Hostname
	if title == "" {
		title = envName
	}
	return text, &core.AIViewPayload{
		Kind:  "host_status",
		Title: "主机状态 · " + title,
		Data:  string(raw),
	}
}

// parseHostStatus 解析机器可读段。
func parseHostStatus(section string) hostStatusData {
	data := hostStatusData{CPUPercent: -1}
	var stat1, stat2 string
	mode := "" // "" / disk / proc
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch line {
		case "__DISK__":
			mode = "disk"
			continue
		case "__PROC__":
			mode = "proc"
			continue
		}
		switch mode {
		case "disk":
			if d, ok := parseDiskLine(line); ok {
				data.Disks = append(data.Disks, d)
			}
		case "proc":
			if p, ok := parseProcLine(line); ok {
				data.Procs = append(data.Procs, p)
			}
		default:
			key, value, found := strings.Cut(line, "=")
			if !found {
				continue
			}
			value = strings.TrimSpace(value)
			switch key {
			case "hostname":
				data.Hostname = value
			case "os":
				data.OS = value
			case "uptime_sec":
				data.UptimeSec, _ = strconv.ParseInt(value, 10, 64)
			case "cores":
				data.Cores, _ = strconv.Atoi(value)
			case "loadavg":
				if parts := strings.Fields(value); len(parts) > 0 {
					data.Load1, _ = strconv.ParseFloat(parts[0], 64)
				}
			case "stat1":
				stat1 = value
			case "stat2":
				stat2 = value
			case "mem":
				if total, used, ok := parseMem(value); ok {
					data.MemTotal, data.MemUsed = total, used
				}
			}
		}
	}
	if pct, ok := cpuPercentFromStats(stat1, stat2); ok {
		data.CPUPercent = pct
	}
	return data
}

// parseDiskLine 解析 "挂载点|总字节|已用字节"。
func parseDiskLine(line string) (hostDiskEntry, bool) {
	parts := strings.Split(line, "|")
	if len(parts) != 3 {
		return hostDiskEntry{}, false
	}
	total, err1 := strconv.ParseInt(parts[1], 10, 64)
	used, err2 := strconv.ParseInt(parts[2], 10, 64)
	if err1 != nil || err2 != nil || total <= 0 {
		return hostDiskEntry{}, false
	}
	return hostDiskEntry{Mount: parts[0], Total: total, Used: used}, true
}

// parseProcLine 解析 "pid|user|pcpu|pmem|comm"。
func parseProcLine(line string) (hostProcEntry, bool) {
	parts := strings.Split(line, "|")
	if len(parts) != 5 {
		return hostProcEntry{}, false
	}
	cpu, _ := strconv.ParseFloat(parts[2], 64)
	mem, _ := strconv.ParseFloat(parts[3], 64)
	return hostProcEntry{PID: parts[0], User: parts[1], CPU: cpu, Mem: mem, Cmd: parts[4]}, true
}

// parseMem 解析 "total,used"（字节）。
func parseMem(value string) (int64, int64, bool) {
	parts := strings.Split(value, ",")
	if len(parts) != 2 {
		return 0, 0, false
	}
	total, err1 := strconv.ParseInt(parts[0], 10, 64)
	used, err2 := strconv.ParseInt(parts[1], 10, 64)
	if err1 != nil || err2 != nil || total <= 0 {
		return 0, 0, false
	}
	return total, used, true
}

// cpuPercentFromStats 用两次 /proc/stat cpu 行的差值算瞬时 CPU 使用率。
// 行格式：cpu user nice system idle iowait irq softirq steal ...
func cpuPercentFromStats(s1, s2 string) (float64, bool) {
	v1, ok1 := parseStatLine(s1)
	v2, ok2 := parseStatLine(s2)
	if !ok1 || !ok2 {
		return 0, false
	}
	totalDelta := v2.total - v1.total
	idleDelta := v2.idle - v1.idle
	if totalDelta <= 0 {
		return 0, false
	}
	pct := float64(totalDelta-idleDelta) / float64(totalDelta) * 100
	if pct < 0 || pct > 100 {
		return 0, false
	}
	return pct, true
}

type cpuStat struct{ total, idle int64 }

// parseStatLine 解析 /proc/stat 的 cpu 汇总行。idle = idle + iowait。
func parseStatLine(line string) (cpuStat, bool) {
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuStat{}, false
	}
	var st cpuStat
	for i, f := range fields[1:] {
		n, err := strconv.ParseInt(f, 10, 64)
		if err != nil {
			return cpuStat{}, false
		}
		st.total += n
		// 第 4、5 列（idle、iowait）计入空闲
		if i == 3 || i == 4 {
			st.idle += n
		}
	}
	return st, true
}
