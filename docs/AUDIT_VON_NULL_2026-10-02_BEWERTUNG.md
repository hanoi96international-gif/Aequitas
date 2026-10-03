# Bewertung: „Aequitas — Audit von null“ (02.10.2026)

Das Audit eines Dritten wurde am 02.10.2026 gegen `main` 9ca2ab5 und den
Live-Knoten gemessen (18:23 CEST). Diese Bewertung prüft jede Aussage, die sich
**am Code oder an den Repos** prüfen lässt. Live-Werte (Höhe, Ports, Uptime,
Zähler) konnten von hier nicht gemessen werden; sie sind als „nicht
nachgeprüft“ markiert und werden nicht bezweifelt.

## Ergebnis in einem Satz

**Das Urteil „öffentliche Beta: nein“ ist richtig**, und die Blocker decken
sich mit der eigenen Launch-Checkliste. Vier Einzelbefunde sind aber falsch
oder überzeichnet; sie ändern das Urteil nicht, gehören aber nicht in eine
öffentliche Antwort darauf.

## 1. Bestätigt am Code

| Audit-Aussage | Beleg | Bewertung |
|---|---|---|
| StateRoot-Abweichung wird nur geloggt, Block angenommen | `block.go` ~7997, Meldung „accepted (TXs individually verified)“ ~8022 | richtig; bekannte Entscheidung, Sicherheit liegt in der Prüfung jeder Transaktion |
| Produzentenliste geschlossen, weil das Nachspielen nicht jeden Wert nachrechnet (K-2) | `produzenten_geschlossen`, Checkliste Blocker 18 | richtig. Für die Wirtschaft konkret: Liegegeld wird beim Nachspielen nur **beobachtend** geprüft (`liegegeld_pruefung.go`); vor einem zweiten Produzenten muss der gemeinsame Stichtag `nachrechnenStrengAbUnix` gesetzt sein (seit #274 gilt er auch für das Liegegeld; vorher nur je Knoten per `AEQUITAS_LIEGEGELD_PRUEFUNG`) |
| Herkunft eines Beweises nur 15 min im Arbeitsspeicher | `prove_provenance.go`, `proveHerkunftTTL`, `sync.Map` | richtig. Nach einem Neustart scheitert die Registrierung (schließt ab, kein Loch), der Mensch muss den Beweis neu erzeugen |
| Wallet nicht im Circuit, erst v4 | Kommentar in `prove_provenance.go` | richtig, bekannt |
| MPC-Tor lässt alles durch, solange `MPC_REQUIRED` nicht `true` | `mpc_api.go:388` | richtig und gewollt: MPC ist Gegenprobe, nicht Pflichtweg (Whitepaper 3.2). Die Duplikatprüfung läuft im Vergleichsdienst |
| Erfolgsmeldung sagt „V7“ | `register.go:352` | richtig, kosmetisch; korrigieren |
| Statusfeld heißt `contract_v7` | `api.go:1366` | richtig, kosmetisch; umbenennen oder V8-Feld daneben |
| „Launched June 2026“, „fairest money“, „nobody can print more“ | `landing.go` 561, 605, 705, 1345 | Texte stehen da. „Launched June 2026“ stimmt für diese Kette nicht (Start 30.09.) |
| Impressum/Datenschutz 404, DSGVO-Entscheidung offen, Zwei-Personen-Test, kein zweiter Operator, Workflow-Freigabe | Checkliste 02.10., Blocker 3, 5, 4, 6, 17 | richtig, deckungsgleich mit der eigenen Liste |

## 2. Falsch oder überzeichnet

| Audit-Aussage | Tatsächlich |
|---|---|
| „`/challenge` auf proof1 ist 404“ (Blocker 4) | `/challenge` ist ein **POST**-Endpunkt des **Coordinators** (`aequitas-biometric-beta/coordinator/app/main.py:511`), nicht des Proof-Servers. „Cannot GET“ ist die Antwort von Express auf eine Methode, die es dort nicht gibt. Die Forderung dahinter (einen Frisch-Wallet-Durchlauf belegt ablegen) bleibt richtig |
| „Slot-Falle: V7-Spiegel schreibt V7-Slots auf der V8-Kette, wenn der Fallback an ist“ | Auf V8 lehnt `register.go` (~627) den Ersatzweg **immer** ab, unabhängig von `ALLOW_V7_MIRROR_FALLBACK`: „V8 hat keinen Spiegel-Ersatz“. Keine Falle |
| „Öffentliche GitHub-Releases enden bei App 1.3.3 im Juli“ | `aequitas-app` hat v1.9.2 (02.10., 15:44 UTC), v1.9.1, v1.9.0, v1.8.9 – vor der Messung veröffentlicht. Vermutlich wurde ein anderes Repo angesehen. Ob die Website-URL eine APK liefert, ist davon unabhängig und wurde hier nicht gemessen |
| „Proof-Index 0 gegen 1 Mensch = Sybil-Loch“ | **Überzeichnet, aber mit echtem Kern.** `bio_hash_count` zählt den Zwischenspeicher des Proof-Servers (exakt gleiche Bio-Hashes). Die eigentliche Duplikatprüfung für ein Gesicht läuft im **Vergleichsdienst** über Propose/Commit mit Quorum; seine Galerie zählt `/health` dort (`gallery_real`, `gallery_test`). Die hat das Audit nicht gemessen. Der Kern: Der Proof-Server wird nach einer Registrierung nur benachrichtigt, wenn `PROOF_SERVER_URLS` **und** `CHAIN_SERVICE_TOKEN` auf dem Knoten gesetzt sind; sonst wird das **still übersprungen** (`notifyProofServer`, `attempted=false`). Eine Schutzschicht fehlt dann, ohne dass es jemand sieht |
| „Proof-Server-Quelltext nicht auf GitHub“ | liegt in `hanoi96international-gif/aequitas-proof-server`. Ist das Repo privat, war er für den Prüfer nicht sichtbar; dann ist die Aussage aus seiner Sicht richtig |

## 3. Nicht nachgeprüft (Live)

Höhe, Ports von C2, TLS auf C2, Uptime, Boot-Höhe, WAL-Zähler, Platte, Peers,
Bytecode-Größen, `bio_hash_count`. C2 ist laut Checkliste bewusst aus
(`C2_AKTIV=nein`); dass er von außen keine Kette ist, passt dazu.

## 4. Was daraus folgt

Zusätzlich zur eigenen Checkliste:

1. **Duplikatprüfung belegen:** Galerie-Zähler des Vergleichsdienstes
   (`gallery_real` + `gallery_test`) muss die Zahl der Menschen auf der Kette
   erklären. Dazu `PROOF_SERVER_URLS` und `CHAIN_SERVICE_TOKEN` auf C1 prüfen
   und das stille Überspringen in `notifyProofServer` sichtbar machen
   (Statusfeld oder Alarm), damit ein Index 0 nicht wieder unbemerkt bleibt.
2. **Website ehrlich machen:** „Launched June 2026“ entfernen; „fairest money“
   als Ziel formulieren, solange die Blocker offen sind.
3. **Kosmetik:** Registrierungsmeldung „V7“, Statusfeld `contract_v7`.
4. **Wirtschaft:** Liegegeld-Prüfung auf `streng`, bevor ein zweiter
   unabhängiger Produzent Blöcke erzeugt (`docs/UNTERNEHMEN_KONZEPT.md`,
   Abschnitt 11).
5. **Antwort an den Prüfer:** die vier Punkte aus Abschnitt 2 mit Beleg
   zurückgeben, damit die nächste Messung sie richtig ansetzt (POST auf den
   Coordinator, Galerie des Vergleichsdienstes, Repo `aequitas-app`).

### Umgesetzt am 02.10.2026 (Branch `claude/beta-launch-business-integration-vr7u4c`)

- **Punkt 1, Sichtbarkeit:** `notifyProofServer` überspringt nicht mehr still.
  Warnung im Log, und `/api/health/combined` zeigt unter `proof_server_sync`
  ob die Meldung eingerichtet ist (`konfiguriert`) und Zähler für
  `erfolgreich`, `uebersprungen`, `fehlgeschlagen` seit dem Start. Nur Ja/Nein
  und Zahlen, keine URL, kein Token (`proof_sync_status.go`, Test
  `TestProofSyncUebersprungenIstSichtbar`). **Offen, nur auf C1 prüfbar:** ob
  `PROOF_SERVER_URLS` und `CHAIN_SERVICE_TOKEN` gesetzt sind, und ob
  `gallery_real` im Vergleichsdienst die Zahl der Menschen erklärt.
- **Punkt 2, Website:** „Launched June 2026“ ersetzt durch „Restarted at zero
  on 30 Sep 2026“, „The fairest money in the world“ durch „On the way to the
  fairest money in the world“, in allen 12 Sprachen auf Startseite und
  Explorer.
- **Punkt 3, Kosmetik:** Registrierungsmeldung nennt die geltende
  Vertragsfassung statt „V7“. `/api/status` hat das neue Feld
  `register_contract`; `contract_v7` bleibt als alter Name für bestehende Leser.

### Umgesetzt am 02.10.2026, zweiter Teil

- **Herkunftsnotiz übersteht einen Neustart.** Zusätzlich zum Arbeitsspeicher
  eine knotenlokale Tabelle `prove_herkunft` (kein Kettenzustand, nicht
  repliziert). Gelesen nur, wenn der Arbeitsspeicher nichts weiß; jeder
  Fehler heißt „keine Herkunft“; Wartezeit je Anfrage 2 s; Abgelaufenes wird
  bei jedem Schreiben gelöscht (`prove_herkunft_dauerhaft.go`). Tests gegen
  echtes Postgres: Neustart (richtige Wallet ja, fremde nein), abgelaufene
  Zeile, Datenbank weg.
- **Welcher Vertrag läuft.** `/api/status` zeigt `register_contract_code` und
  `bio_verifier_code`: Adresse, keccak256 des Laufzeit-Codes, Länge. Jeder kann
  ihn mit dem eigenen Kompilat oder einem zweiten Knoten vergleichen
  (`vertrag_code_hash.go`, Test gegen echtes Postgres).
- **Liegegeld streng:** im Code fertig. Der Test
  `TestLiegegeldPruefungBeimNachspielen` zeigt Zählen bei `beobachten` und
  Ablehnen bei `streng`, der Blockpfad rollt den ganzen Block zurück. **Aber:**
  Abweichungen zählt nur ein Knoten, der fremde Blöcke nachspielt. C1 erzeugt
  selbst, C2 ist aus; heute beobachtet also niemand, und das Kriterium
  „Wochen ohne Abweichung“ kann nicht erfüllt werden. Der Weg: Der lesende
  Validator eines zweiten Betreibers (Audit-Blocker 6) läuft mit
  `beobachten` (Voreinstellung); sein `/api/wirtschaft/regeln` →
  `liegegeld_pruefung.abweichungen` und `nachrechnen.abweichungen` müssen
  über die Zeit 0 bleiben, dann wird der gemeinsame Stichtag
  `nachrechnenStrengAbUnix` gesetzt (gilt für alle Knoten zugleich, seit
  #274 auch für das Liegegeld), bevor ein zweiter Knoten Blöcke erzeugt.

## 5. Bei dieser Prüfung zusätzlich gefunden (Wirtschaft)

- **Gründungsphase doppelt über Mitinhaber-Reihenfolge.** Beim Aufnehmen eines
  Mitinhabers wurde die Liste der Verantwortlichen alphabetisch sortiert; der
  „Gründer“ an erster Stelle konnte wechseln, und eine zweite Firma derselben
  Gründerin bekam im selben Jahr noch einmal die Gründungsphase (nachgestellt:
  80 statt 180 AEQ/Monat). Korrigiert in `wirtschaft.go`
  (`applyUnternehmenMitinhaberLocked`), Missbrauchstest
  `TestGruenderBleibtNachMitinhaber`. Konsensrelevant: Auf der Live-Kette gibt
  es einen Menschen, also kein Unternehmen mit zwei Verantwortlichen; das
  Nachspielen ändert sich nicht. Vor dem Merge eigene Sicherheitsprüfung.
- Geprüft und **in Ordnung**: Grenze von 250 AEQ für freie Adressen gilt auch
  beim Tausch Stable → AEQ; Schließen eines Unternehmens nur durch einen
  Verantwortlichen (Schnittstelle und Nachspielen).
