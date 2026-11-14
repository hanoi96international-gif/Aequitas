package keeper

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// PQ-Zusage (pq_zusage.go): Gutfall und jeder Angriff -- fremder Absender,
// Wiederholung, Ersetzen durch einen Dieb, falsche Form, Zeit, kein Mensch,
// Register voll, vor dem Stichtag.

var pqRegeln = []string{"pq_form", "pq_zeit", "pq_kein_mensch", "pq_unterschrift", "pq_fest", "pq_voll", "umzug_form"}

func pqZaehler() map[string]int64 {
	m := map[string]int64{}
	for _, r := range pqRegeln {
		m[r] = erhaltungZaehler(r)
	}
	return m
}

func pqNeu(vorher map[string]int64) map[string]int64 {
	m := map[string]int64{}
	for _, r := range pqRegeln {
		if d := erhaltungZaehler(r) - vorher[r]; d != 0 {
			m[r] = d
		}
	}
	return m
}

func pqHashVon(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func neuerPQFall(t *testing.T) *uzFall {
	t.Helper()
	vorher := pqZusageAbOverride.Load()
	pqZusageAbOverride.Store(1)
	t.Cleanup(func() { pqZusageAbOverride.Store(vorher) })
	return neuerUzFall(t)
}

func (f *uzFall) pqTx(key *ecdsa.PrivateKey, wallet, hash string, zeit int64) Transaction {
	return Transaction{Type: pqZusageTxArt, Wallet: wallet, DistributionAt: zeit,
		Umzug: &UmzugAngaben{PQHash: hash, Sig: personalSign(f.t, key, pqZusageNachricht(wallet, hash, zeit))}}
}

func (f *uzFall) pqAnwenden(tx Transaction, blockZeit int64) map[string]int64 {
	f.t.Helper()
	vorher := pqZaehler()
	f.anwenden(tx, blockZeit)
	return pqNeu(vorher)
}

func TestPQZusage_Gutfall(t *testing.T) {
	f := neuerPQFall(t)
	leer := f.cs.StateRoot()
	h := pqHashVon("mldsa-schluessel")
	if b := f.pqAnwenden(f.pqTx(f.altKey, f.alt, h, uzZeit), uzZeit); len(b) != 0 {
		t.Fatalf("gueltige Zusage: %v", b)
	}
	reg := f.cs.umzugRegisterJetzt()
	z, ok := reg.pqZusage(f.alt)
	if !ok || z.Hash != h || z.Zeit != uzZeit {
		t.Fatalf("Zusage nicht im Register: %+v", z)
	}
	if f.cs.StateRoot() == leer {
		t.Fatal("die Zusage muss in die StateRoot")
	}
	wieder, err := umzugRegisterLesen(reg.kodiert())
	if err != nil || wieder.hash != reg.hash {
		t.Fatalf("Register nicht wieder lesbar: %v", err)
	}
	// Ohne Zusagen bleibt die Schreibweise des Registers wie vorher (kein
	// "pq"-Feld) -- die Wurzel bestehender Knoten aendert sich nicht.
	if strings.Contains((&umzugRegister{Codes: []umzugCode{{Wallet: f.alt, Hashes: []string{h}, Zeit: 1}}}).fertig().kodiert(), `"pq"`) {
		t.Fatal("leere PQ-Liste in der Schreibweise")
	}
	// Ein Umzug laesst die Zusage bei der Wallet (das Guthaben bleibt dort).
	f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, uzZeit+5), uzZeit+5)
	f.anwenden(f.antrag(f.neuKey, f.neu, f.alt, uzN, uzZeit+10, uzCode), uzZeit+10)
	if !f.umgezogen() {
		t.Fatal("Umzug mit Code nicht vollzogen")
	}
	if _, ok := f.cs.umzugRegisterJetzt().pqZusage(f.alt); !ok {
		t.Fatal("Zusage nach dem Umzug verloren")
	}
}

// Ein Angreifer, der den secp256k1-Schluessel abgeleitet hat (oder den Seed
// gestohlen), kann die Zusage nicht ersetzen -- und eine Wiederholung
// derselben Transaktion bewirkt nichts.
func TestPQZusage_NichtErsetzbarUndKeineWiederholung(t *testing.T) {
	f := neuerPQFall(t)
	echt := f.pqTx(f.altKey, f.alt, pqHashVon("echt"), uzZeit)
	f.pqAnwenden(echt, uzZeit)
	if b := f.pqAnwenden(f.pqTx(f.altKey, f.alt, pqHashVon("dieb"), uzZeit+10), uzZeit+10); b["pq_fest"] != 1 {
		t.Fatalf("Ersetzen: %v", b)
	}
	if b := f.pqAnwenden(echt, uzZeit+20); b["pq_fest"] != 1 {
		t.Fatalf("Wiederholung: %v", b)
	}
	if z, _ := f.cs.umzugRegisterJetzt().pqZusage(f.alt); z.Hash != pqHashVon("echt") {
		t.Fatalf("Zusage veraendert: %+v", z)
	}
}

func TestPQZusage_Missbrauch(t *testing.T) {
	f := neuerPQFall(t)
	h := pqHashVon("x")
	fremdKey, fremd := neuerSchluessel(t)
	addHuman(f.cs, fremd, 0)
	nichtMenschKey, nichtMensch := neuerSchluessel(t)

	falscheForm := f.pqTx(f.altKey, f.alt, h, uzZeit)
	falscheForm.Umzug.CodeHash = h
	zuKurz := f.pqTx(f.altKey, f.alt, h[:62], uzZeit)
	gross := f.pqTx(f.altKey, f.alt, strings.ToUpper(h), uzZeit)
	mitNullifier := f.pqTx(f.altKey, f.alt, h, uzZeit)
	mitNullifier.Nullifier = uzN
	ohneAngaben := Transaction{Type: pqZusageTxArt, Wallet: f.alt, DistributionAt: uzZeit}
	// Unterschrift ueber einen anderen Hash als den eingereichten.
	vertauscht := f.pqTx(f.altKey, f.alt, h, uzZeit)
	vertauscht.Umzug.PQHash = pqHashVon("anders")

	for name, fall := range map[string]struct {
		tx    Transaction
		block int64
		regel string
	}{
		"fremder Absender":       {f.pqTx(fremdKey, f.alt, h, uzZeit), uzZeit, "pq_unterschrift"},
		"Hash vertauscht":        {vertauscht, uzZeit, "pq_unterschrift"},
		"kein Mensch":            {f.pqTx(nichtMenschKey, nichtMensch, h, uzZeit), uzZeit, "pq_kein_mensch"},
		"Code-Hash dabei":        {falscheForm, uzZeit, "pq_form"},
		"Hash zu kurz":           {zuKurz, uzZeit, "pq_form"},
		"Hash gross geschrieben": {gross, uzZeit, "pq_form"},
		"Nullifier dabei":        {mitNullifier, uzZeit, "pq_form"},
		"ohne Angaben":           {ohneAngaben, uzZeit, "pq_form"},
		"zu alt":                 {f.pqTx(f.altKey, f.alt, h, uzZeit), uzZeit + umzugHoechstensAlt + 1, "pq_zeit"},
		"zu weit in der Zukunft": {f.pqTx(f.altKey, f.alt, h, uzZeit+umzugHoechstensVoraus+1), uzZeit, "pq_zeit"},
		"ohne Zeit":              {f.pqTx(f.altKey, f.alt, h, 0), uzZeit, "pq_zeit"},
	} {
		if b := f.pqAnwenden(fall.tx, fall.block); b[fall.regel] != 1 {
			t.Errorf("%s: %v (erwartet %s)", name, b, fall.regel)
		}
	}
	if !f.cs.umzugRegisterJetzt().leer() {
		t.Fatal("eine abgelehnte Zusage steht im Register")
	}
	// Ein Wiederherstellungscode mit untergeschobenem pq_hash ist kein Code.
	code := f.codeTx(f.altKey, f.alt, uzCode, uzZeit)
	code.Umzug.PQHash = h
	if b := f.anwenden(code, uzZeit); b["umzug_form"] != 1 {
		t.Fatalf("Code mit pq_hash: %v", b)
	}
}

func TestPQZusage_StichtagUndFenster(t *testing.T) {
	vorher := pqZusageAbOverride.Load()
	pqZusageAbOverride.Store(0)
	defer pqZusageAbOverride.Store(vorher)
	if bekannteTxArt(pqZusageTxArt, pqZusageAbUnix-1) || !bekannteTxArt(pqZusageTxArt, pqZusageAbUnix) {
		t.Fatal("pq_zusage muss genau ab pqZusageAbUnix bekannt sein")
	}
	f := &uzFall{t: t}
	key, w := neuerSchluessel(t)
	// Vor dem Stichtag unterschrieben: in keinen Block, auch nicht danach.
	frueh := f.pqTx(key, w, pqHashVon("a"), pqZusageAbUnix-10)
	if err := blockTauglich(&frueh, pqZusageAbUnix+10); err == nil {
		t.Fatal("vor dem Stichtag unterschriebene Zusage als tauglich angenommen")
	}
	if fe := auftragsFenster(&frueh, pqZusageAbUnix+10); fe.von <= fe.bis {
		t.Fatalf("Fenster nicht leer: %+v", fe)
	}
	gut := f.pqTx(key, w, pqHashVon("a"), pqZusageAbUnix+10)
	if err := blockTauglich(&gut, pqZusageAbUnix+20); err != nil {
		t.Fatalf("gueltige Zusage nicht tauglich: %v", err)
	}
	if err := blockTauglich(&gut, pqZusageAbUnix+10+umzugHoechstensAlt+1); err == nil {
		t.Fatal("zu alte Zusage tauglich")
	}
	fe := auftragsFenster(&gut, pqZusageAbUnix+20)
	// Frueher als 5 min vor der Unterschrift, aber nie vor dem Stichtag.
	if fe.von != pqZusageAbUnix || fe.bis != pqZusageAbUnix+10+umzugHoechstensAlt {
		t.Fatalf("Fenster %+v", fe)
	}
	// Das Urteil selbst lehnt vor dem Stichtag ab (Block davor).
	f2 := neuerUzFall(t)
	if b := f2.pqAnwenden(f2.pqTx(f2.altKey, f2.alt, pqHashVon("a"), uzZeit), uzZeit); b["pq_form"] != 1 {
		t.Fatalf("vor dem Stichtag: %v", b)
	}
}

func TestPQZusage_RegisterFormUndGrenze(t *testing.T) {
	a := "0x00000000000000000000000000000000000000a1"
	b := "0x00000000000000000000000000000000000000b2"
	h := pqHashVon("x")
	for name, roh := range map[string]string{
		"unsortiert":       `{"pq":[{"w":"` + b + `","h":"` + h + `","z":1},{"w":"` + a + `","h":"` + h + `","z":1}]}`,
		"doppelt":          `{"pq":[{"w":"` + a + `","h":"` + h + `","z":1},{"w":"` + a + `","h":"` + h + `","z":2}]}`,
		"Hash kaputt":      `{"pq":[{"w":"` + a + `","h":"zz","z":1}]}`,
		"ohne Zeit":        `{"pq":[{"w":"` + a + `","h":"` + h + `","z":0}]}`,
		"Adresse kaputt":   `{"pq":[{"w":"0xA1","h":"` + h + `","z":1}]}`,
		"unbekanntes Feld": `{"pq":[{"w":"` + a + `","h":"` + h + `","z":1,"x":1}]}`,
	} {
		if _, err := umzugRegisterLesen(roh); err == nil {
			t.Errorf("%s: angenommen", name)
		}
	}
	if _, err := umzugRegisterLesen(`{"pq":[{"w":"` + a + `","h":"` + h + `","z":1},{"w":"` + b + `","h":"` + h + `","z":1}]}`); err != nil {
		t.Fatalf("gueltiges Register abgelehnt: %v", err)
	}

	// Register voll: keine weitere Zusage.
	f := neuerPQFall(t)
	voll := make([]pqZusage, pqZusagenHoechstens)
	for i := range voll {
		voll[i] = pqZusage{Wallet: "0x" + strings.Repeat("0", 32) + hex.EncodeToString([]byte{byte(i >> 24), byte(i >> 16), byte(i >> 8), byte(i)}), Hash: h, Zeit: 1}
	}
	f.cs.umzugReg.Store((&umzugRegister{PQ: voll}).fertig())
	if b := f.pqAnwenden(f.pqTx(f.altKey, f.alt, h, uzZeit), uzZeit); b["pq_voll"] != 1 {
		t.Fatalf("volles Register: %v", b)
	}
	if err := pqZusagenGueltig(append(voll, pqZusage{Wallet: "0x" + strings.Repeat("f", 40), Hash: h, Zeit: 1})); err == nil {
		t.Fatal("Register ueber der Grenze gelesen")
	}
}

// Pruefvektor, byte-gleich in der App (lib/__tests__/pq.test.ts).
func TestPQZusage_PruefvektorFuerDieApp(t *testing.T) {
	w := "0x00000000000000000000000000000000000000a1"
	if got, want := pqZusageNachricht(w, "ab", 7), "aequitas-pq-zusage-v1|chain:1926|"+w+"|ab|7"; got != want {
		t.Fatalf("%q != %q", got, want)
	}
}
