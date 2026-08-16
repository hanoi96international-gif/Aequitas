// ── CONFIG ──────────────────────────────────────────────────────────────────
// FIX (launch audit 2026-08-15): this was hardcoded to
// 'https://aequitas.digital/api'. But this file is only ever served by
// handleDapp, from a node that already exposes the full /api surface on its
// own origin — so every node's /dapp page was reading a DIFFERENT node's
// chain instead of its own. Verified today: aequitas.digital answers
// /api/status with total_humans 0 and height ~17k while the Contabo node
// serving this page is at height 3.8M with 15 humans, so the dApp showed an
// empty chain, a 0 balance and a 0 supply to everyone regardless of which
// node they loaded it from. It also made the page depend on a second host's
// CORS headers and DNS for no reason. Same-origin is what the Explorer
// already does (plain '/api/...' throughout explorer.js) — match it, so the
// dApp always reports the state of the node the user actually connected to.
const API          = '/api';
const CHAIN_ID     = '0x786';  // 1926
const V7           = '0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78';

// ── STATE ────────────────────────────────────────────────────────────────────
let wallet     = null;
let poolData   = null;
let priceHist  = [];
let chartMs    = 3600000;
// SECURITY/FIX (launch audit 2026-07-03): must be lowercase — the backend
// (handleSwap) does a case-sensitive check against exactly "aeq_to_tusd" /
// "tusd_to_aeq" and rejects anything else, including this constant's old
// uppercase value, before the swap logic ever runs.
let swapDir    = 'aeq_to_tusd';
let ubiTimer   = null;
let ubiLeft    = 0;
let lwChart    = null, lwCandle = null, lwVol = null;

const BUCKET = {60000:10000,300000:30000,1800000:120000,3600000:300000,14400000:900000,0:300000};

// ── TABS ─────────────────────────────────────────────────────────────────────
function showTab(id) {
  document.querySelectorAll('.tab-content').forEach(el => el.classList.remove('active'));
  document.querySelectorAll('.nav-btn').forEach(el => el.classList.remove('active'));
  document.getElementById('tab-' + id).classList.add('active');
  document.querySelector('.nav-btn[data-tab="' + id + '"]').classList.add('active');
  if (id === 'trade') setTimeout(drawChart, 80);
}

// ── WALLET ───────────────────────────────────────────────────────────────────
async function connectWallet() {
  if (!window.ethereum) { alert('MetaMask is required. Please install it first.'); return; }
  try {
    const accounts = await window.ethereum.request({ method: 'eth_requestAccounts' });
    wallet = accounts[0];
    await ensureChain();
    onWalletConnected();
  } catch(e) { console.error(e); }
}

async function ensureChain() {
  const chainHex = await window.ethereum.request({ method: 'eth_chainId' });
  if (chainHex !== CHAIN_ID) await addToMetaMask();
}

async function addToMetaMask() {
  try {
    await window.ethereum.request({ method: 'wallet_addEthereumChain', params: [{
      chainId: CHAIN_ID,
      chainName: 'Aequitas Chain',
      nativeCurrency: { name: 'Aequitas', symbol: 'AEQ', decimals: 18 },
      // FIX (audit 2026-08-16): same class as this file's own API_BASE fix
      // above — hardcoded to a domain that doesn't resolve to this network
      // until 2026-08-18. window.location.origin is whichever node actually
      // served this page, correct on both sides of the domain switch.
      rpcUrls: [window.location.origin + '/rpc'],
      blockExplorerUrls: [window.location.origin],
    }]});
  } catch(e) { console.error(e); }
}

function onWalletConnected() {
  const short = wallet.slice(0,6) + '…' + wallet.slice(-4);
  document.getElementById('wallet-addr-display').textContent = short;
  const btn = document.getElementById('connect-btn');
  btn.textContent = short;
  btn.classList.add('connected');
  document.getElementById('wallet-not-connected').style.display = 'none';
  document.getElementById('wallet-connected').style.display = 'block';
  document.getElementById('id-not-connected').style.display = 'none';
  document.getElementById('id-connected').style.display = 'block';
  loadBalance();
}

function copyWalletAddr() {
  if (!wallet) return;
  navigator.clipboard.writeText(wallet).catch(()=>{});
}

window.ethereum && window.ethereum.on('accountsChanged', accs => {
  if (accs.length) { wallet = accs[0]; onWalletConnected(); }
  else { wallet = null; }
});

// ── API HELPERS ──────────────────────────────────────────────────────────────
// FIX (P3-h, audit 2026-07-06): loadStatus/loadBalance/loadPriceHistory/loadPool
// all swallow fetch failures via empty catch(e) {} with zero user-facing
// signal. Tracking connectivity here, in the one shared function all of them
// route through, surfaces it everywhere at once without touching those 4
// call sites. A single failure isn't enough to flag (transient blips are
// normal); require a couple of consecutive failures first.
let apiFailStreak = 0;
const API_STALE_THRESHOLD = 2;

function setChainConnectionStatus(ok) {
  const dot = document.getElementById('chain-status-dot');
  const note = document.getElementById('chain-stale-note');
  if (!dot || !note) return;
  dot.className = ok ? 'dot-green' : 'dot-stale';
  note.style.display = ok ? 'none' : 'inline';
}

async function api(path) {
  try {
    const r = await fetch(API + path);
    if (!r.ok) throw new Error(r.statusText);
    const data = await r.json();
    apiFailStreak = 0;
    setChainConnectionStatus(true);
    return data;
  } catch (e) {
    apiFailStreak++;
    if (apiFailStreak >= API_STALE_THRESHOLD) setChainConnectionStatus(false);
    throw e;
  }
}

function fmtAEQ(v) {
  if (v == null) return '—';
  const n = parseFloat(v);
  if (n >= 1e6) return (n/1e6).toFixed(2) + 'M';
  if (n >= 1e3) return (n/1e3).toFixed(2) + 'K';
  return n.toFixed(4);
}

// ── HOME: STATUS POLLING ──────────────────────────────────────────────────────
async function loadStatus() {
  try {
    const d = await api('/status');
    document.getElementById('h-height').textContent  = d.height ? '#' + d.height.toLocaleString() : '—';
    // FIX (launch audit 2026-08-15): total_humans is a COUNT of people, not an
    // AEQ amount, but it was routed through fmtAEQ — the currency formatter.
    // That rendered the "Verified Humans" tile as "15.0000" today, and would
    // abbreviate it to "1.50K" once the registry passed a thousand people,
    // losing the exact figure precisely when it starts to matter. The explorer
    // already prints this field with plain toLocaleString (fmt in explorer.js);
    // match it so both pages agree.
    document.getElementById('h-humans').textContent  =
      (typeof d.total_humans === 'number') ? d.total_humans.toLocaleString() : '—';
    document.getElementById('h-supply').textContent  = d.total_supply || '—';
    document.getElementById('h-index').textContent   = typeof d.index === 'number' ? d.index.toFixed(1) : '—';
    document.getElementById('h-gini').textContent    = typeof d.gini === 'number' ? d.gini.toFixed(4) : '—';
    document.getElementById('h-pool-ubi').textContent        = (d.pool_ubi || '0') + ' AEQ';
    document.getElementById('h-pool-treasury').textContent   = (d.pool_treasury || '0') + ' AEQ';
    document.getElementById('h-pool-validators').textContent = (d.pool_validators || '0') + ' AEQ';
    const phase = d.phase || 0;
    for (let i = 0; i <= 3; i++) {
      const el = document.getElementById('ph-' + i);
      if (el) el.classList.toggle('active', i === phase);
    }
    const descs = ['Bootstrap — sliding wealth cap 5×→25×','Growth — expanding human registry','Stability — redistribution active','Maturity — full decentralization'];
    document.getElementById('h-phase-desc').textContent = descs[phase] || '';
    if (d.ubi_next_payout_secs > 0) startUBITimer(d.ubi_next_payout_secs);
    const pct = Math.min(100, Math.max(0, (86400 - (d.ubi_next_payout_secs || 0)) / 86400 * 100));
    document.getElementById('h-ubi-bar').style.width = pct.toFixed(1) + '%';
  } catch(e) {}
}

function startUBITimer(secs) {
  ubiLeft = secs;
  if (ubiTimer) clearInterval(ubiTimer);
  ubiTimer = setInterval(() => {
    ubiLeft = Math.max(0, ubiLeft - 1);
    const h = Math.floor(ubiLeft / 3600), m = Math.floor((ubiLeft % 3600) / 60), s = ubiLeft % 60;
    document.getElementById('h-ubi-time').textContent =
      String(h).padStart(2,'0') + ':' + String(m).padStart(2,'0') + ':' + String(s).padStart(2,'0');
  }, 1000);
}

// ── BALANCE ──────────────────────────────────────────────────────────────────
async function loadBalance() {
  if (!wallet) return;
  try {
    const d = await api('/balance?wallet=' + wallet);
    document.getElementById('w-aeq').textContent  = fmtAEQ(d.balance);
    document.getElementById('w-tusd').textContent = fmtAEQ(d.tusd_balance);
    document.getElementById('demurrage-warn').style.display = d.demurrage_active ? 'block' : 'none';
    document.getElementById('id-verified').style.display   = d.is_human ? 'flex' : 'none';
    document.getElementById('id-unverified').style.display = d.is_human ? 'none' : 'flex';
    document.getElementById('id-register-card').style.display = d.is_human ? 'none' : 'block';
  } catch(e) {}
}

// ── PRICE CHART ───────────────────────────────────────────────────────────────
async function loadPriceHistory() {
  try {
    const d = await api('/price-history');
    // Response: {count, history: [{t, p}] | null}
    const raw = Array.isArray(d) ? d : (d && d.history ? d.history : null);
    if (raw) priceHist = raw.map(p => ({t: p.t || p.timestamp || 0, p: p.p || p.price || 0}));
    drawChart();
  } catch(e) {}
}

function buildOHLC(pts, filterMs, bms) {
  const now = Date.now();
  const data = filterMs > 0 ? pts.filter(p => now - p.t <= filterMs && p.p > 0) : pts.filter(p => p.p > 0);
  if (!data.length) return [];
  const buckets = {};
  data.forEach(pt => {
    const b = Math.floor(pt.t / bms) * bms;
    if (!buckets[b]) buckets[b] = { time: Math.floor(b / 1000), open: pt.p, high: pt.p, low: pt.p, close: pt.p, vol: 1 };
    else { buckets[b].high = Math.max(buckets[b].high, pt.p); buckets[b].low = Math.min(buckets[b].low, pt.p); buckets[b].close = pt.p; buckets[b].vol++; }
  });
  return Object.values(buckets).sort((a, b) => a.time - b.time);
}

function initChart() {
  const el = document.getElementById('price-chart');
  if (!el || lwChart || !window.LightweightCharts) return;
  lwChart = LightweightCharts.createChart(el, {
    width: el.clientWidth, height: 240,
    layout: { background: { color: '#0A0C16' }, textColor: '#8892A4', fontSize: 11, fontFamily: 'JetBrains Mono,monospace' },
    grid: { vertLines: { color: 'rgba(255,255,255,0.03)' }, horzLines: { color: 'rgba(255,255,255,0.03)' } },
    crosshair: {
      mode: LightweightCharts.CrosshairMode.Normal,
      vertLine: { color: 'rgba(155,114,246,.5)', labelBackgroundColor: '#9B72F6' },
      horzLine: { color: 'rgba(155,114,246,.5)', labelBackgroundColor: '#9B72F6' },
    },
    rightPriceScale: { borderColor: 'rgba(255,255,255,0.06)' },
    timeScale: { borderColor: 'rgba(255,255,255,0.06)', timeVisible: true, secondsVisible: false, rightOffset: 4 },
  });
  lwCandle = lwChart.addCandlestickSeries({
    upColor: '#34D399', downColor: '#F87171',
    borderUpColor: '#34D399', borderDownColor: '#F87171',
    wickUpColor: '#34D399', wickDownColor: '#F87171',
    priceFormat: { type: 'price', precision: 6, minMove: 0.000001 },
  });
  lwVol = lwChart.addHistogramSeries({ priceFormat: { type: 'volume' }, priceScaleId: 'vol', scaleMargins: { top: 0.8, bottom: 0 } });
  try { lwChart.priceScale('vol').applyOptions({ scaleMargins: { top: 0.8, bottom: 0 } }); } catch(_) {}
  new ResizeObserver(entries => {
    if (lwChart && entries[0]) lwChart.applyOptions({ width: entries[0].contentRect.width });
  }).observe(el);
}

function drawChart() {
  const el = document.getElementById('price-chart');
  if (!el || !el.offsetParent) return;
  if (!lwChart) initChart();
  if (!lwChart) return;
  const bms = BUCKET[chartMs] || 300000;
  const candles = buildOHLC(priceHist, chartMs, bms);
  updatePriceDisplay(candles);
  if (!candles.length) { lwCandle && lwCandle.setData([]); lwVol && lwVol.setData([]); return; }
  lwCandle.setData(candles.map(c => ({ time: c.time, open: c.open, high: c.high, low: c.low, close: c.close })));
  lwVol.setData(candles.map(c => ({ time: c.time, value: c.vol, color: c.close >= c.open ? 'rgba(52,211,153,0.22)' : 'rgba(248,113,113,0.22)' })));
  lwChart.timeScale().fitContent();
}

function updatePriceDisplay(candles) {
  const pEl = document.getElementById('t-price'), cEl = document.getElementById('t-change');
  if (!pEl) return;
  if (!candles.length) { pEl.textContent = '—'; return; }
  const last = candles[candles.length - 1], first = candles[0];
  const chg = first.open > 0 ? (last.close - first.open) / first.open * 100 : 0;
  pEl.textContent = last.close.toFixed(6) + ' tUSD';
  if (cEl) { cEl.textContent = (chg >= 0 ? '▲ +' : '▼ ') + Math.abs(chg).toFixed(2) + '%'; cEl.style.color = chg >= 0 ? '#34D399' : '#F87171'; }
}

function setChartInterval(ms) {
  chartMs = ms;
  document.querySelectorAll('.ci-btn').forEach(b => b.classList.remove('ci-active'));
  const id = ms === 60000 ? 'ci-1m' : ms === 300000 ? 'ci-5m' : ms === 1800000 ? 'ci-30m' : ms === 3600000 ? 'ci-1h' : ms === 14400000 ? 'ci-4h' : 'ci-all';
  const el = document.getElementById(id);
  if (el) el.classList.add('ci-active');
  drawChart();
}

// ── POOL STATUS ───────────────────────────────────────────────────────────────
async function loadPool() {
  try {
    const d = await api('/pool');
    // Response: {reserve_aeq, reserve_tusd, price_aeq_in_tusd}
    poolData = { aeq_reserve: d.reserve_aeq, tusd_reserve: d.reserve_tusd, price_aeq_in_tusd: d.price_aeq_in_tusd };
    const aeq = parseFloat(d.reserve_aeq || 0), tusd = parseFloat(d.reserve_tusd || 0);
    document.getElementById('swap-pool').textContent = aeq > 0
      ? fmtAEQ(aeq) + ' AEQ / ' + fmtAEQ(tusd) + ' tUSD'
      : 'No liquidity yet';
    document.getElementById('t-pool-info').textContent = aeq > 0 ? ('1 AEQ ≈ ' + d.price_aeq_in_tusd.toFixed(4) + ' tUSD') : '';
    // Append latest price point
    if (d.reserve_aeq > 0 && d.price_aeq_in_tusd > 0) {
      priceHist.push({ t: Date.now(), p: d.price_aeq_in_tusd });
      if (priceHist.length > 1000) priceHist.shift();
    }
    calcSwapPreview();
    drawChart();
  } catch(e) {}
}

// ── SWAP ──────────────────────────────────────────────────────────────────────
function setSwapDir(dir) {
  swapDir = dir;
  document.getElementById('dir-aeq-to-tusd').classList.toggle('active', dir === 'aeq_to_tusd');
  document.getElementById('dir-tusd-to-aeq').classList.toggle('active', dir === 'tusd_to_aeq');
  document.getElementById('swap-from-label').textContent = 'Amount (' + (dir === 'aeq_to_tusd' ? 'AEQ' : 'tUSD') + ')';
  calcSwapPreview();
}

function calcSwapPreview() {
  if (!poolData) return;
  const amountEl = document.getElementById('swap-amount');
  if (!amountEl || !amountEl.offsetParent) return; // trade tab not visible — same guard as drawChart()
  const amt = parseFloat(amountEl.value) || 0;
  if (amt <= 0) { document.getElementById('swap-out').textContent = '—'; document.getElementById('swap-min').textContent = '—'; return; }
  const aeqR = parseFloat(poolData.aeq_reserve || 0), tusdR = parseFloat(poolData.tusd_reserve || 0);
  let out;
  if (swapDir === 'aeq_to_tusd') {
    const inF = amt * 0.999;
    out = (tusdR * inF) / (aeqR + inF);
    document.getElementById('swap-out').textContent = out.toFixed(6) + ' tUSD';
    document.getElementById('swap-min').textContent = (out * 0.99).toFixed(6) + ' tUSD';
  } else {
    const inF = amt * 0.999;
    out = (aeqR * inF) / (tusdR + inF);
    document.getElementById('swap-out').textContent = out.toFixed(6) + ' AEQ';
    document.getElementById('swap-min').textContent = (out * 0.99).toFixed(6) + ' AEQ';
  }
}

async function doSwap() {
  if (!wallet) { connectWallet(); return; }
  const amt = parseFloat(document.getElementById('swap-amount').value);
  if (!amt || amt <= 0) { setStatus('swap-status', 'Enter an amount', 'err'); return; }
  const btn = document.getElementById('swap-btn');
  btn.disabled = true;
  setStatus('swap-status', 'Getting nonce…', 'info');
  try {
    const nonce = (await api('/nonce?wallet=' + wallet)).nonce;
    // FIX (launch audit 2026-07-03): Date.now() is milliseconds; the server
    // compares this timestamp against time.Now().Unix() (seconds) and
    // rejects anything outside a 60s window — a millisecond value is off by
    // ~1000x, so every swap failed the staleness check regardless of the
    // signature. Must be seconds, matching the message the server signs.
    const ts = Math.floor(Date.now() / 1000);
    const msg = 'Aequitas Swap: ' + swapDir + ' ' + amt.toFixed(8) + ' nonce:' + nonce + ' ts:' + ts;
    setStatus('swap-status', 'Sign in MetaMask…', 'info');
    const sig = await window.ethereum.request({ method: 'personal_sign', params: [msg, wallet] });
    setStatus('swap-status', 'Submitting swap…', 'info');
    let minOut = 0;
    if (poolData) {
      const aeqR = parseFloat(poolData.aeq_reserve || 0), tusdR = parseFloat(poolData.tusd_reserve || 0);
      const inF = amt * 0.999;
      const out = swapDir === 'aeq_to_tusd' ? (tusdR * inF) / (aeqR + inF) : (aeqR * inF) / (tusdR + inF);
      minOut = out * 0.99;
    }
    const r = await fetch(API + '/swap', { method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ wallet, direction: swapDir, amount: amt.toFixed(8), nonce, timestamp: ts, signature: sig, min_amount_out: minOut }) });
    const d = await r.json();
    // FIX (launch audit 2026-07-03): handleSwap always responds HTTP 200,
    // signaling failure only via {"success":false,"message":"..."} in the
    // body (no field named "error" exists in that response either) — `r.ok`
    // is always true here regardless of outcome, so this never actually
    // caught a failed swap before. Must check the body's own success flag.
    if (!d.success) throw new Error(d.message || 'swap failed');
    setStatus('swap-status', '✓ Swap complete!', 'ok');
    document.getElementById('swap-amount').value = '';
    calcSwapPreview();
    setTimeout(loadBalance, 1500);
  } catch(e) { setStatus('swap-status', '✗ ' + e.message, 'err'); }
  btn.disabled = false;
}

// parseAEQToWei converts a decimal AEQ amount STRING directly to a wei
// BigInt, entirely in string/BigInt arithmetic — no floating-point step at
// any point. FIX (P2-9, beta-launch audit 2026-07-05): the previous
// `BigInt(Math.round(amt * 1e18))` ran the multiplication in JS `Number`
// (53-bit mantissa) BEFORE the BigInt conversion — sub-wei precision was
// already lost above roughly 0.009 AEQ, and even whole-AEQ integer
// precision broke down past ~9,007,199 AEQ. The transaction still went
// through, just for a silently rounded amount instead of the exact one the
// user typed. Returns null for anything that isn't a plain, non-negative
// decimal number string.
function parseAEQToWei(str) {
  str = String(str).trim();
  if (!/^\d+(\.\d*)?$/.test(str)) return null; // accepts a trailing bare "." (e.g. "5.") same as parseFloat did
  const parts = str.split('.');
  const intPart = parts[0];
  const fracPart = (parts[1] || '').padEnd(18, '0').slice(0, 18);
  return BigInt(intPart) * (10n ** 18n) + BigInt(fracPart || '0');
}

// ── SEND AEQ ─────────────────────────────────────────────────────────────────
async function doSend() {
  if (!wallet) { connectWallet(); return; }
  const to = document.getElementById('send-to').value.trim();
  const amtWei = parseAEQToWei(document.getElementById('send-amount').value);
  if (!/^0x[0-9a-fA-F]{40}$/.test(to)) { setStatus('send-status', 'Invalid recipient address', 'err'); return; }
  if (amtWei === null || amtWei <= 0n) { setStatus('send-status', 'Enter an amount', 'err'); return; }
  setStatus('send-status', 'Confirm in MetaMask…', 'info');
  try {
    const value = '0x' + amtWei.toString(16);
    const txHash = await window.ethereum.request({ method: 'eth_sendTransaction', params: [{ from: wallet, to, value }] });
    setStatus('send-status', '✓ Sent! Tx: ' + txHash.slice(0, 12) + '…', 'ok');
    document.getElementById('send-to').value = '';
    document.getElementById('send-amount').value = '';
    setTimeout(loadBalance, 3000);
  } catch(e) { setStatus('send-status', '✗ ' + e.message, 'err'); }
}

// ── FAUCET ────────────────────────────────────────────────────────────────────
async function doFaucet() {
  if (!wallet) { connectWallet(); return; }
  setStatus('faucet-status', 'Requesting…', 'info');
  try {
    // FIX (launch audit 2026-07-03): handleFaucet requires a signed,
    // timestamped request (personal_sign, same pattern as doSwap) — this
    // used to POST only {wallet}, which always failed server-side with
    // "timestamp required" before ever reaching the actual faucet logic.
    const ts = Math.floor(Date.now() / 1000);
    const msg = 'Aequitas tUSD Faucet Claim: ' + wallet.toLowerCase() + ' ts:' + ts;
    setStatus('faucet-status', 'Sign in MetaMask…', 'info');
    const sig = await window.ethereum.request({ method: 'personal_sign', params: [msg, wallet] });
    setStatus('faucet-status', 'Requesting…', 'info');
    const r = await fetch(API + '/faucet', { method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ wallet, timestamp: ts, signature: sig }) });
    const d = await r.json();
    // FIX (launch audit 2026-07-03): same always-200 response pattern as
    // /swap — must check the body's success flag, not r.ok/d.error.
    if (!d.success) throw new Error(d.message || 'faucet claim failed');
    setStatus('faucet-status', '✓ tUSD sent to your wallet', 'ok');
    setTimeout(loadBalance, 2000);
  } catch(e) { setStatus('faucet-status', '✗ ' + e.message, 'err'); }
}

// ── UTILS ─────────────────────────────────────────────────────────────────────
function setStatus(id, msg, cls) {
  const el = document.getElementById(id);
  if (!el) return;
  el.textContent = msg;
  el.className = 'status-line' + (cls ? ' ' + cls : '');
}

// ── EVENT WIRING ─────────────────────────────────────────────────────────────
// FIX (Monster Audit follow-up, 2026-07-12, P1): every element below used to
// carry an onclick=/oninput= attribute, which forced 'unsafe-inline' into
// this page's script-src (it had no CSP at all — see handleDapp's own
// comment in api.go). All static, so plain addEventListener-by-id is enough
// here — unlike explorer.js this page never injects HTML with handlers
// baked into it, so the delegated data-act system built for explorer.js
// isn't needed.
function wireEvents() {
  document.getElementById('wallet-addr-display').addEventListener('click', copyWalletAddr);
  document.getElementById('connect-btn').addEventListener('click', connectWallet);
  document.getElementById('wallet-connect-btn').addEventListener('click', connectWallet);
  const idConnectBtn = document.getElementById('id-connect-btn');
  if (idConnectBtn) idConnectBtn.addEventListener('click', connectWallet);

  document.querySelectorAll('.ci-btn[data-interval-ms]').forEach(btn => {
    btn.addEventListener('click', () => setChartInterval(Number(btn.dataset.intervalMs)));
  });
  document.querySelectorAll('.dir-btn[data-swap-dir]').forEach(btn => {
    btn.addEventListener('click', () => setSwapDir(btn.dataset.swapDir));
  });
  document.getElementById('swap-amount').addEventListener('input', calcSwapPreview);
  document.getElementById('swap-btn').addEventListener('click', doSwap);

  document.getElementById('send-btn').addEventListener('click', doSend);
  document.getElementById('add-network-btn').addEventListener('click', addToMetaMask);
  document.getElementById('faucet-btn').addEventListener('click', doFaucet);

  document.querySelectorAll('.nav-btn[data-tab]').forEach(btn => {
    btn.addEventListener('click', () => showTab(btn.dataset.tab));
  });
}

// ── INIT ──────────────────────────────────────────────────────────────────────
wireEvents();
loadStatus();
loadPriceHistory();
loadPool();

// FIX (P3, performance audit 2026-07-06): these used to be 3 independent
// setInterval timers, uncoordinated — no reason a page load couldn't
// happen to line their firing up back-to-back, and each ran its own
// separate implicit retry cascade on failure. One shared 1s scheduler
// tick dispatches to each job at its own cadence instead, same effective
// polling rate, one place to see/adjust it.
const pollJobs = [
  { fn: loadStatus, intervalMs: 10000, lastRun: 0 },
  { fn: loadPool,   intervalMs: 8000,  lastRun: 0 },
  { fn: () => { if (wallet) loadBalance(); }, intervalMs: 15000, lastRun: 0 },
];
setInterval(() => {
  const now = Date.now();
  for (const job of pollJobs) {
    if (now - job.lastRun >= job.intervalMs) {
      job.lastRun = now;
      job.fn();
    }
  }
}, 1000);

// Restore wallet connection if MetaMask already authorized
(async () => {
  if (!window.ethereum) return;
  try {
    const accounts = await window.ethereum.request({ method: 'eth_accounts' });
    if (accounts.length) { wallet = accounts[0]; onWalletConnected(); }
  } catch(_) {}
})();
