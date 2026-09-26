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

**Umgesetzt (26.09.2026, `auftrag_nachweis.go`).** Tausch, Liquidität (hinzufügen und abziehen), Faucet, Rückholung aus der Treuhand und die drei Unternehmens-Aufträge tragen ab derselben Aktivierung ihren `Nachweis`. Er enthält die Unterschrift(en) und die unterschriebenen Werte. Jeder Validator baut die Nachricht in denselben Formaten nach wie die Annahme und prüft dann:
- die Unterschrift, bei Unternehmen beide Unterschriften,
- die Beträge,
- das Zeitfenster zur Blockzeit (höchstens eine Stunde alt, höchstens fünf Minuten voraus),
- bei Tausch und Liquidität die neue `NaechsteAuftragsNonce` im gemeinsamen Zustand.

Bei Mitinhaber und Schließen muss der Unterzeichner zusätzlich im Augenblick der Ausführung verantwortlich sein. Ein Verstoß macht den ganzen Block ungültig. Nicht dabei sind Aufträge, die das System selbst aus Regeln erzeugt (Verteilungsrunden, Treuhand nach Inaktivität, Zuschuss-Staffel). Die rechnet jeder Validator nach.

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

**Umgesetzt (26.09.2026).** `absender_cache.go` ordnet dem Hash der Rohtransaktion den Absender zu. Der Hash wird aus den Bytes berechnet, nie aus dem TxHash-Feld. Der Cache ist fest begrenzt und merkt sich nur Erfolge. Die Nonce bei der Annahme wird nur gelesen, nicht erneut wiederhergestellt. `personal_sign`-Unterschriften der Aufträge werden ebenso nur einmal geprüft.

### 1.3 Schreibmenge sammeln, einmal je Block schreiben

- Phase 1 lädt alle betroffenen Konten, Phase 2 rechnet rein im Speicher, Phase 3 schreibt **eine** gebündelte Anweisung je Block. Das Muster steht schon in `replay_parallel.go` und wird auf den ganzen Block ausgedehnt, nicht nur auf disjunkte Überweisungsfolgen.
- Überweisungen mit Konflikt (gleiches Konto) laufen nach dem Block-STM-Prinzip: optimistisch parallel, bei Konflikt nur die betroffene Transaktion erneut.

**Umgesetzt (26.09.2026), erster Teil: Sammelempfänger.** Viele Zahlungen an denselben Empfänger (ein Geschäft, ein Lohnempfänger) beenden den parallelen Lauf nicht mehr (`collectDisjointTransferBatch`). Regeln:
- Absender sind im Lauf eindeutig.
- Ein Empfänger darf im Lauf nie senden.
- Gutschriften werden je Empfänger in ganzen Mikro-AEQ summiert. Das ist exakt und von der Reihenfolge unabhängig.
- Wohlstandsgrenze und Buchführung laufen in Blockreihenfolge, mit dem laufenden Stand des Empfängers.

Tests: `replay_parallel_sammelempfaenger_test.go`. Er prüft gegen den seriellen Pfad Kontostände, Demurrage-Uhren, StateRoot und Buchkonten. Die Grenze reißt dabei erst mit der dritten Gutschrift. Die Gegenprobe ohne laufenden Stand schlägt fehl. Kein neuer Schalter: Der Pfad bleibt eine reine Beschleunigung, und das Ergebnis ist bitgleich mit dem seriellen.

**Umgesetzt (26.09.2026), zweiter Teil: beliebige Wiederholungen.** Jede Adresse darf im Lauf senden und empfangen, in jeder Reihenfolge. Block-STM wird dafür nicht gebraucht. Phase 1b rechnet den Lauf seriell im Speicher vor: Deckung und Grenze gegen laufende Stände. Danach wird je Konto die Netto-Summe parallel angewandt. Die Ausführung ist eine Ganzzahl-Addition. Teuer sind nur Signaturen und Datenbank, und die sind ohnehin herausgezogen. Block-STM lohnt sich erst, wenn die Ausführung selbst teuer wird.

Der Zufallstest `TestParallelesNachspielen_WiederholteAdressenWieSeriell` prüft Blöcke mit 120 Überweisungen in einem Kreis von 24 Konten, auch mit knappen Konten, gegen den seriellen Pfad.

**Nebenbefund (behoben):** Alle Schnellpfade (Annahme und paralleles Nachspielen) prüften die Wohlstandsgrenze ohne LP-Anteile. Der serielle Pfad kappte mit ihnen. Ein Empfänger mit Pool-Anteilen führte deshalb zu verschiedenen StateRoots. `wuerdeKappenLocked` ist jetzt die eine Vorprüfung für alle.

### 1.4 Platte

- Liegt eine zweite Platte vor, kommt das WAL dorthin (`AEQUITAS_WAL_PATH`). Das ist der von `WAS_DU_NOCH_TUN_MUSST.md` benannte nächste echte Schritt.
- Mit nur einer Platte: die fsync-Latenz des neuen Servers messen, bevor irgendetwas geschätzt wird.

### 1.5 Blocktakt

- Erst nach 1.1–1.3: mehrere Blöcke je Takt oder ein kürzerer Takt (`ENABLE_MULTI_BLOCK_TICK`), damit die Grenze von 20.000 Überweisungen je Block nicht deckelt.

**Umgesetzt (26.09.2026), 1.4:** `tools/fsync-messung` misst eine Platte in genau der Form, in der der WAL schreibt: vorbelegte Datei, kleine Sätze, fdatasync nach jedem. Es gibt p50, p90, p99 und das Maximum aus, dazu die WAL-Obergrenze je Bündelgröße. Bei mehreren `-dir` nennt es die Platte mit dem niedrigsten p99. Im laufenden Betrieb zeigt `/health` die Verteilung der WAL-Syncs bereits an (`sync_verteilung.go`).

**1.5:** Der Schalter `ENABLE_MULTI_BLOCK_TICK` besteht (`block_cadence.go`, bis zu 4 zusätzliche Blöcke je Takt, nur bei vollem Block). Es fehlt nur die Messung.

**Messplan vor dem Einschalten (auf beiden Validatoren, von einem Menschen):**
1. `go run ./tools/fsync-messung -dir <Datenverzeichnis> [-dir <zweite Platte>]`. Liegt das p99 über 20 ms, erst die Platte klären (1.4).
2. Lasttest `tools/contabo-loadtest` (fund, warmup, run) ohne Schalter. Aus `/api/health/combined` festhalten: `replay_pfad.parallel_pct`, `replay_phasen`, `zustands_ablehnung`, `absender_cache` und die Blockhöhe beider Knoten.
3. Derselbe Lauf mit `ENABLE_MULTI_BLOCK_TICK=1`, zuerst auf einem Knoten, dann auf beiden.
4. Einschalten, wenn der Durchsatz steigt, beide Knoten auf gleicher Höhe bleiben, `zustands_ablehnung` bei 0 bleibt und kein StateRoot-Fehler auftritt.

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

### Umgesetzt (26.09.2026)

Aktivierung per Zeit (`verteilteAnnahmeAbUnix`, aus). Sie greift immer erst mit dem nächsten Term, und nur mit Wahl (ab 3 Validatoren).

- **Zuständigkeit** (`zustaendigkeit.go`): `zuteilung[H(konto|term) mod n]`. Die globalen Konten (Töpfe, Pool, Faucet) gehören dem Leiter.
- **Wer ist Validator:** Die Menge, deren Mehrheit zählt, ist der Satz der bestehenden Leitung (Raft-Mitgliedschaft, `leitung.go`). Der Leiter legt beim Antritt die Zuteilung fest und schickt sie mit jeder Lease. Sie gilt fest für den Term und überlebt Neustarts.
- **Annahme nur mit bestätigter Lease** (`leitung_verteilt.go`). Ein Mitglied nimmt höchstens LeaseDauer/2 nach der letzten Lease an, die der Leiter mit Mehrheit bestätigt hat. Ein abgeschnittenes Mitglied hört so auf, bevor die Mehrheit wählen kann.
- **Übergabe an der Epochengrenze:** Der Leiter schickt einen Abschluss. Jedes Mitglied meldet „entleert“ mit seinem letzten Block. Die Neuen nehmen erst an, wenn sie alle diese Blöcke nachgespielt haben. Nach einer Wahl gilt stattdessen eine Ruhezeit.
- **Ausfall:** Der Leiter führt ein Mitglied als ausgefallen, dauerhaft für den Term. Der Nächste im Ring übernimmt nach LeaseDauer + Ruhezeit.
- **Weiterleitung** an den Zuständigen: RPC nach Absender, REST nach Konto.
- **Vermögensgrenze** (`kappung_verteilt.go`): Gutschriften kappen nicht mehr selbst. Der Zuständige kappt mit eigener Transaktion und festem Betrag.
- **Tausch und Liquidität** (`vorbehalt.go`) laufen in zwei Schritten. Zuerst legt der Zuständige des Kontos den Einsatz auf ein Vorbehaltskonto. Dann führt der Leiter den Auftrag aus oder erstattet. Doppelt ausführen ist ausgeschlossen, weil das Konto danach leer ist. Offene Vorbehalte überleben Neustarts.
- **Tägliche Verteilung:** im verteilten Term nur beim Leiter.

**Nachweise:**
- Simulation: nie zwei Annehmende für ein Konto, unter Ausfällen, Neustarts, Netztrennungen, Nachrichtenverlust und Uhrengang. Dazu opt-in 120 weitere Läufe mit 3 bis 7 Validatoren. Die Simulation fand zwei echte Fehler, beide behoben.
- Drei gleichzeitig annehmende Knoten mit leerlaufenden Konten bleiben bitgleich. Ohne Zuständigkeit laufen sie auseinander.
- Die Kappung ist bitgleich auch bei gleichzeitiger Ausgabe und Gutschrift.
- Der Tausch per Vorbehalt ist bitgleich über drei Knoten, erstattet beim Scheitern und führt nie doppelt aus.

**Kosten:** Jede geplante Übergabe lässt etwa 3 s niemanden annehmen. Bei 10 Minuten Amtszeit ist das ein halbes Prozent. Ein Tausch, dessen Konto nicht dem Leiter gehört, ist erst mit dem nächsten Block des Leiters ausgeführt.

**Bleibt wie vorher:** Registrierung (der Herkunftszwang bindet sie an den ausstellenden Knoten) und das Vertrauensmodell für Systemaufträge. Der Leiter rechnet die Verteilung, jeder spielt sie mit den getragenen Beträgen nach.

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

### Umgesetzt (26.09.2026): das Protokoll als Paket, mit Simulation

Das Paket `x/humanity/bereiche` enthält das ganze Protokoll als reine, deterministische Rechnung, ohne Netz und ohne Datenbank:

| Teil | Datei |
|---|---|
| Zustand je Bereich als Sparse-Merkle-Baum (Tiefe 256): Beweise für jedes Konto, neue Wurzel aus den Beweisen der berührten Konten | `smt.go` |
| Zustandsübergang eines Bereichsblocks, derselbe für Ausschuss, Prüfer, Fehlerbeweis und Referenz: Quittungen (genau einmal), Registrierung, Überweisungen (ed25519, Nonce, Gebühr), Ausstiege, Kappung, Grundeinkommen je Konto in O(1) | `zustand.go` |
| Bereichsblock, 2/3-Signaturen, Prüfausschuss, billige Prüfung (Unterschriften, Kette, Rahmen, Quittungssummen, Geldmenge über die Kopfkette) | `block.go` |
| Losung der Ausschüsse und des Prüfausschusses, Aktivierungsregel (≥ 32 und ≥ S × m) | `losung.go` |
| Sammelblock je Epoche: Geldmenge, Menschen, Grundeinkommen je Mensch, Grenze, Zufall der nächsten Losung, Erhaltung | `sammel.go` |
| Fehlerbeweis: berührte Konten mit Beweisen, nachrechnen, vergleichen. Ein Beweis gegen einen richtigen Block oder mit fehlenden Konten taugt nichts | `beweis.go` |

**Simulation** (`simulation_test.go`): 40 Validatoren, 4 Bereiche zu je 8, 120 Menschen. Über viele Epochen laufen Registrierungen, Überweisungen innerhalb und zwischen Bereichen, absichtlich ungültige Aufträge und Ausstiege. Geprüft wird:
- jeder Block besteht die billige Prüfung,
- die Geldmenge bleibt erhalten,
- jede Quittung wird genau einmal eingelöst,
- jedes Konto stimmt mit einem unabhängig geführten Hauptbuch überein.

Ein gekaperter Ausschuss (6 von 8) stiehlt und will auszahlen. Der Fehlerbeweis entdeckt ihn, der Prüfausschuss verweigert die Auszahlung, die sechs Unterzeichner werden ausgeschlossen, und am Ende steht derselbe Zustand wie ohne Angriff. Wird der Betrug erst zwei Epochen später gefunden, und hat die Beute das Gestohlene inzwischen weitergegeben, stoppt das Netz beim letzten bestätigten Stand. Es rechnet die Zeit danach neu und landet ebenfalls beim ehrlichen Zustand.

**Einbindung in den laufenden Knoten** (erst, wenn ≥ 32 unabhängige Validatoren in Sicht sind, siehe Aktivierung):
1. Bereichsblöcke und Sammelblock als eigene Nachrichtenarten im P2P-Netz, signiert mit den Validator-Schlüsseln (secp256k1 statt ed25519 der Simulation).
2. Speicherung je Bereich: `chain_accounts` erhält die Bereichsspalte. Der Baum wird beim Start aus den Konten gebaut.
3. Umstellung an einer festen Epoche: Der letzte Block der einen Kette wird zur Genesis aller Bereiche (Konten nach `BereichVon`).
4. Zufall aus einer VRF statt aus dem Sammelblock-Hash. Der letzte Ausschuss einer Epoche kann den Hash in Grenzen beeinflussen.
5. Wirtschaftsregeln (Unternehmen, Umlauf) als Teil des Bereichszustands. Heute rechnet die Simulation Überweisungen, Registrierung, Grundeinkommen, Kappung und Ausstiege.

---

## Stufe 4 — Gültigkeitsbeweise (Forschung)

- Je Block ein ZK-Beweis, dass der Zustandsübergang korrekt ist.
- Prüfen kostet dann Millisekunden, auch auf einem Handy. Übernehmen ohne Nachrechnen wird **ohne Vertrauen** möglich.
- Offen: Kosten des Beweisens, passende Schaltkreise für die Wirtschaftsregeln. Wird nicht vor Stufe 3 begonnen.

### Forschungsstand (26.09.2026): Prototyp und gemessene Kosten

Der Prototyp liegt in `forschung/zk-gueltigkeit`, einem eigenen Go-Modul. `gnark` würde sonst die Kryptobibliothek von `go-ethereum` im Hauptmodul anheben. Er erzeugt einen echten Gültigkeitsbeweis (Groth16 auf BN254) dafür, dass ein Bündel von Überweisungen den Zustand von einer alten zu einer neuen Wurzel führt.

Der Schaltkreis erzwingt für jede Überweisung:
- das Absenderkonto unter der Wurzel (MiMC-Merkle-Pfad),
- die Signatur des Absenders (EdDSA auf der Twisted-Edwards-Kurve von BN254),
- die Nonce,
- die Deckung (64-Bit-Bereich, keine Überziehung),
- verschiedene Konten für Absender und Empfänger,
- die neue Wurzel nach Belastung und Gutschrift.

Öffentlich sind nur die beiden Wurzeln. Die Tests prüfen, dass jede Fälschung den Beweis unmöglich macht: anderer Betrag, fremde Signatur, erfundenes Guthaben, verbrauchte Nonce, Überweisung an sich selbst, Überziehung, falsche Wurzel. Läuft in CI (Job `zk-forschung`).

**Gemessen** (4 Kerne, Groth16, MiMC):

| Konten | Bündel | Bedingungen je Überweisung | Beweis je Überweisung | Prüfung des Bündels | Setup |
|---|---|---|---|---|---|
| 256 (Tiefe 8) | 4 | 35.021 | 0,38 s | 1,0 ms | 16 s |
| 65.536 (Tiefe 16) | 16 | 56.237 | 0,52 s | 1,5 ms | 90 s |

**Was daraus folgt:**
- **Prüfen ist wirklich fast kostenlos.** Die Prüfung dauert 1 bis 1,5 ms, unabhängig von der Bündelgröße. Das gilt auch auf einem Handy. Das Ziel „Übernehmen ohne Nachrechnen“ ist technisch erreicht.
- **Beweisen ist der Engpass.** Eine Überweisung kostet etwa 2 Kernsekunden. Für 10.000 Überweisungen je Sekunde bräuchte man etwa 20.000 Kerne. Auf günstiger Hardware ist das nicht tragbar. Stufe 4 ersetzt das Nachrechnen deshalb nicht für den vollen Durchsatz, sondern ergänzt es.
- **Der Merkle-Pfad dominiert.** Pro Ebene kommen etwa 2.600 Bedingungen dazu. Bei Tiefe 32 (vier Milliarden Konten) wären es etwa 98.000 je Überweisung, davon rund 14.000 für die Signatur. Poseidon statt MiMC senkt das Hashen etwa um den Faktor 3 bis 5. Das ist der erste Hebel.
- **Das vertrauenswürdige Setup** von Groth16 bräuchte eine Mehrparteien-Zeremonie. Alternativen sind PLONK mit universellem Setup (in `gnark` vorhanden, Beweise etwas größer) oder STARKs ohne Setup (Beweise deutlich größer).

**Schaltkreise für die Wirtschaftsregeln** (abgeschätzt, nicht gebaut):
- Gebühr samt Staffel: Vergleiche und Multiplikation, einige hundert Bedingungen.
- Vermögensgrenze: ein Bereichsvergleich gegen die öffentliche Grenze, gut hundert.
- Grundeinkommen: nach dem Muster aus Stufe 3 (kumulierter Index, `GEStand`) eine Addition.
- Buchführung der Unternehmen: ein zweiter Baum, je berührtem Buchkonto ein weiterer Pfad, etwa +20.000.

**Empfehlung für den Weg dahin:**
1. Stufe 3 läuft optimistisch mit Fehlerbeweisen.
2. Beweise werden nachgereicht, je Bereichsblock und asynchron. Sobald ein Block bewiesen ist, endet sein Einspruchsfenster sofort. Umtausch und Ausstieg werden damit früher sicher, ohne dass jeder Knoten beweisen muss.
3. Beweisen ist eine offene Rolle, die jeder mit Rechenleistung übernehmen kann. Sie verleiht keinen Rang und keine Stimme. Geprüft wird von allen.
4. Nächste Messungen: Poseidon2, PLONK, GPU-Beweiser und rekursive Zusammenfassung der S Bereichsbeweise im Sammelblock.

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
