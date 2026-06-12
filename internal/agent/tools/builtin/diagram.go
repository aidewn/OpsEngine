// render_diagram 工具：模型主动画示意图——"画图为主"对自由解释类输出的实现。
// 模型输出受限图 schema（nodes/edges/groups），由前端 DiagramView 受控渲染——
// 无 HTML/SVG 注入面，风格永远一致。排障因果链、流程、组件关系、方案对比都走这里。

package builtin

import (
	"encoding/json"
	"fmt"
	"strings"

	"OpsEngine/internal/agent/tools"
	"OpsEngine/internal/core"
)

// maxDiagramNodes 单图节点上限，防止模型产出超大图撑爆渲染。
const maxDiagramNodes = 30

// diagramSpec 是 render_diagram 的输入/输出结构（前端 DiagramView 直接消费）。
type diagramSpec struct {
	Title  string         `json:"title,omitempty"`
	Nodes  []diagramNode  `json:"nodes"`
	Edges  []diagramEdge  `json:"edges,omitempty"`
	Groups []diagramGroup `json:"groups,omitempty"`
}

type diagramNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Kind 决定节点配色：fact/judgment/suggestion（因果链）或 default/primary/warning/danger（通用）。
	Kind string `json:"kind,omitempty"`
}

type diagramEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label,omitempty"`
}

type diagramGroup struct {
	Label string   `json:"label"`
	Nodes []string `json:"nodes"`
}

// RenderDiagram 把模型给的图 spec 校验后产出 diagram 视图。
type RenderDiagram struct{}

func (RenderDiagram) Spec() tools.Spec {
	return tools.Spec{
		Name: "render_diagram",
		Description: "把结构、流程、因果或对比关系画成示意图展示给用户（画图优先于长篇文字）。" +
			"适用：排障时画\"事实→判断→建议\"因果链（节点 kind 用 fact/judgment/suggestion）；" +
			"解释架构/部署流程/方案对比时画关系图。" +
			"spec 为 JSON：{title, nodes:[{id,label,kind}], edges:[{from,to,label}], groups:[{label,nodes:[id...]}]}。" +
			"节点 id 唯一、edges 端点必须存在、节点数≤30。调用后用一两句话点出图的结论即可，不要复述图里已有的内容。",
		Tier: tools.TierRead,
		Params: map[string]tools.ParamSpec{
			"spec": {Type: "string", Description: "图的 JSON 描述（见上）。", Required: true},
		},
	}
}

func (RenderDiagram) Execute(_ tools.ToolContext, args map[string]any) (tools.Result, error) {
	raw, err := argString(args, "spec")
	if err != nil {
		return tools.Result{}, err
	}
	var spec diagramSpec
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &spec); err != nil {
		return tools.Result{}, fmt.Errorf("spec 不是合法 JSON: %w", err)
	}
	if err := validateDiagram(spec); err != nil {
		return tools.Result{}, err
	}
	// 规范化后回写（去除多余字段，保证前端拿到干净结构）
	clean, _ := json.Marshal(spec)
	title := spec.Title
	if title == "" {
		title = "示意图"
	}
	return tools.Result{
		Output: fmt.Sprintf("已生成示意图「%s」（%d 节点）。请用一两句话点出结论，不要复述图中内容。", title, len(spec.Nodes)),
		DisplaySummary: fmt.Sprintf("render_diagram「%s」(%d 节点)", title, len(spec.Nodes)),
		View:           &core.AIViewPayload{Kind: "diagram", Title: title, Data: string(clean)},
	}, nil
}

// validateDiagram 校验图 spec：节点非空、id 唯一且数量受限、边端点存在、分组引用有效。
func validateDiagram(spec diagramSpec) error {
	if len(spec.Nodes) == 0 {
		return fmt.Errorf("nodes 不能为空")
	}
	if len(spec.Nodes) > maxDiagramNodes {
		return fmt.Errorf("节点数 %d 超过上限 %d，请精简或拆分", len(spec.Nodes), maxDiagramNodes)
	}
	ids := make(map[string]bool, len(spec.Nodes))
	for _, n := range spec.Nodes {
		id := strings.TrimSpace(n.ID)
		if id == "" {
			return fmt.Errorf("存在缺少 id 的节点")
		}
		if ids[id] {
			return fmt.Errorf("节点 id 重复: %s", id)
		}
		ids[id] = true
	}
	for _, e := range spec.Edges {
		if !ids[e.From] {
			return fmt.Errorf("边引用了不存在的节点: %s", e.From)
		}
		if !ids[e.To] {
			return fmt.Errorf("边引用了不存在的节点: %s", e.To)
		}
	}
	for _, g := range spec.Groups {
		for _, nid := range g.Nodes {
			if !ids[nid] {
				return fmt.Errorf("分组 %q 引用了不存在的节点: %s", g.Label, nid)
			}
		}
	}
	return nil
}
