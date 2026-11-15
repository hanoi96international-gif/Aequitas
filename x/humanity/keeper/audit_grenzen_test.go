package keeper

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// B3: Wallet-Sperren verschwinden, sobald niemand sie haelt, und schliessen
// sich weiterhin gegenseitig aus (kein zweiter Mutex fuer dieselbe Wallet,
// solange der erste gehalten wird).
func TestWalletSperre_RaeumtAufUndSchliesstAus(t *testing.T) {
	vorher := walletSperrenAnzahl()
	for i := 0; i < 1000; i++ {
		lockWallet(fmt.Sprintf("0x%040x", 0xc000+i))()
	}
	if n := walletSperrenAnzahl(); n != vorher {
		t.Fatalf("Sperren bleiben liegen: %d -> %d", vorher, n)
	}

	const w = "0x00000000000000000000000000000000000000d1"
	var drin, maxDrin int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			frei := lockWallet(w)
			mu.Lock()
			drin++
			if drin > maxDrin {
				maxDrin = drin
			}
			mu.Unlock()
			time.Sleep(200 * time.Microsecond)
			mu.Lock()
			drin--
			mu.Unlock()
			frei()
			frei() // doppelte Freigabe darf nichts kaputt machen
		}()
	}
	wg.Wait()
	if maxDrin != 1 {
		t.Fatalf("%d Registrierungen derselben Wallet gleichzeitig", maxDrin)
	}
	if n := walletSperrenAnzahl(); n != vorher {
		t.Fatalf("nach Gleichlauf %d Sperren uebrig (vorher %d)", n, vorher)
	}
}

// B9: lastRegistrationAttempt waechst nicht ueber die Grenze; ist sie mit
// frischen Eintraegen voll, wird abgewiesen (fail-closed), abgelaufene
// werden entfernt.
func TestRegistrationRateLimit_Begrenzt(t *testing.T) {
	registrationRateLimitMu.Lock()
	gesichert := lastRegistrationAttempt
	lastRegistrationAttempt = map[string]int64{}
	registrationRateLimitMu.Unlock()
	defer func() {
		registrationRateLimitMu.Lock()
		lastRegistrationAttempt = gesichert
		registrationRateLimitMu.Unlock()
	}()
	for i := 0; i < registrationAttemptsHoechstens; i++ {
		if registrationRateLimited(fmt.Sprintf("0x%040x", i)) {
			t.Fatalf("Eintrag %d abgewiesen", i)
		}
	}
	if !registrationRateLimited("0x00000000000000000000000000000000ffffffff") {
		t.Fatal("volle Karte nimmt weitere Absender an")
	}
	if n := len(lastRegistrationAttempt); n > registrationAttemptsHoechstens {
		t.Fatalf("Karte ueber der Grenze: %d", n)
	}
	// alle Eintraege altern lassen: dann wird aufgeraeumt und angenommen
	registrationRateLimitMu.Lock()
	alt := time.Now().Add(-2 * registrationRateLimitWindow).UnixNano()
	for k := range lastRegistrationAttempt {
		lastRegistrationAttempt[k] = alt
	}
	registrationRateLimitMu.Unlock()
	if registrationRateLimited("0x00000000000000000000000000000000ffffffff") {
		t.Fatal("nach Ablauf weiter abgewiesen")
	}
	if n := len(lastRegistrationAttempt); n != 1 {
		t.Fatalf("abgelaufene Eintraege nicht entfernt: %d", n)
	}
}

// B13: Text vom Aufrufer wird fuers Log gekuerzt; %q maskiert Umbrueche.
func TestLogKurz(t *testing.T) {
	boese := "eth_x\n[RPC] ✓ Transfer 1000 AEQ" + strings.Repeat("ä", 200)
	k := logKurz(boese, 64)
	if len(k) > 64+len("…") {
		t.Fatalf("zu lang: %d", len(k))
	}
	if z := fmt.Sprintf("%q", k); strings.Contains(z, "\n") {
		t.Fatalf("Zeilenumbruch im Log: %s", z)
	}
	if logKurz("kurz", 64) != "kurz" {
		t.Fatal("kurzer Text veraendert")
	}
}

// B14: /mpc/budget nennt keinen Pfad und liest die Datei nicht bei jeder
// Anfrage neu.
func TestMPCBudget_OhnePfadUndGepuffert(t *testing.T) {
	dir := t.TempDir()
	geheim := dir + "/geheimer-ort/tripel.bin"
	t.Setenv("MPC_TRIPLE_FILE", geheim)
	mpcBudgetPuffer.Lock()
	mpcBudgetPuffer.zeit = time.Time{}
	mpcBudgetPuffer.Unlock()
	a := &APIServer{}
	jetzt := time.Now()
	b := a.mpcTripleBudgetGepuffert(512, jetzt)
	if strings.Contains(b.Fehler, "geheimer-ort") || strings.Contains(b.Datei, "geheimer-ort") {
		t.Fatalf("Pfad in der Antwort: %+v", b)
	}
	if b.Datei != "gesetzt" || b.Fehler == "" {
		t.Fatalf("unerwartet: %+v", b)
	}
	// innerhalb der Frische: dieselbe Antwort, auch wenn sich die Umgebung aendert
	t.Setenv("MPC_TRIPLE_FILE", "")
	if b2 := a.mpcTripleBudgetGepuffert(512, jetzt.Add(10*time.Second)); b2 != b {
		t.Fatalf("Puffer nicht benutzt: %+v", b2)
	}
	if b3 := a.mpcTripleBudgetGepuffert(512, jetzt.Add(mpcBudgetFrische+time.Second)); b3.Datei != "" {
		t.Fatalf("nach Ablauf nicht neu berechnet: %+v", b3)
	}
}
