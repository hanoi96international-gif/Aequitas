# Bescheinigung im Konsens einschalten

**Stand 22.10.2026 — gebaut, schlafend.** Hintergrund: `AUDIT_2026-10-21.md`, K1.

## Worum es geht

Bisher prüfte nur der annehmende Knoten (über seinen Proof-Server), dass eine
Registrierung durch die Gesichtsprüfung ging. Jeder andere Knoten sah nur einen
gültigen Groth16-Beweis — und den kann jeder erzeugen, der Blöcke erzeugt: der
Beweisschlüssel ist öffentlich, `bio` eine Zufallszahl. Ein Erzeuger konnte so
beliebig viele „Menschen“ anlegen.

Ab dem Stichtag trägt jede `register_human` die Bescheinigung (`bio`,
Zeitpunkt, Modus, Ed25519-Unterschriften von Coordinator und
Vergleichsdienst), und **jeder** Knoten prüft selbst:
`Poseidon(bio) = Nullifier des Beweises`, Unterschriften von Schlüsseln aus der
Genesis, Bindung an genau diese Wallet, Quorum (`x/humanity/keeper/bio_bescheinigung_konsens.go`).

`bio` ist kein biometrisches Merkmal, sondern die Zufallszahl des Coordinators.
Sie steht danach öffentlich im Block; aus ihr lässt sich kein Gesicht und keine
Wallet ableiten.

## Einschalten (Betreiber)

1. Die öffentlichen Schlüssel holen (64 Hex, klein):
   - Coordinator: `COORDINATOR_PUBLIC_KEYS` des Proof-Servers auf C1 (bzw.
     `attestation_key` aus `/health` des Coordinators, ungekürzt auf der Box).
   - Vergleichsdienst: `PERSONHOOD_PUBLIC_KEYS` des Proof-Servers.
2. In `genesis.json` eintragen — **dieselben Werte wie der Proof-Server**:
   ```json
   "bescheinigung": {
     "ab_unix": 1793750400,
     "modus": "test",
     "quorum": 1,
     "coordinator": ["<64 hex>"],
     "bezeuger": ["<64 hex>"]
   }
   ```
   `ab_unix`: mindestens zwei Tage nach dem Deploy. `modus` = `ERWARTETER_BIO_MODUS`
   des Proof-Servers, `quorum` = `BIO_ATTESTATION_QUORUM`. JSON ohne Kommentare —
   eine unlesbare Angabe hält den Knoten an.
3. Per PR mergen und auf **alle** Knoten ausrollen, bevor `ab_unix - 1 Tag`
   erreicht ist. Ab dann verlangt die Annahme die Bescheinigung; ab `ab_unix`
   weist jeder Knoten einen Block mit einer Registrierung ohne gültige ab.
4. Prüfen: `/api/status` → `bescheinigung_konsens` zeigt Stichtag, Modus,
   Quorum und die Zahl der Schlüssel.

Ein Schlüsselwechsel (neuer Coordinator, zweiter Vergleichsdienst) ist eine
neue Genesis-Fassung mit neuem Stichtag — auf allen Knoten zugleich.

Ein Knoten mit unlesbarer `bescheinigung` (Tippfehler, Schlüssel kleiner
Ordnung, Quorum größer als die Zahl der Schlüssel) startet nicht.
