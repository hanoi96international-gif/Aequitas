#!/usr/bin/env python3
"""Nachpruefung von kohaerent.py mit EXAKTEN SDPs (optional, nicht in der CI).

Braucht cvxpy und numpy (pip install cvxpy==1.9.3 numpy) -- darum nicht im
CI-Selbsttest, der ohne Abhaengigkeiten laeuft. Jede Zeile vergleicht das
Optimum ueber ALLE Strategien (auch verschraenkt bzw. adaptiv) mit dem Wert,
den kohaerent.py annimmt. Stand 17.11.2026 (SCS, eps 1e-8/1e-9):

  W  max F_A + F_B, beliebiger Kloner 1 -> 2       1,707107 = 1 + 1/sqrt 2
  W  Wert je Stelle (q = 1/2, tol 4 %, s = 1,1)     0,995228 = Formel
  W  zwei Stellen gemeinsam (verschraenkt)          0,990478 = Quadrat
  S  beste Antwort in beiden Basen                  0,853553 = 1/2 + 1/sqrt 8
  S  zwei Stellen gemeinsam (Gewicht s = 1,3)       1,577702 = Quadrat
  W  adaptiv (Kamm mit zwei Runden), eine Stelle    0,995228 = nicht adaptiv
  W  adaptiv, zwei Stellen gemeinsam                0,990478 = Quadrat (3 min 48 s)

Aufruf:  python3 sdp_nachpruefung.py
"""
# ---------------------------------------------------------------- W: Kloner
import numpy as np, cvxpy as cp, math, itertools
k0=np.array([1,0],complex);k1=np.array([0,1],complex)
p=(k0+k1)/math.sqrt(2);m=(k0-k1)/math.sqrt(2)
bb84=[k0,k1,p,m]; I2=np.eye(2)
def proj(v): return np.outer(v,v.conj())
def R_eins(gewicht):
    # gewicht(P) -> Operator auf (A,B) (4x4); R = 1/4 sum conj(psi) (x) W_psi
    return sum(np.kron(proj(v).conj(), gewicht(proj(v))) for v in bb84)/4
def sdp(R, din, dout):
    J=cp.Variable((din*dout,din*dout),hermitian=True)
    # partial trace over out
    cons=[J>>0, cp.partial_trace(J,[din,dout],axis=1)==np.eye(din)]
    pr=cp.Problem(cp.Maximize(cp.real(cp.trace(J@R))),cons); pr.solve(solver="SCS",eps=1e-9,max_iters=200000)
    return pr.value
# (a) Summe der Treffsicherheiten
R=R_eins(lambda P: np.kron(P,I2)+np.kron(I2,P))
print("(a) max F_A+F_B =", round(sdp(R,2,4),6), " 1+1/sqrt2 =", round(1+1/math.sqrt(2),6))
# (b) Erzeugende Funktion je Stelle, q=1/2, tau=0.04, s=1.1
q,tau,s=0.5,0.04,1.1
def W(P):
    A=np.kron(P,I2); B=np.kron(I2,P); I4=np.eye(4)
    tr=lambda X: (q/2)*s**tau*((1-1/s)*X+I4/s)
    return (1-q)*I4 + tr(A)+tr(B)
R1=R_eins(W); v1=sdp(R1,2,4)
formel=1-q+(q/2)*s**tau*((1+1/math.sqrt(2))*(1-1/s)+2/s)
print("(b) eine Stelle: SDP", round(v1,6), " Formel", round(formel,6))
# zwei Stellen: R1(in1,out1) (x) R1(in2,out2) -> (in1,in2,out1,out2)
T=np.kron(R1,R1).reshape([2,4,2,4]*2)
T=T.transpose(0,2,1,3,4,6,5,7).reshape(64,64)
v2=sdp(T,4,16)
print("(b) zwei Stellen gemeinsam (auch verschraenkt): SDP", round(v2,6), " Quadrat", round(v1**2,6))

# ---------------------------------------------------------------- S: zwei Basen
zust=[(0,0,k0),(0,1,k1),(1,0,p),(1,1,m)]   # (basis, wert, vektor)
proj=lambda v: np.outer(v,v.conj())
def gewicht_eins(b,t,a):     # a = (antwort Z, antwort X); gefragt wird die Basis b
    return 1.0 if a[b]==t else 0.0
def sdp_s(dim, antworten, Q):
    M={a:cp.Variable((dim,dim),hermitian=True) for a in antworten}
    cons=[M[a]>>0 for a in antworten]+[sum(M.values())==np.eye(dim)]
    pr=cp.Problem(cp.Maximize(cp.real(sum(cp.trace(M[a]@Q[a]) for a in antworten))),cons)
    pr.solve(solver="SCS",eps=1e-9,max_iters=200000); return pr.value
A1=list(itertools.product([0,1],[0,1]))
Q1={a:sum(gewicht_eins(b,t,a)*proj(v) for b,t,v in zust)/4 for a in A1}
v1=sdp_s(2,A1,Q1)
print("S, eine Stelle: max Treffsicherheit", round(v1,6), " 1/2+1/sqrt8 =", round(0.5+1/math.sqrt(8),6))
# erzeugende Funktion mit Gewicht s je richtiger Antwort, zwei Stellen gemeinsam
s=1.3
w1=lambda b,t,a: (s if a[b]==t else 1.0)
Q1s={a:sum(w1(b,t,a)*proj(v) for b,t,v in zust)/4 for a in A1}
g1=sdp_s(2,A1,Q1s)
A2=list(itertools.product(A1,A1))
Q2={(a,c):np.kron(Q1s[a],Q1s[c]) for a,c in A2}
g2=sdp_s(4,A2,Q2)
print("S, Gewicht s=1.3: eine Stelle", round(g1,6), " zwei Stellen gemeinsam", round(g2,6), " Quadrat", round(g1**2,6))

# ---------------------------------------------------------------- W adaptiv: Kamm
q,tau,s=0.5,0.04,1.1
W=lambda P: 0.5*I2+0.5*s**tau*((1-1/s)*P+I2/s)        # Gewicht einer gefragten Kopie
marken=[(q,'A'),(q,'B'),(1-2*q,'-')]
def R_stelle():
    R=np.zeros((2*2*3*2,)*2,complex)
    for v in bb84:
        P=proj(v)
        for j,(pm,mk) in enumerate(marken):
            e=np.zeros((3,3));e[j,j]=1
            WA=W(P) if mk=='A' else I2; WB=W(P) if mk=='B' else I2
            R+=0.25*pm*np.kron(np.kron(np.kron(P.conj(),WA),e),WB)
    return R
def kamm(R, d_in1, d_a, d_in2, d_b):
    D=d_in1*d_a*d_in2*d_b
    Q=cp.Variable((D,D),hermitian=True); Q1=cp.Variable((d_in1*d_a,d_in1*d_a),hermitian=True)
    trB=cp.partial_trace(Q,[d_in1*d_a*d_in2,d_b],axis=1)            # (in1 A in2)
    # Tr_B Q = Q1 (x) I_in2  (Reihenfolge in1 A in2)
    cons=[Q>>0,Q1>>0, trB==cp.kron(Q1,np.eye(d_in2)), cp.partial_trace(Q1,[d_in1,d_a],axis=1)==np.eye(d_in1)]
    pr=cp.Problem(cp.Maximize(cp.real(cp.trace(Q@R))),cons); pr.solve(solver="SCS",eps=1e-8,max_iters=300000)
    return pr.value
R1=R_stelle()
v1=kamm(R1,2,2,3,2)
formel=1-q+(q/2)*s**tau*((1+1/math.sqrt(2))*(1-1/s)+2/s)
print("adaptiv, eine Stelle:", round(v1,6), " Formel (nicht adaptiv):", round(formel,6))
# zwei Stellen: Reihenfolge (in1a in1b, Aa Ab, in2a in2b, Ba Bb)
T=np.kron(R1,R1).reshape([2,2,3,2]*4)
# Achsen: zeile (a: 0..3, b: 4..7), spalte (a: 8..11, b: 12..15)
T=T.transpose(0,4,1,5,2,6,3,7, 8,12,9,13,10,14,11,15).reshape(576,576)
v2=kamm(T,4,4,9,4)
print("adaptiv, zwei Stellen gemeinsam:", round(v2,6), " Quadrat:", round(v1**2,6))
