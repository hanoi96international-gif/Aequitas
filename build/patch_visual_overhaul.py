#!/usr/bin/env python3
"""Full visual overhaul: all 4 charts + CSS improvements for api_html.go"""

import sys

PATH_CHAIN = r"C:\Users\aequitas-chain\x\humanity\keeper\api_html.go"
PATH_WORK  = r"C:\Users\Benutzer7\aequitas-work\x\humanity\keeper\api_html.go"

with open(PATH_CHAIN, 'r', encoding='utf-8') as f:
    content = f.read()

def require_replace(old, new, label):
    if old not in content:
        print("ERROR: not found: " + label)
        sys.exit(1)
    return content.replace(old, new)

# ─────────────────────────────────────────────────────────────
# 1. Canvas heights
# ─────────────────────────────────────────────────────────────
content = content.replace('<canvas id="price-chart" height="110"',       '<canvas id="price-chart" height="160"')
content = content.replace('<canvas id="gini-history-chart" height="120"','<canvas id="gini-history-chart" height="160"')
content = content.replace('<canvas id="lorenz-chart" height="220"',      '<canvas id="lorenz-chart" height="270"')
content = content.replace('<canvas id="wcap-slide-chart" height="90"',   '<canvas id="wcap-slide-chart" height="120"')
print("Canvas heights updated")

# ─────────────────────────────────────────────────────────────
# 2. CSS: .idx card hover + title bar
# ─────────────────────────────────────────────────────────────
OLD = """.idx{background:var(--card);border:1px solid var(--border);border-radius:var(--radius);padding:24px;box-shadow:var(--glow-purple)}
.idx-title{font-size:0.6rem;color:var(--purple);letter-spacing:2px;text-transform:uppercase;margin-bottom:10px;font-weight:600}"""
NEW = """.idx{background:var(--card);border:1px solid var(--border);border-radius:var(--radius);padding:24px;box-shadow:var(--glow-purple);transition:border-color 0.25s,box-shadow 0.25s}
.idx:hover{border-color:rgba(139,92,246,0.32);box-shadow:0 0 30px rgba(139,92,246,0.18)}
.idx-title{font-size:0.6rem;color:var(--purple);letter-spacing:2px;text-transform:uppercase;margin-bottom:12px;font-weight:700;display:flex;align-items:center;gap:8px}
.idx-title::before{content:'';display:inline-block;width:3px;height:12px;background:linear-gradient(180deg,var(--purple),var(--teal));border-radius:2px;flex-shrink:0}"""
content = require_replace(OLD, NEW, ".idx CSS"); print(".idx CSS improved")

# ─────────────────────────────────────────────────────────────
# 3. CSS: .nc card hover + title bar
# ─────────────────────────────────────────────────────────────
OLD = """.nc{background:var(--card);border:1px solid var(--border);border-radius:var(--radius);padding:20px;box-shadow:var(--glow-purple)}
.nc-title{font-size:0.6rem;color:var(--purple);letter-spacing:1.5px;text-transform:uppercase;margin-bottom:14px;font-weight:600}"""
NEW = """.nc{background:var(--card);border:1px solid var(--border);border-radius:var(--radius);padding:20px;box-shadow:var(--glow-purple);transition:border-color 0.25s,box-shadow 0.25s}
.nc:hover{border-color:rgba(139,92,246,0.32);box-shadow:0 0 30px rgba(139,92,246,0.18)}
.nc-title{font-size:0.6rem;color:var(--purple);letter-spacing:1.5px;text-transform:uppercase;margin-bottom:14px;font-weight:700;display:flex;align-items:center;gap:8px}
.nc-title::before{content:'';display:inline-block;width:3px;height:12px;background:linear-gradient(180deg,var(--purple),var(--teal));border-radius:2px;flex-shrink:0}"""
content = require_replace(OLD, NEW, ".nc CSS"); print(".nc CSS improved")

# ─────────────────────────────────────────────────────────────
# 4. CSS: .spect table row hover
# ─────────────────────────────────────────────────────────────
OLD = """.spect{width:100%;border-collapse:collapse}
.spect td{padding:8px 0;border-bottom:1px solid rgba(139,92,246,0.08);font-size:0.63rem}
.spect tr:last-child td{border-bottom:none}
.spect td:first-child{color:var(--muted);width:45%}
.spect td:last-child{text-align:right;font-family:var(--font-mono);font-size:0.6rem;color:var(--purple)}"""
NEW = """.spect{width:100%;border-collapse:collapse}
.spect td{padding:9px 4px;border-bottom:1px solid rgba(139,92,246,0.08);font-size:0.63rem;transition:background 0.15s}
.spect tr:hover td{background:rgba(139,92,246,0.05)}
.spect tr:last-child td{border-bottom:none}
.spect td:first-child{color:var(--muted);width:45%;padding-left:2px}
.spect td:last-child{text-align:right;font-family:var(--font-mono);font-size:0.6rem;color:var(--purple);padding-right:2px}"""
content = require_replace(OLD, NEW, ".spect CSS"); print(".spect CSS improved")

# ─────────────────────────────────────────────────────────────
# 5. CSS: .stab sub-tab active indicator line
# ─────────────────────────────────────────────────────────────
OLD = """.stabs{display:flex;gap:2px;padding:8px 20px 0;overflow-x:auto;background:rgba(8,0,16,0.5);border-bottom:1px solid rgba(139,92,246,0.1);-webkit-overflow-scrolling:touch;scrollbar-width:none}
.stabs::-webkit-scrollbar{display:none}
.stab{padding:7px 15px;font-size:0.6rem;color:var(--muted);cursor:pointer;border-radius:20px 20px 0 0;letter-spacing:0.5px;font-weight:600;white-space:nowrap;transition:all 0.2s;border:1px solid transparent;border-bottom:none;flex-shrink:0}
.stab:hover{color:var(--text);background:rgba(139,92,246,0.08)}
.stab.active{color:var(--purple);background:rgba(139,92,246,0.12);border-color:rgba(139,92,246,0.2)}"""
NEW = """.stabs{display:flex;gap:2px;padding:8px 20px 0;overflow-x:auto;background:rgba(8,0,16,0.5);border-bottom:1px solid rgba(139,92,246,0.1);-webkit-overflow-scrolling:touch;scrollbar-width:none}
.stabs::-webkit-scrollbar{display:none}
.stab{padding:7px 15px;font-size:0.6rem;color:var(--muted);cursor:pointer;border-radius:6px 6px 0 0;letter-spacing:0.5px;font-weight:600;white-space:nowrap;transition:all 0.2s;border:1px solid transparent;border-bottom:none;flex-shrink:0;position:relative}
.stab:hover{color:var(--text);background:rgba(139,92,246,0.1)}
.stab.active{color:var(--purple);background:rgba(139,92,246,0.14);border-color:rgba(139,92,246,0.22)}
.stab.active::after{content:'';position:absolute;bottom:-1px;left:0;right:0;height:2px;background:linear-gradient(90deg,var(--purple),var(--teal));border-radius:2px 2px 0 0}"""
content = require_replace(OLD, NEW, ".stab CSS"); print(".stab CSS improved")

# ─────────────────────────────────────────────────────────────
# 6. drawGiniHistoryChart — smooth bezier + gradient + glow
# ─────────────────────────────────────────────────────────────
OLD = """async function drawGiniHistoryChart() {
  const canvas = document.getElementById('gini-history-chart');
  if (!canvas || !canvas.offsetParent) return;
  canvas.width = canvas.offsetWidth;
  const ctx = canvas.getContext('2d');
  const W = canvas.width, H = canvas.height;
  ctx.clearRect(0, 0, W, H);
  try {
    const d = await (await fetch('/api/gini/history')).json();
    const history = (d.history || []).slice().reverse();
    const emptyEl = document.getElementById('gini-history-empty');
    if (!history.length) {
      if (emptyEl) { emptyEl.style.display = 'block'; canvas.style.display = 'none'; } return;
    }
    if (emptyEl) { emptyEl.style.display = 'none'; canvas.style.display = 'block'; }
    const pad = {l:36,r:16,t:14,b:8};
    ctx.strokeStyle = 'rgba(255,255,255,0.05)'; ctx.lineWidth = 1;
    for (let i = 0; i <= 4; i++) {
      const y = pad.t + (H-pad.t-pad.b)*(1-i/4);
      ctx.beginPath(); ctx.moveTo(pad.l,y); ctx.lineTo(W-pad.r,y); ctx.stroke();
      ctx.fillStyle = 'rgba(255,255,255,0.3)'; ctx.font = '9px monospace'; ctx.textAlign = 'right';
      ctx.fillText((i*25)+'', pad.l-3, y+3);
    }
    const targetY = pad.t + (H-pad.t-pad.b)*(1-35/100);
    ctx.strokeStyle = 'rgba(0,255,209,0.3)'; ctx.setLineDash([4,4]);
    ctx.beginPath(); ctx.moveTo(pad.l,targetY); ctx.lineTo(W-pad.r,targetY); ctx.stroke();
    ctx.setLineDash([]); ctx.fillStyle = 'rgba(0,255,209,0.6)'; ctx.font = '8px monospace'; ctx.textAlign = 'left';
    ctx.fillText('Target 35', W-pad.r-48, targetY-3);
    const xs = (W-pad.l-pad.r)/Math.max(history.length-1,1);
    ctx.strokeStyle = '#C9A84C'; ctx.lineWidth = 2;
    ctx.beginPath();
    history.forEach((pt,i)=>{ const x=pad.l+i*xs, y=pad.t+(H-pad.t-pad.b)*(1-pt.idx/100); if(i===0)ctx.moveTo(x,y); else ctx.lineTo(x,y); });
    ctx.stroke();
    ctx.fillStyle = '#C9A84C';
    history.forEach((pt,i)=>{ const x=pad.l+i*xs, y=pad.t+(H-pad.t-pad.b)*(1-pt.idx/100); ctx.beginPath(); ctx.arc(x,y,3,0,2*Math.PI); ctx.fill(); });
  } catch(e) {}
}"""
NEW = """async function drawGiniHistoryChart() {
  const canvas = document.getElementById('gini-history-chart');
  if (!canvas || !canvas.offsetParent) return;
  canvas.width = canvas.offsetWidth;
  const ctx = canvas.getContext('2d');
  const W = canvas.width, H = canvas.height;
  ctx.clearRect(0, 0, W, H);
  try {
    const d = await (await fetch('/api/gini/history')).json();
    const history = (d.history || []).slice().reverse();
    const emptyEl = document.getElementById('gini-history-empty');
    if (!history.length) {
      if (emptyEl) { emptyEl.style.display = 'block'; canvas.style.display = 'none'; } return;
    }
    if (emptyEl) { emptyEl.style.display = 'none'; canvas.style.display = 'block'; }
    const pad = {l:48,r:24,t:36,b:32};
    const cW = W-pad.l-pad.r, cH = H-pad.t-pad.b;
    const toX = (i) => pad.l + cW*i/Math.max(history.length-1,1);
    const toY = (v) => pad.t + cH*(1-v/100);
    // danger zone (>70) subtle red tint
    const dg = ctx.createLinearGradient(0,toY(100),0,toY(70));
    dg.addColorStop(0,'rgba(248,113,113,0.06)'); dg.addColorStop(1,'rgba(248,113,113,0)');
    ctx.fillStyle=dg; ctx.fillRect(pad.l,toY(100),cW,toY(70)-toY(100));
    // grid lines
    for (let i=0;i<=4;i++) {
      const v=i*25, y=toY(v);
      ctx.strokeStyle = v===0?'rgba(139,92,246,0.2)':'rgba(139,92,246,0.08)';
      ctx.lineWidth = v===0?1.5:1;
      ctx.beginPath(); ctx.moveTo(pad.l,y); ctx.lineTo(W-pad.r,y); ctx.stroke();
      ctx.fillStyle='rgba(200,168,76,0.75)'; ctx.font='10px JetBrains Mono,monospace'; ctx.textAlign='right';
      ctx.fillText(v+'', pad.l-6, y+4);
    }
    // target 35 line
    const targetY = toY(35);
    ctx.save(); ctx.shadowColor='rgba(0,255,209,0.7)'; ctx.shadowBlur=5;
    ctx.strokeStyle='rgba(0,255,209,0.55)'; ctx.lineWidth=1.5; ctx.setLineDash([6,5]);
    ctx.beginPath(); ctx.moveTo(pad.l,targetY); ctx.lineTo(W-pad.r,targetY); ctx.stroke();
    ctx.setLineDash([]); ctx.restore();
    ctx.fillStyle='rgba(0,255,209,0.85)'; ctx.font='bold 9px JetBrains Mono,monospace'; ctx.textAlign='right';
    ctx.fillText('TARGET 35', W-pad.r-2, targetY-5);
    // bezier path helper
    function pathBez(pts) {
      ctx.moveTo(toX(0), toY(pts[0].idx));
      if (pts.length<3) { for(var k=1;k<pts.length;k++) ctx.lineTo(toX(k),toY(pts[k].idx)); return; }
      for (var k=1;k<pts.length-1;k++) {
        var mx=(toX(k)+toX(k+1))/2, my=(toY(pts[k].idx)+toY(pts[k+1].idx))/2;
        ctx.quadraticCurveTo(toX(k),toY(pts[k].idx),mx,my);
      }
      ctx.lineTo(toX(pts.length-1), toY(pts[pts.length-1].idx));
    }
    // gradient fill
    var fg=ctx.createLinearGradient(0,pad.t,0,H-pad.b);
    fg.addColorStop(0,'rgba(200,168,76,0.28)'); fg.addColorStop(0.7,'rgba(200,168,76,0.07)'); fg.addColorStop(1,'rgba(200,168,76,0.01)');
    ctx.beginPath(); pathBez(history);
    ctx.lineTo(toX(history.length-1),H-pad.b); ctx.lineTo(toX(0),H-pad.b); ctx.closePath();
    ctx.fillStyle=fg; ctx.fill();
    // glowing line
    ctx.save(); ctx.shadowColor='rgba(200,168,76,0.6)'; ctx.shadowBlur=10;
    ctx.strokeStyle='#C9A84C'; ctx.lineWidth=2.5;
    ctx.beginPath(); pathBez(history); ctx.stroke(); ctx.restore();
    // dots
    history.forEach(function(pt,i){
      var x=toX(i), y=toY(pt.idx);
      ctx.save(); ctx.shadowColor='rgba(200,168,76,0.9)'; ctx.shadowBlur=12;
      ctx.beginPath(); ctx.arc(x,y,4.5,0,2*Math.PI); ctx.fillStyle='#C9A84C'; ctx.fill(); ctx.restore();
      ctx.beginPath(); ctx.arc(x,y,2,0,2*Math.PI); ctx.fillStyle='#fff'; ctx.fill();
    });
    // latest value label
    var lpt=history[history.length-1], lx=toX(history.length-1), ly=toY(lpt.idx);
    ctx.fillStyle='rgba(200,168,76,0.95)'; ctx.font='bold 11px JetBrains Mono,monospace';
    ctx.textAlign = lx>W*0.7?'right':'left';
    ctx.fillText('Gini: '+lpt.idx.toFixed(3), lx+(lx>W*0.7?-8:8), ly-9);
    // title
    ctx.fillStyle='rgba(200,168,76,0.38)'; ctx.font='10px Inter,sans-serif'; ctx.textAlign='left';
    ctx.fillText('GINI INDEX HISTORY  —  0 = perfect equality  ·  100 = max inequality', pad.l, 20);
  } catch(e) {}
}"""
content = require_replace(OLD, NEW, "drawGiniHistoryChart"); print("drawGiniHistoryChart replaced")

# ─────────────────────────────────────────────────────────────
# 7. drawLorenzCurve — gradient fill + glow + axis labels + Gini annotation
# ─────────────────────────────────────────────────────────────
OLD = """async function drawLorenzCurve() {
  const canvas = document.getElementById('lorenz-chart');
  if (!canvas || !canvas.offsetParent) return;
  canvas.width = canvas.offsetWidth;
  const ctx = canvas.getContext('2d');
  const W = canvas.width, H = canvas.height;
  ctx.clearRect(0,0,W,H);
  const pad = {l:36,r:16,t:14,b:28};
  try {
    const d = await (await fetch('/api/humans')).json();
    const humans = d.humans || [];
    if (humans.length < 2) {
      ctx.fillStyle = 'rgba(255,255,255,0.3)'; ctx.font = '11px monospace'; ctx.textAlign = 'center';
      ctx.fillText('Not enough humans registered yet', W/2, H/2); return;
    }
    const bals = humans.map(h=>h.balance||0).sort((a,b)=>a-b);
    const total = bals.reduce((s,b)=>s+b,0);
    const n = bals.length;
    ctx.strokeStyle = 'rgba(255,255,255,0.12)'; ctx.setLineDash([4,4]); ctx.lineWidth = 1;
    ctx.beginPath(); ctx.moveTo(pad.l,H-pad.b); ctx.lineTo(W-pad.r,pad.t); ctx.stroke();
    ctx.setLineDash([]);
    for (let i=0;i<=4;i++) {
      const x=pad.l+(W-pad.l-pad.r)*i/4, y=pad.t+(H-pad.t-pad.b)*(1-i/4);
      ctx.strokeStyle='rgba(255,255,255,0.05)'; ctx.beginPath(); ctx.moveTo(x,pad.t); ctx.lineTo(x,H-pad.b); ctx.stroke();
      ctx.fillStyle='rgba(255,255,255,0.25)'; ctx.font='9px monospace'; ctx.textAlign='center'; ctx.fillText((i*25)+'%',x,H-4);
    }
    ctx.strokeStyle='#C9A84C'; ctx.lineWidth=2;
    ctx.beginPath(); ctx.moveTo(pad.l,H-pad.b);
    let cum=0;
    bals.forEach((b,i)=>{ cum+=b; const x=pad.l+(W-pad.l-pad.r)*(i+1)/n, y=(H-pad.b)-(H-pad.t-pad.b)*(cum/total); ctx.lineTo(x,y); });
    ctx.stroke();
    ctx.fillStyle='rgba(200,168,76,0.12)'; ctx.beginPath(); ctx.moveTo(pad.l,H-pad.b);
    cum=0; bals.forEach((b,i)=>{ cum+=b; const x=pad.l+(W-pad.l-pad.r)*(i+1)/n, y=(H-pad.b)-(H-pad.t-pad.b)*(cum/total); ctx.lineTo(x,y); });
    ctx.lineTo(W-pad.r,H-pad.b); ctx.closePath(); ctx.fill();
    ctx.fillStyle='rgba(255,255,255,0.3)'; ctx.font='9px monospace'; ctx.textAlign='center';
    ctx.fillText('% of AEQ (cumulative)', pad.l-30, H/2);
  } catch(e) {}
}"""
NEW = """async function drawLorenzCurve() {
  const canvas = document.getElementById('lorenz-chart');
  if (!canvas || !canvas.offsetParent) return;
  canvas.width = canvas.offsetWidth;
  const ctx = canvas.getContext('2d');
  const W = canvas.width, H = canvas.height;
  ctx.clearRect(0,0,W,H);
  const pad = {l:48,r:24,t:36,b:44};
  const cW = W-pad.l-pad.r, cH = H-pad.t-pad.b;
  try {
    const d = await (await fetch('/api/humans')).json();
    const humans = d.humans || [];
    if (humans.length < 2) {
      ctx.fillStyle='rgba(139,92,246,0.5)'; ctx.font='12px Inter,sans-serif'; ctx.textAlign='center';
      ctx.fillText('Awaiting more registered humans...', W/2, H/2); return;
    }
    const bals = humans.map(function(h){return h.balance||0;}).sort(function(a,b){return a-b;});
    const total = bals.reduce(function(s,b){return s+b;},0);
    const n = bals.length;
    // grid
    for (var i=0;i<=4;i++) {
      var gx=pad.l+cW*i/4, gy=pad.t+cH*(1-i/4);
      ctx.strokeStyle='rgba(139,92,246,0.08)'; ctx.lineWidth=1;
      ctx.beginPath(); ctx.moveTo(gx,pad.t); ctx.lineTo(gx,H-pad.b); ctx.stroke();
      ctx.beginPath(); ctx.moveTo(pad.l,gy); ctx.lineTo(W-pad.r,gy); ctx.stroke();
      ctx.fillStyle='rgba(200,168,76,0.6)'; ctx.font='10px JetBrains Mono,monospace'; ctx.textAlign='center';
      ctx.fillText((i*25)+'%', gx, H-pad.b+16);
      ctx.textAlign='right';
      ctx.fillText((i*25)+'%', pad.l-6, H-pad.b-cH*i/4+4);
    }
    // axis labels
    ctx.save(); ctx.translate(13,pad.t+cH/2); ctx.rotate(-Math.PI/2);
    ctx.fillStyle='rgba(139,92,246,0.55)'; ctx.font='10px Inter,sans-serif'; ctx.textAlign='center';
    ctx.fillText('% of AEQ held (cumulative)', 0, 0); ctx.restore();
    ctx.fillStyle='rgba(139,92,246,0.55)'; ctx.font='10px Inter,sans-serif'; ctx.textAlign='center';
    ctx.fillText('% of Population (poorest → richest)', pad.l+cW/2, H-2);
    // equality diagonal (gradient)
    var dg=ctx.createLinearGradient(pad.l,H-pad.b,W-pad.r,pad.t);
    dg.addColorStop(0,'rgba(139,92,246,0.4)'); dg.addColorStop(1,'rgba(6,182,212,0.4)');
    ctx.strokeStyle=dg; ctx.lineWidth=1.5; ctx.setLineDash([6,5]);
    ctx.beginPath(); ctx.moveTo(pad.l,H-pad.b); ctx.lineTo(W-pad.r,pad.t); ctx.stroke();
    ctx.setLineDash([]);
    ctx.fillStyle='rgba(139,92,246,0.5)'; ctx.font='9px Inter,sans-serif'; ctx.textAlign='right';
    ctx.fillText('Perfect Equality', W-pad.r-3, pad.t+13);
    // Lorenz points
    var pts=[{x:pad.l,y:H-pad.b}]; var cum=0;
    bals.forEach(function(b,i){
      cum+=b; pts.push({x:pad.l+cW*(i+1)/n, y:(H-pad.b)-cH*(cum/total)});
    });
    // inequality fill
    var fg=ctx.createLinearGradient(0,pad.t,0,H-pad.b);
    fg.addColorStop(0,'rgba(200,168,76,0.2)'); fg.addColorStop(1,'rgba(200,168,76,0.04)');
    ctx.beginPath(); ctx.moveTo(pts[0].x,pts[0].y);
    pts.forEach(function(p){ctx.lineTo(p.x,p.y);});
    ctx.lineTo(W-pad.r,H-pad.b); ctx.closePath(); ctx.fillStyle=fg; ctx.fill();
    // Lorenz line with glow
    ctx.save(); ctx.shadowColor='rgba(200,168,76,0.6)'; ctx.shadowBlur=12;
    ctx.strokeStyle='#C9A84C'; ctx.lineWidth=2.5;
    ctx.beginPath(); ctx.moveTo(pts[0].x,pts[0].y);
    pts.forEach(function(p){ctx.lineTo(p.x,p.y);}); ctx.stroke(); ctx.restore();
    // endpoint dot
    var ep=pts[pts.length-1];
    ctx.save(); ctx.shadowColor='rgba(200,168,76,0.9)'; ctx.shadowBlur=14;
    ctx.beginPath(); ctx.arc(ep.x,ep.y,5,0,2*Math.PI); ctx.fillStyle='#C9A84C'; ctx.fill(); ctx.restore();
    ctx.beginPath(); ctx.arc(ep.x,ep.y,2.5,0,2*Math.PI); ctx.fillStyle='#fff'; ctx.fill();
    // Gini annotation (trapezoidal)
    var lorenzArea=0; cum=0;
    bals.forEach(function(b){cum+=b; lorenzArea+=(cum/total)*(1/n);});
    var gini=1-2*lorenzArea;
    ctx.fillStyle='rgba(200,168,76,0.95)'; ctx.font='bold 12px JetBrains Mono,monospace'; ctx.textAlign='left';
    ctx.fillText('Gini: '+(gini*100).toFixed(1), pad.l+8, pad.t+24);
    // title
    ctx.fillStyle='rgba(200,168,76,0.35)'; ctx.font='10px Inter,sans-serif';
    ctx.fillText('LORENZ CURVE  —  WEALTH DISTRIBUTION', pad.l, 20);
  } catch(e) {}
}"""
content = require_replace(OLD, NEW, "drawLorenzCurve"); print("drawLorenzCurve replaced")

# ─────────────────────────────────────────────────────────────
# 8. drawWcapSlideChart — rounded bars + gradients + better labels
# ─────────────────────────────────────────────────────────────
OLD = """function drawWcapSlideChart() {
  const canvas = document.getElementById('wcap-slide-chart');
  if (!canvas || !canvas.offsetParent) return;
  canvas.width = canvas.offsetWidth;
  const ctx = canvas.getContext('2d');
  const W = canvas.width, H = canvas.height;
  ctx.clearRect(0,0,W,H);
  const pad = {l:36,r:16,t:8,b:24};
  const maxN = 28;
  const bw = (W-pad.l-pad.r)/maxN;
  for (let n=1; n<=maxN; n++) {
    const mult = Math.max(5,Math.min(n,25));
    const bh = (H-pad.t-pad.b)*(mult/25);
    const x = pad.l+(n-1)*bw, y = H-pad.b-bh;
    ctx.fillStyle = n<=25 ? (n===25?'rgba(0,255,209,0.55)':'rgba(200,168,76,0.55)') : 'rgba(255,255,255,0.1)';
    ctx.fillRect(x+1,y,bw-2,bh);
    if (n===1||n===5||n===10||n===15||n===20||n===25) {
      ctx.fillStyle='rgba(255,255,255,0.5)'; ctx.font='8px monospace'; ctx.textAlign='center';
      ctx.fillText(mult+'×',x+bw/2,y-1);
      ctx.fillStyle='rgba(255,255,255,0.3)';
      ctx.fillText('N='+n,x+bw/2,H-2);
    }
  }
  ctx.strokeStyle='rgba(0,255,209,0.3)'; ctx.lineWidth=1; ctx.setLineDash([3,3]);
  const lockY = H-pad.b-(H-pad.t-pad.b);
  ctx.beginPath(); ctx.moveTo(pad.l+(25-1)*bw,lockY); ctx.lineTo(W-pad.r,lockY); ctx.stroke();
  ctx.setLineDash([]);
}"""
NEW = """function drawWcapSlideChart() {
  const canvas = document.getElementById('wcap-slide-chart');
  if (!canvas || !canvas.offsetParent) return;
  canvas.width = canvas.offsetWidth;
  const ctx = canvas.getContext('2d');
  const W = canvas.width, H = canvas.height;
  ctx.clearRect(0,0,W,H);
  const pad = {l:44,r:20,t:36,b:32};
  const cW = W-pad.l-pad.r, cH = H-pad.t-pad.b;
  const maxN = 28;
  const bw = cW/maxN;
  // horizontal reference lines
  [5,10,15,20,25].forEach(function(v){
    var y=H-pad.b-cH*(v/25);
    ctx.strokeStyle=v===25?'rgba(0,255,209,0.2)':'rgba(139,92,246,0.08)'; ctx.lineWidth=1;
    ctx.beginPath(); ctx.moveTo(pad.l,y); ctx.lineTo(W-pad.r,y); ctx.stroke();
    ctx.fillStyle='rgba(200,168,76,0.7)'; ctx.font='10px JetBrains Mono,monospace'; ctx.textAlign='right';
    ctx.fillText(v+'x', pad.l-5, y+4);
  });
  // bars
  for (var n=1;n<=maxN;n++) {
    var mult=Math.max(5,Math.min(n,25));
    var bh=cH*(mult/25), bx=pad.l+(n-1)*bw+1, bw2=bw-2;
    var y=H-pad.b-bh, r=Math.min(3,bw2/2);
    var barGrad;
    if (n>25) { barGrad='rgba(255,255,255,0.06)'; }
    else if (n===25) { var g=ctx.createLinearGradient(0,y,0,H-pad.b); g.addColorStop(0,'rgba(0,255,209,0.8)'); g.addColorStop(1,'rgba(0,255,209,0.25)'); barGrad=g; }
    else if (n>=20) { var g2=ctx.createLinearGradient(0,y,0,H-pad.b); g2.addColorStop(0,'rgba(200,168,76,0.85)'); g2.addColorStop(1,'rgba(200,168,76,0.28)'); barGrad=g2; }
    else { var g3=ctx.createLinearGradient(0,y,0,H-pad.b); g3.addColorStop(0,'rgba(200,168,76,0.6)'); g3.addColorStop(1,'rgba(200,168,76,0.18)'); barGrad=g3; }
    // rounded top bar
    ctx.beginPath();
    ctx.moveTo(bx+r,y); ctx.lineTo(bx+bw2-r,y);
    ctx.arcTo(bx+bw2,y,bx+bw2,y+r,r);
    ctx.lineTo(bx+bw2,H-pad.b); ctx.lineTo(bx,H-pad.b); ctx.lineTo(bx,y+r);
    ctx.arcTo(bx,y,bx+r,y,r); ctx.closePath();
    if (n===25){ctx.save();ctx.shadowColor='rgba(0,255,209,0.55)';ctx.shadowBlur=8;}
    ctx.fillStyle=barGrad; ctx.fill();
    if (n===25) ctx.restore();
    // labels at key N values
    if (n===1||n===5||n===10||n===15||n===20||n===25) {
      ctx.fillStyle=n===25?'rgba(0,255,209,0.9)':'rgba(200,168,76,0.85)';
      ctx.font='bold 9px JetBrains Mono,monospace'; ctx.textAlign='center';
      ctx.fillText(mult+'x', bx+bw2/2, y-4);
      ctx.fillStyle='rgba(255,255,255,0.4)'; ctx.font='8px JetBrains Mono,monospace';
      ctx.fillText('N='+n, bx+bw2/2, H-pad.b+13);
    }
  }
  // lock line at N=25
  var lockY=H-pad.b-cH;
  ctx.save(); ctx.shadowColor='rgba(0,255,209,0.5)'; ctx.shadowBlur=5;
  ctx.strokeStyle='rgba(0,255,209,0.55)'; ctx.lineWidth=1.5; ctx.setLineDash([5,4]);
  ctx.beginPath(); ctx.moveTo(pad.l+(25-1)*bw,lockY); ctx.lineTo(W-pad.r,lockY); ctx.stroke();
  ctx.setLineDash([]); ctx.restore();
  ctx.fillStyle='rgba(0,255,209,0.8)'; ctx.font='bold 9px JetBrains Mono,monospace'; ctx.textAlign='left';
  ctx.fillText('LOCKED AT 25x', pad.l+25*bw+4, lockY-4);
  // title
  ctx.fillStyle='rgba(200,168,76,0.35)'; ctx.font='10px Inter,sans-serif'; ctx.textAlign='left';
  ctx.fillText('WEALTH CAP  —  BOOTSTRAP MULTIPLIER  ·  max(5, min(N, 25))×', pad.l, 20);
}"""
content = require_replace(OLD, NEW, "drawWcapSlideChart"); print("drawWcapSlideChart replaced")

# ─────────────────────────────────────────────────────────────
# 9. drawPriceChart — bezier + glow + better labels
# ─────────────────────────────────────────────────────────────
OLD = """function drawPriceChart() {
  const canvas = document.getElementById('price-chart');
  if (!canvas || !priceHistory.length) return;
  canvas.width = canvas.offsetWidth;
  const ctx = canvas.getContext('2d');
  const W = canvas.width, H = canvas.height;
  ctx.clearRect(0, 0, W, H);
  const pad = {l:52, r:16, t:14, b:24};
  const pts = priceHistory;
  const prices = pts.map(p => p.p);
  const minP = Math.min(...prices), maxP = Math.max(...prices);
  const range = maxP - minP || minP * 0.01 || 0.0001;
  const toX = i => pad.l + (W - pad.l - pad.r) * i / Math.max(pts.length - 1, 1);
  const toY = p => pad.t + (H - pad.t - pad.b) * (1 - (p - minP) / range);
  // Grid
  ctx.strokeStyle = 'rgba(255,255,255,0.05)'; ctx.lineWidth = 1;
  for (let i = 0; i <= 4; i++) {
    const y = pad.t + (H - pad.t - pad.b) * i / 4;
    ctx.beginPath(); ctx.moveTo(pad.l, y); ctx.lineTo(W - pad.r, y); ctx.stroke();
    const v = maxP - (range * i / 4);
    ctx.fillStyle = 'rgba(255,255,255,0.3)'; ctx.font = '9px monospace'; ctx.textAlign = 'right';
    ctx.fillText(v.toFixed(4), pad.l - 3, y + 3);
  }
  // Fill under line
  ctx.beginPath();
  pts.forEach((p, i) => { const x = toX(i), y = toY(p.p); i === 0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y); });
  ctx.lineTo(toX(pts.length - 1), H - pad.b);
  ctx.lineTo(toX(0), H - pad.b);
  ctx.closePath();
  const grad = ctx.createLinearGradient(0, pad.t, 0, H - pad.b);
  grad.addColorStop(0, 'rgba(139,92,246,0.3)'); grad.addColorStop(1, 'rgba(139,92,246,0.02)');
  ctx.fillStyle = grad; ctx.fill();
  // Line
  ctx.beginPath(); ctx.strokeStyle = '#8B5CF6'; ctx.lineWidth = 2;
  pts.forEach((p, i) => { const x = toX(i), y = toY(p.p); i === 0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y); });
  ctx.stroke();
  // Last price dot
  const lx = toX(pts.length - 1), ly = toY(prices[prices.length - 1]);
  ctx.beginPath(); ctx.arc(lx, ly, 4, 0, 2 * Math.PI);
  ctx.fillStyle = '#8B5CF6'; ctx.fill();
  ctx.fillStyle = 'rgba(139,92,246,0.9)'; ctx.font = 'bold 10px monospace'; ctx.textAlign = 'left';
  ctx.fillText(prices[prices.length - 1].toFixed(4) + ' tUSD', lx + 7, ly + 4);
  // X-axis: time labels
  ctx.fillStyle = 'rgba(255,255,255,0.25)'; ctx.font = '8px monospace'; ctx.textAlign = 'center';
  [0, Math.floor(pts.length / 2), pts.length - 1].forEach(i => {
    if (i < 0 || i >= pts.length) return;
    const d = new Date(pts[i].t);
    ctx.fillText(d.getHours().toString().padStart(2,'0') + ':' + d.getMinutes().toString().padStart(2,'0') + ':' + d.getSeconds().toString().padStart(2,'0'), toX(i), H - 6);
  });
}"""
NEW = """function drawPriceChart() {
  const canvas = document.getElementById('price-chart');
  if (!canvas || !priceHistory.length) return;
  canvas.width = canvas.offsetWidth;
  const ctx = canvas.getContext('2d');
  const W = canvas.width, H = canvas.height;
  ctx.clearRect(0, 0, W, H);
  const pad = {l:58, r:24, t:36, b:36};
  const pts = priceHistory;
  const prices = pts.map(function(p){return p.p;});
  const minP = Math.min.apply(null,prices), maxP = Math.max.apply(null,prices);
  const range = maxP - minP || minP * 0.01 || 0.0001;
  const padR = range * 0.1;
  const lo = minP - padR, hi = maxP + padR;
  const cW = W-pad.l-pad.r, cH = H-pad.t-pad.b;
  const toX = function(i){return pad.l + cW * i / Math.max(pts.length - 1, 1);};
  const toY = function(p){return pad.t + cH * (1 - (p - lo) / (hi - lo));};
  // grid
  for (var gi=0;gi<=4;gi++) {
    var gy = pad.t + cH*gi/4;
    ctx.strokeStyle = gi===4?'rgba(139,92,246,0.2)':'rgba(139,92,246,0.08)'; ctx.lineWidth=1;
    ctx.beginPath(); ctx.moveTo(pad.l,gy); ctx.lineTo(W-pad.r,gy); ctx.stroke();
    var gv = hi - (hi-lo)*gi/4;
    ctx.fillStyle='rgba(139,92,246,0.75)'; ctx.font='10px JetBrains Mono,monospace'; ctx.textAlign='right';
    ctx.fillText(gv.toFixed(4), pad.l-5, gy+4);
  }
  // bezier fill
  ctx.beginPath(); ctx.moveTo(toX(0),toY(pts[0].p));
  for (var bi=1;bi<pts.length-1;bi++) {
    var mx=(toX(bi)+toX(bi+1))/2, my=(toY(pts[bi].p)+toY(pts[bi+1].p))/2;
    ctx.quadraticCurveTo(toX(bi),toY(pts[bi].p),mx,my);
  }
  if (pts.length>1) ctx.lineTo(toX(pts.length-1),toY(pts[pts.length-1].p));
  ctx.lineTo(toX(pts.length-1),H-pad.b); ctx.lineTo(toX(0),H-pad.b); ctx.closePath();
  var grad=ctx.createLinearGradient(0,pad.t,0,H-pad.b);
  grad.addColorStop(0,'rgba(139,92,246,0.38)'); grad.addColorStop(0.65,'rgba(139,92,246,0.1)'); grad.addColorStop(1,'rgba(139,92,246,0.01)');
  ctx.fillStyle=grad; ctx.fill();
  // glowing bezier line
  ctx.save(); ctx.shadowColor='rgba(139,92,246,0.7)'; ctx.shadowBlur=12;
  ctx.strokeStyle='#8B5CF6'; ctx.lineWidth=2.5;
  ctx.beginPath(); ctx.moveTo(toX(0),toY(pts[0].p));
  for (var li=1;li<pts.length-1;li++) {
    var mx2=(toX(li)+toX(li+1))/2, my2=(toY(pts[li].p)+toY(pts[li+1].p))/2;
    ctx.quadraticCurveTo(toX(li),toY(pts[li].p),mx2,my2);
  }
  if (pts.length>1) ctx.lineTo(toX(pts.length-1),toY(pts[pts.length-1].p));
  ctx.stroke(); ctx.restore();
  // last price dot
  var lx=toX(pts.length-1), ly=toY(prices[prices.length-1]);
  ctx.save(); ctx.shadowColor='rgba(139,92,246,0.9)'; ctx.shadowBlur=16;
  ctx.beginPath(); ctx.arc(lx,ly,5,0,2*Math.PI); ctx.fillStyle='#8B5CF6'; ctx.fill(); ctx.restore();
  ctx.beginPath(); ctx.arc(lx,ly,2.5,0,2*Math.PI); ctx.fillStyle='#fff'; ctx.fill();
  var pLabel=prices[prices.length-1].toFixed(6)+' tUSD';
  ctx.fillStyle='rgba(139,92,246,0.95)'; ctx.font='bold 11px JetBrains Mono,monospace';
  ctx.textAlign = lx>W*0.75?'right':'left';
  ctx.fillText(pLabel, lx+(lx>W*0.75?-8:8), ly-9);
  // x-axis time labels
  ctx.fillStyle='rgba(139,92,246,0.5)'; ctx.font='9px JetBrains Mono,monospace'; ctx.textAlign='center';
  [0, Math.floor(pts.length/2), pts.length-1].forEach(function(i){
    if (i<0||i>=pts.length) return;
    var dd=new Date(pts[i].t);
    var ts=dd.getHours().toString().padStart(2,'0')+':'+dd.getMinutes().toString().padStart(2,'0')+':'+dd.getSeconds().toString().padStart(2,'0');
    ctx.fillText(ts, toX(i), H-pad.b+16);
  });
  // title
  ctx.fillStyle='rgba(139,92,246,0.38)'; ctx.font='10px Inter,sans-serif'; ctx.textAlign='left';
  ctx.fillText('AEQ / tUSD  —  LIVE PRICE  (x·y = k  AMM)', pad.l, 20);
}"""
content = require_replace(OLD, NEW, "drawPriceChart"); print("drawPriceChart replaced")

# ─────────────────────────────────────────────────────────────
# Write both files
# ─────────────────────────────────────────────────────────────
with open(PATH_CHAIN, 'w', encoding='utf-8') as f:
    f.write(content)
print("Written:", PATH_CHAIN)

with open(PATH_WORK, 'w', encoding='utf-8') as f:
    f.write(content)
print("Written:", PATH_WORK)

print("\nAll done!")
