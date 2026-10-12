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
// Regel: in `for range X.C { ... }` und `case <-X.C:` darf auf oberster Ebene
// nur SafeCall(...) stehen -- ausser reiner Ablaufsteuerung ohne jeden Aufruf
// (`break collect` der Sammler) und einer Fristpruefung `if <Bedingung> {
// return }`. Gilt fuer den Keeper und cmd/aequitasd.
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
			ast.Inspect(f, func(n ast.Node) bool {
				var rumpf []ast.Stmt
				switch k := n.(type) {
				case *ast.RangeStmt:
					if istTaktKanal(k.X) {
						rumpf = k.Body.List
					}
				case *ast.CommClause:
					if e, ok := k.Comm.(*ast.ExprStmt); ok {
						if u, ok := e.X.(*ast.UnaryExpr); ok && u.Op == token.ARROW && istTaktKanal(u.X) {
							rumpf = k.Body
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
					if !istSafeCall(st) && !ohneAufruf(st) && !istFristReturn(st) {
						t.Errorf("%s: Takt ohne SafeCall -- eine Panik beendet diesen Arbeiter fuer immer",
							fset.Position(st.Pos()))
					}
				}
				if len(rumpf) == 0 {
					t.Errorf("%s: leerer Takt", fset.Position(n.Pos()))
				}
				return true
			})
		}
	}
	if gefunden < 15 {
		t.Fatalf("nur %d Takte gefunden -- findet die Pruefung die Schleifen noch?", gefunden)
	}
}

// istTaktKanal: X.C, der Kanal eines time.Ticker/time.Timer.
func istTaktKanal(e ast.Expr) bool {
	s, ok := e.(*ast.SelectorExpr)
	return ok && s.Sel.Name == "C"
}

// taktAusnahmen: Datei:Funktion, mit Grund oben.
var taktAusnahmen = map[string]bool{
	"api.go:handleBlockEvents": true, // SSE-Keepalive je Anfrage
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

// ohneAufruf: eine Anweisung ohne jeden Funktionsaufruf (break, return ...).
func ohneAufruf(st ast.Stmt) bool {
	aufruf := false
	ast.Inspect(st, func(n ast.Node) bool {
		if _, ok := n.(*ast.CallExpr); ok {
			aufruf = true
		}
		return !aufruf
	})
	return !aufruf
}

// istFristReturn: `if <Bedingung> { return }` ohne else -- beendet den
// Arbeiter gewollt.
func istFristReturn(st ast.Stmt) bool {
	i, ok := st.(*ast.IfStmt)
	if !ok || i.Else != nil || i.Init != nil || len(i.Body.List) != 1 {
		return false
	}
	r, ok := i.Body.List[0].(*ast.ReturnStmt)
	return ok && len(r.Results) == 0
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
