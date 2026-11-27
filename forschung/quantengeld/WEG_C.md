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
| **C2** | Knoten, schlafend: neue Token-Art „Leitung“. Meldung Λ, Teilmenge mit Zufall nach der Meldung, Größen fail-closed. Eigene Sicherheitsprüfung. | ich |
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
