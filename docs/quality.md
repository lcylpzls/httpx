# 质量保障

## 门槛(每个版本强制)

- 语句覆盖率 100%(核心 + http3 子包);
- `go vet` / `staticcheck` 零告警;
- `go test -race` 全绿;
- fuzz 目标至少 3 个(errors、backoff、redirect);
- 三平台 CI(ubuntu / windows / macos)× Go 1.26;
- 示例全部可构建并通过 vet。

## 测试策略

- 单元测试:错误分类、退避、重定向方法转换、Cookie、钩子、统计、限流;
- 集成测试:H1 / H2(httptest + EnableHTTP2)/ H3(本地 QUIC 服务器);
- 脚本化 RoundTripper:精确覆盖重试与钩子分支;
- 并发测试:race 检测 + 峰值断言;
- 回归种子:CI fuzz 发现的边界输入入库(testdata/)。

## 性能基准

见 [performance.md](performance.md)。热路径禁止引入额外分配,
`Do` 目标与裸 `net/http` 同量级(不高于 +20%)。

## API 兼容性

- <1.0.0 允许有意的破坏性变更,须在 CHANGELOG 说明;
- v1.0.0 起 API 冻结,破坏性变更仅随大版本;
- CI 每次发布前执行 apidiff 对比上一 tag(informational)。

## 依赖策略

- 核心:标准库 + errx + logx + golang.org/x/net(官方扩展);
- http3 子包:quic-go 唯一重依赖;
- go.mod 漂移检查在 CI 强制(tidy 后无 diff)。
