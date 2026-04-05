package inject

import (
	"reflect"
	"testing"

	"github.com/go-playground/assert/v2"
)

type MyService struct {
	Name string
}
type MyInterface interface {
	Hello() string
}

func TestInjector_InterfaceOf(t *testing.T) {
	of := InterfaceOf((*MyInterface)(nil))
	assert.Equal(t, reflect.Interface, of.Kind())
	iType := InterfaceOf((**MyInterface)(nil))
	assert.Equal(t, reflect.Interface, iType.Kind())
}
func TestInjector_Set(t *testing.T) {

}
func TestInjector_GetVal(t *testing.T) {

}
func TestInjector_SetParent(t *testing.T) {

}
