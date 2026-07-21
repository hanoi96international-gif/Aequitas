#!/usr/bin/env python3
"""
Patch api_html.go:
1. Rename Swap -> Exchange tab, split into Swap + Liquidity sub-tabs
2. Add sub-route URL support (pushState in showStab/showTab, activateTabFromPath)
3. Fix double <div class="is"> in eqi-score panel
4. Update server-side tab name references
"""
import sys, re

FILE = r'C:\Users\aequitas-chain\x\humanity\keeper\api_html.go'
with open(FILE, 'r', encoding='utf-8') as f:
    src = f.read()

orig = src
errors = []

# ─────────────────────────────────────────────────────────────────────────────
# 1. Tab nav: rename Swap -> Exchange
# ─────────────────────────────────────────────────────────────────────────────
OLD_NAV = """  <div class="tab" onclick="showTab('swap',this)">🔄 Swap</div>"""
NEW_NAV = """  <div class="tab" onclick="showTab('exchange',this)">🔄 Exchange</div>"""
if OLD_NAV not in src:
    errors.append("MISS: tab nav swap->exchange")
else:
    src = src.replace(OLD_NAV, NEW_NAV, 1)

# ─────────────────────────────────────────────────────────────────────────────
# 2. Restructure the entire Swap tab -> Exchange tab with sub-tabs
# ─────────────────────────────────────────────────────────────────────────────
# Find boundaries
SWAP_MARKER  = '\n<!-- SWAP -->\n'
INDEX_MARKER = '\n<!-- INDEX (Equality) -->\n'

if SWAP_MARKER not in src:
    errors.append("MISS: <!-- SWAP --> marker"); sys.exit(1) if errors else None
if INDEX_MARKER not in src:
    errors.append("MISS: <!-- INDEX (Equality) --> marker"); sys.exit(1) if errors else None

idx_swap  = src.index(SWAP_MARKER)
idx_index = src.index(INDEX_MARKER)

# Extract the old tab-swap inner content (between <div class="rs"> and its closing </div></div>)
swap_block = src[idx_swap + len(SWAP_MARKER) : idx_index]
# swap_block starts with: <div id="tab-swap" class="tab-content">\n<div class="rs">\n
# and ends with: </div>\n</div>\n

# Extract the .rs inner HTML (everything inside the rs div)
rs_open = '<div class="rs">\n'
rs_start = swap_block.index(rs_open) + len(rs_open)
# Find the matching closing </div></div> at the end
# The block ends with </div>\n</div>\n (closes .rs and tab-swap)
rs_inner = swap_block[rs_start:]
# Strip the two closing divs at the end
rs_inner = rs_inner.rstrip()
if rs_inner.endswith('</div>'):
    rs_inner = rs_inner[:-6].rstrip()  # remove tab-swap closing div
if rs_inner.endswith('</div>'):
    rs_inner = rs_inner[:-6].rstrip()  # remove .rs closing div

# Split rs_inner into SWAP portion and LIQUIDITY portion
# The add-liquidity section starts with: <div class="ic" style="margin-top:20px">
#   followed immediately by: <div class="ic-title" data-i18n="swap-addliq-title">
ADDLIQ_MARKER = '\n    <div class="ic" style="margin-top:20px">\n      <div class="ic-title" data-i18n="swap-addliq-title">'
if ADDLIQ_MARKER not in rs_inner:
    errors.append("MISS: addliq marker in rs_inner")
    # fallback: keep everything in swap panel
    swap_content = rs_inner
    liq_content = ''
else:
    split_idx = rs_inner.index(ADDLIQ_MARKER)
    swap_content = rs_inner[:split_idx].rstrip()
    liq_content  = rs_inner[split_idx:].lstrip()

# The liq_content currently ends with:
#   </div>  (closes ic - lp position)
#   </div>  (closes rcard)
#   <div class="ic">  Pool status
#   <div class="ic">  Pool addresses
# We want pool status + pool addresses to remain in the SWAP panel (moved there)
# Find where the pool status ic starts in liq_content
POOL_STATUS_MARKER = '\n  <div class="ic">\n    <div class="ic-title" data-i18n="swap-pool-title">'
if POOL_STATUS_MARKER in liq_content:
    ps_idx = liq_content.index(POOL_STATUS_MARKER)
    # Pool status and everything after -> move back to swap panel
    pool_section = liq_content[ps_idx:].strip()
    liq_content  = liq_content[:ps_idx].strip()
    swap_content = swap_content + '\n\n' + pool_section

NEW_EXCHANGE_BLOCK = f"""
<!-- EXCHANGE -->
<div id="tab-exchange" class="tab-content">
<nav class="stabs">
  <div class="stab active" onclick="showStab('tab-exchange','exch-swap',this)">🔄 Swap</div>
  <div class="stab" onclick="showStab('tab-exchange','exch-liquidity',this)">💧 Liquidity</div>
</nav>
<div id="exch-swap" class="stab-panel active">
<div class="rs">
{swap_content}
</div>
</div>
<div id="exch-liquidity" class="stab-panel">
<div class="rs">
  <div class="rhero">
    <div class="rhero-title">💧 Liquidity</div>
    <div class="rhero-sub">Provide AEQ / tUSD liquidity to earn 30% of all swap fees, distributed daily.</div>
  </div>
{liq_content}
</div>
</div>
</div>
"""

src = src[:idx_swap] + NEW_EXCHANGE_BLOCK + src[idx_index:]
print("Exchange tab restructured with Swap + Liquidity sub-panels")

# ─────────────────────────────────────────────────────────────────────────────
# 3. Fix double <div class="is"> in eqi-score panel
# ─────────────────────────────────────────────────────────────────────────────
OLD_DOUBLE_IS = '<div id="eqi-score" class="stab-panel active">\n<div class="is">\n<div class="is">'
NEW_SINGLE_IS = '<div id="eqi-score" class="stab-panel active">\n<div class="is">'
if OLD_DOUBLE_IS not in src:
    errors.append("MISS: double .is div in eqi-score")
else:
    src = src.replace(OLD_DOUBLE_IS, NEW_SINGLE_IS, 1)
    # Also fix the extra closing </div> at the end of eqi-score
    # The score panel ends with: </div>\n</div>\n</div>\n (is, is, eqi-score)
    # Now it should end with:    </div>\n</div>            (is, eqi-score)
    OLD_SCORE_CLOSE = '</div>\n</div>\n</div>\n<div id="eqi-economy" class="stab-panel">'
    NEW_SCORE_CLOSE = '</div>\n</div>\n<div id="eqi-economy" class="stab-panel">'
    if OLD_SCORE_CLOSE in src:
        src = src.replace(OLD_SCORE_CLOSE, NEW_SCORE_CLOSE, 1)
    else:
        errors.append("MISS: eqi-score triple close divs")
    print("Fixed double .is wrapper in eqi-score")

# ─────────────────────────────────────────────────────────────────────────────
# 4. Update showStab() — add URL push with tab/stab slug mapping
# ─────────────────────────────────────────────────────────────────────────────
OLD_SHOW_STAB = """function showStab(parentId, stabId, el) {
  const parent = document.getElementById(parentId);
  parent.querySelectorAll('.stab-panel').forEach(p => p.classList.remove('active'));
  parent.querySelectorAll('.stab').forEach(s => s.classList.remove('active'));
  document.getElementById(stabId).classList.add('active');
  el.classList.add('active');
  if (stabId === 'eqi-charts') { drawGiniHistoryChart(); drawLorenzCurve(); drawWcapSlideChart(); drawPriceChart(); }
}"""

NEW_SHOW_STAB = """function showStab(parentId, stabId, el) {
  const parent = document.getElementById(parentId);
  parent.querySelectorAll('.stab-panel').forEach(p => p.classList.remove('active'));
  parent.querySelectorAll('.stab').forEach(s => s.classList.remove('active'));
  document.getElementById(stabId).classList.add('active');
  el.classList.add('active');
  if (stabId === 'eqi-charts') { drawGiniHistoryChart(); drawLorenzCurve(); drawWcapSlideChart(); drawPriceChart(); }
  // Push sub-route URL
  const tabSlugMap = {'tab-register':'register','tab-explorer':'explorer','tab-index':'index','tab-network':'network','tab-exchange':'exchange'};
  const stabSlugMap = {'sep-blocks':'blocks','sep-humans':'humans','eqi-score':'score','eqi-economy':'economy','eqi-charts':'charts','net-overview':'overview','net-runnode':'node','net-protocol':'protocol','exch-swap':'swap','exch-liquidity':'liquidity'};
  const tabSlug = tabSlugMap[parentId];
  const stabSlug = stabSlugMap[stabId];
  if (tabSlug && stabSlug) history.pushState(null, '', '/' + tabSlug + '/' + stabSlug);
}"""

if OLD_SHOW_STAB not in src:
    errors.append("MISS: showStab function")
else:
    src = src.replace(OLD_SHOW_STAB, NEW_SHOW_STAB, 1)
    print("showStab() updated with URL push")

# ─────────────────────────────────────────────────────────────────────────────
# 5. Update showTab() — rename 'swap' -> 'exchange', update loadPoolStatus call
# ─────────────────────────────────────────────────────────────────────────────
OLD_SHOW_TAB = """  if (name === 'swap') loadPoolStatus();
  history.pushState(null, '', '/' + name);
}"""
NEW_SHOW_TAB = """  if (name === 'exchange') loadPoolStatus();
  history.pushState(null, '', '/' + name);
}"""
if OLD_SHOW_TAB not in src:
    errors.append("MISS: showTab swap->exchange")
else:
    src = src.replace(OLD_SHOW_TAB, NEW_SHOW_TAB, 1)
    print("showTab() updated")

# ─────────────────────────────────────────────────────────────────────────────
# 6. Update activateTabFromPath() — handle sub-paths, exchange, URL-push
# ─────────────────────────────────────────────────────────────────────────────
OLD_ACTIVATE = """function activateTabFromPath(path) {
  const tabNames = ['register','explorer','index','network','swap'];
  const name = (path || '').replace(/^\\//, '').split('/')[0];
  if (!name || !tabNames.includes(name)) return;
  // Use attribute iteration instead of CSS attribute selector (more reliable cross-browser)
  let tabEl = null;
  document.querySelectorAll('.tab').forEach(t => {
    if ((t.getAttribute('onclick') || '').includes("'" + name + "'")) tabEl = t;
  });
  if (!tabEl) return;
  document.querySelectorAll('.tab-content').forEach(t => t.classList.remove('active'));
  document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
  const tabContent = document.getElementById('tab-' + name);
  if (!tabContent) return;
  tabContent.classList.add('active');
  tabEl.classList.add('active');
  // Restore the first stab-panel within this tab so direct URL navigation works
  const panels = tabContent.querySelectorAll('.stab-panel');
  const stabs = tabContent.querySelectorAll('.stab');
  if (panels.length) {
    panels.forEach(p => p.classList.remove('active'));
    stabs.forEach(s => s.classList.remove('active'));
    panels[0].classList.add('active');
    if (stabs[0]) stabs[0].classList.add('active');
  }
  if (name === 'swap') loadPoolStatus();
}"""

NEW_ACTIVATE = """function activateTabFromPath(path) {
  const tabNames = ['register','explorer','index','network','exchange'];
  const parts = (path || '').replace(/^\\//, '').split('/');
  let name = parts[0];
  const stabSlug = parts[1] || '';
  // Backwards-compat: /swap -> /exchange
  if (name === 'swap') name = 'exchange';
  if (!name || !tabNames.includes(name)) return;
  let tabEl = null;
  document.querySelectorAll('.tab').forEach(t => {
    if ((t.getAttribute('onclick') || '').includes("'" + name + "'")) tabEl = t;
  });
  if (!tabEl) return;
  document.querySelectorAll('.tab-content').forEach(t => t.classList.remove('active'));
  document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
  const tabContent = document.getElementById('tab-' + name);
  if (!tabContent) return;
  tabContent.classList.add('active');
  tabEl.classList.add('active');
  // Activate stab-panel: use URL slug if present, otherwise first panel
  const stabMap = {
    explorer:  {blocks:'sep-blocks', humans:'sep-humans'},
    index:     {score:'eqi-score', economy:'eqi-economy', charts:'eqi-charts'},
    network:   {overview:'net-overview', node:'net-runnode', protocol:'net-protocol'},
    exchange:  {swap:'exch-swap', liquidity:'exch-liquidity'}
  };
  const panels = tabContent.querySelectorAll('.stab-panel');
  const stabs  = tabContent.querySelectorAll('.stab');
  if (panels.length) {
    panels.forEach(p => p.classList.remove('active'));
    stabs.forEach(s => s.classList.remove('active'));
    const targetId = stabSlug && stabMap[name] && stabMap[name][stabSlug];
    const targetEl = targetId ? document.getElementById(targetId) : panels[0];
    if (targetEl) targetEl.classList.add('active');
    // Activate matching stab button
    const stabBtn = targetId
      ? tabContent.querySelector('.stab[onclick*=\\"' + targetId + '\\"]')
      : stabs[0];
    if (stabBtn) stabBtn.classList.add('active');
    else if (stabs[0]) stabs[0].classList.add('active');
  }
  if (name === 'exchange') loadPoolStatus();
}"""

if OLD_ACTIVATE not in src:
    errors.append("MISS: activateTabFromPath function")
else:
    src = src.replace(OLD_ACTIVATE, NEW_ACTIVATE, 1)
    print("activateTabFromPath() updated with sub-route support")

# ─────────────────────────────────────────────────────────────────────────────
# 7. goTab(): update 'swap' reference to 'exchange'
# ─────────────────────────────────────────────────────────────────────────────
# goTab is used internally — no swap reference currently, but check
if "goTab('swap'" in src:
    src = src.replace("goTab('swap'", "goTab('exchange'", )
    print("goTab swap->exchange updated")

# ─────────────────────────────────────────────────────────────────────────────
# 8. checkProofParams(): if it clicks register tab, no change needed
# ─────────────────────────────────────────────────────────────────────────────

# ─────────────────────────────────────────────────────────────────────────────
# 9. Canvas chart improvements: make charts larger and add labels
# ─────────────────────────────────────────────────────────────────────────────
# Find the canvas elements in eqi-charts panel and ensure they have good height
OLD_GINI_CANVAS = 'id="gini-history-chart" style="width:100%;height:140px'
NEW_GINI_CANVAS = 'id="gini-history-chart" style="width:100%;height:200px'
if OLD_GINI_CANVAS in src:
    src = src.replace(OLD_GINI_CANVAS, NEW_GINI_CANVAS, 1)
    print("Gini chart height increased to 200px")

OLD_LORENZ_CANVAS = 'id="lorenz-chart" style="width:100%;height:140px'
NEW_LORENZ_CANVAS = 'id="lorenz-chart" style="width:100%;height:200px'
if OLD_LORENZ_CANVAS in src:
    src = src.replace(OLD_LORENZ_CANVAS, NEW_LORENZ_CANVAS, 1)
    print("Lorenz chart height increased to 200px")

OLD_WCAP_CANVAS = 'id="wcap-slide-chart" style="width:100%;height:100px'
NEW_WCAP_CANVAS = 'id="wcap-slide-chart" style="width:100%;height:140px'
if OLD_WCAP_CANVAS in src:
    src = src.replace(OLD_WCAP_CANVAS, NEW_WCAP_CANVAS, 1)
    print("Wcap chart height increased to 140px")

OLD_PRICE_CANVAS = 'id="price-chart" style="width:100%;height:120px'
NEW_PRICE_CANVAS = 'id="price-chart" style="width:100%;height:180px'
if OLD_PRICE_CANVAS in src:
    src = src.replace(OLD_PRICE_CANVAS, NEW_PRICE_CANVAS, 1)
    print("Price chart height increased to 180px")

# ─────────────────────────────────────────────────────────────────────────────
# Report and write
# ─────────────────────────────────────────────────────────────────────────────
if errors:
    print("ERRORS:")
    for e in errors:
        print(" ", e)
    sys.exit(1)

with open(FILE, 'w', encoding='utf-8') as f:
    f.write(src)
print(f"Done. {len(src.splitlines())} lines written.")
