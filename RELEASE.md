# Cutting a release

Binaries live in **GitHub Releases**, never in git — that keeps the repo tiny
and auditable. The kibitz.chat setup page links straight at the latest release
assets, so the asset filenames must stay exactly as `build.sh` produces them.

## Steps

```sh
# 1. Build every desktop/Pi target into dist/
./build.sh

# 2. Add the Android app (built in the kibitz repo: gradle :app:assembleKibitzDebug)
cp <kibitz>/android/app/build/outputs/apk/kibitz/debug/app-kibitz-debug.apk dist/kibitz-offline-android.apk

# 3. Tag + create the GitHub release with all assets
git tag v0.2.0 && git push origin v0.2.0
gh release create v0.2.0 dist/* \
  --title "Kibitz relay v0.2.0" \
  --notes "Offline LAN video-call relay — TURN media + zero-input discovery. Run one on your Wi-Fi; everyone calls in a browser at kibitz.chat."
```

That uploads the five binaries, the **Android APK**, `kibitz-offline.service`, and the Pi `README.md`.
(No `gh` CLI? Use the Releases REST API: `POST …/releases` to create it, then POST each `dist/*` to
its `uploads.github.com/…/releases/<id>/assets?name=<file>` URL — that's how v0.2.0 was cut.)

## Asset names (do not rename — the download page depends on them)

```
kibitz-offline-windows.exe
kibitz-offline-mac-apple-silicon
kibitz-offline-mac-intel
kibitz-offline-linux
kibitz-offline-raspberry-pi
kibitz-offline-android.apk   (the Android app — built in the kibitz repo)
kibitz-offline.service
README.md                    (the Pi guide)
```

The kibitz.chat download buttons point at
`https://github.com/<org>/kibitz-offline/releases/latest/download/<asset>`, which
always resolves to the newest release — so a new release needs no web redeploy.
