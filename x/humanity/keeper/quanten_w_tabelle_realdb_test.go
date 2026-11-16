package keeper

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Register der Variante W je Zeile (quanten_w_tabelle.go) gegen eine echte
// Datenbank: nur geaenderte Zeilen, Neustart, Rueckroll, Umzug aus
// chain_config, Snapshot und Resync, manipulierte Zeilen.

type qwtZeile struct {
	art, k, daten, xmin string
}

func qwtZeilen(t *testing.T, cs *ChainState) map[string]qwtZeile {
	t.Helper()
	rows, err := cs.db.Query(`SELECT art, k, daten, xmin::text FROM ` + quantenWTabelle)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]qwtZeile{}
	for rows.Next() {
		var z qwtZeile
		if err := rows.Scan(&z.art, &z.k, &z.daten, &z.xmin); err != nil {
			t.Fatal(err)
		}
		out[z.art+"|"+z.k] = z
	}
	return out
}

func qwtSetzen(t *testing.T, cs *ChainState, ctx context.Context, r *quantenWRegister) error {
	t.Helper()
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.quantenWSetzenLocked(ctx, r)
}

func qwtLaden(cs *ChainState) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.quantenWLaden(context.Background())
}

// qwtStaende: echte Register aus den Ablaeufen der Dienst-Tests (ohne
// Datenbank) -- gepraegt, Anfrage offen, geprueft, eingeloest (tot); dazu
// ein S-Token aus einer zweiten Welt.
func qwtStaende(t *testing.T) (gepraegt, offen, geprueft, tot *quantenWRegister, token map[string]*quantenWSEintrag) {
	t.Helper()
	q := neueQwDienste(t, 200)
	w := q.w
	q.praegen(t)
	gepraegt = w.cs.quantenWJetzt()
	q.zeit += 10
	w.annehmen(w.anfrageTx(w.anna, 0, 16, "", q.zeit), q.zeit+1)
	q.zeit += 2
	q.takt()
	a := w.anfrage(w.cs)
	w.annehmen(w.antwortTx(w.anna, 0, w.p.ehrlich(a.Stellen, a.Basen)), q.zeit+1)
	offen = w.cs.quantenWJetzt()
	q.zeit += 2
	q.takt()
	geprueft = w.cs.quantenWJetzt()
	q.zeit += 50
	w.annehmen(w.anfrageTx(w.bert, 1, qwLK, w.bert, q.zeit), q.zeit+1)
	q.zeit += 2
	q.takt()
	a = w.anfrage(w.cs)
	w.annehmen(w.antwortTx(w.bert, 1, w.p.ehrlich(a.Stellen, a.Basen)), q.zeit+1)
	q.zeit += 2
	q.takt()
	tot = w.cs.quantenWJetzt()
	if len(gepraegt.Noten) != 1 || offen.Noten[w.p.K].note.Anfrage == nil || geprueft.Noten[w.p.K].note.Anfrage != nil || len(tot.Noten) != 0 || len(tot.Tote) != 1 {
		t.Fatal("Voraussetzung: die Ablaeufe liefern nicht die erwarteten Staende")
	}
	s := neueQsDienste(t, 64, 24)
	s.ausgeben(t)
	token = s.w.cs.quantenWJetzt().Token
	if len(token) != 1 {
		t.Fatal("Voraussetzung: kein S-Token")
	}
	return
}

func qwtMit(r *quantenWRegister, token map[string]*quantenWSEintrag) *quantenWRegister {
	n := r.kopie()
	for k, e := range token {
		n.Token[k] = e
	}
	return n
}

func TestQuantenW_Tabelle_RealDB(t *testing.T) {
	gepraegt, offen, geprueft, tot, token := qwtStaende(t)
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-quanten-w-tabelle-realdb-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	ctx := context.Background()

	// Praegen: eine Zeile je Eintrag, chain_config bleibt leer, die Wurzel
	// ist die des ganzen Registers (wie vor der Tabelle).
	r1 := qwtMit(gepraegt, token)
	if err := qwtSetzen(t, cs, ctx, r1); err != nil {
		t.Fatal(err)
	}
	z1 := qwtZeilen(t, cs)
	if len(z1) != 2 || cs.getConfigValueDB(quantenWKonfigSchluessel) != "" {
		t.Fatalf("%d Zeilen, chain_config %q", len(z1), cs.getConfigValueDB(quantenWKonfigSchluessel))
	}
	if cs.quantenWWurzel() != neuerQuantenWStand(r1, r1.kodiert()).wurzel || cs.quantenWWurzel() == "" {
		t.Fatal("Wurzel nicht die des ganzen Registers")
	}

	// Anfrage, Pruefung: nur die Zeile der Note wird geschrieben, die des
	// Tokens bleibt unberuehrt (xmin gleich).
	var tokZeile string
	for k, z := range z1 {
		if z.art == quantenWArtTok {
			tokZeile = k
		}
	}
	for _, r := range []*quantenWRegister{qwtMit(offen, token), qwtMit(geprueft, token)} {
		vorher := qwtZeilen(t, cs)
		if err := qwtSetzen(t, cs, ctx, r); err != nil {
			t.Fatal(err)
		}
		nachher := qwtZeilen(t, cs)
		if len(nachher) != 2 || nachher[tokZeile].xmin != vorher[tokZeile].xmin {
			t.Fatal("unveraenderte Zeile neu geschrieben")
		}
	}
	// Dieselbe Lage noch einmal: nichts wird geschrieben.
	vorher := qwtZeilen(t, cs)
	if err := qwtSetzen(t, cs, ctx, qwtMit(geprueft, token)); err != nil {
		t.Fatal(err)
	}
	for k, z := range qwtZeilen(t, cs) {
		if vorher[k].xmin != z.xmin {
			t.Fatalf("Zeile %s ohne Aenderung neu geschrieben", k)
		}
	}

	// Einloesen: die Note geht, eine tote kommt.
	r4 := qwtMit(tot, token)
	if err := qwtSetzen(t, cs, ctx, r4); err != nil {
		t.Fatal(err)
	}
	z4 := qwtZeilen(t, cs)
	arten := map[string]int{}
	for _, z := range z4 {
		arten[z.art]++
	}
	if len(z4) != 2 || arten[quantenWArtTot] != 1 || arten[quantenWArtTok] != 1 {
		t.Fatalf("nach dem Einloesen: %v", arten)
	}
	wurzel := cs.quantenWWurzel()

	// Neustart: dasselbe Register, dieselbe Wurzel.
	neu := testKnoten(t, "unused-quanten-w-tabelle-realdb-test-2.json")
	if neu.quantenWWurzel() != wurzel || neu.quantenWStandJetzt().kodiert != r4.kodiert() {
		t.Fatal("nach dem Neustart ein anderes Register")
	}

	// Rueckroll: eine Aenderung in einer Transaktion, die zurueckgeht, steht
	// nicht in der Tabelle; der Stand im Speicher kehrt zurueck (wie
	// blockRollbackSnapshot) und schreibt danach richtig weiter.
	stand := cs.quantenWReg.Load()
	tx, err := cs.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	err = qwtSetzen(t, cs, withTx(ctx, tx), qwtMit(geprueft, token))
	cs.quantenWReg.Store(stand)
	tx.Rollback()
	if err != nil {
		t.Fatal(err)
	}
	for k, z := range qwtZeilen(t, cs) {
		if z4[k].xmin != z.xmin {
			t.Fatalf("Rueckroll: Zeile %s veraendert", k)
		}
	}
	if err := qwtSetzen(t, cs, ctx, r1); err != nil || len(qwtZeilen(t, cs)) != 2 {
		t.Fatalf("nach dem Rueckroll weiterschreiben: %v", err)
	}
	if err := qwtSetzen(t, cs, ctx, r4); err != nil {
		t.Fatal(err)
	}

	// Missbrauch: Tabelle und Speicher auseinander (eine Zeile fehlt) --
	// Schreiben scheitert, statt still weiterzumachen.
	if _, err := cs.db.Exec(`DELETE FROM `+quantenWTabelle+` WHERE art = $1`, quantenWArtTot); err != nil {
		t.Fatal(err)
	}
	if err := qwtSetzen(t, cs, ctx, qwtMit(neuesQuantenWRegister(), token)); err == nil {
		t.Fatal("geloeschte Zeile still uebergangen")
	}
	if err := qwtLaden(cs); err != nil {
		t.Fatal(err)
	}
	if err := qwtSetzen(t, cs, ctx, r4); err != nil {
		t.Fatal(err)
	}

	// Snapshot: das Register als ein Wert wie bisher.
	snap := cs.ExportSnapshot(nil, 0, true)
	if snap == nil || snap.ChainConfig[quantenWKonfigSchluessel] != r4.kodiert() {
		t.Fatal("Snapshot traegt das Register nicht")
	}

	// Resync: die Tabelle ersetzend (eine fremde Zeile verschwindet), dieselbe
	// Wurzel, chain_config wieder leer.
	if _, err := cs.db.Exec(`INSERT INTO `+quantenWTabelle+` (art, k, daten) VALUES ($1, $2, $3)`, quantenWArtTot, strings.Repeat("e", 64),
		fmt.Sprintf(`{"k":"%s","zeit":5}`, strings.Repeat("e", 64))); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALLOW_RESYNC_REGRESSION", "true")
	g := &resyncSnapshot{snap: snap, peerURL: "http://peer.invalid/api/snapshot", signer: resyncTestSigner}
	if err := cs.wendeResyncSnapshotAn(g); err != nil {
		t.Fatalf("Resync: %v", err)
	}
	if z := qwtZeilen(t, cs); len(z) != 2 || cs.quantenWWurzel() != wurzel || cs.getConfigValueDB(quantenWKonfigSchluessel) != "" {
		t.Fatalf("nach dem Resync: %d Zeilen, chain_config %q", len(z), cs.getConfigValueDB(quantenWKonfigSchluessel))
	}
	// Ein Snapshot ohne Register leert die Tabelle.
	leer := *snap
	leer.Timestamp = time.Now().Unix()
	leer.ChainConfig = map[string]string{}
	for k, v := range snap.ChainConfig {
		if k != quantenWKonfigSchluessel {
			leer.ChainConfig[k] = v
		}
	}
	if err := cs.wendeResyncSnapshotAn(&resyncSnapshot{snap: &leer, peerURL: g.peerURL, signer: resyncTestSigner}); err != nil {
		t.Fatalf("Resync ohne Register: %v", err)
	}
	if len(qwtZeilen(t, cs)) != 0 || cs.quantenWWurzel() != "" {
		t.Fatal("Resync ohne Register: Tabelle nicht leer")
	}

	// Umzug: ein Knoten vor der Tabelle hat das Register in chain_config --
	// beim Laden zieht es um, chain_config wird leer, die Wurzel bleibt.
	if err := cs.setConfigValueDB(quantenWKonfigSchluessel, r4.kodiert()); err != nil {
		t.Fatal(err)
	}
	if err := qwtLaden(cs); err != nil {
		t.Fatal(err)
	}
	if len(qwtZeilen(t, cs)) != 2 || cs.quantenWWurzel() != wurzel || cs.getConfigValueDB(quantenWKonfigSchluessel) != "" {
		t.Fatal("Umzug aus chain_config nicht vollstaendig")
	}
	// Hat die Tabelle schon Zeilen (Snapshot-Import in einen laufenden
	// Knoten setzt chain_config), gelten sie; chain_config wird geleert.
	if err := cs.setConfigValueDB(quantenWKonfigSchluessel, qwtMit(gepraegt, token).kodiert()); err != nil {
		t.Fatal(err)
	}
	if err := qwtLaden(cs); err != nil {
		t.Fatal(err)
	}
	if cs.quantenWWurzel() != wurzel || cs.getConfigValueDB(quantenWKonfigSchluessel) != "" {
		t.Fatal("laufender Knoten hat seinen Stand verloren")
	}

	// Missbrauch: manipulierte Zeilen halten den Knoten an (fail-closed).
	var notenZeile string
	for _, z := range qwtZeilen(t, cs) {
		if z.art == quantenWArtTok {
			notenZeile = z.daten
		}
	}
	tk := strings.Repeat("e", 64)
	for _, f := range []struct{ name, art, k, daten string }{
		{"unbekannte Art", "mehr", tk, fmt.Sprintf(`{"k":"%s","zeit":5}`, tk)},
		{"K der Zeile anders als im Eintrag", quantenWArtTot, tk, fmt.Sprintf(`{"k":"%s","zeit":5}`, strings.Repeat("d", 64))},
		{"unbekanntes Feld", quantenWArtTot, tk, fmt.Sprintf(`{"k":"%s","zeit":5,"mehr":1}`, tk)},
		{"Daten nach dem Eintrag", quantenWArtTot, tk, fmt.Sprintf(`{"k":"%s","zeit":5}{}`, tk)},
		{"kein JSON", quantenWArtTot, tk, `nein`},
		{"Zeit null", quantenWArtTot, tk, fmt.Sprintf(`{"k":"%s","zeit":0}`, tk)},
		{"K kein Hash", quantenWArtTot, "xyz", `{"k":"xyz","zeit":5}`},
		{"Token als tote Note", quantenWArtTot, "", ""},
		{"Token-Daten als Note", quantenWArtNote, "", ""},
	} {
		art, k, daten := f.art, f.k, f.daten
		if f.name == "Token als tote Note" || f.name == "Token-Daten als Note" {
			// Ein K in zwei Arten: der Token noch einmal als tote Note bzw.
			// als (falsch geformte) Note.
			for kt := range token {
				k = kt
			}
			daten = fmt.Sprintf(`{"k":"%s","zeit":5}`, k)
			if art == quantenWArtNote {
				daten = notenZeile
			}
		}
		if _, err := cs.db.Exec(`INSERT INTO `+quantenWTabelle+` (art, k, daten) VALUES ($1, $2, $3)`, art, k, daten); err != nil {
			t.Fatalf("%s: %v", f.name, err)
		}
		if err := qwtLaden(cs); err == nil {
			t.Errorf("%s: geladen", f.name)
		}
		if _, err := cs.db.Exec(`DELETE FROM `+quantenWTabelle+` WHERE art = $1 AND k = $2`, art, k); err != nil {
			t.Fatal(err)
		}
	}
	if err := qwtLaden(cs); err != nil || cs.quantenWWurzel() != wurzel {
		t.Fatalf("aufgeraeumt, trotzdem nicht ladbar: %v", err)
	}
	// Mehr Zeilen, als es geben kann.
	if _, err := cs.db.Exec(`INSERT INTO `+quantenWTabelle+` (art, k, daten)
SELECT 'tot', k, '{"k":"' || k || '","zeit":5}' FROM (SELECT lpad(to_hex(g), 64, '0') AS k FROM generate_series(1, $1) g) s`, quantenWZeilenHoechstens); err != nil {
		t.Fatal(err)
	}
	if err := qwtLaden(cs); err == nil || !strings.Contains(err.Error(), "mehr als") {
		t.Fatalf("zu viele Zeilen: %v", err)
	}
}
