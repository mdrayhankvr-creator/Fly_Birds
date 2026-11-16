# 2026-11-16 — `fix/marksheet-pdf-2`

The showcase still answered "The document could not be prepared" after
`1812` shipped. The first fix was right and is working; what it uncovered is a
second, unrelated fault.

## What was ruled out first

On `fbshow` (read only; the stack was not restarted, rebuilt or changed):

- `SELECT max(version) FROM schema_migrations` → **1812**. The migration is
  applied.
- `result_documents_scope_ck` → `CHECK (scope = ANY (ARRAY['assessment','term','annual']))`.
  The constraint is widened as intended.
- Four rows exist in `result_documents`, scope `assessment`, kind `marksheet`,
  English and Bangla, status `failed` with `attempts = 3`. So the insert that
  used to fail now succeeds and the renderer really runs — which is exactly
  what `1812` was for.

There is no second constraint, the locked state is not involved, and no cached
row is in the way (see the last section).

## The cause

The backend log says it plainly:

```
WARN result document failed  kind=marksheet
  error: render failed: exit status 1: error: number must be positive
    ┌─ marksheet.typ:147:26
    │
147 │   table.cell(colspan: parts.len(), fill: tint, h(L.obtained_marks)),
```

An exam can be run the simple way: **Exams → create an exam → add subjects**,
each marked out of its full marks, with no component breakdown (no CQ, MCQ or
practical) and no blueprint behind it. That is the most common setup in a
school, and it is this school's: on `fbshow`, `exam_subject_components` is
empty and all **108** published subject lines carry **zero** components.

The marksheet is meant to print such a result as totals only — `layoutOf` says
so in as many words ("an exam without components shows totals only") and
returns no component columns. The template did not hold up its end: the
"Obtained marks" heading always spanned `columns.len()` columns, and Typst
refuses a span of zero. So the render died before a single page, the document
row ended `failed`, the API answered 200 with that status, and the page showed
its generic message.

It was never the logo, the photos, the locked state or the data of this one
student. Every marksheet of every component-less exam was unprintable.

## The fix

`backend/internal/documents/templates/marksheet.typ`: with no component
columns the marks table's header is one row and the "Obtained marks" band is
left out altogether. With component columns nothing changes — two header rows,
the band spanning them.

## The regression test

`backend/internal/documents/marksheet_template_test.go`:

- `TestPlainAssessmentHasNoComponentColumns` pins the data contract the
  template has to cope with: a component-less assessment set yields no
  columns, no parts on any row, the marks still in the subject totals, and a
  payload carrying `"columns":[]`, not null.
- `TestMarksheetTemplateRenders` **compiles the real template** with Typst,
  which is the only way to catch a mistake in it, for three shapes: no
  component columns, component columns, and a class-test column alone. It
  skips when no Typst binary is to hand (`TYPST_BIN`, `DOCUMENT_FONTS_DIR`),
  because a fake renderer would prove nothing about a template.

Run against the version of Typst the backend image pins (0.15.1, in a
throwaway `golang:1.26.11-alpine` image with that binary copied in):

- **without** the fix it fails with the showcase's error, the same line and
  the same message;
- **with** the fix it passes.

`e2e/marksheet-plain-exam.mjs` covers the whole path on a running stack:
create the exam with no components, mark the section, publish, download the
staff marksheet in English and Bangla as one-page PDFs, check the student's
published version, **lock the exam and download again**, and do a second
student. It is now part of CI's stack job.

## Why no test caught it

Every browser suite that downloads a marksheet builds its exam from a
blueprint, and a blueprint sheet must have at least one component — the
validator refuses `no_components`. So no check had ever rendered a marksheet
for an exam without them, although that is the simpler and more common way to
run one. The new suite is what closes that.

## Checks

- `go vet ./...` clean; the new files are gofmt-clean.
- Full `go test ./... -p 1 -count=1` on a fresh database (`msfix2` in
  `m15-testdb`): **30 packages ok, 10 with no test files, 0 failures**,
  exit 0, no out-of-memory.
- `TestMarksheetTemplateRenders` with the pinned Typst: fails before the fix
  with the exact showcase error, passes after.
- On `m15b1`, before rebuilding (so with the old template),
  `e2e/marksheet-plain-exam.mjs` reproduced it: **4 failures**, every one
  `number must be positive` at `marksheet.typ:147`, for the published exam,
  the locked exam, both languages and two students. After rebuilding the
  backend from this branch: **12 of 12 checks passed**.
- On `m15b1`, recreated and seeded again so the suites had the freshly
  seeded school they expect: `e2e/result-publication.mjs` **101 of 101
  checks passed**, `e2e/result-pack.mjs` **27 of 27 checks passed**, and
  `e2e/marksheet-plain-exam.mjs` **12 of 12** with two one-page PDFs of
  about 193 KB each.

  A first attempt at the pair, on a school that several suites had already
  run against, is not counted: `result-publication` ended when the browser's
  own connection dropped, after passing everything it had reached, and
  `result-pack` failed two admit-card checks with 400 because the newest
  term with a published routine was the one `marksheet-admit.mjs` had left
  behind, which belongs to another class. On the freshly seeded school
  `result-pack` reports that no term has a published routine and skips that
  section, which is what it is meant to do.

## `fbshow` needs no command of its own

- No data to clean up. A `failed` document row is set back to `queued` with
  `attempts = 0` the next time the same document is asked for
  (`documents.Service.request`, the `ON CONFLICT … DO UPDATE`), so the four
  failed rows retry themselves.
- No new migration, no environment change. Deploying the merged commit and
  restarting is all of it, the same ordinary update as last time.
- `fbshow` was not restarted, rebuilt or changed while finding this, and its
  data was only read.
