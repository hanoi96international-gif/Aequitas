package keeper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
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
//
// Alle Bloecke haengen in einer Kette (nach Zeit), an ihrer Spitze der Anker
// 30 s vor der Runde -- gezaehlt wird nur, was im Kegel des Ankers liegt.
type lohnFall struct {
	t       *testing.T
	cs      *ChainState
	rundeAt int64
	anker   string
	// eltern: die Eltern des tragenden Blocks beim Nachrechnen; nil = der Anker.
	eltern []string
	// hexe: Bloecke mit echtem Hash (Anker muessen Blockhashes sein).
	hexe           []string
	a, b, c, n, r  string
	ka, kb, kn, ku string
}

// lohnAnkerScore: hoeher als jeder echte Blue-Score in der Testdatenbank.
const lohnAnkerScore = int64(1) << 50

// hexHash: ein echter Blockhash fuer name, als Testblock vermerkt.
func (f *lohnFall) hexHash(name string) string {
	h := sha256.Sum256([]byte("lohn-test-" + name))
	x := hex.EncodeToString(h[:])
	f.hexe = append(f.hexe, x)
	return x
}

// verketten: alle 'lohn-%'-Bloecke nach Zeit zu einer Kette; der Anker ist
// der juengste. Nach jedem Einfuegen von Bloecken neu aufrufen.
func (f *lohnFall) verketten() {
	f.t.Helper()
	if _, err := f.cs.db.Exec(`WITH o AS (SELECT hash, lag(hash) OVER (ORDER BY timestamp, hash) AS vor
		FROM chain_blocks WHERE hash LIKE 'lohn-%' OR hash = ANY($1))
		UPDATE chain_blocks c SET parent_hashes = CASE WHEN o.vor IS NULL THEN '[]' ELSE json_build_array(o.vor)::text END
		FROM o WHERE c.hash = o.hash`, pq.Array(f.hexe)); err != nil {
		f.t.Fatal(err)
	}
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
	// Auf die volle Minute: gezaehlt wird je Kalenderminute (timestamp/60),
	// die Bloecke liegen bei seit + k*60 + {0,1,2}. Lag die Sekunde von
	// rundeAt auf 58/59, rutschte der letzte Block einer Minute in die
	// naechste, und A bekam eine Minute mehr (361 statt 360, in der CI rot).
	f := &lohnFall{t: t, cs: cs, rundeAt: time.Now().Unix() / 60 * 60,
		a: distTestAddr(60), b: distTestAddr(61), c: distTestAddr(62), n: distTestAddr(63), r: distTestAddr(64),
		ka: distTestAddr(70), kb: distTestAddr(71), kn: distTestAddr(72), ku: distTestAddr(73)}
	for _, q := range []string{`DELETE FROM chain_blocks WHERE hash LIKE 'lohn-%'`, `DELETE FROM validator_verlauf`, `DELETE FROM registered_nodes`} {
		if _, err := cs.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		cs.db.Exec(`DELETE FROM chain_blocks WHERE hash LIKE 'lohn-%' OR hash = ANY($1)`, pq.Array(f.hexe))
	})
	anker := f.hexHash("anker")
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
		{`INSERT INTO chain_blocks (hash, height, parent_hashes, proposer, timestamp, blue_score)
		  VALUES ($4, 1, '[]', $1, $2, $3)`, []interface{}{f.ku, f.rundeAt - 30, lohnAnkerScore, anker}},
		{`INSERT INTO registered_nodes (wallet_address, signing_address, blocks_produced) VALUES ($1, $2, 1000000)`, []interface{}{f.r, f.ka}},
	} {
		if _, err := cs.db.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("%v", err)
		}
	}
	f.anker = anker
	f.verketten()
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
		if s.RundeAt != f.rundeAt || s.Anker != f.anker {
			f.t.Fatalf("Gutschrift fuer %s: Rundenzeit %d, Anker %q", s.Wallet, s.RundeAt, s.Anker)
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
	f.cs.nachrechnenEltern = f.eltern
	if f.eltern == nil {
		f.cs.nachrechnenEltern = []string{f.anker}
	}
	defer func() { f.cs.nachrechnenEltern = nil }()
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
			txs = append(txs, Transaction{Type: "validator_distribution", Wallet: w, Amount: b, DistributionAt: rundeAt, Anker: f.anker})
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
	minuten, summe, stand, err := f.cs.validatorAnwesenheitAusKetteLocked(context.Background(), f.rundeAt, f.anker)
	f.cs.mu.Unlock()
	if err != nil || stand != kegelVollstaendig {
		t.Fatal(stand, err)
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
	if len(shares) != 1 || shares[0].Wallet != f.r || shares[0].RundeAt != 0 || shares[0].Anker != "" {
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
		"validator_ohne_runde", "validator_runde", "validator_kein_mensch", "validator_unlesbar",
		"validator_ohne_anker", "validator_anker", "validator_kegel"}
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
			return append(f.gutschriften(ehrlich, f.rundeAt), Transaction{Type: "validator_distribution", Wallet: f.a, Amount: 15, DistributionAt: f.rundeAt, Anker: f.anker})
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
	f.verketten()
	f.cs.mu.Lock()
	f.cs.accounts.Set(d, &AccountState{Address: d, IsHuman: true})
	err := f.cs.saveAccountToDB(&AccountState{Address: d, IsHuman: true})
	minuten, _, _, aerr := f.cs.validatorAnwesenheitAusKetteLocked(context.Background(), f.rundeAt, f.anker)
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
	f.verketten()
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

// ------------------------------------------------------- Anker und Kegel

// Missbrauch des Ankers: ohne Anker, zwei Anker in einer Runde, ein Anker
// ausserhalb der zehn Minuten vor der Runde (zu alt, in der Zukunft).
func TestValidatorLohnAusKette_AnkerFaelschungen_RealDB(t *testing.T) {
	f := neuerLohnFall(t)
	zwei, alt, spaet := f.hexHash("anker-zwei"), f.hexHash("anker-alt"), f.hexHash("anker-spaet")
	for _, b := range []struct {
		hash string
		zeit int64
	}{{zwei, f.rundeAt - 40}, {alt, f.rundeAt - 1200}, {spaet, f.rundeAt + 60}} {
		if _, err := f.cs.db.Exec(`INSERT INTO chain_blocks (hash, height, parent_hashes, proposer, timestamp) VALUES ($1, 1, '[]', $2, $3)`,
			b.hash, f.ku, b.zeit); err != nil {
			t.Fatal(err)
		}
	}
	f.verketten()
	ehrlich := map[string]float64{f.a: 15, f.b: 30, f.c: 45}
	zu := f.rundeAt + 5
	mitAnker := func(txs []Transaction, i int, anker string) []Transaction {
		txs[i].Anker = anker
		return txs
	}
	for _, x := range []struct {
		name, regel string
		txs         []Transaction
	}{
		{"ohne Anker", "validator_ohne_anker", mitAnker(f.gutschriften(ehrlich, f.rundeAt), 0, "")},
		{"zwei Anker", "validator_anker", mitAnker(f.gutschriften(ehrlich, f.rundeAt), 1, zwei)},
		{"Anker zu alt", "validator_anker", mitAnker(mitAnker(mitAnker(f.gutschriften(ehrlich, f.rundeAt), 0, alt), 1, alt), 2, alt)},
		{"Anker kein Blockhash", "validator_anker", mitAnker(f.gutschriften(ehrlich, f.rundeAt), 0, "LOHN-ANKER")},
		{"Anker gross geschrieben", "validator_anker", mitAnker(mitAnker(mitAnker(f.gutschriften(ehrlich, f.rundeAt), 0, strings.ToUpper(f.anker)), 1, strings.ToUpper(f.anker)), 2, strings.ToUpper(f.anker))},
		{"Anker nach der Runde", "validator_anker", mitAnker(mitAnker(mitAnker(f.gutschriften(ehrlich, f.rundeAt), 0, spaet), 1, spaet), 2, spaet)},
	} {
		f.konten()
		if n := f.nachrechnen(x.regel, zu, x.txs); n == 0 {
			t.Fatalf("%s: nicht erkannt (%s)", x.name, x.regel)
		}
	}
	// Ein anderer gueltiger Anker im Fenster ist kein Fehler: der Kegel von
	// lohn-anker-zwei enthaelt hier dieselben Bloecke des Fensters.
	f.konten()
	alle := []string{"validator_anteil", "validator_kein_betreiber", "validator_empfaenger", "validator_anker", "validator_kegel"}
	vorher := int64(0)
	for _, r := range alle {
		vorher += erhaltungZaehler(r)
	}
	txs := f.gutschriften(ehrlich, f.rundeAt)
	for i := range txs {
		txs[i].Anker = zwei
	}
	f.nachrechnen("validator_anteil", zu, txs)
	for _, r := range alle {
		vorher -= erhaltungZaehler(r)
	}
	if vorher != 0 {
		t.Fatalf("gueltiger zweiter Anker: %d Abweichungen", -vorher)
	}
}

// Ein erfundener Anker, oder einer, der kein Vorfahr des tragenden Blocks
// ist, ist eine Abweichung -- KEIN "unsicher": sonst schaltete der Erzeuger
// das Nachrechnen mit einem Hash ab, den es nicht gibt (Pruefung des
// Kegel-PR, HOCH 1), und naehme den ganzen Topf.
func TestValidatorLohnAusKette_ErfundenerAnkerIstAbweichung_RealDB(t *testing.T) {
	f := neuerLohnFall(t)
	zu := f.rundeAt + 5
	echt := f.anker
	f.anker = f.hexHash("gibt-es-nicht")
	f.eltern = []string{echt}
	if n := f.nachrechnen("validator_anker", zu, f.gutschriften(map[string]float64{f.a: 90}, f.rundeAt)); n == 0 {
		t.Fatal("erfundener Anker nicht erkannt")
	}
	// Ein echter Block, aber nicht Vorfahr des tragenden Blocks.
	f.konten()
	f.anker = echt
	f.eltern = []string{"lohn-kb-rand-0"}
	if n := f.nachrechnen("validator_anker", zu, f.gutschriften(map[string]float64{f.a: 15, f.b: 30, f.c: 45}, f.rundeAt)); n == 0 {
		t.Fatal("Anker ausserhalb der Vorfahren nicht erkannt")
	}
}

// Fehlt DIESEM Knoten ein Block auf dem Weg zum Anker (eigene Luecke),
// rechnet er nicht nach -- prueft aber weiter, dass nur Menschen bekommen.
// Ueber "unsicher" entscheidet die eigene Geschichte, nicht der Erzeuger.
func TestValidatorLohnAusKette_EigeneLueckeUnsicher_RealDB(t *testing.T) {
	f := neuerLohnFall(t)
	zu := f.rundeAt + 5
	f.eltern = []string{f.hexHash("eltern-fehlen-hier")}
	if n := f.nachrechnen("validator_anker", zu, f.gutschriften(map[string]float64{f.a: 89}, f.rundeAt)); n != 0 {
		t.Fatalf("mit eigener Luecke geurteilt (%d)", n)
	}
	f.konten()
	if n := f.nachrechnen("validator_anteil", zu, f.gutschriften(map[string]float64{f.a: 89}, f.rundeAt)); n != 0 {
		t.Fatalf("mit eigener Luecke nachgerechnet (%d)", n)
	}
	f.konten()
	if n := f.nachrechnen("validator_kein_mensch", zu, f.gutschriften(map[string]float64{f.n: 1}, f.rundeAt)); n == 0 {
		t.Fatal("eigene Luecke: Nicht-Mensch nicht erkannt")
	}
}

// Gezaehlt wird nur der Kegel: Bloecke, die vom Anker aus nicht erreichbar
// sind (ein Seitenzweig, den nur dieser Knoten kennt, oder ein spaet
// eingereichter Block), aendern weder die Verteilung noch das Nachrechnen.
// Genau das macht die Zaehlung bei jedem Knoten gleich.
func TestValidatorLohnAusKette_NurDerKegelZaehlt_RealDB(t *testing.T) {
	f := neuerLohnFall(t)
	seit, _ := anwesenheitsFenster(f.rundeAt)
	// KB baut in der zweiten Tageshaelfte -- aber nicht im Kegel.
	if _, err := f.cs.db.Exec(`INSERT INTO chain_blocks (hash, height, parent_hashes, proposer, timestamp)
		SELECT 'lohn-seite-' || g, g, '[]', $1, $2 + g * 60 FROM generate_series(0, 719) g`, f.kb, seit+12*3600); err != nil {
		t.Fatal(err)
	}
	anteile := f.runde()
	if anteile[f.a] != 15 || anteile[f.b] != 30 || anteile[f.c] != 45 || len(anteile) != 3 {
		t.Fatalf("Anteile %v -- der Seitenzweig hat mitgezaehlt", anteile)
	}
	f.konten()
	if n := f.nachrechnen("validator_anteil", f.rundeAt+5, f.gutschriften(map[string]float64{f.a: 15, f.b: 30, f.c: 45}, f.rundeAt)); n != 0 {
		t.Fatalf("Nachrechnen mit Seitenzweig: %d Abweichungen", n)
	}
	// Gegenprobe: haengt der Zweig im Kegel, zaehlt er.
	f.verketten()
	f.konten()
	if anteile := f.runde(); anteile[f.b] <= 30 {
		t.Fatalf("Anteile %v -- der verkettete Zweig zaehlt nicht", anteile)
	}
}

// Ein Kegel ueber der Grenze: der Erzeuger zahlt nicht aus, und wer es doch
// tut, wird erkannt -- derselbe Anker ergibt bei jedem denselben Kegel.
func TestValidatorLohnAusKette_KegelZuGross_RealDB(t *testing.T) {
	f := neuerLohnFall(t)
	seit, _ := anwesenheitsFenster(f.rundeAt)
	var vorrat, kegel int
	if err := f.cs.db.QueryRow(`SELECT count(*) FROM chain_blocks WHERE timestamp >= $1 AND timestamp <= $2`,
		seit-anwesenheitKegelPuffer, f.rundeAt-30+anwesenheitKegelPuffer).Scan(&vorrat); err != nil {
		t.Fatal(err)
	}
	if err := f.cs.db.QueryRow(`SELECT count(*) FROM chain_blocks WHERE hash LIKE 'lohn-%' AND timestamp >= $1`,
		seit-anwesenheitKegelPuffer).Scan(&kegel); err != nil {
		t.Fatal(err)
	}
	alt := anwesenheitKegelGrenze
	t.Cleanup(func() { anwesenheitKegelGrenze = alt })
	anwesenheitKegelGrenze = kegel - 10 // der Kegel ist knapp zu gross, der Vorrat passt
	if 2*anwesenheitKegelGrenze < vorrat {
		t.Fatalf("Testdatenbank zu voll: %d Zeilen im Zeitraum, Kegel %d", vorrat, kegel)
	}
	if anteile := f.runde(); len(anteile) != 0 {
		t.Fatalf("zu grosser Kegel ausgezahlt: %v", anteile)
	}
	if got := stand(f.cs, validatorsPoolAddr); got != 90 {
		t.Fatalf("Topf %.2f statt 90", got)
	}
	f.konten()
	if n := f.nachrechnen("validator_kegel", f.rundeAt+5, f.gutschriften(map[string]float64{f.a: 15, f.b: 30, f.c: 45}, f.rundeAt)); n == 0 {
		t.Fatal("Auszahlung trotz zu grossem Kegel nicht erkannt")
	}
}

// Wird der Vorrat DIESES Knotens zu gross (er kennt mehr Bloecke als andere),
// ist das kein Urteil: der Erzeuger zahlt nicht aus, wer nachspielt, rechnet
// nicht nach -- niemand wird zu Unrecht beschuldigt.
func TestValidatorLohnAusKette_VorratZuGrossKeinUrteil_RealDB(t *testing.T) {
	f := neuerLohnFall(t)
	alt := anwesenheitKegelGrenze
	t.Cleanup(func() { anwesenheitKegelGrenze = alt })
	anwesenheitKegelGrenze = 100
	if anteile := f.runde(); len(anteile) != 0 {
		t.Fatalf("mit zu grossem Vorrat ausgezahlt: %v", anteile)
	}
	f.konten()
	for _, regel := range []string{"validator_kegel", "validator_anteil"} {
		if n := f.nachrechnen(regel, f.rundeAt+5, f.gutschriften(map[string]float64{f.a: 89}, f.rundeAt)); n != 0 {
			t.Fatalf("zu grosser Vorrat: %s angeschlagen (%d)", regel, n)
		}
		f.konten()
	}
}

// Kein Anker in den zehn Minuten vor der Runde: der Topf bleibt stehen.
func TestValidatorLohnAusKette_OhneAnkerTopfBleibt_RealDB(t *testing.T) {
	f := neuerLohnFall(t)
	if _, err := f.cs.db.Exec(`DELETE FROM chain_blocks WHERE timestamp >= $1 AND timestamp <= $2`,
		f.rundeAt-validatorAnkerHoechstensAlt, f.rundeAt); err != nil {
		t.Fatal(err)
	}
	if anteile := f.runde(); len(anteile) != 0 {
		t.Fatalf("ohne Anker ausgezahlt: %v", anteile)
	}
	if got := stand(f.cs, validatorsPoolAddr); got != 90 {
		t.Fatalf("Topf %.2f statt 90", got)
	}
}
