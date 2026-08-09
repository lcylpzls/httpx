// http3 示例:通过可选子包启用 HTTP/3(QUIC)。
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/lcylpzls/httpx"
	_ "github.com/lcylpzls/httpx/http3" // 注册 HTTP/3 传输层
)

func main() {
	client, err := httpx.New(
		httpx.WithProtocol(httpx.ProtocolHTTP3),
		httpx.WithTimeout(5*time.Second),
	)
	if err != nil {
		panic(err)
	}
	defer client.CloseIdleConnections()

	resp, err := client.Get(context.Background(), "https://www.cloudflare.com/cdn-cgi/trace")
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	fmt.Printf("协议:%s 状态码:%d\n", resp.Proto, resp.StatusCode)
}
