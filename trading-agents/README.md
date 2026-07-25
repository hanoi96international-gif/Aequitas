# Trading Agents

Ein eigenständiges Multi-Agenten-System (unabhängig vom Aequitas-Chain-Code
in diesem Repo), das täglich Handelssignale für eine Watchlist erarbeitet.
Drei spezialisierte KI-Agenten analysieren jede Aktie unabhängig voneinander;
anschließend prüft jeweils ein anderer Agent das Ergebnis eines Kollegen
gegen ("Peer Review"), bevor ein Chefstratege daraus eine finale Einschätzung
zusammenfasst.

**Wichtig:** Das System erzeugt nur Research-Signale (Markdown/JSON-Report).
Es platziert keine echten Trades und ist nicht an einen Broker angebunden.
Keine Anlageberatung.

## Ablauf pro Ticker

1. **Kursdaten** — `yfinance` liefert Kurshistorie + einfache Indikatoren
   (SMA20/50, RSI14, Volatilität).
2. **Drei unabhängige Analysten** (parallel):
   - `technical` — urteilt ausschließlich auf Basis der Kursdaten.
   - `fundamental` — recherchiert per Web-Suche Unternehmensnews, Earnings,
     Makro-Umfeld.
   - `sentiment` — recherchiert per Web-Suche aktuelle Markt-/Nachrichtenstimmung.
3. **Peer Review** — jedes Signal wird von einem anderen Agenten
   gegengeprüft (Round-Robin, niemand prüft sich selbst):
   - Sentiment-Analyst prüft Technical
   - Technical-Analyst prüft Fundamental
   - Fundamental-Analyst prüft Sentiment
4. **Synthese** — ein Chefstratege wägt Signale und Kritiken gegeneinander
   ab und gibt eine finale Einschätzung inkl. abweichender Meinungen aus.

## Setup

```bash
cd trading-agents
python3 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
cp .env.example .env   # ANTHROPIC_API_KEY eintragen
```

## Ausführen

```bash
python main.py --tickers SPY AAPL MSFT NVDA
```

Schreibt `reports/<datum>.md` (lesbarer Report) und `reports/<datum>.json`
(alle Rohdaten: jedes Analysten-Signal, jede Kritik, finale Synthese).

Für einen täglichen automatischen Lauf z.B. per Cron:

```cron
0 22 * * 1-5 cd /pfad/zu/trading-agents && .venv/bin/python main.py >> run.log 2>&1
```

## Konfiguration

- `config.py`: Standard-Watchlist, Modell (`claude-opus-5`), Kurshistorie-Zeitraum.
- `--tickers`: überschreibt die Watchlist pro Lauf.

## Grenzen / nächste Schritte

- Keine Portfolio- oder Positionsgrößen-Logik — jedes Signal ist pro Ticker
  isoliert.
- Keine echte Orderausführung; das ist bewusst so, um finanzielles und
  regulatorisches Risiko aus einem automatisierten Erstentwurf herauszuhalten.
  Eine Anbindung an eine Broker-API (z.B. Paper-Trading zuerst) wäre ein
  separater, expliziter nächster Schritt.
- `yfinance` liefert verzögerte/kostenlose Daten — für produktiven Einsatz
  ggf. gegen einen bezahlten Marktdaten-Feed austauschen.
