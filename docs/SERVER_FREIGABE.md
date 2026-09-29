# Server-Zugang nur mit Freigabe

Stand 29.09.2026. Gehört zur Beta nach Launch-Maßstab (AGENTS.md, Abschnitt Sicherheit).

## Warum

201 Workflows in diesem Repo tragen SSH-Schlüssel mit Root-Zugang auf C1 und C2.
Bis heute brauchte keiner davon eine Freigabe, und 24 starteten schon, wenn nur ein
Branch gepusht wurde. **Wer Schreibrechte auf GitHub hatte, hatte damit Root auf beiden
Servern.** Das gilt für jeden Menschen mit Schreibrechten, für jede Automatisierung und
für einen gestohlenen GitHub-Zugang.

Ein Push wirkt dabei mit der Workflow-Datei des **gepushten** Branches. Eine Änderung nur
auf `main` reicht deshalb nicht: Ein alter Branch mit einer alten Datei konnte die
Schlüssel weiter benutzen. Das schließt nur eine GitHub-Umgebung mit eigenen Secrets.

## Was im Code schon umgestellt ist

- Jeder Job, der Secrets benutzt, läuft in der Umgebung **`produktion`**.
- Kein Server-Workflow startet mehr per Push. Einzige Ausnahme ist
  `deploy-c1-dann-c2.yml` bei einem Merge nach `main`.
- `scripts/lint_server_freigabe.py` hält das im CI fest. Ein neuer Workflow ohne
  Freigabe-Umgebung macht den Build rot.

Solange die Umgebung in GitHub noch keine Regeln hat, läuft alles wie bisher.
**Die Absicherung greift erst mit den Schritten unten.**

## Was du in GitHub einstellst (etwa 10 Minuten)

### 1. Umgebung anlegen und schützen

Repo → **Settings** → **Environments** → `produktion` (wird beim ersten Lauf automatisch
angelegt; sonst **New environment**, Name genau `produktion`):

- **Required reviewers**: dich selbst eintragen. Jeder Server-Workflow wartet dann auf
  deinen Klick „Approve and deploy“, in der GitHub-App auch vom Handy.
- **Prevent self-review**: nicht anhaken, sonst kannst du deine eigenen Starts nicht
  freigeben, solange du allein bist.
- **Deployment branches and tags** → **Selected branches and tags** → Regel `main`.
  Damit kommt nur Code an die Schlüssel, der auf `main` gemergt ist, also geprüft und
  mit grünem CI. Ein Workflow auf einem Neben-Branch bekommt sie nicht.

### 2. Secrets in die Umgebung verschieben

Unter **Environments → produktion → Environment secrets** mit denselben Namen und Werten
neu anlegen:

| Secret | Wofür |
|---|---|
| `CONTABO_SSH_KEY` | SSH C1 |
| `CONTABO2_SSH_KEY` | SSH C2 |
| `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID` | Wachhund-Meldungen |
| `Railway` | alter Railway-Zugang, siehe unten |

**Danach** unter **Settings → Secrets and variables → Actions → Repository secrets**
dieselben Namen **löschen**. Erst mit dem Löschen kommen alte Branches nicht mehr an die
Schlüssel heran.

Die Werte kannst du in GitHub nicht mehr ansehen. Neu anlegen heißt also: den Schlüssel
aus deiner eigenen Ablage einfügen. Hast du ihn nicht mehr, erzeuge ich auf Wunsch für
beide Boxen ein neues Schlüsselpaar und trage den öffentlichen Teil ein. Das ist ohnehin
ratsam, weil der bisherige Schlüssel lange ohne Freigabe erreichbar war.

### 3. Prüfen

Einen harmlosen Workflow starten, etwa **Actions → „Lage C1/C2“ → Run workflow**. Er muss
auf deine Freigabe warten und danach grün laufen.

## Was sich für dich ändert

- **Jeder Deploy und jede Messung wartet auf deinen Klick.** Das ist der Zweck: Ohne dich
  kommt niemand auf die Server, auch keine Claude-Sitzung.
- Messungen und Diagnosen laufen nur noch mit Workflows von `main`.

## Offen

- `Railway`: Die Railway-Instanz ist seit 14.09. abgeschaltet. Wird der Token nicht mehr
  gebraucht, bitte in Railway widerrufen und das Secret löschen.
