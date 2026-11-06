package option

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateHopPorts(t *testing.T) {
	for _, hopPorts := range []string{"1", "443", "65535", "443,8443", "1000-1002", "1002-1000", "1-65535"} {
		require.NoError(t, ValidateHopPorts(hopPorts), hopPorts)
	}
	for _, hopPorts := range []string{"", "0", "0-10", "10-0", "65536", "1-65536", "443,", ",443", "a", "1-2-3", "-1", "1-", " 443"} {
		require.Error(t, ValidateHopPorts(hopPorts), hopPorts)
	}
}
