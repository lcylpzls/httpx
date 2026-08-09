package httpx

// Metrics 是最小指标接口,默认 no-op,便于外部接 Prometheus 等适配器。
type Metrics interface {
	// IncCounter 增加一个计数指标。
	IncCounter(name string, labels ...string)
	// ObserveDuration 记录一次耗时观测。
	ObserveDuration(name string, seconds float64, labels ...string)
}
