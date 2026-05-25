package core

import "time"

type PortType string

// ── 端口类型 ──────────────────────────────────────────────
// Exec = 执行流，其余为数据流；Dynamic = 动态类型（由节点 config 决定）

const (
	PortTypeExec          PortType = "Exec"
	// 基础数据类型
	PortTypeString        PortType = "String"
	PortTypeInt           PortType = "Int"
	PortTypeFloat         PortType = "Float"
	PortTypeBool          PortType = "Bool"
	// 动态类型（由节点 config 决定真实类型）
	PortTypeDynamic       PortType = "Dynamic"
	// 任意类型（调试 sink / 转换源；可连任意数据端口）
	PortTypeAny           PortType = "Any"
	// 业务句柄类型
	PortTypeLinuxSsh        PortType = "LinuxSshConnection"
	PortTypeLinuxFileHandle PortType = "LinuxFileHandle"
	PortTypeDockerContext   PortType = "DockerContext"
	PortTypeK8sContext      PortType = "K8sContext"
	PortTypeNginxInstance   PortType = "NginxInstance"
)

// ── 节点分类 ──────────────────────────────────────────────
// 决定端口结构（有没有 exec 端口），与 UI 分组 category 不同

type NodeKind string

const (
	NodeKindEvent       NodeKind = "event"        // 事件源：无 exec in，有 exec out
	NodeKindAction      NodeKind = "action"       // 执行节点：exec in + exec out
	NodeKindPure        NodeKind = "pure"          // 纯数据：无 exec 端口
	NodeKindFlowControl NodeKind = "flow_control"  // 流程控制：exec in + 多个 exec out
)

// ── 节点状态 ──────────────────────────────────────────────

type NodeState string

const (
	NodeStateIdle        NodeState = "Idle"
	NodeStateConfiguring NodeState = "Configuring"
	NodeStateChecking    NodeState = "Checking"
	NodeStateCheckFailed NodeState = "CheckFailed"
	NodeStateReady       NodeState = "Ready"
	NodeStateExecuting   NodeState = "Executing"
	NodeStateSuccess     NodeState = "Success"
	NodeStateFailed      NodeState = "Failed"
	NodeStateSkipped     NodeState = "Skipped"
	// NodeStateTerminated 节点被 break / Stop 中断（中断时正处于 Executing）
	NodeStateTerminated NodeState = "Terminated"
)

// ── 节点不再带 Stage 字段 ─────────────────────────────────
// 阶段归属由"被哪个事件源节点（system_ready / system_update / system_over）可达"导出
// 详见技术设计文档 §5.1

// ── 执行模式 ──────────────────────────────────────────────

type ExecutionMode string

const (
	ExecutionModeRemoteCmd ExecutionMode = "remote_cmd"
	ExecutionModeAgent     ExecutionMode = "agent"
	ExecutionModeFlow      ExecutionMode = "flow"
)

// ── 工作流状态 ────────────────────────────────────────────

type WorkflowStatus string

const (
	WorkflowStatusIdle       WorkflowStatus = "Idle"
	WorkflowStatusRunning    WorkflowStatus = "Running"
	WorkflowStatusSuccess    WorkflowStatus = "Success"
	WorkflowStatusFailed     WorkflowStatus = "Failed"
	WorkflowStatusTerminated WorkflowStatus = "Terminated"
)

type WorkflowPhase string

const (
	WorkflowPhaseReady  WorkflowPhase = "Ready"
	WorkflowPhaseUpdate WorkflowPhase = "Update"
	WorkflowPhaseOver   WorkflowPhase = "Over"
)

// ── 子工作流激活状态 ──────────────────────────────────────

type ActivationStatus string

const (
	ActivationDormant     ActivationStatus = "Dormant"
	ActivationActivating  ActivationStatus = "Activating"
	ActivationActivated   ActivationStatus = "Activated"
	ActivationRunningOver ActivationStatus = "RunningOver"
)

// ── 时间戳辅助 ────────────────────────────────────────────

type TimeRange struct {
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}
