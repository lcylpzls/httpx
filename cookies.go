package httpx

import "net/http"

// injectCookies 将 CookieJar 中的会话 Cookie 注入请求。
// 未配置 jar 时为空操作。
func (c *Client) injectCookies(req *http.Request) {
	if c.cfg.cookieJar == nil {
		return
	}
	for _, cookie := range c.cfg.cookieJar.Cookies(req.URL) {
		req.AddCookie(cookie)
	}
}

// storeCookies 将响应 Set-Cookie 保存到 CookieJar。
// 未配置 jar 时为空操作。
func (c *Client) storeCookies(resp *http.Response) {
	if c.cfg.cookieJar == nil {
		return
	}
	if resp.Request == nil {
		return
	}
	if cookies := resp.Cookies(); len(cookies) > 0 {
		c.cfg.cookieJar.SetCookies(resp.Request.URL, cookies)
	}
}
