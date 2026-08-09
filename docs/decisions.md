# httpx 架构决策记录(ADR)

> 状态说明:**已接受**(已定)。D1–D6 已于 2026-08-09 全部按推荐确认。

## ADR-001:薄客户端,复用 net/http

- **状态**:已接受
- **决策**:不实现协议解析,基于 `net/http.Transport` 与 `quic-go/http3`;
- **影响**:协议正确性由标准库/上游保证,性能上限与裸 net/http 一致。

## ADR-002:协议固定于 New,连接池按协议隔离

- **状态**:已接受
- **决策**:协议在创建 Client 时固定,H3 使用独立 RoundTripper 与连接池;
- **影响**:避免运行期切换协议导致连接池错乱,语义清晰。

## ADR-003:重试默认关闭,幂等优先

- **状态**:已接受(对应 D3)
- **决策**:默认不重试;显式开启后仅重试幂等方法与可重试错误;
- **影响**:默认行为最安全,重试语义由调用方明确选择。

## ADR-004:观测默认 no-op

- **状态**:已接受(对应 D5)
- **决策**:logx 与 Metrics 外部注入,未注入时热路径零开销;
- **影响**:与 dbx/logx 一致,不引入隐式 I/O。

## ADR-005:HTTP/3 走可选子包

- **状态**:已接受(对应 D1)
- **决策**:`httpx/http3` 子包承载 quic-go 依赖,核心包不直接依赖;
  ProtocolHTTP3 需先导入子包完成注册,否则 New 返回 HTX_UNSUPPORTED_PROTOCOL;
- **影响**:核心包保持零重依赖,CI 仅在 ubuntu 验证 H3。

## ADR-006:Do 返回标准 *http.Response

- **状态**:已接受(对应 D2)
- **决策**:`Do` / `Get` / `Post` / `Request` 直接返回 `*http.Response`,
  响应读取助手为包级函数(`ReadBody` / `ReadString` / `JSON`);
- **影响**:与 net/http 完全兼容,无自定义包装分配,Body 生命周期由调用方或助手管理。

## ADR-007:默认协议为自动协商

- **状态**:已接受(对应 D4)
- **决策**:默认 `ProtocolAuto`(HTTP/1.1 + HTTP/2 经 ALPN 自动协商),
  `ProtocolHTTP1` / `ProtocolHTTP2` 可显式强制;
- **影响**:一份连接池同时服务 H1/H2,兼容性与性能平衡。

## ADR-008:整体超时由 context 实现

- **状态**:已接受(对应 D6)
- **决策**:Dial / TLS / 响应头三层超时配置在 Transport,
  整体超时在 Do 入口通过 `context.WithTimeout` 实现,四层全可配;
- **影响**:整体超时覆盖重试与请求生命周期,不改变 Transport 语义。

## ADR-009:强制 HTTP/2 使用官方 x/net/http2

- **状态**:已接受
- **决策**:`ProtocolHTTP2` 使用 `golang.org/x/net/http2.Transport`
  (Go 官方扩展,与标准库同源);核心其余代码零第三方;
- **影响**:强制 H2 语义可靠,依赖面仅增加官方扩展库。

## ADR-010:协议构造器接收连接配置,Client 提供资源释放

- **状态**:已接受
- **决策**:`RegisterHTTP3` 构造器接收 `ProtocolConfig`
  (拨号超时与 TLS 配置),使 H3 与核心配置语义一致;
  `Client.CloseIdleConnections` 统一释放空闲连接(H3 走 io.Closer 兜底);
- **影响**:可选协议可复用客户端配置,调用方可在退出前显式释放连接池。

## ADR-011:重定向在客户端内实现,默认跟随上限 10

- **状态**:已接受(v0.2.0)
- **决策**:基于 `resp.Location` 自实现 301 / 302 / 303 / 307 / 308
  跟随,规则对齐 net/http(303 全转 GET,301 / 302 仅 POST 转 GET,
  307 / 308 保留方法);跨域剥离 Authorization / Cookie 等敏感头;
- **影响**:httpx 不使用 http.Client,重定向语义由自身保证,可配置可测。

## ADR-012:Cookie 走标准库 CookieJar,默认关闭

- **状态**:已接受(v0.2.0)
- **决策**:`WithCookieJar` 接入 `http.CookieJar`,未配置时行为与
  `http.DefaultClient` 一致(不维护 Cookie);
- **影响**:会话能力由标准库 jar 提供,httpx 只做自动注入与保存。

## ADR-013:钩子保持轻量,不进中间件框架

- **状态**:已接受(v0.2.0)
- **决策**:仅三个回调(OnRequest / OnResponse / OnError),
  在每次尝试与观测之间调用,不提供链式拦截器;
- **影响**:覆盖主流观测与审计场景,热路径仅多一个 nil 判断。

## ADR-014:统计为客户端级原子计数器

- **状态**:已接受(v0.2.0)
- **决策**:`Client.Stats` 返回请求 / 活跃 / 错误 / 重试快照,
  用 atomic 计数,不依赖 Transport 内部实现;
- **影响**:API 稳定且跨 H1/H2/H3 一致,活跃数在响应返回后递减。

## ADR-015:请求级超时经 context 传递,取更严格者

- **状态**:已接受(v0.3.0)
- **决策**:`WithTimeout` 请求选项将超时编码进请求 context,
  `Do` 统一与客户端级超时比较,取更严格者执行;
- **影响**:单次请求可覆盖客户端默认,语义与 net/http 一致且无 timer 泄漏。

## ADR-016:重试判定可定制,默认规则不变

- **状态**:已接受(v0.3.0)
- **决策**:`WithRetryPolicy` 的 `Retryable` 回调可覆盖默认
  方法白名单 + 状态码/错误分类判定;`WithRetry` 保持兼容;
- **影响**:业务可重试特殊状态码,默认行为不破坏。

## ADR-017:DNS 缓存独立可选,失败自动回退

- **状态**:已接受(v0.3.0)
- **决策**:`WithDNSCache` 开启按 TTL 缓存的主机解析,
  支持注入 `net.Resolver`;解析失败或拨号全部失败时回退系统解析;
- **影响**:默认不改变行为,开启后减少 DNS 往返,TTL 内 IP 变化有延迟。

## ADR-018:并发限流与 HTTP/2 健康检查为显式选项

- **状态**:已接受(v0.3.0)
- **决策**:`WithMaxConcurrency` 客户端级信号量,0 表示不限;
  `WithHTTP2HealthCheck` 设置读空闲与 Ping 超时,默认关闭;
- **影响**:资源上限由调用方显式声明,健康检查仅 H2 生效。

## ADR-019:响应头大小默认受限,连接细节可配

- **状态**:已接受(v0.4.0)
- **决策**:响应头默认上限 10MiB(与 HTTP/2 默认一致),
  `WithMaxResponseHeaderBytes` / `WithMaxConnsPerHost` /
  `WithExpectContinueTimeout` 可配置;
- **影响**:默认防响应头放大攻击,语义与 h2 对齐。

## ADR-020:重试总时长与请求 ID 纳入错误观测

- **状态**:已接受(v0.4.0)
- **决策**:`RetryPolicy.TotalTimeout` 限制整体重试耗时;
  `WithRequestID` 注入 X-Request-ID,日志与错误附带 request_id /
  method / url 字段;
- **影响**:重试有界,排障可关联请求链。

## ADR-021:发布流程纳入 apidiff 与依赖漂移检查

- **状态**:已接受(v0.4.0)
- **决策**:CI 增加 apidiff(对比上一 tag,informational)、
  go.mod tidy 漂移检查与重定向 fuzz;
- **影响**:版本升级影响可控,仓库依赖状态可审计。
