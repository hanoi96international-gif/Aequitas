package keeper

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

// QG3 (quanten_noten_tabelle.go): Noten je Zeile, StateRoot byte-gleich,
// Snapshot-Noten streng geprueft.

func qnTabelleRegister(t *testing.T) *quantenGeldRegister {
	t.Helper()
	r := (&quantenGeldRegister{}).mitSatz(quantenSatzEintrag{Name: "isogenie-8192", Ordnung: qgSatzN.String(), Ab: qgAb + 100, Seit: qgAb})
	for i, k := range []string{strings.Repeat("c", 64), strings.Repeat("a", 64), strings.Repeat("b", 64)} {
		r = r.mitNote(quantenNoteEintrag{K: k, Satz: "isogenie-8192", Von: qnAnna, Anteile: int64(1_000_000 * (i + 1))})
	}
	return r
}

// Die Definition der StateRoot bleibt: SHA-256 ueber das ganze Register als
// JSON (Saetze und Noten, Noten nach K) -- unabhaengig davon, dass die Noten
// jetzt in einer eigenen Tabelle liegen. Dazu ein fester Pruefwert: aendert
// sich das Format, faellt dieser Test, bevor eine Kette auseinanderlaeuft.
func TestQuantenNoten_StateRootUnveraendert(t *testing.T) {
	r := qnTabelleRegister(t)
	wie := struct {
		Saetze []quantenSatzEintrag `json:"saetze,omitempty"`
		Noten  []quantenNoteEintrag `json:"noten,omitempty"`
	}{r.Saetze, r.Noten}
	roh, err := json.Marshal(wie)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(roh)
	if got, want := r.stateRootTeil(), "|quanten:"+hex.EncodeToString(h[:]); got != want {
		t.Fatalf("StateRoot-Teil\n got %s\nwant %s", got, want)
	}
	if r.Noten[0].K != strings.Repeat("a", 64) || r.Noten[2].K != strings.Repeat("c", 64) {
		t.Fatal("Noten nicht nach K geordnet")
	}
	// Fester Pruefwert fuer genau dieses Register, mit dem Stand von main vor
	// QG3 (e82d70d1) gerechnet: der Umbau aendert die StateRoot nicht.
	const fest = "|quanten:8608fdde0b6534899b5d380736eb70175382a3290497f5bbd1165911d3a7514a" // mit dem Code vor QG3 gerechnet
	if got := r.stateRootTeil(); got != fest {
		t.Fatalf("Pruefwert der StateRoot hat sich geaendert:\n got %s\nwant %s", got, fest)
	}
	// Leer: byte-gleich wie vor dem Quantengeld.
	if (&quantenGeldRegister{}).stateRootTeil() != "" || neuesQuantenRegister(nil, nil).stateRootTeil() != "" {
		t.Fatal("leeres Register traegt etwas zur StateRoot bei")
	}
}

// Die Wurzel wird je Stand einmal gerechnet; eine Aenderung ist ein neuer
// Stand mit eigener Wurzel, der alte bleibt, wie er war.
func TestQuantenNoten_WurzelJeStand(t *testing.T) {
	r := qnTabelleRegister(t)
	vorher := r.stateRootTeil()
	neu := r.mitNote(quantenNoteEintrag{K: strings.Repeat("d", 64), Satz: "isogenie-8192", Von: qnAnna, Anteile: 7})
	if neu.stateRootTeil() == vorher {
		t.Fatal("neue Note, gleiche Wurzel")
	}
	if r.stateRootTeil() != vorher || len(r.Noten) != 3 {
		t.Fatal("der alte Stand hat sich veraendert")
	}
	if weg := neu.ohneNote(strings.Repeat("d", 64)); weg.stateRootTeil() != vorher {
		t.Fatal("Note wieder weg, Wurzel anders als vorher")
	}
	// Viele Leser zugleich (unter -race): alle sehen dieselbe Wurzel.
	frisch := qnTabelleRegister(t)
	var wg sync.WaitGroup
	ergebnisse := make([]string, 32)
	for i := range ergebnisse {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ergebnisse[i] = frisch.stateRootTeil()
		}(i)
	}
	wg.Wait()
	for _, e := range ergebnisse {
		if e != vorher {
			t.Fatalf("nebenlaeufig andere Wurzel: %s", e)
		}
	}
}

// In chain_config stehen nur noch die Saetze.
func TestQuantenNoten_KonfigNurSaetze(t *testing.T) {
	r := qnTabelleRegister(t)
	k := r.nurSaetze().kodiert()
	if strings.Contains(k, "noten") || !strings.Contains(k, "isogenie-8192") {
		t.Fatalf("chain_config-Wert: %s", k)
	}
	if err := pruefeSnapshotKonfig(quantenGeldKonfigSchluessel, k); err != nil {
		t.Fatalf("Saetze allein abgewiesen: %v", err)
	}
	// Das alte Format (Noten in chain_config) nimmt kein Snapshot mehr mit.
	if err := pruefeSnapshotKonfig(quantenGeldKonfigSchluessel, r.kodiert()); err == nil {
		t.Fatal("Noten in chain_config aus einem Snapshot angenommen")
	}
}

// Missbrauch: Noten eines Snapshots von einem Peer.
func TestQuantenNoten_SnapshotNotenStreng(t *testing.T) {
	gut := qnTabelleRegister(t).Noten
	if err := pruefeSnapshotQuantenNoten(gut); err != nil {
		t.Fatalf("gute Noten abgewiesen: %v", err)
	}
	if err := pruefeSnapshotQuantenNoten(nil); err != nil {
		t.Fatalf("keine Noten abgewiesen: %v", err)
	}
	kaputt := func(name string, f func(n []quantenNoteEintrag) []quantenNoteEintrag) {
		t.Helper()
		n := f(append([]quantenNoteEintrag(nil), gut...))
		if err := pruefeSnapshotQuantenNoten(n); err == nil {
			t.Errorf("%s: angenommen", name)
		}
	}
	kaputt("K kein Hash", func(n []quantenNoteEintrag) []quantenNoteEintrag { n[0].K = "zz"; return n })
	kaputt("K gross geschrieben", func(n []quantenNoteEintrag) []quantenNoteEintrag { n[0].K = strings.Repeat("A", 64); return n })
	kaputt("Von keine Adresse", func(n []quantenNoteEintrag) []quantenNoteEintrag { n[1].Von = "0x12"; return n })
	kaputt("Satz unsinnig", func(n []quantenNoteEintrag) []quantenNoteEintrag { n[1].Satz = "../x"; return n })
	kaputt("null Anteile", func(n []quantenNoteEintrag) []quantenNoteEintrag { n[2].Anteile = 0; return n })
	kaputt("negative Anteile", func(n []quantenNoteEintrag) []quantenNoteEintrag { n[2].Anteile = -5; return n })
	kaputt("zu viele Anteile", func(n []quantenNoteEintrag) []quantenNoteEintrag {
		n[2].Anteile = quantenAnteileHoechstens + 1
		return n
	})
	kaputt("Summe ueber der Grenze", func(n []quantenNoteEintrag) []quantenNoteEintrag {
		n[0].Anteile, n[1].Anteile = quantenAnteileHoechstens, 1
		return n
	})
	kaputt("ungeordnet", func(n []quantenNoteEintrag) []quantenNoteEintrag { n[0], n[1] = n[1], n[0]; return n })
	kaputt("doppelt", func(n []quantenNoteEintrag) []quantenNoteEintrag { n[1].K = n[0].K; return n })
	viele := make([]quantenNoteEintrag, quantenNotenHoechstens+1)
	if err := pruefeSnapshotQuantenNoten(viele); err == nil {
		t.Error("mehr Noten als erlaubt angenommen")
	}
}
