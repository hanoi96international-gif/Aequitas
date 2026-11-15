# Offline-Bargeld – Entwurf

Stand 15.11.2026. Status: **Entwurf**, noch kein Code in der Kette. Jede Stufe unten kommt
als eigener PR mit Missbrauchstests und eigener Sicherheitsprüfung des Diffs
(AGENTS.md: Konsens, Kontostände, Unterschriften).

## Ziel

Mit AEQ bezahlen, **ohne Netz** – von Handy zu Handy, wie Bargeld. Kein Quantengeld (das
braucht Quanten-Hardware, siehe `forschung/quantengeld/HARDWARE_WEG.md`), sondern echtes
Geld mit einer nachrechenbaren Garantie: **Betrug lohnt sich nicht, und Geschädigte werden
entschädigt.**

## Was es von anderen unterscheidet

Offline-Bargeld im Sicherheitschip planen auch Zentralbanken (digitaler Euro, e-CNY). Dort
haftet der Betrüger, weil seine **Bank** ihn kennt. Dezentrale Währungen können das nicht:
jeder macht beliebig viele Konten auf und verschwindet. Aequitas hat **ein Konto je
Mensch** (Gesichtsnachweis). Ein Betrüger kann nicht verschwinden – ohne dass eine Bank
oder ein Staat jede Zahlung sieht.

## Ehrlich: was der Handy-Chip leistet und was nicht

- Der Schlüssel des Geräts (P-256, Android Keystore, StrongBox wo vorhanden) **verlässt den
  Chip nie**. Niemand kann ihn auf ein zweites Gerät kopieren.
- Android gibt Apps **keinen Hardware-Zähler**. Auf einem manipulierten Handy kann der Chip
  trotzdem zweimal unterschreiben. Der Chip **verhindert** Doppelausgaben also nicht; er
  macht nur jede Unterschrift einem Gerät und damit einem Menschen zuordenbar.
- Die Sicherheit kommt aus zwei Dingen: **Betrug ist beweisbar** (Schicht 2) und **der
  Betrüger haftet** (Schicht 3). Die Grenzen unten machen den möglichen Schaden klein.

## Ablauf

### 1. Pfand anlegen (online)

Der Mensch sperrt einen Betrag `A` auf der Kette im **Offline-Topf** (Adresse ohne
Schlüssel, wie der Quantengeld-Topf) und bindet ihn an den öffentlichen Schlüssel `D`
seines Geräts.

- signierte Überweisung (EVM oder PQ) an den Offline-Topf, dazu `D`;
- nur ein registrierter Mensch; **höchstens ein aktives Pfand je Mensch**;
- `1 AEQ ≤ A ≤ 50 AEQ` (`offlinePfandMax`).

### 2. Pfand-Zertifikat (online, vor dem Losgehen)

Die App holt beim Knoten ein Zertifikat: *„Pfand von Gerät D, Mensch M, Betrag A, gültig
bis T“*, unterschrieben mit dem Signierschlüssel eines Erzeugers. `T ≤ jetzt + 72 h`. Der
Empfänger prüft es offline gegen die Erzeugerliste, die seine App beim letzten Kontakt
geladen hat.

### 3. Bezahlen (offline, QR-Code)

Der Zahlende zeigt einen QR-Code: Zertifikat + Zahlung
`(D, Zähler n, Empfänger R, Betrag b, Summe bisher S)`, unterschrieben vom Chip.

Die App des Empfängers prüft offline:

- Zertifikat gültig, Erzeuger bekannt, `T` nicht abgelaufen (Uhr des Empfängers);
- Unterschrift von `D`; `R` ist der Empfänger selbst;
- `b ≤ 20 AEQ` (`offlineZahlungMax`), `S + b ≤ A`;
- `n` ist höher als jeder Zähler, den dieser Empfänger von `D` schon gesehen hat.

### 4. Einlösen (online, beim Empfänger)

Sobald der Empfänger Netz hat, reicht seine App die Zahlung ein (`offline_einloesung`).
Jeder Knoten prüft selbst: Pfand, Unterschrift, `(D, n)` noch nicht eingelöst, Betrag.
Dann bucht die Kette `b` vom Offline-Topf an `R`.

### 5. Kündigen (online)

Der Mensch kündigt sein Pfand. Der Rest kommt erst nach einer **Frist** zurück:
`72 h` (längstes Zertifikat) **+ 14 Tage** (Einlösefrist). So kann jede Zahlung, die ein
gültiges Zertifikat trug, noch eingelöst werden.

## Betrug: beweisen und haften

**Beweis.** Zwei verschiedene Zahlungen mit demselben `(D, n)` – oder Zahlungen, deren
Summe `A` übersteigt – sind ein **unwiderlegbarer Beweis**: nur der Chip von `D` kann sie
unterschrieben haben. Die Kette erkennt das selbst beim Einlösen.

**Folgen, automatisch, auf jedem Knoten gleich:**

1. Das Pfand gilt als verwirkt; das Gerät `D` ist für immer gesperrt, der Mensch `M` darf
   nie wieder offline bezahlen (das Konto selbst bleibt).
2. Jeder Geschädigte bekommt seinen Betrag: zuerst aus dem Pfand, dann vom Konto von `M`.
3. Was dann noch fehlt, ist eine **Schuld von `M`**, die aus seinen künftigen Eingängen
   (auch dem Grundeinkommen) beglichen wird, bevor er selbst darüber verfügt.

**Warum es sich nicht lohnt:** Ein Betrüger gewinnt höchstens, was er doppelt ausgibt –
und schuldet genau das. Er kann kein zweites Konto aufmachen. Er verliert das
Offline-Bezahlen für immer.

## Grenzen (fest, im Konsens)

| Wert | Grenze | Warum |
|---|---|---|
| Pfand | 1–50 AEQ, eines je Mensch | Schaden je Betrüger klein |
| Zahlung | ≤ 20 AEQ | |
| Zertifikat | ≤ 72 h gültig | Kündigung kann nicht überholen |
| Einlösefrist nach Kündigung | 14 Tage | |
| eingelöste Zähler je Pfand | ≤ 10.000 | Speicher begrenzt |
| Weitergabe ohne Netz | **eine Hand** (Version 1) | der Empfänger löst ein, bevor er weitergibt |

## Datenschutz

Eine eingelöste Zahlung ist auf der Kette so sichtbar wie jede Überweisung (von Pfand an
Empfänger) – nicht mehr. Gerät `D` steht beim Pfand, nicht beim Menschen in der Zahlung.
Nicht eingelöste Zahlungen sieht niemand. Kein Server sammelt Offline-Zahlungen.

## Restrisiko (ehrlich)

- Ein Empfänger kann warten müssen: Hat der Betrüger sein Konto leer, kommt der Rest erst
  aus seinen künftigen Eingängen.
- Die Haftung ist nur so stark wie der Gesichtsnachweis. Wer sich trotz allem zweimal
  registriert, hat ein zweites Konto.
- Ein Empfänger, dessen Uhr falsch geht, kann ein abgelaufenes Zertifikat annehmen. Die
  Kette weiß nicht, wann offline unterschrieben wurde (keine Uhr ist dort vertrauenswürdig);
  sie löst jede gültige Zahlung ein, solange das Pfand nicht abgeschlossen ist. Kommt der
  Empfänger danach, greift die Haftung des Zahlenden (Schuld).

## Stufen

1. **Kette:** Offline-Topf, `offline_pfand`, `offline_einloesung`, `offline_kuendigung`,
   `offline_abschluss`, Betrugserkennung, Schuld; Register in StateRoot, Snapshot,
   Rückroll; schlafend hinter einem Stichtag. Prüfvektoren für die Sätze.
2. **Knoten-API:** Zertifikat, Stand, Einreichen.
3. **App:** Geräteschlüssel (Android Keystore, eigenes natives Modul mit Signieren –
   die vorhandene Bibliothek kann nur attestieren), Pfand anlegen, Zertifikat,
   Bezahlen/Empfangen per QR, Einlösen, Kündigen.
4. **Schuld aus dem Grundeinkommen** (berührt die Verteilung – eigene Stufe).
5. Sicherheitsprüfung je Stufe, Stichtag mit Vorlauf, Freigabe durch den Betreiber.
