package keeper

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/wal"
	"github.com/lib/pq"
)

// WAL nach einem Rollenwechsel (Pruefung von #322, INFO-15).
//
// Ein Beobachter liest das WAL nicht ein (initWALIfEnabled), die Datei
// bleibt liegen. Startete ein abgestuerzter Validator mit ungeflushten
// Saetzen erst als Beobachter, spielte er fremde Bloecke nach -- ohne diese
// Ueberweisungen, wal_seq der Konten bleibt stehen. Lief er danach wieder als
// Validator, wandte recoverFromWAL die alten Saetze auf den weitergelaufenen
// Stand an, ohne Deckungspruefung (PoC: Kontostand -42,042 und eine Zeile im
// Ausgang, die jeder andere Knoten beim Nachspielen abweist).
//
// Zwei Riegel:
//   - PruefeBeobachterWAL: ein Beobachter mit nicht abgeglichenen Saetzen
//     startet nicht (main beendet sich). Erst als Validator wiederanlaufen
//     lassen -- oder den Rest bewusst verwerfen
//     (AEQUITAS_BEOBACHTER_WAL_VERWERFEN=1 setzt die Untergrenze des
//     Wiederanlaufs auf das Ende der Datei, wie nach einem Zustandsersatz).
//   - walSatzParken: recoverFromWAL wendet keinen Satz an, der den Absender
//     ins Minus fuehrte; er landet in wal_geparkt fuer den Betreiber.

// beobachterWALHoechstensKonten begrenzt, wie viele Konten die Pruefung
// beim Start sammelt (ein Eintrag je Konto). Mehr: fail-closed, als waere
// der Rest offen.
var beobachterWALHoechstensKonten = 1_000_000 // var: Tests senken sie

// walPfad: wie initWALIfEnabled und markWALSupersededByStateReplacement.
func walPfad() string {
	if p := os.Getenv("AEQUITAS_WAL_PATH"); p != "" {
		return p
	}
	return "aequitas_transfers.wal"
}

// PruefeBeobachterWAL: nur auf einem Beobachter, unabhaengig von
// AEQUITAS_WAL_ENABLED (die Datei ueberlebt den Schalter). Fehler heisst:
// nicht starten.
func (cs *ChainState) PruefeBeobachterWAL() error {
	if !beobachterModus() {
		return nil
	}
	offen, err := cs.walRestOffen(walPfad())
	if err == nil && offen == 0 {
		return nil
	}
	grund := fmt.Sprintf("%d Konten mit nicht abgeglichenen Saetzen", offen)
	if err != nil {
		grund = err.Error()
	}
	if strings.TrimSpace(os.Getenv("AEQUITAS_BEOBACHTER_WAL_VERWERFEN")) == "1" && cs.db != nil {
		cs.markWALSupersededByStateReplacement()
		if rest, err2 := cs.walRestOffen(walPfad()); err2 != nil || rest != 0 {
			return fmt.Errorf("Beobachter: WAL-Rest in %s (%s) liess sich nicht verwerfen -- Knoten startet nicht", walPfad(), grund)
		}
		fmt.Printf("[WAL] ⚠ Beobachter: WAL-Rest (%s) in %s bewusst verworfen (AEQUITAS_BEOBACHTER_WAL_VERWERFEN=1) -- diese Ueberweisungen wendet kein Wiederanlauf mehr an\n", grund, walPfad())
		return nil
	}
	return fmt.Errorf("Beobachter: das WAL %s traegt nicht abgeglichene Ueberweisungen (%s). "+
		"Als Beobachter spielte dieser Knoten fremde Bloecke ohne sie nach, und ein spaeterer Wiederanlauf als Validator wendete sie auf den weitergelaufenen Stand an. "+
		"Erst als Validator wiederanlaufen lassen (AEQUITAS_BEOBACHTER aus, AEQUITAS_WAL_ENABLED=1) und sauber beenden -- danach meldet dieser Start keinen Rest mehr -- "+
		"oder den Rest bewusst verwerfen: AEQUITAS_BEOBACHTER_WAL_VERWERFEN=1 (diese Ueberweisungen gehen dann nie in einen Block)", walPfad(), grund)
}

// walRestOffen zaehlt die Konten, deren hoechster Satz ueber der
// Untergrenze ueber ihrem chain_accounts.wal_seq liegt -- genau dann traegt
// das WAL einen Satz, den Absender oder Empfaenger noch nicht enthalten.
// Speicher: ein Eintrag je Konto (hoechstens beobachterWALHoechstensKonten),
// nicht je Satz. Liest die Datei nur (wal.ReplayFile), schreibt nichts.
func (cs *ChainState) walRestOffen(path string) (int, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return 0, nil
	} else if err != nil {
		return 0, fmt.Errorf("WAL %s nicht lesbar: %w", path, err)
	}
	if cs.db == nil {
		// Ohne Datenbank weder Untergrenze noch wal_seq: jeder Satz zaehlt.
		n, _, err := wal.ReplayFile(path, func(wal.Entry) error { return nil })
		if err != nil {
			return 0, err
		}
		return n, nil
	}
	floor := cs.walRecoveryFloor()
	hoechster := make(map[string]uint64)
	merken := func(konto string, seq uint64) error {
		if _, da := hoechster[konto]; !da && len(hoechster) >= beobachterWALHoechstensKonten {
			return fmt.Errorf("mehr als %d Konten im WAL-Rest", beobachterWALHoechstensKonten)
		}
		if seq > hoechster[konto] {
			hoechster[konto] = seq
		}
		return nil
	}
	_, _, err := wal.ReplayFile(path, func(e wal.Entry) error {
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
		return 0, fmt.Errorf("WAL %s: %w", path, err)
	}
	if len(hoechster) == 0 {
		return 0, nil
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
			return 0, err
		}
	}
	offen := 0
	for konto, seq := range hoechster {
		if stand[konto] < seq {
			offen++
		}
	}
	return offen, nil
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

// walSatzParken vermerkt einen Satz, den recoverFromWAL nicht anwendet.
// Wiederholbar (derselbe Satz beim naechsten Start: ON CONFLICT). Ein
// Schreibfehler laesst den Wiederanlauf scheitern -- der Schnellpfad bleibt
// dann aus (initWALIfEnabled), statt einen Satz still zu verlieren.
func (cs *ChainState) walSatzParken(seq uint64, rec walTransferRecord, payload []byte, grund string) error {
	if cs.db == nil {
		return fmt.Errorf("WAL-Satz %d nicht geparkt: keine Datenbank", seq)
	}
	if _, err := cs.db.Exec(`INSERT INTO wal_geparkt (seq, von, an, betrag, gebuehr, tx_hash, satz_json, grund, geparkt_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT (seq) DO NOTHING`,
		int64(seq), strings.ToLower(rec.From), strings.ToLower(rec.To), rec.Amount, rec.Gebuehr, rec.TxHash, string(payload), grund, time.Now().Unix()); err != nil {
		return fmt.Errorf("WAL-Satz %d nicht geparkt: %w", seq, err)
	}
	fmt.Printf("[WAL] ✗ Satz %d (%s -> %s, %.6f + %.6f Gebuehr, %s) NICHT angewandt und in wal_geparkt abgelegt: %s\n",
		seq, rec.From, rec.To, rec.Amount, rec.Gebuehr, rec.TxHash, grund)
	if cs.BootstrapDegradedReason() == "" {
		cs.SetBootstrapDegraded(fmt.Sprintf("wal_geparkt: WAL-Satz %d beim Wiederanlauf nicht angewandt (%s) -- bitte pruefen", seq, grund))
	}
	return nil
}
