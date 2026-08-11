package httpx

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPublicAPI 是根包黑盒冒烟测试，覆盖全部公开转发函数。
func TestPublicAPI(t *testing.T) {
	if Version != "v1.4.1" {
		t.Fatalf("Version 不符：%s", Version)
	}

	// 常量
	_ = []any{ProtocolAuto, ProtocolHTTP1, ProtocolHTTP2, ProtocolHTTP3}
	_ = []any{
		CodeInvalidConfig, CodeUnsupportedProtocol, CodeDialFailed, CodeTLSFailed,
		CodeRequestFailed, CodeResponseFailed, CodeRetryExhausted, CodeBodyTooLarge,
		CodeBodyUnreadable, CodeRedirectExceeded, CodeRedirectFailed, CodeUnexpectedStatus,
	}

	// 退避策略
	_ = ExponentialBackoff(time.Second, 2, 0)(1)
	_ = FixedBackoff(time.Second)(1)

	// 客户端级选项与构造
	client, err := New(
		WithTimeout(time.Second),
		WithDialTimeout(time.Second),
		WithTLSHandshakeTimeout(time.Second),
		WithResponseHeaderTimeout(time.Second),
		WithMaxIdleConns(10),
		WithMaxIdleConnsPerHost(5),
		WithIdleConnTimeout(time.Minute),
		WithTLSClientConfig(&tls.Config{}),
		WithProtocol(ProtocolHTTP1),
		WithRetry(2, FixedBackoff(time.Millisecond)),
		WithRetryPolicy(RetryPolicy{MaxAttempts: 1, Backoff: FixedBackoff(time.Millisecond)}),
		WithLogger(nil),
		WithLogRequest(true),
		WithSlowThreshold(time.Millisecond),
		WithMetrics(nil),
		WithMaxRedirects(0),
		WithNoRedirect(),
		WithRedirectPolicy(nil),
		WithCookieJar(nil),
		WithHooks(Hooks{}),
		WithProxy(nil),
		WithDisableCompression(true),
		WithDNSCache(NewDNSCache(time.Second)),
		WithMaxConcurrency(2),
		WithHTTP2HealthCheck(0, 0),
		WithMaxResponseHeaderBytes(1024),
		WithMaxConnsPerHost(2),
		WithExpectContinueTimeout(time.Second),
		WithRoundTripperWrapper(nil),
	)
	if err != nil {
		t.Fatalf("New 失败：%v", err)
	}
	client.CloseIdleConnections()
	_ = client.Stats()

	// 请求级选项
	_ = []RequestOption{
		WithHeader("X-Test", "1"),
		WithQuery("a", "b"),
		WithJSONBody(map[string]string{"k": "v"}),
		WithBytesBody([]byte("x")),
		WithFormBody(url.Values{"a": {"b"}}),
		WithBasicAuth("u", "p"),
		WithBearer("tok"),
		WithUserAgent("ua"),
		WithMultipartFormData(map[string]string{"f": "v"}, nil),
		WithXMLBody(struct{}{}),
		WithRequestTimeout(time.Second),
		WithRequestID("r1"),
	}

	// DNS 缓存
	cache := NewDNSCache(time.Minute)
	cache.Reset()

	// 协议注册
	RegisterHTTP3(func(ProtocolConfig) (http.RoundTripper, error) {
		return nil, nil
	})

	// 响应读取助手
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"a":1}`)),
		Header:     make(http.Header),
	}
	var out map[string]int
	if err := JSON(resp, &out); err != nil {
		t.Fatalf("JSON 失败：%v", err)
	}

	resp = &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("abc")),
		Header:     make(http.Header),
	}
	if _, err := ReadBody(resp, 1024); err != nil {
		t.Fatalf("ReadBody 失败：%v", err)
	}

	resp = &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("abc")),
		Header:     make(http.Header),
	}
	if s, err := ReadString(resp, 1024); err != nil || s != "abc" {
		t.Fatalf("ReadString 失败：%q %v", s, err)
	}

	resp = &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("file")),
		Header:     make(http.Header),
	}
	if err := ReadFile(resp, filepath.Join(t.TempDir(), "x.bin"), 1024); err != nil {
		t.Fatalf("ReadFile 失败：%v", err)
	}

	resp = &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("chunk")),
		Header:     make(http.Header),
	}
	n := 0
	if err := ReadStream(resp, func([]byte) error { n++; return nil }, 1024); err != nil {
		t.Fatalf("ReadStream 失败：%v", err)
	}
	if n == 0 {
		t.Fatal("ReadStream 未回调")
	}

	resp = &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("ok")),
		Header:     make(http.Header),
	}
	if err := EnsureStatus(resp, http.StatusOK); err != nil {
		t.Fatalf("EnsureStatus 失败：%v", err)
	}

	// 错误与超时判定
	_ = ErrorStatus(errors.New("boom"))
	_ = IsRetryable(errors.New("boom"))
	_ = IsTimeout(context.DeadlineExceeded)

	// JSON 错误输出
	rec := httptest.NewRecorder()
	WriteErrorJSON(rec, errors.New("boom"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("WriteErrorJSON 状态码不符：%d", rec.Code)
	}
}
