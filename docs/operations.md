# 运行手册

## 配置速查

| 类别 | 选项 | 默认 |
| --- | --- | --- |
| 协议 | WithProtocol | Auto(H1+H2 ALPN) |
| 整体超时 | WithTimeout | 0(不限) |
| Dial 超时 | WithDialTimeout | 10s |
| TLS 握手超时 | WithTLSHandshakeTimeout | 10s |
| 响应头超时 | WithResponseHeaderTimeout | 30s |
| 空闲连接 | WithMaxIdleConns / PerHost / IdleConnTimeout | 100 / 10 / 90s |
| 每主机连接 | WithMaxConnsPerHost | 不限 |
| 响应头大小 | WithMaxResponseHeaderBytes | 10MiB |
| Expect 100 | WithExpectContinueTimeout | 0(不等) |
| 重定向 | WithMaxRedirects / NoRedirect / Policy | 10 次跟随 |
| Cookie | WithCookieJar | 不维护 |
| 重试 | WithRetry / WithRetryPolicy | 关闭 |
| 重试上限 | RetryPolicy.TotalTimeout / MaxBackoff | 不限 |
| 并发 | WithMaxConcurrency | 不限 |
| DNS | WithDNSCache | 关闭 |
| H2 健康检查 | WithHTTP2HealthCheck | 关闭 |
| 压缩 | WithDisableCompression | false |
| 日志 | WithLogger / WithLogRequest / WithSlowThreshold | 关闭 / false / 100ms |
| 指标 | WithMetrics | no-op |

## 指标

注入 `Metrics` 后输出(标签统一为 method):

- `httpx.requests` 请求尝试计数
- `httpx.errors` 请求错误计数
- `httpx.retries` 重试计数
- `httpx.slow_requests` 慢请求计数
- `httpx.duration` 请求耗时观测(秒)

## 日志

- 开启 `WithLogRequest` 后每次尝试输出 Debug 级请求摘要;
- 请求失败输出 Warn 级 `HTTP 请求失败`;
- 超过慢阈值输出 Warn 级 `慢请求`;
- 携带 `X-Request-ID`(WithRequestID)时附加 `request_id` 字段。

## 注意事项

- `Client` 并发安全,可在多个 goroutine 间共享;
- 整体超时(客户端级或请求级)到期后,`Do` 返回的错误已标记超时,
  此时响应体读取可能因连接关闭失败,流式大响应请留足超时余量;
- `RetryPolicy.MaxBackoff` 会截断服务端返回的超大 `Retry-After`,
  防止长时间阻塞。

## 常见场景

### 常规 API 调用

```go
client, _ := httpx.New(
	httpx.WithTimeout(5*time.Second),
	httpx.WithRetry(3, httpx.ExponentialBackoff(100*time.Millisecond, 2, 0.2)),
)
```

### 高并发网关

```go
client, _ := httpx.New(
	httpx.WithMaxConcurrency(200),
	httpx.WithDNSCache(httpx.NewDNSCache(60*time.Second)),
	httpx.WithMaxIdleConnsPerHost(20),
)
```

### 内部服务(私有 CA + 会话)

```go
jar, _ := cookiejar.New(nil)
client, _ := httpx.New(
	httpx.WithTLSClientConfig(&tls.Config{RootCAs: pool}),
	httpx.WithCookieJar(jar),
)
```
