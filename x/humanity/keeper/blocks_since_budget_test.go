package keeper

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
)

// Seitenabruf aus der Datenbank, 12.09.2026: die alte Fassung dekodierte
// limit+512 Ruempfe fuer eine Seite von ~16 Bloecken (unter Last 300 MB aus
// Postgres und 2-3 GB Zwischenspeicher je Abruf). Diese Tests halten fest,
// dass Ruempfe nur in Seitenreihenfolge und nur bis zum Budget geladen werden.

func neuerBlocksSinceTestState(t *testing.T) *ChainState {
	t.Helper()
	db, err := sql.Open("postgres", "postgres://nirgends/nichts?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return &ChainState{db: db}
}

func koepfeFuerTest(n int) []*Block {
	out := make([]*Block, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, &Block{Hash: fmt.Sprintf("h%03d", i), Height: int64(1000 + i)})
	}
	return out
}

// rumpfJSON baut einen Klartext-Rumpf von ungefaehr `bytes` Groesse.
func rumpfJSON(bytes int) string {
	tx := `{"tx_hash":"0xabc","from":"0xfrom","to":"0xto","amount":1}`
	n := bytes / (len(tx) + 1)
	if n < 1 {
		n = 1
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = tx
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func TestLadeRuempfeMitBudget_GrosseBloeckeNurBisZumBudget(t *testing.T) {
	cs := neuerBlocksSinceTestState(t)
	abfragen := 0
	alt := ladeRuempfeFn
	ladeRuempfeFn = func(cs *ChainState, hashes []string) (map[string]blockRumpfRoh, error) {
		abfragen++
		m := map[string]blockRumpfRoh{}
		for _, h := range hashes {
			m[h] = blockRumpfRoh{raw: rumpfJSON(1 << 20)} // ~1 MB je Block
		}
		return m, nil
	}
	defer func() { ladeRuempfeFn = alt }()

	seite := koepfeFuerTest(200)
	out, err := cs.ladeRuempfeMitBudget(seite, 12<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) < 12 || len(out) > 14 {
		t.Fatalf("bei ~1 MB je Block und 12 MB Budget muessen 12-14 Bloecke kommen, nicht %d", len(out))
	}
	if abfragen != 1 {
		t.Fatalf("grosse Bloecke: nur das erste Haeppchen (%d) darf abgefragt werden, es waren %d Abfragen", dbSinceRumpfHaeppchen, abfragen)
	}
	for i, b := range out {
		if b.Hash != fmt.Sprintf("h%03d", i) || len(b.Transactions) == 0 {
			t.Fatalf("Seitenreihenfolge oder Rumpf verletzt an Position %d: %s (%d txs)", i, b.Hash, len(b.Transactions))
		}
	}
}

func TestLadeRuempfeMitBudget_KleineBloeckeGanzeSeite(t *testing.T) {
	cs := neuerBlocksSinceTestState(t)
	abfragen := 0
	alt := ladeRuempfeFn
	ladeRuempfeFn = func(cs *ChainState, hashes []string) (map[string]blockRumpfRoh, error) {
		abfragen++
		m := map[string]blockRumpfRoh{}
		for _, h := range hashes {
			m[h] = blockRumpfRoh{raw: "[]"}
		}
		return m, nil
	}
	defer func() { ladeRuempfeFn = alt }()

	out, err := cs.ladeRuempfeMitBudget(koepfeFuerTest(100), 12<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 100 {
		t.Fatalf("kleine Bloecke: die ganze Seite (100) muss kommen, nicht %d", len(out))
	}
	if want := (100 + dbSinceRumpfHaeppchen - 1) / dbSinceRumpfHaeppchen; abfragen != want {
		t.Fatalf("%d Haeppchen-Abfragen erwartet, %d gemacht", want, abfragen)
	}
}

// Der erste Block kommt immer, auch wenn er allein ueber dem Budget liegt --
// sonst kaeme ein Aufrufer an einem Riesenblock nie vorbei.
func TestLadeRuempfeMitBudget_ErsterBlockImmer(t *testing.T) {
	cs := neuerBlocksSinceTestState(t)
	alt := ladeRuempfeFn
	ladeRuempfeFn = func(cs *ChainState, hashes []string) (map[string]blockRumpfRoh, error) {
		m := map[string]blockRumpfRoh{}
		for _, h := range hashes {
			m[h] = blockRumpfRoh{raw: rumpfJSON(3 << 20)}
		}
		return m, nil
	}
	defer func() { ladeRuempfeFn = alt }()

	out, err := cs.ladeRuempfeMitBudget(koepfeFuerTest(5), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("genau der erste Block muss kommen, es kamen %d", len(out))
	}
}
