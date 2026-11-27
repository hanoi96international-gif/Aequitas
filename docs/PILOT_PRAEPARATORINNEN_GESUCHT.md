# Gesucht: Menschen, die mit ihrem Handy Quantengeld präparieren (Pilot Weg B)

**Stand 27.11.2026.** Teil 1 ist zum Weitergeben gedacht (Nachricht, Forum, Freundeskreis).
Teil 2 ist die Checkliste des Betreibers. Die Anleitung für den Ablauf selbst steht in
`docs/PILOT_WEG_B.md`.

---

## Teil 1 — Einladung (zum Weitergeben)

Aequitas ist ein Geld, bei dem jeder Mensch genau ein Konto hat und jeden Tag denselben Anteil
bekommt. Dazu gehört ein Versuch mit **Quantengeld**: Token, deren Geheimnis auf einem echten
Quantencomputer entsteht.

**Damit kein Einzelner das Geheimnis kennt**, präparieren es viele Menschen gemeinsam. Für jede
Ausgabe lost die Kette mit öffentlichem Zufall (drand) aus allen Gemeldeten einige aus. Jede
Ausgeloste präpariert ihren Teil des Tokens auf **ihrem eigenen** IBM-Quantenzugang. Fälschen
könnte nur, wer genug Ausgeloste auf seiner Seite hat. **Wir brauchen dafür mindestens 21 Menschen**
(für die Schwelle 4 von 7; für einen kleineren ersten Lauf reichen 12).

**Was du brauchst**

| | |
|---|---|
| Handy | die Aequitas-App, in der Wallet als Mensch registriert (Gesichtsprüfung) |
| IBM-Quantum-Konto | kostenlos bei IBM; der freie Plan hat 10 Minuten Rechenzeit je 28 Tage. Du brauchst daraus den API-Schlüssel und die Instanz (CRN). |
| Zeit | einmal 10 Minuten zum Melden; dann zu verabredeten Terminen eine Stunde die Quantengeld-Seite offen haben |
| Geld | nichts. Du zahlst nichts ein und haftest nicht. |

**So machst du mit**

1. In der App: **Quantengeld** → „Als Präparatorin melden“.
2. In derselben Karte den IBM-API-Schlüssel und die CRN eingeben → „Zugang sichern“. Der Schlüssel
   bleibt nur auf deinem Handy und wird nie wieder angezeigt.
3. Ab dem nächsten Tag kannst du ausgelost werden. Zu verabredeten Terminen: Seite öffnen und
   „Automatisch weiter, solange diese Seite offen ist“ tippen. Den Rest macht die App.

**Ehrlich gesagt**

- Das ist ein **Pilot**. Er ist mit Tests geprüft, aber noch nie mit vielen Menschen auf echten
  Handys gelaufen. Der erste Lauf ist auch eine Messung (wie lange eine Runde dauert, wie viel
  IBM-Rechenzeit eine Messung braucht).
- So entsteht Quantengeld, das **niemand allein fälschen kann**, auch nicht der Betreiber. Es ist
  aber noch nicht „durch Physik unkopierbar“: Nach der Messung ist der Token gewöhnliche Daten.
  Das Ziel dahinter, Photonen, die die Inhaberin selbst misst, braucht Geräte (Weg C).
- Zurzeit läuft nur ein Knoten. Im Pilot vertrauen alle diesem einen Knoten, was Finalität und
  Token betrifft.

---

**English, short:** Aequitas is a money where every human has exactly one account and gets the same
daily share. For its quantum-money pilot we need **at least 21 people** (12 for a smaller first run)
who prepare token secrets on their **own** free IBM Quantum account from their phone. Each issuance
draws a few of them by public randomness (drand); forging would need enough of the drawn people on
one side. You need the Aequitas app (registered as a human), a free IBM Quantum account
(API key and instance CRN), 10 minutes to sign up, and an hour with the app open at agreed times.
You pay nothing and are not liable. In the app: Quantum money → “Register as preparer”, then enter
your IBM key and CRN. It is a pilot: tested, but never run with many people on real phones.

---

## Teil 2 — Checkliste des Betreibers

1. **Wer mitmacht, ist ein eigener Mensch** mit eigener Gesichtsprüfung, eigenem Handy und eigenem
   IBM-Konto. Ein Mensch mit mehreren Handys zählt nicht mehrfach (die Kette lost je Mensch).
2. **Zählen:** Die App zeigt beim Bestellen, wie viele im Los wären und wie viele für das gewählte v
   nötig sind (3·v, der Ausgeber zählt nicht mit). Erst bestellen, wenn genug gemeldet sind — sonst
   weist die Kette ab („Register zu klein“).
3. **Einen Termin verabreden** (Messenger-Gruppe): eine Stunde, in der alle die Quantengeld-Seite
   offen haben. Ohne offene Seite läuft keine Automatik.
4. **Ausgeber, Inhaberin und zwei Annahmestellen festlegen:** vier verschiedene Konten. Der Ausgeber
   braucht AEQ für Deckung und Pfand (Pfand = 10 % der Deckung, mindestens 1 AEQ); die Inhaberin
   braucht App 1.32.0 oder neuer (Pfand der Bindung).
5. **Ablauf** nach `docs/PILOT_WEG_B.md`, Abschnitt „Ablauf“. Hängt etwas: Abschnitt „Wenn etwas
   hängt“.
6. **Danach festhalten** (`docs/SITZUNG_UEBERGABE.md`): Zahl der Gemeldeten, Dauer der Runde,
   IBM-Rechenzeit je Messung, Fehler je Treffer, ob eingelöst wurde. Das sind die ersten echten
   Zahlen für Weg B.

**Wo du Leute findest (Vorschläge):** wer schon in der App registriert ist; Quanten- und
Physik-Gruppen an Hochschulen (IBM Quantum ist dort verbreitet); Foren zu Grundeinkommen. Wichtig:
jede Person mit eigenem IBM-Konto, keine geteilten Schlüssel.
