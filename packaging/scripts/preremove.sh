#!/bin/sh
# ==============================================================================
# Meshbeacon Package Pre-Remove Script
# ==============================================================================
# Description:
#   Stops and disables systemd service before package removal or upgrade.
#
# Exit Codes:
#   0 - Success
# ==============================================================================
set -e

if [ "$1" = "remove" ] || [ "$1" = "upgrade" ]; then
  if [ -d /run/systemd/system ]; then
    systemctl stop meshbeacon.service >/dev/null 2>&1 || true
    if [ "$1" = "remove" ]; then
      systemctl disable meshbeacon.service >/dev/null 2>&1 || true
    fi
  fi
fi
exit 0
