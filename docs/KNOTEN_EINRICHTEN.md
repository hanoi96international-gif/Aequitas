# Einen Aequitas-Knoten betreiben (Validator und Verifier)

**Stand 25.11.2026.** Ein Knotenbetreiber macht beides auf einem Server: Er führt die Kette mit
(**Validator**) und schützt „ein Mensch, ein Konto“ (**Verifier**). Bisher brauchte das zwei Anleitungen
und zwei Durchläufe. Jetzt reicht ein Befehl. Die Einzelanleitungen gelten weiter, wenn du nur einen Teil
willst: `VALIDATOR_EINRICHTEN.md`, `VERIFIER_EINRICHTEN.md`.

## Die Rollen bei Aequitas, kurz

| Rolle | Gerät | Was sie tut |
|---|---|---|
| **Nutzer** | Handy | zahlt, empfängt, ist Inhaberin, Ausgeber oder Annahmestelle eines Tokens — je Vorgang |
| **Präparatorin** | Handy | präpariert Quantengeld (Weg B); braucht ein eigenes IBM-Quantum-Konto, keinen Server |
| **Knotenbetreiber** | Server | Validator + Verifier, mit diesem einen Befehl |
| **zugelassener Knotenbetreiber** | (derselbe Server) | ein Status, kein eigener Job: von zwei Dritteln der zugelassenen Knotenbetreiber aufgenommen (Prüfer-Register); erst dann zählen Bezeugungen und Stimmen |

Warum die Zulassung eine Abstimmung bleibt: Einen Schlüssel zu binden kostet nichts. Würde jeder Knoten
sofort zählen, könnte ein Mensch mit vielen Servern allein ein Quorum stellen (Prüfbefund H1, #310; K2).
Gezählt wird je **Mensch**, nicht je Server.

## Was du brauchst

| | Mindestens (beide zusammen) |
|---|---|
| Server | Ubuntu 22.04 oder 24.04, öffentliche IPv4 |
| Prozessor | **4 dedizierte Kerne** (geteilte mit über 5 % „Steal“ werden abgelehnt) |
| Arbeitsspeicher | **12 GB** (Validator 8 GB, Verifier 4 GB) |
| Festplatte | **80 GB** SSD frei (je 40 GB) |
| Offene Ports | 8080 und 4001 (Validator), 80 und 443 (Verifier, HTTPS) |
| Du selbst | in der Aequitas-App **registriert** |

Kein IBM-Konto: Validator und Verifier präparieren kein Quantengeld (das tun in Weg B die Präparatorinnen).
Deine Wallet, ihre Wörterliste und ihr privater Schlüssel gehören **nicht** auf den Server; dort liegt nur
die Adresse.

## Einrichten: ein Befehl, eine Frage

```bash
curl -fsSL https://get.docker.com | sh
git clone https://github.com/hanoi96international-gif/Aequitas.git
cd Aequitas/deploy/knoten && bash einrichten.sh
```

Das Skript prüft zuerst die Ausstattung (zu schwach: abgelehnt, bevor etwas geladen wird), fragt **einmal**
nach deiner Wallet-Adresse und lässt die IP bestätigen. Dann richtet es nacheinander ein:

1. **Validator**: eigener Schlüssel, Leistungsmessung, Bindung an dich — in der App erscheint eine
   Anfrage mit einer Kontrollzahl; nur wenn sie genau gleich ist, bestätigen. Danach misst das Netz den
   Server noch einmal.
2. **Verifier**: eigene Schlüssel (nie fremde übernehmen), HTTPS über `<IP-mit-Strichen>.sslip.io`,
   Bezeugungsnachweis.

Bricht ein Teil ab, sagt das Skript warum; danach einfach noch einmal starten (vorhandene Einstellungen
bleiben). Der Verifier startet erst, wenn der Validator fertig ist.

## Selbsttest: ist mein Knoten bereit?

```bash
cd Aequitas/deploy/knoten && bash pruefen.sh
```

Prüft nur lesend und sagt in Zeilen mit `ok` / `noch` / `FEHLER`, was steht und was fehlt: Netz
erreichbar, Knoten auf Stand (höchstens 30 Blöcke hinter dem Netz), an dich gebunden, Verifier über
HTTPS erreichbar, Selbstprüfung des Knotens (`/api/wache`). Ergebnis 0 = bereit, 2 = läuft, aber noch
nicht fertig (holt auf oder noch nicht in der App bestätigt), 1 = Fehler.

Von jedem Rechner aus, z. B. für den ersten Betreiber vor seiner Zustimmung: `bash pruefen.sh DEINE-IP`
(dann ohne die Selbstprüfung, die nur auf dem Server selbst abgefragt wird).

## Danach, in der App (Tab „Knoten“)

1. Der Validator läuft als vollwertiger Beobachter mit und prüft jeden Block selbst; die Aufnahme als
   Blockproduzent folgt.
2. Karte „Prüfer“: **Schlüssel freigeben** (Adresse `https://…sslip.io`), dann **Bewerben**. Stimmen zwei
   Drittel der zugelassenen Knotenbetreiber zu, bist du zugelassen.

Sichere beide Dateien: `deploy/validator/.env` und `deploy/verifier/.env`. Gehen sie verloren, ist dein
Knoten ein neuer.
