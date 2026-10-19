# Deployment Guide

This guide installs Fly_Birds for **one school** on a Linux server with
Docker, and covers upgrades and rollback. Repeat it per school: every school
gets its own Compose project, database, attachment storage, backups, secrets
and network. Nothing is shared between schools.

For day-to-day tasks after installation (first administrator, school setup,
password resets, backups and recovery) see [OPERATIONS.md](OPERATIONS.md).

> **Simulated integrations.** Online bKash and Nagad payment and SMS
> notifications use **mock** providers. They record what would happen but no
> money moves and no SMS reaches a phone. Tell the school before go-live, and
> do not advertise either as a live service.

## 1. What gets deployed

`deploy/compose.prod.yaml` defines four services:

| Service | Image | Role | Exposed |
|---|---|---|---|
| `db` | `postgres:17-alpine` | The school's database | Nothing (project network only) |
| `backend` | built from `backend/` | Go API; runs migrations at start; non-root user 10001 | Nothing |
| `web` | built from `frontend/` (production target) | nginx: the built app, security headers, `/api/` proxy, login rate limit | `127.0.0.1:${WEB_PORT}` only |
| `caddy` | `caddy:2-alpine` (profile `edge`) | HTTPS with automatic Let's Encrypt certificates and HSTS | ports 80 and 443 |

Named volumes, prefixed with the project name: `postgres_data`,
`uploads_data`, `backups_data`, `caddy_data`, `caddy_config`.

## 2. Server requirements

- A Linux server (Ubuntu 24.04 LTS or similar), 2 vCPU, 4 GB RAM and 40 GB
  disk are comfortable for one school of a few thousand students.
- Docker Engine 24 or later with the Compose plugin (`docker compose version`).
- A DNS name for the school (for example `school.example.edu.bd`) whose A or
  AAAA record points at the server.
- Ports 80 and 443 open to the internet (Caddy needs port 80 to obtain the
  certificate). Close every other inbound port except SSH.
- Git, to fetch a tagged release.

## 3. First installation

### 3.1 Fetch the release

```sh
sudo mkdir -p /opt/flybirds && sudo chown "$USER" /opt/flybirds
git clone https://github.com/mdrayhankvr-creator/Fly_Birds.git /opt/flybirds/app
cd /opt/flybirds/app
git checkout <release tag or commit>
```

### 3.2 Create the school's environment file

```sh
cp deploy/school.env.example deploy/greenvalley.env
chmod 600 deploy/greenvalley.env
openssl rand -hex 24      # paste as POSTGRES_PASSWORD
openssl rand -base64 32   # paste as JWT_SIGNING_KEY_B64
```

Edit `deploy/greenvalley.env` and fill in every value:

| Variable | Meaning |
|---|---|
| `COMPOSE_PROJECT_NAME` | Short unique school code (lower-case letters, digits, `-`). Names every container, volume and network. **Never change it after installation**: a new name means new, empty volumes. |
| `COMPOSE_PROFILES` | `edge` to run Caddy. Leave empty when a shared reverse proxy terminates TLS (section 5). |
| `SCHOOL_DOMAIN` | The school's public host name. |
| `ACME_EMAIL` | Address for certificate expiry notices. |
| `POSTGRES_USER`, `POSTGRES_DB` | Database user and name (the defaults are fine). |
| `POSTGRES_PASSWORD` | Random, unique to this school. Used only on the first start to create the database. |
| `JWT_SIGNING_KEY_B64` | Base64 of 32 random bytes, unique to this school. Changing it signs everyone out. |
| `WEB_PORT` | Local port of the web container. Unique per school on the same server. |
| `BACKUP_RETENTION` | Completed backup archives to keep (1–365). |
| `BACKUP_INTERVAL` | Automatic backup interval, for example `24h`; `0` for manual only. |
| `NOTIFY_INTERVAL` | Notification dispatch interval (SMS is a mock). |
| `RELEASE_TAG` | Tag recorded on the built images, for example `v1.0.0`. Keep the previous value for rollback. |

The `deploy/*.env` files are ignored by Git; only the example is tracked.
Keep a copy of the file in the school's password manager: without the
database password and signing key the installation cannot be rebuilt as is.

### 3.3 Build and start

```sh
docker compose --env-file deploy/greenvalley.env -f deploy/compose.prod.yaml up -d --build --wait
docker compose --env-file deploy/greenvalley.env -f deploy/compose.prod.yaml ps
```

`--wait` returns once the database, backend and web server are healthy. The
backend applies all database migrations on its first start.

### 3.4 Check it

```sh
curl -fsS https://school.example.edu.bd/api/readyz        # {"data":{"status":"ready"}}
curl -sI https://school.example.edu.bd/ | grep -i -E 'strict-transport|content-security'
curl -sI http://school.example.edu.bd/ | head -1            # 308 redirect to HTTPS
```

Then continue with [OPERATIONS.md](OPERATIONS.md#1-first-administrator) to
create the first administrator and configure the school.

### Short form

Every `docker compose` command for a school repeats the same two options. A
shell alias keeps commands short and avoids touching the wrong school:

```sh
alias fb-greenvalley='docker compose --env-file /opt/flybirds/app/deploy/greenvalley.env -f /opt/flybirds/app/deploy/compose.prod.yaml'
fb-greenvalley ps
```

The rest of the documentation writes `fb` for this alias.

## 4. Several schools on one server

Use one environment file per school, each with its own
`COMPOSE_PROJECT_NAME`, `WEB_PORT`, database password and signing key. Only
one project can own ports 80 and 443, so with more than one school use the
shared-proxy layout below instead of each school's `edge` profile.

## 5. Behind a shared reverse proxy

Set `COMPOSE_PROFILES=` (empty) in each school's file so no per-school Caddy
starts. Run one reverse proxy on the host that terminates TLS for every
school and forwards to `127.0.0.1:<WEB_PORT>`. With Caddy on the host:

```caddyfile
school-a.example.edu.bd {
	header Strict-Transport-Security "max-age=31536000; includeSubDomains"
	reverse_proxy 127.0.0.1:8081
}
school-b.example.edu.bd {
	header Strict-Transport-Security "max-age=31536000; includeSubDomains"
	reverse_proxy 127.0.0.1:8082
}
```

The proxy must use HTTPS: the backend sets `Secure` cookies
(`AUTH_COOKIE_SECURE=true`), so sign-in does not work over plain HTTP.
Login rate limiting is applied by each school's web container per client
address; if your proxy hides the client address, all users share one limit.

## 6. Security settings in this stack

- HTTPS only, HSTS for one year, HTTP redirected to HTTPS.
- The database and API are not reachable from outside the project network.
- Strict Content-Security-Policy, `X-Frame-Options: DENY`, `nosniff`,
  `Referrer-Policy`, `Permissions-Policy` (camera allowed only for QR
  attendance); nginx and Caddy hide their versions.
- Login limited to 30 requests per minute per client address (burst 30),
  plus account lockout in the API after repeated failures.
- Uploads limited to 12 MiB at the web server (10 MiB per attachment in the
  API).
- Backend container runs as an unprivileged user; refresh and CSRF tokens are
  stored only as hashes.

## 7. Upgrading

Upgrades are forward-only: a new release may add database migrations, which
the backend applies when it starts.

1. **Read the release notes** and check for new environment variables in
   `deploy/school.env.example`.
2. **Back up** in the app (Admin → Backups → Create backup, then Verify) and
   copy the archive off the server (see
   [OPERATIONS.md](OPERATIONS.md#5-backups)). Also keep a database dump:

   ```sh
   fb exec -T db sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > greenvalley-pre-upgrade.dump
   ```

3. **Note the running version:** `git rev-parse HEAD` and the current
   `RELEASE_TAG`.
4. **Fetch and build the new release**, with a new `RELEASE_TAG` so the old
   images stay available for rollback:

   ```sh
   git fetch --tags && git checkout <new release>
   sed -i 's/^RELEASE_TAG=.*/RELEASE_TAG=v1.1.0/' deploy/greenvalley.env
   fb up -d --build --wait
   ```

5. **Validate** with the release checklist in
   [OPERATIONS.md](OPERATIONS.md#8-release-validation).

Do not use `docker compose down -v`, `docker volume rm`, `docker system prune
--volumes` or `docker compose up --force-recreate` with a changed project
name: each of these can delete or orphan the school's data.

## 8. Rolling back

**If the new release added no migrations** (the release notes say so; you
can compare `SELECT max(version) FROM schema_migrations` before and after),
switch back to the previous images:

```sh
git checkout <previous release>
sed -i 's/^RELEASE_TAG=.*/RELEASE_TAG=v1.0.0/' deploy/greenvalley.env
fb up -d --wait          # the old images are still tagged; no rebuild needed
```

**If the new release applied migrations**, the old code is not guaranteed to
work with the newer schema. Restore the pre-upgrade backup with the previous
release, as in [OPERATIONS.md, disaster recovery](OPERATIONS.md#6-restoring-and-disaster-recovery):
start the previous release against a **new, empty** project or database, then
restore the archive into it. Changes made after the backup are lost, so roll
back quickly or not at all.

## 9. Windows hosts (testing only)

The stack also runs on Docker Desktop for Windows for evaluation. In Git
Bash, prefix commands with `MSYS_NO_PATHCONV=1` and pass Windows-style paths
to `--env-file`, otherwise Git Bash rewrites `/app/...` and file paths:

```sh
MSYS_NO_PATHCONV=1 docker compose --env-file 'C:\flybirds\deploy\qa.env' -f deploy/compose.prod.yaml exec backend /app/create-user -email you@example.com
```

Use `SCHOOL_DOMAIN=localhost` for a local trial; Caddy then issues a locally
trusted certificate that browsers will warn about.
