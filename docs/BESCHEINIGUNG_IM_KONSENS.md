# Bescheinigung im Konsens einschalten

**Stand 22.10.2026 — gebaut, schlafend.** Hintergrund: `AUDIT_2026-10-21.md`, K1.

## Worum es geht

Bisher prüfte nur der annehmende Knoten (über seinen Proof-Server), dass eine
Registrierung durch die Gesichtsprüfung ging. Jeder andere Knoten sah nur einen
gültigen Groth16-Beweis — und den kann jeder erzeugen, der Blöcke erzeugt: der
Beweisschlüssel ist öffentlich, `bio` eine Zufallszahl. Ein Erzeuger konnte so
beliebig viele „Menschen“ anlegen.

Ab dem Stichtag trägt jede `register_human` die Bescheinigung (Zeitpunkt,
Modus, Ed25519-Unterschriften von Coordinator und Vergleichsdienst), und
**jeder** Knoten prüft selbst: Unterschriften von Schlüsseln aus der Genesis
über den **Nullifier des Beweises**, Bindung an genau diese Wallet, Quorum
(`x/humanity/keeper/bio_bescheinigung_konsens.go`).

**`bio` kommt in keinen Block.** Die erste Fassung trug `bio` im Block — das
ist die `bio_hash`, mit der ein Mensch beim Coordinator seine Einschreibung
abfragt (`/access`) und löscht (`DELETE /enrollment`). Öffentlich hätte jeder
jede Einschreibung löschen können, und der Gelöschte wäre als Fremder ein
zweites Mal durchgekommen. Darum unterschreiben Coordinator und
Vergleichsdienst eine eigene Form v3 über den Nullifier (der ohnehin
öffentlich im Beweis steht):

```
aequitas-bio-attestation-v3|<modus>|<nullifier>|<wallet>|<zeit>   Coordinator
aequitas-bio-personhood-v3|<modus>|<nullifier>|<zeit>             Vergleichsdienst
```

Sie reisen im selben Feld wie v2 (`bioAttestation`, Präfix `v3:`); der Knoten
nimmt sie heraus, bevor er die Anfrage an den Proof-Server weiterreicht. App
und Proof-Server bleiben unverändert.

## Einschalten (Betreiber)

0. Reihenfolge, sonst scheitern Registrierungen:
   1. **Alle Knoten** auf diese Fassung (sie nehmen `v3:` heraus). Ein alter
      Knoten reichte `v3:…` an den Proof-Server weiter, und der wiese ab.
   2. **Vergleichsdienste** neu ausrollen (sie liefern
      `personhood_signature_v3`).
   3. Erst dann im **Coordinator** `BESCHEINIGUNG_V3=ja` setzen.
   4. Prüfen: eine Test-Registrierung; `/api/status` des Knotens zeigt die
      Bescheinigung noch als schlafend, die Registrierung geht trotzdem durch.

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
   Quorum, die Zahl der Schlüssel und einen `fingerabdruck`. Der
   Fingerabdruck muss auf **allen** Knoten gleich sein — sonst haben sie
   verschiedene Schlüssellisten und spalten sich am Stichtag.
5. Jeder Schlüssel, den der Proof-Server annimmt, muss in der Genesis stehen.
   Unterschreibt ein Coordinator mit einem Schlüssel, der dort fehlt, werden
   ab dem Stichtag alle seine Registrierungen abgewiesen (fail-closed).

Ein Schlüsselwechsel (neuer Coordinator, zweiter Vergleichsdienst) ist eine
neue Genesis-Fassung mit neuem Stichtag — auf allen Knoten zugleich.

Ein Knoten mit unlesbarer `bescheinigung` (Tippfehler, Schlüssel kleiner
Ordnung, Quorum größer als die Zahl der Schlüssel), mit einem falsch
geschriebenen Eintragsnamen (`Bescheinigung`, `bescheinigungen`) oder ohne
`genesis.json` startet nicht.

**Alte Registrierungen.** Eine Registrierung, die vor `ab_unix - 1 Tag`
angenommen wurde, trägt keine Bescheinigung. Sie muss in einen Block vor dem
Stichtag; die Erzeugung legt die Blockzeit dafür so früh wie möglich. Steht
die Erzeugung über den Stichtag hinweg länger als einen Tag, passt keine
Blockzeit mehr — dann entsteht kein Block (statt eines, den alle abweisen),
und es gilt die übliche Abhilfe: Resync vom Seed, Ausgang verwerfen. Die
Wiederholung hängengebliebener Registrierungen (`registration_recovery`)
bucht ab dem Vorlauf keine ohne gültige Bescheinigung mehr ein; die Zeile
bleibt offen und sichtbar, der Mensch registriert sich neu.
