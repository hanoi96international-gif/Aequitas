#!/usr/bin/env python3
"""Patch api_html.go: fix Protocol Phases, Swap UI, and Index target text."""
import sys

FILE = r'C:\Users\aequitas-chain\x\humanity\keeper\api_html.go'

with open(FILE, 'r', encoding='utf-8') as f:
    src = f.read()

orig = src
replacements = []

# ── 1. SWAP UI: Replace direction toggle + input with token selector + % buttons ──

OLD_SWAP_UI = """    <div class="swap-dir" id="swap-dir-box" style="display:flex;gap:8px;margin-bottom:12px">
      <button class="rbtn" id="swap-dir-a2t" onclick="setSwapDirection('aeq_to_tusd')" data-i18n="swap-aeq-to-tusd" style="flex:1">AEQ → tUSD</button>
      <button class="rbtn" id="swap-dir-t2a" onclick="setSwapDirection('tusd_to_aeq')" data-i18n="swap-tusd-to-aeq" style="flex:1">tUSD → AEQ</button>
    </div>
    <input type="number" id="swap-amount" placeholder="Amount" oninput="updateFeeEstimate()" style="width:100%;padding:14px;border-radius:8px;border:1px solid var(--border);background:#0A1220;color:#E8EDF5;font-size:16px;margin-bottom:8px;box-sizing:border-box">"""

NEW_SWAP_UI = """    <div style="margin-bottom:8px">
      <div style="font-size:0.6rem;color:var(--muted);text-transform:uppercase;letter-spacing:1px;margin-bottom:4px" data-i18n="swap-sell-label">Sell</div>
      <div style="display:flex;gap:6px">
        <button class="rbtn" id="swap-dir-a2t" onclick="setSwapDirection('aeq_to_tusd')" style="flex:1">AEQ</button>
        <button class="rbtn" id="swap-dir-t2a" onclick="setSwapDirection('tusd_to_aeq')" style="flex:1">tUSD</button>
      </div>
    </div>
    <input type="number" id="swap-amount" placeholder="Amount" oninput="updateFeeEstimate()" style="width:100%;padding:14px;border-radius:8px;border:1px solid var(--border);background:#0A1220;color:#E8EDF5;font-size:16px;margin-bottom:4px;box-sizing:border-box">
    <div class="pct-row" style="display:flex;gap:6px;margin-bottom:8px">
      <button class="rbtn pctbtn" onclick="setSwapPct(0.25)" style="flex:1;padding:8px;font-size:12px">25%</button>
      <button class="rbtn pctbtn" onclick="setSwapPct(0.5)" style="flex:1;padding:8px;font-size:12px">50%</button>
      <button class="rbtn pctbtn" onclick="setSwapPct(0.75)" style="flex:1;padding:8px;font-size:12px">75%</button>
      <button class="rbtn pctbtn" onclick="setSwapPct(1)" style="flex:1;padding:8px;font-size:12px">MAX</button>
    </div>"""

replacements.append((OLD_SWAP_UI, NEW_SWAP_UI))

# ── 2. Add setSwapPct function after setPctAmount ──

OLD_SWAP_PCT_ANCHOR = """// Signs a fixed, human-readable message describing exactly what's being
// authorized — the wallet owner sees this in MetaMask's signing prompt"""

NEW_SWAP_PCT_ANCHOR = """function setSwapPct(pct) {
  const bal = swapDirection === 'aeq_to_tusd' ? myAEQBalance : myTUSDBalance;
  const amt = bal * pct;
  document.getElementById('swap-amount').value = amt > 0 ? amt.toFixed(6) : '';
  updateFeeEstimate();
}

// Signs a fixed, human-readable message describing exactly what's being
// authorized — the wallet owner sees this in MetaMask's signing prompt"""

replacements.append((OLD_SWAP_PCT_ANCHOR, NEW_SWAP_PCT_ANCHOR))

# ── 3. HTML default idx-desc: update "stay below 20" ──

OLD_IDX_DESC_HTML = "Aequitas is mathematically engineered to stay below 20 — enforced automatically, no governance vote, no admin key required.</div>"
NEW_IDX_DESC_HTML = "Aequitas targets Gini below 0.20 (Index below 20) at scale — enforced automatically by the wealth cap and redistribution pools, no governance vote required.</div>"
replacements.append((OLD_IDX_DESC_HTML, NEW_IDX_DESC_HTML))

# ── 4. HTML default gini-what-text: update "below 0.20" ──

OLD_GINI_HTML = "· Aequitas target: below 0.20.</div>"
NEW_GINI_HTML = "· Aequitas long-term target: Gini below 0.20 — enforced by the wealth cap at scale (bootstrap: sliding cap 5×→25× per human).</div>"
replacements.append((OLD_GINI_HTML, NEW_GINI_HTML))

# ── 5. HTML default Protocol Phases ──

OLD_PHASES_DESC_HTML = """    <div class="idx-desc" data-i18n="phases-desc">Phase boundaries define network growth milestones. The wealth cap multiplier is currently fixed at 25× (matching the live Go code constant <em>wealthCapMultiplier = 25.0</em>) — phase-based automatic tightening is a planned future protocol upgrade.</div>"""
NEW_PHASES_DESC_HTML = """    <div class="idx-desc" data-i18n="phases-desc">The wealth cap uses a bootstrap multiplier during Phase 0: max(5, min(N, 25))× average balance. With 1–4 humans: 5× average. Each new human adds 1×. At 25+ humans: locks permanently at 25×. Phase 1+ maintains 25× fixed. All transitions trigger automatically by human count — no governance vote, no admin key required.</div>"""
replacements.append((OLD_PHASES_DESC_HTML, NEW_PHASES_DESC_HTML))

OLD_P0_HTML = '      <tr><td><strong style="color:var(--neon)">Phase 0</strong></td><td style="color:var(--neon)" data-i18n="p0">Bootstrap · &lt;100 humans · Wealth Cap: 25× average balance · Currently active</td></tr>'
NEW_P0_HTML = '      <tr><td><strong style="color:var(--neon)">Phase 0</strong></td><td style="color:var(--neon)" data-i18n="p0">Bootstrap · &lt;100 humans · Wealth Cap: max(5,min(N,25))× average · Slides 5×→25× until 25th human · Currently active</td></tr>'
replacements.append((OLD_P0_HTML, NEW_P0_HTML))

OLD_P1_HTML = '      <tr><td><strong style="color:var(--blue)">Phase 1</strong></td><td style="color:var(--blue)" data-i18n="p1">Growth · 100–10,000 humans · Wealth Cap: 25× (planned tightening: 20×)</td></tr>'
NEW_P1_HTML = '      <tr><td><strong style="color:var(--blue)">Phase 1</strong></td><td style="color:var(--blue)" data-i18n="p1">Growth · 100–10,000 humans · Wealth Cap: 25× average balance</td></tr>'
replacements.append((OLD_P1_HTML, NEW_P1_HTML))

OLD_P2_HTML = '      <tr><td><strong style="color:var(--gold)">Phase 2</strong></td><td style="color:var(--gold)" data-i18n="p2">Stability · 10,000–1M humans · Wealth Cap: 25× (planned tightening: 10×)</td></tr>'
NEW_P2_HTML = '      <tr><td><strong style="color:var(--gold)">Phase 2</strong></td><td style="color:var(--gold)" data-i18n="p2">Stability · 10,000–1M humans · Wealth Cap: 25× average balance</td></tr>'
replacements.append((OLD_P2_HTML, NEW_P2_HTML))

OLD_P3_HTML = '      <tr><td><strong style="color:var(--purple)">Phase 3</strong></td><td style="color:var(--purple)" data-i18n="p3">Maturity · 1M+ humans · Wealth Cap: 25× (planned tightening: 5×)</td></tr>'
NEW_P3_HTML = '      <tr><td><strong style="color:var(--purple)">Phase 3</strong></td><td style="color:var(--purple)" data-i18n="p3">Maturity · 1M+ humans · Wealth Cap: 25× average balance</td></tr>'
replacements.append((OLD_P3_HTML, NEW_P3_HTML))

OLD_WCE_HTML = '    <div class="hlbox" data-i18n="wealth-cap-explain">The <strong>Wealth Cap</strong> is currently set to <strong>25× the average AEQ balance</strong> across all verified humans. This is a fixed constant in the live Go protocol code. The cap is always relative to the live average balance — so it automatically scales as the network grows without needing manual adjustment.</div>'
NEW_WCE_HTML = '    <div class="hlbox" data-i18n="wealth-cap-explain">The <strong>Wealth Cap</strong> during Phase 0 (Bootstrap) uses the formula <strong>max(5, min(N, 25))× average AEQ balance</strong>, where N = registered humans. With 1–4 humans: cap = 5× average. Each new human adds 1×. At 25+ humans: the multiplier locks permanently at 25×. The cap always scales with the live average balance — automatically adjusting as the network grows.</div>'
replacements.append((OLD_WCE_HTML, NEW_WCE_HTML))

# ── 6. EN i18n ──

OLD_EN_IDX_DESC = "'idx-desc':'The Aequitas Index is derived from the <strong style=\"color:var(--teal)\">Gini coefficient</strong> — the international standard for measuring wealth inequality, adopted by the World Bank, OECD, and UN. It captures the complete balance distribution across every verified human simultaneously. <strong style=\"color:var(--neon)\">0 = perfect equality</strong> (every wallet holds the same AEQ). <strong style=\"color:var(--red)\">100 = total concentration</strong> (one wallet holds all AEQ). Bitcoin Gini ≈ 0.85 (Index 85) · South Africa (world record) ≈ 0.63 · Scandinavia ≈ 0.27 · Aequitas target: below 0.20 — enforced automatically, no governance required.',"
NEW_EN_IDX_DESC = "'idx-desc':'The Aequitas Index is derived from the <strong style=\"color:var(--teal)\">Gini coefficient</strong> — the international standard for measuring wealth inequality, adopted by the World Bank, OECD, and UN. It captures the complete balance distribution across every verified human simultaneously. <strong style=\"color:var(--neon)\">0 = perfect equality</strong> (every wallet holds the same AEQ). <strong style=\"color:var(--red)\">100 = total concentration</strong> (one wallet holds all AEQ). Bitcoin Gini ≈ 0.85 (Index 85) · South Africa (world record) ≈ 0.63 · Scandinavia ≈ 0.27 · Aequitas long-term target: Gini below 0.20 (Index below 20) — enforced by the wealth cap and redistribution pools.',"
replacements.append((OLD_EN_IDX_DESC, NEW_EN_IDX_DESC))

OLD_EN_GINI_WHAT = "· Aequitas target: below 0.20.',"
NEW_EN_GINI_WHAT = "· Aequitas long-term target: Gini below 0.20 — enforced by the wealth cap at scale (bootstrap: sliding cap 5×→25× per human).',"
replacements.append((OLD_EN_GINI_WHAT, NEW_EN_GINI_WHAT))

OLD_EN_PHASES_DESC = "  'phases-desc':'Phase boundaries define network growth milestones. The wealth cap multiplier is currently fixed at 25× (matching the live Go code constant wealthCapMultiplier = 25.0) — phase-based automatic tightening is a planned future protocol upgrade.',"
NEW_EN_PHASES_DESC = "  'phases-desc':'The wealth cap uses a bootstrap multiplier during Phase 0: max(5, min(N, 25))× average balance. With 1–4 humans: 5× average. Each new human adds 1×. At 25+ humans: locks permanently at 25×. Phase 1+ maintains 25× fixed. All transitions trigger automatically by human count — no governance, no admin key.',"
replacements.append((OLD_EN_PHASES_DESC, NEW_EN_PHASES_DESC))

OLD_EN_P0 = "  'p0':'Bootstrap · &lt;100 humans · Wealth Cap: 25× average balance · Currently active',"
NEW_EN_P0 = "  'p0':'Bootstrap · &lt;100 humans · Wealth Cap: max(5,min(N,25))× average · Slides 5×→25× until 25th human · Currently active',"
replacements.append((OLD_EN_P0, NEW_EN_P0))

OLD_EN_P1 = "  'p1':'Growth · 100–10,000 humans · Wealth Cap: 25× (planned tightening: 20×)',"
NEW_EN_P1 = "  'p1':'Growth · 100–10,000 humans · Wealth Cap: 25× average balance',"
replacements.append((OLD_EN_P1, NEW_EN_P1))

OLD_EN_P2 = "  'p2':'Stability · 10,000–1M humans · Wealth Cap: 25× (planned tightening: 10×)',"
NEW_EN_P2 = "  'p2':'Stability · 10,000–1M humans · Wealth Cap: 25× average balance',"
replacements.append((OLD_EN_P2, NEW_EN_P2))

OLD_EN_P3 = "  'p3':'Maturity · 1M+ humans · Wealth Cap: 25× (planned tightening: 5×)',"
NEW_EN_P3 = "  'p3':'Maturity · 1M+ humans · Wealth Cap: 25× average balance',"
replacements.append((OLD_EN_P3, NEW_EN_P3))

OLD_EN_WCE = "  'wealth-cap-explain':'The Wealth Cap is currently set to 25× the average AEQ balance across all verified humans. This is a fixed constant in the live Go protocol code. The cap is always relative to the live average balance — so it automatically scales as the network grows.',"
NEW_EN_WCE = "  'wealth-cap-explain':'The Wealth Cap in Phase 0 (Bootstrap) uses max(5, min(N, 25))× average AEQ balance, where N = registered humans. 1–4 humans: cap = 5× average. Each new human adds 1×. 25+ humans: locked permanently at 25×. The cap always scales with the live average balance.',"
replacements.append((OLD_EN_WCE, NEW_EN_WCE))

# ── 7. DE i18n ──

OLD_DE_IDX_DESC = "Aequitas-Ziel: unter 0,20 — automatisch durchgesetzt, keine Governance nötig.'"
NEW_DE_IDX_DESC = "Aequitas-Langzeitziel: Gini unter 0,20 (Index unter 20) — automatisch durchgesetzt durch den Vermögensobergrenze-Mechanismus.'"
replacements.append((OLD_DE_IDX_DESC, NEW_DE_IDX_DESC))

OLD_DE_GINI_WHAT = "Aequitas-Ziel: unter 0,20.',"
NEW_DE_GINI_WHAT = "Aequitas-Langzeitziel: Gini unter 0,20 — durchgesetzt durch den Vermögensdeckel bei Skalierung (Bootstrap: gleitender Deckel 5×→25× pro Mensch).',"
replacements.append((OLD_DE_GINI_WHAT, NEW_DE_GINI_WHAT))

OLD_DE_PHASES_DESC = "  'phases-desc':'Phasengrenzen definieren Netzwerk-Wachstums-Meilensteine. Der Multiplikator ist derzeit fest auf 25× eingestellt (entspricht der Go-Code-Konstante wealthCapMultiplier = 25.0) — phasenbasierte Anpassung ist für ein zukünftiges Protokoll-Upgrade geplant.',"
NEW_DE_PHASES_DESC = "  'phases-desc':'In Phase 0 verwendet die Vermögensobergrenze einen Bootstrap-Multiplikator: max(5, min(N, 25))× Durchschnittsguthaben. Mit 1–4 Menschen: 5× Durchschnitt. Jeder neue Mensch erhöht um 1×. Ab 25+ Menschen: dauerhaft auf 25× fixiert. Phase 1+ behält 25× fest. Alle Übergänge erfolgen automatisch — kein Governance-Vote, kein Admin-Key.',"
replacements.append((OLD_DE_PHASES_DESC, NEW_DE_PHASES_DESC))

OLD_DE_P0 = "  'p0':'Bootstrap · &lt;100 Menschen · Vermögensobergrenze: 25× Durchschnittsguthaben · Derzeit aktiv',"
NEW_DE_P0 = "  'p0':'Bootstrap · &lt;100 Menschen · Vermögensobergrenze: max(5,min(N,25))× Durchschnitt · Gleitet 5×→25× bis zum 25. Menschen · Derzeit aktiv',"
replacements.append((OLD_DE_P0, NEW_DE_P0))

OLD_DE_P1 = "  'p1':'Wachstum · 100–10.000 Menschen · Vermögensobergrenze: 25× (geplante Absenkung: 20×)',"
NEW_DE_P1 = "  'p1':'Wachstum · 100–10.000 Menschen · Vermögensobergrenze: 25× Durchschnittsguthaben',"
replacements.append((OLD_DE_P1, NEW_DE_P1))

OLD_DE_P2 = "  'p2':'Stabilität · 10.000–1M Menschen · Vermögensobergrenze: 25× (geplante Absenkung: 10×)',"
NEW_DE_P2 = "  'p2':'Stabilität · 10.000–1M Menschen · Vermögensobergrenze: 25× Durchschnittsguthaben',"
replacements.append((OLD_DE_P2, NEW_DE_P2))

OLD_DE_P3 = "  'p3':'Reife · 1M+ Menschen · Vermögensobergrenze: 25× (geplante Absenkung: 5×)',"
NEW_DE_P3 = "  'p3':'Reife · 1M+ Menschen · Vermögensobergrenze: 25× Durchschnittsguthaben',"
replacements.append((OLD_DE_P3, NEW_DE_P3))

OLD_DE_WCE = "  'wealth-cap-explain':'Die Vermögensobergrenze ist derzeit auf 25× des Durchschnittsguthabens aller verifizierten Menschen festgelegt. Dies ist eine feste Konstante im Live-Go-Code. Da der Wert immer relativ zum Live-Durchschnitt gilt, skaliert die Obergrenze automatisch mit dem Netzwerkwachstum.',"
NEW_DE_WCE = "  'wealth-cap-explain':'Die Vermögensobergrenze in Phase 0 (Bootstrap) verwendet max(5, min(N, 25))× Durchschnittsguthaben, wobei N = registrierte Menschen. 1–4 Menschen: 5× Durchschnitt. Jeder neue Mensch erhöht um 1×. Ab 25+ Menschen: dauerhaft 25×. Die Obergrenze skaliert stets mit dem Live-Durchschnittsguthaben.',"
replacements.append((OLD_DE_WCE, NEW_DE_WCE))

# ── 8. ES i18n ──

OLD_ES_PHASES_DESC = "  'phases-desc':'Los hitos de fase definen etapas de crecimiento. El multiplicador del límite de riqueza está actualmente fijo en 25× (constante de código Go: wealthCapMultiplier = 25.0) — el ajuste automático por fases es una mejora futura planificada.',"
NEW_ES_PHASES_DESC = "  'phases-desc':'En Fase 0, el límite de riqueza usa un multiplicador de arranque: max(5, min(N, 25))× saldo promedio. Con 1–4 humanos: 5× promedio. Cada nuevo humano añade 1×. A 25+ humanos: fijado permanentemente en 25×. Fase 1+ mantiene 25× fijo. Todas las transiciones son automáticas — sin voto de gobernanza, sin clave de administrador.',"
replacements.append((OLD_ES_PHASES_DESC, NEW_ES_PHASES_DESC))

OLD_ES_P0 = "  'p0':'Bootstrap · &lt;100 humanos · Límite de riqueza: 25× saldo promedio · Actualmente activo',"
NEW_ES_P0 = "  'p0':'Bootstrap · &lt;100 humanos · Límite de Riqueza: max(5,min(N,25))× promedio · Deslizamiento 5×→25× hasta el 25.º humano · Actualmente activo',"
replacements.append((OLD_ES_P0, NEW_ES_P0))

OLD_ES_P1 = "  'p1':'Crecimiento · 100–10,000 humanos · Límite de riqueza: 25× (reducción planificada: 20×)',"
NEW_ES_P1 = "  'p1':'Crecimiento · 100–10,000 humanos · Límite de Riqueza: 25× saldo promedio',"
replacements.append((OLD_ES_P1, NEW_ES_P1))

OLD_ES_P2 = "  'p2':'Estabilidad · 10,000–1M humanos · Límite de riqueza: 25× (reducción planificada: 10×)',"
NEW_ES_P2 = "  'p2':'Estabilidad · 10,000–1M humanos · Límite de Riqueza: 25× saldo promedio',"
replacements.append((OLD_ES_P2, NEW_ES_P2))

OLD_ES_P3 = "  'p3':'Madurez · 1M+ humanos · Límite de riqueza: 25× (reducción planificada: 5×)',"
NEW_ES_P3 = "  'p3':'Madurez · 1M+ humanos · Límite de Riqueza: 25× saldo promedio',"
replacements.append((OLD_ES_P3, NEW_ES_P3))

OLD_ES_WCE = "  'wealth-cap-explain':'El Límite de Riqueza está actualmente fijado en 25× el saldo promedio de todos los humanos verificados. Es una constante fija en el código Go en vivo. Al ser relativo al promedio actual, se escala automáticamente con el crecimiento de la red.',"
NEW_ES_WCE = "  'wealth-cap-explain':'El Límite de Riqueza en Fase 0 (Bootstrap) usa max(5, min(N, 25))× saldo promedio, donde N = humanos registrados. 1–4 humanos: 5× promedio. Cada nuevo humano añade 1×. 25+ humanos: bloqueado en 25× permanentemente. El límite siempre se escala con el saldo promedio actual.',"
replacements.append((OLD_ES_WCE, NEW_ES_WCE))

# ── 9. RU i18n ──

OLD_RU_PHASES_DESC = "  'phases-desc':'Переходы фаз запускаются автоматически по количеству людей — без голосования, без управления, без административных ключей.',"
NEW_RU_PHASES_DESC = "  'phases-desc':'В Фазе 0 (Bootstrap) применяется скользящий множитель: max(5, min(N, 25))× средний баланс. При 1–4 людях: 5× средний. Каждый новый человек прибавляет 1×. При 25+ людях: фиксируется навсегда на 25×. Фаза 1+ сохраняет 25× фиксированным. Переходы автоматические — без голосования, без административных ключей.',"
replacements.append((OLD_RU_PHASES_DESC, NEW_RU_PHASES_DESC))

OLD_RU_P0 = "  'p0':'Начальный этап · &lt;100 людей · Лимит богатства: 50× средний баланс · Сейчас активен',"
NEW_RU_P0 = "  'p0':'Bootstrap · &lt;100 людей · Лимит богатства: max(5,min(N,25))× средний · Скользит 5×→25× до 25-го человека · Сейчас активен',"
replacements.append((OLD_RU_P0, NEW_RU_P0))

OLD_RU_P1 = "  'p1':'Рост · 100–10 000 людей · Лимит богатства: 20× средний баланс',"
NEW_RU_P1 = "  'p1':'Рост · 100–10 000 людей · Лимит богатства: 25× средний баланс',"
replacements.append((OLD_RU_P1, NEW_RU_P1))

OLD_RU_P2 = "  'p2':'Стабильность · 10 000–1М людей · Лимит богатства: 10× средний баланс',"
NEW_RU_P2 = "  'p2':'Стабильность · 10 000–1М людей · Лимит богатства: 25× средний баланс',"
replacements.append((OLD_RU_P2, NEW_RU_P2))

OLD_RU_P3 = "  'p3':'Зрелость · 1М+ людей · Лимит богатства: 3× средний баланс · Максимальное перераспределение',"
NEW_RU_P3 = "  'p3':'Зрелость · 1М+ людей · Лимит богатства: 25× средний баланс',"
replacements.append((OLD_RU_P3, NEW_RU_P3))

OLD_RU_WCE = "  'wealth-cap-explain':'Лимит Богатства устанавливается как кратное текущего среднего баланса всех верифицированных людей. Автоматически корректируется по мере роста сети.',"
NEW_RU_WCE = "  'wealth-cap-explain':'В Фазе 0 (Bootstrap) Лимит Богатства = max(5, min(N, 25))× средний баланс AEQ, где N = количество зарегистрированных людей. 1–4 человека: 5× средний. Каждый новый человек прибавляет 1×. 25+ людей: фиксируется навсегда на 25×. Лимит всегда привязан к актуальному среднему балансу.',"
replacements.append((OLD_RU_WCE, NEW_RU_WCE))

# ── 10. ZH i18n — remove "planned tightening" from p1-p3 ──

OLD_ZH_P1 = "  'p1':'增长期 · 100–10,000人 · 财富上限：25×（计划收紧至：20×）',"
NEW_ZH_P1 = "  'p1':'增长期 · 100–10,000人 · 财富上限：25×平均余额',"
replacements.append((OLD_ZH_P1, NEW_ZH_P1))

OLD_ZH_P2 = "  'p2':'稳定期 · 10,000–1M人 · 财富上限：25×（计划收紧至：10×）',"
NEW_ZH_P2 = "  'p2':'稳定期 · 10,000–1M人 · 财富上限：25×平均余额',"
replacements.append((OLD_ZH_P2, NEW_ZH_P2))

OLD_ZH_P3 = "  'p3':'成熟期 · 1M+人 · 财富上限：25×（计划收紧至：5×）',"
NEW_ZH_P3 = "  'p3':'成熟期 · 1M+人 · 财富上限：25×平均余额',"
replacements.append((OLD_ZH_P3, NEW_ZH_P3))

# ── 11. ID i18n effective block (lines ~1905-1910) ──

OLD_ID_PHASES_DESC = "  'phases-desc':'Batas fase mendefinisikan tonggak pertumbuhan jaringan. Pengganda batas kekayaan saat ini ditetapkan pada 25× (konstanta kode Go: wealthCapMultiplier = 25.0) — penyesuaian otomatis berbasis fase direncanakan untuk peningkatan protokol mendatang.',"
NEW_ID_PHASES_DESC = "  'phases-desc':'Pada Fase 0, batas kekayaan menggunakan pengganda bootstrap: max(5, min(N, 25))× saldo rata-rata. Dengan 1–4 manusia: 5× rata-rata. Setiap manusia baru menambah 1×. Pada 25+ manusia: terkunci permanen di 25×. Fase 1+ mempertahankan 25× tetap. Semua transisi otomatis — tanpa pemungutan suara, tanpa kunci admin.',"
replacements.append((OLD_ID_PHASES_DESC, NEW_ID_PHASES_DESC))

OLD_ID_P0 = "  'p0':'Bootstrap · &lt;100 manusia · Batas Kekayaan: 25× saldo rata-rata · Saat ini aktif',"
NEW_ID_P0 = "  'p0':'Bootstrap · &lt;100 manusia · Batas Kekayaan: max(5,min(N,25))× rata-rata · Meluncur 5×→25× hingga manusia ke-25 · Saat ini aktif',"
replacements.append((OLD_ID_P0, NEW_ID_P0))

OLD_ID_P1 = "  'p1':'Pertumbuhan · 100–10.000 manusia · Batas Kekayaan: 25× (penurunan terencana: 20×)',"
NEW_ID_P1 = "  'p1':'Pertumbuhan · 100–10.000 manusia · Batas Kekayaan: 25× saldo rata-rata',"
replacements.append((OLD_ID_P1, NEW_ID_P1))

OLD_ID_P2 = "  'p2':'Stabilitas · 10.000–1M manusia · Batas Kekayaan: 25× (penurunan terencana: 10×)',"
NEW_ID_P2 = "  'p2':'Stabilitas · 10.000–1M manusia · Batas Kekayaan: 25× saldo rata-rata',"
replacements.append((OLD_ID_P2, NEW_ID_P2))

OLD_ID_P3 = "  'p3':'Kematangan · 1M+ manusia · Batas Kekayaan: 25× (penurunan terencana: 5×)',"
NEW_ID_P3 = "  'p3':'Kematangan · 1M+ manusia · Batas Kekayaan: 25× saldo rata-rata',"
replacements.append((OLD_ID_P3, NEW_ID_P3))

OLD_ID_WCE = "  'wealth-cap-explain':'Batas Kekayaan saat ini ditetapkan pada 25× saldo AEQ rata-rata semua manusia terverifikasi. Ini adalah konstanta tetap dalam kode Go langsung. Karena selalu relatif terhadap rata-rata saat ini, batas secara otomatis diskalakan seiring pertumbuhan jaringan.',"
NEW_ID_WCE = "  'wealth-cap-explain':'Batas Kekayaan pada Fase 0 (Bootstrap) menggunakan max(5, min(N, 25))× saldo AEQ rata-rata, di mana N = manusia terdaftar. 1–4 manusia: 5× rata-rata. Setiap manusia baru menambah 1×. 25+ manusia: terkunci permanen di 25×. Batas selalu mengikuti saldo rata-rata saat ini.',"
replacements.append((OLD_ID_WCE, NEW_ID_WCE))

# ── 12. IT i18n effective block (lines ~2044-2049) ──

OLD_IT_PHASES_DESC = "  'phases-desc':'I confini di fase definiscono le tappe di crescita della rete. Durante il bootstrap (&lt;25 umani registrati) il limite usa un moltiplicatore scorrevole: max(5,min(N,25))× media — 5× con 1–4 umani, cresce di 1× per ogni nuovo umano, raggiunge il pieno 25× a 25+ umani. Previene la concentrazione iniziale prima che esista una reale partecipazione.',"
NEW_IT_PHASES_DESC = "  'phases-desc':'In Fase 0 (Bootstrap) il limite di ricchezza usa un moltiplicatore scorrevole: max(5, min(N, 25))× saldo medio. Con 1–4 umani: 5× media. Ogni nuovo umano aggiunge 1×. A 25+ umani: bloccato permanentemente a 25×. Fase 1+ mantiene 25× fisso. Tutte le transizioni sono automatiche — nessun voto, nessuna chiave admin.',"
replacements.append((OLD_IT_PHASES_DESC, NEW_IT_PHASES_DESC))

OLD_IT_P0 = "  'p0':'Bootstrap · &lt;100 umani · Limite di Ricchezza: 25× saldo medio · Attualmente attivo',"
NEW_IT_P0 = "  'p0':'Bootstrap · &lt;100 umani · Limite di Ricchezza: max(5,min(N,25))× media · Scorre 5×→25× fino al 25° umano · Attualmente attivo',"
replacements.append((OLD_IT_P0, NEW_IT_P0))

OLD_IT_P1 = "  'p1':'Crescita · 100–10.000 umani · Limite di Ricchezza: 25× (riduzione pianificata: 20×)',"
NEW_IT_P1 = "  'p1':'Crescita · 100–10.000 umani · Limite di Ricchezza: 25× saldo medio',"
replacements.append((OLD_IT_P1, NEW_IT_P1))

OLD_IT_P2 = "  'p2':'Stabilità · 10.000–1M umani · Limite di Ricchezza: 25× (riduzione pianificata: 10×)',"
NEW_IT_P2 = "  'p2':'Stabilità · 10.000–1M umani · Limite di Ricchezza: 25× saldo medio',"
replacements.append((OLD_IT_P2, NEW_IT_P2))

OLD_IT_P3 = "  'p3':'Maturità · 1M+ umani · Limite di Ricchezza: 25× (riduzione pianificata: 5×)',"
NEW_IT_P3 = "  'p3':'Maturità · 1M+ umani · Limite di Ricchezza: 25× saldo medio',"
replacements.append((OLD_IT_P3, NEW_IT_P3))

OLD_IT_WCE = "  'wealth-cap-explain':'Il Limite di Ricchezza è attualmente impostato a 25× il saldo AEQ medio di tutti gli umani verificati. Questa è una costante fissa nel codice Go in produzione. Poiché è sempre relativo alla media corrente, il limite si adatta automaticamente alla crescita della rete.',"
NEW_IT_WCE = "  'wealth-cap-explain':'Il Limite di Ricchezza in Fase 0 (Bootstrap) usa max(5, min(N, 25))× saldo AEQ medio, dove N = umani registrati. 1–4 umani: 5× media. Ogni nuovo umano aggiunge 1×. 25+ umani: bloccato permanentemente a 25×. Il limite si adatta sempre al saldo medio corrente.',"
replacements.append((OLD_IT_WCE, NEW_IT_WCE))

# ── 13. JS phases array ──

OLD_JS_PHASES = "      const phases = ['Phase 0: Bootstrap — building the network', 'Phase 1: Growth — expanding human registry', 'Phase 2: Stability — redistribution active', 'Phase 3: Maturity — full decentralization'];"
NEW_JS_PHASES = "      const phases = ['Phase 0: Bootstrap — sliding wealth cap 5×→25× (active)', 'Phase 1: Growth — expanding human registry (cap: 25×)', 'Phase 2: Stability — redistribution active (cap: 25×)', 'Phase 3: Maturity — full decentralization (cap: 25×)'];"
replacements.append((OLD_JS_PHASES, NEW_JS_PHASES))

# ── Apply all replacements ──
errors = []
for i, (old, new) in enumerate(replacements):
    if old not in src:
        errors.append(f"[MISS #{i+1}] String not found: {old[:80]!r}...")
    else:
        count = src.count(old)
        if count > 1:
            errors.append(f"[DUP #{i+1}] Found {count} times: {old[:80]!r}...")
        else:
            src = src.replace(old, new)

if errors:
    print("ERRORS:")
    for e in errors:
        print(" ", e)
    sys.exit(1)

with open(FILE, 'w', encoding='utf-8') as f:
    f.write(src)

changed = len(replacements) - len(errors)
print(f"Done: {changed}/{len(replacements)} replacements applied.")
