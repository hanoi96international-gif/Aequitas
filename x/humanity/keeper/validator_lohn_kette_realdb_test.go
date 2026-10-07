package keeper

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// Validatoren-Belohnung aus der Kette (validator_lohn_kette.go), mit echter
// Datenbank.
//
// Der Fall: A bindet KA, B bindet KB, N (kein Mensch) bindet KN, KU ist nicht
// gebunden. C uebernimmt KA so, dass sein Fenster nach sechs Stunden des
// Tages beginnt. registered_nodes nennt fuer KA einen ganz anderen
// Menschen R -- er zaehlt nicht mehr.
//
//   - KA: drei Bloecke in jeder Minute des Tages -> A 360 Minuten, C 1080
//     (mehr Bloecke je Minute zaehlen nicht mehr).
//   - KB: ein Block je Minute, den halben Tag -> B 720. Dazu Bloecke vor dem
//     Fenster und im Rand vor der Runde -- sie zaehlen nicht.
//   - KN, KU: den ganzen Tag -- kein Mensch bzw. kein Betreiber.
//
// Topf 90 -> A 15, B 30, C 45.
type lohnFall struct {
	t              *testing.T
	cs             *ChainState
	rundeAt        int64
	a, b, c, n, r  string
	ka, kb, kn, ku string
}

func neuerLohnFall(t *testing.T) *lohnFall {
	t.Helper()
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-lohn-kette-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	f := &lohnFall{t: t, cs: cs, rundeAt: time.Now().Unix(),
		a: distTestAddr(60), b: distTestAddr(61), c: distTestAddr(62), n: distTestAddr(63), r: distTestAddr(64),
		ka: distTestAddr(70), kb: distTestAddr(71), kn: distTestAddr(72), ku: distTestAddr(73)}
	for _, q := range []string{`DELETE FROM chain_blocks WHERE hash LIKE 'lohn-%'`, `DELETE FROM validator_verlauf`, `DELETE FROM registered_nodes`} {
		if _, err := cs.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { cs.db.Exec(`DELETE FROM chain_blocks WHERE hash LIKE 'lohn-%'`) })
	seit, bis := anwesenheitsFenster(f.rundeAt)
	lange := seit - 10*86400
	verlaufEintrag(t, cs, f.a, f.ka, lange)
	verlaufEintrag(t, cs, f.b, f.kb, lange)
	verlaufEintrag(t, cs, f.n, f.kn, lange)
	verlaufEintrag(t, cs, f.c, f.ka, seit+6*3600-erzeugerFrist)
	for _, q := range []struct {
		sql  string
		args []interface{}
	}{
		{`INSERT INTO chain_blocks (hash, height, parent_hashes, proposer, timestamp)
		  SELECT 'lohn-ka-' || g, g, '[]', $1, $2 + (g / 3) * 60 + (g % 3) FROM generate_series(0, 1440*3 - 1) g`, []interface{}{f.ka, seit}},
		{`INSERT INTO chain_blocks (hash, height, parent_hashes, proposer, timestamp)
		  SELECT 'lohn-kb-' || g, g, '[]', $1, $2 + g * 60 FROM generate_series(0, 719) g`, []interface{}{f.kb, seit}},
		{`INSERT INTO chain_blocks (hash, height, parent_hashes, proposer, timestamp)
		  SELECT 'lohn-kb-vor-' || g, g, '[]', $1, $2 - 3600 + g * 60 FROM generate_series(0, 59) g`, []interface{}{f.kb, seit}},
		{`INSERT INTO chain_blocks (hash, height, parent_hashes, proposer, timestamp)
		  SELECT 'lohn-kb-rand-' || g, g, '[]', $1, $2 + g * 60 FROM generate_series(0, 14) g`, []interface{}{f.kb, bis}},
		{`INSERT INTO chain_blocks (hash, height, parent_hashes, proposer, timestamp)
		  SELECT 'lohn-kn-' || g, g, '[]', $1, $2 + g * 60 FROM generate_series(0, 1439) g`, []interface{}{f.kn, seit}},
		{`INSERT INTO chain_blocks (hash, height, parent_hashes, proposer, timestamp)
		  SELECT 'lohn-ku-' || g, g, '[]', $1, $2 + g * 60 FROM generate_series(0, 1439) g`, []interface{}{f.ku, seit}},
		{`INSERT INTO registered_nodes (wallet_address, signing_address, blocks_produced) VALUES ($1, $2, 1000000)`, []interface{}{f.r, f.ka}},
	} {
		if _, err := cs.db.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("%v", err)
		}
	}
	f.konten()
	erzeugerSchnittOverride.Store(1)
	t.Cleanup(func() { erzeugerSchnittOverride.Store(0) })
	return f
}

// konten: Menschen mit 0, N kein Mensch, Topf 90 -- vor jeder Runde neu.
func (f *lohnFall) konten() {
	f.t.Helper()
	f.cs.mu.Lock()
	defer f.cs.mu.Unlock()
	for _, acc := range []*AccountState{
		{Address: f.a, IsHuman: true, LastActivityAt: f.rundeAt},
		{Address: f.b, IsHuman: true, LastActivityAt: f.rundeAt},
		{Address: f.c, IsHuman: true, LastActivityAt: f.rundeAt},
		{Address: f.r, IsHuman: true, LastActivityAt: f.rundeAt},
		{Address: f.n, LastActivityAt: f.rundeAt},
		{Address: validatorsPoolAddr, Balance: NewDecimal(90)},
	} {
		if err := f.cs.saveAccountToDB(acc); err != nil {
			f.t.Fatal(err)
		}
		f.cs.accounts.Set(acc.Address, acc)
	}
	f.cs.validatorRunde = validatorRundePruefung{}
}

func (f *lohnFall) runde() map[string]float64 {
	f.t.Helper()
	f.cs.mu.Lock()
	defer f.cs.mu.Unlock()
	shares, err := f.cs.distributeValidatorsPoolLocked(context.Background(), f.rundeAt)
	if err != nil {
		f.t.Fatal(err)
	}
	got := map[string]float64{}
	for _, s := range shares {
		if s.RundeAt != f.rundeAt {
			f.t.Fatalf("Gutschrift fuer %s ohne Rundenzeit (%d)", s.Wallet, s.RundeAt)
		}
		got[s.Wallet] = s.Amount
	}
	return got
}

// nachrechnen: die Gutschriften wie ein Nachspielender, dann der Abschluss.
// Gibt zurueck, wie oft regel dabei angeschlagen hat.
func (f *lohnFall) nachrechnen(regel string, blockZeit int64, txs []Transaction) int64 {
	f.t.Helper()
	vorher := erhaltungZaehler(regel)
	f.cs.mu.Lock()
	defer f.cs.mu.Unlock()
	for i := range txs {
		if err := f.cs.nachrechnenTxLocked(&txs[i], blockZeit); err != nil {
			f.t.Fatal(err)
		}
	}
	if err := f.cs.nachrechnenTxLocked(&Transaction{Type: "validator_distribution_pool_zero"}, blockZeit); err != nil {
		f.t.Fatal(err)
	}
	return erhaltungZaehler(regel) - vorher
}

func (f *lohnFall) gutschriften(betraege map[string]float64, rundeAt int64) []Transaction {
	var txs []Transaction
	for _, w := range []string{f.a, f.b, f.c, f.r, f.n} {
		if b, ok := betraege[w]; ok {
			txs = append(txs, Transaction{Type: "validator_distribution", Wallet: w, Amount: b, DistributionAt: rundeAt})
		}
	}
	return txs
}

// Anwesenheit und Verteilung aus der Kette: Uebergabe zur Blockzeit, eine
// Minute zaehlt einmal, nur gebundene Menschen, nur das Fenster --
// registered_nodes zaehlt nicht mehr.
func TestValidatorLohnAusKette_Verteilung_RealDB(t *testing.T) {
	f := neuerLohnFall(t)
	f.cs.mu.Lock()
	minuten, summe, err := f.cs.validatorAnwesenheitAusKetteLocked(context.Background(), f.rundeAt)
	f.cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, m := range minuten {
		got[m.wallet] = m.minuten
	}
	if got[f.a] != 360 || got[f.b] != 720 || got[f.c] != 1080 || len(got) != 3 || summe != 2160 {
		t.Fatalf("Minuten %v (Summe %d) -- erwartet A 360, B 720, C 1080", got, summe)
	}
	anteile := f.runde()
	if anteile[f.a] != 15 || anteile[f.b] != 30 || anteile[f.c] != 45 || len(anteile) != 3 {
		t.Fatalf("Anteile %v -- erwartet A 15, B 30, C 45, sonst keiner (R aus registered_nodes nicht)", anteile)
	}
}

// Vor der Umstellung (das Fenster beginnt vor erzeugerSchnittAb): wie
// bisher aus registered_nodes, ohne Rundenzeit in der Gutschrift.
func TestValidatorLohnAusKette_VorDerUmstellungWieBisher_RealDB(t *testing.T) {
	f := neuerLohnFall(t)
	seit, _ := anwesenheitsFenster(f.rundeAt)
	erzeugerSchnittOverride.Store(seit + 3600)
	f.cs.mu.Lock()
	shares, err := f.cs.distributeValidatorsPoolLocked(context.Background(), f.rundeAt)
	f.cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != 1 || shares[0].Wallet != f.r || shares[0].RundeAt != 0 {
		t.Fatalf("vor der Umstellung %+v -- erwartet nur R aus registered_nodes, ohne Rundenzeit", shares)
	}
}

// Jeder Knoten rechnet nach. Die ehrliche Runde: keine Abweichung. Jede
// Faelschung des Erzeugers schlaegt an der richtigen Regel an.
func TestValidatorLohnAusKette_Nachrechnen_RealDB(t *testing.T) {
	f := neuerLohnFall(t)
	ehrlich := map[string]float64{f.a: 15, f.b: 30, f.c: 45}
	zu := f.rundeAt + 5
	regeln := []string{"validator_anteil", "validator_kein_betreiber", "validator_doppelt", "validator_empfaenger",
		"validator_ohne_runde", "validator_runde", "validator_kein_mensch", "validator_unlesbar"}
	summe := func() int64 {
		var s int64
		for _, r := range regeln {
			s += erhaltungZaehler(r)
		}
		return s
	}
	vorher := summe()
	f.nachrechnen("validator_anteil", zu, f.gutschriften(ehrlich, f.rundeAt))
	if d := summe() - vorher; d != 0 {
		t.Fatalf("ehrliche Runde: %d Abweichungen", d)
	}

	type faelschung struct {
		regel string
		txs   func() []Transaction
		zeit  int64
	}
	mit := func(m map[string]float64, k string, v float64) map[string]float64 {
		n := map[string]float64{}
		for a, b := range m {
			n[a] = b
		}
		n[k] = v
		return n
	}
	ohne := func(m map[string]float64, k string) map[string]float64 {
		n := map[string]float64{}
		for a, b := range m {
			if a != k {
				n[a] = b
			}
		}
		return n
	}
	for _, x := range []faelschung{
		{"validator_anteil", func() []Transaction { return f.gutschriften(mit(ehrlich, f.a, 15.01), f.rundeAt) }, zu},
		{"validator_kein_betreiber", func() []Transaction { return f.gutschriften(mit(ehrlich, f.r, 1), f.rundeAt) }, zu},
		{"validator_kein_mensch", func() []Transaction { return f.gutschriften(mit(ehrlich, f.n, 1), f.rundeAt) }, zu},
		{"validator_doppelt", func() []Transaction {
			return append(f.gutschriften(ehrlich, f.rundeAt), Transaction{Type: "validator_distribution", Wallet: f.a, Amount: 15, DistributionAt: f.rundeAt})
		}, zu},
		{"validator_empfaenger", func() []Transaction { return f.gutschriften(ohne(ehrlich, f.c), f.rundeAt) }, zu},
		{"validator_ohne_runde", func() []Transaction { return f.gutschriften(ehrlich, 0) }, zu},
		{"validator_runde", func() []Transaction { return f.gutschriften(ehrlich, f.rundeAt) }, f.rundeAt + 3600},
	} {
		f.konten()
		if n := f.nachrechnen(x.regel, x.zeit, x.txs()); n == 0 {
			t.Fatalf("%s: Faelschung nicht erkannt", x.regel)
		}
	}
}

// Ein Knoten ohne die Bloecke des Fensters (frisch aus einem Snapshot)
// rechnet nicht nach, prueft aber weiter, dass jeder Empfaenger ein Mensch
// ist und keiner zweimal bekommt.
func TestValidatorLohnAusKette_OhneFensterUnsicher_RealDB(t *testing.T) {
	f := neuerLohnFall(t)
	seit, _ := anwesenheitsFenster(f.rundeAt)
	if _, err := f.cs.db.Exec(`DELETE FROM chain_blocks WHERE timestamp < $1`, seit+3600); err != nil { // die erste Stunde fehlt
		t.Fatal(err)
	}
	zu := f.rundeAt + 5
	if n := f.nachrechnen("validator_anteil", zu, f.gutschriften(map[string]float64{f.a: 89}, f.rundeAt)); n != 0 {
		t.Fatalf("ohne Fenster nachgerechnet (%d)", n)
	}
	f.konten()
	if n := f.nachrechnen("validator_kein_mensch", zu, f.gutschriften(map[string]float64{f.n: 1}, f.rundeAt)); n == 0 {
		t.Fatal("ohne Fenster: Nicht-Mensch nicht erkannt")
	}
	f.konten()
	txs := f.gutschriften(map[string]float64{f.a: 1}, f.rundeAt)
	if n := f.nachrechnen("validator_doppelt", zu, append(txs, txs[0])); n == 0 {
		t.Fatal("ohne Fenster: doppelte Gutschrift nicht erkannt")
	}
}

// Ein zurueckgewiesener Block mit einer Gutschrift hinterlaesst nichts in
// der Rundenpruefung: derselbe Empfaenger im ehrlichen Block danach ist
// nicht "doppelt".
func TestValidatorLohnAusKette_RuecknahmeVergisstGutschrift_RealDB(t *testing.T) {
	f := neuerLohnFall(t)
	zu := f.rundeAt + 5
	vorher := erhaltungZaehler("validator_doppelt")
	f.cs.mu.Lock()
	erste := f.gutschriften(map[string]float64{f.a: 15}, f.rundeAt)
	if err := f.cs.nachrechnenTxLocked(&erste[0], zu); err != nil {
		t.Fatal(err)
	}
	snap := f.cs.snapshotForRollbackLocked(nil, false, nil)
	zweite := f.gutschriften(map[string]float64{f.b: 30}, f.rundeAt)
	if err := f.cs.nachrechnenTxLocked(&zweite[0], zu); err != nil {
		t.Fatal(err)
	}
	if err := f.cs.restoreFromRollbackLocked(snap); err != nil {
		t.Fatal(err)
	}
	if err := f.cs.nachrechnenTxLocked(&zweite[0], zu); err != nil {
		t.Fatal(err)
	}
	f.cs.mu.Unlock()
	if n := erhaltungZaehler("validator_doppelt") - vorher; n != 0 {
		t.Fatalf("nach der Ruecknahme als doppelt gezaehlt (%d)", n)
	}
}

// Ein Betreiber wechselt mitten in einer Minute den Schluessel: Bloecke
// beider Schluessel in derselben Minute zaehlen einmal.
func TestValidatorLohnAusKette_WechselInDerMinuteZaehltEinmal_RealDB(t *testing.T) {
	f := neuerLohnFall(t)
	d, kd1, kd2 := distTestAddr(65), distTestAddr(74), distTestAddr(75)
	seit, _ := anwesenheitsFenster(f.rundeAt)
	minute := (seit/60 + 120) * 60 // Anfang einer Minute im Fenster
	verlaufEintrag(t, f.cs, d, kd1, seit-10*86400)
	verlaufEintrag(t, f.cs, d, kd2, minute+30-erzeugerFrist) // Fenster von kd2 ab Sekunde 30
	for i, b := range []struct {
		k string
		z int64
	}{{kd1, minute + 5}, {kd2, minute + 40}} {
		if _, err := f.cs.db.Exec(`INSERT INTO chain_blocks (hash, height, parent_hashes, proposer, timestamp) VALUES ($1, 1, '[]', $2, $3)`,
			"lohn-d-"+string(rune('a'+i)), b.k, b.z); err != nil {
			t.Fatal(err)
		}
	}
	f.cs.mu.Lock()
	f.cs.accounts.Set(d, &AccountState{Address: d, IsHuman: true})
	err := f.cs.saveAccountToDB(&AccountState{Address: d, IsHuman: true})
	minuten, _, aerr := f.cs.validatorAnwesenheitAusKetteLocked(context.Background(), f.rundeAt)
	f.cs.mu.Unlock()
	if err != nil || aerr != nil {
		t.Fatal(err, aerr)
	}
	for _, m := range minuten {
		if m.wallet == d && m.minuten != 1 {
			t.Fatalf("D: %d Minuten, erwartet 1", m.minuten)
		}
		if m.wallet == d {
			return
		}
	}
	t.Fatal("D fehlt")
}

// Eine Luecke mitten im Fenster (neu aufgesetzt): nicht nachrechnen statt
// falsch nachrechnen.
func TestValidatorLohnAusKette_LueckeImFensterUnsicher_RealDB(t *testing.T) {
	f := neuerLohnFall(t)
	seit, _ := anwesenheitsFenster(f.rundeAt)
	if _, err := f.cs.db.Exec(`DELETE FROM chain_blocks WHERE timestamp >= $1 AND timestamp < $2`, seit+8*3600, seit+10*3600); err != nil {
		t.Fatal(err)
	}
	if n := f.nachrechnen("validator_anteil", f.rundeAt+5, f.gutschriften(map[string]float64{f.a: 15, f.b: 30, f.c: 45}, f.rundeAt)); n != 0 {
		t.Fatalf("mit Luecke nachgerechnet (%d Abweichungen)", n)
	}
}

// Kann der Erzeuger die Anwesenheit nicht lesen, bleibt nur der
// Validatoren-Topf stehen -- kein Fehler, der die ganze Tagesrunde abbricht.
func TestValidatorLohnAusKette_UnlesbarLaesstTopfStehen_RealDB(t *testing.T) {
	f := neuerLohnFall(t)
	for i := 0; i < anwesenheitSchluesselGrenze+1; i++ {
		if _, err := f.cs.db.Exec(`INSERT INTO chain_blocks (hash, height, parent_hashes, proposer, timestamp) VALUES ($1, 1, '[]', $2, $3)`,
			fmt.Sprintf("lohn-viele-%d", i), fmt.Sprintf("0x%040x", 0xabc000+i), f.rundeAt-7200); err != nil {
			t.Fatal(err)
		}
	}
	f.cs.mu.Lock()
	shares, err := f.cs.distributeValidatorsPoolLocked(context.Background(), f.rundeAt)
	f.cs.mu.Unlock()
	if err != nil || len(shares) != 0 {
		t.Fatalf("zu viele Schluessel: %d Anteile, Fehler %v -- erwartet: Topf bleibt stehen, kein Fehler", len(shares), err)
	}
	if got := stand(f.cs, validatorsPoolAddr); got != 90 {
		t.Fatalf("Topf %.2f statt 90", got)
	}
}
