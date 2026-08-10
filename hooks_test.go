package httpx

import (
	"context"
	"errors"
	testx "github.com/lcylpzls/testx"
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
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), srv.URL)
	testx.RequireNoError(t, err)

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
	testx.RequireNoError(t, err)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	testx.RequireNoError(t, err)

	_, err = client.Do(context.Background(), req)
	testx.RequireError(t, err)

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
	testx.RequireNoError(t, err)

	_, err = client.Get(context.Background(), srv.URL)
	testx.RequireError(t, err)

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
	testx.RequireNoError(t, err)

	_, err = client.Get(context.Background(), "http://"+ln)
	testx.RequireError(t, err)

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
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), srv.URL)
	testx.RequireNoError(t, err)

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
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), srv.URL)
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()
}
