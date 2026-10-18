# M0–M7 Comprehensive QA, Security and Performance Audit (2026-10-18)

Branch `test/comprehensive-system-audit` starts at `origin/main` `32af048` (M7 and QA PR #11 merged). The audit adds no roadmap criteria. Completion stays at **48/83 = 57.8%**.

## Environment

- **Host:** AMD Ryzen 7 PRO 3700U (4 cores, 8 threads), 13.9 GB RAM, Windows 11 Pro. Docker Desktop 29.7.2 VM with 8 CPUs and 6.7 GiB.
- **Isolation:** Compose project `flybirds_audit_20261018` with its own volume and ports:
  - PostgreSQL: 58491
  - API: 58492
  - Vite: 58493
  - Production nginx build: 58494
- **Disposable databases:** separate test, upgrade, restore and race databases inside that container. The primary developer's containers, databases and Demo School were not touched.
- **Data:** fictional only, created through the real API:
  - 500 students (CSV import) and 500 guardians, each linked as primary guardian.
  - 50 staff (45 teachers, 5 non-teaching) and 50 teaching assignments.
  - 10 classes, 20 sections, 5 subjects (including a fourth subject for classes 9–10).
  - 360 routine periods and 10,000 attendance records (20 school days).
  - 20 half-yearly exams with 2,500 marks: 19 published, 1 draft.
  - Nine role accounts.

## Defects found and fixed

| # | Severity | Defect | Fix | Commit | Regression coverage |
|---|---|---|---|---|---|
| 1 | High (performance) | `database.Open` used Go's default pool, which keeps only two idle connections. Under concurrency almost every request opened a new PostgreSQL backend and re-planned every query. In a 15 s mixed load, 858 sessions opened, and planning was 88% of database time. | Pool of 25 open/idle connections, 5-minute idle and 30-minute lifetime limits | `49c84f7` | `TestPostgresPoolKeepsConnectionsBetweenBursts` (fails on old defaults) |
| 2 | Medium (reliability) | A NUL character in any search query, JSON body or CSV import reached PostgreSQL and returned HTTP 500 in every module (19 of 19 probed endpoints except login and audit search) | Reject NUL in query keys/values (middleware), in the shared JSON decoder and in CSV import | `3b5fc5f` | `TestRejectNULQuery`, `TestContainsJSONNUL`, `TestDecodeSchoolJSONRejectsNUL`, `TestPostgresNULInputReturnsBadRequest` (returned 500 before), `TestImportCSVRejectsNUL` |
| 3 | Medium (security) | No login throttling: 30/30 rapid failures across accounts were accepted (lockout covers one account only). No framing protection on the production edge (clickjacking). | nginx: 30 login requests/min per client with a burst of 30; `X-Frame-Options: DENY`, `frame-ancestors 'none'`, `nosniff`, `Referrer-Policy` | `a718eca` | Live edge test: 70 rapid logins gave 31 × 401 and 39 × 429; headers verified; valid login and SPA deep links still work |
| 4 | Low (i18n) | Raw keys `studentName` (exam marks table), and `actions` and `delete` (routine page), rendered in both languages | English and Bangla text, plus `npm run check:i18n` in CI | `92dcc3a` | `check:i18n` fails on any static `t('key')` missing in either language; browser label check |
| 5 | Low (i18n) | Bangla report cards (screen and PDF) printed English subject names. Students, guardians and teachers saw the admin description "Manage exams, enter marks, and publish report cards." | `subject_name_bn` on mark rows; role-specific descriptions | `7d7515c` | `TestPostgresExamLifecycle` now requires the Bangla subject name; browser and PDF checks |
| 6 | Low (rendering) | The English font stack had no Bengali font. Bangla text in English mode used the OS font, and PDFs embedded Windows-only Nirmala UI. | Bundled Noto Sans Bengali after Inter | `9e3838f` | PDF font inspection |
| 7 | Test reliability | `TestPostgresExamRouteProtection` failed on any database the suite had already used, because another test strips teacher grants | Restore M7 exam grants at test start | `c8f80bf` | Passes on a reused database |

## Results by area

| Area | Result | Notes |
|---|---|---|
| Go vet | PASS | Final code |
| Go suite, fresh PostgreSQL | PASS | All 11 packages, including the new regression tests |
| Go race detector with PostgreSQL | PASS (no data races) / 3 test-deadline FAILs | Zero `DATA RACE` reports in any package. Every package except `auth` and `httpapi` passed outright. With a 45-minute limit, all 25 `httpapi` tests ran (17 minutes): 23 passed and 2 failed with `context deadline exceeded` at their fixed 70 s contexts (`TestPostgresExamRouteProtection`, `TestPostgresRoutineRouteProtection`). `TestPostgresAuthenticationLifecycle` likewise exceeded its 30 s context. bcrypt-heavy flows slow sharply under race instrumentation. These are test-harness timeouts, not product failures. |
| OpenAPI route/reference test | PASS | |
| Frontend TypeScript, production build, `check:i18n` | PASS | The build still warns about a 585 kB bundle (pre-existing) |
| Docker Compose validation | PASS | |
| Fresh-install migrations | PASS | Versions 1–701 applied on an empty database |
| Upgrade migration | PASS | An M6-era binary (`c26f8e9`) migrated to 601 and data was created through its API. The current binary upgraded to 701 and preserved all rows, including Bangla text and the pending absence event. An exam was then created, marked, published and graded. A restart re-ran migrations idempotently. |
| Backup and restore | PASS | `pg_dump -Fc` of 27 MB took 2.1 s (1.9 MB file); `pg_restore` took 4.3 s. Row counts were identical across 35 tables, data checksums matched (including photo bytes), and 210 constraints, 108 indexes and 1 trigger were restored. The API on the restored copy returned byte-identical tabulation, report card, monthly report, guardian report and timetable responses. |
| Live security suite (API direct) | 109/110 PASS | The only failure is login throttling, which now lives at the nginx edge and was verified there |
| Concurrency and consistency | 30/30 PASS | Duplicate attendance, QR, admission, user and guardian-link races produced exactly one winner. 100 racing corrections kept absence events consistent. 12 clashing routine periods produced one 201. Publication raced by teacher edits left every mark graded and totals consistent. Locking froze the merit group. Concurrent session activation left one active session. |
| Functional gaps (M1, M3, M4, M6, M7) | 39/39 PASS | Settings and branding, grading validation, exact grade boundaries and the fourth-subject rule through real publication, merit ties, deletion cascade, leave decisions, exam-routine publication |
| Real browser E2E (production build) | 149/149 PASS | 16 admin pages at 1280, 820 and 390 px; five other roles in Bangla at 390 px and English at 820 px; no horizontal overflow, mojibake, unexpected JavaScript errors or HTTP 5xx |
| Accessibility (axe-core 4.10.2, WCAG 2 A/AA) | PASS | 0 violations across 40 page scans (axe verified to detect injected violations). Keyboard-only login works, inputs are labelled, errors use `role=alert`, and the focus ring is visible. Automated rules only; no manual screen-reader session. |
| Report card PDF | PASS | Real PDFs generated with headless Edge and inspected with PyMuPDF. English and Bangla, student and guardian: one A4 page, only the report card printed, Noto Sans Bengali embedded and shaped correctly, GPA 3.88 matches the fourth-subject rule. |
| Multipage PDF | PASS | The 20-period weekly timetable printed to three pages in English and Bangla, and all 20 periods survived the page breaks. The app navigation is also printed (cosmetic). |
| Session expiry and network failure | PASS | A revoked session redirects to login on the next navigation. An offline search shows an error state, and the page recovers after reload. |
| Camera QR scanning | NOT TESTED | Headless Edge on Windows has no `BarcodeDetector`. The page correctly shows the unsupported message, and keyboard/paste QR entry is verified. |

## Security findings without code changes

- **Verified sound:**
  - HS256 is pinned and issuer, audience, expiry and session are required. Tampered, `alg=none`, foreign-key, expired, no-expiry and unknown-session tokens were rejected.
  - Refresh rotation requires a matching CSRF header.
  - Logout and deactivation revoke access immediately.
  - Identical responses for wrong-password, locked and unknown-email logins.
  - All SQL uses bound parameters; SQL payloads were inert.
  - React escapes stored HTML payloads.
  - Photos are sniffed, size- and dimension-capped and re-encoded; an appended script payload was stripped.
  - CSV import is atomic and capped at 1,000 rows.
  - Every module guard holds against direct URLs.
  - Write routes require the admin role even if grants are widened.
  - Responses contain no password hashes, stack traces or version headers.
  - No CORS allowance for foreign origins.
- **Low, open: refresh-token reuse.** A replayed old refresh token is rejected, but the session family is not revoked. Revoking it needs stored previous hashes (a schema change). The refresh token is HttpOnly and SameSite=Strict.
- **Low, open: CSV injection.** Formula-leading names are stored as text. No export exists yet. Neutralise leading `= + - @` when M10 exports are built.
- **Low, open: LIKE wildcards.** Search terms do not escape `%` and `_`. This is not injection, but `%` matches every row.
- **Deployment:** the nginx limit sees the real client only when nginx is the first hop. Behind another proxy or CDN, configure `real_ip` or rate-limit there. HTTPS and Secure cookies remain deployment responsibilities.

## Performance

All numbers were measured with a Node load generator on the same laptop as Docker, so client and server compete for CPU and the figures are conservative. Each step ran for 10 s; mixed runs used weighted role traffic (roster 25%, student cards and timetable 15% each, guardian 15%, teacher writes 10%, admin reads 20%). Latencies are in milliseconds.

| Scenario (concurrency) | Before rps | Before p50/p95/p99 | After rps | After p50/p95/p99 |
|---|---|---|---|---|
| Student search (10) | 56.3 | 150 / 399 / 582 | 238.9 | 35 / 81 / 115 |
| Student search (50) | 49.1 | 996 / 2155 / 2823 | 261.1 | 176 / 360 / 441 |
| Teacher daily roster (50) | 62.9 | 715 / 1993 / 2433 | 233.5 | 201 / 377 / 459 |
| Monthly section report (50) | 37.1 | 1305 / 2927 / 3658 | 144.8 | 334 / 560 / 762 |
| Exam tabulation (50) | 68.2 | 585 / 1945 / 2438 | 235.8 | 184 / 423 / 725 |
| Student report cards (50) | 56.1 | 762 / 2119 / 2610 | 185.8 | 244 / 570 / 672 |
| Guardian child attendance (50) | 77.9 | 560 / 1690 / 2256 | 127.3 | 352 / 888 / 1089 |
| Teacher attendance correction, write (50) | 84.3 | 506 / 1508 / 1977 | 146.5 | 314 / 622 / 769 |
| Mixed, 30 s (10) | 65.0 | 117 / 337 / 668 | 144.6 | 60 / 145 / 220 |
| Mixed, 30 s (50) | 68.8 | 631 / 1631 / 2313 | 199.7 | 236 / 425 / 539 |

- A separate 15 s mixed burst at concurrency 10 rose from 107 to **430 req/s**. In that burst, p95 fell from 192 to 41 ms and new PostgreSQL sessions fell from 858 to 14.
  - The 30 s mixed run above came after eight minutes of sustained load and reached 145 req/s. Laptop CPU throttling is the likely cause of the gap; both figures are measured.
- **Resource use, before:** PostgreSQL CPU had a median of 498% (p95 809%) against an API median of 124%.
- **Resource use, after:** PostgreSQL median 228% (p95 426%) and API median 196%. Peak memory: API 29 MiB, PostgreSQL 177 MiB.
- **Errors:** zero except the test's own 400s. About 4% of correction requests targeted a test-created student with no attendance record (`/attendance/students/null`).
- **Seeding writes:** 10,000 attendance writes at concurrency 8 took 95.6 s before the fix and 34.8 s after.
- **Query profile after the fix:** execution is sub-millisecond on indexed paths, for example the student timetable at 0.6–1.5 ms. The heaviest remaining per-call costs are the student directory count/list joins (5–7 ms) and per-request authentication, grant and module lookups (four queries per request).
- **Next step if needed:** cache grant and module lookups briefly.

## Remaining risks and deployment blockers

1. Remote CI, production migration and deployment were not exercised.
2. HTTPS, Secure cookies and edge proxy configuration are deployment tasks.
3. Camera QR scanning still needs a real device test.
4. Under `-race`, bcrypt-heavy integration tests exceed their fixed deadlines. Raise those deadlines if race runs should be part of CI.
5. `TestPostgresCheckpointFourLifecycle` assumes it is the only active administrator, so the full suite needs a fresh database.
6. A bundle-size warning remains (585 kB).

## Reproducing

From the repository root:

```sh
docker compose -p <unique-name> --env-file <qa.env> up -d --build   # unique ports and credentials
cd backend && go vet ./... && AUTH_TEST_DATABASE_URL=<fresh database> go test ./... -count=1
cd frontend && npm run check:i18n && npm run build
docker build --target production -t flybirds-web ./frontend   # nginx edge with throttling and headers
```

Never point the integration tests at an application database.
