package httpx

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/lcylpzls/errx"
	"golang.org/x/net/http2"
)

// fakeRoundTripper 供 HTTP/3 注册测试使用。
type fakeRoundTripper struct {
	status int
}

func (f *fakeRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: f.status,
		Status:     fmt.Sprintf("%d", f.status),
		Body:       io.NopCloser(nopReader{}),
		Header:     make(http.Header),
	}, nil
}

type nopReader struct{}

func (nopReader) Read([]byte) (int, error) { return 0, io.EOF }

func TestNewRoundTripperAllProtocols(t *testing.T) {
	for _, p := range []Protocol{ProtocolAuto, ProtocolHTTP1, ProtocolHTTP2} {
		cfg := defaultConfig()
		cfg.protocol = p
		rt, err := newRoundTripper(cfg)
		if err != nil {
			t.Fatalf("%v:构建失败:%v", p, err)
		}
		if rt == nil {
			t.Fatalf("%v:RoundTripper 为空", p)
		}
	}
	// 非法协议
	cfg := defaultConfig()
	cfg.protocol = Protocol(42)
	if _, err := newRoundTripper(cfg); err == nil {
		t.Fatal("非法协议应返回错误")
	}
	// HTTP/3 未注册
	cfg.protocol = ProtocolHTTP3
	if _, err := newRoundTripper(cfg); err == nil {
		t.Fatal("未注册 H3 应返回错误")
	}
}

func TestHTTP1Forced(t *testing.T) {
	srv := newH2Server(t)
	client, err := New(WithProtocol(ProtocolHTTP1), withTrustedRoots(srv))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get 失败:%v", err)
	}
	defer resp.Body.Close()
	if resp.ProtoMajor != 1 {
		t.Errorf("强制 HTTP/1 失败:Proto = %s", resp.Proto)
	}
}

func TestHTTP2Forced(t *testing.T) {
	srv := newH2Server(t)
	client, err := New(WithProtocol(ProtocolHTTP2), withTrustedRoots(srv))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get 失败:%v", err)
	}
	defer resp.Body.Close()
	if resp.ProtoMajor != 2 {
		t.Errorf("强制 HTTP/2 失败:Proto = %s", resp.Proto)
	}
}

func TestAutoNegotiatesHTTP2(t *testing.T) {
	srv := newH2Server(t)
	client, err := New(withTrustedRoots(srv))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get 失败:%v", err)
	}
	defer resp.Body.Close()
	if resp.ProtoMajor != 2 {
		t.Errorf("自动协商应选 HTTP/2:Proto = %s", resp.Proto)
	}
}

func TestAutoHTTP1Server(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get 失败:%v", err)
	}
	defer resp.Body.Close()
	if resp.ProtoMajor != 1 {
		t.Errorf("h1 服务器应协商为 HTTP/1:Proto = %s", resp.Proto)
	}
}

func TestHTTP2TLSHandshakeTimeoutZero(t *testing.T) {
	srv := newH2Server(t)
	client, err := New(
		WithProtocol(ProtocolHTTP2),
		WithTLSHandshakeTimeout(0),
		withTrustedRoots(srv),
	)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get 失败:%v", err)
	}
	defer resp.Body.Close()
	if resp.ProtoMajor != 2 {
		t.Errorf("HTTP/2 失败:Proto = %s", resp.Proto)
	}
}

func TestHTTP2AgainstHTTP1Server(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	client, err := New(WithProtocol(ProtocolHTTP2), withTrustedRoots(srv))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("强制 H2 连 h1-only 服务器应失败(ALPN 无交集)")
	}
	if code, _ := errx.CodeOf(err); code != CodeTLSFailed {
		t.Errorf("错误码 = %s,want %s;err=%v", code, CodeTLSFailed, err)
	}
}

func TestHTTP2DialFailed(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	client, err := New(WithProtocol(ProtocolHTTP2))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Get(context.Background(), "https://"+addr)
	if err == nil {
		t.Fatal("连接失败应返回错误")
	}
	if code, _ := errx.CodeOf(err); code != CodeDialFailed {
		t.Errorf("错误码 = %s,want %s;err=%v", code, CodeDialFailed, err)
	}
}

func TestProxyApplied(t *testing.T) {
	var proxied bool
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied = true
		w.WriteHeader(http.StatusOK)
	}))
	defer proxySrv.Close()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("请求不应直达目标")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer target.Close()

	client, err := New(WithProxy(func(*http.Request) (*url.URL, error) {
		return url.Parse(proxySrv.URL)
	}))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(context.Background(), target.URL)
	if err != nil {
		t.Fatalf("代理请求失败:%v", err)
	}
	_ = resp.Body.Close()
	if !proxied {
		t.Error("请求应经过代理")
	}
}

func TestProxyNilDisablesProxy(t *testing.T) {
	client, err := New(WithProxy(nil))
	if err != nil {
		t.Fatal(err)
	}
	tr := client.rt.(*http.Transport)
	if tr.Proxy != nil {
		t.Error("WithProxy(nil) 应显式关闭代理")
	}
}

func TestDefaultProxyFromEnvironment(t *testing.T) {
	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	tr := client.rt.(*http.Transport)
	if tr.Proxy == nil {
		t.Error("默认应使用环境代理")
	}
}

func TestDisableCompressionTransport(t *testing.T) {
	// Auto
	client, err := New(WithDisableCompression(true))
	if err != nil {
		t.Fatal(err)
	}
	if !client.rt.(*http.Transport).DisableCompression {
		t.Error("Auto 模式压缩开关未生效")
	}
	// HTTP/2
	client, err = New(WithProtocol(ProtocolHTTP2), WithDisableCompression(true))
	if err != nil {
		t.Fatal(err)
	}
	if !client.rt.(*http2.Transport).DisableCompression {
		t.Error("HTTP/2 压缩开关未生效")
	}
}

func TestDisableCompressionBehavior(t *testing.T) {
	var acceptEncoding string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acceptEncoding = r.Header.Get("Accept-Encoding")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New(WithDisableCompression(true))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if acceptEncoding != "" {
		t.Errorf("禁用压缩后不应请求 gzip:Accept-Encoding=%q", acceptEncoding)
	}
}

// newH2Server 返回启用 HTTP/2 的 TLS 测试服务器。
func newH2Server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// withTrustedRoots 生成信任测试服务器证书的 Option。
func withTrustedRoots(srv *httptest.Server) Option {
	pool := x509.NewCertPool()
	cert, err := x509.ParseCertificate(srv.Certificate().Raw)
	if err != nil {
		panic(err)
	}
	pool.AddCert(cert)
	return WithTLSClientConfig(&tls.Config{RootCAs: pool})
}
