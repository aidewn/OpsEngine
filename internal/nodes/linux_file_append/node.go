// linux_file_append 节点：把文本追加写入远程 Linux 文件末尾
// 文件不存在时按 0644 权限创建

package linux_file_append

import (
	"fmt"
	"os"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node linux_file_append 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "linux_file_append",
		DisplayName: "Linux 追加文本",
		Category:    "remote",
		NodeKind:    core.NodeKindAction,
		Icon:        "➕",
		Description: "把文本追加写入远程 Linux 文件末尾",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "handle", Label: "file", PortType: core.PortTypeLinuxFileHandle, Required: true},
			{ID: "text", Label: "text", PortType: core.PortTypeString},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "bytes_written", Label: "bytes", PortType: core.PortTypeInt},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "textarea", ID: "text", Label: "文本内容",
				Placeholder: "text 未连接时使用此内容"},
			{Type: "toggle", ID: "add_newline", Label: "自动追加换行", Default: false},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

// Execute 打开远程文件追加写入并返回写入字节数
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	handle, err := inputHandle(ctx)
	if err != nil {
		return nil, err
	}

	text := stringInput(ctx, "text")
	if text == "" {
		text = ctx.ConfigString("text")
	}
	if ctx.ConfigBool("add_newline") {
		text += "\n"
	}

	sc, err := handle.Sftp()
	if err != nil {
		return nil, err
	}

	f, err := sc.OpenFile(handle.Path, os.O_APPEND|os.O_WRONLY|os.O_CREATE)
	if err != nil {
		return nil, fmt.Errorf("打开远程文件失败: %w", err)
	}
	defer f.Close()

	n, err := f.Write([]byte(text))
	if err != nil {
		return nil, fmt.Errorf("追加写入失败: %w", err)
	}
	ctx.Info("追加 %d 字节到 %s", n, handle.Path)
	return engine.Outputs{"bytes_written": int64(n)}, nil
}

// inputHandle 读取必填的 LinuxFileHandle 输入
func inputHandle(ctx engine.ExecContext) (*clients.LinuxFileHandle, error) {
	value, ok := ctx.Input("handle")
	if !ok || value == nil {
		return nil, fmt.Errorf("linux_file_append 节点缺少 handle 输入")
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
