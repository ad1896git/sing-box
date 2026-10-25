package box

import (
	"bytes"
	"io"
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

func TestSetLogWritterMissingWriter(t *testing.T) {
	factory := &struct{ log.Factory }{Factory: log.NewNOPFactory()}
	instance := &Box{logFactory: factory}
	require.NotPanics(t, func() { instance.SetLogWritter(io.Discard) })
	require.Same(t, factory, instance.logFactory)
}
