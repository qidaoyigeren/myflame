package inject

import (
	"fmt"
	"reflect"
)

// TypeMapper:类型映射能力
type TypeMapper interface {
	//Map以值的实际类型为key注册映射
	Map(...interface{}) TypeMapper
	//MapTo以ifacePtr指向的接口类型为key注册映射
	//用法：MapTo(myImpl,(*MyInterface)(nil))
	MapTo(interface{}, interface{}) TypeMapper
	//Set直接用reflect.Type和reflect.Value注册
	Set(reflect.Type, reflect.Value) TypeMapper
	// Value 根据类型查找已注册的值
	Value(reflect.Type) reflect.Value
}

// Applicator:结构体字段注入能力
type Applicator interface {
	//Apply将注入器中的值注入到结构体中带`inject`tag的字段
	Apply(interface{}) error
}

// Invoker:函数调用能力（通过反射，自动注入参数）
type Invoker interface {
	Invoke(interface{}) ([]reflect.Value, error)
}

// FastInvoker：高性能调用接口（跳过反射，直接调用）
// 实现此接口可获得约3倍性能提升
type FastInvoker interface {
	Invoke([]interface{}) ([]reflect.Value, error)
}

// IsFastInvoker 断言判断 handler 是否实现了 FastInvoker
func IsFastInvoker(handler interface{}) bool {
	_, ok := handler.(FastInvoker)
	return ok
}

// Injector：组合以上所有能力，并支持父注入器
type Injector interface {
	Applicator
	Invoker
	TypeMapper
	SetParent(Injector)
}

// injector是Injector接口具体实现
type injector struct {
	values map[reflect.Type]reflect.Value // 类型映射表（类型信息是唯一确定的，所以作为kv）
	parent Injector                       // 父注入器（查找链）
}

func New() Injector {
	return &injector{
		values: make(map[reflect.Type]reflect.Value),
		//interfaces: make(map[reflect.Type]reflect.Value),
	}
}

// Go反射无法直接从接口名获取Type，必须通过“指向接口的指针”间接获取
// InterfaceOf 从一个指向接口的指针中提取接口的 reflect.Type。
// 用法：InterfaceOf((*MyInterface)(nil)) → 返回 MyInterface 的 Type
func InterfaceOf(value interface{}) reflect.Type {
	t := reflect.TypeOf(value)
	//一直解引用指针，直到找到接口类型
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Interface {
		panic("inject.InterfaceOf: 必须传入指向接口的指针，如 (*MyInterface)(nil)")
	}
	return t
}
func (inj *injector) Map(values ...interface{}) TypeMapper {
	// 以值的「实际类型」为 key
	// 例：Map(myLogger) → key 是 *log.Logger
	for _, value := range values {
		inj.values[reflect.TypeOf(value)] = reflect.ValueOf(value)
	}
	return inj
}
func (inj *injector) MapTo(val, ifacePtr interface{}) TypeMapper {
	// 以「接口类型」为 key
	// 例：MapTo(myCtx, (*Context)(nil)) → key 是 Context 接口
	inj.values[InterfaceOf(ifacePtr)] = reflect.ValueOf(val)
	return inj
}
func (inj *injector) Set(typ reflect.Type, val reflect.Value) TypeMapper {
	inj.values[typ] = val
	return inj
}

// Value 是整个注入系统的核心查找函数：三层策略
func (inj *injector) Value(t reflect.Type) reflect.Value {
	val := inj.values[t]
	if val.IsValid() {
		return val
	}
	/*
		这里采用O(N)遍历的方式，是考虑到MapTo()仅在初始化中间件注册时调用一次，不在请求热路径上
		初始化阶段多一次 O(n) 遍历 100 个 service 根本无感。
	*/
	if t.Kind() == reflect.Interface {
		for k, v := range inj.values {
			if k.Implements(t) {
				val = v
				break
			}
		}
	}
	if !val.IsValid() && inj.parent != nil {
		val = inj.parent.Value(t)
	}
	return val
}
func (inj *injector) SetParent(parent Injector) {
	inj.parent = parent
}

// callInvoke 通过反射调用函数 f，从注入器中查找参数值
func (inj *injector) callInvoker(f interface{}, t reflect.Type, numIn int) ([]reflect.Value, error) {
	var in []reflect.Value
	if numIn > 0 {
		in = make([]reflect.Value, numIn)
		for i := 0; i < numIn; i++ {
			argType := t.In(i)
			argVal := inj.Value(argType)
			if !argVal.IsValid() {
				return nil, fmt.Errorf("未找到类型 %v 的注入值", argType)
			}
			in[i] = argVal
		}
	}
	return reflect.ValueOf(f).Call(in), nil
}

// 实现 fastInvoke（高性能调用）
// fastInvoke 通过 FastInvoker 接口调用，跳过反射
func (inj *injector) fastInvoke(f FastInvoker, t reflect.Type, numIn int) ([]reflect.Value, error) {
	var in []interface{}
	if numIn > 0 {
		in = make([]interface{}, numIn)
		for i := 0; i < numIn; i++ {
			argType := t.In(i)
			argVal := inj.Value(argType)
			if !argVal.IsValid() {
				return nil, fmt.Errorf("未找到类型 %v 的注入值", argType)
			}
			in[i] = argVal.Interface()
		}
	}
	return f.Invoke(in)
}

// Invoke 自动调用函数 f，从注入器中查找所有参数值。
// 如果 f 实现了 FastInvoker 则走快速路径，否则走反射路径。
func (inj *injector) Invoke(f interface{}) ([]reflect.Value, error) {
	t := reflect.TypeOf(f)
	//分流：FastInvoker走快速路径
	switch v := f.(type) {
	case FastInvoker:
		return inj.fastInvoke(v, t, t.NumIn())
	default:
		return inj.callInvoker(f, t, t.NumIn())
	}
}

// Apply 遍历结构体的所有字段，将带有 `inject` tag 的字段自动注入值
func (inj *injector) Apply(val interface{}) error {
	//获取val的reflect.Value
	v := reflect.ValueOf(val)
	//在 Go 反射中，如果你传入的是一个指针（如 *User），你是无法直接遍历它的字段的
	//一直解引用直到结构体
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	//非结构体忽略
	if v.Kind() != reflect.Struct {
		return nil
	}
	//遍历字段
	k := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		structField := k.Field(i)
		_, ok := structField.Tag.Lookup("inject")
		if !ok || !f.CanSet() {
			continue
		}
		ft := f.Type()
		injVal := inj.Value(ft)
		if !injVal.IsValid() {
			return fmt.Errorf("字段 %s 需要类型 %v，但注入器中没有注册", structField.Name, ft)
		}
		f.Set(injVal)
		//f.Set() 底层会执行类似于 struct.Field = value 的操作，将容器中的实例塞进结构体的字段里。
	}
	return nil
}
