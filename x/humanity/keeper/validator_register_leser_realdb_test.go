package keeper

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Schritt 3 mit echter Datenbank: Strafkonto, Erzeugerstand und Leitung aus
// dem Verlauf des Kettenregisters (validator_register_leser.go).

// verlaufEintrag: eine Bindung direkt in validator_verlauf (die Pruefung der
// Unterschriften ist Schritt 1 und dort getestet).
func verlaufEintrag(t *testing.T, cs *ChainState, betreiber, signing string, zeit int64) {
	t.Helper()
	if _, err := cs.db.Exec(`INSERT INTO validator_verlauf (operator_wallet, signing_address, bindung_ts, sig_operator, sig_signing)
		VALUES ($1, $2, $3, 'x', 'y') ON CONFLICT DO NOTHING`,
		strings.ToLower(betreiber), strings.ToLower(signing), zeit); err != nil {
		t.Fatal(err)
	}
}

// registerKonto: ein Konto mit 100 AEQ, Mensch oder nicht.
func registerKonto(t *testing.T, cs *ChainState, adresse string, mensch bool) {
	t.Helper()
	cs.mu.Lock()
	acc := &AccountState{Address: adresse, IsHuman: mensch, Balance: NewDecimal(100)}
	cs.accounts.Set(adresse, acc)
	err := cs.saveAccountToDB(acc)
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
}

// strafFall: registered_nodes nennt k.op (100 AEQ, wie bisher), der Verlauf
// einen anderen Betreiber reg (Mensch, 100 AEQ, seit zehn Tagen) -- beide
// fuer denselben Signierschluessel. Das erste Vergehen steht schon im Block.
// bindungsZeit: der Zeitpunkt, zu dem betreiber den Schluessel von k
// gebunden hat. "Umstritten" heisst dieselbe Sekunde -- ein zweites
// nowUnix() liegt unter -race auch mal eine Sekunde spaeter, und dann ist es
// eine Uebernahme (so zuvor zufaellig rot).
func bindungsZeit(k *strafKnoten, betreiber string) int64 {
	k.t.Helper()
	var ts int64
	if err := k.cs.db.QueryRow(`SELECT bindung_ts FROM validator_verlauf WHERE operator_wallet = $1 AND signing_address = $2`,
		strings.ToLower(betreiber), strings.ToLower(k.signer)).Scan(&ts); err != nil {
		k.t.Fatal(err)
	}
	return ts
}

func strafFall(t *testing.T) (*strafKnoten, string) {
	t.Helper()
	k := neuerStrafKnoten(t)
	for _, q := range []string{`DELETE FROM validator_register`, `DELETE FROM validator_verlauf`} {
		if _, err := k.cs.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	reg := distTestAddr(1701)
	registerKonto(t, k.cs, reg, true)
	verlaufEintrag(t, k.cs, reg, k.signer, nowUnix()-10*86400)
	if !k.block(k.doppelsignatur(nowUnix()-5*86400, "aa", "bb")) {
		t.Fatal("erstes Vergehen abgelehnt")
	}
	return k, reg
}

func standVon(t *testing.T, cs *ChainState, adresse string) float64 {
	t.Helper()
	var db float64
	if err := cs.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, adresse).Scan(&db); err != nil {
		t.Fatal(err)
	}
	return db
}

const strafe = 100 - equivocationSecondOffensePenaltyAEQ

// Vor dem Stichtag zahlt, wen registered_nodes nennt -- byte-gleich wie
// bisher, auch wenn der Verlauf etwas anderes sagt.
func TestStrafkonto_VorDemStichtagWieBisher_RealDB(t *testing.T) {
	k, reg := strafFall(t)
	if !k.block(k.doppelsignatur(nowUnix()-3600, "cc", "dd")) {
		t.Fatal("zweites Vergehen abgelehnt")
	}
	if got := standVon(t, k.cs, k.op); got != strafe {
		t.Fatalf("registered_nodes-Betreiber %.2f", got)
	}
	if got := standVon(t, k.cs, reg); got != 100 {
		t.Fatalf("Register-Betreiber vor dem Stichtag belastet: %.2f", got)
	}
}

// ------------------------------------------------------------ ab registerLeserAb:
// spaeter und fuer alle gleich abgerechnet (strafe_abrechnung.go)

func (k *strafKnoten) blockZu(zeit int64, txs ...Transaction) bool {
	k.t.Helper()
	k.n++
	b := &Block{Height: int64(k.n), Hash: fmt.Sprintf("strafe-%s-%d", k.t.Name(), k.n), Timestamp: zeit, Transactions: txs}
	return k.dag.replayTransactions(b, true)
}

// mitStichtag: registerLeserAb fuer die Dauer von f.
func mitStichtag(ab int64, f func()) {
	registerLeserOverride.Store(ab)
	defer registerLeserOverride.Store(0)
	f()
}

func abrechnung(beweis Transaction) Transaction {
	return Transaction{Type: "slash_abrechnung", Wallet: beweis.Wallet, BlockAHash: beweis.BlockAHash,
		BlockBHash: beweis.BlockBHash, DetectedAt: beweis.DetectedAt}
}

// zweitesVergehen: das zweite Vergehen zur Zeit tat, eine Minute danach im
// Block nachgespielt, mit registerLeserAb = 1. Bucht keine Geldstrafe.
func zweitesVergehen(t *testing.T, k *strafKnoten, tat int64) Transaction {
	t.Helper()
	tx := k.doppelsignatur(tat, "cc", "dd")
	var ok bool
	mitStichtag(1, func() { ok = k.blockZu(tat+60, tx) })
	if !ok {
		t.Fatal("zweites Vergehen abgelehnt")
	}
	return tx
}

// abrechnen: slash_abrechnung im Block zur Zeit zeit, mit registerLeserAb = 1.
func abrechnen(k *strafKnoten, beweis Transaction, zeit int64) (ok bool) {
	k.t.Helper()
	mitStichtag(1, func() { ok = k.blockZu(zeit, abrechnung(beweis)) })
	return ok
}

func (k *strafKnoten) strafeStand(beweis Transaction) (offen, erledigt bool) {
	k.t.Helper()
	a, b := beweis.BlockAHash, beweis.BlockBHash
	if a > b {
		a, b = b, a
	}
	if err := k.cs.db.QueryRow(`SELECT strafe_offen, slash_applied FROM equivocation_evidence WHERE block_a_hash = $1 AND block_b_hash = $2`,
		a, b).Scan(&offen, &erledigt); err != nil {
		k.t.Fatal(err)
	}
	return offen, erledigt
}

// Das zweite Vergehen bucht ab dem Stichtag nichts; slash_abrechnung bucht
// die Strafe fruehestens zur Faelligkeit, beim Halter aus dem Verlauf --
// registered_nodes lenkt sie nicht um. Eine zweite Abrechnung bucht nichts.
func TestStrafe_AbStichtagSpaeterAbgerechnet_RealDB(t *testing.T) {
	k, reg := strafFall(t)
	zweites := zweitesVergehen(t, k, nowUnix()-3600)
	if got := standVon(t, k.cs, reg); got != 100 {
		t.Fatalf("beim Vermerk belastet: %.2f", got)
	}
	if offen, erledigt := k.strafeStand(zweites); !offen || erledigt {
		t.Fatalf("offen=%v erledigt=%v nach dem zweiten Vergehen", offen, erledigt)
	}
	if gesperrt, _ := k.cs.IsValidatorSuspended(k.signer, 0); !gesperrt || k.vergehen() != 2 {
		t.Fatalf("Sperre %v, %d Vergehen", gesperrt, k.vergehen())
	}
	faellig := strafeFaelligAb(zweites.DetectedAt)
	if abrechnen(k, zweites, faellig-1) {
		t.Fatal("Abrechnung vor der Faelligkeit angenommen")
	}
	if got := standVon(t, k.cs, reg); got != 100 {
		t.Fatalf("nach der zu fruehen Abrechnung belastet: %.2f", got)
	}
	if !abrechnen(k, zweites, faellig) {
		t.Fatal("faellige Abrechnung abgelehnt")
	}
	if got := standVon(t, k.cs, reg); got != strafe {
		t.Fatalf("Halter aus dem Verlauf %.2f", got)
	}
	if got := standVon(t, k.cs, k.op); got != 100 {
		t.Fatalf("registered_nodes-Betreiber belastet: %.2f", got)
	}
	if !abrechnen(k, zweites, faellig+60) {
		t.Fatal("doppelte Abrechnung weist den Block ab")
	}
	if got := standVon(t, k.cs, reg); got != strafe {
		t.Fatalf("zweimal abgerechnet: %.2f", got)
	}
}

// M2 (#303): wer zahlt. Bindungen bis W nach der Tat zaehlen; der erste
// ANDERE Betreiber in dieser Zeit zahlt, sonst der Halter zur Tat. Kein
// Fall ist "keiner zahlt, weil ein anderer gebunden hat".
func TestStrafe_AbrechnungWerZahlt_RealDB(t *testing.T) {
	type fall struct {
		name    string
		vorher  func(k *strafKnoten, v, b, c string, d int64)
		zahlt   string // "v", "b", "c" oder "" (keine Geldstrafe)
		bMensch bool
	}
	faelle := []fall{
		{"Halter zur Tat", func(k *strafKnoten, v, b, c string, d int64) {}, "v", true},
		{"Uebernahme innerhalb W", func(k *strafKnoten, v, b, c string, d int64) {
			verlaufEintrag(k.t, k.cs, b, k.signer, d+1800)
		}, "b", true},
		{"Uebernahme genau am Ende von W", func(k *strafKnoten, v, b, c string, d int64) {
			verlaufEintrag(k.t, k.cs, b, k.signer, d+strafBeweisFrisch)
		}, "b", true},
		{"Uebernahme nach W", func(k *strafKnoten, v, b, c string, d int64) {
			verlaufEintrag(k.t, k.cs, b, k.signer, d+strafBeweisFrisch+1)
		}, "v", true},
		{"zwei Uebernahmen: der erste zahlt", func(k *strafKnoten, v, b, c string, d int64) {
			verlaufEintrag(k.t, k.cs, c, k.signer, d+1200)
			verlaufEintrag(k.t, k.cs, b, k.signer, d+600)
		}, "b", true},
		{"Uebernahme durch Nicht-Menschen", func(k *strafKnoten, v, b, c string, d int64) {
			verlaufEintrag(k.t, k.cs, b, k.signer, d+600)
		}, "", false},
		{"eigene Neubindung nach der Tat", func(k *strafKnoten, v, b, c string, d int64) {
			verlaufEintrag(k.t, k.cs, v, k.signer, d+600)
		}, "v", true},
		{"eigener Wechsel vor der Tat", func(k *strafKnoten, v, b, c string, d int64) {
			verlaufEintrag(k.t, k.cs, v, distTestAddr(1706), d-600)
		}, "v", true},
		{"Uebernahme im Augenblick der Tat", func(k *strafKnoten, v, b, c string, d int64) {
			verlaufEintrag(k.t, k.cs, b, k.signer, d)
		}, "b", true},
		{"umstritten", func(k *strafKnoten, v, b, c string, d int64) {
			verlaufEintrag(k.t, k.cs, c, k.signer, bindungsZeit(k, v))
		}, "", true},
		{"umstritten, dann uebernommen", func(k *strafKnoten, v, b, c string, d int64) {
			verlaufEintrag(k.t, k.cs, c, k.signer, bindungsZeit(k, v))
			verlaufEintrag(k.t, k.cs, b, k.signer, d+600)
		}, "b", true},
		{"keine Bindung", func(k *strafKnoten, v, b, c string, d int64) {
			k.cs.db.Exec(`DELETE FROM validator_verlauf`)
		}, "", true},
		{"erst nach der Tat gebunden, innerhalb W", func(k *strafKnoten, v, b, c string, d int64) {
			k.cs.db.Exec(`DELETE FROM validator_verlauf`)
			verlaufEintrag(k.t, k.cs, v, k.signer, d+1800)
		}, "v", true},
		{"erst nach W gebunden", func(k *strafKnoten, v, b, c string, d int64) {
			k.cs.db.Exec(`DELETE FROM validator_verlauf`)
			verlaufEintrag(k.t, k.cs, v, k.signer, d+strafBeweisFrisch+1)
		}, "", true},
	}
	for _, f := range faelle {
		k, v := strafFall(t)
		b, c := distTestAddr(1711), distTestAddr(1712)
		registerKonto(t, k.cs, b, f.bMensch)
		registerKonto(t, k.cs, c, true)
		zweites := zweitesVergehen(t, k, nowUnix()-3600)
		f.vorher(k, v, b, c, zweites.DetectedAt)
		if !abrechnen(k, zweites, strafeFaelligAb(zweites.DetectedAt)) {
			t.Fatalf("%s: Abrechnung abgelehnt", f.name)
		}
		for name, adr := range map[string]string{"v": v, "b": b, "c": c, "registered_nodes": k.op} {
			want := 100.0
			if name == f.zahlt {
				want = strafe
			}
			if got := standVon(t, k.cs, adr); got != want {
				t.Fatalf("%s: %s hat %.2f, erwartet %.2f", f.name, name, got, want)
			}
		}
		if _, erledigt := k.strafeStand(zweites); !erledigt {
			t.Fatalf("%s: nicht als abgerechnet vermerkt -- kaeme wieder in den Ausgang", f.name)
		}
		if gesperrt, _ := k.cs.IsValidatorSuspended(k.signer, 0); !gesperrt {
			t.Fatalf("%s: Sperre fehlt", f.name)
		}
	}
}

// M1 (#303): der Erkennende vermerkt, bevor er eine Uebergabe nachgespielt
// hat, die die anderen schon kennen -- oder danach. Abgerechnet wird erst
// spaeter, aus Bindungen bis W nach der Tat: in beiden Reihenfolgen zahlt
// derselbe. Vorher zahlte beim Erkennenden keiner, bei den anderen V.
func TestStrafe_ErkennenderUndNachspielendeRechnenGleich_RealDB(t *testing.T) {
	for _, wann := range []string{"Uebergabe vor der Erkennung bekannt", "Uebergabe erst danach bekannt"} {
		k, v := strafFall(t)
		b := distTestAddr(1713)
		registerKonto(t, k.cs, b, true)
		zweites := k.doppelsignatur(nowUnix()-600, "cc", "dd")
		if wann == "Uebergabe vor der Erkennung bekannt" {
			verlaufEintrag(t, k.cs, b, k.signer, zweites.DetectedAt+300)
		}
		var err error
		mitStichtag(1, func() {
			_, _, err = k.cs.DoppelsignaturErkannt(k.signer, zweites.BlockAHash, zweites.BlockBHash, zweites.DetectedAt, zweites.Doppelbeweis)
		})
		if err != nil {
			t.Fatalf("%s: %v", wann, err)
		}
		if got := standVon(t, k.cs, v) + standVon(t, k.cs, b); got != 200 {
			t.Fatalf("%s: beim Erkennen gebucht (%.2f)", wann, got)
		}
		if wann == "Uebergabe erst danach bekannt" {
			verlaufEintrag(t, k.cs, b, k.signer, zweites.DetectedAt+300)
		}
		if !abrechnen(k, zweites, strafeFaelligAb(zweites.DetectedAt)) {
			t.Fatalf("%s: Abrechnung abgelehnt", wann)
		}
		if got := standVon(t, k.cs, b); got != strafe {
			t.Fatalf("%s: B (Uebernahme innerhalb W) %.2f", wann, got)
		}
		if got := standVon(t, k.cs, v); got != 100 {
			t.Fatalf("%s: V belastet %.2f", wann, got)
		}
	}
}

// Frische (#303, M2): ab dem Stichtag steht ein Beweis hoechstens W nach der
// Tat in einem Block. Sonst haengte ein spaeterer Halter dem frueheren einen
// alten an. Vor dem Stichtag datiert und kurz danach getragen: alter Weg,
// aber nur innerhalb von W.
func TestStrafe_NurFrischeBeweise_RealDB(t *testing.T) {
	k, reg := strafFall(t)
	tat := nowUnix() - 3*3600
	zweites := k.doppelsignatur(tat, "cc", "dd")
	var ok bool
	mitStichtag(1, func() { ok = k.blockZu(zweites.DetectedAt+strafBeweisFrisch+1, zweites) })
	if ok {
		t.Fatal("alter Beweis angenommen")
	}
	if v := k.vergehen(); v != 1 {
		t.Fatalf("%d Vergehen nach der Abweisung", v)
	}
	mitStichtag(1, func() { ok = k.blockZu(zweites.DetectedAt+strafBeweisFrisch, zweites) })
	if !ok || k.vergehen() != 2 {
		t.Fatalf("frischer Beweis: angenommen %v, %d Vergehen", ok, k.vergehen())
	}

	// Vor den Stichtag datiert: innerhalb von W alter Weg (registered_nodes,
	// sofort), danach abgewiesen.
	k, reg = strafFall(t)
	jetzt := nowUnix()
	vorher := k.doppelsignatur(jetzt-3000, "cc", "dd")
	mitStichtag(jetzt-1800, func() { ok = k.blockZu(jetzt, vorher) })
	if !ok || standVon(t, k.cs, k.op) != strafe || standVon(t, k.cs, reg) != 100 {
		t.Fatalf("innerhalb W: angenommen %v, registered_nodes %.2f, Register %.2f", ok, standVon(t, k.cs, k.op), standVon(t, k.cs, reg))
	}
	k, _ = strafFall(t)
	alt := k.doppelsignatur(jetzt-2*3600, "cc", "dd")
	mitStichtag(jetzt-1800, func() { ok = k.blockZu(jetzt, alt) })
	if ok {
		t.Fatal("vor den Stichtag rueckdatierter alter Beweis nach dem Stichtag angenommen")
	}
}

// Missbrauch (Sicherheitspruefung #306): V legt einen Beweis mit DetectedAt
// in zwei Tagen in seinen eigenen Block und gibt den Schluessel morgen an B
// -- B hielte ihn "zur Tat". Der Block wird abgewiesen; V's Erkennung legt
// ihn gar nicht erst.
func TestStrafe_InDieZukunftDatierterBeweis_RealDB(t *testing.T) {
	k, v := strafFall(t)
	b := distTestAddr(1714)
	registerKonto(t, k.cs, b, true)
	jetzt := nowUnix()
	zukunft := k.doppelsignatur(jetzt+2*86400, "cc", "dd")
	verlaufEintrag(t, k.cs, b, k.signer, jetzt+86400)
	var ok bool
	mitStichtag(1, func() { ok = k.blockZu(jetzt, zukunft) })
	if ok {
		t.Fatal("in die Zukunft datierter Beweis angenommen")
	}
	k.cs.db.Exec(`DELETE FROM pending_txs`)
	mitStichtag(1, func() {
		k.cs.DoppelsignaturErkannt(k.signer, zukunft.BlockAHash, zukunft.BlockBHash, zukunft.DetectedAt, zukunft.Doppelbeweis)
	})
	var imAusgang int
	k.cs.db.QueryRow(`SELECT COUNT(*) FROM pending_txs WHERE tx_json LIKE '%slash_equivocation%'`).Scan(&imAusgang)
	if v := k.vergehen(); v != 1 || imAusgang != 0 {
		t.Fatalf("%d Vergehen, %d im Ausgang", v, imAusgang)
	}
	if standVon(t, k.cs, b) != 100 || standVon(t, k.cs, v) != 100 {
		t.Fatal("belastet")
	}
}

// Der Erkennende legt einen alten Beweis nicht mehr in den Ausgang -- er
// wuerde abgewiesen, und vermerkt sperrte er nur auf diesem Knoten.
func TestStrafe_ErkennenderUebergehtAltenBeweis_RealDB(t *testing.T) {
	k, reg := strafFall(t)
	k.cs.db.Exec(`DELETE FROM pending_txs`)
	zweites := k.doppelsignatur(nowUnix()-strafBeweisFrisch+strafBeweisMarge-60, "cc", "dd")
	var err error
	mitStichtag(1, func() {
		_, _, err = k.cs.DoppelsignaturErkannt(k.signer, zweites.BlockAHash, zweites.BlockBHash, zweites.DetectedAt, zweites.Doppelbeweis)
	})
	if err != nil {
		t.Fatal(err)
	}
	var imAusgang int
	k.cs.db.QueryRow(`SELECT COUNT(*) FROM pending_txs WHERE tx_json LIKE '%slash_equivocation%'`).Scan(&imAusgang)
	if v := k.vergehen(); v != 1 || imAusgang != 0 || standVon(t, k.cs, reg) != 100 {
		t.Fatalf("alter Beweis: %d Vergehen, %d im Ausgang", v, imAusgang)
	}
}

// Missbrauch: Abrechnungen, die jeder Knoten abweist -- ohne bekannten
// Beweis, ohne offene Strafe (erstes Vergehen), mit falschem Zeitpunkt, und
// bei unlesbarem Verlauf (fail-closed, nie "keine Strafe").
func TestStrafe_UngueltigeAbrechnungWeistAb_RealDB(t *testing.T) {
	k, reg := strafFall(t)
	zweites := zweitesVergehen(t, k, nowUnix()-3600)
	faellig := strafeFaelligAb(zweites.DetectedAt)

	unbekannt := k.doppelsignatur(nowUnix()-3600, "ee", "ff")
	erstes := k.doppelsignatur(nowUnix()-5*86400, "aa", "bb") // erstes Vergehen, keine Geldstrafe
	falsch := zweites
	falsch.DetectedAt--
	for name, tx := range map[string]Transaction{"unbekannter Beweis": unbekannt, "erstes Vergehen": erstes, "falscher Zeitpunkt": falsch} {
		if abrechnen(k, tx, faellig+86400*6) {
			t.Fatalf("%s: Abrechnung angenommen", name)
		}
	}
	if _, err := k.cs.db.Exec(`ALTER TABLE validator_verlauf RENAME TO validator_verlauf_weg`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.cs.db.Exec(`ALTER TABLE validator_verlauf_weg RENAME TO validator_verlauf`) })
	ok := abrechnen(k, zweites, faellig)
	if _, err := k.cs.db.Exec(`ALTER TABLE validator_verlauf_weg RENAME TO validator_verlauf`); err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("Abrechnung trotz unlesbarem Verlauf angenommen")
	}
	if offen, erledigt := k.strafeStand(zweites); !offen || erledigt || standVon(t, k.cs, reg) != 100 {
		t.Fatalf("nach der Abweisung offen=%v erledigt=%v, Halter %.2f", offen, erledigt, standVon(t, k.cs, reg))
	}
	if !abrechnen(k, zweites, faellig) || standVon(t, k.cs, reg) != strafe {
		t.Fatal("Abrechnung danach nicht gebucht")
	}
}

// Der Leiter legt faellige Strafen in den Ausgang und bucht sie dabei, genau
// einmal. Ein Knoten ohne Annahme legt nichts.
func TestStrafe_LeiterLegtAbrechnungInDenAusgang_RealDB(t *testing.T) {
	k, reg := strafFall(t)
	k.cs.db.Exec(`DELETE FROM pending_txs`)
	tat := nowUnix() - strafBeweisFrisch - erzeugerFrist - strafAbrechnungMarge - 600
	zweites := zweitesVergehen(t, k, tat)
	ausgang := func() int {
		var n int
		k.cs.db.QueryRow(`SELECT COUNT(*) FROM pending_txs WHERE tx_json LIKE '%slash_abrechnung%'`).Scan(&n)
		return n
	}
	mitStichtag(1, k.cs.strafAbrechnungLauf)
	if n := ausgang(); n != 0 || standVon(t, k.cs, reg) != 100 {
		t.Fatalf("ohne Annahme: %d im Ausgang, Halter %.2f", n, standVon(t, k.cs, reg))
	}
	k.cs.annehmendAusdruecklich.Store(true)
	t.Cleanup(func() { k.cs.annehmendAusdruecklich.Store(false) })
	mitStichtag(1, k.cs.strafAbrechnungLauf)
	mitStichtag(1, k.cs.strafAbrechnungLauf)
	if n := ausgang(); n != 1 {
		t.Fatalf("%d Abrechnungen im Ausgang, erwartet 1", n)
	}
	if got := standVon(t, k.cs, reg); got != strafe {
		t.Fatalf("Halter nach der Abrechnung %.2f", got)
	}
	// Ein anderer Erzeuger hat dieselbe Abrechnung gelegt: kein zweiter Abzug.
	if !abrechnen(k, zweites, nowUnix()) || standVon(t, k.cs, reg) != strafe {
		t.Fatalf("Duplikat: Halter %.2f", standVon(t, k.cs, reg))
	}
}

// Die Selbstheilung fuer den BOOTSTRAP_SIGNER haelt ein zweites Vergehen,
// dessen Strafe noch offen ist, fuer bestaetigt -- slash_applied ist dann
// noch falsch.
func TestStrafe_OffeneStrafeIstBestaetigt_RealDB(t *testing.T) {
	k, _ := strafFall(t)
	zweitesVergehen(t, k, nowUnix()-3600)
	t.Setenv("BOOTSTRAP_SIGNER", k.signer)
	k.cs.selfHealUncorroboratedSeedSuspension()
	k.cs.invalidatePenaltyCache()
	if v := k.vergehen(); v != 2 {
		t.Fatalf("Selbstheilung loeschte ein bestaetigtes Vergehen (%d)", v)
	}
}

// Vor dem Stichtag (registered_nodes): aendert sich das Strafkonto zwischen
// der Vorab-Lesung und der Transaktion, versucht der Erkennende es einmal
// neu -- vorher fiel die ganze Erkennung weg (LOW 1, #303).
func TestDoppelsignaturErkannt_StrafkontoAendertSichWaehrenddessen_RealDB(t *testing.T) {
	k, _ := strafFall(t)
	neu := distTestAddr(1708)
	registerKonto(t, k.cs, neu, true)
	aufrufe := 0
	strafkontoVorabGelesen = func() {
		aufrufe++
		if aufrufe == 1 {
			if _, err := k.cs.db.Exec(`UPDATE registered_nodes SET wallet_address = $1 WHERE signing_address = $2`, neu, k.signer); err != nil {
				t.Error(err)
			}
		}
	}
	t.Cleanup(func() { strafkontoVorabGelesen = nil })
	zweites := k.doppelsignatur(nowUnix()-3600, "cc", "dd")
	_, betrag, err := k.cs.DoppelsignaturErkannt(k.signer, zweites.BlockAHash, zweites.BlockBHash, zweites.DetectedAt, zweites.Doppelbeweis)
	if err != nil {
		t.Fatalf("Erkennung verworfen: %v", err)
	}
	if aufrufe != 2 {
		t.Fatalf("%d Versuche, erwartet 2", aufrufe)
	}
	if betrag != equivocationSecondOffensePenaltyAEQ || standVon(t, k.cs, neu) != strafe || standVon(t, k.cs, k.op) != 100 {
		t.Fatalf("Strafe %.2f, neu %.2f, alt %.2f", betrag, standVon(t, k.cs, neu), standVon(t, k.cs, k.op))
	}
	if v := k.vergehen(); v != 2 {
		t.Fatalf("%d Vergehen, erwartet 2", v)
	}
	if n := k.beweise(zweites); n != 1 {
		t.Fatalf("%d Beweise, erwartet 1", n)
	}
}

// Der Erzeugerstand aus dem Verlauf: Zeitraeume menschlicher Betreiber mit
// Frist; geschlossen nur die Schluessel der Liste. Ein Lesefehler schliesst
// die Erzeugerpruefung, die Leitung behaelt den letzten Stand.
func TestErzeugerAusVerlauf_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	for _, q := range []string{`DELETE FROM validator_register`, `DELETE FROM validator_verlauf`} {
		if _, err := f.cs.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	m := func(n int) string { return distTestAddr(1800 + n) }
	s := func(n int) string { return distTestAddr(1850 + n) }
	for i, mensch := range []bool{true, true, true, true, false} {
		registerKonto(t, f.cs, m(i), mensch)
	}
	jetzt := nowUnix()
	lange := jetzt - 30*86400
	verlaufEintrag(t, f.cs, m(0), s(0), lange) // gilt
	verlaufEintrag(t, f.cs, m(1), s(1), lange) // gewechselt ...
	verlaufEintrag(t, f.cs, m(1), s(5), jetzt) // ... gerade eben (Frist laeuft)
	verlaufEintrag(t, f.cs, m(2), s(2), lange) // umstritten ...
	verlaufEintrag(t, f.cs, m(3), s(2), lange) // ... mit diesem
	verlaufEintrag(t, f.cs, m(4), s(4), lange) // kein Mensch

	dag := newOrphanTestDAG()
	dag.state = f.cs
	erzeugerSchnittOverride.Store(1)
	t.Cleanup(func() { erzeugerSchnittOverride.Store(0) })
	f.cs.erzeugerRegisterAuffrischen()
	for addr, want := range map[string]bool{s(0): true, s(1): true, s(5): false, s(2): false, s(4): false} {
		if got := dag.erzeugerNachRegister(addr, jetzt); got != want {
			t.Fatalf("offen, %s jetzt: %v, erwartet %v", addr, got, want)
		}
	}
	if !dag.erzeugerNachRegister(s(5), jetzt+erzeugerFrist) || dag.erzeugerNachRegister(s(1), jetzt+erzeugerFrist) {
		t.Fatal("nach der Frist: Wechsel von s1 zu s5 nicht vollzogen")
	}
	// Leitung: wer den Schluessel zuletzt gebunden hat (auch in der Frist und
	// nach einem Wechsel), nur Menschen, nicht umstritten.
	for addr, want := range map[string]string{s(0): m(0), s(1): m(1), s(5): m(1), s(2): "", s(4): ""} {
		if got := dag.menschAusRegister(addr); got != want {
			t.Fatalf("Mensch zu %s: %q, erwartet %q", addr, got, want)
		}
	}

	// Geschlossen: nur die Schluessel der Liste. s5 wird mitgelesen (sein
	// Betreiber m1 haelt s1, das auf der Liste steht), bekommt aber kein
	// Fenster.
	fest := []string{s(0), s(1), s(4)}
	f.cs.erzeugerFest.Store(&fest)
	t.Cleanup(func() { f.cs.erzeugerFest.Store(nil) })
	f.cs.erzeugerRegisterAuffrischen()
	st := f.cs.erzeugerRegister.Load()
	if st == nil || st.fehler != nil || len(st.fenster) != 2 || len(st.fenster[s(0)]) != 1 || len(st.fenster[s(1)]) != 1 || st.fenster[s(5)] != nil {
		t.Fatalf("geschlossen: %+v", st)
	}
	// Auch fuer die Leitung nur die Schluessel der Liste -- fuer s5 sind
	// nicht alle Zeilen gelesen, ein Mensch daraus koennte falsch sein.
	if _, da := st.halter[s(5)]; da || st.halter[s(1)] != m(1) {
		t.Fatalf("geschlossen, Halter: %v", st.halter)
	}

	// Ein Lesefehler: die Erzeugerpruefung schliesst ab, die Leitung behaelt
	// den letzten Stand.
	if _, err := f.cs.db.Exec(`ALTER TABLE validator_verlauf RENAME TO validator_verlauf_weg`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.cs.db.Exec(`ALTER TABLE validator_verlauf_weg RENAME TO validator_verlauf`) })
	f.cs.erzeugerRegisterAuffrischen()
	if st := f.cs.erzeugerRegister.Load(); st == nil || st.fehler == nil {
		t.Fatalf("Lesefehler nicht im Stand: %+v", st)
	}
	if dag.erzeugerNachRegister(s(0), jetzt) {
		t.Fatal("Erzeugerpruefung mit Lesefehler offen")
	}
	if got := dag.menschAusRegister(s(0)); got != m(0) {
		t.Fatalf("Leitung nach dem Lesefehler: %q", got)
	}
}

// Die eigene Annahme einer Bindung frischt den Erzeugerstand auf.
func TestValidatorBinden_FrischtErzeugerAuf_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	f.cs.annehmendAusdruecklich.Store(true)
	t.Cleanup(func() { f.cs.annehmendAusdruecklich.Store(false) })
	betreiber := f.betreiber()
	knoten, signing := neuerSchluessel(t)
	zeit := nowUnix()
	if err := f.cs.ValidatorBinden(bindungUnterschrieben(t, betreiber, knoten, zeit)); err != nil {
		t.Fatal(err)
	}
	st := f.cs.erzeugerRegister.Load()
	if st == nil || st.fehler != nil || st.erzeugerFenster(signing, zeit+erzeugerFrist) != adrVon(betreiber) {
		t.Fatalf("Erzeugerstand nach der Annahme: %+v", st)
	}
	if f.cs.registerGeaendert.Load() {
		t.Fatal("Merker nach dem Auffrischen nicht zurueckgesetzt")
	}
}

// LOW 4 (#303): das Nachspielen frischt den Stand nur auf, wenn der Block
// den Verlauf erweitert hat -- eine Bindung, die als Zustandsablehnung
// uebersprungen wird, loest keine Abfrage aus.
func TestReplay_FrischtNurBeiNeuerZeileAuf_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	merker := &erzeugerStand{zeit: time.Unix(1, 0)}
	f.cs.erzeugerRegister.Store(merker)
	f.cs.registerGeaendert.Store(false)
	keinMensch, _ := neuerSchluessel(t)
	k1, _ := neuerSchluessel(t)
	if !f.block(f.jetzt, bindungUnterschrieben(t, keinMensch, k1, f.jetzt-60)) {
		t.Fatal("Block abgewiesen")
	}
	if f.cs.erzeugerRegister.Load() != merker {
		t.Fatal("uebersprungene Bindung hat den Stand neu lesen lassen")
	}
	betreiber := f.betreiber()
	k2, signing := neuerSchluessel(t)
	if !f.block(f.jetzt, bindungUnterschrieben(t, betreiber, k2, f.jetzt-60)) {
		t.Fatal("Block abgewiesen")
	}
	st := f.cs.erzeugerRegister.Load()
	if st == merker || st.erzeugerFenster(signing, f.jetzt-60+erzeugerFrist) != adrVon(betreiber) {
		t.Fatalf("neue Bindung nicht im Stand: %+v", st)
	}
}

// Eine Bindung eines ANDEREN Schluessels im selben Augenblick macht den
// Halter nicht umstritten -- es zaehlen nur die Zeilen dieses Schluessels.
func TestStrafkonto_GleicherZeitpunktAndererSchluessel_RealDB(t *testing.T) {
	k, v := strafFall(t)
	var zeit int64
	if err := k.cs.db.QueryRow(`SELECT bindung_ts FROM validator_verlauf WHERE signing_address = $1`, k.signer).Scan(&zeit); err != nil {
		t.Fatal(err)
	}
	fremd := distTestAddr(1710)
	registerKonto(t, k.cs, fremd, true)
	verlaufEintrag(t, k.cs, fremd, distTestAddr(1711), zeit)
	zweites := zweitesVergehen(t, k, nowUnix()-3600)
	if !abrechnen(k, zweites, strafeFaelligAb(zweites.DetectedAt)) {
		t.Fatal("Abrechnung abgelehnt")
	}
	if got := standVon(t, k.cs, v); got != strafe {
		t.Fatalf("Halter nicht belastet: %.2f", got)
	}
}

// Ein Block mit einer Abrechnung, der scheitert, bucht nichts -- auch nicht
// im Speicher: das Strafkonto steht nicht im Block und muss vor dem Abzug
// in die Ruecknahme (kontoNachtragenLocked).
func TestStrafe_ZurueckgewiesenerBlockMitAbrechnungBuchtNicht_RealDB(t *testing.T) {
	k, reg := strafFall(t)
	zweites := zweitesVergehen(t, k, nowUnix()-3600)
	var ok bool
	mitStichtag(1, func() { ok = k.blockZu(strafeFaelligAb(zweites.DetectedAt), abrechnung(zweites), gift()) })
	if ok {
		t.Fatal("Block mit unbekannter Transaktion angenommen")
	}
	if mem, db := stand(k.cs, reg), standVon(t, k.cs, reg); mem != 100 || db != 100 {
		t.Fatalf("nach dem Zurueckrollen: Speicher %.2f, Datenbank %.2f", mem, db)
	}
	if offen, erledigt := k.strafeStand(zweites); !offen || erledigt {
		t.Fatalf("nach dem Zurueckrollen offen=%v erledigt=%v", offen, erledigt)
	}
}
