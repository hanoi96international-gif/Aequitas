# Impressum und Datenschutzerklärung freischalten

**Stand 28.08.2026.**

Beide Seiten sind fertig geschrieben und antworten trotzdem mit **404**. Das ist kein Fehler: es fehlen die Angaben, die nur der Betreiber kennt.

Ein unvollständiges Impressum ist schlechter als gar keines — es benennt jemanden, der vielleicht gar nicht verantwortlich ist, und erweckt den Anschein, die Pflicht sei erfüllt. Deshalb bleibt die Seite ganz weg, bis sie vollständig ist.

## Was noch fehlt

```bash
curl -s https://aequitas.digital/api/legal-status
```

Antwortet mit den Namen der fehlenden Variablen und dem, was jede bedeutet. Öffentlich und ohne Token — es sind Variablennamen, keine Werte. Wer die Seite sucht und 404 bekommt, soll erfahren können, warum.

## Einsetzen

An den HTML-Dateien wird **nichts** geändert. Die Angaben kommen aus Umgebungsvariablen; auf beiden Boxen dieselben.

```bash
cat >> /root/.aequitas.env <<'ENDE'
LEGAL_NAME=Max Mustermann
LEGAL_STRASSE=Musterstraße 1
LEGAL_ORT=12345 Musterstadt
LEGAL_LAND=Deutschland
LEGAL_EMAIL=kontakt@example.org
LEGAL_VERANTWORTLICH=Max Mustermann
LEGAL_AUFSICHTSBEHOERDE=Der Landesbeauftragte für Datenschutz …
ENDE
chmod 600 /root/.aequitas.env
```

Danach den Knoten neu starten. Die Seiten erscheinen von selbst.

### Die sieben Pflichtangaben

| Variable | Bedeutung |
|---|---|
| `LEGAL_NAME` | Name oder Firma des Betreibers |
| `LEGAL_STRASSE` | Straße und Hausnummer — **ladungsfähig**, kein Postfach |
| `LEGAL_ORT` | PLZ und Ort |
| `LEGAL_LAND` | Land |
| `LEGAL_EMAIL` | E-Mail für Impressum **und** Datenschutzanfragen |
| `LEGAL_VERANTWORTLICH` | verantwortliche Person nach § 18 Abs. 2 MStV |
| `LEGAL_AUFSICHTSBEHOERDE` | zuständige Datenschutzbehörde |

**Die Aufsichtsbehörde richtet sich nach dem Sitz.** In Deutschland ist sie je Bundesland verschieden; das kann niemand für dich bestimmen, ohne die Anschrift zu kennen. Für nichtöffentliche Stellen ist es die Landesbehörde des Sitzes — die Liste steht beim BfDI.

### Optional

`LEGAL_TELEFON`, `LEGAL_VERTRETEN_DURCH`, `LEGAL_REGISTERGERICHT`, `LEGAL_REGISTERNUMMER`, `LEGAL_USTID`.

Was leer bleibt, **verschwindet samt seiner Zeile**. Eine Zeile „Telefon:" ohne Nummer sieht aus wie ein Fehler, und ein leeres Registergericht wirft die Frage auf, ob da eine Gesellschaft steht oder nicht.

## Was die Sperre nicht kann

Sie prüft, ob die Felder **gesetzt** sind — nicht, ob sie **stimmen**. `LEGAL_NAME=xyz` schaltet die Seite frei. Die Sperre schützt vor Vergessen, nicht vor Falschangaben; die Verantwortung für den Inhalt bleibt beim Betreiber.

Ebenso wenig ersetzt sie eine Rechtsprüfung. Die Texte sind sorgfältig geschrieben, aber von niemandem mit Zulassung gegengelesen — was für die biometrische Verarbeitung nach Art. 9 DSGVO eine echte Lücke ist und vor dem Echtbetrieb geschlossen gehört.

## Nachprüfen

```bash
curl -s -o /dev/null -w '%{http_code}\n' https://aequitas.digital/impressum
curl -s -o /dev/null -w '%{http_code}\n' https://aequitas.digital/datenschutz
```

Zweimal `200` heißt: freigeschaltet. Danach die Seiten einmal selbst lesen — ein Tippfehler in der Anschrift fällt nur einem Menschen auf.
