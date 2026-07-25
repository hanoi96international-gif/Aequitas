# Content Agency

Ein eigenständiges Multi-Agenten-System, das die **Produktionsarbeit** einer
kleinen Content-Agentur automatisiert: Recherche, Textentwurf, redaktionelle
Qualitätsprüfung — sowie eine Akquise-Hilfe, die potenzielle Kunden findet
und Anschreiben-Entwürfe vorbereitet.

**Wichtig — bewusste Grenze der Automatisierung:** Dieses Tool verschickt
**nichts automatisch** — keine E-Mails, keine Rechnungen, keine
Veröffentlichungen. Alles landet als Entwurf in `outbox/` zur menschlichen
Prüfung. Gründe:

- **Rechtlich**: unaufgefordert automatisiert verschickte Werbe-Mails können
  in Deutschland/der EU gegen UWG/DSGVO verstoßen. Das hier ist keine
  Rechtsberatung — prüfe das für deinen Fall, bevor du irgendetwas verschickst.
- **Reputation**: ein ungeprüfter Text oder ein unpassendes Anschreiben, das
  automatisch an einen echten (potenziellen) Kunden geht, kann mehr Schaden
  anrichten als es nützt.
- **Haftung**: bei Verträgen/Zusagen sollte immer ein Mensch die letzte
  Entscheidung treffen.

## Zwei Pipelines

### 1. Content-Pipeline (`pipeline_content.py`)

```
Researcher (Websuche nach Fakten)
      -> Writer (Entwurf nach Brief + Fakten)
      -> Editor (Faktencheck gegen Recherche + Brief-Konformität)
      -> bei Ablehnung: Writer revidiert (max. 2 Versuche, konfigurierbar)
      -> outbox/content_<slug>_<zeitstempel>.md + .json
```

Der Editor prüft explizit, ob jede Tatsachenbehauptung im Text durch die
Recherche gedeckt ist — unbelegte Behauptungen werden im Report markiert,
auch wenn der Entwurf am Ende "freigegeben" wird.

### 2. Akquise-Pipeline (`pipeline_leads.py`)

```
Prospect Finder (Websuche nach echten, passenden Unternehmen in einer Nische)
      -> Outreach Writer (kurzes, ehrliches, personalisiertes Anschreiben pro Lead)
      -> outbox/leads_<nische>_<zeitstempel>.md + .json  (nichts wird verschickt)
```

Der Prospect Finder erfindet keine Kontaktdaten — nur öffentlich auffindbare
Kontaktseiten werden gemeldet, sonst `null`.

## Setup

```bash
cd content-agency
python3 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
cp .env.example .env   # ANTHROPIC_API_KEY eintragen
```

In `config.py` vor dem ersten Lauf anpassen: `AGENCY_NAME`, `AGENCY_OFFER`,
`AGENCY_CONTACT` (echte Kontaktadresse für Antworten auf Anschreiben).

## Ausführen

```bash
# Einen Blogartikel nach Brief erstellen (Beispiel-Brief liegt in briefs/)
python main.py content --brief briefs/example_blog_post.json

# Potenzielle Kunden in einer Nische finden + Anschreiben entwerfen
python main.py leads --niche "kleine lokale Restaurants ohne aktuelle Speisekarte online" --max-leads 5

# Auftragsübersicht (einfacher JSON-Tracker, keine echte Zahlungsabwicklung)
python main.py orders
python main.py orders --add "Restaurant Zur Post" "Blogartikel 600 Wörter" 60
python main.py orders --status <order_id> paid
```

## Wie du tatsächlich erste Kunden gewinnst

1. **Portfolio bauen**: 2-3 Beispieltexte mit der Content-Pipeline zu
   plausiblen Themen erzeugen (auch ohne echten Auftrag), um bei der
   Akquise etwas vorzeigen zu können.
2. **Marktplätze zuerst**: Fiverr, Upwork, Content-Vermittlungen — dort
   kommen Kunden aktiv auf dich zu, kein Kaltakquise-Risiko.
3. **Akquise-Pipeline als Ergänzung**: `leads`-Kommando für eine konkrete,
   enge Nische laufen lassen, Entwürfe in `outbox/` durchsehen, nur die
   wirklich passenden manuell versenden — und rechtliche Anforderungen an
   Werbe-Mails vorher klären.

## Preisrichtwerte (`config.py` → `PRICING_GUIDE_EUR`)

Nur ein Startpunkt, keine Empfehlung für einen bestimmten Markt:

| Format | Richtpreis |
|---|---|
| Kurzer Blogartikel (~500-800 Wörter) | 40-80 € |
| Langer Blogartikel (~1200-2000 Wörter) | 80-150 € |
| Produktbeschreibung (kurz) | 10-25 € pro Produkt |

## Grenzen

- Keine Zahlungsabwicklung, keine Rechnungsstellung, kein CRM — `orders.py`
  ist eine reine Notiz-/Statusliste.
- Die Qualität hängt an der Qualität der Websuche zum jeweiligen Thema —
  bei sehr spezialisierten/aktuellen Themen ggf. Ergebnisse manuell
  gegenprüfen.
- Kein Nachweis, dass diese Nische/Preisgestaltung tatsächlich Nachfrage
  findet — das musst du am Markt testen, das System liefert nur die
  Produktionsseite schneller.
