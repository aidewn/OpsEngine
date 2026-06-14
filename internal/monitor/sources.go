// 默认数据源装配：把内置的四类 DataSource 注册进一个 Registry。

package monitor

// RegisterDefaults 注册内置 DataSource：host.basic / host.disk / docker.containers /
// k8s.workloads / http.health。任一注册失败立即返回（仅在 Kind 冲突时发生）。
func RegisterDefaults(reg *Registry) error {
	sources := []DataSource{
		hostBasicSource{},
		hostDiskSource{},
		dockerContainersSource{},
		k8sWorkloadsSource{},
		newHTTPHealthSource(),
	}
	for _, ds := range sources {
		if err := reg.Register(ds); err != nil {
			return err
		}
	}
	return nil
}

// DefaultRegistry 构造一个已注册全部内置数据源的注册表。
func DefaultRegistry() (*Registry, error) {
	reg := NewRegistry()
	if err := RegisterDefaults(reg); err != nil {
		return nil, err
	}
	return reg, nil
}
