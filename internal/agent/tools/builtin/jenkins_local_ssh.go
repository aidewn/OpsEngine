// 其余只读工具：Jenkins job 列表、SSH 文件扩展操作、Local 文件操作。
//
//   - Jenkins / SSH find / list_dir 类直接复用 probe.Run
//   - SSH read_file 用 cat（避免 sftp 依赖）
//   - Local 走 Go 原生 os/filepath，不调外部 shell

package builtin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"OpsEngine/internal/agent/tools"
	"OpsEngine/internal/core"
	"OpsEngine/internal/probe"
)

// ── jenkins_list_jobs ──────────────────────────────────────────

// JenkinsListJobs 列 Jenkins 任务。
type JenkinsListJobs struct{}

func (JenkinsListJobs) Spec() tools.Spec {
	return tools.Spec{
		Name: "jenkins_list_jobs",
		Description: "列出当前环境 Jenkins 实例上的任务（job）。支持按 folder 子目录过滤。" +
			"用于了解 CI/CD 流水线情况。",
		Tier: tools.TierRead,
		Params: map[string]tools.ParamSpec{
			"folder": {Type: "string", Description: "Jenkins folder 路径，例 my-team（可选）。"},
		},
	}
}

func (JenkinsListJobs) Execute(ctx tools.ToolContext, args map[string]any) (tools.Result, error) {
	cfg, env, err := pickEnvConfig(ctx, core.EnvConfigKindJenkins)
	if err != nil {
		return tools.Result{}, err
	}
	res, err := probe.Run("env_probe_jenkins_jobs", env, cfg.ID, map[string]any{
		"folder": argStringOptional(args, "folder"),
	})
	if err != nil {
		return tools.Result{}, err
	}
	type entry struct {
		Name string `json:"name"`
		URL  string `json:"url,omitempty"`
	}
	entries := make([]entry, 0, len(res.Items))
	for _, it := range res.Items {
		url, _ := it.Meta["url"].(string)
		entries = append(entries, entry{Name: it.Key, URL: url})
	}
	data, _ := json.MarshalIndent(entries, "", "  ")
	return tools.Result{
		Output:         tools.TruncateOutput(string(data)),
		DisplaySummary: fmt.Sprintf("jenkins_list_jobs @ %s（%d 个）", env.Name, len(entries)),
	}, nil
}

// ── ssh_find_files ─────────────────────────────────────────────

// SSHFindFiles 在 SSH 目标上 find 文件。复用 env_probe_ssh_find_files 的 Probe。
type SSHFindFiles struct{}

func (SSHFindFiles) Spec() tools.Spec {
	return tools.Spec{
		Name: "ssh_find_files",
		Description: "在目标 SSH 服务器的指定目录下按名字模式查找文件。" +
			"用于定位配置文件、日志、二进制等。慎用于 / 等大目录，会拖慢响应。",
		Tier: tools.TierRead,
		Params: map[string]tools.ParamSpec{
			"root":    {Type: "string", Description: "起始目录（绝对路径），例 /etc 或 /var/log。", Required: true},
			"pattern": {Type: "string", Description: "find -name 模式，例 *.conf 或 nginx*。"},
			"max":     {Type: "integer", Description: "返回结果上限，默认 50，上限 200。", Default: 50},
		},
	}
}

func (SSHFindFiles) Execute(ctx tools.ToolContext, args map[string]any) (tools.Result, error) {
	root, err := argString(args, "root")
	if err != nil {
		return tools.Result{}, err
	}
	if !looksLikeSafePath(root) {
		return tools.Result{}, fmt.Errorf("root 必须是绝对路径，不含 shell 特殊字符")
	}
	cfgID, fields, envName, err := pickSSHTargetForTool(ctx)
	if err != nil {
		return tools.Result{}, err
	}
	_ = cfgID
	// 直接走 SSH 命令以避免 probe 路径需要节点 config 包装
	pattern := argStringOptional(args, "pattern")
	if pattern == "" {
		pattern = "*"
	}
	if strings.ContainsAny(pattern, "`$|&\n\r<>;") {
		return tools.Result{}, fmt.Errorf("pattern 不允许 shell 特殊字符")
	}
	maxN := argInt(args, "max", 50)
	if maxN < 1 {
		maxN = 50
	}
	if maxN > 200 {
		maxN = 200
	}
	cmd := fmt.Sprintf("find %s -maxdepth 5 -name %s 2>/dev/null | head -%d",
		shellQuote(root), shellQuote(pattern), maxN)
	out, err := dialAndRun(fields, cmd)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{
		Output:         out,
		DisplaySummary: fmt.Sprintf("ssh_find_files %s %s @ %s", root, pattern, envName),
	}, nil
}

// ── ssh_read_file ──────────────────────────────────────────────

// SSHReadFile 读 SSH 目标上文件的内容（cat），与 ssh_read_log 的 tail 形态区分开。
type SSHReadFile struct{}

func (SSHReadFile) Spec() tools.Spec {
	return tools.Spec{
		Name: "ssh_read_file",
		Description: "读取目标 SSH 服务器上文件的完整内容（cat），最多 64KB。" +
			"用于查看配置文件、小型脚本、文本数据。要看大日志请用 ssh_read_log（tail）。",
		Tier: tools.TierRead,
		Params: map[string]tools.ParamSpec{
			"path": {Type: "string", Description: "文件绝对路径。", Required: true},
		},
	}
}

func (SSHReadFile) Execute(ctx tools.ToolContext, args map[string]any) (tools.Result, error) {
	path, err := argString(args, "path")
	if err != nil {
		return tools.Result{}, err
	}
	if !looksLikeSafePath(path) {
		return tools.Result{}, fmt.Errorf("path 必须是绝对路径，不含 shell 特殊字符")
	}
	_, fields, _, err := pickSSHTargetForTool(ctx)
	if err != nil {
		return tools.Result{}, err
	}
	// head -c 65536：直接限制读取 64KB 字节，比 cat 安全
	cmd := fmt.Sprintf("head -c 65536 %s 2>&1", shellQuote(path))
	out, err := dialAndRun(fields, cmd)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{
		Output:         out,
		DisplaySummary: "ssh_read_file " + path,
	}, nil
}

// ── local_list_dir ─────────────────────────────────────────────

// LocalListDir 列本机目录。不调外部 shell，直接 os.ReadDir。
type LocalListDir struct{}

func (LocalListDir) Spec() tools.Spec {
	return tools.Spec{
		Name: "local_list_dir",
		Description: "列出 OpsEngine 运行所在主机的目录内容。" +
			"用于查看本地数据目录（如 data/workflows/、data/docs/）、确认工件生成情况。",
		Tier: tools.TierRead,
		Params: map[string]tools.ParamSpec{
			"path": {Type: "string", Description: "目录绝对路径或相对工作目录的路径。", Required: true},
		},
	}
}

func (LocalListDir) Execute(_ tools.ToolContext, args map[string]any) (tools.Result, error) {
	path, err := argString(args, "path")
	if err != nil {
		return tools.Result{}, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return tools.Result{}, fmt.Errorf("读取目录失败: %w", err)
	}
	type entry struct {
		Name  string `json:"name"`
		IsDir bool   `json:"is_dir"`
		Size  int64  `json:"size"`
		Mode  string `json:"mode"`
	}
	out := make([]entry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, entry{
			Name: e.Name(), IsDir: e.IsDir(),
			Size: info.Size(), Mode: info.Mode().String(),
		})
	}
	data, _ := json.MarshalIndent(out, "", "  ")
	return tools.Result{
		Output:         tools.TruncateOutput(string(data)),
		DisplaySummary: fmt.Sprintf("local_list_dir %s (%d 项)", path, len(out)),
	}, nil
}

// ── local_find_files ───────────────────────────────────────────

// LocalFindFiles 本机递归查找文件。深度限制 5 层，结果上限 200。
type LocalFindFiles struct{}

func (LocalFindFiles) Spec() tools.Spec {
	return tools.Spec{
		Name: "local_find_files",
		Description: "在 OpsEngine 主机指定根目录下递归查找文件，按名字 glob 模式匹配（最多 5 层深度）。" +
			"用于查找本地配置、日志、工件文件。",
		Tier: tools.TierRead,
		Params: map[string]tools.ParamSpec{
			"root":    {Type: "string", Description: "起始目录路径。", Required: true},
			"pattern": {Type: "string", Description: "文件名 glob 模式（如 *.toml）。默认 '*' 匹配所有。"},
			"max":     {Type: "integer", Description: "返回结果上限，默认 50，上限 200。", Default: 50},
		},
	}
}

func (LocalFindFiles) Execute(_ tools.ToolContext, args map[string]any) (tools.Result, error) {
	root, err := argString(args, "root")
	if err != nil {
		return tools.Result{}, err
	}
	pattern := argStringOptional(args, "pattern")
	if pattern == "" {
		pattern = "*"
	}
	maxN := argInt(args, "max", 50)
	if maxN < 1 {
		maxN = 50
	}
	if maxN > 200 {
		maxN = 200
	}
	results := make([]string, 0, maxN)
	rootDepth := strings.Count(filepath.Clean(root), string(filepath.Separator))
	// WalkDir 内部用闭包累计；命中上限直接返回 filepath.SkipAll 终止遍历
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			// 单个目录无权读时跳过，不让整个 walk 失败
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		depth := strings.Count(filepath.Clean(path), string(filepath.Separator)) - rootDepth
		if depth > 5 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		ok, _ := filepath.Match(pattern, d.Name())
		if !ok {
			return nil
		}
		results = append(results, path)
		if len(results) >= maxN {
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return tools.Result{}, fmt.Errorf("遍历失败: %w", err)
	}
	out := strings.Join(results, "\n")
	if out == "" {
		out = "（无匹配文件）"
	}
	return tools.Result{
		Output:         tools.TruncateOutput(out),
		DisplaySummary: fmt.Sprintf("local_find_files %s %s（%d 项）", root, pattern, len(results)),
	}, nil
}
