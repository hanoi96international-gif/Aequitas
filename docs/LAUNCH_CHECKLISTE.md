# Launch-Checkliste

**Stand 24.09.2026, nachts. Zuerst: eine Gebühr für jede Überweisung. Dann: das fairste Geld (Geldflüsse geprüft und geändert). Dann: rotierender Leiter und Leistungsnachweis (gebaut, aus). Dann: Contabo1 ist abgeschaltet, was das bedeutet und was jetzt gilt. Der Stand vom 23.09. folgt darunter.**

> ## 24.09., nachts: Dieselbe Gebühr auf jeder Überweisung
>
> **Das Problem.** Die 0,1 %, die Website und Whitepaper nennen, zahlte nur, wer über den Token-Vertrag (V7) überwies. Die gewöhnliche Sendung, die die App benutzt, war frei. Wer den Umweg kannte, zahlte nichts; und das Grundeinkommen lebte fast nur von Swap-Gebühren.
>
> **Jetzt** (`ueberweisungsgebuehr.go`):
> - **Jede Überweisung eines Menschen zahlt 0,1 %**, egal über welchen Weg (Annahme-Tor, WAL, Stapel, V7).
> - **Obendrauf:** Der Empfänger bekommt genau den gesendeten Betrag, der Absender zahlt Betrag + Gebühr. Wer 10 AEQ verlangt, bekommt 10 AEQ.
> - **Aufschlag nur für große Guthaben, gemessen am fairen Anteil (1.000 AEQ):** ab 5.000 AEQ +0,1 %, ab 10.000 +0,5 %, ab 20.000 +1 %. Vorher hing der Aufschlag am Anteil an der *gesamten* Geldmenge; in einem kleinen Netz hielt damit jeder „viel“ und zahlte bis zu 0,6 % auf jede gewöhnliche Sendung.
> - **100 % ans Grundeinkommen.** Gutgeschrieben wird, wenn die Überweisung in einem gespeicherten Block steht, auf dem erzeugenden Knoten genau wie auf jedem nachspielenden. Die Transaktion trägt die Gebühr (`gebuehr`), damit jeder Knoten dasselbe abbucht.
> - **Durchsatz:** lokal gemessen kein Unterschied (4 Lastprofile, je 2 Läufe, Abweichung im Rauschen von ±3 %).
>
> **Belegt:** Alle Wege buchen Betrag + Gebühr ab; die Geldmenge bleibt exakt erhalten, wenn man die Gebühren im Grundeinkommen dazuzählt (`TestSupplyConservation_*`, Ring- und Stapeltests). Ein Nachspiel-Test zeigt, dass ein Knoten ohne das Feld `gebuehr` auseinanderläuft (900 statt 901,10 AEQ) und mit ihm nicht.
>
> **Für die App noch nötig:** Beim Senden die Gebühr anzeigen und „Maximum senden“ um die Gebühr kürzen; sonst scheitert eine Sendung des ganzen Guthabens an „zu wenig Guthaben“.

> ## 24.09., abends: „Das fairste Geld der Welt – auf alles bezogen“
>
> Alle Geldflüsse der Kette geprüft. Geändert (Kette, Website in 12 Sprachen, App in 12 Sprachen, README, Whitepaper):
>
> | Geldfluss | vorher | jetzt |
> |---|---|---|
> | **Demurrage** (Verfall über dem fairen Anteil) | 20 % Grundeinkommen, 40 % Validatoren, 30 % Kapitalgeber, 10 % Schatzkammer | **100 % Grundeinkommen** |
> | **Überschuss über der Vermögensgrenze** | ebenso 20/40/30/10 | **100 % Grundeinkommen** |
> | **Swap-Gebühr** | 40 / 30 / 20 / 10 | 40 % Validatoren / 30 % Liquiditätsgeber / **30 % Grundeinkommen / 0 % Schatzkammer** |
> | **Validator-Topf** | nach Blöcken seit Registrierung (Leiter, starke Hardware und die Ersten verdienen mehr) | **gleich je Menschen-Validator**, nur nach Minuten online; nur an Menschen |
> | **Überweisungsgebühr** | 100 % Grundeinkommen, aber nur über den Token-Vertrag | **auf jeder Überweisung**, 100 % Grundeinkommen (siehe oben) |
> | **Grundeinkommen, Registrierungszuschuss** | gleich für jeden Menschen | unverändert |
>
> **Zwei echte Fehler gefunden und behoben:**
> - **Die Töpfe waren Wallets mit bekanntem Schlüssel**, und nichts hinderte diesen Schlüssel daran, einen Topf leerzuräumen, auch das Grundeinkommen aller. Jetzt lehnen alle sechs Wege, auf denen Geld bewegt wird, einen Topf als Absender ab. Zusätzlich prüft `eth_sendRawTransaction` vor jeder EVM-Ausführung (`TestTopfSendetNie`).
> - **Beim täglichen Grundeinkommen verschwand Geld:** Griff beim Auszahlen die Vermögensgrenze, floss ihr Überschuss in den UBI-Topf, und der Abschluss setzte den Topf danach auf null. Im Test waren das 65.100 AEQ. Jetzt schreibt der erzeugende Knoten den Endstand in die Abschluss-Transaktion, und jeder Knoten übernimmt genau ihn. Alte Blöcke tragen 0 und verhalten sich wie damals (`TestUBIRunde_UeberschussBleibtErhalten`, Geldmenge exakt erhalten).
>
> **Konsensänderung:** Die neuen Aufteilungen gelten ab dem Deploy, ohne Aktivierungszeitpunkt. Das ist vertretbar, weil C2 der einzige Validator ist und die Kette ohnehin bei null neu startet. Mit mehreren Validatoren müssen alle zugleich umgestellt werden.
>
> **Noch offen, deine Entscheidung:**
> - **Schatzkammer = keine Entwicklungsfinanzierung mehr.** Ihre 10 % gehen jetzt ans Grundeinkommen. Wer das Projekt weiterentwickelt, bekommt aus dem Protokoll nichts. Umkehrbar mit einer Zeile (`swapGebuehrAnteile`).
> - **Liquiditätsgeber bekommen 30 % der Swap-Gebühr nach eingelegtem Kapital.** Das ist bei jeder Börse so, belohnt aber Vermögen.
> - **Verträge bereitstellen darf nur der Betreiberschlüssel** (`relayerAddressFromEnv`). Fair wäre: niemand oder jeder.
> - **`/api/admin/pool-correction`** (Betreiber kann AMM-Reserven verbrennen, standardmäßig aus): beim Neustart bei null entfernen.
> - **Beim Neustart bei null:** Topf-Adressen ohne bekannten Schlüssel wählen.

> ## 24.09., später: Rotierender Leiter, Leistungsnachweis, Bremse nach Mehrheit (gebaut, standardmäßig AUS)
>
> **Warum.** Bis jetzt nimmt ein fest eingestellter Knoten alle Überweisungen an. Fällt er aus, steht die Annahme. Genau das ist am 24.09. mit C1 passiert. Jetzt wechselt der annehmende Knoten, ähnlich wie der Leader bei Solana.
>
> - **Rotierender Leiter** (`leitung.go`, `leitung_netz.go`): Es nimmt immer genau einer an, die anderen leiten weiter. Weitergeleitet werden `eth_sendRawTransaction`, `eth_getTransactionCount`, Swap, Liquidität und Faucet. Ab **drei** Validatoren wählt die Mehrheit bei einem Ausfall in etwa 12–20 s einen neuen Leiter. Ein vom Netz getrennter Leiter hört auf, bevor irgendwo ein neuer gewählt wird (Lease). Ein planmäßiger Wechsel (Standard: alle 10 Minuten) übergibt erst, wenn alles Angenommene geschrieben ist, und der Neue nimmt erst an, wenn er den letzten Block des Alten hat. Bei **zwei** Validatoren gibt es keine automatische Übernahme. Tot und getrennt lassen sich von außen nicht unterscheiden, zwei Annehmende würden auseinanderlaufen.
> - **Leistungsnachweis** (`leistungsnachweis.go`): Jeder Knoten misst selbst Signaturen je Sekunde (alle Kerne), die Dauer eines Datenbank-Commits und seine Kernzahl. Vorgabe: ≥ 20.000 Signaturen/s, ≤ 20 ms, ≥ 4 Kerne. Es zählt der beste je gemessene Wert, damit Last den Nachweis nicht kostet. Ohne Nachweis wird ein Validator nur Leiter, wenn kein leiterfähiger erreichbar ist (Notbetrieb). Ein Leiter ohne Nachweis gibt ab, sobald einer lebt. **Der Nachweis entscheidet, wer leitet, nie, ob jemand annimmt.** Ergebnis in `/api/health/combined → leistungsnachweis`. Der Nachweis wird auch gemessen, wenn die Leitung aus ist, damit man sieht, ob ein Knoten Leiter sein könnte.
> - **Peer-Lag-Bremse nach Mehrheit:** Ein einzelner langsamer Validator bremst die Annahme nicht mehr, maßgeblich ist der Rückstand der Mehrheit.
>
> **Belegt:** Eine Simulation prüft in jedem 50-ms-Schritt, dass nie zwei gleichzeitig annehmen. Durchgespielt werden Ausfall, Neustart, Netztrennung, nicht transitive Trennung, Stimmengleichstand, planmäßiger Wechsel, 10–30 % Nachrichtenverlust und Uhrengang, dazu 50 Zufallsläufe mit 5 Knoten, 20 davon mit wechselndem Leistungsnachweis. Jeder absichtlich eingebaute Fehler lässt die Tests rot werden: Lease ignoriert (1.103 Verstöße), Lease-Zusage ignoriert (80), kein gestaffelter Neuanlauf, keine Abgabe ohne Nachweis, kein Notbetrieb. Ein Test über echtes HTTP mit echten Signaturen und 3 Knoten übersteht den Ausfall des Leiters. Dabei gefunden und behoben: Zwei Kandidaten im Gleichschritt teilten sich endlos die Stimmen.
>
> **Dezentral, ohne Handliste.** Welche Validatoren mitzählen, trägt niemand in eine Liste ein. Die Kette beginnt mit dem Genesis-Satz (wie jede Kette mit ihrer Genesis). Danach gilt:
> - **Beitritt:** Wer seinen Signierschlüssel an einen registrierten Menschen bindet (`register-validator-key`) und seinen Knoten mit `AEQUITAS_LEITUNG=an` startet, meldet sich bei seinen Peers. Der amtierende Leiter nimmt ihn auf. Niemand muss zustimmen, niemand kann es verbieten.
> - **Austritt:** Wer 30 Minuten nichts von sich hören lässt, wird entfernt, damit Ausgefallene die Mehrheit nicht unerreichbar machen. Meldet er sich wieder, kommt er wieder hinein.
> - **Den ersten Leiter** bestimmt eine Regel (kleinste Adresse im Genesis-Satz), kein Betreiber.
> - **Sicherheit beim Wechsel** nach dem Verfahren, mit dem Raft seine Mitgliedschaft ändert: je Änderung genau ein Validator, die nächste erst nach Bestätigung durch die Mehrheit, gewählt wird nur, wer einen mindestens so neuen Satz hat. Eine Aufnahme, die nicht binnen 30 s bestätigt wird, nimmt der Leiter zurück.
> - **Belegt:** Simulationen für Wachstum von 1 auf 5 Validatoren, Entfernen und Rückkehr, Rücknahme einer Aufnahme ohne Annahmelücke, Netztrennung während einer Aufnahme und das Schrumpfen von 3 auf 2 unter Trennung. Dazu 30 Zufallsläufe mit 6 Knoten, Beitritten, Ausfällen, Trennungen und 294 Satzänderungen, nie zwei Annehmende. Jeder absichtlich eingebaute Fehler lässt die Tests rot werden: Wahlregel ohne Satzstand (614 Verstöße), Bestätigung mit alten Quittungen (1.953), zwei Aufnahmen auf einmal (1.562), zu zweit annehmen ohne Bestätigung (18). Über echtes HTTP mit echten Signaturen (8 von 8 unter `-race`): Die Kette startet mit **einem** Validator. Zwei weitere kennen nur dessen URL, werden aufgenommen und machen nach seinem Ausfall ohne ihn weiter. Beim Bau gefunden und behoben: Eine Konfiguration ohne Entfernungsfrist warf alle anderen sofort hinaus.
>
> **Nachgeschärft nach der Frage „gerecht, korrekt, dezentral?“:**
> - **Ein Mensch, eine Stimme:** Pro Knoten galt „ein Mensch = ein Schlüssel“, aber wer auf zwei Knoten zwei Schlüssel registrierte, bekam zwei Stimmen. Jetzt nimmt die Leitung pro Mensch höchstens einen Schlüssel auf. Geprüft wird das über die signierte Bindung, beim Leiter und bei jedem Folger.
> - **Folger prüfen jede Änderung selbst:** Sie übernehmen keinen Satz, der einen Unregistrierten oder einen zweiten Schlüssel desselben Menschen aufnimmt. Ebenso wenig einen Satz, der einen Validator hinauswirft, den sie selbst noch hören. Dafür sendet jedes Mitglied alle 10 s ein Lebenszeichen an alle. Ohne Mehrheit gilt eine Änderung nie, und der Leiter nimmt sie zurück. Ein lügender Leiter kann die Mitgliedschaft so nicht mehr kapern (Test `TestSatz_LuegenderLeiter`).
> - **Nur abgeschnitten heißt nicht ausgefallen:** Der Leiter entfernt niemanden, den ein Folger in seiner Quittung noch als lebend meldet.
> - **Rotation alle 10 Minuten** statt stündlich: Kein Leiter entscheidet lange allein über die Reihenfolge der Überweisungen.
> - Gegenproben: Jede der neuen Regeln einzeln entfernt lässt Tests rot werden.
> - **Validator-Geld gerecht (entschieden 24.09.: „Es soll das fairste Geld sein“):** Jeder Mensch, der einen Validator betreibt, bekommt den gleichen Anteil am Validator-Topf. Gewichtet wird nur nach den Minuten des Tages, in denen sein Knoten da war. Vorher zählten die produzierten Blöcke seit der Registrierung. Damit verdienten der Leiter unter Last, also starke Hardware, und die ersten Betreiber auf Dauer mehr. Mehrere Blöcke in derselben Minute zählen einmal, und nur registrierte Menschen bekommen etwas (`validator_anwesenheit.go`, Test gegen echte Postgres).
> - **Grenze:** Das ist Absturz-Fehlertoleranz mit Plausibilitätsprüfungen, nicht volle Byzanz-Festigkeit wie bei Tendermint oder Solana (hält, solange weniger als ein Drittel lügt). Ein Leiter kann in seinen 10 Minuten Überweisungen zurückhalten oder umordnen. Validatoren sind an Menschen gebunden, und jede Nachricht ist signiert, also nachvollziehbar.
>
> **Einschalten:**
> ```
> AEQUITAS_LEITUNG=an                                 # jeder Validator
> AEQUITAS_LEITUNG_GENESIS=0xAdresse=http://IP:8080   # NUR die Genesis-Validatoren beim Neustart bei null
> AEQUITAS_LEITUNG_WECHSEL_MINUTEN=10                 # Vorgabe 10; 0 = kein planmäßiger Wechsel
> AEQUITAS_LEITUNG_ZWEI_WECHSELN=1                    # optional: planmäßig wechseln auch zu zweit
> AEQUITAS_LEITER_FAEHIG=ja|nein                      # optional, überstimmt die Messung
> ```
> `ANNAHME_ROLLE` wird mit eingeschalteter Leitung nicht mehr gebraucht. **Heute nicht eingeschaltet:** Es gibt nur C2. Einschalten gehört zum Neustart bei null (Genesis = C2, der neue Server tritt bei). Die automatische Übernahme braucht **drei** Validatoren, zu zweit gibt es sie nicht. Crash-Fehler sind abgedeckt, böswillige Validatoren nicht: Validatoren sind an Menschen gebunden und signieren jede Nachricht.
>
> **Was noch zentral ist:** Der Coordinator mit seinen Vergleichsdiensten (Quorum aus Betreibern), der Proof-Server und die Website mit DNS. Diese Dienste laufen heute auf C2. Wer registriert, prüft ein Quorum mehrerer Betreiber, aber es gibt noch keinen dritten Betreiber (Punkt 13). Die Deploy-Workflows dieses Repos sind Betriebswerkzeuge der beiden Boxen, nicht Teil des Protokolls: Ein fremder Validator braucht sie nicht.

> ## 24.09.: Contabo1 abgeschaltet, C2 trägt allein
>
> **Was passiert ist.** C1 (173.249.37.118) war ab der Nacht zum 24.09. weder per SSH noch per HTTP erreichbar: Das Abo ist ausgelaufen. **Entscheidung (Betreiber): C1 wird nicht verlängert, vor dem Launch startet die Kette ohnehin bei null.** Die Wache meldete den Ausfall um 05:17 UTC. GitHub startet den „alle 10 Minuten“-Zeitplan in der Praxis nur alle paar Stunden; eine engere Überwachung übernimmt der Wachhund auf der Box (ntfy).
>
> **Was ich umgestellt habe (alles umkehrbar):**
> - **C2 nimmt Überweisungen an** (`ANNAHME_ROLLE` entfernt); die Eigenlast-Bremse ist auf C2 aus, weil das beim einzigen annehmenden Knoten gemessen bremst.
> - **Vorladen (PR #180) läuft auf C2.** C2 wurde über `deploy-contabo2-manual.yml` deployt, weil der normale Deploy richtigerweise auf einen C1-Deploy wartet, der nie mehr gelingt. `deploy-contabo.yml` (C1) läuft nicht mehr bei jedem Push.
> - **C1 ist kein vertrauenswürdiger Seed mehr.** Eine gekündigte IP vergibt der Anbieter neu. C1 ist aus den eingebauten Seeds und Bootstrap-Adressen entfernt. Neu: `PRIMARY_NODE_URLS=keine` heißt „einziger Validator, keine Seeds“. Die Selbstheilung, der Totmann-Schalter und das Tor der täglichen Verteilung prüfen dann nicht gegen eine tote Adresse. Dieses Tor hätte die Verteilung auf C2 sonst dauerhaft gesperrt.
> - **Wache und Prüfstand arbeiten mit einer Knotenliste.** Ein neuer Server kommt mit einer Zeile dazu. **Falsch-Grün beseitigt:** „Coordinator ok, Quorum 2“ hieß nur, dass die Konfiguration 2 sagt. Jetzt wird gezählt, wie viele Vergleichsdienste tatsächlich antworten.
>
> **Was deshalb gerade NICHT geht (Prüfstand, ehrlich rot):**
>
> | | Warum | Wer |
> |---|---|---|
> | **aequitas.digital** (Startseite, Hauptadresse der App, APK-Download) | DNS zeigt auf C1 | **du:** A-Eintrag von `aequitas.digital` und `www` auf `194.163.188.71`. Das Not-Frontend auf C2 ist vorbereitet (`c2-emergency-apex-prep.yml`). Danach `tls internal` entfernen, damit ein echtes Zertifikat kommt. Die App 1.7.4 weicht bis dahin auf `proof2.aequitas.digital` aus, sofern sie mit der Ausweichliste gebaut ist. |
> | **Registrierung** | Der Coordinator auf C2 braucht 2 Vergleichsdienste, proof1 lag auf C1 | kommt mit dem zweiten Server zurück. Übergangsweise auf 1 zu senken ist deine Entscheidung (Dublettenschutz dann nur durch einen Dienst). |
> | **Zweiter Validator** | es gibt nur C2 | **du:** neuen Server beschaffen (Empfehlung: dedizierte Kerne, ≥ 8, NVMe). Mit ihm geht `mindestBootstrapKnoten` im Test zurück auf 2. |
>
> **Durchsatz, gemessen am 24.09. (Lauf 6, C2 allein, Generator auf derselben Box):** 4.401/s angenommen, **4.288/s in Blöcken (97 %)**, 0 Fehler, kein Neustart, keine Waisen. Die Kette hält jetzt mit der Annahme Schritt. In Lauf 4 auf C1 landeten von 9.728/s nur 4.483/s in Blöcken. Das Vorladen greift (81 von 81 Blöcken fanden ihren Korb vor). Die Grenze ist jetzt die Box: Knoten, Postgres und Generator teilen 6 Kerne. Ein WAL-Flush dauert unter Last 1,2 s statt Millisekunden, das Speichern eines Blocks 0,5–0,6 s. **10.000/s braucht mehr Rechenleistung** (dedizierte Kerne, schnelle NVMe) und einen Lastgenerator auf einer eigenen Maschine. Mit einer lokalen Freistellung ohne öffentliche Grenze (`AEQUITAS_RPC_RATE_LIMIT_FREI=172.18.0.1`, Docker-Gateway) misst der Generator auf derselben Box. Sie ist nur für den Lauf gesetzt und danach wieder entfernt, weil über IPv6 auch Fremde unter dieser Adresse ankommen könnten.
>
> **Neustart bei null (vor dem Launch):** eigener Schritt, wenn der zweite Server steht. Dazu gehören: neues `genesis_time`, `AUTHORIZED_VALIDATORS` = die beiden neuen Signieradressen, leere Kettendatenbanken, und das Löschen der Testdaten bei Proof-Server, Coordinator und Vergleichsdiensten (Nullifier, Skizzen, Widerspruchsvorgänge). Zu klären ist dabei, wie der erste Validator als registrierter Mensch gilt, wenn es noch keine Menschen gibt. Außerdem braucht es Schlüssel und `.env` beider Boxen **verschlüsselt außerhalb der Box**: Beim Ende von C1 ging sein `NODE_KEY` mit.

> ## 23.09.: Prüfstand
>
> **Live, von außen, beide Knoten: `pruefstand-live.yml` ist GRÜN** (nur lesend; läuft auf Knopfdruck). Geprüft wird, was ein Mensch tatsächlich benutzt: Kette (Höhe, Gleichstand, Commit, `/api/wache`, Divergenz), die App-Endpunkte auf **beiden** API-Basen, RPC, das Annahme-Tor (C2 lehnt ab, C1 nicht — mit einem wertlosen Platzhalter, ohne Transaktion), beide Coordinatoren (Quorum 2/2, Widerspruchsfrist, `/challenge`, `/widerspruch`), Vergleichsdienste, Proof-Server, **die ausgelieferte APK Byte für Byte gegen das neueste Release**, Website. Offen und nur als INFO: `/impressum`, `/datenschutz` (Blocker 3). Der erste Lauf war ROT — an meinem eigenen Prüfskript (jq wertet `false` in `a // b` wie „fehlt"; genau `false` ist dort der gesunde Wert). Behoben, zweiter Lauf grün.
>
> **Gefunden und behoben:**
> - **Nonces verbrannt auf dem nur lesenden Knoten** (Kette). Der Batch-Weg in `handleRPC` reservierte Nonces vorab, *bevor* das Annahme-Tor ablehnte — eine Wallet bekäme danach auf C1 „nonce too high". Jetzt prüft die Vorab-Reservierung dieselben zwei Sperren. Belegt durch drei Tests, einer davon Ende zu Ende durch den HTTP-Handler mit echt signierten Transaktionen (ohne Fix rot).
> - **Wer fälschlich als Duplikat galt, konnte nicht widersprechen** (Art. 22 Abs. 3). Der Coordinator vergab seit dem 26.08. eine Kennung, die App zeigte sie nirgends. **App 1.7.4** (Release `app-v1.7.4`, Tag auf seiner eigenen Quelle, Release-Schlüssel): Kennung, Frist 90 Tage, Knopf „Widerspruch einlegen", in 12 Sprachen. Auf beiden Boxen ausgeliefert.
> - **Widerspruchsvorgänge ohne Löschfrist** (Art. 5 Abs. 1 lit. e). Jetzt: ohne Widerspruch 90 Tage, nach Entscheidung ein Jahr; offene nie. Auf beiden Coordinatoren live. Offene Widersprüche färben die Wache **rot**; ansehen und abschließen mit `widersprueche-pruefen.yml` (nur Metadaten — das Repo ist öffentlich).
> - **Datenschutzerklärung beschrieb nicht den Betrieb.** Lebendigkeitsprüfung, zwei Vergleichsdienste verschiedener Betreiber, Widerspruchsvorgang mit Fristen, Serverstandort (zwei Contabo-Server in Lauterbourg, kein Drittland) — jetzt im Text und per Test an den Betrieb gebunden.
> - **`script_stop: true` zerlegte 21 Workflow-Schritte**, darunter den Wachhund und `rechtstexte-setzen.yml` (dein Ein-Klick-Weg für Blocker 3 — er wäre mit einem Syntaxfehler gestorben). Die Aktion hängt hinter *jede* Zeile eine Exit-Prüfung, auch in Heredocs, hinter `else` und in `case`. Alle umgestellt; `scripts/lint_script_stop.py` verhindert den Rückfall in CI.
> - **Falsch-grüne Tests**: drei App-Testdateien prüften nichts (`jest.isolateModules` wartet nicht auf `async`) — darunter die Belege für die beiden Registrierungsfixes vom 20.09. Die Fixes waren richtig, belegt sind sie erst jetzt. In der Kette übersprang sich ein Erhaltungstest der Schnellpfade immer und ein Pool-Test in jeder Umgebung; beide laufen jetzt gegen echte Postgres in CI.
>
> - **Ein Geldmengen-Alarm, der keiner war — und belegt, dass er keiner war.** Der neue Erhaltungstest der Schnellpfade fiel in CI einmal mit +0,015994 AEQ. Nachgestellt unter CPU-Last: in der CI-Gruppe 8 von 12 Läufen, allein 0 von 160. Die Summe lag auf drei Pool-Konten im Verhältnis 50 / 37,5 / 12,5 — geschrieben vom Pool-Flush eines **fremden** Test-Knotens, dessen Verbindung ein früherer Test nie geschlossen hatte und der nach dem TRUNCATE weiterschrieb. Jeder Untertest hat jetzt eine eigene Datenbank: 12 von 12 grün in der Gruppe unter Last. Der Knoten schöpft nichts. Offen, aber kein Beta-Punkt: viele `_RealDB`-Tests lassen ihren Knoten offen; dieselbe Klasse kann andere DB-Tests stören.
>
> - **Warum die Kette nicht über ~7.000 TPS kam — gefunden am 23.09. abends.** Beide Knoten fahren `AEQUITAS_MAX_TXS_PER_BLOCK=7000` und `ENABLE_MULTI_BLOCK_TICK=1`. Der Zusatzblock je Takt entstand aber nur bei **10.000** Transaktionen (Konstante), die ein Block unter dem Deckel 7.000 nie erreicht. Der Schalter war tot, und seit nur C1 annimmt, war die Kette hart bei 7.000/s. Behoben (PR #178): Voll heißt jetzt „am geltenden, ungebremsten Deckel“. Bremst ein Knoten, entsteht kein Zusatzblock. Ob C2 das Nachspielen stabil trägt, zeigt der Lastlauf.
> - **Lastläufe gegen C1 (23.09. abends), beide stabil** (keine Divergenz, 0 übersprungen, keine Neustarts, C2 höchstens 11 Blöcke zurück, danach 0):
>
>   | | Lauf 1 | Lauf 2 (`AEQUITAS_EIGENLAST_BREMSE=0` auf C1) |
>   |---|---|---|
>   | angenommen | 7.367/s | 6.596/s |
>   | **in Blöcken** | 3.457/s | **6.253/s** |
>   | Tx je C1-Block | 3.640 | 5.742 |
>
>   Ursache in Lauf 1: Unter Last hatte der Blockbau auf C1 eine **feste** Wartezeit von 300 bis 400 ms, unabhängig von der Blockgröße. Die Eigenlast-Bremse schrumpfte den Deckel trotzdem auf ~2.000, ohne Zeit zu sparen. C2 spielt in 70 ms je Block nach und braucht diesen Schutz nicht. **Die Bremse steht auf C1 jetzt aus. Werden die Rollen getauscht, muss das auf den neuen annehmenden Knoten mit umziehen.** Die Peer-Lag-Bremse, die den Nachspielenden schützt, bleibt an.
> - **Die Lasttests hätten nur Ablehnungen gemessen.** Alle liefen gegen C2, der seit dem 22.09. nichts annimmt. Jetzt geht die Last an C1, und der Generator bricht ab, wenn ein Ziel nur liest.
> - **Öffentliche Ratenbegrenzung stand auf beiden Knoten auf 100.000 statt 200** (übrig von früheren Lastläufen). Neu: nur die Partnerbox wird freigestellt (nach TCP-Adresse, nicht fälschbar), die öffentliche Grenze geht auf 1.000 je 10 s und IP.
> - **Test-Isolation an der Wurzel behoben:** 21 Testdateien ließen Knoten offen, deren Hintergrundarbeiter in fremde Tests schrieben.
>
> **Alarm (Blocker 6) ist zu.** Der Wachhund läuft auf beiden Boxen alle 5 min und meldet über **ntfy** — ohne Bot, ohne Konto. Den Kanalnamen habe ich dir im Chat genannt (er steht absichtlich nirgends im öffentlichen Repo; in den Logs maskiert). **Du:** ntfy-App installieren, den Kanal abonnieren. Fertig.
>
> **Bekannt, kein Beta-Blocker:** `TestOneTickCannotOutrunReplay` misst nur (≈ 10.600 Überweisungen je Tick nachgespielt gegen 50.000 im Ziel) — eine Lastgrenze, die 18 Menschen nicht berühren.
>
> **Für den Juristen (Blocker 5), neu:** (a) Rolle des Betreibers von proof2 — gemeinsam Verantwortlicher (Art. 26) oder Auftragsverarbeiter (Art. 28)? Es braucht einen Vertrag in der einen oder anderen Form. (b) Ist die Skizzen-Kopie beim früheren Railway-Coordinator (USA) nachweislich gelöscht? Die Instanz ist seit 14.09. weg; ein Löschnachweis fehlt.
>
> **Was nur du tun kannst — vollständig:**
> 1. `rechtstexte-setzen.yml` mit den sieben Feldern (Blocker 3). Der Prüfstand zeigt danach `/impressum 200`.
> 2. Eine echte Registrierung mit frischem Wallet in App 1.7.4 (Blocker 2).
> 3. Zwei-Personen-Test vor der Kamera (Blocker 4).
> 4. DSGVO-Entscheidung mit Jurist, inkl. (a) und (b) oben (Blocker 5).
> 5. ntfy-Kanal abonnieren (Blocker 6).
>
> ---
>
> **Stand 22.09.2026, 20:06. Beide Boxen laufen auf `06d4b7b`. Die Wache ist GRÜN** — zum ersten Mal seit dem 15.09.
>
> ## Was am 22.09. ausgeliefert wurde
>
> ```
> ✓ Hoehe waechst (7348235 -> 7348260)          beide Boxen, Abstand 0
> ✓ keine Divergenz, nichts uebersprungen        beide Boxen
> ✓ Proof-Server, Vergleichsdienst, Coordinator  Quorum 2 auf proof1 und proof2
> ✓ Platte 33 % / 65 %, nicht degraded
> ✓ letztes erfolgreiches Backup vor 11 h
> GRUEN
> ```
>
> **Der Zweig ist auf `main`** (PR #175), Deploy-Gate grün (`go build`, `go vet`, `go test ./...`, `npx hardhat test`), beide Boxen deployt und nachweislich wieder produzierend.
>
> **Blocker 1 ist geschlossen.** `ANNAHME_ROLLE=nur_lesend` steht auf Contabo2 — im laufenden Prozess nachgeprüft, nicht nur in der Datei. Da `deploy.sh` den Knoten mit `--env-file /root/.aequitas.env` startet, überlebt das jeden künftigen Deploy; ein Handgriff nach jedem Deploy ist es *nicht*. Nur noch Contabo1 nimmt Überweisungen an, und damit kann die Staub-Divergenz nicht mehr entstehen. Die App trifft ohnehin Contabo1 zuerst (`priority: 1` im FallbackProvider).
>
> **Die bestehende Abweichung ist weg.** Resync von Contabo2 aus dem signierten Snapshot von Contabo1.
>
> **Die APK auf den Boxen ist jetzt 1.7.3** (vorher 1.7.2, also die Version *mit* den beiden Registrierungsfehlern). 215 031 434 Bytes, SHA-256 `ae45d64e…` — identisch mit dem Release, direkt vom Knoten ausgeliefert. `AEQUITAS_APK_URL` zeigte auf beiden Boxen noch auf **v1.6.0** und steht jetzt ebenfalls auf 1.7.3.
>
> **Zum Release `app-v1.7.3`:** die veröffentlichte APK ist die richtige — gebaut aus Run 35478370296, also Commit `16647fc` mit beiden Registrierungsfixes. Falsch ist allein der Git-Tag-Zeiger (`4b44c4f`); die Herkunft steht als Build-Run im Release-Text. Kein neues Release nötig. Den Tag umzuhängen bräuchte einen Force-Push, den der Klassifikator verweigert — kosmetisch, nicht dringend.
>
> ## Zwei Fallen, die beim Ausliefern zugeschnappt sind
>
> **`resync-contabo2-only.yml` hat seinen eigenen Job sabotiert.** Der Lauf meldete `failure`, obwohl der Knoten sauber hochkam: `script_stop: true` stellt dem Skript ein `set -e` voran, und das `grep` über das Bootlog gibt bei null Treffern 1 zurück — der Schritt starb vor der Statusausgabe, also vor dem einzigen Teil, der zeigt, ob der Resync etwas gebracht hat. Schwerer wog zweierlei: sein `docker run` hängt **nur die WAL** ein und hat damit MPC-Ordner und lokale APK abgeworfen, und er ließ `RESYNC_FROM_SNAPSHOT=true` im laufenden Container stehen. Mit `--restart unless-stopped` heißt das: jeder Absturz, jeder Reboot hätte den Zustand erneut verworfen — genau die Dauerschleife, die `fix-resync-from-snapshot-flag.yml` im Kopf beschreibt, diesmal von der Wiederherstellung selbst erzeugt. Beides ist jetzt im Workflow behoben; der Lauf endet mit einem zweiten Neustart ohne den Schlüssel und mit vollständigen Mounts.
>
> **`fix-resync-from-snapshot-flag.yml` fasst beide Boxen mit demselben Wert an.** `value=true` hätte auch Contabo1 scharf gestellt — und dessen `deploy.sh` filtert den Schlüssel *nicht* heraus. Der nächste Deploy hätte den Primärknoten seinen eigenen Zustand wegwerfen lassen. Nicht benutzt; der Weg lief über `schraube-setzen.yml` nur für Contabo2.
>
> ## Was jetzt noch offen ist
>
> - **Punkt 11 nachmessen.** Im Code fertig, jetzt auch ausgeliefert — die Messung braucht einen Lastlauf.
> - **Die Wurzel.** Ausführung in kanonischer DAG-Reihenfolge statt in Ankunftsreihenfolge. Das Tor macht die Bedingung unmöglich, unter der der Fehler entsteht; es ersetzt den Umbau nicht. Wann das drankommt, entscheidest du.
> - **Nur du:** Impressum/Datenschutz-Felder, ein Registrierungslauf mit echtem Gerät, der Zwei-Personen-Kameratest, Telegram-Token, DSGVO-Entscheidung.
>
> ## 20.09.: Die App ließ sich überhaupt nicht mehr bauen
>
> Beim Anstoßen des Builds für 1.7.3 stellte sich heraus: der APK-Build scheitert seit Kurzem nach fünf Sekunden, **unabhängig von allem in diesem Zweig**. `android-actions/setup-android` installiert per Voreinstellung das längst zurückgezogene SDK-Paket `tools`; seit das Runner-Image auf `cmdline-tools 16.0` steht, bricht `sdkmanager` daran ab. Die Workflow-Datei war dabei Byte für Byte dieselbe, mit der 1.7.2 am 14.09. noch durchlief — geändert hat sich nur die Außenwelt, weil `@v3` ein wandernder Tag auf einem wandernden Image ist.
>
> Zweimal gelaufen, zweimal identisch gescheitert, also kein Zufall. Behoben mit `packages: ''` — damit überspringt die Action den `sdkmanager`-Aufruf ganz und tut genau das, was der Kommentar darüber ohnehin von ihr wollte. Der Build läuft seither durch das Setup hindurch.
>
> Das ist unabhängig vom Rest wichtig: **ohne diesen Fix hättest du auch keine Notfall-APK bauen können.**
>
> ## 20.09.: Die „Wurzel" hätte es nicht behoben — gemessen
>
> Wir haben beide monatelang dasselbe angenommen: sauber wäre, wenn sich der Zustand *ausschließlich* beim Anwenden eines Blocks ändert und die Annahme nur einreiht. Dann sähen alle Knoten dieselben Blöcke und kämen auf dasselbe Ergebnis.
>
> **Der zweite Teil ist falsch, und das ist jetzt belegt** (`geschwister_reihenfolge_test.go`). Das Experiment baut die Welt *nach* dem Umbau nach: beide Knoten nehmen **gar nichts** an, sie wenden nur Blöcke an — dieselben zwei, bloß in anderer Reihenfolge, weil jeder seinen eigenen zuerst hat. Sender hat 10, Block X will 8, Block Y will 6:
>
> | | Sender | Empfänger X | Empfänger Y |
> |---|---|---|---|
> | Knoten A (X zuerst) | 2 | **8** | 0 |
> | Knoten B (Y zuerst) | 4 | 0 | **6** |
>
> Uneins über **jedes** Konto. Ohne eine einzige Annahme.
>
> **Warum.** Ausgeführt wird in **Ankunftsreihenfolge**, nicht in kanonischer DAG-Reihenfolge. Bei zwei Produzenten entstehen Geschwisterblöcke — keiner ist Vorfahr des anderen — und die kommen bei den zwei Boxen verschieden an. Läuft ein Konto leer, entscheidet genau das, welche Überweisung noch bezahlbar ist; die andere wird übersprungen, auf jeder Box eine andere.
>
> **Was das für die Entscheidung heißt.** Der Umbau des heißen Pfades (WAL-Schnellpfad, Bündler, Shard-Sperren) wäre teuer gewesen und hätte *diese* Divergenz nicht beseitigt. Die bindende Bedingung ist nicht, **wo** mutiert wird, sondern **wer gleichzeitig annehmen darf** — oder dass in kanonischer Reihenfolge ausgeführt wird statt in Ankunftsreihenfolge, und das ist ein weit tieferer Umbau als der, den wir „die Wurzel" genannt haben.
>
> **Damit ist `ANNAHME_ROLLE=nur_lesend` kein Notnagel, sondern die Antwort auf die tatsächliche Bedingung.** Der Test steht grün und hält die Eigenschaft fest; wird die Ausführung eines Tages auf kanonische Reihenfolge umgestellt, geht er rot — und das ist dann die richtige Nachricht.
>
> **Punkt 11 ist im Code fertig.** Die zweite Hälfte („Client überspringt bekannte Hashes") existierte bereits (`schonBekannt` → kein Rumpf geholt); es fehlte nur `tx_root`, und das ist seit gestern da. Offen ist allein das Nachmessen — und das braucht einen Deploy.
>
> ## Blocker 1 hat jetzt einen Fix — dein Schalter
>
> **Setz auf der Box, auf die die App NICHT zeigt: `ANNAHME_ROLLE=nur_lesend`.** Danach nimmt nur noch eine Box Überweisungen an, und die Staub-Divergenz kann nicht mehr entstehen.
>
> **Warum das reicht.** Die Divergenz braucht, dass *dasselbe Konto auf zwei Knoten gleichzeitig belastet* wird. Nimmt nur einer an, geht jede Belastung durch eine einzige Sicht, und der nachspielende Knoten hat beim Anwenden des Blocks alle Vorgänger schon angewandt — sein Kontostand ist dort mindestens so hoch wie der, gegen den geprüft wurde. Es gibt nichts zu überspringen.
>
> **Gemessen, nicht behauptet.** Derselbe Testaufbau, dieselben Konten, dieselbe Last:
>
> | | Konten abweichend | übersprungen |
> |---|---|---|
> | ohne Sperre | 4 von 32 | 432 |
> | **mit Sperre** | **0** | **0** |
>
> **Was es kostet.** Die zweite Box trägt weiter API, RPC-Lesungen und Registrierung — abgelehnt werden nur Überweisungen, Swap, Liquidität und Faucet. Fällt die annehmende Box aus, hängst **du** die Rolle um. Das ist bewusst ein Handgriff: eine automatische Übernahme kann bei einer Netztrennung zwei Annehmende erzeugen, also genau den Zustand, den die Sperre beseitigt.
>
> **Ohne den Schalter ändert sich nichts** — die Voreinstellung ist wie bisher. Sichtbar unter `/api/health/combined → annahme_tor`.
>
> **Die Wurzel bleibt offen, und das ist eine Entscheidung.** Sauber wäre: Zustand ändert sich ausschließlich beim Anwenden eines Blocks, die Annahme reiht nur ein. Das ist die übliche Bauform einer Kette und macht nebenläufige Annahme auf beliebig vielen Knoten sicher — aber es ist ein Umbau des gesamten heißen Pfades (WAL-Schnellpfad, Bündler, Shard-Sperren) und nichts für die Woche vor einem Start. Wann das drankommt, entscheidest du.
>
> ## Was am 19.09. sonst noch behoben wurde
>
> - **Eine angenommene Überweisung wartete bis zu einer Stunde.** Der Nebenbefund von unten: `ProduceBlock` markiert die Ausgangskorb-Zeilen beim Laden und commitet das *vor* den Toren — bricht eines ab, lagen sie bis zum Aufräumer. Der Kommentar im Code behauptete das Gegenteil. Jetzt sofortige Freigabe genau der geladenen IDs.
> - **Punkt 11, erster Teil: `tx_root` steht in `chain_blocks`.** Der Kopf-Modus des Syncs griff nie, weil ein Block aus der Datenbank keinen `tx_root` mitbrachte — deshalb luden beide Boxen alle zwei Sekunden die letzten zwanzig Höhen *komplett mit Rümpfen* voneinander nach (1,92 GB in neun Minuten, Lauf 7). Alle drei Schreib- und fünf Lesepfade nachgezogen. **Offen bleibt der zweite Teil:** die Rümpfe beim Ausliefern gar nicht erst aus der Datenbank zu laden. Das ändert die Seitenlogik des Syncs, und genau dort liegen die dokumentierten Vorfälle — das gehört mit Messwerten von den Boxen gemacht, nicht blind.
> - **Vier Befunde aus dem Selbstcheck über die eigenen Änderungen**, einer davon schwer: die Sperre saß zuerst hinter `ReserveNonce` und hätte jeder Wallet, die an die nur lesende Box geriet, **dauerhaft** eine unbrauchbare Nonce verpasst. Sie sitzt jetzt ganz vorn im RPC-Weg. Außerdem galt sie nur für Überweisungen, während Swap, Liquidität und Faucet denselben Mechanismus offen ließen — jetzt alle sechs Pfade.
>
> **Weiterhin offen und nicht von mir zu schließen:** WP 3 (Tag-7-Prüfung in der App, 12 Sprachen, Coordinator-Endpunkt) ist ein Feature und hängt hinter WP 4, das ≥ 20 echte Registrierungen braucht. Punkt 15 (eigener Signierschlüssel) wartet auf deine Entscheidung.
>
> ## Audit 19.09. — was gefunden und behoben wurde
>
> Alle vier Repos gebaut und getestet: Kette, App (71), Proof-Server (49), Coordinator (103), Matching (185) — **grün**. Die Kette zusätzlich **unter `-race` mit echter Datenbank**, und das erstmals: dabei fiel ein echter Fehler heraus.
>
> **Zwei Fehler, die deinen Blocker 2 direkt treffen** (Registrierung). Beide dieselbe Klasse: Zustand, der nur im Arbeitsspeicher *eines* Knotens liegt, während die App seit v1.7.2 ausweichen kann.
>
> 1. **Der Beweis liegt auf Knoten 1, die Registrierung ging an Knoten 2.** Die Klebrigkeit sollte das verhindern, aber ein Netzfehler in *irgendeinem* Aufruf — ein Kontostand, der im Hintergrund nachlädt — wechselt den aktiven Knoten. Schlimmer: ein Blip auf `/register` selbst ließ den Ausweichknoten die Anfrage dorthin schicken, wo der Beweis nie war — er *erzeugte* den Fehler, den er verhindern sollte. Der Mensch bekommt „this proof did not come from a verified registration on this node" und darf die Gesichtsaufnahme wiederholen. **Behoben:** `/register` geht an den Knoten, der den Beweis ausstellte, und wiederholt bei einem Blip dort statt zu wechseln.
>
> 2. **Der Challenge-Nonce lag bei Coordinator 2, die Aufnahme ging an Coordinator 1.** Knapp verfehlt: Nonce-Frist und App-Klebrigkeit sind beide 5 Minuten, aber die App-Uhr startet einen Netzumlauf früher und läuft zuerst ab. War Coordinator 1 beim Ausstellen krank und antwortet bei der Abgabe wieder, geht die Aufnahme an ihn — `nonce_ungueltig`, nach der vollständigen Aufnahme. **Behoben:** gebunden an den Nonce statt an eine Uhr.
>
> Beide stecken in **App 1.7.3 (versionCode 8)** — die muss veröffentlicht und ausgeliefert werden, sonst ändert sich für niemanden etwas. **Das ist dein Klick.**
>
> **Ein echtes Datenrennen auf dem Geldpfad.** Der EVM-Spiegel las Kontostand, Menschen-Flag und Aktivitätszeit ohne Shard-Sperre und konnte ein halb geschriebenes Konto in die V7-Slots schreiben, die jedes `eth_call` liest. `transferConcurrent` schreibt sich genau diese Regel selbst an die Wand; der Spiegel befolgte sie nicht. Behoben.
>
> **Der Übersprungen-Zähler überlebt jetzt den Neustart.** Er lag nur im Prozess — und `/api/wache` färbt bei > 0 rot, also löschte der Neustart nach rotem Alarm den Alarm statt die Divergenz. Es *sah* aus, als hätte er geholfen. Die Summe steht jetzt in `chain_config`, wird im selben `dbTx` wie der Block fortgeschrieben und **nur vom Resync** abgeräumt — dem Vorgang, der die Divergenz tatsächlich behebt.
>
> **Geprüft und in Ordnung** (kein Befund): pprof hängt korrekt nur auf 127.0.0.1; alle Admin-Endpunkte sind token-geschützt mit konstantzeitigem Vergleich; jeder Geld-Endpunkt (Swap, Liquidität, Faucet, Escrow, Guardian) verlangt Signatur, Zeitfenster und Nonce, und die Signatur wird an die behauptete Wallet gebunden; die Herkunftspflicht an `/api/register` ist verdrahtet; keine Geheimnisse im Quelltext, nur `.env.example`; die Marken der Rechtstexte decken sich exakt mit den Feldern, dein Impressum landet also vollständig auf der Seite; die Löschung (Art. 17) behält bewusst nur den Einmaligkeits-Anker, mit eigener Begründung und eigenem Test.
>
> **Nicht abschließend geprüft:** die Innereien des Matching-Service (9.600 Zeilen Bildverarbeitung) jenseits von Regelwerk, Löschpfad und Schwellen-Frage. Die Schwelle bleibt, wie Punkt 4 sagt, unkalibriert — das entscheidet kein Audit, das entscheidet der Zwei-Personen-Test vor der Kamera.
>
> ## Die Staub-Divergenz ist reproduziert — Ursache benannt
>
> **`zwei_produzenten_realdb_test.go` erzeugt sie auf Kommando**, in Sekunden, ohne Boxen. Der Fingerabdruck stimmt bis ins Detail mit dem vom 15.09. überein:
>
> | | Boxen, 15.09. | Test |
> |---|---|---|
> | einzelne Konten weichen ab | ja | 4 von 32 |
> | Differenzen = ganze Überweisungen | 24–64 | 7.100 Mikro = genau eine zu 0,0071 |
> | **Summen beider Seiten gleich** | ja, bis aufs Mikro | ja (640.000 = 640.000) |
> | fehlender Block / Rollback / Flush-Fehler | keiner | keiner |
>
> **Der Mechanismus.** Beide Boxen produzieren Blöcke. Jeder Knoten wendet seine eigenen Überweisungen bei der **Annahme** an — also *bevor* ein Block ihre Reihenfolge festlegt — und die des anderen beim **Nachspielen**. Jeder durchläuft damit eine andere Zustandsfolge. Solange kein Konto leerläuft, ist das folgenlos (Addition kennt keine Reihenfolge). Läuft eines leer, ist es das nicht mehr: der annehmende Knoten hat die Überweisung gegen *seine* Sicht geprüft und angewandt, der nachspielende prüft sie **ein zweites Mal** gegen seine eigene — und überspringt sie, wenn sie dort nicht bezahlbar ist. Ab da sind sich die beiden über dieses Konto dauerhaft uneins. Im Test ist die Zahl je Runde sichtbar und **nicht symmetrisch**: „A übersprang 16, B übersprang 32". Genau diese Differenz *ist* die Divergenz. Dass es um leerlaufende Konten geht, passt zum Lasttest: „guthaben" war dort mit 14.444 der einzige Ablehnungsgrund, der überhaupt auftrat.
>
> **Warum auf den Boxen trotzdem „0 übersprungen" stand.** Der Zähler ist ein `atomic.Int64` im Prozess — **jeder Neustart und jeder Resync setzt ihn auf null**. Die Null vom 15.09. belegt nicht, dass nie übersprungen wurde. Beim nächsten Lastlauf gehört er auf **beiden** Boxen vorher und nachher abgelesen, ohne Neustart dazwischen (`/api/health/combined` → `zustands_ablehnung`). `/api/wache` färbt bei > 0 bereits rot — der Alarm existiert, er wird nur von jedem Neustart gelöscht.
>
> **Was ein Fix heißt — und warum hier keiner mitgeliefert wird.** Das ist kein Rechenfehler, den man an einer Stelle geradezieht. Dieselbe Überweisung wird zweimal gegen zwei verschiedene Zustände auf Bezahlbarkeit geprüft. Sauber zu schließen ist das nur an der Wurzel: **Zustand ändert sich ausschließlich beim Anwenden eines Blocks, die Annahme reiht nur ein.** Das ist die übliche Bauform einer Kette, ein großer Eingriff — und nichts für die Nacht vor einem Start. Deine Entscheidung, nicht meine. Ausdrücklich *nicht* tun: das Überspringen wieder in ein `hardFailure` zurückdrehen — das war der Zustand vor dem 05.09. und kostete sechs Minuten Stillstand an einer Wand, die sich nie von selbst auflöst.
>
> **Für den Beta-Launch:** 18 Menschen mit echten Beträgen laufen nicht leer, und ohne leerlaufende Konten greift der Mechanismus nicht — die Menschen-Konten waren am 15.09. auf beiden Boxen identisch, das passt. Der Resync stellt beide Boxen gleich, die Wache meldet den Rückfall.
>
> **Das Experiment ist gelaufen — und hat einen anderen Fehler gefunden (belegt, behoben).** Es brauchte keine Boxen: `x/humanity/keeper/annahme_gegen_nachspielen_test.go` spielt beide Knoten in einem Prozess — Knoten A nimmt die Überweisung an, Knoten B spielt sie als Block nach — und vergleicht danach **jedes Feld, das über Geld entscheidet**, nicht nur den Kontostand. Ergebnis: die **Demurrage-Uhr des Empfängers** lief auf beiden Seiten auseinander, um volle **300 Tage**.
>
> Die Regel steht zweimal wörtlich im Code („Empfangen startet die Uhr, es setzt sie nie zurück"). **Drei** Pfade hielten sich daran, **fünf** nicht: der Bündler, der Shard-Schnellpfad, der WAL-Schnellpfad, die WAL-Wiederherstellung und — auf der anderen Seite — das **parallele Nachspielen**. Die Trennlinie lief also quer durch Annahme *und* Nachspielen: ob die Uhr eines Empfängers zurückgesetzt wurde, hing davon ab, welcher Schnellpfad auf der annehmenden Box gerade zuständig war, und auf der nachspielenden davon, ob die Überweisung zufällig in einem bündelbaren Lauf lag. Alle fünf sind auf die Regel gezogen; volle Keeper-Suite grün, auch unter `-race`.
>
> **Warum es niemand sah:** `LastActivityAt` steht absichtlich nicht im `accountLeaf` — die Abweichung geht nicht in den StateRoot ein, also kann kein Wächter sie melden. Sie wird erst zu Geld, wenn `settleDemurrageLocked` daraus zwei Kontostände rechnet. Gemessen im Test: bei 1.500 AEQ und hundert Tagen sind das **24,40 AEQ Unterschied** auf demselben Konto. Nebenbei war es ein Leck: wer die Uhr durch Empfangen zurücksetzen kann, hält jedes Vermögen verfallsfrei, indem ihm alle drei Monate jemand ein Mikro-AEQ schickt.
>
> **Was das NICHT erklärt:** den Lasttest-Staub. Demurrage greift erst oberhalb des fair share von 1.000 AEQ, die Wegwerfkonten halten Bruchteile davon — dieser Fehler kann sie nicht berührt haben. Die Staub-Divergenz bleibt offen (P0, unten). Der neue Befund ist ein eigener, launch-relevanter Konsensfehler, den das Experiment auf dem Weg dorthin gefunden hat.
>
> **Was die Spurensuche ergab (belegt):** beide Boxen haben dieselben Blöcke; jede angenommene Überweisung steckt in genau einem Block (1.307.700 = 1.307.700); jeder Block wird genau einmal nachgespielt; keine Rollbacks, keine übersprungenen Überweisungen, keine Flush-Fehler — und trotzdem weichen einzelne Konten um ganze Bündel (24–64 Überweisungen) ab. Es ist ein Unterschied im **Rechenweg** zwischen Annahme (Produzent) und Nachspielen (Partner), nicht ein verlorener Block. **Alle drei Kandidaten sind jetzt gemessen, zwei davon negativ.** Das Experiment läuft auch gegen **zwei echte Postgres-Datenbanken** (`annahme_gegen_nachspielen_realdb_test.go`): Knoten A nimmt über die echten Schnellpfade an, der Block wird aus dem Ausgangskorb in genau der Reihenfolge gebildet, die `ProduceBlock` liest, Knoten B spielt nach — danach wird jedes Konto in Mikro-AEQ verglichen. Ergebnis: **25.600 Überweisungen mit absichtlich unfreundlichen Beträgen (1/3, 0,0000005) → 0 Konten abweichend.** Und in dem Zustand, der den Lasttest ausmacht — Konten, die leerlaufen (Startguthaben 0,02; 17.576 schon bei der Annahme abgelehnt) — **8.024 Überweisungen → 0 abweichend, 0 übersprungen**. Damit scheiden **Rundung im Batch-Pfad** und **Reihenfolge** (Ausgangskorb-Ordnung ist nicht Annahme-Ordnung) aus. Auch die WAL-Asymmetrie von C2 wurde nachgebaut (Knoten A fährt `AEQUITAS_WAL_ENABLED=1`, belegt 21.831 Überweisungen über den WAL-Schnellpfad): **0 abweichend.** Bis dahin **keine Lastläufe** (18 Menschen erzeugen Bruchteile davon; die Kette ist für echte Nutzung konsistent — Menschen-Konten identisch).
>
> **Nebenbefund — ✅ behoben 19.09.:** `ProduceBlock` markierte bis zu `blockTxCap()` Ausgangskorb-Zeilen *vor* den Produktionstoren; bricht ein Tor ab, blieben sie bis zum stündlichen Sweep liegen (Nachlauf bis 60 min). Jetzt werden genau die geladenen IDs sofort freigegeben.
>
> **C2s Platte:** iostat im Leerlauf `w_await` 6–11 ms, `f_await` 5–20 ms (C1: 1–2 / 0,4–1,4 ms) auf einer NVMe. Test: `mount -o remount,nodiscard /` auf C2, dann iostat; sonst VM-Reboot, dann Contabo-Ticket mit diesen Zahlen.

> **Launch-Linie seit dem 13.09. nachmittags (deine Entscheidung: „Phase 1 muss umgesetzt werden — 1 Mensch,
> 1 Registrierung"): Phase 1 = die Gesichtsprüfung ist das Tor.** `BIO_ATTESTATION_MODE=required` wieder auf beiden
> Proof-Servern (15:13, Quorum 2), die Kette nimmt an `/api/register` nur Nullifier aus einem `/api/prove` desselben
> Knotens (`prove_provenance.go`, Voreinstellung an). Die Phase-0-Linie vom Morgen (Gerätegeheimnis, App
> app-v0.3.1-phase0, `optional`) ist damit zurückgenommen; **die Phase-0-APK darf nicht ausgeliefert werden** — sie
> würde mit `required` jede Registrierung abgewiesen bekommen. Die Website (12 Sprachen, Landing + Explorer +
> Registrierungsseite) nennt jetzt Tor **und** Grenzen: Altkonten ohne Gesichts-Template, unkalibrierte Schwelle,
> Kopfdreh-Lebendigkeit, MPC im Schatten. Jeder Punkt hat ein „fertig heißt" — etwas, das man
nachmessen kann. Was nicht messbar ist, steht nicht drin. Wer hier „✅" setzt,
hat gemessen, nicht geglaubt.

**Launch heißt:** Fremde registrieren sich in der App, bekommen AEQ, nutzen es;
„ein Mensch = ein Konto" hält; wer will, betreibt einen Validator nach
Anleitung; die Betreiber erfahren, wenn etwas kaputtgeht.

## Blocker (ohne die kein Launch)

| # | Punkt | Fertig heißt | Stand | Wer |
|---|---|---|---|---|
| 1 | **Beide Validatoren einig über jeden Kontostand** | `divergenz.abweichend=false` auf beiden Boxen, `uebersprungene_ueberweisungen=0` | ✅ seit 22.09.: nur Contabo1 nimmt an (`ANNAHME_ROLLE=nur_lesend` auf C2, live nachgeprüft — C2 lehnt Überweisungen ab, C1 nicht), Resync, Wache und Prüfstand grün. 23.09.: die Schnellpfade erhalten die Geldmenge gegen echte Postgres (CI), und der Batch-Weg verbraucht auf C2 keine Nonces mehr. Die Wurzel (Ausführung in kanonischer Reihenfolge) bleibt eine spätere Entscheidung | — |
| 2 | **Registrierung funktioniert** | App (Gesichtsprüfung) → Coordinator (Quorum 2) → `/api/prove` → Proof-Server `required` → `register_human` in einem Block; Wache grün | ⚠ Kette, Proof-Server (`/api/prove` ohne Bescheinigung → 403, gemessen) und beide Coordinatoren stehen; **die Website liefert seit 14.09. 19:05 app-v1.7.2** (beide Boxen, sha256 `cba628…`, Release-Schlüssel `473e9d…`; Coordinator proof1 → proof2; 429-Wiederholung; Ausweichknoten proof2 für API/RPC). Seit dem 25.08. hat genau **eine** Person den Weg durchlaufen (`gallery_test: 1`) — mit Quorum 2 noch niemand nachweislich. 14.09.: Coordinator → beide Vergleichsdienste → Quorum live durchgespielt (Nicht-Gesicht → `capture_failed` von beiden, keine Bescheinigung); alle 14 App-Endpunkte + RPC antworten ≤ 0,2 s; **Gruppen hinter einer IP sperren sich nicht mehr gegenseitig** (`ip_burst.go`) | **du:** eine Registrierung mit einem frischen Wallet durchlaufen; Beleg: `total_humans` +1, `gallery_test` +1 auf proof1 **und** proof2 |
| 3 | **Impressum & Datenschutzerklärung** | `/impressum` und `/datenschutz` antworten 200 | ❌ beide 404 — alle sieben `LEGAL_*`-Angaben fehlen (`/api/legal-status`). Seit 14.09. ein Klick: `rechtstexte-setzen.yml` (7 Felder, schreibt beide Boxen, startet die Knoten nacheinander neu, prüft 200). Fußzeilen beider Seiten verweisen auf Impressum/Datenschutz, sobald die Seiten antworten (§ 5 DDG, zwei Klicks) | **du:** die sieben Felder — Name, Anschrift, E-Mail, Verantwortliche(r), Aufsichtsbehörde |
| 4 | **Ein Mensch = ein Konto** | Dieselbe Person, zweites Gerät → `duplicate`; andere Person → durch. Schwelle kalibriert. Altkonten mit Gesicht nachgezogen | ⚠ Tor scharf (`required` seit 13.09. 15:13, Quorum 2, Herkunftspflicht an `/api/register`). **Nachzieh-Weg gebaut und live** (`POST /nachziehen` auf proof1 + proof2, Wallet-Signatur + Ketten-Abgleich + Quorum, keine Prägung; App-Seite in v1.7.0 committed, Identity-Tab „Gesicht nachziehen"). Offen: Schwelle nie mit echten Menschen kalibriert; **die 18 bestehenden Menschen haben kein Gesichts-Template**, bis sie nachziehen (auf der Website benannt); `SERVICE_MODE=test` (Wechsel auf `real` erst mit Galerie-Übernahme, `wuerde_realmodus_abweisen`) | **du + eine zweite Person** vor der Kamera nach `docs/DOPPELREGISTRIERUNG_TEST.md` (Schritte 1–7); danach die 18 einmal durch den Nachzieh-Weg (App v1.7.0) |
| 5 | **DSGVO Phase 2** (Phase 1) | `ALLOW_REAL_BIOMETRIC_DATA=true` + `LEGAL_SIGNOFF_DATE` gesetzt, Drittland (Railway in `sfo`) entschieden | ❌ Unterlagen liegen in `aequitas-biometric-beta/docs/dsgvo/`; Entscheidung offen | **du / Jurist** |
| 6 | **Betreiber merkt, wenn es brennt** | Alarm binnen Minuten | ✅ 23.09.: Wachhund auf beiden Boxen, alle 5 min gegen `/api/wache`, meldet über ntfy (sofort bei Rot, dann alle 6 h, und bei Grün). Testnachricht auf beiden Boxen zugestellt. Dazu `wache.yml` (GitHub, Mail) und `pruefstand-live.yml` auf Knopfdruck | **du:** ntfy-App, Kanal abonnieren (Name im Chat) |

## Wichtig, aber kein Blocker

| # | Punkt | Fertig heißt | Stand | Wer |
|---|---|---|---|---|
| 7 | Neuer Validator nach Anleitung | Fremde Box folgt der Kette und produziert nach Bestätigung; heilt sich selbst | ✅ Weg viermal auf einem Runner geprüft (zuletzt 13.09. 23:15 gegen den Phase-1-Stand: Spitze erreicht, 0 eigene Blöcke). **Selbstheilung:** Rückstand, Stillstand, hängender Block, Totmann **und seit 13.09. belegte Kontostand-Abweichung** lösen beim Laien-Validator den Resync vom signierten Snapshot aus (`AEQUITAS_DIVERGENZ_AUTORESYNC=1` in der Compose-Vorlage; auf C1/C2 aus). `/api/wache` überspringt dort den fehlenden Proof-Server. Guides (12 PDFs) erklären `[AUTO-HEAL]`. Echter Beitritt mit registriertem Wallet fehlt | **du:** ein VPS, `docs/VALIDATOR_EINRICHTEN.md` |
| 8 | Backup | täglich grün, Restore geprüft | ✅ seit 12.09. (Zustand ohne Blöcke, 31–61 MB) | — |
| 9 | Coordinator hängt an Railway (USA) | App zeigt auf einen Contabo-Coordinator; App-Repo auf der Phase-1-Linie; v1.7.0 auf der Website | ✅ 13.09. app-v1.7.0, 14.09. app-v1.7.1 und **19:00 app-v1.7.2** veröffentlicht (Release, Notiz nennt den echten Signierer) und auf beiden Boxen ausgeliefert (`host-apk-locally.yml`); app-v0.3.1-phase0 als *zurückgezogen* markiert (Pre-Release + Hinweis). **Railway-Coordinator ist seit 14.09. 03:45 weg (404 „Application not found“)** — v1.6.0-Installationen können nicht mehr registrieren, v1.7.1 braucht ihn nicht; Wache meldet ihn nur noch als Hinweis | — |
| 10 | Node-Guide in allen 12 Sprachen | keine Railway-Anleitung mehr erreichbar | ✅ alle zwölf am 12.09. neu erzeugt (Docker Compose), 0× „Railway" | — |
| 11 | Lasttest-Konten + Messung | ≥ 300 Paare je Box zahlungsfähig; Messung reproduzierbar | ⚠ 14.09. 30 AEQ nachgefüllt → 330 Konten à 0,02 AEQ, **84/81 Paare je Box** (480 von 649 je Box weiter unter 0,001 AEQ). Vier Messläufe in der Nacht (4 min, `accounts-funded.csv`): Ketten-TPS 1.762–3.375, C1 nimmt 5.100–7.400/s an, C2 nur 1.800–2.800/s. Ursache belegt: C2s Replay hält die Schreibsperre 0,3–3,5 s je Block (`[LOCK] block replay held the exclusive state lock`), weil C2s Platte langsamer ist (WAL-fsync p50 7,6 ms / p90 81 ms gegen 3,2 / 25 ms auf C1) **und** weil beide Knoten alle 2 s die letzten 20 Höhen komplett mit Rümpfen voneinander nachladen (CPU-Profil im Leerlauf: `handleBlocks` 36 %, `doSyncOnce` 28 %; der Kopf-Modus `stripped=1` greift nie, weil `tx_root` nicht in `chain_blocks` liegt). Nächster Schritt: `tx_root` speichern + Client überspringt bekannte Hashes vor dem Nachladen — dann neu messen | **du:** mehr AEQ für ≥ 300 Paare; **ich:** Sync-Umbau mit Messplan |
| 12 | App im Play Store | — | ⏸ bewusst offen bis Punkt 5 | du |
| 13 | Dritter Betreiber | Quorum aus drei Haushalten | ⏸ nach Punkt 4 | du |
| 14 | Lebendigkeit gegen Deepfakes | Blitz + Kopfdrehung + Puls als Score, Herkunft als Risiko, gestaffelter Zuschuss (Klassen grün/gelb/rot), Schattenmodus zuerst | ⚠ **WP 1 live im Schatten** (Score, Risiko, Klasse je Registrierung; `coordinator/health → lebendigkeit`). **WP 2 gebaut und ausgerollt, schlafend** (14.09.: Kette `grant_staffel.go` mit Aktivierung 2100, Proof-Server-Durchreichung, Coordinator-Signatur hinter `LEBENDIGKEIT_VERBINDLICH`, App-Weiterreichung; `account_set_xor` beider Boxen vor/nach Deploy byte-gleich). Scharf nur die Kopfdreh-Challenge. Schwellen erst nach ≥ 20 echten Registrierungen (WP 4) | du (Messlauf = Zwei-Personen-Test), ich (WP 3 Coordinator-Teil, WP 4 nach Daten) |
| 16 | App bleibt am Netz, wenn C1 ausfällt | App erreicht Kette und RPC über einen zweiten Knoten | ✅ 14.09.: `proof2.aequitas.digital/api/*` und `/rpc` führen zum C2-Knoten (`contabo2-api-route.yml`, fünf Routen nachgemessen); App 1.7.2 wählt API_BASE → API_FALLBACKS (klebt 15 min, wechselt nur bei Netzfehler; RPC als FallbackProvider). **Die Website selbst bleibt C1** — DNS-Failover wäre deine Entscheidung beim Registrar | — |
| 15 | Signierschlüssel der Validatoren ≠ persönliche Wallet | `RELAYER_PRIVATE_KEY` auf beiden Boxen ein eigener Schlüssel | ⏸ seit 01.09. bekannt (`docs/WAS_DU_NOCH_TUN_MUSST.md` Punkt 3): auf beiden Boxen ist es derselbe Schlüssel wie deine Wallet. Kein Beta-Blocker; ändert die Blockproduktions-Identität — nicht in der Nacht vor einem Start | **du** (Entscheidung + Schlüssel), ich (Umzug als Workflow, wenn du willst) |

## Bewusst nicht auf der Liste

10.000 TPS. Die Kette packte am 12.09. ~7.000/s auf zwei Boxen; in der Nacht 14./15.09. waren es
1.800–3.400/s, weil C2 unter Last hinterherhinkt (Punkt 11). Für den Launch reichen auch
2.000/s um Größenordnungen — 18 Menschen erzeugen heute Bruchteile davon.

## Wie gemessen wird

- Kette: `bash /tmp/zaehler.sh`, `/api/health/combined` (`divergenz`, `zustands_ablehnung`, `proof_server`, `produktions_ausfaelle`).
- Registrierung: `https://proof1.aequitas.digital/health`, `…/matching/health`, Coordinator `/health` und `/inventory`.
- Rechtstexte: `https://aequitas.digital/api/legal-status`.
- Neuer Validator: `neuer-validator-probe.yml`.
- Alles zusammen: `wache.yml` (Actions → Wache).
- Alles, was ein Mensch benutzt, von außen und auf beiden Knoten: `pruefstand-live.yml` (Actions → Pruefstand live, nur lesend).
