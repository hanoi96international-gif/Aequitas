package keeper

import (
	"math"
	"strings"
	"testing"
)

// block_tauglich.go: was jeder andere Knoten an seiner Zeit scheitern liesse,
// kommt nicht in den Block.

func vormundAuftrag(t *testing.T, zeit int64) Transaction {
	t.Helper()
	k, w := neuerSchluessel(t)
	_, vormund := neuerSchluessel(t)
	return Transaction{Type: "vormund_setzen", Wallet: w, To: vormund,
		Nachweis: &Auftragsnachweis{Zeit: zeit, Sig: personalSign(t, k, vormundSetzenNachricht(vormund, zeit))}}
}

func TestBlockTauglich_AlterAuftragFliegtRaus(t *testing.T) {
	signierteUeberweisungenOverride.Store(1) // TestMain schaltet sie fuer alte Tests ab
	t.Cleanup(func() { signierteUeberweisungenOverride.Store(math.MaxInt64) })
	blockZeit := nowUnix()
	frisch := vormundAuftrag(t, blockZeit-60)
	alt := vormundAuftrag(t, blockZeit-7*3600) // sieben Stunden im Ausgang
	voraus := vormundAuftrag(t, blockZeit+3600)
	ueberweisung := Transaction{Type: "transfer", Wallet: "0xa", To: "0xb", Amount: 1}

	vorher := aussortierteAuftraege.Load()
	got := ohneUntauglicheAuftraege([]Transaction{frisch, alt, ueberweisung, voraus}, blockZeit)
	if len(got) != 2 || got[0].Wallet != frisch.Wallet || got[1].Type != "transfer" {
		t.Fatalf("im Block: %+v -- erwartet der frische Auftrag und die Ueberweisung, in dieser Reihenfolge", got)
	}
	if aussortierteAuftraege.Load()-vorher != 2 {
		t.Fatal("aussortierte Auftraege nicht gezaehlt")
	}
	// Was der Erzeuger hineinlegt, besteht dieselbe Pruefung wie bei jedem
	// Nachspielenden -- und das Weggelassene haette sie nicht bestanden.
	if _, err := pruefeAuftraegeImBlock(got, blockZeit); err != nil {
		t.Fatalf("der gefilterte Block faellt beim Nachspielen durch: %v", err)
	}
	if _, err := pruefeAuftraegeImBlock([]Transaction{alt}, blockZeit); err == nil {
		t.Fatal("Vorbedingung: der alte Auftrag muss beim Nachspielen durchfallen")
	}
}

// Der Erzeuger misst den Ausgang an GENAU der Blockzeit, die im Block steht.
func TestBlockTauglich_ProduceBlockFiltertMitDerBlockzeit(t *testing.T) {
	body := functionBodyFromSource(t, "block.go", "func (dag *BlockDAG) ProduceBlock(")
	for _, muss := range []string{
		"blockZeit := time.Now().Unix()\n\ttxs = ohneUntauglicheAuftraege(txs, blockZeit)\n\tblock := &Block{",
		"Timestamp:    blockZeit,",
	} {
		if !strings.Contains(body, muss) {
			t.Fatalf("ProduceBlock enthaelt nicht mehr:\n%s", muss)
		}
	}
}

// Erneuerung: im strengen Modus weist jeder Knoten eine Bescheinigung ab, die
// aelter als 7 Tage ist (erneuerung_zeit) -- also nicht in den Block.
func TestBlockTauglich_AlteErneuerungImStrengenModus(t *testing.T) {
	stagedGrantActivationOverride.Store(1)
	nachrechnenStrengOverride.Store(1)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0); nachrechnenStrengOverride.Store(0) })
	blockZeit := nowUnix()
	alt := Transaction{Type: "liveness_renewal", Wallet: "0xa", DistributionAt: blockZeit - 8*86400}
	frisch := Transaction{Type: "liveness_renewal", Wallet: "0xb", DistributionAt: blockZeit - 3600}
	got := ohneUntauglicheAuftraege([]Transaction{alt, frisch}, blockZeit)
	if len(got) != 1 || got[0].Wallet != "0xb" {
		t.Fatalf("im Block: %d Erneuerungen -- erwartet nur die frische", len(got))
	}
	// Im Beobachtungsmodus zaehlt das Nachrechnen nur -- dann bleibt sie drin.
	nachrechnenStrengOverride.Store(0)
	if got := ohneUntauglicheAuftraege([]Transaction{alt}, blockZeit); len(got) != 1 {
		t.Fatal("im Beobachtungsmodus aussortiert -- dort weist niemand ab")
	}
}
