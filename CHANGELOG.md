# 更新日志

本项目遵循[语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### 规划

- 完成 PRD、架构、API 草案、迭代计划与决策记录;
- D1–D6 决策点已全部确认并冻结 v0.1.0 API(见 docs/api-design.md)。

## [v0.3.0] - 2026-08-09

### 新增

- 请求级超时:WithTimeout 请求选项,与客户端级超时取更严格者;
- 自定义重试策略:WithRetryPolicy(Retryable 回调),WithRetry 保持兼容;
- DNS 缓存:WithDNSCache(TTL + 可注入 resolver + 失败回退);
- 并发限流:WithMaxConcurrency(0 表示不限,等待受 ctx 取消约束);
- HTTP/2 健康检查:WithHTTP2HealthCheck(读空闲 + Ping 超时);
- ReadStream 流式响应助手,逐块回调,带大小上限。

### 质量

- 核心与 http3 子包覆盖率 100%,race / vet / staticcheck / fuzz 全绿;
- 新增限流与 DNS 缓存示例,三平台 CI 全量验证。

## [v0.2.0] - 2026-08-09

### 新增

- 重定向:默认跟随上限 10,301/302/303/307/308 方法转换,
  跨域剥离 Authorization / Cookie 等敏感头;
  WithMaxRedirects / WithNoRedirect / WithRedirectPolicy;
- Cookie 会话:WithCookieJar 接入标准库 CookieJar,请求自动注入与保存;
- 轻量钩子:WithHooks(OnRequest / OnResponse / OnError);
- 请求体:WithMultipartFormData、WithXMLBody;
- 传输控制:WithProxy(默认环境代理,可显式关闭)、WithDisableCompression;
- Client.Stats 运行统计;ReadFile 响应落盘助手;
- 新错误码:HTX_REDIRECT_EXCEEDED / HTX_REDIRECT_FAILED。

### 变更

- 默认启用环境代理(HTTP_PROXY / HTTPS_PROXY),与 net/http 默认一致。

### 质量

- 核心与 http3 子包覆盖率 100%,race / vet / staticcheck / fuzz 全绿;
- 新增 session 示例,三平台 CI 全量验证。

### 0.1.0(计划中)

- 客户端核心:New / Do / Get / Post / Request,连接池与四层超时;
- 协议:HTTP/1.1、HTTP/2 自动协商与强制,HTTP/3 可选子包;
- 可选幂等重试:指数退避 + 抖动 + Retry-After;
- 观测:logx 注入 + Metrics 接口(默认 no-op);
- 响应助手:ReadBody / ReadString / JSON,带大小上限;
- 错误统一 errx,HTX_* 错误码;
- CloseIdleConnections 统一释放连接池资源;
- 100% 覆盖率、race、staticcheck、fuzz、三平台 CI。
