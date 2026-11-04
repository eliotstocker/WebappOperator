# Headlamp Webapp Plugin

A [Headlamp](https://headlamp.dev) plugin providing visual management, rollout inspection, and cache sync monitoring for `webapp-operator` custom resources (`Website` and `WebsiteRevision`).

## Features

* **Sidebar Navigation:** Dedicated `Webapps` section in Headlamp navigation.
* **Websites Overview:**
  * Status chips (`Ready`, `Prewarming`, `Degraded`, `Pending`).
  * Direct clickable links for configured hostnames (`http(s)://...`).
  * Active OCI image and active revision indicators.
  * Age and namespace filtering.
* **Website Details:**
  * Spec overview (images, injection parameters, ingress settings, history limits).
  * Child `WebsiteRevision` table showing revision status, unpacked asset size, and ready pods.
  * Injected environment variable key-value table.
  * Ingress TLS and class configurations.
* **Revision Details:**
  * Gateway pod cache sync breakdown (list of pods that have fetched and unpacked the revision).
  * Resolved OCI digest snapshot.
  * Unpacked asset byte size on disk.
  * Static environment variables snapshot for the immutable revision.

## Development

```bash
cd plugins/headlamp
npm install
npm run build
```

To run type checking or linter:

```bash
npm run tsc
npm run lint
```

## Installation

### Local Headlamp Desktop

Build the plugin and link/copy the `plugins/headlamp` folder to Headlamp's plugin directory:

* **macOS:** `~/Library/Application Support/Headlamp/plugins/headlamp-webapp` (or `~/.config/Headlamp/plugins/headlamp-webapp`)
* **Linux:** `~/.config/Headlamp/plugins/headlamp-webapp`
* **Windows:** `%APPDATA%\Headlamp\plugins\headlamp-webapp`

```bash
mkdir -p ~/.config/Headlamp/plugins
ln -s "$(pwd)/plugins/headlamp" ~/.config/Headlamp/plugins/headlamp-webapp
```

Restart Headlamp, and the "Webapps" item will appear in the navigation bar.

### In-Cluster Headlamp

Package the plugin using `npm run package` and load it into Headlamp via an init container or ConfigMap mounting `/headlamp/plugins`.
