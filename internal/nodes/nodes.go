// 节点聚合器：通过 anonymous import 触发各节点的 init() 注册
// 主程序 import 本包即可拉起所有内置节点
// 新增节点 = 新增一个子包 + 在这里加一行 import

package nodes

import (
	_ "OpsEngine/internal/nodes/assemble_end"
	_ "OpsEngine/internal/nodes/assemble_param"
	_ "OpsEngine/internal/nodes/assemble_start"
	_ "OpsEngine/internal/nodes/break_node"
	_ "OpsEngine/internal/nodes/linux_exec_command"
	_ "OpsEngine/internal/nodes/parallel"
	_ "OpsEngine/internal/nodes/print"
	_ "OpsEngine/internal/nodes/ssh_with_linux"
	_ "OpsEngine/internal/nodes/system_over"
	_ "OpsEngine/internal/nodes/system_ready"
	_ "OpsEngine/internal/nodes/system_update"
	_ "OpsEngine/internal/nodes/thread"
	_ "OpsEngine/internal/nodes/varget"
	_ "OpsEngine/internal/nodes/varset"
)
