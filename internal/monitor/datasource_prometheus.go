// prometheus.query 数据源：通过 Prometheus HTTP API 执行即时 PromQL 查询。

package monitor

import (
	"context"

	"OpsEngine/internal/clients"
	"OpsEngine/internal/core"
)

// KindPrometheusQuery 是 Prometheus 即时查询数据类型。
const KindPrometheusQuery = "prometheus.query"

// prometheusQuerySource 实现 Prometheus 即时查询。
type prometheusQuerySource struct{}

func (prometheusQuerySource) SourceKind() string { return core.MonitorSourceKindPrometheus }
func (prometheusQuerySource) Kind() string       { return KindPrometheusQuery }

// Collect 执行 PromQL 即时查询。
func (prometheusQuerySource) Collect(ctx context.Context, cc CollectContext) (any, error) {
	client, err := clients.NewPrometheusClient(cc.Source.Config)
	if err != nil {
		return nil, err
	}
	return client.Query(ctx, paramString(cc.Task.Params, "query"), paramString(cc.Task.Params, "time"))
}

// TestPrometheusSource 测试 Prometheus 监控源是否可用。
func TestPrometheusSource(ctx context.Context, source core.MonitorSource) error {
	client, err := clients.NewPrometheusClient(source.Config)
	if err != nil {
		return err
	}
	return client.Ping(ctx)
}
