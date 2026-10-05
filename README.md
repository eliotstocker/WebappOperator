# webapp-operator

A Kubernetes-native platform for hosting and serving Single Page Applications (SPAs) with GitOps, OCI artifacts, and dynamic runtime configuration.

> **Note on Creation & AI Assistance:**
> The idea, problem definition, and architectural direction behind `webapp-operator` were conceived, designed, and guided by human engineers. The underlying implementation, unit/E2E test suites, deployment manifests, and documentation were developed with strong assistance and augmentation from AI.

---

## Why webapp-operator?

Deploying SPAs in Kubernetes traditionally falls into two painful patterns:
1. **The Container Bloat Model:** Packaging Nginx + static files into a new container image for every commit. This wastes 50–100MB of RAM per idle pod, slows down CI builds, and requires rebuilding bundles just to update an API URL.
2. **The "Broken GitOps" Model:** Pushing static assets to external object storage (AWS S3, Cloudflare Pages). This decouples frontend releases from cluster GitOps (ArgoCD/Flux) and breaks preview environments.

**`webapp-operator` bridges this gap.** Frontends are packaged as lightweight OCI artifacts (just the static tarball) and deployed as native Kubernetes Custom Resources (`Website`). A shared, multi-tenant gateway fleet caches and serves the assets with dynamic runtime environment injection.

---

## Key Features

* 📦 **Standard OCI Distribution:** Push static builds to any container registry (GHCR, ECR, Harbor) using `oras` or `docker buildx` with `FROM scratch`.
* ⚙️ **Runtime Configuration Injection:** Change environment variables without rebuilding JS bundles. Injects `window.__ENV__` dynamically via a CSP-compliant endpoint (`/_config.js`) or inline `<script>`.
* 🛡️ **Zero "Chunk 404s":** Multi-revision caching retains previous build chunks across releases, preventing broken navigation for users with open browser tabs.
* ⚡ **Zero External Storage Requirements:** Unpacks directly into pod `emptyDir` volumes. Works instantly on Kind, Minikube, bare metal, or cloud clusters.
* 🔄 **Synchronized Rollouts:** The elected leader coordinates rollout health with Kubernetes `EndpointSlice` to ensure all active pods have cached assets before switching traffic.
* 🌐 **Optional Managed Ingress:** Automatically reconciles standard Kubernetes `Ingress` objects when requested, or sits behind your existing Gateway API / Ingress.

---

## Quickstart

### 1. Push Static Assets as an OCI Artifact

```bash
# Option A: Using ORAS (Fastest, no Docker daemon)
oras push ghcr.io/my-org/frontend:v1.2.0 ./dist

# Option B: Using standard Docker
cat <<EOF > Dockerfile
FROM scratch
COPY ./dist /
EOF
docker buildx build --push -t ghcr.io/my-org/frontend:v1.2.0 .
```

### 2. Deploy the `Website` Custom Resource

```yaml
apiVersion: webapp.io/v1alpha1
kind: Website
metadata:
  name: dashboard
  namespace: default
spec:
  image: ghcr.io/my-org/frontend:v1.2.0
  hostnames:
    - "dashboard.example.com"
  
  # Dynamic environment variables injected at request time
  env:
    API_URL: "https://api.example.com"
    FEATURE_NEW_NAV: "true"
  
  # Injection mode: "endpoint" (default, CSP-safe) or "inline"
  injection:
    mode: "endpoint"
    path: "/_config.js"

  # Optional: Automatically create and reconcile Ingress
  ingress:
    enabled: true
    className: "nginx"
    tls:
      - hosts:
          - "dashboard.example.com"
        secretName: "dashboard-tls"
```

### 3. Consume Config in your Frontend

In your `index.html`:
```html
<script src="/_config.js"></script>
```

In your application code:
```javascript
const apiUrl = window.__ENV__?.API_URL || "http://localhost:8080";
```

---

## Architecture at a Glance

```
GitOps (ArgoCD / Flux)
         │
         ▼
   Website (CRD) ──► creates ──► WebsiteRevision (CRD)
                                          │
            ┌─────────────────────────────┴─────────────────────────────┐
            ▼                                                           ▼
Gateway Pod (Leader)                                        Gateway Pod (Peer)
• Watches EndpointSlice                                     • Fetches OCI layer to emptyDir
• Aggregates peer readiness                                 • Reports to Leader via internal HTTP
• Flips revision to Active                                  • Serves HTTP / assets / _config.js
```

For the complete technical specification and design details, see [docs/architecture.md](docs/architecture.md).  
For Argo CD GitOps health checks integration, see [docs/argocd.md](docs/argocd.md).

---

## Performance & Benchmarks

The unified operator and gateway is built in Go with zero external runtime dependencies, minimal allocations, and high-concurrency throughput.

### HTTP Serving Benchmarks

*Benchmarked on Apple M1 (8 cores) using Go `testing.B` with `-benchmem`:*

| Endpoint / Workload | Payload Size | Mode | Latency | Memory Allocs | Est. Throughput |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Runtime Config (`/_config.js`)** | ~200 B | Parallel | **698 ns** | 16 allocs / 1.1 KB | **~1,430,000 req/sec** |
| **Runtime Config (`/_config.js`)** | ~200 B | Single-core | **1.35 µs** | 16 allocs / 1.1 KB | **~740,000 req/sec** |
| **SPA Fallback (Inline Injection)** | ~2 KB (`index.html`) | Parallel | **20.6 µs** | 45 allocs / 5.5 KB | **~48,500 req/sec** |
| **SPA Fallback (Inline Injection)** | ~2 KB (`index.html`) | Single-core | **36.5 µs** | 45 allocs / 5.5 KB | **~27,400 req/sec** |
| **Small Static Asset (CSS chunk)** | 5 KB | Parallel | **21.5 µs** | 34 allocs / 8.6 KB | **~46,500 req/sec** |
| **Small Static Asset (CSS chunk)** | 5 KB | Single-core | **37.6 µs** | 34 allocs / 8.6 KB | **~26,600 req/sec** |
| **Large Static Bundle (JS bundle)** | 100 KB | Parallel | **24.8 µs** | 34 allocs / 36 KB | **~40,200 req/sec** (~4.0 GB/s) |
| **Large Static Bundle (JS bundle)** | 100 KB | Single-core | **44.8 µs** | 34 allocs / 36 KB | **~22,300 req/sec** (~2.2 GB/s) |

### Memory & Pod Resource Footprint

During a sustained test of **50,000 requests** across mixed routes:
* **Active Heap In-Use:** **1.55 MB**
* **Total Runtime System Memory (`Sys`):** **18.27 MB**
* **Idle CPU:** `< 0.1%` (a few milliseconds per minute)

Because static asset serving leverages the operating system page cache via `http.ServeFile` and OCI layer extraction streams directly to disk in 32KB chunks via `io.Copy`, memory usage remains flat even under heavy load.

### Recommended Pod Sizing

The default manifests and Helm chart ship with minimal resource footprints:

```yaml
resources:
  requests:
    cpu: 10m        # 0.01 core (idle footprint is negligible)
    memory: 32Mi    # Plenty of headroom over the 18MB base footprint
  limits:
    cpu: 500m       # Burst capacity for 40,000+ RPS
    memory: 128Mi   # Buffer for OCI layer pull and decompression
```

You can run the benchmark suite locally with:
```bash
make bench
```

---

## License

Apache 2.0

