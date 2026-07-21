#!/usr/bin/env python3
"""Move AEQ/tUSD price chart from eqi-charts to Exchange > Swap tab.
- Remove eqi-charts stab from Equality nav (now empty)
- Remove eqi-charts stab-panel
- Add price chart to top of exch-swap panel
- Add offsetParent check to drawPriceChart
- Trigger drawPriceChart on Exchange tab + Swap stab click
"""
import sys

PATH_CHAIN = r"C:\Users\aequitas-chain\x\humanity\keeper\api_html.go"
PATH_WORK  = r"C:\Users\Benutzer7\aequitas-work\x\humanity\keeper\api_html.go"

with open(PATH_CHAIN, 'r', encoding='utf-8') as f:
    content = f.read()

def rq(old, new, label):
    if old not in content:
        print("NOT FOUND: " + label)
        sys.exit(1)
    return content.replace(old, new, 1)

# ─────────────────────────────────────────────────────────────
# 1. Remove 📈 Charts stab from Equality nav
# ─────────────────────────────────────────────────────────────
OLD = """  <div class="stab" onclick="showStab('tab-index','eqi-charts',this)">📈 Charts</div>
  <div class="stab" onclick="showStab('tab-index','eqi-story',this)">📖 Story</div>"""
NEW = """  <div class="stab" onclick="showStab('tab-index','eqi-story',this)">📖 Story</div>"""
content = rq(OLD, NEW, "remove Charts stab nav"); print("Charts stab removed from nav")

# ─────────────────────────────────────────────────────────────
# 2. Remove eqi-charts stab-panel (now just price chart, moving it)
# ─────────────────────────────────────────────────────────────
OLD = """<div id="eqi-charts" class="stab-panel">
<div class="is">
<div class="idx" style="grid-column:1/-1">
    <div class="idx-title">AEQ / tUSD — Live Price</div>
    <div style="font-size:0.63rem;color:var(--muted);margin-bottom:12px">Real-time price derived from the pool reserves (x·y=k). Updates every 8 seconds as new pool data arrives. Accumulates up to 60 data points.</div>
    <canvas id="price-chart" height="160" style="width:100%;border-radius:6px;background:var(--card2)"></canvas>
    <div id="price-chart-empty" style="display:none;text-align:center;padding:24px;color:var(--muted);font-size:0.63rem">No pool data yet — add liquidity to see the price chart.</div>
  </div>
</div>
</div>"""
NEW = ""
content = rq(OLD, NEW, "remove eqi-charts panel"); print("eqi-charts panel removed")

# ─────────────────────────────────────────────────────────────
# 3. Add price chart to top of exch-swap (above the swap form)
# ─────────────────────────────────────────────────────────────
OLD = """<div id="exch-swap" class="stab-panel active">
<div class="rs">"""
NEW = """<div id="exch-swap" class="stab-panel active">
<div style="padding:16px 20px 0">
  <div class="idx">
    <div class="idx-title">AEQ / tUSD — Live Price</div>
    <div style="font-size:0.63rem;color:var(--muted);margin-bottom:12px">Real-time price derived from pool reserves (x·y=k). Updates every 8 seconds as new pool data arrives.</div>
    <canvas id="price-chart" height="160" style="width:100%;border-radius:6px;background:var(--card2)"></canvas>
    <div id="price-chart-empty" style="display:none;text-align:center;padding:24px;color:var(--muted);font-size:0.63rem">No pool data yet — add liquidity to see the price chart.</div>
  </div>
</div>
<div class="rs">"""
content = rq(OLD, NEW, "add price chart to exch-swap"); print("Price chart added to exch-swap")

# ─────────────────────────────────────────────────────────────
# 4. Exchange tab onclick → also trigger drawPriceChart
# ─────────────────────────────────────────────────────────────
OLD = """  <div class="tab" onclick="showTab('exchange',this)">🔄 Exchange</div>"""
NEW = """  <div class="tab" onclick="showTab('exchange',this);setTimeout(drawPriceChart,50)">🔄 Exchange</div>"""
content = rq(OLD, NEW, "Exchange tab onclick"); print("Exchange tab onclick updated")

# ─────────────────────────────────────────────────────────────
# 5. Swap stab onclick → also trigger drawPriceChart
# ─────────────────────────────────────────────────────────────
OLD = """  <div class="stab active" onclick="showStab('tab-exchange','exch-swap',this)">🔄 Swap</div>"""
NEW = """  <div class="stab active" onclick="showStab('tab-exchange','exch-swap',this);setTimeout(drawPriceChart,50)">🔄 Swap</div>"""
content = rq(OLD, NEW, "Swap stab onclick"); print("Swap stab onclick updated")

# ─────────────────────────────────────────────────────────────
# 6. Add offsetParent check to drawPriceChart
# ─────────────────────────────────────────────────────────────
OLD = """function drawPriceChart() {
  const canvas = document.getElementById('price-chart');
  if (!canvas || !priceHistory.length) return;
  canvas.width = canvas.offsetWidth;"""
NEW = """function drawPriceChart() {
  const canvas = document.getElementById('price-chart');
  if (!canvas || !priceHistory.length || !canvas.offsetParent) return;
  canvas.width = canvas.offsetWidth;"""
content = rq(OLD, NEW, "offsetParent check"); print("offsetParent check added to drawPriceChart")

# ─────────────────────────────────────────────────────────────
# Write files
# ─────────────────────────────────────────────────────────────
with open(PATH_CHAIN, 'w', encoding='utf-8') as f:
    f.write(content)
print("Written:", PATH_CHAIN)

with open(PATH_WORK, 'w', encoding='utf-8') as f:
    f.write(content)
print("Written:", PATH_WORK)
print("\nDone!")
