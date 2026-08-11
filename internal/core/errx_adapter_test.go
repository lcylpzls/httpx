package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lcylpzls/errx"
)

func TestErrorStatus(t *testing.T) {
	err := errx.New(errx.KindNotFound, errx.Code("demo_not_found"), "记录不存在")
	if got := ErrorStatus(err); got != http.StatusNotFound {
		t.Fatalf("ErrorStatus = %d，期望 404", got)
	}
	if got := ErrorStatus(nil); got != http.StatusInternalServerError {
		t.Fatalf("nil 错误 ErrorStatus = %d，期望 500", got)
	}
}

func TestWriteErrorJSON(t *testing.T) {
	err := errx.New(errx.KindUnauthorized, errx.Code("demo_unauthorized"), "未登录")
	rec := httptest.NewRecorder()
	WriteErrorJSON(rec, err)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d，期望 401", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", ct)
	}
	var body struct {
		Code    string `json:"code"`
		Kind    string `json:"kind"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON 解析失败：%v", err)
	}
	if body.Code != "demo_unauthorized" || body.Kind != errx.KindUnauthorized.String() || body.Message != "demo_unauthorized: 未登录" {
		t.Fatalf("响应体 = %+v", body)
	}
}

func TestWriteErrorJSONNil(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteErrorJSON(rec, nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("nil 错误状态码 = %d，期望 500", rec.Code)
	}
}
