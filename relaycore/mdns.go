package relaycore

import (
	"net"
	"os"
	"strings"

	"github.com/pion/mdns/v2"
	"github.com/wlynxg/anet" // net.Interfaces() is empty on Android 11+ (see relay.go) → use anet
	"golang.org/x/net/ipv4"
)

// hostnameLocal returns the box's "<short-hostname>.local" — the name the OS's
// own mDNS publishes (avahi on a Pi, Bonjour on macOS), kept on the current IP.
// "" when the hostname is missing/loopback/already-qualified-elsewhere.
func hostnameLocal() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return ""
	}
	if i := strings.IndexByte(h, '.'); i >= 0 {
		h = h[:i] // strip any domain → short name
	}
	h = strings.ToLower(h)
	if h == "" || h == "localhost" {
		return ""
	}
	return h + ".local"
}

// mdnsName derives a STABLE, unique `.local` name from the relay's ufrag (which
// is persisted, so the name survives reboots). Advertised as an extra ICE
// candidate so the join code keeps working even when the relay's IP changes —
// the name resolves to the current IP via mDNS, while the raw IP candidate is
// the fast path when it hasn't changed.
func mdnsName(ufrag string) string {
	s := strings.ToLower(ufrag)
	clean := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, s)
	if len(clean) > 8 {
		clean = clean[:8]
	}
	if clean == "" {
		clean = "relay"
	}
	return "kbz-" + clean + ".local"
}

// startMDNS publishes `name` → this box's current IP over mDNS, so a peer's
// browser can resolve the `.local` candidate to whatever the IP is right now.
// Best-effort: a box without multicast (some Android Wi-Fi without a multicast
// lock) just won't answer, and peers fall back to the raw-IP candidate.
func startMDNS(name string) (*mdns.Conn, error) {
	addr, err := net.ResolveUDPAddr("udp4", mdns.DefaultAddressIPv4)
	if err != nil {
		return nil, err
	}
	sock, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, err
	}
	pc := ipv4.NewPacketConn(sock)
	// Best-effort join on every multicast interface so we receive cross-host
	// queries (pion/mdns also joins internally; errors here are non-fatal).
	group := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251)}
	ifis, _ := anet.Interfaces() // anet (not net): empty on Android 11+
	for i := range ifis {
		ifi := ifis[i]
		if ifi.Flags&net.FlagMulticast != 0 && ifi.Flags&net.FlagUp != 0 {
			_ = pc.JoinGroup(&ifi, group)
		}
	}
	conn, err := mdns.Server(pc, nil, &mdns.Config{LocalNames: []string{name}})
	if err != nil {
		_ = sock.Close()
		return nil, err
	}
	return conn, nil
}
