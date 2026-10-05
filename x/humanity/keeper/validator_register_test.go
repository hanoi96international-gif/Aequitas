package keeper

import (
	"crypto/ecdsa"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// Zustandslose Teile des Validator-Registers (validator_register.go).

func neuerSchluessel(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	k, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return k, adrVon(k)
}

// bindungUnterschrieben: eine validator_bindung, wie Betreiber und
// Signierschluessel sie unterschreiben.
func bindungUnterschrieben(t *testing.T, betreiber, signing *ecdsa.PrivateKey, zeit int64) Transaction {
	t.Helper()
	b, s := adrVon(betreiber), adrVon(signing)
	msg := validatorBindungNachricht(s, b, zeit)
	return Transaction{Type: "validator_bindung", Wallet: b, To: s,
		Nachweis: &Auftragsnachweis{Zeit: zeit, Sig: personalSign(t, betreiber, msg), Sig2: personalSign(t, signing, msg)}}
}

func TestBekannteTxArt_ValidatorBindungErstAbStichtag(t *testing.T) {
	validatorRegisterOverride.Store(1000)
	t.Cleanup(func() { validatorRegisterOverride.Store(0) })
	if bekannteTxArt("validator_bindung", 999) {
		t.Fatal("validator_bindung vor dem Stichtag als bekannt durchgelassen")
	}
	if !bekannteTxArt("validator_bindung", 1000) {
		t.Fatal("validator_bindung ab dem Stichtag abgewiesen")
	}
	if bekannteTxArt("erfunden", 1000) || !bekannteTxArt("transfer", 0) || !bekannteTxArt("vormund_setzen", 0) {
		t.Fatal("die uebrigen Arten haben sich veraendert")
	}
	validatorRegisterOverride.Store(0)
	if bekannteTxArt("validator_bindung", nowUnix()) {
		t.Fatal("ohne gesetzten Stichtag muss validator_bindung unbekannt sein")
	}
}

func TestKanonischeAdresse(t *testing.T) {
	gut := "0x" + strings.Repeat("ab", 20)
	if !kanonischeAdresse(gut) {
		t.Fatal("kanonische Adresse abgewiesen")
	}
	for _, a := range []string{"", gut[2:], strings.ToUpper(gut[:4]) + gut[4:], "0x" + strings.Repeat("AB", 20),
		gut + "0", gut[:41], "0x" + strings.Repeat("zz", 20), " " + gut[1:]} {
		if kanonischeAdresse(a) {
			t.Fatalf("%q als kanonisch durchgelassen", a)
		}
	}
}

// Der Satz ist ein anderer als der fuer /api/peers/register -- eine dort
// gesehene Unterschrift gilt auf der Kette nicht.
func TestValidatorBindung_AlteUnterschriftGiltNicht(t *testing.T) {
	betreiber, _ := neuerSchluessel(t)
	signing, s := neuerSchluessel(t)
	tx := bindungUnterschrieben(t, betreiber, signing, nowUnix())
	tx.Nachweis.Sig = personalSign(t, betreiber, "Aequitas: authorize validator "+s)
	if err := pruefeAuftragsNachweis(&tx, tx.Nachweis.Zeit); err == nil {
		t.Fatal("Unterschrift aus /api/peers/register als Bindung angenommen")
	}
}

func TestValidatorBindung_Nachweis(t *testing.T) {
	betreiber, _ := neuerSchluessel(t)
	signing, _ := neuerSchluessel(t)
	fremd, _ := neuerSchluessel(t)
	jetzt := nowUnix()
	gut := bindungUnterschrieben(t, betreiber, signing, jetzt-60)
	if err := pruefeAuftragsNachweis(&gut, jetzt); err != nil {
		t.Fatalf("gueltige Bindung abgewiesen: %v", err)
	}
	// Ohne Zustimmung des Signierschluessels: ein Fremder unterschreibt an
	// seiner Stelle.
	ohne := gut
	n := *gut.Nachweis
	n.Sig2 = personalSign(t, fremd, validatorBindungNachricht(gut.To, gut.Wallet, n.Zeit))
	ohne.Nachweis = &n
	if err := pruefeAuftragsNachweis(&ohne, jetzt); err == nil {
		t.Fatal("Bindung ohne Zustimmung des Signierschluessels angenommen")
	}
	// Andere Signieradresse als unterschrieben.
	umgelenkt := gut
	umgelenkt.To = adrVon(fremd)
	if err := pruefeAuftragsNachweis(&umgelenkt, jetzt); err == nil {
		t.Fatal("auf eine andere Signieradresse umgelenkte Bindung angenommen")
	}
	// Zu alt, aus der Zukunft.
	for _, z := range []int64{jetzt - 2*3600, jetzt + 3600} {
		alt := bindungUnterschrieben(t, betreiber, signing, z)
		if err := pruefeAuftragsNachweis(&alt, jetzt); err == nil {
			t.Fatalf("Bindung mit Zeitpunkt %d (Block %d) angenommen", z, jetzt)
		}
	}
}

func TestPruefeSnapshotValidatoren(t *testing.T) {
	validatorRegisterOverride.Store(1)
	t.Cleanup(func() { validatorRegisterOverride.Store(0) })
	b1, _ := neuerSchluessel(t)
	s1, _ := neuerSchluessel(t)
	b2, _ := neuerSchluessel(t)
	s2, _ := neuerSchluessel(t)
	eintrag := func(b, s *ecdsa.PrivateKey, zeit int64) SnapshotValidator {
		tx := bindungUnterschrieben(t, b, s, zeit)
		return SnapshotValidator{Betreiber: tx.Wallet, Signing: tx.To, Zeit: zeit, SigOperator: tx.Nachweis.Sig, SigSigning: tx.Nachweis.Sig2}
	}
	// Alt ist erlaubt: ein Eintrag ist so alt wie seine Bindung.
	gut := []SnapshotValidator{eintrag(b1, s1, 1_700_000_000), eintrag(b2, s2, 1_700_000_100)}
	if err := pruefeSnapshotValidatoren(gut, nowUnix()+nachweisHoechstensVoraus); err != nil {
		t.Fatalf("gueltiges Register abgewiesen: %v", err)
	}
	_, fremd := neuerSchluessel(t)
	umgelenkt := append([]SnapshotValidator(nil), gut...)
	umgelenkt[1].Signing = fremd
	if err := pruefeSnapshotValidatoren(umgelenkt, nowUnix()+nachweisHoechstensVoraus); err == nil {
		t.Fatal("Snapshot-Eintrag mit fremder Signieradresse angenommen")
	}
	verschoben := append([]SnapshotValidator(nil), gut...)
	verschoben[0].Zeit++
	if err := pruefeSnapshotValidatoren(verschoben, nowUnix()+nachweisHoechstensVoraus); err == nil {
		t.Fatal("Snapshot-Eintrag mit veraendertem Zeitpunkt angenommen")
	}
	// Je nur eine Unterschrift falsch: ein Fremder unterschreibt den
	// richtigen Satz an Stelle des Betreibers bzw. des Signierschluessels.
	fremdKey, _ := neuerSchluessel(t)
	satz := validatorBindungNachricht(gut[0].Signing, gut[0].Betreiber, gut[0].Zeit)
	ohneBetreiber := append([]SnapshotValidator(nil), gut...)
	ohneBetreiber[0].SigOperator = personalSign(t, fremdKey, satz)
	if err := pruefeSnapshotValidatoren(ohneBetreiber, nowUnix()+nachweisHoechstensVoraus); err == nil {
		t.Fatal("Snapshot-Eintrag ohne Unterschrift des Betreibers angenommen")
	}
	ohneSigning := append([]SnapshotValidator(nil), gut...)
	ohneSigning[0].SigSigning = personalSign(t, fremdKey, satz)
	if err := pruefeSnapshotValidatoren(ohneSigning, nowUnix()+nachweisHoechstensVoraus); err == nil {
		t.Fatal("Snapshot-Eintrag ohne Unterschrift des Signierschluessels angenommen")
	}
	doppelt := []SnapshotValidator{gut[0], gut[0]}
	if err := pruefeSnapshotValidatoren(doppelt, nowUnix()+nachweisHoechstensVoraus); err == nil {
		t.Fatal("doppelter Snapshot-Eintrag angenommen")
	}
	// Zwei Betreiber mit derselben Signieradresse sind ein gueltiger Zustand
	// (umstritten, beim Lesen entschieden).
	geteilt := []SnapshotValidator{eintrag(b1, s1, 1_700_000_000), eintrag(b2, s1, 1_700_000_000)}
	if err := pruefeSnapshotValidatoren(geteilt, nowUnix()+nachweisHoechstensVoraus); err != nil {
		t.Fatalf("geteilte Signieradresse abgewiesen: %v", err)
	}
	// Vor dem Stichtag kann es keine Bindung geben.
	validatorRegisterOverride.Store(1_700_000_000 + nachweisHoechstensAlt + 1)
	if err := pruefeSnapshotValidatoren(gut[:1], nowUnix()+nachweisHoechstensVoraus); err == nil {
		t.Fatal("Snapshot-Eintrag von vor dem Stichtag angenommen")
	}
	validatorRegisterOverride.Store(0)
	if err := pruefeSnapshotValidatoren(gut[:1], nowUnix()+nachweisHoechstensVoraus); err == nil {
		t.Fatal("Snapshot-Eintrag ohne gesetzten Stichtag angenommen")
	}
	if err := pruefeSnapshotValidatoren(nil, nowUnix()+nachweisHoechstensVoraus); err != nil {
		t.Fatalf("leeres Register abgewiesen: %v", err)
	}
}

// Die Chain-ID steht im Satz: eine Bindung fuer eine andere Kette gilt nicht.
func TestValidatorBindung_ChainIDImSatz(t *testing.T) {
	if !strings.Contains(validatorBindungNachricht("0xa", "0xb", 1), "chain:1926 ") {
		t.Fatalf("Satz ohne Chain-ID: %s", validatorBindungNachricht("0xa", "0xb", 1))
	}
}

// Stufe 2: Vorbehalt, Ausfuehrung und Kappung sind bis zur Aktivierung
// unbekannt (wie bisher) und danach bekannt -- sonst wiese jeder Knoten ab
// der Aktivierung jeden fremden Block damit ab.
func TestBekannteTxArt_Stufe2ErstAbAktivierung(t *testing.T) {
	verteilteAnnahmeOverride.Store(1000)
	nachrechnenStrengOverride.Store(1000)
	t.Cleanup(func() { verteilteAnnahmeOverride.Store(0); nachrechnenStrengOverride.Store(0) })
	for _, typ := range []string{"vorbehalt", "vorbehalt_ausfuehrung", "kappung"} {
		if bekannteTxArt(typ, 999) {
			t.Fatalf("%s vor der Aktivierung durchgelassen", typ)
		}
		if !bekannteTxArt(typ, 1000) {
			t.Fatalf("%s ab der Aktivierung abgewiesen", typ)
		}
	}
	// Stufe 2 aktiv, aber noch nicht streng nachgerechnet: unbekannt --
	// sonst koennte jeder Erzeuger mit einer Kappung jedes Konto belasten.
	nachrechnenStrengOverride.Store(2000)
	for _, typ := range []string{"vorbehalt", "vorbehalt_ausfuehrung", "kappung"} {
		if bekannteTxArt(typ, 1500) {
			t.Fatalf("%s ohne strenges Nachrechnen durchgelassen", typ)
		}
	}
	verteilteAnnahmeOverride.Store(0)
	nachrechnenStrengOverride.Store(0)
	if bekannteTxArt("vorbehalt", nowUnix()) {
		t.Fatal("ohne gesetzten Stichtag muss vorbehalt unbekannt sein")
	}
}

// Die Unterschrift hat genau eine Schreibweise: sonst liesse sich dieselbe
// Bindung unter vielen Transaktions-Hashes einreichen, und angehaengte Bytes
// landeten im Register.
func TestValidatorBindung_KanonischeSignatur(t *testing.T) {
	betreiber, _ := neuerSchluessel(t)
	signing, _ := neuerSchluessel(t)
	jetzt := nowUnix()
	gut := bindungUnterschrieben(t, betreiber, signing, jetzt-60)
	if err := validatorBindungForm(&gut); err != nil {
		t.Fatalf("kanonische Unterschrift abgewiesen: %v", err)
	}
	sig := gut.Nachweis.Sig
	roh, _ := hex.DecodeString(sig[2:])
	// s -> n - s: dieselbe Unterschrift in der oberen Haelfte, v gekippt.
	hoch := append([]byte(nil), roh...)
	sWert := new(big.Int).SetBytes(hoch[32:64])
	sWert.Sub(crypto.S256().Params().N, sWert)
	sWert.FillBytes(hoch[32:64])
	hoch[64] = 55 - hoch[64] // 27 <-> 28
	v0 := append([]byte(nil), roh...)
	v0[64] -= 27
	for name, falsch := range map[string]string{
		"Grossbuchstaben": "0x" + strings.ToUpper(sig[2:]),
		"ohne 0x":         sig[2:],
		"angehaengt":      sig + "00",
		"Muell hinten":    sig[:130] + "zz",
		"v 0/1":           "0x" + hex.EncodeToString(v0),
		"hohes s":         "0x" + hex.EncodeToString(hoch),
		"zu kurz":         sig[:130],
	} {
		tx := gut
		n := *gut.Nachweis
		n.Sig = falsch
		tx.Nachweis = &n
		if err := validatorBindungForm(&tx); err == nil {
			t.Fatalf("%s: nicht kanonische Unterschrift angenommen (%s)", name, falsch)
		}
		n2 := *gut.Nachweis
		n2.Sig2 = falsch
		tx.Nachweis = &n2
		if err := validatorBindungForm(&tx); err == nil {
			t.Fatalf("%s: nicht kanonische zweite Unterschrift angenommen", name)
		}
	}
	// Die Variante mit hohem s waere sonst gueltig -- genau das soll die
	// Form ausschliessen.
	if err := verifyPersonalSign(validatorBindungNachricht(gut.To, gut.Wallet, gut.Nachweis.Zeit), "0x"+hex.EncodeToString(hoch), gut.Wallet); err != nil {
		t.Fatalf("Vorbedingung: die Variante mit hohem s muss sonst gueltig sein: %v", err)
	}
}

// Snapshot: kein Zeitpunkt aus der Zukunft, und "ueberholt" muss stimmen.
func TestPruefeSnapshotValidatoren_ZukunftUndUeberholt(t *testing.T) {
	validatorRegisterOverride.Store(1)
	t.Cleanup(func() { validatorRegisterOverride.Store(0) })
	a, _ := neuerSchluessel(t)
	c, _ := neuerSchluessel(t)
	s1, _ := neuerSchluessel(t)
	jetzt := nowUnix()
	eintrag := func(b, s *ecdsa.PrivateKey, zeit int64, ueberholt bool) SnapshotValidator {
		tx := bindungUnterschrieben(t, b, s, zeit)
		return SnapshotValidator{Betreiber: tx.Wallet, Signing: tx.To, Zeit: zeit, SigOperator: tx.Nachweis.Sig, SigSigning: tx.Nachweis.Sig2, Ueberholt: ueberholt}
	}
	bis := snapshotValidatorenBis(jetzt)
	if err := pruefeSnapshotValidatoren([]SnapshotValidator{eintrag(a, s1, jetzt+3600, false)}, bis); err == nil {
		t.Fatal("Snapshot-Eintrag aus der Zukunft angenommen")
	}
	if err := pruefeSnapshotValidatoren([]SnapshotValidator{eintrag(a, s1, jetzt-200, false), eintrag(c, s1, jetzt-100, false)}, bis); err == nil {
		t.Fatal("fruehere Bindung an dieselbe Adresse ohne Markierung angenommen")
	}
	if err := pruefeSnapshotValidatoren([]SnapshotValidator{eintrag(a, s1, jetzt-200, true), eintrag(c, s1, jetzt-100, false)}, bis); err != nil {
		t.Fatalf("korrekt markiertes Register abgewiesen: %v", err)
	}
	// Ueberholt ohne spaetere Bindung daneben ist gueltig: der spaetere
	// Betreiber ist weitergezogen.
	if err := pruefeSnapshotValidatoren([]SnapshotValidator{eintrag(a, s1, jetzt-200, true)}, bis); err != nil {
		t.Fatalf("ueberholte Bindung ohne Nachfolger abgewiesen: %v", err)
	}
}

// Stufe 2 darf nie vor dem strengen Nachrechnen gelten (zweiter
// Sicherheitsdurchgang zu #292): sonst erzeugte der Leiter Kappungen und
// Vorbehalte, die jeder andere Knoten als unbekannte Art abweist.
func TestStufe2NieVorStrengemNachrechnen(t *testing.T) {
	if verteilteAnnahmeAbUnix < nachrechnenStrengAbUnix {
		t.Fatalf("verteilteAnnahmeAbUnix (%d) liegt vor nachrechnenStrengAbUnix (%d)", verteilteAnnahmeAbUnix, nachrechnenStrengAbUnix)
	}
}

func TestValidatorBlatt_UeberholtAendertDasBlatt(t *testing.T) {
	if validatorBlatt("0xa", "0xb", 1, true) == validatorBlatt("0xa", "0xb", 1, false) {
		t.Fatal("die Ueberholt-Markierung aendert das Blatt nicht")
	}
}
