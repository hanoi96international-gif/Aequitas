package keeper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"testing"
)

// Stichprobe v2 im Dienst (quanten_s_abschnitt_dienst.go, DEZENTRAL.md 16):
// der Validator meldet je Annahmestelle Teil-Hash und Zaehlung, die Oeffnung
// gibt es erst nach Bindung und Wahl auf der Kette -- mit dem Salz, aus dem
// die Stelle die Bindung v2 nachrechnet.

func (a *qsaWelt) auftragV2(x int) QuantenSAbschnittAnfrage {
	q := a.auftragAbschnitt(x)
	q.ProbeArt = 2
	q.PraegerSig = personalSign(a.w.t, a.w.keys[a.w.praeger], quantenSAbschnittAuftragNachricht(q))
	return q
}

// praeparierenV2: alle Validatoren mit ProbeArt 2; danach Antworten, x, Blaetter, Wurzel.
func (a *qsaWelt) praeparierenV2(t *testing.T) {
	t.Helper()
	w := a.w
	a.antw, a.x, a.blatt = nil, make([]int, a.n), make([][32]byte, a.n)
	for x := 1; x <= len(w.val); x++ {
		ans, err := a.d[x-1].abschnittPraeparieren(a.auftragV2(x))
		if err != nil {
			t.Fatalf("Validator %d praepariert v2 nicht: %v", x, err)
		}
		if verifyPersonalSign(quantenSAbschnittAntwortNachricht(ans), ans.Sig, w.val[x-1]) != nil {
			t.Fatalf("Validator %d: Antwort v2 nicht von ihm", x)
		}
		if len(ans.Teile) != 2 || len(ans.Zaehlung) != 2 {
			t.Fatalf("Validator %d: Teile %v, Zaehlung %v", x, ans.Teile, ans.Zaehlung)
		}
		xs, err := quantenSFuerInhaberinOeffnen(w.keys[w.anna], ans.FuerInhaberin, a.K, ans.Von, ans.Bis)
		if err != nil {
			t.Fatal(err)
		}
		copy(a.x[ans.Von:], xs)
		for i, h := range ans.Blaetter {
			a.blatt[ans.Von+i], _ = quantenWHex32(h)
		}
		a.antw = append(a.antw, ans)
	}
	a.wurzel = quantenWMerkleWurzel(a.blatt)
}

// probenV2: die Bindungen aus den Teil-Hashes der Antworten (was das Werkzeug rechnet).
func (a *qsaWelt) probenV2(t *testing.T) [2][32]byte {
	t.Helper()
	var out [2][32]byte
	for b := 0; b < 2; b++ {
		var teile [][32]byte
		for _, ans := range a.antw {
			h, ok := quantenWHex32(ans.Teile[b])
			if !ok {
				t.Fatal("Teil-Hash")
			}
			teile = append(teile, h)
		}
		var err error
		if out[b], err = quantenSProbeHashV2(a.K, b, teile); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// ausgebenV2: Ausgabe mit den Bindungen v2 auf der Kette (die Kette prueft
// sie bei der Ausgabe nicht -- erst beim Anmelden), dann Bindung, ohne Wahl.
func (a *qsaWelt) ausgebenV2(t *testing.T, proben [2][32]byte) {
	t.Helper()
	w, s := a.w, a.s
	s.messbasen, s.x = a.y, a.x
	ph := []string{hex.EncodeToString(proben[0][:]), hex.EncodeToString(proben[1][:])}
	wurzel := hex.EncodeToString(a.wurzel[:])
	q := &QuantenWAngaben{K: a.K, N: a.n, T: 3, Wurzel: wurzel, Validatoren: w.val, Deckung: 10_000_000, Tol: qsTol,
		Inhaber: w.anna, Annahme: []string{s.stelle[0], s.stelle[1]}, Proben: ph}
	for x := 1; x <= len(w.val); x++ {
		r := a.bestaetigenAnfrage(x)
		r.Proben = ph
		r.PraegerSig = personalSign(w.t, w.keys[w.praeger], quantenWDienstAuftrag(r))
		anker, sig, err := a.d[x-1].bestaetigen(r)
		if err != nil || anker != "" {
			t.Fatalf("Validator %d bestaetigt v2 nicht: %v", x, err)
		}
		q.Bestaetigungen = append(q.Bestaetigungen, sig)
	}
	q.WalletSig = personalSign(w.t, w.keys[w.praeger], quantenWSAusgabeNachricht(w.praeger, a.K, a.n, 3, wurzel, w.val, ph, q.Annahme, w.anna, 10_000_000, a.zeit, qsTol))
	w.annehmen(Transaction{Type: quantenWSAusgebenTxArt, Wallet: w.praeger, DistributionAt: a.zeit, QuantenW: q}, a.zeit+1)
	w.annehmen(s.bindenTx(w.anna), a.zeit+2)
}

func TestQuantenSProbeV2Dienst_Gutfall(t *testing.T) {
	a := neueQsaWelt(t, 140)
	w, s := a.w, a.s
	v := len(w.val)
	a.praeparierenV2(t)
	// Derselbe Auftrag noch einmal (vor der Ausgabe): dieselben Teile und Zaehlungen.
	again, err := a.d[1].abschnittPraeparieren(a.auftragV2(2))
	if err != nil || again.Teile[0] != a.antw[1].Teile[0] || again.Teile[1] != a.antw[1].Teile[1] ||
		again.Zaehlung[0] != a.antw[1].Zaehlung[0] || again.Zaehlung[1] != a.antw[1].Zaehlung[1] {
		t.Fatalf("Wiederholung: %v", err)
	}
	// Vor der Ausgabe: keine Oeffnung, fuer keinen Validator und keine Stelle.
	for x := 1; x <= v; x++ {
		for b := 0; b < 2; b++ {
			if _, err := a.d[x-1].oeffnungFuerStelle(a.K, b); err == nil {
				t.Fatalf("Validator %d oeffnet Stelle %d vor der Ausgabe", x, b)
			}
		}
	}
	proben := a.probenV2(t)
	a.ausgebenV2(t, proben)
	// Gebunden, aber nicht gewaehlt: noch keine Oeffnung.
	for x := 1; x <= v; x++ {
		if _, err := a.d[x-1].oeffnungFuerStelle(a.K, 1); err == nil {
			t.Fatalf("Validator %d oeffnet vor der Wahl", x)
		}
	}
	w.annehmen(s.waehlenTx(w.anna, "1"), a.zeit+3)
	// Gewaehlt, aber noch nicht final (Checkpoint unter der Hoehe, bei der die
	// Bindung zuerst gesehen wurde): keine Oeffnung.
	a.hoehe.Store(50)
	a.final.Store(40)
	for x := 1; x <= v; x++ {
		if _, err := a.d[x-1].oeffnungFuerStelle(a.K, 0); err == nil {
			t.Fatalf("Validator %d oeffnet vor der Finalitaet der Bindung", x)
		}
	}
	// Der Takt haelt die Sichtung fest (sonst raeumte sichtRaeumen sie weg und
	// die Oeffnung wuerde nie reif).
	tok := w.cs.quantenWJetzt().Token[a.K]
	sicht := quantenSBindungSicht(a.K, tok.token)
	for x := 1; x <= v; x++ {
		gesehen := map[string]bool{}
		a.d[x-1].taktS(context.Background(), a.K, a.d[x-1].eintrag(a.K), tok.token, a.zeit+4, gesehen)
		if !gesehen[sicht] {
			t.Fatalf("Validator %d: Takt sichtet die Bindung nicht", x)
		}
		a.d[x-1].sichtRaeumen(gesehen)
	}
	a.final.Store(50)
	// Nach der Wahl: jede Stelle laedt ihre Stichprobe samt Salzen und rechnet
	// die Bindung v2 nach; die Zaehlungen stimmen mit dem, was sie prueft.
	for b := 0; b < 2; b++ {
		alle := map[int]*big.Int{}
		var salze [][32]byte
		for x := 1; x <= v; x++ {
			c, err := a.d[x-1].oeffnungFuerStelle(a.K, b)
			if err != nil {
				t.Fatalf("Validator %d, Stelle %d nach der Wahl: %v", x, b, err)
			}
			von, bis, _ := quantenSAbschnitt(a.n, v, x)
			if _, err := quantenSOeffnungEntschluesseln(w.keys[s.stelle[b]], c, a.K, b, x, von, bis); err == nil {
				t.Fatal("Oeffnung v2 wie bisher gelesen (ohne Salz)")
			}
			m, salz, err := quantenSOeffnungEntschluesselnV2(w.keys[s.stelle[b]], c, a.K, b, x, von, bis)
			if err != nil {
				t.Fatal(err)
			}
			var dort []int
			for i, val := range m {
				alle[i] = val
				dort = append(dort, i)
			}
			if h := quantenSProbeTeilHash(a.K, b, x, salz, dort); hex.EncodeToString(h[:]) != a.antw[x-1].Teile[b] {
				t.Fatalf("Validator %d, Stelle %d: Teil-Hash passt nicht zur Oeffnung", x, b)
			}
			salze = append(salze, salz)
		}
		if _, err := quantenSOeffnungLaden(b, a.K, a.n, a.wurzel, a.blatt, alle, 64); err != nil {
			t.Fatalf("Stelle %d laedt nicht: %v", b, err)
		}
		var probe []int
		tr, fe := 0, 0
		for i, val := range alle {
			probe = append(probe, i)
			u, tw := quantenWZerlegen(val)
			if a.y[i] == u {
				tr++
				if a.x[i] != tw {
					fe++
				}
			}
		}
		sort.Ints(probe)
		if got, err := quantenSProbeV2Pruefen(a.K, b, a.n, v, probe, salze); err != nil || got != proben[b] {
			t.Fatalf("Stelle %d: Bindung v2 nachgerechnet %x (%v)", b, got, err)
		}
		zt, zf := 0, 0
		for _, ans := range a.antw {
			zt, zf = zt+ans.Zaehlung[b][0], zf+ans.Zaehlung[b][1]
		}
		if zt != tr || zf != fe || tr == 0 {
			t.Fatalf("Stelle %d: Zaehlung %d/%d, geprueft %d/%d", b, zt, zf, tr, fe)
		}
	}
}

func TestQuantenSProbeV2Dienst_Missbrauch(t *testing.T) {
	a := neueQsaWelt(t, 140)
	w := a.w
	// Andere Arten als 0 und 2; Art nachtraeglich geaendert (Unterschrift alt).
	for _, art := range []int{1, 3, -2} {
		q := a.auftragAbschnitt(2)
		q.ProbeArt = art
		q.PraegerSig = personalSign(w.t, w.keys[w.praeger], quantenSAbschnittAuftragNachricht(q))
		if _, err := a.d[1].abschnittPraeparieren(q); err == nil {
			t.Fatalf("ProbeArt %d angenommen", art)
		}
	}
	q := a.auftragAbschnitt(2)
	q.ProbeArt = 2 // ohne neu zu unterschreiben
	if _, err := a.d[1].abschnittPraeparieren(q); err == nil {
		t.Fatal("ProbeArt ohne Unterschrift des Ausgebers angenommen")
	}
	if a.mess[1].aufruf.Load() != 0 {
		t.Fatal("ein abgewiesener Auftrag hat die Messquelle benutzt")
	}
	// Die Unterschrift der Antwort deckt Teile und Zaehlung.
	ans, err := a.d[1].abschnittPraeparieren(a.auftragV2(2))
	if err != nil {
		t.Fatal(err)
	}
	for name, f := range map[string]func(QuantenSAbschnittAntwort) QuantenSAbschnittAntwort{
		"Teil-Hash": func(x QuantenSAbschnittAntwort) QuantenSAbschnittAntwort {
			x.Teile = []string{x.Teile[1], x.Teile[0]}
			return x
		},
		"Zaehlung": func(x QuantenSAbschnittAntwort) QuantenSAbschnittAntwort {
			x.Zaehlung = [][2]int{{0, 0}, x.Zaehlung[1]}
			return x
		},
		"ohne Teile": func(x QuantenSAbschnittAntwort) QuantenSAbschnittAntwort { x.Teile, x.Zaehlung = nil, nil; return x },
		"Fehler kleiner": func(x QuantenSAbschnittAntwort) QuantenSAbschnittAntwort {
			x.Zaehlung = [][2]int{x.Zaehlung[0], {x.Zaehlung[1][0], -1}}
			return x
		},
	} {
		g := f(ans)
		if verifyPersonalSign(quantenSAbschnittAntwortNachricht(g), g.Sig, w.val[1]) == nil {
			t.Fatalf("%s geaendert, Unterschrift gilt noch", name)
		}
	}
	// Bisherige Auftraege (ProbeArt 0): unveraendert -- keine Teile, Oeffnung
	// schon vor der Ausgabe (die Kette kennt v2 noch nicht).
	b := neueQsaWelt(t, 140)
	alt, err := b.d[0].abschnittPraeparieren(b.auftragAbschnitt(1))
	if err != nil || alt.Teile != nil || alt.Zaehlung != nil {
		t.Fatalf("v1-Antwort: %v %v %v", alt.Teile, alt.Zaehlung, err)
	}
	c1, err := b.d[0].oeffnungFuerStelle(b.K, 0)
	if err != nil {
		t.Fatalf("v1-Oeffnung vor der Ausgabe: %v", err)
	}
	// Eine Oeffnung ohne Salz (v1) liest die Stelle v2 nicht: ohne Salz
	// koennte sie beim Anmelden nichts nachweisen.
	von1, bis1, _ := quantenSAbschnitt(b.n, len(b.w.val), 1)
	if _, _, err := quantenSOeffnungEntschluesselnV2(b.w.keys[b.s.stelle[0]], c1, b.K, 0, 1, von1, bis1); err == nil {
		t.Fatal("v1-Oeffnung als v2 gelesen (ohne Salz)")
	}
	if _, err := quantenSOeffnungEntschluesseln(b.w.keys[b.s.stelle[0]], c1, b.K, 0, 1, von1, bis1); err != nil {
		t.Fatalf("v1-Oeffnung wie bisher: %v", err)
	}
	q0 := b.auftragAbschnitt(1)
	if quantenSAbschnittAuftragNachricht(q0) != quantenSAbschnittAuftragNachrichtV1Test(q0) {
		t.Fatal("v1-Auftragsnachricht geaendert")
	}
	hf := sha256.Sum256([]byte(alt.FuerInhaberin))
	altText := fmt.Sprintf("%s|chain:%d|%s|%d|%d|%d|%s|%x|%s|%s", "aequitas-quanten-s-abschnitt-antwort-v1", eingefroreneChainID, alt.K, alt.X, alt.Von, alt.Bis,
		quantenSHashListe(alt.Blaetter), hf, quantenSHashListe(alt.Buendel), quantenSHashListe(alt.BuendelSig))
	if quantenSAbschnittAntwortNachricht(alt) != altText {
		t.Fatal("v1-Antwortnachricht geaendert")
	}
}

// quantenSAbschnittAuftragNachrichtV1Test: der Wortlaut vor der Stichprobe v2
// (#585), unabhaengig nachgebaut -- bisherige Unterschriften bleiben gueltig.
func quantenSAbschnittAuftragNachrichtV1Test(q QuantenSAbschnittAnfrage) string {
	hb := sha256.Sum256([]byte(q.Basen))
	return fmt.Sprintf("%s|chain:%d|%s|%s|%d|%d|%d|%d|%s|%s|%x|%s|%s|%s|%s|%d", "aequitas-quanten-s-abschnitt-auftrag-v1", eingefroreneChainID,
		strings.ToLower(q.Praeger), q.K, q.N, q.T, q.Tol, q.X, strings.Join(q.Validatoren, ","), strings.Join(q.Schluessel, ","), hb,
		strings.ToLower(q.Inhaberin), q.InhaberinSchluessel, strings.Join(q.Stellen, ","), strings.Join(q.StellenSchluessel, ","), q.Zeit)
}
