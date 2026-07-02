"""Generates Aequitas Node Operator Guide PDFs in EN and DE."""
from reportlab.lib.pagesizes import A4
from reportlab.lib.styles import getSampleStyleSheet, ParagraphStyle
from reportlab.lib.units import cm
from reportlab.lib.colors import HexColor, white, black
from reportlab.platypus import (SimpleDocTemplate, Paragraph, Spacer, Table,
                                 TableStyle, HRFlowable, KeepTogether)
from reportlab.lib.enums import TA_LEFT, TA_CENTER
from reportlab.platypus import Flowable

W, H = A4

# Colors
GOLD   = HexColor('#F0B429')
PURPLE = HexColor('#9B72F6')
TEAL   = HexColor('#22D3EE')
NEON   = HexColor('#34D399')
RED    = HexColor('#F87171')
BG     = HexColor('#0C0E16')
CARD   = HexColor('#131620')
MUTED  = HexColor('#8892A4')
TEXT   = HexColor('#E8EDF5')

def styles():
    s = getSampleStyleSheet()
    base = dict(fontName='Helvetica', textColor=TEXT, backColor=None,
                leading=14, spaceAfter=4)
    def P(name, **kw):
        d = {**base, **kw}
        return ParagraphStyle(name, **d)

    return {
        'title':    P('T',  fontName='Helvetica-Bold', fontSize=22, textColor=GOLD,
                        leading=28, spaceAfter=4, alignment=TA_CENTER),
        'subtitle': P('ST', fontName='Helvetica', fontSize=10, textColor=MUTED,
                        alignment=TA_CENTER, spaceAfter=16),
        'h1':       P('H1', fontName='Helvetica-Bold', fontSize=13, textColor=PURPLE,
                        spaceBefore=16, spaceAfter=6),
        'h2':       P('H2', fontName='Helvetica-Bold', fontSize=10, textColor=GOLD,
                        spaceBefore=10, spaceAfter=4),
        'body':     P('B',  fontSize=9, textColor=TEXT, leading=14, spaceAfter=6),
        'code':     P('C',  fontName='Courier', fontSize=8, textColor=NEON,
                        backColor=HexColor('#070B16'), leading=12,
                        leftIndent=8, rightIndent=8, spaceAfter=8,
                        borderPad=6),
        'warn':     P('W',  fontSize=8, textColor=RED, leading=12, spaceAfter=6,
                        leftIndent=8),
        'note':     P('N',  fontSize=8, textColor=TEAL, leading=12, spaceAfter=6,
                        leftIndent=8),
        'muted':    P('M',  fontSize=8, textColor=MUTED, leading=12, spaceAfter=4),
        'bullet':   P('BU', fontSize=9, textColor=TEXT, leading=13,
                        leftIndent=14, spaceAfter=3),
    }

def hr():
    return HRFlowable(width='100%', thickness=0.5,
                      color=HexColor('#1E2D45'), spaceAfter=10, spaceBefore=4)

def make_doc(path, lang='en'):
    doc = SimpleDocTemplate(path, pagesize=A4,
                            leftMargin=2*cm, rightMargin=2*cm,
                            topMargin=2*cm, bottomMargin=2*cm,
                            title=f'Aequitas Node Operator Guide ({lang.upper()})')
    S = styles()
    story = []

    # ── HEADER ────────────────────────────────────────────────────────────────
    if lang == 'en':
        story += [
            Paragraph('AEQUITAS NODE OPERATOR GUIDE', S['title']),
            Paragraph('Version 1.0 · June 2026 · aequitas.digital', S['subtitle']),
            Paragraph('Complete step-by-step guide · No prior blockchain experience required · Estimated time: 20–30 min', S['muted']),
            hr(),
        ]
    else:
        story += [
            Paragraph('AEQUITAS NODE-BETREIBER-ANLEITUNG', S['title']),
            Paragraph('Version 1.0 · Juni 2026 · aequitas.digital', S['subtitle']),
            Paragraph('Vollstaendige Schritt-fuer-Schritt-Anleitung · Keine Blockchain-Vorkenntnisse erforderlich · Geschaetzte Zeit: 20-30 Min.', S['muted']),
            hr(),
        ]

    # ── WHAT IS A NODE ─────────────────────────────────────────────────────────
    if lang == 'en':
        story += [
            Paragraph('What is an Aequitas Node?', S['h1']),
            Paragraph(
                'An Aequitas node is a program that runs in the cloud and participates in the Aequitas network. '
                'It keeps a copy of the entire blockchain, validates who is a registered human, and produces new blocks. '
                'The more nodes exist, the more decentralized and resilient the network becomes. '
                'As a reward for running a node, you receive a daily share of all protocol fees — '
                '<b>automatically at 20:00 Berlin time (CEST/CET)</b>, no further action required.',
                S['body']),
        ]
    else:
        story += [
            Paragraph('Was ist ein Aequitas-Node?', S['h1']),
            Paragraph(
                'Ein Aequitas-Node ist ein Programm, das in der Cloud laeuft und am Aequitas-Netzwerk teilnimmt. '
                'Er speichert eine Kopie der gesamten Blockchain, validiert die Menschenregistrierung und produziert neue Bloecke. '
                'Je mehr Nodes existieren, desto dezentraler und widerstandsfaehiger wird das Netzwerk. '
                'Als Belohnung erhaeltst du taeglich einen Anteil der Protokollgebuehren — '
                '<b>automatisch um 20:00 Uhr Berliner Zeit (CEST/CET)</b>, kein weiterer Aufwand erforderlich.',
                S['body']),
        ]

    story.append(hr())

    # ── PREREQUISITES ──────────────────────────────────────────────────────────
    if lang == 'en':
        story.append(Paragraph('Before You Start — What You Need', S['h1']))
        prereqs = [
            ('1.', '<b>Aequitas account:</b> Register as a human via the Android app. You need a wallet address to receive rewards.'),
            ('2.', '<b>GitHub account (free):</b> Go to github.com — you need this to fork the Aequitas code.'),
            ('3.', '<b>Railway account (free):</b> Go to railway.app and sign in with GitHub. No server or command line required.'),
            ('4.', '<b>Dedicated node wallet:</b> A separate MetaMask wallet for your node (NOT your personal AEQ wallet). '
                   'Export its private key: MetaMask → Account Details → Show Private Key → enter password → copy. '
                   '<b>Keep this key strictly private.</b>'),
            ('5.', '<b>10–30 minutes of your time.</b> Railway does most of the work automatically.'),
        ]
    else:
        story.append(Paragraph('Vor dem Start — Was du brauchst', S['h1']))
        prereqs = [
            ('1.', '<b>Aequitas-Konto:</b> Registriere dich als Mensch ueber die Android-App. Du brauchst eine Wallet-Adresse fuer Belohnungen.'),
            ('2.', '<b>GitHub-Konto (kostenlos):</b> Gehe zu github.com — du brauchst es um den Aequitas-Code zu forken.'),
            ('3.', '<b>Railway-Konto (kostenlos):</b> Gehe zu railway.app und melde dich mit GitHub an. Kein Server oder Terminal erforderlich.'),
            ('4.', '<b>Dedizierte Node-Wallet:</b> Eine separate MetaMask-Wallet fuer deinen Node (NICHT deine persoenliche AEQ-Wallet). '
                   'Exportiere den privaten Schluessel: MetaMask → Kontodetails → Privaten Schluessel anzeigen → Passwort eingeben → kopieren. '
                   '<b>Diesen Schluessel streng geheimhalten.</b>'),
            ('5.', '<b>10–30 Minuten Zeit.</b> Railway erledigt den Grossteil automatisch.'),
        ]

    for num, text in prereqs:
        story.append(Paragraph(f'<font color="#F0B429"><b>{num}</b></font>  {text}', S['bullet']))
    story.append(hr())

    # ── ENV VARS TABLE ─────────────────────────────────────────────────────────
    if lang == 'en':
        story.append(Paragraph('Environment Variables — Complete Reference', S['h1']))
        story.append(Paragraph('Collect these values before deploying. Enter them in Railway → Variables.', S['body']))
        story.append(Paragraph('SECURITY: Your RELAYER_PRIVATE_KEY is like a master password. Never share it.', S['warn']))
        hdr = ['Variable', 'Required?', 'What to set']
        rows = [
            ('DATABASE_URL', 'YES', 'Auto-injected by Railway when PostgreSQL is in the same project.\nFormat: postgres://user:pass@host:5432/dbname'),
            ('RELAYER_PRIVATE_KEY', 'YES', 'Private key of your dedicated node wallet (starts with 0x, 66 chars).\nMetaMask → Account Details → Show Private Key'),
            ('RELAYER_ADDRESS', 'Recommended', 'Public address matching RELAYER_PRIVATE_KEY (0x, 42 chars).\nPrevents startup errors.'),
            ('NODE_OPERATOR_WALLET', 'For rewards', 'Your Aequitas HUMAN wallet address (the one you registered with).\nThis receives daily validator rewards at 20:00 Berlin time.'),
            ('NODE_OPERATOR_BINDING_SIGNATURE', 'For rewards', 'Proves you own NODE_OPERATOR_WALLET. Generate at aequitas.digital/node-binding:\nsign the shown message with your HUMAN wallet in MetaMask, paste the\nresulting signature here. Without it your node still runs, but cannot\nauto-register for validator rewards.'),
            ('PEER_SECRET', 'Optional/Legacy', 'Legacy shared-secret fallback. No longer required — nodes authenticate\nautomatically via cryptographic challenge-response (RELAYER_PRIVATE_KEY).\nOnly needed for backward compatibility with older deployments.'),
            ('SELF_URL', 'For multi-node', 'Your node public URL: https://YOUR-NAME.up.railway.app\nFind in Railway → Settings → Networking → Public Networking.'),
            ('PRIMARY_NODE_URL', 'For multi-node', 'Set to: https://aequitas.digital\nYour node registers here automatically on startup.'),
            ('BOOTSTRAP_SNAPSHOT_URL', 'Recommended', 'Set to: https://aequitas.digital/api/snapshot\nLets a brand-new node start from the network\'s current state instead of\nreplaying the entire history from genesis — dramatically faster first sync.'),
            ('BOOTSTRAP_SIGNER', 'With snapshot', 'Primary node\'s signing address — verifies the snapshot is genuine before\nimporting it. Get the current value from https://aequitas.digital/api/status\n→ "signing_address" field. Required whenever BOOTSTRAP_SNAPSHOT_URL is set.'),
            ('SNAPSHOT_TOKEN', 'Optional', 'Not required to bootstrap — without it you still get everything needed to\nrun correctly (accounts, balances, pool, config). Only unlocks the full\nexport (nullifier/wallet + bio_registrations) used for authoritative\nresync of an already-diverged node. Ask the network operator if you need it.'),
            ('RESYNC_FROM_SNAPSHOT', 'Recovery only', 'DANGEROUS, temporary: set to true together with BOOTSTRAP_SNAPSHOT_URL and\nBOOTSTRAP_SIGNER ONLY to recover a node whose state has diverged from the\nnetwork. Replaces local state outright. Restart once, then remove this\nvariable again — leaving it set forces a full resync on every restart.'),
            ('AUTO_HEAL_ON_DIVERGENCE', 'Strongly recommended', 'Important for network security and speed: if your node\'s chain ever\ndiverges (e.g. after downtime or a bad restart), it detects this itself by\ncomparing against PRIMARY_NODE_URL every few minutes and resyncs\nautomatically — no manual RESYNC_FROM_SNAPSHOT needed. A node that stays\ndiverged and keeps broadcasting its own blocks can slow down or destabilize\nthe network for every other operator.'),
            ('NODE_KEY', 'Optional', 'Base64-encoded libp2p private key for stable peer identity.\nIf not set: auto-generated on first start, printed to stderr as\n"SAVE THIS AS NODE_KEY ENVIRONMENT VAR: <base64>"\nCopy that value here to keep a stable node ID across restarts.'),
            ('IS_PRIMARY_NODE', 'NO', 'Leave unset (or false). Only the official primary node uses true.\nSetting true on secondary nodes causes double pool distributions.'),
            ('RESET_STATE', 'NO', 'DANGEROUS: wipes the entire database on restart. Development only.'),
        ]
    else:
        story.append(Paragraph('Umgebungsvariablen — Vollstaendige Referenz', S['h1']))
        story.append(Paragraph('Sammle diese Werte vor dem Deployment. Eingabe in Railway → Variables.', S['body']))
        story.append(Paragraph('SICHERHEIT: Dein RELAYER_PRIVATE_KEY ist wie ein Master-Passwort. Niemals teilen.', S['warn']))
        hdr = ['Variable', 'Erforderlich?', 'Was eintragen']
        rows = [
            ('DATABASE_URL', 'JA', 'Von Railway automatisch gesetzt wenn PostgreSQL im selben Projekt.\nFormat: postgres://user:pass@host:5432/dbname'),
            ('RELAYER_PRIVATE_KEY', 'JA', 'Privater Schluessel deiner Node-Wallet (beginnt mit 0x, 66 Zeichen).\nMetaMask → Kontodetails → Privaten Schluessel anzeigen'),
            ('RELAYER_ADDRESS', 'Empfohlen', 'Oeffentliche Adresse passend zu RELAYER_PRIVATE_KEY (0x, 42 Zeichen).\nVerhindert Startfehler.'),
            ('NODE_OPERATOR_WALLET', 'Fuer Bel.', 'Deine Aequitas-Mensch-Wallet-Adresse (die registrierte).\nErhaelt taeglich Validator-Belohnungen um 20:00 Uhr Berliner Zeit.'),
            ('NODE_OPERATOR_BINDING_SIGNATURE', 'Fuer Bel.', 'Beweist dass dir NODE_OPERATOR_WALLET gehoert. Erzeugen unter\naequitas.digital/node-binding: angezeigte Nachricht mit deiner\nMensch-Wallet in MetaMask signieren, Signatur hier eintragen. Ohne sie\nlaeuft der Node trotzdem, kann sich aber nicht fuer Belohnungen registrieren.'),
            ('PEER_SECRET', 'Optional/Legacy', 'Legacy-Fallback. Nicht mehr erforderlich — Nodes authentifizieren sich\nautomatisch per kryptografischer Challenge-Response (RELAYER_PRIVATE_KEY).\nNur fuer Rueckwaertskompatibilitaet mit aelteren Deployments benoetigt.'),
            ('SELF_URL', 'Multi-Node', 'Oeffentliche URL des Nodes: https://DEIN-NAME.up.railway.app\nIn Railway → Settings → Networking → Public Networking.'),
            ('PRIMARY_NODE_URL', 'Multi-Node', 'Setzen auf: https://aequitas.digital\nDein Node registriert sich dort automatisch beim Start.'),
            ('BOOTSTRAP_SNAPSHOT_URL', 'Empfohlen', 'Setzen auf: https://aequitas.digital/api/snapshot\nLaesst einen neuen Node mit dem aktuellen Netzwerk-Stand starten statt die\ngesamte Historie ab Genesis nachzuspielen — deutlich schnellerer Erststart.'),
            ('BOOTSTRAP_SIGNER', 'Mit Snapshot', 'Signing-Adresse des Primaer-Nodes — prueft Echtheit des Snapshots vor dem\nImport. Aktuellen Wert unter https://aequitas.digital/api/status →\n"signing_address" finden. Erforderlich wenn BOOTSTRAP_SNAPSHOT_URL gesetzt ist.'),
            ('SNAPSHOT_TOKEN', 'Optional', 'Nicht erforderlich zum Bootstrappen — auch ohne erhaeltst du alles Noetige\n(Accounts, Salden, Pool, Config). Schaltet nur den vollen Export frei\n(Nullifier/Wallet-Verknuepfung + bio_registrations) fuer den autoritativen\nResync eines bereits divergenten Nodes. Beim Netzwerkbetreiber erfragen falls noetig.'),
            ('RESYNC_FROM_SNAPSHOT', 'Nur Recovery', 'GEFAEHRLICH, temporaer: nur zusammen mit BOOTSTRAP_SNAPSHOT_URL und\nBOOTSTRAP_SIGNER setzen, um einen vom Netzwerk abgewichenen Node zu\nreparieren. Ersetzt den lokalen Zustand komplett. Einmal neu starten, dann\ndiese Variable wieder entfernen — sonst erfolgt bei jedem Neustart ein voller Resync.'),
            ('AUTO_HEAL_ON_DIVERGENCE', 'Dringend empfohlen', 'Wichtig fuer Sicherheit und Geschwindigkeit des Netzwerks: Wenn die Chain\ndeines Nodes je abweicht (z.B. nach Downtime oder fehlgeschlagenem Neustart),\nerkennt er das selbststaendig durch Abgleich mit PRIMARY_NODE_URL alle paar\nMinuten und synct sich automatisch neu — ganz ohne manuelles\nRESYNC_FROM_SNAPSHOT. Ein abweichender Node, der weiter eigene Bloecke\nverbreitet, kann das gesamte Netzwerk fuer alle anderen verlangsamen.'),
            ('NODE_KEY', 'Optional', 'Base64-kodierter libp2p-Schluessel fuer stabile Peer-Identitaet.\nWenn nicht gesetzt: automatisch generiert, in stderr ausgegeben als\n"SAVE THIS AS NODE_KEY ENVIRONMENT VAR: <base64>"\nDiesen Wert kopieren und hier eintragen.'),
            ('IS_PRIMARY_NODE', 'NEIN', 'Nicht setzen (oder false). Nur der offizielle Primaer-Node nutzt true.\nAuf Sekundaer-Nodes verursacht true doppelte Pool-Ausschuettungen.'),
            ('RESET_STATE', 'NEIN', 'GEFAEHRLICH: loescht die gesamte DB beim Neustart. Nur Entwicklung.'),
        ]

    col_w = [3.8*cm, 2.2*cm, 10.5*cm]
    table_data = [[Paragraph(f'<b>{h}</b>', ParagraphStyle('th', fontName='Helvetica-Bold',
                   fontSize=8, textColor=TEXT, leading=10)) for h in hdr]]
    for var, req, desc in rows:
        req_color = '#F87171' if req in ('YES','JA','For multi-node','Multi-Node') else '#22D3EE' if req in ('Recommended','Empfohlen') else '#34D399' if 'reward' in req.lower() or 'Bel' in req else '#8892A4'
        table_data.append([
            Paragraph(f'<font name="Courier" color="#34D399">{var}</font>',
                      ParagraphStyle('v', fontName='Courier', fontSize=8, textColor=NEON, leading=10)),
            Paragraph(f'<font color="{req_color}"><b>{req}</b></font>',
                      ParagraphStyle('r', fontSize=8, textColor=TEXT, leading=10)),
            Paragraph(desc.replace('\n', '<br/>'),
                      ParagraphStyle('d', fontSize=8, textColor=MUTED, leading=12)),
        ])

    t = Table(table_data, colWidths=col_w, repeatRows=1)
    t.setStyle(TableStyle([
        ('BACKGROUND', (0,0), (-1,0), HexColor('#1A1D2B')),
        ('ROWBACKGROUNDS', (0,1), (-1,-1), [HexColor('#0C0E16'), HexColor('#131620')]),
        ('GRID', (0,0), (-1,-1), 0.3, HexColor('#1E2D45')),
        ('VALIGN', (0,0), (-1,-1), 'TOP'),
        ('TOPPADDING', (0,0), (-1,-1), 5),
        ('BOTTOMPADDING', (0,0), (-1,-1), 5),
        ('LEFTPADDING', (0,0), (-1,-1), 6),
        ('RIGHTPADDING', (0,0), (-1,-1), 6),
    ]))
    story.append(t)
    story.append(Spacer(1, 10))
    story.append(hr())

    # ── DEPLOYMENT STEPS ───────────────────────────────────────────────────────
    if lang == 'en':
        story.append(Paragraph('Step-by-Step Deployment on Railway', S['h1']))
        steps = [
            ('Step 1 — Fork the Repository',
             'Open github.com/hanoi96international-gif/Aequitas in your browser. '
             'Click <b>Fork</b> in the top-right → <b>Create fork</b>. '
             'GitHub creates a copy under your account.'),
            ('Step 2 — Create PostgreSQL Database',
             'Go to railway.app and sign in with GitHub. '
             'Click <b>New Project</b> → <b>+ New</b> → <b>Database</b> → <b>Add PostgreSQL</b>. '
             'Railway creates the database and will auto-inject DATABASE_URL when you add your node.'),
            ('Step 3 — Deploy Your Node',
             'In the same Railway project: <b>+ New</b> → <b>GitHub Repo</b> → select your Aequitas fork. '
             'Railway detects the Dockerfile automatically. Click <b>Deploy Now</b>.'),
            ('Step 4 — Set Environment Variables',
             'Click your Aequitas service → <b>Variables</b>. Add at minimum:\n'
             '  RELAYER_PRIVATE_KEY  = 0xYOUR_PRIVATE_KEY\n'
             '  RELAYER_ADDRESS      = 0xYOUR_NODE_WALLET_ADDRESS\n'
             '  NODE_OPERATOR_WALLET = 0xYOUR_HUMAN_WALLET\n'
             '  SELF_URL             = https://YOUR-RAILWAY-DOMAIN.up.railway.app\n'
             '  PRIMARY_NODE_URL     = https://aequitas.digital\n'
             'Note: PEER_SECRET is no longer required — authentication is automatic.\n'
             'Save → Railway auto-redeploys.'),
            ('Step 5 — Get Your Public URL',
             'Railway → Settings → Networking → <b>Generate Domain</b>. '
             'Open https://YOUR-URL/api/status — you should see JSON with <b>height</b> climbing every ~6 seconds.'),
            ('Step 6 — Verify Success',
             'Check deploy logs for:\n'
             '  ✓ API Server listening on port 8080\n'
             '  [SYNC] Connected to peer https://aequitas.digital\n'
             '  [NODE] Registered node operator wallet: 0x...\n'
             'Block height should match the primary node within 1-2 blocks.'),
            ('Step 7 — Earn Rewards',
             'Rewards are distributed automatically every day at 20:00 Berlin time (CEST/CET). '
             'The Validators Pool (40% of all protocol fees) is split proportionally among all registered node operators. '
             'Keep your node running — no further action needed.'),
        ]
    else:
        story.append(Paragraph('Schritt-fuer-Schritt-Deployment auf Railway', S['h1']))
        steps = [
            ('Schritt 1 — Repository forken',
             'Oeffne github.com/hanoi96international-gif/Aequitas in deinem Browser. '
             'Klicke <b>Fork</b> oben rechts → <b>Create fork</b>. '
             'GitHub erstellt eine Kopie unter deinem Konto.'),
            ('Schritt 2 — PostgreSQL-Datenbank erstellen',
             'Gehe zu railway.app und melde dich mit GitHub an. '
             'Klicke <b>New Project</b> → <b>+ New</b> → <b>Database</b> → <b>Add PostgreSQL</b>. '
             'Railway erstellt die Datenbank und setzt DATABASE_URL automatisch.'),
            ('Schritt 3 — Node deployen',
             'Im selben Railway-Projekt: <b>+ New</b> → <b>GitHub Repo</b> → deinen Aequitas-Fork auswaehlen. '
             'Railway erkennt das Dockerfile automatisch. Klicke <b>Deploy Now</b>.'),
            ('Schritt 4 — Umgebungsvariablen setzen',
             'Klicke deinen Aequitas-Service → <b>Variables</b>. Mindestens eintragen:\n'
             '  RELAYER_PRIVATE_KEY  = 0xDEIN_PRIVATER_SCHLUESSEL\n'
             '  RELAYER_ADDRESS      = 0xDEINE_NODE_WALLET_ADRESSE\n'
             '  NODE_OPERATOR_WALLET = 0xDEINE_MENSCH_WALLET\n'
             '  SELF_URL             = https://DEIN-RAILWAY-DOMAIN.up.railway.app\n'
             '  PRIMARY_NODE_URL     = https://aequitas.digital\n'
             'Hinweis: PEER_SECRET ist nicht mehr erforderlich — Authentifizierung ist automatisch.\n'
             'Speichern → Railway deployt automatisch neu.'),
            ('Schritt 5 — Oeffentliche URL erhalten',
             'Railway → Settings → Networking → <b>Generate Domain</b>. '
             'Oeffne https://DEINE-URL/api/status — du siehst JSON mit <b>height</b> der alle ~6 Sekunden steigt.'),
            ('Schritt 6 — Erfolg pruefen',
             'Pruefe die Deploy-Logs:\n'
             '  ✓ API Server listening on port 8080\n'
             '  [SYNC] Connected to peer https://aequitas.digital\n'
             '  [NODE] Registered node operator wallet: 0x...\n'
             'Die Blockhoehe sollte innerhalb von 1-2 Bloecken mit dem Primaer-Node uebereinstimmen.'),
            ('Schritt 7 — Belohnungen erhalten',
             'Belohnungen werden taeglich automatisch um 20:00 Uhr Berliner Zeit (CEST/CET) ausgeschuettet. '
             'Der Validators-Pool (40% aller Protokollgebuehren) wird proportional auf alle registrierten Node-Betreiber aufgeteilt. '
             'Node laufen lassen — kein weiterer Aufwand erforderlich.'),
        ]

    for title, text in steps:
        story.append(KeepTogether([
            Paragraph(title, S['h2']),
            Paragraph(text.replace('\n', '<br/>'), S['body']),
        ]))

    story.append(hr())

    # ── VALIDATOR KEY REGISTRATION ─────────────────────────────────────────────
    if lang == 'en':
        story += [
            Paragraph('Validator Registration (Automatic)', S['h1']),
            Paragraph(
                'Your node registers automatically on startup using a cryptographic challenge-response '
                'based on your RELAYER_PRIVATE_KEY. No shared PEER_SECRET is needed. '
                'The flow below describes what happens automatically, and can also be triggered manually.',
                S['body']),
            Paragraph('1. Request a challenge:', S['bullet']),
            Paragraph('GET https://aequitas.digital/api/peers/challenge?address=0xYOUR_RELAYER_ADDRESS', S['code']),
            Paragraph('2. Sign the returned challenge string with your node private key (RELAYER_PRIVATE_KEY) using MetaMask or ethers.js personal_sign.', S['bullet']),
            Paragraph('3. POST to https://aequitas.digital/api/peers/register with your signing_address and signature. '
                      'The challenge is valid for 90 seconds.', S['bullet']),
            Paragraph('On the website Network → Run a Node tab, use the "Sign with MetaMask & Register" button for the easiest flow.', S['note']),
        ]
    else:
        story += [
            Paragraph('Validator-Registrierung (Automatisch)', S['h1']),
            Paragraph(
                'Dein Node registriert sich beim Start automatisch per kryptografischer Challenge-Response '
                'basierend auf deinem RELAYER_PRIVATE_KEY. Ein gemeinsames PEER_SECRET ist nicht erforderlich. '
                'Der folgende Ablauf beschreibt was automatisch passiert und kann auch manuell ausgeloest werden.',
                S['body']),
            Paragraph('1. Challenge anfordern:', S['bullet']),
            Paragraph('GET https://aequitas.digital/api/peers/challenge?address=0xDEINE_RELAYER_ADDRESS', S['code']),
            Paragraph('2. Die zurueckgegebene Challenge-Zeichenkette mit deinem privaten Node-Schluessel (RELAYER_PRIVATE_KEY) signieren (MetaMask oder ethers.js personal_sign).', S['bullet']),
            Paragraph('3. POST an https://aequitas.digital/api/peers/register mit signing_address und signature. '
                      'Die Challenge ist 90 Sekunden gueltig.', S['bullet']),
            Paragraph('Auf der Website unter Network → Run a Node: Schaltflaeche "Sign with MetaMask & Register" fuer den einfachsten Ablauf.', S['note']),
        ]

    story.append(hr())

    # ── TROUBLESHOOTING ─────────────────────────────────────────────────────────
    if lang == 'en':
        story.append(Paragraph('Troubleshooting', S['h1']))
        issues = [
            ('Block height stays at 0', 'PRIMARY_NODE_URL not set', 'Set PRIMARY_NODE_URL=https://aequitas.digital and also set SELF_URL to your own node URL. Redeploy.'),
            ('DATABASE_URL error', 'Wrong connection string', 'Format: postgres://user:pass@host:5432/dbname — ensure PostgreSQL is running.'),
            ('"no code at address" in logs', 'V7 contract not deployed yet', 'Normal on first start — node auto-deploys V7. Wait a few seconds.'),
            ('No validator rewards', 'NODE_OPERATOR_WALLET not set', 'Add NODE_OPERATOR_WALLET=0xYOUR_HUMAN_WALLET to Railway Variables.'),
            ('Railway shows "Application error"', 'Build or startup failure', 'Check Deploy Logs. Most common: missing DATABASE_URL or wrong RELAYER_PRIVATE_KEY format (must start with 0x).'),
            ('Port 8080 not reachable (Docker)', 'Firewall config', 'Open TCP port 8080 inbound in your firewall or cloud security group.'),
        ]
    else:
        story.append(Paragraph('Fehlerbehebung', S['h1']))
        issues = [
            ('Blockhoehe bleibt bei 0', 'PRIMARY_NODE_URL nicht gesetzt', 'PRIMARY_NODE_URL=https://aequitas.digital setzen und SELF_URL auf deine Node-URL. Neu deployen.'),
            ('DATABASE_URL-Fehler', 'Falscher Connection-String', 'Format: postgres://user:pass@host:5432/dbname — PostgreSQL muss erreichbar sein.'),
            ('"no code at address" in Logs', 'V7-Contract noch nicht deployed', 'Normal beim ersten Start — Node deployed V7 automatisch. Kurz warten.'),
            ('Keine Validator-Belohnungen', 'NODE_OPERATOR_WALLET nicht gesetzt', 'NODE_OPERATOR_WALLET=0xDEINE_MENSCH_WALLET in Railway Variables hinzufuegen.'),
            ('Railway zeigt "Application error"', 'Build- oder Startfehler', 'Deploy-Logs pruefen. Haeufigste Ursache: fehlende DATABASE_URL oder falsches RELAYER_PRIVATE_KEY-Format (muss mit 0x beginnen).'),
            ('Port 8080 nicht erreichbar (Docker)', 'Firewall-Konfiguration', 'TCP-Port 8080 eingehend in Firewall oder Cloud-Security-Gruppe oeffnen.'),
        ]

    issue_data = [[Paragraph(f'<b>{h}</b>',
                   ParagraphStyle('th2', fontName='Helvetica-Bold', fontSize=8, textColor=TEXT, leading=10))
                   for h in (['Symptom', 'Likely Cause', 'Solution'] if lang=='en'
                              else ['Symptom', 'Wahrscheinliche Ursache', 'Loesung'])]]
    for symptom, cause, sol in issues:
        issue_data.append([
            Paragraph(symptom, ParagraphStyle('s', fontSize=8, textColor=MUTED, leading=11)),
            Paragraph(cause,   ParagraphStyle('c', fontSize=8, textColor=MUTED, leading=11)),
            Paragraph(sol,     ParagraphStyle('so', fontSize=8, textColor=TEXT, leading=11)),
        ])
    t2 = Table(issue_data, colWidths=[4.5*cm, 4*cm, 8*cm], repeatRows=1)
    t2.setStyle(TableStyle([
        ('BACKGROUND', (0,0), (-1,0), HexColor('#1A1D2B')),
        ('ROWBACKGROUNDS', (0,1), (-1,-1), [HexColor('#0C0E16'), HexColor('#131620')]),
        ('GRID', (0,0), (-1,-1), 0.3, HexColor('#1E2D45')),
        ('VALIGN', (0,0), (-1,-1), 'TOP'),
        ('TOPPADDING', (0,0), (-1,-1), 5),
        ('BOTTOMPADDING', (0,0), (-1,-1), 5),
        ('LEFTPADDING', (0,0), (-1,-1), 6),
        ('RIGHTPADDING', (0,0), (-1,-1), 6),
    ]))
    story.append(t2)
    story.append(hr())

    # ── FOOTER ─────────────────────────────────────────────────────────────────
    if lang == 'en':
        story.append(Paragraph('Questions & Feedback', S['h2']))
        story.append(Paragraph(
            'Open an issue on GitHub (github.com/hanoi96international-gif/Aequitas) '
            'or visit aequitas.digital for the latest information. '
            'BETA feedback on node setup, performance, and documentation gaps is especially welcome.',
            S['muted']))
        story.append(Spacer(1, 6))
        story.append(Paragraph(
            'Aequitas Chain · Chain ID 1926 · aequitas.digital · '
            'UBI distributions: every day at 20:00 Berlin time (CEST/CET)',
            S['muted']))
    else:
        story.append(Paragraph('Fragen & Feedback', S['h2']))
        story.append(Paragraph(
            'Oeffne ein Issue auf GitHub (github.com/hanoi96international-gif/Aequitas) '
            'oder besuche aequitas.digital fuer aktuellste Informationen. '
            'BETA-Feedback zu Node-Setup, Performance und Dokumentationsluecken ist sehr willkommen.',
            S['muted']))
        story.append(Spacer(1, 6))
        story.append(Paragraph(
            'Aequitas Chain · Chain ID 1926 · aequitas.digital · '
            'UBI-Ausschuettung: taeglich um 20:00 Uhr Berliner Zeit (CEST/CET)',
            S['muted']))

    doc.build(story)
    print(f'Generated: {path}')


if __name__ == '__main__':
    make_doc('C:/Users/aequitas-chain/downloads/Aequitas_Node_Guide_EN.pdf', 'en')
    make_doc('C:/Users/aequitas-chain/downloads/Aequitas_Node_Guide_DE.pdf', 'de')
    print('Done.')
