package keeper

// OFFLINE-BARGELD -- Stufe 1b: Einbindung (docs/OFFLINE_BARGELD.md).
//
// Stufe 1a (offline_bargeld.go) hat die Regeln als reine Funktionen ueber dem
// Register gebaut. Hier haengen sie an der Kette: vier Transaktionsarten,
// Annahme und Nachspielen mit DERSELBEN Pruefung, das Register in
// chain_config, StateRoot (nur nicht leer), Snapshot, Checkpoint und
// Rueckroll. Alles SCHLAFEND bis offlineAbUnix (MaxInt64): davor sind die
// Arten unbekannt (bekannteTxArt), die Endpunkte antworten 409, kein Block
// traegt sie, und das leere Register aendert keine Wurzel.
//
// DIE VIER ARTEN
//
//   - offline_pfand (Wallet = Mensch): sperrt Offline.Betrag im Offline-Topf
//     und bindet ihn an das Geraet Offline.Geraet. Der Mensch unterschreibt
//     offlinePfandNachricht (Mensch, Geraet, Betrag, Zeit) mit seiner Wallet
//     (EIP-191) oder -- liegt sein Menschsein im Tresor -- mit ML-DSA-65
//     (PQKonto, umzug_pq.go). Eine eigene Art statt einer Ueberweisung an den
//     Topf: die Unterschrift bindet das GERAET an den Menschen; eine
//     Ueberweisung allein liesse jeden Knoten, der sie weiterreicht, ein
//     anderes Geraet einsetzen.
//   - offline_einloesung (Wallet = Empfaenger): eine Zahlung, die das Geraet
//     unterschrieben hat (offlineZahlungNachricht). Jeder Knoten prueft
//     Pfand, Unterschrift, Zaehler und Betrag selbst und rechnet, was aus
//     dem Topf, was vom Konto des Halters kommt und was Schuld bleibt; der
//     Block nennt diese drei Zahlen, und sie muessen genau stimmen.
//   - offline_kuendigung (Wallet = Mensch): unterschrieben wie das Pfand.
//   - offline_abschluss (Wallet = Mensch): entsteht nur in der Tagesrunde,
//     fruehestens offlineAbschlussAb nach der Kuendigung; zahlt den Rest des
//     Pfands aus -- zuerst an offene Forderungen, dann an den Halter.
//
// GELD
//
// Nur diese Wege bewegen Geld vom und zum Topf: Pfand (Mensch -> Topf),
// Einloesung (Topf -> Empfaenger, hoechstens das verfuegbare Pfand; dazu
// Mensch -> Empfaenger, hoechstens sein Guthaben, nur wenn das Pfand nicht
// reicht), Abschluss (Topf -> Glaeubiger/Halter, genau der Rest). Eine
// gewoehnliche Ueberweisung an den Topf nimmt kein Knoten an
// (pruefeTopfNurMitNote) und ab dem Stichtag traegt sie kein Block
// (offlineFeldErlaubt). Der Topf ist ein Protokoll-Topf
// (isTokenomicsPoolAddress): keine Umlaufabgabe, keine Vermoegensgrenze,
// kein Absenden mit Schluessel -- so bleibt seine Summe gleich der Summe
// der verfuegbaren Pfaender.
//
// Ohne Gebuehr (wie Grundeinkommen und Quantengeld aus ihren Toepfen); die
// Umlaufsicherung gilt fuer den Empfaenger danach wie fuer jedes Konto.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"time"
)

const (
	offlinePfandTxArt      = "offline_pfand"
	offlineEinloesungTxArt = "offline_einloesung"
	offlineKuendigungTxArt = "offline_kuendigung"
	offlineAbschlussTxArt  = "offline_abschluss"

	offlineDomaenePfand      = "aequitas-offline-pfand-v1"
	offlineDomaeneKuendigung = "aequitas-offline-kuendigung-v1"

	// Pfand und Kuendigung: hoechstens 5 min nach und einen Tag vor dem
	// Block unterschrieben (wie Umzug und PQ-Zusage).
	offlineHoechstensVoraus int64 = 300
	offlineHoechstensAlt    int64 = 86400

	// offlineAbschluesseJeRunde: so viele Abschluesse je Tagesrunde, der
	// Rest am naechsten Tag (wie umzugVollzugJeRunde).
	offlineAbschluesseJeRunde = 200

	// Koerper der Endpunkte: eine Unterschrift mit ML-DSA (~10,6 KB) plus
	// Felder; 16 KiB wie die PQ-Ueberweisung.
	offlineKoerperMax = 16 << 10
	burstOfflineJeIP  = 20
)

// OfflineAngaben: was die Offline-Arten zusaetzlich tragen (Transaction.Offline).
type OfflineAngaben struct {
	Geraet string `json:"geraet,omitempty"`
	// Betrag in Mikro-AEQ: Pfand (offline_pfand) bzw. Zahlung (Einloesung).
	Betrag  int64  `json:"betrag,omitempty"`
	Zaehler int64  `json:"zaehler,omitempty"`
	Sig     string `json:"sig,omitempty"` // Geraet, P-256 (Einloesung)
	// Unterschrift des Menschen (Pfand, Kuendigung): EIP-191 ODER ML-DSA.
	WalletSig string   `json:"wallet_sig,omitempty"`
	PQ        *PQKonto `json:"pq,omitempty"`
	// Einloesung: vom Erzeuger gerechnet, jeder Knoten rechnet nach.
	AusTopf  int64 `json:"aus_topf,omitempty"`
	VomKonto int64 `json:"vom_konto,omitempty"`
	Schuld   int64 `json:"schuld,omitempty"`
	// Abschluss: an wen der Rest geht (in dieser Reihenfolge).
	Auszahlungen []OfflineAuszahlung `json:"auszahlungen,omitempty"`
}

// OfflineAuszahlung: ein Posten des Abschlusses.
type OfflineAuszahlung struct {
	An    string `json:"an"`
	Mikro int64  `json:"mikro"`
}

func istOfflineTxArt(t string) bool {
	switch t {
	case offlinePfandTxArt, offlineEinloesungTxArt, offlineKuendigungTxArt, offlineAbschlussTxArt:
		return true
	}
	return false
}

// ------------------------------------------------------------ Saetze

func offlinePfandNachricht(mensch, geraet string, betragMik, zeit int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%d|%d", offlineDomaenePfand, eingefroreneChainID,
		strings.ToLower(mensch), strings.ToLower(geraet), betragMik, zeit)
}

func offlineKuendigungNachricht(mensch, geraet string, zeit int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%d", offlineDomaeneKuendigung, eingefroreneChainID,
		strings.ToLower(mensch), strings.ToLower(geraet), zeit)
}

// offlineWalletUnterschrift: hat wallet die Nachricht unterschrieben -- genau
// eins von EIP-191 (WalletSig) oder ML-DSA-65 einer PQ-Adresse (PQ, ab
// umzugPQAb)? Form vor Rechnung.
func offlineWalletUnterschrift(o *OfflineAngaben, nachricht, wallet string, blockZeit int64) error {
	if o.PQ != nil {
		if o.WalletSig != "" || !umzugPQAktiv(blockZeit) || !pqKontoForm(o.PQ) {
			return errors.New("PQ-Unterschrift nicht in gueltiger Form")
		}
		return pqKontoPruefen(wallet, nachricht, o.PQ)
	}
	if !kanonischeSignatur(o.WalletSig) {
		return errors.New("Unterschrift der Wallet fehlt oder nicht in gueltiger Form")
	}
	return pruefePersonalSignGemerkt(nachricht, o.WalletSig, wallet)
}

// ------------------------------------------------------------ Register

// offlineStand: das geltende Register und sein Teil der StateRoot, einmal
// beim Setzen gerechnet -- die Wurzel jedes Blocks liest nur den fertigen
// Hash, statt das Register neu zu kodieren.
type offlineStand struct {
	reg    *offlineRegister
	wurzel string
}

func neuerOfflineStand(r *offlineRegister, kodiert string) *offlineStand {
	st := &offlineStand{reg: r}
	if kodiert != "" {
		h := sha256.Sum256([]byte(kodiert))
		st.wurzel = "|offline:" + hex.EncodeToString(h[:])
	}
	return st
}

func (cs *ChainState) offlineStandJetzt() *offlineStand {
	if st := cs.offlineReg.Load(); st != nil {
		return st
	}
	return &offlineStand{reg: &offlineRegister{}}
}

func (cs *ChainState) offlineJetzt() *offlineRegister { return cs.offlineStandJetzt().reg }

// offlineWurzel: der Teil der StateRoot ("" bei leerem Register: byte-gleich).
func (cs *ChainState) offlineWurzel() string { return cs.offlineStandJetzt().wurzel }

// offlineLaden: das Register aus chain_config (Start, Snapshot, Resync).
func (cs *ChainState) offlineLaden(ctx context.Context) error {
	if cs.db == nil {
		return nil
	}
	r, err := offlineRegisterLesen(cs.getConfigValueCtx(ctx, offlineKonfigSchluessel))
	if err != nil {
		return err
	}
	cs.offlineReg.Store(neuerOfflineStand(r, r.kodiert()))
	return nil
}

// offlineSetzenLocked: in der laufenden Datenbank-Transaktion schreiben.
func (cs *ChainState) offlineSetzenLocked(ctx context.Context, r *offlineRegister) error {
	k := r.kodiert()
	if cs.db != nil {
		if err := cs.setConfigValueCtx(ctx, offlineKonfigSchluessel, k); err != nil {
			return fmt.Errorf("Offline-Register speichern: %w", err)
		}
	}
	cs.offlineReg.Store(neuerOfflineStand(r, k))
	return nil
}

// offlineNurEigeneFelder: eine Offline-Transaktion traegt nur ihre eigenen
// Felder. Kein Erzeuger legt etwas hinein, das ein anderer Teil der Kette
// lesen koennte (Ziel, Gebuehr, Nachweise, Nullifier ...).
func offlineNurEigeneFelder(tx *Transaction) bool {
	k := *tx
	k.Type, k.Wallet, k.Amount, k.DistributionAt, k.Offline, k.TxHash = "", "", 0, 0, nil, ""
	return reflect.DeepEqual(k, Transaction{})
}

// offlineBetragImBlock: was Transaction.Amount bei dieser Art sein muss --
// die Summe, die der Empfaenger bekommt (Anzeige, Verlauf). Der Erzeuger
// rechnet sie, jeder Nachspielende rechnet nach.
func offlineBetragImBlock(tx *Transaction, u offlineUrteil) float64 {
	switch tx.Type {
	case offlinePfandTxArt:
		return Decimal(u.pfand).Float()
	case offlineEinloesungTxArt:
		return Decimal(u.ausTopf + u.vomKonto).Float()
	}
	return 0
}

// offlineUmzug: zieht ein Mensch um (umzug.go), gehen sein aktives Pfand,
// seine archivierten Pfaender, die Sperre und die Schulden mit. Sonst
// entkaeme ein Betrueger Sperre und Schuld, indem er umzieht, und haette mit
// der neuen Adresse ein zweites Pfand. nil = nichts zu tun (byte-gleich).
// Die neue Adresse steht nie schon im Register: umzugZielTauglichLocked
// weist sie vorher ab; hier ist es ein Fehler, nie ein Zusammenlegen.
func offlineUmzug(reg *offlineRegister, alt, neu string) (*offlineRegister, error) {
	alt, neu = strings.ToLower(alt), strings.ToLower(neu)
	if !reg.kenntMensch(alt) {
		return nil, nil
	}
	if reg.kenntMensch(neu) {
		return nil, fmt.Errorf("offline: %s steht schon im Offline-Register", kurzAdresse(neu))
	}
	n := reg.kopie()
	if g, da := n.Aktiv[alt]; da {
		delete(n.Aktiv, alt)
		n.Aktiv[neu] = g
	}
	if n.Gesperrt[alt] {
		delete(n.Gesperrt, alt)
		n.Gesperrt[neu] = true
	}
	if ss, da := n.Schulden[alt]; da {
		delete(n.Schulden, alt)
		n.Schulden[neu] = ss
	}
	for g, p := range n.Pfaender {
		if p.Mensch == alt {
			n.pfandAendern(g).Mensch = neu
		}
	}
	return n, nil
}

// offlineUmzugLocked: in umzugVollziehenLocked, in der Transaktion des Blocks.
func (cs *ChainState) offlineUmzugLocked(ctx context.Context, alt, neu string) error {
	n, err := offlineUmzug(cs.offlineJetzt(), alt, neu)
	if err != nil || n == nil {
		return err
	}
	return cs.offlineSetzenLocked(ctx, n)
}

// ------------------------------------------------------------ Form im Block

// offlineFeldErlaubt: Offline-Angaben nur an den Offline-Arten, die Arten nur
// ab dem Stichtag, und ab dem Stichtag keine Ueberweisung an den Topf (die
// waere Deckung ohne Pfand). Jeder Knoten, beim Nachspielen.
func offlineFeldErlaubt(tx *Transaction, blockZeit int64) error {
	art := istOfflineTxArt(tx.Type)
	if tx.Offline == nil && !art {
		if tx.Type == "transfer" && offlineAktiv(blockZeit) && strings.EqualFold(strings.TrimSpace(tx.To), offlineTopfAddr) {
			return errors.New("Ueberweisung an den Offline-Topf ohne Pfand")
		}
		return nil
	}
	if tx.Offline == nil || !art {
		return fmt.Errorf("%s: Offline-Angaben nur an den Offline-Arten", tx.Type)
	}
	if !offlineAktiv(blockZeit) {
		return fmt.Errorf("%s vor dem Stichtag (Block %d)", tx.Type, blockZeit)
	}
	return nil
}

// offlineZeitFenster: Pfand und Kuendigung tragen ihre Zeit in
// DistributionAt; der Abschluss die Rundenzeit.
func offlineZeitGueltig(tx *Transaction, blockZeit int64) bool {
	z := tx.DistributionAt
	switch tx.Type {
	case offlinePfandTxArt, offlineKuendigungTxArt:
		return z > 0 && z <= sattAdd(blockZeit, offlineHoechstensVoraus) && blockZeit-z <= offlineHoechstensAlt
	case offlineAbschlussTxArt:
		return z > 0 && z <= sattAdd(blockZeit, 60)
	}
	return true
}

// ------------------------------------------------------------ Urteile

// offlineUrteil: was eine Offline-Transaktion bewirkt -- oder warum nicht.
type offlineUrteil struct {
	regel, text string
	reg         *offlineRegister
	// Geldwege, in Mikro-AEQ.
	pfand    int64 // Mensch -> Topf
	ausTopf  int64 // Topf -> Empfaenger
	vomKonto int64 // Mensch -> Empfaenger
	schuld   int64
	auszahl  []OfflineAuszahlung // Topf -> An
}

func offlineBefund(regel, format string, args ...interface{}) offlineUrteil {
	return offlineUrteil{regel: regel, text: fmt.Sprintf(format, args...)}
}

func offlineAusFehler(err error) offlineUrteil {
	var a *offlineAbgelehnt
	if errors.As(err, &a) {
		return offlineUrteil{regel: a.regel, text: a.text}
	}
	return offlineUrteil{regel: "offline_form", text: err.Error()}
}

// guthabenMikro: das Guthaben einer Adresse in Mikro-AEQ (0 ohne Konto).
func (cs *ChainState) offlineGuthabenMikroLocked(ctx context.Context, addr string) int64 {
	cs.ensureAccountLoadedCtx(ctx, addr)
	if acc, ok := cs.accounts.Get(addr); ok && acc.Balance > 0 {
		return acc.Balance.Micro()
	}
	return 0
}

// offlineUrteilLocked: dieselbe Pruefung fuer Annahme (Uhrzeit) und
// Nachspielen (Blockzeit). Caller haelt cs.mu.
func (cs *ChainState) offlineUrteilLocked(ctx context.Context, tx *Transaction, blockZeit int64) offlineUrteil {
	o := tx.Offline
	if !offlineAktiv(blockZeit) {
		return offlineBefund("offline_inaktiv", "Offline-Bargeld ist noch nicht aktiv")
	}
	if o == nil || !istOfflineTxArt(tx.Type) || !offlineNurEigeneFelder(tx) {
		return offlineBefund("offline_form", "keine Offline-Transaktion in gueltiger Form")
	}
	if !offlineZeitGueltig(tx, blockZeit) {
		return offlineBefund("offline_zeit", "%s: Zeit %d passt nicht zu Block %d", tx.Type, tx.DistributionAt, blockZeit)
	}
	wallet := strings.ToLower(strings.TrimSpace(tx.Wallet))
	if !kanonischeAdresse(wallet) {
		return offlineBefund("offline_form", "Wallet nicht kanonisch")
	}
	reg := cs.offlineJetzt()
	switch tx.Type {
	case offlinePfandTxArt:
		if o.Zaehler != 0 || o.Sig != "" || o.AusTopf != 0 || o.VomKonto != 0 || o.Schuld != 0 || len(o.Auszahlungen) != 0 {
			return offlineBefund("offline_form", "Pfand nicht in gueltiger Form")
		}
		if err := offlineWalletUnterschrift(o, offlinePfandNachricht(wallet, o.Geraet, o.Betrag, tx.DistributionAt), wallet, blockZeit); err != nil {
			return offlineBefund("offline_unterschrift", "%s: %v", kurzAdresse(wallet), err)
		}
		cs.ensureAccountLoadedCtx(ctx, wallet)
		acc, ok := cs.accounts.Get(wallet)
		neu, err := offlinePfandAnlegen(reg, wallet, ok && acc.IsHuman, o.Geraet, o.Betrag, blockZeit)
		if err != nil {
			return offlineAusFehler(err)
		}
		if cs.offlineGuthabenMikroLocked(ctx, wallet) < o.Betrag {
			return offlineBefund("offline_deckung", "%s hat keine %d Mikro-AEQ", kurzAdresse(wallet), o.Betrag)
		}
		return offlineUrteil{reg: neu, pfand: o.Betrag}

	case offlineEinloesungTxArt:
		if o.WalletSig != "" || o.PQ != nil || len(o.Auszahlungen) != 0 || o.AusTopf < 0 || o.VomKonto < 0 || o.Schuld < 0 {
			return offlineBefund("offline_form", "Einloesung nicht in gueltiger Form")
		}
		neu, e, err := offlineEinloesen(reg, o.Geraet, o.Zaehler, wallet, o.Betrag, o.Sig, blockZeit)
		if err != nil {
			return offlineAusFehler(err)
		}
		// Was das Pfand nicht deckt, kommt vom Konto des Halters -- hoechstens
		// sein Guthaben; der Rest wird Schuld.
		vomKonto := e.Rest
		if g := cs.offlineGuthabenMikroLocked(ctx, e.Mensch); vomKonto > g {
			vomKonto = g
		}
		schuld := e.Rest - vomKonto
		if neu, err = offlineSchuldEintragen(neu, e.Mensch, wallet, schuld); err != nil {
			return offlineAusFehler(err)
		}
		return offlineUrteil{reg: neu, ausTopf: e.AusTopf, vomKonto: vomKonto, schuld: schuld}

	case offlineKuendigungTxArt:
		if o.Betrag != 0 || o.Zaehler != 0 || o.Sig != "" || o.AusTopf != 0 || o.VomKonto != 0 || o.Schuld != 0 || len(o.Auszahlungen) != 0 {
			return offlineBefund("offline_form", "Kuendigung nicht in gueltiger Form")
		}
		g, da := reg.Aktiv[wallet]
		if !da || g != o.Geraet {
			return offlineBefund("offline_unbekannt", "%s hat kein Pfand mit diesem Geraet", kurzAdresse(wallet))
		}
		if err := offlineWalletUnterschrift(o, offlineKuendigungNachricht(wallet, o.Geraet, tx.DistributionAt), wallet, blockZeit); err != nil {
			return offlineBefund("offline_unterschrift", "%s: %v", kurzAdresse(wallet), err)
		}
		neu, err := offlineKuendigen(reg, wallet, blockZeit)
		if err != nil {
			return offlineAusFehler(err)
		}
		return offlineUrteil{reg: neu}

	case offlineAbschlussTxArt:
		if o.Betrag != 0 || o.Zaehler != 0 || o.Sig != "" || o.WalletSig != "" || o.PQ != nil || o.AusTopf != 0 || o.VomKonto != 0 || o.Schuld != 0 {
			return offlineBefund("offline_form", "Abschluss nicht in gueltiger Form")
		}
		g, da := reg.Aktiv[wallet]
		if !da || g != o.Geraet {
			return offlineBefund("offline_unbekannt", "%s hat kein Pfand mit diesem Geraet", kurzAdresse(wallet))
		}
		// Rundenzeit statt Blockzeit: Erzeuger und Nachspielende rechnen mit
		// derselben Zeit.
		neu, aus, err := offlineAbschliessen(reg, wallet, tx.DistributionAt)
		if err != nil {
			return offlineAusFehler(err)
		}
		var posten []OfflineAuszahlung
		for _, a := range aus {
			posten = append(posten, OfflineAuszahlung{An: a.An, Mikro: a.Mikro})
		}
		return offlineUrteil{reg: neu, auszahl: posten}
	}
	return offlineBefund("offline_form", "unbekannte Art %q", tx.Type)
}

// offlineBewegen: Mikro-AEQ von -> an, ohne Gebuehr. Fehler: Zustand lehnt
// ab (Deckung fehlt) -- dann wird nichts angewandt.
func (cs *ChainState) offlineBewegenLocked(ctx context.Context, von, an string, mikro int64) error {
	if mikro <= 0 {
		return nil
	}
	if von == an {
		return fmt.Errorf("offline: %s an sich selbst: %w", kurzAdresse(von), ErrZustandLehntAb)
	}
	cs.ensureAccountLoadedCtx(ctx, von)
	cs.ensureAccountLoadedCtx(ctx, an)
	quelle, ok := cs.accounts.Get(von)
	betrag := Decimal(mikro)
	if !ok || quelle.Balance < betrag {
		return fmt.Errorf("offline: %s deckt %d Mikro-AEQ nicht: %w", kurzAdresse(von), mikro, ErrZustandLehntAb)
	}
	ziel, da := cs.accounts.Get(an)
	if !da {
		ziel = &AccountState{Address: an}
		cs.accounts.Set(an, ziel)
	}
	quelle.Balance = quelle.Balance.Sub(betrag)
	ziel.Balance = ziel.Balance.Add(betrag)
	for _, a := range []*AccountState{quelle, ziel} {
		cs.updateAccountLeafLocked(a)
		if err := cs.saveAccountToDBCtx(ctx, a); err != nil {
			return fmt.Errorf("offline: %w", err)
		}
	}
	return nil
}

// offlineAnwendenLocked: nach einem Urteil ohne Befund -- Geld bewegen, dann
// das Register setzen. Die Summen pruefen offlineBewegenLocked (Deckung) und
// das Register (Pfand begrenzt den Topf).
func (cs *ChainState) offlineAnwendenLocked(ctx context.Context, tx *Transaction, u offlineUrteil) error {
	wallet := strings.ToLower(strings.TrimSpace(tx.Wallet))
	switch tx.Type {
	case offlinePfandTxArt:
		if err := cs.offlineBewegenLocked(ctx, wallet, offlineTopfAddr, u.pfand); err != nil {
			return err
		}
	case offlineEinloesungTxArt:
		mensch := ""
		if p := u.reg.Pfaender[tx.Offline.Geraet]; p != nil {
			mensch = p.Mensch
		}
		if err := cs.offlineBewegenLocked(ctx, offlineTopfAddr, wallet, u.ausTopf); err != nil {
			return err
		}
		if err := cs.offlineBewegenLocked(ctx, mensch, wallet, u.vomKonto); err != nil {
			return err
		}
	case offlineAbschlussTxArt:
		for _, a := range u.auszahl {
			if err := cs.offlineBewegenLocked(ctx, offlineTopfAddr, a.An, a.Mikro); err != nil {
				return err
			}
		}
	}
	return cs.offlineSetzenLocked(ctx, u.reg)
}

// applyOfflineLocked: beim Nachspielen. Die Zahlen im Block muessen genau
// die gerechneten sein. Fehler: der Block ist falsch (nichts angewandt).
func (cs *ChainState) applyOfflineLocked(ctx context.Context, tx *Transaction, blockZeit int64) error {
	u := cs.offlineUrteilLocked(ctx, tx, blockZeit)
	if u.regel != "" {
		return fmt.Errorf("%s %s: %s", tx.Type, u.regel, u.text)
	}
	o := tx.Offline
	if tx.Amount != offlineBetragImBlock(tx, u) {
		return fmt.Errorf("%s: Betrag im Block %v, gerechnet %v", tx.Type, tx.Amount, offlineBetragImBlock(tx, u))
	}
	switch tx.Type {
	case offlineEinloesungTxArt:
		if o.AusTopf != u.ausTopf || o.VomKonto != u.vomKonto || o.Schuld != u.schuld {
			return fmt.Errorf("offline_einloesung: Block nennt %d/%d/%d (Topf/Konto/Schuld), gerechnet %d/%d/%d",
				o.AusTopf, o.VomKonto, o.Schuld, u.ausTopf, u.vomKonto, u.schuld)
		}
	case offlineAbschlussTxArt:
		if !offlineAuszahlungenGleich(o.Auszahlungen, u.auszahl) {
			return fmt.Errorf("offline_abschluss: Auszahlungen im Block weichen ab")
		}
	}
	return cs.offlineAnwendenLocked(ctx, tx, u)
}

func offlineAuszahlungenGleich(a, b []OfflineAuszahlung) bool {
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

// ------------------------------------------------------------ Tagesrunde

// offlineAbschluesseLocked: die Tagesrunde schliesst jedes gekuendigte Pfand
// ab, dessen Frist zur Rundenzeit abgelaufen ist (hoechstens
// offlineAbschluesseJeRunde, nach Mensch geordnet), und gibt die
// Transaktionen fuer den Block zurueck. Caller haelt cs.mu, in der
// Datenbank-Transaktion der Runde.
func (cs *ChainState) offlineAbschluesseLocked(ctx context.Context, ubiAt int64) ([]Transaction, error) {
	if !offlineAktiv(ubiAt) {
		return nil, nil
	}
	reg := cs.offlineJetzt()
	var faellig []string
	for m, g := range reg.Aktiv {
		if p := reg.Pfaender[g]; p != nil && p.Gekuendigt != 0 && ubiAt >= offlineAbschlussAb(p.Gekuendigt) {
			faellig = append(faellig, m)
		}
	}
	sort.Strings(faellig)
	if len(faellig) > offlineAbschluesseJeRunde {
		faellig = faellig[:offlineAbschluesseJeRunde]
	}
	var txs []Transaction
	for _, m := range faellig {
		tx := Transaction{Type: offlineAbschlussTxArt, Wallet: m, DistributionAt: ubiAt,
			Offline: &OfflineAngaben{Geraet: cs.offlineJetzt().Aktiv[m]}}
		u := cs.offlineUrteilLocked(ctx, &tx, ubiAt)
		if u.regel != "" {
			return nil, fmt.Errorf("offline_abschluss %s: %s: %s", kurzAdresse(m), u.regel, u.text)
		}
		tx.Offline.Auszahlungen = u.auszahl
		if err := cs.offlineAnwendenLocked(ctx, &tx, u); err != nil {
			return nil, err
		}
		txs = append(txs, tx)
	}
	return txs, nil
}

// ------------------------------------------------------------ Annahme

type offlineAbgewiesen struct{ text string }

func (e *offlineAbgewiesen) Error() string { return e.text }

// offlineAnnehmen: unter der Sperre genau die Pruefung jedes Nachspielenden,
// Geld und Register atomar mit dem Ausgang. Gibt die Transaktion mit den
// gerechneten Zahlen zurueck.
func (cs *ChainState) offlineAnnehmen(tx Transaction) (Transaction, error) {
	wallet := strings.ToLower(strings.TrimSpace(tx.Wallet))
	konten := []string{wallet, offlineTopfAddr}
	if tx.Type == offlineEinloesungTxArt && tx.Offline != nil {
		if p := cs.offlineJetzt().Pfaender[tx.Offline.Geraet]; p != nil {
			konten = append(konten, p.Mensch)
		}
	}
	var fertig Transaction
	err := cs.runAtomicWithOutbox(konten, false, func(ctx context.Context) (Transaction, error) {
		at := nowUnix()
		u := cs.offlineUrteilLocked(ctx, &tx, at)
		if u.regel != "" {
			return Transaction{}, &offlineAbgewiesen{text: u.regel + ": " + u.text}
		}
		o := *tx.Offline
		if tx.Type == offlineEinloesungTxArt {
			o.AusTopf, o.VomKonto, o.Schuld = u.ausTopf, u.vomKonto, u.schuld
		}
		tx.Amount = offlineBetragImBlock(&tx, u)
		tx.Offline = &o
		if err := cs.offlineAnwendenLocked(ctx, &tx, u); err != nil {
			return Transaction{}, err
		}
		fertig = tx
		return tx, nil
	})
	return fertig, err
}

// handleOffline: POST /api/offline/pfand | /einloesung | /kuendigung.
//
//	pfand:      {"wallet","geraet","betrag","zeit","wallet_sig" | "pq_pub","pq_sig"}
//	einloesung: {"empfaenger","geraet","zaehler","betrag","sig"}
//	kuendigung: {"wallet","geraet","zeit","wallet_sig" | "pq_pub","pq_sig"}
//
// Vor dem Stichtag 409. Jeder Knoten prueft beim Nachspielen dasselbe.
func (a *APIServer) handleOffline(art string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSONCORS(w)
		if r.Method != http.MethodPost {
			jsonError(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		if !offlineAktiv(time.Now().Unix()) {
			jsonError(w, "offline cash not active yet", http.StatusConflict)
			return
		}
		if !burstErlaubt("offline:"+erneuerungsAbsender(r), burstOfflineJeIP, burstFenster) {
			jsonError(w, "too many requests from this address", http.StatusTooManyRequests)
			return
		}
		var req struct {
			Wallet     string `json:"wallet"`
			Empfaenger string `json:"empfaenger"`
			Geraet     string `json:"geraet"`
			Betrag     int64  `json:"betrag"`
			Zaehler    int64  `json:"zaehler"`
			Zeit       int64  `json:"zeit"`
			Sig        string `json:"sig"`
			WalletSig  string `json:"wallet_sig"`
			PQPub      string `json:"pq_pub"`
			PQSig      string `json:"pq_sig"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, offlineKoerperMax))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		o := &OfflineAngaben{Geraet: strings.ToLower(strings.TrimSpace(req.Geraet))}
		tx := Transaction{Type: art, Offline: o}
		switch art {
		case offlineEinloesungTxArt:
			tx.Wallet = strings.ToLower(strings.TrimSpace(req.Empfaenger))
			o.Zaehler, o.Betrag, o.Sig = req.Zaehler, req.Betrag, strings.ToLower(strings.TrimSpace(req.Sig))
		case offlinePfandTxArt, offlineKuendigungTxArt:
			tx.Wallet = strings.ToLower(strings.TrimSpace(req.Wallet))
			tx.DistributionAt = req.Zeit
			if art == offlinePfandTxArt {
				o.Betrag = req.Betrag
			}
			if req.PQPub != "" || req.PQSig != "" {
				o.PQ = &PQKonto{Pub: req.PQPub, Sig: req.PQSig}
			}
			o.WalletSig = kanonischeSignaturVersuch(req.WalletSig)
			if o.WalletSig == "" && req.WalletSig != "" {
				o.WalletSig = req.WalletSig // falsche Form: das Urteil weist ab
			}
		default:
			jsonError(w, "unknown offline type", http.StatusNotFound)
			return
		}
		fertig, err := a.state.offlineAnnehmen(tx)
		if err != nil {
			var ab *offlineAbgewiesen
			if errors.As(err, &ab) {
				jsonError(w, "rejected: "+ab.text, http.StatusConflict)
				return
			}
			if istWiederholbareAnnahmeAblehnung(err) {
				jsonError(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
			fmt.Printf("[API] %s fuer %s gescheitert: %v\n", art, kurzAdresse(tx.Wallet), err)
			jsonError(w, "internal error, please retry shortly", http.StatusInternalServerError)
			return
		}
		antwort := map[string]interface{}{"ok": true, "art": art}
		if art == offlineEinloesungTxArt {
			antwort["aus_topf"] = fertig.Offline.AusTopf
			antwort["vom_konto"] = fertig.Offline.VomKonto
			antwort["schuld"] = fertig.Offline.Schuld
		}
		json.NewEncoder(w).Encode(antwort)
	}
}

// offlineStatus: fuer /api/status ("offline"). ab fehlt, solange schlafend.
func offlineStatus() map[string]interface{} {
	m := map[string]interface{}{"topf": offlineTopfAddr}
	if ab := offlineAb(); ab != 1<<63-1 {
		m["ab"] = ab
	}
	return m
}
