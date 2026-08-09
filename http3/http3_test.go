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
	if err != nil {
		t.Fatalf("New 失败:%v", err)
	}
	resp, err := client.Get(context.Background(), "https://"+addr+"/hello")
	if err != nil {
		t.Fatalf("HTTP/3 请求失败:%v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取响应失败:%v", err)
	}
	if resp.StatusCode != http.StatusOK || string(data) != "hello h3" {
		t.Errorf("响应不符:status=%d body=%q", resp.StatusCode, data)
	}
	if resp.ProtoMajor != 3 {
		t.Errorf("协议应为 HTTP/3:Proto=%s", resp.Proto)
	}
	client.CloseIdleConnections()
}

func TestHTTP3WithoutTLSConfig(t *testing.T) {
	addr, _ := newH3Server(t)

	client, err := httpx.New(httpx.WithProtocol(httpx.ProtocolHTTP3))
	if err != nil {
		t.Fatalf("New 失败:%v", err)
	}
	// 未注入信任根证书,握手应失败。
	_, err = client.Get(context.Background(), "https://"+addr)
	if err == nil {
		t.Fatal("自签证书未被信任时应失败")
	}
}

func TestHTTP3DialTimeoutZero(t *testing.T) {
	addr, pool := newH3Server(t)

	client, err := httpx.New(
		httpx.WithProtocol(httpx.ProtocolHTTP3),
		httpx.WithDialTimeout(0),
		httpx.WithTLSClientConfig(&tls.Config{RootCAs: pool}),
	)
	if err != nil {
		t.Fatalf("New 失败:%v", err)
	}
	resp, err := client.Get(context.Background(), "https://"+addr)
	if err != nil {
		t.Fatalf("DialTimeout=0 应使用默认拨号:%v", err)
	}
	_ = resp.Body.Close()
	client.CloseIdleConnections()
}

// newH3Server 启动本地 HTTP/3 服务器,返回 UDP 地址与信任证书池。
func newH3Server(t *testing.T) (string, *x509.CertPool) {
	t.Helper()
	cert, leaf := newTestCert(t)
	ln, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("监听 UDP 失败:%v", err)
	}
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
	if err != nil {
		t.Fatalf("生成密钥失败:%v", err)
	}
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
	if err != nil {
		t.Fatalf("创建证书失败:%v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("解析证书失败:%v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, leaf
}
