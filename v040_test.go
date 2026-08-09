package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lcylpzls/errx"
	"golang.org/x/net/http2"
)

// ---------- 连接细节选项 ----------

func TestMaxResponseHeaderBytesDefault(t *testing.T) {
	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	tr := client.rt.(*http.Transport)
	if tr.MaxResponseHeaderBytes != defaultMaxResponseHeaderBytes {
		t.Errorf("默认响应头上限 = %d,want %d", tr.MaxResponseHeaderBytes, defaultMaxResponseHeaderBytes)
	}
}

func TestMaxResponseHeaderBytesCustomAndZero(t *testing.T) {
	client, err := New(WithMaxResponseHeaderBytes(4096))
	if err != nil {
		t.Fatal(err)
	}
	if tr := client.rt.(*http.Transport); tr.MaxResponseHeaderBytes != 4096 {
		t.Errorf("自定义响应头上限 = %d,want 4096", tr.MaxResponseHeaderBytes)
	}
	// 0 表示回退默认
	client, err = New(WithMaxResponseHeaderBytes(0))
	if err != nil {
		t.Fatal(err)
	}
	if tr := client.rt.(*http.Transport); tr.MaxResponseHeaderBytes != defaultMaxResponseHeaderBytes {
		t.Errorf("0 应回退默认:%d", tr.MaxResponseHeaderBytes)
	}
	if _, err := New(WithMaxResponseHeaderBytes(-1)); err == nil {
		t.Error("负数响应头上限应非法")
	}
}

func TestMaxResponseHeaderBytesBehavior(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Big", strings.Repeat("a", 4096))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New(WithMaxResponseHeaderBytes(1024))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("响应头超限应返回错误")
	}
}

func TestMaxConnsPerHostAndExpectContinue(t *testing.T) {
	client, err := New(
		WithMaxConnsPerHost(3),
		WithExpectContinueTimeout(500*time.Millisecond),
	)
	if err != nil {
		t.Fatal(err)
	}
	tr := client.rt.(*http.Transport)
	if tr.MaxConnsPerHost != 3 || tr.ExpectContinueTimeout != 500*time.Millisecond {
		t.Errorf("连接选项未生效:%+v", tr)
	}
	if _, err := New(WithMaxConnsPerHost(-1)); err == nil {
		t.Error("负数 MaxConnsPerHost 应非法")
	}
	if _, err := New(WithExpectContinueTimeout(-1)); err == nil {
		t.Error("负数 ExpectContinueTimeout 应非法")
	}
}

func TestHTTP2HeaderListSize(t *testing.T) {
	client, err := New(WithProtocol(ProtocolHTTP2), WithMaxResponseHeaderBytes(8192))
	if err != nil {
		t.Fatal(err)
	}
	tr := client.rt.(*http2.Transport)
	if tr.MaxHeaderListSize != 8192 {
		t.Errorf("H2 响应头上限 = %d,want 8192", tr.MaxHeaderListSize)
	}
}

func TestHeaderListSize(t *testing.T) {
	if got := headerListSize(0); got != 0 {
		t.Errorf("0 = %d,want 0", got)
	}
	if got := headerListSize(1024); got != 1024 {
		t.Errorf("1024 = %d", got)
	}
	if got := headerListSize(1 << 40); got != ^uint32(0) {
		t.Errorf("超限值 = %d,want %d", got, ^uint32(0))
	}
}

// ---------- 重试总时长 ----------

func TestRetryTotalTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client, err := New(WithRetryPolicy(RetryPolicy{
		MaxAttempts:  5,
		Backoff:      FixedBackoff(time.Second),
		TotalTimeout: 80 * time.Millisecond,
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("重试总时长应触发")
	}
	if kind := errx.KindOf(err); kind != errx.KindCancelled {
		t.Errorf("分类 = %s,want cancelled;err=%v", kind, err)
	}
}

func TestRetryTotalTimeoutEnough(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New(WithRetryPolicy(RetryPolicy{
		MaxAttempts:  2,
		Backoff:      FixedBackoff(10 * time.Millisecond),
		TotalTimeout: 2 * time.Second,
	}))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("总时长充足时应成功:%v", err)
	}
	_ = resp.Body.Close()
}

func TestRetryTotalTimeoutInvalid(t *testing.T) {
	if _, err := New(WithRetryPolicy(RetryPolicy{
		MaxAttempts:  2,
		Backoff:      FixedBackoff(time.Millisecond),
		TotalTimeout: -1,
	})); err == nil {
		t.Error("负数总时长应非法")
	}
}

func TestRetryExhaustedFields(t *testing.T) {
	ln := newClosedListener(t)
	client, err := New(WithRetry(2, FixedBackoff(time.Millisecond)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Get(context.Background(), "http://"+ln)
	if err == nil {
		t.Fatal("应返回重试耗尽")
	}
	e, ok := errx.As(err)
	if !ok {
		t.Fatalf("应为结构化错误:%v", err)
	}
	fields := e.Fields()
	var hasMethod, hasURL bool
	for _, f := range fields {
		switch f.Key {
		case "method":
			hasMethod = f.Value == http.MethodGet
		case "url":
			hasURL = strings.HasPrefix(f.Value.(string), "http://")
		}
	}
	if !hasMethod || !hasURL {
		t.Errorf("重试耗尽错误缺少 method/url 字段:%v", fields)
	}
}

// ---------- 请求 ID ----------

func TestRequestID(t *testing.T) {
	var gotID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID = r.Header.Get("X-Request-ID")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(context.Background(), srv.URL, WithRequestID("req-123"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if gotID != "req-123" {
		t.Errorf("X-Request-ID = %q,want req-123", gotID)
	}
}

func TestRequestIDEmptyIgnored(t *testing.T) {
	var gotID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID = r.Header.Get("X-Request-ID")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(context.Background(), srv.URL, WithRequestID(""))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if gotID != "" {
		t.Errorf("空请求 ID 不应写入:%q", gotID)
	}
}

func TestObserveRequestIDField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	logger := &fakeLogger{}
	client, err := New(WithLogger(logger), WithLogRequest(true))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(context.Background(), srv.URL, WithRequestID("req-abc"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if !logger.hasDebug("HTTP 请求") {
		t.Error("应输出请求日志(含 request_id 字段)")
	}
}

// FuzzRedirect 保证重定向请求构造对任意 Location 不 panic。
func FuzzRedirect(f *testing.F) {
	f.Add("http://example.com/next")
	f.Add("")
	f.Add("://bad")
	f.Add("%zz")
	f.Fuzz(func(t *testing.T, loc string) {
		req, err := http.NewRequest(http.MethodGet, "http://example.com/start", nil)
		if err != nil {
			t.Skip()
		}
		resp := &http.Response{
			StatusCode: http.StatusFound,
			Header:     make(http.Header),
			Request:    req,
		}
		resp.Header.Set("Location", loc)
		_, _ = redirectRequest(req, resp)
	})
}
