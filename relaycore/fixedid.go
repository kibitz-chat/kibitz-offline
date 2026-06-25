package relaycore

// FIXED LAN-HUB IDENTITY — the basis of zero-input (no-QR) discovery.
//
// When FixedIdentity is true, every relay uses ONE well-known WebRTC identity (ICE creds + DTLS cert) and a
// fixed TURN credential on a fixed port. A guest's browser can then DISCOVER and connect to the hub with
// nothing handed to it: it probes the LAN with this baked-in identity, and the one device that answers is the
// hub. (The web side ships the same constants — ufrag/pwd, the cert FINGERPRINT, and the TURN cred.)
//
// Trade-off (intended for a trusted-LAN offline hub): a shared identity is PUBLIC, so the relay becomes an
// OPEN, UNAUTHENTICATED endpoint — anyone on the same Wi-Fi can connect, and DTLS encrypts but no longer
// authenticates (a determined LAN attacker could MITM). That's an accepted trade on your own Wi-Fi/hotspot.
// The QR path still works (its blob just carries these same constants + the IP), and a per-room secret can be
// layered back on later if a gate is ever wanted.
var FixedIdentity = false

const (
	fixedUfrag    = "wbxlanhub01"
	fixedPwd      = "774B1GNZgs48OidH45A7DZx"
	fixedTurnUser = "wblan"
	fixedTurnPass = "Sp64SHsXMZOdzT6rgYcF"
	// fixedTurnPort: a fixed UDP port so the browser knows where the media relay is without being told.
	fixedTurnPort = 3478
)

// The fixed self-signed P-256 DTLS cert. Its SHA-256 fingerprint (which the web bakes in) is
// 88:C4:2F:66:8E:F1:EB:5B:A4:51:78:C3:BD:22:67:2A:6A:F7:3C:5C:BA:88:3B:C7:49:AF:57:57:79:9B:FE:40
// (b64url: iMQvZo7x61ukUXjDvSJnKmr3PFy6iDvHSa9XV3mb_kA).
const fixedCertPEM = `-----BEGIN CERTIFICATE-----
MIIBITCBx6ADAgECAgEBMAoGCCqGSM49BAMCMBoxGDAWBgNVBAMTD3dpdGJpdHot
bGFuLWh1YjAeFw0yNTAxMDEwMDAwMDBaFw00NTAxMDEwMDAwMDBaMBoxGDAWBgNV
BAMTD3dpdGJpdHotbGFuLWh1YjBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABHpo
I8dNgYVDP+qv3iGyNqyghFAhqWSVL7PXamaYpLrII4ouPQXnl9qxY2z0eDXGQhud
T6A+VaBOGD9WlS0xAlwwCgYIKoZIzj0EAwIDSQAwRgIhAPL/RjiRTN366G5/jC+d
oUsXk/Hs/1GUbtCjPCnfqmFgAiEA0kudCGkhWx4m28XymYYGwSf75I/EJ8SlGtJU
6gXsj2s=
-----END CERTIFICATE-----
`

const fixedKeyPEM = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIKTavAkZX7r8Yi6oihvA4o3uvdXK0MYirfdKqqRNnBttoAoGCCqGSM49
AwEHoUQDQgAEemgjx02BhUM/6q/eIbI2rKCEUCGpZJUvs9dqZpikusgjii49BeeX
2rFjbPR4NcZCG51PoD5VoE4YP1aVLTECXA==
-----END EC PRIVATE KEY-----
`
