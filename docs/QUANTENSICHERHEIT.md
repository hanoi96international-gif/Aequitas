# Quantensicherheit – Stand 15.11.2026

**Stand auf einen Blick** (live geprüft 15.11.2026, 08:56 UTC):

| Stufe | Was | Stand |
|---|---|---|
| 0 | Biometrie-Sicherung mit hybridem age (X25519 + ML-KEM) | vorbereitet; wartet auf den PQ-Schlüssel des Betreibers |
| 1 | Registrierung hybrid (Ed25519 + ML-DSA-65) | Schlüssel in der Genesis, beide Dienste unterschreiben (`/health`: Fingerabdrücke = Genesis); jede PQ-Unterschrift wird vor dem Stichtag streng geprüft und gezählt; **scharf nach der ersten echten Registrierung** (Wächter `bio-pq-bereit.sh`) |
| 2 | PQ-Zusage je Wallet | **live seit 14.11.2026, 22:00 UTC** |
| 3 | PQ-Adressen, Überweisungen mit ML-DSA-65 (App 1.20.0, Tresor) | **live seit 14.11.2026, 22:00 UTC** |
| 5 | PQ-Stempel je 60 Blöcke (ML-DSA-65 des Erzeugers) | **Pflicht seit Höhe 2.134.680 (15.11.2026, ~08:55 UTC)**: ein Block auf einer 60er-Höhe ohne gültigen Stempel ist für jeden Knoten ungültig. Block 2.134.680 unabhängig gegen den Schlüssel in `genesis.json` geprüft: gültig (veränderter Hash: abgewiesen). Die Wache meldet, wenn der Erzeuger nicht stempelt. Komitee-Checkpoints hybrid, schlafend bis ≥ 4 Erzeuger |
| 6 | Quantengeld (Forschung) | Weg vollständig beziffert (`forschung/quantengeld/BAUVORGABE.md` §8). **Ganzes Geld durchgespielt** (`labor.py`: jeder prägt, Anmelden mit Deckung, Prüfen, Angriffe). **Kettenseite im Knoten** (`quantenPraegen`, h < N je Parametersatz) – schlafend, kein Satz eingetragen. **Auf Quanten-Hardware** (Spielzeuggröße, Workflow „Quantengeld auf Quanten-Hardware“): Rauschmodelle echter IBM-Maschinen unterscheiden echte Noten (0,89–0,95) von Fälschungen (≤ 0,25); **erster Lauf auf dem echten Chip `ibm_marrakesh` (15.11.2026): echte Note 0,74, beste Fälschung 0,22**. Die Lücke ist das große Prüfgerät (10⁸–10¹⁰ logische Qubits) |

**Ziel:** „Ein Mensch – einmal 1.000 AEQ“ muss auch dann gelten, wenn es
Quantencomputer gibt. Niemand darf mit einem Quantencomputer Menschen
erfinden, fremde Wallets leeren oder Gesichtsdaten entschlüsseln können.

## 1. Begriffe: was gemeint ist und was nicht

| | Was es ist | Machbar? |
|---|---|---|
| **Post-Quanten-Kryptografie (PQC)** | Unterschriften und Verschlüsselung, die auch ein Quantencomputer nicht knackt. Läuft auf normalen Handys und Servern. NIST-Standards seit 2024: ML-DSA (FIPS 204), SLH-DSA (FIPS 205), ML-KEM (FIPS 203). | **ja, jetzt** |
| **Quantengeld** | Geldscheine, die selbst Quantenzustände sind und sich deshalb nicht kopieren lassen. Braucht Quantenspeicher in jedem Gerät. Für die Variante, die jeder selbst prüfen kann, gibt es seit 2025 ein Verfahren auf Isogenien (Zhandry); unser Weg dahin steht in `forschung/quantengeld/BAUVORGABE.md` §8. | **Forschung; Spielzeuggröße läuft auf heutiger Hardware, Alltagsgröße braucht ein Prüfgerät, das es noch nicht gibt** |

Aequitas wird **quantensicher**, nicht „Quantengeld“. So steht es auch nach
außen: wir werben nie mit „Quantenwährung“.

**Warum „1 Person = 1 Registrierung“ nicht aus der Quantenphysik kommen kann:**
Das Kopierverbot der Quantenphysik schützt einen *Token* davor, verdoppelt zu
werden, nicht eine *Person* davor, sich zweimal anzumelden. Wer zweimal kommt,
bekäme einfach zwei echte Quanten-Token. Dass ein Mensch nur einmal zählt,
leistet bei Aequitas der Gesichtsabgleich (Einmaligkeit) – und das bleibt so.
Was der Quantencomputer angreift, sind die **Unterschriften**, die dieses
Ergebnis in die Kette tragen. Die machen wir quantensicher.

## 2. Bedrohung: was ein Quantencomputer bricht

- **Shor-Algorithmus** bricht alles, was auf elliptischen Kurven oder RSA
  beruht: secp256k1 (Wallets), Ed25519 (Coordinator, Bezeuger, Peers), X25519
  (Verschlüsselung), BN254 (Groth16-Beweise).
- **Grover** halbiert nur die Stärke von Hashes und symmetrischer
  Verschlüsselung: SHA-256, Keccak, AES-256 bleiben sicher.
- **Wann?** Google Quantum AI schätzt 2026 für secp256k1 unter 1.500 logische
  bzw. unter 500.000 physikalische Qubits – ein Zehntel älterer Schätzungen.
  Eine solche Maschine gibt es noch nicht. NIST (IR 8547, endgültig 27.10.2026)
  stuft ECDSA und EdDSA ab 2030/31 als veraltet ein und verbietet sie nach
  2035; Hybrid aus klassisch + PQC bleibt erlaubt. Ethereum plant volle
  PQ-Sicherheit bis 2029, Bitcoin hat BIP-360 (neuer Adresstyp) als Entwurf.
- **„Jetzt sammeln, später entschlüsseln“:** Verschlüsselte Daten, die heute
  abgegriffen werden, lassen sich später öffnen. Für Unterschriften gilt das
  nicht – eine Fälschung muss *dann* passieren, wenn die Maschine existiert.
  **Deshalb ist Verschlüsselung dringender als Unterschriften.**

## 3. Bestandsaufnahme Aequitas

| Bereich | heute | Quantenrisiko | Dringlichkeit |
|---|---|---|---|
| **Biometrie-Sicherung** (`backup-biometrie-verschluesselt.yml`) | age mit X25519 (`age1…`). Das verschlüsselte Archiv liegt 30 Tage als Artefakt im **öffentlichen** Repo; jeder angemeldete GitHub-Nutzer kann es laden. | Heute laden, in einigen Jahren entschlüsseln: Gesichts-Auszüge sind ein Leben lang heikel. | **hoch – sofort** |
| **Registrierung** (`bio_bescheinigung_konsens.go`) | Bescheinigung mit **Ed25519** (Coordinator bindet, Bezeuger sagt aus); dazu Groth16-Beweis (BN254) und EIP-712-Unterschrift der Wallet (secp256k1). | Wer den Coordinator- oder Bezeuger-Schlüssel per Quantenrechner ableitet, kann beliebig viele „Menschen“ bescheinigen – je 1.000 AEQ. Groth16 wäre dann ebenfalls fälschbar. Das ist der direkte Angriff auf „1 × 1.000“. | **hoch** |
| **Wallets** (secp256k1, EVM-kompatibel) | Öffentlicher Schlüssel wird mit der ersten Überweisung sichtbar. | Privater Schlüssel daraus ableitbar → Guthaben und Menschsein der Wallet gestohlen. | mittel (bis eine Maschine existiert) |
| **Wiederherstellung/Umzug** | Wiederherstellungscode als **Hash** auf der Kette, dazu Gesicht + 14 Tage Zeitschloss. | Hash ist quantensicher. Ein Angreifer, der den Code im Umlauf sieht, kommt ohne passendes Gesicht nicht durch. **Der Umzug ist schon heute ein quantensicherer Rettungsweg fürs Menschsein** (nicht fürs Guthaben). | gering |
| **Blöcke, Checkpoints, Register** | Erzeuger-Unterschriften secp256k1. | Wer den Erzeuger-Schlüssel ableitet, kann Blöcke im Namen von C1 signieren. Jeder Knoten rechnet aber selbst nach (Kontostände, Bescheinigungen): gefälschte Buchungen fielen auf, falsche Reihenfolge nicht. | mittel |
| **Netz** | libp2p: Ed25519-Kennung, Noise mit X25519. HTTPS über Caddy 2 (Go ≥ 1.24: hybrides X25519MLKEM768 ist dort Standard, sobald der Client es kann). | Peer-Kennungen fälschbar; Mitschnitte des Peer-Verkehrs später lesbar (nur öffentliche Kettendaten). | gering |
| **Hashes** (SHA-256, Keccak, Poseidon) | – | Grover halbiert, bleibt ausreichend. | keine |

## 4. Bausteine

- **ML-DSA (FIPS 204)** für Unterschriften. Go 1.27 bringt `crypto/mldsa` in
  der Standardbibliothek (aus dem FIPS-140-3-Modul) – **keine neue
  Abhängigkeit** für den Knoten. Stufe: **ML-DSA-65** (wie von der Ethereum
  Foundation für langlebige Schlüssel empfohlen). Größen: öffentlicher
  Schlüssel 1.952 Byte, Unterschrift 3.309 Byte (ECDSA: 65 Byte).
- **Hybrid statt Ersatz:** In der Übergangszeit muss **beides** stimmen,
  klassisch *und* ML-DSA. Bricht eines von beiden (Quantenrechner gegen
  Ed25519, oder ein Fehler im neuen Verfahren), hält das andere. NIST lässt
  Hybrid auch nach 2035 zu.
- **age ≥ 1.3 mit `-pq`** (ML-KEM-768 + X25519) für Sicherungen. age weist
  eine Datei mit gemischten (klassisch + PQ) Empfängern ab – gut so.
- **Hash-Zusagen:** Ein Hash eines künftigen PQ-Schlüssels kostet 32 Byte und
  ist heute schon quantensicher. Wer ihn jetzt hinterlegt, kann später
  beweisen, dass ihm die Wallet gehört, selbst wenn ECDSA gebrochen ist.

## 5. Plan

Reihenfolge nach Risiko, nicht nach Aufwand. Jede Stufe mit Stichtag wie beim
Umzug: vor dem Stichtag gilt das Alte, danach das Neue, jeder Knoten prüft
selbst; Sperren in `.github/neustart/` verhindern ein Ausrollen nach dem
Stichtag.

**Stufe 0 – Sicherungen quantensicher (kein Konsens, sofort)** – *Code fertig
(14.11.2026); wirksam, sobald der Betreiber den `age1pq1…`-Schlüssel einträgt.*
1. Betreiber erzeugt offline `age-keygen -pq -o bio-backup-pq.key` und trägt
   den öffentlichen Schlüssel (`age1pq1…`, ~2.000 Zeichen) mit
   `sicherung-einrichten.yml` ein.
2. Code: age ≥ 1.3 auf C1 (geprüftes Release statt apt), Prüfungen in
   `backup-biometrie.sh`, `sicherung-befehl.sh`, `einrichten.sh`,
   `sicherung-einrichten.yml`, `schluessel-sichern.yml` nehmen `age1pq1…` an;
   nach einer Übergangsfrist **nur** noch `age1pq1…`.
3. Alte X25519-Artefakte löschen, sobald die erste PQ-Sicherung grün ist.
4. Artefakte nicht mehr im öffentlichen Repo halten (eigenes privates Repo
   oder nur auf dem Server und außer Haus) – unabhängig von Quanten.

**Stufe 1 – Registrierung hybrid (Konsens, Stichtag)**

*Geprüft am 14.11.2026:* Eine ML-DSA-65-Unterschrift aus Python
(`cryptography` 50.0.2, schon Abhängigkeit von Coordinator und
Vergleichsdienst) prüft Go 1.27 (`crypto/mldsa`, Standardbibliothek) korrekt;
geänderte Nachricht oder ein gekipptes Bit werden abgewiesen. **Keine neue
Abhängigkeit** auf beiden Seiten. Größen: Schlüssel 1.952, Unterschrift 3.309
Byte.

Heute läuft die Bescheinigung so: Coordinator (`bio_attestation.py`) und
Vergleichsdienst (`attest_sign.py`) unterschreiben mit Ed25519, die App reicht
die Unterschriften beim Beweis durch (`lib/attestation.ts`), jeder Knoten prüft
sie gegen die Genesis-Schlüssel (`bio_bescheinigung_konsens.go`, Form v3).

Umsetzung in dieser Reihenfolge, jede Stufe ein eigener PR mit Prüfung:

1. **Knoten auf Go 1.27** – nur der Wechsel der Werkzeugkette (`go.mod`,
   `Dockerfile`, CI), voller Testlauf mit `-race`, `govulncheck`; ausrollen.
2. **Unterschreiben (Biometrie-Repo):** Coordinator und Vergleichsdienst
   unterschreiben *zusätzlich* mit ML-DSA-65, Nachricht `…-v4|…` (gleicher
   Inhalt wie v3). Seeds (32 Byte) als eigene Secrets, auf C1 erzeugt, in
   `schluessel-sichern.yml` mitgesichert. Ohne Seed: keine v4-Unterschrift,
   nichts sonst ändert sich.
3. **App:** reicht das neue Feld unverändert durch; feste Größengrenzen.
4. **Prüfen (Knoten):** Genesis bekommt je Schlüssel den ML-DSA-Schlüssel neben
   Ed25519. Ab Stichtag `bescheinigungPQAb` (bis dahin `MaxInt64`) gilt eine
   Bindung bzw. Aussage nur, wenn **beide** Unterschriften desselben Prüfers
   gültig sind; gezählt werden weiter Schlüssel, nie Unterschriften. Grenzen:
   Unterschrift genau 3.309 Byte, höchstens so viele wie Schlüssel in der
   Genesis. Sperre in `.github/neustart/` gegen ein Ausrollen nach dem Stichtag.
5. **Missbrauchstests:** nur Ed25519; nur ML-DSA; ML-DSA von fremdem Schlüssel;
   ML-DSA über andere Wallet/anderen Nullifier/andere Zeit; Wiederholung;
   übergroße und abgeschnittene Unterschrift; zwei Unterschriften desselben
   Schlüssels zählen einmal; Block vor dem Stichtag ohne ML-DSA bleibt gültig
   (Nachspielen der alten Kette).
6. **Stichtag setzen**, erst wenn 1–3 live sind und eine echte Registrierung
   mit beiden Unterschriften durchging.

**Danach ist „1 × 1.000“ an der Stelle, wo AEQ entsteht, quantensicher.**

**Stufe 2 – Zusage für jede Wallet (Konsens, klein)** – *Knoten: `pq_zusage.go`, Stichtag `pqZusageAbUnix` = 14.11.2026 22:00 UTC*
1. Neue Transaktion `pq_zusage` (`POST /api/pq/zusage`, Stand in
   `GET /api/umzug?wallet=`): die Wallet hinterlegt einmalig
   `SHA-256(ML-DSA-65-Schlüssel)` (die 1.952 Rohbytes des öffentlichen
   Schlüssels), unterschrieben mit EIP-191 über
   `aequitas-pq-zusage-v1|chain:1926|<wallet>|<hash>|<zeit>`.
2. **Ableitung (App):** ML-DSA-Seed ξ = HMAC-SHA256(Schlüssel
   `"aequitas-mldsa65-v1"`, BIP-39-Seed der Seed-Phrase, leere Passphrase);
   daraus der Schlüssel nach FIPS 204. **Nie aus dem secp256k1-Schlüssel** –
   den kann ein Quantenrechner ableiten, den BIP-39-Seed nicht. Der Nutzer
   muss nichts Neues sichern. Die App speichert nur den Wallet-Schlüssel, nicht
   die Seed-Phrase: die Zusage entsteht beim Anlegen/Importieren mit
   Seed-Phrase; bestehende Wallets geben die Seed-Phrase dafür einmal ein.
3. Regeln, die jeder Knoten selbst prüft: nur ein Mensch (`is_human`, das
   begrenzt das Register ohne Gebühr), **genau eine Zusage je Wallet, nicht
   änderbar** (ein Angreifer mit abgeleitetem secp256k1-Schlüssel kann sie
   nicht ersetzen), unterschrieben frühestens zum Stichtag, höchstens 5 min
   voraus und einen Tag alt, höchstens 50.000 Zusagen (dann eigene Tabelle).
   Gespeichert im Umzugs-Register (`chain_config`, StateRoot); ohne Zusagen
   bleibt dessen Schreibweise byte-gleich. Ein Umzug lässt die Zusage bei der
   Wallet – das Guthaben bleibt ja auch dort.
4. Kostet 32 Byte je Wallet und macht heute nichts – aber nach Stufe 4 ist
   genau sie der Beweis, wem die Wallet gehört. **Restrisiko:** eine Wallet
   *ohne* Zusage kann ein Angreifer, der secp256k1 schon bricht, zuerst
   belegen. Deshalb zählen bei der Notbremse nur Zusagen von *vor* ihrem
   Stichtag – und die App legt die Zusage gleich bei der Einrichtung an.
   Unternehmens-Wallets (kein Mensch) folgen mit eigener Grenze.

**Stufe 3 – PQ-Wallets (Konsens + App)** – *Knoten: `pq_ueberweisung.go`, Stichtag `pqUeberweisungAbUnix` = 14.11.2026 22:00 UTC*
1. **PQ-Adresse** = `"0x"` + letzte 20 Byte von
   `SHA-256("aequitas-pq-adresse-v1" || ML-DSA-65-Schlüssel)`. Sie empfängt
   mit jeder gewöhnlichen Überweisung. **Ausgeben** geht nur mit einer
   ML-DSA-65-Unterschrift: eine EVM-Rohform bräuchte einen secp256k1-Schlüssel,
   dessen Keccak-Adresse genau diese 20 Byte trifft (Urbild, auch mit Grover
   2^80 Schritte je Adresse). Kein ML-DSA-Schlüssel ergibt eine gewöhnliche
   Adresse.
2. **Überweisung** = gewöhnliche `transfer`-Transaktion, die statt der
   Rohform `Roh` den Nachweis `pq` trägt (Schlüssel, Unterschrift, Nonce,
   Betrag in Wei). Unterschrieben wird
   `aequitas-pq-ueberweisung-v1|chain:1926|<von>|<an>|<wei>|<nonce>`;
   `tx_hash` = SHA-256 dieser Nachricht. Gebühr, Kontostände, Buchführung,
   Kontoverlauf und Nonce (`NaechsteNonce`) laufen unverändert über die Wege
   der Überweisung; jeder Knoten prüft die Unterschrift beim Nachspielen
   selbst, vorab und parallel wie die EVM-Unterschriften.
3. **Annahme** `POST /api/pq/ueberweisung` (`{von, an, wei, nonce, pub, sig}`)
   nur auf dem seriellen Weg – der WAL-Schnellpfad schriebe den Nachweis nicht
   mit. Stand einer Adresse (Nonce, Guthaben): `GET /api/pq/stand?adresse=`.
4. **Grenzen:** höchstens 500 PQ-Überweisungen je Block (rund 5,4 MB; ein Block
   darf 32 MB groß sein) – der Erzeuger schneidet in Reihenfolge ab, jeder
   Knoten weist mehr ab. Körper ≤ 16 KiB, Burst je IP, Betrag ≤ 10^30 Wei.
5. **App:** der „Tresor“ – die PQ-Adresse aus derselben Seed-Phrase
   (Schlüssel wie Stufe 2). Guthaben hineinschieben ist eine gewöhnliche
   Überweisung; heraus geht es nur mit ML-DSA. `@noble/post-quantum`
   (geprüft in Aequitas-App #65).
6. **Noch nicht** (eigene Schritte): das Menschsein und das Grundeinkommen auf
   eine PQ-Adresse umziehen; Unternehmen; EVM-Verträge von PQ-Adressen aus.

**Stufe 4 – Notbremse (Konsens, Stichtag erst bei echter Gefahr)**
- Ab einem Stichtag nehmen die Knoten von reinen ECDSA-Wallets nichts mehr an.
  Wer eine Zusage (Stufe 2) hat, zieht mit seiner ML-DSA-Unterschrift auf eine
  PQ-Adresse um. Ohne Zusage bleibt der Umzug mit Gesicht + Code (rettet das
  Menschsein, nicht das Guthaben).
- Der Stichtag wird **nicht** jetzt festgelegt, sondern wenn Fachleute einen
  Quantenrechner in Reichweite sehen; die Regel liegt bis dahin schlafend im
  Code (`MaxInt64`), wie andere Stichtage auch.

**Stufe 5 – Validatoren und Netz**
- Erzeuger- und Checkpoint-Unterschriften hybrid (secp256k1 + ML-DSA).
- libp2p: hybride Schlüssel, sobald go-libp2p sie anbietet; bis dahin sind nur
  öffentliche Kettendaten betroffen.

*Gebaut (14.11.2026), Entscheidungen des Betreibers: „die beste Entscheidung“, „direkt bauen“.*

| Größe | Messung |
|---|---|
| Block heute (Mittel aus 50 Live-Blöcken, JSON) | 756 Byte |
| ML-DSA-65-Unterschrift | 3.309 Byte (6.618 als Hex) |
| Unterschreiben / Prüfen (Go `crypto/mldsa`, 2,1-GHz-Xeon) | 0,34 ms / 0,21 ms |

**PQ-Stempel (`pq_stempel.go`), wirkt mit dem einen Erzeuger von heute:** Auf
jeder 60. Höhe (bei 1 Block/s einmal je Minute) unterschreibt der Erzeuger
zusätzlich mit ML-DSA-65 über `aequitas-pq-stempel-v1|chain:1926|<Blockhash>`.
Der Hash deckt die Eltern, also die ganze Geschichte dahinter. Ab der
Stichhöhe `pq_stempel.ab_hoehe` (genesis.json) weist **jeder Knoten selbst** einen Block
auf einer Stempelhöhe ohne gültigen Stempel seines Erzeugers ab; auf allen
anderen Höhen muss das Feld leer sein. Wer secp256k1 mit einem Quantenrechner
bricht, kommt höchstens bis zur nächsten Stempelhöhe. Kosten ≈ 110 Byte je
Block im Mittel statt 6.600. Vor der Stichhöhe wird nur die Form erzwungen;
gültige und falsche Stempel werden gezählt (`/api/status` → `pq.stempel`),
damit man sieht, dass sie stimmen, bevor sie Pflicht werden.

**Schlüssel:** wie Stufe 1 – `erzeuger-pq-schluessel-anlegen.yml` erzeugt den
Seed auf C1 (verlässt die Box nie, wird nie ersetzt), der Knoten leitet den
öffentlichen Schlüssel ab (`/api/pq/stand?stempel=1`), der Lauf gleicht ihn
byte-genau mit genesis.json ab. **Neue Erzeuger** binden ihren PQ-Schlüssel
mit einer Erklärung, die *beide* Schlüssel unterschreiben
(`pruefePQErzeugerBindung`, `checkpoint_pq.go`).

**Komitee-Checkpoints** (`checkpoint_pq.go`, schlafend wie das Zertifikat
selbst, das erst ab 4 Mitgliedern etwas bestätigt): jeder 10. Checkpoint
(alle 10 Minuten) braucht zu jeder secp256k1- auch eine ML-DSA-Unterschrift –
bei bis zu 67 Unterzeichnern rund 370 Byte je Block statt 3,3 KB je Minute
und Mitglied.

**Reihenfolge zum Scharfschalten:** ausrollen → Schlüssel-Workflow
(`anlegen`) → Zeile in genesis.json → ausrollen → `geprueft_ok` steigt,
`geprueft_falsch` bleibt 0 → `pq_stempel.ab_hoehe` setzen (eine Höhe, keine
Zeit: die Blockzeit wählt der Erzeuger, die Höhe prüft jeder Knoten – ein
Fälscher könnte sonst zurückdatieren; Wächter wie bei den Stichtagen).

**Stufe 6 – Quantengeld: eigene Forschung, Bauvorgabe, Kettenseite** (`forschung/quantengeld/`)

Das Ziel hinter „quantischem Geld“ ist ein echtes, offenes Problem: Bargeld,
das man **ohne Kette und ohne Bank** weitergeben kann und das sich trotzdem
nicht kopieren lässt – offline und privat. Aequitas forscht daran selbst
(Auftrag des Betreibers vom 14.11.2026: „Wir sind die Forschungsgruppe“).
Alles ist ausführbar und läuft als Selbsttest in der CI.

**Was wir gebaut und gemessen haben**

| Baustein | Ergebnis | Datei |
|---|---|---|
| Hardware-Maßstab (Bank prüft) | Kohärenz ≈ 2,5–10 × Lagerzeit | `rauschen.py` |
| Preis der Öffentlichkeit | echte Note besteht genau mit (1−p)^m → ohne Fehlerkorrektur unbrauchbar | `rauschen_oeffentlich.py` |
| Echte Isogenie-Gruppenaktion | CSIDH-Art, selbst gebaut, geprüft (h = 27 und 1.491) | `isogenie_geld.py` |
| Mehrteilige Note | Fälschen/Aufteilen ≤ 2⁻¹⁹²; bloße Mehrheit fällt dem Aufteilen zum Opfer | `isogenie_mehrteilig.py` |
| Angriffsstärke | beide Kuperberg-Siebe gemessen, trifft Peikert (512 Bit) | `kuperberg.py` |
| Kosten je Abfrage | Multiplikationen gezählt, Toffoli-Schätzung | `isogenie_kosten.py` |
| Einlösen | Einmal-Signatur; Kettenseite als Spezifikation, 17 Angriffe abgewiesen | `bruecke_kette.py` |

**Bauvorgabe** (`BAUVORGABE.md`, aus eigener Messung, konservativ):
- Sicherheitsziel **2⁻¹⁹²** (ML-DSA-65-Niveau, wie Stufen 1–3).
- Gruppenaktion CSIDH-Art mit **p ≥ 17.000 Bit** (2¹²⁸ ab 8.192 Bit;
  512 Bit sind gebrochen).
- Note aus k Teilnoten mit Schwelle τ > k/2 (Abstand gegen Aufteilen);
  je Teilnote **≈ 17.000 fehlerkorrigierte logische Qubits** (die Kurve
  wird als Zahl in F_p gespeichert: log₂ p Qubits).
- Einlösen per Einmal-Signatur; die Kette prüft klassisch und erzwingt
  „höchstens einmal je Seriennummer“ selbst.

**Was noch fehlt – und warum es heute niemand liefern kann**
1. Fehlerkorrigierte Quantenspeicher in dieser Größe, im Gerät statt im Labor.
2. Eine praktisch nutzbare Einmal-Signatur (im Standardmodell bisher nur über
   Ununterscheidbarkeits-Verschleierung, Shmueli & Zhandry 2025).
3. Ein Sicherheitsbeweis aus Standardannahmen für die Gruppenaktions-Note
   (bisher im generischen Modell, Doliskani 2025).

**Im Knoten** (`x/humanity/keeper/quanten_note.go`, schlafend): die
Prüfregeln der Kettenseite – Seriennummer, Einlöse-Nachricht Byte für Byte,
Einmal-Signatur-Prüfung, Register (Sperren, Einlösen, Rückholen) –, gegen
eine von der Spezifikation erzeugte Unterschrift abgeglichen, mit denselben
17 Angriffen, Erhaltung und Mutationsproben. Die Transaktionsarten sind
unbekannt (Block mit ihnen ungültig); ein Test verhindert das Einschalten,
solange die Lamport-Platzhalter-Signatur statt des Quantenverfahrens
eingetragen ist. **Einschalten** erst, wenn 1–3 erfüllt sind – zusammen mit
dem Anschluss an den Geldweg (Annahme, Nachspielen, Nachrechnen, Kein-Entzug)
in einem eigenen, eigens geprüften Schritt. Wer „Quantenwährung“ heute
verspricht, verspricht etwas, das niemand liefern kann; Aequitas liefert
heute die quantensichere Grundlage (Stufen 1–3 live) und die vollständige
Bauvorgabe für den Tag, an dem die Hardware kommt.

## 6. Sicherheit (AGENTS.md)

Für jede Stufe gilt der Launch-Maßstab: getrennte Sicherheitsprüfung des
Diffs, Missbrauchstests (Fälschung, Wiederholung, fremder Schlüssel,
Übergröße), feste Größengrenzen für die neuen, großen Unterschriften und
Schlüssel, `go test -race`, `govulncheck`. Keine Prüfung wird für den
Durchsatz weggelassen; ML-DSA kommt **zusätzlich**, nicht statt.

## Quellen

- Google Quantum AI, „Securing Elliptic Curve Cryptocurrencies against Quantum
  Vulnerabilities“ (2026): https://quantumai.google/static/site-assets/downloads/cryptocurrency-whitepaper.pdf
- NIST IR 8547 (endgültig 27.10.2026): https://postquantum.com/security-pqc/nist-ir-8547-final/
- Ethereum, Quantenresistenz: https://ethereum.org/roadmap/future-proofing/quantum-resistance/
- BIP-360: https://cointelegraph.com/news/bitcoin-s-quantum-upgrade-path-what-bip-360-changes-and-what-it-does-not
- Go `crypto/mldsa`: https://pkg.go.dev/crypto/mldsa
- age mit Post-Quanten-Schlüsseln: https://github.com/FiloSottile/age
- Zhandry, Quantengeld aus abelschen Gruppenaktionen: https://arxiv.org/pdf/2307.12120
- Sicherheit im generischen Modell (2025): https://eprint.iacr.org/2025/092
