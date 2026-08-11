# 贡献指南

感谢参与 httpx 的打磨。请遵循以下约定。

## 环境与语言

- 开发机为 Windows,执行命令一律使用 PowerShell;
- 所有日志、注释与文档使用简体中文;
- 目标 Go 版本见 go.mod(当前 1.26)。

## 开发流程

1. 从 CHANGELOG 与 [docs/README.md](docs/README.md) 中梳理待办;
2. 在分支上实现,代码风格对齐现有文件(薄封装、显式命名、默认安全);
3. 本地验证:

```powershell
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@2026.1 ./...
go test -race -coverprofile=coverage.out ./...
go tool cover -func coverage.out
```

要求语句覆盖率 100%;新增公开 API 必须同步更新 README 与 CHANGELOG。

## 提交规范

- 提交信息以版本或主题开头,简述变更与验证结果;
- 涉及行为变更时在 CHANGELOG 记录;
- 破坏性变更仅允许在 <1.0.0 版本内,且需在 PR 说明。

## 性能

- 热路径(`Do` / `attempt` / 响应助手)不允许引入额外分配;
- 涉及性能变更时附 `go test -run '^$' -bench . -benchmem .` 对比。
