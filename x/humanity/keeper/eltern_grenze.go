package keeper

import "fmt"

// ELTERN EINES PEER-BLOCKS BEGRENZEN (OFFEN 10, Vorstufe).
//
// Der Erzeuger nimmt hoechstens maxParents() Spitzen (64 bei K=18), aber
// AddPeerBlock nahm jede Zahl an: jeder Elternteil kostet einen Nachschlag
// (ghostdagBlockLookup, ggf. mit DB-Rueckgriff) unter dag.mu, und jede
// Kegel-Pruefung (Erzeugerrate, Anker) waechst mit. Ein einziger Block mit
// zehntausenden Eltern-Hashes band so den ganzen Knoten.
//
// Die Grenze liegt mit 256 weit ueber dem, was ein ehrlicher Erzeuger je
// baut (64; bei hoeherem K 2*K), damit auch Geschichte aus Zeiten anderer
// Einstellungen beim Nachladen nicht abgewiesen wird. Doppelte Eltern baut
// kein ehrlicher Erzeuger (die Spitzen sind eine Menge) -- sie wuerden nur
// Nachschlaege vervielfachen.
//
// Geprueft wird vor dem Hash, vor jeder Sperre und vor jedem Nachschlag:
// billig und fail-closed. Haengt nur an signierten Feldern des Blocks --
// jeder Knoten entscheidet gleich.
const peerElternGrenze = 256

// elternUngueltig: Grund, warum die Eltern des Blocks nicht angenommen
// werden, oder "".
func elternUngueltig(b *Block) string {
	if b == nil {
		return ""
	}
	if n := len(b.ParentHashes); n > peerElternGrenze {
		return fmt.Sprintf("%d Eltern (hoechstens %d)", n, peerElternGrenze)
	}
	gesehen := make(map[string]struct{}, len(b.ParentHashes))
	for _, p := range b.ParentHashes {
		if _, doppelt := gesehen[p]; doppelt {
			return fmt.Sprintf("Elternteil %s doppelt", kurzAdresse(p))
		}
		gesehen[p] = struct{}{}
	}
	return ""
}
