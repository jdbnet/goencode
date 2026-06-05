<div align="center">
  <img src="web/static/icon.png" alt="GoEncode" width="128" />

# GoEncode

GoEncode is a lightweight, high-performance media transcoding server written in Go. Originally built as a Flask app, it has been rewritten from the ground up as a single, self-contained binary. GoEncode features a modern web UI, a robust job queue, automatic watch folder monitoring, real-time log streaming, and MariaDB-backed persistence.

</div>

## Features

- **Watch Folders**: Automatically monitor designated directories for new media files.
- **Smart Queue & Debouncing**: Prevents incomplete files from being processed while they are still being copied to the watch folder.
- **Background Processing**: Processes media sequentially with up to 3 worker threads. Files are safely copied to a temporary directory before encoding to avoid hammering network-attached storage (NAS).
- **Format Intelligence**: Probes media to detect codecs and resolution, automatically skipping files that already meet the target codec/resolution.
- **Web Dashboard**: Modern, responsive interface with Server-Sent Events (SSE) for live tracking of job progress, queue stats, and server logs.
- **Robust Persistence**: Job history, metrics (size saved, time taken), and configuration are all saved to a MariaDB/MySQL database.
- **Docker-Ready**: Packaged in an ultra-slim container image based on Debian, with `ffmpeg` built-in.

## Deployment with Docker

The easiest way to run GoEncode is via Docker using the pre-built image.

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
      # Database Configuration
      - GOENCODE_DB_HOST=db
      - GOENCODE_DB_PORT=3306
      - GOENCODE_DB_USER=goencode
      - GOENCODE_DB_PASS=goencode_password
      - GOENCODE_DB_NAME=goencode
      
      # Web Server Configuration
      - GOENCODE_SERVER_LISTEN=0.0.0.0
      - GOENCODE_SERVER_PORT=8080
      - TZ=Europe/London
      
      # Web UI Authentication (Optional)
      - GOENCODE_AUTH_USER=admin
      - GOENCODE_AUTH_PASS=secret
      
      # Encoding
      - GOENCODE_ENCODER_TEMP=/tmp/goencode
    volumes:
      - /path/to/your/media:/media
    depends_on:
      - db

  db:
    image: mariadb:10.11
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

## Configuration

GoEncode can be configured via `goencode.yaml` or entirely via environment variables (ideal for Docker/Kubernetes). Environment variables take precedence over the YAML file.

### Available Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `GOENCODE_DB_HOST` | Database host | `127.0.0.1` |
| `GOENCODE_DB_PORT` | Database port | `3306` |
| `GOENCODE_DB_USER` | Database username | `goencode` |
| `GOENCODE_DB_PASS` | Database password | |
| `GOENCODE_DB_NAME` | Database name | `goencode` |
| `GOENCODE_SERVER_LISTEN` | IP to bind the web interface | `0.0.0.0` |
| `GOENCODE_SERVER_PORT` | Port for the web interface | `8080` |
| `TZ` | Container TimeZone | `UTC` |
| `GOENCODE_AUTH_USER` | Username for the web UI | |
| `GOENCODE_AUTH_PASS` | Password for the web UI | |
| `GOENCODE_ENCODER_TEMP`| Temp directory for processing jobs | `/tmp/goencode` |

## Building Locally

To build the Docker image locally and push it to your registry:

```bash
# Build the image
docker build -t cr.jdbnet.co.uk/public/goencode:latest .

# Push to your registry
docker push cr.jdbnet.co.uk/public/goencode:latest
```