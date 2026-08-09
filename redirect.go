package httpx

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/lcylpzls/errx"
)

// redirectStatuses 是需要跟随的重定向状态码。
var redirectStatuses = map[int]bool{
	http.StatusMovedPermanently:  true, // 301
	http.StatusFound:             true, // 302
	http.StatusSeeOther:          true, // 303
	http.StatusTemporaryRedirect: true, // 307
	http.StatusPermanentRedirect: true, // 308
}

// sensitiveHeaders 是跨域重定向时需要剥离的敏感请求头。
var sensitiveHeaders = []string{
	"Authorization",
	"Proxy-Authorization",
	"Cookie",
	"Cookie2",
	"WWW-Authenticate",
}

// followRedirects 执行请求并自动跟随重定向。
// 规则对齐 net/http:303 全转 GET,301/302 仅 POST 转 GET,
// 307/308 保留方法与请求体;跨域剥离敏感头。
func (c *Client) followRedirects(ctx context.Context, req *http.Request) (*http.Response, error) {
	cur := req
	via := []*http.Request{req}
	for {
		resp, err := c.do(ctx, cur)
		if err != nil {
			return nil, err
		}
		// 非重定向状态或无 Location:直接返回。
		if !redirectStatuses[resp.StatusCode] || resp.Header.Get("Location") == "" {
			return resp, nil
		}
		// 不跟随(WithNoRedirect / WithMaxRedirects(0)):返回 3xx 响应。
		if c.cfg.maxRedirects == 0 {
			return resp, nil
		}
		// 未自定义策略时按次数上限截断。
		if c.cfg.redirectPolicy == nil && len(via) > c.cfg.maxRedirects {
			_ = resp.Body.Close()
			return nil, errx.New(errx.KindInvalid, CodeRedirectExceeded, "重定向次数超限")
		}
		next, err := redirectRequest(cur, resp)
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}
		// 自定义策略:返回错误即终止跟随。
		if c.cfg.redirectPolicy != nil {
			if err := c.cfg.redirectPolicy(next, via); err != nil {
				return nil, errx.Wrap(err, errx.KindInvalid, CodeRedirectExceeded, "重定向策略终止跟随")
			}
		}
		cur = next
		via = append(via, next)
	}
}

// redirectRequest 基于当前请求与重定向响应构造下一个请求。
func redirectRequest(req *http.Request, resp *http.Response) (*http.Request, error) {
	loc, err := resp.Location()
	if err != nil {
		return nil, errx.Wrap(err, errx.KindInvalid, CodeRedirectFailed, "解析重定向地址失败")
	}
	code := resp.StatusCode
	method := req.Method
	keepBody := true
	switch {
	case req.Method == http.MethodPost && (code == http.StatusMovedPermanently ||
		code == http.StatusFound || code == http.StatusSeeOther):
		method = http.MethodGet
		keepBody = false
	case code == http.StatusSeeOther && req.Method != http.MethodGet && req.Method != http.MethodHead:
		method = http.MethodGet
		keepBody = false
	}
	next := req.Clone(req.Context())
	next.Method = method
	next.URL = loc
	next.Host = ""
	if !keepBody {
		next.Body = nil
		next.GetBody = nil
		next.ContentLength = 0
	} else if req.Body != nil {
		body, err := replayBody(req)
		if err != nil {
			return nil, err
		}
		next.Body = body
		next.ContentLength = req.ContentLength
	}
	// 跨域跳转剥离敏感头。
	if !sameOrigin(req.URL, loc) {
		for _, h := range sensitiveHeaders {
			next.Header.Del(h)
		}
	}
	return next, nil
}

// replayBody 重建可重读的请求体(优先 GetBody,其次 io.ReadSeeker)。
func replayBody(req *http.Request) (io.ReadCloser, error) {
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, errx.Wrap(err, errx.KindUnavailable, CodeRedirectFailed, "重建请求体失败")
		}
		return body, nil
	}
	if rs, ok := req.Body.(io.ReadSeeker); ok {
		if _, err := rs.Seek(0, io.SeekStart); err != nil {
			return nil, errx.Wrap(err, errx.KindUnavailable, CodeRedirectFailed, "重置请求体失败")
		}
		return io.NopCloser(rs), nil
	}
	return nil, errx.New(errx.KindInvalid, CodeBodyUnreadable, "重定向请求体不可重读")
}

// sameOrigin 判断两个 URL 是否同源(scheme + host 相同)。
func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Host, b.Host) && strings.EqualFold(a.Scheme, b.Scheme)
}
