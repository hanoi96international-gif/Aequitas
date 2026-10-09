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
   `RetryRegistrationRecoveries` (alle 5 Minuten) prüft die Pause nicht. Fix:
   die Wiederholung an `annahmePauseGrund()` koppeln. (Auf einem Beobachter
   sperren seit #322 `beobachterOhneAusgang` in `runAtomicWithOutbox`,
   `runAtomicDistributionWithOutbox`, `RegisterHumanAtomic` und
   `RegisterHuman` sowie `RetryRegistrationRecoveries` und der
   WAL-Wiederanlauf den Ausgang; Überweisungen sperrt `annahmeBeginnen`.)
8. **Folger nimmt beim Start bis zu 30 s ohne Leitung an** (Prüfung von
   #322, INFO-7; bestand schon vorher): Bis `StarteLeitung` läuft, ist
   `cs.leitung` nil und `nimmtAnFuer()` true, auch mit `AEQUITAS_LEITUNG=an`;
   Überweisung, Tausch und Faucet nehmen dann lokal an, statt
   weiterzuleiten. Fix: pausieren, solange `leitungAn()` und der Start der
   Leitung noch nicht versucht wurde (versucht, nicht gelungen -- sonst
   hielte eine kaputte Leitung den Knoten für immer an).
9. **WAL nach einem Rollenwechsel ohne Deckungsprüfung** (Prüfung von
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
- Die Freiliste wird etwa einmal je Minute neu aufgebaut: wer den Satz
  verlässt, bleibt bis zu 60 s frei, neue Mitglieder sind bis zu 60 s
  begrenzt.
- **Geteilte Karten der Grenzen je Absender** (Prüfung von #324, LOW-6;
  `begrenzte_karte.go`): Seit #324 halten `registerRateLimit`, `ipBurst`,
  `rpcRateLimit` und `walletRateLimit` je höchstens 200.000 Schlüssel; voll
  heißt, neue Absender werden begrenzt. Ein Absender belegt aber je Funktion
  einen Schlüssel – in `registerRateLimit` bis zu zehn, in `ipBurst` bis zu
  fünf. 20.000 bzw. 40.000 Absender (mit IPv6 je /64: ein bis zwei /48)
  füllen eine Karte, danach sind alle neuen Absender dieser Karte gesperrt,
  solange der Angreifer etwa 3.300 Anfragen je Sekunde hält. Die
  Validator-Bindung hat seit dem Folge-PR eine eigene Karte
  (`bindungRateLimit`), die frei gewählten Wallets ebenso. Fix: ein Eintrag
  je Absender mit den Werten je Funktion; bei voller Karte gröber zählen
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
  einen Zähler. Abhilfe im Betrieb (siehe unten): IPv6 im Docker-Netz
  einschalten oder `"userland-proxy": false`.
- **Proof-Server zählt einen Knoten als einen Absender** (Prüfung von #324,
  Nebenbefund): Der Proof-Server begrenzt je IP (`PROVE_RATE_MAX` = 30 je
  Minute, `server.js`) und sieht nur den Knoten. Der Knoten lässt 12 je
  Minute und Absender durch – drei Absender leeren das gemeinsame Budget.
  Fix: Grenze am Knoten auf das Budget des Proof-Servers abstimmen oder den
  Proof-Server je Wallet zählen lassen (der Knoten reicht sie signiert
  durch).

## Betrieb – bei dir
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
- Außerdem aus `ERINNERUNG.md`: C2 / zweiter unabhängiger Betreiber,
  App 1.10.0 als Release, Altersmodell, `PROOF_SERVER_URLS` und
  `CHAIN_SERVICE_TOKEN` auf dem Server, Impressum und Datenschutz,
  Verantwortlicher, DSFA-Punkte, Kanzlei, Ökonom, Pilot.
