# Eine Box aus /api/health/combined pruefen -- gerufen von wache.yml.
import sys, json, os, time
d = json.load(sys.stdin)
rot = []
def g(*ks, default=None):
    o = d
    for k in ks:
        if not isinstance(o, dict) or k not in o: return default
        o = o[k]
    return o
if not g("proof_server", "reachable"): rot.append("Proof-Server nicht erreichbar -- keine Registrierung moeglich")
# Seit 13.09.2026: das Tor muss ZU sein. "optional"/"off" heisst Registrierung ohne
# Gesichtspruefung -- an dem Tag stand es 14 Stunden so, ohne dass jemand es sah.
_modus = str(g("proof_server", "last_status", "durchsetzung", "mode", default="") or "").strip().lower()
if g("proof_server", "reachable") and _modus != "required": rot.append("Proof-Server BIO_ATTESTATION_MODE=%r statt required -- Registrierung ohne Gesichtspruefung moeglich" % _modus)
# Seit 20.10.2026: das Tor vor dem Praegen (prove_provenance.go). Aus -- oder ein
# Knoten, der es nicht meldet -- heisst: ein lokal erzeugter Beweis praegt 1.000 AEQ
# ohne Gesichtspruefung. Fehlt das Feld, ist das rot (fail-closed).
if g("prove_herkunft", "verlangt") is not True: rot.append("Knoten verlangt keine /prove-Herkunft (prove_herkunft.verlangt=%r) -- Praegen ohne Gesichtspruefung moeglich; REQUIRE_PROVE_PROVENANCE pruefen" % g("prove_herkunft", "verlangt"))
if g("divergenz", "abweichend"): rot.append("Kontenstand weicht vom Partner ab (divergenz.abweichend) -- Resync noetig")
if (g("zustands_ablehnung", "uebersprungene_ueberweisungen") or 0) > 0: rot.append("Ueberweisungen beim Nachspielen uebersprungen: %s -- Knoten weichen ab" % g("zustands_ablehnung", "uebersprungene_ueberweisungen"))
if g("plattenplatz", "kritisch"): rot.append("Platte kritisch (%s MB frei)" % g("plattenplatz", "frei_mb"))
if (g("plattenplatz", "belegt_pct") or 0) > 85: rot.append("Platte zu %.0f %% belegt" % g("plattenplatz", "belegt_pct"))
if g("chain", "dag_degraded"): rot.append("Knoten degraded: %s" % g("chain", "dag_degraded_reason"))
if g("totmann", "ausgeloest"): rot.append("Totmann ausgeloest")
# Seit 23.11.2026 (Quantengeld Weg B, blockzeit_waechter.go): der Takt erzeugt
# jede Sekunde einen Block, auch leer. Liegt der neueste Block mehr als
# BLOCKZEIT_RUECKSTAND hinter der Uhr DIESER Wache, haengt der Knoten -- oder
# die Blockzeit wird zurueckgehalten (dann waere die drand-Runde eines Loses
# schon bekannt). Verglichen wird mit der eigenen Uhr, nie mit der des Knotens.
# Fehlt das Feld (Knoten vor diesem Deploy), nur ein Hinweis.
BLOCKZEIT_RUECKSTAND = 300
_jetzt = int(os.environ.get("WACHE_JETZT") or time.time())
_nb = g("chain", "neuester_block_zeit")
if _nb is None:
    print("Hinweis: Knoten meldet chain.neuester_block_zeit noch nicht (vor dem Deploy von B2d)")
elif not isinstance(_nb, int) or isinstance(_nb, bool) or _nb <= 0:
    rot.append("chain.neuester_block_zeit ungueltig: %r" % (_nb,))
elif _jetzt - _nb > BLOCKZEIT_RUECKSTAND:
    rot.append("neuester Block %d s hinter der Uhr der Wache (Grenze %d s) -- Knoten haengt oder Blockzeit wird zurueckgehalten" % (_jetzt - _nb, BLOCKZEIT_RUECKSTAND))
if g("receipt_flush", "fehler", default=0) and (g("receipt_flush", "puffer") or 0) > 500000: rot.append("Quittungs-Puffer staut: %s" % g("receipt_flush", "puffer"))
for r in rot: print("✗ " + r)
if not rot: print("✓ Proof-Server erreichbar und required, /prove-Herkunft verlangt, keine Divergenz, nichts uebersprungen, Platte %.0f %%, nicht degraded" % (g("plattenplatz", "belegt_pct") or 0))
sys.exit(1 if rot else 0)
