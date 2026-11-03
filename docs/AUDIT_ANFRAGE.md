# Anfrage für ein externes Sicherheits-Audit

Stand: 03.11.2026. Dieses Dokument geht so, wie es ist, an Prüfstellen, die
ein Angebot abgeben sollen. Alles darin ist öffentlich, auch die Repos.

## Worum es geht

Aequitas ist eine eigene Blockchain. Jeder Mensch kann sich einmal mit seinem
Gesicht registrieren und erhält dann einen Startbetrag und später ein
Grundeinkommen in AEQ. Die Registrierung prüft ein Biometrie-Dienst. Ein
Zero-Knowledge-Beweis bindet die Registrierung an die Wallet, ohne das
Gesicht offenzulegen.

Die Beta läuft unter Launch-Bedingungen: echte Menschen und echte Werte.
Bisher haben nur KI-Durchgänge den Code geprüft (Berichte unten). Eine
unabhängige menschliche Prüfung fehlt und ist Voraussetzung für den Start
mit mehreren Betreibern.

## Umfang

| Teil | Repo | Sprache | Zeilen (ohne Tests) | Priorität |
|---|---|---|---|---|
| Kettenknoten: Konsens, Blöcke, Kontostände, Nonces, P2P, HTTP/RPC | [Aequitas](https://github.com/hanoi96international-gif/Aequitas) `x/humanity/`, `cmd/` | Go | ca. 110.000 | 1 |
| Verträge `AequitasV7.sol`, `AequitasV8.sol` | Aequitas `contracts/` | Solidity | ca. 1.200 (mit Mocks) | 2 |
| Coordinator: Registrierung, Quorum, Lebendigkeit, Widerspruch | [aequitas-biometric-beta](https://github.com/hanoi96international-gif/aequitas-biometric-beta) `coordinator/` | Python (FastAPI) | ca. 9.100 | 1 |
| Vergleichsdienst: Gesichtsvergleich, Galerie, Duplikaterkennung, Verschlüsselung | aequitas-biometric-beta `matching-service/` | Python | ca. 15.700 | 1 |
| Proof-Server und Schaltkreis `biometric_v3.circom` (Groth16, Zeremonie v4) | [aequitas-proof-server](https://github.com/hanoi96international-gif/aequitas-proof-server) | JavaScript, Circom | ca. 1.600 und 79 | 2 |
| App: Schlüsselablage, Signatur, Aufnahme | [Aequitas-App](https://github.com/hanoi96international-gif/Aequitas-App) (Zweig `aequitas-app`) | TypeScript (Expo) | ca. 23.000 | 3 |
| Server-Workflows: Zugang zu C1, Freigaben, Schlüsselsicherung | Aequitas `.github/workflows/` | YAML, Bash | 50+ Dateien | 2 |

`Counter.sol` und die `Mock*`-Verträge sind Testhilfen und gehören nicht zum
Umfang. `forschung/` und `tools/` laufen nicht im Betrieb und gehören auch
nicht dazu.

## Die Fragen, auf die es ankommt

1. **Doppelte Prägung.** Kann ein Mensch zweimal registriert werden oder ein
   Startbetrag zweimal entstehen? Mögliche Wege: über den Vergleichsdienst,
   über den Coordinator, über die Kette (Nullifier, Bescheinigung im Konsens)
   oder über einen Wettlauf zwischen ihnen.
2. **Geld aus dem Nichts.** Lassen sich Kontostände, Nonces oder die
   StateRoot so manipulieren, dass ein Knoten einen falschen Block annimmt?
   Gemeint sind auch Blöcke von Peers, die Nachrechnung (Replay), das WAL
   und die Wiederherstellung aus einer Sicherung.
3. **Spaltung.** Kann ein Peer oder ein Nutzer bewirken, dass zwei ehrliche
   Knoten verschiedene Ergebnisse rechnen? Bekannt und behandelt sind die
   FMA-Fusion (`scripts/lint_fma.sh`) und der Lebensnachweis unter
   verteilter Annahme (#412).
4. **Biometrische Daten.** Liegen Vorlagen und Index wirklich nur
   verschlüsselt vor? Kann jemand von außen Gesichtsdaten oder die
   `bio_hash` eines anderen Menschen gewinnen?
5. **Lebendigkeit.** Wie weit tragen die Prüfungen gegen Bildschirm, Video,
   Face-Swap und virtuelle Kamera? Unsere eigene Einschätzung steht in
   `docs/LEBENDIGKEIT_GEGEN_DEEPFAKES.md`.
6. **Betrieb.** Kann jemand mit Schreibrecht auf ein Repo ohne Freigabe auf
   C1 gelangen? Geraten Geheimnisse in Protokolle oder Artefakte?
7. **ZK.** Sind Schaltkreis und Verifier korrekt eingeschränkt, ist der
   Nullifier eindeutig, und ist die Zeremonie v4 nachvollziehbar?

## Was die Prüfstelle bekommt

- Lesezugriff auf alle vier Repos (alle öffentlich), dazu die Testsuiten:
  Go mit `-race` und Postgres-Tests, Python mit pytest, Hardhat.
- Eine Testumgebung nach `docs/VALIDATOR_EINRICHTEN.md`. Auf Wunsch richten
  wir einen eigenen Prüfknoten ein, der von C1 synchronisiert.
- Die bisherigen eigenen Prüfungen als Ausgangspunkt, nicht als Ergebnis:
  `docs/AUDIT_VON_NULL_2026-10-02_BEWERTUNG.md`,
  `docs/SICHERHEITSPRUEFUNG_2026-10-25.md`,
  `docs/SICHERHEITSPRUEFUNG_BRANCH_2026-10-02.md` und Issue #412.
- Eine feste Ansprechperson, die Rückfragen innerhalb von zwei Werktagen
  beantwortet.

Nicht zum Umfang gehören Lasttests, Social Engineering und Angriffe auf die
Infrastruktur des Hosters (netcup).

## Was wir erwarten

- Ein Bericht mit Schweregrad je Befund, einem nachvollziehbaren Weg zum
  Fehler und einem Vorschlag zur Behebung.
- Eine Nachprüfung der Behebungen, im Angebot mit eingerechnet.
- Die Erlaubnis, den Bericht nach der Behebung zu veröffentlichen.

## Angebot bitte mit

Aufwand in Personentagen je Teil der Tabelle, frühester Beginn, Dauer,
Erfahrung mit Go-Ketten, ZK (Circom/Groth16) und Biometrie, und ob ein
Teilangebot möglich ist (zuerst Priorität 1).

Mögliche Prüfstellen, nur als Anhaltspunkt und nicht geprüft: Trail of
Bits, Least Authority, Cure53, Zellic, OtterSec, Veridise (ZK), NCC Group,
Code4rena oder Sherlock (Wettbewerbsformat), und für die Biometrie ein
Labor nach ISO/IEC 30107-3 (Presentation Attack Detection).

## Absender

Das Angebot muss an die Organisation gehen, die Aequitas betreibt. Solange
sie nicht gegründet ist, kann diese Anfrage nur als Voranfrage für Aufwand
und Preis verschickt werden. Absender und Kontakt setzt der Betreiber selbst
ein; hier stehen bewusst keine Namen.
