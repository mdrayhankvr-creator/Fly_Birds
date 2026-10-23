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
| Online bKash and Nagad payment | **Off.** There is no live bKash or Nagad integration. A disposable test installation can switch on a simulation that moves no money and records nothing (below). A real payment gateway contract and integration are needed before parents can pay online. |
| bKash and Nagad payments recorded at the counter | **Live**: the accountant records a payment after checking it, as for cash or bank. |
| SMS notifications | **Simulated** by a mock provider. Messages are queued and marked sent but never reach a phone. In-app notifications are real. |

Tell staff that SMS is not delivered.

### Online payment modes

Online payment has two modes, shown in **Fees and accounts → Fee setup →
Online payments** (administrators only):

- **Off** (every school by default). Guardians see no online payment, and the
  server refuses any attempt with `online_payments_disabled`.
- **Simulation**, for disposable test installations only. It needs both
  `PAYMENT_SIMULATION_ALLOWED=true` in the school's environment file and an
  administrator switching it on. A completed simulation is labelled
  **SIMULATED — NO MONEY TRANSFERRED**, uses a `SIM…` reference, issues no
  receipt, records no payment or income and leaves the balance unchanged.
  Switching it off blocks checkouts already started. Both changes are audited.

Never set `PAYMENT_SIMULATION_ALLOWED=true` for a real school. Students cannot
pay online in either mode; only guardians hold `fees:pay_online`.

### Simulated payments recorded before the payment-safety upgrade

Earlier releases recorded mock bKash and Nagad checkouts as ordinary payments
with receipts, although no money was received. The upgrade does not delete or
rewrite them. It labels every online payment **SIMULATED — NO MONEY
TRANSFERRED** (audit event `fee.simulated_payments_labelled` with the count and
amount), and from then on:

- collection reports, the income and expense summary and analytics leave them
  out; the collection report shows them as a separate simulated figure;
- their receipts are titled "Simulation record — not a receipt", with a
  watermark, and exports carry a `simulation` column;
- they still reduce the student's dues until reviewed, so the statement warns
  the guardian and staff.

To review them, open **Fee setup → Online payments → Simulated payments to
review**. Check each one with the family. If no money was received, enter a
reason and choose **Void**: the dues it covered are owed again, and the
payment stays in the history marked void (audit event
`fee.simulated_payment_voided`). If the family did pay in another way, record
that real payment at the counter first, then void the simulated one. Only
administrators can void, and only simulated payments.

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

### 8.1 Checking published exam results (`exam-verify`)

`exam-verify` recalculates every published or locked exam result from the
stored marks and the school's current grading bands, using the exact result
engine, and lists any stored grade, GPA, total or merit position that would
differ. **It only reads:** it runs in one read-only database transaction and
never changes marks, results or publication. Run it after upgrading past the
exact result engine (E2), and whenever results are questioned:

```sh
fb exec backend /app/exam-verify            # a table; exit 0 = no differences, 3 = differences
fb exec backend /app/exam-verify -json      # the same report as JSON
fb exec backend /app/exam-verify -session <session id>
```

The report names the exam, class, section, student, subject and both
values. Each difference has a kind:

- **boundary_correction:** the stored value was produced by the earlier
  engine, which graded a mark lying exactly on a band edge one band too low
  (for example 16.83 of 51, exactly 33 %, stored as F and failed instead of D
  and passed). To correct it, check the exam with the office, then unpublish
  and publish it again in **Examinations**: results are recalculated with the
  exact engine, and students and guardians see the corrected result. Run
  `exam-verify` again to confirm there are no differences.
- **inconsistent:** neither engine reproduces the stored value from today's
  marks and grading bands. Usually the grading bands or a class's fourth
  subject were changed after the exam was published. Republishing would apply
  today's settings, so decide with the school first; do not republish only to
  silence the report.

The output contains student names and admission numbers. Keep it with the
school's records, not in shared chat or tickets.

### 8.2 Mark sheets and mark statuses

From E3, each exam subject is a **mark sheet** with a review state, and each
mark has a status. Explain this to teachers and the head teacher before the
first exam after the upgrade:

- **Teachers** mark a student **Absent** (fails the subject, as a 0 did
  before) or **Incomplete** (for example, a missing script). Incomplete stops
  publication until it is resolved. When every student has marks or a status,
  the teacher submits the sheet for review; it is then read-only for them.
- **The head teacher** approves a submitted sheet, or returns it with a reason
  that the teacher sees.
- **The administrator** can also **exempt** a student from a subject (the
  subject leaves the result and the GPA) or **withhold** a result (the family
  sees "Result withheld — contact the school office", with no marks). Both
  need a reason, which only staff see.
- An exam cannot be published while a sheet is awaiting review or has been
  returned, or while any mark is missing or incomplete; the exam page lists
  what is left. Publishing approves every sheet; returning an exam to draft
  reopens them for correction.

The upgrade (migration 1501) changes no marks or results: marks become
Present, sheets of published exams become Approved, and sheets of draft exams
become In progress or Not started. The audit log records the counts as
`exam.sheets_migrated`.

### 8.3 Teacher marks workspace

From E4, teachers enter marks in **Marks entry → My exam sheets**: one sheet per subject and section, saved automatically as they type. Tell teachers:

- **Saving:** there is no Save button. The indicator at the top shows Saving, All changes saved, Offline (changes are kept and saved when the connection returns) or a conflict to resolve.
- **Pasting from Excel or Google Sheets:** if the first column holds admission numbers, rows match by student whatever their order. The preview shows every rejected line and the reason before anything is saved.
- **Unsaved changes after a crash:** they are offered back when the sheet is reopened **in the same browser tab**. Nothing is kept after the tab closes or the teacher signs out, so shared computers keep no marks.
- **Approving and returning sheets:** the head teacher and the administrator use **Open sheet** in the exam's sheets panel.
- **Exempt and withheld** stay administrator-only. A teacher sees these rows as "Set by the administrator".

### 8.4 Terms, blueprints and setting up a term

From E5, an administrator designs the year in **Examinations → Terms & blueprints**:

1. **Terms:** add the session's terms (one click adds First Term, Half-Yearly and Annual).
2. **Blueprints:** for each class, start a blueprint (or copy last session's), then:
   - add its class tests;
   - choose how each subject's term mark is made (for example "CT average + final");
   - set components such as CQ and MCQ, and combine Bangla 1st and 2nd papers;
   - when the check is clear, **Activate**. An active version never changes; make a new version for changes.
3. **Optional subjects:** record each student's fourth subject. Once one student in a class has a choice, choices decide for the whole class.
4. **Set up exams:** on the term, give the marks due dates and the teachers for any subject nobody teaches, then **Create term**. Every exam and sheet is made at once; creating again only adds what is missing.

**Term results** (provisional, staff only) show how each student's term mark comes out. They are published from **Results & publication** (§8.5); each exam is still published from the exam page.

**After the upgrade:**
- **Existing exams:** they keep their marks and results exactly. They are linked to terms and to a "from existing exams" blueprint for each class.
- **Published class tests:** students and guardians no longer see them on their own. To show one again, open the class's blueprint version and set the class test's **Students see** to **When published** (audited).
- **Optional-subject changes:** make them before marks are entered. Changing one after publication never changes a published result; the review then shows the difference, which reaches families only as an approved revision (§8.5).

### 8.5 Publishing, correcting and withdrawing results (E6)

Every published result is a version that never changes. Corrections add a
version; earlier versions stay readable to staff under **History**.

1. **Exams:** publish each exam from the exam page as before. When another
   section's results move a student's class position, that student gets a
   position update (a new version).
2. **Term results:** open **Examinations → Results & publication**, choose the
   session and, for a class marked *Ready for review*, **Review**. The head
   teacher checks the provisional tabulation and **Approves these results**.
   An administrator then **Publishes results** and confirms. If marks change
   after approval, publication is refused until the results are approved
   again.
3. **Annual results:** once every term of the class's blueprint is
   published, review, approve and publish the annual result the same way.
4. **Correcting a published result:** on the exam page, under
   **Corrections**, choose the student, give a reason and **Open
   correction**. Change the marks shown beside the published values and
   **Save corrected marks**. Then **Review and publish corrections**: the head
   teacher approves the revision, an administrator publishes it. Families
   keep seeing the published result until then. **Cancel correction** puts the
   published marks back. Afterwards, review the class's term (and annual)
   result: it shows the change, which is approved and published as its own
   revision.
5. **Withdrawing an exam's results** (**Withdraw results**, with a reason)
   hides them from families and reopens the sheets; the versions are kept as
   withdrawn and publishing again adds the next version. Such an exam can no
   longer be deleted.

**After the upgrade (migration 1503):** every result already published is
recorded as version 1 exactly as stored; the audit log records
`exam.result_versions_migrated` with the counts. Nothing is recalculated and
nothing families see changes. Run `exam-verify` before and after, as in §8.1;
it now checks each exam against the grading bands it was published with.

## 9. Data protection checklist before go-live

- [ ] No demo accounts (`@demo.flybirds.test`) exist in the school's database.
- [ ] Every staff account has its own address and a strong password.
- [ ] `PAYMENT_SIMULATION_ALLOWED` is `false` (or absent) and **Fee setup →
      Online payments** shows Off.
- [ ] After an upgrade, every simulated payment to review has been checked
      and voided or explained.
- [ ] The school knows SMS is simulated.
- [ ] Automatic backups are on, a restore drill has passed and an encrypted
      off-site copy exists.
- [ ] The environment file is stored securely off the server.
- [ ] Server access (SSH keys, sudo) is limited to named operators.
