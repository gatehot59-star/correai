#!/usr/bin/env python3
"""Reproducible FAISS HNSW benchmark on public UCI datasets."""
import argparse, hashlib, json, platform, resource, sys, time
from pathlib import Path
from urllib.request import urlopen
import faiss
import numpy as np

DATASETS = {
    "iris_uci": ("https://archive.ics.uci.edu/ml/machine-learning-databases/iris/iris.data", ",", 4),
    "winequality_red_uci": ("https://archive.ics.uci.edu/ml/machine-learning-databases/wine-quality/winequality-red.csv", ";", 11),
}

def skewness(x):
    x = np.asarray(x, dtype="float64")
    sd = x.std()
    return 0.0 if sd == 0 else float(np.mean(((x - x.mean()) / sd) ** 3))

def load_public(url, delimiter, n_features):
    raw = urlopen(url, timeout=60).read()
    text = raw.decode("utf-8", errors="ignore")
    rows = []
    for line in text.splitlines():
        if not line.strip() or line.lower().startswith("fixed acidity"):
            continue
        parts = line.split(delimiter)
        try:
            vals = [float(v) for v in parts[:n_features]]
        except ValueError:
            continue
        if len(vals) == n_features:
            rows.append(vals)
    if len(rows) < 30:
        raise RuntimeError(f"too few rows from {url}: {len(rows)}")
    return np.asarray(rows, dtype="float32"), hashlib.sha256(raw).hexdigest(), len(raw)

def rss_mb():
    return resource.getrusage(resource.RUSAGE_SELF).ru_maxrss / 1024.0

def normalize(x):
    return x / np.maximum(np.linalg.norm(x, axis=1, keepdims=True), 1e-12)

def whiten_fit_apply(train, query):
    mu, scale = train.mean(axis=0), train.std(axis=0)
    scale[scale < 1e-12] = 1.0
    return ((train - mu) / scale).astype("float32"), ((query - mu) / scale).astype("float32")

def exact_gt(train, query, k):
    gt = faiss.IndexFlatL2(train.shape[1])
    gt.add(np.ascontiguousarray(train))
    return gt.search(np.ascontiguousarray(query), k)[1]

def one(name, raw, seed, k, ef, m):
    rng = np.random.default_rng(seed)
    order = rng.permutation(len(raw))
    cut = max(k + 1, int(len(raw) * 0.8))
    train, query = raw[order[:cut]], raw[order[cut:]]
    variants = {"l2": (train, query), "cosine": (normalize(train), normalize(query)), "mahalanobis_diag": whiten_fit_apply(train, query)}
    rows = []
    for metric, (xb, xq) in variants.items():
        gt = exact_gt(xb, xq, k)
        index = faiss.IndexHNSWFlat(xb.shape[1], m, faiss.METRIC_L2)
        index.hnsw.efConstruction, index.hnsw.efSearch = max(ef, 64), ef
        before, t0 = rss_mb(), time.perf_counter()
        index.add(np.ascontiguousarray(xb))
        build_ms = (time.perf_counter() - t0) * 1000
        index_bytes = len(faiss.serialize_index(index))
        lat, found = [], []
        for q in xq:
            q0 = time.perf_counter(); labels = index.search(q.reshape(1, -1), k)[1][0]
            lat.append((time.perf_counter() - q0) * 1000); found.append(labels)
        found = np.asarray(found)
        recall = float(np.mean([len(set(a.tolist()) & set(b.tolist())) / k for a, b in zip(found, gt)]))
        counts = np.bincount(found.ravel(), minlength=len(xb))
        rows.append({"dataset": name, "metric": metric, "n_train": len(xb), "n_query": len(xq), "dim": xb.shape[1], "k": k, "ef_search": ef, "M": m, "recall_at_k": recall, "build_ms": build_ms, "latency_p50_ms": float(np.percentile(lat, 50)), "latency_p95_ms": float(np.percentile(lat, 95)), "qps": float(1000.0 / np.mean(lat)), "rss_delta_mb": max(0.0, rss_mb() - before), "index_bytes": index_bytes, "hubness_skew": skewness(counts)})
    return rows

def main():
    ap = argparse.ArgumentParser(); ap.add_argument("--out", default="hnsw_public_results.json"); ap.add_argument("--seed", type=int, default=20260907); ap.add_argument("--k", type=int, default=10); ap.add_argument("--ef", type=int, default=64); ap.add_argument("--M", type=int, default=16); args = ap.parse_args()
    rows, sources = [], []
    for name, (url, delimiter, dims) in DATASETS.items():
        data, digest, size = load_public(url, delimiter, dims); rows.extend(one(name, data, args.seed, args.k, args.ef, args.M)); sources.append({"dataset": name, "url": url, "sha256": digest, "bytes": size})
    payload = {"instrument": "faiss.IndexHNSWFlat", "faiss_version": faiss.__version__, "python": sys.version, "platform": platform.platform(), "seed": args.seed, "sources": sources, "rows": rows}
    Path(args.out).write_text(json.dumps(payload, indent=2) + "\n"); print(json.dumps(payload, indent=2))

if __name__ == "__main__": main()
