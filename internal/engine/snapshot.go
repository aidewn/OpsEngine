// 执行快照构造：把工作流和它递归引用的所有集合打包成不可变副本
// 启动执行时调用一次，运行中所有图查询都从快照走，不再回 store

package engine

import (
	"fmt"
	"strings"

	"OpsEngine/internal/core"
	"OpsEngine/internal/store"
)

// BuildSnapshot 递归收集 wf 引用的所有集合
// 返回的 snapshot 与 store 解耦，运行期改 store 不影响已启动的执行
func BuildSnapshot(wf core.WorkflowDef, as *store.AssembleStore) (core.ExecutionSnapshot, error) {
	snapshot := core.ExecutionSnapshot{
		Workflow:  wf,
		Assembles: map[string]core.AssembleDef{},
	}

	// DFS 收集
	var collect func(refs []string) error
	collect = func(refs []string) error {
		for _, id := range refs {
			if _, ok := snapshot.Assembles[id]; ok {
				continue
			}
			asm, err := as.Get(id)
			if err != nil {
				return fmt.Errorf("加载引用集合 %s 失败: %w", id, err)
			}
			snapshot.Assembles[id] = asm
			if err := collect(extractAssembleRefs(asm.Nodes)); err != nil {
				return err
			}
		}
		return nil
	}

	if err := collect(extractAssembleRefs(wf.Nodes)); err != nil {
		return core.ExecutionSnapshot{}, err
	}
	return snapshot, nil
}

// extractAssembleRefs 从节点列表中提取所有被引用的集合 ID
// 集合调用节点的 type_id 形如 "assemble:<uuid>"
func extractAssembleRefs(nodes []core.NodeInstance) []string {
	var refs []string
	for _, n := range nodes {
		if strings.HasPrefix(n.TypeID, "assemble:") {
			refs = append(refs, strings.TrimPrefix(n.TypeID, "assemble:"))
		}
	}
	return refs
}
