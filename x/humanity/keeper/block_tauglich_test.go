package keeper

import (
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
)

// block_tauglich.go: die Blockzeit, zu der jeder andere Knoten alle
// Auftraege des Blocks annimmt -- nichts wird weggelassen.

// mitSignaturpflicht: TestMain schaltet die Pflicht fuer alte Tests ab.
func mitSignaturpflicht(t *testing.T) {
	t.Helper()
	signierteUeberweisungenOverride.Store(1)
	t.Cleanup(func() { signierteUeberweisungenOverride.Store(math.MaxInt64) })
}

func vormundAuftragVon(t *testing.T, k *ecdsa.PrivateKey, vormund string, zeit int64) Transaction {
	t.Helper()
	return Transaction{Type: "vormund_setzen", Wallet: adrVon(k), To: vormund,
		Nachweis: &Auftragsnachweis{Zeit: zeit, Sig: personalSign(t, k, vormundSetzenNachricht(vormund, zeit))}}
}

func vormundAuftrag(t *testing.T, zeit int64) Transaction {
	t.Helper()
	k, _ := neuerSchluessel(t)
	_, vormund := neuerSchluessel(t)
	return vormundAuftragVon(t, k, vormund, zeit)
}

// lebenszeichenAuftrag: der Vormund bestaetigt -- haengt am vormund_setzen.
func lebenszeichenAuftrag(t *testing.T, schuetzling string, vormund *ecdsa.PrivateKey, zeit int64) Transaction {
	t.Helper()
	return Transaction{Type: "lebenszeichen", Wallet: schuetzling, To: adrVon(vormund),
		Nachweis: &Auftragsnachweis{Zeit: zeit, Sig: personalSign(t, vormund, lebenszeichenNachricht(schuetzling, zeit))}}
}

// Normalfall: alles frisch -- die Blockzeit ist jetzt.
func TestBlockZeitFuer_Normalfall(t *testing.T) {
	mitSignaturpflicht(t)
	jetzt := int64(1_800_000_000)
	txs := []Transaction{vormundAuftrag(t, jetzt-30), {Type: "transfer", Wallet: "0xa", To: "0xb", Amount: 1}}
	got, err := blockZeitFuer(txs, jetzt, jetzt-1)
	if err != nil || got != jetzt {
		t.Fatalf("Blockzeit %d, %v -- erwartet jetzt (%d)", got, err, jetzt)
	}
}

// Missbrauch/Absturz: der Ausgang ueberlebt sieben Stunden. Vorher trug der
// erste Block die Uhrzeit -- jeder andere Knoten wies ihn ab, die Kette stand.
// Weglassen liesse den Erzeuger abweichen, und ein abhaengiger Auftrag (das
// Lebenszeichen des eben eingetragenen Vormunds) hielte die Kette trotzdem
// an. Jetzt: beide kommen in einen Block mit der Zeit ihrer Annahme, und die
// Pruefung jedes Nachspielenden besteht.
func TestBlockZeitFuer_NachAbsturzTraegtDieAnnahmezeit(t *testing.T) {
	mitSignaturpflicht(t)
	annahme := int64(1_800_000_000)
	jetzt := annahme + 7*3600
	schuetzling, _ := neuerSchluessel(t)
	vormundKey, vormund := neuerSchluessel(t)
	setzen := vormundAuftragVon(t, schuetzling, vormund, annahme-20)
	leben := lebenszeichenAuftrag(t, adrVon(schuetzling), vormundKey, annahme)
	txs := []Transaction{setzen, leben, {Type: "transfer", Wallet: "0xa", To: "0xb", Amount: 1}}

	// Vorbedingung: mit der Uhrzeit faellt der Block bei jedem durch.
	if _, err := pruefeAuftraegeImBlock(txs, jetzt); err == nil {
		t.Fatal("Vorbedingung: mit der Uhrzeit muss der Block durchfallen")
	}
	eltern := annahme - 5 // der letzte Block vor dem Absturz
	got, err := blockZeitFuer(txs, jetzt, eltern)
	if err != nil {
		t.Fatalf("keine Blockzeit: %v", err)
	}
	if got >= jetzt || got < eltern {
		t.Fatalf("Blockzeit %d ausserhalb [%d, %d)", got, eltern, jetzt)
	}
	if want := setzen.Nachweis.Zeit + nachweisHoechstensAlt; got != want {
		t.Fatalf("Blockzeit %d, erwartet die spaeteste passende %d", got, want)
	}
	if _, err := pruefeAuftraegeImBlock(txs, got); err != nil {
		t.Fatalf("der Block mit der gewaehlten Zeit faellt beim Nachspielen durch: %v", err)
	}
	if grund := zeitstempelRueckdatiert(got, eltern); grund != "" {
		t.Fatalf("Nachspielende weisen die Blockzeit ab: %s", grund)
	}
}

// Kein gemeinsamer Zeitpunkt: kein Block (fail-closed) -- nie weglassen.
func TestBlockZeitFuer_OhneGemeinsameZeitKeinBlock(t *testing.T) {
	mitSignaturpflicht(t)
	jetzt := int64(1_800_000_000)
	alt := vormundAuftrag(t, jetzt-7*3600)
	frisch := vormundAuftrag(t, jetzt-10)
	if _, err := blockZeitFuer([]Transaction{alt, frisch}, jetzt, jetzt-8*3600); err == nil {
		t.Fatal("alter und frischer Auftrag: es gibt keine gemeinsame Zeit, trotzdem eine gewaehlt")
	}
	// Die Eltern liegen nach dem Fenster des alten Auftrags (ein fremder
	// Block kam inzwischen): kein Block vor den Eltern.
	if _, err := blockZeitFuer([]Transaction{alt}, jetzt, jetzt-60); err == nil {
		t.Fatal("Blockzeit vor den Eltern gewaehlt")
	}
	// Missbrauch: ein Auftrag aus der Zukunft (mehr als 5 min) passt nie.
	if _, err := blockZeitFuer([]Transaction{vormundAuftrag(t, jetzt+3600)}, jetzt, jetzt-1); err == nil {
		t.Fatal("Auftrag aus der Zukunft in einen Block gelegt")
	}
}

// Das Fenster muss genau die Regeln von blockTauglich spiegeln: fuer jede
// Zeit t gilt "t im Fenster" genau dann, wenn blockTauglich(t) besteht.
func TestAuftragsFenster_SpiegeltBlockTauglich(t *testing.T) {
	mitSignaturpflicht(t)
	stagedGrantActivationOverride.Store(1)
	nachrechnenStrengOverride.Store(1)
	validatorRegisterOverride.Store(1)
	t.Cleanup(func() {
		stagedGrantActivationOverride.Store(0)
		nachrechnenStrengOverride.Store(0)
		validatorRegisterOverride.Store(0)
	})
	basis := rundenZeitStrengAbUnix + 30*86400
	betreiber, _ := neuerSchluessel(t)
	knoten, _ := neuerSchluessel(t)
	faelle := map[string]Transaction{
		"vormund":           vormundAuftrag(t, basis),
		"erneuerung":        {Type: "liveness_renewal", Wallet: "0xa", DistributionAt: basis},
		"erneuerung ohne":   {Type: "liveness_renewal", Wallet: "0xa"},
		"umlauf":            {Type: "umlauf", Wallet: "0xa", DistributionAt: basis},
		"rundenmarke":       {Type: "distribution_round_marker", DistributionAt: basis},
		"validator_bindung": bindungUnterschrieben(t, betreiber, knoten, basis),
		"ohne Nachweis":     {Type: "vormund_setzen", Wallet: "0xa", To: "0xb"},
		"transfer":          {Type: "transfer", Wallet: "0xa", To: "0xb", Amount: 1},
	}
	t.Cleanup(_setVertragForTest(&vertragKonfig{version: vertragVersionV8}))
	faelle["registrierung"] = Transaction{Type: "register_human", Wallet: "0xa", RegAt: basis}
	jetzt := basis + 10*86400
	spiegeln := func(name string, tx Transaction) {
		t.Helper()
		f := auftragsFenster(&tx, jetzt)
		for d := int64(-8 * 86400); d <= 8*86400; d += 37 {
			ts := basis + d
			drin := ts >= f.von && ts <= f.bis
			ok := blockTauglich(&tx, ts) == nil
			if drin != ok {
				t.Fatalf("%s: t=basis%+d im Fenster=%v, blockTauglich=%v (%v)", name, d, drin, ok, blockTauglich(&tx, ts))
			}
		}
	}
	for name, tx := range faelle {
		spiegeln(name, tx)
	}
	// Der Stichtag der Validator-Bindung mitten im Fenster ihres Nachweises.
	validatorRegisterOverride.Store(basis + 100)
	spiegeln("validator_bindung am Stichtag", faelle["validator_bindung"])
}

// Validator-Bindung: vor dem Stichtag passt keine Blockzeit.
func TestBlockZeitFuer_ValidatorBindungErstAbStichtag(t *testing.T) {
	jetzt := int64(1_800_000_000)
	validatorRegisterOverride.Store(jetzt - 100)
	t.Cleanup(func() { validatorRegisterOverride.Store(0) })
	betreiber, _ := neuerSchluessel(t)
	knoten, _ := neuerSchluessel(t)
	tx := bindungUnterschrieben(t, betreiber, knoten, jetzt-3000)
	got, err := blockZeitFuer([]Transaction{tx}, jetzt, jetzt-4000)
	if err != nil || got != jetzt {
		t.Fatalf("Blockzeit %d, %v", got, err)
	}
	// Bindung von vor dem Stichtag, Uhr weit danach: nur Zeiten vor dem
	// Stichtag passten zum Nachweis -- dort gilt die Bindung nicht.
	alt := bindungUnterschrieben(t, betreiber, knoten, jetzt-100-nachweisHoechstensAlt-10)
	if _, err := blockZeitFuer([]Transaction{alt}, jetzt+3*3600, jetzt-2*3600); err == nil {
		t.Fatal("Bindung in einen Block vor dem Stichtag gelegt")
	}
}

// Missbrauch: Vormund und Lebenszeichen nahmen das volle Fenster der
// Nachspielenden an (eine Stunde). Mit Zeit = jetzt - 3599 verfiel der
// Auftrag Sekunden spaeter im Ausgang. Jetzt hoechstens 10 Minuten.
func TestPruefeNachweisJetzt_AnnahmeEngerAlsNachspielen(t *testing.T) {
	jetzt := nowUnix()
	knapp := vormundAuftrag(t, jetzt-nachweisHoechstensAlt+1)
	if err := pruefeNachweisJetzt(&knapp); !errors.Is(err, ErrUeberweisungNichtSigniert) {
		t.Fatalf("knapp vor dem Verfall angenommen: %v", err)
	}
	zuAlt := vormundAuftrag(t, jetzt-nachweisAnnahmeHoechstensAlt-30)
	if err := pruefeNachweisJetzt(&zuAlt); err == nil {
		t.Fatal("11 Minuten alt angenommen")
	}
	frisch := vormundAuftrag(t, jetzt-60)
	if err := pruefeNachweisJetzt(&frisch); err != nil {
		t.Fatalf("frischer Auftrag abgewiesen: %v", err)
	}
}

// Die Annahme haelt an, solange die Erzeugung steht -- gemessen erst ab dem
// ersten ProduceBlock-Versuch -- oder der Ausgang von vor dem Start offen ist.
func TestAnnahmePausiert_ErzeugungSteht(t *testing.T) {
	cs := newTestState()
	if err := cs.annahmeBeginnen("0x" + strings.Repeat("a", 40)); err != nil {
		t.Fatalf("Knoten ohne Erzeugung: %v", err)
	}
	cs.annahmeEnde()

	jetzt := nowUnix()
	cs.erzeugerSeit.Store(jetzt - admissionStallLimit() - 5)
	err := cs.annahmeBeginnen("0x" + strings.Repeat("a", 40))
	if !errors.Is(err, ErrAnnahmePausiert) {
		t.Fatalf("Erzeugung steht seit %d s: %v", admissionStallLimit()+5, err)
	}
	cs.letzterEigenerBlock.Store(jetzt)
	if err := cs.annahmeBeginnen("0x" + strings.Repeat("a", 40)); err != nil {
		t.Fatalf("frischer eigener Block: %v", err)
	}
	cs.annahmeEnde()

	cs.ausgangVorStartBis.Store(42)
	if err := cs.annahmeBeginnen("0x" + strings.Repeat("a", 40)); !errors.Is(err, ErrAnnahmePausiert) {
		t.Fatalf("Ausgang von vor dem Start offen: %v", err)
	}
	if n := cs.annahmenLaufend.Load(); n != 0 {
		t.Fatalf("annahmenLaufend %d nach Ablehnungen", n)
	}
	// Ohne Datenbank loest der erste eigene Block die Startsperre.
	cs.eigenerBlockGespeichert()
	if err := cs.annahmePauseGrund(); err != nil {
		t.Fatalf("nach dem ersten eigenen Block: %v", err)
	}
}

// ProduceBlock nimmt die gewaehlte Blockzeit -- und laesst nichts mehr weg.
func TestProduceBlock_NimmtDieGewaehlteBlockzeit(t *testing.T) {
	body := functionBodyFromSource(t, "block.go", "func (dag *BlockDAG) ProduceBlock(")
	for _, muss := range []string{
		"anzahl, blockZeit, zeitErr := blockZeitPraefix(txs, jetztUnix, elternZeit)",
		"korbPraefix(korbGenommen, pendingTxIDs, korbZeilenPos, behalten, dag.state.korbBis.Load())",
		"dag.vorlauf.verwerfen(dag.state.PendingTxIDsFreigeben)\n\t\ttxs = txs[:anzahl]",
		"parent := dag.ghostdagBlockLookup(ph, nil)",
		"merkeProduktionsAusfall(\"auftraege_ohne_gemeinsame_zeit\")",
		"Timestamp:    blockZeit,",
		"dag.state.eigenerBlockGespeichert()",
		"dag.state.erzeugerSeit.CompareAndSwap(0,",
	} {
		if !strings.Contains(body, muss) {
			t.Fatalf("ProduceBlock enthaelt nicht mehr: %s", muss)
		}
	}
	if strings.Contains(body, "ohneUntauglicheAuftraege") {
		t.Fatal("ProduceBlock laesst wieder Auftraege weg")
	}
}

// Teilen statt alles oder nichts: ein alter Auftrag neben frischen (eine
// Rundenmarke ab dem 07.10.) -- der erste Block traegt den alten mit dessen
// Zeit, der naechste den Rest mit jetzt. Nichts wird weggelassen.
func TestBlockZeitPraefix_TeiltStattStillzustehen(t *testing.T) {
	mitSignaturpflicht(t)
	jetzt := rundenZeitStrengAbUnix + 30*86400
	alt := vormundAuftrag(t, jetzt-2*3600)
	marke := Transaction{Type: "distribution_round_marker", DistributionAt: jetzt}
	frisch := vormundAuftrag(t, jetzt-5)
	txs := []Transaction{alt, marke, frisch}
	eltern := jetzt - 3*3600
	if _, err := blockZeitFuer(txs, jetzt, eltern); err == nil {
		t.Fatal("Vorbedingung: zusammen gibt es keine gemeinsame Zeit")
	}
	k, t1, err := blockZeitPraefix(txs, jetzt, eltern)
	if err != nil || k != 1 {
		t.Fatalf("erster Block: %d Transaktionen, %v", k, err)
	}
	if _, err := pruefeAuftraegeImBlock(txs[:k], t1); err != nil {
		t.Fatalf("erster Block besteht nicht: %v", err)
	}
	k2, t2, err := blockZeitPraefix(txs[k:], jetzt, t1)
	if err != nil || k2 != 2 || t2 != jetzt {
		t.Fatalf("zweiter Block: %d Transaktionen bei %d, %v", k2, t2, err)
	}
	for i := range txs[k:] {
		if err := blockTauglich(&txs[k+i], t2); err != nil {
			t.Fatalf("zweiter Block besteht nicht: %v", err)
		}
	}
	// Passt schon der erste zu keiner Zeit seit den Eltern: kein Block, und
	// die Meldung nennt sein Fenster.
	_, _, err = blockZeitPraefix(txs, jetzt, jetzt-60)
	if err == nil || !strings.Contains(err.Error(), "vormund_setzen") {
		t.Fatalf("erster Auftrag ohne Zeit: %v", err)
	}
}

// BuchAt: der Nachspielende nimmt es nur bis 60 s nach und 7 Tage vor dem
// Block, sonst still die Blockzeit -- also gehoert es ins Fenster.
func TestAuftragsFenster_BuchAt(t *testing.T) {
	wirtschaftAktivOverride.Store(1)
	t.Cleanup(func() { wirtschaftAktivOverride.Store(math.MaxInt64) })
	basis := int64(1_800_000_000)
	tx := Transaction{Type: "transfer", Wallet: "0xa", To: "0xb", Amount: 1, BuchAt: basis}
	f := auftragsFenster(&tx, basis+30*86400)
	for d := int64(-9 * 86400); d <= 9*86400; d += 41 {
		ts := basis + d
		drin := ts >= f.von && ts <= f.bis
		if ok := blockTauglich(&tx, ts) == nil; drin != ok {
			t.Fatalf("t=basis%+d: im Fenster=%v, blockTauglich=%v", d, drin, ok)
		}
		if ok := buchZeitBeimNachspielen(tx.BuchAt, ts) == tx.BuchAt; drin != ok {
			t.Fatalf("t=basis%+d: Fenster=%v, Nachspielende nehmen BuchAt=%v", d, drin, ok)
		}
	}
}

// Haengt der Ausgang (aelteste offene Zeile ueber 10 min), haelt die Annahme
// an -- auch wenn kleine Bloecke weiterlaufen.
func TestAnnahmePausiert_AusgangHaengt(t *testing.T) {
	cs := newTestState()
	t.Cleanup(func() { aeltesteOffeneZeile.Store(0) })
	aeltesteOffeneZeile.Store(nowUnix() - ausgangHoechstensAlt - 5)
	if err := cs.annahmePauseGrund(); !errors.Is(err, ErrAnnahmePausiert) {
		t.Fatalf("haengender Ausgang: %v", err)
	}
	aeltesteOffeneZeile.Store(nowUnix() - 60)
	if err := cs.annahmePauseGrund(); err != nil {
		t.Fatalf("junger Ausgang: %v", err)
	}
	aeltesteOffeneZeile.Store(0)
	if err := cs.annahmePauseGrund(); err != nil {
		t.Fatalf("leerer Ausgang: %v", err)
	}
}

// Missbrauch des RPC-Wegs: waehrend der Startsperre darf eine Ueberweisung
// keine Nonce verbrauchen -- sie bekommt -32005 VOR der Reservierung.
func TestSendRawTransaction_PauseVorDerNonce(t *testing.T) {
	cs := newTestState()
	cs.ausgangVorStartBis.Store(7)
	noteBlockProduced()
	srv := NewEVMRPCServer(&BlockDAG{state: cs}, cs)
	raw, sender := signedRawHex(t, 0, testRecipientHex)
	params, _ := json.Marshal([]string{raw})
	var p []json.RawMessage
	json.Unmarshal(params, &p)
	_, rerr := srv.sendRawTransaction(p, nil)
	if rerr == nil || rerr.Code != -32005 {
		t.Fatalf("waehrend der Startsperre: %+v", rerr)
	}
	if n := srv.nonceShardFor(sender).nonces[sender]; n != 0 {
		t.Fatalf("Nonce verbraucht: %d", n)
	}
}
