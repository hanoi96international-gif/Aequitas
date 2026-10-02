# Erinnerung: was noch offen ist

Stand 02.10.2026. Eine Liste zum Abhaken. Begründungen und Quellen stehen in
den verlinkten Dokumenten.

## ⏰ Frist: 15.10.2026, Stufe 2 der Wirtschaftsregeln

Ab **15.10.2026, 00:00 UTC** gelten die neuen Regeln A, B und C
(`wirtschaft2.go`). Bis dahin:

- [ ] Branch `claude/beta-launch-business-integration-vr7u4c` mergen und
      deployen. **Oder** das Aktivierungsdatum verschieben.
- [ ] Vor dem Deploy prüfen, ob diese Abfrage **0** ergibt:
      `SELECT count(*) FROM wirtschaft_unternehmen WHERE verantwortliche LIKE '%,%'`
      (Erläuterung in `SICHERHEITSPRUEFUNG_BRANCH_2026-10-02.md`)
- [ ] Eigene Sicherheitsprüfung des Diffs als PR-Kommentar (Regel aus
      `AGENTS.md`)
- [ ] Freigabe für die Produktionsumgebung erteilen

## Betrieb, vor dem Beta-Start

- [ ] Auf dem Server sind `PROOF_SERVER_URLS` und `CHAIN_SERVICE_TOKEN`
      gesetzt; `/api/health/combined` → `proof_server_sync` zeigt keine
      übersprungenen Meldungen
- [ ] Die Zahl der Einträge in der Galerie der Vergleichsdienste stimmt mit
      den registrierten Menschen überein
- [ ] Einen zweiten, unabhängigen Betreiber bzw. Validator finden
- [ ] Impressum und Datenschutzerklärung veröffentlichen
      (Vorlagen: `aequitas-biometric-beta/docs/dsgvo/07_*`, `08_*`)

## Entscheidungen, die nur ihr treffen könnt

- [ ] **Mindestalter** für die Gesichtsprüfung festlegen (Vorschlag:
      Selbsterklärung „mindestens 18“ in Einwilligung und App).
      Hintergrund: Spanien und Portugal haben Worldcoin wegen Minderjähriger
      gestoppt; Aequitas fragt heute kein Alter ab.
- [ ] **Verantwortlichen** für den Datenschutz benennen (Name, Anschrift)
- [ ] Die sechs Grundsatzentscheidungen in `GRUNDPFEILER.md`, Abschnitt 13

## Vor dem Öffnen des Biometrie-Riegels (`ALLOW_REAL_BIOMETRIC_DATA`)

- [ ] Mindestalter umsetzen (siehe oben)
- [ ] Festplattenverschlüsselung auf den Servern, die die Sketches halten
- [ ] DSFA-Abwägung „ein Merkmal bleibt nach Löschung“ gegen den
      BayLDA-Bescheid zu Worldcoin prüfen lassen
- [ ] DSFA-Abschnitt R4 (Kette) mit den EDPB-Leitlinien 02/2025 (Fassung 2.0)
      abgleichen
- [ ] Klären, ob die Aufsicht vorher konsultiert werden muss (Art. 36 DSGVO)

## Vor echtem Wert (Stablecoin, Euro, Pilotladen mit echter Ware)

- [ ] **Kanzlei** beauftragen: Fragen 1–36 in `RECHTSFRAGEN_UNTERNEHMEN.md`
      (u. a. MiCA-Whitepaper und Anbieter, Tausch-Pool als
      Krypto-Dienstleistung, E-Geld-Token am Pool)
- [ ] Keinen eigenen Euro-Ein- und -Ausgang betreiben, sondern einen
      lizenzierten Dienstleister nutzen
- [ ] Vereinbarung über gemeinsame Verantwortung (Art. 26 DSGVO), bevor der
      zweite Validator dazukommt
- [ ] **Ökonomin oder Ökonom** liest `WIRTSCHAFT_ZAHLENPRUEFUNG.md` und
      `GRUNDPFEILER.md` gegen
- [ ] Pilot an **einem** Ort mit mehreren Läden, die einander beliefern
      (`PILOTLADEN.md`)

## Später

- [ ] Kein Kreditprotokoll vor dem Ergebnis der MiCA-Überprüfung 2026 und
      einer Kanzlei-Einschätzung
- [ ] Mitbestimmung (Grundpfeiler 8) planen; die Forschung nennt sie eine
      Überlebensbedingung

## Angebote von Claude, die auf eure Antwort warten

- [ ] Pull Request für den Branch erstellen
- [ ] Regel B von Stufe 2 trennen (eigenes Aktivierungsdatum)
- [ ] Ein Wirtschaftsmodell (Simulation) der Regeln bauen
- [ ] `GRUNDPFEILER.md` als Seite veröffentlichen
- [ ] Altersabfrage in App und Einwilligung einbauen

## Wo was steht

| Thema | Dokument |
|---|---|
| Recht und Forschung, mit Quellen | `RECHT_UND_LITERATUR.md` |
| Fragen an die Kanzlei | `RECHTSFRAGEN_UNTERNEHMEN.md` |
| Die Grundpfeiler | `GRUNDPFEILER.md` |
| Unternehmen | `UNTERNEHMEN_KONZEPT.md` |
| Zahlen und Fairness | `WIRTSCHAFT_ZAHLENPRUEFUNG.md`, `WIRTSCHAFT_REIFEPRUEFUNG.md` |
| Sicherheitsprüfung des Branches | `SICHERHEITSPRUEFUNG_BRANCH_2026-10-02.md` |
| Datenschutz-Unterlagen | `aequitas-biometric-beta/docs/dsgvo/` |
