package httpx

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lcylpzls/errx"
)

func TestNewSuccess(t *testing.T) {
	for _, p := range []Protocol{ProtocolAuto, ProtocolHTTP1, ProtocolHTTP2} {
		client, err := New(WithProtocol(p))
		if err != nil {
			t.Fatalf("%v:New 失败:%v", p, err)
		}
		if client.cfg.protocol != p || client.rt == nil {
			t.Errorf("%v:客户端配置不符", p)
		}
	}
}

// TestHTTP2TimeoutBodyRead 回归：HTTP/2 客户端超时不得在读取响应体前取消流。
func TestHTTP2TimeoutBodyRead(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello h2"))
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	rootCAs := srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	client, err := New(
		WithProtocol(ProtocolHTTP2),
		WithTimeout(5*time.Second),
		WithTLSClientConfig(&tls.Config{RootCAs: rootCAs}),
	)
	if err != nil {
		t.Fatalf("New 失败:%v", err)
	}
	resp, err := client.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("HTTP/2 请求失败:%v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("带超时读取响应体失败:%v", err)
	}
	if resp.ProtoMajor != 2 || string(data) != "hello h2" {
		t.Errorf("响应不符:proto=%s body=%q", resp.Proto, data)
	}
}

func TestNewHTTP3Unregistered(t *testing.T) {
	_, err := New(WithProtocol(ProtocolHTTP3))
	if err == nil {
		t.Fatal("未注册 HTTP/3 应返回错误")
	}
	if code, _ := errx.CodeOf(err); code != CodeUnsupportedProtocol {
		t.Errorf("错误码 = %s,want %s", code, CodeUnsupportedProtocol)
	}
}

func TestNewInvalidProtocol(t *testing.T) {
	_, err := New(WithProtocol(Protocol(99)))
	if err == nil {
		t.Fatal("非法协议应返回错误")
	}
	if code, _ := errx.CodeOf(err); code != CodeInvalidConfig {
		t.Errorf("错误码 = %s,want %s", code, CodeInvalidConfig)
	}
}

func TestRegisterHTTP3(t *testing.T) {
	RegisterHTTP3(func(cfg ProtocolConfig) (http.RoundTripper, error) {
		if cfg.DialTimeout != defaultDialTimeout {
			t.Errorf("DialTimeout 未传递:got %v,want %v", cfg.DialTimeout, defaultDialTimeout)
		}
		return &fakeRoundTripper{status: http.StatusCreated}, nil
	})
	defer RegisterHTTP3(nil)

	client, err := New(WithProtocol(ProtocolHTTP3))
	if err != nil {
		t.Fatalf("注册后 New 失败:%v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do 失败:%v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("状态码 = %d,want %d", resp.StatusCode, http.StatusCreated)
	}
}

func TestCloseIdleConnections(t *testing.T) {
	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	// 标准库 Transport 实现 CloseIdleConnections
	client.CloseIdleConnections()
	// io.Closer 分支(如 HTTP/3 Transport)
	client.rt = &closerRT{}
	client.CloseIdleConnections()
}

// closerRT 仅实现 io.Closer,覆盖 CloseIdleConnections 的兜底分支。
type closerRT struct {
	closed bool
}

func (c *closerRT) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("未实现")
}

func (c *closerRT) Close() error {
	c.closed = true
	return nil
}

func TestNewNilOptions(t *testing.T) {
	client, err := New(nil)
	if err != nil {
		t.Fatalf("nil 选项应被忽略:%v", err)
	}
	if client.cfg.protocol != ProtocolAuto {
		t.Error("nil 选项不应改变默认配置")
	}
}

func TestDoNilRequest(t *testing.T) {
	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Do(context.Background(), nil)
	if err == nil {
		t.Fatal("nil 请求应返回错误")
	}
	if code, _ := errx.CodeOf(err); code != CodeInvalidConfig {
		t.Errorf("错误码 = %s,want %s", code, CodeInvalidConfig)
	}
}

func TestDoNilContext(t *testing.T) {
	srv := newEchoServer(t)
	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	//lint:ignore SA1012 有意覆盖 nil context 防护逻辑
	resp, err := client.Do(nil, req)
	if err != nil {
		t.Fatalf("nil context 应视为 Background:%v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("状态码 = %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestGetPostRequest(t *testing.T) {
	srv := newEchoServer(t)
	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Get:header + query 合并
	resp, err := client.Get(ctx, srv.URL+"?a=1",
		WithHeader("X-Test", "v"),
		WithQuery("b", "2"),
		WithQuery("c", "3"),
	)
	if err != nil {
		t.Fatalf("Get 失败:%v", err)
	}
	body := readRespBody(t, resp)
	if !strings.Contains(body, "GET") || !strings.Contains(body, "a=1&b=2&c=3") || !strings.Contains(body, "X-Test: v") {
		t.Errorf("Get 请求不符合预期:%s", body)
	}

	// Post:string body
	resp, err = client.Post(ctx, srv.URL, "plain")
	if err != nil {
		t.Fatalf("Post string 失败:%v", err)
	}
	body = readRespBody(t, resp)
	if !strings.Contains(body, "POST") || !strings.Contains(body, "body=plain") {
		t.Errorf("Post string 请求不符合预期:%s", body)
	}

	// Post:bytes body
	resp, err = client.Post(ctx, srv.URL, []byte("bytes"))
	if err != nil {
		t.Fatalf("Post bytes 失败:%v", err)
	}
	body = readRespBody(t, resp)
	if !strings.Contains(body, "body=bytes") {
		t.Errorf("Post bytes 请求不符合预期:%s", body)
	}

	// Post:url.Values
	resp, err = client.Post(ctx, srv.URL, url.Values{"f": []string{"x"}})
	if err != nil {
		t.Fatalf("Post form 失败:%v", err)
	}
	body = readRespBody(t, resp)
	if !strings.Contains(body, "body=f=x") || !strings.Contains(body, "form-urlencoded") {
		t.Errorf("Post form 请求不符合预期:%s", body)
	}

	// Post:自定义对象 → JSON
	resp, err = client.Post(ctx, srv.URL, map[string]int{"n": 1})
	if err != nil {
		t.Fatalf("Post JSON 失败:%v", err)
	}
	body = readRespBody(t, resp)
	if !strings.Contains(body, `body={"n":1}`) || !strings.Contains(body, "application/json") {
		t.Errorf("Post JSON 请求不符合预期:%s", body)
	}

	// Request:全部请求选项
	resp, err = client.Request(ctx, http.MethodPut, srv.URL,
		WithJSONBody(map[string]string{"k": "v"}),
		WithBasicAuth("user", "pass"),
		WithUserAgent("httpx-test"),
	)
	if err != nil {
		t.Fatalf("Request 失败:%v", err)
	}
	body = readRespBody(t, resp)
	if !strings.Contains(body, "PUT") ||
		!strings.Contains(body, `body={"k":"v"}`) ||
		!strings.Contains(body, "auth=user:pass") ||
		!strings.Contains(body, "ua=httpx-test") {
		t.Errorf("Request 选项不符合预期:%s", body)
	}

	// Request:Bearer 认证(与 BasicAuth 共享 Authorization 头,后设置者覆盖)
	resp, err = client.Request(ctx, http.MethodDelete, srv.URL,
		WithBearer("tok"),
	)
	if err != nil {
		t.Fatalf("Request Bearer 失败:%v", err)
	}
	body = readRespBody(t, resp)
	if !strings.Contains(body, "DELETE") || !strings.Contains(body, "Bearer tok") {
		t.Errorf("Request Bearer 不符合预期:%s", body)
	}
}

func TestRequestWithBytesBodyOverridesJSON(t *testing.T) {
	srv := newEchoServer(t)
	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Post(context.Background(), srv.URL, nil,
		WithJSONBody(map[string]int{"n": 1}),
		WithBytesBody([]byte("override")),
	)
	if err != nil {
		t.Fatalf("请求失败:%v", err)
	}
	body := readRespBody(t, resp)
	if !strings.Contains(body, "body=override") {
		t.Errorf("WithBytesBody 应覆盖 JSON 体:%s", body)
	}
}

func TestBuildRequestInvalidURL(t *testing.T) {
	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Get(context.Background(), "://bad-url")
	if err == nil {
		t.Fatal("非法 URL 应返回错误")
	}
	if code, _ := errx.CodeOf(err); code != CodeInvalidConfig {
		t.Errorf("错误码 = %s,want %s", code, CodeInvalidConfig)
	}
}

func TestRequestInvalidMethod(t *testing.T) {
	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Request(context.Background(), "BAD METHOD", "http://example.com")
	if err == nil {
		t.Fatal("非法方法应返回错误")
	}
	if code, _ := errx.CodeOf(err); code != CodeInvalidConfig {
		t.Errorf("错误码 = %s,want %s", code, CodeInvalidConfig)
	}
}

func TestGetNilContext(t *testing.T) {
	srv := newEchoServer(t)
	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	//lint:ignore SA1012 有意覆盖 nil context 防护逻辑
	resp, err := client.Get(nil, srv.URL)
	if err != nil {
		t.Fatalf("Get nil context 应视为 Background:%v", err)
	}
	_ = resp.Body.Close()
}

func TestPostUnserializableBody(t *testing.T) {
	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Post(context.Background(), "http://example.com", make(chan int))
	if err == nil {
		t.Fatal("不可序列化 body 应返回错误")
	}
	if code, _ := errx.CodeOf(err); code != CodeInvalidConfig {
		t.Errorf("错误码 = %s,want %s", code, CodeInvalidConfig)
	}
}

func TestRequestWithFormBody(t *testing.T) {
	srv := newEchoServer(t)
	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Request(context.Background(), http.MethodPost, srv.URL,
		WithFormBody(url.Values{"f": []string{"x"}}))
	if err != nil {
		t.Fatalf("WithFormBody 请求失败:%v", err)
	}
	body := readRespBody(t, resp)
	if !strings.Contains(body, "body=f=x") || !strings.Contains(body, "form-urlencoded") {
		t.Errorf("WithFormBody 请求不符合预期:%s", body)
	}
}

func TestMarshalJSONBodyError(t *testing.T) {
	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Post(context.Background(), "http://example.com", nil,
		WithJSONBody(map[any]any{make(chan int): 1}))
	if err == nil {
		t.Fatal("不可序列化请求体应返回错误")
	}
	if code, _ := errx.CodeOf(err); code != CodeInvalidConfig {
		t.Errorf("错误码 = %s,want %s", code, CodeInvalidConfig)
	}
}

func TestDoWithTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New(WithTimeout(80 * time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("整体超时应触发")
	}
	if !IsTimeout(err) {
		t.Errorf("应为超时错误:%v", err)
	}
}

func TestResponseHeaderTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New(WithResponseHeaderTimeout(80 * time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("响应头超时应触发")
	}
}

func TestContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	defer cancel()
	_, err = client.Get(ctx, srv.URL)
	if err == nil {
		t.Fatal("取消应返回错误")
	}
	if IsRetryable(err) {
		t.Errorf("取消不应可重试:%v", err)
	}
}

func TestDialFailed(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Get(context.Background(), "http://"+addr)
	if err == nil {
		t.Fatal("连接失败应返回错误")
	}
	if code, _ := errx.CodeOf(err); code != CodeDialFailed {
		t.Errorf("错误码 = %s,want %s;err=%v", code, CodeDialFailed, err)
	}
}

func TestTLSFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Get(context.Background(), "https://"+srv.Listener.Addr().String())
	if err == nil {
		t.Fatal("TLS 失败应返回错误")
	}
	if code, _ := errx.CodeOf(err); code != CodeTLSFailed {
		t.Errorf("错误码 = %s,want %s;err=%v", code, CodeTLSFailed, err)
	}
}

func TestWrapDoError(t *testing.T) {
	plain := errors.New("普通错误")
	structured := errx.New(errx.KindUnavailable, CodeRequestFailed, "结构化错误")
	cases := []struct {
		name string
		err  error
		code errx.Code
	}{
		{"nil", nil, ""},
		{"结构化透传", structured, CodeRequestFailed},
		{"net.OpError dial", &net.OpError{Op: "dial", Err: plain}, CodeDialFailed},
		{"url.Error dial", &url.Error{Op: "dial", Err: plain}, CodeDialFailed},
		{"url.Error tls", &url.Error{Op: "tls handshake", Err: plain}, CodeTLSFailed},
		{"url.Error method", &url.Error{Op: "GET", Err: plain}, CodeRequestFailed},
		{"tls 记录头错误", tls.RecordHeaderError{Msg: "模拟"}, CodeTLSFailed},
		{"tls 证书验证错误", &tls.CertificateVerificationError{Err: errors.New("模拟")}, CodeTLSFailed},
		{"普通错误", plain, CodeRequestFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := wrapDoError(tc.err)
			if tc.err == nil {
				if got != nil {
					t.Fatalf("nil 应返回 nil,got %v", got)
				}
				return
			}
			if code, ok := errx.CodeOf(got); !ok || code != tc.code {
				t.Errorf("错误码 = %s,want %s;err=%v", code, tc.code, got)
			}
		})
	}
}

func TestIsTLSError(t *testing.T) {
	plain := errors.New("普通错误")
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"普通错误", plain, false},
		{"tls 前缀", errors.New("tls: boom"), true},
		{"net.OpError tls", &net.OpError{Op: "tls", Err: plain}, true},
		{"net.OpError dial", &net.OpError{Op: "dial", Err: plain}, false},
		{"tls.RecordHeaderError", tls.RecordHeaderError{Msg: "模拟"}, true},
		{"tls.CertificateVerificationError", &tls.CertificateVerificationError{Err: plain}, true},
	}
	for _, tc := range cases {
		if got := isTLSError(tc.err); got != tc.want {
			t.Errorf("%s:isTLSError = %v,want %v", tc.name, got, tc.want)
		}
	}
}

func TestConcurrentRequests(t *testing.T) {
	srv := newEchoServer(t)
	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	var done atomic.Int64
	for i := 0; i < 16; i++ {
		go func() {
			resp, err := client.Get(context.Background(), srv.URL, WithQuery("q", "1"))
			if err == nil {
				_ = resp.Body.Close()
				done.Add(1)
			}
		}()
	}
	deadline := time.Now().Add(5 * time.Second)
	for done.Load() < 16 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if done.Load() != 16 {
		t.Errorf("并发请求完成数 = %d,want 16", done.Load())
	}
}

// newEchoServer 返回将请求特征回显为文本的测试服务器。
func newEchoServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		authUser, authPass, _ := r.BasicAuth()
		fmt.Fprintf(w, "%s %s %s %s %s %s %s %s|%s",
			r.Method,
			r.URL.RawQuery,
			"X-Test: "+r.Header.Get("X-Test"),
			"Authorization: "+r.Header.Get("Authorization"),
			"ua="+r.Header.Get("User-Agent"),
			"ct="+r.Header.Get("Content-Type"),
			"auth="+authUser+":"+authPass,
			"proto="+r.Proto,
			"body="+string(body),
		)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// readRespBody 读取并关闭响应体。
func readRespBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取响应失败:%v", err)
	}
	return string(data)
}
