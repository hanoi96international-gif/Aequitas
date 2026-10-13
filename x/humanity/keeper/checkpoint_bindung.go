package keeper

import (
	"errors"
	"fmt"
	"strings"
)

// CHECKPOINT AN KETTE UND KOMITEE BINDEN (OFFEN 10, Stufe C, Baustein 2).
//
// Die Pruefung des ersten Bausteins (#344, HOCH) verlangte vor jedem
// Anschluss:
//   - Das Komitee zur Checkpoint-Zeit bestimmt jeder Knoten gleich -- aus
//     dem Register (erzeugerStand), nicht aus einer Liste, die er selbst
//     fuehrt. checkpointKomitee nimmt die Rangliste der Epoche aus dem Stand
//     und davon die ersten targetCommitteeSize, die zur Zeit t ein
//     Erzeugerfenster haben -- wie komiteeAusRegister, aber fuer jede Zeit
//     und ohne Nebenwirkungen. Ein Knoten mit anderem Stand (anderer
//     geschlossener Liste, anderes Register) kommt auf einen anderen
//     Komitee-Hash und lehnt ab: kein stilles Auseinanderlaufen.
//   - Die Zeit waehlt nicht der Einreicher: Hash, Hoehe und Zeit muessen zu
//     einem Block gehoeren, den dieser Knoten nachgespielt hat. Wer eine
//     Epoche mit kleinem Komitee sucht, muesste dort einen echten Block
//     finden -- und das Zertifikat muss frisch sein (hoechstens
//     checkpointHoechstensAlt vor der Bezugszeit, nicht in der Zukunft) und
//     streng nach dem letzten Checkpoint liegen (Hoehe und Zeit).
//
// Schlafend: nirgends angeschlossen. Ein Lesefehler oder kein Stand:
// Ablehnung (fail-closed). "Ablehnung statt stiller Spaltung" gilt nur,
// solange das Zertifikat keine Konsensregel ist -- als Transaktionsart in
// Bloecken waere jede lokale Abweichung (Stand, Replay-Status, Uhr) ein
// Split. Vor dem Anschluss darum (Pruefung dieses Bausteins, HOCH):
// Bezugszeit = Zeit des tragenden Blocks, "nachgespielt" ersetzen durch
// "liegt in der Vergangenheit des tragenden Blocks", Komitee-Stand aus der
// Vergangenheit des tragenden Blocks, konsensfeste Liste im geschlossenen
// Betrieb, eine Regel, welche Bloecke ehrliche Mitglieder unterschreiben,
// und ein dauerhafter Speicher des letzten Checkpoints.

// checkpointHoechstensAlt: so weit darf der Block eines Checkpoints vor der
// Bezugszeit (Einreichung, spaeter: Zeit des tragenden Blocks) liegen.
const checkpointHoechstensAlt int64 = 600

// checkpointKomitee: das Komitee zur Zeit t aus dem Stand. fest: die
// geschlossene Liste (AUTHORIZED_VALIDATORS) -- nur deren Schluessel zaehlen,
// NICHT der eigene Schluessel, den der Stand zusaetzlich liest
// (erzeugerFestListe): sonst haette jeder Knoten ausserhalb der Liste ein
// eigenes Komitee. nil = offen.
func checkpointKomitee(st *erzeugerStand, t int64, fest map[string]bool) (map[string]bool, error) {
	if st == nil {
		return nil, errors.New("kein Register-Stand")
	}
	if st.fehler != nil {
		return nil, fmt.Errorf("Register-Stand fehlerhaft: %w", st.fehler)
	}
	epoche := t / epochLength
	rang, ok := st.komiteeRang[epoche]
	if !ok {
		rang = komiteeRangBerechnen(st, epoche)
	}
	komitee := make(map[string]bool, targetCommitteeSize)
	for _, addr := range rang {
		if len(komitee) >= targetCommitteeSize {
			break
		}
		if fest != nil && !fest[addr] {
			continue
		}
		if st.erzeugerFenster(addr, t) != "" {
			komitee[strings.ToLower(addr)] = true
		}
	}
	return komitee, nil
}

// checkpointGegenKette: nil, wenn das Zertifikat zu einem nachgespielten
// Block dieses Knotens gehoert, frisch ist (Bezugszeit jetzt), nach letzter
// liegt (nil: noch keiner) und vom Komitee zur Blockzeit bestaetigt ist.
// Aufrufer haelt dag.mu NICHT.
func (dag *BlockDAG) checkpointGegenKette(z CheckpointZertifikat, letzter *KettenCheckpoint, jetzt int64) error {
	// Billig zuerst: Form, Zahl der Unterschriften, Stichtag, Zeiten --
	// vor jeder Sperre, jedem Nachschlag und der Komitee-Berechnung.
	if err := z.gueltig(); err != nil {
		return err
	}
	if n := len(z.Unterschriften); n < checkpointQuorum(checkpointMindestKomitee) || n > targetCommitteeSize {
		return fmt.Errorf("%d Unterschriften -- ausserhalb von [%d, %d]", n, checkpointQuorum(checkpointMindestKomitee), targetCommitteeSize)
	}
	if !erzeugerSchnittAktiv(z.Zeit) {
		return fmt.Errorf("Checkpoint vor erzeugerSchnittAb -- das Komitee kommt dann nicht aus dem Register")
	}
	if z.Zeit > jetzt+zeitstempelZukunftToleranz {
		return fmt.Errorf("Checkpoint-Zeit %d liegt in der Zukunft (jetzt %d)", z.Zeit, jetzt)
	}
	if jetzt-z.Zeit > checkpointHoechstensAlt {
		return fmt.Errorf("Checkpoint %d s alt (hoechstens %d)", jetzt-z.Zeit, checkpointHoechstensAlt)
	}
	// Monotonie nur ueber die Zeit: die Hoehe ist nicht konsensfest und
	// von einem Erzeuger aufblaehbar -- ein zertifizierter Block mit
	// ueberhoehter Hoehe sperrte sonst alle weiteren Checkpoints.
	if letzter != nil && z.Zeit <= letzter.Zeit {
		return fmt.Errorf("Checkpoint (Zeit %d) nicht nach dem letzten (Zeit %d)", z.Zeit, letzter.Zeit)
	}
	hoehe, zeit, da := dag.nachgespielterBlockKopf(z.Hash)
	if !da {
		return fmt.Errorf("Block %s hier nicht nachgespielt", kurzAdresse(z.Hash))
	}
	if hoehe != z.Hoehe || zeit != z.Zeit {
		return fmt.Errorf("Checkpoint (Hoehe %d, Zeit %d) passt nicht zum Block (Hoehe %d, Zeit %d)",
			z.Hoehe, z.Zeit, hoehe, zeit)
	}
	if dag.state == nil {
		return errors.New("kein Zustand")
	}
	komitee, err := checkpointKomitee(dag.state.erzeugerRegister.Load(), z.Zeit, dag.festeErzeugerListe())
	if err != nil {
		return err
	}
	return pruefeZertifikat(z, komitee)
}

// nachgespielterBlockKopf: Hoehe und Zeit eines nachgespielten Blocks aus
// dem Speicher -- nur der Kopf, kein Nachladen aus der Datenbank (ein
// ausgeduennter Block traegt Hoehe und Zeit weiter). Nicht im Speicher oder
// nicht nachgespielt: da = false.
func (dag *BlockDAG) nachgespielterBlockKopf(hash string) (hoehe, zeit int64, da bool) {
	gefunden := func() bool {
		dag.mu.RLock()
		defer dag.mu.RUnlock()
		b := dag.blocks[hash]
		if b == nil {
			return false
		}
		hoehe, zeit = b.Height, b.Timestamp
		return true
	}()
	if !gefunden {
		return 0, 0, false
	}
	dag.replayedMu.Lock()
	defer dag.replayedMu.Unlock()
	return hoehe, zeit, dag.replayedBlocks[hash]
}

// festeErzeugerListe: die geschlossene Liste (AUTHORIZED_VALIDATORS), ohne
// den eigenen Schluessel; nil = offen.
func (dag *BlockDAG) festeErzeugerListe() map[string]bool {
	if len(dag.produzentenFest) == 0 {
		return nil
	}
	return dag.produzentenFest
}

// errCheckpointNichtPruefbar: dieser Knoten kann den Checkpoint gerade nicht
// beurteilen (eigene Luecke, Lesefehler, kein Zustand) -- im Konsens hiesse
// das "zurueckstellen, nachladen, erneut versuchen", nie "ungueltig" und nie
// "angenommen" (Pruefung von Baustein 3, HOCH).
var errCheckpointNichtPruefbar = errors.New("Checkpoint hier nicht pruefbar")

// checkpointImTragendenBlock: die Pruefung, wie sie als Konsensregel
// laufen muss (OFFEN 10, Baustein 3) -- ohne lokalen Zustand ausser der
// Geschichte: Bezugszeit ist die Zeit des tragenden Blocks (dessen
// Zeitstempelpruefung lief vorher), und statt "hier nachgespielt" muss der
// Checkpoint-Block in der Vergangenheit des tragenden Blocks liegen
// (vorfahrSuchen). Der Checkpoint-Block darf hoechstens
// checkpointHoechstensAlt vor und -- wie jeder Vorfahr je Stufe --
// hoechstens zeitstempelRueckToleranz nach dem tragenden Block datiert sein.
//
// Reihenfolge: Form, Zahl, Stichtag, Zeiten, Monotonie (billig) -- dann
// Komitee und Unterschriften (hoechstens 100 Recoveries; die Unterschriften
// binden Hash, Hoehe und Zeit) -- erst zuletzt die Suche in der Datenbank.
// So loest nur ein echtes Quorum-Zertifikat die teure Suche aus.
//
// Fehler, die nur diesen Knoten betreffen (Luecke, Lesefehler, kein Stand),
// sind errCheckpointNichtPruefbar. NOCH LOKAL und vor dem Anschluss offen
// (docs/OFFEN.md): Komitee-Stand, "letzter" Checkpoint.
func (dag *BlockDAG) checkpointImTragendenBlock(z CheckpointZertifikat, letzter *KettenCheckpoint, tragenderEltern []string, tragenderZeit int64, lies kopfLeser) error {
	if err := z.gueltig(); err != nil {
		return err
	}
	if n := len(z.Unterschriften); n < checkpointQuorum(checkpointMindestKomitee) || n > targetCommitteeSize {
		return fmt.Errorf("%d Unterschriften -- ausserhalb von [%d, %d]", n, checkpointQuorum(checkpointMindestKomitee), targetCommitteeSize)
	}
	if !erzeugerSchnittAktiv(z.Zeit) {
		return fmt.Errorf("Checkpoint vor erzeugerSchnittAb")
	}
	if z.Zeit > tragenderZeit+zeitstempelRueckToleranz {
		return fmt.Errorf("Checkpoint-Zeit %d mehr als %d s nach dem tragenden Block (%d)", z.Zeit, zeitstempelRueckToleranz, tragenderZeit)
	}
	if tragenderZeit-z.Zeit > checkpointHoechstensAlt {
		return fmt.Errorf("Checkpoint %d s vor dem tragenden Block (hoechstens %d)", tragenderZeit-z.Zeit, checkpointHoechstensAlt)
	}
	if letzter != nil && z.Zeit <= letzter.Zeit {
		return fmt.Errorf("Checkpoint (Zeit %d) nicht nach dem letzten (Zeit %d)", z.Zeit, letzter.Zeit)
	}
	if dag.state == nil {
		return fmt.Errorf("%w: kein Zustand", errCheckpointNichtPruefbar)
	}
	komitee, err := checkpointKomitee(dag.state.erzeugerRegister.Load(), z.Zeit, dag.festeErzeugerListe())
	if err != nil {
		return fmt.Errorf("%w: %v", errCheckpointNichtPruefbar, err)
	}
	if err := pruefeZertifikat(z, komitee); err != nil {
		return err
	}
	kopf, gefunden, vollstaendig, err := vorfahrSuchen(lies, tragenderEltern, z.Hash, z.Zeit-checkpointHoechstensAlt)
	switch {
	case errors.Is(err, errVorfahrZuGross):
		return fmt.Errorf("Vergangenheit des tragenden Blocks zu gross: %w", err)
	case err != nil:
		return fmt.Errorf("%w: Vorfahren nicht lesbar: %v", errCheckpointNichtPruefbar, err)
	case !gefunden && !vollstaendig:
		return fmt.Errorf("%w: Vergangenheit des tragenden Blocks hier unvollstaendig", errCheckpointNichtPruefbar)
	case !gefunden:
		return fmt.Errorf("Block %s liegt nicht in der Vergangenheit des tragenden Blocks", kurzAdresse(z.Hash))
	}
	if kopf.hoehe != z.Hoehe || kopf.zeit != z.Zeit {
		return fmt.Errorf("Checkpoint (Hoehe %d, Zeit %d) passt nicht zum Block (Hoehe %d, Zeit %d)",
			z.Hoehe, z.Zeit, kopf.hoehe, kopf.zeit)
	}
	return nil
}
