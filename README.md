<div align="center">
  <img src="web/static/icon.png" alt="GoEncode" width="128" />

# GoEncode

GoEncode is a lightweight, high-performance media transcoding server written in Go. Originally built as a Flask app, it has been rewritten from the ground up as a single, self-contained binary. GoEncode features a modern web UI, a robust job queue, automatic watch folder monitoring, real-time log streaming, and SQLite by default (MariaDB optional).

</div>

## Features

- **Watch Folders**: Automatically monitor designated directories for new media files.
- **Smart Queue & Debouncing**: Prevents incomplete files from being processed while they are still being copied to the watch folder.
- **Background Processing**: Encodes jobs concurrently with a configurable worker pool (default 1, max 16). Files are copied to a temporary directory before encoding to avoid hammering network-attached storage (NAS).
- **Format Intelligence**: Probes media to detect codecs and resolution, automatically skipping files that already meet the target codec/resolution.
- **Web Dashboard**: Modern, responsive interface with Server-Sent Events (SSE) for live tracking of job progress, queue stats, and server logs.
- **SQLite by default**: Run the binary with no database server. MariaDB/MySQL is still available for larger installs.
- **Notifications**: Optional ntfy, Discord, Gotify, or generic JSON webhooks on encode success, skip, and failure. Success messages include size saved so you can see when a job actually paid off.
- **Docker-Ready**: Packaged in an ultra-slim container image based on Debian, with `ffmpeg` built-in.

## Deployment with Docker

The easiest way to run GoEncode is via Docker using the pre-built image. Docker images do not self-update; pull a new image instead.

### Image Registry

`cr.jdbnet.co.uk/public/goencode:latest`

### Example `docker-compose.yml`

```yaml
services:
  goencode:
    image: cr.jdbnet.co.uk/public/goencode:latest
    container_name: goencode
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      - GOENCODE_DB_DRIVER=mysql
      - GOENCODE_DB_HOST=db
      - GOENCODE_DB_PORT=3306
      - GOENCODE_DB_USER=goencode
      - GOENCODE_DB_PASS=goencode_password
      - GOENCODE_DB_NAME=goencode
      - GOENCODE_LISTEN_ADDR=0.0.0.0
      - GOENCODE_PORT=8080
      - GOENCODE_ENCODER_TEMP=/tmp/goencode
      - GOENCODE_ENCODER_WORKERS=1
      - GOENCODE_ENCODER_MIN_FREE_GB=5
      - TZ=Europe/London
      # Web UI Authentication (Optional)
      - GOENCODE_AUTH_USER=admin
      - GOENCODE_AUTH_PASS=secret
      # Notifications (Optional). Default events: success, skip, failed
      - GOENCODE_WEBHOOK_URL=http://your-webhook-endpoint.com/webhook
      # - GOENCODE_NTFY_URL=https://ntfy.sh/goencode
      # - GOENCODE_DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/id/token
      # - GOENCODE_GOTIFY_URL=https://gotify.example.com
      # - GOENCODE_GOTIFY_TOKEN=app-token
      # - GOENCODE_NOTIFY_EVENTS=success,skip,failed
    volumes:
      - /path/to/your/media:/media
    depends_on:
      - db

  db:
    image: mariadb:latest
    container_name: goencode-db
    restart: unless-stopped
    environment:
      - MARIADB_DATABASE=goencode
      - MARIADB_USER=goencode
      - MARIADB_PASSWORD=goencode_password
      - MARIADB_ROOT_PASSWORD=root_password
    volumes:
      - goencode_db_data:/var/lib/mysql

volumes:
  goencode_db_data:
```

## Deployment with Binary

Linux AMD64. Needs `ffmpeg` (`sudo apt install ffmpeg`, or `dnf` / `pacman -S ffmpeg`).

```bash
curl -L -o goencode https://apps.jdbnet.co.uk/goencode
chmod +x goencode
./goencode
```

Open [http://127.0.0.1:8080](http://127.0.0.1:8080). The SQLite database is `goencode.db` in the current directory. To change settings, copy [goencode.yaml](goencode.yaml) next to the binary or pass `--config`.

To run at boot:

```bash
sudo install -m 755 goencode /usr/local/bin/goencode
sudo mkdir -p /var/lib/goencode
sudo tee /etc/systemd/system/goencode.service >/dev/null <<'EOF'
[Unit]
Description=GoEncode
After=network.target

[Service]
WorkingDirectory=/var/lib/goencode
ExecStart=/usr/local/bin/goencode
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF
sudo systemctl enable --now goencode
```

Same unit: [contrib/goencode.service](contrib/goencode.service). Logs: `journalctl -u goencode -f`.

## Configuration

GoEncode can be configured via `goencode.yaml` or entirely via environment variables (ideal for Docker/Kubernetes). Environment variables take precedence over the YAML file.

### Available Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `GOENCODE_DB_DRIVER` | `sqlite` or `mysql` (`mariadb` is accepted). Unset: sqlite if no host, mysql if `GOENCODE_DB_HOST` is set | `sqlite` |
| `GOENCODE_DB_PATH` | SQLite file path | `goencode.db` |
| `GOENCODE_DB_HOST` | MariaDB/MySQL host. If set and driver is unset, selects mysql | |
| `GOENCODE_DB_PORT` | Database port | `3306` |
| `GOENCODE_DB_USER` | Database username | `goencode` |
| `GOENCODE_DB_PASS` | Database password | |
| `GOENCODE_DB_NAME` | Database name | `goencode` |
| `GOENCODE_LISTEN_ADDR` | IP to bind the web interface. Alias: `GOENCODE_SERVER_LISTEN` | `0.0.0.0` |
| `GOENCODE_PORT` | Port for the web interface. Alias: `GOENCODE_SERVER_PORT` | `8080` |
| `TZ` | Container TimeZone | `UTC` |
| `GOENCODE_AUTH_USER` | Username for the web UI | |
| `GOENCODE_AUTH_PASS` | Password for the web UI | |
| `GOENCODE_WEBHOOK_URL` | Generic JSON webhook (or a Discord webhook URL) | |
| `GOENCODE_NTFY_URL` | ntfy topic URL, e.g. `https://ntfy.sh/mytopic` | |
| `GOENCODE_NTFY_TOKEN` | Optional ntfy access token | |
| `GOENCODE_DISCORD_WEBHOOK_URL` | Discord incoming webhook URL | |
| `GOENCODE_GOTIFY_URL` | Gotify server URL | |
| `GOENCODE_GOTIFY_TOKEN` | Gotify application token | |
| `GOENCODE_NOTIFY_EVENTS` | Comma-separated events: `success`, `skip`, `failed` | `success,skip,failed` |
| `GOENCODE_ENCODER_TEMP` | Temp directory for processing jobs | `/tmp/goencode` |
| `GOENCODE_ENCODER_WORKERS` | Concurrent encode jobs (1-16) | `1` |
| `GOENCODE_ENCODER_MIN_FREE_GB` | Abort encode if temp or output filesystem would drop below this many GB (also refuses copies larger than free space) | `5` |
| `GOENCODE_NO_UPDATE` | Set to `1` or `true` to disable binary auto-update on startup | |

Success notifications include the filename and size saved (for example `Saved 40.0 GB`). Skip notifications fire when a queued job is skipped (already the target codec, or encoded file was larger). Files skipped during a folder scan are not notified, so a library scan does not flood your phone. Use `GOENCODE_NOTIFY_EVENTS=failed` to keep failure-only alerts.
