package httpx

import (
	"context"
	"errors"
	"testing"

	"github.com/lcylpzls/errx"
	"github.com/lcylpzls/testx"
)

// fakeNetError 实现 net.Error,用于覆盖超时/临时性网络错误分支。
type fakeNetError struct {
	timeout bool
}

func (e fakeNetError) Error() string { return "模拟网络错误" }
func (e fakeNetError) Timeout() bool { return e.timeout }
func (e fakeNetError) Temporary() bool {
	return false
}

func TestErrorCodesRegistered(t *testing.T) {
	codes := []errx.Code{
		CodeInvalidConfig,
		CodeUnsupportedProtocol,
		CodeDialFailed,
		CodeTLSFailed,
		CodeRequestFailed,
		CodeResponseFailed,
		CodeRetryExhausted,
		CodeBodyTooLarge,
		CodeBodyUnreadable,
		CodeRedirectExceeded,
		CodeRedirectFailed,
	}
	registered := map[errx.Code]bool{}
	for _, c := range errx.Codes() {
		registered[c.Code] = true
	}
	for _, code := range codes {
		testx.RequireTrue(t, registered[code])
		testx.RequireNotEmpty(t, errx.Describe(code))
	}
}

func TestIsTimeout(t *testing.T) {
	plain := errors.New("普通错误")
	timeoutErr := errx.New(errx.KindTimeout, CodeRequestFailed, "请求超时")
	deadlineErr := errx.New(errx.KindDeadlineExceeded, CodeRequestFailed, "超出截止时间")
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain", plain, false},
		{"deadline exceeded", context.DeadlineExceeded, true},
		{"wrapped deadline", errx.Wrap(context.DeadlineExceeded, errx.KindUnavailable, CodeRequestFailed, "包装超时"), true},
		{"kind timeout", timeoutErr, true},
		{"kind deadline", deadlineErr, true},
		{"net error timeout", fakeNetError{timeout: true}, true},
		{"net error no timeout", fakeNetError{}, false},
	}
	for _, tc := range cases {
		testx.RequireEqual(t, IsTimeout(tc.err), tc.want)
	}
}

func TestIsRetryable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain", errors.New("普通错误"), false},
		{"canceled", context.Canceled, false},
		{"wrapped canceled", errx.Wrap(context.Canceled, errx.KindCancelled, CodeRequestFailed, "已取消"), false},
		{"deadline", context.DeadlineExceeded, true},
		{"kind unavailable", errx.New(errx.KindUnavailable, CodeDialFailed, "连接失败"), true},
		{"kind rate limited", errx.New(errx.KindRateLimited, CodeRequestFailed, "限流"), true},
		{"kind business", errx.New(errx.KindBusiness, CodeInvalidConfig, "业务错误"), false},
		{"net timeout", fakeNetError{timeout: true}, true},
		{"net plain", fakeNetError{}, true},
	}
	for _, tc := range cases {
		testx.RequireEqual(t, IsRetryable(tc.err), tc.want)
	}
}

// FuzzIsTimeout 保证超时判定助手对任意错误不 panic、结果稳定。
func FuzzIsTimeout(f *testing.F) {
	f.Add("")
	f.Add("deadline exceeded")
	f.Add("request timeout")
	f.Fuzz(func(t *testing.T, msg string) {
		err := errors.New(msg)
		_ = IsTimeout(err)
		_ = IsTimeout(errx.Wrap(err, errx.KindTimeout, CodeRequestFailed, "包装"))
	})
}

// FuzzIsRetryable 保证重试判定助手对任意错误不 panic、结果稳定。
func FuzzIsRetryable(f *testing.F) {
	f.Add("")
	f.Add("dial tcp: connection refused")
	f.Fuzz(func(t *testing.T, msg string) {
		err := errors.New(msg)
		_ = IsRetryable(err)
		_ = IsRetryable(errx.Wrap(err, errx.KindUnavailable, CodeDialFailed, "包装"))
	})
}
