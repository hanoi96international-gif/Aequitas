package keeper

// STUFE 2 -- ALLE NEHMEN AN, JEDER FUER SEINE KONTEN.
//
// Siehe docs/SKALIERUNG_DEZENTRAL.md, Stufe 2. Bis hierher nimmt genau ein
// Knoten an (annahme_tor.go, leitung.go): nehmen zwei gleichzeitig
// Belastungen DESSELBEN Kontos an, pruefen sie gegen verschiedene Sichten, und
// die Kontenstaende laufen auseinander (zwei_produzenten_realdb_test.go).
// Gutschriften sind unkritisch -- Addition kennt keine Reihenfolge.
//
// Die Bedingung heisst also nicht "nur einer nimmt an", sondern "jedes Konto
// wird nur von einem belastet". Stufe 2 teilt die Konten deshalb auf:
//
//	zustaendig(konto, term) = zuteilung[H(konto ‖ term) mod n]
//
// zuteilung ist die sortierte Menge der Validatoren, die der Leiter zu Beginn
// seiner Amtszeit (Term) festlegt und in jeder Lease mitschickt. Jeder kann
// das Ergebnis selbst ausrechnen; eine Entscheidung braucht es nicht. Mit
// jedem Term -- planmaessig alle WechselAlle, sonst bei jeder Wahl -- wechselt
// die Zuteilung, niemand haelt dauerhaft Macht ueber bestimmte Konten.
//
// Was sich nicht aufteilen laesst, bleibt beim Leiter des Terms (globale
// Konten): die vier Toepfe, der Liquiditaetspool und der Faucet. Der Leiter
// ist gewaehlt und rotiert -- er ist keine zentrale Instanz, sondern der
// Zustaendige fuer genau diese Konten in genau diesem Term.
//
// Ausfall: meldet sich ein Mitglied nicht mehr, fuehrt der Leiter es als
// ausgefallen; seine Konten gehen fuer den Rest des Terms an den Naechsten im
// Ring (naechsterLebender). Wann die Uebernahme sicher ist, regelt
// leitung_verteilt.go.
//
// Diese Datei ist reine Rechnung: ohne Netz, ohne Zustand, deterministisch.

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
)

// verteilteAnnahmeAbUnix: ab dieser Zeit nimmt jeder Validator fuer seine
// Konten an. math.MaxInt64 heisst AUS. Scharf erst, wenn mindestens drei
// unabhaengige Validatoren laufen (docs/SKALIERUNG_DEZENTRAL.md) und
// signierte Auftraege (Stufe 1.0) aktiv sind -- ohne sie koennte jeder
// Annehmende fremde Konten belasten.
const verteilteAnnahmeAbUnix int64 = math.MaxInt64

// verteilteAnnahmeOverride: nur fuer Tests (0 = Konstante).
var verteilteAnnahmeOverride atomic.Int64

func verteilteAnnahmeAb() int64 {
	if o := verteilteAnnahmeOverride.Load(); o != 0 {
		return o
	}
	return verteilteAnnahmeAbUnix
}

// verteilteAnnahmeAktiv: gilt Stufe 2 zur Zeit zeit (Annahme: jetzt;
// Nachspielen: Blockzeit)?
func verteilteAnnahmeAktiv(zeit int64) bool {
	return zeit >= verteilteAnnahmeAb()
}

// globalesKonto: Konten, die nur der Leiter belastet.
func globalesKonto(konto string) bool {
	k := strings.ToLower(strings.TrimSpace(konto))
	switch k {
	case "", kontoLiquiditaetspool, kontoFaucet:
		return true
	}
	return isTokenomicsPoolAddress(k)
}

// Pseudokonten fuer die Zuteilung von Auftraegen, die kein eigenes Konto
// belasten, sondern einen gemeinsamen Bestand.
const (
	kontoLiquiditaetspool = "amm"
	kontoFaucet           = "faucet"
)

// zuteilungsIndex: Position in der Zuteilung, rein aus Konto und Term.
func zuteilungsIndex(konto string, term uint64, n int) int {
	if n <= 0 {
		return -1
	}
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(konto)) + "|" + strconv.FormatUint(term, 10)))
	return int(binary.BigEndian.Uint64(h[:8]) % uint64(n))
}

// zustaendigFuer: wer im Term term fuer konto annimmt. zuteilung sortiert;
// ausgefallen: Mitglieder, deren Konten an den Naechsten im Ring gehen;
// leiter: fuer globale Konten. Leer, wenn niemand (alle ausgefallen).
func zustaendigFuer(konto string, term uint64, zuteilung []string, ausgefallen map[string]bool, leiter string) string {
	if globalesKonto(konto) {
		return leiter
	}
	n := len(zuteilung)
	i := zuteilungsIndex(konto, term, n)
	if i < 0 {
		return ""
	}
	return naechsterLebender(zuteilung, i, ausgefallen)
}

// naechsterLebender: ab Position i das erste nicht ausgefallene Mitglied.
func naechsterLebender(zuteilung []string, i int, ausgefallen map[string]bool) string {
	n := len(zuteilung)
	for k := 0; k < n; k++ {
		a := zuteilung[(i+k)%n]
		if !ausgefallen[a] {
			return a
		}
	}
	return ""
}

// annahmeKonto: nach welchem Konto sich die Zustaendigkeit fuer einen
// Auftrag richtet -- das Konto, das er BELASTET. Tausch und Liquiditaet
// belasten das Konto des Auftraggebers UND den Pool; sie laufen in Stufe 2
// ueber einen Vorbehalt (vorbehalt.go): zuerst belastet der Zustaendige des
// Auftraggebers dessen Konto, dann fuehrt der Leiter den Pool-Teil aus.
func annahmeKonto(tx *Transaction) string {
	switch tx.Type {
	case "register_human":
		// Einmaligkeit haengt am Nullifier: zwei Knoten duerfen denselben
		// nie gleichzeitig annehmen.
		return "nullifier:" + strings.ToLower(tx.Nullifier)
	case "faucet":
		return kontoFaucet
	case "vorbehalt_ausfuehrung":
		return kontoLiquiditaetspool
	}
	return tx.Wallet
}

// SystemauftraegeHier: leer, wenn dieser Knoten Systemauftraege (taegliche
// Verteilung, Treuhand nach Inaktivitaet, Umlauf) ausfuehren darf; sonst der
// Grund. Sie belasten die Toepfe -- im verteilten Term gehoeren die dem
// Leiter. Ohne verteilten Term aendert sich nichts (wie bisher).
func (cs *ChainState) SystemauftraegeHier() string {
	l := cs.leitung.Load()
	if l == nil {
		return ""
	}
	l.mu.Lock()
	verteilt := l.verteiltImTerm()
	l.mu.Unlock()
	if !verteilt {
		return ""
	}
	if cs.nimmtAnFuer(ubiPoolAddr) {
		return ""
	}
	return "verteilter Term, dieser Knoten ist nicht der Leiter"
}
