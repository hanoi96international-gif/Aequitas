package keeper

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"time"
)

// GET /api/quanten-w/stand: der oeffentliche Ueberblick ueber Variante W/S
// fuer Website und App -- wie viele Noten und Token leben, wie viele beendet
// sind (eingeloest, tot, Urteil offen) und die zuletzt beendeten. Nur Lesen,
// nichts Geheimes (dasselbe steht einzeln in /api/quanten-w/note).
//
// Grenzen: lesenBegrenzt je Absender; die Antwort wird hoechstens alle
// quantenWStandFrische neu gerechnet (die Zaehlung laeuft ueber das ganze
// Register unter cs.mu.RLock, also nie je Anfrage); "zuletzt" hat hoechstens
// quantenWStandZuletzt Eintraege.
const (
	quantenWStandFrische = 15 * time.Second
	quantenWStandZuletzt = 5
)

type quantenWStandEnde struct {
	K      string `json:"k"`
	Urteil string `json:"urteil"`
	Ende   int64  `json:"ende"`
}

type quantenWStandAntwort struct {
	Aktiv       bool                `json:"aktiv"`
	NotenW      int                 `json:"noten_w"`
	TokenS      int                 `json:"token_s"`
	Beendet     int                 `json:"beendet"`
	Eingeloest  int                 `json:"eingeloest"`
	Tot         int                 `json:"tot"`
	UrteilOffen int                 `json:"urteil_offen"`
	TopfMikro   int64               `json:"topf_mikro"`
	Zuletzt     []quantenWStandEnde `json:"zuletzt"`
	Stand       int64               `json:"stand"`
}

var quantenWStandZwischen struct {
	sync.Mutex
	cs      *ChainState // fuer welchen Zustand (Tests haben mehrere)
	zeit    time.Time
	antwort *quantenWStandAntwort
}

// quantenWStandRechnen zaehlt das Register (unter RLock, wie quantenWNoteAnsicht).
func (cs *ChainState) quantenWStandRechnen(ctx context.Context, jetzt time.Time) *quantenWStandAntwort {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	reg := cs.quantenWJetzt()
	a := &quantenWStandAntwort{
		Aktiv:     quantenWAktiv(jetzt.Unix()),
		NotenW:    len(reg.Noten),
		TokenS:    len(reg.Token),
		Beendet:   len(reg.Tote),
		TopfMikro: cs.quantenWMikroLocked(ctx, quantenWTopfAddr),
		Zuletzt:   []quantenWStandEnde{},
		Stand:     jetzt.Unix(),
	}
	alle := make([]quantenWStandEnde, 0, len(reg.Enden))
	for k := range reg.Tote {
		en, ok := reg.Enden[k]
		switch {
		case !ok:
			a.UrteilOffen++
			continue
		case en.Urteil == quantenWUrteilEingeloest:
			a.Eingeloest++
		case en.Urteil == quantenWUrteilTot:
			a.Tot++
		default:
			// Unbekanntes Urteil: nicht als Erfolg zaehlen und nicht zeigen.
			a.UrteilOffen++
			continue
		}
		alle = append(alle, quantenWStandEnde{K: k, Urteil: en.Urteil, Ende: en.Ende})
	}
	sort.Slice(alle, func(i, j int) bool {
		if alle[i].Ende != alle[j].Ende {
			return alle[i].Ende > alle[j].Ende
		}
		return alle[i].K < alle[j].K
	})
	if len(alle) > quantenWStandZuletzt {
		alle = alle[:quantenWStandZuletzt]
	}
	a.Zuletzt = append(a.Zuletzt, alle...)
	return a
}

// quantenWStand: die zwischengespeicherte Antwort, hoechstens quantenWStandFrische alt.
func (cs *ChainState) quantenWStand(ctx context.Context, jetzt time.Time) *quantenWStandAntwort {
	z := &quantenWStandZwischen
	z.Lock()
	defer z.Unlock()
	if z.antwort != nil && z.cs == cs && jetzt.Sub(z.zeit) >= 0 && jetzt.Sub(z.zeit) < quantenWStandFrische {
		return z.antwort
	}
	z.antwort = cs.quantenWStandRechnen(ctx, jetzt)
	z.zeit = jetzt
	z.cs = cs
	return z.antwort
}

func (a *APIServer) handleQuantenWStand(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		jsonError(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	json.NewEncoder(w).Encode(a.state.quantenWStand(r.Context(), time.Now()))
}
