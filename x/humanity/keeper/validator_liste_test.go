package keeper

import (
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestPruefeValidatorListe(t *testing.T) {
	a := map[string]bool{"0x" + strings.Repeat("1", 40): true, "0x" + strings.Repeat("2", 40): true}
	b := map[string]bool{"0x" + strings.Repeat("2", 40): true, "0x" + strings.Repeat("1", 40): true}
	h := validatorListeHash(a)
	if h != validatorListeHash(b) || h != komiteeHash(a) {
		t.Fatal("Hash haengt an der Reihenfolge")
	}
	if validatorListeHash(nil) != "" {
		t.Fatal("leere Liste hat einen Hash")
	}
	for _, soll := range []string{"", "  ", h, " " + strings.ToUpper(h) + " "} {
		if err := pruefeValidatorListe(a, soll); err != nil {
			t.Fatalf("%q: %v", soll, err)
		}
	}
	c := map[string]bool{"0x" + strings.Repeat("1", 40): true, "0x" + strings.Repeat("3", 40): true}
	for name, x := range map[string]struct {
		fest map[string]bool
		soll string
	}{
		"andere Liste": {c, h},
		"Teilmenge":    {map[string]bool{"0x" + strings.Repeat("1", 40): true}, h},
		"leere Liste":  {nil, h},
		"kein Hash":    {a, "abc"},
		"Hash mit 0x":  {a, "0x" + h[2:]},
		"Hash zu lang": {a, h + "0"},
	} {
		if err := pruefeValidatorListe(x.fest, x.soll); err == nil {
			t.Fatalf("%s: angenommen", name)
		}
	}
}

// Die Liste steht im signierten Inhalt: wer sie unterwegs aendert, bricht
// die Signatur.
func TestLeitNachricht_ListeSigniert(t *testing.T) {
	k, _ := crypto.GenerateKey()
	dag := &BlockDAG{signingKey: k}
	jetzt := time.Now()
	m := LeitNachricht{Art: leitArtHallo, Von: strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex()), ZeitMs: jetzt.UnixMilli(), Liste: strings.Repeat("a", 64)}
	dag.signiereLeitNachricht(&m)
	if err := pruefeLeitNachricht(m, jetzt); err != nil {
		t.Fatal(err)
	}
	m.Liste = strings.Repeat("b", 64)
	if err := pruefeLeitNachricht(m, jetzt); err == nil {
		t.Fatal("geaenderte Liste angenommen")
	}
}

// Vier Validatoren, einer mit anderer Liste (er meldet sich wie ein neu
// hinzukommender): die drei mit gleicher Liste bilden die Leitung; der
// vierte wird nie aufgenommen, nie Leiter, und seine Nachrichten werden
// gezaehlt abgewiesen. Ohne die Pruefung wird er nach AufnahmeFrist
// Mitglied (TestSatz_WachsenVonEinsAufFuenf).
func TestLeitung_AndereListeHoertNiemand(t *testing.T) {
	for _, fremd := range []string{strings.Repeat("e", 64), ""} {
		n := neuesSimNetz(t, 4, 31, satzTestKonfig())
		n.genesis = []string{n.knoten[0].addr, n.knoten[1].addr, n.knoten[2].addr}
		for i := range n.knoten {
			n.baue(i)
		}
		gleich := strings.Repeat("d", 64)
		for i, k := range n.knoten {
			if i == 3 {
				k.l.SetzeListe(fremd)
			} else {
				k.l.SetzeListe(gleich)
			}
		}
		n.laufe(60 * time.Second)
		drei := strings.Join(n.genesis, ",")
		for i := 0; i < 3; i++ {
			if m := strings.Join(n.mitglieder(i), ","); m != drei {
				t.Fatalf("Liste %q: Satz bei %d ist %s, erwartet %s", fremd, i, m, drei)
			}
			for _, m := range n.mitglieder(i) {
				if m == n.knoten[3].addr {
					t.Fatalf("Liste %q: Knoten mit anderer Liste ist Mitglied bei %d", fremd, i)
				}
			}
			if a, _ := n.knoten[i].l.Leiter(); a == n.knoten[3].addr {
				t.Fatalf("Liste %q: Knoten mit anderer Liste ist Leiter bei %d", fremd, i)
			}
		}
		if i := n.annehmender(); i == 3 {
			t.Fatalf("Liste %q: Knoten mit anderer Liste nimmt an", fremd)
		}
		// Ausfall des Leiters: die beiden anderen waehlen ohne den vierten.
		leiter, _ := n.knoten[0].l.Leiter()
		for _, k := range n.knoten {
			if k.addr == leiter {
				k.an = false
			}
		}
		n.laufe(60 * time.Second)
		if i := n.annehmender(); i < 0 || i == 3 {
			t.Fatalf("Liste %q: nach Ausfall nimmt %d an, erwartet einer der beiden mit gleicher Liste", fremd, i)
		}
		st := n.knoten[0].l.Stand(n.uhr(0))
		if st["liste_abgewiesen"].(uint64) == 0 {
			t.Fatalf("Liste %q: keine Abweisung gezaehlt", fremd)
		}
	}
}

// Ein Fremder (nicht im Satz, nicht zugelassen) mit anderer Liste zaehlt
// nicht als Abweisung -- der Zaehler bleibt ein Signal fuer Betreiber.
func TestLeitung_FremderZaehltNicht(t *testing.T) {
	n := neuesSimNetz(t, 3, 5, satzTestKonfig())
	n.zugelassen = map[string]bool{n.knoten[0].addr: true, n.knoten[1].addr: true, n.knoten[2].addr: true}
	for i := range n.knoten {
		n.baue(i)
		n.knoten[i].l.SetzeListe(strings.Repeat("d", 64))
	}
	l := n.knoten[0].l
	for i := 0; i < 50; i++ {
		l.Empfange(LeitNachricht{Art: leitArtHallo, Von: "0x" + strings.Repeat("9", 40), Liste: strings.Repeat("e", 64)}, n.uhr(0))
	}
	if st := l.Stand(n.uhr(0)); st["liste_abgewiesen"].(uint64) != 0 {
		t.Fatalf("Fremder gezaehlt: %v", st["liste_abgewiesen"])
	}
	l.Empfange(LeitNachricht{Art: leitArtHallo, Von: n.knoten[1].addr, Liste: strings.Repeat("e", 64)}, n.uhr(0))
	if st := l.Stand(n.uhr(0)); st["liste_abgewiesen"].(uint64) != 1 {
		t.Fatalf("Mitglied mit anderer Liste nicht gezaehlt: %v", st["liste_abgewiesen"])
	}
}
