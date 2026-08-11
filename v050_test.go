package httpx

import (
	"context"
	testx "github.com/lcylpzls/testx"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lcylpzls/errx"
)

func TestVersion(t *testing.T) {
	testx.Equal(t, Version, "v1.3.1")

}

// ---------- EnsureStatus ----------

func TestEnsureStatusMatched(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("ok")),
	}
	if err := EnsureStatus(resp, http.StatusOK); err != nil {
		t.Fatalf("命中状态码应返回 nil:%v", err)
	}
	data, err := io.ReadAll(resp.Body)
	testx.RequireNoError(t, err)

	if string(data) != "ok" {
		t.Errorf("命中时不应关闭 Body:%q", data)
	}
}

func TestEnsureStatusMultipleCodes(t *testing.T) {
	for _, code := range []int{http.StatusOK, http.StatusAccepted} {
		resp := &http.Response{
			StatusCode: code,
			Body:       io.NopCloser(strings.NewReader("")),
		}
		if err := EnsureStatus(resp, http.StatusOK, http.StatusAccepted); err != nil {
			t.Errorf("状态码 %d 应在允许列表:%v", code, err)
		}
	}
}

func TestEnsureStatusMismatch(t *testing.T) {
	body := &closeRecorder{Reader: strings.NewReader("not found")}
	resp := &http.Response{
		StatusCode: http.StatusNotFound,
		Status:     "404 Not Found",
		Header:     make(http.Header),
		Body:       body,
	}
	err := EnsureStatus(resp, http.StatusOK)
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeUnexpectedStatus {
		t.Errorf("错误码 = %s,want %s", code, CodeUnexpectedStatus)
	}
	if !body.closed {
		t.Error("未命中时 Body 应被关闭")
	}
	e, ok := errx.As(err)
	testx.RequireTrue(t, ok)

	var hasStatus, hasBody bool
	for _, f := range e.Fields() {
		switch f.Key {
		case "status":
			hasStatus = f.Value == http.StatusNotFound
		case "body":
			hasBody = f.Value == "not found"
		}
	}
	if !hasStatus || !hasBody {
		t.Errorf("错误字段缺少 status/body:%v", e.Fields())
	}
}

func TestEnsureStatusNilResponse(t *testing.T) {
	if err := EnsureStatus(nil, http.StatusOK); err == nil {
		t.Fatal("nil 响应应返回错误")
	}
}

func TestEnsureStatusNilBody(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusTeapot}
	err := EnsureStatus(resp, http.StatusOK)
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeUnexpectedStatus {
		t.Errorf("错误码 = %s", code)
	}
}

func TestEnsureStatusSummaryTruncated(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", 4096))),
	}
	err := EnsureStatus(resp, http.StatusOK)
	testx.RequireError(t, err)

	e, ok := errx.As(err)
	testx.RequireTrue(t, ok)

	for _, f := range e.Fields() {
		if f.Key == "body" {
			if len(f.Value.(string)) > statusSummaryLimit {
				t.Errorf("摘要未截断:%d", len(f.Value.(string)))
			}
		}
	}
}

// ---------- 流式文件上传 ----------

func TestMultipartStreamingFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("解析 multipart 失败:%v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Errorf("读取文件字段失败:%v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		data, _ := io.ReadAll(f)
		_ = f.Close()
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	client, err := New()
	testx.RequireNoError(t, err)

	resp, err := client.Post(context.Background(), srv.URL, nil,
		WithMultipartFormData(nil, map[string]FileField{
			"file": {Filename: "stream.bin", Reader: strings.NewReader("streamed-content")},
		}))
	testx.RequireNoError(t, err)

	body := readRespBody(t, resp)
	testx.Equal(t, body, "streamed-content")

}

func TestMultipartReaderPrecedence(t *testing.T) {
	// Reader 非 nil 时忽略 Content。
	ro := requestOptions{}
	WithMultipartFormData(nil, map[string]FileField{
		"f": {Filename: "x", Content: []byte("content"), Reader: strings.NewReader("reader")},
	})(&ro)
	f := ro.formFiles["f"]
	if f.Reader == nil {
		t.Fatal("Reader 未保存")
	}
}

// ---------- MaxBackoff ----------

func TestRetryMaxBackoffCapsRetryAfter(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "100") // 若未截断会等 100 秒
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New(WithRetryPolicy(RetryPolicy{
		MaxAttempts: 2,
		Backoff:     FixedBackoff(time.Millisecond),
		MaxBackoff:  10 * time.Millisecond,
	}))
	testx.RequireNoError(t, err)

	start := time.Now()
	resp, err := client.Get(context.Background(), srv.URL)
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("MaxBackoff 未截断 Retry-After:耗时 %v", elapsed)
	}
}

func TestRetryMaxBackoffInvalid(t *testing.T) {
	if _, err := New(WithRetryPolicy(RetryPolicy{
		MaxAttempts: 2,
		Backoff:     FixedBackoff(time.Millisecond),
		MaxBackoff:  -1,
	})); err == nil {
		t.Error("负数 MaxBackoff 应非法")
	}
}

func TestRetryMaxBackoffAllowsLongerBackoff(t *testing.T) {
	// MaxBackoff 大于策略退避时不截断。
	client, err := New(WithRetryPolicy(RetryPolicy{
		MaxAttempts: 2,
		Backoff:     FixedBackoff(time.Millisecond),
		MaxBackoff:  time.Second,
	}))
	testx.RequireNoError(t, err)

	if client.cfg.retry == nil || client.cfg.retry.maxBackoff != time.Second {
		t.Error("MaxBackoff 配置未生效")
	}
}

func TestErrorsRegisteredV050(t *testing.T) {
	if errx.Describe(CodeUnexpectedStatus) == "" {
		t.Error("HTX_UNEXPECTED_STATUS 未注册")
	}
}
