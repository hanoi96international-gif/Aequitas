package keeper

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
)

// VOM KOMITEE UNTERSCHRIEBENER CHECKPOINT (OFFEN 10, Stufe C, Baustein 1).
//
// Warum: blue_score und selected_parent sind nicht konsensfest, und ein
// einzelner Erzeuger kann Hoehe und Blue-Score aufblaehen (docs/OFFEN.md,
// Punkt 10). Ein Bezugspunkt, den kein einzelner Validator steuern kann, ist
// ein Block, den mehr als 2/3 des Komitees zur Zeit des Blocks unterschrieben
// haben. Das Komitee kommt aus dem Register in der Kette
// (komitee_register.go). Ein Zweig nur aus eigenen Bloecken bekommt ohne
// Mehrheit keine Unterschriften.
//
// Dieser Baustein: Format, Unterschreiben und Pruefung eines Zertifikats
// gegen eine gegebene Komitee-Menge. Reine Funktionen, nirgends
// angeschlossen -- schlafend.
//
// Unterschrieben wird: Domaene, Chain-ID, Hoehe, Blockhash, Blockzeit,
// Epoche und der Hash der Komitee-Menge. Ein Zertifikat gilt so fuer genau
// eine Menge: urteilen zwei Knoten mit verschiedenen Sichten auf das
// Komitee, faellt das auf (Komitee-Hash passt nicht), statt still
// verschieden zu urteilen.
//
// Geprueft wird fail-closed und begrenzt, erst billig, dann teuer:
//   - Format: Felder gueltig, Epoche passt zur Zeit, Komitee-Hash passt zur
//     uebergebenen Menge, das Komitee hat mindestens checkpointMindestKomitee
//     Mitglieder (sonst ist ein einzelner Schluessel das Quorum).
//   - Unterschriften: hoechstens so viele wie Mitglieder (und
//     targetCommitteeSize), mindestens das Quorum, streng nach Absender
//     sortiert (damit keiner doppelt und die Bytes eindeutig), jeder Absender
//     Mitglied, kanonisch geschrieben (Absender 0x + 40, Unterschrift 130
//     Hex-Zeichen, klein, low-s, v 0/1) -- alles vor der ersten Recovery.
//   - Dann je Unterschrift die Recovery; sie muss zum Absender passen.
//
// OFFEN vor dem Anschliessen (Pruefung dieses Bausteins, HOCH): Das Komitee
// zur Zeit des Checkpoints muss jeder Knoten gleich bestimmen (heute liest
// der geschlossene Betrieb die eigene Liste), und die Zeit darf der
// Einreicher nicht waehlen: Hash, Hoehe und Zeit muessen aus einem lokal
// vorhandenen Block kommen, die Zeit nahe der Einreichung und streng
// monoton gegenueber dem letzten Checkpoint liegen.

const (
	checkpointDomaene = "Aequitas Checkpoint v1"
	// checkpointChainID: dieselbe Kette wie chainIDHex (evm_rpc.go).
	checkpointChainID int64 = 1926
	// checkpointMindestKomitee: kleinere Komitees bestaetigen nichts.
	checkpointMindestKomitee = 4
)

// secpHalbeOrdnung: n/2 -- groessere s sind die zweite Schreibweise
// derselben Unterschrift.
var secpHalbeOrdnung = new(big.Int).Rsh(crypto.S256().Params().N, 1)

// KettenCheckpoint: der Block, den das Komitee bestaetigt, und das Komitee.
type KettenCheckpoint struct {
	Hoehe   int64  `json:"hoehe"`
	Hash    string `json:"hash"`
	Zeit    int64  `json:"zeit"`
	Epoche  int64  `json:"epoche"`
	Komitee string `json:"komitee"`
}

// CheckpointUnterschrift: eine Unterschrift eines Komiteemitglieds.
type CheckpointUnterschrift struct {
	Von string `json:"von"`
	Sig string `json:"sig"`
}

// CheckpointZertifikat: Checkpoint plus Unterschriften.
type CheckpointZertifikat struct {
	KettenCheckpoint
	Unterschriften []CheckpointUnterschrift `json:"unterschriften"`
}

// istKleinHex: genau n Hex-Zeichen, klein.
func istKleinHex(h string, n int) bool {
	if len(h) != n {
		return false
	}
	for _, c := range h {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func istKanonischeAdresse(a string) bool {
	return strings.HasPrefix(a, "0x") && istKleinHex(a[2:], 40)
}

// komiteeHash: sha256 ueber die sortierten Mitglieder.
func komiteeHash(komitee map[string]bool) string {
	m := make([]string, 0, len(komitee))
	for a := range komitee {
		m = append(m, a)
	}
	sort.Strings(m)
	h := sha256.Sum256([]byte(strings.Join(m, ",")))
	return hex.EncodeToString(h[:])
}

func (c KettenCheckpoint) gueltig() error {
	if c.Hoehe <= 0 || c.Zeit <= 0 {
		return fmt.Errorf("Hoehe %d / Zeit %d ungueltig", c.Hoehe, c.Zeit)
	}
	if !istKleinHex(c.Hash, 64) {
		return fmt.Errorf("Hash %q ist kein Blockhash", kurzAdresse(c.Hash))
	}
	if c.Epoche != c.Zeit/epochLength {
		return fmt.Errorf("Epoche %d passt nicht zur Zeit %d", c.Epoche, c.Zeit)
	}
	if !istKleinHex(c.Komitee, 64) {
		return fmt.Errorf("Komitee-Hash ungueltig")
	}
	return nil
}

// signierHash: was jedes Mitglied unterschreibt.
func (c KettenCheckpoint) signierHash() []byte { return c.signierHashFuer(checkpointChainID) }

func (c KettenCheckpoint) signierHashFuer(chainID int64) []byte {
	// Slot-Laenge und Slot stehen im Inhalt: eine spaetere Aenderung der
	// Slot-Laenge macht alte Unterschriften nicht rueckwirkend zu
	// "Doppelsignaturen" (Pruefung von Baustein 5).
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d|%s|%d|%d|%s|slot=%d/%d",
		checkpointDomaene, chainID, c.Hoehe, c.Hash, c.Zeit, c.Epoche, c.Komitee,
		c.Zeit/checkpointSlotLaenge, checkpointSlotLaenge)))
	return h[:]
}

// neuerCheckpoint: der Checkpoint fuer einen Block und das Komitee zu
// seiner Zeit.
func neuerCheckpoint(hoehe int64, hash string, zeit int64, komitee map[string]bool) KettenCheckpoint {
	return KettenCheckpoint{Hoehe: hoehe, Hash: hash, Zeit: zeit, Epoche: zeit / epochLength, Komitee: komiteeHash(komitee)}
}

// checkpointUnterschreiben: die Unterschrift dieses Schluessels.
func checkpointUnterschreiben(c KettenCheckpoint, key *ecdsa.PrivateKey) (CheckpointUnterschrift, error) {
	if err := c.gueltig(); err != nil {
		return CheckpointUnterschrift{}, err
	}
	sig, err := crypto.Sign(c.signierHash(), key)
	if err != nil {
		return CheckpointUnterschrift{}, err
	}
	return CheckpointUnterschrift{
		Von: strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex()),
		Sig: hex.EncodeToString(sig),
	}, nil
}

// zertifikatOrdnen: Unterschriften nach Absender sortiert (so verlangt es
// pruefeZertifikat).
func zertifikatOrdnen(z *CheckpointZertifikat) {
	sort.Slice(z.Unterschriften, func(i, j int) bool { return z.Unterschriften[i].Von < z.Unterschriften[j].Von })
}

// checkpointQuorum: mehr als 2/3 von n.
func checkpointQuorum(n int) int { return 2*n/3 + 1 }

// kanonischeSig: 65 Bytes, low-s, v 0/1 -- sonst gaebe es zu einer
// Unterschrift weitere Schreibweisen.
func kanonischeSig(s string) ([]byte, bool) {
	if !istKleinHex(s, 130) {
		return nil, false
	}
	sig, err := hex.DecodeString(s)
	if err != nil || sig[64] > 1 {
		return nil, false
	}
	if new(big.Int).SetBytes(sig[32:64]).Cmp(secpHalbeOrdnung) > 0 {
		return nil, false
	}
	return sig, true
}

// pruefeZertifikat: nil, wenn mehr als 2/3 von komitee (klein geschriebene
// Signieradressen) den Checkpoint kanonisch unterschrieben haben.
func pruefeZertifikat(z CheckpointZertifikat, komitee map[string]bool) error {
	// 1. Billig: Format, Komitee, Zahl, Ordnung, Mitgliedschaft.
	if err := z.gueltig(); err != nil {
		return err
	}
	if len(komitee) < checkpointMindestKomitee {
		return fmt.Errorf("Komitee mit %d Mitgliedern bestaetigt nichts (mindestens %d)", len(komitee), checkpointMindestKomitee)
	}
	if z.Komitee != komiteeHash(komitee) {
		return fmt.Errorf("Komitee-Hash passt nicht zur eigenen Sicht")
	}
	n := len(z.Unterschriften)
	if n > targetCommitteeSize || n > len(komitee) {
		return fmt.Errorf("%d Unterschriften bei %d Mitgliedern", n, len(komitee))
	}
	if q := checkpointQuorum(len(komitee)); n < q {
		return fmt.Errorf("%d von %d Unterschriften, noetig %d", n, len(komitee), q)
	}
	sigs := make([][]byte, n)
	for i, u := range z.Unterschriften {
		if !istKanonischeAdresse(u.Von) {
			return fmt.Errorf("Absender %q nicht kanonisch", kurzAdresse(u.Von))
		}
		if i > 0 && u.Von <= z.Unterschriften[i-1].Von {
			return fmt.Errorf("Unterschriften nicht streng nach Absender sortiert (doppelt?)")
		}
		if !komitee[u.Von] {
			return fmt.Errorf("%s ist nicht im Komitee", kurzAdresse(u.Von))
		}
		sig, ok := kanonischeSig(u.Sig)
		if !ok {
			return fmt.Errorf("Unterschrift von %s nicht kanonisch", kurzAdresse(u.Von))
		}
		sigs[i] = sig
	}
	// 2. Teuer: je Unterschrift eine Recovery.
	h := z.signierHash()
	for i, u := range z.Unterschriften {
		pub, err := crypto.SigToPub(h, sigs[i])
		if err != nil {
			return fmt.Errorf("Unterschrift von %s nicht pruefbar", kurzAdresse(u.Von))
		}
		if strings.ToLower(crypto.PubkeyToAddress(*pub).Hex()) != u.Von {
			return fmt.Errorf("Unterschrift passt nicht zu %s", kurzAdresse(u.Von))
		}
	}
	return nil
}
