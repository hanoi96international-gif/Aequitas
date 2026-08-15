// Landing page live data.
//
// Everything drawn here comes from this node's own endpoints. Nothing is
// illustrative: the Lorenz curve is computed from the actual registered
// balances, and if that data cannot be loaded the curve is hidden rather than
// replaced with a plausible-looking shape. A page whose whole argument is
// "we measure our own inequality and publish it" cannot afford a decorative
// chart.

const fmt = (n, d = 0) => n.toLocaleString(undefined, {minimumFractionDigits: d, maximumFractionDigits: d});

function setText(id, value) {
  const el = document.getElementById(id);
  if (el) el.textContent = value;
}

async function loadStats() {
  try {
    const d = await fetch('/api/status').then(r => r.json());

    if (d.total_humans !== undefined) setText('stat-humans', d.total_humans.toLocaleString());
    if (d.total_supply) setText('stat-supply', d.total_supply.replace(' AEQ', ''));
    if (d.height !== undefined) setText('stat-blocks', d.height.toLocaleString());

    if (typeof d.gini === 'number') {
      const g = d.gini.toFixed(4);
      setText('stat-gini', g);
      setText('gini-inline', g);
      // The static comparison bars in landing.go are sized as gini*100%
      // (Bitcoin 0.85 -> 85%). The live bar has to use the identical scale or
      // the chart misrepresents Aequitas against the countries beside it.
      const pct = Math.min(d.gini * 100, 100);
      const barAeq = document.getElementById('bar-aeq');
      if (barAeq) barAeq.style.width = pct + '%';
      setText('val-aeq', g);
    }

    // UBI pool + countdown to the next distribution.
    if (typeof d.pool_ubi === 'string' || typeof d.pool_ubi === 'number') {
      setText('mech-ubi-pool', fmt(parseFloat(d.pool_ubi), 4));
    }
    if (typeof d.ubi_next_payout_secs === 'number') {
      const s = Math.max(0, d.ubi_next_payout_secs);
      const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60);
      setText('mech-ubi-next', h > 0 ? `${h}h ${m}m` : `${m}m`);
    }
  } catch (e) { /* endpoint unavailable — placeholders stay */ }
}

async function loadWealthCap() {
  try {
    const d = await fetch('/api/wealth-cap').then(r => r.json());
    if (typeof d.cap_aeq === 'number') setText('mech-cap', fmt(d.cap_aeq));
    if (d.multiplier !== undefined && d.average_aeq !== undefined) {
      setText('mech-cap-formula', `${fmt(d.multiplier)}× × ${fmt(d.average_aeq)} AEQ average`);
    }
    // Fair share is the average balance: total supply divided equally.
    if (typeof d.average_aeq === 'number') setText('mech-fairshare', fmt(d.average_aeq));
  } catch (e) { /* leave placeholders */ }
}

// ── Lorenz curve ───────────────────────────────────────────────────────────
// Sort every wallet's total value ascending, then plot cumulative share of
// wealth against cumulative share of people. Perfect equality is the diagonal;
// the area between the diagonal and the curve is what the Gini coefficient
// summarises as a single number.
async function loadLorenz() {
  const path = document.getElementById('lorenz-path');
  const note = document.getElementById('lorenz-note');
  const wrap = document.querySelector('.lorenz-wrap');
  if (!path) return;

  try {
    const d = await fetch('/api/humans?limit=500').then(r => r.json());
    const values = (d.humans || [])
      .map(h => (typeof h.total_value_aeq === 'number' ? h.total_value_aeq : h.balance) || 0)
      .filter(v => v > 0)
      .sort((a, b) => a - b);

    if (values.length < 2) {
      // Not enough wallets for a meaningful curve. Say so plainly instead of
      // drawing something that implies a distribution we cannot see.
      if (wrap) wrap.style.display = 'none';
      return;
    }

    const total = values.reduce((s, v) => s + v, 0);
    if (total <= 0) { if (wrap) wrap.style.display = 'none'; return; }

    // Chart box matches the axes drawn in the SVG.
    const X0 = 40, X1 = 300, Y0 = 180, Y1 = 20;
    let cum = 0;
    let pts = `M ${X0} ${Y0}`;
    values.forEach((v, i) => {
      cum += v;
      const x = X0 + ((i + 1) / values.length) * (X1 - X0);
      const y = Y0 - (cum / total) * (Y0 - Y1);
      pts += ` L ${x.toFixed(2)} ${y.toFixed(2)}`;
    });
    // Close along the baseline so the gap to the diagonal reads as an area.
    pts += ` L ${X1} ${Y0} Z`;
    path.setAttribute('d', pts);

    // Bottom 50% share — the single most legible number off a Lorenz curve.
    const half = Math.floor(values.length / 2);
    const bottomHalf = values.slice(0, half).reduce((s, v) => s + v, 0);
    const pctBottom = ((bottomHalf / total) * 100).toFixed(1);
    if (note) {
      note.textContent =
        `${values.length} wallets · the poorest 50% hold ${pctBottom}% of all AEQ ` +
        `(perfect equality would be 50%)`;
    }
  } catch (e) {
    if (wrap) wrap.style.display = 'none';
  }
}

loadStats();
loadWealthCap();
loadLorenz();

// Smooth scroll for anchor links
document.querySelectorAll('a[href^="#"]').forEach(a => {
  a.addEventListener('click', e => {
    e.preventDefault();
    document.querySelector(a.getAttribute('href'))?.scrollIntoView({behavior: 'smooth'});
  });
});
