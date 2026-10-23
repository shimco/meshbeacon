#!/bin/sh
# ==============================================================================
# Meshbeacon Turnkey Installer & Manager
# ==============================================================================
# Description:
#   Auto-detects CPU architecture and operating system, verifies SHA256 checksums,
#   downloads the latest Meshbeacon release binary, and configures OpenWrt procd
#   or Linux systemd. Also supports clean, zero-trace uninstallation.
#
# Usage:
#   Install:     curl -sSL https://raw.githubusercontent.com/shimco/meshbeacon/main/install.sh | sh
#   Install Ver: curl -sSL https://raw.githubusercontent.com/shimco/meshbeacon/main/install.sh | sh -s -- v0.7.0
#   Uninstall:   curl -sSL https://raw.githubusercontent.com/shimco/meshbeacon/main/install.sh | sh -s -- --uninstall
#   Purge All:   curl -sSL https://raw.githubusercontent.com/shimco/meshbeacon/main/install.sh | sh -s -- --purge
#
# Exit Codes:
#   0 - Success
#   1 - Failure / Unsupported platform
# ==============================================================================
set -eu

REPO="shimco/meshbeacon"
INSTALL_DIR_LINUX="${DEST_DIR:-/usr/local/bin}"
INSTALL_DIR_OPENWRT="${DEST_DIR:-/usr/bin}"
CONFIG_DIR="/etc/meshbeacon"

log() {
  printf "\033[1;34m[Meshbeacon]\033[0m %s\n" "$*"
}

error() {
  printf "\033[1;31m[Error]\033[0m %s\n" "$*" >&2
  exit 1
}

run_as_root() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  elif command -v doas >/dev/null 2>&1; then
    doas "$@"
  else
    error "Root privileges required to run: $*"
  fi
}

# ------------------------------------------------------------------------------
# Uninstallation Routine
# ------------------------------------------------------------------------------
uninstall() {
  log "Uninstalling Meshbeacon..."

  # 1. Stop and remove OpenWrt procd service
  if [ -f /etc/init.d/meshbeacon ]; then
    log "Stopping and disabling OpenWrt service..."
    /etc/init.d/meshbeacon stop 2>/dev/null || true
    /etc/init.d/meshbeacon disable 2>/dev/null || true
    rm -f /etc/init.d/meshbeacon
  fi

  # 2. Stop and remove Linux systemd service
  if [ -f /etc/systemd/system/meshbeacon.service ]; then
    log "Stopping and removing systemd service..."
    run_as_root systemctl stop meshbeacon 2>/dev/null || true
    run_as_root systemctl disable meshbeacon 2>/dev/null || true
    run_as_root rm -f /etc/systemd/system/meshbeacon.service
    run_as_root systemctl daemon-reload 2>/dev/null || true
  fi

  # 3. Remove installed binaries
  for bin in "${INSTALL_DIR_LINUX}/meshbeacon" "${INSTALL_DIR_OPENWRT}/meshbeacon"; do
    if [ -f "${bin}" ]; then
      log "Removing binary: ${bin}"
      if [ -w "${bin}" ] && [ -w "$(dirname "${bin}")" ]; then
        rm -f "${bin}"
      else
        run_as_root rm -f "${bin}"
      fi
    fi
  done

  # 4. Handle configuration and cache directory
  if [ "${1:-}" = "--purge" ] || [ "${PURGE:-0}" = "1" ]; then
    if [ -d "${CONFIG_DIR}" ]; then
      log "Purging configuration directory: ${CONFIG_DIR}..."
      run_as_root rm -rf "${CONFIG_DIR}"
    fi
    if [ -d "/var/cache/meshbeacon" ]; then
      log "Purging asset disk cache: /var/cache/meshbeacon..."
      run_as_root rm -rf "/var/cache/meshbeacon"
    fi
  else
    if [ -d "${CONFIG_DIR}" ]; then
      log "Configuration preserved at ${CONFIG_DIR} (pass '--purge' to delete)."
    fi
  fi

  log "Meshbeacon has been cleanly uninstalled."
  exit 0
}

# Check for uninstall flags
case "${1:-}" in
  --uninstall|-u|uninstall)
    uninstall "${2:-}"
    ;;
  --purge|purge)
    uninstall "--purge"
    ;;
  --help|-h|help)
    echo "Meshbeacon Installer & Uninstaller"
    echo "Usage:"
    echo "  install.sh [version]      Install or upgrade Meshbeacon (e.g. v0.7.0 or latest)"
    echo "  install.sh --uninstall    Remove binary and background service (preserves config)"
    echo "  install.sh --purge        Remove binary, background service, and config directory"
    exit 0
    ;;
esac

# ------------------------------------------------------------------------------
# 1. Detect Operating System and Architecture
# ------------------------------------------------------------------------------
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
    error "Unsupported CPU architecture: ${ARCH}"
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

# ------------------------------------------------------------------------------
# 2. Determine Version to Install
# ------------------------------------------------------------------------------
VERSION="${1:-}"
if [ -z "${VERSION}" ]; then
  log "Querying latest release from GitHub..."
  LATEST_JSON="$(curl -sSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null || true)"
  VERSION="$(echo "${LATEST_JSON}" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' || true)"
  if [ -z "${VERSION}" ]; then
    VERSION="v0.7.0"
  fi
fi

log "Installing Meshbeacon ${VERSION}..."

# ------------------------------------------------------------------------------
# 3. Download and Verify Release Binary
# ------------------------------------------------------------------------------
TARBALL="meshbeacon_${TARGET_OS}_${TARGET_ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL}"
CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

log "Downloading ${TARBALL}..."
if ! curl -sSL --fail "${DOWNLOAD_URL}" -o "${TMP_DIR}/${TARBALL}"; then
  error "Failed to download Meshbeacon release asset: ${DOWNLOAD_URL}"
fi

# Verify SHA256 checksum if utility is present
if curl -sSL --fail "${CHECKSUMS_URL}" -o "${TMP_DIR}/checksums.txt" 2>/dev/null; then
  EXPECTED_HASH="$(grep " ${TARBALL}\$" "${TMP_DIR}/checksums.txt" | awk '{print $1}')"
  if [ -n "${EXPECTED_HASH}" ]; then
    if command -v sha256sum >/dev/null 2>&1; then
      ACTUAL_HASH="$(sha256sum "${TMP_DIR}/${TARBALL}" | awk '{print $1}')"
    elif command -v shasum >/dev/null 2>&1; then
      ACTUAL_HASH="$(shasum -a 256 "${TMP_DIR}/${TARBALL}" | awk '{print $1}')"
    else
      ACTUAL_HASH=""
    fi

    if [ -n "${ACTUAL_HASH}" ]; then
      if [ "${ACTUAL_HASH}" != "${EXPECTED_HASH}" ]; then
        error "Checksum verification failed! Expected: ${EXPECTED_HASH}, got: ${ACTUAL_HASH}"
      fi
      log "SHA-256 verification passed: ${ACTUAL_HASH}"
    fi
  fi
fi

tar -xzf "${TMP_DIR}/${TARBALL}" -C "${TMP_DIR}"
chmod +x "${TMP_DIR}/meshbeacon"

# ------------------------------------------------------------------------------
# 4. OpenWrt Environment Installation
# ------------------------------------------------------------------------------
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

  # Create config directory and default configuration if missing
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
    "ttl": "auto"
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

# ------------------------------------------------------------------------------
# 5. Standard Linux Installation
# ------------------------------------------------------------------------------
if [ "${TARGET_OS}" = "linux" ]; then
  log "Installing binary to ${INSTALL_DIR_LINUX}/meshbeacon..."
  run_as_root mkdir -p "${INSTALL_DIR_LINUX}"
  run_as_root mv "${TMP_DIR}/meshbeacon" "${INSTALL_DIR_LINUX}/meshbeacon"

  # Setup default configuration
  run_as_root mkdir -p "${CONFIG_DIR}"
  if [ ! -f "${CONFIG_DIR}/meshbeacon.json" ]; then
    run_as_root tee "${CONFIG_DIR}/meshbeacon.json" > /dev/null << 'EOF'
{
  "advertise_addr": "auto",
  "cache": {
    "enabled": true,
    "ram_size": "auto",
    "disk_dir": "",
    "disk_size": "auto",
    "ttl": "auto"
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
    run_as_root tee /etc/systemd/system/meshbeacon.service > /dev/null << 'EOF'
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
    run_as_root systemctl daemon-reload
    log "Systemd unit installed! Enable and run with:"
    printf "  sudo systemctl enable --now meshbeacon\n"
  fi

  log "Installation complete! Run 'meshbeacon --help' to verify."
  exit 0
fi

# ------------------------------------------------------------------------------
# 6. macOS Installation
# ------------------------------------------------------------------------------
if [ "${TARGET_OS}" = "darwin" ]; then
  DEST="${INSTALL_DIR_LINUX}/meshbeacon"
  TARGET_DIR="$(dirname "${DEST}")"
  IS_WRITABLE=0
  if [ -w "${TARGET_DIR}" ] 2>/dev/null; then
    IS_WRITABLE=1
  elif [ ! -d "${TARGET_DIR}" ] && [ -w "$(dirname "${TARGET_DIR}")" ] 2>/dev/null; then
    IS_WRITABLE=1
  fi

  if [ "${IS_WRITABLE}" = "1" ]; then
    mkdir -p "${INSTALL_DIR_LINUX}"
    mv "${TMP_DIR}/meshbeacon" "${DEST}"
  else
    run_as_root mkdir -p "${INSTALL_DIR_LINUX}"
    run_as_root mv "${TMP_DIR}/meshbeacon" "${DEST}"
  fi
  log "Installed Meshbeacon to ${DEST}"
  log "Run '${DEST} --help' or start with: ${DEST} -jellyfin http://<server>:8096"
  exit 0
fi
