# Kibitz — Offline mode (the LAN hub)

**Browser video calls for everyone on one Wi-Fi — with no internet, no accounts, and nothing to install.**

> 📐 **How it all works** — the protocol, the offline media path, zero‑input discovery, and the
> Android/desktop/Pi hosts: see [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

A plane. A cabin with dead cell signal. An event hall with no guest internet. An
office that won't give you the Wi-Fi password. Run this tiny program on **one**
device on the local network — the **LAN hub** — and everyone else just opens
[kibitz.chat](https://kibitz.chat) in a browser on that same Wi-Fi — and you're
in a call. The video flows phone-to-phone across the LAN; this hub only helps
the browsers find each other.

> **Not the TURN relay.** Kibitz has two unrelated things: the internet **TURN
> relay** forwards an *online* call's encrypted media when a direct connection is
> blocked; this **LAN hub** (Offline mode) replaces the internet entirely for a
> same-Wi-Fi call. This repo is the LAN hub.

> Status: **beta.** The call engine is battle-tested (a real-world, real-device
> voice/video mesh); the Offline-mode flow is newer. Works today; rough edges
> possible.

---

## Is it safe to run? (yes — and you can check)

- **Open source, MIT.** Everything that runs is in this repo. Don't trust the
  prebuilt binary? Build it yourself in one command (below) — it's a single
  static Go program, no network calls home, no telemetry.
- **The hub can't see or hear your audio/video.** Voice and video are a WebRTC
  mesh, encrypted end-to-end (DTLS-SRTP). Where browsers reach each other directly
  the media flows browser-to-browser and the hub only introduces them; on a phone
  Wi-Fi (where iOS/mDNS breaks direct links) the media instead **relays through the
  hub's LAN TURN** — but either way the hub only ever sees **ciphertext**: it carries
  the encrypted packets and can never decrypt, record, or join your media.
- **It sees *who's* in the call — not *what* you say.** Be precise: the hub is the
  **coordination point**, so it carries the presence/roster beacons (who's connected,
  their names) and the WebRTC handshakes that pair the browsers up — that's the
  metadata it sees. But your **content — chat, co-browse, and app messages — is
  end-to-end encrypted** exactly like the audio and video: direct browser-to-browser
  when they can reach each other, or **relayed through the hub's TURN as ciphertext** on
  a phone Wi-Fi — never readable by the hub either way. And because it's a box *you*
  run on *your own* Wi-Fi, even that presence metadata never leaves your network — there's
  no operator but you. (As the coordination point it could, like any signaling server, try to
  interfere with how browsers pair up — the in-call safety code is what catches that.)
- **No accounts, no servers, no internet.** Nothing phones home. The hub binds
  one UDP port on the LAN and answers WebRTC handshakes. That's the whole job.

## How it works (the one clever bit)

A browser can dial a WebRTC connection on its own, but it needs to exchange a
little setup info ("signaling") with the other side first — normally that's what
a server is for. This hub sidesteps that by having a **permanent identity**: a
fixed UDP port, fixed ICE credentials, and a fixed DTLS certificate it persists
across restarts. That entire half of the handshake packs into one short blob,
which becomes the `?galaxy=…` in the join link / QR. The web app reconstructs
the hub's side of the handshake *locally* from that blob — so there's **zero
per-session signaling**. Scan once, connect forever.

**Or no scanning at all — when you *start* a call.** By default the hub uses one *well-known*
identity, so a **creator** whose hub is a headless box (a Pi or a laptop relay with no screen to scan
a QR off) can tap **"📡 Host on this Wi-Fi"** and the browser **discovers** the hub by probing the LAN
for it — no QR, no link. That's discovery's *only* job: bootstrapping the creator onto a hub. Everyone
they then invite gets a link that already names the hub, so **guests never discover** — and there is
deliberately **no "browse and join a nearby call"**: a room is reached only by its un-guessable link,
so no one else on the Wi-Fi can wander into your call. Pass `--fixed-id=false` if you'd rather the hub
be reachable *only* via its own code. (Trade-off: a well-known identity means the hub is **open** on
that Wi-Fi — fine for a trusted network; reaching the open hub still isn't joining a call, which the
room id gates. See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).)

---

## Quick start

> **Preview binaries (v0.2.0).** These builds aren't fully tested on
> every OS yet — Windows, macOS and Raspberry Pi are still unverified. Treat them
> as a preview and please [report what works or breaks](../../issues). Want to be
> certain what you run? [Build from source](#build-from-source) — same one command.

**1. Get the binary** for your machine from the
[**Releases**](../../releases/latest) page (or the
[kibitz.chat Offline-mode page](https://kibitz.chat/relay)):

| File | For |
|------|-----|
| `kibitz-offline-windows.exe` | Windows 10/11 |
| `kibitz-offline-mac-apple-silicon` | M1/M2/M3 Mac |
| `kibitz-offline-mac-intel` | older Intel Mac |
| `kibitz-offline-linux` | 64-bit x86 Linux |
| `kibitz-offline-raspberry-pi` | 64-bit Raspberry Pi OS (Pi 3/4/5) |

**2. Run it.**

- **Windows:** double-click. "More info → Run anyway", and allow the firewall
  prompt once (so phones on the Wi-Fi can reach it).
- **macOS / Linux / Pi:** make it executable, then run:
  ```sh
  chmod +x kibitz-offline-*
  ./kibitz-offline-*          # macOS: first run, right-click → Open (Gatekeeper)
  ```

**3. It prints a QR + a link.** Everyone on the Wi-Fi scans the QR once with
their phone camera → Kibitz opens showing **"📡 On this Wi-Fi · Local call"** →
tap in. After the first scan they can just open kibitz.chat on that Wi-Fi
forever after.

> **No screen on the box?** (a headless Raspberry Pi) The hub also serves the
> same QR over HTTP — open `http://<the-hub's-IP>:8080/` from any phone on the
> Wi-Fi to see, scan, screenshot, or print it.

> **Truly no internet at the venue?** (the hub's own hotspot, a dead-zone
> cabin) Each person should open kibitz.chat once on real internet beforehand —
> it caches itself (PWA) and then loads with no connection.

---

## Build from source

One static binary, no C dependencies. With [Go](https://go.dev/dl/) 1.25+:

```sh
git clone <this repo> && cd kibitz-offline
go build -o kibitz-offline .     # your platform
./kibitz-offline                 # run it
```

Cross-compile every target into `dist/` (the files shipped on Releases):

```sh
./build.sh
```

Useful flags:

```
--fixed-id     use the well-known identity so browsers discover the hub with NO QR
               (default ON; --fixed-id=false → a unique per-relay identity, QR/link only)
--port N       fixed UDP port (default: saved identity, else a free port)
--state PATH   where to persist the permanent identity (default ./relay-state.json)
--base URL     site the printed link points at (default https://kibitz.chat)
--http N       serve the headless QR page on TCP N (default 8080; 0 = off)
--advertise A  override the advertised LAN address
```

## Run it forever on a Raspberry Pi

A Pi makes a perfect set-and-forget appliance: boots → the hub runs → the join
code never changes. See [`pi/README.md`](pi/README.md) for the 5-minute systemd
setup (it ships in every release too).

## Android

The hub also runs as an **Android app** (`kibitz-offline-android.apk` on the
[Releases](../../releases/latest) page) — a spare phone becomes the box and keeps running across
reboots. The app **bundles the web client**, so it **cold-starts the call UI with no internet at all**
(no "load it online once" first). The same `mobile/` package here is what it binds via
[gomobile](https://pkg.go.dev/golang.org/x/mobile/cmd/gomobile); the APK itself is built in the kibitz
repo (the Android shell). Play Store listing pending.

---

## Layout

```
main.go        desktop CLI (flags, the QR printout)
relaycore/     the engine: the fixed-identity WebRTC responder + the data hub
mobile/        gomobile-bindable face of the same engine (Android)
pi/            systemd unit + appliance guide
build.sh       cross-compile all targets into dist/
```

MIT licensed. Built from a battle-tested in-call WebRTC engine. (The internal Go
package is still named `relaycore` — it's the same engine that coordinates the LAN.)
