# Testablauf: Ein Mensch = ein Konto

**Stand 13.09.2026, 18:00.** Der biometrische Nachweis ist das Tor
(`BIO_ATTESTATION_MODE=required` auf beiden Proof-Servern — am 13.09. von 01:15
bis 15:13 kurz `optional`, seitdem wieder `required` —, Quorum 2, Vergleichsdienste
proof1 + proof2). Was fehlt, ist der Beleg mit echten Menschen: genau **eine**
Person hat den Weg bisher durchlaufen (`gallery_test: 1`). Dieser Ablauf liefert
den Beleg — oder die Stelle, an der er scheitert.

Braucht: **zwei Personen, zwei Telefone**, 30 Minuten, die App von
https://aequitas.digital/download/app.apk (v1.6.0 registriert über den
Railway-Coordinator; ab v1.7.0 über proof1/proof2 — für Schritt 6 unten ist
v1.7.0 Pflicht).

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

| 6 | **Altkonto nachziehen** (App ≥ v1.7.0): ein vor dem 25.08. registrierter Mensch öffnet den Identity-Tab → „Gesicht zu diesem Konto nachziehen" | durch: `nachgezogen`; **kein** Zuschuss, Kontostand unverändert | `gallery_test` +1 auf proof1 **und** proof2; Coordinator `/health` → `nachziehen.nachgezogen` +1; `total_humans` unverändert |
| 7 | Derselbe Mensch aus 6 registriert sich auf einem anderen Telefon neu (neues Wallet) | **abgewiesen** als Duplikat | wie 2 — das ist der Beleg, dass der Nachzieh-Weg die Altkonten wirklich schließt |

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
  ein zweites Konto samt 1.000 AEQ. Abhilfe seit 13.09.: der Nachzieh-Weg
  (`POST /nachziehen` am Coordinator, App ≥ v1.7.0, Schritt 6) — freiwillig,
  ohne Prägung. Er schließt die Lücke nur für die, die ihn gehen; wer nicht
  nachzieht, bleibt eine bekannte, auf 18 begrenzte Ausnahme (auf der Website
  so benannt).
- `SERVICE_MODE=test`: die Sketches liegen in der Testgalerie. Beim Wechsel
  auf `real` (nach DSGVO-Phase-2) startet die Realgalerie leer — dann gilt
  dasselbe Problem für alle Testteilnehmer (`wuerde_realmodus_abweisen`).
  Der Wechsel braucht eine Übernahme der Testgalerie, nicht nur den Schalter.
