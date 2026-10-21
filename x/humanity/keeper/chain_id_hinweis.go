package keeper

import (
	"fmt"
	"strings"
	"time"
)

// Ankuendigung des Chain-ID-Wechsels auf der Startseite (docs/CHAIN_ID_WECHSEL.md,
// Schritt 3): solange ein Stichtag gesetzt ist und noch bevorsteht, steht oben
// auf der Seite, wann die Chain-ID wechselt und was Nutzer tun muessen. Danach
// nicht mehr: seit dem Neustart bei null (21.10.2026) laeuft die Kette von
// Genesis an mit der neuen ID, ein Nachlauf kuendigte nur noch Vergangenes an.
// Vom Knoten gerendert, ohne Skript (CSP script-src 'self'); der Inhalt kommt
// nur aus Konstanten, nie aus einer Anfrage.

func chainIDHinweisHTML(jetzt int64) string {
	if !chainIDWechselGeplant() {
		return ""
	}
	ab := chainIDWechselAb()
	if jetzt >= ab {
		return ""
	}
	t := time.Unix(ab, 0).UTC()
	datum := t.Format("02.01.2006, 15:04")
	date := t.Format("Jan 2, 2006, 15:04")
	return fmt.Sprintf(`
<div role="status" style="margin:16px auto;max-width:960px;padding:14px 18px;border:1px solid #c9a227;border-radius:10px;background:rgba(201,162,39,.12);color:inherit;font-size:15px;line-height:1.5">
  <strong>Chain-ID-Wechsel am %[1]s UTC:</strong> 1926 &rarr; %[3]d (0x%[4]x).
  App: auf Version 1.11.0 oder neuer aktualisieren (<a href="/download/app.apk">Download</a>).
  MetaMask und andere Wallets: das Aequitas-Netz mit Chain-ID %[3]d neu anlegen; das alte Netz danach entfernen.
  Ab 10 Minuten vor dem Wechsel nimmt der Knoten kurz keine Überweisungen an.<br>
  <strong>Chain ID change on %[2]s UTC:</strong> 1926 &rarr; %[3]d. Update the app to 1.11.0 or later; re-add the Aequitas network in MetaMask with chain ID %[3]d.
</div>`, datum, date, chainIDNeuWert, chainIDNeuWert)
}

// mitChainIDHinweis setzt den Hinweis direkt hinter das erste <main ...>
// ein (einmal; Startseite und Explorer). Ohne <main> bleibt die Seite gleich.
func mitChainIDHinweis(html string, jetzt int64) string {
	h := chainIDHinweisHTML(jetzt)
	if h == "" {
		return html
	}
	i := strings.Index(html, "<main")
	if i < 0 {
		return html
	}
	j := strings.IndexByte(html[i:], '>')
	if j < 0 {
		return html
	}
	k := i + j + 1
	return html[:k] + h + html[k:]
}
