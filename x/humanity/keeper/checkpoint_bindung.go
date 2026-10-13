package keeper

import (
	"errors"
	"fmt"
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
// Ablehnung (fail-closed).

// checkpointHoechstensAlt: so weit darf der Block eines Checkpoints vor der
// Bezugszeit (Einreichung, spaeter: Zeit des tragenden Blocks) liegen.
const checkpointHoechstensAlt int64 = 600

// checkpointKomitee: das Komitee zur Zeit t aus dem Stand.
func checkpointKomitee(st *erzeugerStand, t int64) (map[string]bool, error) {
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
		if st.erzeugerFenster(addr, t) != "" {
			komitee[addr] = true
		}
	}
	return komitee, nil
}

// checkpointGegenKette: nil, wenn das Zertifikat zu einem nachgespielten
// Block dieses Knotens gehoert, frisch ist (Bezugszeit jetzt), nach letzter
// liegt (nil: noch keiner) und vom Komitee zur Blockzeit bestaetigt ist.
// Aufrufer haelt dag.mu NICHT.
func (dag *BlockDAG) checkpointGegenKette(z CheckpointZertifikat, letzter *KettenCheckpoint, jetzt int64) error {
	if err := z.gueltig(); err != nil {
		return err
	}
	if z.Zeit > jetzt+zeitstempelZukunftToleranz {
		return fmt.Errorf("Checkpoint-Zeit %d liegt in der Zukunft (jetzt %d)", z.Zeit, jetzt)
	}
	if jetzt-z.Zeit > checkpointHoechstensAlt {
		return fmt.Errorf("Checkpoint %d s alt (hoechstens %d)", jetzt-z.Zeit, checkpointHoechstensAlt)
	}
	if letzter != nil && (z.Zeit <= letzter.Zeit || z.Hoehe <= letzter.Hoehe) {
		return fmt.Errorf("Checkpoint (Hoehe %d, Zeit %d) nicht nach dem letzten (Hoehe %d, Zeit %d)",
			z.Hoehe, z.Zeit, letzter.Hoehe, letzter.Zeit)
	}
	if !dag.hatNachgespieltenBlock(z.Hash) {
		return fmt.Errorf("Block %s hier nicht nachgespielt", kurzAdresse(z.Hash))
	}
	b := dag.GetBlockByHash(z.Hash)
	if b == nil {
		return fmt.Errorf("Block %s nicht lesbar", kurzAdresse(z.Hash))
	}
	if b.Height != z.Hoehe || b.Timestamp != z.Zeit {
		return fmt.Errorf("Checkpoint (Hoehe %d, Zeit %d) passt nicht zum Block (Hoehe %d, Zeit %d)",
			z.Hoehe, z.Zeit, b.Height, b.Timestamp)
	}
	if dag.state == nil {
		return errors.New("kein Zustand")
	}
	komitee, err := checkpointKomitee(dag.state.erzeugerRegister.Load(), z.Zeit)
	if err != nil {
		return err
	}
	return pruefeZertifikat(z, komitee)
}
