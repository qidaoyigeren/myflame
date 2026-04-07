package inject

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

type MyService struct {
	Name string
}
type MyInterface interface {
	Hello() string
}

type specialString interface{}

type testStruct struct {
	Dep1 string        `inject:"" json:"-"`
	Dep2 specialString `inject:""`
	Dep3 string
}
type myFastInvoker func(string)

func (myFastInvoker) Invoke([]interface{}) ([]reflect.Value, error) {
	return nil, nil
}
func TestInjector_Invoke(t *testing.T) {
	t.Run("invoke functions", func(t *testing.T) {
		inj := New()
		dep := "some dependency"
		inj.Map(dep)
		dep2 := "another dep"
		inj.MapTo(dep2, (*specialString)(nil))
		dep3 := make(chan *specialString)
		dep4 := make(chan *specialString)
		typRecv := reflect.ChanOf(reflect.RecvDir, reflect.TypeOf(dep3).Elem())
		typSend := reflect.ChanOf(reflect.SendDir, reflect.TypeOf(dep4).Elem())
		inj.Set(typRecv, reflect.ValueOf(dep3))
		inj.Set(typSend, reflect.ValueOf(dep4))
		_, err := inj.Invoke(func(d1 string, d2 specialString, d3 <-chan *specialString, d4 chan<- *specialString) {
			assert.Equal(t, dep, d1)
			assert.Equal(t, dep2, d2)
			assert.Equal(t, reflect.TypeOf(dep3).Elem(), reflect.TypeOf(d3).Elem())
			assert.Equal(t, reflect.TypeOf(dep4).Elem(), reflect.TypeOf(d4).Elem())
			assert.Equal(t, reflect.RecvDir, reflect.TypeOf(d3).ChanDir())
			assert.Equal(t, reflect.SendDir, reflect.TypeOf(d4).ChanDir())
		})
		assert.Nil(t, err)

		_, err = inj.Invoke(myFastInvoker(func(string) {}))
		assert.Nil(t, err)
	})

	t.Run("invoke functions with return values", func(t *testing.T) {
		inj := New()

		dep := "some dependency"
		inj.Map(dep)
		dep2 := "another dep"
		inj.MapTo(dep2, (*specialString)(nil))

		result, err := inj.Invoke(func(d1 string, d2 specialString) string {
			assert.Equal(t, dep, d1)
			assert.Equal(t, dep2, d2)
			return "Hello world"
		})
		assert.Nil(t, err)

		assert.Equal(t, "Hello world", result[0].String())
	})
}
func TestInjector_Apply(t *testing.T) {
	inj := New()
	inj.Map("a dep").MapTo("another dep", (*specialString)(nil)) //方法链式调用

	s := testStruct{}
	assert.Nil(t, inj.Apply(&s))

	assert.Equal(t, "a dep", s.Dep1)
	assert.Equal(t, "another dep", s.Dep2)
}
func TestInjector_InterfaceOf(t *testing.T) {
	of := InterfaceOf((*MyInterface)(nil))
	assert.Equal(t, reflect.Interface, of.Kind())
	iType := InterfaceOf((**MyInterface)(nil))
	assert.Equal(t, reflect.Interface, iType.Kind())
}
func TestInjector_Set(t *testing.T) {
	/*
		go的反射不允许直接实例化纯单向通道，必须先创建双向，
		然后将他视作单向使用。这在依赖注入场景中很常见，
		因为注入器需要按单向类型存储，但实际存储的必须是双向通道实例。
	*/
	inj := New()
	typ := reflect.TypeOf("string")
	typSend := reflect.ChanOf(reflect.SendDir, typ)
	typRecv := reflect.ChanOf(reflect.RecvDir, typ)
	chanSend := reflect.MakeChan(reflect.ChanOf(reflect.BothDir, typ), 0)
	chanRecv := reflect.MakeChan(reflect.ChanOf(reflect.BothDir, typ), 0)
	inj.Set(typSend, chanSend)
	inj.Set(typRecv, chanRecv)
	/*
		不能直接创造单向的通道，
		但是因为本质上单项只是在编译时限制了某些行为，
		所以用单项的reflect.Type作为键，双向通道作为值
	*/
	assert.True(t, inj.Value(typSend).IsValid())
	assert.True(t, inj.Value(typRecv).IsValid())
	assert.False(t, inj.Value(chanSend.Type()).IsValid())
}
func TestInjector_GetVal(t *testing.T) {
	inj := New()
	inj.Map("some dependency")
	assert.True(t, inj.Value(reflect.TypeOf("string")).IsValid())
	assert.False(t, inj.Value(reflect.TypeOf(1)).IsValid())
}
func TestInjector_SetParent(t *testing.T) {
	inj := New()
	inj2 := New()
	inj2.SetParent(inj)
	inj.MapTo("some dependency", (*MyInterface)(nil))
	assert.True(t, inj2.Value(InterfaceOf((*MyInterface)(nil))).IsValid())
}

type greeter struct {
	Name string
}

func (g *greeter) String() string {
	return "Hello, My name is" + g.Name
}

// 注入器能否通过接口类型（fmt.Stringer）找到已经注册的具体实现（*greeter）
func TestInjector_Implementors(t *testing.T) {
	inj := New()
	inj.Map(&greeter{Name: "John"})
	assert.True(t, inj.Value(InterfaceOf((*fmt.Stringer)(nil))).IsValid())
}

func TestIsFastInvoker(t *testing.T) {
	assert.True(t, IsFastInvoker(myFastInvoker(nil)))
}

func BenchmarkInjector_Invoke(b *testing.B) {
	inj := New()
	inj.Map("some dependency").MapTo("another dep", (*specialString)(nil))

	fn := func(d1 string, d2 specialString) string { return "something" }
	for i := 0; i < b.N; i++ {
		_, _ = inj.Invoke(fn)
	}
}

type testFastInvoker func(d1 string, d2 specialString) string

func (f testFastInvoker) Invoke(args []interface{}) ([]reflect.Value, error) {
	f(args[0].(string), args[1].(specialString))
	return nil, nil
}

func BenchmarkInjector_FastInvoke(b *testing.B) {
	inj := New()
	inj.Map("some dependency").MapTo("another dep", (*specialString)(nil))

	fn := testFastInvoker(func(d1 string, d2 specialString) string { return "something" })
	for i := 0; i < b.N; i++ {
		_, _ = inj.Invoke(fn)
	}
}
