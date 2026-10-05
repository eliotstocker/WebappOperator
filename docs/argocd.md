# Argo CD Health Checks Integration

Argo CD uses custom Lua scripts to determine the health status of Custom Resource Definitions (CRDs). Without a health script, Argo CD defaults to marking custom resources as `Healthy` immediately upon creation, which means it will not report when a rollout is `Progressing` (downloading/pre-warming assets) or `Degraded`.

By adding the Lua scripts below to your Argo CD configuration, Argo CD will accurately display live health icons (green heart for Healthy, blue circle for Progressing, red exclamation for Degraded) during website rollouts.

---

## 1. Lua Health Check Scripts

### `webapp.io/Website`

```lua
hs = {}
if obj.status ~= nil then
  if obj.status.phase == "Ready" then
    hs.status = "Healthy"
    hs.message = "Serving revision: " .. (obj.status.activeRevision or "none")
    return hs
  elseif obj.status.phase == "Degraded" then
    hs.status = "Degraded"
    hs.message = "Website is degraded"
    return hs
  elseif obj.status.phase == "Prewarming" or obj.status.phase == "Pending" then
    hs.status = "Progressing"
    hs.message = "Prewarming revision assets across gateway pods"
    return hs
  end
end
hs.status = "Progressing"
hs.message = "Waiting for initial reconciliation"
return hs
```

### `webapp.io/WebsiteRevision`

```lua
hs = {}
if obj.status ~= nil then
  if obj.status.phase == "Active" or obj.status.phase == "Ready" or obj.status.phase == "Retired" then
    hs.status = "Healthy"
    hs.message = string.format("Phase: %s, Synced Pods: %d", obj.status.phase, #(obj.status.readyPods or {}))
    return hs
  elseif obj.status.phase == "Failed" then
    hs.status = "Degraded"
    hs.message = "Failed to pull or unpack OCI artifact"
    return hs
  elseif obj.status.phase == "Prewarming" or obj.status.phase == "Pending" then
    hs.status = "Progressing"
    hs.message = "Pulling OCI artifact into local pod caches"
    return hs
  end
end
hs.status = "Progressing"
hs.message = "Waiting for revision controller"
return hs
```

---

## 2. Configuration Methods

### Option A: Via Argo CD Helm Chart (`values.yaml`)

If you install Argo CD via the official Helm chart (`argo/argo-cd`), add the following to your `values.yaml`:

```yaml
configs:
  cm:
    resource.customizations: |
      webapp.io/Website:
        health.lua: |
          hs = {}
          if obj.status ~= nil then
            if obj.status.phase == "Ready" then
              hs.status = "Healthy"
              hs.message = "Serving revision: " .. (obj.status.activeRevision or "none")
              return hs
            elseif obj.status.phase == "Degraded" then
              hs.status = "Degraded"
              hs.message = "Website is degraded"
              return hs
            elseif obj.status.phase == "Prewarming" or obj.status.phase == "Pending" then
              hs.status = "Progressing"
              hs.message = "Prewarming revision assets across gateway pods"
              return hs
            end
          end
          hs.status = "Progressing"
          hs.message = "Waiting for initial reconciliation"
          return hs

      webapp.io/WebsiteRevision:
        health.lua: |
          hs = {}
          if obj.status ~= nil then
            if obj.status.phase == "Active" or obj.status.phase == "Ready" or obj.status.phase == "Retired" then
              hs.status = "Healthy"
              hs.message = string.format("Phase: %s, Synced Pods: %d", obj.status.phase, #(obj.status.readyPods or {}))
              return hs
            elseif obj.status.phase == "Failed" then
              hs.status = "Degraded"
              hs.message = "Failed to pull or unpack OCI artifact"
              return hs
            elseif obj.status.phase == "Prewarming" or obj.status.phase == "Pending" then
              hs.status = "Progressing"
              hs.message = "Pulling OCI artifact into local pod caches"
              return hs
            end
          end
          hs.status = "Progressing"
          hs.message = "Waiting for revision controller"
          return hs
```

### Option B: Patching `argocd-cm` ConfigMap Directly

If managing manifests directly, edit the `argocd-cm` ConfigMap in your Argo CD namespace (usually `argocd`):

```bash
kubectl edit configmap argocd-cm -n argocd
```

Add the `resource.customizations` block shown above under `data`. Argo CD will automatically detect the changes without needing a restart.
