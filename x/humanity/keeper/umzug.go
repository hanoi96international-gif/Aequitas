package keeper

// Umzug: das Menschsein auf eine neue Wallet holen, wenn die alte verloren,
// gestohlen oder verkauft ist (docs/UMZUG_ENTWURF.md; Entscheidungen des
// Betreibers vom 07.11.2026).
//
// WAS UMZIEHT. Das Menschsein (is_human), der Nullifier, der Stand des
// Lebensnachweises und ein offener Staffelrest gehen von der alten Wallet W1
// auf die neue W2. NICHT um zieht das Guthaben von W1: Geld bewegt nur eine
// Unterschrift. Und es entsteht kein neues Geld -- W2 gilt als "Faucet
// abgeholt", ein zweites Startgeld gibt es nicht.
//
// ZWEI WEGE.
//
//   - Gesicht + Wiederherstellungscode: der Umzug geschieht sofort. Den Code
//     erzeugt die App einmal und zeigt ihn einmal; auf der Kette steht nur
//     sein Hash (umzug_code, von W1 unterschrieben; hoechstens drei je
//     Wallet, ein neuer ersetzt keinen alten). Die alte Wallet kann
//     diesen Umzug nicht aufhalten -- wer Gesicht UND Code hat, ist der
//     Mensch, auch wenn ein Dieb den alten Schluessel hat.
//   - Nur das Gesicht: 14 Tage Frist (umzugFrist). In der Zeit kann die alte
//     Wallet mit ihrer Unterschrift widerrufen (umzug_widerruf). Danach
//     vollzieht die Tagesrunde den Umzug (umzug_vollzug).
//
// DAS GESICHT. Eine v3-Bescheinigung wie bei der Registrierung, mit eigenen
// Domaenen (umzugDomaeneBindung/-Aussage): der Coordinator bindet den
// Nullifier N an die NEUE Wallet, die Vergleichsdienste bezeugen, dass das
// Gesicht zu N gehoert. Dieselben Schluessel und dieselben Regeln wie bei
// der Registrierung (pruefeV3Unterschriften) -- eine Unterschrift fuer eine
// Registrierung passt nie auf einen Umzug und umgekehrt. W2 unterschreibt
// den Antrag selbst (umzugAntragNachricht).
//
// JEDER KNOTEN PRUEFT SELBST: Unterschriften, Nullifier gehoert im eigenen
// Zustand zu W1, W1 ist Mensch, W2 weder Mensch noch Unternehmen noch
// Systemadresse und ohne Nullifier, Code-Hash, Fristen nach Block- bzw.
// Rundenzeit. Das Register (umzugRegister) steht in chain_config und mit
// seinem Hash in der StateRoot -- nur, wenn es nicht leer ist, sonst bleibt
// die Wurzel byte-gleich.
//
// KEIN ENTZUG (kein_entzug_test.go): der Vollzug setzt is_human von W1
// zurueck. Das ist kein Entzug -- das Menschsein bleibt beim Menschen und
// wechselt die Adresse, ausgeloest nur durch das Gesicht desselben
// Menschen; ohne Code mit Frist und Widerruf durch den alten Schluessel.
// Entscheidung des Betreibers vom 07.11.2026.
//
// SCHLAFEND bis umzugAbUnix: davor sind die Arten unbekannt (bekannteTxArt),
// die Endpunkte antworten 409.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	// umzugAbUnix: 2026-11-08 18:00 UTC (vorgezogen vom 12.11. auf Wunsch des
	// Betreibers, zusammen mit dem Stichtag von B1b).
	umzugAbUnix int64 = 1794160800

	// umzugCodeRegelnAbUnix: 2026-11-09 18:00 UTC. Ab dann (Entscheidung des
	// Betreibers vom 08.11.2026): EIN Code je Wallet, einmal hinterlegt nicht
	// aenderbar; er gilt sofort. Einen neuen Code gibt es nur nach einem Umzug
	// auf eine neue Wallet. Grund: wer die Seed-Phrase stiehlt, konnte bisher
	// einen eigenen Code DAZU legen und mit einem Deepfake des Gesichts sofort
	// umziehen, ohne Widerruf. Hat der Mensch seinen Code (die App bietet ihn
	// gleich nach der Registrierung an), legt niemand mehr einen dazu.
	// Gemessen an der unterschriebenen Zeit des Auftrags (tx.DistributionAt),
	// nicht an der Blockzeit: so entscheiden Annahme (Uhrzeit) und Nachspielen
	// (Blockzeit) gleich. Ein Auftrag von vor dem Stichtag kommt nur in einen
	// Block vor dem Stichtag (auftragsFenster, blockTauglich).
	// (Eine Reifezeit und ein Verfallen unreifer Codes waren im Entwurf; die
	// Sicherheitspruefung von #472 zeigte, dass ein Angreifer mit Deepfake
	// allein den Menschen damit aussperren konnte -- verworfen.)
	umzugCodeRegelnAbUnix int64 = 1794247200
	// umzugCodeRegelnPause: so lange vor dem Stichtag nimmt der Knoten keinen
	// Code-Auftrag mit einer Zeit davor mehr an (wie chainIDWechselPause).
	// Sonst stuende ein angenommener Auftrag, der nur in einen Block VOR dem
	// Stichtag darf, noch im Ausgang, waehrend schon ein Block danach
	// entstanden ist -- das Fenster waere leer und die Erzeugung hinge
	// (Sicherheitspruefung von #472, 2. Durchgang, M1).
	umzugCodeRegelnPause int64 = 600

	// umzugNurLetzterCodeAbUnix: 2026-11-12 18:00 UTC. Ab dann zaehlt bei
	// einem Umzug mit Code nur der ZULETZT hinterlegte Code einer Wallet.
	// Vor dem 09.11.2026 durfte eine Wallet bis zu drei Codes hinterlegen;
	// Entscheidung des Betreibers vom 11.11.2026: nur der zuletzt angelegte
	// gilt. Gemessen wie umzugCodeRegelnAbUnix an der unterschriebenen Zeit
	// des Antrags; ein Antrag mit Code von vor dem Stichtag nur in einen Block
	// davor (umzug_zeit, blockTauglich, auftragsFenster), und 600 s davor
	// nimmt der Knoten keinen mehr an.
	umzugNurLetzterCodeAbUnix int64 = 1794506400

	umzugCodeTxArt        = "umzug_code"
	umzugAnkuendigenTxArt = "umzug_ankuendigen"
	umzugWiderrufTxArt    = "umzug_widerruf"
	umzugVollzugTxArt     = "umzug_vollzug"

	umzugDomaeneBindung  = "aequitas-umzug-bindung-v1"
	umzugDomaeneAussage  = "aequitas-umzug-aussage-v1"
	umzugDomaeneAntrag   = "aequitas-umzug-v1"
	umzugDomaeneWiderruf = "aequitas-umzug-widerruf-v1"
	umzugDomaeneCode     = "aequitas-umzug-code-v1"
	umzugDomaeneCodeSetz = "aequitas-umzug-code-setzen-v1"

	// Frist ohne Code.
	umzugFrist int64 = 14 * 86400
	// Frist ohne Code, wenn fuer diesen Menschen im letzten Jahr schon
	// widerrufen wurde: laenger, aber nicht mehr widerrufbar (sonst sperrte
	// ein Dieb mit dem alten Schluessel den Menschen fuer immer aus).
	umzugFristFest int64 = 30 * 86400
	// Widerrufe je Nullifier in 365 Tagen, danach ist ein Umzug fest.
	umzugWiderrufeJeJahr     = 1
	umzugWiderrufeHoechstens = 20000
	// Pruefstufe des Coordinators: ohne Code nur "streng" (Lebendigkeit gruen
	// UND an die Aufnahme gebundene Hardware-Attestierung des Geraets).
	umzugPruefungStreng  = "streng"
	umzugPruefungEinfach = "einfach"
	// Hoechstens zwei vollzogene Umzuege je Nullifier in 365 Tagen.
	umzugJeJahr       = 2
	umzugJahr   int64 = 365 * 86400
	// Erledigte Umzuege bleiben so lange im Register (Jahresgrenze).
	umzugErledigtHalten int64 = 366 * 86400

	// Grenzen des Registers. Bei umzugCodesHoechstens muss das Register
	// spaetestens in eine eigene Tabelle (docs/UMZUG_ENTWURF.md).
	umzugCodesHoechstens    = 50000
	umzugOffenHoechstens    = 1000
	umzugErledigtHoechstens = 20000
	// Codes je Wallet (sie sammeln sich, siehe umzugCode).
	umzugCodesJeWallet = 3
	// Vollzuege je Tagesrunde; der Rest am naechsten Tag.
	umzugVollzugJeRunde = 200

	// Bescheinigung/Antrag: hoechstens 5 min in der Zukunft, hoechstens einen
	// Tag alt.
	umzugHoechstensVoraus int64 = 300
	umzugHoechstensAlt    int64 = 86400

	// Der Code: 24 Zeichen aus Crockfords Base32 (120 Bit).
	umzugCodeLaenge  = 24
	umzugCodeZeichen = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

	umzugKoerperMax = 32 << 10

	umzugKonfigSchluessel = "umzug_register"
)

// umzugAbOverride: nur fuer Tests (0 = Konstante gilt).
var umzugAbOverride atomic.Int64

func umzugAb() int64 {
	if o := umzugAbOverride.Load(); o != 0 {
		return o
	}
	return umzugAbUnix
}

func umzugAktiv(t int64) bool { return t >= umzugAb() }

// umzugCodeRegelnAbOverride: nur fuer Tests (0 = Konstante gilt).
var umzugCodeRegelnAbOverride atomic.Int64

func umzugCodeRegelnAb() int64 {
	if o := umzugCodeRegelnAbOverride.Load(); o != 0 {
		return o
	}
	return umzugCodeRegelnAbUnix
}

// umzugNurLetzterCodeAbOverride: nur fuer Tests (0 = Konstante gilt).
var umzugNurLetzterCodeAbOverride atomic.Int64

func umzugNurLetzterCodeAb() int64 {
	if o := umzugNurLetzterCodeAbOverride.Load(); o != 0 {
		return o
	}
	return umzugNurLetzterCodeAbUnix
}

// umzugNurLetzterCode: zaehlt fuer einen Antrag mit der unterschriebenen
// Zeit z nur der zuletzt hinterlegte Code?
func umzugNurLetzterCode(z int64) bool { return z >= umzugNurLetzterCodeAb() }

// umzugAntragMitCodePausiert: nimmt der Knoten zur Uhrzeit jetzt einen
// Antrag MIT Code und der Zeit z NICHT an (wie umzugCodeAnnahmePausiert)?
func umzugAntragMitCodePausiert(z, jetzt int64) bool {
	return !umzugNurLetzterCode(z) && jetzt >= sattAdd(umzugNurLetzterCodeAb(), -umzugCodeRegelnPause)
}

// umzugCodeAnnahmePausiert: nimmt der Knoten zur Uhrzeit jetzt einen
// Code-Auftrag mit der unterschriebenen Zeit z NICHT an? Nur fuer die
// Annahme -- das Nachspielen entscheidet allein am Urteil.
func umzugCodeAnnahmePausiert(z, jetzt int64) bool {
	return !umzugCodeRegeln(z) && jetzt >= sattAdd(umzugCodeRegelnAb(), -umzugCodeRegelnPause)
}

// umzugCodeRegeln: gilt fuer einen Auftrag mit der unterschriebenen Zeit z
// "ein Code je Wallet, nicht aenderbar"?
func umzugCodeRegeln(z int64) bool { return z >= umzugCodeRegelnAb() }

// UmzugAngaben: was die Umzugs-Transaktionen zusaetzlich tragen.
type UmzugAngaben struct {
	// umzug_ankuendigen: der Wiederherstellungscode (normalisiert), wenn
	// vorhanden. Er wird damit oeffentlich -- und ist danach verbraucht.
	Code string `json:"code,omitempty"`
	// umzug_code: der Hash des Codes (umzugCodeHash).
	CodeHash string `json:"code_hash,omitempty"`
	// Unterschrift (EIP-191): von W2 beim Antrag, von W1 bei Code und Widerruf.
	Sig string `json:"sig,omitempty"`
	// umzug_widerruf: Zeit der Ankuendigung, die widerrufen wird.
	Angekuendigt int64 `json:"angekuendigt,omitempty"`
	// umzug_ankuendigen: Pruefstufe, die der Coordinator mit seiner Bindung
	// unterschrieben hat ("streng" oder "einfach").
	Pruefung string `json:"pruefung,omitempty"`
}

// ------------------------------------------------------------ Nachrichten

func umzugBindungNachricht(modus, n, neu string, zeit int64, pruefung string) []byte {
	return []byte(umzugDomaeneBindung + "|" + modus + "|" + n + "|" + neu + "|" + strconv.FormatInt(zeit, 10) + "|" + pruefung)
}

func umzugAussageNachricht(modus, n string, zeit int64) []byte {
	return []byte(umzugDomaeneAussage + "|" + modus + "|" + n + "|" + strconv.FormatInt(zeit, 10))
}

func umzugAntragNachricht(n, alt, neu string, zeit int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%s|%d", umzugDomaeneAntrag, eingefroreneChainID, n, alt, neu, zeit)
}

func umzugWiderrufNachricht(n, neu string, angekuendigt int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%d", umzugDomaeneWiderruf, eingefroreneChainID, n, neu, angekuendigt)
}

func umzugCodeSetzenNachricht(alt, hash string, zeit int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%d", umzugDomaeneCodeSetz, eingefroreneChainID, alt, hash, zeit)
}

// umzugCodeHash: was auf der Kette steht -- an die Wallet gebunden, damit
// derselbe Code fuer zwei Wallets zwei Hashes ergibt.
func umzugCodeHash(alt, code string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|chain:%d|%s|%s", umzugDomaeneCode, eingefroreneChainID, alt, code)))
	return hex.EncodeToString(h[:])
}

// umzugCodeNormal: Grossbuchstaben, ohne Bindestriche und Leerzeichen; nur
// Zeichen aus umzugCodeZeichen, genau umzugCodeLaenge. "" = ungueltig.
func umzugCodeNormal(roh string) string {
	if len(roh) > 64 {
		return ""
	}
	var b strings.Builder
	for _, c := range strings.ToUpper(roh) {
		if c == '-' || c == ' ' {
			continue
		}
		if !strings.ContainsRune(umzugCodeZeichen, c) {
			return ""
		}
		b.WriteRune(c)
	}
	if b.Len() != umzugCodeLaenge {
		return ""
	}
	return b.String()
}

func istHash64(h string) bool { return len(h) == 64 && kleinHex(h) }

// umzugNullifierKanonisch: der Nullifier als Dezimalzahl ohne fuehrende
// Nullen, im Feld -- wie register_human ihn schreibt.
func umzugNullifierKanonisch(n string) bool {
	if !nullifierDezimal.MatchString(n) {
		return false
	}
	c, err := canonicalNullifier(n)
	return err == nil && c == n && len(n) <= 78
}

// ------------------------------------------------------------ Register

// umzugCode: die Codes einer Wallet. Ein neuer Code kommt DAZU, er ersetzt
// keinen (Sicherheitsdurchgang des Diffs): sonst ersetzte ein Dieb mit dem
// alten Schluessel den Code des Menschen, und den Weg ohne Code koennte er
// mit demselben Schluessel jedes Mal widerrufen -- der Mensch kaeme nie mehr
// an sein Menschsein. Ein Code allein bewirkt nichts ohne das Gesicht.
type umzugCode struct {
	Wallet string   `json:"w"`
	Hashes []string `json:"h"`
	Zeit   int64    `json:"z"`
}

type umzugOffen struct {
	Nullifier string `json:"n"`
	Alt       string `json:"alt"`
	Neu       string `json:"neu"`
	Zeit      int64  `json:"z"`
	FreiAb    int64  `json:"frei_ab"`
	// Fest: nicht mehr widerrufbar (umzugFristFest).
	Fest bool `json:"fest,omitempty"`
}

// umzugWiderrufen: ein Widerruf, fuer die Jahresgrenze der Widerrufe.
type umzugWiderrufen struct {
	Nullifier string `json:"n"`
	Am        int64  `json:"am"`
}

func (o umzugOffen) frist() int64 {
	if o.Fest {
		return umzugFristFest
	}
	return umzugFrist
}

type umzugErledigt struct {
	Nullifier string `json:"n"`
	Alt       string `json:"alt"`
	Neu       string `json:"neu"`
	Am        int64  `json:"am"`
}

// umzugRegister: unveraenderlich; jede Aenderung baut ein neues
// (cs.umzugReg ist ein atomarer Zeiger). Kanonisch: jede Liste sortiert,
// keine Dopplung.
type umzugRegister struct {
	Codes    []umzugCode     `json:"codes,omitempty"`
	Offen    []umzugOffen    `json:"offen,omitempty"`
	Erledigt []umzugErledigt `json:"erledigt,omitempty"`
	// Widerrufe: hoechstens umzugWiderrufeHoechstens, nach (Am, Nullifier).
	Widerrufe []umzugWiderrufen `json:"widerrufe,omitempty"`

	roh  string
	hash string
}

func (r *umzugRegister) leer() bool {
	return r == nil || (len(r.Codes) == 0 && len(r.Offen) == 0 && len(r.Erledigt) == 0 && len(r.Widerrufe) == 0)
}

// fertig: Schreibweise und Hash einmal rechnen (die StateRoot liest den Hash
// je Block).
func (r *umzugRegister) fertig() *umzugRegister {
	if r.leer() {
		r.roh, r.hash = "", ""
		return r
	}
	b, _ := json.Marshal(r)
	r.roh = string(b)
	h := sha256.Sum256(b)
	r.hash = hex.EncodeToString(h[:])
	return r
}

func (r *umzugRegister) kodiert() string {
	if r == nil {
		return ""
	}
	return r.roh
}

func umzugRegisterLesen(s string) (*umzugRegister, error) {
	r := &umzugRegister{}
	if s == "" {
		return r.fertig(), nil
	}
	if len(s) > 64<<20 {
		return nil, errors.New("Umzugs-Register zu gross")
	}
	dec := json.NewDecoder(strings.NewReader(s))
	dec.DisallowUnknownFields()
	if err := dec.Decode(r); err != nil {
		return nil, fmt.Errorf("Umzugs-Register nicht lesbar: %w", err)
	}
	if len(r.Codes) > umzugCodesHoechstens || len(r.Offen) > umzugOffenHoechstens || len(r.Erledigt) > umzugErledigtHoechstens ||
		len(r.Widerrufe) > umzugWiderrufeHoechstens {
		return nil, errors.New("Umzugs-Register ueber seinen Grenzen")
	}
	for i, c := range r.Codes {
		if !kanonischeAdresse(c.Wallet) || len(c.Hashes) == 0 || len(c.Hashes) > umzugCodesJeWallet || c.Zeit <= 0 ||
			(i > 0 && c.Wallet <= r.Codes[i-1].Wallet) {
			return nil, fmt.Errorf("Umzugs-Register: Code %d ungueltig", i)
		}
		for j, h := range c.Hashes {
			if !istHash64(h) || (j > 0 && h == c.Hashes[j-1]) {
				return nil, fmt.Errorf("Umzugs-Register: Code %d ungueltig", i)
			}
		}
	}
	for i, o := range r.Offen {
		if !umzugNullifierKanonisch(o.Nullifier) || !kanonischeAdresse(o.Alt) || !kanonischeAdresse(o.Neu) || o.Alt == o.Neu ||
			o.Zeit <= 0 || o.FreiAb != o.Zeit+o.frist() || (i > 0 && o.Nullifier <= r.Offen[i-1].Nullifier) {
			return nil, fmt.Errorf("Umzugs-Register: offener Umzug %d ungueltig", i)
		}
	}
	for i, e := range r.Erledigt {
		if !umzugNullifierKanonisch(e.Nullifier) || !kanonischeAdresse(e.Alt) || !kanonischeAdresse(e.Neu) || e.Am <= 0 ||
			(i > 0 && !umzugErledigtVor(r.Erledigt[i-1], e)) {
			return nil, fmt.Errorf("Umzugs-Register: erledigter Umzug %d ungueltig", i)
		}
	}
	for i, w := range r.Widerrufe {
		if !umzugNullifierKanonisch(w.Nullifier) || w.Am <= 0 || (i > 0 && !umzugWiderrufVor(r.Widerrufe[i-1], w)) {
			return nil, fmt.Errorf("Umzugs-Register: Widerruf %d ungueltig", i)
		}
	}
	r.fertig()
	if r.kodiert() != s {
		return nil, errors.New("Umzugs-Register nicht in kanonischer Schreibweise")
	}
	return r, nil
}

func umzugErledigtVor(a, b umzugErledigt) bool {
	if a.Am != b.Am {
		return a.Am < b.Am
	}
	if a.Nullifier != b.Nullifier {
		return a.Nullifier < b.Nullifier
	}
	return a.Neu < b.Neu
}

func umzugWiderrufVor(a, b umzugWiderrufen) bool {
	if a.Am != b.Am {
		return a.Am < b.Am
	}
	return a.Nullifier < b.Nullifier
}

// widerrufeSeit: Widerrufe fuer diesen Nullifier in den 365 Tagen vor t.
func (r *umzugRegister) widerrufeSeit(n string, t int64) int {
	z := 0
	for _, w := range r.Widerrufe {
		if w.Nullifier == n && w.Am > t-umzugJahr {
			z++
		}
	}
	return z
}

// mitWiderruf: haengt w an und streicht, was aelter als umzugErledigtHalten
// vor w.Am ist (deterministisch, w.Am kommt aus dem Register).
func (r *umzugRegister) mitWiderruf(w umzugWiderrufen) *umzugRegister {
	k := r.kopie()
	behalten := k.Widerrufe[:0]
	for _, x := range k.Widerrufe {
		if x.Am > w.Am-umzugErledigtHalten {
			behalten = append(behalten, x)
		}
	}
	k.Widerrufe = append(behalten, w)
	sort.Slice(k.Widerrufe, func(i, j int) bool { return umzugWiderrufVor(k.Widerrufe[i], k.Widerrufe[j]) })
	return k.fertig()
}

func (r *umzugRegister) kopie() *umzugRegister {
	codes := make([]umzugCode, len(r.Codes))
	for i, c := range r.Codes {
		codes[i] = umzugCode{Wallet: c.Wallet, Hashes: append([]string(nil), c.Hashes...), Zeit: c.Zeit}
	}
	return &umzugRegister{
		Codes:     codes,
		Offen:     append([]umzugOffen(nil), r.Offen...),
		Erledigt:  append([]umzugErledigt(nil), r.Erledigt...),
		Widerrufe: append([]umzugWiderrufen(nil), r.Widerrufe...),
	}
}

func (r *umzugRegister) code(w string) (umzugCode, bool) {
	i := sort.Search(len(r.Codes), func(i int) bool { return r.Codes[i].Wallet >= w })
	if i < len(r.Codes) && r.Codes[i].Wallet == w {
		return r.Codes[i], true
	}
	return umzugCode{}, false
}

func (r *umzugRegister) offen(n string) (umzugOffen, bool) {
	i := sort.Search(len(r.Offen), func(i int) bool { return r.Offen[i].Nullifier >= n })
	if i < len(r.Offen) && r.Offen[i].Nullifier == n {
		return r.Offen[i], true
	}
	return umzugOffen{}, false
}

// offenMit: der offene Umzug, an dem w als alte oder neue Wallet beteiligt ist.
func (r *umzugRegister) offenMit(w string) (umzugOffen, bool) {
	for _, o := range r.Offen {
		if o.Alt == w || o.Neu == w {
			return o, true
		}
	}
	return umzugOffen{}, false
}

// erledigtSeit: vollzogene Umzuege dieses Nullifiers in den 365 Tagen vor t.
func (r *umzugRegister) erledigtSeit(n string, t int64) int {
	z := 0
	for _, e := range r.Erledigt {
		if e.Nullifier == n && e.Am > t-umzugJahr {
			z++
		}
	}
	return z
}

// mitCode: haengt den Hash an (in der Reihenfolge des Hinterlegens) und
// setzt die Zeit -- c.Hashes traegt genau einen neuen Hash.
func (r *umzugRegister) mitCode(c umzugCode) *umzugRegister {
	k := r.kopie()
	i := sort.Search(len(k.Codes), func(i int) bool { return k.Codes[i].Wallet >= c.Wallet })
	if i < len(k.Codes) && k.Codes[i].Wallet == c.Wallet {
		k.Codes[i].Hashes = append(k.Codes[i].Hashes, c.Hashes...)
		k.Codes[i].Zeit = c.Zeit
	} else {
		k.Codes = append(k.Codes, umzugCode{})
		copy(k.Codes[i+1:], k.Codes[i:])
		k.Codes[i] = c
	}
	return k.fertig()
}

func (r *umzugRegister) ohneCode(w string) *umzugRegister {
	k := r.kopie()
	i := sort.Search(len(k.Codes), func(i int) bool { return k.Codes[i].Wallet >= w })
	if i < len(k.Codes) && k.Codes[i].Wallet == w {
		k.Codes = append(k.Codes[:i], k.Codes[i+1:]...)
	}
	return k.fertig()
}

func (r *umzugRegister) mitOffen(o umzugOffen) *umzugRegister {
	k := r.kopie()
	i := sort.Search(len(k.Offen), func(i int) bool { return k.Offen[i].Nullifier >= o.Nullifier })
	k.Offen = append(k.Offen, umzugOffen{})
	copy(k.Offen[i+1:], k.Offen[i:])
	k.Offen[i] = o
	return k.fertig()
}

func (r *umzugRegister) ohneOffen(n string) *umzugRegister {
	k := r.kopie()
	i := sort.Search(len(k.Offen), func(i int) bool { return k.Offen[i].Nullifier >= n })
	if i < len(k.Offen) && k.Offen[i].Nullifier == n {
		k.Offen = append(k.Offen[:i], k.Offen[i+1:]...)
	}
	return k.fertig()
}

// mitErledigt: haengt e an und streicht, was aelter als umzugErledigtHalten
// vor e.Am ist -- deterministisch, weil e.Am aus der Transaktion kommt.
func (r *umzugRegister) mitErledigt(e umzugErledigt) *umzugRegister {
	k := r.kopie()
	behalten := k.Erledigt[:0]
	for _, x := range k.Erledigt {
		if x.Am > e.Am-umzugErledigtHalten {
			behalten = append(behalten, x)
		}
	}
	k.Erledigt = append(behalten, e)
	sort.Slice(k.Erledigt, func(i, j int) bool { return umzugErledigtVor(k.Erledigt[i], k.Erledigt[j]) })
	return k.fertig()
}

// umzugRegisterJetzt: das Register, ohne Sperre lesbar.
func (cs *ChainState) umzugRegisterJetzt() *umzugRegister {
	if r := cs.umzugReg.Load(); r != nil {
		return r
	}
	return (&umzugRegister{}).fertig()
}

// umzugRegisterLaden: beim Start und nach einem Snapshot-Import. Ein
// unlesbares Register haelt den Knoten an.
func (cs *ChainState) umzugRegisterLaden(ctx context.Context) error {
	if cs.db == nil {
		return nil
	}
	r, err := umzugRegisterLesen(cs.getConfigValueCtx(ctx, umzugKonfigSchluessel))
	if err != nil {
		return err
	}
	cs.umzugReg.Store(r)
	return nil
}

// umzugRegisterSetzenLocked: in derselben Datenbank-Transaktion wie der Rest
// des Blocks; der Zeiger geht mit blockRollbackSnapshot zurueck.
func (cs *ChainState) umzugRegisterSetzenLocked(ctx context.Context, r *umzugRegister) error {
	if cs.db != nil {
		if err := cs.setConfigValueCtx(ctx, umzugKonfigSchluessel, r.kodiert()); err != nil {
			return fmt.Errorf("Umzugs-Register speichern: %w", err)
		}
	}
	cs.umzugReg.Store(r)
	return nil
}

// ------------------------------------------------------------ Zustand

// umzugNullifierInhaberLocked: wem der Nullifier im eigenen Zustand gehoert
// (in der laufenden Datenbank-Transaktion). Fehler = nicht lesbar.
func (cs *ChainState) umzugNullifierInhaberLocked(ctx context.Context, n string) (string, error) {
	if cs.db == nil {
		cs.nullifiersMu.Lock()
		defer cs.nullifiersMu.Unlock()
		return cs.nullifiers[n], nil
	}
	var w string
	err := cs.dbExecCtx(ctx).QueryRow(`SELECT wallet_address FROM nullifiers WHERE nullifier = $1`, n).Scan(&w)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.ToLower(w), nil
}

// umzugHatNullifierLocked: hat die Wallet schon einen Nullifier?
func (cs *ChainState) umzugHatNullifierLocked(ctx context.Context, w string) (bool, error) {
	if cs.db == nil {
		cs.nullifiersMu.Lock()
		defer cs.nullifiersMu.Unlock()
		for _, x := range cs.nullifiers {
			if x == w {
				return true, nil
			}
		}
		return false, nil
	}
	var anzahl int
	if err := cs.dbExecCtx(ctx).QueryRow(`SELECT count(*) FROM nullifiers WHERE wallet_address = $1 OR lower(wallet_address) = $1`, w).Scan(&anzahl); err != nil {
		return false, err
	}
	return anzahl > 0, nil
}

// umzugZielTauglichLocked: darf w2 das Menschsein aufnehmen? "" = ja, sonst
// der Grund.
func (cs *ChainState) umzugZielTauglichLocked(ctx context.Context, w2 string) (string, error) {
	if istSystemAdresse(w2) {
		return "neue Wallet ist eine Systemadresse", nil
	}
	cs.ensureAccountLoadedCtx(ctx, w2)
	if acc, ok := cs.accounts.Get(w2); ok && acc.IsHuman {
		return "neue Wallet ist schon ein Mensch", nil
	}
	if cs.wirt().istUnternehmen(w2) {
		return "neue Wallet ist ein Unternehmen", nil
	}
	hat, err := cs.umzugHatNullifierLocked(ctx, w2)
	if err != nil {
		return "", err
	}
	if hat {
		return "neue Wallet hat schon einen Nullifier", nil
	}
	return "", nil
}

// umzugVollziehenLocked: das Menschsein von alt auf neu. Kein Geld bewegt
// sich, keins entsteht. Caller haelt cs.mu, in der Datenbank-Transaktion des
// Blocks.
func (cs *ChainState) umzugVollziehenLocked(ctx context.Context, n, alt, neu string, t int64) error {
	cs.ensureAccountLoadedCtx(ctx, alt)
	cs.ensureAccountLoadedCtx(ctx, neu)
	a1, ok := cs.accounts.Get(alt)
	if !ok || !a1.IsHuman {
		return fmt.Errorf("umzug %s: alte Wallet ist kein Mensch", kurzAdresse(alt))
	}
	a2, ok := cs.accounts.Get(neu)
	if !ok {
		a2 = &AccountState{Address: neu}
		cs.accounts.Set(neu, a2)
	}
	if a2.IsHuman {
		return fmt.Errorf("umzug %s: neue Wallet ist schon ein Mensch", kurzAdresse(neu))
	}
	if cs.db != nil {
		res, err := cs.dbExecCtx(ctx).Exec(`UPDATE nullifiers SET wallet_address = $1 WHERE nullifier = $2 AND lower(wallet_address) = $3`, neu, n, alt)
		if err != nil {
			return fmt.Errorf("umzug: Nullifier umschreiben: %w", err)
		}
		if k, _ := res.RowsAffected(); k != 1 {
			return fmt.Errorf("umzug: Nullifier gehoert nicht %s", kurzAdresse(alt))
		}
		if _, err := cs.dbExecCtx(ctx).Exec(`UPDATE bio_registrations SET wallet_address = $1 WHERE lower(wallet_address) = $2`, neu, alt); err != nil {
			return fmt.Errorf("umzug: Einschreibung umschreiben: %w", err)
		}
	}
	cs.nullifiersMu.Lock()
	if cs.db == nil {
		cs.nullifiers[n] = neu
	} else {
		// Der Zwischenspeicher wird nur geleert: geht der Block zurueck,
		// liest der naechste Zugriff wieder die Datenbank.
		delete(cs.nullifiers, n)
	}
	cs.nullifiersMu.Unlock()

	a2.IsHuman = true
	a2.FaucetClaimed = true // kein zweites Startgeld
	a2.LebensnachweisAm = a1.LebensnachweisAm
	a2.GrantStagedRest = a1.GrantStagedRest
	a2.GrantStagedUntil = a1.GrantStagedUntil
	a2.LivenessRenewedAt = a1.LivenessRenewedAt
	a2.StaffelEinspruchAm = a1.StaffelEinspruchAm

	a1.IsHuman = false
	a1.LebensnachweisAm = 0
	a1.GrantStagedRest = 0
	a1.GrantStagedUntil = 0
	a1.LivenessRenewedAt = 0
	a1.StaffelEinspruchAm = 0

	if err := cs.saveAccountToDBCtx(ctx, a1); err != nil {
		return fmt.Errorf("umzug: alte Wallet speichern: %w", err)
	}
	if err := cs.saveAccountToDBCtx(ctx, a2); err != nil {
		return fmt.Errorf("umzug: neue Wallet speichern: %w", err)
	}
	reg := cs.umzugRegisterJetzt().ohneOffen(n).ohneCode(alt).mitErledigt(umzugErledigt{Nullifier: n, Alt: alt, Neu: neu, Am: t})
	return cs.umzugRegisterSetzenLocked(ctx, reg)
}

// ------------------------------------------------------------ Urteile

// umzugUrteil: regel != "" = Befund (nicht angewandt).
type umzugUrteil struct {
	regel string
	text  string
	// sofort: Antrag mit gueltigem Code, der Umzug geschieht jetzt.
	sofort bool
	// verfallen: Vollzug, dessen Bedingungen nicht mehr gelten -- der offene
	// Umzug wird gestrichen.
	verfallen bool
	// fest: Antrag ohne Code nach einem Widerruf im letzten Jahr -- 30 Tage,
	// nicht widerrufbar.
	fest bool
}

func umzugBefund(regel, format string, args ...interface{}) umzugUrteil {
	return umzugUrteil{regel: regel, text: fmt.Sprintf(format, args...)}
}

// umzugAnkuendigenUrteilLocked: darf der Antrag zu blockZeit angewandt
// werden? Billig vor teuer. Fehler nur, wenn der eigene Zustand nicht lesbar
// war: dann geht der Block zurueck. Caller haelt cs.mu.
func (cs *ChainState) umzugAnkuendigenUrteilLocked(ctx context.Context, tx *Transaction, blockZeit int64) (umzugUrteil, error) {
	if !umzugAktiv(blockZeit) {
		return umzugBefund("umzug_form", "Umzug vor dem Stichtag"), nil
	}
	neu, alt, n := tx.Wallet, tx.To, tx.Nullifier
	u := tx.Umzug
	b := tx.BioBescheinigung
	if !kanonischeAdresse(neu) || !kanonischeAdresse(alt) || neu == alt || !umzugNullifierKanonisch(n) ||
		u == nil || !kanonischeSignatur(u.Sig) || u.CodeHash != "" || u.Angekuendigt != 0 ||
		(u.Pruefung != umzugPruefungStreng && u.Pruefung != umzugPruefungEinfach) ||
		(u.Code != "" && umzugCodeNormal(u.Code) != u.Code) || b == nil || b.Zeit != tx.DistributionAt {
		return umzugBefund("umzug_form", "Antrag nicht in gueltiger Form"), nil
	}
	z := tx.DistributionAt
	if z <= 0 || z > sattAdd(blockZeit, umzugHoechstensVoraus) || blockZeit-z > umzugHoechstensAlt {
		return umzugBefund("umzug_zeit", "%s: bescheinigt %d, Block %d", kurzAdresse(neu), z, blockZeit), nil
	}
	if err := pruefePersonalSignGemerkt(umzugAntragNachricht(n, alt, neu, z), u.Sig, neu); err != nil {
		return umzugBefund("umzug_unterschrift", "%s: Antrag nicht von der neuen Wallet unterschrieben", kurzAdresse(neu)), nil
	}
	k, err := bescheinigungKonf()
	if err != nil {
		return umzugUrteil{}, err
	}
	// Ohne die Bescheinigung im Konsens (K1) gibt es keine Schluessel, gegen
	// die geprueft wird -- dann kein Umzug.
	if k.AbUnix <= 0 || blockZeit < k.AbUnix {
		return umzugBefund("umzug_bescheinigung", "Bescheinigung im Konsens noch nicht aktiv"), nil
	}
	if b.Modus != k.Modus {
		return umzugBefund("umzug_bescheinigung", "Modus %q, das Netz verlangt %q", b.Modus, k.Modus), nil
	}
	if err := pruefeV3Unterschriften(b, k, umzugBindungNachricht(b.Modus, n, neu, z, u.Pruefung), umzugAussageNachricht(b.Modus, n, z),
		blockZeit, cs.verifierRegisterJetzt().mitglied); err != nil {
		return umzugBefund("umzug_bescheinigung", "%s: %v", kurzAdresse(neu), err), nil
	}
	inhaber, err := cs.umzugNullifierInhaberLocked(ctx, n)
	if err != nil {
		return umzugUrteil{}, err
	}
	if inhaber != alt {
		return umzugBefund("umzug_nullifier", "Nullifier gehoert nicht %s", kurzAdresse(alt)), nil
	}
	cs.ensureAccountLoadedCtx(ctx, alt)
	if a1, ok := cs.accounts.Get(alt); !ok || !a1.IsHuman {
		return umzugBefund("umzug_nullifier", "%s ist kein Mensch", kurzAdresse(alt)), nil
	}
	grund, err := cs.umzugZielTauglichLocked(ctx, neu)
	if err != nil {
		return umzugUrteil{}, err
	}
	if grund != "" {
		return umzugBefund("umzug_ziel", "%s: %s", kurzAdresse(neu), grund), nil
	}
	reg := cs.umzugRegisterJetzt()
	// Ein offener Umzug ohne Code haelt nur einen weiteren OHNE Code auf: mit
	// Gesicht und Code zieht der Mensch sofort um und beendet ihn damit --
	// auch den eines Angreifers.
	if _, offen := reg.offen(n); offen && u.Code == "" {
		return umzugBefund("umzug_offen", "fuer diesen Menschen ist schon ein Umzug offen"), nil
	}
	if o, offen := reg.offenMit(neu); offen && o.Nullifier != n {
		return umzugBefund("umzug_offen", "%s ist schon an einem offenen Umzug beteiligt", kurzAdresse(neu)), nil
	}
	if reg.erledigtSeit(n, z) >= umzugJeJahr {
		return umzugBefund("umzug_jahresgrenze", "schon %d Umzuege in 365 Tagen", umzugJeJahr), nil
	}
	if len(reg.Erledigt) >= umzugErledigtHoechstens {
		return umzugBefund("umzug_voll", "Register voll"), nil
	}
	if u.Code != "" {
		// Ein Antrag mit Code von vor dem Stichtag "nur der letzte Code" nur
		// in einen Block davor -- sonst entschieden Annahme und Nachspielen
		// verschieden.
		if !umzugNurLetzterCode(z) && umzugNurLetzterCode(blockZeit) {
			return umzugBefund("umzug_zeit", "%s: Antrag mit Code bescheinigt %d vor dem Stichtag, Block %d danach", kurzAdresse(neu), z, blockZeit), nil
		}
		c, ok := reg.code(alt)
		passt := false
		if ok && len(c.Hashes) > 0 {
			soll := umzugCodeHash(alt, u.Code)
			if umzugNurLetzterCode(z) {
				// Nur der zuletzt hinterlegte (Hashes in der Reihenfolge des
				// Hinterlegens, umzugRegister.mitCode).
				passt = c.Hashes[len(c.Hashes)-1] == soll
			} else {
				for _, h := range c.Hashes {
					passt = passt || h == soll
				}
			}
		}
		if !passt {
			return umzugBefund("umzug_code", "%s: Wiederherstellungscode passt nicht", kurzAdresse(alt)), nil
		}
		return umzugUrteil{sofort: true}, nil
	}
	// Ohne Code nur nach der strengsten Pruefung des Coordinators: gruene
	// Lebendigkeit UND an die Aufnahme gebundene Hardware-Attestierung (gegen
	// eingespielte Videos und Deepfakes ueber eine virtuelle Kamera).
	if u.Pruefung != umzugPruefungStreng {
		return umzugBefund("umzug_pruefung", "%s: ohne Code nur nach strenger Pruefung", kurzAdresse(neu)), nil
	}
	if len(reg.Offen) >= umzugOffenHoechstens {
		return umzugBefund("umzug_voll", "zu viele offene Umzuege"), nil
	}
	return umzugUrteil{fest: reg.widerrufeSeit(n, z) >= umzugWiderrufeJeJahr}, nil
}

// umzugAnkuendigenAnwendenLocked: nach einem Urteil ohne Befund.
func (cs *ChainState) umzugAnkuendigenAnwendenLocked(ctx context.Context, tx *Transaction, u umzugUrteil) error {
	if u.sofort {
		return cs.umzugVollziehenLocked(ctx, tx.Nullifier, tx.To, tx.Wallet, tx.DistributionAt)
	}
	o := umzugOffen{Nullifier: tx.Nullifier, Alt: tx.To, Neu: tx.Wallet, Zeit: tx.DistributionAt, Fest: u.fest}
	o.FreiAb = o.Zeit + o.frist()
	reg := cs.umzugRegisterJetzt().mitOffen(o)
	return cs.umzugRegisterSetzenLocked(ctx, reg)
}

// umzugWiderrufUrteilLocked: W1 widerruft einen offenen Umzug.
func (cs *ChainState) umzugWiderrufUrteilLocked(tx *Transaction, blockZeit int64) umzugUrteil {
	if !umzugAktiv(blockZeit) {
		return umzugBefund("umzug_form", "Umzug vor dem Stichtag")
	}
	alt, neu, n := tx.Wallet, tx.To, tx.Nullifier
	u := tx.Umzug
	if !kanonischeAdresse(alt) || !kanonischeAdresse(neu) || !umzugNullifierKanonisch(n) || u == nil ||
		!kanonischeSignatur(u.Sig) || u.Code != "" || u.CodeHash != "" || u.Pruefung != "" || u.Angekuendigt <= 0 ||
		tx.BioBescheinigung != nil {
		return umzugBefund("umzug_form", "Widerruf nicht in gueltiger Form")
	}
	o, ok := cs.umzugRegisterJetzt().offen(n)
	if !ok || o.Alt != alt || o.Neu != neu || o.Zeit != u.Angekuendigt {
		return umzugBefund("umzug_nicht_offen", "kein passender offener Umzug")
	}
	if o.Fest {
		return umzugBefund("umzug_fest", "dieser Umzug ist nicht mehr widerrufbar (schon ein Widerruf im letzten Jahr)")
	}
	if err := pruefePersonalSignGemerkt(umzugWiderrufNachricht(n, neu, u.Angekuendigt), u.Sig, alt); err != nil {
		return umzugBefund("umzug_unterschrift", "Widerruf nicht von %s unterschrieben", kurzAdresse(alt))
	}
	return umzugUrteil{}
}

// umzugCodeUrteilLocked: W1 hinterlegt den Hash seines Codes.
func (cs *ChainState) umzugCodeUrteilLocked(ctx context.Context, tx *Transaction, blockZeit int64) umzugUrteil {
	if !umzugAktiv(blockZeit) {
		return umzugBefund("umzug_form", "Umzug vor dem Stichtag")
	}
	alt := tx.Wallet
	u := tx.Umzug
	if !kanonischeAdresse(alt) || u == nil || !istHash64(u.CodeHash) || !kanonischeSignatur(u.Sig) ||
		u.Code != "" || u.Angekuendigt != 0 || u.Pruefung != "" || tx.BioBescheinigung != nil || tx.To != "" || tx.Nullifier != "" {
		return umzugBefund("umzug_form", "Code nicht in gueltiger Form")
	}
	z := tx.DistributionAt
	if z <= 0 || z > sattAdd(blockZeit, umzugHoechstensVoraus) || blockZeit-z > umzugHoechstensAlt {
		return umzugBefund("umzug_zeit", "%s: unterschrieben %d, Block %d", kurzAdresse(alt), z, blockZeit)
	}
	// Ein Auftrag von vor dem Stichtag nur in einem Block vor dem Stichtag --
	// sonst haengte das Ergebnis davon ab, ob mit Uhr- oder Blockzeit
	// gerechnet wird, und ein zurueckdatierter Auftrag umginge die Regel.
	if !umzugCodeRegeln(z) && umzugCodeRegeln(blockZeit) {
		return umzugBefund("umzug_zeit", "%s: unterschrieben %d vor dem Stichtag der Code-Regeln, Block %d danach", kurzAdresse(alt), z, blockZeit)
	}
	cs.ensureAccountLoadedCtx(ctx, alt)
	if a, ok := cs.accounts.Get(alt); !ok || !a.IsHuman {
		return umzugBefund("umzug_code_kein_mensch", "%s ist kein Mensch", kurzAdresse(alt))
	}
	if err := pruefePersonalSignGemerkt(umzugCodeSetzenNachricht(alt, u.CodeHash, z), u.Sig, alt); err != nil {
		return umzugBefund("umzug_unterschrift", "Code nicht von %s unterschrieben", kurzAdresse(alt))
	}
	reg := cs.umzugRegisterJetzt()
	if umzugCodeRegeln(z) {
		if _, ok := reg.code(alt); ok {
			return umzugBefund("umzug_code_fest", "%s: hat schon einen Wiederherstellungscode, er ist nicht aenderbar", kurzAdresse(alt))
		}
	}
	if c, ok := reg.code(alt); ok {
		if c.Zeit >= z {
			// Wiederholung oder aelter als der zuletzt hinterlegte.
			return umzugBefund("umzug_code_alt", "%s: hinterlegter Code ist neuer", kurzAdresse(alt))
		}
		if len(c.Hashes) >= umzugCodesJeWallet {
			return umzugBefund("umzug_code_voll", "%s: schon %d Codes", kurzAdresse(alt), umzugCodesJeWallet)
		}
		for _, h := range c.Hashes {
			if h == u.CodeHash {
				return umzugBefund("umzug_code_alt", "%s: dieser Code ist schon hinterlegt", kurzAdresse(alt))
			}
		}
	} else if len(reg.Codes) >= umzugCodesHoechstens {
		return umzugBefund("umzug_voll", "Code-Register voll")
	}
	return umzugUrteil{}
}

// umzugVollzugUrteilLocked: der Vollzug eines offenen Umzugs zur Rundenzeit
// t = tx.DistributionAt. Gelten die Bedingungen nicht mehr, verfaellt er.
func (cs *ChainState) umzugVollzugUrteilLocked(ctx context.Context, tx *Transaction, blockZeit int64) (umzugUrteil, error) {
	if !umzugAktiv(blockZeit) {
		return umzugBefund("umzug_form", "Umzug vor dem Stichtag"), nil
	}
	neu, alt, n, t := tx.Wallet, tx.To, tx.Nullifier, tx.DistributionAt
	if !kanonischeAdresse(neu) || !kanonischeAdresse(alt) || !umzugNullifierKanonisch(n) || tx.Umzug != nil || tx.BioBescheinigung != nil {
		return umzugBefund("umzug_form", "Vollzug nicht in gueltiger Form"), nil
	}
	o, ok := cs.umzugRegisterJetzt().offen(n)
	if !ok || o.Alt != alt || o.Neu != neu {
		return umzugBefund("umzug_nicht_offen", "kein passender offener Umzug"), nil
	}
	if t < o.FreiAb || t > sattAdd(blockZeit, 60) {
		return umzugBefund("umzug_frist", "Vollzug zur Rundenzeit %d, frei ab %d, Block %d", t, o.FreiAb, blockZeit), nil
	}
	return cs.umzugVollzugBedingungenLocked(ctx, o)
}

// umzugVollzugBedingungenLocked: gelten die Bedingungen des Antrags noch?
// Nein = verfallen (kein Befund: beide Ausgaenge rechnet jeder Knoten gleich).
func (cs *ChainState) umzugVollzugBedingungenLocked(ctx context.Context, o umzugOffen) (umzugUrteil, error) {
	inhaber, err := cs.umzugNullifierInhaberLocked(ctx, o.Nullifier)
	if err != nil {
		return umzugUrteil{}, err
	}
	cs.ensureAccountLoadedCtx(ctx, o.Alt)
	a1, ok := cs.accounts.Get(o.Alt)
	if inhaber != o.Alt || !ok || !a1.IsHuman {
		return umzugUrteil{verfallen: true}, nil
	}
	grund, err := cs.umzugZielTauglichLocked(ctx, o.Neu)
	if err != nil {
		return umzugUrteil{}, err
	}
	return umzugUrteil{verfallen: grund != ""}, nil
}

func (cs *ChainState) umzugVollzugAnwendenLocked(ctx context.Context, n, alt, neu string, t int64, u umzugUrteil) error {
	if u.verfallen {
		return cs.umzugRegisterSetzenLocked(ctx, cs.umzugRegisterJetzt().ohneOffen(n))
	}
	return cs.umzugVollziehenLocked(ctx, n, alt, neu, t)
}

// umzugVollzuegeLocked: die Tagesrunde vollzieht jeden offenen Umzug, dessen
// Frist zur Rundenzeit abgelaufen ist (hoechstens umzugVollzugJeRunde, nach
// Nullifier geordnet).
func (cs *ChainState) umzugVollzuegeLocked(ctx context.Context, ubiAt int64) ([]Transaction, error) {
	if !umzugAktiv(ubiAt) {
		return nil, nil
	}
	var faellig []umzugOffen
	for _, o := range cs.umzugRegisterJetzt().Offen {
		if o.FreiAb <= ubiAt {
			faellig = append(faellig, o)
			if len(faellig) == umzugVollzugJeRunde {
				break
			}
		}
	}
	var txs []Transaction
	for _, o := range faellig {
		u, err := cs.umzugVollzugBedingungenLocked(ctx, o)
		if err != nil {
			return nil, err
		}
		if err := cs.umzugVollzugAnwendenLocked(ctx, o.Nullifier, o.Alt, o.Neu, ubiAt, u); err != nil {
			return nil, err
		}
		txs = append(txs, Transaction{Type: umzugVollzugTxArt, Wallet: o.Neu, To: o.Alt, Nullifier: o.Nullifier, DistributionAt: ubiAt})
	}
	return txs, nil
}

// applyUmzugLocked: alle vier Arten beim Nachspielen. Ein ungueltiger Umzug
// wird nie angewandt -- er wird als Befund gezaehlt und lehnt im strengen
// Modus den Block ab. Zustand nicht lesbar: der ganze Block zurueck.
func (cs *ChainState) applyUmzugLocked(ctx context.Context, tx *Transaction, blockZeit int64) error {
	var u umzugUrteil
	var err error
	switch tx.Type {
	case umzugCodeTxArt:
		u = cs.umzugCodeUrteilLocked(ctx, tx, blockZeit)
	case umzugAnkuendigenTxArt:
		u, err = cs.umzugAnkuendigenUrteilLocked(ctx, tx, blockZeit)
	case umzugWiderrufTxArt:
		u = cs.umzugWiderrufUrteilLocked(tx, blockZeit)
	case umzugVollzugTxArt:
		u, err = cs.umzugVollzugUrteilLocked(ctx, tx, blockZeit)
	default:
		return fmt.Errorf("keine Umzugs-Art: %q", tx.Type)
	}
	if err != nil {
		return err
	}
	if u.regel != "" {
		return nachrechnenAbweichung(u.regel, blockZeit, "%s", u.text)
	}
	return cs.umzugAnwendenLocked(ctx, tx, u)
}

func (cs *ChainState) umzugAnwendenLocked(ctx context.Context, tx *Transaction, u umzugUrteil) error {
	switch tx.Type {
	case umzugCodeTxArt:
		return cs.umzugRegisterSetzenLocked(ctx, cs.umzugRegisterJetzt().mitCode(umzugCode{Wallet: tx.Wallet, Hashes: []string{tx.Umzug.CodeHash}, Zeit: tx.DistributionAt}))
	case umzugAnkuendigenTxArt:
		return cs.umzugAnkuendigenAnwendenLocked(ctx, tx, u)
	case umzugWiderrufTxArt:
		reg := cs.umzugRegisterJetzt()
		o, _ := reg.offen(tx.Nullifier)
		return cs.umzugRegisterSetzenLocked(ctx, reg.ohneOffen(tx.Nullifier).mitWiderruf(umzugWiderrufen{Nullifier: tx.Nullifier, Am: o.Zeit}))
	case umzugVollzugTxArt:
		return cs.umzugVollzugAnwendenLocked(ctx, tx.Nullifier, tx.To, tx.Wallet, tx.DistributionAt, u)
	}
	return fmt.Errorf("keine Umzugs-Art: %q", tx.Type)
}

func istUmzugTxArt(t string) bool {
	switch t {
	case umzugCodeTxArt, umzugAnkuendigenTxArt, umzugWiderrufTxArt, umzugVollzugTxArt:
		return true
	}
	return false
}

// ------------------------------------------------------------ API

const burstUmzugJeIP = 10

type umzugAbgelehnt struct{ text string }

func (e *umzugAbgelehnt) Error() string { return e.text }

// umzugAnnehmen: gemeinsamer Ablauf der drei Endpunkte -- Grenze je
// Absender, unter der Sperre genau die Pruefung jedes Nachspielenden,
// Annahme-Tor, Ausgang atomar mit dem Zustand.
func (a *APIServer) umzugAnnehmen(w http.ResponseWriter, r *http.Request, tx Transaction, konten []string) {
	now := time.Now().Unix()
	if err := a.state.annahmeBeginnen(tx.Wallet, konten...); err != nil {
		jsonError(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer a.state.annahmeEnde()
	var ergebnis umzugUrteil
	err := a.state.runAtomicWithOutbox(append([]string{tx.Wallet}, konten...), false, func(ctx context.Context) (Transaction, error) {
		var u umzugUrteil
		var err error
		switch tx.Type {
		case umzugCodeTxArt:
			u = a.state.umzugCodeUrteilLocked(ctx, &tx, now)
		case umzugAnkuendigenTxArt:
			u, err = a.state.umzugAnkuendigenUrteilLocked(ctx, &tx, now)
		case umzugWiderrufTxArt:
			u = a.state.umzugWiderrufUrteilLocked(&tx, now)
		}
		if err != nil {
			return Transaction{}, err
		}
		if u.regel != "" {
			return Transaction{}, &umzugAbgelehnt{text: u.regel + ": " + u.text}
		}
		if err := a.state.umzugAnwendenLocked(ctx, &tx, u); err != nil {
			return Transaction{}, err
		}
		ergebnis = u
		return tx, nil
	})
	if err != nil {
		var abgelehnt *umzugAbgelehnt
		if errors.As(err, &abgelehnt) {
			jsonError(w, "rejected: "+abgelehnt.text, http.StatusConflict)
			return
		}
		fmt.Printf("[API] umzug %s failed for %s: %v\n", tx.Type, kurzAdresse(tx.Wallet), err)
		jsonError(w, "internal error, please retry shortly", http.StatusInternalServerError)
		return
	}
	antwort := map[string]interface{}{"ok": true, "art": tx.Type}
	if tx.Type == umzugAnkuendigenTxArt {
		antwort["sofort"] = ergebnis.sofort
		if !ergebnis.sofort {
			o := umzugOffen{Zeit: tx.DistributionAt, Fest: ergebnis.fest}
			antwort["frei_ab"] = o.Zeit + o.frist()
			antwort["fest"] = ergebnis.fest
		}
	}
	json.NewEncoder(w).Encode(antwort)
}

func (a *APIServer) umzugVorpruefung(w http.ResponseWriter, r *http.Request, ziel interface{}) bool {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return false
	}
	if !umzugAktiv(time.Now().Unix()) {
		jsonError(w, "wallet move not active yet", http.StatusConflict)
		return false
	}
	if !burstErlaubt("umzug:"+erneuerungsAbsender(r), burstUmzugJeIP, burstFenster) {
		jsonError(w, "too many requests from this address", http.StatusTooManyRequests)
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, umzugKoerperMax)).Decode(ziel); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return false
	}
	return true
}

// handleUmzugCode: {"wallet", "code_hash", "zeit", "sig"} -- die App
// hinterlegt den Hash des Wiederherstellungscodes; der Code selbst verlaesst
// das Telefon nicht.
func (a *APIServer) handleUmzugCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Wallet   string `json:"wallet"`
		CodeHash string `json:"code_hash"`
		Zeit     int64  `json:"zeit"`
		Sig      string `json:"sig"`
	}
	if !a.umzugVorpruefung(w, r, &req) {
		return
	}
	wallet := strings.ToLower(strings.TrimSpace(req.Wallet))
	sig := kanonischeSignaturVersuch(req.Sig)
	if !kanonischeAdresse(wallet) || !istHash64(req.CodeHash) || !kanonischeSignatur(sig) {
		jsonError(w, "wallet, code_hash (64 hex) and sig required", http.StatusBadRequest)
		return
	}
	if err := verifyPersonalSign(umzugCodeSetzenNachricht(wallet, req.CodeHash, req.Zeit), sig, wallet); err != nil {
		jsonError(w, "signature does not match wallet", http.StatusForbidden)
		return
	}
	if umzugCodeAnnahmePausiert(req.Zeit, time.Now().Unix()) {
		jsonError(w, fmt.Sprintf("recovery codes paused for the switch to the new code rules at %d -- please sign again then", umzugCodeRegelnAb()), http.StatusServiceUnavailable)
		return
	}
	a.umzugAnnehmen(w, r, Transaction{Type: umzugCodeTxArt, Wallet: wallet, DistributionAt: req.Zeit,
		Umzug: &UmzugAngaben{CodeHash: req.CodeHash, Sig: sig}}, nil)
}

// handleUmzugAntrag: {"neu", "alt", "nullifier", "zeit", "modus",
// "bescheinigung": ["v3:<sig>", ..., "v3k:<bindung>", ...], "code", "sig"} --
// die App der neuen Wallet reicht die Bescheinigung des Coordinators ein,
// in derselben Schreibweise wie bioAttestation bei der Registrierung.
func (a *APIServer) handleUmzugAntrag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Neu       string   `json:"neu"`
		Alt       string   `json:"alt"`
		Nullifier string   `json:"nullifier"`
		Zeit      int64    `json:"zeit"`
		Modus     string   `json:"modus"`
		Eintraege []string `json:"bescheinigung"`
		Pruefung  string   `json:"pruefung"`
		Code      string   `json:"code"`
		Sig       string   `json:"sig"`
	}
	if !a.umzugVorpruefung(w, r, &req) {
		return
	}
	neu := strings.ToLower(strings.TrimSpace(req.Neu))
	alt := strings.ToLower(strings.TrimSpace(req.Alt))
	sig := kanonischeSignaturVersuch(req.Sig)
	code := ""
	if strings.TrimSpace(req.Code) != "" {
		if code = umzugCodeNormal(req.Code); code == "" {
			jsonError(w, "recovery code has the wrong form", http.StatusBadRequest)
			return
		}
	}
	if !kanonischeAdresse(neu) || !kanonischeAdresse(alt) || !umzugNullifierKanonisch(req.Nullifier) || !kanonischeSignatur(sig) ||
		len(req.Eintraege) == 0 || len(req.Eintraege) > 2*bescheinigungMaxSignaturen {
		jsonError(w, "neu, alt, nullifier, bescheinigung and sig required", http.StatusBadRequest)
		return
	}
	for _, e := range req.Eintraege {
		if strings.Contains(e, ",") || (!strings.HasPrefix(e, bescheinigungV3Praefix) && !strings.HasPrefix(e, bescheinigungBindungPraefix)) {
			jsonError(w, "bescheinigung: only v3: and v3k: entries", http.StatusBadRequest)
			return
		}
	}
	_, unterschriften, roheBindungen := teileBescheinigungsfeldMitBindungen(strings.Join(req.Eintraege, ","))
	var bindungen []BescheinigungsBindung
	for _, roh := range roheBindungen {
		b, ok := leseBescheinigungsBindung(roh)
		if !ok {
			jsonError(w, "bescheinigung: unreadable binding", http.StatusBadRequest)
			return
		}
		bindungen = append(bindungen, b)
	}
	if len(unterschriften) == 0 || len(unterschriften) > bescheinigungMaxSignaturen {
		jsonError(w, "bescheinigung: 1 to 17 signatures", http.StatusBadRequest)
		return
	}
	if err := verifyPersonalSign(umzugAntragNachricht(req.Nullifier, alt, neu, req.Zeit), sig, neu); err != nil {
		jsonError(w, "signature does not match the new wallet", http.StatusForbidden)
		return
	}
	if code != "" && umzugAntragMitCodePausiert(req.Zeit, time.Now().Unix()) {
		jsonError(w, fmt.Sprintf("moves with a recovery code paused for the rule change at %d -- please try again then", umzugNurLetzterCodeAb()), http.StatusServiceUnavailable)
		return
	}
	tx := Transaction{Type: umzugAnkuendigenTxArt, Wallet: neu, To: alt, Nullifier: req.Nullifier, DistributionAt: req.Zeit,
		BioBescheinigung: &BioBescheinigung{Zeit: req.Zeit, Modus: req.Modus, Signaturen: unterschriften, Bindungen: bindungen},
		Umzug:            &UmzugAngaben{Code: code, Sig: sig, Pruefung: req.Pruefung}}
	a.umzugAnnehmen(w, r, tx, []string{alt})
}

// handleUmzugWiderruf: {"alt", "neu", "nullifier", "angekuendigt", "sig"}.
func (a *APIServer) handleUmzugWiderruf(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Alt          string `json:"alt"`
		Neu          string `json:"neu"`
		Nullifier    string `json:"nullifier"`
		Angekuendigt int64  `json:"angekuendigt"`
		Sig          string `json:"sig"`
	}
	if !a.umzugVorpruefung(w, r, &req) {
		return
	}
	alt := strings.ToLower(strings.TrimSpace(req.Alt))
	neu := strings.ToLower(strings.TrimSpace(req.Neu))
	sig := kanonischeSignaturVersuch(req.Sig)
	if !kanonischeAdresse(alt) || !kanonischeAdresse(neu) || !umzugNullifierKanonisch(req.Nullifier) || !kanonischeSignatur(sig) || req.Angekuendigt <= 0 {
		jsonError(w, "alt, neu, nullifier, angekuendigt and sig required", http.StatusBadRequest)
		return
	}
	if err := verifyPersonalSign(umzugWiderrufNachricht(req.Nullifier, neu, req.Angekuendigt), sig, alt); err != nil {
		jsonError(w, "signature does not match the old wallet", http.StatusForbidden)
		return
	}
	a.umzugAnnehmen(w, r, Transaction{Type: umzugWiderrufTxArt, Wallet: alt, To: neu, Nullifier: req.Nullifier,
		Umzug: &UmzugAngaben{Sig: sig, Angekuendigt: req.Angekuendigt}}, []string{neu})
}

// handleUmzugStand: GET ?wallet= -- fuer die App: ist ein Code hinterlegt,
// ist ein Umzug offen (als alte oder neue Wallet)? Oeffentlich wie alles
// auf der Kette; der Code-Hash wird nicht ausgegeben.
// GET ?n= -- wem der Nullifier gehoert (fuer den Coordinator).
func (a *APIServer) handleUmzugStand(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodGet {
		jsonError(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	if !burstErlaubt("umzug-stand:"+erneuerungsAbsender(r), burstNullifierPasstJeIP, burstFenster) {
		jsonError(w, "too many requests from this address", http.StatusTooManyRequests)
		return
	}
	reg := a.state.umzugRegisterJetzt()
	antwort := map[string]interface{}{"aktiv_ab": umzugAb(), "frist_tage": umzugFrist / 86400, "code_regeln_ab": umzugCodeRegelnAb(),
		"nur_letzter_code_ab": umzugNurLetzterCodeAb()}
	if n := strings.TrimSpace(r.URL.Query().Get("n")); n != "" {
		if !umzugNullifierKanonisch(n) {
			jsonError(w, "n must be a decimal nullifier", http.StatusBadRequest)
			return
		}
		antwort["inhaber"] = strings.ToLower(a.state.GetWalletByNullifier(n))
		json.NewEncoder(w).Encode(antwort)
		return
	}
	wallet := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("wallet")))
	if !kanonischeAdresse(wallet) {
		jsonError(w, "wallet or n required", http.StatusBadRequest)
		return
	}
	if c, ok := reg.code(wallet); ok {
		antwort["code_seit"] = c.Zeit
		antwort["codes"] = len(c.Hashes)
		// Wie viele bei einem Umzug zaehlen (ab umzugNurLetzterCodeAbUnix
		// nur der zuletzt hinterlegte).
		if umzugNurLetzterCode(time.Now().Unix()) {
			antwort["codes_gueltig"] = 1
		} else {
			antwort["codes_gueltig"] = len(c.Hashes)
		}
	}
	if o, ok := reg.offenMit(wallet); ok {
		antwort["offen"] = map[string]interface{}{"nullifier": o.Nullifier, "alt": o.Alt, "neu": o.Neu, "angekuendigt": o.Zeit,
			"frei_ab": o.FreiAb, "fest": o.Fest}
	}
	json.NewEncoder(w).Encode(antwort)
}
