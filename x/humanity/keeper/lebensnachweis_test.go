package keeper

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// Jaehrlicher Lebensnachweis (lebensnachweis.go), ohne Datenbank: Fristen,
// Pruefung jedes Knotens samt Missbrauch, Ausschluss vom Grundeinkommen
// (und dass die anderen den Anteil bekommen), Wiederaufnahme, StateRoot.

const (
	lnStichtag        int64 = 1_700_000_000
	lnTag             int64 = 86400
	lnMensch                = "0x00000000000000000000000000000000000000c1" // Coordinator-Mensch (Schluessel unten)
	lnWallet                = "0x00000000000000000000000000000000000000d1"
	lnAndere                = "0x00000000000000000000000000000000000000d2"
	lnDritte                = "0x00000000000000000000000000000000000000d3"
	lnErneuert              = "0x00000000000000000000000000000000000000d4"
	lnRegelOhne             = "lebensnachweis_ohne_bescheinigung"
	lnRegelZeit             = "lebensnachweis_zeit"
	lnRegelFrueh            = "lebensnachweis_zu_frueh"
	lnRegelKeinMensch       = "lebensnachweis_kein_mensch"
)

// lnAktiv: Stichtag und Zulassung der Coordinatoren fuer diesen Test.
func lnAktiv(t *testing.T, stichtag, registerLeser int64) {
	t.Helper()
	vorherAb, vorherLeser := lebensnachweisAbOverride.Load(), registerLeserOverride.Load()
	lebensnachweisAbOverride.Store(stichtag)
	registerLeserOverride.Store(registerLeser)
	t.Cleanup(func() {
		lebensnachweisAbOverride.Store(vorherAb)
		registerLeserOverride.Store(vorherLeser)
	})
}

type lnFall struct {
	t          *testing.T
	cs         *ChainState
	priv       ed25519.PrivateKey
	pubHex     string
	mensch     string
	bindung    CoordinatorBindung
	zugelassen bool
}

// neuerLnFall: ein zugelassener Coordinator (Bindung mit Freigabe und
// Besitznachweis), drei Menschen mit 1.000 AEQ, Stichtag lnStichtag.
func neuerLnFall(t *testing.T) *lnFall {
	t.Helper()
	lnAktiv(t, lnStichtag, 1)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	mk, mensch := neuerSchluessel(t)
	pubHex := hex.EncodeToString(pub)
	f := &lnFall{t: t, cs: newTestState(), priv: priv, pubHex: pubHex, mensch: mensch, zugelassen: true,
		bindung: CoordinatorBindung{Mensch: mensch,
			MenschSig:     personalSign(t, mk, coordinatorFreigabeNachricht(pubHex)),
			SchluesselSig: hex.EncodeToString(ed25519.Sign(priv, []byte(coordinatorBesitzNachricht(mensch))))}}
	addHuman(f.cs, mensch, 10)
	for _, w := range []string{lnWallet, lnAndere, lnDritte} {
		addHuman(f.cs, w, 1000)
	}
	f.cs.zulassungOhneDB = func(m string, _ int64) (bool, error) { return f.zugelassen && m == f.mensch, nil }
	return f
}

func (f *lnFall) signiere(nachricht string) string {
	return hex.EncodeToString(ed25519.Sign(f.priv, []byte(nachricht)))
}

func (f *lnFall) tx(wallet string, issued int64) Transaction {
	return lebensnachweisTransaktion(wallet, issued, f.pubHex, f.signiere(lebensnachweisNachricht(wallet, issued)), f.bindung)
}

// anwenden: wie replayTransactions, Zaehler-Differenz der Regeln zurueck.
func (f *lnFall) anwenden(tx Transaction, blockZeit int64) (map[string]int64, error) {
	f.t.Helper()
	regeln := []string{lnRegelOhne, lnRegelZeit, lnRegelFrueh, lnRegelKeinMensch}
	vorher := map[string]int64{}
	for _, r := range regeln {
		vorher[r] = erhaltungZaehler(r)
	}
	f.cs.mu.Lock()
	err := f.cs.applyLebensnachweisLocked(context.Background(), &tx, blockZeit)
	f.cs.mu.Unlock()
	neu := map[string]int64{}
	for _, r := range regeln {
		if d := erhaltungZaehler(r) - vorher[r]; d != 0 {
			neu[r] = d
		}
	}
	return neu, err
}

func (f *lnFall) am(w string) int64 { return acct(f.cs, w).LebensnachweisAm }

// Der Satz Byte fuer Byte wie coordinator/app/lebensnachweis.py
// (ketten_nachricht): Domaene|chain:<eingefroren>|wallet|issued_at.
func TestLebensnachweis_NachrichtWieDerCoordinator(t *testing.T) {
	got := lebensnachweisNachricht("0x00000000000000000000000000000000000000ab", 1793491200)
	want := "aequitas-proof-of-life-v1|chain:1926|0x00000000000000000000000000000000000000ab|1793491200"
	if got != want {
		t.Fatalf("Satz %q, erwartet %q", got, want)
	}
	if strings.Contains(got, livenessRenewalDomain) || strings.Contains(erneuerungsNachricht("0xab", 1), lebensnachweisDomain) {
		t.Fatal("die Domaenen von Erneuerung und Lebensnachweis muessen verschieden sein")
	}
	if lebensnachweisAbUnix != 1793491200 {
		t.Fatalf("Stichtag %d, festgelegt ist 01.11.2026 00:00:00 UTC = 1793491200", lebensnachweisAbUnix)
	}
}

// Fristen: ohne Nachweis ab dem Stichtag, mit Nachweis ab ihm; pausiert nur,
// wenn aktiv (Stichtag UND Zulassung) und t >= am + 395 Tage.
func TestLebensnachweis_Fristen(t *testing.T) {
	lnAktiv(t, lnStichtag, 1)
	acc := &AccountState{Address: lnWallet, IsHuman: true}
	f := lebensnachweisFristenVon(acc)
	if f.am != lnStichtag || f.faelligAb != lnStichtag+300*lnTag || f.frist != lnStichtag+365*lnTag || f.pauseAb != lnStichtag+395*lnTag {
		t.Fatalf("ohne Nachweis: %+v", f)
	}
	acc.LebensnachweisAm = lnStichtag + 10*lnTag
	if f := lebensnachweisFristenVon(acc); f.am != lnStichtag+10*lnTag || f.pauseAb != lnStichtag+405*lnTag {
		t.Fatalf("mit Nachweis: %+v", f)
	}
	// Ein Wert vor dem Stichtag (gibt es nicht, aber falls) zaehlt als Stichtag.
	acc.LebensnachweisAm = 5
	if f := lebensnachweisFristenVon(acc); f.am != lnStichtag {
		t.Fatalf("vor dem Stichtag: %+v", f)
	}
	acc.LebensnachweisAm = 0
	pause := lnStichtag + 395*lnTag
	if lebensnachweisPausiert(acc, pause-1) || !lebensnachweisPausiert(acc, pause) {
		t.Fatal("pausiert genau ab pause_ab")
	}
	// Ohne Zulassung der Coordinatoren kann niemand nachweisen -- also wird
	// niemand pausiert (kein Geld ohne Zeitgrenze zurueckgehalten).
	registerLeserOverride.Store(math.MaxInt64) // Konstante: MaxInt64
	if lebensnachweisAktiv(pause) || lebensnachweisPausiert(acc, pause+1000*lnTag) {
		t.Fatal("ohne zugelassene Coordinatoren darf niemand pausiert sein")
	}
	registerLeserOverride.Store(1)
	// Vor dem Stichtag nicht aktiv, niemand pausiert.
	if lebensnachweisAktiv(lnStichtag-1) || lebensnachweisPausiert(acc, lnStichtag-1) {
		t.Fatal("vor dem Stichtag aktiv")
	}
	// Kein Mensch: nie pausiert (und kein Stand).
	if lebensnachweisPausiert(&AccountState{Address: lnAndere}, pause) || LebensnachweisStand(&AccountState{Address: lnAndere}, pause, true) != nil {
		t.Fatal("Nicht-Mensch mit Lebensnachweis-Stand")
	}
	// Ueberlauf: am nahe MaxInt64 laeuft nicht ins Negative.
	acc.LebensnachweisAm = math.MaxInt64 - 5
	if f := lebensnachweisFristenVon(acc); f.pauseAb != math.MaxInt64 || f.faelligAb != math.MaxInt64 {
		t.Fatalf("Ueberlauf: %+v", f)
	}
}

// /api/balance: das Objekt lebensnachweis mit genau den sechs Schluesseln,
// die Zeitpunkte als ganze Zahlen (nie Text, nie Komma), aktiv/pausiert als
// bool. Fuer Nicht-Menschen fehlt es. Vor dem Stichtag aktiv=false mit
// berechneten Werten.
func TestLebensnachweis_BalanceForm(t *testing.T) {
	lnAktiv(t, 4_000_000_000, 1) // Stichtag in der Zukunft
	cs := newTestState()
	addHuman(cs, lnWallet, 1000)
	cs.accounts.Set(lnAndere, &AccountState{Address: lnAndere, Balance: NewDecimal(5)})
	a := &APIServer{state: cs}
	lies := func(w string) map[string]json.RawMessage {
		rec := httptest.NewRecorder()
		a.handleBalance(rec, httptest.NewRequest(http.MethodGet, "/api/balance?wallet="+w, nil))
		var body map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: %v", rec.Body.String(), err)
		}
		return body
	}
	body := lies(lnWallet)
	var ln map[string]json.RawMessage
	if err := json.Unmarshal(body["lebensnachweis"], &ln); err != nil {
		t.Fatalf("lebensnachweis fehlt oder ist kein Objekt: %s", body["lebensnachweis"])
	}
	if len(ln) != 6 {
		t.Fatalf("Schluessel: %v", ln)
	}
	for _, k := range []string{"aktiv", "pausiert"} {
		if s := string(ln[k]); s != "true" && s != "false" {
			t.Fatalf("%s ist kein bool: %s", k, s)
		}
	}
	for _, k := range []string{"am", "faellig_ab", "frist", "pause_ab"} {
		n, err := strconv.ParseInt(string(ln[k]), 10, 64)
		if err != nil || n < 0 {
			t.Fatalf("%s ist keine nicht-negative ganze Zahl: %s", k, ln[k])
		}
	}
	if string(ln["aktiv"]) != "false" || string(ln["pausiert"]) != "false" || string(ln["am"]) != "4000000000" ||
		string(ln["faellig_ab"]) != strconv.FormatInt(4_000_000_000+300*lnTag, 10) {
		t.Fatalf("vor dem Stichtag: %v", ln)
	}
	if _, da := lies(lnAndere)["lebensnachweis"]; da {
		t.Fatal("Nicht-Mensch bekommt ein lebensnachweis-Objekt")
	}
	// Nach dem Stichtag, Coordinatoren zugelassen: aktiv.
	lebensnachweisAbOverride.Store(1_000)
	if err := json.Unmarshal(lies(lnWallet)["lebensnachweis"], &ln); err != nil || string(ln["aktiv"]) != "true" {
		t.Fatalf("nach dem Stichtag: %v %v", ln, err)
	}
}

// Kein Konto ohne Nachweis aendert sein Blatt -- die Bedingung fuer ein
// Ausrollen ohne StateRoot-Wechsel. Mit Nachweis MUSS es sich aendern.
func TestLebensnachweis_BlattUnveraendertOhneFeld(t *testing.T) {
	acc := &AccountState{Address: "0xABCDEF0000000000000000000000000000000001", Balance: NewDecimal(1049.286126),
		TUsdBalance: NewDecimal(3), LPShares: NewDecimal(0.5), IsHuman: true, FaucetClaimed: true}
	alt := "acct:" + strings.ToLower(acc.Address) + ":" + strconv.FormatInt(acc.Balance.Micro(), 10) +
		":" + strconv.FormatInt(acc.TUsdBalance.Micro(), 10) + ":" + strconv.FormatInt(acc.LPShares.Micro(), 10) +
		":h=true:fc=true"
	if accountLeaf(acc) != sha256.Sum256([]byte(alt)) {
		t.Fatal("Blatt ohne Lebensnachweis hat sich geaendert")
	}
	acc.LebensnachweisAm = 1793491200
	if accountLeaf(acc) == sha256.Sum256([]byte(alt)) {
		t.Fatal("LebensnachweisAm muss ins Blatt eingehen")
	}
	if accountLeaf(acc) != sha256.Sum256([]byte(alt+":ln=1793491200")) {
		t.Fatal("Blatt mit Lebensnachweis: unerwartete Form")
	}
	// Snapshot: ohne Feld kein Schluessel, mit Feld Rundreise.
	j, _ := json.Marshal(&AccountState{Address: "0x1", IsHuman: true})
	if bytes.Contains(j, []byte("lebensnachweis_am")) {
		t.Fatalf("leeres Feld im JSON: %s", j)
	}
	j, _ = json.Marshal(acc)
	var zurueck AccountState
	if err := json.Unmarshal(j, &zurueck); err != nil || zurueck.LebensnachweisAm != 1793491200 || accountLeaf(&zurueck) != accountLeaf(acc) {
		t.Fatalf("Rundreise: %v %+v", err, zurueck)
	}
}

// Der Gutfall: gueltige Bescheinigung, faellig -- LebensnachweisAm wird
// issued_at. Zweimal nachgespielt aendert nichts und meldet nichts.
func TestLebensnachweis_GueltigUndIdempotent(t *testing.T) {
	f := neuerLnFall(t)
	block := lnStichtag + 310*lnTag
	tx := f.tx(lnWallet, block-60)
	for i := 0; i < 2; i++ {
		befund, err := f.anwenden(tx, block)
		if err != nil || len(befund) != 0 {
			t.Fatalf("Lauf %d: %v %v", i, befund, err)
		}
		if f.am(lnWallet) != block-60 {
			t.Fatalf("Lauf %d: am=%d", i, f.am(lnWallet))
		}
	}
	// Ein aelterer (frueher gueltiger) Nachweis setzt nichts zurueck.
	alt := f.tx(lnWallet, block-120)
	if befund, err := f.anwenden(alt, block); err != nil || len(befund) != 0 || f.am(lnWallet) != block-60 {
		t.Fatalf("aelterer Nachweis: %v %v am=%d", befund, err, f.am(lnWallet))
	}
	// Der naechste ist erst 300 Tage spaeter faellig.
	frueh := f.tx(lnWallet, block+10*lnTag)
	if befund, _ := f.anwenden(frueh, block+10*lnTag); befund[lnRegelFrueh] != 1 || f.am(lnWallet) != block-60 {
		t.Fatalf("zweiter Nachweis im selben Jahr: %v am=%d", befund, f.am(lnWallet))
	}
}

// Missbrauch: jede Faelschung wird bei jedem Knoten nicht angewandt (auch im
// Beobachtungsmodus) und gemeldet.
func TestLebensnachweis_Missbrauch(t *testing.T) {
	block := lnStichtag + 310*lnTag
	issued := block - 60
	_, fremdPriv, _ := ed25519.GenerateKey(rand.Reader)
	faelle := []struct {
		name  string
		tx    func(f *lnFall) Transaction
		regel string
		vorab func(f *lnFall)
	}{
		{"gefaelschte Unterschrift", func(f *lnFall) Transaction {
			tx := f.tx(lnWallet, issued)
			tx.Bescheinigung.Signature = hex.EncodeToString(ed25519.Sign(fremdPriv, []byte(lebensnachweisNachricht(lnWallet, issued))))
			return tx
		}, lnRegelOhne, nil},
		{"Erneuerungs-Bescheinigung als Lebensnachweis", func(f *lnFall) Transaction {
			return lebensnachweisTransaktion(lnWallet, issued, f.pubHex, f.signiere(erneuerungsNachricht(lnWallet, issued)), f.bindung)
		}, lnRegelOhne, nil},
		{"fremde Wallet", func(f *lnFall) Transaction {
			tx := f.tx(lnAndere, issued) // fuer lnAndere unterschrieben ...
			tx.Wallet = lnWallet         // ... fuer lnWallet eingereicht
			return tx
		}, lnRegelOhne, nil},
		{"veraenderter Zeitpunkt", func(f *lnFall) Transaction {
			tx := f.tx(lnWallet, issued)
			tx.DistributionAt = issued + 1
			return tx
		}, lnRegelOhne, nil},
		{"ohne Bescheinigung", func(f *lnFall) Transaction {
			return Transaction{Type: lebensnachweisTxArt, Wallet: lnWallet, DistributionAt: issued}
		}, lnRegelOhne, nil},
		{"ohne Bindung", func(f *lnFall) Transaction {
			tx := f.tx(lnWallet, issued)
			tx.Bescheinigung.Mensch, tx.Bescheinigung.MenschSig, tx.Bescheinigung.SchluesselSig = "", "", ""
			return tx
		}, lnRegelOhne, nil},
		{"Selbstbescheinigung", func(f *lnFall) Transaction {
			return f.tx(f.mensch, issued) // der Coordinator-Mensch bescheinigt sich selbst
		}, lnRegelOhne, nil},
		{"Coordinator nicht zugelassen", func(f *lnFall) Transaction { return f.tx(lnWallet, issued) },
			lnRegelOhne, func(f *lnFall) { f.zugelassen = false }},
		{"Coordinator ist kein Mensch", func(f *lnFall) Transaction { return f.tx(lnWallet, issued) },
			lnRegelOhne, func(f *lnFall) { acct(f.cs, f.mensch).IsHuman = false }},
		{"Coordinator mit offener Staffel", func(f *lnFall) Transaction { return f.tx(lnWallet, issued) },
			lnRegelOhne, func(f *lnFall) { acct(f.cs, f.mensch).GrantStagedRest = NewDecimal(800) }},
		{"Wallet gross geschrieben", func(f *lnFall) Transaction {
			tx := f.tx(lnWallet, issued)
			tx.Wallet = "0x" + strings.ToUpper(lnWallet[2:])
			return tx
		}, lnRegelOhne, nil},
		{"zu frueh", func(f *lnFall) Transaction { return f.tx(lnWallet, lnStichtag+299*lnTag) }, lnRegelFrueh, nil},
		{"aus der Zukunft des Blocks", func(f *lnFall) Transaction {
			return f.tx(lnWallet, block+nachweisHoechstensVoraus+1)
		}, lnRegelZeit, nil},
		{"ohne Zeitpunkt", func(f *lnFall) Transaction {
			tx := f.tx(lnWallet, issued)
			tx.DistributionAt = 0
			return tx
		}, lnRegelZeit, nil},
		{"kein Mensch", func(f *lnFall) Transaction {
			w := "0x00000000000000000000000000000000000000e9"
			f.cs.accounts.Set(w, &AccountState{Address: w, Balance: NewDecimal(1)})
			return f.tx(w, issued)
		}, lnRegelKeinMensch, nil},
	}
	for _, fall := range faelle {
		t.Run(fall.name, func(t *testing.T) {
			f := neuerLnFall(t)
			if fall.vorab != nil {
				fall.vorab(f)
			}
			tx := fall.tx(f)
			befund, err := f.anwenden(tx, block)
			if err != nil {
				t.Fatalf("im Beobachtungsmodus kein Fehler erwartet: %v", err)
			}
			if befund[fall.regel] != 1 {
				t.Fatalf("Befund %v, erwartet %s", befund, fall.regel)
			}
			for _, w := range []string{lnWallet, lnAndere, f.mensch} {
				if f.am(w) != 0 {
					t.Fatalf("%s: angewandt (am=%d)", w, f.am(w))
				}
			}
		})
	}
}

// Umgekehrt: eine Lebensnachweis-Bescheinigung gilt nicht als Erneuerung --
// beide Richtungen der Domaenentrennung.
func TestLebensnachweis_KeineErneuerung(t *testing.T) {
	f := neuerLnFall(t)
	issued := lnStichtag + 310*lnTag
	stand := func(m string) (bool, bool) { return m == f.mensch, false }
	zugelassen := func(m string, _ int64) (bool, error) { return m == f.mensch, nil }
	ln := f.signiere(lebensnachweisNachricht(lnErneuert, issued))
	tx := erneuerungsTransaktion(lnErneuert, issued, f.pubHex, ln, f.bindung)
	if err := bescheinigungPruefen(lnErneuert, issued, tx.Bescheinigung, stand, zugelassen); err == nil ||
		!strings.Contains(err.Error(), "Bescheinigung passt nicht") {
		t.Fatalf("Lebensnachweis als Erneuerung: %v", err)
	}
	// Und die gueltige Erneuerung besteht dieselbe Pruefung (Gegenprobe).
	er := erneuerungsTransaktion(lnErneuert, issued, f.pubHex, f.signiere(erneuerungsNachricht(lnErneuert, issued)), f.bindung)
	if err := bescheinigungPruefen(lnErneuert, issued, er.Bescheinigung, stand, zugelassen); err != nil {
		t.Fatalf("Gegenprobe Erneuerung: %v", err)
	}
	if err := bescheinigungPruefenFuer(lebensnachweisNachricht, lnErneuert, issued, er.Bescheinigung, stand, zugelassen); err == nil {
		t.Fatal("Erneuerung als Lebensnachweis bestanden")
	}
}

// Vor dem Stichtag, und danach ohne zugelassene Coordinatoren: Leerlauf,
// auch fuer eine sonst gueltige Bescheinigung -- und kein Befund.
func TestLebensnachweis_LeerlaufVorAktivierung(t *testing.T) {
	f := neuerLnFall(t)
	issued := lnStichtag + 310*lnTag
	tx := f.tx(lnWallet, issued)
	lebensnachweisAbOverride.Store(issued + 3600)
	if befund, err := f.anwenden(tx, issued+60); err != nil || len(befund) != 0 || f.am(lnWallet) != 0 {
		t.Fatalf("vor dem Stichtag: %v %v am=%d", befund, err, f.am(lnWallet))
	}
	lebensnachweisAbOverride.Store(lnStichtag)
	registerLeserOverride.Store(math.MaxInt64) // registerLeserAbUnix = MaxInt64
	if befund, err := f.anwenden(tx, issued+60); err != nil || len(befund) != 0 || f.am(lnWallet) != 0 {
		t.Fatalf("ohne Zulassung der Coordinatoren: %v %v am=%d", befund, err, f.am(lnWallet))
	}
	// Die Bekanntheit der Art im Peer-Block folgt derselben Regel.
	if bekannteTxArt("proof_of_life", issued) {
		t.Fatal("proof_of_life vor registerLeserAb bekannt")
	}
	registerLeserOverride.Store(1)
	if !bekannteTxArt("proof_of_life", issued) || bekannteTxArt("proof_of_life", lnStichtag-1) {
		t.Fatal("proof_of_life: Bekanntheit folgt nicht lebensnachweisAktiv")
	}
}

// Fail-closed: ist die Zulassung nicht lesbar, wird weder angewandt noch
// verworfen -- der Fehler geht nach oben (beim Nachspielen: Block zurueck).
// Im strengen Modus lehnt jeder Befund den Block ab.
func TestLebensnachweis_ZulassungUnlesbarUndStreng(t *testing.T) {
	f := neuerLnFall(t)
	block := lnStichtag + 310*lnTag
	f.cs.zulassungOhneDB = func(string, int64) (bool, error) { return false, fmt.Errorf("Verbindung weg") }
	if _, err := f.anwenden(f.tx(lnWallet, block-60), block); err == nil || !annahmeFehlerIntern(err) || f.am(lnWallet) != 0 {
		t.Fatalf("unlesbare Zulassung: %v am=%d", err, f.am(lnWallet))
	}
	f.cs.zulassungOhneDB = func(m string, _ int64) (bool, error) { return m == f.mensch, nil }
	nachrechnenStrengOverride.Store(1)
	t.Cleanup(func() { nachrechnenStrengOverride.Store(0) })
	if _, err := f.anwenden(f.tx(lnWallet, lnStichtag+10*lnTag), block); err == nil {
		t.Fatal("strenger Modus: zu frueher Nachweis nicht abgelehnt")
	}
	if _, err := f.anwenden(f.tx(lnWallet, block-60), block); err != nil || f.am(lnWallet) != block-60 {
		t.Fatalf("strenger Modus, gueltig: %v", err)
	}
}

// Die Hilfe fuer Tests ohne Datenbank gilt nie neben einer Datenbank.
func TestLebensnachweis_TestZulassungNurOhneDB(t *testing.T) {
	cs := newTestState()
	cs.zulassungOhneDB = func(string, int64) (bool, error) { return true, nil }
	if ok, err := cs.lebensnachweisZulassung(context.Background())("0x1", 1); !ok || err != nil {
		t.Fatal("ohne Datenbank gilt die Testzulassung")
	}
	body := functionBodyFromSource(t, "lebensnachweis.go", "func (cs *ChainState) lebensnachweisZulassung(")
	if !strings.Contains(body, "cs.zulassungOhneDB != nil && cs.db == nil") {
		t.Fatal("die Testzulassung darf nur ohne Datenbank greifen")
	}
}

// ------------------------------------------------------------ Grundeinkommen

// lnTopf: der UBI-Topf mit betrag AEQ.
func lnTopf(cs *ChainState, betrag float64) {
	cs.accounts.Set(ubiPoolAddr, &AccountState{Address: ubiPoolAddr, Balance: NewDecimal(betrag)})
}

// lnRunde: eine Runde wie RunDailyDistributionAtomic (UBI-Teil), mit den
// Transaktionen, die in den Block gehen.
func lnRunde(t *testing.T, cs *ChainState, ubiAt int64) []Transaction {
	t.Helper()
	ctx := context.Background()
	cs.mu.Lock()
	defer cs.mu.Unlock()
	shares, err := cs.distributeUBIPoolLocked(ctx, ubiAt)
	if err != nil {
		t.Fatal(err)
	}
	var txs []Transaction
	var rundenZeit int64
	if lebensnachweisAktiv(ubiAt) {
		rundenZeit = ubiAt
	}
	for _, s := range shares {
		txs = append(txs, Transaction{Type: "ubi_distribution", Wallet: s.Wallet, Amount: s.Amount, FromDemurrageLost: s.DemurrageLost, DistributionAt: rundenZeit})
	}
	if len(shares) > 0 {
		rest := acct(cs, ubiPoolAddr).Balance.Float()
		if err := cs.applyUBIFinalizeDeltaLocked(ctx, ubiAt, rest); err != nil {
			t.Fatal(err)
		}
		txs = append(txs, Transaction{Type: "ubi_distribution_finalize", DistributionAt: ubiAt, Amount: rest})
	}
	if err := cs.applyDistributionRoundMarkerDeltaLocked(ctx, ubiAt); err != nil {
		t.Fatal(err)
	}
	return append(txs, Transaction{Type: "distribution_round_marker", DistributionAt: ubiAt})
}

// Wer ueberfaellig ist, bekommt keinen Tagesanteil -- und der Anteil geht an
// die anderen: der ganze Topf wird auf die Nicht-Pausierten verteilt. Das
// Guthaben des Pausierten und sein Menschsein bleiben unberuehrt.
func TestLebensnachweis_PausierterOhneTagesanteil_AndereBekommenIhn(t *testing.T) {
	f := neuerLnFall(t)
	ubiAt := lnStichtag + 400*lnTag // am = Stichtag fuer alle: pause_ab ist vorbei
	// lnAndere, lnDritte und der Coordinator haben nachgewiesen, lnWallet nicht.
	for _, w := range []string{lnAndere, lnDritte, f.mensch} {
		acct(f.cs, w).LebensnachweisAm = lnStichtag + 310*lnTag
	}
	lnTopf(f.cs, 300)
	vorher := map[string]float64{}
	for _, w := range []string{lnWallet, lnAndere, lnDritte, f.mensch} {
		vorher[w] = stand(f.cs, w)
	}
	txs := lnRunde(t, f.cs, ubiAt)
	if d := stand(f.cs, lnWallet) - vorher[lnWallet]; d != 0 {
		t.Fatalf("Pausierter bekam %.6f AEQ", d)
	}
	if !acct(f.cs, lnWallet).IsHuman || stand(f.cs, lnWallet) != 1000 {
		t.Fatal("Pausierter verlor Menschsein oder Guthaben")
	}
	for _, w := range []string{lnAndere, lnDritte, f.mensch} {
		if d := stand(f.cs, w) - vorher[w]; d != 100 {
			t.Fatalf("%s bekam %.6f statt 100 (300 AEQ auf 3 statt 4 Menschen)", w, d)
		}
	}
	for _, tx := range txs {
		if tx.Type == "ubi_distribution" && (tx.Wallet == lnWallet || tx.DistributionAt != ubiAt) {
			t.Fatalf("Grundeinkommen: %+v", tx)
		}
	}
	if rest := stand(f.cs, ubiPoolAddr); rest > 1e-6 {
		t.Fatalf("Topf nicht verteilt (Anteil einbehalten?): %.6f", rest)
	}

	// Nachweis kommt: ab der naechsten Runde wieder dabei.
	nachweis := ubiAt + 3600
	if befund, err := f.anwenden(f.tx(lnWallet, nachweis), nachweis+5); err != nil || len(befund) != 0 {
		t.Fatalf("Nachweis: %v %v", befund, err)
	}
	lnTopf(f.cs, 400)
	vorher[lnWallet] = stand(f.cs, lnWallet)
	lnRunde(t, f.cs, ubiAt+lnTag)
	if d := stand(f.cs, lnWallet) - vorher[lnWallet]; d != 100 {
		t.Fatalf("nach dem Nachweis: %.6f statt 100", d)
	}
}

// Ohne zugelassene Coordinatoren (registerLeserAb = MaxInt64, wie heute)
// und vor dem Stichtag bekommt jeder seinen Anteil -- auch wer "ueberfaellig"
// waere. Die Runde traegt dann keine Rundenzeit (Bloecke wie bisher).
func TestLebensnachweis_OhneZulassungNiemandPausiert(t *testing.T) {
	f := neuerLnFall(t)
	registerLeserOverride.Store(math.MaxInt64)
	lnTopf(f.cs, 400)
	txs := lnRunde(t, f.cs, lnStichtag+800*lnTag)
	n := 0
	for _, tx := range txs {
		if tx.Type == "ubi_distribution" {
			n++
			if tx.Amount != 100 || tx.DistributionAt != 0 {
				t.Fatalf("Grundeinkommen ohne Zulassung: %+v", tx)
			}
		}
	}
	if n != 4 {
		t.Fatalf("%d Empfaenger statt 4", n)
	}
}

// Sind alle pausiert, bleibt der Topf stehen -- nichts geht verloren.
func TestLebensnachweis_AllePausiertTopfBleibt(t *testing.T) {
	f := neuerLnFall(t)
	lnTopf(f.cs, 400)
	f.cs.mu.Lock()
	shares, err := f.cs.distributeUBIPoolLocked(context.Background(), lnStichtag+400*lnTag)
	f.cs.mu.Unlock()
	if err != nil || len(shares) != 0 || stand(f.cs, ubiPoolAddr) != 400 {
		t.Fatalf("alle pausiert: %v %v Topf %.6f", shares, err, stand(f.cs, ubiPoolAddr))
	}
}

// Erzeuger und Nachspielender: dieselbe Runde, nachgespielt von einem Knoten
// mit demselben Anfangszustand -- keine Abweichung in nachrechnen_ubi.go,
// gleiche Blaetter. Und ein Erzeuger, der den Pausierten doch bedenkt, wird
// bei jedem Nachspielenden gemeldet (ubi_pausiert, ubi_empfaenger,
// ubi_anteil).
func TestLebensnachweis_NachspielenRechnetAusschlussNach(t *testing.T) {
	ubiAt := lnStichtag + 400*lnTag
	welt := func() (*BlockDAG, *lnFall) {
		f := neuerLnFall(t)
		// Beide Welten brauchen dieselben Adressen: der Coordinator-Mensch
		// (zufaellige Adresse je Fall) wird durch eine feste ersetzt -- fuer
		// das Grundeinkommen ist er nur ein weiterer Mensch.
		f.cs.accounts.Delete(f.mensch)
		f.cs.humanCount--
		f.mensch = lnMensch
		addHuman(f.cs, lnMensch, 10)
		dag := newOrphanTestDAG()
		dag.state = f.cs
		dag.bootHeight = 0
		dag.replayedBlocks = make(map[string]bool)
		dag.replayFailures = make(map[string]replayFailureState)
		dag.stateRootMismatches = map[string]int{}
		dag.stateRootMismatchLastAt = map[string]int64{}
		for _, w := range []string{lnAndere, lnDritte, f.mensch} {
			acct(f.cs, w).LebensnachweisAm = lnStichtag + 310*lnTag
		}
		lnTopf(f.cs, 300)
		return dag, f
	}
	regeln := []string{"ubi_pausiert", "ubi_empfaenger", "ubi_anteil", "ubi_kein_mensch", "ubi_ungleich", "ubi_rundenzeit"}
	zaehle := func() map[string]int64 {
		m := map[string]int64{}
		for _, r := range regeln {
			m[r] = erhaltungZaehler(r)
		}
		return m
	}

	_, erzeuger := welt()
	txs := lnRunde(t, erzeuger.cs, ubiAt)
	dag, nachspieler := welt()
	vorher := zaehle()
	dag.replayTransactions(&Block{Height: 1, Hash: "ln-ehrlich", Timestamp: ubiAt + 3, Transactions: txs}, true)
	for r, n := range zaehle() {
		if n != vorher[r] {
			t.Fatalf("ehrliche Runde gemeldet: %s +%d", r, n-vorher[r])
		}
	}
	for _, w := range []string{lnWallet, lnAndere, lnDritte, erzeuger.mensch} {
		e, n := acct(erzeuger.cs, w), acct(nachspieler.cs, w)
		if e.Balance != n.Balance {
			t.Fatalf("%s: Erzeuger %.6f, Nachspielender %.6f", w, e.Balance.Float(), n.Balance.Float())
		}
	}
	for _, w := range []string{lnWallet, lnAndere, lnDritte, lnMensch} {
		if accountLeaf(acct(erzeuger.cs, w)) != accountLeaf(acct(nachspieler.cs, w)) {
			t.Fatalf("%s: Blatt verschieden", w)
		}
	}

	// Boesartiger Erzeuger: alle vier, je 75 AEQ.
	dag2, f2 := welt()
	var boese []Transaction
	for _, w := range []string{lnWallet, lnAndere, lnDritte, f2.mensch} {
		boese = append(boese, Transaction{Type: "ubi_distribution", Wallet: w, Amount: 75, DistributionAt: ubiAt})
	}
	boese = append(boese, Transaction{Type: "ubi_distribution_finalize", DistributionAt: ubiAt, Amount: 0},
		Transaction{Type: "distribution_round_marker", DistributionAt: ubiAt})
	vorher = zaehle()
	dag2.replayTransactions(&Block{Height: 1, Hash: "ln-boese", Timestamp: ubiAt + 3, Transactions: boese}, true)
	nach := zaehle()
	for _, r := range []string{"ubi_pausiert", "ubi_empfaenger", "ubi_anteil"} {
		if nach[r]-vorher[r] != 1 {
			t.Fatalf("Erzeuger bedenkt Pausierten: %s +%d (erwartet 1)", r, nach[r]-vorher[r])
		}
	}
	// Eine erfundene Rundenzeit (weit neben dem Block) zaehlt nicht -- es
	// gilt die Blockzeit.
	dag3, _ := welt()
	weit := append([]Transaction(nil), txs...)
	for i := range weit {
		if weit[i].Type == "ubi_distribution" {
			weit[i].DistributionAt = ubiAt - 10*lnTag
		}
	}
	vorher = zaehle()
	dag3.replayTransactions(&Block{Height: 1, Hash: "ln-weit", Timestamp: ubiAt + 3, Transactions: weit}, true)
	if d := zaehle()["ubi_rundenzeit"] - vorher["ubi_rundenzeit"]; d != 1 {
		t.Fatalf("erfundene Rundenzeit nicht gemeldet (%d)", d)
	}
}

// Die Zaehlung fuer nachrechnen (ohne Datenbank aus dem Speicher) und die
// Auswahl des Erzeugers sind dieselbe Bedingung.
func TestLebensnachweis_ZaehlungWieAuswahl(t *testing.T) {
	f := neuerLnFall(t)
	ubiAt := lnStichtag + 400*lnTag
	acct(f.cs, lnAndere).LebensnachweisAm = lnStichtag + 310*lnTag
	acct(f.cs, f.mensch).LebensnachweisAm = ubiAt - 395*lnTag     // genau an der Grenze: pausiert
	acct(f.cs, lnDritte).LebensnachweisAm = ubiAt - 395*lnTag + 1 // eine Sekunde danach: nicht
	f.cs.mu.Lock()
	defer f.cs.mu.Unlock()
	n, err := f.cs.countUBIEmpfaengerLocked(context.Background(), ubiAt, true)
	if err != nil {
		t.Fatal(err)
	}
	empf, pausiert := f.cs.lebensnachweisEmpfaengerLocked([]string{lnWallet, lnAndere, lnDritte, f.mensch}, ubiAt, true)
	if n != 2 || len(empf) != 2 || pausiert != 2 || empf[0] != lnAndere || empf[1] != lnDritte {
		t.Fatalf("Zaehlung %d, Auswahl %v (%d pausiert)", n, empf, pausiert)
	}
}

// ------------------------------------------------------------ Registrierung

// Nach dem Stichtag beginnt die Uhr mit der Registrierung (RegAt), bei
// Erzeuger und Nachspielendem mit demselben Wert. Vor dem Stichtag und ohne
// RegAt bleibt das Feld 0 (und das Blatt unveraendert).
func TestLebensnachweis_Registrierung(t *testing.T) {
	regAt := nowUnix() - 40
	lnAktiv(t, regAt-100, 1)
	w := "0x00000000000000000000000000000000000000c7"
	tx := Transaction{Type: "register_human", Wallet: w, RegAt: regAt}
	erzeuger := newTestState()
	if err := erzeuger.RegisterHumanAtomic(w, tx); err != nil {
		t.Fatal(err)
	}
	if got := acct(erzeuger, w).LebensnachweisAm; got != regAt {
		t.Fatalf("Erzeuger: LebensnachweisAm=%d, erwartet RegAt %d", got, regAt)
	}
	nachspieler := newTestState()
	blockZeit := regAt + 30
	nachspieler.mu.Lock()
	err := nachspieler.registerHumanMitZeitenLocked(context.Background(), w, blockZeit, staffelRegZeit(tx.RegAt, blockZeit), tx.GrantClass)
	if err == nil {
		err = nachspieler.lebensnachweisBeiRegistrierungLocked(context.Background(), w, tx.RegAt, blockZeit)
	}
	nachspieler.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if accountLeaf(acct(erzeuger, w)) != accountLeaf(acct(nachspieler, w)) {
		t.Fatal("Erzeuger und Nachspielender haben verschiedene Blaetter")
	}
	if f := lebensnachweisFristenVon(acct(erzeuger, w)); f.am != regAt {
		t.Fatalf("am=%d, erwartet die Registrierung", f.am)
	}
	// Stichtag erreicht, aber Coordinatoren noch nicht zugelassen (heute):
	// nichts -- die Blaetter bleiben bis zur Zulassung, wie sie sind.
	registerLeserOverride.Store(math.MaxInt64) // Konstante: Platzhalter
	w3 := "0x00000000000000000000000000000000000000c9"
	ohneZulassung := newTestState()
	if err := ohneZulassung.RegisterHumanAtomic(w3, Transaction{Type: "register_human", Wallet: w3, RegAt: regAt}); err != nil {
		t.Fatal(err)
	}
	if acct(ohneZulassung, w3).LebensnachweisAm != 0 {
		t.Fatal("Registrierung vor der Zulassung setzt LebensnachweisAm (Blatt aendert sich)")
	}
	registerLeserOverride.Store(1)
	// Vor dem Stichtag: nichts.
	lebensnachweisAbOverride.Store(regAt + 100)
	w2 := "0x00000000000000000000000000000000000000c8"
	vorStichtag := newTestState()
	if err := vorStichtag.RegisterHumanAtomic(w2, Transaction{Type: "register_human", Wallet: w2, RegAt: regAt}); err != nil {
		t.Fatal(err)
	}
	if acct(vorStichtag, w2).LebensnachweisAm != 0 {
		t.Fatal("Registrierung vor dem Stichtag setzt LebensnachweisAm")
	}
	// Ohne RegAt (Spiegel-Ersatzweg): nichts -- es gaebe keinen gemeinsamen Wert.
	if lebensnachweisRegZeit(0, regAt+1000) != 0 {
		t.Fatal("ohne RegAt gesetzt")
	}
	// Das Nachspielen nimmt RegAt und die Blockzeit, wie bei der Staffel.
	body := functionBodyFromSource(t, "block.go", "func (dag *BlockDAG) replayTransactions(")
	if !strings.Contains(body, "lebensnachweisBeiRegistrierungLocked(context.Background(), wallet, tx.RegAt, block.Timestamp)") {
		t.Fatal("replayTransactions setzt den Lebensnachweis der Registrierung nicht nach RegAt")
	}
	if !strings.Contains(body, "applyLebensnachweisLocked(context.Background(), &tx, block.Timestamp)") {
		t.Fatal("replayTransactions wendet proof_of_life nicht mit der Pruefung an")
	}
}

// Zweiter Sicherheitsdurchgang #408, MEDIUM-1: kommt eine Registrierung
// mehr als einen Tag nach der Annahme in einen Block (Wiederanlauf,
// RetryRegistrationRecoveries mit altem RegAt), setzen Erzeuger und
// Nachspielender DENSELBEN Wert -- RegAt selbst, ohne Klemme an die eigene
// Uhr.
func TestLebensnachweis_VerspaeteteRegistrierungGleichesBlatt(t *testing.T) {
	jetzt := nowUnix()
	regAt := jetzt - 3*86400
	lnAktiv(t, regAt-100, 1)
	w := "0x00000000000000000000000000000000000000ca"
	tx := Transaction{Type: "register_human", Wallet: w, RegAt: regAt}
	erzeuger := newTestState()
	if err := erzeuger.RegisterHumanAtomic(w, tx); err != nil {
		t.Fatal(err)
	}
	nachspieler := newTestState()
	blockZeit := jetzt + 4
	nachspieler.mu.Lock()
	err := nachspieler.registerHumanMitZeitenLocked(context.Background(), w, blockZeit, staffelRegZeit(tx.RegAt, blockZeit), tx.GrantClass)
	if err == nil {
		err = nachspieler.lebensnachweisBeiRegistrierungLocked(context.Background(), w, tx.RegAt, blockZeit)
	}
	nachspieler.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	e, n := acct(erzeuger, w).LebensnachweisAm, acct(nachspieler, w).LebensnachweisAm
	if e != regAt || n != regAt {
		t.Fatalf("LebensnachweisAm Erzeuger %d, Nachspielender %d, erwartet RegAt %d", e, n, regAt)
	}
	// Die Blaetter: nur das Lebensnachweis-Feld ist Gegenstand; die Staffel
	// schlaeft (2100) und traegt nichts bei.
	if accountLeaf(acct(erzeuger, w)) != accountLeaf(acct(nachspieler, w)) {
		t.Fatal("verspaetete Registrierung: Erzeuger und Nachspielender haben verschiedene Blaetter")
	}
	// Ein RegAt weit nach dem Bezug zaehlt nicht.
	if lebensnachweisRegZeit(blockZeit+v8NachspielKarenz+1, blockZeit) != 0 {
		t.Fatal("vordatiertes RegAt gesetzt")
	}
}

// Zweiter Sicherheitsdurchgang #408, LOW-1: mit nur einem zugelassenen
// Betreiber pausiert keine Runde (sein eigener Mensch koennte nie
// nachweisen); ab zwei schon. Auch die Anzeige folgt dem.
func TestLebensnachweis_PauseNurMitZweiBetreibern(t *testing.T) {
	f := neuerLnFall(t)
	ubiAt := lnStichtag + 400*lnTag // f.mensch ohne Nachweis: ueber pause_ab
	betreiber := 1
	f.cs.betreiberOhneDB = func(int64) int { return betreiber }
	f.cs.mu.Lock()
	moeglich, err := f.cs.lebensnachweisPauseMoeglichLocked(context.Background(), ubiAt)
	if err != nil || moeglich {
		f.cs.mu.Unlock()
		t.Fatalf("ein Betreiber: moeglich=%v (%v)", moeglich, err)
	}
	empf, pausiert := f.cs.lebensnachweisEmpfaengerLocked([]string{f.mensch}, ubiAt, moeglich)
	n, _ := f.cs.countUBIEmpfaengerLocked(context.Background(), ubiAt, moeglich)
	f.cs.mu.Unlock()
	if pausiert != 0 || len(empf) != 1 || n < 1 {
		t.Fatalf("ein Betreiber: %d pausiert, Zaehlung %d", pausiert, n)
	}
	if st := f.cs.LebensnachweisStandVon(f.mensch, ubiAt); st == nil || st["pausiert"] != false {
		t.Fatalf("Anzeige mit einem Betreiber: %v", st)
	}
	betreiber = 2
	f.cs.mu.Lock()
	moeglich, _ = f.cs.lebensnachweisPauseMoeglichLocked(context.Background(), ubiAt)
	_, pausiert = f.cs.lebensnachweisEmpfaengerLocked([]string{f.mensch}, ubiAt, moeglich)
	f.cs.mu.Unlock()
	if !moeglich || pausiert != 1 {
		t.Fatalf("zwei Betreiber: moeglich=%v, %d pausiert", moeglich, pausiert)
	}
	if st := f.cs.LebensnachweisStandVon(f.mensch, ubiAt); st == nil || st["pausiert"] != true {
		t.Fatalf("Anzeige mit zwei Betreibern: %v", st)
	}
}

// ------------------------------------------------------------ Blockzeit

// Der Erzeuger waehlt keine Blockzeit, zu der ein Nachspielender den
// Nachweis nicht anwendet: nicht vor lebensnachweisAktiv, nicht mehr als
// 5 min vor issued_at.
func TestLebensnachweis_Blockzeit(t *testing.T) {
	f := neuerLnFall(t)
	issued := lnStichtag + 310*lnTag
	tx := f.tx(lnWallet, issued)
	if err := blockTauglich(&tx, issued); err != nil {
		t.Fatalf("gueltige Blockzeit: %v", err)
	}
	if err := blockTauglich(&tx, issued-nachweisHoechstensVoraus-1); err == nil {
		t.Fatal("Block mehr als 5 min vor issued_at")
	}
	if err := blockTauglich(&tx, lnStichtag-1); err == nil {
		t.Fatal("Block vor dem Stichtag")
	}
	// Spaetere Annahme: die Blockzeit rueckt nicht vor issued_at - 5 min.
	if bz, err := blockZeitFuer([]Transaction{tx}, issued+30, issued-3600); err != nil || bz != issued+30 {
		t.Fatalf("blockZeitFuer: %d %v", bz, err)
	}
	if _, err := blockZeitFuer([]Transaction{tx}, issued-nachweisHoechstensVoraus-10, issued-7200); err == nil {
		t.Fatal("blockZeitFuer fand eine Zeit vor issued_at - 5 min")
	}
}

// Die Zulassung der Coordinatoren muss spaetestens mit der ersten
// Faelligkeit kommen. Kaeme sie spaeter, waeren beim Einschalten alle schon
// ueber pause_ab und wuerden ohne Vorlauf pausiert -- wer registerLeserAbUnix
// setzt, sieht hier, was er mitentscheidet.
// Kommt die Zulassung der Coordinatoren erst nach der ersten Faelligkeit,
// beginnt die Uhr mit ihr: niemand ist beim Einschalten schon pausiert, und
// jeder hat die vollen Fristen. Der Platzhalter (MaxInt64) verschiebt nichts.
func TestLebensnachweis_UhrBeginntMitDerZulassung(t *testing.T) {
	stichtag := int64(1_800_000_000)
	spaet := stichtag + 500*86400 // nach Stichtag + 395 Tagen
	lnAktiv(t, stichtag, spaet)
	acc := &AccountState{IsHuman: true}
	f := lebensnachweisFristenVon(acc)
	if f.am != spaet || f.faelligAb != spaet+300*86400 || f.pauseAb != spaet+395*86400 {
		t.Fatalf("Fristen ab der Zulassung erwartet, bekommen %+v", f)
	}
	if lebensnachweisPausiert(acc, spaet) || lebensnachweisPausiert(acc, spaet+394*86400) {
		t.Fatal("beim Einschalten der Zulassung darf niemand ohne Vorlauf pausiert sein")
	}
	if !lebensnachweisPausiert(acc, spaet+395*86400) {
		t.Fatal("nach den vollen Fristen ohne Nachweis pausiert")
	}
	// Ein Nachweis nach der Zulassung zaehlt wie immer.
	acc.LebensnachweisAm = spaet + 320*86400
	if lebensnachweisFristenVon(acc).am != acc.LebensnachweisAm {
		t.Fatal("ein spaeterer Nachweis bestimmt am")
	}
	// Platzhalter: es bleibt beim Stichtag.
	lnAktiv(t, stichtag, math.MaxInt64)
	if got := lebensnachweisFristenVon(&AccountState{IsHuman: true}).am; got != stichtag {
		t.Fatalf("Platzhalter verschiebt am auf %d", got)
	}
	// Die Konstante selbst: heute Platzhalter oder spaetestens so, dass die
	// Rechnung oben greift -- es gibt keinen Wert, der ohne Vorlauf pausiert.
	if registerLeserAbUnix != math.MaxInt64 && registerLeserAbUnix >= lebensnachweisUhrMax {
		t.Fatalf("registerLeserAbUnix %d ist weder Platzhalter noch Termin vor 2100", registerLeserAbUnix)
	}
}

// ------------------------------------------------------------ API

// Vor dem Stichtag antwortet schon die Grenze 409 -- ohne zu zaehlen,
// ohne weiterzuleiten. Danach eigene Zaehler: Lebensnachweise verbrauchen
// nicht das Budget der Erneuerungen und umgekehrt.
func TestLebensnachweis_GrenzeEigenerZaehler(t *testing.T) {
	ip := "192.0.2.77"
	leeren := func() {
		ipBurst.Delete("proof-of-life:" + ip)
		ipBurst.Delete("liveness-renewal:" + ip)
	}
	leeren()
	t.Cleanup(leeren)
	a := &APIServer{state: newTestState()}
	durch := 0
	next := func(w http.ResponseWriter, r *http.Request) { durch++ }
	anfrage := func(h http.HandlerFunc) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/proof-of-life", strings.NewReader(`{}`))
		req.RemoteAddr = ip + ":1234"
		h(rec, req)
		return rec.Code
	}
	lnAktiv(t, 4_000_000_000, 1)
	if code := anfrage(a.lebensnachweisGrenze(next)); code != http.StatusConflict || durch != 0 {
		t.Fatalf("vor dem Stichtag: %d (%d durch)", code, durch)
	}
	lnAktiv(t, 1_000, 1)
	for i := 0; i < burstErneuerungJeIP; i++ {
		if code := anfrage(a.lebensnachweisGrenze(next)); code == http.StatusTooManyRequests {
			t.Fatalf("Anfrage %d schon begrenzt", i)
		}
	}
	if code := anfrage(a.lebensnachweisGrenze(next)); code != http.StatusTooManyRequests {
		t.Fatalf("Anfrage %d nicht begrenzt: %d", burstErneuerungJeIP+1, code)
	}
	// Die Erneuerung hat ihr eigenes Budget.
	stagedGrantActivationOverride.Store(1)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	if code := anfrage(a.erneuerungsGrenze(next)); code == http.StatusTooManyRequests {
		t.Fatal("Lebensnachweise haben das Budget der Erneuerung verbraucht")
	}
}

// Der Handler vor jeder Datenbank: 409 vor der Aktivierung, Form, Fenster,
// Unterschrift mit eigener Domaene (eine Erneuerungs-Unterschrift wird
// abgewiesen, bevor das Register gefragt wird), Koerper hoechstens 8 KiB.
func TestLebensnachweis_HandlerVorDerDatenbank(t *testing.T) {
	f := neuerLnFall(t)
	a := &APIServer{state: f.cs}
	reiche := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		a.handleProofOfLife(rec, httptest.NewRequest(http.MethodPost, "/api/proof-of-life", strings.NewReader(body)))
		return rec
	}
	jetzt := nowUnix()
	koerper := func(wallet string, issued int64, sig string) string {
		b, _ := json.Marshal(map[string]interface{}{"wallet": wallet, "issued_at": issued, "signature": sig, "public_key": f.pubHex})
		return string(b)
	}
	lebensnachweisAbOverride.Store(jetzt + 3600)
	if rec := reiche(koerper(lnWallet, jetzt, f.signiere(lebensnachweisNachricht(lnWallet, jetzt)))); rec.Code != http.StatusConflict {
		t.Fatalf("vor dem Stichtag: %d %s", rec.Code, rec.Body.String())
	}
	lebensnachweisAbOverride.Store(lnStichtag)
	faelle := map[string]struct {
		body string
		code int
	}{
		"kaputtes JSON":       {"{", http.StatusBadRequest},
		"zu gross":            {`{"wallet":"` + strings.Repeat("a", 9<<10) + `"}`, http.StatusBadRequest},
		"keine Adresse":       {koerper("0x1234", jetzt, "00"), http.StatusBadRequest},
		"zu alt":              {koerper(lnWallet, jetzt-901, f.signiere(lebensnachweisNachricht(lnWallet, jetzt-901))), http.StatusBadRequest},
		"aus der Zukunft":     {koerper(lnWallet, jetzt+61, f.signiere(lebensnachweisNachricht(lnWallet, jetzt+61))), http.StatusBadRequest},
		"Erneuerungs-Domaene": {koerper(lnWallet, jetzt, f.signiere(erneuerungsNachricht(lnWallet, jetzt))), http.StatusForbidden},
		"fremde Wallet":       {koerper(lnWallet, jetzt, f.signiere(lebensnachweisNachricht(lnAndere, jetzt))), http.StatusForbidden},
	}
	for name, fall := range faelle {
		if rec := reiche(fall.body); rec.Code != fall.code {
			t.Fatalf("%s: %d %s (erwartet %d)", name, rec.Code, rec.Body.String(), fall.code)
		}
	}
	rec := httptest.NewRecorder()
	a.handleProofOfLife(rec, httptest.NewRequest(http.MethodGet, "/api/proof-of-life", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d", rec.Code)
	}
}

// Weiterleitung: der Lebensnachweis belastet das nachgewiesene Konto
// (anfrageKonten) und wird unterschrieben weitergeleitet, damit der
// Zustaendige unter dem Absender zaehlt.
func TestLebensnachweis_Weiterleitung(t *testing.T) {
	wallet := "0x" + strings.Repeat("ab", 20)
	if got := anfrageKonten("/api/proof-of-life", []byte(`{"wallet":" 0x`+strings.Repeat("AB", 20)+` "}`)); len(got) != 1 || got[0] != wallet {
		t.Fatalf("anfrageKonten: %v", got)
	}
	if !weiterleitungUnterschreiben("/api/proof-of-life") || !weiterleitungUnterschreiben("/api/liveness-renewal") {
		t.Fatal("Lebensnachweis wird nicht unterschrieben weitergeleitet")
	}
	if burstKarte(lebensnachweisVonPraefix+"x|1.2.3.4") != erneuerungVon {
		t.Fatal("weitergeleitete Lebensnachweise zaehlen nicht in der begrenzten Karte der Weiterleitungen")
	}
	k, _ := crypto.GenerateKey()
	mux := (&APIServer{state: newTestState(), blockchain: &BlockDAG{signingKey: k}}).buildMux()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/proof-of-life", strings.NewReader(`{}`)))
	if rec.Code != http.StatusConflict { // vor der Zulassung: die Grenze sagt 409
		t.Fatalf("Route /api/proof-of-life: %d %s", rec.Code, rec.Body.String())
	}
}

// Die echte Tagesrunde des Erzeugers (RunDailyDistributionAtomic): der
// Pausierte steht in keinem Grundeinkommen, jedes traegt die Rundenzeit, und
// ein Nachspielender rechnet die Runde ohne Befund nach.
func TestLebensnachweis_TagesrundeDesErzeugers(t *testing.T) {
	ubiAt := lnStichtag + 400*lnTag
	uhr(t, ubiAt)
	f := neuerLnFall(t)
	f.cs.accounts.Delete(f.mensch)
	f.cs.humanCount--
	for _, w := range []string{lnAndere, lnDritte} {
		acct(f.cs, w).LebensnachweisAm = lnStichtag + 310*lnTag
	}
	lnTopf(f.cs, 200)
	var txs []Transaction
	f.cs.ausgangOhneDB = func(tx Transaction) { txs = append(txs, tx) }
	if err := f.cs.RunDailyDistributionAtomic(ubiAt); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, tx := range txs {
		if tx.Type != "ubi_distribution" {
			continue
		}
		n++
		if tx.Wallet == lnWallet || tx.DistributionAt != ubiAt || tx.Amount != 100 {
			t.Fatalf("Grundeinkommen: %+v", tx)
		}
	}
	if n != 2 {
		t.Fatalf("%d Grundeinkommen statt 2: %+v", n, txs)
	}
	// Nachspielen bei einem Knoten mit demselben Anfangszustand.
	dag, cs := newDeterminismTestDAG()
	for _, w := range []string{lnWallet, lnAndere, lnDritte} {
		addHuman(cs, w, 1000)
	}
	for _, w := range []string{lnAndere, lnDritte} {
		acct(cs, w).LebensnachweisAm = lnStichtag + 310*lnTag
	}
	lnTopf(cs, 200)
	regeln := []string{"ubi_pausiert", "ubi_empfaenger", "ubi_anteil", "ubi_rundenzeit", "ubi_kein_mensch"}
	vorher := map[string]int64{}
	for _, r := range regeln {
		vorher[r] = erhaltungZaehler(r)
	}
	if !dag.replayTransactions(&Block{Height: 1, Hash: "ln-runde", Timestamp: ubiAt + 2, Transactions: txs}, true) {
		t.Fatal("ehrliche Runde abgelehnt")
	}
	for _, r := range regeln {
		if d := erhaltungZaehler(r) - vorher[r]; d != 0 {
			t.Fatalf("ehrliche Runde: %s +%d", r, d)
		}
	}
	for _, w := range []string{lnWallet, lnAndere, lnDritte} {
		if acct(cs, w).Balance != acct(f.cs, w).Balance {
			t.Fatalf("%s: Nachspielender %.6f, Erzeuger %.6f", w, stand(cs, w), stand(f.cs, w))
		}
	}
}
