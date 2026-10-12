"""Abzuege entzerren, Farbstich entfernen, druckfertig ablegen.

Schritte je Bild:
  1. Perspektivische Entzerrung ueber eine Homographie (vier Ecken -> Rechteck)
  2. Randbeschnitt (weisser Papierrand, angeschnittene Kanten)
  3. Weissabgleich und Tonwertspreizung je Kanal, gedaempft beimischbar
  4. Sanfte Saettigung, Gammakorrektur, Unschaerfemaske
"""
from PIL import Image, ImageFilter, ImageEnhance
import numpy as np, json, os

SRC = "/root/.claude/uploads/27eeda6a-8235-58ad-9336-30a868719c73"
SCR = "/tmp/claude-0/-home-user-Aequitas/27eeda6a-8235-58ad-9336-30a868719c73/scratchpad"
ZIEL = "/home/user/Aequitas/minibuch-mama-60/inhalt/bilder"
os.makedirs(ZIEL, exist_ok=True)

DATEIEN = {
    "819278e2": "819278e2-eed711048db04c6b8909b14922e7f5ec.jpeg",
    "b36d5e63": "b36d5e63-0674c379071e45618c77cd6baecb91a7.jpeg",
    "c5293ee5": "c5293ee5-4dca420fb73a467db1eddca2cb2f054c.jpeg",
    "f15fbb92": "f15fbb92-893d81330fa9443fa959a6b63af67e5e.jpeg",
    "5f7e3baa": "5f7e3baa-IMG_6119.jpeg",
}

# name -> (Ausgabename, Beschnitt l/o/r/u in Anteilen, Optionen)
PLAN = {
    "f15fbb92": ("sw-wohnzimmer-sechziger" ,  (.055, .050, .050, .045), dict(sw=True,  levels=.95, sat=0.0, gamma=1.00)),
    "b36d5e63": ("maedchen-auf-dem-fahrrad",  (.004, .012, .028, .018), dict(sw=False, levels=.70, sat=1.25, gamma=0.96)),
    "c5293ee5": ("junge-frau-eckbank",      (.062, .042, .006, .012), dict(sw=False, levels=.65, sat=1.15, gamma=0.98)),
    "819278e2": ("kaffeetafel-mit-kind",    (.010, .022, .010, .010), dict(sw=False, levels=.60, sat=1.12, gamma=1.02)),
    "5f7e3baa": ("familienportrait",        (.010, .010, .010, .010), dict(sw=False, levels=.85, sat=1.10, gamma=0.98)),
}


def homographie(src, dst):
    """DLT: 3x3-Matrix, die dst-Punkte auf src-Punkte abbildet."""
    A, b = [], []
    for (X, Y), (x, y) in zip(dst, src):
        A.append([X, Y, 1, 0, 0, 0, -X * x, -Y * x]); b.append(x)
        A.append([0, 0, 0, X, Y, 1, -X * y, -Y * y]); b.append(y)
    h = np.linalg.solve(np.array(A, float), np.array(b, float))
    return np.append(h, 1.0).reshape(3, 3)


def entzerren(im, ecken):
    q = np.array(ecken, float)
    kante = lambda a, b: np.hypot(*(q[a] - q[b]))
    W = int(round((kante(0, 1) + kante(3, 2)) / 2))
    H = int(round((kante(0, 3) + kante(1, 2)) / 2))
    Hm = homographie(q, [(0, 0), (W, 0), (W, H), (0, H)])

    xs, ys = np.meshgrid(np.arange(W) + .5, np.arange(H) + .5)
    p = np.stack([xs.ravel(), ys.ravel(), np.ones(xs.size)])
    s = Hm @ p
    sx, sy = s[0] / s[2], s[1] / s[2]

    a = np.asarray(im, float)
    ih, iw, _ = a.shape
    sx = np.clip(sx, 0, iw - 1.001); sy = np.clip(sy, 0, ih - 1.001)
    x0, y0 = np.floor(sx).astype(int), np.floor(sy).astype(int)
    fx, fy = (sx - x0)[:, None], (sy - y0)[:, None]
    out = (a[y0, x0] * (1 - fx) * (1 - fy) + a[y0, x0 + 1] * fx * (1 - fy) +
           a[y0 + 1, x0] * (1 - fx) * fy + a[y0 + 1, x0 + 1] * fx * fy)
    return Image.fromarray(out.reshape(H, W, 3).clip(0, 255).astype(np.uint8))


def beschneiden(im, r):
    w, h = im.size
    l, o, re, u = r
    return im.crop((int(w * l), int(h * o), int(w * (1 - re)), int(h * (1 - u))))


def farben(im, sw=False, levels=.8, sat=1.15, gamma=1.0):
    a = np.asarray(im, float)
    if sw:                                   # Papiergilb raus, echtes Graustufenbild
        g = a @ np.array([.299, .587, .114])
        a = np.repeat(g[:, :, None], 3, 2)
    korr = a.copy()
    for c in range(3):                       # Tonwerte je Kanal spreizen = Weissabgleich
        lo, hi = np.percentile(a[:, :, c], (.6, 99.4))
        if hi - lo < 1:
            continue
        korr[:, :, c] = (a[:, :, c] - lo) * (255.0 / (hi - lo))
    a = np.clip(a * (1 - levels) + korr * levels, 0, 255)
    if gamma != 1.0:
        a = 255.0 * (a / 255.0) ** gamma
    im = Image.fromarray(a.clip(0, 255).astype(np.uint8))
    if sat != 1.0 and not sw:
        im = ImageEnhance.Color(im).enhance(sat)
    return im.filter(ImageFilter.UnsharpMask(radius=1.6, percent=95, threshold=3))


print(f"{'Ausgabe':30s} {'entzerrt':>12s}  {'max. Druckbreite bei 300 dpi':>28s}")
print("-" * 76)
ecken = json.load(open(f"{SCR}/ecken.json"))
for key, (name, rand, opt) in PLAN.items():
    im = Image.open(f"{SRC}/{DATEIEN[key]}").convert("RGB")
    if key in ecken:
        im = entzerren(im, ecken[key])
    im = beschneiden(im, rand)
    im = farben(im, **opt)
    im.save(f"{ZIEL}/{name}.jpg", quality=94, subsampling=0, dpi=(300, 300))
    w, h = im.size
    print(f"{name + '.jpg':30s} {f'{w}x{h}':>12s}  {w / 300 * 25.4:24.0f} mm")
