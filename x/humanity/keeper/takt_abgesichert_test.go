package keeper

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Jeder Takt eines langlebigen Arbeiters laeuft in SafeCall.
//
// SafeGoroutine faengt eine Panik nur EINMAL, oben in der Goroutine -- danach
// ist der Arbeiter tot, ohne Absturz, den jemand bemerkt. So konnten eine
// einzige Panik den EVM-Spiegel, die WAL-Kompaktierung (Datei waechst ohne
// Ende), das Speichern der Pool-Konten, den Rueckstau-Messer (die Annahme
// arbeitet dann mit einer eingefrorenen Messung) oder den Totmann fuer immer
// abschalten (Pruefung von #330, 3. Durchgang, INFO-2).
//
// Regel: in `for range X.C`, `case <-X.C:` / `case x := <-X.C:` (auch
// time.Tick/time.After) und in `for { ...; time.Sleep(d) }` darf auf oberster
// Ebene nur SafeCall(...) stehen -- ausser break/continue (die `break collect`
// der Sammler), einem select und einer Frist `if <Zeitvergleich> { return }`.
// Gilt fuer den Keeper und cmd/aequitasd.
//
// Ausgenommen (taktAusnahmen): Takte in einem HTTP-Handler -- dort faengt
// net/http eine Panik je Anfrage, und die Schleife lebt nur so lange wie die
// Anfrage.
func TestTakt_JederTaktInSafeCall(t *testing.T) {
	verzeichnisse := []string{".", filepath.Join("..", "..", "..", "cmd", "aequitasd")}
	gefunden := 0
	for _, verz := range verzeichnisse {
		dateien, err := filepath.Glob(filepath.Join(verz, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, datei := range dateien {
			if strings.HasSuffix(datei, "_test.go") {
				continue
			}
			quelle, err := os.ReadFile(datei)
			if err != nil {
				t.Fatal(err)
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, datei, quelle, 0)
			if err != nil {
				t.Fatal(err)
			}
			n, verstoesse := taktPruefen(fset, f)
			gefunden += n
			for _, v := range verstoesse {
				t.Errorf("%s: Takt ohne SafeCall -- eine Panik beendet diesen Arbeiter fuer immer", v)
			}
		}
	}
	if gefunden < 15 {
		t.Fatalf("nur %d Takte gefunden -- findet die Pruefung die Schleifen noch?", gefunden)
	}
}

// taktPruefen: Zahl der gefundenen Takte und die Stellen, die die Regel
// verletzen.
func taktPruefen(fset *token.FileSet, f *ast.File) (gefunden int, verstoesse []token.Position) {
	// select-Zweige zaehlen nur innerhalb einer Schleife (ohne in
	// eine Funktionsliteral hineinzugehen) -- ein einmaliges Warten
	// wie beim Beenden ist kein Arbeiter.
	inSchleife := map[*ast.CommClause]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		var koerper *ast.BlockStmt
		switch k := n.(type) {
		case *ast.ForStmt:
			koerper = k.Body
		case *ast.RangeStmt:
			koerper = k.Body
		default:
			return true
		}
		ast.Inspect(koerper, func(m ast.Node) bool {
			if _, ok := m.(*ast.FuncLit); ok {
				return false
			}
			if c, ok := m.(*ast.CommClause); ok {
				inSchleife[c] = true
			}
			return true
		})
		return true
	})
	ast.Inspect(f, func(n ast.Node) bool {
		var rumpf []ast.Stmt
		var erlaubt ast.Stmt // die Warteanweisung selbst (for {}-Schleifen)
		switch k := n.(type) {
		case *ast.RangeStmt:
			if istTaktQuelle(k.X) {
				rumpf = k.Body.List
			}
		case *ast.CommClause:
			if inSchleife[k] && istTaktEmpfang(k.Comm) {
				rumpf = k.Body
			}
		case *ast.ForStmt:
			// for { ...; time.Sleep(d) } oder for { <-t.C; ... }
			if k.Cond == nil {
				for _, st := range k.Body.List {
					if istWarten(st) {
						rumpf, erlaubt = k.Body.List, st
						break
					}
				}
			}
		default:
			return true
		}
		if rumpf == nil {
			return true
		}
		gefunden++
		pos := fset.Position(n.Pos())
		if taktAusnahmen[filepath.Base(pos.Filename)+":"+funktionBei(f, n.Pos())] {
			return true
		}
		for _, st := range rumpf {
			if st == erlaubt {
				continue
			}
			if !istSafeCall(st) && !istAblauf(st) && !istFristReturn(st) && !istSelectOhneAufruf(st) {
				verstoesse = append(verstoesse, fset.Position(st.Pos()))
			}
		}
		if len(rumpf) == 0 {
			verstoesse = append(verstoesse, pos)
		}
		return true
	})
	return gefunden, verstoesse
}

// istTaktQuelle: X.C (Kanal eines time.Ticker/time.Timer), time.Tick(...)
// oder time.After(...).
func istTaktQuelle(e ast.Expr) bool {
	switch k := e.(type) {
	case *ast.SelectorExpr:
		return k.Sel.Name == "C"
	case *ast.CallExpr:
		if s, ok := k.Fun.(*ast.SelectorExpr); ok {
			if p, ok := s.X.(*ast.Ident); ok && p.Name == "time" {
				return s.Sel.Name == "Tick" || s.Sel.Name == "After"
			}
		}
	}
	return false
}

// istTaktEmpfang: `<-quelle` als Anweisung oder `x := <-quelle` /
// `x = <-quelle`.
func istTaktEmpfang(st ast.Stmt) bool {
	var e ast.Expr
	switch k := st.(type) {
	case *ast.ExprStmt:
		e = k.X
	case *ast.AssignStmt:
		if len(k.Rhs) == 1 {
			e = k.Rhs[0]
		}
	}
	u, ok := e.(*ast.UnaryExpr)
	return ok && u.Op == token.ARROW && istTaktQuelle(u.X)
}

// istWarten: time.Sleep(...) oder ein Taktempfang als Anweisung.
func istWarten(st ast.Stmt) bool {
	if istTaktEmpfang(st) {
		return true
	}
	e, ok := st.(*ast.ExprStmt)
	if !ok {
		return false
	}
	c, ok := e.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	s, ok := c.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	p, ok := s.X.(*ast.Ident)
	return ok && p.Name == "time" && s.Sel.Name == "Sleep"
}

// istSelectOhneAufruf: ein select, das nur auf Kanaele wartet (etwa der
// Stopp-Kanal) -- seine Zweige prueft die Inspektion selbst.
func istSelectOhneAufruf(st ast.Stmt) bool {
	_, ok := st.(*ast.SelectStmt)
	return ok
}

// taktAusnahmen: Datei:Funktion, mit Grund.
var taktAusnahmen = map[string]bool{
	"api.go:handleBlockEvents": true, // SSE-Keepalive je Anfrage: net/http faengt die Panik je Anfrage
}

func istSafeCall(st ast.Stmt) bool {
	e, ok := st.(*ast.ExprStmt)
	if !ok {
		return false
	}
	c, ok := e.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	switch f := c.Fun.(type) {
	case *ast.Ident:
		return f.Name == "SafeCall"
	case *ast.SelectorExpr:
		return f.Sel.Name == "SafeCall"
	}
	return false
}

// istAblauf: nur break/continue/goto oder leer -- die `break collect` der
// Sammler. Andere Anweisungen ohne Aufruf koennen auch panieren (Index,
// nil-Map, Typzusicherung) und gehoeren in SafeCall.
func istAblauf(st ast.Stmt) bool {
	switch st.(type) {
	case *ast.BranchStmt, *ast.EmptyStmt:
		return true
	}
	return false
}

// istFristReturn: `if <Zeitvergleich> { return }` ohne else -- beendet den
// Arbeiter gewollt. Die Bedingung darf nur Zeitvergleiche aufrufen
// (After/Before/IsZero), sonst gehoert sie in SafeCall.
func istFristReturn(st ast.Stmt) bool {
	i, ok := st.(*ast.IfStmt)
	if !ok || i.Else != nil || i.Init != nil || len(i.Body.List) != 1 {
		return false
	}
	r, ok := i.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(r.Results) != 0 {
		return false
	}
	nurZeit := true
	ast.Inspect(i.Cond, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		s, ok := c.Fun.(*ast.SelectorExpr)
		if !ok || (s.Sel.Name != "After" && s.Sel.Name != "Before" && s.Sel.Name != "IsZero") {
			nurZeit = false
		}
		return true
	})
	return nurZeit
}

// funktionBei: Name der Funktion, die pos enthaelt.
func funktionBei(f *ast.File, pos token.Pos) string {
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Pos() <= pos && pos <= fd.End() {
			return fd.Name.Name
		}
	}
	return ""
}

// Die Luecken aus der Pruefung von #337: jede Form muss erkannt werden, und
// die erlaubten Formen duerfen nicht anschlagen.
func TestTakt_PruefungErkenntJedeForm(t *testing.T) {
	verboten := map[string]string{
		"range":       `for range t.C { arbeit() }`,
		"case":        `for { select { case <-t.C: arbeit() } }`,
		"case-zuweis": `for { select { case jetzt := <-t.C: arbeit(jetzt) } }`,
		"after":       `for { select { case <-time.After(d): arbeit() } }`,
		"tick":        `for range time.Tick(d) { arbeit() }`,
		"sleep":       `for { time.Sleep(d); arbeit() }`,
		"empfang":     `for { <-t.C; arbeit() }`,
		"frist-ruf":   `for range t.C { if bald() { return } ; SafeCall("x", arbeit) }`,
		"ohne-ruf":    `for range t.C { m["x"] = 1 ; SafeCall("x", arbeit) }`,
	}
	erlaubt := map[string]string{
		"safecall":  `for range t.C { SafeCall("x", arbeit) }`,
		"selektor":  `for range t.C { keeper.SafeCall("x", arbeit) }`,
		"frist":     `for jetzt := range t.C { if jetzt.After(frist) { return } ; SafeCall("x", arbeit) }`,
		"sammler":   `for { select { case r := <-ein: _ = r ; case <-t.C: break } }`,
		"einmalig":  `select { case <-fertig: ; case <-time.After(d): arbeit() }`,
		"sleep-ok":  `for { time.Sleep(d); SafeCall("x", arbeit) }`,
		"funclit":   `for range t.C { SafeCall("x", func() { arbeit() }) }`,
		"kein-takt": `for x := range kanal { arbeit(x) }`,
	}
	pruefe := func(name, rumpf string) []token.Position {
		quelle := "package p\nfunc f() {\n" + rumpf + "\n}\n"
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, name+".go", quelle, 0)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		_, v := taktPruefen(fset, f)
		return v
	}
	for name, rumpf := range verboten {
		if len(pruefe(name, rumpf)) == 0 {
			t.Errorf("%s nicht erkannt: %s", name, rumpf)
		}
	}
	for name, rumpf := range erlaubt {
		if v := pruefe(name, rumpf); len(v) != 0 {
			t.Errorf("%s faelschlich gemeldet (%v): %s", name, v, rumpf)
		}
	}
}
