# 更新日志

本项目遵循[语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### 规划

- 完成 PRD、架构、API 草案、迭代计划与决策记录;
- D1–D6 决策点已全部确认并冻结 v0.1.0 API(见 docs/api-design.md)。

## [v1.0.2] - 2026-08-09

### 修复

- 客户端超时不再在响应头返回后立即取消请求上下文：超时改为覆盖
  完整请求生命周期（响应体关闭或计时器到期才取消），修复
  HTTP/2 / HTTP/3 在读取响应体前被本地取消（H3_REQUEST_CANCELLED）
  的问题；
- 新增回归测试：HTTP/3 与 HTTP/2 配置超时后仍可正常读取响应体。

## [v0.4.0] - 2026-08-09

### 新增

- 连接细节:WithMaxResponseHeaderBytes(默认 10MiB)、
  WithMaxConnsPerHost、WithExpectContinueTimeout;
- 重试总时长:RetryPolicy.TotalTimeout;
- 请求 ID:WithRequestID,日志与错误附带 request_id;
- 错误结构化字段:method / url;
- 治理与文档:SECURITY.md、CODEOWNERS、CONTRIBUTING、
  issue/PR 模板、security / operations / quality / release / performance 文档;
- CI:apidiff 对比上一版本、go.mod tidy 漂移检查、重定向 fuzz。

### 质量

- 核心与 http3 子包覆盖率 100%,race / vet / staticcheck / fuzz 全绿。

## [v0.5.0] - 2026-08-09

### 新增

- EnsureStatus 响应状态断言助手(HTX_UNEXPECTED_STATUS,带 status/body 字段);
- FileField.Reader 流式文件上传,大文件无需整块载入内存;
- RetryPolicy.MaxBackoff 单次退避上限(含 Retry-After 截断);
- Version 常量与库版本对齐;
- CI:govulncheck 漏洞扫描。

### 质量

- 核心与 http3 子包覆盖率 100%,race / vet / staticcheck / fuzz / vuln 全绿。

## [v1.0.0] - 2026-08-09

### 正式版

- 公开 API 冻结,遵循语义化版本;
- Version 常量更新为 v1.0.0;
- README 增加稳定性承诺(兼容性策略与发布门禁);
- docs/api-design.md 升级为 API 参考;
- 全量回归:100% 覆盖率、race、staticcheck、fuzz、govulncheck、
  apidiff 对比 v0.5.0、三平台 CI。

### 版本历程

- v0.1.0:客户端核心(连接池 / 分层超时 / H1 / H2 / H3 可选);
- v0.2.0:会话、重定向与钩子(集众家精华);
- v0.3.0:性能与流控(请求级超时 / DNS 缓存 / 限流 / 流式响应);
- v0.4.0:工业级打磨(治理 / 文档 / 连接细节 / 请求 ID);
- v0.5.0:协议与健壮性(状态断言 / 流式上传 / 退避上限 / 漏洞扫描);
- v1.0.0:正式版,API 冻结。

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
