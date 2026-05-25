// linux_file_write 节点：以覆盖语义把文本写入远程 Linux 文件
// 文件不存在时按 0644 权限创建

package linux_file_write

import (
	"fmt"
	"os"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node linux_file_write 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "linux_file_write",
		DisplayName: "Linux 覆盖写入",
		Category:    "remote",
		NodeKind:    core.NodeKindAction,
		Icon:        "💾",
		Description: "用给定文本覆盖远程 Linux 文件全部内容",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "handle", Label: "file", PortType: core.PortTypeLinuxFileHandle, Required: true},
			{ID: "content", Label: "content", PortType: core.PortTypeString},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "bytes_written", Label: "bytes", PortType: core.PortTypeInt},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "textarea", ID: "content", Label: "文本内容",
				Placeholder: "content 未连接时使用此内容"},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

// Execute 以截断方式覆盖远程文件
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	handle, err := inputHandle(ctx)
	if err != nil {
		return nil, err
	}

	content := stringInput(ctx, "content")
	if content == "" {
		content = ctx.ConfigString("content")
	}

	sc, err := handle.Sftp()
	if err != nil {
		return nil, err
	}

	f, err := sc.OpenFile(handle.Path, os.O_TRUNC|os.O_WRONLY|os.O_CREATE)
	if err != nil {
		return nil, fmt.Errorf("打开远程文件失败: %w", err)
	}
	defer f.Close()

	n, err := f.Write([]byte(content))
	if err != nil {
		return nil, fmt.Errorf("写入失败: %w", err)
	}
	ctx.Info("覆盖写入 %d 字节到 %s", n, handle.Path)
	return engine.Outputs{"bytes_written": int64(n)}, nil
}

// inputHandle 读取必填的 LinuxFileHandle 输入
func inputHandle(ctx engine.ExecContext) (*clients.LinuxFileHandle, error) {
	value, ok := ctx.Input("handle")
	if !ok || value == nil {
		return nil, fmt.Errorf("linux_file_write 节点缺少 handle 输入")
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
