// 引擎入口：管理多次执行的生命周期
// Run / Stop / Get / List / Remove 由 Wails 绑定层调用

package engine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"OpsEngine/internal/core"
	"OpsEngine/internal/store"
)

// Engine 引擎实例
// 进程内单例，由 app 在 startup 中创建
type Engine struct {
	workflowStore  *store.WorkflowStore
	assembleStore  *store.AssembleStore
	executionStore *store.ExecutionStore // 持久化终态执行记录（可为 nil）
	emitter        Emitter

	runs map[string]*Runtime // key = execution ID（内存中的运行/已结束未删的）
	mu   sync.RWMutex
}

// New 创建引擎实例
// executionStore 传 nil 时不持久化（仅内存）
func New(
	ws *store.WorkflowStore,
	as *store.AssembleStore,
	es *store.ExecutionStore,
	emitter Emitter,
) *Engine {
	return &Engine{
		workflowStore:  ws,
		assembleStore:  as,
		executionStore: es,
		emitter:        emitter,
		runs:           map[string]*Runtime{},
	}
}

// Run 启动一次工作流执行
// 立即返回执行 ID，实际执行在后台 goroutine
func (e *Engine) Run(workflowID string) (string, error) {
	wf, err := e.workflowStore.Get(workflowID)
	if err != nil {
		return "", err
	}

	snapshot, err := BuildSnapshot(wf, e.assembleStore)
	if err != nil {
		return "", err
	}

	rt := newRuntime(wf, snapshot, e.emitter)

	e.mu.Lock()
	e.runs[rt.ID] = rt
	e.mu.Unlock()

	go e.runMain(rt)
	return rt.ID, nil
}

// runMain 在后台 goroutine 中执行工作流主流
//
// 生命周期：
//  1. emit started
//  2. 启动 scheduler（处理 system_update 周期触发）
//  3. 跑 system_ready 主流（如果有）
//  4. 等待条件：
//     - 主流出错 → 立即停止
//     - 有 system_update → 等用户 cancel（rt.ctx.Done）
//     - 否则等所有后台 thread 完成（rt.wg）
//  5. scheduler.Stop + 等所有 goroutine 退出
//  6. 用独立 ctx 跑 system_over 流（如果有）
//  7. emit finished
func (e *Engine) runMain(rt *Runtime) {
	rt.emitter.Emit(EventStarted, map[string]any{
		"executionID": rt.ID,
		"workflowID":  rt.WorkflowID,
		"snapshot":    rt.Snapshot,
		"startedAt":   rt.StartedAt,
	})
	rt.emitter.Emit(EventStatus, map[string]any{
		"executionID": rt.ID,
		"status":      core.WorkflowStatusRunning,
	})

	nodes := rt.Snapshot.Workflow.Nodes
	edges := rt.Snapshot.Workflow.Edges

	// 启动 update 调度器
	scheduler := newScheduler(rt)
	hasUpdate := scheduler.Start(rt.ctx, nodes, edges)

	// 跑 system_ready 主流
	var readyID string
	for _, n := range nodes {
		if n.TypeID == "system_ready" {
			readyID = n.InstanceID
			break
		}
	}

	var mainErr error
	if readyID != "" {
		mainErr = rt.executeFlow(rt.ctx, rt.rootFrame, nodes, edges, readyID)
	}

	if mainErr != nil && !errors.Is(mainErr, context.Canceled) {
		// 主流自己失败：触发取消，让其他 goroutine 退出
		rt.cancel()
	} else if hasUpdate {
		// 主流跑完但有 update：等用户 Stop（rt.ctx.Done）
		<-rt.ctx.Done()
	}
	// 无 update 且主流正常完成 → 不等待，直接进收尾（如果有 thread 由下面的 wg.Wait 兜底）

	// 停止调度器（关 ticker），等所有 goroutine 退出
	scheduler.Stop()
	rt.wg.Wait()

	// 把仍处于 Executing 的节点标记 Terminated
	// 出现场景：用户 Stop / break 触发时被中断的并发分支或主流节点
	rt.markRemainingTerminated()

	// 跑 system_over 流（用独立 ctx，主 ctx 此时可能已 cancel）
	runOver(rt, nodes, edges)

	// 终态判定
	if mainErr != nil && !errors.Is(mainErr, context.Canceled) {
		rt.markFailed(mainErr.Error())
	} else if errors.Is(rt.ctx.Err(), context.Canceled) {
		rt.markTerminated()
	} else {
		rt.markSuccess()
	}

	// 持久化终态记录到磁盘
	if e.executionStore != nil {
		if err := e.executionStore.Save(rt.Record()); err != nil {
			// 无具体节点，写到 rootFrame 的特殊位置
			rt.appendLog(rt.rootFrame, "", "error", "持久化执行记录失败: "+err.Error())
		}
	}
}

// runOver 跑 system_over 流（如果工作流定义了该节点）
// 用独立 ctx（带 30s 超时），不受主 ctx 取消影响
func runOver(rt *Runtime, nodes []core.NodeInstance, edges []core.EdgeConfig) {
	var overID string
	for _, n := range nodes {
		if n.TypeID == "system_over" {
			overID = n.InstanceID
			break
		}
	}
	if overID == "" {
		return
	}
	next := rt.findNextExec(edges, overID, "exec_out")
	if next == "" {
		return
	}

	overCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rt.appendLog(rt.rootFrame, overID, "info", "触发 system_over 流")
	if err := rt.executeFlow(overCtx, rt.rootFrame, nodes, edges, next); err != nil {
		rt.appendLog(rt.rootFrame, overID, "error", "system_over 流失败: "+err.Error())
	}
}

// Stop 取消执行
func (e *Engine) Stop(executionID string) error {
	e.mu.RLock()
	rt, ok := e.runs[executionID]
	e.mu.RUnlock()
	if !ok {
		return fmt.Errorf("执行 %s 不存在", executionID)
	}
	rt.cancel()
	return nil
}

// Get 按 ID 取 Runtime
func (e *Engine) Get(executionID string) (*Runtime, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	rt, ok := e.runs[executionID]
	return rt, ok
}

// ListSummaries 返回所有执行的 Summary（内存 + 持久化合并去重）
// 按开始时间倒序，内存版本优先（运行中 / 刚结束未保存的）
func (e *Engine) ListSummaries() []core.ExecutionSummary {
	seen := map[string]bool{}
	var list []core.ExecutionSummary

	e.mu.RLock()
	for _, rt := range e.runs {
		list = append(list, rt.Summary())
		seen[rt.ID] = true
	}
	e.mu.RUnlock()

	if e.executionStore != nil {
		if recs, err := e.executionStore.List(); err == nil {
			for _, rec := range recs {
				if seen[rec.ID] {
					continue
				}
				list = append(list, rec.Summary())
			}
		}
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].StartedAt.After(list[j].StartedAt)
	})
	return list
}

// ListSummariesByWorkflow 过滤指定工作流的执行
func (e *Engine) ListSummariesByWorkflow(workflowID string) []core.ExecutionSummary {
	all := e.ListSummaries()
	out := make([]core.ExecutionSummary, 0, len(all))
	for _, s := range all {
		if s.WorkflowID == workflowID {
			out = append(out, s)
		}
	}
	return out
}

// GetRecord 返回执行的完整 Record（内存优先，找不到回 store）
func (e *Engine) GetRecord(executionID string) (core.ExecutionRecord, bool) {
	e.mu.RLock()
	rt, inMem := e.runs[executionID]
	e.mu.RUnlock()
	if inMem {
		return rt.Record(), true
	}
	if e.executionStore != nil {
		if rec, err := e.executionStore.Get(executionID); err == nil {
			return rec, true
		}
	}
	return core.ExecutionRecord{}, false
}

// Remove 删除执行记录（内存 + 持久化）
// 运行中拒绝；终态或仅在持久化中存在的均可删
func (e *Engine) Remove(executionID string) error {
	e.mu.Lock()
	rt, inMem := e.runs[executionID]
	if inMem {
		if rt.Status() == core.WorkflowStatusRunning {
			e.mu.Unlock()
			return fmt.Errorf("不能删除运行中的执行")
		}
		delete(e.runs, executionID)
	}
	e.mu.Unlock()

	// 即使内存中没有，也尝试删持久化（容错）
	if e.executionStore != nil {
		_ = e.executionStore.Delete(executionID)
	}
	return nil
}

