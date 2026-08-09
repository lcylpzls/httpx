# 安全说明

httpx 将以下情况视为安全缺陷:

- 日志或指标中泄露 Authorization / Cookie 等敏感请求头;
- 响应体或响应头大小失控导致内存被打爆(库侧默认值失效);
- 重定向跨域时未剥离敏感头导致凭据泄露;
- TLS 配置允许弱协议或跳过证书校验。

## 报告漏洞

- 请勿在公开 issue 中披露漏洞细节;
- 通过邮件 [lcylpzls@qq.com](mailto:lcylpzls@qq.com) 联系维护者,
  并在标题注明 `[Security]`;
- 修复发布前我们会与报告者保持沟通;发布后可应要求公开致谢。

## 安全使用建议

- 生产环境显式设置整体超时(`WithTimeout`)与大小上限;
- 跨域重定向默认剥离 Authorization / Cookie,请勿关闭该行为;
- 不要在日志钩子中打印请求头全文;
- 私有 CA 通过 `WithTLSClientConfig` 注入根证书,不要跳过证书校验;
- 开启 `WithDNSCache` 时,TTL 内 DNS 变更存在延迟,请权衡业务场景。
