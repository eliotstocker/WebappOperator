# How it works under the hood

This doc explains how `webapp-operator` is built, why I made certain design choices, and how the pieces fit together.

The core idea is simple: instead of running a separate Nginx pod for every single website you own, `webapp-operator` runs as a shared fleet of gateway pods. They pull static files straight from your container registry into a local disk cache and serve them with on-the-fly config injection.

```
GitOps / Argo CD
       │
       ▼
 Website (CRD) ──► creates ──► WebsiteRevision (CRD)
 (desired state)                (immutable build record)
                                       │
         ┌─────────────────────────────┴─────────────────────────────┐
         ▼                                                           ▼
Gateway Pod 1 (Leader)                                      Gateway Pod 2 (Peer)
• Watches CRDs & EndpointSlices                             • Pulls OCI layer to local disk
• Pulls OCI layer to local disk                             • Shoots HTTP ping to Leader:
• Tracks who is ready           ◄── POST /internal/sync ───── "Hey, I've got this revision"
• Flips switch to Active                                    • Serves HTTP traffic
• Serves HTTP traffic                                       • Injects runtime config
         │                                                           │
         └─────────────────────────────┬─────────────────────────────┘
                                       │
                                       ▼
                            ClusterIP / Ingress Rule
                            (Host: app.example.com)
```

---

## 1. Root CRD with Revision CRD for understanding your full deployment pipeline

I intentionally designed this with **two CRDs, not three**:

### `Website` (What you write)
This is what you commit to GitOps. It's the desired state of your app:
* `spec.image`: The OCI image containing your static build.
* `spec.hostnames`: Which domains point here.
* `spec.env`: The runtime environment variables you want injected into the frontend.
* `spec.injection`: How you want config injected (`endpoint`, `inline`, or `both`).
* `spec.ingress`: Optional helper to automatically create an Ingress for you if you don't want to write one manually.

### `WebsiteRevision` (What the operator creates)
You don't edit this. The controller creates an immutable `WebsiteRevision` whenever `spec.image` or `spec.env` changes. 
* It records an immutable release candidate of the build + configuration.
* It tracks which pods have warmed their local cache.
* When all pods are ready, it flips to `Active`.

> **Why no 3rd CRD for pod cache states?**  
> Storing individual pod cache records in etcd is a classic Kubernetes trap. Under HPA scaling or node churn, it causes etcd bloat and reconciliation deadlocks. Instead, pods coordinate their readiness directly over internal HTTP.

---

## 2. Distributed rollouts without the headaches

When you roll out a new version of a frontend, you don't want traffic hitting a pod that hasn't unpacked the new files yet. Here's how I coordinate rollouts across the fleet:

1. **All pods run the exact same binary:** Every pod runs both the controller and the HTTP server. A standard Kubernetes `Lease` elects one pod as the cluster Leader.
2. **AOT caching:** When a new `WebsiteRevision` is created, every gateway pod pulls the OCI layer and unpacks it into its local `emptyDir` cache ahead of time.
3. **Peer-to-Leader sync:** Once a peer finishes unpacking, it sends an internal HTTP POST to the leader over the unified `:8080` port:
   ```http
   POST /internal/sync
   {"podName": "webapp-operator-xyz", "namespace": "default", "revision": "store-abc1234"}
   ```
4. **EndpointSlices as a source of truth:** The Leader watches Kubernetes `EndpointSlices` to know which pods are actually alive and taking traffic. Once all active pods have reported ready, the Leader flips the revision to `Active`.
5. **Simple safety fallback (fetches content idempotently):** What if HPA spins up 5 new pods *after* a rollout? Cold pods pull missing revisions on-demand on the first request. The fetch runs idempotently and deduplicates requests so concurrent traffic doesn't dogpile your container registry.

---

## 3. Runtime config injection & live update notifications

Building different JavaScript bundles for staging vs production is an antipattern. A build should be an immutable release candidate that can run anywhere.

I built in two ways to inject config without touching your built JS:

1. **Discreet config endpoint (Default & CSP-friendly):**
   * Serves dynamic JavaScript at `/_config.js` with `Cache-Control: no-cache`.
   * Sets `window.__ENV__ = Object.freeze({ API_URL: "https://api.prod.example.com" })`.
   * In your `index.html`, just add `<script src="/_config.js"></script>`.
2. **Inline script injection:**
   * Injects `<script id="__ENV__">window.__ENV__ = ...</script>` directly before `</head>` in your `index.html` at request time.
   * Useful if you want zero extra network requests on first paint.

### Detecting new deployments in the frontend

Long-running browser tabs often run stale JavaScript bundles. To let SPAs gracefully notify users or reload when a rollout completes, the injected script automatically:
* Exposes `window.__APP_VERSION__` for the currently loaded revision.
* Polls the version endpoint in the background (`/_version` by default, configurable via `spec.injection.versionPath`).
* Dispatches a `webapp:update` event on both `window` and `document` whenever a new version is detected:
  ```javascript
  window.addEventListener('webapp:update', (event) => {
    console.log('Update ready:', event.detail.currentVersion, '->', event.detail.newVersion);
  });
  ```
* Enabled by default (polls every 30 seconds, configurable via `spec.injection.pollIntervalSeconds`). To disable background polling entirely, set `spec.injection.versionPolling: false` (or `pollIntervalSeconds: 0`).

---

## 4. Traffic & routing

* **It's an upstream service, not an Ingress controller:** `webapp-operator` runs as a plain `ClusterIP` Service. Put your existing Ingress (Nginx, Traefik, AWS ALB) or Gateway API in front of it.
* **Host-based routing:** The server checks the incoming `Host` header to match which `Website` to serve.
* **SPA fallback:** If a request doesn't match an actual file on disk, it serves `index.html` so client-side routers (React Router, Vue Router, etc.) work out of the box.
* **Keeping old versions warm as we can't be sure everyone is running off the latest version:** When a new revision goes live, previous build chunks remain cached on disk for a retention window. Users who have your site open in a tab won't get white-screen 404s when navigating.
