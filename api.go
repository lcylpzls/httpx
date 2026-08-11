package httpx

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"time"

	"github.com/lcylpzls/httpx/internal/core"
	"github.com/lcylpzls/logx"
)

// Version 是当前库版本，与 git tag 保持一致。
const Version = core.Version

// 错误码定义：httpx 各失败场景的错误码，统一为 HTX_*。
const (
	CodeInvalidConfig       = core.CodeInvalidConfig
	CodeUnsupportedProtocol = core.CodeUnsupportedProtocol
	CodeDialFailed          = core.CodeDialFailed
	CodeTLSFailed           = core.CodeTLSFailed
	CodeRequestFailed       = core.CodeRequestFailed
	CodeResponseFailed      = core.CodeResponseFailed
	CodeRetryExhausted      = core.CodeRetryExhausted
	CodeBodyTooLarge        = core.CodeBodyTooLarge
	CodeBodyUnreadable      = core.CodeBodyUnreadable
	CodeRedirectExceeded    = core.CodeRedirectExceeded
	CodeRedirectFailed      = core.CodeRedirectFailed
	CodeUnexpectedStatus    = core.CodeUnexpectedStatus
)

// 协议常量：HTTP/1.1 / HTTP/2 / HTTP/3 选择。
const (
	ProtocolAuto  = core.ProtocolAuto
	ProtocolHTTP1 = core.ProtocolHTTP1
	ProtocolHTTP2 = core.ProtocolHTTP2
	ProtocolHTTP3 = core.ProtocolHTTP3
)

// 公开类型：与 internal/core 保持一致。
type (
	Backoff        = core.Backoff
	Client         = core.Client
	DNSCache       = core.DNSCache
	FileField      = core.FileField
	Hooks          = core.Hooks
	Metrics        = core.Metrics
	Option         = core.Option
	Protocol       = core.Protocol
	ProtocolConfig = core.ProtocolConfig
	RequestOption  = core.RequestOption
	RetryPolicy    = core.RetryPolicy
	Stats          = core.Stats
)

// New 创建 HTTP 客户端。
func New(opts ...Option) (*Client, error) {
	return core.New(opts...)
}

// NewDNSCache 创建 DNS 缓存。
func NewDNSCache(ttl time.Duration) *DNSCache {
	return core.NewDNSCache(ttl)
}

// RegisterHTTP3 注册 HTTP/3 RoundTripper 构造器。
func RegisterHTTP3(builder func(ProtocolConfig) (http.RoundTripper, error)) {
	core.RegisterHTTP3(builder)
}

// EnsureStatus 校验响应状态码。
func EnsureStatus(resp *http.Response, codes ...int) error {
	return core.EnsureStatus(resp, codes...)
}

// ErrorStatus 返回错误对应的 HTTP 状态码。
func ErrorStatus(err error) int {
	return core.ErrorStatus(err)
}

// WriteErrorJSON 将错误以 JSON 形式写入 ResponseWriter。
func WriteErrorJSON(w http.ResponseWriter, err error) {
	core.WriteErrorJSON(w, err)
}

// IsRetryable 判断错误是否值得重试。
func IsRetryable(err error) bool {
	return core.IsRetryable(err)
}

// IsTimeout 判断错误是否为超时。
func IsTimeout(err error) bool {
	return core.IsTimeout(err)
}

// JSON 将响应体解析为 out 并关闭 Body。
func JSON(resp *http.Response, out any) error {
	return core.JSON(resp, out)
}

// ReadBody 读取响应体并关闭 Body。
func ReadBody(resp *http.Response, maxBytes int64) ([]byte, error) {
	return core.ReadBody(resp, maxBytes)
}

// ReadFile 将响应体写入文件并关闭 Body。
func ReadFile(resp *http.Response, path string, maxBytes int64) error {
	return core.ReadFile(resp, path, maxBytes)
}

// ReadStream 逐块读取响应体并回调 fn。
func ReadStream(resp *http.Response, fn func([]byte) error, maxBytes int64) error {
	return core.ReadStream(resp, fn, maxBytes)
}

// ReadString 读取响应体为字符串并关闭 Body。
func ReadString(resp *http.Response, maxBytes int64) (string, error) {
	return core.ReadString(resp, maxBytes)
}

// ExponentialBackoff 返回指数退避策略。
func ExponentialBackoff(base time.Duration, factor, jitter float64) Backoff {
	return core.ExponentialBackoff(base, factor, jitter)
}

// FixedBackoff 返回固定间隔退避策略。
func FixedBackoff(interval time.Duration) Backoff {
	return core.FixedBackoff(interval)
}

// 客户端级选项。
func WithTimeout(d time.Duration) Option {
	return core.WithTimeout(d)
}

func WithDialTimeout(d time.Duration) Option {
	return core.WithDialTimeout(d)
}

func WithTLSHandshakeTimeout(d time.Duration) Option {
	return core.WithTLSHandshakeTimeout(d)
}

func WithResponseHeaderTimeout(d time.Duration) Option {
	return core.WithResponseHeaderTimeout(d)
}

func WithMaxIdleConns(n int) Option {
	return core.WithMaxIdleConns(n)
}

func WithMaxIdleConnsPerHost(n int) Option {
	return core.WithMaxIdleConnsPerHost(n)
}

func WithIdleConnTimeout(d time.Duration) Option {
	return core.WithIdleConnTimeout(d)
}

func WithTLSClientConfig(cfg *tls.Config) Option {
	return core.WithTLSClientConfig(cfg)
}

func WithProtocol(p Protocol) Option {
	return core.WithProtocol(p)
}

func WithRetry(maxAttempts int, backoff Backoff) Option {
	return core.WithRetry(maxAttempts, backoff)
}

func WithRetryPolicy(p RetryPolicy) Option {
	return core.WithRetryPolicy(p)
}

func WithLogger(l logx.Logger) Option {
	return core.WithLogger(l)
}

func WithLogRequest(enabled bool) Option {
	return core.WithLogRequest(enabled)
}

func WithSlowThreshold(d time.Duration) Option {
	return core.WithSlowThreshold(d)
}

func WithMetrics(m Metrics) Option {
	return core.WithMetrics(m)
}

func WithMaxRedirects(n int) Option {
	return core.WithMaxRedirects(n)
}

func WithNoRedirect() Option {
	return core.WithNoRedirect()
}

func WithRedirectPolicy(policy func(*http.Request, []*http.Request) error) Option {
	return core.WithRedirectPolicy(policy)
}

func WithCookieJar(jar http.CookieJar) Option {
	return core.WithCookieJar(jar)
}

func WithHooks(h Hooks) Option {
	return core.WithHooks(h)
}

func WithProxy(proxy func(*http.Request) (*url.URL, error)) Option {
	return core.WithProxy(proxy)
}

func WithDisableCompression(disabled bool) Option {
	return core.WithDisableCompression(disabled)
}

func WithDNSCache(cache *DNSCache) Option {
	return core.WithDNSCache(cache)
}

func WithMaxConcurrency(n int) Option {
	return core.WithMaxConcurrency(n)
}

func WithHTTP2HealthCheck(readIdle, pingTimeout time.Duration) Option {
	return core.WithHTTP2HealthCheck(readIdle, pingTimeout)
}

func WithMaxResponseHeaderBytes(n int64) Option {
	return core.WithMaxResponseHeaderBytes(n)
}

func WithMaxConnsPerHost(n int) Option {
	return core.WithMaxConnsPerHost(n)
}

func WithExpectContinueTimeout(d time.Duration) Option {
	return core.WithExpectContinueTimeout(d)
}

func WithRoundTripperWrapper(wrap func(http.RoundTripper) http.RoundTripper) Option {
	return core.WithRoundTripperWrapper(wrap)
}

// 请求级选项。
func WithHeader(key, value string) RequestOption {
	return core.WithHeader(key, value)
}

func WithQuery(key, value string) RequestOption {
	return core.WithQuery(key, value)
}

func WithJSONBody(v any) RequestOption {
	return core.WithJSONBody(v)
}

func WithBytesBody(b []byte) RequestOption {
	return core.WithBytesBody(b)
}

func WithFormBody(v url.Values) RequestOption {
	return core.WithFormBody(v)
}

func WithBasicAuth(user, pass string) RequestOption {
	return core.WithBasicAuth(user, pass)
}

func WithBearer(token string) RequestOption {
	return core.WithBearer(token)
}

func WithUserAgent(ua string) RequestOption {
	return core.WithUserAgent(ua)
}

func WithMultipartFormData(fields map[string]string, files map[string]FileField) RequestOption {
	return core.WithMultipartFormData(fields, files)
}

func WithXMLBody(v any) RequestOption {
	return core.WithXMLBody(v)
}

func WithRequestTimeout(d time.Duration) RequestOption {
	return core.WithRequestTimeout(d)
}

func WithRequestID(id string) RequestOption {
	return core.WithRequestID(id)
}
