# E6 — Result versions, review, publication, corrections and annual results

Status: implemented and verified locally on
`feature/e6-result-versioning-snapshots` (2026-10-23), from `main`
`7b3723f5`. Migration 1503.

E6 follows the E6 row of
[E1_IMPLEMENTATION_ROADMAP.md](E1_IMPLEMENTATION_ROADMAP.md): "`result_versions`
and snapshots; `exam_results` projection; review screen; publication centre;
correction workflow; annual results (equal, weighted, final-only)", with the
acceptance criteria "every published result has a reproducible snapshot;
corrections supersede, never delete; annual examples match". The design
follows the [blueprint specification §3.7, §5, §7](E1_ASSESSMENT_BLUEPRINT_SPEC.md),
the [calculation rules §3.6, §6, §7, §9](E1_RESULT_CALCULATION_RULES.md) and the
[UX specification §9–§12](E1_PREMIUM_UX_SPEC.md). §9 lists where it differs.
The E6 prompt matches the roadmap; report-card PDFs, also mentioned in the UX
specification, are E7.

## 1. What changes for a school

- **Published results never change.** Every published result is an
  immutable *version* with a snapshot of the student, the rules, the marks
  it used and the values published. A grading-band change, a new blueprint
  version, an optional-subject change or a mark correction does not touch
  it. Report cards are read from the snapshot, never from the live marks.
- **Exams** are published from the exam page as before. Each student's
  result becomes version 1.
  - **Withdraw results** (the old "Return to draft") now needs a reason and
    keeps the versions as *withdrawn*; publishing again adds the next
    version. An exam whose results were ever published can no longer be
    deleted.
  - When another section's results move a student's **class position**, the
    student gets a *position update* — a new version with only the
    positions changed — never an edit in place.
- **Corrections.** An administrator opens a correction for one student of a
  published (or locked) exam, with a reason. Only that student's marks can
  then change, only by an administrator, beside the published values. The
  head teacher (or an administrator) approves the revision; an
  administrator publishes it. Only the students whose result changes get a
  new version (including class positions that move); the previous version is
  *superseded* and stays readable. Cancelling a correction puts the
  published marks and grades back.
- **Term results** of a class (all sections together) are reviewed on the
  *Results & publication* screens, approved by the head teacher and
  published by an administrator — about three clicks from the centre. The
  term exam's own result becomes visible to students when the term result is
  published ("after the term result", E5's visibility).
- **Annual results** are made from the published term results with the
  class blueprint's method (final only, equal, weighted; optional "must pass
  the final term"), reviewed, approved and published the same way.
- **Students and guardians** see published term and annual results (GPA,
  outcome, subjects, the annual method), marked *Revised* with its date when
  a version superseded an earlier one, and "Result withheld — please contact
  the school office" with no marks for a withheld result. Class-test marks
  and contributions never reach them.

## 2. Architecture

### Database (migration 1503)

| Table | Purpose |
|---|---|
| `result_publications` | One batch of one scope (`assessment` = an exam, `term`, `annual`) for one class: a `publication`, `correction` or `position_update`. Status `approved` (waiting; binds to `inputs_sha256`), `published` (with its `revision`) or `discarded`. Holds the `rules` (bands, policy, definition, annual method) once for the batch, with `rules_sha256` |
| `result_versions` | One student's result in one scope, `version` 1, 2, … Status `published` (current), `superseded` or `withdrawn`. Outcome, totals, GPA, positions, `snapshot` (jsonb) and `inputs_sha256`, the reason, who published and who closed it, `supersedes_id` |
| `result_corrections` | An opened correction: exam, student, reason, `open` / `applied` / `cancelled`, the publication that applied it |
| `exam_results.result_version_id` | The projection row of an assessment points at its version |

**Constraints and triggers (database-enforced, not only in Go):**
- one current version per student and scope (partial unique index), one
  approval waiting per scope and class, unique `(scope, scope_id, student, version)`
  and `(scope, scope_id, class, revision)`;
- a published version can only be closed once (published → superseded or
  withdrawn); any other update and every delete is refused;
- a published or discarded publication never changes; none is deleted;
- the inputs and rules hashes are computed by the database from the stored
  jsonb, so every version hashes the same way;
- an exam or term with versions cannot be deleted;
- `exam_visible_to_students()` treats "after the term result" as visible
  once the class's term result is published.

### Snapshot

```text
snapshot = { format: 1, source: engine | migration, scope,
  student: { id, names, admission and roll no, section, class and session (ids and names) },
  context: { exam, term (name, sequence) or annual method },
  inputs:  assessment → every mark used (sheet, status, marks, full and pass marks, components, row version)
           term       → marks by assessment and subject, the subjects not taken
           annual     → each term's published version (id, number, outcome, subject lines),
  outputs: { outcome, total, full, GPA, positions, subject lines (marks, grade, point,
             passed, fail reasons, components; term contributions; annual term shares) } }
```

The rules are on the publication (`rules`, `rules_sha256`). `inputs_sha256`
is the SHA-256 of the stored inputs. **Reproducibility:** recalculating a
version from its inputs and its publication's rules gives exactly its
outputs (positions, which depend on the whole class, are checked per
publication). Staff see this check on every version ("Verified").

### Backend

| File (`internal/exams`) | Contents |
|---|---|
| `versions.go` | Snapshot types, the batch hash, storing, closing and comparing versions |
| `assessment.go` | Exam results as versions: rules, candidates, ranking across sections, position updates, publication, withdrawal |
| `scopes.go` | Term and annual batches and recalculation from a snapshot |
| `review.go` | Review (computed now, beside what is published), approval bound to the inputs hash, publication |
| `corrections.go` | Opening, editing (through the existing mark PATCH), cancelling with restore |
| `published.go`, `history.go` | Report cards from snapshots, family term/annual results, version history and detail, the publication overview |

`internal/blueprints` gains the annual calculation (`annual.go`) and one
term loader (`termdata.go`) shared by the preview and publication.

**Calculation.** Exact throughout, reusing `results/calc`:
- **assessment results:** E2/E3/E5 scoring, unchanged;
- **term results:** E5's engine; blueprint versions grade with their
  activation band snapshot (E5 deferred this pinning to E6), legacy versions
  with the school's bands, as their exams publish;
- **annual results:** per subject at subject-percentage level — final only,
  equal or weighted (Σ w·p / Σ w, `math/big.Rat`), rounded once to two
  decimals (R5) and to whole marks when the school chose R3, graded with the
  final term's pass mark; "final term must pass" fails the subject otherwise.
  A term without the subject (an optional subject chosen later) or with the
  subject exempt leaves the average; a withheld term withholds the annual
  result; a missing published term result makes it incomplete;
- **corrections reuse the rules of the original publication**, so a
  correction never regrades the class under different bands.

### API (10 new routes, all `RequirePermission` + the examinations module)

| Route | Permission |
|---|---|
| `GET /exams/results/overview`, `GET /exams/results/review`, `GET /exams/results/history`, `GET /exams/results/versions/:id`, `GET /exams/:id/corrections` | `exams:read` (administrator, head teacher) |
| `POST /exams/results/approve` | `exams:review` and the head teacher or an administrator |
| `POST /exams/results/publish`, `POST /exams/:id/corrections`, `POST /exams/:id/corrections/:cid/cancel` | `exams:manage` and an administrator |
| `GET /exams/me/term-results`, `GET /exams/children/:studentId/term-results` | `exams:read_self` / `exams:read_children` (linked guardian) |

`PATCH /exams/:id/state` takes a withdrawal `reason`. Errors have their own
codes: `results_not_ready`, `review_stale`, `approval_stale`,
`results_not_approved`, `nothing_to_approve`, `already_published`,
`results_not_published`, `reason_required`, `exam_has_versions`,
`correction_required`. All routes are in `docs/openapi.yaml`.

### Workflow and concurrency

```text
term / annual:  computed (review) ─► approved (head teacher, binds to inputs_sha256) ─► published (admin, revision n)
                                        └─ anything changes ─► approval stale ─► approve again
exam:           published (exam page; E3 sheet review is the review) ─► correction opened ─► marks changed
                ─► reviewed ─► approved ─► published revision (changed students only) ─► …
                └─► withdrawn (versions kept) ─► published again (next version)
```

Every state change runs in one transaction under the exams lock (and the
exam's row lock for an exam). Publishing re-computes and compares the
inputs hash with the approval; two concurrent publications of one approval
publish once (the second gets `already_published`); the unique indexes stop
duplicate current versions even if a lock were bypassed.

**Audit events:** `exam.results_published`, `exam.results_withdrawn`,
`exam.results_approved`, `exam.result_corrected`,
`exam.locked_result_corrected`, `exam.result_positions_updated`,
`exam.correction_opened` (`exam.locked_correction_opened`),
`exam.correction_cancelled`, `exam.result_versions_migrated`; mark changes
under a correction carry `correction_id` in `exam.mark_saved`.

## 3. Screens (`frontend/src/pages/exams/`)

| Screen | Route | Who |
|---|---|---|
| `ResultsCentre.tsx` — Results & publication | `/admin/exams/results` | administrator, head teacher |
| `ResultReview.tsx` — review, approval, publication | `/admin/exams/results/review?scope=…` | administrator, head teacher (approve) |
| `ExamCorrections.tsx` — corrections on the exam page | `/admin/exams` (a published exam) | administrator; head teacher reads |
| `PublishedResults.tsx` — term and annual results | `/student/results`, `/guardian/children/:id/results` | student, guardian |
| `resultUi.tsx` | Shared: state chips, provisional frame, tabulation, before/after table, version history with the snapshot check | |

Calm Ledger rules of the UX specification: one primary action, states as
words + colour, provisional output framed and labelled, tabular right-aligned
numbers, the student column pinned on wide tables and one card per student
on phones. 202 new keys in English and Bangla (`resultTranslations.ts`),
dark-theme tints for the provisional (indigo) and revised (violet) states.

## 4. Security and privacy

- **Students and guardians** read only their own current published versions:
  no drafts, no provisional values, no superseded or withdrawn versions, no
  class-test contributions or keys (removed on the server), no other
  student's results (a guardian gets 404 for a student not linked). A
  withheld result shows only the status.
- **Teachers** reach none of the new routes. A published mark can be changed
  only by an administrator, only for a student with an open correction
  (`correction_required` otherwise); a teacher gets `exam_not_draft`.
- **Head teachers** review and approve; they cannot publish or correct.
- **Administrators** publish and correct; a single-administrator school can
  approve and publish, and the audit shows both steps.
- **Immutability** is enforced by triggers; **every action is audited** (§2).

## 5. Upgrade (no value changes)

Migration 1503 records every result already published as version 1 from the
stored marks, grades and results. Nothing is recalculated; the rules record
the school's current bands and the exam's policy (`source: migration`). The
audit event `exam.result_versions_migrated` records the counts.

**Measured on a disposable copy of a full E5 regression school** (a school
built from `main` `7b3723f5`, its database replaced by a copy of the E5
regression school `e5-fin2`, which was only read):

- **Data:** 19 exams (9 published), 153 results, 1,716 marks, 200 component
  marks, 1 term with its blueprint, 100 optional-subject choices, 268
  students, 73 fee payments.
- **Fingerprints** of every mark, result, sheet, exam, component mark, term,
  blueprint version, choice, student, account, fee payment, due and ledger
  entry are **identical** before and after.
- **Recorded:** 9 publications, 154 versions (one withheld); all 153
  projection rows linked; every inputs hash verifies.
- **Every report card** (153 staff, 3 student, 3 guardian) shows identical
  values before and after. Only the order of a student's list changed: a
  withheld result is now sorted by date with the others instead of last.
- **`exam-verify`:** no differences before or after (9 exams, 153 results,
  680 marks).
- **After the upgrade** the E3, E4, E5 and E6 browser suites pass on the
  same school (§7).

Rollback is the pre-upgrade backup (DEPLOYMENT.md §8), as for every
milestone with a migration.

## 6. Measurements (one laptop, disposable school, not production)

| Flow | Result |
|---|---|
| Publish a reviewed and approved class term result from the centre | 3 clicks (UX target 3; the E1 audit counted 40 for 20 sections) |
| Review screen with 12 students | 0.2–0.6 s to the tabulation |
| Review screen with 100 students (two sections) | 0.8–1.2 s to the tabulation |
| Request failures in the E6 suite | 0 |

## 7. Verification (2026-10-23, disposable resources only)

- **Go:** `go vet` clean; `go test ./... -p 1 -count=1` passes on a fresh
  database; govulncheck finds nothing reachable. New tests:
  - `blueprints`: annual Example 7 of the calculation rules (84.50 A+, 74.17
    A, 76.75 A), the rounding sensitivity (79.60 → 80 with whole marks), final
    term must pass, missing, withheld and incomplete terms, exact parsing;
  - `exams` `TestPostgresResultVersions`: versions on publication, position
    updates across sections, immutability triggers, term review → approval →
    publication, double publication refused, band change and new blueprint
    version leave published results and the review unchanged, an optional
    subject change blocks approval but changes nothing published, a
    correction (teacher refused, other students refused, stale approval
    refused, superseded version readable and reproducible, student sees the
    revised card), the term revision for changed students only, withheld and
    incomplete results, cancelling restores marks and grades, `exam-verify`,
    the annual result, the overview, withdrawal and republication;
  - `exams` `TestPostgresResultVersionsMigrate`: the upgrade step records the
    stored values, changes nothing, cards match, verification passes, a
    second run adds nothing;
  - `httpapi`: route protection for every new route and role; OpenAPI
    coverage.
- **Frontend:** build and type check, i18n (2,574 keys per language), 11 unit
  tests.
- **Browser:** see PROGRESS.md for the full list. `e2e/result-publication.mjs`
  (new, 101 checks) covers §1 end to end, concurrency, permissions and four
  screens at 1440, 768 and 390 px, in English and Bangla, light and dark,
  with three school colours, WCAG AA contrast and axe.

To repeat on a disposable school:

```sh
E2E_BASE_URL=http://localhost:8112 DEMO_PASSWORD=... node e2e/result-publication.mjs
```

## 8. Operations

See OPERATIONS.md §8.5. `exam-verify` now checks each exam against the bands
it was published with and skips students with an open correction.

## 9. Differences from the E1 design

| E1 design | E6 | Why |
|---|---|---|
| `result_versions.status` includes `computed` and `approved` | Computed results are not stored; an approval is a `result_publications` row bound to the inputs hash; versions are only published, superseded or withdrawn | Nothing provisional is ever stored as a result, and an approval cannot drift from what was reviewed |
| Publishing a term writes `exam_results` | Only assessment versions project into `exam_results`; term and annual results live in versions | A term spans several exams; analytics keep reading exam results unchanged |
| Every scope is approved by the head teacher | An exam's first publication stays the exam page's Publish (the E3 sheet review is its review); its corrections, and every term and annual result, need approval | Keeps E3's workflow and the existing suites |
| Class position "pending" until all sections publish | A position update (new version) when another section's results move it | A published value never changes in place, and positions stay complete for analytics |
| The teacher edits the reopened sheet | An administrator edits under the correction | Smaller trust surface; the head teacher still approves |
| Annual `level: gpa` option, "do not rank failed students" | Not offered | Not needed by the configured schools; the averaged-GPA risk is documented in the rules |
| Report-card PDFs regenerated per version | E7 | The document renderer is E7 |
| `inputs_sha256` over a Go canonical encoding | The database hashes the stored jsonb; approvals bind to a Go canonical batch hash | One definition for every version, including those recorded by the upgrade |

## 10. Limitations

- Revisions do not notify families again (the first publication does).
- A position update also shows *Revised* to the family.
- Only exams can be withdrawn; a published term or annual result is changed
  only through a correction (by changing marks) — there is no term-level
  "withdraw" or "lock" yet.
- A student who joins mid-year has no result for earlier terms, which makes
  the class's annual result incomplete until resolved.
- The annual method is the class's active blueprint version's; activating a
  new version before the annual publication uses its method.
- PDFs, bulk reports and the Examination Center are E7–E9.
