# Knoten ohne root (seit 22.10.2026)

Hintergrund: `AUDIT_2026-10-21.md` („Container läuft als root“), Code:
`cmd/aequitasd/rechte.go`.

## Was beim ersten Start passiert

Der Container startet weiter als root. Der Knoten übergibt **genau** die
Datenverzeichnisse unter `/data`, die er laut Konfiguration benutzt
(`AEQUITAS_WAL_PATH`, `MPC_TRIPLE_FILE`), dem Benutzer `aequitas`
(UID/GID 10001) und legt die root-Rechte dann ab — bevor er irgendetwas
öffnet oder lauscht. Danach prüft er, ob er ins WAL-Verzeichnis schreiben kann.

`/app` (Binary, `genesis.json`, Seiten) bleibt root: was root vor dem
Ablegen ausführt oder liest, darf der Knotenbenutzer nicht ändern können —
sonst könnte Code mit seinen Rechten `aequitasd` ersetzen, und beim nächsten
Neustart liefe es als root. Geschrieben wird nur unter `/data`; das Image setzt
dafür `AEQUITAS_WAL_PATH=/data/wal/aequitas_transfers.wal` (eine Angabe in der
`.env` geht vor; alle Deployments setzen ohnehin genau diesen Pfad).

Kein Umzug von Hand nötig. Auf dem Host gehören die eingehängten Verzeichnisse
danach UID 10001 (C1/C2: `/root/aequitas-wal-data`, `/root/aequitas-mpc`;
Validatoren: Volume `waldata`). Backups als root auf dem Host lesen sie weiter.

Am 22.10.2026 so geprüft (Laufzeitstufe des Images, statisch gebautes
Binary): Volume mit root-eigenem WAL (0600, Verzeichnis 0700) → nach dem
Start gehört es 10001, der Prozess läuft mit `Uid/Gid 10001`, ohne
Zusatzgruppen. Schreibgeschützt eingehängt → Start bricht mit `✗` ab.

## Wenn er nicht startet

Die Meldung beginnt mit `✗` und nennt das Verzeichnis. Abhilfe auf dem Host:

```sh
chown -R 10001:10001 /root/aequitas-wal-data   # bzw. das genannte Verzeichnis
```

Nur als Notbehelf, wenn das nicht geht: `AEQUITAS_ALS_ROOT=ja` in die `.env`
— der Knoten läuft dann wie früher als root und sagt das bei jedem Start.

## Prüfen

```sh
docker top aequitas-node -eo user,pid,comm   # aequitasd läuft als 10001
```

(`docker exec … id` zeigt root: `exec` startet einen eigenen Prozess mit dem
Benutzer des Images, nicht den des Knotens.)

Dateien, die später als root auf dem Host in diese Verzeichnisse gelegt werden
(etwa neue MPC-Tripel mit 0600), liest der Knoten erst nach dem nächsten
Neustart — der übergibt sie wieder.
