# Dezentrales Quantengeld komplett: was fehlt, und der Weg dorthin

Stand 18.11.2026. Auftrag des Betreibers: „Bring das dezentrale Quantengeld komplett zu
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
| B, C, W-Härtung | wie oben | Forschung; Partner |

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
   - **Was bleibt:** das Werkzeug im v2-Weg (`quanten-s-ausgabe`, nächster Schritt) und
     dann die Freigabe des Betreibers mit einem Stichtag.
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
