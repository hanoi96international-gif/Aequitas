package keeper

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// KOEPFE IM SNAPSHOT (OFFEN 10, Stufe C, Baustein 8;
// docs/CHECKPOINT_FAHRPLAN.md).
//
// Ein frisch per Snapshot aufgesetzter Knoten hat die Bloecke vor dem
// Snapshot nicht. Ein Checkpoint kurz danach liegt aber bis zu
// checkpointHoechstensAlt vor seinem tragenden Block -- ohne die Koepfe davor
// waere er "nicht pruefbar", und der Knoten bliebe stehen. Ab dem Stichtag
// der Checkpoint-Transaktionen traegt der Snapshot darum die Koepfe (Hash,
// Hoehe, Zeit, Eltern) der letzten snapshotKopfFenster Sekunden, hoechstens
// snapshotKopfGrenze. Der Import legt sie in kopf_vorrat ab; die
// Vorfahren-Suche (kopfLeserDB) liest chain_blocks UND kopf_vorrat.
//
// Vertrauen: wie der ganze Snapshot nur von einem vertrauten Unterzeichner
// (die Unterschrift deckt die Koepfe mit). Begrenzt: Zahl, Elternzahl,
// Laenge der Hashes. Vor dem Stichtag weder exportiert noch importiert --
// bisherige Snapshots bleiben byte-gleich.

const (
	snapshotKopfFenster = 2 * checkpointHoechstensAlt
	snapshotKopfGrenze  = 5000
	snapshotHashLaenge  = 128
)

// SnapshotKopf: ein Blockkopf fuer die Vorfahren-Suche.
type SnapshotKopf struct {
	Hash   string   `json:"hash"`
	Hoehe  int64    `json:"hoehe"`
	Zeit   int64    `json:"zeit"`
	Eltern []string `json:"eltern"`
}

// snapshotKoepfeExport: die Koepfe der letzten snapshotKopfFenster Sekunden
// vor jetzt (nur ab dem Stichtag).
func (cs *ChainState) snapshotKoepfeExport(jetzt int64) ([]SnapshotKopf, error) {
	if !checkpointTxAktiv(jetzt) || cs.db == nil {
		return nil, nil
	}
	rows, err := cs.db.Query(`SELECT hash, height, timestamp, parent_hashes FROM chain_blocks
		WHERE timestamp >= $1 ORDER BY timestamp DESC, hash LIMIT $2`, jetzt-snapshotKopfFenster, snapshotKopfGrenze)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SnapshotKopf
	for rows.Next() {
		var k SnapshotKopf
		var eltern string
		if err := rows.Scan(&k.Hash, &k.Hoehe, &k.Zeit, &eltern); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(eltern), &k.Eltern); err != nil {
			return nil, fmt.Errorf("Eltern von %s: %w", kurzAdresse(k.Hash), err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// pruefeSnapshotKoepfe: Form und Grenzen.
func pruefeSnapshotKoepfe(koepfe []SnapshotKopf) error {
	if len(koepfe) > snapshotKopfGrenze {
		return fmt.Errorf("%d Koepfe (hoechstens %d)", len(koepfe), snapshotKopfGrenze)
	}
	for _, k := range koepfe {
		if k.Hash == "" || len(k.Hash) > snapshotHashLaenge || k.Hoehe < 0 || k.Zeit <= 0 {
			return fmt.Errorf("Kopf %q ungueltig", kurzAdresse(k.Hash))
		}
		if len(k.Eltern) > peerElternGrenze {
			return fmt.Errorf("Kopf %s mit %d Eltern", kurzAdresse(k.Hash), len(k.Eltern))
		}
		for _, e := range k.Eltern {
			if e == "" || len(e) > snapshotHashLaenge {
				return fmt.Errorf("Kopf %s: Elternteil ungueltig", kurzAdresse(k.Hash))
			}
		}
	}
	return nil
}

// snapshotKoepfeImportieren: in kopf_vorrat (vorhandene bleiben). Vor dem
// Stichtag nichts.
func snapshotKoepfeImportieren(tx *sql.Tx, koepfe []SnapshotKopf, jetzt int64) error {
	if len(koepfe) == 0 || !checkpointTxAktiv(jetzt) {
		return nil
	}
	if err := pruefeSnapshotKoepfe(koepfe); err != nil {
		return err
	}
	for _, k := range koepfe {
		eltern, _ := json.Marshal(k.Eltern)
		if _, err := tx.Exec(`INSERT INTO kopf_vorrat (hash, height, timestamp, parent_hashes) VALUES ($1, $2, $3, $4)
			ON CONFLICT (hash) DO NOTHING`, k.Hash, k.Hoehe, k.Zeit, string(eltern)); err != nil {
			return err
		}
	}
	return nil
}
