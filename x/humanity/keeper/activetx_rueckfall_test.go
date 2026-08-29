package keeper

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestKeineNeueDbExecAufrufstelle haelt die Migration vom 29.08.2026 fest.
//
// Am Ende jenes Tages gab es im ganzen Paket keine cs.dbExec()-Aufrufstelle
// mehr -- jeder Schreibvorgang geht durch dbExecCtx(ctx). Das war nicht
// Selbstzweck: solange auch nur eine Stelle die Transaktion implizit aus
// cs.activeTx zieht, kann dieses Feld nicht entfernt und echte
// Nebenlaeufigkeit nicht eingeschaltet werden. Genau daran haengt die
// gemessene Serialisierung -- 74,7 % der Wartezeit in runAtomicWithOutbox,
// das die globale Sperre ueber die ganze DB-Transaktion haelt.
//
// Eine einzige neue cs.dbExec()-Zeile macht diese Arbeit rueckgaengig, ohne
// dass irgendetwas rot wird: sie kompiliert, sie funktioniert, sie ist nur
// wieder implizit. Deshalb dieser Test.
//
// Wer eine braucht, benutzt dbExecCtx(ctx) -- und wenn an der Stelle wirklich
// keine Transaktion gelten soll, dann sichtbar mit context.Background(), so
// wie restoreFromRollbackLockedCtx und loadPenaltyCacheLocked es tun. Der
// Unterschied ist nicht kosmetisch: bei restoreFromRollbackLockedCtx WAERE das
// Durchreichen der Transaktion ein Fehler, weil sie zu dem Zeitpunkt schon
// zurueckgerollt ist.
func TestKeineNeueDbExecAufrufstelle(t *testing.T) {
	dateien, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	// Die Definition selbst ist die eine erlaubte Fundstelle.
	definition := regexp.MustCompile(`func \(cs \*ChainState\) dbExec\(\)`)
	aufruf := regexp.MustCompile(`cs\.dbExec\(\)`)

	var treffer []string
	for _, d := range dateien {
		if strings.HasSuffix(d, "_test.go") {
			continue
		}
		roh, err := os.ReadFile(d)
		if err != nil {
			t.Fatal(err)
		}
		inhalt := strings.ReplaceAll(string(roh), "\r\n", "\n")
		for i, zeile := range strings.Split(inhalt, "\n") {
			getrimmt := strings.TrimSpace(zeile)
			// Kommentare duerfen den Namen nennen -- sie tun es an vielen
			// Stellen, um die Geschichte zu erklaeren.
			if strings.HasPrefix(getrimmt, "//") {
				continue
			}
			if definition.MatchString(zeile) {
				continue
			}
			if aufruf.MatchString(zeile) {
				treffer = append(treffer, d+":"+itoa(i+1)+": "+getrimmt)
			}
		}
	}

	if len(treffer) > 0 {
		t.Fatalf("cs.dbExec() ist an %d Stelle(n) zurueck:\n  %s\n\n"+
			"Diese Schreibweise zieht die Transaktion implizit aus cs.activeTx -- einem einzigen\n"+
			"ChainState-weiten Feld, das echte Nebenlaeufigkeit unmoeglich macht und deshalb am\n"+
			"29.08.2026 aus allen Aufrufstellen entfernt wurde.\n"+
			"Benutze dbExecCtx(ctx). Soll an der Stelle bewusst KEINE Transaktion gelten, dann\n"+
			"sichtbar mit context.Background() -- siehe restoreFromRollbackLockedCtx, wo das\n"+
			"Durchreichen der Transaktion sogar ein Fehler waere.",
			len(treffer), strings.Join(treffer, "\n  "))
	}
}

// TestRueckfallzaehlerBeginntBeiNull stellt sicher, dass die Zahl aussagekraeftig
// bleibt: sie soll den Rueckfall zaehlen, nicht irgendetwas anderes.
func TestRueckfallzaehlerBeginntBeiNull(t *testing.T) {
	vorher := activeTxRueckfaelle.Load()
	stand := ActiveTxRueckfallStand()
	if stand["rueckfaelle"].(int64) != vorher {
		t.Fatalf("Stand meldet %v, Zaehler steht auf %d", stand["rueckfaelle"], vorher)
	}
	notiereActiveTxRueckfall()
	if activeTxRueckfaelle.Load() != vorher+1 {
		t.Fatal("notiereActiveTxRueckfall hat nicht gezaehlt")
	}
	if ActiveTxRueckfallStand()["activetx_entfernbar"].(bool) {
		t.Fatal("bei einem Rueckfall darf activetx_entfernbar nicht wahr sein")
	}
	activeTxRueckfaelle.Store(vorher)
}
