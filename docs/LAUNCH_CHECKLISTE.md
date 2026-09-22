# Launch-Checkliste

**Stand 22.09.2026, 20:06. Beide Boxen laufen auf `06d4b7b`. Die Wache ist GRÜN** — zum ersten Mal seit dem 15.09.

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
| 1 | **Beide Validatoren einig über jeden Kontostand** | `divergenz.abweichend=false` auf beiden Boxen nach Volllast, `uebersprungene_ueberweisungen=0` | ⚠ 13.09. 01:40: Resync C1←C2, danach 4 min Volllast (14.000 Annahmen/s, Kette 6.900/s): 0 übersprungen, 0 Divergenz, Wache grün. **Seit 18.09. ist die Ursache der späteren Staub-Divergenz benannt und auf Kommando reproduzierbar** (`zwei_produzenten_realdb_test.go`, siehe oben): zwei produzierende Knoten + leerlaufende Konten. Für 18 Menschen mit echten Beträgen greift sie nicht, für Lastläufe schon. Der Punkt ist damit kein Häkchen mehr, sondern eine bekannte Grenze mit bekannter Bedingung | **du:** entscheiden, ob die Wurzel vor oder nach dem Beta-Launch angegangen wird |
| 2 | **Registrierung funktioniert** | App (Gesichtsprüfung) → Coordinator (Quorum 2) → `/api/prove` → Proof-Server `required` → `register_human` in einem Block; Wache grün | ⚠ Kette, Proof-Server (`/api/prove` ohne Bescheinigung → 403, gemessen) und beide Coordinatoren stehen; **die Website liefert seit 14.09. 19:05 app-v1.7.2** (beide Boxen, sha256 `cba628…`, Release-Schlüssel `473e9d…`; Coordinator proof1 → proof2; 429-Wiederholung; Ausweichknoten proof2 für API/RPC). Seit dem 25.08. hat genau **eine** Person den Weg durchlaufen (`gallery_test: 1`) — mit Quorum 2 noch niemand nachweislich. 14.09.: Coordinator → beide Vergleichsdienste → Quorum live durchgespielt (Nicht-Gesicht → `capture_failed` von beiden, keine Bescheinigung); alle 14 App-Endpunkte + RPC antworten ≤ 0,2 s; **Gruppen hinter einer IP sperren sich nicht mehr gegenseitig** (`ip_burst.go`) | **du:** eine Registrierung mit einem frischen Wallet durchlaufen; Beleg: `total_humans` +1, `gallery_test` +1 auf proof1 **und** proof2 |
| 3 | **Impressum & Datenschutzerklärung** | `/impressum` und `/datenschutz` antworten 200 | ❌ beide 404 — alle sieben `LEGAL_*`-Angaben fehlen (`/api/legal-status`). Seit 14.09. ein Klick: `rechtstexte-setzen.yml` (7 Felder, schreibt beide Boxen, startet die Knoten nacheinander neu, prüft 200). Fußzeilen beider Seiten verweisen auf Impressum/Datenschutz, sobald die Seiten antworten (§ 5 DDG, zwei Klicks) | **du:** die sieben Felder — Name, Anschrift, E-Mail, Verantwortliche(r), Aufsichtsbehörde |
| 4 | **Ein Mensch = ein Konto** | Dieselbe Person, zweites Gerät → `duplicate`; andere Person → durch. Schwelle kalibriert. Altkonten mit Gesicht nachgezogen | ⚠ Tor scharf (`required` seit 13.09. 15:13, Quorum 2, Herkunftspflicht an `/api/register`). **Nachzieh-Weg gebaut und live** (`POST /nachziehen` auf proof1 + proof2, Wallet-Signatur + Ketten-Abgleich + Quorum, keine Prägung; App-Seite in v1.7.0 committed, Identity-Tab „Gesicht nachziehen"). Offen: Schwelle nie mit echten Menschen kalibriert; **die 18 bestehenden Menschen haben kein Gesichts-Template**, bis sie nachziehen (auf der Website benannt); `SERVICE_MODE=test` (Wechsel auf `real` erst mit Galerie-Übernahme, `wuerde_realmodus_abweisen`) | **du + eine zweite Person** vor der Kamera nach `docs/DOPPELREGISTRIERUNG_TEST.md` (Schritte 1–7); danach die 18 einmal durch den Nachzieh-Weg (App v1.7.0) |
| 5 | **DSGVO Phase 2** (Phase 1) | `ALLOW_REAL_BIOMETRIC_DATA=true` + `LEGAL_SIGNOFF_DATE` gesetzt, Drittland (Railway in `sfo`) entschieden | ❌ Unterlagen liegen in `aequitas-biometric-beta/docs/dsgvo/`; Entscheidung offen | **du / Jurist** |
| 6 | **Betreiber merkt, wenn es brennt** | Alarm binnen Minuten | ⚠ `wache.yml` läuft bei GitHub nur alle paar Stunden. `/api/wache` (200/503) auf beiden Boxen prüft Produktion, Tor (`required`, Quorum ≥ 2), Coordinator, Divergenz, Platte, Partner. **Neu 14.09.: `wachhund-telegram-installieren.yml`** — Cron auf beiden Boxen alle 5 min, Telegram bei Rot (sofort, dann alle 6 h) und bei Grün; braucht nur Bot-Token + Chat-ID als Secrets | **du (2 min):** @BotFather → Token; Chat-ID; Secrets `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`; Workflow starten. Alternative ohne Bot: UptimeRobot auf die beiden `/api/wache` |

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
