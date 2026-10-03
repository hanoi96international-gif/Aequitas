# Erinnerung: was noch offen ist

Stand 03.10.2026. Eine Liste zum Abhaken. Begründungen und Quellen stehen in
den verlinkten Dokumenten.

## Erledigt am 03.10.2026

- [x] Wirtschaft Stufe 2 und Unternehmensverzeichnis gemergt (#267) und auf
      C1 ausgerollt (07:55 UTC, Stand `afc7178`); aktiv ab **03.10.2026,
      12:00 UTC (14:00 MESZ)**
- [x] Vorbedingung auf C1 geprüft: 0 Unternehmen mit mehreren
      Verantwortlichen (Workflow „Wirtschaft Stufe 2 – Vorbedingung“, #268)
- [x] Sicherheitsprüfung der Diffs als PR-Kommentare
- [x] **Mindestalter umgesetzt**, ohne Ausweispflicht: Selbstangabe mit
      Länderregel, Altersschätzung vor dem Vergleich, Altersbürgschaft
      (aequitas-biometric-beta#30, Aequitas-App#26); Einwilligung v3
- [x] Coordinator und Vergleichsdienst auf C1 ausgerollt, Altersprüfung
      `beobachten`
- [x] App 1.10.0 gemergt (Altersangabe, Altersbürgschaft, Unternehmen)

## Betrieb, jetzt

- [ ] **C2 ist nicht erreichbar** (SSH-Zeitüberschreitung am 03.10.). Der
      Coordinator verlangt ein Quorum von 2 aus C1 und C2 -- solange C2 fehlt,
      scheitert jede Registrierung am Quorum. C2 wieder einrichten
      (`VALIDATOR_EINRICHTEN.md`) oder das Quorum bewusst anders festlegen.
- [ ] Altersmodell messen und freigeben (`aequitas-biometric-beta/docs/ALTERSMODELL.md`,
      Workflow „Altersmodell messen“); erst danach `ALTERSPRUEFUNG=erzwingen`
- [ ] App 1.10.0 als Release veröffentlichen (Workflow „APK als Release
      veröffentlichen“)
- [ ] Auf dem Server sind `PROOF_SERVER_URLS` und `CHAIN_SERVICE_TOKEN`
      gesetzt; `/api/health/combined` → `proof_server_sync` zeigt keine
      übersprungenen Meldungen
- [ ] Die Zahl der Einträge in der Galerie der Vergleichsdienste stimmt mit
      den registrierten Menschen überein
- [ ] Einen zweiten, unabhängigen Betreiber bzw. Validator finden
- [ ] Impressum und Datenschutzerklärung veröffentlichen
      (Vorlagen: `aequitas-biometric-beta/docs/dsgvo/07_*`, `08_*`)

## Entscheidungen, die nur ihr treffen könnt

- [ ] **Verantwortlichen** für den Datenschutz benennen (Name, Anschrift) --
      fehlt in Einwilligung, Datenschutzerklärung und Impressum
- [ ] Die sechs Grundsatzentscheidungen in `GRUNDPFEILER.md`, Abschnitt 13

## Vor dem Öffnen des Biometrie-Riegels (`ALLOW_REAL_BIOMETRIC_DATA`)

- [x] Mindestalter umsetzen (03.10.2026, siehe oben)
- [ ] Festplattenverschlüsselung auf den Servern, die die Sketches halten
- [ ] DSFA-Abwägung „ein Merkmal bleibt nach Löschung“ gegen den
      BayLDA-Bescheid zu Worldcoin prüfen lassen
- [ ] DSFA-Abschnitt R4 (Kette) mit den EDPB-Leitlinien 02/2025 (Fassung 2.0)
      abgleichen
- [ ] Klären, ob die Aufsicht vorher konsultiert werden muss (Art. 36 DSGVO)

## Vor echtem Wert (Stablecoin, Euro, Pilotladen mit echter Ware)

- [ ] **Kanzlei** beauftragen (vorerst bewusst ohne, Entscheidung 02.10.): Fragen 1–36 in `RECHTSFRAGEN_UNTERNEHMEN.md`
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

## Technik, in Arbeit

- [ ] Strenges Nachrechnen aller Systembuchungen auf jedem Knoten („K-2
      strict“) -- Voraussetzung dafür, neue Validatoren automatisch und ohne
      Liste aufzunehmen
      - [x] Grundeinkommens-Runde: wer, wie oft, wieviel; Demurrage seit
            der Umlaufsicherung (`nachrechnen_ubi.go`, beobachtend)
      - [x] LP-Runde: jeder Halter genau einmal, Anteil nachgerechnet
            (`nachrechnen_lp.go`, seit der Umlaufsicherung)
      - [x] Validatoren-Runde: nur an Menschen
      - [ ] Validatoren-Gewichte: Sie kommen aus `registered_nodes` und den
            Blöcken der letzten 24 h -- beides ist je Knoten verschieden.
            Nachrechnen geht erst, wenn das Verzeichnis Konsenszustand ist
            (Voraussetzung für offene Zulassung)
      - [x] Staffel-Freigaben: höchstens die Tagesrate, nur mit
            Lebenszeichen, einmal je Runde (`nachrechnen_freigabe.go`)
      - [x] `grant_release` wird in einer doppelten Runde mit übersprungen
            (ohne Wirkung auf Altblöcke: Staffel erst ab 2100 aktiv)
      - [x] `umlauf` (Liegegeld) wird in einer doppelten Runde mit
            übersprungen. Geprüft vorher auf C1 (03.10.): seit 26.09. eine
            einzige Runde, keine einzige umlauf-Buchung -- Altblöcke
            unverändert
      - [x] Liegegeld: streng mit dem gemeinsamen Stichtag statt nur je
            Knoten per Umgebungsvariable; Abweichungen zählen im Nachrechnen
      - [x] Topf nach der Grundeinkommens-Runde: Endstand nicht unter dem
            Zufluss der Runde (kein Geld vernichten); Obergrenze schon in
            erhaltung.go
      - [x] Treuhand: escrow_move nur für 2,5 Jahre inaktive Menschen;
            Freigabe/Rückholung vor der ersten möglichen Treuhand (ab
            Dezember 2028 bzw. Juni 2030) gemeldet
      - [x] Treuhand als Bestand bei jedem Knoten: Nachspielende legen die
            Treuhand-Zeile selbst an (Betrag aus dem eigenen Zustand,
            Blockzeit als Frist) und entfernen sie bei Freigabe/Rückholung;
            gemeldet werden Freigabe/Rückholung ohne oder über Bestand, vor
            der Frist und eine zweite Verschiebung derselben Wallet. Altbestand
            gibt es nicht (erste Verschiebung frühestens 09.12.2028)
      - [ ] Treuhand in die StateRoot aufnehmen -- heute zählt nur das
            genullte Guthaben, nicht die Zeile; bis dahin sichern die
            Nachrechen-Regeln den Bestand
      - [ ] Stichtag `nachrechnenStrengAbUnix` setzen, sobald
            `/api/wirtschaft/regeln` über mehrere Runden 0 Abweichungen zeigt
- [ ] Regel B von Stufe 2 trennen (eigenes Aktivierungsdatum) -- nur falls
      gewünscht
- [ ] Ein Wirtschaftsmodell (Simulation) der Regeln bauen
- [ ] `GRUNDPFEILER.md` als Seite veröffentlichen

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
| Altersmodell, Freigabe | `aequitas-biometric-beta/docs/ALTERSMODELL.md` |
