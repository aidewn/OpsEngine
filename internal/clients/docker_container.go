// Docker 容器运行时句柄
// 由 docker_filter / docker_run 节点输出，被 docker_exec / docker_logs 等节点消费
// 不持有容器生命周期；停容器请走 docker_stop（未实现）

package clients

import (
	"encoding/json"
	"fmt"
	"strings"

	dockerclient "github.com/docker/docker/client"
)

// DockerContainerHandle 容器引用句柄：DockerContext + container ID
// 序列化时只暴露元数据，不展开底层 client
type DockerContainerHandle struct {
	Client      *DockerClient
	ContainerID string
	Name        string
	Image       string
}

// NewDockerContainerHandle 构造容器句柄，做基础参数校验
func NewDockerContainerHandle(client *DockerClient, id, name, image string) (*DockerContainerHandle, error) {
	if client == nil {
		return nil, fmt.Errorf("DockerContext 未提供")
	}
	if id == "" {
		return nil, fmt.Errorf("container_id 为空")
	}
	// docker inspect 返回的 name 形如 "/nginx-demo"，去掉前导斜杠让显示更友好
	name = strings.TrimPrefix(name, "/")
	return &DockerContainerHandle{
		Client:      client,
		ContainerID: id,
		Name:        name,
		Image:       image,
	}, nil
}

// API 直通到关联 DockerContext 的 Docker SDK 客户端
func (h *DockerContainerHandle) API() *dockerclient.Client {
	if h == nil || h.Client == nil {
		return nil
	}
	return h.Client.API()
}

// MarshalJSON 输出可读元信息，不暴露底层 Docker client / SSH 隧道
func (h *DockerContainerHandle) MarshalJSON() ([]byte, error) {
	if h == nil {
		return []byte("null"), nil
	}
	type safeView struct {
		Type        string `json:"type"`
		ContainerID string `json:"container_id"`
		Name        string `json:"name,omitempty"`
		Image       string `json:"image,omitempty"`
		Host        string `json:"host,omitempty"`
	}
	view := safeView{
		Type:        "DockerContainerHandle",
		ContainerID: h.ContainerID,
		Name:        h.Name,
		Image:       h.Image,
	}
	if h.Client != nil {
		view.Host = h.Client.Host
	}
	return json.Marshal(view)
}
