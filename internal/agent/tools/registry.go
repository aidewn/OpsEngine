// 工具注册表：按名称索引可用工具，由 runtime 在每轮 chat 前注入到 LLM 调用。
// 注册表不是全局单例：每个 App 实例持有一份，方便测试与未来"按会话过滤可用工具"。

package tools

import (
	"fmt"
	"sort"
)

// Registry 维护 name → Tool 的映射。零值不可用，请用 NewRegistry。
type Registry struct {
	tools map[string]Tool
}

// NewRegistry 创建一个空注册表。
func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

// Register 加入一个工具。
// 安全策略：v1 只接受 TierRead；未来 P8 启用写工具时这里加分级允许逻辑。
func (r *Registry) Register(t Tool) error {
	spec := t.Spec()
	if spec.Name == "" {
		return fmt.Errorf("工具缺少 Name")
	}
	if spec.Tier != TierRead {
		return fmt.Errorf("工具 %s 的权限级别 %s 当前未启用", spec.Name, spec.Tier)
	}
	if _, dup := r.tools[spec.Name]; dup {
		return fmt.Errorf("工具名重复: %s", spec.Name)
	}
	r.tools[spec.Name] = t
	return nil
}

// Lookup 按名称取工具，未注册时第二返回值为 false。
func (r *Registry) Lookup(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// List 返回所有已注册工具，按名字字典序排序（让 prompt 输入稳定）。
func (r *Registry) List() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Spec().Name < out[j].Spec().Name })
	return out
}

// IsEmpty 报告注册表是否没有任何工具。chat 路径据此决定是否走工具循环。
func (r *Registry) IsEmpty() bool { return len(r.tools) == 0 }
