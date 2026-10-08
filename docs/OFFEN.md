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

## Ratenbegrenzung – bekannte Lücken
- **`clientIP` und IPv6** (Prüfung von #319, LOW-3; bestand schon vorher, gilt
  für alle Grenzen je IP): Jede IPv6-Adresse hat einen eigenen Zähler – aus
  einem /64 kamen 100 von 100 Anfragen durch. Fix: IPv6 je /64 zählen.
- **`X-Forwarded-For` hinter einem privaten TCP-Partner** (ebenda):
  `clientIP` nimmt den ersten Eintrag des Kopfes ungeprüft, sobald die
  Verbindung von einer privaten Adresse kommt. Über Caddy ist das dicht
  (Annahme: Caddy überschreibt den Kopf ohne `trusted_proxies`; Version auf
  C1 prüfen). Offen ist es für andere Container im Docker-Netz und
  vermutlich für `docker-proxy` bei Hairpin oder IPv6 auf dem
  veröffentlichten Port – dort kamen 1000 von 1000 Anfragen mit rotierendem
  Kopf durch. Fix: nur der Adresse des eigenen Proxys glauben (feste
  Adresse statt „privat“) und den letzten, nicht den ersten Eintrag nehmen.
- **`ipBurst` ohne feste Obergrenze** (Prüfung von #319, LOW-4; bestand schon
  vorher): Die Einträge verfallen nach etwa 120 s (Aufräumen alle 60 s),
  ihre Zahl ist aber nur durch Anfragerate × 120 s begrenzt – gemessen
  228 B je Schlüssel, bei 5.000 Anfragen/s etwa 137 MB. Fix: Obergrenze für
  die Zahl der Schlüssel; voll heißt begrenzen.
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
  echten Coordinators. Abhilfe: TLS zwischen Validatoren.
- Die Freiliste wird etwa einmal je Minute neu aufgebaut: wer den Satz
  verlässt, bleibt bis zu 60 s frei, neue Mitglieder sind bis zu 60 s
  begrenzt.

## Betrieb – bei dir
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
