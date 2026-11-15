package keeper

// Audit B2 (14.11.2026): Pruefer-Stimmen hybrid -- Ed25519 UND ML-DSA-65.
//
// # WARUM
//
// Jede Stimme eines Pruefers -- Aufnahme, Austritt, Entzug
// (verifier_register.go) und die Freischaltung eines Quantengeld-Satzes
// (quanten_geld.go) -- ist bisher nur eine Ed25519-Bescheinigung seines
// Coordinators, gebunden an den Menschen mit einer secp256k1-Unterschrift.
// Beides leitet ein Quantenrechner aus oeffentlichen Daten ab. Wer das fuer
// zwei Drittel der Pruefer tut, nimmt beliebige Pruefer auf, wirft die
// ehrlichen hinaus und schaltet einen Quantengeld-Satz frei.
//
// # WAS DAZUKOMMT
//
// Die Stimme traegt zusaetzlich den ML-DSA-65-Schluessel des MENSCHEN (nicht
// des Coordinators) und dessen Unterschrift ueber
//
//	aequitas-pruefer-pq-v1|chain:1926|<mensch>|<satz der Ed25519-Stimme>
//
// Der Schluessel ist der aus der Seed-Phrase (App, lib/pq.ts) -- derselbe,
// den der Mensch mit der PQ-Zusage (pq_zusage.go, Stufe 2) hinterlegt hat.
// Gilt nur, wenn SHA-256(Schluessel) GENAU die Zusage dieses Menschen ist.
// Die Zusage ist nicht aenderbar; ein Angreifer, der secp256k1 bricht, kann
// sie nicht ersetzen. Ein neues Register braucht es nicht.
//
// # REGELN (jeder Knoten prueft selbst, beim Nachspielen und bei der Annahme)
//
//   - Ab prueferPQAbUnix (issued_at der Transaktion) braucht JEDE Stimme den
//     PQ-Teil. Schlafend (MaxInt64), bis die App ihn liefert und eine echte
//     Stimme damit durchging; dann Stichtag mit Waechter wie Stufe 1.
//   - Vorher ist er freiwillig -- aber wenn er da ist, wird er streng
//     geprueft: ein falscher PQ-Teil macht die Stimme ungueltig (nie
//     "ignorieren"), damit der Weg vor dem Stichtag schon echt laeuft.
//   - Feste Groessen: Schluessel genau 1.952, Unterschrift genau 3.309 Byte
//     (als Hex, klein), sonst ab vor jeder Rechnung.
//   - Billig vor teuer: Form, dann Zusage (Speicher), dann ML-DSA.
//
// # GRENZEN
//
// Eine Stimme wird rund 10,5 KB groesser. Eine Abstimmung ueber den
// Sammelweg (/api/verifier/stimme) nimmt je Anfrage EINE Stimme
// (verifierStimmeKoerperMax). Eine Freischaltung traegt alle Stimmen in
// einem Koerper; bei 171 Stimmen (zwei Drittel von 256) sind das ~2 MB --
// mehr als quantenKoerperMax und die Weiterleitung zum Leiter tragen. Dafuer
// gibt es den Sammelweg (quanten_sammeln.go, /api/quanten/stimme): eine
// Stimme je Anfrage, der Leiter baut die Freischaltung selbst. Der Endpunkt
// mit allen Stimmen bleibt fuer kleine Register; zu gross wird abgewiesen,
// nie abgeschnitten.

import (
	"crypto/mldsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
)

const (
	prueferPQDomaene = "aequitas-pruefer-pq-v1"
	// prueferPQAbUnix: 2026-11-15 21:00 UTC (Entscheidung des Betreibers vom
	// 15.11.2026, "Stichtage alle heute"). Die App liefert den PQ-Teil seit
	// 1.21.0 (veroeffentlicht 15.11.2026 16:31 UTC); auf der neuen Kette
	// (Genesis 15.11.2026 16:00 UTC) gibt es noch keine Stimme ohne ihn. Ab
	// dann zaehlt eine Pruefer-Stimme nur mit der ML-DSA-Unterschrift des
	// Menschen, deren Schluessel seine PQ-Zusage ist. Waechter:
	// .github/neustart/pruefer-pq-nicht-verpasst.sh.
	prueferPQAbUnix int64 = 1794776400
)

// prueferPQAbOverride: nur fuer Tests (0 = Konstante gilt).
var prueferPQAbOverride atomic.Int64

func prueferPQAb() int64 {
	if v := prueferPQAbOverride.Load(); v != 0 {
		return v
	}
	return prueferPQAbUnix
}

// prueferPQPflicht: braucht eine Stimme zu einer Transaktion mit dieser
// issued_at den PQ-Teil?
func prueferPQPflicht(issuedAt int64) bool { return issuedAt >= prueferPQAb() }

// PrueferPQ: der Teil der Stimme, den der Mensch selbst mit ML-DSA-65
// unterschreibt (Hex, klein, ohne 0x).
type PrueferPQ struct {
	Pub string `json:"pub"`
	Sig string `json:"sig"`
}

// prueferPQNachricht: was der Mensch mit ML-DSA-65 unterschreibt -- den Satz
// seiner Ed25519-Stimme, an ihn und an die Kette gebunden.
func prueferPQNachricht(mensch, satz string) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s", prueferPQDomaene, eingefroreneChainID, strings.ToLower(strings.TrimSpace(mensch)), satz)
}

var errPrueferPQ = errors.New("PQ-Teil der Stimme")

// prueferPQPruefen: erfuellt die Stimme s die Regel? satz ist genau der Satz,
// den ihre Ed25519-Bescheinigung unterschreibt; zusage liefert den Hash der
// PQ-Zusage eines Menschen (umzugRegister.pqZusage).
func prueferPQPruefen(s *VerifierStimme, satz string, pflicht bool, zusage func(mensch string) (string, bool)) error {
	if s.PQ == nil {
		if pflicht {
			return fmt.Errorf("%w fehlt (Pflicht ab %d)", errPrueferPQ, prueferPQAb())
		}
		return nil
	}
	p := s.PQ
	if len(p.Pub) != pqPubHex || !kleinHex(p.Pub) || len(p.Sig) != pqSigHex || !kleinHex(p.Sig) {
		return fmt.Errorf("%w: Schluessel %d/%d, Unterschrift %d/%d Hex-Zeichen", errPrueferPQ, len(p.Pub), pqPubHex, len(p.Sig), pqSigHex)
	}
	mensch := strings.ToLower(strings.TrimSpace(s.Bescheinigung.Mensch))
	h, ok := zusage(mensch)
	if !ok {
		return fmt.Errorf("%w: %s hat keine PQ-Zusage", errPrueferPQ, kurzAdresse(mensch))
	}
	roh, err := hex.DecodeString(p.Pub)
	if err != nil {
		return fmt.Errorf("%w: Schluessel unlesbar", errPrueferPQ)
	}
	if sum := sha256.Sum256(roh); hex.EncodeToString(sum[:]) != h {
		return fmt.Errorf("%w: Schluessel ist nicht die Zusage von %s", errPrueferPQ, kurzAdresse(mensch))
	}
	pk, err := mldsa.NewPublicKey(mldsa.MLDSA65(), roh)
	if err != nil {
		return fmt.Errorf("%w: Schluessel unlesbar", errPrueferPQ)
	}
	sig, err := hex.DecodeString(p.Sig)
	if err != nil {
		return fmt.Errorf("%w: Unterschrift unlesbar", errPrueferPQ)
	}
	if mldsa.Verify(pk, []byte(prueferPQNachricht(mensch, satz)), sig, nil) != nil {
		return fmt.Errorf("%w: Unterschrift passt nicht", errPrueferPQ)
	}
	return nil
}

// prueferZusageVon: Zusage-Hash eines Menschen aus dem aktuellen
// Umzugs-Register (ohne Sperre lesbar, beim Nachspielen der Stand dieses
// Blocks).
func (cs *ChainState) prueferZusageVon(mensch string) (string, bool) {
	z, ok := cs.umzugRegisterJetzt().pqZusage(mensch)
	return z.Hash, ok
}

// prueferPQStatus: fuer /api/status ("pruefer_pq"). ab fehlt, solange die
// Regel schlaeft -- der Waechter (.github/neustart/pruefer-pq-nicht-verpasst.sh)
// liest das so.
func prueferPQStatus() map[string]interface{} {
	m := map[string]interface{}{}
	if ab := prueferPQAb(); ab != math.MaxInt64 {
		m["ab"] = ab
	}
	return m
}
