package log

import (
	"io"
	"reflect"
)

// SetWriter replaces the default factory's writer, including after Start.
// Nil (including typed-nil) writers and other factories are ignored.
// Each log call takes a snapshot: an in-flight write may finish on the previous
// writer. Writers must support concurrent Write calls; they are not closed here.
func SetWriter(factory Factory, writer io.Writer) {
	f, ok := factory.(*defaultFactory)
	if !ok || f == nil || writer == nil {
		return
	}
	value := reflect.ValueOf(writer)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return
		}
	}
	f.setWriter(writer)
}

func (f *defaultFactory) setWriter(writer io.Writer) {
	f.writerMu.Lock()
	f.writer = writer
	f.writerMu.Unlock()
}

func (f *defaultFactory) logWriter() io.Writer {
	f.writerMu.RLock()
	writer := f.writer
	f.writerMu.RUnlock()
	return writer
}
