package keeper

// HTTP-Schnittstelle fuer Unternehmen (wirtschaft.go).
//
//   GET  /api/wirtschaft/regeln        alle Zahlen, damit Website und App sie nicht abschreiben
//   GET  /api/wirtschaft/konto?adresse= Kontoart, Freibetraege, Liegegeld-Vorschau
//   GET  /api/unternehmen              oeffentliches Register mit Monatssummen
//   POST /api/unternehmen/eroeffnen    signiert von Unternehmensadresse UND Mensch
//   POST /api/unternehmen/mitinhaber   signiert vom neuen Menschen UND einem Verantwortlichen
//   POST /api/unternehmen/schliessen   signiert von einem Verantwortlichen, Konto leer
//
// Die Nachrichten sind menschenlesbar (personal_sign in der Wallet) und tragen
// eine Zeit; angenommen wird nur innerhalb von 5 Minuten. Ein zweites
// Einreichen derselben Nachricht scheitert am Zustand (schon eingetragen).

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

func unternehmenEroeffnenNachricht(u, m, name, kategorie string, zeit int64) string {
	return fmt.Sprintf("Aequitas: Unternehmenskonto eroeffnen\nUnternehmen: %s\nVerantwortlich: %s\nName: %s\nKategorie: %s\nZeit: %d", u, m, name, kategorie, zeit)
}

func unternehmenMitinhaberNachricht(u, m string, zeit int64) string {
	return fmt.Sprintf("Aequitas: Mitinhaber aufnehmen\nUnternehmen: %s\nNeu verantwortlich: %s\nZeit: %d", u, m, zeit)
}

func unternehmenSchliessenNachricht(u string, zeit int64) string {
	return fmt.Sprintf("Aequitas: Unternehmenskonto schliessen\nUnternehmen: %s\nZeit: %d", u, zeit)
}

func zeitFrisch(zeit int64) bool {
	d := time.Now().Unix() - zeit
	return d <= 300 && d >= -60
}

func (a *APIServer) handleWirtschaftRegeln(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"aktiv":    wirtschaftAktiv(time.Now().Unix()),
		"aktiv_ab": wirtschaftAktivAbUnix,
		// Alle Betraege unten sind Vielfache davon (siehe wirtschaft.go).
		"fairer_anteil": registrationGrant,
		"in_fairen_anteilen": map[string]interface{}{
			"mensch_gebuehrenfreie_ausgaben_monat": menschFreiAusgabenMonat / registrationGrant,
			"mensch_tausch_frei_monat":             menschTauschFreiMonat / registrationGrant,
			"mensch_spar_freibetrag":               menschSparFreibetrag / registrationGrant,
			"mensch_vermoegensgrenze":              float64(wealthCapMultiplier),
			"unternehmen_sockel":                   unternehmenSockel / registrationGrant,
			"freie_adresse_grenze":                 freiGrenze / registrationGrant,
		},
		"mensch": map[string]interface{}{
			"gebuehrenfreie_ausgaben_monat": menschFreiAusgabenMonat,
			"tausch_frei_monat":             menschTauschFreiMonat,
			"spar_freibetrag":               menschSparFreibetrag,
			"umlauf_prozent_monat":          menschUmlaufMonat * 100,
			"vermoegensgrenze":              registrationGrant * wealthCapMultiplier,
		},
		"unternehmen": map[string]interface{}{
			"sockel": unternehmenSockel,
			// Freibetrag nach Umsatz (Konzept 14.2)
			"frei_bis_monatsumsaetze":          umsatzFreiFaktor,
			"liegegeld_prozent_monat":          liegeRate1Monat * 100,
			"liegegeld2_ab_monatsumsaetzen":    umsatzStufe2Faktor,
			"liegegeld2_prozent_monat":         liegeRate2Monat * 100,
			"umsatz_fenster_tage":              umsatzFensterTage,
			"umsatz_jahr_tage":                 umsatzJahrTage,
			"gruendung_tage":                   gruendungTage,
			"gruendung_einmal_je_mensch_tage":  gruendungAbstandTage,
			"eigene_einlage_abgabefrei":        true,
			"mensch_zaehlt_hoechstens_quartal": menschZaehltJeUntQuartal,
			"liegegeld_pruefung":               liegegeldPruefungStand(),
			"zwischen_unternehmen_zaehlt":      "ueberschuss",
			"max_je_mensch":                    maxUnternehmenJeMensch,
			"max_verantwortliche":              maxVerantwortlicheJeUnt,
			"kategorien":                       sortierteKategorien(),
		},
		"frei": map[string]interface{}{
			"grenze":               freiGrenze,
			"umlauf_prozent_monat": freiUmlaufMonat * 100,
		},
		"ausstiegsabgabe_prozent": float64(ausstiegsAbgabeBps) / 100,
		"ueberweisung_prozent":    float64(ueberweisungsGebuehrBps) / 100,
		"abgaben_gehen_an":        "grundeinkommen",
	})
}

func sortierteKategorien() []string {
	var k []string
	for x := range unternehmenKategorien {
		k = append(k, x)
	}
	sort.Strings(k)
	return k
}

func (a *APIServer) handleWirtschaftKonto(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	addr, ok := normAdresse(r.URL.Query().Get("adresse"))
	if !ok {
		jsonError(w, "adresse must be a 0x address", http.StatusBadRequest)
		return
	}
	cs := a.state
	jetzt := time.Now().Unix()
	cs.mu.RLock()
	acc, da := cs.accounts.Get(addr)
	var stand float64
	istMensch := false
	if da {
		stand = acc.Balance.Float()
		istMensch = acc.IsHuman
	}
	cs.mu.RUnlock()
	art := cs.kontoartVon(addr, istMensch)
	antwort := map[string]interface{}{
		"adresse": addr, "art": art.String(), "guthaben": stand,
		"aktiv": wirtschaftAktiv(jetzt),
	}
	wi := cs.wirt()
	wi.mu.Lock()
	k := wi.kontoLocked(addr, jetzt)
	switch art {
	case artMensch:
		antwort["gebuehrenfrei_rest_monat"] = round6(math.Max(0, menschFreiAusgabenMonat-k.Ausgegeben))
		antwort["tausch_frei_rest_monat"] = round6(math.Max(0, menschTauschFreiMonat-k.Getauscht))
		antwort["eigene_einlage_abgabefrei"] = round6(k.Eingezahlt)
		antwort["lohn_monat"] = round6(k.Lohn)
		antwort["unternehmen"] = wi.unternehmenVon(addr)
	case artUnternehmen:
		e := wi.unternehmen[addr]
		if e != nil {
			antwort["name"] = e.Name
			antwort["kategorie"] = e.Kategorie
			antwort["verantwortliche"] = len(e.Verantwortliche)
		}
		umsatz := wi.umsatzLocked(addr, jetzt)
		antwort["umsatz"] = map[string]interface{}{
			"monatsumsatz":                    round6(umsatz),
			"monatsumsatz_90_tage":            round6(wi.monatsUmsatzLocked(k, wi.mittelTageLocked(e, jetzt, umsatzFensterTage), jetzt)),
			"monatsumsatz_jahr":               round6(wi.monatsUmsatzLocked(k, wi.mittelTageLocked(e, jetzt, umsatzJahrTage), jetzt)),
			"in_gruendung":                    wi.inGruendungLocked(e, jetzt),
			"knoten_daten_tage":               wi.knotenTageLocked(jetzt),
			"frei_bis":                        round6(math.Max(unternehmenSockel, umsatzFreiFaktor*umsatz)),
			"hohe_stufe_ab":                   round6(math.Max(unternehmenSockel, umsatzStufe2Faktor*umsatz)),
			"mindestens_gemittelt_ueber_tage": umsatzMindestTage,
		}
		antwort["eigene_einlage_abgabefrei"] = round6(k.Eingezahlt)
		antwort["monat"] = map[string]float64{"einnahmen": round6(k.Einnahmen), "lohn_gezahlt": round6(k.LohnGezahlt), "entnahmen": round6(k.Entnahmen)}
	case artFrei:
		antwort["grenze"] = freiGrenze
	}
	wi.mu.Unlock()
	if art != artSystem {
		antwort["abgabe_pro_monat_bei_diesem_stand"] = cs.umlaufBetrag(addr, art, stand, jetzt, sekundenJeMonat)
	}
	json.NewEncoder(w).Encode(antwort)
}

func (a *APIServer) handleUnternehmenListe(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	cs := a.state
	jetzt := time.Now().Unix()
	liste := cs.unternehmenFuerSnapshot()
	out := make([]map[string]interface{}, 0, len(liste))
	for _, e := range liste {
		if !e.offen() {
			continue
		}
		cs.mu.RLock()
		var stand float64
		if acc, ok := cs.accounts.Get(e.Adresse); ok {
			stand = acc.Balance.Float()
		}
		cs.mu.RUnlock()
		wi := cs.wirt()
		wi.mu.Lock()
		k := wi.kontoLocked(e.Adresse, jetzt)
		monat := map[string]float64{"einnahmen": round6(k.Einnahmen), "lohn_gezahlt": round6(k.LohnGezahlt), "entnahmen": round6(k.Entnahmen)}
		wi.mu.Unlock()
		out = append(out, map[string]interface{}{
			"adresse": e.Adresse, "name": e.Name, "kategorie": e.Kategorie,
			"verantwortliche": len(e.Verantwortliche), "eroeffnet_am": e.EroeffnetAm,
			"guthaben": round6(stand), "monat": monat,
		})
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"aktiv": wirtschaftAktiv(jetzt), "aktiv_ab": wirtschaftAktivAbUnix,
		"anzahl": len(out), "unternehmen": out,
	})
}

// unternehmenEinreichen: gemeinsamer Weg fuer die drei POSTs.
func (a *APIServer) unternehmenEinreichen(w http.ResponseWriter, konten []string, tx Transaction, anwenden func(ctx context.Context) error) {
	cs := a.state
	if err := cs.annahmeBeginnen(konten[0]); err != nil {
		jsonError(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer cs.annahmeEnde()
	if err := cs.runAtomicWithOutbox(konten, false, func(ctx context.Context) (Transaction, error) {
		if err := anwenden(ctx); err != nil {
			return Transaction{}, err
		}
		return tx, nil
	}); err != nil {
		jsonError(w, strings.ReplaceAll(err.Error(), ": "+ErrZustandLehntAb.Error(), ""), http.StatusConflict)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "typ": tx.Type, "unternehmen": tx.Wallet})
}

func (a *APIServer) handleUnternehmenEroeffnen(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Unternehmen    string `json:"unternehmen"`
		Mensch         string `json:"mensch"`
		Name           string `json:"name"`
		Kategorie      string `json:"kategorie"`
		Zeit           int64  `json:"zeit"`
		SigUnternehmen string `json:"sig_unternehmen"`
		SigMensch      string `json:"sig_mensch"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	now := time.Now().Unix()
	if !wirtschaftAktiv(now) {
		jsonError(w, "business accounts are not active yet", http.StatusConflict)
		return
	}
	u, ok1 := normAdresse(req.Unternehmen)
	m, ok2 := normAdresse(req.Mensch)
	if !ok1 || !ok2 {
		jsonError(w, "unternehmen and mensch must be 0x addresses", http.StatusBadRequest)
		return
	}
	if !zeitFrisch(req.Zeit) {
		jsonError(w, "zeit expired or in the future", http.StatusBadRequest)
		return
	}
	name := normName(req.Name)
	kat := strings.ToLower(strings.TrimSpace(req.Kategorie))
	msg := unternehmenEroeffnenNachricht(u, m, name, kat, req.Zeit)
	if err := verifyPersonalSign(msg, req.SigUnternehmen, u); err != nil {
		jsonError(w, "business signature invalid: "+err.Error(), http.StatusForbidden)
		return
	}
	if err := verifyPersonalSign(msg, req.SigMensch, m); err != nil {
		jsonError(w, "human signature invalid: "+err.Error(), http.StatusForbidden)
		return
	}
	tx := Transaction{Type: "unternehmen_eroeffnen", Wallet: u, To: m, Name: name, Kategorie: kat}
	a.unternehmenEinreichen(w, []string{u, m}, tx, func(ctx context.Context) error {
		return a.state.applyUnternehmenEroeffnenLocked(ctx, u, m, name, kat, now)
	})
}

func (a *APIServer) handleUnternehmenMitinhaber(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Unternehmen       string `json:"unternehmen"`
		Mensch            string `json:"mensch"`
		Verantwortlich    string `json:"verantwortlich"`
		Zeit              int64  `json:"zeit"`
		SigMensch         string `json:"sig_mensch"`
		SigVerantwortlich string `json:"sig_verantwortlich"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	now := time.Now().Unix()
	u, ok1 := normAdresse(req.Unternehmen)
	m, ok2 := normAdresse(req.Mensch)
	v, ok3 := normAdresse(req.Verantwortlich)
	if !ok1 || !ok2 || !ok3 || !wirtschaftAktiv(now) {
		jsonError(w, "invalid addresses or not active yet", http.StatusBadRequest)
		return
	}
	if !zeitFrisch(req.Zeit) {
		jsonError(w, "zeit expired or in the future", http.StatusBadRequest)
		return
	}
	a.state.wirt().mu.Lock()
	e := a.state.wirt().offenesUnternehmenLocked(u)
	berechtigt := e != nil && e.istVerantwortlich(v)
	a.state.wirt().mu.Unlock()
	if !berechtigt {
		jsonError(w, "verantwortlich is not responsible for this business", http.StatusForbidden)
		return
	}
	msg := unternehmenMitinhaberNachricht(u, m, req.Zeit)
	if verifyPersonalSign(msg, req.SigMensch, m) != nil || verifyPersonalSign(msg, req.SigVerantwortlich, v) != nil {
		jsonError(w, "signatures invalid", http.StatusForbidden)
		return
	}
	tx := Transaction{Type: "unternehmen_mitinhaber", Wallet: u, To: m}
	a.unternehmenEinreichen(w, []string{u, m}, tx, func(ctx context.Context) error {
		return a.state.applyUnternehmenMitinhaberLocked(ctx, u, m, now)
	})
}

func (a *APIServer) handleUnternehmenSchliessen(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Unternehmen    string `json:"unternehmen"`
		Verantwortlich string `json:"verantwortlich"`
		Zeit           int64  `json:"zeit"`
		Sig            string `json:"sig"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	now := time.Now().Unix()
	u, ok1 := normAdresse(req.Unternehmen)
	v, ok2 := normAdresse(req.Verantwortlich)
	if !ok1 || !ok2 || !zeitFrisch(req.Zeit) {
		jsonError(w, "invalid addresses or zeit", http.StatusBadRequest)
		return
	}
	a.state.wirt().mu.Lock()
	e := a.state.wirt().offenesUnternehmenLocked(u)
	berechtigt := e != nil && e.istVerantwortlich(v)
	a.state.wirt().mu.Unlock()
	if !berechtigt {
		jsonError(w, "not responsible for this business", http.StatusForbidden)
		return
	}
	if err := verifyPersonalSign(unternehmenSchliessenNachricht(u, req.Zeit), req.Sig, v); err != nil {
		jsonError(w, "signature invalid: "+err.Error(), http.StatusForbidden)
		return
	}
	tx := Transaction{Type: "unternehmen_schliessen", Wallet: u, To: v}
	a.unternehmenEinreichen(w, []string{u}, tx, func(ctx context.Context) error {
		return a.state.applyUnternehmenSchliessenLocked(ctx, u, now)
	})
}
