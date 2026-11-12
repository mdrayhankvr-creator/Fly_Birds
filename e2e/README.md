# Browser suites

End-to-end checks that drive a real headless Chrome or Edge over the Chrome
DevTools Protocol against a running Fly_Birds stack. They need Node 22 or
later and no npm packages.

| Script | What it checks |
|---|---|
| `suite.mjs` | Landing and login pages, every role's home dashboard and navigation, the attendance register flow, module screens, Bangla and English, mobile layout, dark theme, uncaught exceptions and Content-Security-Policy violations, guardian facilities in the student account, and the school landing page: identity, sign-in form on the page, no vendor marketing, demo quick sign-in only when the server offers it, the school shown once with no header sign-in link, a 375px phone layout, and the footer's one small "Powered by Fly_Birds" (118 checks). |
| `whitelabel.mjs` | School branding: name, colours and logo applied across the shell, then restored (23 checks). |
| `exam-setup.mjs` | E5 exam setup: a term, a class blueprint built in the browser (class tests, components, combined papers, optional subjects, validation, worked sample), the term wizard, subject choices, component entry, provisional term results, who sees what, four screens in three sizes, both languages and themes, contrast and axe (83 checks). Run it after the suites that expect the seeded section sizes. |
| `result-publication.mjs` | E6 result versions: exam publication into versions, the publication centre, head-teacher approval and administrator publication in three clicks, what students and guardians see (no class-test detail), a correction through the exam page with superseded and verified versions, concurrent publication, grading-band and optional-subject changes after publication, withdrawal, withheld results, the annual result, permissions, four screens in three sizes, both languages and themes, contrast and axe (101 checks). It adds a term and a blueprint version to the demo student's class. |
| `daily-tasks.mjs` | Checkpoint 3's daily office tasks: the one-page admission (suggested roll, admission number and login, a guardian on record offered instead of copied, a refused admission leaving nothing behind, the slip and its one-time password that signs in), the fee counter (search by guardian phone and roll, dues oldest first, part, double-submitted and full payments, receipts with both names and the amount in words, keyboard use) and quick attendance on a phone (present by default, tap for absent and late, one Save, another device's mark kept); 375px for all three (55 checks). It records payments and today's attendance in an unmarked section, so run it after `suite.mjs` and `whitelabel.mjs`. |
| `profiles-safety.mjs` | Checkpoint 4: the rich student profile (header, section bar, attendance, results with report-card links, fees) for the admin and the student; the staff profile with the teacher's sharing choices and photo, the administrator's NID set masked and revealed with an audit entry, and what each role receives; buttons (44px on phones, pointer, filled and bordered); an administrator's password reset with its one-time slip, ended sessions and audit, and the forced password change (44 checks). It changes the demo teacher's profile and adds a throwaway user, so run it after `daily-tasks.mjs`. |
| `checkpoint5.mjs` | Checkpoint 5: Create login on a student's profile with its bilingual slip, and signing in with the Student ID (any case) into the forced password change; the same answer for a wrong password and an unknown ID; logins for a whole section with one slip each, printed two across; bulk admission from CSV with per-row problems, no admit while a row has one, siblings sharing a guardian, Bangla names kept, logins created and a re-upload skipping everyone; the public school page below the sign-in form (sections, lightbox, public notices only, nothing personal, map link not embedded, Bangla, 375px); the slip's 14-day expiry date, the school details through `/school/settings`, and names for every phone top bar control in both languages; the public footer (school details without phone numbers, email link, Bangla at 375px, both themes with axe, links from another public page, empty details left out) (64 checks). The demo seed has its own gallery photos and public notice; this script opens its own photo by caption. It admits throwaway students and edits the public page, so run it last. |
| `public-design.mjs` | Public design styles and privacy: Modern Bold default, editor switch and persistence, uploaded-logo palette, reduced motion, private-field exclusion, 320/360/375px overflow, and axe in both languages and themes. Captures 24 full-page screenshots (three styles, two widths, two languages, two themes) in `E2E_SHOTS_DIR`, writes `index.md`, and restores the original page and logo. Run after the other suites on fbqa-cp1b. |
| `marks-workspace.mjs` | E4 teacher marks workspace: keyboard entry, autosave, conflicts, offline and recovery, paste, submission and review, access, three viewports, both languages and themes, contrast and axe, failing saves (49 checks). Prepare the school first with `marks-workspace-setup.mjs`, which adds students to the demo sections, so run the other suites before it; see docs/exams/E4_MARKS_WORKSPACE.md §8. |
| `promotion-ui.mjs` | M15 year-end promotion screens: who sees them (administrator and head teacher only), class progression set on its screen, a batch from the create form, an override with a reason, head-teacher approval, apply through the confirmation, revert refused after activity in the new year with the reason shown, then reverted, the concession review, eight screens at 1440px and 375px in both languages and themes, contrast and axe (34 checks). It adds a session, a class and a fee category to the demo school. |
| `result-documents.mjs` | E7 result documents: a realistic class (60 students, nine subjects with CQ/MCQ/practical, a fourth subject, class tests) published through the API; the head teacher's provisional preview, students' own marksheets in English and Bangla, class documents for the administrator and the class's teacher (the 60-student bundle in the background with progress), who may not have what, the public QR check without personal data, a correction making new documents, blueprint presets and copying another class, five screens at 1440px and 375px in both languages and themes, contrast and axe (31 checks). Sample PDFs go to `E7_SAMPLES`; it reads one QR token from the database container (`E2E_DB_CONTAINER`, default `m15b1-db-1`). Run it on a freshly seeded school after `suite.mjs`. |
| `lifecycle-audit.mjs` | Lifecycle audit fixes: a classmate withdrawn has the later unpaid months cancelled (the month they left kept), seen on the Dues tab by the accountant and reverted with a reason by the administrator; a counter payment reversed from its receipt as a refund (dues owed again, receipt marked Reversed, ledger nets to zero, once, administrator only); a late fine set in Fee setup charged once to a later overdue due and shown at the counter; Friday as the default weekly off day and a published holiday refusing new attendance with the reason shown while recorded attendance stays; four screens at 1440px and 375px in both languages and themes, contrast and axe. Run it after `suite.mjs` and `daily-tasks.mjs`. |
| `exam-routine.mjs` | E8 term exam routine planner: rooms, a term's routine started in the browser, a Friday and a class clash refused with their rule, the generated draft (no hard conflict, hand-placed paper kept, the same draft twice), submission, the head teacher's approval (and no planning buttons), publication with its content hash, the student's next exam and own room, the teacher's duties, class, room, invigilation and student PDFs in English and Bangla (saved to `E2E_PDF_DIR`), an amendment with its difference and the moved paper marked for students; five screens at 1440px and 375px in both languages and themes, contrast and axe. It leaves a published routine; run it on a freshly seeded school after `suite.mjs`. |
| `marksheet-admit.mjs` | Marksheet v2 and admit cards: the class teacher assigned on the Sections page, a teacher's signature (upload refused when broken) shown on their profile, class teacher comments (a preset and own words) and the head teacher's review, a term with combined Bangla papers published, the student's marksheet in English and Bangla (one page each, photo, parents), a failed student's with a long comment, a changed comment making a new document while the result stays, the old report card offering the PDF, the term's routine published, admit cards by the class teacher (own section only), for a whole class (two per page) and the student's own, the fee hold with an unpaid exam fee, the student told why, the accountant's release in the browser; five screens at 1440px and 375px in both languages and themes, contrast and axe. Samples go to `E2E_PDF_DIR`. It adds subjects, a term, results, a routine and a fee, and restores the admit card settings; run it on a freshly seeded school after `suite.mjs`. |
| `narrow-phones.mjs` | No page scrolls sideways on a narrow phone: signed out and as every demo role, every page reachable from the menu and from links (one per route pattern), every tab and the open phone menu, in English and Bangla. A page fails when the document is wider than the screen or anything sticks out past the right edge without a scrolling parent. 360px by default; `WIDTHS=320,360,375`, `LANGS=en` and `ROLES=admin,teacher` change the run (15 checks by default). It only reads pages; CI runs it. |
| `ux-round2.mjs` | UX round 2: a logo uploaded in School settings (served resized, transparency kept, on the public pages); a student's own photo cropped in the browser and sent, the class teacher's rejection with a reason the student sees, a new photo approved by the administrator and becoming the student's photo, the photo in use unchanged until then; the photo setting turned off; ten compact login slips printed on two A4 pages, each within its 67 mm cell (20 checks). It makes demo.teacher the class teacher of the student's section and uploads a logo, both put back at the end, and creates one student login. |

## Requirements

- A stack whose database holds a **freshly seeded demo school** (the suite
  expects today's attendance register to be empty). Never point the suites at
  a real school.
- `DEMO_PASSWORD`: the password given to `seed-demo`.

```sh
# Production-style stack on http://localhost:8081 (see docs/DEPLOYMENT.md)
docker compose --env-file deploy/<qa>.env -f deploy/compose.prod.yaml exec -T -e DEMO_PASSWORD backend /app/seed-demo -confirm

DEMO_PASSWORD='...' E2E_BASE_URL=http://localhost:8081 node e2e/suite.mjs
DEMO_PASSWORD='...' E2E_BASE_URL=http://localhost:8081 node e2e/whitelabel.mjs
```

## Demo quick sign-in (local testing only)

The API can offer one-click sign-in for the five seed-demo staff and student
accounts, for the owner's local testing. It is off unless the API starts with
`DEMO_QUICK_LOGIN=true`; `deploy/compose.prod.yaml` never passes that variable
and school provisioning never sets it. Turn it on for a disposable stack with
an override file kept outside the repository:

```yaml
# quick-login.override.yaml
services:
  backend:
    environment:
      DEMO_QUICK_LOGIN: "true"
```

```sh
docker compose -p <qa> --env-file <qa>.env -f deploy/compose.prod.yaml -f quick-login.override.yaml up -d
```

The server signs in only accounts that seed-demo recorded in `demo_records`
at the demo address, only for requests addressed to `localhost` or a loopback
address from a page on such an origin, and audit-logs each sign-in as
`auth.demo_quick_login`. No password reaches the browser.

## Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `E2E_BASE_URL` | `http://127.0.0.1:5173` | Address of the web app. |
| `DEMO_PASSWORD` | (required) | Demo account password. |
| `E2E_BROWSER` | auto-detected | Path to a Chrome or Edge executable. |
| `E2E_INSECURE_TLS` | unset | `1` accepts a self-signed certificate (local HTTPS only). |
| `E2E_BROWSER_TIMEOUT_MS` | `30000` | How long one browser start may take to become ready. |
| `E2E_BROWSER_ATTEMPTS` | `2` | Browser starts to try before the script stops. |

## Browser startup

`cdp.mjs` starts a private browser for each `launch()` and connects only to
that browser:

- **Port.** The browser picks a free DevTools port and records it in its own
  throwaway profile (`DevToolsActivePort`). The `port` that scripts pass is
  accepted but unused, so a leftover browser or another program on a fixed
  port can never answer instead.
- **Readiness and errors.** Readiness is polled up to `E2E_BROWSER_TIMEOUT_MS`.
  A browser that cannot be started, exits during startup or never becomes
  ready fails that attempt at once, with the last lines of its own error
  output. One failed start is retried; if every attempt fails, the script
  stops with an error and runs no checks.
- **Lost connection.** If the browser exits or its connection drops during a
  run, pending and later commands fail with "browser connection closed"
  instead of hanging.
- **Cleanup.** Only the browser process this run started is stopped (on
  `close()`, on a failed start, or when the script exits), and its profile is
  deleted. `E2E_BROWSER` must point to an existing file; a wrong path is an
  error, not a silent fallback to another browser.

`node --test e2e/cdp.test.mjs` checks this behaviour. Most cases use
`testdata/fake-browser.mjs`, a stand-in that can start slowly, crash, hang or
drop its connection; one case starts the real browser three times while
another program holds the old fixed port.

Screenshots and `suite.json` results are written to `e2e/shots/`, which Git
ignores. Each script exits non-zero when any check fails.
