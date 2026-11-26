package keeper

import (
	"context"
	"math"
	"strings"
	"testing"
)

// P2, Pfand je Praeparatorin (quanten_s_pfaender.go): gegen die echten
// Regeln, annehmender und nachspielender Knoten im Gleichschritt (qwWelt).

const (
	pfNAnna = "1111111111111111111111111111111111111111111111111111111111111111111111111"
	pfNBert = "2222222222222222222222222222222222222222222222222222222222222222222222222"
)

// neuePfandWelt: Weg B, Los behalten und Pfaender aktiv (ab ab); Anna hat
// einen Nullifier, Bert keinen (Konto ohne Gesicht).
func neuePfandWelt(t *testing.T, ab int64) *qwWelt {
	t.Helper()
	w := neuePraepWelt(t)
	quantenSLosBehaltenAbOverride.Store(qwT0 - 86400)
	quantenSPfaenderAbOverride.Store(ab)
	t.Cleanup(func() { quantenSLosBehaltenAbOverride.Store(0); quantenSPfaenderAbOverride.Store(0) })
	w.nullifier(pfNAnna, w.anna)
	return w
}

// nullifier: N gehoert w (auf beiden Knoten); w == "" loescht ihn.
func (w *qwWelt) nullifier(n, wallet string) {
	for _, cs := range []*ChainState{w.cs, w.nach} {
		cs.nullifiersMu.Lock()
		if cs.nullifiers == nil {
			cs.nullifiers = map[string]string{}
		}
		if wallet == "" {
			delete(cs.nullifiers, n)
		} else {
			cs.nullifiers[n] = wallet
		}
		cs.nullifiersMu.Unlock()
	}
}

func pfZurueckTx(konto string) Transaction {
	return Transaction{Type: quantenSPfandZurueckTxArt, QuantenW: &QuantenWAngaben{K: quantenSPraepK(konto)}}
}

func (w *qwWelt) praep(konto string) quantenSPraep {
	return w.cs.quantenWJetzt().Praep[quantenSPraepK(konto)]
}

// Schlafend: Melden wie bisher (kein Pfand, keine Buchung, Bytes ohne
// "pfand"), die Rueckgabe unbekannt.
func TestQuantenSPfand_Schlafend(t *testing.T) {
	w := neuePfandWelt(t, math.MaxInt64)
	z := qwT0
	vorher := w.stand(w.cs, w.anna)
	tx := w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z), z)
	if tx.QuantenW.Pfand != 0 || w.stand(w.cs, w.anna) != vorher || w.praep(w.anna).Pfand != 0 || w.praep(w.anna).N != "" {
		t.Fatalf("schlafend: Pfand genommen: %+v", w.praep(w.anna))
	}
	if k := w.cs.quantenWJetzt().kodiert(); strings.Contains(k, `"pfand"`) || strings.Contains(k, `"n"`) {
		t.Fatalf("schlafend: Pfand in den Bytes: %s", k)
	}
	if bekannteTxArt(quantenSPfandZurueckTxArt, z) {
		t.Error("Rueckgabe vor dem Stichtag bekannt")
	}
	if r := w.praepRegel(pfZurueckTx(w.anna), z+100); r != "quanten_s_pfand_inaktiv" {
		t.Fatalf("Rueckgabe schlafend: %q", r)
	}
	if _, da := quantenWStatus()["pfaender_ab"]; da {
		t.Error("schlafend: pfaender_ab im Status")
	}
}

// Gutfall: Melden nimmt 10 AEQ, die Rueckgabe gibt sie 30 Tage nach der
// Abmeldung zurueck -- an die Wallet des Menschen; Geld bleibt erhalten.
func TestQuantenSPfand_Gutfall(t *testing.T) {
	w := neuePfandWelt(t, qwT0-86400)
	z := qwT0
	summe0 := w.summe(w.cs)
	anna0 := w.stand(w.cs, w.anna)
	tx := w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z), z)
	e := w.praep(w.anna)
	if tx.QuantenW.Pfand != quantenSPraepPfand || e.Pfand != quantenSPraepPfand || e.N != pfNAnna ||
		w.stand(w.cs, w.anna) != anna0-quantenSPraepPfand || w.stand(w.cs, quantenWPfandAddr) != quantenSPraepPfand {
		t.Fatalf("Melden: %+v, Anna %d, Topf %d", e, w.stand(w.cs, w.anna), w.stand(w.cs, quantenWPfandAddr))
	}
	if w.summe(w.cs) != summe0 {
		t.Fatal("Melden: Geld nicht erhalten")
	}
	// Gemeldet: keine Rueckgabe.
	if r := w.praepRegel(pfZurueckTx(w.anna), z+quantenSPraepHaltezeit+10); r != "quanten_s_pfand_gemeldet" {
		t.Fatalf("gemeldet zurueck: %q", r)
	}
	ab := z + 100
	w.annehmen(praepTx(t, quantenSPraepAbmeldenTxArt, w.anna, w.keys[w.anna], ab), ab)
	if r := w.praepRegel(pfZurueckTx(w.anna), ab+quantenSPraepHaltezeit-1); r != "quanten_s_pfand_frist" {
		t.Fatalf("eine Sekunde zu frueh: %q", r)
	}
	// Ein Eintrag mit Pfand wird nicht aufgeraeumt, auch nach dem Nachlauf.
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.bert, w.keys[w.bert], ab+quantenSPraepNachlauf+10), ab+quantenSPraepNachlauf+10)
	if w.praep(w.anna).Pfand != quantenSPraepPfand {
		t.Fatal("Eintrag mit Pfand aufgeraeumt")
	}
	zz := ab + quantenSPraepHaltezeit
	tx = w.annehmen(pfZurueckTx(w.anna), zz)
	if tx.QuantenW.Pfand != quantenSPraepPfand || tx.QuantenW.PfandAn != w.anna || w.stand(w.cs, w.anna) != anna0 {
		t.Fatalf("Rueckgabe: %+v, Anna %d", tx.QuantenW.gerechnet(), w.stand(w.cs, w.anna))
	}
	if e := w.praep(w.anna); e.Pfand != 0 || e.N != "" || e.Abgemeldet != ab {
		t.Fatalf("nach der Rueckgabe: %+v", e)
	}
	if w.summe(w.cs) != summe0 {
		t.Fatal("Rueckgabe: Geld nicht erhalten")
	}
	if r := w.praepRegel(pfZurueckTx(w.anna), zz+10); r != "quanten_s_pfand_unbekannt" {
		t.Fatalf("zweite Rueckgabe: %q", r)
	}
	// Danach gilt der Nachlauf wie bisher: aufgeraeumt beim naechsten Melden.
	spaet := ab + quantenSPraepHaltezeit + quantenSPraepNachlauf
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], spaet), spaet)
	if e := w.praep(w.anna); e.Gemeldet != spaet || e.Pfand != quantenSPraepPfand {
		t.Fatalf("neu gemeldet: %+v", e)
	}
	if ab, _ := quantenWStatus()["pfaender_ab"].(int64); ab != qwT0-86400 {
		t.Fatalf("Status: %v", quantenWStatus()["pfaender_ab"])
	}
}

// Umzug (R-P2): das Pfand geht an die neue Wallet, nie an die alte; ein
// Mensch hat hoechstens ein Pfand.
func TestQuantenSPfand_Umzug(t *testing.T) {
	w := neuePfandWelt(t, qwT0-86400)
	z := qwT0
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z), z)
	anna1 := w.stand(w.cs, w.anna)
	neuKey, neu := neuerSchluessel(t)
	w.keys[neu] = neuKey
	for _, cs := range []*ChainState{w.cs, w.nach} {
		addHuman(cs, neu, 100)
		a, _ := cs.accounts.Get(w.anna)
		a.IsHuman = false
	}
	w.nullifier(pfNAnna, neu)
	// Die neue Wallet meldet sich, solange das alte Pfand liegt: abgewiesen.
	if r := w.praepRegel(praepTx(t, quantenSPraepMeldenTxArt, neu, neuKey, z+50), z+50); r != "quanten_s_pfand_mensch" {
		t.Fatalf("zweites Pfand desselben Menschen: %q", r)
	}
	// Die alte Wallet ist kein Mensch mehr: jeder raeumt sie ab.
	w.annehmen(praepAufraeumenTx(w.anna), z+100)
	tx := w.annehmen(pfZurueckTx(w.anna), z+100+quantenSPraepHaltezeit)
	if tx.QuantenW.PfandAn != neu || w.stand(w.cs, neu) != 100_000_000+quantenSPraepPfand || w.stand(w.cs, w.anna) != anna1 {
		t.Fatalf("nach dem Umzug an %s (neu %s), neu %d, alt %d", tx.QuantenW.PfandAn, neu, w.stand(w.cs, neu), w.stand(w.cs, w.anna))
	}
	// Danach darf der Mensch mit der neuen Wallet wieder.
	zz := z + 200 + quantenSPraepHaltezeit
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, neu, neuKey, zz), zz)
	if e := w.praep(neu); e.N != pfNAnna || e.Pfand != quantenSPraepPfand {
		t.Fatalf("neue Wallet: %+v", e)
	}
}

// Ohne Nullifier (Konto ohne Gesicht): Pfand wie alle, zurueck an das Konto.
// Hat der Mensch hinter N keine Wallet mehr: nichts geht hinaus.
func TestQuantenSPfand_EmpfaengerOhneUndVerloren(t *testing.T) {
	w := neuePfandWelt(t, qwT0-86400)
	z := qwT0
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.bert, w.keys[w.bert], z), z)
	if e := w.praep(w.bert); e.N != "" || e.Pfand != quantenSPraepPfand {
		t.Fatalf("ohne Nullifier: %+v", e)
	}
	w.annehmen(praepTx(t, quantenSPraepAbmeldenTxArt, w.bert, w.keys[w.bert], z+10), z+10)
	if tx := w.annehmen(pfZurueckTx(w.bert), z+10+quantenSPraepHaltezeit); tx.QuantenW.PfandAn != w.bert {
		t.Fatalf("ohne Nullifier an %s", tx.QuantenW.PfandAn)
	}
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z), z)
	w.annehmen(praepTx(t, quantenSPraepAbmeldenTxArt, w.anna, w.keys[w.anna], z+10), z+10)
	w.nullifier(pfNAnna, "")
	if r := w.praepRegel(pfZurueckTx(w.anna), z+10+quantenSPraepHaltezeit); r != "quanten_s_pfand_empfaenger" {
		t.Fatalf("Mensch ohne Wallet: %q (das Pfand ginge an die alte Wallet)", r)
	}
}

// Missbrauch beim Melden: ohne Deckung, Nullifier nicht eindeutig.
func TestQuantenSPfand_MeldenMissbrauch(t *testing.T) {
	w := neuePfandWelt(t, qwT0-86400)
	z := qwT0
	armKey, arm := neuerSchluessel(t)
	for _, cs := range []*ChainState{w.cs, w.nach} {
		addHuman(cs, arm, 9.999999)
	}
	if r := w.praepRegel(praepTx(t, quantenSPraepMeldenTxArt, arm, armKey, z), z); r != "quanten_w_deckung" {
		t.Fatalf("ohne Deckung: %q", r)
	}
	w.nullifier(pfNBert, w.anna) // zwei Nullifier fuer Anna
	if r := w.praepRegel(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z), z); r != "quanten_s_pfand_mensch" {
		t.Fatalf("Nullifier nicht eindeutig: %q", r)
	}
}

// Nachlegen: vor dem Stichtag ohne Pfand gemeldet -- ab dem Stichtag nicht
// mehr im Los, bis das Pfand liegt; Meldezeit bleibt.
func TestQuantenSPfand_Nachlegen(t *testing.T) {
	ab := qwT0 + 1000
	w := neuePfandWelt(t, ab)
	z := qwT0
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z), z)
	T := z + quantenSPraepVorlauf + 10
	if a := w.abbild(w.cs, T); len(a) != 0 {
		t.Fatalf("ohne Pfand nach dem Stichtag im Los: %v", a)
	}
	if a := w.abbild(w.cs, ab-1); len(a) != 0 {
		t.Fatal("Vorbedingung: vor dem Vorlauf nicht im Los")
	}
	tx := w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], ab+10), ab+10)
	e := w.praep(w.anna)
	if tx.QuantenW.Pfand != quantenSPraepPfand || e.Pfand != quantenSPraepPfand || e.Gemeldet != z {
		t.Fatalf("Nachlegen: %+v", e)
	}
	if a := w.abbild(w.cs, T); len(a) != 1 || a[0] != w.anna {
		t.Fatalf("mit Pfand nicht im Los: %v", a)
	}
	if r := w.praepRegel(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], ab+20), ab+20); r != "quanten_s_praep_doppelt" {
		t.Fatalf("zweimal nachlegen: %q", r)
	}
}

// Gebunden (R-P3): im Los eines laufenden Tokens, eines beendeten mit
// offener Frist, eines toten ohne Ende oder einer offenen Bestellung.
func TestQuantenSPfand_Gebunden(t *testing.T) {
	anna := "0x00000000000000000000000000000000000000a1"
	los := []string{anna, "0x00000000000000000000000000000000000000b2", "0x00000000000000000000000000000000000000c3", "0x00000000000000000000000000000000000000d4"}
	K := strings.Repeat("a", 64)
	ende := int64(5_000_000)
	r := neuesQuantenWRegister()
	r.Lose[K] = los
	r.Tote[K] = 1
	r.Enden[K] = quantenWEnde{Ende: ende, Urteil: quantenWUrteilEingeloest}
	if !quantenSPraepGebunden(r, anna, ende+quantenSNachweisFrist) {
		t.Error("am letzten Tag der Frist frei")
	}
	if quantenSPraepGebunden(r, anna, ende+quantenSNachweisFrist+1) {
		t.Error("nach der Frist gebunden")
	}
	if quantenSPraepGebunden(r, "0x00000000000000000000000000000000000000ff", ende) {
		t.Error("fremdes Konto gebunden")
	}
	delete(r.Enden, K)
	if !quantenSPraepGebunden(r, anna, math.MaxInt64-1) {
		t.Error("toter Eintrag ohne Ende frei (fail-closed verletzt)")
	}
	r2 := neuesQuantenWRegister()
	r2.Lose[K] = los
	r2.Token[K] = &quantenWSEintrag{}
	if !quantenSPraepGebunden(r2, anna, math.MaxInt64-1) {
		t.Error("laufender Token frei")
	}
	r3 := neuesQuantenWRegister()
	r3.Best[K] = &quantenSBestellung{Los: los}
	if !quantenSPraepGebunden(r3, anna, math.MaxInt64-1) {
		t.Error("offene Bestellung frei")
	}
}

// Gebunden gegen die Kette: Anna steht im Los eines eingeloesten Tokens.
// Die Rueckgabe wartet die Nachweis-Frist ab -- auch nach den 30 Tagen.
func TestQuantenSPfand_GebundenKette(t *testing.T) {
	w := neuePfandWelt(t, qwT0-86400)
	z := qwT0
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z), z)
	w.annehmen(praepTx(t, quantenSPraepAbmeldenTxArt, w.anna, w.keys[w.anna], z+10), z+10)
	K := strings.Repeat("b", 64)
	ende := z + quantenSPraepHaltezeit
	for _, cs := range []*ChainState{w.cs, w.nach} {
		reg := cs.quantenWJetzt().kopie()
		reg.Tote[K] = z + 5
		reg.Enden[K] = quantenWEnde{Ende: ende, Urteil: quantenWUrteilEingeloest}
		reg.Lose[K] = []string{"0x00000000000000000000000000000000000000b2", w.anna, "0x00000000000000000000000000000000000000c3", "0x00000000000000000000000000000000000000d4"}
		cs.mu.Lock()
		if err := cs.quantenWSetzenLocked(context.Background(), reg); err != nil {
			t.Fatal(err)
		}
		cs.mu.Unlock()
	}
	if r := w.praepRegel(pfZurueckTx(w.anna), ende+quantenSNachweisFrist); r != "quanten_s_pfand_gebunden" {
		t.Fatalf("in der Frist: %q", r)
	}
	if tx := w.annehmen(pfZurueckTx(w.anna), ende+quantenSNachweisFrist+1); tx.QuantenW.PfandAn != w.anna {
		t.Fatalf("nach der Frist: %+v", tx.QuantenW.gerechnet())
	}
}

// Ein Block mit falschem Pfand oder falschem Empfaenger: jeder Knoten
// rechnet selbst und weist ihn ab; Form der Rueckgabe streng.
func TestQuantenSPfand_GefaelschtImBlock(t *testing.T) {
	w := neuePfandWelt(t, qwT0-86400)
	z := qwT0
	melden := praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z)
	u := w.bescheid(w.cs, melden, z)
	for _, pfand := range []int64{0, 1, quantenSPraepPfand + 1} {
		tx := melden
		q := *tx.QuantenW
		q.Pfand = pfand
		tx.QuantenW, tx.BuchAt = &q, z
		if err := w.nachspielen(tx, z+7); err == nil {
			t.Errorf("Melden mit Pfand %d im Block angenommen", pfand)
		}
	}
	if u.felder.Pfand != quantenSPraepPfand {
		t.Fatalf("gerechnet: %+v", u.felder)
	}
	w.annehmen(melden, z)
	w.annehmen(praepTx(t, quantenSPraepAbmeldenTxArt, w.anna, w.keys[w.anna], z+10), z+10)
	zz := z + 10 + quantenSPraepHaltezeit
	gut := pfZurueckTx(w.anna)
	gefaelscht := map[string]func(tx *Transaction){
		"anderer Empfaenger": func(tx *Transaction) { tx.QuantenW.Pfand, tx.QuantenW.PfandAn = quantenSPraepPfand, w.bert },
		"mehr Pfand":         func(tx *Transaction) { tx.QuantenW.Pfand, tx.QuantenW.PfandAn = quantenSPraepPfand+1, w.anna },
	}
	for name, f := range gefaelscht {
		tx := gut
		q := *gut.QuantenW
		tx.QuantenW, tx.BuchAt = &q, zz
		f(&tx)
		if err := w.nachspielen(tx, zz+7); err == nil {
			t.Errorf("%s im Block angenommen", name)
		}
	}
	form := map[string]func(tx *Transaction){
		"mit Wallet":       func(tx *Transaction) { tx.Wallet = w.bert },
		"mit Zeit":         func(tx *Transaction) { tx.DistributionAt = zz },
		"mit Unterschrift": func(tx *Transaction) { tx.QuantenW.WalletSig = "0x00" },
		"mit Index":        func(tx *Transaction) { tx.QuantenW.Index = 1 },
	}
	for name, f := range form {
		tx := gut
		q := *gut.QuantenW
		tx.QuantenW = &q
		f(&tx)
		if r := w.praepRegel(tx, zz); r != "quanten_w_form" && r != "quanten_w_zeit" {
			t.Errorf("%s: %q", name, r)
		}
	}
	w.annehmen(gut, zz)
}

// Lesen: Pfand und Nullifier fail-closed; ein Pfand je Mensch.
func TestQuantenSPfand_RegisterLesen(t *testing.T) {
	w := neuePfandWelt(t, qwT0-86400)
	z := qwT0
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z), z)
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.bert, w.keys[w.bert], z), z)
	gut := w.cs.quantenWJetzt().form()
	if r, err := quantenWRegisterLesen(gut.kodiert()); err != nil || r.kodiert() != gut.kodiert() {
		t.Fatalf("Rundweg: %v", err)
	}
	ia, ib := 0, 1
	if gut.Praep[0].Konto != w.anna {
		ia, ib = 1, 0
	}
	kaputt := map[string]func(f []quantenSPraepForm){
		"Pfand zu hoch":       func(f []quantenSPraepForm) { f[ia].Pfand = quantenSPraepPfand + 1 },
		"Pfand negativ":       func(f []quantenSPraepForm) { f[ib].Pfand = -1 },
		"N ohne Pfand":        func(f []quantenSPraepForm) { f[ia].Pfand = 0 },
		"N nicht kanonisch":   func(f []quantenSPraepForm) { f[ia].N = "0" + pfNAnna },
		"N kein Dezimal":      func(f []quantenSPraepForm) { f[ia].N = "abc" },
		"zwei Pfaender eines": func(f []quantenSPraepForm) { f[ib].N = pfNAnna },
	}
	for name, aendern := range kaputt {
		f := gut
		f.Praep = append([]quantenSPraepForm(nil), gut.Praep...)
		aendern(f.Praep)
		if _, err := quantenWRegisterLesen(f.kodiert()); err == nil {
			t.Errorf("%s: angenommen", name)
		}
	}
	// Zulaessig: weniger Pfand als 10 AEQ (P4 nimmt 10 % je Faelschung).
	f := gut
	f.Praep = append([]quantenSPraepForm(nil), gut.Praep...)
	f.Praep[ia].Pfand = quantenSPraepPfand - quantenSPraepPfand/10
	if _, err := quantenWRegisterLesen(f.kodiert()); err != nil {
		t.Errorf("9 AEQ abgewiesen: %v", err)
	}
}

// Pfaender ohne "Los behalten": schlafend (ohne Lose koennte kein Knoten
// pruefen, ob das Pfand gebunden ist).
func TestQuantenSPfand_NurMitLosBehalten(t *testing.T) {
	w := neuePfandWelt(t, qwT0-86400)
	quantenSLosBehaltenAbOverride.Store(math.MaxInt64)
	z := qwT0
	tx := w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z), z)
	if tx.QuantenW.Pfand != 0 || w.praep(w.anna).Pfand != 0 {
		t.Fatal("Pfand ohne Los behalten genommen")
	}
	if r := w.praepRegel(pfZurueckTx(w.anna), z+10); r != "quanten_s_pfand_inaktiv" {
		t.Fatalf("Rueckgabe ohne Los behalten: %q", r)
	}
}

// Nachlegen nur fuer gemeldete: wer vor dem Stichtag schon abgemeldet hat,
// meldet sich erst nach dem Nachlauf neu (wie bisher), statt still ein Pfand
// auf einen abgemeldeten Eintrag zu legen.
func TestQuantenSPfand_NachlegenNichtAbgemeldet(t *testing.T) {
	ab := qwT0 + 1000
	w := neuePfandWelt(t, ab)
	z := qwT0
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z), z)
	w.annehmen(praepTx(t, quantenSPraepAbmeldenTxArt, w.anna, w.keys[w.anna], z+10), z+10)
	if r := w.praepRegel(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], ab+10), ab+10); r != "quanten_s_praep_doppelt" {
		t.Fatalf("abgemeldet nachlegen: %q", r)
	}
}

// Nachlegen braucht keinen neuen Platz: auch bei vollem Register.
func TestQuantenSPfand_NachlegenVollesRegister(t *testing.T) {
	ab := qwT0 + 1000
	w := neuePfandWelt(t, ab)
	z := qwT0
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z), z)
	reg := w.cs.quantenWJetzt().kopie()
	for len(reg.Praep) < quantenSPraepHoechstens {
		k, a := neuerSchluessel(t)
		reg.Praep[quantenSPraepK(a)] = quantenSPraep{Konto: a, Pub: praepPubTest(k), Gemeldet: z}
	}
	for _, cs := range []*ChainState{w.cs, w.nach} {
		cs.mu.Lock()
		if err := cs.quantenWSetzenLocked(context.Background(), reg); err != nil {
			t.Fatal(err)
		}
		cs.mu.Unlock()
	}
	if r := w.praepRegel(praepTx(t, quantenSPraepMeldenTxArt, w.bert, w.keys[w.bert], ab+5), ab+5); r != "quanten_s_praep_voll" {
		t.Fatalf("Vorbedingung voll: %q", r)
	}
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], ab+10), ab+10)
	if w.praep(w.anna).Pfand != quantenSPraepPfand {
		t.Fatal("bei vollem Register nicht nachgelegt")
	}
}

// Ein laufender Token bindet, was auch immer sonst im Register steht.
func TestQuantenSPfand_GebundenLaufend(t *testing.T) {
	anna := "0x00000000000000000000000000000000000000a1"
	K := strings.Repeat("c", 64)
	r := neuesQuantenWRegister()
	r.Lose[K] = []string{anna}
	r.Token[K] = &quantenWSEintrag{}
	r.Enden[K] = quantenWEnde{Ende: 1, Urteil: quantenWUrteilEingeloest} // kann so nie entstehen
	if !quantenSPraepGebunden(r, anna, 1_000_000_000) {
		t.Error("laufender Token bindet nicht")
	}
}

// Nullifier der Wallet aus der echten Datenbank: keiner, einer (auch bei
// gemischter Schreibweise), mehrere (abgewiesen, nicht geraten).
func TestQuantenSPfand_Nullifier_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-quanten-s-pfand-realdb-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	ctx := context.Background()
	_, a := neuerSchluessel(t)
	_, b := neuerSchluessel(t)
	t.Cleanup(func() {
		cs.db.Exec(`DELETE FROM nullifiers WHERE nullifier IN ($1, $2, $3)`, pfNAnna, pfNBert, "3"+pfNAnna[1:])
	})
	frage := func(w string) (string, bool) {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		n, mehrere, err := cs.quantenSNullifierVonLocked(ctx, w)
		if err != nil {
			t.Fatal(err)
		}
		return n, mehrere
	}
	if n, m := frage(a); n != "" || m {
		t.Fatalf("ohne Nullifier: %q %v", n, m)
	}
	if _, err := cs.db.Exec(`INSERT INTO nullifiers (nullifier, wallet_address) VALUES ($1, $2)`, pfNAnna, strings.ToUpper(a[2:])); err != nil {
		t.Fatal(err)
	}
	// Gross geschrieben ohne 0x passt nicht zur Wallet: kein Treffer.
	if n, _ := frage(a); n != "" {
		t.Fatalf("fremde Schreibweise getroffen: %q", n)
	}
	if _, err := cs.db.Exec(`INSERT INTO nullifiers (nullifier, wallet_address) VALUES ($1, $2)`, pfNBert, "0x"+strings.ToUpper(b[2:])); err != nil {
		t.Fatal(err)
	}
	if n, m := frage(b); n != pfNBert || m {
		t.Fatalf("ein Nullifier: %q %v", n, m)
	}
	if _, err := cs.db.Exec(`INSERT INTO nullifiers (nullifier, wallet_address) VALUES ($1, $2)`, "3"+pfNAnna[1:], b); err != nil {
		t.Fatal(err)
	}
	if n, m := frage(b); n != "" || !m {
		t.Fatalf("zwei Nullifier: %q %v", n, m)
	}
}
