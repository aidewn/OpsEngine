# OpsEngine Phase 8-13 实施计划

> 在 Phase 0-7 完成 MVP 的基础上，补齐编辑器体验、节点配置表单、执行 frame 树、复制粘贴等功能。

---

## 0. 总览

7 个用户提出的问题按依赖关系拆为 6 个 Phase：

| Phase | 主题 | 涉及问题 |
|-------|------|----------|
| 8 | 端口连接规则修订 | 问题 2（exec/data 端口约束 + 重连） |
| 9 | system_update 控制 + break 节点 | 问题 4 |
| 10 | 变量面板编辑 + 节点连接点 hit area | 问题 5 + 问题 7 |
| 11 | 节点配置表单化 | 问题 3 |
| 12 | 执行 Frame 树 + 集合内部状态查看 | 问题 6 |
| 13 | 框选 + 复制粘贴 | 问题 1 |

每个 Phase 都满足「能编译、能跑、能验收」的最小增量原则。

---

## 1. 全局设计约定（贯穿多个 Phase）

### 1.1 端口连接规则（最终版）

| 端口类型 | 约束 |
|---------|------|
| `exec_out`（含 exec_out_<n>、exec_out_done、exec_out_continue、exec_out_thread） | 单出（最多 1 条出边） |
| `exec_in` | **单入**（最多 1 条入边） |
| 数据 output（任何 NodeKindPure 的 output / NodeKindAction 的非 exec output） | 多出（一对多） |
| 数据 input | **单入**（最多 1 条入边） |

**重连语义**（user 已确认）：
- 拖一条边到**已有连接的 input 端口**时：自动断开旧连接 → 创建新连接
- 拖到空白处：仍弹出添加节点弹窗
- 静默完成，无视觉提示

**校验位置**：
- 前端 `isValidConnection`：返回 true 时允许；onConnect 中处理"断开旧连接"
- 后端 `engine.ValidateWorkflow` / `ValidateAssemble`：保存时严格校验所有 input 单连接（不允许 fan-in）

### 1.2 节点状态

新增 `NodeStateTerminated`（被 break / Stop 中断时正在执行的节点）：

```go
const (
    NodeStateIdle       NodeState = "Idle"
    NodeStateExecuting  NodeState = "Executing"
    NodeStateSuccess    NodeState = "Success"
    NodeStateFailed     NodeState = "Failed"
    NodeStateSkipped    NodeState = "Skipped"
    NodeStateTerminated NodeState = "Terminated"  // 新增
)
```

前端 ExecutionStatus 加对应 tone（橙色方块）。

### 1.3 Frame 树数据模型（Phase 12 实装，先约定）

```go
// internal/core/execution.go

type ExecutionRecord struct {
    ID         string                  `json:"id"`
    WorkflowID string                  `json:"workflow_id"`
    Snapshot   ExecutionSnapshot       `json:"snapshot"`
    Status     WorkflowStatus          `json:"status"`
    StartedAt  time.Time               `json:"started_at"`
    FinishedAt *time.Time              `json:"finished_at,omitempty"`
    RootFrame  FrameState              `json:"root_frame"`  // 替代原 NodeStates/NodeLogs/Variables
    Error      string                  `json:"error,omitempty"`
}

type FrameState struct {
    AssembleID string                       `json:"assemble_id"`  // "" 表示主流
    NodeStates map[string]NodeState         `json:"node_states"`
    NodeLogs   map[string][]LogEntry        `json:"node_logs"`
    Variables  map[string]any               `json:"variables"`
    Params     map[string]any               `json:"params,omitempty"`   // 仅 assemble frame
    Returns    map[string]any               `json:"returns,omitempty"`  // 仅 assemble frame
    Children   map[string]*FrameState       `json:"children,omitempty"` // key = caller node instance_id
}
```

Wails 事件 payload 加 `framePath []string`：
- `[]` = 主流 RootFrame
- `["callA"]` = RootFrame.Children["callA"]
- `["callA", "callB"]` = RootFrame.Children["callA"].Children["callB"]

---

## 2. Phase 8：端口连接规则修订

### 目标
- exec_in 改为单入
- 数据 input 改为单入
- 数据 output 多出保持不变
- 已有 input 连接时拖新边 → 自动断开旧的

### 后端改动

**`internal/engine/validate.go`**：
- 新增 `validateInputSingle(edges)`：每个 `(to.Node, to.Port)` 至多 1 条入边
- `ValidateWorkflow` / `ValidateAssemble` 同时调用

```go
func validateInputSingle(edges []core.EdgeConfig) error {
    counts := map[string]int{}
    for _, e := range edges {
        key := e.To.Node + ":" + e.To.Port
        counts[key]++
        if counts[key] > 1 {
            return fmt.Errorf("输入端口 %s 只能接收 1 条边", e.To.Port)
        }
    }
    return nil
}
```

### 前端改动

**`frontend/src/features/workflow/WorkflowCanvas.tsx`**：

- `isValidConnection`：保持允许目标 input 已有连接（不再拒绝），把"断开旧连接"逻辑放到 onConnect
- `onConnect`：
  1. 拿到新连接的 `target` + `targetHandle`
  2. 查找当前 edges 中 `to.node === target && to.port === targetHandle` 的旧边
  3. 如果有，从 edges 数组中过滤掉
  4. 追加新边
  5. 触发 onGraphChange

```ts
const onConnect = useCallback((connection: Connection) => {
  if (!connection.sourceHandle || !connection.targetHandle) return;

  const newEdge: EdgeConfig = {
    from: { node: connection.source, port: connection.sourceHandle },
    to: { node: connection.target, port: connection.targetHandle },
  };

  const g = graphRef.current;

  // 断开目标 input 的旧连接（如果有）
  const cleanedEdges = g.edges.filter(
    (e) => !(e.to.node === connection.target && e.to.port === connection.targetHandle),
  );

  // 检查与新边重复（理论上不会，因为已过滤旧的）
  const exists = cleanedEdges.some(
    (e) =>
      e.from.node === newEdge.from.node &&
      e.from.port === newEdge.from.port &&
      e.to.node === newEdge.to.node &&
      e.to.port === newEdge.to.port,
  );
  if (exists) return;

  // 更新画布 RF edges
  setEdges(() => [...cleanedEdges, newEdge].map(toRfEdge));

  // 持久化
  onGraphChange?.({ ...g, edges: [...cleanedEdges, newEdge] } as T);
}, ...);
```

`isValidConnection` 简化为只检查类型匹配 + exec_out 单出（其他约束由 onConnect 自动处理）。

### 验收

- 拖数据线到已连接的 input → 自动替换为新线
- 拖 exec 线到已连接的 exec_in → 自动替换
- 已存在的工作流如果含多 fan-in → 下次保存被后端拒绝（错误信息提示）
- 单元测试 `TestValidate_InputSingle` 通过

---

## 3. Phase 9：system_update 启动控制 + break 节点

### 目标
- `system_update.config.enabled` 三态：`auto / on / off`，默认 `auto`
- `auto` 模式下：检查 `exec_out` 是否有连接，没有则不启动 scheduler
- 新增 `break` 节点，触发后整个工作流终止
- 新增 `NodeStateTerminated`

### 后端改动

**`internal/nodes/system_update/system_update.go`**：

TypeDef 加 `enabled` 字段：
```go
ConfigSchema: []core.FieldSchema{
    {Type: "select", ID: "enabled", Label: "启用",
        Options: []string{"auto", "on", "off"}, Default: "auto"},
    {Type: "select", ID: "delta_type", Label: "触发方式",
        Options: []string{"interval", "cron", "manual"}, Default: "interval"},
    {Type: "number", ID: "delta_seconds", Label: "间隔（秒）", Default: int64(60)},
    {Type: "text", ID: "cron_expr", Label: "Cron 表达式", Placeholder: "0 */5 * * *"},
},
```

**`internal/engine/scheduler.go`**：

`Scheduler.Start` 加入启动条件判断：

```go
func (s *Scheduler) Start(ctx context.Context, nodes, edges, edges) bool {
    var updateNode *core.NodeInstance
    for i := range nodes {
        if nodes[i].TypeID == "system_update" {
            updateNode = &nodes[i]
            break
        }
    }
    if updateNode == nil {
        return false
    }

    enabled, _ := updateNode.Config["enabled"].(string)
    if enabled == "" {
        enabled = "auto"
    }
    switch enabled {
    case "off":
        return false
    case "auto":
        // 检查 exec_out 是否有连接
        hasDownstream := false
        for _, e := range edges {
            if e.From.Node == updateNode.InstanceID && e.From.Port == "exec_out" {
                hasDownstream = true
                break
            }
        }
        if !hasDownstream {
            return false
        }
    case "on":
        // 强制启动
    }
    // ... 启动 ticker / 不同 delta_type 处理（保持现状）
}
```

**`internal/nodes/break_node/break_node.go`**（新增包，避免 Go 关键字冲突）：

```go
package break_node

import (
    "OpsEngine/internal/core"
    "OpsEngine/internal/engine"
)

func init() { engine.Register(&Node{}) }

type Node struct{}

func (Node) TypeDef() core.NodeTypeDef {
    return core.NodeTypeDef{
        TypeID:      "break",
        DisplayName: "Break",
        Category:    "flow",
        NodeKind:    core.NodeKindAction,
        Icon:        "⛔",
        Description: "终止整个工作流",
        InputPorts: []core.PortDef{
            {ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
        },
        OutputPorts:   []core.PortDef{},
        ExecutionMode: core.ExecutionModeFlow,
    }
}

func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
    // 引擎特判处理，Execute 不会被调用
    return nil, nil
}
```

**`internal/engine/evaluator.go`**：

executeFlow 加 break 特判：
```go
if node.TypeID == "break" {
    r.setNodeState(cur, core.NodeStateSuccess, "")
    r.appendLog(cur, "info", "Break 触发，工作流即将终止")
    r.cancel()  // 取消 ctx，让所有 goroutine 退出，runMain 走 over 流并 markTerminated
    return nil
}
```

**`internal/engine/runtime.go`**：

被 break 中断时正在执行的节点：currently 是 Executing 状态，executeFlow 下次循环检查 ctx.Done 直接 return ctx.Err（不会再 setNodeState）。

新增 `markRemainingTerminated` 兜底（在 runMain wg.Wait 后调用，把所有仍处于 Executing 的节点标为 Terminated）：

```go
func (r *Runtime) markRemainingTerminated() {
    r.mu.Lock()
    defer r.mu.Unlock()
    for nodeID, state := range r.nodeStates {
        if state == core.NodeStateExecuting {
            r.nodeStates[nodeID] = core.NodeStateTerminated
        }
    }
}
```

**`internal/core/types.go`**：加 `NodeStateTerminated` 常量。

**`internal/nodes/nodes.go`**：加 `_ "OpsEngine/internal/nodes/break_node"`。

### 前端改动

**`frontend/src/features/execution/ExecutionStatus.tsx`**：
- `toneOfNodeState` 处理 `Terminated` → 返回 `terminated` tone（橙色方块，已有）

**`frontend/src/types/execution.ts`**：
- `NodeState` 类型加 `'Terminated'`

### 验收

- 默认创建的工作流（system_update 没连线）→ 点击 Run → 主流跑完即结束（不再卡死）
- 工作流配 break 节点 → 触发后整个 runtime 终止 + system_over 跑
- 节点状态：被中断的 print 等显示 Terminated（橙色方块角标）
- 单元测试 `TestSystemUpdate_AutoNoConnection` / `TestBreak_TerminatesWorkflow`

---

## 4. Phase 10：变量面板编辑 + 节点连接点 hit area

### 目标 1：左侧变量面板支持编辑

**`frontend/src/features/workflow/VariablePanel.tsx`**：

每个列表项加 `editing: boolean` 状态。点击列表项主体（非删除按钮区域）→ 切换该项为编辑模式 → 复用 `AddForm` 组件（重命名为 `ItemForm`）渲染。

```tsx
const [editingIdx, setEditingIdx] = useState<number | null>(null);

{items.map((item, idx) =>
  editingIdx === idx ? (
    <ItemForm
      key={idx}
      initial={item}
      showDefault={showDefault}
      existingNames={items.filter((_, i) => i !== idx).map((i) => i.name)}
      onCancel={() => setEditingIdx(null)}
      onSubmit={(updated) => {
        const next = [...items];
        next[idx] = updated;
        onChange(next);
        setEditingIdx(null);
      }}
    />
  ) : (
    <li onClick={() => setEditingIdx(idx)}>...</li>
  )
)}
```

设计要点：
- 改名后旧引用不同步（静默）
- 改类型不警告
- existingNames 排除当前项

### 目标 2：节点连接点 hit area

**`frontend/src/features/workflow/nodes/GenericNode.tsx`** 和其他节点：

Handle 改造：
- 视觉大小保持 14px（exec 方块 12px）
- 用 CSS 让 handle 居中跨越边框（露出半个圆形）
- 加 `::before` 伪元素或 wrapper div 扩大透明 hit area

由于 React Flow 的 Handle 是它自己渲染的 DOM 节点，直接修改 style 即可：

```tsx
<Handle
  type={...}
  position={...}
  id={...}
  style={{
    width: 14,
    height: 14,
    borderRadius: isExec ? 3 : 7,
    background: color,
    border: '2px solid rgba(0,0,0,0.15)',
    // 居中跨边框：transform 偏移
    // left/right 已由 Position 决定，handle 默认 transform: translate(-50%) 或 translate(50%)
  }}
/>
```

补充全局 CSS 让 Handle 有更大的 hit area：

```css
/* index.css */
.react-flow__handle {
  position: relative;
}
.react-flow__handle::before {
  content: '';
  position: absolute;
  inset: -6px;
  border-radius: inherit;
}
```

`HEADER_OFFSET` 和 `ROW_HEIGHT` 不需要改，只是 handle 的视觉直径变大。

### 验收

- 点击变量列表项 → 进入编辑表单 → 修改后保存
- 修改变量名但不同步引用节点 → 引用节点保持不变（运行时 warn）
- 节点 handle 露出半个圆/方块，在边框两侧都好点击
- 鼠标在 handle 外 6px 仍能触发选中（hit area 扩大）

---

## 5. Phase 11：节点配置表单化

这是最大的一个 Phase，把 NodeDetailPanel 的 Config tab 从只读 JSON 替换为可编辑表单。

### 5.1 数据模型扩展

**`internal/core/node.go`**：

`FieldSchema` 加 `textarea` 类型支持：

```go
type FieldSchema struct {
    Type        string   `json:"type"` // text|password|number|select|toggle|textarea
    ID          string   `json:"id"`
    Label       string   `json:"label"`
    Placeholder string   `json:"placeholder,omitempty"`
    Required    bool     `json:"required,omitempty"`
    Min         *int64   `json:"min,omitempty"`
    Max         *int64   `json:"max,omitempty"`
    Default     any      `json:"default,omitempty"`
    Options     []string `json:"options,omitempty"`
    // 未来扩展用：Description / Multiline / Condition 等
}
```

### 5.2 部分节点 ConfigSchema 修订

**print 节点**：加 `default_text` 字段（textarea），message input 未连接时用作 fallback：

```go
ConfigSchema: []core.FieldSchema{
    {Type: "text", ID: "prefix", Label: "前缀", Placeholder: "[DEBUG]"},
    {Type: "textarea", ID: "default_text", Label: "默认文本",
        Placeholder: "message 未连接时使用此文本"},
},
```

Execute 改造：
```go
func (Node) Execute(ctx engine.ExecContext) (engine.Outputs, error) {
    msg, ok := ctx.Input("message")
    if !ok {
        msg = ctx.ConfigString("default_text")
    }
    prefix := ctx.ConfigString("prefix")
    if prefix == "" { prefix = "[INFO]" }
    ctx.Info("%s %v", prefix, msg)
    return nil, nil
}
```

**parallel 节点**：加 `branch_count` 字段：

```go
ConfigSchema: []core.FieldSchema{
    {Type: "number", ID: "branch_count", Label: "分支数",
        Min: ptr(int64(2)), Max: ptr(int64(8)), Default: int64(4)},
},
```

OutputPorts 保持固定 9 个（8 exec_out_<i> + 1 done）。**前端**根据 config.branch_count 只渲染前 N 个 exec_out + done。**前端 isValidConnection** 拦截连接超出 N 的端口。

**system_update**：加 `enabled` 字段（Phase 9 已实施）。

### 5.3 前端：ConfigForm 组件

新增 `frontend/src/features/workflow/ConfigForm.tsx`：

```tsx
interface ConfigFormProps {
  schema: FieldSchema[];
  value: Record<string, unknown>;
  onChange: (next: Record<string, unknown>) => void; // 500ms debounce
}

export function ConfigForm({ schema, value, onChange }: ConfigFormProps) {
  // 内部 local state，debounce 500ms 后调 onChange
  const [local, setLocal] = useState(value);
  const onChangeRef = useRef(onChange);
  onChangeRef.current = onChange;

  // 外部 value 变化时同步（如切换节点）
  useEffect(() => { setLocal(value); }, [value]);

  // debounce
  useEffect(() => {
    const timer = setTimeout(() => {
      if (!isEqual(local, value)) {
        onChangeRef.current(local);
      }
    }, 500);
    return () => clearTimeout(timer);
  }, [local, value]);

  // unmount 时立即 flush
  useEffect(() => {
    return () => {
      if (!isEqual(local, value)) {
        onChangeRef.current(local);
      }
    };
  }, []);

  return (
    <form className="space-y-3">
      {schema.map((field) => (
        <FieldRow key={field.id} field={field}
          value={local[field.id]}
          onChange={(v) => setLocal({ ...local, [field.id]: v })}
        />
      ))}
    </form>
  );
}
```

### 5.4 字段渲染规则

每个 FieldType 一个子组件：

| FieldType | UI 组件 | 备注 |
|-----------|---------|------|
| text | `<Input type=text>` | placeholder |
| password | `<Input type=password>` | placeholder |
| number | `<Input type=number>` | min / max |
| select | 原生 `<select>` 加样式 | options |
| toggle | 自定义 Switch 组件 | bool |
| textarea | `<Textarea rows={4}>` | placeholder |

排版规则：
- label 在上，required 加红 `*`
- 控件在下，全宽
- 字段间距 12px

### 5.5 NodeDetailPanel.Config tab 替换

**`frontend/src/features/workflow/NodeDetailPanel.tsx`**：

```tsx
function ConfigTab({ node, nodeType, onConfigChange }: {
  node: NodeInstance;
  nodeType: NodeTypeDef | undefined;
  onConfigChange: (config: Record<string, unknown>) => void;
}) {
  if (!nodeType || (nodeType.config_schema?.length ?? 0) === 0) {
    return <div className="text-xs text-slate-400">（无配置项）</div>;
  }
  return (
    <ConfigForm
      schema={nodeType.config_schema}
      value={node.config}
      onChange={onConfigChange}
    />
  );
}
```

NodeDetailPanel 通过 props 接收 `onConfigChange` 回调，由父 page 转发到 onGraphChange。

### 5.6 GenericNode 端口渲染：根据 config 动态

**`frontend/src/features/workflow/nodes/GenericNode.tsx`**：

parallel 节点情况：

```tsx
// 计算实际渲染的端口数量
const outputPorts = useMemo(() => {
  if (data.type_id === 'parallel') {
    const n = Math.min(Math.max(Number(data.config.branch_count) || 4, 2), 8);
    return [
      ...Array.from({ length: n }, (_, i) => def?.output_ports.find(p => p.id === `exec_out_${i + 1}`)).filter(Boolean),
      def?.output_ports.find(p => p.id === 'exec_out_done'),
    ].filter(Boolean) as PortDef[];
  }
  return def?.output_ports ?? [];
}, [data.type_id, data.config, def]);
```

通用版本可以抽出工具函数 `effectiveOutputPorts(typeID, config, def)`。

### 5.7 branch_count 改小立刻删边

在 page 层的 `handleWorkflowChange` 加 side effect：

```tsx
const handleWorkflowChange = useCallback((next: WorkflowDef) => {
  // 检查 parallel 节点的 branch_count 是否减小，清理超出端口的边
  const cleaned = cleanupOutOfRangePorts(next);
  update.mutate(cleaned);
}, [update]);
```

`cleanupOutOfRangePorts` 在工作流变化时遍历所有 parallel 节点，删除 `exec_out_<i>` 中 i > branch_count 的边。

### 5.8 isValidConnection 拦截超范围

`WorkflowCanvas.tsx` 的 `isValidConnection`：

```ts
// parallel 节点的 exec_out_<i> 检查 branch_count
if (sourceNode.type_id === 'parallel' && connection.sourceHandle?.match(/^exec_out_(\d+)$/)) {
  const i = parseInt(RegExp.$1);
  const n = Number(sourceNode.config.branch_count) || 4;
  if (i > n) return false;
}
```

### 验收

- 选中 print 节点 → 右侧 Config tab 看到表单（前缀 + 默认文本）
- 修改前缀 → 500ms 后自动保存
- 切到另一个节点 → 前一节点的未保存改动立即 flush
- parallel 节点的 branch_count 改成 3 → 画布上只剩 3 个 exec_out 端口 + done
- 改小后已有边超出范围的自动删除
- system_update 的 enabled = "off" → 运行时不启动 scheduler
- 节点 ConfigSchema 含 textarea 字段时正确渲染多行输入

---

## 6. Phase 12：执行 Frame 树 + 集合内部状态查看

### 6.1 数据模型改造（后端）

**`internal/core/execution.go`**：

```go
type ExecutionRecord struct {
    ID         string                  `json:"id"`
    WorkflowID string                  `json:"workflow_id"`
    Snapshot   ExecutionSnapshot       `json:"snapshot"`
    Status     WorkflowStatus          `json:"status"`
    StartedAt  time.Time               `json:"started_at"`
    FinishedAt *time.Time              `json:"finished_at,omitempty"`
    RootFrame  FrameState              `json:"root_frame"`
    Error      string                  `json:"error,omitempty"`
}

type FrameState struct {
    AssembleID string                       `json:"assemble_id"`
    NodeStates map[string]NodeState         `json:"node_states"`
    NodeLogs   map[string][]LogEntry        `json:"node_logs"`
    Variables  map[string]any               `json:"variables"`
    Params     map[string]any               `json:"params,omitempty"`
    Returns    map[string]any               `json:"returns,omitempty"`
    Children   map[string]*FrameState       `json:"children,omitempty"` // key = caller instance ID
}
```

Phase 7 持久化的 JSON 文件需要兼容 —— 老格式（flat NodeStates）读取时迁移为 RootFrame。或者直接破坏向后兼容（MVP，数据量小）。

### 6.2 Runtime 数据存储

**`internal/engine/runtime.go`**：

替换 `nodeStates / nodeLogs / outputs` 等扁平 map 为 `rootFrame *Frame`，Frame 持有：
- `nodeStates map[string]NodeState`
- `nodeLogs map[string][]LogEntry`
- `outputs map[string]Outputs`
- `variables map[string]any`（替代之前的 mainFrame.Variables）
- `params / returns`（仅 assemble frame）
- `children map[string]*Frame`（key = caller node instance ID）

`Frame` 结构升级：

```go
type Frame struct {
    AssembleID string
    Variables  map[string]any
    Params     map[string]any
    Returns    map[string]any
    Parent     *Frame
    Path       []string  // 从根到该 frame 的 caller ID 序列

    // 该 frame 内的执行状态（之前在 Runtime 上）
    NodeStates map[string]core.NodeState
    NodeLogs   map[string][]core.LogEntry
    Outputs    map[string]Outputs
    Children   map[string]*Frame  // 缓存子 frame 引用，便于按 path 查找
}
```

### 6.3 事件 payload 加 framePath

```go
const (
    // payload 字段：framePath []string
)

func (r *Runtime) setNodeState(frame *Frame, nodeID string, state core.NodeState, errMsg string) {
    frame.NodeStates[nodeID] = state
    payload := map[string]any{
        "executionID": r.ID,
        "framePath":   frame.Path,
        "nodeID":      nodeID,
        "state":       state,
    }
    if errMsg != "" { payload["errorMsg"] = errMsg }
    r.emitter.Emit(EventNode, payload)
}
```

同理 `appendLog` / `setVariable` 都加 framePath。

`evaluator.executeFlow` 接收 `*Frame` 替代 `*FrameStack`，因为 frame 是树节点，stack 退化为「当前 frame 的引用」。

`execAssembleCall` 改成：
```go
func (r *Runtime) execAssembleCall(ctx, parentFrame *Frame, callNode, ...) (Outputs, error) {
    // 创建子 frame
    child := &Frame{
        AssembleID: assembleID,
        Variables:  initVariables(asm.Variables),
        Params:     params,
        Returns:    map[string]any{},
        Parent:     parentFrame,
        Path:       append(append([]string{}, parentFrame.Path...), callNode.InstanceID),
        NodeStates: map[string]core.NodeState{},
        NodeLogs:   map[string][]core.LogEntry{},
        Outputs:    map[string]Outputs{},
    }
    parentFrame.Children[callNode.InstanceID] = child
    // ...
    if err := r.executeFlow(ctx, child, asm.Nodes, asm.Edges, startID); err != nil {
        return nil, err
    }
    // ...
}
```

由于 parallel/thread 分支 fork 不再需要 stack（每个 goroutine 在同一个 frame 内并发），简化为传 `*Frame`。

但**并发分支内调用集合时**：parent.Children[callerID] 会被并发写入，需要锁保护。frame.Children 加 mu。

### 6.4 Runtime.Record 重写

```go
func (r *Runtime) Record() core.ExecutionRecord {
    r.mu.Lock()
    defer r.mu.Unlock()
    return core.ExecutionRecord{
        ID: r.ID,
        WorkflowID: r.WorkflowID,
        Snapshot: r.Snapshot,
        Status: r.status,
        StartedAt: r.StartedAt,
        FinishedAt: r.finishedAt,
        RootFrame: serializeFrame(r.rootFrame),
        Error: r.errMsg,
    }
}

func serializeFrame(f *Frame) core.FrameState {
    state := core.FrameState{
        AssembleID: f.AssembleID,
        NodeStates: copyMap(f.NodeStates),
        NodeLogs:   copyLogsMap(f.NodeLogs),
        Variables:  copyMap(f.Variables),
        Params:     copyMap(f.Params),
        Returns:    copyMap(f.Returns),
    }
    if len(f.Children) > 0 {
        state.Children = make(map[string]*core.FrameState, len(f.Children))
        for k, child := range f.Children {
            sub := serializeFrame(child)
            state.Children[k] = &sub
        }
    }
    return state
}
```

### 6.5 前端数据模型

**`frontend/src/types/execution.ts`**：

```ts
export interface FrameState {
  assemble_id: string;
  node_states: Record<string, NodeState>;
  node_logs: Record<string, LogEntry[]>;
  variables: Record<string, unknown>;
  params?: Record<string, unknown>;
  returns?: Record<string, unknown>;
  children?: Record<string, FrameState>;
}

export interface ExecutionRecord {
  id: string;
  workflow_id: string;
  snapshot: ExecutionSnapshot;
  status: WorkflowStatus;
  started_at: string;
  finished_at?: string;
  root_frame: FrameState;
  error?: string;
}
```

### 6.6 前端 ExecutionStore reducer

事件 payload 多了 `framePath: string[]`，reducer 通过 path 递归找到 frame：

```ts
function getOrCreateFrame(root: FrameState, path: string[]): FrameState {
  let cur = root;
  for (const id of path) {
    cur.children = cur.children ?? {};
    if (!cur.children[id]) {
      cur.children[id] = {
        assemble_id: '',
        node_states: {},
        node_logs: {},
        variables: {},
      };
    }
    cur = cur.children[id];
  }
  return cur;
}

case 'node': {
  const ex = state.executions[p.executionID];
  if (!ex || !ex.snapshot) return state;
  const next = structuredClone(ex);
  const frame = getOrCreateFrame(next.rootFrame!, p.framePath);
  frame.node_states[p.nodeID] = p.state;
  return { executions: { ...state.executions, [p.executionID]: next } };
}
```

为简化，ExecutionState 内可以直接持有 `rootFrame: FrameState`（替代 nodeStates/nodeLogs/variables 的 flat 字段）。

### 6.7 ExecutionDetailPage 面包屑

```tsx
const [framePath, setFramePath] = useState<string[]>([]);

// 通过 framePath 找到当前 frame
const currentFrame = useMemo(() => {
  let f = exec?.root_frame;
  for (const id of framePath) f = f?.children?.[id];
  return f;
}, [exec, framePath]);

// 通过 framePath 找到当前 frame 对应的 nodes / edges
const graph = useMemo(() => {
  if (framePath.length === 0) {
    return { nodes: snapshot.workflow.nodes, edges: snapshot.workflow.edges };
  }
  const assembleID = currentFrame?.assemble_id;
  const asm = snapshot.assembles[assembleID];
  return { nodes: asm.nodes, edges: asm.edges };
}, [snapshot, currentFrame, framePath]);

// 面包屑组件
<Breadcrumb path={framePath} snapshot={snapshot} onClick={(idx) => setFramePath(framePath.slice(0, idx))} />
```

面包屑显示：
```
[主流] / [集合A (caller #abc)] / [集合B (caller #def)]
```

逐层切换：点击某层 → `setFramePath(prefix)`。

### 6.8 双击集合调用节点下钻

**`WorkflowCanvas.tsx`**：onNodeDoubleClick 传入当前节点 typeID 和 instanceID（已经传了 typeID，需要把 instanceID 也加上）。

执行详情页的 `handleNodeDoubleClick`：

```tsx
function handleNodeDoubleClick(typeID: string, instanceID: string) {
  if (typeID.startsWith('assemble:')) {
    // 下钻到该 frame
    setFramePath([...framePath, instanceID]);
  }
}
```

工作流编辑页保持现行为（跳转到集合编辑页）。

### 6.9 NodeDetailPanel 日志数据源

NodeDetailPanel 接收 `framePath`，从 ExecutionStore 读对应 frame 的 nodeLogs。

```tsx
<NodeDetailPanel
  graph={graph}
  selectedNodeId={selectedNodeId}
  executionID={exec.id}
  framePath={framePath}
/>
```

Logs tab 内部：
```tsx
const exec = useExecution(executionID);
const frame = framePathToFrame(exec?.root_frame, framePath);
const logs = frame?.node_logs[nodeID] ?? [];
```

### 6.10 Phase 7 持久化兼容

旧的 ExecutionRecord JSON 格式没有 `root_frame` 字段，加载时容错：

```go
func (s *ExecutionStore) Get(id string) (core.ExecutionRecord, error) {
    // ... 读 JSON
    if rec.RootFrame.NodeStates == nil {
        // 迁移老格式：把扁平 fields 转 RootFrame（如果有的话）
        // 或者忽略，让该记录显示为空 frame（用户重跑即可）
    }
    return rec, nil
}
```

MVP 阶段：忽略迁移，老记录显示为空（用户重跑生成新格式）。

### 验收

- 工作流调用集合 → 运行后双击集合调用节点 → 画布切换到集合内部 + 显示该 frame 的节点状态
- 多次嵌套调用都能正确下钻
- 面包屑显示当前层级，点击中间项跳到该层
- 集合内的变量 / 节点日志正确显示
- 单元测试 `TestExecution_FrameTree`

---

## 7. Phase 13：框选 + 复制粘贴

### 7.1 框选触发

**`WorkflowCanvas.tsx`**：

React Flow 配置：
- `selectionOnDrag={false}`（保持）
- `panOnDrag={true}`（保持）
- 监听 Shift 键：按下时切换为 `selectionOnDrag={true}` + `panOnDrag={false}`

简单实现：

```tsx
const [isShiftHeld, setIsShiftHeld] = useState(false);
useEffect(() => {
  const down = (e: KeyboardEvent) => { if (e.key === 'Shift') setIsShiftHeld(true); };
  const up = (e: KeyboardEvent) => { if (e.key === 'Shift') setIsShiftHeld(false); };
  window.addEventListener('keydown', down);
  window.addEventListener('keyup', up);
  return () => {
    window.removeEventListener('keydown', down);
    window.removeEventListener('keyup', up);
  };
}, []);

<ReactFlow
  selectionOnDrag={isShiftHeld}
  panOnDrag={!isShiftHeld}
  ...
>
```

selectionMode={'partial' | 'full'}：用 `'partial'`（碰到节点即选中，无需完整框住）。

### 7.2 剪贴板状态（React Context）

`frontend/src/features/clipboard/ClipboardContext.tsx`：

```tsx
interface ClipboardData {
  sourceEditorKey: string; // workflow:<id> 或 assemble:<id>，跨 tab 时清空
  nodes: NodeInstance[];
  edges: EdgeConfig[];
}

export function ClipboardProvider({ children }: { children: ReactNode }) {
  const [data, setData] = useState<ClipboardData | null>(null);
  return (
    <ClipboardContext.Provider value={{ data, setData, clear: () => setData(null) }}>
      {children}
    </ClipboardContext.Provider>
  );
}

export function useClipboard() {
  return useContext(ClipboardContext)!;
}
```

挂在 main.tsx 的根。

### 7.3 复制 / 剪切 / 粘贴

在 `WorkflowCanvasPage` / `AssembleCanvasPage` 注册键盘监听：

```tsx
useEffect(() => {
  const editorKey = workflow ? `workflow:${workflow.id}` : `assemble:${assemble.id}`;

  const handler = (e: KeyboardEvent) => {
    const isMod = e.metaKey || e.ctrlKey;
    if (!isMod) return;
    const target = e.target as HTMLElement;
    if (['INPUT', 'TEXTAREA'].includes(target.tagName)) return; // 输入框内不拦截

    if (e.key === 'c') doCopy();
    else if (e.key === 'x') doCut();
    else if (e.key === 'v') doPaste();
  };

  window.addEventListener('keydown', handler);

  // 切到别的 tab 时清空剪贴板
  return () => {
    window.removeEventListener('keydown', handler);
  };
}, [...]);
```

**复制函数**：
```ts
function doCopy() {
  const selectedNodes = workflow.nodes.filter(/* 获取 RF selected 状态 */);
  // 过滤掉单例节点（system_*、assemble_start/end/param）
  const copyable = selectedNodes.filter(n => !isInternalNodeType(n.type_id));
  if (copyable.length === 0) return;
  const ids = new Set(copyable.map(n => n.instance_id));
  // 内部连线（两端都在选中集合内）
  const innerEdges = workflow.edges.filter(e => ids.has(e.from.node) && ids.has(e.to.node));
  setClipboard({ sourceEditorKey: editorKey, nodes: copyable, edges: innerEdges });
}
```

**剪切函数** = 复制 + 删除：
```ts
function doCut() {
  doCopy();
  // 删除选中的可删除节点（onNodesDelete 已有逻辑）
  // 直接调 setNodes / onGraphChange
}
```

**粘贴函数**：
```ts
function doPaste() {
  if (!clipboard.data) return;
  if (clipboard.data.sourceEditorKey !== editorKey) {
    clipboard.clear(); // 跨 tab 清空
    return;
  }
  const mousePos = getCurrentMousePosInFlow(); // 通过 useReactFlow().screenToFlowPosition
  // 计算原节点最左上角作为锚点
  const minX = Math.min(...clipboard.data.nodes.map(n => n.position.x));
  const minY = Math.min(...clipboard.data.nodes.map(n => n.position.y));
  // ID 映射
  const idMap = new Map<string, string>();
  for (const n of clipboard.data.nodes) idMap.set(n.instance_id, newUUID());

  const newNodes = clipboard.data.nodes.map(n => ({
    ...n,
    instance_id: idMap.get(n.instance_id)!,
    position: { x: n.position.x - minX + mousePos.x, y: n.position.y - minY + mousePos.y },
  }));
  const newEdges = clipboard.data.edges.map(e => ({
    from: { node: idMap.get(e.from.node)!, port: e.from.port },
    to: { node: idMap.get(e.to.node)!, port: e.to.port },
  }));
  onGraphChange({ ...workflow, nodes: [...workflow.nodes, ...newNodes], edges: [...workflow.edges, ...newEdges] });
}
```

**鼠标位置获取**：监听 ReactFlow `onMouseMove` 维护最后位置。或者用 `useReactFlow().screenToFlowPosition` + window event。

```tsx
const lastMousePos = useRef({ x: 0, y: 0 });
useEffect(() => {
  const move = (e: MouseEvent) => { lastMousePos.current = { x: e.clientX, y: e.clientY }; };
  window.addEventListener('mousemove', move);
  return () => window.removeEventListener('mousemove', move);
}, []);

function getCurrentMousePosInFlow() {
  return rfInstance.screenToFlowPosition(lastMousePos.current);
}
```

### 7.4 跨 tab 清空

Tab 切换由 React Router 触发，新 page mount 时检查 clipboard.sourceEditorKey 是否匹配，不匹配则 clear。

或者在 `TabsContext` 监听激活 tab 变化，触发 clear。

最简单：page mount 时验证 + 不匹配则 clear。粘贴时再验证一次。

### 7.5 单例节点跳过

`isInternalNodeType` 已有，复制/剪切函数中过滤。

### 验收

- Shift+拖动框选多个节点（动画框出现）
- 框选后左键拖动选中节点 → 整组移动
- Ctrl+C → 内部 ClipboardContext 存数据
- Ctrl+V → 在鼠标位置粘贴新节点 + 内部连线
- 剪切（Ctrl+X）= 复制 + 删除
- 选中含 system_ready → 复制时自动跳过
- 切到另一个工作流 tab → 粘贴失效（剪贴板已清空）

---

## 8. 跨 Phase 风险与开放问题

### 8.1 Phase 12 的并发安全

`Frame.Children` 在并发分支内调用集合时会被并发写入。需要在 Frame 加 mutex（或者在 Runtime 层串行化）。

实现细节：execAssembleCall 中先获取锁再写入 `parent.Children[callerID]`。

### 8.2 Phase 11 的 ConfigForm 性能

每次 onChange 都 setLocal 引起组件重渲染。如果 schema 字段多，可能卡顿。MVP 阶段 schema 一般 < 10 个字段，问题不大。

### 8.3 Phase 11 的 default_text 与 var_get 优先级

print 节点的 message input：
- 连了 var_get → 用 var_get 值
- 没连 → 用 default_text

如果用户连了 var_get(host) 但 host 变量没定义 → var_get 返回 nil → print 收到 nil，应该 fallback 到 default_text 吗？

设计：**Input 返回 false 时才 fallback**。如果连接了但值为 nil，仍然使用 nil（不 fallback），打印「<nil>」字样。这样语义清晰：连了就用连接，没连用默认。

### 8.4 Phase 13 的剪贴板与剪切

剪切 = 复制 + 删除。如果用户剪切后切到别的 tab，剪贴板被清空 → 数据丢了。

折中：剪切的删除操作可以 undo（但目前没有 undo 系统）。MVP 阶段接受这个风险，用户操作不回退。

---

## 9. 阶段间依赖图

```
Phase 8（端口规则） ────┐
                        ├──→ Phase 11（config 表单，包含 parallel branch_count）
Phase 9（update + break）┘

Phase 10（变量编辑 + hit area）── 独立

Phase 12（Frame 树）── 独立于以上

Phase 13（框选 + 复制粘贴）── 独立
```

推荐推进顺序：8 → 9 → 10 → 11 → 12 → 13

也可以并行做 10（独立小改动）。

---

## 10. 验收清单总览

| Phase | 关键里程碑 |
|-------|------------|
| 8 | 数据/exec input 单连接；自动断开旧线 + 连新 |
| 9 | 默认工作流不会卡死；break 节点终止 runtime |
| 10 | 变量列表项可编辑；handle 易于点击 |
| 11 | Config tab 是可用表单；print 默认文本生效；parallel 动态端口数 |
| 12 | 集合调用运行后可下钻查看 frame 内部状态 |
| 13 | 框选 + 复制粘贴流畅可用 |

---

## 11. 下一步

按 Phase 8 → 9 → 10 → 11 → 12 → 13 顺序推进。每个 Phase 完成后做：
1. `go build ./...` + `npx tsc --noEmit` 通过
2. 启动 `wails dev` 人工验收对应阶段功能
3. 复盘是否暴露未考虑到的问题，更新本文档
