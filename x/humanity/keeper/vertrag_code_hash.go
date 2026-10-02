package keeper

// Welcher Vertrag laeuft hier wirklich? (Audit von null, 02.10.2026, Punkt 10)
//
// /api/status nannte die Adressen von Registervertrag und BioVerifier, aber
// nicht, welcher Code dort liegt. Wer pruefen wollte, ob der Knoten den
// veroeffentlichten Vertrag ausfuehrt, musste eth_getCode holen und selbst
// hashen -- und wusste dann noch nicht, womit er vergleichen soll.
//
// Jetzt steht neben jeder Adresse der keccak256 des Laufzeit-Codes und seine
// Laenge. Jeder kann ihn mit dem Hash aus dem eigenen Kompilat vergleichen,
// oder zwei Knoten miteinander. Der Hash ist nur eine Anzeige; keine Regel
// haengt an ihm.
//
// Begrenzt: Vertragscode aendert sich nach dem Deploy nicht. Der Hash wird
// beim ersten Fund festgehalten; solange es noch keinen Code gibt, wird
// hoechstens einmal je Minute in der Datenbank nachgesehen.

import (
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

type codeHashEintrag struct {
	hash    string
	laenge  int
	gefragt time.Time
}

var (
	codeHashMu    sync.Mutex
	codeHashCache = map[string]codeHashEintrag{}
)

const codeHashNeuVersuch = time.Minute

// vertragCodeHash: keccak256 des Laufzeit-Codes an addr und seine Laenge in
// Byte. ("", 0), wenn dort (noch) kein Code liegt oder er nicht lesbar ist.
func vertragCodeHash(cs *ChainState, addr string) (string, int) {
	if cs == nil {
		return "", 0
	}
	addr = strings.ToLower(addr)
	codeHashMu.Lock()
	e, ok := codeHashCache[addr]
	if ok && (e.hash != "" || time.Since(e.gefragt) < codeHashNeuVersuch) {
		codeHashMu.Unlock()
		return e.hash, e.laenge
	}
	codeHashMu.Unlock()

	code, err := cs.LoadContract(addr)
	neu := codeHashEintrag{gefragt: time.Now()}
	if err == nil && len(code) > 0 {
		neu.hash = crypto.Keccak256Hash(code).Hex()
		neu.laenge = len(code)
	}
	codeHashMu.Lock()
	codeHashCache[addr] = neu
	codeHashMu.Unlock()
	return neu.hash, neu.laenge
}

// vertragCodeStand fuer /api/status: null statt eines falschen Werts, wenn
// kein Code da ist.
func vertragCodeStand(cs *ChainState, addr string) map[string]interface{} {
	h, n := vertragCodeHash(cs, addr)
	var hash interface{}
	if h != "" {
		hash = h
	}
	return map[string]interface{}{"adresse": addr, "code_keccak256": hash, "code_bytes": n}
}
