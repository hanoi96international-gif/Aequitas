package keeper

import (
	"sync"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
)

// STUFE 1.2 -- JEDE SIGNATUR NUR EINMAL PRUEFEN.
//
// Die Wiederherstellung des Absenders aus einer secp256k1-Signatur kostet
// gemessen 101 µs (SCALING_ARCHITECTURE.md) und ist mit Abstand der teuerste
// Schritt einer Ueberweisung. Bis 26.09.2026 lief sie fuer dieselbe
// Rohtransaktion mehrfach: bei der Annahme (sendRawTransaction), gleich danach
// noch einmal nur fuer die Nonce (nonceAusVorlage), und beim Nachspielen
// desselben Blocks auf demselben Knoten (Resync, Neustart, Wiederholung nach
// Ruecklauf) jedes Mal wieder.
//
// Der Absender haengt allein an den Bytes der Rohtransaktion: t.Hash() ist
// der keccak ueber die ganze signierte Form, Signatur eingeschlossen. Zwei
// Rohformen mit gleichem Hash sind dieselbe Transaktion, also hat eine
// gespeicherte Zuordnung Hash -> Absender keine Moeglichkeit, falsch zu sein.
// Der Schluessel ist der AUS DEN BYTES berechnete Hash, nie das TxHash-Feld
// eines Blocks -- das koennte ein Erzeuger faelschen.
//
// Nur Erfolge werden gemerkt. Groesse fest (absenderCacheGroesse Eintraege,
// je ~60 Byte), aeltester Eintrag faellt zuerst heraus.
const absenderCacheGroesse = 1 << 17

type absenderCache struct {
	mu      sync.Mutex
	eintrag map[common.Hash]string
	ring    []common.Hash
	pos     int
}

var absenderSpeicher = &absenderCache{eintrag: make(map[common.Hash]string, absenderCacheGroesse)}

var (
	absenderTreffer  atomic.Int64
	absenderVerfehlt atomic.Int64
)

func (c *absenderCache) holen(h common.Hash) (string, bool) {
	c.mu.Lock()
	a, ok := c.eintrag[h]
	c.mu.Unlock()
	if ok {
		absenderTreffer.Add(1)
	} else {
		absenderVerfehlt.Add(1)
	}
	return a, ok
}

func (c *absenderCache) merken(h common.Hash, absender string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.eintrag[h]; ok {
		return
	}
	if len(c.ring) < absenderCacheGroesse {
		c.ring = append(c.ring, h)
	} else {
		delete(c.eintrag, c.ring[c.pos])
		c.ring[c.pos] = h
		c.pos = (c.pos + 1) % absenderCacheGroesse
	}
	c.eintrag[h] = absender
}

// AbsenderCacheStand fuer /health: wie oft eine Wiederherstellung gespart wurde.
func AbsenderCacheStand() map[string]interface{} {
	absenderSpeicher.mu.Lock()
	n := len(absenderSpeicher.eintrag)
	absenderSpeicher.mu.Unlock()
	return map[string]interface{}{
		"eintraege": n,
		"treffer":   absenderTreffer.Load(),
		"verfehlt":  absenderVerfehlt.Load(),
		"bedeutung": "Stufe 1.2: jeder Treffer ist eine gesparte Signaturpruefung (~101 µs).",
	}
}
