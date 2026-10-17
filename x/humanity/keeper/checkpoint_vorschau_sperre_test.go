package keeper

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// Die Checkpoint-Vorschau in ProduceBlock sucht OHNE dag.mu
// (checkpointVorschauOhneDAGSperre). Diese Tests rufen sie mit denselben
// Sperren wie ProduceBlock (replayMu, dann dag.mu) und greifen ueber den
// Kopfleser mitten in die Suche ein.

// vorschauWieProduceBlock: die Sperren wie in ProduceBlock; prueft, dass
// dag.mu bei der Rueckkehr wieder gehalten ist.
func vorschauWieProduceBlock(t *testing.T, dag *BlockDAG, txs []Transaction, eltern []string, zeit int64) (checkpointVorschauErgebnis, string) {
	t.Helper()
	dag.replayMu.Lock()
	defer dag.replayMu.Unlock()
	dag.mu.Lock()
	defer dag.mu.Unlock()
	v, abbruch := dag.checkpointVorschauOhneDAGSperre(txs, eltern, zeit)
	if dag.mu.TryRLock() {
		dag.mu.RUnlock()
		t.Error("dag.mu nach der Vorschau nicht wieder gehalten")
	}
	return v, abbruch
}

// leserMitHaken: der Kopfleser des Falls; haken laeuft einmal, beim ersten
// Lesen -- also mitten in der Suche.
func leserMitHaken(t *testing.T, f *vorschlagFall, haken func()) {
	t.Helper()
	alt := checkpointReplayLeser
	var einmal sync.Once
	checkpointReplayLeser = func(sqlExecutor) kopfLeser {
		innen := f.koepfe.leser()
		return func(h []string) (map[string]blockKopf, error) {
			einmal.Do(haken)
			return innen(h)
		}
	}
	t.Cleanup(func() { checkpointReplayLeser = alt })
}

func letztenCheckpointSetzen(t *testing.T, cs *ChainState, wert string) {
	t.Helper()
	if _, err := cs.db.Exec(`DELETE FROM chain_config WHERE key = $1`, letzterCheckpointSchluessel); err != nil {
		t.Fatal(err)
	}
	if wert != "" {
		if _, err := cs.db.Exec(`INSERT INTO chain_config (key, value) VALUES ($1, $2)`, letzterCheckpointSchluessel, wert); err != nil {
			t.Fatal(err)
		}
	}
}

// Waehrend der Suche ist dag.mu frei (Leser UND Schreiber kommen dran),
// replayMu bleibt gehalten; das Ergebnis ist byte-gleich zur Vorschau unter
// der Sperre -- ohne und mit bisherigem letztem Checkpoint.
func TestCheckpointVorschau_OhneDAGSperre_RealDB(t *testing.T) {
	f := neuerVorschlagFall(t, true)
	checkpointTxAn(t)
	cs := f.dag.state
	t.Cleanup(func() { letztenCheckpointSetzen(t, cs, "") })
	b0, b1 := f.blocks[0], f.blocks[1]
	tx := neueCheckpointTx(f.zertFuer(t, b1))
	txs := []Transaction{{Type: "transfer"}, tx}
	eltern, zeit := []string{b1.Hash}, b1.Timestamp+200

	for name, vorher := range map[string]string{
		"ohne letzten": "",
		"mit letztem":  letzterCheckpointWert(f.vorschlag(t, b0)),
	} {
		t.Run(name, func(t *testing.T) {
			letztenCheckpointSetzen(t, cs, vorher)
			var gesucht, schreibFrei, leseFrei, replayFrei bool
			leserMitHaken(t, f, func() {
				gesucht = true
				if f.dag.mu.TryLock() {
					schreibFrei = true
					f.dag.mu.Unlock()
				}
				if f.dag.mu.TryRLock() {
					leseFrei = true
					f.dag.mu.RUnlock()
				}
				if f.dag.replayMu.TryLock() {
					replayFrei = true
					f.dag.replayMu.Unlock()
				}
			})
			v, abbruch := vorschauWieProduceBlock(t, f.dag, txs, eltern, zeit)
			if !gesucht {
				t.Fatal("die Suche hat den Kopfleser nicht benutzt")
			}
			if !schreibFrei || !leseFrei {
				t.Fatalf("dag.mu waehrend der Suche gehalten (schreiben frei %v, lesen frei %v)", schreibFrei, leseFrei)
			}
			if replayFrei {
				t.Fatal("replayMu waehrend der Suche frei -- ein Nachspielen koennte den Zustand aendern")
			}
			if abbruch != "" || !v.geaendert || v.unklar || v.index != 1 || v.basis != vorher {
				t.Fatalf("ohne Stoerung: %+v, Abbruch %q", v, abbruch)
			}
			// Byte-gleich zur Vorschau, wie sie bisher (unter der Sperre) lief.
			checkpointReplayLeser = func(sqlExecutor) kopfLeser { return f.koepfe.leser() }
			direkt := f.dag.checkpointVorschau(txs, eltern, zeit)
			if direkt.neu != v.neu || direkt.geaendert != v.geaendert || v.neu != letzterCheckpointWert(tx.Checkpoint.KettenCheckpoint) {
				t.Fatalf("anderes Ergebnis: %+v gegen %+v", v, direkt)
			}
			if cs.stateRootMitCheckpoint(v.neu) != cs.stateRootMitCheckpoint(direkt.neu) {
				t.Fatal("andere StateRoot")
			}
			if got := cs.getConfigValueDB(letzterCheckpointSchluessel); got != vorher {
				t.Fatalf("Vorschau hat den letzten Checkpoint gespeichert: %q", got)
			}
		})
	}
}

// Missbrauch/Wettlauf: aendert sich waehrend der Suche (dag.mu frei), worauf
// Vorschau oder Block bauen, wird nichts mit dem alten Ergebnis gebaut --
// kein Block in diesem Takt (die Checkpoint-Transaktion bleibt liegen).
func TestCheckpointVorschau_AenderungWaehrendDerSuche_RealDB(t *testing.T) {
	f := neuerVorschlagFall(t, true)
	checkpointTxAn(t)
	cs := f.dag.state
	t.Cleanup(func() { letztenCheckpointSetzen(t, cs, "") })
	b0, b1 := f.blocks[0], f.blocks[1]
	tx := neueCheckpointTx(f.zertFuer(t, b1))
	txs := []Transaction{tx}
	eltern, zeit := []string{b1.Hash}, b1.Timestamp+200

	faelle := []struct {
		name   string
		haken  func(t *testing.T)
		danach func()
		grund  string
	}{
		{
			// Ein anderer schreibt den letzten Checkpoint, waehrend die
			// Vorschau von "keiner" ausgeht: ihr Ergebnis ist veraltet.
			name: "letzter Checkpoint",
			haken: func(t *testing.T) {
				if _, err := cs.db.Exec(`INSERT INTO chain_config (key, value) VALUES ($1, $2)`,
					letzterCheckpointSchluessel, letzterCheckpointWert(f.vorschlag(t, b0))); err != nil {
					t.Error(err)
				}
			},
			grund: "letzter Checkpoint geaendert",
		},
		{
			// Eine Verteilung (WithBlockProductionPaused) laeuft in der Luecke.
			name: "Produktionspause",
			haken: func(t *testing.T) {
				fertig := make(chan struct{})
				go func() { f.dag.WithBlockProductionPaused(func() {}); close(fertig) }()
				select {
				case <-fertig:
				case <-time.After(2 * time.Second):
					t.Error("WithBlockProductionPaused kam waehrend der Suche nicht an dag.mu")
				}
			},
			grund: "Produktionspause",
		},
		{
			// Ein Elternteil wird in dag.blocks ersetzt.
			name: "Elternteil",
			haken: func(t *testing.T) {
				if !f.dag.mu.TryLock() {
					t.Error("dag.mu gehalten")
					return
				}
				kopie := *b1
				f.dag.blocks[b1.Hash] = &kopie
				f.dag.mu.Unlock()
			},
			danach: func() { f.dag.mu.Lock(); f.dag.blocks[b1.Hash] = b1; f.dag.mu.Unlock() },
			grund:  "Elternteil",
		},
		{
			name:   "Neusynchronisation",
			haken:  func(*testing.T) { f.dag.resyncInProgress.Store(true) },
			danach: func() { f.dag.resyncInProgress.Store(false) },
			grund:  "Neusynchronisation",
		},
		{
			name: "degradiert",
			haken: func(*testing.T) {
				f.dag.degradedMu.Lock()
				f.dag.degradedReason = "Test"
				f.dag.degradedMu.Unlock()
			},
			danach: func() { f.dag.degradedMu.Lock(); f.dag.degradedReason = ""; f.dag.degradedMu.Unlock() },
			grund:  "degradiert",
		},
	}
	for _, fall := range faelle {
		t.Run(fall.name, func(t *testing.T) {
			letztenCheckpointSetzen(t, cs, "")
			if fall.danach != nil {
				t.Cleanup(fall.danach)
			}
			leserMitHaken(t, f, func() { fall.haken(t) })
			v, abbruch := vorschauWieProduceBlock(t, f.dag, txs, eltern, zeit)
			if !strings.Contains(abbruch, fall.grund) {
				t.Fatalf("nicht abgebrochen (Abbruch %q, Ergebnis %+v)", abbruch, v)
			}
		})
	}

	// Ohne Checkpoint-Transaktion: keine Suche, keine Freigabe, kein Abbruch.
	f.dag.mu.Lock()
	v, abbruch := f.dag.checkpointVorschauOhneDAGSperre([]Transaction{{Type: "transfer"}}, eltern, zeit)
	f.dag.mu.Unlock()
	if v.index != -1 || abbruch != "" {
		t.Fatalf("ohne Checkpoint: %+v %q", v, abbruch)
	}
}
