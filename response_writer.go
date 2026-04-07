package myflame

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
)

type BeforeFunc func(ResponseWriter)
type ResponseWriter interface {
	http.ResponseWriter
	http.Flusher //即时通信的“加速器”
	http.Pusher  //网页加载的“预言家”

	//Status 返回已写入的响应状态码，未写入时返回 0
	Status() int
	//Written 返回是否已开始写入响应
	Written() bool
	//Size 返回已写入的响应体字节数
	Size() int
	//Before 注册一个在写入响应头之前执行的回调（FILO 顺序执行）
	Before(BeforeFunc)
}

type responseWriter struct {
	http.ResponseWriter // 内嵌标准 ResponseWriter

	method      string       // 请求方法（HEAD 请求不写 body）
	status      int32        // 已写入的状态码（原子操作保证并发安全）
	size        int          // 已写入字节数
	beforeFuncs []BeforeFunc // 写前回调列表

	writeHeaderOnce sync.Once // 保证 WriteHeader 只调用一次
}

// NewResponseWriter 创建增强的 ResponseWriter
func NewResponseWriter(method string, w http.ResponseWriter) ResponseWriter {
	return &responseWriter{
		ResponseWriter: w,
		method:         method,
	}
}

// ──────────────────────────────────────────────────────────
// 核心方法
// ──────────────────────────────────────────────────────────
// callBefore 以 FILO 顺序执行所有写前回调
func (w *responseWriter) callBefore() {
	for i := len(w.beforeFuncs) - 1; i >= 0; i-- {
		w.beforeFuncs[i](w)
	}
}

// Before implements [ResponseWriter].
func (w *responseWriter) Before(before BeforeFunc) {
	w.beforeFuncs = append(w.beforeFuncs, before)
}

// Size implements [ResponseWriter].
func (w *responseWriter) Size() int {
	return w.size
}

// Status implements [ResponseWriter].
func (w *responseWriter) Status() int {
	return int(atomic.LoadInt32(&w.status))
}

// Write 写入响应体
func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.Written() {
		//之前都没有调用WriteHeader，默认设为200OK
		w.WriteHeader(http.StatusOK)
	}
	if w.method == http.MethodHead {
		// HEAD 请求：按 HTTP 规范，只返回 Header 不返回 Body
		return 0, nil
	}
	size, err := w.ResponseWriter.Write(b)
	w.size += size
	return size, err
}

// WriteHeader 写入状态码（核心！）
// 用 sync.Once 保证即使被多次调用，底层 WriteHeader 也只执行一次
func (w *responseWriter) WriteHeader(s int) {
	w.writeHeaderOnce.Do(func() {
		if w.Written() {
			return
		}
		w.callBefore()
		w.ResponseWriter.WriteHeader(s)
		atomic.StoreInt32(&w.status, int32(s))
	})
}

// Written implements [ResponseWriter].
func (w *responseWriter) Written() bool {
	return w.Status() != 0
}

// ──────────────────────────────────────────────────────────
// 实现 Hijack/Flush/Push（委托给底层）
// ──────────────────────────────────────────────────────────

// Flush 用于服务器推送事件（SSE）
func (w *responseWriter) Flush() {
	if !w.Written() {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Hijack 用于 WebSocket 升级
func (w *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("底层 ResponseWriter 不支持 Hijacker 接口")
	}
	return hijacker.Hijack()
}

// Push 用于 HTTP/2 服务端推送
func (w *responseWriter) Push(target string, opts *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return errors.New("底层 ResponseWriter 不支持 Pusher 接口")
	}
	return pusher.Push(target, opts)
}
