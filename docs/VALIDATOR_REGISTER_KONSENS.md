# Validator-Register als Konsenszustand

Stand: 05.10.2026. Offener Punkt aus `docs/ERINNERUNG.md` („Validatoren-Gewichte
… Nachrechnen geht erst, wenn das Verzeichnis Konsenszustand ist“).

## Heute

Wer Blöcke erzeugen darf, wer Belohnung bekommt und von wem eine Strafe abgezogen
wird, steht in Tabellen, die **jeder Knoten selbst führt**:

| Tabelle / Quelle | Inhalt | befüllt durch |
|---|---|---|
| `validator_slots` | Betreiber-Wallet ↔ Signieradresse, mit Bindungs-Signatur | `/api/peers/register` (`BindValidatorSlot`) |
| `validator_keys` | Mensch ↔ Signieradresse (+ Personhood-Schlüssel, Vergleichsdienst-URL) | Abgleich zwischen Knoten (`syncValidatorsFromPeer`) |
| `registered_nodes` | Betreiber-Wallet ↔ Signieradresse, `blocks_produced` | `RegisterNode`, `BindValidatorSlot`, Zähler je erzeugtem Block |
| `AUTHORIZED_VALIDATORS` | geschlossene Erzeugerliste (K-1) | Umgebungsvariable |
| Leistungsprobe | Zulassung nur nach Messung | gemessen vom annehmenden Knoten |

Die Bindungs-Signatur (`Aequitas: authorize validator <adresse>`, EIP-191 vom
Betreiber) prüft jeder Knoten selbst. **Welche** Bindungen ein Knoten kennt,
hängt aber davon ab, wen er per Abgleich erfahren hat, und die Leistungsprobe
ist eine eigene Messung. Daraus folgt, schon im Code vermerkt:

- Strafkonto (`slash_equivocation`) und Validatoren-Belohnung hängen an
  `registered_nodes` – zwei Knoten können verschieden abziehen bzw. auszahlen.
- Die Komitee-Auswahl (`GetAllRegisteredValidatorAddresses`) sortiert die
  lokal bekannten Adressen.
- Die Signatur trägt keinen Zeitpunkt: eine alte Bindung lässt sich wieder
  einspielen und eine neuere damit zurückdrehen.

## Ziel

Jede Bindung ist eine **Kettentransaktion**. Jeder Knoten prüft sie selbst und
leitet das Register aus der Kette ab; die StateRoot trägt es. Lokal bleiben nur
Dinge ohne Konsenswirkung (Erreichbarkeit, Messwerte als Hinweis).

## Schritte

1. **Kettenregister, schlafend** (dieser Schritt, `validator_register.go`)
   - Transaktion `validator_bindung`: `Wallet` = Betreiber (registrierter
     Mensch), `To` = Signieradresse, `Nachweis` = Zeitpunkt und **zwei**
     Unterschriften über denselben Satz
     `Aequitas: bind validator <signieradresse> to operator <betreiber> chain:1926 ts:<unix>`:
     die des Betreibers und die des Signierschlüssels. Ohne die zweite könnte
     ein Mensch die Adresse eines fremden Knotens unter seinem Namen eintragen
     und, weil jede Adresse nur einem Betreiber gehört, den echten Betreiber
     aussperren (Audit 29.09., H1 – dort für `/api/validator-self-proof`
     behoben).
   - Unterschriften und Zeitfenster prüft jeder Knoten wie bei jedem anderen
     Auftrag (`pruefeAuftragsNachweis`: höchstens eine Stunde alt, höchstens
     fünf Minuten voraus) – Verstoß: Block ungültig. Ebenso Form (kanonische
     Adressen) und Stichtag.
   - Regeln gegen das Register: Betreiber ist Mensch, und die Bindung ist
     **neuer** als seine bisherige. Verstoß: die Transaktion wird
     übersprungen (eigener Zähler `uebersprungene_validator_bindungen`, nicht
     der Alarm für Kontoabweichungen). Ein Lesefehler weist den Block ab und
     wird nie zu „übersprungen“.
   - **Unabhängig von der Reihenfolge.** Geschwisterblöcke spielt jeder
     Knoten in Ankunftsreihenfolge nach; das Register darf davon nicht
     abhängen.
     - Je Betreiber gilt das Maximum über (Zeitpunkt, Signieradresse). Die
       Adresse entscheidet nur bei zwei Bindungen mit demselben Zeitpunkt.
     - Eine Adresse gehört der Bindung mit dem spätesten Zeitpunkt (dorthin
       hat der Schlüssel zuletzt zugestimmt). Stimmt der Schlüssel einem
       anderen Betreiber später zu, wird die frühere Bindung **überholt**
       markiert – in beiden Reihenfolgen. Eine überholte Bindung lebt nicht
       wieder auf, wenn der spätere Betreiber weiterzieht; der Betreiber
       bindet neu, mit neuer Zustimmung des Schlüssels. Teilen sich zwei den
       spätesten Zeitpunkt, gehört die Adresse keinem
       (`validatorZuSignieradresseCtx`).
     - Übrig bleiben zwei Fälle, in denen die Reihenfolge entscheidet:
       „Betreiber ist Mensch“, wenn seine Registrierung in einem
       Geschwisterblock der Bindung steht, und eine Bindung, die erst
       ankommt, nachdem ein späterer Betreiber derselben Adresse schon
       weitergezogen ist (dafür müsste jede Zustimmung unbegrenzt
       aufbewahrt werden). Ein ehrlicher Annehmender erzeugt beides nie
       (DAG-Regel; nimmt nur ein Knoten an, liegen alle Bindungen in einer
       Linie). Ein böswilliger Erzeuger erreicht damit eine
       StateRoot-Abweichung wie mit jeder Zustandsablehnung. Ohne die Prüfung
       „Mensch“ könnte jeder Erzeuger das Register mit erfundenen Schlüsseln
       füllen.
   - Jede Unterschrift hat genau eine Schreibweise (`0x` + 130 Hex klein,
     v 27/28, niedriges s) – sonst ließe sich dieselbe Bindung unter vielen
     Transaktions-Hashes einreichen.
   - Tabelle `validator_register`, Summe `validatorSetXOR` in der StateRoot
     (nur wenn es Einträge gibt – sonst byte-gleich), Rücknahme mit dem Block,
     Neuaufbau beim Start. Der Snapshot trägt das Register mit beiden
     Unterschriften; der importierende Knoten prüft sie selbst (vor jeder
     Sperre), ein falscher Eintrag, einer von vor dem Stichtag oder aus der
     Zukunft, oder eine falsche Überholt-Markierung verwirft den Import. Ist das Register (oder die Treuhand) beim Export nicht
     lesbar, gibt es keinen Snapshot (HTTP 503) statt eines unvollständigen,
     unterschriebenen. Merge-Import nimmt je Betreiber die neuere Bindung;
     `RESET_DB_STATE` und `CLEAR_REGISTRATIONS` leeren das Register.
   - **Vor dem Stichtag `validatorRegisterAbUnix` (Platzhalter
     `math.MaxInt64`) ist jede `validator_bindung` ungültig** – der Block wird
     abgewiesen (`bekannteTxArt`, beim Nachspielen noch einmal). Nichts
     ändert sich.
   - Eine Unterschrift ohne Zeitpunkt (die heutige Form
     `Aequitas: authorize validator <adresse>`) gilt auf der Kette **nicht**.
     Sie ist öffentlich (`/api/validators`) und zeitlos: wer sie kennt, hätte
     einen Betreiber an eine alte Adresse binden können, bevor er selbst
     bindet. Bestehende Betreiber unterschreiben deshalb neu (Schritt 2).
2. **Aussenden** – umgesetzt (05.10., `validator_bindung_annahme.go`),
   schlafend bis zum selben Stichtag
   - `/api/validator-selfproof` liefert ab dem Stichtag zusätzlich
     `bindung_zeit`, `bindung_nachricht` (`Aequitas: bind validator
     <signing> to operator <wallet> chain:<id> ts:<zeit>`) und
     `bindung_signatur_knoten` – nur für den eigenen Betreiber
     (`NODE_OPERATOR_WALLET`), wie der bisherige Nachweis.
   - `/node-binding` lässt die Wallet denselben Satz unterschreiben und
     schickt beide Unterschriften an `POST /api/validator-bindung`.
   - Nur der **Leiter** nimmt an (`annahmeBeginnenLeiter`; Folger leiten
     weiter, `zumLeiter`): das Register gehört keinem Konto, und so liegen
     alle Bindungen in einer Linie von Blöcken. **Ohne rotierenden Leiter
     nimmt nur der Knoten an, der ausdrücklich `ANNAHME_ROLLE=annehmend`
     trägt** – vor dem Stichtag auf genau einem Knoten setzen. Ohne Angabe
     nimmt kein Knoten Bindungen an (per Vorgabe nähmen sonst alle an).
   - Die Annahme prüft Form, beide Unterschriften, „Betreiber ist Mensch“
     und „neuer als die bisherige“ wie jeder Nachspielende – die letzten
     beiden zuerst ohne Schreibsperre (Vorprüfung), verbindlich in derselben
     Transaktion wie der Ausgang.
   - Strenger als das Nachspielen ist nur der Zeitpunkt: höchstens
     **10 Minuten** alt bei der Annahme (das Nachspielen nimmt eine Stunde).
     Liegt eine Bindung länger im Ausgang (Absturz), trägt der nächste Block
     die Zeit ihrer Annahme (`block_tauglich.go`); weggelassen wird nichts.
   - Grenzen: 4 KB Body, höchstens 4 Anfragen zugleich, je IP ein
     Fehlversuch je 30 s (gezählt auf dem Knoten, den der Mensch erreicht,
     vor der Weiterleitung), je Betreiber eine angenommene Bindung je 30 s.
     Signaturen mit `v` 0/1 oder Großbuchstaben werden angeglichen.
   - Angenommen heißt: im Ausgang des Leiters. Auf der Kette steht die
     Bindung mit dem nächsten Block.
   - Vor dem Stichtag: Endpunkt 409, Selbstnachweis ohne die neuen Felder.
   - Jeder bestehende Betreiber bindet nach dem Stichtag einmal neu – bei der
     heutigen Zahl (ein Betreiber, C1) ein Handgriff.
3. **Leser umstellen** (ab dem Stichtag)
   - Strafkonto, Validatoren-Belohnung, Erzeugerliste und Komitee lesen aus
     `validator_register` – über `validatorZuSignieradresseCtx`, und nur
     Betreiber, die Mensch sind; Blockzählung aus der Kette statt aus
     `blocks_produced`.
   - Der Abgleich `syncValidatorsFromPeer` entfällt für Bindungen.
   - Die Leistungsprobe wird zur Entscheidung des Leiters, die als eigene
     Transaktion auf die Kette kommt – oder entfällt.
4. **Coordinator-Register** (`coordinator_keys`): seit 06.10.2026 trägt die
   Erneuerungs-Bescheinigung ihre Bindung selbst, und jeder Knoten prüft sie
   gegen den Kettenzustand (`bescheinigungPruefen`). Offen ist die Zulassung
   und der Entzug von Coordinatoren im Konsens.
   Vorher bleibt die Staffel beim Platzhalter, erzwungen durch einen Test.

## Entscheidungen, die bei euch liegen

- Der Stichtag für Schritt 1–3 (heute Platzhalter `math.MaxInt64`).
- Ob die Leistungsprobe bleibt (dann als Kettenentscheidung) oder entfällt.
- Ob `validator_keys` (Personhood-Schlüssel, Vergleichsdienst-URL) mit ins
  Kettenregister kommt oder Betriebsangabe bleibt.
