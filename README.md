# httpx

基于 `net/http` 的高性能 HTTP 客户端库:连接复用、分层超时、可选重试,
可适配 HTTP/1 / HTTP/2 / HTTP/3,与 errx / logx 打通。

> 当前状态:**v0.5.0 实现完成,待 CI 验证与发布**。

## 定位

httpx **不是新协议实现**,不解决「HTTP 怎么走」的问题;它解决的是项目里每个
HTTP 调用方都要重复的部分:

- 连接池复用、空闲回收与分层超时;
- HTTP/1 / HTTP/2 / HTTP/3 协议可选切换;
- 幂等重试与指数退避;
- 与 errx / logx 打通的错误、日志与指标;
- 响应读取助手(JSON / 文本 / 字节,带大小上限)。

## 快速上手

```go
client, err := httpx.New(
	httpx.WithTimeout(5*time.Second),
	httpx.WithRetry(3, httpx.ExponentialBackoff(100*time.Millisecond, 2, 0.2)),
	httpx.WithLogger(logger),
)
if err != nil {
	panic(err)
}

resp, err := client.Get(ctx, "https://api.example.com/users/1",
	httpx.WithHeader("X-Project", "demo"))
if err != nil {
	panic(err)
}
var user User
if err := httpx.JSON(resp, &user); err != nil {
	panic(err)
}
```

HTTP/3 使用可选子包,导入即注册:

```go
import (
	"github.com/lcylpzls/httpx"
	_ "github.com/lcylpzls/httpx/http3" // 注册 HTTP/3 传输层
)

client, err := httpx.New(httpx.WithProtocol(httpx.ProtocolHTTP3))
```

## 特性

- 连接池:per-host 复用、空闲回收、每主机上限,默认贴合生产实践;
- 四层超时:Dial / TLS / 响应头 / 整体(context),全部可配;
- 协议:HTTP/1.1、HTTP/2(自动协商或强制)、HTTP/3(可选子包);
- 重试:默认关闭,显式开启后仅幂等方法,指数退避 + 抖动 + Retry-After;
- 观测:logx 外部注入 + Metrics 接口,默认 no-op 零开销;
- 错误:统一 errx,HTX_* 错误码,IsTimeout / IsRetryable 判定助手;
- 响应助手:ReadBody / ReadString / JSON / ReadFile,统一关闭 Body 并设大小上限;
- 重定向:默认跟随上限 10,方法转换与跨域敏感头剥离,可关闭/自定义策略;
- 会话:标准库 CookieJar 自动注入与保存;
- 钩子:OnRequest / OnResponse / OnError 轻量回调;
- 请求体:JSON / XML / multipart / 表单 / 字节;
- 统计:Client.Stats 请求、活跃、错误、重试计数。
- 请求级超时:WithRequestTimeout,与客户端超时取更严格者;
- 重试策略:WithRetryPolicy 自定义可重试判定;
- DNS 缓存:WithDNSCache 按 TTL 缓存解析,失败自动回退;
- 并发限流:WithMaxConcurrency 控制同时在途请求;
- HTTP/2 健康检查:WithHTTP2HealthCheck 读空闲 + Ping;
- 流式响应:ReadStream 逐块回调,带大小上限。
- 状态断言:EnsureStatus 校验期望状态码,错误携带 status/body 字段;
- 流式上传:FileField.Reader 大文件不整块载入内存;
- 重试上限:RetryPolicy.MaxBackoff 截断超大 Retry-After。

## 质量门槛

- 语句覆盖率 100%,race、vet、staticcheck、fuzz 全绿;
- govulncheck 漏洞扫描零告警;
- 三平台 CI(ubuntu / windows / macos);
- 性能基准与裸 `net/http` 同量级(见 docs/iteration-plan.md)。

## 文档

- [docs/README.md](docs/README.md) — 文档索引
- [docs/client-research.md](docs/client-research.md) — 热门 HTTP 客户端调研手册
- [docs/operations.md](docs/operations.md) — 运行手册(配置/指标/日志)
- [docs/security.md](docs/security.md) — 安全模型
- [examples/basic](examples/basic) — 基础请求与 JSON 解析
- [examples/retry](examples/retry) — 幂等重试与退避
- [examples/session](examples/session) — Cookie 会话与重定向
- [examples/limit](examples/limit) — 并发限流与 DNS 缓存
- [examples/http3](examples/http3) — HTTP/3 接入

## 贡献与安全

- [CONTRIBUTING.md](CONTRIBUTING.md) — 开发流程与质量门槛
- [SECURITY.md](SECURITY.md) — 安全说明与漏洞报告

## License

MIT © [lcylpzls](https://github.com/lcylpzls)
