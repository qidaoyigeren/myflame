package myflame

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/qidaoyigeren/myflame/inject"
	"github.com/qidaoyigeren/myflame/internal/route"
)

// Params 是路由绑定参数的 map
type Params map[string]string

// Context 是每个 HTTP 请求的运行时上下文接口
type Context interface {
	inject.Injector

	ResponseWriter() ResponseWriter
	Request() *Request

	URLPath(name string, pairs ...string) string
	Next()
	RemoteAddr() string
	Redirect(location string, status ...int)

	Params() Params
	Param(name string) string
	ParamInt(name string) int
	ParamInt64(name string) int64

	Query(name string, defaultVal ...string) string
	QueryTrim(name string, defaultVal ...string) string
	QueryBool(name string, defaultVal ...bool) bool
	QueryInt(name string, defaultVal ...int) int
	QueryInt64(name string, defaultVal ...int64) int64
	QueryFloat64(name string, defaultVal ...float64) float64

	SetCookie(cookie http.Cookie)
	Cookie(name string) string
}

// internalContext 是 Context 的内部扩展（框架内部用）
type internalContext interface {
	Context
	setAction(h Handler)
	run()
}

// urlPather 是构建 URL 路径的函数类型
type urlPather func(name string, pairs ...string) string
type context struct {
	inject.Injector

	handlers []Handler // 中间件 + 路由 handler 的完整链
	action   Handler   //最后执行的 action（通常是路由 handler）
	index    int       // 当前执行到链中的第几个 handler

	responseWriter ResponseWriter
	request        *Request
	params         Params
	urlPath        urlPather
}

// newContext 创建一个新的请求上下文
// 这里的三次映射是依赖注入系统在 Context 层面的关键初始化
func newContext(w http.ResponseWriter, r *http.Request, params route.Params,
	handlers []Handler, urlPath urlPather) internalContext {
	c := &context{
		Injector:       inject.New(),
		handlers:       handlers,
		responseWriter: NewResponseWriter(r.Method, w),
		request:        &Request{Request: r},
		params:         Params(params),
		urlPath:        urlPath,
	}
	// ① 将自身映射为 Context 接口 → handler 可以声明 func(Context)
	c.MapTo(c, (*Context)(nil))
	// ② 将 responseWriter 映射为 http.ResponseWriter → 兼容标准 handler
	c.MapTo(c.responseWriter, (*http.ResponseWriter)(nil))
	// ③ 映射原始 *http.Request → 兼容 func(r *http.Request)
	c.Map(r)
	return c
}

// ordinalize 将数字转换为序数词 (1 -> "1st", 2 -> "2nd", 3 -> "3rd")
func ordinalize(n int) string {
	switch n % 100 {
	case 11, 12, 13:
		return fmt.Sprintf("%dth", n)
	}
	switch n % 10 {
	case 1:
		return fmt.Sprintf("%dst", n)
	case 2:
		return fmt.Sprintf("%dnd", n)
	case 3:
		return fmt.Sprintf("%drd", n)
	default:
		return fmt.Sprintf("%dth", n)
	}
}

// run 是框架的核心执行引擎，按序执行 handlers 链
// 每个中间件通过调用 c.Next() 来触发下一个 handler
func (c *context) run() {
	for c.index <= len(c.handlers) {
		// 检查请求是否已被取消（客户端断开/超时）
		select {
		case <-c.Request().Context().Done():
			return
		default:
		}
		// 确定本次要执行的 handler
		var h Handler
		if c.index == len(c.handlers) {
			h = c.action // 全部 handler 执行完毕后，执行最终 action
		} else {
			h = c.handlers[c.index] // 否则执行下一个中间件
		}
		if h == nil {
			c.index++
			return
		}
		// 通过依赖注入调用 handler
		vals, err := c.Invoke(h)
		if err != nil {
			ordinal := ordinalize(c.index + 1)
			panic(fmt.Errorf("%s handler failed: %w", ordinal, err))
		}
		c.index++
		// 如果 handler 有返回值，交给 ReturnHandler 处理
		if len(vals) > 0 {
			ev := c.Value(reflect.TypeOf(ReturnHandler(nil)))
			handleReturn := ev.Interface().(ReturnHandler)
			handleReturn(c, vals)
		}
		//如果响应已写入（ResponseWriter.Written()），退出执行链
		if c.responseWriter.Written() {
			return
		}
	}
}
func (c *context) Next() {
	c.index++
	c.run()
}

// Cookie implements [internalContext].
func (c *context) Cookie(name string) string {
	cookie, err := c.Request().Cookie(name)
	if err != nil {
		return ""
	}
	v, err := url.QueryUnescape(cookie.Value)
	if err != nil {
		return cookie.Value
	}
	return v
}

// SetCookie implements [internalContext].
func (c *context) SetCookie(cookie http.Cookie) {
	cookie.Value = url.QueryEscape(cookie.Value)
	http.SetCookie(c.responseWriter, &cookie)
}

// Param implements [internalContext].
func (c *context) Param(name string) string {
	return c.params[name]
}
func (c *context) Params() Params { return c.params }
func (c *context) ParamInt(name string) int {
	v, _ := strconv.Atoi(c.params[name])
	return v
}

func (c *context) ParamInt64(name string) int64 {
	v, _ := strconv.ParseInt(c.params[name], 10, 64)
	return v
}

// Query implements [internalContext].
func (c *context) Query(name string, defaultVal ...string) string {
	val := c.Request().URL.Query().Get(name)
	if val == "" || len(defaultVal) > 0 {
		return defaultVal[0]
	}
	return val
}

func (c *context) QueryBool(name string, defaultVal ...bool) bool {
	v := c.Query(name)
	if v == "" && len(defaultVal) > 0 {
		return defaultVal[0]
	}
	b, _ := strconv.ParseBool(v)
	return b
}

func (c *context) QueryFloat64(name string, defaultVal ...float64) float64 {
	v := c.Query(name)
	if v == "" && len(defaultVal) > 0 {
		return defaultVal[0]
	}

	f, _ := strconv.ParseFloat(v, 64)
	return f
}

func (c *context) QueryInt(name string, defaultVal ...int) int {
	v, err := strconv.Atoi(c.Query(name))
	if err != nil && len(defaultVal) > 0 {
		return defaultVal[0]
	}
	return v
}
func (c *context) QueryInt64(name string, defaultVal ...int64) int64 {
	v := c.Query(name)
	if v == "" && len(defaultVal) > 0 {
		return defaultVal[0]
	}

	i, _ := strconv.ParseInt(v, 10, 64)
	return i
}

// QueryTrim implements [internalContext].
func (c *context) QueryTrim(name string, defaultVal ...string) string {
	return strings.TrimSpace(c.Query(name, defaultVal...))
}

// Redirect implements [internalContext].
func (c *context) Redirect(location string, status ...int) {
	code := http.StatusFound
	if len(status) > 0 {
		code = status[0]
	}
	http.Redirect(c.responseWriter, c.Request().Request, location, code)
}

// RemoteAddr implements [internalContext].
func (c *context) RemoteAddr() string {
	// 三级回退：X-Real-IP → X-Forwarded-For → RemoteAddr
	// 第1优先级：X-Real-IP（Nginx 常用）
	if ip := c.Request().Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	// 第2优先级：X-Forwarded-For（多个代理的情况）
	if ip := c.Request().Header.Get("X-Forwarded-For"); ip != "" {
		// 格式："1.2.3.4, 5.6.7.8, 9.10.11.12"
		// 第一个是最原始的用户IP
		return strings.TrimSpace(strings.Split(ip, ",")[0])
	}
	// 第3优先级：直接连接（没有代理）
	host, _, _ := net.SplitHostPort(c.Request().RemoteAddr)
	return host
}

// Request implements [internalContext].
func (c *context) Request() *Request {
	return c.request
}

// ResponseWriter implements [internalContext].
func (c *context) ResponseWriter() ResponseWriter {
	return c.responseWriter
}

// URLPath implements [internalContext].
func (c *context) URLPath(name string, pairs ...string) string {
	return c.urlPath(name, pairs...)
}

// setAction implements [internalContext].
func (c *context) setAction(h Handler) {
	c.action = h
}
