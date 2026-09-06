# ClamAV WebUI

A single Go binary with an embedded web UI to install, update, and operate
[ClamAV](https://www.clamav.net/) on a headless Debian/Ubuntu server.

- Install and upgrade ClamAV via `apt`
- On-demand and scheduled scans with live progress
- Optional loop-mounting of disk-image targets (`.iso`, `.udf`, `.img`) to scan their contents
- Signature updates (`freshclam`) and freshness monitoring
- Quarantine: hold, restore, delete infected files
- Manage the `clamav-daemon`, `clamav-freshclam` and `clamav-clamonacc` services
- Real-time (on-access) scanning with auto-quarantine
- Whitelisted `clamd.conf` / `freshclam.conf` editors
- Dashboard, an activity feed, and notifications (SMTP / webhook / ntfy)

## Install (headless server)

```bash
curl -fsSL https://github.com/FlightlessWeasel/clamav-webui/releases/latest/download/install.sh | sudo bash
```

Then open `http://<host>:8080` and set the admin password. Add `-s -- --with-clamav`
to also `apt`-install ClamAV in the same step.

Update in place. It rolls back if the new binary fails to start.

```bash
curl -fsSL https://github.com/FlightlessWeasel/clamav-webui/releases/latest/download/install.sh | sudo bash -s -- --update
```

`install.sh` flags: `--update`, `--os-upgrade`, `--with-clamav`, `--version TAG`,
`--repo OWNER/NAME`, `--port PORT`, `--no-start`, `--force`.

A Debian package is also attached to each release. It installs the same layout:
binary in `/usr/bin`, unit in `/lib/systemd/system`, state in
`/var/lib/clamav-webui`.

```bash
sudo dpkg -i clamav-webui_*_linux_amd64.deb
sudo systemctl enable --now clamav-webui
```

## Security

The service runs as root. It drives `apt`, `systemd` units, the ClamAV config
files, `fanotify`, and quarantined files. Protect it with the admin password.
Do not expose it to untrusted networks. Bind it to a LAN interface or put it
behind a VPN or an authenticating reverse proxy. For HTTPS, set
`CLAMWEB_TLS_CERT` and `CLAMWEB_TLS_KEY`.

## Configuration

| Env | Default | Meaning |
|---|---|---|
| `CLAMWEB_ADDR` | `:8080` | listen address |
| `CLAMWEB_CONFIG_DIR` | `/var/lib/clamav-webui` | state (DB, session secret, quarantine store) |
| `CLAMWEB_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `CLAMWEB_BROWSE_ROOT` | `/` | confines the path picker, scan targets and watch paths |
| `CLAMWEB_TLS_CERT`, `CLAMWEB_TLS_KEY` | — | enable HTTPS when both set |
| `CLAMWEB_CLAMD_CONF`, `CLAMWEB_FRESHCLAM_CONF` | `/etc/clamav/*.conf` | config files to manage |
| `CLAMWEB_CONFIG_FILE` | — | optional JSON config file |
| `CLAMWEB_DEV_SIM` | — | `1` swaps in an in-memory ClamAV simulator for local UI development |

## Development

Requires Go 1.26+ and Node 20+.

```bash
make dev     # prints the two commands to run (Go API + Vite dev server)
make test    # go test ./... + frontend tests
make build   # builds web/ then the Go binary into dist/
```

Run the whole app locally against the simulator (no ClamAV needed):

```bash
cd web && npm install && npm run build && cd ..
CLAMWEB_CONFIG_DIR=./.devstate CLAMWEB_ADDR=127.0.0.1:8080 CLAMWEB_DEV_SIM=1 go run ./cmd/clamav-webui
```

### Releasing

CI (`.github/workflows`) runs `go test`, `go vet`, `gofmt`, `tsc`, the frontend
tests, and a `goreleaser build` snapshot on every push/PR. Pushing a `vX.Y.Z`
tag builds the frontend and runs `goreleaser release`, which produces the
cross-platform archives, the `.deb`, `checksums.txt`, and attaches
`scripts/install.sh`.

Local dry run:

```bash
npm --prefix web ci && npm --prefix web run build
goreleaser release --snapshot --clean
```

### Docker

`docker build -t clamav-webui .` produces a `debian:bookworm-slim` image with
ClamAV bundled. On-access scanning and service management need host `systemd`
and `fanotify`, so the container has to run `--privileged` with the host's
systemd. The bare-metal install is the supported path.

## License

MIT
