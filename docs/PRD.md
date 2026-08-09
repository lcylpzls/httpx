# httpx 产品需求(PRD)

> 版本:v0.0.0(规划稿) · 状态:评审中

## 1. 背景与动机

自用项目里,每个服务都要调外部 HTTP 接口:

- 连接池、超时、重试、日志、指标各写各的,参数默认值随手拍;
- HTTP/3 支持需要额外引入 quic-go,接入成本高;
- 响应体读取与 JSON 解析重复且容易漏关 Body / 不设大小上限。

结论:**做一个薄的高性能 HTTP 客户端**,协议适配与公共能力沉淀为库能力,
业务代码只描述「调谁、带什么、要什么」。

## 2. 目标

1. 连接复用与空闲回收,默认参数贴合生产实践;
2. 分层超时(Dial / TLS / 响应头 / 整体),防止单点拖垮调用方;
3. 可选幂等重试,指数退避 + 抖动;
4. 协议可选:HTTP/1.1 / HTTP/2(标准库),HTTP/3(可选子包);
5. 与 errx / logx 打通,错误、日志、指标默认可观测;
6. 响应读取助手带大小上限,防内存被打爆。

## 3. 非目标(明确不做)

- 不自研 HTTP 协议解析(复用 net/http 与 quic-go/http3);
- 不做服务端(webx 已覆盖);
- 不做服务发现、负载均衡、熔断器(留给上层);
- 不提供全局单例魔法,显式创建 Client;
- 不追求与第三方 HTTP 库 API 兼容。

## 4. 能力需求

### 4.1 客户端核心

- `New(opts ...Option) (*Client, error)`;
- `Do(ctx, *http.Request) (*http.Response, error)` 与 `Get` / `Post` / `Request`;
- 连接池:MaxIdleConns、MaxIdleConnsPerHost、IdleConnTimeout、
  TLSHandshakeTimeout、DialTimeout、ResponseHeaderTimeout、整体超时。

### 4.2 协议适配

- `ProtocolAuto`(默认):HTTP/1.1 + HTTP/2 自动协商;
- `ProtocolHTTP1` / `ProtocolHTTP2`:强制协议;
- `ProtocolHTTP3`:HTTP/3(QUIC),需导入 `httpx/http3` 子包并显式选择。

### 4.3 重试

- 默认关闭,`WithRetry(max, backoff)` 显式开启;
- 仅重试幂等方法(GET / HEAD / OPTIONS / PUT / DELETE 等)与可重试错误;
- 指数退避 + 随机抖动,受 ctx 取消约束;重试需可重读请求体。

### 4.4 观测

- 请求/错误/重试计数与耗时观测(同 dbx 的 Metrics 接口);
- 日志走外部注入的 logx,支持请求摘要与慢请求日志;
- 支持请求级标签(如 service / path)。

### 4.5 响应助手

- `ReadBody` / `ReadString` / `JSON`,统一关闭 Body 并支持大小上限;
- 超限返回 `HTX_BODY_TOO_LARGE`。

### 4.6 错误模型

- 对外错误统一 errx,错误码 `HTX_*`;
- 保留原始错误链;`IsTimeout` / `IsRetryable` 判定助手。

## 5. 非功能需求

- **性能**:热路径(Do)分配与裸 `net/http` 同量级;基准目标见迭代计划;
- **资源**:连接空闲自动回收,响应体读取设上限,默认压缩关闭;
- **质量**:语句覆盖率 100%、race、staticcheck、vet、fuzz、三平台 CI。

## 6. 验收标准

v0.1.0 发布时:

1. HTTP/1.1 与 HTTP/2 有本地集成测试(httptest 服务器);
2. HTTP/3 在 CI 用本地 QUIC 服务器验证;
3. 重试、超时、大小上限、错误码全路径测试;
4. 100% 语句覆盖率,`go test -race ./...`、staticcheck、go vet 全绿;
5. 性能基准与裸 `net/http` 对比基线写入文档。
