package keeper

import (
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// Die Stichtage in der richtigen Folge: die Leser fruehestens eine Woche
// nach dem Register (bestehende Betreiber binden vorher neu), die
// Erzeugerpruefung nicht vor den Lesern.
func TestRegisterLeser_StichtageInDerRichtigenFolge(t *testing.T) {
	if registerLeserAbUnix != math.MaxInt64 {
		if validatorRegisterAbUnix == math.MaxInt64 || registerLeserAbUnix-validatorRegisterAbUnix < registerLeserVorlauf {
			t.Fatalf("registerLeserAbUnix %d keine Woche nach validatorRegisterAbUnix %d", registerLeserAbUnix, validatorRegisterAbUnix)
		}
	}
	if erzeugerSchnittAbUnix != math.MaxInt64 {
		if registerLeserAbUnix == math.MaxInt64 || erzeugerSchnittAbUnix < registerLeserAbUnix {
			t.Fatalf("erzeugerSchnittAbUnix %d vor registerLeserAbUnix %d", erzeugerSchnittAbUnix, registerLeserAbUnix)
		}
	}
}

// Die Frist reicht ueber das Fenster, in dem der Zeitpunkt einer Bindung vor
// ihrem Block liegen darf -- sonst wirkte eine rueckdatierte Bindung sofort,
// und ein Knoten, der den Block noch nicht nachgespielt hat, urteilte anders.
func TestErzeugerFrist_UeberDemNachweisfenster(t *testing.T) {
	if erzeugerFrist < nachweisHoechstensAlt+3600 {
		t.Fatalf("erzeugerFrist %d: weniger als eine Stunde nach nachweisHoechstensAlt (%d)", erzeugerFrist, nachweisHoechstensAlt)
	}
}

func z(betreiber, signing string, zeit int64) bindungsZeile {
	return bindungsZeile{betreiber: betreiber, signing: signing, zeit: zeit}
}

const ewig = int64(math.MaxInt64)

// Die Zeitraeume: eine Bindung endet mit der naechsten ihres Betreibers oder
// wenn ein anderer Betreiber den Schluessel spaeter bindet; gleicher
// Zeitpunkt zweier Betreiber: beide (umstritten). Unabhaengig von der
// Reihenfolge der Zeilen.
func TestBindungsIntervalle(t *testing.T) {
	for name, fall := range map[string]struct {
		zeilen []bindungsZeile
		want   []bindungsIntervall
	}{
		"eine": {[]bindungsZeile{z("v", "k", 100)}, []bindungsIntervall{{"v", "k", 100, ewig}}},
		"Wechsel": {[]bindungsZeile{z("v", "k", 100), z("v", "k2", 200)},
			[]bindungsIntervall{{"v", "k", 100, 200}, {"v", "k2", 200, ewig}}},
		"Uebernahme": {[]bindungsZeile{z("v", "k", 100), z("b", "k", 150)},
			[]bindungsIntervall{{"v", "k", 100, 150}, {"b", "k", 150, ewig}}},
		"Uebernahme, dann weiter": {[]bindungsZeile{z("v", "k", 100), z("b", "k", 150), z("b", "k3", 300)},
			[]bindungsIntervall{{"v", "k", 100, 150}, {"b", "k", 150, 300}, {"b", "k3", 300, ewig}}},
		"Wechsel, dann Uebernahme": {[]bindungsZeile{z("v", "k", 100), z("v", "k2", 200), z("b", "k", 300)},
			[]bindungsIntervall{{"v", "k", 100, 200}, {"b", "k", 300, ewig}, {"v", "k2", 200, ewig}}},
		"umstritten": {[]bindungsZeile{z("v", "k", 100), z("w", "k", 100)},
			[]bindungsIntervall{{"v", "k", 100, ewig}, {"w", "k", 100, ewig}}},
		"leerer Zeitraum": {[]bindungsZeile{z("v", "k", 100), z("v", "kz", 100)},
			[]bindungsIntervall{{"v", "kz", 100, ewig}}},
		"frueherer Betreiber nach dem spaeteren": {[]bindungsZeile{z("b", "k", 300), z("v", "k", 100)},
			[]bindungsIntervall{{"v", "k", 100, 300}, {"b", "k", 300, ewig}}},
	} {
		got := bindungsIntervalle(fall.zeilen)
		if !reflect.DeepEqual(got, fall.want) {
			t.Fatalf("%s: %+v, erwartet %+v", name, got, fall.want)
		}
		rand.New(rand.NewSource(1)).Shuffle(len(fall.zeilen), func(i, j int) { fall.zeilen[i], fall.zeilen[j] = fall.zeilen[j], fall.zeilen[i] })
		if again := bindungsIntervalle(fall.zeilen); !reflect.DeepEqual(again, got) {
			t.Fatalf("%s: andere Reihenfolge, andere Zeitraeume: %+v", name, again)
		}
	}
}

// standAus: der Erzeugerstand aus Zeilen, jeder Betreiber ein Mensch.
func standAus(zeilen ...bindungsZeile) *erzeugerStand {
	menschen := map[string]bool{}
	for _, r := range zeilen {
		menschen[r.betreiber] = true
	}
	return &erzeugerStand{fenster: fensterAusIntervallen(bindungsIntervalle(zeilen), menschen, nil),
		halter: letzteHalter(zeilen, menschen, nil), zeit: time.Now()}
}

// Die Frist: eine Bindung wirkt erst erzeugerFrist nach ihrem Zeitpunkt, ihr
// Ende ebenso.
func TestErzeugerFenster_Frist(t *testing.T) {
	st := standAus(z("v", "k", 1000), z("v", "k2", 5000))
	D := erzeugerFrist
	for _, f := range []struct {
		addr string
		t    int64
		want string
	}{
		{"k", 1000 + D - 1, ""}, {"k", 1000 + D, "v"}, {"k", 5000 + D - 1, "v"}, {"k", 5000 + D, ""},
		{"k2", 5000 + D - 1, ""}, {"k2", 5000 + D, "v"}, {"k2", 1 << 40, "v"},
	} {
		if got := st.erzeugerFenster(f.addr, f.t); got != f.want {
			t.Fatalf("%s zur Zeit %d: %q, erwartet %q", f.addr, f.t, got, f.want)
		}
	}
	if got := standAus(z("v", "k", 100), z("w", "k", 100)).erzeugerFenster("k", 100+D); got != "" {
		t.Fatalf("umstrittener Schluessel erzeugt fuer %q", got)
	}
}

// Der Kern von HIGH 1 (#303): eine Zeile, die ein Knoten noch nicht kennt,
// aendert sein Urteil ueber keinen Block, dessen Zeit vor ihrem Zeitpunkt
// plus Frist liegt -- fuer jeden Schluessel, bei jeder Vorgeschichte. Wer
// den Block mit der Bindung innerhalb der Frist nachspielt, urteilt also
// wie alle anderen.
func TestErzeugerFenster_UnbekannteZeileAendertNichtsVorIhrerFrist(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	betreiber := []string{"a", "b", "c"}
	schluessel := []string{"k1", "k2", "k3"}
	D := erzeugerFrist
	zeile := func() bindungsZeile {
		return z(betreiber[r.Intn(3)], schluessel[r.Intn(3)], int64(r.Intn(8))*D/2)
	}
	geaendert := 0
	for lauf := 0; lauf < 3000; lauf++ {
		var zeilen []bindungsZeile
		for i := r.Intn(6); i > 0; i-- {
			zeilen = append(zeilen, zeile())
		}
		neu := zeile()
		ohne, mit := standAus(zeilen...), standAus(append(append([]bindungsZeile(nil), zeilen...), neu)...)
		for _, k := range schluessel {
			for t0 := int64(-D); t0 < 6*D; t0 += D / 4 {
				a, b := ohne.erzeugerFenster(k, t0), mit.erzeugerFenster(k, t0)
				if t0 < neu.zeit+D && a != b {
					t.Fatalf("Zeilen %+v, neu %+v: %s zur Zeit %d ohne %q, mit %q", zeilen, neu, k, t0, a, b)
				}
				if a != b {
					geaendert++
				}
			}
		}
	}
	if geaendert == 0 {
		t.Fatal("keine Zeile hat je etwas geaendert -- der Test beweist nichts")
	}
}

// erzeugerTestDAG: geschlossene Liste {A, B}, eigener Schluessel S, dazu X,
// den ein Peer im Abgleich gemeldet hat (authorizedValidators). st = nil:
// noch kein Stand.
func erzeugerTestDAG(st *erzeugerStand) *BlockDAG {
	cs := newTestState()
	if st != nil {
		cs.erzeugerRegister.Store(st)
	}
	dag := newOrphanTestDAG()
	dag.state = cs
	dag.produzentenFest = map[string]bool{"0xa": true, "0xb": true}
	dag.selfProposer = "0xs"
	dag.authorizedValidators = map[string]bool{"0xa": true, "0xb": true, "0xs": true, "0xx": true}
	return dag
}

// registerStand: A, C, S, X gebunden (seit Langem), B nicht.
func registerStand() *erzeugerStand {
	return standAus(z("ma", "0xa", 0), z("mc", "0xc", 0), z("ms", "0xs", 0), z("mx", "0xx", 0))
}

// Die Erzeugerpruefung: vor dem Stichtag wie bisher; danach die
// Schnittmenge -- das Register nimmt weg (B ohne Bindung), fuegt aber nichts
// hinzu (C steht nur im Register), und was ein Peer im Abgleich meldete (X),
// zaehlt nicht mehr. Kein Stand oder ein Lesefehler: niemand (fail-closed).
func TestErzeugerErlaubt_SchnittmengeUndFailClosed(t *testing.T) {
	ab := int64(2_000_000_000)
	erzeugerSchnittOverride.Store(ab)
	t.Cleanup(func() { erzeugerSchnittOverride.Store(0) })
	vorher := ab - zeitstempelRueckToleranz - 1

	dag := erzeugerTestDAG(registerStand())
	for addr, want := range map[string]bool{"0xa": true, "0xb": true, "0xs": true, "0xx": true, "0xc": false} {
		if got := dag.erzeugerErlaubt(addr, vorher); got != want {
			t.Fatalf("vor dem Stichtag %s: %v, erwartet %v (wie bisher)", addr, got, want)
		}
	}
	for addr, want := range map[string]bool{"0xa": true, "0xb": false, "0xs": true, "0xx": false, "0xc": false} {
		if got := dag.erzeugerErlaubt(addr, ab); got != want {
			t.Fatalf("ab dem Stichtag, geschlossen, %s: %v, erwartet %v", addr, got, want)
		}
	}
	// Offen (keine Liste): das Register allein.
	dag.produzentenFest = nil
	for addr, want := range map[string]bool{"0xa": true, "0xb": false, "0xc": true, "0xx": true, "0xy": false} {
		if got := dag.erzeugerErlaubt(addr, ab); got != want {
			t.Fatalf("ab dem Stichtag, offen, %s: %v, erwartet %v", addr, got, want)
		}
	}
	fehlerStand := registerStand()
	fehlerStand.fehler = errors.New("db weg")
	for name, d := range map[string]*BlockDAG{
		"kein Stand":   erzeugerTestDAG(nil),
		"Lesefehler":   erzeugerTestDAG(fehlerStand),
		"ohne Zustand": func() *BlockDAG { d := erzeugerTestDAG(registerStand()); d.state = nil; return d }(),
	} {
		for _, addr := range []string{"0xa", "0xs"} {
			if d.erzeugerErlaubt(addr, ab) {
				t.Fatalf("%s: %s erlaubt -- muss abschliessen", name, addr)
			}
			if !d.erzeugerErlaubt(addr, vorher) && d.state != nil {
				t.Fatalf("%s: vor dem Stichtag %s abgewiesen", name, addr)
			}
		}
	}
}

// Missbrauch (LOW 2, #303): ein Erzeuger ohne Bindung datiert seinen Block
// um die erlaubten zwei Minuten zurueck, um knapp nach dem Stichtag noch
// unter der alten Regel durchzukommen. Die Rueck-Toleranz zaehlt zum
// Stichtag dazu.
func TestErzeugerErlaubt_RueckdatierenUmgehtDenStichtagNicht(t *testing.T) {
	ab := int64(2_000_000_000)
	erzeugerSchnittOverride.Store(ab)
	t.Cleanup(func() { erzeugerSchnittOverride.Store(0) })
	dag := erzeugerTestDAG(registerStand())
	if dag.erzeugerErlaubt("0xb", ab-zeitstempelRueckToleranz) {
		t.Fatal("um die Rueck-Toleranz zurueckdatierter Block ohne Bindung angenommen")
	}
	if !dag.erzeugerErlaubt("0xb", ab-zeitstempelRueckToleranz-1) {
		t.Fatal("eine Sekunde frueher: alte Regel erwartet")
	}
}

// Der Ablauf aus HIGH 1 (#303): V wechselt von K zu K2. Ein Knoten, der die
// Bindung noch nicht nachgespielt hat, und einer, der es hat, urteilen ueber
// jeden Block bis zum Ende der Frist gleich -- der alte Schluessel erzeugt
// nahtlos weiter, der neue beginnt erst danach.
func TestErzeugerErlaubt_WechselOhneSpaltung(t *testing.T) {
	erzeugerSchnittOverride.Store(1)
	t.Cleanup(func() { erzeugerSchnittOverride.Store(0) })
	T := int64(1_900_000_000)
	alt := standAus(z("v", "0xk", T-30*86400))
	neu := standAus(z("v", "0xk", T-30*86400), z("v", "0xk2", T))
	knoten := func(st *erzeugerStand) *BlockDAG {
		d := erzeugerTestDAG(st)
		d.produzentenFest = map[string]bool{"0xk": true, "0xk2": true}
		return d
	}
	a, b := knoten(alt), knoten(neu)
	for t0 := T - 600; t0 < T+erzeugerFrist; t0 += 60 {
		for _, k := range []string{"0xk", "0xk2"} {
			if a.erzeugerErlaubt(k, t0) != b.erzeugerErlaubt(k, t0) {
				t.Fatalf("%s zur Zeit T%+d: die Knoten urteilen verschieden", k, t0-T)
			}
		}
		if !b.erzeugerErlaubt("0xk", t0) || b.erzeugerErlaubt("0xk2", t0) {
			t.Fatalf("zur Zeit T%+d: der alte Schluessel muss noch, der neue darf noch nicht", t0-T)
		}
	}
	if b.erzeugerErlaubt("0xk", T+erzeugerFrist) || !b.erzeugerErlaubt("0xk2", T+erzeugerFrist) {
		t.Fatal("nach der Frist: Uebergabe nicht vollzogen")
	}
}

// Fuer /api/status: welche festen Erzeuger jetzt keine Bindung haben. Offen
// (LOW 5, #303) und ohne lesbaren Stand: null, nie "alles gebunden".
func TestErzeugerOhneBindung(t *testing.T) {
	if got := erzeugerTestDAG(standAus(z("ma", "0xa", 0))).ErzeugerOhneBindung(); !reflect.DeepEqual(got, []string{"0xb"}) {
		t.Fatalf("ohne Bindung: %v", got)
	}
	if got := erzeugerTestDAG(standAus(z("ma", "0xa", 0), z("mb", "0xb", 0))).ErzeugerOhneBindung(); got == nil || len(got) != 0 {
		t.Fatalf("alle gebunden: %v (erwartet [])", got)
	}
	// Gerade erst gebunden: vor Ablauf der Frist noch ohne.
	if got := erzeugerTestDAG(standAus(z("ma", "0xa", 0), z("mb", "0xb", nowUnix()))).ErzeugerOhneBindung(); !reflect.DeepEqual(got, []string{"0xb"}) {
		t.Fatalf("in der Frist: %v", got)
	}
	if got := erzeugerTestDAG(nil).ErzeugerOhneBindung(); got != nil {
		t.Fatalf("ohne Stand: %v (erwartet null)", got)
	}
	fehler := standAus(z("ma", "0xa", 0), z("mb", "0xb", 0))
	fehler.fehler = errors.New("db weg")
	if got := erzeugerTestDAG(fehler).ErzeugerOhneBindung(); got != nil {
		t.Fatalf("Lesefehler: %v (erwartet null)", got)
	}
	offen := erzeugerTestDAG(registerStand())
	offen.produzentenFest = nil
	if got := offen.ErzeugerOhneBindung(); got != nil {
		t.Fatalf("offen: %v (erwartet null)", got)
	}
}

// Missbrauch: ab dem Stichtag nimmt der Abgleich unter Peers niemanden mehr
// auf -- auch nicht mit gueltiger, zeitloser Bindung ("authorize validator
// <adresse>"), die jeder wieder einspielen kann, der sie einmal gesehen hat.
// Geschlossen ab registerLeserAb; offen erst ab erzeugerSchnittAb (LOW 6,
// #303) -- vorher naehme sonst nur der Knoten, bei dem sich ein neuer
// Erzeuger eingetragen hat, dessen Bloecke an.
func TestSyncValidatoren_AbStichtagNichtsVonPeers(t *testing.T) {
	echt := httpSyncClient
	httpSyncClient = &http.Client{}
	t.Cleanup(func() { httpSyncClient = echt })
	t.Cleanup(func() { registerLeserOverride.Store(0); erzeugerSchnittOverride.Store(0) })
	mk, mensch := neuerSchluessel(t)
	_, signing := neuerSchluessel(t)
	sig := personalSign(t, mk, "Aequitas: authorize validator "+signing)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"validators": []map[string]string{
			{"signing_address": signing, "human_wallet": mensch, "operator_binding_signature": sig},
		}})
	}))
	defer srv.Close()
	lauf := func(geschlossen bool) bool {
		cs := newTestState()
		cs.accounts.Set(mensch, &AccountState{Address: mensch, IsHuman: true})
		dag := newOrphanTestDAG()
		dag.state = cs
		if geschlossen {
			dag.produzentenFest = map[string]bool{signing: true}
		}
		dag.syncValidatorsFromPeer(srv.URL)
		return dag.authorizedValidators[signing]
	}
	if !lauf(false) || !lauf(true) {
		t.Fatal("vor dem Stichtag nicht aufgenommen -- der Test beweist nichts")
	}
	registerLeserOverride.Store(1)
	if lauf(true) {
		t.Fatal("geschlossen, ab registerLeserAb vom Peer aufgenommen")
	}
	if !lauf(false) {
		t.Fatal("offen, zwischen den Stichtagen nicht mehr aufgenommen -- das Netz zerfiele")
	}
	erzeugerSchnittOverride.Store(1)
	if lauf(false) {
		t.Fatal("offen, ab erzeugerSchnittAb vom Peer aufgenommen")
	}
}

// Die Leitung fragt ab registerLeserAb den Stand im Speicher, nicht mehr,
// was der Abgleich gemerkt hat, und nie die Datenbank (MEDIUM 2, #303). Ein
// Lesefehler nimmt ihr den Menschen nicht weg -- sonst naehme sie einen
// zweiten Schluessel desselben Menschen auf.
func TestValidatorMenschVon_AbStichtagAusDemStand(t *testing.T) {
	dag := newOrphanTestDAG()
	dag.state = newTestState()
	dag.merkeValidatorMensch("0xaa", "0xmensch")
	if got := dag.validatorMenschVon("0xAA"); got != "0xmensch" {
		t.Fatalf("vor dem Stichtag: %q", got)
	}
	registerLeserOverride.Store(1)
	t.Cleanup(func() { registerLeserOverride.Store(0) })
	if got := dag.validatorMenschVon("0xaa"); got != "" {
		t.Fatalf("ab registerLeserAb ohne Stand: %q", got)
	}
	st := standAus(z("0xm1", "0xaa", 0), z("0xm2", "0xbb", nowUnix()), z("0xm3", "0xcc", 0), z("0xm4", "0xcc", 0),
		z("0xm5", "0xdd", 100), z("0xm6", "0xdd", 200)) // dd: von m5 an m6 uebergeben
	dag.state.erzeugerRegister.Store(st)
	if got := dag.validatorMenschVon("0xAA"); got != "0xm1" {
		t.Fatalf("aus dem Stand: %q", got)
	}
	if got := dag.validatorMenschVon("0xbb"); got != "0xm2" {
		t.Fatalf("gerade gebunden: %q -- der Mensch steht fest, aufgenommen wird nach der Zulassung", got)
	}
	if got := dag.validatorMenschVon("0xcc"); got != "" {
		t.Fatalf("umstritten: %q", got)
	}
	if got := dag.validatorMenschVon("0xdd"); got != "0xm6" {
		t.Fatalf("nach der Uebergabe: %q, erwartet der spaetere 0xm6", got)
	}
	dag.state.erzeugerRegister.Store(&erzeugerStand{fenster: st.fenster, halter: st.halter, fehler: errors.New("db weg")})
	if got := dag.validatorMenschVon("0xaa"); got != "0xm1" {
		t.Fatalf("nach einem Lesefehler: %q -- der letzte Stand muss bleiben", got)
	}
	dag.state.erzeugerRegister.Store(&erzeugerStand{fehler: errors.New("db weg")})
	if got := dag.validatorMenschVon("0xaa"); got != "" {
		t.Fatalf("nie gelesen: %q", got)
	}
}

// Ein Lesefehler im Speicherstand behaelt die Fenster fuer die Leitung, die
// Erzeugerpruefung schliesst trotzdem ab.
func TestErzeugerRegisterAuffrischen_FehlerBehaeltFenster(t *testing.T) {
	cs := newTestState()
	cs.erzeugerRegister.Store(registerStand())
	cs.db, _ = sql.Open("postgres", "postgres://nobody@127.0.0.1:1/x?sslmode=disable&connect_timeout=1")
	t.Cleanup(func() { cs.db.Close() })
	cs.erzeugerRegisterAuffrischen()
	st := cs.erzeugerRegister.Load()
	if st == nil || st.fehler == nil {
		t.Fatalf("Lesefehler nicht im Stand: %+v", st)
	}
	if st.erzeugerFenster("0xa", nowUnix()) != "ma" {
		t.Fatal("Fenster des letzten Stands verloren")
	}
	dag := erzeugerTestDAG(nil)
	dag.state = cs
	erzeugerSchnittOverride.Store(1)
	t.Cleanup(func() { erzeugerSchnittOverride.Store(0) })
	if dag.erzeugerErlaubt("0xa", nowUnix()) {
		t.Fatal("Erzeugerpruefung mit Lesefehler offen")
	}
}

// Missbrauch ueber den echten Eingang (AddPeerBlock): ab erzeugerSchnittAb
// weist der Knoten den Block eines Schluessels ab, den nur der Abgleich unter
// Peers kennt (authorizedValidators), nicht das Register -- auch wenn er
// gueltig unterschrieben ist. Vor dem Stichtag kommt derselbe Block durch die
// Erzeugerpruefung.
func TestAddPeerBlock_ErzeugerpruefungAusDemRegister(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	proposer := strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())
	lauf := func(st *erzeugerStand) (abgewiesenAlsUnbekannt bool) {
		dag := newUnknownProposerTestDAG()
		dag.authorizedValidators[proposer] = true // vom Abgleich gemeldet
		cs := &ChainState{}
		cs.erzeugerRegister.Store(st)
		dag.state = cs
		// Eltern fehlen: wer die Erzeugerpruefung besteht, wird nur Waise
		// (kein Nachspielen gegen den Test-Zustand).
		b := signTestBlockFromKey(t, key, 2, "fehlt-"+t.Name())
		dag.AddPeerBlock(b)
		dag.mu.RLock()
		defer dag.mu.RUnlock()
		_, versucht := dag.unknownProposerLastRecovery[proposer]
		return versucht
	}
	leer := standAus()
	if lauf(leer) {
		t.Fatal("vor dem Stichtag als unbekannt abgewiesen -- der Test beweist nichts")
	}
	erzeugerSchnittOverride.Store(1)
	t.Cleanup(func() { erzeugerSchnittOverride.Store(0) })
	if !lauf(leer) {
		t.Fatal("ab erzeugerSchnittAb: Schluessel ohne Bindung nicht abgewiesen")
	}
	if !lauf(standAus(z("0xm", proposer, nowUnix()))) {
		t.Fatal("ab erzeugerSchnittAb: Bindung in der Frist schon angenommen")
	}
	if lauf(standAus(z("0xm", proposer, 0))) {
		t.Fatal("ab erzeugerSchnittAb: gebundener Schluessel als unbekannt abgewiesen")
	}
}

// fehlerSQL: jede Abfrage scheitert (Verbindung weg).
type fehlerSQL struct{}

func (fehlerSQL) Exec(string, ...interface{}) (sql.Result, error) {
	return nil, errors.New("verbindung weg")
}
func (fehlerSQL) QueryRow(string, ...interface{}) *sql.Row { return nil }
func (fehlerSQL) Query(string, ...interface{}) (*sql.Rows, error) {
	return nil, errors.New("verbindung weg")
}

// Fail-closed: ist der Verlauf nicht lesbar, liefert das Strafkonto einen
// Fehler -- beim Nachspielen weist er den Block ab. Nie "keine Strafe".
func TestStrafkonto_LesefehlerIstFehler(t *testing.T) {
	if w, err := strafKontoZurAbrechnung(fehlerSQL{}, "0xsigner", 2000); err == nil {
		t.Fatalf("Lesefehler ergab Strafkonto %q ohne Fehler", w)
	}
}

// Geschlossen liest der Stand die Liste und den eigenen Schluessel, offen
// alles.
func TestErzeugerFestListe(t *testing.T) {
	dag := erzeugerTestDAG(nil)
	if got := dag.erzeugerFestListe(); !reflect.DeepEqual(got, []string{"0xa", "0xb", "0xs"}) {
		t.Fatalf("geschlossen: %v", got)
	}
	dag.produzentenFest = nil
	if got := dag.erzeugerFestListe(); got != nil {
		t.Fatalf("offen: %v", got)
	}
}

// Auch die Liste des Seeds (registerAndDiscover) ist ein Abgleich: ab dem
// Stichtag nimmt der Knoten daraus keinen Erzeuger mehr auf (LOW 6, #303) --
// sie ist nicht unterschrieben.
func TestRegisterAndDiscover_SeedListeAbStichtagNicht(t *testing.T) {
	echt := httpSyncClient
	httpSyncClient = &http.Client{}
	t.Cleanup(func() { httpSyncClient = echt })
	neu := "0x" + strings.Repeat("ab", 20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/peers/register" {
			json.NewEncoder(w).Encode(map[string]interface{}{"peers": []string{}, "validators": []string{neu}})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	lauf := func() bool {
		dag := newGhostdagTestDAG()
		if !dag.registerAndDiscover("http://self.invalid:8080", srv.URL) {
			t.Fatal("Anmeldung beim Seed gescheitert")
		}
		dag.mu.RLock()
		defer dag.mu.RUnlock()
		return dag.authorizedValidators[neu]
	}
	if !lauf() {
		t.Fatal("vor dem Stichtag nicht aufgenommen -- der Test beweist nichts")
	}
	erzeugerSchnittOverride.Store(1) // offen: newGhostdagTestDAG hat keine Liste
	t.Cleanup(func() { erzeugerSchnittOverride.Store(0) })
	if lauf() {
		t.Fatal("ab dem Stichtag aus der Liste des Seeds aufgenommen")
	}
}

// Missbrauch (L2, #303): nach einem Wechsel von K0 zu K1 gehoert K0 weiter
// demselben Menschen -- die Leitung nimmt K1 nicht als zweiten Sitz auf,
// solange K0 drin ist. Vorher fiel K0 nach der Frist auf "" und zaehlte
// nicht mehr gegen K1.
func TestLeitung_WechselGibtKeinenZweitenSitz(t *testing.T) {
	registerLeserOverride.Store(1)
	t.Cleanup(func() { registerLeserOverride.Store(0) })
	dag := newOrphanTestDAG()
	dag.state = newTestState()
	dag.state.erzeugerRegister.Store(standAus(z("0xm", "0xk0", 0), z("0xm", "0xk1", nowUnix()-3*erzeugerFrist)))
	if a, b := dag.validatorMenschVon("0xk0"), dag.validatorMenschVon("0xk1"); a != "0xm" || b != "0xm" {
		t.Fatalf("Mensch zu K0 %q, zu K1 %q -- beide muessen 0xm sein", a, b)
	}
	l := &Leitung{env: LeitUmgebung{Mensch: dag.validatorMenschVon}}
	if l.aufnehmbar("0xk1", []string{"0xk0"}) {
		t.Fatal("K1 neben K0 aufgenommen -- derselbe Mensch zweimal")
	}
	if !l.aufnehmbar("0xk1", nil) {
		t.Fatal("K1 allein nicht aufnehmbar")
	}
}
