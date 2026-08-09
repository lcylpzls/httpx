// session 示例:Cookie 会话、重定向跟随与请求钩子。
package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"time"

	"github.com/lcylpzls/httpx"
)

func main() {
	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(err)
	}
	client, err := httpx.New(
		httpx.WithTimeout(5*time.Second),
		httpx.WithCookieJar(jar), // 登录后自动维护会话
		httpx.WithMaxRedirects(5),
		httpx.WithHooks(httpx.Hooks{
			OnRequest: func(req *http.Request) error {
				fmt.Printf("请求 %s %s\n", req.Method, req.URL)
				return nil
			},
		}),
	)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	// 登录接口返回 Set-Cookie,后续请求自动携带。
	if _, err := client.Post(ctx, "https://api.example.com/login",
		map[string]string{"user": "demo"}); err != nil {
		panic(err)
	}
	resp, err := client.Get(ctx, "https://api.example.com/profile")
	if err != nil {
		panic(err)
	}
	body, err := httpx.ReadString(resp, 1<<20)
	if err != nil {
		panic(err)
	}
	fmt.Println(body)
}
