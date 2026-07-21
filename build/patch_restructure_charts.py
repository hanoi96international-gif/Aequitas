#!/usr/bin/env python3
"""Restructure chart placement:
- eqi-charts: only AEQ/tUSD price chart
- eqi-score: add Gini History + Lorenz Curve (after score content)
- eqi-economy: add Wealth Cap chart (after demurrage)
- new eqi-story sub-tab: "The Story of Aequitas"
- fix pathBez: function declaration → var expression (avoids try-block hoisting issue)
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
# 1. Fix pathBez: function declaration → var expression
# ─────────────────────────────────────────────────────────────
OLD = """    function pathBez(pts) {
      ctx.moveTo(toX(0), toY(pts[0].idx));
      if (pts.length<3) { for(var k=1;k<pts.length;k++) ctx.lineTo(toX(k),toY(pts[k].idx)); return; }
      for (var k=1;k<pts.length-1;k++) {
        var mx=(toX(k)+toX(k+1))/2, my=(toY(pts[k].idx)+toY(pts[k+1].idx))/2;
        ctx.quadraticCurveTo(toX(k),toY(pts[k].idx),mx,my);
      }
      ctx.lineTo(toX(pts.length-1), toY(pts[pts.length-1].idx));
    }"""
NEW = """    var pathBez = function(pts) {
      ctx.moveTo(toX(0), toY(pts[0].idx));
      if (pts.length<3) { for(var k=1;k<pts.length;k++) ctx.lineTo(toX(k),toY(pts[k].idx)); return; }
      for (var k=1;k<pts.length-1;k++) {
        var mx=(toX(k)+toX(k+1))/2, my=(toY(pts[k].idx)+toY(pts[k+1].idx))/2;
        ctx.quadraticCurveTo(toX(k),toY(pts[k].idx),mx,my);
      }
      ctx.lineTo(toX(pts.length-1), toY(pts[pts.length-1].idx));
    };"""
content = rq(OLD, NEW, "pathBez fix"); print("pathBez fixed")

# ─────────────────────────────────────────────────────────────
# 2. Add Story sub-tab to eqi nav
# ─────────────────────────────────────────────────────────────
OLD = """<nav class="stabs">
  <div class="stab active" onclick="showStab('tab-index','eqi-score',this)">📊 Score</div>
  <div class="stab" onclick="showStab('tab-index','eqi-economy',this)">💸 Economy</div>
  <div class="stab" onclick="showStab('tab-index','eqi-charts',this)">📈 Charts</div>
</nav>"""
NEW = """<nav class="stabs">
  <div class="stab active" onclick="showStab('tab-index','eqi-score',this)">📊 Score</div>
  <div class="stab" onclick="showStab('tab-index','eqi-economy',this)">💸 Economy</div>
  <div class="stab" onclick="showStab('tab-index','eqi-charts',this)">📈 Charts</div>
  <div class="stab" onclick="showStab('tab-index','eqi-story',this)">📖 Story</div>
</nav>"""
content = rq(OLD, NEW, "story stab nav"); print("Story stab nav added")

# ─────────────────────────────────────────────────────────────
# 3. Append Gini History + Lorenz to end of eqi-score (inside .is, after the idx)
# ─────────────────────────────────────────────────────────────
SCORE_END = """    </div>
  </div>
</div>
</div>
<div id="eqi-economy" class="stab-panel">"""

SCORE_END_NEW = """    </div>
  </div>
  <div class="idx" style="grid-column:1/-1">
    <div class="idx-title">Gini Index History</div>
    <div style="font-size:0.63rem;color:var(--muted);margin-bottom:12px">Recorded after each UBI distribution. Shows how equality evolves as the network grows. Lower is better — target is below 35.</div>
    <canvas id="gini-history-chart" height="160" style="width:100%;border-radius:6px;background:var(--card2)"></canvas>
    <div id="gini-history-empty" style="display:none;text-align:center;padding:24px;color:var(--muted);font-size:0.63rem">No snapshots yet — first one saved after the next UBI distribution.</div>
  </div>
  <div class="idx" style="grid-column:1/-1">
    <div class="idx-title">Lorenz Curve — Wealth Distribution Across Humans</div>
    <div style="font-size:0.63rem;color:var(--muted);margin-bottom:12px">Each point = cumulative % of AEQ held by the poorest X% of humans. The diagonal = perfect equality. The further the curve bows below the diagonal, the higher the Gini.</div>
    <canvas id="lorenz-chart" height="270" style="width:100%;border-radius:6px;background:var(--card2)"></canvas>
  </div>
</div>
</div>
<div id="eqi-economy" class="stab-panel">"""
content = rq(SCORE_END, SCORE_END_NEW, "eqi-score append"); print("Gini + Lorenz added to eqi-score")

# ─────────────────────────────────────────────────────────────
# 4. Add Wealth Cap chart to eqi-economy (after demurrage table)
# ─────────────────────────────────────────────────────────────
OLD = """      <tr><td data-i18n="dem-warn-k">Warning System</td><td data-i18n="dem-warn-v">14-day notice (once) + 7-day repeated reminder at login</td></tr>
    </table>
  </div>
</div>
</div>
<div id="eqi-charts" class="stab-panel">"""
NEW = """      <tr><td data-i18n="dem-warn-k">Warning System</td><td data-i18n="dem-warn-v">14-day notice (once) + 7-day repeated reminder at login</td></tr>
    </table>
  </div>
  <div class="idx" style="grid-column:1/-1">
    <div class="idx-title">Wealth Cap Multiplier — Bootstrap Slider</div>
    <div style="font-size:0.63rem;color:var(--muted);margin-bottom:12px">Formula: <code style="color:var(--teal)">max(5, min(N, 25))×</code> average AEQ balance. Each new human slides the cap up by 1×, until the 25th human locks it at 25× permanently.</div>
    <canvas id="wcap-slide-chart" height="120" style="width:100%;border-radius:6px;background:var(--card2)"></canvas>
  </div>
</div>
</div>
<div id="eqi-charts" class="stab-panel">"""
content = rq(OLD, NEW, "wcap chart to eqi-economy"); print("Wealth Cap chart added to eqi-economy")

# ─────────────────────────────────────────────────────────────
# 5. Strip eqi-charts down to only the price chart
#    Remove: Gini History, Lorenz, Wealth Cap, Story blocks
# ─────────────────────────────────────────────────────────────
OLD_CHARTS = """<div id="eqi-charts" class="stab-panel">
<div class="is">
<div class="idx" style="grid-column:1/-1">
    <div class="idx-title">AEQ / tUSD — Live Price</div>
    <div style="font-size:0.63rem;color:var(--muted);margin-bottom:12px">Real-time price derived from the pool reserves (x·y=k). Updates every 8 seconds as new pool data arrives. Accumulates up to 60 data points.</div>
    <canvas id="price-chart" height="160" style="width:100%;border-radius:6px;background:var(--card2)"></canvas>
    <div id="price-chart-empty" style="display:none;text-align:center;padding:24px;color:var(--muted);font-size:0.63rem">No pool data yet — add liquidity to see the price chart.</div>
  </div>
<div class="idx" style="grid-column:1/-1">
    <div class="idx-title">Gini Index History</div>
    <div style="font-size:0.63rem;color:var(--muted);margin-bottom:12px">Recorded after each UBI distribution. Shows how equality evolves as the network grows.</div>
    <canvas id="gini-history-chart" height="160" style="width:100%;border-radius:6px;background:var(--card2)"></canvas>
    <div id="gini-history-empty" style="display:none;text-align:center;padding:24px;color:var(--muted);font-size:0.63rem">No snapshots yet — first one saved after the next UBI distribution.</div>
  </div>
  <div class="idx" style="grid-column:1/-1">
    <div class="idx-title">Lorenz Curve — Wealth Distribution Across Humans</div>
    <div style="font-size:0.63rem;color:var(--muted);margin-bottom:12px">Each point = cumulative % of AEQ held by the poorest X% of humans. The diagonal = perfect equality. The further the curve bows below the diagonal, the higher the Gini.</div>
    <canvas id="lorenz-chart" height="270" style="width:100%;border-radius:6px;background:var(--card2)"></canvas>
  </div>
  <div class="idx" style="grid-column:1/-1">
    <div class="idx-title">Wealth Cap Multiplier — Bootstrap Slider</div>
    <div style="font-size:0.63rem;color:var(--muted);margin-bottom:12px">Formula: <code style="color:var(--teal)">max(5, min(N, 25))×</code> average AEQ balance. Each new human slides the cap up by 1×, until the 25th human locks it at 25× permanently.</div>
    <canvas id="wcap-slide-chart" height="120" style="width:100%;border-radius:6px;background:var(--card2)"></canvas>
  </div>
  <div class="idx" style="grid-column:1/-1">
    <div class="idx-title" data-i18n="story-title">The Story of Aequitas — Why This Exists</div>
    <div class="story" data-i18n="story-text"><p>The year is 2009. Satoshi Nakamoto releases Bitcoin. For the first time, value can transfer between any two people without a bank. A genuine revolution. But something goes wrong almost immediately.</p><p>Early miners accumulate millions of coins at almost zero cost. By 2021, the top 1% of Bitcoin addresses control over 90% of all Bitcoin. Bitcoin's estimated Gini coefficient exceeds 0.85 — higher than any country on Earth. The cryptocurrency that was supposed to democratize finance created the most extreme wealth concentration in human history.</p><p><span style="color:var(--gold)">Aequitas</span> — Latin for "fairness" and "equality" — was created to answer a single question: <em style="color:var(--gold)">"What would a cryptocurrency look like if designed from first principles to be fair to every human being?"</em></p><p>The answer is simple: <strong style="color:var(--text)">Money exists because people exist. Therefore, every person should have an equal share of money simply by virtue of being human.</strong></p><p>Aequitas implements this principle mathematically. Every verified human receives 1,000 AEQ. No mining, no staking, no early-adopter advantage. The wealth cap, demurrage, and redistribution pools ensure that inequality cannot accumulate indefinitely. The Gini coefficient and Aequitas Index are calculated on-chain in real time, and the protocol adjusts automatically.</p><p>The Aequitas network launched in June 2026. Currently in Phase 0 (Bootstrap). The goal: demonstrate that money can be distributed fairly, Gini coefficient held below 0.35 (comparable to the most equal developed nations), and financial inclusion achieved at global scale — without any central authority.</p><p><em style="color:var(--gold)">"Money exists because people exist. Nothing more, nothing less."</em></p></div>
  </div>
</div>
</div>
</div>"""

NEW_CHARTS = """<div id="eqi-charts" class="stab-panel">
<div class="is">
<div class="idx" style="grid-column:1/-1">
    <div class="idx-title">AEQ / tUSD — Live Price</div>
    <div style="font-size:0.63rem;color:var(--muted);margin-bottom:12px">Real-time price derived from the pool reserves (x·y=k). Updates every 8 seconds as new pool data arrives. Accumulates up to 60 data points.</div>
    <canvas id="price-chart" height="160" style="width:100%;border-radius:6px;background:var(--card2)"></canvas>
    <div id="price-chart-empty" style="display:none;text-align:center;padding:24px;color:var(--muted);font-size:0.63rem">No pool data yet — add liquidity to see the price chart.</div>
  </div>
</div>
</div>
<div id="eqi-story" class="stab-panel">
<div class="is">
  <div class="idx" style="grid-column:1/-1">
    <div class="idx-title" data-i18n="story-title">The Story of Aequitas — Why This Exists</div>
    <div class="story" data-i18n="story-text"><p>The year is 2009. Satoshi Nakamoto releases Bitcoin. For the first time, value can transfer between any two people without a bank. A genuine revolution. But something goes wrong almost immediately.</p><p>Early miners accumulate millions of coins at almost zero cost. By 2021, the top 1% of Bitcoin addresses control over 90% of all Bitcoin. Bitcoin's estimated Gini coefficient exceeds 0.85 — higher than any country on Earth. The cryptocurrency that was supposed to democratize finance created the most extreme wealth concentration in human history.</p><p><span style="color:var(--gold)">Aequitas</span> — Latin for "fairness" and "equality" — was created to answer a single question: <em style="color:var(--gold)">"What would a cryptocurrency look like if designed from first principles to be fair to every human being?"</em></p><p>The answer is simple: <strong style="color:var(--text)">Money exists because people exist. Therefore, every person should have an equal share of money simply by virtue of being human.</strong></p><p>Aequitas implements this principle mathematically. Every verified human receives 1,000 AEQ. No mining, no staking, no early-adopter advantage. The wealth cap, demurrage, and redistribution pools ensure that inequality cannot accumulate indefinitely. The Gini coefficient and Aequitas Index are calculated on-chain in real time, and the protocol adjusts automatically.</p><p>The Aequitas network launched in June 2026. Currently in Phase 0 (Bootstrap). The goal: demonstrate that money can be distributed fairly, Gini coefficient held below 0.35 (comparable to the most equal developed nations), and financial inclusion achieved at global scale — without any central authority.</p><p><em style="color:var(--gold)">"Money exists because people exist. Nothing more, nothing less."</em></p></div>
  </div>
</div>
</div>
</div>"""
content = rq(OLD_CHARTS, NEW_CHARTS, "eqi-charts cleanup + story panel"); print("eqi-charts stripped + eqi-story created")

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
