# marketsignals

Ein eigenständiges Go-Modul für Marktsignal-Agenten: neun spezialisierte
Agenten, ein Ensemble, das nur bei unkorrelierter Übereinstimmung spricht, ein
Risikomanager und eine Backtest-Engine, die bewusst schwer zu betrügen ist.

Dieses Modul ist vollständig unabhängig — eigene `go.mod`, nur Go-Standard-
bibliothek, keine Verbindung zum restlichen Repository.

## Was das hier ist — und was nicht

**Es ist nicht:** ein System, das zuverlässig Geld verdient. Solche Systeme
werden verkauft, nicht verschenkt, und die meisten funktionieren nicht. Edges
sind klein, sie verfallen, und Gebühren plus Slippage fressen sie auf.

**Es ist:** die Infrastruktur, mit der man ehrlich herausfindet, ob ein Signal
überhaupt etwas taugt. Der eigentliche Wert liegt nicht in den Indikatoren —
die sind bekannt — sondern in der Bewertung, die sich nicht selbst belügt.

Das Modul platziert **keine Orders**. Es hält keine Keys und spricht mit keiner
Börse.

## Anlageklassen — getrennt behandelt

`ETF`, `Aktie` und `Krypto` sind keine Etiketten, sondern unterschiedliche
Annahmen. Krypto ist zusätzlich in Sektoren geteilt, weil "Krypto" kein Markt
ist: Bitcoin und ein zwei Tage alter Memecoin teilen eine Settlement-Schicht
und sonst nichts.

Ein Experten-Profil (`experts.go`) ist kein eigener Algorithmus pro Sektor —
ein Breakout bleibt ein Breakout. Unterschiedlich ist alles *drumherum*:

| Segment | Kosten/Einheit | Max. Position | Besonderheit |
|---|---|---|---|
| ETF | 3bp | 100% | Mean Reversion erlaubt (ein Korb hat einen Mittelwert) |
| Aktie | 3bp | 50% | Reversion gestrichen (Einzeltitelrisiko) |
| Krypto Major | 9bp | 100% | Voller Agentensatz, Funding nur mit Perp |
| Liquid Alt | 21bp | 50% | Reversion gestrichen |
| Narrative (AI/RWA) | 33bp | 30% | Social-Agent aktiv, 80% der Folds müssen positiv sein |
| Memecoin | **210bp** | 10% | Reversion undefiniert, Social-Agent, 50 Trades Minimum |
| Neuer Launch | — | — | **Nicht handelbar** — zu wenig Historie |
| Stablecoin | — | — | **Nicht handelbar** — Peg-Bruch nicht im Sample |

Die Memecoin-Kostenannahme von 210bp pro Runde ist die wichtigste Zahl der
ganzen Tabelle. Bei diesen Kosten braucht eine Strategie einen sehr großen
Edge, nur um bei null herauszukommen — und die meisten Memecoin-"Edges"
verschwinden genau dort vollständig.

Zwei Segmente werden explizit **verweigert**. Das ist eine nützlichere Ausgabe
als eine selbstbewusste Zahl aus dem Nichts: bei einem Stablecoin misst ein
hoher Sharpe die Abwesenheit des Peg-Bruchs, nicht seine Unwahrscheinlichkeit.

```bash
go run ./cmd/signalctl experts     # alle Profile mit Begründung
```

## Die sieben Agenten

| Agent | Familie | Idee | Warum verteidigbar |
|---|---|---|---|
| `breakout` | trend | Donchian-Ausbruch, ATR-normiert, mit Trendfilter | Leverage-Kaskaden erzeugen Persistenz; der ATR-Filter verhindert Whipsaws in der Range |
| `reversion` | reversion | Fadet Erschöpfung — nur mit Rejection-Wick, nur im Seitwärtsregime | Handelt *erzwungene* Verkäufer, nicht bloß schwache Preise |
| `flow` | flow | Divergenz zwischen Preisextrem und kumulativer Taker-Delta | Einziger Agent, der früh statt bestätigend ist |
| `funding` | positioning | Perp-Funding-Extreme über Perzentilrang | Misst Überfüllung statt Preis — Fehler unkorreliert zu den anderen |
| `fibonacci` | structure | Retracement-Zone, die *gehalten* hat | Level wirken durch Aufmerksamkeit, nicht durch Magie — daher nur mit Rejection |
| `pattern` | structure | Doppeltop/-boden, SKS, Dreiecke — erst beim Bruch | Trigger steht *vor* dem Bruch fest, nicht danach |
| `macro` | macro | Politik-/Konjunkturkalender | Moduliert primär Risiko statt Richtung |
| `social` | social | Glaubwürdige X-Aufmerksamkeit | Überwiegend **contrarian** — Aufmerksamkeit ist meist Ausstiegsliquidität |
| `launch` | screen | Risiko-Screener für neue Token | Beantwortet nicht "wohin", sondern "überhaupt anfassbar?" |

Alle Markt-Agenten haben **bewusst ungetunte** Parameter (runde Zahlen).
Auf denselben Daten optimieren, auf denen man auch bewertet, ist Curve-Fitting.

### Politik: Überraschung, nicht Ausgang

Der Macro-Agent sagt **keine** Wahlausgänge, Zinsentscheide oder Urteile
voraus. Das ist nicht prognostizierbar, und ein System, das darauf wettet,
handelt nicht, sondern spielt. Stattdessen:

- **Vor** einem terminierten binären Ereignis: Risiko runter, bis auf null.
  Ein Referendum in zwei Stunden macht einen Long nicht besser oder schlechter
  — es macht *jede* Größe falsch.
- **Nach** dem Ereignis: mit der *Überraschung* lehnen (Ist gegen Konsens),
  aber erst nach einer `ReactionDelay`. Die ersten Minuten gehören Systemen
  neben der Matching-Engine. Ein Backtest, der sie beansprucht, beschreibt
  einen Trade, den es nie gab.

Der Kalender erzwingt eine Trennung, an der alles hängt: der **Termin** eines
geplanten Ereignisses ist Monate vorher öffentlich, sein **Ausgang** nicht.
`MaskEvents` behält Datum und Konsens, entfernt Ist-Werte und Überraschung bis
zum Eintritt — und blendet *unangekündigte* Ereignisse vollständig aus.

### Social: Aufmerksamkeit ist fast immer zu spät

Rohe Erwähnungszahlen sind wertlos — sie sind der am leichtesten käufliche
Wert im ganzen Paket. Der Agent bewertet stattdessen *Glaubwürdigkeit*:
Autorenvielfalt × Originalität × Kontoalter × Anteil etablierter Konten,
**multiplikativ**, weil jede dieser Größen ein vollständiger Weg ist,
Aufmerksamkeit zu fälschen.

Im mitgelieferten Beispiel landet ein Projekt mit 50.000 Posts auf dem letzten
Platz (Glaubwürdigkeit 0.00) hinter einem mit 900 Posts. `discover` liefert
eine **Watchlist zum Prüfen**, keine Kaufliste — die Rangliste von oben zu
kaufen ist eine mechanische Methode, immer zuletzt anzukommen.

### Das Ensemble zählt Familien, nicht Agenten

Fünf Indikatoren aus denselben Schlusskursen sind sich fast immer einig. Das
ist keine Bestätigung, sondern eine Meinung, fünfmal gezählt. Das Ensemble
verlangt daher Übereinstimmung aus **mindestens zwei verschiedenen Familien**
und vetoiert bei familienübergreifendem Widerspruch: wenn zwei unabhängige
Methoden sich widersprechen, ist die ehrliche Position keine Position.

## Der Launch-Screener

Neue Launches sind die feindseligste Ecke des Marktes. Der Screener ist
deshalb **ablehnungsorientiert**: erst Vetos, dann erst Punkte.

Ein Rug ist kein Token mit schlechten Zahlen — es ist ein Token mit
hervorragenden Zahlen und einer fatalen Eigenschaft. Ein gewichtetes Modell,
in dem tiefe Liquidität eine aktive Mint-Authority überstimmen kann, kauft
genau die Token, die dafür gebaut wurden, gut auszusehen. Deshalb bekommen
diese Bedingungen ein **Veto, kein Gewicht**:

Honeypot (Sell-Simulation scheitert) · Sell-Tax über Limit · aktive Mint- oder
Freeze-Authority · upgradebarer Proxy · unverifizierter Quellcode · LP nicht
gesperrt oder verbrannt · auslaufender Lock · Deployer mit Rug-Historie ·
Mixer-Finanzierung · Whale-/Sniper-Konzentration · Wash-Trading-Verhältnis ·
zu jung für jede Beobachtung.

Im mitgelieferten Beispiel wird `LOOKSGOOD` — 2 Mio. USD Liquidität, 18.000
Holder, LP 400 Tage gesperrt — allein wegen des upgradebaren Proxys abgelehnt.
Das ist der Sinn der Konstruktion, kein Fehlalarm.

## Warum der Backtest nicht lügt

Drei Dinge, die die meisten Backtests falsch machen:

**1. Lookahead ist strukturell unmöglich.** Agenten sehen den Markt nur durch
`View`, das Balken `[0, n)` freigibt — es gibt schlicht keinen Ausdruck in der
API, der einen späteren Balken benennt. Bewiesen wird das nicht durch
Code-Lektüre, sondern empirisch: `lookahead_test.go` schreibt die Zukunft einer
Serie komplett um und prüft, dass sich kein einziges Signal davor ändert.

**2. Ausführung erst beim nächsten Open.**

```
Balken i schließt   →  Strategie sieht 0..i und entscheidet
Balken i+1 öffnet   →  Position wird hier gefüllt, mit Kosten
Balken i+2 öffnet   →  Rendite wird realisiert
```

`TestBacktester_CannotTradeTheGapItPredicted` gibt einer Strategie eine
*perfekte* Vorhersage der nächsten 5%-Lücke — und sie verdient trotzdem nichts
außer Gebühren, weil die Bewegung vor der Ausführung stattfindet.

**2b. Swings sind erst verzögert bekannt.** Ein Hoch auf Balken 100 ist auf
Balken 100 nicht bekannt — erst wenn die Bestätigungsbalken geschlossen haben.
Auf einem gedruckten Chart ist diese Verzögerung unsichtbar, und genau dort
schummelt Pattern-/Fibonacci-Code üblicherweise. `FindSwings` gibt unbestätigte
Extrema gar nicht erst heraus; ein Test fixiert den exakten Grenzbalken.

**3. Selektion wird bestraft.** Wer 200 Varianten testet, findet immer eine
mit gutem Sharpe — auch in reinem Rauschen. `SelectBest` vergleicht daher nicht
gegen null, sondern gegen den Sharpe, den der *Beste von so vielen Versuchen*
allein durch Zufall zeigen würde (Deflated Sharpe Ratio, Bailey & López de
Prado). Schiefe und Fat Tails gehen ein: dieselbe Sharpe-Zahl ist weniger wert,
wenn sie durch das Verkaufen von Crash-Versicherung entsteht.

Unter `P(edge real) = 0.95` lautet das ehrliche Fazit: **nichts nachgewiesen.**

## Die Suche

Bewerten ist nur die halbe Arbeit. Sechs ungetunte Standardkonfigurationen zu
testen und „kein Edge" zu schließen ist keine Suche, sondern eine Stichprobe
von sechs. `search.go` durchsucht den Parameterraum (80 Varianten) — und zwei
Regeln machen das Ergebnis erst brauchbar:

**1. Ausgewählt wird nur mit Vergangenheit.** Anchored Walk-Forward: auf allem
Bekannten eine Konfiguration wählen, auf dem *nächsten* Abschnitt bewerten,
neu wählen. Das konvergiert langsamer und liefert deutlich hässlichere Zahlen
als eine Optimierung über die Gesamthistorie — und genau die hässliche Zahl
war real verfügbar.

**2. Bewertet wird das Verfahren, nicht der Gewinner.** Die Performance der
besten Konfiguration über die Gesamthistorie beantwortet „was hätte ich
verdient, wenn ich die Parameter vorher gewusst hätte" — eine Frage, mit der
niemand etwas anfangen kann.

Auf reinem Rauschen sieht das so aus:

```
bars  866-1197  reversion/lb30/z2.0/wick0.3  in-sample 3.42 → out-of-sample -2.76
VERDICT: no edge: the procedure lost money out of sample
```

Ein In-Sample-Sharpe von **3,42** — damit würden die meisten live gehen. Out
of Sample: **-2,76**.

### Die Hürde: warum die naive Formel echte Edges erschlägt

Der Deflated Sharpe braucht die Streuung, die *edgelose* Varianten zufällig
zeigen würden. Die Literatur schlägt vor, dafür die beobachtete Streuung der
Varianten-Sharpes zu nehmen — das hat aber ein Versagen genau dann, wenn die
Suche *erfolgreich* ist: Wenn ein echter Edge existiert, fangen ihn fast alle
Varianten des richtigen Agenten ein, die Streuung wächst *weil der Edge real
ist*, und die Hürde steigt, bis sie genau das erschlägt, was sie finden sollte.

Gemessen auf denselben 80 Varianten:

| | Nullhürde (analytisch) | Hürde aus beobachteter Streuung |
|---|---|---|
| Rauschen | 6,29 | 6,15 ✓ stimmen überein |
| echter Edge | 6,29 | **21,08** — erschlägt Sharpe 17,24 |

Deshalb entscheidet die analytische Nullhürde (`1/√T`). Die Divergenz der
beiden ist selbst ein Befund: sie heißt „viele Varianten bewegen sich
gemeinsam" — Evidenz *für* eine Entdeckung, nicht dagegen.

```bash
go run ./cmd/signalctl search -csv btc.csv -csv eth.csv -csv sol.csv -grid full
```

Mehrere `-csv` verlangen, dass dieselbe Methode auf mehreren Märkten überlebt
— der Test, der ein Ergebnis am ehesten killt, und deshalb der wertvollste.

## Das Einstellungsverfahren

`SelectBest` *rangiert*. Eine Rangliste hat immer einen Sieger — auch der Beste
eines schlechten Feldes ist schlecht. Das Panel (`interview.go`) fragt etwas
anderes: erfüllt *dieser* Kandidat eine absolute Hürde?

| Kriterium | Wogegen es schützt |
|---|---|
| Deflated Sharpe ≥ 0.95 | Ergebnis ist der beste von vielen Rausch-Zügen |
| ≥ 60% der Folds positiv | Edge existierte in *einem* Regime und ist tot |
| Drawdown haltbar | Rendite real, aber praktisch nicht durchzuhalten |
| Mindestanzahl Trades | Bilanz beruht auf ein paar Glücksfällen |
| **Überlebt 2× Slippage** | Der "Edge" war eine optimistische Gebührenannahme |

**Alle fünf** müssen bestehen. Mitteln würde einem spektakulären Sharpe
erlauben, eine Strategie freizukaufen, deren Edge bei realistischen Kosten
verschwindet. Auf einem Random Walk stellt das Panel **niemanden** ein und
sagt das auch so.

```bash
go run ./cmd/signalctl interview -csv bars.csv -sector meme
```

## Risikomanagement

Volatilitätsziel statt fixem Notional · fraktionales Kelly (Standard: 20%,
weil überbieten langfristig verliert, auch bei echter Edge) · harte Hebel-
grenze · Drawdown-Killswitch, der jede Überzeugung überstimmt · Vol-Floor,
damit eine ruhige Phase die Position nicht ins Unendliche treibt.

## Benutzung

```bash
cd marketsignals

# Zeigt, wie ein ehrliches "hier ist nichts" aussieht (Random Walk)
go run ./cmd/signalctl demo

# Eigene Daten bewerten
go run ./cmd/signalctl backtest -csv bars.csv -interval 1h -folds 5

# Was denken die Agenten über den letzten Balken?
go run ./cmd/signalctl signal -csv bars.csv

# Neue Launches screenen
go run ./cmd/signalctl screen -json examples/launches.json

# Alle Experten-Profile mit Begründung
go run ./cmd/signalctl experts

# Einstellungsverfahren für ein Segment
go run ./cmd/signalctl interview -csv bars.csv -class crypto -sector meme

# Discovery: gehypte Projekte nach glaubwürdiger Aufmerksamkeit
go run ./cmd/signalctl discover -json examples/trending.json

go test ./...
```

CSV-Spalten: `time,open,high,low,close,volume[,buy_volume,sell_volume][,funding]`
— `time` als RFC3339 oder Unix-Epoch (s oder ms). Fehlen Taker-Split oder
Funding, treten die betroffenen Agenten zurück, statt die Eingabe zu erfinden.

## Echtzeit

```bash
go run ./cmd/signald -symbol BTCUSDT -interval 1h -market futures -out signals.jsonl
```

Lädt beim Start ~600 Balken Historie (damit die Agenten nicht tagelang im
Warmup stehen), pollt dann auf Balkenschluss und schreibt bei jedem Schluss ein
Signal — auf stdout und optional als JSONL.

**Der Daemon handelt nicht.** Er hält keinen API-Key, authentifiziert sich nie,
und es gibt in diesem Binary keinen Codepfad, der eine Order platzieren könnte.

### Die eine Regel, an der Live scheitert

Ein REST-Poll liefert als **letztes Element die laufende Kerze**. High, Low und
Close bewegen sich noch. Ein Agent, dem man sie gibt, entscheidet auf Zahlen,
die sich nicht gesetzt haben — sein Signal kippt mehrfach innerhalb einer
Kerze, er steigt auf Spitzen ein, die vor Kerzenschluss wieder weg sind.

Alles, was der Backtest bewiesen hat, ist damit ungültig, denn der Backtest hat
ausschließlich fertige Balken bewertet. Und nichts davon wirft einen Fehler:
der Code läuft, die Signale sehen plausibel aus, und die Verluste kommen als
„die Strategie funktioniert nicht mehr" statt als Absturz.

Deshalb prüfen **beide** Seiten unabhängig, dass ein Balken geschlossen ist —
der Client wirft die laufende Kerze weg, und der Runner weist sie nochmal ab.

### Der Trading-Bot

```bash
go run ./cmd/signald -symbol BTCUSDT -trade -equity 1000
```

Führt die vollständige Ausführungskette aus — gegen ein **Papierkonto**. Das
ist kein Platzhalter: fast jeder Fehler in einem Trading-Bot steckt in der
Mechanik, nicht im Signal, und fast alle davon sind auf Papier sichtbar.

Es gibt in diesem Binary **kein Flag, das auf eine echte Börse zeigt.** Das
`Broker`-Interface wird hier nur von `PaperBroker` erfüllt; etwas
Geld-bewegendes einzusetzen ist eine bewusste Code-Änderung durch den, der die
Zugangsdaten hält — kein Schalter, den eine kopierte Kommandozeile mitbringt.

Drei Dinge, an denen Retail-Bots still scheitern, und die Antwort darauf:

**Abgleich.** Die Vorstellung des Bots von seiner Position und die der Börse
laufen ständig auseinander: abgelehnte Order, Teilausführung, Liquidation,
jemand handelt das Konto von Hand. Ein Bot, der seinem eigenen Gedächtnis
traut, dimensioniert die nächste Order aus einer Position, die er nicht hat.
Die Engine liest das Konto **vor jeder Entscheidung** neu und **hält an**
statt weiterzuhandeln. Ein angehaltener Bot kostet eine Gelegenheit; ein Bot
mit falscher Position kostet das Konto.

Der Abgleich läuft in **Basiseinheiten**, nicht in Kapitalanteilen — 0,02 BTC
sind bei 50.000 $ zehn Prozent und bei 20.000 $ vier Prozent, ohne dass sich
irgendetwas geändert hätte. Eine frühere Fassung verglich Anteile und hielt
bei jeder Kursbewegung fälschlich an.

**Idempotenz.** Ein Timeout heißt nicht „Order abgelehnt", sondern „Ausgang
unbekannt". Blind neu senden verdoppelt die Position. Jede Order trägt einen
deterministischen Schlüssel aus Instrument, Kerze und Ziel — ein Resend ist
dieselbe Order, und bei unklarem Ausgang wird **nicht** erneut gesendet,
sondern angehalten.

**Leitplanken.** Positionsgröße, Einzelordergröße, Tagesverlust und ein
Drawdown-Stop, der die Position **tatsächlich schließt** statt nur keine neue
zu öffnen. Rundung immer Richtung null, damit ein Rundungsfehler eine Order
nie größer macht als beabsichtigt.

Und das Papierkonto schließt die Lücke, vor der der Runner sonst warnen muss:
der Killswitch braucht eine echte Kapitalkurve, und ein Papierkonto hat eine.

### Benachrichtigungen

```bash
export NOTIFY_URL="https://api.telegram.org/bot<TOKEN>/sendMessage"
go run ./cmd/signald -symbol BTCUSDT -notify-chat <CHAT_ID>
```

Funktioniert genauso mit Discord (`-notify-field content`), Slack, ntfy.sh
oder einem eigenen Endpunkt — jeder nimmt ein POST mit JSON entgegen, der Rest
ist Konfiguration. Der Token wird aus der Umgebung gelesen (nicht aus der
Shell-History) und **nie** ausgegeben; auch die Fehlermeldung bei einem 401
enthält die URL nicht, weil genau solche Zeilen in Issue-Tracker kopiert
werden.

**Benachrichtigt wird bei Positionsänderung, nicht pro Kerze.** Eine
Stundenstrategie erzeugt 24 Signale am Tag, von denen 23 „keine Änderung"
sagen — und die verlässliche Folge davon ist, dass das 24. auch ignoriert
wird. Gefiltert wird auf:

- Zielposition bewegt sich um ≥ 20 % des Kapitals, **und** höchstens eine
  Nachricht pro Stunde
- **Seitenwechsel immer** — long → short oder → flat wird nie unterdrückt,
  auch nicht innerhalb der Sperrfrist. „Auf flat" heißt „Position schließen".

Die Nachricht führt mit der Handlung, nicht mit der Analyse: Seite, Größe,
Kerze. Danach erst, welcher Agent was gesehen hat — und die Warnung, falls der
Drawdown-Stop mangels verdrahtetem Konto wirkungslos ist.

### Weitere Live-Eigenschaften

- **Lücken stoppen den Betrieb.** Bricht die Verbindung ab, fehlen Balken. Jeder
  Indikator hier liest ein Fenster *aufeinanderfolgender* Balken; ein ATR über
  ein Loch ist eine Zahl über einen Markt, den es nicht gab. Der Runner meldet
  sich als ungesund und schweigt, bis er neu geseedet wird.
- **Live entscheidet identisch zum Backtest.** Ein Test füttert beide Pfade mit
  denselben Balken und vergleicht Richtung und Stärke. Ohne diese Gleichheit
  wäre jede Zahl aus der Suche eine Aussage über ein anderes System.
- **Der Killswitch braucht dein echtes Konto.** Im Backtest liest er eine
  simulierte Equity-Kurve. Live existiert die nicht — und eine stille Null
  hieße „kein Drawdown", was den einzigen Schutz vor einer verlierenden
  Strategie dauerhaft entschärft. Ohne verdrahtetes `Drawdown` steht die
  Warnung in jedem Signal.

## Echte Daten holen

Ein Befehl, keine Abhängigkeiten, läuft dort wo dein Netz offen ist:

```bash
# Spot-Historie mit echtem Taker-Split (Binance meldet das Taker-Buy-Volumen
# pro Balken — der Flow-Agent bekommt damit eine echte Kauf/Verkauf-Aufteilung
# statt einer Schätzung aus der Kerzenfarbe)
go run ./cmd/fetchdata binance -symbol BTCUSDT -days 720 -out btc.csv

# Perpetual: zusätzlich die Funding-Historie, an die Balken ausgerichtet
go run ./cmd/fetchdata binance -market futures -symbol BTCUSDT -days 720 -out btc_perp.csv

# Dann die Suche über mehrere Märkte
go run ./cmd/signalctl search -csv btc.csv -csv eth.csv -csv sol.csv -grid full
```

Funding wird **vorwärts** gefüllt: ein Balken zeigt die Abrechnung, die zu
seinem Zeitpunkt bereits galt, nie eine spätere. Balken vor der ersten
Abrechnung werden verworfen statt mit einer Null gefüllt — eine Null läse sich
als „Funding ist neutral", während die Wahrheit „unbekannt" ist. Tests prüfen
beides, damit ein Formatfehler nicht erst nach einer Stunde Download auffällt.

### DEX: das Kostenmodell war schlicht falsch

An einer Börse mit Orderbuch ist eine feste Slippage-Zahl eine vertretbare
Vereinfachung. An einem AMM ist sie **falsch**, und zwar in der gefährlichen
Richtung. Ein Pool hat kein Buch — der Preis ist eine Funktion der Reserven,
und dein eigener Trade bewegt ihn, bevor du gefüllt wirst. Bei konstantem
Produkt ist das exakt, nicht näherungsweise:

```
Δx in Reserve X  →  Y·Δx/(X+Δx) heraus
Effektivpreis = (X+Δx)/Y  gegen Spot X/Y
Auswirkung = Δx / X
```

Die Auswirkung ist **linear** in der Größe, die Gesamtkosten also
**quadratisch**. Doppelte Position, vierfache Kosten.

Das entscheidet die meisten DEX-Strategien, und eine Flatrate versteckt es
vollständig. Gemessen an einem 400.000-$-Pool:

| Konto | Größte Position unter 1% Kosten |
|---|---|
| 5.000 $ | **8,0 %** des Kontos |
| 500.000 $ | **0,08 %** des Kontos |

Dieselbe Strategie, derselbe Pool, dasselbe Signal. Nur ist der Händler beim
zweiten Fall selbst der Markt geworden — und **nichts in den Kursdaten** zeigt
diesen Unterschied. Ein Backtest, der nicht weiß, wie viel Geld er handelt,
kann dir nicht sagen, in welchem Fall du bist.

Deshalb verlangt ein DEX-Instrument `PoolLiquidityUSD` **und** `AccountUSD`;
fehlt eines, wird der Trade als unmöglich bepreist statt mit einer
schmeichelhaften Flatrate.

### DEX-Kursdaten

Dexscreener veröffentlicht den **aktuellen** Zustand eines Pools — gut für den
Screener, unbrauchbar für einen Backtest. GeckoTerminal veröffentlicht OHLCV
pro Pool, kostenlos und ohne Key:

```bash
go run ./cmd/fetchdata dex -chain eth -address 0xTOKEN -bars 1000 -account 10000 -out token.csv
```

Wählt den **tiefsten** Pool, nicht den mit dem meisten Volumen — Volumen an
einem dünnen Pool sind überwiegend Arbitrage-Bots, die ihn gegen eine tiefere
Börse korrigieren, und danach zu sortieren wählt zuverlässig den Pool aus, an
dem man am wenigsten handeln will. Danach wird ausgegeben, wie groß deine
Position an diesem Pool überhaupt sein darf.

Ein Taker-Split existiert bei dieser Quelle nicht. Der Flow-Agent tritt auf
DEX-Serien deshalb zurück — korrekt, denn eine aus der Kerzenfarbe erfundene
Aufteilung wäre kein Orderfluss, sondern die Preisreihe in Verkleidung.

### Unbekannt ist nicht sauber

Dexscreener veröffentlicht Preise, Liquidität und Trade-Zahlen — aber **nicht**
das, was über Haltbarkeit entscheidet: Mint-/Freeze-Authority, LP-Lock,
Honeypot-Simulation, Deployer-Historie.

Das ist gefährlicher als es klingt. `MintAuthorityActive` ist im Go-Zero-Value
`false`, und `false` heißt „keine Mint-Authority" — das ist ein **Bestehen**.
Ein aus einem Preis-Aggregator zusammengesetzter Datensatz rutscht damit genau
durch das Veto, das ihn fangen soll: Abwesenheit von Evidenz wird still als
Evidenz von Abwesenheit gelesen, an der einen Stelle, wo der Irrtum die ganze
Position kostet.

`Launch.Checks` hält deshalb fest, welche Prüfungen *tatsächlich durchgeführt*
wurden. Jede nicht durchgeführte Prüfung ist ein hartes Veto:

```
not verified: no sell was simulated — nobody established that the position can be exited
not verified: mint, freeze and upgrade authorities were never inspected
```

Eine ungeprüfte Authority ist keine abwesende Authority.

## Datenquellen — und wo die Grenze liegt

`venues.go` definiert, was das Framework von außen braucht: `BarSource`,
`LaunchSource`, `SocialSource`. Mitgeliefert wird **eine** Implementierung:
Dateien auf der Platte (`FileSource`). Es gibt bewusst **keinen** Live-Adapter
für X, für eine CEX oder für einen DEX-Aggregator.

Das ist kein Rückstand, sondern die ehrliche Grenze: jeder solche Adapter
braucht Zugangsdaten, und Zugangsdaten sind eine Entscheidung darüber, was in
wessen Namen handeln darf. Für dich, der die Keys hat, ist das eine kleine
Arbeit — hier geraten, ergäbe es Code, der nicht testbar ist und dessen
Fehlerfall ein stiller Strom falscher Preise wäre.

Zwei Normalisierungsprobleme sind aber gelöst, weil sie leicht zu übersehen
und teuer sind:

- **AMM-Volumen ist überwiegend Arbitrage.** Ein Pool hat keine Käufer und
  Verkäufer im Sinne eines Orderbuchs; ein großer Teil des Volumens sind Bots,
  die den Pool auf den Preis zurückschieben, den er anderswo schon hat. Roh an
  den Flow-Agenten gegeben ergibt das eine selbstbewusste Lesung von Robotern.
- **CEX/DEX-Divergenz** ist kein Gratisgeld, sondern ein Datenhygiene-Alarm.

## Ehrliche Grenzen

- **Keine Live-Anbindung.** Ein Broker-Adapter ist bewusst nicht enthalten.
- **Politische Prognose findet nicht statt.** Nur Reaktion auf Überraschung,
  und die erste Minute ist bewusst unerreichbar.
- **Fibonacci hat keine physikalische Wirkung.** Die Level funktionieren, wenn
  überhaupt, weil genug Leute hinschauen. Ob das reicht, entscheidet die Hürde.
- **Keine getunten Parameter.** Wer sie tunt, muss die Trial-Zahl in
  `SelectBest` entsprechend erhöhen, sonst wird das Ergebnis wertlos.
- **Slippage ist eine Annahme.** Ändert sich das Ranking, wenn man sie
  verdoppelt, war das Ergebnis eine Gebührenannahme, keine Edge.
- **Der Launch-Screener findet keine Gewinner.** Er entfernt konstruierte
  Verluste. Ein `accept` bleibt eine Wette mit hoher Varianz.
- **Regimewechsel.** Alle vier Markt-Agenten setzen auf Effekte, die
  verschwinden können. Die Fold-Streuung zeigt, wann das passiert ist.
