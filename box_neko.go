package box

import (
	"io"
	"reflect"
	"unsafe"
)

// SetLogWritter used in nekoray, change log writer without providing platform interface
func (s *Box) SetLogWritter(w io.Writer) {
	factory := reflect.Indirect(reflect.ValueOf(s.logFactory))
	if !factory.IsValid() || factory.Kind() != reflect.Struct {
		return
	}
	writer_ := factory.FieldByName("writer")
	if !writer_.IsValid() {
		return
	}
	writer_ = reflect.NewAt(writer_.Type(), unsafe.Pointer(writer_.UnsafeAddr())).Elem()
	writer_.Set(reflect.ValueOf(w))
}
