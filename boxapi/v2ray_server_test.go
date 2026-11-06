package boxapi

import (
	"context"
	"net"
	"testing"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/option"

	"github.com/stretchr/testify/require"
)

func TestSbV2rayServerStatsDisabled(t *testing.T) {
	server := NewSbV2rayServer(option.V2RayStatsServiceOptions{Enabled: false})
	require.True(t, server.StatsService() == nil, "disabled stats must be a nil interface")
	require.NotPanics(t, func() {
		require.Zero(t, server.QueryStats("outbound>>>proxy>>>traffic>>>uplink"))
	})
}

func TestSbV2rayServerStatsEnabled(t *testing.T) {
	server := NewSbV2rayServer(option.V2RayStatsServiceOptions{Enabled: true, Outbounds: []string{"proxy"}})
	require.NotNil(t, server.StatsService())
	require.Zero(t, server.QueryStats("outbound>>>proxy>>>traffic>>>uplink"))
}

func TestDialContextStatsDisabled(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			conn.Close()
		}
	}()

	instance, err := box.New(box.Options{Context: context.Background()})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, instance.Close()) })
	instance.Router().SetV2RayServer(NewSbV2rayServer(option.V2RayStatsServiceOptions{Enabled: false}))

	conn, err := DialContext(context.Background(), instance, "tcp", listener.Addr().String())
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}
