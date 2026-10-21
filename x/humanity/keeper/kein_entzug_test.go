package keeper

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// KEIN ENTZUG (docs/VERIFIER_PRINZIPIEN.md, A1): ein Verifier, Betreiber oder
// Erzeuger darf einen Menschen nur bestaetigen, nie wegnehmen. Auf
// Konsensebene heisst das: KEINE Transaktion im Block nimmt einem
// registrierten Menschen das Menschsein -- egal, was der Erzeuger in ihre
// Felder schreibt, und ohne dass dieser Mensch irgendetwas unterschrieben hat.
//
// Der Test geht direkt auf replayTransactions -- mit den Stichtagen des
// Codes (produktionsStichtage), also mit der Unterschriftspruefung, die
// replayTransactions selbst fuer jeden Block ausfuehrt (signatur_vorab.go,
// pruefeUeberweisungenImBlock, pruefeAuftraegeImBlock). Er bildet einen
// Erzeuger ab, der beliebige Felder in einen Block schreibt.
//
// Geld wird hier nicht gleich streng geprueft wie das Menschsein:
//
//   - Arten, die ein Mensch selbst unterschreibt (Ueberweisung, Tausch,
//     Vorbehalt, ...), duerfen sein Guthaben nur mit seiner Unterschrift
//     senken. Gefaelscht senken sie es nicht -- das prueft dieser Test.
//   - BEKANNTE AUSNAHMEN: Arten, die nach Regel ohne Unterschrift Geld
//     wegnehmen (bekannteGeldAusnahmen unten). Ihre Betraege rechnet heute
//     nachrechnen.go nur BEOBACHTEND nach (nachrechnenStrengAbUnix =
//     MaxInt64): eine Abweichung wird gezaehlt, der Block gilt. Sie werden
//     regelweise streng geschaltet, sobald die Beobachtung 0 Abweichungen
//     zeigt (Phase A2), spaetestens vor dem zweiten Erzeuger. Dieser Schritt
//     aendert ihr Verhalten nicht.
//   - Gutschriften (Grundeinkommen, Lohn, ...) sollten nie senken; drei tun
//     es heute aus demselben Grund (nur beobachtet) -- als Befund in
//     heuteGeldOhneUnterschrift festgehalten, die Liste darf nur schrumpfen.

// geldArt: wie eine Transaktionsart das Guthaben des Betroffenen bewegen darf.
type geldArt int

const (
	// geldNurGutschrift: darf das Guthaben eines Menschen nie senken.
	geldNurGutschrift geldArt = iota
	// geldUnterschrieben: darf senken, aber nur mit Unterschrift des
	// Menschen (Unterschriftspruefung in replayTransactions).
	geldUnterschrieben
	// geldRegelAusnahme: senkt nach Regel ohne Unterschrift; heute nur
	// beobachtend nachgerechnet (siehe bekannteGeldAusnahmen).
	geldRegelAusnahme
)

// replayArten: JEDE Art im Nachspiel-Switch, mit ihrer Geldklasse. Kommt eine
// neue Art hinzu, scheitert TestKeinEntzug_AlleArtenErfasst, bis sie hier
// bewusst eingeordnet ist.
var replayArten = map[string]geldArt{
	"register_human":                   geldNurGutschrift,
	"transfer":                         geldUnterschrieben,
	"swap_aeq_tusd":                    geldUnterschrieben,
	"swap_tusd_aeq":                    geldUnterschrieben,
	"add_liquidity":                    geldUnterschrieben,
	"remove_liquidity":                 geldUnterschrieben,
	"faucet":                           geldNurGutschrift,
	"ubi_distribution":                 geldNurGutschrift,
	"liveness_renewal":                 geldNurGutschrift,
	"proof_of_life":                    geldNurGutschrift, // lebensnachweis.go: bewegt kein Geld, setzt nur LebensnachweisAm
	"grant_release":                    geldNurGutschrift,
	"umlauf":                           geldRegelAusnahme,
	"unternehmen_eroeffnen":            geldUnterschrieben,
	"vorbehalt":                        geldUnterschrieben,
	"vorbehalt_ausfuehrung":            geldUnterschrieben,
	"kappung":                          geldRegelAusnahme,
	"unternehmen_mitinhaber":           geldUnterschrieben,
	"unternehmen_schliessen":           geldUnterschrieben,
	"unternehmen_verzeichnis":          geldUnterschrieben,
	"unternehmen_buergschaft":          geldUnterschrieben,
	"unternehmen_austreten":            geldUnterschrieben,
	"ubi_distribution_finalize":        geldNurGutschrift,
	"pool_correction":                  geldNurGutschrift,
	"distribution_round_marker":        geldNurGutschrift,
	"validator_distribution":           geldNurGutschrift,
	"validator_distribution_pool_zero": geldNurGutschrift,
	"lp_distribution":                  geldNurGutschrift,
	"lp_distribution_pool_zero":        geldNurGutschrift,
	"escrow_move":                      geldRegelAusnahme,
	"escrow_release":                   geldNurGutschrift,
	"escrow_recover":                   geldNurGutschrift,
	"vormund_setzen":                   geldUnterschrieben,
	"lebenszeichen":                    geldUnterschrieben,
	"validator_bindung":                geldUnterschrieben,
	checkpointTxArt:                    geldNurGutschrift,
	"slash_equivocation":               geldRegelAusnahme,
	"slash_abrechnung":                 geldRegelAusnahme,
}

// bekannteGeldAusnahmen: warum diese Arten ohne Unterschrift Geld nehmen
// duerfen und wer das heute begrenzt. Verhalten in diesem Schritt
// unveraendert; streng geschaltet wird regelweise in Phase A2.
var bekannteGeldAusnahmen = map[string]string{
	"escrow_move": "Treuhand bei Inaktivitaet (treuhand_*): Betrag und Zeitpunkt rechnet " +
		"nachrechnen_treuhand.go nach -- heute nur beobachtend.",
	"umlauf": "Umlaufsicherung/Liegegeld: pruefeUmlaufLocked prueft beim Nachspielen, " +
		"die Regelzahlen (liegegeld) rechnet nachrechnen nur beobachtend nach.",
	"kappung": "Vermoegensgrenze: Obergrenze des Betrags (kappung_unter_grenze) nur beobachtend.",
	"slash_equivocation": "Strafe eines Validator-Betreibers bei Doppelsignatur; " +
		"Beweis (zwei eigene Signaturen) noetig, nachrechnenSlashLocked.",
	"slash_abrechnung": "Geldstrafe des zweiten Vergehens aus dem Verlauf (strafe_abrechnung.go).",
}

// ersetzbareKonstanten: Bezeichner, die im Switch statt eines
// Zeichenkettenliterals stehen.
var ersetzbareKonstanten = map[string]string{
	"checkpointTxArt": checkpointTxArt,
}

// replaySwitchArten liest die Arten des Nachspiel-Switches aus block.go
// (replayTransactions, switch tx.Type mit case "register_human").
func replaySwitchArten(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "block.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var arten []string
	gefunden := false
	ast.Inspect(f, func(n ast.Node) bool {
		fd, ok := n.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "replayTransactions" || fd.Body == nil {
			return true
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok || gefunden {
				return true
			}
			sel, ok := sw.Tag.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Type" {
				return true
			}
			var hier []string
			for _, st := range sw.Body.List {
				cc := st.(*ast.CaseClause)
				for _, e := range cc.List {
					switch v := e.(type) {
					case *ast.BasicLit:
						s, err := strconv.Unquote(v.Value)
						if err != nil {
							t.Fatal(err)
						}
						hier = append(hier, s)
					case *ast.Ident:
						w, ok := ersetzbareKonstanten[v.Name]
						if !ok {
							t.Fatalf("unbekannte Konstante %s im Nachspiel-Switch -- in ersetzbareKonstanten eintragen", v.Name)
						}
						hier = append(hier, w)
					default:
						t.Fatalf("unerwarteter Ausdruck im Nachspiel-Switch: %T", e)
					}
				}
			}
			for _, a := range hier {
				if a == "register_human" {
					gefunden = true
					arten = hier
					return false
				}
			}
			return true
		})
		return false
	})
	if !gefunden {
		t.Fatal("Nachspiel-Switch in replayTransactions nicht gefunden")
	}
	sort.Strings(arten)
	return arten
}

// Jede Art im Nachspiel-Switch ist hier eingeordnet -- und nichts ausserhalb.
func TestKeinEntzug_AlleArtenErfasst(t *testing.T) {
	arten := replaySwitchArten(t)
	imSwitch := map[string]bool{}
	for _, a := range arten {
		imSwitch[a] = true
		if _, ok := replayArten[a]; !ok {
			t.Errorf("Art %q im Nachspiel-Switch ist in kein_entzug_test.go nicht eingeordnet (replayArten)", a)
		}
	}
	for a := range replayArten {
		if !imSwitch[a] {
			t.Errorf("replayArten nennt %q, der Nachspiel-Switch kennt die Art nicht", a)
		}
	}
	for a := range bekannteGeldAusnahmen {
		if replayArten[a] != geldRegelAusnahme {
			t.Errorf("%q steht in bekannteGeldAusnahmen, ist aber nicht als geldRegelAusnahme eingeordnet", a)
		}
	}
	for a, k := range replayArten {
		if k == geldRegelAusnahme && bekannteGeldAusnahmen[a] == "" {
			t.Errorf("%q ist geldRegelAusnahme ohne Begruendung in bekannteGeldAusnahmen", a)
		}
	}
}

// entzugUhr: feste Uhr (19.10.2026), nach allen heutigen Stichtagen
// (Unterschriftspflicht, Chain-ID-Wechsel) -- der Lauf haengt nicht am Datum.
const entzugUhr int64 = 1792368000

const (
	entzugOpfer    = "0x00000000000000000000000000000000000000a1"
	entzugZweiter  = "0x00000000000000000000000000000000000000a2"
	entzugAngreifr = "0x00000000000000000000000000000000000000e1"
)

// entzugWelt: zwei registrierte Menschen mit Guthaben, tUSD und LP-Anteilen,
// ein Angreifer ohne Menschsein, ein Pool mit Reserven. Ohne Datenbank.
func entzugWelt() (*BlockDAG, *ChainState) {
	dag, cs := newDeterminismTestDAG()
	entzugBevoelkern(nil, cs)
	return dag, cs
}

// entzugBevoelkern legt die Konten an; mit t auch in der Datenbank.
func entzugBevoelkern(t *testing.T, cs *ChainState) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	konten := []*AccountState{
		{Address: entzugOpfer, IsHuman: true, Balance: NewDecimal(1000), TUsdBalance: NewDecimal(500), LPShares: NewDecimal(10)},
		{Address: entzugZweiter, IsHuman: true, Balance: NewDecimal(1000), TUsdBalance: NewDecimal(500), LPShares: NewDecimal(10)},
		{Address: entzugAngreifr, Balance: NewDecimal(1000)},
	}
	for _, acc := range konten {
		if t != nil {
			if err := cs.saveAccountToDB(acc); err != nil {
				t.Fatalf("Konto anlegen: %v", err)
			}
		}
		cs.accounts.Set(acc.Address, acc)
	}
	cs.pool.ReserveAEQ = NewDecimal(10000)
	cs.pool.ReserveTUSD = NewDecimal(10000)
	cs.pool.TotalLPShares = NewDecimal(100)
}

// zufallsBetrag: auch die Werte, die ein boesartiger Erzeuger versucht.
func zufallsBetrag(rng *rand.Rand) float64 {
	switch rng.Intn(9) {
	case 0:
		return 0
	case 1:
		return -rng.Float64() * 1e6
	case 2:
		return math.MaxFloat64
	case 3:
		return math.Inf(1)
	case 4:
		return math.NaN()
	case 5:
		return 1000 // genau das Guthaben des Opfers
	case 6:
		return 1e-12
	default:
		return rng.Float64() * 5000
	}
}

func zufallsText(rng *rand.Rand) string {
	const zeichen = "0123456789abcdef"
	n := rng.Intn(80)
	b := make([]byte, n)
	for i := range b {
		b[i] = zeichen[rng.Intn(len(zeichen))]
	}
	if rng.Intn(2) == 0 {
		return "0x" + string(b)
	}
	return string(b)
}

// zufallsAdresse: meist das Opfer -- in jedem Adressfeld.
func zufallsAdresse(rng *rand.Rand) string {
	switch rng.Intn(6) {
	case 0, 1, 2:
		return entzugOpfer
	case 3:
		return strings.ToUpper(entzugOpfer[:2]) + strings.ToUpper(entzugOpfer[2:]) // andere Schreibweise
	case 4:
		return entzugAngreifr
	default:
		return zufallsText(rng)
	}
}

// gefaelschteTx: eine Transaktion der Art art, die das Opfer trifft, aber
// nicht von ihm unterschrieben ist (Unterschriftsfelder sind Zufall oder
// leer, Zeiger meist nil, manchmal leer).
func gefaelschteTx(rng *rand.Rand, art string, blockZeit int64) Transaction {
	tx := Transaction{
		Type:                art,
		Wallet:              zufallsAdresse(rng),
		To:                  zufallsAdresse(rng),
		Amount:              zufallsBetrag(rng),
		AmountOut:           zufallsBetrag(rng),
		AmountPerHuman:      zufallsBetrag(rng),
		LPShares:            zufallsBetrag(rng),
		EscrowTUsdConverted: zufallsBetrag(rng),
		FromDemurrageLost:   zufallsBetrag(rng),
		ToDemurrageLost:     zufallsBetrag(rng),
		Gebuehr:             zufallsBetrag(rng),
		BuchAt:              blockZeit - int64(rng.Intn(3600)),
		DistributionAt:      blockZeit - int64(rng.Intn(200000)),
		Roh:                 zufallsText(rng),
		TxHash:              zufallsText(rng),
		Nullifier:           zufallsText(rng),
		Commitment:          zufallsText(rng),
		GrantClass:          zufallsText(rng),
		Name:                zufallsText(rng),
		Kategorie:           zufallsText(rng),
		RegSignatur:         zufallsText(rng),
		RegFrist:            blockZeit + int64(rng.Intn(3600)),
		RegAt:               blockZeit - int64(rng.Intn(3600)),
		BlockAHash:          zufallsText(rng),
		BlockBHash:          zufallsText(rng),
		DetectedAt:          blockZeit - int64(rng.Intn(3600)),
	}
	if rng.Intn(3) == 0 {
		tx.ProofA = []string{zufallsText(rng), zufallsText(rng)}
		tx.ProofB = [][]string{{zufallsText(rng), zufallsText(rng)}, {zufallsText(rng), zufallsText(rng)}}
		tx.ProofC = []string{zufallsText(rng), zufallsText(rng)}
		tx.PubSignals = []string{zufallsText(rng), zufallsText(rng)}
	}
	if rng.Intn(4) == 0 {
		tx.Nachweis = &Auftragsnachweis{Von2: zufallsAdresse(rng)}
	}
	if rng.Intn(6) == 0 {
		tx.Vorbehalt = &VorbehaltAngaben{}
	}
	if rng.Intn(6) == 0 {
		tx.Doppelbeweis = &Doppelbeweis{}
	}
	if rng.Intn(6) == 0 {
		tx.Bescheinigung = &Lebendigkeitsbescheinigung{}
	}
	return tx
}

// entzugNachspielen spielt einen Block mit txs nach. Ein Panic beim
// Nachspielen eines fremden Blocks ist selbst ein Befund (ein Peer koennte
// Knoten damit anhalten) und laesst den Test scheitern.
func entzugNachspielen(t *testing.T, dag *BlockDAG, txs []Transaction, blockZeit int64, hash string) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Panic beim Nachspielen eines gefaelschten Blocks (%s): %v\n%s", hash, r, debug.Stack())
		}
	}()
	dag.replayTransactions(&Block{Height: 1, Hash: hash, Timestamp: blockZeit, Transactions: txs}, true)
}

// produktionsStichtage: die Stichtage im Code gelten, wie auf einem echten
// Knoten -- TestMain haelt sonst signierte Ueberweisungen und die
// Wirtschaftsregeln fuer alle Tests ausgeschaltet (wirtschaft_main_test.go).
// Ohne das liefe der Test gegen eine Kette ohne Unterschriftspruefung.
func produktionsStichtage(t *testing.T) {
	t.Helper()
	sig, wirt := signierteUeberweisungenOverride.Load(), wirtschaftAktivOverride.Load()
	signierteUeberweisungenOverride.Store(0)
	wirtschaftAktivOverride.Store(0)
	t.Cleanup(func() {
		signierteUeberweisungenOverride.Store(sig)
		wirtschaftAktivOverride.Store(wirt)
	})
	if !signierteUeberweisungenPflicht(nowUnix()) {
		t.Fatal("Unterschriftspflicht laut Code-Stichtag nicht aktiv")
	}
}

// heuteGeldOhneUnterschrift: BEFUND, keine Regel. Gutschriftarten, mit
// denen ein Erzeuger heute einem Menschen ohne dessen Unterschrift Guthaben
// nehmen kann: ein negativer Betrag (betrag_ungueltig) oder eine erfundene
// Demurrage (demurrage_nach_umstellung) werden von nachrechnen.go nur
// BEOBACHTET (nachrechnenStrengAbUnix = MaxInt64) -- gezaehlt, der Block
// gilt. Dokumentiert in docs/VERIFIER_PRINZIPIEN.md; in diesem Schritt
// unveraendert, geschlossen wird es mit dem strengen Modus dieser Regeln
// (Phase A2). Die Liste darf nur kuerzer werden: senkt eine weitere Art
// Guthaben, scheitert der Test.
var heuteGeldOhneUnterschrift = map[string]string{
	"ubi_distribution":       "negativer Betrag / erfundene Demurrage, nur beobachtet",
	"lp_distribution":        "negativer Betrag / erfundene Demurrage, nur beobachtet",
	"validator_distribution": "negativer Betrag / erfundene Demurrage, nur beobachtet",
}

// Die Invariante: fuer jede Art im Nachspiel-Switch, mit gefaelschten
// Feldern gegen einen registrierten Menschen (nicht von ihm unterschrieben),
// bleibt IsHuman wahr -- fuer das Opfer und fuer einen unbeteiligten
// Menschen. Ausserdem: welche Arten sein Guthaben senken, steht vollstaendig
// in heuteGeldOhneUnterschrift.
func TestKeinEntzug_GefaelschteTransaktionenNehmenKeinMenschsein(t *testing.T) {
	produktionsStichtage(t)
	uhr(t, entzugUhr)
	blockZeit := nowUnix()
	const jeArt = 40
	arten := replaySwitchArten(t)
	senkt := map[string]float64{}
	for _, art := range arten {
		t.Run(art, func(t *testing.T) {
			rng := rand.New(rand.NewSource(int64(len(art)) * 7919))
			for i := 0; i < jeArt; i++ {
				tx := gefaelschteTx(rng, art, blockZeit)
				dag, cs := entzugWelt()
				entzugNachspielen(t, dag, []Transaction{tx}, blockZeit, fmt.Sprintf("entzug-%s-%d", art, i))
				for _, m := range []string{entzugOpfer, entzugZweiter} {
					if !cs.IsHuman(m) {
						t.Fatalf("%s nahm %s das Menschsein (Lauf %d, wallet=%q to=%q amount=%v)", art, m, i, tx.Wallet, tx.To, tx.Amount)
					}
				}
				if b := stand(cs, entzugOpfer); b < 1000-1e-9 {
					if alt, da := senkt[art]; !da || b < alt {
						senkt[art] = b
					}
				}
			}
		})
	}
	for art, b := range senkt {
		if replayArten[art] == geldUnterschrieben {
			t.Errorf("%s senkt das Guthaben eines Menschen (bis %.6f), obwohl die Art seine Unterschrift verlangt -- die Unterschriftspruefung greift nicht", art, b)
			continue
		}
		_, regel := bekannteGeldAusnahmen[art]
		_, befund := heuteGeldOhneUnterschrift[art]
		if !regel && !befund {
			t.Errorf("%s senkt das Guthaben eines Menschen ohne seine Unterschrift (bis %.6f) und steht weder in bekannteGeldAusnahmen noch in heuteGeldOhneUnterschrift", art, b)
		}
	}
	for art := range heuteGeldOhneUnterschrift {
		if replayArten[art] != geldNurGutschrift {
			t.Errorf("heuteGeldOhneUnterschrift nennt %q, die Art ist aber keine Gutschrift", art)
		}
		if _, da := senkt[art]; !da {
			t.Logf("Hinweis: %s senkte in diesem Lauf kein Guthaben -- ist die Regel streng geworden, aus heuteGeldOhneUnterschrift streichen", art)
		}
	}
	// Gemischt: viele Arten in einem Block, in Zufallsreihenfolge.
	rng := rand.New(rand.NewSource(20261018))
	for runde := 0; runde < 30; runde++ {
		txs := make([]Transaction, 0, 12)
		for j := 0; j < 12; j++ {
			txs = append(txs, gefaelschteTx(rng, arten[rng.Intn(len(arten))], blockZeit))
		}
		dag, cs := entzugWelt()
		entzugNachspielen(t, dag, txs, blockZeit, fmt.Sprintf("entzug-gemischt-%d", runde))
		for _, m := range []string{entzugOpfer, entzugZweiter} {
			if !cs.IsHuman(m) {
				t.Fatalf("gemischter Block %d nahm %s das Menschsein", runde, m)
			}
		}
	}
}

// Dieselbe Invariante mit echter Datenbank (Rueckrollen ueber die
// Datenbank-Transaktion, Lesen aus chain_accounts): geprueft wird der
// Speicher UND die Zeile in chain_accounts. Laeuft in CI (db-tests).
func TestKeinEntzug_GefaelschteTransaktionen_RealDB(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	produktionsStichtage(t)
	uhr(t, entzugUhr)
	blockZeit := nowUnix()
	const jeArt = 8
	arten := replaySwitchArten(t)
	truncateDistTestTables(t)
	k := neuerTreuhandTestKnoten(t, "unused-kein-entzug.json")
	entzugBevoelkern(t, k.cs)
	rng := rand.New(rand.NewSource(19260930))
	for _, art := range arten {
		for i := 0; i < jeArt; i++ {
			tx := gefaelschteTx(rng, art, blockZeit)
			entzugNachspielen(t, k.dag, []Transaction{tx}, blockZeit, fmt.Sprintf("entzug-db-%s-%d", art, i))
			for _, m := range []string{entzugOpfer, entzugZweiter} {
				if !k.cs.IsHuman(m) {
					t.Fatalf("%s nahm %s das Menschsein (Speicher, Lauf %d)", art, m, i)
				}
				var human bool
				if err := k.cs.db.QueryRow(`SELECT is_human FROM chain_accounts WHERE lower(address) = $1`, m).Scan(&human); err != nil {
					t.Fatalf("%s: chain_accounts fuer %s: %v", art, m, err)
				}
				if !human {
					t.Fatalf("%s nahm %s das Menschsein (Datenbank, Lauf %d)", art, m, i)
				}
			}
		}
	}
}

// Waechter: ausserhalb der ausdruecklich erlaubten Stelle setzt keine
// Nicht-Test-Datei des Keepers is_human auf false -- weder in SQL noch in Go.
// Erlaubt ist allein clearRegistrationsFromDB (state.go), und die verweigert
// auf dem Hauptnetz (hauptnetz_schutz.go).
//
// Gesucht wird:
//   - in jedem Zeichenkettenliteral eine Zuweisung is_human = false/'f'/0/NOT ...
//     hinter einem SET (ein WHERE is_human = false ist ein Lesen und erlaubt);
//   - in Go: x.IsHuman = false, x.IsHuman = !..., IsHuman: false.
//
// Persistieren aus dem Speicher (is_human = $3, EXCLUDED.is_human) schreibt
// nur zurueck, was Go setzt -- deshalb die Go-Suche dazu.
func TestKeinEntzug_NiemandSetztIsHumanFalse(t *testing.T) {
	erlaubt := map[string]bool{
		"state.go:clearRegistrationsFromDB": true,
	}
	sqlFalsch := regexp.MustCompile(`(?i)is_human\s*=\s*(false\b|'f'|'false'|0\b|not\b)`)
	setWort := regexp.MustCompile(`(?i)\bSET\b`)
	leseWort := regexp.MustCompile(`(?i)\b(WHERE|AND|OR|WHEN|HAVING)\b`)

	dateien, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	gefunden := 0
	for _, datei := range dateien {
		if strings.HasSuffix(datei, "_test.go") {
			continue
		}
		src, err := os.ReadFile(datei)
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, datei, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn := "(paket)"
			if fd, ok := d.(*ast.FuncDecl); ok {
				fn = fd.Name.Name
			}
			melde := func(pos token.Pos, was string) {
				if erlaubt[datei+":"+fn] {
					gefunden++
					return
				}
				t.Errorf("%s (in %s): %s -- is_human darf nur clearRegistrationsFromDB auf false setzen (Ein Betreiber darf nur bestaetigen, nie wegnehmen)", fset.Position(pos), fn, was)
			}
			ast.Inspect(d, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.BasicLit:
					if v.Kind != token.STRING {
						return true
					}
					s, err := strconv.Unquote(v.Value)
					if err != nil {
						return true
					}
					for _, m := range sqlFalsch.FindAllStringIndex(s, -1) {
						davor := s[:m[0]]
						set := setWort.FindAllStringIndex(davor, -1)
						lese := leseWort.FindAllStringIndex(davor, -1)
						letztesSet, letztesLesen := -1, -1
						if len(set) > 0 {
							letztesSet = set[len(set)-1][0]
						}
						if len(lese) > 0 {
							letztesLesen = lese[len(lese)-1][0]
						}
						if letztesSet >= 0 && letztesSet > letztesLesen {
							melde(v.Pos(), "SQL "+strings.TrimSpace(s[m[0]:m[1]]))
						}
					}
				case *ast.AssignStmt:
					for i, lhs := range v.Lhs {
						sel, ok := lhs.(*ast.SelectorExpr)
						if !ok || sel.Sel.Name != "IsHuman" || i >= len(v.Rhs) {
							continue
						}
						if falschWert(v.Rhs[i]) {
							melde(v.Pos(), "IsHuman = false")
						}
					}
				case *ast.KeyValueExpr:
					if k, ok := v.Key.(*ast.Ident); ok && k.Name == "IsHuman" && falschWert(v.Value) {
						melde(v.Pos(), "IsHuman: false")
					}
				}
				return true
			})
		}
	}
	if gefunden == 0 {
		t.Fatal("die erlaubte Stelle in clearRegistrationsFromDB wurde nicht gefunden -- der Waechter sucht falsch")
	}
}

// falschWert: false oder eine Verneinung (!x).
func falschWert(e ast.Expr) bool {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name == "false"
	case *ast.UnaryExpr:
		return v.Op == token.NOT
	case *ast.ParenExpr:
		return falschWert(v.X)
	}
	return false
}
