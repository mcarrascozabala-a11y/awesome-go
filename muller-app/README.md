# Saisie Ligne Muller

Shift-entry app for the Muller line (Top shells): one record per shift with
opening hours, parts produced, changeovers and stoppages, a weekly view with a
stoppage Pareto, and the CSV export for the "Saisie manuelle" sheet.

It is a single Go binary: a JSON API backed by SQLite, plus an installable web
app (PWA) for the shop-floor tablets and phones. It is a separate Go module
and is not part of the awesome-go site generator.

## Run

```sh
cd muller-app
MULLER_PASSWORD='choose-a-shared-password' go run .
# → http://localhost:8080
```

| Setting | Flag | Environment | Default |
| --- | --- | --- | --- |
| Listen address | `-addr` | `MULLER_ADDR` (or `PORT`) | `:8080` |
| Database file | `-db` | `MULLER_DB` | `muller.db` |
| Shared password (HTTP Basic auth, any user name) | | `MULLER_PASSWORD` | none: open access |

With Docker:

```sh
docker build -t muller-app muller-app
docker run -p 8080:8080 -v muller-data:/data -e MULLER_PASSWORD=... muller-app
```

Back up the database by copying the `muller.db` file (or the `/data` volume).

## Install on tablets

Serve the app over **HTTPS** (for example behind Caddy, nginx or the hosting
platform's TLS). Browsers only allow installing and offline use on HTTPS or
`localhost`. Then open the address on the tablet and choose **Add to Home
Screen** (iOS/iPadOS Safari) or **Install app** (Android Chrome).

Offline behavior: the app opens without network; a shift saved while offline
is kept on the device (the banner shows how many are waiting) and sent
automatically when the server is reachable again. Entries use client-generated
IDs, so a retried save never creates a duplicate.

## Rules enforced (client and server)

- No shift on Sunday; the Sunday→Monday night is entered on Monday.
- Saturday: morning shift only.
- Setter rotation: MR works mornings on odd ISO weeks and afternoons on even
  weeks, BC the opposite (the form proposes it and warns on a mismatch).
- Night shifts have no setter (`equipe` = `Nuit`).

## API

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/entries?year=&week=` | Entries of an ISO week (default: current); `?all=1` for everything |
| `GET` | `/api/entries/{id}` | One entry |
| `PUT` | `/api/entries/{id}` | Create or replace an entry |
| `DELETE` | `/api/entries/{id}` | Delete an entry |
| `GET` | `/api/refs` | Recently used product references |
| `GET` | `/api/export?year=&week=` | "Saisie manuelle" CSV (`;`, French decimals, UTF-8 BOM) |
| `POST` | `/api/import` | Upsert a JSON array of entries |
| `GET` | `/healthz` | Health check (no auth) |

### Importing entries from the former claude.ai page

`POST /api/import` accepts the documents of the old page's `entries`
collection as a JSON array, each with its `id`. Extra fields such as
`authorId` are ignored:

```sh
curl -u x:"$MULLER_PASSWORD" -H 'Content-Type: application/json' \
  --data @muller-import.json https://your-host/api/import
```

## Test

```sh
cd muller-app && go test -cover ./...
```
