// Linux 文件运行时句柄
// 文件节点之间传递的轻量引用：仅包含 SSH 连接 + 远程路径
// 真实读写动作由各节点自行通过 SFTP 发起

package clients

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/sftp"
)

// LinuxFileHandle 远程 Linux 文件的引用句柄
// Client 在序列化时不会被展开，仅暴露其连接元信息
type LinuxFileHandle struct {
	Client *LinuxSshClient
	Path   string
}

// NewLinuxFileHandle 构造文件句柄，并做基础参数校验
func NewLinuxFileHandle(client *LinuxSshClient, path string) (*LinuxFileHandle, error) {
	if client == nil {
		return nil, fmt.Errorf("LinuxSshConnection 未提供")
	}
	if path == "" {
		return nil, fmt.Errorf("文件路径为空")
	}
	if strings.ContainsAny(path, "\r\n") {
		return nil, fmt.Errorf("文件路径不能包含换行")
	}
	return &LinuxFileHandle{Client: client, Path: path}, nil
}

// Sftp 直通到关联连接的 SFTP 子系统
func (h *LinuxFileHandle) Sftp() (*sftp.Client, error) {
	if h == nil || h.Client == nil {
		return nil, fmt.Errorf("LinuxFileHandle 无关联连接")
	}
	return h.Client.Sftp()
}

// MarshalJSON 输出可读元信息，不暴露底层 SSH 连接对象
func (h *LinuxFileHandle) MarshalJSON() ([]byte, error) {
	if h == nil {
		return []byte("null"), nil
	}
	type safeView struct {
		Type string `json:"type"`
		Host string `json:"host"`
		User string `json:"user"`
		Path string `json:"path"`
	}
	view := safeView{Type: "LinuxFileHandle", Path: h.Path}
	if h.Client != nil {
		view.Host = h.Client.Host
		view.User = h.Client.User
	}
	return json.Marshal(view)
}
