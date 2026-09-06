package keeper

import (
	"fmt"
	"sync/atomic"
	"time"
)

// Wieviel Platz die Platte noch hat -- und was der Knoten tut, wenn es eng
// wird.
//
// # DER VORFALL, DER DAS AUSLOEST
//
// Am 06.09.2026 stand Contabo2 zweieinhalb Stunden still: Hoehe eingefroren,
// 11.000 Bloecke Rueckstand, keine Blockproduktion, und die ganze Kette lief
// auf einem einzigen Validator. Kein Waechter schlug an, kein Log nannte
// einen Grund. Sichtbar wurde die Ursache erst, als ein Wartungsskript
// scheiterte:
//
//	grep: write error: No space left on device
//	/dev/sda1  96G  96G  0  100% /
//
// **Null Bytes frei.** Ein Validator ohne Schreibplatz schreibt weder Bloecke
// noch WAL noch Datenbank -- er laeuft weiter, antwortet weiter, und tut
// nichts mehr. Das ist die unangenehmste Sorte Ausfall: alles sieht lebendig
// aus, nur die Hoehe steht.
//
// Danach war die Selbstheilung ebenfalls blockiert: 11.000 Bloecke liegen
// ueber maxOrphanHeightGap (5.000), also lehnt der Knoten jeden neuen Block
// als "far ahead" ab und kommt aus eigener Kraft nie zurueck. Ein Resync war
// noetig -- und der scheiterte zweimal, weil auch er Platz braucht.
//
// # WARUM EIN ZAEHLER NICHT REICHT
//
// Die Platte lief nicht durch einen Fehler voll, sondern durch normalen
// Betrieb: Docker-Build-Cache aus jedem Deploy (33,7 GB), das
// Postgres-Volume aus den Lastlaeufen (24,9 GB), Abbilder (9,4 GB). Nichts
// davon ist ungewoehnlich, und keines davon meldet sich. Deshalb misst der
// Knoten den Platz jetzt selbst und sagt es, bevor er stumm wird.
//
// # WAS ER TUT, UND WAS AUSDRUECKLICH NICHT
//
// Er WARNT (Log + /api/health/combined), und unterhalb der kritischen Grenze
// lehnt er neue Ueberweisungen ab -- mit demselben retrybaren Fehler, den
// die Annahmekontrolle schon benutzt (siehe admission_control.go). Das ist
// bewusst dieselbe Mechanik: ein reversibler Fehler ist immer besser als ein
// Knoten, der Arbeit annimmt und sie dann nicht schreiben kann.
//
// Er loescht NICHTS. Aufraeumen ist eine Entscheidung mit Folgen (welches
// Abbild, welcher Cache, welche Sicherung), und ein Knoten, der unter Druck
// selbst Dateien entfernt, ist gefaehrlicher als einer, der stehen bleibt.
// Die Werkzeuge dafuer existieren als Workflows und gehoeren in die Hand
// eines Menschen.
const (
	// Unterhalb dieser Grenze wird gewarnt: genug Vorlauf, um in Ruhe
	// aufzuraeumen, bevor etwas ausfaellt.
	plattenWarnungGB = 10
	// Unterhalb dieser Grenze werden neue Ueberweisungen abgelehnt. Der Wert
	// laesst Raum fuer die laufende WAL, einen Postgres-Checkpoint und die
	// Logs -- also fuer das, was der Knoten noch schreiben MUSS, um sauber
	// weiterzulaufen und sich spaeter erholen zu koennen.
	plattenKritischGB = 3
	// Wie oft gemessen wird. Statfs ist billig, aber es gibt keinen Grund,
	// es oefter zu tun: eine Platte laeuft nicht in Sekunden voll.
	plattenPruefIntervall = 60 * time.Second
)

var (
	plattenFreiMB     atomic.Int64
	plattenGesamtMB   atomic.Int64
	plattenPruefungen atomic.Int64
	plattenWarnungen  atomic.Int64
	plattenFehler     atomic.Value // string: warum die Messung nicht ging
	plattenPfad       atomic.Value // string
)

// plattenplatzUeberwachen misst regelmaessig den freien Platz des Pfades, in
// dem die Kette schreibt. Wird beim Start angestossen und laeuft, bis der
// Prozess endet.
func plattenplatzUeberwachen(pfad string) {
	if pfad == "" {
		pfad = "."
	}
	plattenPfad.Store(pfad)
	pruefen := func() {
		frei, gesamt, err := freierPlattenplatz(pfad)
		plattenPruefungen.Add(1)
		if err != nil {
			plattenFehler.Store(err.Error())
			return
		}
		plattenFehler.Store("")
		plattenFreiMB.Store(frei / (1 << 20))
		plattenGesamtMB.Store(gesamt / (1 << 20))
		freiGB := float64(frei) / float64(1<<30)
		switch {
		case freiGB < plattenKritischGB:
			plattenWarnungen.Add(1)
			fmt.Printf("[PLATTE] ✗ KRITISCH: nur %.2f GB frei auf %s — neue Ueberweisungen werden abgelehnt, bis Platz da ist. Ein Validator ohne Schreibplatz schreibt weder Bloecke noch WAL noch Datenbank.\n", freiGB, pfad)
		case freiGB < plattenWarnungGB:
			plattenWarnungen.Add(1)
			fmt.Printf("[PLATTE] ⚠ nur %.2f GB frei auf %s — aufraeumen, bevor es eng wird (Docker-Build-Cache und alte Abbilder sind meist der groesste Posten).\n", freiGB, pfad)
		}
	}
	pruefen() // sofort, nicht erst nach einem Intervall
	go func() {
		t := time.NewTicker(plattenPruefIntervall)
		defer t.Stop()
		for range t.C {
			pruefen()
		}
	}()
}

// plattenplatzKritisch meldet, ob der Knoten zu wenig Platz hat, um neue
// Arbeit sicher anzunehmen. Vor der ersten Messung immer false -- ein Knoten,
// der noch nichts weiss, soll nicht ablehnen.
func plattenplatzKritisch() bool {
	if plattenPruefungen.Load() == 0 {
		return false
	}
	if s, _ := plattenFehler.Load().(string); s != "" {
		return false // Messung ging nicht -- das ist kein Grund abzulehnen
	}
	return plattenFreiMB.Load() < plattenKritischGB*1024
}

// PlattenplatzStand zeigt den Platz in /api/health/combined.
func PlattenplatzStand() map[string]interface{} {
	pfad, _ := plattenPfad.Load().(string)
	fehler, _ := plattenFehler.Load().(string)
	frei := plattenFreiMB.Load()
	gesamt := plattenGesamtMB.Load()
	belegtPct := float64(0)
	if gesamt > 0 {
		belegtPct = float64(gesamt-frei) / float64(gesamt) * 100
	}
	return map[string]interface{}{
		"pfad":           pfad,
		"frei_mb":        frei,
		"gesamt_mb":      gesamt,
		"belegt_pct":     belegtPct,
		"kritisch":       plattenplatzKritisch(),
		"warnungen":      plattenWarnungen.Load(),
		"pruefungen":     plattenPruefungen.Load(),
		"messfehler":     fehler,
		"grenze_warn_gb": plattenWarnungGB,
		"grenze_krit_gb": plattenKritischGB,
		"bedeutung": "Freier Plattenplatz. Am 06.09.2026 stand ein Validator zweieinhalb Stunden still, weil die Platte " +
			"zu 100 % voll war -- er lief weiter, antwortete weiter und schrieb nichts mehr, und kein Waechter nannte den Grund. " +
			"Unter grenze_krit_gb werden neue Ueberweisungen retrybar abgelehnt, statt sie anzunehmen und nicht schreiben zu koennen. " +
			"Geloescht wird nichts: Aufraeumen gehoert in die Hand eines Menschen.",
	}
}
