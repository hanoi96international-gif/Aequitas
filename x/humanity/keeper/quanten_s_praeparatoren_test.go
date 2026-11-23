package keeper

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// Weg B, Register der Praeparatoren (quanten_s_praeparatoren.go): ein
// annehmender und ein nachspielender Knoten im Gleichschritt (qwWelt).

func neuePraepWelt(t *testing.T) *qwWelt {
	t.Helper()
	w := neueQwWelt(t, 64)
	quantenSWegBAbOverride.Store(qwT0 - 86400)
	t.Cleanup(func() { quantenSWegBAbOverride.Store(0) })
	return w
}

func praepTx(t *testing.T, art, wallet string, k *ecdsa.PrivateKey, zeit int64) Transaction {
	t.Helper()
	K := quantenSPraepK(wallet)
	return Transaction{Type: art, Wallet: wallet, DistributionAt: zeit,
		QuantenW: &QuantenWAngaben{K: K, WalletSig: personalSign(t, k, quantenSPraepNachricht(art, wallet, K, zeit))}}
}

func praepAufraeumenTx(konto string) Transaction {
	return Transaction{Type: quantenSPraepAbmeldenTxArt, QuantenW: &QuantenWAngaben{K: quantenSPraepK(konto)}}
}

func (w *qwWelt) praepRegel(tx Transaction, zeit int64) string {
	w.t.Helper()
	u := w.bescheid(w.cs, tx, zeit)
	// Der nachspielende Knoten kommt zum selben Ergebnis.
	if u2 := w.bescheid(w.nach, tx, zeit); u2.regel != u.regel {
		w.t.Fatalf("%s: Knoten uneins (%q / %q)", tx.Type, u.regel, u2.regel)
	}
	return u.regel
}

func (w *qwWelt) abbild(cs *ChainState, T int64) []string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.quantenSPraepAbbildLocked(context.Background(), cs.quantenWJetzt(), T)
}

// Gutfall: melden, Vorlauf, abmelden; das Abbild zur Bestellung bleibt.
func TestQuantenSPraep_Gutfall(t *testing.T) {
	w := neuePraepWelt(t)
	z := qwT0
	// Anna unterschreibt 10 min vor der Buchung: es zaehlt die Buchzeit (eine
	// zurueckdatierte Unterschrift verkuerzt den Vorlauf nicht).
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z-600), z)
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.bert, w.keys[w.bert], z+10), z+10)
	e := w.cs.quantenWJetzt().Praep[quantenSPraepK(w.anna)]
	if e.Konto != w.anna || e.Gemeldet != z || e.Abgemeldet != 0 {
		t.Fatalf("Eintrag: %+v", e)
	}
	if a := w.abbild(w.cs, z+quantenSPraepVorlauf-1); len(a) != 0 {
		t.Fatalf("vor dem Vorlauf im Los: %v", a)
	}
	if a := w.abbild(w.cs, z+quantenSPraepVorlauf); !reflect.DeepEqual(a, []string{w.anna}) {
		t.Fatalf("nach dem Vorlauf: %v", a)
	}
	bestellung := z + quantenSPraepVorlauf + 100
	vorher := w.abbild(w.cs, bestellung)
	if len(vorher) != 2 {
		t.Fatalf("Abbild zur Bestellung: %v", vorher)
	}
	// Abmelden NACH der Bestellung verschiebt das Abbild dieser Bestellung
	// nicht (Angriff aus s_weg_b.py: Abmelden nach bekanntem Zufall).
	ab := bestellung + 50
	w.annehmen(praepTx(t, quantenSPraepAbmeldenTxArt, w.anna, w.keys[w.anna], ab), ab)
	if a := w.abbild(w.cs, bestellung); !reflect.DeepEqual(a, vorher) {
		t.Fatalf("Abmeldung nach der Bestellung verschiebt das Abbild: %v statt %v", a, vorher)
	}
	if a := w.abbild(w.cs, ab); !reflect.DeepEqual(a, []string{w.bert}) {
		t.Fatalf("nach der Abmeldung: %v", a)
	}
	if strings.Count(w.cs.quantenWJetzt().kodiert(), `"konto"`) != 2 {
		t.Fatal("Abgemeldeter Eintrag muss stehen bleiben")
	}
}

// Schlafend: vor dem Stichtag von Weg B unbekannt, Befund, 409, und die
// Kodierung des Registers ist byte-gleich (kein Feld praep).
func TestQuantenSPraep_Schlafend(t *testing.T) {
	w := neueQwWelt(t, 64) // Variante W aktiv, Weg B nicht (MaxInt64)
	z := qwT0
	for _, art := range []string{quantenSPraepMeldenTxArt, quantenSPraepAbmeldenTxArt} {
		if bekannteTxArt(art, z) {
			t.Errorf("%s vor dem Stichtag bekannt", art)
		}
		a := &APIServer{state: newTestState()}
		quantenWAbOverride.Store(1)
		rec := httptest.NewRecorder()
		a.handleQuantenW(art)(rec, httptest.NewRequest(http.MethodPost, "/api/quanten-w/x", strings.NewReader(`{}`)))
		if rec.Code != http.StatusConflict {
			t.Errorf("%s: Endpunkt %d statt 409", art, rec.Code)
		}
		quantenWAbOverride.Store(qwT0 - 86400)
	}
	tx := praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z)
	if r := w.praepRegel(tx, z); r != "quanten_s_praep_inaktiv" {
		t.Fatalf("vor dem Stichtag: %q", r)
	}
	tx.BuchAt = z
	if err := w.nachspielen(tx, z); err == nil {
		t.Fatal("Block mit Melden vor dem Stichtag nachgespielt")
	}
	// Mit einer Note im Register: keine Spur von Weg B in den Bytes.
	w.annehmen(w.praegenTx(z, 10_000_000), z)
	if k := w.cs.quantenWJetzt().kodiert(); strings.Contains(k, "praep") || k == "" {
		t.Fatalf("Kodierung vor dem Stichtag veraendert: %s", k)
	}
	if neuesQuantenWRegister().kodiert() != "" {
		t.Fatal("leeres Register nicht leer kodiert")
	}
	quantenSWegBAbOverride.Store(z + 3600)
	t.Cleanup(func() { quantenSWegBAbOverride.Store(0) })
	if !bekannteTxArt(quantenSPraepMeldenTxArt, z+3600) || bekannteTxArt(quantenSPraepMeldenTxArt, z+3599) {
		t.Fatal("Stichtag von Weg B wirkt nicht genau")
	}
	if _, da := quantenWStatus()["weg_b_ab"]; !da {
		t.Fatal("Stichtag fehlt im Status")
	}
}

// Melden: nur ein Mensch, nur fuer sich selbst, nur einmal, nicht alt.
func TestQuantenSPraep_MeldenMissbrauch(t *testing.T) {
	w := neuePraepWelt(t)
	z := qwT0
	fremd, fremdAdr := neuerSchluessel(t) // kein Mensch
	w.cs.ensureAccountLoadedCtx(context.Background(), fremdAdr)

	faelle := []struct {
		name, regel string
		tx          Transaction
		zeit        int64
	}{
		{"kein Mensch", "quanten_s_praep_mensch", praepTx(t, quantenSPraepMeldenTxArt, fremdAdr, fremd, z), z},
		{"fremder Schluessel", "quanten_w_unterschrift", praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.bert], z), z},
		{"alte Unterschrift", "quanten_w_zeit", praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z-quantenWAlt-1), z},
		{"Zeit weit voraus", "quanten_w_zeit", praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z+quantenWVoraus+1), z},
		{"Systemadresse", "quanten_w_form", praepTx(t, quantenSPraepMeldenTxArt, ubiPoolAddr, w.keys[w.anna], z), z},
	}
	// K eines anderen Kontos: Anna meldet unter Berts Schluessel.
	kFremd := praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z)
	kFremd.QuantenW.K = quantenSPraepK(w.bert)
	kFremd.QuantenW.WalletSig = personalSign(t, w.keys[w.anna], quantenSPraepNachricht(quantenSPraepMeldenTxArt, w.anna, kFremd.QuantenW.K, z))
	faelle = append(faelle, struct {
		name, regel string
		tx          Transaction
		zeit        int64
	}{"K eines anderen", "quanten_w_form", kFremd, z})
	// Fremde Felder (z. B. eine Stellenzahl) und eine Nummer.
	mitN := praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z)
	mitN.QuantenW.N = 5
	mitIndex := praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z)
	mitIndex.QuantenW.Index = 1
	mitTo := praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z)
	mitTo.To = w.bert
	// Unterschrift fuer Abmelden als Melden eingereicht (Domaene).
	falscheArt := praepTx(t, quantenSPraepAbmeldenTxArt, w.anna, w.keys[w.anna], z)
	falscheArt.Type = quantenSPraepMeldenTxArt
	for _, f := range []struct {
		name string
		tx   Transaction
		r    string
	}{{"fremdes Feld", mitN, "quanten_w_form"}, {"Nummer", mitIndex, "quanten_w_form"}, {"Empfaenger", mitTo, "quanten_w_form"},
		{"Abmelde-Unterschrift", falscheArt, "quanten_w_unterschrift"}} {
		faelle = append(faelle, struct {
			name, regel string
			tx          Transaction
			zeit        int64
		}{f.name, f.r, f.tx, z})
	}
	for _, f := range faelle {
		if r := w.praepRegel(f.tx, f.zeit); r != f.regel {
			t.Errorf("%s: Regel %q, erwartet %q", f.name, r, f.regel)
		}
	}
	if len(w.cs.quantenWJetzt().Praep) != 0 {
		t.Fatal("abgewiesene Meldung im Register")
	}

	// Zweimal melden: abgewiesen, auch nach dem Abmelden bis zum Nachlauf.
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z), z)
	if r := w.praepRegel(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z+5), z+5); r != "quanten_s_praep_doppelt" {
		t.Fatalf("zweimal gemeldet: %q", r)
	}
	ab := z + 100
	w.annehmen(praepTx(t, quantenSPraepAbmeldenTxArt, w.anna, w.keys[w.anna], ab), ab)
	spaeter := ab + quantenSPraepNachlauf - 1
	if r := w.praepRegel(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], spaeter), spaeter); r != "quanten_s_praep_doppelt" {
		t.Fatalf("vor dem Nachlauf neu gemeldet: %q", r)
	}
	// Nach dem Nachlauf: vergessen, neu melden erlaubt -- die ALTE Meldung
	// laesst sich nicht wieder einspielen (ihre Zeit ist laengst zu alt).
	neu := ab + quantenSPraepNachlauf
	if r := w.praepRegel(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z), neu); r != "quanten_w_zeit" {
		t.Fatalf("alte Meldung wieder eingespielt: %q", r)
	}
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], neu), neu)
	if e := w.cs.quantenWJetzt().Praep[quantenSPraepK(w.anna)]; e.Gemeldet != neu || e.Abgemeldet != 0 {
		t.Fatalf("neu gemeldet: %+v", e)
	}
}

// Register voll: hoechstens quantenSPraepHoechstens Eintraege, auch
// abgemeldete zaehlen bis zum Nachlauf.
func TestQuantenSPraep_Voll(t *testing.T) {
	w := neuePraepWelt(t)
	z := qwT0
	reg := w.cs.quantenWJetzt().kopie()
	for i := 0; i < quantenSPraepHoechstens; i++ {
		_, a := neuerSchluessel(t)
		reg.Praep[quantenSPraepK(a)] = quantenSPraep{Konto: a, Gemeldet: z, Abgemeldet: z + 1}
	}
	for _, cs := range []*ChainState{w.cs, w.nach} {
		cs.mu.Lock()
		if err := cs.quantenWSetzenLocked(context.Background(), reg); err != nil {
			t.Fatal(err)
		}
		cs.mu.Unlock()
	}
	if r := w.praepRegel(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z+10), z+10); r != "quanten_s_praep_voll" {
		t.Fatalf("volles Register: %q", r)
	}
	// Nach dem Nachlauf der Abgemeldeten ist wieder Platz.
	spaet := z + 1 + quantenSPraepNachlauf
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], spaet), spaet)
	if n := len(w.cs.quantenWJetzt().Praep); n != 1 {
		t.Fatalf("nach dem Aufraeumen %d Eintraege", n)
	}
}

// Abmelden: nur das eigene Konto; ohne Wallet nur, wer kein Mensch mehr ist.
func TestQuantenSPraep_AbmeldenMissbrauch(t *testing.T) {
	w := neuePraepWelt(t)
	z := qwT0
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z), z)
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.bert, w.keys[w.bert], z), z)

	// Bert meldet Anna ab (unter Annas K, mit eigener Unterschrift).
	fremd := praepTx(t, quantenSPraepAbmeldenTxArt, w.bert, w.keys[w.bert], z+1)
	fremd.QuantenW.K = quantenSPraepK(w.anna)
	fremd.QuantenW.WalletSig = personalSign(t, w.keys[w.bert], quantenSPraepNachricht(quantenSPraepAbmeldenTxArt, w.bert, fremd.QuantenW.K, z+1))
	// Anna mit Berts Schluessel.
	falsch := praepTx(t, quantenSPraepAbmeldenTxArt, w.anna, w.keys[w.bert], z+1)
	// Ohne Wallet, obwohl Anna ein Mensch ist.
	ohne := praepAufraeumenTx(w.anna)
	// Ohne Wallet, aber mit Zeit oder Unterschrift.
	ohneZeit := praepAufraeumenTx(w.anna)
	ohneZeit.DistributionAt = z
	ohneSig := praepAufraeumenTx(w.anna)
	ohneSig.QuantenW.WalletSig = fremd.QuantenW.WalletSig
	// Wallet aus Leerzeichen bzw. in Grossbuchstaben.
	leer := praepTx(t, quantenSPraepAbmeldenTxArt, w.anna, w.keys[w.anna], z+1)
	leer.Wallet = " "
	gross := praepTx(t, quantenSPraepAbmeldenTxArt, w.anna, w.keys[w.anna], z+1)
	gross.Wallet = strings.ToUpper(w.anna[:2]) + w.anna[2:]
	// Unbekannter Praeparator.
	_, nie := neuerSchluessel(t)
	addHuman(w.cs, nie, 0)
	addHuman(w.nach, nie, 0)
	unbekannt := praepTx(t, quantenSPraepAbmeldenTxArt, nie, w.keys[w.anna], z+1)
	for _, f := range []struct {
		name string
		tx   Transaction
		r    string
	}{{"fremdes Konto", fremd, "quanten_s_praep_fremd"}, {"fremder Schluessel", falsch, "quanten_w_unterschrift"},
		{"ohne Wallet, Mensch", ohne, "quanten_s_praep_mensch"}, {"ohne Wallet mit Zeit", ohneZeit, "quanten_w_form"},
		{"ohne Wallet mit Unterschrift", ohneSig, "quanten_w_form"},
		{"Wallet aus Leerzeichen", leer, "quanten_w_form"}, {"Wallet nicht kanonisch", gross, "quanten_w_form"}, {"nie gemeldet", unbekannt, "quanten_s_praep_unbekannt"}} {
		if r := w.praepRegel(f.tx, z+1); r != f.r {
			t.Errorf("%s: Regel %q, erwartet %q", f.name, r, f.r)
		}
	}
	if e := w.cs.quantenWJetzt().Praep[quantenSPraepK(w.anna)]; e.Abgemeldet != 0 {
		t.Fatal("Anna wurde von einem Fremden abgemeldet")
	}
	// Zweimal abmelden.
	w.annehmen(praepTx(t, quantenSPraepAbmeldenTxArt, w.bert, w.keys[w.bert], z+2), z+2)
	if r := w.praepRegel(praepTx(t, quantenSPraepAbmeldenTxArt, w.bert, w.keys[w.bert], z+3), z+3); r != "quanten_s_praep_unbekannt" {
		t.Fatalf("zweimal abgemeldet: %q", r)
	}
}

// Umzug: die alte Wallet ist kein Mensch mehr. Sie faellt aus dem Abbild, und
// jeder darf ihren Eintrag ohne Unterschrift aufraeumen; die neue Wallet
// meldet sich selbst. Nie zwei Eintraege desselben Menschen im Abbild.
func TestQuantenSPraep_Umzug(t *testing.T) {
	w := neuePraepWelt(t)
	z := qwT0
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z), z)
	neuK, neuAdr := neuerSchluessel(t)
	for _, cs := range []*ChainState{w.cs, w.nach} {
		alt, _ := cs.accounts.Get(w.anna)
		alt.IsHuman = false // wie umzugVollziehenLocked
		addHuman(cs, neuAdr, 0)
	}
	T := z + quantenSPraepVorlauf + 10
	if a := w.abbild(w.cs, T); len(a) != 0 {
		t.Fatalf("alte Wallet nach dem Umzug im Abbild: %v", a)
	}
	w.annehmen(praepAufraeumenTx(w.anna), z+20)
	if e := w.cs.quantenWJetzt().Praep[quantenSPraepK(w.anna)]; e.Abgemeldet != z+20 {
		t.Fatalf("Aufraeumen: %+v", e)
	}
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, neuAdr, neuK, z+30), z+30)
	if a := w.abbild(w.cs, z+30+quantenSPraepVorlauf); !reflect.DeepEqual(a, []string{neuAdr}) {
		t.Fatalf("nach dem Umzug: %v", a)
	}
	// Die alte Wallet kann sich nicht neu melden (kein Mensch).
	if r := w.praepRegel(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], z+40+quantenSPraepNachlauf), z+40+quantenSPraepNachlauf); r != "quanten_s_praep_mensch" {
		t.Fatalf("alte Wallet meldet sich neu: %q", r)
	}
}

// Ueber den echten Annahmeweg (quantenWAnnehmen) und den Endpunkt zum Lesen.
func TestQuantenSPraep_AnnahmeUndAnsicht(t *testing.T) {
	w := neuePraepWelt(t)
	quantenSWegBAbOverride.Store(1)
	quantenWAbOverride.Store(1)
	jetzt := nowUnix()
	fertig, err := w.cs.quantenWAnnehmen(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], jetzt))
	if err != nil {
		t.Fatal(err)
	}
	if err := w.nachspielen(fertig, fertig.BuchAt+3); err != nil {
		t.Fatalf("Nachspielen: %v", err)
	}
	if w.cs.quantenWJetzt().kodiert() != w.nach.quantenWJetzt().kodiert() || w.cs.quantenWWurzel() == "" {
		t.Fatal("Register weichen ab oder fehlen in der StateRoot")
	}
	// Derselbe Satz noch einmal (Wiederholung): abgewiesen.
	if _, err := w.cs.quantenWAnnehmen(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], jetzt)); err == nil {
		t.Fatal("Wiederholung angenommen")
	}
	a := &APIServer{state: w.cs}
	rec := httptest.NewRecorder()
	a.handleQuantenSPraeparatoren(rec, httptest.NewRequest(http.MethodGet, "/api/quanten-w/praeparatoren", nil))
	var antwort struct {
		Aktiv         bool                     `json:"aktiv"`
		Praeparatoren []map[string]interface{} `json:"praeparatoren"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &antwort); err != nil || !antwort.Aktiv || len(antwort.Praeparatoren) != 1 ||
		antwort.Praeparatoren[0]["konto"] != w.anna {
		t.Fatalf("Ansicht: %s (%v)", rec.Body.String(), err)
	}
	rec = httptest.NewRecorder()
	a.handleQuantenSPraeparatoren(rec, httptest.NewRequest(http.MethodPost, "/api/quanten-w/praeparatoren", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST auf die Ansicht: %d", rec.Code)
	}
}

// Register lesen: was so nie entstehen konnte, wird abgewiesen; was entstand,
// liest sich byte-gleich zurueck (auch ueber die Zeilen der Tabelle).
func TestQuantenSPraep_RegisterLesen(t *testing.T) {
	_, a := neuerSchluessel(t)
	_, b := neuerSchluessel(t)
	_, c := neuerSchluessel(t)
	gut := []quantenSPraepForm{{K: quantenSPraepK(a), Konto: a, Gemeldet: 100}, {K: quantenSPraepK(b), Konto: b, Gemeldet: 100, Abgemeldet: 200}}
	if gut[0].K > gut[1].K {
		gut[0], gut[1] = gut[1], gut[0]
	}
	roh := quantenWRegisterForm{Praep: gut}.kodiert()
	r, err := quantenWRegisterLesen(roh)
	if err != nil || r.kodiert() != roh {
		t.Fatalf("Rundweg: %v", err)
	}
	if z := r.form().zeilen(); len(z) != 2 || z[quantenWZeile{quantenWArtPraep, gut[0].K}] == "" {
		t.Fatalf("Zeilen: %v", z)
	}
	kaputt := map[string]func(f []quantenSPraepForm) []quantenSPraepForm{
		"K passt nicht": func(f []quantenSPraepForm) []quantenSPraepForm { f[0].K = quantenSPraepK(c); return f },
		"Ordnung":       func(f []quantenSPraepForm) []quantenSPraepForm { f[0], f[1] = f[1], f[0]; return f },
		"doppelt":       func(f []quantenSPraepForm) []quantenSPraepForm { f[1] = f[0]; return f },
		"Konto gross":   func(f []quantenSPraepForm) []quantenSPraepForm { f[0].Konto = strings.ToUpper(f[0].Konto); return f },
		"ohne Meldung":  func(f []quantenSPraepForm) []quantenSPraepForm { f[0].Gemeldet = 0; return f },
		"vor Meldung abgemeldet": func(f []quantenSPraepForm) []quantenSPraepForm {
			f[0].Abgemeldet = f[0].Gemeldet - 1
			return f
		},
		"negativ abgemeldet": func(f []quantenSPraepForm) []quantenSPraepForm { f[0].Abgemeldet = -1; return f },
	}
	for name, aendern := range kaputt {
		f := aendern(append([]quantenSPraepForm(nil), gut...))
		if _, err := quantenWRegisterLesen(quantenWRegisterForm{Praep: f}.kodiert()); err == nil {
			t.Errorf("%s: angenommen", name)
		}
	}
	if _, err := quantenWRegisterLesen(`{"praep":[{"k":"` + gut[0].K + `","konto":"` + gut[0].Konto + `","gemeldet":1,"x":1}]}`); err == nil {
		t.Error("unbekanntes Feld angenommen")
	}
	var zuViele []quantenSPraepForm
	for i := 0; i <= quantenSPraepHoechstens; i++ {
		_, k := neuerSchluessel(t)
		zuViele = append(zuViele, quantenSPraepForm{K: quantenSPraepK(k), Konto: k, Gemeldet: 1})
	}
	if _, err := quantenSPraepLesen(zuViele); err == nil {
		t.Error("zu viele Praeparatoren angenommen")
	}
}

// Gegen eine echte Datenbank: je Praeparator eine Zeile der Art praep,
// Neustart liest dasselbe Register, Abmelden schreibt nur die eine Zeile,
// eine manipulierte Zeile wird beim Laden abgewiesen.
func TestQuantenSPraep_Tabelle_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-quanten-s-praep-realdb-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	ctx := context.Background()
	r := neuesQuantenWRegister()
	var konten []string
	for i := 0; i < 3; i++ {
		_, a := neuerSchluessel(t)
		konten = append(konten, a)
		r.Praep[quantenSPraepK(a)] = quantenSPraep{Konto: a, Gemeldet: qwT0 + int64(i)}
	}
	if err := qwtSetzen(t, cs, ctx, r); err != nil {
		t.Fatal(err)
	}
	z1 := qwtZeilen(t, cs)
	if len(z1) != 3 {
		t.Fatalf("%d Zeilen statt 3", len(z1))
	}
	for _, z := range z1 {
		if z.art != quantenWArtPraep {
			t.Fatalf("Zeile der Art %q", z.art)
		}
	}
	wurzel := cs.quantenWWurzel()
	neu := testKnoten(t, "unused-quanten-s-praep-realdb-test-2.json")
	if neu.quantenWWurzel() != wurzel || wurzel == "" || neu.quantenWStandJetzt().kodiert != r.kodiert() {
		t.Fatal("nach dem Neustart ein anderes Register")
	}
	r2 := r.kopie()
	e := r2.Praep[quantenSPraepK(konten[0])]
	e.Abgemeldet = qwT0 + 100
	r2.Praep[quantenSPraepK(konten[0])] = e
	if err := qwtSetzen(t, cs, ctx, r2); err != nil {
		t.Fatal(err)
	}
	for k, z := range qwtZeilen(t, cs) {
		geaendert := z1[k].xmin != z.xmin
		if geaendert != (k == quantenWArtPraep+"|"+quantenSPraepK(konten[0])) {
			t.Fatalf("Zeile %s: geaendert = %v", k, geaendert)
		}
	}
	// Manipuliert: K der Zeile gehoert zu einem anderen Konto.
	if _, err := cs.db.Exec(`UPDATE `+quantenWTabelle+` SET daten = replace(daten, $1, $2) WHERE art = $3 AND k = $4`,
		konten[1], konten[2], quantenWArtPraep, quantenSPraepK(konten[1])); err != nil {
		t.Fatal(err)
	}
	if err := qwtLaden(cs); err == nil {
		t.Fatal("manipulierte Zeile geladen")
	}
}
