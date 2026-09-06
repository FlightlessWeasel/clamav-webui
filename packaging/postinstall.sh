#!/bin/sh
set -e

mkdir -p /var/lib/clamav-webui
chmod 0700 /var/lib/clamav-webui

# Kernel modules for the optional "scan disk images" feature: loop for the
# loop-mount, udf for game/optical .iso images. Best-effort; harmless if built
# in already.
for m in loop udf; do
    modprobe "$m" 2>/dev/null || true
done
if [ -d /etc/modules-load.d ]; then
    printf '# clamav-webui: disk-image scanning\nloop\nudf\n' \
        > /etc/modules-load.d/clamav-webui.conf 2>/dev/null || true
fi

if command -v systemctl >/dev/null; then
    systemctl daemon-reload || true
    echo "clamav-webui installed. Enable with: systemctl enable --now clamav-webui"
    echo "Then open http://<this-host>:8080/ to set the admin password."
fi
