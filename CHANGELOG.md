# 更新日志

本项目遵循[语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### 规划

- 完成 PRD、架构、API 草案、迭代计划与决策记录;
- D1–D6 决策点已全部确认并冻结 v0.1.0 API(见 docs/api-design.md)。

### 0.1.0(计划中)

- 客户端核心:New / Do / Get / Post / Request,连接池与四层超时;
- 协议:HTTP/1.1、HTTP/2 自动协商与强制,HTTP/3 可选子包;
- 可选幂等重试:指数退避 + 抖动 + Retry-After;
- 观测:logx 注入 + Metrics 接口(默认 no-op);
- 响应助手:ReadBody / ReadString / JSON,带大小上限;
- 错误统一 errx,HTX_* 错误码;
- CloseIdleConnections 统一释放连接池资源;
- 100% 覆盖率、race、staticcheck、fuzz、三平台 CI。
