# 2026-11-16 — `feature/go-live-prep`

Getting the system ready to be put online for a real school, without creating
any server or account: everything that has to exist in the repository, so the
hour spent on the server is only following steps.

## 1. "Forgot password" by email

A time-limited, single-use link, sent only when the installation has a mail
relay. Without one nothing changes: the sign-in page keeps its
"contact your school office" line and an administrator resets passwords from
**Admin → Users** as before.

**How it is built.** `SMTP_HOST` (with `SMTP_PORT`, `SMTP_SECURITY`,
`SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM`, `SMTP_FROM_NAME`) switches it
on; `config.Load` refuses a half-finished configuration and requires
`PUBLIC_BASE_URL`, because that is where the emailed link points. The new
`internal/mailer` package is one small interface with an SMTP implementation
(STARTTLS on 587, implicit TLS on 465, and `none` only for a fake relay in a
test, which refuses a password). `auth.Service.EnablePasswordResetEmail` is a
setter, like `EnableDemoQuickLogin`, so the router's constructor chain did not
have to change, and the reset store is its own `ResetRepository` interface so
the sign-in path and its test doubles were untouched.

**What the design is careful about.**

- The form answers `202 {"sent":true}` whether or not the address has an
  account, so it cannot be used to find out who has one. The message is handed
  to the relay in a goroutine, so the time the request takes does not give the
  answer away either.
- Migration `1813` keeps the address only as a SHA-256 digest, so the table
  cannot be mined for the school's email addresses, and keeps only the
  SHA-256 of the link's token, the way refresh tokens are kept.
- One table is both the token store and the abuse log, because the limit has
  to count attempts for addresses that have **no** account; those rows carry
  no user and no token. That is also why answering `429` leaks nothing.
- Limits: three links an hour per address (in the database, so it survives a
  restart), and six requests a minute per client in nginx, which is the only
  place the real client address is known.
- Using a link marks it used **and** marks the account's other outstanding
  links used, so an older email in the same inbox is not a second key. It
  also clears any login lockout and any forced password change, and revokes
  every session.
- An inactive account, and a student login whose address is the
  `no-email.invalid` placeholder, are not candidates: they answer exactly as
  an unknown address does.
- `user.password_reset_requested` and `user.password_reset` are audited
  without the address, the link or any password material.

**Front end.** `/forgot-password` and `/reset-password` are public pages in
English and Bangla. The sign-in page shows "Forgot your password?" only when
the server says a relay is configured, and keeps the school-office line
underneath either way. The emailed message is bilingual, with the reader's own
language first.

## 2. `docs/HOSTING_FREE.md`

Step by step, in English (sections 1–12) and Bangla (section 13 onwards, with
mirrored numbers), for an always-free cloud VM. It covers an Oracle Cloud
Always Free ARM machine — every image in the stack, including the pinned Typst
image, has an `arm64` manifest, which was checked rather than assumed —
Docker, the two places Oracle blocks ports 80 and 443, the environment file
and its two secrets, the first administrator, restart on reboot (the restart
policy plus an optional systemd unit), monthly tidying, and a table of what to
look at when something is wrong.

Both addressing options are covered: a free DuckDNS subdomain now, the
school's own domain later, and the three commands that move from one to the
other. The guide insists on real HTTPS for either, because the session cookies
are `Secure`: on plain `http://` a password is accepted and the session is
then lost, which is a confusing way to discover the problem.

Section 11 is the specific job of moving the Jalalabad school's data out of
the laptop's `fbshow` stack and onto the server, including the rule that trips
people up — a restore refuses an archive whose schema version differs from the
target, so both stacks must be on the same release — and what the school will
and will not find afterwards.

## 3. A nightly backup, and a restore that was run

`backend/cmd/create-backup` writes one archive (whole database, every uploaded
file) and prunes to `BACKUP_RETENTION`, so the backup happens at a chosen hour
instead of drifting with the API's uptime; it is in the backend image beside
`restore-backup`.

`deploy/backup-daily.sh` is what cron runs: it makes the archive, copies it out
of the Docker volume onto the host disk, checks the copy against the SHA-256
the archive was written with, reads it with `tar` to catch a broken gzip
stream, and keeps the newest seven. An archive that exists only inside the
project's volume is lost with the volume, which the script and the guide both
say.

## 4. `deploy/healthcheck.sh`

One command after a deploy. It checks the containers and the backend's own
health, HTTPS and the certificate's expiry date, `/healthz`, `/readyz`, the
disk (root filesystem, and Docker's data directory when that is a different
one), and — the check worth having — it compiles a real page with the Typst
binary in the backend image, with the bundled fonts, in **English and
Bangla**, exactly as a marksheet is rendered. That catches a missing binary, a
missing font or an unwritable temporary directory without needing anyone's
password. One line per check, non-zero exit if any failed.

## 5. `e2e/marksheet-admit.mjs`

It was a test problem, and this is why it broke. A class with no blueprint
gets the starting design, and that design has one assessment per term of the
session — including the term the script creates a few lines earlier. The
script dropped that assessment (it adds its own) and left every subject's
sheet pointing at the dropped key, so the server refused the definition with
`EX1 is not an assessment of the blueprint`. The server is right; the script
now prunes the sheets and term rules of anything it removed. It passed before
only because the demo class happened to have an *active* blueprint, in which
case the script branches to a copy of that version and the seed never runs.

Nothing in the application needed changing for it.

## 6. A fault the new e2e suite found

`e2e/password-reset.mjs` failed its Bangla check, which turned out not to be a
test problem. The `GET` that tells the sign-in page whether "forgot password"
exists shared the strict per-client nginx limit with the `POST` that sends
mail, so after a handful of page views the availability call answered `429`,
the client treated that as "off", and the link quietly disappeared from the
sign-in page. The two now have their own zones and only the `POST` is keyed,
because an empty key switches a `limit_req` off for that request. The suite
asserts it: twelve availability calls in a row, no `429`.

## 7. Checks

- `go vet ./...` clean. The new Go files are gofmt-clean (the files
  `gofmt -l` lists repo-wide are the pre-existing CRLF artifact, untouched).
- `go test ./... -p 1 -count=1` on a fresh database (`golive` in
  `m15-testdb`): **31 packages ok, 11 with no test files, 0 failures**, exit 0, no out-of-memory
- New Go tests: `internal/auth` covers the happy path (bilingual message,
  lockout cleared, sessions revoked, audit), single use, expiry to the second,
  a token never issued, a short password leaving the link usable, older links
  dying with the one that is used, an unknown address and a suspended or
  placeholder account answering silently, both rate limits and the window
  moving, and a failing relay not changing the answer. `internal/httpapi`
  covers the endpoints: off without a relay, identical answers for a known and
  an unknown address, `429` at the limit, and the link working once.
  `internal/mailer` covers header injection, dot-stuffing and the settings
  checks; `internal/config` covers the SMTP validation.
- Front end: `tsc --noEmit` clean, `npm run build` passed (it runs `tsc --noEmit` first),
  `npm run check:i18n` passed (4058 English and 4056 Bangla keys),
  `npm run test:unit` 22 of 22.
- On `m15b1`, rebuilt from this branch and freshly seeded:
  `suite.mjs` **130 of 130**, `phone-login.mjs` **29 of 29**,
  `marksheet-admit.mjs` **35 of 35**, `password-reset.mjs` **30 of 30**.
- `deploy/backup-daily.sh` was run three times against `m15b1` with `KEEP=2`:
  each run made, copied and verified an archive, and the third pruned the
  oldest. The restore was then run for real into a scratch database with
  `restore-backup`: **112 tables, 3415 rows and 53 attachments**.
- `deploy/healthcheck.sh` was run against `m15b1`: containers, HTTPS,
  `/healthz`, `/readyz` and the Typst render all passed; the disk check
  correctly failed, because this laptop's disk is 91% full.

## 8. Notes

- `m15b1`'s database was recreated inside its existing Docker volume (the
  volume was kept) and seeded again, because the browser suites need a freshly
  seeded demo school and earlier runs had already changed this one. The same
  demo password as before was used.
- `deploy/m15b1.env` (not in Git) now points `SMTP_HOST`/`SMTP_PORT` at
  `host.docker.internal:2525` with `SMTP_SECURITY=none`, which is what
  `e2e/password-reset.mjs` listens on. No real relay was contacted and no
  message left the machine.
- `deploy/compose.prod.yaml` now passes the seven `SMTP_` settings through to
  the backend; without that the environment file was read and ignored, which
  is how the first run of the new suite failed.
- `.gitattributes` keeps `*.sh` at LF, so the two new scripts are runnable
  after a checkout on any host.
- `fbshow` was not touched: not restarted, not rebuilt, and its data not
  changed. Section 11 of the hosting guide is written for the owner to run.
- This branch does not carry the marksheet fix on `fix/marksheet-pdf-2` (a
  marksheet for an exam with no component breakdown could not be rendered).
  The two change no code in common, but both add a row to the table in
  `e2e/README.md` just above `narrow-phones.mjs`, so whichever merges second
  will want that one line resolved by keeping both rows.

## 9. What is not covered

- No cloud account, server, DNS name or email account was created. The guide's
  section 1 is the list of what only the owner can do.
- The hosting guide's own steps were not run on a real Oracle machine: the
  commands are the standard ones and every part that this repository controls
  (the compose stack, the two scripts, `create-backup`, `restore-backup`, the
  ARM manifests) was checked locally, but the cloud console steps and the
  DuckDNS and Brevo sign-ups are described, not demonstrated.
- The reset email was delivered to a fake relay in this process, never to a
  real provider, so nothing here shows how Brevo treats a new sender. That is
  why the guide warns about SPF and DKIM before promising the feature to a
  school.
- The per-client limit on asking for a link lives in nginx, so it is not
  exercised by the Go tests; the database limit on one address is.
