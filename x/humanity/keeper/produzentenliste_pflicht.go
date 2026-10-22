package keeper

import (
	"fmt"
	"os"
	"strings"
)

// Produzentenliste ist Pflicht (Audit 2026-10-21, H1).
//
// Ohne AUTHORIZED_VALIDATORS nimmt nimmProduzentAufLocked (block.go) jede
// Signieradresse auf, die ein registrierter Mensch ueber
// /api/register-validator-key eintraegt, und der Knoten lernt sie ueber den
// Validator-Abgleich von selbst. Solange das Nachspielen nicht jeden Wert
// selbst prueft (Registrierungen ohne Bescheinigung im Block, K1), hiess das:
// jeder registrierte Mensch konnte auf einem solchen Knoten Geld erzeugen.
// Betroffen waren genau die Knoten, die nach deploy/validator/.env.example
// eingerichtet wurden -- die Vorlage setzte die Liste nicht.
//
// Bis hierhin gab es beim Start nur eine Warnung. Jetzt startet der Knoten
// nicht, ausser der Betreiber erklaert die offene Liste ausdruecklich
// (AEQUITAS_PRODUZENTEN_OFFEN=ja, etwa fuer ein Testnetz). Auch eine
// gesetzte, aber unlesbare Liste haelt an: ein Tippfehler darf nicht still
// "offen" bedeuten.

// PruefeProduzentenliste: Fehler, wenn dieser Knoten jedem registrierten
// Menschen Bloecke glauben wuerde, ohne dass das erklaert ist.
func PruefeProduzentenliste() error {
	roh := strings.TrimSpace(os.Getenv("AUTHORIZED_VALIDATORS"))
	liste := loadAuthorizedValidators()
	if roh != "" {
		eintraege := 0
		for _, e := range strings.Split(roh, ",") {
			if strings.TrimSpace(e) != "" {
				eintraege++
			}
		}
		if len(liste) != eintraege {
			return fmt.Errorf("AUTHORIZED_VALIDATORS enthaelt %d Eintraege, davon nur %d gueltige Adressen (0x + 40 Hex) -- korrigieren", eintraege, len(liste))
		}
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("AEQUITAS_PRODUZENTEN_OFFEN")), "ja") {
		// Nur ausserhalb des Hauptnetzes (Sicherheitspruefung des Diffs,
		// F6): sonst oeffnete eine Zeile in der .env die Luecke aus H1 wieder.
		// Fail-closed: ist das Netz nicht feststellbar, gilt es als Hauptnetz.
		if grund := hauptnetzGrund(); grund != "" {
			return fmt.Errorf("AEQUITAS_PRODUZENTEN_OFFEN=ja gilt nur in einem Testnetz, hier nicht (%s) -- AUTHORIZED_VALIDATORS setzen", grund)
		}
		return nil
	}
	return fmt.Errorf("AUTHORIZED_VALIDATORS ist leer: dieser Knoten wuerde Bloecke JEDES registrierten Menschen " +
		"annehmen, der einen Signierschluessel eintraegt. Die Liste der Netz-Produzenten setzen " +
		"(deploy/validator/.env.example) -- oder fuer ein Testnetz ausdruecklich AEQUITAS_PRODUZENTEN_OFFEN=ja")
}
