# E7 — Result documents and PDFs

Branch `feature/e7-result-pdfs`, rebased onto `main` `0487aaa3` (premium UI
checkpoint 5, migrations 1755–1759). Migration 1800. Builds on
the published result versions of E6: every document comes from a version's
snapshot (or, for a staff-only preview, from the same calculation the review
shows), never from live marks.

## 1. Renderer (spike, 2026-10-31)

The owner's requirements came before the spec's recommendation (headless
Chromium): self-hosted inside the Compose stack, light on memory and disk, and
correct Bangla. With D: nearly full at the start, Typst was tried first.

| | Typst 0.15.1 (chosen) | Headless Chromium (not tried) |
|---|---|---|
| Where it runs | A 56 MB binary copied into the backend image from the official image, pinned by digest; one process per render | A separate service and a much larger image |
| Bangla | Correct: ক্ষ, ত্র, শ্রেণি, স্ত্র, জ্ঞ, ষ্ট্র, reph (শিক্ষার্থী, সর্বোচ্চ, কার্য), vowel signs and Bangla digits, regular and bold, checked by eye at 150 ppi | — |
| Network | None: rendered with the container network off; no packages, no system fonts | — |

Spike measurements (reference laptop, the Typst container, sample data with
nine subjects):

| Render | Time | Peak memory |
|---|---|---|
| 1 marksheet, English | 0.32 s | 11.8 MB |
| 1 marksheet, Bangla | 0.46 s | 12.6 MB |
| 60 marksheets in one PDF, Bangla (2.5 MB) | 3.3 s | 98 MB |
| 60 marksheets in one PDF, English | 2.0 s | 86 MB |
| 60 separate Bangla PDFs, one after another | 15.7 s | 13 MB |
| Joining 60 pages of earlier PDFs into one | 0.5 s | 16 MB |

Image cost: the backend image grows by 59.8 MB (Typst 56.3 MB, fonts 2.36 MB,
API binary 1.1 MB). Fonts: Noto Sans and Noto Sans Bengali, static TTF, SIL OFL
1.1, in `backend/assets/fonts` with their licences and checksums.

## 2. Documents

| Document | Who | Notes |
|---|---|---|
| Marksheet (one student; term or annual) | The student (their own, current version only); staff as below | A4 portrait. School logo (or initials), names in both languages, the EIIN when set, address (Bangla address on Bangla documents when given), phone and email from school settings, accent colour; student block with photo (placeholder if none), ID, class, section, roll, session, exam. Subject table: CQ/MCQ/practical (or each term's marks for annual), class-test share, total, highest in the class, grade, grade point; the fourth subject marked. Summary: total, GPA, GPA without the fourth subject, letter grade, result, class and section positions, publication date. Grading scale, signature lines (head teacher's signature and title from the ID-card settings), verification QR, document code and version. Withheld: a notice instead of marks |
| Marksheets (class or section, one PDF) | Staff | One page per student; more than 12 students are made in the background in parts of 20, with progress |
| Tabulation sheet | Staff | Landscape; A3 above ten subjects; every student × subject (marks and grade), total, GPA, position, result |
| Merit list | Staff | Passed students by class (or section) position |
| Failed subjects | Staff | Each student with a failed subject, the subjects and marks |
| Result summary | Staff | Totals, pass rate (passed ÷ passed + failed), by section, grade distribution by GPA and by subject |
| Provisional preview | Head teacher and administrator | The marksheets of unpublished results, watermarked "PROVISIONAL — NOT PUBLISHED", with no QR code |

Every document exists in English and Bangla (digits, dates and labels).

**Access (server side):** students download only their own current published
marksheet (`exams:read_self`); another student's, a superseded or a withdrawn
version is 404. Staff need `exams:documents` (migration 1800 grants it to the
administrator, the head teacher and teachers): the administrator and head
teacher have every class, a teacher only the classes they teach that session
(`teacher_assignments`). Accountants, guardians and students get 403 on staff
routes.

**Cache:** a document is identified by what it shows and by a hash of the
current published version ids of the whole class (highest marks and positions
depend on it), the template, the branding, the photos shown and the
verification address. A correction publishes new versions, so the next request
makes a new document; the older file and row are deleted once the new one is
ready. Files live in the uploads volume (already in backups).

**Verification:** a printed version gets a random 22-character token
(`result_verifications`); the QR code opens `/verify/result/{token}`, which shows
whether the result is current, corrected later (superseded) or withdrawn, what
it is of, the version, publication date and document code, and the school. No
name, identifier, mark or grade. `PUBLIC_BASE_URL` (default
`https://SCHOOL_DOMAIN`) sets the address in the code; without it the
marksheet prints the document code only.

## 3. Screens

- **Student results:** "Download PDF (English)" and "(বাংলা)" on each published
  term or annual result.
- **Results review** (head teacher, administrator): a "Result documents (PDF)"
  card with language, section and the five class documents; before publication
  only the provisional preview.
- **Teacher's marks page:** "Result documents of my classes", one card per
  published class result.
- **Blueprint builder:** "Quick setup": CQ 70 + MCQ 30 or CQ 50 + MCQ 25 +
  Practical 25, with or without class tests at 20 %, for every subject of a term
  in one click; copy another class's blueprint of the same session (shared
  subjects keep their sheets and rules). Copying last year's blueprint was
  already on the setup screen.

Shared files: one lazily loaded public route in `App.tsx` and the
translations registered in `i18n.ts` (one commit). Texts are in
`e7Translations.ts`.

## 4. API

| Method | Path | Permission |
|---|---|---|
| GET | `/exams/documents/classes` | `exams:documents` |
| POST | `/exams/documents` | `exams:documents` |
| GET | `/exams/documents/{id}` | `exams:documents` |
| GET | `/exams/documents/{id}/file` | `exams:documents` |
| GET | `/exams/me/documents/{versionId}?lang=en\|bn` | `exams:read_self` |
| GET | `/public/results/{token}` | none (examinations module on) |

## 5. Verification (2026-11-01, disposable resources only)

- **Go:** `go test ./... -p 1 -count=1` on a freshly created disposable
  PostgreSQL database: 29 of 29 packages with tests passed (including the new
  `documents` package). `go vet ./...` and gofmt clean (240 files).
  `TestPostgresResultDocuments` covers published-only (409 before publication),
  the head-teacher-only provisional preview without verification tokens,
  layout data (components, class-test share, highest marks, fourth subject,
  GPA without it, failed and passed students), the cache, every role's access,
  students' own marksheets (another student's and superseded versions are 404),
  the verification check (current, superseded, unknown; no personal data in
  the response), a correction giving a new document and deleting the old file,
  and the background worker joining parts with progress.
- **Frontend:** typecheck and production build pass with no large-chunk
  warning; i18n check passed (`e7Translations.ts`: 64 keys in each language);
  unit tests 18 of 18 (new: presets for every subject, a class test added when
  missing, copying another class).
- **Browser,** on `m15b1` rebuilt fresh from this branch and demo-seeded:
  `suite.mjs` 117 of 117, then `result-documents.mjs` 31 of 31 with no console
  errors. Timings in that run (request to download, browser included): a
  student's marksheet 0.49 s; tabulation sheet 1.3 s; the 60-student
  marksheet bundle 7.6 s (English, in the background with progress) and 8.4 s
  (Bangla); 60 provisional marksheets 15.0 s. An earlier run of the same script
  measured 5.1 s and 6.6 s for the bundles.
- **Samples:** `C:\Users\Laptop Click\Fly_Birds_QA_env\pdf-e7\` (outside the
  repository): marksheets with a fourth subject (EN, BN), a failed student (EN),
  a withheld result (BN), the 60-student bundle (EN, BN), the provisional
  preview (BN), tabulation (class EN/BN, section A, after a correction), merit
  list, failed subjects and summary (EN, BN), with page images in `images/` and
  the screens in `screens/`.

Not part of E7: on phones, a signed-in visitor of the public check pages (this
one and the ID card's) sees the app shell's user-menu button and home link
without accessible names (axe `button-name`, `link-name`). Signed out, as a
QR reader is, both pages are clean.

## 6. Startup safety (2026-11-01)

One fresh start of the rebuilt stack failed with `commit migrations: timeout`:
the API gave connecting *and* applying every migration one 10-second
deadline, right after Docker Desktop had started. Measured since:

| | Time |
|---|---|
| All migrations, fresh database (test PostgreSQL in memory, 3 runs) | 2.2 s, 2.2 s, 5.1 s |
| Upgrade from `main` `d35b1da8` (1754 → 1800, 3 runs) | 0.12–0.14 s |
| All migrations, fresh `m15b1` on its Docker volume (3 runs) | 2.0 s, 3.2 s, 1.7 s; `up --wait` 19–22 s; no restarts |

Migrations now have their own deadline (`MIGRATION_TIMEOUT`, default 5
minutes) and the API logs how long they took.
The backend health check keeps its interval, timeout and retries; its start
period grows from 30 s to 120 s, so a slow first start is not reported
unhealthy (and the web container still starts), while the first passing check
marks it healthy at once.

Measured again on 2026-11-05 before pushing, with the API's own
`migrations applied` log line:

| | Time |
|---|---|
| All migrations, fresh database on the `m15-testdb` container (3 runs) | 4.6 s, 4.0 s, 4.9 s |
| Upgrade from `main` `d35b1da8` (1754 → 1800, 3 runs) | 0.19–0.24 s |
| All migrations, fresh `m15b1` (2 runs, the Go suite running alongside) | 3.7 s, 2.9 s; `up --wait` 21 s; no restarts |

The health check reports a backend unhealthy only after 120 s plus three
failed checks 15 s apart (about 165 s), roughly 30 times the slowest first
start measured. When Docker Desktop itself starts, the restart policy starts
the API and PostgreSQL together, without Compose's start order
(`depends_on: condition: service_healthy`, which `compose.prod.yaml` already
has, applies only to `up`). The API then exited on the first refused
connection and restarted until the database was up (6 quick restarts, healthy
within about 20 s on this laptop). Since 2026-11-05 the API waits for the
database instead: it tries again every second for up to a minute, logging
"waiting for the database" once, then migrates. The health checks are
unchanged.

## 7. School details on documents (2026-11-05)

After premium UI checkpoint 5 moved the school's details into
`school_settings` (migration 1759), every document header reads them there:
"EIIN: 123456" under the school name when `eiin` is set (Bangla digits on
Bangla documents; the line is left out when empty), then the address
(`address_bn` on Bangla documents when given, otherwise `address`), the phone
and the email. They are part of the branding key, so changing them makes new
documents. Rendered samples with and without the EIIN are in
`Fly_Birds_QA_env\pdf-e7` (`eiin-*`).
