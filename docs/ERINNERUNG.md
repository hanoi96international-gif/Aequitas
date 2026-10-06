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

- [ ] **C2 ist abgeschaltet (03.10.)** -- Einzelbetrieb nach Entscheidung des
      Betreibers vom 04.10.: Quorum 1 MIT Tagesgrenze (Standard 25 neue
      Menschen je UTC-Tag), sichtbar in `/coordinator/health` und
      `/health` des Proof-Servers (`einzelbetrieb`).
      - [x] Code: Coordinator (aequitas-biometric-beta#36), Proof-Server
            (aequitas-proof-server#10), Wache akzeptiert Quorum 1 nur mit
            aktiver Tagesgrenze
      - [ ] Umstellen: biometric „Coordinator auf eine Contabo-Box
            ausrollen“ mit `einzelbetrieb` (proof1 allein, proof2 abgemeldet);
            proof-server „Einzelbetrieb auf C1“ (nur proof1-Schlüssel)
      - [ ] Zurück auf 2: sobald ein zweiter, unabhängiger Vergleichsdienst
            läuft -- Coordinator automatisch (Mehrheit der bekannten Dienste),
            Proof-Server `BIO_ATTESTATION_QUORUM=2` per `schluessel-festlegen.yml`
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
        - [x] Schritt 1, schlafend (05.10.): Kettentransaktion
              `validator_bindung` mit Unterschrift von Betreiber und
              Signierschlüssel, Register in `validator_register`, Summe in
              der StateRoot, Snapshot (`validator_register.go`). Vor dem
              Stichtag `validatorRegisterAbUnix` (Platzhalter) ungültig
        - [ ] Schritt 2: `/api/peers/register` legt die Bindung in den
              Ausgang, bestehende Betreiber binden neu
        - [ ] Schritt 3: Strafkonto, Belohnung, Erzeugerliste und Komitee
              lesen aus dem Register; Stichtag setzen (eure Entscheidung,
              `docs/VALIDATOR_REGISTER_KONSENS.md`)
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
      - [x] Treuhand in der StateRoot (05.10.): `escrowSetXOR` summiert die
            Zeilen (Wallet und Betrag; die Frist nicht, die setzen Erzeuger
            und Nachspielende verschieden), geht mit dem Block zurück, wird
            beim Start neu aufgebaut, und der Snapshot trägt die Zeilen mit.
            Ohne Treuhand bleibt die Wurzel byte-gleich
            (`treuhand_stateroot.go`)
      - [x] Vollständige Durchsicht aller 31 Transaktionsarten im
            Nachspielen (03.10.): drei Wege nahmen Produzentenwerte noch
            ungeprüft, jetzt nachgerechnet --
            `vorbehalt_ausfuehrung` (Tauschergebnis, LP-Anteile, Betrag gegen
            den unterschriebenen Auftrag, Mindestbetrag;
            `nachrechnen_vorbehalt.go`), `kappung` (nur über der
            Vermögensgrenze) und `slash_equivocation` (zwei erfundene Hashes
            reichten, um jeden Validator zu sperren und ab dem zweiten Mal 50
            AEQ zu nehmen; jetzt trägt die Strafe beide unterschriebenen
            Blockköpfe als Beweis, `slash_beweis.go`). `pool_correction` gilt
            nur auf der alten Kette und wird auf V8 abgelehnt
      - [x] Zurückgewiesener Block hinterlässt keinen Vorbehalt (05.10.):
            die Vorbehaltskonten stehen jetzt in der Rückroll-Liste des
            Blocks, und `cs.vorbehalte` geht mit (`vorbehaltSicherung`).
            Vorher blieben Einsatz und offener Vorbehalt im Speicher stehen;
            der ehrliche Block mit demselben Vorbehalt scheiterte danach
            („existiert schon“) bzw. seine Ausführung wurde übersprungen
      - [x] `RecordEquivocationAndSuspend` öffnete eine eigene
            Datenbank-Transaktion außerhalb der des Blocks (05.10., #286):
            Beweis, Sperre und Strafabzug laufen beim Nachspielen jetzt in
            der Transaktion des Blocks und gehen mit ihm zurück; erkannt wird
            über `DoppelsignaturErkannt` in einer Transaktion samt Ausgang
            (`slashing.go`)
      - K-4 aus dem Audit vom 29.09.: der Wortlaut ist in keinem Commit,
        Kommentar oder Dokument erhalten; bekannt ist nur die Einordnung
        „bis das Nachspielen jeden Wert selbst prüft (K-2, K-3, K-4)“
        (Launch-Checkliste 18). Die Durchsicht oben deckt dieses Thema
        inhaltlich ab; offen davon bleiben die Validatoren-Gewichte und der
        strenge Stichtag. Das K4 vom 18.08.
        (Nullifier an den Beweis gebunden) ist erledigt und getestet
      - [x] `liveness_renewal` trägt die Bescheinigung des Coordinators
            (05.10.): Ed25519 über Wallet und Zeitpunkt steht jetzt im Block,
            jeder Knoten prüft sie selbst, dazu Alter (höchstens 7 Tage,
            höchstens 5 min voraus) und
            Tag 7 (`nachrechnen_erneuerung.go`). Vor der Aktivierung (2100)
            ungeprüft wie bisher. Die Bindung an dieselbe Person leistet der
            Coordinator (WP 3, Wallet-Signatur und Gesichtsabgleich)
      - [x] Staffel-Zeitpunkte (05.10.): `GrantStagedUntil` und
            `LivenessRenewedAt` setzen Erzeuger und Nachspielende jetzt aus
            der Transaktion (`RegAt`, höchstens einen Tag vor dem Block;
            `issued_at` der Bescheinigung) statt jeder nach seiner Uhr bzw.
            der Blockzeit -- sonst wiche ab der Aktivierung jedes gestaffelte
            Konto in der StateRoot ab (`grant_staffel.go`, „EIN ZEITPUNKT“)
      - [x] Divergenz-Wächter vergleicht in der Ruhe neben den Konten auch
            Treuhand- und Register-Summe (05.10., `divergenzAbweichung`) --
            vorher sahen zwei Knoten mit verschiedener Treuhand gleich aus
      - [x] Coordinator-Register für den Konsens (06.10.): die
            Erneuerungs-Bescheinigung trägt jetzt ihre Bindung (Mensch,
            dessen Freigabe, Besitznachweis des Schlüssels), und jeder Knoten
            prüft sie gegen den Kettenzustand -- das knotenlokale
            `coordinator_keys` entscheidet nichts mehr. Neu: kein
            Coordinator bescheinigt sich selbst. Die Erneuerung geht durchs
            Annahme-Tor (zum Leiter). Wirkt mit der Staffel (2100,
            Platzhalter); bestehende Coordinatoren tragen sich vorher einmal
            neu ein (die Unterschriften werden erst seit heute gespeichert)
      - [ ] Stichtag `nachrechnenStrengAbUnix` setzen, sobald
            `/api/wirtschaft/regeln` über mehrere Runden 0 Abweichungen zeigt.
            C1 spielt seine eigenen Blöcke nicht nach, und **C2 gibt es seit
            03.10. nicht mehr**. Die Zahlen liefert jetzt der Workflow
            `nachrechnen-beobachter.yml`: täglich 17:20 UTC ein Knoten auf einem
            GitHub-Runner, Snapshot von C1, danach jeder Block nachgespielt,
            erzeugt selbst nie einen (`AEQUITAS_BEOBACHTER=1`). Ergebnis je
            Regel in der Zusammenfassung des Laufs
        - Auswertung 05.10.: Lauf #4 (04.10.) begann um 20:13 statt 17:20
          und verpasste die Runde um 18:00 -- „0 Abweichungen“ bei 0
          geprüften Transaktionen, **ohne Aussage**. Seit #289 startet der
          Workflow um 15:40 und 16:40 UTC, prüft vorab, ob die nächste Runde
          (`last_ubi_at` von C1 + 24 h) im Lauf liegt, läuft bis 5 min nach
          dieser Runde und kennzeichnet Läufe ohne nachgespielte Runde als
          „Ohne Aussage“. Gezählt werden nur Läufe mit Runde; der Stichtag
          bleibt offen, bis mehrere solche Läufe 0 Abweichungen zeigen
      - [ ] Zweiter Vergleichsdienst für die Registrierung: bis dahin
            Einzelbetrieb mit Tagesgrenze (siehe „Betrieb, jetzt“); nötig ist
            ein zweiter, unabhängiger Betreiber mit Vergleichsdienst
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
