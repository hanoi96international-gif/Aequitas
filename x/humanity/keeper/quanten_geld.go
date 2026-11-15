package keeper

// DEZENTRALES QUANTENGELD -- die Kettenseite (forschung/quantengeld/
// BAUVORGABE.md §8, labor.py, hardware.py; docs/QUANTENSICHERHEIT.md).
//
// # WAS DIE KETTE TUT
//
// Eine Quanten-Note ist ein Quantenzustand (Quantum Lightning, Zhandry 2025):
// PRAEGEN darf jeder, ohne Bank -- die Seriennummer h faellt gleichverteilt
// beim Messen, und niemand, auch der Praeger nicht, kann eine zweite Note mit
// demselben h herstellen. Wert bekommt eine Note erst durch ANMELDEN auf der
// Kette: der Anmelder ueberweist die Deckung an den Quantengeld-Topf und
// traegt K = SHA-256(Domaene | Satz | Anmelder | h) ein (quanten_note.go,
// quantenSerieSchluessel). Danach ist die Note ein Inhaberpapier: sie wandert
// als Quantenzustand, (Satz, Anmelder, h) als Zettel dazu, und JEDER
// Empfaenger prueft selbst -- K steht in der Kette, und der Zustand besteht
// den Quantentest (Variante B). Die Kette sieht nie einen Quantenzustand.
// Einweg (Version 1): aus dem Topf zahlt niemand aus.
//
// # ZWEI TRANSAKTIONSARTEN
//
//	quanten_freischaltung  traegt einen PARAMETERSATZ ein (Name, Gruppen-
//	    ordnung N, ab wann). Nur mit den Stimmen von mindestens zwei Dritteln
//	    der Pruefer (verifier_register.go, dieselbe Pruefung wie eine
//	    Aufnahme), nur N >= 2^256, nur mit quantenFreischaltungVorlauf
//	    Vorlauf. Die Stimme bescheinigt, dass es Hardware gibt, die Noten
//	    dieses Satzes laenger halten als eine Zahlung dauert, und Pruefgeraete
//	    dafuer -- das kann keine Kette messen, das entscheiden Menschen, und
//	    zwar dezentral.
//	transfer an quantenTopfAddr mit Quanten-Angaben  meldet eine Note an.
//	    Eine gewoehnliche, signierte Ueberweisung (EVM-Rohform oder PQ):
//	    Kontostand, Gebuehr, Nonce, Demurrage, Erhaltung und StateRoot der
//	    Konten laufen ueber den Ueberweisungsweg. Dazu unterschreibt die
//	    Wallet (EIP-191) Satz, Seriennummer und den TxHash der Ueberweisung.
//
// # VOR DEM STICHTAG
//
// Vor quantenGeldAbUnix sind beide Arten unbekannt
// (bekannteTxArt), Quanten-Angaben in einem Block machen ihn ungueltig, der
// Endpunkt antwortet 409. Leer ist das Register; dann bleiben StateRoot,
// Snapshot und Rueckroll byte-gleich wie vorher. Auch NACH dem Stichtag
// kann niemand eine Note anmelden, solange keine Freischaltung einen Satz
// eingetragen hat -- und die Freischaltung verlangt N >= 2^256: ein
// Spielzeugsatz wie das Labor (h = 1.491) kommt nie in die Kette.
//
// # WARUM HEUTE NICHT AN
//
// Auf echter Hardware (ibm_marrakesh, 15.11.2026) lebt eine Note ~20 us. Wer
// heute Deckung in eine Note sperrte, verloere sie nach Mikrosekunden fuer
// immer. Darum entscheiden die Pruefer, wann ein Satz gilt -- nicht der Code.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const (
	// quantenGeldAbUnix: Stichtag der beiden Arten (math.MaxInt64 =
	// schlafend). Gesetzt am 15.11.2026 auf Anweisung des Betreibers ("alles
	// in Main, heute laufen"), dann vorgezogen ("in 1 Stunde bereits
	// scharf"): 1794748500 = 15.11.2026 13:15 UTC. Ab dann
	// nimmt das Hauptnetz Freischaltungen und Anmeldungen an; eine Note
	// anmelden kann trotzdem erst, wer einen Satz hat, den zwei Drittel der
	// Pruefer freigeschaltet haben (N >= 2^256, 30 Tage Vorlauf). Der Deploy
	// haelt an, wenn er nicht mindestens 30 Minuten vorher laeuft
	// (.github/neustart/quantengeld-nicht-verpasst.sh).
	quantenGeldAbUnix int64 = 1794748500

	quantenFreischaltungTxArt   = "quanten_freischaltung"
	quantenDomaeneFreischaltung = "aequitas-quanten-freischaltung-v1"
	quantenDomaenePraegen       = "aequitas-quanten-praegen-v1"
	quantenGeldKonfigSchluessel = "quanten_geld"

	// Hoechstens so viele Parametersaetze.
	quantenSaetzeHoechstens = 8
	// Ein Satz gilt fruehestens 30 Tage nach der Abstimmung: jeder Mensch
	// sieht vorher, was kommt.
	quantenFreischaltungVorlauf int64 = 30 * 86400
	// Angemeldete Noten insgesamt und je Anmelder (das Register wird je
	// Aenderung ganz neu geschrieben, wie das Umzugs-Register; darueber
	// gehoert es in eine eigene Tabelle).
	quantenNotenHoechstens   = 50000
	quantenNotenJeAnmelder   = 1000
	quantenMindestDeckungMik = 1_000_000 // 1 AEQ
	quantenKoerperMax        = 256 << 10
	burstQuantenJeIP         = 10
)

// quantenTopfAddr: "0x" + die letzten 20 Byte von SHA-256 der Domaene. Zu
// dieser Adresse gibt es keinen Schluessel (secp256k1-Adressen sind Keccak
// eines oeffentlichen Schluessels; diese ist ein SHA-256 ohne Urbild), und
// sie ist ein Topf (isTokenomicsPoolAddress): keine Demurrage, keine
// Vermoegensgrenze, kein Absenden mit Schluessel.
var quantenTopfAddr = func() string {
	h := sha256.Sum256([]byte("aequitas-quanten-topf-v1"))
	return "0x" + hex.EncodeToString(h[12:])
}()

// quantenGeldAbOverride: nur fuer Tests (0 = Konstante gilt).
var quantenGeldAbOverride atomic.Int64

func quantenGeldAb() int64 {
	if v := quantenGeldAbOverride.Load(); v != 0 {
		return v
	}
	return quantenGeldAbUnix
}

func quantenGeldAktiv(t int64) bool { return t >= quantenGeldAb() }

// QuantenAngaben: Transaction.Quanten. Anmelden (an einer Ueberweisung an den
// Topf): Satz, Serie, Sig. Freischaltung: Satz, Ordnung, Ab, Stimmen.
type QuantenAngaben struct {
	Satz    string           `json:"satz"`
	Serie   string           `json:"serie,omitempty"`
	Sig     string           `json:"sig,omitempty"`
	Ordnung string           `json:"ordnung,omitempty"`
	Ab      int64            `json:"ab,omitempty"`
	Stimmen []VerifierStimme `json:"stimmen,omitempty"`
}

// ------------------------------------------------------------ Register

type quantenSatzEintrag struct {
	Name    string `json:"name"`
	Ordnung string `json:"ordnung"` // dezimal, N >= 2^256
	Ab      int64  `json:"ab"`
	Seit    int64  `json:"seit"` // issued_at der Freischaltung
}

type quantenNoteEintrag struct {
	K     string `json:"k"`
	Satz  string `json:"satz"`
	Von   string `json:"von"`
	Mikro int64  `json:"mikro"`
}

type quantenGeldRegister struct {
	Saetze []quantenSatzEintrag `json:"saetze,omitempty"` // nach Name
	Noten  []quantenNoteEintrag `json:"noten,omitempty"`  // nach K
}

func (r *quantenGeldRegister) leer() bool {
	return r == nil || (len(r.Saetze) == 0 && len(r.Noten) == 0)
}

func (r *quantenGeldRegister) kodiert() string {
	if r.leer() {
		return ""
	}
	b, _ := json.Marshal(r)
	return string(b)
}

func (r *quantenGeldRegister) kopie() *quantenGeldRegister {
	return &quantenGeldRegister{
		Saetze: append([]quantenSatzEintrag(nil), r.Saetze...),
		Noten:  append([]quantenNoteEintrag(nil), r.Noten...),
	}
}

func (r *quantenGeldRegister) satz(name string) (quantenSatzEintrag, bool) {
	for _, s := range r.Saetze {
		if s.Name == name {
			return s, true
		}
	}
	return quantenSatzEintrag{}, false
}

// saetzeZu: die Parametersaetze, die zur Zeit t gelten (Name -> N).
func (r *quantenGeldRegister) saetzeZu(t int64) map[string]*big.Int {
	m := map[string]*big.Int{}
	for _, s := range r.Saetze {
		if t >= s.Ab {
			if n, ok := new(big.Int).SetString(s.Ordnung, 10); ok {
				m[s.Name] = n
			}
		}
	}
	return m
}

func (r *quantenGeldRegister) note(k string) (quantenNoteEintrag, int, bool) {
	i := sort.Search(len(r.Noten), func(i int) bool { return r.Noten[i].K >= k })
	if i < len(r.Noten) && r.Noten[i].K == k {
		return r.Noten[i], i, true
	}
	return quantenNoteEintrag{}, i, false
}

func (r *quantenGeldRegister) notenVon(w string) int {
	n := 0
	for _, e := range r.Noten {
		if e.Von == w {
			n++
		}
	}
	return n
}

func (r *quantenGeldRegister) summeMikro() int64 {
	var s int64
	for _, e := range r.Noten {
		s = plusGesaettigt(s, e.Mikro)
	}
	return s
}

func (r *quantenGeldRegister) mitNote(e quantenNoteEintrag) *quantenGeldRegister {
	k := r.kopie()
	_, i, da := k.note(e.K)
	if da {
		return k
	}
	k.Noten = append(k.Noten, quantenNoteEintrag{})
	copy(k.Noten[i+1:], k.Noten[i:])
	k.Noten[i] = e
	return k
}

func (r *quantenGeldRegister) mitSatz(s quantenSatzEintrag) *quantenGeldRegister {
	k := r.kopie()
	if _, da := k.satz(s.Name); da {
		return k
	}
	k.Saetze = append(k.Saetze, s)
	sort.Slice(k.Saetze, func(i, j int) bool { return k.Saetze[i].Name < k.Saetze[j].Name })
	return k
}

var quantenOrdnungMin = new(big.Int).Lsh(big.NewInt(1), 256)
var quantenOrdnungMax = new(big.Int).Lsh(big.NewInt(1), 8*quantenSerieMaxBytes)

// quantenOrdnungGueltig: dezimal, kanonisch (keine fuehrende Null), 2^256 <=
// N <= 2^(8*1024).
func quantenOrdnungGueltig(s string) (*big.Int, bool) {
	if s == "" || len(s) > 2500 || (len(s) > 1 && s[0] == '0') {
		return nil, false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return nil, false
		}
	}
	n, ok := new(big.Int).SetString(s, 10)
	if !ok || n.Cmp(quantenOrdnungMin) < 0 || n.Cmp(quantenOrdnungMax) > 0 {
		return nil, false
	}
	return n, true
}

func quantenGeldRegisterLesen(s string) (*quantenGeldRegister, error) {
	r := &quantenGeldRegister{}
	if s == "" {
		return r, nil
	}
	if len(s) > 64<<20 {
		return nil, errors.New("Quantengeld-Register zu gross")
	}
	dec := json.NewDecoder(strings.NewReader(s))
	dec.DisallowUnknownFields()
	if err := dec.Decode(r); err != nil {
		return nil, fmt.Errorf("Quantengeld-Register nicht lesbar: %w", err)
	}
	if len(r.Saetze) > quantenSaetzeHoechstens || len(r.Noten) > quantenNotenHoechstens {
		return nil, errors.New("Quantengeld-Register ueber seinen Grenzen")
	}
	for i, x := range r.Saetze {
		if _, ok := quantenOrdnungGueltig(x.Ordnung); !ok || !quantenSatzGueltig(x.Name) || x.Ab <= 0 || x.Seit <= 0 ||
			(i > 0 && x.Name <= r.Saetze[i-1].Name) {
			return nil, fmt.Errorf("Quantengeld-Register: Satz %d ungueltig", i)
		}
	}
	for i, e := range r.Noten {
		if !istHash64(e.K) || !kanonischeAdresse(e.Von) || !quantenSatzGueltig(e.Satz) || e.Mikro < quantenMindestDeckungMik ||
			(i > 0 && e.K <= r.Noten[i-1].K) {
			return nil, fmt.Errorf("Quantengeld-Register: Note %d ungueltig", i)
		}
		if _, ok := r.satz(e.Satz); !ok {
			return nil, fmt.Errorf("Quantengeld-Register: Note %d zu unbekanntem Satz", i)
		}
	}
	return r, nil
}

func (cs *ChainState) quantenGeldJetzt() *quantenGeldRegister {
	if r := cs.quantenGeld.Load(); r != nil {
		return r
	}
	return &quantenGeldRegister{}
}

// quantenGeldLaden: beim Start und nach einem Snapshot-Import. Ein
// unlesbares Register haelt den Knoten an.
func (cs *ChainState) quantenGeldLaden(ctx context.Context) error {
	if cs.db == nil {
		return nil
	}
	r, err := quantenGeldRegisterLesen(cs.getConfigValueCtx(ctx, quantenGeldKonfigSchluessel))
	if err != nil {
		return err
	}
	cs.quantenGeld.Store(r)
	return nil
}

// quantenGeldSetzenLocked: in derselben Datenbank-Transaktion wie der Rest
// des Blocks; der Zeiger geht mit blockRollbackSnapshot zurueck.
func (cs *ChainState) quantenGeldSetzenLocked(ctx context.Context, r *quantenGeldRegister) error {
	if cs.db != nil {
		if err := cs.setConfigValueCtx(ctx, quantenGeldKonfigSchluessel, r.kodiert()); err != nil {
			return fmt.Errorf("Quantengeld-Register speichern: %w", err)
		}
	}
	cs.quantenGeld.Store(r)
	return nil
}

// quantenStateRootTeil: Teil der StateRoot -- nur, wenn das Register nicht
// leer ist; sonst byte-gleich wie vorher.
func (r *quantenGeldRegister) stateRootTeil() string {
	if r.leer() {
		return ""
	}
	h := sha256.Sum256([]byte(r.kodiert()))
	return "|quanten:" + hex.EncodeToString(h[:])
}

// ------------------------------------------------------------ Form

// quantenFeldErlaubt: zustandslos, fuer jede Transaktion eines Blocks. Vor dem
// Stichtag macht jede Quanten-Angabe den Block ungueltig; danach gehoert sie
// nur an eine Freischaltung oder an eine Ueberweisung an den Topf -- und
// jede Ueberweisung an den Topf braucht sie (sonst waere Deckung ohne Note
// verloren).
func quantenFeldErlaubt(tx *Transaction, blockZeit int64) error {
	if tx.Quanten == nil {
		if tx.Type == "transfer" && quantenGeldAktiv(blockZeit) && strings.EqualFold(strings.TrimSpace(tx.To), quantenTopfAddr) {
			return fmt.Errorf("Ueberweisung an den Quantengeld-Topf ohne Note")
		}
		return nil
	}
	if !quantenGeldAktiv(blockZeit) {
		return fmt.Errorf("%s traegt Quanten-Angaben vor dem Stichtag", tx.Type)
	}
	switch tx.Type {
	case quantenFreischaltungTxArt:
		return nil
	case "transfer":
		if !strings.EqualFold(strings.TrimSpace(tx.To), quantenTopfAddr) {
			return fmt.Errorf("Quanten-Angaben an einer Ueberweisung, die nicht an den Topf geht")
		}
		return nil
	}
	return fmt.Errorf("%s traegt Quanten-Angaben", tx.Type)
}

// pruefeTopfNurMitNote: Annahme-Sperre auf jedem Ueberweisungsweg, ab sofort
// (nicht erst ab dem Stichtag): eine gewoehnliche Ueberweisung an den Topf
// waere Deckung ohne Note und ab dem Stichtag in keinem Block gueltig -- der
// Erzeuger fände fuer sie keine Blockzeit, es entstuende kein Block mehr.
// Anmeldungen laufen ueber quantenPraegenAnnehmen, nie hier.
func pruefeTopfNurMitNote(to string, vorlage Transaction) error {
	if strings.EqualFold(strings.TrimSpace(to), quantenTopfAddr) {
		return fmt.Errorf("transfers to the quantum money pool %s are only accepted as note registrations (/api/quanten/praegen)", quantenTopfAddr)
	}
	return nil
}

// quantenPraegenNachricht: was die Wallet zur Anmeldung unterschreibt
// (EIP-191). Der TxHash bindet die Anmeldung an genau diese Ueberweisung.
func quantenPraegenNachricht(wallet, satz, serieHex, txHash string) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%s|%s", quantenDomaenePraegen, eingefroreneChainID,
		strings.ToLower(wallet), satz, serieHex, strings.ToLower(strings.TrimSpace(txHash)))
}

// quantenFreischaltungNachricht: was jeder abstimmende Pruefer bescheinigt.
func quantenFreischaltungNachricht(satz, ordnung string, ab int64) func(wallet string, stimmzeit int64) string {
	return func(_ string, stimmzeit int64) string {
		return fmt.Sprintf("%s|chain:%d|%s|%s|%d|%d", quantenDomaeneFreischaltung, eingefroreneChainID, satz, ordnung, ab, stimmzeit)
	}
}

// quantenSerieAusHex: kanonisches Hex (klein, gerade Laenge) -> Bytes.
func quantenSerieAusHex(s string) ([]byte, bool) {
	if s == "" || len(s)%2 != 0 || len(s) > 2*quantenSerieMaxBytes || !kleinHex(s) {
		return nil, false
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, false
	}
	return b, true
}

// ------------------------------------------------------------ Urteile

type quantenUrteil struct {
	regel, text string
	note        quantenNoteEintrag
	satz        quantenSatzEintrag
	// stimme: 1-basiert die Stimme, an der die Freischaltung scheiterte
	// (0 = keine einzelne) -- der Sammelweg verwirft genau sie
	// (quanten_sammeln.go).
	stimme int
}

func quantenBefund(regel, format string, args ...interface{}) quantenUrteil {
	return quantenUrteil{regel: regel, text: fmt.Sprintf(format, args...)}
}

func (u quantenUrteil) mitStimme(i int) quantenUrteil {
	u.stimme = i
	return u
}

// quantenPraegenUrteilLocked: darf diese Ueberweisung an den Topf eine Note
// anmelden? Dieselbe Pruefung fuer Annahme (Uhrzeit) und Nachspielen
// (Blockzeit). Die Ueberweisung selbst (Rohform, Nonce, Guthaben) prueft der
// Ueberweisungsweg.
func (cs *ChainState) quantenPraegenUrteilLocked(tx *Transaction, blockZeit int64) quantenUrteil {
	q := tx.Quanten
	w := strings.ToLower(strings.TrimSpace(tx.Wallet))
	if !quantenGeldAktiv(blockZeit) || q == nil || tx.Type != "transfer" || !strings.EqualFold(strings.TrimSpace(tx.To), quantenTopfAddr) {
		return quantenBefund("quanten_form", "keine Anmeldung")
	}
	if q.Ordnung != "" || q.Ab != 0 || len(q.Stimmen) != 0 || !kanonischeAdresse(w) || !quantenSatzGueltig(q.Satz) {
		return quantenBefund("quanten_form", "Anmeldung nicht in gueltiger Form")
	}
	serie, ok := quantenSerieAusHex(q.Serie)
	if !ok {
		return quantenBefund("quanten_form", "Seriennummer nicht in gueltiger Form")
	}
	if math.IsNaN(tx.Amount) || math.IsInf(tx.Amount, 0) || tx.Amount <= 0 {
		return quantenBefund("quanten_deckung", "Deckung ungueltig")
	}
	mikro := NewDecimal(tx.Amount).Micro()
	if mikro < quantenMindestDeckungMik {
		return quantenBefund("quanten_deckung", "Deckung %.6f AEQ, mindestens %.6f", tx.Amount, float64(quantenMindestDeckungMik)/1e6)
	}
	reg := cs.quantenGeldJetzt()
	saetze := reg.saetzeZu(blockZeit)
	if !quantenSerieForm(saetze, q.Satz, serie) {
		return quantenBefund("quanten_satz", "Satz %q gilt nicht, oder Seriennummer nicht kanonisch / nicht kleiner als N", q.Satz)
	}
	if !kanonischeSignatur(q.Sig) {
		return quantenBefund("quanten_unterschrift", "Unterschrift fehlt")
	}
	if err := pruefePersonalSignGemerkt(quantenPraegenNachricht(w, q.Satz, q.Serie, tx.TxHash), q.Sig, w); err != nil {
		return quantenBefund("quanten_unterschrift", "Anmeldung nicht von %s unterschrieben", kurzAdresse(w))
	}
	k := quantenSerieSchluessel(q.Satz, w, serie)
	if _, _, da := reg.note(k); da {
		return quantenBefund("quanten_doppelt", "%s: Seriennummer schon angemeldet", kurzAdresse(w))
	}
	if len(reg.Noten) >= quantenNotenHoechstens {
		return quantenBefund("quanten_voll", "Register voll")
	}
	if reg.notenVon(w) >= quantenNotenJeAnmelder {
		return quantenBefund("quanten_voll", "%s: hoechstens %d Noten je Anmelder", kurzAdresse(w), quantenNotenJeAnmelder)
	}
	return quantenUrteil{note: quantenNoteEintrag{K: k, Satz: q.Satz, Von: w, Mikro: mikro}}
}

// quantenFreischaltungUrteilLocked: zwei Drittel der Pruefer zur Zeit
// issued_at (tx.DistributionAt) stimmen fuer genau diesen Satz. Fehler nur,
// wenn die Zulassung nicht lesbar war (dann geht der Block zurueck).
func (cs *ChainState) quantenFreischaltungUrteilLocked(ctx context.Context, tx *Transaction, blockZeit int64) (quantenUrteil, error) {
	q := tx.Quanten
	if !quantenGeldAktiv(blockZeit) || q == nil || tx.Type != quantenFreischaltungTxArt {
		return quantenBefund("quanten_form", "keine Freischaltung"), nil
	}
	if q.Serie != "" || q.Sig != "" || !quantenSatzGueltig(q.Satz) || tx.To != "" || tx.Amount != 0 {
		return quantenBefund("quanten_form", "Freischaltung nicht in gueltiger Form"), nil
	}
	if _, ok := quantenOrdnungGueltig(q.Ordnung); !ok {
		return quantenBefund("quanten_ordnung", "Gruppenordnung ungueltig oder kleiner als 2^256"), nil
	}
	if strings.HasPrefix(q.Satz, "labor") || strings.HasPrefix(q.Satz, "test") {
		return quantenBefund("quanten_form", "Spielzeug-/Testsatz %q", q.Satz), nil
	}
	if n := len(q.Stimmen); n < 1 || n > verifierStimmenHoechstens {
		return quantenBefund("quanten_form", "%d Stimmen", n), nil
	}
	zeit := tx.DistributionAt
	if zeit <= 0 || zeit < blockZeit-nachweisHoechstensAlt || zeit > sattAdd(blockZeit, nachweisHoechstensVoraus) || !quantenGeldAktiv(zeit) {
		return quantenBefund("quanten_zeit", "beschlossen %d, Blockzeit %d", zeit, blockZeit), nil
	}
	if q.Ab < sattAdd(zeit, quantenFreischaltungVorlauf) {
		return quantenBefund("quanten_vorlauf", "gilt ab %d, fruehestens %d", q.Ab, sattAdd(zeit, quantenFreischaltungVorlauf)), nil
	}
	reg := cs.quantenGeldJetzt()
	if _, da := reg.satz(q.Satz); da {
		return quantenBefund("quanten_satz_da", "Satz %q ist schon eingetragen", q.Satz), nil
	}
	if len(reg.Saetze) >= quantenSaetzeHoechstens {
		return quantenBefund("quanten_voll", "hoechstens %d Saetze", quantenSaetzeHoechstens), nil
	}
	pr := cs.verifierRegisterJetzt()
	stimmberechtigt := map[string]bool{}
	for _, m := range pr.mitgliederZu(zeit) {
		stimmberechtigt[m] = true
	}
	noetig := verifierSchwelle(len(stimmberechtigt))
	// Je Mensch hoechstens eine Stimme -- vor jeder teuren Pruefung.
	behauptet := map[string]bool{}
	for i := range q.Stimmen {
		m := strings.ToLower(strings.TrimSpace(q.Stimmen[i].Bescheinigung.Mensch))
		if behauptet[m] {
			return quantenBefund("quanten_stimme", "Stimme %d: %s stimmt zweimal", i+1, kurzAdresse(m)).mitStimme(i + 1), nil
		}
		if !stimmberechtigt[m] {
			return quantenBefund("quanten_stimme", "Stimme %d: %s ist zu %d kein Pruefer", i+1, kurzAdresse(m), zeit).mitStimme(i + 1), nil
		}
		behauptet[m] = true
	}
	stand := func(m string) (bool, bool) {
		cs.ensureAccountLoadedCtx(ctx, m)
		acc, ok := cs.accounts.Get(m)
		return ok && acc.IsHuman, ok && acc.GrantStagedRest > 0
	}
	zulassung := cs.lebensnachweisZulassung(ctx)
	nachricht := quantenFreischaltungNachricht(q.Satz, q.Ordnung, q.Ab)
	stimmen := map[string]bool{}
	for i := range q.Stimmen {
		s := &q.Stimmen[i]
		if s.Zeit <= 0 || s.Zeit > sattAdd(zeit, nachweisHoechstensVoraus) || zeit-s.Zeit > verifierStimmeHoechstensAlt {
			return quantenBefund("quanten_stimme_zeit", "Stimme %d von %d, beschlossen %d", i+1, s.Zeit, zeit).mitStimme(i + 1), nil
		}
		// "fuer" ist eine Adresse, die kein Mensch ist: niemand bescheinigt
		// hier sich selbst etwas (wie beim Austritt, verifier_register.go).
		if err := bescheinigungPruefenFuer(nachricht, ubiPoolAddr, s.Zeit, &s.Bescheinigung, stand, zulassung); err != nil {
			if errors.Is(err, errZulassungNichtLesbar) {
				return quantenUrteil{}, err
			}
			return quantenBefund("quanten_stimme", "Stimme %d: %v", i+1, err).mitStimme(i + 1), nil
		}
		// Audit B2: PQ-Teil des Pruefers (pruefer_pq.go).
		if err := prueferPQPruefen(s, nachricht(ubiPoolAddr, s.Zeit), prueferPQPflicht(zeit), cs.prueferZusageVon); err != nil {
			return quantenBefund("quanten_stimme_pq", "Stimme %d: %v", i+1, err).mitStimme(i + 1), nil
		}
		stimmen[s.Bescheinigung.Mensch] = true
	}
	if len(stimmen) < noetig {
		return quantenBefund("quanten_quorum", "%d verschiedene Pruefer, noetig %d von %d", len(stimmen), noetig, len(stimmberechtigt)), nil
	}
	return quantenUrteil{satz: quantenSatzEintrag{Name: q.Satz, Ordnung: q.Ordnung, Ab: q.Ab, Seit: zeit}}, nil
}

// ------------------------------------------------------------ Nachspielen

// quantenAnmeldungVorUeberweisungLocked: beim Nachspielen VOR der Ueberweisung
// an den Topf. Eine ungueltige Anmeldung wird nie angewandt -- der Erzeuger
// nimmt sie nie an, also ist der Block falsch.
func (cs *ChainState) quantenAnmeldungVorUeberweisungLocked(tx *Transaction, blockZeit int64) (quantenNoteEintrag, error) {
	u := cs.quantenPraegenUrteilLocked(tx, blockZeit)
	if u.regel != "" {
		return quantenNoteEintrag{}, fmt.Errorf("quanten_praegen %s: %s", u.regel, u.text)
	}
	return u.note, nil
}

// quantenNoteEintragenLocked: NACH der gelungenen Ueberweisung.
func (cs *ChainState) quantenNoteEintragenLocked(ctx context.Context, e quantenNoteEintrag) error {
	return cs.quantenGeldSetzenLocked(ctx, cs.quantenGeldJetzt().mitNote(e))
}

// applyQuantenFreischaltungLocked: beim Nachspielen. Ungueltig: nie
// angewandt, Fehler (der Block geht zurueck).
func (cs *ChainState) applyQuantenFreischaltungLocked(ctx context.Context, tx *Transaction, blockZeit int64) error {
	u, err := cs.quantenFreischaltungUrteilLocked(ctx, tx, blockZeit)
	if err != nil {
		return err
	}
	if u.regel != "" {
		return fmt.Errorf("quanten_freischaltung %s: %s", u.regel, u.text)
	}
	return cs.quantenGeldSetzenLocked(ctx, cs.quantenGeldJetzt().mitSatz(u.satz))
}

// ------------------------------------------------------------ Annahme

// quantenPraegenAnnehmen: die Ueberweisung an den Topf und der Eintrag in
// EINER Datenbank-Transaktion (nie ueber Stapel- oder WAL-Pfad: die kennen
// das Register nicht). Wie transferAtomicDirect, mit dem Urteil davor.
func (cs *ChainState) quantenPraegenAnnehmen(tx Transaction) error {
	from := strings.ToLower(strings.TrimSpace(tx.Wallet))
	to := quantenTopfAddr
	if err := cs.annahmeBeginnen(from, to); err != nil {
		return err
	}
	defer cs.annahmeEnde()
	if cs.wal != nil && !cs.walVorSeriellLeeren(from) {
		return fmt.Errorf("server busy: earlier transfers of this sender are still being written, try again shortly")
	}
	return cs.runAtomicWithOutbox([]string{from, to, validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr}, false, func(ctx context.Context) (Transaction, error) {
		at := nowUnix()
		u := cs.quantenPraegenUrteilLocked(&tx, at)
		if u.regel != "" {
			return Transaction{}, fmt.Errorf("%s: %s", u.regel, u.text)
		}
		// Der Satz muss schon eine Stunde gelten: der Block darf eine
		// fruehere Zeit bekommen als die Annahme (blockZeitFuer), und beim
		// Nachspielen muss die Anmeldung zu dieser Zeit genauso bestehen.
		if _, ok := cs.quantenGeldJetzt().saetzeZu(at - nachweisHoechstensAlt)[tx.Quanten.Satz]; !ok {
			return Transaction{}, fmt.Errorf("quanten_satz: Satz %q gilt erst seit weniger als %d s", tx.Quanten.Satz, nachweisHoechstensAlt)
		}
		cs.ensureAccountLoadedCtx(ctx, from)
		if acc, ok := cs.accounts.Get(from); ok {
			if err := pruefeNonceLocked(acc, tx); err != nil {
				return Transaction{}, err
			}
		}
		fromLost, toLost, gebuehr, err := cs.transferLockedMitGebuehr(mitBuchZeit(ctx, at), from, to, tx.Amount)
		if err != nil {
			return Transaction{}, err
		}
		if err := cs.merkeAngenommeneNonceLocked(ctx, from, tx); err != nil {
			return Transaction{}, err
		}
		if err := cs.quantenNoteEintragenLocked(ctx, u.note); err != nil {
			return Transaction{}, err
		}
		tx.Gebuehr = gebuehr
		tx.BuchAt = buchStempel(at)
		tx.FromDemurrageLost = fromLost
		tx.ToDemurrageLost = toLost
		return tx, nil
	})
}

// handleQuantenPraegen: POST {"roh", "satz", "serie", "sig"} -- roh ist die
// signierte EVM-Ueberweisung der Deckung an den Topf, sig die EIP-191-
// Unterschrift ueber quantenPraegenNachricht(wallet, satz, serie, txHash).
func (a *APIServer) handleQuantenPraegen(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	jetzt := time.Now().Unix()
	if !quantenGeldAktiv(jetzt) {
		jsonError(w, "quantum money not active", http.StatusConflict)
		return
	}
	if !burstErlaubt("quanten-praegen:"+erneuerungsAbsender(r), burstQuantenJeIP, burstFenster) {
		jsonError(w, "too many requests from this address", http.StatusTooManyRequests)
		return
	}
	var req struct {
		Roh   string `json:"roh"`
		Satz  string `json:"satz"`
		Serie string `json:"serie"`
		Sig   string `json:"sig"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, quantenKoerperMax))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	t, absender, _, err := decodeAndRecoverSender(strings.TrimSpace(req.Roh))
	if err != nil {
		jsonError(w, "roh: "+err.Error(), http.StatusBadRequest)
		return
	}
	an, betrag, ok := ueberweisungAusRoh(t)
	if !ok || an != quantenTopfAddr {
		jsonError(w, "roh must be a transfer to "+quantenTopfAddr, http.StatusBadRequest)
		return
	}
	tx := Transaction{Type: "transfer", Wallet: absender, To: quantenTopfAddr, Amount: betrag,
		TxHash: strings.ToLower(t.Hash().Hex()), Roh: strings.TrimSpace(req.Roh),
		Quanten: &QuantenAngaben{Satz: req.Satz, Serie: req.Serie, Sig: kanonischeSignaturVersuch(req.Sig)}}
	if _, err := pruefeUeberweisungsRoh(&tx, jetzt); err != nil {
		jsonError(w, err.Error(), http.StatusForbidden)
		return
	}
	if err := a.state.quantenPraegenAnnehmen(tx); err != nil {
		if istWiederholbareAnnahmeAblehnung(err) {
			jsonError(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		jsonError(w, "registration failed: "+err.Error(), http.StatusConflict)
		return
	}
	a.state.SyncBalancesToEVM(V7_CONTRACT_ADDR, absender, quantenTopfAddr)
	recordAppliedTransfer()
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "tx_hash": tx.TxHash,
		"k": quantenSerieSchluessel(req.Satz, absender, mustHex(req.Serie))})
}

func mustHex(s string) []byte {
	b, _ := hex.DecodeString(s)
	return b
}

// QuantenStand: was /api/quanten/stand zeigt -- Stichtag, Topf, Saetze und
// die Zahl der angemeldeten Noten. Mit ?k= der Wert einer Note (0 = nicht
// angemeldet): das prueft jeder Empfaenger vor dem Quantentest.
func (cs *ChainState) QuantenStand(k string) map[string]interface{} {
	reg := cs.quantenGeldJetzt()
	saetze := make([]map[string]interface{}, 0, len(reg.Saetze))
	for _, s := range reg.Saetze {
		saetze = append(saetze, map[string]interface{}{"name": s.Name, "ordnung_bits": len(s.Ordnung) * 3322 / 1000, "ab": s.Ab, "beschlossen": s.Seit})
	}
	ab := quantenGeldAb()
	aus := map[string]interface{}{
		"aktiv":           quantenGeldAktiv(time.Now().Unix()),
		"topf":            quantenTopfAddr,
		"saetze":          saetze,
		"noten":           len(reg.Noten),
		"gedeckt_aeq":     float64(reg.summeMikro()) / 1e6,
		"mindest_deckung": float64(quantenMindestDeckungMik) / 1e6,
	}
	if ab != math.MaxInt64 {
		aus["ab"] = ab
	}
	if k = strings.ToLower(strings.TrimSpace(k)); istHash64(k) {
		e, _, da := reg.note(k)
		aus["note"] = map[string]interface{}{"angemeldet": da, "deckung_aeq": float64(e.Mikro) / 1e6, "satz": e.Satz, "von": e.Von}
	}
	return aus
}

func (a *APIServer) handleQuantenStand(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodGet {
		jsonError(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	json.NewEncoder(w).Encode(a.state.QuantenStand(r.URL.Query().Get("k")))
}

// handleQuantenFreischaltung: POST {"antragsteller", "satz", "ordnung", "ab",
// "beschlossen", "stimmen"} -- reicht eine Freischaltung mit den Stimmen der
// Pruefer ein. Jeder Knoten prueft sie beim Nachspielen selbst.
func (a *APIServer) handleQuantenFreischaltung(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	jetzt := time.Now().Unix()
	if !quantenGeldAktiv(jetzt) {
		jsonError(w, "quantum money not active", http.StatusConflict)
		return
	}
	if !burstErlaubt("quanten-freischaltung:"+erneuerungsAbsender(r), burstQuantenJeIP, burstFenster) {
		jsonError(w, "too many requests from this address", http.StatusTooManyRequests)
		return
	}
	var req struct {
		Antragsteller string           `json:"antragsteller"`
		Satz          string           `json:"satz"`
		Ordnung       string           `json:"ordnung"`
		Ab            int64            `json:"ab"`
		Beschlossen   int64            `json:"beschlossen"`
		Stimmen       []VerifierStimme `json:"stimmen"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, quantenKoerperMax))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	von := strings.ToLower(strings.TrimSpace(req.Antragsteller))
	if !kanonischeAdresse(von) {
		jsonError(w, "antragsteller (address) required", http.StatusBadRequest)
		return
	}
	tx := Transaction{Type: quantenFreischaltungTxArt, Wallet: von, DistributionAt: req.Beschlossen,
		Quanten: &QuantenAngaben{Satz: req.Satz, Ordnung: req.Ordnung, Ab: req.Ab, Stimmen: req.Stimmen}}
	err := a.quantenFreischaltungAnnehmen(tx, jetzt)
	var abgelehnt *quantenAbgelehnt
	switch {
	case errors.As(err, &abgelehnt):
		jsonError(w, abgelehnt.text, http.StatusConflict)
		return
	case err != nil:
		jsonError(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "satz": req.Satz, "ab": req.Ab})
}

// quantenFreischaltungAnnehmen: eine Freischaltung durch das Annahme-Tor --
// dieselbe Pruefung wie beim Nachspielen, dann ins Register und in den
// Ausgang. Fuer den Endpunkt mit allen Stimmen und den Sammelweg.
func (a *APIServer) quantenFreischaltungAnnehmen(tx Transaction, jetzt int64) error {
	if err := a.state.annahmeBeginnen(tx.Wallet); err != nil {
		return err
	}
	defer a.state.annahmeEnde()
	return a.state.runAtomicWithOutbox([]string{tx.Wallet}, false, func(ctx context.Context) (Transaction, error) {
		u, err := a.state.quantenFreischaltungUrteilLocked(ctx, &tx, jetzt)
		if err != nil {
			return Transaction{}, err
		}
		if u.regel != "" {
			return Transaction{}, &quantenAbgelehnt{text: u.regel + ": " + u.text, stimme: u.stimme}
		}
		if err := a.state.quantenGeldSetzenLocked(ctx, a.state.quantenGeldJetzt().mitSatz(u.satz)); err != nil {
			return Transaction{}, err
		}
		return tx, nil
	})
}

type quantenAbgelehnt struct {
	text   string
	stimme int // 1-basiert, 0 = keine einzelne Stimme
}

func (e *quantenAbgelehnt) Error() string { return e.text }

// quantenStatus: der Teil von /api/status (Waechter quantengeld-nicht-
// verpasst.sh). "ab" fehlt, solange schlafend.
func quantenStatus() map[string]interface{} {
	m := map[string]interface{}{"topf": quantenTopfAddr}
	if ab := quantenGeldAb(); ab != math.MaxInt64 {
		m["ab"] = ab
	}
	return m
}
