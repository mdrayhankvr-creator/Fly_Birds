# E5 — Exam terms, assessment blueprints and the setup wizard

Status: implemented and verified locally on
`feature/e5-flexible-assessment-blueprints` (2026-10-22). Migration 1502.

E5 follows the E5 row of
[E1_IMPLEMENTATION_ROADMAP.md](E1_IMPLEMENTATION_ROADMAP.md), the
[blueprint specification](E1_ASSESSMENT_BLUEPRINT_SPEC.md), the
[calculation rules](E1_RESULT_CALCULATION_RULES.md) and the UX
specification §6 (wizard) and §7 (builder). §8 lists where it differs and
what waits for E6.

## 1. What a school can do now

- **Terms.** A session has ordered terms (First Term, Half-Yearly,
  Annual …) with optional dates and a status (planning, running, results,
  closed). "Add First Term, Half-Yearly and Annual" sets up the usual year in
  one click.
- **Blueprints.** Each class has an assessment blueprint per session, in
  versions:
  - **Assessments by term:** class tests, continuous assessment, class
    performance and the term exam, each with student visibility.
  - **Per subject:**
    - its mark sheets and components (Total, CQ + MCQ, CQ + MCQ + Practical,
      or any components with pass marks);
    - how the term mark is made: presets *Final only*, *CT average + final*
      and *Best N class tests + final*, or *Custom* contributions (single,
      sum, average, best N, latest, each with a worth);
    - the term full and pass marks, its role (required, optional, not
      graded) and a combined-papers key (Bangla 1st + 2nd).
  - **School rules:**
    - how combined papers pass (combined or each paper);
    - GPA rounding (half up or cut);
    - what absent means (zero or missing);
    - ties in positions (1, 1, 3 or 1, 1, 2);
    - whole-mark and contribution rounding (R3, R1).
  - **The annual method:** final only, equal or weighted; whether the final
    term must be passed. It is stored and validated now and calculated in
    E6.
- **Builder.** A draft saves itself as it is edited. The server validates it
  on every save, and each problem names its subject, term and values ("Bangla
  · First Term: contributions add up to 90.00, expected 100.00"). The issue
  links to the subject.
  - **Worked sample:** works one subject's term mark from sample marks with
    the exact engine.
  - **Activation:** activating validates, writes the normalised rows,
    snapshots the grading bands, records the definition's SHA-256 and freezes
    the version. Changes then need a new version; exams keep the version
    they were created from.
  - **Copy forward:** "Copy <last session>" copies last year's design onto
    the new session's terms, matched by order.
- **Setup wizard.** Five steps, then one **Create** makes every exam (one per
  section per assessment) and every mark sheet (one per subject, with its
  components).
  - **Steps:** term, classes, marks due dates, sheets and teachers, review.
  - **Saved as you go:** the choices are kept on the term, so a half-finished
    setup survives and nothing reaches teachers before Create.
  - **Teachers:** subjects nobody teaches can be given a teacher on the way.
  - **Refusals:** Create is refused while a name clashes or a due date is
    missing.
  - **Idempotent:** creating again adds only what is missing.
- **Optional subjects.** Each student's fourth subject for the session.
  - While nobody in a class has a choice, the class's fourth subject applies
    to everyone, as before.
  - Once choices exist, a student takes only the optional subject they chose.
  - Sheets, readiness, mark entry and publication all follow this.
- **Component marks.** On a component sheet the marks workspace shows one
  input per component with the exact total.
  - Enter and the arrows move down a component column.
  - A pasted table maps its columns to the components.
  - The server stores the components and their sum.
- **Term results (staff only, provisional).** A class's term results from
  the marks entered so far:
  - each subject's contributions, grades and fail reasons;
  - combined papers, the optional subject, GPA, outcome, section and class
    positions;
  - the sheets still awaiting approval.

  It is framed "Provisional — not published", stores nothing, and students
  never see it.

## 2. Architecture

### Database (migration 1502)

| Table / column | Purpose |
|---|---|
| `exam_terms` | Terms of a session; `setup` holds the wizard's draft; `legacy_key` marks terms made by the upgrade |
| `assessment_blueprints` | One per session and class |
| `blueprint_versions` | Draft, active or retired. The full `definition` (jsonb) and its SHA-256, a grading-band snapshot and the normalised policy columns. Frozen by trigger once activated (only active → retired) |
| `blueprint_assessments`, `blueprint_subjects`, `blueprint_components` | Written on activation for queries and constraints; frozen too, except an assessment's student visibility |
| `exams.term_id`, `assessment_key`, `blueprint_version_id`, `marks_due_on` | Link an exam to its term, assessment and version. A composite FK to the version's assessment, a trigger requiring the version's own session and class, and one exam per term × assessment × section |
| `exam_subject_components` | A sheet's components, copied from the blueprint when the sheet is made (as `exam_subjects` copies full and pass marks) |
| `exam_component_marks` | Component marks of a present mark; `exam_marks.marks` is their sum, written in the same transaction, so analytics read it unchanged |
| `student_subject_choices` | A student's optional subject per session |

Functions:

| Function | Purpose |
|---|---|
| `student_takes_subject()` | Enrolment, used by every roster, count, save, readiness check and publication |
| `exam_visible_to_students()` | Which exams students and guardians may see |
| `exam_link_legacy()` | The upgrade's linking step (§5) |

### Backend

**`internal/blueprints`** (new):

| File | Contents |
|---|---|
| `definition.go` | Definition types and canonical encoding and hash |
| `validate.go` | Every activation rule of the specification (§6), 58 issue codes with parameters |
| `term.go` | Term results on the exact engine (`results/calc`): contributions, component and paper pass rules, combined papers, optional subjects, absence policy, exemptions shrinking full marks with an exactly scaled pass mark, ranking |
| `repository.go`, `service.go` | Terms, blueprints, versions, activation, copy forward, visibility, choices, the worked sample |
| `setup.go` | The wizard: plan, save, create |
| `preview.go` | The provisional term results |

**`internal/exams`** (changed):

- Per-cell saves take component marks and refuse marks for optional
  subjects a student did not choose (`not_enrolled`).
- **Publishing an exam made from a blueprint applies its version:**
  - component and paper pass rules, combined papers (both papers store the
    combined grade; an exempt paper stores none);
  - GPA rounding, absence and dense ranking;
  - per-student optional subjects.
- **Exams outside a blueprint**, and the upgrade's legacy versions, publish
  exactly as before.
- **Class positions** join a term's assessment across sections (E1-05). An
  exam still made the old way joins the upgraded term of the same name and
  type, so positions group as before.
- **Hidden assessments:** students and guardians no longer see their results
  (E1-06), in the result list, a direct link or the withheld notice.
- **`exam-verify`** scores with the same rules, so it stays a faithful
  check.

**Notifications:** result notifications skip hidden assessments.

### API (19 routes, all `RequirePermission` + the examinations module)

Reading needs `exams:read` (administrator, head teacher). Changing needs
`exams:manage` and an administrator.

| Group | Routes |
|---|---|
| Terms | `GET/POST /exams/terms`, `PUT/DELETE /exams/terms/:id` |
| Wizard | `GET/PUT /exams/terms/:id/setup`, `POST /exams/terms/:id/create` |
| Term results | `GET /exams/terms/:id/results?class_id=` |
| Blueprints | `GET/POST /exams/blueprints`, `POST /exams/blueprints/:id/versions` |
| Versions | `GET/PUT/DELETE /exams/blueprint-versions/:id`, `POST …/activate`, `PUT …/visibility`, `POST …/sample` |
| Choices | `GET/PUT /exams/subject-choices` |

Activation problems return 422 `blueprint_invalid` with every issue. All
routes are in `docs/openapi.yaml`.

### Frontend (`frontend/src/pages/exams/`)

| File | Screen |
|---|---|
| `ExamSetup.tsx` | Terms, class blueprints and optional subjects (session picker in the URL) |
| `BlueprintBuilder.tsx` | Structure, rules and check panes on desktop; tabs on phones |
| `TermWizard.tsx` | The five steps with a stepper (progress bar on phones) |
| `TermResults.tsx` | Provisional tabulation with a pinned student column; one card per student on phones |
| `blueprintModel.ts` | Pure helpers (exact sums, presets, apply to all), unit-tested |

Routes are `/admin/exams/setup`, `/admin/exams/blueprints/:versionId`,
`/admin/exams/terms/:termId/setup` and `/admin/exams/terms/:termId/results`.

The exam page links to "Terms & blueprints" and shows marks due dates. The
E4 workspace gains component columns. There are 368 new keys in each
language for the setup screens, and 6 for component entry.

## 3. Calculation

Exact throughout (integer hundredths and `math/big.Rat`), reusing E2's
`results/calc`:

- **Contributions:**
  - **single:** `worth × m / f`;
  - **sum:** `worth × Σm / Σf`;
  - **average:** the average of percentages, so CTs out of 20 and 25 weigh
    equally;
  - **best N:** the best N by percentage, ties by assessment order;
  - **latest:** the last source that is not exempt.
- **Exempt sources** leave the set. If every source is exempt, the
  contribution leaves and the full marks shrink by its worth, with the pass
  mark scaled exactly (rounded up to hundredths).
- **Absent** scores 0, or makes the result incomplete when absent means
  missing.
- **Components:** a component below its pass mark fails the subject whatever
  its total ("Mathematics: CQ is below its pass mark").
- **Combined papers:** graded on their combined marks. Under each-paper,
  every paper must also pass.
- **Optional subject:** never fails the student; it adds max(0, point − 2.00)
  to the GPA, capped at 5.00.
- **Defaults** reproduce today's results. Every example of the calculation
  rules (§8) is a test: 69.90 A-, 72.50 A, the CQ failure, 88/200 C or F,
  GPA 4.42 / 4.41 / 4.08, exemptions and ties.

## 4. Security and privacy

- **Students and guardians** reach none of the new routes (403). They never
  see a hidden assessment, a draft, a provisional term result or another
  student's choice.
- **Teachers** cannot read blueprints, terms or term results. They enter
  marks only for students who take the subject.
- **Head teachers** see everything read-only.
- **Every change is audited:**
  - `exam.term_saved` and `exam.term_deleted`;
  - `exam.blueprint_drafted`, `exam.blueprint_draft_deleted` and
    `exam.blueprint_activated` (with the SHA-256);
  - `exam.assessment_visibility_changed`;
  - `exam.subject_choices_saved` (each student's previous and new subject,
    and how many published exams the class has) and `exam.term_created`;
  - component marks inside `exam.mark_saved`.
- **Frozen versions** are protected by database triggers, not only by the
  application. A trigger also keeps a component sheet's full marks equal to
  the sum of its components, so the old exam page cannot set them apart.
- **Published results are stored.** Report cards read the stored grades and
  results, so a later blueprint or choice change cannot alter what students
  see. Blueprint versions in use are frozen; changes go into a new version.
- **Each school is its own deployment and database,** so no data crosses
  schools.

## 5. Upgrade (no value changes)

Migration 1502 links existing exams:
- a term per distinct exam name and type;
- a legacy, final-only blueprint version per session and class, with each
  exam's own full and pass marks kept on its sheets;
- term, assessment and version on every exam.

It writes no mark, grade, sheet or result. The audit log records
`exam.blueprints_migrated` with the counts, including `class_tests_hidden`:
the published class tests students no longer see on their own.

**Measured on a disposable school built from `main`:**

- **Starting data:** demo seed plus the E3 workflow — 7 published exams
  (one a class test), 41 results and 120 marks, 2 of them not present.
- **Before and after the upgrade,** the fingerprints of every mark (id,
  marks, grade, point, passed, status, reason, version), every result
  (totals, GPA, pass, positions, time), every sheet and every exam are
  **identical**.
- **`exam-verify` after the upgrade:** no differences in 41 results.
- **After a full E5 run on the same school,** the 7 original exams are still
  byte-identical. `exam-verify` checks 15 published exams (425 results,
  2,040 marks) with no differences.

**The one visible change.** Published class-test results are no longer
listed for students and guardians (roadmap acceptance E1-06; decision 5 of
the audit). An administrator can show them again per assessment ("Students
see: When published") on the class's legacy version. The change is audited.

## 6. Measurements (one laptop, disposable school, not production)

| Flow | Result |
|---|---|
| Set up a whole term: 1 class, 2 sections, 3 assessments, 6 subjects | 9–12 interactions, 14–16 s, 6 exams and 36 sheets in one transaction. UX target 10–15; the E1 audit counted 392 interactions for 20 sections |
| Build a class blueprint version (class tests, presets, apply to all, components, combined papers, sample, activate) | 25–32 interactions, about 11 s |
| Enter CQ + MCQ for 48–56 students by keyboard | 288–336 keystrokes, 7–10 s, **1 save request** |
| Provisional term results for 92–100 students | 352–367 ms |
| Request failures in the builder and wizard | 0 |

## 7. Verification (2026-10-22, disposable resources only)

- **Go:**
  - `go vet` clean;
  - `go test ./... -p 1 -count=1` passes on a fresh database;
  - govulncheck finds nothing reachable.
  - **New tests:**
    - `internal/blueprints`: Schools A–D valid; every validation rule; the
      worked examples; positions; terms, versions, activation and copy
      forward; choices; the wizard's refusal, conflicts, creation and
      idempotence;
    - `internal/exams`: the legacy link (fingerprints, links, visibility, a
      second run, old-style exams joining their term); the policy-aware
      scoring; an end-to-end blueprint term (components, enrolment,
      combined each-paper, dense ranking across sections, hidden class
      tests, preview, `exam-verify`);
    - `internal/httpapi`: route protection for every new route and role.
- **Frontend:**
  - `npm run build` passes;
  - the i18n check passes (2,372 keys per language);
  - `npm run test:unit` passes (11, including the blueprint model and
    component paste).
- **Browser, on a fresh demo school:**

  | Suite | Result |
  |---|---|
  | `e2e/exam-setup.mjs` (new) | **83/83** (also 83/83 on the upgraded school) |
  | `suite.mjs` | 106/106 |
  | `whitelabel.mjs` | 23/23 |
  | `payment-safety.mjs` (off) | 12/12 |
  | `exam-workflow.mjs` | 17/17 |
  | `marks-workspace.mjs` | 49/49 |

  `exam-setup.mjs` covers:
  - the setup, builder, wizard and results screens at 1440, 768 and 390 px;
  - English and Bangla, light and dark, and three school colours;
  - no overflow, measured WCAG AA contrast and axe with no WCAG 2.1 AA
    violations.

  The screenshots were reviewed by hand.

To repeat on a disposable school, run the other suites first (the E4
setup enlarges the demo sections):

```sh
E2E_BASE_URL=http://localhost:8107 DEMO_PASSWORD=... node e2e/exam-setup.mjs
```

## 8. Differences from the E1 design, and what waits for E6

| E1 design | E5 | Why |
|---|---|---|
| Blueprints per class *or group* (`group_key`: science, business …) | Per class only | The school data has no student groups yet; subjects are mapped per class |
| `teacher_assignments.section_id` | Not added | Assignments stay per class and subject, as today |
| Component marks each with a status | One status per subject mark; components only for present marks | A partly absent student (practical only) is not modelled yet |
| Legacy migration creates result versions v1 | Not in E5 | Result versions and snapshots are E6 |
| Term and annual results published to students | Term results are a staff preview; the annual method is stored and validated | Publication needs result versions, snapshots and corrections (E6). "With the term result" visibility therefore behaves as hidden until then |
| A version is immutable once marks reference it | Immutable once activated | Simpler and stricter; changes are a new version |
| Results use the version's grading snapshot | The snapshot is stored; an exam's own publication still uses the school's current bands, as before | Pinning results to a snapshot is part of E6's result versions |
| "Do not rank failed students" option | Not offered | `exam_results` positions are required numbers; E6 |
| Visibility per assessment | Plus "class tests hidden" for exams made outside a blueprint | Keeps E1-06 for old-style exams too |

## 9. Limitations

- Changing a student's optional subject after an exam is published does
  not change the published card (it is stored), but `exam-verify` then
  reports that student's result and publishing again would change it. The
  audit records who moved from which subject. Corrections to published
  results are E6; schools should set choices before marks.
- Once a class uses choices, an old-style exam may hold several fourth
  subjects; each student is graded on the one they chose.
- The old exam page's roster cannot enter component marks. Its saves on a
  component sheet are refused with "Enter every component". Teachers use the
  marks workspace.
- The wizard creates exams for classes whose blueprint has assessments in
  the term; routines (E8) and PDFs (E7) are untouched.
- Copy forward maps terms by order and creates missing terms in planning.
  Subjects removed from a class must then be fixed in the new draft
  (validation lists them).
