package log

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type heldLogWriter struct {
	entered chan struct{}
	release chan struct{}
}

func (w *heldLogWriter) Write(p []byte) (int, error) {
	close(w.entered)
	<-w.release
	return len(p), nil
}

func TestSetWriterDuringWrite(t *testing.T) {
	previous := &heldLogWriter{make(chan struct{}), make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(previous.release) }) }
	defer release()
	factory := NewDefaultFactory(context.Background(), Formatter{}, previous, "", nil, false)
	t.Cleanup(func() { require.NoError(t, factory.Close()) })
	logger := factory.Logger()
	done := make(chan struct{})
	go func() {
		defer close(done)
		logger.Info("in-flight entry")
	}()
	select {
	case <-previous.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("log did not reach the previous writer")
	}

	var replacement bytes.Buffer
	replaced := make(chan struct{})
	go func() {
		SetWriter(factory, &replacement)
		close(replaced)
	}()
	select {
	case <-replaced:
	case <-time.After(5 * time.Second):
		t.Fatal("writer replacement waited for an in-flight Write")
	}
	logger.Info("replacement entry")
	require.Contains(t, replacement.String(), "replacement entry")
	require.NotContains(t, replacement.String(), "in-flight entry")
	release()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("previous Write did not finish")
	}
}

func TestSetWriterNilFactory(t *testing.T) {
	var factory *defaultFactory
	require.NotPanics(t, func() { SetWriter(factory, io.Discard) })
}
