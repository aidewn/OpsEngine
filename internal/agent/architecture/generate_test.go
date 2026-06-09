// GenerateArchitectureDoc 编排测试。

package architecture

import (
	"errors"
	"strings"
	"testing"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
)

func sampleGraph() (core.EnvironmentDef, TopologyGraph) {
	env := core.EnvironmentDef{ID: "env-1", Name: "生产环境"}
	g := TopologyGraph{EnvironmentID: env.ID, EnvironmentName: env.Name}
	envID := NewNodeID(NodeKindEnv, env.ID)
	srv := NewNodeID(NodeKindServer, "cfg-1")
	port := NewNodeID(NodeKindPort, "cfg-1", "80", "tcp")
	g.AddNode(TopologyNode{ID: envID, Kind: NodeKindEnv, Label: env.Name})
	g.AddNode(TopologyNode{ID: srv, Kind: NodeKindServer, Label: "web-01",
		Attrs: map[string]string{"hostname": "web-01.prod"}})
	g.AddNode(TopologyNode{ID: port, Kind: NodeKindPort, Label: "nginx:80",
		Attrs: map[string]string{"port": "80", "proto": "tcp", "process": "nginx"}})
	g.AddEdge(TopologyEdge{From: envID, To: srv, Kind: EdgeContains})
	g.AddEdge(TopologyEdge{From: srv, To: port, Kind: EdgeContains})
	return env, g
}

func TestRenderTopologySummaryIncludesAllParts(t *testing.T) {
	_, g := sampleGraph()
	summary := RenderTopologySummary(g)
	for _, want := range []string{
		"生产环境", "节点统计", "主机 web-01", "hostname=web-01.prod",
		"端口:", "nginx:80", "id=port:cfg-1:80:tcp",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary 缺少 %q\n%s", want, summary)
		}
	}
}

func TestGenerateArchitectureDocWithoutLLM(t *testing.T) {
	env, g := sampleGraph()
	doc, err := GenerateArchitectureDoc(env, g, nil)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Kind != core.OpsDocKindArchitecture {
		t.Fatalf("kind = %q", doc.Kind)
	}
	if doc.Source.EnvironmentID != "env-1" {
		t.Fatalf("source 未带入: %#v", doc.Source)
	}
	for _, want := range []string{
		"# 架构分析：生产环境",
		"## 概览", "## 拓扑事实", "## 架构说明", "## 架构图",
		"```mermaid", "graph TD",
		"未启用 LLM 架构说明",
	} {
		if !strings.Contains(doc.Body, want) {
			t.Fatalf("body 缺少 %q\n%s", want, doc.Body)
		}
	}
}

func TestGenerateArchitectureDocWithLLM(t *testing.T) {
	env, g := sampleGraph()
	called := 0
	sum := Summarizer(func(messages []clients.ChatMessage) (string, error) {
		called++
		if !strings.Contains(messages[0].Content, "节点统计") {
			t.Fatalf("LLM prompt 缺少拓扑摘要: %s", messages[0].Content)
		}
		return "**关键服务**\n- web-01 提供 nginx 反代 (port:cfg-1:80:tcp)", nil
	})
	doc, err := GenerateArchitectureDoc(env, g, sum)
	if err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatalf("Summarizer 调用次数 = %d", called)
	}
	if !strings.Contains(doc.Body, "nginx 反代") {
		t.Fatal("LLM 输出未注入正文")
	}
	if strings.Contains(doc.Body, "未启用 LLM 架构说明") {
		t.Fatal("LLM 已启用时不应保留占位提示")
	}
}

func TestGenerateArchitectureDocLLMFailureIsNotFatal(t *testing.T) {
	env, g := sampleGraph()
	sum := Summarizer(func(_ []clients.ChatMessage) (string, error) {
		return "", errors.New("rate limited")
	})
	doc, err := GenerateArchitectureDoc(env, g, sum)
	if err != nil {
		t.Fatalf("LLM 失败不应抛错: %v", err)
	}
	if !strings.Contains(doc.Body, "LLM 架构说明失败") || !strings.Contains(doc.Body, "rate limited") {
		t.Fatalf("失败原因未回显: %s", doc.Body)
	}
}
