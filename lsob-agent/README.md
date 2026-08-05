# LSOB Trade Agent

Ein Trading-Agent für **LSOB — Liquidity Sweep + Order Block**: er erkennt
Setups, testet sie auf historischen Daten, handelt sie im Paper-Modus und
kann (nach ausdrücklicher Freischaltung) echte Orders senden.

Der Kern läuft mit **reiner Standardbibliothek**, Python 3.11+. `ccxt`
brauchst du nur, um Kerzen von einer Börse zu laden oder live zu handeln —
Strategie, Backtester und die komplette Testsuite laufen ohne jede
Installation.

---

## Was der Agent sucht

Die Sequenz, Schritt für Schritt:

1. Ein Swing-Hoch/Tief hinterlässt **ruhende Liquidität** — dort liegen die
   Stops (über Hochs: *buy-side*, unter Tiefs: *sell-side*).
2. Der Preis **raidet** dieses Level und schließt wieder **zurück darunter**
   → ein **Sweep**. Läuft er zu weit darüber hinaus oder bleibt er oben, war
   es ein echter Ausbruch und kein Raid — genau diese Unterscheidung machen
   `max_penetration_atr` und `reclaim_bars`.
3. Danach muss eine **Displacement**-Bewegung folgen: ein impulsiver Move
   weg vom geraideten Level, der die **Marktstruktur bricht** (BOS).
4. Die letzte **Gegenkerze** vor diesem Move ist der **Order Block**.
5. Einstieg per **Limit-Order** an der Kante dieses Blocks, Stop hinter dem
   Extrem des Raids, Ziele als R-Vielfache oder am gegenüberliegenden
   Liquiditätspool.

```
        ┌── Sweep: Docht über das Hoch, Close zurück darunter
        │
   ───▲─┴──────  ← buy-side Liquidität (Swing-Hoch)
      │  ██          █ = Order Block (letzte Aufwärtskerze)
      │  ▼▼
      │   ▼▼▼   ← Displacement + Break of Structure
      │     ▼▼
   ───┴───────  ← das gebrochene Swing-Tief
```

**Jede** dieser Regeln ist ein Wert in `config.toml` — kein Schwellenwert
steckt im Code.

---

## Schnellstart

```bash
cd lsob-agent

# 1) Setups auf eigenen Daten anschauen (keine Installation nötig)
python -m lsob -c config.toml scan

# 2) Backtest mit allen Kennzahlen
python -m lsob -c config.toml backtest --trades

# 3) Kerzen von einer Börse holen (braucht ccxt)
pip install ccxt
python -m lsob -c config.toml fetch --bars 5000

# 4) Paper-Trading live mitlaufen lassen
#    (vorher live.enabled = true setzen; mode bleibt "paper")
python -m lsob -c config.toml run
```

Eigene CSV statt Börse: in `config.toml` unter `[data]` einfach
`csv = "pfad/zu/kerzen.csv"` setzen. Erkannt werden `timestamp`/`time`/
`date`/`open_time` als Zeitspalte, in Millisekunden, Sekunden oder ISO-8601.

---

## Echte Binance-Daten ohne API-Key

```bash
# Ganze Monate aus dem öffentlichen Archiv (data.binance.vision)
python -m lsob -c config.toml fetch --months 2024-01:2024-12
```

Kein Schlüssel, kein Rate-Limit, eine Datei pro Monat — der REST-Endpunkt
ist für mehrere Jahre Historie der umständliche Weg. Die `.zip`-Archive
werden direkt gelesen, ohne Entpacken, und `data.csv` akzeptiert beide
Formate (beschriftete CSV wie auch das headerlose Archivlayout).

Alternativ weiterhin über ccxt, wenn du nur die letzten N Kerzen brauchst:

```bash
pip install ccxt && python -m lsob -c config.toml fetch --bars 5000
```

## Die Ebenen ansehen

```bash
python -m lsob -c config.toml chart --index -1 --out setup.svg
```

Zeichnet ein Setup mit allem, was der Agent gesehen hat: geraidetes Level,
Raid-Extrem, Order Block, Einstieg, Stop und Ziele — auf den Kerzen, an der
Stelle, an der er gehandelt hat.

Ein Backtest sagt dir, *was* passiert ist. Er kann dir nicht sagen, ob die
Linien dort liegen, wo **du** sie gezogen hättest. Dafür ist dieses Bild da.

Farbe trägt nur zwei Dinge: die **Rücklauflinie** und die
**Liquiditätsebenen**. Die Kerzen bleiben neutral — sie rot und grün zu
malen würde vier kräftige Farbtöne gegen die zwei stellen, die etwas
bedeuten. Jede Linie ist zusätzlich direkt beschriftet, nichts hängt allein
an der Farbe. Hell und Dunkel sind getrennt geprüft.

## Datenqualität

Jeder Ladevorgang wird geprüft und meldet, was er findet:

```
Data integrity: 52057 candles, 5751 non-finite or sentinel, 1 intra-bar spikes,
                61 duplicate timestamps, 84 missing bars
  dropped 5813 unusable bars
```

Das ist kein hypothetisches Problem. Ein öffentlicher BTC/USD-Datensatz, an
dem dieser Agent getestet wurde, füllt **11 % seiner Zeilen** mit `1.7e308`
in *jeder* Spalte — Open, High, Low und Close stimmen dann perfekt überein,
das High/Low-Verhältnis ist exakt 1,00, und keine Plausibilitätsprüfung
*innerhalb* eines Balkens findet das. Erst der Vergleich mit dem letzten
guten Close entlarvt es.

Warum das zählt: ATR wird aus der True Range gebildet. Ein solcher Balken
vergiftet jede ATR-skalierte Schwelle für `atr_period` Balken danach — die
Strategie lieferte auf diesem Datensatz **null Signale über 52.000 Kerzen**,
ohne einen einzigen Fehler zu werfen. Lücken werden gezählt, aber nie
gefüllt: eine fehlende Stunde ist eine Information, und sie zu interpolieren
würde Kursverlauf erfinden.

## Backtest-Ausgabe

```
Bars 3000   Signals 41   Filled 23
--------------------------------
Trades            23  (11 long / 12 short)
Win rate          43.5%  (10W / 13L)
Expectancy        +0.184 R per trade
Total R           +4.23 R
Profit factor     1.42
Net P&L           +211.50  (+2.12%)
Max drawdown      6.30%  (631.20)
```

Die Zahl, auf die es ankommt, ist **Expectancy in R**. Der Euro-Betrag ist
nur eine Funktion deiner Positionsgröße; R ist eine Eigenschaft der
Strategie.

---

## Konfiguration

Alle Werte stehen kommentiert in [`config.toml`](config.toml). Die
wichtigsten Stellschrauben:

| Sektion | Parameter | Wirkung |
|---|---|---|
| `[structure]` | `swing_left` / `swing_right` | Wie markant ein Pivot sein muss. `swing_right` ist zugleich die Verzögerung, bis ein Pivot überhaupt bekannt ist. |
| `[liquidity]` | `max_penetration_atr` | Ab hier ist es Ausbruch statt Raid. Der wichtigste Filter überhaupt. |
| | `reclaim_bars` | Wie schnell der Preis zurück muss. `0` = nur Rückeroberung in derselben Kerze. |
| | `min_touches` | Auf `2` nur Equal Highs/Lows handeln — dort liegen mehr Stops. |
| `[orderblock]` | `displacement_atr` | Wie heftig der Move nach dem Sweep sein muss. |
| | `require_bos` | Verlangt den Bruch des gegenüberliegenden Swings. |
| | `require_fvg` | Verlangt zusätzlich eine Fair-Value-Gap im Impuls. |
| | `zone_mode` | `body` / `full` / `body_to_extreme` — wie die Zone geschnitten wird. |
| `[entry]` | `edge` | `proximal` / `mid` / `distal` an der Blockkante, oder `retracement` auf einer Fib-Ebene des Beins. |
| | `retracement` | Rücklauftiefe für `edge = "retracement"`, als Anteil des Beins. `0.882` ist der Vorgabewert, kein belegter Standard. |
| | `sl_anchor` | Stop hinter dem Sweep-Extrem oder hinter dem Order Block. |
| | `tp_rr` / `tp_weights` | Gestaffelte Ziele, Gewichte müssen 1.0 ergeben. |
| | `breakeven_after_tp` | Stop nach dem N-ten Ziel auf Einstand. |
| `[filters]` | `session_enabled` | Nur in den Kill Zones handeln (UTC-Fenster). |
| | `premium_discount` | Shorts nur oberhalb, Longs nur unterhalb des Equilibriums. |
| | `require_unmitigated` | Blocks verwerfen, in die der Preis schon zurücklief. |
| | `require_inducement` | Einstieg erst, wenn die Liquidität des Zwischenswings vor dem Block abgeholt wurde. |
| `[bias]` | `mode` | `off` / `ema` / `htf_structure` — Richtungsfilter. |
| `[risk]` | `risk_pct` | Prozent des Kapitals pro Trade. |

Tippfehler in Schlüsseln sind ein **Fehler**, kein stilles Ignorieren — ein
überlesener Schwellenwert wäre eine Strategieänderung, die du nicht
vorgenommen hast.

---

## Wie ehrlich der Backtest ist

Ein Backtester hat die Aufgabe, dir Strategien **auszureden**. Deshalb sind
alle Annahmen pessimistisch gewählt:

- **Kein Lookahead.** Ein Swing-Pivot wird erst `swing_right` Kerzen später
  überhaupt sichtbar. Die Engine arbeitet strikt inkrementell — Backtest und
  Live-Agent nutzen wörtlich denselben Code, es gibt keinen zweiten Pfad,
  der auseinanderdriften könnte.
- **Bei Mehrdeutigkeit gewinnt der Stop.** Eine Kerze, die Stop *und* Ziel
  berührt haben könnte, wird als Stop gewertet — in jeder Phase des Trades.
- **Kein Same-Bar-Fill.** Ein Signal aus Kerze *i* kann frühestens in *i+1*
  gefüllt werden.
- **Limit-Fills werden nie besser als der Limitpreis.** Springt der Kurs
  über die Order hinweg, wird zum Open gefüllt, nicht zum Wunschpreis.
- **Gebühren und Slippage** werden bei Ein- und Ausstieg verrechnet.

Zur Kontrolle läuft in der Testsuite ein **Random Walk** durch die Engine.
Auf reinem Rauschen liegt die Erwartung bei etwa **−0,23 R pro Trade** über
~1.500 Trades — also genau bei den Handelskosten. Ein deutlich positiver
Wert dort wäre der Beweis für einen Lookahead-Fehler, und der Test schlägt
in dem Fall fehl.

```bash
pip install pytest && python -m pytest -q      # 160 Tests, ~3 s
```

---

## Parameter wählen, ohne sich selbst zu betrügen

```bash
python -m lsob -c config.toml walkforward --params
```

Optimiert auf einem Zeitfenster, wendet den Gewinner **unverändert** auf das
folgende Fenster an und rollt weiter. Nur diese ungesehenen Abschnitte
werden addiert.

Lies dort **nicht** die beste Zeile, sondern zwei andere Zahlen:

- **Out-of-sample expectancy** — was der *Vorgang* „optimieren und dann
  handeln" tatsächlich eingebracht hätte.
- **IS/OOS rank correlation** — wie stark sich die Rangfolge überhaupt
  überträgt. Nahe null heißt: die Daten können gute Einstellungen nicht von
  glücklichen unterscheiden, und einen „Besten" zu küren ist Selbstbetrug.
  Das Tool schreibt dieses Urteil selbst hin und verweigert jede Aussage
  unter 30 Out-of-Sample-Trades.

`selection = "robust"` (Standard) wählt nicht die höchste Einzelpunktzahl,
sondern das breiteste **Plateau** — eine Einstellung, deren Nachbarwerte
ebenfalls funktionieren. Ein einzelner Ausreißer, der bei genau einem Wert
glänzt und daneben abfällt, ist an die Stichprobe angepasst, nicht an den
Markt. `selection = "peak"` gibt es zum Vergleich; es fittet Rauschen.

## Wie groß darf eine Position sein?

```bash
python -m lsob -c config.toml risk --levels 0.25,0.5,1,2,3
```

Das ist der einzige Teil hier, der **Arithmetik statt Prognose** ist: ob ein
Setup einen Vorteil hat, ist eine Vorhersage — wie tief der Drawdown bei
gegebenem Risiko pro Trade ausfällt, folgt zwingend aus der Verteilung. Die
Simulation zieht aus deinen tatsächlich beobachteten R-Werten, behält also
die echte Form der Ergebnisse samt fettem linken Rand.

Zwei Dinge werden dabei sichtbar, die der Intuition widersprechen:

- **Drawdown wächst schneller als das Risiko.** Doppeltes Risiko pro Trade
  kostet mehr als doppelten Drawdown, weil Verluste gegen ein schrumpfendes
  Konto compounden.
- **Verlustserien sind länger als gedacht.** Bei 40 % Trefferquote sind acht
  Verluste in Folge normal, nicht ungewöhnlich.

Und die unbequeme Konsequenz: **Positionsgröße steuert, wie schnell du
verlierst, nicht ob.** Ohne Vorteil verschiebt kleineres Risiko nur den
Zeitpunkt.

## Live-Handel

Der Live-Pfad ist gebaut, aber **dreifach verriegelt**. Es müssen alle drei
Bedingungen gleichzeitig erfüllt sein:

1. `live.enabled = true` **und** `live.mode = "live"` in der Config
2. API-Zugangsdaten in den Umgebungsvariablen (`LSOB_API_KEY`,
   `LSOB_API_SECRET`)
3. das Flag `--live` auf der Kommandozeile

Zusätzlich läuft er auf dem **Testnet**, solange `live.sandbox = true` ist.
Steht es auf `false`, verlangt die CLI eine getippte Bestätigung.

```bash
export LSOB_API_KEY=... LSOB_API_SECRET=...
python -m lsob -c config.toml run --live
```

**Was der Live-Broker leistet und was nicht** — bewusst nüchtern: Er setzt
eine Limit-Entry, danach Stop und Take-Profits als reduce-only Orders, und
storniert abgelaufene Entries. Er macht **keine** Teilfill-Abstimmung über
Neustarts hinweg und übernimmt **keine** Positionen, die er nicht selbst
eröffnet hat. Lass ihn auf dem Testnet einen vollständigen Trade zweimal
durchspielen, bevor echtes Geld im Spiel ist.

Benachrichtigungen laufen über Konsole und optional Telegram
(`LSOB_TG_TOKEN`, `LSOB_TG_CHAT`).

---

## Aufbau

```
lsob/
  model.py       Candle, ATR, Timeframe-Rechnung
  structure.py   Swing-Pivots (verzögert bestätigt), Market Structure / BOS
  liquidity.py   Liquiditätspools, Sweep- vs. Ausbruch-Erkennung
  orderblock.py  Order Blocks, Displacement, Fair Value Gaps
  bias.py        optionaler Richtungsfilter (EMA / höherer Zeitrahmen)
  filters.py     Premium/Discount, Mitigation, Session-Fenster
  strategy.py    die Zustandsmaschine, die daraus Signale macht
  execution.py   Order- und Positionsverwaltung, pessimistische Fills
  backtest.py    Durchlauf über historische Kerzen
  metrics.py     Kennzahlen in R und in Euro
  walkforward.py Optimieren auf der Vergangenheit, werten auf der Zukunft
  sizing.py      Risiko pro Trade -> Drawdown-Wahrscheinlichkeiten
  broker.py      PaperBroker (nutzt execution.py) und LiveBroker (ccxt)
  agent.py       Live-Loop: pollen, füttern, handeln, Status sichern
  chart.py       ein Setup mit allen Ebenen als SVG zeichnen
  cli.py         backtest / scan / chart / walkforward / risk / fetch / run
```

---

## Nächste Schritte

Die Defaults sind ein **Startpunkt, keine getunte Strategie**. Sinnvolle
Reihenfolge:

1. `scan` auf deinem Markt und Zeitrahmen — sehen die Setups nach dem aus,
   was du von Hand handeln würdest?
2. Falls nicht: **einen** Parameter ändern, neu backtesten. Mehrere auf
   einmal sagen dir nichts darüber, welcher gewirkt hat.
3. Auf einem Zeitraum prüfen, gegen den du *nicht* optimiert hast.
4. Paper-Trading, bis du dem Verhalten bei echten Fills traust.

Wenn deine LSOB-Variante von der hier umgesetzten abweicht — andere
Sweep-Definition, andere OB-Auswahl, andere Ziellogik — sind das meistens
Werte in `config.toml`. Was sich dort nicht abbilden lässt, gehört in
`strategy.py`; die Zustandsmaschine dort ist bewusst klein gehalten.
