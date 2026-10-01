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
	grenze  int // 0 = absenderCacheGroesse
}

// VERTEILT AUF absenderCacheTeile SPERREN (seit 01.10.2026).
//
// Gemessen auf dem C1-Pruefstand (4.000 Konten, Goroutine-Schnappschuss):
// 1.204 Goroutinen standen an der EINEN Sperre dieses Caches -- jede
// Ueberweisung fragt ihn bei der Annahme. Der Schluessel ist ein keccak-
// Hash, sein erstes Byte also gleichverteilt; danach wird verteilt. Gesamt-
// groesse und Verdraengung (aeltester zuerst, je Teil) bleiben begrenzt wie
// bisher.
const absenderCacheTeile = 64

type absenderCacheVerteilt struct {
	teile [absenderCacheTeile]*absenderCache
}

func neuerAbsenderCacheVerteilt() *absenderCacheVerteilt {
	v := &absenderCacheVerteilt{}
	je := absenderCacheGroesse / absenderCacheTeile
	for i := range v.teile {
		v.teile[i] = &absenderCache{eintrag: make(map[common.Hash]string, je), grenze: je}
	}
	return v
}

func (v *absenderCacheVerteilt) teil(h common.Hash) *absenderCache {
	return v.teile[int(h[0])%absenderCacheTeile]
}

func (v *absenderCacheVerteilt) holen(h common.Hash) (string, bool) {
	return v.teil(h).holen(h)
}

func (v *absenderCacheVerteilt) merken(h common.Hash, absender string) {
	v.teil(h).merken(h, absender)
}

func (v *absenderCacheVerteilt) anzahl() int {
	n := 0
	for _, t := range v.teile {
		t.mu.Lock()
		n += len(t.eintrag)
		t.mu.Unlock()
	}
	return n
}

var absenderSpeicher = neuerAbsenderCacheVerteilt()

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
	grenze := c.grenze
	if grenze <= 0 {
		grenze = absenderCacheGroesse
	}
	if len(c.ring) < grenze {
		c.ring = append(c.ring, h)
	} else {
		delete(c.eintrag, c.ring[c.pos])
		c.ring[c.pos] = h
		c.pos = (c.pos + 1) % grenze
	}
	c.eintrag[h] = absender
}

// AbsenderCacheStand fuer /health: wie oft eine Wiederherstellung gespart wurde.
func AbsenderCacheStand() map[string]interface{} {
	n := absenderSpeicher.anzahl()
	return map[string]interface{}{
		"eintraege": n,
		"treffer":   absenderTreffer.Load(),
		"verfehlt":  absenderVerfehlt.Load(),
		"bedeutung": "Stufe 1.2: jeder Treffer ist eine gesparte Signaturpruefung (~101 µs).",
	}
}
