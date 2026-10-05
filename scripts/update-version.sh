#!/usr/bin/env bash
set -euo pipefail

# update-version.sh: Updates application and manifest versions across the repository.
# Usage: ./scripts/update-version.sh <version> (e.g. 1.2.3)

NEW_VERSION="${1:-}"

if [ -z "$NEW_VERSION" ]; then
  echo "Error: Version argument missing" >&2
  exit 1
fi

# Strip optional leading 'v'
NEW_VERSION="${NEW_VERSION#v}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# 1. Update Go version package (internal/version/version.go)
VERSION_GO="${REPO_ROOT}/internal/version/version.go"
if [ -f "$VERSION_GO" ]; then
  sed -i.bak "s/var Version = .*/var Version = \"${NEW_VERSION}\"/" "$VERSION_GO"
  rm -f "${VERSION_GO}.bak"
  echo "Updated ${VERSION_GO} to ${NEW_VERSION}"
fi

# 2. Update Helm Chart manifest (charts/webapp-operator/Chart.yaml)
CHART_YAML="${REPO_ROOT}/charts/webapp-operator/Chart.yaml"
if [ -f "$CHART_YAML" ]; then
  sed -i.bak "s/^version: .*/version: ${NEW_VERSION}/" "$CHART_YAML"
  sed -i.bak "s/^appVersion: .*/appVersion: \"${NEW_VERSION}\"/" "$CHART_YAML"
  rm -f "${CHART_YAML}.bak"
  echo "Updated ${CHART_YAML} to ${NEW_VERSION}"
fi

echo "Successfully updated version to ${NEW_VERSION}"
