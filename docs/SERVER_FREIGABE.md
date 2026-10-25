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

## Sicherung ohne Freigabe-Halt (seit 24.10.2026)

Mit Pflicht-Freigabe in `produktion` warteten auch die nächtlichen Sicherungen
(`backup-ledger.yml`, `backup-biometrie-verschluesselt.yml`) auf einen Klick.
Vergessen hieß: keine Sicherung, und die Wache wird nach 36 h rot. Deshalb laufen
genau diese zwei Workflows in einer eigenen Umgebung **`sicherung`** ohne Prüfer,
aber auch **ohne Root-Schlüssel**:

- Ihr Schlüssel `SICHERUNG_SSH_KEY` steht auf C1 in `authorized_keys` als
  `restrict,command="/usr/local/sbin/aequitas-sicherung" …`. Er kann nur die
  festen Befehle aus `deploy/sicherung/sicherung-befehl.sh` ausführen: sichern
  (`ledger`, `bio`), die fertige Datei abholen, Prüfsummen lesen. Keine Shell,
  keine Weiterleitung, kein eigener Code; höchstens ein Lauf je Art zugleich und
  einer je 30 Minuten (ein gestohlener Schlüssel löst also höchstens alle
  30 Minuten einen Dump auf dem laufenden Validator aus).
- Die Skripte, die er ausführt, und der age-Empfänger der Biometrie-Sicherung
  liegen fest auf C1. Eingerichtet und aktualisiert werden sie nur über
  `sicherung-einrichten.yml` in `produktion`, also mit deiner Freigabe.
- Wer den Schlüssel stiehlt, bekommt höchstens, was ohnehin als Artefakt im
  öffentlichen Repo liegt (Zustand ohne biometrische Zeilen), und age-Chiffrat.
- `scripts/lint_server_freigabe.py` lässt `sicherung` nur für diese zwei Workflows
  und nur mit `SICHERUNG_SSH_KEY` zu; `scripts/test_sicherung_befehl.sh` zeigt, dass
  der Befehl jede andere Anfrage abweist.

**Einrichten (einmal, etwa 5 Minuten):**

1. Settings → Environments → **New environment** → Name genau `sicherung`. Keine
   Prüfer. **Deployment branches and tags** → „Selected branches and tags“ → `main`.
2. Dort **Add environment secret** → Name `SICHERUNG_SSH_KEY`, Wert: der private
   Schlüssel als **eine base64-Zeile** (`base64 -w0 schluessel`) -- so übersteht er
   das Einfügen am Handy. Die ganze Datei mit `-----BEGIN/END OPENSSH PRIVATE KEY-----`
   geht auch, selbst ohne Zeilenumbrüche (`deploy/sicherung/schluessel-lesen.sh`).
   Der Workflow prüft, dass das Secret zum öffentlichen Teil in
   `deploy/sicherung/sicherung_ed25519.pub` passt, und sagt sonst genau, was fehlt.
3. Actions → „Sicherungsschluessel auf C1 einrichten“ → Run workflow (bei
   `bio_empfaenger` den öffentlichen age-Schlüssel, siehe
   `docs/BIOMETRIE_SICHERUNG.md`) → freigeben.
4. Actions → „Backup the ledger“ → Run workflow. Er muss **ohne** Freigabe starten
   und grün werden.

**Wichtig:** `CONTABO_SSH_KEY` darf **kein Repository-Secret** mehr sein, nur ein
Secret der Umgebung `produktion` (Schritt 2 oben). Repository-Secrets sieht jeder
Job, auch einer in `sicherung`; nur Umgebungs-Secrets sind getrennt.

Ändern sich `deploy/backup-ledger.sh`, `deploy/backup-biometrie.sh` oder
`deploy/sicherung/*`, `sicherung-einrichten.yml` erneut starten. Bis dahin warnen
die Sicherungsläufe („Sicherungsskript auf C1 veraltet“).

## Root-Schlüssel tauschen (seit 25.10.2026)

`CONTABO_SSH_KEY` lag als Repository-Secret im öffentlichen Repo; den Wert
kennt niemand mehr. Deshalb ein neuer Schlüssel, nur als Secret der Umgebung
`produktion` -- in **Aequitas, aequitas-proof-server und
aequitas-biometric-beta** -- und danach der alte weg von C1. Workflow:
`c1-root-schluessel-tauschen.yml`, jeder Schritt ein eigener Lauf mit Freigabe.

1. `schritt=eintragen` -- mit dem alten Schlüssel den neuen
   (`deploy/c1-root/c1_root_ed25519.pub`) zusätzlich auf C1 eintragen.
2. In allen drei Repos unter Settings → Environments → `produktion` das Secret
   `CONTABO_SSH_KEY` mit dem neuen Schlüssel anlegen -- **alle Zeilen** von
   `-----BEGIN` bis `-----END`, die Server-Workflows lesen ihn ohne
   Nachbearbeitung. Ein Umgebungs-Secret hat Vorrang vor dem gleichnamigen
   Repository-Secret.
3. `schritt=pruefen` -- liest das Secret genau wie die Server-Workflows und
   meldet sich mit dem neuen Schlüssel an. Rot, solange es unlesbar oder noch
   der alte ist.
4. `schritt=alten-entfernen`, `andere_repos_umgestellt=ja` -- nimmt nur den
   alten Root-Schlüssel von C1; alle anderen Zeilen (Biometrie, Sicherung,
   der neue) bleiben, vorher wird gesichert.
5. Repository-Secret `CONTABO_SSH_KEY` in allen drei Repos löschen.

## Offen

- `Railway`: Die Railway-Instanz ist seit 14.09. abgeschaltet. Wird der Token nicht mehr
  gebraucht, bitte in Railway widerrufen und das Secret löschen.
