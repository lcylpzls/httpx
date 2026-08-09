// basic 示例:创建客户端、发起请求并解析 JSON 响应。
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/lcylpzls/httpx"
)

// User 是示例响应结构。
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func main() {
	client, err := httpx.New(httpx.WithTimeout(5 * time.Second))
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	resp, err := client.Get(ctx, "https://api.example.com/users/1",
		httpx.WithHeader("X-Project", "demo"))
	if err != nil {
		panic(err)
	}

	var user User
	if err := httpx.JSON(resp, &user); err != nil {
		panic(err)
	}
	fmt.Printf("用户:%d %s\n", user.ID, user.Name)
}
