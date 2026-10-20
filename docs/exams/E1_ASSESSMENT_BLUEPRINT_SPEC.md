# E1 — Assessment blueprint specification

How a school describes **what is assessed, how marks are entered once, and
how they combine into term and annual results**, without one formula for all
schools. Calculation details are in
[E1_RESULT_CALCULATION_RULES.md](E1_RESULT_CALCULATION_RULES.md); the current
model is in [E1_ARCHITECTURE_AUDIT.md](E1_ARCHITECTURE_AUDIT.md#2-current-data-model).

## 1. Principles

1. **Evolve, don't duplicate.**
   - An `exams` row stays what it is today: one assessment sitting for one section.
   - Blueprints add the structure above it (terms, assessments, formulas) and below it (components, mark statuses).
   - Analytics, exports, notifications and report cards keep reading `exam_results`.
2. **Every mark is entered once.** A class-test mark is entered in its own sheet and feeds every result that uses it (term result, annual result) by formula. Nobody re-keys or hand-copies a CT contribution.
3. **Configuration is data, versioned.** A blueprint version is immutable once any mark references it. Changes create a new version; each result records the version, grading scheme and rounding policy it was computed with.
4. **Defaults reproduce today's behaviour.** A school that does nothing gets single-assessment, final-only results identical to the current engine, with the precision defect E1-01 corrected.
5. **Unpublished is invisible to students.** Visibility is a property of each assessment and of each result version, enforced on the server.

## 2. Concepts

| Concept | Meaning | Example |
|---|---|---|
| **Exam term** | A named assessment period in a session, ordered | First Term (1), Half-Yearly (2), Annual (3) |
| **Blueprint** | The assessment design for one class (optionally one group of the class) in one session | Class 9 Science 2027 |
| **Blueprint version** | Immutable snapshot of a blueprint; v1, v2, … | v2 adds a viva to Physics |
| **Assessment** | Something marks are recorded for, belonging to a term | CT 1, CT 2, CT 3, Class performance, Term examination |
| **Subject rule** | How a subject is assessed and combined: role, papers, components, formula, pass rules | Physics: CQ 50, MCQ 25, practical 25 |
| **Component** | A separately marked part of a subject in an assessment | CQ, MCQ, Practical, Viva, Written, Oral |
| **Combined subject** | Subjects (papers) graded together | Bangla = Bangla 1st paper + Bangla 2nd paper |
| **Mark sheet** | The unit of marks entry: one assessment × one section × one subject (or paper) | CT 2 · Class 7 B · Mathematics |
| **Mark status** | Present, Absent, Exempt, Incomplete, Withheld | — |
| **Result version** | A computed, immutable result for one student and one scope (assessment, term or annual) | Term 1 result v1 (published), v2 (correction) |

## 3. Data model

New tables use migration 1501 onwards. Names are proposals; columns are the minimum needed.

### 3.1 Terms and blueprints

```text
exam_terms
  id, session_id → academic_sessions
  name (≤120), sequence (1..9), kind ∈ {term, annual}
  starts_on, ends_on (optional), status ∈ {planning, running, results, closed}
  UNIQUE(session_id, sequence), UNIQUE(session_id, lower(name))

assessment_blueprints
  id, session_id, class_id, group_key (nullable: 'science', 'business', 'humanities', …)
  name, current_version_id
  UNIQUE(session_id, class_id, coalesce(group_key,''))

blueprint_versions
  id, blueprint_id, version (1..), status ∈ {draft, active, retired}
  definition jsonb  -- the full, validated definition (sections 3.2–3.4)
  definition_sha256, grading_scheme_snapshot jsonb, rounding_policy jsonb, ranking_policy jsonb
  created_by, created_at, activated_by, activated_at
  UNIQUE(blueprint_id, version)
  -- a version with any referencing mark can no longer change (trigger)
```

The definition is stored as validated JSON, because its shape (formulas, best-N, weights) varies per school. The values that queries need are normalised into the tables below and generated from the definition on activation.

### 3.2 Assessments

```text
blueprint_assessments
  id, blueprint_version_id, key ('CT1', 'CT2', 'PERF', 'TERM1', …), term_id → exam_terms
  kind ∈ {class_test, continuous, performance, term_exam}
  default_full_marks, student_visibility ∈ {hidden, after_term_publication, with_assessment_publication}
  UNIQUE(blueprint_version_id, key)
```

**Default visibility:**
- class tests, continuous assessment and performance: `hidden`. Students never see them individually; they see only their contribution inside a published term result, and only if the school allows contribution columns on the report card.
- term exams: visible only through the published term result.

### 3.3 Subject rules and components

```text
blueprint_subjects
  id, blueprint_version_id, subject_id, role ∈ {required, optional, non_graded}
  combined_key (nullable: 'BAN', 'ENG' to combine papers), display_order
  term_formula (reference to a formula in the definition), pass_rule (definition reference)

blueprint_components
  id, blueprint_subject_id, assessment_key, component_key ('CQ','MCQ','PRAC','VIVA','WRITTEN','ORAL','TOTAL')
  full_marks numeric(6,2) > 0, pass_marks numeric(6,2) nullable (≤ full)
  UNIQUE(blueprint_subject_id, assessment_key, component_key)
```

A subject without components in an assessment has one implicit `TOTAL` component; that is exactly today's model.

### 3.4 Formulas (part of the definition)

Each subject's term result is a list of **contributions**, scaled and summed. Each contribution names the assessments it draws from, how to combine them, and how much it is worth.

```json
{
  "term_formulas": {
    "ct20_final80": [
      { "from": ["CT1", "CT2", "CT3"], "combine": "average", "worth": 20 },
      { "from": ["TERM1"], "combine": "single", "worth": 80 }
    ],
    "best2ct_final": [
      { "from": ["CT1", "CT2", "CT3"], "combine": { "best": 2 }, "worth": 20 },
      { "from": ["TERM1"], "combine": "single", "worth": 80 }
    ],
    "final_only": [ { "from": ["TERM1"], "combine": "single", "worth": 100 } ]
  },
  "annual": { "method": "weighted", "weights": { "TERM1": 30, "TERM2": 30, "ANNUAL": 40 }, "level": "subject_percent" }
}
```

- `combine`: `single`, `average`, `sum`, `{ "best": N }` (best N by percentage, then averaged), or `latest`.
- `worth`: the marks the contribution is scaled to. Contributions of one subject must sum to the subject's term full marks (normally 100).
- `annual.method`:
  - `final_only` (the last term counts);
  - `equal` (average of the term percentages);
  - `weighted` (weights summing to 100).
- `annual.level`: `subject_percent` (recommended: combine each subject's percentage, then grade) or `gpa` (average the term GPAs). Section 5 of the calculation rules explains why the level matters.

### 3.5 Per-student subject choice

```text
student_subject_choices
  student_id, session_id, subject_id, role ∈ {optional}   -- the student's fourth subject
  UNIQUE(student_id, session_id, role)
```

The blueprint lists the subjects a student *may* choose as optional. A student without a choice takes no fourth subject.

**Compatibility:** while no choices exist, `academic_class_subjects.is_fourth_subject` keeps defining the class's fourth subject, as today.

### 3.6 Marks

The existing tables gain columns; nothing is renamed.

```text
exams                     + term_id → exam_terms (nullable for legacy), assessment_key, blueprint_version_id (nullable)
exam_subjects             + sheet status ∈ {not_started, in_progress, submitted, returned, approved}
                          + submitted_by/at, reviewed_by/at, return_reason
exam_marks                + status ∈ {present, absent, exempt, incomplete, withheld} default 'present'
                          + row_version integer (optimistic concurrency)
                          marks becomes nullable when status <> 'present'
exam_component_marks      exam_mark_id, component_key, marks numeric(6,2) nullable, status, row_version
                          UNIQUE(exam_mark_id, component_key)
```

- **Sheet:** an `exam_subjects` row is the sheet for one assessment × section × subject. Its status drives the teacher work queue and the review workflow.
- **Components:** with components, `exam_marks.marks` is the derived sum of the component marks, written in the same transaction. Analytics keep reading it unchanged.
- **Teacher scope:** an optional `teacher_assignments.section_id` (nullable = all sections, today's meaning) allows section-level assignment without breaking existing assignments.

### 3.7 Results

```text
result_versions
  id, scope ∈ {assessment, term, annual}, scope_id (exam id, term id or session id), class_id, section_id
  student_id, version (1..), status ∈ {computed, approved, published, superseded, withdrawn}
  blueprint_version_id, inputs_sha256, snapshot jsonb (inputs, rules, outputs, per-subject lines)
  total, full, percent, gpa numeric(3,2), grade, outcome ∈ {passed, failed, incomplete, withheld}
  class_position, section_position (nullable when not ranked)
  computed_at, approved_by/at, published_by/at, correction_reason
  UNIQUE(scope, scope_id, student_id, version)
```

- **Compatibility projection:** publishing an assessment- or term-scope version writes the matching `exam_results` row of that section exam. Analytics, exports and notifications keep working unchanged.
- **Corrections:** a correction never edits a published version. It creates the next version and marks the old one `superseded` (section 7 of the calculation rules).
- **Annual results:** these have no `exams` row. Analytics gains annual reports in a later milestone.

## 4. Mark statuses

| Status | Entered by | Marks value | Effect on the subject | Effect on the result |
|---|---|---|---|---|
| Present | Teacher | Required, 0–full | Graded normally | — |
| Absent | Teacher | Empty | Scores 0 and fails the component and subject (default), or is treated as missing (school option) | Default: fail, like today's 0, but labelled "Absent" instead of a silent 0 |
| Exempt | Admin or head teacher, with a reason | Empty | Removed from the subject or assessment: the contribution is re-normalised over the remaining assessments (CT), or the subject leaves the GPA denominator | Result computed without it; the report card shows "Exempt" |
| Incomplete | Teacher | Empty | Blocks the result | Result outcome `incomplete`; cannot be published until resolved |
| Withheld | Admin, with a reason | Kept but held back | Blocks the result | Outcome `withheld`; shown to the student as "Result withheld — contact the school" with no marks |

Only `present` carries marks.

**Readiness:** a sheet can be submitted only when every active student has a status, and every Present mark is filled.

## 5. Visibility rules (server-enforced)

| Viewer | Draft marks | CT / performance | Term exam marks | Result versions |
|---|---|---|---|---|
| Teacher (assigned) | Own sheets: read/write until submitted; read after | Own sheets | Own sheets | Read computed and published for own classes (preview) |
| Head teacher | Read all | Read all | Read all | Read all; approve |
| Admin | Read all; write with reason after approval | Read all | Read all | Read all; publish, correct, lock |
| Student / guardian | Never | Never individually; only as a contribution line in a published term result if the school enables it | Only inside a published result | Published, non-superseded only |

The existing `/exams/me/results` and `/exams/children/:id/results` will return only result versions whose assessment visibility allows it. A published class test with `hidden` visibility is no longer listed (closes E1-06).

## 6. Validation of a blueprint (on activation)

A version cannot be activated unless:
1. **Assessments and terms:** every assessment key is unique and belongs to a term of the same session.
2. **Contributions:** every subject's contributions reference existing assessments and sum (`worth`) to the subject's term full marks.
3. **Best-N:** N ≤ the number of source assessments.
4. **Components:** for each assessment, component full marks are positive. When a pass mark is set, it is ≤ the component's full marks.
5. **Combined subjects:** each `combined_key` groups at least two subjects of the same role.
6. **Optional subjects:** each student has at most one, chosen from the blueprint's optional subjects.
7. **Annual weights:** these sum to 100, and every weighted term exists.
8. **Grading scheme:** bands cover 0–100 without gaps or overlaps, and exactly one band has a point of 0.
9. **Coverage:** every subject of the class is in the blueprint, or explicitly `non_graded`.

Validation errors name the subject, assessment and rule ("Physics · Term 1: CQ + MCQ + Practical = 90, expected 100").

## 7. Versioning

- **Editing:** a draft version is freely editable. Activation validates and freezes it.
- **New version after marks exist:**
  - New sheets use the new version.
  - Existing sheets keep their version unless an admin migrates them; a migration is allowed only when every existing mark maps one-to-one, and it is audited.
  - Results always record their version.
- **Copy forward:** "Use last session's blueprint" copies a retired version into a new draft for the new session (the main time saver each year).

## 8. Example configurations

Each example is a complete definition the wizard can produce. The calculations are worked in [the rules document](E1_RESULT_CALCULATION_RULES.md#8-worked-examples).

**School A — CT 20 % + term exam 80 %, three terms, weighted annual:**
- Terms: First Term, Half-Yearly, Annual.
- Assessments per term: CT1, CT2 (20 marks each), Term exam (100).
- Formula `ct20_final80` (CTs averaged, scaled to 20; term exam scaled to 80).
- Annual: weighted 25 / 25 / 50 at subject level.

**School B — best two of three CTs:**
- Like A, with `best2ct_final`.

**School C — final only (today's model):**
- One assessment per term, `final_only`.
- Annual: `final_only` (the annual exam decides).
- The migration produces exactly this for every existing exam.

**School D — SSC-style components in Classes 9–10:**
- Physics, Chemistry, Biology and Higher Mathematics: CQ 50 (pass 17), MCQ 25 (pass 8), Practical 25 (pass 8).
- Other subjects: CQ 70 (pass 23), MCQ 30 (pass 10).
- Bangla and English: 1st + 2nd papers combined.
- Higher Mathematics or Agriculture as the student's optional subject.
- *These numbers are a typical configuration, not an authority. Confirm them with the school.*

## 9. Migration of existing data (no rewriting)

Migration 1501+ will:
1. **Terms:** create one `exam_terms` row per distinct (session, exam name, exam type) of the existing exams, ordered by the earliest `created_at`.
2. **Blueprints:** create, per session and class, a **legacy blueprint v1** with one assessment per existing exam name, `final_only`, one `TOTAL` component per subject with the exam's full and pass marks.
3. **Link existing exams:** set `term_id`, `assessment_key` and `blueprint_version_id` on every existing exam. This also replaces the cross-section name match used for class positions (E1-05).
4. **Mark statuses:** set every existing mark to `present`, and every sheet of a published or locked exam to `approved`.
5. **Result versions:** create **v1** for every existing published or locked result, with a snapshot of the current bands and the stored values exactly as published. Nothing is recomputed.
6. **Audit:** record an audit event `exam.blueprints_migrated` with counts.

The upgrade must show identical `exam_results`, `exam_marks` and grade values before and after, checked by row fingerprints in the same way as the Payment Safety upgrade test.

## 10. Permissions

| Permission | Roles (default) | Purpose |
|---|---|---|
| `exams:manage` (existing) | admin | Terms, blueprints, sheets, publication, corrections |
| `exams:review` (new) | head_teacher, admin | Return or approve sheets; approve computed results |
| `exams:enter_marks` (existing) | teacher, admin | Own sheets |
| `exams:read` (existing) | admin, head_teacher | Oversight |
| `exams:read_self`, `exams:read_children` (existing) | student, guardian | Published results only |

Publication stays admin-only by default. A school can grant `exams:manage` publication to the head teacher through the existing permission editor.
