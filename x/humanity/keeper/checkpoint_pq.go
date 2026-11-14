package keeper

// QUANTENSICHERHEIT STUFE 5: DER CHECKPOINT WIRD HYBRID (docs/QUANTENSICHERHEIT.md).
//
// # DIE LUECKE
//
// Das Checkpoint-Zertifikat (checkpoint_zertifikat.go) traegt secp256k1-
// Unterschriften des Komitees. Wer mit einem Quantenrechner aus einer
// oeffentlichen Erzeugeradresse den Schluessel ableitet (Shor), unterschreibt
// fuer dieses Mitglied -- mit genug Mitgliedern beliebige Checkpoints.
//
// # WAS AB checkpointPQAbUnix GILT
//
// Jeder PQ-ANKER braucht von jedem Unterzeichner zusaetzlich eine ML-DSA-65-
// Unterschrift (FIPS 204) seines eingetragenen PQ-Schluessels ueber
// "aequitas-pq-checkpoint-v1|" + hex(signierHash) -- genau den Inhalt, den
// seine secp256k1-Unterschrift deckt. Beide muessen stimmen (hybrid). Ein
// Mitglied ohne eingetragenen PQ-Schluessel zaehlt in einem Anker nicht:
// fail-closed, nie "dann eben ohne".
//
// # WARUM NICHT JEDER CHECKPOINT (Entscheidung 14.11.2026)
//
// ML-DSA-65-Unterschriften sind 3.309 Byte gross und lassen sich nicht
// buendeln; ein Zertifikat traegt bis zu 2/3 von targetCommitteeSize = 100
// Unterschriften. Jeder Minuten-Checkpoint hybrid hiesse bei vollem Komitee
// ~320 MB je Tag. Anker ist darum jeder checkpointPQJederSlot-te Slot (10 ->
// alle 10 Minuten): ~370 Byte je Block. Die Minuten-Checkpoints dazwischen
// bleiben secp256k1. Ein Quantenangreifer kann damit hoechstens die
// Endgueltigkeit seit dem letzten Anker anfechten, nie dahinter.
//
// # NEUE ERZEUGER
//
// Den PQ-Schluessel eines Erzeugers bindet eine Erklaerung, die BEIDE
// Schluessel unterschreiben (pruefePQErzeugerBindung): secp256k1 beweist die
// Adresse, ML-DSA beweist den Besitz des neuen Schluessels. Die Erzeuger der
// Genesis bekommen ihn wie Stufe 1 ueber die Genesis.
//
// # SCHLAFEND
//
// checkpointPQAbUnix = math.MaxInt64; die Funktionen sind nirgends
// angeschlossen -- wie das Checkpoint-Zertifikat selbst, das erst ab vier
// Komiteemitgliedern etwas bestaetigt. Anschliessen (Register der PQ-
// Schluessel, Genesis, Annahme des Zertifikats) in einem eigenen, eigens
// geprueften PR, wenn das Komitee in Betrieb geht.

import (
	"crypto/mldsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
)

const (
	// checkpointPQAbUnix: AUS. Siehe Kopf.
	checkpointPQAbUnix int64 = math.MaxInt64

	// checkpointPQJederSlot: jeder wievielte Checkpoint-Slot ein PQ-Anker ist.
	checkpointPQJederSlot int64 = 10

	pqCheckpointPraefix = "aequitas-pq-checkpoint-v1|"
	pqErzeugerPraefix   = "aequitas-pq-erzeuger-v1"
)

var errCheckpointPQ = errors.New("Checkpoint-PQ abgelehnt")

// checkpointPQAnker: ob dieser Checkpoint quantensicher unterschrieben sein
// muss -- deterministisch aus seiner Zeit (Slot), wie jeder Knoten sie sieht.
func checkpointPQAnker(c KettenCheckpoint) bool { return checkpointPQAnkerAb(c, checkpointPQAbUnix) }

func checkpointPQAnkerAb(c KettenCheckpoint, ab int64) bool {
	if c.Zeit < ab {
		return false
	}
	return (c.Zeit/checkpointSlotLaenge)%checkpointPQJederSlot == 0
}

// checkpointPQNachricht: was der PQ-Schluessel unterschreibt.
func checkpointPQNachricht(c KettenCheckpoint) []byte {
	return []byte(pqCheckpointPraefix + hex.EncodeToString(c.signierHash()))
}

// checkpointPQUnterschreiben: die PQ-Unterschrift (hex) dieses Schluessels.
func checkpointPQUnterschreiben(c KettenCheckpoint, sk *mldsa.PrivateKey) (string, error) {
	if err := c.gueltig(); err != nil {
		return "", err
	}
	sig, err := sk.SignDeterministic(checkpointPQNachricht(c), nil)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sig), nil
}

// pruefeZertifikatPQ: der PQ-Teil, NACH pruefeZertifikat (das Zahl, Ordnung,
// Mitgliedschaft und secp256k1 schon geprueft und begrenzt hat).
//
// pq: eine PQ-Unterschrift je Eintrag von z.Unterschriften (gleiche
// Reihenfolge; leer = keine). schluessel: Adresse -> ML-DSA-65-Schluessel
// (hex) aus dem Register.
//
// Anker: jede Unterschrift braucht eine gueltige PQ-Unterschrift des
// eingetragenen Schluessels. Kein Anker: PQ darf fehlen; ist sie da, wird
// nur ihre Form geprueft (haelt das Feld klein, Fehler stoppen nichts).
func pruefeZertifikatPQ(z CheckpointZertifikat, pq []string, schluessel map[string]string) error {
	return pruefeZertifikatPQAb(z, pq, schluessel, checkpointPQAbUnix)
}

// pruefeZertifikatPQAb: wie pruefeZertifikatPQ mit gegebenem Stichtag (Tests).
func pruefeZertifikatPQAb(z CheckpointZertifikat, pq []string, schluessel map[string]string, ab int64) error {
	if len(pq) != len(z.Unterschriften) {
		return fmt.Errorf("%w: %d PQ-Eintraege zu %d Unterschriften", errCheckpointPQ, len(pq), len(z.Unterschriften))
	}
	for i, s := range pq {
		if s != "" && (len(s) != pqSignaturHex || !kleinHex(s)) {
			return fmt.Errorf("%w: PQ-Unterschrift %d in falscher Form", errCheckpointPQ, i)
		}
	}
	if !checkpointPQAnkerAb(z.KettenCheckpoint, ab) {
		return nil
	}
	nachricht := checkpointPQNachricht(z.KettenCheckpoint)
	for i, u := range z.Unterschriften {
		if pq[i] == "" {
			return fmt.Errorf("%w: Anker ohne PQ-Unterschrift von %s", errCheckpointPQ, kurzAdresse(u.Von))
		}
		pkHex, ok := schluessel[u.Von]
		if !ok {
			return fmt.Errorf("%w: %s hat keinen PQ-Schluessel", errCheckpointPQ, kurzAdresse(u.Von))
		}
		pk, err := pqSchluesselLesen(pkHex)
		if err != nil {
			return fmt.Errorf("%w: PQ-Schluessel von %s: %v", errCheckpointPQ, kurzAdresse(u.Von), err)
		}
		sig, _ := hex.DecodeString(pq[i])
		if mldsa.Verify(pk, nachricht, sig, nil) != nil {
			return fmt.Errorf("%w: PQ-Unterschrift von %s ungueltig", errCheckpointPQ, kurzAdresse(u.Von))
		}
	}
	return nil
}

// pqErzeugerBindungNachricht: was BEIDE Schluessel eines Erzeugers
// unterschreiben, um seinen PQ-Schluessel einzutragen.
func pqErzeugerBindungNachricht(adresse, pqSchluesselHexWert string) []byte {
	roh, _ := hex.DecodeString(pqSchluesselHexWert)
	h := sha256.Sum256(roh)
	return []byte(fmt.Sprintf("%s|chain:%d|%s|%s", pqErzeugerPraefix, checkpointChainID, adresse, hex.EncodeToString(h[:])))
}

// pruefePQErzeugerBindung: secp256k1 (65 Byte, kanonisch, low-s) ueber
// sha256(Nachricht) gehoert zur Adresse, ML-DSA ueber die Nachricht zum
// PQ-Schluessel. Erst billig (Form), dann teuer.
func pruefePQErzeugerBindung(adresse, pqSchluesselHexWert, secpSigHex, pqSigHex string) error {
	if !istKanonischeAdresse(adresse) {
		return fmt.Errorf("%w: Adresse nicht kanonisch", errCheckpointPQ)
	}
	pk, err := pqSchluesselLesen(pqSchluesselHexWert)
	if err != nil {
		return fmt.Errorf("%w: %v", errCheckpointPQ, err)
	}
	secp, ok := kanonischeSig(secpSigHex)
	if !ok {
		return fmt.Errorf("%w: secp256k1-Unterschrift nicht kanonisch", errCheckpointPQ)
	}
	if len(pqSigHex) != pqSignaturHex || !kleinHex(pqSigHex) {
		return fmt.Errorf("%w: PQ-Unterschrift in falscher Form", errCheckpointPQ)
	}
	nachricht := pqErzeugerBindungNachricht(adresse, pqSchluesselHexWert)
	h := sha256.Sum256(nachricht)
	pub, err := crypto.SigToPub(h[:], secp)
	if err != nil || strings.ToLower(crypto.PubkeyToAddress(*pub).Hex()) != adresse {
		return fmt.Errorf("%w: secp256k1-Unterschrift gehoert nicht zu %s", errCheckpointPQ, kurzAdresse(adresse))
	}
	pqSig, _ := hex.DecodeString(pqSigHex)
	if mldsa.Verify(pk, nachricht, pqSig, nil) != nil {
		return fmt.Errorf("%w: PQ-Unterschrift passt nicht zum Schluessel", errCheckpointPQ)
	}
	return nil
}
