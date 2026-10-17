package keeper

import "testing"

// TestSpitzeEinhaengen_ElternblockNachKind: der Ablauf aus dem
// Vier-Knoten-Test (spitzeNurOhneKind, block.go). Elternblock P ist
// gespeichert, aber noch nicht eingehaengt; prefetchParentsFromDB legt ihn
// ohne Spitzen-Buchfuehrung in dag.blocks, Kind K haengt sich an. Danach
// haengt AddPeerBlock P selbst ein -- P darf keine Spitze werden.
func TestSpitzeEinhaengen_ElternblockNachKind(t *testing.T) {
	g := &Block{Hash: "g", Height: 0}
	p := &Block{Hash: "p", Height: 1, ParentHashes: []string{"g"}}
	k := &Block{Hash: "k", Height: 2, ParentHashes: []string{"p"}}
	dag := &BlockDAG{blocks: map[string]*Block{"g": g}, tips: map[string]bool{"g": true}}

	// prefetchParentsFromDB: P aus der Datenbank, ohne Spitze.
	dag.blocks["p"] = p
	// K haengt sich an (AddPeerBlock fuer K: K stand vorher nicht im Speicher).
	dag.blocks["k"] = k
	dag.spitzeEinhaengenLocked(k, false)
	// AddPeerBlock fuer P kommt jetzt erst an seine Einhaenge-Stelle.
	_, vorher := dag.blocks["p"]
	dag.blocks["p"] = p
	dag.spitzeEinhaengenLocked(p, vorher)

	if dag.tips["p"] {
		t.Fatal("P ist Spitze, obwohl K auf ihm aufbaut -- tote Spitze, der naechste eigene Block erbte sie als Elternteil")
	}
	if !dag.tips["k"] || len(dag.tips) != 1 {
		t.Fatalf("Spitzen %v, erwartet nur k", dag.tips)
	}
}

// Gutfall: ein Block, der zum ersten Mal kommt, wird Spitze; einer, der
// vorher (z. B. als Stub oder aus der Datenbank) da war, aber kein Kind hat,
// ebenso.
func TestSpitzeEinhaengen_OhneKindWirdSpitze(t *testing.T) {
	g := &Block{Hash: "g"}
	p := &Block{Hash: "p", Height: 1, ParentHashes: []string{"g"}}
	dag := &BlockDAG{blocks: map[string]*Block{"g": g, "p": p}, tips: map[string]bool{"g": true}}
	dag.spitzeEinhaengenLocked(p, true)
	if !dag.tips["p"] || dag.tips["g"] {
		t.Fatalf("Spitzen %v, erwartet nur p", dag.tips)
	}
	q := &Block{Hash: "q", Height: 2, ParentHashes: []string{"p"}}
	dag.blocks["q"] = q
	dag.spitzeEinhaengenLocked(q, false)
	if !dag.tips["q"] || dag.tips["p"] || len(dag.tips) != 1 {
		t.Fatalf("Spitzen %v, erwartet nur q", dag.tips)
	}
}
