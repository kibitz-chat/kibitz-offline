# Cutting a release

Binaries live in **GitHub Releases**, never in git — that keeps the repo tiny
and auditable. The kibitz.chat setup page links straight at the latest release
assets, so the asset filenames must stay exactly as `build.sh` produces them.

## Steps

```sh
# 1. Build every target into dist/
./build.sh

# 2. Tag the release
git tag v0.1.0
git push origin v0.1.0

# 3. Create the GitHub release and upload all assets
gh release create v0.1.0 dist/* \
  --title "Kibitz relay v0.1.0" \
  --notes "Offline LAN video-call relay. Run one on your Wi-Fi; everyone calls in a browser at kibitz.chat — no internet, no accounts, no install."
```

That uploads the five binaries plus `kibitz-relay.service` and the Pi `README.md`.

## Asset names (do not rename — the download page depends on them)

```
kibitz-relay-windows.exe
kibitz-relay-mac-apple-silicon
kibitz-relay-mac-intel
kibitz-relay-linux
kibitz-relay-raspberry-pi
kibitz-relay.service
README.md            (the Pi guide)
```

The kibitz.chat download buttons point at
`https://github.com/<org>/kibitz-relay/releases/latest/download/<asset>`, which
always resolves to the newest release — so a new release needs no web redeploy.
