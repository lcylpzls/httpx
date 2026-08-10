package httpx

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/lcylpzls/errx"
	"github.com/lcylpzls/resiliencex"
)

// Client 是 HTTP 客户端入口,持有固定协议与连接池配置。
type Client struct {
	cfg   config
	rt    http.RoundTripper
	stats clientStats
	// bulkhead 限制在途请求数（0 表示不限制），算法由 resiliencex 提供。
	bulkhead *resiliencex.Bulkhead
}

// New 创建 HTTP 客户端。协议与连接池在创建时固定,运行期不可变。
func New(opts ...Option) (*Client, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	rt, err := newRoundTripper(cfg)
	if err != nil {
		return nil, err
	}
	if cfg.roundTripperWrapper != nil {
		rt = cfg.roundTripperWrapper(rt)
	}
	c := &Client{cfg: cfg, rt: rt}
	if cfg.maxConcurrency > 0 {
		b, err := resiliencex.NewBulkhead(cfg.maxConcurrency)
		if err != nil {
			return nil, err
		}
		c.bulkhead = b
	}
	return c, nil
}

// Do 执行一次请求并返回响应。
// 配置了整体超时时,超时通过 context 覆盖完整请求生命周期。
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if req == nil {
		return nil, errx.NewCode(CodeInvalidConfig, "请求不能为空")
	}
	// 请求级超时与客户端级超时取更严格者。
	effective := c.cfg.timeout
	if reqTimeout, ok := req.Context().Value(reqTimeoutKey{}).(time.Duration); ok && reqTimeout > 0 {
		if effective == 0 || reqTimeout < effective {
			effective = reqTimeout
		}
	}
	if effective > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, effective)
		req = req.WithContext(ctx)
		resp, err := c.followRedirects(ctx, req)
		if err != nil {
			cancel()
			return nil, wrapDoError(err)
		}
		// 超时覆盖完整请求生命周期：响应体关闭（或超时计时器触发）时
		// 才取消上下文，避免 HTTP/2 / HTTP/3 在读取响应体前被本地取消。
		if resp.Body != nil {
			resp.Body = &timeoutBody{ReadCloser: resp.Body, cancel: cancel}
		} else {
			cancel()
		}
		return resp, nil
	}
	// 并发限流:等待许可,受 context 取消约束。
	if c.bulkhead != nil {
		release, err := c.bulkhead.Acquire(ctx)
		if err != nil {
			return nil, errx.Wrap(err, errx.KindCancelled, CodeRequestFailed, "等待并发许可被取消")
		}
		defer release()
	}
	resp, err := c.followRedirects(ctx, req)
	if err != nil {
		return nil, wrapDoError(err)
	}
	return resp, nil
}

// timeoutBody 延迟取消请求上下文：直到响应体关闭才释放超时资源。
// 超时计时器到期仍会主动取消，符合“超时覆盖完整生命周期”的语义。
type timeoutBody struct {
	io.ReadCloser
	cancel context.CancelFunc
	once   sync.Once
}

// Close 关闭响应体并取消请求上下文。
func (b *timeoutBody) Close() error {
	b.once.Do(b.cancel)
	return b.ReadCloser.Close()
}

// CloseIdleConnections 释放连接池中的空闲连接,不中断正在使用的请求。
// HTTP/3 子包等价于关闭全部空闲 QUIC 连接。
func (c *Client) CloseIdleConnections() {
	switch t := c.rt.(type) {
	case interface{ CloseIdleConnections() }:
		t.CloseIdleConnections()
	case io.Closer:
		_ = t.Close()
	}
}

// Get 发起 GET 请求。
func (c *Client) Get(ctx context.Context, url string, opts ...RequestOption) (*http.Response, error) {
	req, err := c.buildRequest(ctx, http.MethodGet, url, nil, opts)
	if err != nil {
		return nil, err
	}
	return c.Do(ctx, req)
}

// Post 发起 POST 请求,body 规则见 bodyToReader。
func (c *Client) Post(ctx context.Context, url string, body any, opts ...RequestOption) (*http.Response, error) {
	req, err := c.buildRequest(ctx, http.MethodPost, url, body, opts)
	if err != nil {
		return nil, err
	}
	return c.Do(ctx, req)
}

// Request 以任意方法发起请求。
func (c *Client) Request(ctx context.Context, method, url string, opts ...RequestOption) (*http.Response, error) {
	req, err := c.buildRequest(ctx, method, url, nil, opts)
	if err != nil {
		return nil, err
	}
	return c.Do(ctx, req)
}

// buildRequest 将方法、URL、body 与请求选项合并为 *http.Request。
func (c *Client) buildRequest(ctx context.Context, method, rawURL string, body any, opts []RequestOption) (*http.Request, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ro := requestOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&ro)
		}
	}
	r, contentType, err := bodyToReader(body)
	if err != nil {
		return nil, err
	}
	if ro.bodyValue != nil {
		data, err := marshalJSONBody(ro.bodyValue)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(data)
		contentType = "application/json"
	}
	if ro.xmlBody != nil {
		data, err := xml.Marshal(ro.xmlBody)
		if err != nil {
			return nil, errx.WrapCode(err, CodeInvalidConfig, "请求体 XML 序列化失败")
		}
		r = bytes.NewReader(data)
		contentType = "application/xml"
	}
	if ro.multipartSet {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		for k, v := range ro.formFields {
			_ = w.WriteField(k, v)
		}
		for name, f := range ro.formFiles {
			fw, _ := w.CreateFormFile(name, f.Filename)
			if f.Reader != nil {
				_, _ = io.Copy(fw, f.Reader)
			} else {
				_, _ = fw.Write(f.Content)
			}
		}
		_ = w.Close()
		r = &buf
		contentType = w.FormDataContentType()
	}
	if ro.body != nil {
		r = ro.body
	}
	if ro.contentType != "" {
		contentType = ro.contentType
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, r)
	if err != nil {
		return nil, errx.WrapCode(err, CodeInvalidConfig, "构建请求失败")
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, vs := range ro.header {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	if len(ro.query) > 0 {
		q := req.URL.Query()
		for k, vs := range ro.query {
			for _, v := range vs {
				q.Add(k, v)
			}
		}
		req.URL.RawQuery = q.Encode()
	}
	if ro.authUser != "" || ro.authPass != "" {
		req.SetBasicAuth(ro.authUser, ro.authPass)
	}
	if ro.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+ro.bearer)
	}
	if ro.userAgent != "" {
		req.Header.Set("User-Agent", ro.userAgent)
	}
	if ro.requestID != "" {
		req.Header.Set("X-Request-ID", ro.requestID)
	}
	if ro.timeout > 0 {
		req = req.WithContext(context.WithValue(req.Context(), reqTimeoutKey{}, ro.timeout))
	}
	return req, nil
}

// reqTimeoutKey 是请求级超时的 context 键。
type reqTimeoutKey struct{}

// marshalJSONBody 序列化 JSON 请求体。
func marshalJSONBody(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, errx.WrapCode(err, CodeInvalidConfig, "请求体 JSON 序列化失败")
	}
	return data, nil
}

// wrapDoError 将 net/http 传输层错误分类包装为 HTX_* 错误码。
func wrapDoError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errx.As(err); ok {
		return err
	}
	var netOpErr *net.OpError
	if errors.As(err, &netOpErr) && netOpErr.Op == "dial" {
		return errx.WrapCode(err, CodeDialFailed, "建立连接失败")
	}
	if isTLSError(err) {
		return errx.WrapCode(err, CodeTLSFailed, "TLS 握手失败")
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		switch {
		case strings.Contains(urlErr.Op, "dial"):
			return errx.WrapCode(err, CodeDialFailed, "建立连接失败")
		case strings.Contains(urlErr.Op, "tls"):
			return errx.WrapCode(err, CodeTLSFailed, "TLS 握手失败")
		}
		return errx.WrapCode(err, CodeRequestFailed, "请求发送失败")
	}
	return errx.WrapCode(err, CodeRequestFailed, "请求发送失败")
}

// isTLSError 识别 crypto/tls 层的典型错误:
// 记录头错误、证书验证错误与 "tls:" 前缀错误。
func isTLSError(err error) bool {
	var recordErr tls.RecordHeaderError
	if errors.As(err, &recordErr) {
		return true
	}
	var verifyErr *tls.CertificateVerificationError
	if errors.As(err, &verifyErr) {
		return true
	}
	if strings.Contains(err.Error(), "tls:") {
		return true
	}
	var netOpErr *net.OpError
	return errors.As(err, &netOpErr) && strings.Contains(netOpErr.Op, "tls")
}
