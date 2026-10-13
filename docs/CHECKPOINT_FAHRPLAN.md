# Fahrplan: vom Komitee unterschriebene Checkpoints (OFFEN 10, Stufe C)

Stand 13.10.2026. Ziel: ein Bezugspunkt, den kein einzelner Validator
steuern kann – Grundlage für viele Validatoren, für den Anker der
Validatoren-Belohnung (#340) und für Finalität. GHOSTDAG-Werte taugen dafür
nicht (nicht im Blockhash, Startwerte lokal, ein Erzeuger kann Höhe und
Blue-Score aufblähen; Untersuchung zu #340).

## Was schon da ist (schlafend, nirgends angeschlossen)

| Baustein | Inhalt | PR |
|---|---|---|
| 1 | Zertifikat: Format, Unterschreiben, Prüfen gegen eine Komitee-Menge (> 2/3, mindestens 4, kanonisch, billig vor teuer) | #344 |
| 2 | Komitee zur Checkpoint-Zeit aus dem Register; Bindung an einen lokal nachgespielten Block; Frische; Monotonie über die Zeit | #345 |
| 3 | Vorfahren-Suche über Kettendaten; Prüfung mit Bezug auf den tragenden Block; „nicht prüfbar“ getrennt von „ungültig“ | #346 |
| – | Peer-Blöcke höchstens 256 Eltern (Vorstufe) | #343 |

## Ist das Komitee aus dem Register konsensfest?

Fast. Das Register ist so gebaut (`spaet_eingehaengt.go`,
`validator_register_leser.go`), dass ein Urteil zur Zeit t nur an Bindungen
hängt, die mindestens `erzeugerFrist` (2 h) älter sind (auch das Ende einer
Bindung wirkt 2 h später). Die Bindung steht höchstens 1 h nach ihrem
Zeitpunkt in einem Block, und ein Block mit Bindung, der über 30 min hinter
den eigenen Spitzen liegt, wird abgewiesen. Das ist eine starke Annahme,
aber kein Beweis: die Grenze misst an den lokalen Spitzen, Knoten können
einen Block genau an der Grenze verschieden behandeln, und Geschichte vom
vertrauten Seed (FromSync) ist ausgenommen.

Der Status „Mensch“ wird im Betrieb nur gesetzt; entzogen wird er nur beim
Zurücksetzen des ganzen Zustands und mit `CLEAR_REGISTRATIONS`. Das Register
liest den heutigen Status, nicht den zur Zeit t – unschädlich nur, solange
er nie entzogen wird.

Lokal bleiben vier Dinge:

1. **Die geschlossene Liste** (`AUTHORIZED_VALIDATORS`) kommt aus der
   Umgebung jedes Knotens. Verschiedene Listen ergeben verschiedene Komitees.
   → Baustein 4.
2. **Lesefehler** des Register-Stands: heute „nicht prüfbar“ (richtig);
   beim Nachspielen muss das „zurückstellen“ heißen (Baustein 7).
3. **Grenzfälle an der 30-Minuten-Grenze** für spät eingehängte Blöcke mit
   Bindung (siehe oben): beim Unterschreiben nur ein anderer Komitee-Hash
   (Ablehnung), als Konsensregel eine Spaltung. Vor Baustein 6 klären.
4. **Auffrischtakt** des Stands (30 s bzw. nach jedem Block mit Bindung):
   unschädlich, weil Bindungen erst 2 h später wirken – aber nur, solange
   der Stand nach jedem nachgespielten Block mit Bindung neu gelesen wird.
   Test dafür in Baustein 7.

## Die nächsten Bausteine, in dieser Reihenfolge

### 4. Konsensfeste Liste (Betreiber-Entscheidung nötig)

Im geschlossenen Betrieb müssen alle Knoten dieselbe Liste haben. Zwei Wege:

- **(a) Liste in der Kette:** eine Transaktion `validator_liste` mit dem
  vollständigen Satz, unterschrieben von mehr als 2/3 der bisherigen Liste
  (Start: Genesis). Wirkt wie Bindungen erst nach `erzeugerFrist`. Sauber,
  aber ein eigener Konsensweg.
- **(b) Hash der Liste im Zertifikat:** Jeder Knoten prüft, dass der Hash
  seiner Liste zum Komitee-Hash passt (schon heute so – ein anderer Stand
  lehnt ab). Ein Knoten mit falscher Liste kommt dann nicht mehr mit. Das
  ist einfach, macht aber die Umgebungsvariable zur Konsensregel.
- **Empfehlung:** (b) für den geschlossenen Betrieb mit wenigen
  Validatoren, (a) vor dem offenen Betrieb. Im offenen Betrieb entfällt die
  Liste ganz; dann braucht die Rangfolge einen Zufall, den bei der Bindung
  niemand kennt (`komitee_register.go`, „OFFENER BETRIEB“).

### 5. Wer welchen Block unterschreibt (Vorschlag durch den Leiter)

Damit ein Quorum zustande kommt, müssen die ehrlichen Mitglieder denselben
Block unterschreiben. Weil blue_score nicht konsensfest ist, wählt nicht
jeder selbst, sondern:

- Der **Leiter** (`leitung.go`, ab drei Validatoren gewählt) schlägt je Slot
  (z. B. alle 60 s) einen Block vor: einen eigenen, nachgespielten Block, der
  mindestens `zeitstempelRueckToleranz` alt ist.
- Ein **Mitglied unterschreibt** nur (umgesetzt in
  `checkpoint_unterschreiben.go`), wenn:
  - der Block bei ihm nachgespielt ist;
  - sein Slot nicht vor dem zuletzt unterschriebenen liegt;
  - es für diesen Slot noch **keinen anderen** Block unterschrieben hat
    (dauerhaft gespeichert, bevor die Unterschrift herausgeht);
  - der Block nach dem Start des eigenen Prozesses entstand (Startsperre:
    schützt auch, wenn der Merker verloren geht – neue Datenbank,
    Snapshot, Restore).
- **Doppelsignieren** ist festgelegt als: gleiche Kette, gleicher Slot
  (Slot-Länge steht im unterschriebenen Inhalt), verschiedener Blockhash.
  „Älterer Slot nach jüngerem“ ist nicht beweisbar und nie strafbar.
- **Noch offen** (Prüfung von Baustein 5):
  - Vorschläge vom Leiter unterschreiben lassen und prüfen – sonst
    verbraucht jeder, der die Funktion erreicht, den Slot zuerst.
  - Ein böswilliger Leiter kann Slots verbrauchen (jedem Mitglied einen
    anderen Block schicken): Leiterwechsel nach k Slots ohne Zertifikat,
    zwei Vorschläge desselben Leiters im selben Slot als Beweis sammeln.
  - Der vorgeschlagene Block muss den letzten zertifizierten Checkpoint in
    seiner Vergangenheit haben (sonst bilden Checkpoints keine Kette) und
    in der Vergangenheit der eigenen Spitzen liegen.
  - Betrieb: nie zwei Prozesse mit demselben Signierschlüssel zugleich.
- Der Leiter sammelt die Unterschriften (signierte Nachrichten über die
  Leitungswege, je Absender eine, begrenzt) und legt das Zertifikat bei
  Quorum als Transaktion in einen Block.

### 6. Zertifikat in die Kette, „letzter Checkpoint“ im Zustand

- Transaktionsart `checkpoint_zertifikat`. Gültig nach
  `checkpointImTragendenBlock` (Baustein 3) – mit dem Komitee aus Baustein 4.
- Der **letzte Checkpoint** ist Zustand (geht in die StateRoot): der jüngste
  gültige Checkpoint in der Vergangenheit des tragenden Blocks in der
  Nachspiel-Reihenfolge – nicht der lokal zuletzt gesehene.
- Höchstens ein Zertifikat je Block.

### 7. Nachspielen: „nicht prüfbar“ heißt zurückstellen

- `errCheckpointNichtPruefbar` beim Nachspielen: Block zurückstellen
  (wie ein Waise), nachladen, erneut versuchen – nie ablehnen, nie annehmen.
- Suche in derselben Datenbank-Transaktion wie das Nachspielen und außerhalb
  von `dag.mu` (die Daten sind inhaltsadressiert).
- Breiter Kegel: der Checkpoint-Block muss höchstens `vorfahrEbenenGrenze`
  Ebenen zurückliegen; der Vorschlag (Baustein 5) nimmt darum frische Blöcke.

### 8. Snapshot

Der Snapshot trägt den letzten Checkpoint und die Köpfe der letzten 720 s,
sonst ist ein frisch aufgesetzter Knoten bis dahin „nicht prüfbar“.

### Danach

- **Anker der Validatoren-Belohnung (#340):** der jüngste Checkpoint mit
  Zeit ≤ D in der Vergangenheit des tragenden Blocks – kein Erzeuger kann
  ihn mehr wählen.
- **Finalität:** Blöcke in der Vergangenheit eines Checkpoints sind final;
  `isFinalityViolation` misst dann daran statt an lokaler Höhe.
- **Ratengrenze je Erzeuger (OFFEN 10, Stufe A/B):** mit Checkpoints
  weniger dringend, bleibt aber gegen das Aufblähen sinnvoll.

## Was ich vom Betreiber brauche

1. Entscheidung Baustein 4: (b) Liste per Hash – oder gleich (a) in der Kette.
2. Slot-Länge für Checkpoints (Vorschlag 60 s).
3. Ab wann mehr als ein Validator laufen soll (C2 ist vorbereitet, aber aus;
   Checkpoints brauchen mindestens 4 Validatoren).
