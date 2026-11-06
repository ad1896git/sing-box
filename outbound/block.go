package outbound

import (
	"context"
	"io"
	"net"
	"sync"

	"github.com/sagernet/sing-box/adapter"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/canceler"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

var _ adapter.Outbound = (*Block)(nil)

// blockIdleTimeout is a variable so tests can shorten it.
var blockIdleTimeout = C.QUICTimeout

type Block struct {
	myOutboundAdapter
}

func NewBlock(logger log.ContextLogger, tag string) *Block {
	return &Block{
		myOutboundAdapter{
			protocol: C.TypeBlock,
			network:  []string{N.NetworkTCP, N.NetworkUDP},
			logger:   logger,
			tag:      tag,
		},
	}
}

func (h *Block) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	h.logger.InfoContext(ctx, "blocked connection to ", destination)
	return nil, io.EOF
}

func (h *Block) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	h.logger.InfoContext(ctx, "blocked listen packet connection to ", destination)
	return nil, io.EOF
}

func (h *Block) NewConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext) error {
	conn.Close()
	h.logger.InfoContext(ctx, "blocked connection to ", metadata.Destination)
	return nil
}

func (h *Block) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext) error {
	h.logger.InfoContext(ctx, "blocked new packet connection to ", metadata.Destination)
	// The canceler context ends on idle timeout (with or without read deadline
	// support, e.g. udpnat conns) and on cancel of ctx; close conn to stop the read.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ctx, conn = canceler.NewPacketConn(ctx, conn, blockIdleTimeout)
	var closeOnce sync.Once
	closeConn := func() { closeOnce.Do(func() { conn.Close() }) }
	defer closeConn()
	go func() {
		<-ctx.Done()
		closeConn()
	}()
	buffer := buf.NewSize(65535)
	defer buffer.Release()
	for {
		buffer.Reset()
		_, err := conn.ReadPacket(buffer)
		if err != nil {
			return nil
		}
	}
}
