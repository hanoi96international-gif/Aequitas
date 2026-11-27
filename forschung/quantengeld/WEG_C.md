# Weg C – Quantengeld, das die Physik selbst unkopierbar macht

Stand 26.11.2026. Auftrag des Betreibers: „Quantengeld, das die Physik selbst unkopierbar
macht – das soll endlich stimmen. Erste Priorität: einen Weg finden.“

## Warum es heute nicht stimmt

Die S-Token im Hauptnetz entstehen in **einem** Auftrag auf einem Cloud-Quantenrechner:
Präparieren und Messen laufen im selben Schaltkreis bei IBM. Die Inhaberin bekommt nur
klassische Messwerte x. Sie hält nie einen Quantenzustand, den sie kopieren könnte. Ihr fehlt
nur das Wissen über die Präparation, und das schützt die Schwelle t von v.
Das ist Geheimhaltung, nicht Physik (`HEUTE.md` 3.3b: „Unfälschbar wird es erst, wenn die
Nutzerin selbst misst“).

## Was es wahr macht

**Die Inhaberin misst selbst einzelne Photonen**, die die Präparatorinnen ihr über eine
Quantenleitung schicken (Kent, Lowndes, Pitalúa-García, Rarity, npj QI 8, 28 (2022)).
- Jedes Photon trägt einen BB84-Zustand. Basen und Werte legen die Präparatorinnen aus ihrem
  eigenen Zufall fest und binden sie mit Prüfsummen auf der Kette (wie heute).
- Die Inhaberin misst jedes Photon in einer Basis ihrer Wahl, genau einmal. Ein Photon lässt
  sich nicht kopieren (No-Cloning). Wer es misst, erfährt nur eine Basis.
- Um an **beiden** Annahmestellen zu bestehen, bräuchte sie die Werte in beiden Basen. Das
  verbietet die Physik: Der beste Fälscher irrt bei 14,64 % der Stellen (ideal). Mit den
  Prüfgrößen aus `wiesner_parameter.py` besteht er höchstens mit 2⁻⁴⁰.
- Das gilt **auch dann, wenn sie ihr Gerät selbst baut und manipuliert**: Die Grenze kommt
  aus der Quantenmechanik, nicht aus einem Geheimnis.

Dann stimmt der Satz für die Inhaberin: Sie kann ihren Token physikalisch nicht verdoppeln.
Ehrlich bleibt:
- Wer präpariert, kennt seinen Abschnitt. Gegen Präparatorinnen schützt weiter die Schwelle.
- Nach dem Messen ist der Token wieder Daten. Bargeld von Hand zu Hand bleibt es nicht; das
  braucht Quantenspeicher (Variante W).

## Was schon da ist

| Baustein | Stand |
|---|---|
| Schranke des Fälschers über eine echte Leitung, λ(θ, β), Kents Lemma 2/3, Satz 1 | `wiesner_parameter.py` (`kent_qt1`, Selbsttest rechnet Kents Abb. 5 nach) |
| Prüfgröße je Annahmestelle mit Verkippung θ und freien Mehrphotonen-Stellen | `wiesner_parameter.py` (`s3_stichprobe`), Tabelle in `ANALYSE.md` 5c |
| Meldung der gemessenen Pulse Λ, öffentliche Teilmenge mit Zufall **nach** der Meldung | `s_kette.py` (`s3_meldung`, `s3_teilmenge`), mit Missbrauchstests |
| Befund: Zufall gehört den Präparatorinnen (β = 0), dann ist Kents Handsender nach Satz 1 auf 10⁻⁹ unfälschbar | `ANALYSE.md` 5c |
| Stichprobe nach der Bindung, Pfand der Inhaberin, Nachweis, Los der Präparatorinnen | im Knoten, laufen bzw. ab 27.11.2026 |

## Was fehlt (Plan)

| Schritt | Was | Wer |
|---|---|---|
| **C1** ✅ | Prüfgröße mit θ und Anteil freier Stellen. **Gebaut 26.11.:** `wiesner_kette.py` `K_LEITUNG`/`k_leitung` (Stufen 0/2/5/10°, frei bis 25 %, immer aufgerundet, sonst 0). Der Selbsttest rechnet **jede** Zelle exakt nach, Zeile 0 ist die alte Tabelle (nachgerechnet). Missbrauch: Die Tabelle für ideale Qubits ließe über 5° einen Fälscher weit über 2⁻⁴⁰ durch. Knoten `quanten_s_leitung.go` (nur Rechnung, noch nicht im Konsens), Prüfvektor `quanten_s_leitung_vektor.json` in der CI. Mutationen: 8 in der Spezifikation, 5 im Knoten, alle rot. | ich |
| **C2a** ✅ | **Register der Leitungen** (Entscheidung des Betreibers 27.11.: ⅔ der Prüfer, mindestens 5). Gebaut: `quanten_leitung_register.go` über die vorhandene Freischaltung. Eine Freischaltung `leitung-…` trägt ohne Gruppenordnung θ (Zehntelgrad, 0–100) und frei (ppm, ≤ 25 %) ein. Die Stimmen unterschreiben eine eigene Domäne, eine Stimme für einen Satz gilt also nie für eine Leitung. Vorlauf 30 Tage. Die Einträge stehen in einer eigenen Liste im Register, nie als Satz. Leer bleibt alles byte-gleich. Schläft hinter `quantenLeitungAbUnix`, Wächter `quanten-leitung-nicht-verpasst.sh`. 6 Tests mit Missbrauch, 17 Mutationen rot. | ich |
| **C2b-1** ✅ | **Meldung und Teilmenge im Knoten** (nur Rechnung, wie C1). Gebaut 27.11.: `quanten_s3_meldung.go`. Auf der Kette höchstens 65.536 gemeldete Pulse (das Vierfache von N_MAX), als Lücken kodiert (LEB128, genau eine Form, Hex, höchstens 512 KiB, Größe vor dem Lesen). Teilmenge wie `s_kette.s3_teilmenge` mit Zufall eines Blocks **nach** der Meldung. Spezifikation `s3_kodieren`/`s3_dekodieren` mit Selbsttest; Prüfvektoren `quanten_s3_vektor.json` und `quanten_s3_meldung_vektor.json` in der CI. Missbrauch: zweite Form, Überlauf, Grenzen, Zufall vor oder im Block der Meldung. 17 Mutationen, 16 rot; die übrige (Gleichstand zweier SHA-256) tritt nicht ein. | ich |
| **C2b-2a** ✅ | **Ablauf über eine Leitung, spezifiziert** (27.11.): `s_kette.py` `KetteS3`. (1) **Vormerken** durch den Ausgeber: K, Inhaberin, Leitung, Zahl der Pulse N, Wurzel der Prüfsummen über alle N Pulse, Deckung. Die Deckung liegt ab hier im Topf: Wer vormerkt, zahlt, das begrenzt Müll. Ohne Ausgabe geht sie nach `S3_FRIST` = 1 h zurück, höchstens 64 offene Vormerkungen. (2) **Melden** durch die Inhaberin, **genau einmal** (sonst würfelte sie neu, sobald sie den Zufall kennt), mindestens γ_det·N der Leitung. (3) **Ziehen** mit Zufall aus einer Zeit **nach** der Meldung, genau einmal. Stelle i des Tokens ist Puls teil[i]. (4) **Ausgeben**: n = |teil|, Prüfgröße je Annahmestelle ≥ `k_leitung(tol, t, v, θ, frei)`, 2k ≤ n, Wurzel der Vormerkung, Deckung genau einmal. Danach weiter wie S. Selbsttest mit 29 Prüfungen, davon 20 Angriffe. 15 Mutationen, alle rot. **Folgt daraus für C2a:** Die Leitung braucht auch γ_det im Register. | ich |
| **C2b-2b-1** ✅ | **γ_det im Register der Leitungen** (27.11.). Eine Freischaltung `leitung-…` trägt jetzt auch `gamma` (ppm, 1..10⁶, nie 0). Die Prüfer unterschreiben es mit (`…|theta:|frei:|gamma:|…`), ein nachträglich gesenktes γ passt also zu keiner Stimme. Register, Stand und Laden prüfen es streng. Ab dem Stichtag gehört `gamma` wie θ und frei nur an eine Leitung, an Satz, Anmeldung und Einlösung ist es ein Form-Fehler. Davor wird es überlesen wie von älteren Knoten. Weiter schlafend hinter `quantenLeitungAbUnix`. 9 Mutationen rot. | ich |
| **C2b-2b-2** ✅ | **Vormerken, Melden, Ziehen, Frist im Knoten** (27.11., schlafend hinter `quantenS3AbUnix` und `quantenLeitungAktiv`). `quanten_s3_vormerkung.go`, vier Arten `quanten_w_s3_vormerken`, `…_melden`, `…_ziehen`, `…_frist`. Vormerken nur durch einen Menschen, die Deckung geht in den Pfandtopf, die Werte der Leitung (θ, frei, γ) stehen fest in der Vormerkung. Je Ausgeber eine, höchstens 16 zugleich (= Token im Register). Die Meldung (höchstens 128 KiB Hex, unter der Grenze des Endpunkts) steht nur als SHA-256 im Register, das Ziehen bringt sie wieder. Die Runde ist fest: die erste drand-Runde ab Meldung + 600 s, die Meldezeit höchstens 120 s zurückdatiert. Jeder Knoten prüft die BLS-Signatur und zieht selbst. Nach 1 h zahlt die Frist die Deckung zurück (an die Wallet des Nullifiers, wie P3); erst dann ist der Platz frei. Kein anderes Prägen, keine Ausgabe und keine Bestellung mit einem vorgemerkten K. Spezifikation angepasst (explizite Frist, je Ausgeber eine, Abstand, Länge). **Offen für C2b-2b-3:** Abbruch nach dem Ziehen kostet nichts (die Deckung kommt zurück). Ein Ausgeber mit Inhaberin im Bund kann neu vormerken, bis eine Teilmenge viele freie Pulse trägt (Wahrscheinlichkeit wächst mit der Zahl der Versuche, wie Abbruch und neu Bestellen in Weg B, B2c). Vor dem Stichtag braucht es ein Pfand, das beim Abbruch nach dem Ziehen verfällt, oder eine Prüfgröße, die mit den Versuchen rechnet. | ich |
| **C2b-2b-3a** ✅ | **Pfand gegen Abbruch** (27.11., schlafend). Vormerken legt neben der Deckung ein Pfand in den Pfandtopf (`quantenS3PfandFuer` = wie Weg B: max(1 AEQ, ⌈10 % D⌉), Spezifikation `s_kette.s3_pfand`). Frist **ohne** Meldung: Deckung und Pfand zurück. Frist **nach** der Meldung: Deckung zurück, Pfand ans Grundeinkommen — auch ohne Ziehen, denn mit der Meldung steht die Runde fest, und ihr Zufall wird öffentlich. Das Register liest das Pfand streng (genau `quantenS3PfandFuer(D)`). Rechnung `s3_versuch.py`: Auch mit der Schwankung der freien Pulse in der Teilmenge besteht ein Fälscher je Versuch höchstens mit **2⁻³⁹·⁶** (θ 0/2/5/10°, tol 2/4/6, frei bis 25 %, n = 2k und N_MAX); Wiederholen kostet so im Mittel über 2³⁹ AEQ Pfand. 13 Mutationen im Knoten rot. | ich |
| **C2b-2b-3b** ✅ | **Ausgabe über die gezogene Teilmenge** (27.11., schlafend, `quanten_s3_ausgabe.go`). `quanten_w_s_ausgeben` mit vorgemerktem K: erst nach dem Ziehen, nicht nach der Frist, nur Ausgeber, Inhaberin, Wurzel und Deckung der Vormerkung, n = Zahl der gezogenen Pulse. Prüfgröße k = `quantenSKLeitung(tol, t, v, θ, frei)` mit den festgehaltenen Werten; k = 0 oder 2k > n abgewiesen. Der Token merkt sich `pulse`, `n_puls`, `k_min` (fehlen bei bisherigen Token: StateRoot gleich). Stelle i ist Puls teil[i]: Einlösen und Nachweis (P4) prüfen Prüfsumme und Pfad zum Blatt teil[i] im Baum über **alle** Pulse (bis 26 Glieder; bisherige Token weiter 14). Jede Stichprobe beim Anmelden ≥ k_min. Deckung aus dem Pfandtopf genau einmal in den W-Topf, Pfand zurück. Spezifikation `ausgeben_s3` ohne erklärtes k (es wird erst beim Anmelden sichtbar), Selbsttest mit echter Prägung über 6.000 Pulse bis zum Einlösen. Knoten: voller Ablauf mit 20.000 Pulsen (15 Pfadglieder). | ich |
| **C2b-2b-3c** | Werkzeuge und App: Vormerken/Melden/Ziehen/Ausgeben bauen, beim Offenlegen kleinere Teile wählen (lange Pfade, `quantenWKoerperMax`), Annahmestelle prüft vor dem Annehmen gegen `pulse`/`k_min` der Ansicht. Dann **Stichtag** `quantenS3AbUnix` mit Deploy-Wächter (`.github/neustart/…-nicht-verpasst.sh`, Statusfeld `quanten_w.s3_ab`, Test-Skript in `ci.yml`). Voraussetzung: eine eingetragene Leitung (C2a, zwei Drittel der Prüfer) und Partner/Gerät (C4). | ich / Betreiber |
| **C2b-3** | Pulse statt Qubits: Die Präparatorinnen binden ihre Pulse mit Prüfsummen, der Ablauf S läuft auf der Teilmenge. | ich |
| **C3** ✅ | Format Empfänger ↔ App. **Gebaut 27.11.:** App `lib/quantenSEmpfaenger.ts`. Format `aequitas-empfaenger-v1` mit {format, k, n_puls, messungen: [[puls, basis, bit]]}, streng gelesen (Größe vor dem Parsen, genau diese Felder: kein Gerät bescheinigt sich selbst; K muss passen; Pulse aufsteigend und verschieden; Grenzen 2²⁶ Pulse, 10⁶ Messungen). Meldung Λ und Teilmenge rechnen gleich wie `s_kette.py` (Prüfvektor `--vektor-s3`, in der CI gegen die Spezifikation). Ein Nachbau des Empfängers dient nur Tests. 28 Tests, 7 Mutationen rot. Jedes Gerät, das dieses Format liefert, passt. | ich |
| **C4** | Gerät und Partner: Sender bei den Präparatorinnen, Empfänger bei der Inhaberin. Kandidaten sind die Gruppen der Arbeit (Bristol: Handsender; Cambridge: Theorie); Vorlage `ANSCHREIBEN.md`. | Betreiber |
| **C5** | Messung am Gerät: Fehler E, Verkippung θ, Mehrphotonen-Anteil, Seitenkanäle (Kents Annahmen A und G). Erst mit diesen Werten gibt es eine Prüfgröße. | Partner, ich |
| **C6** | Erster Token über eine echte Leitung im Hauptnetz, öffentlicher Nachweis, Gutachten. Dann darf die Website ohne „entwickelt“ sagen: unkopierbar durch Physik. | alle |

## Hardware, ehrlich

- **Handys reichen nicht.** Ein Empfänger braucht Einzelphotonen-Detektoren in zwei Basen. Es
  gibt sie im Labor und als Prototyp-Handgeräte, nicht im Laden.
- **Erster Ort:** eine Annahme-Kasse oder ein Labor mit Empfänger, an dem Inhaberinnen ihren
  Token abholen. Das ist kein Alltag, aber der erste echte Beweis: Ein Token, den die Physik
  vor dem Verdoppeln schützt, auf einer öffentlichen Kette.
- **Laserpulse statt Einzelphotonen** gehen, wenn die mittlere Photonenzahl klein ist. Die
  Mehrphotonen-Pulse gelten dann als frei für den Fälscher, und die Stichprobe wird größer
  (`ANALYSE.md` 5c, Tabelle).

## Warum das die Revolution wäre

Physikalisch unfälschbare Token gibt es bisher nur im Labor, mit einer Bank, die alles weiß
(Kent u. a., Bozzio u. a.). Weg C verbindet drei Dinge zum ersten Mal:
- eine **öffentliche Kette**, die jeden Token nur einmal einlöst;
- **ausgeloste Präparatorinnen** statt einer Bank;
- den **Schutz durch die Physik** bei der Inhaberin.
