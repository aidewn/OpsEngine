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
	Params      []core.ParamDef    `json:"params"`
	Returns     []core.ParamDef    `json:"returns"`
	Variables   []core.VariableDef `json:"variables"`
	Nodes       []DraftNode        `json:"nodes"`
	Edges       []DraftEdge        `json:"edges"`
	Notes       []string           `json:"notes"`
}

// DraftNode 是草案节点，id 为模型给的临时占位（也兼容 instance_id 回显）。
type DraftNode struct {
	ID         string         `json:"id"`
	InstanceID string         `json:"instance_id,omitempty"`
	TypeID     string         `json:"type_id"`
	Config     map[string]any `json:"config"`
	Position   DraftPosition  `json:"position"`
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
	normalizeDraft(&draft)
	return draft, nil
}

// draftNodeTempID 读取节点临时 id：优先 id，其次 instance_id（模型更新已有工作流时常回显后者）。
func draftNodeTempID(n DraftNode) string {
	if id := strings.TrimSpace(n.ID); id != "" {
		return id
	}
	return strings.TrimSpace(n.InstanceID)
}

// normalizeDraft 补齐缺失/重复的节点临时 id，避免 Materialize 因「AI 节点缺少 id」失败。
func normalizeDraft(d *Draft) {
	used := make(map[string]struct{}, len(d.Nodes))
	for i := range d.Nodes {
		id := draftNodeTempID(d.Nodes[i])
		if id == "" {
			id = nextDraftNodeID(used)
		}
		id = ensureUniqueDraftNodeID(id, used)
		d.Nodes[i].ID = id
		used[id] = struct{}{}
	}
}

// nextDraftNodeID 生成尚未占用的 n1/n2/… 占位 id。
func nextDraftNodeID(used map[string]struct{}) string {
	for i := 1; ; i++ {
		id := fmt.Sprintf("n%d", i)
		if _, ok := used[id]; !ok {
			return id
		}
	}
}

// ensureUniqueDraftNodeID 在 id 冲突时追加数字后缀。
func ensureUniqueDraftNodeID(id string, used map[string]struct{}) string {
	if _, ok := used[id]; !ok {
		return id
	}
	base := id
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s_%d", base, i)
		if _, ok := used[candidate]; !ok {
			return candidate
		}
	}
}

// Materialize 把草案转成 core.WorkflowDef：临时 id 重写、节点类型校验、边重映射，最后跑整体校验。
// checker 为 nil 时跳过节点类型校验（仅供单元测试使用，生产路径必须传入）。
func Materialize(draft Draft, checker NodeTypeChecker) (core.WorkflowDef, error) {
	return materializeWorkflow(draft, "", nil, checker)
}

// MaterializeWorkflowUpdate 把草案落成指定 ID 的工作流，用于 AI 直接更新已有工作流。
// 模型回显的已有 instance_id 会保留原值：节点身份跨更新稳定，diff 与画布选中状态才有意义。
func MaterializeWorkflowUpdate(draft Draft, existing core.WorkflowDef, checker NodeTypeChecker) (core.WorkflowDef, error) {
	keep := make(map[string]bool, len(existing.Nodes))
	for _, n := range existing.Nodes {
		keep[n.InstanceID] = true
	}
	wf, err := materializeWorkflow(draft, existing.ID, keep, checker)
	if err != nil {
		return core.WorkflowDef{}, err
	}
	if strings.TrimSpace(wf.Name) == "" {
		wf.Name = existing.Name
	}
	return wf, nil
}

// materializeWorkflow 落地工作流草案。keepIDs 中出现的临时 id 视为已有节点，保留原 instance_id。
func materializeWorkflow(draft Draft, fixedID string, keepIDs map[string]bool, checker NodeTypeChecker) (core.WorkflowDef, error) {
	normalizeDraft(&draft)
	idMap := make(map[string]string, len(draft.Nodes))
	nodes := make([]core.NodeInstance, 0, len(draft.Nodes))
	for _, n := range draft.Nodes {
		tempID := draftNodeTempID(n)
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
		if keepIDs[tempID] {
			instanceID = tempID
		}
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
		ID:          chooseID(fixedID),
		Name:        name,
		Description: strings.TrimSpace(draft.Description),
		Variables:   variables,
		Nodes:       nodes,
		Edges:       edges,
	}
	if err := engine.ValidateWorkflow(wf); err != nil {
		return core.WorkflowDef{}, fmt.Errorf("AI 生成工作流校验失败: %w", err)
	}
	if err := engine.ValidateNodeConfigs(wf.Nodes); err != nil {
		return core.WorkflowDef{}, fmt.Errorf("AI 生成工作流配置校验失败: %w", err)
	}
	return wf, nil
}

// MaterializeAssemble 把草案转成集合定义，并执行集合专用校验。
func MaterializeAssemble(draft Draft, fixedID string, checker NodeTypeChecker) (core.AssembleDef, error) {
	return materializeAssemble(draft, fixedID, nil, checker)
}

// MaterializeAssembleUpdate 把草案落成指定集合的更新版本。
// 与 MaterializeWorkflowUpdate 同理：模型回显的已有 instance_id 保留原值，保证 diff 可对齐。
func MaterializeAssembleUpdate(draft Draft, existing core.AssembleDef, checker NodeTypeChecker) (core.AssembleDef, error) {
	keep := make(map[string]bool, len(existing.Nodes))
	for _, n := range existing.Nodes {
		keep[n.InstanceID] = true
	}
	return materializeAssemble(draft, existing.ID, keep, checker)
}

// materializeAssemble 落地集合草案。keepIDs 中出现的临时 id 保留原 instance_id。
func materializeAssemble(draft Draft, fixedID string, keepIDs map[string]bool, checker NodeTypeChecker) (core.AssembleDef, error) {
	normalizeDraft(&draft)
	idMap := make(map[string]string, len(draft.Nodes))
	nodes := make([]core.NodeInstance, 0, len(draft.Nodes))
	for _, n := range draft.Nodes {
		tempID := draftNodeTempID(n)
		if tempID == "" {
			return core.AssembleDef{}, errors.New("AI 节点缺少 id")
		}
		if _, dup := idMap[tempID]; dup {
			return core.AssembleDef{}, fmt.Errorf("AI 节点 id 重复: %s", tempID)
		}
		typeID := strings.TrimSpace(n.TypeID)
		if typeID == "system_ready" {
			return core.AssembleDef{}, errors.New("集合不能包含 system_ready，请使用 assemble_start 作为入口")
		}
		if checker != nil {
			if err := checker(typeID); err != nil {
				return core.AssembleDef{}, err
			}
		}
		instanceID := uuid.New().String()
		if keepIDs[tempID] {
			instanceID = tempID
		}
		idMap[tempID] = instanceID
		cfg := n.Config
		if cfg == nil {
			cfg = map[string]any{}
		}
		nodes = append(nodes, core.NodeInstance{
			InstanceID: instanceID,
			TypeID:     typeID,
			Config:     cfg,
			Position:   core.Position{X: n.Position.X, Y: n.Position.Y},
		})
	}

	edges := make([]core.EdgeConfig, 0, len(draft.Edges))
	for _, e := range draft.Edges {
		fromID, ok := idMap[e.From.Node]
		if !ok {
			return core.AssembleDef{}, fmt.Errorf("边引用未知节点: %s", e.From.Node)
		}
		toID, ok := idMap[e.To.Node]
		if !ok {
			return core.AssembleDef{}, fmt.Errorf("边引用未知节点: %s", e.To.Node)
		}
		edges = append(edges, core.EdgeConfig{
			From: core.PortRef{Node: fromID, Port: strings.TrimSpace(e.From.Port)},
			To:   core.PortRef{Node: toID, Port: strings.TrimSpace(e.To.Port)},
		})
	}

	name := strings.TrimSpace(draft.Name)
	if name == "" {
		name = "AI 生成集合"
	}
	asm := core.AssembleDef{
		ID:          chooseID(fixedID),
		Name:        name,
		Description: strings.TrimSpace(draft.Description),
		Params:      safeParams(draft.Params),
		Returns:     safeParams(draft.Returns),
		Variables:   safeVariables(draft.Variables),
		Nodes:       nodes,
		Edges:       edges,
	}
	if err := engine.ValidateAssemble(asm); err != nil {
		return core.AssembleDef{}, fmt.Errorf("AI 生成集合校验失败: %w", err)
	}
	if err := engine.ValidateNodeConfigs(asm.Nodes); err != nil {
		return core.AssembleDef{}, fmt.Errorf("AI 生成集合配置校验失败: %w", err)
	}
	return asm, nil
}

func chooseID(fixedID string) string {
	if strings.TrimSpace(fixedID) != "" {
		return strings.TrimSpace(fixedID)
	}
	return uuid.New().String()
}

func safeParams(params []core.ParamDef) []core.ParamDef {
	if params == nil {
		return []core.ParamDef{}
	}
	return params
}

func safeVariables(variables []core.VariableDef) []core.VariableDef {
	if variables == nil {
		return []core.VariableDef{}
	}
	return variables
}
