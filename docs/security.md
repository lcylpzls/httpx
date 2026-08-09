# 安全模型

## 传输安全

- 默认 TLS 最低版本由 Go 标准库保证(TLS 1.2+),不提供跳过证书校验的选项;
- 私有 CA / 客户端证书通过 `WithTLSClientConfig` 注入;
- HTTP/2 强制模式仅走 TLS(h2c 不支持),HTTP/3 走 QUIC + TLS 1.3。

## 凭据保护

- 跨域重定向自动剥离 Authorization / Proxy-Authorization / Cookie /
  Cookie2 / WWW-Authenticate 等敏感请求头;
- 日志只输出 method / path / status / duration / attempt / request_id,
  不打印请求头与请求体;
- 认证信息以 RequestOption 传入,不进入日志字段。

## 资源保护

- 响应头默认上限 10MiB(`WithMaxResponseHeaderBytes` 可调);
- 响应体助手全部带大小上限(JSON 默认 16MiB);
- 并发在途请求可用 `WithMaxConcurrency` 收敛;
- 空闲连接由 `IdleConnTimeout` 回收,退出前调用 `CloseIdleConnections`。

## 已知取舍

- DNS 缓存(`WithDNSCache`)在 TTL 内不感知 DNS 变更;
- 环境代理默认开启,代理配置泄露时等同网络路径风险;
- 重试会放大请求量,务必配合 `RetryPolicy.TotalTimeout` 与整体超时。

## 报告漏洞

见 [SECURITY.md](../SECURITY.md)。
