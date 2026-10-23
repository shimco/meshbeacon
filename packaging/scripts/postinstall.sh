#!/bin/sh
# ==============================================================================
# Meshbeacon Package Post-Install Script
# ==============================================================================
# Description:
#   Reloads systemd daemon and enables/starts the Meshbeacon service.
#
# Exit Codes:
#   0 - Success
# ==============================================================================
set -e

if [ "$1" = "configure" ] || [ "$1" = "abort-upgrade" ]; then
  if [ -d /run/systemd/system ]; then
    systemctl --system daemon-reload >/dev/null 2>&1 || true
    if ! systemctl is-enabled meshbeacon.service >/dev/null 2>&1; then
      systemctl enable meshbeacon.service >/dev/null 2>&1 || true
    fi
    systemctl restart meshbeacon.service >/dev/null 2>&1 || true
  fi
fi
exit 0
