# Putting Ligne Muller on a server

This runs the app and [Caddy](https://caddyserver.com/) (which provides HTTPS)
on one machine with Docker. It takes about 15 minutes.

## What you need

- A machine that stays on: a small Linux server, a mini PC, or a Windows PC.
  Any recent machine is enough; the app uses very little memory.
- It must be on the same network as the tablets (Wi-Fi on the line), with a
  **fixed IP address** (ask IT to reserve one, e.g. `192.168.1.50`).
- Docker: on Linux, `curl -fsSL https://get.docker.com | sh`; on Windows,
  install Docker Desktop.
- Ports 80 and 443 free on that machine.

## 1. Get the files

```sh
git clone https://github.com/mcarrascozabala-a11y/awesome-go.git
cd awesome-go/muller-app/deploy
cp .env.example .env
```

Edit `.env`:

| Setting | What to put |
| --- | --- |
| `MULLER_SITE` | The server's IP address (e.g. `192.168.1.50`) or network name. |
| `MULLER_HTTPS` | `internal` for the factory network only (usual case). `public` only if IT gives the server a public domain name reachable from the internet, with ports 80/443 open. |
| `MULLER_PASSWORD` | A long shared password, typed once on each tablet. |

## 2. Start it

```sh
docker compose up -d --build
```

It restarts by itself after a reboot or a crash. Check it with
`docker compose ps` and `docker compose logs`.

## 3. Prepare each tablet (once)

With `MULLER_HTTPS=internal`, the server signs its own certificate, so each
tablet must trust it once. Without this, the browser shows a warning and the
app cannot be installed or work offline.

1. On the tablet, open `http://<server address>/certificat` (plain `http`).
   This downloads `muller-ca.crt`.
2. Install it:
   - **iPad / iPhone**: Settings › *Profile Downloaded* › Install. Then
     Settings › General › About › Certificate Trust Settings › turn on
     *Caddy Local Authority*.
   - **Android**: Settings › Security › More security settings › Encryption &
     credentials › Install a certificate › CA certificate › pick
     `muller-ca.crt`. (Menu names vary by brand; search "certificate" in
     Settings.)
3. Open `https://<server address>`, enter any user name and the password from
   `.env` (tick "remember").
4. Install the app: **Safari** › Share › *Add to Home Screen*; **Chrome** ›
   ⋮ › *Install app*.

With `MULLER_HTTPS=public`, skip steps 1–2.

## 4. Bring over the existing entries

From any computer on the network, with the `muller-import.json` file:

```sh
curl -k -u x:'YOUR_PASSWORD' -H 'Content-Type: application/json' \
  --data @muller-import.json https://<server address>/api/import
```

It answers `{"imported":2,"rejected":{}}`.

## Backups

Everything is in the `muller-data` Docker volume. To save a copy (the app
pauses for a few seconds; tablets keep any entry made meanwhile and resend it):

```sh
docker compose stop app
docker compose cp app:/data/muller.db ./muller-$(date +%F).db
docker compose start app
```

Run it weekly (cron or Task Scheduler) and keep the copies on another machine.
The weekly CSV export from the app is also a readable copy of the data.

## Updating

```sh
git pull
docker compose up -d --build
```

Data and tablet certificates are kept.

## Troubleshooting

- **Tablet says "connection not private"**: the certificate from step 3 is not
  installed or not trusted (on iPad, the second switch is often missed).
- **Certificate changed**: only if the `caddy-data` volume was deleted
  (`docker compose down -v`). Avoid `-v`; if it happened, redo step 3.
- **Cannot reach the server**: check that the tablet is on the same network
  and that the server's firewall allows ports 80 and 443.
