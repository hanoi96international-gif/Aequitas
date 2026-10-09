package keeper

import (
	"fmt"
	"net"
	"strings"
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
// Voll heisst groeber zaehlen: ein NEUER Absender zaehlt nicht mehr unter
// seiner Adresse (IPv4) bzw. seinem /64 (IPv6), sondern unter seinem Netz --
// IPv4 je /24, IPv6 je /48 -- in einer zweiten, eigenen Karte (grob), bis das
// Aufraeumen wieder Platz schafft; wer schon einen Eintrag hat, zaehlt weiter
// wie bisher. Eine Karte fuellen kann ein Angreifer mit wenigen Netzen: ein
// Absender belegt je Funktion einen Schluessel (in registerRateLimit bis zu
// zehn, in ipBurst bis zu fuenf), 20.000 bzw. 40.000 /64 -- ein einziges /48
// -- fuellen eine Karte. Bisher war danach jeder neue Absender ueberall
// gesperrt (Pruefung von #324, LOW-6); jetzt teilt sich nur das Netz des
// Angreifers eine Grenze, und jedes andere Netz hat seine eigene. Erst wenn
// auch grob voll ist (grobHoechstens Netze), wird ein neues Netz abgewiesen
// (fail-closed). Schluessel ohne IP (Wallet, Betreiber) haben kein Netz: fuer
// sie gilt das schon bei voller Karte -- ausser der Aufrufer nennt den
// Absender (LoadUeber/StoreUeber, so die Wallets von /api/prove).
// Die Validator-Bindung hat eine eigene Karte (bindungRateLimit), ebenso die
// frei gewaehlten Wallets (walletRateLimit) und die Betreiber
// (betreiberRateLimit). Groeber gezaehlte und abgewiesene neue Absender stehen
// je Karte in /api/health/combined (grenzen_je_absender) und hoechstens
// einmal je Minute im Log.

// grenzenSchluesselHoechstens: Eintraege je Karte -- bei 228 B je Schluessel
// etwa 45 MB, und weit ueber dem, was ein Knoten in zwei Minuten an echten
// Absendern sieht.
const grenzenSchluesselHoechstens = 200_000

// grobHoechstens: Netze je Ueberlaufkarte -- etwa 11 MB. 50.000 /48 hat kaum
// ein Angreifer (ein Tunnelanbieter gibt eines bis fuenf), 50.000 /24 sind
// 12,8 Mio. IPv4-Adressen.
const grobHoechstens = 50_000

type begrenzteKarte struct {
	name string
	max  int64
	// wertWennVoll: was Load fuer einen unbekannten Schluessel liefert, solange
	// die Karte voll ist (nil: nichts). registerRateLimit nimmt time.Now():
	// jeder Aufrufer prueft time.Since(ts) < Sperre, ein neuer Absender ist
	// dann gesperrt, statt an der Grenze vorbeizukommen (fail-closed).
	wertWennVoll func() any

	// grob: die Ueberlaufkarte je Netz (siehe oben); nil: keine, voll heisst
	// dann abweisen.
	grob *begrenzteKarte

	m         sync.Map
	anzahl    atomic.Int64
	abgelehnt atomic.Int64
	gegroebt  atomic.Int64 // Zugriffe neuer Absender, die unter ihrem Netz zaehlten
	gemeldet  atomic.Int64 // Unix-Sekunde der letzten Meldung im Log
	meldungen atomic.Int64 // Zahl der Meldungen -- fuer die Tests der Drossel
}

func neueBegrenzteKarte(name string, max int64, wertWennVoll func() any) *begrenzteKarte {
	k := &begrenzteKarte{name: name, max: max, wertWennVoll: wertWennVoll,
		grob: &begrenzteKarte{name: name + "_grob", max: grobHoechstens, wertWennVoll: wertWennVoll}}
	begrenzteKarten = append(begrenzteKarten, k)
	return k
}

// netzSchluessel: der Schluessel des Netzes eines Absenders. absender ist
// "<zweck>:<adresse>" oder "<adresse>", die Adresse wie aus clientIP; das
// Netz ist IPv4 je /24, IPv6 je /48. Ohne Adresse (Wallet, Betreiber, Muell)
// gibt es keins.
func netzSchluessel(absender any) (string, bool) {
	s, ok := absender.(string)
	if !ok {
		return "", false
	}
	zweck, adresse := "", s
	if net.ParseIP(s) == nil {
		i := strings.IndexByte(s, ':')
		if i < 0 {
			return "", false
		}
		zweck, adresse = s[:i+1], s[i+1:]
	}
	ip := net.ParseIP(adresse)
	if ip == nil {
		return "", false
	}
	if v4 := ip.To4(); v4 != nil {
		return zweck + v4.Mask(net.CIDRMask(24, 32)).String() + "/24", true
	}
	return zweck + ip.Mask(net.CIDRMask(48, 128)).String() + "/48", true
}

// begrenzteKarten: alle Karten, fuer den Stand. Nur beim Start gefuellt
// (Paketvariablen).
var begrenzteKarten []*begrenzteKarte

func (k *begrenzteKarte) voll() bool { return k.anzahl.Load() >= k.max }

// melden: hoechstens eine Zeile je Minute und Karte -- die Absender kommen
// von aussen, jede Anfrage eine Zeile waere eine Logflut.
func (k *begrenzteKarte) melden(text string) {
	jetzt := time.Now().Unix()
	if alt := k.gemeldet.Load(); jetzt-alt >= 60 && k.gemeldet.CompareAndSwap(alt, jetzt) {
		k.meldungen.Add(1)
		fmt.Printf("[GRENZE] ⚠ %s voll (%d Absender) -- %s\n", k.name, k.max, text)
	}
}

func (k *begrenzteKarte) ablehnen() {
	k.abgelehnt.Add(1)
	k.melden("neue Absender werden begrenzt, bis das Aufraeumen Platz schafft")
}

// netz: die Ueberlaufkarte und der Schluessel des Netzes, unter dem ein neuer
// Absender zaehlt, solange die Karte voll ist; ok=false: keins (abweisen).
func (k *begrenzteKarte) netz(absender any) (*begrenzteKarte, string, bool) {
	if k.grob == nil {
		return nil, "", false
	}
	n, ok := netzSchluessel(absender)
	if !ok {
		return nil, "", false
	}
	k.gegroebt.Add(1)
	k.melden("neue Absender zaehlen je Netz (/24, /48), bis das Aufraeumen Platz schafft")
	return k.grob, n, true
}

// Load wie sync.Map. Ist die Karte voll und key unbekannt: der Wert seines
// Netzes in grob -- ist auch grob voll oder hat key kein Netz, wertWennVoll
// (falls gesetzt).
func (k *begrenzteKarte) Load(key any) (any, bool) { return k.LoadUeber(key, key) }

// LoadUeber wie Load, nur zaehlt bei voller Karte das Netz von absender statt
// das von key -- fuer Schluessel ohne Adresse (Wallets).
func (k *begrenzteKarte) LoadUeber(key, absender any) (any, bool) {
	if v, ok := k.m.Load(key); ok {
		return v, true
	}
	if !k.voll() {
		return nil, false
	}
	if g, n, ok := k.netz(absender); ok {
		return g.Load(n)
	}
	if k.wertWennVoll != nil {
		k.ablehnen()
		return k.wertWennVoll(), true
	}
	return nil, false
}

// Store wie sync.Map; ein neuer Schluessel nur, solange Platz ist -- sonst
// unter seinem Netz in grob, und ohne Netz oder bei vollem grob false und
// nichts gespeichert.
func (k *begrenzteKarte) Store(key, v any) bool { return k.StoreUeber(key, v, key) }

// StoreUeber wie Store, mit dem Netz von absender (siehe LoadUeber).
func (k *begrenzteKarte) StoreUeber(key, v, absender any) bool {
	if _, ok := k.m.Load(key); !ok && k.voll() {
		if g, n, ok := k.netz(absender); ok {
			return g.Store(n, v)
		}
		k.ablehnen()
		return false
	}
	if _, geladen := k.m.Swap(key, v); !geladen {
		k.anzahl.Add(1)
	}
	return true
}

// LoadOrStore: der vorhandene oder neu gespeicherte Wert -- bei voller Karte
// der seines Netzes in grob; ok=false, wenn key neu ist, die Karte voll und
// kein Netz frei -- dann ist nichts gespeichert.
func (k *begrenzteKarte) LoadOrStore(key, v any) (any, bool) {
	if a, ok := k.m.Load(key); ok {
		return a, true
	}
	if k.voll() {
		if g, n, ok := k.netz(key); ok {
			return g.LoadOrStore(n, v)
		}
		k.ablehnen()
		return nil, false
	}
	a, geladen := k.m.LoadOrStore(key, v)
	if !geladen {
		k.anzahl.Add(1)
	}
	return a, true
}

// Delete loescht key -- auch einen Netzschluessel aus grob (Aufraeumen ueber
// Range).
func (k *begrenzteKarte) Delete(key any) {
	if _, ok := k.m.LoadAndDelete(key); ok {
		k.anzahl.Add(-1)
		return
	}
	if k.grob != nil {
		k.grob.Delete(key)
	}
}

// Range ueber die Karte und danach ueber grob -- das Aufraeumen jeder Karte
// raeumt so beide.
func (k *begrenzteKarte) Range(f func(key, v any) bool) {
	weiter := true
	k.m.Range(func(a, b any) bool {
		weiter = f(a, b)
		return weiter
	})
	if weiter && k.grob != nil {
		k.grob.Range(f)
	}
}

func (k *begrenzteKarte) stand() map[string]int64 {
	return map[string]int64{"eintraege": k.anzahl.Load(), "hoechstens": k.max, "neue_abgelehnt": k.abgelehnt.Load()}
}

// GrenzenJeAbsenderStand: fuer /api/health/combined.
func GrenzenJeAbsenderStand() map[string]interface{} {
	out := map[string]interface{}{
		"bedeutung": "Eintraege je Karte der Grenzen je Absender (begrenzte_karte.go); voll heisst: neue Absender zaehlen je Netz (grob: /24, /48), ist auch grob voll, werden sie begrenzt.",
	}
	for _, k := range begrenzteKarten {
		st := map[string]interface{}{}
		for n, w := range k.stand() {
			st[n] = w
		}
		st["je_netz_gezaehlt"] = k.gegroebt.Load()
		if k.grob != nil {
			st["grob"] = k.grob.stand()
		}
		out[k.name] = st
	}
	out["eigenes_gateway"] = gatewayStandFuerGrenzen()
	return out
}
