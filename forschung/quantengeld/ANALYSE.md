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
| **C** S-Token ohne Speicher (Kent et al.) | ja | nein (Raumzeit-Prüfung im Netz vertrauter Stellen) | ja | nein | nein (vertraute Stellen) → **ja als Variante S** (Abschnitt 5b) | **ja**, voll gezeigt 2024 (V) | No-Cloning + Relativität | heute, aber kein Bargeld |
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
  - **Grenze:** In der Simulation W0 ist Einlösen genau ein Versuch. Auf der Kette (W2b)
    ist ein falsches Einlösen ein Fehlversuch mit Pfand, und die Note bleibt; sonst könnte
    jeder Fremde jede Note für 1 AEQ töten (siehe unten).

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

**Ausführbare Regeln (W2a, geschärft für W2b am 16.11.2026):** `wiesner_kette.py` ist die
Spezifikation. Je Stelle steht beim Prägen nur eine gesalzene Prüfsumme auf der Kette
(Merkle-Wurzel); das Geheimnis (Basis, Bit, 124 Bit Salz) liegt als Shamir-Anteile (t von v)
bei den Validatoren der Note. Eine Prüfung: **Anfrage** mit Pfand; **Zufall**: jeder Validator
der Note legt den nächsten Wert seiner Hash-Kette offen, deren Anker er beim Prägen bestätigt
hat – erst wenn alle da sind, stehen Stellen und Basen fest, und ab da sind die Stellen
verbraucht; **Antwort** des Anfragenden innerhalb von 10 Minuten nach dem Zufall; **Offenlegen** von t
Anteilen je Stelle. Jeder Knoten setzt selbst zusammen, prüft Prüfsumme und Pfad und urteilt;
ein falscher Anteil fällt an der Prüfsumme auf, ein falscher Zufall an der Hash-Kette.
Weniger als 4 Treffer: unentschieden (Pfand zurück, kein Fehlversuch). 3 Fehlversuche sperren
die Note für Prüfungen; Einlösen geht auch gesperrt.

Drei Lücken der ersten Fassung, vor dem Einhängen geschlossen:

1. **Zufall:** Der Blockhash als Quelle der Aufforderung war vom Erzeuger steuerbar (S1 der
   Prüfung von #539). Die Hash-Ketten stehen seit dem Prägen fest. Für den Zufall gibt es
   keine Frist: Die Anfrage bleibt offen, ihr Pfand gesperrt, bis alle Werte da sind. Mit
   einer Frist (Pfand zurück, Stellen trotzdem verbraucht) hätte ein Erzeuger, der einen Wert
   zurückhält, eine Note kostenlos erschöpfen können.
2. **Prüfgröße beim Einlösen:** Wählbar hieß, ein Fälscher wählt k = 8 und kommt mit Raten in
   ~3 % durch. Jetzt gilt beim Einlösen fest k = min(64, freie Stellen). Prüfungen lassen
   min(64, n) Stellen als Reserve für das Einlösen stehen.
3. **Note töten für 1 AEQ:** Ein falsches Einlösen machte die Note tot, und jeder Fremde konnte
   das versuchen. Jetzt zählt es wie eine falsche Prüfung (Pfand weg, Fehlversuch), und die
   Note bleibt. Das Pfand trägt den Wert der verbrauchten Stellen: max(1 AEQ, Wert · k / n).
   Bleiben weniger als 8 Stellen frei, ist die Note erschöpft und ihr Wert geht ans
   Grundeinkommen. Dahin kommt nur, wer zusammen etwa den Wert der Note an Pfand verloren hat.

Jeder Validator bestätigt beim Prägen Wurzel, Anker und Prägezeit. Eine Bestätigung gilt
höchstens einen Tag, und K bleibt belegt: Eine tote Note lässt sich nicht mit alten Ankern neu
prägen, deren Hash-Ketten teils schon offen sind. Eine Note hat mindestens 8 Stellen (sonst wäre
sie nie prüfbar und ihre Deckung läge für immer im Topf). Selbsttest mit Missbrauchsfällen; der Knoten rechnet in Go genau gleich
(`quanten_w_test.go` gegen `testdata/quanten_w_vektor.json`).

**Grenze, ehrlich:** Der Zufall braucht alle v Validatoren der Note. Fällt einer für immer aus,
steht die Note: Es geht kein Wert verloren, aber es kommt auch keiner heraus (fail-closed).
Eine Schwellen-Signatur (t von v) wäre lebendiger; das ist eine spätere Stufe.

**Befund 16.11.2026 abends: Die Toleranz von 15 % ist gegen einen Kloner gebrochen**
(`wiesner_parameter.py`).
- **Der Angriff:** Wer eine echte Note hat, klont jede Stelle mit dem optimalen
  phasenkovarianten Kloner (Bruß u. a. 1999). Beide Kopien haben dann **14,64 %**
  Fehler je Treffer, in jeder BB84-Basis (nachgerechnet am Zustandsvektor). Das liegt
  **unter** 15 %.
- **Was dabei herauskommt:** Gegen die heutigen Regeln (k ≤ 64, Mindesttreffer 4)
  besteht eine geklonte Kopie mit **0,55**, beide Kopien mit rund 0,3. Der Fälscher
  bezahlt zweimal, nur einer der Empfänger kann einlösen.
- **Die Literatur sagt dasselbe:** Klassisch geprüfte Quanten-Token sind nur sicher,
  wenn weniger als 14,6 % Fehler erlaubt sind (Pastawski u. a., PNAS 2012, cv-qtickets:
  Treue-Schwelle (1 + 1/√2)/2).
- **Das alte Modell war zu günstig:** Es ließ einen Fälscher raten (50 % Fehler) und
  kam so auf 3,8·10⁻⁵. Ein Kloner ist viel besser als Raten.
- **Schaden:** keiner. Die Kettenseite ist schlafend.

**Sichere Parameter** (Fälscher ≤ 2⁻⁴⁰ je Prüfung, echte Note fällt ≤ 10⁻⁶ durch, exakt
gerechnet; die Tabelle gibt `python3 -I wiesner_parameter.py` aus):
- **Die Toleranz muss deutlich unter 14,6 % liegen** und knapp über der Fehlerrate der
  Hardware. Bei der auf IBM gemessenen Fehlerrate (0,7 % ohne Warten) sind das **4 %**.
- **Die Prüfung muss groß sein:** bei 4 % **k = 840** Stellen (rund 420 Treffer).
  `K_MAX` = 64 und `N_MAX` = 2048 reichen dafür nicht. Nötig sind k im Bereich von
  Tausend und Noten mit einigen Tausend bis Zehntausend Stellen.
- **Eine Absprache, die den Anteil f der Stellen kennt,** senkt die Fehlerrate des
  Fälschers auf (1 − f)·14,6 %. Mit vielen Validatoren je Note bleibt f klein.

  | Hardware-Fehler e | Absprache f | Toleranz | k (geprüfte Stellen) |
  |---|---|---|---|
  | 0,4 % | 0 / 1/64 / 0,2 | 3 % | 672 / 696 / 1.008 |
  | 0,7 % (IBM ohne Warten) | 0 / 1/64 / 0,2 | 4 / 4 / 3 % | 840 / 864 / 1.072 |
  | 2 % | 0 / 1/64 / 0,2 | 6 / 5 / 5 % | 1.360 / 1.376 / 1.864 |
  | 3 % (mit Bezahlen) | 0 / 1/64 / 0,2 | 7 / 6 / 6 % | 1.792 / 1.864 / 2.672 |
  | 6 % (20 µs gehalten) | 0 / 1/64 / 0,2 | 9 / 9 / 8 % | 3.464 / 3.728 / 6.976 |

  Gegen die alten Regeln besteht eine geklonte Kopie bei k = 16 / 32 / 64 mit
  0,58 / 0,55 / 0,55; beide Kopien zusammen mit rund 0,3.
- **Umgesetzt (W2c, 16.11.2026 abends), in Spezifikation und Knoten:**
  1. **Toleranz je Note:** 2, 3, 4, 5, 6, 8 oder 10 % (`tol`). Die Validatoren
     bestätigen sie mit.
  2. **Prüfgröße k_note:** kommt aus der Tabelle
     `K_TABELLE[tol][f16]` mit f16 = ⌈16(t−1)/v⌉. Eine Absprache unter der Schwelle kennt
     höchstens diesen Anteil der Stellen. Die Tabelle ist exakt gerechnet, als ganze Zahlen
     ohne Gleitkomma im Konsens.
  3. **Unsichere Felder sind 0,** und dafür wird das Prägen abgewiesen. Ein Beispiel:
     t = 3 und v = 5 erzwingen bei 4 % k = 3.472.
  4. **Einlösen genau mit k_note.** Prüfungen lassen k_note Stellen und den letzten Wert
     der Hash-Ketten frei. Eine Note ist erschöpft, wenn weniger als k_note Stellen frei
     sind oder die Ketten verbraucht sind.
  5. **Offenlegen in Teilen** zu höchstens 64 Stellen, der Reihe nach, jeder Teil ganz
     oder gar nicht. Geurteilt wird erst über alle k.
  6. **Neue Grenzen:** `N_MAX` = 16.384, `K_MAX` = 8.192, Pfad ≤ 14, Register höchstens 16
     Noten, benutzte Stellen als Bitfeld.
  7. **Prüfung:** Knoten und Spezifikation sind per Prüfvektor gleich (inkl. Tabelle).
     Jede neue Regel hat einen Missbrauchstest und eine Gegenprobe durch Mutation.
  8. **Freischalten erst nach W4/W5.**
- **Grenze der Aussage:** Gerechnet ist der Produktangriff je Kopie. Dass dieselbe
  Schwelle auch gegen verschränkte Angriffe in genau unserem Schema gilt (jede Stelle wird
  nur einmal gefragt), gehört ins Gutachten (W5).

**Stufenplan der Variante W:**

| Stufe | Was | Wer | Stand |
|---|---|---|---|
| W0 | Simulation, Analyse, Schaltkreise | Aequitas | fertig (`wiesner_verteilt.py`, `wiesner_hardware.py`) |
| W1 | Fehlerraten auf echter Hardware (IBM, Ionenfalle) → Notengröße und Toleranz | Aequitas; Läufe mit Freigabe | Rauschmodell gemessen (HEUTE.md 3.3); IBM-Lauf wartet auf Freigabe; Ionenfalle braucht ein Konto |
| W2 | Kettenseite schlafend: Satzart „privat“, verteilte Schlüsselerzeugung, Prüf-Transaktion, Zähler, Sperre, Einlösen | Aequitas | **W2a fertig** (16.11.2026): Regeln als ausführbare Spezifikation `wiesner_kette.py` und im Knoten `x/humanity/keeper/quanten_w.go`, `quanten_w_register.go` (schlafend), Byte für Byte gleich (Prüfvektor). **W2b** (16.11.2026): Regeln geschärft (#541), eingehängt in Blöcke mit Unterschriften, Buchungen, StateRoot, Snapshot, schlafend (`quanten_w_kette.go`). **W2c** (16.11. abends): sichere Prüfgrößen nach dem Befund Klonangriff (Toleranz je Note, k aus der Tabelle, Offenlegen in Teilen). **W2d**: Dienst der Validatoren (`quanten_w_validator.go`: Tresor, Bestätigung, Anteile erst nach der Antwort, Hintergrund) |
| W3 | Prägung ohne Vertrauen (Weg 2 oder 3 oben) als Protokoll und Simulation | Aequitas | **fertig** (16.11.2026): Weg 3 als Schaltkreis (`wiesner_hardware.note_fern`), Simulation mit Absprache-Rechnung (`wiesner_praegen.py`), Läufer `hardware_ibm.py --wiesner --fern` |
| W4 | Partnerversuch: echte Note in einem Speicher (Stunden), Ausgabe über ein Aequitas-Testnetz, Weitergabe des Geräts, klassische Prüfung, Einlösen | BMFTR-Verbund + Aequitas | Kontakt offen. **Ohne Partner** (Labormaßstab, Note lebt µs): `hardware_ibm.py --wiesner --kette` – Prägen auf der Kette, Bezahlen per Teleportation, Einlösen mit Aufforderung aus den Hash-Ketten der Validatoren, Offenlegen, Urteil, AEQ beim Empfänger; ideal und im Rauschmodell gelaufen, IBM wartet auf Freigabe |
| W5 | Gutachten (Prüfauftrag um Variante W erweitert), Pilot im Testnetz | extern + Aequitas | Prüfauftrag erweitert (`docs/QUANTENGELD_PRUEFAUFTRAG.md`); Gutachter und Pilot offen |
| W6 | Satz „privat“ im Hauptnetz freischalten (⅔ der Prüfer, mindestens 5, 30 Tage Vorlauf), anfangs mit kleiner Höchstdeckung je Note | Prüfer | erst nach W4 und W5 |

## 5b. Ohne Quantenspeicher, heute: Variante S (S-Token, dezentral geprüft)

Nachgetragen am 16.11.2026 abends. Frage des Betreibers: Gibt es unter heutigen Umständen
eine Lösung ohne das Speichergerät?

**Antwort:** Ja, aber mit klarer Grenze. Unfälschbare, private Quanten-Token, die
niemand speichern muss, sind heute machbar. Bargeld, das als Quantenzustand von Hand zu
Hand geht, ist es nicht. Der Weg dorthin sind die **S-Token** (S-money) von Kent u. a.:
„Practical quantum tokens without quantum memories and experimental tests“, npj Quantum
Information 8, 28 (2022), arXiv:2104.11717. Sie wurden mit handelsüblicher QKD-Technik
erprobt.

**So funktioniert ein S-Token (Schema IQT1 der Arbeit):**
1. **Ausgabe.** Die Ausgabestelle B schickt N Qubits in zufälligen BB84-Zuständen
   (Basen u, Werte t). Die Nutzerin A misst **sofort**, jede Stelle in einer zufälligen
   Basis y, und hat danach nur noch Klartext-Ergebnisse x. **Nichts muss gespeichert
   werden.**
2. **Maskierte Basenwahl.** A gibt B ihre Basen nur maskiert: d = y oder d = ȳ, je nach
   einem geheimen Bit z.
3. **Einlösen.** Erst beim Einlösen wählt A eine von 2^M Vorlagestellen (Ort oder
   Zeitfenster), sendet c = b ⊕ z und legt x vor. Geprüft werden nur die Stellen, deren
   Basis zur gewählten Vorlagestelle passt.
4. **Unfälschbar:** Wer an zwei Stellen vorlegen will, braucht die richtigen Ergebnisse
   in beiden Basen. Am besten misst man dafür in der Basis zwischen X und Z
   (Breidbart). Das ergibt **14,6 % Fehler**, genau wie beim Kloner der Variante W.
   Dieselbe Toleranzschwelle gilt also, und `wiesner_parameter.py` rechnet auch hier die
   Prüfgröße (bei 0,7 % Hardware-Fehler: Toleranz 4 %, rund 840 geprüfte Stellen).
5. **Privat:** B erfährt erst beim Einlösen, wo A einlöst.

**Was Aequitas daraus machen kann (Variante S, Entwurf):**
- **Dezentrale Ausgabe:**
  - Die Werte (t, u) kennt nicht eine Bank. Sie entstehen wie bei W3 durch
    Fern-Präparation bei den Validatoren und liegen als Shamir-Anteile bei ihnen
    (`note_fern`, Abschnitt 5).
  - Auf der Kette stehen nur Prüfsummen, wie bei Variante W.
- **Prüfung durch die Kette:**
  - Die Vorlagestelle ist ein Satz von Validatoren bzw. ein Block-Fenster.
  - Beim Einlösen legen die Validatoren die Anteile der geprüften Stellen offen, und
    jeder Knoten urteilt selbst.
  - Das ist derselbe Ablauf wie das Offenlegen der Variante W (`quanten_w*.go`), nur
    ohne den Teil, in dem gemessen und geantwortet wird.
- **Was der Nutzer braucht:**
  - **Bei der Ausgabe:** einen Empfänger, der Einzelphotonen in zwei Basen misst.
    Das ist QKD-Empfängertechnik (handelsüblich; Handgeräte als Prototypen, u. a. Bristol).
  - **Danach:** nur das Telefon mit Klartext-Daten.
- **Heute vorführbar:**
  - Teil 1 (Ausgabe und sofortiges Messen) ist genau der Schaltkreis, der auf IBM schon
    gelaufen ist: Prägen und ohne Warten in zufälligen Basen messen, 0,7 % Fehler.
  - Teil 2 ist reine Klassik und läuft auf der Kette.
  - Eine S-Vorführung auf dem echten Chip geht also **sofort**, mit den vorhandenen
    Werkzeugen.

**Was Variante S nicht ist:**
- **Kein Bargeld von Hand zu Hand.** Weitergeben heißt: klassische Daten übergeben. Der
  Weitergebende kennt sie dann weiter, es gilt „wer zuerst einlöst“. Kent u. a.
  beschreiben übertragbare Varianten mit klassischer Nachricht; ob sie auf eine Kette
  passen, ist offen.
- **Auf einer Kette weniger einzigartig als ohne.** Der Vorteil „sofort gültig ohne
  Rückfrage“ zählt dort, wo nicht auf einen Block gewartet werden kann, etwa an
  Annahmestellen ohne Netz. Die Prüfgeheimnisse müssen dann vorher bei der Annahmestelle
  liegen, und die braucht dafür vertrauenswürdige Hardware (wie beim Offline-Bargeld).

**Einordnung:**
- **Variante S ist das Quantengeld, das unter heutigen Umständen funktioniert:**
  unfälschbar durch Physik, privat, ohne Quantenspeicher, dezentral geprüft.
- **Variante W** bleibt der Weg zu echtem Quanten-Bargeld, sobald Speicher tragen.
- **Beide** brauchen die neuen Prüfgrößen (Befund in Abschnitt 5) und ein Gutachten.
- **Spezifikation `s_kette.py` (16.11. abends):**
  - **Stichproben-Annahmestellen:** Jede Stelle bekommt (u, t) nur auf einer eigenen,
    geheimen, disjunkten Stichprobe von k Stellen. Sie prüft sofort und ohne Netz. Eine
    bestochene Stelle kann an einer anderen Stelle nicht fälschen.
  - **Die Nutzerin braucht kein vertrauenswürdiges Gerät.**
  - **Die Wahl c steht einmal auf der Kette:** Bindung d und Wahl c je einmal; die
    Stellen holen sie vorab. Der Selbsttest zeigt, warum: Dürfte man c je Stelle selbst
    nennen, käme ein Token an beiden Stellen durch.
  - **Missbrauchstests:** zwei Vorlagen (ehrlich gemessen und Breidbart), bestochene
    Stelle, fremde Bindung, zweite Wahl, Vorlage ohne Abgleich, Wiederholung, Abgleich
    auf der Kette. Gegenproben mit alter Toleranz, mit c je Stelle und mit vollem Wissen.
- **Nächste Schritte:**
  1. ~~Spezifikation `s_kette.py`~~ – erledigt (oben), auch die **dezentrale
     Ausgabe** (`stelle_laden`): Jede Annahmestelle lädt ihre Stichprobe als Anteile
     von t Validatoren und prüft jede Stelle gegen Prüfsumme und Merkle-Wurzel auf der
     Kette. Ein falscher Anteil oder Pfad, zu wenige oder zu viele Anteile, ein doppeltes
     x, ein x außerhalb 1..v oder eine fremde Wurzel: Die Stelle lädt nicht. Gegenprobe
     durch Mutation: fünf von fünf erkannt. Offen: Knoten-Regeln.
  2. S-Vorführung auf IBM.
  3. Anschreiben an die Autoren (Cambridge: Kent, Pitalúa-García; Bristol: Rarity) als
     Vorlage 4.

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
