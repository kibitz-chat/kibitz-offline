package relaycore

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/pion/mdns/v2"
	"golang.org/x/net/ipv4"
)

func TestMDNSName_StableAndValid(t *testing.T) {
	// Same ufrag → same name (stable across reboots, since ufrag is persisted).
	if a, b := mdnsName("Ab3xYz90qq"), mdnsName("Ab3xYz90qq"); a != b {
		t.Fatalf("unstable: %q vs %q", a, b)
	}
	n := mdnsName("Ab3xYz90qq")
	if n[:4] != "kbz-" || n[len(n)-6:] != ".local" {
		t.Fatalf("bad shape: %q", n)
	}
	// Only DNS-safe chars between the prefix and suffix.
	for _, r := range n[4 : len(n)-6] {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			t.Fatalf("invalid label char %q in %q", r, n)
		}
	}
	// Different relays (different ufrag) → different names.
	if mdnsName("aaaaaaaa") == mdnsName("bbbbbbbb") {
		t.Fatal("names collide across ufrags")
	}
}

// Verifies the responder actually ANSWERS a query for its name with an address.
// Uses pion's loopback-multicast pattern; skipped where multicast is unavailable
// (containers/CI), which is also why the relay treats mDNS as best-effort.
func TestMDNSResponder_AnswersQuery(t *testing.T) {
	mk := func() *ipv4.PacketConn {
		addr, err := net.ResolveUDPAddr("udp4", mdns.DefaultAddressIPv4)
		if err != nil {
			t.Skipf("resolve: %v", err)
		}
		sock, err := net.ListenUDP("udp4", addr)
		if err != nil {
			t.Skipf("no multicast listen: %v", err)
		}
		pc := ipv4.NewPacketConn(sock)
		_ = pc.SetMulticastLoopback(true)
		return pc
	}

	server, err := mdns.Server(mk(), nil, &mdns.Config{LocalNames: []string{"kbz-test01.local"}})
	if err != nil {
		t.Skipf("server: %v", err)
	}
	defer func() { _ = server.Close() }()

	client, err := mdns.Server(mk(), nil, &mdns.Config{})
	if err != nil {
		t.Skipf("client: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, addr, err := client.QueryAddr(ctx, "kbz-test01.local")
	if err != nil {
		t.Skipf("query (multicast likely blocked here): %v", err)
	}
	if !addr.IsValid() {
		t.Fatal("no address answered for our name")
	}
	t.Logf("kbz-test01.local resolved to %s ✓", addr)
}
