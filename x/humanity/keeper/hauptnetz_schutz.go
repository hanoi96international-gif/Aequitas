package keeper

// HAUPTNETZ-SCHUTZ FUER CLEAR_REGISTRATIONS (docs/VERIFIER_PRINZIPIEN.md, A1)
//
// Grundsatz: ein Verifier oder Betreiber darf nur bestaetigen, nie wegnehmen.
// clearRegistrationsFromDB setzt auf diesem Knoten jeden Menschen auf
// is_human = false und jedes Guthaben auf 0 -- knotenlokal, ohne Block, ohne
// Unterschrift eines einzigen Betroffenen. Drei Umgebungsvariablen und ein
// Neustart genuegten bisher. Auf dem echten Netz ist das ein Entzug durch den
// Betreiber; dort verweigert der Knoten es jetzt, laut und ohne Wirkung.
//
// WELCHES SIGNAL. Die EVM-Chain-ID taugt nicht: 1926 bzw. 19260930 sind
// Konstanten im Programm (chain_id_wechsel.go) und stehen in jedem Build,
// auch auf jeder Testkette. Was das echte Netz von einer Testkette trennt,
// ist die Genesis -- dieselbe Datei, aus der netzKennung und damit
// NETZ_SALT im V8-Vertrag folgen (api_app_grundlagen.go). Testketten haben
// laut STAGING_RUNBOOK.md eine eigene chain_id in ihrer Genesis.
//
// Verweigert wird, wenn EINES davon zutrifft:
//
//   - IS_PRIMARY_NODE=true (dieselbe Rolle, die RESET_DB_STATE schon
//     verweigert, resetDBStateForBootstrap);
//   - genesis.json fehlt, ist unlesbar oder nennt keine chain_id: dann ist
//     nicht feststellbar, welches Netz das ist -- fail-closed. Ohne Datei
//     faellt genesisTimestamp ausserdem auf die Genesis der alten Hauptkette
//     zurueck, der Knoten haelt sich also selbst fuer das Hauptnetz;
//   - chain_id der Genesis ist die des Hauptnetzes ("aequitas-1");
//   - genesis_time ist die einer Hauptnetz-Genesis (V8-Genesis vom
//     30.09.2026 oder die alte vom 13.06.2026) -- auch wenn jemand die
//     chain_id einer Kopie aendert.
//
// Eine Testkette mit eigener chain_id und eigener genesis_time, ohne
// IS_PRIMARY_NODE, kann CLEAR_REGISTRATIONS weiter benutzen (mit allen
// bisherigen Bestaetigungen).
//
// Missbrauchstest: hauptnetz_schutz_test.go.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// hauptnetzChainID: chain_id in der genesis.json des echten Netzes.
const hauptnetzChainID = "aequitas-1"

// hauptnetzGenesisZeiten: genesis_time der Genesis-Dateien des echten Netzes.
var hauptnetzGenesisZeiten = []time.Time{
	time.Date(2026, 9, 30, 18, 10, 0, 0, time.UTC), // V8-Genesis (genesis.json)
	time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC),   // alte Kette, auch Rueckfall von genesisTimestamp
}

// hauptnetzGenesisLesen: woher die Genesis kommt. Nur Tests ersetzen das.
var hauptnetzGenesisLesen = func() ([]byte, error) { return os.ReadFile("genesis.json") }

// hauptnetzGrund prueft, ob dieser Knoten zum echten Netz gehoert (oder das
// nicht auszuschliessen ist). Leer = Testkette; sonst der Grund.
func hauptnetzGrund() string {
	if os.Getenv("IS_PRIMARY_NODE") == "true" {
		return "IS_PRIMARY_NODE=true"
	}
	data, err := hauptnetzGenesisLesen()
	if err != nil {
		return fmt.Sprintf("genesis.json nicht lesbar (%v) -- Netz nicht feststellbar", err)
	}
	return hauptnetzGrundAusGenesis(data)
}

// hauptnetzGrundAusGenesis: wie hauptnetzGrund, nur aus dem Inhalt einer
// genesis.json.
func hauptnetzGrundAusGenesis(data []byte) string {
	var g struct {
		ChainID     string `json:"chain_id"`
		GenesisTime string `json:"genesis_time"`
	}
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&g); err != nil {
		return fmt.Sprintf("genesis.json unlesbar (%v) -- Netz nicht feststellbar", err)
	}
	cid := strings.TrimSpace(g.ChainID)
	if cid == "" {
		return "genesis.json ohne chain_id -- Netz nicht feststellbar"
	}
	if strings.EqualFold(cid, hauptnetzChainID) {
		return fmt.Sprintf("chain_id %q ist das Hauptnetz", cid)
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(g.GenesisTime)); err == nil {
		for _, h := range hauptnetzGenesisZeiten {
			if t.Equal(h) {
				return fmt.Sprintf("genesis_time %s ist die einer Hauptnetz-Genesis", h.Format(time.RFC3339))
			}
		}
	}
	return ""
}
