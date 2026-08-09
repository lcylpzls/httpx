# httpx 文档

## 阅读顺序

1. [PRD.md](PRD.md) — 要什么、不要什么;
2. [architecture.md](architecture.md) — 模块怎么切、性能怎么保证;
3. [api-design.md](api-design.md) — API 草案与待决策点;
4. [decisions.md](decisions.md) — 架构决策记录(ADR);
5. [iteration-plan.md](iteration-plan.md) — 迭代顺序与质量门槛。

运行与治理:

6. [operations.md](operations.md) — 配置速查、指标、日志与场景;
7. [security.md](security.md) — 安全模型与已知取舍;
8. [quality.md](quality.md) — 质量门槛与测试策略;
9. [release.md](release.md) — 版本与发布流程;
10. [performance.md](performance.md) — 性能基准方法与参考数据。

设计输入:[client-research.md](client-research.md) — 热门 HTTP 客户端调研手册
(net/http、fasthttp、resty、go-retryablehttp、OkHttp、httpx、reqwest、undici 等)。

## 决策参与方式

D1–D6 已全部确认(全部采纳推荐项),v0.1.0 API 已冻结,进入实现阶段。
