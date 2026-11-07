#!/usr/bin/env bash
set -euo pipefail

# JakLingko CLI & MCP Server Cross-Platform Installer
# Supports Linux and macOS (amd64, arm64)

REPO="bardiz12/jaklingko-client"
CLI_BIN="jaklingko-cli"
MCP_BIN="jaklingko-mcp"

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_ok()   { echo -e "${GREEN}[OK]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_err()  { echo -e "${RED}[ERROR]${NC} $*" >&2; }

echo "=================================================="
echo "    JakLingko CLI & MCP Server Installer          "
echo "=================================================="

# 1. Detect Operating System
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "${OS}" in
  linux*)  OS="linux" ;;
  darwin*) OS="darwin" ;;
  *)
    log_err "Unsupported operating system: ${OS}. For Windows, please run install.ps1."
    exit 1
    ;;
esac

# 2. Detect CPU Architecture
ARCH="$(uname -m)"
case "${ARCH}" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    log_err "Unsupported architecture: ${ARCH}"
    exit 1
    ;;
esac

log_info "Detected Platform: ${OS}/${ARCH}"

# 3. Resolve Target Version / Tag
TAG="${1:-}"
if [ -z "${TAG}" ]; then
  log_info "Fetching latest release tag from GitHub..."
  LATEST_JSON=$(curl -sSL -H "Accept: application/vnd.github+json" "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null || true)
  TAG=$(echo "${LATEST_JSON}" | grep -o '"tag_name": *"[^"]*"' | head -n 1 | cut -d '"' -f 4 || true)
fi

if [ -z "${TAG}" ]; then
  TAG="v0.1.0"
  log_warn "Could not resolve latest release dynamically. Falling back to default: ${TAG}"
else
  log_info "Target release: ${TAG}"
fi

# Normalize version without 'v' prefix for archive naming convention
VERSION="${TAG#v}"

# 4. Construct Download URL
ARCHIVE_NAME="jaklingko-client_${VERSION}_${OS}_${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/${ARCHIVE_NAME}"

TMP_DIR="$(mktemp -d)"
cleanup() { rm -rf "${TMP_DIR}"; }
trap cleanup EXIT

log_info "Downloading ${ARCHIVE_NAME}..."
download_success=0

# Method 1: Direct download from public release
if curl -fsSL -o "${TMP_DIR}/${ARCHIVE_NAME}" "${DOWNLOAD_URL}" 2>/dev/null; then
  download_success=1
fi

# Method 2: GitHub CLI (gh) if authenticated
if [ "${download_success}" -eq 0 ] && command -v gh >/dev/null 2>&1; then
  if gh release download "${TAG}" -p "${ARCHIVE_NAME}" -R "${REPO}" --dir "${TMP_DIR}" 2>/dev/null; then
    download_success=1
  fi
fi

# Method 3: Authenticated GitHub API if GITHUB_TOKEN or GH_TOKEN is available
if [ "${download_success}" -eq 0 ]; then
  AUTH_TOKEN="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
  if [ -n "${AUTH_TOKEN}" ]; then
    ASSET_ID=$(curl -sSL -H "Authorization: Bearer ${AUTH_TOKEN}" -H "Accept: application/vnd.github+json" \
      "https://api.github.com/repos/${REPO}/releases/tags/${TAG}" 2>/dev/null | \
      grep -B 2 "\"name\": *\"${ARCHIVE_NAME}\"" | grep -o '"id": *[0-9]*' | head -n 1 | awk '{print $2}' || true)
    if [ -n "${ASSET_ID}" ]; then
      if curl -fsSL -H "Authorization: Bearer ${AUTH_TOKEN}" -H "Accept: application/octet-stream" \
        -o "${TMP_DIR}/${ARCHIVE_NAME}" "https://api.github.com/repos/${REPO}/releases/assets/${ASSET_ID}" 2>/dev/null; then
        download_success=1
      fi
    fi
  fi
fi

if [ "${download_success}" -eq 0 ]; then
  log_err "Failed to download from ${DOWNLOAD_URL}"
  log_err "Please verify your network connection, release availability at https://github.com/${REPO}/releases,"
  log_err "or set GITHUB_TOKEN if the repository is private."
  exit 1
fi

log_info "Extracting binaries..."
tar -xzf "${TMP_DIR}/${ARCHIVE_NAME}" -C "${TMP_DIR}"

# 5. Determine Installation Directory
INSTALL_DIR="/usr/local/bin"
USE_SUDO=0

if [ -w "${INSTALL_DIR}" ]; then
  USE_SUDO=0
elif command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
  USE_SUDO=1
elif [ -w "${HOME}/.local/bin" ] || mkdir -p "${HOME}/.local/bin" 2>/dev/null; then
  INSTALL_DIR="${HOME}/.local/bin"
  USE_SUDO=0
else
  USE_SUDO=1
fi

log_info "Installing binaries into ${INSTALL_DIR}..."

if [ "${USE_SUDO}" -eq 1 ]; then
  sudo mv "${TMP_DIR}/${CLI_BIN}" "${INSTALL_DIR}/${CLI_BIN}"
  sudo mv "${TMP_DIR}/${MCP_BIN}" "${INSTALL_DIR}/${MCP_BIN}"
  sudo chmod +x "${INSTALL_DIR}/${CLI_BIN}" "${INSTALL_DIR}/${MCP_BIN}"
else
  mv "${TMP_DIR}/${CLI_BIN}" "${INSTALL_DIR}/${CLI_BIN}"
  mv "${TMP_DIR}/${MCP_BIN}" "${INSTALL_DIR}/${MCP_BIN}"
  chmod +x "${INSTALL_DIR}/${CLI_BIN}" "${INSTALL_DIR}/${MCP_BIN}"
fi

log_ok "Successfully installed:"
log_ok "  - CLI Tool   : ${INSTALL_DIR}/${CLI_BIN}"
log_ok "  - MCP Server : ${INSTALL_DIR}/${MCP_BIN}"

# Check if INSTALL_DIR is in PATH
case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *)
    log_warn "Notice: ${INSTALL_DIR} is not in your current PATH."
    log_warn "Add it by running: export PATH=\"${INSTALL_DIR}:\$PATH\""
    ;;
esac

# 6. Verify Installation
if command -v "${CLI_BIN}" >/dev/null 2>&1; then
  log_ok "$(${CLI_BIN} -version)"
elif [ -f "${INSTALL_DIR}/${CLI_BIN}" ]; then
  log_ok "$("${INSTALL_DIR}/${CLI_BIN}" -version)"
fi

echo ""
echo "Quick Test:"
echo "  ${CLI_BIN} -origin \"Dukuh Atas\" -destination \"Monas\""
echo "  ${MCP_BIN} -version"
echo ""
