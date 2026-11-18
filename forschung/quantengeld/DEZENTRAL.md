# Dezentrales Quantengeld komplett: was fehlt, und der Weg dorthin

Stand 18.11.2026. Auftrag des Betreibers: „Bring das dezentrale Quantengeld komplett zu
laufen, ggf. mit eigener Forschung“. Diese Notiz sagt, was heute läuft, wo es noch an
einer Stelle hängt, und was wir bauen.

## 1. Was heute läuft

- **Variante S im Hauptnetz** seit 17.11.2026 17:00 UTC. Ein Token wurde ausgegeben, die
  Doppelvorlage abgewiesen und die ehrliche Vorlage eingelöst (`HEUTE.md` 3.3c).
- **Mit vier Validatoren** läuft es im Pilot: 3 von 4, k = 5.200, auf IBM `ibm_kingston`
  und im Rauschmodell, auch wenn einer ausfällt und mit Netzlaufzeit (`HEUTE.md` 3.3d).
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
| A3 | `hardware_ibm.py --s-abschnitt`, Werkzeug `quanten-s-ausgabe` im Abschnitts-Modus | wir |
| A4 | Pilot: vier Knoten, je ein eigener Auftrag (im Pilot von einem Konto, das wird benannt) | wir; IBM-Lauf mit Freigabe |
| A5 | Hauptnetz mit einem zweiten Menschen als Validator, wenn möglich bei einem anderen Anbieter | Betreiber |
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
  - Die Messung kommt aus der **eigenen Messquelle** (`AEQUITAS_QS_MESSQUELLE`, eine Datei
    von `hardware_ibm.py --s-abschnitt`). Jeder Datensatz wird vor der Verwendung aus der
    Datei gelöscht, also nie zweimal benutzt.
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
