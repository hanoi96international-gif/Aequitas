package keeper

const landingQuelle = `<!DOCTYPE html>
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
.hero{min-height:0;display:flex;flex-direction:column;align-items:center;justify-content:center;text-align:center;padding:170px 20px 64px;position:relative;overflow:hidden}
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
/* ── BUSINESSES ──────────────────────────────────────────────── */
.biz-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(230px,1fr));gap:14px}
.biz-card{background:var(--card);border:1px solid var(--border);border-radius:var(--radius);padding:22px;box-shadow:var(--shadow)}
.biz-card h3{font-size:1.02rem;margin-bottom:8px}
.biz-card p{font-size:0.92rem;color:var(--muted);line-height:1.5}
.biz-rules-h{font-size:1.1rem;font-weight:800;margin:34px 0 12px}
.biz-rules{border:1px solid var(--border);border-radius:var(--radius);overflow:hidden;background:var(--card)}
.biz-row{display:grid;grid-template-columns:230px 1fr;gap:16px;padding:14px 18px;border-top:1px solid var(--border)}
.biz-row:first-child{border-top:none}
.biz-k{font-weight:700;color:var(--text);font-size:0.92rem}
.biz-v{color:var(--muted);font-size:0.92rem;line-height:1.5}
.biz-live{margin-top:22px;display:flex;flex-wrap:wrap;gap:10px 22px;font-size:0.85rem;color:var(--muted)}
.biz-live strong{color:var(--gold);font-weight:700}
@media(max-width:700px){.biz-row{grid-template-columns:1fr;gap:4px}}
.eco-wrap{background:var(--card);border:1px solid var(--border);border-radius:var(--radius);padding:18px;box-shadow:var(--shadow)}
.eco-svg{width:100%;max-width:720px;display:block;margin:0 auto;font-family:Inter,system-ui,sans-serif}
.eco-svg-v{display:none;max-width:360px}
@media(max-width:600px){.eco-svg-h{display:none}.eco-svg-v{display:block}}
.eco-points{display:grid;grid-template-columns:repeat(auto-fit,minmax(240px,1fr));gap:14px;margin-top:16px}
.cmp-wrap{overflow-x:auto;border:1px solid var(--border);border-radius:var(--radius);background:var(--bg)}
.cmp-table{width:100%;border-collapse:collapse;min-width:640px;font-size:0.9rem}
.cmp-table th,.cmp-table td{padding:13px 14px;text-align:left;vertical-align:top;border-top:1px solid var(--border);line-height:1.45}
.cmp-table thead th{border-top:none;font-size:0.8rem;letter-spacing:0.5px;text-transform:uppercase;color:var(--muted)}
.cmp-table tbody th{font-weight:700;color:var(--text);width:22%}
.cmp-table td{color:var(--muted)}
.cmp-table thead .cmp-p{color:#5B8CFF}.cmp-table thead .cmp-b{color:#F5A524}.cmp-table thead .cmp-f{color:#9AA3B5}
.cmp-note{margin-top:14px;font-size:0.85rem;color:var(--muted)}
.ppl-list{list-style:none;counter-reset:ppl;display:grid;grid-template-columns:repeat(auto-fit,minmax(300px,1fr));gap:12px}
.ppl-list li{counter-increment:ppl;position:relative;background:var(--card);border:1px solid var(--border);border-radius:var(--radius);padding:18px 18px 18px 58px;color:var(--muted);font-size:0.93rem;line-height:1.5;box-shadow:var(--shadow)}
.ppl-list li::before{content:counter(ppl);position:absolute;left:16px;top:16px;width:30px;height:30px;border-radius:50%;background:var(--grad);color:#fff;font-weight:800;display:flex;align-items:center;justify-content:center;font-size:0.9rem}
.ppl-list li strong{color:var(--text)}

.toc{max-width:1100px;margin:0 auto;padding:18px 20px 0;display:flex;flex-wrap:wrap;gap:8px;align-items:center}
.toc-lbl{font-size:0.72rem;color:var(--muted);letter-spacing:2px;text-transform:uppercase;font-weight:700;margin-right:4px}
.toc a{font-size:0.82rem;color:var(--text);text-decoration:none;border:1px solid var(--border);border-radius:var(--radius-pill);padding:6px 12px;background:var(--card)}
.toc a:hover{border-color:var(--accent);color:var(--accent)}
.card-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(230px,1fr));gap:14px}
.ex-card{background:var(--card);border:1px solid var(--border);border-radius:var(--radius);padding:20px;box-shadow:var(--shadow);display:flex;flex-direction:column}
.ex-card h3{font-size:1rem;margin-bottom:8px}
.ex-card p{font-size:0.9rem;color:var(--muted);line-height:1.5;flex:1}
.ex-r{margin-top:12px;font-weight:800;font-size:0.92rem;color:var(--green)}
.ex-card.warn .ex-r{color:var(--gold)}
.note{margin-top:16px;font-size:0.88rem;color:var(--muted);line-height:1.55;max-width:760px}
.fee-wrap{max-width:520px}
.fee-table{min-width:0}
.cmp-table.fee-table tbody th,.cmp-table.fee-table thead th{width:auto;white-space:nowrap}
.lh-list{list-style:none;display:grid;gap:10px}
.lh-list li{background:var(--card);border:1px solid var(--border);border-left:3px solid var(--gold);border-radius:var(--radius);padding:14px 16px;color:var(--muted);font-size:0.92rem;line-height:1.5}
.lh-list li strong,.op-list li strong{color:var(--text)}
.rm-list{list-style:none;border-left:2px solid var(--border);margin-left:8px;display:grid;gap:18px}
.rm-list li{position:relative;padding-left:22px}
.rm-list li::before{content:"";position:absolute;left:-7px;top:6px;width:12px;height:12px;border-radius:50%;background:var(--border)}
.rm-list li.now::before{background:var(--green)}
.rm-list h3{font-size:1rem;margin-bottom:4px}
.rm-list p{font-size:0.9rem;color:var(--muted);line-height:1.5;max-width:720px}
.op-list{list-style:disc;padding-left:20px;display:grid;gap:8px;color:var(--muted);font-size:0.92rem;line-height:1.5;max-width:820px}
.faq details{background:var(--card);border:1px solid var(--border);border-radius:var(--radius);padding:14px 18px;margin-bottom:10px}
.faq summary{cursor:pointer;font-weight:700;font-size:0.96rem;color:var(--text)}
.faq details p{margin-top:10px;color:var(--muted);font-size:0.92rem;line-height:1.55}
.alt{background:var(--card);border-top:1px solid var(--border);border-bottom:1px solid var(--border)}
.ov-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(230px,1fr));gap:14px}
.ov-card{display:flex;flex-direction:column;background:var(--card);border:1px solid var(--border);border-radius:var(--radius);padding:22px;box-shadow:var(--shadow);text-decoration:none;color:inherit;transition:border-color 0.2s,transform 0.2s}
.ov-card:hover{border-color:var(--accent);transform:translateY(-2px)}
.ov-icon{font-size:1.6rem;margin-bottom:10px}
.ov-card h3{font-size:1.08rem;margin-bottom:8px;color:var(--text)}
.ov-card p{font-size:0.9rem;color:var(--muted);line-height:1.5;flex:1}
.ov-more{margin-top:14px;font-size:0.86rem;font-weight:700;color:var(--accent)}
.page-head{padding:150px 20px 36px;border-bottom:1px solid var(--border)}
.page-head h1{font-size:clamp(1.9rem,5vw,2.8rem);line-height:1.15;font-weight:800;letter-spacing:-0.02em;margin:10px 0 12px}
.page-head .section-sub{margin-bottom:22px}
.pg-back{font-size:0.85rem;color:var(--accent);text-decoration:none;font-weight:600}
.page-head .toc{padding:0;margin:0}
.lh-details summary{cursor:pointer;margin:34px 0 12px}
.lh-details[open] summary{margin-bottom:12px}
@media(max-width:600px){.page-head{padding:128px 16px 28px}}
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

/* ── NEUER AUFBAU (25.09.2026) ───────────────────────────────── */
.tab-biz{color:var(--gold);border-color:rgba(245,165,36,0.35)}
.tab-biz:hover{color:var(--gold);background:rgba(245,165,36,0.08)}
.tab-biz.active{background:linear-gradient(135deg,#F5A524,#F57C24);box-shadow:0 0 24px rgba(245,165,36,0.2)}
.tab-sep{flex:0 0 1px;align-self:stretch;margin:6px 6px;background:var(--border)}
.tab-tool{font-weight:500;font-size:0.76rem;padding:10px 12px}
.btn-biz{border-color:rgba(245,165,36,0.45);color:var(--gold)}
.btn-biz:hover{background:rgba(245,165,36,0.1);border-color:var(--gold)}
.band-biz{background:linear-gradient(180deg,rgba(245,165,36,0.07),rgba(245,165,36,0.02));border-top:1px solid rgba(245,165,36,0.25);border-bottom:1px solid rgba(245,165,36,0.25)}
.band-biz .section-label{color:var(--gold)}
.band-biz h2{max-width:760px}
.band-biz .biz-card{border-color:rgba(245,165,36,0.2)}
.fb-example{margin-top:22px;max-width:760px}
.fb-ex-lbl{font-size:0.72rem;color:var(--gold);letter-spacing:2px;text-transform:uppercase;font-weight:700;margin-bottom:8px}
.ex-inline{border-left:3px solid var(--gold)}
.band-btns{display:flex;flex-wrap:wrap;gap:12px;margin-top:28px}
.band-btns .btn-primary{background:var(--gold);color:#1a1205}
.steps-4{grid-template-columns:repeat(4,1fr)}
@media(max-width:1000px){.steps-4{grid-template-columns:repeat(2,1fr)}}
@media(max-width:700px){.steps-4{grid-template-columns:1fr}}
.status-card{max-width:820px;background:var(--card);border:1px solid var(--border);border-left:3px solid var(--warn);border-radius:var(--radius);padding:26px 26px 22px;box-shadow:var(--shadow)}
.status-card h2{font-size:clamp(1.3rem,3vw,1.6rem)}
.status-card p{color:var(--muted);font-size:0.95rem;line-height:1.6}
.status-card .section-label{color:var(--warn)}
.status-card .section-link{margin-top:16px}
.page-head .pg-btns{display:flex;flex-wrap:wrap;gap:12px;margin:4px 0 22px}
.page-biz{background:linear-gradient(180deg,rgba(245,165,36,0.08),transparent)}
.page-biz .section-label,.page-biz .pg-back{color:var(--gold)}
.page-biz .btn-primary{background:var(--gold);color:#1a1205}
@media(max-width:480px){.band-btns{flex-direction:column}.page-head .pg-btns{flex-direction:column}}
#business .section-label,#join .section-label,#bexamples .section-label,#rules .section-label,#loopholes .section-label{color:var(--gold)}
#join .step-num{background:linear-gradient(135deg,#F5A524,#F57C24)}
.age-details summary{cursor:pointer;list-style-position:outside}
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
      <div class="badge badge-health badge-health-healthy" id="health-badge" title="Checking network health…">● GHOSTDAG</div>
      <a href="/register" class="nav-cta" data-i18n="nav-register">Register</a>
    </div>
  </div>
  <div class="tabs">
    <a href="/" class="tab active" data-i18n="nav-home">Home</a>
    <a href="/people" class="tab" data-i18n="nav-people">For people</a>
    <a href="/business" class="tab tab-biz" data-i18n="nav-biz">For businesses</a>
    <a href="/economy" class="tab" data-i18n="nav-how">How it works</a>
    <a href="/roadmap" class="tab" data-i18n="nav-road">Roadmap &amp; FAQ</a>
    <span class="tab-sep" aria-hidden="true"></span>
    <a href="/explorer" class="tab tab-tool" data-i18n="nav-explorer">Explorer</a>
    <a href="/index/score" class="tab tab-tool" data-i18n="nav-equality">Equality</a>
    <a href="/network" class="tab tab-tool" data-i18n="nav-network">Network</a>
    <a href="/exchange" class="tab tab-tool" data-i18n="nav-exchange">Exchange</a>
  </div>
</nav>

<main>
<section class="hero">
  <div class="hero-badge">
    <span class="pulse"></span>
    <span data-i18n="hero-badge">Phase 1 · Chain ID 1926</span>
  </div>
  <h1 data-i18n="hero-h1">Money that belongs<br>to <span>every human</span> equally</h1>
  <p class="hero-sub" data-i18n="hero-sub">Every verified person receives 1,000 AEQ and a basic income every day. Businesses take payments without card fees and pay wages free of charge.</p>
  <div class="hero-btns">
    <a href="/register" class="btn-primary" data-i18n="btn-register">Register now</a>
    <a href="/business" class="btn-secondary btn-biz" data-i18n="btn-biz">For businesses</a>
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

<section id="forbiz" class="band-biz">
  <div class="section-inner">
    <div class="section-label" data-i18n="fb-label">Aequitas for businesses</div>
    <h2 data-i18n="fb-h2">Take payments without fees. Pay wages without deductions.</h2>
    <p class="section-sub" data-i18n="fb-sub">Every verified person receives a basic income every day and looks for places to spend it. Businesses that accept AEQ win these customers, pay nothing per payment and pass the money on as wages and purchases.</p>
    <div class="biz-grid">
      <div class="biz-card"><h3 data-i18n="biz-b1-h">No card fees</h3><p data-i18n="biz-b1-p">A payment costs the business nothing. Customers spend their first 1,000 AEQ each month without any fee.</p></div>
      <div class="biz-card"><h3 data-i18n="biz-b2-h">Fee-free wages</h3><p data-i18n="biz-b2-p">Wages paid in AEQ cost nothing. Everyone can exchange up to 3,000 AEQ a month into euros or dollars without a levy.</p></div>
      <div class="biz-card"><h3 data-i18n="biz-b3-h">No growth cap</h3><p data-i18n="biz-b3-p">People can hold at most 25,000 AEQ; businesses have no fixed limit. Up to one and a half months' turnover never costs anything.</p></div>
      <div class="biz-card"><h3 data-i18n="biz-b4-h">Paid in seconds</h3><p data-i18n="biz-b4-p">Money arrives in seconds. No chargebacks, no waiting for settlement.</p></div>
    </div>
    <div class="fb-example">
      <div class="fb-ex-lbl" data-i18n="fb-ex-label">Example</div>
      <div class="ex-card ex-inline"><h3 data-i18n="ex-cafe-h">A café</h3><p data-i18n="ex-cafe-p">It takes in 3,000 AEQ a month and pays 1,500 in wages, 800 to its supplier and 600 to the owner. It holds far less than one and a half months' turnover, so there is no idle-money levy. Only the payment to the supplier costs 0.1 %.</p><div class="ex-r" data-i18n="ex-cafe-r">pays 0.8 AEQ a month</div></div>
    </div>
    <div class="band-btns">
      <a href="/business" class="btn-primary" data-i18n="fb-btn">Everything for businesses →</a>
      <a href="/business#join" class="btn-secondary" data-i18n="fb-btn2">How to take part</a>
    </div>
  </div>
</section>

<section id="forppl">
  <div class="section-inner">
    <div class="section-label" data-i18n="ppl-label">For people</div>
    <h2 data-i18n="fp-h2">Your fair share, every day</h2>
    <p class="section-sub" data-i18n="fp-sub">Every verified person receives 1,000 AEQ once and then, every day, an equal share of everything the network collects.</p>
    <ol class="ppl-list">
      <li data-i18n="ppl-1"><strong>Your fair share is untouchable.</strong> The first 1,000 AEQ never pay a levy.</li>
      <li data-i18n="ppl-2"><strong>Everyday life costs nothing.</strong> The first 1,000 AEQ you spend each month are free of fees.</li>
      <li data-i18n="ppl-4"><strong>Saving is allowed.</strong> Up to 5,000 AEQ your savings lose nothing.</li>
    </ol>
    <a class="section-link" href="/people" data-i18n="fp-btn">All six promises, the basic income and examples →</a>
  </div>
</section>

<section id="how">
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
    <a class="section-link" href="/register" data-i18n="how-link">Register and claim your 1,000 AEQ →</a>
  </div>
</section>

<section id="status">
  <div class="section-inner">
    <div class="status-card">
      <div class="section-label" data-i18n="op-label">Honestly</div>
      <h2 data-i18n="st-h2">Phase 1: a public test</h2>
      <p data-i18n="st-p">Aequitas runs, but it is not finished. AEQ can only be exchanged into the test currency tUSD, the legal review under the EU crypto regulation is still pending, and the face check has named limits. The economy rules take effect on 1 October 2026.</p>
      <a class="section-link" href="/roadmap" data-i18n="st-link">Roadmap and what is still open →</a>
    </div>
  </div>
</section>

<section id="social">
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

<section id="economy">
  <div class="section-inner">
    <div class="section-label" data-i18n="eco-label">The economy</div>
    <h2 data-i18n="eco-h2">One cycle, three roles</h2>
    <p class="section-sub" data-i18n="eco-sub">People receive the basic income and spend it. Businesses earn it and pass it on as wages and purchases. Whatever sits idle or leaves the network flows back into the basic income, and from there equally to everyone.</p>
    <div class="eco-wrap">
    <svg class="eco-svg eco-svg-h" viewBox="0 0 720 300" role="img" aria-label="One cycle, three roles" xmlns="http://www.w3.org/2000/svg">
      <defs>
        <marker id="eco-a1" markerWidth="9" markerHeight="7" refX="7" refY="3.5" orient="auto"><polygon points="0 0, 9 3.5, 0 7" fill="#5B8CFF"/></marker>
        <marker id="eco-a2" markerWidth="9" markerHeight="7" refX="7" refY="3.5" orient="auto"><polygon points="0 0, 9 3.5, 0 7" fill="#3DDC97"/></marker>
        <marker id="eco-a3" markerWidth="9" markerHeight="7" refX="7" refY="3.5" orient="auto"><polygon points="0 0, 9 3.5, 0 7" fill="#F5A524"/></marker>
      </defs>
      <rect x="30" y="50" width="190" height="90" rx="14" fill="rgba(91,140,255,0.12)" stroke="#5B8CFF" stroke-width="1.5"/>
      <text x="125" y="92" text-anchor="middle" font-size="26">👤</text>
      <text x="125" y="122" text-anchor="middle" font-size="16" font-weight="800" fill="#E8EAF0" data-i18n="eco-svg-people">People</text>
      <rect x="500" y="50" width="190" height="90" rx="14" fill="rgba(245,165,36,0.10)" stroke="#F5A524" stroke-width="1.5"/>
      <text x="595" y="92" text-anchor="middle" font-size="26">🏪</text>
      <text x="595" y="122" text-anchor="middle" font-size="16" font-weight="800" fill="#E8EAF0" data-i18n="eco-svg-biz">Businesses</text>
      <rect x="265" y="215" width="190" height="70" rx="14" fill="rgba(61,220,151,0.12)" stroke="#3DDC97" stroke-width="1.5"/>
      <text x="360" y="257" text-anchor="middle" font-size="16" font-weight="800" fill="#E8EAF0" data-i18n="eco-svg-ubi">Basic income</text>
      <line x1="222" y1="78" x2="494" y2="78" stroke="#5B8CFF" stroke-width="2" marker-end="url(#eco-a1)"/>
      <text x="360" y="68" text-anchor="middle" font-size="13" fill="#9AA3B5" data-i18n="eco-svg-buy">purchases</text>
      <line x1="498" y1="112" x2="226" y2="112" stroke="#F5A524" stroke-width="2" marker-end="url(#eco-a3)"/>
      <text x="360" y="132" text-anchor="middle" font-size="13" fill="#9AA3B5" data-i18n="eco-svg-wages">wages</text>
      <path d="M170 142 Q210 200 290 218" fill="none" stroke="#9AA3B5" stroke-width="1.5" stroke-dasharray="5,4" marker-end="url(#eco-a2)"/>
      <path d="M550 142 Q510 200 430 218" fill="none" stroke="#9AA3B5" stroke-width="1.5" stroke-dasharray="5,4" marker-end="url(#eco-a2)"/>
      <text x="360" y="190" text-anchor="middle" font-size="12" fill="#9AA3B5" data-i18n="eco-svg-levies">idle money · exit · fees</text>
      <path d="M263 262 Q60 262 70 146" fill="none" stroke="#3DDC97" stroke-width="2.5" marker-end="url(#eco-a2)"/>
      <text x="170" y="292" text-anchor="middle" font-size="12" font-weight="700" fill="#3DDC97" data-i18n="eco-svg-daily">daily, equal for all</text>
    </svg>
    <svg class="eco-svg eco-svg-v" viewBox="0 0 360 480" role="img" aria-label="One cycle, three roles" xmlns="http://www.w3.org/2000/svg">
      <defs>
        <marker id="eco-b1" markerWidth="9" markerHeight="7" refX="7" refY="3.5" orient="auto"><polygon points="0 0, 9 3.5, 0 7" fill="#5B8CFF"/></marker>
        <marker id="eco-b2" markerWidth="9" markerHeight="7" refX="7" refY="3.5" orient="auto"><polygon points="0 0, 9 3.5, 0 7" fill="#3DDC97"/></marker>
        <marker id="eco-b3" markerWidth="9" markerHeight="7" refX="7" refY="3.5" orient="auto"><polygon points="0 0, 9 3.5, 0 7" fill="#F5A524"/></marker>
      </defs>
      <rect x="90" y="20" width="180" height="80" rx="14" fill="rgba(91,140,255,0.12)" stroke="#5B8CFF" stroke-width="1.5"/>
      <text x="180" y="55" text-anchor="middle" font-size="24">👤</text>
      <text x="180" y="85" text-anchor="middle" font-size="16" font-weight="800" fill="#E8EAF0" data-i18n="eco-svg-people">People</text>
      <rect x="90" y="200" width="180" height="80" rx="14" fill="rgba(245,165,36,0.10)" stroke="#F5A524" stroke-width="1.5"/>
      <text x="180" y="235" text-anchor="middle" font-size="24">🏪</text>
      <text x="180" y="265" text-anchor="middle" font-size="16" font-weight="800" fill="#E8EAF0" data-i18n="eco-svg-biz">Businesses</text>
      <rect x="90" y="385" width="180" height="70" rx="14" fill="rgba(61,220,151,0.12)" stroke="#3DDC97" stroke-width="1.5"/>
      <text x="180" y="426" text-anchor="middle" font-size="16" font-weight="800" fill="#E8EAF0" data-i18n="eco-svg-ubi">Basic income</text>
      <line x1="150" y1="102" x2="150" y2="193" stroke="#5B8CFF" stroke-width="2" marker-end="url(#eco-b1)"/>
      <text x="142" y="152" text-anchor="end" font-size="13" fill="#9AA3B5" data-i18n="eco-svg-buy">purchases</text>
      <line x1="210" y1="198" x2="210" y2="107" stroke="#F5A524" stroke-width="2" marker-end="url(#eco-b3)"/>
      <text x="218" y="152" text-anchor="start" font-size="13" fill="#9AA3B5" data-i18n="eco-svg-wages">wages</text>
      <line x1="140" y1="282" x2="140" y2="379" stroke="#9AA3B5" stroke-width="1.5" stroke-dasharray="5,4" marker-end="url(#eco-b2)"/>
      <path d="M272 60 C 358 60, 358 420, 276 420" fill="none" stroke="#9AA3B5" stroke-width="1.5" stroke-dasharray="5,4" marker-end="url(#eco-b2)"/>
      <text x="148" y="336" text-anchor="start" font-size="10" fill="#9AA3B5" data-i18n="eco-svg-levies">idle money · exit · fees</text>
      <path d="M88 420 C 8 420, 8 60, 84 60" fill="none" stroke="#3DDC97" stroke-width="2.5" marker-end="url(#eco-b2)"/>
      <text x="24" y="240" text-anchor="middle" font-size="12" font-weight="700" fill="#3DDC97" transform="rotate(-90 24 240)" data-i18n="eco-svg-daily">daily, equal for all</text>
    </svg>
    </div>
    <div class="eco-points">
      <div class="biz-card"><h3 data-i18n="eco-p1-h">Money is created only for people</h3><p data-i18n="eco-p1-p">Every verified person receives 1,000 AEQ once. Nobody else creates money: not businesses, not validators, not the founders.</p></div>
      <div class="biz-card"><h3 data-i18n="eco-p2-h">Circulation is rewarded</h3><p data-i18n="eco-p2-p">Spending, paying wages and paying suppliers costs little or nothing. Money that sits idle beyond clear limits pays a small monthly levy.</p></div>
      <div class="biz-card"><h3 data-i18n="eco-p3-h">Every levy returns to everyone</h3><p data-i18n="eco-p3-p">Transfer fees, idle-money levies and exit levies go 100 % to the basic income, paid out every day in equal shares to every verified person.</p></div>
    </div>
  </div>
</section>

<section id="compare">
  <div class="section-inner">
    <div class="section-label" data-i18n="cmp-label">Person or business?</div>
    <h2 data-i18n="cmp-h2">Three kinds of account, one set of rules</h2>
    <p class="section-sub" data-i18n="cmp-sub">Nobody checks what you are. The rules make hoarding expensive in every form and passing money on cheap, so everyone chooses the account that really fits.</p>
    <div class="cmp-wrap">
      <table class="cmp-table">
        <thead><tr><th scope="col" data-i18n="cmp-col-rule">Rule</th><th scope="col" class="cmp-p">👤 <span data-i18n="cmp-col-person">Person</span></th><th scope="col" class="cmp-b">🏪 <span data-i18n="cmp-col-biz">Business</span></th><th scope="col" class="cmp-f">🔑 <span data-i18n="cmp-col-free">Other address</span></th></tr></thead>
        <tbody>
        <tr><th scope="row" data-i18n="cmp-r1">Basic income and vote</th><td data-i18n="cmp-yes">yes</td><td data-i18n="cmp-no">no</td><td data-i18n="cmp-no">no</td></tr>
        <tr><th scope="row" data-i18n="cmp-r2">Maximum holding</th><td data-i18n="cmp-v-25k">25,000 AEQ (25×)</td><td data-i18n="cmp-nolimit">no fixed limit</td><td data-i18n="cmp-v-1k">1,000 AEQ (1×)</td></tr>
        <tr><th scope="row" data-i18n="cmp-r3">Idle money</th><td data-i18n="cmp-r3-p">0.5 % a month, only above 5,000 AEQ</td><td data-i18n="cmp-r3-b">up to 1.5 months' turnover free, then 0.5 % a month; above 3 months' turnover 2 %</td><td data-i18n="cmp-r3-f">1 % a month</td></tr>
        <tr><th scope="row" data-i18n="cmp-r4">Sending money</th><td data-i18n="cmp-r4-p">first 1,000 AEQ a month free, then 0.1 %</td><td data-i18n="cmp-r4-b">to people free, otherwise 0.1 %</td><td data-i18n="cmp-r4-f">0.1 %</td></tr>
        <tr><th scope="row" data-i18n="cmp-r5">Exchange to euro or dollar</th><td data-i18n="cmp-r5-p">3,000 AEQ a month free, then 2 %</td><td data-i18n="cmp-r5-b">2 %</td><td data-i18n="cmp-r5-b">2 %</td></tr>
        <tr><th scope="row" data-i18n="cmp-r6">Who opens it</th><td data-i18n="cmp-r6-p">every verified person, once</td><td data-i18n="cmp-r6-b">one to ten verified people; at most 3 per person</td><td data-i18n="cmp-r6-f">anyone</td></tr>
        </tbody>
      </table>
    </div>
    <p class="cmp-note" data-i18n="cmp-note">These rules apply from 1 October 2026. Every levy goes 100 % to the basic income.</p>
    <div class="sybil-blurb" data-i18n="fs-note"><strong>Every limit is a multiple of the fair share.</strong> 1,000 AEQ is what the average person holds, because the money supply is always people × 1,000 AEQ. So 2,000 = 2×, 3,000 = 3×, 5,000 = 5× and 25,000 = 25× the fair share. The limits are not tied to the dollar: if AEQ gains or loses value, everyone's fair share changes with it and the limits keep their meaning.</div>
  </div>
</section>

<section id="fairness">
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

<section id="people">
  <div class="section-inner">
    <div class="section-label" data-i18n="ppl-label">For people</div>
    <h2 data-i18n="ppl-h2">Six promises to every person</h2>
    <p class="section-sub" data-i18n="ppl-sub">The fairest money has to feel fair first to the person who has little.</p>
    <ol class="ppl-list">
      <li data-i18n="ppl-1"><strong>Your fair share is untouchable.</strong> The first 1,000 AEQ never pay a levy.</li>
      <li data-i18n="ppl-2"><strong>Everyday life costs nothing.</strong> The first 1,000 AEQ you spend each month are free of fees.</li>
      <li data-i18n="ppl-3"><strong>Wages are wages.</strong> Up to 3,000 AEQ a month can be exchanged into euros or dollars without a levy, whatever the money comes from.</li>
      <li data-i18n="ppl-4"><strong>Saving is allowed.</strong> Up to 5,000 AEQ your savings lose nothing.</li>
      <li data-i18n="ppl-5"><strong>Those who have more contribute more.</strong> Savings above 5,000 AEQ pay 0.5 % a month; the 25,000 AEQ limit stays.</li>
      <li data-i18n="ppl-6"><strong>People always pay less than businesses</strong> for holding and exiting, and everything anyone pays returns to all people equally.</li>
    </ol>
    <div class="biz-rules-h" data-i18n="fee-h">Transfer fee in detail</div>
    <div class="cmp-wrap fee-wrap"><table class="cmp-table fee-table"><thead><tr><th scope="col" data-i18n="fee-col-bal">Payment</th><th scope="col" data-i18n="fee-col-fee">Fee</th></tr></thead><tbody>
      <tr><th scope="row" data-i18n="fee-r1">People: first 1,000 AEQ a month</th><td data-i18n="fee-r1-v">free</td></tr>
      <tr><th scope="row" data-i18n="fee-r2">People: above that</th><td data-i18n="fee-r2-v">0.1 %</td></tr>
      <tr><th scope="row" data-i18n="fee-r3">Businesses to people (wages)</th><td data-i18n="fee-r3-v">free</td></tr>
      <tr><th scope="row" data-i18n="fee-r4">Between businesses, other addresses</th><td data-i18n="fee-r4-v">0.1 %</td></tr>
    </tbody></table></div>
    <p class="note" data-i18n="fee-note">The fee is added on top: the recipient always gets the full amount, so a price of 10 AEQ brings the shop exactly 10 AEQ. There are no surcharges for large balances: wealth is already limited by the 25,000 AEQ cap and the levy on savings above 5,000 AEQ. These rules apply from 1 October 2026.</p>
  </div>
</section>

<section id="ubi">
  <div class="section-inner">
    <div class="section-label" data-i18n="ubi-label">Basic income</div>
    <h2 data-i18n="ubi-h2">Every day, equal shares for everyone</h2>
    <p class="section-sub" data-i18n="ubi-sub">No money is created for the basic income. It is paid only from what the network collects, and everything collected is paid out.</p>
    <div class="card-grid">
      <div class="biz-card"><h3 data-i18n="ubi-c1-h">Start: 1,000 AEQ</h3><p data-i18n="ubi-c1-p">Every verified person receives 1,000 AEQ once when registering: the fair share. The money supply is always verified people × 1,000 AEQ.</p></div>
      <div class="biz-card"><h3 data-i18n="ubi-c2-h">Daily at 20:00</h3><p data-i18n="ubi-c2-p">Every day at 20:00 (Berlin time) the pool is split into equal shares among all verified people. Afterwards it starts again at zero.</p></div>
      <div class="biz-card"><h3 data-i18n="ubi-c3-h">Where it comes from</h3><p data-i18n="ubi-c3-p">Transfer fees (100 %), 30 % of swap fees, the idle-money levy, the 2 % exit levy and anything above the 25,000 AEQ limit.</p></div>
      <div class="biz-card"><h3 data-i18n="ubi-c4-h">Who receives it</h3><p data-i18n="ubi-c4-p">Only verified people. Businesses, validators, founders and other addresses receive nothing from it.</p></div>
    </div>
    <p class="note" data-i18n="ubi-note">How large the daily share is depends on how much money moves: the more AEQ circulates, the more flows back to everyone.</p>
  </div>
</section>

<section id="examples">
  <div class="section-inner">
    <div class="section-label" data-i18n="ex-label">Examples</div>
    <h2 data-i18n="ex-h2">What it means in real numbers</h2>
    <p class="section-sub" data-i18n="ex-sub">Four situations, calculated with the rules that apply from 1 October 2026.</p>
    <div class="card-grid">
      <div class="ex-card"><h3 data-i18n="ex-anna-h">Anna lives on the basic income</h3><p data-i18n="ex-anna-p">She has 1,200 AEQ and spends 800 AEQ a month. No fee (below 1,000 a month), no levy (below 5,000), no exit levy.</p><div class="ex-r" data-i18n="ex-anna-r">pays 0 AEQ a month</div></div>
      <div class="ex-card"><h3 data-i18n="ex-ben-h">Ben works in a café</h3><p data-i18n="ex-ben-p">He earns 2,000 AEQ in wages, spends 1,500 AEQ and exchanges 1,000 AEQ into euros for his rent. Fee on the 500 AEQ above his free amount: 0.5 AEQ. Normal swap fee: 1 AEQ. No exit levy: up to 3,000 AEQ a month are free.</p><div class="ex-r" data-i18n="ex-ben-r">pays 1.5 AEQ a month</div></div>
      <div class="ex-card"><h3 data-i18n="ex-clara-h">Clara has 20,000 AEQ</h3><p data-i18n="ex-clara-p">She spends 3,000 AEQ a month and exchanges 5,000 AEQ. Transfers: 2,000 × 0.1 % = 2 AEQ. Idle money: 15,000 × 0.5 % = 75 AEQ. Exchange: 2,000 × 2 % = 40 AEQ (the first 3,000 are free). All of it goes to the basic income, so also to Anna and Ben.</p><div class="ex-r" data-i18n="ex-clara-r">pays 117 AEQ a month</div></div>
      <div class="ex-card warn"><h3 data-i18n="ex-hoard-h">Someone wants to hoard 100,000 AEQ</h3><p data-i18n="ex-hoard-p">As a person: impossible, the limit is 25,000 AEQ. On 100 other addresses of 1,000 AEQ: 1 % a month = 1,000 AEQ a month. As a “business” without turnover: 98,000 × 2 % = 1,960 AEQ a month.</p><div class="ex-r" data-i18n="ex-hoard-r">Hoarding pays off in no form (about 24 % a year)</div></div>
    </div>
  </div>
</section>

<section id="business">
  <div class="section-inner">
    <div class="section-label" data-i18n="biz-label">For businesses</div>
    <h2 data-i18n="biz-h2">Accept AEQ, pass it on, pay nothing</h2>
    <p class="section-sub" data-i18n="biz-sub">Businesses may accept, hold and spend AEQ. The rules make money flow through them and back to people: leaving it idle costs, passing it on is free.</p>
    <div class="biz-grid">
      <div class="biz-card"><h3 data-i18n="biz-b1-h">No card fees</h3><p data-i18n="biz-b1-p">A payment costs the business nothing. Customers spend their first 1,000 AEQ each month without any fee.</p></div>
      <div class="biz-card"><h3 data-i18n="biz-b2-h">Fee-free wages</h3><p data-i18n="biz-b2-p">Wages paid in AEQ cost nothing. Everyone can exchange up to 3,000 AEQ a month into euros or dollars without a levy.</p></div>
      <div class="biz-card"><h3 data-i18n="biz-b3-h">No growth cap</h3><p data-i18n="biz-b3-p">People can hold at most 25,000 AEQ; businesses have no fixed limit. Up to one and a half months' turnover never costs anything.</p></div>
      <div class="biz-card"><h3 data-i18n="biz-b4-h">Paid in seconds</h3><p data-i18n="biz-b4-p">Money arrives in seconds. No chargebacks, no waiting for settlement.</p></div>
    </div>
  </div>
</section>

<section id="join">
  <div class="section-inner">
    <div class="section-label" data-i18n="jn-label">How to take part</div>
    <h2 data-i18n="jn-h2">Four steps to your first payment</h2>
    <p class="section-sub" data-i18n="jn-sub">What a bakery, a café or a workshop needs to accept AEQ.</p>
    <div class="steps steps-4">
      <div class="step"><div class="step-num">1</div><h3 data-i18n="jn-1-h">Open a business account</h3><p data-i18n="jn-1-p">A verified person and the business wallet sign together. Up to 10 responsible people per business, at most 3 businesses per person. No registry office, no gatekeeper.</p></div>
      <div class="step"><div class="step-num">2</div><h3 data-i18n="shop-1-h">Checkout by QR code</h3><p data-i18n="shop-1-p">Enter the amount, show the QR code, the customer scans and pays, and gets a receipt. Prices can be shown in AEQ or as the euro equivalent at the current rate.</p></div>
      <div class="step"><div class="step-num">3</div><h3 data-i18n="jn-3-h">Pass it on</h3><p data-i18n="jn-3-p">Wages to people cost nothing, payments to suppliers 0.1 %. Up to one and a half months' turnover never pays a levy.</p></div>
      <div class="step"><div class="step-num">4</div><h3 data-i18n="shop-2-h">Accounting export</h3><p data-i18n="shop-2-p">Every payment with date, amount in AEQ and euro value at the time of payment, as a CSV file for the tax adviser.</p></div>
    </div>
    <div class="sybil-blurb" data-i18n="shop-status"><strong>Status:</strong> the rules on the chain are built and apply from 1 October 2026. Checkout mode and export in the app are in progress.</div>
  </div>
</section>

<section id="bexamples">
  <div class="section-inner">
    <div class="section-label" data-i18n="bx-label">Costs</div>
    <h2 data-i18n="bx-h2">What it costs a business</h2>
    <p class="section-sub" data-i18n="bx-sub">Two businesses, calculated with the rules that apply from 1 October 2026.</p>
    <div class="card-grid">
      <div class="ex-card"><h3 data-i18n="ex-cafe-h">A café</h3><p data-i18n="ex-cafe-p">It takes in 3,000 AEQ a month and pays 1,500 in wages, 800 to its supplier and 600 to the owner. It holds far less than one and a half months' turnover, so there is no idle-money levy. Only the payment to the supplier costs 0.1 %.</p><div class="ex-r" data-i18n="ex-cafe-r">pays 0.8 AEQ a month</div></div>
      <div class="ex-card warn"><h3 data-i18n="ex-market-h">A supermarket that hoards</h3><p data-i18n="ex-market-p">40,000 AEQ of purchases every month, but 200,000 AEQ stay in the account. Free up to 60,000 (1.5 months' turnover), 0.5 % on the next 60,000 up to 3 months' turnover, 2 % on the 80,000 above. A normal reserve of two months would cost only 100 AEQ a month.</p><div class="ex-r" data-i18n="ex-market-r">pays 1,900 AEQ a month into the basic income</div></div>
    </div>
  </div>
</section>

<section id="rules">
  <div class="section-inner">
    <div class="section-label" data-i18n="ru-label">Rules</div>
    <h2 data-i18n="ru-h2">The rules for business accounts</h2>
    <p class="section-sub" data-i18n="ru-sub">They apply from 1 October 2026. Every levy goes 100 % to the basic income, equally to every person.</p>
    <div class="biz-rules">
      <div class="biz-row"><span class="biz-k" data-i18n="biz-r1-k">Idle money</span><span class="biz-v" data-i18n="biz-r1-v">Up to 1.5 months' turnover (at least 2,000 AEQ): free · up to 3 months' turnover: 0.5 % per month on the part above · beyond that: 2 % per month</span></div>
      <div class="biz-row"><span class="biz-k" data-i18n="biz-r2-k">What counts as turnover</span><span class="biz-v" data-i18n="biz-r2-v">The average of the last 90 days. Purchases count up to 1,000 AEQ per person and month; between businesses only the surplus counts; wages, your own payments and exchanges into AEQ do not count.</span></div>
      <div class="biz-row"><span class="biz-k" data-i18n="biz-r3-k">Exit to euro or dollar</span><span class="biz-v" data-i18n="biz-r3-v">2 % levy. People: 3,000 AEQ a month are free</span></div>
      <div class="biz-row"><span class="biz-k" data-i18n="biz-r4-k">Business to business</span><span class="biz-v" data-i18n="biz-r4-v">0.1 %</span></div>
      <div class="biz-row"><span class="biz-k" data-i18n="biz-r5-k">Where it goes</span><span class="biz-v" data-i18n="biz-r5-v">Every levy goes 100 % to the basic income, equally to every person</span></div>
      <div class="biz-row"><span class="biz-k" data-i18n="biz-r6-k">Public</span><span class="biz-v" data-i18n="biz-r6-v">Name, category and number of responsible people are visible in the explorer. An account can only be closed when it is empty; the balance is paid out to people beforehand, free of fees.</span></div>
    </div>
    <details class="age-details"><summary class="biz-rules-h" data-i18n="age-h">What counts as turnover?</summary>
    <div class="biz-rules">
      <div class="biz-row"><span class="biz-k" data-i18n="age-r1-k">Purchases by people</span><span class="biz-v" data-i18n="age-r1-v">up to 1,000 AEQ per person and month</span></div>
      <div class="biz-row"><span class="biz-k" data-i18n="age-r2-k">Payments from other businesses</span><span class="biz-v" data-i18n="age-r2-v">only the surplus: income from businesses minus payments to businesses</span></div>
      <div class="biz-row"><span class="biz-k" data-i18n="age-r3-k">Businesses with shared responsible people</span><span class="biz-v" data-i18n="age-r3-v">do not count for each other</span></div>
      <div class="biz-row"><span class="biz-k" data-i18n="age-r4-k">Wages, withdrawals, your own payments, other addresses, exchange into AEQ</span><span class="biz-v" data-i18n="age-r4-v">do not count</span></div>
    </div>
    <p class="note" data-i18n="age-note">Turnover is the average of the last 90 days. The surplus rule stops circles: if three firms send each other money, each has as much coming in as going out, and the allowance does not grow. In the first 30 days of a business there is no idle-money levy.</p>
    </details>
    <div class="biz-live"><span><span data-i18n="biz-live-from">Rules apply from</span> <strong id="biz-from">—</strong></span><span><span data-i18n="biz-live-count">Registered businesses</span>: <strong id="biz-count">—</strong></span></div>
    <a class="section-link" href="https://github.com/hanoi96international-gif/Aequitas/blob/main/docs/UNTERNEHMEN_KONZEPT.md" rel="noopener" data-i18n="biz-link">Read the full concept →</a>
  </div>
</section>

<section id="loopholes">
  <div class="section-inner">
    <div class="section-label" data-i18n="lh-label">Protection against abuse</div>
    <h2 data-i18n="lh-h2">Who is a business? We don't need to know.</h2>
    <p class="section-sub" data-i18n="lh-sub">A decentralised network cannot check whether a real company stands behind an account, and it should not have to: no registry, no authority, no gatekeeper. Instead, hoarding is expensive in every form and passing money on is cheap in every form. Registering as a business only pays off for those whose money really flows.</p>
    <details class="lh-details"><summary class="biz-rules-h" data-i18n="lh-list-h">Every workaround we found, and why it fails</summary>
    <ul class="lh-list">
      <li data-i18n="lh-1"><strong>Registering as a business to get around the 25,000 limit.</strong> Without real turnover a business pays 2 % a month on everything above 2,000 AEQ, four times as much as a person.</li>
      <li data-i18n="lh-2"><strong>Sending money in circles between your own or friendly firms to inflate turnover.</strong> Between businesses only the surplus counts, and your own firms do not count for each other: a circle adds nothing.</li>
      <li data-i18n="lh-3"><strong>Paying money in yourself or through the owner.</strong> Payments from a business's own responsible people do not count as turnover.</li>
      <li data-i18n="lh-4"><strong>Friends who buy and get the money back.</strong> Each person counts at most 1,000 AEQ a month per business. Sheltering 150,000 AEQ would need 100 real people every month, publicly visible.</li>
      <li data-i18n="lh-5"><strong>Founding many firms for many free amounts.</strong> At most 3 business accounts per person, so at most 6,000 AEQ free.</li>
      <li data-i18n="lh-6"><strong>Paying yourself as an “employee”.</strong> Payments to responsible people count as withdrawals, not wages.</li>
      <li data-i18n="lh-7"><strong>Fake wages to friends who exchange and hand back cash.</strong> Exchanges are free of the exit levy only up to 3,000 AEQ per person and month, and wage totals are public.</li>
      <li data-i18n="lh-8"><strong>Parking money in the liquidity pool.</strong> Only people can provide liquidity.</li>
      <li data-i18n="lh-9"><strong>A smart contract as a hiding place.</strong> Contracts are other addresses: at most 1,000 AEQ, 1 % a month.</li>
    </ul>
    </details>
    <div class="sybil-blurb" data-i18n="lh-honest"><strong>What honestly remains:</strong> no money system in the world can stop many real people from colluding. Here every known collusion costs more than it saves, or is limited to small amounts. Business turnover and wage totals are public, so unusual patterns stand out. Anyone who finds a new gap reports it, and the rules are adjusted.</div>
  </div>
</section>

<section id="roadmap">
  <div class="section-inner">
    <div class="section-label" data-i18n="rm-label">Roadmap</div>
    <h2 data-i18n="rm-h2">What comes next</h2>
    <ol class="rm-list">
      <li class="now"><h3 data-i18n="rm-1-h">Now: Phase 1</h3><p data-i18n="rm-1-p">Registration with a live face check by two independent matching services. 1,000 AEQ start, basic income every day.</p></li>
      <li><h3 data-i18n="rm-2-h">1 October 2026</h3><p data-i18n="rm-2-p">The economy rules take effect: three account types, idle-money levy, exit levy, fee-free monthly amounts for people.</p></li>
      <li><h3 data-i18n="rm-3-h">App for shops</h3><p data-i18n="rm-3-p">Checkout mode with QR code and accounting export.</p></li>
      <li><h3 data-i18n="rm-4-h">Pilot town</h3><p data-i18n="rm-4-p">5–10 shops (café, bakery, farm shop, hairdresser, workshop) for three months. Measured: how much stays in circulation, how much leaves, how much reaches the basic income.</p></li>
      <li><h3 data-i18n="rm-5-h">Legal review and real stable coin</h3><p data-i18n="rm-5-p">Before real money: review under the EU crypto regulation (MiCA) and a regulated euro stable coin instead of the test currency tUSD. Only then open more widely.</p></li>
    </ol>
  </div>
</section>

<section id="open">
  <div class="section-inner">
    <div class="section-label" data-i18n="op-label">Honestly</div>
    <h2 data-i18n="op-h2">What is not finished yet</h2>
    <ul class="op-list">
      <li data-i18n="op-1"><strong>Test currency only.</strong> Today tUSD is the only currency to exchange into; there is no real euro or dollar exit yet.</li>
      <li data-i18n="op-2"><strong>Legal review pending.</strong> Whether AEQ and the built-in exchange fall under the EU crypto regulation MiCA must be checked before real money.</li>
      <li data-i18n="op-3"><strong>Exchange-rate risk.</strong> While AEQ is small, its price fluctuates. For cautious shops, immediate exchange is the answer.</li>
      <li data-i18n="op-4"><strong>Taxes.</strong> For businesses, AEQ income is business income at its euro value on the day of payment.</li>
      <li data-i18n="op-5"><strong>The numbers are starting values.</strong> 2,000 AEQ base amount, 1.5 and 3 months' turnover, 0.5 %/2 %, 2 %, 1,000 and 3,000 a month: measured in the pilot town, then adjusted.</li>
    </ul>
    <div class="sybil-blurb" data-i18n="sybil-blurb"><strong>Sybil / protection:</strong> live face check (quorum 2 of independent matching services) + signed attestation + on-chain nullifier, spent once. Named limits: accounts from before the face check (25 Aug 2026 — nearly all of today's 18) have no face template; the matching threshold is not yet calibrated on real captures; liveness is a head-turn challenge, so advanced deepfakes remain a residual risk; the split-share mode (<code>MPC</code>) runs in shadow, each service still holds a whole encrypted template.</div>
  </div>
</section>

<section id="faq">
  <div class="section-inner">
    <div class="section-label" data-i18n="faq-label">Questions</div>
    <h2 data-i18n="faq-h2">Frequently asked</h2>
    <div class="faq">
      <details><summary data-i18n="faq-q1">Why don't businesses simply get dollars only?</summary><p data-i18n="faq-a1">Then every purchase would be a sale of AEQ. There would hardly be buyers, and the price, and with it the basic income, would keep falling. Circulation only happens if businesses can pass AEQ on themselves: to staff, suppliers and other businesses.</p></details>
      <details><summary data-i18n="faq-q2">What changes for me as a person on 1 October 2026?</summary><p data-i18n="faq-a2">For most people nothing, or it gets cheaper: the first 1,000 AEQ you spend each month become free of fees, and the levy on idle money applies only above 5,000 AEQ.</p></details>
      <details><summary data-i18n="faq-q3">Is anything burned?</summary><p data-i18n="faq-a3">No. Every fee and every levy goes 100 % to the basic income and returns to all verified people in equal shares.</p></details>
      <details><summary data-i18n="faq-q4">Can a business receive the basic income or vote?</summary><p data-i18n="faq-a4">No. Basic income, vote and the fair share belong only to verified people. Behind every business account stand one to ten verified people who are responsible for it.</p></details>
      <details><summary data-i18n="faq-q5">What is an “other address”?</summary><p data-i18n="faq-a5">Every address that is neither a verified person nor a business: a visitor's wallet, a tip jar, a simple contract. It may hold at most 1,000 AEQ and pays 1 % a month.</p></details>
      <details><summary data-i18n="faq-q6">Why do rules per person work here?</summary><p data-i18n="faq-a6">Every person exists exactly once at Aequitas. A free amount per person cannot be multiplied with more accounts. No other money can do that.</p></details>
      <details><summary data-i18n="faq-q7">Has money like this ever worked?</summary><p data-i18n="faq-a7">Yes. Wörgl (Austria, 1932) had money that lost 1 % a month; it circulated so fast that the town built roads and bridges with it until the national bank banned it. The Chiemgauer (Bavaria, since 2003) has a circulation levy and hundreds of shops. The WIR Bank (Switzerland, since 1934) runs settlement money between businesses.</p></details>
      <details><summary data-i18n="faq-q8">Why aren't the limits tied to the dollar?</summary><p data-i18n="faq-a8">Because fairness is about each person's share of all the money, not about dollar amounts. The average person always holds exactly one fair share (1,000 AEQ), and every limit is a multiple of it. A dollar link would need a price source that someone could push, and a rising price would quietly tighten the limits. Only the monthly allowances depend on how much of life is paid in AEQ: after the pilot town, the fee-free monthly amount and the exchange allowance are to follow what the median person really spends each month, never less than 1× the fair share.</p></details>
    </div>
  </div>
</section>

<section id="disclaimer" style="padding-top:40px;padding-bottom:40px">
  <div class="disclaimer-card">
    <h3 data-i18n="disc-title">Phase 1 disclaimer</h3>
    <p data-i18n="disc-body">Phase 1: since 25 Aug 2026 the proof server refuses any registration without a signed attestation from the matching quorum — a second phone no longer gives the same face a second account. What is not yet true: accounts registered before that date have no face template and could in principle register again on a new wallet; error rates are not calibrated (that needs ~1,000 impostor pairs); liveness is a head-turn challenge, stronger deepfake defenses are being calibrated. Read “one human, one account” as “checked, with named limits” — not as “impossible to circumvent.”</p>
    <p class="oss-line" data-i18n="oss-line"><strong>Open source:</strong> Core chain public · identity/proof services partly private in Phase 1.</p>
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
