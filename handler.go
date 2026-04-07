package myflame

import (
	"fmt"
	"net/http"
	"reflect"

	"github.com/qidaoyigeren/myflame/inject"
)

// Handler 是框架中任何可调用函数的统称。
// 框架通过反射分析 Handler 的参数类型，并从注入器中自动提供对应的值。
// 如果参数类型在注入器中找不到，框架会在运行时 panic。
type Handler interface{}

//实现三种内置FastInvoker，以「注册时开销」换「请求时性能」

// ──────────────────────────────────────────────────────────
// FastInvoker #1：处理 func(Context) 签名
// ──────────────────────────────────────────────────────────

// 编译期检查：确保 ContextInvoker 实现了 inject.FastInvoker
var _ inject.FastInvoker = (*ContextInvoker)(nil)

type ContextInvoker func(ctx Context)

// Invoke implements [inject.FastInvoker].
func (invoke ContextInvoker) Invoke(args []interface{}) ([]reflect.Value, error) {
	invoke(args[0].(Context))
	return nil, nil
}

// ──────────────────────────────────────────────────────────
// FastInvoker #2：处理 func(http.ResponseWriter, *http.Request) 签名
// ──────────────────────────────────────────────────────────
var _ inject.FastInvoker = (*httpHandlerFuncInvoker)(nil)

type httpHandlerFuncInvoker func(http.ResponseWriter, *http.Request)

func (invoke httpHandlerFuncInvoker) Invoke(args []interface{}) ([]reflect.Value, error) {
	invoke(args[0].(http.ResponseWriter), args[1].(*http.Request))
	return nil, nil
}

// ──────────────────────────────────────────────────────────
// FastInvoker #3：处理 func() (int, string) 签名
// ──────────────────────────────────────────────────────────
var _ inject.FastInvoker = (*teapotInvoker)(nil)

// teapotInvoker 处理返回「状态码 + 响应体」的简单 handler
type teapotInvoker func() (int, string)

func (invoke teapotInvoker) Invoke(_ []interface{}) ([]reflect.Value, error) {
	ret1, ret2 := invoke()
	return []reflect.Value{
		reflect.ValueOf(ret1),
		reflect.ValueOf(ret2),
	}, nil
}

// validateAndWrapHandler 验证 handler 合法性，并尝试将其包装为 FastInvoker。
// 这是框架性能优化的关键：在路由注册时一次性完成包装，请求处理时直接用。
func validateAndWrapHandler(h Handler, wrapper func(Handler) Handler) Handler {
	if reflect.TypeOf(h).Kind() != reflect.Func {
		panic(fmt.Sprintf("handler must be a callable function, but got %T", h))
	}
	if inject.IsFastInvoker(h) {
		return h
	}
	switch v := h.(type) {
	case func(Context):
		return ContextInvoker(v)
	case func(http.ResponseWriter, *http.Request):
		return httpHandlerFuncInvoker(v)
	case http.HandlerFunc:
		return httpHandlerFuncInvoker(v)
	case func() (int, string):
		return teapotInvoker(v)
	}
	if wrapper != nil {
		h = wrapper(h)
	}
	return h
}

// validateAndWrapHandlers 对一组 handler 批量处理
func validateAndWrapHandlers(hs []Handler, wrapper func(Handler) Handler) {
	for i, h := range hs {
		hs[i] = validateAndWrapHandler(h, wrapper)
	}
}
