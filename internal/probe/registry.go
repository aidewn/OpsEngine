// 探测函数注册表（按节点 TypeID 索引）
// 与 engine.Register 同构：各探测子包在 init() 中调用 Register
// 上层（Wails 绑定、env_probe_* 节点 dynamic 模式）均通过 Run 调度

package probe

import (
	"fmt"
	"sort"
	"sync"

	"OpsEngine/internal/core"
)

var (
	registryMu sync.RWMutex
	registry   = map[string]ProbeFunc{}
)

// Register 把某 TypeID 的探测函数挂入注册表
// 重复注册同一 TypeID 视为编程错误，立即 panic（沿用 engine.Register 风格）
func Register(typeID string, fn ProbeFunc) {
	if typeID == "" {
		panic("probe.Register: typeID 不能为空")
	}
	if fn == nil {
		panic("probe.Register: fn 不能为 nil")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[typeID]; exists {
		panic("probe.Register: 重复注册 " + typeID)
	}
	registry[typeID] = fn
}

// Lookup 查找 TypeID 对应的探测函数
func Lookup(typeID string) (ProbeFunc, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	fn, ok := registry[typeID]
	return fn, ok
}

// ListRegisteredTypeIDs 返回所有已注册探测节点 TypeID，按字典序排序。
func ListRegisteredTypeIDs() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(registry))
	for id := range registry {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Run 调用注册表中的探测函数；TypeID 不存在时返回明确错误
func Run(typeID string, env core.EnvironmentDef, configID string, nodeConfig map[string]any) (ProbeResult, error) {
	fn, ok := Lookup(typeID)
	if !ok {
		return ProbeResult{}, fmt.Errorf("未注册的探测节点类型: %s", typeID)
	}
	return fn(env, configID, nodeConfig)
}
