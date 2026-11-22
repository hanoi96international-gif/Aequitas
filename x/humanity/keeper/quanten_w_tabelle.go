package keeper

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// REGISTER DER VARIANTE W JE ZEILE (vor W6, 16.11.2026)
//
// Bis hierhin stand das ganze Register von W und S als EIN JSON-Wert in
// chain_config (quantenWKonfigSchluessel) und wurde bei jeder W- oder
// S-Transaktion ganz neu geschrieben. Mit 16 Noten zu je bis zu 16.384
// Stellen und offener Anfrage (Stellen, Basen, Antwort) sind das einige
// Megabyte -- je Teil der Offenlegung, bis zu 256 Teile je Pruefung. Jetzt:
//
//   - jede lebende Note, jeder Token und jede tote Note eine Zeile in
//     quantenWTabelle (Art, K, Daten = die Form des Eintrags als JSON,
//     dieselben Bytes wie im ganzen Register);
//   - geschrieben wird nur, was sich geaendert hat (quantenWZeilenSchreiben:
//     entfernte Zeilen loeschen, neue und geaenderte schreiben);
//   - die StateRoot ist byte-gleich (SHA-256 ueber das ganze Register als
//     JSON, einmal je Stand gerechnet);
//   - der Snapshot traegt das Register unveraendert als einen Wert
//     (ChainConfig[quantenWKonfigSchluessel], aus dem Speicher zum selben
//     Zeitpunkt wie die Konten); beim Laden zieht er in die Tabelle um
//     (quantenWLaden).
//
// Gelesen wird fail-closed: jede Zeile streng (unbekannte Felder, K der
// Zeile = K im JSON, bekannte Art), hoechstens quantenWZeilenHoechstens
// Zeilen, und das zusammengesetzte Register noch einmal ganz durch
// quantenWRegisterLesen (Grenzen, Ordnung, Summe der Anteile).

const (
	quantenWTabelle = "quanten_w_eintraege"
	quantenWArtNote = "note"
	quantenWArtTok  = "token"
	quantenWArtTot  = "tot"
	// So viele Zeilen kann es geben (quantenWRegisterLesen: Tote bis zur
	// Grenze plus die, die danach noch sterben).
	quantenWZeilenHoechstens = quantenWNotenHoechstens + quantenWTokenHoechstens +
		quantenWToteHoechstens + quantenWNotenHoechstens + quantenWTokenHoechstens + quantenSPraepHoechstens
)

// quantenWZeile: Schluessel einer Zeile.
type quantenWZeile struct {
	art, k string
}

// zeilen: je Eintrag seine Zeile.
func (f quantenWRegisterForm) zeilen() map[quantenWZeile]string {
	out := make(map[quantenWZeile]string, len(f.Noten)+len(f.Token)+len(f.Tote))
	dazu := func(art, k string, v interface{}) {
		roh, err := json.Marshal(v)
		if err != nil {
			panic(fmt.Sprintf("quanten_w: Zeile nicht kodierbar: %v", err)) // nur Zahlen, Texte, Listen
		}
		out[quantenWZeile{art, k}] = string(roh)
	}
	for _, n := range f.Noten {
		dazu(quantenWArtNote, n.K, n)
	}
	for _, t := range f.Token {
		dazu(quantenWArtTok, t.K, t)
	}
	for _, t := range f.Tote {
		dazu(quantenWArtTot, t.K, t)
	}
	for _, p := range f.Praep {
		dazu(quantenWArtPraep, p.K, p)
	}
	return out
}

func quantenWZeilenSortiert(m map[quantenWZeile]bool) []quantenWZeile {
	out := make([]quantenWZeile, 0, len(m))
	for z := range m {
		out = append(out, z)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].art != out[j].art {
			return out[i].art < out[j].art
		}
		return out[i].k < out[j].k
	})
	return out
}

// quantenWZeilenSchreiben: vom Stand alt (wie er in der Tabelle steht) zum
// Stand neu -- entfernte Zeilen loeschen, neue und geaenderte schreiben, in
// fester Reihenfolge. Jede Anweisung trifft genau eine Zeile, sonst Fehler
// (Speicher und Tabelle waeren auseinander).
func quantenWZeilenSchreiben(db sqlExecutor, alt, neu map[quantenWZeile]string) error {
	weg, dazu := map[quantenWZeile]bool{}, map[quantenWZeile]bool{}
	for z := range alt {
		if _, da := neu[z]; !da {
			weg[z] = true
		}
	}
	for z, d := range neu {
		if a, da := alt[z]; !da || a != d {
			dazu[z] = true
		}
	}
	for _, z := range quantenWZeilenSortiert(weg) {
		res, err := db.Exec(`DELETE FROM `+quantenWTabelle+` WHERE art = $1 AND k = $2`, z.art, z.k)
		if err != nil {
			return fmt.Errorf("quanten_w: Zeile %s %s loeschen: %w", z.art, kurzHash(z.k), err)
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			return fmt.Errorf("quanten_w: Zeile %s %s: %d geloescht (%v), erwartet 1", z.art, kurzHash(z.k), n, err)
		}
	}
	for _, z := range quantenWZeilenSortiert(dazu) {
		res, err := db.Exec(`INSERT INTO `+quantenWTabelle+` (art, k, daten) VALUES ($1, $2, $3)
ON CONFLICT (art, k) DO UPDATE SET daten = EXCLUDED.daten`, z.art, z.k, neu[z])
		if err != nil {
			return fmt.Errorf("quanten_w: Zeile %s %s schreiben: %w", z.art, kurzHash(z.k), err)
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			return fmt.Errorf("quanten_w: Zeile %s %s: %d geschrieben (%v), erwartet 1", z.art, kurzHash(z.k), n, err)
		}
	}
	return nil
}

// quantenWZeileLesen: eine Zeile streng in v -- keine unbekannten Felder,
// nichts dahinter.
func quantenWZeileLesen(daten string, v interface{}) error {
	dec := json.NewDecoder(strings.NewReader(daten))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("Daten nach dem Eintrag")
	}
	return nil
}

// quantenWZeilenLesen: alle Zeilen als Kodierung des ganzen Registers (""
// ohne Zeilen) und ihre Zahl. Ob das Register so entstehen konnte, prueft
// danach quantenWRegisterLesen.
func quantenWZeilenLesen(db sqlExecutor) (string, int, error) {
	rows, err := db.Query(`SELECT art, k, daten FROM `+quantenWTabelle+` LIMIT $1`, quantenWZeilenHoechstens+1)
	if err != nil {
		return "", 0, fmt.Errorf("quanten_w: Tabelle lesen: %w", err)
	}
	defer rows.Close()
	var f quantenWRegisterForm
	n := 0
	for rows.Next() {
		n++
		if n > quantenWZeilenHoechstens {
			return "", 0, fmt.Errorf("quanten_w: mehr als %d Zeilen", quantenWZeilenHoechstens)
		}
		var art, k, daten string
		if err := rows.Scan(&art, &k, &daten); err != nil {
			return "", 0, fmt.Errorf("quanten_w: Zeile lesen: %w", err)
		}
		var kImEintrag string
		switch art {
		case quantenWArtNote:
			var e quantenWNoteForm
			err = quantenWZeileLesen(daten, &e)
			kImEintrag = e.K
			f.Noten = append(f.Noten, e)
		case quantenWArtTok:
			var e quantenWSTokenForm
			err = quantenWZeileLesen(daten, &e)
			kImEintrag = e.K
			f.Token = append(f.Token, e)
		case quantenWArtTot:
			var e quantenWTotForm
			err = quantenWZeileLesen(daten, &e)
			kImEintrag = e.K
			f.Tote = append(f.Tote, e)
		case quantenWArtPraep:
			var e quantenSPraepForm
			err = quantenWZeileLesen(daten, &e)
			kImEintrag = e.K
			f.Praep = append(f.Praep, e)
		default:
			return "", 0, fmt.Errorf("quanten_w: Zeile mit unbekannter Art %q", art)
		}
		if err != nil || kImEintrag != k {
			return "", 0, fmt.Errorf("quanten_w: Zeile %s %s: Form (%v)", art, kurzHash(k), err)
		}
	}
	if err := rows.Err(); err != nil {
		return "", 0, fmt.Errorf("quanten_w: Tabelle lesen: %w", err)
	}
	if n == 0 {
		return "", 0, nil
	}
	sort.Slice(f.Noten, func(i, j int) bool { return f.Noten[i].K < f.Noten[j].K })
	sort.Slice(f.Token, func(i, j int) bool { return f.Token[i].K < f.Token[j].K })
	sort.Slice(f.Tote, func(i, j int) bool { return f.Tote[i].K < f.Tote[j].K })
	sort.Slice(f.Praep, func(i, j int) bool { return f.Praep[i].K < f.Praep[j].K })
	return f.kodiert(), n, nil
}
