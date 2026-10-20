# E1 — Implementation roadmap

How the examination overhaul is delivered in small, reviewable, reversible
milestones. Each milestone is one feature branch from `main`, one PR, green CI
and explicit approval before the next starts. The findings and IDs (E1-01 …)
are in the [architecture audit](E1_ARCHITECTURE_AUDIT.md#13-priority-issues).

## 1. Principles

1. **Correctness before features.** The result engine is fixed and frozen by tests before any feature depends on it.
2. **Never rewrite published results.**
   - Published results change only through an explicit, audited correction that creates a new version.
   - Migrations label and link existing data and do not recompute it. Payment Safety (migration 1401) set this precedent.
3. **Compatibility contract.** `exam_results`, the existing exam and routine endpoints, analytics, exports, notifications and the demo seed keep working in every milestone, or are updated in the same PR.
4. **Backend and frontend ship together** for each user-visible capability. Engine-only milestones have no UI changes.
5. **Every milestone states its rollback.** For one with a migration, the rollback is the pre-upgrade backup (DEPLOYMENT.md §8), and the upgrade is tested on a disposable school copied from `main`, with row fingerprints compared before and after.

## 2. Dependency order

```text
E2 Exact result engine ──► E3 Mark statuses & sheet workflow ──► E4 Teacher marks workspace
          │                          │
          └──────────────► E5 Terms, blueprints, setup wizard ──► E6 Result versions, review,
                                                                     publication, corrections, annual
                                                                           │
                                     E7 Document renderer & automatic PDFs ◄┘
                                     E8 Term routine planner & routine PDFs (after E5; PDFs after E7)
                                     E9 Examination Center & bulk reports (after E6–E8)
```

Year-end promotion (QA-04 in the lifecycle audit) depends on annual results (E6). It is planned as its own milestone outside this series.

## 3. Milestones

| Milestone | Scope | Migration | Key acceptance criteria |
|---|---|---|---|
| **E2 — Exact result engine** *(recommended next, section 4)* | Pure calculation package with exact arithmetic and policy defaults equal to today's behaviour; the existing publish path uses it | None | Fixes E1-01; golden examples; equivalence on existing results |
| E3 — Mark statuses & sheet workflow | Absent / exempt / incomplete / withheld; sheet status (submit, return, approve); `exams:review` for the head teacher; optimistic concurrency (`row_version`); per-cell errors; set-based mark writes and per-sheet locks instead of the global lock | 1501 | QA-07 and QA-18 closed; E1-11; existing sheets migrate as approved/present without value changes |
| E4 — Teacher marks workspace | Work queue, spreadsheet grid, keyboard map, paste/import, autosave, unsaved recovery, history panel, mobile one-student mode | None (uses E3) | UX spec §5 criteria; 40-row paste with errors; crash recovery; axe clean |
| E5 — Terms, blueprints, setup wizard | `exam_terms`, blueprints and versions, components, combined subjects, per-student optional subject, CT + Final and best-N term results, visibility rules, wizard, legacy blueprint migration | 15xx | Schools A–D configurable; legacy exams linked without value changes; class position by term (E1-05); CTs hidden from students (E1-06) |
| E6 — Result versions, review, publication, corrections, annual | `result_versions` and snapshots; `exam_results` projection; review screen; publication centre; correction workflow; annual results (equal, weighted, final-only) | 15xx | Every published result has a reproducible snapshot; corrections supersede, never delete; annual examples match |
| E7 — Document renderer & automatic PDFs | Renderer spike, then the `renderer` service; `document_jobs` outbox; report-card template; verification endpoint; provisional previews | 15xx | Routine/PDF spec §12; Bangla sample set; targets met on reference hardware |
| E8 — Term routine planner | Sittings, rooms, weekly off days (shared with attendance, QA-12), conflict validator, deterministic draft generation, approval, amendments with diff, student next-exam card, teacher duties, routine PDFs | 15xx | Routine/PDF spec §12; legacy routines migrated; existing routine endpoints unchanged |
| E9 — Examination Center & bulk reports | Role-specific centre, readiness, review queue, bulk exports (merged PDFs, tabulation, merit lists) | None | UX spec §4 and §13 criteria |

Each later milestone gets its own detailed plan, written against the code as it is when that milestone starts.

## 4. Recommended next milestone — E2: Exact result engine

### Why this one, and why first

- **The only correctness defect.** E1-01 grades 66 exact boundary cases one band too low, and one of them turns a pass into a fail. Every later feature (components, scaled CT contributions, combined papers, annual averages) produces exactly the uneven full marks that trigger it.
- **Smallest safe step:**
  - no migration, no new endpoint, no UI change;
  - the publish path keeps the same inputs and outputs;
  - the change is isolated to one calculation package with exhaustive tests.
- **It freezes the rules** of the [calculation document](E1_RESULT_CALCULATION_RULES.md) in code before blueprints depend on them.

### Scope

1. **New package `backend/internal/results/calc`** (pure, no database):
   - exact arithmetic on integer hundredths and `math/big.Rat`;
   - band lookup by integer comparison;
   - subject grade and pass;
   - GPA with the optional-subject bonus and cap;
   - the rounding policies R1–R5 with today's defaults;
   - competition / dense ranking;
   - a canonical explanation of each step, for later UI and PDF use.
2. **`exams.GradeFor` and `exams.GPA`** become thin wrappers over the new package. The publish path in `SetState` keeps its structure.
3. **Ranking:** the SQL `rank()` stays unchanged in E2 (it already sorts exact numeric values). The package's ranking is used in tests to cross-check it.
4. **Read-only check `cmd/exam-verify`** (operator tool, like `school-provision`):
   - recomputes every **published** result with the exact engine and lists any that would differ, with student, exam, subject and old → new grade;
   - **changes nothing.**
   - Changing a published result stays an explicit correction, which E6 provides. Until then the office can unpublish and republish one exam.
5. **Documentation:**
   - OPERATIONS.md: how to run the check before and after upgrading;
   - PROGRESS.md and the roadmap: the milestone entry.

### Out of scope

Mark statuses, components, terms, blueprints, result versions, any UI change, any migration, and any change to existing published results.

### Acceptance criteria

1. **Exact boundaries:**
   - all 66 float boundary cases (full marks 1–200) grade at the exact band;
   - an exhaustive test over full marks 1–1000 and every band edge passes;
   - 16.83 / 51 grades D and passes.
2. **Golden examples:** every example in section 8 of the calculation rules that does not need E3/E5 features (examples 5, 6, 9, and the grading parts of 1, 2, 3 and 7) produces exactly the documented outputs.
3. **Equivalence:**
   - results identical to the current engine for every case where the float engine was not at a boundary;
   - the demo seed and the existing exam integration tests pass unchanged;
   - the lifecycle fixture's 621 results reproduce, if the lifecycle scripts are available.
4. **`exam-verify`:**
   - reports zero differences on the demo school;
   - reports the expected differences on a fixture seeded with boundary marks;
   - performs no write (tested with a read-only database role).
5. **Defaults unchanged:** GPA rounding stays half up, and there is no mark rounding, so no current behaviour changes except the boundary fix.
6. **Gates:** CI green (backend, frontend and release-stack); `go vet`, gofmt and govulncheck clean.

### Tests

- **Unit:** table tests for bands, pass rules, the GPA bonus and cap, rounding policies at .xx5, and both ranking modes.
- **Property:** grade monotonic in marks; GPA invariant under subject order; ranking invariant under input order.
- **Integration:** publish an exam with boundary marks and check the stored grades. Run `exam-verify` on a database with a pre-fix published boundary result: it is reported, not changed.

### Risks

- **A republished exam can change.** A school that unpublishes and republishes an exam with a boundary mark will see that grade change. It is a correct change, and `exam-verify` lists such cases in advance. OPERATIONS.md explains it.
- **Wider rule questions stay open.** The rounding and ranking policy decisions (audit, section 14) are not needed for E2, since defaults keep today's behaviour. They are needed before E5.

### Branch and commits

- **Branch:** `feature/e2-exact-result-engine` from `main`.
- **Commits** (focused, Conventional Commits, author Rayhan, no AI trailers):
  - `feat(results): add an exact result calculation package`
  - `refactor(exams): grade and GPA through the exact engine`
  - `feat(exams): add a read-only published result check`
  - `test(results): cover boundaries, golden examples and equivalence`
  - `docs: document the exact result engine and the verification check`
- **Merge:** after green CI and your review. Not pushed or merged without approval.
