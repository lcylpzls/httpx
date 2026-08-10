package httpx

import (
	"bytes"
	"context"
	"errors"
	testx "github.com/lcylpzls/testx"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/lcylpzls/errx"
)

// redirectRecorder 记录重定向目标收到的请求特征。
type redirectRecorder struct {
	mu      sync.Mutex
	methods []string
	bodies  []string
	headers []http.Header
}

func (r *redirectRecorder) record(w http.ResponseWriter, req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	r.methods = append(r.methods, req.Method)
	r.bodies = append(r.bodies, string(body))
	r.headers = append(r.headers, req.Header.Clone())
	r.mu.Unlock()
	_, _ = w.Write([]byte("ok"))
}

func (r *redirectRecorder) snapshot() ([]string, []string, []http.Header) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.methods...),
		append([]string(nil), r.bodies...),
		append([]http.Header(nil), r.headers...)
}

// newRedirectServer 返回 /start(重定向)与 /target(记录)两个路径的服务器。
func newRedirectServer(t *testing.T, status int, location string) (*httptest.Server, *redirectRecorder) {
	t.Helper()
	rec := &redirectRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			if location == "" {
				w.WriteHeader(status)
				return
			}
			http.Redirect(w, r, location, status)
		case "/target":
			rec.record(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func TestRedirectFollowed(t *testing.T) {
	srv, rec := newRedirectServer(t, http.StatusFound, "/target")
	client, err := New()
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), srv.URL+"/start")
	testx.RequireNoError(t, err)

	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("响应体 = %q,want ok", body)
	}
	methods, _, _ := rec.snapshot()
	if len(methods) != 1 || methods[0] != http.MethodGet {
		t.Errorf("目标请求不符:%v", methods)
	}
}

func TestRedirectAbsoluteLocation(t *testing.T) {
	srv, rec := newRedirectServer(t, http.StatusMovedPermanently, "http://example.com/x")
	// 覆盖绝对地址解析分支:直接单测 redirectRequest。
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/start", nil)
	testx.RequireNoError(t, err)

	resp := &http.Response{
		StatusCode: http.StatusMovedPermanently,
		Header:     make(http.Header),
		Request:    req,
	}
	resp.Header.Set("Location", "http://example.com/x")
	next, err := redirectRequest(req, resp)
	testx.RequireNoError(t, err)

	if next.URL.String() != "http://example.com/x" {
		t.Errorf("目标 URL = %s", next.URL)
	}
	_ = rec
}

func TestRedirectMethodConversion(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		method     string
		body       string
		wantMethod string
		wantBody   string
	}{
		{"POST 301 转 GET", http.StatusMovedPermanently, http.MethodPost, "p", http.MethodGet, ""},
		{"POST 302 转 GET", http.StatusFound, http.MethodPost, "p", http.MethodGet, ""},
		{"POST 303 转 GET", http.StatusSeeOther, http.MethodPost, "p", http.MethodGet, ""},
		{"PUT 303 转 GET", http.StatusSeeOther, http.MethodPut, "p", http.MethodGet, ""},
		{"PUT 301 保留", http.StatusMovedPermanently, http.MethodPut, "p", http.MethodPut, "p"},
		{"PUT 307 保留", http.StatusTemporaryRedirect, http.MethodPut, "p", http.MethodPut, "p"},
		{"PUT 308 保留", http.StatusPermanentRedirect, http.MethodPut, "p", http.MethodPut, "p"},
		{"GET 303 保留", http.StatusSeeOther, http.MethodGet, "", http.MethodGet, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, rec := newRedirectServer(t, tc.status, "/target")
			client, err := New()
			testx.RequireNoError(t, err)

			req, err := http.NewRequestWithContext(
				context.Background(), tc.method, srv.URL+"/start", strings.NewReader(tc.body))
			testx.RequireNoError(t, err)

			resp, err := client.Do(context.Background(), req)
			testx.RequireNoError(t, err)

			_ = resp.Body.Close()
			methods, bodies, _ := rec.snapshot()
			if len(methods) != 1 || methods[0] != tc.wantMethod {
				t.Errorf("方法 = %v,want %s", methods, tc.wantMethod)
			}
			if len(bodies) != 1 || bodies[0] != tc.wantBody {
				t.Errorf("请求体 = %v,want %q", bodies, tc.wantBody)
			}
		})
	}
}

func TestRedirectNoLocation(t *testing.T) {
	srv, _ := newRedirectServer(t, http.StatusFound, "")
	client, err := New()
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), srv.URL+"/start")
	testx.RequireNoError(t, err)

	defer resp.Body.Close()
	testx.Equal(t, resp.StatusCode, http.StatusFound)

}

func TestRedirectExceeded(t *testing.T) {
	srv, _ := newRedirectServer(t, http.StatusFound, "/start") // 自我循环
	client, err := New(WithMaxRedirects(3))
	testx.RequireNoError(t, err)

	_, err = client.Get(context.Background(), srv.URL+"/start")
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeRedirectExceeded {
		t.Errorf("错误码 = %s,want %s", code, CodeRedirectExceeded)
	}
}

func TestNoRedirect(t *testing.T) {
	srv, _ := newRedirectServer(t, http.StatusFound, "/target")
	client, err := New(WithNoRedirect())
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), srv.URL+"/start")
	testx.RequireNoError(t, err)

	defer resp.Body.Close()
	testx.Equal(t, resp.StatusCode, http.StatusFound)

}

func TestRedirectPolicyAllow(t *testing.T) {
	srv, rec := newRedirectServer(t, http.StatusFound, "/target")
	var called int
	client, err := New(WithRedirectPolicy(func(next *http.Request, via []*http.Request) error {
		called++
		if next.URL.Path != "/target" || len(via) != 1 {
			t.Errorf("策略参数不符:next=%s via=%d", next.URL.Path, len(via))
		}
		return nil
	}))
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), srv.URL+"/start")
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()
	if called != 1 {
		t.Errorf("策略调用次数 = %d,want 1", called)
	}
	methods, _, _ := rec.snapshot()
	if len(methods) != 1 {
		t.Error("目标未被访问")
	}
}

func TestRedirectPolicyReject(t *testing.T) {
	srv, _ := newRedirectServer(t, http.StatusFound, "/target")
	policyErr := errx.New(errx.KindBusiness, "NO_REDIRECT", "拒绝跳转")
	client, err := New(WithRedirectPolicy(func(*http.Request, []*http.Request) error {
		return policyErr
	}))
	testx.RequireNoError(t, err)

	_, err = client.Get(context.Background(), srv.URL+"/start")
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeRedirectExceeded {
		t.Errorf("错误码 = %s,want %s", code, CodeRedirectExceeded)
	}
	testx.ErrorIs(t, err, policyErr)

}

func TestRedirectCrossOriginStripsSensitiveHeaders(t *testing.T) {
	rec := &redirectRecorder{}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(w, r)
	}))
	defer target.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/target", http.StatusFound)
	}))
	defer source.Close()

	client, err := New()
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), source.URL+"/start",
		WithHeader("Authorization", "Bearer secret"),
		WithHeader("Cookie", "session=abc"),
	)
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()
	_, _, headers := rec.snapshot()
	if len(headers) != 1 {
		t.Fatalf("目标请求数 = %d", len(headers))
	}
	if headers[0].Get("Authorization") != "" || headers[0].Get("Cookie") != "" {
		t.Errorf("跨域跳转应剥离敏感头:%v", headers[0])
	}
}

func TestRedirectSameOriginKeepsSensitiveHeaders(t *testing.T) {
	srv, rec := newRedirectServer(t, http.StatusFound, "/target")
	client, err := New()
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), srv.URL+"/start",
		WithHeader("Authorization", "Bearer secret"),
	)
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()
	_, _, headers := rec.snapshot()
	if len(headers) != 1 || headers[0].Get("Authorization") != "Bearer secret" {
		t.Errorf("同源跳转应保留敏感头:%v", headers)
	}
}

func TestRedirectBodyUnreadable(t *testing.T) {
	srv, _ := newRedirectServer(t, http.StatusTemporaryRedirect, "/target")
	client, err := New()
	testx.RequireNoError(t, err)

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPut, srv.URL+"/start", io.NopCloser(strings.NewReader("x")))
	testx.RequireNoError(t, err)

	req.GetBody = nil
	_, err = client.Do(context.Background(), req)
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeBodyUnreadable {
		t.Errorf("错误码 = %s,want %s", code, CodeBodyUnreadable)
	}
}

func TestRedirectBodyReplayable(t *testing.T) {
	srv, rec := newRedirectServer(t, http.StatusTemporaryRedirect, "/target")
	client, err := New()
	testx.RequireNoError(t, err)

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPut, srv.URL+"/start", bytes.NewReader([]byte("payload")))
	testx.RequireNoError(t, err)

	resp, err := client.Do(context.Background(), req)
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()
	methods, bodies, _ := rec.snapshot()
	if len(methods) != 1 || methods[0] != http.MethodPut {
		t.Errorf("方法 = %v,want PUT", methods)
	}
	if len(bodies) != 1 || bodies[0] != "payload" {
		t.Errorf("请求体 = %v,want payload", bodies)
	}
}

func TestRedirectInvalidLocation(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://example.com/start", nil)
	testx.RequireNoError(t, err)

	resp := &http.Response{
		StatusCode: http.StatusFound,
		Header:     make(http.Header),
		Request:    req,
	}
	resp.Header.Set("Location", "http://%zz")
	_, err = redirectRequest(req, resp)
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeRedirectFailed {
		t.Errorf("错误码 = %s,want %s", code, CodeRedirectFailed)
	}
}

func TestReplayBody(t *testing.T) {
	// GetBody 成功
	req, _ := http.NewRequest(http.MethodPut, "http://example.com", strings.NewReader("x"))
	body, err := replayBody(req)
	testx.RequireNoError(t, err)

	data, _ := io.ReadAll(body)
	if string(data) != "x" {
		t.Errorf("GetBody 内容 = %q", data)
	}
	// GetBody 返回错误
	req, _ = http.NewRequest(http.MethodPut, "http://example.com", nil)
	req.Body = io.NopCloser(strings.NewReader("x"))
	req.GetBody = func() (io.ReadCloser, error) {
		return nil, errors.New("重建失败")
	}
	if _, err := replayBody(req); err == nil {
		t.Error("GetBody 失败应返回错误")
	}
	// io.ReadSeeker 成功
	req, _ = http.NewRequest(http.MethodPut, "http://example.com", nil)
	req.Body = seekReadCloser{bytes.NewReader([]byte("y"))}
	req.GetBody = nil
	body, err = replayBody(req)
	testx.RequireNoError(t, err)

	data, _ = io.ReadAll(body)
	if string(data) != "y" {
		t.Errorf("ReadSeeker 内容 = %q", data)
	}
	// Seek 失败
	req, _ = http.NewRequest(http.MethodPut, "http://example.com", nil)
	req.Body = &failSeeker{}
	req.GetBody = nil
	if _, err := replayBody(req); err == nil {
		t.Error("Seek 失败应返回错误")
	}
}

func TestSameOrigin(t *testing.T) {
	parse := func(raw string) *url.URL {
		u, err := url.Parse(raw)
		testx.RequireNoError(t, err)

		return u
	}
	cases := []struct {
		a, b string
		want bool
	}{
		{"http://a.com/x", "http://a.com/y", true},
		{"http://A.com/x", "http://a.com/y", true},
		{"http://a.com/x", "http://b.com/y", false},
		{"http://a.com/x", "https://a.com/y", false},
		{"http://a.com:8080/x", "http://a.com/y", false},
	}
	for _, tc := range cases {
		if got := sameOrigin(parse(tc.a), parse(tc.b)); got != tc.want {
			t.Errorf("%s vs %s = %v,want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
