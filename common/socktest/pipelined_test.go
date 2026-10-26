package socktest

import (
	"bufio"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/sagernet/sing/common/buf"
	sbufio "github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/protocol/socks"

	rwshim "github.com/sagernet/sing-box/common/rwshim"
)

// payloadHandler reads the first bytes the handler receives on conn after CONNECT.
type payloadHandler struct {
	got chan []byte
}

func (h payloadHandler) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	data := make([]byte, 5)
	n, err := io.ReadFull(conn, data)
	if err != nil {
		h.got <- data[:n]
		return err
	}
	h.got <- data
	return nil
}

func (h payloadHandler) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata M.Metadata) error {
	return nil
}

// TestSocksPipelinedPayload mirrors the fork's socks call site: the first byte is
// read by the caller, then HandleConnection0 gets a buffered reader, and the
// handler receives conn. Payload sent in the same write as CONNECT must arrive.
func TestSocksPipelinedPayload(t *testing.T) {
	runSocksPayloadTest(t, true)
}

// TestSocksSequentialPayload sends the payload only after the CONNECT reply.
func TestSocksSequentialPayload(t *testing.T) {
	runSocksPayloadTest(t, false)
}

func runSocksPayloadTest(t *testing.T, pipelined bool) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	got := make(chan []byte, 1)
	serverErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		first := make([]byte, 1)
		if _, err := io.ReadFull(conn, first); err != nil {
			serverErr <- err
			return
		}
		reader := bufio.NewReader(sbufio.NewCachedReader(conn, buf.As(first)))
		serverErr <- socks.HandleConnection0(context.Background(), conn, reader, nil, rwshim.HandlerWithLeftover(payloadHandler{got: got}, reader), M.Metadata{})
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))

	// Greeting (no auth), CONNECT to 127.0.0.1:80, and a payload, all in one write.
	request := []byte{0x05, 0x01, 0x00}
	request = append(request, 0x05, 0x01, 0x00, 0x01, 127, 0, 0, 1, 0x00, 0x50)
	if pipelined {
		request = append(request, []byte("hello")...)
	}
	if _, err := client.Write(request); err != nil {
		t.Fatal(err)
	}

	method := make([]byte, 2)
	if _, err := io.ReadFull(client, method); err != nil {
		t.Fatalf("read method reply: %v", err)
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(client, reply); err != nil {
		t.Fatalf("read connect reply: %v", err)
	}

	if !pipelined {
		if _, err := client.Write([]byte("hello")); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case data := <-got:
		if string(data) != "hello" {
			t.Fatalf("handler received %q, want %q (pipelined payload lost)", data, "hello")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not receive data")
	}
	select {
	case err := <-serverErr:
		if err != nil {
			t.Logf("HandleConnection0 returned: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not finish")
	}
}
