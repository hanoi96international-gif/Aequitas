package keeper

const landingHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="google" content="notranslate">
<title>Aequitas — Proof of Humanity Chain</title>
<meta name="description" content="The world's first currency where every verified human receives equal money. Gini coefficient 0.08 — fairer than any country on Earth.">
<meta name="theme-color" content="#0C0E16">
<!-- FIX (audit 2026-08-14): this page loaded its fonts from Google
     (fonts.googleapis.com / fonts.gstatic.com) while handleLanding's own CSP
     allows style-src/font-src only from fonts.bunny.net — so the stylesheet
     was BLOCKED by CSP and the page silently rendered in the fallback
     sans-serif, not Inter/DM Serif Display. The <link rel=preconnect> tags
     were worse than useless: preconnect is not governed by style-src, so the
     browser still opened a DNS+TLS connection to Google on every page load,
     leaking visitor IPs to a third party for a stylesheet that was then
     discarded. The explorer and /dapp were already migrated to fonts.bunny.net
     (GDPR-friendly, no request logging); this page was missed. Same families,
     same weights, now actually within the CSP. -->
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
    <a href="/explorer" class="nav-link">Explorer</a>
    <a href="/index/score" class="nav-link">Equality</a>
    <a href="/network" class="nav-link">Network</a>
    <a href="/exchange" class="nav-link">Exchange</a>
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
    <span>✓</span> Gini 0.08 — fairer than any country on Earth &nbsp;·&nbsp;
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
        <h3>Biometric Scan</h3>
        <p>Your fingerprint or face is processed by your phone's Hardware Secure Element. Raw biometric data <strong>never leaves your device</strong> — only a mathematical proof is transmitted.</p>
      </div>
      <div class="step">
        <div class="step-num">2</div>
        <h3>Zero-Knowledge Proof</h3>
        <p>A Groth16 ZK-proof is generated on our server. It cryptographically proves you are a unique human without revealing any personal information.</p>
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

<!-- CTA -->
<section>
  <div class="section-inner">
    <div class="cta-section">
      <div class="section-label" style="text-align:center">Get started</div>
      <h2>Join the fairest currency on Earth</h2>
      <p>Download the Aequitas app, scan your biometrics, and receive 1,000 AEQ within 1 second. No fees, no investment, no prerequisites.</p>
      <div class="hero-btns">
        <a href="/download/app.apk" class="btn-primary">📱 Download Aequitas App (Android)</a>
        <a href="/register" class="btn-secondary">🌐 Register via Browser</a>
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
  </div>
  <p>Aequitas Chain · Chain ID 1926 · <span>aequitas.digital</span> · Launched June 2026</p>
  <p style="margin-top:6px">"<em>Money exists because people exist. Nothing more, nothing less.</em>"</p>
</footer>

<script src="/landing.js"></script>
</body>
</html>`
