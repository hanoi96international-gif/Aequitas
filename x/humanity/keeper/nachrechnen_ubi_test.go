package keeper

import (
	"fmt"
	"math"
	"testing"
)

var ubiRegeln = []string{"ubi_kein_mensch", "ubi_doppelt", "ubi_ungleich", "ubi_empfaenger", "ubi_anteil", "demurrage_nach_umstellung"}

// ubiZaehler: Stand aller Regeln dieser Datei.
func ubiZaehler() map[string]int64 {
	m := map[string]int64{}
	for _, r := range ubiRegeln {
		m[r] = erhaltungZaehler(r)
	}
	return m
}

// ubiNeu: welche Regeln seit vorher angeschlagen haben, und wie oft.
func ubiNeu(vorher map[string]int64) map[string]int64 {
	m := map[string]int64{}
	for _, r := range ubiRegeln {
		if d := erhaltungZaehler(r) - vorher[r]; d != 0 {
			m[r] = d
		}
	}
	return m
}

// ubiRundeVomErzeuger: eine echte Tagesrunde, wie distributeUBIPoolLocked sie
// erzeugt, fuer drei Menschen und einen Topf.
func ubiRundeVomErzeuger(t *testing.T, topf float64, anlegen func(*ChainState)) ([]Transaction, int64) {
	t.Helper()
	erzeuger := newTestState()
	if anlegen != nil {
		anlegen(erzeuger)
	} else {
		for _, a := range erhaltungMenschen {
			erzeuger.accounts.Set(a, &AccountState{Address: a, IsHuman: true, Balance: NewDecimal(100)})
		}
		erzeuger.humanCount = int64(len(erhaltungMenschen))
		erzeuger.accounts.Set(ubiPoolAddr, &AccountState{Address: ubiPoolAddr, Balance: NewDecimal(topf)})
	}
	var txs []Transaction
	erzeuger.ausgangOhneDB = func(t Transaction) { txs = append(txs, t) }
	at := nowUnix()
	if err := erzeuger.RunDailyDistributionAtomic(at); err != nil {
		t.Fatal(err)
	}
	var ubi int
	for _, tx := range txs {
		if tx.Type == "ubi_distribution" {
			ubi++
		}
	}
	if ubi != len(erhaltungMenschen) {
		t.Fatalf("Vorbedingung: erwartet %d Gutschriften, bekommen %+v", len(erhaltungMenschen), txs)
	}
	return txs, at
}

// Gutfall: die echte Runde, in einem Block, ueber zwei Bloecke und Gutschrift
// fuer Gutschrift -- keine Regel schlaegt an.
func TestNachrechnenUBI_EchteRundeOhneAbweichung(t *testing.T) {
	txs, at := ubiRundeVomErzeuger(t, 100.000007, nil)
	for _, teilung := range []int{len(txs), 1, 2, 3} {
		vorher := ubiZaehler()
		dag, cs := erhaltungKnoten(t, 100.000007)
		if !dag.replayTransactions(erhaltungBlock(1, at+5, txs[:teilung]...), true) {
			t.Fatal("erster Block abgelehnt")
		}
		if teilung < len(txs) && !dag.replayTransactions(erhaltungBlock(2, at+6, txs[teilung:]...), true) {
			t.Fatal("zweiter Block abgelehnt")
		}
		if neu := ubiNeu(vorher); len(neu) != 0 {
			t.Errorf("Teilung nach %d: ehrliche Runde meldet %v", teilung, neu)
		}
		if cs.ubiRunde.aktiv {
			t.Errorf("Teilung nach %d: Runde nach der Marke noch offen", teilung)
		}
	}
}

// Gutfall mit Demurrage: der Erzeuger rechnet sie vorab in den Topf, beim
// Nachspielen kommt sie mit den Gutschriften -- der Anteil stimmt trotzdem.
func TestNachrechnenUBI_EchteRundeMitDemurrage(t *testing.T) {
	ruhig := nowUnix() - 400*86400
	anlegen := func(cs *ChainState) {
		for i, a := range erhaltungMenschen {
			acc := &AccountState{Address: a, IsHuman: true, Balance: NewDecimal(100)}
			if i == len(erhaltungMenschen)-1 {
				acc.Balance, acc.LastActivityAt = NewDecimal(5000), ruhig
			}
			cs.accounts.Set(a, acc)
		}
		cs.humanCount = int64(len(erhaltungMenschen))
		cs.accounts.Set(ubiPoolAddr, &AccountState{Address: ubiPoolAddr, Balance: NewDecimal(1)})
	}
	txs, at := ubiRundeVomErzeuger(t, 0, anlegen)
	var mit bool
	for _, tx := range txs {
		mit = mit || tx.FromDemurrageLost > 0
	}
	if !mit {
		t.Fatal("Vorbedingung: die Runde muss Demurrage tragen")
	}
	vorher := ubiZaehler()
	dag, cs := nachspielKnoten(t, nil)
	cs.mu.Lock()
	anlegen(cs)
	cs.mu.Unlock()
	if !dag.replayTransactions(erhaltungBlock(1, at+5, txs...), true) {
		t.Fatal("Block abgelehnt")
	}
	if neu := ubiNeu(vorher); len(neu) != 0 {
		t.Errorf("ehrliche Runde mit Demurrage meldet %v", neu)
	}
}

// ubiAngriff spielt eine Runde mit den gegebenen Gutschriften nach (Topf 90,
// drei Menschen, ehrlich waeren 30 je Mensch) und liefert die angeschlagenen
// Regeln.
func ubiAngriff(t *testing.T, gutschriften ...Transaction) map[string]int64 {
	t.Helper()
	dag, cs := erhaltungKnoten(t, 90)
	cs.mu.Lock()
	cs.accounts.Set("0xb200000000000000000000000000000000000f01",
		&AccountState{Address: "0xb200000000000000000000000000000000000f01", Balance: NewDecimal(1)})
	cs.mu.Unlock()
	at := nowUnix()
	vorher := ubiZaehler()
	txs := append(gutschriften,
		Transaction{Type: "ubi_distribution_finalize", DistributionAt: at, Amount: 0},
		Transaction{Type: "distribution_round_marker", DistributionAt: at})
	dag.replayTransactions(erhaltungBlock(1, at, txs...), true)
	return ubiNeu(vorher)
}

func ubiAn(a string, betrag float64) Transaction {
	return Transaction{Type: "ubi_distribution", Wallet: a, Amount: betrag}
}

func TestNachrechnenUBI_EhrlicheRundeAlsVergleich(t *testing.T) {
	if neu := ubiAngriff(t, ubiAn(erhaltungMenschen[0], 30), ubiAn(erhaltungMenschen[1], 30), ubiAn(erhaltungMenschen[2], 30)); len(neu) != 0 {
		t.Fatalf("30/30/30 aus 90 fuer drei Menschen meldet %v", neu)
	}
}

// Missbrauch: der ganze Topf an einen Menschen, in drei Gutschriften.
// Erhaltung sieht nichts (90 aus 90).
func TestNachrechnenUBI_AllesAnEinen(t *testing.T) {
	a := erhaltungMenschen[0]
	neu := ubiAngriff(t, ubiAn(a, 30), ubiAn(a, 30), ubiAn(a, 30))
	if neu["ubi_doppelt"] != 2 {
		t.Fatalf("zweimal doppelt erwartet, gemeldet %v", neu)
	}
}

// Missbrauch: ein Mensch wird durch eine Wallet ersetzt, die keiner ist.
func TestNachrechnenUBI_KeinMensch(t *testing.T) {
	neu := ubiAngriff(t, ubiAn(erhaltungMenschen[0], 30), ubiAn(erhaltungMenschen[1], 30),
		ubiAn("0xb200000000000000000000000000000000000f01", 30))
	if neu["ubi_kein_mensch"] != 1 {
		t.Fatalf("kein_mensch erwartet, gemeldet %v", neu)
	}
}

// Missbrauch: ungleiche Betraege bei richtiger Summe.
func TestNachrechnenUBI_Ungleich(t *testing.T) {
	neu := ubiAngriff(t, ubiAn(erhaltungMenschen[0], 10), ubiAn(erhaltungMenschen[1], 10), ubiAn(erhaltungMenschen[2], 70))
	if neu["ubi_ungleich"] != 1 {
		t.Fatalf("ungleich erwartet, gemeldet %v", neu)
	}
}

// Missbrauch: ein Mensch wird ausgelassen, die anderen bekommen seinen Teil
// nicht -- der Rest bleibt fuer den Produzenten im Topf.
func TestNachrechnenUBI_Ausgelassen(t *testing.T) {
	neu := ubiAngriff(t, ubiAn(erhaltungMenschen[0], 30), ubiAn(erhaltungMenschen[1], 30))
	if neu["ubi_empfaenger"] != 1 {
		t.Fatalf("empfaenger erwartet, gemeldet %v", neu)
	}
}

// Missbrauch: alle gleich, aber zu wenig.
func TestNachrechnenUBI_ZuWenig(t *testing.T) {
	neu := ubiAngriff(t, ubiAn(erhaltungMenschen[0], 1), ubiAn(erhaltungMenschen[1], 1), ubiAn(erhaltungMenschen[2], 1))
	if neu["ubi_anteil"] != 1 {
		t.Fatalf("anteil erwartet, gemeldet %v", neu)
	}
}

// Ein Mikro Rundung ist kein Angriff.
func TestNachrechnenUBI_EinMikroToleranz(t *testing.T) {
	neu := ubiAngriff(t, ubiAn(erhaltungMenschen[0], 29.999999), ubiAn(erhaltungMenschen[1], 29.999999), ubiAn(erhaltungMenschen[2], 29.999999))
	if len(neu) != 0 {
		t.Fatalf("ein Mikro unter dem Anteil meldet %v", neu)
	}
}

// Neustart mitten in der Runde: der Anfang fehlt. Zahl und Anteil werden
// nicht geprueft (sonst meldete eine ehrliche Runde "zu wenig Empfaenger");
// die naechste Runde wird wieder vollstaendig geprueft.
func TestNachrechnenUBI_NeustartMittenInDerRunde(t *testing.T) {
	txs, at := ubiRundeVomErzeuger(t, 90, nil)
	dag, cs := erhaltungKnoten(t, 90)
	cs.mu.Lock()
	cs.ubiRunde.unsicher = true // wie nach loadFromDB
	cs.mu.Unlock()
	vorher := ubiZaehler()
	// Die erste Gutschrift lag vor dem Neustart.
	if !dag.replayTransactions(erhaltungBlock(1, at+5, txs[1:]...), true) {
		t.Fatal("Block abgelehnt")
	}
	if neu := ubiNeu(vorher); len(neu) != 0 {
		t.Fatalf("ehrliche Runde nach Neustart meldet %v", neu)
	}
	if cs.ubiRunde.unsicher {
		t.Fatal("nach der Rundengrenze muss der Anfang wieder beobachtet sein")
	}
	// Naechste Runde, wieder mit Auslassung: jetzt geprueft.
	cs.mu.Lock()
	cs.accounts.Set(ubiPoolAddr, &AccountState{Address: ubiPoolAddr, Balance: NewDecimal(90)})
	cs.mu.Unlock()
	dag.replayTransactions(erhaltungBlock(2, at+86400,
		ubiAn(erhaltungMenschen[0], 30), ubiAn(erhaltungMenschen[1], 30),
		Transaction{Type: "ubi_distribution_finalize", DistributionAt: at + 86400}), true)
	if ubiNeu(vorher)["ubi_empfaenger"] != 1 {
		t.Fatalf("Auslassung in der Runde nach dem Neustart nicht erkannt: %v", ubiNeu(vorher))
	}
}

// Ein zurueckgewiesener Block darf seine Empfaenger nicht in der Runde
// zuruecklassen -- sonst meldete der ehrliche Block danach "doppelt".
func TestNachrechnenUBI_RueckrollenEntferntEmpfaenger(t *testing.T) {
	dag, cs := erhaltungKnoten(t, 90)
	at := nowUnix()
	if !dag.replayTransactions(erhaltungBlock(1, at, ubiAn(erhaltungMenschen[0], 30)), true) {
		t.Fatal("erster Block abgelehnt")
	}
	if dag.replayTransactions(erhaltungBlock(2, at,
		ubiAn(erhaltungMenschen[1], 30),
		// Ohne Wallet: das Nachspielen lehnt den Block ab, NACHDEM die
		// Gutschrift davor schon in der Runde steht.
		Transaction{Type: "ubi_distribution", Amount: 30},
	), true) {
		t.Fatal("Vorbedingung: der Block muss abgelehnt werden")
	}
	if cs.ubiRunde.n != 1 || len(cs.ubiRunde.empfaenger) != 1 {
		t.Fatalf("nach dem Zurueckrollen: n=%d, %d Empfaenger -- erwartet 1/1", cs.ubiRunde.n, len(cs.ubiRunde.empfaenger))
	}
	vorher := ubiZaehler()
	if !dag.replayTransactions(erhaltungBlock(3, at,
		ubiAn(erhaltungMenschen[1], 30), ubiAn(erhaltungMenschen[2], 30),
		Transaction{Type: "ubi_distribution_finalize", DistributionAt: at}), true) {
		t.Fatal("ehrlicher Block abgelehnt")
	}
	if neu := ubiNeu(vorher); len(neu) != 0 {
		t.Fatalf("ehrlicher Rest der Runde nach Zurueckrollen meldet %v", neu)
	}
}

// Seit der Umlaufsicherung gibt es keine Demurrage: eine erfundene nimmt
// einem Konto Geld und gibt es dem Topf. Vorher (Override aus) gilt sie.
func TestNachrechnenUBI_DemurrageNachUmstellung(t *testing.T) {
	zeit := int64(1_800_000_000)
	tx := Transaction{Type: "transfer", Wallet: erhaltungMenschen[0], To: erhaltungMenschen[1], Amount: 1, FromDemurrageLost: 5}
	cs := nachrechnenTestState()
	pruefeZu := func() bool {
		vorher := erhaltungZaehler("demurrage_nach_umstellung")
		cs.mu.Lock()
		err := cs.nachrechnenTxLocked(&tx, zeit)
		cs.mu.Unlock()
		if err != nil {
			t.Fatalf("Beobachtungsmodus darf nicht ablehnen: %v", err)
		}
		return erhaltungZaehler("demurrage_nach_umstellung") > vorher
	}
	if pruefeZu() {
		t.Fatal("vor der Umstellung ist Demurrage erlaubt")
	}
	wirtschaftAktivOverride.Store(zeit - demurrageUmstellungPuffer + 1)
	t.Cleanup(func() { wirtschaftAktivOverride.Store(math.MaxInt64) })
	if pruefeZu() {
		t.Fatal("innerhalb des Puffers nach der Umstellung darf nichts anschlagen")
	}
	wirtschaftAktivOverride.Store(zeit - demurrageUmstellungPuffer)
	if !pruefeZu() {
		t.Fatal("erfundene Demurrage nach der Umstellung nicht erkannt")
	}
	tx.FromDemurrageLost, tx.ToDemurrageLost = 0, 3
	if !pruefeZu() {
		t.Fatal("erfundene Demurrage beim Empfaenger nicht erkannt")
	}
	tx.ToDemurrageLost = 0
	if pruefeZu() {
		t.Fatal("ohne Demurrage darf nichts anschlagen")
	}
}

// Nach der Umstellung erzeugt der Erzeuger selbst keine Demurrage mehr --
// eine echte Runde bleibt ohne Meldung, auch mit lange ruhendem Konto.
func TestNachrechnenUBI_EchteRundeNachUmstellung(t *testing.T) {
	wirtschaftAktivOverride.Store(1)
	t.Cleanup(func() { wirtschaftAktivOverride.Store(math.MaxInt64) })
	ruhig := nowUnix() - 400*86400
	anlegen := func(cs *ChainState) {
		for i, a := range erhaltungMenschen {
			acc := &AccountState{Address: a, IsHuman: true, Balance: NewDecimal(100)}
			if i == 0 {
				acc.Balance, acc.LastActivityAt = NewDecimal(4000), ruhig
			}
			cs.accounts.Set(a, acc)
		}
		cs.humanCount = int64(len(erhaltungMenschen))
		cs.accounts.Set(ubiPoolAddr, &AccountState{Address: ubiPoolAddr, Balance: NewDecimal(60)})
	}
	txs, at := ubiRundeVomErzeuger(t, 0, anlegen)
	vorher := ubiZaehler()
	dag, cs := nachspielKnoten(t, nil)
	cs.mu.Lock()
	anlegen(cs)
	cs.mu.Unlock()
	if !dag.replayTransactions(erhaltungBlock(1, at+5, txs...), true) {
		t.Fatal("Block abgelehnt")
	}
	if neu := ubiNeu(vorher); len(neu) != 0 {
		t.Errorf("echte Runde nach der Umstellung meldet %v", neu)
	}
}

// Missbrauch: ein Produzent haelt eine Runde offen (kein Abschluss, keine
// Marke) und schreibt Block fuer Block erfundene Wallets hinein. Die
// Empfaengermengen wachsen dabei nicht -- sie enthalten nur Menschen bzw.
// LP-Halter, also hoechstens so viele wie der Zustand hat.
func TestNachrechnen_OffeneRundeWaechstNicht(t *testing.T) {
	wirtschaftAktivOverride.Store(1)
	t.Cleanup(func() { wirtschaftAktivOverride.Store(math.MaxInt64) })
	dag, cs := nachspielKnoten(t, nil)
	cs.mu.Lock()
	lpAnlegen(cs)
	cs.accounts.Set(ubiPoolAddr, &AccountState{Address: ubiPoolAddr, Balance: NewDecimal(90)})
	cs.mu.Unlock()
	at := nowUnix()
	for blk := 1; blk <= 5; blk++ {
		var txs []Transaction
		cs.mu.Lock()
		for i := 0; i < 20; i++ {
			// Bestehende Konten, keine Menschen, keine Halter: der Block
			// wird angewendet (sonst raeumte schon das Zurueckrollen auf).
			w := fmt.Sprintf("0xc3%038d", blk*1000+i)
			cs.accounts.Set(w, &AccountState{Address: w, Balance: NewDecimal(1)})
			txs = append(txs, ubiAn(w, 0.000001), lpAn(w, 0.000001))
		}
		cs.mu.Unlock()
		if !dag.replayTransactions(erhaltungBlock(blk, at, txs...), true) {
			t.Fatalf("Block %d abgelehnt -- der Test braucht angewendete Bloecke", blk)
		}
	}
	if n := len(cs.ubiRunde.empfaenger); n != 0 {
		t.Errorf("Grundeinkommen: %d erfundene Wallets in der Empfaengermenge", n)
	}
	if n := len(cs.lpRunde.bedacht); n != 0 {
		t.Errorf("LP: %d erfundene Wallets in der Menge der Bedachten", n)
	}
}
