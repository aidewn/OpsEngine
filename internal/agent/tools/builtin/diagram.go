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
	Title string `json:"title,omitempty"`
	// Direction：TB 垂直流向（默认，适合流程/因果链）/ LR 水平。
	Direction string         `json:"direction,omitempty"`
	Nodes     []diagramNode  `json:"nodes"`
	Edges     []diagramEdge  `json:"edges,omitempty"`
	Groups    []diagramGroup `json:"groups,omitempty"`
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
		Description: "把流程、因果、结构或对比关系画成连线流程图展示给用户（画图优先于长篇文字）。" +
			"适用：排障画\"事实→判断→建议\"因果链（kind=fact/judgment/suggestion）；解释部署/回滚流程、架构关系、方案对比。",
		Tier: tools.TierRead,
		Params: map[string]tools.ParamSpec{
			"spec": {Type: "string", Required: true, Description: "图的 JSON 描述：" +
				`{"title":"标题","direction":"TB或LR","nodes":[{"id":"n1","label":"短文字","kind":"default"}],"edges":[{"from":"n1","to":"n2","label":"可选"}],"groups":[{"label":"分组名","nodes":["n1"]}]}。` +
				"【关键】edges 是流程图的灵魂，必须用 edges 把节点按执行/推导顺序连起来——没有 edges 就是一堆孤立方块，毫无意义。" +
				"edges 的 from/to 必须是 nodes 里的 id（不是 label）。direction 默认 TB（自上而下，适合流程）。" +
				"节点 label 控制在 15 字内、id 唯一、节点数≤30。画完用一两句话点结论，不要复述图中内容。",
			},
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
