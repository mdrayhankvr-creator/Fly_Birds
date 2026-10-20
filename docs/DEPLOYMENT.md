# Deployment Guide

This guide installs Fly_Birds for **one school** on a Linux server with
Docker, and covers upgrades and rollback. Repeat it per school: every school
gets its own Compose project, database, attachment storage, backups, secrets
and network. Nothing is shared between schools.

For day-to-day tasks after installation (first administrator, school setup,
password resets, backups and recovery) see [OPERATIONS.md](OPERATIONS.md).

> **No live payment or SMS integration.** Online bKash and Nagad payment is
> off; a disposable test installation can switch on a simulation that moves no
> money (`PAYMENT_SIMULATION_ALLOWED`, below). SMS uses a **mock** provider: no
> SMS reaches a phone. Tell the school before go-live, and do not advertise
> either as a live service.

## 1. What gets deployed

`deploy/compose.prod.yaml` defines four services:

| Service | Image | Role | Exposed |
|---|---|---|---|
| `db` | `postgres:17-alpine` | The school's database | Nothing (project network only) |
| `backend` | built from `backend/` | Go API; runs migrations at start; non-root user 10001 | Nothing |
| `web` | built from `frontend/` (production target) | nginx: the built app, security headers, `/api/` proxy, login rate limit | `127.0.0.1:${WEB_PORT}` only, plus a socket for Caddy |
| `caddy` | `caddy:2-alpine` (profile `edge`) | HTTPS with automatic Let's Encrypt certificates and HSTS | ports 80 and 443 |

Named volumes, prefixed with the project name: `postgres_data`,
`uploads_data`, `backups_data`, `caddy_data`, `caddy_config`, and
`edge_socket` (the socket between Caddy and `web`; it holds no data).

## 2. Server requirements

- A Linux server (Ubuntu 24.04 LTS or similar), 2 vCPU, 4 GB RAM and 40 GB
  disk are comfortable for one school of a few thousand students.
- Docker Engine 24 or later with the Compose plugin (`docker compose version`).
- A DNS name for the school whose A (and, if the server has IPv6, AAAA)
  record points at the server. See [School subdomains and DNS](#school-subdomains-and-dns).
- Ports 80 and 443 open to the internet (Caddy needs port 80 to obtain the
  certificate). Close every other inbound port except SSH.
- Git, to fetch a tagged release.
- Go 1.26 or later to run `school-provision` (section 3.2), or run it in the
  official Go container as shown there.

## 3. First installation

### 3.1 Fetch the release

```sh
sudo mkdir -p /opt/flybirds && sudo chown "$USER" /opt/flybirds
git clone https://github.com/mdrayhankvr-creator/Fly_Birds.git /opt/flybirds/app
cd /opt/flybirds/app
git checkout <release tag or commit>
```

### 3.2 Provision the school's configuration

`school-provision` writes the school's environment file, `deploy/<code>.env`,
from `deploy/school.env.example`. It generates the school's secrets and
checks that nothing collides with another school. It never overwrites a
file, never starts or removes containers and never prints a secret. Run it
from anywhere inside the repository:

```sh
cd /opt/flybirds/app/backend
go run ./cmd/school-provision create --code greenvalley \
  --domain greenvalley.flybirds.com --acme-email it@greenvalley.edu.bd --dry-run
go run ./cmd/school-provision create --code greenvalley \
  --domain greenvalley.flybirds.com --acme-email it@greenvalley.edu.bd
```

The dry run prints the plan and changes nothing:

```text
Dry run: planned provisioning
  School code        greenvalley (Compose project name)
  Domain             greenvalley.flybirds.com
  Environment file   /opt/flybirds/app/deploy/greenvalley.env (Git-ignored)
  TLS edge           on: Caddy serves https://greenvalley.flybirds.com on ports 80 and 443
  Local web port     127.0.0.1:8081
  Database password  48 random hex characters, would be generated and saved to the file; never shown
  Signing key        32 random bytes (base64), would be generated and saved to the file; never shown
  Docker resources the school gets on its first start (none are created now):
    Volumes          greenvalley_postgres_data, greenvalley_uploads_data, greenvalley_backups_data, greenvalley_caddy_data, greenvalley_caddy_config, greenvalley_edge_socket
    Network          greenvalley_default
    Containers       greenvalley-db-1, greenvalley-backend-1, greenvalley-web-1, greenvalley-caddy-1

Dry run: nothing was written and no container was started.
```

The real run prints the same plan, creates the file and then prints the
commands to start the school and create its first administrator. Every path
in those commands is quoted for both bash and PowerShell, so paths with
spaces (such as `C:\Users\Laptop Click\...`) work as printed. `go run
./cmd/school-provision list` shows the provisioned schools without secrets.

| Option | Meaning |
|---|---|
| `--code` | Required. The school code: 3–32 lower-case letters, digits and single hyphens, starting with a letter and not ending with a hyphen (`greenvalley`, `st-marys-2`). It becomes the Compose project name and prefixes every container, volume and network. It cannot contain `_`, so two schools' `<code>_` volume names never overlap. **Never change it after installation**: a new name means new, empty volumes. |
| `--domain` | Required. The school's host name under any parent domain (`greenvalley.flybirds.com`, `portal.school.edu.bd`). It is lower-cased. A scheme, port, path, wildcard, IP address, trailing dot, space or other unusual character is refused. `localhost` and `<name>.localhost` are accepted for local trials (section 9). |
| `--acme-email` | Mailbox for certificate expiry notices. Required for public domains; example and reserved domains are refused because Let's Encrypt rejects them. |
| `--web-port` | The school's local port (`127.0.0.1` only). By default the first port from 8081 that no other school uses and that is free on this machine. |
| `--no-edge` | Do not run the school's own Caddy. Use it for every school after the first on one server (section 4), or for a plain local trial. |
| `--dry-run` | Validate and print the plan only. Writes nothing, starts nothing, generates no secrets and needs no DNS. |
| `--skip-docker-check` | Do not ask Docker whether the project already has containers or volumes. Only when Docker is not running and you have checked by hand. |
| `--deploy-dir` | Another directory holding `compose.prod.yaml` and the template. Default: `deploy/` of the repository you run it in. |

What `create` checks and does:

- Refuses a code, domain or web port that another school file in `deploy/`
  already uses, and refuses if `deploy/<code>.env` exists.
- Refuses if Docker already has containers, volumes or networks labelled
  with the project name, so an old school's data is never picked up by a
  new configuration.
- Generates `POSTGRES_PASSWORD` (24 random bytes, hex) and
  `JWT_SIGNING_KEY_B64` (32 random bytes, base64) from the operating
  system's secure random source. Every school gets its own.
- Fills `COMPOSE_PROJECT_NAME`, `COMPOSE_PROFILES`, `SCHOOL_DOMAIN`,
  `ACME_EMAIL` and `WEB_PORT`; every other value keeps the template default.
  If the template ever gains a setting without a default, it stops instead
  of writing an incomplete file.
- Runs `docker compose config` with the new values before writing, and
  shows Docker's messages with secrets replaced by `[redacted]`.
- Writes the file in one step: a temporary file in `deploy/` is flushed and
  then linked to `deploy/<code>.env`, which fails rather than replace an
  existing file. An interruption leaves no file, or a hidden
  `.provision-<code>-*.env` if the process was killed outright; the next run
  for that code stops and points to it. On Linux the file is readable only by
  its owner (mode 600).

**Without Go on the server**, run the same command in the official Go
container (the backend build already uses this image). The container has no
Docker CLI, so the tool warns that it could not check Docker; check by hand
afterwards with `docker volume ls --filter label=com.docker.compose.project=<code>`
and `docker compose --env-file deploy/<code>.env -f deploy/compose.prod.yaml config --quiet`.
Its port check also runs inside the container, so pass `--web-port` with a
port you know is free on the server:

```sh
cd /opt/flybirds/app
docker run --rm --user "$(id -u):$(id -g)" -e HOME=/tmp -e GOCACHE=/tmp/go-cache \
  -e GOMODCACHE=/tmp/go-mod -v "$PWD":/src -w /src/backend golang:1.26.9-alpine \
  go run ./cmd/school-provision create --code greenvalley \
  --domain greenvalley.flybirds.com --acme-email it@greenvalley.edu.bd --web-port 8081
```

The file contains:

| Variable | Meaning |
|---|---|
| `COMPOSE_PROJECT_NAME` | The school code. Names every container, volume and network. **Never change it after installation.** |
| `COMPOSE_PROFILES` | `edge` to run Caddy; empty with `--no-edge`. |
| `SCHOOL_DOMAIN` | The school's public host name. |
| `ACME_EMAIL` | Address for certificate expiry notices. |
| `POSTGRES_USER`, `POSTGRES_DB` | Database user and name (template defaults). |
| `POSTGRES_PASSWORD` | Random, unique to this school. Used on the first start to create the database; to change it later, see [OPERATIONS.md, rotating secrets](OPERATIONS.md#rotating-secrets). |
| `JWT_SIGNING_KEY_B64` | Base64 of 32 random bytes, unique to this school. Changing it invalidates current access tokens; sessions continue (see rotating secrets). |
| `WEB_PORT` | Local port of the web container. Unique per school on the same server. |
| `BACKUP_RETENTION` | Completed backup archives to keep (1–365). |
| `BACKUP_INTERVAL` | Automatic backup interval, for example `24h`; `0` for manual only. |
| `PAYMENT_SIMULATION_ALLOWED` | `false` (the default, also when absent). `true` only for a disposable test installation: it lets an administrator switch on simulated online payment, which moves no money. See [OPERATIONS.md, online payment modes](OPERATIONS.md#online-payment-modes). |
| `NOTIFY_INTERVAL` | Notification dispatch interval (SMS is a mock). |
| `RELEASE_TAG` | Tag recorded on the built images, for example `v1.0.0`. Keep the previous value for rollback. |

You may edit the template-default values (backups, notifications, release
tag) afterwards. The `deploy/*.env` files are ignored by Git; only the
example is tracked. **The file is the only copy of the school's secrets.**
Keep a copy in the school's password manager and with its off-site backups:
without the database password the installation cannot be rebuilt from its
volumes.

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

Before the school starts using the installation, you can also run the
automated edge checks on the server. They start a few short-lived test
containers, send failed sign-ins for made-up accounts and confirm that the
login limit applies per visitor and cannot be dodged with forged headers
(section 6.1). They touch no school data, but they use up the login limit
for the server's own address for about a minute:

```sh
bash deploy/test-edge.sh deploy/greenvalley.env               # ends with "All edge checks passed"
```

Then continue with [OPERATIONS.md](OPERATIONS.md#1-first-administrator) to
create the first administrator and configure the school.

### 3.5 Provisioning errors

| Message | What to do |
|---|---|
| `school code ... may contain only ...` | Use 3–32 lower-case letters, digits and single hyphens, starting with a letter. |
| `school domain ... is not a valid host name` | Give only the host name: `greenvalley.flybirds.com`, not `https://greenvalley.flybirds.com/` or `greenvalley.flybirds.com:443`. |
| `--acme-email is required` / `uses a reserved domain` | Give a real mailbox that receives certificate notices. |
| `is already provisioned` / `already uses the domain` / `already assigned` | That school, domain or port exists. `list` shows them. The tool never replaces a file; choose another code, or keep the existing school. |
| `Docker already has resources for project` | Containers or volumes with that project name exist, possibly an earlier school's data. Inspect them with `docker volume ls --filter label=com.docker.compose.project=<code>`; choose another code. Never delete them to make room without a verified backup. |
| `could not check Docker` | Start Docker and run again. Use `--skip-docker-check` only after checking by hand. |
| `web port ... is in use on this machine` | Something already listens there. Omit `--web-port` to pick a free one. |
| `an earlier provisioning ... was interrupted and left ...` | A killed run left a hidden temporary file, which may contain secrets. Look at it, delete it, and run again. |
| `docker compose rejected the configuration` | The message (with secrets redacted) says what Compose refused; usually a changed `deploy/compose.prod.yaml` or template. Nothing was written. |

### 3.6 Limitations of the provisioning tool

- It prepares configuration only. It does not start schools, create DNS
  records, deploy to a server or create the first administrator (that stays
  an interactive `create-user` step, so no password is ever passed on a
  command line).
- It knows only the school files in its own `deploy/` directory. Codes,
  domains and ports are checked against those files and this machine's
  Docker; a school on another server is not seen.
- It needs Go 1.26, or the Go container shown above.
- On Windows the file is not given special permissions; it inherits the
  folder's access list (normally your own profile only).
- Secret rotation is manual (OPERATIONS.md, rotating secrets).

### Short form

Every `docker compose` command for a school repeats the same two options. A
shell alias keeps commands short and avoids touching the wrong school:

```sh
alias fb-greenvalley='docker compose --env-file /opt/flybirds/app/deploy/greenvalley.env -f /opt/flybirds/app/deploy/compose.prod.yaml'
fb-greenvalley ps
```

The rest of the documentation writes `fb` for this alias.

### School subdomains and DNS

Each school gets its own host name, for example `greenvalley.flybirds.com`,
and its own installation with `SCHOOL_DOMAIN=greenvalley.flybirds.com`.

- **One record per school:** an A record (and AAAA for IPv6) for
  `greenvalley.flybirds.com` pointing at the server that runs that school.
- **Or a wildcard:** one `*.flybirds.com` A/AAAA record pointing at a server
  that runs several schools. A wildcard matches any name, so a mistyped
  subdomain also reaches the server. Caddy serves only the school domains it
  is configured with: other names fail the TLS handshake, and plain-HTTP
  requests for them only get a redirect to HTTPS, where they fail too.
- Create the record **before** starting the stack and check it with
  `dig +short greenvalley.flybirds.com`. Caddy requests the certificate on
  the first start; while DNS does not yet point at the server the request
  fails and Caddy retries with increasing delays.
- Caddy obtains a separate certificate for each school name using the
  HTTP-01 challenge, so port 80 must be reachable from the internet. No
  wildcard certificate or DNS provider credentials are needed. Let's Encrypt
  allows about 50 new certificates per registered domain per week, ample for
  a pilot and gradual onboarding.
- If the domain has a CAA record, it must allow `letsencrypt.org`.
- HSTS is sent with `includeSubDomains` for the school's own host name. It
  does not affect `flybirds.com` or other schools' names. Do not add
  `flybirds.com` to the browser HSTS preload list unless every name under it
  will always be served over HTTPS.

## 4. Several schools on one server

Provision each school separately; each gets its own environment file,
Compose project, database, volumes, network, secrets and `WEB_PORT`.
Only one project can own ports 80 and 443, so with more than one school
on one server, provision the others with `--no-edge` and use the
shared-proxy layout below:

```sh
go run ./cmd/school-provision create --code riverside \
  --domain riverside.flybirds.com --acme-email it@riverside.edu.bd --no-edge
go run ./cmd/school-provision list
```

The tool warns when a school with the edge profile already exists in
`deploy/`. Two schools never share data: separate databases on separate
Docker networks (one school's backend cannot reach another's database),
separate signing keys and sessions, and cookies scoped to each school's own
host name. A school on its own server needs no `--no-edge`.

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

> **Login rate limit in this layout.** A shared proxy reaches each school
> over the `WEB_PORT`, a TCP connection from the host. The web container
> does not trust forwarded addresses on that port (section 6.1), so it
> counts every visitor of that school as one client: 30 sign-in attempts per
> minute for the whole school. That is safe against forged headers, but busy
> mornings can see "Too many requests" at sign-in. Until the shared layout
> passes the real client address (planned), run **one school per server
> with the `edge` profile** for pilots.

## 6. Security settings in this stack

- HTTPS only, HSTS for one year, HTTP redirected to HTTPS.
- The database and API are not reachable from outside the project network.
- Strict Content-Security-Policy, `X-Frame-Options: DENY`, `nosniff`,
  `Referrer-Policy`, `Permissions-Policy` (camera allowed only for QR
  attendance); nginx and Caddy hide their versions.
- Login limited to 30 requests per minute per visitor address (burst 30),
  plus account lockout in the API after repeated failures. See 6.1 for how
  the visitor address is determined.
- Uploads limited to 12 MiB at the web server (10 MiB per attachment in the
  API).
- Backend container runs as an unprivileged user; refresh and CSRF tokens are
  stored only as hashes.

### 6.1 Proxy trust and client addresses

The login rate limit in the web container (nginx) needs the visitor's real
address. With the `edge` profile:

1. Caddy terminates HTTPS. It trusts no upstream proxies, so it **discards**
   any `X-Forwarded-For` the visitor sends and sets it to the visitor's real
   address.
2. Caddy forwards to nginx through a Unix socket on the `edge_socket`
   volume, which only the `caddy` and `web` containers mount (Caddy
   read-only).
3. nginx accepts the client address from `X-Forwarded-For` **only on that
   socket** (`set_real_ip_from unix:`). Every TCP connection, whether on the
   host's `127.0.0.1:${WEB_PORT}` or from another container to `web:80`, is
   counted by its own connecting address, and forwarding headers on it are
   ignored.

Trust is therefore tied to the socket, not to an IP range, so another
container on the project network cannot claim to be a proxy. Consequences:

- Visitors are limited individually; one visitor (or attacker) using up the
  limit does not block anyone else.
- Forged `X-Forwarded-For`, `X-Real-IP` or `Forwarded` headers neither lift a
  visitor's limit nor push it onto someone else's address.
- The nginx access log records the real visitor address.
- Many users behind one school network (NAT) share one public address and
  so share one limit of 30 attempts per minute (burst 30).
- The API itself does not use client addresses (account lockout is per
  account), so it needs no proxy configuration.

`deploy/test-edge.sh` verifies all of this on a running stack and runs in CI.
If `web` is ever stopped uncleanly, a leftover socket file is removed when it
starts again.

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

**Upgrading past migration 1401 (payment safety).** Online payment is off
afterwards, students lose `fees:pay_online`, and online payments made with the
old simulator are labelled simulated and left out of collections; nothing is
deleted. An administrator then reviews them (see
[OPERATIONS.md](OPERATIONS.md#simulated-payments-recorded-before-the-payment-safety-upgrade)).
Existing environment files need no change.

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

### Local trial with two schools

Provisioned schools run locally with the production Compose file and
their own disposable volumes; nothing touches the development stack
(`compose.yaml`, project `fly_birds`). Names under `.localhost` resolve to
this machine in browsers and need no DNS or `--acme-email`:

```powershell
cd 'C:\Users\Laptop Click\Fly_Birds\backend'
go run ./cmd/school-provision create --code trial-a --domain trial-a.localhost
go run ./cmd/school-provision create --code trial-b --domain trial-b.localhost --no-edge
```

Start each with the `up -d --build --wait` command the tool printed.
`trial-a` is served by its Caddy at `https://trial-a.localhost` (ports 80
and 443 must be free); `trial-b` at `http://localhost:<its WEB_PORT>`.
Stop them with the same command and `stop` instead of `up ...`; that keeps
their volumes. They are trial schools: do not enter real data.

Use `SCHOOL_DOMAIN=localhost` for a local trial; Caddy then issues a
certificate from its own local authority, which browsers warn about. Run
`deploy/test-edge.sh` from Git Bash with a Windows-style path to the
environment file. The browser tests in `e2e/` accept that certificate with
`E2E_INSECURE_TLS=1` and `E2E_BASE_URL=https://localhost`.
