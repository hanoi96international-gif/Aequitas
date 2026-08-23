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
<link rel="canonical" href="https://aequitas.digital/">
<link rel="icon" href="/favicon.svg" type="image/svg+xml">
<link rel="apple-touch-icon" href="/apple-touch-icon.png">
<!-- Link previews. Without these, every link to this site posted on X or
     Telegram — the two channels the site itself sends people to — renders as
     a bare URL: no title, no image, no description. og:image must be an
     absolute URL; crawlers do not resolve relative paths. -->
<meta property="og:type" content="website">
<meta property="og:site_name" content="Aequitas">
<meta property="og:url" content="https://aequitas.digital/">
<meta property="og:title" content="Aequitas — money that belongs to every human equally">
<meta property="og:description" content="The first blockchain where the money supply is tied to verified human existence. Every person receives 1,000 AEQ. The network measures its own inequality and publishes it live, on chain.">
<meta property="og:image" content="https://aequitas.digital/og-image.png">
<meta property="og:image:width" content="1200">
<meta property="og:image:height" content="630">
<meta property="og:image:alt" content="Aequitas — money that belongs to every human equally. 1,000 AEQ per verified human, Gini measured on chain, zero gas fees.">
<meta name="twitter:card" content="summary_large_image">
<meta name="twitter:site" content="@AequitasMoney">
<meta name="twitter:title" content="Aequitas — money that belongs to every human equally">
<meta name="twitter:description" content="The first blockchain where the money supply is tied to verified human existence. Every person receives 1,000 AEQ. The network measures its own inequality and publishes it live, on chain.">
<meta name="twitter:image" content="https://aequitas.digital/og-image.png">
<link rel="preconnect" href="https://fonts.bunny.net" crossorigin="anonymous">
<link href="https://fonts.bunny.net/css?family=inter:300,400,500,600,700,800,900|dm-serif-display:400&display=swap" rel="stylesheet" referrerpolicy="no-referrer" crossorigin="anonymous">
<style>
/* Flex and grid items default to min-width:auto — meaning a flex/grid item
   refuses to shrink below its own content's unwrapped width. .header-right
   and the rendered wallet-address row already work around this explicitly
   (min-width:0 on each), because without it a single long unbreakable value
   forces its container wider than the viewport, and that width then
   propagates straight up through the grid track to the whole page: every
   sibling column shifts along with it, text that could otherwise wrap gets
   pushed off-screen, and the entire document scrolls horizontally instead of
   the one long value wrapping or eliding. With twelve translations of
   unpredictable length sharing the same layout, a rule fixing this in two
   places and not the other thirty was always going to resurface — this
   applies the same fix everywhere at once. It only changes flex/grid item
   sizing: an element that still needs a hard floor keeps it, since an
   explicit min-width elsewhere in this file is more specific than this
   universal rule and wins regardless of source order. */
*{box-sizing:border-box;margin:0;padding:0;min-width:0}
:root{
  --bg:#0C0E16;--card:#131620;--card2:#1A1D2B;
  --purple:#9B72F6;--teal:#22D3EE;--gold:#F0B429;--green:#34D399;
  --text:#E8EDF5;--muted:#8892A4;--border:rgba(255,255,255,0.07);--red:#F87171;
  --radius:12px;--grad:linear-gradient(135deg,#9B72F6,#22D3EE);
}
html{scroll-behavior:smooth}
section[id]{scroll-margin-top:122px}
body{background:var(--bg);color:var(--text);font-family:'Inter',-apple-system,sans-serif;line-height:1.6;overflow-x:hidden;overflow-wrap:break-word}

/* ── NAV ─────────────────────────────────────────────────────── */
/* The header is the explorer's, deliberately down to the pixel values. This
   page used to carry its own text-link navigation, so moving between Overview
   and any other section changed how the site looked, not just what it showed.
   Same two rows, same pill tabs, same active gradient — the only difference is
   which entry is active. Values lifted from explorer.css so the two cannot
   drift apart by being retyped. */
nav{position:fixed;top:0;left:0;right:0;z-index:100;background:rgba(12,14,22,0.92);backdrop-filter:blur(12px);border-bottom:1px solid var(--border);display:flex;flex-direction:column}
nav::before{content:'';position:absolute;top:0;left:0;right:0;height:2px;background:var(--grad);opacity:0.8}
.nav-top{padding:0 24px;height:60px;display:flex;align-items:center;justify-content:space-between;gap:10px}
/* Lifted from explorer.css .logo-wrap/.logo-icon/.logo-text/.logo-sub. The
   previous edit replaced the whole nav block and dropped these rules with it,
   so the brand rendered as a bare underlined link — visible in the screenshot
   as purple underlined AEQUITAS with no gradient tile. */
.logo-wrap{display:flex;align-items:center;gap:12px;flex-shrink:0;text-decoration:none;position:relative;z-index:1}
.logo-icon{width:34px;height:34px;border-radius:9px;background:var(--grad);display:flex;align-items:center;justify-content:center;font-size:17px;box-shadow:0 0 24px rgba(155,114,246,0.18)}
.logo-text{font-size:1.06rem;font-weight:900;letter-spacing:3px;background:var(--grad);-webkit-background-clip:text;background-clip:text;-webkit-text-fill-color:transparent}
.logo-sub{font-size:0.69rem;color:var(--muted);letter-spacing:2.5px;text-transform:uppercase}
/* Badges, values from explorer.css so both headers stay one header. --neon
   there is #34D399, which is --green here, so the colour is the same value
   under a different name rather than a second shade of the same idea. */
/* Values lifted from explorer.css .lang-sel — the selector has to look the
   same on both pages, since it is the same control. */
.lang-sel{background:rgba(255,255,255,0.06);color:var(--muted);border:1px solid rgba(255,255,255,0.1);border-radius:6px;padding:5px 10px;font-family:inherit;font-size:0.79rem;outline:none;cursor:pointer;flex-shrink:0}
.header-right{display:flex;gap:8px;align-items:center;position:relative;z-index:1;min-width:0;overflow-x:auto;-webkit-overflow-scrolling:touch;scrollbar-width:none;
  /* Same scroll-shadow as the explorer's .header-right — this row is a
     deliberate copy of that header, so it needed the same fix: without it,
     GHOSTDAG/KNIGHTDAG could scroll out of view on a narrow screen with no
     hint that they still exist. */
  background:
    linear-gradient(90deg,rgba(12,14,22,0.92) 30%,rgba(12,14,22,0)),
    linear-gradient(270deg,rgba(12,14,22,0.92) 30%,rgba(12,14,22,0)) 100% 0,
    radial-gradient(farthest-side at 0 50%,rgba(155,114,246,0.45),transparent),
    radial-gradient(farthest-side at 100% 50%,rgba(155,114,246,0.45),transparent) 100% 0;
  background-repeat:no-repeat;
  background-size:32px 100%,32px 100%,10px 100%,10px 100%;
  background-attachment:local,local,scroll,scroll}
.header-right::-webkit-scrollbar{display:none}
.header-right .badge{flex-shrink:0}
.badge{display:flex;align-items:center;gap:5px;padding:5px 11px;border-radius:20px;font-size:0.76rem;letter-spacing:0.5px;font-weight:600}
.badge-live{background:rgba(4,120,87,0.08);border:1px solid rgba(4,120,87,0.25);color:var(--green)}
.badge-dag{background:linear-gradient(135deg,rgba(155,114,246,0.14),rgba(34,211,238,0.08));border:1px solid rgba(155,114,246,0.4);color:var(--purple);font-weight:700;text-shadow:0 0 12px rgba(155,114,246,0.5);animation:knightGlow 3s ease-in-out infinite}
@keyframes knightGlow{0%,100%{box-shadow:0 0 0 rgba(155,114,246,0)}50%{box-shadow:0 0 10px rgba(155,114,246,0.35)}}
.badge-health{cursor:help;transition:background 0.3s,border-color 0.3s,color 0.3s}
.badge-health-healthy{background:rgba(4,120,87,0.08);border:1px solid rgba(4,120,87,0.25);color:var(--green)}
.badge-health-unhealthy{background:rgba(248,113,113,0.1);border:1px solid rgba(248,113,113,0.35);color:var(--red);animation:healthPulse 1.6s infinite}
@keyframes healthPulse{0%,100%{opacity:1}50%{opacity:0.55}}
.tabs{border-top:1px solid var(--border);padding:8px 18px;display:flex;overflow-x:auto;-webkit-overflow-scrolling:touch;scrollbar-width:none;gap:6px}
.tabs::-webkit-scrollbar{display:none}
.tab{padding:10px 16px;font-size:0.81rem;color:var(--muted);text-decoration:none;border-radius:20px;letter-spacing:0.5px;font-weight:600;white-space:nowrap;transition:all 0.2s;flex-shrink:0;border:1px solid transparent}
.tab:hover{color:var(--text);background:rgba(255,255,255,0.04)}
.tab.active{color:#fff;background:var(--grad);box-shadow:0 0 24px rgba(155,114,246,0.18);border-color:transparent}
@media(max-width:600px){
.nav-top{padding:0 14px;height:56px}
.logo-icon{width:30px;height:30px;font-size:15px}
.logo-text{font-size:0.95rem;letter-spacing:2px}
/* Matches explorer.css's identical fix: at a 390px phone, the German
   subtitle alone (MENSCHLICHKEITSNACHWEIS, with its letter-spacing) measured
   222 of the header's 390px, leaving no real room for LIVE + GHOSTDAG +
   KNIGHTDAG. The wordmark alone still identifies the brand; the tagline is
   the one thing here that's safe to drop. */
.logo-sub{display:none}
.tabs{padding:6px 10px;gap:4px}
.tab{padding:9px 13px}
}

/* ── HERO ────────────────────────────────────────────────────── */
.hero{min-height:100vh;display:flex;flex-direction:column;align-items:center;justify-content:center;text-align:center;padding:158px 24px 60px;position:relative;overflow:hidden}
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
.hero-proof{font-size:0.72rem;color:var(--muted);display:flex;align-items:center;gap:8px;flex-wrap:wrap;justify-content:center}
/* Was '.hero-proof span'. The line now carries translated spans as well as
   the ticks, and a bare element selector would have painted the prose green
   too. */
.hero-proof .ok{color:var(--green)}

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
.section-label{font-size:0.72rem;color:var(--purple);letter-spacing:3px;text-transform:uppercase;font-weight:600;margin-bottom:12px}

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

/* ── STATS LIVE LINE ─────────────────────────────────────────── */
/* The one live fact that belongs on an overview and nowhere else: when the
   next equal split happens. Deliberately a single line under the numbers
   rather than a card — the mechanism it comes from is explained on /network. */
.stats-live{padding:18px 24px 0;text-align:center;font-size:0.74rem;color:var(--muted)}
.stats-live strong{color:var(--gold);font-weight:700}

/* ── SECTION LINK ────────────────────────────────────────────── */
/* Every section on this page now ends in a door instead of continuing into
   its own detail. The overview states the claim; the linked section proves
   it. Without these the compression would just be missing information. */
.section-link{display:inline-block;margin-top:30px;font-size:0.8rem;font-weight:600;color:var(--purple);text-decoration:none;border-bottom:1px solid rgba(155,114,246,0.35);padding-bottom:2px;transition:color 0.2s,border-color 0.2s}
.section-link:hover{color:var(--teal);border-color:rgba(34,211,238,0.5)}

/* ── EXPLORE GRID ────────────────────────────────────────────── */
.explore-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(300px,1fr));gap:16px;text-align:left}
.explore-card{display:flex;flex-direction:column;gap:6px;background:var(--card);border:1px solid var(--border);border-radius:var(--radius);padding:22px;text-decoration:none;transition:border-color 0.2s,background 0.2s,transform 0.2s}
.explore-card:hover{border-color:rgba(155,114,246,0.4);background:var(--card2);transform:translateY(-2px)}
.explore-icon{font-size:1.3rem;line-height:1}
.explore-name{font-size:0.95rem;font-weight:700;color:var(--text)}
.explore-desc{font-size:0.78rem;color:var(--muted);line-height:1.65}

/* ── GINI COMPARISON ─────────────────────────────────────────── */
/* Two rows, not six. The country ladder and the Lorenz curve live on the
   Equality section, which is the page that exists to hold them. */
.gini-compact{max-width:560px}
.gini-row{display:flex;align-items:center;gap:12px;margin-bottom:10px}
.gini-label{font-size:0.82rem;min-width:110px;color:var(--muted)}
.gini-bar-wrap{flex:1;height:8px;background:rgba(255,255,255,0.06);border-radius:4px;overflow:hidden}
.gini-bar{height:100%;border-radius:4px}
.gini-val{font-size:0.78rem;font-weight:700;min-width:40px;text-align:right}
.gini-row.aeq .gini-label{color:var(--gold);font-weight:700}
.gini-row.aeq .gini-bar{background:var(--gold)}

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

/* ── SOCIAL SECTION ──────────────────────────────────────────── */
/* Its own section rather than another footer link: the marks used to live
   only in the footer, which is where a first-time visitor never looks. Big
   enough to be the thing you see, and the whole card is the link — not just
   the wordmark. */
.social-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:20px;margin-top:36px;max-width:760px;margin-left:auto;margin-right:auto}
.social-card{background:var(--card2);border:1px solid var(--border);border-radius:var(--radius);padding:38px 28px;display:flex;flex-direction:column;align-items:center;text-align:center;gap:12px;text-decoration:none;transition:transform 0.2s,border-color 0.2s,background 0.2s}
.social-card:hover{border-color:rgba(155,114,246,0.45);background:var(--card);transform:translateY(-3px)}
.social-card svg{width:64px;height:64px;fill:var(--text);transition:fill 0.2s}
.social-card:hover svg{fill:var(--purple)}
.social-name{font-size:1.15rem;font-weight:700;color:var(--text);line-height:1.2}
.social-handle{font-size:0.85rem;font-weight:600;color:var(--purple)}
.social-desc{font-size:0.78rem;color:var(--muted);line-height:1.7;max-width:260px}
@media(max-width:600px){.social-card{padding:30px 22px}.social-card svg{width:52px;height:52px}}
footer p{font-size:0.75rem;color:var(--muted)}
footer p span{color:var(--purple)}

/* ── KEYBOARD FOCUS ──────────────────────────────────────────── */
/* Nothing outside a few text inputs had a focus style, so navigating by
   keyboard meant guessing where you were. :focus-visible rather than :focus
   so a mouse click does not leave a ring behind — the outline appears only
   when the browser judges the interaction to be keyboard-driven. */
:focus-visible{outline:2px solid var(--purple);outline-offset:3px;border-radius:4px}
a:focus-visible,button:focus-visible,select:focus-visible,
[role="button"]:focus-visible,[tabindex]:focus-visible{outline:2px solid var(--purple);outline-offset:3px}
/* Against the gradient pill of the active tab, purple-on-purple disappears. */
.tab.active:focus-visible,.stab.active:focus-visible{outline-color:#fff}

/* ── MOBILE TOUCH TARGETS ────────────────────────────────────── */
@media(max-width:480px){
.btn-primary,.btn-secondary{padding:16px 24px;font-size:0.9rem;width:100%;justify-content:center;border-radius:12px}
.hero-btns{flex-direction:column;width:100%;max-width:320px}
h1{font-size:2rem}
.hero{padding:150px 20px 50px}
section{padding:60px 20px}
}
</style>
</head>
<body>

<!-- NAV -->
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
      <div class="badge badge-live"><span class="pulse"></span><span data-i18n="live">LIVE</span></div>
      <div class="badge badge-health badge-health-healthy" id="health-badge" title="Checking network health…">● GHOSTDAG</div>
      <div class="badge badge-dag" title="KnightDAG: each block infers its own smallest secure K instead of a fixed epoch-wide worst case — inspired by DAGKNIGHT (Sompolinsky &amp; Sutton, 2022), evolving GHOSTDAG beyond a rigid parameter.">◆ KNIGHTDAG</div>
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
<!-- HERO -->
<section class="hero">
  <div class="hero-badge">
    <span class="pulse"></span>
    <span data-i18n="hero-badge">LIVE ON CHAIN ID 1926</span>
  </div>
  <h1 data-i18n="hero-h1">Money that belongs<br>to <span>every human</span> equally</h1>
  <p class="hero-sub" data-i18n="hero-sub">One verified human, 1,000 AEQ — no mining, no investment, no early advantage. The network measures its own inequality and publishes it live.</p>
  <div class="hero-btns">
    <a href="/download/app.apk" class="btn-primary" data-i18n="btn-download">📱 Download Aequitas App</a>
    <a href="/register" class="btn-secondary" data-i18n="btn-explorer">🌐 Open Explorer</a>
  </div>
  <div class="hero-proof">
    <span class="ok">✓</span> <span data-i18n="proof-gini">Gini</span> <span class="ok" id="gini-inline">—</span> <span data-i18n="proof-gini-note">— measured on chain, published live</span> &nbsp;·&nbsp;
    <span class="ok">✓</span> <span data-i18n="proof-gas">Zero gas fees</span> &nbsp;·&nbsp;
    <span class="ok">✓</span> <span data-i18n="proof-oss">Open source</span>
  </div>
</section>

<!-- LIVE STATS -->
<div class="stats-bar">
  <div class="stat-item">
    <div class="stat-num" id="stat-humans" style="color:#34D399">—</div>
    <div class="stat-lbl" data-i18n="stat-humans-lbl">Verified Humans</div>
  </div>
  <div class="stat-item">
    <div class="stat-num" id="stat-supply" style="color:#9B72F6">—</div>
    <div class="stat-lbl" data-i18n="stat-supply-lbl">AEQ in Circulation</div>
  </div>
  <div class="stat-item">
    <div class="stat-num" id="stat-gini" style="color:#F0B429">—</div>
    <div class="stat-lbl" data-i18n="stat-gini-lbl">Gini Coefficient</div>
  </div>
  <div class="stat-item">
    <div class="stat-num" id="stat-blocks" style="color:#22D3EE">—</div>
    <div class="stat-lbl" data-i18n="stat-blocks-lbl">Blocks Produced</div>
  </div>
</div>
<div class="stats-live"><span data-i18n="ubi-pre">Next equal split in</span> <strong id="ubi-next">—</strong> <span data-i18n="ubi-mid">· the pool holds</span> <strong id="ubi-pool">—</strong> AEQ</div>

<!-- HOW IT WORKS -->
<section>
  <div class="section-inner">
    <div class="section-label" data-i18n="how-label">How it works</div>
    <h2 data-i18n="how-h2">Three steps, about a minute</h2>
    <p class="section-sub" data-i18n="how-sub">No bank account, no crypto background, no investment. A smartphone with a fingerprint sensor is the whole requirement.</p>
    <div class="steps">
      <div class="step">
        <div class="step-num">1</div>
        <h3 data-i18n="step1-h">Scan</h3>
        <p data-i18n="step1-p">Your face is captured and checked by independent matching services against everyone already registered. The images are discarded after processing; what is kept is an encrypted template, split so that no validator holds a whole one.</p>
      </div>
      <div class="step">
        <div class="step-num">2</div>
        <h3 data-i18n="step2-h">Prove</h3>
        <p data-i18n="step2-p">A zero-knowledge proof shows the chain one thing: that you are a human who has not registered before.</p>
      </div>
      <div class="step">
        <div class="step-num">3</div>
        <h3 data-i18n="step3-h">Receive</h3>
        <p data-i18n="step3-p">1,000 AEQ arrive in your wallet within a second. Free, once per human, permanent.</p>
      </div>
    </div>
    <a class="section-link" href="/register" data-i18n="how-link">Register and claim your 1,000 AEQ →</a>
  </div>
</section>

<!-- WHY — the one number the project stands or falls on -->
<section style="background:var(--card);border-top:1px solid var(--border);border-bottom:1px solid var(--border)">
  <div class="section-inner">
    <div class="section-label" data-i18n="why-label">Why Aequitas</div>
    <h2 data-i18n="why-h2">Bitcoin's Gini is 0.85 — higher than any country</h2>
    <p class="section-sub" data-i18n="why-sub">Total supply is verified humans × 1,000 AEQ. No pre-mine, no founder allocation, no admin key, no early-adopter advantage. The chain recomputes its own inequality after every distribution and publishes it — the figure below is read from a live node, not from a slide.</p>
    <div class="gini-compact">
      <div class="gini-row aeq">
        <span class="gini-label">Aequitas</span>
        <div class="gini-bar-wrap"><div class="gini-bar" id="bar-aeq" style="width:9%"></div></div>
        <span class="gini-val" id="val-aeq" style="color:var(--gold)">—</span>
      </div>
      <div class="gini-row">
        <span class="gini-label">Bitcoin</span>
        <div class="gini-bar-wrap"><div class="gini-bar" style="width:85%;background:#F87171"></div></div>
        <span class="gini-val" style="color:#F87171">~0.85</span>
      </div>
    </div>
    <a class="section-link" href="/index/score" data-i18n="why-link">Lorenz curve, history and the country ladder →</a>
  </div>
</section>

<!-- WHERE TO GO NEXT -->
<!-- This page used to answer every question it raised, in full, before the
     visitor had asked any of them — the mechanisms, the tokenomics split and
     the architecture tables all repeated what the sections behind the tabs
     already say, at length. An overview that says everything is not an
     overview. It now states the claim and hands over. -->
<section>
  <div class="section-inner">
    <div class="section-label" data-i18n="rest-label">The rest of the site</div>
    <h2 data-i18n="rest-h2">Everything else has its own section</h2>
    <p class="section-sub" data-i18n="rest-sub">This page stops here on purpose. Each section below holds the detail — served live by this node, not written down here a second time.</p>
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
        <span class="explore-desc" data-i18n="card-exchange-d">Swap, liquidity, and where every fee goes: 40 / 30 / 20 / 10.</span>
      </a>
      <a class="explore-card" href="#social">
        <span class="explore-icon">💬</span>
        <span class="explore-name">Social</span>
        <span class="explore-desc" data-i18n="card-social-d">X and Telegram — announcements and the awkward questions.</span>
      </a>
    </div>
  </div>
</section>

<!-- SOCIAL -->
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
  <p>Aequitas Chain · Chain ID 1926 · <span>aequitas.digital</span> · <span data-i18n="foot-launched">Launched June 2026</span></p>
  <p style="margin-top:6px">"<em data-i18n="foot-quote">Money exists because people exist. Nothing more, nothing less.</em>"</p>
</footer>

<script src="/landing.js"></script>
</body>
</html>`
