#!/bin/sh
# ==============================================================================
# Meshbeacon Package Post-Remove Script
# ==============================================================================
# Description:
#   Reloads systemd daemon after removal and purges /etc/meshbeacon if requested.
#
# Exit Codes:
#   0 - Success
# ==============================================================================
set -e

if [ -d /run/systemd/system ]; then
  systemctl --system daemon-reload >/dev/null 2>&1 || true
fi

if [ "$1" = "purge" ]; then
  rm -rf /etc/meshbeacon
fi
exit 0
