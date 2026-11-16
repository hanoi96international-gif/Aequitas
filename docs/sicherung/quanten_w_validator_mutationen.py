import subprocess
R = '/tmp/claude-0/-home-user/eeac440a-ca8c-5963-ab61-835c6132d178/scratchpad/mutrepo/x/humanity/keeper/'
F = R + 'quanten_w_validator.go'
orig = open(F).read()
ENDPUNKT_ALT = '''	jetzt := time.Now().Unix()
	if !quantenWAktiv(jetzt) {
		jsonError(w, "quantum money variant W not active", http.StatusConflict)
		return
	}
	v := a.quantenWValFuer()
	if v == nil {
		jsonError(w, "this node is not a validator", http.StatusServiceUnavailable)
		return
	}
	if !burstErlaubt("quanten-w-val:"+erneuerungsAbsender(r), burstQuantenWValJeIP, burstFenster) {
		jsonError(w, "too many requests from this address", http.StatusTooManyRequests)
		return
	}
	if r.ContentLength'''
ENDPUNKT_NEU = '''	jetzt := time.Now().Unix()
	v := a.quantenWValFuer()
	if v == nil {
		jsonError(w, "this node is not a validator", http.StatusServiceUnavailable)
		return
	}
	if r.ContentLength'''
M = [
    ("reif ohne Finalitaet", "		if v.final() < s.hoehe {\n			return false, 0\n		}\n", ""),
    ("Anteile ohne Reife", "	if reif, _ := v.reifLocked(quantenWAntwortSchluessel(K, a), jetzt); !reif {", "	if false {"),
    ("Anteile ohne Antwort", "	if a == nil || a.Index != index || a.Antwort == nil {", "	if a == nil || a.Index != index {"),
    ("Anteile fuer andere Anfrage", "	if a == nil || a.Index != index || a.Antwort == nil {", "	if a == nil || a.Antwort == nil {"),
    ("Zufall ohne Reife", "			if reif, _ := v.reifLocked(sk, jetzt); !reif {\n				continue\n			}\n			z, ok", "			z, ok"),
    ("Blaetter nicht gegen Wurzel", "	if quantenWMerkleWurzel(blaetter) != wurzel {", "	if false {"),
    ("Unterschrift des Praegers nicht geprueft", "wallet, jetzt); err != nil {\n		return nil, quantenWValNein(http.StatusForbidden, \"Unterschrift", "wallet, jetzt); err != nil && false {\n		return nil, quantenWValNein(http.StatusForbidden, \"Unterschrift"),
    ("Inhalt nicht in der Unterschrift", "strings.Join(validatoren, \",\"), x, inhalt, zeit)", "strings.Join(validatoren, \",\"), x, \"\", zeit)"),
    ("Rang nicht in der Unterschrift", "strings.Join(validatoren, \",\"), x, inhalt, zeit)", "strings.Join(validatoren, \",\"), 0, inhalt, zeit)"),
    ("zweiter Inhalt ersetzt", "		return nil, quantenWValNein(http.StatusConflict, \"fuer k ist schon etwas anderes hinterlegt\")\n", "		delete(v.tresor, K)\n"),
    ("Mensch nicht geprueft", "	if !mensch || guthaben < quantenMindestDeckungMik {", "	if guthaben < quantenMindestDeckungMik {"),
    ("Mindestdeckung nicht geprueft", "	if !mensch || guthaben < quantenMindestDeckungMik {", "	if !mensch {"),
    ("Zeitfenster nicht geprueft", "	if jetzt-h.Zeit > quantenWVoraus || h.Zeit-jetzt > quantenWVoraus {", "	if false {"),
    ("lebende Note nicht geprueft", "	if tot || lebt {", "	if false {"),
    ("Anteil ausserhalb des Koerpers", "		if !ok || y.Cmp(quantenWP) >= 0 {\n			return nil, quantenWValNein(http.StatusBadRequest, \"anteil %d ausserhalb", "		if !ok {\n			return nil, quantenWValNein(http.StatusBadRequest, \"anteil %d ausserhalb"),
    ("Tresor ohne Obergrenze", "	if len(v.tresor) >= quantenWTresorHoechstens {\n		return nil, quantenWValNein(", "	if false {\n		return nil, quantenWValNein("),
    ("ohne Grenze je Praeger", "	if offen >= quantenWTresorJeWallet {", "	if false {"),
    ("mehrfach in der Liste", "			if x != 0 {\n				return nil,", "			if false {\n				return nil,"),
    ("Sammeln glaubt anderen", "				if quantenWPruefsumme(e.K, s, quantenWZusammensetzen(an)) != e.Blaetter[s] {", "				if false {"),
    ("Sammeln mit zu wenigen", "	if len(von) < e.T {", "	if false {"),
    ("Hash-Kette um eins versetzt", "	return quantenWKetteVor(e.Geheim, quantenWZufallL-1-j), true", "	return quantenWKetteVor(e.Geheim, quantenWZufallL-j), true"),
    ("Siegel nicht an K gebunden", "	klar, err := gcm.Open(nil, siegel[:gcm.NonceSize()], siegel[gcm.NonceSize():], []byte(e.K))", "	klar, err := gcm.Open(nil, siegel[:gcm.NonceSize()], siegel[gcm.NonceSize():], nil)"),
    ("Sichtung ohne die Antwort", "a.Stellen, a.Basen, sb.String(), a.AntwortZeit)", "a.Stellen, a.Basen, \"\", 0)"),
    ("Takt vor dem Stichtag", "	if !quantenWAktiv(jetzt) {\n		return 0\n	}\n", ""),
    ("Staffel entfernt", "			if !reif || jetzt < seit+int64(e.X-1)*quantenWValStaffel {", "			if !reif {"),
    ("Raeumen im Praegefenster", "			} else if jetzt > e.Zeit+quantenWAlt+quantenWVoraus+quantenWBuchSpiel {", "			} else if jetzt > e.Zeit+quantenWAlt {"),
    ("tote Note bleibt im Tresor", "				v.tresorLoeschenLocked(K, \"Note tot\")", "				_ = tot"),
    ("Endpunkt hinterlegen ohne Stichtag", ENDPUNKT_ALT, ENDPUNKT_NEU),
]
rot = 0
for name, a, b in M:
    if orig.count(a) != 1:
        print("?? Stelle nicht eindeutig:", name, orig.count(a), flush=True)
        continue
    open(F, 'w').write(orig.replace(a, b))
    r = subprocess.run(['go', 'test', '.', '-run', 'TestQuantenWValidator', '-count=1'], cwd=R, capture_output=True, text=True)
    out = r.stdout + r.stderr
    if 'build failed' in out or 'vet:' in out:
        print("BAU  ", name, [l for l in out.splitlines() if '.go:' in l][:2], flush=True)
    elif r.returncode != 0:
        rot += 1
        f = [l.split()[2] for l in out.splitlines() if l.startswith('--- FAIL')]
        print("rot  ", name, "->", ",".join(x.replace('TestQuantenWValidator_', '') for x in f), flush=True)
    else:
        print("GRUEN", name, flush=True)
open(F, 'w').write(orig)
print(rot, "von", len(M), "rot", flush=True)
