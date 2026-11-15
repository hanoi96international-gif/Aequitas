package keeper

import (
	"crypto/mldsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
)

// Audit B2: Pruefer-Stimmen hybrid (pruefer_pq.go). Missbrauchstests: ohne
// PQ-Teil nach dem Stichtag, fremder Schluessel, Schluessel ohne Zusage,
// Unterschrift ueber einen anderen Satz (andere Art, anderer Betroffener,
// andere Zeit, anderer Mensch), abgeschnitten, uebergross, Grossschrift,
// kaputter PQ-Teil VOR dem Stichtag.

type prueferPQSchluessel struct {
	sk  *mldsa.PrivateKey
	pub string
}

func neuerPrueferPQ(t *testing.T) prueferPQSchluessel {
	t.Helper()
	sk, err := mldsa.GenerateKey(mldsa.MLDSA65())
	if err != nil {
		t.Fatal(err)
	}
	return prueferPQSchluessel{sk: sk, pub: hex.EncodeToString(sk.PublicKey().Bytes())}
}

func (k prueferPQSchluessel) hash() string {
	roh, _ := hex.DecodeString(k.pub)
	h := sha256.Sum256(roh)
	return hex.EncodeToString(h[:])
}

func (k prueferPQSchluessel) unterschreiben(t *testing.T, msg string) string {
	t.Helper()
	sig, err := k.sk.Sign(rand.Reader, []byte(msg), nil)
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(sig)
}

// zusageEintragen: Mensch m hinterlegt den Hash von k (Stufe 2).
func zusageEintragen(cs *ChainState, m string, k prueferPQSchluessel) {
	cs.umzugReg.Store(cs.umzugRegisterJetzt().mitPQZusage(pqZusage{Wallet: m, Hash: k.hash(), Zeit: 1}))
}

// pqStimme: Stimme von Mensch "von" mit PQ-Teil ueber den Satz der Stimme.
func (f *vrFall) pqStimme(von int, k prueferPQSchluessel, art, betroffener string, zeit int64) VerifierStimme {
	s := f.stimme(von, art, betroffener, zeit)
	satz := verifierNachricht(art, betroffener)("", zeit)
	s.PQ = &PrueferPQ{Pub: k.pub, Sig: k.unterschreiben(f.t, prueferPQNachricht(f.m(von), satz))}
	return s
}

func prueferPQStichtag(t *testing.T, ab int64) {
	t.Helper()
	vorher := prueferPQAbOverride.Load()
	prueferPQAbOverride.Store(ab)
	t.Cleanup(func() { prueferPQAbOverride.Store(vorher) })
}

func TestPrueferPQ_SchlafendOhneWirkung(t *testing.T) {
	if prueferPQAbUnix != 1<<63-1 {
		t.Fatal("prueferPQAbUnix muss schlafen, bis App und Sammelweg der Freischaltung fertig sind")
	}
	// Ohne PQ-Teil: Stimme byte-gleich wie vorher (omitempty).
	f := neuerVrFall(t)
	s := f.stimme(0, verifierArtAufnahme, f.m(1), vrAb+10)
	roh, _ := json.Marshal(s)
	if strings.Contains(string(roh), `"pq"`) {
		t.Fatalf("Stimme ohne PQ-Teil enthaelt das Feld: %s", roh)
	}
}

func TestPrueferPQ_NachDemStichtagPflicht(t *testing.T) {
	f := neuerVrFall(t)
	z := vrAb + 1000
	prueferPQStichtag(t, z)
	k := neuerPrueferPQ(t)
	zusageEintragen(f.cs, f.m(0), k)

	// Ohne PQ-Teil: abgewiesen.
	if neu := f.anwenden(f.tx(verifierArtAufnahme, f.m(1), z, 0), z); neu["verifier_stimme_pq"] != 1 {
		t.Fatalf("Stimme ohne PQ-Teil nach dem Stichtag angenommen: %v", neu)
	}
	// Mit gueltigem PQ-Teil: aufgenommen.
	tx := Transaction{Type: verifierTxArt, Wallet: f.m(1), DistributionAt: z,
		Verifier: &VerifierZulassung{Art: verifierArtAufnahme, Stimmen: []VerifierStimme{f.pqStimme(0, k, verifierArtAufnahme, f.m(1), z)}}}
	if neu := f.anwenden(tx, z); len(neu) != 0 {
		t.Fatalf("gueltige hybride Stimme abgewiesen: %v", neu)
	}
	if !f.ist(1, z+erzeugerFrist) {
		t.Fatal("nicht aufgenommen")
	}
}

func TestPrueferPQ_Missbrauch(t *testing.T) {
	f := neuerVrFall(t)
	z := vrAb + 1000
	prueferPQStichtag(t, z)
	k := neuerPrueferPQ(t)
	fremd := neuerPrueferPQ(t)
	zusageEintragen(f.cs, f.m(0), k)
	zusageEintragen(f.cs, f.m(2), fremd) // ein anderer Mensch mit eigener Zusage

	satz := verifierNachricht(verifierArtAufnahme, f.m(1))("", z)
	gut := f.pqStimme(0, k, verifierArtAufnahme, f.m(1), z)
	faelle := map[string]func(s *VerifierStimme){
		"fremder Schluessel (Zusage eines anderen)": func(s *VerifierStimme) {
			s.PQ = &PrueferPQ{Pub: fremd.pub, Sig: fremd.unterschreiben(t, prueferPQNachricht(f.m(0), satz))}
		},
		"frischer Schluessel ohne Zusage": func(s *VerifierStimme) {
			x := neuerPrueferPQ(t)
			s.PQ = &PrueferPQ{Pub: x.pub, Sig: x.unterschreiben(t, prueferPQNachricht(f.m(0), satz))}
		},
		"andere Art": func(s *VerifierStimme) {
			s.PQ.Sig = k.unterschreiben(t, prueferPQNachricht(f.m(0), verifierNachricht(verifierArtEntzug, f.m(1))("", z)))
		},
		"anderer Betroffener": func(s *VerifierStimme) {
			s.PQ.Sig = k.unterschreiben(t, prueferPQNachricht(f.m(0), verifierNachricht(verifierArtAufnahme, f.m(3))("", z)))
		},
		"andere Zeit": func(s *VerifierStimme) {
			s.PQ.Sig = k.unterschreiben(t, prueferPQNachricht(f.m(0), verifierNachricht(verifierArtAufnahme, f.m(1))("", z-1)))
		},
		"anderer Mensch im Satz": func(s *VerifierStimme) {
			s.PQ.Sig = k.unterschreiben(t, prueferPQNachricht(f.m(2), satz))
		},
		"ohne Domaene (nackter Satz)": func(s *VerifierStimme) {
			s.PQ.Sig = k.unterschreiben(t, satz)
		},
		"Unterschrift abgeschnitten": func(s *VerifierStimme) { s.PQ.Sig = s.PQ.Sig[:len(s.PQ.Sig)-2] },
		"Unterschrift zu lang":       func(s *VerifierStimme) { s.PQ.Sig += "00" },
		"Schluessel abgeschnitten":   func(s *VerifierStimme) { s.PQ.Pub = s.PQ.Pub[:len(s.PQ.Pub)-2] },
		"Grossschrift":               func(s *VerifierStimme) { s.PQ.Pub = strings.ToUpper(s.PQ.Pub) },
		"Bit gekippt": func(s *VerifierStimme) {
			b := []byte(s.PQ.Sig)
			if b[10] == '0' {
				b[10] = '1'
			} else {
				b[10] = '0'
			}
			s.PQ.Sig = string(b)
		},
		"leerer PQ-Teil": func(s *VerifierStimme) { s.PQ = &PrueferPQ{} },
	}
	for name, aendern := range faelle {
		s := gut
		pq := *gut.PQ
		s.PQ = &pq
		aendern(&s)
		tx := Transaction{Type: verifierTxArt, Wallet: f.m(1), DistributionAt: z,
			Verifier: &VerifierZulassung{Art: verifierArtAufnahme, Stimmen: []VerifierStimme{s}}}
		if neu := f.anwenden(tx, z); neu["verifier_stimme_pq"] != 1 {
			t.Errorf("%s: nicht abgewiesen (%v)", name, neu)
		}
		if f.ist(1, z+erzeugerFrist) {
			t.Fatalf("%s: trotzdem aufgenommen", name)
		}
	}
}

// Vor dem Stichtag ist der PQ-Teil freiwillig -- aber ein kaputter wird nie
// still uebergangen.
func TestPrueferPQ_VorDemStichtagStreng(t *testing.T) {
	f := neuerVrFall(t)
	z := vrAb + 1000
	k := neuerPrueferPQ(t)
	zusageEintragen(f.cs, f.m(0), k)
	s := f.pqStimme(0, k, verifierArtAufnahme, f.m(1), z)
	s.PQ.Sig = k.unterschreiben(t, "etwas anderes")
	tx := Transaction{Type: verifierTxArt, Wallet: f.m(1), DistributionAt: z,
		Verifier: &VerifierZulassung{Art: verifierArtAufnahme, Stimmen: []VerifierStimme{s}}}
	if neu := f.anwenden(tx, z); neu["verifier_stimme_pq"] != 1 {
		t.Fatalf("kaputter PQ-Teil vor dem Stichtag uebergangen: %v", neu)
	}
	// Ohne PQ-Teil vor dem Stichtag: wie bisher.
	f.aufnehmen(1, z+1, 0)
	// Gueltiger PQ-Teil vor dem Stichtag: angenommen.
	z2 := z + 1 + erzeugerFrist + 10
	tx2 := Transaction{Type: verifierTxArt, Wallet: f.m(2), DistributionAt: z2,
		Verifier: &VerifierZulassung{Art: verifierArtAufnahme, Stimmen: []VerifierStimme{
			f.pqStimme(0, k, verifierArtAufnahme, f.m(2), z2), f.stimme(1, verifierArtAufnahme, f.m(2), z2)}}}
	if neu := f.anwenden(tx2, z2); len(neu) != 0 {
		t.Fatalf("gemischte Stimmen vor dem Stichtag abgewiesen: %v", neu)
	}
}

// Annahme (Sammelweg): dieselbe Regel, mit der Uhrzeit statt issued_at.
func TestPrueferPQ_AnnahmeSammelweg(t *testing.T) {
	f := neuerVrFall(t)
	z := vrAb + 1000
	prueferPQStichtag(t, z)
	k := neuerPrueferPQ(t)
	zusageEintragen(f.cs, f.m(0), k)
	reg := f.cs.verifierRegisterJetzt()
	ohne := f.stimme(0, verifierArtAufnahme, f.m(1), z)
	if err := verifierStimmeVorpruefen(verifierArtAufnahme, f.m(1), &ohne, reg, z, f.cs.prueferZusageVon); err == nil {
		t.Fatal("Sammelweg nimmt Stimme ohne PQ-Teil nach dem Stichtag an")
	}
	mit := f.pqStimme(0, k, verifierArtAufnahme, f.m(1), z)
	mit.PQ.Pub = " " + strings.ToUpper(mit.PQ.Pub) + " " // Annahme bringt in Form
	if err := verifierStimmeVorpruefen(verifierArtAufnahme, f.m(1), &mit, reg, z, f.cs.prueferZusageVon); err != nil {
		t.Fatalf("gueltige hybride Stimme im Sammelweg abgewiesen: %v", err)
	}
	fremd := f.pqStimme(0, neuerPrueferPQ(t), verifierArtAufnahme, f.m(1), z)
	if err := verifierStimmeVorpruefen(verifierArtAufnahme, f.m(1), &fremd, reg, z, f.cs.prueferZusageVon); err == nil {
		t.Fatal("Sammelweg nimmt fremden PQ-Schluessel an")
	}
}

// Ein Koerper mit einer hybriden Stimme passt in die Grenze des Sammelwegs.
func TestPrueferPQ_KoerperGrenze(t *testing.T) {
	f := neuerVrFall(t)
	k := neuerPrueferPQ(t)
	s := f.pqStimme(0, k, verifierArtAufnahme, f.m(1), vrAb+10)
	roh, err := json.Marshal(map[string]interface{}{"art": verifierArtAufnahme, "wallet": f.m(1), "stimme": s})
	if err != nil {
		t.Fatal(err)
	}
	if len(roh) > verifierStimmeKoerperMax {
		t.Fatalf("eine hybride Stimme (%d Byte) passt nicht in verifierStimmeKoerperMax (%d)", len(roh), verifierStimmeKoerperMax)
	}
	if 2*len(roh) < verifierStimmeKoerperMax {
		// Die Grenze soll eng bleiben: hoechstens eine Stimme je Anfrage.
		t.Fatalf("verifierStimmeKoerperMax (%d) ist mehr als doppelt so gross wie eine Stimme (%d)", verifierStimmeKoerperMax, len(roh))
	}
}

// Quantengeld: die Freischaltung eines Satzes braucht nach dem Stichtag
// hybride Stimmen; ein PQ-Teil ueber einen anderen Satz (andere Ordnung,
// anderes ab) zaehlt nicht. So kann auch, wer Ed25519 und secp256k1 von
// zwei Dritteln der Pruefer ableitet, keinen Quantengeld-Satz freischalten.
func TestPrueferPQ_QuantengeldFreischaltung(t *testing.T) {
	f := neuerVrFall(t)
	f.aufnehmen(1, vrAb+100, 0)
	qgAktiv(t)
	zeit := vrAb + 100 + erzeugerFrist + 1000
	ab := zeit + quantenFreischaltungVorlauf
	n := qgSatzN.String()
	prueferPQStichtag(t, zeit)
	k0, k1 := neuerPrueferPQ(t), neuerPrueferPQ(t)
	zusageEintragen(f.cs, f.m(0), k0)
	zusageEintragen(f.cs, f.m(1), k1)

	// Nur Ed25519: abgewiesen.
	if u := qgUrteil(t, f.cs, qgFreischaltung(f, "isogenie-8192", n, ab, zeit, 0, 1), zeit); u.regel != "quanten_stimme_pq" {
		t.Fatalf("Freischaltung ohne PQ-Teil nach dem Stichtag: %q (%s)", u.regel, u.text)
	}
	mitPQ := func(ordnung string, abX int64) Transaction {
		tx := qgFreischaltung(f, "isogenie-8192", n, ab, zeit, 0, 1)
		for i, k := range []prueferPQSchluessel{k0, k1} {
			s := &tx.Quanten.Stimmen[i]
			satz := quantenFreischaltungNachricht("isogenie-8192", ordnung, abX)("", s.Zeit)
			s.PQ = &PrueferPQ{Pub: k.pub, Sig: k.unterschreiben(t, prueferPQNachricht(f.m(i), satz))}
		}
		return tx
	}
	if u := qgUrteil(t, f.cs, mitPQ(n, ab), zeit); u.regel != "" {
		t.Fatalf("gueltige hybride Freischaltung abgewiesen: %q (%s)", u.regel, u.text)
	}
	if u := qgUrteil(t, f.cs, mitPQ(n, ab+1), zeit); u.regel != "quanten_stimme_pq" {
		t.Fatalf("PQ-Teil ueber ein anderes ab angenommen: %q", u.regel)
	}
	if u := qgUrteil(t, f.cs, mitPQ(new(big.Int).Add(qgSatzN, big.NewInt(2)).String(), ab), zeit); u.regel != "quanten_stimme_pq" {
		t.Fatalf("PQ-Teil ueber eine andere Ordnung angenommen: %q", u.regel)
	}
	// Vertauschte Schluessel (Mensch 0 zeigt den von Mensch 1): abgewiesen.
	tx := mitPQ(n, ab)
	tx.Quanten.Stimmen[0].PQ, tx.Quanten.Stimmen[1].PQ = tx.Quanten.Stimmen[1].PQ, tx.Quanten.Stimmen[0].PQ
	if u := qgUrteil(t, f.cs, tx, zeit); u.regel != "quanten_stimme_pq" {
		t.Fatalf("vertauschte PQ-Teile angenommen: %q", u.regel)
	}
}
