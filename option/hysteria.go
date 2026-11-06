package option

import (
	"strconv"
	"strings"

	E "github.com/sagernet/sing/common/exceptions"
)

type HysteriaInboundOptions struct {
	ListenOptions
	Up                  string         `json:"up,omitempty"`
	UpMbps              int            `json:"up_mbps,omitempty"`
	Down                string         `json:"down,omitempty"`
	DownMbps            int            `json:"down_mbps,omitempty"`
	Obfs                string         `json:"obfs,omitempty"`
	Users               []HysteriaUser `json:"users,omitempty"`
	ReceiveWindowConn   uint64         `json:"recv_window_conn,omitempty"`
	ReceiveWindowClient uint64         `json:"recv_window_client,omitempty"`
	MaxConnClient       int            `json:"max_conn_client,omitempty"`
	DisableMTUDiscovery bool           `json:"disable_mtu_discovery,omitempty"`
	InboundTLSOptionsContainer
}

type HysteriaUser struct {
	Name       string `json:"name,omitempty"`
	Auth       []byte `json:"auth,omitempty"`
	AuthString string `json:"auth_str,omitempty"`
}

type HysteriaOutboundOptions struct {
	DialerOptions
	ServerOptions
	Up                  string      `json:"up,omitempty"`
	UpMbps              int         `json:"up_mbps,omitempty"`
	Down                string      `json:"down,omitempty"`
	DownMbps            int         `json:"down_mbps,omitempty"`
	HopPorts            string      `json:"hop_ports,omitempty"`
	HopInterval         int         `json:"hop_interval,omitempty"`
	Obfs                string      `json:"obfs,omitempty"`
	Auth                []byte      `json:"auth,omitempty"`
	AuthString          string      `json:"auth_str,omitempty"`
	ReceiveWindowConn   uint64      `json:"recv_window_conn,omitempty"`
	ReceiveWindow       uint64      `json:"recv_window,omitempty"`
	DisableMTUDiscovery bool        `json:"disable_mtu_discovery,omitempty"`
	Network             NetworkList `json:"network,omitempty"`
	OutboundTLSOptionsContainer
}

// ValidateHopPorts checks a hop_ports value as sing-quic udphop parses it:
// comma-separated ports or start-end ranges, every port in 1-65535.
func ValidateHopPorts(hopPorts string) error {
	for _, portRange := range strings.Split(hopPorts, ",") {
		bounds := strings.Split(portRange, "-")
		if len(bounds) > 2 {
			return E.New("invalid hop_ports range: ", portRange)
		}
		for _, bound := range bounds {
			port, err := strconv.ParseUint(bound, 10, 16)
			if err != nil || port == 0 {
				return E.New("invalid hop_ports ", strconv.Quote(portRange), ": ports must be in 1-65535")
			}
		}
	}
	return nil
}
