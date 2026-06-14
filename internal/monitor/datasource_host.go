// host.basic / host.disk 数据源：通过 SSH 读取 /proc 与 df，解析为结构化指标。
//
// 取数走 Linux 通用接口（/proc/loadavg、/proc/meminfo、/proc/stat、df -kP），
// 不依赖发行版特定命令；解析逻辑与 SSH 分离，便于纯函数单测。

package monitor

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
)

// Kind 标识。
const (
	KindHostBasic = "host.basic"
	KindHostDisk  = "host.disk"
)

// sshCmdTimeout 单次采集命令的硬超时。
const sshCmdTimeout = 20 * time.Second

// HostBasicResult 主机基础指标。
type HostBasicResult struct {
	CPUUsagePercent float64 `json:"cpu_usage_percent"`
	MemTotalKB      int64   `json:"mem_total_kb"`
	MemAvailableKB  int64   `json:"mem_available_kb"`
	MemUsagePercent float64 `json:"mem_usage_percent"`
	Load1           float64 `json:"load1"`
	Load5           float64 `json:"load5"`
	Load15          float64 `json:"load15"`
	UptimeSeconds   float64 `json:"uptime_seconds"`
}

// DiskFilesystem 单个挂载点的容量信息。
type DiskFilesystem struct {
	Filesystem string `json:"filesystem"`
	Mount      string `json:"mount"`
	SizeKB     int64  `json:"size_kb"`
	UsedKB     int64  `json:"used_kb"`
	AvailKB    int64  `json:"avail_kb"`
	UsePercent int    `json:"use_percent"`
}

// HostDiskResult 主机磁盘水位。
type HostDiskResult struct {
	Filesystems []DiskFilesystem `json:"filesystems"`
}

// ── host.basic ──────────────────────────────────────────

type hostBasicSource struct{}

func (hostBasicSource) Kind() string { return KindHostBasic }

// basicCommand 一次性取 loadavg/uptime/meminfo 以及两次 /proc/stat（间隔 1s 算 CPU%）。
const basicCommand = "echo __LOADAVG__; cat /proc/loadavg; " +
	"echo __UPTIME__; cat /proc/uptime; " +
	"echo __MEMINFO__; cat /proc/meminfo; " +
	"echo __STAT1__; grep '^cpu ' /proc/stat; sleep 1; " +
	"echo __STAT2__; grep '^cpu ' /proc/stat"

func (hostBasicSource) Collect(ctx context.Context, cc CollectContext) (any, error) {
	out, err := runSSHCommand(ctx, cc, basicCommand)
	if err != nil {
		return nil, err
	}
	return parseHostBasic(out)
}

// parseHostBasic 解析 basicCommand 的分段输出。
func parseHostBasic(out string) (HostBasicResult, error) {
	sections := splitSections(out)
	res := HostBasicResult{}

	if l1, l5, l15, ok := parseLoadavg(sections["LOADAVG"]); ok {
		res.Load1, res.Load5, res.Load15 = l1, l5, l15
	}
	res.UptimeSeconds = parseUptime(sections["UPTIME"])

	total, avail := parseMeminfo(sections["MEMINFO"])
	res.MemTotalKB, res.MemAvailableKB = total, avail
	if total > 0 {
		res.MemUsagePercent = round2(float64(total-avail) / float64(total) * 100)
	}

	t1, i1, ok1 := parseCPULine(sections["STAT1"])
	t2, i2, ok2 := parseCPULine(sections["STAT2"])
	if ok1 && ok2 && t2 > t1 {
		dTotal := float64(t2 - t1)
		dIdle := float64(i2 - i1)
		res.CPUUsagePercent = round2((1 - dIdle/dTotal) * 100)
	}
	return res, nil
}

// ── host.disk ───────────────────────────────────────────

type hostDiskSource struct{}

func (hostDiskSource) Kind() string { return KindHostDisk }

func (hostDiskSource) Collect(ctx context.Context, cc CollectContext) (any, error) {
	out, err := runSSHCommand(ctx, cc, "df -kP")
	if err != nil {
		return nil, err
	}
	fs := parseDF(out)
	// mount 参数可选：只保留指定挂载点。
	if mount := paramString(cc.Task.Params, "mount"); mount != "" {
		filtered := fs[:0]
		for _, f := range fs {
			if f.Mount == mount {
				filtered = append(filtered, f)
			}
		}
		fs = filtered
	}
	return HostDiskResult{Filesystems: fs}, nil
}

// ── 解析函数（纯逻辑，可单测）──────────────────────────

// splitSections 按 __NAME__ 标记切分输出为各段。
func splitSections(out string) map[string]string {
	sections := map[string]string{}
	current := ""
	var buf []string
	flush := func() {
		if current != "" {
			sections[current] = strings.Join(buf, "\n")
		}
	}
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "__") && strings.HasSuffix(t, "__") && len(t) > 4 {
			flush()
			current = strings.Trim(t, "_")
			buf = nil
			continue
		}
		buf = append(buf, line)
	}
	flush()
	return sections
}

// parseLoadavg 解析 /proc/loadavg 前三个字段。
func parseLoadavg(s string) (l1, l5, l15 float64, ok bool) {
	fields := strings.Fields(s)
	if len(fields) < 3 {
		return 0, 0, 0, false
	}
	l1, e1 := strconv.ParseFloat(fields[0], 64)
	l5, e5 := strconv.ParseFloat(fields[1], 64)
	l15, e15 := strconv.ParseFloat(fields[2], 64)
	if e1 != nil || e5 != nil || e15 != nil {
		return 0, 0, 0, false
	}
	return l1, l5, l15, true
}

// parseUptime 解析 /proc/uptime 第一个字段（秒）。
func parseUptime(s string) float64 {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(fields[0], 64)
	return v
}

// parseMeminfo 解析 /proc/meminfo 的 MemTotal 与 MemAvailable（单位 KB）。
func parseMeminfo(s string) (total, available int64) {
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total, _ = strconv.ParseInt(fields[1], 10, 64)
		case "MemAvailable:":
			available, _ = strconv.ParseInt(fields[1], 10, 64)
		}
	}
	return total, available
}

// parseCPULine 解析 "cpu  ..." 行，返回 total 与 idle（含 iowait）累计 jiffies。
func parseCPULine(s string) (total, idle uint64, ok bool) {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, false
	}
	for i := 1; i < len(fields); i++ {
		v, err := strconv.ParseUint(fields[i], 10, 64)
		if err != nil {
			continue
		}
		total += v
		// 第 4 列 idle、第 5 列 iowait 计入空闲
		if i == 4 || i == 5 {
			idle += v
		}
	}
	return total, idle, total > 0
}

// parseDF 解析 df -kP 输出（跳过表头），mount 取末列、容量列去掉 %。
func parseDF(out string) []DiskFilesystem {
	var result []DiskFilesystem
	scanner := bufio.NewScanner(strings.NewReader(out))
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			first = false // 跳过表头行
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		size, _ := strconv.ParseInt(fields[1], 10, 64)
		used, _ := strconv.ParseInt(fields[2], 10, 64)
		avail, _ := strconv.ParseInt(fields[3], 10, 64)
		usePct, _ := strconv.Atoi(strings.TrimSuffix(fields[4], "%"))
		// 挂载点可能含空格，取第 6 列起的剩余部分
		mount := strings.Join(fields[5:], " ")
		result = append(result, DiskFilesystem{
			Filesystem: fields[0],
			Mount:      mount,
			SizeKB:     size,
			UsedKB:     used,
			AvailKB:    avail,
			UsePercent: usePct,
		})
	}
	return result
}

// round2 保留两位小数。
func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

// ── SSH 执行 ────────────────────────────────────────────

// runSSHCommand 在 targetID 对应的 SSH 连接上执行命令，受 ctx 与硬超时双重约束。
// cc.SSH 非空时复用本轮缓存的连接（不在此关闭）；为空时自行拨号并在返回前关闭。
func runSSHCommand(ctx context.Context, cc CollectContext, command string) (string, error) {
	var client *clients.LinuxSshClient
	if cc.SSH != nil {
		c, err := cc.SSH.Get(cc.Task.TargetID)
		if err != nil {
			return "", err
		}
		client = c
	} else {
		fields, err := sshFieldsByTarget(cc.Env, cc.Task.TargetID)
		if err != nil {
			return "", err
		}
		dial, err := clients.ParseLinuxSshDialConfig(fields)
		if err != nil {
			return "", err
		}
		client, err = dial.Dial()
		if err != nil {
			return "", err
		}
		defer client.Close()
	}

	session, err := client.Client().NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	type result struct {
		out []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := session.CombinedOutput(command)
		done <- result{out, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			return "", fmt.Errorf("命令执行失败: %w", r.err)
		}
		return string(r.out), nil
	case <-ctx.Done():
		_ = session.Close()
		return "", ctx.Err()
	case <-time.After(sshCmdTimeout):
		_ = session.Close()
		return "", fmt.Errorf("SSH 命令超时（%s）", sshCmdTimeout)
	}
}

// sshFieldsByTarget 在环境中按 targetID 定位 SSH 配置的 fields。
func sshFieldsByTarget(env core.EnvironmentDef, targetID string) (map[string]any, error) {
	for i := range env.Configs {
		c := &env.Configs[i]
		if c.ID == targetID {
			if c.Kind != core.EnvConfigKindSSH {
				return nil, fmt.Errorf("目标 %s 类型 %s 不是 ssh", targetID, c.Kind)
			}
			return c.Fields, nil
		}
	}
	return nil, fmt.Errorf("目标配置未找到: %s", targetID)
}
