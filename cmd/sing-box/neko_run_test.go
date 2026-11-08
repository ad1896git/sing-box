package boxmain

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	M "github.com/sagernet/sing/common/metadata"

	"github.com/stretchr/testify/require"
)

// Exercise the production order: Create starts the box before the caller can
// redirect its log writer. A block outbound produces logs without network I/O.
func TestCreateSetLogWritter(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "enabled", true: "NOP"}[disabled], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "startup.log")
			config, err := json.Marshal(map[string]any{
				"log":       map[string]any{"disabled": disabled, "output": path},
				"outbounds": []map[string]any{{"type": "block", "tag": "blocked"}},
			})
			require.NoError(t, err)
			instance, cancel, err := Create(config)
			require.NoError(t, err)
			t.Cleanup(func() {
				cancel()
				require.NoError(t, instance.Close())
			})
			if !disabled {
				startup, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Contains(t, string(startup), "sing-box started")
			}
			outbound, found := instance.Router().Outbound("blocked")
			require.True(t, found)
			var output bytes.Buffer
			instance.SetLogWritter(&output)
			t.Run("typed_nil", func(t *testing.T) {
				require.NotPanics(t, func() {
					instance.SetLogWritter((*bytes.Buffer)(nil))
					_, err = outbound.DialContext(context.Background(), "tcp", M.ParseSocksaddr("127.0.0.1:1234"))
				})
				require.ErrorIs(t, err, io.EOF)
				if disabled {
					require.Empty(t, output.String())
				} else {
					require.Contains(t, output.String(), "blocked connection")
				}
			})

			instance.SetLogWritter(io.Discard)
			start := make(chan struct{})
			var workers sync.WaitGroup
			workers.Add(2)
			go func() {
				defer workers.Done()
				<-start
				for i := 0; i < 1000; i++ {
					outbound.DialContext(context.Background(), "tcp", M.ParseSocksaddr("127.0.0.1:1234"))
				}
			}()
			go func() {
				defer workers.Done()
				<-start
				for i := 0; i < 1000; i++ {
					instance.SetLogWritter(io.Discard)
				}
			}()
			close(start)
			workers.Wait()
		})
	}
}
