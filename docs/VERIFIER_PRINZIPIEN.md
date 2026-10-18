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

## Regeln für jede Änderung

- Neue Transaktionsarten werden in `kein_entzug_test.go` eingeordnet
  (`replayArten`); eine Art, die Geld ohne Unterschrift nimmt, braucht eine
  Begründung in `bekannteGeldAusnahmen` und eine strenge Nachrechnung.
- Kein Code außer `clearRegistrationsFromDB` setzt `is_human` auf `false`.
- Ein Verifier-Entzug wirkt nie rückwirkend.
