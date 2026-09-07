#!/usr/bin/env python3
"""D-30: benchmark reproducible de hubness vs calidad de recuperacion.

No confunde recall contra ground-truth euclidiano con calidad semantica:
si cambia la metrica, ese GT deja de ser neutral. Mide ademas precision@10
por etiquetas, con control positivo y negativo.

Corrido: CPython 3.12.13, numpy/scipy/sklearn, seed=7.
"""
import os, sys
import numpy as np
from scipy.stats import skew
from sklearn.cluster import KMeans
from sklearn.datasets import load_digits

SEED, K = 7, 10
rng = np.random.default_rng(SEED)
fallos = 0

def check(kind, name, ok, detail=""):
    global fallos
    print(f"  [{'ok  ' if ok else 'ROJO'}] {kind:<16}{name} {detail}")
    if not ok: fallos += 1

def weighted_dist(X, Q, precision):
    if precision.ndim == 1:
        a, b = X*np.sqrt(precision), Q*np.sqrt(precision)
        return (b*b).sum(1)[:,None] + (a*a).sum(1)[None,:] - 2*b@a.T
    out = np.empty((len(Q),len(X)))
    for i,q in enumerate(Q): out[i] = ((X-q)**2*precision[i]).sum(1)
    return out

def knn(D, k=K):
    D=D.copy(); np.fill_diagonal(D,np.inf)
    return np.argpartition(D,k,axis=1)[:,:k]

def hub(nn,n):
    c=np.bincount(nn.ravel(),minlength=n)
    return (0.0 if np.all(c==c[0]) else float(skew(c))),c

def precision10(nn,y): return float((y[nn]==y[:,None]).mean())

def aniso(n=3000,d=100,classes=10,ratio=1000):
    scales=np.geomspace(ratio,1,d)
    centers=rng.normal(0,1,(classes,d))*np.sqrt(scales)*.35
    y=rng.integers(0,classes,n)
    return centers[y]+rng.normal(0,1,(n,d))*np.sqrt(scales),y

def iso(n=3000,d=100,classes=10):
    centers=rng.normal(0,1,(classes,d))*.9; y=rng.integers(0,classes,n)
    return centers[y]+rng.normal(0,1,(n,d)),y

def nuisance(n=3000,d=64,classes=10):
    centers=rng.normal(0,1,(classes,d))*.9; y=rng.integers(0,classes,n)
    signal=centers[y]+rng.normal(0,1,(n,d)); noise=rng.normal(0,60,(n,8))
    return np.hstack([signal,noise]),y

def local_precision(X,Q,k):
    km=KMeans(n_clusters=k,n_init=4,random_state=SEED).fit(X)
    gv=X.var(0); P=[]; small=0
    for c in range(k):
        z=X[km.labels_==c]
        if len(z)<10*X.shape[1]: small+=1
        v=z.var(0) if len(z)>=2 else np.zeros(X.shape[1])
        P.append(1/np.maximum(np.maximum(v,gv*.1),1e-8))
    return np.asarray(P)[km.predict(Q)],small

def run(name,X,y,k=None):
    n,d=X.shape; k=k or max(2,min(40,n//max(1,10*d)))
    methods={"euclidiana":np.ones(d),"mahal_global_diag":1/np.maximum(X.var(0),1e-12)}
    p,small=local_precision(X,X,k); methods["mahal_local_diag"]=p
    # cosine is L2 after row normalization
    Z=X/np.maximum(np.linalg.norm(X,axis=1,keepdims=True),1e-12)
    results={}
    for m,w in methods.items():
        nn=knn(weighted_dist(X,X,w)); s,_=hub(nn,n)
        results[m]=(s,precision10(nn,y),nn)
    nn=knn(weighted_dist(Z,Z,np.ones(d))); results["coseno"]=(hub(nn,n)[0],precision10(nn,y),nn)
    print(f"\n== {name}: n={n}, d={d}, clusters={k}, small_clusters={small} ==")
    base=results["euclidiana"]
    print(f"  {'metrica':<22}{'hub_skew':>10}{'prec@10':>10}{'d_hub':>10}{'d_prec':>10}")
    for m in ("euclidiana","coseno","mahal_global_diag","mahal_local_diag"):
        s,p,_=results[m]; print(f"  {m:<22}{s:>10.3f}{p:>10.4f}{s-base[0]:>+10.3f}{p-base[1]:>+10.4f}")
    return results,k

print("D-30 | seed=7 | k=10 | kNN exacto por fuerza bruta")
print("INSTRUMENTO: precision@10 por etiqueta, no recall contra GT de otra metrica")
# Instrument invariants
X,y=iso(400,20,5); D=weighted_dist(X,X,np.ones(20)); E=((X[:,None]-X[None,:])**2).sum(-1)
check("INVARIANTE", "w=1 reproduce distancia euclidiana", np.allclose(D,E,atol=1e-6), f"maxerr={abs(D-E).max():.2e}")
check("INVARIANTE", "kNN exacto excluye self", not any(i in knn(D)[i] for i in range(len(X))))
# positive control: nuisance dimensions must be downweighted
Xp,yp=nuisance(); rp,_=run("CONTROL POSITIVO: ruido de alta varianza",Xp,yp)
check("CONTROL POSITIVO", "diagonal mejora precision >5 puntos", rp["mahal_global_diag"][1]>rp["euclidiana"][1]+.05, f"{rp['euclidiana'][1]:.4f}->{rp['mahal_global_diag'][1]:.4f}")
# negative control: isotropic data should not move quality materially
Xi,yi=iso(); ri,_=run("CONTROL NEGATIVO: isotropico",Xi,yi)
check("CONTROL NEGATIVO", "precision diagonal cambia <.02", abs(ri["mahal_global_diag"][1]-ri["euclidiana"][1])<.02)
# claim dataset and a real labeled dataset
Xa,ya=aniso(); ra,k=run("MEDIDO: sintetico anisotropo",Xa,ya)
d=load_digits(); Xd=d.data.astype(float); rd,_=run("MEDIDO: digits real sklearn",Xd,d.target)
# bad metric ground truth, printed only to expose the trap
for name,r in (("aniso",ra),("digits",rd)):
    b=r["euclidiana"][2]
    vals=[]
    for m in ("mahal_global_diag","mahal_local_diag"):
        vals.append(np.mean([len(set(r[m][2][i])&set(b[i]))/K for i in range(len(b))]))
    print(f"  {name}: recall@10 vs GT euclidiano, global={vals[0]:.3f}, local={vals[1]:.3f} (NO es calidad semantica)")
# measured contradiction
hub_up=ra["mahal_global_diag"][0]>ra["euclidiana"][0]
prec_up=ra["mahal_global_diag"][1]>ra["euclidiana"][1]+.03
print("\n== HALLAZGO D-30 ==")
print(f"  anisotropo: Mahalanobis SUBE hubness {ra['euclidiana'][0]:.3f}->{ra['mahal_global_diag'][0]:.3f}")
print(f"  anisotropo: Mahalanobis SUBE precision@10 {ra['euclidiana'][1]:.4f}->{ra['mahal_global_diag'][1]:.4f}")
print(f"  digits real: Mahalanobis cambia precision {rd['mahal_global_diag'][1]-rd['euclidiana'][1]:+.4f}")
check("INVARIANTE", "hubness y calidad no son claims equivalentes en esta corrida", hub_up and prec_up)
print("  VEREDICTO: queda FALSADA la cadena 'menos hubness => mejor recall'.")
print("  El claim de -64% hubness y +4.3% recall NO puede salir de una misma tabla")
print("  sin explicar que son objetivos distintos y sin usar un benchmark HNSW real.")
print("\n== NO MEDIDO ==")
for x in [
 "GloVe-100: sin red ni archivo local; se acepta como ./glove.6B.100d.txt",
 "HNSW real: este instrumento usa kNN exacto, no construccion/pruning de grafo",
 "QPS, latencia, SIFT/Deep-1M y embeddings d>=512 reales",
 "queries OOD y efecto de bypass OOD",
]: print("  "+x)
print(f"\nfallos={fallos}")
sys.exit(1 if fallos else 0)
