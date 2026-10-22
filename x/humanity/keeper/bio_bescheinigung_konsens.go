package keeper

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// DIE BESCHEINIGUNG IM KONSENS (Audit 2026-10-21, K1).
//
// # DIE LUECKE
//
// Ein Knoten, der einen fremden Block nachspielt, prueft bei register_human:
// gueltiger Groth16-Beweis, Nullifier = pubSignals[1], EIP-712-Unterschrift
// der neuen Wallet, Nullifier neu. Dann bucht er 1.000 AEQ. Ob die
// Gesichtspruefung stattgefunden hat, sieht er nicht: die Bescheinigung von
// Coordinator und Vergleichsdienst prueft nur der Proof-Server des
// annehmenden Knotens, und der Block traegt sie nicht. Der Beweisschluessel
// ist oeffentlich und `bio` eine Zufallszahl des Coordinators -- wer Bloecke
// erzeugt, konnte beliebig viele "Menschen" mit eigenem Beweis und eigener
// Wallet anlegen, und jeder Knoten nahm sie an.
//
// # WAS JETZT GILT (ab bescheinigung.ab_unix in genesis.json)
//
// Die Transaktion traegt die Bescheinigung: Zeitpunkt, Betriebsmodus und die
// Ed25519-Unterschriften (Form v3). Jeder Knoten prueft selbst:
//
//   - Jede Unterschrift passt zu einem Schluessel aus der Genesis und zu
//     genau einer dieser Nachrichten ueber den NULLIFIER des Beweises
//     (pubSignals[1], kanonisch dezimal):
//       Bindung:  aequitas-bio-attestation-v3|<modus>|<nullifier>|<wallet>|<zeit>
//                 nur Coordinator-Schluessel
//       Aussage:  aequitas-bio-personhood-v3|<modus>|<nullifier>|<zeit>
//                 Bezeuger-Schluessel (ohne eigene Liste: Coordinator)
//   - Mindestens eine Bindung an DIESE Wallet; bei getrennten Listen
//     mindestens ein Bezeuger, der nicht gebunden hat; verschiedene
//     Schluessel >= quorum. Gezaehlt werden Schluessel, nie Signaturen.
//   - Der Zeitpunkt liegt hoechstens bescheinigungHoechstensAlt vor der
//     Annahme (RegAt) und hoechstens 60 s danach.
//
// # WARUM DER NULLIFIER, NICHT bio (v3 statt v2)
//
// Die erste Fassung trug `bio` (die Zufallszahl des Coordinators) im Block
// und pruefte Poseidon(bio) = Nullifier. Genau diese Zahl ist aber die
// bio_hash, mit der ein Mensch beim Coordinator seine Einschreibung abfragt
// (/access) und loescht (DELETE /enrollment) -- ihr Besitz gilt dort als
// Nachweis, die Person zu sein. Im Block waere sie oeffentlich gewesen: jeder
// haette jede Einschreibung loeschen koennen, und der Geloeschte kaeme als
// Fremder ein zweites Mal durch. Der Nullifier steht ohnehin oeffentlich im
// Beweis und laesst sich nicht zurueckrechnen; Coordinator und
// Vergleichsdienst unterschreiben ihn darum selbst (v3, aequitas-biometric-beta
// bio_attestation.py und attest_sign.py, dort Poseidon gegen die Schaltung
// getestet). `bio` kommt in keinen Block.
//
// Die v3-Unterschriften kommen im selben Feld wie v2 an (bioAttestation,
// Praefix "v3:"). Dieser Knoten nimmt sie heraus, bevor er die Anfrage an den
// Proof-Server weiterreicht (proveAnfrageOhneV3), der nur v1/v2 kennt.
//
// Schluessel stehen in der Genesis, nicht in einer Umgebungsvariablen: alle
// Knoten muessen dieselbe Antwort geben. Eine neue Liste ist eine neue
// Genesis-Fassung mit eigenem Stichtag -- dasselbe Verfahren wie bei jeder
// anderen Konsensregel hier.
//
// # SCHLAFEND
//
// Ohne ab_unix (oder 0) gilt nichts davon. Die Annahme verlangt die
// Bescheinigung schon einen Tag vor dem Stichtag (bescheinigungAnnahmeVorlauf):
// was danach im Ausgang liegt, traegt sie sicher, und kein Erzeuger steht am
// Stichtag mit einer Registrierung da, die er in keinen Block mehr legen darf.

// BioBescheinigung reist in register_human mit.
type BioBescheinigung struct {
	Zeit       int64    `json:"zeit"`
	Modus      string   `json:"modus"`
	Signaturen []string `json:"sig"`
}

const (
	bescheinigungHoechstensAlt   = 3600
	bescheinigungHoechstensVoran = 60
	bescheinigungAnnahmeVorlauf  = 86400
	bescheinigungMaxSignaturen   = 17 // eine Bindung + hoechstens 16 Bezeuger, wie der Proof-Server
	bescheinigungMaxSchluessel   = 64
)

const (
	bescheinigungDomaeneBindung = "aequitas-bio-attestation-v3"
	bescheinigungDomaeneAussage = "aequitas-bio-personhood-v3"
	// bescheinigungV3Praefix markiert v3-Unterschriften im Feld
	// bioAttestation der /prove-Anfrage.
	bescheinigungV3Praefix = "v3:"
)

var bnFeld, _ = new(big.Int).SetString("21888242871839275222246405745257275088548364400416034343698204186575808495617", 10)

var nullifierDezimal = regexp.MustCompile(`^[0-9]{1,78}$`)

type bescheinigungKonfig struct {
	AbUnix      int64    `json:"ab_unix"`
	Modus       string   `json:"modus"`
	Quorum      int      `json:"quorum"`
	Coordinator []string `json:"coordinator"`
	Bezeuger    []string `json:"bezeuger"`
}

var (
	bescheinigungEinmal  sync.Once
	bescheinigungGelesen bescheinigungKonfig
	bescheinigungFehler  error

	bescheinigungTestMu sync.RWMutex
	bescheinigungTest   *bescheinigungKonfig
)

// bescheinigungAbOverride: nur fuer Tests (0 = Genesis gilt).
var bescheinigungAbOverride atomic.Int64

// ladeBescheinigungKonfig liest "bescheinigung" aus dem Inhalt einer
// genesis.json und prueft sie. Fehlt der Eintrag: schlafend.
func ladeBescheinigungKonfig(data []byte) (bescheinigungKonfig, error) {
	var roh map[string]json.RawMessage
	if err := json.Unmarshal(data, &roh); err != nil {
		return bescheinigungKonfig{}, fmt.Errorf("genesis.json unlesbar: %w", err)
	}
	// Ein Tippfehler im Namen (etwa "Bescheinigung" oder "bescheinigungen")
	// liesse die Regel still schlafen -- dieser Knoten naehme dann
	// Registrierungen ohne Bescheinigung an, die alle anderen abweisen
	// (Sicherheitspruefung des Diffs, F4). Nur der genaue Name gilt.
	for name := range roh {
		if name != "bescheinigung" && strings.HasPrefix(strings.ToLower(name), "bescheinigung") {
			return bescheinigungKonfig{}, fmt.Errorf("genesis.json: Eintrag %q -- gemeint ist \"bescheinigung\"", name)
		}
	}
	teil, da := roh["bescheinigung"]
	if !da {
		return bescheinigungKonfig{}, nil
	}
	tdec := json.NewDecoder(bytes.NewReader(teil))
	tdec.DisallowUnknownFields()
	var k bescheinigungKonfig
	if err := tdec.Decode(&k); err != nil {
		return bescheinigungKonfig{}, fmt.Errorf("bescheinigung: %w", err)
	}
	return k, k.pruefen()
}

func (k bescheinigungKonfig) pruefen() error {
	if k.AbUnix < 0 {
		return errors.New("bescheinigung.ab_unix negativ")
	}
	if k.AbUnix == 0 {
		return nil // schlafend; die uebrigen Felder duerfen vorbereitet sein
	}
	if k.Modus != "test" && k.Modus != "real" {
		return fmt.Errorf("bescheinigung.modus muss \"test\" oder \"real\" sein, nicht %q", k.Modus)
	}
	if len(k.Coordinator) == 0 {
		return errors.New("bescheinigung.coordinator: mindestens ein Schluessel")
	}
	if len(k.Coordinator)+len(k.Bezeuger) > bescheinigungMaxSchluessel {
		return fmt.Errorf("bescheinigung: hoechstens %d Schluessel", bescheinigungMaxSchluessel)
	}
	alle := map[string]bool{}
	for _, liste := range [][]string{k.Coordinator, k.Bezeuger} {
		for _, s := range liste {
			if !ed25519HexTauglich(s) {
				return fmt.Errorf("bescheinigung: Schluessel %.12s... ist kein tauglicher Ed25519-Schluessel (64 Hex klein, nicht kleiner Ordnung)", s)
			}
			alle[s] = true
		}
	}
	if k.Quorum < 1 || k.Quorum > len(alle) {
		return fmt.Errorf("bescheinigung.quorum=%d, es gibt %d verschiedene Schluessel", k.Quorum, len(alle))
	}
	// Mehr verschiedene Schluessel, als eine Bescheinigung Unterschriften
	// tragen darf, kann keine je erreichen -- jede Registrierung waere ab
	// dem Stichtag tot (Sicherheitspruefung des Diffs, F-B).
	if k.Quorum > bescheinigungMaxSignaturen {
		return fmt.Errorf("bescheinigung.quorum=%d, eine Bescheinigung traegt hoechstens %d Unterschriften", k.Quorum, bescheinigungMaxSignaturen)
	}
	return nil
}

func bescheinigungKonf() (bescheinigungKonfig, error) {
	bescheinigungTestMu.RLock()
	t := bescheinigungTest
	bescheinigungTestMu.RUnlock()
	if t != nil {
		return *t, t.pruefen()
	}
	bescheinigungEinmal.Do(func() {
		data, err := os.ReadFile("genesis.json")
		if err != nil {
			return // keine genesis.json: schlafend, wie der Registervertrag (V7)
		}
		bescheinigungGelesen, bescheinigungFehler = ladeBescheinigungKonfig(data)
	})
	return bescheinigungGelesen, bescheinigungFehler
}

// PruefeBescheinigungGenesis beim Start (cmd/aequitasd): ein Fehler haelt an.
// Auch eine fehlende genesis.json (Sicherheitspruefung des Diffs, F4): ohne
// sie schliefe die Regel still, und der Knoten naehme Registrierungen an, die
// alle anderen abweisen. Tests laufen ohne genesis.json im Paketverzeichnis
// und rufen diese Startpruefung nicht.
func PruefeBescheinigungGenesis() error {
	if _, err := os.Stat("genesis.json"); err != nil {
		return fmt.Errorf("genesis.json nicht lesbar (%w) -- ohne sie gilt die Bescheinigungsregel nicht", err)
	}
	_, err := bescheinigungKonf()
	return err
}

// bescheinigungFingerabdruck: SHA-256 der gelesenen Konfiguration (kanonisch
// als JSON der Struktur, nicht der Dateibytes -- Leerzeichen in genesis.json
// aendern ihn nicht). Zwei Knoten mit verschiedenen Schluessellisten zeigen
// in /api/status verschiedene Werte, nicht nur verschiedene Anzahlen.
func bescheinigungFingerabdruck(k bescheinigungKonfig) string {
	roh, err := json.Marshal(k)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(roh)
	return hex.EncodeToString(h[:])
}

func bescheinigungAb() int64 {
	if o := bescheinigungAbOverride.Load(); o != 0 {
		return o
	}
	k, err := bescheinigungKonf()
	if err != nil {
		return 1 // unlesbare Konfiguration: streng (der Start haelt ohnehin an)
	}
	return k.AbUnix
}

// bescheinigungAktiv: Konsensregel fuer einen Block dieser Zeit.
func bescheinigungAktiv(blockZeit int64) bool {
	ab := bescheinigungAb()
	return ab > 0 && blockZeit >= ab
}

// bescheinigungBeiAnnahme: die Annahme verlangt sie schon einen Tag vorher.
func bescheinigungBeiAnnahme(jetzt int64) bool {
	ab := bescheinigungAb()
	return ab > 0 && jetzt >= ab-bescheinigungAnnahmeVorlauf
}

// BescheinigungStand fuer /api/status.
func BescheinigungStand() map[string]any {
	k, err := bescheinigungKonf()
	if err != nil {
		return map[string]any{"fehler": err.Error()}
	}
	return map[string]any{"ab_unix": k.AbUnix, "modus": k.Modus, "quorum": k.Quorum,
		"coordinator": len(k.Coordinator), "bezeuger": len(k.Bezeuger),
		"fingerabdruck": bescheinigungFingerabdruck(k)}
}

// bindungNachrichtV3 und aussageNachrichtV3: byte-gleich zu
// signing_message_v3 (coordinator/app/bio_attestation.py) und
// personhood_message_v3 (matching-service/app/attest_sign.py).
func bindungNachrichtV3(modus, nullifier, wallet string, zeit int64) []byte {
	return []byte(bescheinigungDomaeneBindung + "|" + modus + "|" + nullifier + "|" + wallet + "|" + strconv.FormatInt(zeit, 10))
}

func aussageNachrichtV3(modus, nullifier string, zeit int64) []byte {
	return []byte(bescheinigungDomaeneAussage + "|" + modus + "|" + nullifier + "|" + strconv.FormatInt(zeit, 10))
}

// pruefeBioBescheinigung: die Bescheinigung einer register_human gegen
// Beweis, Wallet und Schluessel der Genesis. Fehler = Block ungueltig.
func pruefeBioBescheinigung(tx Transaction) error {
	k, err := bescheinigungKonf()
	if err != nil {
		return err
	}
	b := tx.BioBescheinigung
	if b == nil {
		return errors.New("Registrierung ohne Bescheinigung")
	}
	if len(tx.PubSignals) < 2 {
		return errors.New("bescheinigung: Beweis ohne Nullifier")
	}
	roh := strings.TrimSpace(tx.PubSignals[1])
	if !nullifierDezimal.MatchString(roh) {
		return errors.New("bescheinigung: pubSignals[1] ist keine Dezimalzahl")
	}
	n, _ := new(big.Int).SetString(roh, 10)
	if n.Cmp(bnFeld) >= 0 {
		return errors.New("bescheinigung: Nullifier liegt nicht im Feld")
	}
	// Kanonisch (ohne fuehrende Nullen), wie die Unterzeichner ihn schreiben.
	nullifier := n.String()
	if b.Modus != k.Modus {
		return fmt.Errorf("bescheinigung: Modus %q, das Netz verlangt %q", b.Modus, k.Modus)
	}
	if tx.RegAt <= 0 {
		return errors.New("bescheinigung: Registrierung ohne Annahmezeit")
	}
	if d := tx.RegAt - b.Zeit; d > bescheinigungHoechstensAlt || d < -bescheinigungHoechstensVoran {
		return fmt.Errorf("bescheinigung: ausgestellt %d, angenommen %d", b.Zeit, tx.RegAt)
	}
	if len(b.Signaturen) == 0 || len(b.Signaturen) > bescheinigungMaxSignaturen {
		return fmt.Errorf("bescheinigung: %d Unterschriften (1..%d)", len(b.Signaturen), bescheinigungMaxSignaturen)
	}
	wallet := strings.ToLower(strings.TrimSpace(tx.Wallet))
	bindungMsg := bindungNachrichtV3(b.Modus, nullifier, wallet, b.Zeit)
	aussageMsg := aussageNachrichtV3(b.Modus, nullifier, b.Zeit)
	// Die Schluessel einmal je Pruefung dekodieren. Ihre Tauglichkeit (64 Hex
	// klein, keine kleine Ordnung) hat bescheinigungKonf schon geprueft --
	// beim Laden der Genesis (ladeBescheinigungKonfig -> pruefen) bzw. bei
	// jeder Test-Konfiguration; mit Fehler kaeme k hier nicht an. Sie je
	// (Unterschrift, Schluessel) neu zu rechnen, kostete bei 17 Unterschriften
	// und 64 Schluesseln ueber 2.000 Skalarmultiplikationen je Registrierung.
	pubs := map[string]ed25519.PublicKey{}
	for _, liste := range [][]string{k.Coordinator, k.Bezeuger} {
		for _, h := range liste {
			roh, err := hex.DecodeString(h)
			if err != nil || len(roh) != ed25519.PublicKeySize {
				return fmt.Errorf("bescheinigung: Schluessel %.12s... unlesbar", h)
			}
			pubs[h] = ed25519.PublicKey(roh)
		}
	}
	passtZu := func(h, sig string, msg []byte) bool {
		if len(sig) != 2*ed25519.SignatureSize || !kleinHex(sig) {
			return false
		}
		roh, err := hex.DecodeString(sig)
		return err == nil && ed25519.Verify(pubs[h], msg, roh)
	}
	bezeuger := k.Bezeuger
	getrennt := len(bezeuger) > 0
	if !getrennt {
		bezeuger = k.Coordinator
	}
	gesehen := map[string]bool{}
	bindet := map[string]bool{}
	bezeugt := map[string]bool{}
	for _, sig := range b.Signaturen {
		if gesehen[sig] {
			return errors.New("bescheinigung: doppelte Unterschrift")
		}
		gesehen[sig] = true
		passt := false
		for _, s := range k.Coordinator {
			if passtZu(s, sig, bindungMsg) {
				bindet[s], bezeugt[s], passt = true, true, true
				break
			}
		}
		if !passt {
			for _, s := range bezeuger {
				if passtZu(s, sig, aussageMsg) {
					bezeugt[s], passt = true, true
					break
				}
			}
		}
		if !passt {
			return errors.New("bescheinigung: eine Unterschrift passt zu keinem Schluessel des Netzes")
		}
	}
	if len(bindet) == 0 {
		return errors.New("bescheinigung: keine Unterschrift bindet sie an diese Wallet")
	}
	if getrennt {
		ohneBindung := 0
		for s := range bezeugt {
			if !bindet[s] {
				ohneBindung++
			}
		}
		if ohneBindung == 0 {
			return errors.New("bescheinigung: kein Vergleichsdienst hat den Menschen bezeugt")
		}
	}
	if len(bezeugt) < k.Quorum {
		return fmt.Errorf("bescheinigung: %d von %d noetigen Schluesseln", len(bezeugt), k.Quorum)
	}
	return nil
}

// bescheinigungAusProveAnfrage liest die v3-Unterschriften ("v3:..." in
// bioAttestation) und den Zeitpunkt aus dem Rumpf eines /prove -- jeden
// Schluessel genau so geschrieben und genau einmal (wie eindeutigeWallet: Go
// ordnet Schluessel ohne Ruecksicht auf Gross-/Kleinschreibung zu, der
// Proof-Server nicht). `bio` wird nicht gelesen und nicht gemerkt. Was nicht
// eindeutig lesbar ist, ergibt keine Bescheinigung -- die Registrierung
// scheitert dann spaeter (die umkehrbare Richtung).
func bescheinigungAusProveAnfrage(body []byte) *BioBescheinigung {
	werte, ok := eindeutigeFelder(body, "bioAttestation", "bioAttestationIssuedAt")
	if !ok {
		return nil
	}
	var sigs string
	var zeit json.Number
	if json.Unmarshal(werte["bioAttestation"], &sigs) != nil {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(werte["bioAttestationIssuedAt"]))
	dec.UseNumber()
	if dec.Decode(&zeit) != nil {
		var s string
		if json.Unmarshal(werte["bioAttestationIssuedAt"], &s) != nil {
			return nil
		}
		zeit = json.Number(s)
	}
	z, err := strconv.ParseInt(string(zeit), 10, 64)
	if err != nil || z <= 0 {
		return nil
	}
	_, v3 := teileBescheinigungsfeld(sigs)
	var liste []string
	for _, teil := range v3 {
		if t := ed25519SigNormal(teil); t != "" {
			liste = append(liste, t)
		}
	}
	if len(liste) == 0 || len(liste) > bescheinigungMaxSignaturen {
		return nil
	}
	return &BioBescheinigung{Zeit: z, Signaturen: liste}
}

// teileBescheinigungsfeld: die Eintraege von bioAttestation in die fuer den
// Proof-Server (ohne Praefix) und die v3-Eintraege (Praefix entfernt).
func teileBescheinigungsfeld(sigs string) (proofServer, v3 []string) {
	for _, teil := range strings.Split(sigs, ",") {
		t := strings.TrimSpace(teil)
		switch {
		case t == "":
		case strings.HasPrefix(t, bescheinigungV3Praefix):
			v3 = append(v3, strings.TrimPrefix(t, bescheinigungV3Praefix))
		default:
			proofServer = append(proofServer, t)
		}
	}
	return proofServer, v3
}

// proveAnfrageOhneV3: der Rumpf, wie er an den Proof-Server geht -- ohne die
// v3-Eintraege in bioAttestation, die er nicht kennt (er wiese sonst die
// ganze Anfrage ab, weil jede Unterschrift passen muss). Alles andere bleibt
// Byte fuer Byte erhalten. Ist bioAttestation nicht eindeutig lesbar oder
// traegt es keinen v3-Eintrag, geht der Rumpf unveraendert weiter -- der
// Proof-Server entscheidet dann wie bisher (fail-closed).
func proveAnfrageOhneV3(body []byte) []byte {
	werte, ok := eindeutigeFelder(body, "bioAttestation")
	if !ok {
		return body
	}
	var sigs string
	if json.Unmarshal(werte["bioAttestation"], &sigs) != nil {
		return body
	}
	rest, v3 := teileBescheinigungsfeld(sigs)
	if len(v3) == 0 {
		return body
	}
	neu, err := json.Marshal(strings.Join(rest, ","))
	if err != nil {
		return body
	}
	// Den Wert an seiner Stelle ersetzen: eindeutigeFelder hat sichergestellt,
	// dass der Schluessel genau einmal und genau so geschrieben vorkommt.
	dec := json.NewDecoder(bytes.NewReader(body))
	if _, err := dec.Token(); err != nil {
		return body
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return body
		}
		vorWert := dec.InputOffset()
		var wert json.RawMessage
		if err := dec.Decode(&wert); err != nil {
			return body
		}
		if t == "bioAttestation" {
			nachWert := dec.InputOffset()
			// Leerraum zwischen ':' und Wert ueberspringen.
			anfang := int(vorWert)
			for anfang < len(body) && body[anfang] != '"' {
				anfang++
			}
			aus := make([]byte, 0, len(body))
			aus = append(aus, body[:anfang]...)
			aus = append(aus, neu...)
			aus = append(aus, body[nachWert:]...)
			return aus
		}
	}
	return body
}

// recoveryBescheinigungPruefen: darf eine wiederholte Registrierung
// (RetryRegistrationRecoveries) ab dem Stichtag noch eingebucht werden?
// Nur mit der Nullifier-Form (die alte ohne Nullifier kennt keine
// Bescheinigung) und einer Bescheinigung, die jeder Knoten annimmt.
func recoveryBescheinigungPruefen(tx Transaction) error {
	if tx.Nullifier == "" {
		return fmt.Errorf("Registrierung ohne Nullifier: nach dem Stichtag der Bescheinigung nicht nachholbar")
	}
	if tx.BioBescheinigung == nil {
		return fmt.Errorf("Registrierung ohne Bescheinigung: nach dem Stichtag nicht nachholbar -- neu registrieren")
	}
	return pruefeBioBescheinigung(tx)
}
