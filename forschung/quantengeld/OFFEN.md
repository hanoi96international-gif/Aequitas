# Offene Punkte – Quantengeld (Stand 16.11.2026, Sitzungsende)

Hauptaugenmerk der nächsten Sitzung: **dezentrales Quantengeld, das unter heutigen
Umständen funktioniert**, damit Aequitas ernstzunehmendes Quantengeld ist. Die Regel
bleibt: Nichts wird behauptet, was nicht gemessen oder bewiesen ist (QUANTENSICHERHEIT.md).

## A. Stand (auf main)

- **Variante W:** privat prüfbar, klassische Prüfung, das Geheimnis liegt als Anteile
  bei den Validatoren.
  - Spezifikation: `wiesner_kette.py`.
  - Regeln im Knoten: `quanten_w.go`, `quanten_w_register.go` (#539).
  - Geschärft und eingehängt, schlafend: `quanten_w_kette.go` (#541, #542).
  - Prägen ohne Vertrauen: `note_fern`, `wiesner_praegen.py`.
  - Ablauf von Anfang bis Ende mit der Kette: `hardware_ibm.py --wiesner --kette`.
  - Workflow-Auswahl: `messen | fern | kette`.
  - Prüfauftrag W5: `docs/QUANTENGELD_PRUEFAUFTRAG.md`.
- **Echte Hardware (IBM `ibm_marrakesh`, HEUTE.md 3.3):**
  - **Ohne Warten:** Der ganze Ablauf läuft mit dezentraler Prüfung. Ergebnis: echt,
    10 AEQ beim Empfänger, Annahme 0,998.
  - **Mit 20 µs Halten und Teleportation:** abgewiesen, 39 % Fehler je Treffer.
  - **3-Qubit-Schutz:** kein messbarer Gewinn.
  - **Prägen ohne Vertrauen (`fern`, `ibm_fez`):** ohne Warten 2,4 % Fehler, Annahme 0,90.
- **Ehrliche Grenze:** Geld zum Mitnehmen gibt es erst mit einem Quantenspeicher. Heute
  lebt eine Note Mikrosekunden.
- **Die frühere Schwäche ist behoben (W2c, #546):**
  - **Befund:** Die alte Toleranz von 15 % bei höchstens 64 Stellen war gegen einen
    optimalen Kloner gebrochen (14,6 % Fehler je Kopie). Eine geklonte Kopie bestand mit
    0,55.
  - **Jetzt:** Toleranz je Note und k_note aus der exakten Tabelle. Ein Fälscher kommt
    damit höchstens mit 2⁻⁴⁰ durch.

## B. Zuerst: die Prüfung deutlich stärker machen

**Stand 16.11. abends:** Die Punkte 1–4 und 7 sind in Spezifikation und Knoten
umgesetzt (W2c: Toleranz je Note, k_note aus der exakten Tabelle mit Absprache-Grenze
f = (t−1)/v, Offenlegen in Teilen, neue Grenzen). Punkt 5 (mehrstufige
Prüfung) ist gerechnet, der Einbau in Spezifikation und Knoten ist offen (W2e). Punkt 6 ist beantwortet (17.11.).

1. **Ziel:** Ein Fälscher besteht höchstens mit 2⁻⁴⁰ je Prüfung. Das gilt auch für eine
   Absprache, die einen Anteil f der Stellen kennt (bei Fern-Präparation f = 1/v).
   **Gerechnet (`wiesner_parameter.py`):** Bei e = 0,7 % (IBM ohne Warten) braucht es
   eine Toleranz von 4 % und k = 840. Die Toleranz muss immer deutlich unter 14,6 %
   liegen (Pastawski u. a. 2012).
2. **Toleranz an die gemessene Fehlerrate koppeln:**
   - τ liegt knapp über der Fehlerrate e der Hardware (aus den W1-Läufen), statt fest
     bei 15 %.
   - Daraus k (geprüfte Stellen) und n (Notengröße) exakt rechnen. Dafür
     `faelschen_wahrscheinlichkeit` um eine Tabelle e → τ, k, n erweitern.
3. **`K_MAX` anheben** (64 → einige Hundert) und, wo nötig, auch `N_MAX`:
   - Spezifikation, Knoten und Prüfvektor gleich ziehen, mit Tests.
   - Die Grenzen werden nur bewusst neu gesetzt, nie gelockert, um etwas grün zu
     bekommen.
4. **Mehr Validatoren je Note** (v bis 64): Jeder kennt dann nur einen kleinen Teil der
   Note (kleines f).
5. **Mehrstufige Prüfung** – gerechnet (17.11., `wiesner_mehrstufig.py`):
   - Zwei unabhängige feste Ziehungen sparen nichts (meist mehr Stellen).
   - Ein sequenzieller Test (Wald) hält den Fälscher streng bei 2⁻⁴⁰, auch mit Absprache
     f. Er entscheidet für eine echte Note im Mittel nach 36–64 % weniger Stellen, braucht
     aber höchstens 2–62 % mehr als die feste Prüfung.
   - **Zwei Stufen** (erst s1 Stellen ziehen, nur bei „unentschieden“ eine zweite Ziehung
     bis k_max): eine echte Note verbraucht im Mittel 20–52 % weniger Stellen, nur 4–12 %
     der Prüfungen brauchen die zweite Stufe.
   - **Mit der heutigen Garantie gerechnet (W2e, 17.11.):** gleiche Garantie wie die feste
     Prüfung. Eine volle Prüfung braucht dann im Mittel 16–31 % weniger Stellen, aber k_max
     = 1,5–1,9 × k_note frei. Beim Einlösen hilft es der Lebensdauer nicht.
   - **Entscheidung:** W2e ist eine Optimierung, keine Sicherheitsfrage. Gebaut wird sie
     erst, wenn die Stellen je Note im Pilot der Engpass sind; vorher haben Gutachten und
     Pilot (W6) Vorrang. Grenzen und Tabelle liegen fertig in `wiesner_mehrstufig.py`.
6. ~~**Fehlerkorrektur je Stelle messen**~~ – beantwortet (17.11., HEUTE.md 3.3,
   `wiesner_schutz.py`):
   - Steane-Code je Stelle im Rauschmodell `ibm_marrakesh`: bei jeder Wartezeit
     schlechter als ungeschützt, ohne Warten 0,8 % gegen 0,3 %, nach 20 µs 9,8 % gegen
     3,0 %.
   - Exakt gerechnet hilft der Code mit IBM-Werten nie. Mit Werten einer Ionenfalle hilft
     er bei 0,3–6 % Rohfehler.
   - Der echte Chip bestätigt es (`ibm_fez`, Lauf 43986013444): ohne Warten 4,6 % gegen
     0,6 %, nach 5 µs 11,4 % gegen 5,3 %.
7. ~~**Quantenmechanische Schranke**~~ – erledigt: Die Tabelle rechnet mit dem
   optimalen quantenmechanischen Kloner (14,64 %), nicht mehr mit einem ratenden Fälscher.
   Ob verschränkte Angriffe über mehrere Stellen mehr bringen, ist Frage 1 im Prüfauftrag
   (`docs/QUANTENGELD_PRUEFAUFTRAG.md`).
   **Beantwortet für nicht-adaptive Angriffe (17.11., `kohaerent.py`):** Sie bringen nichts.
   Das Optimum über alle verschränkten Strategien ist ein Produkt. Die strenge Schranke
   für „beide bestehen“ hält alle Tabelleneinträge mit 27–43 Bit Reserve. Korrektur:
   „je Kopie“ war nicht vorsichtig (asymmetrischer Kloner); maßgeblich ist „beide“.
   **Adaptiv bei W (Kopie B nach der Prüfung von A):** Argument in `kohaerent.py`
   (ADAPTIV). Jede Rückmeldung betrifft nur ihre Stelle; mit der Dualität für Strategien
   (Gutoski und Watrous 2007) bleibt das Optimum ein Produkt, also dieselbe Schranke. Das
   Argument liegt dem Gutachten zur Prüfung vor. Bei S entfällt der Fall, weil die
   Stellen ohne Netz prüfen.
   **Möglich, nicht gebaut:** Mit der strengen Schranke reichten bei f = 0 schon k = 550
   statt 840, bei t = 3, v = 4 k = 3.400 statt 5.200. Kleinere Prüfgrößen erst nach dem
   Gutachten.

## C. Weiter analysieren: Lösungen, die ohne Quantenspeicher auskommen

Für jeden Weg gilt dasselbe Vorgehen: die Quelle selbst lesen (Angaben unten noch
ungeprüft), Annahmen und Sicherheit klären, dann entscheiden, was Aequitas davon heute
bauen kann.

1. **Quanten-Token ohne Quantenspeicher (Raumzeit-Token):**
   - Quelle: Kent, Lowndes, Pitalúa-García, Rarity, „Practical quantum tokens without
     quantum memories and experimental tests“ (npj Quantum Information 2022). Dazu
     gehört Kents „S-money“.
   - Idee: Der Halter misst die Zustände **sofort** und hält danach ein klassisches
     Token. Es gilt nur an genau einem der vorgesehenen Orte bzw. Zeitpunkte.
   - Möglicherweise **der heute machbare Weg**: QKD-Technik statt Speicher, Prüfung
     durch die Kette.
   - **Analysiert (16.11. abends, ANALYSE.md 5b „Variante S“):** Der Weg ist heute
     machbar, unfälschbar, privat und dezentral prüfbar, aber kein Hand-zu-Hand-Bargeld.
     Doppelvorlage per Breidbart-Messung: 14,6 % Fehler, also dieselbe Schwelle wie bei
     W. `s_kette.py` steht (Stichproben-Annahmestellen, Wahl c auf der Kette). Die
     S-Vorführung (`--s-token`) ist gelaufen: ideal, im Rauschmodell und **auf echter
     IBM-Hardware** (0,46 % Fehler, angenommen; Doppelvorlage abgewiesen). Das Einlösen
     auf der Kette ist spezifiziert (`KetteS.einloesen`: Vorlage von der Nutzerin
     unterschrieben, Stichprobe gebunden, Anteile offengelegt). **Im Knoten (schlafend):**
     Regeln S2a und S2b Teil 1 (#547); die Einhängung in die Kette als Satzart im W-Modul
     (S2b Teil 2, `quanten_w_s_kette.go`): Ausgabe mit Bestätigung aller Validatoren,
     Bindung, Wahl, Anmeldung der bei der Ausgabe **festgelegten** Annahmestelle (sonst
     könnte ein Dritter die offengelegte Stichprobe aus der Warteschlange kopieren und
     zuerst einlösen), Offenlegen in Teilen, Frist, Wert über Anteile im W-Topf.
     Validator-Dienst für S mit #554. **Offen:** App, Vorlage 4 abschicken (Nutzer).
   - **S3 über eine echte Leitung gerechnet (ANALYSE.md 5c, `wiesner_parameter.py`):**
     - Verluste, Mehrphotonen-Pulse und verkippte Basen sind eingerechnet, nach Kent u. a.
       2022. Ihre Abb. 5 ist nachgerechnet.
     - Kents Handsender ist sicher, sobald Basen und Werte aus dem Zufall der
       Validatoren kommen (β = 0).
     - Die Tabelle für ideale Qubits reicht über eine Leitung nicht: Bei 10° Verkippung
       käme ein Fälscher in 1,4 % durch.
     - **Offen:** Die Tabelle im Knoten braucht θ und den Anteil freier Stellen als
       Eingang. Meldung Λ und öffentliche Teilmenge sind als Bausteine spezifiziert
       (`s_kette.s3_meldung`, `s3_teilmenge`). Die Einbindung in Ablauf und Knoten kommt
       mit einem Gerät.
     - ~~**Lücke: Die Größe der Stichprobe prüfte niemand.**~~ Geschlossen: Die Annahmestelle
       lädt nur mit mindestens k_note Stellen (`stelle_laden`, fail-closed). Der Knoten
       bleibt bewusst ohne diese Prüfung, denn beim Anmelden ist die Annahme schon
       geschehen (ANALYSE.md 5c). Für die App der Annahmestelle gilt dieselbe Regel.
   - ~~Zu klären: „Ort“ auf Aequitas~~ – analysiert (ANALYSE.md 5d, C1). Ort ist die feste
     Annahmestelle, raumartige Trennung gibt es auf einer Kette nicht. Es bleibt
     Unfälschbarkeit ohne Rechenannahme, Privatsphäre und Annahme ohne Netz nach dem
     Abgleich von c.
2. **Zustellen statt Aufbewahren:**
   - Die Note geht über ein QKD-Netz (EuroQCI-Testnetze, kommerzielle Strecken) direkt
     zum Prüfer oder Empfänger und wird dort sofort gemessen.
   - Das ergibt einen Quanten-Scheck, kein Bargeld.
   - ~~Zu klären: Welche Strecken sind heute zugänglich?~~ – analysiert (ANALYSE.md 5d, C2).
     Die kommerziellen Dienste liefern nur Schlüssel aus geschlossenen Geräten. Es braucht
     ein Testbett oder einen Partner, wie bei S3. Anfragen (QuNET, T-Labs, Berliner
     Testbett) macht der Betreiber.
3. **Praktisches Quantengeld mit schwachen Lichtpulsen:**
   - Quelle: Bozzio u. a., „Experimental investigation of practical unforgeable quantum
     money“ (npj QI 2018).
   - ~~Vergleichen~~ – erledigt (ANALYSE.md 5c): Bei ihnen gelten Mehrphotonen-Pulse als
     frei, µ ≤ 0,40, gemessen 3–5 % Fehler, 10⁴–10⁵ Paare. Das ist dasselbe Muster wie
     S3, aber mit Speicher für echte Karten.
4. **Halb-quantisches Geld (Radian & Sattath):**
   - Bank und Prüfung sind rein klassisch, das passt zur dezentralen Prüfung.
   - Beim Halter braucht es weiter einen Quantenspeicher.
   - ~~Analysieren~~ – erledigt (ANALYSE.md 5d, C4): Beim Halter braucht es einen
     Quantencomputer für LWE in Überlagerung und einen langen Speicher. Beobachten, nicht
     bauen.
5. **Offline-Bargeld als Träger:**
   - Das Bargeld-Verhalten liefern heute Secure Element und Offline-Bargeld Stufe 1b.
   - Der Quanten-Teil steckt nur in Prägung und Einlösung.
   - ~~Genau abgrenzen, was davon „Quantengeld“ heißen darf~~ – erledigt (ANALYSE.md 5d,
     C5, mit Tabelle): Quantengeld heißen nur W und S. Quantensichere Unterschriften heißen
     „quantensicher“. Das Offline-Bargeld ist kein Quantengeld. Nie „Quantenwährung“.
6. **Gemietete Speicherzeit in der Ionenfalle:**
   - Quantinuum und IonQ halten Qubits Sekunden bis Minuten.
   - Ablauf: Note in der Cloud prägen, halten, über die Kette übertragen, prüfen.
     „Mitnehmen“ heißt hier Verwahrung beim Betreiber, nachprüfbar über die Kette.
   - Dafür braucht es ein Konto.

**Ergebnis:** steht in ANALYSE.md 5c und 5d, mit der Empfehlung, was zuerst gebaut wird.
In Abschnitt C ist nur noch C6 offen (Konto des Betreibers).

## D. Bauen und messen

1. **W1 vervollständigen:**
   - IBM ist gemessen (siehe A).
   - `fern` auf dem echten Chip ist gelaufen (`ibm_fez`, HEUTE.md 3.3): ohne Warten 2,4 %
     Fehler, also Toleranz 6 % und 1.360 Stellen; mit Teleportation 12,5 %, das ist nicht
     sicher prüfbar.
   - Offen ist die Ionenfalle (Sekunden statt Mikrosekunden).
   - Aus den Fehlerraten folgen die Parameter für Abschnitt B.
2. ~~**Prüfer-Dienst für Validatoren**~~ – **gebaut (W2d, 16.11. abends,
   `quanten_w_validator.go`):**
   - verschlüsselter Tresor;
   - Prägebestätigung mit Prüfung der Blätter gegen die Wurzel;
   - Anteile erst nach der Antwort;
   - Hintergrund für Zufall, Offenlegen in Teilen über die Nachbarn und Fristen.
     Nachbarn sind die Sync-Peers und Seeds des Knotens, dazu `QUANTEN_W_PEERS`. Sie
     werden gleichzeitig abgefragt, nur öffentlich und ohne Weiterleitung. Es zählen nur
     Anteile, die der Validator selbst unterschrieben hat.
3. ~~**App:** eine Variante-W-Note und ihren Prüfstand anzeigen~~ – erledigt (App #79,
   17.11.): „Note oder Token nachsehen“ auf der Quantengeld-Seite, für W und S.
4. **Vor jeder Freischaltung (W6):**
   - Gutachten ohne offene kritische Punkte (W5).
   - Parameter aus Abschnitt B.
   - Pilot im Testnetz.

## E. Wartet auf dich (Freigaben und Konten)

- ~~**IBM-Lauf `wiesner = fern` auf dem echten Chip**~~ – gelaufen (HEUTE.md 3.3).
- ~~**App 1.23.2**~~ – vollständig veröffentlicht (APK und IPA, `quelle.json` geprüft).
- **Konto für eine Ionenfalle** (Quantinuum oder IonQ), für C.6 und W1.
- **Anschreiben an den BMFTR-Verbund** verschicken (`ANSCHREIBEN.md`, Vorlage 2). Das ist
  W4 mit Speicher.
- **Gutachten:** Der Prüfauftrag (`docs/QUANTENGELD_PRUEFAUFTRAG.md`) ist auf dem Stand
  vom 16.11. abends (W2c, W2d, Variante S). Einen Gutachter suchen.
- **Außerdem offen:** Stufe-0-Schlüssel, eigene Registrierung (Stufe 1 scharf).
