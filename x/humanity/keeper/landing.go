package keeper

const landingHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="google" content="notranslate">
<title>Aequitas — Proof of Humanity Chain</title>
<meta name="description" content="The world's first currency where every verified human receives equal money. The network measures and publishes its own inequality — live, on chain.">
<meta name="theme-color" content="#0C0E16">
<link rel="preconnect" href="https://fonts.bunny.net">
<link href="https://fonts.bunny.net/css?family=inter:300,400,500,600,700,800,900|dm-serif-display:400&display=swap" rel="stylesheet">
<style>
*{box-sizing:border-box;margin:0;padding:0}
:root{
  --bg:#0C0E16;--card:#131620;--card2:#1A1D2B;
  --purple:#9B72F6;--teal:#22D3EE;--gold:#F0B429;--green:#34D399;
  --text:#E8EDF5;--muted:#8892A4;--border:rgba(255,255,255,0.07);
  --radius:12px;--grad:linear-gradient(135deg,#9B72F6,#22D3EE);
}
html{scroll-behavior:smooth}
body{background:var(--bg);color:var(--text);font-family:'Inter',-apple-system,sans-serif;line-height:1.6;overflow-x:hidden}

/* ── NAV ─────────────────────────────────────────────────────── */
nav{position:fixed;top:0;left:0;right:0;z-index:100;background:rgba(12,14,22,0.92);backdrop-filter:blur(12px);border-bottom:1px solid var(--border);padding:0 24px;height:60px;display:flex;align-items:center;justify-content:space-between}
.nav-logo{display:flex;align-items:center;gap:10px;text-decoration:none}
.nav-logo img,.nav-icon{width:32px;height:32px;background:var(--grad);border-radius:8px;display:flex;align-items:center;justify-content:center;font-size:1rem}
.nav-brand{font-weight:800;font-size:0.85rem;letter-spacing:1.5px;color:var(--text)}
.nav-sub{font-size:0.52rem;color:var(--muted);letter-spacing:1px;line-height:1}
.nav-links{display:flex;align-items:center;gap:8px}
.nav-link{color:var(--muted);text-decoration:none;font-size:0.75rem;font-weight:500;padding:6px 12px;border-radius:6px;transition:all 0.2s}
.nav-link:hover{color:var(--text);background:rgba(255,255,255,0.06)}
.nav-cta{background:var(--grad);color:#fff;padding:8px 18px;border-radius:20px;font-size:0.75rem;font-weight:700;text-decoration:none;transition:opacity 0.2s}
.nav-cta:hover{opacity:0.85}
@media(max-width:600px){.nav-links{display:none}}

/* ── HERO ────────────────────────────────────────────────────── */
.hero{min-height:100vh;display:flex;flex-direction:column;align-items:center;justify-content:center;text-align:center;padding:100px 24px 60px;position:relative;overflow:hidden}
.hero::before{content:'';position:absolute;inset:0;background:radial-gradient(ellipse 80% 50% at 50% 0%,rgba(155,114,246,0.12) 0%,transparent 60%),radial-gradient(ellipse 60% 40% at 80% 100%,rgba(34,211,238,0.06) 0%,transparent 60%);pointer-events:none}
.hero-badge{display:inline-flex;align-items:center;gap:8px;background:rgba(155,114,246,0.12);border:1px solid rgba(155,114,246,0.25);border-radius:20px;padding:6px 16px;font-size:0.72rem;color:var(--purple);font-weight:600;letter-spacing:0.5px;margin-bottom:28px}
.pulse{width:7px;height:7px;border-radius:50%;background:var(--green);animation:pulse 2s infinite}
@keyframes pulse{0%,100%{opacity:1;transform:scale(1)}50%{opacity:0.5;transform:scale(0.85)}}
h1{font-family:'DM Serif Display',serif;font-size:clamp(2.2rem,6vw,4rem);line-height:1.15;font-weight:400;max-width:700px;margin-bottom:20px}
h1 span{background:var(--grad);-webkit-background-clip:text;-webkit-text-fill-color:transparent}
.hero-sub{font-size:clamp(0.95rem,2.5vw,1.15rem);color:var(--muted);max-width:540px;margin-bottom:40px;font-weight:400}
.hero-btns{display:flex;flex-wrap:wrap;gap:14px;justify-content:center;margin-bottom:60px}
.btn-primary{display:inline-flex;align-items:center;gap:8px;background:var(--grad);color:#fff;padding:14px 28px;border-radius:10px;font-size:0.85rem;font-weight:700;text-decoration:none;transition:opacity 0.2s;letter-spacing:0.3px}
.btn-primary:hover{opacity:0.88}
.btn-secondary{display:inline-flex;align-items:center;gap:8px;background:rgba(255,255,255,0.05);border:1px solid var(--border);color:var(--text);padding:14px 28px;border-radius:10px;font-size:0.85rem;font-weight:600;text-decoration:none;transition:all 0.2s}
.btn-secondary:hover{background:rgba(255,255,255,0.09);border-color:rgba(155,114,246,0.4)}
.hero-proof{font-size:0.72rem;color:var(--muted);display:flex;align-items:center;gap:8px}
.hero-proof span{color:var(--green)}

/* ── STATS BAR ───────────────────────────────────────────────── */
.stats-bar{background:var(--card);border-top:1px solid var(--border);border-bottom:1px solid var(--border);padding:28px 24px;display:flex;justify-content:center;gap:0}
.stat-item{text-align:center;padding:0 32px;border-right:1px solid var(--border);flex:1;max-width:200px}
.stat-item:last-child{border-right:none}
.stat-num{font-size:clamp(1.4rem,3vw,2rem);font-weight:800;font-family:'DM Serif Display',serif}
.stat-lbl{font-size:0.68rem;color:var(--muted);text-transform:uppercase;letter-spacing:1px;margin-top:4px}
@media(max-width:700px){.stats-bar{flex-wrap:wrap;gap:1px;background:var(--border)}.stat-item{flex:calc(50% - 1px);border-right:none;background:var(--card);padding:20px 16px;max-width:none}}
@media(max-width:380px){.stat-item{flex:100%}}

/* ── SECTION ─────────────────────────────────────────────────── */
section{padding:80px 24px}
.section-inner{max-width:1100px;margin:0 auto}
.section-label{font-size:0.65rem;color:var(--purple);letter-spacing:3px;text-transform:uppercase;font-weight:600;margin-bottom:12px}

/* ── LORENZ CURVE ────────────────────────────────────────────── */
.lorenz-wrap{margin-top:40px;background:var(--card2);border:1px solid var(--border);border-radius:var(--radius);padding:24px}
.lorenz-head{display:flex;justify-content:space-between;align-items:flex-start;gap:20px;flex-wrap:wrap;margin-bottom:8px}
.lorenz-title{font-size:0.9rem;font-weight:700;color:var(--text)}
.lorenz-sub{font-size:0.72rem;color:var(--muted);line-height:1.7;max-width:560px;margin-top:4px}
.lorenz-legend{display:flex;flex-direction:column;gap:6px;font-size:0.68rem;color:var(--muted);white-space:nowrap}
.lorenz-legend span{display:flex;align-items:center;gap:7px}
.lorenz-legend i{width:14px;height:3px;border-radius:2px;display:inline-block}
#lorenz{width:100%;max-width:420px;height:auto;display:block;margin:8px auto 0}
.lorenz-note{font-size:0.7rem;color:var(--muted);text-align:center;margin-top:6px}

/* ── MECHANISMS ──────────────────────────────────────────────── */
.mech-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:20px;margin-top:36px;text-align:left}
.mech-card{background:var(--card2);border:1px solid var(--border);border-radius:var(--radius);padding:26px;display:flex;flex-direction:column}
.mech-icon{width:38px;height:38px;border-radius:10px;display:flex;align-items:center;justify-content:center;font-size:1.05rem;margin-bottom:14px}
.mech-card h3{font-size:1.02rem;font-weight:700;margin-bottom:8px;color:var(--text)}
.mech-lead{font-size:0.8rem;color:var(--muted);line-height:1.8}
.mech-detail{margin-top:16px;padding-top:16px;border-top:1px solid var(--border)}
.mech-detail-label{font-size:0.6rem;letter-spacing:1.6px;text-transform:uppercase;color:var(--purple);font-weight:700;margin-bottom:10px}
.mech-detail ul{list-style:none;display:flex;flex-direction:column;gap:7px}
.mech-detail li{font-size:0.76rem;color:var(--muted);line-height:1.65;padding-left:16px;position:relative}
.mech-detail li::before{content:'';position:absolute;left:0;top:8px;width:5px;height:5px;border-radius:50%;background:var(--purple);opacity:0.6}
.mech-detail li strong{color:var(--text);font-weight:600}
.mech-formula{display:block;background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:11px 13px;font-family:'JetBrains Mono',ui-monospace,monospace;font-size:0.74rem;color:var(--teal);overflow-x:auto;white-space:nowrap}
.mech-detail-note{font-size:0.75rem;color:var(--muted);line-height:1.75;margin-top:10px}
.mech-detail-note strong{color:var(--text)}
.mech-live{margin-top:auto;padding-top:16px;font-size:0.72rem;color:var(--muted)}
.mech-live strong{color:var(--gold);font-weight:700}

/* ── ARCHITECTURE ────────────────────────────────────────────── */
.arch-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(290px,1fr));gap:22px;margin-top:36px;text-align:left}
.arch-col{background:var(--card2);border:1px solid var(--border);border-radius:var(--radius);padding:22px}
.arch-col-title{font-size:0.62rem;letter-spacing:1.8px;text-transform:uppercase;color:var(--teal);font-weight:700;margin-bottom:14px}
.arch-list{display:flex;flex-direction:column;gap:12px}
.arch-list dt{font-size:0.72rem;font-weight:700;color:var(--text)}
.arch-list dd{font-size:0.74rem;color:var(--muted);line-height:1.65;margin-top:2px}
.arch-note{margin-top:26px;background:rgba(155,114,246,0.07);border:1px solid rgba(155,114,246,0.22);border-radius:var(--radius);padding:20px 22px;font-size:0.8rem;color:var(--muted);line-height:1.8;text-align:left}
.arch-note strong{color:var(--text);display:block;margin-bottom:4px}
h2{font-family:'DM Serif Display',serif;font-size:clamp(1.8rem,4vw,2.8rem);line-height:1.2;font-weight:400;margin-bottom:16px}
.section-sub{font-size:1rem;color:var(--muted);max-width:560px;margin-bottom:48px;line-height:1.7}

/* ── HOW IT WORKS ────────────────────────────────────────────── */
.steps{display:grid;grid-template-columns:repeat(3,1fr);gap:24px}
.step{background:var(--card);border:1px solid var(--border);border-radius:var(--radius);padding:28px;position:relative;transition:border-color 0.2s}
.step:hover{border-color:rgba(155,114,246,0.3)}
.step-num{width:40px;height:40px;border-radius:50%;background:var(--grad);display:flex;align-items:center;justify-content:center;font-weight:800;font-size:1rem;margin-bottom:16px;color:#fff}
.step h3{font-size:1rem;font-weight:700;margin-bottom:8px}
.step p{font-size:0.85rem;color:var(--muted);line-height:1.7}
@media(max-width:700px){.steps{grid-template-columns:1fr}}

/* ── WHY SECTION ─────────────────────────────────────────────── */
.why-grid{display:grid;grid-template-columns:1fr 1fr;gap:24px}
.why-card{background:var(--card);border:1px solid var(--border);border-radius:var(--radius);padding:28px}
.why-card .icon{font-size:2rem;margin-bottom:14px}
.why-card h3{font-size:1rem;font-weight:700;margin-bottom:8px}
.why-card p{font-size:0.85rem;color:var(--muted);line-height:1.7}
.why-highlight{background:linear-gradient(135deg,rgba(155,114,246,0.1),rgba(34,211,238,0.06));border-color:rgba(155,114,246,0.25)}
@media(max-width:700px){.why-grid{grid-template-columns:1fr}}

/* ── TOKENOMICS ──────────────────────────────────────────────── */
.token-grid{display:grid;grid-template-columns:repeat(2,1fr);gap:16px}
.token-card{background:var(--card2);border:1px solid var(--border);border-radius:var(--radius);padding:20px}
.token-pct{font-size:1.6rem;font-weight:800;font-family:'DM Serif Display',serif;margin-bottom:4px}
.token-name{font-size:0.78rem;font-weight:700;margin-bottom:6px}
.token-desc{font-size:0.78rem;color:var(--muted);line-height:1.6}
@media(max-width:500px){.token-grid{grid-template-columns:1fr}}

/* ── GINI COMPARISON ─────────────────────────────────────────── */
.gini-row{display:flex;align-items:center;gap:12px;margin-bottom:10px}
.gini-label{font-size:0.82rem;min-width:110px;color:var(--muted)}
.gini-bar-wrap{flex:1;height:8px;background:rgba(255,255,255,0.06);border-radius:4px;overflow:hidden}
.gini-bar{height:100%;border-radius:4px}
.gini-val{font-size:0.78rem;font-weight:700;min-width:40px;text-align:right}
.gini-row.aeq .gini-label{color:var(--gold);font-weight:700}
.gini-row.aeq .gini-bar{background:var(--gold)}

/* ── CTA ─────────────────────────────────────────────────────── */
.cta-section{background:linear-gradient(135deg,rgba(155,114,246,0.12),rgba(34,211,238,0.06));border:1px solid rgba(155,114,246,0.2);border-radius:20px;padding:60px 40px;text-align:center;margin:0 24px}
.cta-section h2{max-width:500px;margin:0 auto 16px}
.cta-section p{color:var(--muted);margin-bottom:36px;font-size:0.95rem}
@media(max-width:600px){.cta-section{padding:40px 24px;border-radius:14px}}

/* ── FOOTER ──────────────────────────────────────────────────── */
footer{border-top:1px solid var(--border);padding:40px 24px;text-align:center}
.footer-links{display:flex;flex-wrap:wrap;justify-content:center;gap:24px;margin-bottom:20px}
.footer-links a{color:var(--muted);text-decoration:none;font-size:0.8rem;transition:color 0.2s}
.footer-links a:hover{color:var(--text)}
/* Social links carry their brand mark. Inline SVG, not an <img>: the page's
   own CSP allows img-src 'self' data: only, and an inline path needs no
   request at all — it also inherits currentColor, so the icon fades with the
   label on hover instead of staying a fixed colour. */
.social{display:inline-flex;align-items:center;gap:7px}
.social svg{width:15px;height:15px;flex:none;fill:currentColor}
.social-row{display:flex;flex-wrap:wrap;gap:12px;justify-content:center;margin-top:14px}

/* ── SOCIAL SECTION ──────────────────────────────────────────── */
/* Its own section rather than another footer link: the marks were only in
   the footer and under the CTA, which is where a first-time visitor never
   looks. Big enough to be the thing you see, and the whole card is the
   link — not just the wordmark. */
.social-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:20px;margin-top:36px;max-width:760px;margin-left:auto;margin-right:auto}
.social-card{background:var(--card2);border:1px solid var(--border);border-radius:var(--radius);padding:38px 28px;display:flex;flex-direction:column;align-items:center;text-align:center;gap:12px;text-decoration:none;transition:transform 0.2s,border-color 0.2s,background 0.2s}
.social-card:hover{border-color:rgba(155,114,246,0.45);background:var(--card);transform:translateY(-3px)}
.social-card svg{width:64px;height:64px;fill:var(--text);transition:fill 0.2s}
.social-card:hover svg{fill:var(--purple)}
.social-name{font-size:1.15rem;font-weight:700;color:var(--text);line-height:1.2}
.social-handle{font-size:0.85rem;font-weight:600;color:var(--purple)}
.social-desc{font-size:0.78rem;color:var(--muted);line-height:1.7;max-width:260px}
@media(max-width:600px){.social-card{padding:30px 22px}.social-card svg{width:52px;height:52px}}
.social-btn{background:rgba(255,255,255,0.05);border:1px solid var(--border);color:var(--text);padding:9px 18px;border-radius:999px;font-size:0.8rem;font-weight:600;text-decoration:none;transition:all 0.2s}
.social-btn:hover{background:rgba(255,255,255,0.09);border-color:rgba(155,114,246,0.4)}
.social-btn svg{width:16px;height:16px}
footer p{font-size:0.75rem;color:var(--muted)}
footer p span{color:var(--purple)}

/* ── GINI SECTION ────────────────────────────────────────────── */
@media(max-width:700px){.gini-section-grid{grid-template-columns:1fr!important}}

/* ── MOBILE TOUCH TARGETS ────────────────────────────────────── */
@media(max-width:480px){
.btn-primary,.btn-secondary{padding:16px 24px;font-size:0.9rem;width:100%;justify-content:center;border-radius:12px}
.hero-btns{flex-direction:column;width:100%;max-width:320px}
h1{font-size:2rem}
.hero{padding:90px 20px 50px}
section{padding:60px 20px}
}
</style>
</head>
<body>

<!-- NAV -->
<nav>
  <a href="/" class="nav-logo">
    <div class="nav-icon">⚖</div>
    <div>
      <div class="nav-brand">AEQUITAS</div>
      <div class="nav-sub">PROOF OF HUMANITY</div>
    </div>
  </a>
  <div class="nav-links">
    <a href="#mechanisms" class="nav-link">Mechanisms</a>
    <a href="#architecture" class="nav-link">Architecture</a>
    <a href="/explorer" class="nav-link">Explorer</a>
    <a href="/index/score" class="nav-link">Equality</a>
    <a href="/network" class="nav-link">Network</a>
    <a href="/exchange" class="nav-link">Exchange</a>
    <a href="#social" class="nav-link">Social</a>
    <a href="/register" class="nav-cta">Register →</a>
  </div>
</nav>

<!-- HERO -->
<section class="hero">
  <div class="hero-badge">
    <span class="pulse"></span>
    LIVE ON CHAIN ID 1926
  </div>
  <h1>Money that belongs<br>to <span>every human</span> equally</h1>
  <p class="hero-sub">Aequitas is the first blockchain where the money supply is mathematically tied to verified human existence. Every person receives 1,000 AEQ — no mining, no investment, no early advantage.</p>
  <div class="hero-btns">
    <a href="/download/app.apk" class="btn-primary">📱 Download Aequitas App</a>
    <a href="/register" class="btn-secondary">🌐 Open Explorer</a>
  </div>
  <div class="hero-proof">
    <span>✓</span> Gini <span id="gini-inline">—</span> — measured on chain, published live &nbsp;·&nbsp;
    <span>✓</span> Zero gas fees &nbsp;·&nbsp;
    <span>✓</span> Open source
  </div>
</section>

<!-- LIVE STATS -->
<div class="stats-bar">
  <div class="stat-item">
    <div class="stat-num" id="stat-humans" style="color:#34D399">—</div>
    <div class="stat-lbl">Verified Humans</div>
  </div>
  <div class="stat-item">
    <div class="stat-num" id="stat-supply" style="color:#9B72F6">—</div>
    <div class="stat-lbl">AEQ in Circulation</div>
  </div>
  <div class="stat-item">
    <div class="stat-num" id="stat-gini" style="color:#F0B429">—</div>
    <div class="stat-lbl">Gini Coefficient</div>
  </div>
  <div class="stat-item">
    <div class="stat-num" id="stat-blocks" style="color:#22D3EE">—</div>
    <div class="stat-lbl">Blocks Produced</div>
  </div>
</div>

<!-- HOW IT WORKS -->
<section>
  <div class="section-inner">
    <div class="section-label">How it works</div>
    <h2>Three steps to financial inclusion</h2>
    <p class="section-sub">No bank account, no crypto background, no investment required. Just a smartphone with a fingerprint sensor.</p>
    <div class="steps">
      <div class="step">
        <div class="step-num">1</div>
        <h3>Biometric Capture</h3>
        <p>Your device captures the biometric signals and reduces them to a one-way hash. The <strong>raw images are never stored</strong> — neither on your phone nor on the network. Only the derived hash is used.</p>
      </div>
      <div class="step">
        <div class="step-num">2</div>
        <h3>Zero-Knowledge Proof</h3>
        <p>A Groth16 proof (BN128) is generated against that hash. It proves you are a <strong>unique, not-yet-registered human</strong> — the chain learns that fact and nothing else about you.</p>
      </div>
      <div class="step">
        <div class="step-num">3</div>
        <h3>1,000 AEQ Granted</h3>
        <p>Your wallet is permanently registered on-chain within 1 second. You receive 1,000 AEQ instantly — completely free, forever immutable.</p>
      </div>
    </div>
  </div>
</section>

<!-- WHY AEQUITAS -->
<section style="background:var(--card);border-top:1px solid var(--border);border-bottom:1px solid var(--border)">
  <div class="section-inner">
    <div class="section-label">Why Aequitas</div>
    <h2>Bitcoin's Gini is 0.85 — higher than any country</h2>
    <p class="section-sub">The cryptocurrency that was supposed to democratize finance created the most extreme wealth concentration in history. Aequitas was designed from scratch to be different.</p>
    <div class="why-grid">
      <div class="why-card why-highlight">
        <div class="icon">⚖️</div>
        <h3>Radical Equality by Design</h3>
        <p>Total supply = verified humans × 1,000 AEQ. No pre-mine, no founder allocation, no early-adopter advantage. The protocol enforces equality through math, not policy.</p>
      </div>
      <div class="why-card">
        <div class="icon">🔒</div>
        <h3>Privacy-First Verification</h3>
        <p>Zero-Knowledge proofs ensure one human, one wallet — without storing any biometric data. Your identity is verified, never recorded.</p>
      </div>
      <div class="why-card">
        <div class="icon">📊</div>
        <h3>Transparent Inequality Tracking</h3>
        <p>The Gini coefficient is computed on-chain after every distribution. Aequitas publishes its own inequality score — currently <span id="gini-inline" style="color:var(--gold);font-weight:700">—</span> — lower than Sweden.</p>
      </div>
      <div class="why-card">
        <div class="icon">🌍</div>
        <h3>For Everyone on Earth</h3>
        <p>No bank account, no credit card, no ID document. An Android phone is all you need. 8 billion potential participants — every one equal from day one.</p>
      </div>
    </div>
  </div>
</section>

<!-- GINI COMPARISON -->
<section>
  <div class="section-inner" style="display:grid;grid-template-columns:1fr 1fr;gap:60px;align-items:center">
    <div>
      <div class="section-label">Wealth Equality</div>
      <h2>The fairest currency ever created</h2>
      <p style="color:var(--muted);font-size:0.9rem;line-height:1.8">Lower Gini = more equality. Aequitas's target is below 0.30 — comparable to Scandinavia. Today we are already far below.</p>
    </div>
    <div>
      <div class="gini-row aeq">
        <span class="gini-label">Aequitas</span>
        <div class="gini-bar-wrap"><div class="gini-bar" id="bar-aeq" style="width:9%;background:var(--gold)"></div></div>
        <span class="gini-val" id="val-aeq" style="color:var(--gold)">—</span>
      </div>
      <div class="gini-row">
        <span class="gini-label">Scandinavia</span>
        <div class="gini-bar-wrap"><div class="gini-bar" style="width:27%;background:#60A5FA"></div></div>
        <span class="gini-val" style="color:#60A5FA">0.27</span>
      </div>
      <div class="gini-row">
        <span class="gini-label">Germany</span>
        <div class="gini-bar-wrap"><div class="gini-bar" style="width:31%;background:#34D399"></div></div>
        <span class="gini-val" style="color:#34D399">0.31</span>
      </div>
      <div class="gini-row">
        <span class="gini-label">World avg</span>
        <div class="gini-bar-wrap"><div class="gini-bar" style="width:38%;background:#A78BFA"></div></div>
        <span class="gini-val" style="color:#A78BFA">0.38</span>
      </div>
      <div class="gini-row">
        <span class="gini-label">USA</span>
        <div class="gini-bar-wrap"><div class="gini-bar" style="width:41%;background:#FCD34D"></div></div>
        <span class="gini-val" style="color:#FCD34D">0.41</span>
      </div>
      <div class="gini-row">
        <span class="gini-label">Bitcoin</span>
        <div class="gini-bar-wrap"><div class="gini-bar" style="width:85%;background:#F87171"></div></div>
        <span class="gini-val" style="color:#F87171">~0.85</span>
      </div>
    </div>

    <!-- LORENZ CURVE — drawn from live balances, not an illustration -->
    <div class="lorenz-wrap">
      <div class="lorenz-head">
        <div>
          <div class="lorenz-title">Lorenz curve — live</div>
          <div class="lorenz-sub">Drawn from every registered wallet's actual balance. The straight line is perfect equality; the gap between the two is the Gini coefficient.</div>
        </div>
        <div class="lorenz-legend">
          <span><i style="background:var(--muted)"></i>Perfect equality</span>
          <span><i style="background:var(--gold)"></i>Aequitas today</span>
        </div>
      </div>
      <svg id="lorenz" viewBox="0 0 320 220" preserveAspectRatio="xMidYMid meet" role="img"
           aria-label="Lorenz curve of the current AEQ distribution">
        <line x1="40" y1="180" x2="300" y2="180" stroke="rgba(255,255,255,0.15)"/>
        <line x1="40" y1="20"  x2="40"  y2="180" stroke="rgba(255,255,255,0.15)"/>
        <line x1="40" y1="180" x2="300" y2="20" stroke="#8892A4" stroke-width="1.5" stroke-dasharray="4 4"/>
        <path id="lorenz-path" fill="rgba(240,180,41,0.14)" stroke="#F0B429" stroke-width="2"/>
        <text x="170" y="205" fill="#8892A4" font-size="9" text-anchor="middle">Share of humans (poorest → richest)</text>
        <text x="14" y="100" fill="#8892A4" font-size="9" text-anchor="middle" transform="rotate(-90 14 100)">Share of wealth</text>
      </svg>
      <div class="lorenz-note" id="lorenz-note">Loading distribution…</div>
    </div>
  </div>

</section>

<!-- THE FOUR MECHANISMS -->
<section id="mechanisms">
  <div class="section-inner">
    <div class="section-label">The mechanisms</div>
    <h2>Four rules keep it fair — permanently</h2>
    <p class="section-sub">Equality is not a promise on this chain, it is enforced by code that runs without a vote, an admin key, or anyone's permission. These four mechanisms all operate automatically.</p>

    <div class="mech-grid">

      <div class="mech-card">
        <div class="mech-icon" style="background:rgba(52,211,153,0.12);color:var(--green)">◈</div>
        <h3>Universal Basic Income</h3>
        <p class="mech-lead">Every verified human receives an equal share of the UBI pool every 24 hours — no application, no means test, no vote.</p>
        <div class="mech-detail">
          <div class="mech-detail-label">The pool fills from four sources</div>
          <ul>
            <li><strong>20%</strong> of every transaction fee</li>
            <li><strong>Wealth-cap overflow</strong> — redistributed the moment it occurs</li>
            <li><strong>20% of demurrage</strong> charged on hoarded balances</li>
            <li><strong>Abandoned wallets</strong> — after 2.5 years to escrow, then to the pool</li>
          </ul>
        </div>
        <div class="mech-live">Next distribution in <strong id="mech-ubi-next">—</strong> · pool holds <strong id="mech-ubi-pool">—</strong> AEQ</div>
      </div>

      <div class="mech-card">
        <div class="mech-icon" style="background:rgba(240,180,41,0.12);color:var(--gold)">▲</div>
        <h3>Wealth Cap</h3>
        <p class="mech-lead">No wallet may hold more than a fixed multiple of the average balance. Everything above that flows straight back into the pools.</p>
        <div class="mech-detail">
          <div class="mech-detail-label">The formula, in full</div>
          <code class="mech-formula">cap = max(5, min(N, 25)) × average_balance</code>
          <p class="mech-detail-note">N is the number of registered humans. The multiplier grows by one with each new person and locks permanently at <strong>25×</strong> from the 25th human onward — there is no admin key and no governance vote that can change it.</p>
        </div>
        <div class="mech-live">Currently <strong id="mech-cap">—</strong> AEQ &nbsp;·&nbsp; <span id="mech-cap-formula">—</span></div>
      </div>

      <div class="mech-card">
        <div class="mech-icon" style="background:rgba(155,114,246,0.12);color:var(--purple)">↻</div>
        <h3>Demurrage</h3>
        <p class="mech-lead">Money is a tool, not a trophy. Holding far above your fair share while doing nothing with it carries a small, steady cost.</p>
        <div class="mech-detail">
          <div class="mech-detail-label">How it is charged</div>
          <code class="mech-formula">(balance − fair_share) × 0.5% / month</code>
          <p class="mech-detail-note">Only after <strong>three months of inactivity</strong>, and only on the portion above your fair share. Nothing is ever destroyed — the fee is split across the four pools and returns to circulation.</p>
        </div>
        <div class="mech-live">Fair share today: <strong id="mech-fairshare">—</strong> AEQ per human</div>
      </div>

      <div class="mech-card">
        <div class="mech-icon" style="background:rgba(34,211,238,0.12);color:var(--teal)">⚭</div>
        <h3>Escrow &amp; Guardians</h3>
        <p class="mech-lead">A wallet that falls silent for years does not lock its value away from everyone else forever.</p>
        <div class="mech-detail">
          <div class="mech-detail-label">What happens over time</div>
          <ul>
            <li><strong>2.5 years</strong> inactive → balance moves to escrow, recoverable at any time</li>
            <li><strong>+1.5 years</strong> → escrow flows into the UBI pool for everyone</li>
            <li>A <strong>guardian</strong> you nominate can recover the wallet — after a 7-day timelock that gives you time to object</li>
          </ul>
        </div>
        <div class="mech-live">Recovery stays possible the entire time the balance sits in escrow</div>
      </div>

    </div>
  </div>
</section>

<!-- TOKENOMICS -->
<section style="background:var(--card);border-top:1px solid var(--border);border-bottom:1px solid var(--border)">
  <div class="section-inner">
    <div class="section-label">Tokenomics</div>
    <h2>Self-correcting economic mechanisms</h2>
    <p class="section-sub">Every fee is automatically redistributed. No manual intervention, no governance vote.</p>
    <div class="token-grid">
      <div class="token-card">
        <div class="token-pct" style="color:#9B72F6">40%</div>
        <div class="token-name">Validators Pool</div>
        <div class="token-desc">Node operators who secure the network earn 40% of all swap fees. Distributed daily at 20:00 Berlin time.</div>
      </div>
      <div class="token-card">
        <div class="token-pct" style="color:#22D3EE">30%</div>
        <div class="token-name">Liquidity Providers</div>
        <div class="token-desc">LP pool contributors earn 30% proportional to their share. Deeper pools = lower price impact for everyone.</div>
      </div>
      <div class="token-card">
        <div class="token-pct" style="color:#34D399">20%</div>
        <div class="token-name">UBI Pool</div>
        <div class="token-desc">20% of all fees flow into the UBI pool, split equally among all verified humans every 24 hours.</div>
      </div>
      <div class="token-card">
        <div class="token-pct" style="color:#F0B429">10%</div>
        <div class="token-name">Treasury</div>
        <div class="token-desc">10% funds protocol development, security audits, and infrastructure — fully on-chain transparent.</div>
      </div>
    </div>
  </div>
</section>

<!-- ARCHITECTURE & SECURITY -->
<section id="architecture">
  <div class="section-inner">
    <div class="section-label">Under the hood</div>
    <h2>Built to be verified, not trusted</h2>
    <p class="section-sub">Every claim on this page is checkable against a running node. The chain is open source, EVM-compatible, and every number above is served by the same API any node exposes.</p>

    <div class="arch-grid">
      <div class="arch-col">
        <div class="arch-col-title">Consensus &amp; network</div>
        <dl class="arch-list">
          <dt>Consensus</dt><dd>BlockDAG with GHOSTDAG ordering — validators produce concurrently instead of competing for one slot</dd>
          <dt>Block time</dt><dd>1 second</dd>
          <dt>Validator entry</dt><dd>No stake required. A node operator must be a registered human — that is the whole barrier</dd>
          <dt>Peer transport</dt><dd>libp2p, plus an independent HTTP sync path so a firewalled node still participates</dd>
          <dt>Finality</dt><dd>Hard checkpoints; equivocating validators are slashed automatically</dd>
        </dl>
      </div>

      <div class="arch-col">
        <div class="arch-col-title">Identity &amp; privacy</div>
        <dl class="arch-list">
          <dt>Proof system</dt><dd>Groth16 over BN128 (circom / snarkjs)</dd>
          <dt>What the chain stores</dt><dd>A nullifier and a one-way hash. No image, no template, no name, no document</dd>
          <dt>What it proves</dt><dd>That this human has not registered before — and nothing else</dd>
          <dt>Duplicate defence</dt><dd>Strength-aware multi-signal matching: a strong biometric match decides alone, weaker signals must corroborate each other</dd>
          <dt>Recovery</dt><dd>Nominated guardian with a 7-day timelock</dd>
        </dl>
      </div>

      <div class="arch-col">
        <div class="arch-col-title">Chain &amp; compatibility</div>
        <dl class="arch-list">
          <dt>Chain ID</dt><dd>1926 — add it to MetaMask like any EVM network</dd>
          <dt>EVM</dt><dd>Full go-ethereum execution; standard tooling works unchanged</dd>
          <dt>Precision</dt><dd>6 decimals (1 AEQ = 1,000,000 micro-AEQ)</dd>
          <dt>Transaction fee</dt><dd>0.1%, redistributed — never burned, never kept by an operator</dd>
          <dt>State</dt><dd>PostgreSQL per node, reconstructable from the chain alone</dd>
        </dl>
      </div>
    </div>

    <div class="arch-note">
      <strong>No pre-mine. No founder allocation. No admin key.</strong>
      Total supply equals verified humans × 1,000 AEQ — the supply cannot grow except by a human joining, and no address can be granted an exception to the wealth cap.
    </div>
  </div>
</section>

<!-- SOCIAL -->
<section id="social" style="background:var(--card);border-top:1px solid var(--border);border-bottom:1px solid var(--border)">
  <div class="section-inner">
    <div class="section-label" style="text-align:center">Social media</div>
    <h2 style="text-align:center">Where the network talks</h2>
    <p class="section-sub" style="text-align:center;margin-left:auto;margin-right:auto">Announcements, the state of the chain, and the awkward questions &mdash; in public, on both.</p>
    <div class="social-grid">
      <a class="social-card" href="https://x.com/AequitasMoney" target="_blank" rel="noopener noreferrer">
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z"/></svg>
        <div class="social-name">X</div>
        <div class="social-handle">@AequitasMoney</div>
        <div class="social-desc">Announcements, and what the chain is actually doing. Short form.</div>
      </a>
      <a class="social-card" href="https://t.me/aequitasmoney" target="_blank" rel="noopener noreferrer">
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z"/></svg>
        <div class="social-name">Telegram</div>
        <div class="social-handle">t.me/aequitasmoney</div>
        <div class="social-desc">The open group: questions, node operators, and help getting registered.</div>
      </a>
    </div>
  </div>
</section>

<!-- CTA -->
<section>
  <div class="section-inner">
    <div class="cta-section">
      <div class="section-label" style="text-align:center">Get started</div>
      <h2>Join the fairest currency on Earth</h2>
      <p>Download the Aequitas app, scan your biometrics, and receive 1,000 AEQ within 1 second. No fees, no investment, no prerequisites.</p>
      <div class="hero-btns">
        <a href="/download/app.apk" class="btn-primary">📱 Download Aequitas App (Android)</a>
        <a href="/register" class="btn-secondary">🌐 Open Explorer</a>
      </div>
      <p style="font-size:0.85rem;color:var(--muted);margin-top:4px">Questions, or want to follow the launch?</p>
      <div class="social-row">
        <a href="https://x.com/AequitasMoney" target="_blank" rel="noopener noreferrer" class="social social-btn"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z"/></svg>@AequitasMoney</a>
        <a href="https://t.me/aequitasmoney" target="_blank" rel="noopener noreferrer" class="social social-btn"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z"/></svg>Telegram</a>
      </div>
      <p style="font-size:0.75rem;color:var(--muted);margin-top:20px">Chain ID 1926 · EVM Compatible · Open Source · <a href="https://github.com/hanoi96international-gif/Aequitas" style="color:var(--purple)">View on GitHub</a></p>
    </div>
  </div>
</section>

<!-- FOOTER -->
<footer>
  <div class="footer-links">
    <a href="/register">Register</a>
    <a href="/explorer">Block Explorer</a>
    <a href="/index/score">Equality Score</a>
    <a href="/network">Network</a>
    <a href="/exchange">Exchange</a>
    <a href="/download/node-guide-en.pdf">Node Guide (EN)</a>
    <a href="/download/node-guide-de.pdf">Node Guide (DE)</a>
    <a href="https://github.com/hanoi96international-gif/Aequitas">GitHub</a>
    <a href="https://x.com/AequitasMoney" target="_blank" rel="noopener noreferrer" class="social"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z"/></svg>@AequitasMoney</a>
    <a href="https://t.me/aequitasmoney" target="_blank" rel="noopener noreferrer" class="social"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z"/></svg>Telegram</a>
  </div>
  <p>Aequitas Chain · Chain ID 1926 · <span>aequitas.digital</span> · Launched June 2026</p>
  <p style="margin-top:6px">"<em>Money exists because people exist. Nothing more, nothing less.</em>"</p>
</footer>

<script src="/landing.js"></script>
</body>
</html>`
