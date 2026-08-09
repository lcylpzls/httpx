# httpx API 设计草案

> 状态:**已冻结 v0.1.0 API**。D1–D6 已于 2026-08-09 全部按推荐确认。

## 1. 快速上手

```go
client, err := httpx.New(
	httpx.WithTimeout(5*time.Second),
	httpx.WithRetry(3, httpx.ExponentialBackoff(100*time.Millisecond, 2, 0.2)),
	httpx.WithLogger(logger),
)

resp, err := client.Get(ctx, "https://api.example.com/users/1",
	httpx.WithHeader("X-Project", "demo"))
if err != nil {
	return err
}
var user User
if err := httpx.JSON(resp, &user); err != nil {
	return err
}
```

## 2. 核心类型

```go
type Client struct { /* 未导出 */ }

func New(opts ...Option) (*Client, error)

func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error)
func (c *Client) Get(ctx context.Context, url string, opts ...RequestOption) (*http.Response, error)
func (c *Client) Post(ctx context.Context, url string, body any, opts ...RequestOption) (*http.Response, error)
func (c *Client) Request(ctx context.Context, method, url string, opts ...RequestOption) (*http.Response, error)
func (c *Client) CloseIdleConnections()
```

`Do` 直接返回 `*http.Response`,与 `net/http` 兼容;Body 由调用方或
响应助手负责关闭;`CloseIdleConnections` 释放连接池空闲连接,
HTTP/3 子包等价于关闭全部空闲 QUIC 连接。

## 3. 配置选项(Option)

```go
// 超时:四层全可配,Dial/TLS/响应头有生产默认值,
// 整体超时 WithTimeout 默认 0(不限制),建议生产显式设置。
func WithTimeout(d time.Duration) Option
func WithDialTimeout(d time.Duration) Option
func WithTLSHandshakeTimeout(d time.Duration) Option
func WithResponseHeaderTimeout(d time.Duration) Option
func WithTLSClientConfig(cfg *tls.Config) Option

// 连接池
func WithMaxIdleConns(n int) Option
func WithMaxIdleConnsPerHost(n int) Option
func WithIdleConnTimeout(d time.Duration) Option

// 协议与重试、观测
func WithProtocol(p Protocol) Option
func WithRetry(maxAttempts int, backoff Backoff) Option
func WithLogger(l logx.Logger) Option
func WithMetrics(m Metrics) Option

// 重定向与会话(v0.2.0)
func WithMaxRedirects(n int) Option
func WithNoRedirect() Option
func WithRedirectPolicy(policy func(*http.Request, []*http.Request) error) Option
func WithCookieJar(jar http.CookieJar) Option
func WithHooks(h Hooks) Option

// 传输控制(v0.2.0)
func WithProxy(proxy func(*http.Request) (*url.URL, error)) Option
func WithDisableCompression(disabled bool) Option

// 性能与流控(v0.3.0)
func WithRetryPolicy(p RetryPolicy) Option
func WithDNSCache(cache *DNSCache) Option
func WithMaxConcurrency(n int) Option
func WithHTTP2HealthCheck(readIdle, pingTimeout time.Duration) Option
```

默认值:MaxIdleConns=100、MaxIdleConnsPerHost=10、
IdleConnTimeout=90s、DialTimeout=10s、TLSHandshakeTimeout=10s、
ResponseHeaderTimeout=30s、MaxRedirects=10;慢请求阈值默认 100ms;
默认使用环境代理(HTTP_PROXY / HTTPS_PROXY)。

```go
// Hooks 是轻量请求钩子,全部可选,默认 no-op。
type Hooks struct {
	OnRequest  func(*http.Request) error
	OnResponse func(*http.Response) error
	OnError    func(error)
}
```

## 4. 协议

```go
type Protocol int

const (
	ProtocolAuto Protocol = iota // HTTP/1.1 + HTTP/2 自动协商(默认)
	ProtocolHTTP1
	ProtocolHTTP2
	ProtocolHTTP3 // 需导入 httpx/http3 子包
)

func WithProtocol(p Protocol) Option
```

HTTP/3 注册机制(仅子包使用):

```go
// ProtocolConfig 是注册协议构造器时可用的连接配置。
type ProtocolConfig struct {
	DialTimeout        time.Duration
	TLSClientConfig    *tls.Config
	DisableCompression bool
}

// RegisterHTTP3 注册 HTTP/3 RoundTripper 构造器,由 httpx/http3 子包 init 调用。
func RegisterHTTP3(builder func(ProtocolConfig) (http.RoundTripper, error))
```

## 5. 请求选项

```go
func WithHeader(key, value string) RequestOption
func WithQuery(key, value string) RequestOption
func WithJSONBody(v any) RequestOption
func WithFormBody(v url.Values) RequestOption
func WithBytesBody(b []byte) RequestOption
func WithBasicAuth(user, pass string) RequestOption
func WithBearer(token string) RequestOption
func WithUserAgent(ua string) RequestOption
func WithMultipartFormData(fields map[string]string, files map[string]FileField) RequestOption
func WithXMLBody(v any) RequestOption
func WithRequestTimeout(d time.Duration) RequestOption
```

`Post(ctx, url, body any, opts...)` 的 body 规则:
`nil` / `io.Reader` / `string` / `[]byte` / `url.Values` 原样处理,
其余类型按 JSON 序列化并自动设置 Content-Type。

```go
// FileField 是 multipart 文件字段。
type FileField struct {
	Filename string
	Content  []byte
}
```

## 6. 重试

```go
type Backoff func(attempt int) time.Duration
func ExponentialBackoff(base time.Duration, factor float64, jitter float64) Backoff
func FixedBackoff(interval time.Duration) Backoff

func WithRetry(maxAttempts int, backoff Backoff) Option
```

```go
// RetryPolicy 是完整重试策略,Retryable 为空时使用默认规则。
type RetryPolicy struct {
	MaxAttempts int
	Backoff     Backoff
	Retryable   func(*http.Request, *http.Response, error) bool
}

func WithRetryPolicy(p RetryPolicy) Option
```

重试规则:默认关闭;开启后仅幂等方法
(GET / HEAD / OPTIONS / PUT / DELETE)与可重试错误参与重试;
状态码 429 / 500 / 502 / 503 / 504 可重试;
尊重 `Retry-After`(优先于退避策略);请求体必须可重读。

## 7. 观测与响应助手

```go
type Metrics interface {
	IncCounter(name string, labels ...string)
	ObserveDuration(name string, seconds float64, labels ...string)
}

func WithLogger(l logx.Logger) Option
func WithMetrics(m Metrics) Option

func ReadBody(resp *http.Response, maxBytes int64) ([]byte, error)
func ReadString(resp *http.Response, maxBytes int64) (string, error)
func JSON(resp *http.Response, out any) error
func ReadFile(resp *http.Response, path string, maxBytes int64) error
func ReadStream(resp *http.Response, fn func([]byte) error, maxBytes int64) error
```

`JSON` 内置 16MiB 大小上限,防止内存被打爆。

## 7.5 运行统计

```go
type Stats struct {
	TotalRequests  uint64 // 累计请求尝试次数
	ActiveRequests uint64 // 当前活跃请求数
	TotalErrors    uint64 // 累计请求错误次数
	Retries        uint64 // 累计重试次数
}

func (c *Client) Stats() Stats
```

```go
// DNSCache 是按 TTL 缓存主机解析的线程安全解析器。
func NewDNSCache(ttl time.Duration) *DNSCache
func (c *DNSCache) Reset()
```

## 8. 错误码

| 错误码 | 含义 |
| --- | --- |
| `HTX_INVALID_CONFIG` | 配置非法 |
| `HTX_UNSUPPORTED_PROTOCOL` | 协议未注册(如未导入 http3 子包) |
| `HTX_DIAL_FAILED` | 建立连接失败 |
| `HTX_TLS_FAILED` | TLS 握手失败 |
| `HTX_REQUEST_FAILED` | 请求发送失败 |
| `HTX_RESPONSE_FAILED` | 读取响应失败 |
| `HTX_RETRY_EXHAUSTED` | 重试耗尽 |
| `HTX_BODY_TOO_LARGE` | 响应体超过上限 |
| `HTX_BODY_UNREADABLE` | 请求体不可重读(无法重试) |

```go
func IsTimeout(err error) bool
func IsRetryable(err error) bool
```

## 9. 已确认决策

| 编号 | 决策 | 结论 |
| --- | --- | --- |
| D1 | HTTP/3 依赖策略 | 可选子包 `httpx/http3`(quic-go),核心零重依赖 |
| D2 | API 形态 | `Do` 返回 `*http.Response`,响应助手为包级函数 |
| D3 | 重试默认值 | 默认关闭,`WithRetry` 显式开启 |
| D4 | 协议默认 | `ProtocolAuto`(H1+H2 ALPN 自动协商) |
| D5 | 观测形态 | logx 外部注入 + Metrics 接口(同 dbx,默认 no-op) |
| D6 | 整体超时实现 | context 控制,四层超时全可配 |
