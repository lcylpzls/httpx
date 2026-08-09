## 变更说明

（简述本次变更解决了什么问题、如何实现）

## 关联项

- Closes #
- 对应计划条目:`docs/iteration-plan.md` 中的阶段编号

## 验证

- [ ] `go vet ./...` 通过
- [ ] `staticcheck ./...` 通过
- [ ] `go test -race -coverprofile=coverage.out ./...` 通过,覆盖率 100%
- [ ] 涉及性能时附 `go test -run '^$' -bench . -benchmem ./...` 对比数据

## 兼容性

（是否破坏现有 API;是否需要同步更新 README、示例与 docs/api-design.md）
