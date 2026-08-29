# Was du noch tun musst

**Stand 29.08.2026, 09:00 UTC.** Alles, was ohne dich geht, ist erledigt und
live geprüft. Hier steht nur, was an einem Zugang, einer Unterschrift oder
einer Entscheidung von dir hängt.

Die Reihenfolge ist nach Dringlichkeit für die Beta sortiert.

---

## 1 · Verifier-Image — **erledigt am 29.08.**

Das Paket `aequitas-biometric-beta/matching` ist öffentlich. Nachgeprüft: das
Manifest lässt sich **ohne jede Anmeldung** abrufen (`200`). Damit ist der
Leitfaden „Run a Verifier" zum ersten Mal für Fremde ausführbar — und der Weg
zu einem dritten Betreiber offen.

Geprüft war vorher, dass dabei nichts mitgeht: das Dockerfile kopiert nur
`requirements.txt`, `app/` und `scripts/`; `poh_beta.db`, `sample_test_images`
und `models/` bleiben draußen, und im ausgelieferten Quelltext stecken keine
Zugangsdaten.

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

## 3 · C1 als Validator — **erledigt am 29.08.**

Das Register trägt jetzt **beide** Knoten, jeweils mit Bezeugungsschlüssel und
Vergleichsdienst-Adresse:

| Betreiber-Wallet | Bezeuger | Vergleichsdienst |
|---|---|---|
| `0x0be8b961…` (C1) | `222ad549…` | `https://proof1.aequitas.digital/matching` |
| `0x1a37dcda…` (C2) | `207b2894…` | `https://proof2.aequitas.digital/matching` |

---

## 4 · Proof-Server-Auslieferung — **erledigt am 29.08.**

Du hast `CONTABO_SSH_KEY` und `CONTABO2_SSH_KEY` gesetzt. Der erste Lauf ist
durchgelaufen: **success**. Damit funktioniert der Auslieferungsweg des
Proof-Servers zum ersten Mal überhaupt — vorher fehlten sowohl die Secrets als
auch das aufgerufene Skript.

Ab jetzt genügt:

```bash
gh workflow run deploy.yml --repo hanoi96international-gif/aequitas-proof-server
```

Das Skript (`deploy/proof-deploy.sh`) sichert vorher die Konfiguration des
laufenden Containers, baut unter einem neuen Tag und nimmt automatisch zurück,
wenn `/health` danach nicht antwortet.

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

## Am 29.08. nachmittags noch geschlossen

**Die letzte handgepflegte Liste ist weg.** Die MPC-Mitgliedschaft lief über ein
statisches `MPC_PEERS` auf beiden Boxen — ein dritter Betreiber hätte dort von
Hand eingetragen werden müssen. Beide laufen jetzt im Entdeckungsmodus und
melden es selbst:

> `[MPC] serving /mpc/exchange as 0x…; committee of 2 drawn from the chain,
> membership resolved per registration`

Vorher geprüft, dass das nicht MPC still abschaltet: der damalige Grund für die
feste Liste war, dass `mpc_ready` nie **gesendet** wurde. Das ist behoben
(`sync_blocks.go` sendet es, die Gegenseite speichert es über
`RegisterWithMPC`). `MPC_PEERS` und `MPC_PARTY_INDEX` sind aus dem Prozess
**und** aus `.aequitas.env` verschwunden.

**SSH: Passwort-Login ist auf beiden Boxen zu.** Vorher nahmen beide
Root-Login per Passwort aus dem offenen Netz an — bei 69.026 bzw. 62.135
Rateversuchen von je rund tausend IPs, laufend, ohne fail2ban. Das war der
einzige reale Weg an den Wallet-Schlüssel; über die Anwendung ist er nicht
erreichbar (kein Endpunkt gibt die Umgebung aus, nie geloggt, nicht im Git,
env-Datei `600 root`, kein Container mit Docker-Socket).

Die Falle dabei: `/etc/ssh/sshd_config.d/50-cloud-init.conf` machte das
Passwort wieder auf. Nur die Hauptdatei zu ändern hätte **nichts** bewirkt.

---

## Was bewusst offen bleibt

- **MPC-Schwelle nicht kalibriert.** Braucht eine zweite Person vor der Kamera;
  ohne echten Doppelversuch lässt sich keine Schwelle belegen.
- **Quorum 2 bei zwei laufenden Vergleichsdiensten.** `/health` sagt das jetzt
  selbst (`bezeuger_bedeutung`) und warnt beim Gleichstand. Echter Ausfallschutz
  braucht einen dritten Betreiber — und der braucht Punkt 1.

---

## Aufgefallen: beide Boxen tragen den Wallet-Schlüssel ihres Betreibers

Auf C1 **und** C2 ist die Signieradresse des Knotens identisch mit
`NODE_OPERATOR_WALLET`:

| Box | Signieradresse = Betreiber-Wallet |
|-----|-----------------------------------|
| C1  | `0x0be8b961…` |
| C2  | `0x1a37DcDa…` |

Der private Schlüssel deiner personengebundenen Wallet liegt damit auf einem
Server am offenen Netz. Wer eine Box übernimmt, übernimmt die Identität
mitsamt Guthaben — und die dreifache Validator-Prüfung schrumpft dort auf
eine, weil „Mensch autorisiert" und „Schlüsselbesitz" derselbe Schlüssel sind.

**Kein Beta-Blocker, aber vor echtem Betrieb zu trennen:**
`RELAYER_PRIVATE_KEY` sollte ein eigener, nur für den Knoten erzeugter
Schlüssel sein; `NODE_OPERATOR_WALLET` bleibt einfach die Adresse, an die
Belohnungen gehen — dafür braucht der Server keinen Schlüssel.

Das ist bewusst nicht heute geändert: ein neuer Signierschlüssel ändert die
Blockproduktions-Identität und macht C2s Registereintrag ungültig. Das gehört
in ein ruhiges Fenster, nicht in die Nacht vor dem Start.

---

## Erste echte Sperren-Messung, 29.08.2026

Bis heute war der Engpass geraten — dreimal, dreimal falsch. Jetzt gemessen
(Go-Mutex-Profil auf C1, unter Last von C2):

| Anteil an der Wartezeit | Ort |
|---|---|
| **74,7 %** | `processTransferBatch` → `runAtomicWithOutbox` |
| 19,1 % | `flushWALBatch` → `shardedAccounts.LockAddrs` |

`runAtomicWithOutbox` hält die **globale** Zustandssperre `cs.mu` über eine
komplette Datenbank-Transaktion, `tx.Commit()` eingeschlossen. Das ist
Absicht — der Kommentar daneben erklärt, dass Speicherzustand und DB-Transaktion
sonst auseinanderlaufen. Der Preis ist die Serialisierung: jede Überweisung
wartet auf jede andere, chainweit.

Das erklärt alles, was vorher widersprüchlich aussah: nur ein Kern von sechs
ausgelastet, mehr Sender ohne Wirkung, einbrechende Blockproduktion.

**Der Weg zu 10k steht im Code selbst** (`dbExecCtx`, state.go): `cs.activeTx`
ist ein einziges ChainState-weites Feld, das echte Nebenläufigkeit unmöglich
macht. Die Migration auf per-Operation-`ctx` läuft seit Juli, **48 Aufrufstellen
sind noch offen** (state.go 22, evm_storage.go 12, block.go 5, guardian.go 4,
snapshot.go 2, slashing.go 2, register.go 1). Der Transferpfad selbst ist
bereits migriert.

Danach — und erst danach — lässt sich die Sperre auf die bereits vorhandenen
Konten-Shards verengen (`shardedAccounts.LockAddrs`, die der WAL-Schreiber
schon benutzt).

Der Code warnt ausdrücklich davor, das in einem Zug zu tun: es quert Transfer,
Swap, Liquidität, Registrierung, Verteilung, Guardian, Slashing, Snapshot-Import
**und Block-Replay**. „one call chain at a time, EACH one verified by the full
test suite."
