# Minibuch zum 60. Geburtstag

Ein A5-Buch, geschrieben in einfachen Textdateien, gedruckt als PDF.

## In 30 Sekunden loslegen

```bash
./bauen.sh          # erzeugt bau/minibuch.pdf
```

Das war's. Danach: eine Datei in `inhalt/` öffnen, schreiben, neu bauen.

## Wie es funktioniert

```
inhalt/       die Kapitel, je eine Textdatei — hier passiert deine Arbeit
bilder/       Fotos, die ins Buch sollen
werkzeug/     Bauskript und Layout (musst du nicht anfassen)
bau/          das Ergebnis: minibuch.html und minibuch.pdf
buch.json     Notizen zu Titel, Namen, Jahreszahlen
```

Die Dateien in `inhalt/` werden nach Dateinamen sortiert und
aneinandergehängt. Willst du ein Kapitel verschieben, benenn es um
(`23-…` wird vor `24-…` einsortiert). Willst du eins weglassen,
lösch die Datei.

## Schreiben

In jeder Kapiteldatei steht oben ein Kommentarblock:

```
<!-- SCHREIBHILFE ... -->
```

Da stehen konkrete Fragen, Satzanfänge und wie lang das Kapitel werden
soll. **Diese Blöcke landen nie im fertigen Buch** — sie sind nur für dich.
Du kannst sie stehen lassen oder löschen.

Alles, was noch fehlt, steht in doppelten geschweiften Klammern:

```
{{Hier kommt noch was hin}}
```

Solche Stellen werden im PDF gelb markiert, und `./bauen.sh` sagt dir nach
jedem Lauf, wie viele noch offen sind. Wenn die Zahl null ist, bist du fertig.

## Was du schreiben kannst

| Schreibweise | Ergebnis |
|---|---|
| `# Titel` | Kapitelüberschrift |
| `## Zwischentitel` | Zwischenüberschrift |
| `### Kleine Marke` | kleine Zwischenmarke |
| `> Text` | Zitat, eingerückt und kursiv |
| `- Punkt` | Aufzählung |
| `1. Punkt` | nummerierte Liste |
| `**fett**` / `*kursiv*` | fett / kursiv |
| `![Bildunterschrift](bilder/foto.jpg)` | Foto mit Unterschrift |
| `---` (eigene Zeile) | Seitenumbruch erzwingen |
| `~~~` (eigene Zeile) | Zierlinie ❦ |
| `{{...}}` | offene Lücke |
| `4\. September` | Datum am Zeilenanfang, wird keine Liste |

Der letzte Punkt ist wichtig: Eine Zeile, die mit **Zahl, Punkt, Leerzeichen**
beginnt, gilt als nummerierte Liste. Ein Datum am Zeilenanfang braucht deshalb
einen Schrägstrich vor dem Punkt — sonst steht im Buch plötzlich `1.` statt `4.`

Leerzeile = neuer Absatz. Sonst gilt: einfach schreiben.

## Fotos

Leg sie in `bilder/` und binde sie mit `![...](bilder/name.jpg)` ein.
JPG oder PNG, gern 1500 px lange Kante — größer bringt im Druck nichts,
kleiner wird unscharf. Alte Papierfotos einfach bei gutem Tageslicht
mit dem Handy abfotografieren, das reicht völlig.

## Handschriftliche Beiträge

Zwei Formate stehen bereit:

```
![handschrift Anette, ihre Schwester](bilder/anette.jpg)
![unterschrift Anette](bilder/anette-name.jpg)
```

`handschrift` setzt das Bild groß, fast auf Seitenhöhe — für einen
vollständig handgeschriebenen Beitrag.
`unterschrift` setzt es klein und linksbündig — für den Namenszug unter
einem gesetzten Text.

**Empfehlung:** Beitrag setzen, Unterschrift scannen. Dann bleibt alles
lesbar, und die persönliche Spur ist trotzdem da. Ganze Seiten nur dort
handschriftlich, wo die Handschrift selbst die Botschaft ist.

### So wird das Foto brauchbar

- **Weißes, unliniertes Papier.** Liniertes sieht im Druck unruhig aus.
  Gib den Leuten am besten gleich ein paar Blätter mit.
- **Blaue oder schwarze Tinte, kein Bleistift.** Bleistift verschwindet
  beim Kopieren fast vollständig.
- **Tageslicht, keine Deckenlampe.** Am Fenster, aber nicht in der
  prallen Sonne — Schatten des eigenen Kopfes vermeiden.
- **Senkrecht von oben** fotografieren, Blatt formatfüllend, Kanten
  parallel zum Bildrand.
- Danach am Handy **Helligkeit hoch und Kontrast hoch** ziehen, bis das
  Papier weiß statt grau ist. Das ist der Schritt, der den größten
  Unterschied macht.
- Wenn möglich: **A5 schreiben lassen** oder ein A4-Blatt quer
  halbieren. Dann muss beim Druck nichts verkleinert werden.

## Alte Fotos ins Buch bringen

Die Bilder liegen in `inhalt/bilder/` und werden beim Bauen als
data-URI direkt in die HTML-Datei einbettet. `bau/minibuch.html` ist
dadurch eine einzige, in sich geschlossene Datei.

Im Text einbinden mit einer von vier Breiten:

```
![foto-s  Bildunterschrift](bilder/x.jpg)     66 mm
![foto-m  Bildunterschrift](bilder/x.jpg)     82 mm
![foto-l  Bildunterschrift](bilder/x.jpg)     96 mm
![foto-xl Bildunterschrift](bilder/x.jpg)    112 mm — volle Satzbreite
```

Die Breite richtet sich nach der **Auflösung**, nicht nach dem Motiv:
Ein Bild soll im Druck nicht unter etwa 250 dpi rutschen. Faustregel —
Pixelbreite geteilt durch 10 ergibt ungefähr die maximale Breite in
Millimetern. Ein 800 Pixel breites Bild verträgt also rund 80 mm.

Fehlt eine Bilddatei, sagt das Bauskript das beim Bauen an — es bricht
nicht ab.

### Abzüge aufbereiten

`werkzeug/fotos-aufbereiten.py` entzerrt abfotografierte Abzüge und
nimmt den Farbstich heraus. Es braucht `pillow` und `numpy`:

```
pip install pillow numpy
python3 werkzeug/fotos-aufbereiten.py
```

Die vier Eckpunkte jedes Abzugs stehen in `werkzeug/fotos-ecken.json`,
Beschnitt und Korrekturstärke in der Tabelle `PLAN` im Skript selbst.

### So wird das Abfotografieren gut

- **Senkrecht von oben**, Abzug formatfüllend, Kanten möglichst parallel
  zum Bildrand. Je weniger schräg, desto weniger muss gerechnet werden.
- **Tageslicht am Fenster**, keine Deckenlampe und kein Blitz —
  Hochglanzabzüge spiegeln sonst.
- **Kein Ausschnitt.** Lieber den ganzen Abzug mit etwas Rand drumherum;
  beschneiden lässt sich später, dazuerfinden nicht.
- **Nichts am Handy nachbearbeiten.** Filter und Auto-Korrektur machen
  die Entzerrung schlechter, nicht besser.

**Besser als jedes Handyfoto ist ein Flachbettscanner mit 600 dpi.** Ein
9×13-Abzug ergibt damit rund 2100 × 3000 Pixel — genug für volle
Satzbreite in bester Qualität. Die Handyfotos reichen für 66 bis 112 mm.
Wenn irgendwo in der Familie ein Drucker mit Scanner steht, lohnt der
Umweg für die wichtigsten Bilder.

## Drucken lassen

`bau/minibuch.pdf` ist bereits **A5, einseitig fortlaufend**. Damit gehst du
in einen Copyshop und sagst:

> „A5, beidseitig bedruckt, Klebebindung oder Spiralbindung,
> 120g Papier für den Innenteil, festerer Karton als Umschlag."

Kostet je nach Umfang meist 10–25 €. Ein Tag Vorlauf reicht normalerweise.

Alternative: Online-Anbieter (epubli, BoD) — schöner gebunden, dafür
brauchst du 1–2 Wochen Lieferzeit.

**Wichtig:** Drucke dir vorher einmal eine Testversion auf normalem Papier
aus und lies sie laut. Man findet damit mehr Fehler als am Bildschirm.

## Kein Node auf deinem Rechner?

`bauen.sh` braucht Node.js. Ohne Node kannst du `bau/minibuch.html`
einfach im Browser öffnen und mit Strg+P / Cmd+P als PDF drucken —
Papierformat **A5**, Ränder **keine**, Hintergrundgrafiken **an**.

## Seitenzahlen

Das PDF hat bewusst keine Seitenzahlen — Chromium kann sie beim
Kommandozeilendruck nicht sauber setzen, und das Inhaltsverzeichnis
kommt ohne aus. Wenn du unbedingt welche willst: `bau/minibuch.html` im
Browser öffnen, Strg+P, und im Druckdialog „Kopf- und Fußzeilen"
aktivieren.
