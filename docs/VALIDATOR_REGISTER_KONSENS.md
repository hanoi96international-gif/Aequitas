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
- Die Komitee-Auswahl (`computeEpochCommittee` über
  `authorizedValidators`) sortiert die lokal bekannten Adressen. Sie
  entscheidet nur, ob ein Knoten selbst erzeugt. GHOSTDAGs K hing bis
  07.10.2026 zusätzlich an der Größe dieses lokalen Komitees, und das nur
  auf Knoten, die bis zur Komiteeprüfung kamen; ab 57 Validatoren
  hätten Erzeuger und Beobachter mit verschiedenem K gerechnet. Seitdem gilt
  für alle K = 18, bis ein Komitee aus der Kette kommt.
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
3. **Leser umstellen** – Teil 1 umgesetzt (07.10., `validator_register_leser.go`),
   schlafend bis zu zwei eigenen Stichtagen (Platzhalter `math.MaxInt64`,
   beide mindestens eine Woche nach `validatorRegisterAb`, per Test erzwungen)
   - **Der Verlauf** (`validator_verlauf`, nach dem Sicherheitsdurchgang
     zu #303): das Register hält je Betreiber nur die letzte Bindung, beide
     Leser brauchen aber, was **zu einer Zeit** galt. Darum steht jede
     Bindung, die das Register geändert hat, mit beiden Unterschriften im
     Verlauf. In der Summe (`validatorVerlaufBlatt`), im Snapshot (jede Zeile
     prüft der Importierende selbst), Rücknahme mit dem Block.
   - **Höchstens eine Bindung je Betreiber und Tag**
     (`validatorBindungAbstand`, zweiter Durchgang, H1): eine Bindung, die
     weniger als einen Tag nach der letzten des Betreibers liegt, wird als
     Zustandsablehnung übersprungen und hinterlässt keine Zeile; der
     Snapshot-Import prüft dasselbe. Sonst füllte jeder Mensch den Verlauf
     (eine Bindung je 30 s über die Annahme, beliebig viele je Block über
     einen Erzeuger) und damit Summe und Snapshot – über 50 MiB scheitert
     jeder Snapshot-Import. So wächst der Verlauf höchstens um eine Zeile je
     Mensch und Tag. Wer seinen Schlüssel wechselt, wartet bis zum nächsten
     Wechsel einen Tag; der alte erzeugt so lange weiter. **Preis:** zwei
     Bindungen desselben Betreibers innerhalb eines Tages in
     Geschwisterblöcken – welche gilt, hängt von der Reihenfolge ab (jede
     Regel, die Zeilen je Betreiber begrenzt, hat diesen Rand; eine, die
     ersetzt statt abweist, löschte Zeilen, auf die die Frist baut). Ein
     ehrlicher, einziger Annehmender legt sie nie nebeneinander; geschlossen
     erreicht die Erzeugerprüfung nur, wer einen Schlüssel der Liste
     betreibt. Aus dem
     Verlauf die **Zeiträume**: eine Bindung gilt ab ihrem Zeitpunkt bis zur
     nächsten Bindung ihres Betreibers oder bis ein anderer Betreiber den
     Schlüssel später bindet. Hängt nur an der Menge der Zeilen, nicht an
     ihrer Reihenfolge.
   - **`registerLeserAb`**. Leitung und Abgleich schalten an der Blockzeit
     bzw. der Uhr, die Geldstrafe an `DetectedAt` (beim Erkennen wie beim
     Nachspielen gleich).
     - Geldstrafe (`strafe_abrechnung.go`, zweiter Sicherheitsdurchgang zu
       #303, M1/M2/L3): `slash_equivocation` vermerkt Beweis, Zähler und
       Sperre wie bisher, bucht aber kein Geld mehr. Ein zweites Vergehen
       wird als `strafe_offen` markiert; die Strafe bucht eine eigene
       Transaktion **`slash_abrechnung`**, frühestens bei Blockzeit
       `DetectedAt` + W + `erzeugerFrist` (W = `strafBeweisFrisch` = 1 h,
       zusammen drei Stunden). Gelegt wird sie vom Leiter bzw. dem einen
       annehmenden Knoten (`StarteStrafAbrechnung`, einmal je Minute).
       - **Wer zahlt** (`strafKontoZurAbrechnung`), nur aus Bindungen bis
         `DetectedAt` + W: hat in dieser Zeit ein **anderer** Betreiber den
         Schlüssel gebunden, der **erste** davon; sonst, wer ihn zuletzt vor
         oder bei der Tat gebunden hat (auch nach einem eigenen Wechsel).
         Umstritten oder keiner: keine Geldstrafe. Nur ein Mensch zahlt.
         Wer einen Schlüssel übernimmt, haftet damit für Beweise, die bis zu
         einer Stunde vor seiner Bindung datiert sind – „nach der Tat hat ein
         anderer gebunden, also zahlt keiner“ gibt es nicht mehr (M2).
       - **Gleich für alle** (M1): jede zählende Bindung steht in einem
         Block höchstens eine Stunde nach ihrem Zeitpunkt; bis zur
         Abrechnung hatte jeder Knoten mindestens eine Stunde, sie
         nachzuspielen. Erkennender und Nachspielende rechnen dasselbe,
         egal, wann sie eine Übergabe gesehen haben.
       - **Nur frische Beweise:** ab dem Stichtag steht `slash_equivocation`
         höchstens W nach und höchstens fünf Minuten vor `DetectedAt` in
         einem Block (sonst ist der Block ungültig); der Erkennende legt
         andere nicht in den Ausgang und vermerkt sie nicht. Ohne die Grenze
         nach vorn legte der Halter einen Beweis mit `DetectedAt` in zwei
         Tagen in seinen Block, übergäbe morgen – und der Nachfolger hielte
         den Schlüssel „zur Tat“. Sonst hängte ein späterer Halter, der den
         Schlüssel kennt, dem früheren einen alt datierten Beweis an. Wer
         nach seiner Bindung X einen Beweis erfindet, datiert ihn auf
         mindestens X − W – und zahlt dann selbst.
       - **Ungültig** (der Block wird abgewiesen): Abrechnung vor der
         Fälligkeit, für einen unbekannten Beweis, ohne offene Strafe, mit
         falschem Unterzeichner oder Zeitpunkt, oder bei einem Lesefehler.
         Eine zweite Abrechnung desselben Paars bucht nichts. Vor dem
         Stichtag ist `slash_abrechnung` unbekannt.
       - Ein vor den Stichtag datierter Beweis nimmt den alten Weg über
         `registered_nodes` nur, wenn er höchstens W nach `DetectedAt` in
         einem Block steht – also nur in der ersten Stunde nach dem
         Stichtag (vorher war das an `max(DetectedAt, Blockzeit)` geschaltet;
         das rechneten Erkennender und Nachspielende um den Stichtag
         verschieden, L3).
       - **Grenzen:** Den Beweis kennt nur, wer `slash_equivocation`
         nachgespielt hat – er steht nicht in der StateRoot und nicht im
         Snapshot; ein Knoten aus einem Snapshot weist eine Abrechnung ab
         (wie bisher das zweite Vergehen, das er nicht zählen konnte). Ohne
         Leiter oder annehmenden Knoten bleibt die Strafe offen. Haben
         innerhalb von W mehrere andere gebunden, zahlt der erste, auch wenn
         ein späterer den Beweis erfunden hat.
     - Leitung (`validatorMenschVon`): der Mensch, der den Schlüssel
       zuletzt gebunden hat, aus dem Stand im Speicher – keine Datenbank
       unter der Sperre der Leitung. Nach einem Wechsel bleibt der alte
       Schlüssel seinem Menschen, bis er die Leitung verlässt; sonst säße
       derselbe Mensch mit beiden Schlüsseln drin (L2). Nach einem Lesefehler
       bleibt der letzte Stand stehen.
     - Mit `AUTHORIZED_VALIDATORS` nimmt der Abgleich unter Peers
       (`syncValidatorsFromPeer` und die Liste des Seeds) nichts mehr auf.
   - **`erzeugerSchnittAb`** (an der Blockzeit plus der Rück-Toleranz von
     zwei Minuten, ≥ `registerLeserAb`): wer Blöcke erzeugen darf – ein
     Schlüssel, dessen Zeitraum **zur Zeit des Blocks** gilt, um die
     **Frist** von zwei Stunden verschoben (`erzeugerFrist` =
     `nachweisHoechstensAlt` + 1 h): eine neue Bindung wirkt erst zwei
     Stunden nach ihrem Zeitpunkt, eine beendete noch zwei Stunden danach.
     Der Zeitpunkt liegt höchstens eine Stunde vor dem tragenden Block, also
     hat jeder Knoten mindestens eine Stunde, ihn nachzuspielen, bevor er auf
     irgendeinen Block wirkt; bis dahin urteilen alle gleich (per Test über
     zufällige Verläufe erzwungen). Eine Übergabe ist nahtlos, der alte
     Schlüssel erzeugt bis zum Ende der Frist. Vorher wies jeder Knoten, der
     eine Übergabe schon nachgespielt hatte, die Blöcke des alten Schlüssels
     ab, während andere sie annahmen – das Netz zerfiel dauerhaft.
     - Mit `AUTHORIZED_VALIDATORS` die **Schnittmenge** aus Liste und
       Register; gelesen werden nur die Schlüssel der Liste. Ohne Liste das
       Register allein (dann endet auch der Abgleich unter Peers erst hier,
       sonst nähme nur der Knoten, bei dem sich ein neuer Erzeuger
       eingetragen hat, dessen Blöcke an). **Für den Stichtag ist nur der
       geschlossene Betrieb freigegeben:** offen liest der Stand den ganzen
       Verlauf, höchstens 100.000 Zeilen, jeder Mensch kann ihn füllen, und
       darüber schließt die Prüfung für alle ab. Vor einem offenen Betrieb
       bräuchte es einen Mindestabstand je Betreiber und einen
       fortgeschriebenen statt neu gelesenen Stand.
     - Der Stand liegt im Speicher (Prüfung unter `dag.mu` ohne Datenbank),
       neu gelesen nach jedem Block, der den Verlauf erweitert hat, nach der
       eigenen Annahme, nach einem Snapshot-Import und alle 30 s (nur, wenn
       ein Stichtag keinen Tag mehr entfernt ist); gelesen vor P2P und
       HTTP-Sync. Kein Stand oder ein Lesefehler: nur noch Blöcke aus der
       Geschichte (fail-closed).
     - Ohne eigene Bindung erzeugt der Knoten nicht. **Erst setzen, wenn
       `/api/status` → `erzeuger_ohne_bindung` leer ist** (null heißt:
       offen oder nicht lesbar) – sonst schlösse sich C1 selbst aus.
   - Bleibt von der Reihenfolge abhängig: eine Übergabe in einem
     Geschwisterblock der Strafe (wie „Mensch im Geschwisterblock“). Ein
     ehrlicher Annehmender legt beides in eine Linie.
   - **Kein spät eingehängter Block mit Bindung oder Beweis** (L1,
     `spaet_eingehaengt.go`): Erzeugerprüfung und Abrechnung setzen voraus,
     dass jeder Knoten eine Zeile kennt, bevor sie wirkt. Ein Erzeuger, der
     einen Block mit Bindung zurückhält und Stunden später über einen
     frischen Nachfolger einhängt, umginge die Finalitätswand (ein
     nachgeholter Vorfahr mit wartendem Nachfolger ist ausgenommen, und
     ruhende Finalität hält nichts auf) – seine Zeile wirkte rückwirkend.
     Ab dem Stichtag nimmt ein Knoten einen Block mit `validator_bindung`
     oder `slash_equivocation` nicht an, wenn dessen Blockzeit mehr als
     **30 Minuten hinter seiner eigenen neuesten Spitze** liegt. Schaden
     entstünde nur, wenn ein Block Y mit t_Y ≥ Zeitpunkt + 2 h schon
     beurteilt wäre – dann liegt die Spitze mindestens eine Stunde nach dem
     späten Block, und er wird abgewiesen. **Bewusst nicht die Uhr:** der
     erste Block des einzigen Erzeugers nach einem Absturz trägt eine
     Bindung aus dem Ausgang mit der Zeit ihrer Annahme (Stunden zurück);
     nach der Uhr wiese ihn jeder ab und die Kette risse, gegen die Spitze
     (Stand vor dem Absturz) ist er pünktlich. Ein nachholender Knoten hat
     ebenso alte Spitzen. Ausgenommen: Geschichte vom vertrauten Seed
     (`FromSync`), in der ein zurückgehaltener Block nie steht. **Grenzen:**
     Gibt ein Erzeuger den Block genau an der Grenze frei, können Knoten ihn
     verschieden behandeln (wie an der Finalitätswand); er verliert damit
     höchstens seinen eigenen Block. Mit mehreren Erzeugern wird der erste
     Block eines abgestürzten Erzeugers abgewiesen, wenn die anderen
     weitergemacht haben und er eine Bindung von vor dem Absturz trägt – er
     setzt dann vom Seed neu auf. Ebenso heilt ein **Erzeuger**, der über 30
     Minuten abgeschnitten war und weiter erzeugt hat, nicht mehr von selbst,
     wenn auf der anderen Seite eine Bindung oder ein Beweis stand; ein
     Knoten, der nicht erzeugt, ist nicht betroffen (seine Spitzen stehen).
   - Erledigt vor `registerLeserAb`: M1, M2, L3 (spätere Abrechnung) und L1.
   - **Teil 2, Validatoren-Belohnung aus der Kette** (`validator_lohn_kette.go`,
     schlafend): dieselbe Regel wie bisher – gleicher Anteil je Minute
     Anwesenheit, mehr Blöcke in einer Minute zählen nicht –, aber jede
     Eingabe steht in der Kette. Die Blöcke aus `chain_blocks`; wem ein Block
     gehört, sagen die Erzeugerfenster aus dem Verlauf der Bindungen (zur
     Blockzeit, umstritten: keiner); nur Menschen. Gezählt werden die 24
     Stunden bis 15 Minuten vor der Runde. Die Gutschrift trägt die
     Rundenzeit (höchstens 10 Minuten neben der Blockzeit), und **jeder
     Knoten rechnet die Runde nach**: Empfänger, Betrag (1 Mikro Rundung),
     keiner doppelt, keiner vergessen (`validator_kein_betreiber`,
     `validator_anteil`, `validator_doppelt`, `validator_empfaenger`,
     `validator_ohne_runde`, `validator_runde`). Ein Knoten mit Lücke im
     Fenster (ein 10-Minuten-Abschnitt ohne Block: frisch aus einem Snapshot
     oder neu aufgesetzt) prüft nur Mensch und doppelt. Kann der Erzeuger die
     Anwesenheit nicht lesen (über 1.000 Schlüssel, Verlauf zu groß), bleibt
     der Validatoren-Topf stehen; die Tagesrunde läuft weiter. **Schalter:**
     erst wenn das ganze Fenster nach `erzeugerSchnittAb` liegt – vorher
     zählten Blöcke ungebundener Schlüssel nicht. **Grenze:** hat ein Knoten
     einen Block des Fensters, den der Erzeuger nicht hat (an der
     Finalitätswand verschieden behandelt), meldet er eine Abweichung.
   - Offen: das Komitee (`getEpochCommittee`) aus derselben Menge statt aus
     den lokal bekannten Adressen.
   - Offen (eure Entscheidung): die Leistungsprobe wird zur Entscheidung des
     Leiters, die als eigene Transaktion auf die Kette kommt – oder entfällt.
4. **Coordinator-Register** (`coordinator_keys`): seit 06.10.2026 trägt die
   Erneuerungs-Bescheinigung ihre Bindung selbst, und jeder Knoten prüft sie
   gegen den Kettenzustand (`bescheinigungPruefen`). **Zulassung und Entzug
   im Konsens (07.10.2026, `coordinator_zulassung.go`):** Coordinator darf
   nur sein, wer zur Zeit der Bescheinigung (`issued_at`) einen
   Validator-Schlüssel im Kettenregister hält – dieselben Erzeugerfenster
   wie bei der Erzeugerprüfung (Frist, nur Menschen, umstritten: keiner).
   Wer seine Bindung verliert oder den Schlüssel abgibt, bescheinigt nicht
   mehr; eine Farm mit einem alten Konto ohne Validator-Bindung auch nicht.
   Vor `registerLeserAb` ist niemand zugelassen (fail-closed), darum muss
   `registerLeserAb` ≤ Staffel-Stichtag sein (Test). Grenze: gezählt wird
   `issued_at`, nicht die Blockzeit – eine vor dem Ende der Bindung
   ausgestellte Bescheinigung bleibt bis zu 7 Tage gültig.
   Die Staffel bleibt beim Platzhalter, bis auch strenger Modus und
   Chain-ID stehen, erzwungen durch einen Test.

## Entscheidungen, die bei euch liegen

- Der Stichtag für Schritt 1–3 (heute Platzhalter `math.MaxInt64`).
- Ob die Leistungsprobe bleibt (dann als Kettenentscheidung) oder entfällt.
- Ob `validator_keys` (Personhood-Schlüssel, Vergleichsdienst-URL) mit ins
  Kettenregister kommt oder Betriebsangabe bleibt.
