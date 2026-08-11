package core

import (
	"context"
	testx "github.com/lcylpzls/testx"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newBenchServer 返回返回固定小响应的测试服务器。
func newBenchServer(b *testing.B) *httptest.Server {
	b.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"id":1,"name":"tom"}`)
	}))
	b.Cleanup(srv.Close)
	return srv
}

// BenchmarkDoReuse 基准:httpx 复用连接请求(目标与裸 net/http 同量级)。
func BenchmarkDoReuse(b *testing.B) {
	srv := newBenchServer(b)
	client, err := New()
	testx.RequireNoError(b, err)

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	testx.RequireNoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := client.Do(ctx, req)
		testx.RequireNoError(b, err)

		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}

// BenchmarkNetHTTPReuse 基准:裸 net/http 复用连接(对照组)。
func BenchmarkNetHTTPReuse(b *testing.B) {
	srv := newBenchServer(b)
	client := &http.Client{}
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	testx.RequireNoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := client.Do(req)
		testx.RequireNoError(b, err)

		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}

// BenchmarkJSON 基准:JSON 响应助手解析 5 字段响应。
func BenchmarkJSON(b *testing.B) {
	srv := newBenchServer(b)
	client, err := New()
	testx.RequireNoError(b, err)

	ctx := context.Background()
	var out struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := client.Get(ctx, srv.URL)
		testx.RequireNoError(b, err)

		if err := JSON(resp, &out); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReadString 基准:响应体读取助手。
func BenchmarkReadString(b *testing.B) {
	srv := newBenchServer(b)
	client, err := New()
	testx.RequireNoError(b, err)

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := client.Get(ctx, srv.URL)
		testx.RequireNoError(b, err)

		if _, err := ReadString(resp, 1024); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBuildRequest 基准:请求构造与选项合并。
func BenchmarkBuildRequest(b *testing.B) {
	client, err := New()
	testx.RequireNoError(b, err)

	opts := []RequestOption{
		WithHeader("X-Test", "v"),
		WithQuery("q", "1"),
		WithUserAgent("bench"),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := client.buildRequest(context.Background(), http.MethodPost,
			"http://example.com/api", strings.NewReader("body"), opts); err != nil {
			b.Fatal(err)
		}
	}
}
