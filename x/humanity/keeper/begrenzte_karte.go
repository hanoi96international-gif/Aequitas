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
// Eine Karte fuellen kann ein Angreifer mit wenigen Netzen: ein Absender
// belegt je Funktion einen Schluessel (in registerRateLimit bis zu zehn, in
// ipBurst bis zu fuenf), 20.000 bzw. 40.000 /64 -- ein einziges /48 --
// fuellen eine Karte. Bisher war danach jeder neue Absender ueberall gesperrt
// (Pruefung von #324, LOW-6).
//
// Voll heisst darum groeber zaehlen: ein NEUER Absender zaehlt nicht mehr
// unter seiner Adresse (IPv4) bzw. seinem /64 (IPv6), sondern unter seinem
// Netz -- IPv4 je /24, IPv6 je /48, ueber alle Funktionen der Karte -- in
// einer zweiten, eigenen Karte (grob), bis das Aufraeumen wieder Platz
// schafft. Wer schon einen Eintrag hat, zaehlt weiter wie bisher. Solange die
// Karte voll ist, teilen sich also alle neuen Absender eines Netzes eine
// Grenze, auch ueber die Funktionen hinweg; das Netz des Angreifers sperrt
// kein anderes. Von einem IPv6-/32 zaehlen hoechstens grobJePraefix /48 je fuer
// sich, jedes weitere unter dem /32. grob fuellen (grobHoechstens Netze)
// kann darum erst, wer Adressen in 50.000 /24 hat oder rund 770 /32 --
// danach wird jedes neue Netz abgewiesen (fail-closed; docs/OFFEN.md).
//
// Schluessel ohne Adresse (Wallet, Betreiber) haben kein Netz: fuer sie gilt
// schon bei voller Karte abweisen -- ausser der Aufrufer nennt den Absender
// (LoadUeber/StoreUeber, so die Wallets von /api/prove).
// Die Validator-Bindung hat eine eigene Karte (bindungRateLimit), ebenso die
// frei gewaehlten Wallets (walletRateLimit) und die Betreiber
// (betreiberRateLimit, ohne Netz). Je Netz gezaehlte und abgewiesene neue
// Absender stehen je Karte in /api/health/combined (grenzen_je_absender) und
// hoechstens einmal je Minute im Log.

// grenzenSchluesselHoechstens: Eintraege je Karte -- bei 228 B je Schluessel
// etwa 45 MB, und weit ueber dem, was ein Knoten in zwei Minuten an echten
// Absendern sieht.
const grenzenSchluesselHoechstens = 200_000

// grobHoechstens: Netze je Ueberlaufkarte. Gemessen 175 B je Netz mit einem
// Zeitpunkt (register, wallet, bindung), bis 1.077 B mit vollem Fenster
// (ip_burst, 20 Zeitpunkte): hoechstens etwa 9 bzw. 54 MB je Karte, ueber
// alle Karten etwa 100 MB.
const grobHoechstens = 50_000

// grobJePraefix: so viele /48 eines IPv6-/32 zaehlen in einer Ueberlaufkarte
// je fuer sich, jedes weitere unter dem /32 selbst. Ein /32 -- die kleinste
// Zuteilung an einen Provider -- hat 65.536 /48 und fuellte sonst allein jede
// Ueberlaufkarte.
const grobJePraefix = 64

type begrenzteKarte struct {
	name    string
	einheit string // fuer die Meldung: "Absender" oder "Netze"
	max     int64
	// wertWennVoll: was Load fuer einen unbekannten Schluessel liefert, wenn
	// er weder in der Karte noch unter seinem Netz Platz hat (nil: nichts).
	// registerRateLimit nimmt time.Now(): jeder Aufrufer prueft
	// time.Since(ts) < Sperre, ein neuer Absender ist dann gesperrt, statt an
	// der Grenze vorbeizukommen (fail-closed).
	wertWennVoll func() any

	// grob: die Ueberlaufkarte je Netz (siehe oben); nil: keine, voll heisst
	// dann abweisen.
	grob *begrenzteKarte
	// praefixe: nur in einer Ueberlaufkarte -- je IPv6-/32 die Zahl seiner /48.
	praefixe *sync.Map

	m            sync.Map
	anzahl       atomic.Int64
	abgelehnt    atomic.Int64
	gegroebt     atomic.Int64 // Zugriffe neuer Absender, die unter ihrem Netz zaehlten
	gemeldet     atomic.Int64 // Unix-Sekunde der letzten Meldung "abgewiesen"
	gemeldetNetz atomic.Int64 // Unix-Sekunde der letzten Meldung "je Netz"
	meldungen    atomic.Int64 // Zahl der Meldungen -- fuer die Tests der Drossel
}

func neueBegrenzteKarte(name string, max int64, wertWennVoll func() any) *begrenzteKarte {
	k := neueBegrenzteKarteOhneNetz(name, max, wertWennVoll)
	k.grob = &begrenzteKarte{name: name + "_grob", einheit: "Netze", max: grobHoechstens,
		wertWennVoll: wertWennVoll, praefixe: &sync.Map{}}
	return k
}

// neueBegrenzteKarteOhneNetz: ohne Ueberlaufkarte -- fuer Schluessel, die nie
// eine Adresse tragen. betreiberRateLimit liest den Betreiber, wie ihn die
// Anfrage nennt, vor der Pruefung der Unterschrift; ein Betreiber "1.2.3.4"
// bekaeme sonst bei voller Karte einen Platz unter "seinem" Netz.
func neueBegrenzteKarteOhneNetz(name string, max int64, wertWennVoll func() any) *begrenzteKarte {
	k := &begrenzteKarte{name: name, einheit: "Absender", max: max, wertWennVoll: wertWennVoll}
	begrenzteKarten = append(begrenzteKarten, k)
	return k
}

// netzSchluessel: das Netz eines Absenders -- IPv4 je /24, IPv6 je /48, dazu
// fuer IPv6 sein /32. absender ist "<zweck>:<adresse>" oder "<adresse>", die
// Adresse wie aus clientIP; der Zweck zaehlt nicht mit. Ohne Adresse (Wallet,
// Betreiber, Muell) gibt es keins.
func netzSchluessel(absender any) (netz, praefix string, ok bool) {
	s, ok := absender.(string)
	if !ok {
		return "", "", false
	}
	adresse := s
	if net.ParseIP(s) == nil {
		i := strings.IndexByte(s, ':')
		if i < 0 {
			return "", "", false
		}
		adresse = s[i+1:]
	}
	ip := net.ParseIP(adresse)
	if ip == nil {
		return "", "", false
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.Mask(net.CIDRMask(24, 32)).String() + "/24", "", true
	}
	return ip.Mask(net.CIDRMask(48, 128)).String() + "/48", ip.Mask(net.CIDRMask(32, 128)).String() + "/32", true
}

// praefixVonNetz: das /32 eines /48-Schluessels in einer Ueberlaufkarte.
func praefixVonNetz(key any) (string, bool) {
	s, ok := key.(string)
	if !ok || !strings.HasSuffix(s, "/48") {
		return "", false
	}
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		return "", false
	}
	return n.IP.Mask(net.CIDRMask(32, 128)).String() + "/32", true
}

// begrenzteKarten: alle Karten, fuer den Stand. Nur beim Start gefuellt
// (Paketvariablen).
var begrenzteKarten []*begrenzteKarte

func (k *begrenzteKarte) voll() bool { return k.anzahl.Load() >= k.max }

// gezaehlt: ein Schluessel mehr (plus 1) oder weniger (-1) -- in einer
// Ueberlaufkarte auch fuer das /32 eines /48. Laufen Neuanlage und Loeschen
// im selben /32 gleichzeitig, kann die Zahl des /32 um wenige zu klein sein
// (begrenzt durch die gleichzeitigen Anfragen); die Zahl der Schluessel
// stimmt immer.
func (k *begrenzteKarte) gezaehlt(key any, plus int64) {
	k.anzahl.Add(plus)
	if k.praefixe == nil {
		return
	}
	p, ok := praefixVonNetz(key)
	if !ok {
		return
	}
	v, _ := k.praefixe.LoadOrStore(p, new(atomic.Int64))
	if z := v.(*atomic.Int64); z.Add(plus) <= 0 {
		k.praefixe.CompareAndDelete(p, z)
	}
}

// melden: hoechstens eine Zeile je Minute, Karte und Art -- die Absender
// kommen von aussen, jede Anfrage eine Zeile waere eine Logflut.
func (k *begrenzteKarte) melden(stempel *atomic.Int64, text string) {
	jetzt := time.Now().Unix()
	if alt := stempel.Load(); jetzt-alt >= 60 && stempel.CompareAndSwap(alt, jetzt) {
		k.meldungen.Add(1)
		fmt.Printf("[GRENZE] ⚠ %s voll (%d %s) -- %s\n", k.name, k.max, k.einheit, text)
	}
}

func (k *begrenzteKarte) ablehnen() {
	k.abgelehnt.Add(1)
	k.melden(&k.gemeldet, "neue werden begrenzt, bis das Aufraeumen Platz schafft")
}

// netz: die Ueberlaufkarte und der Schluessel des Netzes, unter dem ein neuer
// Absender zaehlt, solange die Karte voll ist; ok=false: keins (abweisen).
func (k *begrenzteKarte) netz(absender any) (*begrenzteKarte, string, bool) {
	if k.grob == nil {
		return nil, "", false
	}
	n, praefix, ok := netzSchluessel(absender)
	if !ok {
		return nil, "", false
	}
	k.gegroebt.Add(1)
	k.melden(&k.gemeldetNetz, "neue Absender zaehlen je Netz (/24, /48), bis das Aufraeumen Platz schafft")
	return k.grob, k.grob.jePraefix(n, praefix), true
}

// jePraefix: das /48 selbst, solange es schon zaehlt oder sein /32 weniger
// als grobJePraefix /48 hat -- sonst das /32.
func (g *begrenzteKarte) jePraefix(n, praefix string) string {
	if praefix == "" || g.praefixe == nil {
		return n
	}
	if _, ok := g.m.Load(n); ok {
		return n
	}
	if z, ok := g.praefixe.Load(praefix); ok && z.(*atomic.Int64).Load() >= grobJePraefix {
		return praefix
	}
	return n
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
		k.gezaehlt(key, 1)
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
		k.gezaehlt(key, 1)
	}
	return a, true
}

// Delete loescht key -- auch einen Netzschluessel aus grob (Aufraeumen ueber
// Range).
func (k *begrenzteKarte) Delete(key any) {
	if _, ok := k.m.LoadAndDelete(key); ok {
		k.gezaehlt(key, -1)
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
		"bedeutung": "Eintraege je Karte der Grenzen je Absender (begrenzte_karte.go); voll heisst: neue Absender zaehlen je Netz (grob: /24, /48, je /32 hoechstens 64 /48), ist auch grob voll, werden sie begrenzt. je_netz_gezaehlt zaehlt Zugriffe (Lesen und Schreiben getrennt).",
	}
	for _, k := range begrenzteKarten {
		st := map[string]interface{}{}
		for n, w := range k.stand() {
			st[n] = w
		}
		if k.grob != nil {
			st["je_netz_gezaehlt"] = k.gegroebt.Load()
			st["grob"] = k.grob.stand()
		}
		out[k.name] = st
	}
	out["eigenes_gateway"] = gatewayStandFuerGrenzen()
	return out
}
