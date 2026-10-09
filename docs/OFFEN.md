# Offen – Übergabe (Stand 08.10.2026)

Kurzliste für die nächste Sitzung. Die vollständige Liste steht in
`docs/ERINNERUNG.md`; diese Datei sagt, **wo die Arbeit gerade steht**.

## Erledigt seit dem 07.10.
- #303 Validator-Register Schritt 3, Teil 1 (Verlauf, Erzeuger mit Frist,
  Strafkonto, Leitung) – gemergt.
- #305 WAL-Testaufbau (Datenrennen beim Stopp) – gemergt.
- #306 Geldstrafe später und für alle gleich abrechnen (M1/M2/L3) – gemergt.
- #304 `VALIDATOR_BETREIBER_WALLET` im Verifier-Skript, Fehlermeldungen – gemergt.
- proof-server #11 und biometric #38 (Schlüssel kleiner Ordnung,
  Besitznachweis nur für den Betreiber) – gemergt.

## In Arbeit (andere Sitzung, offen als PRs)
- **#312 Bündel** aus #307–#311 (ein Deploy statt fünf), CI grün:
  - #307 L1: kein spät eingehängter Block mit Bindung oder Beweis (schlafend)
  - #308 Schritt 3, Teil 2: Validatoren-Belohnung aus der Kette (schlafend)
  - #309 GHOSTDAG: K nicht mehr aus einem lokalen Komitee (wirkt sofort,
    K bleibt 18)
  - #310 Coordinatoren nach dem Register zulassen (Baustein, schlafend;
    H1 offen: eine Bindung kostet nichts)
  - #311 Coordinator-Nachrichten mit Chain-ID (v2)
  - Offen vor dem Merge: getrennte Sicherheitsprüfung der Korrekturen im
    Bündel (läuft), dann Merge.
- Mein Zweig `claude/weiter-gehts-r7rb9w-register3b` (Teil 2) ist durch #308
  **überholt** und wird nicht weiterverfolgt.

## Code – danach
1. **Komitee** (`getEpochCommittee`) aus derselben Menge wie die Belohnung
   statt aus lokal bekannten Adressen.
2. **#310 H1:** Zulassung von Coordinatoren braucht einen Preis
   (eine Bindung kostet nichts) – vor dem Staffel-Stichtag.
3. **Vor dem strengen Modus: Anwesenheit aus dem Vergangenheitskegel**
   eines Ankerblocks zählen statt aller Blöcke im Zeitfenster (#308 zählt
   `chain_blocks` im Fenster: ein Knoten mit einem Block mehr oder weniger –
   etwa an der Finalitätswand – rechnet anders und wiese im strengen Modus
   die Runde ab). Mit dem Kegel zählen nur Vorfahren des Ankers; fehlt einem
   Knoten einer, rechnet er nicht nach statt falsch. Bausteine liegen in
   `claude/weiter-gehts-r7rb9w-register3b` (`bloeckeImKegel`,
   `validatorAnkerWaehlen`, Tests).
4. **Strenger Modus** spätestens mit der Staffel; Stichtag
   `nachrechnenStrengAbUnix`, sobald `/api/wirtschaft/regeln` über mehrere
   Runden 0 Abweichungen zeigt.
5. Offener Betrieb (ohne `AUTHORIZED_VALIDATORS`) mit über 1.000
   Validatoren: fortgeschriebener statt neu gelesener Stand (Erzeugerprüfung
   und Anwesenheit).
6. **Pausierter Leiter übergibt nicht** (Prüfung von #322, INFO-1; bestand
   schon vorher): Ein Leiter, der nach einem Neustart wieder übernimmt,
   pausiert bis zu seinem ersten eigenen Block; so lange lehnen die
   Leiter-Wege netzweit wiederholbar ab, und bei zwei Validatoren ohne
   `AEQUITAS_LEITUNG_ZWEI_WECHSELN` beendet kein Wechsel das. Fix: eine
   Pause länger als `admissionStallLimit()` im Takt wie einen verlorenen
   Leistungsnachweis behandeln, also übergeben. (Ein Beobachter baut seit #322
   keine Leitung mehr auf und legt nichts in den Ausgang.)
7. **Einträge in `pending_txs` ohne Annahme-Pause** (Prüfung von #322,
   INFO-2; bestand schon vorher): `DoppelsignaturErkannt` (slashing.go,
   ausgelöst von Peer-Blöcken) legt den Strafbeweis auch auf einem Beobachter
   oder pausierten Knoten in den Ausgang; läuft `strafBeweisFrisch` ab, bevor
   der Knoten erzeugt, helfen nur Resync oder Verwerfen.
   **Erledigt (Zweig `claude/weiter-gehts-hklfhc`):**
   `RetryRegistrationRecoveries` holt nach der EVM (Hash gesetzt) nichts
   nach, solange `annahmePauseGrund()` auf dem annehmenden Knoten pausiert
   (wie `/api/register`). Und (Prüfung von #329, INFO-13): Vor-EVM-Intents
   (`evm_tx_hash = ''`) registriert die Wiederholung **nie** mehr –
   `vorEVMIntentAufloesen` schließt sie nur: Go-Zustand hat den Menschen →
   erledigt; EVM-Spiegel zeigt ihn, Go nicht → bleibt offen für den
   Betreiber (Degraded-Hinweis); sonst → verworfen, der Nutzer reicht neu
   ein. Erst ab 10 Minuten Alter (sonst evtl. noch unterwegs); ein später
   gesetzter EVM-Hash öffnet einen verworfenen Intent wieder. Der Durchlauf
   liest höchstens 1.000 Zeilen. Tests: `registration_recovery_test.go`.
   Offen bleibt hier nur der Strafbeweis-Teil (`DoppelsignaturErkannt`).
8. **WAL nach einem Rollenwechsel ohne Deckungsprüfung** (Prüfung von
   #322, INFO-15): Ein Beobachter liest das WAL nicht ein, die Datei bleibt
   liegen. Startet ein abgestürzter Validator mit ungeflushten Sätzen erst
   als Beobachter (spielt fremde Blöcke nach, `wal_seq` bleibt stehen) und
   dann wieder als Validator, wendet `recoverFromWAL` die alten Sätze ohne
   Deckungsprüfung auf den weitergelaufenen Stand an (PoC: Kontostand
   −42,042, eine Zeile im Ausgang). Nur der umgestellte Knoten ist betroffen
   – die anderen prüfen die Deckung beim Nachspielen und lehnten einen Block
   mit dieser Zeile ab; der Workflow-Beobachter startet mit leerem WAL.
   Fix: Ein Beobachter, dessen WAL nicht abgeglichene Sätze trägt (über der
   Untergrenze, `seq` > `wal_seq` der Konten; rein lesend per
   `wal.ReplayFile`), startet nicht (fail-closed), mit dem Hinweis, erst als
   Validator wiederanlaufen zu lassen oder den Rest bewusst zu verwerfen;
   ergänzend prüft `recoverFromWAL` die Deckung und parkt Sätze, die ins
   Minus führten. Missbrauchstest wie der PoC. Bis dahin (Betrieb): einen
   Validator mit WAL nie als Beobachter neu starten, ohne dass er vorher als
   Validator sauber wiederangelaufen ist.

## Ratenbegrenzung – bekannte Lücken
- **`X-Forwarded-For` hinter einem privaten TCP-Partner** (Prüfung von #319,
  LOW-3; seit #324 zählt der letzte Eintrag, IPv6 je /64, und jede Karte der
  Grenzen je Absender hat eine feste Höchstzahl): `clientIP` glaubt dem Kopf
  weiter, sobald die Verbindung von irgendeiner privaten Adresse kommt –
  offen für andere Container im Docker-Netz und vermutlich für
  `docker-proxy` bei Hairpin oder IPv6 auf dem veröffentlichten Port. Fix:
  nur der Adresse des eigenen Proxys glauben (feste Adresse statt
  „privat“; das Gateway der eigenen Routen ist seit #325 ausgenommen).
  Zwei anhängende Proxys: siehe unten.
- **Freiliste ohne Herkunftsnachweis** (Prüfung von #320, LOW-3; bestand
  schon vorher, seit #320 auf den Satz eingeengt): ein Satzmitglied kann
  jede öffentliche IP ankündigen und sie so von den Grenzen je IP
  freistellen lassen – auch eine geteilte Ausgangsadresse (Mobilfunk-CGNAT,
  VPN). Ein Leiter kann bei einem plausiblen Satzwechsel über `m.URLs` auch
  die URLs anderer Mitglieder setzen (leitung.go, `empfangeLease`). Fix:
  nur die TCP-Quelle freistellen, von der eine gültig signierte
  `/api/leitung`-Nachricht dieses Mitglieds kam (oder Übereinstimmung mit
  der angekündigten IP verlangen); besser noch Weiterleitungen selbst
  unterschreiben (wie für die Erneuerung, #319 MEDIUM-7).
- **Eigene Netze nur für private Adressen geprüft** (Prüfung von #320,
  INFO-11; Spielart von LOW-3): Eine öffentliche Adresse ist ohne Einstellung
  freistellbar und wird nie gegen die eigenen Schnittstellen geprüft. Liegt
  das Docker-Netz in einem globalen Präfix (IPv6 `fixed-cidr-v6` aus dem /64
  des Providers, öffentliches `bip`) oder hat die Schnittstelle nur eine
  Hostroute (Kubernetes/Calico, eth0 /32), kann ein böswilliges Mitglied das
  Gateway ankündigen; frei wäre dann alles, was ein Weiterleiter ohne Kopf
  (docker-proxy, L4-Balancer, SNAT) von dort zustellt. Das Deploy
  (`deploy/validator/docker-compose.yml`, Bridge ohne IPv6) ist nicht
  betroffen. Fix mit LOW-3: auch die angekündigte Adresse selbst nie
  freistellen, wenn sie eine eigene Adresse ist oder in einem Netz der
  eigenen Schnittstellen liegt (Hostrouten ausgenommen), und denselben
  Filter auf die beobachtete Quelle anwenden. In /32-Umgebungen das Pod- oder
  Knotennetz nie in `AEQUITAS_FREILISTE_NETZE` nennen.
- **Weiterleitungen im Klartext** (Prüfung von #319, INFO-21): Wer zwischen
  Folger und Zuständigem mithört (`http://IP:8080`) und den Ring der
  Merkliste in unter 30 s mit 20.000 gültigen Weiterleitungen leert, kann
  einen Nachweis noch einmal einspielen; er zählt dann unter dem Budget des
  echten Coordinators. Verzögert er Weiterleitungen über das Zeitfenster
  oder spielt sie vor dem Original ein, scheitern deren Nachweise und
  zählen unter der Adresse des Folgers (30 je Minute); gültige Nachweise
  bleiben davon seit #319 (LOW-24) unberührt. Abgelehnte Nachweise je Grund
  zeigt `/api/health/combined` → `weiterleitung_nachweis` (dort auch
  Uhrabweichung und unbekannte Folger). Abhilfe: TLS zwischen Validatoren.
- **Satzmitglieder bestimmen den Absender ihrer Weiterleitungen** (Prüfung
  von #319, LOW-31 und INFO-38; bewusste Annahme): Der Zuständige zählt eine
  Erneuerung mit gültigem Nachweis unter (Folger, `fuer`), und `fuer` setzt
  der Folger. Ein böswilliges Mitglied des aktuellen Satzes kann so
  beliebig viele Zähler aufmachen; gültige Nachweise bekommen ihre
  Prüfbuchung zurück (netto kein Prüfbudget) – es erreicht den Handler
  (ecrecover, Ed25519, Registerabfrage) so oft, wie es Anfragen schickt. Wo
  der Knoten direkt auf :8080 erreichbar ist, gibt ihm die Freiliste
  dasselbe schon (LOW-3 oben); hinter Caddy stellt die Freiliste nie frei,
  dort ist der Nachweis ein zusätzliches Recht. Im offenen Betrieb kommt
  jeder registrierte Mensch mit einem aufgeholten Knoten in den Satz (einer
  je Mensch). Seine Zähler liegen in einer eigenen Karte
  (`erneuerung_weitergeleitet`, höchstens 200.000 Schlüssel; `fuer` nur in
  der Schreibweise von `clientIP`, IPv6 je /64): füllt er sie, bekommen nur
  neue weitergeleitete Erneuerungen 429, bis das Aufräumen Platz schafft –
  kein anderer Endpunkt. Vertretbar, weil ein Satzmitglied die Annahme als
  Leiter ohnehin anhalten kann (Verfügbarkeit hängt am Satz, nicht die
  Richtigkeit). Auch Außenstehende füllen die Karte über ehrliche Folger,
  ein Schlüssel je Absendernetz und Folger (Prüfung von #319, INFO-41 und
  INFO-46): rund 200.000 Netze geteilt durch die Zahl der Folger, über die
  sie gehen, je ein bis zwei Minuten – ab etwa sieben Folgern billiger, als
  `ipBurst` zu füllen; dann bekommen neue weitergeleitete Erneuerungen 429,
  direkte nicht. Ein Kontingent je Folger verteilt das nur (dieselben Netze
  über alle Folger füllen alle Kontingente). Fix, falls nötig: je
  `fuer`-Netz über alle Folger begrenzen oder bei voller Karte gröber zählen
  (/48); eine Grenze je Unterzeichner hätte den Nachteil, dass ein Angreifer
  mit vielen Netzen über einen ehrlichen Folger dessen Grenze leeren kann.
- **Aufräumen: Prüfen und Löschen nicht unter einer Sperre** (Prüfung von
  #319, INFO-48; bestand schon vorher, nur Theorie): `ipBurstAufraeumen`
  entscheidet unter der Sperre des Eintrags und löscht danach; bucht genau
  dazwischen jemand, geht höchstens diese eine Buchung mit dem Eintrag
  verloren. Fix: unter der Sperre mit `CompareAndDelete` löschen und den
  Eintrag als tot markieren (`burstBuchen` lädt dann neu).
- **Weiterleitende Validatoren außerhalb des Satzes** (Prüfung von #319,
  INFO-37; gegenüber vorher keine Verschlechterung): Bloß zugelassene
  Validatoren, Bewerber und gerade entfernte Mitglieder werden nicht
  anerkannt. Ihre Weiterleitungen zählen beim Zuständigen unter ihrer
  Adresse – 30 je Minute für alle ihre Coordinatoren, und ein Angreifer mit
  einer Adresse sperrt darüber alle aus; der Folger selbst sieht nur die
  429. Aufgenommen wird meist in Sekunden; dauerhaft draußen bleiben ein
  zweiter Schlüssel desselben Menschen, ein Schlüssel ohne bekannten
  Menschen, ein Knoten mehr als 50 Blöcke zurück und einer in der
  Aufnahmesperre (10 min). Betrieb: Coordinatoren nur an Knoten richten,
  die in `/api/health/combined` → `leitung` → `mitglied: true` zeigen. Fix:
  ein Knoten außerhalb des Satzes leitet Erneuerungen nicht weiter, sondern
  antwortet 503 mit Hinweis.
- **Gleichzeitig offene gültige Prüfungen** (Prüfung von #319, LOW-35;
  Rest): Auch eine gültige Weiterleitung belegt ihren Platz im Prüfbudget
  des Folgers, bis sie geprüft ist (Körper lesen, ecrecover, Merkliste –
  Mikrosekunden; die Satzprüfung wartet seit #319 auf keine Sperre). Nur
  wer über einen Folger 600 Weiterleitungen im selben Augenblick beim
  Zuständigen ankommen lässt (600 Adressen in einem Schwall), schiebt die
  folgenden für diesen Augenblick in die Zählung des Folgers.
- Die Freiliste wird etwa einmal je Minute neu aufgebaut: wer den Satz
  verlässt, bleibt bis zu 60 s frei, neue Mitglieder sind bis zu 60 s
  begrenzt.
- **Geteilte Karten der Grenzen je Absender** (Prüfung von #324, LOW-6;
  `begrenzte_karte.go`): Seit #324 halten `registerRateLimit`, `ipBurst`,
  `rpcRateLimit` und `walletRateLimit` je höchstens 200.000 Schlüssel; voll
  heißt, neue Absender werden begrenzt. Ein Absender belegt aber je Funktion
  einen Schlüssel – in `registerRateLimit` bis zu zehn, in `ipBurst` bis zu
  sieben (mit Erneuerung und Prüfbudget aus #319). 20.000 bzw. etwa 28.600
  Absender (mit IPv6 je /64: weniger als ein /48)
  füllen eine Karte, danach sind alle neuen Absender dieser Karte gesperrt,
  solange der Angreifer etwa 3.300 Anfragen je Sekunde hält. Eigene Karten
  haben seit #325 die Validator-Bindung (`bindungRateLimit`, Fehlversuche je
  IP) und die frei gewählten Wallets, seit dem Folge-PR auch die Grenze je
  Betreiber (`betreiberRateLimit`; ihre Schlüssel entstehen nur nach einer
  angenommenen, unterschriebenen Bindung) und mit #319 die weitergeleiteten
  Erneuerungen, deren Absender der Folger bestimmt
  (`erneuerung_weitergeleitet`). Rest (Prüfung von #325, LOW-1
  und INFO-6): `bindungRateLimit` füllen ungültige Bindungen aus 200.000 /64
  (drei bis vier /48, 2.100–5.700 Anfragen je Sekunde zum Halten) – danach
  bekommt jeder neue Absender bei der Bindung 429; `walletRateLimit` füllen
  schon 8.000–17.000 /64 (12 Wallets je Minute und Absender) – danach
  bekommt jede neue Wallet bei `/api/prove` 429. Fix: ein Eintrag je
  Absender mit den Werten je Funktion; bei voller Karte gröber zählen
  (/48, /24) statt global zu sperren.
- **Zwei anhängende Proxys** (Prüfung von #324, INFO-4/INFO-9): `clientIP`
  nimmt den letzten Eintrag von `X-Forwarded-For` – richtig hinter genau
  einem Proxy (Caddy). Kommt ein zweiter davor (CDN, Load-Balancer), ist der
  letzte Eintrag dieser Proxy, und alle Nutzer zählen unter ihm. Dann
  braucht es eine Liste vertrauenswürdiger Proxys und den letzten Eintrag
  davor.
- **IPv6 über Caddy zählt unter dem Gateway** (Prüfung von #324, MEDIUM-5):
  `aequitas-net` hat kein IPv6; IPv6-Verbindungen an 80/443 nimmt
  docker-proxy an und reicht sie vom Gateway des Docker-Netzes an Caddy.
  Caddy setzt dann das Gateway als Absender – alle IPv6-Nutzer teilen sich
  einen Zähler. Das trifft auch Weiterleitungen (Prüfung von #319,
  INFO-43): erreicht ein Folger den Zuständigen so, zählen er und jeder
  IPv6-Angreifer unter dem Gateway; 600 gefälschte Nachweise leeren das
  gemeinsame Prüfbudget, und alle gültigen Weiterleitungen fallen in 30 je
  Minute. Abhilfe im Betrieb (siehe unten): IPv6 im Docker-Netz einschalten
  oder `"userland-proxy": false`; Leitungs-URLs (`SELF_URL`) vor der
  Staffel nur über IPv4.
- **Proof-Server zählt einen Knoten als einen Absender** (Prüfung von #324,
  Nebenbefund): Der Proof-Server begrenzt je IP (`PROVE_RATE_MAX` = 30 je
  Minute, `server.js`) und sieht nur den Knoten. Der Knoten lässt 12 je
  Minute und Absender durch – drei Absender leeren das gemeinsame Budget.
  Fix: Grenze am Knoten auf das Budget des Proof-Servers abstimmen oder den
  Proof-Server je Wallet zählen lassen (der Knoten reicht sie signiert
  durch).

- **Netzprüfung zweier Endpunkte hinter docker-proxy** (Prüfung von #325,
  INFO-7): `/api/sign-validator-challenge` und `/api/snapshot` mit
  `SNAPSHOT_RESTRICT_TO_PRIVATE_NETWORK` lassen nur private Quellen zu – das
  Gateway des Docker-Netzes besteht diese Prüfung, und über docker-proxy
  (IPv6, Hairpin) kommt jeder vom Gateway. Bewusst nicht geändert: genau so
  erreicht auch der Betreiber auf dem Host (`curl 127.0.0.1:8080`) den
  Knoten. Es schützen weiter das ausdrückliche Einschalten
  (`ALLOW_SIGN_VALIDATOR_CHALLENGE`, `SNAPSHOT_RESTRICT_…`) und
  `SNAPSHOT_TOKEN`; mit 8080 nur auf IPv4 (siehe unten) entfällt der Weg.

## Betrieb – bei dir
- **`AEQUITAS_LEITUNG=an` und gescheiterter Start** (seit #329, fail-closed):
  Ist der Signierschlüssel (`RELAYER_PRIVATE_KEY`) ungültig – fehlt er,
  erzeugt der Knoten einen – oder `AEQUITAS_LEITUNG_GENESIS` ungültig, nimmt
  der Knoten nichts an (wiederholbar abgelehnt) und führt keine
  Systemaufträge aus, statt lokal anzunehmen. Lesen geht weiter;
  `/api/register` läuft wie auf einem Folger durch die Prüfungen und scheitert
  erst an der EVM-Übergabe (`-32005`, Nonce unverbraucht) – registriert wird
  nur am annehmenden Knoten (Prüfung von #329, INFO-12). Zu sehen in `/api/health/combined` → `leitung` →
  `gescheitert: true` bzw. `annahme_pause` → `leitung_gescheitert`, im Log
  `[LEITUNG] ✗`. Nach der Korrektur neu starten. Während des Starts (bis die
  Leitung läuft) zeigt `annahme_pause` → `leitung_startet`.
- **Proxy nur im Docker-Netz** (Prüfung von #325, LOW-2): Der Knoten glaubt
  `X-Forwarded-For` nie vom Gateway seines Docker-Netzes. Ein Proxy auf dem
  Host selbst (Dienst, `network_mode: host`, über `127.0.0.1:8080` oder die
  Container-IP) kommt aber genau von dort – dann zählen alle Nutzer unter
  einer Adresse und bekommen schnell 429. Den Proxy darum als Container in
  `aequitas-net` betreiben und den Knoten per Containername ansprechen (so
  alle Caddyfiles in `deploy/`). Erkennbar in `/api/health/combined` →
  `grenzen_je_absender.eigenes_gateway.xff_vom_gateway_verworfen` und im Log
  (`X-Forwarded-For vom eigenen Gateway … verworfen`). Dieselbe Meldung löst
  auch ein Client aus, der über docker-proxy (IPv6, Hairpin) kommt und den
  Kopf selbst setzt – steigt der Zähler ohne Proxy auf dem Host, den
  nächsten Punkt prüfen.
- **docker-proxy auf `[::]:8080` prüfen** (Prüfung von #324, MEDIUM-5): auf
  C1 und C2 `ss -ltnp 'sport = :8080'`. Steht dort `docker-proxy` auf
  `[::]:8080`, erreichen IPv6-Clients den Knoten am Proxy vorbei vom
  Gateway aus. Seit dem Folge-PR zu #324 glaubt der Knoten dem Gateway keinen
  `X-Forwarded-For` mehr; trotzdem 8080 nur auf IPv4 veröffentlichen
  (`-p 0.0.0.0:8080:8080`) oder in `/etc/docker/daemon.json`
  `"userland-proxy": false` setzen. Auf Boxen mit Caddy muss 8080 gar nicht
  öffentlich sein.
- `COORDINATOR_BETREIBER_WALLET` und `VALIDATOR_BETREIBER_WALLET` auf den
  Boxen setzen (die Deploy-Workflows übernehmen die alte Umgebung, die neuen
  Variablen kommen nicht von selbst). Ohne sie stellt der Dienst keinen
  Besitz-/Bezeugungsnachweis aus (gewollt, fail-closed).
- Nach dem Deploy von biometric #38: die Coordinator- und
  Personhood-Schlüssel von proof1 **sofort** auf der Kette unter deiner
  Betreiber-Wallet eintragen – oder `COORDINATOR_SIGNING_KEY` /
  `VALIDATOR_SIGNING_KEY` rotieren. Vorher ausgestellte Nachweise gelten sonst
  weiter (der Satz hat kein Datum).
- proof-server #11 vor oder zusammen mit #38 ausrollen (die Workflows lesen
  die Schlüssel jetzt aus `/coordinator/inventory` und `/matching/health`).
- `coordinator_keys` / `validator_keys` auf C1 auf Schlüssel kleiner Ordnung
  prüfen; vor einem Image-Update der Verifier deren `.env` prüfen.
- Stichtage (eure Entscheidung): `validatorRegisterAb`, `registerLeserAb`,
  `erzeugerSchnittAb` (erst wenn `/api/status` → `erzeuger_ohne_bindung`
  leer ist), Staffel, strenges Nachrechnen. Nur geschlossener Betrieb
  (`AUTHORIZED_VALIDATORS`) ist für die Register-Stichtage freigegeben.
- Ein Beobachter (`AEQUITAS_BEOBACHTER=1`) darf in keinem Leitungs-Satz
  stehen (`AEQUITAS_LEITUNG_GENESIS` anderer Knoten, registrierter
  Validator): er baut keine Leitung, die anderen hielten ihn für ein
  Mitglied, das nie antwortet (bei zwei Validatoren nimmt dann niemand an).
  Er warnt beim Start laut, wenn er seine Adresse im Genesis-Satz findet.
- Freiliste der Ratenbegrenzung (#320): Validatoren, deren `SELF_URL` ein
  Name (`https://<domain>`) oder eine private/Tailscale-Adresse ist, werden
  nicht mehr freigestellt; ihre Weiterleitungen laufen beim Leiter in die
  Grenze je IP. Private Netze nur gezielt freigeben:
  `AEQUITAS_FREILISTE_NETZE=100.64.0.0/10` (CIDR, durch Kommas getrennt),
  nie das Docker-Netz des Knotens (Gateway, docker-proxy, Proxy). Der Knoten
  verwirft selbst, was zu weit ist (IPv4 kürzer als /8, IPv6 kürzer als /16)
  oder ein Netz seiner eigenen Schnittstellen berührt, und sagt es einmal im
  Log (`AEQUITAS_FREILISTE_NETZE: … -- ignoriert`). Abgewiesene Adressen
  stehen einmal je Mitglied im Log (`[LEITUNG] ⚠ … wird nicht von der
  Ratenbegrenzung freigestellt`). Anfragen mit `X-Forwarded-For`,
  `Forwarded` oder `X-Real-IP` sind nie freigestellt.
- Staffel-Stichtag (#319, INFO-22): erst setzen, wenn **alle** Validatoren
  eine Version mit unterschriebenen Weiterleitungen fahren. Ein alter Folger
  unterschreibt nicht; steht der Zuständige hinter Caddy, zählt er dessen
  Weiterleitungen unter der Adresse des Folgers, und ein einzelner Absender
  sperrt dann alle Coordinatoren dahinter aus.
- Außerdem aus `ERINNERUNG.md`: C2 / zweiter unabhängiger Betreiber,
  App 1.10.0 als Release, Altersmodell, `PROOF_SERVER_URLS` und
  `CHAIN_SERVICE_TOKEN` auf dem Server, Impressum und Datenschutz,
  Verantwortlicher, DSFA-Punkte, Kanzlei, Ökonom, Pilot.
