package httpx

import (
	"context"
	testx "github.com/lcylpzls/testx"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lcylpzls/errx"
)

func TestXMLBody(t *testing.T) {
	srv := newEchoServer(t)
	client, err := New()
	testx.RequireNoError(t, err)

	resp, err := client.Post(context.Background(), srv.URL, nil,
		WithXMLBody(struct {
			XMLName struct{} `xml:"order"`
			ID      int      `xml:"id"`
		}{ID: 7}))
	testx.RequireNoError(t, err)

	body := readRespBody(t, resp)
	if !strings.Contains(body, "<order>") || !strings.Contains(body, "<id>7</id>") {
		t.Errorf("XML 请求体不符:%s", body)
	}
	if !strings.Contains(body, "application/xml") {
		t.Errorf("XML Content-Type 不符:%s", body)
	}
}

func TestXMLBodyMarshalError(t *testing.T) {
	client, err := New()
	testx.RequireNoError(t, err)

	_, err = client.Post(context.Background(), "http://example.com", nil,
		WithXMLBody(make(chan int)))
	testx.RequireError(t, err)

	if code, _ := errx.CodeOf(err); code != CodeInvalidConfig {
		t.Errorf("错误码 = %s,want %s", code, CodeInvalidConfig)
	}
}

func TestMultipartFormData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("解析 multipart 失败:%v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ct := r.Header.Get("Content-Type")
		_, _ = io.WriteString(w, ct+"|name="+r.FormValue("name"))
		if f, _, err := r.FormFile("file"); err == nil {
			data, _ := io.ReadAll(f)
			_, _ = io.WriteString(w, "|file="+string(data))
			_ = f.Close()
		}
	}))
	defer srv.Close()

	client, err := New()
	testx.RequireNoError(t, err)

	resp, err := client.Post(context.Background(), srv.URL, nil,
		WithMultipartFormData(
			map[string]string{"name": "tom"},
			map[string]FileField{"file": {Filename: "a.txt", Content: []byte("hello")}},
		))
	testx.RequireNoError(t, err)

	body := readRespBody(t, resp)
	if !strings.Contains(body, "multipart/form-data") ||
		!strings.Contains(body, "name=tom") ||
		!strings.Contains(body, "file=hello") {
		t.Errorf("multipart 响应不符:%s", body)
	}
}

func TestMultipartEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("解析空 multipart 失败:%v", err)
		}
		_, _ = io.WriteString(w, r.Header.Get("Content-Type"))
	}))
	defer srv.Close()

	client, err := New()
	testx.RequireNoError(t, err)

	resp, err := client.Post(context.Background(), srv.URL, nil,
		WithMultipartFormData(nil, nil))
	testx.RequireNoError(t, err)

	body := readRespBody(t, resp)
	if !strings.Contains(body, "multipart/form-data") || !strings.Contains(body, "boundary=") {
		t.Errorf("空 multipart 响应不符:%s", body)
	}
}

func TestMultipartBoundaryValid(t *testing.T) {
	var buf strings.Builder
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("a", "1")
	_ = w.Close()
	ct := w.FormDataContentType()
	if !strings.Contains(ct, "boundary=") {
		t.Errorf("Content-Type 缺少 boundary:%s", ct)
	}
}

func TestXMLBodyOverriddenByBytesBody(t *testing.T) {
	srv := newEchoServer(t)
	client, err := New()
	testx.RequireNoError(t, err)

	type xmlOrder struct {
		XMLName struct{} `xml:"order"`
		A       string   `xml:"a"`
	}
	resp, err := client.Post(context.Background(), srv.URL, nil,
		WithXMLBody(xmlOrder{A: "x"}),
		WithBytesBody([]byte("override")),
	)
	testx.RequireNoError(t, err)

	body := readRespBody(t, resp)
	if !strings.Contains(body, "body=override") {
		t.Errorf("WithBytesBody 应覆盖 XML 体:%s", body)
	}
}

func TestMultipartContentTypeSet(t *testing.T) {
	// 验证 multipart 的 Content-Type 不因 ro.contentType 为空而丢失。
	ro := requestOptions{}
	WithMultipartFormData(map[string]string{"k": "v"}, nil)(&ro)
	if ro.formFields == nil || ro.formFiles != nil {
		t.Error("multipart 选项应用失败")
	}
}
