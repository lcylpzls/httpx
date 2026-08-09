package httpx

import (
	"context"
	"errors"
	"net"

	"github.com/lcylpzls/errx"
)

// 错误码定义:httpx 各失败场景的错误码,统一为 HTX_*。
const (
	// CodeInvalidConfig 配置或请求参数非法。
	CodeInvalidConfig errx.Code = "HTX_INVALID_CONFIG"
	// CodeUnsupportedProtocol 协议未注册(如未导入 http3 子包)。
	CodeUnsupportedProtocol errx.Code = "HTX_UNSUPPORTED_PROTOCOL"
	// CodeDialFailed 建立连接失败。
	CodeDialFailed errx.Code = "HTX_DIAL_FAILED"
	// CodeTLSFailed TLS 握手失败。
	CodeTLSFailed errx.Code = "HTX_TLS_FAILED"
	// CodeRequestFailed 请求发送失败。
	CodeRequestFailed errx.Code = "HTX_REQUEST_FAILED"
	// CodeResponseFailed 读取响应失败。
	CodeResponseFailed errx.Code = "HTX_RESPONSE_FAILED"
	// CodeRetryExhausted 重试耗尽。
	CodeRetryExhausted errx.Code = "HTX_RETRY_EXHAUSTED"
	// CodeBodyTooLarge 响应体超过大小上限。
	CodeBodyTooLarge errx.Code = "HTX_BODY_TOO_LARGE"
	// CodeBodyUnreadable 请求体不可重读,无法重试。
	CodeBodyUnreadable errx.Code = "HTX_BODY_UNREADABLE"
	// CodeRedirectExceeded 重定向次数超限。
	CodeRedirectExceeded errx.Code = "HTX_REDIRECT_EXCEEDED"
	// CodeRedirectFailed 重定向地址解析或构造失败。
	CodeRedirectFailed errx.Code = "HTX_REDIRECT_FAILED"
	// CodeUnexpectedStatus 响应状态码不在期望列表。
	CodeUnexpectedStatus errx.Code = "HTX_UNEXPECTED_STATUS"
)

func init() {
	errx.RegisterCode(CodeInvalidConfig, "配置或请求参数非法")
	errx.RegisterCodeKind(CodeInvalidConfig, errx.KindInvalid)
	errx.RegisterCode(CodeUnsupportedProtocol, "协议未注册")
	errx.RegisterCodeKind(CodeUnsupportedProtocol, errx.KindInvalid)
	errx.RegisterCode(CodeDialFailed, "建立连接失败")
	errx.RegisterCodeKind(CodeDialFailed, errx.KindUnavailable)
	errx.RegisterCode(CodeTLSFailed, "TLS 握手失败")
	errx.RegisterCodeKind(CodeTLSFailed, errx.KindUnavailable)
	errx.RegisterCode(CodeRequestFailed, "请求发送失败")
	errx.RegisterCodeKind(CodeRequestFailed, errx.KindUnavailable)
	errx.RegisterCode(CodeResponseFailed, "读取响应失败")
	errx.RegisterCodeKind(CodeResponseFailed, errx.KindInvalid)
	errx.RegisterCode(CodeRetryExhausted, "重试耗尽")
	errx.RegisterCodeKind(CodeRetryExhausted, errx.KindUnavailable)
	errx.RegisterCode(CodeBodyTooLarge, "响应体超过大小上限")
	errx.RegisterCodeKind(CodeBodyTooLarge, errx.KindInvalid)
	errx.RegisterCode(CodeBodyUnreadable, "请求体不可重读")
	errx.RegisterCodeKind(CodeBodyUnreadable, errx.KindInvalid)
	errx.RegisterCode(CodeRedirectExceeded, "重定向次数超限")
	errx.RegisterCodeKind(CodeRedirectExceeded, errx.KindInvalid)
	errx.RegisterCode(CodeRedirectFailed, "重定向地址解析或构造失败")
	errx.RegisterCodeKind(CodeRedirectFailed, errx.KindInvalid)
	errx.RegisterCode(CodeUnexpectedStatus, "响应状态码不在期望列表")
	errx.RegisterCodeKind(CodeUnexpectedStatus, errx.KindInvalid)
}

// IsTimeout 判断错误是否为超时:
// 命中 context 截止、errx 超时分类(KindTimeout / KindDeadlineExceeded)
// 或实现了 net.Error 且 Timeout() 为 true。
// nil 错误返回 false。
func IsTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	switch errx.KindOf(err) {
	case errx.KindTimeout, errx.KindDeadlineExceeded:
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// IsRetryable 判断错误是否值得重试:
// 命中 errx 可重试分类(KindTimeout / KindRateLimited / KindQuotaExceeded /
// KindUnavailable)或网络层错误(net.Error,请求未完成,配合幂等方法安全)。
// context 取消、nil 与业务错误返回 false。
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errx.Retryable(err) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne)
}
