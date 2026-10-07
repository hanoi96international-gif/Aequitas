package keeper

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

// Validator-Register, Schritt 3, Teil 2, mit echter Datenbank: Anker, Kegel,
// Gewichte, und die Runde beim Erzeuger und beim Nachspielenden
// (validator_belohnung_kette.go, nachrechnen_validator.go).

// kegelEintrag: ein nachgespielter Block direkt in chain_blocks.
func kegelEintrag(t *testing.T, cs *ChainState, hash string, eltern []string, proposer string, zeit, blueScore int64) {
	t.Helper()
	ph, _ := json.Marshal(eltern)
	if _, err := cs.db.Exec(`INSERT INTO chain_blocks (hash, height, parent_hashes, proposer, timestamp, blue_score, replayed)
		VALUES ($1, 0, $2, $3, $4, $5, true)`, hash, string(ph), proposer, zeit, blueScore); err != nil {
		t.Fatal(err)
	}
}

func kegelLeeren(t *testing.T, cs *ChainState) {
	t.Helper()
	// Wie beim Vorab-Speichern jedes Blocks (AddPeerBlock): die Spalten von
	// chain_blocks stehen, bevor ein Nachspielen sie liest -- sonst wartete
	// ein ALTER auf einer anderen Verbindung auf die Lesesperre des
	// Nachspielens.
	cs.ensureGHOSTDAGColumns()
	cs.ensureReplayedColumn()
	cs.ensureTxRootColumn()
	if _, err := cs.db.Exec(`DELETE FROM chain_blocks`); err != nil {
		t.Fatal(err)
	}
}

// Der Kegel: nur, was vom Anker ueber Eltern erreichbar ist, mit Zeit in
// [seit, bis); abgestiegen wird nur bis zum Puffer unter seit. Ein fehlender
// Block im Kegel ist eine Luecke, einer unter dem Puffer nicht.
func TestBloeckeImKegel_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	kegelLeeren(t, f.cs)
	T := (f.jetzt / 60) * 60
	h := func(s string) string { return t.Name() + "-" + s }
	kegelEintrag(t, f.cs, h("g"), nil, "0xg", T-30*3600, 1)
	kegelEintrag(t, f.cs, h("a1"), []string{h("g")}, "0xa1", T-25*3600, 2)
	kegelEintrag(t, f.cs, h("a2"), []string{h("a1")}, "0xk1", T-20*3600, 3)
	kegelEintrag(t, f.cs, h("a3"), []string{h("a2")}, "0xk2", T-10*3600, 4)
	kegelEintrag(t, f.cs, h("s"), []string{h("a2")}, "0xk3", T-5*3600, 4)
	kegelEintrag(t, f.cs, h("anker"), []string{h("a3"), h("s")}, "0xk1", T-60, 5)
	kegelEintrag(t, f.cs, h("fremd"), []string{h("a3")}, "0xk4", T-3600, 4) // nicht im Kegel
	bloecke, stand, err := bloeckeImKegel(f.cs.db, h("anker"), T-anwesenheitsZeitraum, T)
	if err != nil || stand != kegelVollstaendig {
		t.Fatalf("Kegel: %v %v", stand, err)
	}
	got := map[string]int{}
	for _, b := range bloecke {
		got[b.proposer]++
	}
	want := map[string]int{"0xk1": 2, "0xk2": 1, "0xk3": 1}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("Bloecke im Kegel: %v, erwartet %v", got, want)
	}
	// Unter dem Puffer fehlt Geschichte: keine Luecke.
	f.cs.db.Exec(`DELETE FROM chain_blocks WHERE hash = $1`, h("g"))
	if _, stand, _ := bloeckeImKegel(f.cs.db, h("anker"), T-anwesenheitsZeitraum, T); stand != kegelVollstaendig {
		t.Fatalf("fehlender Block unter dem Puffer: %v", stand)
	}
	// Im Kegel fehlt ein Block: Luecke.
	f.cs.db.Exec(`DELETE FROM chain_blocks WHERE hash = $1`, h("a3"))
	if _, stand, _ := bloeckeImKegel(f.cs.db, h("anker"), T-anwesenheitsZeitraum, T); stand != kegelLuecke {
		t.Fatalf("fehlender Block im Kegel: %v", stand)
	}
	// Grenze: mehr Bloecke als erlaubt.
	alt := anwesenheitKegelGrenze
	anwesenheitKegelGrenze = 2
	t.Cleanup(func() { anwesenheitKegelGrenze = alt })
	if _, stand, _ := bloeckeImKegel(f.cs.db, h("anker"), T-anwesenheitsZeitraum, T); stand != kegelZuGross {
		t.Fatalf("Kegel ueber der Grenze: %v", stand)
	}
}

// gewichteFall: V (Mensch) mit K1, ab T-12h mit K4; W (Mensch) mit K2; X
// (kein Mensch) mit K3; K9 ungebunden. Bloecke im Kegel des Ankers.
type gewichteFall struct {
	f       *registerFall
	T       int64
	v, w, x string
	anker   string
}

func neuerGewichteFall(t *testing.T) *gewichteFall {
	t.Helper()
	f := neuerRegisterFall(t)
	kegelLeeren(t, f.cs)
	for _, q := range []string{`DELETE FROM validator_register`, `DELETE FROM validator_verlauf`} {
		if _, err := f.cs.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	g := &gewichteFall{f: f, T: (f.jetzt / 60) * 60, v: distTestAddr(1901), w: distTestAddr(1902), x: distTestAddr(1903)}
	registerKonto(t, f.cs, g.v, true)
	registerKonto(t, f.cs, g.w, true)
	registerKonto(t, f.cs, g.x, false)
	T := g.T
	verlaufEintrag(t, f.cs, g.v, "0xk1", T-30*86400)
	verlaufEintrag(t, f.cs, g.v, "0xk4", T-12*3600)
	verlaufEintrag(t, f.cs, g.w, "0xk2", T-30*86400)
	verlaufEintrag(t, f.cs, g.x, "0xk3", T-30*86400)
	h := func(s string) string { return t.Name() + "-" + s }
	kette := []struct {
		name     string
		proposer string
		zeit     int64
	}{
		{"b1", "0xk1", T - 20*3600},                    // V
		{"b2", "0xk1", T - 20*3600 + 10},               // V, dieselbe Minute
		{"b3", "0xk2", T - 15*3600},                    // W
		{"b4", "0xk3", T - 14*3600},                    // X: kein Mensch
		{"b5", "0xk9", T - 13*3600},                    // ungebunden
		{"b6", "0xk1", T - 11*3600},                    // V: K1 in der Frist nach dem Wechsel
		{"b7", "0xk1", T - 9*3600},                     // nach der Frist: niemandem
		{"b8", "0xk4", T - 9*3600 + 60},                // V mit K4
		{"b0", "0xk1", T - anwesenheitsZeitraum - 120}, // vor dem Tag
	}
	vorher := h("wurzel")
	kegelEintrag(t, f.cs, vorher, nil, "0xk1", T-anwesenheitsZeitraum-3600, 1)
	// b0 haengt an der Wurzel, die anderen in Zeitfolge.
	reihe := []int{8, 0, 1, 2, 3, 4, 5, 6, 7}
	for i, j := range reihe {
		b := kette[j]
		kegelEintrag(t, f.cs, h(b.name), []string{vorher}, b.proposer, b.zeit, int64(i+2))
		vorher = h(b.name)
	}
	g.anker = h("anker")
	kegelEintrag(t, f.cs, g.anker, []string{vorher}, "0xk4", T-30, 1_000_000)
	return g
}

// Die Gewichte: Minuten mit mindestens einem Block, dem Betreiber
// zugerechnet, dessen Erzeugerfenster den Block zuliess -- mit derselben
// Frist wie die Erzeugerpruefung. Kein Mensch und ungebunden zaehlen nicht.
func TestValidatorGewichte_RealDB(t *testing.T) {
	g := neuerGewichteFall(t)
	gewichte, stand, err := validatorGewichte(g.f.cs.db, g.anker, g.T)
	if err != nil || stand != kegelVollstaendig {
		t.Fatalf("%v %v", stand, err)
	}
	// V: b1/b2 (eine Minute), b6, b8 und der Anker (K4) = 4; W: b3 = 1.
	want := map[string]int64{g.v: 4, g.w: 1}
	if fmt.Sprint(gewichte) != fmt.Sprint(want) {
		t.Fatalf("Gewichte %v, erwartet %v", gewichte, want)
	}
	if a, err := validatorAnkerWaehlen(g.f.cs.db, g.T); err != nil || a != g.anker {
		t.Fatalf("Anker %q (%v), erwartet %q", a, err, g.anker)
	}
}

// validatorRegeln: die Zaehler der Validatoren-Runde.
var validatorRegeln = []string{"validator_kein_mensch", "validator_ohne_anker", "validator_anker_zu_frueh", "validator_zeit",
	"validator_anker_unbekannt", "validator_anker_zeit", "validator_anker_wechsel", "validator_doppelt",
	"validator_nicht_anwesend", "validator_anteil", "validator_empfaenger"}

func validatorZaehler() map[string]int64 {
	m := map[string]int64{}
	for _, r := range validatorRegeln {
		m[r] = erhaltungZaehler(r)
	}
	return m
}

func validatorNeu(vorher map[string]int64) map[string]int64 {
	m := map[string]int64{}
	for _, r := range validatorRegeln {
		if d := erhaltungZaehler(r) - vorher[r]; d != 0 {
			m[r] = d
		}
	}
	return m
}

// topfSetzen: der Validatoren-Topf und die Konten von V und W wie vor der
// Runde (100 AEQ Topf, je 100 AEQ).
func (g *gewichteFall) topfSetzen(t *testing.T) {
	t.Helper()
	registerKonto(t, g.f.cs, g.v, true)
	registerKonto(t, g.f.cs, g.w, true)
	g.f.cs.mu.Lock()
	acc := &AccountState{Address: validatorsPoolAddr, Balance: NewDecimal(100)}
	g.f.cs.accounts.Set(validatorsPoolAddr, acc)
	err := g.f.cs.saveAccountToDB(acc)
	g.f.cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	g.f.cs.validatorRunde = validatorRundePruefung{}
}

// rundeNachspielen: die Zahlungen in einem Block nachspielen; neue
// Abweichungen der Validatoren-Runde zurueck.
func (g *gewichteFall) rundeNachspielen(t *testing.T, txs []Transaction) (bool, map[string]int64) {
	t.Helper()
	g.topfSetzen(t)
	vorher := validatorZaehler()
	ok := g.f.block(g.T+5, txs...)
	return ok, validatorNeu(vorher)
}

// Gutfall und Missbrauch: der Erzeuger zahlt ab dem Stichtag nach den
// Gewichten aus der Kette, mit Anker und Zeit der Runde; jeder
// Nachspielende rechnet nach. Jede Verfaelschung -- Betrag, Empfaenger,
// Anker, fehlender oder doppelter Empfaenger, die alte Form -- wird gemeldet.
func TestValidatorRunde_ErzeugerUndNachspielen_RealDB(t *testing.T) {
	g := neuerGewichteFall(t)
	registerLeserOverride.Store(1)
	t.Cleanup(func() { registerLeserOverride.Store(0) })
	g.topfSetzen(t)
	g.f.cs.mu.Lock()
	shares, err := g.f.cs.distributeValidatorsPoolLocked(context.Background(), g.T)
	g.f.cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != 2 {
		t.Fatalf("Anteile: %+v", shares)
	}
	var echt []Transaction
	for _, s := range shares {
		if s.Anker != g.anker || s.RundenZeit != g.T {
			t.Fatalf("Anteil ohne Anker/Zeit: %+v", s)
		}
		echt = append(echt, Transaction{Type: "validator_distribution", Wallet: s.Wallet, Amount: s.Amount, Anker: s.Anker, DistributionAt: s.RundenZeit})
	}
	soll := map[string]float64{g.v: 80, g.w: 20}
	for _, s := range shares {
		if s.Amount != soll[s.Wallet] {
			t.Fatalf("%s bekam %.6f, erwartet %.6f", s.Wallet, s.Amount, soll[s.Wallet])
		}
	}
	abschluss := Transaction{Type: "validator_distribution_pool_zero", Amount: 0}
	runde := func(txs ...Transaction) []Transaction { return append(append([]Transaction(nil), txs...), abschluss) }

	if ok, neu := g.rundeNachspielen(t, runde(echt...)); !ok || len(neu) != 0 {
		t.Fatalf("ehrliche Runde: angenommen %v, Abweichungen %v", ok, neu)
	}
	mit := func(i int, aendern func(*Transaction)) []Transaction {
		k := append([]Transaction(nil), echt...)
		aendern(&k[i])
		return runde(k...)
	}
	iv := 0
	if echt[0].Wallet != g.v {
		iv = 1
	}
	registerKonto(t, g.f.cs, distTestAddr(1904), true) // Mensch, aber ohne Anwesenheit
	for name, fall := range map[string]struct {
		txs   []Transaction
		regel string
	}{
		"Betrag":         {mit(iv, func(tx *Transaction) { tx.Amount = 90 }), "validator_anteil"},
		"kein Mensch":    {mit(iv, func(tx *Transaction) { tx.Wallet = g.x }), "validator_kein_mensch"},
		"nicht anwesend": {runde(echt[0], echt[1], Transaction{Type: "validator_distribution", Wallet: distTestAddr(1904), Amount: 1, Anker: g.anker, DistributionAt: g.T}), "validator_nicht_anwesend"},
		"Anker unbekannt": {runde(func() []Transaction {
			k := append([]Transaction(nil), echt...)
			for i := range k {
				k[i].Anker = "gibtesnicht"
			}
			return k
		}()...), "validator_anker_unbekannt"},
		"Anker zu alt": {runde(func() []Transaction {
			k := append([]Transaction(nil), echt...)
			for i := range k {
				k[i].Anker = t.Name() + "-b8"
			}
			return k
		}()...), "validator_anker_zeit"},
		"Ankerwechsel": {mit(1, func(tx *Transaction) { tx.Anker = t.Name() + "-b8" }), "validator_anker_wechsel"},
		"Zeit neben Block": {runde(func() []Transaction {
			k := append([]Transaction(nil), echt...)
			for i := range k {
				k[i].DistributionAt = g.T - 3600
			}
			return k
		}()...), "validator_zeit"},
		"fehlt einer": {runde(echt[iv]), "validator_empfaenger"},
		"doppelt":     {runde(echt[0], echt[1], echt[0]), "validator_doppelt"},
		"alte Form":   {runde(Transaction{Type: "validator_distribution", Wallet: g.v, Amount: 80}, Transaction{Type: "validator_distribution", Wallet: g.w, Amount: 20}), "validator_ohne_anker"},
	} {
		_, neu := g.rundeNachspielen(t, fall.txs)
		if neu[fall.regel] == 0 {
			t.Errorf("%s: %s nicht gemeldet (%v)", name, fall.regel, neu)
		}
	}
}

// Fehlt dem Erzeuger Geschichte im Kegel, zahlt er in dieser Runde keine
// Validatoren (der Topf bleibt). Fehlt sie dem Nachspielenden, rechnet er
// nicht nach -- er meldet nichts und weist nichts ab.
func TestValidatorRunde_LueckeImKegel_RealDB(t *testing.T) {
	g := neuerGewichteFall(t)
	registerLeserOverride.Store(1)
	t.Cleanup(func() { registerLeserOverride.Store(0) })
	g.topfSetzen(t)
	g.f.cs.mu.Lock()
	echteShares, err := g.f.cs.distributeValidatorsPoolLocked(context.Background(), g.T)
	g.f.cs.mu.Unlock()
	if err != nil || len(echteShares) != 2 {
		t.Fatalf("Vorbedingung: %v %+v", err, echteShares)
	}
	g.f.cs.db.Exec(`DELETE FROM chain_blocks WHERE hash = $1`, t.Name()+"-b3")
	g.topfSetzen(t)
	g.f.cs.mu.Lock()
	shares, err := g.f.cs.distributeValidatorsPoolLocked(context.Background(), g.T)
	g.f.cs.mu.Unlock()
	if err != nil || len(shares) != 0 {
		t.Fatalf("mit Luecke gezahlt: %v %+v", err, shares)
	}
	if got := stand(g.f.cs, validatorsPoolAddr); got != 100 {
		t.Fatalf("Topf %.6f, erwartet 100 (bleibt stehen)", got)
	}
	var txs []Transaction
	for _, s := range echteShares {
		txs = append(txs, Transaction{Type: "validator_distribution", Wallet: s.Wallet, Amount: s.Amount, Anker: s.Anker, DistributionAt: s.RundenZeit})
	}
	txs = append(txs, Transaction{Type: "validator_distribution_pool_zero"})
	if ok, neu := g.rundeNachspielen(t, txs); !ok || len(neu) != 0 {
		t.Fatalf("Nachspielender mit Luecke: angenommen %v, Abweichungen %v", ok, neu)
	}
}

// Vor dem Stichtag: wie bisher -- keine Anker, Gewichte aus
// registered_nodes, nur "ist Mensch" wird nachgerechnet.
func TestValidatorRunde_VorDemStichtagWieBisher_RealDB(t *testing.T) {
	g := neuerGewichteFall(t)
	if _, err := g.f.cs.db.Exec(`INSERT INTO registered_nodes (wallet_address, signing_address) VALUES ($1, '0xk1'), ($2, '0xk2')`, g.v, g.w); err != nil {
		t.Fatal(err)
	}
	g.topfSetzen(t)
	g.f.cs.mu.Lock()
	shares, err := g.f.cs.distributeValidatorsPoolLocked(context.Background(), g.T)
	g.f.cs.mu.Unlock()
	if err != nil || len(shares) == 0 {
		t.Fatalf("%v %+v", err, shares)
	}
	var txs []Transaction
	for _, s := range shares {
		if s.Anker != "" || s.RundenZeit != 0 {
			t.Fatalf("vor dem Stichtag mit Anker: %+v", s)
		}
		txs = append(txs, Transaction{Type: "validator_distribution", Wallet: s.Wallet, Amount: s.Amount})
	}
	if ok, neu := g.rundeNachspielen(t, append(txs, Transaction{Type: "validator_distribution_pool_zero"})); !ok || len(neu) != 0 {
		t.Fatalf("vor dem Stichtag: angenommen %v, Abweichungen %v", ok, neu)
	}
}

// Ein zurueckgewiesener Block nimmt die Runde mit: der ehrliche Block mit
// denselben Zahlungen meldet danach keinen doppelten Empfaenger.
func TestValidatorRunde_ZurueckgewiesenerBlock_RealDB(t *testing.T) {
	g := neuerGewichteFall(t)
	registerLeserOverride.Store(1)
	t.Cleanup(func() { registerLeserOverride.Store(0) })
	g.topfSetzen(t)
	g.f.cs.mu.Lock()
	shares, err := g.f.cs.distributeValidatorsPoolLocked(context.Background(), g.T)
	g.f.cs.mu.Unlock()
	if err != nil || len(shares) != 2 {
		t.Fatalf("%v %+v", err, shares)
	}
	var txs []Transaction
	for _, s := range shares {
		txs = append(txs, Transaction{Type: "validator_distribution", Wallet: s.Wallet, Amount: s.Amount, Anker: s.Anker, DistributionAt: s.RundenZeit})
	}
	g.topfSetzen(t)
	vorher := validatorZaehler()
	if g.f.block(g.T+5, txs[0], gift()) {
		t.Fatal("vergifteter Block angenommen")
	}
	if !g.f.block(g.T+6, append(txs, Transaction{Type: "validator_distribution_pool_zero"})...) {
		t.Fatal("ehrlicher Block abgewiesen")
	}
	if neu := validatorNeu(vorher); len(neu) != 0 {
		t.Fatalf("nach dem Zurueckweisen: %v", neu)
	}
}
