// Experimental shim: restores the sing v0.4.3 rw helpers removed in sing v0.5.0.
package rwshim

import (
	"io"
	"sync/atomic"

	"github.com/sagernet/sing/common/varbin"
)

type ReadCounter struct {
	io.Reader
	count int64
}

func (r *ReadCounter) Read(p []byte) (n int, err error) {
	n, err = r.Reader.Read(p)
	if n > 0 {
		atomic.AddInt64(&r.count, int64(n))
	}
	return
}

func (r *ReadCounter) Count() int64 {
	return r.count
}

func (r *ReadCounter) Reset() {
	atomic.StoreInt64(&r.count, 0)
}

type WriteCloser interface {
	CloseWrite() error
}

// byteReader adds ReadByte without buffering, so no bytes past the value are consumed.
type byteReader struct {
	io.Reader
	buf [1]byte
}

func (r *byteReader) ReadByte() (byte, error) {
	_, err := io.ReadFull(r.Reader, r.buf[:])
	return r.buf[0], err
}

func NewByteReader(reader io.Reader) varbin.Reader {
	return &byteReader{Reader: reader}
}

type byteWriter struct {
	io.Writer
}

func (w *byteWriter) WriteByte(c byte) error {
	_, err := w.Writer.Write([]byte{c})
	return err
}

func NewByteWriter(writer io.Writer) varbin.Writer {
	return &byteWriter{Writer: writer}
}
