package keeper

import (
	"database/sql"
	"encoding/json"
	"errors"
	"math"
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

// erzeugerTestDAG: geschlossene Liste {A, B}, eigener Schluessel S, dazu X,
// den ein Peer im Abgleich gemeldet hat (authorizedValidators).
func erzeugerTestDAG(register map[string]bool, fehler error) *BlockDAG {
	cs := newTestState()
	if register != nil || fehler != nil {
		cs.erzeugerRegister.Store(&erzeugerStand{adressen: register, fehler: fehler, zeit: time.Now()})
	}
	dag := newOrphanTestDAG()
	dag.state = cs
	dag.produzentenFest = map[string]bool{"0xa": true, "0xb": true}
	dag.selfProposer = "0xs"
	dag.authorizedValidators = map[string]bool{"0xa": true, "0xb": true, "0xs": true, "0xx": true}
	return dag
}

// Die Erzeugerpruefung: vor dem Stichtag wie bisher; danach die
// Schnittmenge -- das Register nimmt weg (B ohne Bindung), fuegt aber nichts
// hinzu (C steht nur im Register), und was ein Peer im Abgleich meldete (X),
// zaehlt nicht mehr. Kein Stand oder ein Lesefehler: niemand (fail-closed).
func TestErzeugerErlaubt_SchnittmengeUndFailClosed(t *testing.T) {
	ab := int64(2_000_000_000)
	erzeugerSchnittOverride.Store(ab)
	t.Cleanup(func() { erzeugerSchnittOverride.Store(0) })
	register := map[string]bool{"0xa": true, "0xc": true, "0xs": true, "0xx": true}

	dag := erzeugerTestDAG(register, nil)
	for addr, want := range map[string]bool{"0xa": true, "0xb": true, "0xs": true, "0xx": true, "0xc": false} {
		if got := dag.erzeugerErlaubt(addr, ab-1); got != want {
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
	for name, d := range map[string]*BlockDAG{
		"kein Stand":   erzeugerTestDAG(nil, nil),
		"Lesefehler":   erzeugerTestDAG(register, errors.New("db weg")),
		"ohne Zustand": func() *BlockDAG { d := erzeugerTestDAG(register, nil); d.state = nil; return d }(),
	} {
		for _, addr := range []string{"0xa", "0xs"} {
			if d.erzeugerErlaubt(addr, ab) {
				t.Fatalf("%s: %s erlaubt -- muss abschliessen", name, addr)
			}
			if !d.erzeugerErlaubt(addr, ab-1) && d.state != nil {
				t.Fatalf("%s: vor dem Stichtag %s abgewiesen", name, addr)
			}
		}
	}
}

// Fuer /api/status: welche festen Erzeuger noch keine Bindung haben.
func TestErzeugerOhneBindung(t *testing.T) {
	if got := erzeugerTestDAG(map[string]bool{"0xa": true}, nil).ErzeugerOhneBindung(); !reflect.DeepEqual(got, []string{"0xb"}) {
		t.Fatalf("ohne Bindung: %v", got)
	}
	if got := erzeugerTestDAG(map[string]bool{"0xa": true, "0xb": true}, nil).ErzeugerOhneBindung(); got == nil || len(got) != 0 {
		t.Fatalf("alle gebunden: %v (erwartet [])", got)
	}
	if got := erzeugerTestDAG(nil, nil).ErzeugerOhneBindung(); got != nil {
		t.Fatalf("ohne Stand: %v (erwartet null)", got)
	}
}

// Missbrauch: ab registerLeserAb nimmt der Abgleich unter Peers niemanden
// mehr auf -- auch nicht mit gueltiger, zeitloser Bindung
// ("authorize validator <adresse>"), die jeder wieder einspielen kann, der
// sie einmal gesehen hat. Vorher wie bisher.
func TestSyncValidatoren_AbStichtagNichtsVonPeers(t *testing.T) {
	echt := httpSyncClient
	httpSyncClient = &http.Client{}
	t.Cleanup(func() { httpSyncClient = echt })
	mk, mensch := neuerSchluessel(t)
	_, signing := neuerSchluessel(t)
	sig := personalSign(t, mk, "Aequitas: authorize validator "+signing)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"validators": []map[string]string{
			{"signing_address": signing, "human_wallet": mensch, "operator_binding_signature": sig},
		}})
	}))
	defer srv.Close()
	lauf := func() bool {
		cs := newTestState()
		cs.accounts.Set(mensch, &AccountState{Address: mensch, IsHuman: true})
		dag := newOrphanTestDAG()
		dag.state = cs
		dag.syncValidatorsFromPeer(srv.URL)
		return dag.authorizedValidators[signing]
	}
	if !lauf() {
		t.Fatal("vor dem Stichtag nicht aufgenommen -- der Test beweist nichts")
	}
	registerLeserOverride.Store(1)
	t.Cleanup(func() { registerLeserOverride.Store(0) })
	if lauf() {
		t.Fatal("ab registerLeserAb vom Peer aufgenommen")
	}
}

// Die Leitung fragt ab registerLeserAb das Register, nicht mehr, was der
// Abgleich gemerkt hat (validatorMenschen).
func TestValidatorMenschVon_AbStichtagNichtAusDemAbgleich(t *testing.T) {
	dag := newOrphanTestDAG()
	dag.state = newTestState() // ohne Datenbank: das Register kennt niemanden
	dag.merkeValidatorMensch("0xaa", "0xmensch")
	if got := dag.validatorMenschVon("0xAA"); got != "0xmensch" {
		t.Fatalf("vor dem Stichtag: %q", got)
	}
	registerLeserOverride.Store(1)
	t.Cleanup(func() { registerLeserOverride.Store(0) })
	if got := dag.validatorMenschVon("0xaa"); got != "" {
		t.Fatalf("ab registerLeserAb aus dem Abgleich: %q", got)
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
	lauf := func(register map[string]bool) (abgewiesenAlsUnbekannt bool) {
		dag := newUnknownProposerTestDAG()
		dag.authorizedValidators[proposer] = true // vom Abgleich gemeldet
		cs := &ChainState{}
		cs.erzeugerRegister.Store(&erzeugerStand{adressen: register, zeit: time.Now()})
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
	if lauf(map[string]bool{}) {
		t.Fatal("vor dem Stichtag als unbekannt abgewiesen -- der Test beweist nichts")
	}
	erzeugerSchnittOverride.Store(1)
	t.Cleanup(func() { erzeugerSchnittOverride.Store(0) })
	if !lauf(map[string]bool{}) {
		t.Fatal("ab erzeugerSchnittAb: Schluessel ohne Bindung nicht abgewiesen")
	}
	if lauf(map[string]bool{proposer: true}) {
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

// Fail-closed: ist das Register nicht lesbar, liefert das Strafkonto einen
// Fehler -- beim Nachspielen weist er den Block ab. Nie "keine Strafe".
func TestStrafkonto_LesefehlerIstFehler(t *testing.T) {
	registerLeserOverride.Store(1)
	t.Cleanup(func() { registerLeserOverride.Store(0) })
	if w, err := strafKonto(fehlerSQL{}, "0xsigner", 100); err == nil {
		t.Fatalf("Lesefehler ergab Strafkonto %q ohne Fehler", w)
	}
}
