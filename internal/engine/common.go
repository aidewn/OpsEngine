// 节点共用的常量和辅助
// 供 internal/nodes/* 包引用，避免重复

package engine

// VarTypeOptions 变量/参数节点的类型 select 选项
// var_get / var_set / assemble_param 共用同一份列表
var VarTypeOptions = []string{
	"String",
	"Int",
	"Float",
	"Bool",
	"LinuxSshConnection",
	"DockerContext",
	"K8sContext",
	"NginxInstance",
}
