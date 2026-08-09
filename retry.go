package httpx

import (
	"context"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lcylpzls/errx"
)

// defaultBackoffBase 是退避策略的默认基础间隔。
const defaultBackoffBase = 100 * time.Millisecond

// Backoff 计算第 attempt 次重试前的等待时长(attempt 从 1 开始)。
type Backoff func(attempt int) time.Duration

// ExponentialBackoff 返回指数退避策略:base * factor^(attempt-1) + 抖动。
// 非法参数(base <= 0、factor < 1、jitter 越界)回退安全默认值。
func ExponentialBackoff(base time.Duration, factor float64, jitter float64) Backoff {
	if base <= 0 {
		base = defaultBackoffBase
	}
	if factor < 1 {
		factor = 2
	}
	if jitter < 0 {
		jitter = 0
	}
	if jitter > 1 {
		jitter = 1
	}
	return func(attempt int) time.Duration {
		if attempt < 1 {
			attempt = 1
		}
		d := float64(base) * math.Pow(factor, float64(attempt-1))
		if jitter > 0 {
			d += d * jitter * (rand.Float64()*2 - 1)
		}
		if math.IsNaN(d) || d <= 0 {
			return base
		}
		// float64(math.MaxInt64) 实际为 2^63,相等边界也会溢出为负,
		// 因此使用 >= 防护。
		if d >= float64(math.MaxInt64) {
			return time.Duration(math.MaxInt64)
		}
		return time.Duration(d)
	}
}

// FixedBackoff 返回固定间隔退避策略。
func FixedBackoff(interval time.Duration) Backoff {
	if interval <= 0 {
		interval = defaultBackoffBase
	}
	return func(int) time.Duration { return interval }
}

// retryPolicy 描述重试行为,maxAttempts 含首次尝试。
type retryPolicy struct {
	maxAttempts int
	backoff     Backoff
}

// retryableStatuses 是可重试的响应状态码:
// 限流与可恢复的服务端错误。
var retryableStatuses = map[int]bool{
	http.StatusTooManyRequests:     true,
	http.StatusInternalServerError: true,
	http.StatusBadGateway:          true,
	http.StatusServiceUnavailable:  true,
	http.StatusGatewayTimeout:      true,
}

// retryableStatus 判断响应状态码是否值得重试。
func retryableStatus(code int) bool {
	return retryableStatuses[code]
}

// idempotentMethods 是可安全重试的方法白名单。
var idempotentMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodHead:    true,
	http.MethodOptions: true,
	http.MethodPut:     true,
	http.MethodDelete:  true,
}

// idempotentMethod 判断请求方法是否幂等(可安全重试)。
func idempotentMethod(method string) bool {
	return idempotentMethods[method]
}

// do 执行请求,配置了重试时按策略循环。
// 重试规则:仅幂等方法参与;网络错误按 IsRetryable 判定;
// 响应状态码按 retryableStatus 判定;请求体必须可重读。
func (c *Client) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	policy := c.cfg.retry
	if policy == nil {
		start := time.Now()
		resp, err := c.rt.RoundTrip(req)
		observe(c.cfg, 1, req, resp, err, start)
		return resp, err
	}
	current := req
	for attempt := 1; ; attempt++ {
		start := time.Now()
		resp, err := c.rt.RoundTrip(current)
		observe(c.cfg, attempt, current, resp, err, start)

		// 成功或不可重试状态码:直接返回。
		if err == nil && !retryableStatus(resp.StatusCode) {
			return resp, nil
		}
		// 错误但不可重试:直接返回。
		if err != nil && !IsRetryable(err) {
			return nil, err
		}
		// 非幂等方法:无论错误或可重试状态码都不重试。
		if !idempotentMethod(req.Method) {
			if err != nil {
				return nil, err
			}
			return resp, nil
		}
		// 最后一次尝试:网络错误包装为重试耗尽,状态码场景返回最终响应。
		if attempt == policy.maxAttempts {
			if err != nil {
				return nil, errx.Wrap(err, errx.KindUnavailable, CodeRetryExhausted, "重试耗尽")
			}
			return resp, nil
		}
		// 释放可重试状态的响应体,归还连接后再等待。
		if err == nil && resp != nil && resp.Body != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		// 为下一次尝试重读请求体。
		next, rerr := cloneRequestForRetry(req)
		if rerr != nil {
			return nil, rerr
		}
		current = next
		// 计算等待时长:Retry-After 优先于退避策略。
		wait := policy.backoff(attempt)
		if resp != nil {
			wait = retryAfter(resp.Header, wait)
		}
		select {
		case <-ctx.Done():
			return nil, errx.Wrap(ctx.Err(), errx.KindCancelled, CodeRequestFailed, "重试等待被取消")
		case <-time.After(wait):
		}
	}
}

// cloneRequestForRetry 为下一次重试克隆请求并重建可重读的请求体。
// 优先使用 http.Request.GetBody(标准库自动为常见 Reader 设置),
// 其次尝试 io.ReadSeeker 复位;均不可用时返回 HTX_BODY_UNREADABLE。
func cloneRequestForRetry(req *http.Request) (*http.Request, error) {
	clone := req.Clone(req.Context())
	if req.Body == nil {
		return clone, nil
	}
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, errx.Wrap(err, errx.KindUnavailable, CodeRequestFailed, "重建请求体失败")
		}
		clone.Body = body
		return clone, nil
	}
	if rs, ok := req.Body.(io.ReadSeeker); ok {
		if _, err := rs.Seek(0, io.SeekStart); err != nil {
			return nil, errx.Wrap(err, errx.KindUnavailable, CodeRequestFailed, "重置请求体失败")
		}
		clone.Body = io.NopCloser(rs)
		return clone, nil
	}
	return nil, errx.New(errx.KindInvalid, CodeBodyUnreadable, "请求体不可重读,无法重试")
}

// retryAfter 解析 Retry-After 响应头(秒数或 HTTP 日期)。
// 解析失败或已过期时回退到 fallback。
func retryAfter(h http.Header, fallback time.Duration) time.Duration {
	v := h.Get("Retry-After")
	if v == "" {
		return fallback
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return fallback
}
