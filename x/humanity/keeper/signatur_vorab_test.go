package keeper

import (
	"testing"
	"time"
)

func signaturVorabLeeren(t *testing.T) {
	t.Helper()
	signaturVorabMu.Lock()
	signaturVorabMap = map[*Block]*signaturVorab{}
	signaturVorabMu.Unlock()
}

// Vorab gepruefter gueltiger Block: angenommen, das Ergebnis wird abgeholt
// und nicht liegen gelassen.
func TestSignaturVorab_GueltigerBlockUeberVorabpruefung(t *testing.T) {
	signaturVorabLeeren(t)
	a := neuerTestSchluessel(t)
	b := neuerTestSchluessel(t)
	dag, cs := nachspielKnoten(t, map[string]float64{a.addr: 1000})

	blk := testBlock(1, signiere(t, a, b.addr, aeqWei(10), 7, 1926))
	vorher := svGetroffen.Load()
	starteSignaturVorpruefung(blk)
	if ok := dag.replayTransactions(blk, true); !ok {
		t.Fatal("gueltiger Block ueber die Vorabpruefung abgelehnt")
	}
	if svGetroffen.Load() != vorher+1 {
		t.Fatal("Nachspielen hat das Vorab-Ergebnis nicht abgeholt")
	}
	if got := kontoVon(t, cs, a.addr).NaechsteNonce; got != 8 {
		t.Fatalf("NaechsteNonce %d statt 8", got)
	}
	signaturVorabMu.Lock()
	offen := len(signaturVorabMap)
	signaturVorabMu.Unlock()
	if offen != 0 {
		t.Fatalf("%d Vorab-Ergebnisse liegen nach dem Nachspielen noch herum", offen)
	}
}

// Das Wichtigste: eine Faelschung faellt auch dann auf, wenn ihre Pruefung
// vorab lief. Der Fehler darf auf dem Weg ueber die Goroutine nicht verloren
// gehen.
func TestSignaturVorab_FaelschungWirdAuchVorabAbgelehnt(t *testing.T) {
	signaturVorabLeeren(t)
	opfer := neuerTestSchluessel(t)
	angreifer := neuerTestSchluessel(t)
	dag, cs := nachspielKnoten(t, map[string]float64{opfer.addr: 5000})

	faelschung := signiere(t, angreifer, angreifer.addr, aeqWei(4000), 0, 1926)
	faelschung.Wallet = opfer.addr
	blk := testBlock(1, faelschung)
	starteSignaturVorpruefung(blk)
	if ok := dag.replayTransactions(blk, true); ok {
		t.Fatal("gefaelschter Block ueber die Vorabpruefung angenommen")
	}
	if got := kontoVon(t, cs, opfer.addr).Balance.Float(); got != 5000 {
		t.Fatalf("Opfer hat %.6f statt 5000", got)
	}
}

// Ein Ergebnis gehoert genau dem *Block, fuer den es gerechnet wurde. Ein
// anderes Objekt mit demselben Hash (etwa eine zweite Zustellung mit anderem
// Inhalt) bekommt es nicht, sondern wird selbst geprueft.
func TestSignaturVorab_ErgebnisGiltNurFuerDasselbeObjekt(t *testing.T) {
	signaturVorabLeeren(t)
	opfer := neuerTestSchluessel(t)
	angreifer := neuerTestSchluessel(t)
	dag, _ := nachspielKnoten(t, map[string]float64{opfer.addr: 5000, angreifer.addr: 100})

	echt := testBlock(1, signiere(t, angreifer, opfer.addr, aeqWei(1), 0, 1926))
	starteSignaturVorpruefung(echt)

	faelschung := signiere(t, angreifer, angreifer.addr, aeqWei(4000), 0, 1926)
	faelschung.Wallet = opfer.addr
	gleicherHash := testBlock(1, faelschung) // selber Hash "0xsig-1"
	if ok := dag.replayTransactions(gleicherHash, true); ok {
		t.Fatal("Faelschung hat das Vorab-Ergebnis eines anderen Blocks mit gleichem Hash benutzt")
	}
}

// Liegengebliebene Ergebnisse (Block nie nachgespielt) wachsen nicht ohne
// Grenze; wer ueber der Grenze kommt, wird im Nachspielen selbst geprueft.
func TestSignaturVorab_Begrenzt(t *testing.T) {
	signaturVorabLeeren(t)
	t.Cleanup(func() { signaturVorabLeeren(t) })
	a := neuerTestSchluessel(t)
	b := neuerTestSchluessel(t)
	_, _ = nachspielKnoten(t, nil)
	tx := signiere(t, a, b.addr, aeqWei(1), 0, 1926)
	for i := 0; i < signaturVorabGrenze+50; i++ {
		starteSignaturVorpruefung(testBlock(i, tx))
	}
	signaturVorabMu.Lock()
	n := len(signaturVorabMap)
	signaturVorabMu.Unlock()
	if n > signaturVorabGrenze {
		t.Fatalf("%d offene Vorab-Ergebnisse, Grenze %d", n, signaturVorabGrenze)
	}
	// Ohne Vorab-Ergebnis rechnet holeSignaturVorpruefung selbst -- richtig.
	v := holeSignaturVorpruefung(testBlock(99999, tx))
	if v.ueberwErr != nil || len(v.ueberweisungen) != 1 || v.ueberweisungen[0].nonce != 0 {
		t.Fatalf("Inline-Pruefung: %+v", v)
	}
	// Alte Eintraege werden beim naechsten Einfuegen geraeumt.
	signaturVorabMu.Lock()
	for _, e := range signaturVorabMap {
		e.gestartet = time.Now().Add(-2 * signaturVorabAlter)
	}
	signaturVorabMu.Unlock()
	starteSignaturVorpruefung(testBlock(100000, tx))
	signaturVorabMu.Lock()
	n = len(signaturVorabMap)
	signaturVorabMu.Unlock()
	if n != 1 {
		t.Fatalf("nach dem Raeumen %d Eintraege, erwartet 1", n)
	}
}
