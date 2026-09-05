package keeper

import "testing"

// Die Behauptung, wegen der es den Sammler gibt, ist NICHT "er sammelt", es
// ist: die Zahl der geschriebenen Zeilen waechst mit den KONTEN und nicht mit
// den UEBERWEISUNGEN. Ein Test, der nur prueft, dass Konten in einer Liste
// landen, koennte diese Behauptung nicht widerlegen -- also prueft er sie hier
// direkt, indem dieselben Konten immer wieder beruehrt werden.

func TestKontenSammler_ZeilenWachsenMitKontenNichtMitUeberweisungen(t *testing.T) {
	s := neuerKontenSammler()

	// Zwanzig Buendel zu je fuenf Ueberweisungen ueber IMMER DIESELBEN vier
	// Konten -- also 200 Kontenberuehrungen auf vier Adressen. Genau die Lage
	// im Lasttest: wenige hundert Konten, tausende Ueberweisungen.
	a := &AccountState{Address: "0xa"}
	b := &AccountState{Address: "0xb"}
	c := &AccountState{Address: "0xc"}
	d := &AccountState{Address: "0xd"}
	for i := 0; i < 20; i++ {
		s.hinzufuegen(a, b, c, d, a, b, c, d, a, b)
	}

	if got := s.anzahl(); got != 4 {
		t.Fatalf("gesammelte Zeilen = %d, erwartet 4 (eine je Konto, nicht eine je Beruehrung)", got)
	}
	if s.dedupiert != 200-4 {
		t.Errorf("dedupiert = %d, erwartet %d", s.dedupiert, 200-4)
	}
	if s.aufrufe != 20 {
		t.Errorf("aufrufe = %d, erwartet 20 (die zusammengelegten Statements)", s.aufrufe)
	}
}

func TestKontenSammler_JedesKontoGenauEinmal(t *testing.T) {
	// Ein Konto zweimal in der Liste waere nicht nur verschwendet, sondern
	// falsch: saveAccountsToDBBatchCtx sperrt optimistisch ueber acc.Version,
	// und der zweite Eintrag traefe eine Zeile, deren Version der erste
	// bereits erhoeht hat. Das ist der Grund fuer die Dedup, nicht Sparsamkeit.
	s := neuerKontenSammler()
	x := &AccountState{Address: "0xdoppelt"}
	y := &AccountState{Address: "0xeinmal"}
	s.hinzufuegen(x, y)
	s.hinzufuegen(x)
	s.hinzufuegen(x, x, x)

	if got := s.anzahl(); got != 2 {
		t.Fatalf("anzahl = %d, erwartet 2", got)
	}
	gesehen := map[string]int{}
	for _, acc := range s.konten {
		gesehen[acc.Address]++
	}
	for addr, n := range gesehen {
		if n != 1 {
			t.Errorf("%s steht %d-mal in der Liste, erwartet genau einmal", addr, n)
		}
	}
}

func TestKontenSammler_ZeigerNichtKopie(t *testing.T) {
	// Gesammelt werden ZEIGER. Eine Kopie waere still falsch: der
	// Schreibvorgang liest acc.Version und acc.Balance erst am Blockende, und
	// bis dahin koennen weitere Ueberweisungen dasselbe Konto veraendert
	// haben. Eine Kopie wuerde einen veralteten Stand schreiben -- und mit
	// einer veralteten Version auch noch die optimistische Sperre verletzen.
	s := neuerKontenSammler()
	acc := &AccountState{Address: "0xwandelbar", Balance: NewDecimal(10), Version: 3}
	s.hinzufuegen(acc)

	acc.Balance = NewDecimal(99)
	acc.Version = 7

	if s.konten[0].Balance.Float() != 99 {
		t.Errorf("gesammelter Kontostand = %v, erwartet 99 -- es wurde eine Kopie gesammelt, kein Zeiger",
			s.konten[0].Balance.Float())
	}
	if s.konten[0].Version != 7 {
		t.Errorf("gesammelte Version = %d, erwartet 7", s.konten[0].Version)
	}
}

func TestKontenSammler_LeerUndNilSindHarmlos(t *testing.T) {
	var s *kontenSammler
	if s.anzahl() != 0 {
		t.Error("nil-Sammler meldet nicht 0")
	}
	cs := &ChainState{}
	if err := cs.sammlerSchreiben(nil, nil); err != nil {
		t.Errorf("nil-Sammler schreiben ergab %v, erwartet nil", err)
	}
	leer := neuerKontenSammler()
	if err := cs.sammlerSchreiben(nil, leer); err != nil {
		t.Errorf("leerer Sammler schreiben ergab %v, erwartet nil", err)
	}
	// nil-Konten in der Liste duerfen nicht durchrutschen
	leer.hinzufuegen(nil, nil)
	if leer.anzahl() != 0 {
		t.Errorf("nil-Konten wurden gesammelt: %d", leer.anzahl())
	}
}
