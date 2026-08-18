// Landing page live data.
//
// Everything drawn here comes from this node's own endpoints, and there is
// deliberately very little of it: the page is an overview, so it shows the
// four headline numbers, the live Gini, and the countdown to the next equal
// split. The Lorenz curve, the wealth-cap arithmetic and the fair-share
// figure used to be rendered here too — they now live on /index/score and
// /network, where there is room to explain them instead of merely printing
// them. Nothing here is illustrative: if a number cannot be loaded it stays
// an em dash rather than becoming a plausible-looking placeholder.

const fmt = (n, d = 0) => n.toLocaleString(undefined, {minimumFractionDigits: d, maximumFractionDigits: d});

function setText(id, value) {
  const el = document.getElementById(id);
  if (el) el.textContent = value;
}

// The GHOSTDAG badge in the header reports whether this page can actually
// reach the node, not whether someone remembered to write "healthy" into the
// markup. loadStats already fetches /api/status every cycle, so the badge
// rides on a request that has to happen anyway: it goes red the moment that
// request stops coming back, which is the only state worth showing.
function setHealthBadge(ok, note) {
  const el = document.getElementById('health-badge');
  if (!el) return;
  el.classList.toggle('badge-health-healthy', ok);
  el.classList.toggle('badge-health-unhealthy', !ok);
  el.textContent = ok ? '● GHOSTDAG' : '● NO NODE';
  el.title = note;
}

async function loadStats() {
  try {
    const d = await fetch('/api/status').then(r => r.json());
    setHealthBadge(true, 'Node answering — height ' + (d.height !== undefined ? d.height.toLocaleString() : '?'));

    if (d.total_humans !== undefined) setText('stat-humans', d.total_humans.toLocaleString());
    if (d.total_supply) setText('stat-supply', d.total_supply.replace(' AEQ', ''));
    if (d.height !== undefined) setText('stat-blocks', d.height.toLocaleString());

    if (typeof d.gini === 'number') {
      const g = d.gini.toFixed(4);
      setText('stat-gini', g);
      setText('gini-inline', g);
      // The static Bitcoin bar in landing.go is sized as gini*100%
      // (0.85 -> 85%). The live bar has to use the identical scale or the
      // comparison misrepresents Aequitas against the bar beside it.
      const pct = Math.min(d.gini * 100, 100);
      const barAeq = document.getElementById('bar-aeq');
      if (barAeq) barAeq.style.width = pct + '%';
      setText('val-aeq', g);
    }

    // UBI pool + countdown to the next distribution — the one live
    // mechanism figure this page still carries.
    if (typeof d.pool_ubi === 'string' || typeof d.pool_ubi === 'number') {
      setText('ubi-pool', fmt(parseFloat(d.pool_ubi), 4));
    }
    if (typeof d.ubi_next_payout_secs === 'number') {
      const s = Math.max(0, d.ubi_next_payout_secs);
      const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60);
      setText('ubi-next', h > 0 ? `${h}h ${m}m` : `${m}m`);
    }
  } catch (e) {
    // The one state the badge exists for: the page is up, the node is not.
    setHealthBadge(false, 'Cannot reach /api/status from this page');
  }
}

loadStats();

// Smooth scroll for anchor links
document.querySelectorAll('a[href^="#"]').forEach(a => {
  a.addEventListener('click', e => {
    e.preventDefault();
    document.querySelector(a.getAttribute('href'))?.scrollIntoView({behavior: 'smooth'});
  });
});
