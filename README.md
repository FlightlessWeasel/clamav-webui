# ClamAV WebUI

A single Go binary with an embedded web UI to install, update, and operate
[ClamAV](https://www.clamav.net/) on a headless Debian/Ubuntu server.

- Install and upgrade ClamAV via `apt`
- On-demand and scheduled scans with live progress
- Signature updates (`freshclam`) and freshness monitoring
- Quarantine: hold, restore, delete infected files
- Manage the `clamav-daemon`, `clamav-freshclam` and `clamav-clamonacc` services
- On-access (real-time) scanning setup
- Dashboard plus notifications (SMTP / webhook / ntfy)

## Status

Early development. See [the build plan](https://github.com/FlightlessWeasel/clamav-webui)
for the roadmap.

## Install (headless server)

```bash
curl -fsSL https://github.com/FlightlessWeasel/clamav-webui/releases/latest/download/install.sh | sudo bash
```

Then open `http://<host>:8080` and set the admin password.

Update in place:

```bash
curl -fsSL https://github.com/FlightlessWeasel/clamav-webui/releases/latest/download/install.sh | sudo bash -s -- --update
```

## Security

The service **runs as root** (it manages `apt`, `systemd` units, ClamAV config
files, and quarantined files). Protect it with the admin password and do **not**
expose it to untrusted networks — bind it to a LAN interface or put it behind a
VPN or an authenticating reverse proxy. Optional TLS: set `CLAMWEB_TLS_CERT` and
`CLAMWEB_TLS_KEY`.

## Development

Requires Go 1.26+ and Node 20+.

```bash
make dev     # prints the two commands to run (Go API + Vite dev server)
make test    # go test ./... + frontend tests
make build   # builds web/ then the Go binary into dist/
```

### Configuration

| Env | Default | Meaning |
|---|---|---|
| `CLAMWEB_ADDR` | `:8080` | listen address |
| `CLAMWEB_CONFIG_DIR` | `/var/lib/clamav-webui` | state (DB, quarantine store) |
| `CLAMWEB_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `CLAMWEB_TLS_CERT`, `CLAMWEB_TLS_KEY` | — | enable HTTPS when both set |
| `CLAMWEB_CONFIG_FILE` | — | optional JSON config file |

## License

MIT
