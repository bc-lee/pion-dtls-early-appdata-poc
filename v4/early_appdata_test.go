package repro

import (
	"context"
	"encoding/binary"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/pion/dtls/v4"
	"github.com/pion/dtls/v4/pkg/crypto/selfsign"
	"github.com/pion/dtls/v4/pkg/protocol"
)

// containsFinalHandshakeRecord reports whether the datagram holds a handshake
// record of epoch 1, which in DTLS 1.2 is the encrypted Finished.
func containsFinalHandshakeRecord(p []byte) bool {
	for len(p) >= 13 {
		contentType := p[0]
		epoch := binary.BigEndian.Uint16(p[3:5])
		length := int(binary.BigEndian.Uint16(p[11:13]))
		if length > len(p)-13 {
			return false
		}
		if contentType == 22 && epoch == 1 {
			return true
		}
		p = p[13+length:]
	}
	return false
}

// slowLastFlight delays handshake completion after sending Finished.
type slowLastFlight struct{ net.PacketConn }

func (c slowLastFlight) WriteTo(p []byte, addr net.Addr) (int, error) {
	n, err := c.PacketConn.WriteTo(p, addr)
	if err == nil && n == len(p) && containsFinalHandshakeRecord(p) {
		time.Sleep(200 * time.Millisecond)
	}
	return n, err
}

func TestEarlyApplicationData(t *testing.T) {
	cert, err := selfsign.GenerateSelfSigned()
	if err != nil {
		t.Fatal(err)
	}
	serverUDP, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	clientUDP, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	// The delay uses the DTLS 1.2 record layout; later v4 releases default to 1.3.
	server, err := dtls.Server(slowLastFlight{serverUDP}, clientUDP.LocalAddr(),
		dtls.WithCertificates(cert),
		dtls.WithMinVersion(protocol.Version1_2), dtls.WithMaxVersion(protocol.Version1_2))
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client, err := dtls.Client(clientUDP, serverUDP.LocalAddr(),
		dtls.WithInsecureSkipVerify(true),
		dtls.WithMinVersion(protocol.Version1_2), dtls.WithMaxVersion(protocol.Version1_2))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.HandshakeContext(ctx) }()
	if err := client.HandshakeContext(ctx); err != nil {
		t.Fatal(err)
	}
	sent := []string{"one", "two", "three"}
	for _, m := range sent {
		if _, err := client.Write([]byte(m)); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("the server handshake did not finish")
	}

	var got []string
	buf := make([]byte, 64)
	for i := range sent {
		if err := server.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		n, err := server.Read(buf)
		if err != nil {
			t.Fatalf("read %d of %d failed: %v; server read %q, client sent %q", i+1, len(sent), err, got, sent)
		}
		got = append(got, string(buf[:n]))
	}
	if !slices.Equal(got, sent) {
		t.Fatalf("server read %q, client sent %q", got, sent)
	}
}
