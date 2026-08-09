package httpx

import (
	"encoding/json"
	"io"
	"math"
	"net/http"

	"github.com/lcylpzls/errx"
)

// defaultMaxJSONBytes 是 JSON 助手的默认响应体大小上限(16MiB),
// 防止内存被打爆。
const defaultMaxJSONBytes int64 = 16 << 20

// ReadBody 读取响应体并统一关闭 Body。
// maxBytes 为大小上限,超过时返回 HTX_BODY_TOO_LARGE 且 Body 已关闭。
func ReadBody(resp *http.Response, maxBytes int64) ([]byte, error) {
	if resp == nil {
		return nil, errx.New(errx.KindInvalid, CodeResponseFailed, "响应不能为空")
	}
	if maxBytes <= 0 {
		return nil, errx.New(errx.KindInvalid, CodeInvalidConfig, "响应体大小上限必须为正数")
	}
	if resp.Body == nil {
		return []byte{}, nil
	}
	defer resp.Body.Close()
	var data []byte
	var err error
	if maxBytes == math.MaxInt64 {
		data, err = io.ReadAll(resp.Body)
	} else {
		data, err = io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
		if err == nil && int64(len(data)) > maxBytes {
			return nil, errx.New(errx.KindInvalid, CodeBodyTooLarge, "响应体超过大小上限")
		}
	}
	if err != nil {
		return nil, errx.Wrap(err, errx.KindUnavailable, CodeResponseFailed, "读取响应体失败")
	}
	return data, nil
}

// ReadString 读取响应体为字符串并统一关闭 Body,上限语义同 ReadBody。
func ReadString(resp *http.Response, maxBytes int64) (string, error) {
	data, err := ReadBody(resp, maxBytes)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// JSON 将响应体解析为 out 并统一关闭 Body。
// 内置 16MiB 大小上限;JSON 解析失败返回 HTX_RESPONSE_FAILED。
func JSON(resp *http.Response, out any) error {
	data, err := ReadBody(resp, defaultMaxJSONBytes)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return errx.Wrap(err, errx.KindInvalid, CodeResponseFailed, "响应 JSON 解析失败")
	}
	return nil
}
