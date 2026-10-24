# S16 (cont.) — systemd deployment and backups

Docker is the fast path; this is the boring one that enterprises actually run. Both use the
same binary and the same governance defaults.

## 1. Install

```bash
sudo install -m 0755 memalyze /usr/local/bin/memalyze
sudo useradd --system --home /var/lib/memalyze --shell /usr/sbin/nologin memalyze
sudo install -d -o memalyze -g memalyze -m 0750 /var/lib/memalyze /etc/memalyze
sudo install -m 0640 -o root -g memalyze deploy/systemd/memalyze.env.example /etc/memalyze/memalyze.env
sudo cp deploy/systemd/memalyze.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now memalyze
```

Edit `/etc/memalyze/memalyze.env` first — **`MEMALYZE_HTTP_TOKEN` must be changed**.
The unit binds `127.0.0.1:8080`; exposing it beyond loopback requires a reverse proxy with
TLS, and memalyze will refuse an off-loopback bind without a token anyway (D-021).

Check it:

```bash
systemctl status memalyze
journalctl -u memalyze -n 30       # stderr only; stdout is the MCP channel
curl -s http://127.0.0.1:8080/healthz
memalyze health --url http://127.0.0.1:8080/healthz
```

## 2. What the hardening actually covers

`ProtectSystem=strict` + `ReadWritePaths=/var/lib/memalyze` means the process can write
exactly one directory. `NoNewPrivileges`, `PrivateTmp`, `ProtectHome`, `RestrictAddressFamilies`
and `SystemCallFilter=@system-service` cut the usual escape routes. `UMask=0077` keeps the
SQLite file (which contains customer data) private by default.

If you run Ollama on the same host as a system service, uncomment `Requires=ollama.service`;
otherwise memalyze starts anyway and degrades to keyword search (D-015), which is the
behaviour you want at boot.

## 3. Backups

Do **not** `cp memalyze.db` while the service runs: WAL means recent pages may live in
`memalyze.db-wal`, and a naive copy gives you a database that opens but is missing rows.

```bash
sudo -u memalyze memalyze backup --db /var/lib/memalyze/memalyze.db \
  --out /var/backups/memalyze-$(date +%F).db
```

`backup` uses `VACUUM INTO`, which writes a consistent snapshot in one SQLite transaction
and refuses to overwrite an existing file. Verify a snapshot by opening it:

```bash
memalyze memory list --db /var/backups/memalyze-2026-10-25.db --limit 1
memalyze audit export --db /var/backups/memalyze-2026-10-25.db --format csv | head
```

Both memories and audit rows are inside the snapshot — an audit log that cannot be restored
alongside the data is not an audit log.

Suggested timer (daily 03:00, keep 14 days):

```ini
# /etc/systemd/system/memalyze-backup.service
[Unit]
Description=memalyze snapshot
[Service]
Type=oneshot
User=memalyze
ExecStart=/usr/local/bin/memalyze backup --db /var/lib/memalyze/memalyze.db --out /var/backups/memalyze-%Y-%m-%d.db
```
```ini
# /etc/systemd/system/memalyze-backup.timer
[Unit]
Description=Daily memalyze snapshot
[Timer]
OnCalendar=*-*-* 03:00:00
Persistent=true
[Install]
WantedBy=timers.target
```

`strftime` expansion in `ExecStart` is not available; wrap it in a shell script with
`date +%F` if you want the date in the filename.

## 4. Upgrades

```bash
sudo systemctl stop memalyze
sudo install -m 0755 memalyze-<new> /usr/local/bin/memalyze
sudo systemctl start memalyze
```

Schema migrations run at startup (`PRAGMA user_version`, currently v2). Downgrades are not
supported: keep a snapshot before upgrading, and roll back by restoring it.
