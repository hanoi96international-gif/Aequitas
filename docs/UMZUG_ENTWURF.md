# Umzug — Entwurf vor dem Bau

Stand 25.10.2026. Ausgangspunkt: `REGISTRIERUNG_ENDFASSUNG.md` 5.4 (Ablauf,
Grenzen, Zwang, Zwillinge) und Nr. 7 der Umsetzung. Dieser Entwurf
beantwortet die offene Frage von dort — **V8-Vertrag oder Go-Tabelle** — und
legt fest, was jeder Knoten prüft, was umzieht, was die StateRoot dafür
braucht und welche Missbrauchstests vor dem Bau stehen. Gebaut ist nichts.

## 1. Entscheidung: kein neuer Vertrag, der Go-Zustand ist das Register

`AequitasV8.sol` schreibt Menschen nur beim Annehmen
(`registerWithSig`), und nur auf dem annehmenden Knoten. Was jeder Knoten
hat, ist der **Go-Zustand**: `chain_accounts.is_human`, `nullifiers`
(Nullifier → Wallet), `bio_registrations` (Commitment → Wallet). Aus ihm baut
`migrateEVMFromGoStateV8` (`evm_storage.go`) die Vertragsplätze 1–7 nach
jedem Snapshot-Import neu auf; `v8_slots.json` erlaubt Go genau diese
Plätze. Der Vertrag ist für Menschen also ein **Spiegel**, keine zweite
Wahrheit.

Daraus folgt: Der Umzug ist eine Go-Transaktion, die den Go-Zustand ändert
und dieselben Plätze spiegelt wie die Migration (3 `isHuman`, 4
`usedCommitments`, 5 `usedNullifiers`, 6 `commitmentOf`, 7 `nullifierOf`).
Kein Vertragswechsel, kein Neustart bei Höhe null, kein Registrar-Schlüssel
im Spiel. Die Alternative (eine Vertragsfunktion `moveWithSig`) brächte
nichts, was die Knoten nicht ohnehin selbst prüfen müssen — die EVM läuft
nur auf dem annehmenden Knoten —, kostete aber eine neue Fassung samt
Speicherplan-Test.

## 2. Befund beim Entwurf: die Zuordnung steht nicht in der StateRoot

`nullifierLeaf` (`state.go`) ist `sha256("null:" + nullifier)` — **ohne
Wallet**. Heute reicht das: ein Nullifier gehört einmal einer Wallet und nie
einer anderen. Mit dem Umzug ändert sich die Zuordnung; ein Knoten, der sie
falsch führt, fiele in keiner StateRoot auf. Darum gehört zum Bau ein
eigener Akkumulator `umzugSetXOR` über
`sha256("umzug:" + N + ":" + W2 + ":" + vollzogen_am)` für jeden vollzogenen
Umzug, und offene Ankündigungen als Kontofeld von W1 (siehe 4). Bestehende
Blätter bleiben unverändert — ohne Umzug ist der Akkumulator null und
ändert die Wurzel nicht (dasselbe Muster wie die Felder, die `accountLeaf`
nur anhängt, wenn sie gesetzt sind).

## 3. Ablauf und Prüfungen

| Schritt | Wer | Was jeder Knoten selbst prüft |
|---|---|---|
| 1. Antrag | App mit neuer Wallet W2, Aufnahme mit voller Aufgabe | – |
| 2. Bescheinigung | Coordinator + Vergleichsdienste: Gesicht bekannt, **eine** Einschreibung, strengere Schwelle als die Duplikatsuche; Poseidon(getroffene bio) = N (wie `erneuerung.py`) | – |
| 3. `umzug_ankuendigen` | Transaktion mit: N, W1, W2, `issued_at`, Bescheinigungen von **zwei verschiedenen** zugelassenen Betreiber-Menschen (Ed25519, Domäne `aequitas-umzug-v1\|chain:<id>\|<N>\|<W1>\|<W2>\|<issued_at>`), neuem Groth16-Beweis für W2 (Commitment c2, derselbe Nullifier N) und EIP-712-Unterschrift von W2 | Unterschriften und Bindungen gegen das Register (`bescheinigungPruefenFuer`, wie beim Einspruch); N gehört im eigenen Zustand zu W1; W1 ist Mensch; W2 ist kein Mensch, hat keinen Nullifier, ist keine System- oder Unternehmensadresse; c2 unbenutzt; Beweis gültig; kein Umzug für N offen; höchstens zwei vollzogene in 365 Tagen; `issued_at` ≤ Block + 5 min und ≥ Block − 24 h |
| 4. Zeitschloss | 14 Tage (Spanne 7–30, eine Konstante) | – |
| 5. `umzug_widerruf` | W1 unterschreibt (EIP-191, Domäne `aequitas-umzug-widerruf-v1\|chain:<id>\|<N>\|<W2>`) | nur W1, nur solange offen |
| 6. `umzug_vollzug` | Tageslauf, wie die Staffel-Freigabe | Frist abgelaufen nach **Rundenzeit**; nicht widerrufen; Bedingungen aus 3 gelten noch (W2 inzwischen Mensch? dann verfällt der Umzug); nachgerechnet wie `grant_release` |

Was beim Vollzug umzieht und was nicht:

| Feld | zieht um | Begründung |
|---|---|---|
| `is_human` | ja (W1 → false, W2 → true) | das Menschsein gehört dem Menschen, nicht dem Schlüssel |
| Nullifier N, neues Commitment c2 | ja; c1 bleibt benutzt | eine Einschreibung, eine Wallet |
| `lebensnachweis_am` | ja | derselbe Mensch hat ihn erbracht |
| offener Staffelrest, `grant_staged_until`, `liveness_renewed_at`, `staffel_einspruch_am` | ja, unverändert | der Rest gehört dem Menschen; kein neuer Startzuschuss |
| künftiges Grundeinkommen | ja | folgt `is_human` |
| **Guthaben, LP-Anteile, tUSD von W1** | **nein** | Geld bewegt nur eine Unterschrift (Grundsatz „nur bestätigen, nie wegnehmen“) |
| Nonces, Vormund-Einträge, Verantwortung in Unternehmen | nein | hängen am Schlüssel; der Mensch setzt sie mit W2 neu |

## 4. Zustand

- Kontofeld `UmzugOffen` bei W1: `{W2, c2, angekuendigt_am, frei_ab}` — nur
  solange offen, im `accountLeaf` nur wenn gesetzt, Spalte in allen Lade-,
  Speicher- und Stapelpfaden (dasselbe Vorgehen wie `StaffelEinspruchAm`).
- Tabelle `umzuege` (N, W1, W2, vollzogen_am) für die Jahresgrenze und den
  Akkumulator aus 2; im Snapshot-Export und -Import.
- Spiegelung in V8 beim Vollzug und in `migrateEVMFromGoStateV8`
  (aus `nullifiers`, das nach dem Vollzug W2 nennt).

## 5. Verifier-Prinzipien

Der Vollzug setzt `is_human` von W1 zurück. Das ist **kein Entzug**: das
Menschsein bleibt beim Menschen und wechselt die Adresse, ausgelöst nur
durch das Gesicht desselben Menschen, mit Frist und Widerruf durch den
alten Schlüssel. `kein_entzug_test.go` braucht dafür eine ausdrückliche,
begründete Ausnahme für `umzug_vollzug` — und **diese Entscheidung gehört
dem Betreiber** (`REGISTRIERUNG_ENDFASSUNG.md` Abschnitt 10, Nr. 3). Ohne sie
wird nicht gebaut.

## 6. Sicherheit (AGENTS.md, vor dem ersten Code-Zeichen)

1. **Wer erreicht den Code?** Jeder über App und Coordinator; die
   Transaktionen kommen in Blöcken von Peers. Feindlich, bis geprüft.
2. **Was ist begrenzt?** Ein offener Umzug je Nullifier, zwei vollzogene je
   365 Tage; zwei bis acht Bescheinigungen; feste Feldgrößen; Annahmefenster
   24 h; eigener Zähler je Absender (`nachweisGrenze`); Zeitschloss mit fester
   Spanne.
3. **Fehlerfall?** Fehlt eine Unterschrift, ist ein Schlüssel unbekannt oder
   ein Feld unlesbar: abgelehnt. Ist die Zulassung der Betreiber nicht
   lesbar: der Block zurück. Ein Umzug, der nicht geprüft werden kann,
   findet nicht statt; ein Vollzug, der nicht nachgerechnet werden kann,
   auch nicht.
4. **Prüft jeder Knoten?** Ja: Unterschriften gegen Register und Genesis,
   Beweis, Zuordnung N → W1 aus dem eigenen Zustand, Fristen nach Block- und
   Rundenzeit, Zuordnung nach dem Vollzug in der StateRoot (2).
5. **Missbrauchstests:** gefälschte oder fremd gebundene Bescheinigung; nur
   ein Betreiber oder derselbe zweimal; alte Bescheinigung wiederholt; Umzug
   auf eine Wallet, die schon Mensch ist oder einen Nullifier hat; auf eine
   Unternehmens- oder Systemadresse; falscher Nullifier für W1; Vollzug vor
   Ablauf; Vollzug nach Widerruf; Widerruf mit fremdem Schlüssel; zwei
   Umzüge gleichzeitig; dritter Umzug im Jahr; Zwillings-Einschreibung ohne
   gemeinsame Aufnahme; Guthaben von W1 bleibt bit-gleich; StateRoot zweier
   Knoten gleich nach Vollzug, Snapshot-Import und Rückrollen; vor der
   Aktivierung ist jede Art unbekannt (`bekannteTxArt`).

## 7. Reihenfolge des Baus

1. Entscheidung Betreiber zur Ausnahme in `kein_entzug_test.go`.
2. Kette, schlafend (Aktivierung 2100): Zustand und Akkumulator, die drei
   Transaktionen, Nachrechnen, Missbrauchstests, Datenbanktests.
3. Coordinator `/umzug` (strengere Schwelle, Zwillingsregel, Bescheinigung).
4. App „Konto zurückholen“ und die unübersehbare Anzeige bei W1.
5. Sicherheitsprüfung des Diffs, dann ein echtes Datum nach dem Ausrollen.
