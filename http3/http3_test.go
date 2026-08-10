package http3

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	testx "github.com/lcylpzls/testx"
	"io"
	"math/big"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lcylpzls/httpx"
	"github.com/quic-go/quic-go/http3"
)

func TestHTTP3Request(t *testing.T) {
	addr, pool := newH3Server(t)

	client, err := httpx.New(
		httpx.WithProtocol(httpx.ProtocolHTTP3),
		httpx.WithTLSClientConfig(&tls.Config{RootCAs: pool}),
	)
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), "https://"+addr+"/hello")
	testx.RequireNoError(t, err)

	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	testx.RequireNoError(t, err)

	if resp.StatusCode != http.StatusOK || string(data) != "hello h3" {
		t.Errorf("响应不符:status=%d body=%q", resp.StatusCode, data)
	}
	if resp.ProtoMajor != 3 {
		t.Errorf("协议应为 HTTP/3:Proto=%s", resp.Proto)
	}
	client.CloseIdleConnections()
}

// TestHTTP3RequestWithTimeout 回归：客户端超时不得在读取响应体前取消流。
func TestHTTP3RequestWithTimeout(t *testing.T) {
	addr, pool := newH3Server(t)

	client, err := httpx.New(
		httpx.WithProtocol(httpx.ProtocolHTTP3),
		httpx.WithTimeout(5*time.Second),
		httpx.WithTLSClientConfig(&tls.Config{RootCAs: pool}),
	)
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), "https://"+addr+"/hello")
	testx.RequireNoError(t, err)

	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	testx.RequireNoError(t, err)

	if resp.StatusCode != http.StatusOK || string(data) != "hello h3" {
		t.Errorf("响应不符:status=%d body=%q", resp.StatusCode, data)
	}
	client.CloseIdleConnections()
}

func TestHTTP3WithoutTLSConfig(t *testing.T) {
	addr, _ := newH3Server(t)

	client, err := httpx.New(httpx.WithProtocol(httpx.ProtocolHTTP3))
	testx.RequireNoError(t, err)

	// 未注入信任根证书,握手应失败。
	_, err = client.Get(context.Background(), "https://"+addr)
	testx.RequireError(t, err)

}

func TestHTTP3DialTimeoutZero(t *testing.T) {
	addr, pool := newH3Server(t)

	client, err := httpx.New(
		httpx.WithProtocol(httpx.ProtocolHTTP3),
		httpx.WithDialTimeout(0),
		httpx.WithTLSClientConfig(&tls.Config{RootCAs: pool}),
	)
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), "https://"+addr)
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()
	client.CloseIdleConnections()
}

func TestHTTP3DisableCompression(t *testing.T) {
	addr, pool := newH3Server(t)

	client, err := httpx.New(
		httpx.WithProtocol(httpx.ProtocolHTTP3),
		httpx.WithDisableCompression(true),
		httpx.WithTLSClientConfig(&tls.Config{RootCAs: pool}),
	)
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), "https://"+addr)
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()
	client.CloseIdleConnections()
}

func TestHTTP3MaxResponseHeaderBytes(t *testing.T) {
	addr, pool := newH3Server(t)

	client, err := httpx.New(
		httpx.WithProtocol(httpx.ProtocolHTTP3),
		httpx.WithMaxResponseHeaderBytes(1<<20),
		httpx.WithTLSClientConfig(&tls.Config{RootCAs: pool}),
	)
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), "https://"+addr)
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()
	client.CloseIdleConnections()
}

// newH3Server 启动本地 HTTP/3 服务器,返回 UDP 地址与信任证书池。
func newH3Server(t *testing.T) (string, *x509.CertPool) {
	t.Helper()
	cert, leaf := newTestCert(t)
	ln, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	testx.RequireNoError(t, err)

	srv := &http3.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Protocol", r.Proto)
			fmt.Fprint(w, "hello h3")
		}),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
	}
	go func() {
		_ = srv.Serve(ln)
	}()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = ln.Close()
	})
	addr := ln.LocalAddr().(*net.UDPAddr).String()
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return addr, pool
}

// newTestCert 生成本地测试自签证书(ECDSA P-256)。
func newTestCert(t *testing.T) (tls.Certificate, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	testx.RequireNoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	testx.RequireNoError(t, err)

	leaf, err := x509.ParseCertificate(der)
	testx.RequireNoError(t, err)

	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, leaf
}
