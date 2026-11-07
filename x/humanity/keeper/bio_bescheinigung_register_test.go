package keeper

import (
	"crypto/ecdsa"
	"math"
	"strings"
	"testing"
)

// K1 mit den Schluesseln der Pruefer (bio_bescheinigung_register.go):
// Gutfall und je Angriff ein Test -- ohne Datenbank.

type b1bPruefer struct {
	mk     *ecdsa.PrivateKey
	mensch string
	k      bTestSchluessel
}

func neuerB1bPruefer(t *testing.T) b1bPruefer {
	t.Helper()
	mk, m := neuerSchluessel(t)
	return b1bPruefer{mk: mk, mensch: m, k: neuerEd25519(t)}
}

func (p b1bPruefer) bindung(t *testing.T) BescheinigungsBindung {
	t.Helper()
	return BescheinigungsBindung{PublicKey: p.k.pub, Mensch: p.mensch,
		MenschSig:     personalSign(t, p.mk, coordinatorFreigabeNachricht(p.k.pub)),
		SchluesselSig: p.k.unterschreibe(coordinatorBesitzNachricht(p.mensch))}
}

type b1bFall struct {
	coord, zeuge bTestSchluessel
	pruefer      map[string]bool
}

// neuerB1bFall: Genesis mit einem Coordinator- und einem Bezeuger-Schluessel,
// Quorum q, Pruefer-Schluessel ab bTestZeit-1000.
func neuerB1bFall(t *testing.T, q int) *b1bFall {
	t.Helper()
	f := &b1bFall{coord: neuerEd25519(t), zeuge: neuerEd25519(t), pruefer: map[string]bool{}}
	setBescheinigungFuerTest(t, &bescheinigungKonfig{AbUnix: 1, Modus: "test", Quorum: q,
		Coordinator: []string{f.coord.pub}, Bezeuger: []string{f.zeuge.pub}})
	vorher := bescheinigungRegisterOverride.Load()
	bescheinigungRegisterOverride.Store(bTestZeit - 1000)
	vorherG := verifierGruender
	t.Cleanup(func() {
		bescheinigungRegisterOverride.Store(vorher)
		verifierGruender = vorherG
	})
	verifierGruender = []string{"0x0000000000000000000000000000000000000abc"}
	return f
}

func (f *b1bFall) istPruefer(m string, t int64) bool {
	return f.pruefer[m] && t == bTestZeit
}

func (f *b1bFall) pruefe(tx Transaction, blockZeit int64) error {
	return pruefeBioBescheinigungMit(tx, blockZeit, f.istPruefer)
}

func mitBindungen(tx Transaction, b ...BescheinigungsBindung) Transaction {
	x := *tx.BioBescheinigung
	x.Bindungen = append([]BescheinigungsBindung(nil), b...)
	tx.BioBescheinigung = &x
	return tx
}

func TestB1b_PrueferCoordinatorBindet(t *testing.T) {
	f := neuerB1bFall(t, 1)
	p := neuerB1bPruefer(t)
	f.pruefer[p.mensch] = true
	tx := mitBindungen(bTestTx(t, bTestNullifier,
		bindung(p.k, "test", bTestNullifier, bTestWallet, bTestZeit),
		aussage(f.zeuge, "test", bTestNullifier, bTestZeit)), p.bindung(t))
	if err := f.pruefe(tx, bTestZeit+60); err != nil {
		t.Fatalf("gueltige Bescheinigung eines Pruefer-Coordinators abgewiesen: %v", err)
	}
}

func TestB1b_ZweiMenschenErfuellenQuorumZwei(t *testing.T) {
	f := neuerB1bFall(t, 2)
	p := neuerB1bPruefer(t)
	f.pruefer[p.mensch] = true
	tx := mitBindungen(bTestTx(t, bTestNullifier,
		bindung(f.coord, "test", bTestNullifier, bTestWallet, bTestZeit),
		aussage(p.k, "test", bTestNullifier, bTestZeit)), p.bindung(t))
	if err := f.pruefe(tx, bTestZeit+60); err != nil {
		t.Fatalf("Genesis + Pruefer = zwei Menschen: %v", err)
	}
	// Ohne den Pruefer nur ein Mensch (beide Genesis-Schluessel): abgewiesen.
	nur := bTestTx(t, bTestNullifier,
		bindung(f.coord, "test", bTestNullifier, bTestWallet, bTestZeit),
		aussage(f.zeuge, "test", bTestNullifier, bTestZeit))
	if err := f.pruefe(nur, bTestZeit+60); err != nil {
		// Ohne Bindungen zaehlt weiter je Schluessel (unveraendert).
		t.Fatalf("ohne Pruefer-Schluessel unveraendert je Schluessel gezaehlt: %v", err)
	}
}

// Ein Mensch mit zwei Schluesseln zaehlt einmal; die Schluessel der Gruender
// zaehlen mit der Genesis als einer.
func TestB1b_MenschenNichtSchluesselZaehlen(t *testing.T) {
	f := neuerB1bFall(t, 2)
	p := neuerB1bPruefer(t)
	p2 := b1bPruefer{mk: p.mk, mensch: p.mensch, k: neuerEd25519(t)}
	f.pruefer[p.mensch] = true
	tx := mitBindungen(bTestTx(t, bTestNullifier,
		bindung(p.k, "test", bTestNullifier, bTestWallet, bTestZeit),
		aussage(p2.k, "test", bTestNullifier, bTestZeit)), p.bindung(t), p2.bindung(t))
	if err := f.pruefe(tx, bTestZeit+60); err == nil || !strings.Contains(err.Error(), "Menschen") {
		t.Fatalf("ein Mensch mit zwei Schluesseln erfuellte Quorum 2: %v", err)
	}
	g := neuerB1bPruefer(t)
	verifierGruender = []string{g.mensch}
	f.pruefer[g.mensch] = true
	tx = mitBindungen(bTestTx(t, bTestNullifier,
		bindung(f.coord, "test", bTestNullifier, bTestWallet, bTestZeit),
		aussage(g.k, "test", bTestNullifier, bTestZeit)), g.bindung(t))
	if err := f.pruefe(tx, bTestZeit+60); err == nil {
		t.Fatal("Gruender-Schluessel + Genesis-Schluessel als zwei Menschen gezaehlt")
	}
}

func TestB1b_Missbrauch(t *testing.T) {
	f := neuerB1bFall(t, 1)
	p := neuerB1bPruefer(t)
	fremd := neuerB1bPruefer(t)
	f.pruefer[p.mensch] = true
	basis := func(k bTestSchluessel) Transaction {
		return bTestTx(t, bTestNullifier,
			bindung(k, "test", bTestNullifier, bTestWallet, bTestZeit),
			aussage(f.zeuge, "test", bTestNullifier, bTestZeit))
	}
	gut := p.bindung(t)
	faelle := []struct {
		name      string
		tx        Transaction
		blockZeit int64
		pruefer   func(string, int64) bool
	}{
		{"kein Pruefer", mitBindungen(basis(fremd.k), fremd.bindung(t)), bTestZeit + 60, f.istPruefer},
		{"Pruefer nur zu anderer Zeit", func() Transaction {
			x := mitBindungen(basis(p.k), gut)
			return x
		}(), bTestZeit + 60, func(m string, ti int64) bool { return f.pruefer[m] && ti != bTestZeit }},
		{"Freigabe einer fremden Wallet", mitBindungen(basis(p.k), func() BescheinigungsBindung {
			b := gut
			b.MenschSig = personalSign(t, fremd.mk, coordinatorFreigabeNachricht(p.k.pub))
			return b
		}()), bTestZeit + 60, f.istPruefer},
		{"Freigabe fuer einen anderen Schluessel", mitBindungen(basis(p.k), func() BescheinigungsBindung {
			b := gut
			b.MenschSig = personalSign(t, p.mk, coordinatorFreigabeNachricht(fremd.k.pub))
			return b
		}()), bTestZeit + 60, f.istPruefer},
		{"Besitznachweis fehlt", mitBindungen(basis(p.k), func() BescheinigungsBindung {
			b := gut
			b.SchluesselSig = fremd.k.unterschreibe(coordinatorBesitzNachricht(p.mensch))
			return b
		}()), bTestZeit + 60, f.istPruefer},
		{"fremder Schluessel unter dem Namen eines Pruefers", mitBindungen(basis(fremd.k), BescheinigungsBindung{
			PublicKey: fremd.k.pub, Mensch: p.mensch, MenschSig: gut.MenschSig,
			SchluesselSig: fremd.k.unterschreibe(coordinatorBesitzNachricht(p.mensch))}), bTestZeit + 60, f.istPruefer},
		{"Bindung fuer einen Genesis-Schluessel", mitBindungen(basis(f.coord), BescheinigungsBindung{
			PublicKey: f.coord.pub, Mensch: p.mensch, MenschSig: personalSign(t, p.mk, coordinatorFreigabeNachricht(f.coord.pub)),
			SchluesselSig: f.coord.unterschreibe(coordinatorBesitzNachricht(p.mensch))}), bTestZeit + 60, f.istPruefer},
		{"doppelte Bindung", mitBindungen(basis(p.k), gut, gut), bTestZeit + 60, f.istPruefer},
		{"Bindung ohne Unterschrift", mitBindungen(basis(f.coord), gut), bTestZeit + 60, f.istPruefer},
		{"zu spaet im Block", mitBindungen(basis(p.k), gut), bTestZeit + registerBindungFenster + 1, f.istPruefer},
		{"vor dem Stichtag", mitBindungen(basis(p.k), gut), bTestZeit - 1001, f.istPruefer},
		{"Register nicht lesbar", mitBindungen(basis(p.k), gut), bTestZeit + 60, nil},
		{"Grossschreibung", mitBindungen(basis(p.k), func() BescheinigungsBindung {
			b := gut
			b.Mensch = strings.ToUpper(b.Mensch[:2]) + b.Mensch[2:]
			return b
		}()), bTestZeit + 60, f.istPruefer},
	}
	for _, fa := range faelle {
		if err := pruefeBioBescheinigungMit(fa.tx, fa.blockZeit, fa.pruefer); err == nil {
			t.Errorf("%s: angenommen", fa.name)
		}
	}
	// Ohne Bindung bleibt ein Pruefer-Schluessel unbekannt.
	if err := f.pruefe(basis(p.k), bTestZeit+60); err == nil || !strings.Contains(err.Error(), "passt zu keinem") {
		t.Fatalf("Pruefer-Schluessel ohne Bindung: %v", err)
	}
	// Die alte Pruefung (ohne Register) nimmt keine Bindung an.
	if err := pruefeBioBescheinigung(mitBindungen(basis(p.k), gut)); err == nil {
		t.Fatal("pruefeBioBescheinigung ohne Register nahm eine Bindung an")
	}
}

func TestB1b_ProveRumpfTraegtBindungenNichtZumProofServer(t *testing.T) {
	p := neuerB1bPruefer(t)
	b := p.bindung(t)
	sig := strings.Repeat("ab", 64)
	rumpf := `{"bio":"123","wallet":"0x1","bioAttestation":"cc,v3:` + sig + `,v3k:` + b.BindungsText() + `","bioAttestationIssuedAt":1800000000}`
	got := bescheinigungAusProveAnfrage([]byte(rumpf))
	if got == nil || len(got.Bindungen) != 1 || got.Bindungen[0] != b || len(got.Signaturen) != 1 {
		t.Fatalf("gelesen: %+v", got)
	}
	if aus := string(proveAnfrageOhneV3([]byte(rumpf))); strings.Contains(aus, "v3k:") || strings.Contains(aus, "v3:") || !strings.Contains(aus, `"cc"`) {
		t.Fatalf("an den Proof-Server: %s", aus)
	}
	// Eine kaputte Bindung: keine Bescheinigung (die Registrierung scheitert).
	kaputt := strings.Replace(rumpf, b.BindungsText(), "xx.yy", 1)
	if bescheinigungAusProveAnfrage([]byte(kaputt)) != nil {
		t.Fatal("kaputte Bindung gelesen")
	}
	if _, ok := leseBescheinigungsBindung(strings.ToUpper(b.BindungsText())); ok {
		t.Fatal("Grossschreibung als Bindung gelesen")
	}
}

func TestB1b_NieVorRegister(t *testing.T) {
	if bescheinigungRegisterAbUnix < verifierRegisterAbUnix {
		t.Fatalf("K1 mit Pruefer-Schluesseln (%d) vor dem Pruefer-Register (%d)", bescheinigungRegisterAbUnix, verifierRegisterAbUnix)
	}
	if verifierWiederaufnahmePause <= registerBindungFenster+erzeugerFrist {
		t.Fatal("Wiederaufnahme-Pause muss das Fenster der Registrierung samt Frist uebersteigen")
	}
}

// Wiederaufnahme fruehestens verifierWiederaufnahmePause nach dem Ende.
func TestB1b_WiederaufnahmeErstNachPause(t *testing.T) {
	f := neuerVrFall(t)
	vorher := bescheinigungRegisterOverride.Load()
	bescheinigungRegisterOverride.Store(vrAb)
	t.Cleanup(func() { bescheinigungRegisterOverride.Store(vorher) })
	z := vrAb + 1000
	f.aufnehmen(1, z, 0)
	aus := z + erzeugerFrist + 100
	if neu := f.anwenden(f.tx(verifierArtAustritt, f.m(1), aus, 1), aus); len(neu) != 0 {
		t.Fatalf("Austritt: %v", neu)
	}
	ende := aus + erzeugerFrist
	zuFrueh := ende + verifierWiederaufnahmePause - 10
	if neu := f.anwenden(f.tx(verifierArtAufnahme, f.m(1), zuFrueh, 0), zuFrueh); neu["verifier_pause"] != 1 {
		t.Fatalf("Wiederaufnahme vor der Pause: %v", neu)
	}
	spaet := ende + verifierWiederaufnahmePause + 10
	if neu := f.anwenden(f.tx(verifierArtAufnahme, f.m(1), spaet, 0), spaet); len(neu) != 0 {
		t.Fatalf("Wiederaufnahme nach der Pause: %v", neu)
	}
	_ = math.MaxInt64
}
