// limit 示例:并发限流与 DNS 缓存。
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/lcylpzls/httpx"
)

func main() {
	client, err := httpx.New(
		httpx.WithTimeout(5*time.Second),
		httpx.WithMaxConcurrency(10), // 最多 10 个同时在途请求
		httpx.WithDNSCache(httpx.NewDNSCache(60*time.Second)),
	)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	resp, err := client.Get(ctx, "https://api.example.com/items",
		httpx.WithRequestTimeout(2*time.Second)) // 单请求级超时
	if err != nil {
		panic(err)
	}
	body, err := httpx.ReadString(resp, 1<<20)
	if err != nil {
		panic(err)
	}
	fmt.Println(body)
}
