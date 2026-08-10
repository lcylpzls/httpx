package httpx

import (
	"errors"
	testx "github.com/lcylpzls/testx"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lcylpzls/errx"
)

func TestReadBody(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(strings.NewReader("hello"))}
	data, err := ReadBody(resp, 1024)
	testx.RequireNoError(t, err)

	if string(data) != "hello" {
		t.Errorf("内容 = %q,want hello", data)
	}
}

func TestReadBodyTooLarge(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(strings.NewReader("hello"))}
	_, err := ReadBody(resp, 3)
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeBodyTooLarge {
		t.Errorf("错误码 = %s,want %s", code, CodeBodyTooLarge)
	}
}

func TestReadBodyReadError(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(&errorReader{})}
	_, err := ReadBody(resp, 1024)
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeResponseFailed {
		t.Errorf("错误码 = %s,want %s", code, CodeResponseFailed)
	}
}

func TestReadBodyNilResponse(t *testing.T) {
	_, err := ReadBody(nil, 1024)
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeResponseFailed {
		t.Errorf("错误码 = %s,want %s", code, CodeResponseFailed)
	}
}

func TestReadBodyNilBody(t *testing.T) {
	data, err := ReadBody(&http.Response{}, 1024)
	testx.RequireNoError(t, err)

	if len(data) != 0 {
		t.Errorf("内容应为空:%q", data)
	}
}

func TestReadBodyInvalidLimit(t *testing.T) {
	for _, limit := range []int64{0, -1} {
		if _, err := ReadBody(&http.Response{}, limit); err == nil {
			t.Fatalf("limit=%d 应返回错误", limit)
		} else if code, _ := errx.CodeOf(err); code != CodeInvalidConfig {
			t.Errorf("错误码 = %s,want %s", code, CodeInvalidConfig)
		}
	}
}

func TestReadBodyMaxInt64(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(strings.NewReader("ok"))}
	data, err := ReadBody(resp, math.MaxInt64)
	testx.RequireNoError(t, err)

	if string(data) != "ok" {
		t.Errorf("内容 = %q,want ok", data)
	}
}

func TestReadBodyClosesBody(t *testing.T) {
	body := &closeRecorder{Reader: strings.NewReader("x")}
	if _, err := ReadBody(&http.Response{Body: body}, 1024); err != nil {
		t.Fatal(err)
	}
	if !body.closed {
		t.Error("ReadBody 应关闭 Body")
	}
}

func TestReadString(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(strings.NewReader("text"))}
	s, err := ReadString(resp, 1024)
	testx.RequireNoError(t, err)

	testx.Equal(t, s, "text")

}

func TestReadStringTooLarge(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(strings.NewReader("toolong"))}
	_, err := ReadString(resp, 3)
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeBodyTooLarge {
		t.Errorf("错误码 = %s,want %s", code, CodeBodyTooLarge)
	}
}

func TestJSON(t *testing.T) {
	type user struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(`{"name":"tom","age":3}`))}
	var u user
	if err := JSON(resp, &u); err != nil {
		t.Fatalf("JSON 解析失败:%v", err)
	}
	if u.Name != "tom" || u.Age != 3 {
		t.Errorf("解析结果不符:%+v", u)
	}
}

func TestJSONInvalidBody(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(strings.NewReader("{bad"))}
	var out map[string]any
	err := JSON(resp, &out)
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeResponseFailed {
		t.Errorf("错误码 = %s,want %s", code, CodeResponseFailed)
	}
}

func TestJSONNilOutput(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(`{}`))}
	if err := JSON(resp, nil); err == nil {
		t.Fatal("nil out 应返回错误")
	}
}

func TestJSONTooLarge(t *testing.T) {
	payload := strings.Repeat("a", int(defaultMaxJSONBytes)+1)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(payload))}
	var out map[string]any
	err := JSON(resp, &out)
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeBodyTooLarge {
		t.Errorf("错误码 = %s,want %s", code, CodeBodyTooLarge)
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	resp := &http.Response{Body: io.NopCloser(strings.NewReader("file content"))}
	if err := ReadFile(resp, path, 1024); err != nil {
		t.Fatalf("ReadFile 失败:%v", err)
	}
	data, err := os.ReadFile(path)
	testx.RequireNoError(t, err)

	if string(data) != "file content" {
		t.Errorf("文件内容 = %q", data)
	}
}

func TestReadFileWriteError(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(strings.NewReader("x"))}
	err := ReadFile(resp, filepath.Join(t.TempDir(), "no", "dir", "x.txt"), 1024)
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeResponseFailed {
		t.Errorf("错误码 = %s,want %s", code, CodeResponseFailed)
	}
}

func TestReadFileTooLarge(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(strings.NewReader("toolong"))}
	err := ReadFile(resp, filepath.Join(t.TempDir(), "x.txt"), 3)
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeBodyTooLarge {
		t.Errorf("错误码 = %s,want %s", code, CodeBodyTooLarge)
	}
}

// errorReader 读取时恒返回错误。
type errorReader struct{}

func (*errorReader) Read([]byte) (int, error) {
	return 0, errors.New("模拟读取失败")
}

// closeRecorder 记录 Close 调用。
type closeRecorder struct {
	*strings.Reader
	closed bool
}

func (c *closeRecorder) Close() error {
	c.closed = true
	return nil
}
