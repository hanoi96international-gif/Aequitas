package keeper

import (
	"bytes"
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

	"github.com/iden3/go-iden3-crypto/poseidon"
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
// Die Transaktion traegt die Bescheinigung: `bio` (die Zufallszahl des
// Coordinators -- kein biometrisches Merkmal), ihren Zeitpunkt, den
// Betriebsmodus und die Ed25519-Unterschriften. Jeder Knoten prueft selbst:
//
//   - Poseidon(bio) ist der Nullifier des Beweises (pubSignals[1]). Poseidon
//     wie circomlib, gegen den Circuit des Proof-Servers mit Testvektoren
//     belegt (TestBescheinigung_PoseidonWieCircuit).
//   - Jede Unterschrift passt zu einem Schluessel aus der Genesis und zu
//     genau der Nachricht, die auch der Proof-Server nachbildet
//     (bio_attestation.js, Form v2):
//       Bindung:  aequitas-bio-attestation-v2|<modus>|<bio>|<wallet>|<zeit>
//                 nur Coordinator-Schluessel
//       Aussage:  aequitas-bio-personhood-v2|<modus>|<bio>|<zeit>
//                 Bezeuger-Schluessel (ohne eigene Liste: Coordinator)
//   - Mindestens eine Bindung an DIESE Wallet; bei getrennten Listen
//     mindestens ein Bezeuger, der nicht gebunden hat; verschiedene
//     Schluessel >= quorum. Gezaehlt werden Schluessel, nie Signaturen.
//   - Der Zeitpunkt liegt hoechstens bescheinigungHoechstensAlt vor der
//     Annahme (RegAt) und hoechstens 60 s danach.
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
	Bio        string   `json:"bio"`
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
	bescheinigungDomaeneBindung = "aequitas-bio-attestation-v2"
	bescheinigungDomaeneAussage = "aequitas-bio-personhood-v2"
)

var bnFeld, _ = new(big.Int).SetString("21888242871839275222246405745257275088548364400416034343698204186575808495617", 10)

var bioDezimal = regexp.MustCompile(`^[0-9]{1,78}$`)

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
func PruefeBescheinigungGenesis() error {
	_, err := bescheinigungKonf()
	return err
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
		"coordinator": len(k.Coordinator), "bezeuger": len(k.Bezeuger)}
}

// poseidonNullifier: Poseidon(bio) wie circomlib (biometric_v3.circom).
func poseidonNullifier(bio *big.Int) (*big.Int, error) {
	return poseidon.Hash([]*big.Int{bio})
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
	if !bioDezimal.MatchString(b.Bio) || (len(b.Bio) > 1 && b.Bio[0] == '0') {
		return errors.New("bescheinigung: bio ist keine kanonische Dezimalzahl")
	}
	bio, _ := new(big.Int).SetString(b.Bio, 10)
	if bio.Cmp(bnFeld) >= 0 {
		return errors.New("bescheinigung: bio liegt nicht im Feld")
	}
	if len(tx.PubSignals) < 2 {
		return errors.New("bescheinigung: Beweis ohne Nullifier")
	}
	nullifier, ok := new(big.Int).SetString(strings.TrimSpace(tx.PubSignals[1]), 10)
	if !ok {
		return errors.New("bescheinigung: pubSignals[1] unlesbar")
	}
	erwartet, err := poseidonNullifier(bio)
	if err != nil {
		return fmt.Errorf("bescheinigung: Poseidon: %w", err)
	}
	if erwartet.Cmp(nullifier) != 0 {
		return errors.New("bescheinigung: Poseidon(bio) ist nicht der Nullifier des Beweises")
	}
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
	zeit := strconv.FormatInt(b.Zeit, 10)
	bindungMsg := []byte(bescheinigungDomaeneBindung + "|" + b.Modus + "|" + b.Bio + "|" + wallet + "|" + zeit)
	aussageMsg := []byte(bescheinigungDomaeneAussage + "|" + b.Modus + "|" + b.Bio + "|" + zeit)
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
			if ed25519PruefenStreng(s, sig, bindungMsg) {
				bindet[s], bezeugt[s], passt = true, true, true
				break
			}
		}
		if !passt {
			for _, s := range bezeuger {
				if ed25519PruefenStreng(s, sig, aussageMsg) {
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

// bescheinigungAusProveAnfrage liest bio, Unterschriften und Zeitpunkt aus
// dem Rumpf eines /prove -- jeden Schluessel genau so geschrieben und genau
// einmal (wie eindeutigeWallet: Go ordnet Schluessel ohne Ruecksicht auf
// Gross-/Kleinschreibung zu, der Proof-Server nicht). Was nicht eindeutig
// lesbar ist, ergibt keine Bescheinigung -- die Registrierung scheitert dann
// spaeter (die umkehrbare Richtung).
func bescheinigungAusProveAnfrage(body []byte) *BioBescheinigung {
	werte, ok := eindeutigeFelder(body, "bio", "bioAttestation", "bioAttestationIssuedAt")
	if !ok {
		return nil
	}
	var bio, sigs string
	var zeit json.Number
	if json.Unmarshal(werte["bio"], &bio) != nil || json.Unmarshal(werte["bioAttestation"], &sigs) != nil {
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
	var liste []string
	for _, teil := range strings.Split(sigs, ",") {
		if t := ed25519SigNormal(teil); t != "" {
			liste = append(liste, t)
		}
	}
	if len(liste) == 0 || len(liste) > bescheinigungMaxSignaturen || !bioDezimal.MatchString(strings.TrimSpace(bio)) {
		return nil
	}
	return &BioBescheinigung{Bio: strings.TrimSpace(bio), Zeit: z, Signaturen: liste}
}
