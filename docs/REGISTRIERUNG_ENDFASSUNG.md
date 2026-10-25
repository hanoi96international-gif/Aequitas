# Registrierung: Endfassung

Stand 25.10.2026. Gegenstück zu `WIRTSCHAFT_ENDFASSUNG.md`. Ordnet, was es
für die Registrierung schon gibt (Kette, Coordinator, Vergleichsdienst,
Proof-Server, App), prüft es gegen Forschung und Praxis und legt fest, wie
die Registrierung auf Dauer aussehen sollte und in welcher Reihenfolge das
gebaut wird.

**Status:** Entscheidungsvorlage. Am Code ist nichts geändert. Die neuen
Bausteine (Umzug, Präsenzaufnahme, Bürgschaft für die Staffel) sind
Konsensänderungen und brauchen eure Entscheidung, eine Aktivierungszeit und
eine eigene Sicherheitsprüfung (AGENTS.md).

**Grenze:** wie in `WIRTSCHAFT_ENDFASSUNG.md`. Originale waren nicht
abrufbar; ausgewertet sind Suchergebnisse, Zusammenfassungen und die
eigenen Analysen (`BIOMETRIE_ANALYSE*.md`, `AUDIT_2026-10-21.md`).

---

## 1. Ergebnis

1. **Registrierung ist kein Augenblick, sondern ein Lebenslauf in fünf
   Stationen:** Anmelden, Bestätigen, jährlich Nachweisen, Umziehen,
   Widersprechen. Drei davon gibt es schon (gebaut, teils schlafend), zwei
   fehlen.
2. **Gesichert wird die Verwendung, nicht nur die Anmeldung.** Eine Anmeldung
   ist einmalig und deshalb billig zu fälschen oder zu verkaufen. Der jährliche
   Lebensnachweis mit demselben Gesicht und der Umzug mit Zeitschloss machen
   gefälschte und gekaufte Identitäten wertlos, ohne dass die Biometrie
   perfekt sein muss.
3. **Der größte fehlende Baustein ist der Umzug:** Wer seinen Schlüssel
   verliert oder sein Konto verkauft hat, holt sein Menschsein mit seinem
   Gesicht auf eine neue Wallet zurück, wirksam nach 14 Tagen, in denen die
   alte Wallet widersprechen kann. Das beantwortet die offene Frage aus
   Befund H5 („wer darf ein Menschsein umziehen, welche Wartefrist“) und
   zerstört den Mietmarkt für Identitäten.
4. **Bis die Schwellen an echten Aufnahmen gemessen sind, ist jede
   Registrierung „gelb“:** 200 AEQ sofort, 800 AEQ nach einer zweiten Prüfung
   ab Tag 7 oder spätestens an Tag 30 (Entscheidung vom 18.10.2026). Grün
   (alles sofort) gibt es erst mit gemessenen Schwellen, im Pilot auch über
   eine Präsenzaufnahme.
5. **Vor echtem Wert müssen scharf sein:** Bescheinigung im Konsens (K1),
   ein zweiter Prüfdienst eines anderen Menschen, Zulassung der Coordinatoren
   im Konsens (sie schaltet den Lebensnachweis frei), die Rechtstexte (K3).
6. **Nicht kommen:** Ausweispflicht, Stimme, Ausschluss nach Gerät, Entzug
   des Menschseins.

---

## 2. Ist-Stand

| Station | Baustein | Stand |
|---|---|---|
| Anmelden | Gesichtsaufnahme mit Blinzeln und Kopfdrehung; Vergleich gegen jede Einschreibung (kein Index, Recall 1); 64-Byte-Auszug statt Vorlage | **scharf**, ein Vergleichsdienst (Quorum 1, höchstens 25 am Tag) |
| | Blitzfarben, Puls, Parallaxe, Antispoof, Geräte-Attestierung | gebaut, **entscheiden nichts** (Schattenmodus) |
| | Alter: Selbstangabe vor der Kamera, Schätzung, Bürgschaft zweier Menschen im Graubereich | gebaut; Schätzung **beobachtet nur** (`modell_bereit=false`) |
| | Einwilligung v4 vor dem ersten Bild; Mengengrenzen je /64; Körpergrenze | **scharf** |
| | Zwillinge über gemeinsame Aufnahme | gebaut (App 1.12.6, Backend aus) |
| | Bescheinigung von Coordinator und Vergleichsdienst über den Nullifier, von **jedem** Knoten geprüft (K1) | gebaut, **schlafend** (`BESCHEINIGUNG_IM_KONSENS.md`) |
| Bestätigen | gestaffelter Zuschuss 200 + 800, Zweitprüfung ab Tag 7 an dieselbe Person gebunden | gebaut, **schlafend bis 2100**; Freigabe an Tag 30 beschlossen, noch nicht eingebaut |
| Jährlich | Lebensnachweis mit demselben Gesicht, Pause beim Grundeinkommen statt Entzug | gebaut, Stichtag 01.11.2026, wirkt erst mit Coordinator-Zulassung im Konsens |
| Umziehen | Menschsein auf eine neue Wallet | **fehlt** (nur als Idee in `GRUNDPFEILER.md` 1 und Audit H5) |
| Widersprechen | Kennung `W-…`, Prüfung durch einen Menschen (Art. 22 DSGVO) | **scharf** |

Belegt mit echten Menschen ist bisher **eine** Registrierung
(`DOPPELREGISTRIERUNG_TEST.md`, `gallery_test: 1`). Fehlerraten sind nicht
gemessen.

---

## 3. Bedrohungen, ehrlich benannt

| # | Bedrohung | Was heute hält | Was fehlt |
|---|---|---|---|
| B1 | **Kunstfigur** (Deepfake, vorbereitete Aufnahme) | Kopfdrehung mit zwei Ausprägungen | Blitzfarben und mehrstufige Aufgabe als Bedingung für grün, Staffel (Audit K4) |
| B2 | **Erzeuger prägt Menschen** ohne Gesichtsprüfung | nur ein Erzeuger (der Betreiber) | Bescheinigung im Konsens scharf (K1), vor dem zweiten Erzeuger |
| B3 | **Gekaufte oder gemietete Identität** (echter Mensch, fremder Nutzen) | nichts | Lebensnachweis aktiv, Umzug (Abschnitt 5.4) |
| B4 | **Schlüssel verloren oder gestohlen** | nichts; Dieb erbt das Grundeinkommen | Umzug |
| B5 | **Fälschlich abgewiesen** (Doppelgänger, Licht) | Widerspruch, Verzögern statt Sperren | gemessene Schwelle |
| B6 | **Zwang** (Gesicht unter Druck aufnehmen lassen) | – | Zeitschloss im Umzug |
| B7 | **Tagesgrenze leergefüllt** (25 Kunstfiguren am Tag sperren alle anderen) | zusätzlich 3 je Netz (/24 bzw. /48) und Tag (`einzelbetrieb.py`) | zweiter Prüfdienst hebt die Grenze auf; viele Netze bleiben ein Weg |
| B8 | **Minderjährige** | Selbstangabe, Bürgschaft | Schätzung verbindlich |
| B9 | **Ein Mensch hat alle Macht** (Erzeuger, Prüfdienst, Coordinator) | Texte sagen es offen (K5, behoben) | zweiter Betreiber |

Die Wirtschafts-Endfassung macht B1, B3 und B4 wichtiger: Eine Identität
bringt dann neben 1.000 AEQ rund 60 AEQ im Jahr
(`WIRTSCHAFT_ENDFASSUNG.md`, 8.3). Der Strom läuft aber nur mit dem
jährlichen Gesicht des echten Menschen, und genau das ist der Hebel.

---

## 4. Was Forschung und Praxis sagen

| Befund | Was daraus folgt | Quelle |
|---|---|---|
| Ohne zentrale, vertraute Stelle sind Sybil-Angriffe in offenen Netzen praktisch immer möglich. | Einmaligkeit braucht einen Anker außerhalb des Netzes. Bei Aequitas ist es das Gesicht plus die Vergleichsdienste; die müssen mehrere und unabhängig sein. | Douceur, The Sybil Attack (2002) |
| Pseudonym-Partys: Ein Mensch kann nicht zur selben Zeit an zwei Orten sein. Ford hält föderierte Präsenztreffen für den einzigen Weg, Inklusion, Gleichheit, Sicherheit und Privatsphäre zugleich zu erreichen. Encointer macht daraus Treffen mit Grundeinkommen je Teilnahme. | Präsenz ist das stärkste Signal, aber teuer. Für eine Pilotstadt ist es machbar (5.2). | Ford/Strauss 2008; Ford 2020; Encointer |
| Idena: echte, gleichzeitig geprüfte Menschen wurden von „Puppeteers“ bezahlt, ihre Konten abzugeben. | Gegen Vermietung hilft keine bessere Prüfung. Sie muss sich für den Käufer nicht lohnen: Rückholbarkeit durch den Menschen. | Ohlhaver, Compressed to 0 (Harvard Ash Center); `BIOMETRIE_ANALYSE_TEIL3.md` |
| World-IDs wurden für rund 30 $ gehandelt. World reagierte mit Gesichtsabgleich bei der Nutzung (Face Auth) und dem Zurückholen einer World-ID durch einen erneuten Scan. | Genau die beiden Bausteine Lebensnachweis und Umzug. World nutzt sie, Aequitas hat den einen gebaut, den anderen nicht. | The Block 2023; The Defiant (Face Auth) |
| Ğ1 (Duniter): Mitgliedschaft braucht 5 Bürgschaften, gilt ein Jahr und muss erneuert werden; Bürgschaften verfallen nach zwei Jahren, jeder hat höchstens 100 und kann nur alle 5 Tage eine ausstellen. | Bürgschaft funktioniert nur knapp, befristet und mit Ratenbegrenzung. Jährliche Erneuerung ist erprobt. | Duniter-Wiki |
| Proof of Humanity (Kleros): Eintrag braucht die Bürgschaft eines Registrierten und eine Kaution, danach ein Einspruchsfenster von einigen Tagen; Streit entscheidet ein Schiedsgericht. | Ein **Einspruchsfenster** vor der vollen Wirkung ist ein bewährtes Muster (wie Tag 30 bei der Staffel). | PoH-Governance (HIPs) |
| Buterin: Biometrische Personennachweise haben vier Risiken (Privatsphäre, Zugang, Zentralisierung, Sicherheit einschließlich Zwang); Verfahren kombinieren. | Gesicht **plus** ein zweites, nicht biometrisches Verfahren (Bürgschaft, Präsenz). | `GRUNDPFEILER.md` 1 |

---

## 5. Die Endfassung: fünf Stationen

| Station | Wann | Was der Mensch tut | Was er bekommt | Wer prüft |
|---|---|---|---|---|
| 1 Anmelden | einmal | Einwilligung, Alter, Gesicht mit zufälliger Aufgabe | Menschsein, 200 AEQ (gelb) oder 1.000 AEQ (grün) | Vergleichsdienste (k von n, verschiedene Menschen); jeder Knoten prüft die Bescheinigung |
| 2 Bestätigen | Tag 7 bis 30 | zweite Prüfung **oder** zwei Bürgen **oder** nichts | Rest von 800 AEQ: sofort nach Prüfung oder Bürgen, sonst an Tag 30 | wie 1; Einspruch nur mit Quorum |
| 3 Jährlich nachweisen | ab Tag 300, Frist 365 + 30 | dasselbe Gesicht | weiter das Grundeinkommen | wie 1, gebunden an den Nullifier der Wallet |
| 4 Umziehen | bei Bedarf | dasselbe Gesicht mit neuer Wallet | Menschsein und künftiges Grundeinkommen auf der neuen Wallet, nach 14 Tagen | wie 3; alte Wallet kann widersprechen |
| 5 Widersprechen | nach Abweisung, 90 Tage | Kennung `W-…`, eigener Standpunkt | Prüfung durch einen Menschen | Betreiber-Mensch, später Quorum |

### 5.1 Anmelden

Bleibt im Kern, wie es ist. Drei Änderungen:

1. **Blitzfarben und eine mehrstufige Kopfaufgabe** (vom Coordinator gewählt,
   zufällige Reihenfolge, z. B. links, oben, rechts, in derselben Aufnahme)
   werden **Bedingung für grün**, nicht für den Zugang. Wer sie nicht
   besteht, wird gelb, nicht abgewiesen. Das hält euren Grundsatz „verzögern,
   nie aussperren“ (Audit-Umsetzung K4) und nimmt der Kunstfigur trotzdem den
   Großteil des Zuschusses: Mit zwei vorbereiteten Animationen kommt sie nur
   noch an 200 AEQ.
2. **Attestierung und Herkunft entscheiden ebenso nur die Klasse.** Fehlt
   Play Integrity oder App Attest, ist die Registrierung gelb. Kein Ausschluss
   nach Gerät.
3. **Altersschätzung verbindlich**, sobald das Modell bereit ist; bis dahin
   keine Registrierung Dritter mit echtem Wert.

Die Tagesgrenze im Einzelbetrieb (25 am Tag, 3 je Netz) bleibt, bis der
zweite Prüfdienst sie aufhebt. Für Präsenztage (5.2) braucht sie eine
Ausnahme je angemeldeter Station, sonst kommen an einem Ort nur drei Menschen
am Tag durch.

### 5.2 Bestätigen

- **Solange die Schwellen nicht an echten Aufnahmen gemessen sind** (WP 4 in
  `LEBENDIGKEIT_GEGEN_DEEPFAKES.md`: mindestens 20 echte Registrierungen für
  die Lebendigkeit, rund 1.000 Paare verschiedener Menschen für die
  Verwechslungsrate), **ist jede Registrierung gelb.** Das ist fail-closed:
  Eine Klasse, deren Grenze niemand gemessen hat, darf nicht „alles sofort“
  bedeuten.
- **Freigabe spätestens an Tag 30**, anhalten nur durch einen begründeten
  Einspruch mit Quorum (beschlossen 18.10.2026). In `grant_staffel.go`
  einbauen, **bevor** die Staffel scharf wird.
- **Neu: zwei Bürgen beschleunigen.** Zwei Menschen, die selbst mindestens
  90 Tage registriert sind, bestätigen mit ihrer Unterschrift „Ich kenne
  diese Person, und sie ist nur einmal hier“. Dann wird der Rest sofort
  frei, wie nach der Zweitprüfung. Die Bürgschaft bringt **kein zusätzliches
  Geld**, nur Zeit. Grenzen wie bei der Altersbürgschaft und bei Ğ1:
  höchstens 5 Bürgschaften je Mensch und Jahr; wer für eine Registrierung
  bürgt, die später als Doppelung nachgewiesen wird, darf zwei Jahre nicht
  mehr bürgen. Niemandem wird etwas genommen.
- **Neu: Präsenzaufnahme im Pilot.** In der Pilotstadt nimmt eine
  Aufnahmestation auf, die ein registrierter Mensch betreut (eigener
  Schlüssel, im Register gebunden wie ein Coordinator). Die Station
  unterschreibt zusätzlich „aufgenommen am Ort, zur Zeit“. Das ist das
  Pseudonym-Party-Prinzip im Kleinen und macht eine Registrierung grün,
  auch ohne gemessene Schwellen für die Fernprüfung.

### 5.3 Jährlich nachweisen

Gebaut (`lebensnachweis.go`, Coordinator `lebensnachweis.py`). Es fehlt nur,
dass Coordinatoren im Konsens zugelassen sind (`registerLeserAb` steht auf
einem Platzhalter); vorher wird niemand pausiert. **Mit der
Wirtschafts-Endfassung wird das eine Voraussetzung**: Die Dividende ist der
Grund, eine Identität zu kaufen, und der jährliche Nachweis ist das, was den
Kauf entwertet. Vor echtem Wert scharf schalten.

Nebenwirkung, die nichts kostet: Ein Konto, dessen Gesicht schon unter einer
anderen Wallet eingeschrieben ist, kann den Nachweis nie bestehen (die
getroffene Einschreibung muss zum Nullifier **dieser** Wallet gehören). Eine
doppelte Registrierung, die durchgerutscht ist, verliert so ihr
Grundeinkommen, ohne dass jemand das Menschsein entzieht.

### 5.4 Umziehen (neu)

**Wozu.** Seed verloren, Telefon gestohlen, Konto verkauft oder unter Druck
abgegeben: Heute ist das Menschsein dann für immer weg oder beim Falschen
(Audit H5). Der Umzug gibt es dem Menschen zurück, mit dem einzigen, was er
nicht verlieren kann: seinem Gesicht.

**Ablauf.**

1. In der App: „Konto zurückholen“. Eine **neue** Wallet W2 (noch kein
   Mensch) und eine Aufnahme mit voller Aufgabe (Blitzfarben, Kopfdrehung).
2. Das Vergleichs-Quorum erkennt das Gesicht als **bekannt**, alle unter
   derselben Einschreibung, mit einer **strengeren** Schwelle als bei der
   Duplikatsuche. (Die Duplikatsuche irrt lieber zu „schon da“; der Umzug muss
   lieber zu „nein“ irren.)
3. Coordinator und Vergleichsdienste unterschreiben über den Nullifier N
   dieser Einschreibung, die alte Wallet W1 (aus der Kette: N gehört zu W1),
   die neue Wallet W2 und die Zeit, mit eigener Domäne
   (`aequitas-umzug-v1|chain:<id>|<N>|<W1>|<W2>|<zeit>`). W2 unterschreibt den
   Antrag.
4. Transaktion `umzug_ankuendigen`. **Jeder Knoten prüft selbst**: Schlüssel
   aus dem Register beziehungsweise der Genesis, Quorum verschiedener
   Betreiber-Menschen, N gehört zu W1, W1 ist Mensch, W2 ist kein Mensch,
   kein anderer Umzug offen, Zeitfenster der Bescheinigung.
5. **Zeitschloss 14 Tage.** In dieser Zeit kann W1 mit seiner Unterschrift
   widersprechen (`umzug_widerruf`); dann ist der Antrag erledigt. Die App von
   W1 zeigt die Ankündigung sofort und unübersehbar; sie steht öffentlich
   auf der Kette.
6. Nach Ablauf `umzug_vollzug` (im Tageslauf): Menschsein, Nullifier,
   Lebensnachweis-Stand, offener Staffelrest und das künftige
   Grundeinkommen gehen auf W2 über.

**Was nicht umzieht: das Guthaben von W1.** Es bleibt bei dem, der den alten
Schlüssel hat. Geld bewegt nur eine Unterschrift (Grundsatz „nur bestätigen,
nie wegnehmen“). Mit der Wirtschafts-Endfassung fließt Geld hinter einem
verlorenen Schlüssel ohnehin langsam an alle zurück.

**Grenzen.** Höchstens ein offener Umzug je Nullifier und zwei vollzogene je
365 Tage. Zeitschloss in der Spanne 7 bis 30 Tage, Vorgabe 14.

**Zwillinge.** Bei einer Einschreibung aus der gemeinsamen Aufnahme
(`zwilling.py`) erkennt der Vergleich beide Zwillinge. Ein Umzug ist dort nur
über eine erneute gemeinsame Aufnahme möglich, sonst gar nicht.

**Zwang.** Ein Erpresser müsste sein Opfer 14 Tage festhalten oder dessen
alten Schlüssel haben; mit dem alten Schlüssel braucht er keinen Umzug. Nach
einem erzwungenen Umzug kann der Mensch selbst wieder umziehen (zweiter
Umzug im Jahr).

**Warum das den Mietmarkt zerstört.** Wer eine Identität kauft, kauft einen
Schlüssel. Kann der Verkäufer sie jederzeit in 14 Tagen zurückholen, ist der
Schlüssel für den Käufer nichts wert, und den jährlichen Nachweis braucht er
ohnehin vom Verkäufer. Aus einem einmaligen Verkauf wird eine Beziehung, aus
der der Mensch jederzeit aussteigen kann (`BIOMETRIE_ANALYSE_TEIL3.md` 2.4).

**Verträglichkeit mit den Verifier-Prinzipien.** Der Umzug setzt bei W1
`is_human` zurück. Das ist **kein Entzug**: Das Menschsein bleibt beim
Menschen und wechselt die Adresse, ausgelöst nur durch das Gesicht desselben
Menschen, mit Frist und Widerruf durch den alten Schlüssel. Es braucht
trotzdem eine ausdrückliche Ausnahme in `kein_entzug_test.go` mit Begründung,
und die Entscheidung gehört euch (Abschnitt 10, Nr. 3).

**Was V8 dafür braucht.** `AequitasV8.sol` kennt keinen Umzug
(`usedNullifiers[N] = W1`, `isHuman[W1]`). Nötig ist eine Vertragsfunktion
für den Registrar (neue Fassung) oder eine Umzugstabelle im Go-Zustand, auf
die die Spiegelung der V8-Felder achtet. Das ist der aufwendigste Teil und
gehört in einen eigenen Entwurf vor dem Bau.

### 5.5 Widersprechen

Bleibt. Ergänzung: eine **Frist für die Antwort** (z. B. 14 Tage) und eine
Zahl auf der Statusseite (offene Widersprüche, älteste Wartezeit). Ein
Widerspruchsweg, auf den niemand antwortet, ist keiner.

---

## 6. Was vor echtem Wert scharf sein muss

In dieser Reihenfolge (sie folgt der Abhängigkeit):

| # | Was | Stand | Warum zuerst |
|---|---|---|---|
| 1 | Impressum, Datenschutz, Einwilligung mit Verantwortlichem, DSFA, geheimer Seed, Testmodus ehrlich (K3) | offen, Betreiber | Ohne sie ist schon die Aufnahme Dritter rechtlich angreifbar |
| 2 | Bescheinigung im Konsens (K1) | gebaut, schlafend | Sonst kann ein Erzeuger Menschen prägen |
| 3 | Zweiter Prüfdienst, betrieben von einem anderen Menschen, Quorum 2 | offen | Ein Mensch entscheidet heute allein (K5); hebt die Tagesgrenze auf |
| 4 | Coordinatoren im Konsens zugelassen (`registerLeserAb`) | gebaut, Platzhalter | Schaltet Lebensnachweis und Staffel-Voraussetzungen frei |
| 5 | Staffel mit Tag-30-Freigabe, „alle gelb“ bis zur Kalibrierung | gebaut ohne Tag 30, schlafend | Begrenzt den Schaden jeder durchgerutschten Kunstfigur |
| 6 | Blitzfarben und mehrstufige Aufgabe als Bedingung für grün | gebaut, aus | K4 |
| 7 | Umzug | fehlt | H5, Mietmarkt |
| 8 | Schwellen gemessen, dann grün scharf | offen, braucht echte Menschen | erst danach „alles sofort“ |

---

## 7. Und Unternehmen?

Ein Unternehmen wird nicht registriert wie ein Mensch und nicht geprüft
(`WIRTSCHAFT_ZAHLENPRUEFUNG.md` 2). Mit der Wirtschafts-Endfassung bringt
das Unternehmenskonto kein Geld mehr; die Eröffnung durch einen
verifizierten Menschen (höchstens 3 je Mensch) und die Anzeigen (Kundschaft,
Bürgen, Website) genügen. Anmeldung und Kasse in der App kommen mit dem
ersten Pilotladen.

---

## 8. Meine Abwägung

- **Das Gesicht reicht für den Pilot, nicht für die Welt.** Bei 10⁸ bis 10⁹
  Menschen gibt es Doppelgänger und Zwillinge, die ein Gesichtsvergleich nicht
  sauber trennt; das Whitepaper sagt das selbst. Iris ist dafür das bessere
  Merkmal. Aber die Hardware steht nicht fest und zentralisiert, und vor
  allem ist die Biometrie **nicht der Engpass**. Idena hat echte Menschen
  sauber geprüft und wurde trotzdem gekapert. Ich würde vor dem Pilot kein
  Geld in Iris-Hardware stecken, sondern in Umzug, Lebensnachweis und den
  zweiten Prüfdienst.
- **Die stärkste Sicherung ist wirtschaftlich, nicht technisch.** Mit Staffel,
  jährlichem Nachweis und Umzug bringt eine gekaufte Identität dem Käufer
  200 AEQ sofort, den Rest nur ohne Einspruch nach 30 Tagen, das
  Grundeinkommen nur mit dem Gesicht des Verkäufers jedes Jahr, und alles
  kann der Verkäufer in 14 Tagen zurückholen. Das ist billig zu bauen und
  wirkt auch dann, wenn die Biometrie Fehler macht.
- **Präsenz ist das ehrlichste Signal.** In einer Pilotstadt kostet ein
  Aufnahmetag wenig und liefert genau das, was keine Fernprüfung kann: ein
  Mensch, ein Ort, eine Zeit.
- **Fremde Anker nur als Zusatz.** eID-Chip oder World ID könnten später eine
  Registrierung grün machen, aber nie Bedingung sein; rund 850 Millionen
  Menschen haben keinen Ausweis, und World hat eigene
  Datenschutzprobleme. Nicht vor dem Pilot.
- **Ehrlichkeit nach außen ist Teil der Sicherheit.** Solange ein Mensch
  Erzeuger, Prüfdienst und Coordinator ist, muss das überall so stehen (K5).

---

## 9. Umsetzung

Je Schritt eine eigene PR mit Sicherheitsabschnitt (AGENTS.md) und, wo
Konsens betroffen ist, einer Aktivierungszeit nach dem Ausrollen auf alle
Knoten.

| # | Schritt | Repo, Ort | Konsens? |
|---|---|---|---|
| 1 | Tag-30-Freigabe und Einspruch mit Quorum in die Staffel | Aequitas `grant_staffel.go`, Nachrechnen | ja |
| 2 | Coordinator: Klasse „gelb“ als Vorgabe bis zur Kalibrierung (`LEBENDIGKEIT_VERBINDLICH`, neue Einstellung für die Vorgabe) | biometric-beta `lebendigkeit.py` | nein |
| 3 | Staffel scharf: `stagedGrantActivationUnix` setzen (Reihenfolge in `LEBENDIGKEIT_GEGEN_DEEPFAKES.md`) | Aequitas | ja |
| 4 | Blitzfarben und mehrstufige Kopfaufgabe als Bedingung für grün | biometric-beta, App | nein |
| 5 | Ausnahme von der Tagesgrenze je Netz für angemeldete Präsenzstationen | biometric-beta `einzelbetrieb.py` | nein |
| 6 | Bürgschaft für die Staffel: Transaktion mit zwei Unterschriften, Grenzen, Sperre nach nachgewiesener Doppelung | Aequitas, App | ja |
| 7 | Umzug: Entwurf (V8 oder Go-Tabelle), dann Coordinator `/umzug`, Kette `umzug_ankuendigen` / `_widerruf` / `_vollzug`, App „Konto zurückholen“ | alle vier | ja |
| 8 | Präsenzaufnahme: Stationsschlüssel im Register, Unterschrift in der Bescheinigung, Klasse grün | biometric-beta, Aequitas | ja |
| 9 | Widerspruch: Antwortfrist und Zahlen auf der Statusseite | biometric-beta | nein |

**Sicherheit für Umzug und Bürgschaft (die zwei neuen Konsensregeln):**

1. *Wer erreicht den Code?* Jeder über die App und den Coordinator (öffentlich,
   feindlich); die Transaktionen kommen in Blöcken von Peers.
2. *Was ist begrenzt?* Ein offener Umzug je Nullifier, zwei je Jahr;
   Bürgschaften 5 je Mensch und Jahr; feste Größe der Unterschriften und
   Felder; Zeitschloss mit fester Spanne.
3. *Fehlerfall?* Fehlt eine Unterschrift, ist ein Schlüssel unbekannt oder
   ein Feld unlesbar: abgelehnt. Ein Umzug, der nicht geprüft werden kann,
   findet nicht statt.
4. *Prüft jeder Knoten?* Ja: Unterschriften gegen Register und Genesis,
   Zuordnung N → W1 aus dem eigenen Zustand, Fristen nach Blockzeit.
5. *Missbrauchstests:* gefälschte Coordinator- oder Bezeuger-Unterschrift;
   alte Bescheinigung wiederholt; Umzug auf eine Wallet, die schon Mensch
   ist; Vollzug vor Ablauf; Widerruf mit fremdem Schlüssel; zwei Umzüge
   gleichzeitig; Umzug einer Zwillings-Einschreibung ohne gemeinsame
   Aufnahme; Bürgschaft durch Nicht-Menschen, durch zu junge Konten, über der
   Jahresgrenze, für sich selbst; Guthaben von W1 bleibt unverändert.

---

## 10. Zu entscheiden

1. **Fünf Stationen** als Leitbild der Registrierung. Empfehlung: **ja**.
2. **„Alle gelb“ bis zur Kalibrierung** (Staffel für jede Registrierung,
   Freigabe spätestens an Tag 30). Empfehlung: **ja**.
3. **Umzug mit Zeitschloss**, ohne das Guthaben, als ausdrückliche Ausnahme
   in `kein_entzug_test.go`. Empfehlung: **ja**, Entwurf vor dem Bau.
4. **Bürgschaft** als Beschleuniger der Staffel, ohne Geldvorteil.
   Empfehlung: **ja**.
5. **Präsenzaufnahme** in der Pilotstadt. Empfehlung: **ja**.
6. **Iris-Hardware vor dem Pilot.** Empfehlung: **nein**.

## Quellen

- Douceur, The Sybil Attack (IPTPS 2002): https://www.springerprofessional.de/the-sybil-attack/13551662
- Proof of Personhood (Übersicht, Pseudonym-Partys, Borge u. a. 2017): https://en.wikipedia.org/wiki/Proof_of_personhood ; Ford, Folien 2021: https://bford.info/talk/2021-07-27-personhood/personhood.pdf ; Ford 2020: https://arxiv.org/pdf/2011.02412
- Ohlhaver, Compressed to 0 (Harvard Ash Center): https://ash.harvard.edu/wp-content/uploads/2024/06/proof-of-personhood_ohlhaver.pdf
- World-ID-Handel, Gegenmaßnahmen und Zurückholen per erneutem Scan: https://www.theblock.co/post/231433/worldcoin-iris-black-market ; Face Auth: https://thedefiant.io/worldcoin-pilots-face-auth-account-verification-to-prevent-fraud
- Duniter, Web of Trust: https://duniter.org/wiki/web-of-trust/
- Proof of Humanity, Governance (Bürgschaft, Einspruch): https://gov.proofofhumanity.id/t/proposal-allow-consuming-projects-to-fine-tune-their-security-requirements/1068
- Encointer: https://hackernoon.com/encointer-a-blockchain-experiment-in-local-currencies-and-universal-basic-income
- Buterin zu biometrischen Personennachweisen: https://www.theblock.co/post/241088/vitalik-buterin-four-major-risks-worldcoin

Eigene Grundlagen: `BIOMETRIE_ANALYSE.md`, `BIOMETRIE_ANALYSE_TEIL2.md`,
`BIOMETRIE_ANALYSE_TEIL3.md`, `AUDIT_2026-10-21.md` (K1, K3, K4, K5, H5),
`VERIFIER_PRINZIPIEN.md`, `LEBENDIGKEIT_GEGEN_DEEPFAKES.md`,
`BESCHEINIGUNG_IM_KONSENS.md`, `DOPPELREGISTRIERUNG_TEST.md`.
