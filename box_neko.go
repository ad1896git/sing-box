package box

import (
	"io"
	"reflect"
	"unsafe"
)

// SetLogWritter used in nekoray, change log writer without providing platform interface
//
// Limitations: it does nothing when logging is disabled (the NOP factory has
// no writer), when w is nil, or for a factory without a writer field. The
// write is not synchronized with loggers already running, so call it before
// the box produces log output from other goroutines.
func (s *Box) SetLogWritter(w io.Writer) {
	if w == nil {
		return
	}
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
