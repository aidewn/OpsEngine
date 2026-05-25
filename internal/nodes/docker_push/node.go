// docker_push 节点：把本地镜像推送到远端 registry
// 凭证以独立字段填写（username/password/server），节点内部包装成 Docker SDK 要求的 base64 JSON 串
// 公共镜像无需鉴权时三项留空，节点会传一个最小占位 auth（daemon 仍需要这个字段非空）

package docker_push

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"

	"github.com/docker/docker/api/types/image"
)

func init() { engine.Register(&Node{}) }

type Node struct{}

func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "docker_push",
		DisplayName: "Docker 推送镜像",
		Category:    "docker",
		NodeKind:    core.NodeKindAction,
		Icon:        "⬆",
		Description: "把本地镜像推送到远端 registry",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "client", Label: "Docker", PortType: core.PortTypeDockerContext, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "image", Label: "镜像引用",
				Placeholder: "registry.example.com/myapp:1.0", Required: true},
			{Type: "text", ID: "username", Label: "用户名（可选）"},
			{Type: "password", ID: "password", Label: "密码 / Token（可选）"},
			{Type: "text", ID: "server_address", Label: "Registry 地址（可选）",
				Placeholder: "默认从镜像引用推断"},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	client, err := dockerClientFromInput(ctx)
	if err != nil {
		return nil, err
	}
	ref := strings.TrimSpace(ctx.ConfigString("image"))
	if ref == "" {
		return nil, fmt.Errorf("docker_push: image 未配置")
	}
	user := strings.TrimSpace(ctx.ConfigString("username"))
	password := ctx.ConfigString("password")
	server := strings.TrimSpace(ctx.ConfigString("server_address"))
	if server == "" {
		server = registryFromRef(ref)
	}

	authStr, err := encodeRegistryAuth(user, password, server)
	if err != nil {
		return nil, fmt.Errorf("docker_push: 编码 registry 凭证失败: %w", err)
	}

	ctx.Info("推送镜像 %s -> %s ...", ref, server)
	rc, err := client.API().ImagePush(ctx.Context(), ref, image.PushOptions{RegistryAuth: authStr})
	if err != nil {
		return nil, fmt.Errorf("docker_push: ImagePush %q 失败: %w", ref, err)
	}
	defer rc.Close()

	if err := drainPushStream(ctx, rc); err != nil {
		return nil, fmt.Errorf("docker_push: %w", err)
	}

	ctx.Info("镜像推送完成: %s", ref)
	return engine.Outputs{}, nil
}

// encodeRegistryAuth 把用户名/密码/server 拼成 Docker SDK 要求的 base64(JSON) 鉴权串
// 任何一个字段为空时仍会编码（daemon 要求 PushOptions.RegistryAuth 非空），daemon 自行降级到匿名访问
func encodeRegistryAuth(user, password, server string) (string, error) {
	cfg := struct {
		Username      string `json:"username,omitempty"`
		Password      string `json:"password,omitempty"`
		ServerAddress string `json:"serveraddress,omitempty"`
	}{
		Username:      user,
		Password:      password,
		ServerAddress: server,
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(data), nil
}

// registryFromRef 从 image 引用里推断 registry 地址
// docker.io/library/nginx:1.27 -> docker.io
// registry.example.com/myapp:1.0 -> registry.example.com
// nginx:1.27 -> docker.io（隐式 default registry）
func registryFromRef(ref string) string {
	if idx := strings.Index(ref, "/"); idx > 0 {
		first := ref[:idx]
		// 带端口（registry:5000/foo）或带 . 都判定为 registry 域名
		if strings.ContainsAny(first, ".:") {
			return first
		}
	}
	return "docker.io"
}

// drainPushStream 读完 push 的 JSON 流并打印关键阶段；错误行直接 return
func drainPushStream(ctx engine.ExecContext, r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		var msg struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.Error != "" {
			return fmt.Errorf("push 错误: %s", msg.Error)
		}
		// 只打印里程碑：layer pushed / digest
		if strings.HasPrefix(msg.Status, "Pushed") ||
			strings.HasPrefix(msg.Status, "Layer already exists") ||
			strings.HasPrefix(msg.Status, "Mounted") ||
			strings.Contains(msg.Status, "digest:") {
			ctx.Info("%s", msg.Status)
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return fmt.Errorf("读取 push 响应失败: %w", err)
	}
	return nil
}

func dockerClientFromInput(ctx engine.ExecContext) (*clients.DockerClient, error) {
	v, ok := ctx.Input("client")
	if !ok || v == nil {
		return nil, fmt.Errorf("client 输入端口未连接 DockerContext")
	}
	c, ok := v.(*clients.DockerClient)
	if !ok {
		return nil, fmt.Errorf("client 输入端口类型不是 *DockerClient，得到 %T", v)
	}
	return c, nil
}
