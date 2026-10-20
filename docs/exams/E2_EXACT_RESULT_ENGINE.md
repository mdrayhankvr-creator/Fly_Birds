# E2 — Exact result engine

E2 replaces floating-point grading with exact arithmetic, centralises the
result rules in a pure package, and adds the read-only `exam-verify` tool. It
implements the "E2" milestone of
[E1_IMPLEMENTATION_ROADMAP.md](E1_IMPLEMENTATION_ROADMAP.md#4-recommended-next-milestone--e2-exact-result-engine).
The rules themselves are specified in
[E1_RESULT_CALCULATION_RULES.md](E1_RESULT_CALCULATION_RULES.md).

There is no migration, no API change and no UI change.

## 1. Root cause (E1-01)

`exams.GradeFor` computed the percentage as `marks * 100 / full` in `float64`
and compared it with the integer band edges. Marks have two decimals, which
binary floating point cannot represent exactly, so a mark exactly on an edge
could compute to just under it:

- 16.83 / 51 is exactly 33 %, but computed as 32.999999999999993.
- Over full marks 1–200 and the edges 33, 40, 50, 60, 70 and 80, **66 exact edge marks were graded one band too low.**
- The worst case turns a pass into a fail: a D in a required subject became F, which failed the whole result (GPA 0.00).

Whole-number full marks of 100, 50 and 25 were never affected. That is why
the lifecycle audit's 621 published results all matched an independent
calculation.

## 2. Corrected method

The engine is in `backend/internal/results/calc` and is pure: no database, no clock.

- **Exact values:** marks and grade points are integer hundredths (`Marks`, `Points`). 16.83 is 1683.
  - Stored `numeric(6,2)` values are converted exactly.
  - Anything with more than two decimals is refused.
- **Band lookup** compares integers and never computes a percentage. A mark *m* out of *f* reaches a lower edge *L* when `m × 100 ≥ L × f`. 16.83 / 51 gives 168,300 ≥ 168,300, so it is a D.
  - Upper edges keep today's meaning: 79.99 % is in 70–79, and the band ending at 100 includes 100 %.
- **Pass rule** (unchanged): the marks reach the pass mark, and the band's grade point is above 0.
- **GPA** (unchanged rules, exact arithmetic):
  - min(5.00, (required points + max(0, optional point − 2.00)) / required count);
  - computed as a rational and rounded half up to two decimals;
  - truncation is available as a policy for later milestones;
  - any failed required subject gives 0.00.
- **Totals** are exact sums. Grades, grade points, totals and GPAs are written to PostgreSQL as two-decimal strings cast to `numeric`, so no float reaches a stored value.
- **Ranking:** the SQL `rank()` used at publication is unchanged, because it already orders exact `numeric` values. `calc.Rank` (competition or dense; passed, then GPA, then total) reproduces it and is used by `exam-verify`.

`exams.GradeFor` and `exams.GPA` keep their signatures and delegate to the
package. The publish path uses `exams.Score`, which returns exact totals.

### Before / after

| Marks / full | Exact % | Before (float) | After (exact) |
|---|---|---|---|
| 16.83 / 51, pass mark 16.83 | 33 | F, 0.00, **failed** | **D, 1.00, passed** |
| 16.82 / 51 | 32.98 | F, failed | F, failed (unchanged) |
| 10.20 / 17 | 60 | B, 3.00 | A-, 3.50 |
| 18.40 / 23 | 80 | A, 4.00 | A+, 5.00 |
| 18.39 / 23 | 79.96 | A | A (unchanged) |
| 34.30 / 49 | 70 | A-, 3.50 | A, 4.00 |
| 9.20 / 23 | 40 | D, 1.00 | C, 2.00 |
| 33 / 100, 79.99 / 100, 80 / 100 | — | D, A, A+ | D, A, A+ (unchanged) |

In a running school (section 5), the student with 16.83 / 51 went from
**GPA 0.00 failed** to **GPA 3.00 passed** after republication.

## 3. Compatibility and expected result changes

- **Changes:** only marks lying *exactly* on a band edge, where the float computed a value just below it. The grade moves up one band, to the correct one. That can:
  - turn a failed subject (and result) into a pass;
  - raise a GPA;
  - change merit positions in the class and section.
- **No changes:**
  - whole-number full marks (100, 50, 25 and similar) where float division is exact at the edges;
  - GPA rounding: all 40,032 combinations of grade points for 1–9 subjects, with and without an optional subject, agree with the old formula;
  - pass marks, the fourth-subject bonus, the 5.00 cap and ranking.
- **Published results are not recalculated.** Stored values stay as published until an administrator unpublishes and republishes the exam. `exam-verify` lists which ones would change; OPERATIONS.md §8.1 gives the procedure.
- **Other modules are unaffected:**
  - analytics, exports and notifications read the same columns;
  - analytics' own GPA-to-letter mapping compares stored two-decimal GPAs with half-point band values plus a tolerance, which is exact.

## 4. Future rules, isolated

`calc` also contains the building blocks that later milestones (E5, E6) need:
- contributions (single, sum, average, best-N, latest), with R1–R3 rounding;
- components with separate pass marks;
- combined papers (combined or each-paper pass);
- annual marks (final-only, equal, weighted);
- mark statuses (absent, exempt, incomplete, withheld) with both absent policies.

**Nothing in production calls them,** because their configuration and data
model do not exist yet. Each is tested against the worked examples of the
calculation rules.

## 5. `exam-verify`

`backend/cmd/exam-verify`, shipped in the backend image as `/app/exam-verify`, works as follows:
- **Recalculates** every published or locked result from the stored marks with the exact engine. Optionally it checks one session only.
- **Compares** each stored mark grade, result GPA and pass, total, and class and section position.
- **Classifies each difference:**
  - `boundary_correction` when the pre-E2 float engine reproduces the stored value;
  - otherwise `inconsistent` (for example, bands changed after publication).
- **Outputs** a table or JSON. Exit code 0 means no differences, 3 means differences were found, 1 means an error.
- **Writes nothing:**
  - it runs in one `READ ONLY`, repeatable-read transaction;
  - it never runs migrations;
  - it is tested over a connection on which PostgreSQL refuses every write (`default_transaction_read_only=on`).

**Disposable school run** (`e2-school`, 2026-10-21):
1. Started on `main` (`cdd5d7e4`, float engine), then seeded with the demo school.
2. An exam was published with 16.83 / 51. The old engine stored F, failed, GPA 0.00, position 6.
3. The same school was upgraded to this branch. Fingerprints of `exam_marks`, `exam_results` and `exams` were identical before and after the upgrade, and after running the tool.
4. `exam-verify` reported **2 boundary corrections**, 0 inconsistent, exit 3:
   - the Bangla grade (stored F 0.00 failed, exact D 1.00 passed);
   - the result (stored 0.00 failed, exact 3.00 passed).

   The six seeded demo exams showed no differences.
5. After unpublishing and republishing the exam, the stored grade was D and the GPA 3.00 passed. `exam-verify` then reported **no differences**, exit 0.

## 6. Tests

| Test | What it proves |
|---|---|
| `TestLegacyFloatDefectReproduced` | A verbatim copy of the old engine misgrades exactly the 66 edges recorded in the audit, including 16.83 / 51 → F |
| `TestExactEdgesEverywhere` | Every edge for every full mark 0.01–1000.00 at which the edge is a two-decimal mark: just below, at and just above (over 100,000 edges) |
| `TestReportedBoundaryCases`, `TestCommonFullMarks` | The audit's cases; full marks 100, 50 and 25 around every edge agree with the old engine; irregular full marks (37.50) |
| `TestEquivalenceWithLegacyEngine` | 200,000 random results (biased to edges): 3,598 differ, **every one** because a subject sits exactly on an edge and the exact engine gives the higher band |
| `TestGPARoundingAgreesWithLegacy` | 40,032 GPA combinations: no difference |
| Property tests | Grade monotonic in marks; subject, source and student order never matter; best-N ≥ average |
| Golden examples 1–9 | CT + Final, best-N, sum, latest, R1/R3 rounding, components, combined papers, GPA policies and cap, annual methods, absent/exempt/incomplete/withheld, merit ties |
| `TestExactBandEdges` (exams) | The production wrappers grade 16.83 / 51 as D; exact totals (0.10 + 0.20 = 0.30) |
| `TestPostgresExactPublicationAndVerify` | Publication through the real service stores D and correct positions; `exam-verify` then finds exactly 6 boundary corrections and 1 inconsistency on simulated pre-E2 data, and changes no row |
| `TestPostgresVerifyNeedsNoWrites`, `TestRunsReadOnly` | The verifier and the command work over a write-refusing connection |
| Existing `TestPostgresExamLifecycle`, `TestBangladeshGradesAndFourthSubject` | Unchanged and passing |
