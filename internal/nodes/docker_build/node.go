// docker_build 节点：用 Dockerfile 文本在远端构建镜像
// 为简化实现，build context 只包含一个 Dockerfile（不支持 COPY/ADD 本地文件）
// 复杂的多文件构建可以走 docker_exec 直接 `docker build -f` 兜底

package docker_build

import (
	"archive/tar"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"

	"github.com/docker/docker/api/types/build"
)

func init() { engine.Register(&Node{}) }

type Node struct{}

func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "docker_build",
		DisplayName: "Docker 构建镜像",
		Category:    "docker",
		NodeKind:    core.NodeKindAction,
		Icon:        "🏗",
		Description: "用 Dockerfile 文本在远端构建镜像（不支持 COPY 本地文件，仅适合 FROM/RUN/ENV/CMD 类构建）",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "client", Label: "Docker", PortType: core.PortTypeDockerContext, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "image_id", Label: "ID", PortType: core.PortTypeString},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "text", ID: "tag", Label: "镜像标签", Placeholder: "myapp:latest", Required: true},
			{Type: "textarea", ID: "dockerfile", Label: "Dockerfile 内容", Required: true,
				Placeholder: "FROM nginx:1.27\nRUN apt-get update && apt-get install -y curl\n"},
			{Type: "textarea", ID: "build_args", Label: "构建参数（每行一个 KEY=VALUE）"},
			{Type: "text", ID: "platform", Label: "平台（可选）", Placeholder: "linux/amd64"},
			{Type: "toggle", ID: "no_cache", Label: "禁用缓存", Default: false},
			{Type: "toggle", ID: "pull", Label: "强制拉取基础镜像", Default: false},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	client, err := dockerClientFromInput(ctx)
	if err != nil {
		return nil, err
	}
	tag := strings.TrimSpace(ctx.ConfigString("tag"))
	if tag == "" {
		return nil, fmt.Errorf("docker_build: tag 未配置")
	}
	dockerfile := ctx.ConfigString("dockerfile")
	if strings.TrimSpace(dockerfile) == "" {
		return nil, fmt.Errorf("docker_build: dockerfile 内容为空")
	}

	buildArgs, err := parseBuildArgs(ctx.ConfigString("build_args"))
	if err != nil {
		return nil, fmt.Errorf("docker_build: %w", err)
	}

	// 把 Dockerfile 打成 tar，作为 build context 喂给 ImageBuild
	tarBuf, err := tarSingleFile("Dockerfile", []byte(dockerfile))
	if err != nil {
		return nil, fmt.Errorf("docker_build: 打包 build context 失败: %w", err)
	}

	opts := build.ImageBuildOptions{
		Tags:        []string{tag},
		Dockerfile:  "Dockerfile",
		Remove:      true,
		ForceRemove: true,
		NoCache:     ctx.ConfigBool("no_cache"),
		PullParent:  ctx.ConfigBool("pull"),
		BuildArgs:   buildArgs,
		Platform:    strings.TrimSpace(ctx.ConfigString("platform")),
	}

	ctx.Info("构建镜像 %s ...", tag)
	resp, err := client.API().ImageBuild(ctx.Context(), tarBuf, opts)
	if err != nil {
		return nil, fmt.Errorf("docker_build: ImageBuild 失败: %w", err)
	}
	defer resp.Body.Close()

	imageID, err := drainBuildStream(ctx, resp.Body)
	if err != nil {
		return nil, fmt.Errorf("docker_build: %w", err)
	}

	ctx.Info("镜像构建完成: %s (id=%s)", tag, imageID)
	return engine.Outputs{
		"image_id": imageID,
	}, nil
}

// parseBuildArgs 解析多行 KEY=VALUE，转成 SDK 需要的 map[string]*string
// 用指针是因为 daemon 区分 "" 和 nil（未设置 vs 设为空）
func parseBuildArgs(text string) (map[string]*string, error) {
	out := make(map[string]*string)
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			return nil, fmt.Errorf("build_args 第 %d 行缺少 '='：%q", i+1, line)
		}
		k := strings.TrimSpace(line[:eq])
		v := strings.TrimSpace(line[eq+1:])
		out[k] = &v
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// tarSingleFile 把单个文件内容打成 tar，返回可重读的 Reader
func tarSingleFile(name string, content []byte) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{
		Name:    name,
		Mode:    0644,
		Size:    int64(len(content)),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, err
	}
	if _, err := tw.Write(content); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return &buf, nil
}

// drainBuildStream 读完 ImageBuild 的 JSON 流；逐行打印关键阶段；末尾解析 aux 拿 image ID
// 必须读到 EOF 才算 build 真正完成
func drainBuildStream(ctx engine.ExecContext, r io.Reader) (string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	var imageID string
	for scanner.Scan() {
		line := scanner.Text()
		var msg struct {
			Stream      string          `json:"stream"`
			ErrorDetail json.RawMessage `json:"errorDetail,omitempty"`
			Error       string          `json:"error,omitempty"`
			Aux         json.RawMessage `json:"aux,omitempty"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.Error != "" {
			return "", fmt.Errorf("build 错误: %s", msg.Error)
		}
		if s := strings.TrimSpace(msg.Stream); s != "" {
			ctx.Info("%s", s)
		}
		if len(msg.Aux) > 0 {
			var aux struct {
				ID string `json:"ID"`
			}
			if err := json.Unmarshal(msg.Aux, &aux); err == nil && aux.ID != "" {
				imageID = aux.ID
			}
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return imageID, fmt.Errorf("读取 build 响应失败: %w", err)
	}
	return imageID, nil
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
