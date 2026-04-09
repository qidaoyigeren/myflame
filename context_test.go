package myflame

import (
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/qidaoyigeren/myflame/internal/route"
)

func TestContext_Run_Chain(t *testing.T) {
	// 测试：三个 handler 按序执行
	order := []int{}
	handlers := []Handler{
		func(c Context) { order = append(order, 1); c.Next() },
		func(c Context) { order = append(order, 2); c.Next() },
		func(c Context) { order = append(order, 3) },
	}

	// 创建模拟的 HTTP 请求和响应
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	// 构建 URL 路径生成函数（测试中不需要真实逻辑）
	urlPath := func(name string, pairs ...string) string { return "" }

	// 创建上下文，传入 handlers 和空的路由参数
	ctx := newContext(w, req, route.Params{}, handlers, urlPath)

	// 执行处理链
	ctx.run()

	// 断言执行顺序正确
	expected := []int{1, 2, 3}
	if !reflect.DeepEqual(order, expected) {
		t.Errorf("expected order %v, got %v", expected, order)
	}
}
