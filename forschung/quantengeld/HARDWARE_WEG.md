# Quantengeld – Weg auf der Hardware

Stand 15.11.2026. Was die Software braucht, ist fertig (Register, Anmeldung, Abgabe im
Topf, Einlösen, Freischaltung). Was fehlt, ist Hardware für drei Dinge: **halten**,
**groß genug prüfen**, **weitergeben**. Hier: was es heute gibt, und die nächsten Schritte.

## Heute gemessen (IBM, supraleitend)

- Prägen, Bezahlen (Teleportation) und Prüfen laufen auf `ibm_marrakesh` im Spielzeugmaß
  (Z₈): echte Note 0,81, Fälschung 0,22, Abstand +0,59; Bezahlen 0,49.
- **Lebensdauer einer Note ~20 µs.** Das ist die Grenze supraleitender Qubits (T₂ im
  Bereich 100 µs, mit Gattern weniger).

## Was es schon gibt (Recherche 15.11.2026)

| Weg | Stand | Was es für Aequitas heißt |
|---|---|---|
| **Ionenfallen** (Quantinuum Helios, 98 Qubits) | T₂ im Bereich von **Sekunden**, Messung mitten im Schaltkreis mit bedingter Verzweigung, Cloud-Zugang (Datenblatt Juni 2026); logischer Speicher 99,9995 % (Okt. 2026) | **Nächster Schritt:** dasselbe Labor auf Ionenfallen → Lebensdauer einer Note von 20 µs auf Sekunden (×10⁵). |
| **Seltenerd-Kristalle** (Eu:YSO, Kernspins) | Kohärenz **6 h** (Spin, 2015), über 10 h in Festkörper-Kernspins (2025); Photonenspeicher mit 90 % Effizienz (2026) | Weg für **lange Lagerung** – heute Labor, nicht mietbar. |
| **Kommerzieller Quantenspeicher** (Qunnect Qu-MEM, Raumtemperatur) | erhältlich; Speicherzeit nicht veröffentlicht | für **Netzknoten**, kurze Speicherzeiten. |
| **Quantennetz mit Speichern** | Verschränkung zweier Speicher über **420 km** Glasfaser (USTC, Aug. 2026) | Grundlage für **Weitergabe** über Entfernung – Labor. |
| **Quanten-Token ohne Speicher** (S-Tokens, Kent et al.) | vollständig experimentell gezeigt, fehler- und verlusttolerant (npj QI 2022); Folgearbeiten zur Transaktionszeit | heute machbar, aber **ausgebergebunden** und nicht von Hand zu Hand – kein Ersatz für Lightning, eher ein Baustein für schnelle Zahlungen. |
| **Öffentliches Quantengeld, Theorie** | Konstruktionen aus iO + LWE (2022, 2026 mit Anonymität); Einmal-Unterschriften im Standardmodell (2025) | die **Verfahren** existieren; praktikabel sind sie noch nicht. |

## Schritte

1. **Ionenfalle – gebaut (15.11.2026), wartet auf den Zugang:** `hardware_quantinuum.py`
   übersetzt dieselben Schaltkreise nach pytket und schickt sie über Quantinuum Nexus.
   Workflow „Quantengeld auf Quanten-Hardware“, Modus
   - `quantinuum-pruefen`: Syntaxprüfung auf H2-1SC (kostenlos) – zuerst;
   - `quantinuum-emulator`: H2-1E, mit dem Rauschmodell der Maschine;
   - `quantinuum`: die Maschine (Variable `QUANTINUUM_DEVICE`, Standard H2-1).

   `warten_reihe` gilt hier in **Millisekunden** (z. B. `0,10,100,1000`); das Warten ist
   Quantinuums `sleep(t)` in Sekunden. Der Läufer prüft nach dem Übersetzen, dass jedes
   `sleep` noch im Schaltkreis steht – sonst wird nicht ausgeführt (keine Kosten, keine
   falschen Zahlen). Kostendeckel je Schaltkreis: `QUANTINUUM_HQC_JE_KREIS` (Standard 50),
   höchstens 16 Schaltkreise je Fall und 2.000 Schüsse.

   **Einmal einzurichten (Betreiber):** Konto bei Quantinuum Nexus; dort einmal anmelden
   und den Refresh-Token als Secret `QUANTINUUM_TOKEN` in der Umgebung `quantenhardware`
   (GitHub → Settings → Environments) hinterlegen. Der Token wird nie ausgegeben und nie
   auf die Platte geschrieben. Erst `quantinuum-pruefen`, dann der Emulator, dann die
   Maschine.

   **Offen bis zum ersten Lauf:** die Einheit von `sleep` (Sekunden laut `hqslib1_dev`)
   in Quantinuums Doku bestätigen – der Emulator zeigt es auch: fällt die Annahme schon
   bei 10 ms, stimmt die Einheit nicht.
2. **Größerer Satz auf Ionen:** größere Gruppe als Z₈ (Z₁₆, Z₃₂) mit mehr Qubits und
   tieferen Schaltkreisen; der Abstand echt/Fälschung muss halten.
3. **Lagerung:** Kontakt zu Gruppen mit Seltenerd-Speichern (Stunden) für einen
   gemeinsamen Versuch: Note prägen → in den Speicher → nach Minuten prüfen.
4. **Weitergabe:** Teleportation zwischen zwei Geräten über ein Netz mit Speichern
   (Partner mit Testbett).
5. **Bezeugen:** die Prüfer stimmen für einen Satz erst, wenn 1–4 für ihn gezeigt und
   begutachtet sind (`docs/QUANTENGELD_PRUEFAUFTRAG.md`).

## Quellen

- Kent et al., *Practical quantum tokens without quantum memories and experimental tests*, npj Quantum Information (2022): https://www.nature.com/articles/s41534-022-00524-4
- *Practical quantum tokens: challenges and perspectives* (2026): https://arxiv.org/html/2602.10621
- *Long-lived quantum memories with rare-earth ions doped in crystals* (2026): https://link.springer.com/article/10.1007/s44214-026-00110-1
- *90%-Efficient Optical Storage in a Rare-Earth Crystal* (2026): https://arxiv.org/pdf/2609.19807
- Verschränkung von Quantenspeichern über 420 km (2026): https://phys.org/news/2026-08-physicists-entangle-quantum-memories-km.html
- Quantinuum Helios, Datenblatt (Juni 2026): https://docs.quantinuum.com/systems/_static/assets/data_sheets/Quantinuum%20Helios%20Product%20Data%20Sheet.pdf
- Quantinuum, logische Speichertreue 99,9995 % (2026): https://www.quantinuum.com/blog/quantinuum-closes-in-on-one-million-error-free-operations
- Qunnect Qu-MEM: https://thequantuminsider.com/2026/10/15/stony-brook-patented-quantum-memory-time-best-inventions/
- Çakan, Goyal, Yamakawa, *Anonymous Public-Key Quantum Money* (CRYPTO 2026): https://link.springer.com/chapter/10.1007/978-3-032-35409-9_6
- *Demonstration of quantum-digital payments* (2023): https://arxiv.org/pdf/2305.14504
