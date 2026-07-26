# UBI-Verteilung bei großer Nutzerzahl — Entwurf, keine Implementierung

Roadmap-Schritt 3 lautet „UBI auf Lazy-Claim", erwartete Wirkung „O(N)-Kettenstillstand entfällt", blockiert durch „nichts".

Dieses Dokument kommt zu einem anderen Schluss: **der Schritt ist nicht durch Technik blockiert, sondern durch eine ökonomische Entscheidung, die der Roadmap-Eintrag nicht benennt** — und es gibt eine Alternative, die dieselbe Wirkung erzielt, ohne diese Entscheidung überhaupt zu erzwingen.

Geschrieben als Entwurf, weil die frühere Bewertung (2026-07-12) ausdrücklich festhielt, das brauche „eine eigene Design-Sitzung, keinen übereilten Patch".

## Das Problem, konkret

`distributeUBIPoolLocked` hält `cs.mu` (Schreibsperre, blockiert damit **jede** Transaktion) über:

1. Aufzählung aller `is_human`-Adressen aus Postgres
2. `ensureAccountsLoadedCtx` — alle N Konten in den Speicher laden
3. Schleife 1: Demurrage für alle N abrechnen
4. Schleife 2: alle N gutschreiben, Vermögensgrenze je Konto prüfen
5. Ein Batch-Schreibvorgang über N Zeilen

Schritt 5 wurde am 2026-07-21 bereits von N Einzelschreibvorgängen auf einen Batch reduziert — die verbleibenden O(N)-Anteile sind das Laden, die beiden Schleifen und die Größe des Batches selbst.

Bei 14 Menschen ist das unmessbar. Bei 1 Mio. Menschen ist es eine mehrminütige Vollsperre, einmal täglich.

## Warum Lazy-Claim hier teurer ist, als es aussieht

Der Entwurf wäre: statt N Konten gutzuschreiben, eine Epoche eintragen (`epoch_id`, Zeitpunkt, Anteil je Mensch), den Pool nullen, und jedes Konto beim nächsten Zugriff nachträglich gutschreiben. Beides O(1).

Technisch lösbar, inklusive StateRoot: Der Wurzelwert ist ein XOR-Akkumulator über Konto-Blätter, und ein Epochen-Register ließe sich als eigene Komponente mit einbeziehen — Konto-Blatt plus offene Epochen ergibt deterministisch dasselbe auf jedem Knoten.

**Der Haken ist nicht technisch.**

`settleDemurrageLocked` lässt untätige Guthaben verfallen und schüttet den Verfall anteilig in die Pools zurück, 20 % davon in genau den UBI-Pool. Das ist der Anti-Hortungs-Mechanismus der Kette.

Nicht abgeholtes UBI läge außerhalb des Kontoguthabens und würde damit **nicht verfallen**. Wer nicht abholt, entgeht der Demurrage — Nicht-Abholen wird zur profitablen Strategie. Das kehrt die Absicht des Mechanismus um, und zwar nicht als Randfall, sondern für jeden, der es merkt.

Die Auswege sind allesamt Entscheidungen über Wirtschaftsregeln, nicht über Code:

- **Demurrage auf nicht abgeholtes UBI anwenden.** Wohin fließt der Verfall? Zurück in den UBI-Pool, aus dem es stammt — dann zirkuliert derselbe Betrag zwischen Pool und Anspruch, ohne je jemanden zu erreichen.
- **Nicht abgeholtes UBI verfällt nach X Epochen.** Ändert die Zusage „jeder Mensch bekommt täglich seinen Anteil" in „…, wenn er rechtzeitig zugreift". Eine soziale Entscheidung, keine technische.
- **Demurrage bewusst aussetzen.** Macht die Lücke zur akzeptierten Regel und muss dann auch so kommuniziert werden.

Dazu kommt die Vermögensgrenze: `enforceWealthCapLocked` greift heute im Moment der Verteilung. Bei Lazy-Claim greift sie beim Abholen — ein Konto kann die Grenze zwischenzeitlich überschreiten, und wer wann abholt, verändert das Ergebnis.

## Die Alternative, die keine dieser Fragen stellt

**Verteilung über mehrere Blöcke stückeln, statt sie zu verschieben.**

Die Verteilung bleibt ein Push mit unveränderter Semantik, wird aber deterministisch in Abschnitte zerlegt: pro Block ein fester Höchstwert an Konten (z. B. 5.000), in stabiler Reihenfolge (nach Adresse sortiert), bis alle durch sind. Der Anteil je Mensch wird **einmal zu Beginn** festgelegt und mit der Epoche festgeschrieben, damit alle Abschnitte denselben Betrag verwenden.

Was das leistet:

- `cs.mu` wird pro Block nur für einen beschränkten Abschnitt gehalten — Dauer unabhängig von N
- Demurrage, Vermögensgrenze und Gesamtangebot bleiben **exakt** wie heute; es gibt keine ökonomische Änderung, über die zu entscheiden wäre
- Deterministisch und damit replayfähig: Abschnittsgrenzen ergeben sich aus Epoche und sortierter Adressliste, nicht aus der Ankunftszeit
- Keine „virtuellen" Guthaben, keine zweite Bilanzgröße, kein Sonderfall im StateRoot

Was es nicht leistet: Die Gesamtarbeit bleibt O(N), sie wird nur über Blöcke verteilt. Bei 1 Mio. Menschen und 5.000 pro Block sind das 200 Blöcke, also bei `BLOCK_TIME=1s` gut drei Minuten, in denen die Kette normal weiterläuft, statt drei Minuten Stillstand.

## Was noch zu klären ist, bevor irgendetwas gebaut wird

1. **Ist die Stückelung ausreichend?** Sie beseitigt den Stillstand, nicht die Gesamtarbeit. Wenn tägliches O(N) grundsätzlich unerwünscht ist, führt kein Weg an der ökonomischen Entscheidung oben vorbei.
2. **Aktivierungshöhe.** Beide Varianten ändern, welche Blöcke welche Gutschriften enthalten — also Konsens. Gestaffelter Rollout nötig, wie bei jeder Konsensänderung hier.
3. **Wechselwirkung mit `ApplyUBIDelta`.** Sekundärknoten spielen exakte Beträge nach; die Abschnittsbildung muss in den Deltas sichtbar sein, nicht nur beim Produzenten.
4. **Messung zuerst.** Bei 14 Menschen ist die Verteilung heute unmessbar. Bevor sie umgebaut wird, gehört gemessen, wie lange `distributeUBIPoolLocked` bei realistisch großer Kontozahl tatsächlich `cs.mu` hält — das Harness dafür existiert (`scaling_unknowns_bench_test.go`, opt-in) und der Weg auf die Zielhardware ist etabliert (`bench-signature-cgo-contabo2.yml`).

## Empfehlung

Nicht Lazy-Claim. **Stückelung**, weil sie dieselbe Wirkung auf die Sperrdauer hat und dabei keine einzige Wirtschaftsregel anfasst — und weil eine Änderung, die niemand ökonomisch bewerten muss, ungleich leichter sicher auszurollen ist.

Und erst nach Punkt 4: Solange die Sperrdauer bei realistischer Kontozahl nicht gemessen ist, wäre auch die Stückelung eine Lösung für ein Problem unbekannter Größe.
