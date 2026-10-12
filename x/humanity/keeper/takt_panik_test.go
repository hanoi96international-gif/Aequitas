package keeper

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// Pruefung #337, MITTEL: ein Takt, der bei jedem Lauf paniert, darf das Log
// nicht fluten -- 1.000 Paniken unter einem Label ergeben einen Stack, die
// naechste Meldung nach Ablauf des Abstands nennt die Zahl der uebrigen.
func TestPanik_StackJeLabelGedrosselt(t *testing.T) {
	var aus bytes.Buffer
	altAus, altAbstand := panikAusgabe, panikStackAbstand
	panikZaehlerMu.Lock()
	altZaehler := panikZaehler
	panikZaehler = map[string]*panikStand{}
	panikZaehlerMu.Unlock()
	panikAusgabe, panikStackAbstand = &aus, time.Hour
	t.Cleanup(func() {
		panikAusgabe, panikStackAbstand = altAus, altAbstand
		panikZaehlerMu.Lock()
		panikZaehler = altZaehler
		panikZaehlerMu.Unlock()
	})

	for i := 0; i < 1000; i++ {
		SafeCall("flut", func() { panic("immer wieder") })
	}
	if n := strings.Count(aus.String(), "[PANIC RECOVERED]"); n != 1 {
		t.Fatalf("%d Meldungen fuer 1.000 Paniken, erwartet 1", n)
	}
	if got := PanikStand()["flut"]; got != 1000 {
		t.Fatalf("gezaehlt %d, erwartet 1000", got)
	}
	// Ein anderes Label hat seine eigene erste Meldung.
	SafeCall("anders", func() { panic("x") })
	if n := strings.Count(aus.String(), "[PANIC RECOVERED]"); n != 2 {
		t.Fatalf("%d Meldungen, erwartet 2", n)
	}
	// Nach dem Abstand wieder ein Stack, mit der Zahl der unterdrueckten.
	panikStackAbstand = 0
	aus.Reset()
	SafeCall("flut", func() { panic("immer wieder") })
	if !strings.Contains(aus.String(), "insgesamt 1001, seit der letzten Meldung 999 ohne Stack") {
		t.Fatalf("Zusammenfassung fehlt: %q", firstLine(aus.String()))
	}
}

func TestPanik_LabelTabelleBegrenzt(t *testing.T) {
	var aus bytes.Buffer
	altAus := panikAusgabe
	panikZaehlerMu.Lock()
	altZaehler := panikZaehler
	panikZaehler = map[string]*panikStand{}
	panikZaehlerMu.Unlock()
	panikAusgabe = &aus
	t.Cleanup(func() {
		panikAusgabe = altAus
		panikZaehlerMu.Lock()
		panikZaehler = altZaehler
		panikZaehlerMu.Unlock()
	})
	for i := 0; i < 3*panikLabelsMax; i++ {
		SafeCall(strings.Repeat("l", i%50)+string(rune('a'+i%26))+time.Duration(i).String(), func() { panic("x") })
	}
	if n := len(PanikStand()); n > panikLabelsMax+1 {
		t.Fatalf("%d Labels, Grenze %d", n, panikLabelsMax)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Pruefung #337, MITTEL: ein Peer, dessen Antworten jede Runde zur Panik
// machen, wird nicht mehr jede Sekunde abgefragt -- die Panik zaehlt als
// Fehlschlag, die Wartezeit steigt bis 30 s. Ein Erfolg setzt sie zurueck.
func TestSyncTakt_PanikZaehltAlsFehlschlag(t *testing.T) {
	st := &syncTaktStand{backoff: time.Second}
	var gesetzt []time.Duration
	reset := func(d time.Duration) { gesetzt = append(gesetzt, d) }
	panikZaehlerMu.Lock()
	altAus := panikAusgabe
	panikZaehlerMu.Unlock()
	panikAusgabe = &bytes.Buffer{}
	t.Cleanup(func() { panikAusgabe = altAus })

	for i := 0; i < 8; i++ {
		SafeCall("syncWithNode-test", func() {
			st.runde(func() {}, func() bool { panic("feindliche Antwort") }, reset)
		})
	}
	if st.backoff != 30*time.Second {
		t.Fatalf("Wartezeit nach acht Paniken %s, erwartet 30s", st.backoff)
	}
	if len(gesetzt) == 0 || gesetzt[len(gesetzt)-1] != 30*time.Second {
		t.Fatalf("Ticker nicht verlangsamt: %v", gesetzt)
	}
	st.runde(func() {}, func() bool { return true }, reset)
	if st.backoff != time.Second || gesetzt[len(gesetzt)-1] != time.Second {
		t.Fatalf("Erfolg setzt nicht zurueck: %s %v", st.backoff, gesetzt)
	}
	// Gewoehnlicher Fehlschlag wie vorher.
	st.runde(func() {}, func() bool { return false }, reset)
	if st.backoff != 2*time.Second {
		t.Fatalf("Fehlschlag: %s", st.backoff)
	}
	// Validatoren alle validatorSyncInterval Takte, auch wenn eine Runde paniert.
	st2 := &syncTaktStand{backoff: time.Second}
	val := 0
	for i := 0; i < 2*validatorSyncInterval; i++ {
		SafeCall("syncWithNode-test", func() {
			st2.runde(func() { val++ }, func() bool { panic("x") }, reset)
		})
	}
	if val != 2 {
		t.Fatalf("Validatorabgleich %d mal, erwartet 2", val)
	}
}

// Pruefung #337, NIEDRIG: eine Panik beim Speichern der Pool-Konten kann den
// Zustand halb fortgeschrieben haben -- der Knoten startet neu, statt still
// weiterzulaufen, und die Konten bleiben zum Speichern markiert.
func TestPoolFlush_PanikStartetNeu(t *testing.T) {
	alt := poolFlushNeustart
	code := -1
	poolFlushNeustart = func(c int) { code = c }
	t.Cleanup(func() { poolFlushNeustart = alt })

	// Ein Pool-Konto im Speicher, useDB an, aber keine Datenbank: das
	// Speichern paniert mitten im Ablauf (nil *sql.DB).
	cs := &ChainState{accounts: newShardedAccounts(), useDB: true}
	cs.accounts.Set(validatorsPoolAddr, &AccountState{Address: validatorsPoolAddr, Balance: NewDecimal(1)})
	cs.poolFlushDirty.Store(true)
	SafeCall("poolFlush-test", cs.poolFlushTakt)
	if code != 9 {
		t.Fatalf("Neustart-Code %d, erwartet 9", code)
	}
	if !cs.poolFlushDirty.Load() {
		t.Fatal("Pool-Konten nicht wieder als ungespeichert markiert")
	}
}
