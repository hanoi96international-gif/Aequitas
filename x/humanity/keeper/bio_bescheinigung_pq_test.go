package keeper

import (
	"crypto/mldsa"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Missbrauchstests zur quantensicheren Bescheinigung (bio_bescheinigung_pq.go,
// docs/QUANTENSICHERHEIT.md Stufe 1). Jeder Fall zeigt einen Angriff.

type pqTestSchluessel struct {
	ed  bTestSchluessel
	pq  *mldsa.PrivateKey
	pub string
}

func neuerPQ(t *testing.T) pqTestSchluessel {
	t.Helper()
	sk, err := mldsa.GenerateKey(mldsa.MLDSA65())
	if err != nil {
		t.Fatal(err)
	}
	return pqTestSchluessel{ed: neuerEd25519(t), pq: sk, pub: hex.EncodeToString(sk.PublicKey().Bytes())}
}

func (s pqTestSchluessel) pqUnterschrift(t *testing.T, msg string) string {
	t.Helper()
	sig, err := s.pq.Sign(rand.Reader, pqNachricht([]byte(msg)), nil)
	if err != nil {
		t.Fatal(err)
	}
	return s.ed.pub + ":" + hex.EncodeToString(sig)
}

func bindungsText(modus, nullifier, wallet string, zeit int64) string {
	return "aequitas-bio-attestation-v3|" + modus + "|" + nullifier + "|" + wallet + "|" + strconv.FormatInt(zeit, 10)
}

func aussageText(modus, nullifier string, zeit int64) string {
	return "aequitas-bio-personhood-v3|" + modus + "|" + nullifier + "|" + strconv.FormatInt(zeit, 10)
}

// pqUmgebung: Coordinator und Bezeuger mit PQ-Schluesseln; Stichtag ab.
func pqUmgebung(t *testing.T, ab int64) (coord, zeuge pqTestSchluessel) {
	t.Helper()
	coord, zeuge = neuerPQ(t), neuerPQ(t)
	setBescheinigungFuerTest(t, &bescheinigungKonfig{AbUnix: 1, Modus: "test", Quorum: 2,
		Coordinator: []string{coord.ed.pub}, Bezeuger: []string{zeuge.ed.pub},
		PQAbUnix: ab, PQ: map[string]string{coord.ed.pub: coord.pub, zeuge.ed.pub: zeuge.pub}})
	return coord, zeuge
}

func pqGuteTx(t *testing.T, coord, zeuge pqTestSchluessel) Transaction {
	t.Helper()
	tx := bTestTx(t, bTestNullifier, bindung(coord.ed, "test", bTestNullifier, bTestWallet, bTestZeit), aussage(zeuge.ed, "test", bTestNullifier, bTestZeit))
	tx.BioBescheinigung.PQ = []string{
		coord.pqUnterschrift(t, bindungsText("test", bTestNullifier, bTestWallet, bTestZeit)),
		zeuge.pqUnterschrift(t, aussageText("test", bTestNullifier, bTestZeit)),
	}
	return tx
}

func TestPQBescheinigung_GueltigUndMissbrauch(t *testing.T) {
	coord, zeuge := pqUmgebung(t, bTestZeit) // Stichtag = Ausstellung: Pflicht
	gut := pqGuteTx(t, coord, zeuge)
	if err := pruefeBioBescheinigung(gut); err != nil {
		t.Fatalf("gueltige hybride Bescheinigung abgelehnt: %v", err)
	}
	fremd := neuerPQ(t)
	mit := func(pq ...string) Transaction {
		x := pqGuteTx(t, coord, zeuge)
		x.BioBescheinigung.PQ = pq
		return x
	}
	gb, ga := gut.BioBescheinigung.PQ[0], gut.BioBescheinigung.PQ[1]
	kipp := func(e string) string {
		b := []byte(e)
		if b[100] == 'a' {
			b[100] = 'b'
		} else {
			b[100] = 'a'
		}
		return string(b)
	}
	faelle := map[string]Transaction{
		"nur Ed25519 (Quantenrechner faelscht Ed25519)": mit(),
		"PQ des Bezeugers fehlt":                        mit(gb),
		"PQ des Coordinators fehlt":                     mit(ga),
		"ein Bit der ML-DSA-Unterschrift gekippt":       mit(kipp(gb), ga),
		"ML-DSA ueber andere Wallet": mit(coord.pqUnterschrift(t, bindungsText("test", bTestNullifier,
			"0xb0d1000000000000000000000000000000000002", bTestZeit)), ga),
		"ML-DSA ueber anderen Nullifier": mit(gb, zeuge.pqUnterschrift(t, aussageText("test", "111", bTestZeit))),
		"ML-DSA ueber andere Zeit":       mit(gb, zeuge.pqUnterschrift(t, aussageText("test", bTestNullifier, bTestZeit+1))),
		"ML-DSA ohne Domaene (rohe v3-Nachricht)": func() Transaction {
			sig, _ := coord.pq.Sign(rand.Reader, []byte(bindungsText("test", bTestNullifier, bTestWallet, bTestZeit)), nil)
			return mit(coord.ed.pub+":"+hex.EncodeToString(sig), ga)
		}(),
		"Bezeuger unterschreibt mit Coordinator-Nachricht": mit(gb, zeuge.pqUnterschrift(t, bindungsText("test", bTestNullifier, bTestWallet, bTestZeit))),
		"fremder ML-DSA-Schluessel unter fremdem Ed25519":  mit(gb, ga, fremd.pqUnterschrift(t, aussageText("test", bTestNullifier, bTestZeit))),
		"fremder ML-DSA-Schluessel unter Bezeuger-Ed25519": mit(gb,
			zeuge.ed.pub+":"+strings.SplitN(fremd.pqUnterschrift(t, aussageText("test", bTestNullifier, bTestZeit)), ":", 2)[1]),
		"zweimal derselbe Schluessel": mit(gb, ga, ga),
		"abgeschnittene Unterschrift": mit(gb[:len(gb)-2], ga),
		"zu lange Unterschrift":       mit(gb+"00", ga),
		"Grossbuchstaben im Hex":      mit(strings.ToUpper(gb[:64])+gb[64:], ga),
		"ohne Trennzeichen":           mit(strings.Replace(gb, ":", "0", 1), ga),
		"mehr Eintraege als erlaubt":  mit(append([]string{gb, ga}, make([]string, bescheinigungMaxSignaturen)...)...),
	}
	for name, tx := range faelle {
		if err := pruefeBioBescheinigung(tx); err == nil {
			t.Errorf("%s: angenommen", name)
		}
	}
}

func TestPQBescheinigung_StichtagOhneStau(t *testing.T) {
	coord, zeuge := pqUmgebung(t, bTestZeit+100) // Stichtag NACH der Ausstellung
	ohnePQ := pqGuteTx(t, coord, zeuge)
	ohnePQ.BioBescheinigung.PQ = nil
	// Vor dem Stichtag ausgestellt, im naechsten Block (nach dem Stichtag):
	// gilt noch -- sonst fiele eine kurz vorher angenommene Registrierung aus
	// dem Block und der Erzeuger stuende.
	if err := pruefeBioBescheinigungMit(ohnePQ, bTestZeit+200, nil); err != nil {
		t.Fatalf("vor dem Stichtag ausgestellt, kurz danach im Block: %v", err)
	}
	// Ein Umzug darf 24 h alte Bescheinigungen tragen: auch 3 h nach dem
	// Stichtag gilt die vorher ausgestellte noch.
	if err := pruefeBioBescheinigungMit(ohnePQ, bTestZeit+100+3*3600, nil); err != nil {
		t.Fatalf("vor dem Stichtag ausgestellt, 3 h danach im Block: %v", err)
	}
	// Ein rueckdatierter Bescheid hilft nach Ablauf der Frist nicht mehr.
	if err := pruefeBioBescheinigungMit(ohnePQ, bTestZeit+100+pqNachlauf, nil); err == nil {
		t.Fatal("ohne PQ lange nach dem Stichtag im Block angenommen")
	}
	// Ab dem Stichtag ausgestellt: sofort Pflicht.
	coord2, zeuge2 := pqUmgebung(t, bTestZeit)
	spaet := pqGuteTx(t, coord2, zeuge2)
	spaet.BioBescheinigung.PQ = nil
	if err := pruefeBioBescheinigungMit(spaet, bTestZeit+10, nil); err == nil {
		t.Fatal("ab dem Stichtag ausgestellt, ohne PQ angenommen")
	}
}

func TestPQBescheinigung_SchlafendPrueftNurForm(t *testing.T) {
	coord, zeuge := pqUmgebung(t, 0) // schlafend
	tx := pqGuteTx(t, coord, zeuge)
	tx.BioBescheinigung.PQ = nil
	if err := pruefeBioBescheinigung(tx); err != nil {
		t.Fatalf("schlafend, ohne PQ: %v", err)
	}
	// Eine ungueltige, aber wohlgeformte PQ-Unterschrift stoppt vor dem
	// Stichtag nichts (der Coordinator darf sie vorab mitschicken) ...
	falsch := neuerPQ(t)
	tx.BioBescheinigung.PQ = []string{coord.ed.pub + ":" + strings.SplitN(falsch.pqUnterschrift(t, "x"), ":", 2)[1]}
	if err := pruefeBioBescheinigung(tx); err != nil {
		t.Fatalf("schlafend, wohlgeformte PQ-Unterschrift: %v", err)
	}
	// ... die Form aber schon (Grenze fuer Groesse und Anzahl).
	tx.BioBescheinigung.PQ = []string{"kurz"}
	if err := pruefeBioBescheinigung(tx); err == nil {
		t.Fatal("schlafend, PQ-Eintrag in falscher Form angenommen")
	}
}

func TestPQBescheinigung_GenesisFailClosed(t *testing.T) {
	a, b := neuerPQ(t), neuerPQ(t)
	basis := func() bescheinigungKonfig {
		return bescheinigungKonfig{AbUnix: 1, Modus: "test", Quorum: 2,
			Coordinator: []string{a.ed.pub}, Bezeuger: []string{b.ed.pub}}
	}
	ok := basis()
	ok.PQAbUnix, ok.PQ = 5, map[string]string{a.ed.pub: a.pub, b.ed.pub: b.pub}
	if err := ok.pruefen(); err != nil {
		t.Fatalf("gueltige PQ-Konfiguration abgelehnt: %v", err)
	}
	faelle := map[string]func(k *bescheinigungKonfig){
		"Stichtag ohne alle Schluessel": func(k *bescheinigungKonfig) { k.PQAbUnix, k.PQ = 5, map[string]string{a.ed.pub: a.pub} },
		"PQ fuer fremden Ed25519":       func(k *bescheinigungKonfig) { k.PQ = map[string]string{neuerEd25519(t).pub: a.pub} },
		"kaputter ML-DSA-Schluessel":    func(k *bescheinigungKonfig) { k.PQ = map[string]string{a.ed.pub: a.pub[:100]} },
		"ML-DSA-Schluessel gross":       func(k *bescheinigungKonfig) { k.PQ = map[string]string{a.ed.pub: strings.ToUpper(a.pub)} },
		"negativer Stichtag":            func(k *bescheinigungKonfig) { k.PQAbUnix = -1 },
		"PQ-Stichtag vor der Bescheinigung": func(k *bescheinigungKonfig) {
			k.AbUnix, k.PQAbUnix, k.PQ = 10, 5, map[string]string{a.ed.pub: a.pub, b.ed.pub: b.pub}
		},
	}
	for name, aendern := range faelle {
		k := basis()
		aendern(&k)
		if err := k.pruefen(); err == nil {
			t.Errorf("%s: angenommen", name)
		}
	}
	// Ohne PQ-Felder bleibt der Fingerabdruck unveraendert (omitempty).
	ohne := basis()
	if bescheinigungFingerabdruck(ohne) != bescheinigungFingerabdruck(bescheinigungKonfig{AbUnix: 1, Modus: "test", Quorum: 2,
		Coordinator: []string{a.ed.pub}, Bezeuger: []string{b.ed.pub}, PQ: map[string]string{}}) {
		t.Error("leere PQ-Felder aendern den Fingerabdruck")
	}
}

func TestPQBescheinigung_AusProveAnfrageUndProofServer(t *testing.T) {
	coord, zeuge := pqUmgebung(t, bTestZeit)
	gut := pqGuteTx(t, coord, zeuge)
	feld := "v3:" + gut.BioBescheinigung.Signaturen[0] + ",v3:" + gut.BioBescheinigung.Signaturen[1] +
		",pq:" + gut.BioBescheinigung.PQ[0] + ",pq:" + gut.BioBescheinigung.PQ[1] + ",ps1"
	body := []byte(`{"bioAttestation":"` + feld + `","bioAttestationIssuedAt":` + strconv.FormatInt(bTestZeit, 10) + `}`)
	b := bescheinigungAusProveAnfrage(body)
	if b == nil || len(b.PQ) != 2 || len(b.Signaturen) != 2 {
		t.Fatalf("PQ-Eintraege nicht gelesen: %+v", b)
	}
	// Der Proof-Server bekommt weder v3 noch PQ.
	if raus := string(proveAnfrageOhneV3(body)); strings.Contains(raus, "pq:") || strings.Contains(raus, "v3:") || !strings.Contains(raus, `"ps1"`) {
		t.Fatalf("an den Proof-Server: %s", raus)
	}
	// Falsche Form im Feld: keine Bescheinigung (fail-closed).
	schlecht := []byte(`{"bioAttestation":"v3:` + gut.BioBescheinigung.Signaturen[0] + `,pq:abc","bioAttestationIssuedAt":` + strconv.FormatInt(bTestZeit, 10) + `}`)
	if bescheinigungAusProveAnfrage(schlecht) != nil {
		t.Fatal("PQ-Eintrag in falscher Form ergab eine Bescheinigung")
	}
}

// Pruefvektor aus dem Python-Coordinator (aequitas-biometric-beta,
// coordinator/tests/test_pq_unterschrift.py, gleiche Seeds): eine echte
// Ed25519-Bindung plus ML-DSA-65 aus cryptography 50. Weicht die Nachricht,
// das Praefix oder die Feldform zwischen den Repos ab, wird dieser Test rot.
func TestPQBescheinigung_VektorAusDemPythonCoordinator(t *testing.T) {
	roh, err := os.ReadFile(filepath.Join("testdata", "pq_bescheinigung_vektor.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Modus, Nullifier, Wallet                    string
		Zeit                                        int64
		Ed25519Pub, Ed25519Sig, MldsaPub, PQEintrag string
	}
	var m map[string]any
	if err := json.Unmarshal(roh, &m); err != nil {
		t.Fatal(err)
	}
	v.Modus, v.Nullifier, v.Wallet = m["modus"].(string), m["nullifier"].(string), m["wallet"].(string)
	v.Zeit = int64(m["zeit"].(float64))
	v.Ed25519Pub, v.Ed25519Sig = m["ed25519_pub"].(string), m["ed25519_sig"].(string)
	v.MldsaPub, v.PQEintrag = m["mldsa_pub"].(string), m["pq_eintrag"].(string)

	setBescheinigungFuerTest(t, &bescheinigungKonfig{AbUnix: 1, Modus: v.Modus, Quorum: 1,
		Coordinator: []string{v.Ed25519Pub}, PQAbUnix: v.Zeit, PQ: map[string]string{v.Ed25519Pub: v.MldsaPub}})
	feld := "v3:" + v.Ed25519Sig + "," + v.PQEintrag
	b := bescheinigungAusProveAnfrage([]byte(`{"bioAttestation":"` + feld + `","bioAttestationIssuedAt":` + strconv.FormatInt(v.Zeit, 10) + `}`))
	if b == nil {
		t.Fatal("Feld aus dem Coordinator nicht lesbar")
	}
	b.Modus = v.Modus
	tx := Transaction{Type: "register_human", Wallet: v.Wallet, RegAt: v.Zeit + 30,
		PubSignals: []string{"1", v.Nullifier}, BioBescheinigung: b}
	if err := pruefeBioBescheinigung(tx); err != nil {
		t.Fatalf("Bescheinigung aus dem Python-Coordinator abgelehnt: %v", err)
	}
	// Gegenprobe: ein Zeichen der ML-DSA-Unterschrift geaendert.
	kaputt := []byte(b.PQ[0])
	if kaputt[200] == '0' {
		kaputt[200] = '1'
	} else {
		kaputt[200] = '0'
	}
	b.PQ = []string{string(kaputt)}
	if err := pruefeBioBescheinigung(tx); err == nil {
		t.Fatal("veraenderte ML-DSA-Unterschrift angenommen")
	}
}
