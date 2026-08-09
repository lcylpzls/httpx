# 性能基准

## 方法

```powershell
go test -run '^$' -bench . -benchmem -benchtime=1s .
```

CI 的 bench job 记录每次 main 推送的基准日志(artifact),
不设硬性门禁,供版本间对比。

## 目标

| 场景 | 目标 |
| --- | --- |
| `Do` 复用连接 | 与裸 net/http 同量级(不高于 +20%) |
| `Do` 冷连接 | 不引入额外分配 |
| `JSON` 解析 5 字段响应 | ≤3 次额外分配 |
| 请求构造 | 选项合并零堆分配(字段内联) |

## 参考数据(v0.1.0,Windows / AMD Ryzen 5 7600)

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| DoReuse(httpx) | 37845 | 4631 | 60 |
| NetHTTPReuse(裸 net/http) | 35261 | 4593 | 60 |
| JSON | 61367 | 6018 | 71 |
| ReadString | 180971 | 5783 | 65 |
| BuildRequest | 14452 | 2376 | 20 |

`Do` 相对裸 net/http 约 +7%,分配次数一致;差异主要来自
统计计数与观测分支(默认 no-op 时仅原子计数)。

## 优化原则

- 热路径不增加分配(attempt / observe 分支先行判断);
- 可选能力默认关闭(重试、DNS 缓存、钩子、日志);
- 连接复用优先于连接创建。
