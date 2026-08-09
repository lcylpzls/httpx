// Package http3 提供 HTTP/3(QUIC) 传输层接入。
// 导入本包后,httpx.New(httpx.WithProtocol(httpx.ProtocolHTTP3))
// 即可使用 HTTP/3 发起请求。
package http3

import (
	"context"
	"crypto/tls"
	"net/http"
	"time"

	"github.com/lcylpzls/httpx"
	"github.com/quic-go/quic-go"
	quichttp3 "github.com/quic-go/quic-go/http3"
)

// 默认 QUIC 连接参数。
const (
	defaultHandshakeIdleTimeout = 5 * time.Second
	defaultMaxIdleTimeout       = 30 * time.Second
)

// init 将 HTTP/3 传输层注册到 httpx,连接配置来自 httpx 客户端。
func init() {
	httpx.RegisterHTTP3(func(cfg httpx.ProtocolConfig) (http.RoundTripper, error) {
		tr := &quichttp3.Transport{
			QUICConfig: &quic.Config{
				HandshakeIdleTimeout: defaultHandshakeIdleTimeout,
				MaxIdleTimeout:       defaultMaxIdleTimeout,
			},
			DisableCompression: cfg.DisableCompression,
		}
		if cfg.TLSClientConfig != nil {
			tr.TLSClientConfig = cfg.TLSClientConfig.Clone()
		} else {
			tr.TLSClientConfig = &tls.Config{}
		}
		if cfg.DialTimeout > 0 {
			tr.Dial = func(ctx context.Context, addr string, tlsCfg *tls.Config, qcfg *quic.Config) (*quic.Conn, error) {
				dctx, cancel := context.WithTimeout(ctx, cfg.DialTimeout)
				defer cancel()
				return quic.DialAddr(dctx, addr, tlsCfg, qcfg)
			}
		}
		return tr, nil
	})
}
