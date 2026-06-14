# Kibitz relay on a Raspberry Pi — set-and-forget appliance

Make the Pi a permanent offline-call rendezvous: boots → relay runs → the join
code never changes. ~5 minutes, once.

## 1. Give the Pi a STATIC IP (important)
The join code embeds the Pi's IP, so a changing IP breaks printed/saved codes.
Reserve the Pi's IP in your router's DHCP settings, or set a static IP on the Pi
(`sudo nmtui` / `/etc/dhcpcd.conf`). Note the IP — e.g. `192.168.1.50`.

## 2. Install the binary + service
```sh
# copy the Pi binary (64-bit Raspberry Pi OS = arm64)
sudo cp kibitz-offline-raspberry-pi /usr/local/bin/kibitz-offline
sudo chmod +x /usr/local/bin/kibitz-offline

# install the service
sudo cp kibitz-offline.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now kibitz-offline
```

That's it — the relay now starts on every boot and restarts if it ever crashes.

## 3. Get the join code (no screen needed)
Any of these — they all give the SAME permanent `https://kibitz.chat/?galaxy=…`:
```sh
# the link, as text
cat /var/lib/kibitz-offline/relay-link.txt
curl http://localhost:8080/link

# the QR, on any phone on the Wi-Fi — open in a browser, scan / screenshot / print:
#   http://<the Pi's IP>:8080/
```
Print/laminate that QR and stick it on the wall. New guests scan it once; after
that they just open kibitz.chat on this Wi-Fi and they're in the call.

## Reboot behaviour
- **Identity (the code): survives** — persisted in `/var/lib/kibitz-offline`.
- **Process: auto-restarts** — that's what this service is for.
- **IP: more robust than it sounds.** The code carries BOTH the raw IP *and* a
  stable `.local` name (e.g. `kbz-ab12cd.local`) the relay publishes over mDNS.
  A guest's browser tries both: the IP is the fast path, and if the IP ever
  changes the `.local` name still resolves to the new one — so a printed code
  keeps working across most IP changes even without a static IP. A static IP
  (step 1) is still the most bulletproof, but the name is a strong safety net.

## Handy
```sh
systemctl status kibitz-offline      # is it running?
journalctl -u kibitz-offline -f      # live logs
sudo systemctl restart kibitz-offline
```

For an internet-less venue (a cabin with no router), make the Pi its own Wi-Fi
hotspot (hostapd) — then this relay is the whole infrastructure. Each guest
must have opened kibitz.chat once on real internet first (PWA cache).
