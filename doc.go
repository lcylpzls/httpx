// Package httpx 提供基于 net/http 的薄高性能 HTTP 客户端库：
// 连接复用、分层超时、可选幂等重试与协议适配
// （HTTP/1.1 / HTTP/2 / HTTP/3），与 errx / logx 打通，
// 统一错误、日志与指标。
// 实现主体位于 internal/core，本包仅暴露稳定公开 API。
package httpx
