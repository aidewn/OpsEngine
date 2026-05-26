// 桌面应用主结构，所有 public 方法自动绑定到前端 JS

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
	"OpsEngine/internal/engine"
	_ "OpsEngine/internal/nodes" // 触发所有内置节点的 init() 注册（同时携带 probe 注册）
	"OpsEngine/internal/probe"
	"OpsEngine/internal/store"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// 集合节点类型 ID 前缀，用于区分内置节点和集合调用节点
const assembleTypePrefix = "assemble:"

// App 桌面应用实例，public 方法通过 Wails 绑定暴露给前端调用
type App struct {
	ctx              context.Context
	workflowStore    *store.WorkflowStore
	assembleStore    *store.AssembleStore
	executionStore   *store.ExecutionStore
	environmentStore *store.EnvironmentStore
	engine           *engine.Engine
}

// NewApp 创建应用实例
func NewApp() *App {
	return &App{}
}

// startup Wails 生命周期钩子，窗口创建前调用
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// 初始化日志
	logger, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(logger)

	// 确保数据目录存在
	for _, dir := range []string{"data/workflows", "data/assembles", "data/executions", "data/environments", "data/logs"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			zap.L().Fatal("创建目录失败", zap.Error(err))
		}
	}

	a.workflowStore = store.NewWorkflowStore("data/workflows")
	a.assembleStore = store.NewAssembleStore("data/assembles")
	a.executionStore = store.NewExecutionStore("data/executions")
	a.environmentStore = store.NewEnvironmentStore("data/environments")
	a.engine = engine.New(
		a.workflowStore,
		a.assembleStore,
		a.executionStore,
		a.environmentStore,
		engine.NewWailsEmitter(ctx),
	)
	zap.L().Info("OpsEngine 桌面应用启动完成")
}

// ── 工作流 CRUD ──────────────────────────────────────────────

// ListWorkflows 获取所有工作流
func (a *App) ListWorkflows() ([]core.WorkflowDef, error) {
	return a.workflowStore.List()
}

// GetWorkflow 按 ID 获取工作流详情
func (a *App) GetWorkflow(id string) (core.WorkflowDef, error) {
	return a.workflowStore.Get(id)
}

// CreateWorkflow 创建新工作流，返回生成的 ID
func (a *App) CreateWorkflow(name string, description string) (string, error) {
	wf := core.WorkflowDef{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		Nodes:       []core.NodeInstance{},
		Edges:       []core.EdgeConfig{},
	}
	if err := a.workflowStore.Save(wf); err != nil {
		return "", err
	}
	return wf.ID, nil
}

// UpdateWorkflow 整体覆盖更新工作流
// 保存前校验：单例节点 + exec 输出端口单连接
func (a *App) UpdateWorkflow(wf core.WorkflowDef) error {
	if err := engine.ValidateWorkflow(wf); err != nil {
		return err
	}
	return a.workflowStore.Save(wf)
}

// DeleteWorkflow 删除工作流
func (a *App) DeleteWorkflow(id string) error {
	return a.workflowStore.Delete(id)
}

// ── 集合 CRUD ───────────────────────────────────────────────

// ListAssembles 获取所有集合
func (a *App) ListAssembles() ([]core.AssembleDef, error) {
	return a.assembleStore.List()
}

// GetAssemble 按 ID 获取集合详情
func (a *App) GetAssemble(id string) (core.AssembleDef, error) {
	return a.assembleStore.Get(id)
}

// CreateAssemble 创建新集合，返回生成的 ID
func (a *App) CreateAssemble(name string, description string) (string, error) {
	a2 := core.AssembleDef{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		Params:      []core.ParamDef{},
		Returns:     []core.ParamDef{},
		Variables:   []core.VariableDef{},
		Nodes:       []core.NodeInstance{},
		Edges:       []core.EdgeConfig{},
	}
	if err := a.assembleStore.Save(a2); err != nil {
		return "", err
	}
	return a2.ID, nil
}

// UpdateAssemble 整体覆盖更新集合
// 保存前校验：结构合法性（单例 + exec_out 单连接） + 循环引用
func (a *App) UpdateAssemble(a2 core.AssembleDef) error {
	if err := engine.ValidateAssemble(a2); err != nil {
		return err
	}
	if err := a.checkCircularRef(a2); err != nil {
		return err
	}
	return a.assembleStore.Save(a2)
}

// checkCircularRef 检查集合 target 是否包含直接或间接引用自身的循环
func (a *App) checkCircularRef(target core.AssembleDef) error {
	visited := map[string]bool{}
	for _, refID := range extractAssembleRefs(target.Nodes) {
		if err := a.dfsCheckRef(target.ID, refID, visited); err != nil {
			return err
		}
	}
	return nil
}

// dfsCheckRef DFS 检测集合引用链中是否再次出现 targetID
func (a *App) dfsCheckRef(targetID, currentID string, visited map[string]bool) error {
	if targetID == currentID {
		return fmt.Errorf("循环引用：集合不能直接或间接引用自身")
	}
	if visited[currentID] {
		return nil
	}
	visited[currentID] = true

	sub, err := a.assembleStore.Get(currentID)
	if err != nil {
		// 引用的集合不存在，跳过（保留容错）
		return nil
	}
	for _, refID := range extractAssembleRefs(sub.Nodes) {
		if err := a.dfsCheckRef(targetID, refID, visited); err != nil {
			return err
		}
	}
	return nil
}

// extractAssembleRefs 从节点列表中提取所有被引用的集合 ID
func extractAssembleRefs(nodes []core.NodeInstance) []string {
	var refs []string
	for _, n := range nodes {
		if strings.HasPrefix(n.TypeID, assembleTypePrefix) {
			refs = append(refs, strings.TrimPrefix(n.TypeID, assembleTypePrefix))
		}
	}
	return refs
}

// DeleteAssemble 删除集合
func (a *App) DeleteAssemble(id string) error {
	return a.assembleStore.Delete(id)
}

// ── 执行 ─────────────────────────────────────────────────

// RunWorkflow 启动一次工作流执行
// 立即返回执行 ID；执行在后台 goroutine 中进行
// 前端通过 Wails 事件订阅状态/日志变化
func (a *App) RunWorkflow(workflowID string) (string, error) {
	return a.engine.Run(workflowID)
}

// StopWorkflow 取消执行
func (a *App) StopWorkflow(executionID string) error {
	return a.engine.Stop(executionID)
}

// ListExecutions 列出所有执行（内存 + 持久化，按开始时间倒序）
func (a *App) ListExecutions() []core.ExecutionSummary {
	return a.engine.ListSummaries()
}

// ListExecutionsByWorkflow 列出某工作流的所有执行（含历史）
func (a *App) ListExecutionsByWorkflow(workflowID string) []core.ExecutionSummary {
	return a.engine.ListSummariesByWorkflow(workflowID)
}

// GetExecution 获取执行详情（内存优先，找不到回持久化）
func (a *App) GetExecution(executionID string) (core.ExecutionRecord, error) {
	rec, ok := a.engine.GetRecord(executionID)
	if !ok {
		return core.ExecutionRecord{}, fmt.Errorf("执行 %s 不存在", executionID)
	}
	return rec, nil
}

// DeleteExecution 从内存移除执行记录（仅终态可删）
func (a *App) DeleteExecution(executionID string) error {
	return a.engine.Remove(executionID)
}

// ── 节点类型 ─────────────────────────────────────────────────

// GetNodeTypes 获取所有可用的节点类型定义
// 返回 = engine 注册表中的内置节点 + 当前所有集合动态生成的节点类型
func (a *App) GetNodeTypes() []core.NodeTypeDef {
	types := engine.AllTypeDefs()

	// 把每个已存在的集合转换成一个可调用的节点类型
	if a.assembleStore != nil {
		if assembles, err := a.assembleStore.List(); err == nil {
			for _, asm := range assembles {
				types = append(types, assembleToNodeType(asm))
			}
		}
	}
	return types
}

// ── 环境（Environment）CRUD ──────────────────────────────────

// ListEnvironments 获取所有环境
func (a *App) ListEnvironments() ([]core.EnvironmentDef, error) {
	return a.environmentStore.List()
}

// GetEnvironment 按 ID 获取环境详情
func (a *App) GetEnvironment(id string) (core.EnvironmentDef, error) {
	return a.environmentStore.Get(id)
}

// CreateEnvironment 创建新环境，返回生成的 ID
// Configs 初始化为空切片，便于前端零状态展示与后续追加
func (a *App) CreateEnvironment(name, description string) (string, error) {
	env := core.EnvironmentDef{
		ID:          uuid.New().String(),
		Name:        strings.TrimSpace(name),
		Description: description,
		Configs:     []core.EnvConfigItem{},
	}
	if err := validateEnvironment(env); err != nil {
		return "", err
	}
	if err := a.environmentStore.Save(env); err != nil {
		return "", err
	}
	return env.ID, nil
}

// UpdateEnvironment 整体覆盖更新环境定义
func (a *App) UpdateEnvironment(env core.EnvironmentDef) error {
	if err := validateEnvironment(env); err != nil {
		return err
	}
	return a.environmentStore.Save(env)
}

// DeleteEnvironment 删除环境
func (a *App) DeleteEnvironment(id string) error {
	return a.environmentStore.Delete(id)
}

// TestEnvConfig 测试环境内某条配置的连通性
// MVP 阶段仅 SSH 实现真实拨号；其它 kind 返回明确「暂未支持」错误
func (a *App) TestEnvConfig(envID, configID string) error {
	env, err := a.environmentStore.Get(envID)
	if err != nil {
		return err
	}
	var item *core.EnvConfigItem
	for i := range env.Configs {
		if env.Configs[i].ID == configID {
			item = &env.Configs[i]
			break
		}
	}
	if item == nil {
		return fmt.Errorf("配置未找到: %s", configID)
	}

	switch item.Kind {
	case core.EnvConfigKindSSH:
		return testSSHConfig(item.Fields)
	case core.EnvConfigKindDocker:
		return testDockerConfig(env, item.Fields)
	case core.EnvConfigKindK8s:
		return testK8sConfig(item.Fields)
	case core.EnvConfigKindJenkins:
		return testJenkinsConfig(item.Fields)
	default:
		return fmt.Errorf("kind %s 暂未支持测试连接", item.Kind)
	}
}

// testSSHConfig 用 fields 中的账号密码做一次完整 SSH 握手，握手后立即关闭
// 字段缺失或不合法时给出与节点 Execute 一致的提示
func testSSHConfig(fields map[string]any) error {
	host := strings.TrimSpace(envFieldString(fields, "host"))
	user := strings.TrimSpace(envFieldString(fields, "user"))
	password := envFieldString(fields, "password")
	port := envFieldInt(fields, "port")
	timeoutSeconds := envFieldInt(fields, "timeout_seconds")

	if host == "" {
		return fmt.Errorf("SSH 配置缺少 host")
	}
	if user == "" {
		return fmt.Errorf("SSH 配置缺少 user")
	}
	if password == "" {
		return fmt.Errorf("SSH 配置缺少 password")
	}
	if port <= 0 {
		port = 22
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 10
	}

	client, err := clients.DialLinuxSsh(host, port, user, password, timeoutSeconds)
	if err != nil {
		return err
	}
	defer client.Close()
	return nil
}

// ── 编辑态探测（ProbeEnvNode） ──────────────────────────

// ProbeEnvNodeRequest 探测请求
// TypeID = 探测节点类型 ID（如 env_probe_ssh_list_dir）
// NodeConfig = 该节点 config 的浅拷贝（含 path / pattern 等节点特有字段）
type ProbeEnvNodeRequest struct {
	TypeID     string         `json:"type_id"`
	EnvID      string         `json:"env_id"`
	ConfigID   string         `json:"config_id"`
	NodeConfig map[string]any `json:"node_config"`
}

// ProbeEnvNodeResult 探测结果（与 probe.ProbeResult 同形态，Wails 绑定层暴露用）
type ProbeEnvNodeResult struct {
	Items []probe.ProbeItem `json:"items"`
}

// ProbeEnvNode 编辑态「探测一次」入口
// 流程：取环境 → 按 TypeID 在 probe registry 查函数 → 执行 → 返回 items
// 不写 execution 记录；失败错误直接抛给前端 toast
func (a *App) ProbeEnvNode(req ProbeEnvNodeRequest) (ProbeEnvNodeResult, error) {
	if req.TypeID == "" {
		return ProbeEnvNodeResult{}, fmt.Errorf("type_id 未提供")
	}
	if req.EnvID == "" {
		return ProbeEnvNodeResult{}, fmt.Errorf("env_id 未提供")
	}
	env, err := a.environmentStore.Get(req.EnvID)
	if err != nil {
		return ProbeEnvNodeResult{}, err
	}
	res, err := probe.Run(req.TypeID, env, req.ConfigID, req.NodeConfig)
	if err != nil {
		return ProbeEnvNodeResult{}, err
	}
	return ProbeEnvNodeResult{Items: res.Items}, nil
}

// testDockerConfig 通过 over_ssh 模式拨号 + Ping，验证 Docker daemon 可达
// 仅 mode=over_ssh 受支持；其它 mode 返回明确错误
func testDockerConfig(env core.EnvironmentDef, fields map[string]any) error {
	mode := strings.TrimSpace(envFieldString(fields, "mode"))
	if mode == "" {
		mode = "over_ssh"
	}
	if mode != "over_ssh" {
		return fmt.Errorf("暂不支持 Docker mode=%s（Phase 3 仅 over_ssh）", mode)
	}
	sshConfigID := strings.TrimSpace(envFieldString(fields, "ssh_config_id"))
	if sshConfigID == "" {
		return fmt.Errorf("Docker 配置缺少 ssh_config_id")
	}
	var sshFields map[string]any
	for i := range env.Configs {
		c := &env.Configs[i]
		if c.ID == sshConfigID {
			if c.Kind != core.EnvConfigKindSSH {
				return fmt.Errorf("ssh_config_id %s 类型 %s 不是 ssh", sshConfigID, c.Kind)
			}
			sshFields = c.Fields
			break
		}
	}
	if sshFields == nil {
		return fmt.Errorf("ssh_config_id 未找到: %s", sshConfigID)
	}
	host := strings.TrimSpace(envFieldString(sshFields, "host"))
	user := strings.TrimSpace(envFieldString(sshFields, "user"))
	password := envFieldString(sshFields, "password")
	port := envFieldInt(sshFields, "port")
	timeout := envFieldInt(sshFields, "timeout_seconds")
	if host == "" || user == "" || password == "" {
		return fmt.Errorf("引用的 SSH 配置缺少 host/user/password")
	}
	if port <= 0 {
		port = 22
	}
	if timeout <= 0 {
		timeout = 10
	}
	socketPath := strings.TrimSpace(envFieldString(fields, "socket_path"))
	if socketPath == "" {
		socketPath = "/var/run/docker.sock"
	}

	linuxClient, err := clients.DialLinuxSsh(host, port, user, password, timeout)
	if err != nil {
		return err
	}
	dockerClient, err := clients.NewDockerClientOverSSH(linuxClient.Client(), host, port, user, socketPath)
	if err != nil {
		_ = linuxClient.Close()
		return fmt.Errorf("构造 Docker 客户端失败: %w", err)
	}
	defer dockerClient.Close()

	pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := dockerClient.Ping(pingCtx); err != nil {
		return fmt.Errorf("Docker daemon 不可达: %w", err)
	}
	return nil
}

// testJenkinsConfig 用 base_url + user + api_token Ping Jenkins
func testJenkinsConfig(fields map[string]any) error {
	baseURL := strings.TrimSpace(envFieldString(fields, "base_url"))
	user := strings.TrimSpace(envFieldString(fields, "user"))
	token := envFieldString(fields, "api_token")
	timeout := envFieldInt(fields, "timeout_seconds")
	if timeout <= 0 {
		timeout = 10
	}
	client, err := clients.NewJenkinsClient(baseURL, user, token, timeout)
	if err != nil {
		return err
	}
	pingCtx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()
	return client.Ping(pingCtx)
}

// testK8sConfig 解析 kubeconfig + Ping，验证 API Server 可达
func testK8sConfig(fields map[string]any) error {
	kubeconfig := envFieldString(fields, "kubeconfig_yaml")
	if strings.TrimSpace(kubeconfig) == "" {
		return fmt.Errorf("K8s 配置缺少 kubeconfig_yaml")
	}
	contextName := strings.TrimSpace(envFieldString(fields, "context"))
	namespace := strings.TrimSpace(envFieldString(fields, "namespace"))
	k8sClient, err := clients.NewK8sClientFromKubeconfig(kubeconfig, namespace, contextName)
	if err != nil {
		return err
	}
	pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return k8sClient.Ping(pingCtx)
}

// validateEnvironment 校验环境定义结构合法性
// 非空 Name、config ID 唯一、Kind 必须在四个枚举内
func validateEnvironment(env core.EnvironmentDef) error {
	if strings.TrimSpace(env.Name) == "" {
		return fmt.Errorf("环境名称不能为空")
	}
	seen := map[string]bool{}
	for _, item := range env.Configs {
		if item.ID == "" {
			return fmt.Errorf("配置缺少 ID")
		}
		if seen[item.ID] {
			return fmt.Errorf("配置 ID 重复: %s", item.ID)
		}
		seen[item.ID] = true
		if !isValidEnvConfigKind(item.Kind) {
			return fmt.Errorf("配置 %s 的 kind %q 非法", item.ID, item.Kind)
		}
	}
	return nil
}

// isValidEnvConfigKind 是否为受支持的配置 kind
func isValidEnvConfigKind(k core.EnvConfigKind) bool {
	switch k {
	case core.EnvConfigKindSSH,
		core.EnvConfigKindDocker,
		core.EnvConfigKindK8s,
		core.EnvConfigKindJenkins:
		return true
	}
	return false
}

// envFieldString 从 fields map 中读字符串字段，缺失或类型不符回退空串
func envFieldString(fields map[string]any, key string) string {
	if fields == nil {
		return ""
	}
	if v, ok := fields[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// envFieldInt 抹平 Wails JSON 反序列化导致的数值类型差异
// 实际可能出现：float64（JSON 默认）、int64（TOML 解码）、int、json.Number、字符串数字
func envFieldInt(fields map[string]any, key string) int {
	if fields == nil {
		return 0
	}
	v, ok := fields[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case float32:
		return int(n)
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case json.Number:
		i, err := n.Int64()
		if err == nil {
			return int(i)
		}
	case string:
		// 兼容 JSON 中以字符串承载数字的极端情况
		var i int
		_, err := fmt.Sscanf(n, "%d", &i)
		if err == nil {
			return i
		}
	}
	return 0
}

// assembleToNodeType 把集合定义转换为可调用的节点类型
// 端口 = exec_in/out + 每个 param/return 一个数据端口
func assembleToNodeType(asm core.AssembleDef) core.NodeTypeDef {
	inputPorts := []core.PortDef{
		{ID: "exec_in", Label: "▶", PortType: core.PortTypeExec, Required: true},
	}
	for _, p := range asm.Params {
		inputPorts = append(inputPorts, core.PortDef{
			ID:       "param_" + p.Name,
			Label:    p.Name,
			PortType: p.VarType,
		})
	}
	outputPorts := []core.PortDef{
		{ID: "exec_out", Label: "▶", PortType: core.PortTypeExec},
	}
	for _, r := range asm.Returns {
		outputPorts = append(outputPorts, core.PortDef{
			ID:       "return_" + r.Name,
			Label:    r.Name,
			PortType: r.VarType,
		})
	}
	return core.NodeTypeDef{
		TypeID:        assembleTypePrefix + asm.ID,
		DisplayName:   asm.Name,
		Category:      "assemble_call",
		NodeKind:      core.NodeKindAction,
		Icon:          "📦",
		Description:   asm.Description,
		InputPorts:    inputPorts,
		OutputPorts:   outputPorts,
		ExecutionMode: core.ExecutionModeFlow,
	}
}

