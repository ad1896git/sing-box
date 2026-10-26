package rwshim

import (
	"bufio"
	"context"
	"net"

	"github.com/sagernet/sing/common/buf"
	sbufio "github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/protocol/socks"
)

// leftoverHandler passes the bytes that the socks reader buffered past the
// handshake to the next handler, so pipelined payload is not lost.
type leftoverHandler struct {
	socks.Handler
	reader *bufio.Reader
}

// HandlerWithLeftover wraps a socks handler for use with HandleConnection0.
func HandlerWithLeftover(handler socks.Handler, reader *bufio.Reader) socks.Handler {
	return leftoverHandler{Handler: handler, reader: reader}
}

func (h leftoverHandler) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	if n := h.reader.Buffered(); n > 0 {
		leftover, err := h.reader.Peek(n)
		if err != nil {
			return err
		}
		cached := make([]byte, n)
		copy(cached, leftover)
		_, _ = h.reader.Discard(n)
		conn = sbufio.NewCachedConn(conn, buf.As(cached))
	}
	return h.Handler.NewConnection(ctx, conn, metadata)
}

var _ N.TCPConnectionHandler = leftoverHandler{}
