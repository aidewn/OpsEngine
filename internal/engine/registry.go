// 节点注册表
// 每个节点包在 init() 时调用 Register 把自己注册进来
// app.GetNodeTypes() 通过 AllTypeDefs() 拿到全部已注册节点

package engine

import (
	"fmt"
	"sort"

	"OpsEngine/internal/core"
)

// registry 全局节点注册表，key = type_id
var registry = map[string]Node{}

// Register 注册一个节点
// 通常在每个节点包的 init() 中调用
// 重复注册会 panic（提前暴露错误）
func Register(n Node) {
	typeID := n.TypeDef().TypeID
	if _, exists := registry[typeID]; exists {
		panic(fmt.Sprintf("节点类型重复注册: %s", typeID))
	}
	registry[typeID] = n
}

// Lookup 按 typeID 查找节点
func Lookup(typeID string) (Node, bool) {
	n, ok := registry[typeID]
	return n, ok
}

// AllTypeDefs 返回所有静态注册节点的 TypeDef
// 集合调用节点（assemble:<id>）由 app 层动态拼接，不在此处
// 返回结果按 TypeID 排序，保证前端 AddNodeDialog 列表稳定
func AllTypeDefs() []core.NodeTypeDef {
	defs := make([]core.NodeTypeDef, 0, len(registry))
	for _, n := range registry {
		defs = append(defs, n.TypeDef())
	}
	sort.Slice(defs, func(i, j int) bool {
		return defs[i].TypeID < defs[j].TypeID
	})
	return defs
}
