package httpx

import (
	"context"
	testx "github.com/lcylpzls/testx"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStatsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New()
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), srv.URL)
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()
	stats := client.Stats()
	if stats.TotalRequests != 1 || stats.ActiveRequests != 0 ||
		stats.TotalErrors != 0 || stats.Retries != 0 {
		t.Errorf("统计不符:%+v", stats)
	}
}

func TestStatsError(t *testing.T) {
	ln := newClosedListener(t)
	client, err := New()
	testx.RequireNoError(t, err)

	_, err = client.Get(context.Background(), "http://"+ln)
	testx.RequireError(t, err)

	stats := client.Stats()
	if stats.TotalRequests != 1 || stats.TotalErrors != 1 {
		t.Errorf("统计不符:%+v", stats)
	}
}

func TestStatsRetries(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New(WithRetry(2, FixedBackoff(10*time.Millisecond)))
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), srv.URL)
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()
	stats := client.Stats()
	if stats.TotalRequests != 2 || stats.Retries != 1 || stats.TotalErrors != 0 {
		t.Errorf("统计不符:%+v", stats)
	}
}

func TestStatsActiveDuringRequest(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New()
	testx.RequireNoError(t, err)

	var activeDuring atomic.Uint64
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		resp, err := client.Get(context.Background(), srv.URL)
		if err != nil {
			t.Errorf("请求失败:%v", err)
			return
		}
		_ = resp.Body.Close()
	}()
	// 等待请求进入阻塞状态。
	deadline := time.Now().Add(3 * time.Second)
	for client.Stats().ActiveRequests == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	activeDuring.Store(client.Stats().ActiveRequests)
	close(release)
	wg.Wait()
	if activeDuring.Load() != 1 {
		t.Errorf("请求进行中活跃数 = %d,want 1", activeDuring.Load())
	}
	if client.Stats().ActiveRequests != 0 {
		t.Errorf("请求完成后活跃数应为 0:%d", client.Stats().ActiveRequests)
	}
}

func TestStatsConcurrent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New()
	testx.RequireNoError(t, err)

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
	stats := client.Stats()
	if stats.TotalRequests != 8 || stats.ActiveRequests != 0 {
		t.Errorf("并发统计不符:%+v", stats)
	}
}

// newClosedListener 返回已关闭监听器的地址字符串。
func newClosedListener(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	testx.RequireNoError(t, err)

	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}
