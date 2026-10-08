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

## In Arbeit (andere Sitzung, offen als PRs)
- **#312 Bündel** aus #307–#311 (ein Deploy statt fünf), CI grün:
  - #307 L1: kein spät eingehängter Block mit Bindung oder Beweis (schlafend)
  - #308 Schritt 3, Teil 2: Validatoren-Belohnung aus der Kette (schlafend)
  - #309 GHOSTDAG: K nicht mehr aus einem lokalen Komitee (wirkt sofort,
    K bleibt 18)
  - #310 Coordinatoren nach dem Register zulassen (Baustein, schlafend;
    H1 offen: eine Bindung kostet nichts)
  - #311 Coordinator-Nachrichten mit Chain-ID (v2)
  - Offen vor dem Merge: getrennte Sicherheitsprüfung der Korrekturen im
    Bündel (läuft), dann Merge.
- Mein Zweig `claude/weiter-gehts-r7rb9w-register3b` (Teil 2) ist durch #308
  **überholt** und wird nicht weiterverfolgt.

## Code – danach
1. **Komitee** (`getEpochCommittee`) aus derselben Menge wie die Belohnung
   statt aus lokal bekannten Adressen.
2. **#310 H1:** Zulassung von Coordinatoren braucht einen Preis
   (eine Bindung kostet nichts) – vor dem Staffel-Stichtag.
3. **Vor dem strengen Modus: Anwesenheit aus dem Vergangenheitskegel**
   eines Ankerblocks zählen statt aller Blöcke im Zeitfenster (#308 zählt
   `chain_blocks` im Fenster: ein Knoten mit einem Block mehr oder weniger –
   etwa an der Finalitätswand – rechnet anders und wiese im strengen Modus
   die Runde ab). Mit dem Kegel zählen nur Vorfahren des Ankers; fehlt einem
   Knoten einer, rechnet er nicht nach statt falsch. Bausteine liegen in
   `claude/weiter-gehts-r7rb9w-register3b` (`bloeckeImKegel`,
   `validatorAnkerWaehlen`, Tests).
4. **Strenger Modus** spätestens mit der Staffel; Stichtag
   `nachrechnenStrengAbUnix`, sobald `/api/wirtschaft/regeln` über mehrere
   Runden 0 Abweichungen zeigt.
5. Offener Betrieb (ohne `AUTHORIZED_VALIDATORS`) mit über 1.000
   Validatoren: fortgeschriebener statt neu gelesener Stand (Erzeugerprüfung
   und Anwesenheit).
6. **Pausierter Leiter übergibt nicht** (Prüfung von #322, INFO-1; bestand
   schon vorher): Ein Leiter, der nach einem Neustart wieder übernimmt,
   pausiert bis zu seinem ersten eigenen Block; so lange lehnen die
   Leiter-Wege netzweit wiederholbar ab, und bei zwei Validatoren ohne
   `AEQUITAS_LEITUNG_ZWEI_WECHSELN` beendet kein Wechsel das. Fix: eine
   Pause länger als `admissionStallLimit()` im Takt wie einen verlorenen
   Leistungsnachweis behandeln, also übergeben. (Beobachter sind seit #322
   nicht leiterfähig.)
7. **Einträge in `pending_txs` ohne Annahme-Pause** (Prüfung von #322,
   INFO-2; bestand schon vorher): `DoppelsignaturErkannt` (slashing.go,
   ausgelöst von Peer-Blöcken) legt den Strafbeweis auch auf einem Beobachter
   oder pausierten Knoten in den Ausgang; läuft `strafBeweisFrisch` ab, bevor
   der Knoten erzeugt, helfen nur Resync oder Verwerfen.
   `RetryRegistrationRecoveries` (alle 5 Minuten) prüft die Pause nicht. Fix:
   auf einem Beobachter den Beweis nur vermerken; die Wiederholung an
   `annahmePauseGrund()` koppeln.

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
