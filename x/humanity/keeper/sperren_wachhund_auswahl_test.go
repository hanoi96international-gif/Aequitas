package keeper

import (
	"strings"
	"testing"
)

// Der Abzug vom 23.09.2026 (C1, 10.000/s Annahme) nannte keinen Halter:
// AddPeerBlock hielt replayMu und wartete selbst auf eine Mutex -- und alles,
// dessen oberster Rahmen eine Mutex ist, fiel aus der Auswahl.
func TestSperrWachhund_HalterDerSelbstWartetBleibtSichtbar(t *testing.T) {
	abzug := strings.Join([]string{
		"goroutine 7 [sync.Mutex.Lock]:\n" +
			"sync.runtime_SemacquireMutex(0xc0, 0x0, 0x1)\n" +
			"github.com/x/humanity/keeper.(*ChainState).lockExclusive(...)\n" +
			"github.com/x/humanity/keeper.(*BlockDAG).replayInCanonicalOrder(0xc0)\n" +
			"github.com/x/humanity/keeper.(*BlockDAG).AddPeerBlock(0xc0)",
		"goroutine 9 [sync.RWMutex.RLock]:\n" +
			"sync.runtime_SemacquireRWMutex(0xc0, 0x0, 0x1)\n" +
			"github.com/x/humanity/keeper.(*BlockDAG).Height(0xc0)",
		"goroutine 11 [select]:\n" +
			"net/http.(*persistConn).roundTrip(0xc0)\n" +
			"github.com/x/humanity/keeper.(*BlockDAG).doSyncOnce(0xc0)",
	}, "\n\n")
	aus := goroutinenAuswerten(abzug)
	text := strings.Join(aus, "\n")
	if !strings.Contains(text, "HAELT UND WARTET? goroutine 7") {
		t.Errorf("AddPeerBlock, das replayMu haelt und selbst auf eine Mutex wartet, fehlt:\n%s", text)
	}
	if !strings.Contains(text, "HALTER? goroutine 11") {
		t.Errorf("der Sync im Netz fehlt als Verdaechtiger:\n%s", text)
	}
	if strings.Contains(text, "goroutine 9 ") {
		t.Errorf("ein blosser Leser, der auf die Sperre wartet, ist Rauschen:\n%s", text)
	}
	if len(aus) < 2 || !strings.HasPrefix(aus[0], "HAELT UND WARTET?") && !strings.HasPrefix(aus[0], "HALTER?") {
		t.Errorf("Verdaechtige gehoeren an den Anfang:\n%s", text)
	}
}
