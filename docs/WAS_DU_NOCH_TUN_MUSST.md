# Was du noch tun musst

**Stand 29.08.2026, 09:00 UTC.** Alles, was ohne dich geht, ist erledigt und
live geprüft. Hier steht nur, was an einem Zugang, einer Unterschrift oder
einer Entscheidung von dir hängt.

Die Reihenfolge ist nach Dringlichkeit für die Beta sortiert.

---

## 1 · Das Verifier-Image öffentlich stellen — **Beta-Blocker**

Der Leitfaden „Run a Verifier" sagt jedem Leser als ersten Schritt:

```
docker pull ghcr.io/hanoi96international-gif/aequitas-biometric-beta/matching:latest
```

Das Paket ist privat. Der Befehl scheitert bei **jedem**, der die Anleitung
befolgt. Damit kann heute niemand außer dir einen Vergleichsdienst betreiben —
und ohne dritten Betreiber gibt es keinen echten Ausfallschutz.

**Geprüft, was dabei veröffentlicht würde:** das Dockerfile kopiert nur
`requirements.txt`, `app/` und `scripts/`. Weder `poh_beta.db` noch
`sample_test_images` noch `models/` landen im Image, und im ausgelieferten
Quelltext stecken keine Zugangsdaten. **Es reicht also, das *Paket* öffentlich
zu stellen — das Repo muss nicht öffentlich werden.**

GitHub bietet dafür keine Programmierschnittstelle; es geht nur über die
Oberfläche:

1. https://github.com/orgs/hanoi96international-gif/packages
2. Paket `aequitas-biometric-beta/matching` öffnen
3. **Package settings** → ganz unten **Change visibility** → **Public**

Danach zur Kontrolle:

```bash
docker pull ghcr.io/hanoi96international-gif/aequitas-biometric-beta/matching:latest
```

---

## 2 · Der Testmodus ist entschieden — und jetzt abgesichert

Du hast entschieden: die Beta startet bewusst mit `SERVICE_MODE=test`.

**Dabei ist ein Geldschöpfungspfad aufgefallen, der jetzt verschlossen ist.**
Die Galerien sind strikt nach Modus getrennt — `sketch_gallery.py` sagt es
selbst, *„eine echte Einschreibung wäre im Testmodus unsichtbar"*. Umgekehrt
gilt dasselbe:

1. Jede Beta-Anmeldung landet nur in der **Test**-Galerie. Die Kette zahlt
   dafür den Registrierungszuschuss.
2. Beim späteren Umstellen auf `real` ist die **echte** Galerie leer.
3. Dieselbe Person meldet sich erneut an → kein Kandidat → `new_enrollment` →
   frisch gewürfelter `bio_hash` (`secrets.randbelow`) → **neuer Nullifier** →
   **zweiter Zuschuss**. Einer je Beta-Teilnehmer.

Der Nullifier-Schutz der Kette greift nicht: er hängt genau an dem Hash, der
neu gewürfelt wurde. Dieselbe Klasse wie die Löschfunktion am 24.08.

**Jetzt live auf beiden Boxen:** ein Tor weist den Realmodus ab, solange die
Realgalerie leer ist *und* Testeinschreibungen bestehen. `/matching/health`
zeigt den Zustand — aktuell `test_galerie: 1`, `real_galerie: 0`,
`wuerde_realmodus_abweisen: true`.

### Wenn du später auf real umstellst

Entscheide **zuerst**, was mit den Testeinschreibungen geschieht — übernehmen
oder verwerfen. Ich kopiere sie bewusst nicht automatisch: sie wurden unter
einer anderen Einwilligungsfassung und bei geschlossener Rechtsschranke
erhoben. Danach auf beiden Boxen:

```
REAL_MODE_START_ACCEPTED="<warum, mit Datum>"
```

Ein leerer Wert genügt nicht. Zusätzlich verlangt der Realmodus weiterhin
`ALLOW_REAL_BIOMETRIC_DATA=true` **und** `LEGAL_SIGNOFF_DATE` — und `gate.py`
sagt selbst, dass dieses Datum erst nach abgeschlossener Phase-2-Rechtsprüfung
(DSGVO Art. 9, Einwilligungsablauf, Löschkonzept) gesetzt werden darf. Das ist
keine Konfiguration, sondern eine Behauptung über eine stattgefundene Prüfung.

---

## 3 · C1 als Validator eintragen — 2 Minuten

Im Register steht heute nur C2 (`0x1a37DcDaa…`). C1s Betreiberadresse ist
`0x0BE8b961CBf6564bd1931B0803D35C0659E0D016`.

Beide Knoten **produzieren** bereits Blöcke (in den letzten 20 genau 10 zu 10),
es fehlt nur der Registereintrag. Seite öffnen, mit **genau dieser** Wallet
verbinden, auf **Connect Wallet & Register** klicken:

```
https://aequitas.digital/node-binding
```

Der Knoten weist seinen eigenen Signierschlüssel selbst nach; du unterschreibst
nur den einen Satz. Das kostet nichts und bewegt nichts — es ist
`personal_sign`, keine Transaktion.

Falls die Seite alt aussieht: **Strg + Umschalt + R**.

---

## 4 · Zwei Secrets für den Proof-Server — 3 Minuten

Der Auslieferungsweg des Proof-Servers hat **nie** funktioniert: das Repo
`aequitas-proof-server` hat gar keine Secrets, also auch keinen SSH-Schlüssel,
und der Lauf bricht mit *„can't connect without a private SSH key or password"*
ab. Ich habe gestern von Hand ausgeliefert und das fehlende Skript
(`deploy/proof-deploy.sh`, mit automatischer Rücknahme) ergänzt — aber ohne die
Schlüssel bleibt es Handbetrieb.

Es sind dieselben Schlüssel, die im Ketten-Repo bereits funktionieren. **Ich
fasse private Schlüssel nicht an**; führe das selbst aus:

```bash
gh secret set CONTABO_SSH_KEY --repo hanoi96international-gif/aequitas-proof-server < ~/.ssh/contabo1
```

```bash
gh secret set CONTABO2_SSH_KEY --repo hanoi96international-gif/aequitas-proof-server < ~/.ssh/contabo2
```

(Dateinamen anpassen, falls deine Schlüssel anders heißen.) Danach zur Probe
einen Lauf auslösen:

```bash
gh workflow run deploy.yml --repo hanoi96international-gif/aequitas-proof-server
```

---

## 5 · Impressum und Datenschutz — deine Entscheidung, bereits umgesetzt

Du hast entschieden, dass es kein Impressum gibt. Das ist sauber umgesetzt:
`/impressum` und `/datenschutz` antworten mit 404, und **keine Seite der
Website verlinkt sie** — es gibt also keinen toten Verweis. Die sieben
`LEGAL_*`-Felder bleiben leer.

Falls du es später doch willst, sagt `https://aequitas.digital/api/legal-status`
genau, welche Felder fehlen und was sie bedeuten.

---

## 6 · Heute 12:09 UTC — Übertrag prüfen (2 Minuten)

Das Tor `POOL_REMAINDER_CARRY_FROM_UNIX` öffnet um **12:09 UTC**. Danach:

```bash
curl -s https://aequitas.digital/api/health/combined | python -c "import sys,json;r=json.load(sys.stdin)['chain']['supply_reconciliation'];print(r['difference'],r['beyond_known_gap'],r['supply_alarm'])"
```

**Erwartet:** `-0.000014 -0.000014 False` — unverändert.

Wichtig: die Differenz darf **nicht** auf 0 zurückgehen. Die 0,000014 AEQ sind
vernichtet, nicht verlegt; eine Rückkehr auf 0 hieße, dass irgendwo neu
geschöpft wurde. Erst wenn der Wert nach dem Tor stabil bleibt, darf die
Grundlinie der Überwachung eingefroren werden.

---

## Was bewusst offen bleibt

- **MPC-Schwelle nicht kalibriert.** Braucht eine zweite Person vor der Kamera;
  ohne echten Doppelversuch lässt sich keine Schwelle belegen.
- **Quorum 2 bei zwei laufenden Vergleichsdiensten.** `/health` sagt das jetzt
  selbst (`bezeuger_bedeutung`) und warnt beim Gleichstand. Echter Ausfallschutz
  braucht einen dritten Betreiber — und der braucht Punkt 1.
