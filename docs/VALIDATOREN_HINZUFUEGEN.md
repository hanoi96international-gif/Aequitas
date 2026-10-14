# Weitere Validatoren aufnehmen (Betreiber)

Ziel: von einem Validator (C1) auf mindestens vier. Erst ab vier
Komiteemitgliedern bestätigen Checkpoints etwas (`checkpointMindestKomitee`),
und erst ab drei Validatoren wählt die Leitung bei Ausfall selbst einen neuen
Leiter.

## 1. Der neue Validator richtet sich ein

Nach `docs/VALIDATOR_EINRICHTEN.md`. Am Ende ist er ein registrierter Mensch,
sein Signierschlüssel ist an ihn gebunden, und der Knoten läuft als
Beobachter. Er schickt dir seine **Signieradresse** (`0x…`, 40 Hex-Zeichen).

## 2. Neue Liste und ihr Hash

Die Liste sind die Signieradressen **aller** Blockproduzenten, mit Komma
getrennt – **auch die eigene** jedes Validators. Der Hash ist sha256 über die
gültigen, klein geschriebenen, sortierten Adressen ohne Doppelte, mit Komma
verbunden:

```bash
LISTE="0xAAA…,0xBBB…,0xCCC…,0xDDD…"
echo "$LISTE" | tr ',' '\n' | tr 'A-FX' 'a-fx' | sed 's/^ *//;s/ *$//' \
  | grep -E '^0x[0-9a-f]{40}$' | LC_ALL=C sort -u | paste -sd, | tr -d '\n' \
  | sha256sum | cut -d' ' -f1
```

**Maßgeblich ist der Wert des Knotens selbst:** mit der neuen Liste, aber
noch ohne festgeschriebenen Hash gestartet, druckt er `Validatorenliste NICHT
festgeschrieben -- auf jedem Validator AUTHORIZED_VALIDATORS_HASH=… setzen`,
und `/api/status` zeigt ihn als `validator_liste_hash`. Weicht die Formel
davon ab, gilt der Wert des Knotens (ungültige Einträge verwirft er still –
dann die Liste prüfen).

Der Hash schützt gegen Fehlkonfiguration (Tippfehler, vergessene Adresse),
nicht gegen einen böswilligen Validator – den Hash meldet jeder selbst.

## 3. Auf allen Validatoren gleichzeitig setzen

In `/root/.aequitas.env` jedes Validators:

```
AUTHORIZED_VALIDATORS=0xAAA…,0xBBB…,0xCCC…,0xDDD…
AUTHORIZED_VALIDATORS_HASH=<Hash aus Schritt 2>
AEQUITAS_LEITUNG=an
```

Dann neu starten. **Passt die Liste nicht zum Hash, startet der Knoten nicht**
(`FATAL: AUTHORIZED_VALIDATORS … passt nicht`): Tippfehler fallen so beim
Start auf, nicht später als Spaltung. Knoten mit verschiedenen Listen hören
in der Leitung nicht aufeinander.

Nur auf den Validatoren, mit denen eine Kette neu beginnt, zusätzlich
`AEQUITAS_LEITUNG_GENESIS` (siehe Kopf von `leitung_netz.go`). Wer später
dazukommt, braucht es nicht: er meldet sich und wird aufgenommen.

## 4. Prüfen

Auf jedem Validator:

```bash
curl -s http://localhost:8080/api/status | grep -oE '"validator_liste_hash":"[0-9a-f]*"'
curl -s http://localhost:8080/api/health/combined   # Feld "leitung"
```

- `validator_liste_hash` ist überall gleich.
- In der Leitung: alle als Mitglieder im `satz`, `liste_abgewiesen` steigt
  nicht.
- Die Höhe steigt überall (`/api/status`).

## 5. Warten

Eine neue Bindung eines Signierschlüssels wirkt erst nach `erzeugerFrist`
(2 Stunden). Erst danach zählt der neue Validator im Komitee.

## Zurück

Alte Liste und alten Hash wieder auf allen setzen und neu starten. Ein
Validator, den du entfernst, bekommt die neue Liste nicht; er läuft als
Beobachter weiter oder wird abgeschaltet.

## Was noch fehlt (docs/CHECKPOINT_FAHRPLAN.md)

Die Checkpoints sind gebaut, aber noch nicht eingeschaltet. Erst mit vier
Validatoren lässt sich ihr Betrieb erproben.
