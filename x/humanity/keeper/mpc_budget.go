package keeper

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/mpc"
)

// Wieviel Vorrat an Multiplikationstripeln noch da ist — und für wie viele
// Registrierungen das reicht.
//
// WARUM DAS NÖTIG IST
//
// Der MPC-Pfad ist der einzige, bei dem NIEMAND das biometrische Merkmal
// sieht: jede Partei hält nur einen additiven Anteil, der für sich genommen
// Rauschen ist. Genau deshalb soll er eines Tages allein entscheiden — dann
// braucht dieser Dienst überhaupt keine vollständige Vorlage mehr zu halten.
//
// Er hat aber eine harte Grenze, und die ist quadratisch. Verglichen wird
// erschöpfend gegen jede Einschreibung (jede Kandidaten-Vorauswahl hat bei
// dieser Schwelle eine Trefferquote nahe null — gemessen 0,008 % bei 20
// Tabellen à 27 Bit, siehe MPCAllShares). Die n-te Registrierung kostet also
// n-1 Vergleiche, und jeder Vergleich kostet 2·512 Tripel, durch die
// Verifikation verdoppelt auf 2048.
//
//	Registrierungen R  ⇒  1024·R² Tripel
//	2.000.000 Tripel   ⇒  R = 44
//
// Läuft der Vorrat leer, verweigert mpcTriples zu Recht die Wiederverwendung
// — ein zweimal benutztes Tripel hört auf zu verblenden und verrät die
// Differenz der Geheimnisse, auf die es angewandt wurde. Nur erfährt das
// bisher niemand, bevor die erste Registrierung daran scheitert.
//
// Dieselbe Lehre wie bei der Bestandsdrift und den fehlenden Schlüsseln: ein
// Zustand, den man erst bemerkt, wenn er bricht, ist kein überwachter
// Zustand.

// tripleBudget beschreibt den Vorrat einer Partei.
type tripleBudget struct {
	Datei          string `json:"datei"`
	Gesamt         int    `json:"tripel_gesamt"`
	Verbraucht     int    `json:"tripel_verbraucht"`
	Verbleibend    int    `json:"tripel_verbleibend"`
	ProzentFrei    int    `json:"prozent_frei"`
	Eingeschrieben int    `json:"eingeschrieben"`
	NochRegistrier int    `json:"registrierungen_moeglich"`
	NaechsteKostet int    `json:"naechste_registrierung_kostet"`
	ReichtFuerNoch bool   `json:"naechste_registrierung_gedeckt"`
	Fehler         string `json:"fehler,omitempty"`
}

// tripleKosten ist der Preis einer Registrierung, die gegen n Einschreibungen
// vergleicht — einschließlich der Verdopplung durch die Verifikation.
func tripleKosten(einschreibungen, templateLen int) int {
	if einschreibungen <= 0 {
		return 0
	}
	return mpc.TriplesForVerifiedWork(mpc.TriplesForManyComparison(templateLen, einschreibungen))
}

// registrierungenAusVorrat löst 1024·R² ≤ vorrat nach R auf, mit dem bereits
// vorhandenen Bestand als Startpunkt.
//
// Aufsummiert statt geschlossen gerechnet: die Formel gilt nur, solange bei
// null angefangen wird, und der übliche Fall ist ein schon gefüllter Bestand.
func registrierungenAusVorrat(verbleibend, bestand, templateLen int) int {
	if verbleibend <= 0 {
		return 0
	}
	// Entartung vorab abfangen, nicht in der Schleife: bei templateLen 0
	// kostet JEDE Registrierung nichts und die Schleife liefe ewig.
	//
	// Der Test hat gezeigt, warum die Pruefung nicht in die Schleife gehoert:
	// dort traf sie schon bei der ERSTEN Registrierung zu, die zu Recht
	// nichts kostet -- gegen einen leeren Bestand gibt es nichts zu
	// vergleichen. Ergebnis war MaxInt32 statt 44.
	if templateLen <= 0 {
		return math.MaxInt32
	}
	moeglich := 0
	for n := bestand; ; n++ {
		kosten := tripleKosten(n, templateLen)
		if kosten > verbleibend {
			return moeglich
		}
		verbleibend -= kosten
		moeglich++
	}
}

// mpcTripleBudget liest den Vorrat, ohne ihn anzufassen.
func (a *APIServer) mpcTripleBudget(templateLen int) tripleBudget {
	b := tripleBudget{Datei: strings.TrimSpace(os.Getenv("MPC_TRIPLE_FILE"))}
	if b.Datei == "" {
		b.Fehler = "MPC_TRIPLE_FILE ist nicht gesetzt — diese Partei hat keine Tripel"
		return b
	}
	roh, err := os.ReadFile(b.Datei)
	if err != nil {
		b.Fehler = fmt.Sprintf("%s ist nicht lesbar: %v", b.Datei, err)
		return b
	}
	alle, err := mpc.DecodeTriples(roh)
	if err != nil {
		b.Fehler = fmt.Sprintf("%s ist beschädigt: %v", b.Datei, err)
		return b
	}
	b.Gesamt = len(alle)

	// Der Zähler steht in derselben Konfigurationstabelle, aus der ihn auch
	// der Zuteiler liest. Nur lesen, nicht fortschreiben — eine Abfrage darf
	// keinen Vorrat verbrauchen.
	if raw := a.blockchain.state.getConfigValueDB(mpcTripleOffsetKey); raw != "" {
		if v, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && v > 0 {
			b.Verbraucht = v
		}
	}
	b.Verbleibend = b.Gesamt - b.Verbraucht
	if b.Verbleibend < 0 {
		b.Verbleibend = 0
	}
	if b.Gesamt > 0 {
		b.ProzentFrei = int(float64(b.Verbleibend) / float64(b.Gesamt) * 100)
	}

	if committee, _, err := a.mpcCommittee(); err == nil {
		if ids, _, err := a.blockchain.state.MPCAllShares(committee.ID, 0); err == nil {
			b.Eingeschrieben = len(ids)
		}
	}

	b.NaechsteKostet = tripleKosten(b.Eingeschrieben, templateLen)
	b.ReichtFuerNoch = b.NaechsteKostet <= b.Verbleibend
	b.NochRegistrier = registrierungenAusVorrat(b.Verbleibend, b.Eingeschrieben, templateLen)
	return b
}

// handleMPCBudget beantwortet GET /mpc/budget.
//
// Ohne Token: der Vorrat ist keine Aussage über einen einzelnen Menschen,
// sondern eine Betriebsgröße — und eine, die sichtbar sein muss, BEVOR sie
// eine Registrierung scheitern lässt. Die Zahl der Eingeschriebenen steht
// ohnehin auf der Kette.
func (a *APIServer) handleMPCBudget(w http.ResponseWriter, r *http.Request) {
	const templateLen = 512 // Sketch-Bits; siehe face_sketch.py
	b := a.mpcTripleBudget(templateLen)

	w.Header().Set("Content-Type", "application/json")
	if b.Fehler != "" {
		// 200, nicht 503: die Antwort IST die Auskunft. Ein Fehlercode würde
		// eine Überwachung dazu bringen, die Ursache zu verschweigen.
		w.WriteHeader(http.StatusOK)
	}
	_ = json.NewEncoder(w).Encode(b)
}
