#!/usr/bin/env bash
set -euo pipefail

REPO="firestar/voxel-world"
VERSION_URL="https://raw.githubusercontent.com/${REPO}/main/version.txt"
RELEASE_BASE="https://github.com/${REPO}/releases/download"

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required but was not found" >&2
  exit 1
fi

VERSION=$(curl -fsSL "${VERSION_URL}" | tr -d '\r\n')
if [[ -z "${VERSION}" ]]; then
  echo "Failed to determine release version from ${VERSION_URL}" >&2
  exit 1
fi

echo "Using release version: ${VERSION}"

WORKDIR=${VOXEL_WORLD_HOME:-"${PWD}/voxel-world-${VERSION}"}
mkdir -p "${WORKDIR}" "${WORKDIR}/configs"

CENTRAL_BIN="${WORKDIR}/central"
CHUNK_BIN="${WORKDIR}/chunkserver"
CONFIG_DIR="${WORKDIR}/configs"

fetch() {
  local url="$1"
  local dest="$2"
  echo "Downloading ${url} -> ${dest}"
  curl -fsSL "${url}" -o "${dest}"
}

fetch "${RELEASE_BASE}/${VERSION}/central-linux-amd64" "${CENTRAL_BIN}"
chmod +x "${CENTRAL_BIN}"

fetch "${RELEASE_BASE}/${VERSION}/chunk-server-linux-amd64" "${CHUNK_BIN}"
chmod +x "${CHUNK_BIN}"

fetch "https://raw.githubusercontent.com/${REPO}/main/central/central.yaml" "${CONFIG_DIR}/central.yaml"
for cfg in chunk-east.json chunk-west.json; do
  fetch "https://raw.githubusercontent.com/${REPO}/main/chunk-server/configs/${cfg}" "${CONFIG_DIR}/${cfg}"
done

tmp_config="${CONFIG_DIR}/central.yaml.tmp"
sed \
  -e "s|/usr/local/bin/chunkserver|${CHUNK_BIN}|g" \
  -e "s|/etc/central/configs/|${CONFIG_DIR}/|g" \
  "${CONFIG_DIR}/central.yaml" > "${tmp_config}"
mv "${tmp_config}" "${CONFIG_DIR}/central.yaml"

echo "Starting central server..."
exec "${CENTRAL_BIN}" --config "${CONFIG_DIR}/central.yaml"
