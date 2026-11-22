package keeper

// QUANTENGELD S, WEG B -- STUFE B2a: PRAEPARATOREN-REGISTER, BESTELLUNG UND
// LOS IN DER KETTE (schlafend bis quantenSWegBAbUnix; Spezifikation
// forschung/quantengeld/s_weg_b.py, DEZENTRAL.md 18).
//
// Jeder registrierte Mensch kann sich als Praeparator melden. Je Bestellung
// lost die Kette v Praeparatoren aus; der Ausgeber waehlt niemanden. Die
// Ausgabe gegen das Los folgt in B2b -- bis dahin weist die Kette eine
// Ausgabe zu einem bestellten K ab.
//
// DIE VIER ARTEN (Satzarten im W-Modul, quanten_w_kette.go; Stichtag von W
// UND von Weg B muessen erreicht sein)
//
//   - quanten_w_s_melden (Wallet, unterschrieben, Zeit): die Wallet ist ein
//     Mensch mit Nullifier. Je Wallet ein Eintrag. Hat derselbe Mensch
//     (Nullifier, etwa nach einem Umzug) noch einen aktiven Eintrag unter
//     einer anderen Wallet, wird der jetzt abgemeldet -- ein Mensch, ein
//     Praeparator.
//   - quanten_w_s_abmelden (Wallet, unterschrieben, Zeit): bindet die
//     Meldezeit des Eintrags (eine alte Unterschrift trifft keine spaetere
//     Meldung).
//   - quanten_w_s_bestellen (Wallet = Ausgeber, ein Mensch, unterschrieben,
//     Zeit): K, v, t. Je Wallet eine offene Bestellung.
//   - quanten_w_s_losen (ohne Wallet): drand-Runde und Signatur. Die Runde
//     ist FEST: die erste mit Zeit >= Bestellzeit + quantenSLosAbstand. Wer
//     einreicht, kann also nicht unter vielen Runden waehlen. Jeder Knoten
//     prueft die BLS-Signatur selbst (drand_zufall.go) und rechnet das Los
//     aus dem Register ZUR BESTELLUNG nach: im Los ist, wer mindestens
//     quantenSMeldeVorlauf vor der Bestellung gemeldet und zur Bestellung
//     nicht abgemeldet war (s_weg_b.py, Abbild). Eine spaetere Abmeldung
//     verschiebt das Los nicht.
//
// FRISTEN UND GRENZEN
//
//   - Gelost werden muss bis Bestellzeit + quantenSLosFrist, ausgegeben bis
//     Bestellzeit + quantenSAusgabeFrist; sonst verfaellt die Bestellung
//     (es haengt kein Geld daran).
//   - Abgemeldete Eintraege bleiben quantenWToteBehalten (eine alte
//     Melde-Unterschrift ist dann abgelaufen) und solange eine ungeloste
//     Bestellung aelter als die Abmeldung ist.
//   - Hoechstens quantenSPraepHoechstens Eintraege und
//     quantenSBestellHoechstens Bestellungen.
//   - Faellt drand aus, wird nicht gelost (fail-closed); die Bestellung
//     verfaellt.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
)

const (
	quantenSMeldenTxArt    = "quanten_w_s_melden"
	quantenSAbmeldenTxArt  = "quanten_w_s_abmelden"
	quantenSBestellenTxArt = "quanten_w_s_bestellen"
	quantenSLosenTxArt     = "quanten_w_s_losen"
	// Pfaender (Entscheidung des Betreibers 22.11.2026): beide ohne Wallet,
	// jeder darf sie einreichen.
	quantenSVerfallenTxArt    = "quanten_w_s_verfallen"     // abgelaufene Bestellung: Pfand ans Grundeinkommen
	quantenSPfandZurueckTxArt = "quanten_w_s_pfand_zurueck" // Praeparator-Pfand nach der Haltezeit zurueck

	quantenSDomMelden    = "aequitas-quanten-s-weg-b-melden-v1"
	quantenSDomAbmelden  = "aequitas-quanten-s-weg-b-abmelden-v1"
	quantenSDomBestellen = "aequitas-quanten-s-weg-b-bestellen-v1"

	// quantenSWegBAbUnix: Stichtag von Weg B (math.MaxInt64 = schlafend).
	quantenSWegBAbUnix int64 = math.MaxInt64

	quantenSMeldeVorlauf int64 = 86400     // im Los erst einen Tag nach der Meldung
	quantenSLosAbstand   int64 = 30        // drand-Runde fruehestens so lange nach der Bestellung
	quantenSLosFrist     int64 = 3600      // bis dahin muss gelost sein
	quantenSAusgabeFrist int64 = 3 * 86400 // bis dahin muss ausgegeben sein (B2b)

	// Pfaender (Mikro-AEQ) im W-Pfandtopf; der Betrag steht im Eintrag.
	quantenSBestellPfand int64 = 1_000_000  // 1 AEQ je Bestellung, zurueck bei der Ausgabe
	quantenSPraepPfand   int64 = 10_000_000 // 10 AEQ je Praeparator
	// quantenSPfandHalten: so lange nach der Abmeldung bleibt das Pfand
	// gesperrt (fuer den Nachweis einer Faelschung, folgende Stufe).
	quantenSPfandHalten int64 = 30 * 86400

	quantenSPraepHoechstens   = 4096
	quantenSBestellHoechstens = 32

	quantenWArtPraep = "praep"
	quantenWArtBest  = "best"
)

// quantenSDrand: das drand-Netz fuer das Los (var: Tests setzen ein Netz mit
// eigenem Schluessel ein, um beliebige Runden zu unterschreiben).
var quantenSDrand = drandQuicknet

// quantenSWegBAbOverride: nur fuer Tests (0 = Konstante gilt).
var quantenSWegBAbOverride atomic.Int64

func quantenSWegBAb() int64 {
	if v := quantenSWegBAbOverride.Load(); v != 0 {
		return v
	}
	return quantenSWegBAbUnix
}

func quantenSWegBAktiv(t int64) bool { return quantenWAktiv(t) && t >= quantenSWegBAb() }

func istQuantenSWegBTxArt(t string) bool {
	switch t {
	case quantenSMeldenTxArt, quantenSAbmeldenTxArt, quantenSBestellenTxArt, quantenSLosenTxArt, quantenSVerfallenTxArt, quantenSPfandZurueckTxArt:
		return true
	}
	return false
}

// quantenSPraep: ein Eintrag im Praeparatoren-Register (Schluessel: Wallet).
type quantenSPraep struct {
	Nullifier  string
	Gemeldet   int64
	Abgemeldet int64 // 0 = aktiv
	Pfand      int64
	Erbe       string // nach einem Umzug: die neue Wallet des Menschen (bekommt das Pfand)
}

// quantenSBestellung: eine Bestellung (Schluessel: K).
type quantenSBestellung struct {
	Von   string
	V, T  int
	Zeit  int64
	Runde uint64   // 0 = noch nicht gelost
	Los   []string // die Gelosten in Losreihenfolge
	Pfand int64
}

type quantenSPraepForm struct {
	Konto      string `json:"konto"`
	Nullifier  string `json:"nullifier"`
	Gemeldet   int64  `json:"gemeldet"`
	Abgemeldet int64  `json:"abgemeldet,omitempty"`
	Pfand      int64  `json:"pfand"`
	Erbe       string `json:"erbe,omitempty"`
}

type quantenSBestellForm struct {
	K     string   `json:"k"`
	Von   string   `json:"von"`
	V     int      `json:"v"`
	T     int      `json:"t"`
	Zeit  int64    `json:"zeit"`
	Runde uint64   `json:"runde,omitempty"`
	Los   []string `json:"los,omitempty"`
	Pfand int64    `json:"pfand"`
}

// ------------------------------------------------------------ Form

// quantenSNullifierForm: wie ihn die Registrierung speichert (Text, hoechstens
// 128 Zeichen, druckbar ohne Leerzeichen).
func quantenSNullifierForm(n string) bool {
	if n == "" || len(n) > 128 {
		return false
	}
	for _, c := range n {
		if c <= ' ' || c > '~' {
			return false
		}
	}
	return true
}

func quantenSPraepLesen(f []quantenSPraepForm) (map[string]quantenSPraep, error) {
	if len(f) > quantenSPraepHoechstens {
		return nil, errors.New("quanten_w: Praeparatoren ueber der Grenze")
	}
	out := make(map[string]quantenSPraep, len(f))
	aktiv := map[string]bool{}
	for i, p := range f {
		if !kanonischeAdresse(p.Konto) || (i > 0 && p.Konto <= f[i-1].Konto) || !quantenSNullifierForm(p.Nullifier) ||
			p.Gemeldet <= 0 || (p.Abgemeldet != 0 && p.Abgemeldet < p.Gemeldet) || p.Pfand <= 0 || p.Pfand > quantenAnteileHoechstens ||
			(p.Erbe != "" && (!kanonischeAdresse(p.Erbe) || p.Erbe == p.Konto || p.Abgemeldet == 0)) {
			return nil, fmt.Errorf("quanten_w: Praeparator %d: Form", i)
		}
		if p.Abgemeldet == 0 {
			if aktiv[p.Nullifier] {
				return nil, fmt.Errorf("quanten_w: Praeparator %d: Mensch zweimal aktiv", i)
			}
			aktiv[p.Nullifier] = true
		}
		out[p.Konto] = quantenSPraep{Nullifier: p.Nullifier, Gemeldet: p.Gemeldet, Abgemeldet: p.Abgemeldet, Pfand: p.Pfand, Erbe: p.Erbe}
	}
	return out, nil
}

func quantenSBestellLesen(f []quantenSBestellForm, r *quantenWRegister) (map[string]quantenSBestellung, error) {
	if len(f) > quantenSBestellHoechstens {
		return nil, errors.New("quanten_w: Bestellungen ueber der Grenze")
	}
	out := make(map[string]quantenSBestellung, len(f))
	von := map[string]bool{}
	for i, b := range f {
		if !istHash64(b.K) || (i > 0 && b.K <= f[i-1].K) || r.Noten[b.K] != nil || r.Token[b.K] != nil || r.Tote[b.K] != 0 ||
			!kanonischeAdresse(b.Von) || von[b.Von] || !quantenSVTGueltig(b.V, b.T) || b.Zeit <= 0 ||
			(b.Runde == 0) != (b.Los == nil) || (b.Los != nil && len(b.Los) != b.V) ||
			(b.Runde != 0 && b.Runde != quantenSLosRunde(b.Zeit)) || b.Pfand <= 0 || b.Pfand > quantenAnteileHoechstens {
			return nil, fmt.Errorf("quanten_w: Bestellung %d: Form", i)
		}
		gesehen := map[string]bool{}
		for _, k := range b.Los {
			if !kanonischeAdresse(k) || gesehen[k] {
				return nil, fmt.Errorf("quanten_w: Bestellung %d: Los", i)
			}
			gesehen[k] = true
		}
		von[b.Von] = true
		out[b.K] = quantenSBestellung{Von: b.Von, V: b.V, T: b.T, Zeit: b.Zeit, Runde: b.Runde, Los: slices.Clone(b.Los), Pfand: b.Pfand}
	}
	return out, nil
}

func (r *quantenWRegister) wegBForm(f *quantenWRegisterForm) {
	for k, p := range r.Praep {
		f.Praep = append(f.Praep, quantenSPraepForm{Konto: k, Nullifier: p.Nullifier, Gemeldet: p.Gemeldet, Abgemeldet: p.Abgemeldet, Pfand: p.Pfand, Erbe: p.Erbe})
	}
	for k, b := range r.Bestellt {
		f.Bestellt = append(f.Bestellt, quantenSBestellForm{K: k, Von: b.Von, V: b.V, T: b.T, Zeit: b.Zeit, Runde: b.Runde, Los: b.Los, Pfand: b.Pfand})
	}
	sort.Slice(f.Praep, func(i, j int) bool { return f.Praep[i].Konto < f.Praep[j].Konto })
	sort.Slice(f.Bestellt, func(i, j int) bool { return f.Bestellt[i].K < f.Bestellt[j].K })
}

func quantenSVTGueltig(v, t int) bool {
	return v >= quantenSLosVMin && v <= quantenSLosVMax && t >= 2 && t <= v
}

// quantenSLosRunde: die erste drand-Runde (quicknet) mit Zeit >=
// Bestellzeit + quantenSLosAbstand. Fest -- niemand waehlt sie.
func quantenSLosRunde(bestellZeit int64) uint64 {
	n := quantenSDrand
	ziel := sattAdd(bestellZeit, quantenSLosAbstand)
	if ziel <= n.genesis {
		return 1
	}
	d := ziel - n.genesis
	return uint64((d+n.periode-1)/n.periode) + 1
}

// quantenSAbbild: das Register zur Bestellung (s_weg_b.py, Abbild).
func quantenSAbbild(praep map[string]quantenSPraep, bestellZeit int64) []string {
	var out []string
	for k, p := range praep {
		if p.Gemeldet <= bestellZeit-quantenSMeldeVorlauf && (p.Abgemeldet == 0 || p.Abgemeldet > bestellZeit) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// quantenSBestellungVerfallen: ungelost nach der Losfrist oder nicht
// ausgegeben nach der Ausgabefrist. Weg ist sie erst mit quanten_w_s_verfallen
// (das Pfand geht dann ans Grundeinkommen) -- nichts verschwindet still.
func quantenSBestellungVerfallen(b quantenSBestellung, zeit int64) bool {
	return (b.Runde == 0 && zeit > sattAdd(b.Zeit, quantenSLosFrist)) || zeit > sattAdd(b.Zeit, quantenSAusgabeFrist)
}

// quantenSPfandFrei: darf das Pfand des abgemeldeten Eintrags zurueck? Nach
// der Haltezeit, und keine ungeloste, nicht verfallene Bestellung braucht ihn
// noch fuer ihr Abbild.
func quantenSPfandFrei(r *quantenWRegister, p quantenSPraep, zeit int64) bool {
	if p.Abgemeldet == 0 || zeit <= sattAdd(p.Abgemeldet, max(quantenSPfandHalten, quantenWToteBehalten)) {
		return false
	}
	for _, b := range r.Bestellt {
		if b.Runde == 0 && b.Zeit < p.Abgemeldet && !quantenSBestellungVerfallen(b, zeit) {
			return false
		}
	}
	return true
}

// ------------------------------------------------------------ Saetze

func quantenSMeldenNachricht(wallet string, zeit int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%d", quantenSDomMelden, eingefroreneChainID, strings.ToLower(wallet), zeit)
}

func quantenSAbmeldenNachricht(wallet string, gemeldet, zeit int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|gemeldet:%d|%d", quantenSDomAbmelden, eingefroreneChainID, strings.ToLower(wallet), gemeldet, zeit)
}

func quantenSBestellenNachricht(wallet, K string, v, t int, zeit int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%d|%d|%d", quantenSDomBestellen, eingefroreneChainID, strings.ToLower(wallet), K, v, t, zeit)
}

// quantenSWegBNurEigeneFelder: je Art nur ihre Angaben (q: K und Index schon
// geleert, Salze normiert).
func quantenSWegBNurEigeneFelder(tx *Transaction, q QuantenWAngaben) bool {
	if tx.QuantenW.Index != 0 {
		return false
	}
	switch tx.Type {
	case quantenSMeldenTxArt, quantenSAbmeldenTxArt:
		q.WalletSig, q.PQ, q.Pfand = "", nil, 0
		return reflect0(q) && tx.QuantenW.K == "" && tx.DistributionAt > 0
	case quantenSBestellenTxArt:
		q.V, q.T, q.WalletSig, q.PQ, q.Pfand = 0, 0, "", nil, 0
		return reflect0(q) && tx.DistributionAt > 0
	case quantenSLosenTxArt:
		q.Runde, q.DrandSig = 0, ""
		return reflect0(q) && tx.Wallet == "" && tx.DistributionAt == 0
	case quantenSVerfallenTxArt:
		q.Pfand = 0
		return reflect0(q) && tx.Wallet == "" && tx.DistributionAt == 0
	case quantenSPfandZurueckTxArt:
		q.An, q.Pfand, q.PfandAn = "", 0, ""
		return reflect0(q) && tx.QuantenW.K == "" && tx.Wallet == "" && tx.DistributionAt == 0
	}
	return false
}

func reflect0(q QuantenWAngaben) bool {
	return q.N == 0 && q.T == 0 && q.Wurzel == "" && q.Validatoren == nil && q.Anker == nil && q.Bestaetigungen == nil &&
		q.Deckung == 0 && q.Tol == 0 && q.Stellen == 0 && q.An == "" && q.Zufall == nil && q.Bits == "" && q.Offen == nil &&
		q.Ab == 0 && q.Inhaber == "" && q.Annahme == nil && q.Proben == nil && q.D == "" && q.C == "" && q.B == "" &&
		q.X == "" && q.Probe == nil && q.Vorlage == "" && q.ProbeArt == 0 && q.Salze == nil && q.WalletSig == "" &&
		q.PQ == nil && q.Anteile == 0 && q.Pfand == 0 && q.PfandAn == "" && q.Wert == 0 && q.WertAn == "" && q.Urteil == "" &&
		q.V == 0 && q.Runde == 0 && q.DrandSig == ""
}

// ------------------------------------------------------------ Urteil

// quantenSNullifierVonLocked: der Nullifier der Wallet ("" = keiner).
// Fehler = nicht lesbar oder nicht eindeutig.
func (cs *ChainState) quantenSNullifierVonLocked(ctx context.Context, w string) (string, error) {
	if cs.db == nil {
		cs.nullifiersMu.Lock()
		defer cs.nullifiersMu.Unlock()
		var gefunden []string
		for n, x := range cs.nullifiers {
			if strings.EqualFold(x, w) {
				gefunden = append(gefunden, n)
			}
		}
		if len(gefunden) > 1 {
			return "", errors.New("Nullifier nicht eindeutig")
		}
		if len(gefunden) == 1 {
			return gefunden[0], nil
		}
		return "", nil
	}
	rows, err := cs.dbExecCtx(ctx).Query(`SELECT nullifier FROM nullifiers WHERE lower(wallet_address) = $1 LIMIT 2`, w)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var gefunden []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return "", err
		}
		gefunden = append(gefunden, n)
	}
	if err := rows.Err(); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if len(gefunden) > 1 {
		return "", errors.New("Nullifier nicht eindeutig")
	}
	if len(gefunden) == 1 {
		return gefunden[0], nil
	}
	return "", nil
}

// quantenSWegBBescheidLocked: Melden, Abmelden, Bestellen, Losen, Verfallen,
// Pfand zurueck. Geld bewegen nur die Pfaender (W-Pfandtopf). Caller haelt
// cs.mu.
func (cs *ChainState) quantenSWegBBescheidLocked(ctx context.Context, tx *Transaction, wallet string, reg *quantenWRegister, zeit int64) (quantenWBescheid, error) {
	q := tx.QuantenW
	if !quantenSWegBAktiv(zeit) {
		return quantenWBefund("quanten_s_weg_b_inaktiv", "Weg B ist noch nicht aktiv"), nil
	}
	neu := reg.kopie()
	u := quantenWBescheid{}
	pfandRein := func(p int64) (quantenWBescheid, bool) {
		if cs.quantenWMikroLocked(ctx, wallet) < p {
			return quantenWBefund("quanten_w_deckung", "%s hat keine %d Mikro-AEQ Pfand", kurzAdresse(wallet), p), false
		}
		u.wege = []quantenWWeg{{wallet, quantenWPfandAddr, p}}
		u.felder.Pfand = p
		return quantenWBescheid{}, true
	}
	switch tx.Type {
	case quantenSMeldenTxArt, quantenSAbmeldenTxArt, quantenSBestellenTxArt:
		if !kanonischeAdresse(wallet) || istSystemAdresse(wallet) {
			return quantenWBefund("quanten_w_form", "%s: Wallet", tx.Type), nil
		}
		cs.ensureAccountLoadedCtx(ctx, wallet)
		if acc, ok := cs.accounts.Get(wallet); !ok || !acc.IsHuman {
			return quantenWBefund("quanten_s_mensch", "%s ist kein registrierter Mensch", kurzAdresse(wallet)), nil
		}
	}
	switch tx.Type {
	case quantenSMeldenTxArt:
		if err := quantenWUnterschrift(q, quantenSMeldenNachricht(wallet, tx.DistributionAt), wallet, zeit); err != nil {
			return quantenWBefund("quanten_w_unterschrift", "%s: %v", kurzAdresse(wallet), err), nil
		}
		if _, da := neu.Praep[wallet]; da {
			return quantenWBefund("quanten_s_register", "%s steht schon im Register (nach dem Abmelden erst nach Ablauf wieder)", kurzAdresse(wallet)), nil
		}
		n, err := cs.quantenSNullifierVonLocked(ctx, wallet)
		if err != nil {
			return quantenWBescheid{}, fmt.Errorf("quanten_w: Nullifier nicht lesbar: %w", err)
		}
		if !quantenSNullifierForm(n) {
			return quantenWBefund("quanten_s_mensch", "%s hat keinen Nullifier", kurzAdresse(wallet)), nil
		}
		if len(neu.Praep) >= quantenSPraepHoechstens {
			return quantenWBefund("quanten_s_register", "Register voll"), nil
		}
		// Ein Mensch, ein Praeparator: ein aktiver Eintrag desselben Menschen
		// (alte Wallet nach einem Umzug) wird jetzt abgemeldet.
		for k, p := range neu.Praep {
			if p.Nullifier == n && p.Abgemeldet == 0 {
				p.Abgemeldet = zeit
				neu.Praep[k] = p
			}
		}
		if b, ok := pfandRein(quantenSPraepPfand); !ok {
			return b, nil
		}
		neu.Praep[wallet] = quantenSPraep{Nullifier: n, Gemeldet: zeit, Pfand: quantenSPraepPfand}

	case quantenSAbmeldenTxArt:
		p, da := neu.Praep[wallet]
		if !da || p.Abgemeldet != 0 {
			return quantenWBefund("quanten_s_register", "%s ist nicht aktiv gemeldet", kurzAdresse(wallet)), nil
		}
		if err := quantenWUnterschrift(q, quantenSAbmeldenNachricht(wallet, p.Gemeldet, tx.DistributionAt), wallet, zeit); err != nil {
			return quantenWBefund("quanten_w_unterschrift", "%s: %v", kurzAdresse(wallet), err), nil
		}
		p.Abgemeldet = zeit
		neu.Praep[wallet] = p

	case quantenSBestellenTxArt:
		if !quantenSVTGueltig(q.V, q.T) {
			return quantenWBefund("quanten_w_form", "Bestellung: v = %d, t = %d", q.V, q.T), nil
		}
		if err := quantenWUnterschrift(q, quantenSBestellenNachricht(wallet, q.K, q.V, q.T, tx.DistributionAt), wallet, zeit); err != nil {
			return quantenWBefund("quanten_w_unterschrift", "%s: %v", kurzAdresse(wallet), err), nil
		}
		if reg.Noten[q.K] != nil || reg.Token[q.K] != nil || reg.Tote[q.K] != 0 || neu.Bestellt[q.K].Zeit != 0 {
			return quantenWBefund("quanten_s_bestellung", "K %s ist schon vergeben", q.K[:12]), nil
		}
		for _, b := range neu.Bestellt {
			if b.Von == wallet {
				return quantenWBefund("quanten_s_bestellung", "%s hat schon eine Bestellung (eine verfallene erst mit quanten_w_s_verfallen raeumen)", kurzAdresse(wallet)), nil
			}
		}
		if len(neu.Bestellt) >= quantenSBestellHoechstens {
			return quantenWBefund("quanten_s_bestellung", "zu viele offene Bestellungen"), nil
		}
		if b, ok := pfandRein(quantenSBestellPfand); !ok {
			return b, nil
		}
		neu.Bestellt[q.K] = quantenSBestellung{Von: wallet, V: q.V, T: q.T, Zeit: zeit, Pfand: quantenSBestellPfand}

	case quantenSLosenTxArt:
		b, da := neu.Bestellt[q.K]
		if !da || b.Runde != 0 || quantenSBestellungVerfallen(b, zeit) {
			return quantenWBefund("quanten_s_los", "keine ungeloste, laufende Bestellung zu %s", q.K[:12]), nil
		}
		soll := quantenSLosRunde(b.Zeit)
		if q.Runde != soll {
			return quantenWBefund("quanten_s_los", "Runde %d, fest ist %d", q.Runde, soll), nil
		}
		zufall, err := quantenSDrand.pruefen(q.Runde, q.DrandSig)
		if err != nil {
			return quantenWBefund("quanten_s_los", "%v", err), nil
		}
		los, err := quantenSLos(quantenSAbbild(neu.Praep, b.Zeit), zufall, q.K, b.V)
		if err != nil {
			return quantenWBefund("quanten_s_los", "%v", err), nil
		}
		b.Runde, b.Los = q.Runde, los
		neu.Bestellt[q.K] = b

	case quantenSVerfallenTxArt:
		b, da := neu.Bestellt[q.K]
		if !da || !quantenSBestellungVerfallen(b, zeit) {
			return quantenWBefund("quanten_s_bestellung", "keine verfallene Bestellung zu %s", q.K[:12]), nil
		}
		delete(neu.Bestellt, q.K)
		u.wege = []quantenWWeg{{quantenWPfandAddr, ubiPoolAddr, b.Pfand}}
		u.felder.Pfand = b.Pfand // PfandAn "" = Grundeinkommen

	case quantenSPfandZurueckTxArt:
		konto := strings.ToLower(strings.TrimSpace(q.An))
		p, da := neu.Praep[konto]
		if q.An != konto || !da || !quantenSPfandFrei(neu, p, zeit) {
			return quantenWBefund("quanten_s_register", "kein Eintrag zu %q, dessen Pfand frei ist", q.An), nil
		}
		an := konto
		if p.Erbe != "" {
			an = p.Erbe // nach einem Umzug: die neue Wallet, nie die alte
		}
		delete(neu.Praep, konto)
		u.wege = []quantenWWeg{{quantenWPfandAddr, an, p.Pfand}}
		u.felder.Pfand, u.felder.PfandAn = p.Pfand, an
	}
	u.reg = neu
	return u, nil
}

// quantenSLosBestaetigt: jeder Geloste hat nachricht(x) per EIP-191
// unterschrieben (Ausgabe gegen das Los, B2b).
func quantenSLosBestaetigt(los, bestaetigungen []string, nachricht func(x int) string) quantenWBescheid {
	if len(bestaetigungen) != len(los) {
		return quantenWBefund("quanten_w_form", "Bestaetigungen")
	}
	for x, v := range los {
		if !kanonischeSignatur(bestaetigungen[x]) || pruefePersonalSignGemerkt(nachricht(x), bestaetigungen[x], v) != nil {
			return quantenWBefund("quanten_w_bestaetigung", "Praeparator %d (%s) hat nicht bestaetigt", x+1, kurzAdresse(v))
		}
	}
	return quantenWBescheid{}
}

// quantenSUmzugLocked: beim Vollzug eines Umzugs den aktiven Eintrag der
// alten Wallet abmelden (zur Umzugszeit; fuer fruehere Bestellungen zaehlt er
// weiter, wie jede Abmeldung). Caller haelt cs.mu, in der Transaktion des
// Blocks.
func (cs *ChainState) quantenSUmzugLocked(ctx context.Context, alt, nach string, t int64) error {
	reg := cs.quantenWJetzt()
	neu := reg.kopie()
	geaendert := false
	// Fruehere Umzuege desselben Menschen: ihr Pfand geht jetzt an die neue Wallet.
	for k, e := range neu.Praep {
		if e.Erbe == alt {
			e.Erbe = nach
			neu.Praep[k] = e
			geaendert = true
		}
	}
	p, da := neu.Praep[alt]
	if !da || p.Erbe != "" {
		if geaendert {
			return cs.quantenWSetzenLocked(ctx, neu)
		}
		return nil
	}
	if p.Abgemeldet == 0 {
		p.Abgemeldet = max(t, p.Gemeldet)
	}
	p.Erbe = nach // das Pfand gehoert dem Menschen, nicht der alten Wallet
	neu.Praep[alt] = p
	return cs.quantenWSetzenLocked(ctx, neu)
}
