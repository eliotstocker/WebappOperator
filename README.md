# webapp-operator

> **Kinda like GitHub Pages for your Kubernetes cluster.**  
> An efficient, Kubernetes-native way to run as many single page web apps as your heart desires.

> **Note on Creation & AI Assistance:**  
> The idea, problem definition, and architectural direction behind `webapp-operator` were conceived, designed, and guided by human engineers. The underlying implementation, unit/E2E test suites, deployment manifests, and documentation were developed with strong assistance and augmentation from AI.

---

## Why does this exist?

I'm old, right? When I think of single page web apps, I remember FTPing two files onto a server and being done with it. 

Of course things have moved on for the better, but we've somehow turned deploying a web app into a complex and inefficient chore when really we still just need to ship some files and configuration to people's browsers.

`webapp-operator` was built because I wanted a sane, native way to host static frontends on Kubernetes:

* **The "Container Smell":** Packaging Nginx into a container image for every single website commit has a weird smell of *"why do we need all of this?"* If you're serving one or two sites, that's fine. But as soon as you have any sort of scale, running dedicated pods for every frontend is wasteful and clunky.
* **A Single Way to Ship Software:** If your APIs and backends run on Kubernetes, why throw your frontends onto AWS S3, Cloudflare Pages, or Vercel? Keep all your Infrastructure-as-Code in one place, use one registry for all your release candidates, and use a single API for deployment handling.
* **Build Once, Deploy Everywhere:** It’s an antipattern to ship different build artifacts to different environments. Every built asset should be a deployable release candidate for *any* environment. It injects runtime environment variables dynamically so you never have to rebuild your frontend just to update an API URL.

---

## How it works

The developer workflow is simple:
1. You run your build (`npm run build`, `vite build`, etc.).
2. You push the static output directory to your container registry as an OCI artifact (using `oras` or `docker buildx` with `FROM scratch`).
3. You update your Kubernetes manifest.

A shared gateway fleet runs in your cluster, watches your `Website` resources, pulls the static files into a local cache, and serves them with sub-millisecond dynamic configuration injection.

```
GitOps (Argo CD / Flux)
         │
         ▼
   Website (CRD) ──► creates ──► WebsiteRevision (CRD)
                                          │
            ┌─────────────────────────────┴─────────────────────────────┐
            ▼                                                           ▼
Gateway Pod (Leader)                                        Gateway Pod (Peer)
• Watches EndpointSlice                                     • Fetches OCI layer to local cache
• Confirms peer readiness                                   • Reports ready to Leader
• Promotes revision to Active                               • Serves HTTP / assets / _config.js
```

---

## Quickstart

### 1. Push your static build to your registry

You don't need a base OS or web server image—just the static directory:

```bash
# Option A: Using ORAS (Fastest, no Docker daemon needed)
oras push ghcr.io/my-org/my-app:v1.0.0 ./dist

# Option B: Using standard Docker
cat <<EOF > Dockerfile
FROM scratch
COPY ./dist /
EOF
docker buildx build --push -t ghcr.io/my-org/my-app:v1.0.0 .
```

### 2. Deploy your `Website`

```yaml
apiVersion: webapp.io/v1alpha1
kind: Website
metadata:
  name: store
  namespace: default
spec:
  image: ghcr.io/my-org/my-app:v1.0.0
  hostnames:
    - "store.example.com"
  
  # Dynamic environment variables injected at request time
  env:
    API_URL: "https://api.example.com"
    ENVIRONMENT: "production"
  
  # Injection mode: "endpoint" (default, CSP-friendly) or "inline"
  injection:
    mode: "endpoint"
    path: "/_config.js"
    # versionPath: "/_version"  # Custom path for version check (defaults to /_version)
    # Live version update polling is ON by default (checks every 30s)
    # versionPolling: false     # Set to false to disable background polling
    # pollIntervalSeconds: 30   # Custom check interval (defaults to 30)

  # Optional: Automatically create and manage an Ingress
  ingress:
    enabled: true
    className: "nginx"
    tls:
      - hosts:
          - "store.example.com"
        secretName: "store-tls"
```

### 3. Consume config in your frontend

In your `index.html`:
```html
<script src="/_config.js"></script>
```

In your application code:
```javascript
const apiUrl = window.__ENV__?.API_URL || "http://localhost:8080";
```

### 4. Listen for new version deployments

The injected script automatically polls `/_version` in the background (every 30 seconds by default). When a new rollout completes, it fires a `webapp:update` event on both `window` and `document` so your application can prompt the user to refresh or reload dynamically:

```javascript
window.addEventListener('webapp:update', (event) => {
  const { currentVersion, newVersion } = event.detail;
  console.log(`Update ready: ${newVersion} (currently running ${currentVersion})`);

  // Prompt the user or reload the page
  showBanner({
    message: "A new version of the app is available!",
    actionText: "Reload",
    onClick: () => window.location.reload()
  });
});
```

You can also read `window.__APP_VERSION__` directly at any time.

---

## Request throughput

I ran realistic benchmarks on an Apple M1 (8 cores) to see how each pod handles heavy traffic. Because static files are streamed straight from disk cache using the OS page cache and config is generated in-memory, your network or ingress controller will choke long before the pod does:

| Scenario | What's happening | Request throughput | RAM usage |
| :--- | :--- | :--- | :--- |
| **Discreet config endpoint** | Serving dynamic `/_config.js` with your environment variables | **~1,400,000 req / sec** | ~18 MB |
| **Inline environment script** | Serving `index.html` with your config injected into the `<head>` | **~48,500 req / sec** | ~18 MB |
| **JS bundle** | Streaming 100 KB static chunks straight from local disk | **~40,000 req / sec** (~4 GB/s) | ~18 MB |
| **Real world app load** | Mixed continuous traffic across pages, bundles, and config endpoints | **~45,000 req / sec** | ~18 MB |

### Efficient resource usage

During a sustained test of **50,000 requests** across mixed routes:
* **Active Heap In-Use:** **1.55 MB**
* **Total Process Memory (`Sys`):** **18.27 MB**
* **Idle CPU:** `< 0.1%` (virtually zero at rest)

Because it barely sips resources, the default Kubernetes deployment footprint is tiny:

```yaml
resources:
  requests:
    cpu: 10m        # 0.01 core
    memory: 32Mi    # Plenty of headroom over the 18MB base footprint
  limits:
    cpu: 500m       # Burst capacity for high-volume spikes
    memory: 128Mi   # Headroom for pulling and unpacking OCI layers
```

---

## Documentation

* [How it works under the hood](docs/architecture.md): Root and revision CRD model, peer sync, and idempotent fallback fetches.
* [Argo CD Health Checks](docs/argocd.md): Custom Lua scripts to show real-time rollout health in the Argo CD UI.

---

## License

Apache 2.0
