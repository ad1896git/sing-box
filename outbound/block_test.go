package outbound

import (
	"context"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	"github.com/stretchr/testify/require"
)

// noDeadlinePacketConn behaves like the sing udpnat conn: deadlines are not supported.
type noDeadlinePacketConn struct {
	packets   chan []byte
	closed    chan struct{}
	closeOnce sync.Once
	reads     int
	readErr   error
}

var _ N.PacketConn = (*noDeadlinePacketConn)(nil)

func newNoDeadlinePacketConn() *noDeadlinePacketConn {
	return &noDeadlinePacketConn{
		packets: make(chan []byte, 8),
		closed:  make(chan struct{}),
	}
}

func (c *noDeadlinePacketConn) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	select {
	case packet := <-c.packets:
		if buffer.FreeLen() < len(packet) {
			c.readErr = io.ErrShortBuffer
			return M.Socksaddr{}, io.ErrShortBuffer
		}
		c.reads++
		copy(buffer.Extend(len(packet)), packet)
		return M.ParseSocksaddrHostPort("127.0.0.1", 53), nil
	case <-c.closed:
		return M.Socksaddr{}, net.ErrClosed
	}
}

func (c *noDeadlinePacketConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	buffer.Release()
	return nil
}

func (c *noDeadlinePacketConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *noDeadlinePacketConn) isClosed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

func (c *noDeadlinePacketConn) LocalAddr() net.Addr                { return &net.UDPAddr{} }
func (c *noDeadlinePacketConn) SetDeadline(t time.Time) error      { return os.ErrInvalid }
func (c *noDeadlinePacketConn) SetReadDeadline(t time.Time) error  { return os.ErrInvalid }
func (c *noDeadlinePacketConn) SetWriteDeadline(t time.Time) error { return os.ErrInvalid }

func setBlockIdleTimeout(t *testing.T, timeout time.Duration) {
	oldTimeout := blockIdleTimeout
	blockIdleTimeout = timeout
	t.Cleanup(func() { blockIdleTimeout = oldTimeout })
}

func runBlockPacketConnection(ctx context.Context, conn N.PacketConn) <-chan error {
	block := NewBlock(log.NewNOPFactory().NewLogger("block"), "block")
	done := make(chan error, 1)
	go func() {
		done <- block.NewPacketConnection(ctx, conn, adapter.InboundContext{})
	}()
	return done
}

func waitBlockReturn(t *testing.T, done <-chan error, within time.Duration) {
	t.Helper()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(within):
		t.Fatal("blackhole packet connection did not return")
	}
}

func TestBlockPacketIdleTimeoutWithoutDeadline(t *testing.T) {
	setBlockIdleTimeout(t, 100*time.Millisecond)
	conn := newNoDeadlinePacketConn()
	done := runBlockPacketConnection(context.Background(), conn)
	waitBlockReturn(t, done, 5*time.Second)
	require.True(t, conn.isClosed())
}

func TestBlockPacketIdleTimeoutWithDeadline(t *testing.T) {
	setBlockIdleTimeout(t, 100*time.Millisecond)
	udpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer udpConn.Close()
	done := runBlockPacketConnection(context.Background(), bufio.NewPacketConn(udpConn))
	waitBlockReturn(t, done, 5*time.Second)
	_, err = udpConn.WriteTo([]byte{0}, udpConn.LocalAddr())
	require.ErrorIs(t, err, net.ErrClosed)
}

func TestBlockPacketContextCancel(t *testing.T) {
	setBlockIdleTimeout(t, time.Hour)
	conn := newNoDeadlinePacketConn()
	ctx, cancel := context.WithCancel(context.Background())
	done := runBlockPacketConnection(ctx, conn)
	cancel()
	waitBlockReturn(t, done, 5*time.Second)
	require.True(t, conn.isClosed())
}

func TestBlockPacketBufferResetBetweenReads(t *testing.T) {
	setBlockIdleTimeout(t, 200*time.Millisecond)
	conn := newNoDeadlinePacketConn()
	for i := 0; i < 3; i++ {
		conn.packets <- make([]byte, 40000)
	}
	done := runBlockPacketConnection(context.Background(), conn)
	waitBlockReturn(t, done, 5*time.Second)
	require.NoError(t, conn.readErr)
	require.Equal(t, 3, conn.reads)
}
