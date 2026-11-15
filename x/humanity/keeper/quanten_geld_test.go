package keeper

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"encoding/hex"
	"math"
	"math/big"
	"strings"
	"testing"
	"time"
)

// Dezentrales Quantengeld, Kettenseite (quanten_geld.go): Freischaltung durch
// zwei Drittel der Pruefer, Anmelden einer Note an den Topf, Form, Stichtag,
// StateRoot -- Gutfall und Angriffe.

const qgAb int64 = 1_800_000_000

// qgSatzN: eine Gruppenordnung >= 2^256 (wie bei 8.192-Bit-Primzahlen ~2^4096).
var qgSatzN = new(big.Int).Lsh(big.NewInt(1), 4096)

func qgAktiv(t *testing.T) {
	t.Helper()
	vorher := quantenGeldAbOverride.Load()
	quantenGeldAbOverride.Store(qgAb)
	t.Cleanup(func() { quantenGeldAbOverride.Store(vorher) })
}

func TestQuantenGeld_SchlafendOhneWirkung(t *testing.T) {
	if quantenGeldAktiv(time.Now().Unix()) && quantenGeldAbUnix == math.MaxInt64 {
		t.Fatal("aktiv trotz MaxInt64")
	}
	if quantenGeldAbUnix == math.MaxInt64 && bekannteTxArt(quantenFreischaltungTxArt, math.MaxInt64-1) {
		t.Fatal("Freischaltung bekannt, obwohl schlafend")
	}
	// Leeres Register: StateRoot byte-gleich, nichts in chain_config.
	r := &quantenGeldRegister{}
	if r.stateRootTeil() != "" || r.kodiert() != "" {
		t.Fatal("leeres Register veraendert StateRoot oder Speicher")
	}
	cs := newTestState()
	vorher := cs.StateRoot()
	cs.quantenGeld.Store(&quantenGeldRegister{})
	if cs.StateRoot() != vorher {
		t.Fatal("leeres Register aendert die StateRoot")
	}
}

func TestQuantenGeld_TopfOhneSchluesselUndGeschuetzt(t *testing.T) {
	if !kanonischeAdresse(quantenTopfAddr) {
		t.Fatalf("Topf %q nicht kanonisch", quantenTopfAddr)
	}
	if !isTokenomicsPoolAddress(quantenTopfAddr) {
		t.Fatal("Topf ist kein Topf (Demurrage/Grenze/Absender)")
	}
	if pruefeAbsenderKeinTopf(quantenTopfAddr) == nil {
		t.Fatal("aus dem Topf kann man mit Schluessel zahlen")
	}
	for _, p := range []string{validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr} {
		if p == quantenTopfAddr {
			t.Fatal("Topf faellt mit einem anderen Topf zusammen")
		}
	}
}

func TestQuantenGeld_OrdnungNurKryptografisch(t *testing.T) {
	klein := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	for _, s := range []string{"", "1491", klein.String(), "0" + qgSatzN.String(), "12a", "-5",
		new(big.Int).Add(quantenOrdnungMax, big.NewInt(1)).String()} {
		if _, ok := quantenOrdnungGueltig(s); ok {
			t.Errorf("Ordnung %.20q angenommen", s)
		}
	}
	for _, n := range []*big.Int{quantenOrdnungMin, qgSatzN, quantenOrdnungMax} {
		if _, ok := quantenOrdnungGueltig(n.String()); !ok {
			t.Errorf("Ordnung 2^%d abgewiesen", n.BitLen()-1)
		}
	}
}

func TestQuantenGeld_FeldNurWoErlaubt(t *testing.T) {
	q := &QuantenAngaben{Satz: "isogenie-8192"}
	// Vor dem Stichtag: jede Quanten-Angabe macht den Block ungueltig.
	if quantenGeldAbUnix == math.MaxInt64 {
		if quantenFeldErlaubt(&Transaction{Type: "transfer", To: quantenTopfAddr, Quanten: q}, 2_000_000_000) == nil {
			t.Fatal("Quanten-Angabe vor dem Stichtag angenommen")
		}
	}
	qgAktiv(t)
	z := qgAb + 10
	faelle := []struct {
		name string
		tx   Transaction
		ok   bool
	}{
		{"Anmeldung an den Topf", Transaction{Type: "transfer", To: quantenTopfAddr, Quanten: q}, true},
		{"Freischaltung", Transaction{Type: quantenFreischaltungTxArt, Quanten: q}, true},
		{"gewoehnliche Ueberweisung", Transaction{Type: "transfer", To: "0x00000000000000000000000000000000000000b2"}, true},
		{"Angabe an fremder Ueberweisung", Transaction{Type: "transfer", To: "0x00000000000000000000000000000000000000b2", Quanten: q}, false},
		{"Topf ohne Note (Deckung waere verloren)", Transaction{Type: "transfer", To: quantenTopfAddr}, false},
		{"Angabe an anderer Art", Transaction{Type: "swap_aeq_tusd", Quanten: q}, false},
	}
	for _, f := range faelle {
		if err := quantenFeldErlaubt(&f.tx, z); (err == nil) != f.ok {
			t.Errorf("%s: %v", f.name, err)
		}
	}
	if quantenFeldErlaubt(&Transaction{Type: "transfer", To: quantenTopfAddr, Quanten: q}, qgAb-1) == nil {
		t.Fatal("vor dem Stichtag angenommen")
	}
	if !bekannteTxArt(quantenFreischaltungTxArt, z) || bekannteTxArt(quantenFreischaltungTxArt, qgAb-1) {
		t.Fatal("Freischaltung nicht genau ab dem Stichtag bekannt")
	}
}

func TestQuantenGeld_RegisterNurKanonisch(t *testing.T) {
	gut := &quantenGeldRegister{
		Saetze: []quantenSatzEintrag{{Name: "isogenie-8192", Ordnung: qgSatzN.String(), Ab: qgAb, Seit: qgAb - 1}},
		Noten:  []quantenNoteEintrag{{K: strings.Repeat("a", 64), Satz: "isogenie-8192", Von: qnAnna, Mikro: 1_000_000}},
	}
	r, err := quantenGeldRegisterLesen(gut.kodiert())
	if err != nil || r.kodiert() != gut.kodiert() {
		t.Fatalf("Gutfall: %v", err)
	}
	schlecht := []string{
		`{"saetze":[{"name":"x","ordnung":"1491","ab":1,"seit":1}]}`,
		`{"unbekannt":1}`,
		`{"saetze":[{"name":"b","ordnung":"` + qgSatzN.String() + `","ab":1,"seit":1},{"name":"a","ordnung":"` + qgSatzN.String() + `","ab":1,"seit":1}]}`,
		`{"noten":[{"k":"` + strings.Repeat("a", 64) + `","satz":"nirgends","von":"` + qnAnna + `","mikro":1000000}]}`,
		`{"saetze":[{"name":"s","ordnung":"` + qgSatzN.String() + `","ab":1,"seit":1}],"noten":[{"k":"` + strings.Repeat("a", 64) + `","satz":"s","von":"` + qnAnna + `","mikro":5}]}`,
	}
	for _, s := range schlecht {
		if _, err := quantenGeldRegisterLesen(s); err == nil {
			t.Errorf("angenommen: %.90s", s)
		}
	}
}

// ---- Freischaltung ----

func qgStimme(f *vrFall, von int, satz, ordnung string, ab, zeit int64) VerifierStimme {
	b := f.menschen[von]
	sig := ed25519.Sign(b.priv, []byte(quantenFreischaltungNachricht(satz, ordnung, ab)("", zeit)))
	return VerifierStimme{Zeit: zeit, Bescheinigung: Lebendigkeitsbescheinigung{PublicKey: b.pubHex,
		Signature: hex.EncodeToString(sig), Mensch: b.bindung.Mensch, MenschSig: b.bindung.MenschSig, SchluesselSig: b.bindung.SchluesselSig}}
}

func qgFreischaltung(f *vrFall, satz, ordnung string, ab, zeit int64, von ...int) Transaction {
	q := &QuantenAngaben{Satz: satz, Ordnung: ordnung, Ab: ab}
	for _, i := range von {
		q.Stimmen = append(q.Stimmen, qgStimme(f, i, satz, ordnung, ab, zeit))
	}
	return Transaction{Type: quantenFreischaltungTxArt, Wallet: f.m(0), DistributionAt: zeit, Quanten: q}
}

func qgUrteil(t *testing.T, cs *ChainState, tx Transaction, blockZeit int64) quantenUrteil {
	t.Helper()
	cs.mu.Lock()
	defer cs.mu.Unlock()
	u, err := cs.quantenFreischaltungUrteilLocked(context.Background(), &tx, blockZeit)
	if err != nil {
		t.Fatalf("Fehler statt Befund: %v", err)
	}
	return u
}

// Drei Pruefer: zwei Stimmen noetig. Eine reicht nicht, zwei schon.
func TestQuantenGeld_FreischaltungZweiDrittel(t *testing.T) {
	f := neuerVrFall(t)
	f.aufnehmen(1, vrAb+100, 0)
	t2 := vrAb + 100 + erzeugerFrist + 10 // 1 ist erst nach der Frist Pruefer
	f.aufnehmen(2, t2, 0, 1)
	qgAktiv(t)
	zeit := t2 + erzeugerFrist + 1000
	ab := zeit + quantenFreischaltungVorlauf
	n := qgSatzN.String()
	if u := qgUrteil(t, f.cs, qgFreischaltung(f, "isogenie-8192", n, ab, zeit, 0), zeit); u.regel != "quanten_quorum" {
		t.Fatalf("eine von drei Stimmen: %+v", u)
	}
	tx := qgFreischaltung(f, "isogenie-8192", n, ab, zeit, 0, 1)
	if u := qgUrteil(t, f.cs, tx, zeit); u.regel != "" {
		t.Fatalf("zwei von drei Stimmen abgewiesen: %s %s", u.regel, u.text)
	}
	f.cs.mu.Lock()
	err := f.cs.applyQuantenFreischaltungLocked(context.Background(), &tx, zeit)
	f.cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	reg := f.cs.quantenGeldJetzt()
	if _, ok := reg.satz("isogenie-8192"); !ok || len(reg.saetzeZu(ab-1)) != 0 || len(reg.saetzeZu(ab)) != 1 {
		t.Fatal("Satz nicht genau ab ab gueltig")
	}
	// StateRoot traegt das Register jetzt.
	if reg.stateRootTeil() == "" {
		t.Fatal("Register nicht in der StateRoot")
	}
	// Derselbe Satz kein zweites Mal.
	if u := qgUrteil(t, f.cs, tx, zeit); u.regel != "quanten_satz_da" {
		t.Fatalf("Satz doppelt: %+v", u)
	}
}

func TestQuantenGeld_FreischaltungAngriffe(t *testing.T) {
	f := neuerVrFall(t)
	f.aufnehmen(1, vrAb+100, 0)
	qgAktiv(t)
	zeit := vrAb + 100 + erzeugerFrist + 1000
	ab := zeit + quantenFreischaltungVorlauf
	n := qgSatzN.String()
	klein := "1491"
	faelle := []struct {
		name  string
		tx    Transaction
		regel string
	}{
		{"Spielzeuggruppe", qgFreischaltung(f, "isogenie-8192", klein, ab, zeit, 0, 1), "quanten_ordnung"},
		{"Laborsatz", qgFreischaltung(f, "labor-h1491", n, ab, zeit, 0, 1), "quanten_form"},
		{"ohne Vorlauf", qgFreischaltung(f, "isogenie-8192", n, zeit+60, zeit, 0, 1), "quanten_vorlauf"},
		{"Nicht-Pruefer stimmt", qgFreischaltung(f, "isogenie-8192", n, ab, zeit, 0, 3), "quanten_stimme"},
		{"zweimal derselbe", qgFreischaltung(f, "isogenie-8192", n, ab, zeit, 0, 0), "quanten_stimme"},
		{"ohne Stimme", qgFreischaltung(f, "isogenie-8192", n, ab, zeit), "quanten_form"},
	}
	for _, c := range faelle {
		if u := qgUrteil(t, f.cs, c.tx, zeit); u.regel != c.regel {
			t.Errorf("%s: %q (%s), erwartet %q", c.name, u.regel, u.text, c.regel)
		}
	}
	// Stimme fuer einen anderen Satz / eine andere Ordnung / ein anderes ab
	// zaehlt nicht (die Unterschrift gilt nur fuer genau ihren Satz).
	tx := qgFreischaltung(f, "isogenie-8192", n, ab, zeit, 0, 1)
	tx.Quanten.Ab = ab + 1
	if u := qgUrteil(t, f.cs, tx, zeit); u.regel != "quanten_stimme" {
		t.Errorf("umgeschriebenes ab angenommen: %+v", u)
	}
	tx = qgFreischaltung(f, "isogenie-8192", n, ab, zeit, 0, 1)
	tx.Quanten.Ordnung = new(big.Int).Add(qgSatzN, big.NewInt(1)).String()
	if u := qgUrteil(t, f.cs, tx, zeit); u.regel != "quanten_stimme" {
		t.Errorf("umgeschriebene Ordnung angenommen: %+v", u)
	}
	// Blockzeit weit nach dem Beschluss.
	tx = qgFreischaltung(f, "isogenie-8192", n, ab, zeit, 0, 1)
	if u := qgUrteil(t, f.cs, tx, zeit+nachweisHoechstensAlt+1); u.regel != "quanten_zeit" {
		t.Errorf("alter Beschluss angenommen: %+v", u)
	}
	if len(f.cs.quantenGeldJetzt().Saetze) != 0 {
		t.Fatal("abgewiesene Freischaltung hat das Register veraendert")
	}
}

// ---- Anmelden ----

type qgPraegeFall struct {
	t      *testing.T
	cs     *ChainState
	key    *ecdsa.PrivateKey
	wallet string
}

func neuerQgPraegeFall(t *testing.T) *qgPraegeFall {
	t.Helper()
	qgAktiv(t)
	f := &qgPraegeFall{t: t, cs: newTestState()}
	f.key, f.wallet = neuerSchluessel(t)
	addHuman(f.cs, f.wallet, 1000)
	f.cs.quantenGeld.Store((&quantenGeldRegister{}).mitSatz(quantenSatzEintrag{Name: "isogenie-8192", Ordnung: qgSatzN.String(), Ab: qgAb + 100, Seit: qgAb}))
	return f
}

func (f *qgPraegeFall) tx(satz, serie, txHash string, aeq float64) Transaction {
	return Transaction{Type: "transfer", Wallet: f.wallet, To: quantenTopfAddr, Amount: aeq, TxHash: txHash,
		Quanten: &QuantenAngaben{Satz: satz, Serie: serie, Sig: personalSign(f.t, f.key, quantenPraegenNachricht(f.wallet, satz, serie, txHash))}}
}

func (f *qgPraegeFall) urteil(tx Transaction, z int64) quantenUrteil {
	f.cs.mu.Lock()
	defer f.cs.mu.Unlock()
	return f.cs.quantenPraegenUrteilLocked(&tx, z)
}

func TestQuantenGeld_AnmeldenGutfall(t *testing.T) {
	f := neuerQgPraegeFall(t)
	z := qgAb + 200
	tx := f.tx("isogenie-8192", "04d2", "0x"+strings.Repeat("1", 64), 300)
	u := f.urteil(tx, z)
	if u.regel != "" {
		t.Fatalf("%s: %s", u.regel, u.text)
	}
	if u.note.K != quantenSerieSchluessel("isogenie-8192", f.wallet, []byte{0x04, 0xd2}) || u.note.Mikro != 300_000_000 {
		t.Fatalf("Eintrag falsch: %+v", u.note)
	}
	f.cs.mu.Lock()
	err := f.cs.quantenNoteEintragenLocked(context.Background(), u.note)
	f.cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	st := f.cs.QuantenStand(u.note.K)
	if st["noten"].(int) != 1 || !st["note"].(map[string]interface{})["angemeldet"].(bool) {
		t.Fatalf("Stand: %v", st)
	}
	// Dieselbe Note kein zweites Mal (auch mit neuer Ueberweisung).
	if u := f.urteil(f.tx("isogenie-8192", "04d2", "0x"+strings.Repeat("2", 64), 5), z); u.regel != "quanten_doppelt" {
		t.Fatalf("doppelt: %+v", u)
	}
}

func TestQuantenGeld_AnmeldenAngriffe(t *testing.T) {
	f := neuerQgPraegeFall(t)
	z := qgAb + 200
	h := "0x" + strings.Repeat("3", 64)
	n := new(big.Int).Set(qgSatzN)
	nHex := n.Text(16)
	if len(nHex)%2 == 1 {
		nHex = "0" + nHex
	}
	faelle := []struct {
		name  string
		tx    Transaction
		blk   int64
		regel string
	}{
		{"Satz gilt noch nicht", f.tx("isogenie-8192", "04d2", h, 5), qgAb + 50, "quanten_satz"},
		{"unbekannter Satz", f.tx("labor-h1491", "04d2", h, 5), z, "quanten_satz"},
		{"Seriennummer >= N", f.tx("isogenie-8192", nHex, h, 5), z, "quanten_satz"},
		{"fuehrende Null", f.tx("isogenie-8192", "0004d2", h, 5), z, "quanten_satz"},
		{"Grossbuchstaben", f.tx("isogenie-8192", "04D2", h, 5), z, "quanten_form"},
		{"unter Mindestdeckung", f.tx("isogenie-8192", "04d2", h, 0.5), z, "quanten_deckung"},
		{"Deckung NaN", f.tx("isogenie-8192", "04d2", h, math.NaN()), z, "quanten_deckung"},
		{"Deckung unendlich", f.tx("isogenie-8192", "04d2", h, math.Inf(1)), z, "quanten_deckung"},
		{"Deckung negativ", f.tx("isogenie-8192", "04d2", h, -5), z, "quanten_deckung"},
		{"vor dem Stichtag", f.tx("isogenie-8192", "04d2", h, 5), qgAb - 1, "quanten_form"},
	}
	for _, c := range faelle {
		if u := f.urteil(c.tx, c.blk); u.regel != c.regel {
			t.Errorf("%s: %q (%s), erwartet %q", c.name, u.regel, u.text, c.regel)
		}
	}
	// Unterschrift gehoert zu einer anderen Ueberweisung (TxHash umgeschrieben).
	tx := f.tx("isogenie-8192", "04d2", h, 5)
	tx.TxHash = "0x" + strings.Repeat("4", 64)
	if u := f.urteil(tx, z); u.regel != "quanten_unterschrift" {
		t.Errorf("fremder TxHash: %+v", u)
	}
	// Seriennummer umgeschrieben.
	tx = f.tx("isogenie-8192", "04d2", h, 5)
	tx.Quanten.Serie = "04d3"
	if u := f.urteil(tx, z); u.regel != "quanten_unterschrift" {
		t.Errorf("umgeschriebene Seriennummer: %+v", u)
	}
	// Fremder Absender: unterschreibt ein anderer als die Wallet.
	andererKey, _ := neuerSchluessel(t)
	tx = f.tx("isogenie-8192", "04d2", h, 5)
	tx.Quanten.Sig = personalSign(t, andererKey, quantenPraegenNachricht(f.wallet, "isogenie-8192", "04d2", h))
	if u := f.urteil(tx, z); u.regel != "quanten_unterschrift" {
		t.Errorf("fremde Unterschrift: %+v", u)
	}
	// Nicht an den Topf.
	tx = f.tx("isogenie-8192", "04d2", h, 5)
	tx.To = "0x00000000000000000000000000000000000000b2"
	if u := f.urteil(tx, z); u.regel != "quanten_form" {
		t.Errorf("nicht an den Topf: %+v", u)
	}
	if len(f.cs.quantenGeldJetzt().Noten) != 0 {
		t.Fatal("abgewiesene Anmeldung hat das Register veraendert")
	}
}

// Rueckrollen: ein zurueckgewiesener Block nimmt das Register mit zurueck.
func TestQuantenGeld_Rueckrollen(t *testing.T) {
	f := neuerQgPraegeFall(t)
	f.cs.mu.Lock()
	snap := f.cs.snapshotForRollbackLocked(nil, true, nil)
	_ = f.cs.quantenNoteEintragenLocked(context.Background(), quantenNoteEintrag{K: strings.Repeat("b", 64), Satz: "isogenie-8192", Von: f.wallet, Mikro: 1_000_000})
	err := f.cs.restoreFromRollbackLockedCtx(context.Background(), snap)
	f.cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(f.cs.quantenGeldJetzt().Noten) != 0 {
		t.Fatal("Rueckroll laesst die Note stehen")
	}
}

// Annahme Ende zu Ende (ohne Datenbank): Deckung geht an den Topf, Note steht
// im Register, Geldmenge bleibt gleich; eine zweite Anmeldung derselben Note
// bewegt kein Geld.
func TestQuantenGeld_AnnahmeEndeZuEnde(t *testing.T) {
	f := neuerQgPraegeFall(t)
	// Annahme rechnet mit der Uhr: Stichtag und Satz liegen fuer diesen Test
	// in der Vergangenheit.
	quantenGeldAbOverride.Store(1)
	f.cs.quantenGeld.Store((&quantenGeldRegister{}).mitSatz(quantenSatzEintrag{Name: "isogenie-8192", Ordnung: qgSatzN.String(), Ab: 2, Seit: 1}))
	summe := func() float64 { return f.cs.TotalSupply() }
	start := summe()
	tx := f.tx("isogenie-8192", "04d2", "0x"+strings.Repeat("5", 64), 300)
	if err := f.cs.quantenPraegenAnnehmen(tx); err != nil {
		t.Fatal(err)
	}
	topf, _ := f.cs.accounts.Get(quantenTopfAddr)
	if topf == nil || topf.Balance.Float() < 299.999 {
		t.Fatalf("Topf hat %v", topf)
	}
	if acc, _ := f.cs.accounts.Get(f.wallet); acc.Balance.Float() > 700.001 {
		t.Fatalf("Anmelder hat noch %v", acc.Balance.Float())
	}
	if len(f.cs.quantenGeldJetzt().Noten) != 1 {
		t.Fatal("Note nicht im Register")
	}
	if d := summe() - start; d > 1e-6 || d < -1e-6 {
		t.Fatalf("Geldmenge veraendert um %v", d)
	}
	vorTopf := topf.Balance.Float()
	if err := f.cs.quantenPraegenAnnehmen(f.tx("isogenie-8192", "04d2", "0x"+strings.Repeat("6", 64), 5)); err == nil {
		t.Fatal("zweite Anmeldung derselben Note angenommen")
	}
	if topf, _ = f.cs.accounts.Get(quantenTopfAddr); topf.Balance.Float() != vorTopf {
		t.Fatal("abgewiesene Anmeldung hat Geld bewegt")
	}
}

// Annahme-Sperre: eine gewoehnliche Ueberweisung an den Topf (ohne Note)
// nimmt kein Weg an -- sie hielte sonst ab dem Stichtag die Blockproduktion
// an. Auch vor dem Stichtag.
func TestQuantenGeld_TopfOhneNoteNirgendsAngenommen(t *testing.T) {
	cs := newTestState()
	_, w := neuerSchluessel(t)
	addHuman(cs, w, 1000)
	if _, _, err := cs.TransferAtomic(w, quantenTopfAddr, 10, Transaction{Type: "transfer", Wallet: w, To: quantenTopfAddr, Amount: 10}); err == nil {
		t.Fatal("TransferAtomic an den Topf ohne Note angenommen")
	}
	if _, _, _, err := cs.TransferWithV7FeeAtomic(w, quantenTopfAddr, 10, Transaction{Type: "transfer", Wallet: w, To: quantenTopfAddr, Amount: 10}); err == nil {
		t.Fatal("TransferWithV7FeeAtomic an den Topf ohne Note angenommen")
	}
	if acc, _ := cs.accounts.Get(w); acc.Balance.Float() != 1000 {
		t.Fatalf("Guthaben bewegt: %v", acc.Balance.Float())
	}
	if pruefeTopfNurMitNote("0x"+strings.ToUpper(quantenTopfAddr[2:]), Transaction{}) == nil {
		t.Fatal("Grossschreibung umgeht die Sperre")
	}
}

// Ein Satz, der eben erst gilt, wird bei der Annahme noch nicht genommen
// (der Block koennte eine fruehere Zeit bekommen).
func TestQuantenGeld_AnnahmeErstEineStundeNachSatzbeginn(t *testing.T) {
	f := neuerQgPraegeFall(t)
	quantenGeldAbOverride.Store(1)
	jetzt := time.Now().Unix()
	f.cs.quantenGeld.Store((&quantenGeldRegister{}).mitSatz(quantenSatzEintrag{Name: "isogenie-8192", Ordnung: qgSatzN.String(), Ab: jetzt - 60, Seit: 1}))
	if err := f.cs.quantenPraegenAnnehmen(f.tx("isogenie-8192", "04d2", "0x"+strings.Repeat("7", 64), 5)); err == nil {
		t.Fatal("Anmeldung eine Minute nach Satzbeginn angenommen")
	}
	if len(f.cs.quantenGeldJetzt().Noten) != 0 {
		t.Fatal("Register veraendert")
	}
}
