package core

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	testx "github.com/lcylpzls/testx"
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
	"github.com/lcylpzls/resiliencex"
)

func TestNewSuccess(t *testing.T) {
	for _, p := range []Protocol{ProtocolAuto, ProtocolHTTP1, ProtocolHTTP2} {
		client, err := New(WithProtocol(p))
		testx.RequireNoError(t, err)

		if client.cfg.protocol != p || client.rt == nil {
			t.Errorf("%v:客户端配置不符", p)
		}
	}
}

// wrapRT 测试用 RoundTripper 包装器。
type wrapRT struct {
	inner http.RoundTripper
}

func (w *wrapRT) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("X-Trace-Wrapped", "1")
	return w.inner.RoundTrip(req)
}

// TestWithRoundTripperWrapper 覆盖传输层包装（链路追踪插拔点）。
func TestWithRoundTripperWrapper(t *testing.T) {
	var inner http.RoundTripper
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Trace-Wrapped") != "1" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	client, err := New(WithRoundTripperWrapper(func(rt http.RoundTripper) http.RoundTripper {
		inner = rt
		return &wrapRT{inner: rt}
	}))
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), srv.URL)
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()
	testx.RequireEqual(t, resp.StatusCode, http.StatusOK)

	testx.RequireNotNil(t, inner)

	// nil 包装器被忽略。
	client2, err := New(WithRoundTripperWrapper(nil))
	if err != nil || client2 == nil {
		t.Fatalf("nil 包装器应忽略:err=%v", err)
	}
}

// nilBodyRT 返回无响应体的 RoundTripper。
type nilBodyRT struct{}

func (nilBodyRT) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header)}, nil
}

// TestTimeoutNilBody 覆盖超时响应体为空的取消分支。
func TestTimeoutNilBody(t *testing.T) {
	client, err := New(WithTimeout(time.Second))
	testx.RequireNoError(t, err)

	client.rt = nilBodyRT{}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	testx.RequireNoError(t, err)

	resp, err := client.Do(context.Background(), req)
	testx.RequireNoError(t, err)

	if resp.Body != nil {
		t.Fatal("应保持 nil Body")
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
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), srv.URL)
	testx.RequireNoError(t, err)

	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	testx.RequireNoError(t, err)

	if resp.ProtoMajor != 2 || string(data) != "hello h2" {
		t.Errorf("响应不符:proto=%s body=%q", resp.Proto, data)
	}
}

func TestNewHTTP3Unregistered(t *testing.T) {
	_, err := New(WithProtocol(ProtocolHTTP3))
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeUnsupportedProtocol {
		t.Errorf("错误码 = %s,want %s", code, CodeUnsupportedProtocol)
	}
}

func TestNewInvalidProtocol(t *testing.T) {
	_, err := New(WithProtocol(Protocol(99)))
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeInvalidConfig {
		t.Errorf("错误码 = %s,want %s", code, CodeInvalidConfig)
	}
}

func TestRegisterHTTP3(t *testing.T) {
	RegisterHTTP3(func(cfg ProtocolConfig) (http.RoundTripper, error) {
		testx.Equal(t, cfg.DialTimeout, defaultDialTimeout)

		return &fakeRoundTripper{status: http.StatusCreated}, nil
	})
	defer RegisterHTTP3(nil)

	client, err := New(WithProtocol(ProtocolHTTP3))
	testx.RequireNoError(t, err)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com", nil)
	testx.RequireNoError(t, err)

	resp, err := client.Do(context.Background(), req)
	testx.RequireNoError(t, err)

	testx.Equal(t, resp.StatusCode, http.StatusCreated)

}

func TestCloseIdleConnections(t *testing.T) {
	client, err := New()
	testx.RequireNoError(t, err)

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
	testx.RequireNoError(t, err)

	testx.Equal(t, client.cfg.protocol, ProtocolAuto)

}

func TestDoNilRequest(t *testing.T) {
	client, err := New()
	testx.RequireNoError(t, err)

	_, err = client.Do(context.Background(), nil)
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeInvalidConfig {
		t.Errorf("错误码 = %s,want %s", code, CodeInvalidConfig)
	}
}

func TestDoNilContext(t *testing.T) {
	srv := newEchoServer(t)
	client, err := New()
	testx.RequireNoError(t, err)

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	testx.RequireNoError(t, err)

	//lint:ignore SA1012 有意覆盖 nil context 防护逻辑
	resp, err := client.Do(nil, req)
	testx.RequireNoError(t, err)

	testx.Equal(t, resp.StatusCode, http.StatusOK)

	_ = resp.Body.Close()
}

func TestGetPostRequest(t *testing.T) {
	srv := newEchoServer(t)
	client, err := New()
	testx.RequireNoError(t, err)

	ctx := context.Background()

	// Get:header + query 合并
	resp, err := client.Get(ctx, srv.URL+"?a=1",
		WithHeader("X-Test", "v"),
		WithQuery("b", "2"),
		WithQuery("c", "3"),
	)
	testx.RequireNoError(t, err)

	body := readRespBody(t, resp)
	if !strings.Contains(body, "GET") || !strings.Contains(body, "a=1&b=2&c=3") || !strings.Contains(body, "X-Test: v") {
		t.Errorf("Get 请求不符合预期:%s", body)
	}

	// Post:string body
	resp, err = client.Post(ctx, srv.URL, "plain")
	testx.RequireNoError(t, err)

	body = readRespBody(t, resp)
	if !strings.Contains(body, "POST") || !strings.Contains(body, "body=plain") {
		t.Errorf("Post string 请求不符合预期:%s", body)
	}

	// Post:bytes body
	resp, err = client.Post(ctx, srv.URL, []byte("bytes"))
	testx.RequireNoError(t, err)

	body = readRespBody(t, resp)
	if !strings.Contains(body, "body=bytes") {
		t.Errorf("Post bytes 请求不符合预期:%s", body)
	}

	// Post:url.Values
	resp, err = client.Post(ctx, srv.URL, url.Values{"f": []string{"x"}})
	testx.RequireNoError(t, err)

	body = readRespBody(t, resp)
	if !strings.Contains(body, "body=f=x") || !strings.Contains(body, "form-urlencoded") {
		t.Errorf("Post form 请求不符合预期:%s", body)
	}

	// Post:自定义对象 → JSON
	resp, err = client.Post(ctx, srv.URL, map[string]int{"n": 1})
	testx.RequireNoError(t, err)

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
	testx.RequireNoError(t, err)

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
	testx.RequireNoError(t, err)

	body = readRespBody(t, resp)
	if !strings.Contains(body, "DELETE") || !strings.Contains(body, "Bearer tok") {
		t.Errorf("Request Bearer 不符合预期:%s", body)
	}
}

func TestRequestWithBytesBodyOverridesJSON(t *testing.T) {
	srv := newEchoServer(t)
	client, err := New()
	testx.RequireNoError(t, err)

	resp, err := client.Post(context.Background(), srv.URL, nil,
		WithJSONBody(map[string]int{"n": 1}),
		WithBytesBody([]byte("override")),
	)
	testx.RequireNoError(t, err)

	body := readRespBody(t, resp)
	if !strings.Contains(body, "body=override") {
		t.Errorf("WithBytesBody 应覆盖 JSON 体:%s", body)
	}
}

func TestBuildRequestInvalidURL(t *testing.T) {
	client, err := New()
	testx.RequireNoError(t, err)

	_, err = client.Get(context.Background(), "://bad-url")
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeInvalidConfig {
		t.Errorf("错误码 = %s,want %s", code, CodeInvalidConfig)
	}
}

func TestRequestInvalidMethod(t *testing.T) {
	client, err := New()
	testx.RequireNoError(t, err)

	_, err = client.Request(context.Background(), "BAD METHOD", "http://example.com")
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeInvalidConfig {
		t.Errorf("错误码 = %s,want %s", code, CodeInvalidConfig)
	}
}

func TestGetNilContext(t *testing.T) {
	srv := newEchoServer(t)
	client, err := New()
	testx.RequireNoError(t, err)

	//lint:ignore SA1012 有意覆盖 nil context 防护逻辑
	resp, err := client.Get(nil, srv.URL)
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()
}

func TestPostUnserializableBody(t *testing.T) {
	client, err := New()
	testx.RequireNoError(t, err)

	_, err = client.Post(context.Background(), "http://example.com", make(chan int))
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeInvalidConfig {
		t.Errorf("错误码 = %s,want %s", code, CodeInvalidConfig)
	}
}

func TestRequestWithFormBody(t *testing.T) {
	srv := newEchoServer(t)
	client, err := New()
	testx.RequireNoError(t, err)

	resp, err := client.Request(context.Background(), http.MethodPost, srv.URL,
		WithFormBody(url.Values{"f": []string{"x"}}))
	testx.RequireNoError(t, err)

	body := readRespBody(t, resp)
	if !strings.Contains(body, "body=f=x") || !strings.Contains(body, "form-urlencoded") {
		t.Errorf("WithFormBody 请求不符合预期:%s", body)
	}
}

func TestMarshalJSONBodyError(t *testing.T) {
	client, err := New()
	testx.RequireNoError(t, err)

	_, err = client.Post(context.Background(), "http://example.com", nil,
		WithJSONBody(map[any]any{make(chan int): 1}))
	testx.RequireError(t, err)

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
	testx.RequireNoError(t, err)

	_, err = client.Get(context.Background(), srv.URL)
	testx.RequireError(t, err)

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
	testx.RequireNoError(t, err)

	_, err = client.Get(context.Background(), srv.URL)
	testx.RequireError(t, err)

}

func TestContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New()
	testx.RequireNoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	defer cancel()
	_, err = client.Get(ctx, srv.URL)
	testx.RequireError(t, err)

	if IsRetryable(err) {
		t.Errorf("取消不应可重试:%v", err)
	}
}

func TestDialFailed(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	testx.RequireNoError(t, err)

	addr := ln.Addr().String()
	_ = ln.Close()

	client, err := New()
	testx.RequireNoError(t, err)

	_, err = client.Get(context.Background(), "http://"+addr)
	testx.RequireError(t, err)

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
	testx.RequireNoError(t, err)

	_, err = client.Get(context.Background(), "https://"+srv.Listener.Addr().String())
	testx.RequireError(t, err)

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
				testx.RequireNil(t, got)

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
	testx.RequireNoError(t, err)

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
	testx.RequireNoError(t, err)

	return string(data)
}

// TestNewBulkheadFailure 覆盖舱壁构造失败分支。
func TestNewBulkheadFailure(t *testing.T) {
	orig := newBulkhead
	newBulkhead = func(int, ...resiliencex.Option) (*resiliencex.Bulkhead, error) {
		return nil, errx.NewCode(CodeInvalidConfig, "舱壁构造失败")
	}
	defer func() { newBulkhead = orig }()

	_, err := New(WithMaxConcurrency(1))
	testx.RequireError(t, err)
}
