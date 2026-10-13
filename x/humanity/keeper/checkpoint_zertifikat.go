package keeper

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
// (komitee_register.go) -- jeder Knoten mit demselben Stand prueft gleich,
// ohne lokale GHOSTDAG-Werte. Ein Zweig nur aus eigenen Bloecken bekommt
// ohne Mehrheit keine Unterschriften.
//
// Dieser Baustein: das Format, das Unterschreiben und die Pruefung eines
// Zertifikats gegen eine gegebene Komitee-Menge. Reine Funktionen, nirgends
// angeschlossen -- schlafend. Danach: Unterschriften sammeln (Gossip),
// Zertifikate in die Kette (eigene Transaktionsart), Anker der
// Validatoren-Belohnung (#340) und Finalitaet daran haengen.
//
// Was unterschrieben wird: Domaene, Chain-ID, Hoehe, Blockhash, Blockzeit --
// eine Unterschrift fuer eine andere Kette oder einen anderen Zweck passt
// nie. Geprueft wird fail-closed und begrenzt: hoechstens
// targetCommitteeSize Unterschriften, bevor irgendeine Kryptographie laeuft;
// jede muss zu ihrem Absender passen, der Absender im Komitee sein und darf
// nur einmal zaehlen (ein doppelter Eintrag macht das ganze Zertifikat
// ungueltig); ein leeres Komitee bestaetigt nichts (Quorum von 0 ist 1).

const (
	checkpointDomaene = "Aequitas Checkpoint v1"
	// checkpointChainID: dieselbe Kette wie chainIDHex (evm_rpc.go).
	checkpointChainID int64 = 1926
)

// KettenCheckpoint: der Block, den das Komitee bestaetigt.
type KettenCheckpoint struct {
	Hoehe int64  `json:"hoehe"`
	Hash  string `json:"hash"`
	Zeit  int64  `json:"zeit"`
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

// istHex64: 64 Hex-Zeichen, klein -- so speichert chain_blocks Hashes.
func istHex64(h string) bool {
	if len(h) != 64 {
		return false
	}
	for _, c := range h {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func (c KettenCheckpoint) gueltig() error {
	if c.Hoehe <= 0 || c.Zeit <= 0 {
		return fmt.Errorf("Hoehe %d / Zeit %d ungueltig", c.Hoehe, c.Zeit)
	}
	if !istHex64(c.Hash) {
		return fmt.Errorf("Hash %q ist kein Blockhash", kurzAdresse(c.Hash))
	}
	return nil
}

// signierHash: was jedes Mitglied unterschreibt.
func (c KettenCheckpoint) signierHash() []byte {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d|%s|%d",
		checkpointDomaene, checkpointChainID, c.Hoehe, c.Hash, c.Zeit)))
	return h[:]
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

// checkpointQuorum: mehr als 2/3 von n.
func checkpointQuorum(n int) int { return 2*n/3 + 1 }

// pruefeZertifikat: nil, wenn mehr als 2/3 von komitee (klein geschriebene
// Signieradressen) den Checkpoint unterschrieben haben.
func pruefeZertifikat(z CheckpointZertifikat, komitee map[string]bool) error {
	if err := z.gueltig(); err != nil {
		return err
	}
	if n := len(z.Unterschriften); n > targetCommitteeSize || n > len(komitee) {
		return fmt.Errorf("%d Unterschriften bei %d Mitgliedern", n, len(komitee))
	}
	h := z.signierHash()
	gezaehlt := make(map[string]bool, len(z.Unterschriften))
	for _, u := range z.Unterschriften {
		von := strings.ToLower(strings.TrimSpace(u.Von))
		if gezaehlt[von] {
			return fmt.Errorf("%s unterschreibt doppelt", kurzAdresse(von))
		}
		if !komitee[von] {
			return fmt.Errorf("%s ist nicht im Komitee", kurzAdresse(von))
		}
		sig, err := hex.DecodeString(strings.TrimPrefix(u.Sig, "0x"))
		if err != nil || len(sig) != 65 {
			return fmt.Errorf("Unterschrift von %s unlesbar", kurzAdresse(von))
		}
		pub, err := crypto.SigToPub(h, sig)
		if err != nil {
			return fmt.Errorf("Unterschrift von %s nicht pruefbar", kurzAdresse(von))
		}
		if strings.ToLower(crypto.PubkeyToAddress(*pub).Hex()) != von {
			return fmt.Errorf("Unterschrift passt nicht zu %s", kurzAdresse(von))
		}
		gezaehlt[von] = true
	}
	if q := checkpointQuorum(len(komitee)); len(gezaehlt) < q {
		return fmt.Errorf("%d von %d Unterschriften, noetig %d", len(gezaehlt), len(komitee), q)
	}
	return nil
}
