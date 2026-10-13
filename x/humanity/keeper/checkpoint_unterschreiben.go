package keeper

import (
	"database/sql"
	"errors"
	"fmt"
)

// WANN EIN MITGLIED EINEN CHECKPOINT UNTERSCHREIBT (OFFEN 10, Stufe C,
// Baustein 5, Mitglieds-Seite; docs/CHECKPOINT_FAHRPLAN.md).
//
// Der Leiter schlaegt je Slot (checkpointSlotLaenge) einen Block vor; jedes
// Komiteemitglied entscheidet hier selbst, ob es ihn unterschreibt:
//   - der Vorschlag ist gueltig, ab erzeugerSchnittAb, sein Block
//     mindestens zeitstempelRueckToleranz und hoechstens
//     checkpointHoechstensAlt alt;
//   - dieser Knoten ist zur Blockzeit Mitglied, und der Komitee-Hash des
//     Vorschlags ist seiner -- er unterschreibt nie fuer eine Menge, die er
//     anders sieht;
//   - der Block ist hier nachgespielt, Hoehe und Zeit passen;
//   - DOPPELSIGNIEREN AUSGESCHLOSSEN: je Slot hoechstens ein Block, und nie
//     ein Slot vor einem schon unterschriebenen. Die Entscheidung steht
//     dauerhaft in checkpoint_unterschriften, BEVOR die Unterschrift
//     herausgeht -- ein Neustart kann keine zweite fuer denselben Slot
//     erzeugen. Derselbe Vorschlag nochmal: dieselbe Unterschrift
//     (idempotent, der Leiter darf erneut fragen).
//
// Fail-closed: ohne Datenbank, ohne Schluessel, bei jedem Lesefehler keine
// Unterschrift. Schlafend: nirgends angeschlossen.

const checkpointSlotLaenge int64 = 60

func checkpointSlot(zeit int64) int64 { return zeit / checkpointSlotLaenge }

// checkpointUnterschriftTabelle: der dauerhafte Merker je Slot.
func checkpointUnterschriftTabelle(q sqlExecutor) error {
	_, err := q.Exec(`CREATE TABLE IF NOT EXISTS checkpoint_unterschriften (
		slot  BIGINT PRIMARY KEY,
		hash  TEXT NOT NULL,
		hoehe BIGINT NOT NULL,
		zeit  BIGINT NOT NULL
	)`)
	return err
}

// checkpointMitunterschreiben: die eigene Unterschrift fuer den Vorschlag,
// oder ein Grund, warum nicht.
func (dag *BlockDAG) checkpointMitunterschreiben(v KettenCheckpoint, jetzt int64) (CheckpointUnterschrift, error) {
	nein := func(f string, a ...interface{}) (CheckpointUnterschrift, error) {
		return CheckpointUnterschrift{}, fmt.Errorf(f, a...)
	}
	if err := v.gueltig(); err != nil {
		return nein("Vorschlag ungueltig: %v", err)
	}
	if !erzeugerSchnittAktiv(v.Zeit) {
		return nein("vor erzeugerSchnittAb")
	}
	if alter := jetzt - v.Zeit; alter < zeitstempelRueckToleranz || alter > checkpointHoechstensAlt {
		return nein("Block %d s alt -- unterschrieben wird zwischen %d und %d s", alter, zeitstempelRueckToleranz, checkpointHoechstensAlt)
	}
	if dag.signingKey == nil || dag.selfProposer == "" {
		return nein("kein Signierschluessel")
	}
	if dag.state == nil {
		return nein("kein Zustand")
	}
	komitee, err := checkpointKomitee(dag.state.erzeugerRegister.Load(), v.Zeit, dag.festeErzeugerListe())
	if err != nil {
		return nein("Komitee nicht bestimmbar: %v", err)
	}
	if !komitee[dag.selfProposer] {
		return nein("nicht im Komitee zur Blockzeit")
	}
	if len(komitee) < checkpointMindestKomitee {
		return nein("Komitee mit %d Mitgliedern bestaetigt nichts", len(komitee))
	}
	if v.Komitee != komiteeHash(komitee) {
		return nein("Komitee-Hash des Vorschlags ist nicht meiner")
	}
	hoehe, zeit, da := dag.nachgespielterBlockKopf(v.Hash)
	if !da {
		return nein("Block %s hier nicht nachgespielt", kurzAdresse(v.Hash))
	}
	if hoehe != v.Hoehe || zeit != v.Zeit {
		return nein("Vorschlag (Hoehe %d, Zeit %d) passt nicht zum Block (Hoehe %d, Zeit %d)", v.Hoehe, v.Zeit, hoehe, zeit)
	}
	if dag.state.db == nil {
		return nein("keine Datenbank -- ohne dauerhaften Merker keine Unterschrift")
	}
	if err := dag.checkpointSlotMerken(v); err != nil {
		return CheckpointUnterschrift{}, err
	}
	return checkpointUnterschreiben(v, dag.signingKey)
}

var errCheckpointSlotBelegt = errors.New("Slot schon fuer einen anderen Block unterschrieben")

// checkpointSlotMerken: den Slot dauerhaft fuer diesen Block belegen. Den
// selben Slot schuetzt schon der Primaerschluessel (die zweite Einfuegung
// scheitert); die Sperre der Tabelle schuetzt die Reihenfolge der Slots
// (zwei gleichzeitige Anfragen fuer verschiedene Slots sehen sonst beide
// keinen juengeren). nil: unterschreiben erlaubt.
func (dag *BlockDAG) checkpointSlotMerken(v KettenCheckpoint) error {
	db := dag.state.db
	if err := checkpointUnterschriftTabelle(db); err != nil {
		return fmt.Errorf("Merker nicht anlegbar: %w", err)
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("Merker nicht lesbar: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`LOCK TABLE checkpoint_unterschriften IN EXCLUSIVE MODE`); err != nil {
		return fmt.Errorf("Merker nicht sperrbar: %w", err)
	}
	slot := checkpointSlot(v.Zeit)
	var hash string
	switch err := tx.QueryRow(`SELECT hash FROM checkpoint_unterschriften WHERE slot = $1`, slot).Scan(&hash); {
	case err == nil:
		if hash != v.Hash {
			return fmt.Errorf("%w (Slot %d: %s)", errCheckpointSlotBelegt, slot, kurzAdresse(hash))
		}
		return nil // derselbe Vorschlag: dieselbe Unterschrift
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("Merker nicht lesbar: %w", err)
	}
	var juengster sql.NullInt64
	if err := tx.QueryRow(`SELECT MAX(slot) FROM checkpoint_unterschriften`).Scan(&juengster); err != nil {
		return fmt.Errorf("Merker nicht lesbar: %w", err)
	}
	if juengster.Valid && slot < juengster.Int64 {
		return fmt.Errorf("Slot %d liegt vor dem schon unterschriebenen Slot %d", slot, juengster.Int64)
	}
	if _, err := tx.Exec(`INSERT INTO checkpoint_unterschriften (slot, hash, hoehe, zeit) VALUES ($1, $2, $3, $4)`,
		slot, v.Hash, v.Hoehe, v.Zeit); err != nil {
		return fmt.Errorf("Merker nicht schreibbar: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("Merker nicht schreibbar: %w", err)
	}
	return nil
}
