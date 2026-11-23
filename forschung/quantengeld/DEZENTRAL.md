# Dezentrales Quantengeld komplett: was fehlt, und der Weg dorthin

Stand 18.11.2026, Abschnitt 1 aktualisiert und Abschnitt 17 am 20.11.2026, R8 und R9 am 21.11.2026, R10 am 22.11.2026. Auftrag des Betreibers: „Bring das dezentrale Quantengeld komplett zu
laufen, ggf. mit eigener Forschung“. Diese Notiz sagt, was heute läuft, wo es noch an
einer Stelle hängt, und was wir bauen.

## 1. Was heute läuft

- **Variante S im Hauptnetz** seit 17.11.2026 17:00 UTC. Ein Token wurde ausgegeben, die
  Doppelvorlage abgewiesen und die ehrliche Vorlage eingelöst (`HEUTE.md` 3.3c).
- **Mit vier Validatoren** läuft es im Pilot: 3 von 4, k = 5.200, auf IBM `ibm_kingston`
  und im Rauschmodell, auch wenn einer ausfällt und mit Netzlaufzeit (`HEUTE.md` 3.3d).
- **Ausgabe in Abschnitten (Weg A, A1–A4):** Jeder Validator präpariert seinen Abschnitt
  in einem eigenen Auftrag mit eigener Messquelle. Im Pilot grün im Rauschmodell und auf
  IBM (`ibm_marrakesh`, `ibm_fez`), auch mit Ausfall eines Validators (`HEUTE.md` 3.3e, 12).
- **Jeder Knoten mit Signierschlüssel** startet den Validator-Dienst von selbst
  (`StarteQuantenWDienst`, `cmd/aequitasd/main.go`). Ein neuer Validator macht ohne weitere
  Einrichtung mit.
- **Die Prüfung ist dezentral:** Jeder Knoten wertet jede Offenlegung selbst aus, und ein
  falscher Anteil wird als Ganzes abgewiesen (`QUANTENGELD_PRUEFAUFTRAG.md`, W8).
- **Stand 20.11.2026:**
  - **Messfehler:** Mehrheit aus 5 Schüssen, auf dem Chip 0 Fehler bei passender Basis
    (Abschnitt 4).
  - **Toleranz:** 2 % statt 4 %, damit ein Drittel der Qubits; die ehrliche Vorlage wird
    vor der Ausgabe exakt geprüft (`HEUTE.md` 3.3f, 3.3g).
  - **Basen automatisch zum Validator** (Abschnitt 15).
  - **Befund 5i und sein Ausweg „Stichprobe v2“** (Abschnitt 16):
    - Im Hauptnetz scharf seit 19.11.2026 19:30 UTC.
    - Erster Token im Hauptnetz damit am 20.11.2026 auf `ibm_kingston`: Doppelvorlage
      abgewiesen, ehrliche Vorlage eingelöst (`HEUTE.md` 3.3i).
    - C1 hat dafür ein Messverzeichnis (Abschnitt 16, Schritt 7).
  - **Getrennte Rollen** (Abschnitt 17, R1–R3): Inhaberin, Ausgeber und Annahmestelle
    haben je einen eigenen Befehl und eigenen Schlüssel. Der Ausgeber sieht x nie.
    Inhaberin und Annahmestelle prüfen gegen die Kette. Auf vier echten Knoten grün,
    auch mit Ausfall eines Validators; im Hauptnetz noch nicht gelaufen.
  - **Basen y je Validator verschlüsselt** (Abschnitt 17, R8, 21.11.2026): Die
    Inhaberin kann y an die Validatoren verschlüsseln; der Ausgeber sieht dann nicht
    mehr, wo sie einlösen will.
  - **Öffnung aus t Anteilen** (Abschnitt 17, R9, 21.11.2026): Fällt ein präparierender
    Validator nach der Bindung aus, lädt die Annahmestelle seinen Abschnitt aus t
    Paketen der anderen. Wirkt erst ab mehreren Validatoren.
  - **Was am Betreiber hängt:**
    - Der zweite Mensch als Validator mit eigenem Server und IBM-Konto (A5). Erst dann
      kennt niemand den ganzen Token.
    - Ein Gutachten.
    - Ein Partner mit eigenem Empfänger für die Inhaberin (Weg C).

## 2. Befund: Die Ausgabe hängt an einer Stelle

Die Prüfgröße k kommt aus der Tabelle (`quantenWKTabelle`, `wiesner_parameter.py`). Sie
setzt voraus, dass eine Absprache von weniger als t Validatoren **höchstens den Anteil
f = (t−1)/v der Stellen** kennt. So steht es auch in der Spezifikation (`s_kette.py`):
„(u, t) liegen als Anteile bei den Validatoren (Fern-Präparation wie W3)“. Jeder Validator
kennt also nur die Stellen, die er selbst präpariert hat.

**Die heutige Ausgabe erfüllt das nicht.**
- `hardware_ibm.py --s-messen` erzeugt Basen u und Werte t **aller** n Stellen in einem
  einzigen IBM-Auftrag. Darin misst auch die Inhaberin (Basen y, Ergebnisse x).
- `quanten_s_ausgabe.go` liest diese Datei und teilt das Geheimnis danach auf die
  Validatoren auf.
- Damit kennen **der Ausgeber** (wer den Auftrag absetzt und die Datei hat) und **IBM** den
  ganzen Token.

Folge: Wie viele Validatoren es gibt, ändert nichts daran, dass der Ausgeber mit einer
Inhaberin fälschen könnte. Sie könnte an beiden Annahmestellen durchkommen, wenn er ihr
u und t verrät. Die Validatoren schützen das Geheimnis erst **nach** der Ausgabe.

Beim ersten Token im Hauptnetz war das ohnehin so, denn C1 war Ausgeber und einziger
Validator (`HEUTE.md` 3.3c benennt es). Mit mehreren Validatoren wäre es aber eine stille
Lücke: Die Anzeige „3 von 4“ verspräche mehr, als die Ausgabe hält.

## 3. Weg A: Präparation je Validator (bauen wir jetzt)

**Idee:** Jeder Validator präpariert seinen **eigenen Abschnitt** der Stellen in einem
**eigenen** Auftrag beim Quantenrechner, mit eigenem Zufall. Das Geheimnis seiner Stellen
teilt er selbst auf alle Validatoren auf. Dann kennt niemand mehr den ganzen Token, und
die Annahme der Tabelle wird wahr.

**Ablauf (Entwurf, Einzelheiten in der Spezifikation A1):**
1. Der Ausgeber legt K, n, t, die v Validatoren, die Toleranz und die beiden
   Annahmestellen fest. Die Stellen 0 … n−1 werden in v Abschnitte geteilt.
2. Die Inhaberin wählt ihre Messbasen y geheim und schickt jedem Validator nur y auf dessen
   Abschnitt.
3. Validator j präpariert seinen Abschnitt: zufällige Basen u und Werte t, die Messung der
   Inhaberin in y, Ergebnisse x.
   - Danach zieht er je Stelle ein Salz und rechnet die Prüfsumme (Blatt) aus.
   - Den Wert jeder Stelle teilt er nach Shamir t-von-v auf alle Validatoren auf und
     schickt jedem dessen Anteile, unterschrieben und verschlüsselt.
   - Der Inhaberin gibt er x auf seinem Abschnitt, dem Ausgeber die Blätter.
4. Der Ausgeber baut aus allen Blättern die Merkle-Wurzel.
5. **Erst nach der Messung** ziehen die Annahmestellen ihre geheimen Stichproben. Je
   Abschnitt holen sie sich (u, t) und die Pfade beim zuständigen Validator und prüfen sie
   gegen die Wurzel (`stelle_laden`, wie heute).
6. Jeder Validator prüft die Blätter gegen die Wurzel, hält Anteile aller Stellen und
   bestätigt. Danach folgt die Ausgabe auf der Kette.
7. **Einlösen bleibt unverändert:** Offenlegung mit t Anteilen je Stelle, jeder Knoten
   prüft. **Die Kette selbst ändert sich nicht.** Wurzel, Anteile, Bestätigungen und
   Offenlegung haben dieselbe Form wie heute.

**Wer was weiß:**

| Rolle | weiß | weiß nicht |
|---|---|---|
| Validator j | u, t, y, x auf seinem Abschnitt (Anteil 1/v); Anteile aller Stellen | u, t der anderen Abschnitte (weniger als t Anteile) |
| m < t Validatoren gemeinsam | m/v ≤ (t−1)/v = f der Stellen | den Rest: genau die Annahme der Tabelle |
| Ausgeber | Blätter, Wurzel | u, t, x |
| Inhaberin | y, x | u, t |
| Annahmestelle b | (u, t) auf ihrer Stichprobe | die andere Stichprobe |
| Anbieter des Quantenrechners | alles, was über ihn läuft | Abschnitte bei anderen Anbietern |

**Was es kostet** (Toleranz 4 %, k = `quantenWKTabelle[4][f16]`, f16 = ⌈16 (t−1)/v⌉,
n = 2k):

| v | t | f | k je Stelle | n Qubits | je Validator |
|---|---|---|---|---|---|
| 1 | 1 | 0 | 840 | 1.680 | 1.680 |
| 4 | 3 | 0,50 | 5.200 | 10.400 | 2.600 |
| 4 | 2 | 0,25 | 1.576 | 3.152 | 788 |
| 7 | 3 | 0,29 | 1.960 | 3.920 | 560 |
| 10 | 4 | 0,30 | 1.960 | 3.920 | 392 |
| 16 | 5 | 0,25 | 1.576 | 3.152 | 197 |

Mit mehr Validatoren wird es je Kopf billiger: Bei 10 Validatoren mit Schwelle 4 sind es
392 Qubits je Validator und Token. Dazu kommt eine Abwägung bei t: Ein großes t schützt das
Geheimnis besser, aber zum Einlösen müssen t Validatoren erreichbar sein.

**Neue Missbrauchsfälle, die die Spezifikation abdecken muss:**
- **Ein Validator meldet der Inhaberin falsche x.** Er kann eine ehrliche Inhaberin damit
  durchfallen lassen, also stören; fälschen kann er so nicht. Gegenmittel ist zu prüfen,
  etwa eine Probe der Inhaberin auf einem Teil ihres Abschnitts vor der Bindung.
- **Ein Validator verteilt widersprüchliche Anteile.** Das fällt erst beim Einlösen auf. Es
  ist dieselbe Frage wie W4 im Prüfauftrag; geprüfte Anteile nach Feldman oder Pedersen
  wären eine Lösung.
- **Ein Validator verschweigt seinen Abschnitt** und bestätigt nicht: Dann gibt es keine
  Ausgabe. Das stört, schadet aber niemandem.

## 4. Was danach noch an Einzelnen hängt

- **Der Anbieter des Quantenrechners** sieht jeden Auftrag, der über ihn läuft. Nutzen alle
  Validatoren IBM, kennt IBM den ganzen Token. Abhilfe: verschiedene Anbieter je Validator,
  mit Weg A ohne Änderung an der Kette. Ganz weg ist es erst mit eigenem Gerät (Weg C).
- **Die Messung der Inhaberin** läuft im Auftrag des Validators, denn eine Quantenleitung
  zu ihr gibt es nicht. Sie muss ihm x glauben (siehe oben). Mit eigenem Empfänger (Weg C)
  misst sie selbst.
  - Weil Präparation und Messung im selben Auftrag laufen, darf jede Stelle mehrmals
    präpariert und in derselben Basis y gemessen werden; die Mehrheit senkt den
    Auslesefehler, ohne über t mehr zu verraten (`HEUTE.md` 3.3f, `--wiederholen`).
- **Menschen:** Jeder Validator muss an einen anderen Menschen gebunden sein. Heute gibt
  es einen. Weg A wirkt im Hauptnetz, sobald ein zweiter Mensch einen Validator mit eigenem
  Konto bei einem Quantenrechner betreibt. Das kann nur der Betreiber anstoßen.

## 5. Weitere Wege (Forschung)

- **Weg B, Präparatoren aus dem Menschenregister:**
  - Jeder registrierte Mensch mit einem Konto bei einem Quantenrechner kann einen Abschnitt
    präparieren und Anteile halten, ohne eigenen Server.
  - Die Kette zieht die v Beteiligten per Zufall, damit der Ausgeber keine Komplizen wählen
    kann. Sybil-Schutz liefert die Registrierung: ein Mensch, eine Stimme.
  - Zu klären: Erreichbarkeit beim Einlösen (Handys) und die Auswahl als Konsensregel.
- **Weg C, eigenes Gerät:** QKD-Sender bei den Validatoren und Empfänger bei der Inhaberin
  (S3, `ANALYSE.md` 5c). Dann sieht kein Anbieter mehr etwas. Das braucht Partner und
  Geräte (`ANSCHREIBEN.md`, Vorlage 4).
- **Härtung bei W:** Zurückhalten durch v − t + 1 Validatoren kann die letzte Einlösung
  einer W-Note zunichtemachen (Prüfauftrag W8). Den Zufall braucht es heute von allen v
  (W3). Beides wird wichtig, sobald es mehrere Validatoren gibt. Für S stellt es sich so
  nicht.

## 6. Bauabschnitte

| Abschnitt | Was | Wer |
|---|---|---|
| A1 | Spezifikation in `s_kette.py`: Ausgabe in Abschnitten, wer was weiß, Missbrauchsfälle | **fertig** (siehe unten) |
| A2 | Validator-Dienst: eigenen Abschnitt präparieren (Messung von außen, wie heute über eine Datei), Anteile an die anderen verteilen, Stichprobe für die Annahmestellen | **fertig**: A2a Bausteine (9), A2b Dienst (10) |
| A3 | `hardware_ibm.py --s-abschnitt`, Werkzeug `quanten-s-ausgabe` im Abschnitts-Modus | **Werkzeug fertig** (11); Messung je Abschnitt folgt mit A4 |
| A4 | Pilot: vier Knoten, je ein eigener Auftrag (im Pilot von einem Konto, das wird benannt) | **fertig** (12): beide Tests grün im Rauschmodell (44347416357) und auf dem Chip `ibm_fez` (44356741223), auch mit Ausfall eines Validators |
| A5 | Hauptnetz mit einem zweiten Menschen als Validator, wenn möglich bei einem anderen Anbieter | **Werkzeug bereit** (13–15): Messquelle als Verzeichnis, `--s-abschnitt`, Einrichten mit einem Befehl, Basen automatisch vorgemerkt und gemessen; der zweite Mensch: Betreiber |
| B1 | Weg B, Spezifikation: Präparatoren-Register, Los, Verfügbarkeit, Kontingent, Missbrauch (`s_weg_b.py`) | **fertig** (18) |
| B2–B5 | Weg B: Kette (Register, Zufall, Los), Dienst/Briefkasten, App als Präparatorin, Pilot | B2a–B2d gebaut, schlafend; B3 und B4a gebaut (18); B4b, Stichtag, B5 offen |
| C, W-Härtung | wie oben | Forschung; Partner |

Die Kette ändert sich in A1 bis A4 nicht. Geändert werden die Ausgabe außerhalb der Kette
und der Dienst der Validatoren. Jede Änderung am Dienst bekommt eine eigene
Sicherheitsprüfung.

## 7. A1: die Spezifikation (fertig, 18.11.2026)

In `s_kette.py` stehen `abschnitte`, `Praeparator`, `AusgabeAbschnitte` und
`fehler_je_abschnitt`. Der Selbsttest (Abschnitt 5) zeigt:

- **Gutfall 3 von 4:** Die Annahmestellen laden ihre Stichproben dezentral aus Anteilen von
  je drei Validatoren und prüfen sie gegen die Wurzel. Die ehrliche Vorlage besteht, die
  Doppelvorlage fällt durch.
- **Wer was weiß:**
  - Jede Absprache von t − 1 Validatoren kennt genau ihre eigenen Abschnitte, also den
    Anteil (t − 1)/v. Das ist die Annahme der Tabelle.
  - Ab t Validatoren lässt sich alles zusammensetzen; t − 1 Anteile ergeben den Wert nicht.
- **Missbrauch in voller Größe** (k = 5.200, 3 von 4, vier verschiedene Absprachen):
  - Zwei Validatoren verraten der Inhaberin ihre Abschnitte, den Rest misst sie nach
    Breidbart. Sie kommt an keiner zweiten Stelle durch.
  - **Gegenprobe:** Wer den ganzen Token kennt, wie bei der Ausgabe mit einem Auftrag,
    kommt an beiden Stellen durch. Genau diese Lücke schließen die Abschnitte.
- **Störung zuordenbar:** Meldet ein Validator der ehrlichen Inhaberin falsche x, fällt sie
  durch. Die Fehler liegen dann alle in seinem Abschnitt (`fehler_je_abschnitt`).
- **Fail-closed:** Es gibt keine zweite Messung und keine Basen für fremde Stellen, die
  Zahl der Basen muss stimmen, und jeder Validator braucht seinen eigenen Zufall.
- **Gegenprobe der Tests:** 5 Mutationen (Wissen schon ab t − 1, zweite Messung erlaubt,
  gleicher Zufall, Zuordnung über die falsche Basis, Angriff mit ganzem Wissen) machen
  den Test rot.

Die Kette bleibt unverändert. Der Prüfvektor für den Knoten (`vektor()`) ist derselbe,
weil Wurzel, Anteile und Offenlegung dieselbe Form haben.

## 8. A2: Entwurf für den Dienst, mit zwei Befunden (18.11.2026)

### Befund: Die Stichproben müssen bis zur Bindung geheim bleiben
Kennt die Inhaberin **beide** Stichproben, bevor sie ihre Basen bindet, setzt sie d so, dass
an jeder Stelle genau ihre gemessenen Basen geprüft werden:
- auf S₀ gilt dᵢ = yᵢ ⊕ c,
- auf S₁ gilt dᵢ = yᵢ ⊕ 1 ⊕ c.

Dann kommt sie an **beiden** Stellen durch, ohne irgendein Geheimnis zu kennen (Selbsttest
5g). Heute zieht der Ausgeber beide Stichproben, kennt sie also, und muss deshalb
vertrauenswürdig sein. Dezentral braucht es einen anderen Weg.

### Lösung: das Stellen-Bit
- Jeder Präparator legt in seinem Abschnitt je Stelle fest, zu welcher Annahmestelle sie
  gehört, je genau zur Hälfte.
- Das Bit steht als unterstes Bit im Salz des gesalzenen Werts (`stellen_bit`). Die
  Prüfsumme bindet es also, und für die Kette ist es Salz; sie ändert sich nicht. Das Salz
  hat danach 123 statt 124 Bit Zufall.
- Annahmestelle b bekommt von jedem Präparator direkt nur die Werte mit Bit b
  (`Praeparator.oeffnung`) und prüft selbst (`stelle_laden_bit`):
  - die Blätter ergeben die Wurzel,
  - jeder Wert passt zu seinem Blatt,
  - sein Bit ist b,
  - es sind mindestens k_note Stellen.
- Damit sind die Stichproben **durch die Blätter disjunkt**, und niemand kennt beide: Jeder
  Präparator kennt nur seinen Abschnitt.
- Getestet (5h):
  - Gutfall und Doppelvorlage.
  - Abgewiesen werden eine untergeschobene Stelle der anderen Annahmestelle, ein falscher
    Wert, zurückgehaltene Werte und eine fremde Wurzel.
  - In voller Größe kommen t − 1 Präparatoren zusammen mit der Inhaberin an keiner zweiten
    Stelle durch.
- Gegenprobe der Tests: 5 Mutationen rot (Bit nicht geprüft, Wert nicht gegen das Blatt,
  nicht ausgewogen, Öffnung gibt alles, Wurzel nicht geprüft).

### Der Entwurf für den Dienst
1. **Auftrag** (Ausgeber an Validator j, unterschrieben wie heute `bestaetigen`): K, n, t,
   Toleranz, die Validatoren mit ihren öffentlichen Schlüsseln (jeder prüft, dass Schlüssel
   und Adresse zusammenpassen), Abschnitt j, die Basen der Inhaberin auf diesem Abschnitt
   und ihr öffentlicher Schlüssel.
2. **Messung:** Der Dienst holt sie von seiner **eigenen** Messquelle. Das ist eine Datei
   von `hardware_ibm.py --s-abschnitt` oder später ein Dienst neben dem Knoten mit dem
   eigenen Konto beim Quantenrechner. Sie kommt nie über die öffentliche API.
3. **Antwort an den Ausgeber**, vom Validator unterschrieben:
   - die Blätter seines Abschnitts;
   - x **verschlüsselt für die Inhaberin**;
   - je anderer Validator ein Bündel seiner Anteile, **verschlüsselt für diesen**
     (ECIES an den secp256k1-Schlüssel, wie in go-ethereum). Der Ausgeber leitet nur
     weiter und kann nichts lesen.
4. **Bestätigung** (Ausgeber an jeden Validator):
   - Mit dabei sind alle Blätter und die für ihn bestimmten Bündel.
   - Der Validator entschlüsselt, prüft jede Unterschrift des Präparators und setzt seine
     Anteile aller Stellen zusammen.
   - Er prüft Blätter gegen Wurzel und unterschreibt wie heute.
5. **Stichproben:** Annahmestelle b holt sich bei jedem Validator die Öffnung für Bit b,
   mit Unterschrift der Stelle, nur einmal je Token, und nur von der Stelle, die im Auftrag
   steht. Den Hash ihrer Stichprobe gibt sie dem Ausgeber für die Ausgabe auf der Kette.

**Neue Eingänge und ihre Grenzen** (Missbrauchstests im Bau):
- Auftrag: nur von einem Menschen mit Deckung, wie heute.
- Bündel: Größe begrenzt (n ≤ 16.384 Anteile), nur vom Präparator unterschrieben, nur für
  diesen Empfänger, nur zu einem offenen Auftrag.
- Öffnung: nur an die Stelle aus dem Auftrag, einmal, vor der Ausgabe auf der Kette.
- Wiederholung desselben Auftrags liefert dieselbe Antwort; ein anderer Auftrag für
  dasselbe K wird abgewiesen.

## 9. A2a: die Bausteine in Go (18.11.2026)

Die Datei `x/humanity/keeper/quanten_s_abschnitt.go` enthält nur reine Funktionen ohne
Netz:
- `quantenSAbschnitt` (Grenzen) und `quantenSStellenBit`;
- `quantenSAbschnittWerte`: gesalzene Werte mit Stellen-Bit, je Abschnitt genau zur Hälfte
  verteilt, gemischt mit kryptographischem Zufall;
- `quantenSTeilen` (Shamir t-von-v) und `quantenSSchluesselPruefen` (der öffentliche
  Schlüssel muss zur Adresse passen);
- `quantenSBuendelVersiegeln` und `quantenSBuendelOeffnen`: ECIES aus go-ethereum, also
  keine neue Abhängigkeit, dazu die Unterschrift des Präparators über das Chiffrat;
- `quantenSOeffnungLaden`: das Laden bei der Annahmestelle.

**Prüfvektor:** Er kommt aus der Spezifikation (`s_kette.py --vektor-abschnitte`,
`testdata/quanten_s_abschnitt_vektor.json`), und das CI vergleicht Byte für Byte. Grenzen,
Stellen-Bit, Blätter, Wurzel und Stichproben stimmen mit Python überein.

**Missbrauchstests:**
- *Bündel:* Abgewiesen werden ein anderer Empfänger, ein getauschtes Chiffrat, eine fremde
  Unterschrift, ein anderer Präparator, ein anderes K, ein anderer Abschnitt, eine andere
  Empfängernummer, Übergröße, Großbuchstaben, ein unbekanntes Feld, ein Anteil ≥ P, zu
  wenige Anteile, eine fremde Domäne und zwei Objekte. Ein Dritter kann nicht
  entschlüsseln.
- *Öffnung:* Abgewiesen werden die Stelle der anderen Annahmestelle, ein falscher Wert,
  ein Wert über der Grenze, eine Stelle außerhalb, ein fehlender Wert, eine zu kleine
  Stichprobe, eine fremde Wurzel, getauschte Blätter, Stelle 2, ein anderes K und die
  Mindestgröße 0.
- *Schlüssel:* Abgewiesen werden eine fremde Adresse, eine zu kurze Form, kein Hex und ein
  Wert, der kein Punkt der Kurve ist.

**Gegenprobe:** 7 Mutationen machen die Tests rot (Bit, Wert gegen Blatt, Unterschrift,
Abschnitt, Schlüssel-Adresse, Ausgewogenheit, Wurzel). `go test -race` ist für alle
Quantengeld-Tests grün.

## 10. A2b: der Dienst (18.11.2026)

Die Datei `x/humanity/keeper/quanten_s_abschnitt_dienst.go` bringt zwei neue Endpunkte
beim Validator und einen neuen Weg der Bestätigung.

- **`POST /api/quanten-w/validator/abschnitt`**
  - Der Auftrag kommt vom Ausgeber und ist von ihm unterschrieben; der Ausgeber muss ein
    Mensch mit Deckung sein.
  - Geprüft werden: alle Schlüssel gegen die Adressen, die eigene Nummer, die Basen der
    Inhaberin auf genau diesem Abschnitt, die zwei Annahmestellen, die Mindestgröße je
    Stelle und das Zeitfenster.
  - Die Messung kommt aus der **eigenen Messquelle** (`AEQUITAS_QS_MESSQUELLE`). Das ist
    eine Datei von `hardware_ibm.py --s-abschnitte` (Pilot) oder ein Verzeichnis mit einer
    Datei je Messung (Betrieb, Abschnitt 13). Jeder Datensatz wird vor der Verwendung
    entfernt, also nie zweimal benutzt.
  - Die Antwort ist unterschrieben und enthält: die Blätter, x verschlüsselt für die
    Inhaberin und je anderem Validator ein verschlüsseltes, unterschriebenes Bündel.
  - Der Eintrag liegt danach *wartend* im Tresor und gilt wie ein offener Auftrag, für die
    Grenzen wie fürs Aufräumen.
  - Derselbe Auftrag noch einmal ergibt dieselben Werte; ein anderer Auftrag für dasselbe K
    wird abgewiesen.
- **`GET /api/quanten-w/validator/oeffnung?k=&b=`**: die Werte des eigenen Abschnitts mit
  Stellen-Bit b, **verschlüsselt für die Annahmestelle b aus dem Auftrag**.
- **Bestätigen mit Bündeln** statt Anteilen:
  - Jedes Bündel muss vom Präparator unterschrieben und für diesen Validator bestimmt sein.
  - Die eigenen Blätter müssen gleich sein, und die Blätter müssen die Wurzel ergeben.
  - Dann folgt dieselbe Bestätigung wie bisher. Eine Bestätigung ohne Bündel für ein
    wartendes K wird abgewiesen.

**Ende-zu-Ende-Test** (`TestQuantenSAbschnittDienst_Gutfall`, 5 Validatoren, Schwelle 3):
1. Präparation in Abschnitten.
2. Die Inhaberin liest ihr x, und die Annahmestellen laden ihre Stichproben selbst; sie
   sind disjunkt und je zur Hälfte.
3. Alle bestätigen, der Token wird ausgegeben, gebunden und gewählt.
4. Das Einlösen durch den Dienst läuft **über die aus den Bündeln zusammengesetzten
   Anteile**. Die Stelle bekommt den Wert.

**Missbrauchstests:**
- *Auftrag*, 14 Fälle: fremde Unterschrift, veränderter Inhalt, fremder Schlüssel, falsche
  Nummer, Basen zu kurz oder keine Bits, Stellen gleich, Stelle gleich Inhaberin,
  Schlüssel von Stelle oder Inhaberin fremd, zu wenige Stellen, alte Zeit, doppelter
  Validator, fremder Ausgeber.
  - Dazu: ohne Messquelle, ohne passende Messung und ohne Deckung. Ein abgewiesener Auftrag
    benutzt die Messquelle nie.
- *Öffnung:* unbekanntes K, Stelle 2. Lesen kann nur die richtige Stelle, nicht die andere
  und nicht die Inhaberin. Einem anderen Validator zugeschrieben: abgewiesen.
- *Bestätigung*, 10 Fälle: Bündel vertauscht, fremd unterschrieben, an sich selbst oder
  fehlend, nicht beauftragt, Auftrag über andere Bündel, Wurzel falsch, eigenes Blatt
  anders, Anteile dazu, andere Toleranz. Der wartende Eintrag bleibt dabei unverändert;
  eine Wiederholung ergibt dieselbe Bestätigung.
- *Messdatei:* genau einmal; unbekanntes Feld, kaputte Bits, kein JSON und fehlende Datei
  werden abgewiesen.

**Gegenprobe:** 9 Mutationen rot. `go test -race` für alle Quantengeld-Tests grün.

**Grenze:** Ein Token mit sehr vielen Stellen (über etwa 14.000) passt mit allen Blättern und
Bündeln nicht mehr in den Rumpf der Bestätigung (2 MiB). Er wird dann abgewiesen
(fail-closed); die Grenze bleibt, wie sie ist.

## 11. A3: das Werkzeug im Abschnitts-Modus (18.11.2026)

`aequitasd quanten-s-ausgabe` erkennt die Eingabe. Steht dort `basen`, also die Messbasen
der Inhaberin je Abschnitt (`QuantenSBasen`), läuft der Abschnitts-Modus
(`quanten_s_ausgabe_abschnitte.go`), sonst wie bisher die Messung aller Stellen.

Im Abschnitts-Modus **sieht das Werkzeug nie Basen u und Werte t der Ausgabe**. Es spielt
Ausgeber, Inhaberin und beide Annahmestellen:
1. Es holt die Schlüssel der Validatoren (`GET /api/quanten-w/validator/schluessel`) und
   prüft jeden gegen seine Adresse. Danach schickt es jedem Validator seinen Auftrag.
2. Jede Antwort muss vom Validator unterschrieben sein und zum Abschnitt passen. x der
   Inhaberin wird entschlüsselt.
3. Aus allen Blättern entsteht die Wurzel. Jede Stelle lädt ihre Stichprobe selbst.
4. Es folgen die Bestätigungen mit den Bündeln.
5. Danach läuft alles wie beim Weg mit einem Auftrag (`quantenSAusgabeAbschluss`, nur
   herausgelöst, sonst unverändert): Ausgabe, Bindung, Wahl, Doppelvorlage, ehrliche
   Vorlage.

Die Fehlerrate der Hardware kommt aus den Stichproben der Stellen.

**Geprüft** (in-process gegen den echten Dienst, durch JSON):
- *Gutfall* für jede Stelle b und Maske z: Die Doppelvorlage wird abgewiesen, die ehrliche
  Vorlage eingelöst, der Wert kommt zurück. Beide Knoten haben dasselbe Register, und
  jeder Validator misst genau einmal.
- *Fälschung durch einen Validator* (fremder Schlüssel, fremd unterschriebene Antwort,
  vertauschte Öffnung): Das Werkzeug bricht ab, bevor etwas auf die Kette geht.
- *Unpassende Basen*: Das Werkzeug fragt keinen Validator, und niemand misst.
- *Gegenprobe:* 3 Mutationen rot.
  - Eine überlebte zunächst, weil der Validator die Mindestgröße ohnehin noch einmal prüft.
  - Danach wurde der Test geschärft: Das Werkzeug darf dann gar nicht erst ins Netz gehen.

Die Antworten von `abschnitt` und `oeffnung` dürfen bis 8 MiB groß sein
(`quantenSAbschnittAntwortMax`), alle übrigen weiter 1 MiB.

## 12. A4: der Pilot im Abschnitts-Modus (18.11.2026)

- **Messung je Abschnitt:** `hardware_ibm.py --s-abschnitte VERZEICHNIS --k K --v V
  [--ausgaben A]`.
  - Je Ausgabe und Validator setzt es einen **eigenen Auftrag** für dessen Abschnitt ab,
    in Basen y, die es für die Inhaberin zieht.
  - Es schreibt `mess_<x>.json` (u, t, y, x aller A Ausgaben, nur für Validator x) und
    `basen_<a>.json` je Ausgabe (nur y, für das Werkzeug), alle mit Rechten 0600 und erst,
    wenn alle Aufträge gemessen sind. A ist höchstens 4.
  - Der Dienst verbraucht jeden Abschnitt genau einmal. Darum braucht der Pilot zwei
    Ausgaben, eine je Test (Befund unten).
  - Im Rauschmodell geprüft: 5 Abschnitte, 5 Aufträge.
- **Vier-Knoten-Test** `TestVierKnotenQuantenSAbschnitte(Ausfall)_RealDB`:
  - Jeder Knoten bekommt **seine** Messquelle. Im Pilot ist das die Datei aus
    `AEQUITAS_QS_ABSCHNITTE`, sonst eine erzeugte.
  - Das Werkzeug läuft im Abschnitts-Modus über echtes HTTP.
  - Lokal grün, mit und ohne Ausfall eines Knotens vor der ersten Vorlage: Doppelvorlage
    abgewiesen, ehrliche Vorlage eingelöst, alle lebenden Knoten auf derselben StateRoot.
- **Workflow** `quanten-s-pilot.yml`: neue Wahl `ausgabe` (`auftrag` | `abschnitte`).
  - Im Abschnitts-Modus misst der Mess-Job vier Abschnitte in vier Aufträgen.
  - Der Pilot-Job prüft, dass genau die sechs erwarteten Dateien da sind, die Basen jeder
    Ausgabe zusammen n ergeben und jede Messquelle zwei Abschnitte hat. Danach laufen die
    Abschnitts-Tests, ohne Ausfall mit Ausgabe 1, mit Ausfall mit Ausgabe 2.
  - **Ehrlich:** Im Pilot kommen die vier Aufträge vom selben Konto und laufen im selben
    Job. Gezeigt wird der Ablauf. Dass niemand den ganzen Token kennt, gilt erst, wenn
    jeder Validator mit eigenem Konto selbst misst.

### Erste Läufe (18.11.2026)

- **Rauschmodell** (Lauf 44340478883) und **IBM `ibm_marrakesh`** (Lauf 44341842380), je
  vier eigene Aufträge: Der Test ohne Ausfall war in beiden **grün**. Die Doppelvorlage
  wurde abgewiesen, die ehrliche Vorlage eingelöst, alle vier Knoten standen gleich.
  Fehler bei passender Basis auf dem Chip 0,79 %. Einzelheiten in `HEUTE.md` 3.3e.
- **Befund:** Der Ausfall-Test scheiterte in beiden Läufen nach 15 s mit „keine Messung zu
  diesen Basen“. Er las dieselbe Messung wie der erste Test, und der Dienst gibt jeden
  Abschnitt nur einmal her. Das ist richtig so. Falsch war der Pilot, der nur eine
  Ausgabe gemessen hatte.
- **Korrektur:** `--ausgaben 2`, `basen_1.json` und `basen_2.json`, je Test eine eigene
  Ausgabe. Der Dienst und die Kette bleiben unverändert.
- **Nachlauf** (Lauf 44347416357, Rauschmodell, 8 Aufträge): **beide Tests grün**. Mit
  Ausfall eines Validators vor der ersten Vorlage: Doppelvorlage abgewiesen, ehrliche
  Vorlage eingelöst, die drei lebenden Knoten gleich.
- **Auf dem Chip** (Lauf 44356741223, IBM `ibm_fez`, 8 Aufträge, Fehler 0,52–0,68 %):
  **beide Tests grün**, auch mit Ausfall eines Validators. A4 ist damit fertig.

## 13. A5 vorbereitet: messen im Betrieb (18.11.2026)

**Befund:** Bis hier gab es die Messung nur für den Pilot. `hardware_ibm.py --s-abschnitte`
misst alle Abschnitte im Voraus von einem Konto aus, in eine Datei je Validator. Im Betrieb
geht das so nicht:
- Die Inhaberin wählt ihre Basen y erst für eine Ausgabe und schickt jedem Validator nur y
  auf seinem Abschnitt. Der Validator misst also, **während sein Dienst läuft**.
- Die Datei-Messquelle liest die Datei, löscht den benutzten Eintrag und schreibt sie neu.
  Schriebe ein Messender zur selben Zeit hinein, könnte ein gerade verbrauchter Eintrag
  zurückkommen. Dann ginge dieselbe Messung zweimal in eine Ausgabe.
- Den Befehl `--s-abschnitt`, auf den Abschnitt 10 verwies, gab es noch nicht.

**Lösung, ohne Änderung an der Kette:**
- **Messquelle als Verzeichnis** (`quanten_s_messverzeichnis.go`). Zeigt
  `AEQUITAS_QS_MESSQUELLE` auf ein Verzeichnis, liegt darin je Messung eine Datei
  `<sha256(y)>.json` mit genau einem Datensatz.
  - Der Dienst liest **nur** die Datei zu den verlangten Basen und prüft sie streng.
    Eine gewöhnliche Datei (kein Verweis) muss es sein, dieselbe wie beim Nachsehen, höchstens
    1 MiB, ohne fremde Felder und ohne Text danach. Die Basen im Inhalt müssen zum Namen
    passen, die Bits müssen stimmen.
  - Dann **entfernt** er sie und verwendet sie erst danach. Nur ein Entfernen kann
    gelingen. Die Einmaligkeit hängt am Dateisystem, nicht an einer Absprache zwischen
    Messen und Dienst.
  - Abgewiesene Dateien bleiben liegen, damit der Betreiber sie ansehen kann.
- **`hardware_ibm.py --s-abschnitt VERZEICHNIS --basen Y`:** Ein Validator misst seinen
  Abschnitt in einem eigenen Auftrag, in den Basen der Inhaberin.
  - Er legt das Ergebnis erst unter einem zufälligen Namen an (O_EXCL, 0600, auf die
    Platte geschrieben) und dann per `os.link` unter dem endgültigen Namen. Das scheitert,
    wenn es den Namen schon gibt; überschrieben wird nie.
  - Basen werden streng gelesen: nur 0 und 1, höchstens 16.384. Das Verzeichnis muss es
    geben. Gemessen wird als Benutzer des Knotens, sonst kann der Knoten die Datei nicht
    lesen und weist ab.

**Ablauf einer Ausgabe im Betrieb:**
1. Die Inhaberin wählt y und schickt jedem Validator y auf seinem Abschnitt.
2. Jeder Validator misst mit `--s-abschnitt` in sein Verzeichnis, mit seinem eigenen Konto
   und, wenn möglich, bei einem eigenen Anbieter.
3. Der Ausgeber startet `aequitasd quanten-s-ausgabe` mit den Basen (Abschnitts-Modus,
   Abschnitt 11). Fehlt eine Messung noch, weist der Validator den Auftrag ab („keine
   Messung zu diesen Basen“), und die Ausgabe beginnt nicht.

**Tests** (`quanten_s_messverzeichnis_test.go`):
- Gutfall und „genau einmal“.
- Missbrauch, 9 Fälle; die abgewiesene Datei bleibt jeweils liegen:
  - fremde Basen unter dem Namen, unbekanntes Feld, Text nach dem Datensatz;
  - kaputte Bits, zu kurz, kein JSON;
  - zu groß, Verweis auf einen gültigen Eintrag, Verzeichnis statt Datei.
- Wettlauf zweier Messquellen ohne gemeinsame Sperre, 20 Runden mit je 16 Zugriffen:
  genau einer bekommt die Messung.
- Messen während des Verbrauchs: 64 Messungen, jede genau einmal verwendet, keine kommt
  zurück.
- Name gleich in Go und Python (Vektor in beiden Selbsttests).
- Wahl von Datei oder Verzeichnis aus der Umgebung.
- Quer geprüft: Python misst (ideal) und legt ab, der Go-Dienst liest genau einmal, die
  Datei ist danach weg.

**Gegenprobe:** 7 Mutationen, alle rot:
- nicht entfernen;
- Fehler beim Entfernen übergehen;
- Basen im Inhalt nicht prüfen;
- Text danach erlauben;
- Bits nicht prüfen;
- gewöhnliche Datei und Austausch nicht prüfen (beide Schichten zusammen);
- Größe nicht prüfen (beide Stellen zusammen).

`go test -race` grün.

**Was für A5 bleibt (Betreiber):** ein zweiter Mensch, der einen Validator mit eigenem
Server betreibt, mit eigenem Konto bei einem Quantenrechner und, wenn möglich, bei einem
anderen Anbieter. Auf C1 braucht der Betrieb in Abschnitten dann ein Verzeichnis für den
Knotenbenutzer und `AEQUITAS_QS_MESSQUELLE`. Beides richtet der Betreiber mit dem Deploy
ein.

## 14. A5: Einrichten und Messen auf dem Server eines Validators (18.11.2026)

Damit ein zweiter Mensch seinen Abschnitt wirklich selbst präparieren kann, braucht er
auf seinem Server mehr als den Befehl `--s-abschnitt`: eine Python-Umgebung mit qiskit,
sein IBM-Konto, ein Messverzeichnis mit den richtigen Rechten und einen Knoten, der es
liest. Das gibt es jetzt mit je einem Befehl (Anleitung: `docs/VALIDATOR_EINRICHTEN.md`,
„Quantengeld“):

- **`deploy/validator/quanten-einrichten.sh`** (einmal, als root):
  - Messverzeichnis `/srv/aequitas-quanten`: 0700, Benutzer 10001 (der Benutzer des
    Knotens im Container); ein Verweis an dieser Stelle wird abgelehnt.
  - Python-Umgebung `/opt/aequitas-quanten/venv` mit den Paketen aus
    `hardware-anforderungen.txt` (`pip --require-hashes --no-deps`, dieselben Hashes wie
    im Pilot) und die Messskripte daneben; beides gehört root, der Knotenbenutzer darf
    nur lesen und ausführen. Sein HOME ist ein eigenes Verzeichnis (0700).
  - API-Schlüssel des eigenen IBM-Kontos einmal abgefragt (ohne Anzeige) und in
    `/etc/aequitas-quanten/ibm-token` abgelegt: Verzeichnis 0700, Datei 0600, root. Liegt
    schon einer da, bleibt er.
  - Am Ende eine Probe ohne IBM.
- **`deploy/validator/docker-compose.quanten.yml`** (Zusatzdatei): bindet das Verzeichnis
  als `/data/quanten` ein und setzt `AEQUITAS_QS_MESSQUELLE`. Ohne die Datei läuft der
  Knoten wie bisher.
- **`deploy/validator/quanten-messen.sh BASEN`** (bei jeder Ausgabe, als root):
  - Basen nur aus 0 und 1, 1 bis 16.384 Zeichen, genau ein Argument; sonst Abbruch ohne
    Messung. Die Länge wird vor dem Muster geprüft: `^[01]{1,16384}$` hätte bash bei jedem
    Aufruf zu einem großen Automaten übersetzt (1,5 s, rund 1 GB Speicher).
  - Misst als Knotenbenutzer (`setpriv --reuid 10001 --regid 10001 --clear-groups`), damit
    der Knoten die Datei lesen kann; je Stelle 5 Schüsse mit Mehrheit.
  - Der Schlüssel geht nur über die Umgebung an die Messung, nie auf die Befehlszeile
    (dort sähe ihn jeder Benutzer in `ps`) und nie ins Log.
  - `--probe`: Selbsttest und eine ideale Messung als Knotenbenutzer in ein
    Probeverzeichnis; prüft Besitzer und Rechte des Messverzeichnisses. Das
    Probeverzeichnis entsteht in einem Verzeichnis, das root gehört (aus der eigenen
    Sicherheitsprüfung: im HOME des Knotenbenutzers hätte der es zwischen `mktemp` und
    `chown` gegen einen Verweis tauschen können).

**Geprüft in der Arbeitsumgebung, als echter root mit Benutzerwechsel:** Einrichten in
16 s, Probe bestanden, Schlüsseldatei root 0600, Messdatei Benutzer 10001 0600; der Go-Dienst
als Benutzer 10001 hat sie gelesen und entfernt, eine zweite Verwendung wurde abgewiesen;
gleiche Basen überschreiben nicht; falsche Basen ohne Messung abgewiesen.

**Test** `scripts/test_quanten_messen_skript.sh` (in CI), mit gefälschtem `id`, `setpriv` und
Python, ohne root und ohne IBM:
- Gutfall: genau der erwartete Aufruf, als 10001 ohne Zusatzgruppen, eigenes HOME,
  Schlüssel in der Umgebung, aber nicht in Argumenten, Ausgabe oder Fehlerausgabe;
  16.384 Stellen gehen.
- Missbrauch, jeweils Abbruch **ohne** Messung: leer, `012`, Leerzeichen, `;id`, `'`,
  `$(id)`, führendes `-`, 16.385 Stellen, zwei Argumente, nicht root, kein Schlüssel,
  leerer Schlüssel, nicht eingerichtet.
- Gegenprobe: 10 Mutationen, alle rot (Längengrenze, nur Anfang oder nur Ende geprüft,
  ohne Benutzerwechsel, ohne `--clear-groups`, ohne root-Prüfung, leerer Schlüssel erlaubt,
  mehrere Argumente erlaubt, Schlüssel auf der Befehlszeile, ohne eigenes HOME).

**Nebenbefund aus dem Chip-Lauf mit 2 % (HEUTE.md 3.3g):** Jeder Auftrag wählt sein Gerät
selbst; die Messung nannte nur das Gerät des letzten. Jetzt steht es in jeder Zeile.

**Was weiter am Betreiber hängt:** der zweite Mensch mit eigenem Server und eigenem
IBM-Konto. Wie die Inhaberin ihm die Basen schickt, war hier noch Handarbeit; seit
Abschnitt 15 geht es automatisch.

## 15. A5: die Basen kommen automatisch zum Validator (19.11.2026)

**Befund:** Die Messung braucht Minuten (Warteschlange beim Anbieter) und läuft auf dem
Server des Validators, nicht im Knoten. Der Abschnitts-Auftrag muss sie fertig vorfinden.
Bisher musste die Inhaberin dem Betreiber y von Hand schicken, und er musste von Hand
messen. Bei mehreren Validatoren und jeder Ausgabe geht das nicht.

**Lösung, ohne Änderung an der Kette** (`quanten_s_vormerken.go`):
1. **Vormerken** (`POST /api/quanten-w/validator/vormerken`):
   - Der Ausgeber schickt y für genau den Abschnitt dieses Validators, unterschrieben
     (eigene Domäne `aequitas-quanten-s-vormerkung-v1`, Kette, n, Nummer,
     Validatorenliste, Hash von y, Zeit).
   - Der Knoten prüft wie beim Auftrag: Nummer gehört zu ihm, Länge und Bits,
     Unterschrift, Mensch mit Deckung. Die Zeit darf höchstens 10 min alt sein.
   - Er legt `<Messverzeichnis>/vorgemerkt/<sha256(y)>.basen` neu an (Zufallsname,
     O_EXCL, 0600, fsync, dann Link; nie überschrieben).
   - Antwort `vorgemerkt`, `wartet` oder `gemessen`. Wiederholen zählt nicht.
   - Grenzen unter derselben Sperre wie das Anlegen: 8 wartende, 8 je Ausgeber und Tag,
     32 am Tag. Mehr als 32 Einträge im Verzeichnis gelten als voll (fail-closed).
   - Dieselben Basen höchstens einmal am Tag. Eine mitgelesene und wiederholte
     Vormerkung löst nach dem Verbrauch keine zweite Messung aus (aus der eigenen
     Sicherheitsprüfung). Nach einem Neustart des Knotens ginge es einmal mehr, dann
     greift die Tagesgrenze des Servers.
   - Ein Verweis statt Verzeichnis oder statt Messung wird abgewiesen.
   - Nur mit einer Messquelle als Verzeichnis; im Pilot (Datei) abgewiesen.
2. **Messen auf dem Server** (`quanten-messen.sh --warteschlange`, mit
   `quanten-einrichten.sh --automatisch` jede Minute per systemd-Timer):
   - Gemessen wird als Knotenbenutzer, nie zwei Läufe zugleich (`flock`).
   - `hardware_ibm.py --s-warteschlange` liest jede Vormerkung als feindlich:
     - nur eine gewöhnliche Datei, beim Öffnen dieselbe (`O_NOFOLLOW`, Gerät und
       Inode); eine FIFO würde sonst die Warteschlange blockieren;
     - höchstens 64 KiB, genau drei Felder;
     - der Name passt zu y, Ausgeber und Zeit sind wohlgeformt.
   - Abgewiesene werden zu `.abgewiesen-<name>` (bleiben zum Ansehen liegen).
     Veraltete (über 6 h), solche aus der Zukunft (über 1 h) und schon gemessene werden
     nicht gemessen.
   - Die älteste zuerst, höchstens 4 je Lauf und `QS_JE_TAG` (Standard 8) am Tag.
   - Der Tageszähler liegt im HOME des Knotenbenutzers, außerhalb des Verzeichnisses,
     das der Knoten beschreiben kann. Ein übernommener Knoten kann ihn also nicht
     zurücksetzen. Gezählt wird vor der Messung; ein kaputter Zähler bricht ab.
3. **Warten im Werkzeug** (`aequitasd quanten-s-ausgabe --messen-warten DAUER`,
   höchstens 4 h):
   - Ohne Basen in der Eingabe (`{"n": …, "basen": []}`) zieht das Werkzeug y selbst; es
     spielt die Inhaberin.
   - Es merkt bei jedem Validator vor und fragt alle 20 s nach, bis jeder `gemessen`
     meldet. Erst dann schickt es die Abschnitts-Aufträge.
   - Eine Abweisung bricht sofort ab; an der Frist bricht es ebenfalls ab. In beiden
     Fällen ist nichts ausgegeben.
   - Der Stand ist eine Auskunft, keine Zusage. Meldet ein Validator `gemessen` ohne
     Messung, weist er danach den Auftrag ab.

**Tests:**
- Go (`quanten_s_vormerken_test.go`):
  - Gutfall mit Inhalt und Rechten, Wiederholung, `gemessen`; nach dem Verbrauch
    dieselben Basen erst am nächsten Tag wieder.
  - 19 Missbrauchsfälle, dazu ohne Deckung, ohne Messquelle und mit Datei-Messquelle;
    danach liegt nichts.
  - Grenzen: wartende, je Ausgeber, je Tag, Tageswechsel.
  - Verzeichnis: Verweis, Datei, Verweis als Messung, Verweis als Vormerkung, zu
    viele Einträge.
  - Gleichzeitig: dieselben Basen genau einmal, verschiedene genau bis zur Grenze.
  - HTTP (Methode, kein JSON, fremdes Feld, Text danach, zu groß, falscher Typ, fremde
    Nummer, geänderte Basen).
  - Werkzeug: Gutfall, Frist, Abweisung sofort, fremde Antwort, Abbruch, Eingaben.
  - Gegen den echten Dienst: vormerken, messen, warten, danach nimmt jeder Validator
    den Auftrag an.
- Python (Selbsttest): Gutfall und 10 Missbrauchsfälle, darunter Verweis und FIFO,
  veraltet, schon gemessen, je Lauf, je Tag, Tageswechsel, älteste zuerst, kaputter
  Zähler, Verweis statt Verzeichnis.
- Shell (`test_quanten_messen_skript.sh`): Aufruf als Knotenbenutzer, Zähler im HOME,
  `QS_JE_TAG` (7 falsche Werte), Argumente, root, Schlüssel; ein zweiter Lauf misst
  nicht, solange die Sperre gehalten ist.
- Quer geprüft: Der Go-Knoten merkt vor, Python misst ideal aus der Warteschlange, Go
  liest die Messung genau einmal.
- Gegenprobe: 36 Mutationen.
  - 35 wurden rot, zwei davon erst nach der Schärfung der Tests: Abweisung sofort, FIFO.
  - Eine ist gleichwertig: Zählen auch, wenn der Link an einem schon vorhandenen Namen
    scheitert. Unter der Sperre erreicht das nur ein fremder Prozess zwischen Prüfung
    und Anlegen, und dann ist es harmlos.
- `go test -race` grün.

**Was bleibt:** Vormerken setzt voraus, dass der Ausgeber die Validatoren über ihre API
erreicht (wie beim Auftrag). Die App spielt die Inhaberin noch nicht; das Werkzeug zieht
y selbst.

## 16. Befund: Eine Annahmestelle, die mit der Inhaberin zusammenarbeitet (19.11.2026)

Der Prüfauftrag fragt (S, Punkt 2): „Kann eine bestochene Stelle an der anderen Stelle
durchkommen?“ **Ja, und zwar ohne ein einziges Geheimnis der Validatoren.**

**Warum:**
- An Stelle i prüft Annahmestelle 0 in der Basis dᵢ ⊕ c, Annahmestelle 1 in dᵢ ⊕ 1 ⊕ c.
  Das sind immer entgegengesetzte Basen. Die Inhaberin hat jede Stelle in genau einer
  Basis yᵢ gemessen und kann also je Stelle nur für eine Annahmestelle richtig antworten.
- Gegen das Doppeltausgeben schützt allein, dass sie beim Binden von d **nicht weiß,
  welche Stelle welche Annahmestelle prüft** (Befund 5g).
- Die Stichproben sind disjunkt, mit dem Stellen-Bit (Abschnitt 8) sogar genaue
  Gegenstücke: Was nicht zu Stelle 0 gehört, gehört zu Stelle 1.
- Verrät Annahmestelle 0 der Inhaberin vor der Bindung nur, **welche** Stellen ihre sind
  (nicht einmal u oder t), dann setzt sie d dort auf Annahmestelle 0 und überall sonst auf
  Annahmestelle 1 aus. Sie besteht an beiden.
- Die bestochene Stelle meldet zuerst auf der Kette an und bekommt die Deckung. Die andere
  hat ohne Netz angenommen und geht leer aus.
- Belegt in `s_kette.py`, Selbsttest 5i: Mit den verratenen Stellen besteht sie an
  beiden. Gegenprobe: Mit einer zufällig geratenen Hälfte fällt sie an mindestens einer
  durch. Beide Prüfungen werden bei Mutation rot (falsche Stelle verraten, Gegenprobe mit
  Wissen).

**Was das heute bedeutet:**
- Wer eine Stichprobe kennt, darf vor der Bindung nicht mit der Inhaberin
  zusammenarbeiten. Heute kennen sie **beide Annahmestellen und das Ausgabe-Werkzeug**:
  Es lädt beide Stichproben, um ihren Hash auf die Kette zu legen und die ehrliche
  Vorlage exakt nachzurechnen (`quanten_s_ausgabe_abschnitte.go`, Schritt 3).
- Im Hauptnetz und im Pilot war das Werkzeug zugleich Ausgeber, Inhaberin und beide
  Stellen. Es konnte nur sich selbst schaden; eine offene Gefahr gab es also nicht.
- **Vor einer Ausgabe an Dritte gilt:**
  - Die Inhaberin darf weder Ausgeber noch Annahmestelle sein, und sie darf mit keiner
    der beiden zusammenarbeiten.
  - Die beiden Annahmestellen müssen einander vertrauen, etwa zwei Kassen desselben
    Betreibers. So steht es auch bei Kent u. a., wo die Annahmestellen Filialen einer
    Bank sind.
  - Das ist die Antwort auf Prüfauftrag S, Punkt 8: Diese Rollen müssen getrennt sein.
    Die Bindung der Validatoren an verschiedene Menschen reicht dafür nicht.

**Ausweg (spezifiziert in `s_kette.py` 5j, im Knoten noch nicht gebaut): Die Stichprobe
bleibt bis zur Bindung bei den Validatoren.** Eine Annahmestelle braucht ihre Stichprobe erst, wenn die Inhaberin
vorlegt, und das ist immer nach der Bindung. Vorher braucht sie niemand, wenn:
1. **Jeder Validator die Probe für seinen Abschnitt selbst rechnet.** Er kennt dort u, t,
   x, y und die Stellen-Bits, weil er die Messung selbst gemacht hat. Er meldet dem
   Ausgeber nur, unterschrieben:
   - je Annahmestelle die Zahl der Treffer und Fehler seines Abschnitts (für die exakte
     Probe der ehrlichen Vorlage);
   - einen Hash der Stellen seines Abschnitts für diese Annahmestelle, gesalzen.
   - Das Werkzeug zählt zusammen und kennt keine einzige Stelle der Stichproben.
2. **Die Kette den Hash der Stichprobe aus den Hashes je Abschnitt bildet.** Beim
   Anmelden teilt sie die Stichprobe der Stelle nach Abschnitten auf und rechnet nach.
   Das ist eine **Änderung der Kettenregel**, nur für neue Token: neue Version des
   Stichproben-Hashes, eigener Stichtag, alte Token unverändert.
3. **Die Validatoren die Öffnung erst herausgeben, wenn d auf der Kette gebunden ist**,
   und nur an die Stelle aus dem Auftrag. Das ist eine Änderung im Dienst, nicht in der
   Kette.
4. **Die Annahmestelle sich ihre Stichprobe nach der Bindung einmal mit Netz holt.** Die
   Annahme selbst bleibt ohne Netz.

Danach kann eine Annahmestelle der Inhaberin vor der Bindung nichts verraten, weil sie
selbst noch nichts weiß. Zusammenarbeit nach der Bindung hilft nicht mehr, denn d steht
fest. Offen bleibt nur, was jede Stichproben-Prüfung voraussetzt: Ein Validator, der seine
Bits verrät, verrät sie für seinen Abschnitt. Das ist dieselbe Schwelle wie heute bei
seinen Werten (t − 1 Validatoren kennen genau ihre Abschnitte, Abschnitt 7).

**Spezifiziert (`s_kette.py`, Selbsttest 5j):**
- `Praeparator.probe_teil(b)`: gesalzener Teil-Hash seiner Stellen für Annahmestelle b.
  Das Salz leitet er aus seinen geheimen Werten ab; nur er kennt es.
- `Praeparator.probe_zaehlung(b, y, x)`: Treffer und Fehler auf seinen Stellen, nur Zahlen.
  Ihre Summe ist genau das, was die Stelle bei ehrlicher Vorlage prüft (getestet). Die
  exakte Probe der ehrlichen Vorlage geht damit, ohne dass jemand eine Stelle kennt.
- `probe_hash_v2`: die Bindung auf der Kette aus den Teil-Hashes der Reihe nach. Beim
  Anmelden rechnet die Kette sie aus der Stichprobe der Stelle und den Salzen nach
  (`probe_v2_pruefen`, Token mit `probe_art = 2`). Alte Token bleiben unverändert.
- `Praeparator.oeffnung_nach_bindung(b, kette)`: Öffnung und Salz erst, wenn d und c auf
  der Kette stehen.
- Tests:
  - Gutfall und Doppelvorlage; Einlösen auf der Kette mit Teil-Hashes.
  - Öffnung vor der Bindung und vor der Wahl abgewiesen (für jeden Validator und jede
    Stelle).
  - Anmelden abgewiesen mit: Teilmenge, einer Stelle mehr, falschem Salz, ohne Salz, dem
    Salz der anderen Stelle, der Stichprobe der anderen Stelle.
  - Ein Token mit `probe_art = 1` nimmt Teil-Hashes nicht an.
- Gegenprobe: 8 Mutationen, alle rot:
  - Öffnung ohne Bindung; Öffnung ohne Wahl;
  - v2 nicht geprüft; Salz ignoriert; Abschnitte nicht getrennt;
  - Zählung ohne Basis; Teil-Hash über alle Stellen; Salz für beide Stellen gleich.
- Die Prüfvektoren sind unverändert.

**Nächste Schritte:**
1. ~~Prüfvektor und Bausteine in Go~~ **fertig (19.11.2026):**
   - `s_kette.py --vektor-probe-v2` → `testdata/quanten_s_probe_v2_vektor.json`; CI
     vergleicht Byte für Byte.
   - `quanten_s_probe_v2.go`: Teil-Hash, Bindung v2 und das Nachrechnen der Kette.
     Das Nachrechnen schließt ab bei doppelten Stellen, Stellen außerhalb, falscher
     Zahl der Salze oder Stelle ≠ 0/1. Noch nirgends eingehängt.
   - Test gegen den Vektor (auch unsortiert) und 20 verschiedene Missbrauchsfälle:
     andere Bindung oder ausdrücklicher Fehler.
   - Gegenprobe: 10 Mutationen rot, 3 davon erst nach Schärfung der Tests (ein Salz zu
     viel, unsortiert, Stelle 2). Zwei Grenzen prüft auch `quantenSAbschnitt`; beide
     Schichten zusammen mutiert sind rot.
2. ~~Dienst~~ **fertig (19.11.2026)**, nur der Dienst, nicht die Kette
   (`quanten_s_abschnitt_dienst.go`):
   - Ein Auftrag mit `probe_art = 2` (in der Unterschrift des Ausgebers) bekommt je
     Annahmestelle Teil-Hash und Zählung in der Antwort; die Unterschrift des Validators
     deckt beides.
   - Das Salz (32 Byte, Zufall) liegt verschlüsselt im Tresor. Die Öffnung gibt es erst,
     wenn d und c auf der Kette stehen **und final sind**, und das Salz steckt im
     verschlüsselten Inhalt.
   - Die Finalität kam aus der eigenen Sicherheitsprüfung (`reif`, wie bei den Anteilen).
     Fiele der Block der Bindung noch aus der Kette, hätte eine bestochene Stelle die
     Stichprobe gesehen, und die Inhaberin bände danach mit diesem Wissen neu.
   - Der Takt sichtet Bindung und Wahl, sobald sie stehen; ab dann läuft die Uhr.
     `quantenSOeffnungEntschluesselnV2` verlangt es; die bisherige Lesart weist eine
     Öffnung mit Salz ab.
   - Bisherige Aufträge (`probe_art` fehlt): Nachrichten, Antwort und Öffnung bleiben
     Wort für Wort gleich (Test gegen den alten Wortlaut).
   - Tests (`quanten_s_probe_v2_dienst_test.go`):
     - Gutfall gegen die Kette: keine Öffnung vor der Ausgabe und nicht vor der Wahl.
     - Danach rechnet jede Stelle Teil-Hashes und die Bindung v2 nach, und die Summe der
       Zählungen ist genau ihre Prüfung.
     - Wiederholung liefert dasselbe.
     - Missbrauch: Art 1, 3 und −2; Art ohne Unterschrift; geänderte Teile oder Zählung
       in der Antwort; v1-Öffnung als v2 gelesen.
   - Gegenprobe: 14 Mutationen rot, eine erst nach Schärfung des Tests (v2 ohne Salz).
     Drei davon gelten der Finalität: Öffnung ohne `reif`, Takt sichtet nicht, Takt
     ohne Prüfung der Wahl.
   - Das Werkzeug nutzt v2 noch nicht. Es braucht die Kettenregel, um anzumelden.
3. **Kettenregel `probe_art = 2`: gebaut, schlafend (19.11.2026).** Das ist eine
   **Konsensänderung**; scharf wird sie erst, wenn der Betreiber den Stichtag
   `quantenSProbeV2AbUnix` setzt. Bis dahin gilt die Regel oben: Rollen trennen, die
   Stellen müssen einander vertrauen.
   - Ausgabe mit `probe_art = 2`: Der Ausgeber unterschreibt `|probe:2` mit. Ein Token
     trägt die Art, andere Arten als 0 und 2 werden abgewiesen.
   - Anmelden bei einem v2-Token: je Abschnitt ein Salz, die Kette rechnet
     `quantenSProbeV2Pruefen` nach. Ein bisheriger Token weist Salze ab.
   - Register: `probe_art` und `salze` nur, wenn gesetzt. Bisherige Token behalten ihr
     Register Wort für Wort (getestet), der StateRoot bleibt gleich. Das Lesen schließt
     ab: unbekannte Art, fehlende oder falsche Salze, Salze an einem bisherigen Token.
   - Schlafend: Vor dem Stichtag bekommt jeder S-Satz mit diesen Feldern einen Befund
     (Grund unten). Ein leeres `salze` gilt wie ein fehlendes, derselbe Bescheid und
     derselbe Hash.
   - Deploy-Wächter `.github/neustart/quanten-s-probe-v2-nicht-verpasst.sh` (nach dem
     Muster von W/S) und `/api/status` → `quanten_w.probe_v2_ab`, nur wenn gesetzt.
     Getestet schlafend und mit gesetztem Stichtag.
   - Tests (`quanten_s_probe_v2_kette_test.go`): ganzer Weg nach dem Stichtag.
     - Dienst v2 → Ausgabe v2 → Bindung, Wahl, Finalität → Stichproben mit Salzen.
     - Doppelvorlage an Stelle 1 abgewiesen, ehrliche Vorlage an Stelle 0 eingelöst
       (Validatoren legen offen, die Deckung geht an die Stelle).
     - Missbrauch: Art 1 und 3; Art ohne und mit falschem Wortlaut unterschrieben;
       Anmelden ohne Salze, mit falschem, fehlendem, fremdem oder kaputtem Salz, mit
       Teilmenge, mit fremder Stichprobe.
     - Register rund und fail-closed.
   - Gegenprobe: 15 Mutationen rot, zwei erst nach Schärfung der Tests (Art 3 ohne
     Anmeldung, Salze an einem bisherigen Token im Register).
   - **Was bleibt:** das Werkzeug (Schritt 4, unten) und die Freigabe des Betreibers mit
     einem Stichtag.
4. **Werkzeug im v2-Weg: fertig (19.11.2026)** (`aequitasd quanten-s-ausgabe
   --stichprobe-v2`, `quanten_s_ausgabe_abschnitte.go`):
   - Vor jedem Auftrag fragt das Werkzeug den eigenen Knoten
     (`/api/status` → `quanten_w.probe_v2_ab ≤ jetzt`). Schläft die Regel, wird nichts
     beauftragt, und keine Messung wird verbraucht. Nur im Abschnitts-Modus.
   - Aufträge mit `probe_art = 2`. Jede Antwort muss zwei Teil-Hashes und mögliche
     Zählungen tragen (0 ≤ Fehler ≤ Treffer ≤ Stellen); eine v1-Antwort mit Teilen wird
     abgewiesen.
   - Die Bindungen der Ausgabe kommen aus den Teil-Hashes, die exakte Probe der ehrlichen
     Vorlage aus den Zählungen. Das Werkzeug kennt dabei keine einzige Stelle.
   - Nach Bindung und Wahl lädt jede Annahmestelle ihre Stichprobe und die Salze. Bis die
     Validatoren sie nach der Finalität herausgeben, fragt sie begrenzt nach
     (`--frist`). Danach prüft sie selbst:
     - Wurzel, Blatt, Stellen-Bit und k_note;
     - die Bindung v2 aus den Salzen;
     - je Validator seine Zählung. Weicht sie ab, kommt ein **BEFUND mit seiner Nummer**,
       noch vor der ersten Vorlage.
   - Anmelden mit den Salzen.
   - Tests (`quanten_s_ausgabe_v2_test.go`, gegen echte Dienste und Kette):
     - Gutfall (Doppelvorlage abgewiesen, ehrliche eingelöst, keine Öffnung vor
       Bindung und Wahl);
     - Nachfragen bis zur Finalität; falsche Zählung (Befund mit Nummer);
     - falsches Salz; falscher Wert; vertauschte Öffnung; Antworten ohne Teile oder mit
       Teilen bei v1;
     - verrauschte Messung (Abbruch vor der Ausgabe); schlafende Kette;
     - Status des Knotens (9 Fälle); Plausibilität der Antworten (8 Fälle).
   - Gegenprobe: 14 Mutationen rot, zwei erst nach Schärfung der Tests (falscher Wert,
     verrauschte Messung).
   - **Was bleibt:** die Freigabe des Betreibers mit einem Stichtag. Dann einen Pilot mit
     `--stichprobe-v2` fahren (der Vier-Knoten-Pilot bekommt die Wahl dazu).
   - **Wichtig beim Bau (berichtigt):** Ein älterer Knoten verwirft unbekannte Felder
     beim Einlesen. Der Blockhash läuft aber über `json.Marshal` der Transaktionen
     (`calculateBlockHash`); einen Block mit `probe_art` oder `salze` kann er also nicht
     nachrechnen und weist ihn ganz ab.
     - „Schlafend“ heißt deshalb: Vor dem Stichtag gibt der neue Knoten jedem Satz mit
       diesen Feldern einen Befund. Damit kommt er nicht in die Annahme, und ein Block
       mit ihm scheitert beim Nachspielen. Alte und neue Knoten weisen denselben Block ab.
     - „Übergehen“ (eine frühere Fassung dieser Notiz) wäre falsch: Der neue Knoten nähme
       den Block an, der alte nicht.
   - Das Register trägt `probe_art` und die Salze nur, wenn sie gesetzt sind. Der
     StateRoot bestehender Token bleibt so gleich.
5. **Pilot mit vier Validatoren im v2-Weg: grün (19.11.2026)** (`quanten-s-pilot.yml`,
   Eingabe `stichprobe_v2 = ja`, nur mit `ausgabe = abschnitte`; `HEUTE.md` 3.3h):
   - Lauf [44564851989](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44564851989),
     Rauschmodell, k = 1.760, Toleranz 2 %: beide Tests grün, auch mit Ausfall eines
     Validators vor der ersten Vorlage. Fehler 0/1.771 und 0/1.753 aus den Zählungen der
     Validatoren; Stichproben 10 s nach der Wahl geladen; Werkzeug 3m50s und 3m37s.
   - Die Regel wird nur im Testprozess der vier Knoten scharf geschaltet
     (`AEQUITAS_QS_V2=1`, `t.Cleanup` setzt zurück). Ohne Abschnitts-Modus bricht der
     Test ab, statt still den alten Weg zu gehen.
   - Nachgeschärft: Der Test liest jetzt im Register jedes Knotens nach, welcher Weg
     wirklich lief (`probe_art`, Zahl der nachgerechneten Salze). Vorher hätte er auch
     grün werden können, wenn der v2-Weg nicht gegriffen hätte.
   - Lauf 2 mit dieser Prüfung
     [44568310804](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44568310804):
     beide Tests grün. Auf der Kette standen jedes Mal `probe_art 2` und 4 nachgerechnete
     Salze. Fehler 0/1.787 und 0/1.772. Die Stelle der Inhaberin war diesmal Stelle 0,
     damit sind beide Richtungen belegt.
   - **Was bleibt:** die Freigabe des Betreibers mit einem Stichtag
     (`quantenSProbeV2AbUnix`). Vorher müssen alle Knoten den Stand mit der Regel haben
     (seit #596 auf C1); der Deploy-Wächter verlangt mindestens 30 min Vorlauf.
6. **Stichtag gesetzt (19.11.2026):** Der Betreiber hat freigegeben: „Stichtag heute so
   schnell wie möglich“. `quantenSProbeV2AbUnix = 1795116600` = **19.11.2026 19:30 UTC**.
   - Früher ging nicht. Der Deploy-Wächter verlangt beim Ausrollen mindestens 30 min
     Vorlauf, davor liegen CI, Merge und die Freigabe des Deploys. Der Deploy muss also
     bis 19:00 UTC laufen. Sonst hält der Wächter an, und es braucht einen neuen
     Stichtag; der Wächter wird nicht gelockert.
   - Wirksam ist der Stichtag erst, wenn C1 mit diesem Stand läuft (`/api/status` →
     `quanten_w.probe_v2_ab`). Bis dahin schläft die Regel auf C1 weiter.
   - Ab dem Stichtag: Ausgaben mit `probe_art = 2` und Anmelden mit Salzen. Der
     bisherige Weg bleibt erlaubt; bestehende Token ändern sich nicht.
   - Tests: Der Stichtag ist im Test festgeschrieben, samt Grenze (eine Sekunde davor
     schlafend). Die Zeit davor wird per Override geprüft, unabhängig von der Uhr.
     Gegenprobe: anderer Stichtag, `>` statt `>=` und schlafend, alle drei rot.
   - **Ausgerollt:** C1-Deploy #107
     ([44584732345](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44584732345)),
     freigegeben 18:33 UTC, grün. Der Wächter meldete: „Repo 1795116600, laufend schlafend,
     jetzt 1795113226 – ausrollen erlaubt“ (57 min Vorlauf). Scharf ab 19:30 UTC nach der
     Blockzeit. Die Wache (`wache.yml`, #600) gibt `probe_v2_ab` als Hinweis aus.
   - **Belegt scharf:** Wache-Lauf
     [44601606565](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44601606565)
     (19:45:58 UTC, grün): „Quantengeld ab 1794934800 (aktiv), probe_v2_ab 1795116600
     (aktiv)“.
   - **Was bleibt:**
     - Der erste Hauptnetz-Token im v2-Weg. Er braucht eine Messung auf dem Chip, also
       die Freigabe `quantenhardware`.
     - Der zweite Mensch als Validator (A5, Rollen trennen).
     - Ein Gutachten (Prüfauftrag S 2).
7. **Hauptnetz-Weg im v2-Weg (vorbereitet, 19.11.2026):** Der erste Token mit
   `probe_art = 2` auf dem Hauptnetz, C1 als Ausgeber und einziger Validator
   (t = v = 1).
   - **Größe:** Bei t = v = 1 verlangt die Tabelle der Kette k = 552 bei 2 % Toleranz
     (672 bei 3 %, 840 bei 4 %). Ein Token hat also 1.104 Qubits, ein kleiner Auftrag.
   - **C1 bekommt ein Messverzeichnis** (`deploy/quanten-c1-messverzeichnis.sh`, beim
     Deploy vor dem Bauen):
     - `/srv/aequitas-quanten`: Benutzer 10001, 0700, kein Verweis.
     - `docker-compose.override.yml`: eine Kopie von `docker-compose.quanten.yml` mit
       Kennzeile. Docker Compose liest sie bei jedem Aufruf im Verzeichnis, also auch
       bei Rücknahme und Neustarts aus anderen Skripten. So hat der Knoten die
       Messquelle immer oder nie.
     - Eine fremde override-Datei, ein Verweis oder ein abweichendes `QS_VERZEICHNIS`
       halten den Deploy an.
     - Neu erreichbar auf C1: Abschnitts-Aufträge (ohne passende Messung abgewiesen)
       und Vormerkungen (unterschrieben, Mensch mit Deckung, höchstens 8 wartend,
       8 je Ausgeber und Tag, 32 am Tag; `DEZENTRAL.md` 15). C1 hat keine
       Warteschlange, die misst; nach 8 Vormerkungen weist C1 weitere ab, bis der
       Betreiber aufräumt.
   - **Workflow** `quanten-s-ausgabe.yml`, Eingabe `weg = abschnitte-v2`:
     - Prüft vor jeder Messung, dass die Stichprobe v2 auf der Kette gilt.
     - Zieht die Basen der Inhaberin aus dem Zufall des Betriebssystems.
     - Misst genau diesen Abschnitt (`hardware_ibm.py --s-abschnitt`, 5 Schüsse mit
       Mehrheit).
     - Legt die Messung über `deploy/quanten-s-ausgabe-abschnitt.sh` ins
       Messverzeichnis: erst unter eigenem Namen 0600, dann per `ln`, nie
       überschreiben.
     - Startet `aequitasd quanten-s-ausgabe --stichprobe-v2`. Eine nicht verbrauchte
       Messung wird danach entfernt.
     - Im Modus `rauschen` nur die Messung mit dem Rauschmodell (Probe, keine Ausgabe).
   - **Tests** (in CI):
     - `scripts/test_quanten_s_ausgabe_abschnitt_skript.sh` (43 Fälle):
       - Gutfall: Aufruf, Basen über stdin, Messung unter dem Namen der Basen mit 0600.
       - Abbruch ohne Ablage und ohne Aufruf: Deckung, Toleranz, Name, Knotenbenutzer,
         Verweise, Größen, Rechte und Besitzer des Verzeichnisses, Container, fehlende
         oder andere Messquelle, schon vorhandene Messung (bleibt unverändert).
       - Aufräumen auch im Fehlerfall.
     - `scripts/test_quanten_c1_messverzeichnis.sh` (19 Fälle): anlegen, gleichbleibend,
       eigenes altes override ersetzt, Rechte gerichtet; fremdes override, Verweise,
       Datei statt Verzeichnis, anderes `QS_VERZEICHNIS`, fehlende Dateien und
       falscher Knotenbenutzer halten an, ohne etwas zu ändern.
     - Beide Tests auch ohne root grün.
     - Gegenprobe: 14 Mutationen, 12 rot. Die 2 überlebenden sind gleichwertig (eine
       doppelte Prüfung: `ln` scheitert ohnehin an einem vorhandenen Namen;
       `[ -d ] && [ ! -L ]` fängt den Verweis).
   - **Ausgerollt und gelaufen (20.11.2026):**
     - C1-Deploy #108
       ([44611475140](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44611475140)):
       Messverzeichnis und `AEQUITAS_QS_MESSQUELLE=/data/quanten` belegt.
     - **Erster Token mit Stichprobe v2 im Hauptnetz:**
       [44691759501](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44691759501),
       `ibm_kingston`, Fehler 0/548, K = `1a1e0800…6f23edd5`. Doppelvorlage abgewiesen,
       ehrliche Vorlage eingelöst, 15m21s (`HEUTE.md` 3.3i).
     - Je Schritt rund 5 Minuten Wartezeit auf die Finalität: Stichproben und Anteile
       gibt der Dienst erst nach der Finalität heraus.
   - **Ehrlich dazu:**
     - Für genau einen Validator im Abschnitts-Weg gibt es keinen eigenen Go-Test; die
       Testwelten haben fünf Validatoren. Der Hauptnetz-Lauf vom 20.11. hat den Weg
       belegt.
     - Der Hauptnetz-Lauf selbst ist die Probe. Scheitert er nach der Ausgabe, kann die
       Deckung des Tokens (1 AEQ) bis zur Frist gebunden sein.
     - Wie am 17.11. laufen Präparation und Messung im selben Auftrag beim Betreiber,
       und C1 kennt als einziger Validator den ganzen Token.

## 17. Getrennte Rollen: Inhaberin, Ausgeber, Annahmestelle (20.11.2026)

Auftrag des Betreibers (20.11.2026): „Dezentrales Quantengeld muss komplett implementiert
werden.“

### Was „komplett“ hier heißt

1. **Jetzt gebaut (R1–R3, dieser Abschnitt):** Jede Rolle mit eigenem Befehl und eigenem
   Schlüssel. Kein Werkzeug hält mehr alles in der Hand.
2. **Baubar, als Nächstes:**
   - ~~R4a: Abholen mit K, kompakte Vorlage~~ – gebaut (unten).
   - R4b: Die App als Inhaberin (Bestellung, abholen mit K, binden, Vorlage als QR-Code).
   - Ein Hauptnetz-Lauf mit getrennten Rollen (Workflow).
   - ~~Die Öffnung aus Anteilen statt nur vom präparierenden Validator (Verfügbarkeit)~~
     – gebaut (R9, unten).
   - ~~Die Basen y je Validator verschlüsseln (Privatsphäre)~~ – gebaut (R8, unten).
3. **Hängt am Betreiber:**
   - Ein zweiter Mensch als Validator mit eigenem Server und IBM-Konto (A5).
   - Ein Gutachter (Prüfauftrag S 2).
   - Ein Partner mit eigenem Empfänger für die Inhaberin (Weg C).
4. **Heute physikalisch nicht möglich:** Quantenzustände in den Händen der Menschen. Das
   braucht Quantenspeicher in Geräten, und die gibt es nirgends (`HEUTE.md`, Punkt 1).
   Variante S ist deshalb ein Token ohne Speicher: Gemessen wird bei der Präparation, die
   Inhaberin bekommt klassische Ergebnisse x.

### Das Problem

`aequitasd quanten-s-ausgabe` spielte bisher alle Rollen außer den Validatoren:
- Es erzeugte den Schlüssel der Inhaberin und den der zweiten Annahmestelle selbst.
- Es entschlüsselte x der Inhaberin.
- Es band, wählte und legte an beiden Stellen vor.

Das belegt, dass die Kette richtig urteilt. Wer das Werkzeug laufen lässt, hält aber alles
in der Hand, und das ist nicht dezentral. Der Validator-Dienst konnte die Trennung schon:
- Im Auftrag stehen Inhaberin und Stellen mit ihren öffentlichen Schlüsseln.
- x geht verschlüsselt an die Inhaberin.
- Die Öffnung geht verschlüsselt an die jeweilige Stelle.

### Gebaut (`x/humanity/keeper/quanten_s_rollen.go`)

| Rolle | Befehl | Was sie prüft und tut |
|---|---|---|
| Inhaberin | `aequitasd quanten-s-inhaberin neu --datei F --n N --v V --stellen adr=schl,adr=schl` | Neuer Schlüssel; Basen y aus `crypto/rand`. Die Datei F hat Schlüssel und y, 0600, wird nie überschrieben. Heraus kommt die **Bestellung** (öffentlich). |
| Ausgeber | `aequitasd quanten-s-ausgabe --fuer-inhaberin` < Bestellung | Abschnitte bei den Validatoren, Bestätigungen, Ausgabe auf der Kette. Nur mit Stichprobe v2, nicht für sich selbst. Er hat keinen Schlüssel der Inhaberin oder der Stellen. Heraus kommt das **Paket** (öffentlich, x nur für die Inhaberin verschlüsselt). |
| Inhaberin | `aequitasd quanten-s-inhaberin annehmen --datei F --api URL --validatoren 0xadr,…` < Paket | Unterschrift jedes Validators über seinen Abschnitt; die Validatoren nennt sie selbst. Blätter gegen die Wurzel, Proben gegen die Teil-Hashes. Wurzel, Proben, n, t, v, Toleranz und Stellen gegen die **Kette**. Aus den unterschriebenen Zählungen prüft sie, dass die ehrliche Vorlage bestünde. Dann liest sie x. |
| Inhaberin | `aequitasd quanten-s-inhaberin einloesen --datei F --api URL --stelle b` | Sie speichert die Maske z, **bevor** sie die Bindung einreicht. Sie bindet d = y ⊕ z und wählt c = b ⊕ z, jeweils nur, wenn es noch nicht auf der Kette steht. Sie unterschreibt die **Vorlage** nur für die gewählte Stelle; für die andere wäre es eine Doppelvorlage. |
| Annahmestelle | `aequitasd quanten-s-annahme --validatoren adr=api,…` < Vorlage | Den Schlüssel liest sie aus einer Umgebungsvariablen, nicht von der Befehlszeile. Vor jeder Anfrage an einen Validator prüft sie: die Unterschrift der Vorlage gegen den Inhaber **auf der Kette**, die Blätter gegen die Wurzel auf der Kette, dass die Vorlage für **diese** Stelle ist, Bindung und Wahl stehen. Nach der Finalität lädt sie ihre Stichprobe mit ihrem eigenen Schlüssel und prüft Salze und Stichprobe gegen die Probe auf der Kette. Dann rechnet sie das Urteil der Kette **vorab** nach (Basis d ⊕ b ⊕ c), meldet an und wartet auf das Urteil. Widersprechen sich Vorab-Rechnung und Urteil, ist das ein BEFUND. |

- **Kein neuer Konsens.** Die Kette prüfte das schon: Nur die Inhaberin bindet und wählt
  (`quantenSToken.binden`/`waehlen`), die Vorlage ist von ihr unterschrieben, an Stelle b
  wird in d ⊕ b ⊕ c geprüft (`offenlegenTeil`).
- **Neu in der Ansicht** (`GET /api/quanten-w/note`, nur lesend): `wurzel`, `proben`,
  `d` (erst nach der Bindung) und `c` (erst nach der Wahl). Das steht ohnehin öffentlich
  in den Transaktionen. Damit glaubt keine Rolle einem Paket oder einer Vorlage.
- **Die Unterschrift des Validators** über seinen Abschnitt lässt sich jetzt auch aus den
  Hashes der Bündel nachrechnen (`quantenSAbschnittAntwortNachrichtAus`). Sie ist
  byte-gleich mit der bisherigen; die Inhaberin bekommt die Bündel für die anderen
  Validatoren nicht.
- **Validator-APIs** kommen nur aus der eigenen Angabe, nie aus Paket oder Vorlage. Keine
  Rolle fragt eine fremde Adresse an.
- **Den Stand der Kette** liest jede Rolle bei dem Knoten, den sie selbst nennt (`--api`),
  und glaubt ihm, wie jede Wallet ohne eigenen Knoten. Die Annahmestelle nimmt darum ihren
  eigenen Knoten (Vorgabe `127.0.0.1`). Lügt der Knoten der Inhaberin, kann sie einen Token
  annehmen, den es nicht gibt. Einlösen kann ihn dann niemand, verloren geht kein Wert.
- **Grenzen:**
  - Bestellung, Paket, Vorlage und Datei höchstens 4 MiB.
  - Strenges JSON: keine unbekannten Felder, nur ein Wert.
  - Jede Anfrage mit Frist.
  - Die Datei der Inhaberin wird nur gelesen, wenn sie 0600 ist und kein Verweis, auch
    nicht zwischen Prüfen und Öffnen vertauscht (`os.SameFile`). Ersetzt wird sie atomar.
  - Schlüssel stehen in keiner Ausgabe und keiner Fehlermeldung.

### Belegt

- **Tests in CI** (`quanten_s_rollen_test.go`, gegen die echten Regeln und den echten
  Dienst):
  - Gutfall für beide Stellen. Der Ausgeber kann x nicht lesen. Der Wert geht an die
    gewählte Stelle, nichts an die andere.
  - Ein fälschender Ausgeber: x neu verschlüsselt, Blatt, Teil-Hash oder Zählung
    vertauscht, Wurzel oder Probe anders, andere Validatoren, Inhaberin oder Stellen,
    Abschnitte vertauscht oder fehlend, v1 statt v2.
  - Ein böser Ausgeber, der seine Probe der ehrlichen Vorlage überspringt: Die Kette
    nimmt die Ausgabe an, die Inhaberin nicht.
  - Die Kette entscheidet: Steht der Token nicht auf der Kette, ist er schon gebunden
    oder hat er eine andere Wurzel, wird nichts angenommen.
  - Keine Doppelvorlage aus dem Werkzeug der Inhaberin. Die Maske wird zuerst
    gespeichert.
  - Eine böse Inhaberin unterschreibt selbst für die andere Stelle: vorab abgewiesen, von
    der Kette abgewiesen (ein Fehlversuch); danach löst die gewählte Stelle ehrlich ein.
  - Eine gefälschte Vorlage (Unterschrift, x, Blatt, falsche Stelle, unbekanntes K)
    löst keine einzige Anfrage an die Validatoren aus.
  - Vor Bindung und Wahl wird nicht angemeldet. Ein falsches Salz fliegt beim Laden auf.
  - Vorab „abgewiesen“, aber ein lügender Knoten meldet „eingelöst“: BEFUND.
  - Die Datei der Inhaberin: 0600, kein Verweis, nie überschrieben, keine fremden Felder.
  - Die Befehle: Sperren (Unterbefehle, Optionen, nicht als root, kein Schlüssel auf der
    Befehlszeile, kein Schlüssel in der Ausgabe).
  - Unter `go test -race` grün.
- **Gegenprobe:** 24 Mutationen, alle rot. Zwei überlebten zuerst und zeigten Lücken in
  den Tests: die eigene Zählprobe der Inhaberin, und dass eine gefälschte Vorlage schon
  vor der Kette auffällt. Beide haben jetzt einen Test.
- **Vier echte Knoten** (`TestVierKnotenQuantenSRollen(Ausfall)_RealDB`; eigene
  Datenbank, echtes HTTP, 3 von 4): Der Ausgeber spricht mit dem Folger, die Inhaberin
  mit einem dritten Knoten, beide Stellen mit dem Leiter. Jede Übergabe geht durch JSON
  wie eine Datei: Bestellung 918 Byte, Paket 22.651 Byte, Vorlage 17.651 Byte bei n = 256.
  Lokal am 20.11.2026:
  - Ohne Ausfall grün (51 s): Doppelvorlage vorab „abgewiesen“ (42 Fehler bei 69
    Treffern), von der Kette abgewiesen. Ehrlich vorab „besteht“ (0 Fehler bei 68
    Treffern), eingelöst. Alle vier Knoten im Gleichstand.
  - Mit Ausfall grün (62 s): Ein Validator fällt bei der ehrlichen Anmeldung aus,
    nachdem die Stelle ihre Stichprobe geladen hat. Die drei übrigen legen offen,
    eingelöst. Drei Knoten im Gleichstand.
  - Die bisherigen Wege (ein Auftrag; Abschnitte mit und ohne v2) laufen nach dem Umbau
    unverändert grün.
  - **Auf GitHub in voller Größe:** Lauf
    [44728392142](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44728392142)
    (`quanten-s-pilot.yml`, Zweig `claude/weiter-uw2p42`, aae3dd9b; Rauschmodell
    `fake_marrakesh`, Mehrheit aus 5, k = 1.760, n = 3.520, Toleranz 2 %, `ausgabe =
    abschnitte`, `stichprobe_v2 = ja`, `rollen = ja`). Beide Tests grün:

    | Test | Fehler (Zählungen) | Doppelvorlage an Stelle 0 | ehrlich an Stelle 1 | Rollen gesamt | danach |
    |---|---|---|---|---|---|
    | alle vier (240,9 s) | 0/1.763 | vorab „abgewiesen“ (431 Fehler bei 874 Treffern), Kette **abgewiesen** nach 1m38s | vorab „besteht“ (0/877), Kette **eingelöst** nach 2m00s | 3m40s | 4 Knoten gleich, Höhe 2256, StateRoot `b1dbce17…69f897` |
    | ein Validator fällt aus (257,6 s) | 0/1.736 | vorab „abgewiesen“ (468/908), **abgewiesen** nach 1m42s | vorab „besteht“ (0/884), **eingelöst** nach 2m10s | 4m03s | 3 Knoten gleich, Höhe 2170, StateRoot `adfcbd4e…72930` |

    - In beiden Tests: „Auf der Kette: probe_art 2, 4 Validatoren, Anmeldung mit 4 Salzen
      nachgerechnet“.
    - Die Inhaberin wählte beide Male Stelle 1, die böse Doppelvorlage ging an Stelle 0.
      Das ist Zufall (`quantenSZufallInt`); lokal war es umgekehrt (Stelle 1 abgewiesen,
      Stelle 0 eingelöst), damit sind beide Richtungen belegt.
    - Größen bei n = 3.520: Bestellung 4.183 Byte, Paket 247.897 Byte, Vorlage 239.603 Byte.
      Fast alles davon sind Blätter (64 Hex-Zeichen je Stelle).
    - SHA-256: `ergebnis.txt` `037778cc…048799`, `ausgabe.txt.gz` `25fc1152…ff7ee`
      (Artefakt `pilot-44728392142`, 90 Tage).

### R4a: Abholen mit K, kompakte Vorlage (20.11.2026)

Das Paket des Ausgebers ist groß (247.897 Byte bei n = 3.520), und die Vorlage mit allen
Blättern ebenso (239.603 Byte). Für ein Telefon und einen Laden taugt das nicht. Beides
fällt jetzt weg:

- **Neuer Endpunkt am Validator:** `GET /api/quanten-w/validator/fuer-inhaberin?k=K`
  (`abschnittFuerInhaberin`, `handleQuantenSFuerInhaberin`).
  - Er liefert den eigenen Abschnitt, von diesem Validator unterschrieben. Die Bündel für
    die anderen Validatoren stehen nur als Hashes darin, x nur für die Inhaberin
    verschlüsselt. Blätter, Teil-Hashes und Zählungen sind Bindungen ohne Geheimnis.
  - Öffentlich und nur lesend. Er antwortet erst, wenn der Token auf der Kette steht, und
    nur bei Stichprobe v2. Begrenzt wie die anderen Anfragen an den Dienst (je Adresse,
    `burstQuantenWDienstJeIP`).
  - K steht öffentlich auf der Kette; jeder kann den Endpunkt also fragen. Er gibt nichts
    heraus, womit man einlösen könnte (keine Werte, keine Salze, x verschlüsselt).
  - Vor der Ausgabe antwortet er gleich, ob ein Abschnitt im Tresor liegt oder nicht; er
    verrät nicht, was vorbereitet ist (zuerst wird die Kette geprüft).
  - Je Anfrage entsteht die Antwort neu: v − 1 Bündel versiegeln, eine Verschlüsselung,
    eine Unterschrift. Das ist dieselbe Größenordnung wie die Öffnung und je Adresse
    begrenzt.
- **Die Inhaberin holt ab:** `aequitasd quanten-s-inhaberin abholen --k K --validatoren
  adr=api,…`.
  - Den Stand liest sie von ihrer Kette, jeden Abschnitt direkt vom Validator.
  - Danach prüft sie genau wie beim Paket: Unterschriften, Wurzel, Proben, Zählungen,
    Kette. Das Paket des Ausgebers braucht sie nicht mehr; nur K muss sie erfahren.
- **Die Vorlage ist kompakt:** K, Stelle, x und die Unterschrift, bei n = 256 nur 486 Byte
  statt 17.651, bei n = 1.104 rund 1,3 KB. Das passt in einen QR-Code.
  - Die Annahmestelle holt die Blätter selbst bei den Validatoren. Jeder Abschnitt muss
    von seinem Validator unterschrieben sein, und alle zusammen müssen die Wurzel auf der
    Kette ergeben.
  - Blätter dürfen weiter in der Vorlage stehen. Dann prüft die Stelle sie gegen die
    Wurzel, ohne einen Validator zu fragen.
- **Belegt:**
  - Tests: Abholen mit K (falsche Zahl von Validatoren, unbekanntes K, ein lügender
    Validator; dasselbe x wie aus dem Paket). Der Endpunkt: unbekanntes K, vor der
    Ausgabe, v1, nur GET, K-Form, Begrenzung, Unterschrift. Kompakte Vorlage mit einem
    Validator, der über ein Blatt lügt: weder Öffnung noch Anmeldung.
  - Eine gefälschte Vorlage fragt jetzt auch keinen Abschnitt mehr ab.
  - Gegenprobe: 11 neue Mutationen, alle rot; die 22 aus R1–R3 weiter rot. Eine überlebte
    zuerst: Ein Tresor-Eintrag ohne Abschnitt (Token aus dem Weg mit einem Auftrag) war
    nicht getestet; ohne die Prüfung gäbe es dort einen Nil-Zugriff. Jetzt mit Test.
  - Vier echte Knoten lokal, beide grün (50 s und 62 s): Die Inhaberin holte nur mit K
    ab, die Vorlage hatte 486 Byte.
  - **Auf GitHub in voller Größe:** Lauf
    [44734484196](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44734484196)
    (c85c486a; Rauschmodell, k = 1.760, n = 3.520, Toleranz 2 %, `rollen = ja`), beide
    Tests grün:
    - Die Vorlage hatte **3.750 Byte statt 239.603**, das Paket des Ausgebers wurde nicht
      benutzt.
    - Ohne Ausfall (241,7 s): Fehler 0/1.785. Doppelvorlage vorab „abgewiesen“ (427/833),
      Kette abgewiesen; ehrlich vorab „besteht“ (0/858), eingelöst. 4 Knoten gleich,
      StateRoot `edfab994…fdb55ea`.
    - Mit Ausfall (370,2 s): Fehler 0/1.800. Doppelvorlage abgewiesen, ehrlich eingelöst
      (0/923), 3 Knoten gleich, StateRoot `4738cd89…798b09`.
    - SHA-256: `ergebnis.txt` `f144c5ce…243c9ba`, `ausgabe.txt.gz` `0432a0ca…900515d`
      (Artefakt `pilot-44734484196`, 90 Tage).
    - Danach kam nur noch 1a58a191 (Reihenfolge der Prüfungen im Endpunkt), durch Tests und
      Mutationen belegt.
- **#606 gemergt (7dcfe2e7).**

### R4b: die App als Inhaberin (20.11.2026)

- **Teil 1, Bibliothek** (Aequitas-App `lib/quantenS.ts`): Sie rechnet wie das Werkzeug der
  Inhaberin.
  - x aus `fuer_inhaberin` lesen (ECIES wie go-ethereum: secp256k1-ECDH, Concat-KDF mit
    SHA-256, AES-128-CTR, HMAC-SHA-256, Tag in konstanter Zeit verglichen).
  - Jeden Abschnitt gegen die Unterschrift seines Validators prüfen; Wurzel, Proben,
    ehrliche Vorlage und alles gegen die Kette.
  - Bindung, Wahl und Vorlage unterschreiben.
- **Prüfvektor des Knotens** (`testdata/quanten_s_inhaberin_vektor.json`,
  `quanten_s_inhaberin_vektor_test.go`):
  - Feste Test-Schlüssel, Werte und Blätter; die Chiffrate für die Inhaberin einmal
    erzeugt und gespeichert.
  - Go liest sie zurück, prüft jede Unterschrift und rechnet alle übrigen Felder neu. Die
    App rechnet denselben Vektor nach.
  - Ergebnis: dasselbe x aus denselben Chiffraten und byte-gleiche Unterschriften.
- **Teil 2a, Ablauf** (`lib/quantenSInhaberin.ts`, App #88): neu, abholen, einlösen,
  vergessen. Speicherung im SecureStore (höchstens 2048 Byte je Wert, darum x und y als
  Hex). Eine neue Bestellung wird abgewiesen, solange eine alte besteht, damit kein
  bezahlter Token verloren geht.
- **Teil 2b, Bildschirm** (`app/quanten-token.tsx`, App #89, gemergt ba77ee5b): Bestellung als
  QR-Code (und zum Kopieren, sie enthält nichts Geheimes), K und Validatoren eingeben,
  abholen, Stelle 0 oder 1 wählen. Die Vorlage gibt es **nur als QR-Code**, nicht in der
  Zwischenablage, mit dem Hinweis, dass sie bei Stelle b wie Bargeld wirkt. Texte in 12
  Sprachen, 5 Bildschirm-Tests, 1010 App-Tests und tsc grün.
- **Noch nicht:** in keinem ausgelieferten Build (Bauen und Veröffentlichen gehen nur von
  Hand); die Annahmestelle hat noch keinen QR-Leser (das Werkzeug liest die Vorlage von
  der Standardeingabe); im Hauptnetz ist die App noch keinen Weg gegangen.
- **Verfügbarkeit, unverändert:** Die Stelle braucht zum Laden ohnehin jeden
  präparierenden Validator (Öffnung); jetzt auch für die Blätter.

### R5: getrennte Rollen im Hauptnetz, Messung versiegelt (20.11.2026, gebaut)

- **Weg `rollen`** in `quanten-s-ausgabe.yml`: Die Inhaberin ist nicht der Lauf,
  sondern z. B. die App. Ihre Bestellung (öffentlich: n, ihre Basen y, ihre Adresse
  und ihr öffentlicher Schlüssel, die zwei Annahmestellen) kommt als Eingabe.
  - Messen auf IBM in genau ihren Basen y.
  - C1 gibt mit `quanten-s-ausgabe --fuer-inhaberin` aus. Die Zusammenfassung nennt
    K und die Validatoren-Zeile; damit holt die App den Token ab.
  - Binden, wählen und die Vorlage unterschreiben tut nur die App.
  - Prüfung der Bestellung im Workflow: genau diese Felder, ein Abschnitt, n = 2 k
    zur Toleranz, nur Bits. Das Werkzeug prüft danach streng (Schlüssel passt zur
    Adresse, Stellen verschieden, nicht die Inhaberin, nicht der Ausgeber).
- **Einlösen an C1** (`quanten-s-annahme.yml`): C1 ist Annahmestelle mit dem
  Schlüssel seines Knotens. Eingabe ist die Vorlage aus der App (der Text im
  QR-Code). Vorab prüft der Workflow, dass C1 die Stelle b dieses Tokens ist; alles
  Weitere prüft `quanten-s-annahme` (R3).
  - Die Vorlage wird damit öffentlich. Das ist für C1 ohne Gefahr: Anmelden kann nur
    C1 selbst, der Wert geht an C1, und an der anderen Stelle besteht sie nicht.
- **Messung versiegelt** (Wege `abschnitte-v2` und `rollen`): Die Messung (u, t, y,
  x) ist die Präparation des Tokens. Wer sie hat und zugleich Inhaberin ist, könnte
  an beiden Stellen bestehen. Bisher ging sie im Klartext als Job-Ausgabe durch
  GitHub.
  - Jetzt versiegelt der Mess-Job sie mit `forschung/quantengeld/s_versiegeln.py`
    auf den Schlüssel von C1 (ECIES wie go-ethereum). Den Schlüssel holt er vom
    Validator-Dienst und prüft, dass er zur festgelegten Adresse von C1 gehört
    (Keccak-256).
  - Nur das Chiffrat verlässt den Mess-Job. Auf C1 öffnet der Knoten es selbst
    (`aequitasd quanten-s-messung-oeffnen`, Schlüssel aus dem Container) und legt
    es selbst in sein Messverzeichnis: erst unter eigenem Zufallsnamen (O_EXCL,
    0600), dann per Link auf den Namen der Basen, nie überschrieben, keinem Verweis
    gefolgt. Root legt im Verzeichnis des Knotenbenutzers nichts an und ändert
    nichts (Härtung aus der Sicherheitsprüfung von #608).
  - Geprüft beim Öffnen: richtiger Schlüssel, unverändert, Domäne, Empfänger = C1,
    genau die sechs Felder, Bits in der Länge n, Basen passen zum Namen, Größe.
  - Belege: Prüfvektor in beide Richtungen (`testdata/quanten_s_versiegelt_vektor.json`
    aus Python, von Go gelesen; die Chiffrate der Inhaberin aus Go, von Python
    gelesen); 27 Missbrauchsfälle im Knoten und 5 am Befehl; 13 Mutationen im Go-Teil und 10 im Python-Teil
    rot. Das Ablage-Skript `deploy/quanten-s-ausgabe-abschnitt.sh` mit einem
    nachgebauten Container getestet (Gutfall, fremder Schlüssel, verändert, falsche
    Basen, falsches n).
- **Noch nicht gelaufen.** Dafür braucht es den C1-Deploy dieses Stands (neuer Befehl
  im Knoten) und deine Freigaben `quantenhardware` und `produktion`.
- **Ehrlich:** C1 ist weiter einziger Validator und kennt die Messung. Versiegeln
  nimmt sie GitHub und dem Workflow-Protokoll weg, nicht C1. Gegen C1 hilft erst
  ein zweiter Mensch als Validator (A5).

### R6: die App als Annahmestelle (20.11.2026, gebaut)

- **Zweck:** Eine zweite Person nimmt den Token der Inhaberin an. Sie scannt deren
  Vorlage (QR-Code), prüft alles selbst und löst ein. Der Wert geht an ihr Konto.
  Näher an „Quantengeld von Hand zu Hand“ kommt man heute nicht: Übergeben wird
  kein Quantenzustand, sondern das Ergebnis einer Quantenmessung, das nur die
  Inhaberin (und der präparierende Validator) kennt und das nur einmal und nur an
  einer der zwei Stellen besteht.
- **Rechnung** (Aequitas-App `lib/quantenSStelle.ts`) wie `quantenSAnnahmeLauf`:
  - Stand und Stelle b auf der Kette prüfen.
  - Vorlage von der Inhaberin auf der Kette unterschrieben.
  - Blätter vom Validator (seine Unterschrift) gegen die Wurzel.
  - Öffnung erst nach der Finalität, auf den Schlüssel dieses Kontos.
  - Jeder Wert gegen sein Blatt und das Stellen-Bit.
  - Stichprobe und Salze gegen die Bindung der Ausgabe.
  - Vorab-Urteil wie die Kette, dann anmelden. Weicht das Urteil ab: BEFUND.
- **Prüfvektor des Knotens** (`testdata/quanten_s_stelle_vektor.json`,
  `quanten_s_stelle_vektor_test.go`):
  - Form des Hauptnetzes: ein Validator, n = 1.104, Toleranz 2 %, k = 552.
  - Ehrliche Vorlage mit 2 Messfehlern bei 290 Treffern.
  - Go prüft jeden Schritt mit den Funktionen des Knotens. Die App liefert
    dieselbe Anmelde-Nachricht, dieselbe Unterschrift und denselben Body.
- **In der App** (App #91):
  - Karte „Annahmestelle“: eigene Kennung als QR, Vorlage scannen und einlösen.
  - „C1 eintragen“ (Schlüssel vom Validator-Dienst, muss zur Adresse gehören) und
    „Mein Konto eintragen“ in der Bestellung.
  - 16 Angriffe brechen vor dem Anmelden ab; 20 Mutationen rot.
- **Ehrlich:** Die Stelle muss bei der Bestellung feststehen (eine der zwei
  Stellen). Weitergeben an eine dritte Person geht nicht. Das ist gewollt: Eine
  kopierte Vorlage ist an keiner anderen Stelle etwas wert.

### R7: Nachweis nach dem Ende (20.11.2026, gebaut)

Auftrag des Betreibers zu „heute weltweit unmöglich“: „so gut wie möglich umsetzen“.
Ein Cloud-Lauf lässt sich Dritten gegenüber nicht als echt beweisen. Was sich
beweisen lässt: dass genau diese Präparation bei der Ausgabe festgelegt war, in
welchen Basen gemessen wurde und wie viele Messfehler es gab. Das macht dieser
Schritt öffentlich nachrechenbar.

- **Wann:** erst, wenn der Token eingelöst oder tot ist **und** dieses Ende final ist.
  Vorher bleibt die Präparation geheim (wer sie kennt und Inhaberin ist, könnte an
  beiden Stellen bestehen). Für einen lebenden Token gibt der Dienst nie etwas
  heraus.
- **Was** (je Validator, von ihm unterschrieben, `GET
  /api/quanten-w/validator/nachweis?k=`):
  - die gesalzenen Werte seines Abschnitts (daraus Basis u, Wert t und Stellen-Bit
    jeder Stelle);
  - die Salze der Stichprobe v2;
  - die Messergebnisse x der Inhaberin (sie standen schon in der Anmeldung auf der
    Kette);
  - Bindung d und Wahl c, wie er sie auf der Kette gesehen hat, und die Stelle b der
    letzten Anmeldung. Daraus folgen die Basen, in denen die Kette geprüft hat
    (d ⊕ b ⊕ c).
- **Prüfen** (`aequitasd quanten-s-nachweis --k K --validatoren … --wurzel …
  --proben …`; die Annahmestelle mit `quanten-s-annahme --nachweis-warten`):
  - Unterschrift jedes Validators, Abschnitte lückenlos;
  - aus den Werten die Blätter und die Wurzel, aus den Salzen die Proben – gegen die
    Wurzel und Proben der Ausgabe, die die Stelle **vor** dem Anmelden von der Kette
    gelesen hat (sonst: BEFUND);
  - Treffer und Fehler über alle Stellen und an der gewählten Stelle.
  - Nennen Validatoren verschiedene Stellen, bleibt die Zählung nach Basen offen,
    statt falsch zu sein.
- **Im Workflow** `quanten-s-annahme.yml`: Die Stelle wartet nach dem Einlösen bis zu
  20 Minuten auf den Nachweis und prüft ihn. Der rohe Nachweis kommt als Artefakt in
  den Lauf, das Ergebnis in die Zusammenfassung.
- **Auch im Weg `abschnitte-v2`** (`quanten-s-ausgabe.yml`, das Werkzeug spielt alle
  Rollen): `quanten-s-ausgabe --stichprobe-v2 --nachweis-warten 20m`. Ein Lauf mit den
  Freigaben `quantenhardware` und `produktion` erzeugt damit einen öffentlichen Nachweis
  im Hauptnetz (Artefakt `nachweis-K`). Auf vier echten Knoten belegt
  (`TestVierKnotenQuantenSAbschnitte_RealDB` mit Stichprobe v2: alle 4 legen offen, 0
  Fehler bei 56 Treffern an der Stelle).
- **Im Hauptnetz (21.11.2026, `HEUTE.md` 3.3k):** Lauf 44904131085, IBM `ibm_kingston`:
  Token `a5acd868…ab0f` eingelöst, Doppelvorlage abgewiesen. C1 legte nach dem Ende
  offen; die Präparation ergibt Wurzel und Proben der Ausgabe, 0 Fehler bei 540 Treffern
  (an der Stelle 0 bei 283).
- **Nachweis sichern:** Der Validator hält ihn nur im Speicher (höchstens 64, nach einem
  Neustart weg). `quanten-s-nachweis.yml` holt ihn ohne Schlüssel und Freigabe über die
  öffentliche Adresse, rechnet ihn nach und legt ihn als Artefakt ab (90 Tage).
- **Belegt:**
  - in-process: nach dem Ende, erst nach der Finalität, nie für einen lebenden
    Token; Grenze 64 im Speicher; Endpunkt begrenzt je Absender.
  - Fälschungen abgewiesen: Wert oder Salz verändert, neu unterschrieben, fremde
    Unterschrift, vertauschte Reihenfolge, fehlender Nachweis, andere Wahl,
    verkürzte Bindung.
  - Alle Validatoren erfinden zusammen eine andere Präparation: BEFUND gegen die
    Wurzel der Ausgabe.
  - **Vier echte Knoten** (`TestVierKnotenQuantenSRollen_RealDB`): alle 4 legen
    offen, die Präparation ergibt die Wurzel der Ausgabe, und an der Stelle zählt der
    Nachweis genau das, was die Annahmestelle gezählt hat (0 Fehler bei 59 Treffern).
- **Ehrlich:**
  - Das beweist nicht, dass ein Quantenrechner gemessen hat. Ein Betreiber, der
    Präparation und Messung selbst erfindet und sie bei der Ausgabe festlegt, fällt
    damit nicht auf. Dagegen helfen nur mehrere unabhängige Validatoren, von denen
    jeder seinen Abschnitt in seinem eigenen IBM-Auftrag präpariert (Weg A, A5).
  - Die IBM-Job-Kennung steht bei der Ausgabe auf der Kette; prüfen kann sie nur,
    wer Zugang zu dem IBM-Konto hat.
  - Der Nachweis liegt nur im Speicher des Dienstes (höchstens 64, nicht über einen
    Neustart). Dauerhaft ist das Artefakt des Workflows.
  - An die Kette gebunden sind Werte und Salze (über Wurzel und Proben). Bindung d,
    Wahl c, Stelle und x sind Angaben des Validators, von ihm unterschrieben, aber
    nicht gegen die Kette geprüft. Ein Validator könnte damit Treffer und Fehler
    falsch melden, nicht die Präparation.
  - Aus der Sicherheitsprüfung (#611, nichts Ausnutzbares), als Restrisiko notiert:
    - Die Sichtung des Endes hängt am Token, nicht am Block des Endes. Ein Umbau des
      DAG im Finalitätsfenster, der das Ende in einen späteren Block schiebt, ohne
      dass ein Takt den Token dazwischen lebend sieht, könnte früher offenlegen.
      Ausnutzbar wäre das nur mit einem zweiten Umbau, der das Ende ganz entfernt.
      Dasselbe Muster haben die bestehenden Sichtungen.
    - W-Noten und S-Token teilen sich die Kennungen K.
    - Steht die Finalität still, wird ein beendeter Eintrag nach dem Prägefenster
      ohne Nachweis vergessen (seit diesem PR, getestet).
    - Scheitert der Nachweis nach einem erfolgreichen Einlösen, endet
      `quanten-s-annahme` mit Code 3 („eingelöst, Nachweis nicht bestätigt“), nicht
      als Fehlschlag des Einlösens.

### R8: die Basen y je Validator verschlüsselt (21.11.2026, gebaut)

Bisher stand y der Inhaberin im Klartext in der Bestellung. Der Ausgeber reichte es an
die Validatoren weiter. Mit d = y ⊕ z auf der Kette kannte er z, und aus c die gewählte
Stelle, bevor die Inhaberin vorlegte. Er wusste also, **wo** sie einlösen will (nicht x).

- **Was neu ist** (`x/humanity/keeper/quanten_s_basen_chiffrat.go`):
  - `quanten-s-inhaberin neu --verschluesseln adr=api,…`: Die Inhaberin holt bei jedem
    Validator den öffentlichen Schlüssel und prüft ihn gegen die Adresse. Dann
    verschlüsselt sie y seines Abschnitts an ihn (ECIES wie die Bündel).
  - In der Bestellung stehen nur die Chiffrate (`basen_verschluesselt`). y bleibt in
    ihrer Datei (0600).
  - Der Ausgeber reicht das Chiffrat weiter: in der Vormerkung (Abschnitt 15) und im
    Abschnitts-Auftrag. Er unterschreibt dessen Hash mit.
  - Der Validator öffnet es mit seinem eigenen Schlüssel, misst bzw. präpariert in
    genau diesem y und legt die Vormerkung unter diesem y ab.
- **Gebunden:** Im Chiffrat stehen Domäne, Kette, Empfänger, n, v, Nummer, der Hash der
  ganzen Validatorenliste und die Inhaberin. Der Validator prüft jedes Feld gegen den
  Auftrag.
  - Ein Chiffrat gilt nur bei dem Validator, für den es gemacht ist, nur für diesen
    Abschnitt und nur für diese Inhaberin.
  - Ohne die Bindung an die Inhaberin könnte ein Ausgeber ihr Chiffrat in einem
    eigenen Token verbrauchen und aus dessen Nachweis Teile von y lernen.
  - Ohne die Bindung an die Liste könnte er es in einem Auftrag wiederholen, der eigene
    Adressen als übrige Validatoren nennt, und bekäme Anteile und Zählungen zu ihren
    Basen (Restrisiko aus der Sicherheitsprüfung, im selben PR geschlossen).
- **Gegen Tausch:** Der Validator unterschreibt in seiner Antwort den Hash des
  Chiffrats, das er geöffnet hat. Die Inhaberin vergleicht ihn beim Annehmen und beim
  Abholen mit ihrem eigenen Chiffrat.
  - Setzt der Ausgeber eigene Basen ein (an denselben Validator verschlüsselt oder im
    Klartext), meldet sie BEFUND und bindet nicht.
  - Auch der Ausgeber prüft den Hash schon bei der Ausgabe.
- **Reihenfolge, fail-closed:** Geöffnet wird erst nach allen billigen Prüfungen, nach
  der Unterschrift des Ausgebers, der Deckung und dem Zeitfenster. Jede Abweichung ist
  ein Fehler, nie ein Rückfall auf Klartext. Klartext und Chiffrat zugleich wird
  abgewiesen.
- **Grenzen:** Ein Chiffrat hat höchstens 2 · (16.384 + 1.024) Hex-Zeichen (nur
  Kleinbuchstaben, gerade Länge). Der Inhalt wird streng gelesen (genau diese Felder,
  nichts danach). Der größte Auftrag passt in die 64 KiB des Endpunkts (Test).
- **Unverändert:** die Kette, das Format für Klartext-Bestellungen, die Nachrichten
  bisheriger Aufträge und Antworten. Der Zusatz `|basen:` steht nur bei Chiffraten.
- **Belegt:**
  - Gutfall über alle Rollen: Bestellung, Aufträge, Paket und Protokoll des Ausgebers
    enthalten kein y. Jeder Validator misst in genau dem y der Inhaberin. Annehmen,
    Abholen, Einlösen und Annahme laufen wie bisher.
  - Missbrauch abgewiesen, ohne zu messen:
    - Chiffrat an einen anderen Validator, für einen anderen Abschnitt, eine andere
      Validatorenliste oder eine andere Inhaberin;
    - Klartext und Chiffrat zugleich, Großbuchstaben, ungerade Länge, zu groß,
      verändert;
    - Chiffrat nach der Unterschrift getauscht, nicht unterschrieben.
  - Inhalt streng: falsche Domäne, Kette, Empfänger, n, v oder Validatorenliste, ein
    Feld zu viel oder zu wenig, falsche Bits, Daten nach dem JSON.
  - Tausch durch den Ausgeber (eigenes Chiffrat, Klartext): BEFUND beim Annehmen und
    beim Abholen. Ein Validator ohne Hash: der Ausgeber gibt nichts aus.
  - Gegenprobe: 22 Mutationen, alle rot.
- **Ehrlich:**
  - **Wer misst, kennt y.** Das bringt erst etwas, wenn der Validator auf seinem
    eigenen Server misst (A5, Vormerken). Im Hauptnetz misst heute ein GitHub-Lauf des
    Betreibers für C1, und dort ist C1 zugleich Ausgeber. Darum nimmt
    `quanten-s-ausgabe.yml` (Weg `rollen`) weiter nur Bestellungen im Klartext an.
  - Der Anbieter des Quantenrechners sieht y weiterhin (Abschnitt 4).
  - Die App bestellt noch im Klartext. Verschlüsseln in der App ist der nächste
    Schritt (dieselbe ECIES-Form; die App kann schon für die Inhaberin entschlüsseln).

### R9: Öffnung aus t Anteilen (21.11.2026, gebaut)

- **Problem:** Bei Stichprobe v2 lädt eine Annahmestelle ihre Stichprobe aus der Öffnung
  jedes präparierenden Validators. Nur er kennt die Werte seines Abschnitts und weiß,
  welche Stelle zu welcher Annahmestelle gehört (Stellen-Bit). Fällt er nach der Bindung
  und vor dem Laden aus, wartet die Stelle.
- **Warum nicht einfach die Anteile der anderen:** Die anderen kennen das Stellen-Bit
  nicht. Gäben sie der Stelle ihre Anteile an allen Stellen, lernte sie auch die
  Stichprobe der anderen Stelle. Damit wären die Stichproben nicht mehr getrennt
  (Selbsttest 5g).
- **Gebaut** (`quanten_s_anteil_oeffnung.go`):
  - Der Präparator x legt in sein Bündel an jeden anderen Validator j je Annahmestelle b
    ein **Paket**: die Anteile von j an genau den Stellen mit Stellen-Bit b, dazu das Salz
    des Teil-Hashes von b. Das Paket ist an den Schlüssel der Stelle b verschlüsselt und
    an K, b, x, j und den Abschnitt gebunden. Es liegt im unterschriebenen Bündel, der
    Ausgeber kann es weder lesen noch tauschen.
  - j kann das Paket nicht lesen. Es erfährt weder Werte noch die Aufteilung, die bis
    zur Bindung geheim bleibt (Befund 5i). Bei Stichprobe v2 bestätigt j nur Bündel mit
    genau zwei Paketen.
  - j hält die Pakete im Tresor und gibt sie erst heraus, wenn Bindung und Wahl auf der
    Kette stehen und final sind. Das prüft j selbst wie der Präparator
    (`GET /api/quanten-w/validator/anteil-oeffnung?k=&x=&b=`).
  - Antwortet x nicht, holt die Stelle die Pakete der anderen. Sie setzt die Werte aus t
    Paketen zusammen (Lagrange) und prüft jeden Wert gegen sein Blatt und sein
    Stellen-Bit. Ist ein Paket falsch, probiert sie weitere Teilmengen, höchstens 64.
    Danach folgen dieselben Prüfungen wie bisher: Wurzel, k_note, Bindung v2 aus den
    Salzen, Zählungen der Validatoren.
  - Bei der kompakten Vorlage holt die Stelle die Blätter bei jedem Validator. Fehlt
    einer, nennt ein anderer alle Blätter (`GET /api/quanten-w/validator/blaetter?k=`,
    erst, wenn der Token auf der Kette steht). Blätter sind Bindungen ohne Geheimnis; die
    Stelle prüft sie gegen die Wurzel auf der Kette.
  - Keine Kettenänderung; Stichprobe v1 und bisherige Nachrichten sind unverändert.
- **Grenzen:**
  - Wer t Validatoren hat, kennt den Token ohnehin. Weniger als t, die zu früh
    herausgeben, liefern weniger als t Anteile und damit nichts.
  - Bei v = 1 (Hauptnetz heute) gibt es keine anderen Validatoren. R9 wirkt erst mit A5.
  - Die Bündel werden größer: je Stelle kommt ein Anteil dazu, verschachtelt
    verschlüsselt, grob das 2,5-Fache. Die Grenze für die Bestätigung (2 MiB) bleibt.
    Damit sinkt das größte mögliche n·v, für n = 1104 bei wenigen Validatoren reicht es.
  - Alle Validatoren müssen diese Fassung haben, bevor ein Präparator Pakete mitschickt.
    Eine ältere Fassung weist Bündel mit dem neuen Feld ab (fail-closed).
- **Belegt:**
  - Gutfall: Präparator 1 oder 3 fällt nach der Bindung aus, die Ausgabe läuft durch
    (Doppelvorlage abgewiesen, ehrliche Vorlage eingelöst).
  - Vier echte Knoten über HTTP (`TestVierKnotenQuantenSRollenAusfallVorDemLaden_RealDB`):
    Knoten 2 fällt nach der Bindung und vor dem Laden aus. Die Stelle lädt seinen
    Abschnitt aus den Paketen der drei anderen und die Blätter von einem anderen
    Validator. Doppelvorlage abgewiesen, ehrlich eingelöst (0 Fehler bei 62 Treffern).
  - Ohne Ausfall fragt niemand nach Paketen.
  - Nach jedem Schritt der Ausgabe fragt ein Prüfnetz jeden Validator nach jedem Paket:
    Vor Bindung und Wahl gibt keiner eines heraus, ohne Finalität auch nicht.
  - Missbrauch abgewiesen:
    - nur t − 1 lesbare Pakete (die anderen für die falsche Stelle): kein Laden, keine
      Vorlage;
    - ein Bündel ohne Pakete bei v2 (selbst unterschrieben): keine Bestätigung;
    - beim Lesen 13 Fälle: fremde Stelle, anderes K, andere Stelle b, anderer
      Präparator, anderer Validator, anderer Abschnitt, Stelle außerhalb oder mit
      führender Null, Anteil ≥ P, Großbuchstaben, Salz, Übergröße;
    - beim Zusammensetzen: ein falscher Anteil unter vier Paketen (die ehrliche
      Teilmenge wird gefunden), einer unter drei (abgewiesen), doppelter Validator,
      andere Stellen oder anderes Salz;
    - Bündel mit einem, drei, zu großen oder falsch geformten Paketen.
  - Gegenprobe: 19 Mutationen, 17 rot. Zwei doppelte Absicherungen überleben, weil eine
    spätere Prüfung dasselbe abfängt: Präparator gleich sich selbst (dafür gibt es kein
    Paket im Tresor) und das Stellen-Bit beim Zusammensetzen (prüft danach
    `quantenSOeffnungLaden`).

### R10: eindeutiges Urteil – eingelöst oder tot (22.11.2026, gebaut, Stichtag 22.11.2026 13:00 UTC; der erste Stichtag 09:30 UTC wurde verpasst)

- **Befund:** Endet ein Token, legt die Kette ihn in beiden Fällen (eingelöst oder tot nach
  drei falschen Vorlagen) unter `Tote` ab. Die Ansicht antwortet für beide nur
  `{"tot": true}`. Das Urteil stand nur in der entscheidenden Offenlegung (`urteil: "echt"`
  mit `wert_an` an die Stelle, oder `"abgewiesen"` mit dem Wert an das Grundeinkommen).
  - Die Werkzeuge schlossen deshalb aus der Zahl der Fehlversuche: Nach 0 oder 1 kann ein
    verschwundener Token nur eingelöst sein. Beim letzten Versuch ging das nicht mehr, also
    meldeten sie dann gar nicht erst an. Ein Token mit zwei Fehlversuchen ließ sich mit den
    Werkzeugen nicht mehr einlösen.
  - Außerdem merkte sich die Kette einen Token nur, wenn er höchstens
    `quantenWToteBehalten` (gut 8 Tage) nach der Ausgabe endete. Später verschwand er
    spurlos und sah aus wie „nie ausgegeben“.
- **Gebaut** (`quanten_w_urteil.go`, Konsensregel ab Stichtag):
  - Jeder Knoten schreibt beim Ende selbst `urteil` (`eingeloest` oder `tot`) und `ende`
    (Blockzeit) in den Eintrag unter `Tote`, aus dem Block, den er nachrechnet. Das gilt
    immer, auch bei langer Laufzeit, für Variante S und W.
  - Aufgeräumt wird `quantenWToteBehalten` nach dem späteren von Ausgabe und Ende. Die
    Grenze `quantenWToteHoechstens` bleibt (voll heißt keine neue Ausgabe, fail-closed).
  - Ansicht: Beendete Token zeigen `urteil` und `ende`, lebende `urteil_eindeutig: true`.
  - Werkzeug und App melden beim letzten Versuch nur an, wenn `urteil_eindeutig` steht.
    Danach gilt nur das Urteil der Kette. Fehlt es, behaupten sie keinen Erfolg.
  - Vor dem Stichtag ist alles byte-gleich: Die Felder fehlen, die StateRoot ist dieselbe.
    Lesen ist fail-closed: Erlaubt ist nur `eingeloest`/`tot` mit einer Endzeit oder beides
    leer.
  - Stichtag `quantenWUrteilAbUnix`, Status `quanten_w.urteil_ab`, Deploy-Wächter
    `.github/neustart/quanten-urteil-nicht-verpasst.sh` (mindestens 30 min Vorlauf, nie
    rückwirkend). Ein älterer Knoten rechnete nach dem Stichtag eine andere StateRoot; darum
    müssen alle Validatoren vorher diese Fassung haben (heute nur C1).
- **Belegt:** `quanten_w_urteil_test.go`, jeweils mit annehmendem und nachspielendem Knoten:
  - schlafend wie bisher: kein Urteil, Kodierung ohne die Felder;
  - aktiv: eingelöst und tot unterscheidbar;
  - spätes Ende: schlafend spurlos, aktiv mit Urteil, aufgeräumt erst nach dem Ende;
  - Register lesen: 6 Fehlformen abgewiesen, Rundweg byte-gleich;
  - W-Note eingelöst;
  - Werkzeug (10 Fälle); App 43 Tests.

### Ehrlich dazu (offen)

- **Der Ausgeber sieht die Basen y**, wenn die Inhaberin im Klartext bestellt (heute die
  App und der Hauptnetz-Workflow). Mit d auf der Kette kennt er damit z, und aus c die
  gewählte Stelle, bevor die Inhaberin vorlegt. Das verrät, *wo* sie einlösen will,
  nicht x. Einlösen oder binden kann er nicht. Mit `--verschluesseln` (R8) sieht er y
  nicht mehr; das wirkt, sobald ein Validator selbst misst.
- **Verfügbarkeit:** Bei Stichprobe v2 lädt eine Stelle ihre Stichprobe aus der Öffnung
  **jedes** präparierenden Validators. Nur er kennt die Werte seines Abschnitts; die
  Anteile der anderen reichen zum Offenlegen auf der Kette, nicht zum Laden. Fällt einer
  nach der Bindung und vor dem Laden aus, muss die Stelle warten, bis er zurück ist. Der
  Wert ist nicht verloren, denn die Anmeldung kommt erst nach dem Laden. Im Pilot fällt
  der Validator deshalb nach dem Laden aus. Seit R9 lädt die Stelle aus t Paketen der
  anderen; das wirkt erst ab mehreren Validatoren (A5).
- **Hat ein Token schon zwei Fehlversuche,** meldet das Werkzeug nur an, wenn die Kette das
  Urteil schreibt (R10, ab dem Stichtag); vorher wie bisher nicht.
- **Getrennte Schlüssel heißen noch nicht getrennte Menschen.** Im Pilot laufen alle
  Rollen im selben Testprozess. Im Hauptnetz ist C1 einziger Validator und kennt den
  ganzen Token (Abschnitt 16). Den Token der Inhaberin einlösen kann C1 trotzdem nicht,
  denn ohne ihre Unterschrift gibt es keine Vorlage. Mit ihr zusammen fälschen könnte er.
  Dagegen hilft erst A5 (ein zweiter Mensch als Validator).
- Es liegt kein Gutachten vor.

## 18. Weg B: Präparieren per App, Spezifikation (B1, 22.11.2026)

**Ziel:** Die Hürde für Dezentralität sinkt von „einen Server betreiben“ auf „ein Handy und
ein kostenloses Konto bei einem Quantenrechner“. Jeder registrierte Mensch kann sich als
Präparator melden. Je Token **lost die Kette** v Präparatoren aus. Jeder präpariert seinen
Abschnitt in seinem eigenen Auftrag und hält Anteile der anderen, genau wie ein Validator
in Weg A. Der Ausgeber wählt niemanden.

**Spezifikation:** `s_weg_b.py` (Selbsttest im CI). Sie setzt auf `s_kette.py` auf;
Präparation, Anteile, Stichprobe v2, Stellen-Bit und R9 bleiben unverändert.

### Regeln (neu gegenüber Weg A)

1. **Register:** nur aktive Menschen aus dem Menschenregister, je Mensch höchstens ein
   Präparator. Im Los ist nur, wer mindestens `MELDE_VORLAUF` Blöcke vor der Bestellung
   gemeldet war. Abmelden nimmt aus dem Los, aber nur für Bestellungen nach der Abmeldung:
   Für das Los zählt der Stand **zur Bestellung**, nicht zum Losen. Sonst könnte, wer die
   drand-Runde schon kennt, sich vor dem Los abmelden und es so verschieben (Angriffstest im
   Selbsttest; die Kette behält Abmeldungen mit Zeitpunkt).
2. **Los:** Die Bestellung wird auf Höhe h endgültig. Gelost wird mit dem Zufall der Höhe
   h + `LOS_ABSTAND` aus dem Abbild des Registers auf Höhe h: die v Konten mit dem
   kleinsten H(Zufall, K, Konto). Jeder Knoten rechnet das Los nach.
3. **Zufall:** aus Beiträgen mehrerer (Zusage, dann Offenlegung; mindestens zwei).
   Kennt ihn einer vorher, kann er durch Probieren vieler K t Komplizen erzwingen. Der
   Selbsttest zeigt genau das als Gegenprobe.
4. **Ausgabe:** Es liefern genau die Gelosten in Losreihenfolge. Ein fremdes Konto, eine
   vertauschte Reihenfolge oder ein fehlender Präparator werden abgewiesen.
5. **Einlösen mit Handys:** Die Stelle lädt ihre Stichprobe von jedem erreichbaren
   Präparator, für die anderen aus t Anteilen (R9). Sind weniger als t erreichbar,
   wartet die Einlösung; der Wert geht nicht verloren.

### Zahlen (`python3 -I s_weg_b.py --tabelle`, 200 Gemeldete, davon 20 % böse)

| t von v | Stichprobe k | P(Fälschung möglich) | einlösbar, p = 0,8 | einlösbar, p = 0,6 |
|---|---|---|---|---|
| 3 von 4 | 5.200 | 2,6 % | 81,9 % | 47,5 % |
| 4 von 7 | 3.472 | 3,1 % | 96,7 % | 71,0 % |
| 5 von 9 | 5.200 | 1,7 % | 98,0 % | 73,3 % |
| 6 von 11 | 5.200 | 0,95 % | 98,8 % | 75,4 % |
| 7 von 13 | 5.200 | 0,53 % | 99,3 % | 77,1 % |

„P(Fälschung möglich)“ heißt: Unter den Gelosten sind mindestens t Böse. Nur dann kann
die Inhaberin mit ihnen an zwei Stellen durchkommen, und nur bei diesem einen Token.
p ist die Wahrscheinlichkeit, dass ein Handy in der Frist erreichbar ist.

**Kontingent:** Das kostenlose IBM-Konto hat 10 Minuten je 28 Tage. Bei geschätzt 10 s je
Auftrag sind das 60 Abschnitte je Mensch und Monat. Die 10 s sind eine Schätzung und
werden im ersten Lauf gemessen.

### Selbsttest und Gegenproben

Der Selbsttest prüft: Register (ein Mensch, ein Präparator; ohne Menschen; Vorlauf;
Abmelden), Zufall (ein Beitrag, falsche Offenlegung), Los (nachrechenbar, hängt vom
Zufall ab, nicht vor dem Zufall, nicht zweimal, Register zu klein, v außerhalb der
Grenzen), Ausgabe nur durch die Gelosten, ehrliches Einlösen mit 3 von 7 Handys offline
(R9), Verschworene unter t plus Inhaberin kommen an keiner zweiten Stelle durch
(Gegenprobe: t vom Ausgeber gewählte Komplizen kommen durch), Probieren vieler K mit
unbekanntem Zufall bleibt bei der Erwartung (Gegenprobe: mit bekanntem Zufall nicht),
Zahlen. **8 Mutationen** machen ihn rot: ein Mensch zweimal, Vorlauf aus, Los vor dem
Zufall, Ausgabeprüfung als Menge, R9 mit t − 1, Zusage ungeprüft, Los ohne Zufall,
R9-Schwelle aus.

### Ehrlich dazu

- **Die Zahlen reichen noch nicht für echtes Geld.** Bei 20 % Bösen gelingt bei 4 von 7
  etwa jede 33. Fälschung. Dagegen helfen größere v (7 von 13: etwa jede 190.), ein
  **Pfand** je Präparator, das bei nachgewiesener Fälschung verfällt (Doppelvorlage auf
  der Kette mit beiden Stellen bestanden), und eine Obergrenze des Werts je Token. Das
  Pfand gehört in B2.
- **Zufall:** Heute erzeugt C1 allein die Blöcke. Ohne Beiträge mehrerer könnte C1 das
  Los steuern. Weg B braucht darum den Zufall aus Zusage und Offenlegung (wie W3), und
  wer zuletzt offenlegt, kann zurückhalten und so eine neue Losung erzwingen. Die Kette
  muss das begrenzen.
- **Anbieter:** Nutzen alle IBM, sieht IBM jeden Abschnitt (wie Weg A).
- **Messung:** Die Inhaberin muss dem Präparator ihr x glauben. Stört er, fällt das auf
  seinen Abschnitt zurück (`fehler_je_abschnitt`); fälschen kann er allein nicht.
- **Handys:** Ein Handy muss nach der Bindung einmal erreichbar sein (Öffnung oder
  Anteile). Dafür braucht es einen Briefkasten bei den Knoten (verschlüsselt, nur
  Ablegen und Abholen) und Hintergrundabfragen in der App.

### B2-Entwurf: woher der Zufall für das Los kommt

Den Zufall dürfen weder Ausgeber noch Inhaberin bestimmen, denn genau sie wären bei einer
Fälschung Komplizen. Auch C1 als einziger Blockerzeuger scheidet aus. Geprüfte Wege:

| Weg | Wer bestimmt mit | Bewertung |
|---|---|---|
| Blockhash von h + Δ | der Blockerzeuger (heute C1 allein) | **ungeeignet** |
| Zusage/Offenlegung der Inhaberin + Blockhash | Inhaberin und Blockerzeuger, die zusammen fälschen wollen | **ungeeignet** |
| Zusage/Offenlegung der Gemeldeten | die Gemeldeten selbst; wer zuletzt offenlegt, kann zurückhalten | möglich, aufwendig (Fristen, Pfand fürs Zurückhalten) |
| **drand** (League of Entropy, Runde nach h + Δ) | ein Schwellenverbund unabhängiger Organisationen | **empfohlen** |

**drand:** Die Runde r wird erst nach der Bestellung erzeugt (die Kette verlangt, dass ihre
Zeit nach der Blockzeit der Bestellung plus Abstand liegt). Wer das Los einreicht, legt
Runde und Signatur bei. **Jeder Knoten prüft die BLS-Signatur selbst** gegen den
öffentlichen Schlüssel des Verbunds, der fest im Knoten steht. Der Knoten holt nichts aus
dem Netz und glaubt keinem Server; eine falsche Signatur wird abgewiesen. Zufall =
SHA-256 der Signatur. Bibliothek: `gnark-crypto` (BLS12-381, schon im Projekt, wird
direkte Abhängigkeit). Verzerren könnte das Los nur eine Mehrheit des Verbunds, und auch
die nur, wenn sie mit Ausgeber und Inhaberin zusammenarbeitet.

Ehrlich: Das ist eine Abhängigkeit von einem externen Verbund. Fällt drand aus, wird
nicht gelost und nicht ausgegeben (fail-closed); ausgegebene Token sind nicht betroffen.

### B2a: Register der Präparatoren (22.11.2026, gebaut, schlafend)

`x/humanity/keeper/quanten_s_praeparatoren.go`. Zwei neue Arten im W-Modul, beide **schlafend**
(`quantenSWegBAbUnix` = MaxInt64; vorher im Block unbekannt, Endpunkt 409, Register und
StateRoot byte-gleich):

- `quanten_w_s_praep_melden` (Wallet = Konto, unterschrieben): nur ein Konto, das jetzt
  `is_human` ist. Ein Mensch hat genau eine Wallet mit `is_human` (Nullifier), also höchstens
  einen Eintrag im Abbild. Meldezeit = Buchzeit des Knotens, nicht die unterschriebene Zeit.
- `quanten_w_s_praep_abmelden` (Wallet = Konto, unterschrieben): der Eintrag bleibt mit
  Abmeldezeit stehen (Stand zur Bestellung zählt). **Ohne Wallet** räumt jeder einen Eintrag
  auf, dessen Konto kein Mensch mehr ist (nach einem Umzug auf eine neue Wallet); ein Mensch
  kann so nicht abgemeldet werden.
- Grenzen: höchstens 256 Einträge; Abgemeldete werden `quantenWToteBehalten` (gut 8 Tage)
  nach der Abmeldung vergessen, erst dann darf das Konto neu melden. So lässt sich keine alte
  Melde-Unterschrift wieder einspielen (dieselbe Rechnung wie bei den Toten).
- Abbild zur Zeit T (`quantenSPraepAbbildLocked`, für B2b): gemeldet spätestens T − 1 Tag,
  zu T nicht abgemeldet, zu T ein Mensch. B2b rechnet es **in der Bestellung** und hält es
  fest; eine Bestellung muss verfallen, bevor ein Eintrag aus ihrem Abbild vergessen wird.
- Lesen: `GET /api/quanten-w/praeparatoren` (begrenzt wie die anderen Lese-Endpunkte).

Abweichung von `s_weg_b.py`: Zeiten statt Blockhöhen (das W-Modul rechnet mit der Buchzeit).
Ehrlich: den Vorlauf kann der einzige Blockerzeuger heute um bis zu 7 Tage verkürzen
(Buchzeit zurückdatiert). Das schadet dem Los nicht, denn sein Zufall kommt aus einer
drand-Runde nach der Bestellung.

Tests: Gutfall, Schlafen, Missbrauch beim Melden (kein Mensch, fremder Schlüssel, alte
Unterschrift, K eines anderen, fremde Felder, Wiedereinspielen nach dem Vergessen), Abmelden
(fremdes Konto, ohne Wallet bei einem Menschen), Umzug, volles Register, strenges Lesen,
echte Datenbank, nicht kanonische Wallet. 13 Mutationen machen sie rot.

### B2b: Bestellung, Los, Ausgabe gegen das Los (23.11.2026, gebaut, schlafend)

`x/humanity/keeper/quanten_s_bestellung.go`, zwei neue Arten (schlafend wie B2a):

- `quanten_w_s_bestellen` (Ausgeber, unterschrieben; K, v = `praep_zahl`, t): nur ein
  Mensch, eine offene Bestellung je Ausgeber, höchstens 32. Die Kette rechnet das Abbild
  **in der Bestellung** und hält es fest. Die Buchzeit darf höchstens 120 s vor dem Block
  liegen (sonst wäre die Runde des Loses schon bekannt).
- `quanten_w_s_losen` (jeder, ohne Wallet): die **feste** Runde = erste drand-Runde mit Zeit
  ≥ Bestellzeit + 10 min, ihre Signatur prüft jeder Knoten selbst. Los = `quantenSLos`.
- `quanten_w_s_ausgeben` für ein bestelltes K: nur der Ausgeber der Bestellung, t wie
  bestellt, Validatoren = Los in Losreihenfolge, jeder Geloste bestätigt mit dem Schlüssel
  seines Kontos. Weg A ist für ein bestelltes K gesperrt. Danach ist die Bestellung verbraucht.
- Frist 2 Tage für Losen und Ausgabe; eine verfallene Bestellung sperrt ihren Ausgeber bis
  zum Ende der Frist.

**Befund (neu, in `s_weg_b.py` 7b):** Je Los ist der Zufall unverzerrt, aber ein Ausgeber
kann nach dem Los **abbrechen und neu bestellen**, bis ein Los t Komplizen trägt:
1 − (1 − p)^m nach m Versuchen (4 von 7, 20 % Böse: p ≈ 2,5 %, nach 40 Versuchen ≈ 63 %).
Die Grenze „eine Bestellung je Mensch und Frist“ bremst das nur (40 Versuche ≈ 80 Tage je
Mensch). **Weg B wird erst scharf, wenn ein Pfand je Bestellung beim Abbruch verfällt
(B2c).**

Ehrlich: Solange C1 allein Blöcke erzeugt, kann C1 die Blockzeit hinter der echten Zeit
halten (Knoten prüfen nur „nicht in der Zukunft“ und „nicht vor den Eltern“). Dann läge die
feste Runde schon in der Vergangenheit. Das ist von außen sichtbar (Blockzeit gegen
drand-Zeit); ein Wächter dafür gehört vor den Stichtag.

Tests: Gutfall, Abbild fest (Vorlauf, Abmeldung und Meldung danach), Missbrauch bei
Bestellen, Losen (andere Runde echt signiert, fremder Verbund, gekippte Signatur, doppelt,
nach der Frist) und Ausgabe (vor dem Los, Fremder, Reihenfolge, einer fehlt/zu viel,
fremder Schlüssel, fremder Ausgeber, nach der Frist), Buchzeit, echter quicknet-Beacon
(Runde 1.000.000), strenges Lesen, keine W-Note mit bestelltem K (sonst wäre das Register nicht mehr ladbar). 19 Mutationen rot.

### B2c: Pfand je Bestellung (23.11.2026, gebaut, schlafend)

Die Bestellung legt die Deckung D fest; der Ausgeber hinterlegt ein **Pfand** von
max(1 AEQ, ⌈10 % von D⌉) im W-Pfandtopf. Bei der Ausgabe (nur mit genau dieser Deckung) geht
es zurück. Verfällt die Bestellung, geht es ans Grundeinkommen — über die neue Art
`quanten_w_s_bestellung_frist` (jeder, ohne Wallet) oder beim nächsten Bestellen.

Warum 10 %: Ein Los mit t Komplizen gibt es je Bestellung mit p ≤ ~3,1 % (Tabelle oben).
Wer abbricht, bis eines kommt, verliert im Mittel Pfand / p ≥ 3 D — mehr, als die
Doppeleinlösung (D) einbringt. Damit ist der Befund aus B2b nicht mehr lohnend; die Bremse
„eine Bestellung je Mensch und Frist“ bleibt zusätzlich.

Tests: Pfand genommen, zurück bei Ausgabe, verfallen über die Frist-Art und beim nächsten
Bestellen, kein Pfand ohne Guthaben, Deckung unter dem Mindestwert, Deckung durch die
Unterschrift gebunden, andere Deckung bei der Ausgabe, Höhe (Rundung, Überlauf, ≥ 3 D bei
p = 3,1 %), strenges Lesen. 11 Mutationen rot; die 18 aus B2b weiter rot.

### B2d: Wächter Blockzeit (23.11.2026, gebaut)

Der Takt erzeugt jede Sekunde einen Block, auch leer (`block_cadence.go`). Der neueste Block
liegt also nur Sekunden hinter der echten Zeit. `/api/health/combined` meldet seinen
Zeitstempel (`chain.neuester_block_zeit`, ohne auf die DAG-Sperre zu warten), und die Wache
(`.github/wache_pruefung.py`) wird rot, wenn er mehr als 300 s hinter **ihrer eigenen** Uhr
liegt. Damit fällt auf, wenn C1 die Blockzeit zurückhält (und so eine schon bekannte
drand-Runde losen ließe) oder hängt. Keine Konsensregel. Lint mit 6 neuen Fällen,
Gegenprobe rot.

### B4a: die App als Präparatorin — melden und abmelden (23.11.2026, gebaut)

Aequitas-App `lib/quantenSPraeparator.ts` und eine Karte auf der Quantengeld-Seite (12
Sprachen): das Register lesen (streng, ≤ 256 Einträge, ≤ 128 KB), den eigenen Stand zeigen
(nicht gemeldet, wartet auf den Vorlauf, im Los, abgemeldet bis …) und mit der
**Menschen-Wallet** melden oder abmelden — erst nach ausdrücklicher Bestätigung. Die App
prüft die Unterschrift der Wallet selbst (kanonisch, gehört zur eigenen Adresse) und reicht
sonst nichts ein. Solange Weg B schläft, zeigt die Karte das an und bietet nichts an.
Prüfvektor Knoten ↔ App: `testdata/quanten_s_praep_vektor.json` (K, Nachrichten und
Unterschriften bytegleich; der Knoten nimmt genau diese Sätze an). 8 Mutationen in der App rot.

### B3: Briefkasten bei den Knoten (23.11.2026, gebaut)

`x/humanity/keeper/briefkasten.go`: Handys tauschen verschlüsselte Pakete über den Knoten.
`POST /api/briefkasten/ablegen` (nur ein registrierter Mensch, unterschrieben mit seiner
Wallet über Absender, Empfänger, SHA-256 des Inhalts und Zeit), `…/abholen` und
`…/quittieren` (nur der Empfänger, unterschrieben). Der Knoten sieht nur Chiffrat.

Grenzen: Inhalt ≤ 64 KB; je Absender ≤ 512 Briefe und ≤ 32 MB, je Empfänger ≤ 1024 Briefe (davon je Absender ≤ 64),
im ganzen Kasten ≤ 1 GB; nach 8 Tagen verfällt ein Brief; Abholen ≤ 32 Briefe bzw. 2 MB je
Aufruf; 60 Anfragen je IP und Minute; unterschriebene Zeiten ± 5 min. Dieselbe Ablage zweimal
wird abgewiesen. Ohne Datenbank oder bei einem Lesefehler nimmt der Kasten nichts an (503).

Keine Konsensregel: ein Dienst des einzelnen Knotens (keine StateRoot, kein Snapshot). Wer
sicher gehen will, legt bei mehreren Knoten ab und holt bei mehreren ab. 3 Tests (auch gegen
echtes PostgreSQL), 16 Mutationen rot.

### B4b-1: Schaltkreis eines Abschnitts im Handy (23.11.2026, gebaut; Weg 1: alles im Gerät)

Aequitas-App `lib/quantenSSchaltkreis.ts`: je Stelle X (t), H (u), Barriere, H (y), messen
— dieselbe Physik wie `hardware_ibm.py` `s_messen_basen`, aber ohne qiskit direkt in den
nativen Gattern (`rz`, `sx`, `x`) auf physischen Qubits als OpenQASM 3 (H = rz(π/2)·sx·rz(π/2)
bis auf eine globale Phase); Auswertung mit Mehrheit je Stelle wie im Python. Belegt durch
exakte Simulation und einen Prüfvektor (40 Fälle), den `app_schaltkreis_pruefen.py` mit
qiskit 2.6.2 gegen `wiesner_hardware.note` prüft (Operator gleich, Messbits richtig).

### B4b-2: IBM Quantum direkt aus dem Handy (23.11.2026, gebaut; Probe auf Hardware offen)

Aequitas-App `lib/ibmQuantum.ts`: Token aus dem IBM-Schlüssel (IBM Cloud IAM), Sampler-Auftrag
mit den Schaltkreisen des Abschnitts, Status, Ergebnis — Schlüssel und Instanz bleiben im
Gerät. Belegt aus `qiskit-ibm-runtime` 0.52.1: Pfade (`…/api/v1/jobs`, `…/jobs/{id}`,
`…/results`), Kopfzeilen (`Authorization: Bearer`, `Service-CRN`, `IBM-API-Version:
2026-04-15`), Status unter `state.status`, Ergebnis mit `__type__`-Hüllen bis zum BitArray
(ndarray als base64(zlib(npy))). Die App liest ein mit `RuntimeEncoder` kodiertes Ergebnis
bitgenau (Prüfvektor).

**Nachtrag (Doku gelesen):** Die IBM-Doku „Sampler with the REST API“ (über die Websuche;
der direkte Abruf war aus der Entwicklung gesperrt) belegt: Schaltkreise gehen als
**OpenQASM-3-Text** in `pubs`, `program_id` `sampler`, Kopfzeilen `Service-CRN` und
`IBM-API-Version` (2026-04-15). Die Doku liest das Ergebnis aber in einer **einfachen Form**:
`results[i].data.c.samples`, je Schuss ein Hex-Wert. Die kodierte Form mit `__type__`-Hüllen
bekommt nur, wer `"support_qiskit": true` schickt, wie der Python-Client
(`options.py`). Die App schickt den Schalter nicht und liest jetzt **beide Formen**, jede
streng: nur Register `c`, Bitzahl, höchstens 15 Schüsse, Hex mit höchstens 4 Stellen, keine
Bits über n. Ein zweiter Prüfvektor (`testdata/ibm_ergebnis_einfach_vektor.json`) muss
dieselben Schüsse ergeben.

**Instanz und Gerät selbst finden (23.11., nach Lauf 1 der Probe):** Lauf 1 brach ab, weil
`IBM_QUANTUM_INSTANCE` und `IBM_QUANTUM_BACKEND` im Repo nicht gesetzt sind. Die bisherigen
qiskit-Läufe brauchen sie nicht: `qiskit-ibm-runtime` sucht die Instanzen über IBM Cloud Global
Search (`POST api.global-search-tagging.cloud.ibm.com/v3/resources/search`, Abfrage
`service_name:quantum-computing`) und nimmt das freie Gerät mit der kürzesten Schlange
(`GET …/backends`, `status.name` `online`, keine Attrappe, `queue_length`). App
(`instanzenSuchen`, `geraeteListe`, `geraetWaehlen`) und Probe tun jetzt dasselbe; die
Präparatorin gibt nur ihren Schlüssel ein. Grenzen: höchstens 100 Suchtreffer, 8 Instanzen,
256 Geräte; nur strenge CRN- und Gerätenamen; eine Instanz ohne Zugang (401/403) wird
übersprungen, jeder andere Fehler bricht ab.

**Auf echter Hardware noch offen:** dass der Körper der App genau so angenommen wird und
welche Form zurückkommt. Dafür `quanten-s-rest-probe.yml` (Umgebung `quantenhardware`,
Freigabe des Betreibers): schickt **genau den Körper der App** (`testdata/ibm_auftrag_vektor.json`,
nur das Gerät wird ersetzt), liest das Ergebnis wie die App (Bericht nennt die Form) und prüft
die Physik (Treffer mit u = y müssen t zeigen, ≤ 10 % Fehler). Kosten: 3 Schaltkreise × 5
Schüsse. Wird der Auftrag abgewiesen, legt der Lauf den Grund von IBM ab.

**Auf echter Hardware belegt (23.11.2026, Lauf 5, 45213805068):** `ibm_marrakesh`, Auftrag
`dc26bvqnj2vs73dg7i0g` — der Körper der App wurde angenommen (COMPLETED), das Ergebnis kam in
der **einfachen Form**, 0 Fehler bei 18 Treffern. Die 403 der Läufe 2–4 kamen von der Kennung
`Python-urllib` (einzige Änderung zu Lauf 5); Probe und App schicken eine feste eigene Kennung.

### B4b-3a: Bausteine der Präparatorin in der App (23.11.2026, gebaut)

Aequitas-App `lib/quantenSPraeparieren.ts`, genau wie der Validator-Dienst
(`quanten_s_abschnitt.go`): gesalzene Werte mit Stellen-Bit (je Abschnitt genau zur Hälfte,
123 Bit Salz), Shamir t-von-v über F_P (Koeffizienten, Anteil je Validator), Blätter
(Prüfsumme je Stelle), je Empfänger ein Bündel (JSON wie `quantenSBuendelInhalt`, ECIES an
seinen Schlüssel, EIP-191 über den Hash des Chiffrats). Zufall nur vom Aufrufer, durch
Verwerfen gleichverteilt, schlechter Zufall bricht ab. Prüfvektor
`testdata/quanten_s_praep_buendel_vektor.json` entsteht in der App mit festem Zufall; der
Knoten öffnet genau diese Bündel mit seinem eigenen Code (`quanten_s_praep_buendel_vektor_test.go`).
Noch offen (B4b-3b): Pakete für die Annahmestellen (Stichprobe v2), `fuer_inhaberin`,
Antwort-Nachricht und Bestätigung.

### Nächste Schritte

| Schritt | Was |
|---|---|
| B4b | App als Präparatorin, Teil 2: gelost werden erkennen, Abschnitt präparieren (IBM-Schlüssel nur im Gerät, REST-Auftrag), Anteile verteilen, nach der Bindung öffnen. Braucht B3 (Briefkasten) für Pakete zwischen Handys. |
| Stichtag | Weg B scharf schalten: nur auf Entscheidung des Betreibers, mit Deploy-Wächter wie bei den anderen Stichtagen; vorher Pfand-Höhe am echten Register prüfen (`s_weg_b.py --tabelle`). Ohne B4 (App als Präparatorin) kann sich noch niemand bequem melden. |
| B2 (gesamt) | Kette: Präparatoren-Register (B2a gebaut), Zufall aus Zusage und Offenlegung, Los, Prüfung der Ausgabe gegen das Los, Pfand. Konsensänderung mit Stichtag, eigene Sicherheitsprüfung, Prüfvektor aus `s_weg_b.py`. |
| B3 | Briefkasten bei den Knoten: verschlüsselte Pakete (Basen y, Anteile, Öffnungen) ablegen und abholen, begrenzt je Absender und Größe. |
| B4 | App als Präparatorin: IBM-Schlüssel nur im Gerät, Auftrag über die REST-Schnittstelle, Abschnitt präparieren, Anteile verteilen, nach der Bindung öffnen. |
| B5 | Pilot im Hauptnetz mit mindestens 7 Menschen (4 von 7); erst danach größere v. |

