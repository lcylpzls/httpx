package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lcylpzls/errx"
)

func TestHooksOnRequestAndResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var onRequest, onResponse atomic.Int32
	client, err := New(WithHooks(Hooks{
		OnRequest: func(*http.Request) error {
			onRequest.Add(1)
			return nil
		},
		OnResponse: func(*http.Response) error {
			onResponse.Add(1)
			return nil
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
	if onRequest.Load() != 1 || onResponse.Load() != 1 {
		t.Errorf("钩子调用次数不符:onRequest=%d onResponse=%d", onRequest.Load(), onResponse.Load())
	}
}

func TestHooksOnRequestError(t *testing.T) {
	client, err := New(WithHooks(Hooks{
		OnRequest: func(*http.Request) error {
			return errors.New("禁止请求")
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Do(context.Background(), req)
	if err == nil {
		t.Fatal("OnRequest 错误应终止请求")
	}
	if code, _ := errx.CodeOf(err); code != CodeRequestFailed {
		t.Errorf("错误码 = %s,want %s", code, CodeRequestFailed)
	}
}

func TestHooksOnResponseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New(WithHooks(Hooks{
		OnResponse: func(*http.Response) error {
			return errors.New("响应不合规")
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("OnResponse 错误应终止请求")
	}
	if code, _ := errx.CodeOf(err); code != CodeRequestFailed {
		t.Errorf("错误码 = %s,want %s", code, CodeRequestFailed)
	}
}

func TestHooksOnError(t *testing.T) {
	ln := newClosedListener(t)
	var onError atomic.Int32
	client, err := New(WithHooks(Hooks{
		OnError: func(error) { onError.Add(1) },
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Get(context.Background(), "http://"+ln)
	if err == nil {
		t.Fatal("应返回连接错误")
	}
	if onError.Load() != 1 {
		t.Errorf("OnError 调用次数 = %d,want 1", onError.Load())
	}
}

func TestHooksCalledPerRetryAttempt(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var onRequest atomic.Int32
	client, err := New(
		WithRetry(2, FixedBackoff(time.Millisecond)),
		WithHooks(Hooks{OnRequest: func(*http.Request) error {
			onRequest.Add(1)
			return nil
		}}),
	)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if onRequest.Load() != 2 {
		t.Errorf("OnRequest 应随每次尝试调用:次数 = %d", onRequest.Load())
	}
}

func TestHooksNilNoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	client, err := New(WithHooks(Hooks{}))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
}
