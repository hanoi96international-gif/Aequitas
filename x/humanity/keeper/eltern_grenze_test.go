package keeper

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// Ein Peer-Block mit zu vielen oder doppelten Eltern wird abgewiesen, bevor
// irgendetwas nachgeschlagen wird. Beobachtbar: die Erzeugerpruefung dahinter
// (unbekannter Erzeuger -> Register-Nachfrage) wird gar nicht erst erreicht.
func TestElternGrenze_PeerBlock(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	mit := func(eltern []string) *Block {
		b := signTestBlockFromKey(t, key, 1, "deadbeef")
		b.ParentHashes = eltern
		b.Hash = calculateBlockHash(b)
		h, _ := hex.DecodeString(b.Hash)
		sig, err := crypto.Sign(h, key)
		if err != nil {
			t.Fatal(err)
		}
		b.Signature = hex.EncodeToString(sig)
		return b
	}
	viele := func(n int) []string {
		out := []string{"deadbeef"}
		for i := 1; i < n; i++ {
			out = append(out, fmt.Sprintf("%064x", i))
		}
		return out
	}
	erreicht := func(b *Block) bool {
		dag := newUnknownProposerTestDAG()
		dag.AddPeerBlock(b)
		_, ja := dag.unknownProposerLastRecovery[b.Proposer]
		return ja
	}
	if !erreicht(mit(viele(peerElternGrenze))) {
		t.Fatalf("%d Eltern (an der Grenze) frueh abgewiesen", peerElternGrenze)
	}
	if erreicht(mit(viele(peerElternGrenze + 1))) {
		t.Fatalf("%d Eltern nicht frueh abgewiesen", peerElternGrenze+1)
	}
	if erreicht(mit(viele(20000))) {
		t.Fatal("20000 Eltern nicht frueh abgewiesen")
	}
	if erreicht(mit([]string{"deadbeef", "deadbeef"})) {
		t.Fatal("doppelter Elternteil nicht abgewiesen")
	}
}

// Was der Erzeuger baut, nimmt jeder andere an: maxParents bleibt unter der
// Grenze, auch bei grossem K.
func TestElternGrenze_ErzeugerBleibtDarunter(t *testing.T) {
	dag := newOrphanTestDAG()
	for _, k := range []int64{0, int64(ghostdagKBase), 64, 200, 10000} {
		dag.activeGhostdagK.Store(int32(k))
		if m := dag.maxParents(); m > peerElternGrenze || m < maxParentsPerBlock {
			t.Fatalf("K=%d: maxParents %d ausserhalb [%d, %d]", k, m, maxParentsPerBlock, peerElternGrenze)
		}
	}
	if elternUngueltig(nil) != "" || elternUngueltig(&Block{ParentHashes: []string{"a", "b"}}) != "" {
		t.Fatal("Gutfall abgewiesen")
	}
}

// Die Bruecke (BridgeHistoricalGap) legt fuer fehlende Eltern Stuempfe an --
// fuer einen Block mit zu vielen Eltern keinen einzigen.
func TestElternGrenze_BrueckeLegtKeineStuempfeAn(t *testing.T) {
	eltern := []string{}
	for i := 0; i < peerElternGrenze+1; i++ {
		eltern = append(eltern, fmt.Sprintf("%064x", i+1))
	}
	boese := &Block{Hash: "boese", Height: 5000, ParentHashes: eltern, Proposer: "0xabc"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]*Block{boese})
	}))
	t.Cleanup(srv.Close)
	echt := httpSyncClient
	httpSyncClient = &http.Client{}
	t.Cleanup(func() { httpSyncClient = echt })

	dag := newOrphanTestDAG()
	dag.state = &ChainState{}
	dag.bootHeight = 10000
	vorher := len(dag.blocks)
	dag.BridgeHistoricalGap([]string{srv.URL})
	if n := len(dag.blocks) - vorher; n != 0 {
		t.Fatalf("%d Stuempfe fuer einen Block mit %d Eltern angelegt", n, len(eltern))
	}
	// Gegenprobe: derselbe Block mit einem Elternteil bekommt seinen Stumpf.
	boese.ParentHashes = eltern[:1]
	dag.BridgeHistoricalGap([]string{srv.URL})
	if n := len(dag.blocks) - vorher; n != 1 {
		t.Fatalf("Gutfall: %d Stuempfe, erwartet 1", n)
	}
}

// Ebenso beim Checkpoint aus dem Schnappschuss.
func TestElternGrenze_CheckpointLegtKeineStuempfeAn(t *testing.T) {
	dag := newOrphanTestDAG()
	eltern := []string{}
	for i := 0; i < peerElternGrenze+1; i++ {
		eltern = append(eltern, fmt.Sprintf("%064x", i+1))
	}
	vorher := len(dag.blocks)
	dag.mu.Lock()
	dag.seedCheckpointParentStubsLocked(&Block{Hash: "cp", Height: 100, ParentHashes: eltern})
	dag.mu.Unlock()
	if n := len(dag.blocks) - vorher; n != 0 {
		t.Fatalf("%d Stuempfe angelegt", n)
	}
	dag.mu.Lock()
	dag.seedCheckpointParentStubsLocked(&Block{Hash: "cp", Height: 100, ParentHashes: eltern[:2]})
	dag.mu.Unlock()
	if n := len(dag.blocks) - vorher; n != 2 {
		t.Fatalf("Gutfall: %d Stuempfe, erwartet 2", n)
	}
}
