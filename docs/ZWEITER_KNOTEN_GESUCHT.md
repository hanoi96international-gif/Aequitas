# Gesucht: der zweite Mensch, der einen Aequitas-Knoten betreibt

**Stand 28.11.2026.** Teil 1 ist zum Weitergeben gedacht (Nachricht, Forum, Freundeskreis).
Teil 2 ist die Checkliste des Betreibers für den Moment, in dem jemand zusagt.

---

## Teil 1 — Einladung (zum Weitergeben)

Aequitas ist ein Geld, bei dem jeder Mensch genau ein Konto hat und jeden Tag denselben Anteil
bekommt. Damit kein Einzelner das Netz beherrscht, braucht es mehrere Menschen, die unabhängig
voneinander einen Knoten betreiben. Heute läuft genau einer. **Wir suchen den zweiten.**

**Was dein Knoten tut**

- **Validator:** Er führt die Kette mit und prüft jeden Block selbst — jede Signatur, jeden
  Kontostand. Er glaubt niemandem, auch nicht dem ersten Knoten.
- **Verifier (Prüfer):** Er schützt „ein Mensch, ein Konto“. Sobald du zugelassen bist, kann der
  erste Betreiber allein keine neuen Menschen mehr eintragen: Jede Registrierung braucht dann die
  Unterschriften von zwei verschiedenen Menschen. Diese Regel steht schon in jedem Knoten und schaltet
  sich mit dem zweiten Prüfer von selbst scharf.

**Was du brauchst**

| | Mindestens |
|---|---|
| Server | gemietet (VPS/Root-Server), Ubuntu 22.04 oder 24.04, öffentliche IPv4 — etwa 10–20 € im Monat |
| Prozessor | 4 **dedizierte** Kerne (keine „geteilten vCPU“; das Skript misst das und lehnt ab) |
| Arbeitsspeicher / Platte | 12 GB / 80 GB SSD |
| Ports | 8080, 4001, 80, 443 offen |
| Du | in der Aequitas-App registriert (Gesichtsprüfung abgeschlossen) |
| Zeit | ca. 30 Minuten zum Einrichten, danach ein Update ab und zu |

Kein IBM-Konto, keine Programmierkenntnisse. Deine Wallet und ihre Wörterliste bleiben auf deinem
Handy; auf dem Server liegt nur ihre **Adresse**.

**Einrichten: ein Befehl**

```bash
ssh root@DEINE-SERVER-IP
curl -fsSL https://get.docker.com | sh
git clone https://github.com/hanoi96international-gif/Aequitas.git
cd Aequitas/deploy/knoten && bash einrichten.sh
```

Das Skript fragt nach deiner Wallet-Adresse und zeigt am Ende eine Kontrollzahl. In der App, Tab
**„Knoten“**, bestätigst du nur, wenn die Zahl genau gleich ist. Ob alles steht, sagt dir danach
`bash pruefen.sh` (im selben Ordner). Dann, in der App, Karte **„Prüfer“**: **Schlüssel freigeben**,
dann **Bewerben**. Alles Weitere: `docs/KNOTEN_EINRICHTEN.md`.

**Was du bekommst — ehrlich:** einen gleichen Anteil am täglichen Validator-Topf (je Mensch, nicht je
Server). Und mit dir gilt: **kein Einzelner — auch nicht der Betreiber — kann allein Menschen
erfinden.** (Zwei Prüfer im Bund könnten es; deshalb sollen später viele dazukommen.)

**Was du zusagst:** den Server laufen lassen (fällt er aus, können sich neue Menschen nicht mehr
registrieren, bis er wieder läuft) und Updates einspielen. Aufhören geht jederzeit — sag vorher
Bescheid, damit du sauber aus dem Prüfer-Register austrittst (danach gilt wieder „einer reicht“).

---

**English, short:** Aequitas is a money where every human has exactly one account and gets the same
daily share. One node runs today; we are looking for the **second independent human** to run one
(validator + verifier). You need a rented Ubuntu server (4 dedicated cores, 12 GB RAM, 80 GB SSD,
ports 8080/4001/80/443, about €10–20/month) and to be registered in the Aequitas app. Setup is one
command (above) plus confirming a six-digit code in the app (tab “Node”), then “Prüfer” → release key →
apply. Your wallet and seed phrase never go on the server. Full guide: `docs/KNOTEN_EINRICHTEN.md`
(PDF in 12 languages: `downloads/Aequitas_Node_Guide_*.pdf`).

---

## Teil 2 — Checkliste des Betreibers (wenn jemand zusagt)

**Reihenfolge ist wichtig.** Ab dem Moment, in dem das Prüfer-Register einen zweiten Menschen hat,
weist jeder Knoten Registrierungen ab, die nur deine Genesis-Schlüssel tragen (Regel „Der Betreiber
allein erschafft keine Menschen“, aktiv seit 27.11.2026 12:00 UTC). Läuft sein Dienst dann nicht,
stehen Registrierungen still (fail-closed, gewollt).

1. **Er ist ein anderer Mensch.** Registriert mit eigener Gesichtsprüfung, eigenes Handy, eigener
   Server bei einem Anbieter seiner Wahl. Nicht du für ihn — sonst ist es kein zweiter Mensch.
2. **Er richtet ein** (`deploy/knoten/einrichten.sh`). Ergebnis: Validator läuft als Beobachter und
   holt auf; Verifier antwortet unter `https://<IP-mit-Strichen>.sslip.io`.
3. **Prüfen, bevor du zustimmst:**
   - `bash deploy/knoten/pruefen.sh SEINE-IP` (von jedem Rechner mit dem Repo) muss mit „Bereit.“
     enden: Knoten auf Stand, an ihn gebunden, Verifier über HTTPS erreichbar.
   - In der App (Karte „Prüfer“) ist sein Schlüssel freigegeben und seine Bewerbung sichtbar.
4. **In der App zustimmen** (Karte „Prüfer“). Den ersten Prüfer neben dir nimmst du als Gründer
   allein auf — öffentlich auf der Kette. Ab dem dritten braucht jede Aufnahme zwei Drittel.
5. **Gleich danach testen:** eine Registrierung (Testperson). Sie muss **zwei** Bescheinigungen tragen
   und durchgehen. Hakt es: er prüft seinen Verifier (`docker compose logs` im Ordner
   `deploy/verifier`).
6. **Als Blockproduzent aufnehmen** (getrennt, kann später kommen): `docs/VALIDATOREN_HINZUFUEGEN.md`
   — Liste und Hash auf beiden Validatoren gleichzeitig setzen. Mit zwei Produzenten gibt es noch
   keine Checkpoints (erst ab vier), aber die Kette hängt nicht mehr an einem Server.
7. **Diese Übergabe nachziehen** (`docs/SITZUNG_UEBERGABE.md`): wer, seit wann, Signieradresse,
   Verifier-Adresse.

**Wo du Leute findest (Vorschläge):** Menschen, die schon in der App registriert sind und einen
Server betreiben können; lokale Linux-/Hackspace-Gruppen; Foren zu Grundeinkommen und
Kryptogeld mit Fokus auf Dezentralität. Wichtig: kein Freundschaftsdienst „auf deinem Server“ —
eigener Server, eigene Schlüssel.
