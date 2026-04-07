package myflame

import (
	"io"
	"net/http"
)

// Request 是 http.Request 的简单包装，增加了便捷方法
type Request struct {
	*http.Request
}

// Body 返回请求体的便捷读取器
func (r *Request) Body() *RequestBody {
	return &RequestBody{reader: r.Request.Body}
}

// RequestBody 封装请求体，提供多种读取方式
type RequestBody struct {
	reader io.ReadCloser
}

// Bytes 读取全部请求体为字节切片
func (r *RequestBody) Bytes() ([]byte, error) {
	return io.ReadAll(r.reader)
}

// String 读取全部请求体为字符串
func (r *RequestBody) String() (string, error) {
	data, err := r.Bytes()
	return string(data), err
}

// ReadCloser 获取原始 ReadCloser（用于流式处理）
func (r *RequestBody) ReadCloser() io.ReadCloser {
	return r.reader
}
