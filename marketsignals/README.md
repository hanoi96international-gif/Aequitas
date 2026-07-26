# marketsignals

Ein eigenständiges Go-Modul für Marktsignal-Agenten: fünf spezialisierte
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

## Die fünf Agenten

| Agent | Familie | Idee | Warum verteidigbar |
|---|---|---|---|
| `breakout` | trend | Donchian-Ausbruch, ATR-normiert, mit Trendfilter | Leverage-Kaskaden erzeugen Persistenz; der ATR-Filter verhindert Whipsaws in der Range |
| `reversion` | reversion | Fadet Erschöpfung — nur mit Rejection-Wick, nur im Seitwärtsregime | Handelt *erzwungene* Verkäufer, nicht bloß schwache Preise |
| `flow` | flow | Divergenz zwischen Preisextrem und kumulativer Taker-Delta | Einziger Agent, der früh statt bestätigend ist |
| `funding` | positioning | Perp-Funding-Extreme über Perzentilrang | Misst Überfüllung statt Preis — Fehler unkorreliert zu den anderen |
| `launch` | screen | Risiko-Screener für neue Token | Beantwortet nicht "wohin", sondern "überhaupt anfassbar?" |

Die vier Markt-Agenten haben **bewusst ungetunte** Parameter (runde Zahlen).
Auf denselben Daten optimieren, auf denen man auch bewertet, ist Curve-Fitting.

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

**3. Selektion wird bestraft.** Wer 200 Varianten testet, findet immer eine
mit gutem Sharpe — auch in reinem Rauschen. `SelectBest` vergleicht daher nicht
gegen null, sondern gegen den Sharpe, den der *Beste von so vielen Versuchen*
allein durch Zufall zeigen würde (Deflated Sharpe Ratio, Bailey & López de
Prado). Schiefe und Fat Tails gehen ein: dieselbe Sharpe-Zahl ist weniger wert,
wenn sie durch das Verkaufen von Crash-Versicherung entsteht.

Unter `P(edge real) = 0.95` lautet das ehrliche Fazit: **nichts nachgewiesen.**

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

go test ./...
```

CSV-Spalten: `time,open,high,low,close,volume[,buy_volume,sell_volume][,funding]`
— `time` als RFC3339 oder Unix-Epoch (s oder ms). Fehlen Taker-Split oder
Funding, treten die betroffenen Agenten zurück, statt die Eingabe zu erfinden.

## Ehrliche Grenzen

- **Keine Live-Anbindung.** Ein Broker-Adapter ist bewusst nicht enthalten.
- **Keine getunten Parameter.** Wer sie tunt, muss die Trial-Zahl in
  `SelectBest` entsprechend erhöhen, sonst wird das Ergebnis wertlos.
- **Slippage ist eine Annahme.** Ändert sich das Ranking, wenn man sie
  verdoppelt, war das Ergebnis eine Gebührenannahme, keine Edge.
- **Der Launch-Screener findet keine Gewinner.** Er entfernt konstruierte
  Verluste. Ein `accept` bleibt eine Wette mit hoher Varianz.
- **Regimewechsel.** Alle vier Markt-Agenten setzen auf Effekte, die
  verschwinden können. Die Fold-Streuung zeigt, wann das passiert ist.
