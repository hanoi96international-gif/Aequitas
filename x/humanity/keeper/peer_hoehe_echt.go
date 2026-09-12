package keeper

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// DIE ECHTE HOEHE DES PARTNERS, NICHT DIE AUS DEM SYNC-ZYKLUS ABGELEITETE.
//
// Die Bremse in peer_lag_bremse.go verkleinert Bloecke, wenn ein Peer
// zurueckfaellt. Sie las den Rueckstand bisher aus zwei Werten, die
// advancePeerSyncHeight nebeneinander fuehrt:
//
//	peerSyncHeight[url]      -- nur fortgeschrieben, wenn HOEHER ("if height >")
//	peerSyncEigeneHoehe[url] -- IMMER auf die aktuelle eigene Hoehe gesetzt
//
// Uebergeben wird dabei highestSeen: die hoechste Hoehe, die diese eine
// Sync-Runde beim Peer gesehen hat. Das ist nicht die Hoehe des Peers. Holt
// eine Runde nichts Neues -- weil die Seite schon bekannte Bloecke enthielt,
// weil das Zeitbudget riss, weil gerade nichts anlag --, bleibt der eine Wert
// stehen, waehrend der andere weiterlaeuft. Der "Rueckstand" waechst dann mit
// jedem selbst produzierten Block, ohne dass der Partner zurueckliegt.
//
// GEMESSEN AM 11.09.2026: beide Boxen auf identischer Hoehe, byte-identischer
// Blockhash -- und beide meldeten gegenseitig einen Rueckstand von rund 5.160
// Bloecken. Die Bremse drosselte daraufhin 100 % aller Bloecke auf ihren Boden
// von 3.000, waehrend der harte Deckel bei 7.000 stand. Gemessen wurden
// 2.949 Transaktionen je Block -- also mehr als die Haelfte der Kapazitaet
// verschenkt, aus einem Rechenfehler.
//
// Derselbe Fehler wurde am 02.09.2026 schon einmal behoben; der Kommentar in
// groesstenFrischenRueckstand beschreibt ihn wortwoertlich ("sonst waechst der
// Rueckstand mit jedem selbst produzierten Block"). Der Fix setzte damals an
// der Rechnung an, nicht an der Quelle -- und die Quelle war das Problem.
//
// Hier wird die Hoehe darum direkt gefragt, so wie autoheal.go es fuer den
// Primary laengst tut (fetchPrimaryHeight). Eine HTTP-Abfrage je Peer alle
// paar Sekunden kostet nichts gegen einen halbierten Durchsatz.

const (
	peerHoeheIntervall = 5 * time.Second
	// Aelter als das, und der Wert wird nicht mehr benutzt: dann greift der
	// alte Weg. Grosszuegig, weil eine einzelne verpasste Abfrage keine
	// Drosselung ausloesen soll.
	peerHoeheFrische = 30 * time.Second
)

var (
	peerEchteHoeheMu sync.RWMutex
	peerEchteHoehe   = map[string]int64{}
	peerEchteHoeheAt = map[string]time.Time{}
	// peerEchteHoeheEigene: die EIGENE Hoehe im Moment der Abfrage. Der
	// Rueckstand ist die Differenz zweier Hoehen ZUM SELBEN ZEITPUNKT; wer
	// die Antwort von vor 4 s gegen die eigene Hoehe von jetzt rechnet, sieht
	// bei einer Hoehe je Sekunde 4 Hoehen Rueckstand, die es nicht gibt.
	// Gemessen 12.09.2026, Lauf 12:02 UTC: lag 6 bei Slack 5, obwohl beide
	// jeden Takt produzierten -- die Bremse hielt C1 auf 3.600 statt 7.000.
	peerEchteHoeheEigene               = map[string]int64{}
	peerHoeheAbfragen, peerHoeheFehler int64
)

// StartePeerHoehenAbfrage fragt regelmaessig die echte Hoehe jedes bekannten
// Peers ab. Die Peers kommen aus peerSyncHeight -- wer dort steht, hat schon
// einmal geantwortet.
func (dag *BlockDAG) StartePeerHoehenAbfrage() {
	SafeGoroutine("peerHoehenAbfrage", func() {
		t := time.NewTicker(peerHoeheIntervall)
		defer t.Stop()
		for range t.C {
			SafeCall("peerHoehenAbfrage-tick", func() { dag.peerHoehenEinmalHolen() })
		}
	})
}

func (dag *BlockDAG) peerHoehenEinmalHolen() {
	dag.syncPeerMu.Lock()
	urls := make([]string, 0, len(dag.peerSyncHeight))
	for u := range dag.peerSyncHeight {
		urls = append(urls, u)
	}
	dag.syncPeerMu.Unlock()

	for _, u := range urls {
		eigene := dag.heightSchnell.Load()
		h, ok := holePeerHoehe(u)
		peerEchteHoeheMu.Lock()
		peerHoeheAbfragen++
		if ok {
			peerEchteHoehe[u] = h
			peerEchteHoeheAt[u] = time.Now()
			peerEchteHoeheEigene[u] = eigene
		} else {
			peerHoeheFehler++
		}
		peerEchteHoeheMu.Unlock()
	}
}

// holePeerHoehe liest die Hoehe aus /api/status.
func holePeerHoehe(url string) (int64, bool) {
	resp, err := httpSyncClient.Get(url + "/api/status")
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, false
	}
	var st struct {
		Height int64 `json:"height"`
	}
	if json.NewDecoder(resp.Body).Decode(&st) != nil {
		return 0, false
	}
	return st.Height, true
}

// echteHoeheVonPeer liefert die zuletzt gemessene Hoehe, wenn sie frisch
// genug ist. false heisst: der alte Weg muss ran.
func echteHoeheVonPeer(url string) (int64, bool) {
	h, _, ok := echteHoeheVonPeerMitEigener(url)
	return h, ok
}

// echteHoeheVonPeerMitEigener liefert dazu die eigene Hoehe im Moment der
// Abfrage -- gegen DIE ist der Rueckstand zu rechnen.
func echteHoeheVonPeerMitEigener(url string) (peer, eigene int64, ok bool) {
	peerEchteHoeheMu.RLock()
	defer peerEchteHoeheMu.RUnlock()
	h, da := peerEchteHoehe[url]
	if !da {
		return 0, 0, false
	}
	if time.Since(peerEchteHoeheAt[url]) > peerHoeheFrische {
		return 0, 0, false
	}
	return h, peerEchteHoeheEigene[url], true
}

// PeerHoehenStand zeigt, worauf sich die Bremse gerade stuetzt.
func PeerHoehenStand() map[string]interface{} {
	peerEchteHoeheMu.RLock()
	defer peerEchteHoeheMu.RUnlock()
	je := map[string]interface{}{}
	for u, h := range peerEchteHoehe {
		je[u] = fmt.Sprintf("%d (vor %s)", h, time.Since(peerEchteHoeheAt[u]).Round(time.Second))
	}
	return map[string]interface{}{
		"bedeutung": "Die direkt gefragte Hoehe jedes Peers. Die Bremse rechnete frueher mit " +
			"der hoechsten in einer Sync-Runde GESEHENEN Hoehe -- das ist nicht dasselbe, und " +
			"am 11.09.2026 meldeten beide Boxen dadurch gegenseitig 5.160 Bloecke Rueckstand, " +
			"obwohl sie byte-identisch auf derselben Hoehe standen.",
		"hoehen":   je,
		"abfragen": peerHoeheAbfragen,
		"fehler":   peerHoeheFehler,
	}
}
