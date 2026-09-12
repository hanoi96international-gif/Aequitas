package keeper

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Ein Protokoll je Produktionsversuch -- die Zeitreihe, die den Mittelwerten
// fehlt.
//
// WARUM. Am 12.09.2026 stand nach einem sauberen Lauf fest: C1 packte nur
// 2.145 Ueberweisungen je Block, obwohl es 5.300/s annahm und ueber eine
// Million im Rueckstand hatte. Kandidaten: die Peer-Lag-Bremse (Deckel
// zwischen 1.500 und 7.000, je nach Rueckstand des Partners), verworfene
// Ticks (ein Tick ueber 1 s laesst den naechsten ausfallen), Sperrwartezeit.
// Die Mittelwerte in produktion_phasen.go koennen das nicht trennen: sie
// mischen leere Bloecke aus der Ruhe mit vollen aus der Last. Erst die Reihe
// "Hoehe, Deckel, Rueckstand, Ueberweisungen, Dauer, davon Sperren" je
// Versuch sagt, welcher Regler wann gegriffen hat.
//
// Ring der letzten produktionsProtokollLaenge Versuche, unter
// /api/produktion?n=600 abrufbar. Ein Eintrag mit grund != "" ist ein
// Versuch ohne Block.

const produktionsProtokollLaenge = 1800 // 30 Minuten bei einem Takt je Sekunde

type produktionsEintrag struct {
	At          int64   `json:"at_ms"`
	Hoehe       int64   `json:"hoehe"`
	Txs         int     `json:"txs"`
	Deckel      int64   `json:"deckel"`
	Rueckstand  int64   `json:"rueckstand"`
	GesamtMs    float64 `json:"gesamt_ms"`
	SperrenMs   float64 `json:"sperren_ms"`
	DbPaarMs    float64 `json:"db_paar_ms"`
	SpeichernMs float64 `json:"speichern_ms"`
	Grund       string  `json:"grund,omitempty"`
}

var (
	produktionsProtokollMu sync.Mutex
	produktionsProtokoll   []produktionsEintrag
	produktionsProtokollAb int // Ringanfang
)

func merkeProduktionsProtokoll(e produktionsEintrag) {
	produktionsProtokollMu.Lock()
	defer produktionsProtokollMu.Unlock()
	if len(produktionsProtokoll) < produktionsProtokollLaenge {
		produktionsProtokoll = append(produktionsProtokoll, e)
		return
	}
	produktionsProtokoll[produktionsProtokollAb] = e
	produktionsProtokollAb = (produktionsProtokollAb + 1) % produktionsProtokollLaenge
}

// ProduktionsProtokoll liefert die letzten n Eintraege, aelteste zuerst.
func ProduktionsProtokoll(n int) []produktionsEintrag {
	produktionsProtokollMu.Lock()
	defer produktionsProtokollMu.Unlock()
	l := len(produktionsProtokoll)
	if n <= 0 || n > l {
		n = l
	}
	out := make([]produktionsEintrag, 0, n)
	for i := l - n; i < l; i++ {
		idx := i
		if l == produktionsProtokollLaenge {
			idx = (produktionsProtokollAb + i) % produktionsProtokollLaenge
		}
		out = append(out, produktionsProtokoll[idx])
	}
	return out
}

func (a *APIServer) handleProduktionsProtokoll(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n <= 0 {
		n = 600
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"bedeutung": "Ein Eintrag je Produktionsversuch, aelteste zuerst. grund gesetzt = kein Block. " +
			"deckel/rueckstand sind der Stand der Peer-Lag-Bremse bei diesem Versuch.",
		"jetzt_ms":  time.Now().UnixMilli(),
		"eintraege": ProduktionsProtokoll(n),
	})
}

// Dasselbe fuer die ANNAHME fremder Bloecke: je Block, wie lange AddPeerBlock
// auf replayMu gewartet hat, wie lange das Replay lief (inklusive des
// Wartens auf die exklusive Zustandssperre) und wie lange das Einhaengen in
// den DAG. Die Summe ist, was ProduceBlock auf demselben Knoten als
// "sperren" sieht.

type annahmeEintrag struct {
	At           int64   `json:"at_ms"`
	Hoehe        int64   `json:"hoehe"`
	Txs          int     `json:"txs"`
	SperreMs     float64 `json:"sperre_ms"`
	ReplayMs     float64 `json:"replay_ms"`
	EinhaengenMs float64 `json:"einhaengen_ms"`
}

var (
	annahmeProtokollMu sync.Mutex
	annahmeProtokoll   []annahmeEintrag
	annahmeProtokollAb int
)

func merkeAnnahmeProtokoll(e annahmeEintrag) {
	annahmeProtokollMu.Lock()
	defer annahmeProtokollMu.Unlock()
	if len(annahmeProtokoll) < produktionsProtokollLaenge {
		annahmeProtokoll = append(annahmeProtokoll, e)
		return
	}
	annahmeProtokoll[annahmeProtokollAb] = e
	annahmeProtokollAb = (annahmeProtokollAb + 1) % produktionsProtokollLaenge
}

// AnnahmeProtokoll liefert die letzten n Eintraege, aelteste zuerst.
func AnnahmeProtokoll(n int) []annahmeEintrag {
	annahmeProtokollMu.Lock()
	defer annahmeProtokollMu.Unlock()
	l := len(annahmeProtokoll)
	if n <= 0 || n > l {
		n = l
	}
	out := make([]annahmeEintrag, 0, n)
	for i := l - n; i < l; i++ {
		idx := i
		if l == produktionsProtokollLaenge {
			idx = (annahmeProtokollAb + i) % produktionsProtokollLaenge
		}
		out = append(out, annahmeProtokoll[idx])
	}
	return out
}

func (a *APIServer) handleAnnahmeProtokoll(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n <= 0 {
		n = 600
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"bedeutung": "Ein Eintrag je angenommenem fremden Block, aelteste zuerst: Warten auf replayMu, " +
			"Replay (inkl. Warten auf die exklusive Zustandssperre), Einhaengen in den DAG.",
		"jetzt_ms":  time.Now().UnixMilli(),
		"eintraege": AnnahmeProtokoll(n),
	})
}
