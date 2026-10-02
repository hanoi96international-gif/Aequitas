# Sicherheitsprüfung: Branch `claude/beta-launch-business-integration-vr7u4c`

Getrennter Durchgang über den Diff gegen `main` (9ca2ab5), 02.10.2026, nach
AGENTS.md („Änderungen an Konsens, Signaturen, Kontoständen, Contracts oder
Netzwerk-Eingängen“). Gehört als Kommentar in den PR, sobald es einen gibt.

## Ergebnis

**Ein Befund in den eigenen Änderungen, behoben.** Sonst keine Befunde. Eine
Bedingung vor dem Ausrollen (Abschnitt 1).

## 1. Gründer bleibt an erster Stelle (`wirtschaft.go`, Konsens)

- **Wer erreicht es:** `unternehmen_mitinhaber`, signiert von zwei Menschen;
  Annahme und Nachspielen.
- **Änderung:** Verantwortliche werden angehängt statt sortiert. Alle anderen
  Verwendungen (`istVerantwortlich`, `gemeinsameVerantwortliche`, Anzahl) sind
  reihenfolgeunabhängig; Speichern, Snapshot und Laden behalten die
  Reihenfolge; Doppelte verhindert die bestehende Prüfung.
- **Determinismus:** Die Reihenfolge folgt der Blockreihenfolge, ist also auf
  jedem Knoten gleich.
- **Bedingung vor dem Ausrollen:** Ein Knoten, dessen Datenbank schon ein
  Unternehmen mit zwei oder mehr Verantwortlichen in sortierter Reihenfolge
  hält, würde nach dem Update anders rechnen als ein Knoten, der neu
  nachspielt. Laut Audit gibt es auf der Kette einen Menschen, also kann es
  kein solches Unternehmen geben. **Vor dem Deploy auf C1 prüfen:**
  `SELECT count(*) FROM wirtschaft_unternehmen WHERE verantwortliche LIKE '%,%'`
  muss 0 sein. Ist es nicht 0: nicht ausrollen, sondern eine Aktivierungszeit
  einbauen.
- **Missbrauchstest:** `TestGruenderBleibtNachMitinhaber` (fällt mit dem alten
  Code).

## 2. Herkunftsnotiz in der Datenbank (`prove_herkunft_dauerhaft.go`)

- **Wer erreicht es:** Schreiben nur nach einer erfolgreichen
  `/api/prove`-Antwort (Proof-Server hat Bescheinigung geprüft). Lesen über
  `/api/register`, öffentlich.
- **Befund (behoben):** In der ersten Fassung fragte jede Registrierung mit
  unbekanntem Nullifier die Datenbank, bis 2 s lang. `/api/register` hat im
  Knoten keine Ratenbremse; viele gleichzeitige Anfragen hätten
  Datenbankverbindungen gebunden, die die Blockerzeugung braucht (Regel 2).
  **Behoben:** Gelesen wird nur in den ersten 15 Minuten nach dem Start (danach
  kennt der Arbeitsspeicher jede gültige Notiz) und höchstens 4 Anfragen
  gleichzeitig; ist die Grenze voll, gibt es keine Herkunft.
- **Fehlerfall:** Jeder Datenbankfehler heißt „keine Herkunft“; die
  Registrierung wird abgelehnt.
- **Prüfung bleibt:** Nullifier kanonisch, genau diese Wallet, nicht älter als
  15 Minuten, wie im Arbeitsspeicher. Die Nullifier-Einmaligkeit prüfen weiter
  Vertrag und `TryClaimNullifier`.
- **Missbrauchstests (echtes Postgres):** fremde Wallet nach Neustart,
  abgelaufene Zeile, Datenbank weg, Fenster vorbei, Grenze voll.
- **Hinweis, kein Befund:** `/api/register` ohne Ratenbremse im Knoten ist
  älter als dieser Branch. Ob davor ein Proxy bremst, ist von hier nicht
  prüfbar; das sollte auf C1 geklärt werden.

## 3. Code-Hash der Verträge im Status (`vertrag_code_hash.go`)

- **Wer erreicht es:** `/api/status`, öffentlich.
- **Begrenzt:** Hash wird beim ersten Fund festgehalten; ohne Code höchstens
  eine Datenbankanfrage je Minute und Adresse. Läuft ohne `cs.mu`
  (`TestStatus_AntwortetAuchBeiGehaltenerSperre` grün).
- **Rest:** `LoadContract` hat keine eigene Wartezeit; durch den
  Zwischenspeicher tritt das höchstens einmal je Minute auf.
- Veröffentlicht wird nur, was über `eth_getCode` ohnehin öffentlich ist.

## 4. Sichtbarkeit der Proof-Server-Meldung (`proof_sync_status.go`)

- Veröffentlicht: `konfiguriert` (ja/nein) und drei Zähler. Keine URL, kein
  Token; Test prüft, dass kein Feld Text ist.
- Abwägung: „konfiguriert: false“ verrät, dass eine Schicht fehlt. Die
  eigentliche Duplikatprüfung liegt im Vergleichsdienst; Sichtbarkeit wiegt
  hier schwerer (Audit).

## 5. Texte (Website, Explorer, Registrierungsmeldung)

- Feste Texte, keine Benutzereingaben; `innerHTML` setzt nur eigene
  Übersetzungen. Registrierungsmeldung ohne Geheimnisse.
- `contract_v7` bleibt als Alias, damit bestehende Leser nicht brechen.

## Tests

`go test ./x/humanity/keeper/` grün mit `DATABASE_URL` (einschließlich
`_RealDB`); geänderte Bereiche zusätzlich mit `-race`.
