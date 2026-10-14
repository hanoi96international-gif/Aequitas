package keeper

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
)

// ZERTIFIKAT IN DIE KETTE, "LETZTER CHECKPOINT" IM ZUSTAND (OFFEN 10, Stufe C,
// Baustein 6; docs/CHECKPOINT_FAHRPLAN.md).
//
// SCHLAFEND bis checkpointTxAb (heute: nie). Davor ist die Transaktionsart
// unbekannt (bekannteTxArt) -- ein Block mit ihr wird abgewiesen, und die
// StateRoot bleibt byte-gleich.
//
// Transaktion "checkpoint_zertifikat": nur Type, TxHash (= checkpointTxHash
// des Zertifikats) und Checkpoint -- jedes andere Feld leer. Hoechstens eine
// je Block.
//
// Beim Nachspielen prueft JEDER Knoten selbst (checkpointImTragendenBlock,
// Baustein 3): Komitee zur Checkpoint-Zeit, mehr als 2/3 kanonische
// Unterschriften, der Checkpoint-Block liegt in der Vergangenheit des
// tragenden Blocks, Zeiten relativ zum tragenden Block. Dazu: der bisherige
// letzte Checkpoint liegt in der Vergangenheit des neuen.
//
//   - Verstoss gegen Form oder Zertifikat: Block ungueltig.
//   - Nicht juenger als der letzte, oder nicht dessen Nachfolger: haengt an
//     der Nachspiel-Reihenfolge (zwei parallele Bloecke mit Zertifikaten) --
//     die Transaktion wird uebersprungen (ErrZustandLehntAb), der Block
//     bleibt gueltig.
//   - Zu alt fuer den tragenden Block (mehr als checkpointHoechstensAlt):
//     uebersprungen -- ein Erzeuger, der sie schon angenommen hat, bliebe
//     sonst mit ihr stehen.
//   - Nicht pruefbar (Luecke, Lesefehler): Block zurueckgestellt (der
//     bestehende Weg ueber die Wiederholung, softRetry), nie angenommen.
//
// STICHTAG ERST SETZEN, WENN DIESE SPERREN GELOEST SIND (Pruefung von
// Baustein 6; docs/CHECKPOINT_FAHRPLAN.md):
//   1. Komitee konsensfest (HOCH): checkpointImTragendenBlock liest den
//      aktuellen Register-Stand und die lokale Liste, nicht den Stand des
//      tragenden Blocks -- verschiedene Staende spalten.
//   2. Zeitfrist (MITTEL): die Gesamtfrist der Suche macht "nicht pruefbar"
//      vom Knoten abhaengig; nach der Wiederholungsfrist verwirft ein
//      langsamer Knoten, was ein schneller annimmt -- dort nachladen statt
//      verwerfen.
//   3. Erzeuger wendet an (MITTEL): der Leiter setzt den Wert schon beim
//      Einreichen. Wird die Transaktion beim Nachspielen uebersprungen,
//      weicht sein Wert ab (StateRoot-Abweichung, ein Resync uebernaehme
//      ihn) -- der Erzeuger muss Checkpoint-Transaktionen erst beim Bauen
//      des Blocks mit dessen Zeit anwenden.
//
// Der letzte Checkpoint steht in chain_config (letzterCheckpointSchluessel)
// und geht -- nur wenn gesetzt -- in die StateRoot ein (stateRootLocked);
// Rueckrollen, Snapshot und Resync tragen ihn mit.

const (
	checkpointTxArt             = "checkpoint_zertifikat"
	letzterCheckpointSchluessel = "letzter_checkpoint"
)

// checkpointTxAbUnix: Stichtag. math.MaxInt64 = aus.
const checkpointTxAbUnix int64 = math.MaxInt64

// checkpointTxOverride: nur Tests.
var checkpointTxOverride atomic.Int64

func checkpointTxAb() int64 {
	if v := checkpointTxOverride.Load(); v != 0 {
		return v
	}
	return checkpointTxAbUnix
}

func checkpointTxAktiv(t int64) bool { return t >= checkpointTxAb() }

// checkpointTxHash: der TxHash einer Checkpoint-Transaktion -- aus dem
// Inhalt, damit derselbe Checkpoint nicht unter zwei Hashes kommt.
func checkpointTxHash(z CheckpointZertifikat) string {
	b, _ := json.Marshal(z)
	h := sha256.Sum256(append([]byte("aequitas-checkpoint-tx|"), b...))
	return "0x" + hex.EncodeToString(h[:])
}

func neueCheckpointTx(z CheckpointZertifikat) Transaction {
	k := zertifikatKopie(&z)
	return Transaction{Type: checkpointTxArt, TxHash: checkpointTxHash(*k), Checkpoint: k}
}

var leereTxJSON, _ = json.Marshal(Transaction{})

// checkpointTxForm: Type, TxHash und Checkpoint gesetzt, alles andere leer.
func checkpointTxForm(tx *Transaction) error {
	if tx.Type != checkpointTxArt || tx.Checkpoint == nil {
		return fmt.Errorf("checkpoint_zertifikat: ohne Zertifikat")
	}
	if tx.TxHash != checkpointTxHash(*tx.Checkpoint) {
		return fmt.Errorf("checkpoint_zertifikat: TxHash passt nicht zum Inhalt")
	}
	rest := *tx
	rest.Type, rest.TxHash, rest.Checkpoint = "", "", nil
	b, err := json.Marshal(rest)
	if err != nil || string(b) != string(leereTxJSON) {
		return fmt.Errorf("checkpoint_zertifikat: weitere Felder gesetzt")
	}
	return nil
}

// letzterCheckpointWert/letzterCheckpointLesen: "zeit:hoehe:hash" in
// chain_config. "" = noch keiner.
func letzterCheckpointWert(c KettenCheckpoint) string {
	return fmt.Sprintf("%d:%d:%s", c.Zeit, c.Hoehe, c.Hash)
}

func letzterCheckpointLesen(wert string) (*KettenCheckpoint, error) {
	if wert == "" {
		return nil, nil
	}
	teile := strings.Split(wert, ":")
	if len(teile) != 3 {
		return nil, fmt.Errorf("letzter Checkpoint %q unlesbar", wert)
	}
	zeit, err1 := strconv.ParseInt(teile[0], 10, 64)
	hoehe, err2 := strconv.ParseInt(teile[1], 10, 64)
	if err1 != nil || err2 != nil || zeit <= 0 || hoehe <= 0 || !istKleinHex(teile[2], 64) {
		return nil, fmt.Errorf("letzter Checkpoint %q unlesbar", wert)
	}
	if letzterCheckpointWert(KettenCheckpoint{Zeit: zeit, Hoehe: hoehe, Hash: teile[2]}) != wert {
		return nil, fmt.Errorf("letzter Checkpoint %q nicht kanonisch", wert)
	}
	return &KettenCheckpoint{Zeit: zeit, Hoehe: hoehe, Hash: teile[2]}, nil
}

// applyCheckpointTxLocked: Nachspielen einer Checkpoint-Transaktion im
// Block mit den Eltern tragenderEltern und der Zeit tragenderZeit. lies:
// Koepfe aus derselben Transaktionssicht wie das Nachspielen.
func (dag *BlockDAG) applyCheckpointTxLocked(ctx context.Context, tx *Transaction, tragenderEltern []string, tragenderZeit int64, lies kopfLeser) error {
	if !checkpointTxAktiv(tragenderZeit) {
		return fmt.Errorf("checkpoint_zertifikat vor dem Stichtag (Block %d, ab %d)", tragenderZeit, checkpointTxAb())
	}
	if err := checkpointTxForm(tx); err != nil {
		return err
	}
	cs := dag.state
	if cs == nil || cs.db == nil {
		return fmt.Errorf("%w: keine Datenbank", errCheckpointNichtPruefbar)
	}
	roh, err := letzterCheckpointRoh(ctx, cs)
	if err != nil {
		return fmt.Errorf("%w: %v", errCheckpointNichtPruefbar, err)
	}
	letzter, err := letzterCheckpointLesen(roh)
	if err != nil {
		return fmt.Errorf("%w: %v", errCheckpointNichtPruefbar, err)
	}
	z := *tx.Checkpoint
	// Zu alt fuer diesen Block: ueberspringen, nicht ablehnen -- sonst
	// blieb ein Erzeuger, der die Transaktion schon angenommen hat, mit ihr
	// stehen (blockZeitPraefix: weglassen liesse ihn abweichen, einbinden
	// liesse jeden anderen den Block abweisen).
	if tragenderZeit-z.Zeit > checkpointHoechstensAlt {
		return checkpointZustand("Checkpoint %d s vor dem tragenden Block (hoechstens %d)", tragenderZeit-z.Zeit, checkpointHoechstensAlt)
	}
	// Von der Reihenfolge abhaengig: ueberspringen, nicht ablehnen.
	if letzter != nil && z.Zeit <= letzter.Zeit {
		return checkpointZustand("Checkpoint (Zeit %d) nicht nach dem letzten (Zeit %d)", z.Zeit, letzter.Zeit)
	}
	if err := dag.checkpointImTragendenBlock(z, nil, tragenderEltern, tragenderZeit, lies); err != nil {
		return err
	}
	if letzter != nil {
		_, gefunden, vollstaendig, err := vorfahrSuchen(lies, []string{z.Hash}, letzter.Hash, letzter.Zeit)
		switch {
		case errors.Is(err, errVorfahrZuGross):
			// Der letzte liegt zu weit zurueck (etwa nach langem Ausfall der
			// Leitung): die Zeit-Monotonie oben genuegt -- sonst rueckte der
			// Wert nie mehr vor, jeder spaetere Abstand waere noch groesser
			// (Pruefung von Baustein 6, F4).
		case err != nil:
			return fmt.Errorf("%w: Vorfahren nicht lesbar: %v", errCheckpointNichtPruefbar, err)
		case err == nil && !gefunden && !vollstaendig:
			return fmt.Errorf("%w: Vergangenheit unvollstaendig", errCheckpointNichtPruefbar)
		case !gefunden:
			return checkpointZustand("letzter Checkpoint %s nicht in der Vergangenheit von %s", kurzAdresse(letzter.Hash), kurzAdresse(z.Hash))
		}
	}
	return cs.setConfigValueCtx(ctx, letzterCheckpointSchluessel, letzterCheckpointWert(z.KettenCheckpoint))
}

// checkpointTxEinreichen: der Leiter traegt ein angenommenes Zertifikat als
// Transaktion ein (wie eine Bindung: im Zustand gesetzt und in pending_txs,
// in einer Datenbank-Transaktion). Vor dem Stichtag nichts.
func (dag *BlockDAG) checkpointTxEinreichen(z CheckpointZertifikat, jetzt int64) error {
	if !checkpointTxAktiv(jetzt) {
		return nil
	}
	cs := dag.state
	if cs == nil || cs.db == nil {
		return errors.New("keine Datenbank")
	}
	tx := neueCheckpointTx(z)
	if err := cs.annahmeBeginnenLeiter(); err != nil {
		return err
	}
	defer cs.annahmeEnde()
	return cs.runAtomicWithOutbox(nil, false, func(ctx context.Context) (Transaction, error) {
		roh, err := letzterCheckpointRoh(ctx, cs)
		if err != nil {
			return Transaction{}, err
		}
		letzter, err := letzterCheckpointLesen(roh)
		if err != nil {
			return Transaction{}, err
		}
		if letzter != nil && z.Zeit <= letzter.Zeit {
			return Transaction{}, checkpointZustand("Checkpoint (Zeit %d) nicht nach dem letzten (Zeit %d)", z.Zeit, letzter.Zeit)
		}
		if err := cs.setConfigValueCtx(ctx, letzterCheckpointSchluessel, letzterCheckpointWert(z.KettenCheckpoint)); err != nil {
			return Transaction{}, err
		}
		return tx, nil
	})
}

// snapshotKonfigUebernehmen: vor dem Stichtag wird ein letzter Checkpoint
// aus einem Snapshot nicht uebernommen (Pruefung von Baustein 6, F8).
func snapshotKonfigUebernehmen(key string, jetzt int64) bool {
	return key != letzterCheckpointSchluessel || checkpointTxAktiv(jetzt)
}

// pruefeSnapshotKonfig: ein Wert aus dem Snapshot, den die StateRoot liest,
// muss lesbar sein -- sonst verwirft der Import den Snapshot.
func pruefeSnapshotKonfig(key, wert string) error {
	if key == letzterCheckpointSchluessel {
		if _, err := letzterCheckpointLesen(wert); err != nil {
			return err
		}
	}
	return nil
}

// checkpointZustand: haengt an der Nachspiel-Reihenfolge -- die
// Transaktion wird uebersprungen, der Block bleibt gueltig.
func checkpointZustand(format string, args ...interface{}) error {
	return fmt.Errorf("checkpoint_zertifikat: "+format+": %w", append(args, ErrZustandLehntAb)...)
}

// checkpointReplayLeser: Koepfe fuer das Nachspielen, aus der Transaktion
// des Nachspielens. Tests ersetzen ihn.
var checkpointReplayLeser = func(q sqlExecutor) kopfLeser { return kopfLeserDB(q) }

// letzterCheckpointRoh: der Wert aus chain_config in der Sicht von ctx --
// mit Fehler (getConfigValueCtx verschluckt ihn; dann fehlte die Pruefung
// der Reihenfolge, Pruefung von Baustein 6, F7). "" = noch keiner.
func letzterCheckpointRoh(ctx context.Context, cs *ChainState) (string, error) {
	var v string
	err := cs.dbExecCtx(ctx).QueryRow(`SELECT value FROM chain_config WHERE key = $1`, letzterCheckpointSchluessel).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// checkpointFeldErlaubt: das Feld Checkpoint nur an checkpoint_zertifikat.
func checkpointFeldErlaubt(tx *Transaction) error {
	if tx.Checkpoint != nil && tx.Type != checkpointTxArt {
		return fmt.Errorf("%s traegt ein Checkpoint-Zertifikat", tx.Type)
	}
	return nil
}

// checkpointTxGrenze: wie viele Transaktionen vom Anfang ein Block tragen
// kann, ohne ein zweites checkpoint_zertifikat.
func checkpointTxGrenze(txs []Transaction) int {
	n := 0
	for i := range txs {
		if txs[i].Type == checkpointTxArt {
			n++
			if n > 1 {
				return i
			}
		}
	}
	return len(txs)
}
