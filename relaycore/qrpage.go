package relaycore

import (
	"encoding/base64"
	"fmt"
	"html"
	"net"
	"net/http"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// startQRPage serves a tiny page at http://<lan-ip>:<port>/ showing the relay's
// permanent QR + link, so a HEADLESS box (a Pi with no monitor) can still hand
// out the code: open the Pi's address on any phone on the Wi-Fi and you see the
// QR to scan, screenshot, or print. The page is plain HTTP — it only displays
// an image of the (https) link; nothing sensitive crosses it.
func startQRPage(port int, link string, addrs []string) (*http.Server, error) {
	png, err := qrcode.Encode(link, qrcode.Medium, 512)
	if err != nil {
		return nil, err
	}
	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	body := qrPageHTML(dataURI, link, addrs)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	})
	// Plain-text link for scripting (curl http://<ip>:<port>/link).
	mux.HandleFunc("/link", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(link + "\n"))
	})

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	return srv, nil
}

func qrPageHTML(qrDataURI, link string, addrs []string) string {
	rows := make([]string, len(addrs))
	for i, a := range addrs {
		rows[i] = "<code>" + html.EscapeString(a) + "</code>"
	}
	return `<!doctype html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Kibitz relay</title>
<style>
 body{font-family:system-ui,sans-serif;background:#0b3d2e;color:#f3efe4;margin:0;
   display:flex;min-height:100vh;align-items:center;justify-content:center;padding:24px}
 .card{max-width:420px;text-align:center}
 img{width:min(80vw,360px);height:auto;background:#fff;border-radius:14px;padding:10px}
 h1{font-size:1.3rem;margin:.3rem 0}
 p{opacity:.85;line-height:1.4}
 a{color:#e2c069;word-break:break-all}
 .addrs{opacity:.6;font-size:.8rem;margin-top:12px}
</style></head><body><div class="card">
 <h1>📡 Kibitz — call on this Wi-Fi</h1>
 <img src="` + qrDataURI + `" alt="Scan to join">
 <p>Scan this with your phone camera, or open the link — Kibitz opens and
    everyone here is in the same call. No internet needed.</p>
 <p><a href="` + html.EscapeString(link) + `">` + html.EscapeString(link) + `</a></p>
 <div class="addrs">relay at ` + strings.Join(rows, " · ") + `</div>
</div></body></html>`
}
