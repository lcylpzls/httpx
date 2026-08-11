package core

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"os"

	"github.com/lcylpzls/errx"
)

// defaultMaxJSONBytes 是 JSON 助手的默认响应体大小上限(16MiB),
// 防止内存被打爆。
const defaultMaxJSONBytes int64 = 16 << 20

// ReadBody 读取响应体并统一关闭 Body。
// maxBytes 为大小上限,超过时返回 HTX_BODY_TOO_LARGE 且 Body 已关闭。
func ReadBody(resp *http.Response, maxBytes int64) ([]byte, error) {
	if resp == nil {
		return nil, errx.NewCode(CodeResponseFailed, "响应不能为空")
	}
	if maxBytes <= 0 {
		return nil, errx.NewCode(CodeInvalidConfig, "响应体大小上限必须为正数")
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
			return nil, errx.NewCode(CodeBodyTooLarge, "响应体超过大小上限")
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
		return errx.WrapCode(err, CodeResponseFailed, "响应 JSON 解析失败")
	}
	return nil
}

// ReadFile 将响应体写入文件并统一关闭 Body,上限语义同 ReadBody。
func ReadFile(resp *http.Response, path string, maxBytes int64) error {
	data, err := ReadBody(resp, maxBytes)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return errx.Wrap(err, errx.KindInternal, CodeResponseFailed, "写入文件失败")
	}
	return nil
}

// streamChunkSize 是 ReadStream 的单块读取大小。
const streamChunkSize = 32 * 1024

// ReadStream 逐块读取响应体并回调 fn,统一关闭 Body。
// maxBytes 为大小上限,超出立即返回 HTX_BODY_TOO_LARGE;
// fn 返回错误时终止读取并返回 HTX_RESPONSE_FAILED。
func ReadStream(resp *http.Response, fn func([]byte) error, maxBytes int64) error {
	if resp == nil {
		return errx.NewCode(CodeResponseFailed, "响应不能为空")
	}
	if maxBytes <= 0 {
		return errx.NewCode(CodeInvalidConfig, "响应体大小上限必须为正数")
	}
	if resp.Body == nil {
		return nil
	}
	defer resp.Body.Close()
	buf := make([]byte, streamChunkSize)
	var total int64
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			total += int64(n)
			if total > maxBytes {
				return errx.NewCode(CodeBodyTooLarge, "响应体超过大小上限")
			}
			if err := fn(buf[:n]); err != nil {
				return errx.Wrap(err, errx.KindCancelled, CodeResponseFailed, "流式回调终止")
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return errx.Wrap(err, errx.KindUnavailable, CodeResponseFailed, "读取响应体失败")
		}
	}
}

// statusSummaryLimit 是 EnsureStatus 错误信息中响应体摘要的最大字节数。
const statusSummaryLimit = 512

// EnsureStatus 校验响应状态码在允许列表中。
// 命中时返回 nil 且不关闭 Body(调用方可继续读取);
// 未命中时读取并关闭 Body,返回 HTX_UNEXPECTED_STATUS,
// 错误附带 status 与响应体摘要字段。
func EnsureStatus(resp *http.Response, codes ...int) error {
	if resp == nil {
		return errx.NewCode(CodeResponseFailed, "响应不能为空")
	}
	for _, code := range codes {
		if resp.StatusCode == code {
			return nil
		}
	}
	summary := ""
	if resp.Body != nil {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, statusSummaryLimit))
		_ = resp.Body.Close()
		summary = string(data)
	}
	return errx.NewCodef(CodeUnexpectedStatus,
		"响应状态码 %d 不在允许列表", resp.StatusCode).
		WithField("status", resp.StatusCode).
		WithField("body", summary)
}
