# Biometrie- und Proof-Sicherung (verschlüsselt)

Stand 16.10.2026. Erledigt `docs/BETA_BEREITSCHAFT.md`, Betreiber-Punkt 5, sobald
das Secret gesetzt und die Übung einmal gelaufen ist.

## Was gesichert wird

Workflow `.github/workflows/backup-biometrie-verschluesselt.yml`, täglich 02:43 UTC
und von Hand. Er führt `deploy/backup-biometrie.sh` auf C1 (188.172.229.121) aus.

| Datei im Archiv | Quelle auf C1 | Kopie | Prüfung |
|---|---|---|---|
| `kette-bio.dump` | Postgres des Knotens: `mpc_shares`, `bio_hashes`, `bio_registrations`, `bio_registrations_bio_hash_dedup_backup`, `registration_recovery` | `pg_dump -Fc` | Wiederherstellung in Wegwerf-Postgres |
| `proof.dump` | Container `proof-postgres` (Volume `proof_pgdata`), Datenbank `aequitas_proof` | `pg_dump -Fc` | Wiederherstellung in Wegwerf-Postgres |
| `coordinator/*.db` | `aequitas-coordinator`, `/root/coordinator-daten` (`widersprueche.db`, `einzelbetrieb.db`, `altersbuergschaften.db`) | SQLite-Online-Backup | `PRAGMA integrity_check` |
| `matching/poh_beta.db` | `aequitas-matching`, `/root/aequitas-matching-data` (Template-Galerie) | SQLite-Online-Backup | `PRAGMA integrity_check` |
| `INHALT.txt` | Zeilenzahlen beim Sichern | | |

Alles wird **auf C1** mit `age` an einen öffentlichen Schlüssel verschlüsselt.
Danach wird der Klartext gelöscht, auch bei einem Abbruch (trap). GitHub bekommt nur
`bio-c1.tar.age` (Artefakt, 30 Tage). Auf C1 bleiben die letzten 7 `.age`-Dateien in
`/root/bio-sicherung/archiv`.

Nicht im Archiv:

- Schlüssel aus den `.env`-Dateien, besonders `TEMPLATE_ENCRYPTION_KEY` des
  Vergleichsdienstes (`/root/.aequitas-matching.env`). Ohne ihn sind die Templates in
  `poh_beta.db` nicht lesbar. Verwahre ihn getrennt offline.
- Die Datenbanken werden je für sich konsistent kopiert, aber nicht im selben
  Augenblick. Zwischen den vier Kopien liegen Sekunden.

## Einmalig einrichten (Betreiber, etwa 5 Minuten)

Es ist genau **ein** neues Secret nötig.

1. Auf einem eigenen Rechner, nicht auf C1 und nicht in GitHub:
   ```
   age-keygen -o bio-backup.key
   ```
   (`age` aus dem Paketmanager, z. B. `apt install age` oder `brew install age`.)
2. `bio-backup.key` enthält den **privaten** Schlüssel (`AGE-SECRET-KEY-1…`). Lege ihn
   in den Passwortmanager und zusätzlich offline ab, z. B. auf einem USB-Stick im Safe.
   Er darf nie auf einen Server, in ein Repo oder in ein Secret.
3. Die Zeile `# public key: age1…` in der Datei ist der **öffentliche** Schlüssel.
   Trage ihn ein unter GitHub → Settings → Environments → `produktion` → Environment
   secrets → **`BIO_BACKUP_AGE_RECIPIENT`**. Nur der Wert `age1…`, ohne `# public key:`.
4. Actions → „Biometrie- und Proof-Datenbanken sichern (verschlüsselt)“ → Run workflow,
   dann freigeben. Der Lauf muss grün sein.

Fehlt das Secret oder ist es kein `age1…`-Schlüssel, bricht der Lauf vor dem ersten
Kontakt zu C1 rot ab. Hochgeladen wird dann nichts.

`CONTABO_SSH_KEY` ist schon vorhanden. Mit Pflicht-Freigabe in `produktion` wartet auch
der nächtliche Lauf auf deinen Klick.

## Zurückspielen üben (einmal jetzt, danach vierteljährlich)

Auf einem eigenen Rechner mit Docker, nicht auf C1:

1. Artefakt holen: Actions → letzter grüner Lauf → Artifacts → `biometrie-c1-<id>`.
   Oder: `gh run download <run-id> -n biometrie-c1-<run-id>`.
2. Prüfsumme gegen die Zusammenfassung des Laufs (`age_sha256`):
   ```
   sha256sum bio-c1.tar.age
   ```
3. Entschlüsseln in ein leeres Verzeichnis:
   ```
   mkdir -m 700 uebung && age -d -i bio-backup.key bio-c1.tar.age | tar -C uebung -xf -
   cat uebung/INHALT.txt
   ```
4. Postgres-Teile in eine Testinstanz:
   ```
   docker run -d --name bio-uebung -e POSTGRES_HOST_AUTH_METHOD=trust \
     -v "$PWD/uebung:/in:ro" postgres:16-alpine
   sleep 5
   docker exec bio-uebung createdb -U postgres kette_bio
   docker exec bio-uebung createdb -U postgres proof
   docker exec bio-uebung pg_restore -U postgres --no-owner --no-acl --exit-on-error -d kette_bio /in/kette-bio.dump
   docker exec bio-uebung pg_restore -U postgres --no-owner --no-acl --exit-on-error -d proof /in/proof.dump
   ```
5. Zählen und mit `INHALT.txt` vergleichen:
   ```
   docker exec bio-uebung psql -U postgres -d kette_bio -tAc "SELECT count(*) FROM bio_hashes"
   docker exec bio-uebung psql -U postgres -d kette_bio -tAc "SELECT count(*) FROM mpc_shares"
   docker exec bio-uebung psql -U postgres -d proof -tAc "SELECT count(*) FROM bio_hashes"
   ```
6. SQLite-Teile:
   ```
   for f in uebung/coordinator/*.db uebung/matching/*.db; do
     echo "$f: $(sqlite3 "$f" 'PRAGMA integrity_check')"
   done
   ```
7. Aufräumen. Der Klartext darf nicht liegen bleiben:
   ```
   docker rm -f bio-uebung && rm -rf uebung
   ```
8. Datum und Zählungen der Übung in `docs/BETA_BEREITSCHAFT.md` bei Punkt 5 eintragen.

Für den Ernstfall auf C1 gilt dieselbe Reihenfolge: Dienst stoppen, Datenbank bzw.
Datei zurückspielen, Dienst starten. Die Ketten-Tabellen kommen mit
`pg_restore --data-only` in die laufende Datenbank des Knotens, nachdem
`backup-ledger.yml` den übrigen Zustand wiederhergestellt hat.

## Schlüssel verloren oder kompromittiert

- **Privater Schlüssel verloren:** Die alten Archive sind nicht mehr lesbar. Neues
  Paar erzeugen, Secret ersetzen, sofort einen Lauf starten.
- **Privater Schlüssel kompromittiert:** genauso, zusätzlich alle Artefakte der
  letzten 30 Tage und `/root/bio-sicherung/archiv/*` auf C1 löschen.
