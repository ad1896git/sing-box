package box

import (
	"io"

	"github.com/sagernet/sing-box/log"
)

// SetLogWritter used in nekoray, change log writer without providing platform interface
//
// It is safe to call while loggers are running. Nil (including typed-nil)
// writers, disabled logging, and unsupported factories are ignored.
func (s *Box) SetLogWritter(w io.Writer) {
	log.SetWriter(s.logFactory, w)
}
