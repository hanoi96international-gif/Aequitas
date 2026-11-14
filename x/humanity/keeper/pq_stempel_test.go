package keeper

import (
	"context"
	"crypto/mldsa"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// PQ-Stempel (pq_stempel.go): Konfiguration, Gutfall, Faelschungen,
// Erzeuger, Hash bleibt, Datenbank behaelt den Stempel.

const stempelErzeuger = "0x00000000000000000000000000000000000000e1"

func stempelKonf(ab int64, schluessel map[string]*mldsa.PrivateKey) pqStempelKonfig {
	k := pqStempelKonfig{AbHoehe: ab, Schluessel: map[string]string{}}
	for a, sk := range schluessel {
		k.Schluessel[a] = pqHex(sk)
	}
	return k
}

func stempelBlock(hoehe, zeit int64, sk *mldsa.PrivateKey) *Block {
	b := &Block{Height: hoehe, Timestamp: zeit, Proposer: stempelErzeuger, Hash: fmt.Sprintf("%064x", hoehe*7919)}
	if sk != nil {
		pqStempelSchluesselTest.Store(sk)
		(&BlockDAG{}).pqStempelSetzen(b)
		pqStempelSchluesselTest.Store(nil)
	}
	return b
}

func TestPQStempel_Konfiguration(t *testing.T) {
	sk := testPQSchluessel(t)
	gut := fmt.Sprintf(`{"chain_id":"x","pq_stempel":{"ab_hoehe":2100000,"schluessel":{%q:%q}}}`, stempelErzeuger, pqHex(sk))
	if k, err := ladePQStempelKonfig([]byte(gut)); err != nil || k.AbHoehe != 2100000 || len(k.Schluessel) != 1 {
		t.Fatalf("gute Konfiguration: %v %+v", err, k)
	}
	if k, err := ladePQStempelKonfig([]byte(`{"chain_id":"x"}`)); err != nil || k.AbHoehe != 0 {
		t.Fatalf("ohne Eintrag schlafend erwartet: %v", err)
	}
	schlecht := map[string]string{
		"Stichhoehe ohne Schluessel":  `{"pq_stempel":{"ab_hoehe":2100000}}`,
		"Stichhoehe negativ":          `{"pq_stempel":{"ab_hoehe":-1}}`,
		"Tippfehler im Namen":         `{"pqstempel":{"ab_hoehe":0}}`,
		"alte Zeit statt Hoehe":       `{"pq_stempel":{"ab_unix":1800000000}}`,
		"unbekanntes Feld":            `{"pq_stempel":{"ab_hoehe":0,"abstand":1}}`,
		"Adresse gross geschrieben":   fmt.Sprintf(`{"pq_stempel":{"ab_hoehe":0,"schluessel":{%q:%q}}}`, strings.ToUpper(stempelErzeuger), pqHex(sk)),
		"Schluessel in falscher Form": fmt.Sprintf(`{"pq_stempel":{"ab_hoehe":0,"schluessel":{%q:"abcd"}}}`, stempelErzeuger),
	}
	for name, roh := range schlecht {
		if _, err := ladePQStempelKonfig([]byte(roh)); err == nil {
			t.Errorf("angenommen: %s", name)
		}
	}
}

func TestPQStempel_GutfallUndFaelschungen(t *testing.T) {
	sk := testPQSchluessel(t)
	k := stempelKonf(60, map[string]*mldsa.PrivateKey{stempelErzeuger: sk})
	nach := int64(1_800_000_000)

	gut := stempelBlock(120, nach, sk)
	if gut.PQStempel == "" {
		t.Fatal("Erzeuger hat auf Hoehe 120 nicht gestempelt")
	}
	if err := pruefePQStempelMit(gut, k); err != nil {
		t.Fatalf("Gutfall abgelehnt: %v", err)
	}
	if b := stempelBlock(121, nach, sk); b.PQStempel != "" {
		t.Fatal("Erzeuger hat ausserhalb der Stempelhoehe gestempelt")
	}
	if err := pruefePQStempelMit(stempelBlock(121, nach, nil), k); err != nil {
		t.Fatalf("Block zwischen den Stempeln abgelehnt: %v", err)
	}

	angriff := func(name string, b *Block) {
		t.Helper()
		if pruefePQStempelMit(b, k) == nil {
			t.Errorf("Angriff ging durch: %s", name)
		}
	}
	angriff("Stempelhoehe ohne Stempel (gebrochenes secp256k1 allein)", stempelBlock(120, nach, nil))
	// F1: Zurueckdatieren hilft nicht -- die Pflicht haengt an der Hoehe.
	angriff("Blockzeit weit zurueckdatiert, ohne Stempel", stempelBlock(120, 1, nil))
	angriff("Stempel eines fremden Schluessels", stempelBlock(120, nach, testPQSchluessel(t)))

	wieder := stempelBlock(180, nach, nil)
	wieder.PQStempel = gut.PQStempel
	angriff("Stempel eines anderen Blocks wiederverwendet", wieder)

	fremderErzeuger := stempelBlock(120, nach, sk)
	fremderErzeuger.Proposer = "0x00000000000000000000000000000000000000e2"
	angriff("Erzeuger ohne eingetragenen Schluessel", fremderErzeuger)

	zwischen := stempelBlock(121, nach, nil)
	zwischen.PQStempel = gut.PQStempel
	angriff("Stempel ausserhalb der Stempelhoehe (Blockgroesse)", zwischen)

	kaputt := stempelBlock(120, nach, sk)
	kaputt.PQStempel = kaputt.PQStempel[:len(kaputt.PQStempel)-2]
	angriff("Stempel abgeschnitten", kaputt)

	gross := stempelBlock(120, nach, sk)
	gross.PQStempel = strings.ToUpper(gross.PQStempel)
	angriff("Stempel gross geschrieben (zweite Schreibweise)", gross)

	gekippt := stempelBlock(120, nach, sk)
	bs := []byte(gekippt.PQStempel)
	if bs[300] == '0' {
		bs[300] = '1'
	} else {
		bs[300] = '0'
	}
	gekippt.PQStempel = string(bs)
	angriff("ein Zeichen des Stempels geaendert", gekippt)

	geaendert := stempelBlock(120, nach, sk)
	geaendert.Hash = fmt.Sprintf("%064x", 1)
	angriff("Block nach dem Stempeln veraendert (anderer Hash)", geaendert)

	// Gross geschriebener Erzeuger im Block ist derselbe Erzeuger.
	gemischt := stempelBlock(120, nach, sk)
	gemischt.Proposer = strings.ToUpper(stempelErzeuger[:2]) + strings.ToUpper(stempelErzeuger[2:])
	if err := pruefePQStempelMit(gemischt, k); err != nil {
		t.Fatalf("Erzeugeradresse in anderer Schreibweise abgelehnt: %v", err)
	}
}

// Vor der Stichhoehe: nichts wird abgewiesen ausser falscher Form; gueltige
// und falsche Stempel werden gezaehlt.
func TestPQStempel_VorDerStichhoehe(t *testing.T) {
	sk := testPQSchluessel(t)
	k := stempelKonf(180, map[string]*mldsa.PrivateKey{stempelErzeuger: sk})
	vor := int64(1_800_000_000) // Zeit spielt keine Rolle; Hoehe 120 < 180

	if err := pruefePQStempelMit(stempelBlock(120, vor, nil), k); err != nil {
		t.Fatalf("vor der Stichhoehe ohne Stempel abgelehnt: %v", err)
	}
	ok0, falsch0 := pqStempelOK.Load(), pqStempelFalsch.Load()
	if err := pruefePQStempelMit(stempelBlock(120, vor, sk), k); err != nil {
		t.Fatal(err)
	}
	if err := pruefePQStempelMit(stempelBlock(120, vor, testPQSchluessel(t)), k); err != nil {
		t.Fatalf("vor der Stichhoehe falscher Stempel abgelehnt (soll nur gezaehlt werden): %v", err)
	}
	if pqStempelOK.Load()-ok0 != 1 || pqStempelFalsch.Load()-falsch0 != 1 {
		t.Fatalf("Zaehler: ok +%d, falsch +%d", pqStempelOK.Load()-ok0, pqStempelFalsch.Load()-falsch0)
	}
	falscheForm := stempelBlock(121, vor, nil)
	falscheForm.PQStempel = strings.Repeat("ab", pqSignaturHex/2)
	if pruefePQStempelMit(falscheForm, k) == nil {
		t.Fatal("vor der Stichhoehe Stempel ausserhalb der Stempelhoehe angenommen")
	}
	// Schlafend (ohne Eintrag in der Genesis): Form gilt trotzdem.
	if err := pruefePQStempelMit(stempelBlock(600, vor, nil), pqStempelKonfig{}); err != nil {
		t.Fatalf("schlafend abgelehnt: %v", err)
	}
}

// Der Stempel aendert den Blockhash nicht und reist im JSON mit.
func TestPQStempel_HashUndJSON(t *testing.T) {
	dag := newOrphanTestDAG()
	b := &Block{Height: 60, Timestamp: 1_800_000_000, Proposer: stempelErzeuger, ParentHashes: []string{strings.Repeat("a", 64)}}
	h1 := dag.calculateHash(b)
	b.PQStempel = strings.Repeat("ab", pqSignaturHex/2)
	if h2 := dag.calculateHash(b); h1 != h2 {
		t.Fatal("PQ-Stempel veraendert den Blockhash")
	}
	roh, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var zurueck Block
	if err := json.Unmarshal(roh, &zurueck); err != nil || zurueck.PQStempel != b.PQStempel {
		t.Fatal("PQ-Stempel geht im JSON verloren")
	}
	b.PQStempel = ""
	roh, _ = json.Marshal(b)
	if strings.Contains(string(roh), "pq_stempel") {
		t.Fatal("leeres Feld wird mitgeschickt (omitempty)")
	}
}

// Die Datenbank behaelt den Stempel auf jedem Leseweg, ueber den Bloecke an
// Peers gehen -- sonst nimmt ab dem Stichtag kein Nachholender sie an.
func TestPQStempel_DatenbankBehaeltIhn_RealDB(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-pq-stempel-realdb.json")
	if !cs.useDB {
		t.Fatal("keine Datenbank")
	}
	hoehe := (int64(9_000_000+time.Now().UnixNano()%1_000_000) / pqStempelAbstand) * pqStempelAbstand
	b := stempelBlock(hoehe, time.Now().Unix(), testPQSchluessel(t))
	b.Hash = fmt.Sprintf("%064x", time.Now().UnixNano())
	b.PQStempel = strings.Repeat("cd", pqSignaturHex/2)
	t.Cleanup(func() { _ = cs.DeleteBlockFromDB(b.Hash) })
	if err := cs.SaveBlockToDB(b, false); err != nil {
		t.Fatalf("SaveBlockToDB: %v", err)
	}
	if err := cs.MarkBlockReplayed(context.Background(), b.Hash); err != nil {
		t.Fatal(err)
	}
	wege := map[string]*Block{
		"LoadBlockFromDBByHash":   cs.LoadBlockFromDBByHash(b.Hash),
		"LoadBlockFromDBByHeight": cs.LoadBlockFromDBByHeight(hoehe),
	}
	if l, err := cs.LoadBlocksByHashesFromDB([]string{b.Hash}); err == nil && len(l) == 1 {
		wege["LoadBlocksByHashesFromDB"] = l[0]
	} else {
		t.Fatalf("LoadBlocksByHashesFromDB: %v", err)
	}
	if l, err := cs.LoadNachgespielteBlocksSinceFromDB(hoehe-1, "", 10); err == nil {
		for _, x := range l {
			if x.Hash == b.Hash {
				wege["LoadNachgespielteBlocksSinceFromDB"] = x
			}
		}
	}
	if m, err := cs.LoadBlocksFromDB(hoehe - 1); err == nil {
		wege["LoadBlocksFromDB"] = m[b.Hash]
	}
	for name, x := range wege {
		if x == nil {
			t.Errorf("%s: Block nicht gefunden", name)
			continue
		}
		if x.PQStempel != b.PQStempel {
			t.Errorf("%s: Stempel verloren", name)
		}
	}
}
