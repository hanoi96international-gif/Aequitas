package keeper

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// BEGRENZTE KARTEN FUER DIE GRENZEN JE ABSENDER (Pruefung von #319, LOW-4).
//
// ipBurst, rpcRateLimit und registerRateLimit fuehren je Absender einen
// Eintrag. Die Eintraege verfallen (Aufraeumen jede Minute), ihre ZAHL war
// aber nur durch Anfragerate mal Lebensdauer begrenzt: gemessen 228 B je
// Schluessel, bei 5.000 neuen Adressen je Sekunde rund 137 MB allein in
// ipBurst. Jetzt hat jede Karte eine feste Hoechstzahl.
//
// Voll heisst begrenzen: ein NEUER Absender wird abgewiesen, bis das
// Aufraeumen Platz schafft (je nach Karte nach etwa ein bis zwei Minuten);
// wer schon einen Eintrag hat, zaehlt weiter wie bisher. Mit IPv6 je /64
// (clientIP) braucht ein Angreifer dafuer so viele Netze wie Eintraege.
// Abgewiesene neue Absender stehen je Karte in /api/health/combined
// (grenzen_je_absender) und hoechstens einmal je Minute im Log.

// grenzenSchluesselHoechstens: Eintraege je Karte -- bei 228 B je Schluessel
// etwa 45 MB, und weit ueber dem, was ein Knoten in zwei Minuten an echten
// Absendern sieht.
const grenzenSchluesselHoechstens = 200_000

type begrenzteKarte struct {
	name string
	max  int64
	// wertWennVoll: was Load fuer einen unbekannten Schluessel liefert, solange
	// die Karte voll ist (nil: nichts). registerRateLimit nimmt time.Now():
	// jeder Aufrufer prueft time.Since(ts) < Sperre, ein neuer Absender ist
	// dann gesperrt, statt an der Grenze vorbeizukommen (fail-closed).
	wertWennVoll func() any

	m         sync.Map
	anzahl    atomic.Int64
	abgelehnt atomic.Int64
	gemeldet  atomic.Int64 // Unix-Sekunde der letzten Meldung im Log
}

func neueBegrenzteKarte(name string, max int64, wertWennVoll func() any) *begrenzteKarte {
	k := &begrenzteKarte{name: name, max: max, wertWennVoll: wertWennVoll}
	begrenzteKarten = append(begrenzteKarten, k)
	return k
}

// begrenzteKarten: alle Karten, fuer den Stand. Nur beim Start gefuellt
// (Paketvariablen).
var begrenzteKarten []*begrenzteKarte

func (k *begrenzteKarte) voll() bool { return k.anzahl.Load() >= k.max }

func (k *begrenzteKarte) ablehnen() {
	k.abgelehnt.Add(1)
	jetzt := time.Now().Unix()
	if alt := k.gemeldet.Load(); jetzt-alt >= 60 && k.gemeldet.CompareAndSwap(alt, jetzt) {
		fmt.Printf("[GRENZE] ⚠ %s voll (%d Absender) -- neue Absender werden begrenzt, bis das Aufraeumen Platz schafft\n", k.name, k.max)
	}
}

// Load wie sync.Map; ist die Karte voll und key unbekannt, liefert es
// wertWennVoll (falls gesetzt).
func (k *begrenzteKarte) Load(key any) (any, bool) {
	if v, ok := k.m.Load(key); ok {
		return v, true
	}
	if k.wertWennVoll != nil && k.voll() {
		k.ablehnen()
		return k.wertWennVoll(), true
	}
	return nil, false
}

// Store wie sync.Map; ein neuer Schluessel nur, solange Platz ist (sonst
// false und nichts gespeichert).
func (k *begrenzteKarte) Store(key, v any) bool {
	if _, ok := k.m.Load(key); !ok && k.voll() {
		k.ablehnen()
		return false
	}
	if _, geladen := k.m.Swap(key, v); !geladen {
		k.anzahl.Add(1)
	}
	return true
}

// LoadOrStore: der vorhandene oder neu gespeicherte Wert; ok=false, wenn key
// neu ist und die Karte voll -- dann ist nichts gespeichert.
func (k *begrenzteKarte) LoadOrStore(key, v any) (any, bool) {
	if a, ok := k.m.Load(key); ok {
		return a, true
	}
	if k.voll() {
		k.ablehnen()
		return nil, false
	}
	a, geladen := k.m.LoadOrStore(key, v)
	if !geladen {
		k.anzahl.Add(1)
	}
	return a, true
}

func (k *begrenzteKarte) Delete(key any) {
	if _, ok := k.m.LoadAndDelete(key); ok {
		k.anzahl.Add(-1)
	}
}

func (k *begrenzteKarte) Range(f func(key, v any) bool) { k.m.Range(f) }

// GrenzenJeAbsenderStand: fuer /api/health/combined.
func GrenzenJeAbsenderStand() map[string]interface{} {
	out := map[string]interface{}{
		"bedeutung": "Eintraege je Karte der Grenzen je Absender (begrenzte_karte.go); voll heisst: neue Absender werden begrenzt.",
	}
	for _, k := range begrenzteKarten {
		out[k.name] = map[string]int64{"eintraege": k.anzahl.Load(), "hoechstens": k.max, "neue_abgelehnt": k.abgelehnt.Load()}
	}
	return out
}
