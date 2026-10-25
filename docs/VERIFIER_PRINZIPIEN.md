# Verifier-Prinzipien: nur bestätigen, nie wegnehmen

Stand 18.10.2026. Dieses Dokument hält fest, nach welchen Grundsätzen
Verifier, Betreiber und Erzeuger in Aequitas handeln dürfen, wo die Kette
heute davon abweicht und in welcher Reihenfolge das geschlossen wird. Es
wird mit jedem Schritt fortgeschrieben.

## Die vier Grundsätze

1. **Nur bestätigen, nie wegnehmen.** Ein Verifier oder Betreiber kann
   bestätigen, dass ein Mensch einzigartig und lebendig ist. Er kann einem
   registrierten Menschen weder das Menschsein noch sein Guthaben nehmen –
   weder über einen Block noch über eine Umgebungsvariable, eine
   Datenbankanweisung oder einen Betreiberzugang. Was einem Menschen
   genommen wird, nimmt nur er selbst (mit seiner Unterschrift) oder eine
   Regel, die jeder Knoten selbst nachrechnet und streng durchsetzt.
2. **Mehrere austauschbare Verifier.** Kein einzelner Verifier entscheidet
   allein. Bestätigungen kommen von mehreren, voneinander unabhängigen
   Betreibern (verschiedene Menschen), jeder ist ersetzbar, und der Ausfall
   oder Entzug eines Verifiers macht keine frühere Bestätigung ungültig.
3. **Nichts sehen.** Ein Verifier braucht für seine Bestätigung keine
   Rohdaten. Biometrie bleibt auf dem Gerät oder ist so aufgeteilt, dass
   kein Einzelner sie zusammensetzen kann; gespeichert wird nur, was für
   den Abgleich nötig ist, und nicht im Klartext.
4. **Überprüfbar.** Jede Bestätigung ist eine Unterschrift, die jeder
   Knoten beim Nachspielen selbst prüft. Welche Schlüssel bestätigen dürfen,
   steht öffentlich und nachvollziehbar fest. Niemand muss einem Betreiber
   glauben.

## Befunde (Stand heute)

### 1. CLEAR_REGISTRATIONS konnte alle Menschen löschen – geschlossen (A1)

`clearRegistrationsFromDB` (`x/humanity/keeper/state.go`) setzte auf dem
Knoten jeden Menschen auf `is_human = false` und jedes Guthaben auf 0 –
knotenlokal, ohne Block, ohne Unterschrift eines Betroffenen. Dafür
reichten drei Umgebungsvariablen und ein Neustart. Auf dem echten Netz
wäre das ein Entzug durch den Betreiber.

Seit A1 verweigert der Knoten das, laut (`[ALERT] [CLEAR-REG] VERWEIGERT`)
und ohne jeden Datenbankzugriff, wenn eines davon zutrifft
(`hauptnetz_schutz.go`):

- `IS_PRIMARY_NODE=true` (wie schon bei `RESET_DB_STATE`);
- `genesis.json` fehlt, ist unlesbar oder nennt keine `chain_id` –
  dann ist das Netz nicht feststellbar (fail-closed);
- `chain_id` der Genesis ist `aequitas-1` (das Hauptnetz);
- `genesis_time` ist die einer Hauptnetz-Genesis (V8-Genesis vom
  30.09.2026 18:10 UTC oder die alte vom 13.06.2026) – auch bei einer
  Kopie mit umbenannter `chain_id`.

Die EVM-Chain-ID (1926 / 19260930) taugt als Signal nicht: sie ist eine
Konstante im Programm und steht in jedem Build, auch auf Testketten.
Testketten mit eigener Genesis (eigene `chain_id`, eigene Zeit, siehe
`STAGING_RUNBOOK.md`) können den Weg mit allen bisherigen Bestätigungen
weiter benutzen.

Tests: `hauptnetz_schutz_test.go` (Missbrauch: alle Bestätigungen gesetzt,
Hauptnetz-Genesis → kein einziger Datenbankzugriff; Gegenversuch auf einer
Testkette; mit echter Datenbank bleibt der Mensch Mensch, mit Guthaben und
Nullifier).

### 1b. Betreiberzugang zur Box – offen

Das Tor schließt nur den Weg über Umgebungsvariablen. Wer Shell- oder
Datenbankzugriff auf C1 hat, kann den lokalen Stand weiterhin direkt
ändern. Eine abweichende StateRoot wird heute nur gewarnt
(`block.go`, „accepted“), und Auto-Heal übernimmt den Snapshot von C1.
Grundsatz 1 gilt für den Betreiberzugang daher erst, wenn andere Knoten
selbst nachrechnen und streng ablehnen (A3, B3) und Snapshots nicht mehr
von einem einzigen Betreiber kommen (A6/B4). Bis dahin ist der Betreiber
von C1 vertrauenswürdig – so ehrlich muss es hier stehen.

### 2. Kein Block nimmt das Menschsein – jetzt als Invariante getestet

`kein_entzug_test.go` spielt für **jede** Transaktionsart im
Nachspiel-Switch (`replayTransactions`) gefälschte Transaktionen mit
Zufallsfeldern gegen einen registrierten Menschen nach, der nichts
unterschrieben hat – einzeln und gemischt, ohne und mit echter Datenbank.
`IsHuman` bleibt in jedem Fall wahr. Kommt eine neue Art hinzu, scheitert
der Test, bis sie bewusst eingeordnet ist.

Ein Wächter-Test durchsucht alle Nicht-Test-Dateien des Keepers nach SQL
(`SET … is_human = false/'f'/0/NOT`) und Go-Code (`IsHuman = false`,
`IsHuman: false`, `IsHuman = !…`), der das Menschsein wegnimmt. Erlaubt ist
allein `clearRegistrationsFromDB` – und die verweigert auf dem Hauptnetz.
Der Wächter ist ein Stolperdraht für wörtliche Rückschritte, keine
vollständige Analyse: `IsHuman = v` mit `v == false`, ein neu gebauter
Kontosatz ohne `IsHuman`, SQL mit Parameter (`SET is_human = $1`),
`DELETE`/`TRUNCATE` oder Code außerhalb des Pakets erkennt er nicht. Die
eigentliche Absicherung gegen Entzug über Blöcke ist der Invariantentest
oben.

Zeilen in `chain_accounts` verschwinden außerdem beim Import eines
signierten Snapshots (`snapshot.go`, `RESET_DB_STATE` auf einem
Nicht-Primary) und beim Zurückrollen eines abgewiesenen Blocks. Beides
stellt einen anderen, vollständigen Stand her, statt einzelnen Menschen
etwas zu nehmen; der Snapshot hängt aber am Vertrauen in seinen
Unterzeichner und gehört zu A6/B4.

### 3. Geld ohne Unterschrift – bekannt, nur beobachtet

Regeln, die einem Konto nach Regel ohne Unterschrift Geld nehmen:

| Art | Regel | Durchsetzung heute |
|---|---|---|
| `escrow_move` | Treuhand bei Inaktivität (`treuhand_*`) | `nachrechnen_treuhand.go`, nur beobachtend |
| `umlauf` | Umlaufsicherung / Liegegeld | `pruefeUmlaufLocked`, streng nur bei LP-Auflösung oder `AEQUITAS_LIEGEGELD_PRUEFUNG=streng` (je Knoten!) |
| `kappung` | Vermögensgrenze | Obergrenze `kappung_unter_grenze`, nur beobachtend |
| `slash_equivocation`, `slash_abrechnung` | Strafe für Doppelsignatur eines Validator-Betreibers | Beweis mit zwei eigenen Signaturen, Verlauf |

`nachrechnenStrengAbUnix` steht auf `MaxInt64`: jede Abweichung wird
gezählt und unter `/api/wirtschaft/regeln` veröffentlicht, der Block gilt.

**Befund aus dem neuen Test:** auch drei Gutschriftarten
(`ubi_distribution`, `lp_distribution`, `validator_distribution`) senken
heute ein Guthaben, wenn der Erzeuger einen negativen Betrag oder eine
erfundene Demurrage einträgt – `betrag_ungueltig` und
`demurrage_nach_umstellung` werden ebenfalls nur beobachtet. Der Test hält
das in `heuteGeldOhneUnterschrift` fest; die Liste darf nur kürzer werden.
Verhalten in A1 unverändert, geschlossen mit A2.

Arten, die ein Mensch selbst unterschreibt (Überweisung, Tausch,
Liquidität, Vorbehalt, Unternehmen, Vormund), senken gefälscht kein
Guthaben: die Unterschriftsprüfung läuft in `replayTransactions` auf jedem
Knoten selbst.

### 4. Kernbefund: Verifier-Signaturen werden beim Nachspielen nicht geprüft

Heute prüft beim Nachspielen eines `register_human` jeder Knoten den
ZK-Beweis, die Bindung des Nullifiers an den Beweis, die V8-Unterschrift
des Menschen und die Einmaligkeit des Nullifiers. **Kein Knoten prüft, dass
ein Verifier (Coordinator, Vergleichsdienst) die biometrische
Einzigartigkeit bestätigt hat.** Der ZK-Beweis zeigt nur die Kenntnis eines
Geheimnisses, nicht dass dahinter ein neuer, lebendiger Mensch steht.

Folge: ein Erzeuger könnte mit selbst erzeugten Beweisen und frischen
Wallets beliebig viele „Menschen“ prägen, und jeder Knoten würde sie
übernehmen. Das ist tragbar, **solange es genau einen Erzeuger gibt – den
Betreiber**, der ohnehin den Registrierungsweg betreibt. Mit dem zweiten
Erzeuger ist es das nicht mehr: **A3 muss vor dem zweiten Erzeuger streng
sein.**

## Plan

### Phase A – jetzt, ein Server

- **A1 – diese Invariante.** `CLEAR_REGISTRATIONS` auf dem Hauptnetz
  verweigert; Invariantentest „kein Block nimmt das Menschsein“; Wächter
  gegen `is_human = false`; dieses Dokument. *(erledigt)*
- **A2 – Regeln, die Geld wegnehmen, streng.** `treuhand_*`, `liegegeld`,
  `kappung` (und `betrag_ungueltig`, `demurrage_nach_umstellung` der
  Gutschriften) regelweise streng schalten, sobald die Beobachtungsdaten
  0 Abweichungen zeigen. Stichtag fest im Code, nicht je Knoten per
  Umgebungsvariable.
- **A3 – Registrierungsnachweis im Konsens.** Coordinator- und
  Vergleichsdienst-Signaturen über Nullifier + Wallet im Block; jeder Knoten
  prüft k-von-n Unterschriften verschiedener Betreiber-Menschen. Erst
  beobachtend, dann streng – spätestens vor dem zweiten Erzeuger.
- **A4 – Löschen per Wallet nur mit Unterschrift des Menschen.** Ein
  Mensch kann seine Registrierung selbst aufgeben; niemand sonst.
- **A5 – austauschbare Matcher-Schnittstelle.** Der Abgleich hinter einer
  Schnittstelle, die mehrere Implementierungen/Betreiber erlaubt;
  `bio_hash` nicht mehr im Klartext.
- **A6 – Transparenz des Schlüsselsets.** Welche Schlüssel bestätigen
  (Verifier, Coordinator, Snapshot-Unterzeichner), öffentlich und mit
  Verlauf.

### Phase B – drei unabhängige Betreiber

- **B1 – Verifier-Register auf der Kette.** Zulassung durch 2/3
  verschiedener zugelassener Menschen; ein Entzug wirkt nie rückwirkend
  (früher bestätigte Menschen bleiben Menschen).
- **B2 – Register-Leser einschalten.**
- **B3 – Quorum 2 von 3 im Konsens streng;** `nachrechnenStreng` vor dem
  zweiten Erzeuger.
- **B4 – Proof-Server ohne Vertrauensrolle.** Er rechnet nur; was er
  liefert, prüft jeder Knoten selbst.
- **B5 – mehrere Coordinatoren.**

### Phase C

- MPC als entscheidende Instanz für den Abgleich.
- Circuit v4.
- Transparenzlog für Bestätigungen und Schlüssel.
- Aufteilung der biometrischen Daten auf dem Gerät.

## Festgelegte Entscheidungen (18.10.2026)

Diese Entscheidungen leiten sich aus dem Leitbild ab: Geld eines Menschen für
alle Menschen, so fair wie möglich. Bis eine Mehrheit der Betreiber etwas
anderes beschließt, gelten sie als Vorgabe. Jede Abweichung braucht eine
schriftliche Begründung in diesem Dokument.

### Verifier-Zulassung (für B1)

1. **Ein Mensch, ein Verifier.** Verifier kann nur ein registrierter Mensch
   werden, und jeder Mensch zählt einmal. Mehrere Schlüssel, Server oder
   Firmen machen niemanden zu mehreren Verifiern.
2. **Kein Kapitaleinsatz.** Für die Zulassung zählen weder Guthaben noch
   Stake. Wer mehr Geld hat, bekommt dadurch keine Stimme mehr.
3. **Zulassung mit 2/3** der bereits zugelassenen Verifier, auf der Kette und
   mit Unterschriften. Damit B1 starten kann, sind am Anfang drei Betreiber
   gesetzt. Jeder weitere wird durch Abstimmung aufgenommen.
4. **Austritt jederzeit und ohne Bedingung.**
5. **Entzug nur aus zwei Gründen:** durch einen Betrugsbeweis auf der Kette,
   etwa zwei widersprüchliche Bestätigungen derselben Registrierung, oder
   durch einen Beschluss von 2/3 der Verifier. Ein Entzug wirkt **nie
   rückwirkend**. Menschen, die der Verifier früher bestätigt hat, bleiben
   Menschen (Grundsatz 1).
6. **Verschiedene Betreiber-Menschen.** Das Quorum k-von-n in A3 und B3 zählt
   verschiedene Menschen, nicht verschiedene Schlüssel.

### Klasse „gelb“: 800 AEQ werden nicht auf Dauer zurückgehalten

Heute (`LEBENDIGKEIT_GEGEN_DEEPFAKES.md`, Zeile 66) pausiert die Staffel einer
gelben Registrierung ohne Zeitgrenze, solange die zweite
Lebendigkeitsprüfung fehlt. Damit verliert ein Mensch, den die Prüfung zu
Unrecht als unsicher einstuft, den größten Teil seines Grundbetrags auf
unbestimmte Zeit. Das verträgt sich nicht mit „nur bestätigen, nie
wegnehmen“.

**Festgelegt:**

- Die zweite Prüfung beschleunigt die Staffel: Sie läuft dann wie bisher.
- Bleibt die zweite Prüfung aus, werden die 800 AEQ trotzdem freigegeben,
  **30 Tage nach der Registrierung**.
- Die automatische Freigabe kann nur ein **begründeter Einspruch** anhalten.
  Er muss von verschiedenen Betreiber-Menschen unterschrieben sein (dasselbe
  Quorum wie in A3), einen nachprüfbaren Grund enthalten und vor Tag 30
  vorliegen. Ein Einspruch hält die Freigabe nur an. Er nimmt nichts weg,
  was schon ausgezahlt ist.
- Umsetzung in `grant_staffel.go`: Die Staffel ist heute ruhend (Aktivierung
  2100). Bis dahin bekommen alle Registrierungen den vollen Betrag sofort, und
  niemand wird benachteiligt.
- **Eingebaut 25.10.2026** (schlafend wie die Staffel):
  `staffelFreigabeHoechstens` gibt ab Tag 30 den ganzen Rest frei, mit
  Zweitprüfung vorher die Tagesrate; wer die Prüfung macht, ist nie
  langsamer. Der Einspruch (`staffel_einspruch.go`, Transaktion
  `staffel_einspruch`) braucht Bescheinigungen von **zwei verschiedenen**
  zugelassenen Betreiber-Menschen, einen Grund (10–280 Byte, Hash in jeder
  Unterschrift) und muss vor Tag 30 bescheinigt sein. Er hält die Freigabe
  höchstens bis Tag 180 an; eine neuere Zweitprüfung des Menschen beendet
  die Pause sofort. Nur der erste Einspruch zählt. Jeder Knoten prüft alles
  selbst; `StaffelEinspruchAm` steht im Blatt der StateRoot, nur wenn
  gesetzt. Missbrauchstests: `staffel_einspruch_test.go`. Der Grund steht
  für immer öffentlich in der Kette: **keine personenbezogenen Daten**, nur
  der Befund und eine Vorgangskennung (z. B. „W-…“ aus dem Widerspruchsweg).

### Jährlicher Lebensnachweis (21.10.2026)

Das Grundeinkommen soll an lebende Menschen gehen, die ihr Konto selbst
halten — nicht an Verstorbene oder gekaufte Konten. Einmal im Jahr
bestätigt deshalb jeder mit demselben Gesicht wie bei der Registrierung,
dass er lebt (`x/humanity/keeper/lebensnachweis.go`, Transaktion
`proof_of_life`; Coordinator `/lebensnachweis`).

**Wie das mit Grundsatz 1 zusammengeht:**

- **Nichts wird weggenommen.** Mensch-Status und Guthaben bleiben
  unangetastet, auch nach Jahren ohne Nachweis. Kein Code setzt dafür
  `is_human` auf `false`; `proof_of_life` ist in `kein_entzug_test.go` als
  reine Gutschrift-Art eingeordnet.
- **Nichts wird zurückgehalten.** Wer überfällig ist, nimmt an der
  Verteilung nicht teil, solange er überfällig ist; sein Anteil geht an alle
  anderen, es entsteht kein Guthaben, das jemand verwaltet oder freigeben
  müsste. Das ist keine Regel, die „Geld ohne Zeitgrenze zurückhält“: es
  gibt kein zurückgehaltenes Geld, und das Ende der Pause liegt allein beim
  Menschen selbst — nach dem Nachweis läuft der Anteil ab der nächsten
  Runde.
- **Nur, wenn ein Nachweis möglich ist.** Pausiert wird nur, solange
  Coordinatoren im Konsens zugelassen sind (`registerLeserAktiv`). Die Uhr
  beginnt am späteren von Stichtag (01.11.2026) und Zulassung
  (`lebensnachweisUhrAb`): niemand wird beim Einschalten ohne Vorlauf
  pausiert.
- **Großzügige Fristen.** Frühestens nach 300 Tagen bestätigen, Frist 365
  Tage, danach 30 Tage Nachfrist.
- **Jeder Knoten prüft selbst** die Bescheinigung (Ed25519 des
  Coordinators, eigene Domäne `aequitas-proof-of-life-v1`, Bindung des
  Schlüssels an einen zugelassenen Menschen) und rechnet nach, dass kein
  Pausierter einen Anteil bekommt (`nachrechnen_ubi.go`, Befund
  `ubi_pausiert`) — beobachtend bis `nachrechnenStrengAbUnix`.

### Rechtsrahmen

Die Entwürfe liegen in `docs/RECHTSVORLAGEN_ENTWURF.md`. Es sind
**Entwürfe für eine Anwältin oder einen Anwalt**, keine geprüften Texte.

## Regeln für jede Änderung

- Neue Transaktionsarten werden in `kein_entzug_test.go` eingeordnet
  (`replayArten`); eine Art, die Geld ohne Unterschrift nimmt, braucht eine
  Begründung in `bekannteGeldAusnahmen` und eine strenge Nachrechnung.
- Kein Code außer `clearRegistrationsFromDB` setzt `is_human` auf `false`.
- Ein Verifier-Entzug wirkt nie rückwirkend.
- Keine Regel hält einem Menschen Geld ohne zeitliche Grenze zurück.
