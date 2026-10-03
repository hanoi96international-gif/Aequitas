package keeper

import (
	"context"
	"fmt"
	"strings"
)

// Liquiditaetsgeber-Runde nachrechnen (Audit 2026-09-29, K-2, Schritt 3).
//
// Wie beim Grundeinkommen (nachrechnen_ubi.go) prueft erhaltung.go nur die
// Summe. Die Regel des Erzeugers (distributeLPPoolLocked) ist: jeder Halter
// von LP-Anteilen genau einmal, und zwar floor6(Anteile / alle Anteile * Topf).
// Beides steht im eigenen Zustand jedes Knotens -- es wird nachgerechnet:
//
//   - lp_kein_halter: Empfaenger haelt keine LP-Anteile.
//   - lp_doppelt:     derselbe Halter zweimal in einer Runde.
//   - lp_anteil:      Betrag weicht vom nachgerechneten ab (1 Mikro Rundung,
//     weil die Summe der Anteile je nach Reihenfolge im letzten Bit abweicht).
//   - lp_empfaenger:  nicht jeder Halter bekam seine Gutschrift.
//
// NUR NACH DER UMLAUFSICHERUNG. Davor zog der Erzeuger vor der Teilung die
// Demurrage jedes Halters ab und loeste dafuer notfalls LP-Anteile auf
// (releaseLPForAEQ) -- die Anteile, mit denen er teilte, sind beim
// Nachspielen erst nach den Gutschriften da. Seit der Umlaufsicherung gibt es
// keine Demurrage mehr; Anteile und Topf zu Beginn der Runde sind beim
// Erzeuger und beim Nachspielenden dieselben.
//
// Die Erwartung wird beim ersten lp_distribution der Runde einmal fuer alle
// Halter berechnet (O(Halter), wie beim Erzeuger) und danach nicht mehr
// veraendert; im Rueckroll-Snapshot reicht deshalb eine flache Kopie, die
// Menge der schon Bedachten wird wie beim Grundeinkommen ueber die laufende
// Nummer zurueckgesetzt. Neustart mitten in der Runde: wie beim
// Grundeinkommen -- bis zur naechsten Rundengrenze nur, was ohne Anfang geht.

type lpHolder struct {
	addr   string
	shares float64
}

// lpHalterLocked: alle Halter von LP-Anteilen und die Summe ihrer Anteile.
// Die eine Aufzaehlung fuer Erzeuger (distributeLPPoolLocked) und
// Nachspielenden.
//
// Aus der Datenbank, nicht nur aus dem Speicher (FIX P3, beta-launch audit
// 2026-07-05): ein Halter, dessen Konto aus dem Speicher verdraengt war, ging
// sonst leer aus. In der laufenden Transaktion (dbExecCtx), weil der Erzeuger
// hier in runAtomicDistributionWithOutbox und der Nachspielende in
// replayTransactions steckt -- eine eigene Verbindung haette sich mit der
// gehaltenen verklemmt.
func (cs *ChainState) lpHalterLocked(ctx context.Context) ([]lpHolder, float64, error) {
	var holders []lpHolder
	totalShares := 0.0
	if cs.db == nil {
		// Ohne Datenbank (Tests): aus dem Speicher.
		cs.accounts.Range(func(addr string, acc *AccountState) bool {
			if acc.LPShares > 0 {
				holders = append(holders, lpHolder{addr, acc.LPShares.Float()})
				totalShares += acc.LPShares.Float()
			}
			return true
		})
		return holders, totalShares, nil
	}
	rows, err := cs.dbExecCtx(ctx).Query(`SELECT lower(address) FROM chain_accounts WHERE lp_shares > 0`)
	if err != nil {
		return nil, 0, fmt.Errorf("could not enumerate LP holders: %w", err)
	}
	var addrs []string
	for rows.Next() {
		var addr string
		rows.Scan(&addr)
		if addr != "" {
			addrs = append(addrs, addr)
		}
	}
	rows.Close()
	cs.ensureAccountsLoadedCtx(ctx, addrs)
	for _, addr := range addrs {
		if acc, ok := cs.accounts.Get(addr); ok && acc.LPShares.Float() > 0 {
			holders = append(holders, lpHolder{addr, acc.LPShares.Float()})
			totalShares += acc.LPShares.Float()
		}
	}
	return holders, totalShares, nil
}

// lpAnteil: die eine Formel fuer den Anteil eines Halters.
func lpAnteil(anteile, alleAnteile, topf float64) float64 {
	return floor6((anteile / alleAnteile) * topf)
}

// lpRundePruefung: Stand der laufenden LP-Runde beim Nachspielen.
type lpRundePruefung struct {
	unsicher bool
	aktiv    bool
	// Nur wenn aktiv und nicht unsicher: Wallet -> erwarteter Betrag (Mikro).
	// Nach dem Anlegen unveraendert.
	erwartet map[string]int64
	n        int64
	bedacht  map[string]int64 // nur Halter; Wallet -> laufende Nummer, wie ubiRundePruefung
}

func (r lpRundePruefung) zurueck() lpRundePruefung {
	for k, nr := range r.bedacht {
		if nr > r.n {
			delete(r.bedacht, k)
		}
	}
	return r
}

// nachrechnenLPLocked: eine LP-Gutschrift, VOR dem Anwenden.
func (cs *ChainState) nachrechnenLPLocked(tx *Transaction, blockZeit int64) error {
	if !wirtschaftAktiv(blockZeit - demurrageUmstellungPuffer) {
		return nil
	}
	wallet := strings.ToLower(strings.TrimSpace(tx.Wallet))
	r := &cs.lpRunde
	if !r.aktiv {
		unsicher := r.unsicher
		*r = lpRundePruefung{aktiv: true, unsicher: unsicher, bedacht: make(map[string]int64)}
		if !unsicher {
			holders, alle, err := cs.lpHalterLocked(context.Background())
			if err != nil {
				r.unsicher = true
				if aerr := nachrechnenAbweichung("lp_halter_unlesbar", blockZeit, "%v", err); aerr != nil {
					return aerr
				}
			} else {
				topf := cs.topfMikroLocked(lpPoolAddr)
				r.erwartet = make(map[string]int64, len(holders))
				for _, h := range holders {
					r.erwartet[h.addr] = NewDecimal(lpAnteil(h.shares, alle, NewDecimalFromMicro(topf).Float())).Micro()
				}
			}
		}
	}
	r.n++
	// Erst: haelt der Empfaenger Anteile? Nur Halter kommen in die Menge der
	// Bedachten -- sie ist damit durch den Zustand begrenzt, nicht durch das,
	// was ein Block hineinschreibt.
	var soll int64
	if r.unsicher {
		cs.ensureAccountLoadedCtx(context.Background(), wallet)
		if acc, ok := cs.accounts.Get(wallet); !ok || acc.LPShares <= 0 {
			return nachrechnenAbweichung("lp_kein_halter", blockZeit, "%s haelt keine LP-Anteile", kurzAdresse(wallet))
		}
	} else {
		var ok bool
		if soll, ok = r.erwartet[wallet]; !ok {
			return nachrechnenAbweichung("lp_kein_halter", blockZeit,
				"%s haelt keine LP-Anteile (%.6f AEQ)", kurzAdresse(wallet), tx.Amount)
		}
	}
	if _, schon := r.bedacht[wallet]; schon {
		if err := nachrechnenAbweichung("lp_doppelt", blockZeit,
			"%s bekommt in dieser Runde zum zweiten Mal", kurzAdresse(wallet)); err != nil {
			return err
		}
	} else {
		r.bedacht[wallet] = r.n
	}
	if r.unsicher {
		return nil // ohne Anfang keine Erwartung
	}
	if d := NewDecimal(tx.Amount).Micro() - soll; d > 1 || d < -1 {
		return nachrechnenAbweichung("lp_anteil", blockZeit,
			"%s: %.6f AEQ, nachgerechnet %.6f", kurzAdresse(wallet), tx.Amount, NewDecimalFromMicro(soll).Float())
	}
	return nil
}

// nachrechnenLPAbschlussLocked: bei lp_distribution_pool_zero oder
// spaetestens der Rundenmarke.
func (cs *ChainState) nachrechnenLPAbschlussLocked(blockZeit int64) error {
	r := cs.lpRunde
	cs.lpRunde = lpRundePruefung{}
	if !r.aktiv || r.unsicher || r.erwartet == nil {
		return nil
	}
	fehlt := 0
	for w := range r.erwartet {
		if _, ok := r.bedacht[w]; !ok {
			fehlt++
		}
	}
	if fehlt > 0 {
		return nachrechnenAbweichung("lp_empfaenger", blockZeit,
			"%d von %d Haltern ohne Gutschrift", fehlt, len(r.erwartet))
	}
	return nil
}
