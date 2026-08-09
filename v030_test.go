package httpx

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lcylpzls/errx"
	"golang.org/x/net/http2"
)

// ---------- 请求级超时 ----------

func TestRequestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Get(context.Background(), srv.URL, WithRequestTimeout(80*time.Millisecond))
	if err == nil {
		t.Fatal("请求级超时应触发")
	}
	if !IsTimeout(err) {
		t.Errorf("应为超时错误:%v", err)
	}
}

func TestRequestTimeoutStricterThanClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New(WithTimeout(250 * time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Get(context.Background(), srv.URL, WithRequestTimeout(80*time.Millisecond))
	if err == nil {
		t.Fatal("请求级超时更严格时应生效")
	}
}

func TestClientTimeoutStricterThanRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New(WithTimeout(80 * time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Get(context.Background(), srv.URL, WithRequestTimeout(250*time.Millisecond))
	if err == nil {
		t.Fatal("客户端级超时更严格时应生效")
	}
}

func TestRequestTimeoutZeroIgnored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(context.Background(), srv.URL, WithRequestTimeout(0))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
}

// ---------- 自定义重试策略 ----------

func TestRetryPolicyCustomRetryable(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusPaymentRequired) // 402,默认不可重试
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New(WithRetryPolicy(RetryPolicy{
		MaxAttempts: 2,
		Backoff:     FixedBackoff(time.Millisecond),
		Retryable: func(_ *http.Request, resp *http.Response, _ error) bool {
			return resp != nil && resp.StatusCode == http.StatusPaymentRequired
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("自定义重试应成功:%v", err)
	}
	_ = resp.Body.Close()
	if hits.Load() != 2 {
		t.Errorf("请求次数 = %d,want 2", hits.Load())
	}
}

func TestRetryPolicyCustomAllowsPOST(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New(WithRetryPolicy(RetryPolicy{
		MaxAttempts: 2,
		Backoff:     FixedBackoff(time.Millisecond),
		Retryable: func(_ *http.Request, resp *http.Response, _ error) bool {
			return resp != nil && resp.StatusCode >= 500
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Post(context.Background(), srv.URL, "x")
	if err != nil {
		t.Fatalf("自定义策略应允许 POST 重试:%v", err)
	}
	_ = resp.Body.Close()
	if hits.Load() != 2 {
		t.Errorf("POST 重试次数 = %d,want 2", hits.Load())
	}
}

func TestRetryPolicyCustomRejects(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client, err := New(WithRetryPolicy(RetryPolicy{
		MaxAttempts: 3,
		Backoff:     FixedBackoff(time.Millisecond),
		Retryable: func(*http.Request, *http.Response, error) bool {
			return false
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if hits.Load() != 1 {
		t.Errorf("拒绝重试时请求次数 = %d,want 1", hits.Load())
	}
}

func TestRetryPolicyInvalid(t *testing.T) {
	if _, err := New(WithRetryPolicy(RetryPolicy{MaxAttempts: 0, Backoff: FixedBackoff(time.Millisecond)})); err == nil {
		t.Error("MaxAttempts=0 应非法")
	}
	if _, err := New(WithRetryPolicy(RetryPolicy{MaxAttempts: 2})); err == nil {
		t.Error("nil Backoff 应非法")
	}
}

func TestShouldRetryNoPolicy(t *testing.T) {
	client, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if client.shouldRetry(nil, nil, errors.New("x")) {
		t.Error("无重试策略不应重试")
	}
}

// ---------- DNS 缓存 ----------

// fakeResolver 是可控的 ipResolver 实现。
type fakeResolver struct {
	mu    sync.Mutex
	calls int
	ips   []net.IPAddr
	err   error
}

func (f *fakeResolver) LookupIPAddr(_ context.Context, _ string) ([]net.IPAddr, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return append([]net.IPAddr(nil), f.ips...), nil
}

func (f *fakeResolver) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestNewDNSCacheDefaultTTL(t *testing.T) {
	cache := NewDNSCache(0)
	if cache.ttl != defaultDNSTTL {
		t.Errorf("默认 TTL = %v,want %v", cache.ttl, defaultDNSTTL)
	}
}

func TestDNSCacheHitAndExpire(t *testing.T) {
	resolver := &fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}}
	cache := NewDNSCache(5 * time.Millisecond)
	cache.resolver = resolver
	ctx := context.Background()

	ips, err := cache.LookupIPAddr(ctx, "a.example")
	if err != nil || len(ips) != 1 {
		t.Fatalf("首次解析失败:%v %v", ips, err)
	}
	if resolver.count() != 1 {
		t.Errorf("首次解析应调用 resolver")
	}
	// 缓存命中
	if _, err := cache.LookupIPAddr(ctx, "a.example"); err != nil {
		t.Fatal(err)
	}
	if resolver.count() != 1 {
		t.Errorf("缓存命中不应再解析:次数 = %d", resolver.count())
	}
	// TTL 过期
	time.Sleep(10 * time.Millisecond)
	if _, err := cache.LookupIPAddr(ctx, "a.example"); err != nil {
		t.Fatal(err)
	}
	if resolver.count() != 2 {
		t.Errorf("过期后应重新解析:次数 = %d", resolver.count())
	}
}

func TestDNSCacheResolverError(t *testing.T) {
	resolver := &fakeResolver{err: errors.New("解析失败")}
	cache := NewDNSCache(time.Hour)
	cache.resolver = resolver
	if _, err := cache.LookupIPAddr(context.Background(), "bad.example"); err == nil {
		t.Fatal("解析失败应返回错误")
	}
	if len(cache.entries) != 0 {
		t.Error("失败结果不应缓存")
	}
}

func TestDNSCacheReset(t *testing.T) {
	resolver := &fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}}
	cache := NewDNSCache(time.Hour)
	cache.resolver = resolver
	_, _ = cache.LookupIPAddr(context.Background(), "a.example")
	cache.Reset()
	_, _ = cache.LookupIPAddr(context.Background(), "a.example")
	if resolver.count() != 2 {
		t.Errorf("Reset 后应重新解析:次数 = %d", resolver.count())
	}
}

func TestDNSCacheConcurrent(t *testing.T) {
	resolver := &fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}}
	cache := NewDNSCache(time.Hour)
	cache.resolver = resolver
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_, _ = cache.LookupIPAddr(context.Background(), "a.example")
				if j%5 == 0 {
					cache.Reset()
				}
			}
		}()
	}
	wg.Wait()
}

func TestDialWithDNSCache(t *testing.T) {
	dialer := &net.Dialer{Timeout: time.Second}
	ctx := context.Background()
	// IP 直连跳过解析
	cache := NewDNSCache(time.Hour)
	cache.resolver = &fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}}
	conn, err := dialWithDNSCache(ctx, dialer, cache, "tcp", "127.0.0.1:9")
	if err == nil {
		_ = conn.Close()
	}
	if cache.resolver.(*fakeResolver).count() != 0 {
		t.Error("IP 直连不应触发解析")
	}
	// 解析失败回退系统解析
	cache2 := NewDNSCache(time.Hour)
	cache2.resolver = &fakeResolver{err: errors.New("解析失败")}
	_, err = dialWithDNSCache(ctx, dialer, cache2, "tcp", "no-such-host-httpx-test.invalid:9")
	if err == nil {
		t.Error("解析失败且系统解析也不存在时应返回错误")
	}
	// 空解析结果回退系统解析
	cacheEmpty := NewDNSCache(time.Hour)
	cacheEmpty.resolver = &fakeResolver{ips: []net.IPAddr{}}
	_, err = dialWithDNSCache(ctx, dialer, cacheEmpty, "tcp", "no-such-host-httpx-test.invalid:9")
	if err == nil {
		t.Error("空解析结果回退失败时应返回错误")
	}
	// 非法地址
	if _, err := dialWithDNSCache(ctx, dialer, cache, "tcp", "bad-addr"); err == nil {
		t.Error("非法地址应返回错误")
	}
	// 逐 IP 失败后回退系统解析(利用 localhost 可达)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	cache3 := NewDNSCache(time.Hour)
	cache3.resolver = &fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("127.0.0.2")}}}
	conn, err = dialWithDNSCache(ctx, dialer, cache3, "tcp", net.JoinHostPort("localhost", port))
	if err != nil {
		t.Fatalf("回退系统解析应成功:%v", err)
	}
	_ = conn.Close()
}

func TestDNSCacheIntegration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())

	resolver := &fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}}
	cache := NewDNSCache(time.Hour)
	cache.resolver = resolver
	client, err := New(WithDNSCache(cache))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		resp, err := client.Get(context.Background(), "http://test.local:"+port)
		if err != nil {
			t.Fatalf("第 %d 次请求失败:%v", i+1, err)
		}
		_ = resp.Body.Close()
	}
	if resolver.count() != 1 {
		t.Errorf("DNS 解析次数 = %d,want 1(缓存命中)", resolver.count())
	}
}

// ---------- 并发限流 ----------

func TestMaxConcurrency(t *testing.T) {
	var active, peak atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		cur := active.Add(1)
		for {
			p := peak.Load()
			if cur <= p || peak.CompareAndSwap(p, cur) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		active.Add(-1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New(WithMaxConcurrency(2))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Get(context.Background(), srv.URL)
			if err == nil {
				_ = resp.Body.Close()
			}
		}()
	}
	wg.Wait()
	if peak.Load() > 2 {
		t.Errorf("并发峰值 = %d,超过上限 2", peak.Load())
	}
}

func TestMaxConcurrencyCancelWhileWaiting(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New(WithMaxConcurrency(1))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		resp, err := client.Get(context.Background(), srv.URL)
		if err == nil {
			_ = resp.Body.Close()
		}
		close(done)
	}()
	// 等第一个请求占用许可
	deadline := time.Now().Add(3 * time.Second)
	for client.Stats().ActiveRequests == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = client.Get(ctx, srv.URL)
	close(release)
	<-done
	if err == nil {
		t.Fatal("等待许可被取消应返回错误")
	}
	if kind := errx.KindOf(err); kind != errx.KindCancelled {
		t.Errorf("分类 = %s,want cancelled", kind)
	}
}

func TestMaxConcurrencyInvalid(t *testing.T) {
	if _, err := New(WithMaxConcurrency(-1)); err == nil {
		t.Error("负数并发上限应非法")
	}
}

// ---------- HTTP/2 健康检查 ----------

func TestHTTP2HealthCheck(t *testing.T) {
	client, err := New(
		WithProtocol(ProtocolHTTP2),
		WithHTTP2HealthCheck(30*time.Second, 5*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	tr := client.rt.(*http2.Transport)
	if tr.ReadIdleTimeout != 30*time.Second || tr.PingTimeout != 5*time.Second {
		t.Errorf("健康检查参数未生效:%+v", tr)
	}
	if _, err := New(WithHTTP2HealthCheck(-1, 0)); err == nil {
		t.Error("负数读空闲超时应非法")
	}
	if _, err := New(WithHTTP2HealthCheck(0, -1)); err == nil {
		t.Error("负数 Ping 超时应非法")
	}
}

// ---------- 流式响应 ----------

// chunkReader 按块返回数据,模拟流式响应体。
type chunkReader struct {
	chunks [][]byte
	idx    int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if r.idx >= len(r.chunks) {
		return 0, io.EOF
	}
	chunk := r.chunks[r.idx]
	r.idx++
	return copy(p, chunk), nil
}

func (r *chunkReader) Close() error { return nil }

func TestReadStream(t *testing.T) {
	body := &chunkReader{chunks: [][]byte{
		bytes.Repeat([]byte("a"), 20*1024),
		bytes.Repeat([]byte("b"), 20*1024),
		[]byte("c"),
	}}
	var received []byte
	var chunks int
	resp := &http.Response{Body: body}
	err := ReadStream(resp, func(chunk []byte) error {
		chunks++
		received = append(received, chunk...)
		return nil
	}, 1<<20)
	if err != nil {
		t.Fatalf("流式读取失败:%v", err)
	}
	if len(received) != 40*1024+1 || chunks != 3 {
		t.Errorf("读取结果不符:len=%d chunks=%d", len(received), chunks)
	}
}

func TestReadStreamTooLarge(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(strings.NewReader("hello"))}
	var chunks int
	err := ReadStream(resp, func([]byte) error {
		chunks++
		return nil
	}, 3)
	if err == nil {
		t.Fatal("超限应返回错误")
	}
	if code, _ := errx.CodeOf(err); code != CodeBodyTooLarge {
		t.Errorf("错误码 = %s,want %s", code, CodeBodyTooLarge)
	}
	if chunks != 0 {
		t.Errorf("超限时回调不应触发:chunks = %d", chunks)
	}
}

func TestReadStreamCallbackError(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(strings.NewReader("x"))}
	cbErr := errors.New("回调终止")
	err := ReadStream(resp, func([]byte) error { return cbErr }, 1024)
	if err == nil {
		t.Fatal("回调错误应返回")
	}
	if code, _ := errx.CodeOf(err); code != CodeResponseFailed {
		t.Errorf("错误码 = %s,want %s", code, CodeResponseFailed)
	}
	if !errors.Is(err, cbErr) {
		t.Error("应保留回调原始错误")
	}
}

func TestReadStreamReadError(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(&errorReader{})}
	err := ReadStream(resp, func([]byte) error { return nil }, 1024)
	if err == nil {
		t.Fatal("读取错误应返回")
	}
	if code, _ := errx.CodeOf(err); code != CodeResponseFailed {
		t.Errorf("错误码 = %s,want %s", code, CodeResponseFailed)
	}
}

func TestReadStreamEdgeCases(t *testing.T) {
	if err := ReadStream(nil, func([]byte) error { return nil }, 1024); err == nil {
		t.Error("nil 响应应返回错误")
	}
	for _, limit := range []int64{0, -1} {
		if err := ReadStream(&http.Response{}, func([]byte) error { return nil }, limit); err == nil {
			t.Errorf("limit=%d 应返回错误", limit)
		}
	}
	if err := ReadStream(&http.Response{}, func([]byte) error { return nil }, 1024); err != nil {
		t.Errorf("nil Body 应直接返回:%v", err)
	}
}

func TestReadStreamClosesBody(t *testing.T) {
	body := &closeRecorder{Reader: strings.NewReader("x")}
	if err := ReadStream(&http.Response{Body: body}, func([]byte) error { return nil }, 1024); err != nil {
		t.Fatal(err)
	}
	if !body.closed {
		t.Error("ReadStream 应关闭 Body")
	}
}
