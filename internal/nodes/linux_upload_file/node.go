// linux_upload_file 节点：通过 SFTP 把 OpsEngine 服务器本地文件上传到远端主机指定目录
// 返回上传后的文件名、远端目录和完整路径

package linux_upload_file

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

// Node linux_upload_file 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "linux_upload_file",
		DisplayName: "Linux 上传文件",
		Category:    "remote",
		NodeKind:    core.NodeKindAction,
		Icon:        "⬆",
		Description: "通过 SFTP 把本机磁盘文件上传到远端 Linux 主机的指定目录",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "client", Label: "SSH", PortType: core.PortTypeLinuxSsh, Required: true},
			{ID: "local_path", Label: "本地路径", PortType: core.PortTypeString},
			{ID: "remote_dir", Label: "远端目录", PortType: core.PortTypeString},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "filename", Label: "文件名", PortType: core.PortTypeString},
			{ID: "remote_dir", Label: "远端目录", PortType: core.PortTypeString},
			{ID: "remote_path", Label: "远端路径", PortType: core.PortTypeString},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "local_path", Label: "本地文件路径", Required: true,
				Placeholder: "/tmp/myapp.jar"},
			{Type: "text", ID: "remote_dir", Label: "远端目录", Required: true,
				Placeholder: "/opt/deploy"},
			{Type: "toggle", ID: "ensure_parent_dir", Label: "自动创建远端父目录", Default: true},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

// Execute 通过 SFTP 上传本地文件
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	sshClient, err := inputClient(ctx)
	if err != nil {
		return nil, err
	}

	localPath := strings.TrimSpace(stringInput(ctx, "local_path"))
	if localPath == "" {
		localPath = strings.TrimSpace(ctx.ConfigString("local_path"))
	}
	if localPath == "" {
		return nil, fmt.Errorf("linux_upload_file 节点的 local_path 未配置")
	}

	remoteDir := strings.TrimSpace(stringInput(ctx, "remote_dir"))
	if remoteDir == "" {
		remoteDir = strings.TrimSpace(ctx.ConfigString("remote_dir"))
	}
	if remoteDir == "" {
		return nil, fmt.Errorf("linux_upload_file 节点的 remote_dir 未配置")
	}

	// 打开本地文件，filepath.Base 在 Windows 主机上也能正确取文件名
	f, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer f.Close()

	filename := filepath.Base(localPath)
	// 远端路径强制用 Linux path（正斜杠），避免 Windows 宿主机 filepath 混入反斜杠
	remotePath := path.Join(remoteDir, filename)

	sc, err := sshClient.Sftp()
	if err != nil {
		return nil, err
	}

	if ctx.ConfigBool("ensure_parent_dir") {
		if err := sc.MkdirAll(remoteDir); err != nil {
			return nil, fmt.Errorf("创建远端目录 %s 失败: %w", remoteDir, err)
		}
	}

	dst, err := sc.Create(remotePath)
	if err != nil {
		return nil, fmt.Errorf("创建远端文件 %s 失败: %w", remotePath, err)
	}
	defer dst.Close()

	n, err := io.Copy(dst, f)
	if err != nil {
		return nil, fmt.Errorf("上传失败: %w", err)
	}

	ctx.Info("上传完成: %s -> %s (%d 字节)", localPath, remotePath, n)
	return engine.Outputs{
		"filename":    filename,
		"remote_dir":  remoteDir,
		"remote_path": remotePath,
	}, nil
}

// inputClient 读取必填的 SSH 连接句柄
func inputClient(ctx engine.ExecContext) (*clients.LinuxSshClient, error) {
	value, ok := ctx.Input("client")
	if !ok || value == nil {
		return nil, fmt.Errorf("linux_upload_file 节点缺少 client 输入")
	}
	c, ok := value.(*clients.LinuxSshClient)
	if !ok {
		return nil, fmt.Errorf("client 输入类型不是 LinuxSshConnection")
	}
	return c, nil
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
