package keeper

import "sync/atomic"

// WARUM das Nachspielen auf den teuren seriellen Pfad ausweicht.
//
// # DIE MESSUNG, DIE DAS AUSLOEST
//
// Am 06.09.2026 wurde der teuerste Replay-Halt aufgeschluesselt: Hoehe
// 5850990, 10.000 Transaktionen, 47.394 ms, davon
//
//	seriell   29.512 ms   62,3 %
//	parallel  10.084 ms   21,3 %
//
// Fast zwei Drittel gehen an einen Pfad, der je Ueberweisung zwei
// Datenbank-Umlaeufe kostet, waehrend der Buendelpfad daneben alle Konten
// eines ganzen Blocks in EIN Statement schreibt. Und das, obwohl die
// Mindestbuendelgroesse laengst auf eins steht -- ein Buendel der Laenge eins
// waere also zulaessig.
//
// Es liegt demnach nicht an zu kleinen Laeufen, sondern daran, dass der
// Buendelpfad ABLEHNT. Er hat vier Gruende dafuer, und sie verlangen
// vollkommen verschiedene Antworten:
//
//	zu_klein        collectDisjointTransferBatch fand gar nichts (Demurrage
//	                oder Nicht-Transfer bricht den Lauf sofort ab)
//	konto_fehlt     ein Konto ist nicht resident -- kalter Cache
//	guthaben        der Sender kann sich die Ueberweisung nicht leisten
//	wohlstands_cap  die Obergrenze fuer den Empfaenger greift
//
// Ohne diese Aufteilung ist jede Massnahme geraten. Ueberwiegt "guthaben",
// ist es ein Artefakt des Lasttests (leerlaufende Wegwerfkonten) und die
// Kette ist gesund. Ueberwiegt "zu_klein", liegt es an der Demurrage, und
// dann waere sie im Buendelpfad zu behandeln statt ihn abbrechen zu lassen.
// Ueberwiegt "konto_fehlt", fehlt eine Vorwaermung.
//
// Kosten: eine atomare Addition, nur im Ablehnungsfall.
var (
	baZuKlein       atomic.Int64
	baKontoFehlt    atomic.Int64
	baGuthaben      atomic.Int64
	baWohlstandsCap atomic.Int64
)

func merkeBuendelAblehnung(z *atomic.Int64) { z.Add(1) }

// BuendelAblehnungStand zeigt die Gruende in /api/health/combined.
func BuendelAblehnungStand() map[string]interface{} {
	k, f, g, w := baZuKlein.Load(), baKontoFehlt.Load(), baGuthaben.Load(), baWohlstandsCap.Load()
	return map[string]interface{}{
		"zu_klein":       k,
		"konto_fehlt":    f,
		"guthaben":       g,
		"wohlstands_cap": w,
		"summe":          k + f + g + w,
		"bedeutung": "Warum das Nachspielen auf den seriellen Pfad ausweicht -- gemessen 62 % der Sperrzeit im teuersten Block. " +
			"guthaben ueberwiegt = Artefakt leerlaufender Testkonten, die Kette ist gesund. zu_klein ueberwiegt = Demurrage " +
			"bricht die Laeufe ab, und sie gehoerte in den Buendelpfad. konto_fehlt ueberwiegt = es fehlt eine Vorwaermung.",
	}
}
