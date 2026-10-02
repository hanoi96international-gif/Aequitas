package keeper

import (
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"
)

// GET /binden?adresse=0x..&wallet=0x..&beweis=0x..
//
// Eine Bruecke vom Link zur App. Der Bindungscode eines Knotens
// (aequitasapp://knoten-binden?..., deploy/validator/einrichten.sh,
// deploy/knoten-binden.sh) kam bisher nur als QR-Code -- und den kann man
// nicht mit dem Handy scannen, auf dem man ihn gerade ansieht (gemeldet am
// 02.10.2026). Ein https-Link laesst sich dagegen ueberall antippen:
// Browser, Messenger, GitHub. Diese Seite zeigt Knoten und Wallet und einen
// Knopf, der die App mit genau diesem Code oeffnet. Die App prueft und
// unterschreibt wie nach dem Scannen (lib/knotenBindung.ts); hier wird nichts
// unterschrieben, nichts gespeichert, nichts angefragt.
//
// Eingaben von aussen: alle drei Werte muessen exakt dem Format entsprechen
// (Adresse 0x+40 Hex, Signatur 0x+130 Hex), sonst 400 ohne Wiedergabe der
// Eingabe. Die Seite hat kein Skript (CSP default-src 'none').

var (
	bindenAdresseRE = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
	bindenBeweisRE  = regexp.MustCompile(`^0x[0-9a-fA-F]{130}$`)
)

// knotenBindenLink: der App-Link, oder "" wenn eine Eingabe nicht passt.
func knotenBindenLink(adresse, wallet, beweis string) string {
	if !bindenAdresseRE.MatchString(adresse) || !bindenAdresseRE.MatchString(wallet) || !bindenBeweisRE.MatchString(beweis) {
		return ""
	}
	return "aequitasapp://knoten-binden?adresse=" + strings.ToLower(adresse) +
		"&wallet=" + strings.ToLower(wallet) + "&beweis=" + strings.ToLower(beweis)
}

func (a *APIServer) handleKnotenBindenSeite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	setHSTS(w, r)

	q := r.URL.Query()
	link := knotenBindenLink(q.Get("adresse"), q.Get("wallet"), q.Get("beweis"))
	if link == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, knotenBindenSeite(`<h1>Link unvollständig</h1><p>Dieser Link ist nicht vollständig oder beschädigt. Kopiere ihn bitte noch einmal ganz – oder starte die Bindung neu.</p>`))
		return
	}
	adresse := html.EscapeString(strings.ToLower(q.Get("adresse")))
	wallet := html.EscapeString(strings.ToLower(q.Get("wallet")))
	fmt.Fprint(w, knotenBindenSeite(`<h1>Knoten mit deiner Wallet verbinden</h1>
<p>Tippe auf den Knopf. Die Aequitas-App öffnet sich und zeigt dir die Bindung zum Bestätigen. Das kostet nichts und bewegt kein Geld.</p>
<a class="knopf" href="`+html.EscapeString(link)+`">In der Aequitas-App öffnen</a>
<dl><dt>Knoten (Signieradresse)</dt><dd>`+adresse+`</dd><dt>Deine Wallet</dt><dd>`+wallet+`</dd></dl>
<p class="klein">Prüfe in der App, dass die Wallet deine ist. Noch keine App? <a href="/download/app.apk">Hier herunterladen</a>.</p>`))
}

func knotenBindenSeite(inhalt string) string {
	return `<!DOCTYPE html><html lang="de"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>Knoten verbinden – Aequitas</title>
<style>
body{margin:0;background:#0A0E1A;color:#E6E9F2;font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif;display:flex;justify-content:center;padding:24px 16px}
main{max-width:460px;width:100%;background:#111827;border:1px solid #1E2D45;border-radius:16px;padding:24px}
h1{font-size:1.3rem;margin:0 0 12px}
p{line-height:1.5}
.knopf{display:block;text-align:center;background:#5B6CF0;color:#fff;text-decoration:none;font-weight:700;font-size:1.1rem;padding:16px;border-radius:999px;margin:20px 0}
dl{font-size:.85rem;word-break:break-all}
dt{color:#9AA3B8;margin-top:8px}
dd{margin:2px 0 0;font-family:ui-monospace,monospace}
.klein{font-size:.85rem;color:#9AA3B8}
.klein a{color:#9AA3B8}
</style></head><body><main>` + inhalt + `</main></body></html>`
}
