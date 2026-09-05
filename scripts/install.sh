#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────────────────
# clamav-webui installer / updater  (Debian/Ubuntu, systemd)
#
#   Fresh install : download the latest release archive, write a systemd unit
#                   (running as root), enable + start it.
#   --update      : replace the installed binary with the latest release
#                   (or --version TAG) and restart the service, with
#                   automatic rollback if it fails to come back up.
#   --os-upgrade  : run `apt-get update && apt-get -y upgrade` first.
#   --with-clamav : after the service is up, apt-install clamav + clamav-daemon
#                   + clamav-freshclam so the box is immediately usable.
#
# No Go or Node toolchain required. It pulls the prebuilt archives that
# GoReleaser publishes to GitHub Releases.
#
# Bootstrap straight from the web:
#   curl -fsSL https://github.com/FlightlessWeasel/clamav-webui/releases/latest/download/install.sh | sudo bash
#   curl -fsSL https://github.com/FlightlessWeasel/clamav-webui/releases/latest/download/install.sh | sudo bash -s -- --update
#
# The layout matches the .deb package: binary in /usr/bin, state (SQLite DB,
# session secret, quarantine store) in /var/lib/<service>.
#
# SECURITY: the service runs as root. Do not expose it to untrusted networks.
# Bind it to a LAN interface or put it behind a VPN / authenticating proxy.
# ─────────────────────────────────────────────────────────

REPO="${CLAMWEB_REPO:-FlightlessWeasel/clamav-webui}"
SVC_NAME="clamav-webui"
BINDIR="/usr/bin"
PORT="8080"
VERSION=""          # empty => latest
DO_UPDATE=false
DO_OS_UPGRADE=false
WITH_CLAMAV=false
NO_START=false
FORCE=false

usage() {
  cat <<EOF
Usage: install.sh [options]

  --update            Update an existing install to the latest release, then restart.
  --os-upgrade        Run apt-get update && apt-get -y upgrade first.
  --with-clamav       apt-install clamav + clamav-daemon + clamav-freshclam after setup.
  --version TAG       Install/update to a specific release tag (e.g. v1.2.3). Default: latest.
  --repo OWNER/NAME   GitHub repo to fetch releases from. Default: ${REPO}.
  --service NAME      systemd service + state-dir name. Default: ${SVC_NAME}.
  --bindir PATH       Directory to install the binary into. Default: ${BINDIR}.
  --port PORT         TCP port the server listens on (CLAMWEB_ADDR=:PORT). Default: ${PORT}.
  --no-start          Install and enable the unit but do not start it now.
  --force             Reinstall / rewrite the unit even if already at the target version.
  -h, --help          Show this help.

Environment:
  CLAMWEB_REPO        Same as --repo.
  GITHUB_TOKEN        Optional; raises the GitHub API rate limit for tag lookup.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --update)      DO_UPDATE=true; shift ;;
    --os-upgrade)  DO_OS_UPGRADE=true; shift ;;
    --with-clamav) WITH_CLAMAV=true; shift ;;
    --version)     VERSION="${2:?--version needs a tag}"; shift 2 ;;
    --repo)        REPO="${2:?--repo needs owner/name}"; shift 2 ;;
    --service)     SVC_NAME="${2:?--service needs a name}"; shift 2 ;;
    --bindir)      BINDIR="${2:?--bindir needs a path}"; shift 2 ;;
    --port)        PORT="${2:?--port needs a value}"; shift 2 ;;
    --no-start)    NO_START=true; shift ;;
    --force)       FORCE=true; shift ;;
    -h|--help)     usage; exit 0 ;;
    *)             echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

BIN="${BINDIR}/${SVC_NAME}"
BACKUP="${BIN}.bak"
STATE_DIR="/var/lib/${SVC_NAME}"
VERSION_FILE="${STATE_DIR}/.version"
UNIT_FILE="/etc/systemd/system/${SVC_NAME}.service"

die()  { echo "error: $*" >&2; exit 1; }
info() { echo "==> $*"; }

[[ "$(id -u)" -eq 0 ]] || die "must run as root (try: sudo $0 $*)"

for c in curl tar sha256sum systemctl install; do
  command -v "$c" >/dev/null 2>&1 || die "required command not found: $c"
done

# ── OS package upgrade ──────────────────────────────────
os_upgrade() {
  info "upgrading OS packages"
  command -v apt-get >/dev/null 2>&1 || die "--os-upgrade needs apt-get (Debian/Ubuntu)"
  DEBIAN_FRONTEND=noninteractive apt-get update
  DEBIAN_FRONTEND=noninteractive apt-get -y upgrade
}

# ── arch detection ──────────────────────────────────────
detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)  echo amd64 ;;
    aarch64|arm64) echo arm64 ;;
    *)             die "unsupported architecture: $(uname -m)" ;;
  esac
}

# ── release resolution ──────────────────────────────────
latest_tag() {
  local api="https://api.github.com/repos/${REPO}/releases/latest"
  local hdr=(-fsSL -H "Accept: application/vnd.github+json")
  [[ -n "${GITHUB_TOKEN:-}" ]] && hdr+=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
  local body
  body="$(curl "${hdr[@]}" "$api")" || return 1
  printf '%s\n' "$body" \
    | grep -m1 '"tag_name"' \
    | sed -E 's/.*"tag_name" *: *"([^"]+)".*/\1/'
}

# download_release <tag> <arch> <destdir> -> sets $EXTRACTED to the binary path
download_release() {
  local tag="$1" arch="$2" dest="$3"
  local ver="${tag#v}"
  local asset="${SVC_NAME}_${ver}_linux_${arch}.tar.gz"
  local base="https://github.com/${REPO}/releases/download/${tag}"

  info "downloading ${asset}"
  curl -fSL -o "${dest}/${asset}" "${base}/${asset}" \
    || die "download failed: ${base}/${asset}"

  if curl -fsSL -o "${dest}/checksums.txt" "${base}/checksums.txt"; then
    info "verifying checksum"
    ( cd "$dest" && grep " ${asset}\$" checksums.txt | sha256sum -c - ) \
      || die "checksum verification failed for ${asset}"
  else
    echo "    (no checksums.txt in release — skipping checksum verify)" >&2
  fi

  tar -C "$dest" -xzf "${dest}/${asset}"
  [[ -f "${dest}/${SVC_NAME}" ]] || die "archive did not contain '${SVC_NAME}'"
  EXTRACTED="${dest}/${SVC_NAME}"
}

# ── state dir ───────────────────────────────────────────
ensure_state() {
  mkdir -p "$STATE_DIR"
  chmod 0700 "$STATE_DIR"
}

# ── systemd unit ────────────────────────────────────────
# Kept in sync with packaging/clamav-webui.service (the .deb ships that copy).
# Runs as root; no sandboxing directives (needs apt/systemctl/fanotify).
write_unit() {
  info "writing ${UNIT_FILE}"
  cat > "$UNIT_FILE" <<EOF
[Unit]
Description=ClamAV WebUI
Documentation=https://github.com/${REPO}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
Environment=CLAMWEB_CONFIG_DIR=${STATE_DIR}
Environment=CLAMWEB_ADDR=:${PORT}
ExecStart=${BIN}
Restart=on-failure
RestartSec=5
StateDirectory=${SVC_NAME}

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
}

install_binary() {
  local src="$1"
  mkdir -p "$BINDIR"
  if [[ -f "$BIN" ]]; then
    cp -p "$BIN" "$BACKUP"
    echo "    backed up existing binary to ${BACKUP}"
  fi
  local tmp
  tmp="$(mktemp "${BIN}.new.XXXXXX")"
  install -m 0755 "$src" "$tmp"
  mv -f "$tmp" "$BIN"
}

rollback_binary() {
  [[ -f "$BACKUP" ]] || { echo "    no backup to roll back to" >&2; return; }
  info "rolling back ${BIN}"
  local tmp
  tmp="$(mktemp "${BIN}.rollback.XXXXXX")"
  cp -p "$BACKUP" "$tmp"
  mv -f "$tmp" "$BIN"
  systemctl restart "$SVC_NAME" || true
}

wait_active() {
  local i
  for ((i = 0; i < 10; i++)); do
    systemctl is-active --quiet "$SVC_NAME" && return 0
    sleep 1
  done
  return 1
}

install_clamav() {
  info "installing ClamAV packages"
  command -v apt-get >/dev/null 2>&1 || die "--with-clamav needs apt-get (Debian/Ubuntu)"
  DEBIAN_FRONTEND=noninteractive apt-get update
  DEBIAN_FRONTEND=noninteractive apt-get -y install clamav clamav-daemon clamav-freshclam
}

# ── main ────────────────────────────────────────────────
$DO_OS_UPGRADE && os_upgrade

ARCH="$(detect_arch)"

TAG="$VERSION"
if [[ -z "$TAG" ]]; then
  info "resolving latest release for ${REPO}"
  TAG="$(latest_tag)" || die "could not reach the GitHub releases API"
  [[ -n "$TAG" ]] || die "could not determine latest release tag"
fi
info "target version: ${TAG} (linux/${ARCH})"

CURRENT=""
[[ -f "$VERSION_FILE" ]] && CURRENT="$(cat "$VERSION_FILE")"

if [[ "$TAG" == "$CURRENT" && "$FORCE" == false && "$DO_UPDATE" == false ]]; then
  echo "already at ${TAG} — nothing to do (use --force to reinstall)"
  exit 0
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
EXTRACTED=""
download_release "$TAG" "$ARCH" "$WORK"

ensure_state

# ── update path ─────────────────────────────────────────
if $DO_UPDATE; then
  [[ -f "$BIN" ]] || die "--update: no existing install at ${BIN} (run without --update first)"
  install_binary "$EXTRACTED"
  echo "$TAG" > "$VERSION_FILE"
  if systemctl is-enabled "$SVC_NAME" >/dev/null 2>&1; then
    info "restarting ${SVC_NAME}"
    if ! systemctl restart "$SVC_NAME" || ! wait_active; then
      echo "    service did not come back up" >&2
      rollback_binary
      echo "$CURRENT" > "$VERSION_FILE"
      die "update failed — rolled back to previous binary"
    fi
    echo "    restarted on ${TAG}"
  else
    echo "    service ${SVC_NAME} not enabled — binary updated, start it yourself"
  fi
  info "done"
  exit 0
fi

# ── fresh install (or --force reinstall) ────────────────
install_binary "$EXTRACTED"
echo "$TAG" > "$VERSION_FILE"

if [[ ! -f "$UNIT_FILE" || "$FORCE" == true ]]; then
  write_unit
else
  echo "    ${UNIT_FILE} already exists — leaving it untouched (use --force to rewrite)"
fi

systemctl enable "$SVC_NAME" >/dev/null

if $NO_START; then
  echo "    --no-start: not starting ${SVC_NAME}"
else
  info "starting ${SVC_NAME}"
  systemctl restart "$SVC_NAME"
  if wait_active; then
    echo "    active on ${TAG}"
  else
    systemctl status "$SVC_NAME" --no-pager || true
    die "service failed to start — check: journalctl -u ${SVC_NAME} -e"
  fi
fi

$WITH_CLAMAV && install_clamav

cat <<EOF

clamav-webui ${TAG} is installed.

  Web UI:   http://<this-host>:${PORT}/   (open it to set the admin password)
  State:    ${STATE_DIR}   (SQLite DB, session secret, quarantine store)
  Service:  systemctl status ${SVC_NAME}
  Logs:     journalctl -u ${SVC_NAME} -f
  Update:   sudo $0 --update

The service runs as root; keep it off untrusted networks.

EOF
info "done"
