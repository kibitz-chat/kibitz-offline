// Package mobile is the gomobile-bindable face of the relay — the same
// relaycore engine, exposed through a tiny gomobile-friendly API (simple types
// only) so an Android app can run the relay in-process as a foreground
// service. Build:  gomobile bind -target=android -o relay.aar ./mobile
package mobile

import (
	"sync"

	"kibitz-offline/relaycore"
)

var (
	mu      sync.Mutex
	current *relaycore.Relay
)

// Start launches the relay, persisting its permanent identity under
// stateDir/relay-state.json. `linkBase` is the site for the returned ?galaxy=
// link (e.g. "https://kibitz.chat"; "" defaults to DefaultLinkBase). Returns the
// full <linkBase>/?galaxy=<blob> link (scan once per phone). Idempotent: a
// second call returns the running relay's link. Port 0 = auto-pick (then pinned).
func Start(stateDir string, port int, linkBase string) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if current != nil {
		return current.Link(), nil
	}
	// The phone IS the LAN hub → use the fixed, well-known identity so a guest's browser can discover + connect
	// with nothing handed to it (zero-input, no QR). See relaycore/fixedid.go for the trade-off.
	relaycore.FixedIdentity = true
	r, err := relaycore.Start(relaycore.Config{Port: port, StatePath: stateDir + "/relay-state.json", LinkBase: linkBase})
	if err != nil {
		return "", err
	}
	current = r
	return r.Link(), nil
}

// Link returns the running relay's link, or "" if not started.
func Link() string {
	mu.Lock()
	defer mu.Unlock()
	if current == nil {
		return ""
	}
	return current.Link()
}

// Blob returns the running relay's permanent pairing blob, or "".
func Blob() string {
	mu.Lock()
	defer mu.Unlock()
	if current == nil {
		return ""
	}
	return current.Blob()
}

// Running reports whether a relay is up.
func Running() bool {
	mu.Lock()
	defer mu.Unlock()
	return current != nil
}

// Stop shuts the relay down and frees the port.
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if current != nil {
		current.Stop()
		current = nil
	}
}
