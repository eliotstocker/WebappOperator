# AGENTS.md — Development Guidelines for AI Agents

This document defines core architectural invariants, code organization rules, and operational boundaries for AI agents working in this repository.

---

## 1. Core Architectural Invariants

* **Unified Binary:** The project builds a single Go binary that functions as both the Kubernetes controller and the HTTP static file server. Do not split into separate data-plane and control-plane binaries.
* **Upstream Service (Not an Ingress):** The gateway runs as a standard `ClusterIP` Service behind external ingress controllers (Ingress-Nginx, Traefik, Gateway API). The Go server resolves sites via the HTTP `Host` header.
* **Two-Tier CRD Model Only:**
  * `Website`: User-defined desired target state (managed via GitOps).
  * `WebsiteRevision`: Controller-generated immutable record of a specific build + config.
  * **DO NOT** create a 3rd CRD for individual pod cache states (avoid etcd bloat and HPA deadlocks).
* **Distributed Sync via Internal HTTP:** Replicas notify the elected Leader of cache readiness via internal HTTP (`POST /internal/sync`). Only the elected Leader writes status updates to the Kubernetes API to avoid `409 Conflict` contention.
* **EndpointSlice Dynamic Truth:** Rollouts consider all pods synced when `EndpointSlice.ActivePods ⊆ WebsiteRevision.status.readyPods`. This prevents deadlocks when HPA scales down pods during a release.
* **Single-Flight on Miss:** Always preserve a single-flight cache-miss fetcher. If HPA scales up cold pods post-rollout, they must pull on-demand without failing requests.
* **Immutability:** `WebsiteRevision.spec` is immutable once created. Any change to `image` or `env` triggers a new `WebsiteRevision`.

---

## 2. Directory Structure Conventions

Agents should organize code strictly by domain:

```
├── api/v1alpha1/          # Kubebuilder CRD structs and deepcopy code
├── cmd/operator/          # main.go entrypoint (flags, leader election, signal handling)
├── internal/
│   ├── controller/        # Reconciliation loops (Website, WebsiteRevision, Ingress)
│   ├── server/            # HTTP static file server, host routing, SPA fallback
│   ├── injection/         # Dynamic env injection (/_config.js and inline script)
│   ├── oci/               # OCI registry puller (ORAS / go-containerregistry) & tar unpacker
│   ├── cache/             # Local disk (emptyDir) manager, LRU retention, single-flight
│   └── syncer/            # Leader-peer sync HTTP server & client
└── docs/                  # Architecture specs and reference documentation
```

---

## 3. Engineering & Code Standards

* **Simplicity > Theory:** Choose the simplest, most maintainable implementation. Resist unnecessary abstractions, premature interfaces, or microservice splitting.
* **Minimal Dependencies:** Prioritize Go standard library where possible (`net/http`, `archive/tar`, `compress/gzip`). Use standard packages for Kubernetes (`sigs.k8s.io/controller-runtime`) and OCI (`oras.land/oras-go` or `github.com/google/go-containerregistry`).
* **Clean Error Handling:** Log actionable details on critical errors. Do not let transient background errors panic the server or crash the pod.
* **DocBlocks:** Every exported function and non-obvious code block must include comments explaining the *why*, not just the *what*.
* **No Premature Refactoring:** Confine changes strictly to the requested feature or fix. Do not reformat unrelated files.
