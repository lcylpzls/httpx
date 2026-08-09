package httpx

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lcylpzls/errx"
	"github.com/lcylpzls/logx"
)

// 生产实践默认值:连接池与超时参数在未显式配置时使用。
const (
	defaultMaxIdleConns          = 100
	defaultMaxIdleConnsPerHost   = 10
	defaultIdleConnTimeout       = 90 * time.Second
	defaultDialTimeout           = 10 * time.Second
	defaultTLSHandshakeTimeout   = 10 * time.Second
	defaultResponseHeaderTimeout = 30 * time.Second
	defaultSlowThreshold         = 100 * time.Millisecond
)

// Protocol 表示 HTTP 协议选择,在 New 时固定,运行期不可变。
type Protocol int

const (
	// ProtocolAuto 默认:HTTP/1.1 + HTTP/2 经 TLS ALPN 自动协商。
	ProtocolAuto Protocol = iota
	// ProtocolHTTP1 强制 HTTP/1.1(禁用 ALPN 协商 HTTP/2)。
	ProtocolHTTP1
	// ProtocolHTTP2 强制 HTTP/2(TLS 之上,使用官方 x/net/http2)。
	ProtocolHTTP2
	// ProtocolHTTP3 强制 HTTP/3(QUIC),需先导入 httpx/http3 子包完成注册。
	ProtocolHTTP3
)

// String 返回协议的稳定名称,用于日志与错误信息。
func (p Protocol) String() string {
	switch p {
	case ProtocolAuto:
		return "auto"
	case ProtocolHTTP1:
		return "http/1.1"
	case ProtocolHTTP2:
		return "http/2"
	case ProtocolHTTP3:
		return "http/3"
	default:
		return "unknown"
	}
}

// config 是 Client 的完整配置,由 Option 按顺序修改。
type config struct {
	protocol              Protocol
	timeout               time.Duration
	dialTimeout           time.Duration
	tlsHandshakeTimeout   time.Duration
	responseHeaderTimeout time.Duration
	maxIdleConns          int
	maxIdleConnsPerHost   int
	idleConnTimeout       time.Duration
	tlsClientConfig       *tls.Config
	retry                 *retryPolicy
	logger                logx.Logger
	logRequest            bool
	slowThreshold         time.Duration
	metrics               Metrics
}

// defaultConfig 返回生产实践默认配置。
func defaultConfig() config {
	return config{
		protocol:              ProtocolAuto,
		dialTimeout:           defaultDialTimeout,
		tlsHandshakeTimeout:   defaultTLSHandshakeTimeout,
		responseHeaderTimeout: defaultResponseHeaderTimeout,
		maxIdleConns:          defaultMaxIdleConns,
		maxIdleConnsPerHost:   defaultMaxIdleConnsPerHost,
		idleConnTimeout:       defaultIdleConnTimeout,
		slowThreshold:         defaultSlowThreshold,
	}
}

// Option 修改 Client 配置,在 New 时按顺序应用。
type Option func(*config)

// WithTimeout 设置整体超时(context 实现,覆盖完整请求生命周期与重试)。
// 0 表示不限制;生产环境建议显式设置。
func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// WithDialTimeout 设置建立 TCP 连接的超时。
func WithDialTimeout(d time.Duration) Option {
	return func(c *config) { c.dialTimeout = d }
}

// WithTLSHandshakeTimeout 设置 TLS 握手超时。
func WithTLSHandshakeTimeout(d time.Duration) Option {
	return func(c *config) { c.tlsHandshakeTimeout = d }
}

// WithResponseHeaderTimeout 设置等待响应头的超时。
func WithResponseHeaderTimeout(d time.Duration) Option {
	return func(c *config) { c.responseHeaderTimeout = d }
}

// WithMaxIdleConns 设置连接池最大空闲连接数。
func WithMaxIdleConns(n int) Option {
	return func(c *config) { c.maxIdleConns = n }
}

// WithMaxIdleConnsPerHost 设置每主机最大空闲连接数。
func WithMaxIdleConnsPerHost(n int) Option {
	return func(c *config) { c.maxIdleConnsPerHost = n }
}

// WithIdleConnTimeout 设置空闲连接回收时间。
func WithIdleConnTimeout(d time.Duration) Option {
	return func(c *config) { c.idleConnTimeout = d }
}

// WithTLSClientConfig 设置客户端 TLS 配置(私有 CA、客户端证书等)。
// 内部会克隆配置,不共享外部可变状态。
func WithTLSClientConfig(cfg *tls.Config) Option {
	return func(c *config) { c.tlsClientConfig = cfg }
}

// WithProtocol 设置协议:Auto(默认)/ HTTP1 / HTTP2 / HTTP3。
func WithProtocol(p Protocol) Option {
	return func(c *config) { c.protocol = p }
}

// WithRetry 显式开启重试:最多 maxAttempts 次尝试(含首次),
// 退避策略由 backoff 决定。默认关闭。
func WithRetry(maxAttempts int, backoff Backoff) Option {
	return func(c *config) {
		c.retry = &retryPolicy{
			maxAttempts: maxAttempts,
			backoff:     backoff,
		}
	}
}

// WithLogger 注入结构化日志实现,空表示关闭日志(默认)。
func WithLogger(l logx.Logger) Option {
	return func(c *config) { c.logger = l }
}

// WithLogRequest 设置是否以 Debug 级别打印每次请求摘要。
func WithLogRequest(enabled bool) Option {
	return func(c *config) { c.logRequest = enabled }
}

// WithSlowThreshold 设置慢请求阈值,0 表示使用默认值(100ms)。
func WithSlowThreshold(d time.Duration) Option {
	return func(c *config) { c.slowThreshold = d }
}

// WithMetrics 注入指标钩子,空表示关闭指标(默认)。
func WithMetrics(m Metrics) Option {
	return func(c *config) { c.metrics = m }
}

// validateConfig 校验配置参数,负数超时/连接池参数与非法协议均视为非法。
func validateConfig(cfg config) error {
	if cfg.timeout < 0 {
		return errx.New(errx.KindInvalid, CodeInvalidConfig, "整体超时不能为负数")
	}
	if cfg.dialTimeout < 0 {
		return errx.New(errx.KindInvalid, CodeInvalidConfig, "DialTimeout 不能为负数")
	}
	if cfg.tlsHandshakeTimeout < 0 {
		return errx.New(errx.KindInvalid, CodeInvalidConfig, "TLSHandshakeTimeout 不能为负数")
	}
	if cfg.responseHeaderTimeout < 0 {
		return errx.New(errx.KindInvalid, CodeInvalidConfig, "ResponseHeaderTimeout 不能为负数")
	}
	if cfg.maxIdleConns < 0 {
		return errx.New(errx.KindInvalid, CodeInvalidConfig, "MaxIdleConns 不能为负数")
	}
	if cfg.maxIdleConnsPerHost < 0 {
		return errx.New(errx.KindInvalid, CodeInvalidConfig, "MaxIdleConnsPerHost 不能为负数")
	}
	if cfg.idleConnTimeout < 0 {
		return errx.New(errx.KindInvalid, CodeInvalidConfig, "IdleConnTimeout 不能为负数")
	}
	if cfg.slowThreshold < 0 {
		return errx.New(errx.KindInvalid, CodeInvalidConfig, "SlowThreshold 不能为负数")
	}
	if cfg.protocol < ProtocolAuto || cfg.protocol > ProtocolHTTP3 {
		return errx.Newf(errx.KindInvalid, CodeInvalidConfig, "不支持的协议: %v", cfg.protocol)
	}
	if cfg.retry != nil {
		if cfg.retry.maxAttempts < 1 {
			return errx.New(errx.KindInvalid, CodeInvalidConfig, "重试次数必须大于等于 1")
		}
		if cfg.retry.backoff == nil {
			return errx.New(errx.KindInvalid, CodeInvalidConfig, "重试退避策略不能为空")
		}
	}
	return nil
}

// RequestOption 修改单个请求的行为,在 Get / Post / Request 时应用。
type RequestOption func(*requestOptions)

// requestOptions 是单个请求的可变配置。
type requestOptions struct {
	header      http.Header
	query       url.Values
	bodyValue   any
	body        io.Reader
	contentType string
	authUser    string
	authPass    string
	bearer      string
	userAgent   string
}

// WithHeader 设置请求头(同名多次调用时后者覆盖前者)。
func WithHeader(key, value string) RequestOption {
	return func(r *requestOptions) {
		if r.header == nil {
			r.header = make(http.Header)
		}
		r.header.Set(key, value)
	}
}

// WithQuery 追加查询参数,与 URL 自带参数合并。
func WithQuery(key, value string) RequestOption {
	return func(r *requestOptions) {
		if r.query == nil {
			r.query = make(url.Values)
		}
		r.query.Add(key, value)
	}
}

// WithJSONBody 以 JSON 请求体发送,序列化延迟到请求构建时执行,
// 自动设置 Content-Type: application/json。
func WithJSONBody(v any) RequestOption {
	return func(r *requestOptions) { r.bodyValue = v }
}

// WithBytesBody 以字节切片作为请求体(可重读,支持重试)。
func WithBytesBody(b []byte) RequestOption {
	return func(r *requestOptions) { r.body = bytes.NewReader(b) }
}

// WithFormBody 以表单编码发送 url.Values,
// 自动设置 Content-Type: application/x-www-form-urlencoded。
func WithFormBody(v url.Values) RequestOption {
	return func(r *requestOptions) {
		r.body = strings.NewReader(v.Encode())
		r.contentType = "application/x-www-form-urlencoded"
	}
}

// WithBasicAuth 设置 HTTP Basic 认证。
func WithBasicAuth(user, pass string) RequestOption {
	return func(r *requestOptions) {
		r.authUser = user
		r.authPass = pass
	}
}

// WithBearer 设置 Bearer Token 认证(Authorization: Bearer <token>)。
func WithBearer(token string) RequestOption {
	return func(r *requestOptions) { r.bearer = token }
}

// WithUserAgent 设置 User-Agent 请求头。
func WithUserAgent(ua string) RequestOption {
	return func(r *requestOptions) { r.userAgent = ua }
}

// bodyToReader 将 Post 的 body 参数转换为可读请求体:
// nil / io.Reader / string / []byte / url.Values 原样处理,
// 其余类型按 JSON 序列化。
func bodyToReader(body any) (io.Reader, string, error) {
	switch b := body.(type) {
	case nil:
		return nil, "", nil
	case io.Reader:
		return b, "", nil
	case string:
		return strings.NewReader(b), "", nil
	case []byte:
		return bytes.NewReader(b), "", nil
	case url.Values:
		return strings.NewReader(b.Encode()), "application/x-www-form-urlencoded", nil
	default:
		data, err := json.Marshal(b)
		if err != nil {
			return nil, "", errx.Wrap(err, errx.KindInvalid, CodeInvalidConfig, "请求体 JSON 序列化失败")
		}
		return bytes.NewReader(data), "application/json", nil
	}
}
