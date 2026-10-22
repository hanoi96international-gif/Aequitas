package keeper

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

// Audit 2026-10-21 (niedrig): bei einem Vertragsaufruf stand die
// Erfolgsquittung "0x1" VOR der Ausfuehrung in der Datenbank. Ein Absturz
// dazwischen hinterliess dauerhaft "erfolgreich" fuer einen Aufruf, der nie
// lief; eine gleichzeitige Abfrage sah den Erfolg vor der Ausfuehrung.
//
// Geprueft wird die Reihenfolge im Quelltext: zwischen dem Laden des Vertrags
// und s.evm.CallContract steht weder eine "0x1"-Quittung noch ein
// Erfolgsmerker; die "0x1"-Quittung des Aufrufs folgt erst nach
// PersistContractStorage.
func TestQuittung_ErfolgErstNachDerAusfuehrung(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "evm_rpc.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var laden, aufruf, sichern token.Pos
	var erfolgsQuittungen, erfolgsMerker []token.Pos
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "sendRawTransaction" {
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				switch types.ExprString(x.Fun) {
				case "s.state.LoadContract":
					laden = x.Pos()
				case "s.evm.CallContract":
					aufruf = x.Pos()
				case "s.evm.PersistContractStorage":
					sichern = x.Pos()
				case "s.state.SaveTxReceipt":
					if len(x.Args) >= 4 {
						if lit, ok := x.Args[3].(*ast.BasicLit); ok && lit.Value == `"0x1"` {
							erfolgsQuittungen = append(erfolgsQuittungen, x.Pos())
						}
					}
				}
			case *ast.AssignStmt:
				if len(x.Lhs) == 1 && len(x.Rhs) == 1 && types.ExprString(x.Lhs[0]) == "sh.status[txHash]" {
					if id, ok := x.Rhs[0].(*ast.Ident); ok && id.Name == "true" {
						erfolgsMerker = append(erfolgsMerker, x.Pos())
					}
				}
			}
			return true
		})
	}
	if laden == token.NoPos || aufruf == token.NoPos || sichern == token.NoPos || laden > aufruf || aufruf > sichern {
		t.Fatalf("Aufrufweg nicht gefunden (LoadContract %v, CallContract %v, PersistContractStorage %v)", laden, aufruf, sichern)
	}
	for _, p := range append(erfolgsQuittungen, erfolgsMerker...) {
		if p > laden && p < aufruf {
			t.Errorf("Erfolg vor der Ausfuehrung vermerkt (%s)", fset.Position(p))
		}
	}
	danach := false
	for _, p := range erfolgsQuittungen {
		if p > sichern {
			danach = true
		}
	}
	if !danach {
		t.Errorf("keine Erfolgsquittung nach PersistContractStorage")
	}
}
