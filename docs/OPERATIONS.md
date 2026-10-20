# Operations Guide

Day-to-day running of one school's Fly_Birds installation, after
[DEPLOYMENT.md](DEPLOYMENT.md). Commands use the per-school alias from the
deployment guide:

```sh
alias fb='docker compose --env-file /opt/flybirds/app/deploy/greenvalley.env -f /opt/flybirds/app/deploy/compose.prod.yaml'
```

Commands that ask for a password (`create-user`, `reset-password`) need an
interactive terminal: run them over SSH, not from a script.

## What is real and what is simulated

| Feature | Status |
|---|---|
| Accounts, attendance, exams, results, fees records, receipts, ledger, notices, diary, in-app notifications, analytics, backups | **Live**: real records in the school's database |
| Online bKash and Nagad payment | **Simulated** by a mock provider. No money moves. A real payment gateway contract and integration are needed before parents can pay online. |
| SMS notifications | **Simulated** by a mock provider. Messages are queued and marked sent but never reach a phone. In-app notifications are real. |

Until a real gateway exists, switch off online payment by removing the
`fees:pay_online` permission from the Student and Guardian roles in
**Admin → Permissions**, and tell staff that SMS is not delivered.

## 1. First administrator

Right after the first start, create the school's administrator. The command
asks for the password twice without echoing it (12 to 72 characters):

```sh
fb exec backend /app/create-user -email head.office@school.example.edu.bd -role admin
```

A school prepared with `school-provision` prints this command with its own
environment file filled in. No default administrator or password exists:
the first account is always created this way, by someone at the server.
`create-user` refuses to run without an interactive terminal, so the
password is never passed on a command line or stored in shell history. In
Git Bash on Windows, prefix the command with `MSYS_NO_PATHCONV=1`.

Sign in at `https://<SCHOOL_DOMAIN>/login` with that account. Create every
other account from **Admin → Users** in the app, not from the command line.
`create-user` also accepts `-role head_teacher|teacher|accountant|student|guardian`.

Never run `seed-demo` on a real school's database. It creates demo accounts
with a shared, known password.

## 2. School settings and modules

As the administrator:

1. **Admin → School settings:** school name in English and Bangla, address,
   phone, email, logo URL (HTTPS), primary and secondary colours (the server
   refuses colours with less than 4.5:1 contrast on white), preferred
   language and grading scheme.
2. **Admin → Modules:** switch on the modules the school uses. Dependencies
   are enforced, for example Attendance needs Students and Staff, and
   Routines need Academics and Staff. Authentication, Permissions and School
   settings are always on.
3. **Admin → Permissions:** review the role permission matrix. The defaults
   suit most schools; every change is audited.

## 3. Initial academic setup

In this order, because later steps depend on earlier ones:

1. **Academic sessions:** create the session (for example "2027", 1 January
   to 31 December) and activate it. Only one session is active at a time.
2. **Classes**, then **Subjects**, then map subjects to each class (mark the
   fourth subject where it applies).
3. **Sections** for each class in the active session.
4. **Teachers and staff:** create staff profiles, link each to a teacher
   account, and assign session, class and subject.
5. **Students and guardians:** add students one by one or import a CSV, then
   link guardians. Create student and guardian accounts if they will sign in.
6. **Routines**, **fee categories and structures**, **exams** and
   **analytics settings** as the school starts using those modules.

## 4. Accounts and passwords

- A user who forgets their password, or is locked out (5 failed attempts lock
  the account for 15 minutes), contacts the school office. The login page
  says so.
- There is **no self-service reset or in-app password change yet**. After
  confirming the person's identity, the operator runs:

  ```sh
  fb exec backend /app/reset-password -email teacher@school.example.edu.bd
  ```

  It sets the new password, clears the lockout, signs the user out on every
  device and writes a `user.password_reset` entry to the audit log. Give the
  new password to the person through a private channel.
- Deactivate staff who leave in **Admin → Users**; deactivation ends their
  sessions.

## 5. Backups

A backup is a `flybirds-backup-<time>.tar.gz` archive of every table and
every uploaded file, with a SHA-256 manifest. Archives are kept in the
`backups_data` volume; the oldest beyond `BACKUP_RETENTION` are deleted.

- **Automatic:** set `BACKUP_INTERVAL=24h` in the school's environment file.
- **Manual:** **Admin → Backups → Create backup**, then **Verify**. Always do
  this before an upgrade.
- **Off-site copies are essential.** Backups on the same server are lost
  with the server. At least weekly, download a verified archive (Admin →
  Backups → Download) or copy it from the volume, encrypt it and store it
  elsewhere:

  ```sh
  fb exec backend ls -l /app/backups
  fb cp backend:/app/backups/flybirds-backup-<time>.tar.gz .
  sha256sum flybirds-backup-<time>.tar.gz      # compare with the value shown in Admin → Backups
  gpg --symmetric --cipher-algo AES256 flybirds-backup-<time>.tar.gz
  ```

  Archives contain personal data about children. Store them encrypted, limit
  who can read them and delete copies past the school's retention period.
- Keep the school's environment file (database password and signing key)
  with the off-site backups, in a separate secure place. Backup archives
  contain the school's data but not this file. To move a school's Docker
  volumes to another server you need the file's `POSTGRES_PASSWORD`, which
  PostgreSQL stored when the database was created. To rebuild from a backup
  archive (6.2) the saved file is preferred; a newly provisioned file also
  works, because the archive is restored into a new, empty database, but its
  new signing key means everyone signs in again.

## 6. Restoring and disaster recovery

`restore-backup` loads an archive into a database. It migrates the target to
the current schema first and refuses an archive from a different schema
version. **It refuses a database that already has users** unless `-force` is
given. Never use `-force` on a live school without a fresh backup and a
deliberate decision.

### 6.1 Restore drill (do this every term)

A drill proves that backups can be restored, without touching the live data.
It restores into a temporary database next to the live one:

```sh
fb exec db sh -c 'createdb -U "$POSTGRES_USER" restore_drill'
fb exec -e DATABASE_URL="postgres://<user>:<password>@db:5432/restore_drill?sslmode=disable" \
  backend /app/restore-backup -archive /app/backups/flybirds-backup-<time>.tar.gz -uploads-dir /tmp/restore-drill -confirm
```

Compare a few counts between the live database and the drill database:

```sh
for d in flybirds restore_drill; do
  fb exec -T db sh -c "psql -U \"\$POSTGRES_USER\" -d $d -Atc \"select (select count(*) from users), (select count(*) from students), (select count(*) from fee_payments), (select count(*) from student_attendance)\""
done
```

Then remove the drill copy:

```sh
fb exec db sh -c 'dropdb -U "$POSTGRES_USER" restore_drill'
fb exec backend rm -rf /tmp/restore-drill
```

### 6.2 Recovering on a new server

1. Install the **same release** the backup came from (`restore-backup`
   refuses an archive whose schema version differs from the release's), following
   [DEPLOYMENT.md](DEPLOYMENT.md) with the school's saved environment file.
   Start it and **do not create any user**.
2. Copy the archive into the backups volume and restore it:

   ```sh
   fb cp flybirds-backup-<time>.tar.gz backend:/app/backups/
   fb exec backend /app/restore-backup -archive /app/backups/flybirds-backup-<time>.tar.gz -confirm
   fb restart backend
   ```

3. Sign in as an administrator and check recent attendance, payments and
   attachments. Then upgrade to the latest release if needed.

## Rotating secrets

Rotate a school's secrets when they may have been exposed, for example a
copied environment file, or when staff with server access leave. Make and
verify a backup first, and update the saved copy of the environment file
afterwards. The commands use the `fb` alias.

**Signing key (`JWT_SIGNING_KEY_B64`).** Replace the value in the
environment file with a new one (`openssl rand -base64 32`) and run
`fb up -d --wait`, which recreates the backend. Access tokens signed with
the old key stop working at once, but **sessions continue**: browsers
silently obtain new tokens with their session cookie, which is stored in the
database and does not depend on the key. To also end every session, so
everyone must sign in again, revoke them:

```sh
fb exec db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "UPDATE auth_sessions SET revoked_at = now(), updated_at = now() WHERE revoked_at IS NULL"'
```

Revoking takes effect immediately for every access token too, with or
without a new key. To sign out one person, use `reset-password` (section 4).

**Database password (`POSTGRES_PASSWORD`).** PostgreSQL keeps the password
it was created with; editing the file alone breaks the backend's next
connection. Change it in PostgreSQL first, then in the file:

1. Generate a new value: `openssl rand -hex 24`.
2. Set it in PostgreSQL. `\password` asks for it twice without echoing it:

   ```sh
   fb exec db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
   # at the psql prompt:  \password flybirds     (your POSTGRES_USER), then \q
   ```

3. Put the same value in `POSTGRES_PASSWORD` in the environment file.
4. `fb up -d --wait` recreates `db` (its data volume is kept) and `backend`.
5. Check `https://<domain>/api/readyz`, sign in, and update the saved copy of
   the file.

If the backend restarts in a loop after step 4, the two values differ:
repeat step 2 with the value in the file.

## 7. Troubleshooting

| Symptom | Check |
|---|---|
| Site unreachable | `fb ps`: all services `healthy`/`running`? `fb logs caddy` for certificate errors (DNS must point at the server; ports 80 and 443 open). |
| "Service unavailable" or 502 | `fb logs backend`. `fb exec backend wget -qO- http://127.0.0.1:8080/readyz` reports whether the database is reachable. |
| Backend restarts in a loop | `fb logs backend`: a missing or short `JWT_SIGNING_KEY_B64`, a wrong database password, or a failed migration. Never delete volumes to "fix" this. |
| Everyone signed out | Sessions were revoked (see rotating secrets), or the database was restored from a backup. Signing in again is enough. A changed signing key alone does not sign people out. |
| Provisioning a new school fails | See [DEPLOYMENT.md, provisioning errors](DEPLOYMENT.md#35-provisioning-errors). |
| Sign-in fails for everyone | The site must be opened over HTTPS (secure cookies). Check the browser address and the proxy. |
| "Too many requests" at login | The rate limit (30 per minute per address). Wait a minute. Many users behind one school NAT share an address. |
| A user is locked out | Wait 15 minutes or use `reset-password` (section 4). |
| Upload refused | Files above 10 MiB (PDF) or 5 MiB (image) are refused. |
| Disk filling up | `docker system df -v`; lower `BACKUP_RETENTION`; move old archives off-site. Do not prune volumes. |

Logs: `fb logs --since 1h backend`. Audit trail: **Admin → Audit log**.
System figures: **Admin → Control center**.

## 8. Release validation

After every installation, upgrade or rollback:

- [ ] `fb ps`: `db`, `backend` and `web` healthy; `caddy` running.
- [ ] `https://<domain>/api/readyz` returns `ready`.
- [ ] `http://<domain>/` redirects to HTTPS; the response has
      `Strict-Transport-Security` and `Content-Security-Policy`.
- [ ] Sign in as the administrator; the dashboard shows the school's real
      figures, name, logo and colours.
- [ ] `fb logs backend` shows the expected migration version and no errors.
- [ ] Create and verify a backup.
- [ ] A teacher account can open today's attendance register (look, don't
      save, unless it is a real class).
- [ ] Switch to Bangla and back.

If any check fails, stop and roll back as in
[DEPLOYMENT.md](DEPLOYMENT.md#8-rolling-back).

## 9. Data protection checklist before go-live

- [ ] No demo accounts (`@demo.flybirds.test`) exist in the school's database.
- [ ] Every staff account has its own address and a strong password.
- [ ] Online payment is switched off (`fees:pay_online` removed) or the school
      understands it is simulated.
- [ ] The school knows SMS is simulated.
- [ ] Automatic backups are on, a restore drill has passed and an encrypted
      off-site copy exists.
- [ ] The environment file is stored securely off the server.
- [ ] Server access (SSH keys, sudo) is limited to named operators.
