package keeper

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// Offline-Bargeld, Stufe 1b (offline_einbindung.go): die vier Arten an der
// Kette -- Gutfall vom Pfand bis zum Abschluss, Betrug mit Haftung ueber
// Pfand, Konto und Schuld, und je Angriff ein Missbrauchstest. Echte
// P-256-Unterschriften (Geraet) und EIP-191 (Wallet).

type offWelt struct {
	t      *testing.T
	cs     *ChainState
	mk     *ecdsa.PrivateKey
	mensch string
	g      offGeraet
	empf   string
}

func neueOffWelt(t *testing.T, guthaben float64) *offWelt {
	t.Helper()
	offAktiv(t)
	w := &offWelt{t: t, cs: newTestState(), g: neuesOffGeraet(t)}
	w.mk, w.mensch = neuerSchluessel(t)
	addHuman(w.cs, w.mensch, guthaben)
	_, w.empf = neuerSchluessel(t)
	return w
}

func (w *offWelt) stand(a string) int64 {
	if acc, ok := w.cs.accounts.Get(a); ok {
		return acc.Balance.Micro()
	}
	return 0
}

func (w *offWelt) pfandTx(mik, zeit int64) Transaction {
	msg := offlinePfandNachricht(w.mensch, w.g.hex, mik, zeit)
	return Transaction{Type: offlinePfandTxArt, Wallet: w.mensch, DistributionAt: zeit,
		Offline: &OfflineAngaben{Geraet: w.g.hex, Betrag: mik, WalletSig: personalSign(w.t, w.mk, msg)}}
}

func (w *offWelt) kuendigungTx(zeit int64) Transaction {
	msg := offlineKuendigungNachricht(w.mensch, w.g.hex, zeit)
	return Transaction{Type: offlineKuendigungTxArt, Wallet: w.mensch, DistributionAt: zeit,
		Offline: &OfflineAngaben{Geraet: w.g.hex, WalletSig: personalSign(w.t, w.mk, msg)}}
}

func (w *offWelt) zahlung(zaehler int64, an string, mik int64) Transaction {
	return Transaction{Type: offlineEinloesungTxArt, Wallet: an,
		Offline: &OfflineAngaben{Geraet: w.g.hex, Zaehler: zaehler, Betrag: mik, Sig: w.g.zahle(w.t, zaehler, an, mik)}}
}

// urteil: was jeder Knoten zu tx sagt (Regel, "" = gilt).
func (w *offWelt) urteil(tx Transaction, t int64) offlineUrteil {
	w.t.Helper()
	w.cs.mu.Lock()
	defer w.cs.mu.Unlock()
	return w.cs.offlineUrteilLocked(context.Background(), &tx, t)
}

// annehmen: wie der Erzeuger (rechnet die Zahlen) -- gibt die Transaktion
// fuer den Block zurueck.
func (w *offWelt) annehmen(tx Transaction, t int64) Transaction {
	w.t.Helper()
	w.cs.mu.Lock()
	defer w.cs.mu.Unlock()
	u := w.cs.offlineUrteilLocked(context.Background(), &tx, t)
	if u.regel != "" {
		w.t.Fatalf("%s abgewiesen: %s: %s", tx.Type, u.regel, u.text)
	}
	o := *tx.Offline
	o.AusTopf, o.VomKonto, o.Schuld = u.ausTopf, u.vomKonto, u.schuld
	tx.Offline = &o
	if err := w.cs.offlineAnwendenLocked(context.Background(), &tx, u); err != nil {
		w.t.Fatal(err)
	}
	return tx
}

// summeInvariante: der Topf haelt genau die Summe der verfuegbaren Pfaender.
func (w *offWelt) summeInvariante() {
	w.t.Helper()
	var summe int64
	for _, p := range w.cs.offlineJetzt().Pfaender {
		summe += p.verfuegbar()
	}
	if got := w.stand(offlineTopfAddr); got != summe {
		w.t.Fatalf("Topf %d, verfuegbare Pfaender %d", got, summe)
	}
}

func TestOfflineEinbindung_GutfallVomPfandBisZumAbschluss(t *testing.T) {
	w := neueOffWelt(t, 100)
	t0 := offT0
	w.annehmen(w.pfandTx(30_000_000, t0), t0+10)
	if w.stand(w.mensch) != 70_000_000 || w.stand(offlineTopfAddr) != 30_000_000 {
		t.Fatalf("Pfand: Mensch %d, Topf %d", w.stand(w.mensch), w.stand(offlineTopfAddr))
	}
	w.summeInvariante()
	// Zwei Zahlungen an den Empfaenger.
	z1 := w.annehmen(w.zahlung(1, w.empf, 5_000_000), t0+100)
	w.annehmen(w.zahlung(2, w.empf, 7_000_000), t0+200)
	if z1.Offline.AusTopf != 5_000_000 || z1.Offline.VomKonto != 0 || z1.Offline.Schuld != 0 {
		t.Fatalf("Zahlen der Einloesung: %+v", z1.Offline)
	}
	if w.stand(w.empf) != 12_000_000 || w.stand(offlineTopfAddr) != 18_000_000 {
		t.Fatalf("nach Einloesung: Empfaenger %d, Topf %d", w.stand(w.empf), w.stand(offlineTopfAddr))
	}
	w.summeInvariante()
	// Dieselbe Zahlung noch einmal: abgewiesen.
	if u := w.urteil(w.zahlung(1, w.empf, 5_000_000), t0+300); u.regel != "offline_doppelt" {
		t.Fatalf("Wiederholung: %+v", u)
	}
	// Kuendigen, Abschluss erst nach der Frist, in der Tagesrunde.
	w.annehmen(w.kuendigungTx(t0+1000), t0+1010)
	frueh := offlineAbschlussAb(t0+1010) - 1
	w.cs.mu.Lock()
	txs, err := w.cs.offlineAbschluesseLocked(context.Background(), frueh)
	w.cs.mu.Unlock()
	if err != nil || len(txs) != 0 {
		t.Fatalf("Abschluss vor der Frist: %v %v", txs, err)
	}
	w.cs.mu.Lock()
	txs, err = w.cs.offlineAbschluesseLocked(context.Background(), frueh+1)
	w.cs.mu.Unlock()
	if err != nil || len(txs) != 1 {
		t.Fatalf("Abschluss: %v %v", txs, err)
	}
	if got := txs[0].Offline.Auszahlungen; len(got) != 1 || got[0].An != w.mensch || got[0].Mikro != 18_000_000 {
		t.Fatalf("Auszahlungen: %+v", got)
	}
	if w.stand(w.mensch) != 88_000_000 || w.stand(offlineTopfAddr) != 0 {
		t.Fatalf("nach Abschluss: Mensch %d, Topf %d", w.stand(w.mensch), w.stand(offlineTopfAddr))
	}
	if _, aktiv := w.cs.offlineJetzt().Aktiv[w.mensch]; aktiv {
		t.Fatal("Pfand nach Abschluss noch aktiv")
	}
	w.summeInvariante()
}

// Betrug: zweite Zahlung mit demselben Zaehler und Ueberziehen. Der
// Geschaedigte bekommt sein Geld -- aus dem Pfand, dann vom Konto, der Rest
// ist Schuld; der Mensch ist gesperrt.
func TestOfflineEinbindung_BetrugHaftetMitPfandKontoUndSchuld(t *testing.T) {
	w := neueOffWelt(t, 3) // nach dem Pfand nur noch 3 AEQ auf dem Konto
	w.cs.accounts.Get(w.mensch)
	acc, _ := w.cs.accounts.Get(w.mensch)
	acc.Balance = NewDecimal(13)
	t0 := offT0
	w.annehmen(w.pfandTx(10_000_000, t0), t0+10) // Konto jetzt 3 AEQ
	_, b := neuerSchluessel(t)
	w.annehmen(w.zahlung(1, w.empf, 8_000_000), t0+100)
	// Derselbe Zaehler, andere Zahlung, an b: 8 AEQ -- Pfand hat noch 2.
	z := w.annehmen(w.zahlung(1, b, 8_000_000), t0+200)
	if z.Offline.AusTopf != 2_000_000 || z.Offline.VomKonto != 3_000_000 || z.Offline.Schuld != 3_000_000 {
		t.Fatalf("Haftung: %+v", z.Offline)
	}
	if w.stand(b) != 5_000_000 || w.stand(w.mensch) != 0 {
		t.Fatalf("b %d, Mensch %d", w.stand(b), w.stand(w.mensch))
	}
	reg := w.cs.offlineJetzt()
	if !reg.Gesperrt[w.mensch] || len(reg.Schulden[w.mensch]) != 1 || reg.Schulden[w.mensch][0].Mikro != 3_000_000 || reg.Schulden[w.mensch][0].Glaeubiger != b {
		t.Fatalf("Register nach Betrug: %+v", reg)
	}
	w.summeInvariante()
	// Ein Gesperrter legt kein neues Pfand an.
	w.g = neuesOffGeraet(t)
	acc.Balance = NewDecimal(20)
	if u := w.urteil(w.pfandTx(5_000_000, t0+300), t0+310); u.regel != "offline_doppelt" && u.regel != "offline_gesperrt" && u.regel != "offline_schuld" {
		t.Fatalf("neues Pfand nach Betrug: %+v", u)
	}
}

func TestOfflineEinbindung_PfandMissbrauch(t *testing.T) {
	t0 := offT0
	faelle := []struct {
		name  string
		bau   func(w *offWelt) Transaction
		zeit  int64
		regel string
	}{
		{"fremde Wallet unterschreibt", func(w *offWelt) Transaction {
			tx := w.pfandTx(5_000_000, t0)
			fremd, _ := neuerSchluessel(w.t)
			tx.Offline.WalletSig = personalSign(w.t, fremd, offlinePfandNachricht(w.mensch, w.g.hex, 5_000_000, t0))
			return tx
		}, t0 + 10, "offline_unterschrift"},
		{"anderer Betrag unterschrieben", func(w *offWelt) Transaction {
			tx := w.pfandTx(5_000_000, t0)
			tx.Offline.Betrag = 50_000_000
			return tx
		}, t0 + 10, "offline_unterschrift"},
		{"anderes Geraet eingesetzt", func(w *offWelt) Transaction {
			tx := w.pfandTx(5_000_000, t0)
			tx.Offline.Geraet = neuesOffGeraet(w.t).hex
			return tx
		}, t0 + 10, "offline_unterschrift"},
		{"andere Zeit", func(w *offWelt) Transaction {
			tx := w.pfandTx(5_000_000, t0)
			tx.DistributionAt = t0 + 1
			return tx
		}, t0 + 10, "offline_unterschrift"},
		{"zu alt", func(w *offWelt) Transaction { return w.pfandTx(5_000_000, t0) }, t0 + offlineHoechstensAlt + 1, "offline_zeit"},
		{"aus der Zukunft", func(w *offWelt) Transaction { return w.pfandTx(5_000_000, t0+1000) }, t0, "offline_zeit"},
		{"ueber der Grenze", func(w *offWelt) Transaction { return w.pfandTx(offlinePfandMaxMik+1, t0) }, t0 + 10, "offline_grenze"},
		{"mehr als das Guthaben", func(w *offWelt) Transaction { return w.pfandTx(40_000_000, t0) }, t0 + 10, "offline_deckung"},
		{"mit EIP-191 und PQ zugleich", func(w *offWelt) Transaction {
			tx := w.pfandTx(5_000_000, t0)
			tx.Offline.PQ = &PQKonto{Pub: strings.Repeat("ab", 1952), Sig: strings.Repeat("cd", 3309)}
			return tx
		}, t0 + 10, "offline_unterschrift"},
		{"mit Ueberweisungsteilen", func(w *offWelt) Transaction {
			tx := w.pfandTx(5_000_000, t0)
			tx.To = offlineTopfAddr
			return tx
		}, t0 + 10, "offline_form"},
		{"mit Einloesungszahlen", func(w *offWelt) Transaction {
			tx := w.pfandTx(5_000_000, t0)
			tx.Offline.AusTopf = 1
			return tx
		}, t0 + 10, "offline_form"},
	}
	for _, c := range faelle {
		w := neueOffWelt(t, 30)
		if u := w.urteil(c.bau(w), c.zeit); u.regel != c.regel {
			t.Fatalf("%s: %q (%s), erwartet %q", c.name, u.regel, u.text, c.regel)
		}
		if w.stand(offlineTopfAddr) != 0 || w.stand(w.mensch) != 30_000_000 {
			t.Fatalf("%s: Geld bewegt", c.name)
		}
	}
	// Kein Mensch: kein Pfand.
	w := neueOffWelt(t, 30)
	acc, _ := w.cs.accounts.Get(w.mensch)
	acc.IsHuman = false
	if u := w.urteil(w.pfandTx(5_000_000, t0), t0+10); u.regel != "offline_mensch" {
		t.Fatalf("kein Mensch: %+v", u)
	}
	// Ein zweites Pfand nicht; dasselbe Geraet fuer einen anderen Menschen nicht.
	w = neueOffWelt(t, 30)
	w.annehmen(w.pfandTx(5_000_000, t0), t0+10)
	if u := w.urteil(w.pfandTx(5_000_000, t0+20), t0+30); u.regel != "offline_doppelt" {
		t.Fatalf("zweites Pfand: %+v", u)
	}
	zweiter := &offWelt{t: t, cs: w.cs, g: w.g}
	zweiter.mk, zweiter.mensch = neuerSchluessel(t)
	addHuman(w.cs, zweiter.mensch, 30)
	if u := zweiter.urteil(zweiter.pfandTx(5_000_000, t0+20), t0+30); u.regel != "offline_doppelt" {
		t.Fatalf("Geraet eines anderen: %+v", u)
	}
}

func TestOfflineEinbindung_EinloesungMissbrauch(t *testing.T) {
	t0 := offT0
	w := neueOffWelt(t, 30)
	w.annehmen(w.pfandTx(20_000_000, t0), t0+10)
	fremd := neuesOffGeraet(t)
	faelle := []struct {
		name  string
		tx    Transaction
		regel string
	}{
		{"fremdes Geraet unterschreibt", func() Transaction {
			tx := w.zahlung(1, w.empf, 1_000_000)
			tx.Offline.Sig = fremd.zahle(t, 1, w.empf, 1_000_000)
			return tx
		}(), "offline_unterschrift"},
		{"Betrag erhoeht", func() Transaction {
			tx := w.zahlung(1, w.empf, 1_000_000)
			tx.Offline.Betrag = 2_000_000
			return tx
		}(), "offline_unterschrift"},
		{"Empfaenger getauscht", func() Transaction {
			tx := w.zahlung(1, w.empf, 1_000_000)
			_, b := neuerSchluessel(t)
			tx.Wallet = b
			return tx
		}(), "offline_unterschrift"},
		{"Zaehler veraendert", func() Transaction {
			tx := w.zahlung(1, w.empf, 1_000_000)
			tx.Offline.Zaehler = 2
			return tx
		}(), "offline_unterschrift"},
		{"an den Topf", w.zahlung(1, offlineTopfAddr, 1_000_000), "offline_form"},
		{"an den Quantengeld-Topf", w.zahlung(1, quantenTopfAddr, 1_000_000), "offline_form"},
		{"an sich selbst", w.zahlung(1, w.mensch, 1_000_000), "offline_form"},
		{"ueber der Zahlungsgrenze", w.zahlung(1, w.empf, offlineZahlungMaxMik+1), "offline_grenze"},
		{"unbekanntes Geraet", func() Transaction {
			return Transaction{Type: offlineEinloesungTxArt, Wallet: w.empf,
				Offline: &OfflineAngaben{Geraet: fremd.hex, Zaehler: 1, Betrag: 1_000_000, Sig: fremd.zahle(t, 1, w.empf, 1_000_000)}}
		}(), "offline_unbekannt"},
		{"mit Wallet-Unterschrift", func() Transaction {
			tx := w.zahlung(1, w.empf, 1_000_000)
			tx.Offline.WalletSig = "0x" + strings.Repeat("11", 65)
			return tx
		}(), "offline_form"},
	}
	for _, c := range faelle {
		if u := w.urteil(c.tx, t0+100); u.regel != c.regel {
			t.Fatalf("%s: %q (%s), erwartet %q", c.name, u.regel, u.text, c.regel)
		}
	}
	if w.stand(offlineTopfAddr) != 20_000_000 {
		t.Fatal("eine abgewiesene Einloesung hat Geld bewegt")
	}
	// Der Erzeuger nennt andere Zahlen, als die Kette rechnet: der Block ist falsch.
	tx := w.zahlung(1, w.empf, 1_000_000)
	tx.Offline.AusTopf = 1_000_001
	w.cs.mu.Lock()
	err := w.cs.applyOfflineLocked(context.Background(), &tx, t0+100)
	w.cs.mu.Unlock()
	if err == nil || w.stand(w.empf) != 0 {
		t.Fatalf("falsche Zahlen im Block angenommen: %v", err)
	}
	// Mit den richtigen Zahlen (und dem gerechneten Betrag) gilt derselbe Block.
	tx.Offline.AusTopf = 1_000_000
	tx.Amount = 1
	w.cs.mu.Lock()
	err = w.cs.applyOfflineLocked(context.Background(), &tx, t0+100)
	w.cs.mu.Unlock()
	if err != nil || w.stand(w.empf) != 1_000_000 {
		t.Fatalf("richtige Zahlen abgewiesen: %v", err)
	}
}

func TestOfflineEinbindung_KuendigungUndAbschlussMissbrauch(t *testing.T) {
	t0 := offT0
	w := neueOffWelt(t, 30)
	w.annehmen(w.pfandTx(10_000_000, t0), t0+10)
	// Kuendigung mit fremder Unterschrift, fuer ein anderes Geraet.
	tx := w.kuendigungTx(t0 + 20)
	fremd, _ := neuerSchluessel(t)
	tx.Offline.WalletSig = personalSign(t, fremd, offlineKuendigungNachricht(w.mensch, w.g.hex, t0+20))
	if u := w.urteil(tx, t0+30); u.regel != "offline_unterschrift" {
		t.Fatalf("fremde Kuendigung: %+v", u)
	}
	tx = w.kuendigungTx(t0 + 20)
	tx.Offline.Geraet = neuesOffGeraet(t).hex
	if u := w.urteil(tx, t0+30); u.regel != "offline_unbekannt" {
		t.Fatalf("Kuendigung fuer anderes Geraet: %+v", u)
	}
	// Abschluss vor der Kuendigung und vor der Frist: nein.
	ab := Transaction{Type: offlineAbschlussTxArt, Wallet: w.mensch, DistributionAt: t0 + 40, Offline: &OfflineAngaben{Geraet: w.g.hex}}
	if u := w.urteil(ab, t0+40); u.regel != "offline_frist" {
		t.Fatalf("Abschluss ohne Kuendigung: %+v", u)
	}
	w.annehmen(w.kuendigungTx(t0+50), t0+60)
	if u := w.urteil(w.kuendigungTx(t0+70), t0+80); u.regel != "offline_doppelt" {
		t.Fatalf("zweite Kuendigung: %+v", u)
	}
	spaet := offlineAbschlussAb(t0 + 60)
	ab = Transaction{Type: offlineAbschlussTxArt, Wallet: w.mensch, DistributionAt: spaet - 1, Offline: &OfflineAngaben{Geraet: w.g.hex}}
	if u := w.urteil(ab, spaet-1); u.regel != "offline_frist" {
		t.Fatalf("Abschluss vor der Frist: %+v", u)
	}
	// Ein Block mit erfundenen Auszahlungen ist falsch.
	ab = Transaction{Type: offlineAbschlussTxArt, Wallet: w.mensch, DistributionAt: spaet,
		Offline: &OfflineAngaben{Geraet: w.g.hex, Auszahlungen: []OfflineAuszahlung{{An: fremdAdr(), Mikro: 10_000_000}}}}
	w.cs.mu.Lock()
	err := w.cs.applyOfflineLocked(context.Background(), &ab, spaet)
	w.cs.mu.Unlock()
	if err == nil || w.stand(offlineTopfAddr) != 10_000_000 {
		t.Fatalf("erfundene Auszahlung angenommen: %v", err)
	}
	ab.Offline.Auszahlungen = []OfflineAuszahlung{{An: w.mensch, Mikro: 10_000_000}}
	w.cs.mu.Lock()
	err = w.cs.applyOfflineLocked(context.Background(), &ab, spaet)
	w.cs.mu.Unlock()
	if err != nil || w.stand(offlineTopfAddr) != 0 || w.stand(w.mensch) != 30_000_000 {
		t.Fatalf("richtiger Abschluss: %v", err)
	}
}

func fremdAdr() string { return "0x00000000000000000000000000000000000000e7" }

// Vor dem Stichtag: alle Arten unbekannt, kein Block traegt sie, der
// Endpunkt antwortet 409, eine Ueberweisung an den Topf bleibt wie bisher.
func TestOfflineEinbindung_VorDemStichtag(t *testing.T) {
	if offlineAbUnix != math.MaxInt64 {
		t.Fatal("offlineAbUnix muss schlafen, bis Stufe 1b gepruft und freigegeben ist")
	}
	if offlineAktiv(offT0) {
		t.Fatal("ohne Ueberschreibung aktiv")
	}
	for _, art := range []string{offlinePfandTxArt, offlineEinloesungTxArt, offlineKuendigungTxArt, offlineAbschlussTxArt} {
		tx := Transaction{Type: art, Wallet: fremdAdr(), DistributionAt: offT0, Offline: &OfflineAngaben{}}
		if bekannteTxArt(art, offT0) {
			t.Fatalf("%s vor dem Stichtag bekannt", art)
		}
		if blockTauglich(&tx, offT0) == nil {
			t.Fatalf("%s vor dem Stichtag tauglich", art)
		}
		if fe := auftragsFenster(&tx, offT0); fe.von <= fe.bis && fe.von <= offT0 {
			t.Fatalf("%s hat vor dem Stichtag ein Fenster", art)
		}
	}
	plain := Transaction{Type: "transfer", Wallet: fremdAdr(), To: offlineTopfAddr, Amount: 1}
	if err := offlineFeldErlaubt(&plain, offT0); err != nil {
		t.Fatalf("alte Ueberweisung an den Topf vor dem Stichtag abgewiesen: %v", err)
	}
	if s := offlineStatus(); s["ab"] != nil {
		t.Fatalf("Status nennt einen Stichtag: %v", s)
	}
	a := &APIServer{state: newTestState()}
	for _, pfad := range []string{offlinePfandTxArt, offlineEinloesungTxArt, offlineKuendigungTxArt} {
		r := httptest.NewRequest(http.MethodPost, "/api/offline/x", strings.NewReader(`{}`))
		w := httptest.NewRecorder()
		a.handleOffline(pfad)(w, r)
		if w.Code != http.StatusConflict {
			t.Fatalf("%s vor dem Stichtag: %d", pfad, w.Code)
		}
	}
}

// Form im Block: Angaben nur an den Offline-Arten; ab dem Stichtag keine
// gewoehnliche Ueberweisung an den Topf; die Annahme weist sie schon heute ab.
func TestOfflineEinbindung_FeldUndTopf(t *testing.T) {
	offAktiv(t)
	fremd := Transaction{Type: "transfer", Wallet: fremdAdr(), To: "0x00000000000000000000000000000000000000e8", Amount: 1, Offline: &OfflineAngaben{}}
	if offlineFeldErlaubt(&fremd, offT0) == nil {
		t.Fatal("Offline-Angaben an einer Ueberweisung")
	}
	ohne := Transaction{Type: offlineEinloesungTxArt, Wallet: fremdAdr()}
	if offlineFeldErlaubt(&ohne, offT0) == nil {
		t.Fatal("Offline-Art ohne Angaben")
	}
	anTopf := Transaction{Type: "transfer", Wallet: fremdAdr(), To: offlineTopfAddr, Amount: 1}
	if offlineFeldErlaubt(&anTopf, offT0) == nil {
		t.Fatal("Ueberweisung an den Topf ab dem Stichtag")
	}
	if pruefeTopfNurMitNote(strings.ToUpper(offlineTopfAddr[2:]), Transaction{}) == nil && pruefeTopfNurMitNote(offlineTopfAddr, Transaction{}) == nil {
		t.Fatal("Annahme nimmt eine Ueberweisung an den Offline-Topf")
	}
	if !isTokenomicsPoolAddress(offlineTopfAddr) || !istSystemAdresse(offlineTopfAddr) || !istSystemAdresse(quantenTopfAddr) {
		t.Fatal("die Toepfe sind keine Protokoll-Toepfe")
	}
	if !offlineKeinKonto(offlineTopfAddr) || !offlineKeinKonto(quantenTopfAddr) {
		t.Fatal("offlineKeinKonto kennt einen Topf nicht")
	}
	// Abschluss: hoechstens 60 s vor seiner Rundenzeit im Block.
	ab := Transaction{Type: offlineAbschlussTxArt, Wallet: fremdAdr(), DistributionAt: offT0 + 1000, Offline: &OfflineAngaben{}}
	if blockTauglich(&ab, offT0+1000-61) == nil || blockTauglich(&ab, offT0+1000-60) != nil {
		t.Fatal("Fenster des Abschlusses")
	}
}

// Leeres Register: StateRoot byte-gleich; mit Pfand traegt sie den Hash.
// Rueckrollen stellt das Register wieder her.
func TestOfflineEinbindung_StateRootUndRueckroll(t *testing.T) {
	w := neueOffWelt(t, 30)
	if w.cs.offlineJetzt().stateRootTeil() != "" {
		t.Fatal("leeres Register in der StateRoot")
	}
	wurzel := func() string {
		w.cs.mu.RLock()
		defer w.cs.mu.RUnlock()
		return w.cs.stateRootLocked("1", "")
	}
	vorher := wurzel()
	snap := w.cs.snapshotForRollback([]string{w.mensch, offlineTopfAddr}, false)
	w.annehmen(w.pfandTx(10_000_000, offT0), offT0+10)
	if vorher == wurzel() || !strings.Contains(w.cs.offlineJetzt().stateRootTeil(), "|offline:") {
		t.Fatal("Pfand aendert die StateRoot nicht")
	}
	if err := w.cs.restoreFromRollback(snap); err != nil {
		t.Fatal(err)
	}
	if !w.cs.offlineJetzt().leer() || wurzel() != vorher {
		t.Fatal("Rueckrollen stellt das Offline-Register nicht wieder her")
	}
	// Ohne Angaben byte-gleich.
	roh, _ := json.Marshal(Transaction{Type: "transfer"})
	if bytes.Contains(roh, []byte(`"offline"`)) {
		t.Fatal("Transaktion ohne Offline-Angaben enthaelt das Feld")
	}
}

// Befund der eigenen Sicherheitspruefung (Stufe 1b): eine Doppelzahlung --
// derselbe Zaehler, eine andere Zahlung -- wurde nicht als eingeloest
// vermerkt und liess sich beliebig oft einreichen; jede zog erneut vom
// Konto des Halters. Jetzt gilt sie genau einmal.
func TestOfflineEinbindung_DoppelzahlungNurEinmal(t *testing.T) {
	w := neueOffWelt(t, 30)
	w.annehmen(w.pfandTx(10_000_000, offT0), offT0+10)
	w.annehmen(w.zahlung(1, w.empf, 3_000_000), offT0+20)
	_, b := neuerSchluessel(t)
	w.annehmen(w.zahlung(1, b, 3_000_000), offT0+30) // der Beweis: einmal eingeloest
	for i := 0; i < 3; i++ {
		if u := w.urteil(w.zahlung(1, b, 3_000_000), offT0+40); u.regel != "offline_doppelt" {
			t.Fatalf("Doppelzahlung zum %d. Mal: %+v", i+2, u)
		}
	}
	if w.stand(b) != 3_000_000 || w.stand(w.mensch) != 20_000_000 {
		t.Fatalf("b %d, Mensch %d", w.stand(b), w.stand(w.mensch))
	}
	// Auch ein Register, das sie anders verbucht, wird nicht geglaubt.
	reg := w.cs.offlineJetzt()
	p := reg.Pfaender[w.g.hex]
	if len(p.Doppelt) != 1 || !p.Betrug {
		t.Fatalf("Doppel nicht vermerkt: %+v", p)
	}
	kaputt := reg.kopie()
	q := kaputt.pfandAendern(w.g.hex)
	q.Betrug = false
	if _, err := offlineRegisterLesen(kaputt.kodiert()); err == nil {
		t.Fatal("Doppel ohne Betrug gelesen")
	}
	q.Betrug = true
	for d := range q.Doppelt {
		q.Doppelt[d] = 2 // Zaehler ohne erste Zahlung
	}
	if _, err := offlineRegisterLesen(kaputt.kodiert()); err == nil {
		t.Fatal("Doppel zu unbekanntem Zaehler gelesen")
	}
}

// Ein Register, das gilt, aendert keine Regel: jede arbeitet auf einer
// Kopie (geteilte Eintraege werden vor dem Aendern geklont).
func TestOffline_RegisterBleibtUnveraendert(t *testing.T) {
	offAktiv(t)
	g, m, e, f := neuesOffGeraet(t), offAdr(1), offAdr(2), offAdr(3)
	r := offMitPfand(t, g, m, 10_000_000)
	pruefe := func(name string, vorher string, r *offlineRegister) {
		t.Helper()
		if r.kodiert() != vorher {
			t.Fatalf("%s hat das alte Register veraendert", name)
		}
	}
	schritte := []struct {
		name string
		tu   func(r *offlineRegister) (*offlineRegister, error)
	}{
		{"Einloesung", func(r *offlineRegister) (*offlineRegister, error) {
			n, _, err := offlineEinloesen(r, g.hex, 1, e, 4_000_000, g.zahle(t, 1, e, 4_000_000), offT0+1)
			return n, err
		}},
		{"Doppelzahlung", func(r *offlineRegister) (*offlineRegister, error) {
			n, _, err := offlineEinloesen(r, g.hex, 1, f, 8_000_000, g.zahle(t, 1, f, 8_000_000), offT0+2)
			return n, err
		}},
		{"Schuld", func(r *offlineRegister) (*offlineRegister, error) { return offlineSchuldEintragen(r, m, f, 2_000_000) }},
		{"Schuld waechst", func(r *offlineRegister) (*offlineRegister, error) { return offlineSchuldEintragen(r, m, f, 1) }},
		{"Kuendigung", func(r *offlineRegister) (*offlineRegister, error) { return offlineKuendigen(r, m, offT0+3) }},
		{"Abschluss", func(r *offlineRegister) (*offlineRegister, error) {
			n, _, err := offlineAbschliessen(r, m, offlineAbschlussAb(offT0+3))
			return n, err
		}},
		{"Umzug", func(r *offlineRegister) (*offlineRegister, error) { return offlineUmzug(r, m, offAdr(9)) }},
		{"neues Pfand", func(r *offlineRegister) (*offlineRegister, error) {
			return offlinePfandAnlegen(r, offAdr(4), true, neuesOffGeraet(t).hex, 2_000_000, offlineAbschlussAb(offT0+3)+offlineArchivDauer+1)
		}},
	}
	for _, s := range schritte {
		vorher := r.kodiert()
		n, err := s.tu(r)
		if err != nil || n == nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		pruefe(s.name, vorher, r)
		r = n
	}
}

// Das Register als Ganzes ist begrenzt: hoechstens offlinePfaenderMax
// Pfaender; abgelaufenes Archiv faellt beim naechsten neuen Pfand heraus.
func TestOffline_RegisterGrenzeUndArchivAblauf(t *testing.T) {
	offAktiv(t)
	r := &offlineRegister{}
	zeit := offT0
	for i := 0; i < offlinePfaenderMax; i++ {
		var err error
		if r, err = offlinePfandAnlegen(r, offAdr(10+i), true, neuesOffGeraet(t).hex, 1_000_000, zeit); err != nil {
			t.Fatalf("Pfand %d: %v", i, err)
		}
	}
	if _, err := offlinePfandAnlegen(r, offAdr(9), true, neuesOffGeraet(t).hex, 1_000_000, zeit); offRegel(err) != "offline_voll" {
		t.Fatalf("Pfand ueber der Grenze: %v", err)
	}
	// Einer schliesst ab; sein Archiv haelt den Platz bis zum Ablauf.
	m := offAdr(10)
	r, _ = offlineKuendigen(r, m, zeit)
	ab := offlineAbschlussAb(zeit)
	r, _, _ = offlineAbschliessen(r, m, ab)
	if _, err := offlinePfandAnlegen(r, offAdr(9), true, neuesOffGeraet(t).hex, 1_000_000, ab+offlineArchivDauer); offRegel(err) != "offline_voll" {
		t.Fatalf("Archiv vor dem Ablauf freigegeben: %v", err)
	}
	n, err := offlinePfandAnlegen(r, offAdr(9), true, neuesOffGeraet(t).hex, 1_000_000, ab+offlineArchivDauer+1)
	if err != nil || len(n.Pfaender) != offlinePfaenderMax {
		t.Fatalf("nach dem Ablauf: %v, %d Pfaender", err, len(n.Pfaender))
	}
	// Mehr als die Grenze liest niemand ein.
	voll := n.kopie()
	voll.Pfaender[neuesOffGeraet(t).hex] = nil
	for g := range voll.Pfaender {
		if voll.Pfaender[g] == nil {
			x := neuesOffGeraet(t).hex
			delete(voll.Pfaender, g)
			voll.Pfaender[x] = &offlinePfand{Mensch: offAdr(7), Geraet: x, PfandMikro: 1_000_000, Seit: zeit, Abgeschlossen: zeit}
		}
	}
	if _, err := offlineRegisterLesen(voll.kodiert()); err == nil {
		t.Fatal("Register ueber der Grenze gelesen")
	}
}

// Der schlimmste Fall: jedes Pfand mit allen Einloesungen und laengsten
// Zaehlern. So gross wird das Register hoechstens (ohne Sperren und
// Schulden, die je Mensch begrenzt sind).
func TestOffline_RegisterGroesseBegrenzt(t *testing.T) {
	g := neuesOffGeraet(t)
	p := &offlinePfand{Mensch: offAdr(1), Geraet: g.hex, PfandMikro: offlinePfandMaxMik, AusgezahltMikro: offlinePfandMaxMik,
		Seit: offT0, Gekuendigt: offT0, Abgeschlossen: offT0, Betrug: true, Zaehler: map[int64]string{}}
	for i := int64(0); i < offlineZaehlerJePfand; i++ {
		p.Zaehler[offlineZaehlerMax-i] = strings.Repeat("f", 64)
	}
	b, _ := json.Marshal(map[string]*offlinePfand{g.hex: p})
	je := len(b)
	gesamt := je * offlinePfaenderMax
	t.Logf("schlimmster Fall: %d Byte je Pfand, %d Byte fuer %d Pfaender", je, gesamt, offlinePfaenderMax)
	if gesamt > 10<<20 {
		t.Fatalf("Register im schlimmsten Fall %d Byte, mehr als 10 MiB", gesamt)
	}
}

// Form im Block: nur die eigenen Felder, und der Betrag ist der gerechnete.
func TestOfflineEinbindung_StrengeForm(t *testing.T) {
	w := neueOffWelt(t, 30)
	w.annehmen(w.pfandTx(10_000_000, offT0), offT0+10)
	gestopft := []func(tx *Transaction){
		func(tx *Transaction) { tx.Nullifier = "0x" + strings.Repeat("1", 64) },
		func(tx *Transaction) { tx.Gebuehr = 1 },
		func(tx *Transaction) { tx.BuchAt = offT0 },
		func(tx *Transaction) { tx.AmountOut = 1 },
		func(tx *Transaction) { tx.Nachweis = &Auftragsnachweis{} },
		func(tx *Transaction) { tx.PubSignals = []string{"1"} },
	}
	for i, stopf := range gestopft {
		tx := w.zahlung(1, w.empf, 1_000_000)
		stopf(&tx)
		if u := w.urteil(tx, offT0+20); u.regel != "offline_form" {
			t.Fatalf("Fall %d: %+v", i, u)
		}
	}
	tx := w.zahlung(1, w.empf, 1_000_000)
	tx.Offline.AusTopf = 1_000_000
	tx.Amount = 50 // der Erzeuger behauptet mehr
	w.cs.mu.Lock()
	err := w.cs.applyOfflineLocked(context.Background(), &tx, offT0+20)
	w.cs.mu.Unlock()
	if err == nil || w.stand(w.empf) != 0 {
		t.Fatalf("falscher Betrag im Block angenommen: %v", err)
	}
	tx.Amount = 1
	w.cs.mu.Lock()
	err = w.cs.applyOfflineLocked(context.Background(), &tx, offT0+20)
	w.cs.mu.Unlock()
	if err != nil || w.stand(w.empf) != 1_000_000 {
		t.Fatalf("richtiger Betrag abgewiesen: %v", err)
	}
	// Die Wurzel steht fertig im Stand und ist die des Registers.
	if w.cs.offlineWurzel() != w.cs.offlineJetzt().stateRootTeil() || w.cs.offlineWurzel() == "" {
		t.Fatal("zwischengespeicherte Wurzel weicht ab")
	}
}

// Befund der eigenen Sicherheitspruefung: ein Betrueger entkam Sperre und
// Schuld, indem er auf eine neue Wallet umzog (Stufe 3.6), und hatte dort
// ein zweites Pfand. Jetzt ziehen Pfand, Archiv, Sperre und Schulden mit.
func TestOfflineEinbindung_UmzugNimmtSperreUndSchuldMit(t *testing.T) {
	offAktiv(t)
	f := neuerUzFall(t)
	w := &offWelt{t: t, cs: f.cs, mk: f.altKey, mensch: f.alt, g: neuesOffGeraet(t)}
	_, w.empf = neuerSchluessel(t)
	acc, _ := f.cs.accounts.Get(f.alt)
	acc.Balance = NewDecimal(10)
	w.annehmen(w.pfandTx(10_000_000, offT0), offT0+10)
	w.annehmen(w.zahlung(1, w.empf, 8_000_000), offT0+20)
	_, b := neuerSchluessel(t)
	w.annehmen(w.zahlung(1, b, 8_000_000), offT0+30) // Betrug: 2 vom Pfand, 0 vom Konto, 6 Schuld
	reg := f.cs.offlineJetzt()
	if !reg.Gesperrt[f.alt] || len(reg.Schulden[f.alt]) != 1 {
		t.Fatalf("Ausgangslage: %+v", reg)
	}
	f.cs.mu.Lock()
	err := f.cs.umzugVollziehenLocked(context.Background(), uzN, f.alt, f.neu, offT0+40)
	f.cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	reg = f.cs.offlineJetzt()
	if reg.kenntMensch(f.alt) || !reg.Gesperrt[f.neu] || len(reg.Schulden[f.neu]) != 1 ||
		reg.Aktiv[f.neu] != w.g.hex || reg.Pfaender[w.g.hex].Mensch != f.neu {
		t.Fatalf("nach dem Umzug: %+v", reg)
	}
	// Mit der neuen Wallet kein neues Pfand: gesperrt.
	w2 := &offWelt{t: t, cs: f.cs, mk: f.neuKey, mensch: f.neu, g: neuesOffGeraet(t)}
	acc2, _ := f.cs.accounts.Get(f.neu)
	acc2.Balance = NewDecimal(20)
	if u := w2.urteil(w2.pfandTx(5_000_000, offT0+50), offT0+60); u.regel != "offline_gesperrt" {
		t.Fatalf("Pfand nach Umzug: %+v", u)
	}
	// Ein Umzug auf eine Wallet, die im Register steht, ist nicht tauglich.
	fremd := offAdr(77)
	f.cs.mu.Lock()
	_ = f.cs.offlineSetzenLocked(context.Background(), func() *offlineRegister {
		r := f.cs.offlineJetzt().kopie()
		r.Gesperrt[fremd] = true
		return r
	}())
	f.cs.mu.Unlock()
	if grund, err := f.cs.umzugZielTauglichLocked(context.Background(), fremd); err != nil || grund == "" {
		t.Fatalf("Ziel im Offline-Register tauglich: %q %v", grund, err)
	}
	if _, err := offlineUmzug(f.cs.offlineJetzt(), f.neu, fremd); err == nil {
		t.Fatal("Umzug in einen bestehenden Eintrag")
	}
}

// Befund des zweiten Pruefdurchgangs: der Rueckroll eines Blocks
// (blockTouchedAddresses) kannte Topf, Halter und Abschluss-Empfaenger nicht
// -- scheiterte spaeter eine andere Transaktion im Block, blieben ihre Staende
// im Speicher veraendert, waehrend die Datenbank zurueckrollte. Ebenso der
// Quantengeld-Topf bei quanten_einloesen (live).
func TestOfflineEinbindung_RueckrollKenntAlleKonten(t *testing.T) {
	_, empf := neuerSchluessel(t)
	halter, glaeubiger := offAdr(41), offAdr(42)
	block := &Block{Transactions: []Transaction{
		{Type: quantenEinloesenTxArt, Wallet: empf},
		{Type: offlinePfandTxArt, Wallet: halter, Offline: &OfflineAngaben{}},
		{Type: offlineEinloesungTxArt, Wallet: empf, Offline: &OfflineAngaben{VomKonto: 1, Halter: halter}},
		{Type: offlineAbschlussTxArt, Wallet: halter, Offline: &OfflineAngaben{Auszahlungen: []OfflineAuszahlung{{An: glaeubiger, Mikro: 1}}}},
	}}
	addrs, _ := blockTouchedAddresses(block)
	for _, a := range []string{quantenTopfAddr, offlineTopfAddr, halter, glaeubiger, strings.ToLower(empf)} {
		if !slices.Contains(addrs, a) {
			t.Fatalf("%s fehlt im Rueckroll: %v", a, addrs)
		}
	}
}

// Der Halter steht genau dann im Block, wenn sein Konto belastet wird, und
// ist der Halter des Pfands.
func TestOfflineEinbindung_HalterImBlock(t *testing.T) {
	w := neueOffWelt(t, 30)
	w.annehmen(w.pfandTx(5_000_000, offT0), offT0+10)
	w.annehmen(w.zahlung(1, w.empf, 5_000_000), offT0+20)
	// Ueberziehen: 3 AEQ vom Konto des Halters.
	tx := w.zahlung(2, w.empf, 3_000_000)
	tx.Offline.VomKonto, tx.Amount = 3_000_000, 3
	for _, falsch := range []string{"", fremdAdr(), w.empf} {
		k := tx
		o := *tx.Offline
		o.Halter = falsch
		k.Offline = &o
		w.cs.mu.Lock()
		err := w.cs.applyOfflineLocked(context.Background(), &k, offT0+30)
		w.cs.mu.Unlock()
		if err == nil {
			t.Fatalf("Halter %q angenommen", falsch)
		}
	}
	tx.Offline.Halter = w.mensch
	w.cs.mu.Lock()
	err := w.cs.applyOfflineLocked(context.Background(), &tx, offT0+30)
	w.cs.mu.Unlock()
	if err != nil || w.stand(w.mensch) != 22_000_000 {
		t.Fatalf("richtiger Halter: %v, Mensch %d", err, w.stand(w.mensch))
	}
	// Ohne Belastung kein Halter im Block.
	v := neueOffWelt(t, 30)
	v.annehmen(v.pfandTx(5_000_000, offT0), offT0+10)
	ohne := v.zahlung(1, v.empf, 1_000_000)
	ohne.Offline.AusTopf, ohne.Amount, ohne.Offline.Halter = 1_000_000, 1, v.mensch
	v.cs.mu.Lock()
	err = v.cs.applyOfflineLocked(context.Background(), &ohne, offT0+20)
	v.cs.mu.Unlock()
	if err == nil {
		t.Fatal("Halter ohne Belastung angenommen")
	}
	ohne.Offline.Halter = ""
	v.cs.mu.Lock()
	err = v.cs.applyOfflineLocked(context.Background(), &ohne, offT0+20)
	v.cs.mu.Unlock()
	if err != nil {
		t.Fatalf("ohne Halter: %v", err)
	}
	for _, art := range []Transaction{w.pfandTx(5_000_000, offT0+50), w.kuendigungTx(offT0 + 50)} {
		art.Offline.Halter = w.mensch
		if u := w.urteil(art, offT0+60); u.regel != "offline_form" {
			t.Fatalf("%s mit Halter: %+v", art.Type, u)
		}
	}
}

// Die Annahme prueft die Unterschrift vor der Sperre (Vorfilter).
func TestOfflineEinbindung_UnterschriftVorab(t *testing.T) {
	w := neueOffWelt(t, 30)
	gut := w.zahlung(1, w.empf, 1_000_000)
	if !offlineUnterschriftVorab(&gut, offT0) {
		t.Fatal("gute Zahlung abgewiesen")
	}
	falsch := w.zahlung(1, w.empf, 1_000_000)
	falsch.Offline.Betrag = 2_000_000
	if offlineUnterschriftVorab(&falsch, offT0) {
		t.Fatal("geaenderte Zahlung durchgelassen")
	}
	p := w.pfandTx(5_000_000, offT0)
	if !offlineUnterschriftVorab(&p, offT0) {
		t.Fatal("gutes Pfand abgewiesen")
	}
	p.Offline.Betrag = 6_000_000
	if offlineUnterschriftVorab(&p, offT0) {
		t.Fatal("geaendertes Pfand durchgelassen")
	}
	k := w.kuendigungTx(offT0)
	k.Wallet = fremdAdr()
	if offlineUnterschriftVorab(&k, offT0) {
		t.Fatal("Kuendigung fuer fremde Wallet durchgelassen")
	}
	ab := Transaction{Type: offlineAbschlussTxArt, Offline: &OfflineAngaben{}}
	if offlineUnterschriftVorab(&ab, offT0) {
		t.Fatal("Abschluss ueber die Annahme")
	}
}

// Der Endpunkt: Pfand und Einloesung von Anfang bis Ende ueber HTTP, und je
// eine Faelschung, die abgewiesen wird, ohne Geld zu bewegen.
func TestOfflineEinbindung_Endpunkt(t *testing.T) {
	offlineAbOverride.Store(1)
	t.Cleanup(func() { offlineAbOverride.Store(0) })
	w := &offWelt{t: t, cs: newTestState(), g: neuesOffGeraet(t)}
	w.mk, w.mensch = neuerSchluessel(t)
	addHuman(w.cs, w.mensch, 30)
	_, w.empf = neuerSchluessel(t)
	a := &APIServer{state: w.cs}
	post := func(art string, body map[string]interface{}) (int, map[string]interface{}) {
		t.Helper()
		roh, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, "/api/offline/x", bytes.NewReader(roh))
		r.RemoteAddr = "192.0.2.7:1234"
		rec := httptest.NewRecorder()
		a.handleOffline(art)(rec, r)
		var aus map[string]interface{}
		_ = json.Unmarshal(rec.Body.Bytes(), &aus)
		return rec.Code, aus
	}
	jetzt := nowUnix()
	sig := personalSign(t, w.mk, offlinePfandNachricht(w.mensch, w.g.hex, 10_000_000, jetzt))
	// Gefaelscht: anderer Betrag als unterschrieben.
	if code, _ := post(offlinePfandTxArt, map[string]interface{}{"wallet": w.mensch, "geraet": w.g.hex, "betrag": 20_000_000, "zeit": jetzt, "wallet_sig": sig}); code != http.StatusConflict {
		t.Fatalf("gefaelschtes Pfand: %d", code)
	}
	if code, aus := post(offlinePfandTxArt, map[string]interface{}{"wallet": w.mensch, "geraet": w.g.hex, "betrag": 10_000_000, "zeit": jetzt, "wallet_sig": sig}); code != http.StatusOK {
		t.Fatalf("Pfand: %d %v", code, aus)
	}
	if w.stand(offlineTopfAddr) != 10_000_000 {
		t.Fatalf("Topf %d", w.stand(offlineTopfAddr))
	}
	// Unbekanntes Feld: abgewiesen.
	if code, _ := post(offlineEinloesungTxArt, map[string]interface{}{"empfaenger": w.empf, "geraet": w.g.hex, "zaehler": 1, "betrag": 1_000_000, "sig": w.g.zahle(t, 1, w.empf, 1_000_000), "aus_topf": 5}); code != http.StatusBadRequest {
		t.Fatalf("unbekanntes Feld: %d", code)
	}
	// Gefaelscht: anderer Empfaenger als unterschrieben.
	_, b := neuerSchluessel(t)
	if code, _ := post(offlineEinloesungTxArt, map[string]interface{}{"empfaenger": b, "geraet": w.g.hex, "zaehler": 1, "betrag": 1_000_000, "sig": w.g.zahle(t, 1, w.empf, 1_000_000)}); code != http.StatusConflict {
		t.Fatalf("Zahlung an anderen Empfaenger: %d", code)
	}
	code, aus := post(offlineEinloesungTxArt, map[string]interface{}{"empfaenger": w.empf, "geraet": w.g.hex, "zaehler": 1, "betrag": 1_000_000, "sig": w.g.zahle(t, 1, w.empf, 1_000_000)})
	if code != http.StatusOK || aus["aus_topf"] != float64(1_000_000) {
		t.Fatalf("Einloesung: %d %v", code, aus)
	}
	if w.stand(w.empf) != 1_000_000 || w.stand(b) != 0 {
		t.Fatalf("Empfaenger %d, b %d", w.stand(w.empf), w.stand(b))
	}
	// Dieselbe noch einmal: 409.
	if code, _ := post(offlineEinloesungTxArt, map[string]interface{}{"empfaenger": w.empf, "geraet": w.g.hex, "zaehler": 1, "betrag": 1_000_000, "sig": w.g.zahle(t, 1, w.empf, 1_000_000)}); code != http.StatusConflict {
		t.Fatalf("Wiederholung: %d", code)
	}
	// Abschluss gibt es ueber die Annahme nicht.
	if code, _ := post(offlineAbschlussTxArt, map[string]interface{}{"wallet": w.mensch}); code != http.StatusNotFound {
		t.Fatalf("Abschluss ueber HTTP: %d", code)
	}
}
