#!/usr/bin/env python3
"""All remaining website improvements."""
import sys

FILE = r'C:\Users\aequitas-chain\x\humanity\keeper\api_html.go'

with open(FILE, 'r', encoding='utf-8') as f:
    src = f.read()

replacements = []

# 1. cap-box HTML default
replacements.append((
    '    <div class="hlbox" data-i18n="cap-box">Cap = 25× current average AEQ balance of all verified humans<br>Automatically adjusts as the network grows and balances change<br>Applies to ALL addresses except the 4 protocol pool addresses<br>Excess AEQ is instantly redistributed to the 4 redistribution pools<br>No manual intervention required — enforced at the protocol level on every incoming transfer</div>',
    '    <div class="hlbox" data-i18n="cap-box">Bootstrap cap: max(5,min(N,25))× current average AEQ balance<br>1–4 humans: 5× · grows +1× per new human · 25+ humans: 25× permanently<br>Applies to ALL addresses except the 4 protocol pool addresses<br>Excess AEQ instantly redistributed · No manual intervention required</div>'
))

# 2. cap-box EN i18n (cap-title and cap-box are on same line)
replacements.append((
    "  'cap-title':'4. WEALTH CAP — Mathematical Fairness','cap-box':'Cap = 25× current average balance of all verified humans<br>Automatically adjusts as the network grows<br>Excess AEQ instantly redistributed to redistribution pools',",
    "  'cap-title':'4. WEALTH CAP — Mathematical Fairness','cap-box':'Bootstrap cap: max(5,min(N,25))× current average AEQ balance<br>1–4 humans: 5× · +1× per human · 25+: 25× permanently<br>Excess AEQ instantly redistributed · No manual intervention',"
))

# 3. cap-box DE i18n
replacements.append((
    "  'cap-title':'4. VERMÖGENSOBERGRENZE — Mathematische Fairness-Durchsetzung','cap-box':'Obergrenze = 25× aktuelles Durchschnittsguthaben aller verifizierten Menschen<br>Passt sich automatisch an während das Netzwerk wächst und sich Guthaben ändern<br>Gilt für ALLE Adressen außer den 4 Protokoll-Pool-Adressen<br>Überschuss-AEQ wird sofort an die 4 Umverteilungspools weitergeleitet<br>Keine manuelle Eingriffe erforderlich — auf Protokollebene bei jeder eingehenden Überweisung erzwungen',",
    "  'cap-title':'4. VERMÖGENSOBERGRENZE — Mathematische Fairness-Durchsetzung','cap-box':'Bootstrap-Deckel: max(5,min(N,25))× aktuelles Durchschnittsguthaben<br>1–4 Menschen: 5× · +1× pro Mensch · 25+: dauerhaft 25×<br>Gilt für ALLE Adressen außer den 4 Protokoll-Pool-Adressen<br>Überschuss-AEQ sofort weitergeleitet · Keine manuellen Eingriffe',"
))

# 4. cap-box ES i18n
replacements.append((
    "  'cap-title':'4. LÍMITE DE RIQUEZA — Aplicación de Justicia Matemática','cap-box':'Límite = 25× saldo promedio actual de todos los humanos verificados<br>Se ajusta automáticamente mientras la red crece y los saldos cambian<br>Se aplica a TODAS las direcciones excepto las 4 direcciones del pool de protocolo<br>El exceso de AEQ se redistribuye instantáneamente a los 4 pools de redistribución<br>Sin intervención manual — aplicado a nivel de protocolo en cada transferencia entrante',",
    "  'cap-title':'4. LÍMITE DE RIQUEZA — Aplicación de Justicia Matemática','cap-box':'Límite bootstrap: max(5,min(N,25))× saldo promedio actual<br>1–4 humanos: 5× · +1× por humano · 25+: 25× permanente<br>Se aplica a TODAS las direcciones excepto las 4 pools del protocolo<br>Exceso AEQ redistribuido instantáneamente · Sin intervención manual',"
))

# 5. cap-box RU i18n
replacements.append((
    "  'cap-title':'4. ЛИМИТ БОГАТСТВА — Математическое Обеспечение Справедливости','cap-box':'Лимит = 25× текущий средний баланс всех верифицированных людей<br>Автоматически корректируется · Применяется ко всем адресам кроме 4 протокольных пулов<br>Избыточный AEQ мгновенно перераспределяется в 4 пула · Без ручного вмешательства',",
    "  'cap-title':'4. ЛИМИТ БОГАТСТВА — Математическое Обеспечение Справедливости','cap-box':'Bootstrap-лимит: max(5,min(N,25))× текущий средний баланс<br>1–4 людей: 5× · +1× за человека · 25+: 25× навсегда<br>Применяется ко всем адресам кроме 4 протокольных пулов<br>Избыток AEQ мгновенно перераспределяется · Без ручного вмешательства',"
))

# 6. cap-box ID i18n
replacements.append((
    "  'cap-title':'4. BATAS KEKAYAAN — Penerapan Keadilan Matematis','cap-box':'Batas = 25× saldo AEQ rata-rata semua manusia terverifikasi saat ini<br>Otomatis menyesuaikan seiring pertumbuhan jaringan dan perubahan saldo<br>Berlaku untuk SEMUA alamat kecuali 4 alamat pool protokol<br>Kelebihan AEQ langsung didistribusikan ulang ke 4 pool redistribusi<br>Tanpa intervensi manual — diterapkan di tingkat protokol pada setiap transfer masuk',",
    "  'cap-title':'4. BATAS KEKAYAAN — Penerapan Keadilan Matematis','cap-box':'Batas bootstrap: max(5,min(N,25))× saldo rata-rata saat ini<br>1–4 manusia: 5× · +1× per manusia · 25+: 25× permanen<br>Berlaku untuk SEMUA alamat kecuali 4 pool protokol<br>Kelebihan AEQ langsung didistribusikan ulang · Tanpa intervensi manual',"
))

# 7. ubi-src-cap-d DE i18n (full key name: Vermögensobergrenze-Überschuss)
replacements.append((
    "  'ubi-src-cap':'Vermögensobergrenze-Überschuss','ubi-src-cap-d':'Wallets die 25× den Durchschnittssaldo überschreiten werden sofort gekappt. 20% fließt direkt an UBI.',",
    "  'ubi-src-cap':'Vermögensobergrenze-Überschuss','ubi-src-cap-d':'Wallets die den Vermögensdeckel (max(5,min(N,25))× Durchschnitt) überschreiten werden sofort gekappt. 20% fließt direkt an UBI.',"
))

# 8. ubi-src-cap-d ES i18n
replacements.append((
    "  'ubi-src-cap':'Desbordamiento del Límite','ubi-src-cap-d':'Wallets que superan 25× el saldo promedio son confiscadas al instante. El 20% fluye al UBI.',",
    "  'ubi-src-cap':'Desbordamiento del Límite','ubi-src-cap-d':'Wallets que superan el límite de riqueza (max(5,min(N,25))× promedio) son confiscadas al instante. El 20% fluye al UBI.',"
))

# 9. ubi-src-cap-d RU i18n
replacements.append((
    "  'ubi-src-cap':'Превышение Лимита Богатства','ubi-src-cap-d':'Кошельки превышающие 25× средний баланс конфискуются мгновенно. 20% поступает в UBI немедленно.',",
    "  'ubi-src-cap':'Превышение Лимита Богатства','ubi-src-cap-d':'Кошельки превышающие лимит (max(5,min(N,25))× средний) конфискуются мгновенно. 20% поступает в UBI немедленно.',"
))

# 10. ubi-src-cap-d ID i18n
replacements.append((
    "Dompet melebihi 25× saldo rata-rata langsung disita kelebihannya. 20% mengalir ke UBI segera.',",
    "Dompet yang melebihi batas kekayaan (max(5,min(N,25))× rata-rata) langsung disita kelebihannya. 20% mengalir ke UBI segera.',"
))

# 11. MetaMask alert() -> addLog
replacements.append((
    "  if (!window.ethereum) { alert('MetaMask not found. Please install MetaMask.'); return; }",
    "  if (!window.ethereum) { addLog('\U0001f98a MetaMask not found — <a href=\"https://metamask.io/download/\" target=\"_blank\" style=\"color:var(--gold)\">install MetaMask</a> to use this feature.', 'warn'); return; }"
))

# 12. Mobile CSS: add tab padding at 480px + grade-box 2-col at 600px
replacements.append((
    '@media(max-width:480px){.stats-grid{grid-template-columns:repeat(2,1fr)}.stat-val{font-size:1.4rem}header{height:52px}.logo-text{font-size:0.85rem;letter-spacing:2px}.badge-dag{display:none}.main-grid{padding:0 12px 12px}.hero{padding:14px 12px 0}}',
    '@media(max-width:480px){.stats-grid{grid-template-columns:repeat(2,1fr)}.stat-val{font-size:1.4rem}header{height:52px}.logo-text{font-size:0.85rem;letter-spacing:2px}.badge-dag{display:none}.main-grid{padding:0 12px 12px}.hero{padding:14px 12px 0}.tab{padding:12px 10px;font-size:0.6rem}}@media(max-width:600px){.idx-grade-grid{grid-template-columns:repeat(2,1fr)!important}}'
))

# 13. Add idx-grade-grid class to the 4-column grade boxes container
replacements.append((
    '    <div style="margin-top:10px;display:grid;grid-template-columns:repeat(4,1fr);gap:8px">',
    '    <div class="idx-grade-grid" style="margin-top:10px;display:grid;grid-template-columns:repeat(4,1fr);gap:8px">'
))

# 14. Story text HTML default — unique: contains "(Bootstrap)" on line 763
replacements.append((
    'Currently in Phase 0 (Bootstrap). The goal: demonstrate that money can be distributed fairly, equality maintained through mathematical governance, and financial inclusion achieved at global scale — without any central authority.</p>',
    'Currently in Phase 0 (Bootstrap). The goal: demonstrate that money can be distributed fairly, Gini coefficient held below 0.35 (comparable to the most equal developed nations), and financial inclusion achieved at global scale — without any central authority.</p>'
))

# 15. Story text EN i18n — unique: contains "Phase 0." without "(Bootstrap)" on line 1219
replacements.append((
    'Currently in Phase 0. The goal: demonstrate that money can be distributed fairly, equality maintained through mathematical governance, and financial inclusion achieved at global scale — without any central authority.</p>',
    'Currently in Phase 0. The goal: demonstrate that money can be distributed fairly, Gini coefficient held below 0.35 (comparable to the most equal developed nations), and financial inclusion achieved at global scale — without any central authority.</p>'
))

# 16. Insert live wealth-cap widget between grade boxes close and Gini-why box
# Line 657 closes </div> of grade boxes, line 658 opens the Gini-why box
replacements.append((
    '    </div>\n    <div style="margin-top:10px;background:rgba(245,166,35,0.04);border:1px solid rgba(245,166,35,0.15);border-radius:var(--radius-sm);padding:16px">',
    '    </div>\n    <div id="wealth-cap-info" style="margin-top:10px;background:var(--card2);border:1px solid rgba(0,255,209,0.2);border-radius:var(--radius-sm);padding:12px 16px;font-size:0.63rem;color:var(--muted);line-height:1.8">\n      <span style="color:var(--neon);font-weight:700" data-i18n="wcap-lbl">Current Wealth Cap:</span>\n      <span id="live-cap-aeq" style="color:var(--gold);font-weight:700;margin:0 6px">—</span>AEQ\n      <span style="margin:0 8px;opacity:0.4">·</span>\n      <span data-i18n="wcap-mult">Multiplier:</span>\n      <span id="live-cap-mult" style="color:var(--teal);font-weight:700;margin-left:4px">—</span>\n      <span style="margin:0 8px;opacity:0.4">·</span>\n      <span data-i18n="wcap-avg">Avg balance:</span>\n      <span id="live-cap-avg" style="color:var(--purple);font-weight:700;margin-left:4px">—</span> AEQ\n    </div>\n    <div style="margin-top:10px;background:rgba(245,166,35,0.04);border:1px solid rgba(245,166,35,0.15);border-radius:var(--radius-sm);padding:16px">'
))

# 17. Add EN i18n labels for wealth-cap widget
replacements.append((
    "  'curr-idx':'Current Index','bar-0':'0 — Perfect Equality','bar-100':'100 — Max Inequality',",
    "  'curr-idx':'Current Index','bar-0':'0 — Perfect Equality','bar-100':'100 — Max Inequality','wcap-lbl':'Current Wealth Cap:','wcap-mult':'Multiplier:','wcap-avg':'Avg balance:',"
))

# 18. Remove dead ID i18n block with wrong 50x/3x phase values
# (overridden later by the correct block with max(5,min(N,25))x formula)
OLD_DEAD_ID = (
    "  'phases-title':'Fase Protokol',\n"
    "  'phases-desc':'Transisi fase dipicu secara otomatis oleh jumlah manusia — tanpa pemungutan suara, tata kelola, atau kunci admin.',\n"
    "  'p0':'Bootstrap · &lt;100 manusia · Batas kekayaan: 50× saldo rata-rata · Aktif saat ini',\n"
    "  'p1':'Pertumbuhan · 100–10.000 manusia · Batas kekayaan: 20× saldo rata-rata',\n"
    "  'p2':'Stabilitas · 10.000–1J manusia · Batas kekayaan: 10× saldo rata-rata',\n"
    "  'p3':'Kematangan · 1J+ manusia · Batas kekayaan: 3× saldo rata-rata · Redistribusi maksimum',\n"
    "  'wealth-cap-explain':'Batas Kekayaan ditetapkan sebagai kelipatan saldo rata-rata semua manusia terverifikasi saat ini — bukan angka tetap. Secara otomatis menyesuaikan seiring pertumbuhan jaringan.',"
)
replacements.append((OLD_DEAD_ID, "  'phases-title':'Fase Protokol',"))

# 19. Remove dead IT i18n block with wrong 50x/3x phase values
# (overridden later by the correct block with max(5,min(N,25))x formula)
OLD_DEAD_IT = (
    "  'phases-title':'Fasi del Protocollo',\n"
    "  'phases-desc':'Le transizioni di fase vengono attivate automaticamente dal numero di umani — nessun voto, nessuna governance, nessuna chiave admin necessaria.',\n"
    "  'p0':'Bootstrap · &lt;100 umani · Limite ricchezza: 50× saldo medio · Attualmente attivo',\n"
    "  'p1':'Crescita · 100–10.000 umani · Limite ricchezza: 20× saldo medio',\n"
    "  'p2':'Stabilità · 10.000–1M umani · Limite ricchezza: 10× saldo medio',\n"
    "  'p3':'Maturità · 1M+ umani · Limite ricchezza: 3× saldo medio · Massima redistribuzione',\n"
    "  'wealth-cap-explain':'Il Limite di Ricchezza è impostato come multiplo del saldo medio attuale di tutti gli umani verificati — non un numero fisso. Si adatta automaticamente man mano che la rete cresce, mantenendo sempre l\\'equità relativa.',"
)
replacements.append((OLD_DEAD_IT, "  'phases-title':'Fasi del Protocollo',"))

# 20. JS: add live wealth-cap fetch into loadStatus (after index block, before closing })
OLD_JS = (
    "    if (d.index !== undefined) {\n"
    "      document.getElementById('idx-bar').style.width = Math.min(d.index, 100) + '%';\n"
    "      const phases = ['Phase 0: Bootstrap — sliding wealth cap 5×→25× (active)', 'Phase 1: Growth — expanding human registry (cap: 25×)', 'Phase 2: Stability — redistribution active (cap: 25×)', 'Phase 3: Maturity — full decentralization (cap: 25×)'];\n"
    "      document.getElementById('idx-phase-desc').textContent = phases[d.phase || 0] || 'Phase ' + (d.phase || 0);\n"
    "    }\n"
    "  } catch (e) {}\n"
    "}"
)
NEW_JS = (
    "    if (d.index !== undefined) {\n"
    "      document.getElementById('idx-bar').style.width = Math.min(d.index, 100) + '%';\n"
    "      const phases = ['Phase 0: Bootstrap — sliding wealth cap 5×→25× (active)', 'Phase 1: Growth — expanding human registry (cap: 25×)', 'Phase 2: Stability — redistribution active (cap: 25×)', 'Phase 3: Maturity — full decentralization (cap: 25×)'];\n"
    "      document.getElementById('idx-phase-desc').textContent = phases[d.phase || 0] || 'Phase ' + (d.phase || 0);\n"
    "    }\n"
    "  } catch (e) {}\n"
    "  // Populate live wealth-cap widget (non-blocking)\n"
    "  try {\n"
    "    const wc = await (await fetch('/api/wealth-cap')).json();\n"
    "    const capEl = document.getElementById('live-cap-aeq');\n"
    "    const multEl = document.getElementById('live-cap-mult');\n"
    "    const avgEl = document.getElementById('live-cap-avg');\n"
    "    if (capEl && wc.cap_aeq !== undefined) capEl.textContent = wc.cap_aeq.toFixed(2);\n"
    "    if (multEl && wc.multiplier !== undefined) multEl.textContent = wc.multiplier.toFixed(0) + '×';\n"
    "    if (avgEl && wc.average_aeq !== undefined) avgEl.textContent = wc.average_aeq.toFixed(2);\n"
    "  } catch(_) {}\n"
    "}"
)
replacements.append((OLD_JS, NEW_JS))

# ══════════════════════════════════════════════════════════════════
# Apply all replacements
# ══════════════════════════════════════════════════════════════════
errors = []
for i, (old, new) in enumerate(replacements):
    cnt = src.count(old)
    if cnt == 0:
        errors.append(f'[MISS #{i+1}] {old[:100]!r}')
    elif cnt > 1:
        errors.append(f'[DUP #{i+1}] found {cnt}x: {old[:100]!r}')
    else:
        src = src.replace(old, new)

if errors:
    print('ERRORS:')
    for e in errors:
        print(' ', e)
    sys.exit(1)

with open(FILE, 'w', encoding='utf-8') as f:
    f.write(src)

print(f'Done: {len(replacements)} replacements applied successfully.')
