package httpx

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/lcylpzls/errx"
	"golang.org/x/net/http2"
)

// ProtocolConfig 是注册协议构造器时可用的连接配置。
type ProtocolConfig struct {
	// DialTimeout 建立连接的超时。
	DialTimeout time.Duration
	// TLSClientConfig 客户端 TLS 配置(可能为 nil)。
	TLSClientConfig *tls.Config
}

// h3Builder 由 httpx/http3 子包 init 注册,仅 ProtocolHTTP3 使用。
var h3Builder func(ProtocolConfig) (http.RoundTripper, error)

// RegisterHTTP3 注册 HTTP/3 RoundTripper 构造器,供可选子包调用。
// 构造器接收连接配置,可据此应用拨号超时与 TLS 设置。
// 重复注册以最后一次为准。
func RegisterHTTP3(builder func(ProtocolConfig) (http.RoundTripper, error)) {
	h3Builder = builder
}

// newRoundTripper 按协议构建 RoundTripper。
// Auto / HTTP1 / HTTP2 共用 net/http 生态,HTTP3 走独立子包注册。
func newRoundTripper(cfg config) (http.RoundTripper, error) {
	switch cfg.protocol {
	case ProtocolAuto:
		return newStdTransport(cfg, true), nil
	case ProtocolHTTP1:
		return newStdTransport(cfg, false), nil
	case ProtocolHTTP2:
		return newHTTP2Transport(cfg), nil
	case ProtocolHTTP3:
		if h3Builder == nil {
			return nil, errx.New(errx.KindInvalid, CodeUnsupportedProtocol,
				"HTTP/3 未注册,请导入 github.com/lcylpzls/httpx/http3")
		}
		return h3Builder(ProtocolConfig{
			DialTimeout:     cfg.dialTimeout,
			TLSClientConfig: cfg.tlsClientConfig,
		})
	default:
		return nil, errx.Newf(errx.KindInvalid, CodeInvalidConfig, "不支持的协议: %v", cfg.protocol)
	}
}

// newStdTransport 构建标准库 http.Transport:
// h2 为 true 时允许 ALPN 自动协商 HTTP/2,否则显式禁用。
func newStdTransport(cfg config, h2 bool) *http.Transport {
	dialer := &net.Dialer{
		Timeout:   cfg.dialTimeout,
		KeepAlive: 30 * time.Second,
	}
	tr := &http.Transport{
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     h2,
		MaxIdleConns:          cfg.maxIdleConns,
		MaxIdleConnsPerHost:   cfg.maxIdleConnsPerHost,
		IdleConnTimeout:       cfg.idleConnTimeout,
		TLSHandshakeTimeout:   cfg.tlsHandshakeTimeout,
		ResponseHeaderTimeout: cfg.responseHeaderTimeout,
	}
	if cfg.tlsClientConfig != nil {
		tr.TLSClientConfig = cfg.tlsClientConfig.Clone()
	}
	if !h2 {
		// 空 map 禁止自动 HTTP/2(标准库约定 TLSNextProto 非 nil 即不启用)。
		tr.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper)
	}
	return tr
}

// newHTTP2Transport 构建强制 HTTP/2 传输层(x/net/http2,官方扩展)。
// DialTLSContext 应用 DialTimeout 与 TLSHandshakeTimeout。
func newHTTP2Transport(cfg config) *http2.Transport {
	dialer := &net.Dialer{
		Timeout:   cfg.dialTimeout,
		KeepAlive: 30 * time.Second,
	}
	tr := &http2.Transport{
		AllowHTTP:       false,
		IdleConnTimeout: cfg.idleConnTimeout,
		DialTLSContext: func(ctx context.Context, network, addr string, tlsCfg *tls.Config) (net.Conn, error) {
			raw, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			tc := tls.Client(raw, tlsCfg)
			if cfg.tlsHandshakeTimeout > 0 {
				_ = tc.SetDeadline(time.Now().Add(cfg.tlsHandshakeTimeout))
			}
			if err := tc.HandshakeContext(ctx); err != nil {
				_ = tc.Close()
				return nil, err
			}
			_ = tc.SetDeadline(time.Time{})
			return tc, nil
		},
	}
	// http2.Transport 会基于 TLSClientConfig 自动补全 h2 ALPN 与 ServerName。
	if cfg.tlsClientConfig != nil {
		tr.TLSClientConfig = cfg.tlsClientConfig.Clone()
	}
	return tr
}
