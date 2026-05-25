// linux_open_file 节点：返回远程 Linux 文件的轻量引用句柄
// 不真正持有远程 fd；可选 create_if_missing 时通过 SFTP 创建空文件

package linux_open_file

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node linux_open_file 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "linux_open_file",
		DisplayName: "Linux 打开文件",
		Category:    "remote",
		NodeKind:    core.NodeKindAction,
		Icon:        "📂",
		Description: "返回远程文件的轻量引用句柄，供后续追加 / 覆盖 / 替换节点使用",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "client", Label: "SSH", PortType: core.PortTypeLinuxSsh, Required: true},
			{ID: "path", Label: "路径", PortType: core.PortTypeString},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "handle", Label: "file", PortType: core.PortTypeLinuxFileHandle},
			{ID: "exists", Label: "exists", PortType: core.PortTypeBool},
			{ID: "size_bytes", Label: "size", PortType: core.PortTypeInt},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "path", Label: "文件路径", Required: true,
				Placeholder: "/etc/hosts"},
			{Type: "toggle", ID: "create_if_missing", Label: "不存在时创建", Default: false},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

// Execute 检查文件是否存在并构造 LinuxFileHandle
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	client, err := inputClient(ctx)
	if err != nil {
		return nil, err
	}
	filePath := strings.TrimSpace(stringInput(ctx, "path"))
	if filePath == "" {
		filePath = strings.TrimSpace(ctx.ConfigString("path"))
	}
	if filePath == "" {
		return nil, fmt.Errorf("linux_open_file 节点的 path 未配置")
	}

	sc, err := client.Sftp()
	if err != nil {
		return nil, err
	}

	exists := true
	var size int64
	info, statErr := sc.Stat(filePath)
	if statErr != nil {
		if !errors.Is(statErr, os.ErrNotExist) {
			return nil, fmt.Errorf("Stat 远程文件失败: %w", statErr)
		}
		exists = false
	} else if info.IsDir() {
		return nil, fmt.Errorf("路径 %s 指向目录，不是文件", filePath)
	} else {
		size = info.Size()
	}

	if !exists && ctx.ConfigBool("create_if_missing") {
		f, createErr := sc.Create(filePath)
		if createErr != nil {
			return nil, fmt.Errorf("创建远程文件失败: %w", createErr)
		}
		_ = f.Close()
		exists = true
		size = 0
		ctx.Info("已创建空文件 %s", filePath)
	}

	handle, err := clients.NewLinuxFileHandle(client, filePath)
	if err != nil {
		return nil, err
	}

	ctx.Info("已打开文件引用 %s（存在=%t, size=%d）", filePath, exists, size)
	return engine.Outputs{
		"handle":     handle,
		"exists":     exists,
		"size_bytes": size,
	}, nil
}

// inputClient 读取必填的 SSH 连接句柄
func inputClient(ctx engine.ExecContext) (*clients.LinuxSshClient, error) {
	value, ok := ctx.Input("client")
	if !ok || value == nil {
		return nil, fmt.Errorf("linux_open_file 节点缺少 client 输入")
	}
	c, ok := value.(*clients.LinuxSshClient)
	if !ok {
		return nil, fmt.Errorf("client 输入类型不是 LinuxSshConnection")
	}
	return c, nil
}

// stringInput 从指定输入端口读字符串值，缺失或类型不匹配返回空串
func stringInput(ctx engine.ExecContext, portID string) string {
	value, ok := ctx.Input(portID)
	if !ok || value == nil {
		return ""
	}
	s, _ := value.(string)
	return s
}
