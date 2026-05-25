// linux_find_file 节点：在远程 Linux 文件系统中按正则递归搜索文件
// 仅匹配 basename；通过 SFTP Walk 实现，遵守最大深度防止扫整盘

package linux_find_file

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

const (
	defaultStartDir = "/"
	defaultDepth    = 5
)

func init() { engine.Register(&Node{}) }

// Node linux_find_file 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	minDepth, maxDepth := int64(1), int64(32)
	return core.NodeTypeDef{
		TypeID:      "linux_find_file",
		DisplayName: "Linux 搜索文件",
		Category:    "remote",
		NodeKind:    core.NodeKindAction,
		Icon:        "🔎",
		Description: "通过 SFTP 在远程 Linux 文件系统按正则递归搜索文件",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "client", Label: "SSH", PortType: core.PortTypeLinuxSsh, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "paths", Label: "paths", PortType: core.PortTypeString},
			{ID: "count", Label: "count", PortType: core.PortTypeInt},
			{ID: "first_path", Label: "first", PortType: core.PortTypeString},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "pattern", Label: "文件名正则", Required: true,
				Placeholder: `^.*\.log$`},
			{Type: "text", ID: "start_dir", Label: "起始目录",
				Placeholder: "/", Default: defaultStartDir},
			{Type: "number", ID: "max_depth", Label: "最大深度",
				Min: &minDepth, Max: &maxDepth, Default: int64(defaultDepth)},
			{Type: "toggle", ID: "case_sensitive", Label: "区分大小写", Default: true},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

// Execute 通过 SFTP Walk 搜索匹配的文件路径
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	client, err := inputClient(ctx)
	if err != nil {
		return nil, err
	}

	pattern := strings.TrimSpace(ctx.ConfigString("pattern"))
	if pattern == "" {
		return nil, fmt.Errorf("linux_find_file 节点的 pattern 未配置")
	}
	if !ctx.ConfigBool("case_sensitive") {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("正则编译失败: %w", err)
	}

	startDir := strings.TrimSpace(ctx.ConfigString("start_dir"))
	if startDir == "" {
		startDir = defaultStartDir
	}
	maxDepth := int(ctx.ConfigInt("max_depth"))
	if maxDepth <= 0 {
		maxDepth = defaultDepth
	}

	sc, err := client.Sftp()
	if err != nil {
		return nil, err
	}

	ctx.Info("搜索 %s 下匹配 %s 的文件（最大深度 %d）", startDir, pattern, maxDepth)

	cleanedStart := path.Clean(startDir)
	startDepth := strings.Count(cleanedStart, "/")
	var matches []string
	walker := sc.Walk(cleanedStart)
	for walker.Step() {
		if walker.Err() != nil {
			ctx.Warn("遍历跳过: %v", walker.Err())
			continue
		}
		if err := ctx.Context().Err(); err != nil {
			return nil, err
		}
		current := walker.Path()
		depth := strings.Count(path.Clean(current), "/") - startDepth
		if depth > maxDepth {
			walker.SkipDir()
			continue
		}
		info := walker.Stat()
		if info == nil || info.IsDir() {
			continue
		}
		if re.MatchString(info.Name()) {
			matches = append(matches, current)
		}
	}

	first := ""
	if len(matches) > 0 {
		first = matches[0]
	}
	ctx.Info("命中 %d 个文件", len(matches))
	return engine.Outputs{
		"paths":      strings.Join(matches, "\n"),
		"count":      int64(len(matches)),
		"first_path": first,
	}, nil
}

// inputClient 读取必填的 SSH 连接句柄
func inputClient(ctx engine.ExecContext) (*clients.LinuxSshClient, error) {
	value, ok := ctx.Input("client")
	if !ok || value == nil {
		return nil, fmt.Errorf("linux_find_file 节点缺少 client 输入")
	}
	c, ok := value.(*clients.LinuxSshClient)
	if !ok {
		return nil, fmt.Errorf("client 输入类型不是 LinuxSshConnection")
	}
	return c, nil
}
