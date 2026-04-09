package myflame

import (
	"net/http"
	"reflect"

	"github.com/qidaoyigeren/myflame/inject"
)

// ReturnHandler 是一个服务：当 route handler 有返回值时，
// 由它负责将返回值转成 HTTP 响应写入 ResponseWriter。
type ReturnHandler func(Context, []reflect.Value)

// defaultReturnHandler 返回框架内置的默认返回值处理器
/*
f.Get("/users", func() string { return "hello" })
f.Get("/create", func() (int, string) { return 201, "created" })
f.Get("/fail", func() error { return errors.New("oops") })
*/
func defaultReturnHandler() ReturnHandler {
	// 辅助：判断是否可以解引用（接口/指针）
	canDeref := func(val reflect.Value) bool {
		return val.Kind() == reflect.Interface || val.Kind() == reflect.Ptr
	}
	// 辅助：判断是否是 []byte
	isByteSlice := func(val reflect.Value) bool {
		return val.Kind() == reflect.Slice && val.Type().Elem().Kind() == reflect.Uint8
	}
	return func(c Context, vals []reflect.Value) {
		//从注入器中取出ResponseWriter
		v := c.Value(inject.InterfaceOf((*http.ResponseWriter)(nil)))
		w := v.Interface().(http.ResponseWriter)
		var respVal reflect.Value
		switch len(vals) {
		case 1:
			// 单返回值：string / []byte / error
			respVal = vals[0]
		case 2:
			// (int, string/[]byte/error)：第一个是状态码
			if vals[0].Kind() == reflect.Int {
				w.WriteHeader(int(vals[0].Int()))
				respVal = vals[1]
				break
			}
			// (string/[]byte, error)：优先用 string/[]byte，有 error 用 error
			if vals[0].Kind() == reflect.String || isByteSlice(vals[0]) {
				respVal = vals[0]
				if _, ok := vals[1].Interface().(error); ok {
					respVal = vals[1] // error 优先级更高
				}
			}
		}
		if !respVal.IsValid() {
			return // 不认识的返回值，忽略
		}
		// 有 error 且不为 nil → 500
		if err, ok := respVal.Interface().(error); ok && err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			return
		}
		// 零值（nil 或空字符串） → 不写内容
		if respVal.IsZero() {
			return
		}
		//解引用接口/指针
		if canDeref(respVal) {
			respVal = respVal.Elem()
		}
		//写响应体
		if isByteSlice(respVal) {
			_, _ = w.Write(respVal.Bytes())
		} else {
			_, _ = w.Write([]byte(respVal.String()))
		}
	}
}
