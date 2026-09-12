# Eine Box aus /api/health/combined pruefen -- gerufen von wache.yml.
import sys, json
d = json.load(sys.stdin)
rot = []
def g(*ks, default=None):
    o = d
    for k in ks:
        if not isinstance(o, dict) or k not in o: return default
        o = o[k]
    return o
if not g("proof_server", "reachable"): rot.append("Proof-Server nicht erreichbar -- keine Registrierung moeglich")
if g("divergenz", "abweichend"): rot.append("Kontenstand weicht vom Partner ab (divergenz.abweichend) -- Resync noetig")
if (g("zustands_ablehnung", "uebersprungene_ueberweisungen") or 0) > 0: rot.append("Ueberweisungen beim Nachspielen uebersprungen: %s -- Knoten weichen ab" % g("zustands_ablehnung", "uebersprungene_ueberweisungen"))
if g("plattenplatz", "kritisch"): rot.append("Platte kritisch (%s MB frei)" % g("plattenplatz", "frei_mb"))
if (g("plattenplatz", "belegt_pct") or 0) > 85: rot.append("Platte zu %.0f %% belegt" % g("plattenplatz", "belegt_pct"))
if g("chain", "dag_degraded"): rot.append("Knoten degraded: %s" % g("chain", "dag_degraded_reason"))
if g("totmann", "ausgeloest"): rot.append("Totmann ausgeloest")
if g("receipt_flush", "fehler", default=0) and (g("receipt_flush", "puffer") or 0) > 500000: rot.append("Quittungs-Puffer staut: %s" % g("receipt_flush", "puffer"))
for r in rot: print("✗ " + r)
if not rot: print("✓ Proof-Server erreichbar, keine Divergenz, nichts uebersprungen, Platte %.0f %%, nicht degraded" % (g("plattenplatz", "belegt_pct") or 0))
sys.exit(1 if rot else 0)
