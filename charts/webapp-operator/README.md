# webapp-operator Helm Chart

Installs the `webapp-operator` controller and shared HTTP static serving gateway fleet on Kubernetes.

## Prerequisites

* Kubernetes 1.26+
* Helm 3.8+

## Installing the Chart

```bash
# Install directly from local chart directory
helm install webapp-operator ./charts/webapp-operator -n webapp-system --create-namespace
```

## Key Configuration Parameters

| Parameter | Description | Default |
| :--- | :--- | :--- |
| `replicaCount` | Number of gateway operator pods | `2` |
| `image.repository` | Operator container image | `ghcr.io/eliotstocker/webapp-operator` |
| `image.tag` | Image tag (defaults to `Chart.AppVersion`) | `0.1.0` |
| `service.port` | HTTP traffic port exposed by the gateway Service | `80` |
| `cache.dir` | Path on pod filesystem where OCI layers are unpacked | `/var/cache/webapp-operator` |
| `cache.emptyDir.sizeLimit` | Disk storage limit for local emptyDir volume | `10Gi` |
| `config.leaderElect` | Enable leader election across gateway pods | `true` |
| `resources` | CPU/Memory requests and limits | `requests: 50m / 64Mi` |

## Uninstalling the Chart

```bash
helm uninstall webapp-operator -n webapp-system
```
