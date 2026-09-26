package keeper

// LEISTUNGSPROBE: DIE ANDEREN MESSEN, NICHT DER KNOTEN SELBST.
//
// docs/SKALIERUNG_DEZENTRAL.md, Stufe 2, "Wer ist Validator?": registrierter
// Mensch plus die Mindestleistung aus einem Leistungstest -- das Netz stellt
// eine Aufgabe, die anderen messen die Zeit, zu zufaelligen Zeitpunkten. Der
// Test entscheidet nur ueber ja oder nein, nie ueber Rang oder Macht.
//
// Bisher mass jeder Knoten sich selbst (leistungsnachweis.go) und kuendigte
// das Ergebnis an. Wer dabei luegt, "schadet der Geschwindigkeit, nicht der
// Sicherheit" -- mit Stufe 2 aber traegt jedes Mitglied der Zuteilung einen
// Teil der Konten; ein zu langsames bremst genau diese Menschen aus.
//
// Die Probe: ein Mitglied schickt einem anderen eine signierte Aufgabe
// (Zufallswert, Anzahl n). Der Gepruefte leitet n Schluessel aus dem Wert ab,
// signiert und stellt n-mal den Absender wieder her -- genau die Arbeit einer
// Ueberweisung -- und gibt den Hash aller Ergebnisse zurueck. Der Pruefer
// rechnet dasselbe nach (er ist selbst Validator) und misst die Zeit bis zur
// Antwort. Bestanden, wenn das Ergebnis stimmt und die Zeit unter
// n / MinSigProSek plus Laufzeit liegt.
//
// Verwendet vom Leiter beim Antritt: in die Zuteilung des Terms kommen nur
// Mitglieder, deren letzte Probe (hoechstens probeGueltig alt) bestanden ist
// -- oder alle, wenn er noch keine hat pruefen koennen. Weil die Zuteilung
// allein der Leiter festlegt und mit der Lease verteilt, ist sie fuer alle
// gleich, egal wer wen gemessen hat.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

const (
	probeAnzahl  = 4000             // Signaturen je Probe
	probeGueltig = 30 * time.Minute // so lange zaehlt ein Ergebnis
	probeAbstand = 3 * time.Minute  // mittlerer Abstand je Mitglied (zufaellig 0,5x..1,5x)
)

// leistungsprobeRechnen: die Arbeit selbst. Deterministisch aus seed und n,
// parallel auf allen Kernen.
func leistungsprobeRechnen(seed []byte, n int) (string, error) {
	if n <= 0 || n > 100_000 {
		return "", fmt.Errorf("Anzahl %d ausserhalb 1..100000", n)
	}
	ergebnisse := make([][20]byte, n)
	fehler := make([]error, n)
	workers := runtime.NumCPU()
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := w; i < n; i += workers {
				var buf [8]byte
				binary.BigEndian.PutUint64(buf[:], uint64(i))
				kh := sha256.Sum256(append(append([]byte("probe-schluessel:"), seed...), buf[:]...))
				key, err := crypto.ToECDSA(kh[:])
				if err != nil {
					fehler[i] = err
					continue
				}
				mh := sha256.Sum256(append(append([]byte("probe-nachricht:"), seed...), buf[:]...))
				sig, err := crypto.Sign(mh[:], key)
				if err != nil {
					fehler[i] = err
					continue
				}
				pub, err := crypto.SigToPub(mh[:], sig)
				if err != nil {
					fehler[i] = err
					continue
				}
				copy(ergebnisse[i][:], crypto.PubkeyToAddress(*pub).Bytes())
			}
		}(w)
	}
	wg.Wait()
	h := sha256.New()
	for i := range ergebnisse {
		if fehler[i] != nil {
			return "", fehler[i]
		}
		h.Write(ergebnisse[i][:])
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// probeBestanden: Ergebnis richtig und schnell genug? rtt: gemessene
// Laufzeit einer leeren Anfrage, wird abgezogen.
func probeBestanden(erwartet, erhalten string, dauer, rtt time.Duration, n int, minSigProSek float64) bool {
	if erwartet == "" || erwartet != erhalten {
		return false
	}
	if minSigProSek <= 0 {
		return true
	}
	rechen := dauer - rtt
	if rechen < 0 {
		rechen = 0
	}
	// Zwei Operationen je Signatur (signieren und wiederherstellen), die
	// Schwelle zaehlt Wiederherstellungen: doppelte Zeit erlaubt.
	grenze := time.Duration(2 * float64(n) / minSigProSek * float64(time.Second))
	return rechen <= grenze
}

// --- Ergebnisse ------------------------------------------------------------------

type probeErgebnis struct {
	bestanden bool
	zeit      time.Time
	dauer     time.Duration
}

type probenStand struct {
	mu        sync.Mutex
	ergebnis  map[string]probeErgebnis
	naechste  map[string]time.Time
	zuletztAn map[string]time.Time // wer hat MICH zuletzt geprueft (Ratenbegrenzung)
}

var proben = &probenStand{ergebnis: map[string]probeErgebnis{}, naechste: map[string]time.Time{}, zuletztAn: map[string]time.Time{}}

func (p *probenStand) merke(addr string, e probeErgebnis) {
	p.mu.Lock()
	p.ergebnis[addr] = e
	p.mu.Unlock()
}

// geprueft: hat addr die letzte Probe bestanden (und ist sie frisch)?
// unbekannt=true, wenn es keine frische Probe gibt.
func (p *probenStand) geprueft(addr string, jetzt time.Time) (bestanden, unbekannt bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.ergebnis[addr]
	if !ok || jetzt.Sub(e.zeit) > probeGueltig {
		return false, true
	}
	return e.bestanden, false
}

// faellig: soll addr jetzt geprueft werden? Zufaellige Abstaende, damit sich
// niemand auf den Zeitpunkt einstellen kann.
func (p *probenStand) faellig(addr string, jetzt time.Time, r *rand.Rand) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	t, ok := p.naechste[addr]
	if ok && jetzt.Before(t) {
		return false
	}
	abstand := time.Duration(float64(probeAbstand) * (0.5 + r.Float64()))
	if !ok {
		abstand = time.Duration(float64(abstand) * r.Float64()) // erste Probe bald
	}
	p.naechste[addr] = jetzt.Add(abstand)
	return ok
}

// annehmen: Ratenbegrenzung fuer eingehende Proben -- hoechstens eine je
// Pruefer alle 30 s, sonst waere die Probe ein Hebel, einen Knoten
// auszulasten.
func (p *probenStand) annehmen(von string, jetzt time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if t, ok := p.zuletztAn[von]; ok && jetzt.Sub(t) < 30*time.Second {
		return false
	}
	p.zuletztAn[von] = jetzt
	return true
}

// ProbenStand fuer /health.
func ProbenStand() map[string]interface{} {
	proben.mu.Lock()
	defer proben.mu.Unlock()
	out := map[string]interface{}{}
	for a, e := range proben.ergebnis {
		out[a] = map[string]interface{}{"bestanden": e.bestanden, "vor_s": int(time.Since(e.zeit).Seconds()), "dauer_ms": e.dauer.Milliseconds()}
	}
	return map[string]interface{}{"ergebnisse": out, "anzahl_je_probe": probeAnzahl,
		"bedeutung": "Stufe 2: Leistungsprobe der anderen Mitglieder. Nur wer besteht, bekommt im naechsten Term Konten zugeteilt."}
}
