// Kibitz Offline mode — the LAN hub: a fixed-identity WebRTC responder (CLI front-end).
//
// A browser page may dial out over WebRTC without certificates, but it needs a
// signaling channel first. This hub removes that need by having a PERMANENT
// half of the handshake: fixed UDP port, fixed ICE credentials, fixed DTLS
// certificate (persisted across restarts). Its entire identity packs into one
// short blob; the web app reconstructs the hub's SDP "answer" locally from
// that blob — zero per-session signaling. The result: everyone on one Wi-Fi can
// open a browser video call with no internet, no accounts, and no install.
// (This is the offline LAN hub — not the internet TURN relay.)
//
// The reusable engine lives in package relaycore (shared with the Android app
// binding under mobile/). This file is just the desktop CLI shell:
// flags, the human/QR printout, and blocking until killed.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"kibitz-offline/relaycore"

	"github.com/mdp/qrterminal/v3"
)

// fatal logs, then on a real (double-clicked) console keeps the window open
// long enough to read the message. When stdin is NOT a terminal (piped from a
// test harness) it exits at once — otherwise it would hang holding the port.
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	if fi, err := os.Stdin.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) != 0 {
		fmt.Println("\nPress Enter to close…")
		_, _ = fmt.Scanln()
	}
	os.Exit(1)
}

func main() {
	portFlag := flag.Int("port", 0, "fixed UDP port (default: saved identity, else first free candidate)")
	statePath := flag.String("state", "relay-state.json", "identity persistence path")
	advertise := flag.String("advertise", "", "address to advertise in the blob (default: detected LAN IPs)")
	base := flag.String("base", relaycore.DefaultLinkBase, "site for the printed ?galaxy= link (e.g. https://kibitz.chat)")
	httpPort := flag.Int("http", 8080, "serve the QR page on this TCP port for headless setup (0 = off)")
	// Default ON: a downloaded hub is usually DOUBLE-CLICKED (no flags), and the point is that browsers DISCOVER it
	// with no QR. The link stays permanent on a static IP either way. --fixed-id=false → a unique per-relay identity
	// (dev/testing, or a relay you want reachable ONLY via its QR/link).
	fixedId := flag.Bool("fixed-id", true, "fixed well-known identity so browsers discover this hub with no QR (default ON); --fixed-id=false for a unique per-relay identity")
	flag.Parse()
	relaycore.FixedIdentity = *fixedId

	relay, err := relaycore.Start(relaycore.Config{
		Port: *portFlag, StatePath: *statePath, Advertise: *advertise, LinkBase: *base, HTTPPort: *httpPort,
	})
	if err != nil {
		fatal("%v", err)
	}
	defer relay.Stop()

	// #3 — drop the link next to the state file so it's trivial to grab/print later.
	if dir := filepath.Dir(*statePath); dir != "" {
		_ = os.WriteFile(filepath.Join(dir, "relay-link.txt"), []byte(relay.Link()+"\n"), 0o644)
	}

	addrs := relay.Addrs()
	fmt.Println("\n=== Kibitz · Offline mode (LAN hub) ===")
	fmt.Printf("Listening on UDP %d. Addresses advertised (phone tries each):\n", relay.Port())
	for i, a := range addrs {
		tag := ""
		if relaycore.Rank(a) == 2 {
			tag = "  (virtual adapter — phones usually can't reach this)"
		} else if i == 0 {
			tag = "  <- most likely your hotspot/Wi-Fi"
		}
		fmt.Printf("   %s:%d%s\n", a, relay.Port(), tag)
	}
	if n := relay.MDNSName(); n != "" {
		fmt.Printf("   %s:%d  (stable name — keeps the code working if the IP changes)\n", n, relay.Port())
	}
	fmt.Printf("BLOB %s\n", relay.Blob()) // machine-readable line (test harnesses parse this)
	fmt.Printf("\nLINK %s\n", relay.Link())
	if hp := relay.HTTPPort(); hp != 0 {
		// Headless box (no monitor): open this on a phone on the Wi-Fi to SEE the
		// QR — scan it, screenshot it, or print it. Same code for everyone, forever.
		fmt.Println("\nNo screen here? Open the QR page on any phone on this Wi-Fi:")
		for _, a := range addrs {
			if relaycore.Rank(a) < 2 {
				fmt.Printf("   http://%s:%d/\n", a, hp)
			}
		}
		fmt.Println("Tip: give this box a STATIC IP so a printed/saved code never goes stale.")
	}
	fmt.Print("\nScan once per phone (regular camera), then everyone here is in the call:\n\n")
	qrterminal.GenerateWithConfig(relay.Link(), qrterminal.Config{
		Writer: os.Stdout, Level: qrterminal.L,
		BlackChar: qrterminal.BLACK, WhiteChar: qrterminal.WHITE, QuietZone: 1,
	})
	fmt.Println()

	// Block until Ctrl-C / kill; the loop runs in relaycore's goroutine.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}
