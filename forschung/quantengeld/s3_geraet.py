"""Weg C: Welches Geraetepaar reicht? (Gerätevorgabe, WEG_C.md C4/C5)

Rechnet aus den Messgroessen eines Senders (schwache Laserpulse) und eines
Empfaengers, ob die Kette einen Token ueber diese Leitung annimmt, wie viele
Pulse dafuer noetig sind und wie oft eine ECHTE Note faelschlich abgewiesen
wird. Grundlage sind dieselben Regeln wie im Knoten:

  - Pruefgroesse k = wk.k_leitung(tol, t, v, theta, frei): ein Faelscher
    besteht hoechstens mit 2^-40 (Tabelle K_LEITUNG, jede Zelle exakt
    nachgerechnet);
  - frei = Anteil der Mehrphotonen-Pulse unter den gemeldeten (vorsichtig:
    jeder Mehrphotonen-Puls gilt als angekommen und dem Faelscher bekannt),
    mit derselben Sicherheitsmarge wie wiesner_parameter.s3_stichprobe;
  - 2k <= n <= N_MAX (16.384), gemeldet <= MELDUNG_MAX (65.536).

Modell (Poisson-Quelle, keine Dunkelzaehlungen -- die gehen in e ein):
  gamma = 1 - exp(-mu * eta * T)       Anteil der Pulse, die gemessen werden
  p2    = 1 - (1 + mu) exp(-mu)        Anteil der Pulse mit >= 2 Photonen
  frei  = p2' / gamma                  p2' = p2 mit Marge (Hoeffding/Chernoff)

Ehrlich: Das ist eine Vorgabe aus der Spezifikation, keine Messung. Erst die
Messung am Geraet (C5: Fehler e, Verkippung theta, mu, eta, Seitenkanaele)
entscheidet. Die Werte unten sind Beispiele aus Datenblaettern und Literatur.

Aufruf:  python3 -I s3_geraet.py              Tabelle der Beispiele
         python3 -I s3_geraet.py --selbsttest
"""
import importlib.util
import math
import os
import sys


def _laden(name: str):
    spec = importlib.util.spec_from_file_location(name, os.path.join(os.path.dirname(os.path.abspath(__file__)), name + ".py"))
    m = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(m)
    return m


wk = _laden("wiesner_kette")
wp = _laden("wiesner_parameter")

MELDUNG_MAX = 1 << 16          # wie s_kette.MELDUNG_MAX
ZIEL_MARGE = 1e-9              # Marge fuer den Mehrphotonen-Anteil (wie s3_stichprobe)
ZIEL_EHRLICH = wp.ZIEL_EHRLICH  # hoechstens so oft faellt eine echte Note durch


def ehrlich_faellt(k: int, e: float, tol: int) -> float:
    """Wahrscheinlichkeit, dass eine ECHTE Note mit Fehlerrate e bei k
    geprueften Stellen abgewiesen wird (nicht: unentschieden)."""
    echt, unent = wp.annahme(k, e, tol, wk.MIN_TREFFER)
    return max(0.0, 1.0 - echt - unent)


def frei_anteil(mu: float, gamma: float, n_puls: float) -> float:
    p2 = wp.p_mehrphoton(mu)
    if p2 <= 0:
        return 0.0
    nu = p2 * (1 + math.sqrt(3 * math.log(2.000001 / ZIEL_MARGE) / (p2 * n_puls)))
    return nu / gamma


def vorgabe(mu: float, eta: float, T: float, e: float, theta_grad: float, rate_hz: float,
            t: int = 1, v: int = 1) -> dict:
    """Ergebnis fuer ein Geraetepaar. ok = False mit Grund, wenn die Kette
    keinen Token ueber diese Leitung annaehme."""
    gamma = 1 - math.exp(-mu * eta * T)
    theta_z = math.ceil(theta_grad * 10 - 1e-9)
    aus = {"mu": mu, "eta": eta, "T": T, "e": e, "theta": theta_grad, "t": t, "v": v, "gamma": gamma}
    if theta_z > wk.THETA_ZEHNTEL_MAX:
        return {**aus, "ok": False, "grund": "Verkippung ueber 10 Grad"}
    # n_puls haengt von k ab und k von frei (ueber n_puls): von oben
    # schaetzen (wenige Pulse = groessere Marge) und zweimal nachziehen.
    n_puls = MELDUNG_MAX / gamma
    bestes = None
    for _ in range(3):
        frei = frei_anteil(mu, gamma, n_puls)
        frei_ppm = math.ceil(frei * 1e6)
        if frei_ppm > wk.FREI_PPM_MAX:
            return {**aus, "frei": frei, "ok": False, "grund": f"Mehrphotonen-Anteil {frei:.1%} > 25 %"}
        bestes = None
        for tol in wk.TOLERANZEN:
            if tol <= e * 100:
                continue
            k = wk.k_leitung(tol, t, v, theta_z, frei_ppm)
            if k == 0 or 2 * k > wk.N_MAX:
                continue
            f = ehrlich_faellt(k, e, tol)
            if f <= ZIEL_EHRLICH:
                bestes = (k, tol, f)
                break
        if bestes is None:
            return {**aus, "frei": frei, "ok": False, "grund": "keine Toleranz mit sicherer Pruefgroesse und 2k <= 16.384"}
        k = bestes[0]
        n = 2 * k
        n_puls = n / gamma
    k, tol, f = bestes
    gemeldet = 2 * k
    if gemeldet > MELDUNG_MAX:
        return {**aus, "ok": False, "grund": "Meldung zu gross"}
    return {**aus, "ok": True, "frei": frei, "tol": tol, "k": k, "n": gemeldet, "n_puls": math.ceil(n_puls),
            "sekunden": n_puls / rate_hz, "ehrlich_faellt": f}


# Beispiele. Annahmen, keine Messungen: eta = Detektor x Kopplung (Si-SPAD bei
# 850 nm laut Haendlerangabe ~58 %), T = Durchlass der Strecke, e = Fehlerrate
# je Treffer. Erst C5 (Messung am Geraet) setzt die echten Werte.
GERAET = {"eta": 0.50, "T": 0.90, "e": 0.020, "theta": 2.0, "rate": 1e6}
BEISPIELE = [
    # name, mu, eta, T, e, theta, rate
    ("Eigenbau, mu 0,10", 0.10, 0.50, 0.90, 0.020, 2.0, 1e6),
    ("Eigenbau, schlechter justiert (e 4 %, 5 Grad, eta 0,40, T 0,80)", 0.10, 0.40, 0.80, 0.040, 5.0, 1e6),
    ("zu viel Licht: mu 0,5", 0.50, 0.50, 0.90, 0.020, 2.0, 1e6),
    ("zu viel Verlust: eta 0,10, T 0,50", 0.10, 0.10, 0.50, 0.020, 2.0, 1e6),
]
ABSPRACHE = [(1, 1), (2, 3), (3, 5), (4, 7)]
MUS = [0.03, 0.05, 0.10, 0.20]


def tabelle():
    g = GERAET
    print(f"Gerätevorgabe Weg C: eta {g['eta']}, T {g['T']}, e {g['e']:.0%}, theta {g['theta']} Grad, {g['rate']:.0e} Pulse/s\n")
    print("Prüfgröße k je Annahmestelle (Pulse je Token) nach Photonenzahl mu und Schwelle t von v:")
    print("mu     | " + " | ".join(f"t {t} von {v}" for t, v in ABSPRACHE))
    for mu in MUS:
        zeile = []
        for t, v in ABSPRACHE:
            r = vorgabe(mu, g["eta"], g["T"], g["e"], g["theta"], g["rate"], t=t, v=v)
            zeile.append(f"k {r['k']} ({r['n_puls'] / 1e6:.2f} M)" if r["ok"] else "nein")
        print(f"{mu:.2f}   | " + " | ".join(zeile))
    print("\nEinzelne Fälle (t 1 von 1):")
    for name, mu, eta, T, e, th, rate in BEISPIELE:
        r = vorgabe(mu, eta, T, e, th, rate)
        if r["ok"]:
            print(f"- {name}: JA. gamma {r['gamma']:.3f}, frei {r['frei']:.1%}, tol {r['tol']} %, k {r['k']}, "
                  f"n {r['n']}, Pulse {r['n_puls']:,} ({r['sekunden']:.2f} s), echte Note fällt {r['ehrlich_faellt']:.1e}")
        else:
            print(f"- {name}: NEIN – {r['grund']}")


def selbsttest() -> bool:
    fehler = []

    def soll(b, text):
        if not b:
            fehler.append(text)

    gut = vorgabe(0.10, 0.50, 0.90, 0.020, 2.0, 1e6)
    soll(gut["ok"] and 2 * gut["k"] <= wk.N_MAX and gut["n"] <= MELDUNG_MAX, "Eigenbau mit guten Werten geht")
    soll(gut["ok"] and gut["k"] == wk.k_leitung(gut["tol"], 1, 1, 20, math.ceil(gut["frei"] * 1e6)), "k aus der Tabelle der Kette")
    soll(gut["ok"] and gut["ehrlich_faellt"] <= ZIEL_EHRLICH, "echte Note faellt selten")
    soll(gut["ok"] and gut["tol"] > 100 * gut["e"], "Toleranz ueber der Fehlerrate")
    soll(not vorgabe(0.5, 0.5, 0.9, 0.02, 2.0, 1e6)["ok"], "zu viel Licht: frei > 25 %")
    soll(not vorgabe(0.1, 0.5, 0.9, 0.02, 10.5, 1e6)["ok"], "Verkippung > 10 Grad")
    soll(not vorgabe(0.1, 0.5, 0.9, 0.12, 2.0, 1e6)["ok"], "Fehlerrate 12 %: keine sichere Toleranz")
    # Monoton: mehr Licht = mehr frei; mehr Verlust = mehr frei.
    soll(frei_anteil(0.2, 0.1, 1e6) > frei_anteil(0.1, 0.1, 1e6) * 1.5, "frei waechst mit mu")
    a = vorgabe(0.10, 0.50, 0.90, 0.020, 2.0, 1e6)
    b = vorgabe(0.10, 0.25, 0.90, 0.020, 2.0, 1e6)
    soll(a["ok"] and (not b["ok"] or b["frei"] > a["frei"]), "weniger Wirkungsgrad = mehr frei")
    # Absprache macht es teurer, nie billiger.
    c = vorgabe(0.10, 0.50, 0.90, 0.020, 2.0, 1e6, t=3, v=5)
    soll(not c["ok"] or c["k"] >= a["k"], "Absprache senkt k nicht")
    # Ehrliche Note: bei e = tol faellt sie oft.
    soll(ehrlich_faellt(1000, 0.04, 4) > 0.3, "e = tol faellt haeufig")
    for f in fehler:
        print("FEHLER:", f)
    print("s3_geraet: Selbsttest", "bestanden" if not fehler else "NICHT bestanden")
    return not fehler


if __name__ == "__main__":
    if "--selbsttest" in sys.argv:
        sys.exit(0 if selbsttest() else 1)
    tabelle()
