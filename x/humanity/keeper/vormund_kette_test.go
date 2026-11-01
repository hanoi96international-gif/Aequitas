package keeper

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// Unterschriften und Zeitfenster -- ohne Datenbank, ueber denselben Weg, den
// jeder Nachspielende geht (pruefeAuftragsNachweis).
func TestVormundNachweis_Unterschriften(t *testing.T) {
	schuetzlingKey, _ := crypto.GenerateKey()
	vormundKey, _ := crypto.GenerateKey()
	fremdKey, _ := crypto.GenerateKey()
	ward, vormund := adrVon(schuetzlingKey), adrVon(vormundKey)
	zeit := int64(1_800_000_000)

	setzen := func(sig string, z int64) *Transaction {
		return &Transaction{Type: "vormund_setzen", Wallet: ward, To: vormund, Nachweis: &Auftragsnachweis{Sig: sig, Zeit: z}}
	}
	leben := func(sig string, z int64) *Transaction {
		return &Transaction{Type: "lebenszeichen", Wallet: ward, To: vormund, Nachweis: &Auftragsnachweis{Sig: sig, Zeit: z}}
	}

	// Gutfall.
	if err := pruefeAuftragsNachweis(setzen(personalSign(t, schuetzlingKey, vormundSetzenNachricht(vormund, zeit)), zeit), zeit+60); err != nil {
		t.Fatalf("gueltiges vormund_setzen abgelehnt: %v", err)
	}
	if err := pruefeAuftragsNachweis(leben(personalSign(t, vormundKey, lebenszeichenNachricht(ward, zeit)), zeit), zeit+60); err != nil {
		t.Fatalf("gueltiges lebenszeichen abgelehnt: %v", err)
	}

	faelle := []struct {
		name string
		tx   *Transaction
		blk  int64
	}{
		{"vormund unterschreibt seine eigene Einsetzung", setzen(personalSign(t, vormundKey, vormundSetzenNachricht(vormund, zeit)), zeit), zeit},
		{"Fremder setzt einen Vormund", setzen(personalSign(t, fremdKey, vormundSetzenNachricht(vormund, zeit)), zeit), zeit},
		{"altes Format ohne Zeitpunkt (setzen)", setzen(personalSign(t, schuetzlingKey, "Aequitas: set guardian "+vormund), zeit), zeit},
		{"Schuetzling bestaetigt sich selbst", leben(personalSign(t, schuetzlingKey, lebenszeichenNachricht(ward, zeit)), zeit), zeit},
		{"Fremder bestaetigt", leben(personalSign(t, fremdKey, lebenszeichenNachricht(ward, zeit)), zeit), zeit},
		{"altes Format ohne Zeitpunkt (leben)", leben(personalSign(t, vormundKey, "Aequitas: confirm alive "+ward), zeit), zeit},
		{"Wiederverwendung nach ueber einer Stunde", leben(personalSign(t, vormundKey, lebenszeichenNachricht(ward, zeit)), zeit), zeit + 3601},
		{"Zeitpunkt in der Zukunft", leben(personalSign(t, vormundKey, lebenszeichenNachricht(ward, zeit+600)), zeit+600), zeit},
		{"unterschriebener und behaupteter Zeitpunkt verschieden", leben(personalSign(t, vormundKey, lebenszeichenNachricht(ward, zeit)), zeit+100), zeit + 100},
	}
	for _, f := range faelle {
		if err := pruefeAuftragsNachweis(f.tx, f.blk); err == nil {
			t.Errorf("%s: angenommen", f.name)
		}
	}
	// Ohne Nachweis.
	if err := pruefeAuftragsNachweis(&Transaction{Type: "lebenszeichen", Wallet: ward, To: vormund}, zeit); err == nil {
		t.Error("lebenszeichen ohne Nachweis angenommen")
	}
	if !braucheNachweis("vormund_setzen") || !braucheNachweis("lebenszeichen") {
		t.Error("beide Arten muessen im Block ihren Nachweis tragen")
	}
}

// Gegen eine echte Datenbank: Annehmen, Regeln, Nachspielen.
func TestVormundKette_RealDB(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	if os.Getenv("DATABASE_URL") == "" {
		t.Fatal("DATABASE_URL must point at a disposable local Postgres database")
	}
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-vormund-kette-db-test.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}

	schuetzlingKey, _ := crypto.GenerateKey()
	vormundKey, _ := crypto.GenerateKey()
	fremdKey, _ := crypto.GenerateKey()
	ward, vormund, fremd := adrVon(schuetzlingKey), adrVon(vormundKey), adrVon(fremdKey)
	for _, a := range []string{ward, vormund, fremd} {
		if _, err := cs.db.Exec(`DELETE FROM guardians WHERE wallet_address = $1 OR guardian_address = $1`, a); err != nil {
			t.Fatal(err)
		}
		cs.mu.Lock()
		acc := &AccountState{Address: a, IsHuman: true, Balance: NewDecimal(100)}
		acc.LastActivityAt = 1
		err := cs.saveAccountToDB(acc)
		cs.mu.Unlock()
		if err != nil {
			t.Fatalf("seed %s: %v", a, err)
		}
	}
	jetzt := time.Now().Unix()
	setzen := func(k string, z int64) *Auftragsnachweis {
		return &Auftragsnachweis{Sig: personalSign(t, schuetzlingKey, vormundSetzenNachricht(k, z)), Zeit: z}
	}

	// 1. Einsetzen: Zeitpunkt der Eintragung ist der unterschriebene.
	if err := cs.VormundSetzen(ward, vormund, setzen(vormund, jetzt)); err != nil {
		t.Fatalf("VormundSetzen: %v", err)
	}
	if g, setAt, err := cs.GetGuardian(ward); err != nil || strings.ToLower(g) != vormund || setAt != jetzt {
		t.Fatalf("Eintrag: %s %d %v", g, setAt, err)
	}

	// 2. Sieben-Tage-Frist: ein neuer Vormund sofort danach wird abgelehnt.
	if err := cs.VormundSetzen(ward, fremd, setzen(fremd, jetzt)); err == nil {
		t.Fatal("Vormundwechsel innerhalb der Frist angenommen")
	}

	// 3. Missbrauch: ein Fremder bestaetigt mit eigener, gueltiger
	//    Unterschrift -- er ist nicht der Vormund.
	fremdNachweis := &Auftragsnachweis{Sig: personalSign(t, fremdKey, lebenszeichenNachricht(ward, jetzt)), Zeit: jetzt}
	if err := cs.Lebenszeichen(ward, fremd, fremdNachweis); err == nil {
		t.Fatal("Lebenszeichen eines Fremden angenommen")
	}
	cs.mu.Lock()
	acc, _ := cs.accounts.Get(ward)
	vorher := acc.LastActivityAt
	cs.mu.Unlock()
	if vorher > 1 {
		t.Fatalf("Aktivitaet durch einen Fremden veraendert: %d", vorher)
	}

	// 4. Der Vormund bestaetigt.
	echt := &Auftragsnachweis{Sig: personalSign(t, vormundKey, lebenszeichenNachricht(ward, jetzt)), Zeit: jetzt}
	if err := cs.Lebenszeichen(ward, vormund, echt); err != nil {
		t.Fatalf("Lebenszeichen des Vormunds: %v", err)
	}
	cs.mu.Lock()
	acc, _ = cs.accounts.Get(ward)
	nachher := acc.LastActivityAt
	cs.mu.Unlock()
	if nachher <= 1 {
		t.Fatal("Lebenszeichen hat die Aktivitaet nicht zurueckgesetzt")
	}

	// 5. Nachspielen: die Regeln gegen die eigene Tabelle, ueber den
	//    Eingang des Nachspielens (nachrechnenTxLocked).
	zaehle := func(regel string, tx Transaction) int64 {
		vor := erhaltungZaehler(regel)
		cs.mu.Lock()
		err := cs.nachrechnenTxLocked(&tx, jetzt+60)
		cs.mu.Unlock()
		if err != nil {
			t.Fatalf("im Beobachtungsmodus darf nichts abgelehnt werden: %v", err)
		}
		return erhaltungZaehler(regel) - vor
	}
	if d := zaehle("lebenszeichen_fremd", Transaction{Type: "lebenszeichen", Wallet: ward, To: fremd, Nachweis: fremdNachweis}); d != 1 {
		t.Errorf("Lebenszeichen eines Fremden im Block nicht erkannt (%d)", d)
	}
	if d := zaehle("lebenszeichen_fremd", Transaction{Type: "lebenszeichen", Wallet: ward, To: vormund, Nachweis: echt}); d != 0 {
		t.Errorf("echtes Lebenszeichen als Abweichung gezaehlt (%d)", d)
	}
	// Kreis: der Vormund will den Schuetzling zu seinem Vormund machen.
	kreis := &Auftragsnachweis{Sig: "egal", Zeit: jetzt}
	if d := zaehle("vormund_regel", Transaction{Type: "vormund_setzen", Wallet: vormund, To: ward, Nachweis: kreis}); d != 1 {
		t.Errorf("Kreis im Block nicht erkannt (%d)", d)
	}
	// Frist: ein Wechsel im Block innerhalb der sieben Tage.
	if d := zaehle("vormund_regel", Transaction{Type: "vormund_setzen", Wallet: ward, To: fremd, Nachweis: kreis}); d != 1 {
		t.Errorf("Frist im Block nicht geprueft (%d)", d)
	}

	// 6. Nachspielen setzt die Aktivitaet auf die Blockzeit, nicht die Uhr.
	//    (Die Aktivitaet laeuft nie rueckwaerts -- also von "untaetig" aus.)
	blockZeit := jetzt - 3000
	cs.mu.Lock()
	acc, _ = cs.accounts.Get(ward)
	acc.LastActivityAt = 1
	err := cs.applyLebenszeichenLocked(context.Background(), &Transaction{Type: "lebenszeichen", Wallet: ward, To: vormund, Nachweis: echt}, blockZeit)
	acc, _ = cs.accounts.Get(ward)
	gesetzt := acc.LastActivityAt
	cs.mu.Unlock()
	if err != nil || gesetzt != blockZeit {
		t.Fatalf("Nachspielen: Aktivitaet %d, erwartet Blockzeit %d (%v)", gesetzt, blockZeit, err)
	}
}
