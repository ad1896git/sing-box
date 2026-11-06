package outbound

import (
	"context"
	"net"
	"testing"

	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-vmess/vless"
	M "github.com/sagernet/sing/common/metadata"

	"github.com/stretchr/testify/require"
)

const testVLESSUUID = "b831381d-6324-4d53-ad4f-8cda48b30811"

func newTestVLESS(flow string, multiplex bool) (*VLESS, error) {
	return NewVLESS(context.Background(), nil, log.NewNOPFactory().NewLogger("vless"), "vless-out", option.VLESSOutboundOptions{
		ServerOptions: option.ServerOptions{Server: "127.0.0.1", ServerPort: 443},
		UUID:          testVLESSUUID,
		Flow:          flow,
		Multiplex:     &option.OutboundMultiplexOptions{Enabled: multiplex},
	})
}

func TestVLESSFlowWithMultiplexRejected(t *testing.T) {
	_, err := newTestVLESS(vless.FlowVision, true)
	require.ErrorContains(t, err, "flow xtls-rprx-vision is not supported with multiplex")
}

func TestVLESSFlowPreserved(t *testing.T) {
	outbound, err := newTestVLESS(vless.FlowVision, false)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, outbound.Close()) })
	require.Nil(t, outbound.multiplexDialer)

	// Vision needs a TLS connection; a plain pipe proves the client kept the flow.
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()
	_, err = outbound.client.DialEarlyConn(clientConn, M.ParseSocksaddrHostPort("example.com", 443))
	require.ErrorContains(t, err, "initialize vision")
}

func TestVLESSMultiplexWithoutFlow(t *testing.T) {
	outbound, err := newTestVLESS("", true)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, outbound.Close()) })
	require.NotNil(t, outbound.multiplexDialer)
}
