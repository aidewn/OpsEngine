// 意图识别单测：覆盖肯定 / 否定 / 显式指定 / 边界用例。

package intent

import "testing"

// TestResolveRules 验证规则识别命中的若干主路径与回退路径。
func TestResolveRules(t *testing.T) {
	cases := []struct {
		name      string
		operation string
		message   string
		want      Kind
	}{
		{"巡检优先触发", "auto", "帮我生成一份 Linux 服务器巡检工作流", KindInspectServer},
		{"纯巡检触发", "auto", "做一份服务器巡检", KindInspectServer},
		{"英文 inspection", "auto", "run an inspection on the server", KindInspectServer},
		{"排查类问答走 troubleshoot", "auto", "这台服务器 CPU 持续偏高应该怎么排查", KindTroubleshoot},
		{"故障描述走 troubleshoot", "auto", "服务起不来了，帮我看下原因", KindTroubleshoot},
		{"普通运维问答", "auto", "Linux 怎么看磁盘 IO 使用率", KindChat},
		{"无关闲聊", "", "你好", KindChat},
		{"否定抑制巡检误命中", "auto", "解释一下这个巡检报告，不要生成工作流", KindChat},
		{"显式指定原样透传", "generate_workflow", "随便问问", KindGenerateWorkflow},
		{"显式指定 inspect", "inspect_server", "随便", KindInspectServer},
		{"显式指定 fix_execution", "fix_execution", "修复", KindFixExecution},
		{"架构分析触发", "auto", "帮我分析下生产环境的架构", KindAnalyzeArchitecture},
		{"英文 architecture", "auto", "show me the architecture of this env", KindAnalyzeArchitecture},
		{"排查优先于架构", "auto", "排查架构里的问题", KindTroubleshoot},
		// P5 回归用例表：生成/修改类输入不再被关键词路由，统一进工具循环对话
		{"生成请求进工具循环", "auto", "create a deploy workflow", KindChat},
		{"解释工作流不误路由", "auto", "解释一下这个工作流是干什么的", KindChat},
		{"先解释再生成进工具循环", "auto", "先解释这个报错，再帮我生成一个修复工作流", KindChat},
		{"排查沉淀进工具循环", "auto", "把刚才的结论固化成一个标准操作", KindChat},
		{"修改请求进工具循环", "auto", "把当前工作流加一个打印节点", KindChat},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.operation, tt.message)
			if got.Kind != tt.want {
				t.Fatalf("Resolve(%q,%q) = %q (%s), want %q", tt.operation, tt.message, got.Kind, got.Reason, tt.want)
			}
			if got.Reason == "" {
				t.Fatalf("Reason 不能为空")
			}
		})
	}
}
