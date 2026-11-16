# Quantengeld – vollständige Analyse und alle Lösungswege

Stand 16.11.2026. Grundlage: der Code und die Simulationen in diesem Ordner, dazu
die unten genannten Arbeiten. Die mit (V) markierten habe ich im Volltext oder in
der Zusammenfassung von arXiv/IACR selbst gelesen, die mit (S) nur über
Suchergebnisse. Vor jeder Zitierung oder Kontaktaufnahme selbst lesen.

## Kurzfassung

1. **Die Kettenseite ist fertig und läuft.** Register, Anmelden, Umlaufabgabe im Topf,
   Einlösen (gesperrt), Freischaltung (⅔ der Prüfer, mindestens 5) und QG3 sind gebaut.
   Es fehlt nichts an der Kette. Es fehlt die Quantenseite.
2. **Der bisherige Weg** setzt auf öffentlich prüfbare Noten, Quantum Lightning und eine
   Einmal-Unterschrift zum Einlösen. Er hängt an drei Dingen, die es 2026 nicht gibt:
   - **Einlösen braucht iO.** Die neue Einmal-Unterschrift von Huang & Vaikuntanathan
     (2025/26) ist viel effizienter, braucht im Standardmodell aber weiter
     Ununterscheidbarkeits-Verschleierung (iO) plus LWE (V).
   - **Speicher:** Eine öffentlich prüfbare Note mit 64 Qubits braucht eine Kohärenzzeit
     von 3.184 × Lagerzeit (`rauschen_oeffentlich.py`). Das heißt: fehlerkorrigierte
     logische Qubits.
   - **Prüfaufwand:** rund 2⁵⁵ Toffoli-Gatter je Prüfung bei 8.192 Bit (`isogenie_kosten.py`).
3. **Es gibt einen anderen Weg, der um Jahre näher liegt:** privat prüfbare Noten aus der
   Wiesner-Familie mit **klassischer Prüfung**.
   - **Prüfgeheimnis:** Es liegt **verteilt bei den Validatoren** statt bei einer Bank
     (hier „Variante W“, Abschnitt 5).
   - **Sicherheit:** ohne Rechenannahme; tolerant gegen Rauschen bis 21,4 % (Kumar 2019, V).
   - **Speicher:** 2,5–10 × Lagerzeit statt 3.184 ×, gerechnet für die Prüfung mit
     Rückgabe der Note. Bei klassischer Prüfung braucht es mehr Qubits, etwa 1.170
     statt 309 für 2⁻¹²⁸. Speicher mit über 10 h Kohärenz gibt es im Labor
     (Wang et al. 2025).
   - **Partner:** Ein deutscher BMFTR-Verbund aus sieben Projekten baut genau solche
     Token-Geräte (V).
   - **Neu für Aequitas** ist nur, das Geheimnis über die Validatoren zu verteilen. Dafür
     gibt es mit `x/humanity/mpc` schon Bausteine.
4. **Was Nutzer von Quantengeld eigentlich wollen** (offline zahlen wie mit Bargeld),
   liefert das **Offline-Bargeld** ohne jede Quantenhardware. Stufe 1 ist fertig, die
   Stufen 2–5 sind beschrieben (`docs/OFFLINE_BARGELD.md`).
5. **Ionenfallen gibt es auch ohne Quantinuum** (S):
   - AQT IBEX Q1 (12 Ionen) über Open Quantum, 50 $ freie Rechenzeit alle 90 Tage;
   - IonQ über Qollab, Azure (500 $ je Anbieter beim ersten Mal) und IonQ-Forschungsguthaben
     bis 10.000 $.
6. **Geld und Begutachtung gehen auch anders:**
   - Unitary Foundation: Kleinförderung 4.000 $, jederzeit (S);
   - Prototype Fund: jährlich im Herbst (S);
   - der BMFTR-Token-Verbund als fachlicher Partner, darunter Juliane Krämer (Regensburg)
     für Post-Quanten-Kryptografie und Token (V).
7. **Empfehlung: drei Spuren zugleich.**
   - **Jetzt nutzbar:** Offline-Bargeld zu Ende bauen.
   - **Nahe Quantenspur:** Variante W simulieren, die Kettenseite schlafend spezifizieren
     und mit dem Verbund auf echten Speichern testen.
   - **Lange Spur:** öffentlich prüfbare Noten bleiben schlafend, bis iO oder
     fehlerkorrigierte Speicher reif sind.

Was davon **heute** schon funktionieren kann, steht in `HEUTE.md`.

## 1. Was Quantengeld für Aequitas leisten soll

| | Eigenschaft | Warum |
|---|---|---|
| E1 | **unfälschbar**, auch für wer die Kette nicht fragt | der Kern: Physik statt Kontenbuch |
| E2 | **offline prüfbar** | Zahlen ohne Netz, wie Bargeld |
| E3 | **privat** | niemand sieht, wer wem zahlt |
| E4 | **von Hand zu Hand** weiterzugeben | ohne Rückweg über eine Stelle |
| E5 | **dezentral** | keine Bank, keine einzelne Stelle mit Geheimnis |

Kein bekannter Weg erfüllt 2026 alle fünf mit heutiger Hardware. Abschnitt 4 zeigt,
welcher Weg welche Eigenschaft wann erreicht.

## 2. Stand bei Aequitas

| Teil | Stand |
|---|---|
| Register (Sätze, Noten), Anmelden mit Deckung in AEQ, StateRoot, Snapshot, Rückroll | live seit 15.11.2026 13:15 UTC |
| Umlaufabgabe im Topf (Noten verlieren anteilig wie jedes Guthaben) | live |
| Einlösen mit Einmal-Unterschrift | gebaut, **gesperrt** (`quantenEinmalPruefer` leer) |
| Freischaltung eines Satzes: ⅔ der Prüfer, **mindestens 5**, hybrid unterschrieben, N ≥ 2²⁵⁶, 30 Tage Vorlauf | gemergt (#534), Deploy wartet auf Freigabe |
| QG3: Noten je Zeile in `quanten_noten` | gemergt (#535), Deploy wartet auf Freigabe |
| Abstimmen aus der App | App 1.21+; Anzeige der Mindestzahl ab 1.23.2 (gemergt) |
| Labor: Prägen, Bezahlen (Teleportation), Prüfen auf IBM (Z₈) | gemessen: echt 0,81, Fälschung 0,22, **Lebensdauer ~20 µs** |
| Ionenfalle (Quantinuum) | Läufer gebaut, **wartet auf Zugang** |
| Gutachten | Prüfauftrag geschrieben (`docs/QUANTENGELD_PRUEFAUFTRAG.md`), nicht vergeben |

## 3. Warum der bisherige Weg (öffentlich prüfbar) noch nicht trägt

**H1 – Kryptografie.**
- **Öffentliche Prüfung:** Zhandry (2023, überarbeitet 2025) gibt öffentlich prüfbares Geld
  *und* Lightning aus abelschen Gruppenaktionen, sicher im generischen Modell (V).
  - Einen Bruch dieses Verfahrens habe ich bis 11/2026 nicht gefunden.
  - Der Angriff von Kim et al. (Quantum, 11/2026) trifft das andere Isogenie-Verfahren
    (Montgomery & Sharif). Er bleibt exponentiell (V).
  - Die Hartley-Variante (Doliskani et al., 3/2026) vereinfacht die Amplituden (V).
- **Einlösen:** Dafür braucht die Kette eine **Einmal-Unterschrift**.
  - Shmueli & Zhandry (CRYPTO 2025): bedingungslos nur im Orakelmodell, sonst iO + LWE (V).
  - Huang & Vaikuntanathan (ePrint 2025/1906, arXiv 2510.10899, v2 vom 27.07.2026) machen
    sie viel kleiner: Schlüssel Θ(λ²) statt Θ(λ⁴) Qubits, perfekte Korrektheit. Im
    Standardmodell bleibt es aber bei **iO + subexponentiellem LWE** (Satz 1.1, V).
    Suchmaschinen fassen das fälschlich als „nur LWE“ zusammen.
  - Muraleedharan et al. (arXiv 2606.23612, 10/2026) liefern erstmals Schaltkreise, aber
    **vor** der Verschleierung (V):
    - 206 logische Qubits bei λ = 2 bis 2.438 bei λ = 10, je 1 Bit Nachricht;
    - Schlüsselerzeugung mit 2,3·10⁵ (λ = 2) bis rund 7·10⁸ Gattern (λ = 9 und 10).
    - Sicher wird es erst mit Verschleierung und einem neuen Beweis.

**H2 – Speicher.**
- **Rauschen:** Eine öffentlich prüfbare Note besteht nach der Lagerung genau mit
  (1 − p)^m. Das heißt, jedes Qubit muss m-mal besser sein (`rauschen_oeffentlich.py`):
  64 Qubits → Kohärenz 3.184 × Lagerzeit; für einen Tag ≈ 9 Jahre.
- **Auf IBM:** Fehlerkorrektur im Speicher hilft dort nicht. Der Code kostet mehr, als er
  rettet (`HARDWARE_WEG.md`).
- **Was nötig ist:** fehlerkorrigierte logische Qubits als Speicher. Atom Computing zeigt
  2026 wiederholte Fehlerkorrektur im torischen Code über 90 Runden mit Nachladen
  verlorener Atome (V). Es ist ein Weg, aber noch kein lagerfähiger Speicher.

**H3 – Prüfaufwand.** Eine Note zu prüfen kostet bei 8.192 Bit ≈ 2⁵⁵ Toffoli
(`isogenie_kosten.py`). Das ist die Klasse fehlertoleranter Rechner, wie sie die
Hersteller für die Zeit um 2029 und später ankündigen (S, Roadmaps).

**H4 – Weitergabe.**
- Über Entfernung braucht es ein Quantennetz mit Speichern (420 km im Labor, 2026).
- Von Hand zu Hand braucht es ein **Gerät mit Speicher**. Genau das ist das „Quanten-Token“
  der deutschen BMFTR-Projekte (Abschnitt 6).

**H5 – Zugang, Geld, Gutachten:** Abschnitt 6.

## 4. Alle Lösungswege im Vergleich

| Weg | E1 unfälschbar | E2 offline | E3 privat | E4 Hand zu Hand | E5 dezentral | Hardware heute | Sicherheit beruht auf | Zeithorizont |
|---|---|---|---|---|---|---|---|---|
| **A** Öffentlich prüfbar + Lightning (bisher) | ja | ja | ja | ja (mit Gerät) | ja | nein (H1–H3) | Gruppenaktion, iO + LWE für Einlösen | fehlertolerante Ära |
| **B** **Variante W**: privat prüfbar, klassische Prüfung, Geheimnis verteilt | ja | **teilweise**: klassisch online, kurze Nachricht | teilweise | ja (Gerät) | ja (Schwelle der Validatoren) | **Labor heute** (Speicher > 10 h, Token-Geräte im Bau) | No-Cloning, ohne Rechenannahme; Schwelle t von n | **nah**: Demonstrator mit Partnern |
| **C** S-Token ohne Speicher (Kent et al.) | ja | nein (Raumzeit-Prüfung im Netz vertrauter Stellen) | ja | nein | nein (vertraute Stellen) | **ja**, voll gezeigt 2024 (V) | No-Cloning + Relativität | heute, aber kein Bargeld |
| **D** Quanten-digitale Zahlungen (Schiansky et al.) | ja | nein (online) | ja | nein | nein (Bank) | **ja**, über Stadtglasfaser gezeigt (V) | No-Cloning | heute, aber Kartenzahlung |
| **E** Quanten-PUF (physikalische Fingerabdrücke) | teilweise | ja | – | – | ja | Labor | Unkenntnis der Struktur | Baustein, kein Geld |
| **F** **Offline-Bargeld** (Secure Element + Haftung über „ein Mensch, ein Konto“) | nein (Betrug beweisbar, nicht unmöglich) | **ja** | teilweise | ja | ja | **ja** (Android Keystore) | Haftung + Pfand + Geräteschlüssel | **jetzt** (Stufe 1 fertig) |
| **G** Post-Quanten-E-Cash (Gitter-Blindsignaturen) | rechnerisch | teilweise | **ja** | nein | Schwelle möglich | ja, aber schwer (bis 199 MB je Transaktion in einer Variante, S) | Gitter | Forschung, 1–3 Jahre |
| **H** Noten im Quanten-Tresor (verwahrt) | ja | nein | nein | nein | nein | Labor | – | kein Gewinn gegenüber der Kette |

**Lesart:**
- **F** ist der Weg für Nutzer *jetzt*.
- **B** ist der Quantenweg mit echter Aussicht in den nächsten Jahren.
- **A** bleibt das Fernziel, das als einziges alle fünf Eigenschaften hat.
- **C und D** sind ausgereift, lösen aber andere Probleme (vertraute Stellen, online).
- **H** bringt nichts, was die Kette nicht schon kann.

## 5. Variante W im Detail

**Grundidee:** Wiesners Noten aus Zuständen in geheimen Basen, geprüft nicht durch
Rücksenden der Note, sondern **klassisch**. Wer die Note hält, misst auf eine
Aufforderung hin einen Teil der Qubits in vorgegebenen Basen und schickt nur die
Messwerte. Die Theorie dazu:
- Gavinsky 2011: klassische Prüfung, sicher gegen adaptive Angreifer (V);
- Molina, Vidick & Watrous 2012: optimale Fälschung (3/4)ⁿ, auch für die klassische
  Variante untersucht (V);
- Kumar 2019: wiederverwendbare Noten, ohne Rechenannahme sicher, Rauschen bis 21,4 %,
  Prüfung mit passiver linearer Optik (V);
- Bozzio et al. 2017: experimentell umgesetzt mit schwachen kohärenten Lichtpulsen (V).

**Das Neue für Aequitas:** Die „Bank“, die die Basen kennt, ist **keine Stelle**. Das
Geheimnis jeder Note liegt in Anteilen (t von n) bei den Validatoren und wird nie
zusammengesetzt. Die Prüfung rechnen sie gemeinsam (MPC). Einen vergleichbaren Aufbau
mit verteiltem Schlüssel habe ich in der Literatur nicht gefunden (S). Deshalb gehört
er zuerst in eine Simulation und vor ein Gutachten.

**Ablauf:**
1. **Prägen:**
   - Eine Prägestelle erzeugt n Qubits in zufälligen BB84-Zuständen. Die Zufallsbits
     kommen aus gemeinsamer Zufälligkeit der Validatoren, damit die Stelle sie nicht
     wählen kann.
   - Basen und Werte gehen nur als Anteile an die Validatoren. Seriennummer und Deckung
     stehen auf der Kette: Register, Topf und Umlaufabgabe sind schon gebaut.
2. **Halten:** in einem Token-Gerät mit Speicher.
3. **Prüfen beim Bezahlen:**
   - Der Empfänger fordert eine Prüfung an. Die Kette wählt Teilmenge und Basen aus
     gemeinsamer Zufälligkeit.
   - Das Gerät misst und sendet die klassischen Werte. Die Validatoren vergleichen sie im
     MPC und bescheinigen „echt“ oder „falsch“ mit ⅔-Mehrheit, wie die
     Lebendigkeitsbescheinigung.
   - Jede Prüfung verbraucht die gemessenen Qubits. Die Zahl der Prüfungen je Note ist
     begrenzt.
4. **Weitergeben:** Das Gerät wechselt den Besitzer, der Empfänger prüft wie in Schritt 3.
5. **Erneuern oder Einlösen:** nach k Prüfungen oder nach einer Frist. Eine Prüfstelle
   prüft vollständig, und die Note wird neu geprägt oder in AEQ eingelöst.

**Simulation (`wiesner_verteilt.py`, Selbsttest in der CI):**
- **Aufbau:** Shamir t von v (ein Polynom vom Grad t − 1, beliebige t Anteile setzen
  zusammen). Prüfen auf den Anteilen: Basisvergleich und Bitvergleich sind linear, je
  Stelle eine Beaver-Multiplikation. Geöffnet werden nur zwei Zähler (Treffer, Fehler).
- **Gezeigt:**
  - t − 1 Anteile verraten nichts (in F₁₁ exakt nachgezählt);
  - die gemeinsame Prüfung zählt genau wie eine Bank, die alles weiß;
  - echte Noten bestehen, auch mit 2 % Rauschen bei 15 % Toleranz;
  - abgewiesen werden: Fälschen ohne Note, Raten mit Rückmeldung (A3), alte Antworten
    (A5), zu viele Prüfungen (A6), mehrfaches Einlösen (A7);
  - der einfache Kopierer besteht mit 25/32 je Qubit. Er liegt damit unter der
    bewiesenen Schranke.
- **Gegenproben,** jede schlägt an: ohne Sperre, ohne Verbrauch gemessener Qubits, mit
  vorhersagbarer Aufforderung, mehrfaches Einlösen, falscher Basisvergleich, Toleranz
  ignoriert.
- **Fund aus der Simulation:** Sperrt eine Ablehnung die Note (nötig gegen Raten), kann
  jeder, der die Seriennummer kennt, eine fremde Note mit Unsinn-Antworten sperren.
  - **Ausweg:** Der echte Halter löst eine gesperrte Note trotzdem ein, mit vollständiger
    Prüfung aller unbenutzten Qubits.
  - **Grenze:** Einlösen ist genau ein Versuch, danach ist die Note eingelöst oder tot.
    Raten über das Einlösen geht so nicht.

**Sicherheit, ehrlich:**
- **Fälschen ohne Geheimnis:**
  - Mit Rückgabe der Note an eine Prüfstelle gilt höchstens (3/4)ⁿ, für 2⁻¹²⁸ also
    309 Qubits.
  - Bei **klassischer** Prüfung besteht der beste Fälscher zwei Prüfungen aus einer
    Note mit höchstens (3/4 + √2/8)ⁿ ≈ 0,927ⁿ (MVW 2012, V). Für 2⁻¹²⁸ sind das
    **1.167 Qubits**.
  - Beides ohne Rauschen gerechnet; mit Rauschen gibt Kumar 2019 die Schranken.
  - Die Simulation `wiesner_verteilt.py` gibt diese Tabelle aus.
- **Adaptiver Angriff (Brodutch et al. 2014):** Er bricht Wiesner, wenn eine geprüfte
  Note zurückkommt und eine abgewiesene zerstört wird (V). Gegenmaßnahmen:
  - ein abgelehntes Ergebnis sperrt die Note auf der Kette;
  - begrenzte Prüfungen;
  - jede Prüfung misst neue, zufällig gewählte Qubits.

  Das muss die Simulation zeigen.
- **Absprache von t oder mehr Validatoren:** Sie können Noten fälschen. Das ist dieselbe
  Vertrauensannahme wie bei Freischaltung und Bescheinigung (⅔). Darunter erfahren sie
  nichts über eine Note.
- **Privatsphäre:** Die Validatoren sehen, welche Seriennummer wann geprüft wird.
  Abhilfen, etwa blinde Prüfanfragen, sind zu untersuchen. E3 ist also nur teilweise
  erfüllt.
- **Netz:** Die Prüfung braucht eine kurze klassische Verbindung. Ohne Netz kann der
  Empfänger erst später prüfen. E2 ist damit nur teilweise erfüllt; dafür trägt das
  Offline-Bargeld.

**Hardware:**
- **Forderung:** Kohärenz 2,5–10 × Lagerzeit (`rauschen.py`, Wiesner mit Rückgabe)
  statt 3.184 × (öffentlich, 64 Qubits). Für die klassische Prüfung ist die Rechnung
  mit den Schranken von Kumar 2019 noch zu machen. Weil die Fälschungsschranke je Qubit
  schwächer ist (0,927 statt 0,75), wird der Abstand zum ehrlichen Rauschen kleiner.
- **Labor:** Kernspins im Festkörper > 10 h (Wang et al., PRX Quantum 2025), 39 min bei
  Raumtemperatur (Si-28, 2013) (V).
- **Damit:** Eine Note, die einige Stunden hält, ist im Labor erreichbar, eine, die Tage
  hält, noch nicht.
- **Prüfgerät:** Für die klassische Prüfung reicht nach Kumar 2019 passive lineare Optik.

**Was die Kette dafür bräuchte** (nichts davon ist gebaut):
- Satzart „privat“ neben „öffentlich“;
- verteiltes Prüfgeheimnis: verteilte Schlüsselerzeugung und Anteile je Note;
- Prüf-Transaktion mit Aufforderung aus gemeinsamer Zufälligkeit, MPC-Urteil und
  ⅔-Bescheinigung;
- Zähler der Prüfungen je Note, Sperre nach Fehlschlag, Erneuern.

Register, Topf, Abgabe, Freischaltung und Einlösen in AEQ gibt es schon.

**Offene Frage, nachgetragen am 16.11.2026: Wer prägt, kennt das Geheimnis.**
- **Das Problem:** Das Gerät, das die Qubits in die BB84-Zustände bringt, muss Basen und
  Bits im Klartext kennen. Wer es betreibt, könnte sie mitschreiben und jede Note
  beliebig oft nachbauen. Die Teilung unter den Validatoren schützt also nur die Prüfung,
  nicht die Prägung. `wiesner_verteilt.py` hat das bisher nicht gezeigt.
- **Drei Wege, vom schwächsten zum stärksten:**
  1. **Vertraute Prägestelle** (Labor, Stufe W4 unten): Ein Partner prägt und löscht das
     Geheimnis nach dem Teilen. Das ist eine Vertrauensannahme wie bei einer
     Notenbank. Sie reicht für einen Versuch, nicht für echtes Geld.
  2. **Mehrere Prägestellen je Note:** Jede prägt einen Teil der Qubits, und keine kennt
     die ganze Note. Fälschen braucht dann die Absprache aller beteiligten Stellen.
  3. **Fern-Präparation (remote state preparation):**
     - Die Prägestelle erzeugt verschränkte Paare und schickt je eine Hälfte über eine
       kurze Quantenleitung an Validatoren, wie bei QKD.
     - Jeder Validator misst seine Hälften in eigenen Zufallsbasen. Damit legt er fest,
       in welchem Zustand die andere Hälfte steht, und nur er kennt ihn.
     - Die Prägestelle erfährt nichts. Jeder Validator kennt nur seinen Teil der Note.
     - Das ist mit heutiger QKD-Technik im Labor machbar und gehört zum Partnerversuch.

**Stufenplan der Variante W:**

| Stufe | Was | Wer | Stand |
|---|---|---|---|
| W0 | Simulation, Analyse, Schaltkreise | Aequitas | fertig (`wiesner_verteilt.py`, `wiesner_hardware.py`) |
| W1 | Fehlerraten auf echter Hardware (IBM, Ionenfalle) → Notengröße und Toleranz | Aequitas; Läufe mit Freigabe | IBM-Lauf bereit; Ionenfalle braucht ein Konto |
| W2 | Kettenseite schlafend: Satzart „privat“, verteilte Schlüsselerzeugung, Prüf-Transaktion, Zähler, Sperre, Einlösen | Aequitas | offen |
| W3 | Prägung ohne Vertrauen (Weg 2 oder 3 oben) als Protokoll und Simulation | Aequitas | offen |
| W4 | Partnerversuch: echte Note in einem Speicher (Stunden), Ausgabe über ein Aequitas-Testnetz, Weitergabe des Geräts, klassische Prüfung, Einlösen | BMFTR-Verbund + Aequitas | Kontakt offen |
| W5 | Gutachten (Prüfauftrag um Variante W erweitert), Pilot im Testnetz | extern + Aequitas | offen |
| W6 | Satz „privat“ im Hauptnetz freischalten (⅔ der Prüfer, mindestens 5, 30 Tage Vorlauf), anfangs mit kleiner Höchstdeckung je Note | Prüfer | erst nach W4 und W5 |

## 6. Zugang, Geld, Begutachtung – andere Wege

| Bedarf | Bisher | Andere Wege (Stand laut Quellen, vor Nutzung prüfen) |
|---|---|---|
| **Ionenfalle** (Speicher Sekunden) | Quantinuum Nexus, Konto fehlt | **AQT IBEX Q1** über Open Quantum: 12 Ionen, 98,85 % 2-Qubit-Treue, **50 $ frei alle 90 Tage**, auch über qBraid (S). **IonQ** über Qollab (100 Credits alle 30 Tage, S), Azure (500 $ je Anbieter beim ersten Mal, S), IonQ-Forschungsguthaben bis 10.000 $ (S). Offen: ob IBEX Messungen mitten im Schaltkreis mit Verzweigung kann. Unser Bezahl-Schaltkreis braucht das, Prägen und Prüfen nicht. |
| **Neutralatome** (Kohärenz bis ~40 s, S) | – | Atom Computing über Azure, QuEra (S); Fehlerkorrektur mit Nachladen gezeigt (V) |
| **Lange Speicher, Token-Geräte** | – | **BMFTR „Grand Challenge der Quantenkommunikation“**, sieben Projekte zu Quanten-Token: QuaMToMe, HybridQToken, Q.TOK, QPIS, DIQTOK, NEQSIS, Q-ToRX (V). Beteiligt u. a. WMI/TU München, MCQST, PTB, KIT, Stuttgart, Ulm, HU Berlin, Heidelberg, Regensburg. Sie sagen selbst: Eine ganze Token-Infrastruktur von der Ausgabe bis zur Prüfung ist noch nicht gezeigt (V). Aequitas könnte genau diesen Teil liefern (Ausgabe, Register, Prüfung, Einlösen auf einer echten Kette). |
| **Geld** | – | **Unitary Foundation**: Kleinförderung 4.000 $, laufend (S). **Prototype Fund** (DE): bis 47.500 € / 6 Monate, jährlich im Herbst (S). **NLnet/NGI Zero** (wechselnde Fristen). Forschungsguthaben der Cloud-Anbieter (oben). Über einen Partner im Verbund: dessen BMFTR-Mittel. |
| **Begutachtung** | externer Auftrag, offen | (1) Fachkontakt im Verbund, z. B. **Juliane Krämer** (Regensburg), Autorin des Abschnitts „Quanten-Token in der Post-Quanten-Welt“ (V). (2) Veröffentlichung als IACR-ePrint mit offenem Aufruf. (3) Abschlussarbeit an einem Lehrstuhl (Bachelor/Master). (4) Kopfgeld für die Kettenseite. Ein Gutachten ersetzt keiner dieser Wege, aber alle senken den Preis. |

## 7. Zeithorizonte

| Weg | Was | Wann (Schätzung, nicht zugesagt) |
|---|---|---|
| F Offline-Bargeld | Stufen 2–5 (Knoten-API, App-Modul, Schuld aus Eingängen, Stichtag) | Wochen bis Monate, nur Software |
| B Variante W | Simulation und Spezifikation der Kette | Tage bis Wochen (kann ich jetzt) |
| B Variante W | Demonstrator mit echtem Speicher und Partner | 1–3 Jahre, hängt am Verbund |
| A öffentlich prüfbar | lagerfähige Note und Prüfer in kryptografischer Größe | fehlertolerante Ära, frühestens Ende der 2020er |
| A Einlösen | Einmal-Unterschrift ohne iO oder praktikables iO | offene Forschung |

## 8. Empfehlung und nächste Schritte

**Kann ich ohne dich tun:**
1. ~~Simulation Variante W~~ – **erledigt** (`wiesner_verteilt.py`, Abschnitt 5). Offen
   bleiben:
   - die Rauschschranken nach Kumar 2019 statt des einfachen Modells;
   - blinde Prüfanfragen für mehr Privatsphäre.
2. **Spezifikation der Kettenseite W** als ausführbares Modell, wie `bruecke_kette.py`.
   Danach schlafend im Knoten, wie bisher.
3. **Offline-Bargeld Stufe 2** (Knoten-API, Zertifikat, Stand) weiterbauen.
4. **Vorlagen:** Das deutsche Anschreiben an den BMFTR-Token-Verbund und der Entwurf
   für die Unitary Foundation stehen in `ANSCHREIBEN.md` (Vorlagen 2 und 3). Offen ist
   noch der ePrint-Entwurf zur Variante W.

**Brauche ich von dir:**
1. **Konto bei Open Quantum** (frei) oder Quantinuum. Dann laufen Prägen und Prüfen auf
   einer Ionenfalle. Das Secret kommt in die Umgebung `quantenhardware`.
2. **Kontakt aufnehmen** zum Verbund und zu Forschenden. Die Vorlagen schreibe ich, du
   schickst sie ab.
3. **Entscheiden,** ob Variante W als zweite Satzart auf die Liste kommt. Bis zu deiner
   Entscheidung baue ich nur Simulation und Spezifikation, nichts Scharfes.

## Quellen

Selbst gelesen (V):
- Huang, Vaikuntanathan: *A Simple and Efficient One-Shot Signature Scheme*, arXiv 2510.10899 v2 (27.07.2026) / ePrint 2025/1906 – Satz 1.1: Standardmodell mit pqIO + subexp. LWE.
- Muraleedharan et al.: *A quantum algorithm for one-shot signatures*, arXiv 2606.23612 v2 (14.10.2026) – Tab. II, IV.
- Kukharchyk et al. (BMFTR-Verbund): *Practical quantum tokens: challenges and perspectives*, arXiv 2602.10621 (11.02.2026).
- Zhandry: *Quantum Money from Abelian Group Actions*, arXiv 2307.12120 v5 (2025).
- Kim, Heo, Hong, Lee, Kim: *Cryptanalysis of Isogeny-Based Quantum Money with Rational Points*, arXiv 2508.00351 v3 (05.11.2026).
- Doliskani, Mirzaei, Mousavi: *Public-Key Quantum Money and Fast Real Transforms*, arXiv 2503.18890 v4 (13.03.2026).
- Goldin, Morimae: *Quantum Key Exchange from Public Quantum Money*, arXiv 2611.05985 (04.11.2026).
- Atom Computing et al.: *Quantum error correction with the toric code*, arXiv 2606.04079 (02.06.2026).
- Gavinsky: *Quantum Money with Classical Verification*, arXiv 1109.0372.
- Molina, Vidick, Watrous: *Optimal counterfeiting attacks and generalizations for Wiesner's quantum money*, arXiv 1202.4010.
- Brodutch, Nagaj, Sattath, Unruh: *An adaptive attack on Wiesner's quantum money*, arXiv 1404.1507.
- Kumar: *Practically feasible robust quantum money with classical verification*, arXiv 1908.04114.
- Bozzio et al.: *Experimental investigation of practical unforgeable quantum money*, arXiv 1705.01428.
- Jiang, Kent, Pitalúa-García et al.: *Experimental practical quantum tokens with transaction time advantage*, arXiv 2408.13063.
- Schiansky et al.: *Demonstration of quantum-digital payments*, arXiv 2305.14504.
- Yuen: *Noise-tolerant public-key quantum money from a classical oracle*, arXiv 2407.06463.
- Wang et al.: *Nuclear spins in a solid exceeding 10-hour coherence times*, PRX Quantum 6, 010302 (2025) – über die Literaturliste von 2602.10621.

Nur über Suchergebnisse (S), vor Nutzung prüfen:
- Open Quantum / AQT IBEX Q1: https://www.openquantum.com/providers/aqt , https://www.aqt.eu/quantum-rings-and-aqt-partner-to-bring-the-ibex-q1-trapped-ion-quantum-computer-to-open-quantum-platform/
- qBraid, AQT-Geräte: https://docs.qbraid.com/v2/sdk/user-guide/providers/aqt
- Qollab: https://qollab.xyz/explore/quantum-cloud-computing
- IonQ Quantum Cloud (Forschungsguthaben): https://www.ionq.com/quantum-cloud
- Azure Quantum Preise/Guthaben: https://learn.microsoft.com/azure/quantum/pricing
- Unitary Foundation / Deltakit-Fonds: https://thequantuminsider.com/2026/07/28/riverlane-unitary-foundation-deltakit-community-fund/
- Prototype Fund: https://www.prototypefund.de/en/funding
- Neutralatome (Kohärenz, Anbieter): https://postquantum.com/quantum-modalities/neutral-atom-quantum/
- Gitter-Blindsignaturen / E-Cash: https://eprint.iacr.org/2024/1289.pdf
