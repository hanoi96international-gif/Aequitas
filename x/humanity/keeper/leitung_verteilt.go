package keeper

// STUFE 2 IN DER LEITUNG: WER DARF IN DIESEM AUGENBLICK FUER WELCHES KONTO
// ANNEHMEN?
//
// Die Leitung (leitung.go) garantiert heute: nie zwei Leiter gleichzeitig.
// Stufe 2 braucht mehr und weniger zugleich: viele Annehmende, aber fuer jedes
// Konto nie zwei. Dieselben Bausteine tragen es -- Lease, Mehrheit, Frist:
//
//  1. ZUTEILUNG JE TERM. Der Leiter legt beim Amtsantritt fest, auf welche
//     Mitglieder die Konten verteilt werden (sein Satz in diesem Augenblick),
//     und schickt die Zuteilung mit jeder Lease. Sie aendert sich im Term
//     nicht -- wer im Term dazukommt, bekommt Konten ab dem naechsten.
//     zustaendigFuer (zustaendigkeit.go) rechnet daraus den Zustaendigen.
//
//  2. ANNAHME NUR MIT FRISCHER, BESTAETIGTER LEASE. Ein Mitglied nimmt fuer
//     seine Konten an, solange es vor hoechstens LeaseDauer/2 eine Lease
//     empfangen hat, in der der Leiter "bestaetigt" meldet -- also selbst
//     annehmen darf, weil eine Mehrheit ihm innerhalb von LeaseDauer
//     geantwortet hat. Ein Mitglied, das mit dem Leiter von der Mehrheit
//     abgeschnitten ist, hoert damit spaetestens LeaseDauer + LeaseDauer/2
//     nach der Trennung auf; die Mehrheit waehlt fruehestens nach FolgerFrist
//     (12 s > 9 s), und die Mitglieder des neuen Terms warten zusaetzlich die
//     Ruhezeit ab.
//
//  3. GEORDNETE UEBERGABE. Beim planmaessigen Wechsel schickt der Leiter
//     "Abschluss": niemand nimmt mehr an. Jedes Mitglied quittiert, sobald bei
//     ihm nichts mehr offen ist (Entleert), mit seinem letzten Block. Die
//     Uebergabe traegt alle diese Bloecke (Abschluesse); wer im neuen Term
//     annimmt, muss sie vorher nachgespielt haben. Meldet sich ein Mitglied
//     nicht binnen LeaseDauer, wird ohne es uebergeben -- es hat dann laengst
//     aufgehoert (Punkt 2); was es angenommen und nicht verteilt hat, gilt
//     als nicht geschehen (Ueberholt), wie beim ausgefallenen Leiter heute.
//
//  4. AUSFALL IM TERM. Quittiert ein Mitglied laenger als LeaseDauer nicht,
//     fuehrt der Leiter es als ausgefallen, fuer den Rest des Terms. Das
//     Mitglied selbst hoert auf, sobald es das in einer Lease sieht, und
//     spaetestens LeaseDauer/2 nach der letzten Lease, die es noch erreicht
//     hat. Der Naechste im Ring uebernimmt erst LeaseDauer + Ruhezeit nach
//     dem Augenblick, in dem er davon erfaehrt.
//
// Globale Konten (Toepfe, Liquiditaetspool, Faucet) bleiben beim Leiter.
//
// Bei zwei Validatoren gibt es keine Mehrheit (leitung.go, "Mit zwei
// Validatoren") -- dort bleibt es beim einen Annehmenden.
//
// Die Eigenschaft "fuer kein Konto zwei Annehmende" prueft die Simulation
// (leitung_verteilt_sim_test.go) in jedem Schritt, mit Uhrengang,
// Nachrichtenverlust, Ausfaellen, Neustarts und Netztrennungen.

import (
	"sort"
	"time"
)

type verteiltStand struct {
	term             uint64 // fuer welchen Term dieser Stand gilt
	zuteilung        []string
	ausgefallen      map[string]time.Time // Mitglied -> seit wann (Ortszeit)
	termSeit         time.Time
	geordnet         bool     // Term begann mit geordneter Uebergabe
	wartetAuf        []string // Bloecke, die vor dem Annehmen nachgespielt sein muessen
	letzteBestaetigt time.Time
	abschlussGesehen bool
	hatAngenommen    bool
	ueberholtFuer    uint64 // fuer diesen Term wurde Ueberholt schon gemeldet

	// Leiter, waehrend er abschliesst.
	abschliessenSeit time.Time
	abschlussVon     map[string]string
}

// verteilt: soll ein Term, der JETZT beginnt, verteilt annehmen? Ab drei
// Validatoren (mit Wahl), und wenn Stufe 2 eingeschaltet oder aktiviert ist.
//
// Ob ein Term verteilt IST, entscheidet der Leiter bei seinem Antritt und
// schickt es mit der Zuteilung (verteiltImTerm). Eine Aktivierung mitten im
// Term greift deshalb erst mit dem naechsten: sonst naehmen Mitglieder an,
// bevor sie die Bloecke des bisher einzigen Annehmenden haben.
func (l *Leitung) verteilt() bool {
	return l.mitWahl() && (l.cfg.Verteilt || verteilteAnnahmeAktiv(nowUnix()))
}

// verteiltImTerm: nimmt der laufende Term verteilt an?
func (l *Leitung) verteiltImTerm() bool {
	return l.vt.term == l.term && len(l.vt.zuteilung) > 0
}

// DarfAnnehmenFuer: darf dieser Knoten JETZT einen Auftrag annehmen, der
// konto belastet (annahmeKonto)?
func (l *Leitung) DarfAnnehmenFuer(konto string, jetzt time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.darfAnnehmenFuer(konto, jetzt)
}

func (l *Leitung) darfAnnehmenFuer(konto string, jetzt time.Time) bool {
	if !l.verteiltImTerm() {
		// Ein Annehmender fuer alles -- der Leiter, wie bisher. Ein Folger
		// ohne Zuteilung (noch keine Lease des verteilten Terms) nimmt nicht an.
		return l.rolle == leitLeiter && l.darfAnnehmen(jetzt)
	}
	if !l.bereit(jetzt) {
		return false
	}
	if l.zustaendigLocked(konto) != l.ich || !l.uebernahmeSicher(konto, jetzt) {
		return false
	}
	ok := false
	switch l.rolle {
	case leitLeiter:
		ok = l.darfAnnehmen(jetzt)
	case leitFolger:
		ok = l.leiter != "" && !l.vt.abschlussGesehen && !l.vt.letzteBestaetigt.IsZero() &&
			jetzt.Sub(l.vt.letzteBestaetigt) < l.cfg.LeaseDauer/2 && l.vt.ausgefallen[l.ich].IsZero()
	}
	if ok {
		l.vt.hatAngenommen = true
	}
	return ok
}

// Zustaendig: Adresse und URL dessen, der im laufenden Term fuer konto
// annimmt -- fuer die Weiterleitung. Leer, wenn unbekannt.
func (l *Leitung) Zustaendig(konto string) (string, string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.verteiltImTerm() {
		return l.leiter, l.urls[l.leiter]
	}
	a := l.zustaendigLocked(konto)
	return a, l.urls[a]
}

func (l *Leitung) zustaendigLocked(konto string) string {
	aus := make(map[string]bool, len(l.vt.ausgefallen))
	for a := range l.vt.ausgefallen {
		aus[a] = true
	}
	return zustaendigFuer(konto, l.term, l.vt.zuteilung, aus, l.leiter)
}

// bereit: sind die Bloecke der Vorgaenger da, und ist nach einem Term ohne
// geordnete Uebergabe die Ruhezeit verstrichen?
func (l *Leitung) bereit(jetzt time.Time) bool {
	for _, h := range l.vt.wartetAuf {
		if l.env.HatBlock != nil && !l.env.HatBlock(h) {
			return false
		}
	}
	return l.vt.geordnet || jetzt.Sub(l.vt.termSeit) >= l.cfg.Ruhezeit
}

// uebernahmeSicher: gehoert konto diesem Knoten nur, weil Mitglieder davor im
// Ring ausgefallen sind, muss seit deren Ausfall LeaseDauer + Ruhezeit
// vergangen sein -- erst dann haben sie sicher aufgehoert und ihre letzten
// Bloecke koennen angekommen sein.
func (l *Leitung) uebernahmeSicher(konto string, jetzt time.Time) bool {
	if globalesKonto(konto) {
		return true
	}
	n := len(l.vt.zuteilung)
	i := zuteilungsIndex(konto, l.term, n)
	for k := 0; k < n; k++ {
		a := l.vt.zuteilung[(i+k)%n]
		seit, aus := l.vt.ausgefallen[a]
		if !aus {
			return true // a ist der Zustaendige (darfAnnehmenFuer prueft, ob ich es bin)
		}
		if jetzt.Sub(seit) < l.cfg.LeaseDauer+l.cfg.Ruhezeit {
			return false
		}
	}
	return false
}

// neuerTermVerteilt: Stand fuer einen neuen Term. uebergabe: der Beleg der
// geordneten Uebergabe (nil nach einer Wahl).
func (l *Leitung) neuerTermVerteilt(term uint64, zuteilung []string, uebergabe *LeitNachricht, jetzt time.Time) {
	alt := l.vt
	// Wer im alten Term angenommen hat und im Abschluss nicht vorkommt, hat
	// womoeglich Angenommenes, das nie in einen Block kam: nicht geschehen.
	if alt.hatAngenommen && alt.term != 0 && alt.term < term && alt.ueberholtFuer != alt.term && l.env.Ueberholt != nil {
		if uebergabe == nil || uebergabe.Abschluesse[l.ich] == "" {
			defer l.env.Ueberholt(alt.term)
		}
	}
	l.vt = verteiltStand{
		term:        term,
		zuteilung:   normSatz(zuteilung),
		ausgefallen: map[string]time.Time{},
		termSeit:    jetzt,
	}
	if uebergabe != nil && uebergabe.Abschluesse != nil {
		l.vt.geordnet = true
		for _, h := range uebergabe.Abschluesse {
			if h != "" {
				l.vt.wartetAuf = append(l.vt.wartetAuf, h)
			}
		}
		sort.Strings(l.vt.wartetAuf)
	}
}

// leiterAntrittVerteilt: aus werdeLeiter.
func (l *Leitung) leiterAntrittVerteilt(jetzt time.Time, beleg *LeitNachricht) {
	if !l.verteilt() {
		return
	}
	l.neuerTermVerteilt(l.term, l.satz, beleg, jetzt)
}

// leaseVerteilt: was der Leiter in jede Lease schreibt.
func (l *Leitung) leaseVerteilt(m *LeitNachricht, jetzt time.Time) {
	if !l.verteiltImTerm() {
		return
	}
	m.Zuteilung = append([]string(nil), l.vt.zuteilung...)
	for a := range l.vt.ausgefallen {
		m.Ausgefallen = append(m.Ausgefallen, a)
	}
	sort.Strings(m.Ausgefallen)
	m.Abschluss = l.abschliessen
	m.Bestaetigt = !l.abschliessen && l.darfAnnehmen(jetzt)
}

// ausfaelleErkennen: der Leiter fuehrt Mitglieder als ausgefallen, die
// laenger als LeaseDauer keine Lease quittiert haben. Nie zurueck im Term.
func (l *Leitung) ausfaelleErkennen(jetzt time.Time) {
	if !l.verteiltImTerm() || jetzt.Sub(l.leiterSeit) < l.cfg.LeaseDauer {
		return
	}
	neu := false
	for _, a := range l.vt.zuteilung {
		if a == l.ich {
			continue
		}
		if _, schon := l.vt.ausgefallen[a]; schon {
			continue
		}
		if t, ok := l.acks[a]; !ok || jetzt.Sub(t) >= l.cfg.LeaseDauer {
			l.vt.ausgefallen[a] = jetzt
			neu = true
		}
	}
	if neu {
		l.speichern() // ueberlebt einen Neustart (LeitSpeicher.Ausgefallen)
	}
}

// quittungVerteilt: der Leiter merkt sich, wer nach dem Abschluss entleert
// gemeldet hat, mit welchem letzten Block.
func (l *Leitung) quittungVerteilt(m LeitNachricht) {
	if !l.verteiltImTerm() || !l.abschliessen || !m.Entleert || m.Term != l.term {
		return
	}
	if l.vt.abschlussVon == nil {
		l.vt.abschlussVon = map[string]string{}
	}
	l.vt.abschlussVon[m.Von] = m.BlockHash
}

// abschlussFertig: darf der Leiter jetzt uebergeben? Alle Mitglieder der
// Zuteilung haben entleert gemeldet, oder LeaseDauer ist seit Beginn des
// Abschlusses vergangen (die Schweigenden haben dann sicher aufgehoert).
func (l *Leitung) abschlussFertig(jetzt time.Time) bool {
	if !l.verteiltImTerm() {
		return true
	}
	if jetzt.Sub(l.vt.abschliessenSeit) >= l.cfg.LeaseDauer {
		return true
	}
	for _, a := range l.vt.zuteilung {
		if a == l.ich {
			continue
		}
		if _, ok := l.vt.abschlussVon[a]; !ok {
			return false
		}
	}
	return true
}

// abschlussBeginnt: aus Takt, wenn der Leiter abschliessen setzt.
func (l *Leitung) abschlussBeginnt(jetzt time.Time) {
	l.vt.abschliessenSeit = jetzt
	l.vt.abschlussVon = map[string]string{}
}

// uebergabeVerteilt: die Abschluesse in die Uebergabe.
func (l *Leitung) uebergabeVerteilt(ue *LeitNachricht) {
	// Auch aus einem Term mit einem Annehmenden heraus, wenn der naechste
	// verteilt sein wird: dann wartet jeder auf den letzten Block des Leiters.
	if !l.verteilt() && !l.verteiltImTerm() {
		return
	}
	ue.Abschluesse = map[string]string{}
	for a, h := range l.vt.abschlussVon {
		ue.Abschluesse[a] = h
	}
	ue.Abschluesse[l.ich] = ue.BlockHash
	l.vt.ueberholtFuer = l.vt.term // selbst uebergeben: nicht ueberholt
}

// folgerLeaseVerteilt: aus empfangeLease, nachdem die Lease angenommen ist.
// Setzt, falls noetig, den Stand des neuen Terms und fuellt die Quittung.
func (l *Leitung) folgerLeaseVerteilt(m LeitNachricht, ack *LeitNachricht, jetzt time.Time) {
	// Ob verteilt wird, hat der Leiter entschieden: er schickt die Zuteilung.
	if m.Term != l.term || len(m.Zuteilung) == 0 || !l.mitWahl() {
		return
	}
	if l.vt.term != m.Term {
		l.neuerTermVerteilt(m.Term, m.Zuteilung, m.Uebergabe, jetzt)
	} else if !gleicheZuteilung(l.vt.zuteilung, normSatz(m.Zuteilung)) {
		// Dieselbe Amtszeit, eine andere Zuteilung: darf nicht sein (siehe
		// LeitSpeicher.Zuteilung). Lieber bis zum naechsten Term gar nicht
		// annehmen als nach zwei verschiedenen Zuteilungen.
		l.vt.zuteilung = nil
		return
	}
	if len(l.vt.zuteilung) == 0 {
		return
	}
	for _, a := range m.Ausgefallen {
		if _, schon := l.vt.ausgefallen[a]; !schon {
			l.vt.ausgefallen[a] = jetzt
		}
	}
	if m.Abschluss {
		l.vt.abschlussGesehen = true
	} else if m.Bestaetigt {
		l.vt.letzteBestaetigt = jetzt
	}
	if l.vt.abschlussGesehen && (l.env.Entleert == nil || l.env.Entleert()) {
		ack.Entleert = true
		if l.env.LetzterBlk != nil {
			ack.BlockHash = l.env.LetzterBlk()
		}
	}
}

// standVerteilt fuer /api/health/combined.
func (l *Leitung) standVerteilt(jetzt time.Time) map[string]interface{} {
	if !l.verteiltImTerm() {
		return map[string]interface{}{"an": false, "naechster_term_verteilt": l.verteilt()}
	}
	var aus []string
	for a := range l.vt.ausgefallen {
		aus = append(aus, a)
	}
	sort.Strings(aus)
	eigene := 0
	for _, a := range l.vt.zuteilung {
		if a == l.ich {
			eigene = 1
		}
	}
	return map[string]interface{}{
		"an":          true,
		"term":        l.vt.term,
		"zuteilung":   l.vt.zuteilung,
		"ausgefallen": aus,
		"geordnet":    l.vt.geordnet,
		"bereit":      l.vt.term == l.term && l.bereit(jetzt),
		"mitglied":    eigene == 1,
		"abschluss":   l.vt.abschlussGesehen || l.abschliessen,
	}
}

func gleicheZuteilung(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
