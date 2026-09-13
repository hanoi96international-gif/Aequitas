# Lebendigkeit gegen Deepfakes — Entwurf und Arbeitsplan

**Entscheidung 13.09.2026 (Nutzer): keine Stimme.** Kein zweites biometrisches
Merkmal, keine neue Einwilligung. Die Verteidigung liegt in der Lebendigkeit
und in reversiblen Folgen — nicht in mehr Daten über Menschen.

## Bedrohung, genau benannt

Ein Deepfake greift nicht die Einmaligkeit an, sondern die Lebendigkeit. Ein
synthetisches Gesicht ist per Definition neu; der Gesichtsvergleich lässt es
durch, weil er nur „kennen wir dieses Gesicht?" fragt. Ein Angreifer erzeugt
tausend einmalige Gesichter und spielt sie ein — per Bildschirm vor der Kamera
(Replay) oder als virtuelle Kamera auf einem gerooteten Gerät (Injektion). Jede
durchgelassene Kunstfigur prägt 1.000 AEQ und bezieht danach Grundeinkommen.

Was dagegen hilft, muss beweisen: **jetzt steht ein echter Mensch vor einer
echten Kamera** — und wenn das nicht sicher ist, darf die Folge nicht endgültig
sein (Grundsatz: verzögern, nie aussperren; kein Ausschluss über Gerätequalität
oder Besitz).

## Stand, live gemessen am 13.09.2026

| Baustein | Wo | Stand |
|---|---|---|
| Kopfdreh-Challenge (Coordinator würfelt, App zeigt, Vergleichsdienst prüft) | alle drei | **scharf** (`REQUIRE_CHALLENGE_LIVENESS=true`) |
| Blitz-Lebendigkeit (server-gewählte Farbfolge, Spiegelung im Gesicht) | App zeigt sie bereits; `flash_liveness.py` prüft | gebaut, **aus** |
| Puls (rPPG aus Bildfolge), Puls-Regionenkonsistenz | App liefert Burst; `pulse.py` | gebaut, **aus** |
| Parallaxe / 3D-Konsistenz, IMU-Bewegung | `parallax.py`, `imu_motion.py` | gebaut, **aus** |
| Antispoof (MiniFASNet, v2) | `antispoof*.py` | gebaut, nur informativ |
| Sensor-Fingerabdruck (PRNU), Moiré | `prnu.py`, `moire.py` | gebaut, **aus** |
| Play Integrity / App Attest | Coordinator | gebaut, gates nichts |
| Risiko-Score | `risk.py` | nur Geräte-/Subnetz-Häufung |

Ein Live-Deepfake (Face-Swap auf der Kamera) dreht den Kopf mit. Die einzige
scharfe Prüfung hält ihn nicht auf.

## Die vier Schichten

### 1. Zufällige Herausforderungen in einer Aufnahme

Der Coordinator würfelt je Sitzung eine **Challenge-Menge**: Blitzfarbfolge
(Reihenfolge, Zeitpunkte) + Kopfdrehrichtung + Zeitfenster. Alles in einer
durchgehenden Aufnahme; die Antwort muss innerhalb weniger Sekunden kommen.
Ein Echtzeit-Deepfake müsste ein unbekanntes Farbmuster live und physikalisch
plausibel auf das Gesicht relighten und dabei 3D-konsistent drehen — das ist
die Hürde, die heute fehlt. Replays scheitern an der Farbfolge, flache
Injektionen an der Parallaxe.

**Lebendigkeits-Score** `L ∈ [0,1]` aus: Challenge bestanden, Blitz-Korrelation,
Parallaxe, Puls vorhanden und regionenkonsistent, Antispoof-Konfidenz. Jedes
Signal liefert `checked/passed/konfidenz`; nicht geprüfte Signale zählen
neutral, nie negativ (ein altes Handy ohne Gyroskop darf nicht leiden).

### 2. Herkunft als Risikosignal, nicht als Tor

Fehlender oder inkonsistenter Sensor-Fingerabdruck (virtuelle Kamera),
fehlende Attestierung, Moiré (Bildschirm vor der Kamera), Gerätehäufung —
ergeben `R ∈ [0,1]`. Kein Signal davon sperrt jemanden aus. Es entscheidet
über die Klasse in Schicht 3.

### 3. Reversible Folgen: drei Klassen

| Klasse | Bedingung (erste Schwellen, aus Messdaten zu setzen) | Folge |
|---|---|---|
| **grün** | L hoch, R niedrig | Registrierung + 1.000 AEQ sofort, wie heute |
| **gelb** | L mittel oder R erhöht | Registrierung sofort; 200 AEQ sofort, 800 AEQ als **Staffel über 30 Tage**, die nur läuft, wenn nach 7 Tagen eine **zweite Lebendigkeitsprüfung** (neue Challenge-Menge) bestanden wurde. Bleibt sie aus, pausiert die Staffel — sie läuft weiter, sobald sie kommt. |
| **rot** | L niedrig | keine Prägung; Zweitprüfung an einem anderen Tag oder Widerspruch. Kein Ausschluss. |

Damit lohnt sich keine Deepfake-Farm mehr: jede Kunstfigur müsste wiederholt
zufällige Prüfungen bestehen, um überhaupt an den Großteil des Zuschusses zu
kommen — und ein echter Mensch, der einmal in schlechtem Licht saß, verliert
nichts außer Zeit.

Die Klasse steht **in der Attestierung des Coordinators** (signiert), nie in
der Hand der App: `grant_class ∈ {sofort, gestaffelt}` wandert als Feld in
`register_human`; die Kette prägt danach. Der Proof-Server reicht das Feld
durch und prüft die Signatur wie heute.

### 4. Später, optional: zweites Dedup-Merkmal ohne neuen Sensor

Periokular-/Sklera-Sketch aus derselben Aufnahme (`periocular.py`,
`sclera.py`) als zweiter Vergleich. Hilft gegen Doppelanmeldung echter
Menschen, nicht gegen Deepfakes — deshalb nachrangig. Braucht eigene
DSGVO-Bewertung (weiteres Merkmal nach Art. 9).

## Was die Kette dafür braucht (dieses Repo)

- `register_human` mit `grant_class`; bei `gestaffelt`: Konto-Felder
  `grant_gestaffelt_rest` (Mikro-AEQ), `grant_staffel_bis` (Unix),
  `lebendigkeit_erneuert_am` (Unix). **Alle drei in `accountLeaf`** (StateRoot)
  und im Snapshot-Export/-Import, sonst laufen die Validatoren auseinander.
- Neue Transaktion `liveness_renewal` (Attestierung des Coordinators, wallet,
  Zeitpunkt) — deterministisch replaybar, idempotent.
- Tägliche Freigabe der Staffel im selben Lauf wie das Grundeinkommen
  (deterministisch nach Blockzeit, nicht nach Wanduhr).
- Tests: Replay-Determinismus, Snapshot-Roundtrip, Staffel pausiert/läuft.

## Arbeitspakete

| WP | Inhalt | Repo | Aufwand |
|---|---|---|---|
| 1 | Lebendigkeits-Score + Klasse im Coordinator, **Schattenmodus** (rechnen, loggen, zählen, nichts ändern) — **✅ 13.09.2026** (`coordinator/app/lebendigkeit.py`, live auf proof1+proof2: `/health → lebendigkeit` mit Klassenzählern + letzten 50 Bewertungen ohne Kennung; `RegisterResponse.lebendigkeit` informativ, `verbindlich: false`). Erste Schwellen: grün L ≥ 0,70 ∧ R < 0,30, rot L < 0,40 | biometric-beta | erledigt |
| 2 | Gestaffelter Zuschuss + `liveness_renewal` auf der Kette, mit Tests — **✅ 14.09.2026, schlafend** (`x/humanity/keeper/grant_staffel.go`, Aktivierung `stagedGrantActivationUnix` = 2100 bis WP 4; Proof-Server reicht die signierte Klasse durch, Coordinator signiert sie nur mit `LEBENDIGKEIT_VERBINDLICH=true`, App reicht sie weiter — alle drei Schalter aus). Live geprüft: `account_set_xor` beider Boxen vor und nach dem Deploy byte-gleich | aequitas-chain, proof-server, biometric-beta, app | erledigt |
| 3 | Zweite Lebendigkeitsprüfung in der App (Tag 7), Anzeige der Staffel (`/api/balance → staffel`), 12 Sprachen; Coordinator-Endpunkt, der die Erneuerung signiert (`aequitas-liveness-renewal-v1|wallet|issued_at`) und die App an `/api/liveness-renewal` weiterreicht | aequitas-app, biometric-beta | 1–2 Tage |
| 4 | Schwellen aus echten Aufnahmen setzen, dann Klassen scharf schalten | biometric-beta | nach ≥ 20 echten Registrierungen |

**Scharfschalten (nach WP 4), in dieser Reihenfolge:** (1) `stagedGrantActivationUnix` auf ein Datum in der
Zukunft setzen und beide Knoten ausrollen; (2) am Tag danach `LEBENDIGKEIT_VERBINDLICH=true` an beiden
Coordinatoren — ab da tragen gelbe Registrierungen die Klasse; (3) App mit Zweitprüfung (WP 3) muss vorher
draußen sein, sonst kann niemand seine Staffel starten.

Reihenfolge: 1 → 2 → 3 → 4. Nichts davon geht scharf, bevor der
Zwei-Personen-Test (`DOPPELREGISTRIERUNG_TEST.md`) die ersten Messwerte für
alle Signale geliefert hat — er ist zugleich der Messlauf für WP 1: nach jeder
echten Registrierung steht die Bewertung unter
`https://proof1.aequitas.digital/coordinator/health` → `lebendigkeit.letzte`
(L, R, Klasse, je Signal der gemittelte Wert). Erwartung für echte Menschen
bei gutem Licht: grün. Alles andere ist ein Kalibrierpunkt, kein Ausschluss.

## Was bewusst nicht kommt

Stimme (Entscheidung 13.09.), Ausweise, Geräte-Ausschluss, Videos speichern.
Die Aufnahme bleibt im Arbeitsspeicher; gespeichert wird weiterhin nur der
Sketch, die Klasse und die Zeitpunkte.
