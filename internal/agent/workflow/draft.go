// Workflow 生成的中间数据结构与解析逻辑。
// 模型只输出"草案 JSON"，本包负责把草案落成 core.WorkflowDef：
//   - 临时 id 重写成 UUID
//   - 节点类型存在性校验（通过外部回调，避免反向依赖 store 层）
//   - 边端口映射
//   - 工作流整体校验

package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"

	"github.com/google/uuid"
)

// Draft 是模型输出的工作流草案（结构与历史 ai.go 的 aiGeneratedWorkflow 一致）。
type Draft struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Variables   []core.VariableDef `json:"variables"`
	Nodes       []DraftNode        `json:"nodes"`
	Edges       []DraftEdge        `json:"edges"`
	Notes       []string           `json:"notes"`
}

// DraftNode 是草案节点，id 为模型给的临时占位。
type DraftNode struct {
	ID       string         `json:"id"`
	TypeID   string         `json:"type_id"`
	Config   map[string]any `json:"config"`
	Position DraftPosition  `json:"position"`
}

// DraftPosition 是画布坐标。
type DraftPosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// DraftEdge 是草案边，引用临时节点 id。
type DraftEdge struct {
	From DraftPortRef `json:"from"`
	To   DraftPortRef `json:"to"`
}

// DraftPortRef 是端口引用。
type DraftPortRef struct {
	Node string `json:"node"`
	Port string `json:"port"`
}

// NodeTypeChecker 由调用方注入：判断 type_id 是否对应已注册节点或现存集合。
// 这样 workflow 包不需要直接依赖 assembleStore 等业务设施。
type NodeTypeChecker func(typeID string) error

// ParseDraft 从模型回复中提取并解析 JSON 草案。
// 兼容模型常见的"带 Markdown 围栏 / 前后解释文字"输出，截取首个 { 到末尾 } 之间。
func ParseDraft(reply string) (Draft, error) {
	text := strings.TrimSpace(reply)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return Draft{}, errors.New("AI 返回内容不是 JSON")
	}
	var draft Draft
	if err := json.Unmarshal([]byte(text[start:end+1]), &draft); err != nil {
		return Draft{}, fmt.Errorf("解析 AI 工作流草案失败: %w", err)
	}
	if len(draft.Nodes) == 0 {
		return Draft{}, errors.New("AI 返回的节点列表为空")
	}
	return draft, nil
}

// Materialize 把草案转成 core.WorkflowDef：临时 id 重写、节点类型校验、边重映射，最后跑整体校验。
// checker 为 nil 时跳过节点类型校验（仅供单元测试使用，生产路径必须传入）。
func Materialize(draft Draft, checker NodeTypeChecker) (core.WorkflowDef, error) {
	idMap := make(map[string]string, len(draft.Nodes))
	nodes := make([]core.NodeInstance, 0, len(draft.Nodes))
	for _, n := range draft.Nodes {
		tempID := strings.TrimSpace(n.ID)
		if tempID == "" {
			return core.WorkflowDef{}, errors.New("AI 节点缺少 id")
		}
		if _, dup := idMap[tempID]; dup {
			return core.WorkflowDef{}, fmt.Errorf("AI 节点 id 重复: %s", tempID)
		}
		if checker != nil {
			if err := checker(n.TypeID); err != nil {
				return core.WorkflowDef{}, err
			}
		}
		instanceID := uuid.New().String()
		idMap[tempID] = instanceID
		cfg := n.Config
		if cfg == nil {
			cfg = map[string]any{}
		}
		nodes = append(nodes, core.NodeInstance{
			InstanceID: instanceID,
			TypeID:     n.TypeID,
			Config:     cfg,
			Position:   core.Position{X: n.Position.X, Y: n.Position.Y},
		})
	}

	edges := make([]core.EdgeConfig, 0, len(draft.Edges))
	for _, e := range draft.Edges {
		fromID, ok := idMap[e.From.Node]
		if !ok {
			return core.WorkflowDef{}, fmt.Errorf("边引用未知节点: %s", e.From.Node)
		}
		toID, ok := idMap[e.To.Node]
		if !ok {
			return core.WorkflowDef{}, fmt.Errorf("边引用未知节点: %s", e.To.Node)
		}
		edges = append(edges, core.EdgeConfig{
			From: core.PortRef{Node: fromID, Port: strings.TrimSpace(e.From.Port)},
			To:   core.PortRef{Node: toID, Port: strings.TrimSpace(e.To.Port)},
		})
	}

	name := strings.TrimSpace(draft.Name)
	if name == "" {
		name = "AI 生成工作流"
	}
	variables := draft.Variables
	if variables == nil {
		variables = []core.VariableDef{}
	}
	wf := core.WorkflowDef{
		ID:          uuid.New().String(),
		Name:        name,
		Description: strings.TrimSpace(draft.Description),
		Variables:   variables,
		Nodes:       nodes,
		Edges:       edges,
	}
	if err := engine.ValidateWorkflow(wf); err != nil {
		return core.WorkflowDef{}, fmt.Errorf("AI 生成工作流校验失败: %w", err)
	}
	return wf, nil
}
