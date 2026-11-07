package keeper

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"math"
	"strings"
	"testing"
)

// Das Pruefer-Register mit Abstimmung (verifier_register.go), ohne
// Datenbank: Gutfall, Wirkung auf die Coordinator-Zulassung und je Angriff
// ein Missbrauchstest.

var vrRegeln = []string{"verifier_form", "verifier_zeit", "verifier_wiederholt", "verifier_schon", "verifier_voll",
	"verifier_kein_mitglied", "verifier_kein_mensch", "verifier_quorum", "verifier_stimme_zeit", "verifier_stimme", "verifier_pause"}

func vrZaehler() map[string]int64 {
	m := map[string]int64{}
	for _, r := range vrRegeln {
		m[r] = erhaltungZaehler(r)
	}
	return m
}

func vrNeu(vorher map[string]int64) map[string]int64 {
	m := map[string]int64{}
	for _, r := range vrRegeln {
		if d := erhaltungZaehler(r) - vorher[r]; d != 0 {
			m[r] = d
		}
	}
	return m
}

const vrAb int64 = 1_800_000_000

type vrFall struct {
	t          *testing.T
	cs         *ChainState
	menschen   []seBetreiber // 0 = Gruender, 1..5 = weitere Menschen
	schluessel map[string]bool
}

// neuerVrFall: Register aktiv ab vrAb, Coordinatoren im Konsens lesbar, sechs
// registrierte Menschen mit Coordinator-Schluessel und Validator-Schluessel;
// Mensch 0 ist der einzige Gruender.
func neuerVrFall(t *testing.T) *vrFall {
	t.Helper()
	vorherLeser := registerLeserOverride.Load()
	registerLeserOverride.Store(1)
	vorherAb := verifierRegisterOverride.Load()
	verifierRegisterOverride.Store(vrAb)
	vorherGruender := verifierGruender
	t.Cleanup(func() {
		registerLeserOverride.Store(vorherLeser)
		verifierRegisterOverride.Store(vorherAb)
		verifierGruender = vorherGruender
	})
	f := &vrFall{t: t, cs: newTestState(), schluessel: map[string]bool{}}
	for i := 0; i < 6; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		mk, mensch := neuerSchluessel(t)
		pubHex := hex.EncodeToString(pub)
		f.menschen = append(f.menschen, seBetreiber{priv: priv, pubHex: pubHex, bindung: CoordinatorBindung{Mensch: mensch,
			MenschSig:     personalSign(t, mk, coordinatorFreigabeNachricht(pubHex)),
			SchluesselSig: hex.EncodeToString(ed25519.Sign(priv, []byte(coordinatorBesitzNachricht(mensch))))}})
		addHuman(f.cs, mensch, 1000)
		f.schluessel[mensch] = true // haelt einen Validator-Schluessel
	}
	verifierGruender = []string{f.m(0)}
	f.cs.zulassungOhneDB = func(m string, _ int64) (bool, error) { return f.schluessel[m], nil }
	return f
}

func (f *vrFall) m(i int) string { return f.menschen[i].bindung.Mensch }

func (f *vrFall) stimme(von int, art, betroffener string, zeit int64) VerifierStimme {
	b := f.menschen[von]
	sig := ed25519.Sign(b.priv, []byte(verifierNachricht(art, betroffener)("", zeit)))
	return VerifierStimme{Zeit: zeit, Bescheinigung: Lebendigkeitsbescheinigung{PublicKey: b.pubHex,
		Signature: hex.EncodeToString(sig), Mensch: b.bindung.Mensch, MenschSig: b.bindung.MenschSig,
		SchluesselSig: b.bindung.SchluesselSig}}
}

func (f *vrFall) tx(art, betroffener string, zeit int64, von ...int) Transaction {
	z := &VerifierZulassung{Art: art}
	for _, i := range von {
		z.Stimmen = append(z.Stimmen, f.stimme(i, art, betroffener, zeit))
	}
	return Transaction{Type: verifierTxArt, Wallet: betroffener, DistributionAt: zeit, Verifier: z}
}

func (f *vrFall) anwenden(tx Transaction, blockZeit int64) map[string]int64 {
	f.t.Helper()
	vorher := vrZaehler()
	f.cs.mu.Lock()
	err := f.cs.applyVerifierZulassungLocked(context.Background(), &tx, blockZeit)
	f.cs.mu.Unlock()
	if err != nil {
		f.t.Fatalf("Fehler statt Befund: %v", err)
	}
	return vrNeu(vorher)
}

func (f *vrFall) ist(i int, t int64) bool {
	return f.cs.verifierRegisterJetzt().mitglied(f.m(i), t)
}

// aufnehmen: Mensch i wird mit den Stimmen von zu zeit aufgenommen.
func (f *vrFall) aufnehmen(i int, zeit int64, von ...int) {
	f.t.Helper()
	if neu := f.anwenden(f.tx(verifierArtAufnahme, f.m(i), zeit, von...), zeit); len(neu) != 0 {
		f.t.Fatalf("Aufnahme von %d abgelehnt: %v", i, neu)
	}
}

func TestVerifierRegister_GruenderNimmtDenZweitenAuf(t *testing.T) {
	f := neuerVrFall(t)
	z := vrAb + 1000
	if !f.ist(0, z) || f.ist(1, z) {
		t.Fatal("am Anfang ist nur der Gruender Pruefer")
	}
	// Einer von einem: zwei Drittel von 1, aufgerundet, ist 1.
	f.aufnehmen(1, z, 0)
	w := z + erzeugerFrist
	if f.ist(1, w-1) || !f.ist(1, w) {
		t.Fatalf("Wirkung erst ab issued_at + erzeugerFrist (%d)", w)
	}
	// Jetzt zwei: fuer den dritten braucht es beide.
	z2 := w + 100
	if neu := f.anwenden(f.tx(verifierArtAufnahme, f.m(2), z2, 0), z2); neu["verifier_quorum"] != 1 {
		t.Fatalf("eine von zwei Stimmen darf nicht reichen: %v", neu)
	}
	f.aufnehmen(2, z2+1, 0, 1)
	if !f.ist(2, z2+1+erzeugerFrist) {
		t.Fatal("mit beiden Stimmen aufgenommen")
	}
}

func TestVerifierRegister_Schwelle(t *testing.T) {
	for n, want := range map[int]int{0: 1, 1: 1, 2: 2, 3: 2, 4: 3, 5: 4, 6: 4, 9: 6, 10: 7, 256: 171} {
		if got := verifierSchwelle(n); got != want {
			t.Fatalf("Schwelle(%d) = %d, erwartet %d", n, got, want)
		}
	}
	if verifierSchwelle(verifierHoechstens) != verifierStimmenHoechstens {
		t.Fatal("verifierStimmenHoechstens muss die Schwelle bei vollem Register tragen")
	}
}

// Missbrauch (#310 H1): ein Mensch mit frischem Validator-Schluessel, aber
// ohne Aufnahme, bescheinigt. Bisher genuegte der Schluessel.
func TestVerifierRegister_SchluesselAlleinReichtNichtMehr(t *testing.T) {
	f := neuerVrFall(t)
	z := vrAb + 1000
	zul := f.cs.lebensnachweisZulassung(context.Background())
	if ok, _ := zul(f.m(3), z); ok {
		t.Fatal("Validator-Schluessel ohne Aufnahme darf nicht als Coordinator zaehlen")
	}
	if ok, _ := zul(f.m(0), z); !ok {
		t.Fatal("der Gruender mit Schluessel ist zugelassen")
	}
	// Vor dem Stichtag wie bisher: der Schluessel genuegt.
	if ok, _ := zul(f.m(3), vrAb-1); !ok {
		t.Fatal("vor dem Stichtag zaehlt nur der Validator-Schluessel")
	}
	// Ohne Validator-Schluessel bleibt auch ein Pruefer draussen.
	f.schluessel[f.m(0)] = false
	if ok, _ := zul(f.m(0), z); ok {
		t.Fatal("ohne Validator-Schluessel nicht zugelassen")
	}
}

// Missbrauch: ein Mensch, der kein Pruefer ist, stimmt mit.
func TestVerifierRegister_FremdeStimmeZaehltNicht(t *testing.T) {
	f := neuerVrFall(t)
	z := vrAb + 1000
	if neu := f.anwenden(f.tx(verifierArtAufnahme, f.m(1), z, 3), z); neu["verifier_stimme"] != 1 {
		t.Fatalf("Stimme eines Nicht-Pruefers: %v", neu)
	}
	if f.ist(1, z+erzeugerFrist) {
		t.Fatal("aufgenommen ohne gueltige Stimme")
	}
}

// Missbrauch: der Kandidat stimmt fuer sich selbst.
func TestVerifierRegister_KeineSelbststimme(t *testing.T) {
	f := neuerVrFall(t)
	f.aufnehmen(1, vrAb+1000, 0)
	z := vrAb + 1000 + erzeugerFrist + 10
	// Mensch 1 ist Pruefer; fuer die Aufnahme von 2 braucht es 2 von 2.
	// Mensch 2 stimmt fuer sich: abgelehnt.
	if neu := f.anwenden(f.tx(verifierArtAufnahme, f.m(2), z, 0, 2), z); neu["verifier_stimme"] != 1 {
		t.Fatalf("Selbststimme: %v", neu)
	}
}

// Missbrauch: derselbe Mensch stimmt zweimal (auch mit verschiedener
// Stimmzeit) -- abgewiesen, bevor eine Stimme etwas kostet.
func TestVerifierRegister_MenschZaehltEinmal(t *testing.T) {
	f := neuerVrFall(t)
	f.aufnehmen(1, vrAb+1000, 0)
	z := vrAb + 1000 + erzeugerFrist + 10
	tx := f.tx(verifierArtAufnahme, f.m(2), z, 0)
	tx.Verifier.Stimmen = append(tx.Verifier.Stimmen, f.stimme(0, verifierArtAufnahme, f.m(2), z-5))
	if neu := f.anwenden(tx, z); neu["verifier_stimme"] != 1 {
		t.Fatalf("zwei Stimmen desselben Menschen: %v", neu)
	}
}

// Missbrauch: dieselbe Transaktion noch einmal, und alte Stimmen nach einem
// Entzug fuer eine neue Aufnahme.
func TestVerifierRegister_KeineWiederholung(t *testing.T) {
	f := neuerVrFall(t)
	z := vrAb + 1000
	tx := f.tx(verifierArtAufnahme, f.m(1), z, 0)
	f.anwenden(tx, z)
	if neu := f.anwenden(tx, z+10); neu["verifier_wiederholt"] != 1 {
		t.Fatalf("Wiederholung: %v", neu)
	}
	// Mensch 1 tritt aus; die alte Stimme des Gruenders nimmt ihn nicht
	// wieder auf, auch in einer neuen Transaktion.
	za := z + erzeugerFrist + 100
	if neu := f.anwenden(f.tx(verifierArtAustritt, f.m(1), za, 1), za); len(neu) != 0 {
		t.Fatalf("Austritt: %v", neu)
	}
	zn := za + erzeugerFrist + 100
	alt := f.tx(verifierArtAufnahme, f.m(1), zn, 0)
	alt.Verifier.Stimmen[0] = f.stimme(0, verifierArtAufnahme, f.m(1), z) // Stimmzeit vor dem Austritt
	if neu := f.anwenden(alt, zn); neu["verifier_stimme_zeit"] != 1 {
		t.Fatalf("alte Stimme nach dem Austritt: %v", neu)
	}
	f.aufnehmen(1, zn+1, 0) // mit frischer Stimme geht es
	if !f.ist(1, zn+1+erzeugerFrist) {
		t.Fatal("neu aufgenommen")
	}
}

// Entzug: zwei Drittel der UEBRIGEN, nie rueckwirkend.
func TestVerifierRegister_EntzugNieRueckwirkend(t *testing.T) {
	f := neuerVrFall(t)
	z := vrAb + 1000
	f.aufnehmen(1, z, 0)
	z2 := z + erzeugerFrist + 10
	f.aufnehmen(2, z2, 0, 1)
	z3 := z2 + erzeugerFrist + 10 // jetzt drei Pruefer
	// Entzug von 2: noetig 2 von den 2 uebrigen.
	if neu := f.anwenden(f.tx(verifierArtEntzug, f.m(2), z3, 0), z3); neu["verifier_quorum"] != 1 {
		t.Fatalf("eine Stimme fuer einen Entzug von dreien: %v", neu)
	}
	// Der Betroffene selbst zaehlt nicht mit.
	if neu := f.anwenden(f.tx(verifierArtEntzug, f.m(2), z3+1, 0, 2), z3+1); neu["verifier_stimme"] != 1 {
		t.Fatalf("Stimme des Betroffenen beim Entzug: %v", neu)
	}
	if neu := f.anwenden(f.tx(verifierArtEntzug, f.m(2), z3+2, 0, 1), z3+2); len(neu) != 0 {
		t.Fatalf("Entzug mit zwei Dritteln: %v", neu)
	}
	ende := z3 + 2 + erzeugerFrist
	// Fruehere Bescheinigungen bleiben gueltig: zu jeder Zeit davor ist er Pruefer.
	if !f.ist(2, z2+erzeugerFrist) || !f.ist(2, ende-1) || f.ist(2, ende) {
		t.Fatalf("Entzug wirkt ab %d, nie frueher", ende)
	}
}

func TestVerifierRegister_AlleinNurAustritt(t *testing.T) {
	f := neuerVrFall(t)
	z := vrAb + 1000
	if neu := f.anwenden(f.tx(verifierArtEntzug, f.m(0), z, 0), z); neu["verifier_stimme"]+neu["verifier_quorum"] != 1 {
		t.Fatalf("der einzige Pruefer entzieht sich nicht: %v", neu)
	}
	// Austritt nur durch ihn selbst.
	if neu := f.anwenden(f.tx(verifierArtAustritt, f.m(0), z+1, 1), z+1); neu["verifier_stimme"] != 1 {
		t.Fatalf("Austritt durch einen anderen: %v", neu)
	}
	if neu := f.anwenden(f.tx(verifierArtAustritt, f.m(0), z+2, 0), z+2); len(neu) != 0 {
		t.Fatalf("Austritt des Gruenders: %v", neu)
	}
	if !f.ist(0, z+2+erzeugerFrist-1) || f.ist(0, z+2+erzeugerFrist) {
		t.Fatal("der Gruender ist bis zur Wirkung Pruefer, danach nicht mehr")
	}
}

func TestVerifierRegister_KandidatMussMenschSein(t *testing.T) {
	f := neuerVrFall(t)
	z := vrAb + 1000
	fremd := "0x" + strings.Repeat("ab", 20)
	if neu := f.anwenden(f.tx(verifierArtAufnahme, fremd, z, 0), z); neu["verifier_kein_mensch"] != 1 {
		t.Fatalf("Aufnahme eines Nicht-Menschen: %v", neu)
	}
	if neu := f.anwenden(f.tx(verifierArtAufnahme, f.m(0), z, 0), z); neu["verifier_schon"] != 1 {
		t.Fatalf("der Gruender ist schon Pruefer: %v", neu)
	}
}

// Missbrauch: alte oder vordatierte Zeiten.
func TestVerifierRegister_Zeiten(t *testing.T) {
	f := neuerVrFall(t)
	z := vrAb + 100_000
	// issued_at mehr als nachweisHoechstensAlt vor dem Block.
	if neu := f.anwenden(f.tx(verifierArtAufnahme, f.m(1), z, 0), z+nachweisHoechstensAlt+1); neu["verifier_zeit"] != 1 {
		t.Fatalf("zu alt: %v", neu)
	}
	if neu := f.anwenden(f.tx(verifierArtAufnahme, f.m(1), z+nachweisHoechstensVoraus+1, 0), z); neu["verifier_zeit"] != 1 {
		t.Fatalf("aus der Zukunft: %v", neu)
	}
	// Stimme aelter als 14 Tage.
	tx := f.tx(verifierArtAufnahme, f.m(1), z, 0)
	tx.Verifier.Stimmen[0] = f.stimme(0, verifierArtAufnahme, f.m(1), z-verifierStimmeHoechstensAlt-1)
	if neu := f.anwenden(tx, z); neu["verifier_stimme_zeit"] != 1 {
		t.Fatalf("verjaehrte Stimme: %v", neu)
	}
	// Vor dem Stichtag bescheinigt.
	if neu := f.anwenden(f.tx(verifierArtAufnahme, f.m(1), vrAb-1, 0), vrAb); neu["verifier_zeit"] != 1 {
		t.Fatalf("vor dem Stichtag: %v", neu)
	}
}

// Missbrauch: eine Stimme fuer einen anderen Kandidaten oder eine andere Art
// wird umgehaengt.
func TestVerifierRegister_StimmeGiltNurFuerIhrenSatz(t *testing.T) {
	f := neuerVrFall(t)
	z := vrAb + 1000
	tx := f.tx(verifierArtAufnahme, f.m(1), z, 0)
	tx.Wallet = f.m(2)
	if neu := f.anwenden(tx, z); neu["verifier_stimme"] != 1 {
		t.Fatalf("Stimme fuer einen anderen Kandidaten: %v", neu)
	}
	f.aufnehmen(1, z+1, 0)
	z2 := z + 1 + erzeugerFrist + 10
	ent := f.tx(verifierArtAufnahme, f.m(1), z2, 0)
	ent.Verifier.Art = verifierArtEntzug
	if neu := f.anwenden(ent, z2); neu["verifier_stimme"] != 1 {
		t.Fatalf("Aufnahme-Stimme als Entzug: %v", neu)
	}
}

func TestVerifierRegister_VorDemStichtagOhneWirkung(t *testing.T) {
	// TestMain haelt das Register schlafend (verifierRegisterOverride).
	if verifierRegisterAktiv(math.MaxInt64-1) || bekannteTxArt(verifierTxArt, 2_000_000_000) {
		t.Fatal("schlafend: unbekannte Art, keine Wirkung")
	}
	cs := newTestState()
	if !cs.verifierZugelassen("0x"+strings.Repeat("cd", 20), 2_000_000_000) {
		t.Fatal("schlafend: die Zulassung bleibt wie bisher")
	}
	// Mit dem Stichtag: vorher bekannt nicht, danach ja.
	vorher := verifierRegisterOverride.Load()
	verifierRegisterOverride.Store(verifierRegisterAbUnix)
	t.Cleanup(func() { verifierRegisterOverride.Store(vorher) })
	if bekannteTxArt(verifierTxArt, verifierRegisterAbUnix-1) || !bekannteTxArt(verifierTxArt, verifierRegisterAbUnix) {
		t.Fatal("die Art gilt genau ab dem Stichtag")
	}
}

func TestVerifierRegister_NieVorRegisterLeser(t *testing.T) {
	if verifierRegisterAbUnix != math.MaxInt64 && verifierRegisterAbUnix < registerLeserAbUnix {
		t.Fatalf("Pruefer-Register ab %d, Coordinatoren erst ab %d im Konsens", verifierRegisterAbUnix, registerLeserAbUnix)
	}
	for _, g := range verifierGruender {
		if !kanonischeAdresse(g) {
			t.Fatalf("Gruender %q nicht kanonisch", g)
		}
	}
}

func TestVerifierRegister_LesenNurKanonisch(t *testing.T) {
	a, b := "0x"+strings.Repeat("11", 20), "0x"+strings.Repeat("22", 20)
	gut := &verifierRegister{Eintraege: []verifierEintrag{{Mensch: a, Seit: 10, Zuletzt: 5}, {Mensch: b, Seit: 10, Bis: 20, Zuletzt: 9}}}
	if r, err := verifierRegisterLesen(gut.kodiert()); err != nil || r.kodiert() != gut.kodiert() {
		t.Fatalf("Rundreise: %v", err)
	}
	if r, err := verifierRegisterLesen(""); err != nil || !r.leer() {
		t.Fatalf("leer: %v", err)
	}
	for _, schlecht := range []string{
		`{"e":[{"m":"` + b + `","seit":10,"z":5},{"m":"` + a + `","seit":10,"z":5}]}`, // nicht sortiert
		`{"e":[{"m":"` + a + `","seit":10,"bis":5,"z":5}]}`,                           // Ende vor Beginn
		`{"e":[{"m":"0xABC","seit":10,"z":5}]}`,                                       // keine Adresse
		`{"e":[{"m":"` + a + `","seit":0,"z":5}]}`,                                    // ohne Beginn
		`{"e":[{"z":5,"m":"` + a + `","seit":10}]}`,                                   // nicht kanonisch
		`{"e":[],"x":1}`, // unbekanntes Feld
		`kaputt`,
	} {
		if _, err := verifierRegisterLesen(schlecht); err == nil {
			t.Fatalf("angenommen: %s", schlecht)
		}
	}
}

func TestVerifierRegister_StateRootUndRueckrollen(t *testing.T) {
	f := neuerVrFall(t)
	f.cs.mu.Lock()
	ohne := f.cs.stateRootLocked("", "")
	snap := f.cs.snapshotForRollbackLocked(nil, true, nil)
	f.cs.mu.Unlock()
	// Ein leeres Register aendert die Wurzel nicht (byte-gleich).
	f.cs.verifierReg.Store(&verifierRegister{})
	f.cs.mu.Lock()
	if f.cs.stateRootLocked("", "") != ohne {
		f.cs.mu.Unlock()
		t.Fatal("ohne Eintraege byte-gleich")
	}
	f.cs.mu.Unlock()
	f.aufnehmen(1, vrAb+1000, 0)
	f.cs.mu.Lock()
	defer f.cs.mu.Unlock()
	if f.cs.stateRootLocked("", "") == ohne {
		t.Fatal("ein Eintrag muss in der StateRoot stehen")
	}
	if err := f.cs.restoreFromRollbackLockedCtx(context.Background(), snap); err != nil {
		t.Fatal(err)
	}
	if !f.cs.verifierRegisterJetzt().leer() || f.cs.stateRootLocked("", "") != ohne {
		t.Fatal("der Rueckroll muss das Register zuruecksetzen")
	}
}

// Ehemalige fuellen das Register nicht auf Dauer: ein beendeter Eintrag
// faellt 30 Tage nach seinem Ende bei der naechsten Aenderung weg -- ein
// Gruender nie (ohne Eintrag waere er wieder Pruefer).
func TestVerifierRegister_EhemaligeFallenWeg(t *testing.T) {
	f := neuerVrFall(t)
	z := vrAb + 1000
	f.aufnehmen(1, z, 0)
	za := z + erzeugerFrist + 10
	if neu := f.anwenden(f.tx(verifierArtAustritt, f.m(1), za, 1), za); len(neu) != 0 {
		t.Fatalf("Austritt: %v", neu)
	}
	// Der Gruender tritt aus und bleibt draussen.
	if neu := f.anwenden(f.tx(verifierArtAustritt, f.m(0), za+1, 0), za+1); len(neu) != 0 {
		t.Fatalf("Austritt des Gruenders: %v", neu)
	}
	if n := len(f.cs.verifierRegisterJetzt().Eintraege); n != 2 {
		t.Fatalf("zwei beendete Eintraege, gefunden %d", n)
	}
	if !verifierVeraltet(verifierEintrag{Mensch: f.m(1), Seit: 1, Bis: za}, za+erzeugerFrist+verifierEndeAufbewahren+1) {
		t.Fatal("ein Ehemaliger veraltet nach 30 Tagen")
	}
	if verifierVeraltet(verifierEintrag{Mensch: f.m(0), Seit: 1, Bis: za}, za+365*86400) {
		t.Fatal("ein Gruender veraltet nie")
	}
	// Spaeter: das neue Register haelt den Ehemaligen nicht mehr, den
	// Gruender schon.
	reg := f.cs.verifierRegisterJetzt().mitSetzen(verifierEintrag{Mensch: f.m(2), Seit: 1, Zuletzt: za + 60*86400})
	if _, da := reg.eintrag(f.m(1)); da {
		t.Fatal("der Ehemalige muss wegfallen")
	}
	if _, da := reg.eintrag(f.m(0)); !da || reg.mitglied(f.m(0), za+60*86400) {
		t.Fatal("der ausgetretene Gruender bleibt eingetragen und draussen")
	}
}
