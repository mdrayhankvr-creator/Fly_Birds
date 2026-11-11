# Hosting Guide

How to host Fly_Birds for one school on a Linux server: from choosing the server to a working, backed-up installation that you can update and keep tidy. It is the short path. [DEPLOYMENT.md](DEPLOYMENT.md) and [OPERATIONS.md](OPERATIONS.md) have the full detail and the less common cases (several schools on one server, a shared proxy, rotating secrets, rollback).

Every command here was run against this release. Replace `greenvalley` and `school.greenvalley.edu.bd` with the school's own code and domain.

## 1. Server size

The application itself is light. Measured on a demo school (40 students, all modules on), the three running containers used about 70 MB of memory at rest, and the database was 16 MB. Making result PDFs takes more for a moment: the largest, one PDF with the marksheets of 60 students, peaked under 100 MB, and at most two PDFs are made at a time. What needs room is building the images on the server, the backups you keep, and photos.

| School | CPU | Memory | Disk (SSD) |
|---|---|---|---|
| Up to about 1,000 students | 2 vCPU | 2 GB, plus 2 GB swap | 30 GB |
| **Typical school, 1,000–3,000 students (recommended)** | 2 vCPU | 4 GB | 40 GB |
| Larger, or several schools on one server | 4 vCPU | 8 GB | 80 GB or more |

Where the disk goes:

- **Images:** about 900 MB for the backend, web, PostgreSQL and Caddy images. The backend image includes the PDF renderer and its fonts. Each update keeps the previous release's images for rollback.
- **Build cache:** building the images on the server leaves about 2 GB of Docker build cache each time. Section 10 clears it monthly.
- **Backups:** every archive holds the whole database and every uploaded file. With `BACKUP_RETENTION=14`, plan for 14 times the size of one archive. After the first week, check the actual size under **Admin → Backups** and adjust.
- **Photos** make up most of the growth: student and staff photos, the landing slideshow (up to 8) and the public page gallery (up to 48). The server resizes each one.
- **Result PDFs** are kept with the uploads (so they are in the backups) and reused until the results change. A correction makes new ones and deletes the old files.

Keep the disk below 80% full. Use Ubuntu 24.04 LTS (or similar) with Docker Engine 24 or later and the Compose plugin (`docker compose version`).

## 2. Domain and HTTPS

1. Choose the school's address, for example `school.greenvalley.edu.bd`.
2. Create an **A record** (and **AAAA** if the server has IPv6) pointing at the server. Check it before going further:

   ```sh
   dig +short school.greenvalley.edu.bd
   ```

3. Open ports **80 and 443** to the internet, and close every other inbound port except SSH.
4. If the domain has a CAA record, it must allow `letsencrypt.org`.

HTTPS is automatic. The stack includes Caddy (the `edge` profile), which gets a Let's Encrypt certificate on the first start and renews it. Port 80 must stay open for that. Sign-in works only over HTTPS: the session cookies are marked secure.

## 3. Get the release

```sh
sudo mkdir -p /opt/flybirds && sudo chown "$USER" /opt/flybirds
git clone https://github.com/mdrayhankvr-creator/Fly_Birds.git /opt/flybirds/app
cd /opt/flybirds/app
git checkout <release tag or commit>
```

## 4. The environment file

Each school has one private file, `deploy/<code>.env`. The easiest way to make it is `school-provision`, which fills in the school's values and generates its secrets without printing them ([DEPLOYMENT.md §3.2](DEPLOYMENT.md#32-provision-the-schools-configuration)):

```sh
cd /opt/flybirds/app/backend
go run ./cmd/school-provision create --code greenvalley \
  --domain school.greenvalley.edu.bd --acme-email it@greenvalley.edu.bd --dry-run      # shows the plan only
go run ./cmd/school-provision create --code greenvalley \
  --domain school.greenvalley.edu.bd --acme-email it@greenvalley.edu.bd
```

To write it by hand instead, copy `deploy/school.env.example` to `deploy/greenvalley.env` and fill it in. Every value in angle brackets below is a placeholder:

```sh
COMPOSE_PROJECT_NAME=greenvalley          # school code; never change it after the first start
COMPOSE_PROFILES=edge                     # run Caddy for HTTPS
SCHOOL_DOMAIN=school.greenvalley.edu.bd
PUBLIC_BASE_URL=https://school.greenvalley.edu.bd   # printed in the QR code on every marksheet
ACME_EMAIL=it@greenvalley.edu.bd            # receives certificate notices
POSTGRES_USER=flybirds
POSTGRES_PASSWORD=<output of: openssl rand -hex 24>
POSTGRES_DB=flybirds
JWT_SIGNING_KEY_B64=<output of: openssl rand -base64 32>
WEB_PORT=8081
BACKUP_RETENTION=14
BACKUP_INTERVAL=24h
PAYMENT_SIMULATION_ALLOWED=false          # always false for a real school
NOTIFY_INTERVAL=1m
RELEASE_TAG=v1.0.0
```

- **Keep the file private.** Git ignores `deploy/*.env`; never commit it, send it in chat or share it between schools. On Linux, `chmod 600 deploy/greenvalley.env`.
- **Keep a copy off the server**, in the school's password manager. It is the only copy of the database password and signing key, and backup archives do not contain it.
- **`PUBLIC_BASE_URL`** is the address printed in the verification QR code on every result marksheet. Scanning it opens `<PUBLIC_BASE_URL>/verify/result/<code>`, which anyone can open without signing in.
  - When the line is missing, it is `https://` plus `SCHOOL_DOMAIN`, which is right for a school on its own domain. `school-provision` does not write the line.
  - Set it only when the school is reached at another address or port, for example behind a shared proxy. Use `https://`, with no path, query or trailing slash; the backend refuses to start with anything that isn't an `http(s)://` address.
  - **Get it right before the first results are published.** Marksheets already printed keep the address they were made with, so a wrong value means QR codes that lead nowhere. An `http://localhost:…` value is for local trials only.
  - After changing it, restart the backend: `fb up -d backend`.
- **No live payment or SMS.** Online bKash and Nagad payment is off, and SMS goes to a mock provider that never reaches a phone. Tell the school before go-live.

The rest of this guide uses an alias so that every command goes to the right school:

```sh
alias fb='docker compose --env-file /opt/flybirds/app/deploy/greenvalley.env -f /opt/flybirds/app/deploy/compose.prod.yaml'
```

## 5. First start

```sh
cd /opt/flybirds/app
fb up -d --build --wait
fb ps
```

`--wait` returns once the database, backend and web server are healthy. The backend creates every table on its first start. Then check it from outside:

```sh
curl -fsS https://school.greenvalley.edu.bd/api/readyz        # {"data":{"status":"ready"}}
curl -sI http://school.greenvalley.edu.bd/ | head -1           # a redirect to HTTPS
```

If the HTTPS check fails, `fb logs caddy` usually shows why: DNS not pointing at the server yet, or port 80 closed.

## 6. The first administrator

There is no default account or password. Someone at the server creates the first administrator. The command asks for the password twice without showing it (12 to 72 characters):

```sh
fb exec backend /app/create-user -email head.office@greenvalley.edu.bd -role admin -name "Office Administrator"
```

`-name` is the name shown in the app (optional; without it the account shows only its email). It needs an interactive terminal, so run it over SSH, not from a script. Then sign in at `https://school.greenvalley.edu.bd/`, where the sign-in form is on the school's home page. Create every other account in the app (**Admin → Users**, student logins from **Students**), and follow the administrator chapter of the [Bangla user guide](USER_GUIDE_BN.md) for the school's setup.

**Never run `seed-demo` on a real school:** it adds demo accounts with a shared, known password.

## 7. Daily backups with an encrypted off-server copy

### 7.1 On the server

With `BACKUP_INTERVAL=24h` the application makes a backup every 24 hours, counted from when the backend started. A restart starts the count again, so a server that restarts every night may never reach 24 hours: the script below warns when the newest archive is too old. An administrator can also take one at any time in **Admin → Backups → Back up now**, and **Verify** checks an archive against its checksums. Archives are kept in the `backups_data` volume; the newest `BACKUP_RETENTION` are kept and older ones are deleted.

### 7.2 Off the server, every day

A backup that lives only on the server is lost with the server. This script copies the newest archive off the server once a day. It checks the copy against the server's checksum, encrypts it with a passphrase, sends it to another machine, and keeps 30 days of copies locally.

1. Create a passphrase file that only root can read. Store the same passphrase in the school's password manager: without it, the copies cannot be opened.

   ```sh
   sudo sh -c 'umask 077; openssl rand -base64 32 > /root/.flybirds-backup-passphrase'
   ```

2. Set up SSH key access from this server to a backup machine (here `backup@backup.example.net`), and create the folder there.
3. Save the script as `/opt/flybirds/offsite-backup.sh` and make it executable (`chmod 700`):

   ```sh
   #!/bin/sh
   # Copies the newest Fly_Birds backup off the server, encrypted.
   set -eu
   SCHOOL=greenvalley
   APP=/opt/flybirds/app
   OUT=/var/backups/flybirds/$SCHOOL
   PASSFILE=/root/.flybirds-backup-passphrase
   REMOTE=backup@backup.example.net:flybirds/$SCHOOL/
   fb() { docker compose --env-file "$APP/deploy/$SCHOOL.env" -f "$APP/deploy/compose.prod.yaml" "$@"; }

   latest=$(fb exec -T backend sh -c 'ls -1t /app/backups/flybirds-backup-*.tar.gz | head -n 1')
   name=$(basename "$latest")
   age=$(fb exec -T backend sh -c "echo \$(( \$(date +%s) - \$(stat -c %Y '$latest') ))")
   if [ "$age" -gt 93600 ]; then echo "WARNING: newest backup $name is older than 26 hours" >&2; fi

   mkdir -p "$OUT"
   if [ ! -f "$OUT/$name.gpg" ]; then
     want=$(fb exec -T backend sha256sum "$latest" | cut -d' ' -f1)
     fb cp "backend:$latest" "$OUT/$name"
     have=$(sha256sum "$OUT/$name" | cut -d' ' -f1)
     if [ "$want" != "$have" ]; then echo "checksum mismatch for $name" >&2; rm -f "$OUT/$name"; exit 1; fi
     gpg --batch --yes --pinentry-mode loopback --passphrase-file "$PASSFILE" \
       --symmetric --cipher-algo AES256 -o "$OUT/$name.gpg" "$OUT/$name"
     rm -f "$OUT/$name"
   fi
   rsync -a "$OUT/$name.gpg" "$REMOTE"
   find "$OUT" -name 'flybirds-backup-*.gpg' -mtime +30 -delete
   ```

4. Run it once by hand (`sudo /opt/flybirds/offsite-backup.sh`) and check that the `.gpg` file arrived on the backup machine. Then schedule it daily in root's crontab (`sudo crontab -e`):

   ```cron
   30 3 * * * /opt/flybirds/offsite-backup.sh >> /var/log/flybirds-offsite.log 2>&1
   ```

To open a copy later:

```sh
gpg --batch --pinentry-mode loopback --passphrase-file /root/.flybirds-backup-passphrase \
  -d flybirds-backup-<time>.tar.gz.gpg > flybirds-backup-<time>.tar.gz
```

Archives contain personal data about children and staff, and password hashes. Keep every copy encrypted, limit who can read the backup machine, and delete copies past the school's retention period. Keep the environment file (section 4) somewhere separate from the archives.

## 8. Restore drill (once a term)

A drill proves that a backup can really be restored. It restores into a temporary database next to the live one and never touches the live data. Pick an archive name from `fb exec backend ls /app/backups`:

```sh
A=/app/backups/flybirds-backup-<time>.tar.gz
fb exec -T db sh -c 'createdb -U "$POSTGRES_USER" restore_drill'
fb exec -T backend sh -c "DATABASE_URL=\"\$(echo \"\$DATABASE_URL\" | sed 's#/[^/?]*?#/restore_drill?#')\" /app/restore-backup -archive $A -uploads-dir /tmp/restore-drill -confirm"
```

The second command points `restore-backup` at `restore_drill` instead of the live database, without typing the password, and ends with a line such as `restored 85 tables, 3410 rows and 0 attachments`. Compare a few counts between the two databases. The two lines should match, apart from anything recorded since the backup:

```sh
for d in flybirds restore_drill; do
  fb exec -T db sh -c "psql -U \"\$POSTGRES_USER\" -d $d -Atc \"select (select count(*) from users), (select count(*) from students), (select count(*) from fee_payments), (select count(*) from student_attendance)\""
done
```

(`flybirds` is the `POSTGRES_DB` value.) Then remove the drill copy:

```sh
fb exec -T db sh -c 'dropdb -U "$POSTGRES_USER" restore_drill'
fb exec -T backend rm -rf /tmp/restore-drill
```

To drill an off-server copy, decrypt it, copy it in with `fb cp flybirds-backup-<time>.tar.gz backend:/app/backups/`, and run the same commands. Rebuilding a whole school on a new server is in [OPERATIONS.md §6.2](OPERATIONS.md#62-recovering-on-a-new-server).

## 9. Updates

Updates only move forward: a new release may add database changes, which the backend applies when it starts.

1. Read the release notes, and compare `deploy/school.env.example` with the school's file for new settings.
2. Take a backup (**Admin → Backups → Back up now**, then **Verify**) and run the off-server script.
3. Note the running version: `git rev-parse HEAD` and the current `RELEASE_TAG`.
4. Build and start the new release with a new `RELEASE_TAG`, so the old images stay for rollback:

   ```sh
   cd /opt/flybirds/app
   git fetch --tags && git checkout <new release>
   sed -i 's/^RELEASE_TAG=.*/RELEASE_TAG=v1.1.0/' deploy/greenvalley.env
   fb up -d --build --wait
   ```

5. Check `https://school.greenvalley.edu.bd/api/readyz`, sign in, and work through the release checklist in [OPERATIONS.md §8](OPERATIONS.md#8-release-validation).

If something is wrong, roll back as in [DEPLOYMENT.md §8](DEPLOYMENT.md#8-rolling-back).

**Updating to the release with the lifecycle audit fixes** (migrations 1801–1803) needs no new setting, but it changes daily work, so tell the school before you update:

- **Attendance on Fridays stops.** Every school starts with Friday as its weekly off day. New attendance is also refused on published holidays. If the school works on Fridays or is also closed on Saturdays, the administrator changes this under **Admin → School settings → Weekly off days** straight after the update.
- **Late fines are off.** Existing fee structures get a late fine of 0. The backend checks for late fines when it starts and every 30 minutes, and logs `late fines charged` when it adds any.
- **Leaving cancels later dues.** From now on, withdrawing or transferring a student cancels their unpaid monthly dues after the month they leave. Students who left before the update keep their dues as they were.

The Bangla user guide covers these screens ([USER_GUIDE_BN.md](USER_GUIDE_BN.md), sections ১.৭, ৪.৪ and ৪.৫).

## 10. Monthly Docker disk cleanup

Builds and updates leave unused cache and old images behind. Once a month, after checking that backups are current:

```sh
docker system df                          # what is using space
docker builder prune -f                   # build cache (rebuilt when needed)
docker image prune -f                     # untagged leftovers only
docker image ls 'greenvalley-*'           # this school's release images
docker image rm greenvalley-backend:<old tag> greenvalley-web:<old tag>   # keep the running and the previous tag
df -h /
```

To run the first two automatically, add this to root's crontab (first day of the month, 04:30):

```cron
30 4 1 * * docker builder prune -f && docker image prune -f
```

**Never** run `docker volume prune`, `docker system prune --volumes`, `docker compose down -v` or `docker volume rm` on a school's server. The volumes hold the school's database, uploads and backups. If the disk is still full, lower `BACKUP_RETENTION`, move old archives off the server and see the troubleshooting table in [OPERATIONS.md §7](OPERATIONS.md#7-troubleshooting).

## 11. Checklist before go-live

- [ ] The server matches section 1, DNS points at it, and only ports 22, 80 and 443 are open.
- [ ] `https://<domain>/api/readyz` answers `ready`, and HTTP redirects to HTTPS.
- [ ] The environment file is private on the server (`chmod 600`) and saved off it.
- [ ] One administrator was created with `create-user`; no demo accounts exist.
- [ ] `PAYMENT_SIMULATION_ALLOWED=false`.
- [ ] `BACKUP_INTERVAL=24h`, the off-server script runs daily, and its copy can be decrypted.
- [ ] A restore drill has passed.
- [ ] `PUBLIC_BASE_URL` (or `SCHOOL_DOMAIN`) is the school's real address: after the first results are published, download one marksheet and scan its QR code with a phone on mobile data. It should open the **Result check** page.
- [ ] The weekly off days in **Admin → School settings** match the school (Friday by default), and the year's holidays are published in the **Events calendar**, before teachers take attendance.
- [ ] The monthly cleanup is scheduled.
- [ ] The school knows SMS is simulated and online payment is off.
