package keeper

import (
	"database/sql"
	"os"
	"testing"
	"time"
)

// Echte Kundschaft und Grundeinkommen gegen echtes Postgres (wegwerfbar).
// Gutfall und alles, was NICHT zaehlen darf: dieselbe Person zweimal, eigene
// Verantwortliche, Nicht-Menschen, Kleinstbetraege, alte Zahlungen.
func TestKundschaftUndGrundeinkommen_RealDB(t *testing.T) {
	// Echtes Schema, nur Zeilen leeren: Frueher loeschte dieser Test
	// chain_accounts und legte eine Minimalfassung an -- danach fehlte die
	// Tabelle jedem spaeteren DB-Test im selben Lauf.
	truncateDistTestTables(t)
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() }) // nach leeren (Cleanups laufen rueckwaerts)
	cs := &ChainState{db: db, useDB: true}
	cs.ensureKontoVerlaufTable()
	leeren := func() {
		if _, err := db.Exec(`DELETE FROM chain_konto_verlauf`); err != nil {
			t.Fatal(err)
		}
	}
	leeren()
	t.Cleanup(leeren)

	wirtschaftAn(t)
	uhr(t, time.Now().Unix())
	for _, m := range []string{wMensch1, wMensch2, wMensch3} {
		if _, err := db.Exec(`INSERT INTO chain_accounts (address, is_human) VALUES ($1, true)`, m); err != nil {
			t.Fatal(err)
		}
	}
	cs.wirt().unternehmen[wFirmaA] = &unternehmenEintrag{Adresse: wFirmaA, Name: "Laden", Kategorie: "handel",
		Verantwortliche: []string{wMensch1}, EroeffnetAm: nowUnix()}
	jetzt := time.Now().Unix()
	zeile := func(h int, adresse, art, gegen string, seite int, betrag float64, zeit int64) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO chain_konto_verlauf (adresse, block_height, tx_index, seite, art, gegenpartei, betrag, zeit)
			VALUES ($1,$2,0,$3,$4,$5,$6,$7)`, adresse, h, seite, art, gegen, betrag, zeit); err != nil {
			t.Fatal(err)
		}
	}
	zeile(1, wFirmaA, "transfer", wMensch2, 1, 10, jetzt-100)                                     // zaehlt
	zeile(2, wFirmaA, "transfer", wMensch2, 1, 10, jetzt-90)                                      // dieselbe Person: einmal
	zeile(3, wFirmaA, "transfer", wMensch3, 1, 5, jetzt-80)                                       // zaehlt
	zeile(4, wFirmaA, "transfer", wMensch1, 1, 50, jetzt-70)                                      // eigene Verantwortliche: nicht
	zeile(5, wFirmaA, "transfer", wFrei, 1, 50, jetzt-60)                                         // kein Mensch: nicht
	zeile(6, wFirmaA, "transfer", "0xa1000000000000000000000000000000000000ff", 1, 0.5, jetzt-50) // Kleinstbetrag (und kein Mensch)
	zeile(7, wFirmaA, "transfer", wMensch3, 0, 5, jetzt-40)                                       // Ausgang der Firma: nicht
	zeile(8, wFirmaB, "transfer", wMensch2, 1, 10, jetzt-30)                                      // anderes Konto
	zeile(9, wFirmaA, "transfer", wMensch2, 1, 10, jetzt-100*86400)                               // aelter als 90 Tage
	zeile(11, wFirmaA, "swap_aeq_tusd", "", 0, 20, jetzt-10)                                      // Ausstieg
	zeile(12, wFirmaA, "swap_aeq_tusd", "", 0, 999, jetzt-100*86400)                              // Ausstieg, zu alt

	// Grundeinkommen: zwei Runden fuer Mensch2 und Mensch3, eine fuer den Neuen.
	zeile(20, wMensch2, "ubi_distribution", "", 0, 0.3, jetzt-20*86400)
	zeile(21, wMensch2, "ubi_distribution", "", 0, 0.2, jetzt-86400)
	zeile(20, wMensch3, "ubi_distribution", "", 0, 0.3, jetzt-20*86400)
	zeile(21, wMensch3, "ubi_distribution", "", 0, 0.2, jetzt-86400)
	zeile(21, wMensch1, "ubi_distribution", "", 0, 0.2, jetzt-86400)
	zeile(10, wMensch2, "ubi_distribution", "", 0, 9, jetzt-40*86400) // ausserhalb der 30 Tage

	kundschaftCacheLeeren()
	t.Cleanup(kundschaftCacheLeeren)
	je, ubi, ok := cs.kundschaftUndGrundeinkommen()
	if n := je[wFirmaA]; n != 2 {
		t.Fatalf("Kundschaft A: 2 verschiedene Menschen erwartet, bekommen %d (%v)", n, je)
	}
	if !ok || !fast(ubi, 0.5) {
		t.Fatalf("Grundeinkommen 30 Tage: 0,5 erwartet, bekommen %v (ok=%v)", ubi, ok)
	}

	// Weitergabe: Eingang 125,5 (alles an A in 90 Tagen, auch von Nicht-Menschen),
	// weitergegeben 5, getauscht 20.
	g, ok := weitergabeVon(wFirmaA)
	if !ok || !fast(g.Ein, 125.5) || !fast(g.Weiter, 5) || !fast(g.Ausstieg, 20) {
		t.Fatalf("Weitergabe A: %+v (ok=%v)", g, ok)
	}
	q := g.quoten()
	if !fast(q["weitergabe"].(float64), round6(5/125.5)) || !fast(q["ausstieg"].(float64), round6(20/125.5)) {
		t.Fatalf("Quoten: %v", q)
	}
	if leer := (weitergabe{}).quoten(); leer["weitergabe"] != nil {
		t.Fatalf("ohne Eingang keine Quote, nicht 0: %v", leer)
	}
	if mehr := (weitergabe{Ein: 10, Weiter: 30}).quoten(); mehr["weitergabe"].(float64) != 1 {
		t.Fatalf("Weitergabe aus frueherem Guthaben wird auf 1 begrenzt: %v", mehr)
	}
}

// Ein frischer Knoten (Beobachter, neuer Validator) hat die Verlaufstabelle
// noch nicht -- sie entsteht beim ersten Schreiben. Lesen davor scheiterte mit
// "relation chain_konto_verlauf does not exist" (Beobachter-Lauf 03.10.).
func TestGrundeinkommen30_OhneVerlaufstabelle_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`DROP TABLE IF EXISTS chain_konto_verlauf`); err != nil {
		t.Fatal(err)
	}
	cs := &ChainState{db: db, useDB: true} // frisch: kontoVerlaufOnce nicht gelaufen
	if _, err := cs.rechneGrundeinkommen30(time.Now()); err != nil {
		t.Fatalf("Lesen ohne Tabelle: %v", err)
	}
}
