#!/usr/bin/env bash
set -euo pipefail

# publish-image.sh: Builds and pushes multi-arch container images with lowercased repository names.
# Usage: ./scripts/publish-image.sh <version>

VERSION="${1:-}"

if [ -z "$VERSION" ]; then
  echo "Error: Version argument missing" >&2
  exit 1
fi

REPO="${GITHUB_REPOSITORY:-eliotstocker/webapp-operator}"
REPO_LOWER=$(echo "$REPO" | tr '[:upper:]' '[:lower:]')
IMAGE_NAME="ghcr.io/${REPO_LOWER}"

echo "Building and pushing multi-arch container image: ${IMAGE_NAME}:${VERSION} and ${IMAGE_NAME}:latest"

docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t "${IMAGE_NAME}:${VERSION}" \
  -t "${IMAGE_NAME}:latest" \
  --push .
