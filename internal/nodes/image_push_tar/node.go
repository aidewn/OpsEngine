// image_push_tar 节点
// 流程：本机 tar → docker.ImageLoad → 解析 loaded 引用 → docker.ImageTag(target) → docker.ImagePush
//
// 取舍：
//   - 复用上游 docker_connect / env_connect_docker 输出的 DockerContext（over_ssh 隧道里 ImageLoad 也工作）
//   - tar 文件路径通过前端 file_path 字段拿到本机绝对路径
//   - registry 凭据走环境配置（kind=registry）；ImagePush 用 SDK 的 X-Registry-Auth header
//   - 失败不回滚（已 load 的镜像可由下游手动清理），保持节点职责单一

package image_push_tar

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"

	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"
)

func init() { engine.Register(&Node{}) }

// Node image_push_tar 节点实现
type Node struct{}

// TypeDef 节点元信息
func (Node) TypeDef() core.NodeTypeDef {
	return core.NodeTypeDef{
		TypeID:      "image_push_tar",
		DisplayName: "镜像 · 上传 tar 到仓库",
		Category:    "docker",
		NodeKind:    core.NodeKindAction,
		Icon:        "📦",
		Description: "把本机镜像 tar 包 docker load → tag → push 到指定 registry，输出最终镜像引用",
		InputPorts: []core.PortDef{
			{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
			{ID: "docker", Label: "Docker", PortType: core.PortTypeDockerContext, Required: true},
		},
		OutputPorts: []core.PortDef{
			{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
			{ID: "image", Label: "image", PortType: core.PortTypeString},
		},
		ConfigSchema: []core.FieldSchema{
			{Type: "file_path", ID: "tar_path", Label: "镜像 tar 包",
				Placeholder: "拖入 .tar 文件，或粘贴绝对路径", Required: true},
			{Type: "env_select", ID: "environment_id", Label: "环境", Required: true},
			{Type: "env_config_select", ID: "registry_config_id", Label: "镜像仓库",
				Required: true, ConfigKindFilter: string(core.EnvConfigKindRegistry)},
			{Type: "text", ID: "repository", Label: "Repository", Required: true,
				Placeholder: "myteam/myapp"},
			{Type: "text", ID: "tag", Label: "Tag", Required: true,
				Placeholder: "v1.0"},
		},
		ExecutionMode: core.ExecutionModeRemoteCmd,
	}
}

// Execute 主流程
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
	tarPath := strings.TrimSpace(ctx.ConfigString("tar_path"))
	if tarPath == "" {
		return nil, fmt.Errorf("tar_path 未配置")
	}
	repository := strings.TrimSpace(ctx.ConfigString("repository"))
	tag := strings.TrimSpace(ctx.ConfigString("tag"))
	if repository == "" || tag == "" {
		return nil, fmt.Errorf("repository / tag 必须填写")
	}

	// 解析环境 → 找 registry 配置
	envID := strings.TrimSpace(ctx.ConfigString("environment_id"))
	configID := strings.TrimSpace(ctx.ConfigString("registry_config_id"))
	if envID == "" || configID == "" {
		return nil, fmt.Errorf("environment_id / registry_config_id 未配置")
	}
	envStore := ctx.EnvironmentStore()
	if envStore == nil {
		return nil, fmt.Errorf("引擎未配置 environmentStore")
	}
	env, err := envStore.Get(envID)
	if err != nil {
		return nil, err
	}
	regItem, err := findRegistryConfig(env, configID)
	if err != nil {
		return nil, err
	}
	regURL := stringField(regItem.Fields, "url")
	regUser := stringField(regItem.Fields, "user")
	regPassword := stringField(regItem.Fields, "password")
	registryHost, err := clients.NormalizeRegistryHost(regURL)
	if err != nil {
		return nil, err
	}

	// 取上游 DockerContext
	dockerCli, err := dockerClientFromInput(ctx)
	if err != nil {
		return nil, err
	}
	api := dockerCli.API()

	// 打开 tar 并 ImageLoad
	tarFile, err := os.Open(tarPath)
	if err != nil {
		return nil, fmt.Errorf("打开 tar 失败 %s: %w", tarPath, err)
	}
	defer tarFile.Close()

	ctx.Info("docker load %s ...", tarPath)
	loadResp, err := api.ImageLoad(ctx.Context(), tarFile, dockerclient.ImageLoadWithQuiet(false))
	if err != nil {
		return nil, fmt.Errorf("ImageLoad 失败: %w", err)
	}
	defer loadResp.Body.Close()

	loadedRef, err := parseLoadedRef(loadResp.Body, func(msg string) {
		ctx.Info("docker load: %s", msg)
	})
	if err != nil {
		return nil, err
	}
	if loadedRef == "" {
		return nil, fmt.Errorf("ImageLoad 输出中未识别到镜像引用；请确认 tar 是 docker save 产物")
	}
	ctx.Info("已加载镜像: %s", loadedRef)

	// Tag: <registryHost>/<repository>:<tag>
	targetRef := fmt.Sprintf("%s/%s:%s", registryHost, repository, tag)
	if err := api.ImageTag(ctx.Context(), loadedRef, targetRef); err != nil {
		return nil, fmt.Errorf("ImageTag %s → %s 失败: %w", loadedRef, targetRef, err)
	}
	ctx.Info("已 tag: %s", targetRef)

	// Push（带凭据；匿名仓库 BuildRegistryAuth 返回空串，SDK 也能接受）
	authToken, err := clients.BuildRegistryAuth(regUser, regPassword, registryHost)
	if err != nil {
		return nil, err
	}
	ctx.Info("docker push %s ...", targetRef)
	pushResp, err := api.ImagePush(ctx.Context(), targetRef, image.PushOptions{
		RegistryAuth: authToken,
	})
	if err != nil {
		return nil, fmt.Errorf("ImagePush %s 失败: %w", targetRef, err)
	}
	defer pushResp.Close()
	if err := drainPushResponse(pushResp, func(msg string) {
		ctx.Info("docker push: %s", msg)
	}); err != nil {
		return nil, err
	}
	ctx.Info("镜像推送完成: %s", targetRef)

	return engine.Outputs{
		"image": targetRef,
	}, nil
}

// parseLoadedRef 从 docker load 流式 JSON 输出中提取被加载的镜像引用
// 典型行：{"stream":"Loaded image: nginx:1.19\n"}
//
//	或：{"stream":"Loaded image ID: sha256:<digest>\n"}
//
// 优先返回 repo:tag；只有 ID 时返回 sha256:... 也能作为 ImageTag 的源
func parseLoadedRef(body io.Reader, onLine func(string)) (string, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var loadedRef string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg struct {
			Stream string `json:"stream"`
			Error  string `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			// 非 JSON 行直接当文本回传日志
			if onLine != nil {
				onLine(line)
			}
			continue
		}
		if msg.Error != "" {
			return "", fmt.Errorf("docker load 报错: %s", msg.Error)
		}
		text := strings.TrimSpace(msg.Stream)
		if text == "" {
			continue
		}
		if onLine != nil {
			onLine(text)
		}
		// "Loaded image: foo:bar" 优先
		if strings.HasPrefix(text, "Loaded image: ") {
			loadedRef = strings.TrimSpace(strings.TrimPrefix(text, "Loaded image: "))
		} else if loadedRef == "" && strings.HasPrefix(text, "Loaded image ID: ") {
			loadedRef = strings.TrimSpace(strings.TrimPrefix(text, "Loaded image ID: "))
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return "", fmt.Errorf("读取 ImageLoad 输出失败: %w", err)
	}
	return loadedRef, nil
}

// drainPushResponse 必须读到 EOF 才算 push 完成
// 同时检测 error 字段，提前中断报错
func drainPushResponse(body io.Reader, onLine func(string)) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg struct {
			Status         string `json:"status"`
			Error          string `json:"error"`
			ProgressDetail struct {
				Current int64 `json:"current"`
				Total   int64 `json:"total"`
			} `json:"progressDetail"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			if onLine != nil {
				onLine(line)
			}
			continue
		}
		if msg.Error != "" {
			return fmt.Errorf("docker push 报错: %s", msg.Error)
		}
		// 只把关键阶段输到日志，避免每条 progress 都刷屏
		if msg.Status != "" && msg.ProgressDetail.Total == 0 && onLine != nil {
			onLine(msg.Status)
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return fmt.Errorf("读取 ImagePush 输出失败: %w", err)
	}
	return nil
}

// findRegistryConfig 在环境中按 configID 定位 registry 配置
func findRegistryConfig(env core.EnvironmentDef, configID string) (*core.EnvConfigItem, error) {
	for i := range env.Configs {
		c := &env.Configs[i]
		if c.ID == configID {
			if c.Kind != core.EnvConfigKindRegistry {
				return nil, fmt.Errorf("配置 %s 类型 %s 不是 registry", configID, c.Kind)
			}
			return c, nil
		}
	}
	return nil, fmt.Errorf("配置未找到: %s", configID)
}

// dockerClientFromInput 从 docker 输入端口取 DockerContext 句柄
func dockerClientFromInput(ctx engine.ExecContext) (*clients.DockerClient, error) {
	v, ok := ctx.Input("docker")
	if !ok || v == nil {
		return nil, fmt.Errorf("docker 输入端口未连接 DockerContext")
	}
	c, ok := v.(*clients.DockerClient)
	if !ok {
		return nil, fmt.Errorf("docker 输入端口类型不是 *DockerClient，得到 %T", v)
	}
	return c, nil
}

// stringField 从 fields map 中读字符串字段
func stringField(fields map[string]any, key string) string {
	if fields == nil {
		return ""
	}
	if v, ok := fields[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
