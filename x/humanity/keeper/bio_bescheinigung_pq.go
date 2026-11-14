package keeper

import (
	"crypto/mldsa"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// DIE BESCHEINIGUNG QUANTENSICHER (docs/QUANTENSICHERHEIT.md, Stufe 1).
//
// # DIE LUECKE
//
// Die Bescheinigung (bio_bescheinigung_konsens.go) traegt Ed25519-
// Unterschriften von Coordinator und Vergleichsdienst. Wer einen dieser
// Schluessel mit einem Quantenrechner aus dem oeffentlichen Schluessel
// ableitet (Shor), bescheinigt beliebig viele "Menschen" -- je 1.000 AEQ.
// Genau dort entsteht AEQ; "ein Mensch, einmal 1.000 AEQ" haengt daran.
//
// # WAS AB pq_ab_unix GILT (genesis.json, bescheinigung)
//
// Jeder Schluessel, der fuer die Bescheinigung zaehlt (bindet oder bezeugt),
// braucht ZUSAETZLICH eine gueltige ML-DSA-65-Unterschrift (FIPS 204,
// crypto/mldsa der Standardbibliothek) seines in der Genesis hinterlegten
// Post-Quanten-Schluessels, ueber "aequitas-pq-v1|" + genau die Nachricht, die
// sein Ed25519-Schluessel unterschrieben hat. Beide muessen stimmen (hybrid):
// bricht Ed25519 durch einen Quantenrechner, haelt ML-DSA; hat ML-DSA einen
// Fehler, haelt Ed25519. Gezaehlt werden weiter Schluessel, nie Unterschriften.
//
// Feld: "pq:<ed25519-schluessel hex>:<ml-dsa-unterschrift hex>" in
// bioAttestation, wie "v3:". Der Proof-Server sieht sie nie.
//
// # WANN (deterministisch, ohne Stau)
//
// Pflicht, wenn die Bescheinigung ab dem Stichtag ausgestellt ist (b.Zeit,
// von den Pruefern unterschrieben) ODER der Block so spaet liegt, dass eine
// vor dem Stichtag ausgestellte Bescheinigung nicht mehr gueltig sein kann
// (Stichtag + bescheinigungHoechstensAlt + 1 h). So faellt keine kurz vor dem
// Stichtag angenommene Registrierung aus dem naechsten Block (der Fall, der
// beim Umzug M1 hiess), und ein Erzeuger kann mit einer rueckdatierten
// Bescheinigung nach Ablauf dieser Frist nichts mehr erreichen.
//
// Vor dem Stichtag werden PQ-Eintraege nur auf ihre Form geprueft (feste
// Laengen, hoechstens bescheinigungMaxSignaturen) -- so kann der Coordinator
// sie schon mitschicken, ohne dass ein Fehler darin Registrierungen stoppt.
//
// # GRENZEN
//
// Schluessel 1.952 Byte, Unterschrift 3.309 Byte (je doppelt als Hex). Der
// Rumpf von /prove bleibt auf 64 KiB begrenzt (readBodyLimited); damit passen
// etwa sieben PQ-Unterschriften -- mehr braucht ein Quorum heute nicht, und
// die Grenze wird fuer PQ nicht gelockert.

const (
	pqNachrichtPraefix = "aequitas-pq-v1|"
	pqFeldPraefix      = "pq:"
	pqSchluesselHex    = 2 * 1952
	pqSignaturHex      = 2 * 3309
	pqEintragLaenge    = 2*32 + 1 + pqSignaturHex
	// Nach dem Stichtag gilt eine vorher ausgestellte Bescheinigung hoechstens
	// noch so lange (Annahme bis bescheinigungHoechstensAlt, dann Block).
	pqNachlauf = bescheinigungHoechstensAlt + 3600
)

// pruefenPQ: die PQ-Felder der Genesis. Fehler halten den Start an.
func (k bescheinigungKonfig) pruefenPQ() error {
	if k.PQAbUnix < 0 {
		return errors.New("bescheinigung.pq_ab_unix negativ")
	}
	if len(k.PQ) > bescheinigungMaxSchluessel {
		return fmt.Errorf("bescheinigung.pq: hoechstens %d Schluessel", bescheinigungMaxSchluessel)
	}
	alle := map[string]bool{}
	for _, liste := range [][]string{k.Coordinator, k.Bezeuger} {
		for _, s := range liste {
			alle[s] = true
		}
	}
	for ed, pq := range k.PQ {
		if !alle[ed] {
			return fmt.Errorf("bescheinigung.pq: %.12s... ist kein Schluessel der Bescheinigung", ed)
		}
		if _, err := pqSchluesselLesen(pq); err != nil {
			return fmt.Errorf("bescheinigung.pq[%.12s...]: %w", ed, err)
		}
	}
	if k.PQAbUnix == 0 {
		return nil // schlafend; Schluessel duerfen vorbereitet sein
	}
	if k.AbUnix == 0 || k.PQAbUnix < k.AbUnix {
		return errors.New("bescheinigung.pq_ab_unix verlangt eine aktive Bescheinigung (ab_unix > 0, nicht spaeter)")
	}
	// Ohne PQ-Schluessel koennte ein Pruefer ab dem Stichtag nie mehr zaehlen:
	// eine Luecke hier hiesse nach dem Stichtag keine Registrierung mehr.
	for s := range alle {
		if _, ok := k.PQ[s]; !ok {
			return fmt.Errorf("bescheinigung.pq: Schluessel %.12s... ohne PQ-Schluessel", s)
		}
	}
	return nil
}

func pqSchluesselLesen(h string) (*mldsa.PublicKey, error) {
	if len(h) != pqSchluesselHex || !kleinHex(h) {
		return nil, fmt.Errorf("ML-DSA-65-Schluessel muss %d Hex-Zeichen (klein) haben", pqSchluesselHex)
	}
	roh, err := hex.DecodeString(h)
	if err != nil {
		return nil, err
	}
	return mldsa.NewPublicKey(mldsa.MLDSA65(), roh)
}

// pqEintragTeilen: "<ed25519 hex>:<signatur hex>" in genau fester Form.
func pqEintragTeilen(e string) (ed, sig string, ok bool) {
	if len(e) != pqEintragLaenge || e[64] != ':' {
		return "", "", false
	}
	ed, sig = e[:64], e[65:]
	if !kleinHex(ed) || !kleinHex(sig) {
		return "", "", false
	}
	return ed, sig, true
}

// pqFeldForm: immer geprueft (auch vor dem Stichtag) -- haelt das Feld klein.
func pqFeldForm(eintraege []string) error {
	if len(eintraege) > bescheinigungMaxSignaturen {
		return fmt.Errorf("bescheinigung: %d PQ-Unterschriften (hoechstens %d)", len(eintraege), bescheinigungMaxSignaturen)
	}
	for _, e := range eintraege {
		if _, _, ok := pqEintragTeilen(e); !ok {
			return errors.New("bescheinigung: PQ-Unterschrift in falscher Form")
		}
	}
	return nil
}

// pqPflicht: muss diese Bescheinigung PQ-Unterschriften tragen?
func pqPflicht(k bescheinigungKonfig, bescheinigungZeit, blockZeit int64) bool {
	ab := k.PQAbUnix
	if o := pqAbOverride.Load(); o != 0 {
		ab = o
	}
	if ab <= 0 {
		return false
	}
	return bescheinigungZeit >= ab || blockZeit >= ab+pqNachlauf
}

// pruefePQ: nach der Ed25519-Pruefung (pruefeV3Kern). bindet/bezeugt sind
// die Schluessel, die dort gezaehlt haben; jeder braucht seine ML-DSA-
// Unterschrift, und keine PQ-Unterschrift darf zu einem anderen gehoeren.
func pruefePQ(b *BioBescheinigung, k bescheinigungKonfig, bindungMsg, aussageMsg []byte, bindet, bezeugt map[string]bool, blockZeit int64) error {
	if err := pqFeldForm(b.PQ); err != nil {
		return err
	}
	if !pqPflicht(k, b.Zeit, blockZeit) {
		return nil
	}
	sigs := map[string]string{}
	for _, e := range b.PQ {
		ed, sig, _ := pqEintragTeilen(e)
		if _, doppelt := sigs[ed]; doppelt {
			return errors.New("bescheinigung: zwei PQ-Unterschriften desselben Schluessels")
		}
		sigs[ed] = sig
	}
	for ed := range sigs {
		if !bezeugt[ed] {
			return errors.New("bescheinigung: PQ-Unterschrift eines Schluessels, der nicht unterschrieben hat")
		}
	}
	for s := range bezeugt {
		pkHex, ok := k.PQ[s]
		if !ok {
			// Auch Schluessel von Pruefern aus dem Register (B1b): ohne
			// hinterlegten PQ-Schluessel zaehlen sie ab dem Stichtag nicht.
			return fmt.Errorf("bescheinigung: Schluessel %.12s... hat keinen PQ-Schluessel", s)
		}
		sigHex, ok := sigs[s]
		if !ok {
			return fmt.Errorf("bescheinigung: PQ-Unterschrift von %.12s... fehlt", s)
		}
		pk, err := pqSchluesselLesen(pkHex)
		if err != nil {
			return fmt.Errorf("bescheinigung: PQ-Schluessel %.12s... unlesbar", s)
		}
		roh, err := hex.DecodeString(sigHex)
		if err != nil {
			return errors.New("bescheinigung: PQ-Unterschrift unlesbar")
		}
		msg := aussageMsg
		if bindet[s] {
			msg = bindungMsg
		}
		if mldsa.Verify(pk, pqNachricht(msg), roh, nil) != nil {
			return fmt.Errorf("bescheinigung: PQ-Unterschrift von %.12s... ungueltig", s)
		}
	}
	return nil
}

// pqNachricht: byte-gleich zu pq_nachricht() im Coordinator und im
// Vergleichsdienst (aequitas-biometric-beta).
func pqNachricht(msg []byte) []byte {
	return append([]byte(pqNachrichtPraefix), msg...)
}

// pqEintraegeAusFeld: die "pq:"-Eintraege aus bioAttestation (Praefix
// entfernt). ok=false bei falscher Form -- dann gibt es keine Bescheinigung.
func pqEintraegeAusFeld(sigs string) ([]string, bool) {
	var aus []string
	for _, teil := range strings.Split(sigs, ",") {
		t := strings.TrimSpace(teil)
		if !strings.HasPrefix(t, pqFeldPraefix) {
			continue
		}
		aus = append(aus, strings.TrimPrefix(t, pqFeldPraefix))
	}
	if pqFeldForm(aus) != nil {
		return nil, false
	}
	return aus, true
}
