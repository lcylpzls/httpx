package core

import "github.com/lcylpzls/metricsx"

// Metrics 是最小指标协议（家族统一契约，定义见 metricsx.Sink）。
// 调用方按 Sink 签名传入标签切片；无标签时传 nil。
type Metrics = metricsx.Sink
