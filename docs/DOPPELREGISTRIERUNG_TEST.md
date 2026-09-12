# Testablauf: Ein Mensch = ein Konto

**Stand 12.09.2026.** Der biometrische Nachweis ist seit dem 25.08. ein Tor
(`BIO_ATTESTATION_MODE=required` auf beiden Proof-Servern, Quorum 2 von drei
Vergleichsdiensten). Was fehlt, ist der Beleg mit echten Menschen: genau **eine**
Person hat den Weg bisher durchlaufen (`gallery_test: 1`). Dieser Ablauf liefert
den Beleg — oder die Stelle, an der er scheitert.

Braucht: **zwei Personen, zwei Telefone**, 30 Minuten, die App v1.6.0 von
https://aequitas.digital/download/app.apk.

## Vorher ablesen (Nullpunkt)

```bash
curl -s https://coordinator-production-e067.up.railway.app/inventory | python3 -m json.tool | head -40
curl -s https://proof1.aequitas.digital/matching/health
curl -s https://proof2.aequitas.digital/matching/health
curl -s https://proof1.aequitas.digital/health
curl -s https://aequitas.digital/api/status | grep -oE '"total_humans":[0-9]+'
```

Notieren: `gallery_test` je Vergleichsdienst, `bio_hash_count`, `total_humans`.

## Ablauf

| # | Schritt | Erwartung | Woran man es sieht |
|---|---|---|---|
| 1 | Person A registriert sich auf Telefon 1 | durch: Wallet erhält 1.000 AEQ | `total_humans` +1, `gallery_test` +1 auf allen erreichbaren Vergleichsdiensten, `bio_hash_count` +1 |
| 2 | **Person A** registriert sich erneut, auf **Telefon 2** (neues Wallet) | **abgewiesen** als Duplikat | App zeigt Ablehnung; Coordinator-Log `duplicate`; `total_humans` unverändert; Zähler erkannter Duplikatsversuche +1 |
| 3 | Person A löscht die App auf Telefon 1 und installiert neu (neues Gerätegeheimnis, neues Wallet) | **abgewiesen** — das Gesicht ist bekannt | wie 2 |
| 4 | Person B registriert sich auf Telefon 2 | durch | `total_humans` +1 |
| 5 | Person A mit Brille / anderem Licht / anderem Abstand (falls 1 ohne Brille war) auf Telefon 2 | **abgewiesen** | wie 2 |

Jeder Schritt, der anders ausgeht als erwartet, ist der Befund. Nicht weiter
testen, sondern die Antwort des Coordinators (JSON) und die Logzeile sichern:

```bash
gh workflow run container-stand.yml --ref main -f container=aequitas-matching -f zeilen=60
```

## Was das Ergebnis bedeutet

- **2, 3, 5 abgewiesen, 1 und 4 durch:** das Tor hält. Dann Schwelle mit den
  gemessenen Ähnlichkeiten (Coordinator-Antwort `score`) festhalten — das ist
  der erste Kalibrierpunkt.
- **2 oder 3 durch:** die Erkennung trifft die gleiche Person nicht — Schwelle
  zu streng oder Kandidatensuche (LSH) findet den Eintrag nicht. Dann ist
  „ein Mensch = ein Konto" nicht gegeben, und das Netz darf nicht öffentlich
  registrieren, bis es behoben ist.
- **4 abgewiesen:** Schwelle zu locker — ein anderer Mensch gilt als Duplikat.
  Das sperrt echte Menschen aus und ist genauso ein Blocker.

## Was dieser Test NICHT klärt

- Die 18 vor dem 25.08. registrierten Menschen haben **keinen Sketch** in den
  Vergleichsdiensten. Jeder von ihnen käme heute als „neu" durch und erhielte
  ein zweites Konto samt 1.000 AEQ. Abhilfe: sie einmal freiwillig durch das
  Tor schicken (Registrierung mit dem bestehenden Wallet nachziehen) — dafür
  braucht es einen Weg im Coordinator, der ein bestehendes Wallet an einen
  Sketch bindet, ohne neu zu prägen. Den gibt es noch nicht.
- `SERVICE_MODE=test`: die Sketches liegen in der Testgalerie. Beim Wechsel
  auf `real` (nach DSGVO-Phase-2) startet die Realgalerie leer — dann gilt
  dasselbe Problem für alle Testteilnehmer (`wuerde_realmodus_abweisen`).
  Der Wechsel braucht eine Übernahme der Testgalerie, nicht nur den Schalter.
