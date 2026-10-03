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

---

# Nachtrag: zweite Stufe der Wirtschaft (`wirtschaft2.go`, `kundschaft.go`)

Getrennter Durchgang über die Commits `823839d` und `78c9982` sowie die
Testkorrektur in `block_orphan_grace_test.go`.

## Ergebnis

Keine blockierenden Befunde. Zwei bewusst getragene Restrisiken (B, C) und
zwei Hinweise zum Aufwand.

## C: Ablehnen statt wegnehmen (Konsens-nah, Annahme)

- **Wer erreicht es:** jede Überweisung und jeder Tausch Stable → AEQ
  (öffentlich).
- **Nachspielen unverändert:** Die Prüfung sitzt nur in den Annahmepfaden
  (`transferMutateLocked`, `swapLockedMitAbgabe`). Bloecke werden über
  `applyTransferDeltaLockedSammelnd` angewandt und kappen wie bisher. Ein
  Block eines alten Knotens wird daher genauso nachgespielt; es gibt keinen
  neuen Ablehnungsgrund beim Nachspielen.
- **Gleiche Rechnung wie die Kappung:** Durchschnitt mal Multiplikator, Guthaben
  plus LP-Wert (`TestGrenzpruefungZaehltLPAnteile`). Eine Zahlung, die hier
  durchgeht, wird dort nicht gekappt.
- **Fail-closed:** Vor jeder Buchung geprüft; nach Ablehnung ändert sich nichts
  (`TestUeberGrenzeWirdAbgelehntStattGekappt`, `TestTauschUeberGrenzeAbgelehnt`).
- **Restrisiko (getragen):** Jemand kann ein Menschenkonto bis an die Grenze
  „vollschenken“, sodass weitere Zahlungen abgelehnt werden. Das kostet den
  Angreifer genau das verschenkte Geld, das Opfer kann es ausgeben oder
  weitergeben. Vorher wurde der Überschuss dem Opfer genommen; der neue
  Zustand ist für das Opfer besser.
- **Nicht in der verteilten Annahme** (Stufe 2 der Annahme, inaktiv): dort
  bleibt die verschobene Kappung. Muss bei deren Aktivierung neu bewertet
  werden.

## A: Erstes Unternehmen wie ein Mensch (Konsens: Liegegeld)

- **Deterministisch:** folgt allein aus dem Register (`erstesOffenesLocked`,
  kleinste Eröffnungszeit, bei Gleichstand kleinste Adresse).
- **Kein Vervielfachen:** nur ein Unternehmen je Gründerin; der Platz bis zur
  Grenze wird mit ihrem eigenen Guthaben geteilt
  (`TestNurDasErsteUnternehmenWieEinMensch`,
  `TestErstesUnternehmenTeiltDenPlatzMitDerGruenderin`).
- **Hinweis Aufwand:** ein Durchlauf über das Register je Unternehmen im
  Tageslauf, also quadratisch (wie die bestehende Gründungsphase). Bei 10.000
  Unternehmen etwa 10^8 Vergleiche je Tag; vor einer Größenordnung mehr
  braucht es einen Index.

## B: Firmen-Eingänge gedeckelt (Konsens: Liegegeld)

- **Deterministisch:** Buchführung in `nachUeberweisung`, das Annahme und
  Nachspielen mit derselben Buchungszeit aufrufen
  (`TestStufeZweiNachspielenGleichWieErzeuger`).
- **Rollback:** neues Tagesfeld wird in `kopie()` als Wert mitkopiert.
- **Restrisiko (getragen):** Ein Kreis befreundeter Firmen bekommt je zahlender
  Firma bis 9.000 AEQ im Quartal als Umsatz (`TestKreisZwischenFirmenIstGedeckelt`).
  Jede zahlende Firma braucht einen eigenen verifizierten Menschen; Ersparnis
  bis etwa 70 AEQ/Monat je Partner. Dafür kostet Weitergeben an Lieferanten
  nichts mehr. Hin und zurück bringt nur einer Seite etwas
  (`TestHinUndZurueckZwischenZweiFirmen`).
- **Hinweis Größe:** Der Zähler je zahlender Firma liegt in deren Buchkonto und
  wird bei jeder Überweisung als JSON geschrieben. Er wächst mit der Zahl der
  verschiedenen Firmen, an die sie im laufenden und vorigen Quartal zahlt.
  Begrenzt durch das Register, aber bei sehr vielen Lieferanten spürbar.

## Aktivierung

`wirtschaft2AktivAbUnix` = 15.10.2026 00:00 UTC. **Alle Knoten müssen vorher
ausgerollt sein.** Ein alter Knoten würde A und B nicht anwenden; die
Liegegeld-Prüfung beim Nachspielen ist heute nur beobachtend, er würde also
nicht ablehnen, aber Abweichungen zählen. Wird bis zum 14.10. nicht
ausgerollt: Datum vorher verschieben.

## Kundschaft und Grundeinkommen (`kundschaft.go`, Anzeige)

- **Wer erreicht es:** `/api/unternehmen`, `/api/wirtschaft/regeln`, öffentlich.
- **Begrenzt:** je Zahl eine Sammelabfrage mit 5 s Wartezeit, 10 Minuten
  zwischengespeichert, nur eine Rechnung zugleich (`TryLock`); weitere Anfragen
  bekommen den alten Wert. Nur Parameter, keine zusammengesetzten SQL-Texte.
- **Fehlerfall:** alter Wert bleibt, sonst `null`, nie eine erfundene 0.
- **Index:** `CREATE INDEX CONCURRENTLY IF NOT EXISTS` im Hintergrund-Schreiber
  des Verlaufs, nicht auf dem Blockpfad. Scheitert das Anlegen, bleibt ein
  ungültiger Index stehen und die Abfrage wird langsamer, aber nicht falsch.
- **Kein Geldvorteil:** Die Zahl ist nur Anzeige; keine Regel liest sie.

## Testkorrektur (`block_orphan_grace_test.go`)

Der Mauer-Zähler (`replay_mauer.go`) ist paketweit. Abweisungen von Block #1
aus unabhängigen Tests summierten sich, und die dritte löste die
Selbstheilung samt `os.Exit` aus; je nach Testauswahl brach der Lauf ab. Jeder
Test-DAG setzt den Zähler jetzt zurück. Produktionscode unverändert.
