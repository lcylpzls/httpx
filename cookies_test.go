package httpx

import (
	"context"
	testx "github.com/lcylpzls/testx"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCookieSession(t *testing.T) {
	var gotCookie bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("session"); err == nil {
			gotCookie = true
		}
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc"})
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	jar, err := cookiejar.New(nil)
	testx.RequireNoError(t, err)

	client, err := New(WithCookieJar(jar))
	testx.RequireNoError(t, err)

	// 第一次请求:保存 Set-Cookie。
	resp, err := client.Get(context.Background(), srv.URL)
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()
	testx.RequireFalse(t, gotCookie)

	// 第二次请求:自动注入。
	gotCookie = false
	resp, err = client.Get(context.Background(), srv.URL)
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()
	testx.True(t, gotCookie)

}

func TestCookieAcrossRedirects(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("session"); err != nil {
			t.Errorf("重定向目标应收到会话 Cookie")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "xyz"})
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer source.Close()

	jar, err := cookiejar.New(nil)
	testx.RequireNoError(t, err)

	client, err := New(WithCookieJar(jar))
	testx.RequireNoError(t, err)

	resp, err := client.Get(context.Background(), source.URL)
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()
}

func TestNoCookieJar(t *testing.T) {
	var sawCookie bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("session"); err == nil {
			sawCookie = true
		}
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc"})
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := New()
	testx.RequireNoError(t, err)

	for i := 0; i < 2; i++ {
		resp, err := client.Get(context.Background(), srv.URL)
		testx.RequireNoError(t, err)

		_ = resp.Body.Close()
	}
	testx.False(t, sawCookie)

}

func TestCookieJarWithScriptedResponse(t *testing.T) {
	// 覆盖 storeCookies 对 resp.Request 为 nil 的防御分支。
	jar, err := cookiejar.New(nil)
	testx.RequireNoError(t, err)

	client, err := New(WithCookieJar(jar))
	testx.RequireNoError(t, err)

	client.rt = &scriptedRT{results: []roundTripResult{
		{resp: &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    nil,
		}},
	}}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	testx.RequireNoError(t, err)

	resp, err := client.Do(context.Background(), req)
	testx.RequireNoError(t, err)

	_ = resp.Body.Close()
}

func TestInjectCookiesWithoutJar(t *testing.T) {
	client, err := New()
	testx.RequireNoError(t, err)

	req, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
	testx.RequireNoError(t, err)

	client.injectCookies(req) // 空操作,不应 panic
	if req.Header.Get("Cookie") != "" {
		t.Error("无 jar 不应注入 Cookie")
	}
}
