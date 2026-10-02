package keeper

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// ------------------------------------------------------------ Verzeichnis

func verzeichnis(cs *ChainState, ctx context.Context, u, v, ort, annahme, web string, zeit int64) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.applyUnternehmenVerzeichnisLocked(ctx, u, v, ort, annahme, web, zeit, nowUnix())
}

func TestVerzeichnisNurVonVerantwortlichen(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	if err := verzeichnis(cs, ctx, wFirmaA, wMensch2, "Rosenheim", "bis 20 %", "", nowUnix()); !errors.Is(err, ErrZustandLehntAb) {
		t.Fatalf("Fremder setzt den Eintrag: %v", err)
	}
	if err := verzeichnis(cs, ctx, wFirmaA, wMensch1, "Rosenheim", "bis 20 % des Einkaufs", "https://baeckerei-beispiel.de", nowUnix()); err != nil {
		t.Fatal(err)
	}
	e := cs.wirt().unternehmen[wFirmaA]
	if e.Ort != "Rosenheim" || e.Annahme != "bis 20 % des Einkaufs" || e.Webseite != "https://baeckerei-beispiel.de" {
		t.Fatalf("Eintrag: %+v", e)
	}
}

func TestVerzeichnisAlteUnterschriftUeberschreibtNicht(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	z := nowUnix()
	if err := verzeichnis(cs, ctx, wFirmaA, wMensch1, "Neu", "", "", z); err != nil {
		t.Fatal(err)
	}
	for _, alt := range []int64{z, z - 1} {
		if err := verzeichnis(cs, ctx, wFirmaA, wMensch1, "Alt", "", "", alt); !errors.Is(err, ErrZustandLehntAb) {
			t.Fatalf("Wiederholung einer alten Unterschrift (%d) muss scheitern: %v", alt, err)
		}
	}
	if cs.wirt().unternehmen[wFirmaA].Ort != "Neu" {
		t.Fatal("alter Eintrag hat den neuen ueberschrieben")
	}
}

func TestVerzeichnisWebseiteNurHttpsUndRechnername(t *testing.T) {
	for _, gut := range []string{"", "https://laden.de", "https://www.mein-laden.example.org", "HTTPS://Laden.DE/"} {
		if _, ok := normWebseite(gut); !ok {
			t.Errorf("%q sollte gehen", gut)
		}
	}
	for _, schlecht := range []string{
		"http://laden.de", "https://laden.de/pfad", "https://laden.de:8443", "https://user@laden.de",
		"https://127.0.0.1", "https://localhost", "javascript:alert(1)", "https://laden.de?x=1",
		"https://-laden.de", "https://" + strings.Repeat("a", 100) + ".de", "ftp://laden.de",
	} {
		if _, ok := normWebseite(schlecht); ok {
			t.Errorf("%q darf nicht gehen", schlecht)
		}
	}
}

func TestVerzeichnisTextMussNormalisiertSein(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	for _, ort := range []string{"Zeile\nZwei", " Leerzeichen", strings.Repeat("x", 61), "a|b"} {
		if err := verzeichnis(cs, ctx, wFirmaA, wMensch1, ort, "", "", nowUnix()); !errors.Is(err, ErrZustandLehntAb) {
			t.Errorf("Ort %q: muss abgelehnt werden (der Produzent normalisiert vorher), bekam %v", ort, err)
		}
	}
}

func TestVerzeichnisVorDerAktivierungAbgelehnt(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	vorher := unternehmenVerzeichnisOverride.Load()
	unternehmenVerzeichnisOverride.Store(nowUnix() + 1)
	t.Cleanup(func() { unternehmenVerzeichnisOverride.Store(vorher) })
	if err := verzeichnis(cs, ctx, wFirmaA, wMensch1, "X", "", "", nowUnix()); !errors.Is(err, ErrZustandLehntAb) {
		t.Fatalf("vor der Aktivierung: %v", err)
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if !errors.Is(cs.applyUnternehmenBuergschaftLocked(ctx, wFirmaA, wMensch2, nowUnix()), ErrZustandLehntAb) ||
		!errors.Is(cs.applyUnternehmenAustretenLocked(ctx, wFirmaA, wMensch2, nowUnix()), ErrZustandLehntAb) {
		t.Fatal("Buergschaft und Austreten vor der Aktivierung muessen scheitern")
	}
}

// ------------------------------------------------------------ Buergschaft

func buerge(cs *ChainState, ctx context.Context, u, m string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.applyUnternehmenBuergschaftLocked(ctx, u, m, nowUnix())
}

func TestBuergschaftRegeln(t *testing.T) {
	cs, ctx, vor := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	if err := buerge(cs, ctx, wFirmaA, wMensch1); !errors.Is(err, ErrZustandLehntAb) {
		t.Fatalf("Verantwortliche buergt fuer sich selbst: %v", err)
	}
	if err := buerge(cs, ctx, wFirmaA, wFrei); !errors.Is(err, ErrZustandLehntAb) {
		t.Fatalf("freie Adresse buergt: %v", err)
	}
	if err := buerge(cs, ctx, wFirmaA, wMensch2); err != nil {
		t.Fatal(err)
	}
	if err := buerge(cs, ctx, wFirmaA, wMensch2); !errors.Is(err, ErrZustandLehntAb) {
		t.Fatalf("zweite Buergschaft desselben Menschen im selben Jahr: %v", err)
	}
	vor(366 * tag)
	if err := buerge(cs, ctx, wFirmaA, wMensch2); err != nil {
		t.Fatalf("nach einem Jahr wieder erlaubt: %v", err)
	}
	if e := cs.wirt().unternehmen[wFirmaA]; e.BuergenAnzahl != 2 || len(e.Buergen) != 1 {
		t.Fatalf("gezaehlt 2, gehalten nur die des letzten Jahres: %d / %d", e.BuergenAnzahl, len(e.Buergen))
	}
}

func TestBuergschaftGrenzeUeberAlleUnternehmen(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	gruender := []string{wMensch1, wMensch3}
	var firmen []string
	for i := 0; i < buergschaftenJeJahr+1; i++ {
		f := fmt.Sprintf("0xb4%038x", i+1)
		g := gruender[i%2]
		if i >= 4 { // hoechstens 3 Unternehmen je Mensch
			g = fmt.Sprintf("0xa5%038x", i)
			addHuman(cs, g, 1000)
		}
		eroeffne(t, cs, ctx, f, g)
		firmen = append(firmen, f)
	}
	for i, f := range firmen {
		err := buerge(cs, ctx, f, wMensch2)
		if i < buergschaftenJeJahr && err != nil {
			t.Fatalf("Buergschaft %d: %v", i+1, err)
		}
		if i == buergschaftenJeJahr && !errors.Is(err, ErrZustandLehntAb) {
			t.Fatalf("Buergschaft %d muss scheitern: %v", i+1, err)
		}
	}
}

func TestBuergschaftGrenzeUeberstehtSnapshot(t *testing.T) {
	// Ein Knoten, der von einem Snapshot startet, kennt das Register, aber
	// nicht die Buchfuehrung. Die Grenze muss bei ihm genauso greifen, sonst
	// spaltet sich die Kette an der sechsten Buergschaft.
	cs, ctx, _ := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	if err := buerge(cs, ctx, wFirmaA, wMensch2); err != nil {
		t.Fatal(err)
	}
	neu := newTestState()
	addHuman(neu, wMensch2, 1000)
	neu.unternehmenAusSnapshot(cs.unternehmenFuerSnapshot())
	if err := buerge(neu, ctx, wFirmaA, wMensch2); !errors.Is(err, ErrZustandLehntAb) {
		t.Fatalf("nach Snapshot: zweite Buergschaft im selben Jahr muss scheitern, bekam %v", err)
	}
}

func TestBuergschaftAnnahmeVerlangtEinkauf(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	if cs.hatDortBezahlt(wMensch2, wFirmaA, nowUnix()) {
		t.Fatal("ohne Einkauf darf es nicht als bezahlt gelten")
	}
	acct(cs, wMensch2).Balance = NewDecimal(4000)
	ueberweise(t, cs, ctx, wMensch2, wFirmaA, 25)
	if !cs.hatDortBezahlt(wMensch2, wFirmaA, nowUnix()) {
		t.Fatal("nach dem Einkauf muss es als bezahlt gelten")
	}
	if cs.hatDortBezahlt(wMensch3, wFirmaA, nowUnix()) {
		t.Fatal("ein anderer Mensch hat nicht bezahlt")
	}
}

// ------------------------------------------------------------ Austreten

func TestAustretenMitinhaberJaGruenderinNein(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	cs.mu.Lock()
	if err := cs.applyUnternehmenMitinhaberLocked(ctx, wFirmaA, wMensch2, nowUnix()); err != nil {
		cs.mu.Unlock()
		t.Fatal(err)
	}
	gruenderin := cs.applyUnternehmenAustretenLocked(ctx, wFirmaA, wMensch1, nowUnix())
	fremd := cs.applyUnternehmenAustretenLocked(ctx, wFirmaA, wMensch3, nowUnix())
	mit := cs.applyUnternehmenAustretenLocked(ctx, wFirmaA, wMensch2, nowUnix())
	cs.mu.Unlock()
	if !errors.Is(gruenderin, ErrZustandLehntAb) {
		t.Fatalf("Gruenderin tritt aus: %v", gruenderin)
	}
	if !errors.Is(fremd, ErrZustandLehntAb) {
		t.Fatalf("Fremder tritt aus: %v", fremd)
	}
	if mit != nil {
		t.Fatal(mit)
	}
	if v := cs.wirt().unternehmen[wFirmaA].Verantwortliche; len(v) != 1 || v[0] != wMensch1 {
		t.Fatalf("Verantwortliche danach: %v", v)
	}
}

// ------------------------------------------------------------ Nachweis

func TestNachweisNeueArtenGegenFaelschung(t *testing.T) {
	k, _ := crypto.GenerateKey()
	fremd, _ := crypto.GenerateKey()
	v := strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex())
	zeit := int64(1_800_000_000)
	msg := unternehmenVerzeichnisNachricht(wFirmaA, v, "Ort", "bis 20 %", "https://laden.de", zeit)
	tx := Transaction{Type: "unternehmen_verzeichnis", Wallet: wFirmaA, To: v, Ort: "Ort", Annahme: "bis 20 %",
		Webseite: "https://laden.de", Nachweis: &Auftragsnachweis{Sig: personalSign(t, k, msg), Zeit: zeit}}
	if err := pruefeAuftragsNachweis(&tx, zeit); err != nil {
		t.Fatalf("echte Unterschrift: %v", err)
	}
	// Der Produzent aendert nach der Unterschrift den Text.
	geaendert := tx
	geaendert.Annahme = "bis 100 %"
	if err := pruefeAuftragsNachweis(&geaendert, zeit); err == nil {
		t.Fatal("geaenderter Annahme-Text muss die Unterschrift brechen")
	}
	// Unterschrift mit einem fremden Schluessel.
	falsch := tx
	falsch.Nachweis = &Auftragsnachweis{Sig: personalSign(t, fremd, msg), Zeit: zeit}
	if err := pruefeAuftragsNachweis(&falsch, zeit); err == nil {
		t.Fatal("fremder Schluessel muss scheitern")
	}
	// Buergschaft und Austreten: unterschrieben vom Menschen (To).
	for _, typ := range []string{"unternehmen_buergschaft", "unternehmen_austreten"} {
		var m string
		if typ == "unternehmen_buergschaft" {
			m = unternehmenBuergschaftNachricht(wFirmaA, v, zeit)
		} else {
			m = unternehmenAustretenNachricht(wFirmaA, v, zeit)
		}
		ok := Transaction{Type: typ, Wallet: wFirmaA, To: v, Nachweis: &Auftragsnachweis{Sig: personalSign(t, k, m), Zeit: zeit}}
		if err := pruefeAuftragsNachweis(&ok, zeit); err != nil {
			t.Fatalf("%s echt: %v", typ, err)
		}
		umgelenkt := ok
		umgelenkt.Wallet = wFirmaB
		if err := pruefeAuftragsNachweis(&umgelenkt, zeit); err == nil {
			t.Fatalf("%s auf ein anderes Unternehmen umgelenkt muss scheitern", typ)
		}
		ohne := ok
		ohne.Nachweis = nil
		if err := pruefeAuftragsNachweis(&ohne, zeit); err == nil {
			t.Fatalf("%s ohne Nachweis muss scheitern", typ)
		}
	}
}

// ------------------------------------------------------------ Speicher

func TestVerzeichnisSpeicherUndKopie(t *testing.T) {
	e := &unternehmenEintrag{Adresse: wFirmaA, Ort: "O", Annahme: "A", Webseite: "https://w.de", VerzeichnisZeit: 7,
		Buergen: []buergeEintrag{{M: wMensch1, At: 3}}, BuergenAnzahl: 4}
	var zurueck unternehmenEintrag
	verzeichnisAusJSON(&zurueck, verzeichnisJSON(e))
	if zurueck.Ort != "O" || zurueck.Annahme != "A" || zurueck.Webseite != "https://w.de" || zurueck.VerzeichnisZeit != 7 ||
		zurueck.BuergenAnzahl != 4 || len(zurueck.Buergen) != 1 || zurueck.Buergen[0].M != wMensch1 {
		t.Fatalf("Rundreise: %+v", zurueck)
	}
	if verzeichnisJSON(&unternehmenEintrag{Adresse: wFirmaA}) != "" {
		t.Fatal("leerer Eintrag soll leer gespeichert werden")
	}
	cp := e.kopie()
	cp.Buergen[0].M = "x"
	if e.Buergen[0].M != wMensch1 {
		t.Fatal("kopie teilt die Buergen-Liste mit dem Original (Rueckrollen waere falsch)")
	}
}

// ------------------------------------------------------------ Nachspielen

// Ende zu Ende durch replayTransactions: echte Unterschriften, und ein Block
// mit einer gefaelschten oder unberechtigten Angabe wird abgelehnt.
func TestVerzeichnisBuergschaftAustretenNachspielen(t *testing.T) {
	wirtschaftAn(t)
	vorher := unternehmenVerzeichnisOverride.Load()
	unternehmenVerzeichnisOverride.Store(1)
	t.Cleanup(func() { unternehmenVerzeichnisOverride.Store(vorher) })
	firma, inhaber, mit, kunde, fremd := neuerTestSchluessel(t), neuerTestSchluessel(t), neuerTestSchluessel(t),
		neuerTestSchluessel(t), neuerTestSchluessel(t)
	dag, cs := nachspielKnoten(t, nil)
	for _, k := range []testSchluessel{inhaber, mit, kunde, fremd} {
		addHuman(cs, k.addr, 100)
	}
	jetzt := nowUnix()
	h := 0
	block := func(txs ...Transaction) *Block {
		h++
		b := testBlock(h, txs[0])
		b.Transactions = txs
		return b
	}
	eroeffnen := unternehmenEroeffnenNachricht(firma.addr, inhaber.addr, "Laden", "handel", jetzt)
	mitMsg := unternehmenMitinhaberNachricht(firma.addr, mit.addr, jetzt)
	if !dag.replayTransactions(block(
		Transaction{Type: "unternehmen_eroeffnen", Wallet: firma.addr, To: inhaber.addr, Name: "Laden", Kategorie: "handel",
			Nachweis: &Auftragsnachweis{Sig: persoenlichSignieren(t, firma, eroeffnen), Sig2: persoenlichSignieren(t, inhaber, eroeffnen), Zeit: jetzt}},
		Transaction{Type: "unternehmen_mitinhaber", Wallet: firma.addr, To: mit.addr,
			Nachweis: &Auftragsnachweis{Sig: persoenlichSignieren(t, mit, mitMsg), Sig2: persoenlichSignieren(t, inhaber, mitMsg), Von2: inhaber.addr, Zeit: jetzt}},
	), true) {
		t.Fatal("Eroeffnen und Mitinhaber abgelehnt")
	}

	verz := func(signer testSchluessel, zeit int64, annahme string) Transaction {
		msg := unternehmenVerzeichnisNachricht(firma.addr, signer.addr, "Rosenheim", annahme, "https://laden.de", zeit)
		return Transaction{Type: "unternehmen_verzeichnis", Wallet: firma.addr, To: signer.addr, Ort: "Rosenheim",
			Annahme: annahme, Webseite: "https://laden.de", Nachweis: &Auftragsnachweis{Sig: persoenlichSignieren(t, signer, msg), Zeit: zeit}}
	}
	// Fremder mit gueltiger eigener Unterschrift: nicht verantwortlich.
	if dag.replayTransactions(block(verz(fremd, jetzt, "bis 100 %")), true) {
		t.Fatal("Verzeichniseintrag eines Nicht-Verantwortlichen angenommen")
	}
	// Produzent aendert den Text nach der Unterschrift.
	gefaelscht := verz(inhaber, jetzt, "bis 20 %")
	gefaelscht.Annahme = "bis 100 %"
	if dag.replayTransactions(block(gefaelscht), true) {
		t.Fatal("geaenderter Verzeichnistext angenommen")
	}
	if !dag.replayTransactions(block(verz(inhaber, jetzt, "bis 20 %")), true) {
		t.Fatal("echter Verzeichniseintrag abgelehnt")
	}
	// Dieselbe Unterschrift noch einmal (Wiederholung): abgelehnt.
	if dag.replayTransactions(block(verz(inhaber, jetzt, "bis 20 %")), true) {
		t.Fatal("wiederholter Verzeichniseintrag angenommen")
	}

	buerg := func(k testSchluessel) Transaction {
		return Transaction{Type: "unternehmen_buergschaft", Wallet: firma.addr, To: k.addr,
			Nachweis: &Auftragsnachweis{Sig: persoenlichSignieren(t, k, unternehmenBuergschaftNachricht(firma.addr, k.addr, jetzt)), Zeit: jetzt}}
	}
	if dag.replayTransactions(block(buerg(inhaber)), true) {
		t.Fatal("Buergschaft der Verantwortlichen angenommen")
	}
	if !dag.replayTransactions(block(buerg(kunde)), true) {
		t.Fatal("Buergschaft eines Menschen abgelehnt")
	}
	if dag.replayTransactions(block(buerg(kunde)), true) {
		t.Fatal("zweite Buergschaft im selben Jahr angenommen")
	}
	ohne := buerg(fremd)
	ohne.Nachweis = nil
	if dag.replayTransactions(block(ohne), true) {
		t.Fatal("Buergschaft ohne Nachweis angenommen")
	}

	aus := func(k testSchluessel) Transaction {
		return Transaction{Type: "unternehmen_austreten", Wallet: firma.addr, To: k.addr,
			Nachweis: &Auftragsnachweis{Sig: persoenlichSignieren(t, k, unternehmenAustretenNachricht(firma.addr, k.addr, jetzt)), Zeit: jetzt}}
	}
	if dag.replayTransactions(block(aus(inhaber)), true) {
		t.Fatal("Gruenderin ausgetreten")
	}
	if !dag.replayTransactions(block(aus(mit)), true) {
		t.Fatal("Mitinhaber konnte nicht austreten")
	}

	cs.wirt().mu.Lock()
	e := cs.wirt().offenesUnternehmenLocked(firma.addr)
	cs.wirt().mu.Unlock()
	if e.Annahme != "bis 20 %" || e.BuergenAnzahl != 1 || len(e.Verantwortliche) != 1 || e.Verantwortliche[0] != inhaber.addr {
		t.Fatalf("Zustand nach dem Nachspielen: %+v", e)
	}
}
