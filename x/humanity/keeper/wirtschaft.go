package keeper

// Aequitas fuer Unternehmen (docs/UNTERNEHMEN_KONZEPT.md, Entscheidung 25.09.2026).
//
// DREI KONTOARTEN. Mensch (verifiziert), Unternehmen (von 1-10 Menschen
// eroeffnet, Register unten) und freie Adresse (alles andere ausser den
// Protokoll-Toepfen). Unternehmen haben keine 25.000-Grenze, zahlen aber
// Liegegeld auf Geld, das bei ihnen liegen bleibt. Freie Adressen duerfen
// hoechstens 1.000 AEQ halten und zahlen 1 %/Monat. Menschen zahlen auf
// Erspartes erst ueber 5.000 AEQ etwas.
//
// WAS KONSENS IST UND WAS BUCHFUEHRUNG. Konsens ist nur das Register der
// Unternehmen: es entscheidet, ob die Vermoegensgrenze greift
// (enforceWealthCapLockedCtx), und wird deshalb auf jedem Knoten aus den
// Transaktionen unternehmen_* gleich aufgebaut. Alles andere -- das Alter
// des Geldes, die Monatszaehler fuer die Freibetraege -- ist Buchfuehrung
// des erzeugenden Knotens. Die BETRAEGE, die daraus folgen (Gebuehr,
// Ausstiegsabgabe, Liegegeld), stehen in den Transaktionen, und jeder
// nachspielende Knoten wendet genau sie an. Dasselbe Modell wie die
// Demurrage (FromDemurrageLost): der Erzeuger rechnet, die Kette traegt das
// Ergebnis. Die Buchfuehrung wird auch beim Nachspielen mitgefuehrt, damit
// ein anderer Knoten, der die Erzeugung uebernimmt, dieselben Zahlen hat.
//
// DAS ALTER DES GELDES. Jedes Konto fuehrt seine AEQ in Paketen: Betrag,
// "Seit" (seit wann das Geld unterwegs ist, ohne bei einem Menschen
// angekommen zu sein) und "Ankunft" (wann es auf diesem Konto ankam). Das
// Alter reist mit dem Geld. Neu wird es nur, wenn es mindestens 30 Tage bei
// einem Menschen lag oder frisch entsteht (Grundeinkommen, Registrierung,
// Einstieg aus tUSD). Unternehmen geben das aelteste Geld zuerst aus,
// Menschen das neueste -- so muss Geld, das ein Unternehmen ueber einen
// Freund "waschen" will, wirklich 30 Tage bei ihm liegen.
//
// AKTIVIERUNG. Nichts davon wirkt vor wirtschaftAktivAbUnix. Danach laufen
// alle Ueberweisungen ueber transferMutateLocked (die schnellen Pfade
// treten zurueck), damit die Regeln an genau einer Stelle stehen.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// 2026-10-01T00:00:00Z
	wirtschaftAktivAbUnix int64 = 1790812800

	// Menschen (Fairness-Garantie, Konzept Abschnitt 3)
	menschFreiAusgabenMonat = 1000.0 // gebuehrenfreie Ausgaben je Monat
	menschTauschFreiMonat   = 1000.0 // Umtausch ohne Abgabe je Monat
	lohnTauschFreiMonat     = 3000.0 // erhaltener Lohn, zusaetzlich tauschbar
	menschSparFreibetrag    = 5000.0 // darunter keine Umlaufsicherung
	menschUmlaufMonat       = 0.005  // 0,5 %/Monat auf den Teil darueber
	menschReifSekunden      = 30 * 86400

	// Freie Adressen
	freiGrenze      = 1000.0
	freiUmlaufMonat = 0.01

	// Unternehmen
	unternehmenSockel       = 2000.0
	liegeStufe1Sekunden     = 30 * 86400
	liegeStufe2Sekunden     = 90 * 86400
	liegeRate1Monat         = 0.01
	liegeRate2Monat         = 0.03
	maxUnternehmenJeMensch  = 3
	maxVerantwortlicheJeUnt = 10

	// Ausstieg AEQ -> Stable
	ausstiegsAbgabeBps = 200 // 2 %

	sekundenJeMonat = 30 * 86400
)

var unternehmenKategorien = map[string]bool{
	"lebensmittel": true, "gastronomie": true, "handel": true, "handwerk": true,
	"dienstleistung": true, "gesundheit": true, "bildung": true, "kultur": true,
	"verein": true, "landwirtschaft": true, "technik": true, "sonstiges": true,
}

// wirtschaftAktivOverride: nur fuer Tests (0 = Konstante gilt).
var wirtschaftAktivOverride atomic.Int64

func wirtschaftAktiv(unix int64) bool {
	a := wirtschaftAktivAbUnix
	if o := wirtschaftAktivOverride.Load(); o != 0 {
		a = o
	}
	return unix >= a
}

type kontoart int

const (
	artFrei kontoart = iota
	artMensch
	artUnternehmen
	artSystem
)

func (k kontoart) String() string {
	switch k {
	case artMensch:
		return "mensch"
	case artUnternehmen:
		return "unternehmen"
	case artSystem:
		return "system"
	}
	return "frei"
}

func istSystemAdresse(addr string) bool {
	addr = strings.ToLower(addr)
	return isTokenomicsPoolAddress(addr) || addr == strings.ToLower(V7_CONTRACT_ADDR) ||
		strings.HasPrefix(addr, "0x00000000000000000000000000000000000000")
}

// ------------------------------------------------------------ Zustand

type unternehmenEintrag struct {
	Adresse         string   `json:"adresse"`
	Name            string   `json:"name"`
	Kategorie       string   `json:"kategorie"`
	Verantwortliche []string `json:"verantwortliche"`
	EroeffnetAm     int64    `json:"eroeffnet_am"`
	GeschlossenAm   int64    `json:"geschlossen_am,omitempty"`
}

func (e *unternehmenEintrag) offen() bool { return e != nil && e.GeschlossenAm == 0 }

func (e *unternehmenEintrag) istVerantwortlich(mensch string) bool {
	for _, v := range e.Verantwortliche {
		if v == mensch {
			return true
		}
	}
	return false
}

type paket struct {
	Betrag  float64 `json:"b"`
	Seit    int64   `json:"s"`
	Ankunft int64   `json:"a"`
}

type buchKonto struct {
	Pakete []paket `json:"p,omitempty"`
	Monat  int     `json:"m,omitempty"` // JJJJMM der Zaehler
	// Menschen
	Ausgegeben float64 `json:"aus,omitempty"`
	Getauscht  float64 `json:"tau,omitempty"`
	Lohn       float64 `json:"lohn,omitempty"`
	// Unternehmen (oeffentliche Monatssummen)
	Einnahmen   float64 `json:"ein,omitempty"`
	LohnGezahlt float64 `json:"lgz,omitempty"`
	Entnahmen   float64 `json:"ent,omitempty"`
}

type wirtschaft struct {
	mu          sync.Mutex
	unternehmen map[string]*unternehmenEintrag
	buch        map[string]*buchKonto
	schmutzig   map[string]bool
	// letzter Umlauf-Durchlauf (Blockzeit), fuer die Laenge des Zeitraums
	letzterUmlauf int64
	flusher       sync.Once
}

func neueWirtschaft() *wirtschaft {
	return &wirtschaft{
		unternehmen: map[string]*unternehmenEintrag{},
		buch:        map[string]*buchKonto{},
		schmutzig:   map[string]bool{},
	}
}

func (cs *ChainState) wirt() *wirtschaft {
	if w := cs.wirtschaftP.Load(); w != nil {
		return w
	}
	cs.wirtschaftP.CompareAndSwap(nil, neueWirtschaft())
	return cs.wirtschaftP.Load()
}

func monatVon(unix int64) int {
	t := time.Unix(unix, 0).UTC()
	return t.Year()*100 + int(t.Month())
}

// kontoLocked: Buchkonto, Zaehler auf den laufenden Monat gebracht. w.mu gehalten.
func (w *wirtschaft) kontoLocked(addr string, jetzt int64) *buchKonto {
	k := w.buch[addr]
	if k == nil {
		k = &buchKonto{}
		w.buch[addr] = k
	}
	if m := monatVon(jetzt); k.Monat != m {
		k.Monat = m
		k.Ausgegeben, k.Getauscht, k.Lohn = 0, 0, 0
		k.Einnahmen, k.LohnGezahlt, k.Entnahmen = 0, 0, 0
	}
	return k
}

func (w *wirtschaft) markiere(addr string) { w.schmutzig[addr] = true }

func (w *wirtschaft) offenesUnternehmenLocked(addr string) *unternehmenEintrag {
	if e := w.unternehmen[addr]; e.offen() {
		return e
	}
	return nil
}

// istUnternehmen: offenes Unternehmenskonto? (Konsens: nur aus dem Register.)
func (w *wirtschaft) istUnternehmen(addr string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.offenesUnternehmenLocked(strings.ToLower(addr)) != nil
}

func (w *wirtschaft) unternehmenVon(mensch string) []string {
	var r []string
	for a, e := range w.unternehmen {
		if e.offen() && e.istVerantwortlich(mensch) {
			r = append(r, a)
		}
	}
	sort.Strings(r)
	return r
}

func (cs *ChainState) kontoartVon(addr string, istMensch bool) kontoart {
	addr = strings.ToLower(addr)
	switch {
	case istSystemAdresse(addr):
		return artSystem
	case istMensch:
		return artMensch
	case cs.wirt().istUnternehmen(addr):
		return artUnternehmen
	}
	return artFrei
}

// ------------------------------------------------------------ Pakete

func summePakete(p []paket) float64 {
	var s float64
	for _, x := range p {
		s += x.Betrag
	}
	return s
}

// entnehmen nimmt betrag aus den Paketen: Menschen das neueste zuerst,
// alle anderen das aelteste zuerst. Liefert die entnommenen Stuecke.
func entnehmen(k *buchKonto, betrag float64, mensch bool) []paket {
	if betrag <= 0 {
		return nil
	}
	if mensch {
		sort.SliceStable(k.Pakete, func(i, j int) bool { return k.Pakete[i].Ankunft < k.Pakete[j].Ankunft })
	} else {
		sort.SliceStable(k.Pakete, func(i, j int) bool { return k.Pakete[i].Seit < k.Pakete[j].Seit })
	}
	var out []paket
	rest := betrag
	for rest > 1e-9 && len(k.Pakete) > 0 {
		idx := 0
		if mensch {
			idx = len(k.Pakete) - 1
		}
		p := &k.Pakete[idx]
		n := math.Min(p.Betrag, rest)
		out = append(out, paket{Betrag: n, Seit: p.Seit, Ankunft: p.Ankunft})
		p.Betrag -= n
		rest -= n
		if p.Betrag <= 1e-9 {
			k.Pakete = append(k.Pakete[:idx], k.Pakete[idx+1:]...)
		}
	}
	return out
}

// abgleichen bringt die Summe der Pakete auf den Kontostand: Zufluesse, die
// an keiner Buchungsstelle vorbeikamen (Grundeinkommen, Registrierung,
// Einstieg aus tUSD), sind frisch; Abfluesse ohne Buchungsstelle (Gebuehren,
// Abgaben) gehen in der Ausgabereihenfolge ab.
func abgleichen(k *buchKonto, stand float64, mensch bool, jetzt int64) {
	diff := stand - summePakete(k.Pakete)
	switch {
	case diff > 1e-6:
		k.Pakete = append(k.Pakete, paket{Betrag: diff, Seit: jetzt, Ankunft: jetzt})
	case diff < -1e-6:
		entnehmen(k, -diff, mensch)
	}
}

// verdichten haelt die Paketliste klein: gleicher Tag (Seit, Ankunft)
// verschmilzt; bei Unternehmen ist alles ueber 90 Tage ein Paket, bei
// Menschen alles, was laenger als 30 Tage da ist.
func verdichten(k *buchKonto, mensch bool, jetzt int64) {
	type schluessel struct{ s, a int64 }
	m := map[schluessel]*paket{}
	var reihenfolge []schluessel
	for _, p := range k.Pakete {
		if p.Betrag <= 1e-9 {
			continue
		}
		s := schluessel{p.Seit / 86400, p.Ankunft / 86400}
		if mensch && jetzt-p.Ankunft >= menschReifSekunden {
			s = schluessel{-1, -1}
		}
		if !mensch && jetzt-p.Seit > liegeStufe2Sekunden {
			s = schluessel{-2, -2}
		}
		if q, ok := m[s]; ok {
			q.Betrag += p.Betrag
			if p.Seit < q.Seit {
				q.Seit = p.Seit
			}
			if p.Ankunft < q.Ankunft {
				q.Ankunft = p.Ankunft
			}
			continue
		}
		cp := p
		m[s] = &cp
		reihenfolge = append(reihenfolge, s)
	}
	k.Pakete = k.Pakete[:0]
	for _, s := range reihenfolge {
		k.Pakete = append(k.Pakete, *m[s])
	}
}

// ------------------------------------------------------------ Gebuehren

// gebuehrMitWirtschaft ersetzt ueberweisungsGebuehrFuer ab der Aktivierung:
//   - Mensch: die ersten 1.000 AEQ Ausgaben im Monat gebuehrenfrei, danach
//     wie bisher (0,1 % + Aufschlag nach Guthaben).
//   - Unternehmen -> Mensch: 0 (Lohn, Entnahme, Erstattung).
//   - Unternehmen -> sonst: 0,1 % ohne Aufschlag.
//   - freie Adresse: wie bisher.
func (cs *ChainState) gebuehrMitWirtschaft(from, to string, fromArt, toArt kontoart, amount, guthaben float64, jetzt int64) float64 {
	if !wirtschaftAktiv(jetzt) {
		return ueberweisungsGebuehrFuer(amount, guthaben)
	}
	switch fromArt {
	case artMensch:
		w := cs.wirt()
		w.mu.Lock()
		frei := math.Max(0, menschFreiAusgabenMonat-w.kontoLocked(from, jetzt).Ausgegeben)
		w.mu.Unlock()
		return ueberweisungsGebuehrFuer(math.Max(0, amount-frei), guthaben)
	case artUnternehmen:
		if toArt == artMensch {
			return 0
		}
		return round6(amount * float64(ueberweisungsGebuehrBps) / 10_000)
	}
	return ueberweisungsGebuehrFuer(amount, guthaben)
}

// pruefeEmpfaengerWirtschaft: eine freie Adresse haelt hoechstens 1.000 AEQ.
// Nur bei der Annahme -- das Nachspielen bestehender Bloecke aendert sich nicht.
func pruefeEmpfaengerWirtschaft(toArt kontoart, standVorher, zufluss float64, jetzt int64) error {
	if !wirtschaftAktiv(jetzt) || toArt != artFrei {
		return nil
	}
	if standVorher+zufluss > freiGrenze+1e-9 {
		return fmt.Errorf("recipient is a free address and may hold at most %.0f AEQ (Empfaenger ist eine freie Adresse: hoechstens %.0f AEQ; fuer mehr ein Unternehmenskonto eroeffnen)", freiGrenze, freiGrenze)
	}
	return nil
}

// nachUeberweisung fuehrt Alter und Monatszaehler nach einer Ueberweisung.
// Staende sind die NACH der Ueberweisung. Buchfuehrung, kein Konsens.
func (cs *ChainState) nachUeberweisung(from, to string, fromArt, toArt kontoart, amount, gebuehr, fromNach, toNach float64, jetzt int64) {
	if !wirtschaftAktiv(jetzt) || fromArt == artSystem || toArt == artSystem {
		return
	}
	w := cs.wirt()
	w.mu.Lock()
	defer w.mu.Unlock()
	fk := w.kontoLocked(from, jetzt)
	tk := w.kontoLocked(to, jetzt)
	fMensch, tMensch := fromArt == artMensch, toArt == artMensch
	abgleichen(fk, fromNach+amount+gebuehr, fMensch, jetzt)
	abgleichen(tk, math.Max(0, toNach-amount), tMensch, jetzt)

	stuecke := entnehmen(fk, amount, fMensch)
	entnehmen(fk, gebuehr, fMensch)
	for _, s := range stuecke {
		if fMensch && jetzt-s.Ankunft >= menschReifSekunden {
			s.Seit = jetzt // einen Monat beim Menschen: es war sein Geld
		}
		s.Ankunft = jetzt
		tk.Pakete = append(tk.Pakete, s)
	}
	abgleichen(fk, fromNach, fMensch, jetzt)
	abgleichen(tk, toNach, tMensch, jetzt)
	verdichten(fk, fMensch, jetzt)
	verdichten(tk, tMensch, jetzt)

	if fMensch {
		fk.Ausgegeben += amount
	}
	if fromArt == artUnternehmen {
		if tMensch {
			if e := w.unternehmen[from]; e != nil && e.istVerantwortlich(to) {
				fk.Entnahmen += amount
			} else {
				fk.LohnGezahlt += amount
				tk.Lohn += amount
			}
		}
	}
	if toArt == artUnternehmen {
		tk.Einnahmen += amount
	}
	w.markiere(from)
	w.markiere(to)
	cs.startBuchFlusher()
}

// ausstiegsAbgabe: 2 % auf AEQ -> Stable. Menschen tauschen erhaltenen Lohn
// (bis 3.000/Monat) und 1.000 AEQ im Monat ohne Abgabe.
func (cs *ChainState) ausstiegsAbgabe(addr string, art kontoart, amountIn float64, jetzt int64) float64 {
	if !wirtschaftAktiv(jetzt) || amountIn <= 0 {
		return 0
	}
	pflichtig := amountIn
	if art == artMensch {
		w := cs.wirt()
		w.mu.Lock()
		k := w.kontoLocked(addr, jetzt)
		frei := menschTauschFreiMonat + math.Min(k.Lohn, lohnTauschFreiMonat) - k.Getauscht
		w.mu.Unlock()
		pflichtig = math.Max(0, amountIn-math.Max(0, frei))
	}
	if art == artSystem {
		return 0
	}
	return round6(pflichtig * ausstiegsAbgabeBps / 10_000)
}

func (cs *ChainState) nachTausch(addr string, amountIn float64, jetzt int64) {
	if !wirtschaftAktiv(jetzt) {
		return
	}
	w := cs.wirt()
	w.mu.Lock()
	w.kontoLocked(addr, jetzt).Getauscht += amountIn
	w.markiere(addr)
	w.mu.Unlock()
	cs.startBuchFlusher()
}

// ------------------------------------------------------------ Umlauf (taeglich)

// umlaufBetrag: was ein Konto fuer den Zeitraum sekunden schuldet.
func (cs *ChainState) umlaufBetrag(addr string, art kontoart, stand float64, jetzt, sekunden int64) float64 {
	if stand <= 0 || sekunden <= 0 {
		return 0
	}
	anteil := float64(sekunden) / sekundenJeMonat
	switch art {
	case artMensch:
		return round6(math.Max(0, stand-menschSparFreibetrag) * menschUmlaufMonat * anteil)
	case artFrei:
		return round6(stand * freiUmlaufMonat * anteil)
	case artUnternehmen:
		w := cs.wirt()
		w.mu.Lock()
		defer w.mu.Unlock()
		k := w.kontoLocked(addr, jetzt)
		abgleichen(k, stand, false, jetzt)
		verdichten(k, false, jetzt)
		p := append([]paket(nil), k.Pakete...)
		sort.Slice(p, func(i, j int) bool { return p[i].Seit < p[j].Seit })
		sockel := unternehmenSockel
		var schuld float64
		for _, x := range p {
			b := x.Betrag
			if sockel > 0 {
				n := math.Min(sockel, b)
				sockel -= n
				b -= n
			}
			if b <= 0 {
				continue
			}
			switch alter := jetzt - x.Seit; {
			case alter > liegeStufe2Sekunden:
				schuld += b * liegeRate2Monat * anteil
			case alter > liegeStufe1Sekunden:
				schuld += b * liegeRate1Monat * anteil
			}
		}
		return round6(schuld)
	}
	return 0
}

// umlaufLocked berechnet die Umlauf-Transaktionen eines Tagesdurchlaufs und
// wendet sie an. Reihenfolge nach Adresse, damit jeder Knoten dieselbe
// Folge sieht. Vor der Aktivierung leer. Caller haelt cs.mu.
func (cs *ChainState) umlaufLocked(ctx context.Context, at int64) ([]Transaction, error) {
	if !wirtschaftAktiv(at) {
		return nil, nil
	}
	w := cs.wirt()
	w.mu.Lock()
	sekunden := int64(86400)
	if w.letzterUmlauf > 0 {
		sekunden = at - w.letzterUmlauf
	}
	w.mu.Unlock()
	if sekunden <= 0 {
		return nil, nil
	}
	if sekunden > 7*86400 {
		sekunden = 7 * 86400
	}

	kandidaten := map[string]bool{}
	if cs.db != nil {
		rows, err := cs.dbExecCtx(ctx).Query(
			`SELECT lower(address) FROM chain_accounts
			 WHERE (is_human = true AND balance > $1) OR (is_human = false AND balance > 0)
			 ORDER BY 1`, menschSparFreibetrag)
		if err != nil {
			return nil, fmt.Errorf("umlauf: %w", err)
		}
		for rows.Next() {
			var a string
			if rows.Scan(&a) == nil {
				kandidaten[a] = true
			}
		}
		rows.Close()
	} else {
		cs.accounts.Range(func(a string, acc *AccountState) bool {
			if acc.Balance.Float() > 0 {
				kandidaten[strings.ToLower(a)] = true
			}
			return true
		})
	}
	adressen := make([]string, 0, len(kandidaten))
	for a := range kandidaten {
		if !istSystemAdresse(a) {
			adressen = append(adressen, a)
		}
	}
	sort.Strings(adressen)

	var txs []Transaction
	for _, a := range adressen {
		cs.ensureAccountLoadedCtx(ctx, a)
		acc, ok := cs.accounts.Get(a)
		if !ok {
			continue
		}
		art := cs.kontoartVon(a, acc.IsHuman)
		betrag := cs.umlaufBetrag(a, art, acc.Balance.Float(), at, sekunden)
		if betrag <= 0 {
			continue
		}
		if err := cs.applyUmlaufDeltaLocked(ctx, a, betrag, at); err != nil {
			return nil, err
		}
		txs = append(txs, Transaction{Type: "umlauf", Wallet: a, Amount: betrag, DistributionAt: at})
	}
	w.mu.Lock()
	w.letzterUmlauf = at
	w.mu.Unlock()
	cs.speichereLetztenUmlauf(ctx, at)
	return txs, nil
}

// applyUmlaufDeltaLocked: Betrag vom Konto ins Grundeinkommen. Auf das
// Guthaben gedeckelt -- ein Block nimmt nie mehr, als da ist. Caller haelt cs.mu.
func (cs *ChainState) applyUmlaufDeltaLocked(ctx context.Context, wallet string, amount float64, at int64) error {
	wallet = strings.ToLower(strings.TrimSpace(wallet))
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return fmt.Errorf("umlauf %s: Betrag %v: %w", wallet, amount, ErrZustandLehntAb)
	}
	if istSystemAdresse(wallet) {
		return fmt.Errorf("umlauf %s: Systemadresse: %w", wallet, ErrZustandLehntAb)
	}
	cs.ensureAccountLoadedCtx(ctx, wallet)
	acc, ok := cs.accounts.Get(wallet)
	if !ok {
		return nil
	}
	b := NewDecimal(amount)
	if b > acc.Balance {
		b = acc.Balance
	}
	if b <= 0 {
		return nil
	}
	acc.Balance = acc.Balance.Sub(b)
	if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
		return fmt.Errorf("umlauf %s: %w", wallet, err)
	}
	cs.ensureAccountLoadedCtx(ctx, ubiPoolAddr)
	ubiAcc, ok := cs.accounts.Get(ubiPoolAddr)
	if !ok {
		ubiAcc = &AccountState{Address: ubiPoolAddr}
		cs.accounts.Set(ubiPoolAddr, ubiAcc)
	}
	ubiAcc.Balance = ubiAcc.Balance.Add(b)
	if err := cs.saveAccountToDBCtx(ctx, ubiAcc); err != nil {
		return fmt.Errorf("umlauf: Grundeinkommen: %w", err)
	}
	w := cs.wirt()
	w.mu.Lock()
	if at > w.letzterUmlauf {
		w.letzterUmlauf = at
	}
	w.mu.Unlock()
	return nil
}

// ------------------------------------------------------------ Register (Konsens)

func normAdresse(a string) (string, bool) {
	a = strings.ToLower(strings.TrimSpace(a))
	if len(a) != 42 || !strings.HasPrefix(a, "0x") {
		return "", false
	}
	for _, c := range a[2:] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return "", false
		}
	}
	return a, true
}

func normName(n string) string {
	n = strings.TrimSpace(n)
	n = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || r == '|' {
			return ' '
		}
		return r
	}, n)
	if r := []rune(n); len(r) > 60 {
		n = string(r[:60])
	}
	return n
}

// pruefeMenschLocked: registrierter Mensch? Caller haelt cs.mu.
func (cs *ChainState) pruefeMenschLocked(ctx context.Context, addr string) error {
	cs.ensureAccountLoadedCtx(ctx, addr)
	acc, ok := cs.accounts.Get(addr)
	if !ok || !acc.IsHuman {
		return fmt.Errorf("%s ist kein registrierter Mensch: %w", addr, ErrZustandLehntAb)
	}
	return nil
}

// applyUnternehmenEroeffnenLocked traegt ein Unternehmenskonto ein.
// Deterministisch aus dem Zustand -- Erzeuger und Nachspielen pruefen gleich.
func (cs *ChainState) applyUnternehmenEroeffnenLocked(ctx context.Context, unternehmen, mensch, name, kategorie string, at int64) error {
	if !wirtschaftAktiv(at) {
		return fmt.Errorf("unternehmen_eroeffnen vor der Aktivierung: %w", ErrZustandLehntAb)
	}
	u, ok1 := normAdresse(unternehmen)
	m, ok2 := normAdresse(mensch)
	if !ok1 || !ok2 || u == m {
		return fmt.Errorf("unternehmen_eroeffnen: ungueltige Adressen: %w", ErrZustandLehntAb)
	}
	kategorie = strings.ToLower(strings.TrimSpace(kategorie))
	if !unternehmenKategorien[kategorie] {
		return fmt.Errorf("unternehmen_eroeffnen: unbekannte Kategorie %q: %w", kategorie, ErrZustandLehntAb)
	}
	if istSystemAdresse(u) {
		return fmt.Errorf("unternehmen_eroeffnen: Systemadresse: %w", ErrZustandLehntAb)
	}
	if err := cs.pruefeMenschLocked(ctx, m); err != nil {
		return err
	}
	cs.ensureAccountLoadedCtx(ctx, u)
	if acc, ok := cs.accounts.Get(u); ok && acc.IsHuman {
		return fmt.Errorf("unternehmen_eroeffnen: %s ist ein Mensch: %w", u, ErrZustandLehntAb)
	}
	w := cs.wirt()
	w.mu.Lock()
	if w.offenesUnternehmenLocked(u) != nil {
		w.mu.Unlock()
		return fmt.Errorf("unternehmen_eroeffnen: %s ist schon ein Unternehmen: %w", u, ErrZustandLehntAb)
	}
	if len(w.unternehmenVon(m)) >= maxUnternehmenJeMensch {
		w.mu.Unlock()
		return fmt.Errorf("unternehmen_eroeffnen: %s ist schon fuer %d Unternehmen verantwortlich: %w", m, maxUnternehmenJeMensch, ErrZustandLehntAb)
	}
	e := &unternehmenEintrag{Adresse: u, Name: normName(name), Kategorie: kategorie, Verantwortliche: []string{m}, EroeffnetAm: at}
	w.unternehmen[u] = e
	w.mu.Unlock()
	return cs.speichereUnternehmen(ctx, e)
}

func (cs *ChainState) applyUnternehmenMitinhaberLocked(ctx context.Context, unternehmen, mensch string, at int64) error {
	if !wirtschaftAktiv(at) {
		return fmt.Errorf("unternehmen_mitinhaber vor der Aktivierung: %w", ErrZustandLehntAb)
	}
	u, ok1 := normAdresse(unternehmen)
	m, ok2 := normAdresse(mensch)
	if !ok1 || !ok2 {
		return fmt.Errorf("unternehmen_mitinhaber: ungueltige Adressen: %w", ErrZustandLehntAb)
	}
	if err := cs.pruefeMenschLocked(ctx, m); err != nil {
		return err
	}
	w := cs.wirt()
	w.mu.Lock()
	e := w.offenesUnternehmenLocked(u)
	switch {
	case e == nil:
		w.mu.Unlock()
		return fmt.Errorf("unternehmen_mitinhaber: %s ist kein offenes Unternehmen: %w", u, ErrZustandLehntAb)
	case e.istVerantwortlich(m):
		w.mu.Unlock()
		return fmt.Errorf("unternehmen_mitinhaber: %s ist schon verantwortlich: %w", m, ErrZustandLehntAb)
	case len(e.Verantwortliche) >= maxVerantwortlicheJeUnt:
		w.mu.Unlock()
		return fmt.Errorf("unternehmen_mitinhaber: hoechstens %d Verantwortliche: %w", maxVerantwortlicheJeUnt, ErrZustandLehntAb)
	case len(w.unternehmenVon(m)) >= maxUnternehmenJeMensch:
		w.mu.Unlock()
		return fmt.Errorf("unternehmen_mitinhaber: %s ist schon fuer %d Unternehmen verantwortlich: %w", m, maxUnternehmenJeMensch, ErrZustandLehntAb)
	}
	e.Verantwortliche = append(e.Verantwortliche, m)
	sort.Strings(e.Verantwortliche)
	cp := *e
	w.mu.Unlock()
	return cs.speichereUnternehmen(ctx, &cp)
}

// applyUnternehmenSchliessenLocked: nur mit leerem Konto. Danach ist die
// Adresse wieder eine freie Adresse.
func (cs *ChainState) applyUnternehmenSchliessenLocked(ctx context.Context, unternehmen string, at int64) error {
	u, ok := normAdresse(unternehmen)
	if !ok {
		return fmt.Errorf("unternehmen_schliessen: ungueltige Adresse: %w", ErrZustandLehntAb)
	}
	cs.ensureAccountLoadedCtx(ctx, u)
	if acc, ok := cs.accounts.Get(u); ok && acc.Balance.Float() > 0.000001 {
		return fmt.Errorf("unternehmen_schliessen: Konto haelt noch %.6f AEQ -- erst auszahlen: %w", acc.Balance.Float(), ErrZustandLehntAb)
	}
	w := cs.wirt()
	w.mu.Lock()
	e := w.offenesUnternehmenLocked(u)
	if e == nil {
		w.mu.Unlock()
		return fmt.Errorf("unternehmen_schliessen: %s ist kein offenes Unternehmen: %w", u, ErrZustandLehntAb)
	}
	e.GeschlossenAm = at
	cp := *e
	w.mu.Unlock()
	return cs.speichereUnternehmen(ctx, &cp)
}

// ------------------------------------------------------------ Speicher

func (cs *ChainState) wirtschaftInitDB() {
	if cs.db == nil {
		return
	}
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS wirtschaft_unternehmen (
			address TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '', kategorie TEXT NOT NULL DEFAULT '',
			verantwortliche TEXT NOT NULL DEFAULT '', eroeffnet_at BIGINT NOT NULL DEFAULT 0, geschlossen_at BIGINT NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS wirtschaft_buch (address TEXT PRIMARY KEY, daten TEXT NOT NULL DEFAULT '{}')`,
		`CREATE TABLE IF NOT EXISTS wirtschaft_meta (schluessel TEXT PRIMARY KEY, wert BIGINT NOT NULL DEFAULT 0)`,
	} {
		if _, err := cs.db.Exec(q); err != nil {
			fmt.Printf("[WIRTSCHAFT] Tabelle: %v\n", err)
		}
	}
}

func (cs *ChainState) speichereUnternehmen(ctx context.Context, e *unternehmenEintrag) error {
	if cs.db == nil {
		return nil
	}
	_, err := cs.dbExecCtx(ctx).Exec(`INSERT INTO wirtschaft_unternehmen (address, name, kategorie, verantwortliche, eroeffnet_at, geschlossen_at)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (address) DO UPDATE SET name=$2, kategorie=$3, verantwortliche=$4, eroeffnet_at=$5, geschlossen_at=$6`,
		e.Adresse, e.Name, e.Kategorie, strings.Join(e.Verantwortliche, ","), e.EroeffnetAm, e.GeschlossenAm)
	if err != nil {
		return fmt.Errorf("unternehmen speichern: %w", err)
	}
	return nil
}

func (cs *ChainState) speichereLetztenUmlauf(ctx context.Context, at int64) {
	if cs.db == nil {
		return
	}
	cs.dbExecCtx(ctx).Exec(`INSERT INTO wirtschaft_meta (schluessel, wert) VALUES ('letzter_umlauf', $1)
		ON CONFLICT (schluessel) DO UPDATE SET wert = GREATEST(wirtschaft_meta.wert, $1)`, at)
}

// wirtschaftLaden: beim Start, nach den Konten.
func (cs *ChainState) wirtschaftLaden() {
	if cs.db == nil {
		return
	}
	w := cs.wirt()
	w.mu.Lock()
	defer w.mu.Unlock()
	if rows, err := cs.db.Query(`SELECT address, name, kategorie, verantwortliche, eroeffnet_at, geschlossen_at FROM wirtschaft_unternehmen`); err == nil {
		for rows.Next() {
			var e unternehmenEintrag
			var v string
			if rows.Scan(&e.Adresse, &e.Name, &e.Kategorie, &v, &e.EroeffnetAm, &e.GeschlossenAm) == nil {
				if v != "" {
					e.Verantwortliche = strings.Split(v, ",")
				}
				w.unternehmen[e.Adresse] = &e
			}
		}
		rows.Close()
	}
	if rows, err := cs.db.Query(`SELECT address, daten FROM wirtschaft_buch`); err == nil {
		for rows.Next() {
			var a, d string
			if rows.Scan(&a, &d) == nil {
				var k buchKonto
				if json.Unmarshal([]byte(d), &k) == nil {
					w.buch[a] = &k
				}
			}
		}
		rows.Close()
	}
	var at sql.NullInt64
	if cs.db.QueryRow(`SELECT wert FROM wirtschaft_meta WHERE schluessel = 'letzter_umlauf'`).Scan(&at) == nil && at.Valid {
		w.letzterUmlauf = at.Int64
	}
	fmt.Printf("✓ Wirtschaft: %d Unternehmen, %d Buchkonten\n", len(w.unternehmen), len(w.buch))
}

// startBuchFlusher schreibt die Buchfuehrung alle 10 Sekunden weg. Geht sie
// bei einem Absturz verloren, heilt abgleichen() das beim naechsten Zugriff
// (fehlendes Geld gilt dann als frisch -- zugunsten der Kontoinhaber).
func (cs *ChainState) startBuchFlusher() {
	if cs.db == nil {
		return
	}
	w := cs.wirt()
	w.flusher.Do(func() {
		SafeGoroutine("WirtschaftBuchFlush", func() {
			t := time.NewTicker(10 * time.Second)
			defer t.Stop()
			for range t.C {
				cs.flushBuch()
			}
		})
	})
}

func (cs *ChainState) flushBuch() {
	w := cs.wirt()
	w.mu.Lock()
	daten := make(map[string]string, len(w.schmutzig))
	for a := range w.schmutzig {
		if k := w.buch[a]; k != nil {
			if b, err := json.Marshal(k); err == nil {
				daten[a] = string(b)
			}
		}
	}
	w.schmutzig = map[string]bool{}
	w.mu.Unlock()
	for a, d := range daten {
		if _, err := cs.db.Exec(`INSERT INTO wirtschaft_buch (address, daten) VALUES ($1,$2)
			ON CONFLICT (address) DO UPDATE SET daten = $2`, a, d); err != nil {
			fmt.Printf("[WIRTSCHAFT] Buch %s: %v\n", a, err)
		}
	}
}

// ------------------------------------------------------------ Snapshot

func (cs *ChainState) unternehmenFuerSnapshot() []*unternehmenEintrag {
	w := cs.wirt()
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]*unternehmenEintrag, 0, len(w.unternehmen))
	for _, e := range w.unternehmen {
		cp := *e
		cp.Verantwortliche = append([]string(nil), e.Verantwortliche...)
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Adresse < out[j].Adresse })
	return out
}

func (cs *ChainState) unternehmenAusSnapshot(liste []*unternehmenEintrag) {
	if len(liste) == 0 {
		return
	}
	w := cs.wirt()
	w.mu.Lock()
	for _, e := range liste {
		if e == nil {
			continue
		}
		cp := *e
		w.unternehmen[cp.Adresse] = &cp
	}
	w.mu.Unlock()
	for _, e := range liste {
		if e != nil {
			cs.speichereUnternehmen(context.Background(), e)
		}
	}
}

// abgabeInsGrundeinkommen schreibt eine Abgabe dem UBI-Topf gut (Aufrufer
// hat sie dem Konto schon abgezogen). Caller haelt cs.mu.
func (cs *ChainState) abgabeInsGrundeinkommen(ctx context.Context, betrag float64) error {
	if betrag <= 0 {
		return nil
	}
	cs.ensureAccountLoadedCtx(ctx, ubiPoolAddr)
	ubiAcc, ok := cs.accounts.Get(ubiPoolAddr)
	if !ok {
		ubiAcc = &AccountState{Address: ubiPoolAddr}
		cs.accounts.Set(ubiPoolAddr, ubiAcc)
	}
	ubiAcc.Balance = ubiAcc.Balance.Add(NewDecimal(betrag))
	if err := cs.saveAccountToDBCtx(ctx, ubiAcc); err != nil {
		return fmt.Errorf("Abgabe ans Grundeinkommen: %w", err)
	}
	return nil
}
