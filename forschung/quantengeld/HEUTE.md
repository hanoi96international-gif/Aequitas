# Quantengeld heute – was unter den heutigen Umständen funktionieren kann

Stand 16.11.2026. Ergänzt `ANALYSE.md` um die Frage: **Was davon kann Aequitas heute
schon sein, und was fehlt dafür noch?** Grundlage sind die Messungen in diesem Ordner,
der Code im Knoten und in der App sowie die Quellen in `ANALYSE.md`.

## Die Antwort in Kürze

1. **Echtes Quantengeld** (die Noten selbst sind Quantenzustände, und Menschen tragen
   sie bei sich) **funktioniert heute nirgends auf der Welt.** Das liegt nicht an
   Aequitas. Es gibt weder Quantenspeicher in Geräten noch Quantenleitungen zu
   Endnutzern. Wer es verspricht, verspricht etwas, das niemand liefern kann. Unsere
   eigene Regel lautet deshalb: Wir werben nie mit „Quantenwährung“
   (`docs/QUANTENSICHERHEIT.md`).
2. **Heute funktionieren können vier Dinge, und Aequitas kann alle vier sein:**
   - **(a) quantensicheres Geld für jeden, ab Werk:** Kein Quantenrechner kann AEQ
     stehlen oder Menschen erfinden;
   - **(b) die erste laufende Ausgabestelle für Quantengeld:** Register, Deckung,
     Abgabe, Freischaltung, Einlösen;
   - **(c) echte Quanten-Noten von Anfang bis Ende auf echter Hardware,** als
     Vorführung im Labormaßstab;
   - **(d) Bargeld-Verhalten ohne Quanten:** offline zahlen mit Haftung (Offline-Bargeld).
3. **Davon ist (b) fertig, (a) zu großen Teilen.** Für (a) fehlt vor allem, dass der
   Quantenschutz **automatisch** an ist. Heute muss jeder ihn selbst einschalten. (c)
   lässt sich mit dem vorhandenen IBM-Zugang in Tagen bauen. (d) ist zur Hälfte gebaut.

## 1. Drei Bedeutungen von „funktionierendem Quantengeld“

| Bedeutung | Was es heißt | Heute möglich? | Aequitas heute |
|---|---|---|---|
| **Quantensicheres Geld** | Geld, das auch ein Quantenrechner nicht stehlen, fälschen oder erfinden kann. Der Schutz kommt aus Mathematik (Post-Quanten-Kryptografie), nicht aus Physik. | **ja**, auf normalen Handys | Stufen 1–3, 3.6, 5, 5b, 1b live; Lücken in Abschnitt 3.1 |
| **Physikalisches Quantengeld** | Die Note ist ein Quantenzustand und lässt sich physikalisch nicht kopieren. | **nein** für den Alltag; **ja** als Vorführung auf Laborhardware | Ausgabestelle live (schlafend); Noten auf IBM gemessen (Z₈, ~20 µs) |
| **Quanten-Zufall** | Zufall aus einem Quantenrechner, dessen Echtheit klassisch geprüft wird (z. B. Quantinuum/JPMorgan, Nature 2025). | ja, bei wenigen Anbietern | nicht genutzt; wäre ein Baustein für faire Auswahl, kein Geld |

## 2. Die harten Grenzen von heute

| Grenze | Beleg | Folge |
|---|---|---|
| Eine Note lebt auf IBM etwa **20 µs**, auf Ionenfallen **Sekunden** | eigene Messung (`HARDWARE_WEG.md`); Datenblatt Quantinuum | Eine Note überlebt keinen Gang von Hand zu Hand |
| Ein Cloud-Lauf endet mit der Messung, nichts **überdauert einen Lauf** | Arbeitsweise aller Cloud-Quantenrechner | Eine Note existiert nur innerhalb **eines** Laufs |
| Ein Cloud-Lauf ist **gegenüber Dritten nicht als echt beweisbar** | Das Ergebnis meldet der Betreiber | Eine „Quanten-Note in der Cloud“ ist nicht sicherer als eine Buchung. Sie zeigt, dass der **Ablauf** funktioniert, schützt aber nichts. |
| Endnutzer haben **keinen Quantenspeicher und keine Quantenleitung** | Speicher > 10 h nur im Labor (Wang 2025); Quantennetze sind Testbetten | Keine Note in der Hosentasche, auf Jahre |
| Öffentliche Prüfung braucht **fehlerkorrigierte Speicher**; Einlösen braucht **iO** | `rauschen_oeffentlich.py`; Huang & Vaikuntanathan 2026, Satz 1.1 | Der Weg der Bauvorgabe liegt in der fehlertoleranten Ära |

**Folgerung:**
- **Physikalisches Quantengeld** ist heute eine **Vorführung**, kein Schutz.
- **Quantensicheres Geld** ist heute ein **echter Schutz**. Dort liegt der größte Nutzen
  für Menschen mit AEQ.

## 3. Was heute funktioniert – und was dafür noch fehlt

### 3.1 Quantensicheres Geld für jeden (größte Wirkung)

**Live:**
- Registrierung hybrid (Stufe 1, scharf nach der ersten echten Registrierung);
- PQ-Zusage je Wallet (Stufe 2);
- PQ-Adressen und Überweisungen mit ML-DSA-65 (Stufe 3, Tresor);
- Menschsein und Grundeinkommen in den Tresor (3.6);
- PQ-Stempel der Blöcke (5);
- Prüfer-Stimmen hybrid (5b);
- PQ-Schlüssel der Prüfer-Dienste (1b).

**Lücken:**

| Lücke | Warum sie zählt | Was zu tun ist | Wer |
|---|---|---|---|
| **Quantenschutz ist freiwillig.** Die App bietet „Quantenschutz aktivieren“ als Karte an, der Tresor ist ein eigener Schritt. | Eine Wallet ohne Zusage ist nach ihrer ersten Überweisung angreifbar, sobald es einen passenden Quantenrechner gibt. Die meisten Menschen schalten nichts selbst ein. | **Ab Werk:** Die App hinterlegt die Zusage nach der Registrierung selbst. Der Schlüssel kommt aus derselben Seed-Phrase, der Mensch muss nichts tun. Für Wallets ohne Seed-Phrase in der App (WalletConnect) bleibt es ein Hinweis. | ich (App), danach dein Release |
| **Stufe 0:** Biometrie-Sicherung hybrid | Gesichtsdaten im Backup wären sonst später entschlüsselbar („jetzt speichern, später knacken“) | PQ-Schlüssel des Betreibers anlegen | du |
| **Stufe 1 scharf** | Seit dem Neustart bei null fehlt die erste echte Registrierung | dich registrieren (App 1.21+) | du |
| **Vom Tresor aus noch nicht möglich:** Prüfer werden, Wächter setzen, Treuhand zurückholen, Unternehmen, EVM-Verträge | Wer ganz in den Tresor umzieht, verliert diese Wege | je ein eigener Schritt (Knoten, App) | ich, nacheinander |
| **Komitee-Checkpoints hybrid** schlafend; libp2p klassisch | Betrifft heute nur öffentliche Kettendaten | erst, wenn es mehrere Erzeuger gibt bzw. go-libp2p hybride Schlüssel kann | später |
| **Notbremse** (Stufe 4) schlafend | Sie ist richtig so: erst scharf, wenn ein Rechner in Reichweite ist | nichts | – |

### 3.2 Laufende Ausgabestelle für Quantengeld (fertig)

**Fertig gebaut sind:**
- Register und Topf mit Umlaufabgabe;
- Freischaltung durch ⅔ der Prüfer, mindestens 5;
- Einlösen, gesperrt bis zu einem geprüften Verfahren;
- QG3 (Noten je Zeile).

**Live ist nur ein Teil davon.** Mindestzahl und QG3 werden erst mit den beiden
Deploys aktiv, die auf deine Freigabe warten. Das ist die Seite, die eine Kette
leisten muss, und sie ist da. Ein Satz wird erst freigeschaltet, wenn es Noten gibt,
die tragen. Bis dahin ist „schlafend“ der ehrliche Zustand.

### 3.3 Echte Quanten-Noten von Anfang bis Ende (Vorführung, Labormaßstab)

Mit dem vorhandenen IBM-Zugang (Umgebung `quantenhardware`, 14 Läufe bisher) und dem
vorhandenen Läufer (`hardware_ibm.py`: dynamische Schaltkreise, Warten, Teleportation)
geht heute in **einem** Lauf:
1. **Prägen** einer Variante-W-Note (8–16 Qubits, BB84). Basen und Bits legt vorher die
   Kette fest und verteilt sie als Anteile, wie in `wiesner_verteilt.py`.
2. **Halten** für eine Wartezeit (0–1.000 µs).
3. **Bezahlen** per Teleportation an ein zweites Register (den Empfänger).
4. **Prüfen, klassisch:** Die Basen der Aufforderung kommen aus Quanten-Zufall im selben
   Lauf. Messen, dann prüfen die (simulierten) Validatoren auf den Anteilen.
5. **Ergebnis:** Lauf-ID, Messwerte und Urteil, veröffentlicht und nachrechenbar.

**Was das bringt:**
- **Erstens** zeigt es, dass der ganze Ablauf auf echter Hardware läuft.
- **Zweitens** liefert es **echte Fehlerraten** je Qubit und Wartezeit. Genau daraus
  folgen Toleranz und Notengröße der Variante W. Bisher sind sie nur angenommen (2 %).

**Was es nicht bringt:** Sicherheit. Die Note ist zu klein und lebt Mikrosekunden.
Prägestelle und Halter sind derselbe Schaltkreis, und den Lauf bezeugt nur IBM. So muss
es auch dastehen: als **Vorführung**, mit Symbolwert, nie mit echten AEQ.

**Kosten:** wenige Sekunden Rechenzeit aus dem Freikontingent. Ein Lauf braucht deine
Freigabe der Umgebung `quantenhardware`.

### 3.4 Bargeld-Verhalten ohne Quanten (Offline-Bargeld)

Was Menschen sich von Quantengeld wünschen, ist meist offline zu zahlen, wie mit
Bargeld. Das leistet das Offline-Bargeld (`docs/OFFLINE_BARGELD.md`):
- **Mechanismus:** Ein Geräteschlüssel im Android-Keystore, ein Pfand und die Haftung
  über „ein Mensch, ein Konto“.
- **Grenze:** Betrug ist nicht unmöglich, aber beweisbar, und der Betrüger haftet.
- **Stand:** Stufe 1 ist gebaut und schläft. Offen sind Stufe 2 (Knoten-API), 3
  (App-Modul, QR), 4 (Schuld aus künftigen Eingängen) und 5 (Stichtag). Das ist nur
  Software.

## 4. Was heute nicht behauptet werden darf

- „Aequitas ist Quantengeld“ oder „Quantenwährung“. Richtig ist:
  **quantensicher** (Kryptografie) plus **Ausgabestelle für Quantengeld** (schlafend)
  plus **Vorführung auf echter Hardware**.
- „Physikalisch unfälschbar“ für AEQ. AEQ ist durch Mathematik geschützt (ML-DSA),
  nicht durch Physik.
- Eine Note aus einem Cloud-Lauf als Wertträger. Sie beweist den Ablauf, nicht die
  Echtheit.

Diese Unterscheidung ist auch rechtlich klug, denn irreführende Werbung ist angreifbar.
Für die Menschen, die AEQ halten, ist sie ehrlich.

## 5. Das Heute-Paket

| # | Schritt | Wirkung | Aufwand | Wer |
|---|---|---|---|---|
| 1 | Deploys #534 (Mindestzahl 5 Prüfer) und #535 (Noten je Zeile) freigeben | Ausgabestelle auf letztem Stand | Minuten | du |
| 2 | **Quantenschutz ab Werk** in der App (Zusage automatisch nach der Registrierung), Release | jede neue Wallet quantensicher, ohne Zutun | Tage | ich, dann dein Release |
| 3 | Dich registrieren → Stufe 1 wird scharf | Registrierung vollständig hybrid geprüft | Minuten | du |
| 4 | PQ-Schlüssel für die Biometrie-Sicherung (Stufe 0) | Gesichtsdaten im Backup quantensicher | Minuten | du |
| 5 | **Vorführung „Quanten-Note Nr. 1“** auf IBM (Abschnitt 3.3) | echter Ablauf auf echter Hardware, echte Fehlerraten für Variante W | Tage | ich; Lauf mit deiner Freigabe |
| 6 | Offline-Bargeld Stufe 2 | Weg zu „offline zahlen“ für alle | Wochen | ich |
| 7 | Tresor-Lücken schließen (Prüfer, Wächter, Treuhand vom Tresor aus) | vollständiger Umzug in den Tresor möglich | Wochen | ich |
| 8 | Partner und Förderung (Vorlagen in `ANSCHREIBEN.md`) | Weg zu Noten mit Speichern von Stunden | Monate | du schickst ab |

**Am Ende von 1–5** kann Aequitas ehrlich sagen: *Jedes neue Konto ist ab Werk
quantensicher. Die Kette hat eine laufende Ausgabestelle für Quantengeld. Der ganze
Ablauf einer Quanten-Note ist auf echter Quantenhardware vorgeführt.* Jeder dieser drei
Sätze ist nachprüfbar: im Code, auf der Kette und im veröffentlichten Lauf.
