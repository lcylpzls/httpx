// retry 示例:幂等重试与指数退避。
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/lcylpzls/httpx"
)

func main() {
	client, err := httpx.New(
		httpx.WithTimeout(10*time.Second),
		// 最多 3 次尝试(含首次),100ms 起,指数退避,20% 抖动。
		httpx.WithRetry(3, httpx.ExponentialBackoff(100*time.Millisecond, 2, 0.2)),
	)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	resp, err := client.Get(ctx, "https://api.example.com/status")
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	fmt.Printf("状态码:%d\n", resp.StatusCode)
}
