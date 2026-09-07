package keeper

import (
	"sync/atomic"
	"time"
)

// Wo die Zeit beim Blockbau bleibt.
//
// WARUM ES DAS GIBT. Am 07.09.2026 stand fest, dass ProduceBlock unter Last
// bis zu 4,6 Sekunden braucht -- bei BLOCK_TIME=1s. Alle naheliegenden
// Erklaerungen wurden EINZELN gemessen und schieden aus:
//
//	die beiden DB-Abfragen   0 Treffer ueber der Meldeschwelle von 1,2 s
//	den Block speichern      max 0,9 s (17 Treffer)
//	die Zustandssperre       zu 2,9 % belegt, Replay im Mittel 57 ms
//	das Sync-Tor             offen, gate_skips 3 bei clean_cycles 682
//
// Damit sind rund 3,7 der 4,6 Sekunden unerklaert, und die betroffene Box kam
// auf 0,50 statt 0,80 Bloecke/s -- der Unterschied zwischen 5.900 und ueber
// 8.000 Ueberweisungen je Sekunde in der Kette.
//
// Dieselbe Uhr hat beim Replay (replay_phasen_stats.go) drei Fehlannahmen in
// Folge widerlegt. Die Phasen plus `rest` ergeben die gemessene Gesamtdauer,
// damit ein grosses `rest` auch wirklich als solches herauskommt statt still
// in eine benannte Phase zu wandern.
var (
	pbGesamtNanos atomic.Int64
	pbBloecke     atomic.Int64
	pbSperrenNs   atomic.Int64 // auf replayMu und dag.mu warten
	pbDbPaarNs    atomic.Int64 // LoadPendingTxs + StateRoot (nebenlaeufig)
	pbBauenNs     atomic.Int64 // Block zusammensetzen und signieren
	pbSpeichernNs atomic.Int64 // SaveBlockWithPendingTxsAtomic
	pbVerteilenNs atomic.Int64 // an die Peers geben

	pbMaxGesamtNs    atomic.Int64
	pbMaxSperrenNs   atomic.Int64
	pbMaxDbPaarNs    atomic.Int64
	pbMaxBauenNs     atomic.Int64
	pbMaxSpeichernNs atomic.Int64
	pbMaxVerteilenNs atomic.Int64
	pbMaxTxAnzahl    atomic.Int64
)

func merkeProduktionsPhase(z *atomic.Int64, start time.Time) {
	z.Add(int64(time.Since(start)))
}

// merkeProduktionsBlock haelt einen abgeschlossenen Blockbau fest -- den
// Mittelwert und, getrennt davon, den teuersten Durchlauf. Ein Mittelwert
// verschluckt genau den Ausreisser, der die Blockproduktion aus dem Takt
// wirft (gemessen: Mittel unter 1 s, schlimmster 4,6 s).
func merkeProduktionsBlock(gesamt, sperren, dbPaar, bauen, speichern, verteilen time.Duration, txAnzahl int) {
	pbBloecke.Add(1)
	pbGesamtNanos.Add(int64(gesamt))
	pbSperrenNs.Add(int64(sperren))
	pbDbPaarNs.Add(int64(dbPaar))
	pbBauenNs.Add(int64(bauen))
	pbSpeichernNs.Add(int64(speichern))
	pbVerteilenNs.Add(int64(verteilen))
	for {
		alt := pbMaxGesamtNs.Load()
		if int64(gesamt) <= alt {
			break
		}
		if pbMaxGesamtNs.CompareAndSwap(alt, int64(gesamt)) {
			pbMaxSperrenNs.Store(int64(sperren))
			pbMaxDbPaarNs.Store(int64(dbPaar))
			pbMaxBauenNs.Store(int64(bauen))
			pbMaxSpeichernNs.Store(int64(speichern))
			pbMaxVerteilenNs.Store(int64(verteilen))
			pbMaxTxAnzahl.Store(int64(txAnzahl))
			break
		}
	}
}

func msAus(z *atomic.Int64, teiler int64) float64 {
	if teiler <= 0 {
		return 0
	}
	return float64(z.Load()) / float64(teiler) / 1e6
}

// ProduktionsPhasenStand meldet Mittelwerte je Block und den teuersten Bau.
func ProduktionsPhasenStand() map[string]interface{} {
	n := pbBloecke.Load()
	gesamt := msAus(&pbGesamtNanos, n)
	sperren := msAus(&pbSperrenNs, n)
	dbPaar := msAus(&pbDbPaarNs, n)
	bauen := msAus(&pbBauenNs, n)
	speichern := msAus(&pbSpeichernNs, n)
	verteilen := msAus(&pbVerteilenNs, n)
	maxGesamt := float64(pbMaxGesamtNs.Load()) / 1e6
	maxBenannt := float64(pbMaxSperrenNs.Load()+pbMaxDbPaarNs.Load()+pbMaxBauenNs.Load()+
		pbMaxSpeichernNs.Load()+pbMaxVerteilenNs.Load()) / 1e6
	return map[string]interface{}{
		"bedeutung": "Wo die Zeit beim Blockbau bleibt, je Block gemittelt. rest_ms ist die " +
			"gemessene Gesamtdauer minus alle benannten Phasen -- ist sie gross, liegt die " +
			"Ursache NICHT bei den benannten. schlimmster_* haelt den teuersten Durchlauf " +
			"vollstaendig fest, weil der Mittelwert genau den Ausreisser verschluckt, der " +
			"die Produktion aus dem Takt wirft.",
		"bloecke":               n,
		"gesamt_ms":             gesamt,
		"sperren_ms":            sperren,
		"db_paar_ms":            dbPaar,
		"bauen_ms":              bauen,
		"speichern_ms":          speichern,
		"verteilen_ms":          verteilen,
		"rest_ms":               gesamt - (sperren + dbPaar + bauen + speichern + verteilen),
		"schlimmster_ms":        maxGesamt,
		"schlimmster_sperren":   float64(pbMaxSperrenNs.Load()) / 1e6,
		"schlimmster_db_paar":   float64(pbMaxDbPaarNs.Load()) / 1e6,
		"schlimmster_bauen":     float64(pbMaxBauenNs.Load()) / 1e6,
		"schlimmster_speichern": float64(pbMaxSpeichernNs.Load()) / 1e6,
		"schlimmster_verteilen": float64(pbMaxVerteilenNs.Load()) / 1e6,
		"schlimmster_rest":      maxGesamt - maxBenannt,
		"schlimmster_tx":        pbMaxTxAnzahl.Load(),
	}
}

// ProduktionsPhasenZuruecksetzen gibt es fuer die Tests.
func ProduktionsPhasenZuruecksetzen() {
	for _, z := range []*atomic.Int64{&pbGesamtNanos, &pbBloecke, &pbSperrenNs, &pbDbPaarNs,
		&pbBauenNs, &pbSpeichernNs, &pbVerteilenNs, &pbMaxGesamtNs, &pbMaxSperrenNs,
		&pbMaxDbPaarNs, &pbMaxBauenNs, &pbMaxSpeichernNs, &pbMaxVerteilenNs, &pbMaxTxAnzahl} {
		z.Store(0)
	}
}
