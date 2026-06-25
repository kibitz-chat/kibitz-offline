package relaycore

import (
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/pion/turn/v4"
)

// TestTurnAllocates proves the relay's LAN TURN actually works: a pion/turn client can authenticate with the
// g2 blob's credential and ALLOCATE a relay address through it. That allocation is the media path offline calls
// now use (audio/video relays through the hub instead of failing peer-to-peer on a LAN). Presence is already
// covered by the LAN e2e; this covers the media relay.
func TestTurnAllocates(t *testing.T) {
	dir := t.TempDir()
	r, err := Start(Config{StatePath: dir + "/id.json"})
	if err != nil {
		t.Skipf("relay needs a private LAN address (none in this env): %v", err)
	}
	defer r.Stop()

	// blob = g2|ufrag|pwd|fp|<port>,<user>,<pass>|eps...
	parts := strings.Split(r.Blob(), "|")
	if parts[0] != "g2" {
		t.Fatalf("expected a g2 blob (TURN present), got %q in %s", parts[0], r.Blob())
	}
	spec := strings.Split(parts[4], ",")
	if len(spec) != 3 {
		t.Fatalf("bad turn spec %q", parts[4])
	}
	port, _ := strconv.Atoi(spec[0])
	user, pass := spec[1], spec[2]
	if port == 0 || user == "" || pass == "" {
		t.Fatalf("empty turn spec %q", parts[4])
	}

	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	addr := "127.0.0.1:" + strconv.Itoa(port)
	c, err := turn.NewClient(&turn.ClientConfig{
		STUNServerAddr: addr,
		TURNServerAddr: addr,
		Conn:           conn,
		Username:       user,
		Password:       pass,
		Realm:          turnRealm,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Listen(); err != nil {
		t.Fatal(err)
	}
	relayConn, err := c.Allocate()
	if err != nil {
		t.Fatalf("TURN Allocate failed (media would not relay): %v", err)
	}
	defer relayConn.Close()
	if relayConn.LocalAddr() == nil {
		t.Fatal("allocated relay conn has no address")
	}
	t.Logf("TURN allocated relay address %s via %s", relayConn.LocalAddr(), addr)
}

// TestFixedIdentity proves that with FixedIdentity on, every relay advertises the SAME well-known identity
// (ufrag + cert fingerprint) and a fixed TURN port/cred — the basis of zero-input discovery (the web bakes in
// the same constants and probes the LAN for whatever answers to them).
func TestFixedIdentity(t *testing.T) {
	FixedIdentity = true
	defer func() { FixedIdentity = false }()
	r, err := Start(Config{StatePath: t.TempDir() + "/id.json"})
	if err != nil {
		t.Skipf("relay needs a private LAN address: %v", err)
	}
	defer r.Stop()
	p := strings.Split(r.Blob(), "|")
	if p[0] != "g2" || p[1] != fixedUfrag || p[2] != fixedPwd {
		t.Fatalf("blob doesn't carry the fixed identity: %q", r.Blob())
	}
	// field 3 is the cert fingerprint (b64url); must equal the known fixed one.
	if p[3] != "iMQvZo7x61ukUXjDvSJnKmr3PFy6iDvHSa9XV3mb_kA" {
		t.Fatalf("wrong fingerprint in blob: %q", p[3])
	}
	spec := strings.Split(p[4], ",")
	if spec[0] != "3478" || spec[1] != fixedTurnUser || spec[2] != fixedTurnPass {
		t.Fatalf("turn spec isn't the fixed one: %q", p[4])
	}
}
