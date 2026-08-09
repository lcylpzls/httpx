# 热门 HTTP 客户端调研手册

> 调研日期:2026-08-09 · 范围:Go / Java / Python / Rust / Node / curl
> 目的:只取设计思想,代码全部自研。

## 1. Go 生态

### 1.1 net/http(标准库)

**设计思想**

- `RoundTripper` 接口抽象:传输层可插拔,协议实现(HTTP/1、HTTP/2、HTTP/3)
  都收敛到同一个执行入口;
- `Transport` 连接池:keep-alive、每主机空闲连接上限
  (`MaxIdleConnsPerHost`)、空闲回收(`IdleConnTimeout`)、
  最大空闲连接(`MaxIdleConns`);
- `Dialer`:拨号超时、TCP keepalive、Happy Eyeballs(双栈并行);
- HTTP/2 自动协商:`ForceAttemptHTTP2` + TLS ALPN,一份连接池同时服务 H1/H2;
- 错误语义:`net.Error` 的 `Timeout()` / `Temporary()` 为超时/重试分类打底。

**取其精华**:RoundTripper 抽象、连接池参数模型、错误语义接口。

**去其糟粕**:默认 `ResponseHeaderTimeout=0` 等保守默认;全局
`http.DefaultClient` 共享 Transport 容易被误用。

### 1.2 fasthttp

**设计思想**

- 零分配目标:请求/响应对象进 `sync.Pool` 复用,GC 压力最小化;
- 每连接 worker 与连接池 per-host;DNS 缓存;响应头/体分离处理;
- 自研极简 HTTP 解析,追求极致吞吐。

**取其精华**:对象复用、连接池 per-host、资源敏感的热路径态度。

**去其糟粕**:不兼容 `net/http` API(自造 `RequestCtx`);
字节复用导致响应体生命周期陷阱(读完即失效);自研协议带来安全与
维护成本;性能收益主要在高并发服务端,客户端场景用 `net/http` 已足够。

### 1.3 resty

**设计思想**

- 链式 API + 请求/客户端两级配置;
- 中间件钩子:`OnBeforeRequest` / `OnAfterResponse` / `OnError`;
- 可配置重试:重试条件、退避策略、`Retry-After` 尊重、默认仅幂等方法;
- JSON / 表单自动序列化、debug 日志、trace 集成。

**取其精华**:客户端级默认 + 请求级覆盖;条件化重试;钩子式扩展。

**去其糟粕**:魔法式隐式转换;响应包装增加分配;功能面过大;
全局单例便利但状态污染风险高。

### 1.4 go-retryablehttp(HashiCorp)

**设计思想**

- 薄封装 `net/http`,API 熟悉;
- 默认 5xx 重试(除 501),指数退避 + 可解析 `Retry-After`(429/503);
- 日志钩子(`LeveledLogger`)输出每次尝试;
- 可重读请求体处理;错误分类(网络错误 vs 响应错误)。

**取其精华**:退避 + `Retry-After`、尝试级日志、薄封装。

**去其糟粕**:默认重试所有 5xx 需要调用方留意;部分默认值偏激进。

### 1.5 heimdall

**设计思想**:重试 + 熔断器(hystrix 风格)组合,可插拔退避策略。

**取其精华**:重试与容错的组合意识。

**去其糟粕**:熔断器属于上层治理职责,放进客户端过重;库活跃度低。

### 1.6 quic-go/http3

**设计思想**

- 实现 `http.RoundTripper`,与 `net/http.Client` 无缝配合;
- QUIC 连接池、多路复用、连接迁移、0-RTT 能力。

**取其精华**:以 RoundTripper 接入,协议对上层透明。

**去其糟粕**:依赖重、QUIC/TLS 细节多,必须隔离到可选子包,不能进核心。

## 2. Java 生态

### 2.1 OkHttp

**设计思想**

- 连接池:复用 + 空闲回收 + 每主机上限,HTTP/2 单连接多路复用;
- 超时分层:connect / read / write / call;
- **拦截器链**(应用拦截器 + 网络拦截器):中间件范式的标杆;
- `EventListener`:按阶段埋点(连接、TLS、请求、响应、耗时);
- 连接失败自动重试(幂等语义),尊重 `Retry-After`。

**取其精华**:拦截器链、阶段埋点、超时分层、连接池管理。

**去其糟粕**:拦截器易被滥用做重活;API 面较大。

## 3. Python 生态

### 3.1 httpx

**设计思想**

- **严格默认超时(5s)**,超时模型分 pool / connect / read / write;
- 连接池 `Limits`(最大保持连接、每主机上限);
- HTTP/1.1 + HTTP/2 可选;同步/异步双 API;
- Transport 抽象 + extensions 机制(请求级超时随请求传递)。

**取其精华**:默认超时、分层超时模型、连接池上限、请求级配置传递。

**去其糟粕**:同步/异步双 API 维护成本高;HTTP/3 依赖第三方。

### 3.2 requests

**设计思想**:Session / Adapter 抽象(连接复用 + 每主机适配器)、
Cookie / 重定向处理、流式响应。

**取其精华**:Session 级连接复用的心智模型。

**去其糟粕**:默认无超时(经典教训);重试默认关闭;全局 Session 状态污染。

## 4. Rust 生态

### 4.1 reqwest

**设计思想**

- 基于 **tower 中间件架构**:重试、超时、限流可组合;
- 连接池默认开启:`pool_max_idle_per_host` / `pool_idle_timeout`;
- HTTP/2 / HTTP/3(经 quinn)支持;
- 错误链区分:构建错误、重定向错误、请求错误、响应错误;
- 自动 gzip / br 解压。

**取其精华**:中间件可组合、连接池默认值、错误链分阶段。

**去其糟粕**:中间件需额外 crate;特性开关面大;绑定 tokio 异步生态。

## 5. Node 生态

### 5.1 undici

**设计思想**

- Dispatcher 分层:`Client`(单连接)→ `Pool`(连接池)→ `Agent`(per-origin);
- HTTP/2 单连接多路复用,连接内并发上限可调;
- HTTP/1.1 pipelining;`keepAliveMaxTimeout` 控制连接存活;
- interceptors 可组合;`client.stats` 暴露池状态。

**取其精华**:调度器分层、连接并发上限、可观测的连接池状态。

**去其糟粕**:低层 API 复杂;默认参数(如 headers 大小)需调优。

## 6. curl

**设计思想**:连接复用;`--retry` / `--retry-all-errors`;
超时族(`--connect-timeout` / `--max-time`);协议可选项
(`--http2` / `--http3`)。

**取其精华**:协议可选项与超时族的心智模型。

**去其糟粕**:命令行语义,库化时需取舍(不做全量参数面)。

## 7. 设计思想汇总(精华清单)

### 7.1 连接与资源

- 连接池 per-host,keep-alive 复用;
- 空闲连接回收(`IdleConnTimeout`)与每主机上限;
- HTTP/2 单连接多路复用;HTTP/3 同 QUIC 多路复用;
- 响应体大小上限,Body 生命周期统一管理。

### 7.2 超时

- 分层超时:pool / connect / TLS / 响应头 / read / write / 整体;
- **默认严格超时**(吸取 requests 无超时的教训)。

### 7.3 重试

- 默认关闭或严格幂等白名单;
- 指数退避 + 随机抖动;尊重 `Retry-After`(429 / 503);
- 网络错误与 5xx 分类;ctx 取消优先。

### 7.4 架构

- `RoundTripper` 接口抽象,H1/H2 共用连接池,H3 独立可选;
- 轻量钩子(请求前 / 请求后 / 错误)而非重型中间件框架;
- 客户端级默认 + 请求级覆盖;显式 `New`,不用全局单例。

### 7.5 观测

- 阶段埋点(连接、TLS、请求、响应);尝试级日志;
- 日志与指标外部注入,默认 no-op。

### 7.6 错误

- 错误链区分阶段(构建 / 网络 / 响应),结构化错误保留原始链;
- 超时与可重试判定助手。

## 8. 明确不采纳的糟粕

- fasthttp 式自研协议与字节复用陷阱;
- resty 式魔法隐式转换与全局单例;
- heimdall 式把熔断器内置进客户端;
- requests 式默认无超时;
- 默认重试非幂等方法的策略;
- tower 式重型中间件框架(轻量钩子即可);
- 大而全的特性开关面。

## 9. 对 httpx 的设计映射

| 精华思想 | httpx 落点 |
| --- | --- |
| RoundTripper 抽象、协议可插拔 | `transport.go` + `http3/` 可选子包 |
| 连接池 per-host + 空闲回收 | `WithMaxIdleConnsPerHost` / `WithIdleConnTimeout` |
| 分层超时 + 默认严格 | 四层超时全可配,默认 Dial/TLS/Header 均有值 |
| 重试:幂等 + 退避 + Retry-After | 默认关闭,`WithRetry` 显式开启 |
| 轻量钩子与阶段埋点 | logx 请求摘要 + Metrics 计数/耗时 |
| 错误链分阶段 | `HTX_DIAL_FAILED` / `HTX_REQUEST_FAILED` / `HTX_RESPONSE_FAILED` 等 |
| 客户端级默认 + 请求级覆盖 | `Option` / `RequestOption` 两级 |
| 响应体大小上限 | `ReadBody` / `ReadString` / `JSON` 助手 |

> 本手册为 httpx 设计输入,不构成对外承诺;实现时按需取舍。
