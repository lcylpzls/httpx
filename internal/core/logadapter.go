package core

import (
	"net/http"
	"time"

	"github.com/lcylpzls/logx"
)

// 指标名称:与 dbx 风格一致,统一为 httpx.* 前缀。
const (
	metricRequests     = "httpx.requests"
	metricErrors       = "httpx.errors"
	metricRetries      = "httpx.retries"
	metricSlowRequests = "httpx.slow_requests"
	metricDuration     = "httpx.duration"
)

// observe 统一记录一次请求尝试的指标与日志(默认 no-op)。
func observe(cfg config, attempt int, req *http.Request, resp *http.Response, err error, start time.Time) {
	duration := time.Since(start)
	threshold := cfg.slowThreshold
	if threshold <= 0 {
		threshold = defaultSlowThreshold
	}
	isSlow := duration >= threshold
	method := req.Method
	path := req.URL.Path

	if cfg.metrics != nil {
		cfg.metrics.IncCounter(metricRequests, []string{method})
		cfg.metrics.ObserveDuration(metricDuration, duration.Seconds(), []string{method})
		if err != nil {
			cfg.metrics.IncCounter(metricErrors, []string{method})
		}
		if attempt > 1 {
			cfg.metrics.IncCounter(metricRetries, []string{method})
		}
		if isSlow {
			cfg.metrics.IncCounter(metricSlowRequests, []string{method})
		}
	}
	if cfg.logger == nil {
		return
	}
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	fields := []logx.Field{
		logx.String("method", method),
		logx.String("path", path),
		logx.Int("status", status),
		logx.Int("attempt", attempt),
		logx.String("duration", duration.String()),
	}
	if id := req.Header.Get("X-Request-ID"); id != "" {
		fields = append(fields, logx.String("request_id", id))
	}
	if err != nil {
		fields = append(fields, logx.String("error", err.Error()))
	}
	if cfg.logRequest {
		cfg.logger.Debug("HTTP 请求", logx.Fields(fields...))
	}
	if err != nil {
		cfg.logger.Warn("HTTP 请求失败", logx.Fields(fields...))
	}
	if isSlow {
		cfg.logger.Warn("慢请求", logx.Fields(fields...))
	}
}
