package keeper

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Jede Sperre von cs.mu und dag.mu wird mit defer freigegeben (OFFEN 7b).
//
// Paniert etwas zwischen Lock und einem Unlock ohne defer, bleibt die Sperre
// liegen: ein recover weiter oben (HTTP-Handler, SafeCall) faengt die Panik,
// der Knoten steht trotzdem -- und bei einem einzelnen Validator merkt das
// der Totmann nicht. Regel: direkt nach X.Lock()/X.RLock() folgt
// `defer X.Unlock()` (bzw. RUnlock) oder ein `defer func() { ... X.Unlock()
// ... }()`. Was die Regel noch nicht erfuellt, steht in sperreOhneDefer --
// die Liste darf nur kuerzer werden: eine neue Stelle schlaegt an, und eine
// behobene muss aus der Liste.
func TestSperre_ImmerMitDefer(t *testing.T) {
	gefunden := map[string]int{}
	dateien, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, datei := range dateien {
		if strings.HasSuffix(datei, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, datei, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range sperrenOhneDefer(f) {
			gefunden[datei+":"+v]++
		}
	}
	var neu, erledigt []string
	for k, n := range gefunden {
		if sperreOhneDefer[k] < n {
			neu = append(neu, k)
		}
	}
	for k, n := range sperreOhneDefer {
		if gefunden[k] < n {
			erledigt = append(erledigt, k)
		}
	}
	sort.Strings(neu)
	sort.Strings(erledigt)
	for _, k := range neu {
		t.Errorf("Sperre ohne defer: %s (%d) -- mit defer freigeben, oder zustandAendern", k, gefunden[k])
	}
	for _, k := range erledigt {
		t.Errorf("%s steht noch in sperreOhneDefer, hat aber nur noch %d Stellen -- Liste kuerzen", k, gefunden[k])
	}
}

// sperrenOhneDefer: Funktionsnamen, in denen auf cs.mu/dag.mu (auch
// X.state.mu) ein Lock/RLock nicht von einem passenden defer gefolgt wird.
func sperrenOhneDefer(f *ast.File) []string {
	var aus []string
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		name := fd.Name.Name
		if fd.Recv != nil && len(fd.Recv.List) == 1 {
			if s, ok := fd.Recv.List[0].Type.(*ast.StarExpr); ok {
				if id, ok := s.X.(*ast.Ident); ok {
					name = id.Name + "." + name
				}
			}
		}
		// Doppelt freigegeben: eine Funktion mit `defer X.Unlock()` darf X
		// nicht auch noch selbst freigeben (ausser in einer eigenen
		// Funktionsliteral) -- das waere ein nicht abfangbarer Fatal-Fehler.
		aus = append(aus, doppelteFreigaben(fd.Body, name)...)
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			var liste []ast.Stmt
			switch b := n.(type) {
			case *ast.BlockStmt:
				liste = b.List
			case *ast.CaseClause:
				liste = b.Body
			case *ast.CommClause:
				liste = b.Body
			default:
				return true
			}
			for i, st := range liste {
				mu, art := sperrAufruf(st)
				if mu == "" {
					continue
				}
				if i+1 < len(liste) && freigabeImDefer(liste[i+1], mu, art) {
					continue
				}
				aus = append(aus, name)
			}
			return true
		})
	}
	return aus
}

// doppelteFreigaben: je Sperre mit defer-Freigabe im Rumpf (ohne
// Funktionsliterale) ein Eintrag "name (doppelt)", wenn sie dort auch direkt
// freigegeben wird.
func doppelteFreigaben(rumpf *ast.BlockStmt, name string) []string {
	perDefer := map[string]bool{}
	direkt := map[string]int{}
	var gehe func(n ast.Node, imDefer bool)
	gehe = func(n ast.Node, imDefer bool) {
		ast.Inspect(n, func(m ast.Node) bool {
			switch k := m.(type) {
			case *ast.FuncLit:
				if !imDefer {
					return false
				}
			case *ast.DeferStmt:
				gehe(k.Call, true)
				return false
			case *ast.CallExpr:
				if sel, ok := k.Fun.(*ast.SelectorExpr); ok && (sel.Sel.Name == "Unlock" || sel.Sel.Name == "RUnlock") {
					mu := ausdruck(sel.X)
					if gepruefteSperre(mu) {
						if imDefer {
							perDefer[mu+"."+sel.Sel.Name] = true
						} else {
							direkt[mu+"."+sel.Sel.Name]++
						}
					}
				}
			}
			return true
		})
	}
	gehe(rumpf, false)
	var aus []string
	for k := range perDefer {
		if direkt[k] > 0 {
			aus = append(aus, name+" (doppelt)")
		}
	}
	return aus
}

// sperrAufruf: "cs.mu"/"dag.mu"/"x.state.mu" und "Lock"/"RLock", sonst "".
func sperrAufruf(st ast.Stmt) (string, string) {
	e, ok := st.(*ast.ExprStmt)
	if !ok {
		return "", ""
	}
	c, ok := e.X.(*ast.CallExpr)
	if !ok || len(c.Args) != 0 {
		return "", ""
	}
	sel, ok := c.Fun.(*ast.SelectorExpr)
	if !ok || (sel.Sel.Name != "Lock" && sel.Sel.Name != "RLock") {
		return "", ""
	}
	mu := ausdruck(sel.X)
	if !gepruefteSperre(mu) {
		return "", ""
	}
	return mu, sel.Sel.Name
}

// gepruefteSperre: die Sperren, die die Regel abdeckt -- cs.mu, dag.mu (auch
// als d.mu), x.state.mu, die Buchfuehrung (w.mu; das Zuruecksetzen nach einer
// Panik wartet auf sie) und offeneVorbehalteMu.
func gepruefteSperre(mu string) bool {
	for _, endung := range []string{"cs.mu", "dag.mu", "state.mu"} {
		if strings.HasSuffix(mu, endung) {
			return true
		}
	}
	return mu == "d.mu" || mu == "w.mu" || mu == "offeneVorbehalteMu"
}

func freigabeImDefer(st ast.Stmt, mu, art string) bool {
	d, ok := st.(*ast.DeferStmt)
	if !ok {
		return false
	}
	// atomicAbschluss.beiPanik gibt die Sperre bei einer Panik frei (die
	// regulaeren Wege entsperren selbst und markieren das).
	if sel, ok := d.Call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "beiPanik" && art == "Lock" && strings.HasSuffix(mu, "cs.mu") {
		return true
	}
	frei := "Unlock"
	if art == "RLock" {
		frei = "RUnlock"
	}
	treffer := false
	ast.Inspect(d.Call, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == frei && ausdruck(sel.X) == mu {
			treffer = true
		}
		return !treffer
	})
	return treffer
}

func ausdruck(e ast.Expr) string {
	switch k := e.(type) {
	case *ast.Ident:
		return k.Name
	case *ast.SelectorExpr:
		return ausdruck(k.X) + "." + k.Sel.Name
	}
	return "?"
}

// sperreOhneDefer: Datei:Funktion -> Zahl der Stellen, die die Regel noch
// nicht erfuellen (Stand bei Einfuehrung). Nur kuerzer werden lassen.
var sperreOhneDefer = map[string]int{
	"autoheal.go:BlockDAG.runChainDivergenceCheckOnce":         1,
	"block.go:BlockDAG.AddPeerBlock":                           4,
	"block.go:BlockDAG.BridgeHistoricalGap":                    1,
	"block.go:BlockDAG.prefetchMergeSetFromDB":                 2,
	"block.go:BlockDAG.prefetchParentsFromDB":                  2,
	"block.go:BlockDAG.pruneOldDAGBlocks":                      1,
	"block.go:BlockDAG.queueOrphan":                            1,
	"block.go:BlockDAG.repairUnreplayedBlocks":                 1,
	"block.go:BlockDAG.replayInCanonicalOrder":                 1,
	"block.go:BlockDAG.replayTransactions":                     1,
	"block.go:BlockDAG.retryAndFlushSoftRetry":                 1,
	"dag_ausduennen.go:BlockDAG.ausduennenEinmal":              2,
	"evm_storage.go:ChainState.MigrateEVMFromGoState":          3,
	"evm_storage.go:ChainState.SavePriceSnapshot":              1,
	"evm_storage.go:ChainState.gemeinsameAuftragsNonce":        2,
	"evm_storage.go:ChainState.migrateEVMFromGoStateV8":        1,
	"grant_staffel.go:APIServer.handleLivenessRenewal":         1,
	"pool_flush.go:ChainState.flushPoolAccountsIfDirty":        1,
	"snapshot.go:ChainState.ExportSnapshot":                    1,
	"snapshot.go:ChainState.ImportSnapshotFromURL":             1,
	"snapshot.go:ChainState.ResyncFromSnapshotURL":             2,
	"state.go:ChainState.DistributeUBIPool":                    1,
	"state.go:ChainState.registerHumanMitZeitenLocked":         1,
	"sync_blocks.go:BlockDAG.doSyncOnce":                       1,
	"sync_blocks.go:BlockDAG.fetchMissingAncestors":            1,
	"sync_blocks.go:BlockDAG.reconcileDeferrals":               1,
	"sync_blocks.go:BlockDAG.registerAndDiscover":              1,
	"sync_blocks.go:BlockDAG.syncValidatorsFromPeer":           3,
	"transfer_wal.go:ChainState.transferConcurrentWALGesperrt": 1,
}

func TestSperre_PruefungErkenntJedeForm(t *testing.T) {
	verboten := []string{
		`cs.mu.Lock(); arbeit(); cs.mu.Unlock()`,
		`dag.mu.RLock(); x := dag.blocks[h]; dag.mu.RUnlock(); _ = x`,
		`d.state.mu.Lock(); arbeit(); d.state.mu.Unlock()`,
		`cs.mu.Lock(); defer cs.mu.RUnlock()`,      // falsche Freigabe
		`cs.mu.RLock(); defer anders.mu.RUnlock()`, // andere Sperre
		`if a { cs.mu.Lock(); arbeit(); cs.mu.Unlock() }`,
		`d.mu.Lock(); d.computeGHOSTDAGState(b); d.mu.Unlock()`,
		`w.mu.Lock(); arbeit(); w.mu.Unlock()`,
		`offeneVorbehalteMu.Lock(); arbeit(); offeneVorbehalteMu.Unlock()`,
		`cs.mu.Lock(); defer cs.mu.Unlock(); if a { cs.mu.Unlock() }`, // doppelt
		`dag.mu.Lock(); defer ab.beiPanik()`,                          // beiPanik nur fuer cs.mu
	}
	erlaubt := []string{
		`cs.mu.Lock(); defer cs.mu.Unlock(); arbeit()`,
		`dag.mu.RLock(); defer dag.mu.RUnlock()`,
		`cs.mu.Lock(); defer func() { r := recover(); cs.mu.Unlock(); _ = r }()`,
		`cs.mu.Lock(); defer ab.beiPanik()`,
		`andere.sperre.Lock(); arbeit(); andere.sperre.Unlock()`,
		`func() { cs.mu.Lock(); defer cs.mu.Unlock() }(); go func() { cs.mu.Lock(); defer cs.mu.Unlock() }()`,
		`cs.mu.Lock(); defer func() { if r := recover(); r != nil { cs.mu.Unlock(); panic(r) }; cs.mu.Unlock() }()`,
	}
	pruefe := func(rumpf string) int {
		f, err := parser.ParseFile(token.NewFileSet(), "p.go", "package p\nfunc f() {\n"+rumpf+"\n}\n", 0)
		if err != nil {
			t.Fatal(err)
		}
		return len(sperrenOhneDefer(f))
	}
	for _, r := range verboten {
		if pruefe(r) == 0 {
			t.Errorf("nicht erkannt: %s", r)
		}
	}
	for _, r := range erlaubt {
		if n := pruefe(r); n != 0 {
			t.Errorf("faelschlich gemeldet: %s", r)
		}
	}
}
