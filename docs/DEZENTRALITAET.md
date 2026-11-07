# Dezentralität — Stand je Schicht und Weg dorthin

Stand 07.11.2026. Ziel des Betreibers: **alles auf volle Dezentralität
ausgelegt.** Kein Mensch, keine Firma und kein Server soll entscheiden können,
wer Mensch ist, wer Geld bekommt oder welche Regeln gelten. Dieses Dokument
sagt ehrlich, wo das heute schon stimmt, wo der Code es kann, aber noch ein
Mensch fehlt, und wo noch gebaut werden muss.

Grundsätze und Entscheidungen: `VERIFIER_PRINZIPIEN.md`. Skalierung ohne
zentrale Instanz: `SKALIERUNG_DEZENTRAL.md`.

## Was „dezentral“ hier heißt

1. **Jeder Knoten prüft alles selbst.** Signaturen, Nonces, Deckung,
   StateRoot, Bescheinigungen. Kein Knoten glaubt einem anderen.
2. **Jede Rolle kann jeder Mensch übernehmen.** Zugang über die
   Registrierung (ein Mensch, eine Stimme), nie über Geld oder Rechenleistung.
3. **Keine Rolle liegt bei nur einem.** Für jede Aufgabe gibt es mehrere,
   voneinander unabhängige Menschen; der Ausfall eines Einzelnen hält nichts
   an.
4. **Regeln ändern sich nur gemeinsam.** Neue Prüfer, ein neuer Satz, ein
   Entzug: nur mit Unterschriften mehrerer Menschen, im Konsens geprüft.

## Stand je Schicht

| Schicht | Prüft jeder Knoten selbst? | Wer darf mitmachen? | Heute wie viele? | Was fehlt |
|---|---|---|---|---|
| **Blöcke erzeugen** | ja, mit Erzeugerprüfung aus dem Kettenregister (seit 28.10.) | jeder registrierte Mensch mit Validator-Bindung | **1** (C1) | weitere Menschen mit eigenem Server |
| **Transaktionen annehmen** | ja, jede Überweisung trägt die Rohsignatur (Stufe 1.0) | — | **1** annehmender Knoten | Stufe 2 (`verteilteAnnahmeAbUnix`) ist gebaut; scharf frühestens ab drei unabhängigen Validatoren |
| **Wirtschaftsregeln** | ja, streng seit 01.11. (`nachrechnenStrengAbUnix`) | — | — | — |
| **Mensch bestätigen (K1)** | ja, ab 07.11. 17:00 UTC: jeder Knoten prüft die Unterschriften von Coordinator und Vergleichsdienst | Schlüssel stehen in der Genesis | **1** Betreiber | Schlüssel aus dem Prüfer-Register statt der Genesis (Schritt B1b unten) |
| **Prüfer (Coordinator) zulassen** | ja | **neu:** Aufnahme durch zwei Drittel der Prüfer (`verifier_register.go`), ab 07.11. 17:00 UTC | **1** (Gründer) | zweiter und dritter Prüfer |
| **Vergleichsdienst (Gesichtsabgleich)** | Ergebnis ja (K1), der Abgleich selbst nein | wer den Dienst betreibt | **1**, mit Tagesgrenze 25 | zweiter Dienst bei einem anderen Menschen; später MPC (Phase C) |
| **Proof-Server (ZK-Beweis)** | ja, der Beweis wird überall geprüft | jeder | **1** | B4: ohne Vertrauensrolle, jeder kann rechnen |
| **Grundeinkommen, Staffel, Lebensnachweis** | ja | Einspruch nur mit zwei verschiedenen Prüfern | — | Staffel schläft, bis die Zulassung knapp ist (jetzt gebaut) |
| **Satz des Gleichgewichts ändern** | ja | zwei verschiedene zugelassene Betreiber-Menschen | 1 → **unmöglich allein** | gut so: allein kann niemand den Satz ändern |
| **Snapshots für neue Knoten** | Import prüft Unterschrift und StateRoot | — | von C1 | Checkpoints mit mehreren Unterschriften (`CHECKPOINT_FAHRPLAN.md`) |
| **Website, App-Download** | — | — | aequitas.digital, ein GitHub-Konto | Spiegel bei anderen Menschen; App-Signatur nicht nur bei einer Person; GitHub-Organisation mit zwei Inhabern |
| **Code ändern und ausrollen** | jeder Knoten wählt selbst, was er laufen lässt | — | ein Konto | Organisation, Zwei-Personen-Freigabe für Deploys |

**Ehrlich zusammengefasst:** Der Code ist in den meisten Schichten auf viele
Beteiligte gebaut, und jede Prüfung läuft bei jedem Knoten. Was heute fehlt,
sind vor allem **weitere unabhängige Menschen**, die einen Knoten, einen
Vergleichsdienst oder einen Coordinator betreiben. Solange es nur einen
Betreiber gibt, kann dieser Betreiber vieles allein. Das lässt sich nicht mit
Code beheben, nur mit Menschen. Der Code muss aber dafür sorgen, dass jeder
Weitere ohne Erlaubnis des Betreibers dazukommen kann und danach nicht mehr
übergangen werden kann.

## Neu: das Prüfer-Register mit Abstimmung (B1)

`x/humanity/keeper/verifier_register.go`, Transaktion `verifier_zulassung`,
Endpunkt `/api/verifier`.

**Das Problem (Sicherheitsdurchgang #310, H1):** Wer einen Validator-Schlüssel
band, war sofort als Coordinator zugelassen und konnte Erneuerungen,
Lebensnachweise und Einsprüche bescheinigen. Eine Bindung kostet nichts.
Deshalb musste die Staffel schlafen.

**Jetzt:** Coordinator ist nur, wer **einen Validator-Schlüssel hält und im
Prüfer-Register steht**. Ins Register kommt man:

| Art | Wer unterschreibt | Bedingung |
|---|---|---|
| Gründer | niemand | steht im Code (`verifierGruender`), heute der eine Betreiber |
| Aufnahme | Prüfer | mindestens zwei Drittel aller Prüfer zur Zeit der Abstimmung, aufgerundet; Kandidat ist registrierter Mensch |
| Austritt | der Prüfer selbst | jederzeit, ohne Bedingung |
| Entzug | die übrigen Prüfer | mindestens zwei Drittel der übrigen; wer allein ist, kann nur austreten |

- **Wirkt nie rückwirkend:** Jede Änderung gilt ab `issued_at` + Frist.
  Bescheinigungen von vorher bleiben gültig.
- **Menschen, nicht Schlüssel:** Jede Stimme ist ein Mensch. Zwei Schlüssel
  desselben Menschen zählen einmal.
- **Keine alten Stimmen:** Eine Stimme verfällt nach 14 Tagen und gilt nie
  vor der letzten Änderung beim Betroffenen.
- **Jeder Knoten prüft alles selbst**, beim Annehmen wie beim Nachspielen.
  Das Register steht in der StateRoot, im Snapshot und im Rückroll-Stand.

**Wachstum:** Mit einem Prüfer reicht dessen Stimme für den zweiten. Ab dann
braucht jede Aufnahme beide, ab drei Prüfern zwei von drei, und so weiter. Der
Gründer kann also allein nur den Ersten neben sich holen, danach nicht mehr.

**Stichtag 07.11.2026 17:00 UTC**, zugleich mit K1 (Anweisung des
Betreibers). Der Deploy hält an, wenn er nicht mindestens 30 Minuten vorher
läuft (`.github/neustart/verifier-stichtag-nicht-verpasst.sh`).

Damit der Gründer bescheinigen und abstimmen kann, braucht sein Coordinator
die Betreiber-Wallet (`bio-deploy-coordinator-to-box.yml`, Eingabe
`betreiber_wallet`) und einmal die Freigabe des Schlüssels durch die Wallet
selbst (Unterschrift in der App). Für K1 ist das nicht nötig: K1 prüft die
Schlüssel aus der Genesis.

## Reihenfolge der nächsten Schritte

| # | Schritt | Braucht | Stand |
|---|---|---|---|
| 1 | Prüfer-Register mit Abstimmung (B1) | Code | **gebaut, scharf ab 07.11. 17:00 UTC** |
| 2 | B1b: K1 liest Prüfer-Schlüssel aus dem Register statt aus der Genesis | Code (Kette, Coordinator, Proof-Server) | offen |
| 3 | Zweiter Prüfer mit eigenem Coordinator und Vergleichsdienst | **einen Menschen** | offen |
| 4 | K1-Quorum auf 2 verschiedene Menschen | Schritt 2 und 3 | offen |
| 5 | Staffel scharf | Schritt 1 aktiv, dann Test `TestStaffel_SchlaeftBisZulassungUndStreng` bewusst anpassen | offen |
| 6 | Zweiter und dritter Validator | **zwei Menschen mit Server** | offen |
| 7 | Stufe 2: alle nehmen an | Schritt 6 | gebaut, schlafend |
| 8 | Checkpoints mit mehreren Unterschriften | Schritt 6 | geplant |
| 9 | Proof-Server ohne Vertrauensrolle (B4) | Code | offen |
| 10 | GitHub-Organisation, zwei Inhaber, Zwei-Personen-Freigabe | **zweiten Menschen** | offen (`GITHUB_ORGANISATION.md`) |
| 11 | Website- und App-Spiegel bei anderen Betreibern | Menschen | offen |
| 12 | Abgleich per MPC, Biometrie aufgeteilt (Phase C) | Forschung | offen |
