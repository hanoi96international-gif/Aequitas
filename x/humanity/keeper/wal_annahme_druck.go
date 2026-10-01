package keeper

import (
	"fmt"
	"sync/atomic"
)

// ANNAHME UNTER WAL-DRUCK: ABWEISEN STATT AUSWEICHEN.
//
// Gemessen am 01.10.2026 (C1-Pruefstand, 4.000 Konten, Goroutine-
// Schnappschuss im Messfenster): 1.027 von 3.376 Goroutinen standen in
// TransferAtomic -> walVorSeriellLeeren -> FlushWALNow an einer einzigen
// Sperre. Die Zeitreihe dazu: die WAL-Warteschlange lief in Spitzen bis
// 19.573 (Grenze walFlushMaxQueueDepth 20.000). Am Anschlag weicht der
// schnelle Pfad auf den seriellen aus (fbWarteschlange), und jede signierte
// Ueberweisung auf dem seriellen Weg erzwingt vorher einen VOLLEN Flush --
// einer nach dem anderen. Der Knoten verbrachte die Ueberlast damit, sich
// selbst auszubremsen: Inflight stand dauerhaft bei 32.000, genutzt wurden
// rund 2,4 von 8 Kernen.
//
// Ab 80 % der Warteschlange weist die Annahme deshalb mit -32005 ab
// ("gleich nochmal"), wie bei vollem Rueckstau -- in admissionRefusalReason,
// also VOR jeder Nonce-Reservierung (sendRawTransaction und
// preReserveBatchNonces pruefen sie beide zuerst). Was bereits angenommen
// ist, fliesst ab; die Grenze selbst bleibt unveraendert.
//
// walWarteschlangeStand spiegelt len(cs.walFlushQueue) und wird unter
// walFlushMu gesetzt, wo immer sich die Laenge aendert.
var (
	walWarteschlangeStand     atomic.Int64
	walWarteschlangeAbgelehnt atomic.Int64
)

// walDruckSchwelle: ab dieser Tiefe nimmt der Knoten nichts Neues an.
func walDruckSchwelle() int64 {
	return int64(walFlushMaxQueueDepth) * 8 / 10
}

// walDruckGrund: leer, solange die Warteschlange Platz hat.
func walDruckGrund() string {
	schwelle := walDruckSchwelle()
	if schwelle <= 0 {
		return ""
	}
	if n := walWarteschlangeStand.Load(); n >= schwelle {
		walWarteschlangeAbgelehnt.Add(1)
		return fmt.Sprintf("server busy: %d accepted transfers are still being written to the database (limit %d); try again shortly", n, schwelle)
	}
	return ""
}

// WALDruckStand fuer /api/health/combined.
func WALDruckStand() map[string]interface{} {
	return map[string]interface{}{
		"bedeutung": "Ab 80 % der WAL-Warteschlange weist die Annahme mit -32005 ab, statt auf den " +
			"seriellen Weg auszuweichen (der je Ueberweisung einen vollen Flush erzwingt).",
		"stand":     walWarteschlangeStand.Load(),
		"schwelle":  walDruckSchwelle(),
		"abgelehnt": walWarteschlangeAbgelehnt.Load(),
	}
}
