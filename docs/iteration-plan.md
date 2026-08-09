# httpx 迭代计划与质量门槛

## 1. 迭代阶段

### P0 项目骨架

- go.mod(module github.com/lcylpzls/httpx,go 1.26)、目录、CI(三平台 +
  staticcheck + fuzz)、错误码注册与空包测试。

### P1 客户端核心(HTTP/1 + HTTP/2)

- `Client` / `New` / `Do` / `Get` / `Post` / `Request`;
- 连接池与四层超时;`ProtocolAuto` / `ProtocolHTTP1` / `ProtocolHTTP2`。

**验收**:httptest 服务器集成测试,100% 覆盖率。

### P2 重试与观测

- 幂等重试、指数退避 + 抖动、可重读请求体;
- logx 请求摘要/慢请求、Metrics 计数与耗时。

**验收**:重试/退避/取消全路径测试,`FuzzRetry` 通过。

### P3 响应助手

- `ReadBody` / `ReadString` / `JSON`,大小上限与 Body 关闭保证。

### P4 HTTP/3 子包

- `httpx/http3`(quic-go)接入;本地 QUIC 服务器集成测试。

### P5 基准、示例与 v0.1.0

- examples(基础调用、重试、H3);README/docs 收尾;API 基线;发布。

### P6 会话、重定向与钩子(v0.2.0)

- 重定向策略与跨域敏感头剥离;CookieJar 自动维护;
- OnRequest / OnResponse / OnError 轻量钩子;
- multipart / XML 请求体;代理与压缩开关;
- Client.Stats 与 ReadFile;
- 更新示例(会话与重定向);发布 v0.2.0。

**验收**:重定向方法转换 / 跨域剥离 / 跳转超限、Cookie 会话、钩子、
multipart、Stats、ReadFile 全路径测试,100% 覆盖率。

### P7 性能与流控(v0.3.0)

- 请求级超时与自定义重试策略;
- DNS 缓存(TTL + 可注入 resolver + 失败回退);
- 并发限流与 HTTP/2 健康检查;
- ReadStream 流式响应助手;
- 更新示例(限流 / DNS 缓存);发布 v0.3.0。

**验收**:请求级超时合并语义、自定义重试判定、DNS 缓存命中/过期/
回退、限流峰值与取消、ReadStream 超限/回调错误全路径测试,100% 覆盖率。

### P8 工业级打磨(v0.4.0)

- 连接细节选项与默认响应头上限;
- 重试总时长、请求 ID、错误结构化字段;
- 治理文件与安全/运行/质量/发布/性能文档;
- CI:apidiff、tidy 检查、重定向 fuzz;
- 发布 v0.4.0。

### P9 正式版 v1.0.0

- API 冻结审查与稳定性承诺;
- 全量回归与三平台验证;发布 v1.0.0。

**验收**:打磨批次全路径测试 100% 覆盖;apidiff 报告无意外破坏;
正式版 API 冻结。

## 2. 质量门槛(每阶段强制)

- 语句覆盖率 100%;`go vet` / `staticcheck` 零告警;`go test -race` 全绿;
- fuzz:重试退避、URL 解析至少 1 个目标(CI 短跑);
- 三平台 CI × Go 1.26;HTTP/3 集成在 ubuntu 上运行;
- 所有日志、注释、文档使用简体中文。

## 3. 性能基准(目标,实现后建立基线)

| 场景 | 目标 |
| --- | --- |
| `Do` 复用连接 | 与裸 `net/http` 同量级(不高于 +20%) |
| `Do` 冷连接(Dial+TLS) | 不引入额外分配 |
| `JSON` 解析 5 字段响应 | ≤3 次分配 |
| 连接池空闲回收 | IdleConnTimeout 后连接释放,无泄漏 |

## 4. 风险与对策

| 风险 | 对策 |
| --- | --- |
| HTTP/3 依赖重、平台差异 | 放可选子包,核心不受影响;CI 仅 ubuntu 验证 |
| 重试语义误用 | 默认关闭 + 幂等白名单 + 文档强调 |
| 与裸 net/http 性能偏差 | 基准对比写入文档,热路径保持薄封装 |
