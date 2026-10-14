package keeper

// Bekannte Peers ueber einen Neustart hinweg (Beta-Pruefung 14.10.2026,
// Dezentral, BLOCKER).
//
// Bisher lagen Peers nur im Speicher (PeerRegistry), und die Seeds waren nur
// C1. Ein Knoten, der neu startet, waehrend C1 nicht erreichbar ist, fand
// niemanden -- obwohl er Minuten vorher mit anderen Validatoren synchron war.
// Jetzt merkt er sich jeden Peer, mit dem ein Abgleich gelungen ist, und
// zieht beim Start die juengsten davon. Dazu kommen die Adressen der
// Leitungsmitglieder (leitungPeersZiehen).
//
// HARTE GRENZE: diese Peers sind nie trustedSeeds. Ihre Bloecke kommen als
// gewoehnliche Peer-Bloecke (FromSync=false) und durchlaufen jede Pruefung;
// nur vom Betreiber konfigurierte Seeds geniessen die FromSync-Ausnahmen.
//
// Grenzen: hoechstens peerVorratGrenze Eintraege, je Peer hoechstens ein
// Schreibzugriff je peerVorratSchreibAbstand, Eintraege verfallen nach
// peerVorratTTL, beim Start hoechstens peerVorratStart (dazu maxSyncPeers).
// Jede URL geht durch isAllowedPeerURL (oeffentliche Adresse, SSRF-Schutz).

import (
	"strings"
	"sync"
	"time"
)

const (
	peerVorratGrenze         = 64
	peerVorratStart          = 8
	peerVorratTTL            = 7 * 24 * time.Hour
	peerVorratSchreibAbstand = 10 * time.Minute
	leitungPeersAlle         = 2 * time.Minute
)

var (
	peerVorratTabelleEinmal sync.Once
	peerVorratGeschrieben   sync.Map // url -> time.Time
)

func (cs *ChainState) peerVorratTabelle() {
	peerVorratTabelleEinmal.Do(func() {
		cs.db.Exec(`CREATE TABLE IF NOT EXISTS bekannte_peers (url TEXT PRIMARY KEY, zuletzt BIGINT NOT NULL)`)
	})
}

// peerMerken: mit url ist gerade ein Abgleich gelungen.
func (cs *ChainState) peerMerken(url string, jetzt time.Time) {
	if cs == nil || cs.db == nil {
		return
	}
	url = strings.TrimRight(url, "/")
	if len(url) > 256 || !isAllowedPeerURL(url) {
		return
	}
	if v, ok := peerVorratGeschrieben.Load(url); ok && jetzt.Sub(v.(time.Time)) < peerVorratSchreibAbstand {
		return
	}
	peerVorratGeschrieben.Store(url, jetzt)
	cs.peerVorratTabelle()
	cs.db.Exec(`INSERT INTO bekannte_peers (url, zuletzt) VALUES ($1, $2)
ON CONFLICT (url) DO UPDATE SET zuletzt = EXCLUDED.zuletzt`, url, jetzt.Unix())
	cs.db.Exec(`DELETE FROM bekannte_peers WHERE zuletzt < $1`, jetzt.Add(-peerVorratTTL).Unix())
	cs.db.Exec(`DELETE FROM bekannte_peers WHERE url NOT IN
(SELECT url FROM bekannte_peers ORDER BY zuletzt DESC, url LIMIT $1)`, peerVorratGrenze)
}

// peerVorratLaden: die juengsten gemerkten Peers, ohne selbst.
func (cs *ChainState) peerVorratLaden(selbst string, jetzt time.Time) []string {
	if cs == nil || cs.db == nil {
		return nil
	}
	cs.peerVorratTabelle()
	selbst = strings.TrimRight(selbst, "/")
	rows, err := cs.db.Query(`SELECT url FROM bekannte_peers WHERE zuletzt >= $1 AND url <> $3 ORDER BY zuletzt DESC, url LIMIT $2`,
		jetzt.Add(-peerVorratTTL).Unix(), peerVorratStart, selbst)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if rows.Scan(&u) != nil {
			continue
		}
		if u != selbst && isAllowedPeerURL(u) {
			out = append(out, u)
		}
	}
	return out
}

// gemerktePeersZiehen: beim Start die gemerkten Peers abgleichen -- als
// gewoehnliche Peers, nie als trustedSeeds.
func (dag *BlockDAG) gemerktePeersZiehen(selbst string) {
	for _, u := range dag.state.peerVorratLaden(selbst, time.Now()) {
		if dag.isTrustedSyncSource(u) {
			continue // ohnehin Seed
		}
		dag.startSyncForPeer(u)
	}
}

// MitgliedURLs: die bekannten URLs der Mitglieder des Satzes, ohne diesen
// Knoten.
func (l *Leitung) MitgliedURLs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for _, a := range l.satz {
		if a == l.ich {
			continue
		}
		if u := l.urls[a]; u != "" {
			out = append(out, strings.TrimRight(u, "/"))
		}
	}
	return out
}

// leitungPeersZiehen: die Leitungsmitglieder als Sync-Peers -- regelmaessig,
// denn ein Abgleich, dessen Peer lange nicht antwortet, endet nicht, und neue
// Mitglieder kommen hinzu. startSyncForPeer prueft URL und Obergrenze.
func (dag *BlockDAG) leitungPeersZiehen(l *Leitung) {
	SafeGoroutine("leitung-peers", func() {
		ticker := time.NewTicker(leitungPeersAlle)
		defer ticker.Stop()
		for ; ; <-ticker.C {
			// Je Takt abgesichert (takt_abgesichert_test.go).
			SafeCall("leitung-peers", func() {
				for _, u := range l.MitgliedURLs() {
					dag.startSyncForPeer(u)
				}
			})
		}
	})
}
