package box

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"

	"github.com/stretchr/testify/require"
)

func TestSetLogWritterDisabled(t *testing.T) {
	instance, err := New(Options{Options: option.Options{
		Log: &option.LogOptions{Disabled: true},
	}})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, instance.Close()) })

	factory := instance.logFactory
	require.NotPanics(t, func() { instance.SetLogWritter(io.Discard) })
	require.Equal(t, factory, instance.logFactory)
	var output bytes.Buffer
	instance.SetLogWritter(&output)
	instance.logFactory.Logger().Info("logging stays disabled")
	require.Empty(t, output.String())
}

func TestSetLogWritterEnabled(t *testing.T) {
	instance, err := New(Options{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, instance.Close()) })

	var original, replacement bytes.Buffer
	instance.SetLogWritter(&original)
	instance.logFactory.Logger().Info("original writer")
	require.Contains(t, original.String(), "original writer")
	original.Reset()

	instance.SetLogWritter(&replacement)
	instance.logFactory.Logger().Info("replacement writer")
	require.Contains(t, replacement.String(), "replacement writer")
	require.Empty(t, original.String())
}

func TestSetLogWritterNilFactory(t *testing.T) {
	instance := &Box{}
	require.NotPanics(t, func() { instance.SetLogWritter(io.Discard) })
	require.Nil(t, instance.logFactory)
}

func TestSetLogWritterNilWriter(t *testing.T) {
	instance, err := New(Options{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, instance.Close()) })

	var output bytes.Buffer
	instance.SetLogWritter(&output)
	require.NotPanics(t, func() { instance.SetLogWritter(nil) })
	instance.logFactory.Logger().Info("writer kept")
	require.Contains(t, output.String(), "writer kept")
}

func TestSetLogWritterMissingWriter(t *testing.T) {
	factory := &struct{ log.Factory }{Factory: log.NewNOPFactory()}
	instance := &Box{logFactory: factory}
	require.NotPanics(t, func() { instance.SetLogWritter(io.Discard) })
	require.Same(t, factory, instance.logFactory)
}

type nilMapWriter map[string]string
type nilSliceWriter []byte
type nilFuncWriter func([]byte) (int, error)
type nilChanWriter chan []byte

func (nilMapWriter) Write([]byte) (int, error)   { panic("nil map writer used") }
func (nilSliceWriter) Write([]byte) (int, error) { panic("nil slice writer used") }
func (nilFuncWriter) Write([]byte) (int, error)  { panic("nil func writer used") }
func (nilChanWriter) Write([]byte) (int, error)  { panic("nil chan writer used") }

func TestSetLogWritterTypedNil(t *testing.T) {
	for _, observable := range []bool{false, true} {
		for _, writer := range []io.Writer{(*bytes.Buffer)(nil), nilMapWriter(nil), nilSliceWriter(nil), nilFuncWriter(nil), nilChanWriter(nil)} {
			t.Run(fmt.Sprintf("observable=%v/%T", observable, writer), func(t *testing.T) {
				var output bytes.Buffer
				factory := log.NewDefaultFactory(context.Background(), log.Formatter{}, &output, "", nil, observable)
				t.Cleanup(func() { require.NoError(t, factory.Close()) })
				instance := &Box{logFactory: factory}
				require.NotPanics(t, func() {
					instance.SetLogWritter(writer)
					factory.Logger().Info("writer kept")
				})
				require.Contains(t, output.String(), "writer kept")
			})
		}
	}
}

type synchronizedLogBuffer struct {
	sync.Mutex
	bytes.Buffer
}

func (w *synchronizedLogBuffer) Write(p []byte) (int, error) {
	w.Lock()
	defer w.Unlock()
	return w.Buffer.Write(p)
}

func TestSetLogWritterConcurrent(t *testing.T) {
	for _, observable := range []bool{false, true} {
		t.Run(fmt.Sprint(observable), func(t *testing.T) {
			var first, second synchronizedLogBuffer
			factory := log.NewDefaultFactory(context.Background(), log.Formatter{}, &first, "", nil, observable)
			t.Cleanup(func() { require.NoError(t, factory.Close()) })
			instance := &Box{logFactory: factory}
			logger := factory.NewLogger("existing logger")
			start := make(chan struct{})
			var workers sync.WaitGroup
			for worker := 0; worker < 4; worker++ {
				workers.Add(1)
				go func(setter bool) {
					defer workers.Done()
					<-start
					for i := 0; i < 1000; i++ {
						if setter {
							instance.SetLogWritter(&first)
							instance.SetLogWritter(&second)
						} else {
							logger.Info("concurrent entry")
						}
					}
				}(worker < 2)
			}
			close(start)
			workers.Wait()
			require.Equal(t, 2000, strings.Count(first.String()+second.String(), "concurrent entry"))
			var final bytes.Buffer
			instance.SetLogWritter(&final)
			logger.Info("final writer")
			require.Contains(t, final.String(), "final writer")
		})
	}
}
