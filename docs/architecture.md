# httpx 架构设计

> 状态:已实现(v1.4.1),本文描述当前架构;公开 API 以 `go doc` 与 README 为准。
> 实现主体位于 `internal/core`,根包仅保留公开 API(类型别名 + 转发)。

## 1. 总体分层

```text
业务代码
├── httpx.Client(Do/Get/Post/Request + 请求选项)
├── 观测层(logx 日志 / Metrics 指标 / 重试)
├── RoundTripper 协议层(HTTP/1+2 stdlib / HTTP/3 quic-go)
└── net/http + quic-go/http3
```

## 2. 核心模块职责

| 模块 | 职责 |
| --- | --- |
| `client.go` | `Client`、`New`、`Do` / `Get` / `Post` / `Request` |
| `options.go` | `Option` / `RequestOption`、超时与连接池参数 |
| `transport.go` | 按协议构建 RoundTripper,连接池与 TLS 配置 |
| `retry.go` | 幂等重试、指数退避与抖动、可重读请求体 |
| `response.go` | 响应读取助手(字节 / 文本 / JSON,大小上限) |
| `errors.go` | `HTX_*` 错误码与判定助手 |
| `logadapter.go` | logx 日志、请求摘要、慢请求 |
| `metrics.go` | 指标钩子(与 dbx 同形态) |
| `http3/` | 可选子包:quic-go/http3 RoundTripper 接入 |
| `internal/core` | 全部实现与白盒测试;根包薄转发,保证公开 API 稳定 |

## 3. 协议层

- 默认 `ProtocolAuto`:`http.Transport` + `ForceAttemptHTTP2=true`,
  TLS `NextProtos` 由标准库协商,一份连接池同时服务 H1/H2;
- `ProtocolHTTP3`:使用 `httpx/http3` 子包构建 `http3.RoundTripper`,
  独立于 H1/H2 连接池;DSN/URL 无协议差异,`Do` 对上层透明;
- 协议选择在 `New` 时固定,运行期不可变(避免连接池错乱)。

## 4. 超时模型

四层超时,全部可配:

1. DialTimeout:建立 TCP 连接;
2. TLSHandshakeTimeout:TLS 握手;
3. ResponseHeaderTimeout:等待响应头;
4. 整体超时:以 context 实现,覆盖完整请求生命周期。

## 5. 重试模型

- `WithRetry(maxAttempts, policy)` 开启;默认关闭;
- 幂等判定:方法与错误分类(`IsRetryable`),5xx 与网络错误可重试,
  4xx 与 ctx 取消不重试;
- 退避:`base * 2^n + jitter`,受 `ctx` 取消约束;
- 请求体必须可重读(`bytes.Reader` / `io.ReadSeeker`),否则不重试直接报错。

## 6. 观测

- 每个请求统一 `observe(op, req, resp, err, start)`:
  计数(请求 / 错误 / 重试)、耗时观测、logx 请求摘要与慢请求日志;
- 日志与指标均为可选,外部注入,默认 no-op(零开销分支)。

## 7. 错误模型

- 所有对外错误为 errx,携带 `HTX_*` 错误码;
- 驱动/网络原始错误保留在错误链;`IsTimeout` / `IsRetryable` 基于
  errx Kind 与错误码判定。

## 8. 目标目录结构

```text
httpx/
├── README.md
├── CHANGELOG.md
├── go.mod             # module github.com/lcylpzls/httpx
├── api.go             # 根包薄转发(类型别名 + 公开函数)
├── internal/core/     # 全部实现与白盒测试
│   ├── client.go
│   ├── options.go
│   ├── transport.go
│   ├── retry.go
│   ├── response.go
│   ├── errors.go
│   ├── logadapter.go
│   └── metrics.go
├── http3/             # 可选:H3 RoundTripper(quic-go)
├── docs/
└── examples/
```

## 9. 依赖策略

- 核心包:标准库 + errx + logx + golang.org/x/net/http2
  (Go 官方扩展,仅 ProtocolHTTP2 使用,与标准库同源);
- `http3` 子包:quic-go 唯一第三方依赖,导入即获得 H3 能力;
- 禁止为核心功能引入额外第三方。
