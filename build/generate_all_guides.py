"""
Aequitas Node Operator Guide — Complete PDF Generator
Matches the website inline guide 100%. White background, readable dark text.
"""
import os
from reportlab.lib.pagesizes import A4
from reportlab.lib.styles import ParagraphStyle
from reportlab.lib.units import cm
from reportlab.lib.colors import HexColor, white
from reportlab.platypus import (SimpleDocTemplate, Paragraph, Spacer, Table,
                                 TableStyle, HRFlowable, KeepTogether, PageBreak)
from reportlab.lib.enums import TA_CENTER, TA_LEFT, TA_RIGHT

# Colors — dark text on white background
PURPLE  = HexColor('#5B21B6')
GOLD    = HexColor('#B45309')
TEAL    = HexColor('#0F766E')
GREEN   = HexColor('#047857')
RED     = HexColor('#B91C1C')
NAVY    = HexColor('#1E1B4B')
GRAY    = HexColor('#374151')
MUTED   = HexColor('#6B7280')
LPUR    = HexColor('#EDE9FE')  # light purple bg
LGOLD   = HexColor('#FFFBEB')  # light gold bg
LRED    = HexColor('#FEF2F2')  # light red bg
LTEAL   = HexColor('#F0FDFA')  # light teal bg
LBORDER = HexColor('#DDD6FE')
LINE    = HexColor('#E5E7EB')
ALTROW  = HexColor('#F9FAFB')

def S(name, **kw):
    d = dict(fontName='Helvetica', textColor=NAVY, leading=15, spaceAfter=4,
             fontSize=9.5)
    d.update(kw)
    return ParagraphStyle(name, **d)

STYLES = {
    'title':  S('T',  fontName='Helvetica-Bold', fontSize=20, textColor=PURPLE,
                 leading=26, spaceAfter=2, alignment=TA_CENTER),
    'sub':    S('SU', fontSize=9, textColor=MUTED, alignment=TA_CENTER, spaceAfter=4),
    'tag':    S('TG', fontSize=8, textColor=MUTED, alignment=TA_CENTER, spaceAfter=14),
    'h1':     S('H1', fontName='Helvetica-Bold', fontSize=11, textColor=PURPLE,
                 spaceBefore=16, spaceAfter=6),
    'h2':     S('H2', fontName='Helvetica-Bold', fontSize=10, textColor=GOLD,
                 spaceBefore=10, spaceAfter=4),
    'body':   S('BO', spaceAfter=6, leading=15),
    'sm':     S('SM', fontSize=8.5, textColor=GRAY, leading=13, spaceAfter=4),
    'code':   S('CO', fontName='Courier', fontSize=7.5, textColor=PURPLE,
                 backColor=LPUR, leading=11, leftIndent=8, rightIndent=8,
                 spaceAfter=8, spaceBefore=2),
    'warn':   S('WN', fontSize=8.5, textColor=RED, leading=13, spaceAfter=6,
                 leftIndent=8, fontName='Helvetica-Bold'),
    'info':   S('IN', fontSize=8.5, textColor=TEAL, leading=13, spaceAfter=6,
                 leftIndent=8),
    'bullet': S('BU', leftIndent=14, spaceAfter=3, leading=14),
    'foot':   S('FO', fontSize=7.5, textColor=MUTED, alignment=TA_CENTER, leading=11),
}

def HR():
    return HRFlowable(width='100%', thickness=0.4, color=LINE, spaceAfter=8, spaceBefore=4)

def box(text, color=TEAL, bg=LTEAL):
    d = [[Paragraph(text, S('bx', fontSize=8.5, textColor=color, leading=13))]]
    t = Table(d, colWidths=[16.6*cm])
    t.setStyle(TableStyle([
        ('BACKGROUND', (0,0), (-1,-1), bg),
        ('BOX', (0,0), (-1,-1), 0.5, color),
        ('LEFTPADDING', (0,0), (-1,-1), 10),
        ('RIGHTPADDING', (0,0), (-1,-1), 10),
        ('TOPPADDING', (0,0), (-1,-1), 6),
        ('BOTTOMPADDING', (0,0), (-1,-1), 6),
    ]))
    return t

def step_block(num, title, content_items, color=PURPLE):
    """Build a numbered step with title and content."""
    story = []
    # Number badge + title in one row
    badge = Paragraph(str(num), S('bd', fontName='Helvetica-Bold', fontSize=11,
                                   textColor=white, alignment=TA_CENTER, leading=14))
    head  = Paragraph(f'<b>{title}</b>', S('sh', fontName='Helvetica-Bold',
                                             fontSize=10, textColor=color, leading=13))
    row = Table([[badge, head]], colWidths=[0.8*cm, 15.8*cm])
    row.setStyle(TableStyle([
        ('BACKGROUND', (0,0), (0,0), color),
        ('VALIGN', (0,0), (-1,-1), 'MIDDLE'),
        ('TOPPADDING', (0,0), (0,0), 4),
        ('BOTTOMPADDING', (0,0), (0,0), 4),
        ('TOPPADDING', (0,0), (1,0), 2),
        ('BOTTOMPADDING', (0,0), (1,0), 2),
        ('LEFTPADDING', (0,0), (1,0), 8),
        ('LEFTPADDING', (0,0), (0,0), 0),
        ('RIGHTPADDING', (0,0), (-1,-1), 0),
    ]))
    story.append(row)
    for item in content_items:
        story.append(item)
    story.append(Spacer(1, 6))
    return story

def var_table(rows, cols):
    def th(t): return Paragraph(f'<b>{t}</b>', S('th', fontName='Helvetica-Bold',
                                                   fontSize=8, textColor=white, leading=11))
    def tv(t): return Paragraph(t, S('tv', fontName='Courier', fontSize=7.5,
                                      textColor=PURPLE, leading=10))
    def tr_req(t):
        c = RED if t in ('YES','JA','SI','SÍ','SÌ','OUI','SIM','EVET','YA','ДА','是','نعم','हाँ') else \
            GREEN if 'reward' in t.lower() or 'Bel' in t or 'Bel.' in t else \
            GOLD if 'Rec' in t or 'Emp' in t or 'Multi' in t or 'Opt' in t else MUTED
        return Paragraph(f'<b>{t}</b>', S('tq', fontName='Helvetica-Bold', fontSize=8,
                                           textColor=c, leading=10))
    def td(t): return Paragraph(t, S('td', fontSize=8, textColor=GRAY, leading=12))

    data = [[th(c) for c in cols]]
    for r in rows:
        data.append([tv(r[0]), tr_req(r[1]), td(r[2])])
    cw = [4*cm, 2.2*cm, 10.4*cm]
    t = Table(data, colWidths=cw, repeatRows=1)
    t.setStyle(TableStyle([
        ('BACKGROUND', (0,0), (-1,0), PURPLE),
        ('ROWBACKGROUNDS', (0,1), (-1,-1), [white, ALTROW]),
        ('GRID', (0,0), (-1,-1), 0.3, LINE),
        ('VALIGN', (0,0), (-1,-1), 'TOP'),
        ('TOPPADDING', (0,0), (-1,-1), 5),
        ('BOTTOMPADDING', (0,0), (-1,-1), 5),
        ('LEFTPADDING', (0,0), (-1,-1), 6),
        ('RIGHTPADDING', (0,0), (-1,-1), 6),
    ]))
    return t

def trouble_table(rows, cols):
    def th(t): return Paragraph(f'<b>{t}</b>', S('th2', fontName='Helvetica-Bold',
                                                   fontSize=8, textColor=white, leading=11))
    def td1(t): return Paragraph(t, S('t1', fontSize=8, textColor=RED,
                                       fontName='Helvetica-Bold', leading=12))
    def td2(t): return Paragraph(t, S('t2', fontSize=8, textColor=MUTED, leading=12))
    def td3(t): return Paragraph(t, S('t3', fontSize=8, textColor=GRAY, leading=12))
    data = [[th(c) for c in cols]]
    for r in rows:
        data.append([td1(r[0]), td2(r[1]), td3(r[2])])
    t = Table(data, colWidths=[4.5*cm, 4*cm, 8.1*cm], repeatRows=1)
    t.setStyle(TableStyle([
        ('BACKGROUND', (0,0), (-1,0), PURPLE),
        ('ROWBACKGROUNDS', (0,1), (-1,-1), [white, ALTROW]),
        ('GRID', (0,0), (-1,-1), 0.3, LINE),
        ('VALIGN', (0,0), (-1,-1), 'TOP'),
        ('TOPPADDING', (0,0), (-1,-1), 5),
        ('BOTTOMPADDING', (0,0), (-1,-1), 5),
        ('LEFTPADDING', (0,0), (-1,-1), 6),
        ('RIGHTPADDING', (0,0), (-1,-1), 6),
    ]))
    return t

# ── CONTENT ───────────────────────────────────────────────────────────────────

EN = dict(
title    = 'AEQUITAS NODE OPERATOR GUIDE',
version  = 'v2.0 · September 2026 · aequitas.digital',
tagline  = 'Run a validator on your own server · Docker Compose · about 15 minutes, 10 of them building',

prereq_title = 'Before You Start — What You Need',
prereqs = [
    ('1.', '<b>You are a registered human:</b> Install the Aequitas app, complete the biometric registration, and note your wallet address. The network refuses a validator whose wallet is not a registered human — one human, one validator. Renting servers buys no extra vote.'),
    ('2.', '<b>A server (VPS) with a public IPv4 address:</b> Ubuntu 22.04 or 24.04, at least 4 vCPU, 8 GB RAM, 60 GB SSD (the database is about 18 GB today and grows). The two founder nodes run on 6 vCPU / 12 GB / 100 GB. Ports 8080 (API) and 4001 (P2P) must be reachable from the internet.'),
    ('3.', '<b>Docker with the Compose plugin, and git.</b> On Ubuntu: <font name="Courier">curl -fsSL https://get.docker.com | sh</font> installs both.'),
    ('4.', '<b>About 15 minutes.</b> Ten of them are the first build (the node is compiled from source on your server). Later updates take the same.'),
],

vars_title = 'Step 1 — Configuration (.env)',
vars_warn  = 'Security warning: RELAYER_PRIVATE_KEY and NODE_KEY are secrets. Whoever has them IS your node. Never paste them into chat, e-mail or a ticket. The .env file stays on your server.',
var_cols   = ['Variable', 'Required?', 'What to set'],
vars = [
    ('POSTGRES_PASSWORD',   'YES',         'A long random password for the local database. Used only by the two containers on your server.'),
    ('SELF_URL',            'YES',         'How other nodes reach YOUR node: http://YOUR-PUBLIC-IP:8080 (or https://your-domain if you put a proxy in front). Without it the node only follows the chain as an observer and never registers as a validator.'),
    ('NODE_OPERATOR_WALLET','YES',         'Your own wallet address — MUST be a registered human (app registration completed). This is what ties the validator to a person. Receives the validator rewards.'),
    ('RELAYER_PRIVATE_KEY', 'Recommended', 'The key that signs your blocks (0x…, 66 characters). Simplest: the private key of NODE_OPERATOR_WALLET — then no extra binding is needed. Left empty, the node generates one on first start and prints it ONCE (\"SAVE THIS AS …\"); put it into .env and restart, or your identity changes on every restart.'),
    ('NODE_OPERATOR_BINDING_SIGNATURE', 'Only if the keys differ', 'If RELAYER_PRIVATE_KEY is not the key of NODE_OPERATOR_WALLET: sign the message shown at aequitas.digital/node-binding with your wallet and paste the signature here.'),
    ('NODE_KEY',            'Recommended', 'P2P identity. Generated and printed on first start if empty — save it into .env, same reason as above.'),
    ('PRIMARY_NODE_URLS',   'Preset',      'The nodes yours registers with and fetches the state snapshot from on first start. Preset to the two founder nodes; leave as is.'),
    ('GOMEMLIMIT',          'Preset',      'Memory ceiling of the node process, preset 5GiB (fits 12 GB RAM). On 8 GB RAM set 3GiB.'),
    ('POSTGRES_SHARED_BUFFERS', 'Preset',  'Postgres cache, preset 1GB. A quarter of your RAM is a good value.'),
],

railway_title = 'Step 2 — Start the node (Docker Compose)',
railway_intro = 'Everything the two founder nodes do, as one file. The first command builds the node from source (about 10 minutes), starts Postgres and the node, and restarts both automatically after a crash or a reboot.',
railway_steps = [
    'On your server: <font name="Courier">git clone https://github.com/hanoi96international-gif/Aequitas.git</font>',
    '<font name="Courier">cd Aequitas/deploy/validator</font> and <font name="Courier">cp .env.example .env</font>',
    'Edit <font name="Courier">.env</font> (for example with <font name="Courier">nano .env</font>): fill in POSTGRES_PASSWORD, SELF_URL and NODE_OPERATOR_WALLET — see the table above',
    '<font name="Courier">docker compose up -d --build</font> — builds and starts. Takes about 10 minutes the first time',
    'Watch the log: <font name="Courier">docker compose logs -f node</font>. On first start the node imports the network state (snapshot) from a founder node, checks its signature, then pulls the blocks since then: <font name="Courier" color="#5B21B6">[BOOTSTRAP] Fresh node — importing state from …</font> followed by <font name="Courier" color="#0F766E">[HTTP-SYNC] Added … new blocks</font>',
    'If the log printed <font name="Courier">SAVE THIS AS NODE_KEY</font> or <font name="Courier">SET THIS AS RELAYER_PRIVATE_KEY</font>: copy those values into .env now and run <font name="Courier">docker compose up -d</font> again — otherwise your node gets a new identity on every restart',
    'Once caught up you will see <font name="Courier">[Block #…]</font> lines — those are blocks YOUR node produced. Until then the node deliberately produces nothing (<font name="Courier">Frischer Knoten: … produziert nichts, bis er aufgeholt hat</font>) — a node that has never seen the chain must not invent one',
],
railway_vars_code = (
    '# deploy/validator/.env — the three required values\n'
    'POSTGRES_PASSWORD      = a-long-random-password\n'
    'SELF_URL               = http://YOUR-PUBLIC-IP:8080\n'
    'NODE_OPERATOR_WALLET   = 0xYOUR_HUMAN_WALLET\n'
    '# recommended: the key that signs your blocks (or leave empty on first start)\n'
    'RELAYER_PRIVATE_KEY    = 0xYOUR_PRIVATE_KEY\n'
    '# preset, leave as is\n'
    'PRIMARY_NODE_URLS      = http://173.249.37.118:8080,http://194.163.188.71:8080'
),

docker_title = 'Step 2b — Update, restart, stop',
docker_intro = 'The node keeps its state in two Docker volumes (database and transfer log). Updating rebuilds the image from the latest code; the state stays. Never restart every validator of the network at the same time — one after the other.',
docker_code  = (
    '# Update to the latest code (about 10 minutes)\n'
    'cd Aequitas && git pull && cd deploy/validator && docker compose up -d --build\n\n'
    '# Restart only the node\n'
    'docker compose restart node\n\n'
    '# Stop everything (state is kept)\n'
    'docker compose down\n\n'
    '# Watch resource use\n'
    'docker stats'
),

verify_title = 'Step 3 — Verify Your Node is Running',
verify_body  = 'Compare your node with the network. Replace YOUR-PUBLIC-IP with your server address.',
verify_code  = (
    'curl -s http://YOUR-PUBLIC-IP:8080/api/status | grep -oE \'"height":[0-9]+\'\n'
    'curl -s https://aequitas.digital/api/status  | grep -oE \'"height":[0-9]+\'\n'
    ' → Both numbers must be equal within a few blocks.\n\n'
    'http://YOUR-PUBLIC-IP:8080/           → your own explorer\n'
    'http://YOUR-PUBLIC-IP:8080/api/health/combined → everything the node measures about itself'
),
verify_note  = 'Right after the first start the height is far below the network while the snapshot imports and the recent blocks are pulled. If it stays far behind for more than 15 minutes, check the log for [BOOTSTRAP] errors and that ports 8080 and 4001 are open.',

valkey_title = 'Step 3b — Bind your wallet to the node key (only if they differ)',
valkey_body  = 'If RELAYER_PRIVATE_KEY is not the private key of NODE_OPERATOR_WALLET, prove once that both belong to you: open the page below, sign the shown message with your wallet, and put the signature into .env as NODE_OPERATOR_BINDING_SIGNATURE.',
valkey_code  = 'https://aequitas.digital/node-binding',
valkey_note  = 'With the simple single-key setup (RELAYER_PRIVATE_KEY = your wallet\'s key) this step is not needed.',

mm_title = 'Step 4 — Connect MetaMask to Your Node (Optional)',
mm_body  = 'In MetaMask: network dropdown → Add network → Add a network manually, then enter:',
mm_rows  = [
    ('Network Name',    'Aequitas Chain'),
    ('RPC URL',         'http://YOUR-PUBLIC-IP:8080/rpc'),
    ('Chain ID',        '1926'),
    ('Currency Symbol', 'AEQ'),
    ('Decimals',        '18'),
    ('Block Explorer',  'https://aequitas.digital'),
],

rewards_title = 'Step 5 — Validator Rewards',
rewards_box   = 'The validators pool collects 40% of all protocol fees (swap fees, demurrage, wealth-cap overflow). Every day at 20:00 Berlin time the pool is distributed to the registered validators in proportion to the blocks they produced. Nothing to do beyond keeping the node running.',
rewards_steps = [
    'NODE_OPERATOR_WALLET must be a registered human — otherwise the network rejects the registration (log: <font name="Courier">NODE_OPERATOR_WALLET is not a registered human</font>).',
    'Confirm in the log: <font name="Courier" color="#0F766E">[PEERS] Auto-authorized validator … (wallet: 0x…)</font> on a founder node, and <font name="Courier">[Block #…]</font> lines on yours.',
    'A node that is down produces no blocks and therefore earns nothing for that time — restarts are harmless, the node catches up on its own.',
    'What a validator does NOT do without extra software: accept new human registrations (those endpoints answer 503). Transfers, blocks and rewards work without it.',
],

trouble_title = 'Troubleshooting',
trouble_cols  = ['Symptom', 'Likely Cause', 'Solution'],
trouble_rows  = [
    ('NODE_OPERATOR_WALLET is not a registered human', 'The wallet has not completed the app registration', 'Register in the app first, then restart the node.'),
    ('operator_binding_signature missing or invalid', 'Signing key and wallet differ, no binding', 'Step 3b: sign at /node-binding and set NODE_OPERATOR_BINDING_SIGNATURE.'),
    ('Frischer Knoten: … produziert nichts, bis er aufgeholt hat', 'Normal while catching up', 'Wait. Production starts once the node has completed clean sync cycles with the founder nodes.'),
    ('SELF_URL not set — … Beobachter', 'SELF_URL missing in .env', 'Set SELF_URL to http://YOUR-PUBLIC-IP:8080 and run docker compose up -d.'),
    ('Height stays far below the network', 'Snapshot import failed, or ports closed', 'Check the log for [BOOTSTRAP] lines; open TCP 8080 and 4001 inbound in the firewall / cloud security group.'),
    ('Node restarts in a loop, "OOMKilled"', 'Not enough RAM', 'Lower GOMEMLIMIT in .env (3GiB on 8 GB RAM) or give the server more memory.'),
    ('docker compose: build fails', 'No outbound internet during build', 'The build downloads Go modules; check DNS and outbound HTTPS on the server.'),
],

footer = 'Aequitas Chain · Chain ID 1926 · aequitas.digital · Validator rewards: daily at 20:00 Berlin time (CEST/CET)',
)

DE = dict(
title    = 'AEQUITAS NODE-BETREIBER-LEITFADEN',
version  = 'v2.0 · September 2026 · aequitas.digital',
tagline  = 'Einen Validator auf dem eigenen Server betreiben · Docker Compose · etwa 15 Minuten, davon 10 Bauen',

prereq_title = 'Bevor du anfängst — Was du brauchst',
prereqs = [
    ('1.', '<b>Du bist ein registrierter Mensch:</b> Aequitas-App installieren, die biometrische Registrierung abschließen, Wallet-Adresse notieren. Das Netz lehnt einen Validator ab, dessen Wallet kein registrierter Mensch ist — ein Mensch, ein Validator. Gemietete Server kaufen keine zusätzliche Stimme.'),
    ('2.', '<b>Ein Server (VPS) mit öffentlicher IPv4-Adresse:</b> Ubuntu 22.04 oder 24.04, mindestens 4 vCPU, 8 GB RAM, 60 GB SSD (die Datenbank hat heute etwa 18 GB und wächst). Die beiden Gründerknoten laufen mit 6 vCPU / 12 GB / 100 GB. Die Ports 8080 (API) und 4001 (P2P) müssen aus dem Internet erreichbar sein.'),
    ('3.', '<b>Docker mit Compose-Plugin und git.</b> Unter Ubuntu installiert <font name="Courier">curl -fsSL https://get.docker.com | sh</font> beides.'),
    ('4.', '<b>Etwa 15 Minuten.</b> Zehn davon ist der erste Build (der Knoten wird auf deinem Server aus dem Quelltext gebaut). Spätere Updates dauern genauso lang.'),
],

vars_title = 'Schritt 1 — Konfiguration (.env)',
vars_warn  = 'Sicherheitshinweis: RELAYER_PRIVATE_KEY und NODE_KEY sind Geheimnisse. Wer sie hat, IST dein Knoten. Nie in Chat, E-Mail oder ein Ticket kopieren. Die .env-Datei bleibt auf deinem Server.',
var_cols   = ['Variable', 'Pflicht?', 'Was eintragen'],
vars = [
    ('POSTGRES_PASSWORD',   'JA',          'Ein langes zufälliges Passwort für die lokale Datenbank. Nur die beiden Container auf deinem Server benutzen es.'),
    ('SELF_URL',            'JA',          'Wie andere Knoten DEINEN erreichen: http://DEINE-ÖFFENTLICHE-IP:8080 (oder https://deine-domain, wenn ein Proxy davor steht). Ohne SELF_URL folgt der Knoten der Kette nur als Beobachter und meldet sich nie als Validator an.'),
    ('NODE_OPERATOR_WALLET','JA',          'Deine eigene Wallet-Adresse — MUSS ein registrierter Mensch sein (App-Registrierung abgeschlossen). Das bindet den Validator an eine Person. Empfängt die Validator-Belohnungen.'),
    ('RELAYER_PRIVATE_KEY', 'Empfohlen',   'Der Schlüssel, der deine Blöcke signiert (0x…, 66 Zeichen). Am einfachsten: der private Schlüssel von NODE_OPERATOR_WALLET — dann ist keine Bindung nötig. Leer gelassen erzeugt der Knoten beim ersten Start einen und druckt ihn EINMAL („SAVE THIS AS …“); in .env eintragen und neu starten, sonst wechselt deine Identität bei jedem Neustart.'),
    ('NODE_OPERATOR_BINDING_SIGNATURE', 'Nur bei getrennten Schlüsseln', 'Wenn RELAYER_PRIVATE_KEY nicht der Schlüssel von NODE_OPERATOR_WALLET ist: die auf aequitas.digital/node-binding gezeigte Nachricht mit dem Wallet signieren und die Signatur hier eintragen.'),
    ('NODE_KEY',            'Empfohlen',   'P2P-Identität. Wird beim ersten Start erzeugt und gedruckt, wenn leer — in .env sichern, aus demselben Grund wie oben.'),
    ('PRIMARY_NODE_URLS',   'Voreingestellt', 'Die Knoten, bei denen sich deiner anmeldet und von denen er beim ersten Start den Zustands-Snapshot holt. Voreingestellt auf die beiden Gründerknoten; so lassen.'),
    ('GOMEMLIMIT',          'Voreingestellt', 'Speicherdeckel des Knotens, voreingestellt 5GiB (passt zu 12 GB RAM). Bei 8 GB RAM 3GiB eintragen.'),
    ('POSTGRES_SHARED_BUFFERS', 'Voreingestellt', 'Postgres-Cache, voreingestellt 1GB. Ein Viertel des RAM ist ein guter Wert.'),
],

railway_title = 'Schritt 2 — Knoten starten (Docker Compose)',
railway_intro = 'Alles, was die beiden Gründerknoten tun, als eine Datei. Der erste Befehl baut den Knoten aus dem Quelltext (etwa 10 Minuten), startet Postgres und den Knoten und startet beide nach Absturz oder Reboot von allein neu.',
railway_steps = [
    'Auf deinem Server: <font name="Courier">git clone https://github.com/hanoi96international-gif/Aequitas.git</font>',
    '<font name="Courier">cd Aequitas/deploy/validator</font> und <font name="Courier">cp .env.example .env</font>',
    '<font name="Courier">.env</font> bearbeiten (z. B. mit <font name="Courier">nano .env</font>): POSTGRES_PASSWORD, SELF_URL und NODE_OPERATOR_WALLET ausfüllen — siehe Tabelle oben',
    '<font name="Courier">docker compose up -d --build</font> — baut und startet. Beim ersten Mal etwa 10 Minuten',
    'Log ansehen: <font name="Courier">docker compose logs -f node</font>. Beim ersten Start holt sich der Knoten den Netzzustand (Snapshot) von einem Gründerknoten, prüft dessen Signatur und zieht dann die Blöcke seitdem nach: <font name="Courier" color="#5B21B6">[BOOTSTRAP] Fresh node — importing state from …</font>, danach <font name="Courier" color="#0F766E">[HTTP-SYNC] Added … new blocks</font>',
    'Stand im Log <font name="Courier">SAVE THIS AS NODE_KEY</font> oder <font name="Courier">SET THIS AS RELAYER_PRIVATE_KEY</font>: diese Werte jetzt in .env eintragen und <font name="Courier">docker compose up -d</font> erneut ausführen — sonst bekommt dein Knoten bei jedem Neustart eine neue Identität',
    'Sobald er aufgeholt hat, erscheinen <font name="Courier">[Block #…]</font>-Zeilen — das sind Blöcke, die DEIN Knoten produziert. Bis dahin produziert er absichtlich nichts (<font name="Courier">Frischer Knoten: … produziert nichts, bis er aufgeholt hat</font>) — ein Knoten, der die Kette nie gesehen hat, darf keine erfinden',
],
railway_vars_code = (
    '# deploy/validator/.env — die drei Pflichtwerte\n'
    'POSTGRES_PASSWORD      = ein-langes-zufaelliges-passwort\n'
    'SELF_URL               = http://DEINE-OEFFENTLICHE-IP:8080\n'
    'NODE_OPERATOR_WALLET   = 0xDEIN_MENSCH_WALLET\n'
    '# empfohlen: der Schluessel, der deine Bloecke signiert (oder beim ersten Start leer lassen)\n'
    'RELAYER_PRIVATE_KEY    = 0xDEIN_PRIVATER_SCHLUESSEL\n'
    '# voreingestellt, so lassen\n'
    'PRIMARY_NODE_URLS      = http://173.249.37.118:8080,http://194.163.188.71:8080'
),

docker_title = 'Schritt 2b — Aktualisieren, neu starten, stoppen',
docker_intro = 'Der Knoten hält seinen Zustand in zwei Docker-Volumes (Datenbank und Überweisungs-Log). Ein Update baut das Image aus dem neuesten Code neu; der Zustand bleibt. Starte nie alle Validatoren des Netzes gleichzeitig neu — einen nach dem anderen.',
docker_code  = (
    '# Auf den neuesten Code aktualisieren (etwa 10 Minuten)\n'
    'cd Aequitas && git pull && cd deploy/validator && docker compose up -d --build\n\n'
    '# Nur den Knoten neu starten\n'
    'docker compose restart node\n\n'
    '# Alles stoppen (Zustand bleibt erhalten)\n'
    'docker compose down\n\n'
    '# Ressourcen beobachten\n'
    'docker stats'
),

verify_title = 'Schritt 3 — Prüfen, ob dein Knoten läuft',
verify_body  = 'Vergleiche deinen Knoten mit dem Netz. DEINE-ÖFFENTLICHE-IP durch deine Serveradresse ersetzen.',
verify_code  = (
    'curl -s http://DEINE-OEFFENTLICHE-IP:8080/api/status | grep -oE \'"height":[0-9]+\'\n'
    'curl -s https://aequitas.digital/api/status         | grep -oE \'"height":[0-9]+\'\n'
    ' → Beide Zahlen müssen bis auf ein paar Blöcke gleich sein.\n\n'
    'http://DEINE-OEFFENTLICHE-IP:8080/           → dein eigener Explorer\n'
    'http://DEINE-OEFFENTLICHE-IP:8080/api/health/combined → alles, was der Knoten über sich misst'
),
verify_note  = 'Direkt nach dem ersten Start liegt die Höhe weit unter dem Netz, solange der Snapshot importiert und die jüngsten Blöcke nachgezogen werden. Bleibt sie länger als 15 Minuten weit zurück: Log auf [BOOTSTRAP]-Fehler prüfen und ob die Ports 8080 und 4001 offen sind.',

valkey_title = 'Schritt 3b — Wallet an den Knotenschlüssel binden (nur bei getrennten Schlüsseln)',
valkey_body  = 'Wenn RELAYER_PRIVATE_KEY nicht der private Schlüssel von NODE_OPERATOR_WALLET ist, beweise einmal, dass beide dir gehören: die Seite unten öffnen, die gezeigte Nachricht mit deinem Wallet signieren und die Signatur als NODE_OPERATOR_BINDING_SIGNATURE in .env eintragen.',
valkey_code  = 'https://aequitas.digital/node-binding',
valkey_note  = 'Mit dem einfachen Ein-Schlüssel-Aufbau (RELAYER_PRIVATE_KEY = Schlüssel deines Wallets) entfällt dieser Schritt.',

mm_title = 'Schritt 4 — MetaMask mit deinem Knoten verbinden (optional)',
mm_body  = 'In MetaMask: Netzwerk-Auswahl → Netzwerk hinzufügen → Manuell hinzufügen, dann eintragen:',
mm_rows  = [
    ('Netzwerkname',     'Aequitas Chain'),
    ('RPC-URL',          'http://DEINE-OEFFENTLICHE-IP:8080/rpc'),
    ('Chain-ID',         '1926'),
    ('Währungssymbol',   'AEQ'),
    ('Dezimalstellen',   '18'),
    ('Block-Explorer',   'https://aequitas.digital'),
],

rewards_title = 'Schritt 5 — Validator-Belohnungen',
rewards_box   = 'Der Validatoren-Pool sammelt 40 % aller Protokollgebühren (Swap-Gebühren, Demurrage, Vermögensdeckel-Überschuss). Täglich um 20:00 Uhr Berliner Zeit wird der Pool an die registrierten Validatoren im Verhältnis ihrer produzierten Blöcke verteilt. Außer den Knoten laufen zu lassen ist nichts zu tun.',
rewards_steps = [
    'NODE_OPERATOR_WALLET muss ein registrierter Mensch sein — sonst lehnt das Netz die Anmeldung ab (Log: <font name="Courier">NODE_OPERATOR_WALLET is not a registered human</font>).',
    'Im Log bestätigen: <font name="Courier" color="#0F766E">[PEERS] Auto-authorized validator … (wallet: 0x…)</font> auf einem Gründerknoten und <font name="Courier">[Block #…]</font>-Zeilen auf deinem.',
    'Ein Knoten, der nicht läuft, produziert keine Blöcke und verdient in dieser Zeit nichts — Neustarts sind harmlos, der Knoten holt von allein auf.',
    'Was ein Validator ohne zusätzliche Software NICHT tut: neue Menschen registrieren (diese Endpunkte antworten 503). Überweisungen, Blöcke und Belohnungen funktionieren ohne das.',
],

trouble_title = 'Fehlerbehebung',
trouble_cols  = ['Symptom', 'Wahrscheinliche Ursache', 'Lösung'],
trouble_rows  = [
    ('NODE_OPERATOR_WALLET is not a registered human', 'Das Wallet hat die App-Registrierung nicht abgeschlossen', 'Erst in der App registrieren, dann den Knoten neu starten.'),
    ('operator_binding_signature missing or invalid', 'Signierschlüssel und Wallet verschieden, keine Bindung', 'Schritt 3b: auf /node-binding signieren und NODE_OPERATOR_BINDING_SIGNATURE setzen.'),
    ('Frischer Knoten: … produziert nichts, bis er aufgeholt hat', 'Normal beim Aufholen', 'Warten. Die Produktion beginnt, sobald der Knoten saubere Sync-Zyklen mit den Gründerknoten hat.'),
    ('SELF_URL not set — … Beobachter', 'SELF_URL fehlt in .env', 'SELF_URL auf http://DEINE-OEFFENTLICHE-IP:8080 setzen und docker compose up -d ausführen.'),
    ('Höhe bleibt weit unter dem Netz', 'Snapshot-Import gescheitert oder Ports zu', 'Log auf [BOOTSTRAP]-Zeilen prüfen; TCP 8080 und 4001 eingehend in Firewall / Cloud-Sicherheitsgruppe öffnen.'),
    ('Knoten startet in Schleife neu, „OOMKilled“', 'Zu wenig RAM', 'GOMEMLIMIT in .env senken (3GiB bei 8 GB RAM) oder dem Server mehr Speicher geben.'),
    ('docker compose: Build schlägt fehl', 'Kein ausgehendes Internet beim Bauen', 'Der Build lädt Go-Module; DNS und ausgehendes HTTPS auf dem Server prüfen.'),
],

footer = 'Aequitas Chain · Chain-ID 1926 · aequitas.digital · Validator-Belohnungen: täglich um 20:00 Uhr Berliner Zeit (MESZ/MEZ)',
)

# ── PDF BUILDER ───────────────────────────────────────────────────────────────

def build_pdf(path, L):
    doc = SimpleDocTemplate(path, pagesize=A4,
                            leftMargin=2*cm, rightMargin=2*cm,
                            topMargin=2*cm, bottomMargin=2*cm,
                            title=L['title'])
    fl = []

    # HEADER
    fl += [Paragraph(L['title'],   STYLES['title']),
           Paragraph(L['version'], STYLES['sub']),
           Paragraph(L['tagline'], STYLES['tag']),
           HR()]

    # BEFORE YOU START
    fl += [Paragraph(L['prereq_title'], STYLES['h1'])]
    for num, text in L['prereqs']:
        row = Table([[Paragraph(f'<b><font color="#5B21B6">{num}</font></b>',
                                 S('pn', fontName='Helvetica-Bold', fontSize=10,
                                   textColor=PURPLE, alignment=TA_CENTER, leading=14)),
                      Paragraph(text, STYLES['body'])]],
                    colWidths=[0.7*cm, 15.9*cm])
        row.setStyle(TableStyle([('VALIGN',(0,0),(-1,-1),'TOP'),
                                  ('TOPPADDING',(0,0),(-1,-1),2),
                                  ('BOTTOMPADDING',(0,0),(-1,-1),2),
                                  ('LEFTPADDING',(0,0),(1,0),8),
                                  ('LEFTPADDING',(0,0),(0,0),0)]))
        fl.append(row)
    fl.append(HR())

    # STEP 3 VARS
    fl += [Paragraph(L['vars_title'], STYLES['h1']),
           box(L['vars_warn'], RED, LRED),
           Spacer(1,8),
           var_table(L['vars'], L['var_cols']),
           HR()]

    # STEP 4 RAILWAY
    fl += [Paragraph(L['railway_title'], STYLES['h1']),
           Paragraph(L['railway_intro'], STYLES['body'])]
    for i, step in enumerate(L['railway_steps'], 1):
        color = GOLD if i in (4, 7) else PURPLE
        row = Table([[Paragraph(str(i), S('sn', fontName='Helvetica-Bold', fontSize=10,
                                           textColor=white, alignment=TA_CENTER, leading=13)),
                      Paragraph(step, STYLES['body'])]],
                    colWidths=[0.7*cm, 15.9*cm])
        row.setStyle(TableStyle([('BACKGROUND',(0,0),(0,0),color),
                                  ('VALIGN',(0,0),(-1,-1),'TOP'),
                                  ('TOPPADDING',(0,0),(0,0),3),
                                  ('BOTTOMPADDING',(0,0),(0,0),3),
                                  ('TOPPADDING',(0,0),(1,0),1),
                                  ('BOTTOMPADDING',(0,0),(1,0),4),
                                  ('LEFTPADDING',(0,0),(0,0),0),
                                  ('LEFTPADDING',(0,0),(1,0),8)]))
        fl.append(row)
    fl += [Spacer(1,6),
           Paragraph(L['railway_vars_code'].replace('\n','<br/>').replace(' ','&nbsp;'),
                     STYLES['code']),
           HR()]

    # STEP 4B DOCKER
    fl += [Paragraph(L['docker_title'], STYLES['h1']),
           Paragraph(L['docker_intro'], STYLES['body']),
           Paragraph(L['docker_code'].replace('\n','<br/>').replace(' ','&nbsp;'),
                     STYLES['code']),
           HR()]

    # STEP 5 VERIFY
    fl += [Paragraph(L['verify_title'], STYLES['h1']),
           Paragraph(L['verify_body'], STYLES['body']),
           Paragraph(L['verify_code'].replace('\n','<br/>').replace(' ','&nbsp;'),
                     STYLES['code']),
           box(L['verify_note'], TEAL, LTEAL),
           HR()]

    # STEP 5B VALIDATOR KEY
    fl += [Paragraph(L['valkey_title'], STYLES['h1']),
           Paragraph(L['valkey_body'], STYLES['body']),
           Paragraph(L['valkey_code'], STYLES['code']),
           Paragraph(L['valkey_note'], STYLES['info']),
           HR()]

    # STEP 6 METAMASK
    fl += [Paragraph(L['mm_title'], STYLES['h1']),
           Paragraph(L['mm_body'], STYLES['body'])]
    mm_data = []
    for k, v in L['mm_rows']:
        mm_data.append([Paragraph(k, S('mk', fontSize=8.5, textColor=GRAY,
                                        fontName='Helvetica-Bold', leading=12)),
                         Paragraph(v, S('mv', fontName='Courier', fontSize=8.5,
                                         textColor=PURPLE, leading=12))])
    mm = Table(mm_data, colWidths=[5*cm, 11.6*cm])
    mm.setStyle(TableStyle([('ROWBACKGROUNDS',(0,0),(-1,-1),[white,ALTROW]),
                             ('GRID',(0,0),(-1,-1),0.3,LINE),
                             ('TOPPADDING',(0,0),(-1,-1),5),
                             ('BOTTOMPADDING',(0,0),(-1,-1),5),
                             ('LEFTPADDING',(0,0),(-1,-1),8)]))
    fl += [mm, HR()]

    # STEP 7 REWARDS
    fl += [Paragraph(L['rewards_title'], STYLES['h1']),
           box(L['rewards_box'], GOLD, LGOLD)]
    for i, step in enumerate(L['rewards_steps'], 1):
        row = Table([[Paragraph(str(i), S('rn', fontName='Helvetica-Bold', fontSize=9,
                                           textColor=white, alignment=TA_CENTER, leading=12)),
                      Paragraph(step, STYLES['body'])]],
                    colWidths=[0.65*cm, 15.95*cm])
        row.setStyle(TableStyle([('BACKGROUND',(0,0),(0,0),GOLD),
                                  ('VALIGN',(0,0),(-1,-1),'TOP'),
                                  ('TOPPADDING',(0,0),(-1,-1),3),
                                  ('BOTTOMPADDING',(0,0),(-1,-1),3),
                                  ('LEFTPADDING',(0,0),(0,0),0),
                                  ('LEFTPADDING',(0,0),(1,0),8)]))
        fl.append(row)
    fl.append(HR())

    # TROUBLESHOOTING
    fl += [Paragraph(L['trouble_title'], STYLES['h1']),
           trouble_table(L['trouble_rows'], L['trouble_cols']),
           Spacer(1,12), HR()]

    # FOOTER
    fl.append(Paragraph(L['footer'], STYLES['foot']))

    doc.build(fl)

ES = {'title': 'GUÍA DEL OPERADOR DE NODO AEQUITAS',
 'version': 'v2.0 · 2026-09 · aequitas.digital',
 'tagline': 'Ejecuta un validador en tu propio servidor · Docker Compose · unos 15 minutos, 10 de ellos '
            'compilando',
 'prereq_title': 'Antes de empezar — Qué necesitas',
 'prereqs': [('1.',
              '<b>Eres un humano registrado:</b> instala la app de Aequitas, completa el registro biométrico '
              'y anota tu dirección de wallet. La red rechaza un validador cuya wallet no sea un humano '
              'registrado — un humano, un validador. Alquilar servidores no compra votos.'),
             ('2.',
              '<b>Un servidor (VPS) con IPv4 pública:</b> Ubuntu 22.04 o 24.04, al menos 4 vCPU, 8 GB de '
              'RAM, 60 GB SSD (la base de datos ocupa hoy unos 18 GB y crece). Los dos nodos fundadores usan '
              '6 vCPU / 12 GB / 100 GB. Los puertos 8080 (API) y 4001 (P2P) deben ser accesibles desde '
              'internet.'),
             ('3.',
              '<b>Docker con el plugin Compose y git.</b> En Ubuntu, <font name="Courier">curl -fsSL '
              'https://get.docker.com | sh</font> instala ambos.'),
             ('4.',
              '<b>Unos 15 minutos.</b> Diez son la primera compilación (el nodo se compila desde el código '
              'fuente en tu servidor). Las actualizaciones tardan lo mismo.')],
 'vars_title': 'Paso 1 — Configuración (.env)',
 'vars_warn': 'Aviso de seguridad: RELAYER_PRIVATE_KEY y NODE_KEY son secretos. Quien los tenga ES tu nodo. '
              'Nunca los pegues en un chat, correo o ticket. El archivo .env se queda en tu servidor.',
 'var_cols': ['Variable', '¿Obligatoria?', 'Qué poner'],
 'vars': [('POSTGRES_PASSWORD',
           'SÍ',
           'Una contraseña larga y aleatoria para la base de datos local. Solo la usan los dos contenedores '
           'de tu servidor.'),
          ('SELF_URL',
           'SÍ',
           'Cómo te alcanzan los demás nodos: http://TU-IP-PÚBLICA:8080 (o https://tu-dominio si pones un '
           'proxy delante). Sin ella el nodo solo sigue la cadena como observador y nunca se registra como '
           'validador.'),
          ('NODE_OPERATOR_WALLET',
           'SÍ',
           'Tu propia dirección de wallet — DEBE ser un humano registrado (registro en la app completado). '
           'Esto vincula el validador a una persona. Recibe las recompensas de validador.'),
          ('RELAYER_PRIVATE_KEY',
           'Recomendada',
           'La clave que firma tus bloques (0x…, 66 caracteres). Lo más sencillo: la clave privada de '
           'NODE_OPERATOR_WALLET — así no hace falta ninguna vinculación. Si la dejas vacía, el nodo genera '
           'una en el primer arranque y la imprime UNA vez ("SAVE THIS AS …"); ponla en .env y reinicia, o '
           'tu identidad cambiará en cada reinicio.'),
          ('NODE_OPERATOR_BINDING_SIGNATURE',
           'Solo si las claves difieren',
           'Si RELAYER_PRIVATE_KEY no es la clave de NODE_OPERATOR_WALLET: firma el mensaje que muestra '
           'aequitas.digital/node-binding con tu wallet y pega aquí la firma.'),
          ('NODE_KEY',
           'Recomendada',
           'Identidad P2P. Se genera e imprime en el primer arranque si está vacía — guárdala en .env, por '
           'la misma razón.'),
          ('PRIMARY_NODE_URLS',
           'Preconfigurada',
           'Los nodos en los que el tuyo se registra y de los que obtiene la instantánea de estado en el '
           'primer arranque. Preconfigurado a los dos nodos fundadores; déjalo así.'),
          ('GOMEMLIMIT',
           'Preconfigurada',
           'Límite de memoria del proceso del nodo, preconfigurado 5GiB (para 12 GB de RAM). Con 8 GB pon '
           '3GiB.'),
          ('POSTGRES_SHARED_BUFFERS',
           'Preconfigurada',
           'Caché de Postgres, preconfigurado 1GB. Un cuarto de tu RAM es un buen valor.')],
 'railway_title': 'Paso 2 — Arrancar el nodo (Docker Compose)',
 'railway_intro': 'Todo lo que hacen los dos nodos fundadores, en un archivo. El primer comando compila el '
                  'nodo desde el código (unos 10 minutos), arranca Postgres y el nodo, y reinicia ambos '
                  'automáticamente tras un fallo o un reinicio del servidor.',
 'railway_steps': ['En tu servidor: <font name="Courier">git clone '
                   'https://github.com/hanoi96international-gif/Aequitas.git</font>',
                   '<font name="Courier">cd Aequitas/deploy/validator</font> y <font name="Courier">cp '
                   '.env.example .env</font>',
                   'Edita <font name="Courier">.env</font> (por ejemplo con <font name="Courier">nano '
                   '.env</font>): rellena POSTGRES_PASSWORD, SELF_URL y NODE_OPERATOR_WALLET — ver la tabla '
                   'de arriba',
                   '<font name="Courier">docker compose up -d --build</font> — compila y arranca. La primera '
                   'vez tarda unos 10 minutos',
                   'Mira el registro: <font name="Courier">docker compose logs -f node</font>. En el primer '
                   'arranque el nodo importa el estado de la red (instantánea) desde un nodo fundador, '
                   'comprueba su firma y descarga los bloques posteriores: <font name="Courier" '
                   'color="#5B21B6">[BOOTSTRAP] Fresh node — importing state from …</font> seguido de <font '
                   'name="Courier" color="#0F766E">[HTTP-SYNC] Added … new blocks</font>',
                   'Si el registro imprimió <font name="Courier">SAVE THIS AS NODE_KEY</font> o <font '
                   'name="Courier">SET THIS AS RELAYER_PRIVATE_KEY</font>: copia esos valores en .env ahora '
                   'y vuelve a ejecutar <font name="Courier">docker compose up -d</font> — si no, tu nodo '
                   'tendrá una identidad nueva en cada reinicio',
                   'Cuando esté al día verás líneas <font name="Courier">[Block #…]</font> — son bloques '
                   'producidos por TU nodo. Hasta entonces el nodo no produce nada a propósito (<font '
                   'name="Courier">Frischer Knoten: … produziert nichts, bis er aufgeholt hat</font>) — un '
                   'nodo que nunca ha visto la cadena no debe inventarse una'],
 'railway_vars_code': 'POSTGRES_PASSWORD      = una-contraseña-larga-y-aleatoria\n'
                      'SELF_URL               = http://TU-IP-PUBLICA:8080\n'
                      'NODE_OPERATOR_WALLET   = 0xTU_WALLET_HUMANA\n'
                      '# recomendado: la clave que firma tus bloques (o vacío en el primer arranque)\n'
                      'RELAYER_PRIVATE_KEY    = 0xTU_CLAVE_PRIVADA\n'
                      '# preconfigurado, déjalo así\n'
                      'PRIMARY_NODE_URLS      = http://173.249.37.118:8080,http://194.163.188.71:8080',
 'docker_title': 'Paso 2b — Actualizar, reiniciar, parar',
 'docker_intro': 'El nodo guarda su estado en dos volúmenes Docker (base de datos y registro de '
                 'transferencias). Actualizar reconstruye la imagen con el código más reciente; el estado se '
                 'conserva. Nunca reinicies todos los validadores de la red a la vez — uno tras otro.',
 'docker_code': '# Actualizar al código más reciente (unos 10 minutos)\n'
                'cd Aequitas && git pull && cd deploy/validator && docker compose up -d --build\n'
                '\n'
                '# Reiniciar solo el nodo\n'
                'docker compose restart node\n'
                '\n'
                '# Parar todo (el estado se conserva)\n'
                'docker compose down\n'
                '\n'
                '# Ver el uso de recursos\n'
                'docker stats',
 'verify_title': 'Paso 3 — Comprobar que tu nodo funciona',
 'verify_body': 'Compara tu nodo con la red. Sustituye TU-IP-PUBLICA por la dirección de tu servidor.',
 'verify_code': 'curl -s http://TU-IP-PUBLICA:8080/api/status | grep -oE \'"height":[0-9]+\'\n'
                'curl -s https://aequitas.digital/api/status  | grep -oE \'"height":[0-9]+\'\n'
                ' → Ambos números deben coincidir salvo unos pocos bloques.\n'
                '\n'
                'http://TU-IP-PUBLICA:8080/           → tu propio explorador\n'
                'http://TU-IP-PUBLICA:8080/api/health/combined → todo lo que el nodo mide sobre sí mismo',
 'verify_note': 'Justo después del primer arranque la altura está muy por debajo de la red mientras se '
                'importa la instantánea y se descargan los bloques recientes. Si sigue muy atrás más de 15 '
                'minutos, revisa el registro en busca de errores [BOOTSTRAP] y que los puertos 8080 y 4001 '
                'estén abiertos.',
 'valkey_title': 'Paso 3b — Vincular tu wallet a la clave del nodo (solo si difieren)',
 'valkey_body': 'Si RELAYER_PRIVATE_KEY no es la clave privada de NODE_OPERATOR_WALLET, demuestra una vez '
                'que ambas son tuyas: abre la página de abajo, firma el mensaje mostrado con tu wallet y pon '
                'la firma en .env como NODE_OPERATOR_BINDING_SIGNATURE.',
 'valkey_code': 'https://aequitas.digital/node-binding',
 'valkey_note': 'Con la configuración sencilla de una sola clave (RELAYER_PRIVATE_KEY = clave de tu wallet) '
                'este paso no es necesario.',
 'mm_title': 'Paso 4 — Conectar MetaMask a tu nodo (opcional)',
 'mm_body': 'En MetaMask: menú de redes → Añadir red → Añadir una red manualmente, e introduce:',
 'mm_rows': [('Nombre de la red', 'Aequitas Chain'),
             ('URL RPC', 'http://TU-IP-PUBLICA:8080/rpc'),
             ('ID de cadena', '1926'),
             ('Símbolo', 'AEQ'),
             ('Decimales', '18'),
             ('Explorador de bloques', 'https://aequitas.digital')],
 'rewards_title': 'Paso 5 — Recompensas de validador',
 'rewards_box': 'El fondo de validadores recoge el 40 % de todas las comisiones del protocolo (comisiones de '
                'swap, demurrage, exceso del tope de riqueza). Cada día a las 20:00 hora de Berlín el fondo '
                'se reparte entre los validadores registrados en proporción a los bloques que produjeron. No '
                'hay que hacer nada más que mantener el nodo en marcha.',
 'rewards_steps': ['NODE_OPERATOR_WALLET debe ser un humano registrado — si no, la red rechaza el registro '
                   '(registro: <font name="Courier">NODE_OPERATOR_WALLET is not a registered human</font>).',
                   'Confirma en el registro: <font name="Courier" color="#0F766E">[PEERS] Auto-authorized '
                   'validator … (wallet: 0x…)</font> en un nodo fundador, y líneas <font '
                   'name="Courier">[Block #…]</font> en el tuyo.',
                   'Un nodo apagado no produce bloques y por tanto no gana nada durante ese tiempo — los '
                   'reinicios son inofensivos, el nodo se pone al día solo.',
                   'Lo que un validador NO hace sin software adicional: aceptar nuevos registros de humanos '
                   '(esos endpoints responden 503). Las transferencias, los bloques y las recompensas '
                   'funcionan sin él.'],
 'trouble_title': 'Solución de problemas',
 'trouble_cols': ['Síntoma', 'Causa probable', 'Solución'],
 'trouble_rows': [('NODE_OPERATOR_WALLET is not a registered human',
                   'La wallet no ha completado el registro en la app',
                   'Regístrate primero en la app y reinicia el nodo.'),
                  ('operator_binding_signature missing or invalid',
                   'La clave de firma y la wallet difieren, sin vinculación',
                   'Paso 3b: firma en /node-binding y pon NODE_OPERATOR_BINDING_SIGNATURE.'),
                  ('Frischer Knoten: … produziert nichts, bis er aufgeholt hat',
                   'Normal mientras se pone al día',
                   'Espera. La producción empieza cuando el nodo completa ciclos de sincronización limpios '
                   'con los nodos fundadores.'),
                  ('SELF_URL not set — … Beobachter',
                   'Falta SELF_URL en .env',
                   'Pon SELF_URL en http://TU-IP-PUBLICA:8080 y ejecuta docker compose up -d.'),
                  ('La altura se queda muy por debajo de la red',
                   'Falló la importación de la instantánea o los puertos están cerrados',
                   'Revisa el registro en busca de líneas [BOOTSTRAP]; abre TCP 8080 y 4001 entrantes en el '
                   'firewall / grupo de seguridad.'),
                  ('El nodo se reinicia en bucle, "OOMKilled"',
                   'RAM insuficiente',
                   'Baja GOMEMLIMIT en .env (3GiB con 8 GB de RAM) o dale más memoria al servidor.'),
                  ('docker compose: falla la compilación',
                   'Sin internet saliente durante la compilación',
                   'La compilación descarga módulos de Go; comprueba DNS y HTTPS saliente en el servidor.')],
 'footer': 'Aequitas Chain · Chain ID 1926 · aequitas.digital · Recompensas de validador: diarias a las '
           '20:00 hora de Berlín'}

FR = {'title': "GUIDE DE L'OPÉRATEUR DE NŒUD AEQUITAS",
 'version': 'v2.0 · 2026-09 · aequitas.digital',
 'tagline': 'Faites tourner un validateur sur votre propre serveur · Docker Compose · environ 15 minutes, '
            'dont 10 de compilation',
 'prereq_title': "Avant de commencer — Ce qu'il vous faut",
 'prereqs': [('1.',
              "<b>Vous êtes un humain enregistré :</b> installez l'application Aequitas, terminez "
              "l'enregistrement biométrique et notez votre adresse de wallet. Le réseau refuse un validateur "
              "dont le wallet n'est pas un humain enregistré — un humain, un validateur. Louer des serveurs "
              "n'achète aucune voix."),
             ('2.',
              '<b>Un serveur (VPS) avec une IPv4 publique :</b> Ubuntu 22.04 ou 24.04, au moins 4 vCPU, 8 Go '
              "de RAM, 60 Go SSD (la base de données fait aujourd'hui environ 18 Go et grandit). Les deux "
              'nœuds fondateurs tournent sur 6 vCPU / 12 Go / 100 Go. Les ports 8080 (API) et 4001 (P2P) '
              'doivent être joignables depuis internet.'),
             ('3.',
              '<b>Docker avec le plugin Compose, et git.</b> Sous Ubuntu, <font name="Courier">curl -fsSL '
              'https://get.docker.com | sh</font> installe les deux.'),
             ('4.',
              "<b>Environ 15 minutes.</b> Dix d'entre elles sont la première compilation (le nœud est "
              'compilé depuis les sources sur votre serveur). Les mises à jour prennent autant.')],
 'vars_title': 'Étape 1 — Configuration (.env)',
 'vars_warn': 'Avertissement de sécurité : RELAYER_PRIVATE_KEY et NODE_KEY sont des secrets. Qui les détient '
              'EST votre nœud. Ne les collez jamais dans un chat, un e-mail ou un ticket. Le fichier .env '
              'reste sur votre serveur.',
 'var_cols': ['Variable', 'Obligatoire ?', 'Que renseigner'],
 'vars': [('POSTGRES_PASSWORD',
           'OUI',
           'Un long mot de passe aléatoire pour la base de données locale. Seuls les deux conteneurs de '
           "votre serveur l'utilisent."),
          ('SELF_URL',
           'OUI',
           'Comment les autres nœuds joignent le VÔTRE : http://VOTRE-IP-PUBLIQUE:8080 (ou '
           'https://votre-domaine si un proxy est devant). Sans elle, le nœud suit la chaîne en simple '
           "observateur et ne s'enregistre jamais comme validateur."),
          ('NODE_OPERATOR_WALLET',
           'OUI',
           "Votre propre adresse de wallet — DOIT être un humain enregistré (enregistrement dans l'app "
           "terminé). C'est ce qui lie le validateur à une personne. Reçoit les récompenses de validateur."),
          ('RELAYER_PRIVATE_KEY',
           'Recommandé',
           'La clé qui signe vos blocs (0x…, 66 caractères). Le plus simple : la clé privée de '
           "NODE_OPERATOR_WALLET — aucune liaison supplémentaire n'est alors nécessaire. Laissée vide, le "
           "nœud en génère une au premier démarrage et l'affiche UNE fois (« SAVE THIS AS … ») ; mettez-la "
           'dans .env et redémarrez, sinon votre identité change à chaque redémarrage.'),
          ('NODE_OPERATOR_BINDING_SIGNATURE',
           'Seulement si les clés diffèrent',
           "Si RELAYER_PRIVATE_KEY n'est pas la clé de NODE_OPERATOR_WALLET : signez le message affiché sur "
           'aequitas.digital/node-binding avec votre wallet et collez la signature ici.'),
          ('NODE_KEY',
           'Recommandé',
           'Identité P2P. Générée et affichée au premier démarrage si vide — enregistrez-la dans .env, pour '
           'la même raison.'),
          ('PRIMARY_NODE_URLS',
           'Prérempli',
           "Les nœuds auprès desquels le vôtre s'enregistre et dont il récupère l'instantané d'état au "
           'premier démarrage. Prérempli avec les deux nœuds fondateurs ; laissez tel quel.'),
          ('GOMEMLIMIT',
           'Prérempli',
           'Plafond mémoire du processus du nœud, prérempli 5GiB (pour 12 Go de RAM). Avec 8 Go, mettez '
           '3GiB.'),
          ('POSTGRES_SHARED_BUFFERS',
           'Prérempli',
           'Cache Postgres, prérempli 1GB. Un quart de votre RAM est une bonne valeur.')],
 'railway_title': 'Étape 2 — Démarrer le nœud (Docker Compose)',
 'railway_intro': 'Tout ce que font les deux nœuds fondateurs, en un fichier. La première commande compile '
                  'le nœud depuis les sources (environ 10 minutes), démarre Postgres et le nœud, et relance '
                  'les deux automatiquement après un plantage ou un redémarrage du serveur.',
 'railway_steps': ['Sur votre serveur : <font name="Courier">git clone '
                   'https://github.com/hanoi96international-gif/Aequitas.git</font>',
                   '<font name="Courier">cd Aequitas/deploy/validator</font> puis <font name="Courier">cp '
                   '.env.example .env</font>',
                   'Modifiez <font name="Courier">.env</font> (par exemple avec <font name="Courier">nano '
                   '.env</font>) : renseignez POSTGRES_PASSWORD, SELF_URL et NODE_OPERATOR_WALLET — voir le '
                   'tableau ci-dessus',
                   '<font name="Courier">docker compose up -d --build</font> — compile et démarre. Environ '
                   '10 minutes la première fois',
                   'Suivez le journal : <font name="Courier">docker compose logs -f node</font>. Au premier '
                   "démarrage, le nœud importe l'état du réseau (instantané) depuis un nœud fondateur, "
                   'vérifie sa signature, puis récupère les blocs suivants : <font name="Courier" '
                   'color="#5B21B6">[BOOTSTRAP] Fresh node — importing state from …</font> puis <font '
                   'name="Courier" color="#0F766E">[HTTP-SYNC] Added … new blocks</font>',
                   'Si le journal a affiché <font name="Courier">SAVE THIS AS NODE_KEY</font> ou <font '
                   'name="Courier">SET THIS AS RELAYER_PRIVATE_KEY</font> : copiez ces valeurs dans .env '
                   'maintenant et relancez <font name="Courier">docker compose up -d</font> — sinon votre '
                   'nœud obtient une nouvelle identité à chaque redémarrage',
                   'Une fois à jour, vous verrez des lignes <font name="Courier">[Block #…]</font> — ce sont '
                   'des blocs produits par VOTRE nœud. Jusque-là, le nœud ne produit délibérément rien '
                   '(<font name="Courier">Frischer Knoten: … produziert nichts, bis er aufgeholt hat</font>) '
                   "— un nœud qui n'a jamais vu la chaîne ne doit pas en inventer une"],
 'railway_vars_code': 'POSTGRES_PASSWORD      = un-long-mot-de-passe-aleatoire\n'
                      'SELF_URL               = http://VOTRE-IP-PUBLIQUE:8080\n'
                      'NODE_OPERATOR_WALLET   = 0xVOTRE_WALLET_HUMAIN\n'
                      '# recommandé : la clé qui signe vos blocs (ou vide au premier démarrage)\n'
                      'RELAYER_PRIVATE_KEY    = 0xVOTRE_CLE_PRIVEE\n'
                      '# prérempli, laisser tel quel\n'
                      'PRIMARY_NODE_URLS      = http://173.249.37.118:8080,http://194.163.188.71:8080',
 'docker_title': 'Étape 2b — Mettre à jour, redémarrer, arrêter',
 'docker_intro': 'Le nœud garde son état dans deux volumes Docker (base de données et journal des '
                 "transferts). Une mise à jour reconstruit l'image avec le code le plus récent ; l'état "
                 "reste. Ne redémarrez jamais tous les validateurs du réseau en même temps — l'un après "
                 "l'autre.",
 'docker_code': '# Mettre à jour vers le code le plus récent (environ 10 minutes)\n'
                'cd Aequitas && git pull && cd deploy/validator && docker compose up -d --build\n'
                '\n'
                '# Redémarrer seulement le nœud\n'
                'docker compose restart node\n'
                '\n'
                "# Tout arrêter (l'état est conservé)\n"
                'docker compose down\n'
                '\n'
                '# Surveiller les ressources\n'
                'docker stats',
 'verify_title': 'Étape 3 — Vérifier que votre nœud fonctionne',
 'verify_body': "Comparez votre nœud au réseau. Remplacez VOTRE-IP-PUBLIQUE par l'adresse de votre serveur.",
 'verify_code': 'curl -s http://VOTRE-IP-PUBLIQUE:8080/api/status | grep -oE \'"height":[0-9]+\'\n'
                'curl -s https://aequitas.digital/api/status  | grep -oE \'"height":[0-9]+\'\n'
                ' → Les deux nombres doivent être égaux à quelques blocs près.\n'
                '\n'
                'http://VOTRE-IP-PUBLIQUE:8080/           → votre propre explorateur\n'
                'http://VOTRE-IP-PUBLIQUE:8080/api/health/combined → tout ce que le nœud mesure sur lui-même',
 'verify_note': "Juste après le premier démarrage, la hauteur est bien en dessous du réseau pendant l'import "
                "de l'instantané et la récupération des blocs récents. Si elle reste loin derrière plus de "
                '15 minutes, cherchez des erreurs [BOOTSTRAP] dans le journal et vérifiez que les ports 8080 '
                'et 4001 sont ouverts.',
 'valkey_title': 'Étape 3b — Lier votre wallet à la clé du nœud (seulement si elles diffèrent)',
 'valkey_body': "Si RELAYER_PRIVATE_KEY n'est pas la clé privée de NODE_OPERATOR_WALLET, prouvez une fois "
                'que les deux vous appartiennent : ouvrez la page ci-dessous, signez le message affiché avec '
                'votre wallet et mettez la signature dans .env comme NODE_OPERATOR_BINDING_SIGNATURE.',
 'valkey_code': 'https://aequitas.digital/node-binding',
 'valkey_note': 'Avec la configuration simple à une seule clé (RELAYER_PRIVATE_KEY = clé de votre wallet), '
                'cette étape est inutile.',
 'mm_title': 'Étape 4 — Connecter MetaMask à votre nœud (facultatif)',
 'mm_body': 'Dans MetaMask : menu des réseaux → Ajouter un réseau → Ajouter un réseau manuellement, puis '
            'saisissez :',
 'mm_rows': [('Nom du réseau', 'Aequitas Chain'),
             ('URL RPC', 'http://VOTRE-IP-PUBLIQUE:8080/rpc'),
             ('ID de chaîne', '1926'),
             ('Symbole', 'AEQ'),
             ('Décimales', '18'),
             ('Explorateur de blocs', 'https://aequitas.digital')],
 'rewards_title': 'Étape 5 — Récompenses de validateur',
 'rewards_box': 'Le fonds des validateurs collecte 40 % de tous les frais du protocole (frais de swap, '
                'démurrage, dépassement du plafond de richesse). Chaque jour à 20 h heure de Berlin, le '
                'fonds est réparti entre les validateurs enregistrés au prorata des blocs produits. Rien à '
                'faire sinon garder le nœud en marche.',
 'rewards_steps': ['NODE_OPERATOR_WALLET doit être un humain enregistré — sinon le réseau refuse '
                   'l\'enregistrement (journal : <font name="Courier">NODE_OPERATOR_WALLET is not a '
                   'registered human</font>).',
                   'Confirmez dans le journal : <font name="Courier" color="#0F766E">[PEERS] Auto-authorized '
                   'validator … (wallet: 0x…)</font> sur un nœud fondateur, et des lignes <font '
                   'name="Courier">[Block #…]</font> sur le vôtre.',
                   'Un nœud arrêté ne produit pas de blocs et ne gagne donc rien pendant ce temps — les '
                   'redémarrages sont sans danger, le nœud rattrape seul.',
                   "Ce qu'un validateur NE fait PAS sans logiciel supplémentaire : accepter de nouveaux "
                   "enregistrements d'humains (ces endpoints répondent 503). Transferts, blocs et "
                   'récompenses fonctionnent sans.'],
 'trouble_title': 'Dépannage',
 'trouble_cols': ['Symptôme', 'Cause probable', 'Solution'],
 'trouble_rows': [('NODE_OPERATOR_WALLET is not a registered human',
                   "Le wallet n'a pas terminé l'enregistrement dans l'app",
                   "Enregistrez-vous d'abord dans l'app, puis redémarrez le nœud."),
                  ('operator_binding_signature missing or invalid',
                   'Clé de signature et wallet différents, sans liaison',
                   'Étape 3b : signez sur /node-binding et renseignez NODE_OPERATOR_BINDING_SIGNATURE.'),
                  ('Frischer Knoten: … produziert nichts, bis er aufgeholt hat',
                   'Normal pendant le rattrapage',
                   'Attendez. La production démarre quand le nœud a bouclé des cycles de synchronisation '
                   'propres avec les nœuds fondateurs.'),
                  ('SELF_URL not set — … Beobachter',
                   'SELF_URL manque dans .env',
                   'Mettez SELF_URL à http://VOTRE-IP-PUBLIQUE:8080 et lancez docker compose up -d.'),
                  ('La hauteur reste loin derrière le réseau',
                   "Import de l'instantané échoué, ou ports fermés",
                   'Cherchez des lignes [BOOTSTRAP] dans le journal ; ouvrez TCP 8080 et 4001 en entrée dans '
                   'le pare-feu / groupe de sécurité.'),
                  ('Le nœud redémarre en boucle, « OOMKilled »',
                   'Pas assez de RAM',
                   'Baissez GOMEMLIMIT dans .env (3GiB avec 8 Go de RAM) ou donnez plus de mémoire au '
                   'serveur.'),
                  ('docker compose : la compilation échoue',
                   "Pas d'internet sortant pendant la compilation",
                   'La compilation télécharge des modules Go ; vérifiez le DNS et le HTTPS sortant sur le '
                   'serveur.')],
 'footer': 'Aequitas Chain · Chain ID 1926 · aequitas.digital · Récompenses de validateur : chaque jour à 20 '
           'h heure de Berlin'}

IT = {'title': "GUIDA PER L'OPERATORE DI NODO AEQUITAS",
 'version': 'v2.0 · 2026-09 · aequitas.digital',
 'tagline': 'Esegui un validatore sul tuo server · Docker Compose · circa 15 minuti, 10 dei quali di '
            'compilazione',
 'prereq_title': 'Prima di iniziare — Cosa ti serve',
 'prereqs': [('1.',
              "<b>Sei un umano registrato:</b> installa l'app Aequitas, completa la registrazione biometrica "
              'e annota il tuo indirizzo wallet. La rete rifiuta un validatore il cui wallet non è un umano '
              'registrato — un umano, un validatore. Affittare server non compra voti.'),
             ('2.',
              '<b>Un server (VPS) con IPv4 pubblico:</b> Ubuntu 22.04 o 24.04, almeno 4 vCPU, 8 GB di RAM, '
              '60 GB SSD (il database oggi occupa circa 18 GB e cresce). I due nodi fondatori girano su 6 '
              'vCPU / 12 GB / 100 GB. Le porte 8080 (API) e 4001 (P2P) devono essere raggiungibili da '
              'internet.'),
             ('3.',
              '<b>Docker con il plugin Compose e git.</b> Su Ubuntu, <font name="Courier">curl -fsSL '
              'https://get.docker.com | sh</font> installa entrambi.'),
             ('4.',
              '<b>Circa 15 minuti.</b> Dieci sono la prima compilazione (il nodo viene compilato dai '
              'sorgenti sul tuo server). Gli aggiornamenti richiedono lo stesso tempo.')],
 'vars_title': 'Passo 1 — Configurazione (.env)',
 'vars_warn': 'Avviso di sicurezza: RELAYER_PRIVATE_KEY e NODE_KEY sono segreti. Chi li possiede È il tuo '
              "nodo. Non incollarli mai in una chat, un'e-mail o un ticket. Il file .env resta sul tuo "
              'server.',
 'var_cols': ['Variabile', 'Obbligatoria?', 'Cosa inserire'],
 'vars': [('POSTGRES_PASSWORD',
           'SÌ',
           'Una password lunga e casuale per il database locale. La usano solo i due container sul tuo '
           'server.'),
          ('SELF_URL',
           'SÌ',
           'Come gli altri nodi raggiungono il TUO: http://TUO-IP-PUBBLICO:8080 (o https://tuo-dominio se '
           "davanti c'è un proxy). Senza, il nodo segue la catena solo come osservatore e non si registra "
           'mai come validatore.'),
          ('NODE_OPERATOR_WALLET',
           'SÌ',
           "Il tuo indirizzo wallet — DEVE essere un umano registrato (registrazione nell'app completata). È "
           'ciò che lega il validatore a una persona. Riceve le ricompense del validatore.'),
          ('RELAYER_PRIVATE_KEY',
           'Consigliata',
           'La chiave che firma i tuoi blocchi (0x…, 66 caratteri). La via più semplice: la chiave privata '
           'di NODE_OPERATOR_WALLET — così non serve alcun collegamento. Se lasciata vuota, il nodo ne '
           'genera una al primo avvio e la stampa UNA volta ("SAVE THIS AS …"); mettila in .env e riavvia, '
           'altrimenti la tua identità cambia a ogni riavvio.'),
          ('NODE_OPERATOR_BINDING_SIGNATURE',
           'Solo se le chiavi differiscono',
           'Se RELAYER_PRIVATE_KEY non è la chiave di NODE_OPERATOR_WALLET: firma il messaggio mostrato su '
           'aequitas.digital/node-binding con il tuo wallet e incolla qui la firma.'),
          ('NODE_KEY',
           'Consigliata',
           'Identità P2P. Generata e stampata al primo avvio se vuota — salvala in .env, per lo stesso '
           'motivo.'),
          ('PRIMARY_NODE_URLS',
           'Preimpostata',
           'I nodi presso cui il tuo si registra e da cui scarica lo snapshot dello stato al primo avvio. '
           'Preimpostato sui due nodi fondatori; lascialo così.'),
          ('GOMEMLIMIT',
           'Preimpostata',
           'Limite di memoria del processo del nodo, preimpostato 5GiB (per 12 GB di RAM). Con 8 GB imposta '
           '3GiB.'),
          ('POSTGRES_SHARED_BUFFERS',
           'Preimpostata',
           'Cache di Postgres, preimpostata 1GB. Un quarto della tua RAM è un buon valore.')],
 'railway_title': 'Passo 2 — Avviare il nodo (Docker Compose)',
 'railway_intro': 'Tutto ciò che fanno i due nodi fondatori, in un file. Il primo comando compila il nodo '
                  'dai sorgenti (circa 10 minuti), avvia Postgres e il nodo e riavvia entrambi '
                  'automaticamente dopo un crash o un riavvio del server.',
 'railway_steps': ['Sul tuo server: <font name="Courier">git clone '
                   'https://github.com/hanoi96international-gif/Aequitas.git</font>',
                   '<font name="Courier">cd Aequitas/deploy/validator</font> e <font name="Courier">cp '
                   '.env.example .env</font>',
                   'Modifica <font name="Courier">.env</font> (ad esempio con <font name="Courier">nano '
                   '.env</font>): compila POSTGRES_PASSWORD, SELF_URL e NODE_OPERATOR_WALLET — vedi la '
                   'tabella sopra',
                   '<font name="Courier">docker compose up -d --build</font> — compila e avvia. La prima '
                   'volta circa 10 minuti',
                   'Guarda il log: <font name="Courier">docker compose logs -f node</font>. Al primo avvio '
                   'il nodo importa lo stato della rete (snapshot) da un nodo fondatore, ne verifica la '
                   'firma e poi scarica i blocchi successivi: <font name="Courier" '
                   'color="#5B21B6">[BOOTSTRAP] Fresh node — importing state from …</font> seguito da <font '
                   'name="Courier" color="#0F766E">[HTTP-SYNC] Added … new blocks</font>',
                   'Se il log ha stampato <font name="Courier">SAVE THIS AS NODE_KEY</font> o <font '
                   'name="Courier">SET THIS AS RELAYER_PRIVATE_KEY</font>: copia ora quei valori in .env ed '
                   'esegui di nuovo <font name="Courier">docker compose up -d</font> — altrimenti il tuo '
                   'nodo avrà una nuova identità a ogni riavvio',
                   'Una volta allineato vedrai righe <font name="Courier">[Block #…]</font> — sono blocchi '
                   'prodotti dal TUO nodo. Fino ad allora il nodo non produce nulla di proposito (<font '
                   'name="Courier">Frischer Knoten: … produziert nichts, bis er aufgeholt hat</font>) — un '
                   'nodo che non ha mai visto la catena non deve inventarne una'],
 'railway_vars_code': 'POSTGRES_PASSWORD      = una-password-lunga-e-casuale\n'
                      'SELF_URL               = http://TUO-IP-PUBBLICO:8080\n'
                      'NODE_OPERATOR_WALLET   = 0xTUO_WALLET_UMANO\n'
                      '# consigliato: la chiave che firma i tuoi blocchi (o vuota al primo avvio)\n'
                      'RELAYER_PRIVATE_KEY    = 0xTUA_CHIAVE_PRIVATA\n'
                      '# preimpostato, lascia così\n'
                      'PRIMARY_NODE_URLS      = http://173.249.37.118:8080,http://194.163.188.71:8080',
 'docker_title': 'Passo 2b — Aggiornare, riavviare, fermare',
 'docker_intro': 'Il nodo conserva il suo stato in due volumi Docker (database e registro dei '
                 "trasferimenti). Un aggiornamento ricostruisce l'immagine dal codice più recente; lo stato "
                 "resta. Non riavviare mai tutti i validatori della rete insieme — uno dopo l'altro.",
 'docker_code': '# Aggiornare al codice più recente (circa 10 minuti)\n'
                'cd Aequitas && git pull && cd deploy/validator && docker compose up -d --build\n'
                '\n'
                '# Riavviare solo il nodo\n'
                'docker compose restart node\n'
                '\n'
                '# Fermare tutto (lo stato resta)\n'
                'docker compose down\n'
                '\n'
                '# Osservare le risorse\n'
                'docker stats',
 'verify_title': 'Passo 3 — Verificare che il nodo funzioni',
 'verify_body': "Confronta il tuo nodo con la rete. Sostituisci TUO-IP-PUBBLICO con l'indirizzo del tuo "
                'server.',
 'verify_code': 'curl -s http://TUO-IP-PUBBLICO:8080/api/status | grep -oE \'"height":[0-9]+\'\n'
                'curl -s https://aequitas.digital/api/status  | grep -oE \'"height":[0-9]+\'\n'
                ' → I due numeri devono coincidere a meno di pochi blocchi.\n'
                '\n'
                'http://TUO-IP-PUBBLICO:8080/           → il tuo explorer\n'
                'http://TUO-IP-PUBBLICO:8080/api/health/combined → tutto ciò che il nodo misura su di sé',
 'verify_note': "Subito dopo il primo avvio l'altezza è molto sotto la rete mentre lo snapshot viene "
                'importato e i blocchi recenti scaricati. Se resta molto indietro per più di 15 minuti, '
                'cerca errori [BOOTSTRAP] nel log e verifica che le porte 8080 e 4001 siano aperte.',
 'valkey_title': 'Passo 3b — Collegare il wallet alla chiave del nodo (solo se differiscono)',
 'valkey_body': 'Se RELAYER_PRIVATE_KEY non è la chiave privata di NODE_OPERATOR_WALLET, dimostra una volta '
                'che entrambe sono tue: apri la pagina sotto, firma il messaggio mostrato con il tuo wallet '
                'e metti la firma in .env come NODE_OPERATOR_BINDING_SIGNATURE.',
 'valkey_code': 'https://aequitas.digital/node-binding',
 'valkey_note': 'Con la configurazione semplice a chiave unica (RELAYER_PRIVATE_KEY = chiave del tuo wallet) '
                'questo passo non serve.',
 'mm_title': 'Passo 4 — Collegare MetaMask al tuo nodo (facoltativo)',
 'mm_body': 'In MetaMask: menu delle reti → Aggiungi rete → Aggiungi una rete manualmente, poi inserisci:',
 'mm_rows': [('Nome della rete', 'Aequitas Chain'),
             ('URL RPC', 'http://TUO-IP-PUBBLICO:8080/rpc'),
             ('ID catena', '1926'),
             ('Simbolo', 'AEQ'),
             ('Decimali', '18'),
             ('Block explorer', 'https://aequitas.digital')],
 'rewards_title': 'Passo 5 — Ricompense del validatore',
 'rewards_box': 'Il fondo dei validatori raccoglie il 40 % di tutte le commissioni del protocollo '
                '(commissioni di swap, demurrage, eccedenza del tetto di ricchezza). Ogni giorno alle 20:00 '
                'ora di Berlino il fondo viene distribuito ai validatori registrati in proporzione ai '
                "blocchi prodotti. Non c'è nulla da fare se non tenere il nodo acceso.",
 'rewards_steps': ['NODE_OPERATOR_WALLET deve essere un umano registrato — altrimenti la rete rifiuta la '
                   'registrazione (log: <font name="Courier">NODE_OPERATOR_WALLET is not a registered '
                   'human</font>).',
                   'Conferma nel log: <font name="Courier" color="#0F766E">[PEERS] Auto-authorized validator '
                   '… (wallet: 0x…)</font> su un nodo fondatore, e righe <font name="Courier">[Block '
                   '#…]</font> sul tuo.',
                   'Un nodo spento non produce blocchi e quindi non guadagna nulla in quel periodo — i '
                   'riavvii sono innocui, il nodo si riallinea da solo.',
                   'Cosa un validatore NON fa senza software aggiuntivo: accettare nuove registrazioni di '
                   'umani (quegli endpoint rispondono 503). Trasferimenti, blocchi e ricompense funzionano '
                   'senza.'],
 'trouble_title': 'Risoluzione dei problemi',
 'trouble_cols': ['Sintomo', 'Causa probabile', 'Soluzione'],
 'trouble_rows': [('NODE_OPERATOR_WALLET is not a registered human',
                   "Il wallet non ha completato la registrazione nell'app",
                   "Registrati prima nell'app, poi riavvia il nodo."),
                  ('operator_binding_signature missing or invalid',
                   'Chiave di firma e wallet diversi, nessun collegamento',
                   'Passo 3b: firma su /node-binding e imposta NODE_OPERATOR_BINDING_SIGNATURE.'),
                  ('Frischer Knoten: … produziert nichts, bis er aufgeholt hat',
                   'Normale durante il riallineamento',
                   'Aspetta. La produzione inizia quando il nodo ha completato cicli di sincronizzazione '
                   'puliti con i nodi fondatori.'),
                  ('SELF_URL not set — … Beobachter',
                   'SELF_URL manca in .env',
                   'Imposta SELF_URL su http://TUO-IP-PUBBLICO:8080 ed esegui docker compose up -d.'),
                  ("L'altezza resta molto sotto la rete",
                   'Import dello snapshot fallito, o porte chiuse',
                   'Cerca righe [BOOTSTRAP] nel log; apri TCP 8080 e 4001 in ingresso nel firewall / gruppo '
                   'di sicurezza.'),
                  ('Il nodo si riavvia in loop, "OOMKilled"',
                   'RAM insufficiente',
                   'Abbassa GOMEMLIMIT in .env (3GiB con 8 GB di RAM) o dai più memoria al server.'),
                  ('docker compose: la compilazione fallisce',
                   'Nessuna connessione in uscita durante la compilazione',
                   'La compilazione scarica moduli Go; verifica DNS e HTTPS in uscita sul server.')],
 'footer': 'Aequitas Chain · Chain ID 1926 · aequitas.digital · Ricompense del validatore: ogni giorno alle '
           '20:00 ora di Berlino'}

PT = {'title': 'GUIA DO OPERADOR DE NÓ AEQUITAS',
 'version': 'v2.0 · 2026-09 · aequitas.digital',
 'tagline': 'Execute um validador no seu próprio servidor · Docker Compose · cerca de 15 minutos, 10 deles a '
            'compilar',
 'prereq_title': 'Antes de começar — O que precisa',
 'prereqs': [('1.',
              '<b>Você é um humano registado:</b> instale a app Aequitas, conclua o registo biométrico e '
              'anote o endereço da sua wallet. A rede recusa um validador cuja wallet não seja um humano '
              'registado — um humano, um validador. Alugar servidores não compra votos.'),
             ('2.',
              '<b>Um servidor (VPS) com IPv4 público:</b> Ubuntu 22.04 ou 24.04, pelo menos 4 vCPU, 8 GB de '
              'RAM, 60 GB SSD (a base de dados tem hoje cerca de 18 GB e cresce). Os dois nós fundadores '
              'usam 6 vCPU / 12 GB / 100 GB. As portas 8080 (API) e 4001 (P2P) têm de estar acessíveis a '
              'partir da internet.'),
             ('3.',
              '<b>Docker com o plugin Compose e git.</b> No Ubuntu, <font name="Courier">curl -fsSL '
              'https://get.docker.com | sh</font> instala ambos.'),
             ('4.',
              '<b>Cerca de 15 minutos.</b> Dez deles são a primeira compilação (o nó é compilado a partir do '
              'código-fonte no seu servidor). As atualizações demoram o mesmo.')],
 'vars_title': 'Passo 1 — Configuração (.env)',
 'vars_warn': 'Aviso de segurança: RELAYER_PRIVATE_KEY e NODE_KEY são segredos. Quem os tiver É o seu nó. '
              'Nunca os cole num chat, e-mail ou ticket. O ficheiro .env fica no seu servidor.',
 'var_cols': ['Variável', 'Obrigatória?', 'O que colocar'],
 'vars': [('POSTGRES_PASSWORD',
           'SIM',
           'Uma palavra-passe longa e aleatória para a base de dados local. Só os dois contentores do seu '
           'servidor a usam.'),
          ('SELF_URL',
           'SIM',
           'Como os outros nós chegam ao SEU: http://SEU-IP-PUBLICO:8080 (ou https://seu-dominio se houver '
           'um proxy à frente). Sem ela, o nó segue a cadeia apenas como observador e nunca se regista como '
           'validador.'),
          ('NODE_OPERATOR_WALLET',
           'SIM',
           'O endereço da sua própria wallet — TEM de ser um humano registado (registo na app concluído). É '
           'isto que liga o validador a uma pessoa. Recebe as recompensas de validador.'),
          ('RELAYER_PRIVATE_KEY',
           'Recomendada',
           'A chave que assina os seus blocos (0x…, 66 caracteres). O mais simples: a chave privada de '
           'NODE_OPERATOR_WALLET — assim não é precisa nenhuma ligação extra. Se ficar vazia, o nó gera uma '
           'no primeiro arranque e imprime-a UMA vez ("SAVE THIS AS …"); coloque-a em .env e reinicie, senão '
           'a sua identidade muda a cada reinício.'),
          ('NODE_OPERATOR_BINDING_SIGNATURE',
           'Só se as chaves forem diferentes',
           'Se RELAYER_PRIVATE_KEY não for a chave de NODE_OPERATOR_WALLET: assine a mensagem mostrada em '
           'aequitas.digital/node-binding com a sua wallet e cole aqui a assinatura.'),
          ('NODE_KEY',
           'Recomendada',
           'Identidade P2P. Gerada e impressa no primeiro arranque se vazia — guarde-a em .env, pela mesma '
           'razão.'),
          ('PRIMARY_NODE_URLS',
           'Predefinida',
           'Os nós junto dos quais o seu se regista e de onde obtém o snapshot do estado no primeiro '
           'arranque. Predefinido para os dois nós fundadores; deixe como está.'),
          ('GOMEMLIMIT',
           'Predefinida',
           'Limite de memória do processo do nó, predefinido 5GiB (para 12 GB de RAM). Com 8 GB use 3GiB.'),
          ('POSTGRES_SHARED_BUFFERS',
           'Predefinida',
           'Cache do Postgres, predefinida 1GB. Um quarto da sua RAM é um bom valor.')],
 'railway_title': 'Passo 2 — Arrancar o nó (Docker Compose)',
 'railway_intro': 'Tudo o que os dois nós fundadores fazem, num só ficheiro. O primeiro comando compila o nó '
                  'a partir do código (cerca de 10 minutos), arranca o Postgres e o nó, e reinicia ambos '
                  'automaticamente após uma falha ou um reinício do servidor.',
 'railway_steps': ['No seu servidor: <font name="Courier">git clone '
                   'https://github.com/hanoi96international-gif/Aequitas.git</font>',
                   '<font name="Courier">cd Aequitas/deploy/validator</font> e <font name="Courier">cp '
                   '.env.example .env</font>',
                   'Edite <font name="Courier">.env</font> (por exemplo com <font name="Courier">nano '
                   '.env</font>): preencha POSTGRES_PASSWORD, SELF_URL e NODE_OPERATOR_WALLET — ver a tabela '
                   'acima',
                   '<font name="Courier">docker compose up -d --build</font> — compila e arranca. Da '
                   'primeira vez demora cerca de 10 minutos',
                   'Acompanhe o log: <font name="Courier">docker compose logs -f node</font>. No primeiro '
                   'arranque, o nó importa o estado da rede (snapshot) a partir de um nó fundador, verifica '
                   'a assinatura e depois descarrega os blocos seguintes: <font name="Courier" '
                   'color="#5B21B6">[BOOTSTRAP] Fresh node — importing state from …</font> seguido de <font '
                   'name="Courier" color="#0F766E">[HTTP-SYNC] Added … new blocks</font>',
                   'Se o log imprimiu <font name="Courier">SAVE THIS AS NODE_KEY</font> ou <font '
                   'name="Courier">SET THIS AS RELAYER_PRIVATE_KEY</font>: copie esses valores para .env '
                   'agora e execute novamente <font name="Courier">docker compose up -d</font> — caso '
                   'contrário o seu nó ganha uma identidade nova a cada reinício',
                   'Quando estiver em dia verá linhas <font name="Courier">[Block #…]</font> — são blocos '
                   'produzidos pelo SEU nó. Até lá, o nó deliberadamente não produz nada (<font '
                   'name="Courier">Frischer Knoten: … produziert nichts, bis er aufgeholt hat</font>) — um '
                   'nó que nunca viu a cadeia não deve inventar uma'],
 'railway_vars_code': 'POSTGRES_PASSWORD      = uma-palavra-passe-longa-e-aleatoria\n'
                      'SELF_URL               = http://SEU-IP-PUBLICO:8080\n'
                      'NODE_OPERATOR_WALLET   = 0xSUA_WALLET_HUMANA\n'
                      '# recomendado: a chave que assina os seus blocos (ou vazia no primeiro arranque)\n'
                      'RELAYER_PRIVATE_KEY    = 0xSUA_CHAVE_PRIVADA\n'
                      '# predefinido, deixe como está\n'
                      'PRIMARY_NODE_URLS      = http://173.249.37.118:8080,http://194.163.188.71:8080',
 'docker_title': 'Passo 2b — Atualizar, reiniciar, parar',
 'docker_intro': 'O nó guarda o seu estado em dois volumes Docker (base de dados e registo de '
                 'transferências). Atualizar reconstrói a imagem com o código mais recente; o estado '
                 'mantém-se. Nunca reinicie todos os validadores da rede ao mesmo tempo — um após o outro.',
 'docker_code': '# Atualizar para o código mais recente (cerca de 10 minutos)\n'
                'cd Aequitas && git pull && cd deploy/validator && docker compose up -d --build\n'
                '\n'
                '# Reiniciar apenas o nó\n'
                'docker compose restart node\n'
                '\n'
                '# Parar tudo (o estado mantém-se)\n'
                'docker compose down\n'
                '\n'
                '# Observar os recursos\n'
                'docker stats',
 'verify_title': 'Passo 3 — Verificar que o nó funciona',
 'verify_body': 'Compare o seu nó com a rede. Substitua SEU-IP-PUBLICO pelo endereço do seu servidor.',
 'verify_code': 'curl -s http://SEU-IP-PUBLICO:8080/api/status | grep -oE \'"height":[0-9]+\'\n'
                'curl -s https://aequitas.digital/api/status  | grep -oE \'"height":[0-9]+\'\n'
                ' → Os dois números têm de ser iguais a menos de alguns blocos.\n'
                '\n'
                'http://SEU-IP-PUBLICO:8080/           → o seu próprio explorador\n'
                'http://SEU-IP-PUBLICO:8080/api/health/combined → tudo o que o nó mede sobre si próprio',
 'verify_note': 'Logo após o primeiro arranque a altura fica muito abaixo da rede enquanto o snapshot é '
                'importado e os blocos recentes descarregados. Se ficar muito atrás por mais de 15 minutos, '
                'procure erros [BOOTSTRAP] no log e confirme que as portas 8080 e 4001 estão abertas.',
 'valkey_title': 'Passo 3b — Ligar a sua wallet à chave do nó (só se forem diferentes)',
 'valkey_body': 'Se RELAYER_PRIVATE_KEY não for a chave privada de NODE_OPERATOR_WALLET, prove uma vez que '
                'ambas são suas: abra a página abaixo, assine a mensagem mostrada com a sua wallet e coloque '
                'a assinatura em .env como NODE_OPERATOR_BINDING_SIGNATURE.',
 'valkey_code': 'https://aequitas.digital/node-binding',
 'valkey_note': 'Com a configuração simples de chave única (RELAYER_PRIVATE_KEY = chave da sua wallet) este '
                'passo não é necessário.',
 'mm_title': 'Passo 4 — Ligar a MetaMask ao seu nó (opcional)',
 'mm_body': 'Na MetaMask: menu de redes → Adicionar rede → Adicionar uma rede manualmente, depois introduza:',
 'mm_rows': [('Nome da rede', 'Aequitas Chain'),
             ('URL RPC', 'http://SEU-IP-PUBLICO:8080/rpc'),
             ('ID da cadeia', '1926'),
             ('Símbolo', 'AEQ'),
             ('Decimais', '18'),
             ('Explorador de blocos', 'https://aequitas.digital')],
 'rewards_title': 'Passo 5 — Recompensas de validador',
 'rewards_box': 'O fundo dos validadores recolhe 40 % de todas as taxas do protocolo (taxas de swap, '
                'demurrage, excedente do teto de riqueza). Todos os dias às 20:00, hora de Berlim, o fundo é '
                'distribuído pelos validadores registados em proporção aos blocos produzidos. Não há nada a '
                'fazer além de manter o nó a correr.',
 'rewards_steps': ['NODE_OPERATOR_WALLET tem de ser um humano registado — senão a rede recusa o registo '
                   '(log: <font name="Courier">NODE_OPERATOR_WALLET is not a registered human</font>).',
                   'Confirme no log: <font name="Courier" color="#0F766E">[PEERS] Auto-authorized validator '
                   '… (wallet: 0x…)</font> num nó fundador, e linhas <font name="Courier">[Block #…]</font> '
                   'no seu.',
                   'Um nó desligado não produz blocos e por isso não ganha nada nesse período — os reinícios '
                   'são inofensivos, o nó recupera sozinho.',
                   'O que um validador NÃO faz sem software adicional: aceitar novos registos de humanos '
                   '(esses endpoints respondem 503). Transferências, blocos e recompensas funcionam sem '
                   'ele.'],
 'trouble_title': 'Resolução de problemas',
 'trouble_cols': ['Sintoma', 'Causa provável', 'Solução'],
 'trouble_rows': [('NODE_OPERATOR_WALLET is not a registered human',
                   'A wallet não concluiu o registo na app',
                   'Registe-se primeiro na app e reinicie o nó.'),
                  ('operator_binding_signature missing or invalid',
                   'Chave de assinatura e wallet diferentes, sem ligação',
                   'Passo 3b: assine em /node-binding e defina NODE_OPERATOR_BINDING_SIGNATURE.'),
                  ('Frischer Knoten: … produziert nichts, bis er aufgeholt hat',
                   'Normal enquanto recupera',
                   'Aguarde. A produção começa quando o nó concluir ciclos de sincronização limpos com os '
                   'nós fundadores.'),
                  ('SELF_URL not set — … Beobachter',
                   'Falta SELF_URL em .env',
                   'Defina SELF_URL como http://SEU-IP-PUBLICO:8080 e execute docker compose up -d.'),
                  ('A altura fica muito abaixo da rede',
                   'Importação do snapshot falhou, ou portas fechadas',
                   'Procure linhas [BOOTSTRAP] no log; abra TCP 8080 e 4001 de entrada na firewall / grupo '
                   'de segurança.'),
                  ('O nó reinicia em ciclo, "OOMKilled"',
                   'RAM insuficiente',
                   'Baixe GOMEMLIMIT em .env (3GiB com 8 GB de RAM) ou dê mais memória ao servidor.'),
                  ('docker compose: a compilação falha',
                   'Sem internet de saída durante a compilação',
                   'A compilação descarrega módulos Go; verifique o DNS e o HTTPS de saída no servidor.')],
 'footer': 'Aequitas Chain · Chain ID 1926 · aequitas.digital · Recompensas de validador: diariamente às '
           '20:00, hora de Berlim'}

TR = {'title': 'AEQUITAS DÜĞÜM OPERATÖRÜ KILAVUZU',
 'version': 'v2.0 · 2026-09 · aequitas.digital',
 'tagline': "Kendi sunucunuzda bir doğrulayıcı çalıştırın · Docker Compose · yaklaşık 15 dakika, 10'u "
            'derleme',
 'prereq_title': 'Başlamadan önce — Neye ihtiyacınız var',
 'prereqs': [('1.',
              '<b>Kayıtlı bir insansınız:</b> Aequitas uygulamasını kurun, biyometrik kaydı tamamlayın ve '
              'cüzdan adresinizi not edin. Ağ, cüzdanı kayıtlı bir insan olmayan doğrulayıcıyı reddeder — '
              'bir insan, bir doğrulayıcı. Sunucu kiralamak oy satın almaz.'),
             ('2.',
              '<b>Genel IPv4 adresli bir sunucu (VPS):</b> Ubuntu 22.04 veya 24.04, en az 4 vCPU, 8 GB RAM, '
              '60 GB SSD (veritabanı bugün yaklaşık 18 GB ve büyüyor). İki kurucu düğüm 6 vCPU / 12 GB / 100 '
              'GB ile çalışıyor. 8080 (API) ve 4001 (P2P) portları internetten erişilebilir olmalı.'),
             ('3.',
              '<b>Compose eklentili Docker ve git.</b> Ubuntu\'da <font name="Courier">curl -fsSL '
              'https://get.docker.com | sh</font> ikisini de kurar.'),
             ('4.',
              '<b>Yaklaşık 15 dakika.</b> Bunun onu ilk derlemedir (düğüm sunucunuzda kaynak koddan '
              'derlenir). Güncellemeler de aynı sürer.')],
 'vars_title': 'Adım 1 — Yapılandırma (.env)',
 'vars_warn': 'Güvenlik uyarısı: RELAYER_PRIVATE_KEY ve NODE_KEY gizlidir. Bunlara sahip olan, düğümünüz '
              'OLUR. Asla sohbete, e-postaya veya bir bilete yapıştırmayın. .env dosyası sunucunuzda kalır.',
 'var_cols': ['Değişken', 'Zorunlu mu?', 'Ne yazılacak'],
 'vars': [('POSTGRES_PASSWORD',
           'EVET',
           'Yerel veritabanı için uzun, rastgele bir parola. Yalnızca sunucunuzdaki iki konteyner kullanır.'),
          ('SELF_URL',
           'EVET',
           'Diğer düğümlerin SİZİN düğümünüze nasıl ulaştığı: http://GENEL-IP:8080 (önünde proxy varsa '
           'https://alan-adiniz). Bu olmadan düğüm zinciri yalnızca gözlemci olarak izler ve asla '
           'doğrulayıcı olarak kaydolmaz.'),
          ('NODE_OPERATOR_WALLET',
           'EVET',
           'Kendi cüzdan adresiniz — kayıtlı bir insan OLMALI (uygulama kaydı tamamlanmış). Doğrulayıcıyı '
           'bir kişiye bağlayan budur. Doğrulayıcı ödüllerini alır.'),
          ('RELAYER_PRIVATE_KEY',
           'Önerilir',
           "Bloklarınızı imzalayan anahtar (0x…, 66 karakter). En basiti: NODE_OPERATOR_WALLET'ın özel "
           'anahtarı — o zaman ek bağlama gerekmez. Boş bırakılırsa düğüm ilk başlatmada bir tane üretir ve '
           'BİR kez yazdırır ("SAVE THIS AS …"); .env dosyasına koyup yeniden başlatın, yoksa kimliğiniz her '
           'yeniden başlatmada değişir.'),
          ('NODE_OPERATOR_BINDING_SIGNATURE',
           'Yalnızca anahtarlar farklıysa',
           "RELAYER_PRIVATE_KEY, NODE_OPERATOR_WALLET'ın anahtarı değilse: aequitas.digital/node-binding "
           'sayfasındaki mesajı cüzdanınızla imzalayın ve imzayı buraya yapıştırın.'),
          ('NODE_KEY',
           'Önerilir',
           'P2P kimliği. Boşsa ilk başlatmada üretilir ve yazdırılır — aynı nedenle .env dosyasına '
           'kaydedin.'),
          ('PRIMARY_NODE_URLS',
           'Önceden ayarlı',
           'Düğümünüzün kaydolduğu ve ilk başlatmada durum anlık görüntüsünü aldığı düğümler. İki kurucu '
           'düğüme önceden ayarlıdır; olduğu gibi bırakın.'),
          ('GOMEMLIMIT',
           'Önceden ayarlı',
           "Düğüm sürecinin bellek sınırı, önceden 5GiB (12 GB RAM için). 8 GB RAM'de 3GiB yazın."),
          ('POSTGRES_SHARED_BUFFERS',
           'Önceden ayarlı',
           "Postgres önbelleği, önceden 1GB. RAM'inizin dörtte biri iyi bir değerdir.")],
 'railway_title': 'Adım 2 — Düğümü başlatın (Docker Compose)',
 'railway_intro': 'İki kurucu düğümün yaptığı her şey, tek dosyada. İlk komut düğümü kaynaktan derler '
                  "(yaklaşık 10 dakika), Postgres'i ve düğümü başlatır ve bir çökme ya da sunucu yeniden "
                  'başlatması sonrasında ikisini de otomatik yeniden başlatır.',
 'railway_steps': ['Sunucunuzda: <font name="Courier">git clone '
                   'https://github.com/hanoi96international-gif/Aequitas.git</font>',
                   '<font name="Courier">cd Aequitas/deploy/validator</font> ve <font name="Courier">cp '
                   '.env.example .env</font>',
                   '<font name="Courier">.env</font> dosyasını düzenleyin (örneğin <font name="Courier">nano '
                   '.env</font> ile): POSTGRES_PASSWORD, SELF_URL ve NODE_OPERATOR_WALLET değerlerini girin '
                   '— yukarıdaki tabloya bakın',
                   '<font name="Courier">docker compose up -d --build</font> — derler ve başlatır. İlk '
                   'seferde yaklaşık 10 dakika',
                   'Günlüğü izleyin: <font name="Courier">docker compose logs -f node</font>. İlk başlatmada '
                   'düğüm ağ durumunu (anlık görüntü) bir kurucu düğümden alır, imzasını doğrular ve sonraki '
                   'blokları çeker: <font name="Courier" color="#5B21B6">[BOOTSTRAP] Fresh node — importing '
                   'state from …</font> ardından <font name="Courier" color="#0F766E">[HTTP-SYNC] Added … '
                   'new blocks</font>',
                   'Günlükte <font name="Courier">SAVE THIS AS NODE_KEY</font> veya <font name="Courier">SET '
                   'THIS AS RELAYER_PRIVATE_KEY</font> yazdıysa: bu değerleri şimdi .env dosyasına '
                   'kopyalayın ve <font name="Courier">docker compose up -d</font> komutunu yeniden '
                   'çalıştırın — yoksa düğümünüz her yeniden başlatmada yeni bir kimlik alır',
                   'Yetiştiğinde <font name="Courier">[Block #…]</font> satırları görürsünüz — bunlar SİZİN '
                   'düğümünüzün ürettiği bloklardır. O zamana kadar düğüm bilerek hiçbir şey üretmez (<font '
                   'name="Courier">Frischer Knoten: … produziert nichts, bis er aufgeholt hat</font>) — '
                   'zinciri hiç görmemiş bir düğüm kendi zincirini uyduramaz'],
 'railway_vars_code': 'POSTGRES_PASSWORD      = uzun-rastgele-bir-parola\n'
                      'SELF_URL               = http://GENEL-IP:8080\n'
                      'NODE_OPERATOR_WALLET   = 0xINSAN_CUZDANINIZ\n'
                      '# önerilir: bloklarınızı imzalayan anahtar (ya da ilk başlatmada boş)\n'
                      'RELAYER_PRIVATE_KEY    = 0xOZEL_ANAHTARINIZ\n'
                      '# önceden ayarlı, olduğu gibi bırakın\n'
                      'PRIMARY_NODE_URLS      = http://173.249.37.118:8080,http://194.163.188.71:8080',
 'docker_title': 'Adım 2b — Güncelleme, yeniden başlatma, durdurma',
 'docker_intro': 'Düğüm durumunu iki Docker biriminde tutar (veritabanı ve transfer günlüğü). Güncelleme '
                 'imajı en yeni koddan yeniden oluşturur; durum korunur. Ağdaki tüm doğrulayıcıları asla '
                 'aynı anda yeniden başlatmayın — birbiri ardına.',
 'docker_code': '# En yeni koda güncelleyin (yaklaşık 10 dakika)\n'
                'cd Aequitas && git pull && cd deploy/validator && docker compose up -d --build\n'
                '\n'
                '# Yalnızca düğümü yeniden başlatın\n'
                'docker compose restart node\n'
                '\n'
                '# Her şeyi durdurun (durum korunur)\n'
                'docker compose down\n'
                '\n'
                '# Kaynak kullanımını izleyin\n'
                'docker stats',
 'verify_title': 'Adım 3 — Düğümünüzün çalıştığını doğrulayın',
 'verify_body': 'Düğümünüzü ağla karşılaştırın. GENEL-IP yerine sunucu adresinizi yazın.',
 'verify_code': 'curl -s http://GENEL-IP:8080/api/status | grep -oE \'"height":[0-9]+\'\n'
                'curl -s https://aequitas.digital/api/status  | grep -oE \'"height":[0-9]+\'\n'
                ' → İki sayı birkaç blok farkla eşit olmalı.\n'
                '\n'
                'http://GENEL-IP:8080/           → kendi gezgininiz\n'
                'http://GENEL-IP:8080/api/health/combined → düğümün kendisi hakkında ölçtüğü her şey',
 'verify_note': 'İlk başlatmadan hemen sonra, anlık görüntü içe aktarılır ve son bloklar çekilirken '
                'yükseklik ağın çok altındadır. 15 dakikadan uzun süre çok geride kalırsa günlükte '
                '[BOOTSTRAP] hatalarını arayın ve 8080 ile 4001 portlarının açık olduğunu kontrol edin.',
 'valkey_title': 'Adım 3b — Cüzdanınızı düğüm anahtarına bağlayın (yalnızca farklıysa)',
 'valkey_body': "RELAYER_PRIVATE_KEY, NODE_OPERATOR_WALLET'ın özel anahtarı değilse ikisinin de size ait "
                'olduğunu bir kez kanıtlayın: aşağıdaki sayfayı açın, gösterilen mesajı cüzdanınızla '
                'imzalayın ve imzayı .env dosyasına NODE_OPERATOR_BINDING_SIGNATURE olarak koyun.',
 'valkey_code': 'https://aequitas.digital/node-binding',
 'valkey_note': 'Basit tek anahtar kurulumunda (RELAYER_PRIVATE_KEY = cüzdanınızın anahtarı) bu adım '
                'gerekmez.',
 'mm_title': "Adım 4 — MetaMask'ı düğümünüze bağlayın (isteğe bağlı)",
 'mm_body': "MetaMask'ta: ağ menüsü → Ağ ekle → Ağı elle ekle, sonra girin:",
 'mm_rows': [('Ağ adı', 'Aequitas Chain'),
             ('RPC URL', 'http://GENEL-IP:8080/rpc'),
             ('Zincir kimliği', '1926'),
             ('Sembol', 'AEQ'),
             ('Ondalık', '18'),
             ('Blok gezgini', 'https://aequitas.digital')],
 'rewards_title': 'Adım 5 — Doğrulayıcı ödülleri',
 'rewards_box': "Doğrulayıcı havuzu tüm protokol ücretlerinin %40'ını toplar (takas ücretleri, demurrage, "
                "servet tavanı fazlası). Her gün Berlin saatiyle 20:00'de havuz, ürettikleri bloklarla "
                'orantılı olarak kayıtlı doğrulayıcılara dağıtılır. Düğümü çalışır tutmaktan başka yapacak '
                'bir şey yok.',
 'rewards_steps': ['NODE_OPERATOR_WALLET kayıtlı bir insan olmalı — aksi halde ağ kaydı reddeder (günlük: '
                   '<font name="Courier">NODE_OPERATOR_WALLET is not a registered human</font>).',
                   'Günlükte doğrulayın: bir kurucu düğümde <font name="Courier" color="#0F766E">[PEERS] '
                   'Auto-authorized validator … (wallet: 0x…)</font>, sizinkinde <font name="Courier">[Block '
                   '#…]</font> satırları.',
                   'Kapalı bir düğüm blok üretmez ve o sürede hiçbir şey kazanmaz — yeniden başlatmalar '
                   'zararsızdır, düğüm kendi kendine yetişir.',
                   'Bir doğrulayıcının ek yazılım olmadan YAPMADIĞI şey: yeni insan kayıtlarını kabul etmek '
                   '(bu uç noktalar 503 döner). Transferler, bloklar ve ödüller onsuz çalışır.'],
 'trouble_title': 'Sorun giderme',
 'trouble_cols': ['Belirti', 'Olası neden', 'Çözüm'],
 'trouble_rows': [('NODE_OPERATOR_WALLET is not a registered human',
                   'Cüzdan uygulama kaydını tamamlamamış',
                   'Önce uygulamada kaydolun, sonra düğümü yeniden başlatın.'),
                  ('operator_binding_signature missing or invalid',
                   'İmza anahtarı ile cüzdan farklı, bağlama yok',
                   'Adım 3b: /node-binding üzerinde imzalayın ve NODE_OPERATOR_BINDING_SIGNATURE ayarlayın.'),
                  ('Frischer Knoten: … produziert nichts, bis er aufgeholt hat',
                   'Yetişirken normal',
                   'Bekleyin. Düğüm kurucu düğümlerle temiz eşitleme döngülerini tamamlayınca üretim '
                   'başlar.'),
                  ('SELF_URL not set — … Beobachter',
                   '.env dosyasında SELF_URL yok',
                   'SELF_URL değerini http://GENEL-IP:8080 yapın ve docker compose up -d çalıştırın.'),
                  ('Yükseklik ağın çok altında kalıyor',
                   'Anlık görüntü içe aktarma başarısız ya da portlar kapalı',
                   'Günlükte [BOOTSTRAP] satırlarını arayın; güvenlik duvarında / güvenlik grubunda TCP 8080 '
                   've 4001 gelen trafiğini açın.'),
                  ('Düğüm döngü halinde yeniden başlıyor, "OOMKilled"',
                   'Yetersiz RAM',
                   ".env dosyasında GOMEMLIMIT değerini düşürün (8 GB RAM'de 3GiB) veya sunucuya daha çok "
                   'bellek verin.'),
                  ('docker compose: derleme başarısız',
                   'Derleme sırasında giden internet yok',
                   "Derleme Go modüllerini indirir; sunucuda DNS ve giden HTTPS'yi kontrol edin.")],
 'footer': 'Aequitas Chain · Chain ID 1926 · aequitas.digital · Doğrulayıcı ödülleri: her gün Berlin '
           'saatiyle 20:00'}

ID = {'title': 'PANDUAN OPERATOR NODE AEQUITAS',
 'version': 'v2.0 · 2026-09 · aequitas.digital',
 'tagline': 'Jalankan validator di server Anda sendiri · Docker Compose · sekitar 15 menit, 10 di antaranya '
            'untuk kompilasi',
 'prereq_title': 'Sebelum mulai — Yang Anda butuhkan',
 'prereqs': [('1.',
              '<b>Anda adalah manusia terdaftar:</b> pasang aplikasi Aequitas, selesaikan pendaftaran '
              'biometrik, dan catat alamat wallet Anda. Jaringan menolak validator yang wallet-nya bukan '
              'manusia terdaftar — satu manusia, satu validator. Menyewa server tidak membeli suara.'),
             ('2.',
              '<b>Sebuah server (VPS) dengan IPv4 publik:</b> Ubuntu 22.04 atau 24.04, minimal 4 vCPU, 8 GB '
              'RAM, 60 GB SSD (basis data saat ini sekitar 18 GB dan terus tumbuh). Dua node pendiri memakai '
              '6 vCPU / 12 GB / 100 GB. Port 8080 (API) dan 4001 (P2P) harus dapat diakses dari internet.'),
             ('3.',
              '<b>Docker dengan plugin Compose, dan git.</b> Di Ubuntu, <font name="Courier">curl -fsSL '
              'https://get.docker.com | sh</font> memasang keduanya.'),
             ('4.',
              '<b>Sekitar 15 menit.</b> Sepuluh di antaranya adalah kompilasi pertama (node dikompilasi dari '
              'kode sumber di server Anda). Pembaruan memakan waktu yang sama.')],
 'vars_title': 'Langkah 1 — Konfigurasi (.env)',
 'vars_warn': 'Peringatan keamanan: RELAYER_PRIVATE_KEY dan NODE_KEY adalah rahasia. Siapa pun yang '
              'memilikinya ADALAH node Anda. Jangan pernah menempelkannya di chat, email, atau tiket. Berkas '
              '.env tetap di server Anda.',
 'var_cols': ['Variabel', 'Wajib?', 'Yang diisi'],
 'vars': [('POSTGRES_PASSWORD',
           'YA',
           'Kata sandi panjang dan acak untuk basis data lokal. Hanya dipakai oleh dua kontainer di server '
           'Anda.'),
          ('SELF_URL',
           'YA',
           'Cara node lain menjangkau node ANDA: http://IP-PUBLIK-ANDA:8080 (atau https://domain-anda jika '
           'ada proxy di depannya). Tanpa ini node hanya mengikuti rantai sebagai pengamat dan tidak pernah '
           'mendaftar sebagai validator.'),
          ('NODE_OPERATOR_WALLET',
           'YA',
           'Alamat wallet Anda sendiri — HARUS manusia terdaftar (pendaftaran di aplikasi selesai). Inilah '
           'yang mengikat validator ke seseorang. Menerima imbalan validator.'),
          ('RELAYER_PRIVATE_KEY',
           'Disarankan',
           'Kunci yang menandatangani blok Anda (0x…, 66 karakter). Paling sederhana: kunci privat dari '
           'NODE_OPERATOR_WALLET — maka tidak perlu pengikatan tambahan. Jika dikosongkan, node membuatnya '
           'saat pertama kali dijalankan dan mencetaknya SEKALI ("SAVE THIS AS …"); masukkan ke .env dan '
           'mulai ulang, atau identitas Anda berubah setiap kali dimulai ulang.'),
          ('NODE_OPERATOR_BINDING_SIGNATURE',
           'Hanya jika kuncinya berbeda',
           'Jika RELAYER_PRIVATE_KEY bukan kunci dari NODE_OPERATOR_WALLET: tanda tangani pesan yang '
           'ditampilkan di aequitas.digital/node-binding dengan wallet Anda dan tempel tanda tangannya di '
           'sini.'),
          ('NODE_KEY',
           'Disarankan',
           'Identitas P2P. Dibuat dan dicetak saat pertama dijalankan jika kosong — simpan ke .env, dengan '
           'alasan yang sama.'),
          ('PRIMARY_NODE_URLS',
           'Sudah diatur',
           'Node tempat node Anda mendaftar dan mengambil snapshot keadaan saat pertama dijalankan. Sudah '
           'diatur ke dua node pendiri; biarkan apa adanya.'),
          ('GOMEMLIMIT',
           'Sudah diatur',
           'Batas memori proses node, sudah diatur 5GiB (untuk RAM 12 GB). Dengan RAM 8 GB isi 3GiB.'),
          ('POSTGRES_SHARED_BUFFERS',
           'Sudah diatur',
           'Cache Postgres, sudah diatur 1GB. Seperempat RAM Anda adalah nilai yang baik.')],
 'railway_title': 'Langkah 2 — Menjalankan node (Docker Compose)',
 'railway_intro': 'Semua yang dilakukan dua node pendiri, dalam satu berkas. Perintah pertama mengompilasi '
                  'node dari kode sumber (sekitar 10 menit), menjalankan Postgres dan node, dan memulai '
                  'ulang keduanya secara otomatis setelah crash atau reboot server.',
 'railway_steps': ['Di server Anda: <font name="Courier">git clone '
                   'https://github.com/hanoi96international-gif/Aequitas.git</font>',
                   '<font name="Courier">cd Aequitas/deploy/validator</font> lalu <font name="Courier">cp '
                   '.env.example .env</font>',
                   'Sunting <font name="Courier">.env</font> (misalnya dengan <font name="Courier">nano '
                   '.env</font>): isi POSTGRES_PASSWORD, SELF_URL, dan NODE_OPERATOR_WALLET — lihat tabel di '
                   'atas',
                   '<font name="Courier">docker compose up -d --build</font> — mengompilasi dan menjalankan. '
                   'Pertama kali sekitar 10 menit',
                   'Pantau log: <font name="Courier">docker compose logs -f node</font>. Saat pertama '
                   'dijalankan, node mengimpor keadaan jaringan (snapshot) dari node pendiri, memeriksa '
                   'tanda tangannya, lalu mengunduh blok-blok sesudahnya: <font name="Courier" '
                   'color="#5B21B6">[BOOTSTRAP] Fresh node — importing state from …</font> diikuti <font '
                   'name="Courier" color="#0F766E">[HTTP-SYNC] Added … new blocks</font>',
                   'Jika log mencetak <font name="Courier">SAVE THIS AS NODE_KEY</font> atau <font '
                   'name="Courier">SET THIS AS RELAYER_PRIVATE_KEY</font>: salin nilai tersebut ke .env '
                   'sekarang dan jalankan lagi <font name="Courier">docker compose up -d</font> — jika '
                   'tidak, node Anda mendapat identitas baru setiap kali dimulai ulang',
                   'Setelah mengejar ketertinggalan Anda akan melihat baris <font name="Courier">[Block '
                   '#…]</font> — itu blok yang diproduksi node ANDA. Sampai saat itu node sengaja tidak '
                   'memproduksi apa pun (<font name="Courier">Frischer Knoten: … produziert nichts, bis er '
                   'aufgeholt hat</font>) — node yang belum pernah melihat rantai tidak boleh mengarangnya'],
 'railway_vars_code': 'POSTGRES_PASSWORD      = kata-sandi-panjang-dan-acak\n'
                      'SELF_URL               = http://IP-PUBLIK-ANDA:8080\n'
                      'NODE_OPERATOR_WALLET   = 0xWALLET_MANUSIA_ANDA\n'
                      '# disarankan: kunci yang menandatangani blok Anda (atau kosong saat pertama '
                      'dijalankan)\n'
                      'RELAYER_PRIVATE_KEY    = 0xKUNCI_PRIVAT_ANDA\n'
                      '# sudah diatur, biarkan apa adanya\n'
                      'PRIMARY_NODE_URLS      = http://173.249.37.118:8080,http://194.163.188.71:8080',
 'docker_title': 'Langkah 2b — Memperbarui, memulai ulang, menghentikan',
 'docker_intro': 'Node menyimpan keadaannya di dua volume Docker (basis data dan log transfer). Pembaruan '
                 'membangun ulang image dari kode terbaru; keadaan tetap. Jangan pernah memulai ulang semua '
                 'validator jaringan sekaligus — satu per satu.',
 'docker_code': '# Perbarui ke kode terbaru (sekitar 10 menit)\n'
                'cd Aequitas && git pull && cd deploy/validator && docker compose up -d --build\n'
                '\n'
                '# Mulai ulang node saja\n'
                'docker compose restart node\n'
                '\n'
                '# Hentikan semuanya (keadaan tersimpan)\n'
                'docker compose down\n'
                '\n'
                '# Pantau penggunaan sumber daya\n'
                'docker stats',
 'verify_title': 'Langkah 3 — Memastikan node Anda berjalan',
 'verify_body': 'Bandingkan node Anda dengan jaringan. Ganti IP-PUBLIK-ANDA dengan alamat server Anda.',
 'verify_code': 'curl -s http://IP-PUBLIK-ANDA:8080/api/status | grep -oE \'"height":[0-9]+\'\n'
                'curl -s https://aequitas.digital/api/status  | grep -oE \'"height":[0-9]+\'\n'
                ' → Kedua angka harus sama, selisih paling banyak beberapa blok.\n'
                '\n'
                'http://IP-PUBLIK-ANDA:8080/           → explorer Anda sendiri\n'
                'http://IP-PUBLIK-ANDA:8080/api/health/combined → semua yang diukur node tentang dirinya',
 'verify_note': 'Tepat setelah dijalankan pertama kali, tinggi blok jauh di bawah jaringan selagi snapshot '
                'diimpor dan blok terbaru diunduh. Jika tetap jauh tertinggal lebih dari 15 menit, periksa '
                'log untuk kesalahan [BOOTSTRAP] dan pastikan port 8080 dan 4001 terbuka.',
 'valkey_title': 'Langkah 3b — Mengikat wallet ke kunci node (hanya jika berbeda)',
 'valkey_body': 'Jika RELAYER_PRIVATE_KEY bukan kunci privat NODE_OPERATOR_WALLET, buktikan sekali bahwa '
                'keduanya milik Anda: buka halaman di bawah, tanda tangani pesan yang ditampilkan dengan '
                'wallet Anda, dan masukkan tanda tangannya ke .env sebagai NODE_OPERATOR_BINDING_SIGNATURE.',
 'valkey_code': 'https://aequitas.digital/node-binding',
 'valkey_note': 'Dengan pengaturan sederhana satu kunci (RELAYER_PRIVATE_KEY = kunci wallet Anda) langkah '
                'ini tidak diperlukan.',
 'mm_title': 'Langkah 4 — Menghubungkan MetaMask ke node Anda (opsional)',
 'mm_body': 'Di MetaMask: menu jaringan → Tambah jaringan → Tambah jaringan secara manual, lalu masukkan:',
 'mm_rows': [('Nama jaringan', 'Aequitas Chain'),
             ('URL RPC', 'http://IP-PUBLIK-ANDA:8080/rpc'),
             ('ID rantai', '1926'),
             ('Simbol', 'AEQ'),
             ('Desimal', '18'),
             ('Block explorer', 'https://aequitas.digital')],
 'rewards_title': 'Langkah 5 — Imbalan validator',
 'rewards_box': 'Kolam validator mengumpulkan 40 % dari semua biaya protokol (biaya swap, demurrage, '
                'kelebihan batas kekayaan). Setiap hari pukul 20:00 waktu Berlin kolam dibagikan ke '
                'validator terdaftar sebanding dengan blok yang diproduksi. Tidak ada yang perlu dilakukan '
                'selain menjaga node tetap berjalan.',
 'rewards_steps': ['NODE_OPERATOR_WALLET harus manusia terdaftar — jika tidak, jaringan menolak pendaftaran '
                   '(log: <font name="Courier">NODE_OPERATOR_WALLET is not a registered human</font>).',
                   'Pastikan di log: <font name="Courier" color="#0F766E">[PEERS] Auto-authorized validator '
                   '… (wallet: 0x…)</font> di node pendiri, dan baris <font name="Courier">[Block #…]</font> '
                   'di node Anda.',
                   'Node yang mati tidak memproduksi blok sehingga tidak mendapat apa-apa selama itu — '
                   'memulai ulang tidak berbahaya, node mengejar sendiri.',
                   'Yang TIDAK dilakukan validator tanpa perangkat lunak tambahan: menerima pendaftaran '
                   'manusia baru (endpoint tersebut menjawab 503). Transfer, blok, dan imbalan berjalan '
                   'tanpanya.'],
 'trouble_title': 'Pemecahan masalah',
 'trouble_cols': ['Gejala', 'Kemungkinan penyebab', 'Solusi'],
 'trouble_rows': [('NODE_OPERATOR_WALLET is not a registered human',
                   'Wallet belum menyelesaikan pendaftaran di aplikasi',
                   'Daftar dulu di aplikasi, lalu mulai ulang node.'),
                  ('operator_binding_signature missing or invalid',
                   'Kunci penandatangan dan wallet berbeda, tanpa pengikatan',
                   'Langkah 3b: tanda tangani di /node-binding dan atur NODE_OPERATOR_BINDING_SIGNATURE.'),
                  ('Frischer Knoten: … produziert nichts, bis er aufgeholt hat',
                   'Normal selagi mengejar',
                   'Tunggu. Produksi dimulai setelah node menyelesaikan siklus sinkronisasi bersih dengan '
                   'node pendiri.'),
                  ('SELF_URL not set — … Beobachter',
                   'SELF_URL tidak ada di .env',
                   'Atur SELF_URL ke http://IP-PUBLIK-ANDA:8080 dan jalankan docker compose up -d.'),
                  ('Tinggi blok tetap jauh di bawah jaringan',
                   'Impor snapshot gagal, atau port tertutup',
                   'Cari baris [BOOTSTRAP] di log; buka TCP 8080 dan 4001 masuk di firewall / grup '
                   'keamanan.'),
                  ('Node berulang kali mulai ulang, "OOMKilled"',
                   'RAM tidak cukup',
                   'Turunkan GOMEMLIMIT di .env (3GiB pada RAM 8 GB) atau beri server lebih banyak memori.'),
                  ('docker compose: kompilasi gagal',
                   'Tidak ada internet keluar selama kompilasi',
                   'Kompilasi mengunduh modul Go; periksa DNS dan HTTPS keluar di server.')],
 'footer': 'Aequitas Chain · Chain ID 1926 · aequitas.digital · Imbalan validator: setiap hari pukul 20:00 '
           'waktu Berlin'}

RU = {'title': 'РУКОВОДСТВО ОПЕРАТОРА УЗЛА AEQUITAS',
 'version': 'v2.0 · 2026-09 · aequitas.digital',
 'tagline': 'Запустите валидатор на собственном сервере · Docker Compose · около 15 минут, из них 10 — '
            'сборка',
 'prereq_title': 'Перед началом — что нужно',
 'prereqs': [('1.',
              '<b>Вы — зарегистрированный человек:</b> установите приложение Aequitas, пройдите '
              'биометрическую регистрацию и запишите адрес кошелька. Сеть отклоняет валидатор, чей кошелёк '
              'не принадлежит зарегистрированному человеку — один человек, один валидатор. Аренда серверов '
              'голосов не покупает.'),
             ('2.',
              '<b>Сервер (VPS) с публичным IPv4:</b> Ubuntu 22.04 или 24.04, минимум 4 vCPU, 8 ГБ ОЗУ, 60 ГБ '
              'SSD (база данных сейчас около 18 ГБ и растёт). Два узла-основателя работают на 6 vCPU / 12 ГБ '
              '/ 100 ГБ. Порты 8080 (API) и 4001 (P2P) должны быть доступны из интернета.'),
             ('3.',
              '<b>Docker с плагином Compose и git.</b> В Ubuntu <font name="Courier">curl -fsSL '
              'https://get.docker.com | sh</font> устанавливает оба.'),
             ('4.',
              '<b>Около 15 минут.</b> Десять из них — первая сборка (узел компилируется из исходников на '
              'вашем сервере). Обновления занимают столько же.')],
 'vars_title': 'Шаг 1 — Настройка (.env)',
 'vars_warn': 'Предупреждение: RELAYER_PRIVATE_KEY и NODE_KEY — секреты. Кто ими владеет, тот И ЕСТЬ ваш '
              'узел. Никогда не вставляйте их в чат, письмо или тикет. Файл .env остаётся на вашем сервере.',
 'var_cols': ['Переменная', 'Обязательно?', 'Что указать'],
 'vars': [('POSTGRES_PASSWORD',
           'ДА',
           'Длинный случайный пароль локальной базы данных. Его используют только два контейнера на вашем '
           'сервере.'),
          ('SELF_URL',
           'ДА',
           'Как другие узлы находят ВАШ: http://ВАШ-ПУБЛИЧНЫЙ-IP:8080 (или https://ваш-домен, если впереди '
           'прокси). Без этого узел лишь наблюдает за цепочкой и никогда не регистрируется как валидатор.'),
          ('NODE_OPERATOR_WALLET',
           'ДА',
           'Адрес вашего кошелька — ДОЛЖЕН быть зарегистрированным человеком (регистрация в приложении '
           'завершена). Именно это привязывает валидатор к человеку. Получает вознаграждения валидатора.'),
          ('RELAYER_PRIVATE_KEY',
           'Рекомендуется',
           'Ключ, которым подписываются ваши блоки (0x…, 66 символов). Проще всего: приватный ключ '
           'NODE_OPERATOR_WALLET — тогда дополнительная привязка не нужна. Если оставить пустым, узел '
           'создаст ключ при первом запуске и напечатает его ОДИН раз («SAVE THIS AS …»); внесите его в .env '
           'и перезапустите, иначе личность узла меняется при каждом перезапуске.'),
          ('NODE_OPERATOR_BINDING_SIGNATURE',
           'Только если ключи различаются',
           'Если RELAYER_PRIVATE_KEY — не ключ NODE_OPERATOR_WALLET: подпишите сообщение со страницы '
           'aequitas.digital/node-binding своим кошельком и вставьте подпись сюда.'),
          ('NODE_KEY',
           'Рекомендуется',
           'P2P-идентичность. Создаётся и печатается при первом запуске, если пусто — сохраните в .env по '
           'той же причине.'),
          ('PRIMARY_NODE_URLS',
           'Предустановлено',
           'Узлы, у которых ваш регистрируется и откуда при первом запуске берёт снимок состояния. '
           'Предустановлены два узла-основателя; оставьте как есть.'),
          ('GOMEMLIMIT',
           'Предустановлено',
           'Предел памяти процесса узла, предустановлено 5GiB (для 12 ГБ ОЗУ). При 8 ГБ укажите 3GiB.'),
          ('POSTGRES_SHARED_BUFFERS',
           'Предустановлено',
           'Кэш Postgres, предустановлено 1GB. Четверть вашей ОЗУ — хорошее значение.')],
 'railway_title': 'Шаг 2 — Запуск узла (Docker Compose)',
 'railway_intro': 'Всё, что делают два узла-основателя, в одном файле. Первая команда собирает узел из '
                  'исходников (около 10 минут), запускает Postgres и узел и автоматически перезапускает оба '
                  'после сбоя или перезагрузки сервера.',
 'railway_steps': ['На сервере: <font name="Courier">git clone '
                   'https://github.com/hanoi96international-gif/Aequitas.git</font>',
                   '<font name="Courier">cd Aequitas/deploy/validator</font> и <font name="Courier">cp '
                   '.env.example .env</font>',
                   'Отредактируйте <font name="Courier">.env</font> (например, <font name="Courier">nano '
                   '.env</font>): заполните POSTGRES_PASSWORD, SELF_URL и NODE_OPERATOR_WALLET — см. таблицу '
                   'выше',
                   '<font name="Courier">docker compose up -d --build</font> — собирает и запускает. В '
                   'первый раз около 10 минут',
                   'Смотрите журнал: <font name="Courier">docker compose logs -f node</font>. При первом '
                   'запуске узел импортирует состояние сети (снимок) с узла-основателя, проверяет его '
                   'подпись, затем подтягивает последующие блоки: <font name="Courier" '
                   'color="#5B21B6">[BOOTSTRAP] Fresh node — importing state from …</font>, затем <font '
                   'name="Courier" color="#0F766E">[HTTP-SYNC] Added … new blocks</font>',
                   'Если в журнале появилось <font name="Courier">SAVE THIS AS NODE_KEY</font> или <font '
                   'name="Courier">SET THIS AS RELAYER_PRIVATE_KEY</font>: скопируйте эти значения в .env '
                   'сейчас и снова выполните <font name="Courier">docker compose up -d</font> — иначе узел '
                   'получает новую личность при каждом перезапуске',
                   'Когда узел догонит сеть, появятся строки <font name="Courier">[Block #…]</font> — это '
                   'блоки, произведённые ВАШИМ узлом. До этого узел намеренно ничего не производит (<font '
                   'name="Courier">Frischer Knoten: … produziert nichts, bis er aufgeholt hat</font>) — '
                   'узел, никогда не видевший цепочку, не должен её выдумывать'],
 'railway_vars_code': 'POSTGRES_PASSWORD      = длинный-случайный-пароль\n'
                      'SELF_URL               = http://ВАШ-ПУБЛИЧНЫЙ-IP:8080\n'
                      'NODE_OPERATOR_WALLET   = 0xВАШ_КОШЕЛЁК_ЧЕЛОВЕКА\n'
                      '# рекомендуется: ключ, подписывающий ваши блоки (или пусто при первом запуске)\n'
                      'RELAYER_PRIVATE_KEY    = 0xВАШ_ПРИВАТНЫЙ_КЛЮЧ\n'
                      '# предустановлено, оставьте как есть\n'
                      'PRIMARY_NODE_URLS      = http://173.249.37.118:8080,http://194.163.188.71:8080',
 'docker_title': 'Шаг 2b — Обновление, перезапуск, остановка',
 'docker_intro': 'Узел хранит состояние в двух томах Docker (база данных и журнал переводов). Обновление '
                 'пересобирает образ из свежего кода; состояние сохраняется. Никогда не перезапускайте все '
                 'валидаторы сети одновременно — по одному.',
 'docker_code': '# Обновить до свежего кода (около 10 минут)\n'
                'cd Aequitas && git pull && cd deploy/validator && docker compose up -d --build\n'
                '\n'
                '# Перезапустить только узел\n'
                'docker compose restart node\n'
                '\n'
                '# Остановить всё (состояние сохраняется)\n'
                'docker compose down\n'
                '\n'
                '# Следить за ресурсами\n'
                'docker stats',
 'verify_title': 'Шаг 3 — Проверка работы узла',
 'verify_body': 'Сравните свой узел с сетью. Замените ВАШ-ПУБЛИЧНЫЙ-IP адресом сервера.',
 'verify_code': 'curl -s http://ВАШ-ПУБЛИЧНЫЙ-IP:8080/api/status | grep -oE \'"height":[0-9]+\'\n'
                'curl -s https://aequitas.digital/api/status  | grep -oE \'"height":[0-9]+\'\n'
                ' → Оба числа должны совпадать с точностью до нескольких блоков.\n'
                '\n'
                'http://ВАШ-ПУБЛИЧНЫЙ-IP:8080/           → ваш собственный обозреватель\n'
                'http://ВАШ-ПУБЛИЧНЫЙ-IP:8080/api/health/combined → всё, что узел измеряет о себе',
 'verify_note': 'Сразу после первого запуска высота сильно ниже сети, пока импортируется снимок и '
                'подтягиваются свежие блоки. Если отставание держится дольше 15 минут, ищите в журнале '
                'ошибки [BOOTSTRAP] и проверьте, открыты ли порты 8080 и 4001.',
 'valkey_title': 'Шаг 3b — Привязка кошелька к ключу узла (только если они различаются)',
 'valkey_body': 'Если RELAYER_PRIVATE_KEY — не приватный ключ NODE_OPERATOR_WALLET, один раз докажите, что '
                'оба ваши: откройте страницу ниже, подпишите показанное сообщение кошельком и внесите '
                'подпись в .env как NODE_OPERATOR_BINDING_SIGNATURE.',
 'valkey_code': 'https://aequitas.digital/node-binding',
 'valkey_note': 'При простой схеме с одним ключом (RELAYER_PRIVATE_KEY = ключ вашего кошелька) этот шаг не '
                'нужен.',
 'mm_title': 'Шаг 4 — Подключение MetaMask к узлу (необязательно)',
 'mm_body': 'В MetaMask: меню сетей → Добавить сеть → Добавить сеть вручную, затем введите:',
 'mm_rows': [('Название сети', 'Aequitas Chain'),
             ('URL RPC', 'http://ВАШ-ПУБЛИЧНЫЙ-IP:8080/rpc'),
             ('ID цепочки', '1926'),
             ('Символ', 'AEQ'),
             ('Десятичные', '18'),
             ('Обозреватель блоков', 'https://aequitas.digital')],
 'rewards_title': 'Шаг 5 — Вознаграждения валидатора',
 'rewards_box': 'Пул валидаторов собирает 40 % всех комиссий протокола (комиссии обмена, демерредж, '
                'превышение потолка богатства). Ежедневно в 20:00 по берлинскому времени пул распределяется '
                'между зарегистрированными валидаторами пропорционально произведённым блокам. Делать ничего '
                'не нужно — только держать узел включённым.',
 'rewards_steps': ['NODE_OPERATOR_WALLET должен быть зарегистрированным человеком — иначе сеть отклонит '
                   'регистрацию (журнал: <font name="Courier">NODE_OPERATOR_WALLET is not a registered '
                   'human</font>).',
                   'Проверьте в журнале: <font name="Courier" color="#0F766E">[PEERS] Auto-authorized '
                   'validator … (wallet: 0x…)</font> на узле-основателе и строки <font name="Courier">[Block '
                   '#…]</font> на вашем.',
                   'Выключенный узел не производит блоки и ничего не зарабатывает в это время — перезапуски '
                   'безопасны, узел догоняет сам.',
                   'Чего валидатор НЕ делает без дополнительного ПО: не принимает новые регистрации людей '
                   '(эти конечные точки отвечают 503). Переводы, блоки и вознаграждения работают без этого.'],
 'trouble_title': 'Устранение неполадок',
 'trouble_cols': ['Симптом', 'Вероятная причина', 'Решение'],
 'trouble_rows': [('NODE_OPERATOR_WALLET is not a registered human',
                   'Кошелёк не завершил регистрацию в приложении',
                   'Сначала зарегистрируйтесь в приложении, затем перезапустите узел.'),
                  ('operator_binding_signature missing or invalid',
                   'Ключ подписи и кошелёк различаются, привязки нет',
                   'Шаг 3b: подпишите на /node-binding и задайте NODE_OPERATOR_BINDING_SIGNATURE.'),
                  ('Frischer Knoten: … produziert nichts, bis er aufgeholt hat',
                   'Нормально, пока узел догоняет',
                   'Подождите. Производство начнётся после чистых циклов синхронизации с '
                   'узлами-основателями.'),
                  ('SELF_URL not set — … Beobachter',
                   'В .env нет SELF_URL',
                   'Задайте SELF_URL = http://ВАШ-ПУБЛИЧНЫЙ-IP:8080 и выполните docker compose up -d.'),
                  ('Высота остаётся далеко ниже сети',
                   'Импорт снимка не удался или порты закрыты',
                   'Ищите строки [BOOTSTRAP] в журнале; откройте входящие TCP 8080 и 4001 в брандмауэре / '
                   'группе безопасности.'),
                  ('Узел перезапускается по кругу, «OOMKilled»',
                   'Недостаточно ОЗУ',
                   'Уменьшите GOMEMLIMIT в .env (3GiB при 8 ГБ ОЗУ) или дайте серверу больше памяти.'),
                  ('docker compose: сборка не удаётся',
                   'Нет исходящего интернета во время сборки',
                   'Сборка загружает модули Go; проверьте DNS и исходящий HTTPS на сервере.')],
 'footer': 'Aequitas Chain · Chain ID 1926 · aequitas.digital · Вознаграждения валидатора: ежедневно в 20:00 '
           'по берлинскому времени'}

ZH = {'title': 'AEQUITAS 节点运营者指南',
 'version': 'v2.0 · 2026-09 · aequitas.digital',
 'tagline': '在自己的服务器上运行验证节点 · Docker Compose · 约 15 分钟，其中 10 分钟用于编译',
 'prereq_title': '开始之前 — 你需要什么',
 'prereqs': [('1.',
              '<b>你是已注册的真人：</b>安装 Aequitas 应用，完成生物特征注册，并记下你的钱包地址。钱包不是已注册真人的验证节点会被网络拒绝 — '
              '一个人，一个验证节点。租用服务器买不到投票权。'),
             ('2.',
              '<b>一台有公网 IPv4 的服务器 (VPS)：</b>Ubuntu 22.04 或 24.04，至少 4 vCPU、8 GB 内存、60 GB SSD（数据库目前约 18 GB '
              '且持续增长）。两个创始节点使用 6 vCPU / 12 GB / 100 GB。端口 8080 (API) 和 4001 (P2P) 必须能从互联网访问。'),
             ('3.',
              '<b>带 Compose 插件的 Docker，以及 git。</b>在 Ubuntu 上，<font name="Courier">curl -fsSL '
              'https://get.docker.com | sh</font> 会同时安装两者。'),
             ('4.', '<b>约 15 分钟。</b>其中 10 分钟是首次编译（节点在你的服务器上从源码编译）。之后的更新耗时相同。')],
 'vars_title': '第 1 步 — 配置 (.env)',
 'vars_warn': '安全警告：RELAYER_PRIVATE_KEY 和 NODE_KEY 是机密。谁拥有它们，谁就是你的节点。绝不要粘贴到聊天、邮件或工单中。.env 文件只留在你的服务器上。',
 'var_cols': ['变量', '必填？', '填写内容'],
 'vars': [('POSTGRES_PASSWORD', '是', '本地数据库的长随机密码。只有你服务器上的两个容器会使用它。'),
          ('SELF_URL',
           '是',
           '其他节点如何访问你的节点：http://你的公网IP:8080（若前面有代理则为 https://你的域名）。没有它，节点只会作为观察者跟随链，永远不会注册为验证节点。'),
          ('NODE_OPERATOR_WALLET', '是', '你自己的钱包地址 — 必须是已注册的真人（已完成应用内注册）。这把验证节点绑定到一个人。接收验证节点奖励。'),
          ('RELAYER_PRIVATE_KEY',
           '建议',
           '为你的区块签名的密钥（0x…，66 个字符）。最简单：使用 NODE_OPERATOR_WALLET 的私钥 — 这样无需额外绑定。留空时，节点会在首次启动时生成一个并只打印一次（"SAVE '
           'THIS AS …"）；把它写入 .env 并重启，否则每次重启身份都会改变。'),
          ('NODE_OPERATOR_BINDING_SIGNATURE',
           '仅当密钥不同时',
           '若 RELAYER_PRIVATE_KEY 不是 NODE_OPERATOR_WALLET 的密钥：用你的钱包在 aequitas.digital/node-binding '
           '签署显示的消息，并把签名粘贴到这里。'),
          ('NODE_KEY', '建议', 'P2P 身份。留空时在首次启动生成并打印 — 出于同样原因保存到 .env。'),
          ('PRIMARY_NODE_URLS', '预设', '你的节点向其注册、并在首次启动时获取状态快照的节点。预设为两个创始节点；保持不变。'),
          ('GOMEMLIMIT', '预设', '节点进程的内存上限，预设 5GiB（对应 12 GB 内存）。8 GB 内存请填 3GiB。'),
          ('POSTGRES_SHARED_BUFFERS', '预设', 'Postgres 缓存，预设 1GB。内存的四分之一是合适的值。')],
 'railway_title': '第 2 步 — 启动节点 (Docker Compose)',
 'railway_intro': '两个创始节点所做的一切，写在一个文件里。第一条命令从源码编译节点（约 10 分钟），启动 Postgres 和节点，并在崩溃或服务器重启后自动重启两者。',
 'railway_steps': ['在服务器上：<font name="Courier">git clone '
                   'https://github.com/hanoi96international-gif/Aequitas.git</font>',
                   '<font name="Courier">cd Aequitas/deploy/validator</font> 然后 <font name="Courier">cp '
                   '.env.example .env</font>',
                   '编辑 <font name="Courier">.env</font>（例如用 <font name="Courier">nano .env</font>）：填写 '
                   'POSTGRES_PASSWORD、SELF_URL 和 NODE_OPERATOR_WALLET — 见上表',
                   '<font name="Courier">docker compose up -d --build</font> — 编译并启动。首次约 10 分钟',
                   '查看日志：<font name="Courier">docker compose logs -f '
                   'node</font>。首次启动时，节点从创始节点导入网络状态（快照），校验其签名，然后拉取之后的区块：<font name="Courier" '
                   'color="#5B21B6">[BOOTSTRAP] Fresh node — importing state from …</font> 随后 <font '
                   'name="Courier" color="#0F766E">[HTTP-SYNC] Added … new blocks</font>',
                   '若日志打印了 <font name="Courier">SAVE THIS AS NODE_KEY</font> 或 <font name="Courier">SET THIS '
                   'AS RELAYER_PRIVATE_KEY</font>：立即把这些值复制到 .env 并再次运行 <font name="Courier">docker compose '
                   'up -d</font> — 否则每次重启节点都会获得新身份',
                   '追上进度后会看到 <font name="Courier">[Block #…]</font> 行 — 这是你的节点产出的区块。在此之前节点刻意不产出任何区块（<font '
                   'name="Courier">Frischer Knoten: … produziert nichts, bis er aufgeholt hat</font>）— '
                   '从未见过链的节点不得凭空造链'],
 'railway_vars_code': 'POSTGRES_PASSWORD      = 一个长随机密码\n'
                      'SELF_URL               = http://你的公网IP:8080\n'
                      'NODE_OPERATOR_WALLET   = 0x你的真人钱包\n'
                      '# 建议：为你的区块签名的密钥（首次启动也可留空）\n'
                      'RELAYER_PRIVATE_KEY    = 0x你的私钥\n'
                      '# 预设，保持不变\n'
                      'PRIMARY_NODE_URLS      = http://173.249.37.118:8080,http://194.163.188.71:8080',
 'docker_title': '第 2b 步 — 更新、重启、停止',
 'docker_intro': '节点把状态保存在两个 Docker 卷中（数据库和转账日志）。更新会用最新代码重建镜像；状态保留。绝不要同时重启网络中所有验证节点 — 逐个进行。',
 'docker_code': '# 更新到最新代码（约 10 分钟）\n'
                'cd Aequitas && git pull && cd deploy/validator && docker compose up -d --build\n'
                '\n'
                '# 仅重启节点\n'
                'docker compose restart node\n'
                '\n'
                '# 停止全部（状态保留）\n'
                'docker compose down\n'
                '\n'
                '# 查看资源占用\n'
                'docker stats',
 'verify_title': '第 3 步 — 确认节点正在运行',
 'verify_body': '把你的节点与网络比较。将"你的公网IP"替换为服务器地址。',
 'verify_code': 'curl -s http://你的公网IP:8080/api/status | grep -oE \'"height":[0-9]+\'\n'
                'curl -s https://aequitas.digital/api/status  | grep -oE \'"height":[0-9]+\'\n'
                ' → 两个数字必须相等，最多相差几个区块。\n'
                '\n'
                'http://你的公网IP:8080/           → 你自己的浏览器\n'
                'http://你的公网IP:8080/api/health/combined → 节点对自身测量的一切',
 'verify_note': '首次启动后，在导入快照和拉取最新区块期间，高度会远低于网络。若落后超过 15 分钟仍未追上，请在日志中查找 [BOOTSTRAP] 错误，并确认端口 8080 和 4001 已开放。',
 'valkey_title': '第 3b 步 — 将钱包绑定到节点密钥（仅当两者不同时）',
 'valkey_body': '若 RELAYER_PRIVATE_KEY 不是 NODE_OPERATOR_WALLET 的私钥，需一次性证明两者都属于你：打开下面页面，用钱包签署显示的消息，并把签名作为 '
                'NODE_OPERATOR_BINDING_SIGNATURE 写入 .env。',
 'valkey_code': 'https://aequitas.digital/node-binding',
 'valkey_note': '采用简单的单密钥配置（RELAYER_PRIVATE_KEY = 钱包密钥）时无需此步骤。',
 'mm_title': '第 4 步 — 把 MetaMask 连接到你的节点（可选）',
 'mm_body': '在 MetaMask 中：网络菜单 → 添加网络 → 手动添加网络，然后填写：',
 'mm_rows': [('网络名称', 'Aequitas Chain'),
             ('RPC URL', 'http://你的公网IP:8080/rpc'),
             ('链 ID', '1926'),
             ('货币符号', 'AEQ'),
             ('小数位', '18'),
             ('区块浏览器', 'https://aequitas.digital')],
 'rewards_title': '第 5 步 — 验证节点奖励',
 'rewards_box': '验证节点池收取全部协议费用的 40%（兑换费、滞留费、财富上限溢出）。每天柏林时间 20:00，池按各已注册验证节点产出的区块比例分配。除了保持节点运行，无需做任何事。',
 'rewards_steps': ['NODE_OPERATOR_WALLET 必须是已注册真人 — 否则网络拒绝注册（日志：<font name="Courier">NODE_OPERATOR_WALLET is '
                   'not a registered human</font>）。',
                   '在日志中确认：创始节点上出现 <font name="Courier" color="#0F766E">[PEERS] Auto-authorized validator … '
                   '(wallet: 0x…)</font>，你的节点上出现 <font name="Courier">[Block #…]</font> 行。',
                   '离线的节点不产出区块，因此那段时间没有收益 — 重启无害，节点会自行追上。',
                   '验证节点在没有额外软件时不会做的事：接受新的真人注册（这些接口返回 503）。转账、区块和奖励不依赖它。'],
 'trouble_title': '故障排除',
 'trouble_cols': ['现象', '可能原因', '解决办法'],
 'trouble_rows': [('NODE_OPERATOR_WALLET is not a registered human', '钱包未完成应用内注册', '先在应用中注册，再重启节点。'),
                  ('operator_binding_signature missing or invalid',
                   '签名密钥与钱包不同且未绑定',
                   '第 3b 步：在 /node-binding 签名并设置 NODE_OPERATOR_BINDING_SIGNATURE。'),
                  ('Frischer Knoten: … produziert nichts, bis er aufgeholt hat',
                   '追赶期间的正常现象',
                   '等待。节点与创始节点完成干净的同步周期后才开始产出。'),
                  ('SELF_URL not set — … Beobachter',
                   '.env 中缺少 SELF_URL',
                   '将 SELF_URL 设为 http://你的公网IP:8080 并运行 docker compose up -d。'),
                  ('高度远低于网络', '快照导入失败，或端口未开放', '在日志中查找 [BOOTSTRAP] 行；在防火墙/安全组中开放入站 TCP 8080 和 4001。'),
                  ('节点反复重启，"OOMKilled"', '内存不足', '降低 .env 中的 GOMEMLIMIT（8 GB 内存时 3GiB）或给服务器增加内存。'),
                  ('docker compose：编译失败', '编译期间没有出站网络', '编译需要下载 Go 模块；检查服务器的 DNS 和出站 HTTPS。')],
 'footer': 'Aequitas Chain · Chain ID 1926 · aequitas.digital · 验证节点奖励：每天柏林时间 20:00'}

AR = {'title': 'دليل مشغّل عقدة AEQUITAS',
 'version': 'v2.0 · 2026-09 · aequitas.digital',
 'tagline': 'شغّل مدقّقًا على خادمك الخاص · Docker Compose · نحو 15 دقيقة، منها 10 للبناء',
 'prereq_title': 'قبل أن تبدأ — ما تحتاجه',
 'prereqs': [('1.',
              '<b>أنت إنسان مسجَّل:</b> ثبّت تطبيق Aequitas، وأكمل التسجيل البيومتري، ودوّن عنوان محفظتك. '
              'ترفض الشبكة أي مدقّق ليست محفظته لإنسان مسجَّل — إنسان واحد، مدقّق واحد. استئجار الخوادم لا '
              'يشتري أصواتًا.'),
             ('2.',
              '<b>خادم (VPS) بعنوان IPv4 عام:</b> Ubuntu 22.04 أو 24.04، على الأقل 4 vCPU و8 GB ذاكرة و60 GB '
              'SSD (قاعدة البيانات اليوم نحو 18 GB وتكبر). تعمل عقدتا المؤسسين على 6 vCPU / 12 GB / 100 GB. '
              'يجب أن يكون المنفذان 8080 (API) و4001 (P2P) متاحين من الإنترنت.'),
             ('3.',
              '<b>Docker مع إضافة Compose، وgit.</b> على Ubuntu يثبّت <font name="Courier">curl -fsSL '
              'https://get.docker.com | sh</font> كليهما.'),
             ('4.',
              '<b>نحو 15 دقيقة.</b> عشر منها هي البناء الأول (تُبنى العقدة من الشيفرة المصدرية على خادمك). '
              'التحديثات تستغرق المدة نفسها.')],
 'vars_title': 'الخطوة 1 — الإعداد (.env)',
 'vars_warn': 'تحذير أمني: RELAYER_PRIVATE_KEY وNODE_KEY أسرار. من يملكهما هو عقدتك. لا تلصقهما أبدًا في '
              'محادثة أو بريد أو تذكرة. يبقى ملف .env على خادمك.',
 'var_cols': ['المتغيّر', 'إلزامي؟', 'ما تضعه'],
 'vars': [('POSTGRES_PASSWORD',
           'نعم',
           'كلمة مرور طويلة وعشوائية لقاعدة البيانات المحلية. لا يستخدمها سوى الحاويتين على خادمك.'),
          ('SELF_URL',
           'نعم',
           'كيف تصل العقد الأخرى إلى عقدتك: http://عنوانك-العام:8080 (أو https://نطاقك إذا كان أمامها وكيل). '
           'بدونه تتابع العقدة السلسلة كمراقب فقط ولا تسجّل نفسها مدقّقًا أبدًا.'),
          ('NODE_OPERATOR_WALLET',
           'نعم',
           'عنوان محفظتك — يجب أن يكون لإنسان مسجَّل (اكتمل التسجيل في التطبيق). هذا ما يربط المدقّق بشخص. '
           'تستقبل مكافآت المدقّق.'),
          ('RELAYER_PRIVATE_KEY',
           'موصى به',
           'المفتاح الذي يوقّع كتلك (0x…، 66 حرفًا). الأبسط: المفتاح الخاص لـ NODE_OPERATOR_WALLET — عندها '
           'لا حاجة لأي ربط إضافي. إن تُرك فارغًا تولّد العقدة مفتاحًا عند أول تشغيل وتطبعه مرة واحدة ("SAVE '
           'THIS AS …")؛ ضعه في .env وأعد التشغيل، وإلا تتغيّر هويتك مع كل إعادة تشغيل.'),
          ('NODE_OPERATOR_BINDING_SIGNATURE',
           'فقط إذا اختلف المفتاحان',
           'إذا لم يكن RELAYER_PRIVATE_KEY مفتاح NODE_OPERATOR_WALLET: وقّع الرسالة المعروضة في '
           'aequitas.digital/node-binding بمحفظتك والصق التوقيع هنا.'),
          ('NODE_KEY',
           'موصى به',
           'هوية P2P. تُولَّد وتُطبع عند أول تشغيل إن كانت فارغة — احفظها في .env للسبب نفسه.'),
          ('PRIMARY_NODE_URLS',
           'مُعدّ مسبقًا',
           'العقد التي تسجّل عقدتك نفسها لديها وتجلب منها لقطة الحالة عند أول تشغيل. مُعدّة مسبقًا على عقدتي '
           'المؤسسين؛ اتركها كما هي.'),
          ('GOMEMLIMIT',
           'مُعدّ مسبقًا',
           'حد ذاكرة عملية العقدة، مُعدّ مسبقًا 5GiB (لذاكرة 12 GB). مع 8 GB ضع 3GiB.'),
          ('POSTGRES_SHARED_BUFFERS',
           'مُعدّ مسبقًا',
           'ذاكرة Postgres المؤقتة، مُعدّة مسبقًا 1GB. ربع ذاكرتك قيمة جيدة.')],
 'railway_title': 'الخطوة 2 — تشغيل العقدة (Docker Compose)',
 'railway_intro': 'كل ما تفعله عقدتا المؤسسين، في ملف واحد. يبني الأمر الأول العقدة من المصدر (نحو 10 '
                  'دقائق)، ويشغّل Postgres والعقدة، ويعيد تشغيلهما تلقائيًا بعد أي انهيار أو إعادة تشغيل '
                  'للخادم.',
 'railway_steps': ['على خادمك: <font name="Courier">git clone '
                   'https://github.com/hanoi96international-gif/Aequitas.git</font>',
                   '<font name="Courier">cd Aequitas/deploy/validator</font> ثم <font name="Courier">cp '
                   '.env.example .env</font>',
                   'حرّر <font name="Courier">.env</font> (مثلًا بـ <font name="Courier">nano .env</font>): '
                   'املأ POSTGRES_PASSWORD وSELF_URL وNODE_OPERATOR_WALLET — انظر الجدول أعلاه',
                   '<font name="Courier">docker compose up -d --build</font> — يبني ويشغّل. نحو 10 دقائق في '
                   'المرة الأولى',
                   'تابع السجل: <font name="Courier">docker compose logs -f node</font>. عند أول تشغيل '
                   'تستورد العقدة حالة الشبكة (اللقطة) من عقدة مؤسِّسة، وتتحقق من توقيعها، ثم تجلب الكتل '
                   'اللاحقة: <font name="Courier" color="#5B21B6">[BOOTSTRAP] Fresh node — importing state '
                   'from …</font> ثم <font name="Courier" color="#0F766E">[HTTP-SYNC] Added … new '
                   'blocks</font>',
                   'إذا طبع السجل <font name="Courier">SAVE THIS AS NODE_KEY</font> أو <font '
                   'name="Courier">SET THIS AS RELAYER_PRIVATE_KEY</font>: انسخ هذه القيم إلى .env الآن '
                   'وشغّل <font name="Courier">docker compose up -d</font> مجددًا — وإلا تحصل عقدتك على هوية '
                   'جديدة مع كل إعادة تشغيل',
                   'بعد اللحاق بالشبكة سترى أسطر <font name="Courier">[Block #…]</font> — هذه كتل أنتجتها '
                   'عقدتك. حتى ذلك الحين لا تنتج العقدة شيئًا عمدًا (<font name="Courier">Frischer Knoten: … '
                   'produziert nichts, bis er aufgeholt hat</font>) — عقدة لم ترَ السلسلة قط لا يجوز أن '
                   'تخترع واحدة'],
 'railway_vars_code': 'POSTGRES_PASSWORD      = كلمة-مرور-طويلة-عشوائية\n'
                      'SELF_URL               = http://عنوانك-العام:8080\n'
                      'NODE_OPERATOR_WALLET   = 0xمحفظتك_البشرية\n'
                      '# موصى به: المفتاح الذي يوقّع كتلك (أو فارغًا عند أول تشغيل)\n'
                      'RELAYER_PRIVATE_KEY    = 0xمفتاحك_الخاص\n'
                      '# مُعدّ مسبقًا، اتركه كما هو\n'
                      'PRIMARY_NODE_URLS      = http://173.249.37.118:8080,http://194.163.188.71:8080',
 'docker_title': 'الخطوة 2b — التحديث وإعادة التشغيل والإيقاف',
 'docker_intro': 'تحفظ العقدة حالتها في مجلّدي Docker (قاعدة البيانات وسجل التحويلات). التحديث يعيد بناء '
                 'الصورة من أحدث شيفرة؛ الحالة تبقى. لا تعِد أبدًا تشغيل جميع مدقّقي الشبكة في وقت واحد — '
                 'واحدًا تلو الآخر.',
 'docker_code': '# التحديث إلى أحدث شيفرة (نحو 10 دقائق)\n'
                'cd Aequitas && git pull && cd deploy/validator && docker compose up -d --build\n'
                '\n'
                '# إعادة تشغيل العقدة فقط\n'
                'docker compose restart node\n'
                '\n'
                '# إيقاف كل شيء (تبقى الحالة)\n'
                'docker compose down\n'
                '\n'
                '# مراقبة استهلاك الموارد\n'
                'docker stats',
 'verify_title': 'الخطوة 3 — التحقق من عمل عقدتك',
 'verify_body': 'قارن عقدتك بالشبكة. استبدل "عنوانك-العام" بعنوان خادمك.',
 'verify_code': 'curl -s http://عنوانك-العام:8080/api/status | grep -oE \'"height":[0-9]+\'\n'
                'curl -s https://aequitas.digital/api/status  | grep -oE \'"height":[0-9]+\'\n'
                ' → يجب أن يتطابق الرقمان مع فارق بضع كتل على الأكثر.\n'
                '\n'
                'http://عنوانك-العام:8080/           → مستكشفك الخاص\n'
                'http://عنوانك-العام:8080/api/health/combined → كل ما تقيسه العقدة عن نفسها',
 'verify_note': 'بعد أول تشغيل مباشرة يكون الارتفاع أدنى بكثير من الشبكة أثناء استيراد اللقطة وجلب الكتل '
                'الحديثة. إذا بقي متأخرًا كثيرًا أكثر من 15 دقيقة فابحث في السجل عن أخطاء [BOOTSTRAP] وتأكد '
                'من فتح المنفذين 8080 و4001.',
 'valkey_title': 'الخطوة 3b — ربط محفظتك بمفتاح العقدة (فقط إذا اختلفا)',
 'valkey_body': 'إذا لم يكن RELAYER_PRIVATE_KEY المفتاح الخاص لـ NODE_OPERATOR_WALLET فأثبت مرة واحدة أن '
                'كليهما لك: افتح الصفحة أدناه، ووقّع الرسالة المعروضة بمحفظتك، وضع التوقيع في .env باسم '
                'NODE_OPERATOR_BINDING_SIGNATURE.',
 'valkey_code': 'https://aequitas.digital/node-binding',
 'valkey_note': 'مع الإعداد البسيط بمفتاح واحد (RELAYER_PRIVATE_KEY = مفتاح محفظتك) لا حاجة لهذه الخطوة.',
 'mm_title': 'الخطوة 4 — ربط MetaMask بعقدتك (اختياري)',
 'mm_body': 'في MetaMask: قائمة الشبكات ← إضافة شبكة ← إضافة شبكة يدويًا، ثم أدخل:',
 'mm_rows': [('اسم الشبكة', 'Aequitas Chain'),
             ('رابط RPC', 'http://عنوانك-العام:8080/rpc'),
             ('معرّف السلسلة', '1926'),
             ('الرمز', 'AEQ'),
             ('الخانات العشرية', '18'),
             ('مستكشف الكتل', 'https://aequitas.digital')],
 'rewards_title': 'الخطوة 5 — مكافآت المدقّق',
 'rewards_box': 'يجمع صندوق المدقّقين 40% من جميع رسوم البروتوكول (رسوم المبادلة، رسم الاحتفاظ، فائض سقف '
                'الثروة). كل يوم في الساعة 20:00 بتوقيت برلين يُوزَّع الصندوق على المدقّقين المسجَّلين بنسبة '
                'الكتل التي أنتجوها. لا شيء يلزم فعله سوى إبقاء العقدة تعمل.',
 'rewards_steps': ['يجب أن يكون NODE_OPERATOR_WALLET إنسانًا مسجَّلًا — وإلا ترفض الشبكة التسجيل (السجل: '
                   '<font name="Courier">NODE_OPERATOR_WALLET is not a registered human</font>).',
                   'تأكد في السجل: <font name="Courier" color="#0F766E">[PEERS] Auto-authorized validator … '
                   '(wallet: 0x…)</font> على عقدة مؤسِّسة، وأسطر <font name="Courier">[Block #…]</font> على '
                   'عقدتك.',
                   'العقدة المتوقفة لا تنتج كتلًا ولا تكسب شيئًا في تلك المدة — إعادة التشغيل غير ضارة، '
                   'والعقدة تلحق بنفسها.',
                   'ما لا يفعله المدقّق دون برمجيات إضافية: قبول تسجيلات بشر جدد (تلك النقاط تجيب بـ 503). '
                   'التحويلات والكتل والمكافآت تعمل بدونه.'],
 'trouble_title': 'استكشاف الأخطاء',
 'trouble_cols': ['العرَض', 'السبب المحتمل', 'الحل'],
 'trouble_rows': [('NODE_OPERATOR_WALLET is not a registered human',
                   'المحفظة لم تُكمل التسجيل في التطبيق',
                   'سجّل أولًا في التطبيق ثم أعد تشغيل العقدة.'),
                  ('operator_binding_signature missing or invalid',
                   'مفتاح التوقيع والمحفظة مختلفان دون ربط',
                   'الخطوة 3b: وقّع في /node-binding واضبط NODE_OPERATOR_BINDING_SIGNATURE.'),
                  ('Frischer Knoten: … produziert nichts, bis er aufgeholt hat',
                   'طبيعي أثناء اللحاق',
                   'انتظر. يبدأ الإنتاج بعد إكمال دورات مزامنة نظيفة مع عقد المؤسسين.'),
                  ('SELF_URL not set — … Beobachter',
                   'SELF_URL مفقود في .env',
                   'اضبط SELF_URL على http://عنوانك-العام:8080 وشغّل docker compose up -d.'),
                  ('الارتفاع يبقى أدنى بكثير من الشبكة',
                   'فشل استيراد اللقطة أو المنافذ مغلقة',
                   'ابحث عن أسطر [BOOTSTRAP] في السجل؛ افتح TCP 8080 و4001 للوارد في الجدار الناري / مجموعة '
                   'الأمان.'),
                  ('العقدة تعيد التشغيل بلا توقف، "OOMKilled"',
                   'ذاكرة غير كافية',
                   'خفّض GOMEMLIMIT في .env (\u200f3GiB مع 8 GB) أو أعطِ الخادم ذاكرة أكبر.'),
                  ('docker compose: فشل البناء',
                   'لا إنترنت صادر أثناء البناء',
                   'يحمّل البناء وحدات Go؛ تحقق من DNS وHTTPS الصادر على الخادم.')],
 'footer': 'Aequitas Chain · Chain ID 1926 · aequitas.digital · مكافآت المدقّق: يوميًا في 20:00 بتوقيت برلين'}

HI = {'title': 'AEQUITAS नोड ऑपरेटर गाइड',
 'version': 'v2.0 · 2026-09 · aequitas.digital',
 'tagline': 'अपने सर्वर पर वैलिडेटर चलाएँ · Docker Compose · लगभग 15 मिनट, जिनमें 10 बिल्ड के',
 'prereq_title': 'शुरू करने से पहले — आपको क्या चाहिए',
 'prereqs': [('1.',
              '<b>आप एक पंजीकृत मानव हैं:</b> Aequitas ऐप इंस्टॉल करें, बायोमेट्रिक पंजीकरण पूरा करें और '
              'अपना वॉलेट पता नोट करें। जिस वैलिडेटर का वॉलेट पंजीकृत मानव का नहीं है, नेटवर्क उसे अस्वीकार '
              'करता है — एक मानव, एक वैलिडेटर। सर्वर किराए पर लेने से वोट नहीं मिलते।'),
             ('2.',
              '<b>सार्वजनिक IPv4 वाला एक सर्वर (VPS):</b> Ubuntu 22.04 या 24.04, कम से कम 4 vCPU, 8 GB RAM, '
              '60 GB SSD (डेटाबेस अभी लगभग 18 GB है और बढ़ रहा है)। दोनों संस्थापक नोड 6 vCPU / 12 GB / 100 '
              'GB पर चलते हैं। पोर्ट 8080 (API) और 4001 (P2P) इंटरनेट से पहुँच योग्य होने चाहिए।'),
             ('3.',
              '<b>Compose प्लगइन के साथ Docker, और git।</b> Ubuntu पर <font name="Courier">curl -fsSL '
              'https://get.docker.com | sh</font> दोनों इंस्टॉल कर देता है।'),
             ('4.',
              '<b>लगभग 15 मिनट।</b> इनमें से दस पहला बिल्ड है (नोड आपके सर्वर पर स्रोत कोड से बनता है)। '
              'अपडेट में उतना ही समय लगता है।')],
 'vars_title': 'चरण 1 — कॉन्फ़िगरेशन (.env)',
 'vars_warn': 'सुरक्षा चेतावनी: RELAYER_PRIVATE_KEY और NODE_KEY गोपनीय हैं। जिसके पास ये हैं, वही आपका नोड '
              'है। इन्हें कभी चैट, ईमेल या टिकट में न चिपकाएँ। .env फ़ाइल आपके सर्वर पर ही रहती है।',
 'var_cols': ['वेरिएबल', 'अनिवार्य?', 'क्या भरें'],
 'vars': [('POSTGRES_PASSWORD',
           'हाँ',
           'स्थानीय डेटाबेस के लिए एक लंबा, यादृच्छिक पासवर्ड। इसे केवल आपके सर्वर के दो कंटेनर इस्तेमाल '
           'करते हैं।'),
          ('SELF_URL',
           'हाँ',
           'दूसरे नोड आपके नोड तक कैसे पहुँचें: http://आपका-सार्वजनिक-IP:8080 (या https://आपका-डोमेन, यदि '
           'आगे प्रॉक्सी है)। इसके बिना नोड केवल पर्यवेक्षक के रूप में चेन का अनुसरण करता है और कभी वैलिडेटर '
           'के रूप में पंजीकृत नहीं होता।'),
          ('NODE_OPERATOR_WALLET',
           'हाँ',
           'आपका अपना वॉलेट पता — यह पंजीकृत मानव होना चाहिए (ऐप में पंजीकरण पूरा)। यही वैलिडेटर को एक '
           'व्यक्ति से जोड़ता है। वैलिडेटर पुरस्कार प्राप्त करता है।'),
          ('RELAYER_PRIVATE_KEY',
           'अनुशंसित',
           'वह कुंजी जो आपके ब्लॉकों पर हस्ताक्षर करती है (0x…, 66 अक्षर)। सबसे सरल: NODE_OPERATOR_WALLET की '
           'निजी कुंजी — तब कोई अतिरिक्त बाइंडिंग नहीं चाहिए। खाली छोड़ने पर नोड पहली बार चलने पर एक कुंजी '
           'बनाता है और उसे केवल एक बार प्रिंट करता है ("SAVE THIS AS …"); उसे .env में डालें और पुनः आरंभ '
           'करें, वरना हर पुनः आरंभ पर आपकी पहचान बदल जाएगी।'),
          ('NODE_OPERATOR_BINDING_SIGNATURE',
           'केवल यदि कुंजियाँ अलग हों',
           'यदि RELAYER_PRIVATE_KEY, NODE_OPERATOR_WALLET की कुंजी नहीं है: aequitas.digital/node-binding पर '
           'दिखाए गए संदेश पर अपने वॉलेट से हस्ताक्षर करें और हस्ताक्षर यहाँ चिपकाएँ।'),
          ('NODE_KEY',
           'अनुशंसित',
           'P2P पहचान। खाली हो तो पहली बार चलने पर बनती और प्रिंट होती है — उसी कारण से .env में सहेजें।'),
          ('PRIMARY_NODE_URLS',
           'पूर्व-निर्धारित',
           'वे नोड जिनके पास आपका नोड पंजीकरण करता है और पहली बार चलने पर जहाँ से स्थिति का स्नैपशॉट लेता '
           'है। दोनों संस्थापक नोड पर पूर्व-निर्धारित; वैसा ही रहने दें।'),
          ('GOMEMLIMIT',
           'पूर्व-निर्धारित',
           'नोड प्रक्रिया की मेमोरी सीमा, पूर्व-निर्धारित 5GiB (12 GB RAM के लिए)। 8 GB RAM पर 3GiB रखें।'),
          ('POSTGRES_SHARED_BUFFERS',
           'पूर्व-निर्धारित',
           'Postgres कैश, पूर्व-निर्धारित 1GB। आपकी RAM का एक चौथाई अच्छा मान है।')],
 'railway_title': 'चरण 2 — नोड शुरू करें (Docker Compose)',
 'railway_intro': 'दोनों संस्थापक नोड जो कुछ करते हैं, एक फ़ाइल में। पहला कमांड नोड को स्रोत से बनाता है '
                  '(लगभग 10 मिनट), Postgres और नोड शुरू करता है, और क्रैश या सर्वर रीबूट के बाद दोनों को '
                  'स्वतः पुनः आरंभ करता है।',
 'railway_steps': ['अपने सर्वर पर: <font name="Courier">git clone '
                   'https://github.com/hanoi96international-gif/Aequitas.git</font>',
                   '<font name="Courier">cd Aequitas/deploy/validator</font> और <font name="Courier">cp '
                   '.env.example .env</font>',
                   '<font name="Courier">.env</font> संपादित करें (जैसे <font name="Courier">nano '
                   '.env</font> से): POSTGRES_PASSWORD, SELF_URL और NODE_OPERATOR_WALLET भरें — ऊपर की '
                   'तालिका देखें',
                   '<font name="Courier">docker compose up -d --build</font> — बनाता और शुरू करता है। पहली '
                   'बार लगभग 10 मिनट',
                   'लॉग देखें: <font name="Courier">docker compose logs -f node</font>। पहली बार चलने पर नोड '
                   'एक संस्थापक नोड से नेटवर्क की स्थिति (स्नैपशॉट) आयात करता है, उसके हस्ताक्षर की जाँच '
                   'करता है, फिर बाद के ब्लॉक खींचता है: <font name="Courier" color="#5B21B6">[BOOTSTRAP] '
                   'Fresh node — importing state from …</font> उसके बाद <font name="Courier" '
                   'color="#0F766E">[HTTP-SYNC] Added … new blocks</font>',
                   'यदि लॉग में <font name="Courier">SAVE THIS AS NODE_KEY</font> या <font '
                   'name="Courier">SET THIS AS RELAYER_PRIVATE_KEY</font> छपा हो: वे मान अभी .env में कॉपी '
                   'करें और <font name="Courier">docker compose up -d</font> फिर चलाएँ — वरना हर पुनः आरंभ '
                   'पर आपके नोड को नई पहचान मिलेगी',
                   'बराबरी पर पहुँचने के बाद आपको <font name="Courier">[Block #…]</font> पंक्तियाँ दिखेंगी — '
                   'ये आपके नोड के बनाए ब्लॉक हैं। तब तक नोड जानबूझकर कुछ नहीं बनाता (<font '
                   'name="Courier">Frischer Knoten: … produziert nichts, bis er aufgeholt hat</font>) — जिस '
                   'नोड ने चेन कभी देखी ही नहीं, उसे चेन गढ़नी नहीं चाहिए'],
 'railway_vars_code': 'POSTGRES_PASSWORD      = एक-लंबा-यादृच्छिक-पासवर्ड\n'
                      'SELF_URL               = http://आपका-सार्वजनिक-IP:8080\n'
                      'NODE_OPERATOR_WALLET   = 0xआपका_मानव_वॉलेट\n'
                      '# अनुशंसित: आपके ब्लॉकों पर हस्ताक्षर करने वाली कुंजी (या पहली बार खाली)\n'
                      'RELAYER_PRIVATE_KEY    = 0xआपकी_निजी_कुंजी\n'
                      '# पूर्व-निर्धारित, वैसा ही रहने दें\n'
                      'PRIMARY_NODE_URLS      = http://173.249.37.118:8080,http://194.163.188.71:8080',
 'docker_title': 'चरण 2b — अपडेट, पुनः आरंभ, बंद',
 'docker_intro': 'नोड अपनी स्थिति दो Docker वॉल्यूम में रखता है (डेटाबेस और ट्रांसफ़र लॉग)। अपडेट नवीनतम कोड '
                 'से इमेज फिर बनाता है; स्थिति बनी रहती है। नेटवर्क के सभी वैलिडेटर कभी एक साथ पुनः आरंभ न '
                 'करें — एक के बाद एक।',
 'docker_code': '# नवीनतम कोड पर अपडेट करें (लगभग 10 मिनट)\n'
                'cd Aequitas && git pull && cd deploy/validator && docker compose up -d --build\n'
                '\n'
                '# केवल नोड पुनः आरंभ करें\n'
                'docker compose restart node\n'
                '\n'
                '# सब बंद करें (स्थिति बनी रहती है)\n'
                'docker compose down\n'
                '\n'
                '# संसाधन उपयोग देखें\n'
                'docker stats',
 'verify_title': 'चरण 3 — जाँचें कि आपका नोड चल रहा है',
 'verify_body': 'अपने नोड की तुलना नेटवर्क से करें। "आपका-सार्वजनिक-IP" की जगह अपने सर्वर का पता लिखें।',
 'verify_code': 'curl -s http://आपका-सार्वजनिक-IP:8080/api/status | grep -oE \'"height":[0-9]+\'\n'
                'curl -s https://aequitas.digital/api/status  | grep -oE \'"height":[0-9]+\'\n'
                ' → दोनों संख्याएँ कुछ ब्लॉकों के अंतर तक बराबर होनी चाहिए।\n'
                '\n'
                'http://आपका-सार्वजनिक-IP:8080/           → आपका अपना एक्सप्लोरर\n'
                'http://आपका-सार्वजनिक-IP:8080/api/health/combined → नोड अपने बारे में जो कुछ मापता है',
 'verify_note': 'पहली बार चलने के तुरंत बाद, स्नैपशॉट आयात और हाल के ब्लॉक खींचे जाने तक ऊँचाई नेटवर्क से '
                'बहुत नीचे रहती है। यदि 15 मिनट से अधिक समय तक बहुत पीछे रहे, तो लॉग में [BOOTSTRAP] '
                'त्रुटियाँ देखें और जाँचें कि पोर्ट 8080 और 4001 खुले हैं।',
 'valkey_title': 'चरण 3b — वॉलेट को नोड कुंजी से जोड़ें (केवल यदि अलग हों)',
 'valkey_body': 'यदि RELAYER_PRIVATE_KEY, NODE_OPERATOR_WALLET की निजी कुंजी नहीं है, तो एक बार सिद्ध करें '
                'कि दोनों आपके हैं: नीचे का पेज खोलें, दिखाए गए संदेश पर अपने वॉलेट से हस्ताक्षर करें और '
                'हस्ताक्षर को NODE_OPERATOR_BINDING_SIGNATURE के रूप में .env में रखें।',
 'valkey_code': 'https://aequitas.digital/node-binding',
 'valkey_note': 'सरल एकल-कुंजी सेटअप (RELAYER_PRIVATE_KEY = आपके वॉलेट की कुंजी) में यह चरण आवश्यक नहीं है।',
 'mm_title': 'चरण 4 — MetaMask को अपने नोड से जोड़ें (वैकल्पिक)',
 'mm_body': 'MetaMask में: नेटवर्क मेनू → नेटवर्क जोड़ें → मैन्युअल रूप से नेटवर्क जोड़ें, फिर भरें:',
 'mm_rows': [('नेटवर्क का नाम', 'Aequitas Chain'),
             ('RPC URL', 'http://आपका-सार्वजनिक-IP:8080/rpc'),
             ('चेन ID', '1926'),
             ('प्रतीक', 'AEQ'),
             ('दशमलव', '18'),
             ('ब्लॉक एक्सप्लोरर', 'https://aequitas.digital')],
 'rewards_title': 'चरण 5 — वैलिडेटर पुरस्कार',
 'rewards_box': 'वैलिडेटर पूल सभी प्रोटोकॉल शुल्कों का 40% एकत्र करता है (स्वैप शुल्क, डिमरेज, धन-सीमा '
                'अधिशेष)। हर दिन बर्लिन समय 20:00 पर पूल पंजीकृत वैलिडेटरों में उनके बनाए ब्लॉकों के अनुपात '
                'में बाँटा जाता है। नोड चालू रखने के अलावा कुछ नहीं करना है।',
 'rewards_steps': ['NODE_OPERATOR_WALLET पंजीकृत मानव होना चाहिए — वरना नेटवर्क पंजीकरण अस्वीकार करता है '
                   '(लॉग: <font name="Courier">NODE_OPERATOR_WALLET is not a registered human</font>)।',
                   'लॉग में पुष्टि करें: किसी संस्थापक नोड पर <font name="Courier" color="#0F766E">[PEERS] '
                   'Auto-authorized validator … (wallet: 0x…)</font>, और आपके नोड पर <font '
                   'name="Courier">[Block #…]</font> पंक्तियाँ।',
                   'बंद नोड ब्लॉक नहीं बनाता, इसलिए उस समय कुछ नहीं कमाता — पुनः आरंभ हानिरहित है, नोड स्वयं '
                   'बराबरी कर लेता है।',
                   'अतिरिक्त सॉफ़्टवेयर के बिना वैलिडेटर जो नहीं करता: नए मानव पंजीकरण स्वीकार करना (वे '
                   'एंडपॉइंट 503 देते हैं)। ट्रांसफ़र, ब्लॉक और पुरस्कार इसके बिना काम करते हैं।'],
 'trouble_title': 'समस्या निवारण',
 'trouble_cols': ['लक्षण', 'संभावित कारण', 'समाधान'],
 'trouble_rows': [('NODE_OPERATOR_WALLET is not a registered human',
                   'वॉलेट ने ऐप में पंजीकरण पूरा नहीं किया',
                   'पहले ऐप में पंजीकरण करें, फिर नोड पुनः आरंभ करें।'),
                  ('operator_binding_signature missing or invalid',
                   'हस्ताक्षर कुंजी और वॉलेट अलग, कोई बाइंडिंग नहीं',
                   'चरण 3b: /node-binding पर हस्ताक्षर करें और NODE_OPERATOR_BINDING_SIGNATURE सेट करें।'),
                  ('Frischer Knoten: … produziert nichts, bis er aufgeholt hat',
                   'बराबरी करते समय सामान्य',
                   'प्रतीक्षा करें। संस्थापक नोडों के साथ स्वच्छ सिंक चक्र पूरे होने पर उत्पादन शुरू होता '
                   'है।'),
                  ('SELF_URL not set — … Beobachter',
                   '.env में SELF_URL नहीं है',
                   'SELF_URL को http://आपका-सार्वजनिक-IP:8080 पर सेट करें और docker compose up -d चलाएँ।'),
                  ('ऊँचाई नेटवर्क से बहुत नीचे रहती है',
                   'स्नैपशॉट आयात विफल, या पोर्ट बंद',
                   'लॉग में [BOOTSTRAP] पंक्तियाँ देखें; फ़ायरवॉल / सुरक्षा समूह में इनबाउंड TCP 8080 और '
                   '4001 खोलें।'),
                  ('नोड बार-बार पुनः आरंभ होता है, "OOMKilled"',
                   'RAM अपर्याप्त',
                   '.env में GOMEMLIMIT घटाएँ (8 GB RAM पर 3GiB) या सर्वर को अधिक मेमोरी दें।'),
                  ('docker compose: बिल्ड विफल',
                   'बिल्ड के दौरान आउटबाउंड इंटरनेट नहीं',
                   'बिल्ड Go मॉड्यूल डाउनलोड करता है; सर्वर पर DNS और आउटबाउंड HTTPS जाँचें।')],
 'footer': 'Aequitas Chain · Chain ID 1926 · aequitas.digital · वैलिडेटर पुरस्कार: प्रतिदिन बर्लिन समय 20:00'}

# ── GENERATE ──────────────────────────────────────────────────────────────────

# ── FONTS (Unicode coverage per script) ────────────────────────────────────────
# Base-14 Helvetica/Courier only cover WinAnsi (Latin-1) — not Turkish
# (ğ/ş/ı), Cyrillic, CJK, Arabic, or Devanagari. Registering a Unicode TTF
# under the SAME name ("Helvetica", "Courier", ...) makes every existing
# fontName='Helvetica' reference in this file (STYLES, var_table,
# trouble_table, box, etc.) pick it up automatically — no need to touch the
# rendering code itself, just swap which physical font backs those names
# before building each language's PDF.
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont

FONTS_DIR = 'C:/Windows/Fonts'

def register_latin_cyrillic():
    # Covers Latin Extended-A (Turkish ğşıİ) + Cyrillic (Russian) + Latin-1
    # (German/Spanish/French/Italian/Portuguese/Indonesian accents).
    pdfmetrics.registerFont(TTFont('Helvetica', f'{FONTS_DIR}/arial.ttf'))
    pdfmetrics.registerFont(TTFont('Helvetica-Bold', f'{FONTS_DIR}/arialbd.ttf'))
    pdfmetrics.registerFont(TTFont('Courier', f'{FONTS_DIR}/arial.ttf'))
    pdfmetrics.registerFont(TTFont('Courier-Bold', f'{FONTS_DIR}/arialbd.ttf'))

def register_chinese():
    # FIX: registering msyh.ttc (Microsoft YaHei, a .ttc collection) directly
    # as a TTFont dropped a large fraction of Hanzi glyphs silently —
    # reportlab's TTF parser doesn't fully handle every cmap subtable format
    # some .ttc files use. simsunb.ttf (a plain, non-collection .ttf) was
    # tried next and was WORSE — most Hanzi rendered as visible tofu boxes,
    # confirming that font's cmap genuinely lacks many of the characters
    # used here. Fix: extract face #0 out of msyh.ttc into a standalone
    # .ttf with fontTools (build/msyh_extracted.ttf, generated once via
    # `python -c "from fontTools.ttLib import TTFont; TTFont('msyh.ttc',
    # fontNumber=0).save('msyh_extracted.ttf')"`) — same 29,905-glyph cmap
    # as the original YaHei, but as a single-font file reportlab's TTF
    # parser handles without the collection-container code path. Verified:
    # every character used in this file's Chinese content is present in
    # the extracted cmap.
    msyh = f'{os.path.dirname(__file__)}/msyh_extracted.ttf'
    if not os.path.exists(msyh):
        # Generated on first use, not committed — a 30k-glyph CJK font is
        # ~19MB, not worth carrying in the repo when the source (Windows'
        # own msyh.ttc) is already present on any machine that can render
        # this PDF's reference output anyway.
        from fontTools.ttLib import TTFont as FTFont
        FTFont(f'{FONTS_DIR}/msyh.ttc', fontNumber=0).save(msyh)
    pdfmetrics.registerFont(TTFont('Helvetica', msyh))
    pdfmetrics.registerFont(TTFont('Helvetica-Bold', msyh))
    pdfmetrics.registerFont(TTFont('Courier', msyh))
    pdfmetrics.registerFont(TTFont('Courier-Bold', msyh))

def register_arabic():
    # Segoe UI has Arabic glyph coverage; reportlab does not run an OpenType
    # shaping engine, so arabic_reshaper (applied to the text content, see
    # ARABIC_RESHAPE below) supplies the correct joined letter forms before
    # they ever reach this font.
    pdfmetrics.registerFont(TTFont('Helvetica', f'{FONTS_DIR}/segoeui.ttf'))
    pdfmetrics.registerFont(TTFont('Helvetica-Bold', f'{FONTS_DIR}/segoeuib.ttf'))
    pdfmetrics.registerFont(TTFont('Courier', f'{FONTS_DIR}/segoeui.ttf'))
    pdfmetrics.registerFont(TTFont('Courier-Bold', f'{FONTS_DIR}/segoeuib.ttf'))

def register_hindi():
    # Nirmala UI has Devanagari glyph coverage. KNOWN LIMITATION: reportlab
    # has no OpenType shaping engine, so Devanagari conjuncts and matra
    # (vowel sign) repositioning — both required for fully correct
    # Devanagari typesetting — are not applied. Text remains readable
    # (codepoints render in logical order with the right glyphs) but is not
    # pixel-perfect professional Hindi typesetting. Flagged honestly rather
    # than silently shipped as if it were equivalent to the other 11
    # languages' rendering quality.
    pdfmetrics.registerFont(TTFont('Helvetica', f'{FONTS_DIR}/Nirmala.ttf'))
    pdfmetrics.registerFont(TTFont('Helvetica-Bold', f'{FONTS_DIR}/NirmalaB.ttf'))
    pdfmetrics.registerFont(TTFont('Courier', f'{FONTS_DIR}/Nirmala.ttf'))
    pdfmetrics.registerFont(TTFont('Courier-Bold', f'{FONTS_DIR}/NirmalaB.ttf'))

def reshape_arabic_dict(L):
    """Returns a copy of L with arabic_reshaper applied to every plain-text
    string (and to the text inside each tuple/list entry), so isolated
    Arabic letterforms become correctly joined before reaching the font.
    HTML-like tags (<b>, <font ...>) are preserved: reshaping is applied
    per-segment around tags, not across them, so tag syntax never gets
    mangled."""
    import re
    import arabic_reshaper
    reshaper = arabic_reshaper.ArabicReshaper()
    tag_re = re.compile(r'(<[^>]+>)')

    def reshape_text(s):
        if not isinstance(s, str):
            return s
        parts = tag_re.split(s)
        return ''.join(p if tag_re.fullmatch(p) else reshaper.reshape(p) for p in parts)

    def walk(v):
        if isinstance(v, str):
            return reshape_text(v)
        if isinstance(v, tuple):
            return tuple(walk(x) for x in v)
        if isinstance(v, list):
            return [walk(x) for x in v]
        return v

    return {k: walk(v) for k, v in L.items()}


def _run_group(group):
    out = 'C:/Users/aequitas-chain/downloads'
    os.makedirs(out, exist_ok=True)
    if group == 'latin_cyrillic':
        register_latin_cyrillic()
        for lang_key, L in [('EN', EN), ('DE', DE), ('ES', ES), ('FR', FR),
                             ('IT', IT), ('PT', PT), ('TR', TR), ('ID', ID),
                             ('RU', RU)]:
            path = f'{out}/Aequitas_Node_Guide_{lang_key}.pdf'
            build_pdf(path, L)
            print(f'Generated: {path}')
    elif group == 'zh':
        register_chinese()
        build_pdf(f'{out}/Aequitas_Node_Guide_ZH.pdf', ZH)
        print(f'Generated: {out}/Aequitas_Node_Guide_ZH.pdf')
    elif group == 'ar':
        register_arabic()
        # Right-align body-text styles for RTL reading direction. Only the
        # ones with no explicit alignment already set (body copy, not the
        # already-centered title/sub/tag/footer) need this — those default
        # to TA_LEFT, which is wrong for Arabic paragraphs of reshaped RTL
        # text. Safe to mutate here: this runs in its own subprocess, never
        # affecting any other language's STYLES.
        for key in ('h1', 'h2', 'body', 'sm', 'bullet', 'warn', 'info'):
            STYLES[key].alignment = TA_RIGHT
        build_pdf(f'{out}/Aequitas_Node_Guide_AR.pdf', reshape_arabic_dict(AR))
        print(f'Generated: {out}/Aequitas_Node_Guide_AR.pdf')
    elif group == 'hi':
        register_hindi()
        # Nirmala UI (Windows' Devanagari font) has no glyph for U+2192
        # (→) — confirmed via fontTools cmap check — so every "X → Y" menu
        # path in this file's English-derived UI breadcrumbs (MetaMask →
        # Account Details → ...) silently dropped the arrows. Substitute a
        # plain ASCII arrow the font does have glyphs for.
        def replace_arrow(v):
            if isinstance(v, str):
                return v.replace('→', '->')
            if isinstance(v, tuple):
                return tuple(replace_arrow(x) for x in v)
            if isinstance(v, list):
                return [replace_arrow(x) for x in v]
            return v
        HI_fixed = {k: replace_arrow(v) for k, v in HI.items()}
        build_pdf(f'{out}/Aequitas_Node_Guide_HI.pdf', HI_fixed)
        print(f'Generated: {out}/Aequitas_Node_Guide_HI.pdf')


if __name__ == '__main__':
    import sys
    if len(sys.argv) > 1:
        # Internal: invoked as a subprocess for one font group only — see below.
        _run_group(sys.argv[1])
    else:
        # FIX (2026-06-29): registering a SECOND, different font file under
        # the same name ("Helvetica") that a FIRST font was already
        # registered under, within the same Python process, silently
        # corrupts rendering for the second font — confirmed by direct
        # reproduction: arial.ttf registered as "Helvetica", a PDF built
        # successfully, then msyh_extracted.ttf (Chinese) re-registered as
        # "Helvetica" in the SAME process produced a PDF where almost every
        # Hanzi glyph rendered as a blank/tofu box, despite the exact same
        # font file working perfectly when registered as "Helvetica" in a
        # fresh process with nothing registered under that name before it.
        # reportlab caches per-font-name encoding/width data older than the
        # registerFont() call that's supposed to replace it, and doesn't
        # invalidate that cache on re-registration. Each script-name needs
        # its OWN font file aliased to "Helvetica"/"Courier" (so the many
        # existing fontName='Helvetica' references throughout this file
        # keep working unchanged for every language, including the inline
        # <font name="Courier"...> tags embedded in the translated text
        # itself) — the only reliable fix is to never register a second
        # font under a name already used in this process, hence: one fresh
        # subprocess per font family instead of sequential re-registration.
        import subprocess
        for group in ('latin_cyrillic', 'zh', 'ar', 'hi'):
            result = subprocess.run([sys.executable, __file__, group])
            if result.returncode != 0:
                raise SystemExit(f'Font group {group!r} failed (exit {result.returncode})')
        print('Done.')
