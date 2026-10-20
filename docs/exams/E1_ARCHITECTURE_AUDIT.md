# E1 — Examination system architecture audit

**Baseline:** `main` at `82b4e73d` (after PR #25, Payment Safety), audited
2026-10-21 on branch `feature/e1-exam-architecture-ux`. No application code,
database or deployment was changed.

**Companion documents:**
- [Assessment blueprint spec](E1_ASSESSMENT_BLUEPRINT_SPEC.md)
- [Result calculation rules](E1_RESULT_CALCULATION_RULES.md)
- [Premium UX spec](E1_PREMIUM_UX_SPEC.md)
- [Routine and PDF spec](E1_ROUTINE_AND_PDF_SPEC.md)
- [Implementation roadmap](E1_IMPLEMENTATION_ROADMAP.md)

**Method:**
- Read the exam, routine, grading, class and subject, teacher assignment, analytics, export, notification and demo-seed code and migrations.
- Read the frontend pages, API client, navigation and print styles.
- Reused the evidence of the lifecycle audit (`test/full-school-lifecycle-audit`, `docs/qa/EXAM_RESULT_VERIFICATION.md`), which verified 621 published results independently.
- Checked one numerical question that the lifecycle audit did not cover: grade-band boundaries. A test was run against the real `exams.GradeFor` through a Go build overlay, so no repository file was changed.

## 1. Summary

**What works.** Within one exam and one section, results are correct for whole-number and common marks:
- GPA, pass/fail, totals and merit positions matched an independent calculation for all 621 lifecycle results.
- Publication is explicit, refuses missing marks, recomputes from stored marks, and is audited.
- Students and guardians see only published results.

**What blocks the vision:**
1. **One exam = one section = one sitting.** There is no exam term, no class-level configuration and no link between a class test and a term exam. A school with 20 sections and three terms creates 60 exams and configures 378 subject rows by hand.
2. **No assessment model.** Class tests, continuous assessment, components (CQ/MCQ/practical/viva), CT + Final, annual averages and best-N do not exist. Each exam is a flat list of subjects with one mark each.
3. **Grade boundaries use binary floating point.** 66 exact boundary cases are graded one band too low. In the worst case a student who scored exactly 33 % fails (section 5.2, issue E1-01).
4. **No result history.** Unpublishing deletes the results, so corrections leave no snapshot or version; report cards are not documents.
5. **No PDF capability.** Report cards and routines are on-screen pages printed through the browser.
6. **Teacher marks entry** is a single long list (one row per student per subject) with one save button, no paste, no autosave and no absent mark. Unsaved marks are lost when the teacher picks another exam.

## 2. Current data model

All amounts are `numeric(6,2)` for marks and `numeric(9,2)` for totals. Every table below is in migration `0701_exams_results.sql`, except where noted.

| Table | Purpose | Key constraints |
|---|---|---|
| `grading_schemes`, `grading_bands` (0003) | One scheme per school (`school_settings.grading_scheme_id`); integer percentage bands with a grade point | `bd_default`: A+ 80–100 (5.00), A 70–79 (4.00), A- 60–69 (3.50), B 50–59 (3.00), C 40–49 (2.00), D 33–39 (1.00), F 0–32 (0.00). Bands are editable (`grading:update`). |
| `academic_class_subjects` (0102) | Subjects of a class | `is_required <> is_fourth_subject`: the fourth subject is a **class** property, the same for every student in the class |
| `teacher_assignments` (0401) | Teacher ↔ session, class, subject | **No section.** A teacher assigned to Class 6 Bangla can enter Bangla marks in every Class 6 section. |
| `exams` | One exam for one **section** | `exam_type` ∈ class_test, half_yearly, annual, other; `status` ∈ draft, published, locked; name unique per session and section; optional `routine_id` (one exam per routine) |
| `exam_subjects` | Subjects of an exam | `full_marks` 0–1000, `pass_marks` ≤ full; one row per subject; must be a class subject |
| `exam_marks` | One mark per student per exam subject | `marks >= 0`; trigger refuses marks above full marks; `grade`, `grade_point`, `passed` filled only on publication; student context FK (session, class, section) |
| `exam_results` | One result per student per exam | total, full, GPA 0–5, passed, class and section position; **exists only while published or locked** |
| `exam_routines` (0601) | One exam routine per section | `is_published`; name unique per session and section |
| `exam_routine_slots` (0601) | One slot per **subject** per routine | `UNIQUE(routine_id, subject_id)`: theory and practical of one subject cannot both be scheduled |
| `school_events` (0901) | Calendar events with kind `holiday` and `exam` | Not used by routines or attendance |
| `file_objects` (0901) | Stored files with SHA-256 | `purpose` ∈ notice, diary, homework; no report-card or routine purpose yet |

**Permissions (0701, 0601):**
- `exams:manage` (admin);
- `exams:read` (admin, head teacher);
- `exams:enter_marks` (admin, teacher);
- `exams:read_self` (student);
- `exams:read_children` (guardian);
- `routine:*` in the same pattern.

The head teacher can read everything and approve nothing.

**Migration history that matters for E1:**
- 0003 grading;
- 0102 subjects;
- 0401 assignments;
- 0601 routines;
- 0701 exams;
- 0901 events and files;
- 1001 module dependencies (examinations needs students and staff);
- 1101 analytics;
- 1201 demo records;
- 1401 payment safety (unrelated tables).

The next free migration number is **1501**.

## 3. Current workflows and effort

"Interactions" counts clicks, selections and field entries in the current UI code (typing characters is not counted). The school is the lifecycle audit's: 10 classes, 2 sections each, 63 class–subject mappings.

| Task | Current steps | Count |
|---|---|---|
| Create one exam for one section with *S* subjects (default 100/33 marks) | Open Exams; name, type, session, class, section, (routine), Save; then per subject: choose subject, Add | 7 + 2*S* (Class 6, 7 subjects: **21**) |
| Set up one term for the whole school | 20 exams + 126 subject rows | **392** |
| Set up three terms (CT, half-yearly, annual) | — | **1,176** (the audit created 60 exams and 378 subject sheets) |
| Teacher enters one subject for one section (40 students) | Open Marks entry, find the exam (list of 10 per page, search), open it, scroll an interleaved list, 40 fields, Save | **~44**, repeated per section and per exam: a Bangla teacher of two Class 6 sections does it 6 times a year, plus every class test |
| Admin publishes a term | Per section: open exam, Publish; then Lock | **40** for 20 sections |
| Build one exam routine for one section | Name, session, class, section, Save; per slot: subject, date, start, end, room, Add | 5 + 6*S* (Class 6: **47**) |
| Build routines for one term | 20 routines + 126 slots | **856** |
| Student opens a report card | My results → card → Print | 3 |

**Friction points found in the code:**
- **Marks list** (`ExamsPage.tsx:64`): one row per student *and* subject, sorted by roll then subject code. A teacher with two subjects in the class tabs through alternating fields. The only keyboard navigation is Tab.
- **Data loss:** choosing another exam clears unsaved marks (`choose()` resets `markDraft`) with no warning.
- **One bad cell rejects the batch:** all marks go in one request. Any 400/409 shows the same "conflict" message with no cell highlighted.
- **No absent mark:** offices enter 0, which records F and fails the student (QA-07).
- **Concurrent edits:** last write wins without warning (QA-18).
- **Routine subject list:** the slot form lists every school subject; a subject not taught in the class is refused only after Save.
- **Teachers cannot see exam routines:** the routine page hides the exam tab for teachers (`RoutinePage.tsx:90`), and there is no teacher route for them.
- **Exam statuses reuse attendance colours** ("late" amber for draft, "present" green for published, "leave" blue for locked).

## 4. API surface (baseline)

| Method | Path | Guard |
|---|---|---|
| GET | `/exams`, `/exams/:id`, `/exams/:id/roster`, `/exams/:id/tabulation`, `/exams/:id/results/:studentId` | `exams:read` |
| POST, PATCH, DELETE | `/exams`, `/exams/:id`, `/exams/:id/subjects[/:subjectId]` | `exams:manage` + admin role |
| PATCH | `/exams/:id/state` (draft ↔ published → locked) | `exams:manage` + admin role |
| GET | `/exams/assigned`, `/exams/assigned/:id`, `/exams/:id/assigned-roster` | `exams:enter_marks` (teacher scope) |
| PUT | `/exams/:id/marks` (≤ 500 entries) | `exams:enter_marks`; non-admins must hold a matching teacher assignment |
| GET | `/exams/me/results[/:examId]` | `exams:read_self` |
| GET | `/exams/children/:studentId/results[/:examId]` | `exams:read_children` + guardian link |
| GET/POST/PATCH/DELETE | `/routines/exams…`, `/routines/exams/:id/slots…`, `/routines/exams/:id/publication` | `routine:*`; writes admin only |
| GET | `/routines/exams/me…`, `/routines/exams/children/:studentId…` | published routines of the student's section only |

## 5. Calculation logic and numerical precision

### 5.1 How a result is produced today

On `PATCH /exams/:id/state` → `published` (`repository.go` `SetState`):
1. Refuse if the exam has no subjects, more than one fourth subject, no active students, or **any** missing mark.
2. Read the school's grading bands.
3. Per student: `GradeFor` each subject, `GPA` across subjects, total and full marks; write the grade to each mark and insert `exam_results`.
4. Rank with SQL `rank()` (competition ranking) by GPA, then total.
   - **Class position** ranks across all exams of the same session, class, **exam name (case-insensitive)** and type.
   - **Section position** ranks within the section.

Unpublishing deletes the results and clears the grades; republishing recomputes them with whatever bands are current.

### 5.2 Precision defect (E1-01)

`GradeFor` computes `pct := marks * 100 / full` in `float64` and compares it with integer band edges (`pct >= MinMarks`). Marks have two decimals, which binary floating point cannot represent exactly.

I enumerated every full mark from 1 to 200 and every band edge (33, 40, 50, 60, 70, 80), taking the mark that is exactly on the edge. **66 cases fall just below their edge.** A Go test run through a build overlay against the real `GradeFor` confirmed them:

| Marks / full | Exact % | Application grade | Correct grade |
|---|---|---|---|
| 16.83 / 51 (pass mark 16.83) | 33 | **F, failed** | D, passed |
| 10.20 / 17 | 60 | B (3.00) | A- (3.50) |
| 18.40 / 23 | 80 | A (4.00) | A+ (5.00) |
| 32.80 / 41 | 80 | A | A+ |
| 36.80 / 46 | 80 | A | A+ |

Whole-number full marks such as 100, 50 and 25 are not affected, which is why the lifecycle audit's 621 results matched. Component-based blueprints, however, produce exactly these uneven full marks (scaled CT contributions, 1st and 2nd papers combined), so the defect must be fixed before E1 is built on it. GPA rounding uses `math.Round` on a float, which has the same class of risk at .xx5 boundaries.

### 5.3 Rules hard-coded today

| Rule | Current behaviour | Configurable? |
|---|---|---|
| Subject pass | `marks >= pass_marks` **and** the band's point > 0 | Pass marks per exam subject; the band floor (33 %) always applies |
| Fail | Any failed required subject → GPA 0.00, fail | No |
| Fourth subject | Adds max(0, point − 2.00); its failure never fails the student; at most one | No |
| GPA | (required points + bonus) / required count, capped at 5.00, rounded half away from zero to 2 decimals | No |
| Mark rounding before grading | None (79.99 → A) | No |
| Ranking | Competition ranking by GPA, then total; failed students ranked by total among GPA 0 | No |
| Class position across sections | Joined by exam **name** string | No — a renamed section exam silently ranks alone |

### 5.4 Concurrency and performance

- **One global lock:** every exam write takes the same transaction advisory lock, `pg_advisory_xact_lock(517007)`. All teachers saving marks school-wide are serialised. That is safe, but it will queue at exam time (40 teachers × autosave).
- **Per-entry queries:** `SaveMarks` runs about 4 queries and 1 audit insert per entry, so a 500-entry batch is about 2,500 statements inside the global lock.
- **No optimistic concurrency:** there is no version or ETag on marks (QA-18).

## 6. Authorization and publication security

**Sound:**
- Every write needs authentication, a permission and the module; setup and state changes also need the admin role.
- Teacher writes are checked per entry against `teacher_assignments`.
- Student and guardian reads go through `scopeStudent`: a student reads only their own results, a guardian only linked children. Both see only `published` or `locked` exams; a draft card returns 404.
- Analytics, exports' published counts and result notifications only read published or locked exams. Notifications for an exam that returns to draft are withdrawn.
- Every mark change is audited with old and new values.

**Gaps against the E1 rules:**

| Rule (E1 brief) | Current | Gap |
|---|---|---|
| Students must not see CT, performance or draft marks | A published `class_test` exam is a normal result card for students | **Gap:** CT marks become visible on publication |
| Teachers enter and revise by permission | Any assigned teacher can change any mark of the class-subject in any section until publication | Section-level assignment and a submit/return workflow are missing |
| Computation automatic, approval and publication explicit | Computation happens only at publication; there is no approval step and the head teacher cannot approve | Missing review stage |
| Only published results and approved report cards visible | Yes, on screen | No report-card document exists |
| Preview before publication | `GET /exams/:id/results/:studentId` skips the status check for staff, but results exist only after publication, so a draft returns 404 | No provisional preview: staff see results only after students do |
| Student and guardian share one account (product direction) | Guardians have their own accounts and routes | **Out of scope for E1**; do not change identity here. The design keeps both routes working. |

## 7. Frontend and state

- **ExamsPage.tsx** (71 lines) serves all four roles. It holds local `useState` for the form, marks and selection, with TanStack Query keys `['m7-exams', …]`. The tabulation shows only in admin mode, so the head teacher sees the list, roster and tabulation through `/admin/exams`.
- **RoutinePage.tsx** (133 lines): class routine week grid (a good basis) and an exam routine table.
- **Shared design system** (`attendanceUi.tsx`): `ui` tokens, `PageHeader`, `StatTile`, `StatusBadge`, `SegmentedStatus`, `Tabs`, `Notice`, `EmptyState`, `SectionCard`, `Skeleton`, `Pager`, `Avatar`; `ConfirmDialog`, `StateScreen` and `Icon`. Brand variables `--brand-primary`, `--brand-ink`, `--brand-on` and `--brand-soft` follow the school settings; there is a dark theme. The fonts are Inter and Noto Sans Bengali (bundled with `@fontsource`).
- **Printing:** `@media print` in `styles.css` isolates `.exam-report`. There is no `@page` size, so paper size and margins depend on the browser.

## 8. PDF capability

There is none:
- no PDF library in `go.mod` or `package.json`;
- receipts, report cards and routines are browser-printed pages (the lifecycle audit "printed to PDF" with the browser).

Automatic PDFs after publication need a server-side renderer that shapes Bangla correctly (conjuncts such as ক্ষ, জ্ঞ, ষ্ট্র). The options and the recommendation are in the [routine and PDF spec](E1_ROUTINE_AND_PDF_SPEC.md#6-pdf-renderer).

## 9. Consumers that a new engine must keep working

| Consumer | Reads | Contract to preserve |
|---|---|---|
| Analytics (`analytics/repository.go`, `reports.go`) | `exam_results`, `exam_marks`, `exams.status` | One row per student per published exam with total, full, GPA, passed and positions |
| Admin export (`admin/export.go`) | `exam_results` | Same columns |
| Control centre (`admin/overview.go`) | Counts of published and draft exams | Status values |
| Result notifications (`notify/repository.go`) | `exam_results` of published exams; withdrawn when back to draft | Publication remains the trigger |
| Demo seed (`demo/seed.go`) | Inserts exams, subjects and marks, then publishes | Seed must keep working or be updated in the same milestone |

**Design consequence:** the new engine keeps writing `exam_results` (as a projection of the latest published result version), so analytics, exports and notifications keep working unchanged.

## 10. Compatibility with Payment Safety (PR #25)

- **No shared tables or permissions.** Exams never read fees, and migration 1401 touched only fee and payment tables.
- **Patterns to reuse:**
  - label legacy data instead of rewriting it;
  - administrator-only, audited corrections with a required reason;
  - explicit, separately enabled modes;
  - watermark anything that is not authoritative (here: "PROVISIONAL — NOT PUBLISHED").
- **Next migration number:** 1501.

## 11. Tests

**Existing:**
- `TestBangladeshGradesAndFourthSubject`: band edges at whole and .99 values, scaled full marks, fourth-subject cap, required failure, pass threshold, two fourth subjects.
- `TestPostgresExamLifecycle`.
- Route protection tests for exams and routines.
- `TestPostgresRoutineLifecycle`.
- The lifecycle audit scripts (not merged).

**Missing:**
- exact-boundary cases with uneven full marks (E1-01);
- GPA rounding at .xx5;
- ranking ties across sections with differently named exams;
- concurrent mark edits;
- a teacher of section A writing section B (currently allowed by design);
- a grading-band change between publish and republish;
- the CT visibility rule;
- large-class performance of marks save;
- routine conflicts across sessions, holidays and theory/practical;
- print layout.

## 12. Feature coverage matrix

Legend: **Done** = already implemented; **Improve** = implemented but needs improvement; **Partial** = partially supported; **Missing**.

| Capability | Status | Evidence / note |
|---|---|---|
| First, second, third term exams | Partial | Exam types class_test, half_yearly, annual, other; no "term" concept or ordering |
| 100-mark term exams | Done | Full marks per exam subject |
| Class tests during the term | Partial | Separate exam per CT per section; not linked to the term result |
| Continuous assessment / class performance | Missing | — |
| CT + Final combined result | Missing | — |
| Equal annual average | Missing | QA-04 |
| Weighted annual average | Missing | QA-04 |
| Final-only results | Done | A single exam is a final-only result |
| Best-N assessments | Missing | — |
| Subject-specific calculations | Partial | Only full and pass marks differ per subject |
| Optional / fourth subject | Improve | Bonus rule correct; fourth subject fixed per class, not chosen per student |
| Subject-group aggregation (1st + 2nd paper, groups) | Missing | — |
| MCQ, CQ, practical, viva components | Missing | — |
| Separate component pass rules | Missing | — |
| Absent, exempt, incomplete, withheld | Missing | QA-07 |
| School-configured rounding | Missing | No rounding; GPA half-up only |
| School-configured grading | Improve | Bands editable, one scheme per school; not versioned with results |
| Versioned exam blueprints | Missing | — |
| Enter each mark once | Partial | True per exam; CT marks cannot feed a term result, so they are re-keyed or recalculated by hand |
| Automatic result computation | Improve | At publication only; no readiness preview |
| Explicit approval | Missing | Publish only; no approval role |
| Explicit publication | Done | Admin publish, unpublish, lock |
| Students see only published results | Done | — |
| Students do not see CT / performance | Missing | Published CTs are visible |
| Report cards on screen | Done | `ReportCardView` |
| PDF report cards, automatic after publication | Missing | Browser print only |
| Report card version, signatures, verification | Missing | — |
| Merit position | Improve | Correct competition ranking; cross-section link by exam name |
| Tie handling | Improve | Competition ranking; no configurable policy |
| Missing-marks blocking | Done | Publication refuses with 409 |
| Corrections after publication | Improve | Unpublish → edit → republish; no snapshot of the previous version |
| Result snapshots / version history | Missing | Results deleted on unpublish |
| Audit trail | Done | Per mark, subject, exam and state change |
| Examination Center dashboard | Missing | Exams list only |
| Role-specific exam layouts | Partial | One page with mode switches |
| Marks entry grid with keyboard navigation | Improve | List with Tab only |
| Bulk paste / import | Missing | — |
| Autosave and save state | Missing | — |
| Unsaved data recovery | Missing | Drafts lost on exam switch |
| Validation feedback per cell | Improve | Min/max on input; server errors not mapped to cells |
| Edit history visible to staff | Partial | Audit log only |
| Submit for review | Missing | — |
| Aggregation preview | Missing | — |
| Exam calendar | Partial | `school_events` kind `exam` exists; not linked to routines |
| Routine draft builder | Improve | Per section, one slot at a time |
| Room / time / subject management | Done | Per slot |
| Conflict detection | Improve | Same section or room overlap on a date; no teacher/invigilator, holiday or weekend checks |
| Holidays and unavailable dates | Partial | `school_events` holidays exist; routines ignore them |
| Section and class scheduling | Improve | Per section only; no class-wide routine |
| Bulk routine generation | Missing | — |
| Routine approval and publication | Partial | Publish/unpublish; no approval or amendment history |
| Routine versioning | Missing | — |
| Mobile routine display | Improve | Table with horizontal scroll |
| Branded A4 routine PDF | Missing | Browser print |
| Student upcoming exams | Missing | Routine list only |
| Teacher sees exam routine | Missing | Exam tab hidden for teachers |
| Bangla and English | Done | i18n throughout; Bangla names on cards |
| School branding | Done | Logo, names and brand colours |

## 13. Priority issues

| ID | Priority | Issue | Recommendation |
|---|---|---|---|
| E1-01 | **P0 (correctness)** | Float grading misgrades 66 exact boundaries, including a pass graded as fail | Exact decimal arithmetic in a pure calculation package (next milestone) |
| E1-02 | P1 | No assessment model: CT, components, CT + Final, annual results | Versioned blueprints ([spec](E1_ASSESSMENT_BLUEPRINT_SPEC.md)) |
| E1-03 | P1 | Absent / exempt marks impossible (QA-07) | Mark statuses with defined effects |
| E1-04 | P1 | No result versions; unpublish deletes results | Result versions and snapshots; `exam_results` becomes a projection |
| E1-05 | P1 | Class position joins sections by exam name | Explicit exam term ↔ section exams link |
| E1-06 | P1 | Students can see class-test marks | Visibility per assessment, default hidden for CT and performance |
| E1-07 | P1 | No approval step; head teacher cannot approve | Submit → review → approve → publish workflow |
| E1-08 | P1 | No PDF report cards | Server renderer with Bangla shaping, generated on publication |
| E1-09 | P2 | Fourth subject fixed per class | Per-student optional subject choice |
| E1-10 | P2 | Marks entry: data loss, no paste, no autosave, one error rejects all | Marks workspace ([UX spec](E1_PREMIUM_UX_SPEC.md), section 5) |
| E1-11 | P2 | Global advisory lock and N+1 writes | Per-sheet locking, set-based upsert, optimistic versions |
| E1-12 | P2 | Grading bands not versioned with results | Snapshot the scheme into each result version |
| E1-13 | P2 | Routine per section; one slot per subject; no holidays | Term routine model ([spec](E1_ROUTINE_AND_PDF_SPEC.md)) |
| E1-14 | P3 | Teachers cannot see exam routines | Teacher exam schedule view |
| E1-15 | P3 | Exam statuses use attendance colours | Dedicated exam status tones |
| E1-16 | P3 | Misleading "not found" when removing a subject in use (QA-17) | Specific error codes |

## 14. Decisions needed from the school / product owner

These are configuration defaults; the design supports each option.

1. **GPA rounding:** round half up to two decimals (current) or truncate.
2. **Mark rounding before grading:** none (current; 79.5 → A) or to whole marks (79.5 → A+).
3. **Absent in a required subject:** fail (recommended default, like 0) or "incomplete" (no result until resolved).
4. **Failed students in merit lists:** ranked after all passed students by total (current, effectively) or not ranked.
5. **Class tests and continuous assessment:** confirm they are never shown to students individually (E1 brief), only their contribution inside the final result.
6. **Student and guardian on one account:** a later identity milestone. E1 designs the Student Portal for both and does not change authentication.
7. **Curriculum rules:** Bangladeshi curricula and assessment rules have changed several times in recent years. Each school's component marks, pass marks and annual formula must be confirmed with the school rather than taken from this document.
