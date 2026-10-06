# Wirtschaftsmodell

Stand 06.10.2026. Das Modell aus `WIRTSCHAFT_ZAHLENPRUEFUNG.md`, Abschnitt 4.2:
feste Akteure, die echten Formeln, und für jede Zahl, wen sie wie trifft.

```
go run ./cmd/wirtschaftsmodell                           # heutige Zahlen, 12 Monate
go run ./cmd/wirtschaftsmodell -setze UmlaufMonat=0.01   # eine Zahl ändern
go run ./cmd/wirtschaftsmodell -empfindlichkeit          # jede Zahl halbiert und verdoppelt
go run ./cmd/wirtschaftsmodell -menschen 5000            # Topf je Mensch bei 5.000 Menschen
```

## Was es rechnet

Je Akteur und Monat (`x/humanity/keeper/wirtschaftsmodell.go`):

- **Gebühr:** Menschen die ersten 1.000 AEQ Ausgaben im Monat frei, danach
  0,1 %; Unternehmen an Menschen frei, sonst 0,1 %.
- **Umlaufsicherung:** Menschen 0,5 % im Monat auf den Teil über 5.000 AEQ;
  freie Adressen 1 % auf alles; Unternehmen Liegegeld nach Guthaben und
  anrechenbarem Monatsumsatz (Sockel 2.000, frei bis 1,5 Monatsumsätze,
  0,5 % bis 3, darüber 1 %). Das erste Unternehmen einer Gründerin zahlt nie
  mehr als „wie ein Mensch“.
- **Ausstiegsabgabe:** 2 % auf Tausch in Stable, Menschen 3.000 AEQ im Monat frei.
- **Kappung:** Menschen und freie Adressen über 25 × Durchschnitt.
- **Anrechenbarer Umsatz:** Einkäufe eines Menschen zählen je Unternehmen
  höchstens 9.000 AEQ im Quartal; Rückzahlungen an denselben Menschen heben
  sie auf; zwischen Unternehmen der höhere Wert aus Überschuss und den je
  Zahler gedeckelten Eingängen.

Die Formeln stehen im Modell ein zweites Mal, mit den Zahlen als Parametern.
`TestWirtschaftsmodell_FormelnGleichDerKette` vergleicht sie bei den heutigen
Zahlen mit den Funktionen der Kette (`gebuehrMitWirtschaft`,
`umlaufBetragRoh`, `ausstiegsAbgabe`, `liegegeldLocked` in beiden Zweigen) --
ändert jemand eine Formel nur an einer Stelle, wird der Test rot.

**Was es nicht rechnet:** Preise, Verhalten, Wachstum, Pool und Kurs, die
30-Tage-Mindestdaten eines Knotens, die Grenze freier Adressen bei der
Annahme. Es zeigt, wen welche Zahl trifft -- nicht, wie sich eine Wirtschaft
entwickelt. Die Beträge der Akteure sind Annahmen für eine kleine Stadt,
keine Messung; vor echtem Geld gehören sie durch Messwerte aus dem Pilot
ersetzt (Zahlenprüfung 4.1) und das Modell durch eine Ökonomin oder einen
Ökonomen geprüft (4.3).

## Die Akteure bei den heutigen Zahlen (12 Monate)

| Akteur | Art | Start | Ende nach 12 Mon. | Abgaben/Monat | davon Gebuehr | Umlauf/Liegegeld | Ausstieg | Kappung | Belastung % vom Guthaben/Monat | anrechenbarer Umsatz/Monat |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| Mensch mit wenig | mensch | 300 | 540 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.000 | – |
| Mensch mit viel | mensch | 30000 | 20711 | 474.12 | 1.30 | 91.55 | 0.00 | 381.27 | 2.076 | – |
| Baeckerei | unternehmen | 4000 | 6328 | 6.00 | 6.00 | 0.00 | 0.00 | 0.00 | 0.114 | 9000 |
| Supermarkt | unternehmen | 40000 | 51460 | 45.00 | 45.00 | 0.00 | 0.00 | 0.00 | 0.097 | 60000 |
| Grosshaendler | unternehmen | 120000 | 141840 | 180.00 | 180.00 | 0.00 | 0.00 | 0.00 | 0.137 | 120000 |
| Huelle | unternehmen | 50000 | 44546 | 454.46 | 0.00 | 454.46 | 0.00 | 0.00 | 0.967 | 0 |
| Absprache unter Freunden | unternehmen | 30000 | 26819 | 265.10 | 0.00 | 265.10 | 0.00 | 0.00 | 0.939 | 0 |
| Freie Adresse (Kasse) | frei | 200 | 176 | 2.03 | 0.15 | 1.88 | 0.00 | 0.00 | 1.091 | – |

Ins Grundeinkommen von diesen Akteuren: 17120.58 AEQ in 12 Monaten (1426.71 je Monat) -- verteilt auf 1000 Menschen 1.4267 AEQ je Mensch und Monat.

## Jede Zahl halbiert und verdoppelt

| Parameter| Wert | Mensch mit wenig | Mensch mit viel | Baeckerei | Supermarkt | Grosshaendler | Huelle | Absprache unter Freunden | Freie Adresse (Kasse) | 
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| AusstiegBps ×0.5 | 100 | · | · | · | · | · | · | · | · | 
| AusstiegBps ×2 | 400 | · | · | · | · | · | · | · | · | 
| Durchschnitt ×0.5 | 500 | · | +985.79 | · | · | · | · | · | · | 
| Durchschnitt ×2 | 2000 | · | -360.81 | · | · | · | · | · | · | 
| FreiAusgabenMonat ×0.5 | 500 | +0.38 | +0.44 | · | · | · | · | · | · | 
| FreiAusgabenMonat ×2 | 2000 | · | -0.89 | · | · | · | · | · | · | 
| FreiGrenze ×0.5 | 125 | · | · | · | · | · | · | · | · | 
| FreiGrenze ×2 | 500 | · | · | · | · | · | · | · | · | 
| FreiUmlaufMonat ×0.5 | 0.005 | · | · | · | · | · | · | · | -0.92 | 
| FreiUmlaufMonat ×2 | 0.02 | · | · | · | · | · | · | · | +1.69 | 
| GebuehrBps ×0.5 | 5 | · | -0.58 | -3.00 | -22.50 | -90.00 | · | · | -0.07 | 
| GebuehrBps ×2 | 20 | · | +1.16 | +6.00 | +45.00 | +180.00 | · | · | +0.14 | 
| GrenzeFaktor ×0.5 | 12.5 | · | +985.79 | · | · | · | · | · | · | 
| GrenzeFaktor ×2 | 50 | · | -360.81 | · | · | · | · | · | · | 
| LiegeRate1Monat ×0.5 | 0.0025 | · | · | · | · | · | · | · | · | 
| LiegeRate1Monat ×2 | 0.01 | · | · | · | · | · | · | · | · | 
| LiegeRate2Monat ×0.5 | 0.005 | · | · | · | · | · | -220.95 | -128.89 | · | 
| LiegeRate2Monat ×2 | 0.02 | · | · | · | · | · | +406.67 | +237.23 | · | 
| Sockel ×0.5 | 1000 | · | · | · | · | · | +9.47 | +9.47 | · | 
| Sockel ×2 | 4000 | · | · | · | · | · | -18.94 | -18.94 | · | 
| SparFreibetrag ×0.5 | 2500 | · | +11.18 | · | · | · | · | · | · | 
| SparFreibetrag ×2 | 10000 | · | -22.35 | · | · | · | · | · | · | 
| TauschFreiMonat ×0.5 | 1500 | · | · | · | · | · | · | · | · | 
| TauschFreiMonat ×2 | 6000 | · | · | · | · | · | · | · | · | 
| UmlaufMonat ×0.5 | 0.0025 | · | -40.10 | · | · | · | · | · | · | 
| UmlaufMonat ×2 | 0.01 | · | +77.15 | · | · | · | · | · | · | 
| UmsatzFreiFaktor ×0.5 | 0.75 | · | · | · | +10.38 | +204.02 | · | · | · | 
| UmsatzFreiFaktor ×2 | 3 | · | · | · | · | · | · | · | · | 
| UmsatzStufe2Faktor ×0.5 | 1.5 | · | · | · | · | · | · | · | · | 
| UmsatzStufe2Faktor ×2 | 6 | · | · | · | · | · | · | · | · | 
| ZaehltJeQuartal ×0.5 | 4500 | · | · | · | · | +204.02 | · | · | · | 
| ZaehltJeQuartal ×2 | 18000 | · | · | · | · | · | · | · | · | 

Werte: Aenderung der Abgaben je Monat (AEQ) gegenueber den heutigen Zahlen; · = keine Aenderung.

## Was daraus folgt

- **Wer wenig hat, zahlt nichts.** Unter den Freibeträgen (1.000 AEQ
  Ausgaben, 5.000 AEQ Guthaben) fällt beim Menschen mit wenig keine Abgabe an.
- **Die Hülle trägt die Last, nicht der Laden.** Eine Firma ohne Umsatz zahlt
  rund 1 % ihres Guthabens im Monat; Bäckerei, Supermarkt und Großhändler
  zahlen nur die Gebühr auf Zahlungen an andere Unternehmen.
- **Die Absprache unter Freunden bringt nichts.** Einkäufe, die als „Lohn“ an
  dieselben Menschen zurückgehen, zählen nicht als Umsatz; die Firma zahlt
  Liegegeld wie eine Hülle.
- **Der Großhändler hängt am Deckel je Zahler.** Halbiert man
  `ZaehltJeQuartal` oder `UmsatzFreiFaktor`, rutscht er ins Liegegeld -- die
  Zahl ist für Großhandel eng bemessen (Zahlenprüfung 3.3).
- **Die Kappung wirkt sofort und stark.** Wer über 25 × Durchschnitt liegt,
  gibt den Überschuss ab; halbiert man den Faktor oder den Durchschnitt, steigt
  die Abgabe des vermögenden Menschen um ein Vielfaches.
- **Gebühren treffen Unternehmen linear.** Eine Verdopplung von `GebuehrBps`
  verdoppelt die Last aller Firmen mit Zahlungen an andere Firmen.
