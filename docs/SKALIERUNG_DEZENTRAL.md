# Skalierung ohne zentrale Instanz — Gesamtkonzept

*Stand 25.09.2026. Baut auf [`SCALING_ARCHITECTURE.md`](../SCALING_ARCHITECTURE.md) auf (Messungen und Umsetzungsstand Juli 2026) und ersetzt es nicht.*

## Ziel und Bedingungen

Mehr Durchsatz, ohne dass irgendeine Rolle nur Starken oder Einzelnen vorbehalten ist.

1. **Keine zentrale Instanz.** Kein fester Primary, kein bevorzugter Server, kein Betreiber, dem man vertrauen muss.
2. **1 Mensch = 1 Validator.** Zugang allein über die Registrierung, nie über Geld oder Rechenleistung.
3. **Günstige Hardware reicht.** Ein Server im Bereich 30–50 € im Monat muss als Validator genügen.
4. **Jede Stufe bekommt eine Aktivierungsregel** (Blockhöhe oder Bedingung auf der Kette, wie `KNIGHTDAG_ACTIVATION_HEIGHT`). Gebaut wird vollständig, scharf geschaltet wird erst, wenn Tests und Messungen grün sind.

## Ausgangslage (gemessen)

| | Wert | Quelle |
|---|---|---|
| Durchsatz live, C2 allein | 4.288 TPS in Blöcken | `LAUNCH_CHECKLISTE.md`, 24.09. |
| CPU je Überweisung | ~486 µs | `OPEN_TASKS.md` |
| Auslastung unter Last | ~1 von 6 Kernen | `OPEN_TASKS.md` — es wird gewartet, nicht gerechnet |
| Signaturprüfung (libsecp256k1) | 101 µs je Prüfung, 41.000/s auf 6 Kernen | `SCALING_ARCHITECTURE.md` |
| Höchstens je Block | 20.000 Überweisungen | `maxTxsPerBlock` |
| Annahme | **nur ein Knoten** nimmt an | `annahme_tor.go` |
| Paralleles Nachspielen | **ab 1.10.2026 aus** — der Pfad führt keine Unternehmens-Buchführung | `block.go`, `wirtschaftAktiv` |

Zwei Dinge daraus sind entscheidend:
- **Die heutige Architektur hat selbst eine zentrale Stelle**: den einen annehmenden Knoten. Stufe 2 entfernt sie.
- **Mit den Wirtschaftsregeln wird die Kette langsamer**, weil das Nachspielen wieder seriell läuft. Stufe 1.1 behebt das.

## Überblick

| Stufe | Was | Dezentral | Wirkung | Aktivierung |
|---|---|---|---|---|
| **1** | Jeder Knoten effizienter | unverändert | mehr TPS pro Knoten, für alle gleich | Schalter + Determinismus-Tests |
| **2** | Alle nehmen an, jeder für seine Konten | **zentraler Annehmer fällt weg** | Ausfallsicherheit, keine Einzelstelle | Blockhöhe |
| **3** | Bereiche mit ausgelosten Ausschüssen | ja | TPS wächst mit der Zahl der Validatoren | erst ab genug unabhängigen Validatoren |
| **4** | Gültigkeitsbeweise (ZK) je Block | ja | Prüfen fast kostenlos, auch mobil | Forschung |

---

## Stufe 1.0 — Jede Überweisung ist von jedem Validator nachprüfbar (Voraussetzung)

**Befund (25.09.2026):** Überweisungen entstehen nur aus vom Absender signierten EVM-Transaktionen (`eth_sendRawTransaction`). Der annehmende Knoten prüft die Signatur, übernimmt aber nur `Wallet`, `To`, `Amount` und `TxHash` in den Block (`evm_rpc.go`, `pendingTxTemplate`). Beim Nachspielen prüfen die anderen Validatoren nur die **Signatur des Blocks** (`block.go`, AddPeerBlock), nicht die des Absenders. Die Nonce lebt nur im RPC-Server des annehmenden Knotens, nicht im gemeinsamen Zustand.

**Folge:** Wer den annehmenden Knoten kontrolliert, kann Überweisungen von fremden Konten in Blöcke schreiben, ohne dass ein anderer Validator es erkennen kann. Das ist eine zentrale Instanz. Stufe 2 würde das auf jeden Validator ausweiten. Deshalb kommt Stufe 1.0 vor allem anderen.

**Lösung:**
- **Rohform im Block:** Jede Überweisung trägt die signierte Rohtransaktion (`Transaction.Roh`, Hex).
- **Jeder Validator prüft beim Nachspielen**, und zwar parallel vorab für den ganzen Block (das ist zugleich Stufe 1.2):
  - Absender aus der Signatur = `Wallet`
  - Empfänger und Betrag passen zur Rohform
  - Chain-ID 1926
  - `keccak(Roh)` = `TxHash`
- **Schutz gegen Wiederholung im gemeinsamen Zustand:** Jedes Konto führt `NaechsteNonce`. Eine Überweisung braucht `nonce ≥ NaechsteNonce`, danach gilt `NaechsteNonce = nonce + 1`. Wallets, die schon Nonces über 0 haben, funktionieren unverändert weiter. Der Wert geht nur in den StateRoot ein, wenn er nicht 0 ist. Vor der Aktivierung ist er überall 0, alte Blöcke ergeben also denselben StateRoot.
- **Annahme nach denselben Regeln:** Der annehmende Knoten lehnt ab, was die anderen ablehnen würden. So erzeugt er nie einen Block, den sie verwerfen.
- **Ungültige Signatur oder Nonce im Block** heißt: Der ganze Block wird abgelehnt, denn der Erzeuger hat sich falsch verhalten.
- **Snapshot:** `NaechsteNonce` reist mit, sonst ließe sich nach einem Resync eine alte Überweisung erneut einreichen.

**Aktivierung:** per Blockzeit (`SIGNIERTE_UEBERWEISUNGEN_AB`). Erst wenn alle Knoten die neue Version haben, denn ältere Knoten würden Blöcke mit `Roh` am Blockhash ablehnen.

**Danach dieselbe Regel für alle anderen geldbewegenden Transaktionen** (Tausch, Liquidität, Wächter und Treuhand): Jede trägt den Nachweis ihres Auftraggebers im Block.

---

## Stufe 1 — Jeder Knoten effizienter

Ziel: Ein günstiger Server schafft ein Vielfaches von heute. Das Vertrauensmodell ändert sich nicht, jeder Validator rechnet weiterhin alles nach.

### 1.1 Buchführung parallelfähig (zuerst)

Heute führt nur der serielle Pfad `nachUeberweisung` (Umsatz, Freibeträge) aus. Deshalb ist das parallele Nachspielen ab dem 1.10. aus.

- Die Buchführung wird in zwei Teile zerlegt: **je Überweisung** ein reiner Beitrag (wer, an wen, Betrag, Art), berechnet ohne gemeinsamen Zustand, und **je Block** eine deterministische Zusammenführung in fester Reihenfolge (nach Transaktionsindex).
- Damit darf der parallele Pfad auch nach der Aktivierung laufen. Das Ergebnis muss bitgleich dem seriellen sein.
- **Test:** Fuzz über gemischte Blöcke. Serieller Pfad, paralleler Pfad und Annahmepfad ergeben denselben StateRoot und dieselbe Buchführung.

### 1.2 Signatur nur einmal prüfen, auf allen Kernen

- Beim Nachspielen werden alle Signaturen eines Blocks **vorab parallel** geprüft (Worker-Pool, reine Funktion), bevor irgendein Zustand angefasst wird. Für den RPC-Pfad ist das schon umgesetzt (`evm_rpc.go`).
- Die ermittelte Absenderadresse wird je Transaktion zwischengespeichert. Annahme, Nachspielen und Resync zahlen die 101 µs nicht mehrfach.

### 1.3 Schreibmenge sammeln, einmal je Block schreiben

- Phase 1 lädt alle betroffenen Konten, Phase 2 rechnet rein im Speicher, Phase 3 schreibt **eine** gebündelte Anweisung je Block. Das Muster steht schon in `replay_parallel.go` und wird auf den ganzen Block ausgedehnt, nicht nur auf disjunkte Überweisungsfolgen.
- Überweisungen mit Konflikt (gleiches Konto) laufen nach dem Block-STM-Prinzip: optimistisch parallel, bei Konflikt nur die betroffene Transaktion erneut.

**Umgesetzt (26.09.2026), erster Teil: Sammelempfänger.** Viele Zahlungen an denselben Empfänger (ein Geschäft, ein Lohnempfänger) beenden den parallelen Lauf nicht mehr (`collectDisjointTransferBatch`). Regeln:
- Absender sind im Lauf eindeutig.
- Ein Empfänger darf im Lauf nie senden.
- Gutschriften werden je Empfänger in ganzen Mikro-AEQ summiert. Das ist exakt und von der Reihenfolge unabhängig.
- Wohlstandsgrenze und Buchführung laufen in Blockreihenfolge, mit dem laufenden Stand des Empfängers.

Tests: `replay_parallel_sammelempfaenger_test.go`. Er prüft gegen den seriellen Pfad Kontostände, Demurrage-Uhren, StateRoot und Buchkonten. Die Grenze reißt dabei erst mit der dritten Gutschrift. Die Gegenprobe ohne laufenden Stand schlägt fehl. Kein neuer Schalter: Der Pfad bleibt eine reine Beschleunigung, und das Ergebnis ist bitgleich mit dem seriellen. Offen bleibt der Fall, dass ein Konto im selben Block empfängt und danach sendet. Dafür kommt Block-STM.

### 1.4 Platte

- Liegt eine zweite Platte vor, kommt das WAL dorthin (`AEQUITAS_WAL_PATH`). Das ist der von `WAS_DU_NOCH_TUN_MUSST.md` benannte nächste echte Schritt.
- Mit nur einer Platte: die fsync-Latenz des neuen Servers messen, bevor irgendetwas geschätzt wird.

### 1.5 Blocktakt

- Erst nach 1.1–1.3: mehrere Blöcke je Takt oder ein kürzerer Takt (`ENABLE_MULTI_BLOCK_TICK`), damit die Grenze von 20.000 Überweisungen je Block nicht deckelt.

**Aktivierung:** je Punkt ein Schalter. Ein Punkt wird eingeschaltet, wenn der Determinismus-Fuzz grün ist und die Messung auf beiden Validatoren den Gewinn zeigt.

**Erwartung** (vorsichtig): pro Knoten ~10.000–20.000 TPS auf einem Server mit 8 dedizierten Kernen. Die Signaturprüfung setzt die harte Obergrenze: 101 µs × TPS ÷ Kerne.

---

## Stufe 2 — Alle nehmen an, ohne Konflikt

Ziel: Der eine annehmende Knoten entfällt. Jeder Validator nimmt an, und niemand ist Einzelstelle.

### Das Problem, das `annahme_tor.go` löst

Belasten zwei Knoten **dasselbe Konto** gleichzeitig, prüfen sie gegen verschiedene Sichten und weichen danach dauerhaft voneinander ab. Gutschriften sind unkritisch, denn Addition kennt keine Reihenfolge.

### Die Lösung: zuständiger Knoten je Konto

- `zustaendig(konto, epoche) = validatoren[H(konto ‖ epoche) mod n]`. Die Liste `validatoren` ist die auf der Kette geführte, sortierte Menge aktiver Menschen-Validatoren. Jeder kann das Ergebnis selbst berechnen, es braucht niemanden, der entscheidet.
- **Belastungen** eines Kontos nimmt nur der zuständige Knoten an. Andere leiten die Transaktion an ihn weiter.
- **Gutschriften** darf jeder aufnehmen.
- Eine **Epoche** umfasst eine feste Zahl von Blöcken. Mit jeder Epoche wechselt die Zuständigkeit, so hat niemand dauerhaft Macht über bestimmte Konten.

### Übergabe an der Epochengrenze

- Der alte Zuständige nimmt bis Höhe *h* an. Der neue beginnt erst, wenn er alle Blöcke bis *h* angewandt hat, plus eine kurze Ruhezeit Δ.
- Transaktionen dazwischen bleiben liegen, sie gehen nicht verloren.

### Ausfall eines Zuständigen

- Liefert der zuständige Knoten *k* Blöcke lang nichts, geht die Zuständigkeit für den Rest der Epoche an den Nächsten im Ring über. Diese Regel ist ebenfalls aus der Kette berechenbar.

### Offene Punkte, die Stufe 2 klären muss

- **Liegegeld und Vermögensgrenze bei Gutschriften.** Heute kann auch eine Gutschrift Liegegeld abrechnen und die Pools berühren. Diese Effekte müssen je Block deterministisch zusammengeführt werden (wie 1.1), statt beim Annehmen.
- **Wer ist Validator?** Registrierter Mensch plus die Mindestleistung aus einem **Leistungstest**: Das Netz stellt eine Aufgabe (aufgezeichnete Blöcke mit bekanntem StateRoot), die anderen messen die Zeit, und es wird zu zufälligen Zeitpunkten wiederholt. Der Test entscheidet nur über *ja oder nein*, **nicht über Rang oder Macht**.

**Aktivierung:** Blockhöhe, frühestens wenn mindestens 3 unabhängige Validatoren laufen.

---

## Stufe 3 — Bereiche mit ausgelosten Ausschüssen

Ziel: Der Durchsatz wächst mit der Zahl der Menschen, die Knoten betreiben. Kein Knoten muss alles rechnen.

### Aufbau

- Die Konten werden in **S Bereiche** geteilt (nach Adresse).
- Für jeden Bereich wird je Epoche ein **Ausschuss** aus zufällig gelosten Validatoren gebildet. Die Zufallszahl stammt aus einem Blockhash, später aus einer VRF.
- Der Ausschuss rechnet seinen Bereich **vollständig** und unterschreibt das Ergebnis mit ≥ 2/3 Mehrheit.
- Alle anderen **übernehmen** das Ergebnis dieses Bereichs mit billigen Sofortprüfungen:
  - Geldmenge bleibt erhalten
  - kein Konto unter null
  - StateRoot passt
  - Mehrheit des Ausschusses hat unterschrieben

### Warum das bei Aequitas besonders gut passt

Ausgeloste Ausschüsse sind nur sicher, wenn niemand viele Identitäten hat. Andere Netze lösen das über Geldeinsatz, dann bestimmt Reichtum. Aequitas hat **1 Mensch = 1 Validator**. Die Ausschüsse bestehen aus einzelnen, echten Menschen.

### Überweisungen zwischen Bereichen

- Die Belastung erfolgt in Bereich A, dazu entsteht eine Quittung.
- Die Gutschrift folgt in Bereich B im nächsten Block.
- Die Geldmenge bleibt über beide Schritte erhalten, das ist prüfbar.

### Gemeinsame Größen (die eigentliche Schwierigkeit)

Pools, Grundeinkommen, Liegegeld-Summe, Vermögensgrenze (Durchschnitt) und Gini sind heute **global**.

- Jeder Bereich führt **Teilsummen**.
- Ein **Sammelblock** je Epoche addiert sie deterministisch. Daraus ergeben sich der Anteil je Mensch und die Grenzen.
- Das **Grundeinkommen** zahlt jeder Bereich an seine Menschen aus, mit dem im Sammelblock festgelegten Betrag. Das löst nebenbei das O(N)-Problem der Tagesverteilung (`UBI_DISTRIBUTION_DESIGN.md`).

### Schutz gegen einen gekaperten Ausschuss

- **Umtausch und Ausstieg** aus dem System werden erst ausgeführt, wenn der Block zusätzlich von einem **zweiten, unabhängig gelosten Prüfausschuss** bestätigt ist. Ein Betrug lässt sich so nicht zu Geld machen, bevor er auffällt.
- **Fehlerbeweis:** Wer einen falschen Bereichsblock findet, veröffentlicht die Transaktion samt Vorzustand. Jeder kann das billig nachrechnen. Die Folgen:
  1. Stopp beim letzten bestätigten Stand.
  2. Der Ausschuss wird ausgeschlossen.
  3. Die Zeit danach wird neu gerechnet.

**Aktivierung:** nur wenn `validatoren ≥ S × m` gilt, mit einer Mindestgröße *m* je Ausschuss (Vorschlag 16) und **nicht unter 32 Validatoren**. Vorher bleibt Stufe 3 aus. Sie wird mit Simulationen vieler Knoten getestet, nicht live.

---

## Stufe 4 — Gültigkeitsbeweise (Forschung)

- Je Block ein ZK-Beweis, dass der Zustandsübergang korrekt ist.
- Prüfen kostet dann Millisekunden, auch auf einem Handy. Übernehmen ohne Nachrechnen wird **ohne Vertrauen** möglich.
- Offen: Kosten des Beweisens, passende Schaltkreise für die Wirtschaftsregeln. Wird nicht vor Stufe 3 begonnen.

---

## Außerhalb der Kette ebenfalls dezentral

Die Kette allein genügt nicht. Heute betreibt ein Team Coordinator, Vergleichsdienste und Proof-Server.
- **Coordinator:** Jeder kann einen betreiben (`EIGENER_COORDINATOR.md`).
- **Vergleichsdienste:** Der Code muss öffentlich werden, und unabhängige Betreiber müssen dazukommen. Jede Registrierung wird von Diensten **verschiedener Betreiber** bezeugt (Bescheinigungs-Quorum).
- **Proof-Server:** Er kann bei jedem Validator mitlaufen.

---

## Offene Entscheidungen

| # | Frage | Vorschlag |
|---|---|---|
| E1 | Wie lange darf Umtausch/Ausstieg in Stufe 3 warten? | Sekunden bis wenige Minuten, nie Stunden |
| E2 | Mindestgröße eines Ausschusses | 16 Menschen |
| E3 | Länge einer Epoche (Stufe 2/3) | 600 Blöcke (~10 Minuten) |
| E4 | Mindestleistung für Validatoren | aus dem Leistungstest, so gewählt, dass ein Server für ~40 € sie erreicht |
| E5 | Was passiert mit Zahlungen nach einem Fehlerbeweis? | neu rechnen ab dem letzten bestätigten Stand; Betroffene werden angezeigt |

## Reihenfolge

1. **1.1** Buchführung parallelfähig, weil sonst ab 1.10. alles langsamer wird
2. **1.2, 1.3** Signaturen und Schreibpfad
3. Messung auf beiden Validatoren
4. **1.4, 1.5** je nach Messung
5. **Stufe 2** (Aktivierung, sobald ≥ 3 unabhängige Validatoren laufen)
6. **Stufe 3** bauen und simulieren, Aktivierung per Schwelle
7. **Stufe 4** als Forschung

Nichts davon wird vor dem 1.10.2026 auf den Produktionsknoten eingeschaltet, ohne ausdrückliche Freigabe.
