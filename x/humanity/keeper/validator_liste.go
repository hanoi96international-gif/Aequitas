package keeper

import (
	"fmt"
	"os"
	"strings"
)

// KONSENSFESTE VALIDATORENLISTE (OFFEN 10, Stufe C, Baustein 4, Weg (b);
// docs/CHECKPOINT_FAHRPLAN.md).
//
// Im geschlossenen Betrieb kommt die Liste der Blockproduzenten aus
// AUTHORIZED_VALIDATORS -- aus der Umgebung jedes Knotens. Verschiedene
// Listen ergaeben verschiedene Komitees. Darum wird die Liste zur Regel:
//
//   - Ihr Hash (sha256 ueber die sortierten, klein geschriebenen Adressen;
//     dasselbe Verfahren wie komiteeHash) kann mit
//     AUTHORIZED_VALIDATORS_HASH festgeschrieben werden. Passt die Liste
//     nicht dazu, startet der Knoten nicht (fail-closed) -- ein Tippfehler
//     in der Liste faellt beim Start auf, nicht erst als Spaltung.
//   - Jede Leitungs-Nachricht traegt den Hash (Feld "liste"). Ein Knoten
//     hoert nur auf Validatoren mit derselben Liste: wer eine andere hat,
//     wird weder Leiter noch Mitglied seiner Leitung, und seine Stimmen
//     zaehlen nicht.
//   - Checkpoint-Zertifikate tragen schon den Komitee-Hash; eine andere
//     Liste ergibt ein anderes Komitee und damit Ablehnung.
//
// Schutz gegen Fehlkonfiguration, nicht gegen Bosheit: den Hash meldet jeder
// selbst; ein boeswilliger Validator meldet einfach den richtigen.
//
// Eine neue Liste heisst: auf allen Validatoren gleichzeitig Liste und Hash
// tauschen (docs/VALIDATOREN_HINZUFUEGEN.md). Vor dem offenen Betrieb wird
// das durch eine Liste in der Kette ersetzt (Weg (a)).

const validatorListeHashEnv = "AUTHORIZED_VALIDATORS_HASH"

// validatorListeHash: der Hash der geschlossenen Liste; "" ohne Liste.
func validatorListeHash(fest map[string]bool) string {
	if len(fest) == 0 {
		return ""
	}
	return komiteeHash(fest)
}

// pruefeValidatorListe: nil, wenn soll leer ist (nicht festgeschrieben) oder
// genau der Hash der Liste ist.
func pruefeValidatorListe(fest map[string]bool, soll string) error {
	soll = strings.ToLower(strings.TrimSpace(soll))
	if soll == "" {
		return nil
	}
	if !istKleinHex(soll, 64) {
		return fmt.Errorf("%s ist kein Hash (64 Hex-Zeichen)", validatorListeHashEnv)
	}
	if len(fest) == 0 {
		return fmt.Errorf("%s ist gesetzt, AUTHORIZED_VALIDATORS aber leer", validatorListeHashEnv)
	}
	if ist := validatorListeHash(fest); ist != soll {
		return fmt.Errorf("AUTHORIZED_VALIDATORS (%d Adressen, Hash %s) passt nicht zu %s=%s",
			len(fest), ist, validatorListeHashEnv, soll)
	}
	return nil
}

// validatorListeBeimStart: Meldung und Pruefung beim Start. Fehler: der
// Aufrufer startet nicht. self: die eigene Signieradresse -- steht sie
// nicht in der festgeschriebenen Liste, zaehlt dieser Knoten sich selbst
// trotzdem ins Komitee (erzeugerFestListe) und hat damit ein anderes als
// die anderen: laute Warnung (Checkpoints lehnen ihn dann ab).
func validatorListeBeimStart(fest map[string]bool, self string) error {
	soll := os.Getenv(validatorListeHashEnv)
	if err := pruefeValidatorListe(fest, soll); err != nil {
		return err
	}
	if h := validatorListeHash(fest); h != "" {
		if strings.TrimSpace(soll) == "" {
			fmt.Printf("[SICHERHEIT] Validatorenliste NICHT festgeschrieben -- auf jedem Validator %s=%s setzen\n", validatorListeHashEnv, h)
		} else {
			fmt.Printf("[SICHERHEIT] ✓ Validatorenliste festgeschrieben (Hash %s)\n", h)
		}
		if self != "" && !fest[self] {
			fmt.Printf("[SICHERHEIT] ⚠ eigene Signieradresse %s steht NICHT in AUTHORIZED_VALIDATORS -- dieser Knoten sieht ein anderes Komitee als die anderen\n", self)
		}
	}
	return nil
}

// ValidatorListeHash: fuer /api/status -- Betreiber vergleichen ihn ueber
// alle Validatoren. Ohne Sperre: produzentenFest wird nach dem Start nie
// geschrieben.
func (dag *BlockDAG) ValidatorListeHash() string { return validatorListeHash(dag.produzentenFest) }
