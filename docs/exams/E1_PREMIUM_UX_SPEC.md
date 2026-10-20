# E1 — Premium examination UX specification

Screen-by-screen design for the Smart Examination & Result Management System.
The baseline is the current `ExamsPage.tsx`, `RoutinePage.tsx` and the shared
design system (`attendanceUi.tsx`, `AppShell`, `ConfirmDialog`, `StateScreen`,
`Icon`); see [the audit](E1_ARCHITECTURE_AUDIT.md#3-current-workflows-and-effort)
for current effort. Data and rules come from the
[blueprint spec](E1_ASSESSMENT_BLUEPRINT_SPEC.md) and
[calculation rules](E1_RESULT_CALCULATION_RULES.md).

## 1. Design direction: "calm ledger"

Exams are high-stakes, number-heavy and seasonal. The interface should feel
like a well-kept register: quiet, precise, and obviously trustworthy. Polish
comes from typography, alignment and state clarity, not decoration.

1. **One primary action per screen**, in the brand colour. Everything else is secondary or ghost.
2. **Numbers lead:**
   - tabular figures (`tabular-nums`), right-aligned, with the same number of decimals in a column;
   - full marks shown beside every mark ("68 / 100").
3. **Status means words + colour + icon,** never colour alone. Exam states get their own tones (section 2) instead of the attendance tones they borrow today.
4. **Brand colour is for identity and action** (header, primary buttons, focus, selected rows). It never encodes status, so any school colour works.
5. **Context is always visible:** session › term › class › section › subject sit in a sticky context bar on work screens.
6. **Nothing provisional looks final.** Previews carry a diagonal "PROVISIONAL — NOT PUBLISHED" watermark and a dashed border. The Payment Safety screens used the same pattern for simulated records.
7. **Motion:**
   - 150–200 ms, ease-out, only for state changes (save tick, row highlight, panel slide);
   - fully disabled under `prefers-reduced-motion`.
8. **Improve the existing system, don't replace it.** Keep the `ui` tokens, radius scale (`rounded-xl` controls, `rounded-2xl` cards), the shadow, Inter / Noto Sans Bengali, `PageHeader`, `Tabs`, `EmptyState`, `Skeleton`. New shared pieces live in `pages/examUi.tsx`.

### 1.1 Typography and spacing

| Role | Size / line | Weight | Use |
|---|---|---|---|
| Page title | 24 / 32 | 700 | `PageHeader` (existing) |
| Section title | 16 / 24 | 700 | `ui.sectionTitle` (existing) |
| Body | 14 / 20 | 400 | Text, form fields |
| Grid cell | 14 / 20, tabular | 500 | Marks |
| Table header | 12 / 16, uppercase, tracking-wide | 600 | `ui.th` (existing) |
| Caption / meta | 12 / 16 | 500 | Timestamps, hints |
| Big figure | 30 / 36, tabular | 700 | GPA, progress counts |

**Spacing** uses a 4 px grid: cards pad 16 px (mobile) / 24 px (desktop), sections are 20 px apart, the grid rows are 40 px (comfortable) or 32 px (compact, a user toggle stored per device).

**Bangla:** Noto Sans Bengali at the same size, with line height +2 px for Bangla runs. Digits stay Latin by default (current behaviour). Report cards offer Bangla digits as a school option (section 11).

### 1.2 Icons

Use the existing `Icon` set. New glyphs follow its 24 px, 1.75 px stroke style: `clipboard-check` (sheet), `calendar-grid` (routine), `stamp` (approve), `megaphone` (publish), `file-pdf`, `lock`, `history`, `keyboard`.

## 2. Exam status system

| State | Tone | Icon | Words (EN / BN) |
|---|---|---|---|
| Not started | slate outline | circle-dashed | Not started / শুরু হয়নি |
| In progress | sky | pencil | In progress / চলমান |
| Submitted | amber | send | Submitted / জমা দেওয়া হয়েছে |
| Returned | rose | arrow-uturn | Returned / ফেরত পাঠানো হয়েছে |
| Approved | emerald | check-badge | Approved / অনুমোদিত |
| Provisional (computed) | indigo, striped | beaker | Provisional / অস্থায়ী |
| Published | brand solid | megaphone | Published / প্রকাশিত |
| Locked | slate-800 | lock | Locked / লক করা |
| Revised (new version) | violet outline | history | Revised / সংশোধিত |

All tones meet WCAG 2.2 AA (4.5:1 text, 3:1 non-text) in light and dark themes. They replace the attendance tones in `StatusBadge` for exam screens.

## 3. Shared components (new, in `examUi.tsx`)

| Component | Behaviour |
|---|---|
| `ContextBar` | Sticky under the app header. Breadcrumb chips (Session › Term › Class › Section › Subject); each chip opens a switcher with keyboard search. On mobile it collapses to one line ("Class 7 B · Mathematics · CT 2") with a sheet to change. |
| `ProgressMeter` | "34 / 40" plus a thin bar; colour follows the state. Screen readers hear "34 of 40 marks entered". |
| `StatusChip` | Section 2 states. |
| `SaveIndicator` | Saved ✓ (time) · Saving… · 3 unsaved (offline) · Couldn't save — Retry. It lives in the context bar and is announced politely (`aria-live="polite"`). |
| `MarksGrid` | Spreadsheet grid (section 5). |
| `ReadinessList` | Grouped blocking items, each a link to the fix ("Class 7 B · English · CT 2 — 3 marks missing"). |
| `ContextToolbar` | Actions for the current selection (paste, mark absent, filter); it becomes a bottom sheet on mobile. |
| `DiffTable` | Before / after values for corrections, with the reason. |
| `ProvisionalFrame` | Watermark and dashed border for computed, unpublished output. |

**States everywhere:**
- **Loading:** `Skeleton` with the real layout shape. Never a spinner over an empty page.
- **Empty:** `EmptyState` with the next action ("No term yet — Set up the first term").
- **Error:** `Notice` with the server's specific message and a Retry. The generic "examConflict" message is retired; every error code maps to a sentence.

## 4. Examination Center — `/exams`

**Purpose:** one place that answers "what needs my attention in exams today?"

**Who:**
- admin (operations);
- head teacher (review);
- teacher (own work queue).

Students and guardians do not use it; they go to Student Results (screen 8).

**Baseline:**
- `/admin/exams` is the exam list for admin and head teacher;
- `/teacher/exams` is the marks list;
- both redirect to the new routes, so bookmarks keep working.

**Desktop layout (admin):**

```text
┌ PageHeader: Examinations · [Session 2027 ▾]               [+ New term] ┐
├ Term strip: First Term ● running │ Half-Yearly ○ planning │ Annual ○    ┤
├──────────────────────────────┬──────────────────────────────────────────┤
│ Marks completion             │ Needs attention (ReadinessList)           │
│  Class 1 ████████░░ 82 %     │  • 6 sheets not started (CT 2)            │
│  …  per class, click → drill │  • 2 sheets returned to teachers          │
│  Overall 412 / 520 sheets    │  • English 7 B: 3 marks invalid           │
├──────────────────────────────┼──────────────────────────────────────────┤
│ Upcoming (next 14 days)      │ Results pipeline                          │
│  Sun 12 Jan · Class 6–10     │  Computed 8 · Approved 5 · Published 3    │
│  Bangla 1st · 10:00          │  Report cards: 120 ready · 0 failed        │
└──────────────────────────────┴──────────────────────────────────────────┘
```

**Head teacher:**
- the "Needs attention" panel becomes **Review queue**, listing sheets submitted for approval, oldest first;
- a compact class comparison (pass rate, average GPA) for published terms is shown, reusing analytics queries.

**Teacher:**
- the page is the **work queue** with sheets grouped *Due soon / In progress / Returned / Submitted*;
- each row shows "Class 7 B · Mathematics · CT 2 — 34 / 40 — due Thu", with the primary action **Continue**;
- "My exam duties" (routine slots) sit beside it.

**Key interactions:**
- every number is a link to the filtered list behind it;
- the term strip selects the term for every panel;
- no card repeats another's figure.

**Mobile:**
- single column in priority order (attention → upcoming → progress → pipeline);
- the term strip scrolls horizontally with snap.

**APIs:** new `GET /exams/center?session_id&term_id`, which returns counts, the attention list and upcoming slots in one call.

**Acceptance criteria:**
- **Load:** first paint shows skeletons; the full data arrives in one request (< 300 ms on the 200-student fixture).
- **Clicks:** the teacher reaches a sheet in **2 clicks** from login (nav → Continue).
- **Scope:** the head teacher sees only review actions, never publish (unless granted).

## 5. Teacher Marks Workspace — `/exams/marks` and `/exams/marks/:sheetId`

**Purpose:** enter one sheet (assessment × section × subject) fast and safely.

**Baseline:** the interleaved list in `ExamsPage.tsx` with one Save button.

**Desktop layout:**

```text
┌ ContextBar: 2027 › First Term › Class 7 › B › Mathematics › CT 2   [Saved ✓ 10:42] ┐
├ Toolbar: [Filter: All ▾] [Search roll/name] [Paste] [Mark selected: Absent ▾]  34/40 ━━━━━━─ │
├───────┬──────────────────────┬────────┬────────┬────────┬──────────┬─────────────────┤
│ Roll  │ Student              │ CQ /50 │ MCQ/25 │ Total  │ Status   │ Preview         │
│  1    │ Ayesha Hossain       │  41    │  22    │  63    │ Present  │ A- (3.50)        │
│  2    │ Rahim Akter          │  —     │  —     │  —     │ Absent   │ F · Absent       │
│  3    │ …                    │ [  ]   │        │        │          │                  │
└───────┴──────────────────────┴────────┴────────┴────────┴──────────┴─────────────────┘
  History (side panel, toggle H): 10:40 Ayesha CQ 39 → 41 (you)
  [Submit for review]  ← disabled until complete; shows what is missing on hover/focus
```

**Grid behaviour:**
- **Pinned context:** the Roll and Student columns are pinned, and the header is sticky.
- **Columns** follow the blueprint: one per component plus the computed Total.
- **Preview column** (provisional grade) shows the subject grade for this assessment only and is labelled "Preview". For a CT, it shows the percentage instead of a grade.

**Keyboard map** (shown with `?`):

| Key | Action |
|---|---|
| Arrow keys | Move between cells |
| Enter / Shift+Enter | Commit and move down / up (column-wise entry is the most common) |
| Tab / Shift+Tab | Move right / left |
| Typing a number | Starts editing |
| Esc | Cancels the edit |
| `a` | Set Absent |
| `i` | Set Incomplete |
| `p` | Set Present (clears the status) |
| Delete | Clear |
| Ctrl+V | Paste a block |
| Ctrl+Z | Undo the last local change (before save) |
| Ctrl+F | Focus search |

**Paste and import:**
- **Paste:** pasting tab-separated data from Excel or Google Sheets opens a preview dialog. Rows match by admission number or roll when the first column contains them, otherwise by grid order.
- **Validation:** invalid values (above full marks, three decimals, text) are highlighted, and the teacher applies valid cells only or fixes them first.
- **File import:** a CSV file uses the same dialog.

**Validation:**
- invalid cells get a rose border and inline message on focus ("Above full marks (25)");
- errors never block saving other cells;
- the server validates per cell and returns per-cell errors.

**Autosave:**
- changed cells save in batches after 800 ms idle, or on leaving the row;
- each cell sends its `row_version`;
- the `SaveIndicator` shows the state.

**Conflicts:**
- if another user changed a cell, it shows a conflict badge with both values and a choice ("Keep mine" / "Use theirs");
- nothing is silently overwritten (closes QA-18).

**Unsaved recovery:**
- unsent changes are kept in `localStorage` under the sheet and user, values only (student id → marks), for at most 7 days;
- reopening the sheet offers "Restore 3 unsaved marks from 10:42?";
- the local copy is cleared on successful save and on sign-out (a shared school computer must not keep marks);
- leaving the page with unsent changes asks for confirmation.

**Filters:** All · Missing · Invalid · Absent · Changed since submission.

**Large classes:**
- rows virtualise above 60 students;
- typing latency must stay under 16 ms per keystroke at 120 students × 4 components.

**Edit history:**
- a side panel shows this sheet's audit events (who, when, old → new);
- after approval, changes go through the correction workflow and show its reason.

**Submit for review:**
- enabled when every student has a status and every Present mark is valid;
- the button explains what is missing when focused;
- submitting locks the sheet for the teacher and notifies the head teacher.

**Mobile (≤ 640 px):**
- **One student per screen:** name, roll and a large numeric input per component.
- **Next / Previous** buttons sit in a sticky bottom bar together with the progress meter.
- **Status:** Absent is one tap.
- **Navigation:** a list view toggle shows all students with their entered values.
- **Paste** is unavailable; import from file remains.

**APIs:**
- `GET /exams/sheets?mine=1` (queue);
- `GET /exams/sheets/:id` (rows, components, statuses, versions);
- `PATCH /exams/sheets/:id/marks` (per-cell results);
- `POST /exams/sheets/:id/submit`.

The old `PUT /exams/:id/marks` stays for one release as a compatibility path.

**Effort (40 students, one subject, one CT):**

| | Current | Target |
|---|---|---|
| Navigation to the sheet | 3–4 | 2 |
| Entering 40 marks | 40 field entries, interleaved with other subjects | 40 keystroke entries down one column, or **1 paste** |
| Saving | 1 manual Save; loss on navigation | 0 (autosave), recovery on crash |
| Fixing one invalid mark | Whole batch refused, no cell shown | Only that cell flagged |
| Total interactions | ~44 + risk of re-entry | **~3 with paste**, ~43 typed, no loss |

**Acceptance criteria:**
- a full sheet can be entered with the keyboard only;
- paste of 40 rows with 2 invalid values applies 38 cells and flags 2;
- killing the tab after typing offers restore;
- two users editing one cell produce a visible conflict, never a silent overwrite;
- axe has no violations;
- the grid works with a screen reader as a table with row and column headers.

## 6. Exam Setup Wizard — `/exams/terms/new`

**Purpose:** create a term for the whole school (all classes, all sections, every assessment and sheet) in one guided flow.

**Baseline:** 7 + 2*S* interactions per exam per section; **392** for one term.

**Steps** (a stepper on the left; on mobile, a progress bar with step titles):

1. **Term:**
   - name, kind (term or annual), dates;
   - "Copy from" the same term last session, or another term.
2. **Classes:**
   - all classes are pre-selected, with checkboxes;
   - each class shows the blueprint it will use (active version), "Needs a blueprint" with a link to the builder, or "Use last year's blueprint" (one click copy).
3. **Assessments:**
   - from the blueprints (e.g. CT 1, CT 2, Term exam), with due dates for marks entry;
   - one date set applies to all classes, with per-class overrides.
4. **Sheets and teachers:**
   - **Preview:** the sheets the term will create ("520 sheets for 20 sections"), each with its assigned teacher from `teacher_assignments`.
   - **Gaps:** unassigned sheets are listed, and an assignee can be picked for each.
5. **Review & create:**
   - summary counts and warnings (no routine yet, subjects without a teacher);
   - **Create term** runs in one transaction and is audited.

**Key interactions:**
- Every step is saved as a draft term, so a half-finished setup is never lost.
- Nothing reaches teachers until **Create**.

**Effort:** ~10–15 interactions for the whole school when blueprints exist (copy forward), against 392 today (**> 95 % fewer**).

**Acceptance criteria:**
- Creating a term for the 20-section fixture produces one exam per section per assessment and one sheet per subject, linked by `term_id` and `assessment_key`.
- Cancelling leaves a draft and no sheets.
- Re-running "Create" is idempotent.

## 7. Assessment Blueprint Builder — `/exams/blueprints/:id`

**Purpose:** define a class's assessments, subject rules, components and formulas, versioned.

**Layout:** three panes on desktop, stacked tabs on mobile.

| Pane | Content |
|---|---|
| Left: structure | Terms → assessments (drag to order); subjects with role (Required / Optional / Not graded); combined subjects shown as groups |
| Centre: rules for the selection | For a subject: components table (key, full, pass); term formula picker (presets "CT 20 % + Final 80 %", "Best 2 of 3 CTs + Final", "Final only", "Custom"); pass rules (combined / each paper; final must pass) |
| Right: live check | Validation list (section 6 of the blueprint spec) and a **worked sample**: enter sample marks and see the calculated subject mark, grade and GPA with every step shown (the same explanation as the calculation rules document) |

**Key interactions:**
- **Presets first.** Custom formulas use structured controls (sources, combine, worth), never free-text expressions.
- **Apply to many:** "Apply to all subjects" copies a rule, and per-subject overrides are marked.
- **Activation:** **Activate version** validates; errors link to the exact field.
- **Read-only once used:** an active version in use opens read-only with **Create new version**.
- **Versions:** the history tab compares two versions as a side-by-side diff.
- **Grading scheme:** a read-only view of the bands with their snapshot date; editing stays in school settings.

**Acceptance criteria:**
- each example configuration in the blueprint spec (Schools A–D) can be built without custom formulas;
- the worked sample matches the calculation rules' examples exactly;
- an invalid blueprint cannot be activated;
- an active, used version cannot be edited.

## 8. Exam Routine Planner — `/exams/terms/:termId/routine`

See the [routine spec](E1_ROUTINE_AND_PDF_SPEC.md#2-target-routine-model) for the model.

**Layout (desktop):**
- **Calendar grid:** dates as columns, sittings (Morning / Afternoon) as rows. School holidays (from `school_events`) and weekends are shaded and blocked.
- **Papers:** a side list of unscheduled papers per class (e.g. "Class 9 · Physics · Theory").
- **Placing a paper:** drag a paper onto a cell, or select it and press Enter, then pick the cell with the arrow keys. Classes share a cell; rooms are assigned per section in the slot drawer.
- **Conflicts:** they appear live, as a red edge with a reason ("Class 9 already has Chemistry this sitting"). Hard conflicts block publishing; soft ones ("two exams for Class 6 on consecutive sittings") warn.
- **Generate draft:** fills unscheduled papers with the rules from the routine spec, shows what it could not place and why, and never overwrites placed papers.
- **Actions:** Submit for approval → Approve → **Publish**. After publication, changes create an **amendment** with a reason and a notice to affected students.

**Mobile:** a per-day list with an add button. The planner is usable but intended mainly for desktop. The published routine is mobile-first (screen 8).

**Effort:** **856** interactions per term today; the target is 1 generate + adjustments (typically < 60 for 63 papers), or about 2 per paper placed manually.

**Acceptance criteria:**
- **Blocking:** no hard conflict can be published; holidays and weekends are blocked.
- **Amendments:** an amendment keeps the previous version readable and notifies only the affected classes.
- **Generation:** the generator is deterministic for the same input.

## 9. Result Review & Approval — `/exams/terms/:termId/review`

**Who:** head teacher (`exams:review`); admin.

**Layout:**
- **Left:** queue of submitted sheets, then classes ready for result approval.
- **Centre: a class tabulation** (students × subjects) with a provisional frame. It shows each subject's mark and grade, then GPA, outcome and positions.
- **Signals:**
  - outliers are flagged quietly (subject average far from the class's other subjects, many absences, a sheet with all equal marks);
  - flags are hints, never blockers.
- **Sheet review:** a read-only grid with history. **Approve** or **Return with reason** (the reason is required and goes to the teacher's queue).
- **Result approval:** approve per class when the readiness list is empty. Approval records who and when.

**Mobile:** queue and approve or return work. The full tabulation scrolls horizontally with pinned student names.

**Acceptance criteria:**
- **Returning:** a returned sheet reappears in the teacher's queue with the reason.
- **Approval:** approval is impossible with blocking items.
- **Record:** every action is audited.

## 10. Result Publication Center — `/exams/terms/:termId/publish`

**Who:** admin (or whoever holds publication).

**Layout:**
- **Readiness by class:** sheets approved, results approved, report-card template ready, routine status.
- **Publish:** select classes and choose **Publish results**. A confirmation shows counts ("Publish 8 classes, 412 students; report cards will be generated; students and guardians will be notified").
- **After publishing:** live progress of report-card generation (ready / queued / failed with retry), notification count, and a link to the Student Results preview as a sample student.
- **Corrections:** a published class opens a **Correction** dialog: choose student and subject, give a reason, and the sheet is reopened for them. After re-approval, **Publish revision** shows a `DiffTable` of every changed result, including position changes for other students.
- **Lock** closes the term (with confirmation).

**Effort:** **40** clicks for 20 sections today; the target is **3** (select all ready classes → Publish → confirm).

**Acceptance criteria:**
- **Atomic:** publishing a class is all-or-nothing.
- **Report cards:** these are generated automatically, and each student's PDF is available within the target time (routine spec, section 7).
- **Revisions:** a revision supersedes the earlier versions without deleting them.

## 11. Student Results — `/student/results` and `/guardian/children/:id/results`

**Purpose:** published routine and results only, for a student or a guardian.

**Layout (mobile-first):**
- **Next exam** card at the top, during an exam period: "Tomorrow · Physics · 10:00 · Room 204". It comes from the published routine of the student's section.
- **Results list:** the newest term first. Each card shows the term, GPA (big figure), outcome chip, "Revised" if applicable, and **Report card** / **Download PDF**.
- **Annual result card,** when published, shows the method label ("Weighted: 25 % First Term, 25 % Half-Yearly, 50 % Annual").
- **Never shown:** CT or performance marks individually, drafts, provisional values, superseded versions.

**States:**
- **Before any publication:** "Results appear here after the school publishes them."
- **Withheld:** "Result withheld — please contact the school office", with no marks.

**Guardian:** the same screens for each linked child, with a child switcher. The product direction of one shared account is left to a later identity milestone.

**Acceptance criteria:**
- **Server-side hiding:** CT marks never reach the browser for a student (checked at the API level).
- **Revisions:** a revised result shows only the latest version and its date.
- **Accessibility:** works at 320 px; screen-reader labels describe GPA and outcome.

## 12. Student Report Card — `/student/results/:resultId`

**On screen:** the same structure as the PDF (routine and PDF spec, section 8) in a responsive layout:
- **Header:** school identity; student name, roll, class, section.
- **Summary:** GPA, outcome and position, if the school shows them.
- **Subject table:** columns follow the blueprint (Marks, Grade, Point; and CT / Term contributions and components when the school enables them).
- **Footer:** publication date and version.
- **Mobile:** subject rows collapse into cards, one per subject.

**Actions:**
- **Download PDF:** the generated, stored file, never a client-side render.
- **Print:** prints the PDF.
- **Verify:** shows the verification code.

**Acceptance criteria:**
- the on-screen values equal the PDF values (same snapshot);
- the PDF link works only for the student, a linked guardian and staff.

## 13. Bulk Reports & PDF Export — `/exams/terms/:termId/reports`

**Who:** admin, head teacher.

**Outputs:**

| Output | Scope | Format |
|---|---|---|
| Report cards | Class or section | One merged PDF (print-ready, ordered by roll) or a ZIP of single PDFs |
| Tabulation sheet | Class or section | PDF (A4 landscape or A3) and CSV |
| Merit list | Class | PDF and CSV |
| Mark sheets (blank) | Sheet | PDF with the roster for manual marking |
| Mark sheets (filled) | Sheet | PDF for teacher sign-off |
| Routine | Term / class / section | PDF (A4) |
| Result summary | Term | PDF: pass rate and GPA distribution per class |

**Interactions:**
- choose output and scope, then Generate. Jobs run in the background with progress;
- finished files stay listed for 30 days, with who generated them;
- re-generation of the same version returns the stored file (idempotent).

**Acceptance criteria:**
- a 40-student class report-card PDF is ready in the target time (routine spec, section 7);
- every file records its source result version;
- CSV numbers match the PDFs exactly.

## 14. Accessibility and quality bar

- **Standard:** WCAG 2.2 AA.
  - Every function is reachable by keyboard.
  - Visible focus uses the existing brand focus ring.
  - Targets are at least 44 × 44 px on touch.
- **Grid semantics:** the grid uses `role="grid"` with row and column headers. Cell errors are tied to the cell with `aria-describedby`.
- **Live regions:** save state, conflicts and job progress are announced.
- **Bangla:** every string is in both languages. Report-card language follows the viewer, with a school default for PDFs.
- **Viewports:** layouts are tested at 320, 390, 768, 1280 and 1440 px, in light and dark themes.
- **Data limits:** no horizontal page scroll; wide tables scroll inside their container with pinned columns.

## 15. Summary of effort targets

| Workflow | Current | Target | Reduction |
|---|---|---|---|
| Set up one term (20 sections) | 392 | 10–15 | > 95 % |
| Teacher: one sheet (40 students) | ~44, data-loss risk | ~3 with paste; ~43 typed, autosaved | navigation 50 %, loss eliminated |
| Publish a term (20 sections) | 40 | 3 | 92 % |
| Build a term routine | 856 | < 60 (generate + adjust) | > 90 % |
| Find what is missing before publishing | Try to publish, read 409 | Readiness list | — |
| Student: see result | 3 | 2 (list → card), PDF 1 more | — |
