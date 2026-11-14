# Quantensicherheit – Entwurf (14.11.2026)

**Ziel:** „Ein Mensch – einmal 1.000 AEQ“ muss auch dann gelten, wenn es
Quantencomputer gibt. Niemand darf mit einem Quantencomputer Menschen
erfinden, fremde Wallets leeren oder Gesichtsdaten entschlüsseln können.

## 1. Begriffe: was gemeint ist und was nicht

| | Was es ist | Machbar? |
|---|---|---|
| **Post-Quanten-Kryptografie (PQC)** | Unterschriften und Verschlüsselung, die auch ein Quantencomputer nicht knackt. Läuft auf normalen Handys und Servern. NIST-Standards seit 2024: ML-DSA (FIPS 204), SLH-DSA (FIPS 205), ML-KEM (FIPS 203). | **ja, jetzt** |
| **Quantengeld** | Geldscheine, die selbst Quantenzustände sind und sich deshalb nicht kopieren lassen. Braucht Quantenspeicher in jedem Gerät; die Variante, die jeder selbst prüfen kann, ist mathematisch ungelöst. | **nein** |

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
1. Coordinator und Bezeuger bekommen je einen ML-DSA-65-Schlüssel; die
   öffentlichen Schlüssel kommen in die Genesis-/Register-Liste neben Ed25519.
2. Bescheinigung v4: dieselben Nachrichten (`…-v4|modus|nullifier|wallet|zeit`),
   je Schlüssel **Ed25519 und ML-DSA**. Ab Stichtag lehnt jeder Knoten eine
   Registrierung ohne gültige ML-DSA-Unterschriften ab (fail-closed).
3. Biometrie-Repo (Python): ML-DSA im Coordinator und Vergleichsdienst.
   Offene Frage: Bibliothek (pyca/cryptography über OpenSSL ≥ 3.5, oder
   liboqs) – nur mit Begründung und Prüfung.
4. Missbrauchstests: fehlende/gefälschte ML-DSA-Unterschrift, nur Ed25519,
   fremder Schlüssel, Wiederholung, übergroße Unterschrift.
   **Danach ist „1 × 1.000“ an der Stelle, wo AEQ entsteht, quantensicher.**

**Stufe 2 – Zusage für jede Wallet (Konsens, klein)**
1. Neue Transaktion „PQ-Zusage“: die Wallet hinterlegt einmalig
   `SHA-256(ML-DSA-Schlüssel)`. Die App erzeugt den Schlüssel aus derselben
   Seed-Phrase (eigener Ableitungspfad), der Nutzer muss nichts Neues sichern.
2. Kostet 32 Byte je Wallet und macht heute nichts – aber nach Stufe 4 ist
   genau sie der Beweis, wem die Wallet gehört.

**Stufe 3 – PQ-Wallets (Konsens + App)**
1. Neuer Transaktionstyp mit ML-DSA-65-Unterschrift; Adresse = Hash des
   ML-DSA-Schlüssels.
2. App: ML-DSA in JavaScript/nativ (Kandidat `@noble/post-quantum`; neue
   Abhängigkeit, nur mit Prüfung). Umzug von alter auf neue Adresse in der App
   mit einem Knopf.
3. Durchsatz: Unterschrift ~50× größer als ECDSA, Prüfung ähnlich schnell.
   Vorher im Prüfstand messen.

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

**Stufe 6 – Forschung: Quantengeld (kein Code)**

Das eigentliche Ziel hinter „quantischem Geld“ ist ein echtes, offenes
Problem: Bargeld, das man **ohne Kette und ohne Bank** weitergeben kann und
das sich trotzdem nicht kopieren lässt – offline und privat.

- *Stand der Forschung (2025/26):* Öffentlich prüfbares Quantengeld hat
  Kandidaten (Zhandry 2024, aus abelschen Gruppenaktionen; 2025 im generischen
  Modell als sicher bewiesen), aber **keinen Beweis aus Standardannahmen**;
  mehrere ältere Verfahren sind gebrochen (Aaronson 2009, Gitter-Varianten).
- *Was fehlt außer der Mathematik:* Jeder Inhaber bräuchte einen
  Quantenspeicher, der Zustände über Tage hält, und einen Quantenrechner zum
  Prüfen. Beides gibt es in keinem Handy, und auf absehbare Zeit nicht.
- *Wie Aequitas dazu passen könnte (Gedankenmodell, keine Zusage):* Die Kette
  bleibt die Ausgabestelle – „ein Mensch, einmal 1.000 AEQ“ entsteht dort. Wer
  später Quanten-Banknoten will, sperrt AEQ auf der Kette und erhält dafür
  Noten mit öffentlicher Seriennummer; wer eine Note zurückgibt, bekommt die
  AEQ wieder. Die Einmaligkeit des Menschen leistet weiterhin die
  Registrierung, die Unkopierbarkeit der Noten die Physik.
- *Was wir jetzt tun können:* die Literatur verfolgen, mit Forschungsgruppen
  sprechen (z. B. QuSoft Amsterdam, Gruppen um Aaronson und Zhandry) und
  Aequitas so bauen, dass eine solche Ausgabestelle später andocken kann –
  also genau die Stufen 0–5. Wer „Quantenwährung“ heute verspricht, verspricht
  etwas, das niemand liefern kann.

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
