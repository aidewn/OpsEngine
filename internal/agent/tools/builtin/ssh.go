// 内置 SSH 工具集合的共享辅助：选目标 + 单命令执行。
//
// 设计：把 SSH 拨号细节集中在 dialAndRun，让每个工具的 Execute 只关心命令构造和参数。
// 工具内一律走只读命令，安全审计点在工具名层，不接受用户/模型传入的任意 shell 字符串。

package builtin

import (
	"fmt"
	"strings"
	"time"

	"OpsEngine/internal/agent/inspection"
	"OpsEngine/internal/agent/tools"
	"OpsEngine/internal/clients"
)

// sshExecTimeout 是单次 SSH 命令的硬超时。
// 设短一点（15s）保证 chat 工具循环不会被长命令拖死。
const sshExecTimeout = 15 * time.Second

// pickSSHTargetForTool 解析 ToolContext 到具体的 SSH 配置 fields。
// 复用 inspection.PickSSHTarget 的"显式 > 唯一 SSH > 报错"决策。
func pickSSHTargetForTool(ctx tools.ToolContext) (configID string, fields map[string]any, envName string, err error) {
	if ctx.EnvLookup == nil {
		return "", nil, "", fmt.Errorf("环境查询未注入")
	}
	env, err := ctx.EnvLookup(ctx.EnvironmentID)
	if err != nil {
		return "", nil, "", err
	}
	pickedID, err := inspection.PickSSHTarget(env, ctx.PreferredConfigID)
	if err != nil {
		return "", nil, "", err
	}
	for _, c := range env.Configs {
		if c.ID == pickedID {
			return pickedID, c.Fields, env.Name, nil
		}
	}
	return "", nil, "", fmt.Errorf("未在环境 %s 中找到配置 %s", env.ID, pickedID)
}

// dialAndRun 拨号目标 SSH 并执行一条命令，返回 stdout+stderr 合并的字符串（已截断到 MaxOutputBytes）。
// 命令在协程内执行，外层用 select+计时器卡超时，避免 SSH session 卡死阻塞 chat 循环。
func dialAndRun(fields map[string]any, command string) (string, error) {
	dial, err := clients.ParseLinuxSshDialConfig(fields)
	if err != nil {
		return "", err
	}
	client, err := dial.Dial()
	if err != nil {
		return "", err
	}
	defer client.Close()

	session, err := client.Client().NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	type result struct {
		out []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := session.CombinedOutput(command)
		done <- result{out, err}
	}()
	select {
	case r := <-done:
		// 命令非零退出不致命：模型可能正想看错误输出（如 ls 一个不存在的目录）。
		// 错误本身只在拨号 / IO 异常时回传。
		return tools.TruncateOutput(strings.TrimRight(string(r.out), "\n")), nil
	case <-time.After(sshExecTimeout):
		_ = session.Close()
		return "", fmt.Errorf("SSH 命令超时（%s）", sshExecTimeout)
	}
}

// argString 是从 LLM args 提取必填字符串的便捷函数。
func argString(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok {
		return "", fmt.Errorf("缺少参数 %s", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("参数 %s 必须是字符串", key)
	}
	return strings.TrimSpace(s), nil
}

// argInt 是从 LLM args 提取整数的便捷函数；
// 兼容 JSON 数字反序列化为 float64 的情况。可选参数缺失时返回 dflt。
func argInt(args map[string]any, key string, dflt int) int {
	v, ok := args[key]
	if !ok {
		return dflt
	}
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	default:
		return dflt
	}
}
