# Architecture & Technical Specification

## 1. System Overview

`webapp-operator` is a Kubernetes-native platform for hosting and serving Single Page Applications (SPAs). It replaces:
- **Nginx-in-Docker containers:** Wastes RAM, creates slow CI/CD image build steps, and complicates runtime configuration.
- **External Object Storage (S3 / Cloudflare Pages):** Disconnects frontend releases from Kubernetes GitOps (ArgoCD/Flux).

Instead, static build artifacts are packaged into standard OCI registry layers, deployed via Kubernetes Custom Resources, cached locally in a shared gateway fleet, and served with runtime configuration injection.

```
+-------------------------------------------------------------------------+
|                              GitOps / ArgoCD                            |
|                                     │                                   |
|                                     ▼                                   |
|                              Website (CRD)                              |
|                         (spec.image, env, host)                         |
+-------------------------------------+-----------------------------------+
                                      │ reconciles
                                      ▼
                        WebsiteRevision (CRD) [Child]
                    (immutable build & rollout tracker)
                                      │
            ┌─────────────────────────┴─────────────────────────┐
            ▼                                                   ▼
┌───────────────────────┐                           ┌───────────────────────┐
│ Gateway Pod 1 (Leader)│                           │ Gateway Pod 2 (Peer)  │
│ ───────────────────── │                           │ ───────────────────── │
│ • Watches CRDs        │                           │ • Watches CRDs        │
│ • Watches EndpointSlice                           │ • Pulls OCI layer     │
│ • Pulls OCI layer     │◄──── POST /internal/sync ─┤ • Reports to Leader   │
│ • Aggregates readyPods│      (cache complete)     │ • Serves HTTP traffic │
│ • Marks Revision Ready│                           │                       │
│ • Serves HTTP traffic │                           │                       │
└───────────┬───────────┘                           └───────────┬───────────┘
            │                                                   │
            └─────────────────────────┬─────────────────────────┘
                                      │
                                      ▼
                           ClusterIP / Ingress Rule
                           (Host: app.example.com)
```

---

## 2. Custom Resource Model

### A. `Website` (Parent / Desired State)
Declarative, user-managed state. Managed directly by GitOps.

- **`spec.image`**: OCI artifact reference (`ghcr.io/org/frontend:v1.2.0`).
- **`spec.hostnames`**: List of FQDNs routed to this website.
- **`spec.env`**: Key-value map injected into the client application at runtime.
- **`spec.injection`**: Injection mode (`endpoint` | `inline` | `both`) and endpoint path (e.g. `/_config.js`).
- **`spec.imagePullSecrets`**: Secrets for authenticating against private OCI registries.
- **`spec.revisionHistoryLimit`**: Number of old `WebsiteRevision` objects to retain (default: 3).
- **`spec.ingress`**: Optional automated reconciliation of `networking.k8s.io/v1` `Ingress`.
- **`status.activeRevision`**: Name of the currently serving `WebsiteRevision`.
- **`status.phase`**: `Pending` | `Prewarming` | `Ready` | `Degraded`.

### B. `WebsiteRevision` (Child / Immutable Release)
Controller-generated child resource with `ownerReferences` pointing to `Website`.

- **`spec`**: Immutable snapshot of `image`, `digest`, `env`, and `injection` settings.
- **`status.phase`**: `Pending` -> `Prewarming` -> `Ready` -> `Active` -> `Retired`.
- **`status.readyPods`**: List of gateway pod names that have successfully fetched and unpacked this revision.

---

## 3. Distributed Cache & Rollout Coordination

### The Shared Gateway Model
All pods run the same unified Go binary acting as both a Kubernetes controller and HTTP file server. A Kubernetes `Lease` elects a single Leader among the gateway pods.

### Avoiding Rollout Deadlocks & 409 Contention
1. **OCI Layer Download:** When a new `WebsiteRevision` appears in `Prewarming`, all gateway pods concurrently download and unpack the OCI layer into local `emptyDir` disk storage.
2. **Internal Sync Ping:** Once a replica pod finishes unpacking, it sends an internal HTTP ping to the elected Leader:
   ```http
   POST http://<leader-ip>:8080/internal/sync
   Content-Type: application/json

   { "podName": "webapp-gateway-xyz", "revision": "storefront-v1-2-0" }
   ```
3. **EndpointSlice Tracking:** The Leader watches the gateway's `EndpointSlice` to know the exact set of active, healthy pods receiving traffic.
4. **Promotion to Active:** When all live pods in the `EndpointSlice` appear in `readyPods`, the Leader marks `WebsiteRevision.status.phase = Active` and updates `Website.status.activeRevision`.
5. **Safety Valve (Fetch-on-Miss):** If a pod starts up during/after a rollout (e.g., HPA scale-out), it uses a **single-flight lock** to fetch and unpack the revision on first request without returning 404s.

---

## 4. Storage & Asset Lifecycle

- **Local `emptyDir` Cache:** Pods unpack OCI tarballs into subdirectories: `/cache/<namespace>/<website>/<revision>/`.
- **Solving "Chunk 404s":** The gateway maintains an in-memory routing table for active and recently retired revisions. Chunk files (`/assets/*.js`, `/assets/*.css`) include immutable cache headers (`Cache-Control: public, max-age=31536000, immutable`). Old chunks remain accessible across the retention window.
- **Pruning:** When a `WebsiteRevision` is deleted or garbage-collected, pods remove the corresponding directory from disk.

---

## 5. Runtime Configuration Injection

Solves the need to rebuild JavaScript bundles for different environments.

### Modes
1. **Endpoint Mode (Default & CSP-Safe):**
   - Serves dynamic JavaScript at a configurable path (e.g. `/_config.js` or `/__env.js`).
   - Content: `window.__ENV__ = { "API_URL": "https://api.prod.example.com" };`
   - Headers: `Cache-Control: no-cache, no-store, must-revalidate`.
   - Client includes: `<script src="/_config.js"></script>` in `index.html`.
2. **Inline Mode:**
   - Injects `<script id="__ENV__">window.__ENV__ = ...</script>` directly before `</head>` in `index.html`.
3. **Both:** Serves the endpoint and injects the inline script.

---

## 6. Traffic & Ingress Integration

`webapp-operator` is an upstream HTTP service (ClusterIP), not an edge ingress controller.

1. **Host-Based Routing:** The server examines the incoming HTTP `Host` header to resolve the target `Website` and its `activeRevision`.
2. **SPA Fallback:** Any request that does not match an existing static file on disk falls back to serving `index.html`.
3. **Optional Ingress Reconciliation:** When `spec.ingress.enabled: true`, the operator creates and maintains a `networking.k8s.io/v1` `Ingress` pointing to its own Service. If disabled, users configure Ingress, Gateway API `HTTPRoute`, or wildcard DNS manually.
