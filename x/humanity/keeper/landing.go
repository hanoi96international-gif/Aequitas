package keeper

const landingHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="google" content="notranslate">
<title>Aequitas — Proof of Humanity Chain</title>
<meta name="description" content="Phase 1: one human, one account, 1,000 AEQ start — every new registration passes a live face check by independent matching services. Live Gini on chain.">
<meta name="theme-color" content="#0B0D14">
<link rel="canonical" href="https://aequitas.digital/">
<link rel="icon" href="/favicon.svg" type="image/svg+xml">
<link rel="apple-touch-icon" href="/apple-touch-icon.png">
<meta property="og:type" content="website">
<meta property="og:site_name" content="Aequitas">
<meta property="og:url" content="https://aequitas.digital/">
<meta property="og:title" content="Aequitas — money that belongs to every human equally">
<meta property="og:description" content="Phase 1: one human, one account, 1,000 AEQ start. Live face check at registration. Live on-chain Gini.">
<meta property="og:image" content="https://aequitas.digital/og-image.png">
<meta property="og:image:width" content="1200">
<meta property="og:image:height" content="630">
<meta property="og:image:alt" content="Aequitas — money that belongs to every human equally. 1,000 AEQ per verified human, Gini measured on chain, 0.1% protocol fee on transfers.">
<meta name="twitter:card" content="summary_large_image">
<meta name="twitter:site" content="@AequitasMoney">
<meta name="twitter:title" content="Aequitas — money that belongs to every human equally">
<meta name="twitter:description" content="Phase 1: one human, one account, 1,000 AEQ start. Live face check at registration. Live on-chain Gini.">
<meta name="twitter:image" content="https://aequitas.digital/og-image.png">
<link rel="preconnect" href="https://fonts.bunny.net" crossorigin="anonymous">
<link href="https://fonts.bunny.net/css?family=inter:300,400,500,600,700,800,900&display=swap" rel="stylesheet" referrerpolicy="no-referrer" crossorigin="anonymous">
<style>
*{box-sizing:border-box;margin:0;padding:0;min-width:0}
:root{
  --bg:#0B0D14;--card:#12151F;--card2:#181C28;
  --accent:#5B8CFF;--teal:#5B8CFF;--gold:#F5A524;--green:#3DDC97;
  --text:#E8EAF0;--muted:#9AA3B5;--border:rgba(255,255,255,0.08);--red:#FF6B6B;--warn:#F5A524;
  --radius:16px;--radius-pill:999px;
  --grad:linear-gradient(135deg,#5B8CFF,#3DDC97);
  --shadow:0 8px 32px rgba(0,0,0,.35);
}
html{scroll-behavior:smooth}
section[id]{scroll-margin-top:122px}
body{background:var(--bg);color:var(--text);font-family:Inter,system-ui,-apple-system,sans-serif;line-height:1.55;font-size:17px;overflow-x:hidden;overflow-wrap:break-word}

/* ── NAV ─────────────────────────────────────────────────────── */
nav{position:fixed;top:0;left:0;right:0;z-index:100;background:rgba(11,13,20,0.92);backdrop-filter:blur(14px);border-bottom:1px solid var(--border);display:flex;flex-direction:column}
nav::before{content:'';position:absolute;top:0;left:0;right:0;height:2px;background:var(--grad);opacity:0.85}
.nav-top{padding:0 20px;height:60px;display:flex;align-items:center;justify-content:space-between;gap:10px}
.logo-wrap{display:flex;align-items:center;gap:12px;flex-shrink:0;text-decoration:none;position:relative;z-index:1}
.logo-icon{width:34px;height:34px;border-radius:10px;background:var(--grad);display:flex;align-items:center;justify-content:center;font-size:17px;box-shadow:0 0 20px rgba(91,140,255,0.22)}
.logo-text{font-size:1.06rem;font-weight:900;letter-spacing:3px;background:var(--grad);-webkit-background-clip:text;background-clip:text;-webkit-text-fill-color:transparent}
.logo-sub{font-size:0.69rem;color:var(--muted);letter-spacing:2.5px;text-transform:uppercase}
.lang-sel{background:rgba(255,255,255,0.06);color:var(--muted);border:1px solid rgba(255,255,255,0.1);border-radius:var(--radius-pill);padding:6px 12px;font-family:inherit;font-size:0.79rem;outline:none;cursor:pointer;flex-shrink:0}
.header-right{display:flex;gap:8px;align-items:center;position:relative;z-index:1;min-width:0;overflow-x:auto;-webkit-overflow-scrolling:touch;scrollbar-width:none}
.header-right::-webkit-scrollbar{display:none}
.header-right .badge,.header-right .phase0-badge,.header-right .nav-cta{flex-shrink:0}
.badge{display:flex;align-items:center;gap:5px;padding:5px 11px;border-radius:var(--radius-pill);font-size:0.76rem;letter-spacing:0.5px;font-weight:600}
.badge-live{background:rgba(61,220,151,0.1);border:1px solid rgba(61,220,151,0.3);color:var(--green)}
.badge-health{cursor:help;transition:background 0.3s,border-color 0.3s,color 0.3s}
.badge-health-healthy{background:rgba(61,220,151,0.1);border:1px solid rgba(61,220,151,0.3);color:var(--green)}
.badge-health-unhealthy{background:rgba(255,107,107,0.1);border:1px solid rgba(255,107,107,0.35);color:var(--red);animation:healthPulse 1.6s infinite}
@keyframes healthPulse{0%,100%{opacity:1}50%{opacity:0.55}}
.phase0-badge{display:inline-flex;align-items:center;gap:6px;padding:5px 12px;border-radius:var(--radius-pill);font-size:0.74rem;font-weight:700;letter-spacing:0.3px;background:rgba(245,165,36,0.12);border:1px solid rgba(245,165,36,0.4);color:var(--warn)}
.nav-cta{display:inline-flex;align-items:center;padding:8px 16px;border-radius:var(--radius-pill);background:var(--accent);color:#fff;font-size:0.78rem;font-weight:700;text-decoration:none;transition:opacity 0.2s}
.nav-cta:hover{opacity:0.9}
.tabs{border-top:1px solid var(--border);padding:8px 18px;display:flex;overflow-x:auto;-webkit-overflow-scrolling:touch;scrollbar-width:none;gap:6px}
.tabs::-webkit-scrollbar{display:none}
.tab{padding:10px 16px;font-size:0.81rem;color:var(--muted);text-decoration:none;border-radius:var(--radius-pill);letter-spacing:0.5px;font-weight:600;white-space:nowrap;transition:all 0.2s;flex-shrink:0;border:1px solid transparent}
.tab:hover{color:var(--text);background:rgba(255,255,255,0.04)}
.tab.active{color:#fff;background:var(--grad);box-shadow:0 0 24px rgba(91,140,255,0.2);border-color:transparent}
@media(max-width:700px){
.nav-top{padding:0 12px;height:56px}
.logo-icon{width:30px;height:30px;font-size:15px}
.logo-text{font-size:0.95rem;letter-spacing:2px}
.logo-sub{display:none}
.badge-health{display:none}
.tabs{padding:6px 10px;gap:4px}
.tab{padding:9px 13px}
}

/* ── HERO ────────────────────────────────────────────────────── */
.hero{min-height:100vh;display:flex;flex-direction:column;align-items:center;justify-content:center;text-align:center;padding:150px 20px 56px;position:relative;overflow:hidden}
.hero::before{content:'';position:absolute;inset:0;background:radial-gradient(ellipse 80% 50% at 50% 0%,rgba(91,140,255,0.14) 0%,transparent 60%),radial-gradient(ellipse 55% 40% at 85% 100%,rgba(61,220,151,0.06) 0%,transparent 60%);pointer-events:none}
.hero-badge{display:inline-flex;align-items:center;gap:8px;background:rgba(245,165,36,0.12);border:1px solid rgba(245,165,36,0.35);border-radius:var(--radius-pill);padding:7px 16px;font-size:0.75rem;color:var(--warn);font-weight:700;letter-spacing:0.4px;margin-bottom:24px}
.pulse{width:7px;height:7px;border-radius:50%;background:var(--green);animation:pulse 2s infinite}
@keyframes pulse{0%,100%{opacity:1;transform:scale(1)}50%{opacity:0.5;transform:scale(0.85)}}
h1{font-family:Inter,system-ui,sans-serif;font-size:clamp(2.5rem,6vw,3rem);line-height:1.1;font-weight:800;max-width:720px;margin-bottom:18px;letter-spacing:-0.02em}
h1 span{background:var(--grad);-webkit-background-clip:text;-webkit-text-fill-color:transparent}
.hero-sub{font-size:clamp(1rem,2.5vw,1.125rem);color:var(--muted);max-width:540px;margin-bottom:28px;font-weight:400;line-height:1.55}
.hero-btns{display:flex;flex-wrap:wrap;gap:12px;justify-content:center;margin-bottom:28px}
.btn-primary{display:inline-flex;align-items:center;justify-content:center;gap:8px;background:var(--accent);color:#fff;padding:14px 28px;border-radius:var(--radius-pill);font-size:0.95rem;font-weight:700;text-decoration:none;transition:opacity 0.2s,transform 0.15s;letter-spacing:0.2px;box-shadow:var(--shadow)}
.btn-primary:hover{opacity:0.92;transform:translateY(-1px)}
.btn-secondary{display:inline-flex;align-items:center;justify-content:center;gap:8px;background:rgba(255,255,255,0.04);border:1px solid var(--border);color:var(--text);padding:14px 28px;border-radius:var(--radius-pill);font-size:0.95rem;font-weight:600;text-decoration:none;transition:all 0.2s}
.btn-secondary:hover{background:rgba(91,140,255,0.1);border-color:rgba(91,140,255,0.4)}
.hero-pills{display:flex;flex-wrap:wrap;gap:8px;justify-content:center;margin-bottom:8px}
.pill{display:inline-flex;align-items:center;padding:6px 14px;border-radius:var(--radius-pill);font-size:0.75rem;font-weight:600;color:var(--muted);background:rgba(255,255,255,0.04);border:1px solid var(--border)}
.pill strong{color:var(--text);font-weight:700;margin-right:4px}

/* ── STATS BAR ───────────────────────────────────────────────── */
.stats-bar{background:transparent;border:none;padding:20px 20px 8px;display:flex;justify-content:center;gap:12px;flex-wrap:wrap}
.stat-item{text-align:center;padding:18px 16px;border:1px solid var(--border);border-radius:var(--radius);background:var(--card);flex:1;max-width:220px;min-width:140px;box-shadow:var(--shadow)}
.stat-item:last-child{border-right:1px solid var(--border)}
.stat-num{font-size:clamp(1.4rem,3vw,1.85rem);font-weight:800;font-variant-numeric:tabular-nums}
.stat-lbl{font-size:0.7rem;color:var(--muted);text-transform:uppercase;letter-spacing:1px;margin-top:6px;font-weight:600}
@media(max-width:700px){.stats-bar{padding:12px 12px 4px;gap:8px}.stat-item{flex:calc(50% - 8px);max-width:none;min-width:calc(50% - 8px);padding:16px 12px}}
@media(max-width:380px){.stat-item{flex:100%;min-width:100%}}
.stats-live{padding:18px 20px 0;text-align:center;font-size:0.78rem;color:var(--muted)}
.stats-live strong{color:var(--gold);font-weight:700}

/* ── SECTION ─────────────────────────────────────────────────── */
section{padding:72px 20px}
.section-inner{max-width:1100px;margin:0 auto}
.section-label{font-size:0.72rem;color:var(--accent);letter-spacing:3px;text-transform:uppercase;font-weight:700;margin-bottom:12px}
h2{font-family:Inter,system-ui,sans-serif;font-size:clamp(1.6rem,4vw,2.2rem);line-height:1.2;font-weight:800;margin-bottom:14px;letter-spacing:-0.02em}
.section-sub{font-size:1.05rem;color:var(--muted);max-width:560px;margin-bottom:40px;line-height:1.55}

.steps{display:grid;grid-template-columns:repeat(3,1fr);gap:16px}
.step{background:var(--card);border:1px solid var(--border);border-radius:var(--radius);padding:24px;position:relative;transition:border-color 0.2s,box-shadow 0.2s;box-shadow:var(--shadow)}
.step:hover{border-color:rgba(91,140,255,0.35)}
.step-num{width:40px;height:40px;border-radius:50%;background:var(--grad);display:flex;align-items:center;justify-content:center;font-weight:800;font-size:1rem;margin-bottom:14px;color:#fff}
.step h3{font-size:1.05rem;font-weight:700;margin-bottom:8px}
.step p{font-size:0.92rem;color:var(--muted);line-height:1.55}
@media(max-width:700px){.steps{grid-template-columns:1fr}}

.sybil-blurb{margin-top:28px;padding:18px 20px;background:rgba(91,140,255,0.06);border:1px solid rgba(91,140,255,0.2);border-radius:var(--radius);font-size:0.92rem;color:var(--muted);line-height:1.55}
.sybil-blurb strong{color:var(--text)}

.section-link{display:inline-block;margin-top:28px;font-size:0.88rem;font-weight:600;color:var(--accent);text-decoration:none;border-bottom:1px solid rgba(91,140,255,0.35);padding-bottom:2px;transition:color 0.2s,border-color 0.2s}
.section-link:hover{color:var(--green);border-color:rgba(61,220,151,0.5)}

.explore-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:14px;text-align:left}
.explore-card{display:flex;flex-direction:column;gap:6px;background:var(--card);border:1px solid var(--border);border-radius:var(--radius);padding:22px;text-decoration:none;transition:border-color 0.2s,background 0.2s,transform 0.2s;box-shadow:var(--shadow)}
.explore-card:hover{border-color:rgba(91,140,255,0.4);background:var(--card2);transform:translateY(-2px)}
.explore-icon{font-size:1.3rem;line-height:1}
.explore-name{font-size:0.95rem;font-weight:700;color:var(--text)}
.explore-desc{font-size:0.82rem;color:var(--muted);line-height:1.55}

.gini-compact{max-width:560px}
.gini-row{display:flex;align-items:center;gap:12px;margin-bottom:10px}
.gini-label{font-size:0.85rem;min-width:110px;color:var(--muted)}
.gini-bar-wrap{flex:1;height:8px;background:rgba(255,255,255,0.06);border-radius:4px;overflow:hidden}
.gini-bar{height:100%;border-radius:4px}
.gini-val{font-size:0.8rem;font-weight:700;min-width:40px;text-align:right}
.gini-row.aeq .gini-label{color:var(--gold);font-weight:700}
.gini-row.aeq .gini-bar{background:var(--gold)}

/* Phase-0 disclaimer — full width, warn-tint border */
.disclaimer-card{max-width:1100px;margin:0 auto;background:rgba(245,165,36,0.07);border:1px solid rgba(245,165,36,0.38);border-radius:var(--radius);padding:22px 24px;box-shadow:var(--shadow)}
.disclaimer-card h3{font-size:0.95rem;font-weight:800;color:var(--warn);margin-bottom:10px;letter-spacing:0.2px}
.disclaimer-card p{font-size:0.92rem;color:var(--muted);line-height:1.55}
.oss-line{margin-top:14px;font-size:0.82rem;color:var(--muted)}
.oss-line strong{color:var(--text)}

footer{border-top:1px solid var(--border);padding:40px 20px;text-align:center}
.footer-links{display:flex;flex-wrap:wrap;justify-content:center;gap:20px;margin-bottom:20px}
.footer-links a{color:var(--muted);text-decoration:none;font-size:0.82rem;transition:color 0.2s}
.footer-links a:hover{color:var(--text)}
.social{display:inline-flex;align-items:center;gap:7px}
.social svg{width:15px;height:15px;flex:none;fill:currentColor}
.social-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(260px,1fr));gap:16px;margin-top:32px;max-width:760px;margin-left:auto;margin-right:auto}
.social-card{background:var(--card2);border:1px solid var(--border);border-radius:var(--radius);padding:32px 24px;display:flex;flex-direction:column;align-items:center;text-align:center;gap:10px;text-decoration:none;transition:transform 0.2s,border-color 0.2s,background 0.2s;box-shadow:var(--shadow)}
.social-card:hover{border-color:rgba(91,140,255,0.45);background:var(--card);transform:translateY(-3px)}
.social-card svg{width:56px;height:56px;fill:var(--text);transition:fill 0.2s}
.social-card:hover svg{fill:var(--accent)}
.social-name{font-size:1.1rem;font-weight:700;color:var(--text);line-height:1.2}
.social-handle{font-size:0.85rem;font-weight:600;color:var(--accent)}
.social-desc{font-size:0.8rem;color:var(--muted);line-height:1.55;max-width:260px}
@media(max-width:600px){.social-card{padding:26px 18px}.social-card svg{width:48px;height:48px}}
footer p{font-size:0.78rem;color:var(--muted)}
footer p span{color:var(--accent)}

:focus-visible{outline:2px solid var(--accent);outline-offset:3px;border-radius:4px}
a:focus-visible,button:focus-visible,select:focus-visible,
[role="button"]:focus-visible,[tabindex]:focus-visible{outline:2px solid var(--accent);outline-offset:3px}
.tab.active:focus-visible{outline-color:#fff}

@media(max-width:480px){
.btn-primary,.btn-secondary{padding:16px 22px;font-size:0.95rem;width:100%;border-radius:var(--radius-pill)}
.hero-btns{flex-direction:column;width:100%;max-width:340px}
h1{font-size:2.5rem}
.hero{padding:140px 16px 44px}
section{padding:56px 16px}
.nav-cta{padding:7px 12px;font-size:0.72rem}
.disclaimer-card{padding:18px 16px}
}
</style>
</head>
<body>

<nav>
  <div class="nav-top">
    <a href="/" class="logo-wrap">
      <div class="logo-icon">⚖</div>
      <div><div class="logo-text">AEQUITAS</div><div class="logo-sub" data-i18n="logo-sub">PROOF OF HUMANITY</div></div>
    </a>
    <select class="lang-sel" id="lang-sel" aria-label="Language">
      <option value="en">🌐 EN</option>
      <option value="de">🌐 DE</option>
      <option value="es">🌐 ES</option>
      <option value="fr">🌐 FR</option>
      <option value="pt">🌐 PT</option>
      <option value="ru">🌐 RU</option>
      <option value="zh">🌐 ZH</option>
      <option value="ar">🌐 AR</option>
      <option value="hi">🌐 HI</option>
      <option value="id">🌐 ID</option>
      <option value="it">🌐 IT</option>
      <option value="tr">🌐 TR</option>
    </select>
    <div class="header-right">
      <div class="phase0-badge" data-i18n="phase0-badge">Phase 1 · face check</div>
      <div class="badge badge-live"><span class="pulse"></span><span data-i18n="live">LIVE</span></div>
      <div class="badge badge-health badge-health-healthy" id="health-badge" title="Checking network health…">● GHOSTDAG</div>
      <a href="/register" class="nav-cta" data-i18n="nav-register">Register</a>
    </div>
  </div>
  <div class="tabs">
    <a href="/" class="tab active">🏠 Overview</a>
    <a href="/register" class="tab">🔐 Register</a>
    <a href="/explorer" class="tab">🔍 Explorer</a>
    <a href="/index/score" class="tab">⚖️ Equality</a>
    <a href="/network" class="tab">🌐 Network</a>
    <a href="/exchange" class="tab">🔄 Exchange</a>
    <a href="#social" class="tab">💬 Social</a>
  </div>
</nav>

<main>
<section class="hero">
  <div class="hero-badge">
    <span class="pulse"></span>
    <span data-i18n="hero-badge">Phase 1 · Chain ID 1926</span>
  </div>
  <h1 data-i18n="hero-h1">Money that belongs<br>to <span>every human</span> equally</h1>
  <p class="hero-sub" data-i18n="hero-sub">Phase 1: one human, one account, 1,000 AEQ start — every new registration passes a live face check by two independent matching services.</p>
  <div class="hero-btns">
    <a href="/register" class="btn-primary" data-i18n="btn-register">Register now</a>
    <a href="/explorer" class="btn-secondary" data-i18n="btn-explorer">Open explorer</a>
  </div>
  <div class="hero-pills">
    <span class="pill" data-i18n="pill-fee"><strong>0.1%</strong> fee</span>
    <span class="pill" data-i18n="pill-gini"><strong>Live</strong> on-chain Gini</span>
    <span class="pill" data-i18n="pill-phase"><strong>Phase 1</strong></span>
  </div>
</section>

<div class="stats-bar">
  <div class="stat-item">
    <div class="stat-num" id="stat-humans" style="color:#3DDC97">—</div>
    <div class="stat-lbl" data-i18n="stat-humans-lbl">Verified humans</div>
  </div>
  <div class="stat-item">
    <div class="stat-num" id="stat-supply" style="color:#5B8CFF">—</div>
    <div class="stat-lbl" data-i18n="stat-supply-lbl">AEQ in circulation</div>
  </div>
  <div class="stat-item">
    <div class="stat-num" id="stat-gini" style="color:#F5A524">—</div>
    <div class="stat-lbl" data-i18n="stat-gini-lbl">Gini</div>
  </div>
  <div class="stat-item">
    <div class="stat-num" id="stat-blocks" style="color:#9AA3B5">—</div>
    <div class="stat-lbl" data-i18n="stat-blocks-lbl">Blocks</div>
  </div>
</div>
<div class="stats-live"><span data-i18n="ubi-pre">Next equal split in</span> <strong id="ubi-next">—</strong> <span data-i18n="ubi-mid">· the pool holds</span> <strong id="ubi-pool">—</strong> AEQ</div>

<section>
  <div class="section-inner">
    <div class="section-label" data-i18n="how-label">How it works</div>
    <h2 data-i18n="how-h2">Three honest steps (Phase 1)</h2>
    <p class="section-sub" data-i18n="how-sub">Wallet on your phone, a short live face capture, and a one-time grant — no bank account required.</p>
    <div class="steps">
      <div class="step">
        <div class="step-num">1</div>
        <h3 data-i18n="step1-h">Scan</h3>
        <p data-i18n="step1-p">Wallet on your phone; the app captures your face with a random head-turn challenge, and two independent matching services compare it against everyone registered since the face check began. Images are discarded; each service keeps an encrypted template.</p>
      </div>
      <div class="step">
        <div class="step-num">2</div>
        <h3 data-i18n="step2-h">Prove</h3>
        <p data-i18n="step2-p">Zero-knowledge proof to the chain that this face-bound identity is not yet registered (nullifier). The proof server accepts it only with the matching services' signed attestation.</p>
      </div>
      <div class="step">
        <div class="step-num">3</div>
        <h3 data-i18n="step3-h">Receive</h3>
        <p data-i18n="step3-p">1,000 AEQ once per successful registration.</p>
      </div>
    </div>
    <div class="sybil-blurb" data-i18n="sybil-blurb"><strong>Sybil / protection:</strong> live face check (quorum 2 of independent matching services) + signed attestation + on-chain nullifier, spent once. Named limits: accounts from before the face check (25 Aug 2026 — nearly all of today's 18) have no face template; the matching threshold is not yet calibrated on real captures; liveness is a head-turn challenge, so advanced deepfakes remain a residual risk; the split-share mode (<code>MPC</code>) runs in shadow, each service still holds a whole encrypted template.</div>
    <a class="section-link" href="/register" data-i18n="how-link">Register and claim your 1,000 AEQ →</a>
  </div>
</section>

<section style="background:var(--card);border-top:1px solid var(--border);border-bottom:1px solid var(--border)">
  <div class="section-inner">
    <div class="section-label" data-i18n="why-label">Fairness</div>
    <h2 data-i18n="why-h2">Bitcoin's Gini is ~0.85 — higher than any country</h2>
    <p class="section-sub" data-i18n="why-sub">Supply = verified humans × 1,000 AEQ. Gini from the live node. Go L1 is the ledger source of truth (not the mirror contract alone).</p>
    <div class="gini-compact">
      <div class="gini-row aeq">
        <span class="gini-label">Aequitas</span>
        <div class="gini-bar-wrap"><div class="gini-bar" id="bar-aeq" style="width:9%"></div></div>
        <span class="gini-val" id="val-aeq" style="color:var(--gold)">—</span>
      </div>
      <div class="gini-row">
        <span class="gini-label">Bitcoin</span>
        <div class="gini-bar-wrap"><div class="gini-bar" style="width:85%;background:#FF6B6B"></div></div>
        <span class="gini-val" style="color:#FF6B6B">~0.85</span>
      </div>
    </div>
    <a class="section-link" href="/index/score" data-i18n="why-link">Lorenz curve, history and the country ladder →</a>
  </div>
</section>

<section style="padding-top:40px;padding-bottom:40px">
  <div class="disclaimer-card">
    <h3 data-i18n="disc-title">Phase 1 disclaimer</h3>
    <p data-i18n="disc-body">Phase 1: since 25 Aug 2026 the proof server refuses any registration without a signed attestation from the matching quorum — a second phone no longer gives the same face a second account. What is not yet true: accounts registered before that date have no face template and could in principle register again on a new wallet; error rates are not calibrated (that needs ~1,000 impostor pairs); liveness is a head-turn challenge, stronger deepfake defenses are being calibrated. Read “one human, one account” as “checked, with named limits” — not as “impossible to circumvent.”</p>
    <p class="oss-line" data-i18n="oss-line"><strong>Open source:</strong> Core chain public · identity/proof services partly private in Phase 1.</p>
  </div>
</section>

<section>
  <div class="section-inner">
    <div class="section-label" data-i18n="rest-label">The rest of the site</div>
    <h2 data-i18n="rest-h2">Everything else has its own section</h2>
    <p class="section-sub" data-i18n="rest-sub">This page stops here on purpose. Each section below holds the detail — served live by this node.</p>
    <div class="explore-grid">
      <a class="explore-card" href="/register">
        <span class="explore-icon">🔐</span>
        <span class="explore-name">Register</span>
        <span class="explore-desc" data-i18n="card-register-d">Get verified and claim the 1,000 AEQ that come with being a human.</span>
      </a>
      <a class="explore-card" href="/explorer">
        <span class="explore-icon">🔍</span>
        <span class="explore-name">Explorer</span>
        <span class="explore-desc" data-i18n="card-explorer-d">Blocks, transactions and the human registry as they happen.</span>
      </a>
      <a class="explore-card" href="/index/score">
        <span class="explore-icon">⚖️</span>
        <span class="explore-name">Equality</span>
        <span class="explore-desc" data-i18n="card-equality-d">The Gini coefficient in full: Lorenz curve, history, wealth cap.</span>
      </a>
      <a class="explore-card" href="/network">
        <span class="explore-icon">🌐</span>
        <span class="explore-name">Network</span>
        <span class="explore-desc" data-i18n="card-network-d">Consensus, nodes, UBI, demurrage and guardians — the rules themselves.</span>
      </a>
      <a class="explore-card" href="/exchange">
        <span class="explore-icon">🔄</span>
        <span class="explore-name">Exchange</span>
        <span class="explore-desc" data-i18n="card-exchange-d">Swap and liquidity. Transfer fees go 100% to UBI; swap fees split 40 / 30 / 30.</span>
      </a>
      <a class="explore-card" href="#social">
        <span class="explore-icon">💬</span>
        <span class="explore-name">Social</span>
        <span class="explore-desc" data-i18n="card-social-d">X and Telegram — announcements and the awkward questions.</span>
      </a>
    </div>
  </div>
</section>

<section id="social" style="background:var(--card);border-top:1px solid var(--border);border-bottom:1px solid var(--border)">
  <div class="section-inner">
    <div class="section-label" style="text-align:center" data-i18n="soc-label">Social media</div>
    <h2 style="text-align:center" data-i18n="soc-h2">Where the network talks</h2>
    <p class="section-sub" style="text-align:center;margin-left:auto;margin-right:auto" data-i18n="soc-sub">Announcements, the state of the chain, and the awkward questions &mdash; in public, on both.</p>
    <div class="social-grid">
      <a class="social-card" href="https://x.com/AequitasMoney" target="_blank" rel="noopener noreferrer">
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z"/></svg>
        <div class="social-name">X</div>
        <div class="social-handle" dir="ltr">@AequitasMoney</div>
        <div class="social-desc" data-i18n="soc-x-d">Announcements, and what the chain is actually doing. Short form.</div>
      </a>
      <a class="social-card" href="https://t.me/aequitasmoney" target="_blank" rel="noopener noreferrer">
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z"/></svg>
        <div class="social-name">Telegram</div>
        <div class="social-handle" dir="ltr">t.me/aequitasmoney</div>
        <div class="social-desc" data-i18n="soc-tg-d">The open group: questions, node operators, and help getting registered.</div>
      </a>
    </div>
  </div>
</section>

</main>

<footer>
  <div class="footer-links">
    <a href="/register">Register</a>
    <a href="/explorer">Block Explorer</a>
    <a href="/index/score">Equality Score</a>
    <a href="/network">Network</a>
    <a href="/exchange">Exchange</a>
    <a href="/download/node-guide-en.pdf">Node Guide (EN)</a>
    <a href="/download/node-guide-de.pdf">Node Guide (DE)</a>
    <a href="https://github.com/hanoi96international-gif/Aequitas">GitHub</a><!--LEGAL_LINKS-->
    <a href="https://x.com/AequitasMoney" target="_blank" rel="noopener noreferrer" class="social"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z"/></svg>@AequitasMoney</a>
    <a href="https://t.me/aequitasmoney" target="_blank" rel="noopener noreferrer" class="social"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z"/></svg>Telegram</a>
  </div>
  <p>Aequitas Chain · Chain ID 1926 · <span>aequitas.digital</span> · <span data-i18n="foot-launched">Launched June 2026</span> · <span data-i18n="foot-phase">Phase 1</span></p>
  <p style="margin-top:6px">"<em data-i18n="foot-quote">Money exists because people exist. Nothing more, nothing less.</em>"</p>
</footer>

<script src="/landing.js"></script>
</body>
</html>`
