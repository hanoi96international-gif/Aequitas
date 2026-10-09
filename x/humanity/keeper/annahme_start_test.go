package keeper

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// Missbrauch der Annahme (Pruefung von #318): ein Beobachter erzeugt nie
// (ProduceBlock kehrt vor der Messung um). Er mass darum nie und nahm ueber
// die HTTP-Wege an, bis der Rueckstau-Messer nach 10 Minuten griff -- ein
// Ausgang, der in keinen Block kommt.
func TestAnnahme_BeobachterNimmtNichtAn(t *testing.T) {
	_, cs := newDeterminismTestDAG()
	if err := cs.annahmeBeginnen(distTestAddr(2201)); err != nil {
		t.Fatalf("Vorbedingung: ohne Beobachter nimmt der Knoten an: %v", err)
	}
	cs.annahmeEnde()
	setzeBeobachterFuerTest(t, true)
	if err := cs.annahmeBeginnen(distTestAddr(2202)); !errors.Is(err, ErrAnnahmePausiert) {
		t.Fatalf("Beobachter nimmt an: %v", err)
	}
	if n := cs.annahmenLaufend.Load(); n != 0 {
		t.Fatalf("abgelehnte Annahme haengt im Zaehler: %d", n)
	}
	if st := cs.AnnahmePauseStand(); st["pausiert"] != true {
		t.Fatalf("/api/health/combined zeigt den Beobachter nicht als pausiert: %v", st)
	}
}

// Missbrauch der Annahme (Pruefung von #318): die Messung begann erst mit
// dem ersten Erzeugungsversuch, also nach Bootstrap und Resync beim Start.
// Bis dahin nahmen die HTTP-Wege ohne Pause an. Jetzt beginnt sie beim Bau
// des Knotens (main.go), einmal.
func TestAnnahme_MessungAbStart(t *testing.T) {
	_, cs := newDeterminismTestDAG()
	if cs.erzeugerSeit.Load() != 0 {
		t.Fatal("Vorbedingung: ein frisch gebauter Knoten misst noch nicht")
	}
	vorher := time.Now().Unix()
	cs.AnnahmeMessungBeginnen()
	if s := cs.erzeugerSeit.Load(); s < vorher || s > time.Now().Unix() {
		t.Fatalf("Messbeginn %d, erwartet jetzt (%d)", s, vorher)
	}
	if err := cs.annahmeBeginnen(distTestAddr(2203)); err != nil {
		t.Fatalf("binnen der Frist nach dem Start angehalten: %v", err)
	}
	cs.annahmeEnde()

	// Kein Block seit dem Start (Bootstrap dauert): nach der Frist angehalten,
	// ohne dass ProduceBlock je lief.
	alt := time.Now().Unix() - admissionStallLimit() - 1
	cs.erzeugerSeit.Store(alt)
	if err := cs.annahmeBeginnen(distTestAddr(2204)); !errors.Is(err, ErrAnnahmePausiert) {
		t.Fatalf("Annahme %d s nach dem Start ohne eigenen Block nicht angehalten: %v", admissionStallLimit()+1, err)
	}
	// Ein zweiter Aufruf setzt die laufende Messung nicht zurueck.
	cs.AnnahmeMessungBeginnen()
	if cs.erzeugerSeit.Load() != alt {
		t.Fatal("ein zweiter Aufruf hat die Messung neu begonnen")
	}
	// Der erste eigene Block hebt die Pause auf.
	cs.letzterEigenerBlock.Store(time.Now().Unix())
	if err := cs.annahmeBeginnen(distTestAddr(2205)); err != nil {
		t.Fatalf("nach dem ersten eigenen Block weiter angehalten: %v", err)
	}
	cs.annahmeEnde()
}

// nil-sicher wie die uebrigen Methoden des Ausgangs.
func TestAnnahme_MessungAbStartOhneZustand(t *testing.T) {
	var cs *ChainState
	cs.AnnahmeMessungBeginnen()
}

// Missbrauch (Pruefung von #322, LOW-1): ein Beobachter nimmt auch dann keine
// Registrierung an, wenn er sonst nichts annimmt (ANNAHME_ROLLE=nur_lesend,
// Folger). Vorher lief sie an der Pause vorbei bis zur EVM, und der Mensch
// bekam "Registered", ohne je in einen Block zu kommen.
func TestAnnahme_BeobachterNimmtKeineRegistrierungAn(t *testing.T) {
	cs := newTestState()
	cs.SetzeNurLesend(true)
	a := &APIServer{state: cs}
	ip := "198.51.100.231"
	ipBurst.Delete("register:" + ip)
	t.Cleanup(func() { ipBurst.Delete("register:" + ip) })
	schicke := func() string {
		body := `{"wallet":"0x00000000000000000000000000000000000000ab","pA":["1","2"],"pB":[["1","2"],["3","4"]],"pC":["1","2"],"pubSignals":["1","2"],"signature":"0x01"}`
		req := httptest.NewRequest(http.MethodPost, "/api/register", strings.NewReader(body))
		req.RemoteAddr = ip + ":4711"
		w := httptest.NewRecorder()
		a.handleRegister(w, req)
		var resp RegisterResponse
		json.NewDecoder(w.Body).Decode(&resp)
		if resp.Success {
			t.Fatal("Registrierung angenommen")
		}
		return resp.Message
	}
	// Gegenprobe: ohne Beobachter kommt die Anfrage bis zur EVM (die es im
	// Test nicht gibt) -- die Pause gilt fuer einen nur lesenden Knoten nicht.
	if msg := schicke(); !strings.Contains(msg, "EVM engine unavailable") {
		t.Fatalf("Vorbedingung: ohne Beobachter bis zur EVM, bekam %q", msg)
	}
	setzeBeobachterFuerTest(t, true)
	if msg := schicke(); !strings.Contains(msg, "Beobachter") {
		t.Fatalf("Beobachter (nur lesend) nimmt eine Registrierung an: %q", msg)
	}
}

// Ein Beobachter wird nicht Leiter (Pruefung von #322, INFO-1): er erzeugt
// nie, als Leiter hielte er die Annahme des ganzen Netzes an.
func TestLeitung_BeobachterNichtLeiterfaehig(t *testing.T) {
	leistung.mu.Lock()
	altZwang := leistung.zwang
	leistung.zwang = "ja"
	leistung.mu.Unlock()
	t.Cleanup(func() { leistung.mu.Lock(); leistung.zwang = altZwang; leistung.mu.Unlock() })
	cs := newTestState()
	if !leiterFaehig(cs) {
		t.Fatal("Vorbedingung: mit Leistungsnachweis und annehmend leiterfaehig")
	}
	setzeBeobachterFuerTest(t, true)
	if leiterFaehig(cs) {
		t.Fatal("Beobachter ist leiterfaehig")
	}
}

// Missbrauch (Pruefung von #322, LOW-4 und INFO-2): ein Beobachter legt
// nichts in den Ausgang -- weder die eigene Tagesverteilung noch irgendeinen
// anderen Auftrag. Die Sperre sitzt dort, wo jeder Ausgang durchgeht, und
// greift vor jeder Aenderung am Zustand.
func TestBeobachter_LegtNichtsInDenAusgang(t *testing.T) {
	cs := newTestState()
	lief := false
	einzeln := func(ctx context.Context) (Transaction, error) { lief = true; return Transaction{}, nil }
	viele := func(ctx context.Context) ([]Transaction, error) { lief = true; return nil, nil }
	if err := cs.runAtomicWithOutbox(nil, false, einzeln); err != nil || !lief {
		t.Fatalf("Vorbedingung: ohne Beobachter laeuft der Auftrag (%v, %v)", err, lief)
	}
	setzeBeobachterFuerTest(t, true)
	lief = false
	if err := cs.runAtomicWithOutbox(nil, false, einzeln); !errors.Is(err, ErrAnnahmePausiert) || lief {
		t.Fatalf("Beobachter: runAtomicWithOutbox = %v, Auftrag gelaufen = %v", err, lief)
	}
	if err := cs.runAtomicDistributionWithOutbox(viele); !errors.Is(err, ErrAnnahmePausiert) || lief {
		t.Fatalf("Beobachter: runAtomicDistributionWithOutbox = %v, Auftrag gelaufen = %v", err, lief)
	}
	if err := cs.RunDailyDistributionAtomic(time.Now().Unix()); !errors.Is(err, ErrAnnahmePausiert) {
		t.Fatalf("Beobachter verteilt die Tagesrunde selbst: %v", err)
	}
}

// Ein Beobachter baut keine Leitung (Pruefung von #322, INFO-6 und INFO-8):
// sonst bliebe er Startleiter, gespeicherter Leiter oder Leiter im
// Notbetrieb und bekaeme in Stufe 2 Konten zugeteilt.
func TestLeitung_BeobachterBautKeineLeitung(t *testing.T) {
	t.Setenv("AEQUITAS_LEITUNG", "an")
	t.Setenv("AEQUITAS_LEITUNG_GENESIS", "")
	altGestartet := leistungGestartet.Swap(true) // keine Messung im Test
	t.Cleanup(func() { leistungGestartet.Store(altGestartet) })
	setzeBeobachterFuerTest(t, true)
	k, _ := crypto.GenerateKey()
	cs := newTestState()
	if l := StarteLeitung(&BlockDAG{signingKey: k}, cs, "http://203.0.113.9:8080"); l != nil || cs.leitung.Load() != nil {
		t.Fatal("Beobachter hat eine Leitung gebaut")
	}
}

// Missbrauch (Pruefung von #322, LOW-9): ein Beobachter holt keine
// Registrierung nach -- der nebenlaeufige Pfad schrieb Konto, Nullifier und
// Ausgang an runAtomicWithOutbox vorbei. Stand und Annahme-Tor sagen, dass er
// nicht annimmt (INFO-10).
func TestBeobachter_RegistriertNicht(t *testing.T) {
	cs := newTestState()
	w := "0x00000000000000000000000000000000000000d1"
	setzeBeobachterFuerTest(t, true)
	if err := cs.RegisterHumanAtomic(w, Transaction{Type: "register_human", Wallet: w}); !errors.Is(err, ErrAnnahmePausiert) {
		t.Fatalf("Beobachter registriert: %v", err)
	}
	if cs.IsHuman(w) {
		t.Fatal("Beobachter hat das Konto als Mensch gesetzt")
	}
	if st := cs.AnnahmeTorStand(); st["nimmt_an"] != false || st["beobachter"] != true {
		t.Fatalf("Annahme-Tor meldet fuer den Beobachter %v", st)
	}
	if st := cs.LeitungStand(); st["beobachter"] != true {
		t.Fatalf("Leitungsstand meldet den Beobachter nicht: %v", st)
	}
}

// Dasselbe mit Datenbank ueber die Wiederholung (alle 5 Minuten und
// /api/admin/registration-recovery/retry): beide Wege -- mit Nullifier
// (nebenlaeufig) und ohne (RegisterHuman, ganz ohne Ausgang) -- holen auf
// einem Beobachter nichts nach; die Zeilen bleiben fuer einen Rollenwechsel.
func TestBeobachter_HoltKeineRegistrierungNach_RealDB(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	cs := testKnoten(t, "unused-beobachter-recovery-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	if _, err := cs.db.Exec(`DELETE FROM registration_recovery`); err != nil {
		t.Fatal(err)
	}
	mit, ohne := "0x00000000000000000000000000000000000000d2", "0x00000000000000000000000000000000000000d3"
	for _, f := range [][2]string{{mit, "0x" + strings.Repeat("d2", 32)}, {ohne, ""}} {
		if _, err := cs.SaveRegistrationIntent(f[0], f[1], Transaction{Type: "register_human", Wallet: f[0], Nullifier: f[1]}); err != nil {
			t.Fatal(err)
		}
	}
	var vorher int
	cs.db.QueryRow(`SELECT COUNT(*) FROM pending_txs`).Scan(&vorher)
	setzeBeobachterFuerTest(t, true)
	if n := cs.RetryRegistrationRecoveries(); n != 0 {
		t.Fatalf("Beobachter hat %d Registrierungen nachgeholt", n)
	}
	var nachher int
	cs.db.QueryRow(`SELECT COUNT(*) FROM pending_txs`).Scan(&nachher)
	if nachher != vorher || cs.IsHuman(mit) || cs.IsHuman(ohne) || cs.CountUnrecoveredRegistrations() != 2 {
		t.Fatalf("Beobachter: Ausgang %d -> %d, Mensch %v/%v, offen %d", vorher, nachher, cs.IsHuman(mit), cs.IsHuman(ohne), cs.CountUnrecoveredRegistrations())
	}
	// Gegenprobe: ohne Beobachter holt derselbe Knoten beide nach.
	beobachterAn.Store(false)
	if n := cs.RetryRegistrationRecoveries(); n != 2 || !cs.IsHuman(mit) || !cs.IsHuman(ohne) {
		t.Fatalf("Gegenprobe: %d nachgeholt, Mensch %v/%v", n, cs.IsHuman(mit), cs.IsHuman(ohne))
	}
}

// Der Kern von LOW-9 mit Datenbank (Pruefung von #322, INFO-14): mit
// Nullifier nimmt RegisterHumanAtomic den nebenlaeufigen Pfad, der Konto,
// Nullifier und Ausgang selbst schreibt -- auf einem Beobachter nicht.
// RegisterHuman (ohne Ausgang) ist ebenso gesperrt.
func TestBeobachter_RegisterHumanAtomicMitNullifier_RealDB(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	cs := testKnoten(t, "unused-beobachter-register-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	w, w2 := "0x00000000000000000000000000000000000000d4", "0x00000000000000000000000000000000000000d5"
	var vorher int
	cs.db.QueryRow(`SELECT COUNT(*) FROM pending_txs`).Scan(&vorher)
	setzeBeobachterFuerTest(t, true)
	if err := cs.RegisterHumanAtomic(w, Transaction{Type: "register_human", Wallet: w, Nullifier: "0x" + strings.Repeat("d4", 32)}); !errors.Is(err, ErrAnnahmePausiert) {
		t.Fatalf("Beobachter: RegisterHumanAtomic mit Nullifier = %v", err)
	}
	if err := cs.RegisterHuman(w2); !errors.Is(err, ErrAnnahmePausiert) {
		t.Fatalf("Beobachter: RegisterHuman = %v", err)
	}
	var nachher int
	cs.db.QueryRow(`SELECT COUNT(*) FROM pending_txs`).Scan(&nachher)
	if nachher != vorher || cs.IsHuman(w) || cs.IsHuman(w2) {
		t.Fatalf("Beobachter: Ausgang %d -> %d, Mensch %v/%v", vorher, nachher, cs.IsHuman(w), cs.IsHuman(w2))
	}
}

// Missbrauch (Pruefung von #322, LOW-13): ein Knoten mit nicht geflushtem
// WAL wird zum Beobachter. Er liest das WAL nicht ein -- keine Ueberweisung
// angewandt, nichts in pending_txs, die Datei bleibt liegen.
func TestBeobachter_LiestKeinWALEin_RealDB(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "beobachter.wal")
	truncateDistTestTables(t)
	csA := newWALTestState(t, walPath)
	from, to := distTestAddr(830), distTestAddr(831)
	seedConcurrentTestAccount(t, csA, from, 1000, time.Now().Unix())
	seedConcurrentTestAccount(t, csA, to, 0, time.Now().Unix())
	if _, _, applied, err := csA.transferConcurrentWAL(from, to, 42, Transaction{Type: "transfer", Wallet: from, To: to, Amount: 42, TxHash: "0xbeobachterwal1"}); !applied || err != nil {
		t.Fatalf("Vorbedingung: Ueberweisung ueber das WAL: applied=%v err=%v", applied, err)
	}
	csA.stopWALFlushWorkerForTest()
	if err := csA.wal.Close(); err != nil {
		t.Fatal(err)
	}
	var vorher int
	csA.db.QueryRow(`SELECT COUNT(*) FROM pending_txs`).Scan(&vorher)
	groesse := func() int64 {
		st, err := os.Stat(walPath)
		if err != nil {
			t.Fatal(err)
		}
		return st.Size()
	}
	vorherGroesse := groesse()

	setzeBeobachterFuerTest(t, true)
	csB := NewChainState("unused-wal-test.json")
	t.Cleanup(func() {
		if csB.db != nil {
			csB.db.Close()
		}
	})
	if csB.wal != nil {
		t.Fatal("Beobachter hat das WAL geoeffnet")
	}
	time.Sleep(300 * time.Millisecond)
	var dbFrom float64
	if err := csB.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, from).Scan(&dbFrom); err != nil {
		t.Fatal(err)
	}
	var nachher int
	csB.db.QueryRow(`SELECT COUNT(*) FROM pending_txs`).Scan(&nachher)
	if dbFrom != 1000 || nachher != vorher || groesse() != vorherGroesse {
		t.Fatalf("Beobachter hat das WAL angewandt: Kontostand %v (erwartet 1000), Ausgang %d -> %d, WAL %d -> %d Byte", dbFrom, vorher, nachher, vorherGroesse, groesse())
	}
}

// Missbrauch (docs/OFFEN.md; Pruefung von #322, INFO-7): mit
// AEQUITAS_LEITUNG=an ist cs.leitung bis StarteLeitung nil, und nimmtAnFuer()
// meldet true -- ein Folger naehme beim Start lokal an, statt an den Leiter
// weiterzuleiten, und zwei Knoten naehmen zugleich an. Bis der Start versucht
// ist, haelt jeder annehmende Weg an (die Ausgangsschreiber ohne Annahme
// stehen in docs/OFFEN.md, Punkt 7), und es laufen keine Systemauftraege;
// registriert wird wie auf einem Folger. Scheitert der Start (kein
// Signierschluessel), bleibt es so -- fail-closed, mit eigenem Grund im
// Stand (Pruefung von #329, Befund 2).
func TestAnnahme_FolgerWartetAufLeitung(t *testing.T) {
	t.Setenv("AEQUITAS_LEITUNG", "an")
	t.Setenv("AEQUITAS_LEITUNG_GENESIS", "")
	altGestartet := leistungGestartet.Swap(true) // keine Messung im Test
	t.Cleanup(func() { leistungGestartet.Store(altGestartet) })
	cs := newTestState()
	w := "0x00000000000000000000000000000000000000e1"
	pruefe := func(wort string) {
		t.Helper()
		// Ueberweisung, Tausch, Faucet, Unternehmen, Vormund, Erneuerung ...
		err := cs.annahmeBeginnen(w)
		if !errors.Is(err, ErrAnnahmePausiert) || !strings.Contains(err.Error(), wort) {
			t.Fatalf("annahmeBeginnen: %v, erwartet Pause (%q)", err, wort)
		}
		if n := cs.annahmenLaufend.Load(); n != 0 {
			t.Fatalf("abgelehnte Annahme zaehlt noch: %d", n)
		}
		// Validator-Bindung, Strafabrechnung.
		if err := cs.annahmeBeginnenLeiter(); !errors.Is(err, ErrAnnahmePausiert) {
			t.Fatalf("annahmeBeginnenLeiter: %v", err)
		}
		// RPC (eth_sendRawTransaction) und Nonce-Reservierung fragen den Grund.
		if err := cs.annahmePauseGrund(); err == nil || !strings.Contains(err.Error(), wort) {
			t.Fatalf("annahmePauseGrund: %v", err)
		}
		// Das Tor selbst: unabhaengig von der Reihenfolge der Aufrufer
		// (Pruefung von #329, Befund 6).
		if cs.nimmtAnFuer() || cs.nimmtAnFuer(w) {
			t.Fatal("nimmtAnFuer meldet Annahme, obwohl die Leitung nicht bereit ist")
		}
		if err := cs.pruefeAnnahmeTorFuer(w); !errors.Is(err, ErrAnnahmePausiert) {
			t.Fatalf("pruefeAnnahmeTorFuer: %v", err)
		}
		// Keine Systemauftraege (Pruefung von #329, INFO-8).
		if grund := cs.SystemauftraegeHier(); !strings.Contains(grund, wort) {
			t.Fatalf("SystemauftraegeHier: %q", grund)
		}
		// Registrierung: wie auf einem Folger lokal weiter (der Knoten nimmt
		// nichts an, also verblockt er auch keinen Ausgang) -- bis zur EVM,
		// die es im Test nicht gibt.
		a := &APIServer{state: cs}
		ip := "198.51.100.232"
		ipBurst.Delete("register:" + ip)
		t.Cleanup(func() { ipBurst.Delete("register:" + ip) })
		body := `{"wallet":"0x00000000000000000000000000000000000000ab","pA":["1","2"],"pB":[["1","2"],["3","4"]],"pC":["1","2"],"pubSignals":["1","2"],"signature":"0x01"}`
		req := httptest.NewRequest(http.MethodPost, "/api/register", strings.NewReader(body))
		req.RemoteAddr = ip + ":4711"
		rec := httptest.NewRecorder()
		a.handleRegister(rec, req)
		var resp RegisterResponse
		json.NewDecoder(rec.Body).Decode(&resp)
		if resp.Success || !strings.Contains(resp.Message, "EVM engine unavailable") {
			t.Fatalf("Registrierung nicht wie auf einem Folger: %+v", resp)
		}
		if st := cs.AnnahmeTorStand(); st["nimmt_an"] != false {
			t.Fatalf("Annahme-Tor meldet nimmt_an: %v", st)
		}
	}

	pruefe("startet noch")
	if st := cs.AnnahmePauseStand(); st["leitung_startet"] != true || st["pausiert"] != true {
		t.Fatalf("Stand meldet den Start nicht: %v", st)
	}
	if st := cs.LeitungStand(); st["startet"] != true {
		t.Fatalf("Leitungsstand meldet den Start nicht: %v", st)
	}

	// Start gescheitert (kein Signierschluessel): die Annahme bleibt zu.
	if l := StarteLeitung(&BlockDAG{}, cs, ""); l != nil || cs.leitung.Load() != nil {
		t.Fatal("Vorbedingung: ohne Signierschluessel keine Leitung")
	}
	pruefe("laeuft nicht")
	if st := cs.AnnahmePauseStand(); st["leitung_startet"] != false || st["leitung_gescheitert"] != true {
		t.Fatalf("Stand nach gescheitertem Start: %v", st)
	}
	if st := cs.LeitungStand(); st["gescheitert"] != true {
		t.Fatalf("Leitungsstand meldet den gescheiterten Start nicht: %v", st)
	}
}

// Gegenprobe: ohne AEQUITAS_LEITUNG wartet niemand auf eine Leitung, auch
// nicht nach StarteLeitung (das main immer aufruft).
func TestAnnahme_OhneLeitungKeinWarten(t *testing.T) {
	t.Setenv("AEQUITAS_LEITUNG", "")
	altGestartet := leistungGestartet.Swap(true)
	t.Cleanup(func() { leistungGestartet.Store(altGestartet) })
	cs := newTestState()
	w := "0x00000000000000000000000000000000000000e2"
	if err := cs.annahmeBeginnen(w); err != nil {
		t.Fatalf("ohne Leitung angehalten: %v", err)
	}
	cs.annahmeEnde()
	if l := StarteLeitung(&BlockDAG{}, cs, ""); l != nil {
		t.Fatal("ohne AEQUITAS_LEITUNG eine Leitung gebaut")
	}
	if err := cs.annahmeBeginnen(w); err != nil || cs.leitungGescheitert.Load() {
		t.Fatalf("ohne Leitung nach StarteLeitung angehalten: %v (gescheitert %v)", err, cs.leitungGescheitert.Load())
	}
	cs.annahmeEnde()
}

// Ein Beobachter laeuft bewusst ohne Leitung: kein gescheiterter Start (er
// haelt aus eigenem Grund an).
func TestAnnahme_BeobachterOhneLeitungNichtGescheitert(t *testing.T) {
	t.Setenv("AEQUITAS_LEITUNG", "an")
	t.Setenv("AEQUITAS_LEITUNG_GENESIS", "")
	altGestartet := leistungGestartet.Swap(true)
	t.Cleanup(func() { leistungGestartet.Store(altGestartet) })
	setzeBeobachterFuerTest(t, true)
	cs := newTestState()
	StarteLeitung(&BlockDAG{}, cs, "")
	if cs.leitungGescheitert.Load() {
		t.Fatal("Beobachter gilt als gescheiterter Start")
	}
	if err := cs.annahmeBeginnen("0x00000000000000000000000000000000000000e3"); err == nil || !strings.Contains(err.Error(), "Beobachter") {
		t.Fatalf("Beobachter: %v", err)
	}
}

// folgerLeitungAufbau: AEQUITAS_LEITUNG=an mit einem Genesis-Satz, in dem die
// kleinste Adresse (der Startleiter) jemand anders ist -- dieser Knoten ist
// Folger. Der Takt startet nicht (keine Goroutine ohne Ende im Test), die
// Freiliste wird wiederhergestellt.
func folgerLeitungAufbau(t *testing.T) (*BlockDAG, func(dag *BlockDAG, l *Leitung, cs *ChainState)) {
	t.Helper()
	k, _ := crypto.GenerateKey()
	ich := strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex())
	leiter := "0x0000000000000000000000000000000000000001"
	t.Setenv("AEQUITAS_LEITUNG", "an")
	t.Setenv("AEQUITAS_LEITUNG_GENESIS", leiter+"=http://203.0.113.1:8080,"+ich+"=http://203.0.113.2:8080")
	altGestartet := leistungGestartet.Swap(true)
	altTakt := leitungTaktStarten
	altFrei := rpcRateLimitFreiListe.Load()
	t.Cleanup(func() {
		leistungGestartet.Store(altGestartet)
		leitungTaktStarten = altTakt
		rpcRateLimitFreiListe.Store(altFrei)
	})
	return &BlockDAG{signingKey: k}, altTakt
}

// Der Erfolgsweg (Pruefung von #329, Befund 3): als Folger steht die Leitung
// in cs.leitung, BEVOR der Start als versucht gilt (geprueft im Takt-Start,
// der nach dem Speichern und vor der Rueckkehr laeuft), und die Annahme geht
// danach an den Leiter (ErrNichtLeiter), nicht lokal.
func TestAnnahme_FolgerMitLeitung(t *testing.T) {
	dag, _ := folgerLeitungAufbau(t)
	cs := newTestState()
	taktGestartet := false
	leitungTaktStarten = func(_ *BlockDAG, l *Leitung, c *ChainState) {
		taktGestartet = true
		if c.leitung.Load() != l {
			t.Error("Takt startet, bevor die Leitung gespeichert ist")
		}
		if c.leitungStartVersucht.Load() {
			t.Error("Start gilt als versucht, bevor er zurueckkehrt")
		}
	}
	l := StarteLeitung(dag, cs, "")
	if l == nil || cs.leitung.Load() != l || !taktGestartet {
		t.Fatalf("Leitung nicht gebaut (l=%v, gespeichert=%v, Takt=%v)", l != nil, cs.leitung.Load() != nil, taktGestartet)
	}
	if !cs.leitungStartVersucht.Load() || cs.leitungGescheitert.Load() {
		t.Fatalf("nach dem Start: versucht %v, gescheitert %v", cs.leitungStartVersucht.Load(), cs.leitungGescheitert.Load())
	}
	if err := cs.annahmeBeginnen("0x00000000000000000000000000000000000000e4"); !errors.Is(err, ErrNichtLeiter) {
		t.Fatalf("Folger nach dem Start: %v, erwartet ErrNichtLeiter", err)
	}
}

// Gegenprobe zu Befund 1 (Pruefung von #329): viele Annahmen laufen, waehrend
// die Leitung startet. Keine einzige darf lokal angenommen werden -- weder vor
// dem Start (Pause) noch danach (Folger). Die Umschaltkante selbst ist hier
// zu schmal, um sie sicher zu treffen; die Reihenfolge, die sie schliesst,
// prueft TestAnnahme_LeitungVorDemTor.
func TestAnnahme_UmschaltkanteNimmtNichtAn(t *testing.T) {
	dag, _ := folgerLeitungAufbau(t)
	leitungTaktStarten = func(*BlockDAG, *Leitung, *ChainState) {}
	w := "0x00000000000000000000000000000000000000e5"
	for runde := 0; runde < 100; runde++ {
		cs := newTestState()
		var angenommen atomic.Int64
		los := make(chan struct{})
		var wg sync.WaitGroup
		for g := 0; g < 32; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-los
				for i := 0; i < 50; i++ {
					if cs.annahmeBeginnen(w) == nil {
						angenommen.Add(1)
						cs.annahmeEnde()
					}
					if cs.annahmeBeginnenLeiter() == nil {
						angenommen.Add(1)
						cs.annahmeEnde()
					}
				}
			}()
		}
		close(los)
		StarteLeitung(dag, cs, "")
		wg.Wait()
		if n := angenommen.Load(); n != 0 {
			t.Fatalf("Runde %d: %d Annahmen lokal auf dem Folger", runde, n)
		}
	}
}

// Befund 1 aus der Pruefung von #329, ohne Zufall: das Tor liest cs.leitung,
// die Pause den Merker. Steht das Tor vorn, sieht eine Annahme an der
// Umschaltkante "keine Leitung" und danach "Start versucht" und nimmt lokal
// an. Darum fragt jeder annehmende Weg leitungNichtBereit, BEVOR er
// cs.leitung liest (Syntaxbaum, nicht Textsuche).
func TestAnnahme_LeitungVorDemTor(t *testing.T) {
	for _, f := range []struct{ datei, fn, vorher, nachher string }{
		{"annahme_tor.go", "annahmeBeginnen", "cs.leitungNichtBereit", "cs.pruefeAnnahmeTorFuer"},
		{"validator_bindung_annahme.go", "annahmeBeginnenLeiter", "cs.leitungNichtBereit", "cs.leitung.Load"},
		{"validator_bindung_annahme.go", "annahmeBeginnenLeiter", "cs.leitungNichtBereit", "cs.pruefeAnnahmeTorFuer"},
	} {
		datei, err := parser.ParseFile(token.NewFileSet(), f.datei, nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", f.datei, err)
		}
		var rumpf []ast.Stmt
		for _, d := range datei.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == f.fn {
				rumpf = fn.Body.List
			}
		}
		if rumpf == nil {
			t.Fatalf("%s: %s nicht gefunden", f.datei, f.fn)
		}
		erste := func(aufruf string) int {
			for i, st := range rumpf {
				gefunden := false
				ast.Inspect(st, func(n ast.Node) bool {
					if c, ok := n.(*ast.CallExpr); ok && types.ExprString(c.Fun) == aufruf {
						gefunden = true
					}
					return !gefunden
				})
				if gefunden {
					return i
				}
			}
			return -1
		}
		i, j := erste(f.vorher), erste(f.nachher)
		if i < 0 || j < 0 || i >= j {
			t.Errorf("%s: %s (Anweisung %d) muss vor %s (Anweisung %d) stehen", f.fn, f.vorher, i, f.nachher, j)
		}
		// ... und die Ablehnung kehrt zurueck (Pruefung von #329, Befund 6).
		if i >= 0 {
			wi, ok := rumpf[i].(*ast.IfStmt)
			zurueck := false
			if ok {
				for _, st := range wi.Body.List {
					if _, r := st.(*ast.ReturnStmt); r {
						zurueck = true
					}
				}
			}
			if !zurueck {
				t.Errorf("%s: die Anweisung mit %s kehrt bei Ablehnung nicht zurueck", f.fn, f.vorher)
			}
		}
	}
}

// aufrufStellen: die erste Stelle jedes Aufrufs (als Text, etwa
// "cs.leitung.Load") im Rumpf von fn in datei.
func aufrufStellen(t *testing.T, datei, fn string) map[string]token.Pos {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), datei, nil, 0)
	if err != nil {
		t.Fatalf("%s: %v", datei, err)
	}
	stellen := map[string]token.Pos{}
	gefunden := false
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name != fn {
			continue
		}
		gefunden = true
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok {
				k := types.ExprString(c.Fun)
				if _, da := stellen[k]; !da {
					stellen[k] = c.Pos()
				}
			}
			return true
		})
	}
	if !gefunden {
		t.Fatalf("%s: %s nicht gefunden", datei, fn)
	}
	return stellen
}

// Befund 6 aus der Pruefung von #329: die Reihenfolgen, an denen das
// Startfenster haengt. StarteLeitung setzt gescheitert VOR versucht (sonst
// saehe ein Leser kurz "versucht, nicht gescheitert, keine Leitung");
// leitungNichtBereit liest versucht VOR gescheitert (aus demselben Grund);
// der RPC-Weg fragt die Pause vor dem Tor und vor der Nonce-Reservierung.
func TestAnnahme_MerkerReihenfolge(t *testing.T) {
	for _, f := range []struct{ datei, fn, vorher, nachher string }{
		{"leitung_netz.go", "StarteLeitung", "cs.leitungGescheitert.Store", "cs.leitungStartVersucht.Store"},
		{"annahme_pause.go", "leitungNichtBereit", "cs.leitungStartVersucht.Load", "cs.leitungGescheitert.Load"},
		{"evm_rpc.go", "sendRawTransaction", "s.state.annahmePauseGrund", "s.state.pruefeAnnahmeTorFuer"},
		{"evm_rpc.go", "sendRawTransaction", "s.state.annahmePauseGrund", "s.reserveNoncePerItem"},
	} {
		st := aufrufStellen(t, f.datei, f.fn)
		v, okV := st[f.vorher]
		n, okN := st[f.nachher]
		if !okV || !okN || v >= n {
			t.Errorf("%s: %s muss vor %s stehen (gefunden %v/%v)", f.fn, f.vorher, f.nachher, okV, okN)
		}
	}
}
