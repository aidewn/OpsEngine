// docker_pull 节点：从远端镜像仓库拉取镜像
// 流式响应必须读到 EOF 才算完成 pull，否则 daemon 会判作客户端中断

package docker_pull

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"

	"github.com/docker/docker/api/types/image"
)

func init() { engine.Register(&Node{}) }

// Node docker_pull 节点实现
type Node struct{}

func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "docker_pull",
		DisplayName: "Docker 拉取镜像",
		Category:    "docker",
		NodeKind:    core.NodeKindAction,
		Icon:        "⬇",
		Description: "从镜像仓库拉取镜像到远端 Docker daemon",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "client", Label: "Docker", PortType: core.PortTypeDockerContext, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "image", Label: "镜像名", Placeholder: "nginx:1.27", Required: true},
			{Type: "text", ID: "platform", Label: "平台（可选）", Placeholder: "linux/amd64"},
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
		return nil, fmt.Errorf("docker_pull: image 未配置")
	}
	platform := strings.TrimSpace(ctx.ConfigString("platform"))

	ctx.Info("拉取镜像 %s ...", ref)
	rc, err := client.API().ImagePull(ctx.Context(), ref, image.PullOptions{Platform: platform})
	if err != nil {
		return nil, fmt.Errorf("docker_pull: ImagePull %q 失败: %w", ref, err)
	}
	defer rc.Close()

	// 流式响应逐行打印关键阶段，丢弃细节，必须读到 EOF
	scanner := bufio.NewScanner(rc)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, `"status":"Pull complete"`) ||
			strings.Contains(line, `"status":"Already exists"`) ||
			strings.Contains(line, `"status":"Status: Downloaded`) ||
			strings.Contains(line, `"status":"Status: Image is up to date`) {
			ctx.Info("%s", line)
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return nil, fmt.Errorf("docker_pull: 读取拉取响应失败: %w", err)
	}

	ctx.Info("镜像 %s 拉取完成", ref)
	return engine.Outputs{}, nil
}

// dockerClientFromInput 从 client 输入端口取出 DockerContext 句柄
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
