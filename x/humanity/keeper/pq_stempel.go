package keeper

// QUANTENSICHERHEIT STUFE 5: DER PQ-STEMPEL (docs/QUANTENSICHERHEIT.md).
//
// # DIE LUECKE
//
// Jeder Block traegt eine secp256k1-Unterschrift seines Erzeugers. Wer mit
// einem Quantenrechner aus der oeffentlichen Erzeugeradresse den Schluessel
// ableitet (Shor), unterschreibt Bloecke als dieser Erzeuger.
//
// # DER STEMPEL
//
// Auf jeder pqStempelAbstand-ten Hoehe (60: bei 1 Block/s einmal je Minute)
// unterschreibt der Erzeuger zusaetzlich mit ML-DSA-65 (FIPS 204) ueber
// "aequitas-pq-stempel-v1|chain:1926|" + Blockhash. Der Hash deckt die
// Eltern und damit die ganze Geschichte dahinter. Ab der Stichhoehe
// (genesis.json, pq_stempel.ab_hoehe) weist JEDER Knoten selbst einen Block
// auf einer Stempelhoehe ohne gueltigen Stempel ab -- ein Faelscher mit
// gebrochenem secp256k1 kommt hoechstens bis zur naechsten Stempelhoehe,
// nie darueber. Auf allen anderen Hoehen muss das Feld leer sein (haelt
// Bloecke klein: ~110 Byte je Block im Mittel statt 6.600).
//
// # SCHLUESSEL
//
// Der oeffentliche Schluessel je Erzeugeradresse steht in der Genesis
// (pq_stempel.schluessel), erzeugt auf dem Server und dort byte-genau
// abgeglichen (Workflow erzeuger-pq-schluessel-anlegen.yml), wie Stufe 1.
// Der Seed liegt nur auf dem Erzeuger (Umgebung ERZEUGER_MLDSA_SEED) und wird
// nie ausgegeben. Ein Erzeuger ohne eingetragenen Schluessel kann ab dem
// Stichhoehe keinen Block auf einer Stempelhoehe mehr erzeugen, der
// angenommen wird -- fail-closed; neue Erzeuger tragen ihren Schluessel mit
// pruefePQErzeugerBindung ein (checkpoint_pq.go).
//
// # WARUM EINE HOEHE, KEINE ZEIT (Sicherheitspruefung des Diffs, F1)
//
// Die Blockzeit waehlt der Erzeuger; die Kette verlangt nur, dass sie nicht
// viel aelter ist als die seiner Eltern -- sie muss nicht steigen. Ein
// Faelscher mit gebrochenem secp256k1 koennte von einem Block vor einem
// Zeit-Stichtag einen Zweig mit eingefrorener Zeit bauen und nie stempeln.
// Die Hoehe dagegen prueft jeder Knoten selbst (maxElternHoehe + 1): jeder
// Zweig erreicht die Stichhoehe und braucht dort den Stempel.
//
// # VOR DER STICHHOEHE
//
// Nur die Form zaehlt (Laenge, Hex, nur auf Stempelhoehen). Ist der Schluessel
// bekannt, wird trotzdem geprueft und gezaehlt -- abgewiesen wird nichts. So
// zeigt /api/status, dass die Stempel stimmen, BEVOR sie Pflicht werden.

import (
	"bytes"
	"crypto/mldsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	pqStempelAbstand int64 = 60
	pqStempelPraefix       = "aequitas-pq-stempel-v1"
	// pqStempelMaxSchluessel: wie das Komitee (targetCommitteeSize).
	pqStempelMaxSchluessel = targetCommitteeSize
)

var errPQStempel = errors.New("PQ-Stempel abgelehnt")

// pqStempelKonfig: "pq_stempel" in genesis.json.
type pqStempelKonfig struct {
	AbHoehe    int64             `json:"ab_hoehe"`
	Schluessel map[string]string `json:"schluessel,omitempty"` // Erzeugeradresse (0x, klein) -> ML-DSA-65-Schluessel (hex)
}

func ladePQStempelKonfig(data []byte) (pqStempelKonfig, error) {
	var roh map[string]json.RawMessage
	if err := json.Unmarshal(data, &roh); err != nil {
		return pqStempelKonfig{}, fmt.Errorf("genesis.json unlesbar: %w", err)
	}
	for name := range roh {
		// Ein Tippfehler liesse die Regel still schlafen (wie bei der
		// Bescheinigung, F4): nur der genaue Name gilt.
		if name != "pq_stempel" && strings.Contains(strings.ToLower(name), "stempel") {
			return pqStempelKonfig{}, fmt.Errorf("genesis.json: Eintrag %q -- gemeint ist \"pq_stempel\"", name)
		}
	}
	teil, da := roh["pq_stempel"]
	if !da {
		return pqStempelKonfig{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(teil))
	dec.DisallowUnknownFields()
	var k pqStempelKonfig
	if err := dec.Decode(&k); err != nil {
		return pqStempelKonfig{}, fmt.Errorf("pq_stempel: %w", err)
	}
	return k, k.pruefen()
}

func (k pqStempelKonfig) pruefen() error {
	if k.AbHoehe < 0 {
		return errors.New("pq_stempel.ab_hoehe negativ")
	}
	if len(k.Schluessel) > pqStempelMaxSchluessel {
		return fmt.Errorf("pq_stempel.schluessel: hoechstens %d", pqStempelMaxSchluessel)
	}
	for a, h := range k.Schluessel {
		if !istKanonischeAdresse(a) {
			return fmt.Errorf("pq_stempel.schluessel: Adresse %q nicht kanonisch (0x + 40 Hex, klein)", kurzAdresse(a))
		}
		if _, err := pqSchluesselLesen(h); err != nil {
			return fmt.Errorf("pq_stempel.schluessel[%s]: %w", kurzAdresse(a), err)
		}
	}
	if k.AbHoehe > 0 && len(k.Schluessel) == 0 {
		// Ohne Schluessel waere ab der Stichhoehe jede Stempelhoehe tot.
		return errors.New("pq_stempel.ab_hoehe gesetzt, aber kein Schluessel eingetragen")
	}
	return nil
}

var (
	pqStempelEinmal   sync.Once
	pqStempelGelesen  pqStempelKonfig
	pqStempelFehler   error
	pqStempelTestMu   sync.RWMutex
	pqStempelTestKonf *pqStempelKonfig

	// Zaehler fuer /api/status: vor der Stichhoehe gepruefte Stempel.
	pqStempelOK     atomic.Int64
	pqStempelFalsch atomic.Int64
)

func pqStempelKonf() (pqStempelKonfig, error) {
	pqStempelTestMu.RLock()
	t := pqStempelTestKonf
	pqStempelTestMu.RUnlock()
	if t != nil {
		return *t, t.pruefen()
	}
	pqStempelEinmal.Do(func() {
		data, err := os.ReadFile("genesis.json")
		if err != nil {
			return // keine genesis.json: schlafend (Startpruefung haelt den echten Knoten an)
		}
		pqStempelGelesen, pqStempelFehler = ladePQStempelKonfig(data)
	})
	return pqStempelGelesen, pqStempelFehler
}

// PruefePQStempelGenesis beim Start (cmd/aequitasd): ein Fehler haelt an.
func PruefePQStempelGenesis() error {
	_, err := pqStempelKonf()
	return err
}

func pqStempelHoehe(h int64) bool { return h > 0 && h%pqStempelAbstand == 0 }

func pqStempelNachricht(blockHash string) []byte {
	return []byte(fmt.Sprintf("%s|chain:%d|%s", pqStempelPraefix, eingefroreneChainID, strings.ToLower(blockHash)))
}

// pruefePQStempel: jeder Knoten, fuer jeden Block von aussen (AddPeerBlock,
// nach der secp256k1-Pruefung). Erst billig (Form), dann teuer (ML-DSA).
func pruefePQStempel(b *Block) error {
	k, err := pqStempelKonf()
	if err != nil {
		return fmt.Errorf("%w: Konfiguration: %v", errPQStempel, err) // fail-closed
	}
	return pruefePQStempelMit(b, k)
}

func pruefePQStempelMit(b *Block, k pqStempelKonfig) error {
	stempelHoehe := pqStempelHoehe(b.Height)
	if b.PQStempel != "" {
		if !stempelHoehe {
			return fmt.Errorf("%w: Stempel auf Hoehe %d (nur jede %d.)", errPQStempel, b.Height, pqStempelAbstand)
		}
		if len(b.PQStempel) != pqSignaturHex || !kleinHex(b.PQStempel) {
			return fmt.Errorf("%w: Stempel in falscher Form", errPQStempel)
		}
	}
	if !stempelHoehe {
		return nil
	}
	pflicht := k.AbHoehe > 0 && b.Height >= k.AbHoehe
	pkHex, bekannt := k.Schluessel[strings.ToLower(b.Proposer)]
	if !pflicht {
		// Beobachten: pruefen, zaehlen, nichts abweisen.
		if b.PQStempel != "" && bekannt {
			if pqStempelGueltig(b, pkHex) {
				pqStempelOK.Add(1)
			} else {
				pqStempelFalsch.Add(1)
				fmt.Printf("[PQ-STEMPEL] Block #%d: Stempel passt nicht (vor der Stichhoehe, nur gezaehlt)\n", b.Height)
			}
		}
		return nil
	}
	if b.PQStempel == "" {
		return fmt.Errorf("%w: Hoehe %d ohne Stempel", errPQStempel, b.Height)
	}
	if !bekannt {
		return fmt.Errorf("%w: Erzeuger %s hat keinen PQ-Schluessel", errPQStempel, kurzAdresse(b.Proposer))
	}
	if !pqStempelGueltig(b, pkHex) {
		return fmt.Errorf("%w: Stempel von %s ungueltig", errPQStempel, kurzAdresse(b.Proposer))
	}
	return nil
}

func pqStempelGueltig(b *Block, pkHex string) bool {
	pk, err := pqSchluesselLesen(pkHex)
	if err != nil {
		return false
	}
	sig, err := hex.DecodeString(b.PQStempel)
	if err != nil {
		return false
	}
	return mldsa.Verify(pk, pqStempelNachricht(b.Hash), sig, nil) == nil
}

// ---- Erzeuger ----

var (
	pqStempelSchluesselEinmal sync.Once
	pqStempelSchluesselWert   *mldsa.PrivateKey
	pqStempelSchluesselTest   atomic.Pointer[mldsa.PrivateKey]
)

// pqStempelSchluessel: ML-DSA-65 aus ERZEUGER_MLDSA_SEED (64 Hex). Der Wert
// wird nie ausgegeben.
func pqStempelSchluessel() *mldsa.PrivateKey {
	if t := pqStempelSchluesselTest.Load(); t != nil {
		return t
	}
	pqStempelSchluesselEinmal.Do(func() {
		roh := strings.ToLower(strings.TrimSpace(os.Getenv("ERZEUGER_MLDSA_SEED")))
		if roh == "" {
			return
		}
		seed, err := hex.DecodeString(roh)
		if err != nil || len(seed) != 32 {
			fmt.Println("[PQ-STEMPEL] ERZEUGER_MLDSA_SEED ist kein 32-Byte-Seed (64 Hex) -- keine Stempel")
			return
		}
		sk, err := mldsa.NewPrivateKey(mldsa.MLDSA65(), seed)
		if err != nil {
			fmt.Println("[PQ-STEMPEL] ERZEUGER_MLDSA_SEED unbrauchbar -- keine Stempel")
			return
		}
		pqStempelSchluesselWert = sk
	})
	return pqStempelSchluesselWert
}

// pqStempelSetzen: beim Erzeugen, nach der secp256k1-Unterschrift. Ohne
// Schluessel bleibt das Feld leer -- ab der Stichhoehe nimmt dann niemand den
// Block auf einer Stempelhoehe an, darum warnt der Start (PQStempelBereit).
func (dag *BlockDAG) pqStempelSetzen(block *Block) {
	if !pqStempelHoehe(block.Height) {
		return
	}
	sk := pqStempelSchluessel()
	if sk == nil {
		return
	}
	sig, err := sk.SignDeterministic(pqStempelNachricht(block.Hash), nil)
	if err != nil {
		fmt.Printf("[PQ-STEMPEL] Block #%d: nicht unterschrieben: %v\n", block.Height, err)
		return
	}
	block.PQStempel = hex.EncodeToString(sig)
	// Selbstpruefung gegen die Genesis: zeigt in /api/status, dass der Stempel
	// stimmt, den Peers pruefen werden -- auch auf dem einzigen Erzeuger.
	if k, err := pqStempelKonf(); err == nil {
		if pk, ok := k.Schluessel[strings.ToLower(block.Proposer)]; ok {
			if pqStempelGueltig(block, pk) {
				pqStempelOK.Add(1)
			} else {
				pqStempelFalsch.Add(1)
				fmt.Printf("[PQ-STEMPEL] Block #%d: eigener Stempel passt NICHT zum Genesis-Schluessel\n", block.Height)
			}
		}
	}
}

// pqStempelStatus fuer /api/status.
func pqStempelStatus() map[string]interface{} {
	k, err := pqStempelKonf()
	st := map[string]interface{}{
		"ab_hoehe":        k.AbHoehe,
		"abstand":         pqStempelAbstand,
		"schluessel":      len(k.Schluessel),
		"eigener":         pqStempelSchluessel() != nil,
		"geprueft_ok":     pqStempelOK.Load(),
		"geprueft_falsch": pqStempelFalsch.Load(),
	}
	if err != nil {
		st["fehler"] = "Konfiguration ungueltig"
	}
	return st
}
