# Daten hier ablegen

Diese Session kann keine Börse erreichen — der Egress-Gateway beantwortet jeden
Marktdaten-Host mit 403, per `curl` wie per Fetch-Tool. Was **funktioniert**,
ist git. Also läuft der Weg über das Repository.

## Drei Schritte

```bash
# 1. Daten holen (auf deiner Maschine, wo das Netz offen ist)
cd marketsignals
go run ./cmd/fetchdata binance -symbol BTCUSDT -interval 1h -days 720 -out data/BTCUSDT.csv
go run ./cmd/fetchdata binance -symbol ETHUSDT -interval 1h -days 720 -out data/ETHUSDT.csv
go run ./cmd/fetchdata binance -symbol SOLUSDT -interval 1h -days 720 -out data/SOLUSDT.csv

# 2. Hochladen
git add data/*.csv && git commit -m "add real bars" && git push

# 3. Sagen, dass es da ist — dann ziehe ich und rechne
```

Mehr Instrumente sind besser: unter acht Namen weigert sich der Cross-Sectional-
Allokator zu ranken, weil vier Assets in Dezile zu sortieren ein Münzwurf mit
Extraschritten ist.

## Ohne Go, nur mit curl und jq

```bash
curl -s 'https://api.binance.com/api/v3/klines?symbol=BTCUSDT&interval=1h&limit=1000' \
| jq -r '"time,open,high,low,close,volume,buy_volume,sell_volume",
         (.[] | [(.[0]/1000|floor), .[1], .[2], .[3], .[4], .[5], .[9],
                 ((.[5]|tonumber) - (.[9]|tonumber))] | @csv)' \
> data/BTCUSDT.csv
```

Das liefert 1000 Balken statt 720 Tagen, reicht aber für einen ersten Blick.
`.[9]` ist das Taker-Buy-Volumen — daher bekommt der Flow-Agent eine echte
Kauf/Verkauf-Aufteilung statt einer Schätzung aus der Kerzenfarbe.

## Was dann läuft

```bash
go run ./cmd/signalctl analyse -dir data
```

Sucht über alle Varianten je Instrument, baut ein Buch über das ganze
Verzeichnis, und stellt beides demselben Einstellungsverfahren — in **einem**
Feld, damit der Trial-Count ehrlich bleibt.

Erwartungshaltung: wahrscheinlich wird niemand eingestellt. Das ist auf echten
Daten das übliche Ergebnis und ein Befund, kein Fehler.
