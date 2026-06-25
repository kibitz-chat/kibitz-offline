// Package relaycore is the Kibitz relay as an importable library — a
// fixed-identity WebRTC responder shared by the CLI binary (main.go) and
// the Android app binding (mobile/). See main.go's header for the
// architecture; this file is that logic minus the CLI shell.
package relaycore

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	mrand "math/rand"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pion/mdns/v2"

	"github.com/pion/dtls/v3"
	"github.com/pion/datachannel"
	"github.com/pion/logging"
	"github.com/pion/sctp"
	"github.com/pion/stun/v3"
	"github.com/pion/turn/v4"
	"github.com/wlynxg/anet" // Android-safe interface enumeration: net.InterfaceAddrs()/Interfaces() return EMPTY
	//                          on Android 11+ (the OS blocks the RTM_GETLINK netlink call), so we'd find no LAN
	//                          address even on Wi-Fi. anet uses getifaddrs (cgo) and works there.
)

// Alphanumeric subset of RFC ice-chars: keeps the blob URL-safe so the
// ?galaxy= link needs no escaping anywhere it travels.
const iceChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// Candidate ports tried in order when none is pinned yet.
var defaultPorts = []int{4711, 4712, 4713, 9711, 19711}

func randIce(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = iceChars[mrand.Intn(len(iceChars))]
	}
	return string(b)
}

// ---- persisted identity ----------------------------------------------------

type identity struct {
	Ufrag   string `json:"ufrag"`
	Pwd     string `json:"pwd"`
	CertPEM string `json:"certPem"`
	KeyPEM  string `json:"keyPem"`
	// The port is part of the PERMANENT identity (baked into every scanned QR)
	// — chosen on first run from free candidates, then pinned.
	Port int `json:"port,omitempty"`
}

func loadOrCreateIdentity(path string) (*identity, tls.Certificate, error) {
	if FixedIdentity {
		// Zero-input discovery: every relay shares one well-known identity so a browser can find + connect with
		// nothing handed to it (see fixedid.go). No persistence — the identity is a constant.
		cert, err := tls.X509KeyPair([]byte(fixedCertPEM), []byte(fixedKeyPEM))
		if err != nil {
			return nil, tls.Certificate{}, fmt.Errorf("fixed identity: %w", err)
		}
		return &identity{Ufrag: fixedUfrag, Pwd: fixedPwd, CertPEM: fixedCertPEM, KeyPEM: fixedKeyPEM}, cert, nil
	}
	if raw, err := os.ReadFile(path); err == nil {
		var id identity
		if json.Unmarshal(raw, &id) == nil && id.Ufrag != "" {
			cert, err := tls.X509KeyPair([]byte(id.CertPEM), []byte(id.KeyPEM))
			if err == nil {
				return &id, cert, nil
			}
		}
	}
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, tls.Certificate{}, err
	}
	tpl := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "kibitz-relay"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(20, 0, 0), // effectively forever
	}
	der, err := x509.CreateCertificate(rand.Reader, &tpl, &tpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, tls.Certificate{}, err
	}
	keyDer, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, tls.Certificate{}, err
	}
	id := identity{
		Ufrag:   randIce(8),
		Pwd:     randIce(24),
		CertPEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		KeyPEM:  string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDer})),
	}
	raw, _ := json.MarshalIndent(id, "", " ")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return nil, tls.Certificate{}, err
	}
	cert, err := tls.X509KeyPair([]byte(id.CertPEM), []byte(id.KeyPEM))
	return &id, cert, err
}

func fingerprint(cert tls.Certificate) []byte {
	sum := sha256.Sum256(cert.Certificate[0])
	return sum[:]
}

func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// Rank orders advertised addresses: real Wi-Fi/hotspot subnets first, virtual
// adapters (172.16-31.*, where WSL/Docker/Hyper-V live and phones can't reach)
// last. 0 = best. Exported so the CLI can annotate its printout consistently.
func Rank(ip string) int {
	switch {
	case strings.HasPrefix(ip, "192.168."):
		return 0
	case strings.HasPrefix(ip, "10."):
		return 1
	default:
		return 2
	}
}

// ---- per-remote virtual connection over the shared socket -------------------

type vconn struct {
	sock   *net.UDPConn
	remote *net.UDPAddr
	inbox  chan []byte
	closed chan struct{}
	once   sync.Once
}

func (c *vconn) ReadFrom(p []byte) (int, net.Addr, error) {
	select {
	case b := <-c.inbox:
		return copy(p, b), c.remote, nil
	case <-c.closed:
		return 0, nil, net.ErrClosed
	}
}
func (c *vconn) WriteTo(p []byte, _ net.Addr) (int, error) { return c.sock.WriteToUDP(p, c.remote) }
func (c *vconn) Close() error                              { c.once.Do(func() { close(c.closed) }); return nil }
func (c *vconn) LocalAddr() net.Addr                       { return c.sock.LocalAddr() }
func (c *vconn) RemoteAddr() net.Addr                      { return c.remote }
func (c *vconn) SetDeadline(t time.Time) error             { return nil }
func (c *vconn) SetReadDeadline(t time.Time) error         { return nil }
func (c *vconn) SetWriteDeadline(t time.Time) error        { return nil }

// ---- hub: ids + routing ------------------------------------------------------

type peer struct {
	id   int
	room string // multi-room: peers/to routing is scoped to this room. "" = the single shared room — a client
	//              that never sends "join" stays in it, so old clients (e.g. iwhist) are unchanged (back-compat).
	dc *datachannel.DataChannel
	mu sync.Mutex
}

func (p *peer) send(v any) {
	raw, _ := json.Marshal(v)
	p.mu.Lock()
	defer p.mu.Unlock()
	_, _ = p.dc.WriteDataChannel(raw, true)
}

type hub struct {
	mu    sync.Mutex
	next  int
	peers map[int]*peer
}

func newHub() *hub { return &hub{peers: map[int]*peer{}} }

func (h *hub) add(dc *datachannel.DataChannel) *peer {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.next++
	p := &peer{id: h.next, dc: dc}
	h.peers[p.id] = p
	return p
}

func (h *hub) remove(id int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.peers, id)
}

func (h *hub) get(id int) *peer {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.peers[id]
}

func (h *hub) ids() []int {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]int, 0, len(h.peers))
	for id := range h.peers {
		out = append(out, id)
	}
	return out
}

// --- multi-room: peers/to are scoped to a room so one relay carries many independent calls ---

// setRoom assigns a peer to a room (the "join" frame). Guarded by the same lock as the peers map so
// other peers' room reads (idsInRoom/dstInRoom) are race-free.
func (h *hub) setRoom(p *peer, room string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	p.room = room
}

// idsInRoom returns the ids of peers in `room` only — the room-scoped twin of ids(), so a "peers"
// discovery never reveals members of OTHER rooms on the same relay.
func (h *hub) idsInRoom(room string) []int {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]int, 0, len(h.peers))
	for id, pr := range h.peers {
		if pr.room == room {
			out = append(out, id)
		}
	}
	return out
}

// dstInRoom returns peer `id` only if it shares `room` — so a "to" can't cross rooms even if an id
// were guessed (room isolation is enforced, not just hidden by discovery scoping).
func (h *hub) dstInRoom(id int, room string) *peer {
	h.mu.Lock()
	defer h.mu.Unlock()
	if d := h.peers[id]; d != nil && d.room == room {
		return d
	}
	return nil
}

func (h *hub) serve(p *peer) {
	defer func() {
		h.remove(p.id)
		_ = p.dc.Close()
	}()
	p.send(map[string]any{"t": "id", "id": p.id})
	buf := make([]byte, 65536)
	for {
		n, _, err := p.dc.ReadDataChannel(buf)
		if err != nil {
			return
		}
		var msg struct {
			T       string          `json:"t"`
			ID      int             `json:"id"`
			Room    string          `json:"room"`
			Payload json.RawMessage `json:"payload"`
			X       json.RawMessage `json:"x"`
		}
		if json.Unmarshal(buf[:n], &msg) != nil {
			continue
		}
		switch msg.T {
		case "echo":
			p.send(map[string]any{"t": "echo", "x": msg.X})
		case "join":
			// multi-room: scope this peer to a room. A client sends this once (right after it gets its
			// "id", before "peers"). Omitted → the peer stays in the default room "" (back-compat with
			// old single-room clients), so one relay can now carry many independent calls at once.
			h.setRoom(p, msg.Room)
		case "peers":
			p.send(map[string]any{"t": "peers", "ids": h.idsInRoom(p.room)})
		case "to":
			if dst := h.dstInRoom(msg.ID, p.room); dst != nil { // same-room only → room isolation
				dst.send(map[string]any{"t": "from", "id": p.id, "payload": msg.Payload})
			}
		}
	}
}

// ---- public handle -----------------------------------------------------------

// DefaultLinkBase is the site whose ?galaxy= page the printed link points at.
// A var (not const) so a product's binaries can bake their own default at build
// time: -ldflags "-X kibitz-offline/relaycore.DefaultLinkBase=https://example.com".
var DefaultLinkBase = "https://kibitz.chat"

// Config controls a relay instance. Port 0 = pick from the saved identity, else
// the first free default candidate.
type Config struct {
	Port      int
	StatePath string
	Advertise string // override the advertised address (default: detected LAN IPs)
	LinkBase  string // site for the printed ?galaxy= link (default DefaultLinkBase)
	HTTPPort  int    // serve the QR page on this TCP port for headless setup (0 = off)
}

// Relay is a running relay. Stop() shuts it down.
type Relay struct {
	port     int
	addrs    []string
	blob     string
	linkBase string
	sock     *net.UDPConn
	httpSrv  *http.Server
	httpPort int
	mdns     *mdns.Conn
	mdnsName string
	turnSrv  *turn.Server // LAN TURN so offline MEDIA relays through the hub (nil if it couldn't start)
}

// MDNSName returns the published `.local` name (resilient fallback candidate),
// or "" if mDNS couldn't start on this box.
func (r *Relay) MDNSName() string { return r.mdnsName }


// Port returns the bound UDP port.
func (r *Relay) Port() int { return r.port }

// Addrs returns the advertised LAN addresses, best-first (see Rank).
func (r *Relay) Addrs() []string { return append([]string(nil), r.addrs...) }

// Blob returns the permanent `g1|…` pairing blob.
func (r *Relay) Blob() string { return r.blob }

// Link returns the full <linkBase>/?galaxy=<blob> URL.
func (r *Relay) Link() string { return r.linkBase + "/?galaxy=" + r.blob }

// HTTPPort returns the QR-page port, or 0 if the page isn't being served.
func (r *Relay) HTTPPort() int { return r.httpPort }

// Stop closes the socket (ending the packet loop), the QR page, mDNS, and frees ports.
func (r *Relay) Stop() {
	if r.mdns != nil {
		_ = r.mdns.Close()
		r.mdns = nil
	}
	if r.httpSrv != nil {
		_ = r.httpSrv.Close()
		r.httpSrv = nil
	}
	if r.sock != nil {
		_ = r.sock.Close()
		r.sock = nil
	}
	if r.turnSrv != nil {
		_ = r.turnSrv.Close()
		r.turnSrv = nil
	}
}

const turnRealm = "kibitz" // TURN auth realm; the web uses long-term creds so the realm just has to match.

// startTurn brings up a LAN TURN server (pion/turn) so OFFLINE media can RELAY THROUGH THE HUB. Offline media
// is otherwise peer-to-peer with iceServers:[] (host candidates only), and an iPhone + an Android can't resolve
// each other's mDNS `.local` host candidates -> presence connects but audio/video never does. The hub already
// has a real LAN IP both phones reach for presence, so relaying media through it is the robust path. Fresh
// random credential per run, carried in the g2 blob. Best-effort: returns srv=nil on any failure so the caller
// emits a g1 blob and peers fall back to direct media. Binds a random high UDP port (no fixed-port conflicts).
func startTurn(relayIP string) (port int, user, pass string, srv *turn.Server) {
	ip := net.ParseIP(relayIP)
	if ip == nil {
		return 0, "", "", nil
	}
	// Fixed identity → a fixed TURN port + cred so the browser knows the media relay without being told.
	bindAddr := "0.0.0.0:0"
	if FixedIdentity {
		bindAddr = fmt.Sprintf("0.0.0.0:%d", fixedTurnPort)
	}
	conn, err := net.ListenPacket("udp4", bindAddr)
	if err != nil {
		return 0, "", "", nil
	}
	port = conn.LocalAddr().(*net.UDPAddr).Port
	user, pass = randIce(10), randIce(18)
	if FixedIdentity {
		user, pass = fixedTurnUser, fixedTurnPass
	}
	key := turn.GenerateAuthKey(user, turnRealm, pass)
	s, err := turn.NewServer(turn.ServerConfig{
		Realm: turnRealm,
		AuthHandler: func(u, _ string, _ net.Addr) ([]byte, bool) {
			if u == user {
				return key, true
			}
			return nil, false
		},
		PacketConnConfigs: []turn.PacketConnConfig{{
			PacketConn: conn,
			RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
				RelayAddress: ip,        // advertise the hub's LAN IP for relayed transports
				Address:      "0.0.0.0", // listen for relayed sockets on all interfaces
			},
		}},
	})
	if err != nil {
		_ = conn.Close()
		return 0, "", "", nil
	}
	return port, user, pass, s
}

// Start binds the socket, builds the permanent blob, launches the packet loop in
// a goroutine, and returns immediately with a handle.
func Start(cfg Config) (*Relay, error) {
	id, cert, err := loadOrCreateIdentity(cfg.StatePath)
	if err != nil {
		return nil, fmt.Errorf("identity: %w", err)
	}
	fp := fingerprint(cert)

	var candidates []int
	switch {
	case cfg.Port != 0:
		candidates = []int{cfg.Port}
	case id.Port != 0:
		candidates = []int{id.Port}
	default:
		candidates = defaultPorts
	}
	var sock *net.UDPConn
	var port int
	for _, p := range candidates {
		s, err := net.ListenUDP("udp4", &net.UDPAddr{Port: p})
		if err == nil {
			sock, port = s, p
			break
		}
	}
	if sock == nil {
		return nil, fmt.Errorf("no usable UDP port (tried %v)", candidates)
	}
	if id.Port != port {
		id.Port = port
		if raw, err := json.MarshalIndent(id, "", " "); err == nil {
			_ = os.WriteFile(cfg.StatePath, raw, 0o600)
		}
	}

	addrs := []string{}
	if cfg.Advertise != "" {
		addrs = append(addrs, cfg.Advertise)
	} else {
		ifaces, _ := anet.InterfaceAddrs() // anet (not net): net.InterfaceAddrs() is empty on Android 11+
		for _, a := range ifaces {
			if ipn, ok := a.(*net.IPNet); ok {
				ip4 := ipn.IP.To4()
				if ip4 != nil && ip4.IsPrivate() {
					addrs = append(addrs, ip4.String())
				}
			}
		}
	}
	sort.SliceStable(addrs, func(i, j int) bool { return Rank(addrs[i]) < Rank(addrs[j]) })
	if len(addrs) == 0 {
		_ = sock.Close()
		return nil, fmt.Errorf("no private LAN address found — connect to the Wi-Fi/hotspot first")
	}

	eps := make([]string, 0, len(addrs)+2)
	for _, a := range addrs {
		eps = append(eps, fmt.Sprintf("%s~%d", a, port))
	}
	// Stable `.local` names as fallback candidates so a printed/saved code keeps
	// working when the IP moves. Raw IPs stay the fast path (listed first); the
	// names come last (lowest ICE priority). Two complementary sources:
	//
	//  1. The system hostname (e.g. raspberrypi.local) — published by the OS's
	//     own mDNS (avahi on a Pi, Bonjour on macOS). This is the RELIABLE path
	//     on a Pi: avahi is always on and keeps the name on the current IP, so
	//     the Pi works WITHOUT a static IP. (Dead/harmless where no OS mDNS.)
	if h := hostnameLocal(); h != "" {
		eps = append(eps, fmt.Sprintf("%s~%d", h, port))
	}
	//  2. Our own kbz-<id>.local via pion/mdns — covers boxes with NO system
	//     responder (Android, bare Windows). Skipped if the port is taken (e.g.
	//     a Pi where avahi already holds 5353 — there, source 1 carries it).
	name := mdnsName(id.Ufrag)
	mdnsConn, mdnsErr := startMDNS(name)
	if mdnsErr == nil {
		eps = append(eps, fmt.Sprintf("%s~%d", name, port))
	}
	// LAN TURN so MEDIA can relay through the hub (see startTurn). g2 carries `<port>,<user>,<pass>` as field
	// 5; the TURN listens on all interfaces, so the web aims it at each raw-IP endpoint. Best-effort: a
	// bind/setup failure falls back to g1 (no TURN -> peers attempt direct media, the old behavior).
	turnPort, turnUser, turnPass, turnSrv := startTurn(addrs[0])
	var blob string
	if turnSrv != nil {
		turnSpec := fmt.Sprintf("%d,%s,%s", turnPort, turnUser, turnPass)
		blob = strings.Join(append([]string{"g2", id.Ufrag, id.Pwd, b64url(fp), turnSpec}, eps...), "|")
	} else {
		blob = strings.Join(append([]string{"g1", id.Ufrag, id.Pwd, b64url(fp)}, eps...), "|")
	}

	linkBase := cfg.LinkBase
	if linkBase == "" {
		linkBase = DefaultLinkBase
	}
	r := &Relay{port: port, addrs: addrs, blob: blob, linkBase: linkBase, sock: sock, mdns: mdnsConn, turnSrv: turnSrv}
	if mdnsErr == nil {
		r.mdnsName = name
	}
	// Optional QR page for headless setup — best-effort (a busy port just means
	// no page; the relay itself still runs).
	if cfg.HTTPPort != 0 {
		if srv, err := startQRPage(cfg.HTTPPort, r.Link(), addrs); err == nil {
			r.httpSrv = srv
			r.httpPort = cfg.HTTPPort
		}
	}
	go runLoop(sock, cert, id.Pwd)
	return r, nil
}

// runLoop is the UDP demux → ICE-lite/DTLS/SCTP/DCEP → hub pipeline. It returns
// when the socket is closed (Stop).
func runLoop(sock *net.UDPConn, cert tls.Certificate, pwd string) {
	lf := logging.NewDefaultLoggerFactory()
	lf.DefaultLogLevel = logging.LogLevelError
	h := newHub()

	conns := map[string]*vconn{}
	var mu sync.Mutex
	integrity := stun.NewShortTermIntegrity(pwd)

	buf := make([]byte, 1500)
	for {
		n, remote, err := sock.ReadFromUDP(buf)
		if err != nil {
			return // socket closed — Stop()
		}
		pkt := make([]byte, n)
		copy(pkt, buf[:n])

		if stun.IsMessage(pkt) {
			m := &stun.Message{Raw: pkt}
			if m.Decode() != nil || m.Type != stun.BindingRequest {
				continue
			}
			if integrity.Check(m) != nil {
				continue
			}
			resp, err := stun.Build(m, stun.BindingSuccess,
				&stun.XORMappedAddress{IP: remote.IP, Port: remote.Port},
				integrity, stun.Fingerprint)
			if err == nil {
				_, _ = sock.WriteToUDP(resp.Raw, remote)
			}
			continue
		}

		key := remote.String()
		mu.Lock()
		c, ok := conns[key]
		if !ok {
			c = &vconn{sock: sock, remote: remote, inbox: make(chan []byte, 64), closed: make(chan struct{})}
			conns[key] = c
			go func() {
				defer func() {
					mu.Lock()
					delete(conns, key)
					mu.Unlock()
					_ = c.Close()
				}()
				dconn, err := dtls.Server(c, c.RemoteAddr(), &dtls.Config{
					Certificates:         []tls.Certificate{cert},
					ClientAuth:           dtls.RequireAnyClientCert,
					InsecureSkipVerify:   true,
					ExtendedMasterSecret: dtls.RequestExtendedMasterSecret,
					// Chrome attaches use_srtp to EVERY WebRTC DTLS handshake,
					// datachannel-only included; no matching profile = fatal
					// insufficient_security. We never carry media; the overlap
					// just unblocks the handshake.
					SRTPProtectionProfiles: []dtls.SRTPProtectionProfile{
						dtls.SRTP_AEAD_AES_128_GCM,
						dtls.SRTP_AES128_CM_HMAC_SHA1_80,
					},
					LoggerFactory: lf,
				})
				if err != nil {
					return
				}
				assoc, err := sctp.Server(sctp.Config{NetConn: dconn, LoggerFactory: lf})
				if err != nil {
					return
				}
				dc, err := datachannel.Accept(assoc, &datachannel.Config{LoggerFactory: lf})
				if err != nil {
					return
				}
				h.serve(h.add(dc))
			}()
		}
		mu.Unlock()
		select {
		case c.inbox <- pkt:
		default:
		}
	}
}
