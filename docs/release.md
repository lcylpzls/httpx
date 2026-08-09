# 发布流程

## 版本规则

- 语义化版本;<1.0.0 允许破坏性变更;
- CHANGELOG 按版本记录新增、变更与质量验证。

## 发布步骤

1. 本地全量验证(vet / staticcheck / race / 覆盖率 100% / fuzz 短跑);
2. 更新 CHANGELOG 与 README(当前版本、特性、示例);
3. 提交并推送 main,等待 CI 全绿:
   - test(三平台)、fuzz、examples、bench、apidiff、tidy;
4. 打 tag `vX.Y.Z` 并推送,Release workflow 自动:
   - 执行 `go test -race ./...`;
   - 创建 GitHub Release(自动生成说明);
5. 验证 `go list -m -versions github.com/lcylpzls/httpx` 出现新版本。

## 正式版(v1.0.0)额外要求

- API 冻结审查:确认无意外导出符号变化;
- README 增加稳定性承诺;
- apidiff 报告归档到 Release 说明。
