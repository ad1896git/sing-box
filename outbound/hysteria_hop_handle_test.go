//go:build with_quic

package outbound

import (
	"context"
	"net"
	"os"
	"strconv"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-vmess/vless"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	"github.com/stretchr/testify/require"
)

// A transport with observable operations; accidental I/O fails without blocking.
type hopHandleTransport struct {
	ioCalls atomic.Int32
	closes  atomic.Int32
}

func (c *hopHandleTransport) Read([]byte) (int, error) {
	c.ioCalls.Add(1)
	return 0, net.ErrClosed
}

func (c *hopHandleTransport) Write([]byte) (int, error) {
	c.ioCalls.Add(1)
	return 0, net.ErrClosed
}

func (c *hopHandleTransport) Close() error {
	c.closes.Add(1)
	return nil
}

func (c *hopHandleTransport) LocalAddr() net.Addr {
	return M.ParseSocksaddr("127.0.0.1:12345").UDPAddr()
}

func (c *hopHandleTransport) RemoteAddr() net.Addr {
	return M.ParseSocksaddr("198.51.100.1:23456").UDPAddr()
}

func (c *hopHandleTransport) SetDeadline(time.Time) error {
	c.ioCalls.Add(1)
	return net.ErrClosed
}

func (c *hopHandleTransport) SetReadDeadline(t time.Time) error  { return c.SetDeadline(t) }
func (c *hopHandleTransport) SetWriteDeadline(t time.Time) error { return c.SetDeadline(t) }

// These capabilities belong to the transport, not to the private hop handle.
type hopHandleRichConn struct{ *hopHandleTransport }

func (c *hopHandleRichConn) ReadFrom([]byte) (int, net.Addr, error) {
	panic("unexpected packet read on raw hop transport")
}

func (c *hopHandleRichConn) WriteTo([]byte, net.Addr) (int, error) {
	panic("unexpected packet write on raw hop transport")
}

func (c *hopHandleRichConn) ReadPacket(*buf.Buffer) (M.Socksaddr, error) {
	panic("unexpected packet read on raw hop transport")
}

func (c *hopHandleRichConn) WritePacket(*buf.Buffer, M.Socksaddr) error {
	panic("unexpected packet write on raw hop transport")
}

func (c *hopHandleRichConn) InitializeReadWaiter(N.ReadWaitOptions) bool { return false }
func (c *hopHandleRichConn) WaitReadBuffer() (*buf.Buffer, error) {
	panic("unexpected read waiter on raw hop transport")
}

func (c *hopHandleRichConn) Upstream() any                         { return c.hopHandleTransport }
func (c *hopHandleRichConn) FrontHeadroom() int                    { return 16 }
func (c *hopHandleRichConn) SyscallConn() (syscall.RawConn, error) { return nil, os.ErrInvalid }
func (c *hopHandleRichConn) SetReadBuffer(int) error               { return nil }
func (c *hopHandleRichConn) SetWriteBuffer(int) error              { return nil }

var (
	_ net.Conn            = (*hopHandleRichConn)(nil)
	_ net.PacketConn      = (*hopHandleRichConn)(nil)
	_ N.PacketConn        = (*hopHandleRichConn)(nil)
	_ N.ReadWaiter        = (*hopHandleRichConn)(nil)
	_ common.WithUpstream = (*hopHandleRichConn)(nil)
	_ syscall.Conn        = (*hopHandleRichConn)(nil)
)

func newHopHandleVLESS(t *testing.T, transport net.Conn) net.Conn {
	t.Helper()
	client, err := vless.NewClient("b831381d-6324-4d53-ad4f-8cda48b30811", "", log.NewNOPFactory().Logger())
	require.NoError(t, err)
	conn, err := client.DialEarlyPacketConn(transport, M.ParseSocksaddr("192.0.2.1:443"))
	require.NoError(t, err)
	// VLESS v0.1.12 has WritePacket but no ReadPacket: it is not N.PacketConn.
	require.Implements(t, (*net.PacketConn)(nil), conn)
	require.NotImplements(t, (*N.PacketConn)(nil), conn)
	require.NotImplements(t, (*N.ReadWaiter)(nil), conn)
	return conn
}

func TestHysteriaHopHandleScope(t *testing.T) {
	for _, test := range []struct {
		name, network, hopPorts string
		wrapped                 bool
	}{
		{"tcp-hop", "tcp", "443", false},
		{"udp-no-hop", "udp", "", false},
		{"udp-hop", "udp", "443", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport := &hopHandleTransport{}
			rich := &hopHandleRichConn{transport}
			router := &hopHandleRouter{detour: &hopHandleDetour{conn: rich, packet: rich}}
			d, err := newHysteriaDialer(router, option.DialerOptions{Detour: "front"}, test.hopPorts)
			require.NoError(t, err)
			destination := M.ParseSocksaddr("192.0.2.1:443")
			conn, err := d.DialContext(context.Background(), test.network, destination)
			require.NoError(t, err)
			defer conn.Close()
			if test.wrapped {
				require.Equal(t, destination, M.SocksaddrFromNet(conn.RemoteAddr()))
				require.Equal(t, rich.LocalAddr(), conn.LocalAddr())
				checkHopHandle(t, conn)
				require.Zero(t, transport.ioCalls.Load())
			} else {
				// Identity preserves all optional interfaces, including future ones.
				require.Same(t, rich, conn)
			}
			packet, err := d.ListenPacket(context.Background(), destination)
			require.NoError(t, err)
			require.Same(t, rich, packet)
		})
	}
}

func checkHopHandle(t *testing.T, conn net.Conn) {
	t.Helper()
	require.NotImplements(t, (*net.PacketConn)(nil), conn)
	require.NotImplements(t, (*N.PacketConn)(nil), conn)
	require.NotImplements(t, (*N.ReadWaiter)(nil), conn)
	require.NotImplements(t, (*common.WithUpstream)(nil), conn)
	require.NotImplements(t, (*syscall.Conn)(nil), conn)
	require.NotImplements(t, (*interface{ FrontHeadroom() int })(nil), conn)
	require.NotImplements(t, (*interface{ SetReadBuffer(int) error })(nil), conn)
	require.NotImplements(t, (*interface{ SetWriteBuffer(int) error })(nil), conn)
	n, err := conn.Read(make([]byte, 1))
	require.Zero(t, n)
	require.ErrorIs(t, err, os.ErrInvalid)
	n, err = conn.Write([]byte{1})
	require.Zero(t, n)
	require.ErrorIs(t, err, os.ErrInvalid)
	for _, setDeadline := range []func(time.Time) error{conn.SetDeadline, conn.SetReadDeadline, conn.SetWriteDeadline} {
		require.ErrorIs(t, setDeadline(time.Now()), os.ErrInvalid)
	}
}

func TestHysteriaHopHandleVLESS(t *testing.T) {
	transport := &hopHandleTransport{}
	vlessConn := newHopHandleVLESS(t, transport)
	router := &hopHandleRouter{detour: &hopHandleDetour{conn: vlessConn}}
	d, err := newHysteriaDialer(router, option.DialerOptions{Detour: "front"}, "443")
	require.NoError(t, err)
	conn, err := d.DialContext(context.Background(), "udp", M.ParseSocksaddr("192.0.2.1:443"))
	require.NoError(t, err)
	defer func() {
		if transport.closes.Load() == 0 {
			conn.Close()
		}
	}()
	checkHopHandle(t, conn)
	require.Zero(t, transport.ioCalls.Load())
	require.NoError(t, conn.Close())
	require.EqualValues(t, 1, transport.closes.Load())
}

type hopHandleDetour struct {
	adapter.Outbound
	conn    net.Conn
	packet  net.PacketConn
	listens atomic.Int32
}

func (d *hopHandleDetour) DialContext(context.Context, string, M.Socksaddr) (net.Conn, error) {
	return d.conn, nil
}

func (d *hopHandleDetour) ListenPacket(context.Context, M.Socksaddr) (net.PacketConn, error) {
	d.listens.Add(1)
	return d.packet, nil
}

type hopHandleRouter struct {
	adapter.Router
	detour *hopHandleDetour
}

func (r *hopHandleRouter) Outbound(tag string) (adapter.Outbound, bool) {
	return r.detour, tag == "front"
}

// Real clients must send QUIC packets through ListenPacket and close the raw
// transport on handshake failure, without performing data I/O on the handle.
func TestHysteriaHopHandleClients(t *testing.T) {
	for name, newOutbound := range hysteriaTestConstructors {
		for _, kind := range []string{"rich", "vless"} {
			t.Run(name+"/"+kind, func(t *testing.T) {
				server, err := net.ListenPacket("udp", "127.0.0.1:0")
				require.NoError(t, err)
				defer server.Close()
				packet, err := net.ListenPacket("udp", "127.0.0.1:0")
				require.NoError(t, err)
				defer packet.Close()
				transport := &hopHandleTransport{}
				var raw net.Conn = &hopHandleRichConn{transport}
				if kind == "vless" {
					raw = newHopHandleVLESS(t, transport)
				}
				detour := &hopHandleDetour{conn: raw, packet: packet}
				port := uint16(server.LocalAddr().(*net.UDPAddr).Port)
				outbound, err := newOutbound(&hopHandleRouter{detour: detour}, "127.0.0.1", port, strconv.Itoa(int(port)), "front")
				require.NoError(t, err)
				defer outbound.(interface{ Close() error }).Close()
				ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
				defer cancel()
				conn, err := outbound.DialContext(ctx, "tcp", M.ParseSocksaddr("192.0.2.1:80"))
				if conn != nil {
					defer conn.Close()
				}
				require.ErrorIs(t, err, context.DeadlineExceeded)
				require.Positive(t, detour.listens.Load())
				require.Zero(t, transport.ioCalls.Load())
				require.EqualValues(t, 1, transport.closes.Load())
				require.NoError(t, server.SetReadDeadline(time.Now().Add(time.Second)))
				n, peer, err := server.ReadFrom(make([]byte, 2048))
				require.NoError(t, err)
				require.Positive(t, n)
				require.Equal(t, packet.LocalAddr(), peer)
				_, err = packet.WriteTo([]byte{1}, server.LocalAddr())
				require.ErrorIs(t, err, net.ErrClosed)
			})
		}
	}
}
