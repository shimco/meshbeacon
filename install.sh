#!/bin/sh
# ==============================================================================
# Meshbeacon Turnkey Installer
# ==============================================================================
# Description:
#   Auto-detects CPU architecture and operating system, downloads the latest
#   Meshbeacon release binary, and configures OpenWrt procd or Linux systemd.
#
# Arguments / Flags:
#   $1 - Optional version tag (e.g. v0.7.0). Defaults to latest release.
#
# Exit Codes:
#   0 - Success
#   1 - Failure / Unsupported platform
# ==============================================================================
set -eu

REPO="shimco/meshbeacon"
INSTALL_DIR_LINUX="/usr/local/bin"
INSTALL_DIR_OPENWRT="/usr/bin"
CONFIG_DIR="/etc/meshbeacon"

log() {
  printf "\033[1;34m[Meshbeacon Installer]\033[0m %s\n" "$*"
}

error() {
  printf "\033[1;31m[Error]\033[0m %s\n" "$*" >&2
  exit 1
}

# 1. Detect Operating System and Architecture
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "${ARCH}" in
  x86_64|amd64)
    TARGET_ARCH="amd64"
    ;;
  aarch64|arm64)
    TARGET_ARCH="arm64"
    ;;
  armv7*|armv6*|arm)
    TARGET_ARCH="armv7"
    ;;
  mips*le|mipsel)
    TARGET_ARCH="mipsle"
    ;;
  *)
    error "Unsupported architecture: ${ARCH}"
    ;;
esac

case "${OS}" in
  linux)
    TARGET_OS="linux"
    ;;
  darwin)
    TARGET_OS="darwin"
    ;;
  *)
    error "Unsupported operating system: ${OS}"
    ;;
esac

log "Detected platform: ${TARGET_OS}/${TARGET_ARCH}"

# 2. Determine Version to Install
VERSION="${1:-}"
if [ -z "${VERSION}" ]; then
  log "Fetching latest release version from GitHub..."
  LATEST_JSON="$(curl -sSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null || true)"
  VERSION="$(echo "${LATEST_JSON}" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' || true)"
  if [ -z "${VERSION}" ]; then
    VERSION="v0.7.0"
  fi
fi

log "Target version: ${VERSION}"

# 3. Download Release Binary
TARBALL="meshbeacon_${TARGET_OS}_${TARGET_ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL}"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

log "Downloading ${DOWNLOAD_URL}..."
if ! curl -sSL --fail "${DOWNLOAD_URL}" -o "${TMP_DIR}/${TARBALL}"; then
  # Fallback to direct raw binary if available
  RAW_URL="https://github.com/${REPO}/releases/download/${VERSION}/meshbeacon-${TARGET_OS}-${TARGET_ARCH}"
  log "Archive download failed, attempting direct binary: ${RAW_URL}..."
  if ! curl -sSL --fail "${RAW_URL}" -o "${TMP_DIR}/meshbeacon"; then
    error "Failed to download Meshbeacon binary for ${TARGET_OS}/${TARGET_ARCH} (${VERSION})."
  fi
else
  tar -xzf "${TMP_DIR}/${TARBALL}" -C "${TMP_DIR}"
fi

chmod +x "${TMP_DIR}/meshbeacon"

# 4. Check for OpenWrt Environment
if [ -f /etc/openwrt_release ] || command -v opkg >/dev/null 2>&1; then
  log "OpenWrt environment detected. Installing to ${INSTALL_DIR_OPENWRT}..."
  mv "${TMP_DIR}/meshbeacon" "${INSTALL_DIR_OPENWRT}/meshbeacon"

  # Setup procd init script
  cat << 'EOF' > /etc/init.d/meshbeacon
#!/bin/sh /etc/rc.common
USE_PROCD=1
START=95
STOP=10

PROG=/usr/bin/meshbeacon
CONFIG_FILE=/etc/meshbeacon/meshbeacon.json

start_service() {
    [ -f "$CONFIG_FILE" ] || return 1
    procd_open_instance
    procd_set_param command "$PROG" -config "$CONFIG_FILE"
    procd_set_param respawn 3600 5 0
    procd_set_param stdout 1
    procd_set_param stderr 1
    procd_close_instance
}
EOF
  chmod +x /etc/init.d/meshbeacon

  # Create config directory and example config if missing
  mkdir -p "${CONFIG_DIR}"
  if [ ! -f "${CONFIG_DIR}/meshbeacon.json" ]; then
    cat << 'EOF' > "${CONFIG_DIR}/meshbeacon.json"
{
  "advertise_addr": "auto",
  "cache": {
    "enabled": true,
    "ram_size": "auto",
    "disk_dir": "",
    "disk_size": "auto",
    "ttl": "30d"
  },
  "services": [
    {
      "name": "jellyfin",
      "local_port": 8096,
      "upstream_url": "http://mediaserver:8096",
      "discovery": {
        "type": "jellyfin",
        "port": 7359
      }
    }
  ]
}
EOF
    log "Created template configuration at ${CONFIG_DIR}/meshbeacon.json"
  fi

  /etc/init.d/meshbeacon enable
  log "OpenWrt service installed! Start with: /etc/init.d/meshbeacon start"
  exit 0
fi

# 5. Standard Linux Installation
if [ "${TARGET_OS}" = "linux" ]; then
  log "Installing binary to ${INSTALL_DIR_LINUX}/meshbeacon..."
  sudo mkdir -p "${INSTALL_DIR_LINUX}"
  sudo mv "${TMP_DIR}/meshbeacon" "${INSTALL_DIR_LINUX}/meshbeacon"

  # Setup default configuration
  sudo mkdir -p "${CONFIG_DIR}"
  if [ ! -f "${CONFIG_DIR}/meshbeacon.json" ]; then
    sudo tee "${CONFIG_DIR}/meshbeacon.json" > /dev/null << 'EOF'
{
  "advertise_addr": "auto",
  "cache": {
    "enabled": true,
    "ram_size": "auto",
    "disk_dir": "",
    "disk_size": "auto",
    "ttl": "30d"
  },
  "services": [
    {
      "name": "jellyfin",
      "local_port": 8096,
      "upstream_url": "http://mediaserver:8096",
      "discovery": {
        "type": "jellyfin",
        "port": 7359
      }
    }
  ]
}
EOF
    log "Created template configuration at ${CONFIG_DIR}/meshbeacon.json"
  fi

  # Setup systemd service if available
  if command -v systemctl >/dev/null 2>&1; then
    log "Configuring systemd service..."
    sudo tee /etc/systemd/system/meshbeacon.service > /dev/null << 'EOF'
[Unit]
Description=Meshbeacon - Local LAN Media Gateway & Autodiscovery
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/meshbeacon -config /etc/meshbeacon/meshbeacon.json
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
    sudo systemctl daemon-reload
    log "Systemd unit installed! Enable and run with:"
    echo "  sudo systemctl enable --now meshbeacon"
  fi

  log "Installation complete! Run 'meshbeacon --help' to verify."
  exit 0
fi

# 6. macOS Installation
if [ "${TARGET_OS}" = "darwin" ]; then
  DEST="/usr/local/bin/meshbeacon"
  mkdir -p /usr/local/bin 2>/dev/null || sudo mkdir -p /usr/local/bin
  sudo mv "${TMP_DIR}/meshbeacon" "${DEST}"
  log "Installed Meshbeacon to ${DEST}"
  log "Run 'meshbeacon --help' or start with: meshbeacon -jellyfin http://<server>:8096"
  exit 0
fi
