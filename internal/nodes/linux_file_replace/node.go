// linux_file_replace 节点：替换远程 Linux 文件中的指定文本
// 纯文本匹配（不走正则），默认保留 .bak 备份

package linux_file_replace

import (
	"fmt"
	"io"
	"os"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node linux_file_replace 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "linux_file_replace",
		DisplayName: "Linux 替换文本",
		Category:    "remote",
		NodeKind:    core.NodeKindAction,
		Icon:        "🔁",
		Description: "替换远程 Linux 文件中的指定文本（纯文本匹配）",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "handle", Label: "file", PortType: core.PortTypeLinuxFileHandle, Required: true},
			{ID: "search", Label: "search", PortType: core.PortTypeString},
			{ID: "replacement", Label: "replace", PortType: core.PortTypeString},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "occurrences", Label: "count", PortType: core.PortTypeInt},
			{ID: "bytes_written", Label: "bytes", PortType: core.PortTypeInt},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "search", Label: "搜索文本", Required: true},
			{Type: "text", ID: "replacement", Label: "替换为"},
			{Type: "toggle", ID: "replace_all", Label: "替换全部", Default: true},
			{Type: "toggle", ID: "backup", Label: "保留 .bak 备份", Default: true},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

// Execute 读取文件全文 → 内存中替换 → 写回；可选写 .bak 备份
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	handle, err := inputHandle(ctx)
	if err != nil {
		return nil, err
	}

	search := stringInput(ctx, "search")
	if search == "" {
		search = ctx.ConfigString("search")
	}
	if search == "" {
		return nil, fmt.Errorf("linux_file_replace 节点的 search 未配置")
	}
	replacement := stringInput(ctx, "replacement")
	if replacement == "" {
		replacement = ctx.ConfigString("replacement")
	}

	sc, err := handle.Sftp()
	if err != nil {
		return nil, err
	}

	src, err := sc.Open(handle.Path)
	if err != nil {
		return nil, fmt.Errorf("打开远程文件失败: %w", err)
	}
	original, readErr := io.ReadAll(src)
	_ = src.Close()
	if readErr != nil {
		return nil, fmt.Errorf("读取远程文件失败: %w", readErr)
	}

	content := string(original)
	count := strings.Count(content, search)
	n := count
	if !ctx.ConfigBool("replace_all") {
		n = 1
	}
	if count == 0 || n == 0 {
		ctx.Info("未命中替换文本，文件保持不变")
		return engine.Outputs{
			"occurrences":   int64(0),
			"bytes_written": int64(0),
		}, nil
	}
	updated := strings.Replace(content, search, replacement, n)

	if ctx.ConfigBool("backup") {
		backupPath := handle.Path + ".bak"
		bf, bErr := sc.OpenFile(backupPath, os.O_TRUNC|os.O_WRONLY|os.O_CREATE)
		if bErr != nil {
			return nil, fmt.Errorf("写入备份文件失败: %w", bErr)
		}
		if _, wErr := bf.Write(original); wErr != nil {
			_ = bf.Close()
			return nil, fmt.Errorf("写入备份文件失败: %w", wErr)
		}
		_ = bf.Close()
		ctx.Info("已备份原文件到 %s", backupPath)
	}

	dst, err := sc.OpenFile(handle.Path, os.O_TRUNC|os.O_WRONLY|os.O_CREATE)
	if err != nil {
		return nil, fmt.Errorf("打开远程文件失败: %w", err)
	}
	defer dst.Close()
	written, err := dst.Write([]byte(updated))
	if err != nil {
		return nil, fmt.Errorf("写回替换结果失败: %w", err)
	}

	ctx.Info("替换 %d 处，写回 %d 字节到 %s", n, written, handle.Path)
	return engine.Outputs{
		"occurrences":   int64(n),
		"bytes_written": int64(written),
	}, nil
}

// inputHandle 读取必填的 LinuxFileHandle 输入
func inputHandle(ctx engine.ExecContext) (*clients.LinuxFileHandle, error) {
	value, ok := ctx.Input("handle")
	if !ok || value == nil {
		return nil, fmt.Errorf("linux_file_replace 节点缺少 handle 输入")
	}
	h, ok := value.(*clients.LinuxFileHandle)
	if !ok {
		return nil, fmt.Errorf("handle 输入类型不是 LinuxFileHandle")
	}
	return h, nil
}

// stringInput 读字符串输入端口，缺失或类型不匹配返回空串
func stringInput(ctx engine.ExecContext, portID string) string {
	value, ok := ctx.Input(portID)
	if !ok || value == nil {
		return ""
	}
	s, _ := value.(string)
	return s
}
