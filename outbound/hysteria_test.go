//go:build with_quic

package outbound

import (
	"context"
	"errors"
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
	"github.com/sagernet/sing-vmess/vless"
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

// These fakes exercise DNS policy and both address families without requiring
// IPv6 on the host. Every dial fails so serial fallback visits every result.
type hopPolicyRouter struct {
	adapter.Router
	addresses      []netip.Addr
	strategy       dns.DomainStrategy
	lookups        int
	defaultLookups int
	lookupErr      error
	detour         *hopPolicyDetour
}

func (r *hopPolicyRouter) Outbound(tag string) (adapter.Outbound, bool) {
	return r.detour, tag == "front"
}

func (r *hopPolicyRouter) Lookup(ctx context.Context, domain string, strategy dns.DomainStrategy) ([]netip.Addr, error) {
	r.lookups++
	r.strategy = strategy
	return r.addresses, r.lookupErr
}

func (r *hopPolicyRouter) LookupDefault(ctx context.Context, domain string) ([]netip.Addr, error) {
	r.defaultLookups++
	return r.addresses, r.lookupErr
}

type hopPolicyDetour struct {
	adapter.Outbound
	destinations []M.Socksaddr
	conn         net.Conn
}

func (d *hopPolicyDetour) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	d.destinations = append(d.destinations, destination)
	if d.conn != nil {
		return d.conn, nil
	}
	return nil, errors.New("test detour dial failed")
}

func TestHysteriaHopDetourDNSPolicy(t *testing.T) {
	ipv4 := netip.MustParseAddr("192.0.2.1")
	ipv6 := netip.MustParseAddr("2001:db8::1")
	for _, strategy := range []dns.DomainStrategy{
		dns.DomainStrategyAsIS, dns.DomainStrategyPreferIPv4, dns.DomainStrategyPreferIPv6,
		dns.DomainStrategyUseIPv4, dns.DomainStrategyUseIPv6,
	} {
		t.Run(strconv.Itoa(int(strategy)), func(t *testing.T) {
			addresses := []netip.Addr{ipv4, ipv6}
			switch strategy {
			case dns.DomainStrategyPreferIPv6:
				addresses = []netip.Addr{ipv6, ipv4}
			case dns.DomainStrategyUseIPv4:
				addresses = []netip.Addr{ipv4}
			case dns.DomainStrategyUseIPv6:
				addresses = []netip.Addr{ipv6}
			}
			router := &hopPolicyRouter{addresses: addresses, detour: &hopPolicyDetour{}}
			d, err := newHysteriaDialer(router, option.DialerOptions{Detour: "front", DomainStrategy: option.DomainStrategy(strategy)}, "443")
			require.NoError(t, err)
			_, err = d.DialContext(context.Background(), "udp", M.ParseSocksaddrHostPort(hopTestServer, 443))
			require.Error(t, err)
			if strategy == dns.DomainStrategyAsIS {
				require.Equal(t, 1, router.defaultLookups)
				require.Zero(t, router.lookups)
			} else {
				require.Equal(t, 1, router.lookups)
				require.Zero(t, router.defaultLookups)
				require.Equal(t, strategy, router.strategy)
			}
			var expected []M.Socksaddr
			for _, address := range addresses {
				expected = append(expected, M.SocksaddrFrom(address, 443))
			}
			require.Equal(t, expected, router.detour.destinations)
		})
	}
}

func TestHysteriaHopDetourDNSFailure(t *testing.T) {
	router := &hopPolicyRouter{lookupErr: dns.RCodeNameError, detour: &hopPolicyDetour{}}
	d, err := newHysteriaDialer(router, option.DialerOptions{Detour: "front"}, "443")
	require.NoError(t, err)
	_, err = d.DialContext(context.Background(), "udp", M.ParseSocksaddrHostPort(hopTestServer, 443))
	require.ErrorIs(t, err, dns.RCodeNameError)
	require.Empty(t, router.detour.destinations)
}

func TestHysteriaDetourResolutionScope(t *testing.T) {
	for _, test := range []struct{ name, server, hopPorts string }{
		{"no-hop", hopTestServer, ""},
		{"literal-ipv4", "192.0.2.1", "443"},
		{"literal-ipv6", "2001:db8::1", "443"},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := &hopPolicyRouter{detour: &hopPolicyDetour{}}
			d, err := newHysteriaDialer(router, option.DialerOptions{Detour: "front"}, test.hopPorts)
			require.NoError(t, err)
			destination := M.ParseSocksaddrHostPort(test.server, 443)
			_, err = d.DialContext(context.Background(), "udp", destination)
			require.Error(t, err)
			require.Equal(t, []M.Socksaddr{destination}, router.detour.destinations)
			require.Zero(t, router.lookups+router.defaultLookups)
		})
	}
}

// VLESS PacketConn embeds the proxy transport: RemoteAddr describes the
// proxy, not the logical UDP destination. Hopping must use the latter.
func TestHysteriaHopDetourRemoteAddress(t *testing.T) {
	for _, address := range []string{"192.0.2.1", "2001:db8::1"} {
		t.Run(address, func(t *testing.T) {
			client, err := vless.NewClient("b831381d-6324-4d53-ad4f-8cda48b30811", "", log.NewNOPFactory().Logger())
			require.NoError(t, err)
			transport, peer := net.Pipe()
			defer transport.Close()
			defer peer.Close()
			resolved := netip.MustParseAddr(address)
			destination := M.SocksaddrFrom(resolved, 443)
			packetConn, err := client.DialEarlyPacketConn(transport, destination)
			require.NoError(t, err)
			router := &hopPolicyRouter{addresses: []netip.Addr{resolved}, detour: &hopPolicyDetour{conn: packetConn}}
			d, err := newHysteriaDialer(router, option.DialerOptions{Detour: "front"}, "443")
			require.NoError(t, err)
			conn, err := d.DialContext(context.Background(), "udp", M.ParseSocksaddrHostPort(hopTestServer, 443))
			require.NoError(t, err)
			defer conn.Close()
			require.Equal(t, destination, M.SocksaddrFromNet(conn.RemoteAddr()))
			require.Equal(t, []M.Socksaddr{destination}, router.detour.destinations)
		})
	}
}
