package httpx

import (
	"bytes"
	"context"
	"errors"
	testx "github.com/lcylpzls/testx"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lcylpzls/errx"
)

func TestRetryableStatus(t *testing.T) {
	cases := []struct {
		code int
		want bool
	}{
		{http.StatusOK, false},
		{http.StatusNotFound, false},
		{http.StatusTooManyRequests, true},
		{http.StatusInternalServerError, true},
		{http.StatusBadGateway, true},
		{http.StatusServiceUnavailable, true},
		{http.StatusGatewayTimeout, true},
		{http.StatusNotImplemented, false},
	}
	for _, tc := range cases {
		if got := retryableStatus(tc.code); got != tc.want {
			t.Errorf("%d:retryableStatus = %v,want %v", tc.code, got, tc.want)
		}
	}
}

func TestIdempotentMethod(t *testing.T) {
	cases := []struct {
		method string
		want   bool
	}{
		{http.MethodGet, true},
		{http.MethodHead, true},
		{http.MethodOptions, true},
		{http.MethodPut, true},
		{http.MethodDelete, true},
		{http.MethodPost, false},
		{http.MethodPatch, false},
		{"CUSTOM", false},
	}
	for _, tc := range cases {
		if got := idempotentMethod(tc.method); got != tc.want {
			t.Errorf("%s:idempotentMethod = %v,want %v", tc.method, got, tc.want)
		}
	}
}

func TestExponentialBackoff(t *testing.T) {
	// jitter=0:确定性指数序列
	b := ExponentialBackoff(100*time.Millisecond, 2, 0)
	if got := b(1); got != 100*time.Millisecond {
		t.Errorf("attempt 1 = %v,want 100ms", got)
	}
	if got := b(2); got != 200*time.Millisecond {
		t.Errorf("attempt 2 = %v,want 200ms", got)
	}
	if got := b(3); got != 400*time.Millisecond {
		t.Errorf("attempt 3 = %v,want 400ms", got)
	}
	// attempt < 1 归一为 1
	if got := b(0); got != 100*time.Millisecond {
		t.Errorf("attempt 0 = %v,want 100ms", got)
	}
	// 非法参数回退
	bad := ExponentialBackoff(-1, 0, -1)
	if got := bad(1); got != defaultBackoffBase {
		t.Errorf("非法参数回退失败:%v", got)
	}
	// jitter=1:结果在 [0, 2*base] 范围内
	jittered := ExponentialBackoff(100*time.Millisecond, 2, 1)
	for i := 0; i < 50; i++ {
		d := jittered(1)
		if d < 0 || d > 2*100*time.Millisecond {
			t.Fatalf("抖动结果越界:%v", d)
		}
	}
	// jitter > 1 归一为 1
	over := ExponentialBackoff(100*time.Millisecond, 2, 5)
	for i := 0; i < 20; i++ {
		if d := over(1); d < 0 || d > 2*100*time.Millisecond {
			t.Fatalf("jitter 越界归一失败:%v", d)
		}
	}
	// 溢出保护:超大 attempt 不产生负值/Inf
	huge := ExponentialBackoff(time.Nanosecond, 10, 0)
	if d := huge(10000); d != time.Duration(^uint64(0)>>1) {
		t.Errorf("溢出保护失败:%v", d)
	}
	// NaN 因子:归一为 2,避免 NaN
	nanB := ExponentialBackoff(100*time.Millisecond, 0, 0)
	if d := nanB(2); d != 200*time.Millisecond {
		t.Errorf("NaN 因子归一失败:%v", d)
	}
	// 显式 NaN 因子:命中 IsNaN 保护,回退 base
	nanBase := ExponentialBackoff(100*time.Millisecond, math.NaN(), 0)
	if d := nanBase(3); d != 100*time.Millisecond {
		t.Errorf("NaN 保护失败:%v", d)
	}
}

func TestFixedBackoff(t *testing.T) {
	b := FixedBackoff(50 * time.Millisecond)
	if got := b(1); got != 50*time.Millisecond {
		t.Errorf("固定退避 = %v,want 50ms", got)
	}
	if got := b(99); got != 50*time.Millisecond {
		t.Errorf("固定退避应不随 attempt 变化:%v", got)
	}
	bad := FixedBackoff(0)
	if got := bad(1); got != defaultBackoffBase {
		t.Errorf("非法间隔回退失败:%v", got)
	}
}

func TestRetryAfter(t *testing.T) {
	fallback := 5 * time.Second
	cases := []struct {
		name string
		val  string
		want time.Duration
	}{
		{"空", "", fallback},
		{"秒数", "3", 3 * time.Second},
		{"零秒", "0", 0},
		{"带空格", " 7 ", 7 * time.Second},
		{"非法文本", "abc", fallback},
		{"负秒数", "-1", fallback},
		{"过期日期", time.Now().UTC().Add(-time.Minute).Format(http.TimeFormat), fallback},
		{"未来日期", time.Now().UTC().Add(2 * time.Second).Format(http.TimeFormat), 2 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := make(http.Header)
			h.Set("Retry-After", tc.val)
			got := retryAfter(h, fallback)
			// 日期分支允许 ±500ms 误差
			if tc.name == "未来日期" {
				if got <= 0 || got > 3*time.Second {
					t.Errorf("日期解析 = %v,want ~2s", got)
				}
				return
			}
			testx.Equal(t, got, tc.want)

		})
	}
}

func TestRetrySuccessAfter503(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New(WithRetry(3, FixedBackoff(time.Millisecond)))
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), srv.URL)
	testx.RequireNoError(t, err)

	defer resp.Body.Close()
	testx.Equal(t, resp.StatusCode, http.StatusOK)

	if hits.Load() != 3 {
		t.Errorf("请求次数 = %d,want 3", hits.Load())
	}
}

func TestRetryNetworkErrorExhausted(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	testx.RequireNoError(t, err)

	addr := ln.Addr().String()
	_ = ln.Close()

	client, err := New(WithRetry(3, FixedBackoff(time.Millisecond)))
	testx.RequireNoError(t, err)

	_, err = client.Get(context.Background(), "http://"+addr)
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeRetryExhausted {
		t.Errorf("错误码 = %s,want %s", code, CodeRetryExhausted)
	}
}

func TestRetryStatusExhausted(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client, err := New(WithRetry(3, FixedBackoff(time.Millisecond)))
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), srv.URL)
	testx.RequireNoError(t, err)

	defer resp.Body.Close()
	testx.Equal(t, resp.StatusCode, http.StatusServiceUnavailable)

	if hits.Load() != 3 {
		t.Errorf("请求次数 = %d,want 3", hits.Load())
	}
}

func TestRetryNonIdempotent(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client, err := New(WithRetry(3, FixedBackoff(time.Millisecond)))
	testx.RequireNoError(t, err)

	resp, err := client.Post(context.Background(), srv.URL, "x")
	testx.RequireNoError(t, err)

	defer resp.Body.Close()
	testx.Equal(t, resp.StatusCode, http.StatusInternalServerError)

	if hits.Load() != 1 {
		t.Errorf("非幂等请求不应重试:次数 = %d", hits.Load())
	}
}

func TestRetryBodyUnreadable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client, err := New(WithRetry(2, FixedBackoff(time.Millisecond)))
	testx.RequireNoError(t, err)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	testx.RequireNoError(t, err)

	// 手动 Body:GetBody 为空且不可 Seek,重试时应报 HTX_BODY_UNREADABLE。
	req.Body = io.NopCloser(strings.NewReader("x"))
	_, err = client.Do(context.Background(), req)
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeBodyUnreadable {
		t.Errorf("错误码 = %s,want %s", code, CodeBodyUnreadable)
	}
}

func TestRetryBodyReplayable(t *testing.T) {
	var bodies []string
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(data))
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New(WithRetry(2, FixedBackoff(time.Millisecond)))
	testx.RequireNoError(t, err)

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPut, srv.URL, bytes.NewReader([]byte("payload")))
	testx.RequireNoError(t, err)

	resp, err := client.Do(context.Background(), req)
	testx.RequireNoError(t, err)

	defer resp.Body.Close()
	testx.Equal(t, resp.StatusCode, http.StatusOK)

	if len(bodies) != 2 || bodies[0] != "payload" || bodies[1] != "payload" {
		t.Errorf("重试请求体不符:%v", bodies)
	}
}

func TestRetryCancelDuringBackoff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client, err := New(WithRetry(3, FixedBackoff(time.Second)))
	testx.RequireNoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = client.Get(ctx, srv.URL)
	testx.RequireError(t, err)

	if kind := errx.KindOf(err); kind != errx.KindCancelled {
		t.Errorf("分类 = %s,want cancelled;err=%v", kind, err)
	}
}

func TestRetryAfterHeaderRespected(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New(WithRetry(2, FixedBackoff(time.Second)))
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), srv.URL)
	testx.RequireNoError(t, err)

	defer resp.Body.Close()
	testx.Equal(t, resp.StatusCode, http.StatusOK)

}

func TestRetryDisabledByDefault(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client, err := New()
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), srv.URL)
	testx.RequireNoError(t, err)

	defer resp.Body.Close()
	if hits.Load() != 1 {
		t.Errorf("默认不应重试:次数 = %d", hits.Load())
	}
}

func TestDoRetryNonRetryableError(t *testing.T) {
	rt := &scriptedRT{results: []roundTripResult{
		{err: errx.New(errx.KindBusiness, "BIZ", "业务错误")},
	}}
	client, err := New(WithRetry(3, FixedBackoff(time.Millisecond)))
	testx.RequireNoError(t, err)

	client.rt = rt
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	_, err = client.Do(context.Background(), req)
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != "BIZ" {
		t.Errorf("错误码 = %s,want BIZ", code)
	}
	if rt.calls != 1 {
		t.Errorf("不可重试错误不应重试:次数 = %d", rt.calls)
	}
}

func TestDoRetryNonIdempotentError(t *testing.T) {
	rt := &scriptedRT{results: []roundTripResult{
		{err: fakeNetError{timeout: true}},
	}}
	client, err := New(WithRetry(3, FixedBackoff(time.Millisecond)))
	testx.RequireNoError(t, err)

	client.rt = rt
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com", nil)
	_, err = client.Do(context.Background(), req)
	testx.RequireError(t, err)

	if rt.calls != 1 {
		t.Errorf("非幂等方法不应重试:次数 = %d", rt.calls)
	}
}

func TestDoRetryReplayableBodyAcrossAttempts(t *testing.T) {
	rt := &scriptedRT{results: []roundTripResult{
		{resp: statusResponse(http.StatusServiceUnavailable)},
		{resp: statusResponse(http.StatusOK)},
	}}
	client, err := New(WithRetry(2, FixedBackoff(time.Millisecond)))
	testx.RequireNoError(t, err)

	client.rt = rt
	req, _ := http.NewRequestWithContext(
		context.Background(), http.MethodPut, "http://example.com", bytes.NewReader([]byte("p")))
	resp, err := client.Do(context.Background(), req)
	testx.RequireNoError(t, err)

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || rt.calls != 2 {
		t.Errorf("重试结果不符:status=%d calls=%d", resp.StatusCode, rt.calls)
	}
}

// roundTripResult 是脚本化 RoundTripper 的一次结果。
type roundTripResult struct {
	resp *http.Response
	err  error
}

// scriptedRT 按顺序返回预设结果,超出时重复最后一条。
type scriptedRT struct {
	mu      sync.Mutex
	results []roundTripResult
	calls   int
}

func (r *scriptedRT) RoundTrip(*http.Request) (*http.Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	idx := r.calls
	if idx >= len(r.results) {
		idx = len(r.results) - 1
	}
	r.calls++
	return r.results[idx].resp, r.results[idx].err
}

// statusResponse 构造指定状态码的响应(空 Body)。
func statusResponse(code int) *http.Response {
	return &http.Response{
		StatusCode: code,
		Status:     http.StatusText(code),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("")),
	}
}

func TestCloneRequestForRetry(t *testing.T) {
	// 无 Body
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	clone, err := cloneRequestForRetry(req)
	if err != nil || clone == nil {
		t.Fatalf("无 Body 克隆失败:%v", err)
	}
	// GetBody 自动设置(标准库为 bytes.Reader 生成)
	req, _ = http.NewRequest(http.MethodPut, "http://example.com", bytes.NewReader([]byte("x")))
	if req.GetBody == nil {
		t.Fatal("bytes.Reader 应自动生成 GetBody")
	}
	clone, err = cloneRequestForRetry(req)
	testx.RequireNoError(t, err)

	data, _ := io.ReadAll(clone.Body)
	if string(data) != "x" {
		t.Errorf("克隆请求体不符:%q", data)
	}
	// GetBody 返回错误
	req, _ = http.NewRequest(http.MethodPut, "http://example.com", nil)
	req.Body = io.NopCloser(strings.NewReader("x"))
	req.GetBody = func() (io.ReadCloser, error) {
		return nil, errors.New("重建失败")
	}
	if _, err := cloneRequestForRetry(req); err == nil {
		t.Error("GetBody 失败应返回错误")
	}
	// io.ReadSeeker 手动 Body
	rs := bytes.NewReader([]byte("y"))
	req, _ = http.NewRequest(http.MethodPut, "http://example.com", nil)
	req.Body = seekReadCloser{rs}
	req.GetBody = nil
	clone, err = cloneRequestForRetry(req)
	testx.RequireNoError(t, err)

	data, _ = io.ReadAll(clone.Body)
	if string(data) != "y" {
		t.Errorf("ReadSeeker 克隆请求体不符:%q", data)
	}
	// Seek 失败
	req, _ = http.NewRequest(http.MethodPut, "http://example.com", nil)
	req.Body = &failSeeker{}
	req.GetBody = nil
	if _, err := cloneRequestForRetry(req); err == nil {
		t.Error("Seek 失败应返回错误")
	}
	// 不可重读
	req, _ = http.NewRequest(http.MethodPut, "http://example.com", nil)
	req.Body = io.NopCloser(strings.NewReader("z"))
	req.GetBody = nil
	_, err = cloneRequestForRetry(req)
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeBodyUnreadable {
		t.Errorf("错误码 = %s,want %s", code, CodeBodyUnreadable)
	}
}

// seekReadCloser 是同时实现 ReadSeeker 与 Close 的请求体。
type seekReadCloser struct {
	*bytes.Reader
}

func (seekReadCloser) Close() error { return nil }

// failSeeker 实现 io.ReadSeeker,Seek 恒失败。
type failSeeker struct{}

func (*failSeeker) Read([]byte) (int, error) { return 0, io.EOF }
func (*failSeeker) Seek(int64, int) (int64, error) {
	return 0, errors.New("seek 失败")
}
func (*failSeeker) Close() error { return nil }

// FuzzBackoff 保证退避策略对任意参数不 panic、不产生负等待。
func FuzzBackoff(f *testing.F) {
	f.Add(int64(100000000), 2.0, 0.5, 5)
	f.Add(int64(-1), 0.0, 1.5, -3)
	f.Fuzz(func(t *testing.T, baseNs int64, factor, jitter float64, attempt int) {
		b := ExponentialBackoff(time.Duration(baseNs), factor, jitter)
		if d := b(attempt); d < 0 {
			t.Fatalf("负等待时长:%v", d)
		}
		fb := FixedBackoff(time.Duration(baseNs))
		if d := fb(attempt); d < 0 {
			t.Fatalf("固定退避负值:%v", d)
		}
	})
}
