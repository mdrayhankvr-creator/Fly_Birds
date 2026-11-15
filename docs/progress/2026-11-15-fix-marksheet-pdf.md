# 2026-11-15 — `fix/marksheet-pdf`

A marksheet of an exam's own published result could not be made. On the
showcase (`fbshow`, port 8094, built from `main`), a published result in the
student view answered both "Download PDF (English)" and "Download PDF
(বাংলা)" with "The document could not be prepared. Please try again."

## The cause

`result_documents` (migration `1800`) was added when result documents covered
term and annual results only, so its scope column carries
`CHECK (scope IN ('term', 'annual'))`. Transcript v3 later added marksheets
for an exam's **own** published result — the `'assessment'` scope, which
`result_versions` and `result_publications` (`1503`) have allowed from the
start — and the Go code supports it end to end:
`Request.normalize` admits it for `marksheet` and `marksheets`,
`StudentMarksheet` passes the version's own scope straight through, and both
`extras.go` and `marksheet.go` have `assessment` branches.

The check on `result_documents` was never widened. Every such request died on
the `INSERT` in `documents.Service.request` with

```
ERROR: new row for relation "result_documents" violates check constraint
"result_documents_scope_check" (SQLSTATE 23514)
```

A constraint violation is not one of the service's sentinel errors, so
`documentError` fell through to its default and answered **500**, which the
frontend shows as `e7Err_generic`. Nothing was ever rendered: the failure is
before `process`, which is why the backend logged no "result document failed"
warning — only bare `500 POST /exams/documents` lines.

It was never the renderer. Typst is a binary inside the backend image (there
is no separate renderer service, and `fbshow` is not missing one), the
`Available()` guard passed — a missing binary or font directory answers 503,
not 500 — and the logo fetcher swallows its own errors and falls back to the
school's initials. The first failure took 1.01 s and the rest 32–80 ms, which
is the logo fetch being attempted once and then served from its
ten-minute failure cache while the constraint kept rejecting the row.

Evidence from `fbshow` (read only; the stack was not restarted, rebuilt or
changed, and its data was not touched): seven `500 POST /exams/documents`
between 21:56:31 and 21:56:45 on 2026-11-13, each directly after
`GET /exams/{examId}/results/{studentId}` — an exam's own result, so scope
`assessment`. No panic, no recovery trace, no renderer warning.

## The fix

`backend/internal/database/migrations/1812_result_documents_assessment_scope.sql`
drops the old check and adds `CHECK (scope IN ('assessment', 'term', 'annual'))`.

Widening the check is the whole fix. The row's other constraints already fit
an exam-scope marksheet: `result_documents_student_ck` wants `student_id` set
for `kind = 'marksheet'`, and `session_id` is the exam's session via
`sessionOf`. No stored row changes, because no `assessment` document could
ever be saved. Class documents (tabulation, merit list, failed list, summary)
stay term- and annual-only, which `normalize` enforces and the test asserts.

`result_comments` carries the same `('term', 'annual')` check and was left
alone on purpose: `documents/extras.go` only *reads* it, and the write path
(`classteacher.sessionOfScope`) already refuses an assessment scope with a
clean `ErrInvalid` → 400.

## The regression test

`backend/internal/documents/marksheet_assessment_integration_test.go`
(`TestPostgresAssessmentMarksheet`) covers the staff path and the student's
own download in English and Bangla, the file coming back as a PDF through
`Open`, the cached second request rendering nothing new, another student's
exam result staying `ErrNotFound`, and class documents still refused for an
exam scope.

Reproduced before fixing: with `1812` held back, the test fails with exactly
the constraint violation above, which also confirmed the auto-generated
constraint name the migration drops. With `1812` in place it passes on a
fresh database **and** on the database that already had the old constraint,
so the upgrade path is covered too.

## Checks

- `go vet ./...` clean. `gofmt` clean on the new file (the 260 files
  `gofmt -l` lists repo-wide are a pre-existing CRLF artifact, untouched here).
- Full `go test ./... -p 1 -count=1` on a fresh database (`fixms_v2` in
  `m15-testdb`): **30 packages ok, 10 with no test files, 0 failures**,
  exit 0, no out-of-memory. An earlier attempt was void — the host ran out of
  RAM mid-run and several packages died with `fatal error: out of memory`;
  that run proved nothing and was rerun alone.
- Migration timing: fresh database 3.45 s; upgrade of an existing
  `m15b1` database 252 ms, no restarts, ledger at `1812`, and the constraint
  verified as `('assessment', 'term', 'annual')` afterwards.
- On `m15b1` rebuilt from this branch (both images) and demo-seeded:
  `e2e/result-publication.mjs` **101 of 101 checks passed**, axe clean, no
  console errors, then `e2e/result-pack.mjs` **27 of 27 checks passed**
  against the term it published. Both exited 0 with no failures.

## Notes

- **No change is needed on `fbshow` itself** — no missing env, no missing
  service. It picks `1812` up from the ledger on its next ordinary restart,
  the way `m15b1` did in 252 ms. The showcase was deliberately left running
  and untouched.
- `m15b1`'s database was recreated inside its existing Docker volume (the
  volume was kept) so the demo accounts could be seeded with a known
  password; the previous session's password was not recorded and
  `seed-demo` leaves an existing account's password alone. Agreed before
  doing it.
- The laptop went down twice during this work. Nothing was taken from a
  partial run: the first full Go suite died of host out-of-memory and was
  rerun from a new database, and the e2e suites were rerun or resumed only
  where a script had printed its own final count.
- Scratch databases used for this work (`fixms_repro`, `fixms_fix`,
  `fixms_full`, `fixms_v2`) were dropped afterwards; `m15-testdb` holds only
  its own `m15` database again.

## What is not covered

- No browser check of the exact showcase screen: the fix is verified by the
  Go test on both a fresh and an already-migrated database, and by the
  constraint and ledger on `m15b1` after the upgrade, not by clicking
  "Download PDF" on `fbshow` — which was left untouched on purpose.
- `e2e/result-documents.mjs` and `e2e/marksheet-admit.mjs` download a
  student's marksheet of a **term** result, so neither exercised this path;
  that is why the bug reached the showcase. The new Go test is what closes
  the gap.
