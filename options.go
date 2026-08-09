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
	defaultMaxIdleConns           = 100
	defaultMaxIdleConnsPerHost    = 10
	defaultIdleConnTimeout        = 90 * time.Second
	defaultDialTimeout            = 10 * time.Second
	defaultTLSHandshakeTimeout    = 10 * time.Second
	defaultResponseHeaderTimeout  = 30 * time.Second
	defaultSlowThreshold          = 100 * time.Millisecond
	defaultMaxRedirects           = 10
	defaultMaxResponseHeaderBytes = 10 << 20 // 10MiB,与 HTTP/2 默认一致
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
	protocol               Protocol
	timeout                time.Duration
	dialTimeout            time.Duration
	tlsHandshakeTimeout    time.Duration
	responseHeaderTimeout  time.Duration
	maxIdleConns           int
	maxIdleConnsPerHost    int
	idleConnTimeout        time.Duration
	tlsClientConfig        *tls.Config
	retry                  *retryPolicy
	logger                 logx.Logger
	logRequest             bool
	slowThreshold          time.Duration
	metrics                Metrics
	maxRedirects           int
	redirectPolicy         func(*http.Request, []*http.Request) error
	cookieJar              http.CookieJar
	hooks                  Hooks
	proxy                  func(*http.Request) (*url.URL, error)
	proxySet               bool
	disableCompression     bool
	dnsCache               *DNSCache
	maxConcurrency         int
	h2ReadIdleTimeout      time.Duration
	h2PingTimeout          time.Duration
	maxResponseHeaderBytes int64
	maxConnsPerHost        int
	expectContinueTimeout  time.Duration
	roundTripperWrapper    func(http.RoundTripper) http.RoundTripper
}

// defaultConfig 返回生产实践默认配置。
func defaultConfig() config {
	return config{
		protocol:               ProtocolAuto,
		dialTimeout:            defaultDialTimeout,
		tlsHandshakeTimeout:    defaultTLSHandshakeTimeout,
		responseHeaderTimeout:  defaultResponseHeaderTimeout,
		maxIdleConns:           defaultMaxIdleConns,
		maxIdleConnsPerHost:    defaultMaxIdleConnsPerHost,
		idleConnTimeout:        defaultIdleConnTimeout,
		slowThreshold:          defaultSlowThreshold,
		maxRedirects:           defaultMaxRedirects,
		maxResponseHeaderBytes: defaultMaxResponseHeaderBytes,
	}
}

// Hooks 是轻量请求钩子,全部可选,默认 no-op。
type Hooks struct {
	// OnRequest 在每次请求尝试前调用;返回错误将终止本次请求。
	OnRequest func(*http.Request) error
	// OnResponse 在每次响应后调用;返回错误将终止本次请求。
	OnResponse func(*http.Response) error
	// OnError 在每次请求错误后调用,仅用于观察。
	OnError func(error)
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

// RetryPolicy 描述完整重试行为,MaxAttempts 含首次尝试。
type RetryPolicy struct {
	// MaxAttempts 最大尝试次数(含首次)。
	MaxAttempts int
	// Backoff 退避策略。
	Backoff Backoff
	// Retryable 自定义可重试判定;nil 使用默认规则
	// (幂等方法 + 可重试错误 + 429/5xx 状态码)。
	Retryable func(*http.Request, *http.Response, error) bool
	// TotalTimeout 整体重试耗时上限,0 表示不限制。
	// 覆盖该请求从首次尝试到最终返回(含退避等待)。
	TotalTimeout time.Duration
	// MaxBackoff 单次退避等待的上限(含 Retry-After),0 表示不限制。
	// 防止服务端返回超大 Retry-After 时长时间阻塞。
	MaxBackoff time.Duration
}

// WithRetryPolicy 以完整策略开启重试,支持自定义可重试判定。
func WithRetryPolicy(p RetryPolicy) Option {
	return func(c *config) {
		c.retry = &retryPolicy{
			maxAttempts:  p.MaxAttempts,
			backoff:      p.Backoff,
			retryable:    p.Retryable,
			totalTimeout: p.TotalTimeout,
			maxBackoff:   p.MaxBackoff,
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

// WithMaxRedirects 设置最大重定向次数;0 表示不跟随重定向,
// 直接返回 3xx 响应;负数视为非法配置。
func WithMaxRedirects(n int) Option {
	return func(c *config) { c.maxRedirects = n }
}

// WithNoRedirect 关闭重定向跟随,直接返回 3xx 响应。
func WithNoRedirect() Option {
	return func(c *config) { c.maxRedirects = 0 }
}

// WithRedirectPolicy 设置自定义重定向策略:
// 在每次跳转前调用,参数为下一个请求与已访问请求列表;
// 返回错误将终止跟随。设置后覆盖次数上限语义。
func WithRedirectPolicy(policy func(*http.Request, []*http.Request) error) Option {
	return func(c *config) { c.redirectPolicy = policy }
}

// WithCookieJar 接入标准库 Cookie 会话:请求前自动注入、响应后自动保存。
// 未配置时行为与 http.DefaultClient 一致(不维护 Cookie)。
func WithCookieJar(jar http.CookieJar) Option {
	return func(c *config) { c.cookieJar = jar }
}

// WithHooks 注入轻量请求钩子(OnRequest / OnResponse / OnError)。
func WithHooks(h Hooks) Option {
	return func(c *config) { c.hooks = h }
}

// WithProxy 设置自定义代理;传入 nil 显式关闭代理
// (默认行为是读取环境变量 HTTP_PROXY / HTTPS_PROXY)。
func WithProxy(proxy func(*http.Request) (*url.URL, error)) Option {
	return func(c *config) {
		c.proxy = proxy
		c.proxySet = true
	}
}

// WithDisableCompression 关闭自动解压(gzip / br 等由 Transport 处理)。
func WithDisableCompression(disabled bool) Option {
	return func(c *config) { c.disableCompression = disabled }
}

// WithDNSCache 开启按 TTL 缓存的主机解析,减少 DNS 往返。
// cache 由 NewDNSCache 创建;解析失败或拨号全部失败时自动回退系统解析。
func WithDNSCache(cache *DNSCache) Option {
	return func(c *config) { c.dnsCache = cache }
}

// WithMaxConcurrency 限制客户端同时在途请求数;0 表示不限制。
// 超过上限的请求排队等待,等待受 context 取消约束。
func WithMaxConcurrency(n int) Option {
	return func(c *config) { c.maxConcurrency = n }
}

// WithHTTP2HealthCheck 启用强制 HTTP/2 连接的读空闲与 Ping 健康检查。
// 仅 ProtocolHTTP2 生效;任一参数为 0 表示对应检查关闭。
func WithHTTP2HealthCheck(readIdle, pingTimeout time.Duration) Option {
	return func(c *config) {
		c.h2ReadIdleTimeout = readIdle
		c.h2PingTimeout = pingTimeout
	}
}

// WithMaxResponseHeaderBytes 设置响应头大小上限;0 表示使用默认值(10MiB)。
func WithMaxResponseHeaderBytes(n int64) Option {
	return func(c *config) {
		if n == 0 {
			n = defaultMaxResponseHeaderBytes
		}
		c.maxResponseHeaderBytes = n
	}
}

// WithMaxConnsPerHost 设置每主机最大连接数;0 表示不限制。
// HTTP/2 单连接多路复用,该选项仅 H1 / Auto 生效。
func WithMaxConnsPerHost(n int) Option {
	return func(c *config) { c.maxConnsPerHost = n }
}

// WithExpectContinueTimeout 设置 Expect: 100-continue 的等待超时;
// 0 表示不等待。
func WithExpectContinueTimeout(d time.Duration) Option {
	return func(c *config) { c.expectContinueTimeout = d }
}

// WithRoundTripperWrapper 包装内部 RoundTripper（保留协议选择，
// 适合注入链路追踪、限流等传输中间层）。nil 包装器会被忽略。
func WithRoundTripperWrapper(wrap func(http.RoundTripper) http.RoundTripper) Option {
	return func(c *config) {
		if wrap != nil {
			c.roundTripperWrapper = wrap
		}
	}
}

// validateConfig 校验配置参数,负数超时/连接池参数与非法协议均视为非法。
func validateConfig(cfg config) error {
	if cfg.timeout < 0 {
		return errx.NewCode(CodeInvalidConfig, "整体超时不能为负数")
	}
	if cfg.dialTimeout < 0 {
		return errx.NewCode(CodeInvalidConfig, "DialTimeout 不能为负数")
	}
	if cfg.tlsHandshakeTimeout < 0 {
		return errx.NewCode(CodeInvalidConfig, "TLSHandshakeTimeout 不能为负数")
	}
	if cfg.responseHeaderTimeout < 0 {
		return errx.NewCode(CodeInvalidConfig, "ResponseHeaderTimeout 不能为负数")
	}
	if cfg.maxIdleConns < 0 {
		return errx.NewCode(CodeInvalidConfig, "MaxIdleConns 不能为负数")
	}
	if cfg.maxIdleConnsPerHost < 0 {
		return errx.NewCode(CodeInvalidConfig, "MaxIdleConnsPerHost 不能为负数")
	}
	if cfg.idleConnTimeout < 0 {
		return errx.NewCode(CodeInvalidConfig, "IdleConnTimeout 不能为负数")
	}
	if cfg.slowThreshold < 0 {
		return errx.NewCode(CodeInvalidConfig, "SlowThreshold 不能为负数")
	}
	if cfg.maxRedirects < 0 {
		return errx.NewCode(CodeInvalidConfig, "MaxRedirects 不能为负数")
	}
	if cfg.maxConcurrency < 0 {
		return errx.NewCode(CodeInvalidConfig, "MaxConcurrency 不能为负数")
	}
	if cfg.h2ReadIdleTimeout < 0 {
		return errx.NewCode(CodeInvalidConfig, "HTTP/2 读空闲超时不能为负数")
	}
	if cfg.h2PingTimeout < 0 {
		return errx.NewCode(CodeInvalidConfig, "HTTP/2 Ping 超时不能为负数")
	}
	if cfg.maxResponseHeaderBytes < 0 {
		return errx.NewCode(CodeInvalidConfig, "MaxResponseHeaderBytes 不能为负数")
	}
	if cfg.maxConnsPerHost < 0 {
		return errx.NewCode(CodeInvalidConfig, "MaxConnsPerHost 不能为负数")
	}
	if cfg.expectContinueTimeout < 0 {
		return errx.NewCode(CodeInvalidConfig, "ExpectContinueTimeout 不能为负数")
	}
	if cfg.protocol < ProtocolAuto || cfg.protocol > ProtocolHTTP3 {
		return errx.NewCodef(CodeInvalidConfig, "不支持的协议: %v", cfg.protocol)
	}
	if cfg.retry != nil {
		if cfg.retry.maxAttempts < 1 {
			return errx.NewCode(CodeInvalidConfig, "重试次数必须大于等于 1")
		}
		if cfg.retry.backoff == nil {
			return errx.NewCode(CodeInvalidConfig, "重试退避策略不能为空")
		}
		if cfg.retry.totalTimeout < 0 {
			return errx.NewCode(CodeInvalidConfig, "重试总时长不能为负数")
		}
		if cfg.retry.maxBackoff < 0 {
			return errx.NewCode(CodeInvalidConfig, "重试退避上限不能为负数")
		}
	}
	return nil
}

// RequestOption 修改单个请求的行为,在 Get / Post / Request 时应用。
type RequestOption func(*requestOptions)

// requestOptions 是单个请求的可变配置。
type requestOptions struct {
	header       http.Header
	query        url.Values
	bodyValue    any
	body         io.Reader
	contentType  string
	authUser     string
	authPass     string
	bearer       string
	userAgent    string
	xmlBody      any
	formFields   map[string]string
	formFiles    map[string]FileField
	multipartSet bool
	timeout      time.Duration
	requestID    string
}

// FileField 是 multipart 文件字段。
type FileField struct {
	// Filename 文件名(展示用)。
	Filename string
	// Content 文件内容。
	Content []byte
	// Reader 流式文件内容;非 nil 时优先于 Content,
	// 用于大文件避免整块载入内存。
	Reader io.Reader
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

// WithMultipartFormData 以 multipart/form-data 发送字段与文件,
// 自动生成带边界的 Content-Type。
func WithMultipartFormData(fields map[string]string, files map[string]FileField) RequestOption {
	return func(r *requestOptions) {
		r.formFields = fields
		r.formFiles = files
		r.multipartSet = true
	}
}

// WithXMLBody 以 XML 请求体发送,自动设置 Content-Type: application/xml。
func WithXMLBody(v any) RequestOption {
	return func(r *requestOptions) { r.xmlBody = v }
}

// WithRequestTimeout 设置请求级整体超时,覆盖该请求的完整生命周期(含重试与重定向)。
// 与客户端级超时同时存在时取更严格者。
func WithRequestTimeout(d time.Duration) RequestOption {
	return func(r *requestOptions) { r.timeout = d }
}

// WithRequestID 设置请求 ID,写入 X-Request-ID 请求头,
// 并随日志与错误结构化字段输出,便于链路排障。
func WithRequestID(id string) RequestOption {
	return func(r *requestOptions) { r.requestID = id }
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
			return nil, "", errx.WrapCode(err, CodeInvalidConfig, "请求体 JSON 序列化失败")
		}
		return bytes.NewReader(data), "application/json", nil
	}
}
