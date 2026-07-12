async function loadStats() {
  try {
    const d = await fetch('/api/status').then(r=>r.json());
    if(d.total_humans !== undefined) document.getElementById('stat-humans').textContent = d.total_humans.toLocaleString();
    if(d.total_supply) document.getElementById('stat-supply').textContent = d.total_supply.replace(' AEQ','');
    if(typeof d.gini === 'number') {
      const g = d.gini.toFixed(4);
      document.getElementById('stat-gini').textContent = g;
      const gi = document.getElementById('gini-inline');
      if(gi) gi.textContent = g;
      const pct = Math.min(d.gini * 100, 100);
      const barAeq = document.getElementById('bar-aeq');
      if(barAeq) barAeq.style.width = pct + '%';
      const valAeq = document.getElementById('val-aeq');
      if(valAeq) valAeq.textContent = g;
    }
    if(d.height !== undefined) document.getElementById('stat-blocks').textContent = d.height.toLocaleString();
  } catch(e) {}
}
loadStats();
// Smooth scroll for anchor links
document.querySelectorAll('a[href^="#"]').forEach(a => {
  a.addEventListener('click', e => {
    e.preventDefault();
    document.querySelector(a.getAttribute('href'))?.scrollIntoView({behavior:'smooth'});
  });
});
