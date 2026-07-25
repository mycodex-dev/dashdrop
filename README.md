# Dashdrop

Upload-first static HTML hosting — drop a single `.html` file, get a live link in seconds. Self-hosted, local-first, single-container deployment.

Inspired by [Tiiny.host](https://tiiny.host) and [Netlify Drop](https://app.netlify.com/drop), scoped to single HTML files with a visual dashboard library.

## Quick Start

```bash
git clone https://github.com/mycodex-dev/dashdrop.git
cd dashdrop
docker compose up -d
```

Open [http://localhost:8080](http://localhost:8080), drag-and-drop an HTML file, and copy your live URL.

## Features

- **Upload first** — drag-and-drop is the primary experience
- **Instant publishing** — live URL in seconds, no account required
- **Static by design** — no build step, no SSR, no runtime transforms
- **Visual library** — browse all dashboards with auto-generated thumbnails
- **Tags** — organize dashboards with tags and filter the library
- **Local first** — filesystem storage, Docker Compose, one container

## Configuration

Set via environment variables (see [`.env.example`](.env.example)):

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP listen port |
| `DATA_DIR` | `/data` | Storage root (use a volume mount) |
| `MAX_UPLOAD_BYTES` | `5242880` | Max HTML file size (5 MB) |
| `MAX_THUMB_BYTES` | `1048576` | Max thumbnail size (1 MB) |
| `BASE_URL` | _(empty)_ | Optional absolute URL for copy-link (e.g. `https://dash.example.com`) |
| `MAX_UPLOADS_PER_MIN` | `10` | Rate limit per IP |

## Development

Requires Go 1.22+.

```bash
go run ./cmd/dashdrop
```

Data is stored in `./data` by default when running locally.

## API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/upload` | Upload HTML + thumbnail (`multipart: html`, `thumb`) |
| `GET` | `/api/dashboards` | List active dashboards (`?tag=` to filter; `?archived=1` for archived only). Expired dashboards are archived automatically. |
| `GET` | `/api/tags` | List unique tags from active dashboards |
| `PUT` | `/api/dashboards/{slug}` | Replace dashboard with a new HTML version |
| `PATCH` | `/api/dashboards/{slug}` | Update title, URL slug, tags, `archived`, and/or `expires_at` (YYYY-MM-DD or RFC3339; empty string clears) |
| `GET` | `/api/slugs/{slug}` | Check if a slug is available (`?except=` for current slug) |
| `DELETE` | `/api/dashboards/{slug}` | Delete a dashboard |
| `GET` | `/api/dashboards/{slug}/download` | Download the HTML file |
| `GET` | `/d/{slug}` | Serve published HTML (404 if archived) |
| `GET` | `/api/dashboards/{slug}/thumb.png` | Dashboard thumbnail |

## Security Notes

Dashdrop is **fully open** by design — anyone who can reach the instance can upload and browse dashboards. This is suitable for trusted networks (homelab, LAN) or instances placed behind your own access controls.

- Uploaded HTML runs with full browser privileges when visited
- Place behind a reverse proxy with TLS for production exposure
- Rate limiting is enabled by default (10 uploads/min/IP)

## Data Layout

```
data/
  manifest.json
  dashboards/
    {slug}/
      index.html
      thumb.png
      meta.json
```

## License

MIT
