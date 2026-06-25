# Kibitz Offline — architecture & how it all fits

How a group on **one Wi‑Fi with no internet** has a real browser video call — no accounts,
no install (for guests), no signaling server. This doc is the map of the whole system: the
relay in *this* repo, the blob protocol, the media path, room scoping, zero‑input discovery,
and the three kinds of host (Android app, desktop binaries, Raspberry Pi) plus the browser
guests. It also records the non‑obvious decisions, because several of them are subtle.

---

## 1. The problem, and the trick

A browser can dial **out** over WebRTC without certificates, but WebRTC still needs a
**signaling channel** to exchange SDP offers/answers — normally a server on the internet.
Offline, there is no server.

**The trick:** make one device a hub with a **permanent, pre‑known half of the handshake** —
a fixed UDP port, fixed ICE credentials (ufrag/pwd), and a fixed DTLS certificate. That whole
identity packs into one short **blob**. A browser given the blob reconstructs the hub's SDP
**answer locally** (`synthGalaxyAnswer`) — so there is **zero per‑session signaling**. Everyone
on the Wi‑Fi points their browser at the same hub via that blob and they're in a call.

The hub is deliberately **dumb**: it assigns each peer an id and routes `{to, payload}` frames.
*All* call semantics — presence, roster, media negotiation, chat — live in the **web layer** as
frames over the hub. That's why the hub binary changes rarely and the app iterates fast.

Two distinct relays, don't confuse them:
- **The offline LAN hub** (this repo): a fixed‑identity WebRTC responder for same‑Wi‑Fi calls.
- **The internet TURN relay** (the cloud `/api/turn`): for normal online calls. Unrelated.

---

## 2. The relay (this repo, package `relaycore`)

- **`relay.go`** — the engine. Binds a UDP socket; `runLoop` demuxes inbound STUN/DTLS by ICE
  ufrag (validated against pwd); is an ICE‑lite/DTLS responder; assigns peer ids; routes frames.
  This is the battle‑tested core — touch with care.
- **`loadOrCreateIdentity`** — returns the relay's identity (ufrag, pwd, DTLS cert). Two modes:
  - **Random + persisted** (default): a unique identity saved to `relay-state.json`, stable across
    restarts → the join code never changes on a static IP.
  - **Fixed** (`FixedIdentity = true`): the one **well‑known** identity (see §6).
- **`startTurn`** — a small **pion/turn** server bound on the relay (see §5).
- **`mdns.go`** — advertises a stable `.local` name so the code survives an IP change.
- **`qrpage.go`** — a tiny HTTP page (`--http`) so a headless box can show its QR on a phone.
- **`main.go`** — the desktop CLI shell (flags, the human/QR printout, block until killed).
- **`mobile/mobile.go`** — the gomobile binding the **Android app** embeds (it sets
  `FixedIdentity = true`, because the app is a discoverable appliance).

---

## 3. The blob (the link)

The blob is the hub's identity + endpoints, packed for a URL: `…/?galaxy=<blob>`.

```
g1|<ufrag>|<pwd>|<fp_b64url>|<ip~port> <ip~port> …
g2|<ufrag>|<pwd>|<fp_b64url>|<turnPort>,<turnUser>,<turnPass>|<ip~port> …
```

- `fp_b64url` — base64url of the SHA‑256 of the DTLS cert (the browser pins it).
- **g2** adds the TURN spec, so the media path can route through the hub (§5). g1 = no TURN
  (media falls back to direct host candidates — fine on a forgiving LAN, not on a phone LAN).

The web parses the blob (`parseGalaxyBlob`) and, from the local offer, **synthesizes the hub's
answer** (`synthGalaxyAnswer`) — the fingerprint, ICE creds, and candidate lines — with no round
trip. `?galaxy=…` persists in `localStorage`, so one visit to the link makes that Wi‑Fi work
forever after; `?galaxy=off` clears it.

---

## 4. Two connections per call: presence vs media

- **Presence / data hub** (`galaxyHub.ts → connectGalaxy`): the control connection to the relay —
  roster, signaling frames, chat. `lanRoom.ts` (`joinLanRoom`) builds the room on top.
- **Media mesh** (`lanMesh.ts`): the actual audio/video peer connections.

They are separate on purpose: presence is cheap and always up; media is heavy and only at join.

---

## 5. TURN media relay — why media routes *through* the hub

On a **phone** LAN, peer‑to‑peer media often fails: iOS (and others) hide host ICE candidates
behind **mDNS `.local` names** that the *other* phone can't resolve, so the direct link never
forms — even though presence (via the hub) connects fine. Symptom: two tiles appear, but remote
audio/video is dead.

Fix: the relay runs a **TURN server** (`startTurn`, pion/turn) on a real LAN IP both phones can
reach. The g2 blob carries its port + credential; the media mesh dials with
`iceTransportPolicy: 'relay'` so audio/video **relays through the hub**. That's the single change
that made offline media actually work on iPhone↔Android.

---

## 6. Fixed identity + zero‑input discovery (no QR)

The link works, but it can't help the **first** person — a *creator* who wants to start a call on a
hub that **isn't their own device** (a dedicated or headless relay, with no screen to scan a QR off).
For them, discovery bootstraps onto the hub with **zero prior info**. That's the *only* job it does:
everyone the creator then invites gets a link that already names the hub, so guests never discover —
and there is deliberately **no "browse and join a nearby call"** (a room is reached only by its link).
Discovery needs the hub's IP + ICE creds + DTLS **fingerprint** to bootstrap WebRTC — and a browser
**can't** do the obvious "type the IP, HTTP‑probe it" because an HTTPS page → `http://LAN‑IP` is
**mixed content** (blocked) and a LAN IP can't get a cert. **WebRTC isn't subject to mixed content**,
so we probe via WebRTC — but only if the browser already knows the fingerprint to pin.

**Solution — one well‑known identity.** Every hub can run the **same fixed** ICE creds + DTLS cert
(`relaycore/fixedid.go`), and the web bakes in the **same constants** (`src/core/hubDiscover.ts`,
must match `fixedid.go`). Discovery then = probe the LAN with that identity; whoever answers is the
hub. Gated by `FixedIdentity`: the Android binding sets it, and the desktop/Pi CLI `--fixed-id` flag **defaults ON**
(a double‑clicked hub can't pass flags, and the point is to be discoverable); `--fixed-id=false` → a unique
per‑relay identity (dev, or a relay reachable only via its QR/link).

The probe (`hubDiscover.ts`):
1. **`localSubnets()`** — grab the mic briefly (iOS only reveals the raw LAN IP after a media grant),
   then collect **every** private /24 from the ICE candidates. *Plural* matters: a laptop has Wi‑Fi
   **plus** VPN/Docker/virtual NICs, and the hotspot is often not the first one — scanning only the
   first silently probes the wrong subnet (this exact bug failed the desktop until fixed).
2. **`candidateIps()`** — probe order: the **last hub** that answered (cached in `kbz.lastHub` →
   near‑instant repeat connects), then each network's **`.1` gateway** (the host *is* the gateway on
   a hotspot), then common hotspot/router gateways, then the **`.2–.30` low‑DHCP** range, then the
   rest of each /24.
3. For each IP, try `connectGalaxy(configFor(ip), shortTimeout)` in small concurrent batches; the
   first to bring up the hub wins. A visible status banner (`DiscoveryStatus.tsx`) reports
   searching / found / none.

**Trade‑off (intended):** a shared identity is **public**, so the hub is **open** on the Wi‑Fi —
anyone on it can connect, and DTLS encrypts but no longer *authenticates*. That's the accepted model
for a trusted‑LAN appliance. The QR/link path still works (its blob just carries the same constants),
and a per‑room secret could be layered back on if a real gate is ever wanted.

---

## 7. The hosts

**Android app** (`kibitz/android`, the `witbitz`/`kibitz` brand flavors). The phone IS the hub: it
embeds the relay via the gomobile **AAR** (`mobile/mobile.go`, `FixedIdentity = true`) and shows the
web app in a WebView pointed at its own relay (`?galaxy=…`). **The web app is bundled into the APK**
(`src/<flavor>/assets`, served via `WebViewAssetLoader` from a virtual `https://appassets.androidplatform.net`
origin — *not* `file://`, which breaks absolute paths + the service worker). MainActivity tries the
**live** site first (fresh when online) and **falls back to the bundled copy** on any network error →
the host **cold‑starts offline, zero internet ever**. Build step per flavor: build the brand `dist`,
copy it into `src/<flavor>/assets` (gitignored), then `gradle assemble<Flavor>Debug`.

**Desktop binaries** (`build.sh` → Windows/macOS×2/Linux x64). Double‑clickable hubs. Because they're
double‑clicked (no flags), `--fixed-id` should default **ON** for them to be discoverable out of the
box (see RELEASE.md / the build).

**Raspberry Pi** (`pi/`). A set‑and‑forget appliance: static IP + a systemd unit → boots → relay runs
→ permanent code. Same binary; run with `--fixed-id` to be discoverable too.

**Browser guests** — any phone/laptop on the Wi‑Fi. They have **no bundle**, so they must load the
web app from the internet **once** to cache it (the service worker precaches the shell; `clients.claim()`
means a **single** online load is enough — no second open needed). After that the installed PWA / cached
tab opens offline. This is the inherent bootstrap: a browser guest must fetch the app once; only a
native host (the APK) can truly cold‑start with no internet.

---

## 8. The web client (in the `kibitz` repo)

- `core/galaxySignal.ts` — blob parse + `synthGalaxyAnswer` + `turnServersFor`.
- `core/galaxyHub.ts` — `connectGalaxy` (the hub) + `ensureGalaxyHub` (blob → connect, else → discover).
- `core/hubDiscover.ts` — the discovery probe + `kbz.lastHub` cache + the status emitter.
- `core/lanRoom.ts` / `core/lanMesh.ts` — room presence + the media mesh (relay‑only ICE).
- `sw.ts` — the service worker: precache the shell, navigate‑fallback to `/index.html` offline,
  `clients.claim()` so one online load suffices.
- `demo/DiscoveryStatus.tsx` — the on‑screen "Searching… / Found / No call found" banner.

---

## 9. Repos (post‑reconciliation)

- **`kibitz-chat/kibitz-offline`** (this repo) — the **single source of truth** for the relay
  (Go, MIT, open source). The desktop/Pi binaries build from here; the Android AAR builds from a
  clone of it.
- **`kibitz`** — the web app + the Android shell. Consumes the relay (AAR built from the clone). It
  does **not** vendor a copy (an earlier `relay/` vendoring was removed — it predated finding this repo).
- **`witbitz`** — a brand of the kibitz web app + the storefront; ships the Witbitz+ offline‑host APK.

`~/kibitz-relay` on the build machine is a working **clone** of this repo (changes push here).
