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
version  = 'v1.0 · June 2026 · aequitas.digital',
tagline  = 'Complete step-by-step guide · No prior blockchain experience required · ~20–30 min',

prereq_title = 'Before You Start — What You Need',
prereqs = [
    ('1.', '<b>An Aequitas account:</b> You must first be registered as a human on Aequitas. Install the Android app, complete biometric registration, and note your wallet address. Without this, you cannot receive validator rewards.'),
    ('2.', '<b>A GitHub account (free):</b> Go to github.com and create a free account. You need this to copy (fork) the Aequitas code so Railway can deploy it.'),
    ('3.', '<b>A Railway account (free):</b> Go to railway.app and sign in with GitHub. Railway is a hosting platform that runs your node in the cloud — no server or command line required.'),
    ('4.', '<b>Node signing key (RELAYER_PRIVATE_KEY):</b> Your node needs a dedicated Ethereum wallet to sign on-chain registrations. This can be any MetaMask wallet. Export its private key: MetaMask → Account Details → Show Private Key → enter password → copy. Keep strictly private. <b>IMPORTANT:</b> To receive validator rewards you also need NODE_OPERATOR_WALLET set to your <b>registered Aequitas human wallet</b> (the one verified with AequitasBio). Only verified humans can earn validator rewards.'),
    ('5.', '<b>10–30 minutes of your time.</b> Railway does most of the work automatically.'),
],

vars_title = 'Step 1 — Environment Variables',
vars_warn  = 'Security Warning: Your RELAYER_PRIVATE_KEY is like a master password. Anyone who has it controls your node wallet. Never share it publicly, never paste it in chat or email. Use a separate MetaMask wallet for RELAYER_PRIVATE_KEY (signing). NODE_OPERATOR_WALLET (for rewards) must be your registered Aequitas human wallet.',
var_cols   = ['Variable', 'Required?', 'What to set'],
vars = [
    ('DATABASE_URL',        'YES',         'Your PostgreSQL connection string. On Railway: auto-injected when PostgreSQL is in the same project. Format: postgres://user:pass@host:5432/dbname'),
    ('RELAYER_PRIVATE_KEY', 'YES',         'The private key (0x…, 66 chars) of your dedicated node wallet. MetaMask: Account Details → Show Private Key → enter password → copy.'),
    ('RELAYER_ADDRESS',     'Recommended', 'The wallet address (0x…, 42 chars) matching RELAYER_PRIVATE_KEY. Copy from MetaMask. A fallback exists but setting this explicitly prevents startup errors.'),
    ('NODE_OPERATOR_WALLET','For rewards', 'Your Aequitas human wallet address — registered via the Android app. Receives your daily validator rewards (40% of all protocol fees). Must be a registered human.'),
    ('NODE_OPERATOR_BINDING_SIGNATURE', 'For rewards', 'Proves you own NODE_OPERATOR_WALLET. Generate it at aequitas.digital/node-binding: sign the shown message with your human wallet in MetaMask, paste the resulting signature here. Without it your node still runs, but cannot auto-register for validator rewards.'),
    ('PEER_SECRET',         'Optional/Legacy', 'Legacy shared-secret fallback. No longer required — nodes authenticate automatically via cryptographic challenge-response (RELAYER_PRIVATE_KEY). Only needed for backward compatibility with older deployments.'),
    ('SELF_URL',            'Multi-node',  'Your node\'s own public HTTPS URL (e.g. https://my-node.up.railway.app). Required for peer discovery self-exclusion. Find in Railway: Settings → Networking → Public Networking.'),
    ('PRIMARY_NODE_URL',    'Multi-node',  'Set to: https://aequitas.digital — the primary node your node registers with for automatic peer discovery. On startup your node posts its URL + signing address to the primary, gets the full peer list back, and joins the network automatically.'),
    ('BOOTSTRAP_SNAPSHOT_URL', 'Recommended', 'Set to: https://aequitas.digital/api/snapshot — lets a brand-new node start from the network\'s current state instead of replaying the entire history from genesis. Dramatically faster first sync.'),
    ('BOOTSTRAP_SIGNER',    'With snapshot', 'The primary node\'s signing address, used to verify the snapshot is genuine before importing it. Get the current value from https://aequitas.digital/api/status → "signing_address". Required whenever BOOTSTRAP_SNAPSHOT_URL is set.'),
    ('SNAPSHOT_TOKEN',      'Optional',    'Not required to bootstrap a new node — without it you still get everything needed to run correctly (accounts, balances, pool, config). Only unlocks the full export (nullifier/wallet linkage + bio_registrations), used for authoritative resync of an already-diverged node. Ask the network operator only if you actually need that.'),
    ('RESYNC_FROM_SNAPSHOT', 'Recovery only', 'DANGEROUS, temporary: set to true together with BOOTSTRAP_SNAPSHOT_URL and BOOTSTRAP_SIGNER only to recover a node whose state has diverged from the network. Replaces local state outright. Restart once, then remove this variable again — leaving it set forces a full resync on every restart.'),
    ('AUTO_HEAL_ON_DIVERGENCE', 'Strongly recommended', 'Important for network security and speed: if your node\'s chain ever diverges from the network (e.g. after downtime or a bad restart), it detects this itself by comparing against PRIMARY_NODE_URL every few minutes and resyncs automatically — no manual RESYNC_FROM_SNAPSHOT needed. A node that stays diverged and keeps broadcasting its own blocks can slow down or destabilize the network for every other operator. Set to true together with PRIMARY_NODE_URL, BOOTSTRAP_SNAPSHOT_URL and BOOTSTRAP_SIGNER.'),
    ('PORT',                'No',          'Leave unset on Railway — Railway sets this automatically. Default is 8080.'),
    ('NODE_KEY',            'No',          'Base64 libp2p key for stable P2P identity. Auto-generated if omitted, but changes on every restart. If not set, the node prints it to stderr: "SAVE THIS AS NODE_KEY ENVIRONMENT VAR: <base64>". Copy and paste it here.'),
    ('IS_PRIMARY_NODE',     'No',          'Leave unset or false. Distribution now uses a DB-level lock — any node can run it without this variable. Setting true on a secondary node is no longer necessary.'),
    ('RESET_STATE',         'No',          'DANGEROUS: Setting this to true wipes your entire database on every restart. Development use only. Never in production.'),
],

railway_title = 'Step 2 — Deploy on Railway (Recommended)',
railway_intro = 'Railway is the easiest way to run your node — no server setup, no command line required. The free tier covers all requirements. Total time: about 10–15 minutes.',
railway_steps = [
    'Fork github.com/hanoi96international-gif/Aequitas to your own GitHub account (click <b>Fork</b> → <b>Create fork</b>)',
    'On railway.app, sign in with GitHub, then <b>New Project</b> → <b>+ New</b> → <b>Database</b> → <b>Add PostgreSQL</b>',
    'In that same Railway project, click <b>+ New → GitHub Repo</b> and select your Aequitas fork — Railway detects the Dockerfile automatically',
    'Click <b>Deploy Now</b> — a first build starts (may fail without env vars, that is normal)',
    'Click your Aequitas service → <b>Variables</b> → add each variable (see table above). Minimum required: RELAYER_PRIVATE_KEY, RELAYER_ADDRESS, NODE_OPERATOR_WALLET, SELF_URL, PRIMARY_NODE_URL=https://aequitas.digital (PEER_SECRET is no longer required)',
    'Click <b>Deploy</b> (or save variables to trigger auto-redeploy). Build takes ~3 minutes while Go compiles the node binary.',
    'Watch <b>Deploy Logs</b>. Success looks like: <font name="Courier" color="#5B21B6">Aequitas Node Running</font> and <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    'Go to <b>Settings → Networking → Generate Domain</b> to get your public URL',
    'Open <font name="Courier">https://YOUR-URL/api/status</font> — you should see JSON with <b>height</b> climbing every ~6 seconds',
],
railway_vars_code = (
    '# Railway auto-sets DATABASE_URL if PostgreSQL is in the same project\n'
    'RELAYER_PRIVATE_KEY    = 0xYOUR_PRIVATE_KEY\n'
    'RELAYER_ADDRESS        = 0xYOUR_NODE_WALLET_ADDRESS\n'
    'NODE_OPERATOR_WALLET   = 0xYOUR_HUMAN_WALLET\n'
    '# PEER_SECRET is no longer required — authentication is automatic\n'
    'SELF_URL               = https://YOUR-RAILWAY-DOMAIN.up.railway.app\n'
    'PRIMARY_NODE_URL       = https://aequitas.digital'
),

docker_title = 'Step 2b — Alternative: Deploy with Docker (Advanced)',
docker_intro = 'Use this if you have your own server (VPS, home server, cloud VM). Requires Docker and a PostgreSQL database.',
docker_code  = (
    '# 1. Download the code\n'
    'git clone https://github.com/hanoi96international-gif/Aequitas && cd Aequitas\n\n'
    '# 2. Build the node image (~3 min for Go compilation)\n'
    'docker build -t aequitas-node .\n\n'
    '# 3. Start the node\n'
    'docker run -d --name aequitas-node --restart unless-stopped \\\n'
    '  -e DATABASE_URL="postgres://user:pass@host:5432/aequitas" \\\n'
    '  -e RELAYER_PRIVATE_KEY="0xYOUR_PRIVATE_KEY" \\\n'
    '  -e RELAYER_ADDRESS="0xYOUR_NODE_WALLET_ADDRESS" \\\n'
    '  -e NODE_OPERATOR_WALLET="0xYOUR_HUMAN_WALLET" \\\n'
    '  # -e PEER_SECRET="..." (optional/legacy, not required) \\\n'
    '  -e SELF_URL="https://YOUR-PUBLIC-URL" \\\n'
    '  -e PRIMARY_NODE_URL="https://aequitas.digital" \\\n'
    '  -p 8080:8080 aequitas-node\n\n'
    '# 4. Watch the live logs\n'
    'docker logs -f aequitas-node'
),

verify_title = 'Step 3 — Verify Your Node is Running',
verify_body  = 'Open these URLs in your browser. Replace YOUR-NODE-URL with your actual Railway domain or server address.',
verify_code  = (
    'https://YOUR-NODE-URL/api/status\n'
    ' → Expected: {"height": 1234, "total_humans": N, "aequitas_index": N}\n\n'
    'https://YOUR-NODE-URL/rpc\n'
    ' → Expected: {"jsonrpc":"2.0","error":"method not specified"} — RPC is alive'
),
verify_note  = 'The block height should match the primary node within 1–2 blocks within seconds of startup. If it stays at 0, check that PRIMARY_NODE_URL=https://aequitas.digital is set and reachable.',

valkey_title = 'Step 3b — Register Your Validator Key (Decentralized Auth)',
valkey_body  = 'Instead of a shared PEER_SECRET, register your node signing key with your human wallet. This cryptographically proves you control both keys. Get the signing key signature by running this on your server (SSH/Railway shell):',
valkey_code  = 'curl "http://localhost:8080/api/sign-validator-challenge?wallet=0xYOUR_HUMAN_WALLET"',
valkey_note  = 'Then use the website Network → Run a Node tab and click "Sign with MetaMask & Register" to complete the registration.',

mm_title = 'Step 4 — Connect MetaMask to Your Node (Optional)',
mm_body  = 'In MetaMask: click the network dropdown → Add network → Add a network manually, then enter:',
mm_rows  = [
    ('Network Name',    'Aequitas Chain'),
    ('RPC URL',         'https://YOUR-NODE-URL/rpc'),
    ('Chain ID',        '1926'),
    ('Currency Symbol', 'AEQ'),
    ('Decimals',        '18'),
    ('Block Explorer',  'https://aequitas.digital'),
],

rewards_title = 'Step 5 — Earning Validator Rewards',
rewards_box   = 'The Validators Pool collects 40% of all protocol fees (swap fees, demurrage, wealth cap overflow). Every day at 20:00 Berlin time (CEST/CET, handles DST automatically) the node distributes the pool balance to all registered node operators proportionally by blocks produced. The more consistently your node runs, the larger your share.',
rewards_steps = [
    'Make sure you are registered as a human on Aequitas. If not: install the Android app and complete biometric registration first. You will receive a wallet address and 1,000 AEQ.',
    'Set <font name="Courier">NODE_OPERATOR_WALLET</font> = your Aequitas human wallet address in your Railway Variables',
    'Save — Railway redeploys automatically. On Docker: <font name="Courier">docker restart aequitas-node</font>',
    'In your node logs, confirm: <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    'Rewards are distributed automatically every day at 20:00 Berlin time (CEST/CET). Just keep your node running — no further action needed.',
],

trouble_title = 'Troubleshooting',
trouble_cols  = ['Symptom', 'Likely Cause', 'Solution'],
trouble_rows  = [
    ('Block height stays at 0',        'PRIMARY_NODE_URL not set or wrong',        'Set PRIMARY_NODE_URL=https://aequitas.digital and redeploy. Also set SELF_URL to your node\'s public URL.'),
    ('DATABASE_URL error on startup',  'Wrong connection string',                  'Check format: postgres://user:pass@host:5432/dbname — make sure PostgreSQL is running and accessible.'),
    ('"no code at address" in logs',   'V7 contract not yet deployed',             'Normal on first start — node auto-deploys V7. Wait a few seconds and check again.'),
    ('"NODE_OPERATOR_WALLET not set"', 'Missing environment variable',             'Add NODE_OPERATOR_WALLET=0xYOUR_HUMAN_WALLET. Node runs fine without it but you won\'t receive rewards.'),
    ('Railway "Application error"',    'Build or startup failure',                 'Check Deploy Logs. Most common: DATABASE_URL missing or RELAYER_PRIVATE_KEY in wrong format (must start with 0x).'),
    ('Port 8080 not reachable (Docker)','Firewall or cloud provider config',       'Open TCP port 8080 inbound in your firewall or cloud security group settings.'),
    ('Docker build fails (module error)','No internet during build',               'Docker build needs outbound internet to download Go modules. Railway handles this automatically.'),
],

footer = 'Aequitas Chain · Chain ID 1926 · aequitas.digital · Validator rewards: daily at 20:00 Berlin time (CEST/CET)',
)

DE = dict(
title    = 'AEQUITAS NODE-BETREIBER-ANLEITUNG',
version  = 'v1.0 · Juni 2026 · aequitas.digital',
tagline  = 'Vollstaendige Schritt-fuer-Schritt-Anleitung · Keine Vorkenntnisse noetig · ca. 20–30 Min.',

prereq_title = 'Vor dem Start — Was du brauchst',
prereqs = [
    ('1.', '<b>Ein Aequitas-Konto:</b> Du musst zuerst als Mensch auf Aequitas registriert sein. Installiere die Android-App, schliesse die biometrische Registrierung ab und notiere deine Wallet-Adresse. Ohne dies kannst du keine Validator-Belohnungen erhalten.'),
    ('2.', '<b>Ein GitHub-Konto (kostenlos):</b> Erstelle eines auf github.com. Du brauchst es um den Aequitas-Code zu forken, damit Railway ihn deployen kann.'),
    ('3.', '<b>Ein Railway-Konto (kostenlos):</b> Gehe zu railway.app und melde dich mit GitHub an. Railway ist eine Hosting-Plattform die deinen Node in der Cloud betreibt — kein eigener Server oder Terminal erforderlich.'),
    ('4.', '<b>Node Signing-Key (RELAYER_PRIVATE_KEY):</b> Dein Node braucht eine dedizierte Ethereum-Wallet zum Signieren. Das kann jede MetaMask-Wallet sein. Exportiere den privaten Schluessel: MetaMask → Kontodetails → Privaten Schluessel anzeigen → Passwort eingeben → kopieren. Streng geheimhalten. <b>WICHTIG:</b> Um Validator-Belohnungen zu erhalten, muss NODE_OPERATOR_WALLET deine <b>registrierte Aequitas-Mensch-Wallet</b> sein (die mit AequitasBio verifizierte). Nur verifizierte Menschen koennen Validator-Belohnungen verdienen.'),
    ('5.', '<b>10–30 Minuten deiner Zeit.</b> Railway erledigt den Grossteil automatisch.'),
],

vars_title = 'Schritt 1 — Umgebungsvariablen',
vars_warn  = 'Sicherheitswarnung: Dein RELAYER_PRIVATE_KEY ist wie ein Master-Passwort. Wer ihn hat, kontrolliert deine Node-Wallet. Niemals oeffentlich teilen, niemals in Chat oder E-Mail einfuegen. Verwende fuer RELAYER_PRIVATE_KEY eine separate Wallet. NODE_OPERATOR_WALLET (fuer Belohnungen) muss deine registrierte Aequitas-Mensch-Wallet sein.',
var_cols   = ['Variable', 'Erforderlich?', 'Was eintragen'],
vars = [
    ('DATABASE_URL',        'JA',           'Dein PostgreSQL-Verbindungsstring. Auf Railway: automatisch gesetzt wenn PostgreSQL im gleichen Projekt. Format: postgres://user:pass@host:5432/dbname'),
    ('RELAYER_PRIVATE_KEY', 'JA',           'Privater Schluessel deiner Node-Wallet (0x…, 66 Zeichen). MetaMask: Kontodetails → Privaten Schluessel anzeigen → Passwort → kopieren.'),
    ('RELAYER_ADDRESS',     'Empfohlen',    'Wallet-Adresse (0x…, 42 Zeichen) passend zu RELAYER_PRIVATE_KEY. Aus MetaMask kopieren. Verhindert Startfehler.'),
    ('NODE_OPERATOR_WALLET','Fuer Bel.',    'Deine Aequitas-Mensch-Wallet — die via Android-App registrierte. Erhaelt taeglich Validator-Belohnungen (40% aller Protokollgebuehren). Muss ein registrierter Mensch sein.'),
    ('NODE_OPERATOR_BINDING_SIGNATURE', 'Fuer Bel.', 'Beweist dass dir NODE_OPERATOR_WALLET gehoert. Erzeugen unter aequitas.digital/node-binding: angezeigte Nachricht mit deiner Mensch-Wallet in MetaMask signieren, Signatur hier eintragen. Ohne sie laeuft der Node trotzdem, kann sich aber nicht fuer Belohnungen registrieren.'),
    ('PEER_SECRET',         'Optional/Legacy', 'Legacy-Fallback. Nicht mehr erforderlich — Nodes authentifizieren sich automatisch per Challenge-Response (RELAYER_PRIVATE_KEY). Nur fuer Rueckwaertskompatibilitaet mit aelteren Deployments benoetigt.'),
    ('SELF_URL',            'Multi-Node',   'Eigene oeffentliche HTTPS-URL des Nodes (z.B. https://mein-node.up.railway.app). In Railway: Settings → Networking → Public Networking.'),
    ('PRIMARY_NODE_URL',    'Multi-Node',   'Auf https://aequitas.digital setzen — der Primaer-Node bei dem sich dein Node registriert. Beim Start postet der Node URL + Signing-Adresse und bekommt die Peer-Liste zurueck.'),
    ('BOOTSTRAP_SNAPSHOT_URL', 'Empfohlen', 'Auf https://aequitas.digital/api/snapshot setzen — laesst einen neuen Node mit dem aktuellen Netzwerk-Stand starten statt die gesamte Historie ab Genesis nachzuspielen. Deutlich schnellerer Erststart.'),
    ('BOOTSTRAP_SIGNER',    'Mit Snapshot', 'Signing-Adresse des Primaer-Nodes — prueft die Echtheit des Snapshots vor dem Import. Aktuellen Wert unter https://aequitas.digital/api/status → "signing_address" finden. Erforderlich wenn BOOTSTRAP_SNAPSHOT_URL gesetzt ist.'),
    ('SNAPSHOT_TOKEN',      'Optional',     'Nicht erforderlich zum Bootstrappen eines neuen Nodes — auch ohne erhaeltst du alles Noetige (Accounts, Salden, Pool, Config). Schaltet nur den vollen Export frei (Nullifier/Wallet-Verknuepfung + bio_registrations) fuer den autoritativen Resync eines bereits divergenten Nodes. Beim Netzwerkbetreiber erfragen falls noetig.'),
    ('RESYNC_FROM_SNAPSHOT', 'Nur Recovery', 'GEFAEHRLICH, temporaer: nur zusammen mit BOOTSTRAP_SNAPSHOT_URL und BOOTSTRAP_SIGNER setzen, um einen vom Netzwerk abgewichenen Node zu reparieren. Ersetzt den lokalen Zustand komplett. Einmal neu starten, dann diese Variable wieder entfernen — sonst erfolgt bei jedem Neustart ein voller Resync.'),
    ('AUTO_HEAL_ON_DIVERGENCE', 'Dringend empfohlen', 'Wichtig fuer Sicherheit und Geschwindigkeit des Netzwerks: Wenn die Chain deines Nodes je vom Netzwerk abweicht (z.B. nach Downtime oder einem fehlgeschlagenen Neustart), erkennt er das selbststaendig — durch regelmaessigen Abgleich mit PRIMARY_NODE_URL alle paar Minuten — und synct sich automatisch neu, ganz ohne manuelles RESYNC_FROM_SNAPSHOT. Ein abweichender Node, der weiter eigene Bloecke verbreitet, kann das gesamte Netzwerk fuer alle anderen Betreiber verlangsamen oder destabilisieren. Zusammen mit PRIMARY_NODE_URL, BOOTSTRAP_SNAPSHOT_URL und BOOTSTRAP_SIGNER auf true setzen.'),
    ('PORT',                'Nein',         'Auf Railway nicht setzen — wird automatisch gesetzt. Standard ist 8080.'),
    ('NODE_KEY',            'Nein',         'Base64 libp2p-Schluessel fuer stabile Peer-Identitaet. Auto-generiert wenn nicht gesetzt, aendert sich dann bei jedem Neustart. Beim ersten Start in stderr ausgegeben: "SAVE THIS AS NODE_KEY: <base64>". Kopieren und hier setzen.'),
    ('IS_PRIMARY_NODE',     'Nein',         'Nicht setzen oder false lassen. Die Ausschuettung nutzt jetzt einen DB-Lock — jeder Node kann sie ohne diese Variable ausfuehren.'),
    ('RESET_STATE',         'Nein',         'GEFAEHRLICH: True loescht die gesamte DB bei jedem Neustart. Nur fuer Entwicklung. Niemals in Produktion.'),
],

railway_title = 'Schritt 2 — Deployment auf Railway (Empfohlen)',
railway_intro = 'Railway ist der einfachste Weg deinen Node zu betreiben — kein Server-Setup, kein Terminal erforderlich. Der kostenlose Tarif deckt alle Anforderungen. Gesamtzeit: ca. 10–15 Minuten.',
railway_steps = [
    'github.com/hanoi96international-gif/Aequitas forken (eigenes GitHub-Konto, <b>Fork</b> → <b>Create fork</b>)',
    'Auf railway.app mit GitHub anmelden, dann <b>New Project</b> → <b>+ New</b> → <b>Database</b> → <b>Add PostgreSQL</b>',
    'Im selben Railway-Projekt: <b>+ New → GitHub Repo</b> klicken und deinen Aequitas-Fork auswaehlen — Railway erkennt das Dockerfile automatisch',
    '<b>Deploy Now</b> klicken — ein erster Build startet (kann ohne Env Vars fehlschlagen, das ist normal)',
    'Aequitas-Service → <b>Variables</b> → Variablen hinzufuegen (siehe Tabelle oben). Mindest-Anforderung: RELAYER_PRIVATE_KEY, RELAYER_ADDRESS, NODE_OPERATOR_WALLET, SELF_URL, PRIMARY_NODE_URL=https://aequitas.digital (PEER_SECRET nicht mehr erforderlich)',
    '<b>Deploy</b> klicken (oder Variablen speichern fuer Auto-Redeploy). Build dauert ~3 Minuten fuer Go-Kompilierung.',
    'Deploy-Logs beobachten. Erfolg sieht so aus: <font name="Courier" color="#5B21B6">Aequitas Node Running</font> und <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    '<b>Settings → Networking → Generate Domain</b> fuer deine oeffentliche URL',
    '<font name="Courier">https://DEINE-URL/api/status</font> aufrufen — du siehst JSON mit <b>height</b> der alle ~6 Sekunden steigt',
],
railway_vars_code = (
    '# Railway setzt DATABASE_URL automatisch wenn PostgreSQL im gleichen Projekt\n'
    'RELAYER_PRIVATE_KEY    = 0xDEIN_PRIVATER_SCHLUESSEL\n'
    'RELAYER_ADDRESS        = 0xDEINE_NODE_WALLET_ADRESSE\n'
    'NODE_OPERATOR_WALLET   = 0xDEINE_MENSCH_WALLET\n'
    '# PEER_SECRET ist nicht mehr erforderlich — Authentifizierung ist automatisch\n'
    'SELF_URL               = https://DEIN-RAILWAY-DOMAIN.up.railway.app\n'
    'PRIMARY_NODE_URL       = https://aequitas.digital'
),

docker_title = 'Schritt 2b — Alternative: Docker-Deployment (Fortgeschritten)',
docker_intro = 'Nutze dies wenn du einen eigenen Server hast (VPS, Heimserver, Cloud-VM). Erfordert Docker und eine PostgreSQL-Datenbank.',
docker_code  = (
    '# 1. Code herunterladen\n'
    'git clone https://github.com/hanoi96international-gif/Aequitas && cd Aequitas\n\n'
    '# 2. Node-Image erstellen (~3 Min fuer Go-Kompilierung)\n'
    'docker build -t aequitas-node .\n\n'
    '# 3. Node starten — alle Platzhalter ersetzen\n'
    'docker run -d --name aequitas-node --restart unless-stopped \\\n'
    '  -e DATABASE_URL="postgres://user:pass@host:5432/aequitas" \\\n'
    '  -e RELAYER_PRIVATE_KEY="0xDEIN_PRIVATER_SCHLUESSEL" \\\n'
    '  -e RELAYER_ADDRESS="0xDEINE_NODE_WALLET_ADRESSE" \\\n'
    '  -e NODE_OPERATOR_WALLET="0xDEINE_MENSCH_WALLET" \\\n'
    '  # -e PEER_SECRET="..." (optional/legacy, nicht erforderlich) \\\n'
    '  -e SELF_URL="https://DEINE-OEFFENTLICHE-URL" \\\n'
    '  -e PRIMARY_NODE_URL="https://aequitas.digital" \\\n'
    '  -p 8080:8080 aequitas-node\n\n'
    '# 4. Live-Logs beobachten\n'
    'docker logs -f aequitas-node'
),

verify_title = 'Schritt 3 — Node-Betrieb pruefen',
verify_body  = 'Oeffne diese URLs im Browser. Ersetze DEINE-NODE-URL durch deine Railway-Domain oder Server-Adresse.',
verify_code  = (
    'https://DEINE-NODE-URL/api/status\n'
    ' → Erwartet: {"height": 1234, "total_humans": N, "aequitas_index": N}\n\n'
    'https://DEINE-NODE-URL/rpc\n'
    ' → Erwartet: {"jsonrpc":"2.0","error":"method not specified"} — RPC laeuft'
),
verify_note  = 'Die Blockhoehe sollte innerhalb von Sekunden mit dem Primaer-Node uebereinstimmen (1–2 Bloecke). Bleibt sie bei 0: PRIMARY_NODE_URL=https://aequitas.digital pruefen.',

valkey_title = 'Schritt 3b — Validator-Schluessel registrieren (Dezentrale Auth)',
valkey_body  = 'Statt eines gemeinsamen PEER_SECRET kannst du deinen Node-Signing-Key mit deiner Mensch-Wallet registrieren. Fuhre diesen Befehl auf deinem Server aus (SSH/Railway Shell):',
valkey_code  = 'curl "http://localhost:8080/api/sign-validator-challenge?wallet=0xDEINE_MENSCH_WALLET"',
valkey_note  = 'Dann auf der Website unter Network → Run a Node den Button "Sign with MetaMask & Register" nutzen um die Registrierung abzuschliessen.',

mm_title = 'Schritt 4 — MetaMask mit deinem Node verbinden (Optional)',
mm_body  = 'In MetaMask: Netzwerk-Dropdown → Netzwerk hinzufuegen → Netzwerk manuell hinzufuegen:',
mm_rows  = [
    ('Network Name',    'Aequitas Chain'),
    ('RPC URL',         'https://DEINE-NODE-URL/rpc'),
    ('Chain ID',        '1926'),
    ('Currency Symbol', 'AEQ'),
    ('Decimals',        '18'),
    ('Block Explorer',  'https://aequitas.digital'),
],

rewards_title = 'Schritt 5 — Validator-Belohnungen erhalten',
rewards_box   = 'Der Validators-Pool sammelt 40% aller Protokollgebuehren (Swap-Gebuehren, Demurrage, Wealth-Cap-Ueberschuss). Jeden Tag um 20:00 Uhr Berliner Zeit (CEST/CET, DST automatisch) verteilt der Node den Pool-Saldo proportional nach produzierten Bloecken an alle registrierten Node-Betreiber. Je laenger dein Node laeuft, desto groesser dein Anteil.',
rewards_steps = [
    'Stelle sicher, dass du als Mensch auf Aequitas registriert bist. Falls nicht: Android-App installieren und biometrische Registrierung abschliessen. Du erhaeltst eine Wallet-Adresse und 1.000 AEQ.',
    '<font name="Courier">NODE_OPERATOR_WALLET</font> = deine Aequitas-Mensch-Wallet-Adresse in Railway Variables setzen',
    'Speichern — Railway redeployt automatisch. Mit Docker: <font name="Courier">docker restart aequitas-node</font>',
    'In den Node-Logs bestaetigen: <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    'Belohnungen werden automatisch jeden Tag um 20:00 Uhr Berliner Zeit (CEST/CET) verteilt. Node laufen lassen — kein weiterer Eingriff noetig.',
],

trouble_title = 'Fehlerbehebung',
trouble_cols  = ['Symptom', 'Wahrscheinliche Ursache', 'Loesung'],
trouble_rows  = [
    ('Blockhoehe bleibt bei 0',         'PRIMARY_NODE_URL nicht gesetzt',             'PRIMARY_NODE_URL=https://aequitas.digital setzen und neu deployen. SELF_URL auf Node-URL setzen.'),
    ('DATABASE_URL-Fehler beim Start',   'Falscher Connection-String',                'Format pruefen: postgres://user:pass@host:5432/dbname — PostgreSQL muss erreichbar sein.'),
    ('"no code at address" in Logs',     'V7-Contract noch nicht deployed',            'Normal beim ersten Start — Node deployed V7 automatisch. Kurz warten.'),
    ('"NODE_OPERATOR_WALLET not set"',   'Fehlende Umgebungsvariable',                'NODE_OPERATOR_WALLET=0xDEINE_MENSCH_WALLET hinzufuegen. Node laeuft ohne, aber keine Belohnungen.'),
    ('Railway "Application error"',      'Build- oder Startfehler',                   'Deploy-Logs pruefen. Haeufigste Ursache: fehlende DATABASE_URL oder falsches Schluessel-Format.'),
    ('Port 8080 nicht erreichbar (Docker)','Firewall oder Cloud-Konfiguration',        'TCP-Port 8080 eingehend in Firewall oder Cloud-Security-Gruppe oeffnen.'),
    ('Docker Build scheitert (Module)',   'Kein Internet beim Build',                  'Docker Build benoetigt ausgehenden Internetzugang. Railway erledigt das automatisch.'),
],

footer = 'Aequitas Chain · Chain ID 1926 · aequitas.digital · Validator-Belohnungen: taeglich 20:00 Uhr Berliner Zeit (CEST/CET)',
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

ES = dict(
title    = 'GUÍA DEL OPERADOR DE NODO AEQUITAS',
version  = 'v1.0 · Junio 2026 · aequitas.digital',
tagline  = 'Guía completa paso a paso · No requiere experiencia previa en blockchain · ~20–30 min',

prereq_title = 'Antes de empezar — Qué necesitas',
prereqs = [
    ('1.', '<b>Una cuenta de Aequitas:</b> Primero debes registrarte como humano en Aequitas. Instala la app de Android, completa el registro biométrico y anota tu dirección de wallet. Sin esto no puedes recibir recompensas de validador.'),
    ('2.', '<b>Una cuenta de GitHub (gratis):</b> Ve a github.com y crea una cuenta gratuita. La necesitas para copiar (fork) el código de Aequitas para que Railway pueda desplegarlo.'),
    ('3.', '<b>Una cuenta de Railway (gratis):</b> Ve a railway.app e inicia sesión con GitHub. Railway es una plataforma de hosting que ejecuta tu nodo en la nube — no necesitas servidor ni línea de comandos.'),
    ('4.', '<b>Clave de firma del nodo (RELAYER_PRIVATE_KEY):</b> Tu nodo necesita una wallet de Ethereum dedicada para firmar los registros en cadena. Puede ser cualquier wallet de MetaMask. Exporta su clave privada: MetaMask → Detalles de la cuenta → Mostrar clave privada → introduce la contraseña → copia. Mantenla estrictamente privada. <b>IMPORTANTE:</b> Para recibir recompensas de validador también necesitas que NODE_OPERATOR_WALLET esté configurada con tu <b>wallet humana registrada en Aequitas</b> (la verificada con AequitasBio). Solo los humanos verificados pueden ganar recompensas de validador.'),
    ('5.', '<b>10–30 minutos de tu tiempo.</b> Railway hace la mayor parte del trabajo automáticamente.'),
],

vars_title = 'Paso 1 — Variables de entorno',
vars_warn  = 'Advertencia de seguridad: Tu RELAYER_PRIVATE_KEY es como una contraseña maestra. Cualquiera que la tenga controla tu wallet del nodo. Nunca la compartas públicamente, nunca la pegues en chat o correo. Usa una wallet de MetaMask separada para RELAYER_PRIVATE_KEY (firma). NODE_OPERATOR_WALLET (para recompensas) debe ser tu wallet humana registrada en Aequitas.',
var_cols   = ['Variable', '¿Obligatoria?', 'Qué configurar'],
vars = [
    ('DATABASE_URL',        'SÍ',          'Tu cadena de conexión PostgreSQL. En Railway: se inyecta automáticamente si PostgreSQL está en el mismo proyecto. Formato: postgres://user:pass@host:5432/dbname'),
    ('RELAYER_PRIVATE_KEY', 'SÍ',          'La clave privada (0x…, 66 caracteres) de tu wallet dedicada del nodo. MetaMask: Detalles de la cuenta → Mostrar clave privada → introduce la contraseña → copia.'),
    ('RELAYER_ADDRESS',     'Recomendado', 'La dirección de wallet (0x…, 42 caracteres) que corresponde a RELAYER_PRIVATE_KEY. Cópiala de MetaMask. Existe un fallback, pero configurarla explícitamente evita errores de inicio.'),
    ('NODE_OPERATOR_WALLET','Para recompensas', 'Tu dirección de wallet humana de Aequitas — registrada vía la app de Android. Recibe tus recompensas diarias de validador (40% de todas las comisiones del protocolo). Debe ser un humano registrado.'),
    ('NODE_OPERATOR_BINDING_SIGNATURE', 'Para recompensas', 'Demuestra que eres propietario de NODE_OPERATOR_WALLET. Genérala en aequitas.digital/node-binding: firma el mensaje mostrado con tu wallet humana en MetaMask y pega aquí la firma resultante. Sin ella tu nodo funciona igual, pero no puede registrarse automáticamente para recompensas.'),
    ('PEER_SECRET',         'Opcional/Legado', 'Mecanismo de respaldo heredado. Ya no es obligatorio — los nodos se autentican automáticamente mediante un desafío-respuesta criptográfico (RELAYER_PRIVATE_KEY). Solo necesario por compatibilidad con despliegues antiguos.'),
    ('SELF_URL',            'Multi-nodo',  'La URL pública HTTPS de tu propio nodo (ej. https://mi-nodo.up.railway.app). Necesaria para excluirte a ti mismo en el descubrimiento de pares. Está en Railway: Settings → Networking → Public Networking.'),
    ('PRIMARY_NODE_URL',    'Multi-nodo',  'Configura: https://aequitas.digital — el nodo primario con el que se registra tu nodo para el descubrimiento automático de pares. Al iniciar, tu nodo envía su URL + dirección de firma al primario, recibe la lista completa de pares y se une a la red automáticamente.'),
    ('BOOTSTRAP_SNAPSHOT_URL', 'Recomendado', 'Configura: https://aequitas.digital/api/snapshot — permite que un nodo nuevo arranque desde el estado actual de la red en vez de reproducir todo el historial desde el génesis. Sincronización inicial mucho más rápida.'),
    ('BOOTSTRAP_SIGNER',    'Con snapshot', 'La dirección de firma del nodo primario, usada para verificar que el snapshot es genuino antes de importarlo. Obtén el valor actual en https://aequitas.digital/api/status → "signing_address". Obligatoria siempre que se configure BOOTSTRAP_SNAPSHOT_URL.'),
    ('SNAPSHOT_TOKEN',      'Opcional',    'No es necesaria para arrancar un nodo nuevo — sin ella igualmente obtienes todo lo necesario para funcionar correctamente (cuentas, saldos, pool, configuración). Solo desbloquea la exportación completa (vínculo nullifier/wallet + bio_registrations), usada para una resincronización autoritativa de un nodo ya divergido. Pregunta al operador de la red solo si realmente la necesitas.'),
    ('RESYNC_FROM_SNAPSHOT', 'Solo recuperación', 'PELIGROSO, temporal: configúrala como true junto con BOOTSTRAP_SNAPSHOT_URL y BOOTSTRAP_SIGNER solo para recuperar un nodo cuyo estado ha divergido de la red. Reemplaza el estado local por completo. Reinicia una vez y luego elimina esta variable — si la dejas puesta, fuerza una resincronización completa en cada reinicio.'),
    ('AUTO_HEAL_ON_DIVERGENCE', 'Muy recomendado', 'Importante para la seguridad y la velocidad de la red: si la cadena de tu nodo alguna vez diverge de la red (por ejemplo, tras una caída o un reinicio fallido), lo detecta por sí mismo — comparándose con PRIMARY_NODE_URL cada pocos minutos — y se resincroniza automáticamente, sin necesidad de RESYNC_FROM_SNAPSHOT manual. Un nodo que permanece divergente y sigue difundiendo sus propios bloques puede ralentizar o desestabilizar toda la red para el resto de operadores. Configúralo como true junto con PRIMARY_NODE_URL, BOOTSTRAP_SNAPSHOT_URL y BOOTSTRAP_SIGNER.'),
    ('PORT',                'No',          'No la configures en Railway — Railway la establece automáticamente. El valor por defecto es 8080.'),
    ('NODE_KEY',            'No',          'Clave libp2p en Base64 para una identidad P2P estable. Se autogenera si se omite, pero cambia en cada reinicio. Si no está configurada, el nodo la imprime en stderr: "SAVE THIS AS NODE_KEY ENVIRONMENT VAR: <base64>". Copia y pégala aquí.'),
    ('IS_PRIMARY_NODE',     'No',          'Déjala sin configurar o en false. La distribución ahora usa un bloqueo a nivel de base de datos — cualquier nodo puede ejecutarla sin esta variable.'),
    ('RESET_STATE',         'No',          'PELIGROSO: poner esto en true borra toda tu base de datos en cada reinicio. Solo para desarrollo. Nunca en producción.'),
],

railway_title = 'Paso 2 — Desplegar en Railway (Recomendado)',
railway_intro = 'Railway es la forma más sencilla de ejecutar tu nodo — sin configurar servidores, sin línea de comandos. El plan gratuito cubre todos los requisitos. Tiempo total: unos 10–15 minutos.',
railway_steps = [
    'Haz fork de github.com/hanoi96international-gif/Aequitas a tu propia cuenta de GitHub (clic en <b>Fork</b> → <b>Create fork</b>)',
    'En railway.app, inicia sesión con GitHub, luego <b>New Project</b> → <b>+ New</b> → <b>Database</b> → <b>Add PostgreSQL</b>',
    'En ese mismo proyecto de Railway, haz clic en <b>+ New → GitHub Repo</b> y selecciona tu fork de Aequitas — Railway detecta el Dockerfile automáticamente',
    'Haz clic en <b>Deploy Now</b> — empieza un primer build (puede fallar sin variables de entorno, eso es normal)',
    'Haz clic en tu servicio de Aequitas → <b>Variables</b> → añade cada variable (ver tabla arriba). Mínimo requerido: RELAYER_PRIVATE_KEY, RELAYER_ADDRESS, NODE_OPERATOR_WALLET, SELF_URL, PRIMARY_NODE_URL=https://aequitas.digital (PEER_SECRET ya no es obligatoria)',
    'Haz clic en <b>Deploy</b> (o guarda las variables para activar el auto-redeploy). El build tarda ~3 minutos mientras Go compila el binario del nodo.',
    'Observa los <b>Deploy Logs</b>. El éxito se ve así: <font name="Courier" color="#5B21B6">Aequitas Node Running</font> y <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    'Ve a <b>Settings → Networking → Generate Domain</b> para obtener tu URL pública',
    'Abre <font name="Courier">https://TU-URL/api/status</font> — deberías ver JSON con <b>height</b> subiendo cada ~6 segundos',
],
railway_vars_code = (
    '# Railway configura DATABASE_URL automáticamente si PostgreSQL está en el mismo proyecto\n'
    'RELAYER_PRIVATE_KEY    = 0xTU_CLAVE_PRIVADA\n'
    'RELAYER_ADDRESS        = 0xTU_DIRECCION_WALLET_NODO\n'
    'NODE_OPERATOR_WALLET   = 0xTU_WALLET_HUMANA\n'
    '# PEER_SECRET ya no es obligatoria — la autenticación es automática\n'
    'SELF_URL               = https://TU-DOMINIO-RAILWAY.up.railway.app\n'
    'PRIMARY_NODE_URL       = https://aequitas.digital'
),

docker_title = 'Paso 2b — Alternativa: Desplegar con Docker (Avanzado)',
docker_intro = 'Usa esto si tienes tu propio servidor (VPS, servidor doméstico, VM en la nube). Requiere Docker y una base de datos PostgreSQL.',
docker_code  = (
    '# 1. Descarga el código\n'
    'git clone https://github.com/hanoi96international-gif/Aequitas && cd Aequitas\n\n'
    '# 2. Construye la imagen del nodo (~3 min de compilación Go)\n'
    'docker build -t aequitas-node .\n\n'
    '# 3. Inicia el nodo\n'
    'docker run -d --name aequitas-node --restart unless-stopped \\\n'
    '  -e DATABASE_URL="postgres://user:pass@host:5432/aequitas" \\\n'
    '  -e RELAYER_PRIVATE_KEY="0xTU_CLAVE_PRIVADA" \\\n'
    '  -e RELAYER_ADDRESS="0xTU_DIRECCION_WALLET_NODO" \\\n'
    '  -e NODE_OPERATOR_WALLET="0xTU_WALLET_HUMANA" \\\n'
    '  # -e PEER_SECRET="..." (opcional/legado, no requerido) \\\n'
    '  -e SELF_URL="https://TU-URL-PUBLICA" \\\n'
    '  -e PRIMARY_NODE_URL="https://aequitas.digital" \\\n'
    '  -p 8080:8080 aequitas-node\n\n'
    '# 4. Observa los logs en vivo\n'
    'docker logs -f aequitas-node'
),

verify_title = 'Paso 3 — Verifica que tu nodo está funcionando',
verify_body  = 'Abre estas URLs en tu navegador. Sustituye TU-URL-DE-NODO por tu dominio real de Railway o dirección del servidor.',
verify_code  = (
    'https://TU-URL-DE-NODO/api/status\n'
    ' → Esperado: {"height": 1234, "total_humans": N, "aequitas_index": N}\n\n'
    'https://TU-URL-DE-NODO/rpc\n'
    ' → Esperado: {"jsonrpc":"2.0","error":"method not specified"} — el RPC está activo'
),
verify_note  = 'La altura de bloque debería coincidir con el nodo primario (1–2 bloques de diferencia) en segundos tras el inicio. Si se queda en 0, comprueba que PRIMARY_NODE_URL=https://aequitas.digital esté configurado y sea accesible.',

valkey_title = 'Paso 3b — Registra tu clave de validador (Autenticación descentralizada)',
valkey_body  = 'En vez de un PEER_SECRET compartido, registra la clave de firma de tu nodo con tu wallet humana. Esto demuestra criptográficamente que controlas ambas claves. Obtén la firma de la clave de firma ejecutando esto en tu servidor (SSH/Railway shell):',
valkey_code  = 'curl "http://localhost:8080/api/sign-validator-challenge?wallet=0xTU_WALLET_HUMANA"',
valkey_note  = 'Luego usa la pestaña Network → Run a Node del sitio web y haz clic en "Sign with MetaMask & Register" para completar el registro.',

mm_title = 'Paso 4 — Conecta MetaMask a tu nodo (Opcional)',
mm_body  = 'En MetaMask: haz clic en el desplegable de red → Add network → Add a network manually, e introduce:',
mm_rows  = [
    ('Network Name',    'Aequitas Chain'),
    ('RPC URL',         'https://TU-URL-DE-NODO/rpc'),
    ('Chain ID',        '1926'),
    ('Currency Symbol', 'AEQ'),
    ('Decimals',        '18'),
    ('Block Explorer',  'https://aequitas.digital'),
],

rewards_title = 'Paso 5 — Obtener recompensas de validador',
rewards_box   = 'El Pool de Validadores recopila el 40% de todas las comisiones del protocolo (comisiones de swap, demurrage, excedente del límite de riqueza). Cada día a las 20:00 hora de Berlín (CEST/CET, gestiona el horario de verano automáticamente) el nodo distribuye el saldo del pool entre todos los operadores de nodo registrados de forma proporcional a los bloques producidos. Cuanto más tiempo funcione tu nodo, mayor será tu parte.',
rewards_steps = [
    'Asegúrate de estar registrado como humano en Aequitas. Si no: instala la app de Android y completa primero el registro biométrico. Recibirás una dirección de wallet y 1.000 AEQ.',
    'Configura <font name="Courier">NODE_OPERATOR_WALLET</font> = tu dirección de wallet humana de Aequitas en las Variables de Railway',
    'Guarda — Railway redespliega automáticamente. Con Docker: <font name="Courier">docker restart aequitas-node</font>',
    'En los logs de tu nodo, confirma: <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    'Las recompensas se distribuyen automáticamente cada día a las 20:00 hora de Berlín (CEST/CET). Solo mantén tu nodo funcionando — no se necesita ninguna acción más.',
],

trouble_title = 'Solución de problemas',
trouble_cols  = ['Síntoma', 'Causa probable', 'Solución'],
trouble_rows  = [
    ('La altura de bloque se queda en 0', 'PRIMARY_NODE_URL no configurada o incorrecta', 'Configura PRIMARY_NODE_URL=https://aequitas.digital y redespliega. Configura también SELF_URL con la URL pública de tu nodo.'),
    ('Error de DATABASE_URL al iniciar',  'Cadena de conexión incorrecta',               'Comprueba el formato: postgres://user:pass@host:5432/dbname — asegúrate de que PostgreSQL esté en ejecución y sea accesible.'),
    ('"no code at address" en los logs',  'El contrato V7 aún no está desplegado',       'Normal en el primer arranque — el nodo despliega V7 automáticamente. Espera unos segundos y comprueba de nuevo.'),
    ('"NODE_OPERATOR_WALLET not set"',    'Falta la variable de entorno',                'Añade NODE_OPERATOR_WALLET=0xTU_WALLET_HUMANA. El nodo funciona sin ella, pero no recibirás recompensas.'),
    ('"Application error" en Railway',    'Fallo de build o de arranque',                'Revisa los Deploy Logs. Lo más común: falta DATABASE_URL o RELAYER_PRIVATE_KEY en formato incorrecto (debe empezar con 0x).'),
    ('Puerto 8080 no accesible (Docker)', 'Configuración del firewall o del proveedor cloud', 'Abre el puerto TCP 8080 entrante en tu firewall o en los ajustes de grupo de seguridad de la nube.'),
    ('Falla el build de Docker (error de módulo)', 'Sin internet durante el build',       'El build de Docker necesita internet de salida para descargar los módulos de Go. Railway lo gestiona automáticamente.'),
],

footer = 'Aequitas Chain · Chain ID 1926 · aequitas.digital · Recompensas de validador: diarias a las 20:00 hora de Berlín (CEST/CET)',
)

FR = dict(
title    = 'GUIDE DE L\'OPÉRATEUR DE NŒUD AEQUITAS',
version  = 'v1.0 · Juin 2026 · aequitas.digital',
tagline  = 'Guide complet étape par étape · Aucune expérience blockchain requise · ~20–30 min',

prereq_title = 'Avant de commencer — Ce dont vous avez besoin',
prereqs = [
    ('1.', '<b>Un compte Aequitas :</b> Vous devez d\'abord être enregistré comme humain sur Aequitas. Installez l\'application Android, terminez l\'enregistrement biométrique et notez votre adresse de wallet. Sans cela, vous ne pouvez pas recevoir de récompenses de validateur.'),
    ('2.', '<b>Un compte GitHub (gratuit) :</b> Allez sur github.com et créez un compte gratuit. Vous en avez besoin pour copier (fork) le code d\'Aequitas afin que Railway puisse le déployer.'),
    ('3.', '<b>Un compte Railway (gratuit) :</b> Allez sur railway.app et connectez-vous avec GitHub. Railway est une plateforme d\'hébergement qui exécute votre nœud dans le cloud — aucun serveur ni ligne de commande requis.'),
    ('4.', '<b>Clé de signature du nœud (RELAYER_PRIVATE_KEY) :</b> Votre nœud a besoin d\'un wallet Ethereum dédié pour signer les enregistrements on-chain. Cela peut être n\'importe quel wallet MetaMask. Exportez sa clé privée : MetaMask → Détails du compte → Afficher la clé privée → entrez le mot de passe → copiez. Gardez-la strictement privée. <b>IMPORTANT :</b> Pour recevoir des récompenses de validateur, NODE_OPERATOR_WALLET doit aussi être votre <b>wallet humain enregistré sur Aequitas</b> (celui vérifié avec AequitasBio). Seuls les humains vérifiés peuvent gagner des récompenses de validateur.'),
    ('5.', '<b>10–30 minutes de votre temps.</b> Railway fait la majeure partie du travail automatiquement.'),
],

vars_title = 'Étape 1 — Variables d\'environnement',
vars_warn  = 'Avertissement de sécurité : Votre RELAYER_PRIVATE_KEY est comme un mot de passe maître. Quiconque la possède contrôle le wallet de votre nœud. Ne la partagez jamais publiquement, ne la collez jamais dans un chat ou un e-mail. Utilisez un wallet MetaMask séparé pour RELAYER_PRIVATE_KEY (signature). NODE_OPERATOR_WALLET (pour les récompenses) doit être votre wallet humain enregistré sur Aequitas.',
var_cols   = ['Variable', 'Obligatoire ?', 'Que renseigner'],
vars = [
    ('DATABASE_URL',        'OUI',         'Votre chaîne de connexion PostgreSQL. Sur Railway : injectée automatiquement si PostgreSQL est dans le même projet. Format : postgres://user:pass@host:5432/dbname'),
    ('RELAYER_PRIVATE_KEY', 'OUI',         'La clé privée (0x…, 66 caractères) de votre wallet de nœud dédié. MetaMask : Détails du compte → Afficher la clé privée → mot de passe → copier.'),
    ('RELAYER_ADDRESS',     'Recommandé',  'L\'adresse du wallet (0x…, 42 caractères) correspondant à RELAYER_PRIVATE_KEY. Copiez-la depuis MetaMask. Un fallback existe mais la renseigner explicitement évite des erreurs de démarrage.'),
    ('NODE_OPERATOR_WALLET','Pour récompenses', 'Votre adresse de wallet humain Aequitas — enregistrée via l\'application Android. Reçoit vos récompenses quotidiennes de validateur (40 % de tous les frais de protocole). Doit être un humain enregistré.'),
    ('NODE_OPERATOR_BINDING_SIGNATURE', 'Pour récompenses', 'Prouve que vous possédez NODE_OPERATOR_WALLET. Générez-la sur aequitas.digital/node-binding : signez le message affiché avec votre wallet humain dans MetaMask, puis collez ici la signature obtenue. Sans elle, votre nœud fonctionne quand même, mais ne peut pas s\'enregistrer automatiquement pour les récompenses.'),
    ('PEER_SECRET',         'Optionnel/Legacy', 'Mécanisme de secret partagé historique. Plus obligatoire — les nœuds s\'authentifient désormais automatiquement via un challenge-réponse cryptographique (RELAYER_PRIVATE_KEY). Utile seulement pour la compatibilité avec d\'anciens déploiements.'),
    ('SELF_URL',            'Multi-nœud',  'L\'URL HTTPS publique de votre propre nœud (ex. https://mon-noeud.up.railway.app). Nécessaire pour s\'exclure soi-même lors de la découverte de pairs. À trouver dans Railway : Settings → Networking → Public Networking.'),
    ('PRIMARY_NODE_URL',    'Multi-nœud',  'Définissez : https://aequitas.digital — le nœud principal auprès duquel votre nœud s\'enregistre pour la découverte automatique de pairs. Au démarrage, votre nœud envoie son URL + adresse de signature au nœud principal, reçoit la liste complète des pairs et rejoint le réseau automatiquement.'),
    ('BOOTSTRAP_SNAPSHOT_URL', 'Recommandé', 'Définissez : https://aequitas.digital/api/snapshot — permet à un nouveau nœud de démarrer depuis l\'état actuel du réseau au lieu de rejouer tout l\'historique depuis la genèse. Première synchronisation bien plus rapide.'),
    ('BOOTSTRAP_SIGNER',    'Avec snapshot', 'L\'adresse de signature du nœud principal, utilisée pour vérifier que le snapshot est authentique avant import. Récupérez la valeur actuelle sur https://aequitas.digital/api/status → "signing_address". Obligatoire dès que BOOTSTRAP_SNAPSHOT_URL est défini.'),
    ('SNAPSHOT_TOKEN',      'Optionnel',   'Pas nécessaire pour démarrer un nouveau nœud — vous obtenez quand même tout ce qu\'il faut pour fonctionner correctement (comptes, soldes, pool, config). Ne déverrouille que l\'export complet (lien nullifier/wallet + bio_registrations), utilisé pour une resynchronisation autoritative d\'un nœud déjà divergent. Demandez à l\'opérateur du réseau seulement si vous en avez vraiment besoin.'),
    ('RESYNC_FROM_SNAPSHOT', 'Récupération seulement', 'DANGEREUX, temporaire : à définir sur true avec BOOTSTRAP_SNAPSHOT_URL et BOOTSTRAP_SIGNER uniquement pour récupérer un nœud dont l\'état a divergé du réseau. Remplace entièrement l\'état local. Redémarrez une fois, puis retirez cette variable — la laisser force une resynchronisation complète à chaque redémarrage.'),
    ('AUTO_HEAL_ON_DIVERGENCE', 'Fortement recommandé', 'Important pour la sécurité et la rapidité du réseau : si la chaîne de votre nœud venait à diverger du réseau (par ex. après une interruption ou un redémarrage raté), il le détecte lui-même — en se comparant à PRIMARY_NODE_URL toutes les quelques minutes — et se resynchronise automatiquement, sans RESYNC_FROM_SNAPSHOT manuel. Un nœud resté divergent qui continue de diffuser ses propres blocs peut ralentir ou déstabiliser tout le réseau pour les autres opérateurs. À définir sur true avec PRIMARY_NODE_URL, BOOTSTRAP_SNAPSHOT_URL et BOOTSTRAP_SIGNER.'),
    ('PORT',                'Non',         'Ne pas définir sur Railway — Railway le configure automatiquement. La valeur par défaut est 8080.'),
    ('NODE_KEY',            'Non',         'Clé libp2p en Base64 pour une identité P2P stable. Auto-générée si omise, mais change à chaque redémarrage. Si non définie, le nœud l\'imprime dans stderr : "SAVE THIS AS NODE_KEY ENVIRONMENT VAR: <base64>". Copiez-la et collez-la ici.'),
    ('IS_PRIMARY_NODE',     'Non',         'Laissez non défini ou false. La distribution utilise désormais un verrou au niveau de la base de données — n\'importe quel nœud peut l\'exécuter sans cette variable.'),
    ('RESET_STATE',         'Non',         'DANGEREUX : mettre ceci à true efface toute votre base de données à chaque redémarrage. Usage développement uniquement. Jamais en production.'),
],

railway_title = 'Étape 2 — Déployer sur Railway (Recommandé)',
railway_intro = 'Railway est la façon la plus simple d\'exécuter votre nœud — aucune configuration de serveur, aucune ligne de commande. L\'offre gratuite couvre tous les besoins. Durée totale : environ 10–15 minutes.',
railway_steps = [
    'Forkez github.com/hanoi96international-gif/Aequitas vers votre propre compte GitHub (cliquez sur <b>Fork</b> → <b>Create fork</b>)',
    'Sur railway.app, connectez-vous avec GitHub, puis <b>New Project</b> → <b>+ New</b> → <b>Database</b> → <b>Add PostgreSQL</b>',
    'Dans ce même projet Railway, cliquez sur <b>+ New → GitHub Repo</b> et sélectionnez votre fork d\'Aequitas — Railway détecte automatiquement le Dockerfile',
    'Cliquez sur <b>Deploy Now</b> — un premier build démarre (peut échouer sans variables d\'environnement, c\'est normal)',
    'Cliquez sur votre service Aequitas → <b>Variables</b> → ajoutez chaque variable (voir tableau ci-dessus). Minimum requis : RELAYER_PRIVATE_KEY, RELAYER_ADDRESS, NODE_OPERATOR_WALLET, SELF_URL, PRIMARY_NODE_URL=https://aequitas.digital (PEER_SECRET n\'est plus obligatoire)',
    'Cliquez sur <b>Deploy</b> (ou enregistrez les variables pour déclencher un auto-redéploiement). Le build prend ~3 minutes pendant que Go compile le binaire du nœud.',
    'Observez les <b>Deploy Logs</b>. Le succès ressemble à : <font name="Courier" color="#5B21B6">Aequitas Node Running</font> et <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    'Allez dans <b>Settings → Networking → Generate Domain</b> pour obtenir votre URL publique',
    'Ouvrez <font name="Courier">https://VOTRE-URL/api/status</font> — vous devriez voir du JSON avec <b>height</b> qui augmente toutes les ~6 secondes',
],
railway_vars_code = (
    '# Railway définit DATABASE_URL automatiquement si PostgreSQL est dans le même projet\n'
    'RELAYER_PRIVATE_KEY    = 0xVOTRE_CLE_PRIVEE\n'
    'RELAYER_ADDRESS        = 0xVOTRE_ADRESSE_WALLET_NOEUD\n'
    'NODE_OPERATOR_WALLET   = 0xVOTRE_WALLET_HUMAIN\n'
    '# PEER_SECRET n\'est plus obligatoire — l\'authentification est automatique\n'
    'SELF_URL               = https://VOTRE-DOMAINE-RAILWAY.up.railway.app\n'
    'PRIMARY_NODE_URL       = https://aequitas.digital'
),

docker_title = 'Étape 2b — Alternative : déploiement avec Docker (Avancé)',
docker_intro = 'Utilisez ceci si vous avez votre propre serveur (VPS, serveur personnel, VM cloud). Nécessite Docker et une base de données PostgreSQL.',
docker_code  = (
    '# 1. Téléchargez le code\n'
    'git clone https://github.com/hanoi96international-gif/Aequitas && cd Aequitas\n\n'
    '# 2. Construisez l\'image du nœud (~3 min de compilation Go)\n'
    'docker build -t aequitas-node .\n\n'
    '# 3. Démarrez le nœud\n'
    'docker run -d --name aequitas-node --restart unless-stopped \\\n'
    '  -e DATABASE_URL="postgres://user:pass@host:5432/aequitas" \\\n'
    '  -e RELAYER_PRIVATE_KEY="0xVOTRE_CLE_PRIVEE" \\\n'
    '  -e RELAYER_ADDRESS="0xVOTRE_ADRESSE_WALLET_NOEUD" \\\n'
    '  -e NODE_OPERATOR_WALLET="0xVOTRE_WALLET_HUMAIN" \\\n'
    '  # -e PEER_SECRET="..." (optionnel/legacy, non requis) \\\n'
    '  -e SELF_URL="https://VOTRE-URL-PUBLIQUE" \\\n'
    '  -e PRIMARY_NODE_URL="https://aequitas.digital" \\\n'
    '  -p 8080:8080 aequitas-node\n\n'
    '# 4. Observez les logs en direct\n'
    'docker logs -f aequitas-node'
),

verify_title = 'Étape 3 — Vérifiez que votre nœud fonctionne',
verify_body  = 'Ouvrez ces URL dans votre navigateur. Remplacez VOTRE-URL-DE-NOEUD par votre domaine Railway réel ou l\'adresse de votre serveur.',
verify_code  = (
    'https://VOTRE-URL-DE-NOEUD/api/status\n'
    ' → Attendu : {"height": 1234, "total_humans": N, "aequitas_index": N}\n\n'
    'https://VOTRE-URL-DE-NOEUD/rpc\n'
    ' → Attendu : {"jsonrpc":"2.0","error":"method not specified"} — le RPC est actif'
),
verify_note  = 'La hauteur de bloc devrait correspondre au nœud principal à 1–2 blocs près, en quelques secondes après le démarrage. Si elle reste à 0, vérifiez que PRIMARY_NODE_URL=https://aequitas.digital est défini et accessible.',

valkey_title = 'Étape 3b — Enregistrez votre clé de validateur (Authentification décentralisée)',
valkey_body  = 'Au lieu d\'un PEER_SECRET partagé, enregistrez la clé de signature de votre nœud avec votre wallet humain. Cela prouve cryptographiquement que vous contrôlez les deux clés. Obtenez la signature en exécutant ceci sur votre serveur (SSH/Railway shell) :',
valkey_code  = 'curl "http://localhost:8080/api/sign-validator-challenge?wallet=0xVOTRE_WALLET_HUMAIN"',
valkey_note  = 'Utilisez ensuite l\'onglet Network → Run a Node du site et cliquez sur "Sign with MetaMask & Register" pour terminer l\'enregistrement.',

mm_title = 'Étape 4 — Connecter MetaMask à votre nœud (Optionnel)',
mm_body  = 'Dans MetaMask : cliquez sur le menu déroulant des réseaux → Add network → Add a network manually, puis saisissez :',
mm_rows  = [
    ('Network Name',    'Aequitas Chain'),
    ('RPC URL',         'https://VOTRE-URL-DE-NOEUD/rpc'),
    ('Chain ID',        '1926'),
    ('Currency Symbol', 'AEQ'),
    ('Decimals',        '18'),
    ('Block Explorer',  'https://aequitas.digital'),
],

rewards_title = 'Étape 5 — Obtenir des récompenses de validateur',
rewards_box   = 'Le Pool des Validateurs collecte 40 % de tous les frais de protocole (frais de swap, démurrage, excédent du plafond de richesse). Chaque jour à 20h00 heure de Berlin (CEST/CET, gère automatiquement l\'heure d\'été) le nœud distribue le solde du pool à tous les opérateurs de nœud enregistrés, proportionnellement aux blocs produits. Plus votre nœud fonctionne de façon constante, plus votre part est grande.',
rewards_steps = [
    'Assurez-vous d\'être enregistré comme humain sur Aequitas. Si non : installez l\'application Android et terminez d\'abord l\'enregistrement biométrique. Vous recevrez une adresse de wallet et 1 000 AEQ.',
    'Définissez <font name="Courier">NODE_OPERATOR_WALLET</font> = votre adresse de wallet humain Aequitas dans vos Variables Railway',
    'Enregistrez — Railway redéploie automatiquement. Avec Docker : <font name="Courier">docker restart aequitas-node</font>',
    'Dans les logs de votre nœud, confirmez : <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    'Les récompenses sont distribuées automatiquement chaque jour à 20h00 heure de Berlin (CEST/CET). Laissez simplement votre nœud fonctionner — aucune autre action n\'est nécessaire.',
],

trouble_title = 'Résolution de problèmes',
trouble_cols  = ['Symptôme', 'Cause probable', 'Solution'],
trouble_rows  = [
    ('La hauteur de bloc reste à 0',      'PRIMARY_NODE_URL non défini ou incorrect', 'Définissez PRIMARY_NODE_URL=https://aequitas.digital et redéployez. Définissez aussi SELF_URL avec l\'URL publique de votre nœud.'),
    ('Erreur DATABASE_URL au démarrage',  'Mauvaise chaîne de connexion',             'Vérifiez le format : postgres://user:pass@host:5432/dbname — assurez-vous que PostgreSQL fonctionne et est accessible.'),
    ('"no code at address" dans les logs','Le contrat V7 n\'est pas encore déployé', 'Normal au premier démarrage — le nœud déploie V7 automatiquement. Attendez quelques secondes et vérifiez à nouveau.'),
    ('"NODE_OPERATOR_WALLET not set"',    'Variable d\'environnement manquante',     'Ajoutez NODE_OPERATOR_WALLET=0xVOTRE_WALLET_HUMAIN. Le nœud fonctionne sans, mais vous ne recevrez pas de récompenses.'),
    ('"Application error" sur Railway',   'Échec de build ou de démarrage',           'Vérifiez les Deploy Logs. Le plus fréquent : DATABASE_URL manquant ou RELAYER_PRIVATE_KEY au mauvais format (doit commencer par 0x).'),
    ('Port 8080 inaccessible (Docker)',   'Configuration du firewall ou du fournisseur cloud', 'Ouvrez le port TCP 8080 entrant dans votre firewall ou les paramètres de groupe de sécurité cloud.'),
    ('Le build Docker échoue (erreur de module)', 'Pas d\'accès internet pendant le build', 'Le build Docker nécessite un accès internet sortant pour télécharger les modules Go. Railway gère cela automatiquement.'),
],

footer = 'Aequitas Chain · Chain ID 1926 · aequitas.digital · Récompenses de validateur : quotidiennes à 20h00 heure de Berlin (CEST/CET)',
)

IT = dict(
title    = 'GUIDA PER OPERATORI DI NODO AEQUITAS',
version  = 'v1.0 · Giugno 2026 · aequitas.digital',
tagline  = 'Guida completa passo dopo passo · Nessuna esperienza blockchain richiesta · ~20–30 min',

prereq_title = 'Prima di iniziare — Cosa ti serve',
prereqs = [
    ('1.', '<b>Un account Aequitas:</b> Devi prima registrarti come essere umano su Aequitas. Installa l\'app Android, completa la registrazione biometrica e annota il tuo indirizzo wallet. Senza questo non puoi ricevere ricompense da validatore.'),
    ('2.', '<b>Un account GitHub (gratuito):</b> Vai su github.com e crea un account gratuito. Ti serve per copiare (fork) il codice di Aequitas affinché Railway possa distribuirlo.'),
    ('3.', '<b>Un account Railway (gratuito):</b> Vai su railway.app e accedi con GitHub. Railway è una piattaforma di hosting che esegue il tuo nodo nel cloud — nessun server o riga di comando richiesta.'),
    ('4.', '<b>Chiave di firma del nodo (RELAYER_PRIVATE_KEY):</b> Il tuo nodo ha bisogno di un wallet Ethereum dedicato per firmare le registrazioni on-chain. Può essere qualsiasi wallet MetaMask. Esporta la chiave privata: MetaMask → Dettagli account → Mostra chiave privata → inserisci la password → copia. Mantienila strettamente privata. <b>IMPORTANTE:</b> Per ricevere ricompense da validatore, NODE_OPERATOR_WALLET deve essere il tuo <b>wallet umano registrato su Aequitas</b> (quello verificato con AequitasBio). Solo gli umani verificati possono guadagnare ricompense da validatore.'),
    ('5.', '<b>10–30 minuti del tuo tempo.</b> Railway svolge la maggior parte del lavoro automaticamente.'),
],

vars_title = 'Passo 1 — Variabili d\'ambiente',
vars_warn  = 'Avviso di sicurezza: La tua RELAYER_PRIVATE_KEY è come una password principale. Chiunque la possieda controlla il wallet del tuo nodo. Non condividerla mai pubblicamente, non incollarla mai in chat o e-mail. Usa un wallet MetaMask separato per RELAYER_PRIVATE_KEY (firma). NODE_OPERATOR_WALLET (per le ricompense) deve essere il tuo wallet umano registrato su Aequitas.',
var_cols   = ['Variabile', 'Obbligatoria?', 'Cosa impostare'],
vars = [
    ('DATABASE_URL',        'SÌ',          'La tua stringa di connessione PostgreSQL. Su Railway: iniettata automaticamente se PostgreSQL è nello stesso progetto. Formato: postgres://user:pass@host:5432/dbname'),
    ('RELAYER_PRIVATE_KEY', 'SÌ',          'La chiave privata (0x…, 66 caratteri) del tuo wallet di nodo dedicato. MetaMask: Dettagli account → Mostra chiave privata → inserisci password → copia.'),
    ('RELAYER_ADDRESS',     'Consigliata',  'L\'indirizzo wallet (0x…, 42 caratteri) corrispondente a RELAYER_PRIVATE_KEY. Copialo da MetaMask. Esiste un fallback, ma impostarlo esplicitamente evita errori di avvio.'),
    ('NODE_OPERATOR_WALLET','Per ricompense', 'Il tuo indirizzo wallet umano Aequitas — registrato tramite l\'app Android. Riceve le tue ricompense quotidiane da validatore (40% di tutte le commissioni di protocollo). Deve essere un umano registrato.'),
    ('NODE_OPERATOR_BINDING_SIGNATURE', 'Per ricompense', 'Dimostra che possiedi NODE_OPERATOR_WALLET. Generala su aequitas.digital/node-binding: firma il messaggio mostrato con il tuo wallet umano in MetaMask, incolla qui la firma risultante. Senza di essa il nodo funziona comunque, ma non può registrarsi automaticamente per le ricompense.'),
    ('PEER_SECRET',         'Opzionale/Legacy', 'Meccanismo di fallback ereditato. Non più obbligatorio — i nodi si autenticano automaticamente tramite una sfida-risposta crittografica (RELAYER_PRIVATE_KEY). Necessario solo per compatibilità con vecchi deployment.'),
    ('SELF_URL',            'Multi-nodo',  'L\'URL HTTPS pubblico del tuo nodo (es. https://mio-nodo.up.railway.app). Necessario per autoescludersi nella scoperta dei peer. Si trova in Railway: Settings → Networking → Public Networking.'),
    ('PRIMARY_NODE_URL',    'Multi-nodo',  'Impostare su: https://aequitas.digital — il nodo primario presso cui il tuo nodo si registra per la scoperta automatica dei peer. All\'avvio il tuo nodo invia il suo URL + indirizzo di firma al primario, riceve l\'intera lista di peer e si unisce automaticamente alla rete.'),
    ('BOOTSTRAP_SNAPSHOT_URL', 'Consigliata', 'Impostare su: https://aequitas.digital/api/snapshot — permette a un nodo nuovo di partire dallo stato attuale della rete invece di rigiocare l\'intera storia dalla genesi. Prima sincronizzazione molto più rapida.'),
    ('BOOTSTRAP_SIGNER',    'Con snapshot', 'L\'indirizzo di firma del nodo primario, usato per verificare che lo snapshot sia genuino prima dell\'importazione. Ottieni il valore attuale da https://aequitas.digital/api/status → "signing_address". Obbligatoria quando BOOTSTRAP_SNAPSHOT_URL è impostata.'),
    ('SNAPSHOT_TOKEN',      'Opzionale',   'Non necessaria per avviare un nodo nuovo — anche senza ottieni tutto il necessario per funzionare correttamente (account, saldi, pool, config). Sblocca solo l\'esportazione completa (collegamento nullifier/wallet + bio_registrations), usata per una resincronizzazione autoritativa di un nodo già divergente. Chiedi all\'operatore di rete solo se ne hai davvero bisogno.'),
    ('RESYNC_FROM_SNAPSHOT', 'Solo recupero', 'PERICOLOSO, temporaneo: impostare su true insieme a BOOTSTRAP_SNAPSHOT_URL e BOOTSTRAP_SIGNER solo per recuperare un nodo il cui stato è divergente dalla rete. Sostituisce completamente lo stato locale. Riavvia una volta, poi rimuovi questa variabile — lasciarla forza una resincronizzazione completa a ogni riavvio.'),
    ('AUTO_HEAL_ON_DIVERGENCE', 'Fortemente consigliata', 'Importante per la sicurezza e la velocità della rete: se la chain del tuo nodo dovesse mai divergere dalla rete (ad es. dopo un\'interruzione o un riavvio fallito), lo rileva da solo — confrontandosi con PRIMARY_NODE_URL ogni pochi minuti — e si risincronizza automaticamente, senza bisogno di RESYNC_FROM_SNAPSHOT manuale. Un nodo che resta divergente e continua a diffondere i propri blocchi può rallentare o destabilizzare l\'intera rete per tutti gli altri operatori. Impostare su true insieme a PRIMARY_NODE_URL, BOOTSTRAP_SNAPSHOT_URL e BOOTSTRAP_SIGNER.'),
    ('PORT',                'No',          'Non impostarla su Railway — Railway la configura automaticamente. Il valore predefinito è 8080.'),
    ('NODE_KEY',            'No',          'Chiave libp2p in Base64 per un\'identità P2P stabile. Auto-generata se omessa, ma cambia a ogni riavvio. Se non impostata, il nodo la stampa su stderr: "SAVE THIS AS NODE_KEY ENVIRONMENT VAR: <base64>". Copiala e incollala qui.'),
    ('IS_PRIMARY_NODE',     'No',          'Lasciare non impostata o false. La distribuzione ora usa un lock a livello di database — qualsiasi nodo può eseguirla senza questa variabile.'),
    ('RESET_STATE',         'No',          'PERICOLOSO: impostare questo su true elimina tutto il database a ogni riavvio. Solo per sviluppo. Mai in produzione.'),
],

railway_title = 'Passo 2 — Distribuire su Railway (Consigliato)',
railway_intro = 'Railway è il modo più semplice per eseguire il tuo nodo — nessuna configurazione server, nessuna riga di comando. Il piano gratuito copre tutti i requisiti. Tempo totale: circa 10–15 minuti.',
railway_steps = [
    'Fai fork di github.com/hanoi96international-gif/Aequitas sul tuo account GitHub (clicca <b>Fork</b> → <b>Create fork</b>)',
    'Su railway.app, accedi con GitHub, poi <b>New Project</b> → <b>+ New</b> → <b>Database</b> → <b>Add PostgreSQL</b>',
    'Nello stesso progetto Railway, clicca <b>+ New → GitHub Repo</b> e seleziona il tuo fork di Aequitas — Railway rileva automaticamente il Dockerfile',
    'Clicca <b>Deploy Now</b> — parte una prima build (può fallire senza env var, è normale)',
    'Clicca sul tuo servizio Aequitas → <b>Variables</b> → aggiungi ogni variabile (vedi tabella sopra). Minimo richiesto: RELAYER_PRIVATE_KEY, RELAYER_ADDRESS, NODE_OPERATOR_WALLET, SELF_URL, PRIMARY_NODE_URL=https://aequitas.digital (PEER_SECRET non è più obbligatoria)',
    'Clicca <b>Deploy</b> (o salva le variabili per attivare l\'auto-redeploy). La build richiede ~3 minuti mentre Go compila il binario del nodo.',
    'Osserva i <b>Deploy Logs</b>. Il successo appare così: <font name="Courier" color="#5B21B6">Aequitas Node Running</font> e <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    'Vai su <b>Settings → Networking → Generate Domain</b> per ottenere il tuo URL pubblico',
    'Apri <font name="Courier">https://TUO-URL/api/status</font> — dovresti vedere JSON con <b>height</b> che sale ogni ~6 secondi',
],
railway_vars_code = (
    '# Railway imposta DATABASE_URL automaticamente se PostgreSQL è nello stesso progetto\n'
    'RELAYER_PRIVATE_KEY    = 0xTUA_CHIAVE_PRIVATA\n'
    'RELAYER_ADDRESS        = 0xTUO_INDIRIZZO_WALLET_NODO\n'
    'NODE_OPERATOR_WALLET   = 0xTUO_WALLET_UMANO\n'
    '# PEER_SECRET non è più obbligatoria — l\'autenticazione è automatica\n'
    'SELF_URL               = https://TUO-DOMINIO-RAILWAY.up.railway.app\n'
    'PRIMARY_NODE_URL       = https://aequitas.digital'
),

docker_title = 'Passo 2b — Alternativa: distribuzione con Docker (Avanzato)',
docker_intro = 'Usa questo se hai un tuo server (VPS, server domestico, VM cloud). Richiede Docker e un database PostgreSQL.',
docker_code  = (
    '# 1. Scarica il codice\n'
    'git clone https://github.com/hanoi96international-gif/Aequitas && cd Aequitas\n\n'
    '# 2. Costruisci l\'immagine del nodo (~3 min di compilazione Go)\n'
    'docker build -t aequitas-node .\n\n'
    '# 3. Avvia il nodo\n'
    'docker run -d --name aequitas-node --restart unless-stopped \\\n'
    '  -e DATABASE_URL="postgres://user:pass@host:5432/aequitas" \\\n'
    '  -e RELAYER_PRIVATE_KEY="0xTUA_CHIAVE_PRIVATA" \\\n'
    '  -e RELAYER_ADDRESS="0xTUO_INDIRIZZO_WALLET_NODO" \\\n'
    '  -e NODE_OPERATOR_WALLET="0xTUO_WALLET_UMANO" \\\n'
    '  # -e PEER_SECRET="..." (opzionale/legacy, non richiesto) \\\n'
    '  -e SELF_URL="https://TUO-URL-PUBBLICO" \\\n'
    '  -e PRIMARY_NODE_URL="https://aequitas.digital" \\\n'
    '  -p 8080:8080 aequitas-node\n\n'
    '# 4. Osserva i log in diretta\n'
    'docker logs -f aequitas-node'
),

verify_title = 'Passo 3 — Verifica che il tuo nodo sia in funzione',
verify_body  = 'Apri questi URL nel browser. Sostituisci TUO-URL-NODO con il tuo dominio Railway reale o l\'indirizzo del server.',
verify_code  = (
    'https://TUO-URL-NODO/api/status\n'
    ' → Atteso: {"height": 1234, "total_humans": N, "aequitas_index": N}\n\n'
    'https://TUO-URL-NODO/rpc\n'
    ' → Atteso: {"jsonrpc":"2.0","error":"method not specified"} — l\'RPC è attivo'
),
verify_note  = 'L\'altezza dei blocchi dovrebbe corrispondere al nodo primario entro 1–2 blocchi nel giro di pochi secondi dall\'avvio. Se resta a 0, verifica che PRIMARY_NODE_URL=https://aequitas.digital sia impostata e raggiungibile.',

valkey_title = 'Passo 3b — Registra la tua chiave di validatore (Autenticazione decentralizzata)',
valkey_body  = 'Invece di un PEER_SECRET condiviso, registra la chiave di firma del tuo nodo con il tuo wallet umano. Questo dimostra crittograficamente che controlli entrambe le chiavi. Ottieni la firma eseguendo questo sul tuo server (SSH/Railway shell):',
valkey_code  = 'curl "http://localhost:8080/api/sign-validator-challenge?wallet=0xTUO_WALLET_UMANO"',
valkey_note  = 'Poi usa la scheda Network → Run a Node del sito e clicca "Sign with MetaMask & Register" per completare la registrazione.',

mm_title = 'Passo 4 — Collega MetaMask al tuo nodo (Opzionale)',
mm_body  = 'In MetaMask: clicca il menu a tendina delle reti → Add network → Add a network manually, poi inserisci:',
mm_rows  = [
    ('Network Name',    'Aequitas Chain'),
    ('RPC URL',         'https://TUO-URL-NODO/rpc'),
    ('Chain ID',        '1926'),
    ('Currency Symbol', 'AEQ'),
    ('Decimals',        '18'),
    ('Block Explorer',  'https://aequitas.digital'),
],

rewards_title = 'Passo 5 — Ottenere ricompense da validatore',
rewards_box   = 'Il Validators Pool raccoglie il 40% di tutte le commissioni di protocollo (commissioni di swap, demurrage, eccedenza del tetto di ricchezza). Ogni giorno alle 20:00 ora di Berlino (CEST/CET, gestisce automaticamente l\'ora legale) il nodo distribuisce il saldo del pool a tutti gli operatori di nodo registrati proporzionalmente ai blocchi prodotti. Più a lungo il tuo nodo resta attivo, maggiore sarà la tua quota.',
rewards_steps = [
    'Assicurati di essere registrato come umano su Aequitas. Se non lo sei: installa l\'app Android e completa prima la registrazione biometrica. Riceverai un indirizzo wallet e 1.000 AEQ.',
    'Imposta <font name="Courier">NODE_OPERATOR_WALLET</font> = il tuo indirizzo wallet umano Aequitas nelle Variables di Railway',
    'Salva — Railway redistribuisce automaticamente. Con Docker: <font name="Courier">docker restart aequitas-node</font>',
    'Nei log del tuo nodo, conferma: <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    'Le ricompense vengono distribuite automaticamente ogni giorno alle 20:00 ora di Berlino (CEST/CET). Lascia semplicemente il nodo in funzione — nessuna ulteriore azione richiesta.',
],

trouble_title = 'Risoluzione dei problemi',
trouble_cols  = ['Sintomo', 'Causa probabile', 'Soluzione'],
trouble_rows  = [
    ('L\'altezza dei blocchi resta a 0',  'PRIMARY_NODE_URL non impostata o errata',  'Imposta PRIMARY_NODE_URL=https://aequitas.digital e ridistribuisci. Imposta anche SELF_URL con l\'URL pubblico del tuo nodo.'),
    ('Errore DATABASE_URL all\'avvio',    'Stringa di connessione errata',            'Verifica il formato: postgres://user:pass@host:5432/dbname — assicurati che PostgreSQL sia in esecuzione e raggiungibile.'),
    ('"no code at address" nei log',      'Il contratto V7 non è ancora distribuito', 'Normale al primo avvio — il nodo distribuisce V7 automaticamente. Attendi qualche secondo e verifica di nuovo.'),
    ('"NODE_OPERATOR_WALLET not set"',    'Variabile d\'ambiente mancante',            'Aggiungi NODE_OPERATOR_WALLET=0xTUO_WALLET_UMANO. Il nodo funziona senza, ma non riceverai ricompense.'),
    ('"Application error" su Railway',    'Errore di build o avvio',                  'Controlla i Deploy Logs. Più comune: DATABASE_URL mancante o RELAYER_PRIVATE_KEY in formato errato (deve iniziare con 0x).'),
    ('Porta 8080 non raggiungibile (Docker)', 'Configurazione firewall o provider cloud', 'Apri la porta TCP 8080 in entrata nel tuo firewall o nelle impostazioni del security group cloud.'),
    ('La build Docker fallisce (errore modulo)', 'Nessuna connessione internet durante la build', 'La build Docker richiede internet in uscita per scaricare i moduli Go. Railway lo gestisce automaticamente.'),
],

footer = 'Aequitas Chain · Chain ID 1926 · aequitas.digital · Ricompense da validatore: quotidiane alle 20:00 ora di Berlino (CEST/CET)',
)

PT = dict(
title    = 'GUIA DO OPERADOR DE NÓ AEQUITAS',
version  = 'v1.0 · Junho de 2026 · aequitas.digital',
tagline  = 'Guia completo passo a passo · Nenhuma experiência prévia com blockchain necessária · ~20–30 min',

prereq_title = 'Antes de começar — O que você precisa',
prereqs = [
    ('1.', '<b>Uma conta Aequitas:</b> Você precisa primeiro estar registrado como humano na Aequitas. Instale o app Android, conclua o registro biométrico e anote seu endereço de wallet. Sem isso, você não pode receber recompensas de validador.'),
    ('2.', '<b>Uma conta GitHub (gratuita):</b> Acesse github.com e crie uma conta gratuita. Você precisa dela para copiar (fork) o código da Aequitas para que o Railway possa implantá-lo.'),
    ('3.', '<b>Uma conta Railway (gratuita):</b> Acesse railway.app e faça login com GitHub. O Railway é uma plataforma de hospedagem que executa seu nó na nuvem — nenhum servidor ou linha de comando necessário.'),
    ('4.', '<b>Chave de assinatura do nó (RELAYER_PRIVATE_KEY):</b> Seu nó precisa de uma wallet Ethereum dedicada para assinar os registros on-chain. Pode ser qualquer wallet MetaMask. Exporte a chave privada: MetaMask → Detalhes da conta → Mostrar chave privada → digite a senha → copie. Mantenha estritamente privada. <b>IMPORTANTE:</b> Para receber recompensas de validador, NODE_OPERATOR_WALLET também precisa ser sua <b>wallet humana registrada na Aequitas</b> (a verificada com AequitasBio). Apenas humanos verificados podem ganhar recompensas de validador.'),
    ('5.', '<b>10–30 minutos do seu tempo.</b> O Railway faz a maior parte do trabalho automaticamente.'),
],

vars_title = 'Passo 1 — Variáveis de ambiente',
vars_warn  = 'Aviso de segurança: Sua RELAYER_PRIVATE_KEY é como uma senha master. Quem a tiver controla a wallet do seu nó. Nunca a compartilhe publicamente, nunca a cole em chat ou e-mail. Use uma wallet MetaMask separada para RELAYER_PRIVATE_KEY (assinatura). NODE_OPERATOR_WALLET (para recompensas) deve ser sua wallet humana registrada na Aequitas.',
var_cols   = ['Variável', 'Obrigatória?', 'O que definir'],
vars = [
    ('DATABASE_URL',        'SIM',         'Sua string de conexão PostgreSQL. No Railway: injetada automaticamente quando o PostgreSQL está no mesmo projeto. Formato: postgres://user:pass@host:5432/dbname'),
    ('RELAYER_PRIVATE_KEY', 'SIM',         'A chave privada (0x…, 66 caracteres) da sua wallet de nó dedicada. MetaMask: Detalhes da conta → Mostrar chave privada → digite a senha → copie.'),
    ('RELAYER_ADDRESS',     'Recomendado', 'O endereço da wallet (0x…, 42 caracteres) correspondente à RELAYER_PRIVATE_KEY. Copie do MetaMask. Existe um fallback, mas defini-lo explicitamente evita erros de inicialização.'),
    ('NODE_OPERATOR_WALLET','Para recompensas', 'Seu endereço de wallet humana Aequitas — registrado via app Android. Recebe suas recompensas diárias de validador (40% de todas as taxas de protocolo). Deve ser um humano registrado.'),
    ('NODE_OPERATOR_BINDING_SIGNATURE', 'Para recompensas', 'Comprova que você é proprietário de NODE_OPERATOR_WALLET. Gere em aequitas.digital/node-binding: assine a mensagem exibida com sua wallet humana no MetaMask, cole aqui a assinatura resultante. Sem ela seu nó ainda funciona, mas não pode se registrar automaticamente para recompensas.'),
    ('PEER_SECRET',         'Opcional/Legado', 'Mecanismo de fallback herdado. Não é mais obrigatório — os nós se autenticam automaticamente via um desafio-resposta criptográfico (RELAYER_PRIVATE_KEY). Necessário apenas para compatibilidade com implantações antigas.'),
    ('SELF_URL',            'Multi-nó',    'A URL HTTPS pública do seu próprio nó (ex. https://meu-no.up.railway.app). Necessária para autoexclusão na descoberta de pares. Encontre no Railway: Settings → Networking → Public Networking.'),
    ('PRIMARY_NODE_URL',    'Multi-nó',    'Defina como: https://aequitas.digital — o nó primário com o qual seu nó se registra para descoberta automática de pares. Na inicialização, seu nó envia sua URL + endereço de assinatura ao primário, recebe a lista completa de pares e entra na rede automaticamente.'),
    ('BOOTSTRAP_SNAPSHOT_URL', 'Recomendado', 'Defina como: https://aequitas.digital/api/snapshot — permite que um nó novo comece a partir do estado atual da rede em vez de reproduzir todo o histórico desde o gênesis. Sincronização inicial muito mais rápida.'),
    ('BOOTSTRAP_SIGNER',    'Com snapshot', 'O endereço de assinatura do nó primário, usado para verificar que o snapshot é genuíno antes de importá-lo. Obtenha o valor atual em https://aequitas.digital/api/status → "signing_address". Obrigatória sempre que BOOTSTRAP_SNAPSHOT_URL estiver definida.'),
    ('SNAPSHOT_TOKEN',      'Opcional',    'Não é necessária para inicializar um nó novo — mesmo sem ela você obtém tudo o que é necessário para funcionar corretamente (contas, saldos, pool, config). Apenas desbloqueia a exportação completa (vínculo nullifier/wallet + bio_registrations), usada para uma resincronização autoritativa de um nó já divergente. Pergunte ao operador da rede apenas se realmente precisar.'),
    ('RESYNC_FROM_SNAPSHOT', 'Somente recuperação', 'PERIGOSO, temporário: defina como true junto com BOOTSTRAP_SNAPSHOT_URL e BOOTSTRAP_SIGNER apenas para recuperar um nó cujo estado divergiu da rede. Substitui completamente o estado local. Reinicie uma vez e depois remova esta variável — deixá-la força uma resincronização completa a cada reinicialização.'),
    ('AUTO_HEAL_ON_DIVERGENCE', 'Altamente recomendado', 'Importante para a segurança e a velocidade da rede: se a chain do seu nó algum dia divergir da rede (por exemplo, após uma queda ou uma reinicialização malsucedida), ele detecta isso sozinho — comparando-se com PRIMARY_NODE_URL a cada poucos minutos — e ressincroniza automaticamente, sem necessidade de RESYNC_FROM_SNAPSHOT manual. Um nó que permanece divergente e continua transmitindo seus próprios blocos pode desacelerar ou desestabilizar toda a rede para os demais operadores. Defina como true junto com PRIMARY_NODE_URL, BOOTSTRAP_SNAPSHOT_URL e BOOTSTRAP_SIGNER.'),
    ('PORT',                'Não',         'Não defina no Railway — o Railway a configura automaticamente. O padrão é 8080.'),
    ('NODE_KEY',            'Não',         'Chave libp2p em Base64 para identidade P2P estável. Gerada automaticamente se omitida, mas muda a cada reinicialização. Se não definida, o nó a imprime em stderr: "SAVE THIS AS NODE_KEY ENVIRONMENT VAR: <base64>". Copie e cole aqui.'),
    ('IS_PRIMARY_NODE',     'Não',         'Deixe não definida ou false. A distribuição agora usa um bloqueio em nível de banco de dados — qualquer nó pode executá-la sem esta variável.'),
    ('RESET_STATE',         'Não',         'PERIGOSO: definir isso como true apaga todo o seu banco de dados a cada reinicialização. Uso apenas para desenvolvimento. Nunca em produção.'),
],

railway_title = 'Passo 2 — Implantar no Railway (Recomendado)',
railway_intro = 'O Railway é a forma mais fácil de executar seu nó — sem configuração de servidor, sem linha de comando. O plano gratuito cobre todos os requisitos. Tempo total: cerca de 10–15 minutos.',
railway_steps = [
    'Faça fork de github.com/hanoi96international-gif/Aequitas para sua própria conta GitHub (clique em <b>Fork</b> → <b>Create fork</b>)',
    'No railway.app, faça login com GitHub, depois <b>New Project</b> → <b>+ New</b> → <b>Database</b> → <b>Add PostgreSQL</b>',
    'No mesmo projeto Railway, clique em <b>+ New → GitHub Repo</b> e selecione seu fork da Aequitas — o Railway detecta o Dockerfile automaticamente',
    'Clique em <b>Deploy Now</b> — um primeiro build começa (pode falhar sem variáveis de ambiente, isso é normal)',
    'Clique no seu serviço Aequitas → <b>Variables</b> → adicione cada variável (veja a tabela acima). Mínimo necessário: RELAYER_PRIVATE_KEY, RELAYER_ADDRESS, NODE_OPERATOR_WALLET, SELF_URL, PRIMARY_NODE_URL=https://aequitas.digital (PEER_SECRET não é mais obrigatória)',
    'Clique em <b>Deploy</b> (ou salve as variáveis para acionar o auto-redeploy). O build leva ~3 minutos enquanto o Go compila o binário do nó.',
    'Observe os <b>Deploy Logs</b>. O sucesso se parece com: <font name="Courier" color="#5B21B6">Aequitas Node Running</font> e <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    'Vá em <b>Settings → Networking → Generate Domain</b> para obter sua URL pública',
    'Abra <font name="Courier">https://SUA-URL/api/status</font> — você deve ver JSON com <b>height</b> subindo a cada ~6 segundos',
],
railway_vars_code = (
    '# O Railway define DATABASE_URL automaticamente se o PostgreSQL estiver no mesmo projeto\n'
    'RELAYER_PRIVATE_KEY    = 0xSUA_CHAVE_PRIVADA\n'
    'RELAYER_ADDRESS        = 0xSEU_ENDERECO_WALLET_NO\n'
    'NODE_OPERATOR_WALLET   = 0xSUA_WALLET_HUMANA\n'
    '# PEER_SECRET não é mais obrigatória — a autenticação é automática\n'
    'SELF_URL               = https://SEU-DOMINIO-RAILWAY.up.railway.app\n'
    'PRIMARY_NODE_URL       = https://aequitas.digital'
),

docker_title = 'Passo 2b — Alternativa: implantação com Docker (Avançado)',
docker_intro = 'Use isso se você tem seu próprio servidor (VPS, servidor doméstico, VM em nuvem). Requer Docker e um banco de dados PostgreSQL.',
docker_code  = (
    '# 1. Baixe o código\n'
    'git clone https://github.com/hanoi96international-gif/Aequitas && cd Aequitas\n\n'
    '# 2. Construa a imagem do nó (~3 min de compilação Go)\n'
    'docker build -t aequitas-node .\n\n'
    '# 3. Inicie o nó\n'
    'docker run -d --name aequitas-node --restart unless-stopped \\\n'
    '  -e DATABASE_URL="postgres://user:pass@host:5432/aequitas" \\\n'
    '  -e RELAYER_PRIVATE_KEY="0xSUA_CHAVE_PRIVADA" \\\n'
    '  -e RELAYER_ADDRESS="0xSEU_ENDERECO_WALLET_NO" \\\n'
    '  -e NODE_OPERATOR_WALLET="0xSUA_WALLET_HUMANA" \\\n'
    '  # -e PEER_SECRET="..." (opcional/legado, não necessário) \\\n'
    '  -e SELF_URL="https://SUA-URL-PUBLICA" \\\n'
    '  -e PRIMARY_NODE_URL="https://aequitas.digital" \\\n'
    '  -p 8080:8080 aequitas-node\n\n'
    '# 4. Observe os logs em tempo real\n'
    'docker logs -f aequitas-node'
),

verify_title = 'Passo 3 — Verifique se seu nó está funcionando',
verify_body  = 'Abra estas URLs no navegador. Substitua SUA-URL-DO-NO pelo seu domínio Railway real ou endereço do servidor.',
verify_code  = (
    'https://SUA-URL-DO-NO/api/status\n'
    ' → Esperado: {"height": 1234, "total_humans": N, "aequitas_index": N}\n\n'
    'https://SUA-URL-DO-NO/rpc\n'
    ' → Esperado: {"jsonrpc":"2.0","error":"method not specified"} — o RPC está ativo'
),
verify_note  = 'A altura do bloco deve corresponder ao nó primário em 1–2 blocos, em poucos segundos após a inicialização. Se ficar em 0, verifique se PRIMARY_NODE_URL=https://aequitas.digital está definida e acessível.',

valkey_title = 'Passo 3b — Registre sua chave de validador (Autenticação descentralizada)',
valkey_body  = 'Em vez de um PEER_SECRET compartilhado, registre a chave de assinatura do seu nó com sua wallet humana. Isso prova criptograficamente que você controla ambas as chaves. Obtenha a assinatura executando isto no seu servidor (SSH/Railway shell):',
valkey_code  = 'curl "http://localhost:8080/api/sign-validator-challenge?wallet=0xSUA_WALLET_HUMANA"',
valkey_note  = 'Depois use a aba Network → Run a Node do site e clique em "Sign with MetaMask & Register" para concluir o registro.',

mm_title = 'Passo 4 — Conectar o MetaMask ao seu nó (Opcional)',
mm_body  = 'No MetaMask: clique no menu suspenso de rede → Add network → Add a network manually e insira:',
mm_rows  = [
    ('Network Name',    'Aequitas Chain'),
    ('RPC URL',         'https://SUA-URL-DO-NO/rpc'),
    ('Chain ID',        '1926'),
    ('Currency Symbol', 'AEQ'),
    ('Decimals',        '18'),
    ('Block Explorer',  'https://aequitas.digital'),
],

rewards_title = 'Passo 5 — Ganhar recompensas de validador',
rewards_box   = 'O Validators Pool coleta 40% de todas as taxas de protocolo (taxas de swap, demurrage, excedente do limite de riqueza). Todos os dias às 20h00 horário de Berlim (CEST/CET, ajusta automaticamente o horário de verão) o nó distribui o saldo do pool a todos os operadores de nó registrados proporcionalmente aos blocos produzidos. Quanto mais tempo seu nó funcionar de forma consistente, maior será sua parte.',
rewards_steps = [
    'Certifique-se de estar registrado como humano na Aequitas. Se não estiver: instale o app Android e conclua primeiro o registro biométrico. Você receberá um endereço de wallet e 1.000 AEQ.',
    'Defina <font name="Courier">NODE_OPERATOR_WALLET</font> = seu endereço de wallet humana Aequitas nas Variables do Railway',
    'Salve — o Railway reimplanta automaticamente. Com Docker: <font name="Courier">docker restart aequitas-node</font>',
    'Nos logs do seu nó, confirme: <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    'As recompensas são distribuídas automaticamente todos os dias às 20h00 horário de Berlim (CEST/CET). Basta manter seu nó em execução — nenhuma ação adicional necessária.',
],

trouble_title = 'Solução de problemas',
trouble_cols  = ['Sintoma', 'Causa provável', 'Solução'],
trouble_rows  = [
    ('A altura do bloco fica em 0',        'PRIMARY_NODE_URL não definida ou incorreta', 'Defina PRIMARY_NODE_URL=https://aequitas.digital e reimplante. Defina também SELF_URL com a URL pública do seu nó.'),
    ('Erro de DATABASE_URL na inicialização', 'String de conexão incorreta',             'Verifique o formato: postgres://user:pass@host:5432/dbname — certifique-se de que o PostgreSQL esteja em execução e acessível.'),
    ('"no code at address" nos logs',      'O contrato V7 ainda não foi implantado',     'Normal na primeira inicialização — o nó implanta V7 automaticamente. Aguarde alguns segundos e verifique novamente.'),
    ('"NODE_OPERATOR_WALLET not set"',     'Variável de ambiente ausente',                'Adicione NODE_OPERATOR_WALLET=0xSUA_WALLET_HUMANA. O nó funciona sem ela, mas você não receberá recompensas.'),
    ('"Application error" no Railway',     'Falha de build ou inicialização',            'Verifique os Deploy Logs. Mais comum: DATABASE_URL ausente ou RELAYER_PRIVATE_KEY em formato errado (deve começar com 0x).'),
    ('Porta 8080 inacessível (Docker)',    'Configuração de firewall ou provedor de nuvem', 'Abra a porta TCP 8080 de entrada no seu firewall ou nas configurações de security group da nuvem.'),
    ('Build do Docker falha (erro de módulo)', 'Sem internet durante o build',            'O build do Docker precisa de internet de saída para baixar os módulos Go. O Railway gerencia isso automaticamente.'),
],

footer = 'Aequitas Chain · Chain ID 1926 · aequitas.digital · Recompensas de validador: diárias às 20h00 horário de Berlim (CEST/CET)',
)

TR = dict(
title    = 'AEQUITAS NODE OPERATÖRÜ KILAVUZU',
version  = 'v1.0 · Haziran 2026 · aequitas.digital',
tagline  = 'Tam adım adım kılavuz · Önceden blockchain deneyimi gerekmez · ~20–30 dk',

prereq_title = 'Başlamadan Önce — İhtiyacınız Olanlar',
prereqs = [
    ('1.', '<b>Bir Aequitas hesabı:</b> Önce Aequitas\'ta insan olarak kayıtlı olmalısınız. Android uygulamasını yükleyin, biyometrik kaydı tamamlayın ve wallet adresinizi not edin. Bu olmadan validator ödülü alamazsınız.'),
    ('2.', '<b>Bir GitHub hesabı (ücretsiz):</b> github.com adresine gidip ücretsiz bir hesap oluşturun. Railway\'in dağıtabilmesi için Aequitas kodunu kopyalamak (fork) için buna ihtiyacınız var.'),
    ('3.', '<b>Bir Railway hesabı (ücretsiz):</b> railway.app adresine gidip GitHub ile giriş yapın. Railway, node\'unuzu bulutta çalıştıran bir hosting platformudur — sunucu veya komut satırı gerekmez.'),
    ('4.', '<b>Node imzalama anahtarı (RELAYER_PRIVATE_KEY):</b> Node\'unuzun zincir üzerindeki kayıtları imzalamak için özel bir Ethereum wallet\'ına ihtiyacı var. Bu herhangi bir MetaMask wallet\'ı olabilir. Özel anahtarını dışa aktarın: MetaMask → Account Details → Show Private Key → şifreyi girin → kopyala. Son derece gizli tutun. <b>ÖNEMLİ:</b> Validator ödülü almak için NODE_OPERATOR_WALLET\'ın da <b>Aequitas\'ta kayıtlı insan wallet\'ınız</b> olması gerekir (AequitasBio ile doğrulanmış olan). Sadece doğrulanmış insanlar validator ödülü kazanabilir.'),
    ('5.', '<b>10–30 dakikanız.</b> Railway işin büyük kısmını otomatik olarak yapar.'),
],

vars_title = 'Adım 1 — Ortam Değişkenleri',
vars_warn  = 'Güvenlik Uyarısı: RELAYER_PRIVATE_KEY\'iniz bir ana şifre gibidir. Ona sahip olan herkes node wallet\'ınızı kontrol eder. Asla herkese açık paylaşmayın, asla sohbete veya e-postaya yapıştırmayın. RELAYER_PRIVATE_KEY (imzalama) için ayrı bir MetaMask wallet\'ı kullanın. NODE_OPERATOR_WALLET (ödüller için) Aequitas\'ta kayıtlı insan wallet\'ınız olmalıdır.',
var_cols   = ['Değişken', 'Gerekli mi?', 'Ne ayarlanmalı'],
vars = [
    ('DATABASE_URL',        'EVET',        'PostgreSQL bağlantı dizeniz. Railway\'de: PostgreSQL aynı projedeyse otomatik olarak eklenir. Format: postgres://user:pass@host:5432/dbname'),
    ('RELAYER_PRIVATE_KEY', 'EVET',        'Özel node wallet\'ınızın özel anahtarı (0x…, 66 karakter). MetaMask: Account Details → Show Private Key → şifreyi girin → kopyala.'),
    ('RELAYER_ADDRESS',     'Önerilir',    'RELAYER_PRIVATE_KEY\'e karşılık gelen wallet adresi (0x…, 42 karakter). MetaMask\'tan kopyalayın. Bir yedek mekanizma var ama bunu açıkça ayarlamak başlatma hatalarını önler.'),
    ('NODE_OPERATOR_WALLET','Ödüller için', 'Android uygulaması üzerinden kayıtlı Aequitas insan wallet adresiniz. Günlük validator ödüllerinizi (tüm protokol ücretlerinin %40\'ı) alır. Kayıtlı bir insan olmalıdır.'),
    ('NODE_OPERATOR_BINDING_SIGNATURE', 'Ödüller için', 'NODE_OPERATOR_WALLET\'a sahip olduğunuzu kanıtlar. aequitas.digital/node-binding adresinde oluşturun: gösterilen mesajı MetaMask\'ta insan wallet\'ınızla imzalayın, ortaya çıkan imzayı buraya yapıştırın. Bu olmadan node\'unuz çalışır ama ödüller için otomatik kayıt olamaz.'),
    ('PEER_SECRET',         'Opsiyonel/Eski', 'Eski paylaşılan-gizli yedek mekanizması. Artık gerekli değil — node\'lar artık kriptografik bir challenge-response (RELAYER_PRIVATE_KEY) ile otomatik olarak kimlik doğrular. Yalnızca eski dağıtımlarla geriye uyumluluk için gereklidir.'),
    ('SELF_URL',            'Çoklu node',  'Kendi node\'unuzun herkese açık HTTPS URL\'si (örn. https://benim-nodum.up.railway.app). Peer keşfinde kendinizi hariç tutmak için gereklidir. Railway\'de bulunur: Settings → Networking → Public Networking.'),
    ('PRIMARY_NODE_URL',    'Çoklu node',  'Şuna ayarlayın: https://aequitas.digital — node\'unuzun otomatik peer keşfi için kaydolduğu birincil node. Başlangıçta node\'unuz URL\'sini + imza adresini birincil node\'a gönderir, tam peer listesini geri alır ve ağa otomatik olarak katılır.'),
    ('BOOTSTRAP_SNAPSHOT_URL', 'Önerilir',  'Şuna ayarlayın: https://aequitas.digital/api/snapshot — yeni bir node\'un tüm geçmişi genesis\'ten yeniden oynatmak yerine ağın güncel durumundan başlamasını sağlar. Çok daha hızlı ilk senkronizasyon.'),
    ('BOOTSTRAP_SIGNER',    'Snapshot ile', 'Snapshot\'ı içe aktarmadan önce gerçek olduğunu doğrulamak için kullanılan birincil node\'un imza adresi. Güncel değeri https://aequitas.digital/api/status → "signing_address" adresinden alın. BOOTSTRAP_SNAPSHOT_URL ayarlandığında zorunludur.'),
    ('SNAPSHOT_TOKEN',      'Opsiyonel',   'Yeni bir node başlatmak için gerekli değildir — onsuz da doğru çalışmak için gereken her şeyi alırsınız (hesaplar, bakiyeler, pool, config). Sadece zaten ayrışmış bir node\'un yetkili resync\'i için kullanılan tam dışa aktarımı (nullifier/wallet bağlantısı + bio_registrations) açar. Sadece gerçekten ihtiyacınız varsa ağ operatöründen isteyin.'),
    ('RESYNC_FROM_SNAPSHOT', 'Yalnızca kurtarma', 'TEHLİKELİ, geçici: yalnızca ağdan ayrışmış durumdaki bir node\'u kurtarmak için BOOTSTRAP_SNAPSHOT_URL ve BOOTSTRAP_SIGNER ile birlikte true olarak ayarlayın. Yerel durumu tamamen değiştirir. Bir kez yeniden başlatın, sonra bu değişkeni kaldırın — bırakılırsa her yeniden başlatmada tam bir resync\'i zorlar.'),
    ('AUTO_HEAL_ON_DIVERGENCE', 'Kesinlikle önerilir', 'Ağın güvenliği ve hızı için önemli: node\'unuzun zinciri ağdan ayrışırsa (örneğin çevrimdışı kaldıktan veya başarısız bir yeniden başlatmadan sonra), bunu kendisi tespit eder — PRIMARY_NODE_URL ile birkaç dakikada bir karşılaştırarak — ve elle RESYNC_FROM_SNAPSHOT gerekmeden otomatik olarak yeniden senkronize olur. Ayrışmış durumda kalıp kendi bloklarını yaymaya devam eden bir node, tüm ağı diğer tüm operatörler için yavaşlatabilir veya istikrarsızlaştırabilir. PRIMARY_NODE_URL, BOOTSTRAP_SNAPSHOT_URL ve BOOTSTRAP_SIGNER ile birlikte true olarak ayarlayın.'),
    ('PORT',                'Hayır',       'Railway\'de ayarlamayın — Railway bunu otomatik olarak ayarlar. Varsayılan 8080\'dir.'),
    ('NODE_KEY',            'Hayır',       'Kararlı bir P2P kimliği için Base64 libp2p anahtarı. Belirtilmezse otomatik oluşturulur ama her yeniden başlatmada değişir. Ayarlanmamışsa node bunu stderr\'e yazdırır: "SAVE THIS AS NODE_KEY ENVIRONMENT VAR: <base64>". Kopyalayıp buraya yapıştırın.'),
    ('IS_PRIMARY_NODE',     'Hayır',       'Ayarlanmamış veya false bırakın. Dağıtım artık veritabanı seviyesinde bir kilit kullanıyor — herhangi bir node bu değişken olmadan çalıştırabilir.'),
    ('RESET_STATE',         'Hayır',       'TEHLİKELİ: Bunu true yapmak her yeniden başlatmada tüm veritabanınızı siler. Yalnızca geliştirme amaçlı. Üretimde asla.'),
],

railway_title = 'Adım 2 — Railway\'de Dağıtım (Önerilir)',
railway_intro = 'Railway, node\'unuzu çalıştırmanın en kolay yoludur — sunucu kurulumu, komut satırı gerekmez. Ücretsiz plan tüm gereksinimleri karşılar. Toplam süre: yaklaşık 10–15 dakika.',
railway_steps = [
    'github.com/hanoi96international-gif/Aequitas\'ı kendi GitHub hesabınıza fork edin (<b>Fork</b> → <b>Create fork</b>\'a tıklayın)',
    'railway.app\'te GitHub ile giriş yapın, sonra <b>New Project</b> → <b>+ New</b> → <b>Database</b> → <b>Add PostgreSQL</b>',
    'Aynı Railway projesinde <b>+ New → GitHub Repo</b>\'ya tıklayın ve Aequitas fork\'unuzu seçin — Railway Dockerfile\'ı otomatik olarak algılar',
    '<b>Deploy Now</b>\'a tıklayın — ilk bir build başlar (env var olmadan başarısız olabilir, bu normaldir)',
    'Aequitas servisinize tıklayın → <b>Variables</b> → her değişkeni ekleyin (yukarıdaki tabloya bakın). Minimum gereken: RELAYER_PRIVATE_KEY, RELAYER_ADDRESS, NODE_OPERATOR_WALLET, SELF_URL, PRIMARY_NODE_URL=https://aequitas.digital (PEER_SECRET artık gerekli değil)',
    '<b>Deploy</b>\'a tıklayın (veya auto-redeploy\'u tetiklemek için değişkenleri kaydedin). Go node binary\'sini derlerken build ~3 dakika sürer.',
    '<b>Deploy Logs</b>\'u izleyin. Başarı şöyle görünür: <font name="Courier" color="#5B21B6">Aequitas Node Running</font> ve <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    'Herkese açık URL\'nizi almak için <b>Settings → Networking → Generate Domain</b>\'a gidin',
    '<font name="Courier">https://URL-NIZ/api/status</font>\'u açın — <b>height</b>\'in her ~6 saniyede yükseldiği JSON görmelisiniz',
],
railway_vars_code = (
    '# PostgreSQL aynı projedeyse Railway DATABASE_URL\'i otomatik ayarlar\n'
    'RELAYER_PRIVATE_KEY    = 0xOZEL_ANAHTARINIZ\n'
    'RELAYER_ADDRESS        = 0xNODE_WALLET_ADRESINIZ\n'
    'NODE_OPERATOR_WALLET   = 0xINSAN_WALLET_INIZ\n'
    '# PEER_SECRET artık gerekli değil — kimlik doğrulama otomatik\n'
    'SELF_URL               = https://RAILWAY-ALAN-ADINIZ.up.railway.app\n'
    'PRIMARY_NODE_URL       = https://aequitas.digital'
),

docker_title = 'Adım 2b — Alternatif: Docker ile Dağıtım (Gelişmiş)',
docker_intro = 'Kendi sunucunuz varsa bunu kullanın (VPS, ev sunucusu, bulut VM). Docker ve bir PostgreSQL veritabanı gerektirir.',
docker_code  = (
    '# 1. Kodu indirin\n'
    'git clone https://github.com/hanoi96international-gif/Aequitas && cd Aequitas\n\n'
    '# 2. Node imajını oluşturun (~3 dk Go derlemesi)\n'
    'docker build -t aequitas-node .\n\n'
    '# 3. Node\'u başlatın\n'
    'docker run -d --name aequitas-node --restart unless-stopped \\\n'
    '  -e DATABASE_URL="postgres://user:pass@host:5432/aequitas" \\\n'
    '  -e RELAYER_PRIVATE_KEY="0xOZEL_ANAHTARINIZ" \\\n'
    '  -e RELAYER_ADDRESS="0xNODE_WALLET_ADRESINIZ" \\\n'
    '  -e NODE_OPERATOR_WALLET="0xINSAN_WALLET_INIZ" \\\n'
    '  # -e PEER_SECRET="..." (opsiyonel/eski, gerekli değil) \\\n'
    '  -e SELF_URL="https://HERKESE-ACIK-URL-NIZ" \\\n'
    '  -e PRIMARY_NODE_URL="https://aequitas.digital" \\\n'
    '  -p 8080:8080 aequitas-node\n\n'
    '# 4. Canlı logları izleyin\n'
    'docker logs -f aequitas-node'
),

verify_title = 'Adım 3 — Node\'unuzun Çalıştığını Doğrulayın',
verify_body  = 'Bu URL\'leri tarayıcınızda açın. NODE-URL-NIZ\'i gerçek Railway alan adınız veya sunucu adresinizle değiştirin.',
verify_code  = (
    'https://NODE-URL-NIZ/api/status\n'
    ' → Beklenen: {"height": 1234, "total_humans": N, "aequitas_index": N}\n\n'
    'https://NODE-URL-NIZ/rpc\n'
    ' → Beklenen: {"jsonrpc":"2.0","error":"method not specified"} — RPC çalışıyor'
),
verify_note  = 'Blok yüksekliği, başlangıçtan sonraki saniyeler içinde birincil node ile 1–2 blok farkla eşleşmelidir. 0\'da kalırsa, PRIMARY_NODE_URL=https://aequitas.digital\'in ayarlandığını ve erişilebilir olduğunu kontrol edin.',

valkey_title = 'Adım 3b — Validator Anahtarınızı Kaydedin (Merkezi Olmayan Kimlik Doğrulama)',
valkey_body  = 'Paylaşılan bir PEER_SECRET yerine, node imzalama anahtarınızı insan wallet\'ınızla kaydedin. Bu, her iki anahtarı da kontrol ettiğinizi kriptografik olarak kanıtlar. Sunucunuzda (SSH/Railway shell) şunu çalıştırarak imzayı alın:',
valkey_code  = 'curl "http://localhost:8080/api/sign-validator-challenge?wallet=0xINSAN_WALLET_INIZ"',
valkey_note  = 'Sonra sitedeki Network → Run a Node sekmesini kullanın ve kaydı tamamlamak için "Sign with MetaMask & Register"\'a tıklayın.',

mm_title = 'Adım 4 — MetaMask\'ı Node\'unuza Bağlayın (Opsiyonel)',
mm_body  = 'MetaMask\'ta: ağ açılır menüsüne tıklayın → Add network → Add a network manually, sonra girin:',
mm_rows  = [
    ('Network Name',    'Aequitas Chain'),
    ('RPC URL',         'https://NODE-URL-NIZ/rpc'),
    ('Chain ID',        '1926'),
    ('Currency Symbol', 'AEQ'),
    ('Decimals',        '18'),
    ('Block Explorer',  'https://aequitas.digital'),
],

rewards_title = 'Adım 5 — Validator Ödülleri Kazanma',
rewards_box   = 'Validators Pool tüm protokol ücretlerinin (swap ücretleri, demurrage, varlık tavanı fazlası) %40\'ını toplar. Her gün Berlin saatiyle 20:00\'de (CEST/CET, yaz saatini otomatik olarak yönetir) node, pool bakiyesini üretilen bloklara oranla tüm kayıtlı node operatörlerine dağıtır. Node\'unuz ne kadar tutarlı çalışırsa, payınız o kadar büyük olur.',
rewards_steps = [
    'Aequitas\'ta insan olarak kayıtlı olduğunuzdan emin olun. Değilseniz: önce Android uygulamasını yükleyip biyometrik kaydı tamamlayın. Bir wallet adresi ve 1.000 AEQ alacaksınız.',
    'Railway Variables\'ınızda <font name="Courier">NODE_OPERATOR_WALLET</font> = Aequitas insan wallet adresinizi ayarlayın',
    'Kaydedin — Railway otomatik olarak yeniden dağıtır. Docker ile: <font name="Courier">docker restart aequitas-node</font>',
    'Node loglarınızda şunu onaylayın: <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    'Ödüller her gün Berlin saatiyle 20:00\'de otomatik olarak dağıtılır (CEST/CET). Sadece node\'unuzu çalışır durumda tutun — başka bir işlem gerekmez.',
],

trouble_title = 'Sorun Giderme',
trouble_cols  = ['Belirti', 'Olası Neden', 'Çözüm'],
trouble_rows  = [
    ('Blok yüksekliği 0\'da kalıyor',      'PRIMARY_NODE_URL ayarlanmamış veya yanlış', 'PRIMARY_NODE_URL=https://aequitas.digital ayarlayın ve yeniden dağıtın. SELF_URL\'i de node\'unuzun herkese açık URL\'siyle ayarlayın.'),
    ('Başlangıçta DATABASE_URL hatası',    'Yanlış bağlantı dizesi',                    'Formatı kontrol edin: postgres://user:pass@host:5432/dbname — PostgreSQL\'in çalıştığından ve erişilebilir olduğundan emin olun.'),
    ('Loglarda "no code at address"',      'V7 sözleşmesi henüz dağıtılmadı',           'İlk başlangıçta normal — node V7\'yi otomatik olarak dağıtır. Birkaç saniye bekleyip tekrar kontrol edin.'),
    ('"NODE_OPERATOR_WALLET not set"',     'Eksik ortam değişkeni',                     'NODE_OPERATOR_WALLET=0xINSAN_WALLET_INIZ ekleyin. Node onsuz çalışır ama ödül almazsınız.'),
    ('Railway "Application error"',       'Build veya başlatma hatası',                 'Deploy Logs\'u kontrol edin. En yaygın: DATABASE_URL eksik veya RELAYER_PRIVATE_KEY yanlış formatta (0x ile başlamalı).'),
    ('Port 8080 erişilemiyor (Docker)',   'Firewall veya bulut sağlayıcı yapılandırması', 'Firewall\'ınızda veya bulut güvenlik grubu ayarlarınızda gelen TCP port 8080\'i açın.'),
    ('Docker build başarısız (modül hatası)', 'Build sırasında internet yok',           'Docker build, Go modüllerini indirmek için giden internet erişimine ihtiyaç duyar. Railway bunu otomatik olarak yönetir.'),
],

footer = 'Aequitas Chain · Chain ID 1926 · aequitas.digital · Validator ödülleri: her gün Berlin saatiyle 20:00 (CEST/CET)',
)

ID = dict(
title    = 'PANDUAN OPERATOR NODE AEQUITAS',
version  = 'v1.0 · Juni 2026 · aequitas.digital',
tagline  = 'Panduan lengkap langkah demi langkah · Tidak perlu pengalaman blockchain sebelumnya · ~20–30 menit',

prereq_title = 'Sebelum Memulai — Yang Anda Butuhkan',
prereqs = [
    ('1.', '<b>Akun Aequitas:</b> Anda harus terdaftar sebagai manusia di Aequitas terlebih dahulu. Instal aplikasi Android, selesaikan registrasi biometrik, dan catat alamat wallet Anda. Tanpa ini Anda tidak dapat menerima hadiah validator.'),
    ('2.', '<b>Akun GitHub (gratis):</b> Buka github.com dan buat akun gratis. Anda memerlukannya untuk menyalin (fork) kode Aequitas agar Railway dapat men-deploy-nya.'),
    ('3.', '<b>Akun Railway (gratis):</b> Buka railway.app dan masuk dengan GitHub. Railway adalah platform hosting yang menjalankan node Anda di cloud — tidak perlu server atau command line.'),
    ('4.', '<b>Kunci penandatanganan node (RELAYER_PRIVATE_KEY):</b> Node Anda membutuhkan wallet Ethereum khusus untuk menandatangani registrasi on-chain. Bisa berupa wallet MetaMask apa pun. Ekspor kunci privatnya: MetaMask → Account Details → Show Private Key → masukkan kata sandi → salin. Jaga kerahasiaannya secara ketat. <b>PENTING:</b> Untuk menerima hadiah validator, NODE_OPERATOR_WALLET juga harus berupa <b>wallet manusia terdaftar Aequitas</b> Anda (yang terverifikasi dengan AequitasBio). Hanya manusia terverifikasi yang dapat memperoleh hadiah validator.'),
    ('5.', '<b>10–30 menit waktu Anda.</b> Railway melakukan sebagian besar pekerjaan secara otomatis.'),
],

vars_title = 'Langkah 1 — Variabel Lingkungan',
vars_warn  = 'Peringatan Keamanan: RELAYER_PRIVATE_KEY Anda seperti kata sandi utama. Siapa pun yang memilikinya mengontrol wallet node Anda. Jangan pernah membagikannya secara publik, jangan pernah menempelkannya di chat atau email. Gunakan wallet MetaMask terpisah untuk RELAYER_PRIVATE_KEY (penandatanganan). NODE_OPERATOR_WALLET (untuk hadiah) harus berupa wallet manusia terdaftar Aequitas Anda.',
var_cols   = ['Variabel', 'Wajib?', 'Apa yang harus diisi'],
vars = [
    ('DATABASE_URL',        'YA',          'String koneksi PostgreSQL Anda. Di Railway: otomatis disuntikkan jika PostgreSQL berada dalam proyek yang sama. Format: postgres://user:pass@host:5432/dbname'),
    ('RELAYER_PRIVATE_KEY', 'YA',          'Kunci privat (0x…, 66 karakter) dari wallet node khusus Anda. MetaMask: Account Details → Show Private Key → masukkan kata sandi → salin.'),
    ('RELAYER_ADDRESS',     'Disarankan',  'Alamat wallet (0x…, 42 karakter) yang sesuai dengan RELAYER_PRIVATE_KEY. Salin dari MetaMask. Ada fallback, tetapi mengaturnya secara eksplisit mencegah error saat startup.'),
    ('NODE_OPERATOR_WALLET','Untuk hadiah', 'Alamat wallet manusia Aequitas Anda — terdaftar melalui aplikasi Android. Menerima hadiah validator harian Anda (40% dari semua biaya protokol). Harus berupa manusia terdaftar.'),
    ('NODE_OPERATOR_BINDING_SIGNATURE', 'Untuk hadiah', 'Membuktikan Anda pemilik NODE_OPERATOR_WALLET. Buat di aequitas.digital/node-binding: tanda tangani pesan yang ditampilkan dengan wallet manusia Anda di MetaMask, tempel tanda tangan yang dihasilkan di sini. Tanpa ini node Anda masih berjalan, tetapi tidak dapat mendaftar otomatis untuk hadiah.'),
    ('PEER_SECRET',         'Opsional/Legacy', 'Mekanisme cadangan lama. Tidak lagi diperlukan — node sekarang mengautentikasi secara otomatis melalui challenge-response kriptografis (RELAYER_PRIVATE_KEY). Hanya diperlukan untuk kompatibilitas mundur dengan deployment lama.'),
    ('SELF_URL',            'Multi-node',  'URL HTTPS publik node Anda sendiri (mis. https://node-saya.up.railway.app). Diperlukan untuk pengecualian diri dalam penemuan peer. Temukan di Railway: Settings → Networking → Public Networking.'),
    ('PRIMARY_NODE_URL',    'Multi-node',  'Atur ke: https://aequitas.digital — node utama tempat node Anda mendaftar untuk penemuan peer otomatis. Saat startup, node Anda mengirim URL + alamat penandatanganannya ke node utama, menerima daftar peer lengkap, dan bergabung ke jaringan secara otomatis.'),
    ('BOOTSTRAP_SNAPSHOT_URL', 'Disarankan', 'Atur ke: https://aequitas.digital/api/snapshot — memungkinkan node baru mulai dari status jaringan saat ini, bukan memutar ulang seluruh riwayat dari genesis. Sinkronisasi awal jauh lebih cepat.'),
    ('BOOTSTRAP_SIGNER',    'Dengan snapshot', 'Alamat penandatanganan node utama, digunakan untuk memverifikasi bahwa snapshot asli sebelum diimpor. Dapatkan nilai saat ini dari https://aequitas.digital/api/status → "signing_address". Wajib jika BOOTSTRAP_SNAPSHOT_URL diatur.'),
    ('SNAPSHOT_TOKEN',      'Opsional',    'Tidak diperlukan untuk bootstrap node baru — tanpanya Anda tetap mendapatkan semua yang diperlukan untuk berjalan dengan benar (akun, saldo, pool, config). Hanya membuka ekspor lengkap (tautan nullifier/wallet + bio_registrations), digunakan untuk resync otoritatif node yang sudah divergen. Tanyakan operator jaringan hanya jika Anda benar-benar membutuhkannya.'),
    ('RESYNC_FROM_SNAPSHOT', 'Hanya recovery', 'BERBAHAYA, sementara: atur ke true bersama BOOTSTRAP_SNAPSHOT_URL dan BOOTSTRAP_SIGNER hanya untuk memulihkan node yang statusnya telah menyimpang dari jaringan. Mengganti status lokal secara total. Restart sekali, lalu hapus kembali variabel ini — membiarkannya memaksa resync penuh setiap restart.'),
    ('AUTO_HEAL_ON_DIVERGENCE', 'Sangat disarankan', 'Penting untuk keamanan dan kecepatan jaringan: jika rantai node Anda pernah menyimpang dari jaringan (misalnya setelah downtime atau restart yang gagal), node akan mendeteksinya sendiri — dengan membandingkan diri terhadap PRIMARY_NODE_URL setiap beberapa menit — dan melakukan resync otomatis, tanpa perlu RESYNC_FROM_SNAPSHOT manual. Node yang tetap menyimpang dan terus menyiarkan bloknya sendiri dapat memperlambat atau mengganggu stabilitas seluruh jaringan bagi operator lain. Atur ke true bersama PRIMARY_NODE_URL, BOOTSTRAP_SNAPSHOT_URL, dan BOOTSTRAP_SIGNER.'),
    ('PORT',                'Tidak',       'Jangan atur di Railway — Railway mengaturnya secara otomatis. Default-nya adalah 8080.'),
    ('NODE_KEY',            'Tidak',       'Kunci libp2p Base64 untuk identitas P2P yang stabil. Otomatis dibuat jika dihilangkan, tetapi berubah setiap restart. Jika tidak diatur, node mencetaknya ke stderr: "SAVE THIS AS NODE_KEY ENVIRONMENT VAR: <base64>". Salin dan tempel di sini.'),
    ('IS_PRIMARY_NODE',     'Tidak',       'Biarkan tidak diatur atau false. Distribusi sekarang menggunakan lock tingkat database — node apa pun dapat menjalankannya tanpa variabel ini.'),
    ('RESET_STATE',         'Tidak',       'BERBAHAYA: mengatur ini ke true menghapus seluruh database Anda setiap restart. Hanya untuk penggunaan pengembangan. Jangan pernah di produksi.'),
],

railway_title = 'Langkah 2 — Deploy di Railway (Disarankan)',
railway_intro = 'Railway adalah cara termudah untuk menjalankan node Anda — tanpa setup server, tanpa command line. Paket gratis mencakup semua kebutuhan. Total waktu: sekitar 10–15 menit.',
railway_steps = [
    'Fork github.com/hanoi96international-gif/Aequitas ke akun GitHub Anda sendiri (klik <b>Fork</b> → <b>Create fork</b>)',
    'Di railway.app, masuk dengan GitHub, lalu <b>New Project</b> → <b>+ New</b> → <b>Database</b> → <b>Add PostgreSQL</b>',
    'Di proyek Railway yang sama, klik <b>+ New → GitHub Repo</b> dan pilih fork Aequitas Anda — Railway mendeteksi Dockerfile secara otomatis',
    'Klik <b>Deploy Now</b> — build pertama dimulai (dapat gagal tanpa env var, itu normal)',
    'Klik layanan Aequitas Anda → <b>Variables</b> → tambahkan setiap variabel (lihat tabel di atas). Minimum yang diperlukan: RELAYER_PRIVATE_KEY, RELAYER_ADDRESS, NODE_OPERATOR_WALLET, SELF_URL, PRIMARY_NODE_URL=https://aequitas.digital (PEER_SECRET tidak lagi diperlukan)',
    'Klik <b>Deploy</b> (atau simpan variabel untuk memicu auto-redeploy). Build memakan waktu ~3 menit saat Go mengompilasi binary node.',
    'Perhatikan <b>Deploy Logs</b>. Keberhasilan terlihat seperti: <font name="Courier" color="#5B21B6">Aequitas Node Running</font> dan <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    'Buka <b>Settings → Networking → Generate Domain</b> untuk mendapatkan URL publik Anda',
    'Buka <font name="Courier">https://URL-ANDA/api/status</font> — Anda akan melihat JSON dengan <b>height</b> yang naik setiap ~6 detik',
],
railway_vars_code = (
    '# Railway otomatis mengatur DATABASE_URL jika PostgreSQL berada dalam proyek yang sama\n'
    'RELAYER_PRIVATE_KEY    = 0xKUNCI_PRIVAT_ANDA\n'
    'RELAYER_ADDRESS        = 0xALAMAT_WALLET_NODE_ANDA\n'
    'NODE_OPERATOR_WALLET   = 0xWALLET_MANUSIA_ANDA\n'
    '# PEER_SECRET tidak lagi diperlukan — autentikasi otomatis\n'
    'SELF_URL               = https://DOMAIN-RAILWAY-ANDA.up.railway.app\n'
    'PRIMARY_NODE_URL       = https://aequitas.digital'
),

docker_title = 'Langkah 2b — Alternatif: Deploy dengan Docker (Lanjutan)',
docker_intro = 'Gunakan ini jika Anda memiliki server sendiri (VPS, server rumah, VM cloud). Membutuhkan Docker dan database PostgreSQL.',
docker_code  = (
    '# 1. Unduh kode\n'
    'git clone https://github.com/hanoi96international-gif/Aequitas && cd Aequitas\n\n'
    '# 2. Bangun image node (~3 menit kompilasi Go)\n'
    'docker build -t aequitas-node .\n\n'
    '# 3. Jalankan node\n'
    'docker run -d --name aequitas-node --restart unless-stopped \\\n'
    '  -e DATABASE_URL="postgres://user:pass@host:5432/aequitas" \\\n'
    '  -e RELAYER_PRIVATE_KEY="0xKUNCI_PRIVAT_ANDA" \\\n'
    '  -e RELAYER_ADDRESS="0xALAMAT_WALLET_NODE_ANDA" \\\n'
    '  -e NODE_OPERATOR_WALLET="0xWALLET_MANUSIA_ANDA" \\\n'
    '  # -e PEER_SECRET="..." (opsional/legacy, tidak diperlukan) \\\n'
    '  -e SELF_URL="https://URL-PUBLIK-ANDA" \\\n'
    '  -e PRIMARY_NODE_URL="https://aequitas.digital" \\\n'
    '  -p 8080:8080 aequitas-node\n\n'
    '# 4. Perhatikan log secara langsung\n'
    'docker logs -f aequitas-node'
),

verify_title = 'Langkah 3 — Verifikasi Node Anda Berjalan',
verify_body  = 'Buka URL ini di browser Anda. Ganti URL-NODE-ANDA dengan domain Railway atau alamat server Anda yang sebenarnya.',
verify_code  = (
    'https://URL-NODE-ANDA/api/status\n'
    ' → Diharapkan: {"height": 1234, "total_humans": N, "aequitas_index": N}\n\n'
    'https://URL-NODE-ANDA/rpc\n'
    ' → Diharapkan: {"jsonrpc":"2.0","error":"method not specified"} — RPC aktif'
),
verify_note  = 'Tinggi blok harus sesuai dengan node utama dalam 1–2 blok dalam beberapa detik setelah startup. Jika tetap di 0, periksa apakah PRIMARY_NODE_URL=https://aequitas.digital sudah diatur dan dapat dijangkau.',

valkey_title = 'Langkah 3b — Daftarkan Kunci Validator Anda (Autentikasi Terdesentralisasi)',
valkey_body  = 'Sebagai pengganti PEER_SECRET bersama, daftarkan kunci penandatanganan node Anda dengan wallet manusia Anda. Ini membuktikan secara kriptografis bahwa Anda mengontrol kedua kunci tersebut. Dapatkan tanda tangan dengan menjalankan ini di server Anda (SSH/Railway shell):',
valkey_code  = 'curl "http://localhost:8080/api/sign-validator-challenge?wallet=0xWALLET_MANUSIA_ANDA"',
valkey_note  = 'Kemudian gunakan tab Network → Run a Node di situs web dan klik "Sign with MetaMask & Register" untuk menyelesaikan registrasi.',

mm_title = 'Langkah 4 — Hubungkan MetaMask ke Node Anda (Opsional)',
mm_body  = 'Di MetaMask: klik dropdown jaringan → Add network → Add a network manually, lalu masukkan:',
mm_rows  = [
    ('Network Name',    'Aequitas Chain'),
    ('RPC URL',         'https://URL-NODE-ANDA/rpc'),
    ('Chain ID',        '1926'),
    ('Currency Symbol', 'AEQ'),
    ('Decimals',        '18'),
    ('Block Explorer',  'https://aequitas.digital'),
],

rewards_title = 'Langkah 5 — Mendapatkan Hadiah Validator',
rewards_box   = 'Validators Pool mengumpulkan 40% dari semua biaya protokol (biaya swap, demurrage, kelebihan batas kekayaan). Setiap hari pukul 20:00 waktu Berlin (CEST/CET, menangani DST secara otomatis) node mendistribusikan saldo pool secara proporsional ke semua operator node terdaftar berdasarkan blok yang diproduksi. Semakin konsisten node Anda berjalan, semakin besar bagian Anda.',
rewards_steps = [
    'Pastikan Anda terdaftar sebagai manusia di Aequitas. Jika belum: instal aplikasi Android dan selesaikan registrasi biometrik terlebih dahulu. Anda akan menerima alamat wallet dan 1.000 AEQ.',
    'Atur <font name="Courier">NODE_OPERATOR_WALLET</font> = alamat wallet manusia Aequitas Anda di Variables Railway Anda',
    'Simpan — Railway redeploy secara otomatis. Dengan Docker: <font name="Courier">docker restart aequitas-node</font>',
    'Di log node Anda, konfirmasi: <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    'Hadiah didistribusikan secara otomatis setiap hari pukul 20:00 waktu Berlin (CEST/CET). Cukup jaga node Anda tetap berjalan — tidak perlu tindakan lebih lanjut.',
],

trouble_title = 'Pemecahan Masalah',
trouble_cols  = ['Gejala', 'Kemungkinan Sebab', 'Solusi'],
trouble_rows  = [
    ('Tinggi blok tetap di 0',          'PRIMARY_NODE_URL tidak diatur atau salah', 'Atur PRIMARY_NODE_URL=https://aequitas.digital dan deploy ulang. Atur juga SELF_URL dengan URL publik node Anda.'),
    ('Error DATABASE_URL saat startup', 'String koneksi salah',                     'Periksa format: postgres://user:pass@host:5432/dbname — pastikan PostgreSQL berjalan dan dapat dijangkau.'),
    ('"no code at address" di log',     'Kontrak V7 belum di-deploy',                'Normal saat startup pertama — node men-deploy V7 secara otomatis. Tunggu beberapa detik dan periksa lagi.'),
    ('"NODE_OPERATOR_WALLET not set"',  'Variabel lingkungan hilang',                'Tambahkan NODE_OPERATOR_WALLET=0xWALLET_MANUSIA_ANDA. Node tetap berjalan tanpanya, tetapi Anda tidak akan menerima hadiah.'),
    ('"Application error" di Railway',  'Kegagalan build atau startup',              'Periksa Deploy Logs. Paling umum: DATABASE_URL hilang atau RELAYER_PRIVATE_KEY format salah (harus dimulai dengan 0x).'),
    ('Port 8080 tidak dapat diakses (Docker)', 'Konfigurasi firewall atau penyedia cloud', 'Buka port TCP 8080 masuk di firewall atau pengaturan security group cloud Anda.'),
    ('Build Docker gagal (error modul)', 'Tidak ada internet saat build',            'Build Docker membutuhkan akses internet keluar untuk mengunduh modul Go. Railway menangani ini secara otomatis.'),
],

footer = 'Aequitas Chain · Chain ID 1926 · aequitas.digital · Hadiah validator: harian pukul 20:00 waktu Berlin (CEST/CET)',
)

RU = dict(
title    = 'РУКОВОДСТВО ОПЕРАТОРА УЗЛА AEQUITAS',
version  = 'v1.0 · Июнь 2026 · aequitas.digital',
tagline  = 'Полное пошаговое руководство · Опыт работы с блокчейном не требуется · ~20–30 мин',

prereq_title = 'Перед началом — что вам нужно',
prereqs = [
    ('1.', '<b>Учётная запись Aequitas:</b> Сначала вы должны зарегистрироваться как человек в Aequitas. Установите приложение Android, завершите биометрическую регистрацию и запишите адрес своего кошелька. Без этого вы не сможете получать награды валидатора.'),
    ('2.', '<b>Учётная запись GitHub (бесплатно):</b> Перейдите на github.com и создайте бесплатную учётную запись. Она нужна для копирования (форка) кода Aequitas, чтобы Railway мог его развернуть.'),
    ('3.', '<b>Учётная запись Railway (бесплатно):</b> Перейдите на railway.app и войдите через GitHub. Railway — это хостинг-платформа, которая запускает ваш узел в облаке — сервер или командная строка не требуются.'),
    ('4.', '<b>Ключ подписи узла (RELAYER_PRIVATE_KEY):</b> Вашему узлу нужен отдельный кошелёк Ethereum для подписи регистраций в блокчейне. Это может быть любой кошелёк MetaMask. Экспортируйте приватный ключ: MetaMask → Account Details → Show Private Key → введите пароль → скопируйте. Храните строго конфиденциально. <b>ВАЖНО:</b> Для получения наград валидатора NODE_OPERATOR_WALLET также должен быть вашим <b>зарегистрированным человеческим кошельком Aequitas</b> (тем, что верифицирован через AequitasBio). Только верифицированные люди могут получать награды валидатора.'),
    ('5.', '<b>10–30 минут вашего времени.</b> Railway выполняет большую часть работы автоматически.'),
],

vars_title = 'Шаг 1 — Переменные окружения',
vars_warn  = 'Предупреждение о безопасности: ваш RELAYER_PRIVATE_KEY — это как мастер-пароль. Любой, у кого он есть, контролирует кошелёк вашего узла. Никогда не делитесь им публично, никогда не вставляйте его в чат или письмо. Используйте отдельный кошелёк MetaMask для RELAYER_PRIVATE_KEY (подпись). NODE_OPERATOR_WALLET (для наград) должен быть вашим зарегистрированным человеческим кошельком Aequitas.',
var_cols   = ['Переменная', 'Обязательно?', 'Что указать'],
vars = [
    ('DATABASE_URL',        'ДА',           'Строка подключения PostgreSQL. На Railway: автоматически добавляется, если PostgreSQL в том же проекте. Формат: postgres://user:pass@host:5432/dbname'),
    ('RELAYER_PRIVATE_KEY', 'ДА',           'Приватный ключ (0x…, 66 символов) вашего отдельного кошелька узла. MetaMask: Account Details → Show Private Key → введите пароль → скопируйте.'),
    ('RELAYER_ADDRESS',     'Рекомендуется', 'Адрес кошелька (0x…, 42 символа), соответствующий RELAYER_PRIVATE_KEY. Скопируйте из MetaMask. Есть резервный механизм, но явное указание предотвращает ошибки запуска.'),
    ('NODE_OPERATOR_WALLET','Для наград', 'Ваш человеческий кошелёк Aequitas — зарегистрированный через приложение Android. Получает ваши ежедневные награды валидатора (40% всех протокольных сборов). Должен быть зарегистрированным человеком.'),
    ('NODE_OPERATOR_BINDING_SIGNATURE', 'Для наград', 'Доказывает, что вы владеете NODE_OPERATOR_WALLET. Создайте на aequitas.digital/node-binding: подпишите показанное сообщение своим человеческим кошельком в MetaMask, вставьте полученную подпись здесь. Без неё ваш узел всё равно работает, но не может автоматически зарегистрироваться для наград.'),
    ('PEER_SECRET',         'Опционально/Устарело', 'Устаревший резервный механизм общего секрета. Больше не требуется — узлы теперь аутентифицируются автоматически через криптографический challenge-response (RELAYER_PRIVATE_KEY). Нужен только для обратной совместимости со старыми развёртываниями.'),
    ('SELF_URL',            'Мультиузел',   'Собственный публичный HTTPS URL вашего узла (напр. https://мой-узел.up.railway.app). Требуется для самоисключения при обнаружении пиров. Найдите в Railway: Settings → Networking → Public Networking.'),
    ('PRIMARY_NODE_URL',    'Мультиузел',   'Установите: https://aequitas.digital — основной узел, у которого регистрируется ваш узел для автоматического обнаружения пиров. При запуске ваш узел отправляет свой URL + адрес подписи основному узлу, получает полный список пиров и автоматически присоединяется к сети.'),
    ('BOOTSTRAP_SNAPSHOT_URL', 'Рекомендуется', 'Установите: https://aequitas.digital/api/snapshot — позволяет новому узлу начать с текущего состояния сети вместо воспроизведения всей истории с генезиса. Значительно более быстрая первая синхронизация.'),
    ('BOOTSTRAP_SIGNER',    'Со снапшотом', 'Адрес подписи основного узла, используемый для проверки подлинности снапшота перед импортом. Получите текущее значение на https://aequitas.digital/api/status → "signing_address". Обязателен, если установлен BOOTSTRAP_SNAPSHOT_URL.'),
    ('SNAPSHOT_TOKEN',      'Опционально',  'Не требуется для загрузки нового узла — без него вы всё равно получаете всё необходимое для корректной работы (аккаунты, балансы, пул, конфигурация). Открывает только полный экспорт (связь nullifier/кошелёк + bio_registrations), используемый для авторитетной ресинхронизации уже разошедшегося узла. Спрашивайте у оператора сети только если это действительно нужно.'),
    ('RESYNC_FROM_SNAPSHOT', 'Только восстановление', 'ОПАСНО, временно: устанавливайте в true только вместе с BOOTSTRAP_SNAPSHOT_URL и BOOTSTRAP_SIGNER для восстановления узла, состояние которого разошлось с сетью. Полностью заменяет локальное состояние. Перезапустите один раз, затем удалите эту переменную снова — если оставить, при каждом перезапуске будет принудительная полная ресинхронизация.'),
    ('AUTO_HEAL_ON_DIVERGENCE', 'Настоятельно рекомендуется', 'Важно для безопасности и скорости сети: если цепочка вашего узла когда-либо разойдётся с сетью (например, после простоя или неудачного перезапуска), узел обнаружит это самостоятельно — сравнивая себя с PRIMARY_NODE_URL каждые несколько минут — и автоматически ресинхронизируется, без ручного RESYNC_FROM_SNAPSHOT. Узел, который остаётся разошедшимся и продолжает рассылать собственные блоки, может замедлить или дестабилизировать всю сеть для всех остальных операторов. Установите в true вместе с PRIMARY_NODE_URL, BOOTSTRAP_SNAPSHOT_URL и BOOTSTRAP_SIGNER.'),
    ('PORT',                'Нет',          'Не устанавливайте на Railway — Railway устанавливает это автоматически. По умолчанию 8080.'),
    ('NODE_KEY',            'Нет',          'Base64 ключ libp2p для стабильной идентичности P2P. Автоматически генерируется, если не указан, но меняется при каждом перезапуске. Если не установлен, узел выводит его в stderr: "SAVE THIS AS NODE_KEY ENVIRONMENT VAR: <base64>". Скопируйте и вставьте здесь.'),
    ('IS_PRIMARY_NODE',     'Нет',          'Оставьте неустановленным или false. Распределение теперь использует блокировку на уровне базы данных — любой узел может его выполнять без этой переменной.'),
    ('RESET_STATE',         'Нет',          'ОПАСНО: установка в true удаляет всю вашу базу данных при каждом перезапуске. Только для разработки. Никогда в продакшене.'),
],

railway_title = 'Шаг 2 — Развёртывание на Railway (Рекомендуется)',
railway_intro = 'Railway — самый простой способ запустить ваш узел — без настройки сервера, без командной строки. Бесплатный тариф покрывает все требования. Общее время: около 10–15 минут.',
railway_steps = [
    'Сделайте форк github.com/hanoi96international-gif/Aequitas в свою учётную запись GitHub (нажмите <b>Fork</b> → <b>Create fork</b>)',
    'На railway.app войдите через GitHub, затем <b>New Project</b> → <b>+ New</b> → <b>Database</b> → <b>Add PostgreSQL</b>',
    'В том же проекте Railway нажмите <b>+ New → GitHub Repo</b> и выберите свой форк Aequitas — Railway автоматически обнаружит Dockerfile',
    'Нажмите <b>Deploy Now</b> — начнётся первая сборка (может завершиться ошибкой без переменных окружения, это нормально)',
    'Нажмите на свой сервис Aequitas → <b>Variables</b> → добавьте каждую переменную (см. таблицу выше). Минимум: RELAYER_PRIVATE_KEY, RELAYER_ADDRESS, NODE_OPERATOR_WALLET, SELF_URL, PRIMARY_NODE_URL=https://aequitas.digital (PEER_SECRET больше не требуется)',
    'Нажмите <b>Deploy</b> (или сохраните переменные для запуска авто-redeploy). Сборка занимает ~3 минуты, пока Go компилирует бинарник узла.',
    'Следите за <b>Deploy Logs</b>. Успех выглядит так: <font name="Courier" color="#5B21B6">Aequitas Node Running</font> и <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    'Перейдите в <b>Settings → Networking → Generate Domain</b>, чтобы получить публичный URL',
    'Откройте <font name="Courier">https://ВАШ-URL/api/status</font> — вы должны увидеть JSON с <b>height</b>, растущим каждые ~6 секунд',
],
railway_vars_code = (
    '# Railway автоматически устанавливает DATABASE_URL, если PostgreSQL в том же проекте\n'
    'RELAYER_PRIVATE_KEY    = 0xВАШ_ПРИВАТНЫЙ_КЛЮЧ\n'
    'RELAYER_ADDRESS        = 0xВАШ_АДРЕС_КОШЕЛЬКА_УЗЛА\n'
    'NODE_OPERATOR_WALLET   = 0xВАШ_ЧЕЛОВЕЧЕСКИЙ_КОШЕЛЁК\n'
    '# PEER_SECRET больше не требуется — аутентификация автоматическая\n'
    'SELF_URL               = https://ВАШ-ДОМЕН-RAILWAY.up.railway.app\n'
    'PRIMARY_NODE_URL       = https://aequitas.digital'
),

docker_title = 'Шаг 2b — Альтернатива: развёртывание с Docker (Расширенный)',
docker_intro = 'Используйте это, если у вас есть свой сервер (VPS, домашний сервер, облачная ВМ). Требуется Docker и база данных PostgreSQL.',
docker_code  = (
    '# 1. Скачайте код\n'
    'git clone https://github.com/hanoi96international-gif/Aequitas && cd Aequitas\n\n'
    '# 2. Соберите образ узла (~3 мин компиляции Go)\n'
    'docker build -t aequitas-node .\n\n'
    '# 3. Запустите узел\n'
    'docker run -d --name aequitas-node --restart unless-stopped \\\n'
    '  -e DATABASE_URL="postgres://user:pass@host:5432/aequitas" \\\n'
    '  -e RELAYER_PRIVATE_KEY="0xВАШ_ПРИВАТНЫЙ_КЛЮЧ" \\\n'
    '  -e RELAYER_ADDRESS="0xВАШ_АДРЕС_КОШЕЛЬКА_УЗЛА" \\\n'
    '  -e NODE_OPERATOR_WALLET="0xВАШ_ЧЕЛОВЕЧЕСКИЙ_КОШЕЛЁК" \\\n'
    '  # -e PEER_SECRET="..." (опционально/устарело, не требуется) \\\n'
    '  -e SELF_URL="https://ВАШ-ПУБЛИЧНЫЙ-URL" \\\n'
    '  -e PRIMARY_NODE_URL="https://aequitas.digital" \\\n'
    '  -p 8080:8080 aequitas-node\n\n'
    '# 4. Следите за логами в реальном времени\n'
    'docker logs -f aequitas-node'
),

verify_title = 'Шаг 3 — Проверьте, что ваш узел работает',
verify_body  = 'Откройте эти URL в браузере. Замените ВАШ-URL-УЗЛА на ваш реальный домен Railway или адрес сервера.',
verify_code  = (
    'https://ВАШ-URL-УЗЛА/api/status\n'
    ' → Ожидается: {"height": 1234, "total_humans": N, "aequitas_index": N}\n\n'
    'https://ВАШ-URL-УЗЛА/rpc\n'
    ' → Ожидается: {"jsonrpc":"2.0","error":"method not specified"} — RPC работает'
),
verify_note  = 'Высота блока должна совпадать с основным узлом в пределах 1–2 блоков в течение нескольких секунд после запуска. Если остаётся на 0, проверьте, что PRIMARY_NODE_URL=https://aequitas.digital установлен и доступен.',

valkey_title = 'Шаг 3b — Зарегистрируйте ключ валидатора (Децентрализованная аутентификация)',
valkey_body  = 'Вместо общего PEER_SECRET зарегистрируйте ключ подписи вашего узла с вашим человеческим кошельком. Это криптографически доказывает, что вы контролируете оба ключа. Получите подпись, выполнив это на вашем сервере (SSH/Railway shell):',
valkey_code  = 'curl "http://localhost:8080/api/sign-validator-challenge?wallet=0xВАШ_ЧЕЛОВЕЧЕСКИЙ_КОШЕЛЁК"',
valkey_note  = 'Затем используйте вкладку Network → Run a Node на сайте и нажмите "Sign with MetaMask & Register" для завершения регистрации.',

mm_title = 'Шаг 4 — Подключите MetaMask к вашему узлу (Опционально)',
mm_body  = 'В MetaMask: нажмите выпадающее меню сети → Add network → Add a network manually, затем введите:',
mm_rows  = [
    ('Network Name',    'Aequitas Chain'),
    ('RPC URL',         'https://ВАШ-URL-УЗЛА/rpc'),
    ('Chain ID',        '1926'),
    ('Currency Symbol', 'AEQ'),
    ('Decimals',        '18'),
    ('Block Explorer',  'https://aequitas.digital'),
],

rewards_title = 'Шаг 5 — Получение наград валидатора',
rewards_box   = 'Validators Pool собирает 40% всех протокольных сборов (комиссии за свап, демередж, избыток лимита богатства). Каждый день в 20:00 по берлинскому времени (CEST/CET, автоматически учитывает переход на летнее время) узел распределяет баланс пула пропорционально произведённым блокам между всеми зарегистрированными операторами узлов. Чем дольше стабильно работает ваш узел, тем больше ваша доля.',
rewards_steps = [
    'Убедитесь, что вы зарегистрированы как человек в Aequitas. Если нет: сначала установите приложение Android и завершите биометрическую регистрацию. Вы получите адрес кошелька и 1 000 AEQ.',
    'Установите <font name="Courier">NODE_OPERATOR_WALLET</font> = ваш адрес человеческого кошелька Aequitas в Variables Railway',
    'Сохраните — Railway переразвернёт автоматически. С Docker: <font name="Courier">docker restart aequitas-node</font>',
    'В логах вашего узла подтвердите: <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    'Награды распределяются автоматически каждый день в 20:00 по берлинскому времени (CEST/CET). Просто держите узел запущенным — дальнейших действий не требуется.',
],

trouble_title = 'Устранение неполадок',
trouble_cols  = ['Симптом', 'Вероятная причина', 'Решение'],
trouble_rows  = [
    ('Высота блока остаётся на 0',         'PRIMARY_NODE_URL не установлен или неверен', 'Установите PRIMARY_NODE_URL=https://aequitas.digital и переразверните. Также установите SELF_URL с публичным URL вашего узла.'),
    ('Ошибка DATABASE_URL при запуске',    'Неверная строка подключения',                'Проверьте формат: postgres://user:pass@host:5432/dbname — убедитесь, что PostgreSQL работает и доступен.'),
    ('"no code at address" в логах',       'Контракт V7 ещё не развёрнут',                'Нормально при первом запуске — узел автоматически развёртывает V7. Подождите несколько секунд и проверьте снова.'),
    ('"NODE_OPERATOR_WALLET not set"',     'Отсутствует переменная окружения',            'Добавьте NODE_OPERATOR_WALLET=0xВАШ_ЧЕЛОВЕЧЕСКИЙ_КОШЕЛЁК. Узел работает без неё, но вы не будете получать награды.'),
    ('"Application error" на Railway',    'Ошибка сборки или запуска',                   'Проверьте Deploy Logs. Чаще всего: отсутствует DATABASE_URL или неверный формат RELAYER_PRIVATE_KEY (должен начинаться с 0x).'),
    ('Порт 8080 недоступен (Docker)',      'Настройка файрвола или облачного провайдера', 'Откройте входящий TCP порт 8080 в настройках файрвола или группы безопасности облака.'),
    ('Сборка Docker не удаётся (ошибка модуля)', 'Нет интернета во время сборки',          'Сборка Docker требует исходящего доступа в интернет для загрузки модулей Go. Railway обрабатывает это автоматически.'),
],

footer = 'Aequitas Chain · Chain ID 1926 · aequitas.digital · Награды валидатора: ежедневно в 20:00 по берлинскому времени (CEST/CET)',
)

ZH = dict(
title    = 'AEQUITAS 节点运营者指南',
version  = 'v1.0 · 2026年6月 · aequitas.digital',
tagline  = '完整的分步指南 · 无需事先的区块链经验 · 约20–30分钟',

prereq_title = '开始之前——您需要的东西',
prereqs = [
    ('1.', '<b>Aequitas 账户：</b>您必须先在 Aequitas 上注册为人类。安装 Android 应用，完成生物识别注册，并记下您的钱包地址。没有这个您无法获得验证者奖励。'),
    ('2.', '<b>GitHub 账户（免费）：</b>访问 github.com 并创建一个免费账户。您需要它来复制（fork）Aequitas 代码，以便 Railway 可以部署它。'),
    ('3.', '<b>Railway 账户（免费）：</b>访问 railway.app 并使用 GitHub 登录。Railway 是一个在云端运行您节点的托管平台——不需要服务器或命令行。'),
    ('4.', '<b>节点签名密钥（RELAYER_PRIVATE_KEY）：</b>您的节点需要一个专用的以太坊钱包来签署链上注册。可以是任何 MetaMask 钱包。导出其私钥：MetaMask → Account Details → Show Private Key → 输入密码 → 复制。严格保密。<b>重要：</b>要获得验证者奖励，NODE_OPERATOR_WALLET 也必须是您<b>已注册的 Aequitas 人类钱包</b>（已通过 AequitasBio 验证的钱包）。只有经过验证的人类才能获得验证者奖励。'),
    ('5.', '<b>10–30分钟的时间。</b>Railway 会自动完成大部分工作。'),
],

vars_title = '第1步——环境变量',
vars_warn  = '安全警告：您的 RELAYER_PRIVATE_KEY 就像主密码。任何拥有它的人都能控制您的节点钱包。切勿公开分享，切勿粘贴到聊天或电子邮件中。为 RELAYER_PRIVATE_KEY（签名）使用单独的 MetaMask 钱包。NODE_OPERATOR_WALLET（用于奖励）必须是您已注册的 Aequitas 人类钱包。',
var_cols   = ['变量', '是否必需？', '应设置什么'],
vars = [
    ('DATABASE_URL',        '是',         '您的 PostgreSQL 连接字符串。在 Railway 上：如果 PostgreSQL 在同一项目中会自动注入。格式：postgres://user:pass@host:5432/dbname'),
    ('RELAYER_PRIVATE_KEY', '是',         '您专用节点钱包的私钥（0x…，66个字符）。MetaMask：Account Details → Show Private Key → 输入密码 → 复制。'),
    ('RELAYER_ADDRESS',     '推荐',       '与 RELAYER_PRIVATE_KEY 对应的钱包地址（0x…，42个字符）。从 MetaMask 复制。有备用机制，但明确设置可避免启动错误。'),
    ('NODE_OPERATOR_WALLET','用于奖励',   '您的 Aequitas 人类钱包地址——通过 Android 应用注册。接收您每日的验证者奖励（所有协议费用的40%）。必须是已注册的人类。'),
    ('NODE_OPERATOR_BINDING_SIGNATURE', '用于奖励', '证明您拥有 NODE_OPERATOR_WALLET。在 aequitas.digital/node-binding 生成：用您的人类钱包在 MetaMask 中签署显示的消息，将得到的签名粘贴在此处。没有它您的节点仍能运行，但无法自动注册奖励。'),
    ('PEER_SECRET',         '可选/旧版',  '旧版共享密钥备用机制。不再需要——节点现在通过加密的挑战-响应（RELAYER_PRIVATE_KEY）自动进行身份验证。仅为与旧部署的向后兼容性而需要。'),
    ('SELF_URL',            '多节点',     '您自己节点的公共 HTTPS URL（例如 https://my-node.up.railway.app）。在节点发现中自我排除时需要。可在 Railway 中找到：Settings → Networking → Public Networking。'),
    ('PRIMARY_NODE_URL',    '多节点',     '设置为：https://aequitas.digital——您的节点为自动节点发现而注册的主节点。启动时，您的节点将其 URL + 签名地址发送给主节点，获取完整的节点列表，并自动加入网络。'),
    ('BOOTSTRAP_SNAPSHOT_URL', '推荐',    '设置为：https://aequitas.digital/api/snapshot——让新节点从网络当前状态开始，而不是从创世区块重放整个历史。首次同步速度大幅提升。'),
    ('BOOTSTRAP_SIGNER',    '配合快照',   '主节点的签名地址，用于在导入前验证快照是否真实。从 https://aequitas.digital/api/status → "signing_address" 获取当前值。设置 BOOTSTRAP_SNAPSHOT_URL 时必需。'),
    ('SNAPSHOT_TOKEN',      '可选',       '启动新节点不需要它——没有它您仍能获得正确运行所需的一切（账户、余额、池、配置）。它只解锁完整导出（nullifier/钱包关联 + bio_registrations），用于已经分歧节点的权威重新同步。仅在确实需要时向网络运营者询问。'),
    ('RESYNC_FROM_SNAPSHOT', '仅用于恢复', '危险，临时性：仅在恢复状态已偏离网络的节点时，与 BOOTSTRAP_SNAPSHOT_URL 和 BOOTSTRAP_SIGNER 一起设置为 true。会完全替换本地状态。重启一次后再次移除此变量——如果保留，将在每次重启时强制进行完整重新同步。'),
    ('AUTO_HEAL_ON_DIVERGENCE', '强烈推荐', '对网络的安全性和速度都很重要：如果你的节点链在某个时刻与网络产生分歧（例如宕机后或一次失败的重启后），节点会自行检测到这一点——每隔几分钟与 PRIMARY_NODE_URL 进行比对——并自动重新同步，无需手动设置 RESYNC_FROM_SNAPSHOT。一个持续分歧却仍在广播自己区块的节点，可能会拖慢甚至破坏整个网络对所有其他运营者的稳定性。请将其与 PRIMARY_NODE_URL、BOOTSTRAP_SNAPSHOT_URL 和 BOOTSTRAP_SIGNER 一起设置为 true。'),
    ('PORT',                '否',         '不要在 Railway 上设置——Railway 会自动设置。默认值为8080。'),
    ('NODE_KEY',            '否',         '用于稳定 P2P 身份的 Base64 libp2p 密钥。如果省略会自动生成，但每次重启都会变化。如果未设置，节点会将其打印到 stderr："SAVE THIS AS NODE_KEY ENVIRONMENT VAR: <base64>"。复制并粘贴到此处。'),
    ('IS_PRIMARY_NODE',     '否',         '保持未设置或为 false。分发现在使用数据库级锁——任何节点都可以在没有此变量的情况下运行它。'),
    ('RESET_STATE',         '否',         '危险：将此设置为 true 会在每次重启时清空整个数据库。仅用于开发。绝不要在生产环境中使用。'),
],

railway_title = '第2步——在 Railway 上部署（推荐）',
railway_intro = 'Railway 是运行您节点最简单的方式——无需服务器配置，无需命令行。免费套餐涵盖所有需求。总耗时：约10–15分钟。',
railway_steps = [
    '将 github.com/hanoi96international-gif/Aequitas fork 到您自己的 GitHub 账户（点击<b>Fork</b> → <b>Create fork</b>）',
    '在 railway.app 上使用 GitHub 登录，然后<b>New Project</b> → <b>+ New</b> → <b>Database</b> → <b>Add PostgreSQL</b>',
    '在同一个 Railway 项目中，点击<b>+ New → GitHub Repo</b>并选择您的 Aequitas fork——Railway 会自动检测 Dockerfile',
    '点击<b>Deploy Now</b>——首次构建开始（没有环境变量可能会失败，这是正常的）',
    '点击您的 Aequitas 服务 → <b>Variables</b> → 添加每个变量（见上表）。最低要求：RELAYER_PRIVATE_KEY、RELAYER_ADDRESS、NODE_OPERATOR_WALLET、SELF_URL、PRIMARY_NODE_URL=https://aequitas.digital（PEER_SECRET 不再需要）',
    '点击<b>Deploy</b>（或保存变量以触发自动重新部署）。构建大约需要3分钟，因为 Go 要编译节点二进制文件。',
    '观察<b>Deploy Logs</b>。成功的样子是：<font name="Courier" color="#5B21B6">Aequitas Node Running</font> 和 <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    '转到<b>Settings → Networking → Generate Domain</b>以获取您的公共 URL',
    '打开<font name="Courier">https://YOUR-URL/api/status</font>——您应该看到 JSON，其中<b>height</b>每约6秒增长一次',
],
railway_vars_code = (
    '# 如果 PostgreSQL 在同一项目中，Railway 会自动设置 DATABASE_URL\n'
    'RELAYER_PRIVATE_KEY    = 0x你的私钥\n'
    'RELAYER_ADDRESS        = 0x你的节点钱包地址\n'
    'NODE_OPERATOR_WALLET   = 0x你的人类钱包\n'
    '# PEER_SECRET 不再需要——身份验证是自动的\n'
    'SELF_URL               = https://你的RAILWAY域名.up.railway.app\n'
    'PRIMARY_NODE_URL       = https://aequitas.digital'
),

docker_title = '第2b步——替代方案：使用 Docker 部署（高级）',
docker_intro = '如果您有自己的服务器（VPS、家用服务器、云虚拟机），请使用此方法。需要 Docker 和 PostgreSQL 数据库。',
docker_code  = (
    '# 1. 下载代码\n'
    'git clone https://github.com/hanoi96international-gif/Aequitas && cd Aequitas\n\n'
    '# 2. 构建节点镜像（约3分钟 Go 编译时间）\n'
    'docker build -t aequitas-node .\n\n'
    '# 3. 启动节点\n'
    'docker run -d --name aequitas-node --restart unless-stopped \\\n'
    '  -e DATABASE_URL="postgres://user:pass@host:5432/aequitas" \\\n'
    '  -e RELAYER_PRIVATE_KEY="0x你的私钥" \\\n'
    '  -e RELAYER_ADDRESS="0x你的节点钱包地址" \\\n'
    '  -e NODE_OPERATOR_WALLET="0x你的人类钱包" \\\n'
    '  # -e PEER_SECRET="..."（可选/旧版，不需要） \\\n'
    '  -e SELF_URL="https://你的公共URL" \\\n'
    '  -e PRIMARY_NODE_URL="https://aequitas.digital" \\\n'
    '  -p 8080:8080 aequitas-node\n\n'
    '# 4. 观察实时日志\n'
    'docker logs -f aequitas-node'
),

verify_title = '第3步——验证您的节点正在运行',
verify_body  = '在浏览器中打开这些 URL。将 YOUR-NODE-URL 替换为您实际的 Railway 域名或服务器地址。',
verify_code  = (
    'https://YOUR-NODE-URL/api/status\n'
    ' → 预期：{"height": 1234, "total_humans": N, "aequitas_index": N}\n\n'
    'https://YOUR-NODE-URL/rpc\n'
    ' → 预期：{"jsonrpc":"2.0","error":"method not specified"}——RPC 正常运行'
),
verify_note  = '启动后几秒钟内，区块高度应与主节点相差1–2个区块以内。如果一直停留在0，请检查 PRIMARY_NODE_URL=https://aequitas.digital 是否已设置且可访问。',

valkey_title = '第3b步——注册您的验证者密钥（去中心化身份验证）',
valkey_body  = '不使用共享的 PEER_SECRET，而是用您的人类钱包注册节点签名密钥。这能在密码学上证明您同时控制这两个密钥。在您的服务器上运行以下命令获取签名（SSH/Railway shell）：',
valkey_code  = 'curl "http://localhost:8080/api/sign-validator-challenge?wallet=0x你的人类钱包"',
valkey_note  = '然后使用网站上的 Network → Run a Node 标签，点击"Sign with MetaMask & Register"完成注册。',

mm_title = '第4步——将 MetaMask 连接到您的节点（可选）',
mm_body  = '在 MetaMask 中：点击网络下拉菜单 → Add network → Add a network manually，然后输入：',
mm_rows  = [
    ('Network Name',    'Aequitas Chain'),
    ('RPC URL',         'https://YOUR-NODE-URL/rpc'),
    ('Chain ID',        '1926'),
    ('Currency Symbol', 'AEQ'),
    ('Decimals',        '18'),
    ('Block Explorer',  'https://aequitas.digital'),
],

rewards_title = '第5步——获得验证者奖励',
rewards_box   = '验证者池收取所有协议费用的40%（兑换费用、负利息、财富上限超额部分）。每天柏林时间20:00（CEST/CET，自动处理夏令时），节点会按照生产的区块数量比例，将池余额分配给所有已注册的节点运营者。您的节点运行越持续稳定，您的份额就越大。',
rewards_steps = [
    '确保您已在 Aequitas 上注册为人类。如果没有：先安装 Android 应用并完成生物识别注册。您将获得一个钱包地址和1,000 AEQ。',
    '在您的 Railway Variables 中设置<font name="Courier">NODE_OPERATOR_WALLET</font> = 您的 Aequitas 人类钱包地址',
    '保存——Railway 会自动重新部署。使用 Docker：<font name="Courier">docker restart aequitas-node</font>',
    '在您的节点日志中确认：<font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    '奖励每天柏林时间20:00（CEST/CET）自动分发。只需保持您的节点运行——无需进一步操作。',
],

trouble_title = '故障排除',
trouble_cols  = ['症状', '可能原因', '解决方案'],
trouble_rows  = [
    ('区块高度停留在0',            'PRIMARY_NODE_URL 未设置或错误',  '设置 PRIMARY_NODE_URL=https://aequitas.digital 并重新部署。同时将 SELF_URL 设置为您节点的公共 URL。'),
    ('启动时出现 DATABASE_URL 错误', '连接字符串错误',                '检查格式：postgres://user:pass@host:5432/dbname——确保 PostgreSQL 正在运行且可访问。'),
    ('日志中出现"no code at address"', 'V7 合约尚未部署',              '首次启动时正常——节点会自动部署 V7。等待几秒钟后再检查。'),
    ('"NODE_OPERATOR_WALLET not set"', '缺少环境变量',                '添加 NODE_OPERATOR_WALLET=0x你的人类钱包。没有它节点也能正常运行，但您不会收到奖励。'),
    ('Railway 出现"Application error"', '构建或启动失败',              '检查 Deploy Logs。最常见的原因：缺少 DATABASE_URL 或 RELAYER_PRIVATE_KEY 格式错误（必须以0x开头）。'),
    ('端口8080无法访问（Docker）',  '防火墙或云服务商配置',            '在您的防火墙或云安全组设置中开放入站 TCP 端口8080。'),
    ('Docker 构建失败（模块错误）',  '构建期间没有互联网连接',          'Docker 构建需要出站互联网连接来下载 Go 模块。Railway 会自动处理这一点。'),
],

footer = 'Aequitas Chain · Chain ID 1926 · aequitas.digital · 验证者奖励：每天柏林时间20:00（CEST/CET）',
)

AR = dict(
title    = 'دليل مشغّل عقدة AEQUITAS',
version  = 'الإصدار 1.0 · يونيو 2026 · aequitas.digital',
tagline  = 'دليل كامل خطوة بخطوة · لا حاجة لخبرة سابقة بالبلوكتشين · حوالي 20–30 دقيقة',

prereq_title = 'قبل البدء — ما تحتاجه',
prereqs = [
    ('1.', '<b>حساب Aequitas:</b> يجب أن تكون مسجلاً كإنسان على Aequitas أولاً. ثبّت تطبيق Android، أكمل التسجيل البيومتري، ودوّن عنوان محفظتك. بدون ذلك لا يمكنك الحصول على مكافآت المُصادِق.'),
    ('2.', '<b>حساب GitHub (مجاني):</b> اذهب إلى github.com وأنشئ حساباً مجانياً. تحتاجه لنسخ (فورك) كود Aequitas حتى يتمكن Railway من نشره.'),
    ('3.', '<b>حساب Railway (مجاني):</b> اذهب إلى railway.app وسجّل الدخول باستخدام GitHub. Railway هي منصة استضافة تُشغّل عقدتك في السحابة — لا حاجة لخادم أو سطر أوامر.'),
    ('4.', '<b>مفتاح توقيع العقدة (RELAYER_PRIVATE_KEY):</b> تحتاج عقدتك إلى محفظة Ethereum مخصصة لتوقيع التسجيلات على السلسلة. يمكن أن تكون أي محفظة MetaMask. صدّر مفتاحها الخاص: MetaMask ← Account Details ← Show Private Key ← أدخل كلمة المرور ← انسخ. احتفظ بها سرّية للغاية. <b>مهم:</b> للحصول على مكافآت المُصادِق، يجب أيضاً أن يكون NODE_OPERATOR_WALLET هو <b>محفظتك البشرية المسجّلة في Aequitas</b> (المُحقّقة عبر AequitasBio). فقط البشر المُحقّقون يمكنهم كسب مكافآت المُصادِق.'),
    ('5.', '<b>10–30 دقيقة من وقتك.</b> يقوم Railway بمعظم العمل تلقائياً.'),
],

vars_title = 'الخطوة 1 — متغيرات البيئة',
vars_warn  = 'تحذير أمني: RELAYER_PRIVATE_KEY هو مثل كلمة مرور رئيسية. أي شخص يحصل عليها يتحكم بمحفظة عقدتك. لا تشاركها علناً مطلقاً، ولا تلصقها في محادثة أو بريد إلكتروني. استخدم محفظة MetaMask منفصلة لـ RELAYER_PRIVATE_KEY (التوقيع). يجب أن يكون NODE_OPERATOR_WALLET (للمكافآت) محفظتك البشرية المسجّلة في Aequitas.',
var_cols   = ['المتغير', 'مطلوب؟', 'ما الذي يجب تعيينه'],
vars = [
    ('DATABASE_URL',        'نعم',        'سلسلة اتصال PostgreSQL الخاصة بك. على Railway: تُحقن تلقائياً عند وجود PostgreSQL في المشروع نفسه. الصيغة: postgres://user:pass@host:5432/dbname'),
    ('RELAYER_PRIVATE_KEY', 'نعم',        'المفتاح الخاص (0x…، 66 حرفاً) لمحفظة عقدتك المخصصة. MetaMask: Account Details ← Show Private Key ← أدخل كلمة المرور ← انسخ.'),
    ('RELAYER_ADDRESS',     'مُوصى به',   'عنوان المحفظة (0x…، 42 حرفاً) المطابق لـ RELAYER_PRIVATE_KEY. انسخه من MetaMask. يوجد احتياط، لكن تعيينه صريحاً يمنع أخطاء بدء التشغيل.'),
    ('NODE_OPERATOR_WALLET','للمكافآت',   'عنوان محفظتك البشرية في Aequitas — المسجّلة عبر تطبيق Android. تستلم مكافآت المُصادِق اليومية (40% من جميع رسوم البروتوكول). يجب أن تكون إنساناً مسجّلاً.'),
    ('NODE_OPERATOR_BINDING_SIGNATURE', 'للمكافآت', 'يُثبت أنك تملك NODE_OPERATOR_WALLET. أنشئه على aequitas.digital/node-binding: وقّع الرسالة المعروضة بمحفظتك البشرية في MetaMask، ثم ألصق التوقيع الناتج هنا. بدونه تعمل عقدتك بشكل طبيعي لكنها لا تستطيع التسجيل تلقائياً للمكافآت.'),
    ('PEER_SECRET',         'اختياري/قديم', 'آلية احتياطية قديمة بسر مشترك. لم تعد مطلوبة — تتحقق العُقد الآن تلقائياً عبر تحدٍ-استجابة تشفيري (RELAYER_PRIVATE_KEY). مطلوبة فقط للتوافق مع عمليات النشر القديمة.'),
    ('SELF_URL',            'متعدد العُقد', 'عنوان URL العام (HTTPS) لعقدتك الخاصة (مثل https://my-node.up.railway.app). مطلوب لاستثناء النفس عند اكتشاف الأقران. يوجد في Railway: Settings ← Networking ← Public Networking.'),
    ('PRIMARY_NODE_URL',    'متعدد العُقد', 'عيّنه إلى: https://aequitas.digital — العقدة الأساسية التي تسجّل عقدتك معها لاكتشاف الأقران تلقائياً. عند بدء التشغيل، ترسل عقدتك عنوان URL + عنوان التوقيع إلى العقدة الأساسية، وتحصل على قائمة الأقران الكاملة، وتنضم إلى الشبكة تلقائياً.'),
    ('BOOTSTRAP_SNAPSHOT_URL', 'مُوصى به', 'عيّنه إلى: https://aequitas.digital/api/snapshot — يتيح لعقدة جديدة البدء من الحالة الحالية للشبكة بدلاً من إعادة تشغيل كل التاريخ من بداية السلسلة. مزامنة أولى أسرع بكثير.'),
    ('BOOTSTRAP_SIGNER',    'مع لقطة الحالة', 'عنوان توقيع العقدة الأساسية، يُستخدم للتحقق من أن لقطة الحالة أصلية قبل استيرادها. احصل على القيمة الحالية من https://aequitas.digital/api/status ← "signing_address". مطلوب كلما تم تعيين BOOTSTRAP_SNAPSHOT_URL.'),
    ('SNAPSHOT_TOKEN',      'اختياري',    'غير مطلوب لتأسيس عقدة جديدة — بدونه تحصل مع ذلك على كل ما تحتاجه للعمل بشكل صحيح (الحسابات، الأرصدة، المجمع، التهيئة). يفتح فقط التصدير الكامل (ربط nullifier/المحفظة + bio_registrations)، المستخدَم لإعادة مزامنة موثوقة لعقدة منحرفة بالفعل. اسأل مشغّل الشبكة فقط إذا كنت تحتاج ذلك فعلاً.'),
    ('RESYNC_FROM_SNAPSHOT', 'للاستعادة فقط', 'خطير، مؤقت: عيّنه إلى true مع BOOTSTRAP_SNAPSHOT_URL و BOOTSTRAP_SIGNER فقط لاستعادة عقدة انحرفت حالتها عن الشبكة. يستبدل الحالة المحلية بالكامل. أعد التشغيل مرة واحدة، ثم أزل هذا المتغير مجدداً — تركه يفرض إعادة مزامنة كاملة في كل إعادة تشغيل.'),
    ('AUTO_HEAL_ON_DIVERGENCE', 'يُوصى به بشدة', 'مهم لأمان الشبكة وسرعتها: إذا انحرفت سلسلة عقدتك يومًا عن الشبكة (مثلاً بعد توقف أو إعادة تشغيل فاشلة)، فإنها تكتشف ذلك بنفسها — من خلال مقارنة نفسها بـ PRIMARY_NODE_URL كل بضع دقائق — وتعيد المزامنة تلقائيًا، دون الحاجة إلى RESYNC_FROM_SNAPSHOT يدويًا. العقدة التي تبقى منحرفة وتستمر في بث كتلها الخاصة قد تُبطئ أو تزعزع استقرار الشبكة بأكملها لبقية المشغّلين. عيّنه إلى true مع PRIMARY_NODE_URL و BOOTSTRAP_SNAPSHOT_URL و BOOTSTRAP_SIGNER.'),
    ('PORT',                'لا',         'لا تعيّنه على Railway — يضبطه Railway تلقائياً. القيمة الافتراضية هي 8080.'),
    ('NODE_KEY',            'لا',         'مفتاح libp2p بترميز Base64 لهوية P2P مستقرة. يُنشأ تلقائياً إذا حُذف، لكنه يتغير في كل إعادة تشغيل. إذا لم يُعيَّن، تطبعه العقدة في stderr: "SAVE THIS AS NODE_KEY ENVIRONMENT VAR: <base64>". انسخه وألصقه هنا.'),
    ('IS_PRIMARY_NODE',     'لا',         'اتركه غير معيَّن أو false. التوزيع يستخدم الآن قفلاً على مستوى قاعدة البيانات — يمكن لأي عقدة تشغيله بدون هذا المتغير.'),
    ('RESET_STATE',         'لا',         'خطير: تعيين هذا إلى true يمحو قاعدة بياناتك بالكامل في كل إعادة تشغيل. للاستخدام في التطوير فقط. لا تستخدمه أبداً في الإنتاج.'),
],

railway_title = 'الخطوة 2 — النشر على Railway (مُوصى به)',
railway_intro = 'Railway هي أسهل طريقة لتشغيل عقدتك — بدون إعداد خادم، بدون سطر أوامر. الباقة المجانية تغطي جميع المتطلبات. الوقت الإجمالي: حوالي 10–15 دقيقة.',
railway_steps = [
    'افعل فورك لـ github.com/hanoi96international-gif/Aequitas إلى حسابك الخاص على GitHub (انقر <b>Fork</b> ← <b>Create fork</b>)',
    'في railway.app، سجّل الدخول باستخدام GitHub، ثم <b>New Project</b> ← <b>+ New</b> ← <b>Database</b> ← <b>Add PostgreSQL</b>',
    'في نفس مشروع Railway، انقر <b>+ New ← GitHub Repo</b> واختر فورك Aequitas الخاص بك — يكتشف Railway ملف Dockerfile تلقائياً',
    'انقر <b>Deploy Now</b> — يبدأ أول بناء (قد يفشل بدون متغيرات البيئة، هذا أمر طبيعي)',
    'انقر على خدمة Aequitas الخاصة بك ← <b>Variables</b> ← أضف كل متغير (انظر الجدول أعلاه). الحد الأدنى المطلوب: RELAYER_PRIVATE_KEY، RELAYER_ADDRESS، NODE_OPERATOR_WALLET، SELF_URL، PRIMARY_NODE_URL=https://aequitas.digital (PEER_SECRET غير مطلوب بعد الآن)',
    'انقر <b>Deploy</b> (أو احفظ المتغيرات لتشغيل إعادة النشر التلقائي). يستغرق البناء حوالي 3 دقائق بينما تُجمّع Go ملف العقدة الثنائي.',
    'راقب <b>Deploy Logs</b>. يبدو النجاح كهذا: <font name="Courier" color="#5B21B6">Aequitas Node Running</font> و <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    'اذهب إلى <b>Settings ← Networking ← Generate Domain</b> للحصول على عنوان URL العام',
    'افتح <font name="Courier">https://YOUR-URL/api/status</font> — يجب أن ترى JSON مع <b>height</b> يزداد كل حوالي 6 ثوانٍ',
],
railway_vars_code = (
    '# يضبط Railway تلقائياً DATABASE_URL إذا كان PostgreSQL في المشروع نفسه\n'
    'RELAYER_PRIVATE_KEY    = 0xمفتاحك_الخاص\n'
    'RELAYER_ADDRESS        = 0xعنوان_محفظة_عقدتك\n'
    'NODE_OPERATOR_WALLET   = 0xمحفظتك_البشرية\n'
    '# PEER_SECRET غير مطلوب بعد الآن — المصادقة تلقائية\n'
    'SELF_URL               = https://YOUR-RAILWAY-DOMAIN.up.railway.app\n'
    'PRIMARY_NODE_URL       = https://aequitas.digital'
),

docker_title = 'الخطوة 2ب — بديل: النشر باستخدام Docker (متقدم)',
docker_intro = 'استخدم هذا إذا كان لديك خادمك الخاص (VPS، خادم منزلي، جهاز افتراضي سحابي). يتطلب Docker وقاعدة بيانات PostgreSQL.',
docker_code  = (
    '# 1. نزّل الكود\n'
    'git clone https://github.com/hanoi96international-gif/Aequitas && cd Aequitas\n\n'
    '# 2. بناء صورة العقدة (حوالي 3 دقائق لتجميع Go)\n'
    'docker build -t aequitas-node .\n\n'
    '# 3. تشغيل العقدة\n'
    'docker run -d --name aequitas-node --restart unless-stopped \\\n'
    '  -e DATABASE_URL="postgres://user:pass@host:5432/aequitas" \\\n'
    '  -e RELAYER_PRIVATE_KEY="0xمفتاحك_الخاص" \\\n'
    '  -e RELAYER_ADDRESS="0xعنوان_محفظة_عقدتك" \\\n'
    '  -e NODE_OPERATOR_WALLET="0xمحفظتك_البشرية" \\\n'
    '  # -e PEER_SECRET="..." (اختياري/قديم، غير مطلوب) \\\n'
    '  -e SELF_URL="https://عنوانك-العام" \\\n'
    '  -e PRIMARY_NODE_URL="https://aequitas.digital" \\\n'
    '  -p 8080:8080 aequitas-node\n\n'
    '# 4. راقب السجلات المباشرة\n'
    'docker logs -f aequitas-node'
),

verify_title = 'الخطوة 3 — تحقق من أن عقدتك تعمل',
verify_body  = 'افتح هذه العناوين في متصفحك. استبدل YOUR-NODE-URL بنطاق Railway الفعلي أو عنوان الخادم الخاص بك.',
verify_code  = (
    'https://YOUR-NODE-URL/api/status\n'
    ' ← المتوقع: {"height": 1234, "total_humans": N, "aequitas_index": N}\n\n'
    'https://YOUR-NODE-URL/rpc\n'
    ' ← المتوقع: {"jsonrpc":"2.0","error":"method not specified"} — RPC يعمل'
),
verify_note  = 'يجب أن يتطابق ارتفاع البلوك مع العقدة الأساسية ضمن 1–2 بلوك في غضون ثوانٍ من بدء التشغيل. إذا بقي عند 0، تحقق من أن PRIMARY_NODE_URL=https://aequitas.digital معيَّن وقابل للوصول.',

valkey_title = 'الخطوة 3ب — سجّل مفتاح المُصادِق الخاص بك (مصادقة غير مركزية)',
valkey_body  = 'بدلاً من PEER_SECRET المشترك، سجّل مفتاح توقيع عقدتك بمحفظتك البشرية. هذا يُثبت تشفيرياً أنك تتحكم بكلا المفتاحين. احصل على التوقيع بتشغيل هذا على خادمك (SSH/Railway shell):',
valkey_code  = 'curl "http://localhost:8080/api/sign-validator-challenge?wallet=0xمحفظتك_البشرية"',
valkey_note  = 'ثم استخدم علامة التبويب Network ← Run a Node في الموقع وانقر "Sign with MetaMask & Register" لإكمال التسجيل.',

mm_title = 'الخطوة 4 — ربط MetaMask بعقدتك (اختياري)',
mm_body  = 'في MetaMask: انقر القائمة المنسدلة للشبكة ← Add network ← Add a network manually، ثم أدخل:',
mm_rows  = [
    ('Network Name',    'Aequitas Chain'),
    ('RPC URL',         'https://YOUR-NODE-URL/rpc'),
    ('Chain ID',        '1926'),
    ('Currency Symbol', 'AEQ'),
    ('Decimals',        '18'),
    ('Block Explorer',  'https://aequitas.digital'),
],

rewards_title = 'الخطوة 5 — الحصول على مكافآت المُصادِق',
rewards_box   = 'يجمع مجمع المُصادِقين 40% من جميع رسوم البروتوكول (رسوم التبديل، التراجع التضخمي، فائض سقف الثروة). كل يوم في الساعة 20:00 بتوقيت برلين (CEST/CET، يتعامل مع التوقيت الصيفي تلقائياً) توزّع العقدة رصيد المجمع على جميع مشغّلي العُقد المسجّلين بالتناسب مع البلوكات المُنتَجة. كلما عمِلت عقدتك باستمرار أطول، كانت حصتك أكبر.',
rewards_steps = [
    'تأكد من أنك مسجّل كإنسان على Aequitas. إذا لم تكن: ثبّت تطبيق Android أولاً وأكمل التسجيل البيومتري. ستحصل على عنوان محفظة و1,000 AEQ.',
    'عيّن <font name="Courier">NODE_OPERATOR_WALLET</font> = عنوان محفظتك البشرية في Aequitas في متغيرات Railway الخاصة بك',
    'احفظ — يعيد Railway النشر تلقائياً. مع Docker: <font name="Courier">docker restart aequitas-node</font>',
    'في سجلات عقدتك، تأكد من: <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    'تُوزَّع المكافآت تلقائياً كل يوم في الساعة 20:00 بتوقيت برلين (CEST/CET). فقط حافظ على تشغيل عقدتك — لا حاجة لأي إجراء آخر.',
],

trouble_title = 'استكشاف الأخطاء وإصلاحها',
trouble_cols  = ['العَرَض', 'السبب المحتمل', 'الحل'],
trouble_rows  = [
    ('ارتفاع البلوك يبقى عند 0',          'PRIMARY_NODE_URL غير معيَّن أو خاطئ',  'عيّن PRIMARY_NODE_URL=https://aequitas.digital وأعد النشر. عيّن أيضاً SELF_URL بعنوان URL العام لعقدتك.'),
    ('خطأ DATABASE_URL عند بدء التشغيل',  'سلسلة اتصال خاطئة',                    'تحقق من الصيغة: postgres://user:pass@host:5432/dbname — تأكد من أن PostgreSQL يعمل ويمكن الوصول إليه.'),
    ('"no code at address" في السجلات',   'عقد V7 لم يُنشَر بعد',                  'طبيعي عند أول بدء تشغيل — تنشر العقدة V7 تلقائياً. انتظر بضع ثوانٍ وتحقق مرة أخرى.'),
    ('"NODE_OPERATOR_WALLET not set"',    'متغير بيئة مفقود',                      'أضف NODE_OPERATOR_WALLET=0xمحفظتك_البشرية. تعمل العقدة بدونه لكنك لن تحصل على مكافآت.'),
    ('"Application error" على Railway',   'فشل في البناء أو بدء التشغيل',          'تحقق من Deploy Logs. الأكثر شيوعاً: DATABASE_URL مفقود أو RELAYER_PRIVATE_KEY بصيغة خاطئة (يجب أن يبدأ بـ 0x).'),
    ('المنفذ 8080 غير قابل للوصول (Docker)', 'إعدادات جدار الحماية أو مزود السحابة', 'افتح منفذ TCP الوارد 8080 في جدار الحماية أو إعدادات مجموعة الأمان السحابية.'),
    ('فشل بناء Docker (خطأ في الوحدة)',    'لا يوجد إنترنت أثناء البناء',           'يحتاج بناء Docker إلى اتصال إنترنت صادر لتحميل وحدات Go. يتعامل Railway مع هذا تلقائياً.'),
],

footer = 'Aequitas Chain · Chain ID 1926 · aequitas.digital · مكافآت المُصادِق: يومياً في الساعة 20:00 بتوقيت برلين (CEST/CET)',
)

HI = dict(
title    = 'AEQUITAS नोड ऑपरेटर गाइड',
version  = 'v1.0 · जून 2026 · aequitas.digital',
tagline  = 'पूर्ण चरण-दर-चरण गाइड · किसी पूर्व ब्लॉकचेन अनुभव की आवश्यकता नहीं · ~20–30 मिनट',

prereq_title = 'शुरू करने से पहले — आपको क्या चाहिए',
prereqs = [
    ('1.', '<b>एक Aequitas खाता:</b> आपको पहले Aequitas पर मानव के रूप में पंजीकृत होना होगा। Android ऐप इंस्टॉल करें, बायोमेट्रिक पंजीकरण पूरा करें, और अपना वॉलेट पता नोट करें। इसके बिना आप वैलिडेटर रिवॉर्ड प्राप्त नहीं कर सकते।'),
    ('2.', '<b>एक GitHub खाता (मुफ्त):</b> github.com पर जाएं और एक मुफ्त खाता बनाएं। आपको Aequitas कोड को कॉपी (फोर्क) करने के लिए इसकी आवश्यकता है ताकि Railway इसे डिप्लॉय कर सके।'),
    ('3.', '<b>एक Railway खाता (मुफ्त):</b> railway.app पर जाएं और GitHub से साइन इन करें। Railway एक होस्टिंग प्लेटफॉर्म है जो आपके नोड को क्लाउड में चलाता है — किसी सर्वर या कमांड लाइन की आवश्यकता नहीं।'),
    ('4.', '<b>नोड साइनिंग की (RELAYER_PRIVATE_KEY):</b> आपके नोड को ऑन-चेन पंजीकरण साइन करने के लिए एक समर्पित Ethereum वॉलेट की आवश्यकता है। यह कोई भी MetaMask वॉलेट हो सकता है। इसकी प्राइवेट की एक्सपोर्ट करें: MetaMask → Account Details → Show Private Key → पासवर्ड दर्ज करें → कॉपी करें। इसे बिल्कुल गुप्त रखें। <b>महत्वपूर्ण:</b> वैलिडेटर रिवॉर्ड प्राप्त करने के लिए NODE_OPERATOR_WALLET को भी आपका <b>पंजीकृत Aequitas मानव वॉलेट</b> होना चाहिए (वह जो AequitasBio से सत्यापित है)। केवल सत्यापित मानव ही वैलिडेटर रिवॉर्ड कमा सकते हैं।'),
    ('5.', '<b>आपके 10–30 मिनट।</b> Railway अधिकांश काम स्वचालित रूप से करता है।'),
],

vars_title = 'चरण 1 — एनवायरनमेंट वेरिएबल्स',
vars_warn  = 'सुरक्षा चेतावनी: आपकी RELAYER_PRIVATE_KEY एक मास्टर पासवर्ड की तरह है। जिसके पास भी यह है वह आपके नोड वॉलेट को नियंत्रित करता है। इसे कभी सार्वजनिक रूप से साझा न करें, कभी चैट या ईमेल में पेस्ट न करें। RELAYER_PRIVATE_KEY (साइनिंग) के लिए एक अलग MetaMask वॉलेट का उपयोग करें। NODE_OPERATOR_WALLET (रिवॉर्ड के लिए) आपका पंजीकृत Aequitas मानव वॉलेट होना चाहिए।',
var_cols   = ['वेरिएबल', 'आवश्यक?', 'क्या सेट करें'],
vars = [
    ('DATABASE_URL',        'हाँ',        'आपकी PostgreSQL कनेक्शन स्ट्रिंग। Railway पर: यदि PostgreSQL एक ही प्रोजेक्ट में है तो स्वचालित रूप से जोड़ा जाता है। फॉर्मेट: postgres://user:pass@host:5432/dbname'),
    ('RELAYER_PRIVATE_KEY', 'हाँ',        'आपके समर्पित नोड वॉलेट की प्राइवेट की (0x…, 66 अक्षर)। MetaMask: Account Details → Show Private Key → पासवर्ड दर्ज करें → कॉपी करें।'),
    ('RELAYER_ADDRESS',     'अनुशंसित',   'RELAYER_PRIVATE_KEY से मेल खाने वाला वॉलेट पता (0x…, 42 अक्षर)। MetaMask से कॉपी करें। एक फॉलबैक मौजूद है, लेकिन इसे स्पष्ट रूप से सेट करना स्टार्टअप त्रुटियों को रोकता है।'),
    ('NODE_OPERATOR_WALLET','रिवॉर्ड के लिए', 'आपका Aequitas मानव वॉलेट पता — Android ऐप के माध्यम से पंजीकृत। आपके दैनिक वैलिडेटर रिवॉर्ड (सभी प्रोटोकॉल शुल्क का 40%) प्राप्त करता है। एक पंजीकृत मानव होना चाहिए।'),
    ('NODE_OPERATOR_BINDING_SIGNATURE', 'रिवॉर्ड के लिए', 'सिद्ध करता है कि NODE_OPERATOR_WALLET आपका है। इसे aequitas.digital/node-binding पर जनरेट करें: दिखाए गए संदेश को MetaMask में अपने मानव वॉलेट से साइन करें, परिणामी हस्ताक्षर यहां पेस्ट करें। इसके बिना आपका नोड फिर भी चलता है, लेकिन रिवॉर्ड के लिए स्वचालित रूप से पंजीकृत नहीं हो सकता।'),
    ('PEER_SECRET',         'वैकल्पिक/पुराना', 'पुरानी साझा-गुप्त फॉलबैक प्रणाली। अब आवश्यक नहीं है — नोड अब क्रिप्टोग्राफिक चैलेंज-रिस्पॉन्स (RELAYER_PRIVATE_KEY) के माध्यम से स्वचालित रूप से प्रमाणित होते हैं। केवल पुराने डिप्लॉयमेंट के साथ पिछड़ी संगतता के लिए आवश्यक है।'),
    ('SELF_URL',            'मल्टी-नोड',  'आपके अपने नोड का सार्वजनिक HTTPS URL (जैसे https://my-node.up.railway.app)। पीयर खोज में स्वयं को बाहर रखने के लिए आवश्यक। Railway में पाएं: Settings → Networking → Public Networking।'),
    ('PRIMARY_NODE_URL',    'मल्टी-नोड',  'इसे सेट करें: https://aequitas.digital — प्राइमरी नोड जिसके साथ आपका नोड स्वचालित पीयर खोज के लिए पंजीकृत होता है। स्टार्टअप पर, आपका नोड अपना URL + साइनिंग पता प्राइमरी को भेजता है, पूरी पीयर सूची प्राप्त करता है, और स्वचालित रूप से नेटवर्क से जुड़ जाता है।'),
    ('BOOTSTRAP_SNAPSHOT_URL', 'अनुशंसित', 'इसे सेट करें: https://aequitas.digital/api/snapshot — एक नए नोड को जेनेसिस से पूरे इतिहास को फिर से चलाने के बजाय नेटवर्क की वर्तमान स्थिति से शुरू करने देता है। काफी तेज पहली सिंक।'),
    ('BOOTSTRAP_SIGNER',    'स्नैपशॉट के साथ', 'प्राइमरी नोड का साइनिंग पता, इम्पोर्ट करने से पहले स्नैपशॉट के असली होने को सत्यापित करने के लिए उपयोग किया जाता है। वर्तमान मान https://aequitas.digital/api/status → "signing_address" से प्राप्त करें। जब भी BOOTSTRAP_SNAPSHOT_URL सेट हो, आवश्यक है।'),
    ('SNAPSHOT_TOKEN',      'वैकल्पिक',   'एक नए नोड को बूटस्ट्रैप करने के लिए आवश्यक नहीं — इसके बिना भी आपको सही ढंग से चलने के लिए आवश्यक सब कुछ मिलता है (अकाउंट्स, बैलेंस, पूल, कॉन्फिग)। यह केवल पूर्ण एक्सपोर्ट (nullifier/वॉलेट लिंकेज + bio_registrations) को अनलॉक करता है, जो पहले से डायवर्ज हो चुके नोड के आधिकारिक रीसिंक के लिए उपयोग होता है। नेटवर्क ऑपरेटर से केवल तभी पूछें जब आपको वास्तव में इसकी आवश्यकता हो।'),
    ('RESYNC_FROM_SNAPSHOT', 'केवल रिकवरी', 'खतरनाक, अस्थायी: इसे true पर सेट करें केवल BOOTSTRAP_SNAPSHOT_URL और BOOTSTRAP_SIGNER के साथ, ऐसे नोड को रिकवर करने के लिए जिसकी स्थिति नेटवर्क से भिन्न हो गई है। स्थानीय स्थिति को पूरी तरह से बदल देता है। एक बार रीस्टार्ट करें, फिर इस वेरिएबल को फिर से हटा दें — इसे छोड़ने से हर रीस्टार्ट पर पूर्ण रीसिंक होगा।'),
    ('AUTO_HEAL_ON_DIVERGENCE', 'अत्यधिक अनुशंसित', 'नेटवर्क की सुरक्षा और गति के लिए महत्वपूर्ण: यदि आपके नोड की चेन कभी नेटवर्क से भिन्न हो जाती है (जैसे डाउनटाइम या असफल रीस्टार्ट के बाद), तो यह इसे स्वयं पहचान लेता है — हर कुछ मिनट में PRIMARY_NODE_URL से तुलना करके — और बिना किसी मैनुअल RESYNC_FROM_SNAPSHOT के अपने आप रीसिंक हो जाता है। एक नोड जो भिन्न बना रहता है और अपने खुद के ब्लॉक प्रसारित करता रहता है, वह पूरे नेटवर्क को बाकी सभी ऑपरेटरों के लिए धीमा या अस्थिर कर सकता है। इसे PRIMARY_NODE_URL, BOOTSTRAP_SNAPSHOT_URL, और BOOTSTRAP_SIGNER के साथ true पर सेट करें।'),
    ('PORT',                'नहीं',       'Railway पर सेट न करें — Railway इसे स्वचालित रूप से सेट करता है। डिफॉल्ट 8080 है।'),
    ('NODE_KEY',            'नहीं',       'स्थिर P2P पहचान के लिए Base64 libp2p की। यदि छोड़ दिया जाए तो स्वचालित रूप से जनरेट होती है, लेकिन हर रीस्टार्ट पर बदल जाती है। यदि सेट नहीं है, तो नोड इसे stderr में प्रिंट करता है: "SAVE THIS AS NODE_KEY ENVIRONMENT VAR: <base64>"। इसे कॉपी करके यहां पेस्ट करें।'),
    ('IS_PRIMARY_NODE',     'नहीं',       'इसे अनसेट या false छोड़ें। डिस्ट्रीब्यूशन अब एक डेटाबेस-स्तरीय लॉक का उपयोग करता है — कोई भी नोड इस वेरिएबल के बिना इसे चला सकता है।'),
    ('RESET_STATE',         'नहीं',       'खतरनाक: इसे true पर सेट करने से हर रीस्टार्ट पर आपका पूरा डेटाबेस मिट जाता है। केवल डेवलपमेंट उपयोग के लिए। प्रोडक्शन में कभी नहीं।'),
],

railway_title = 'चरण 2 — Railway पर डिप्लॉय करें (अनुशंसित)',
railway_intro = 'Railway आपके नोड को चलाने का सबसे आसान तरीका है — कोई सर्वर सेटअप नहीं, कोई कमांड लाइन नहीं। मुफ्त टियर सभी आवश्यकताओं को कवर करता है। कुल समय: लगभग 10–15 मिनट।',
railway_steps = [
    'github.com/hanoi96international-gif/Aequitas को अपने GitHub खाते में फोर्क करें (<b>Fork</b> → <b>Create fork</b> पर क्लिक करें)',
    'railway.app पर, GitHub से साइन इन करें, फिर <b>New Project</b> → <b>+ New</b> → <b>Database</b> → <b>Add PostgreSQL</b>',
    'उसी Railway प्रोजेक्ट में, <b>+ New → GitHub Repo</b> पर क्लिक करें और अपना Aequitas फोर्क चुनें — Railway स्वचालित रूप से Dockerfile का पता लगाता है',
    '<b>Deploy Now</b> पर क्लिक करें — पहला बिल्ड शुरू होता है (env vars के बिना यह फेल हो सकता है, यह सामान्य है)',
    'अपनी Aequitas सेवा पर क्लिक करें → <b>Variables</b> → हर वेरिएबल जोड़ें (ऊपर तालिका देखें)। न्यूनतम आवश्यक: RELAYER_PRIVATE_KEY, RELAYER_ADDRESS, NODE_OPERATOR_WALLET, SELF_URL, PRIMARY_NODE_URL=https://aequitas.digital (PEER_SECRET अब आवश्यक नहीं है)',
    '<b>Deploy</b> पर क्लिक करें (या ऑटो-रीडिप्लॉय ट्रिगर करने के लिए वेरिएबल्स सेव करें)। Go नोड बाइनरी को कंपाइल करने में बिल्ड को ~3 मिनट लगते हैं।',
    '<b>Deploy Logs</b> देखें। सफलता इस तरह दिखती है: <font name="Courier" color="#5B21B6">Aequitas Node Running</font> और <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    'अपना सार्वजनिक URL पाने के लिए <b>Settings → Networking → Generate Domain</b> पर जाएं',
    '<font name="Courier">https://YOUR-URL/api/status</font> खोलें — आपको JSON दिखना चाहिए जिसमें <b>height</b> हर ~6 सेकंड में बढ़ रहा है',
],
railway_vars_code = (
    '# यदि PostgreSQL एक ही प्रोजेक्ट में है तो Railway स्वचालित रूप से DATABASE_URL सेट करता है\n'
    'RELAYER_PRIVATE_KEY    = 0xआपकी_प्राइवेट_की\n'
    'RELAYER_ADDRESS        = 0xआपका_नोड_वॉलेट_पता\n'
    'NODE_OPERATOR_WALLET   = 0xआपका_मानव_वॉलेट\n'
    '# PEER_SECRET अब आवश्यक नहीं है — प्रमाणीकरण स्वचालित है\n'
    'SELF_URL               = https://YOUR-RAILWAY-DOMAIN.up.railway.app\n'
    'PRIMARY_NODE_URL       = https://aequitas.digital'
),

docker_title = 'चरण 2b — विकल्प: Docker के साथ डिप्लॉय करें (एडवांस्ड)',
docker_intro = 'यदि आपके पास अपना सर्वर है (VPS, होम सर्वर, क्लाउड VM) तो इसका उपयोग करें। Docker और एक PostgreSQL डेटाबेस की आवश्यकता है।',
docker_code  = (
    '# 1. कोड डाउनलोड करें\n'
    'git clone https://github.com/hanoi96international-gif/Aequitas && cd Aequitas\n\n'
    '# 2. नोड इमेज बनाएं (Go कंपाइलेशन के लिए ~3 मिनट)\n'
    'docker build -t aequitas-node .\n\n'
    '# 3. नोड शुरू करें\n'
    'docker run -d --name aequitas-node --restart unless-stopped \\\n'
    '  -e DATABASE_URL="postgres://user:pass@host:5432/aequitas" \\\n'
    '  -e RELAYER_PRIVATE_KEY="0xआपकी_प्राइवेट_की" \\\n'
    '  -e RELAYER_ADDRESS="0xआपका_नोड_वॉलेट_पता" \\\n'
    '  -e NODE_OPERATOR_WALLET="0xआपका_मानव_वॉलेट" \\\n'
    '  # -e PEER_SECRET="..." (वैकल्पिक/पुराना, आवश्यक नहीं) \\\n'
    '  -e SELF_URL="https://आपका-सार्वजनिक-URL" \\\n'
    '  -e PRIMARY_NODE_URL="https://aequitas.digital" \\\n'
    '  -p 8080:8080 aequitas-node\n\n'
    '# 4. लाइव लॉग देखें\n'
    'docker logs -f aequitas-node'
),

verify_title = 'चरण 3 — सत्यापित करें कि आपका नोड चल रहा है',
verify_body  = 'इन URLs को अपने ब्राउज़र में खोलें। YOUR-NODE-URL को अपने वास्तविक Railway डोमेन या सर्वर पते से बदलें।',
verify_code  = (
    'https://YOUR-NODE-URL/api/status\n'
    ' → अनुमानित: {"height": 1234, "total_humans": N, "aequitas_index": N}\n\n'
    'https://YOUR-NODE-URL/rpc\n'
    ' → अनुमानित: {"jsonrpc":"2.0","error":"method not specified"} — RPC चल रहा है'
),
verify_note  = 'स्टार्टअप के कुछ सेकंड के भीतर ब्लॉक हाइट को प्राइमरी नोड से 1–2 ब्लॉक के भीतर मिलान करना चाहिए। यदि यह 0 पर रहता है, तो जांचें कि PRIMARY_NODE_URL=https://aequitas.digital सेट है और पहुंच योग्य है।',

valkey_title = 'चरण 3b — अपनी वैलिडेटर की पंजीकृत करें (विकेन्द्रीकृत प्रमाणीकरण)',
valkey_body  = 'साझा PEER_SECRET के बजाय, अपने नोड साइनिंग की को अपने मानव वॉलेट के साथ पंजीकृत करें। यह क्रिप्टोग्राफिक रूप से सिद्ध करता है कि आप दोनों कीज़ को नियंत्रित करते हैं। अपने सर्वर पर यह चलाकर हस्ताक्षर प्राप्त करें (SSH/Railway shell):',
valkey_code  = 'curl "http://localhost:8080/api/sign-validator-challenge?wallet=0xआपका_मानव_वॉलेट"',
valkey_note  = 'फिर वेबसाइट पर Network → Run a Node टैब का उपयोग करें और पंजीकरण पूरा करने के लिए "Sign with MetaMask & Register" पर क्लिक करें।',

mm_title = 'चरण 4 — MetaMask को अपने नोड से कनेक्ट करें (वैकल्पिक)',
mm_body  = 'MetaMask में: नेटवर्क ड्रॉपडाउन पर क्लिक करें → Add network → Add a network manually, फिर दर्ज करें:',
mm_rows  = [
    ('Network Name',    'Aequitas Chain'),
    ('RPC URL',         'https://YOUR-NODE-URL/rpc'),
    ('Chain ID',        '1926'),
    ('Currency Symbol', 'AEQ'),
    ('Decimals',        '18'),
    ('Block Explorer',  'https://aequitas.digital'),
],

rewards_title = 'चरण 5 — वैलिडेटर रिवॉर्ड कमाना',
rewards_box   = 'वैलिडेटर्स पूल सभी प्रोटोकॉल शुल्क का 40% एकत्र करता है (स्वैप शुल्क, डिमरेज, वेल्थ कैप अतिरेक)। हर दिन बर्लिन समय 20:00 बजे (CEST/CET, DST को स्वचालित रूप से हैंडल करता है) नोड पूल बैलेंस को उत्पादित ब्लॉकों के अनुपात में सभी पंजीकृत नोड ऑपरेटरों में वितरित करता है। आपका नोड जितना अधिक लगातार चलता है, आपका हिस्सा उतना ही बड़ा होता है।',
rewards_steps = [
    'सुनिश्चित करें कि आप Aequitas पर मानव के रूप में पंजीकृत हैं। यदि नहीं: पहले Android ऐप इंस्टॉल करें और बायोमेट्रिक पंजीकरण पूरा करें। आपको एक वॉलेट पता और 1,000 AEQ प्राप्त होगा।',
    'अपने Railway Variables में <font name="Courier">NODE_OPERATOR_WALLET</font> = अपना Aequitas मानव वॉलेट पता सेट करें',
    'सेव करें — Railway स्वचालित रूप से रीडिप्लॉय करता है। Docker के साथ: <font name="Courier">docker restart aequitas-node</font>',
    'अपने नोड लॉग्स में पुष्टि करें: <font name="Courier" color="#0F766E">[NODE] Registered node operator wallet: 0x…</font>',
    'रिवॉर्ड स्वचालित रूप से हर दिन बर्लिन समय 20:00 बजे (CEST/CET) वितरित होते हैं। केवल अपना नोड चलते रहने दें — कोई और कार्रवाई आवश्यक नहीं।',
],

trouble_title = 'समस्या निवारण',
trouble_cols  = ['लक्षण', 'संभावित कारण', 'समाधान'],
trouble_rows  = [
    ('ब्लॉक हाइट 0 पर रहता है',           'PRIMARY_NODE_URL सेट नहीं है या गलत है', 'PRIMARY_NODE_URL=https://aequitas.digital सेट करें और रीडिप्लॉय करें। SELF_URL को भी अपने नोड के सार्वजनिक URL से सेट करें।'),
    ('स्टार्टअप पर DATABASE_URL त्रुटि',   'गलत कनेक्शन स्ट्रिंग',                    'फॉर्मेट जांचें: postgres://user:pass@host:5432/dbname — सुनिश्चित करें कि PostgreSQL चल रहा है और पहुंच योग्य है।'),
    ('लॉग्स में "no code at address"',     'V7 कॉन्ट्रैक्ट अभी डिप्लॉय नहीं हुआ',     'पहले स्टार्टअप पर सामान्य — नोड स्वचालित रूप से V7 डिप्लॉय करता है। कुछ सेकंड प्रतीक्षा करें और फिर से जांचें।'),
    ('"NODE_OPERATOR_WALLET not set"',     'एनवायरनमेंट वेरिएबल गायब',               'NODE_OPERATOR_WALLET=0xआपका_मानव_वॉलेट जोड़ें। नोड इसके बिना ठीक चलता है लेकिन आपको रिवॉर्ड नहीं मिलेंगे।'),
    ('Railway पर "Application error"',    'बिल्ड या स्टार्टअप विफलता',               'Deploy Logs जांचें। सबसे सामान्य: DATABASE_URL गायब या RELAYER_PRIVATE_KEY गलत फॉर्मेट में (0x से शुरू होना चाहिए)।'),
    ('पोर्ट 8080 पहुंच योग्य नहीं (Docker)', 'फायरवॉल या क्लाउड प्रोवाइडर कॉन्फिग',   'अपने फायरवॉल या क्लाउड सिक्योरिटी ग्रुप सेटिंग्स में इनबाउंड TCP पोर्ट 8080 खोलें।'),
    ('Docker बिल्ड विफल (मॉड्यूल त्रुटि)', 'बिल्ड के दौरान इंटरनेट नहीं',             'Docker बिल्ड को Go मॉड्यूल डाउनलोड करने के लिए आउटबाउंड इंटरनेट की आवश्यकता है। Railway इसे स्वचालित रूप से संभालता है।'),
],

footer = 'Aequitas Chain · Chain ID 1926 · aequitas.digital · वैलिडेटर रिवॉर्ड: हर दिन बर्लिन समय 20:00 बजे (CEST/CET)',
)

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
