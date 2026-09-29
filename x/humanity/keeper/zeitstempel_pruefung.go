package keeper

import "fmt"

// Zeitstempel fremder Bloecke (Audit 2026-09-29, K-3).
//
// # WARUM
//
// block.Timestamp entscheidet beim Nachspielen ueber Regeln: ab wann
// Ueberweisungen signiert sein muessen (signierteUeberweisungenPflicht), ab
// wann ein Nullifier an den Beweis gebunden ist, wie viel Liegegeld und
// Umlaufabgabe anfaellt. Geprueft wurde der Wert nie. Ein Produzent konnte
// einen Block auf einen Tag VOR einer Aktivierung datieren und damit genau
// die Pruefung abschalten, die die Aktivierung eingefuehrt hat -- etwa
// unsignierte Ueberweisungen fremder Konten.
//
// ZWEI REGELN
//
//  1. Nicht aus der Zukunft: hoechstens zeitstempelZukunftToleranz Sekunden
//     nach der eigenen Uhr. Ehrliche Knoten laufen mit NTP; zwei Minuten
//     decken jede normale Abweichung.
//  2. Nicht zurueckdatiert: hoechstens zeitstempelRueckToleranz Sekunden vor
//     dem juengsten Elternblock. Der Produzent stempelt mit seiner eigenen
//     Uhr (ProduceBlock: time.Now()), ein Elternteil von einem anderen Knoten
//     kann also um die Uhrabweichung juenger sein -- daher Toleranz statt
//     strenger Monotonie. Ruecksprung ueber Tage, wie ihn ein Angriff auf
//     eine Aktivierung braucht, ist damit ausgeschlossen.
//
// Regel 2 gilt erst, wenn der juengste Elternblock nach
// zeitstempelPruefungAbUnix liegt -- ein Nachspielen der Geschichte von
// Anfang an darf nicht an einem alten Block haengen bleiben, dessen Zeit die
// Regel damals nicht kannte. Der Stichtag haengt am ELTERN-, nicht am
// eigenen Zeitstempel: wer rueckdatiert, kann seinen eigenen Wert waehlen,
// den seiner Eltern nicht.
const (
	zeitstempelZukunftToleranz int64 = 120
	zeitstempelRueckToleranz   int64 = 120
	// 2026-10-01 00:00:00 UTC.
	zeitstempelPruefungAbUnix int64 = 1790812800
)

func zeitstempelZukunft(blockZeit, jetzt int64) string {
	if blockZeit > jetzt+zeitstempelZukunftToleranz {
		return fmt.Sprintf("timestamp %d is %ds in the future (max %ds)",
			blockZeit, blockZeit-jetzt, zeitstempelZukunftToleranz)
	}
	return ""
}

func zeitstempelRueckdatiert(blockZeit, maxElternZeit int64) string {
	if maxElternZeit < zeitstempelPruefungAbUnix {
		return ""
	}
	if blockZeit < maxElternZeit-zeitstempelRueckToleranz {
		return fmt.Sprintf("timestamp %d is %ds older than its newest parent (max %ds)",
			blockZeit, maxElternZeit-blockZeit, zeitstempelRueckToleranz)
	}
	return ""
}
