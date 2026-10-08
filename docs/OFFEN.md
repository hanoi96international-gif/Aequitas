# Offen – Übergabe (Stand 08.10.2026)

Kurzliste für die nächste Sitzung. Die vollständige Liste steht in
`docs/ERINNERUNG.md`; diese Datei sagt, **wo die Arbeit gerade steht**.

## Erledigt seit dem 07.10.
- #303 Validator-Register Schritt 3, Teil 1 (Verlauf, Erzeuger mit Frist,
  Strafkonto, Leitung) – gemergt.
- #305 WAL-Testaufbau (Datenrennen beim Stopp) – gemergt.
- #306 Geldstrafe später und für alle gleich abrechnen (M1/M2/L3) – gemergt.
- #304 `VALIDATOR_BETREIBER_WALLET` im Verifier-Skript, Fehlermeldungen – gemergt.
- proof-server #11 und biometric #38 (Schlüssel kleiner Ordnung,
  Besitznachweis nur für den Betreiber) – gemergt.

## Code – als Nächstes
1. **Schritt 3, Teil 2: Validatoren-Belohnung aus der Kette.** Gebaut und
   getestet im Zweig `claude/weiter-gehts-r7rb9w-register3b` (Commit
   f0e2632, gepusht, noch **kein PR**). Dateien:
   `validator_belohnung_kette.go`, `nachrechnen_validator.go`, Tests
   `validator_belohnung_kette_realdb_test.go`. Zu tun: auf das heutige
   `main` bringen (#305/#306 sind neu; `erzeugerAusVerlauf` liefert seit
   #303 auch `halter` – `fensterAusVerlauf` anpassen), Mutationen, `-race`,
   getrennte Sicherheitsprüfung, PR, Merge.
2. **L1** vor den Stichtagen: ein zurückgehaltener, spät eingehängter Block
   mit `validator_bindung` wirkt rückwirkend (die Frist setzt pünktliches
   Nachspielen voraus).
3. **Komitee** aus derselben Menge wie die Belohnung (Teil 2b).
4. **Vor dem Staffel-Stichtag:** Coordinatoren im Konsens zulassen und
   entziehen; strenger Modus spätestens mit der Staffel; Chain-ID in die
   Nachrichten der Bindung und der Bescheinigung (v2).
5. Stichtag `nachrechnenStrengAbUnix` setzen, sobald
   `/api/wirtschaft/regeln` über mehrere Runden 0 Abweichungen zeigt.

## Betrieb – bei dir
- `COORDINATOR_BETREIBER_WALLET` und `VALIDATOR_BETREIBER_WALLET` auf den
  Boxen setzen (die Deploy-Workflows übernehmen die alte Umgebung, die neuen
  Variablen kommen nicht von selbst). Ohne sie stellt der Dienst keinen
  Besitz-/Bezeugungsnachweis aus (gewollt, fail-closed).
- Nach dem Deploy von biometric #38: die Coordinator- und
  Personhood-Schlüssel von proof1 **sofort** auf der Kette unter deiner
  Betreiber-Wallet eintragen – oder `COORDINATOR_SIGNING_KEY` /
  `VALIDATOR_SIGNING_KEY` rotieren. Vorher ausgestellte Nachweise gelten sonst
  weiter (der Satz hat kein Datum).
- proof-server #11 vor oder zusammen mit #38 ausrollen (die Workflows lesen
  die Schlüssel jetzt aus `/coordinator/inventory` und `/matching/health`).
- `coordinator_keys` / `validator_keys` auf C1 auf Schlüssel kleiner Ordnung
  prüfen; vor einem Image-Update der Verifier deren `.env` prüfen.
- Stichtage (eure Entscheidung): `validatorRegisterAb`, `registerLeserAb`,
  `erzeugerSchnittAb` (erst wenn `/api/status` → `erzeuger_ohne_bindung`
  leer ist), Staffel, strenges Nachrechnen. Nur geschlossener Betrieb
  (`AUTHORIZED_VALIDATORS`) ist für die Register-Stichtage freigegeben.
- Außerdem aus `ERINNERUNG.md`: C2 / zweiter unabhängiger Betreiber,
  App 1.10.0 als Release, Altersmodell, `PROOF_SERVER_URLS` und
  `CHAIN_SERVICE_TOKEN` auf dem Server, Impressum und Datenschutz,
  Verantwortlicher, DSFA-Punkte, Kanzlei, Ökonom, Pilot.
