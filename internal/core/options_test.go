package core

import (
	"bytes"
	"crypto/tls"
	testx "github.com/lcylpzls/testx"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/lcylpzls/errx"
)

func TestProtocolString(t *testing.T) {
	cases := []struct {
		p    Protocol
		want string
	}{
		{ProtocolAuto, "auto"},
		{ProtocolHTTP1, "http/1.1"},
		{ProtocolHTTP2, "http/2"},
		{ProtocolHTTP3, "http/3"},
		{Protocol(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.p.String(); got != tc.want {
			t.Errorf("%v:String = %q,want %q", tc.p, got, tc.want)
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()
	testx.Equal(t, cfg.protocol, ProtocolAuto)

	if cfg.dialTimeout != defaultDialTimeout ||
		cfg.tlsHandshakeTimeout != defaultTLSHandshakeTimeout ||
		cfg.responseHeaderTimeout != defaultResponseHeaderTimeout {
		t.Error("默认超时参数不符")
	}
	if cfg.maxIdleConns != defaultMaxIdleConns ||
		cfg.maxIdleConnsPerHost != defaultMaxIdleConnsPerHost ||
		cfg.idleConnTimeout != defaultIdleConnTimeout {
		t.Error("默认连接池参数不符")
	}
	if cfg.retry != nil || cfg.logger != nil || cfg.metrics != nil {
		t.Error("重试/观测默认应为关闭")
	}
}

func TestOptionsApply(t *testing.T) {
	logger := &fakeLogger{}
	metrics := &fakeMetrics{}
	tlsCfg := &tls.Config{ServerName: "example.com"}
	jar, err := cookiejar.New(nil)
	testx.RequireNoError(t, err)

	var policyCalls int
	cfg := defaultConfig()
	opts := []Option{
		WithTimeout(1 * time.Second),
		WithDialTimeout(2 * time.Second),
		WithTLSHandshakeTimeout(3 * time.Second),
		WithResponseHeaderTimeout(4 * time.Second),
		WithMaxIdleConns(5),
		WithMaxIdleConnsPerHost(6),
		WithIdleConnTimeout(7 * time.Second),
		WithTLSClientConfig(tlsCfg),
		WithProtocol(ProtocolHTTP2),
		WithRetry(3, FixedBackoff(time.Millisecond)),
		WithLogger(logger),
		WithLogRequest(true),
		WithSlowThreshold(50 * time.Millisecond),
		WithMetrics(metrics),
		WithMaxRedirects(5),
		WithRedirectPolicy(func(*http.Request, []*http.Request) error {
			policyCalls++
			return nil
		}),
		WithCookieJar(jar),
		WithHooks(Hooks{OnError: func(error) {}}),
		WithProxy(nil),
		WithDisableCompression(true),
		WithMaxResponseHeaderBytes(8192),
		WithMaxConnsPerHost(4),
		WithExpectContinueTimeout(time.Second),
		nil,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.timeout != time.Second ||
		cfg.dialTimeout != 2*time.Second ||
		cfg.tlsHandshakeTimeout != 3*time.Second ||
		cfg.responseHeaderTimeout != 4*time.Second {
		t.Error("超时选项应用失败")
	}
	if cfg.maxIdleConns != 5 || cfg.maxIdleConnsPerHost != 6 || cfg.idleConnTimeout != 7*time.Second {
		t.Error("连接池选项应用失败")
	}
	if cfg.tlsClientConfig == nil || cfg.tlsClientConfig.ServerName != "example.com" {
		t.Error("TLS 配置选项应用失败")
	}
	testx.Equal(t, cfg.protocol, ProtocolHTTP2)

	if cfg.retry == nil || cfg.retry.maxAttempts != 3 {
		t.Error("重试选项应用失败")
	}
	if cfg.logger != logger || !cfg.logRequest || cfg.slowThreshold != 50*time.Millisecond || cfg.metrics != metrics {
		t.Error("观测选项应用失败")
	}
	if cfg.maxRedirects != 5 || cfg.redirectPolicy == nil {
		t.Error("重定向选项应用失败")
	}
	testx.Equal(t, cfg.cookieJar, jar)

	if cfg.hooks.OnError == nil {
		t.Error("钩子选项应用失败")
	}
	if !cfg.proxySet || cfg.proxy != nil {
		t.Error("代理选项应用失败")
	}
	if !cfg.disableCompression {
		t.Error("压缩开关应用失败")
	}
	if cfg.maxResponseHeaderBytes != 8192 || cfg.maxConnsPerHost != 4 ||
		cfg.expectContinueTimeout != time.Second {
		t.Error("连接细节选项应用失败")
	}
	if policyCalls != 0 {
		t.Error("策略回调不应立即调用")
	}
}

func TestValidateConfig(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*config)
		wantErr bool
		code    errx.Code
	}{
		{"默认合法", func(*config) {}, false, ""},
		{"整体超时负数", func(c *config) { c.timeout = -1 }, true, CodeInvalidConfig},
		{"Dial 超时负数", func(c *config) { c.dialTimeout = -1 }, true, CodeInvalidConfig},
		{"TLS 超时负数", func(c *config) { c.tlsHandshakeTimeout = -1 }, true, CodeInvalidConfig},
		{"响应头超时负数", func(c *config) { c.responseHeaderTimeout = -1 }, true, CodeInvalidConfig},
		{"最大空闲负数", func(c *config) { c.maxIdleConns = -1 }, true, CodeInvalidConfig},
		{"每主机负数", func(c *config) { c.maxIdleConnsPerHost = -1 }, true, CodeInvalidConfig},
		{"空闲回收负数", func(c *config) { c.idleConnTimeout = -1 }, true, CodeInvalidConfig},
		{"慢阈值负数", func(c *config) { c.slowThreshold = -1 }, true, CodeInvalidConfig},
		{"重定向上限负数", func(c *config) { c.maxRedirects = -1 }, true, CodeInvalidConfig},
		{"非法协议", func(c *config) { c.protocol = Protocol(99) }, true, CodeInvalidConfig},
		{"重试次数为 0", func(c *config) {
			c.retry = &retryPolicy{maxAttempts: 0, backoff: FixedBackoff(time.Millisecond)}
		}, true, CodeInvalidConfig},
		{"重试退避为空", func(c *config) {
			c.retry = &retryPolicy{maxAttempts: 2, backoff: nil}
		}, true, CodeInvalidConfig},
		{"重试总时长负数", func(c *config) {
			c.retry = &retryPolicy{maxAttempts: 2, backoff: FixedBackoff(time.Millisecond), totalTimeout: -1}
		}, true, CodeInvalidConfig},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			tc.mutate(&cfg)
			err := validateConfig(cfg)
			if tc.wantErr {
				testx.RequireError(t, err)

				if code, _ := errx.CodeOf(err); code != tc.code {
					t.Errorf("错误码 = %s,want %s", code, tc.code)
				}
				return
			}
			testx.RequireNoError(t, err)

		})
	}
}

func TestBodyToReader(t *testing.T) {
	form := url.Values{"a": []string{"1"}}
	// nil
	r, ct, err := bodyToReader(nil)
	if err != nil || r != nil || ct != "" {
		t.Errorf("nil:r=%v ct=%q err=%v", r, ct, err)
	}
	// io.Reader
	r, ct, err = bodyToReader(strings.NewReader("abc"))
	if err != nil || ct != "" {
		t.Fatalf("io.Reader 分支失败:%v", err)
	}
	data := make([]byte, 3)
	if _, err := r.Read(data); err != nil || string(data) != "abc" {
		t.Errorf("io.Reader 内容不符:%s %v", data, err)
	}
	// string
	r, ct, err = bodyToReader("hello")
	if err != nil || ct != "" {
		t.Fatalf("string 分支失败:%v", err)
	}
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(r); err != nil || buf.String() != "hello" {
		t.Errorf("string 内容不符:%q %v", buf.String(), err)
	}
	// []byte
	r, ct, err = bodyToReader([]byte("world"))
	if err != nil || ct != "" {
		t.Fatalf("[]byte 分支失败:%v", err)
	}
	buf.Reset()
	if _, err := buf.ReadFrom(r); err != nil || buf.String() != "world" {
		t.Errorf("[]byte 内容不符:%q %v", buf.String(), err)
	}
	// url.Values
	r, ct, err = bodyToReader(form)
	if err != nil || ct != "application/x-www-form-urlencoded" {
		t.Fatalf("url.Values 分支失败:ct=%q err=%v", ct, err)
	}
	buf.Reset()
	if _, err := buf.ReadFrom(r); err != nil || buf.String() != "a=1" {
		t.Errorf("url.Values 内容不符:%q %v", buf.String(), err)
	}
	// 其他类型 → JSON
	r, ct, err = bodyToReader(map[string]int{"n": 1})
	if err != nil || ct != "application/json" {
		t.Fatalf("JSON 分支失败:ct=%q err=%v", ct, err)
	}
	buf.Reset()
	if _, err := buf.ReadFrom(r); err != nil || buf.String() != `{"n":1}` {
		t.Errorf("JSON 内容不符:%q %v", buf.String(), err)
	}
	// JSON 序列化失败
	_, _, err = bodyToReader(map[any]any{make(chan int): 1})
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeInvalidConfig {
		t.Errorf("错误码 = %s,want %s", code, CodeInvalidConfig)
	}
}

func TestRequestOptionsApply(t *testing.T) {
	ro := requestOptions{}
	opts := []RequestOption{
		WithHeader("X-Test", "v1"),
		WithHeader("X-Test", "v2"),
		WithQuery("q", "1"),
		WithQuery("q", "2"),
		WithJSONBody(map[string]string{"k": "v"}),
		WithBytesBody([]byte("raw")),
		WithFormBody(url.Values{"f": []string{"x"}}),
		WithBasicAuth("user", "pass"),
		WithBearer("tok"),
		WithUserAgent("httpx-test"),
		WithMultipartFormData(map[string]string{"f": "x"}, map[string]FileField{"file": {Filename: "a.txt", Content: []byte("c")}}),
		WithXMLBody(map[string]string{"k": "v"}),
		nil,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&ro)
		}
	}
	if got := ro.header.Get("X-Test"); got != "v2" {
		t.Errorf("WithHeader 覆盖语义不符:%q", got)
	}
	if q := ro.query.Encode(); q != "q=1&q=2" {
		t.Errorf("WithQuery 追加语义不符:%q", q)
	}
	if ro.bodyValue == nil || ro.body == nil || ro.contentType != "application/x-www-form-urlencoded" {
		t.Error("请求体选项应用失败")
	}
	if ro.authUser != "user" || ro.authPass != "pass" || ro.bearer != "tok" || ro.userAgent != "httpx-test" {
		t.Error("认证/UA 选项应用失败")
	}
	if ro.formFields == nil || ro.formFiles == nil || ro.xmlBody == nil {
		t.Error("XML/multipart 选项应用失败")
	}
}
