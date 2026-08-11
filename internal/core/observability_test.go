package core

import (
	"context"
	testx "github.com/lcylpzls/testx"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestObserveMetrics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := newFakeMetrics()
	client, err := New(WithMetrics(m))
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), srv.URL)
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()

	if m.counter(metricRequests, http.MethodGet) != 1 {
		t.Errorf("requests 计数 = %d,want 1", m.counter(metricRequests, http.MethodGet))
	}
	if len(m.durations[metricDuration+"|"+http.MethodGet]) != 1 {
		t.Error("duration 未记录")
	}
	if m.counter(metricErrors, http.MethodGet) != 0 {
		t.Error("成功请求不应计错误")
	}
	if m.counter(metricRetries, http.MethodGet) != 0 {
		t.Error("无重试不应计 retries")
	}
	if m.counter(metricSlowRequests, http.MethodGet) != 0 {
		t.Error("快速请求不应计 slow")
	}
}

func TestObserveErrorMetrics(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	testx.RequireNoError(t, err)

	addr := ln.Addr().String()
	_ = ln.Close()

	m := newFakeMetrics()
	client, err := New(WithMetrics(m))
	testx.RequireNoError(t, err)

	_, err = client.Get(context.Background(), "http://"+addr)
	testx.RequireError(t, err)

	if m.counter(metricErrors, http.MethodGet) != 1 {
		t.Errorf("errors 计数 = %d,want 1", m.counter(metricErrors, http.MethodGet))
	}
}

func TestObserveRetryMetrics(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := newFakeMetrics()
	client, err := New(WithRetry(2, FixedBackoff(time.Millisecond)), WithMetrics(m))
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), srv.URL)
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()

	if m.counter(metricRequests, http.MethodGet) != 2 {
		t.Errorf("requests 计数 = %d,want 2", m.counter(metricRequests, http.MethodGet))
	}
	if m.counter(metricRetries, http.MethodGet) != 1 {
		t.Errorf("retries 计数 = %d,want 1", m.counter(metricRetries, http.MethodGet))
	}
}

func TestObserveSlowMetrics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := newFakeMetrics()
	client, err := New(WithMetrics(m), WithSlowThreshold(time.Millisecond))
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), srv.URL)
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()

	if m.counter(metricSlowRequests, http.MethodGet) != 1 {
		t.Errorf("slow 计数 = %d,want 1", m.counter(metricSlowRequests, http.MethodGet))
	}
}

func TestObserveZeroSlowThresholdFallsBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := newFakeMetrics()
	client, err := New(WithMetrics(m), WithSlowThreshold(0))
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), srv.URL)
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()

	if m.counter(metricSlowRequests, http.MethodGet) != 0 {
		t.Error("默认 100ms 阈值下快速请求不应计 slow")
	}
}

func TestObserveLogs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	logger := &fakeLogger{}
	client, err := New(
		WithLogger(logger),
		WithLogRequest(true),
		WithSlowThreshold(time.Millisecond),
	)
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), srv.URL)
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()

	if !logger.hasDebug("HTTP 请求") {
		t.Error("应输出请求摘要日志")
	}
	if !logger.hasWarn("慢请求") {
		t.Error("应输出慢请求日志")
	}
}

func TestObserveErrorLogs(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	testx.RequireNoError(t, err)

	addr := ln.Addr().String()
	_ = ln.Close()

	logger := &fakeLogger{}
	client, err := New(WithLogger(logger))
	testx.RequireNoError(t, err)

	_, err = client.Get(context.Background(), "http://"+addr)
	testx.RequireError(t, err)

	if !logger.hasWarn("HTTP 请求失败") {
		t.Error("应输出请求失败日志")
	}
}

func TestObserveWithoutLogRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	logger := &fakeLogger{}
	client, err := New(WithLogger(logger))
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), srv.URL)
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()

	if logger.hasDebug("HTTP 请求") {
		t.Error("未开启 LogRequest 不应输出摘要日志")
	}
}
