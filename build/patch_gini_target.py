#!/usr/bin/env python3
"""Update Gini target from 0.20 → 0.35 and align grade-box thresholds."""
import sys

FILE = r'C:\Users\aequitas-chain\x\humanity\keeper\api_html.go'

with open(FILE, 'r', encoding='utf-8') as f:
    src = f.read()

replacements = []

# ── 1. HTML default idx-desc ──
replacements.append((
    'Aequitas targets Gini below 0.20 (Index below 20) at scale — enforced automatically by the wealth cap and redistribution pools, no governance vote required.',
    'Aequitas targets Gini below 0.35 (Index below 35) at scale — comparable to the most equal developed economies — enforced automatically by the wealth cap and redistribution pools, no governance vote required.'
))

# ── 2. HTML default gini-what-text ──
replacements.append((
    'Aequitas long-term target: Gini below 0.20 — enforced by the wealth cap at scale (bootstrap: sliding cap 5×→25× per human).</div>',
    'Aequitas long-term target: Gini below 0.35 — comparable to Scandinavia and Germany, enforced by the wealth cap at scale (bootstrap: sliding cap 5×→25× per human).</div>'
))

# ── 3. EN i18n idx-desc ──
replacements.append((
    'Aequitas long-term target: Gini below 0.20 (Index below 20) — enforced by the wealth cap and redistribution pools.',
    'Aequitas long-term target: Gini below 0.35 (Index below 35) — comparable to the most equal developed economies, enforced by the wealth cap and redistribution pools.'
))

# ── 4. EN i18n gini-what-text ──
replacements.append((
    'Aequitas long-term target: Gini below 0.20 — enforced by the wealth cap at scale (bootstrap: sliding cap 5×→25× per human).',
    'Aequitas long-term target: Gini below 0.35 — comparable to Scandinavia and Germany, enforced by the wealth cap at scale (bootstrap: sliding cap 5×→25× per human).'
))

# ── 5. DE i18n idx-desc ──
replacements.append((
    'Aequitas-Langzeitziel: Gini unter 0,20 (Index unter 20) — automatisch durchgesetzt durch den Vermögensobergrenze-Mechanismus.',
    'Aequitas-Langzeitziel: Gini unter 0,35 (Index unter 35) — vergleichbar mit den gleichheitsstärksten Industrieländern, automatisch durchgesetzt durch den Vermögensobergrenze-Mechanismus.'
))

# ── 6. DE i18n gini-what-text ──
replacements.append((
    'Aequitas-Langzeitziel: Gini unter 0,20 — durchgesetzt durch den Vermögensdeckel bei Skalierung (Bootstrap: gleitender Deckel 5×→25× pro Mensch).',
    'Aequitas-Langzeitziel: Gini unter 0,35 — vergleichbar mit Skandinavien und Deutschland, durchgesetzt durch den Vermögensdeckel (Bootstrap: gleitender Deckel 5×→25× pro Mensch).'
))

# ── 7. Grade box 0–20 IDEAL → 0–35 IDEAL ──
replacements.append((
    '<div style="font-size:1.05rem;font-weight:700;color:var(--neon);font-family:var(--font-display)">0 – 20</div>',
    '<div style="font-size:1.05rem;font-weight:700;color:var(--neon);font-family:var(--font-display)">0 – 35</div>'
))
replacements.append((
    'Near-perfect equality. Better than any country on Earth. Wealth cap and demurrage passively maintaining balance. No additional protocol action.',
    'Healthier than most nations on Earth. Comparable to Scandinavia (0.27) and Germany (0.31). Wealth cap and demurrage successfully maintaining fair distribution.'
))

# ── 8. Grade box 20–40 GOOD → 35–50 GOOD ──
replacements.append((
    '<div style="font-size:1.05rem;font-weight:700;color:var(--blue);font-family:var(--font-display)">20 – 40</div>',
    '<div style="font-size:1.05rem;font-weight:700;color:var(--blue);font-family:var(--font-display)">35 – 50</div>'
))
replacements.append((
    'Mild inequality — comparable to Scandinavia. Redistribution mechanisms actively flattening the distribution. Demurrage and wealth cap intensifying.',
    'Comparable to the USA (0.41) or France (0.32). Within the range of most developed economies. Redistribution mechanisms actively flattening the curve.'
))

# ── 9. Grade box 40–65 WARNING → 50–70 WARNING ──
replacements.append((
    '<div style="font-size:1.05rem;font-weight:700;color:var(--gold);font-family:var(--font-display)">40 – 65</div>',
    '<div style="font-size:1.05rem;font-weight:700;color:var(--gold);font-family:var(--font-display)">50 – 70</div>'
))
replacements.append((
    'Noticeable concentration — comparable to developing countries. Protocol phase advancing. Redistribution pressure at maximum for current phase.',
    'Higher than most European nations — comparable to Brazil (0.53) or Russia. Protocol redistribution at elevated intensity.'
))

# ── 10. Grade box 65–100 CRITICAL → 70–100 CRITICAL ──
replacements.append((
    '<div style="font-size:1.05rem;font-weight:700;color:var(--red);font-family:var(--font-display)">65 – 100</div>',
    '<div style="font-size:1.05rem;font-weight:700;color:var(--red);font-family:var(--font-display)">70 – 100</div>'
))
replacements.append((
    'Worse than Bitcoin (85) or any nation on Earth (max 63). Protocol at maximum intervention. Wealth cap at 25× mean enforced.',
    'Worse than any country on Earth (South Africa record: 0.63). Approaching Bitcoin (0.85). Protocol at maximum intervention — wealth cap and redistribution at full force.'
))

# ── Apply ──
errors = []
for i, (old, new) in enumerate(replacements):
    if old not in src:
        errors.append(f'[MISS #{i+1}] {old[:80]!r}')
    elif src.count(old) > 1:
        errors.append(f'[DUP #{i+1}] {old[:80]!r}')
    else:
        src = src.replace(old, new)

if errors:
    print('ERRORS:')
    for e in errors:
        print(' ', e)
    sys.exit(1)

with open(FILE, 'w', encoding='utf-8') as f:
    f.write(src)

print(f'Done: {len(replacements)} replacements applied.')
