//go:build with_quic

package outbound

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-dns"
	M "github.com/sagernet/sing/common/metadata"

	"github.com/stretchr/testify/require"
)

const hopTestServer = "hop.example"

func newTestHysteria(router adapter.Router, server string, port uint16, hopPorts string, detour string) (adapter.Outbound, error) {
	return NewHysteria(context.Background(), router, log.NewNOPFactory().NewLogger("hysteria"), "hysteria-out", option.HysteriaOutboundOptions{
		DialerOptions:               option.DialerOptions{Detour: detour},
		ServerOptions:               option.ServerOptions{Server: server, ServerPort: port},
		UpMbps:                      10,
		DownMbps:                    10,
		HopPorts:                    hopPorts,
		OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{TLS: &option.OutboundTLSOptions{Enabled: true, Insecure: true}},
	})
}

func newTestHysteria2(router adapter.Router, server string, port uint16, hopPorts string, detour string) (adapter.Outbound, error) {
	return NewHysteria2(context.Background(), router, log.NewNOPFactory().NewLogger("hysteria2"), "hysteria2-out", option.Hysteria2OutboundOptions{
		DialerOptions:               option.DialerOptions{Detour: detour},
		ServerOptions:               option.ServerOptions{Server: server, ServerPort: port},
		HopPorts:                    hopPorts,
		OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{TLS: &option.OutboundTLSOptions{Enabled: true, Insecure: true}},
	})
}

var hysteriaTestConstructors = map[string]func(adapter.Router, string, uint16, string, string) (adapter.Outbound, error){
	"hysteria":  newTestHysteria,
	"hysteria2": newTestHysteria2,
}

func TestHysteriaHopPortsValidated(t *testing.T) {
	for name, newOutbound := range hysteriaTestConstructors {
		t.Run(name, func(t *testing.T) {
			for _, hopPorts := range []string{"0", "10-0", "65536", "443,"} {
				_, err := newOutbound(nil, "127.0.0.1", 443, hopPorts, "")
				require.ErrorContains(t, err, "hop_ports", hopPorts)
			}
			for _, hopPorts := range []string{"", "443", "443,1000-1002"} {
				outbound, err := newOutbound(nil, "127.0.0.1", 443, hopPorts, "")
				require.NoError(t, err, hopPorts)
				require.NoError(t, outbound.(interface{ Close() error }).Close())
			}
		})
	}
}

// hopTestRouter resolves hopTestServer with its own DNS and serves the detour.
type hopTestRouter struct {
	adapter.Router
	resolved netip.Addr
	detour   *hopTestDetour
}

func (r *hopTestRouter) Outbound(tag string) (adapter.Outbound, bool) {
	return r.detour, tag == r.detour.tag
}

func (r *hopTestRouter) Lookup(ctx context.Context, domain string, strategy dns.DomainStrategy) ([]netip.Addr, error) {
	return r.LookupDefault(ctx, domain)
}

func (r *hopTestRouter) LookupDefault(ctx context.Context, domain string) ([]netip.Addr, error) {
	if domain != hopTestServer {
		return nil, dns.RCodeNameError
	}
	return []netip.Addr{r.resolved}, nil
}

// hopTestDetour records the destinations it is asked to reach.
type hopTestDetour struct {
	adapter.Outbound
	tag    string
	access sync.Mutex
	dialed []M.Socksaddr
}

func (d *hopTestDetour) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	d.access.Lock()
	d.dialed = append(d.dialed, destination)
	d.access.Unlock()
	return (&net.Dialer{}).DialContext(ctx, network, destination.String())
}

func (d *hopTestDetour) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	return net.ListenPacket("udp", "127.0.0.1:0")
}

func (d *hopTestDetour) dialedDestinations() []M.Socksaddr {
	d.access.Lock()
	defer d.access.Unlock()
	return append([]M.Socksaddr(nil), d.dialed...)
}

// With port hopping the server must be an IP before the hop starts, so a
// detour must not receive the bare name: the router resolves it first.
func TestHysteriaHopDetourResolvesServer(t *testing.T) {
	for name, newOutbound := range hysteriaTestConstructors {
		t.Run(name, func(t *testing.T) {
			// A silent UDP peer: the handshake times out, which is enough here.
			server, err := net.ListenPacket("udp", "127.0.0.1:0")
			require.NoError(t, err)
			defer server.Close()
			port := uint16(server.LocalAddr().(*net.UDPAddr).Port)
			router := &hopTestRouter{
				resolved: netip.MustParseAddr("127.0.0.1"),
				detour:   &hopTestDetour{tag: "front"},
			}
			outbound, err := newOutbound(router, hopTestServer, port, strconv.Itoa(int(port)), "front")
			require.NoError(t, err)
			defer outbound.(interface{ Close() error }).Close()
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			conn, err := outbound.DialContext(ctx, "tcp", M.ParseSocksaddrHostPort("example.com", 80))
			if err == nil {
				conn.Close()
			}
			dialed := router.detour.dialedDestinations()
			require.NotEmpty(t, dialed)
			require.Equal(t, M.SocksaddrFrom(router.resolved, port), dialed[0])
		})
	}
}
