package keeper

// Herkunftsnotiz ueber einen Neustart hinweg (Audit von null, 02.10.2026).
//
// prove_provenance.go haelt das Paar (Nullifier, Wallet) nach einem
// erfolgreichen /prove 15 Minuten im Arbeitsspeicher. Startete der Knoten in
// dieser Zeit neu, war die Notiz weg und die Registrierung scheiterte: kein
// Loch (die Pruefung schloss ab), aber ein Mensch, der Gesicht und Beweis noch
// einmal machen musste. Gemessen lief der Primary am 02.10. erst fuenf Minuten.
//
// Was sich NICHT aendert: Die Notiz bleibt knotenlokal und kurzlebig. Sie ist
// kein Kettenzustand, wird nicht repliziert und nicht im Snapshot getragen.
// Die Tabelle ist nur ein zweites Gedaechtnis desselben Knotens.
//
// Sicherheit:
//   - Gelesen wird die Tabelle nur, wenn der Arbeitsspeicher nichts weiss.
//     Jeder Fehler (keine Verbindung, Zeitueberschreitung, kaputte Zeile)
//     heisst "keine Herkunft" -- die Pruefung schliesst ab.
//   - Dieselben Regeln wie im Arbeitsspeicher: kanonischer Nullifier, genau
//     diese Wallet, nicht aelter als proveHerkunftTTL.
//   - Nur nach einem Neustart: Die Tabelle kann nur Notizen liefern, die vor
//     dem Start geschrieben wurden, und die verfallen spaetestens
//     proveHerkunftTTL nach dem Start. Danach kennt der Arbeitsspeicher alles,
//     und /api/register fragt die Datenbank gar nicht mehr. Ohne diese Grenze
//     haette jede Registrierung mit erfundenem Nullifier eine
//     Datenbankanfrage ausgeloest (eigene Sicherheitspruefung 02.10.2026).
//   - Auch in diesem Fenster hoechstens proveHerkunftDBGleichzeitig Anfragen
//     zugleich; ist die Grenze voll, gibt es keine Herkunft.
//   - Begrenzt: jede Datenbankanfrage hat eine feste Wartezeit; alte Zeilen
//     werden bei jedem Schreiben geloescht, die Tabelle haelt also nur die
//     Notizen der letzten 15 Minuten. Wie viele das sind, begrenzen die
//     Ratenbremsen von /prove (Proof-Server je IP, Wallet und Bio-Hash).

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"time"
)

const (
	proveHerkunftDBWartezeit    = 2 * time.Second
	proveHerkunftDBGleichzeitig = 4
)

// proveHerkunftDBAb: wann die dauerhafte Ablage eingeschaltet wurde (Start
// des Knotens). Gelesen wird nur bis proveHerkunftTTL danach.
var proveHerkunftDBAb atomic.Int64

// proveHerkunftDBPlaetze begrenzt gleichzeitige Leseanfragen.
var proveHerkunftDBPlaetze = make(chan struct{}, proveHerkunftDBGleichzeitig)

// proveHerkunftDB: die Datenbank dieses Knotens, gesetzt von NewAPIServer.
// nil (Tests ohne Datenbank, Knoten ohne Postgres): nur Arbeitsspeicher, wie
// bisher.
var proveHerkunftDB atomic.Pointer[sql.DB]

// richteProveHerkunftDBEin legt die Tabelle an und schaltet die dauerhafte
// Ablage ein. Scheitert das Anlegen, bleibt es beim Arbeitsspeicher.
func richteProveHerkunftDBEin(db *sql.DB) {
	if db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS prove_herkunft (
		nullifier TEXT PRIMARY KEY,
		wallet    TEXT NOT NULL,
		zeit_unix BIGINT NOT NULL)`); err != nil {
		fmt.Printf("[PROVE] Herkunftstabelle nicht angelegt, nur Arbeitsspeicher: %v\n", err)
		return
	}
	proveHerkunftDBAb.Store(time.Now().UnixNano())
	proveHerkunftDB.Store(db)
}

// speichereProveHerkunftDB schreibt die Notiz und raeumt Abgelaufenes weg.
// Fehler werden geloggt und sonst ignoriert: die Notiz im Arbeitsspeicher
// steht schon, und eine fehlende Zeile kann nur zu einem Abschliessen fuehren.
func speichereProveHerkunftDB(schluessel string, h herkunft) {
	db := proveHerkunftDB.Load()
	if db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), proveHerkunftDBWartezeit)
	defer cancel()
	if _, err := db.ExecContext(ctx, `INSERT INTO prove_herkunft (nullifier, wallet, zeit_unix) VALUES ($1, $2, $3)
		ON CONFLICT (nullifier) DO UPDATE SET wallet = $2, zeit_unix = $3`,
		schluessel, h.wallet, h.zeit.Unix()); err != nil {
		fmt.Printf("[PROVE] Herkunft nicht dauerhaft gespeichert (Arbeitsspeicher gilt): %v\n", err)
		return
	}
	grenze := h.zeit.Add(-proveHerkunftTTL).Unix()
	if _, err := db.ExecContext(ctx, `DELETE FROM prove_herkunft WHERE zeit_unix < $1`, grenze); err != nil {
		fmt.Printf("[PROVE] Abgelaufene Herkunft nicht geloescht: %v\n", err)
	}
}

// ladeProveHerkunftDB: die Notiz aus der Tabelle, nur wenn es sie gibt und
// alles lesbar ist. Sonst ok=false.
func ladeProveHerkunftDB(schluessel string) (herkunft, bool) {
	db := proveHerkunftDB.Load()
	if db == nil {
		return herkunft{}, false
	}
	// Nach dem Fenster weiss der Arbeitsspeicher alles: nicht fragen.
	if time.Since(time.Unix(0, proveHerkunftDBAb.Load())) > proveHerkunftTTL {
		return herkunft{}, false
	}
	select {
	case proveHerkunftDBPlaetze <- struct{}{}:
		defer func() { <-proveHerkunftDBPlaetze }()
	default:
		return herkunft{}, false // Grenze voll: keine Herkunft
	}
	ctx, cancel := context.WithTimeout(context.Background(), proveHerkunftDBWartezeit)
	defer cancel()
	var wallet string
	var zeit int64
	if err := db.QueryRowContext(ctx, `SELECT wallet, zeit_unix FROM prove_herkunft WHERE nullifier = $1`, schluessel).Scan(&wallet, &zeit); err != nil {
		return herkunft{}, false
	}
	if zeit <= 0 || !isValidWalletAddr(wallet) {
		return herkunft{}, false
	}
	return herkunft{zeit: time.Unix(zeit, 0), wallet: wallet}, true
}
