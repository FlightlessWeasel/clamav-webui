#!/bin/sh
set -e

mkdir -p /var/lib/clamav-webui
chmod 0700 /var/lib/clamav-webui

if command -v systemctl >/dev/null; then
    systemctl daemon-reload || true
    echo "clamav-webui installed. Enable with: systemctl enable --now clamav-webui"
    echo "Then open http://<this-host>:8080/ to set the admin password."
fi
