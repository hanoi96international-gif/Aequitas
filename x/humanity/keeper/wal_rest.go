package keeper

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/wal"
	"github.com/lib/pq"
)

// WAL-Rest beim Start (Pruefung von #322, INFO-15; Pruefung von #331).
//
// Ein Knoten, der den Rest seines WAL nicht einspielt, darf nicht laufen:
// Spielte er fremde Bloecke ohne diese Ueberweisungen nach (Beobachter) oder
// nahm er weiter an (Validator mit ausgeschaltetem WAL oder gescheitertem
// Wiederanlauf), liefe sein Stand ohne sie weiter. Ein spaeterer Wiederanlauf
// wendete die alten Saetze dann auf den weitergelaufenen Stand an -- ohne
// Deckungspruefung (PoC: Kontostand -42,042 und eine Zeile im Ausgang, die
// jeder andere Knoten beim Nachspielen abweist), und der Absender haette das
// schon ueberwiesene Guthaben inzwischen ein zweites Mal ausgeben koennen.
//
// PruefeWALRest: main beendet sich, wenn das WAL nicht abgeglichene Saetze
// traegt und dieser Prozess sie nicht eingespielt hat (cs.wal == nil).
// Auswege: als Validator mit eingeschaltetem WAL wiederanlaufen lassen -- oder
// den Rest bewusst verwerfen, einmalig fuer genau den gemeldeten Stand
// (AEQUITAS_WAL_REST_VERWERFEN=<Kopf-seq>; setzt die Untergrenze des
// Wiederanlaufs auf das Dateiende, wie nach einem Zustandsersatz).

// walRestHoechstensKonten begrenzt, wie viele Konten die Pruefung beim Start
// sammelt (ein Eintrag je Konto). Mehr: fail-closed, als waere der Rest offen.
var walRestHoechstensKonten = 1_000_000 // var: Tests senken sie

// walPfad: wie initWALIfEnabled und markWALSupersededByStateReplacement.
func walPfad() string {
	if p := os.Getenv("AEQUITAS_WAL_PATH"); p != "" {
		return p
	}
	return "aequitas_transfers.wal"
}

// PruefeWALRest: Fehler heisst, nicht starten. Hat dieser Prozess das WAL
// geoeffnet (cs.wal), ist der Rest eingespielt und wird geflusht -- dann wird
// die Datei gar nicht erst gelesen.
func (cs *ChainState) PruefeWALRest() error {
	if cs.wal != nil {
		return nil
	}
	pfad := walPfad()
	offen, kopf, err := cs.walRestOffen(pfad)
	// Ist der Wiederanlauf gescheitert, startet der Knoten nie -- auch wenn
	// wal_seq keinen Rest mehr zeigt: ein Teil kann eingespielt und vom
	// Flush-Arbeiter schon nach Postgres geschrieben sein, im Korb-Modus ohne
	// Ausgangszeile (Pruefung von #331, 2. Durchgang, Befund 1).
	gescheitert := cs.walWiederanlaufFehler
	if err == nil && offen == 0 && gescheitert == "" {
		return nil
	}
	warum := "der Wiederanlauf des WAL ist gescheitert (siehe die [WAL]-Meldungen oben)"
	switch {
	case gescheitert != "":
		warum = "der Wiederanlauf des WAL ist gescheitert: " + gescheitert
	case beobachterModus():
		warum = "ein Beobachter (AEQUITAS_BEOBACHTER) liest das WAL nicht ein"
	case os.Getenv("AEQUITAS_WAL_ENABLED") != "1":
		warum = "das WAL ist ausgeschaltet (AEQUITAS_WAL_ENABLED)"
	case cs.db == nil:
		warum = "ohne Datenbank wird das WAL nicht eingespielt"
	}
	if err != nil {
		return fmt.Errorf("WAL %s nicht pruefbar (%v), und %s -- Knoten startet nicht", pfad, err, warum)
	}
	soll := strconv.FormatUint(kopf, 10)
	if v := strings.TrimSpace(os.Getenv("AEQUITAS_WAL_REST_VERWERFEN")); v != "" && cs.db != nil {
		if v != soll {
			return fmt.Errorf("WAL %s: AEQUITAS_WAL_REST_VERWERFEN=%s passt nicht zum Stand der Datei (Kopf-seq %s) -- nichts verworfen, Knoten startet nicht", pfad, v, soll)
		}
		cs.markWALSupersededByStateReplacement()
		if rest, _, err2 := cs.walRestOffen(pfad); err2 != nil || rest != 0 {
			return fmt.Errorf("WAL %s: Rest (%d Konten) liess sich nicht verwerfen -- Knoten startet nicht", pfad, offen)
		}
		if gescheitert != "" {
			// Dieser Prozess hat einen halben Wiederanlauf hinter sich; erst der
			// naechste Start ueberspringt den verworfenen Rest sauber.
			return fmt.Errorf("WAL %s: Rest bis seq %s verworfen, aber der Wiederanlauf dieses Prozesses war gescheitert (%s) -- bitte neu starten (ohne AEQUITAS_WAL_REST_VERWERFEN)", pfad, soll, gescheitert)
		}
		fmt.Printf("[WAL] ⚠ WAL-Rest in %s bis seq %s bewusst verworfen (AEQUITAS_WAL_REST_VERWERFEN) -- %d Konten; diese Ueberweisungen wendet kein Wiederanlauf mehr an. Die Variable jetzt entfernen.\n", pfad, soll, offen)
		return nil
	}
	if offen == 0 {
		return fmt.Errorf("WAL %s: %s -- Knoten startet nicht. Ursache beheben und neu starten; den Rest bewusst verwerfen: AEQUITAS_WAL_REST_VERWERFEN=%s", pfad, warum, soll)
	}
	return fmt.Errorf("das WAL %s traegt nicht abgeglichene Ueberweisungen (%d Konten, bis seq %s), und %s. "+
		"So liefe der Stand ohne sie weiter, und ein spaeterer Wiederanlauf wendete sie auf den weitergelaufenen Stand an. "+
		"Ausweg: als Validator mit AEQUITAS_WAL_ENABLED=1 (ohne AEQUITAS_BEOBACHTER) starten, bis der Wiederanlauf gelingt, und sauber beenden -- "+
		"oder den Rest bewusst verwerfen: AEQUITAS_WAL_REST_VERWERFEN=%s (nur fuer genau diesen Stand; diese Ueberweisungen gehen nie in einen Block). "+
		"Die WAL-Datei in keinem Fall loeschen: die Zaehlung begaenne neu, und neue Saetze laegen unter der Untergrenze", pfad, offen, soll, warum, soll)
}

// walRestOffen zaehlt die Konten, deren hoechster Satz ueber der
// Untergrenze ueber ihrem chain_accounts.wal_seq liegt -- genau dann traegt
// das WAL einen Satz, den Absender oder Empfaenger noch nicht enthalten.
// Speicher: ein Eintrag je Konto (hoechstens walRestHoechstensKonten),
// nicht je Satz. Liest die Datei nur (wal.ReplayFile), schreibt nichts.
func (cs *ChainState) walRestOffen(path string) (offen int, kopf uint64, err error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return 0, 0, nil
	} else if err != nil {
		return 0, 0, fmt.Errorf("WAL %s nicht lesbar: %w", path, err)
	}
	if cs.db == nil {
		// Ohne Datenbank weder Untergrenze noch wal_seq: jeder Satz zaehlt.
		n, _, err := wal.ReplayFile(path, func(e wal.Entry) error {
			kopf = max(kopf, e.Seq)
			return nil
		})
		if err != nil {
			return 0, 0, err
		}
		return n, kopf, nil
	}
	floor := cs.walRecoveryFloor()
	hoechster := make(map[string]uint64)
	merken := func(konto string, seq uint64) error {
		if _, da := hoechster[konto]; !da && len(hoechster) >= walRestHoechstensKonten {
			return fmt.Errorf("mehr als %d Konten im WAL-Rest", walRestHoechstensKonten)
		}
		if seq > hoechster[konto] {
			hoechster[konto] = seq
		}
		return nil
	}
	_, _, err = wal.ReplayFile(path, func(e wal.Entry) error {
		kopf = max(kopf, e.Seq)
		if e.Seq <= floor {
			return nil
		}
		var rec walTransferRecord
		if err := json.Unmarshal(e.Payload, &rec); err != nil {
			return fmt.Errorf("Satz %d nicht lesbar: %w", e.Seq, err)
		}
		if err := merken(strings.ToLower(rec.From), e.Seq); err != nil {
			return err
		}
		return merken(strings.ToLower(rec.To), e.Seq)
	})
	if err != nil {
		return 0, 0, fmt.Errorf("WAL %s: %w", path, err)
	}
	if len(hoechster) == 0 {
		return 0, kopf, nil
	}
	liste := make([]string, 0, len(hoechster))
	for k := range hoechster {
		liste = append(liste, k)
	}
	stand := make(map[string]uint64, len(liste))
	const portion = 10_000
	for i := 0; i < len(liste); i += portion {
		teil := liste[i:min(i+portion, len(liste))]
		if err := cs.walSeqLesen(teil, stand); err != nil {
			return 0, 0, err
		}
	}
	for konto, seq := range hoechster {
		if stand[konto] < seq {
			offen++
		}
	}
	return offen, kopf, nil
}

// walSeqLesen liest wal_seq der Konten in stand; fehlende Konten bleiben 0.
func (cs *ChainState) walSeqLesen(konten []string, stand map[string]uint64) error {
	rows, err := cs.db.Query(`SELECT lower(address), COALESCE(wal_seq, 0) FROM chain_accounts WHERE lower(address) = ANY($1)`, pq.Array(konten))
	if err != nil {
		return fmt.Errorf("wal_seq nicht lesbar: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var a string
		var s int64
		if err := rows.Scan(&a, &s); err != nil {
			return fmt.Errorf("wal_seq nicht lesbar: %w", err)
		}
		stand[a] = uint64(s)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("wal_seq nicht lesbar: %w", err)
	}
	return nil
}
