# E1 — Result calculation rules

The result engine turns marks into results. It must be **exact, deterministic
and explainable**: the same inputs always give the same outputs, every number on
a report card can be traced to marks and rules, and no step uses binary
floating point. Data structures are in the
[blueprint spec](E1_ASSESSMENT_BLUEPRINT_SPEC.md).

## 1. Arithmetic

1. **Stored values:** marks are stored as `numeric(6,2)` and handled in code as integer hundredths ("centimarks"). 16.83 is 1683.
2. **Exact intermediates:** derived values (scaled contributions, averages, weighted sums) are exact rationals (Go `math/big.Rat`; no new dependency). They are rounded only at the points in section 2.
3. **Exact grade lookup:** a percentage is never computed for comparison. A mark *m* out of *f* reaches a band with lower edge *L* % when `m × 100 ≥ L × f`, comparing integers. This removes defect E1-01: 16.83 / 51 gives 1683 × 100 = 168,300 and 33 × 5,100 = 168,300, so it is exactly at the D edge and passes.
4. **Rounding mode:** where rounding is configured, it is **half up** (towards +∞ at exactly .5) on non-negative values, unless the school chooses truncation.
5. **No time dependence:** results depend only on marks, statuses and the rule snapshot, never on the clock or the order rows were read. Every list is sorted by a stable key before calculation.

## 2. Rounding points (school policy, recorded in each result)

| Point | Options | Default |
|---|---|---|
| R1 — each contribution (scaled CT, scaled term exam) | none; 2 decimals | none |
| R2 — subject mark used for grading and printing | 2 decimals (always: displayed = graded) | 2 decimals |
| R3 — whole-mark rounding before grading | none; to whole marks | none (today's behaviour: 79.99 → A) |
| R4 — GPA | 2 decimals half up; 2 decimals truncated | half up (today's behaviour) |
| R5 — annual subject mark | 2 decimals | 2 decimals |

Rounding happens once per point, never repeatedly. Displayed values are always the values that were graded.

## 3. Pipeline

For each student and each scope:

```text
component marks ──► assessment subject mark ──► term subject mark ──► combined subject
      (sum)              (per assessment)        (contributions)        (papers)
                                                                           │
                                     annual subject mark ◄─────────────────┤
                                     (equal / weighted / final-only)       │
                                                                           ▼
                                     subject grade, point, pass ──► GPA, outcome, positions
```

### 3.1 Assessment subject mark

- **Value:** the subject mark for one assessment is the sum of its components.
- **Component pass:** a component with a pass mark passes when its mark ≥ that pass mark. An absent component scores 0 and fails, unless the school chose "absent = missing".

### 3.2 Term subject mark (contributions)

For each contribution with sources *S*, combination *c* and worth *w*:

| Combination | Value of the contribution |
|---|---|
| single | `w × m / f` of the one source |
| sum | `w × Σm / Σf` |
| average | `w × (Σ (m_i / f_i)) / n` — the average of percentages, so CTs out of 20 and 25 weigh equally |
| best N | the N sources with the highest `m / f`, ties broken by assessment order, then averaged as above |
| latest | the last source in assessment order that is not exempt |

**Exempt and absent sources:**
- An **exempt** source leaves the set, and the average or best-N uses the remaining sources. If none remains, the contribution itself is exempt and the subject's full marks shrink by *w*.
- An **absent** source counts as 0 (default policy).

The term subject mark is the sum of contributions, then R2 (and R3 if enabled).

### 3.3 Combined subjects (1st and 2nd papers)

- **Marks:** combined marks = Σ paper marks; combined full = Σ paper full marks. The grade comes from the combined marks.
- **Pass rule (school option):**
  - **combined:** pass when the combined mark reaches the pass percentage;
  - **each paper:** additionally every paper ≥ its own pass mark.

### 3.4 Subject grade and pass

1. Find the band with exact integer comparison (section 1.3) on the subject mark.
2. The subject **passes** when all of these hold:
   - the band's point is > 0;
   - the mark ≥ the subject pass mark;
   - every component with a pass mark passes;
   - for combined subjects, the combined pass rule holds.
3. A failed subject's grade is shown as **F** with point 0.00, even if its total alone falls in a higher band. A subject that fails only on a component shows its total with grade F and the reason "CQ below pass mark".

### 3.5 GPA and outcome

1. **Required subjects:** *R* is the set of required (and not exempt) subjects; *P* is the sum of their points.
2. **Optional subject (at most one per student):** bonus *B* = max(0, point − 2.00). An optional subject never fails the student.
3. **GPA:**
   - when every required subject passes: `min(5.00, (P + B) / |R|)`, rounded by R4;
   - otherwise 0.00.
4. **Outcome:**
   - **passed** when every required subject passes;
   - **failed** when any fails;
   - **incomplete** when any input is Incomplete or missing;
   - **withheld** when any input is Withheld.
   Incomplete and withheld results are not ranked and cannot be published until resolved. A withheld result can be published as "withheld" (no marks shown).

These rules reproduce the current engine exactly for single-assessment results with whole-number full marks (verified on 621 lifecycle results).

### 3.6 Annual result

Computed per subject first (recommended `level: subject_percent`):

| Method | Annual subject percentage |
|---|---|
| final_only | the last term's subject percentage |
| equal | the average of the term subject percentages |
| weighted | Σ (weight × term percentage) / 100 |

- **Annual mark:** the result is expressed on the subject's annual full marks (normally 100) and rounded by R5, then graded with section 3.4.
- **Extra pass rule:** optionally, the subject must also pass in the final term (`final_must_pass`).
- **Annual GPA and outcome:** these follow section 3.5.

*Why not average GPAs?* Averaging GPAs hides failure: a GPA of 5.00 and a failed term (0.00) average to 2.50, which looks like a pass. The `gpa` level is available for schools that require it, and it is labelled as such on the report card.

## 4. Readiness and missing marks

A result scope (assessment, term or annual, for one class) is **ready** when:
1. every sheet it depends on is `approved`;
2. every active student has a status in every sheet;
3. no outcome is `incomplete`.

**Computation runs automatically** whenever an approved sheet changes, producing `computed` versions as a preview for staff. Students never see these.

**Publication is refused** (409) unless the scope is ready. The readiness panel lists the blocking sheets and students by name.

## 5. Merit ranking

**Rank key (default, as today):** outcome passed before failed, then GPA descending, then total marks descending.

**Tie policy (school option):**
- **competition** (1, 1, 3; today's behaviour);
- **dense** (1, 1, 2).

No arbitrary tie-breaker (such as roll number) is applied. Tied students share the position.

**Scopes:**
- **Section position:** within the section.
- **Class position:** across all sections of the class for the same term and assessment, joined by `term_id` and `assessment_key`, not by exam name (closes E1-05).

**Not ranked:** incomplete and withheld results. Failed students are ranked after all passed students, unless the school chooses "do not rank failed students" (position shown as "—").

**Display:** the school may hide positions on report cards. Positions are still computed for staff.

## 6. Approval and publication

```text
sheet:   not_started → in_progress → submitted ─┬─► approved
                                   ▲             └─► returned (reason) ─┐
                                   └─────────────────────────────────────┘
result:  computed ──► approved (head teacher) ──► published (admin) ──► superseded (by a correction)
                                                     └──► locked (no further correction except by a new version with reason)
```

- **Sheet submission:** the teacher submits; the head teacher (`exams:review`) approves or returns it with a reason. Approved sheets are read-only for the teacher.
- **Result approval:** approving a computed scope records who approved it.
- **Publication:**
  - publishing makes the versions visible, writes the `exam_results` projection, sends result notifications (existing mechanism) and starts report-card PDF generation;
  - all students of a scope publish together in one transaction;
  - class positions need all sections of the class published together, or the class position stays "pending" until they are.

## 7. Corrections after publication

1. **Open:** an admin opens a correction for one student and subject with a required reason.
2. **Edit:** the affected sheet reopens for that student only. The teacher or admin edits, and the head teacher approves.
3. **Recompute:** the engine recomputes the scope, which is needed because positions can change.
4. **New versions:** every student whose outputs changed gets a new `computed` version, with the reason "correction of …" or "position changed by a correction".
5. **Publish:** the approved versions publish. The previous versions become `superseded` and stay readable to staff. The student sees the new version marked **Revised** with its date.
6. **Documents:** report-card PDFs are regenerated only for students whose version changed. Each new PDF carries the new version number; old PDFs are kept for staff and never overwritten.

**Locked scopes:** a locked scope (e.g. after the year closes) can be corrected only by an admin with a reason, and the action is audited as `exam.locked_result_corrected`.

## 8. Worked examples

Default policy unless stated: no R1/R3 rounding, R2 to 2 decimals, GPA half up, `bd_default` bands.

### Example 1 — CT 20 % + term exam 80 % (School A, Mathematics)

| Input | Value |
|---|---|
| CT 1, CT 2 | 17 / 20, 14 / 20 |
| Term exam | 68 / 100 |

1. CT contribution: average of 85 % and 70 % = 77.5 % → 20 × 0.775 = **15.50**
2. Term exam contribution: 80 × 0.68 = **54.40**
3. Term subject mark: 15.50 + 54.40 = **69.90 / 100**

| Policy | Grade | Point |
|---|---|---|
| R3 none (default) | A- (69.90 < 70) | 3.50 |
| R3 whole marks | 70 → A | 4.00 |

The rounding policy changes the grade, so it must be a recorded school choice.

### Example 2 — Best two of three CTs (School B)

| Input | Value |
|---|---|
| CT 1, 2, 3 | 12, 18, 15 (each / 20) |
| Term exam | 70 / 100 |

- **Best two:** 18 and 15 → (90 % + 75 %) / 2 = 82.5 % → 20 × 0.825 = **16.50**
- **Term exam:** 80 × 0.70 = **56.00**
- **Subject mark:** **72.50** → A (4.00)

For comparison, the plain average of all three CTs is 75 % → 15.00, giving 71.00 → also A.

### Example 3 — Component failure despite a high total (School D, Physics)

| Component | Marks | Pass mark | Passed? |
|---|---|---|---|
| CQ | 16 / 50 | 17 | **No** |
| MCQ | 25 / 25 | 8 | Yes |
| Practical | 25 / 25 | 8 | Yes |
| **Total** | **66 / 100** | 33 | — |

- 66 % alone would be **A- (3.50)**.
- CQ is below its pass mark, so **Physics fails: grade F, point 0.00**.
- Physics is required, so the student's **GPA is 0.00 and the outcome is failed**, whatever the other subjects score.
- The report card shows "Physics 66 — F (CQ below pass mark)".

### Example 4 — Combined papers (Bangla)

| Paper | Marks |
|---|---|
| Bangla 1st paper | 58 / 100 |
| Bangla 2nd paper | 30 / 100 (pass 33) |
| Combined | 88 / 200 = 44 % → C (2.00) |

| School pass rule | Bangla result |
|---|---|
| combined | Passes (44 % ≥ 33 %): C, 2.00 |
| each paper | Fails (2nd paper 30 < 33): F, 0.00 → the student fails |

### Example 5 — GPA with an optional (fourth) subject

| Required subject grades (6) | A+, A+, A, A, A-, B → points 5 + 5 + 4 + 4 + 3.5 + 3 = **24.5** |
|---|---|

| Case | Calculation | GPA |
|---|---|---|
| No optional subject | 24.5 / 6 = 4.0833… | **4.08** |
| Optional subject A (4.00) | bonus 2.00 → 26.5 / 6 = 4.41666… | **4.42** half up; 4.41 truncated |
| Optional subject F | bonus 0 → 24.5 / 6 | **4.08** (does not fail) |
| Five required A+ and an optional A+ | (25 + 3) / 5 = 5.6 → cap | **5.00** |

### Example 6 — Exact boundary (defect E1-01)

| Input | Current engine (float) | Exact rule |
|---|---|---|
| 16.83 / 51, pass mark 16.83 | 32.999… % → **F, failed** | 1683 × 100 = 33 × 5100 → **D, 1.00, passed** |
| 10.20 / 17 | 59.999… % → B, 3.00 | exactly 60 % → **A-, 3.50** |
| 18.40 / 23 | 79.999… % → A, 4.00 | exactly 80 % → **A+, 5.00** |

### Example 7 — Annual result (Mathematics)

| Term subject marks | First Term 72.00, Half-Yearly 66.00, Annual 84.50 |
|---|---|

| Method | Calculation | Annual mark | Grade |
|---|---|---|---|
| final_only | 84.50 | 84.50 | A+ (5.00) |
| equal | (72 + 66 + 84.5) / 3 = 74.1666… | 74.17 | A (4.00) |
| weighted 25 / 25 / 50 | 18 + 16.5 + 42.25 | 76.75 | A (4.00) |

**Rounding sensitivity:** weighted 30 / 30 / 40 with terms of 79, 79 and 80.50 gives 23.7 + 23.7 + 32.2 = **79.60**.

| Policy | Grade |
|---|---|
| R3 none | A |
| R3 whole marks | 80 → A+ |

**`final_must_pass`:** suppose English is First Term 85, Half-Yearly 80 and Annual 25 (fail), with equal weights.
- The average is 63.33 → A-.
- With `final_must_pass` on, English **fails** for the year.
- Off, it passes with A-.

The school must choose.

### Example 8 — Absent and exempt

| Case | Result |
|---|---|
| Absent in the English term exam (required), default policy | English scores 0, shown as **Absent**; F; outcome failed, GPA 0.00 |
| Absent policy "missing" | Outcome **incomplete**; cannot publish until resolved (re-sit mark or exemption) |
| CT 2 exempt (illness); CT 1 = 17 / 20 | CT contribution uses CT 1 only: 85 % → 20 × 0.85 = **17.00** |
| Physical Education exempt (medical); five remaining required points sum to 20 | GPA = 20 / 5 = **4.00** (the exempt subject leaves the denominator) |

### Example 9 — Merit ties

| Student | Outcome | GPA | Total | Competition | Dense |
|---|---|---|---|---|---|
| Rafi | passed | 5.00 | 912 | **1** | 1 |
| Nila | passed | 5.00 | 912 | **1** | 1 |
| Tanim | passed | 5.00 | 905 | **3** | 2 |
| Sumi | passed | 4.83 | 930 | **4** | 3 |
| Arif | failed | 0.00 | 610 | **5** | 4 |

Sumi has the highest total but ranks after the 5.00 GPAs, because GPA comes first in the rank key.

## 9. Snapshots, determinism and audit

**Snapshot.** Each result version stores a snapshot with:
- the inputs: every mark and status used, with each sheet's version;
- the rules: blueprint version, grading bands, rounding, ranking and pass policies;
- the outputs: per-subject lines, GPA, outcome and positions;
- `inputs_sha256` over a canonical encoding of the inputs and rules.

**Determinism check.** Recomputing from a snapshot must reproduce the outputs byte for byte. This is tested and can be run by an admin as "verify results" at any time.

**Audit events:**
- `exam.blueprint_activated`;
- `exam.sheet_submitted`, `exam.sheet_returned`, `exam.sheet_approved`;
- `exam.mark_status_changed`;
- `exam.results_computed`, `exam.results_approved`, `exam.results_published`;
- `exam.result_corrected`, `exam.result_withheld`;
- `exam.report_cards_generated`.

The existing `exam.mark_saved` events continue.

## 10. Required tests

**Golden tests.** Every example in section 8 is a test with the exact expected outputs.

**Boundaries:**
- all 66 float boundary cases;
- every band edge for full marks 1–1000 with two-decimal marks;
- GPA values at .xx5.

**Equivalence.** The new engine and the current engine agree on all 621 lifecycle results and on the demo seed.

**Properties:**
- adding marks never lowers a percentage;
- exempting a CT never changes another student's result;
- ranking is invariant under input order.

**Corrections.** A correction creates new versions only for the students whose outputs changed, and superseded versions stay unchanged.
