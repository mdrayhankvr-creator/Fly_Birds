# E1 — Exam routine and PDF specification

Covers the smart exam routine (calendar, planner, conflicts, generation,
publication, amendments, student display) and the document system (automatic
report-card PDFs, routine PDFs, bulk export, verification, immutability). UX
details are in the [UX spec](E1_PREMIUM_UX_SPEC.md) (screens 8, 10, 12, 13).

## 1. Current routine model and limits

From migration `0601_routines.sql` and `routine/repository.go`:
- **Ownership:** one `exam_routines` row per **section**, with a publish flag. Writes are admin-only, every write is audited, and a published routine cannot be edited.
- **Slots:** `exam_routine_slots` has **one slot per subject** (`UNIQUE(routine_id, subject_id)`), so theory and practical of one subject cannot both be scheduled.
- **Conflict check** (same date, overlapping minutes): the same section, or the same room in any routine. Nothing else is checked: teachers, invigilators, class-wide clashes, holidays, weekends and term dates are all ignored.
- **No amendments:** to change a published routine, you unpublish, edit and republish, and no history is kept.
- **Visibility:**
  - students and guardians see published routines of their section;
  - teachers have no exam-routine view.
- **Output:** browser print only.

**Effort:** 856 interactions to build one term for 20 sections (audit, section 3).

## 2. Target routine model

```text
exam_sittings          id, term_id, name ('Morning'), start_minute, end_minute, display_order
routine_versions       id, term_id, version, status ∈ {draft, submitted, approved, published, superseded}
                       reason (amendments), approved_by/at, published_by/at, sha256 of the canonical content
routine_papers         id, routine_version_id, class_id, subject_id, paper_kind ∈ {theory, practical, viva, oral}
                       duration_minutes, needs_room_kind (nullable: 'lab'), optional_group (alternatives taken by different students)
routine_slots          id, paper_id, exam_date, sitting_id (or explicit start/end)
routine_slot_rooms     id, slot_id, section_id, room_id, invigilator_ids uuid[] (optional)
exam_rooms             id, name, kind ('classroom','lab','hall'), capacity (optional), is_active
school_weekly_off_days day_of_week (0–6); default Friday. Shared with attendance (fixes QA-12)
```

- **One routine per term:** a version covers every class of the term, and sections share their class's slot by default.
- **Rooms are per section,** so all sections of Class 9 sit Physics at the same time in different rooms (the usual practice). A school that staggers sections can give a section its own slot.
- **Amendments:** a published version is immutable; an amendment is a new version.
- **Compatibility:**
  - existing per-section routines migrate to a legacy version of their term, with each slot's subject as a theory paper;
  - the existing student, guardian and admin routine endpoints keep working by reading the current published version;
  - an exam's `routine_id` link is replaced by `term_id`.
- **Calendar sources:**
  - `school_events` of kind `holiday` (existing);
  - weekly off days (new, shared with attendance);
  - the term's dates.

## 3. Conflict rules

**Hard conflicts** block approval and publication:

| Rule | Example message |
|---|---|
| A slot is on a holiday, a weekly off day or outside the term dates | "12 Jan is Shab-e-Barat (holiday)" |
| Two papers of the same class overlap | "Class 9 already has Chemistry this sitting" |
| Any student would sit two papers at once (from subject choices, including optional subjects) | "14 Class 9 students take both Higher Math and Biology" |
| One room holds two sections over its capacity, or two classes without a hall | "Room 204 holds 40; 78 assigned" |
| One invigilator is in two rooms at once | "Mr Karim is in Room 101 and Room 204" |
| A paper is scheduled twice, or not at all at publication | "Class 6 English 2nd paper has no date" |
| A paper's duration is longer than its sitting | "Physics theory needs 180 min; Morning is 150 min" |
| A practical needs a lab and the room is not one | "Chemistry practical must be in a lab" |

**Soft rules** warn but don't block. Each can be switched off per school:
- more than one paper a day for a class;
- fewer than *N* rest days before a heavy subject (marked in the blueprint);
- a teacher invigilating their own subject;
- a paper on the day after a long holiday.

The **same validator** runs for manual placement, generation, approval and publication, so a published routine always satisfies every hard rule.

## 4. Draft generation

**What it is:** a deterministic, greedy placement that proposes a draft. It is **not** a promise of a feasible or optimal timetable.

**Algorithm:**
1. **Order the papers** by tightness:
   - first, papers with the fewest feasible cells (lab rooms, long durations);
   - then classes with the most papers;
   - then a stable key (class order, subject code, paper kind).
2. **Cells:** take the available cells (date × sitting) in date order, after removing holidays, off days and cells outside the term.
3. **Place each paper** in the first cell with no hard conflict and the lowest soft-rule penalty. Ties go to the earliest cell. Rooms are allocated per section from the room list, preferring each section's home room when one is configured.
4. **Never move** a paper that the planner placed by hand ("pinned").
5. **Report unplaced papers** with the rule that blocked every cell ("No sitting is long enough for Physics theory").

**Limits** (stated in the UI):
- the greedy method can fail where a different order would succeed;
- the generator reports what it could not place and never weakens a hard rule to make something fit;
- it is checked against fixture schools before its output is trusted, and its result is always reviewed by a person.

**Tests:**
- same input → same output;
- every generated slot passes the validator;
- the lifecycle school (10 classes, 63 papers, 2 sittings, 15 days) is placed fully;
- an infeasible fixture reports the right reasons.

## 5. Approval, publication and amendments

```text
draft ──► submitted ──► approved (head teacher) ──► published (admin) ──► superseded by an amendment
```

- **Publishing:**
  - makes the version visible to students, guardians and teachers;
  - generates the routine PDFs (one per class, and one for the whole school);
  - posts a notice through the existing notices module (optional, pre-filled).
- **Amending a published routine:**
  - creates the next version, with a required reason;
  - publishing it shows a **diff** ("Class 7 Mathematics: 14 Jan → 16 Jan");
  - notifies only the affected sections;
  - supersedes the older PDFs, which are kept for staff.
- **What students see:** the current version, with "Amended on 10 Jan" and the changed rows highlighted for 7 days.
- **Audit events:**
  - `routine.version_submitted`, `routine.version_approved`;
  - `routine.version_published`, `routine.version_amended`;
  - `routine.generated` (with the counts of placed and unplaced papers).

## 6. PDF renderer

Automatic report cards need a server-side renderer, because there is no PDF capability today. **Bangla shaping is the deciding requirement:** conjuncts and vowel signs such as শিক্ষার্থী, পরীক্ষা, বিজ্ঞান, দ্বিতীয়, স্বাস্থ্য, ক্ষ, জ্ঞ, ষ্ট্র must render correctly. That needs OpenType shaping (HarfBuzz or equivalent).

| Option | Bangla shaping | Fidelity with the on-screen card | Operations | Verdict |
|---|---|---|---|---|
| Browser print (today) | Yes (user's browser) | Varies by browser and paper settings | None | Not automatic; not reproducible. Keep only for "Print" |
| Pure-Go PDF libraries | The libraries we know of do not do Indic shaping | New layout code | Small | Rejected unless the spike proves otherwise |
| Typst CLI (single binary) | Yes (shaping engine built in) | New template language | Small static binary, fast | **Alternative** if image size or memory matters |
| Headless Chromium (HTML → PDF), as an internal service | Yes (HarfBuzz) | Same HTML and CSS approach as the app | Larger image; memory per render | **Recommended** |

**Recommendation:** a separate `renderer` service in `deploy/compose.prod.yaml` running headless Chromium:
- **Image:** pinned by digest.
- **Network:** reachable only from the backend on the internal network, with no internet egress.
- **Input:** server-rendered HTML from Go `html/template` with **JavaScript disabled**. The fonts are bundled locally (Noto Sans Bengali and Inter from the existing packages); no remote fonts or images, and the school logo is passed inline.
- **Limits:** per-render timeout of 20 s and memory capped in Compose.

**Spike before building** (pass criteria):
1. The Bangla sample set above renders identically to the reference rendering in the app, checked by eye and by a pixel diff stored as a test fixture.
2. One report card renders in under 3 s, and 40 cards in under 60 s, on the reference laptop; peak memory is under 512 MB.
3. The image size and the cold-start time are recorded.
4. There are no outbound network requests (checked with the container network disabled).

If Chromium fails 2 or 3 on the reference hardware, use Typst and repeat the spike.

## 7. Automatic generation pipeline

```text
publish (transaction) ──► document_jobs rows (outbox) ──► worker (backend loop, SKIP LOCKED)
     ──► HTML from snapshot ──► renderer ──► file_objects (purpose report_card | routine) ──► link to result/routine version
```

**Outbox.** `document_jobs` holds:
- `id`, `kind ∈ {report_card, routine, tabulation, merit_list, bulk_merge}`;
- `subject_id` (the result version or routine version);
- `language ∈ {en, bn}`;
- `status ∈ {queued, running, done, failed}`;
- `attempts`, `last_error`, `file_id`, `created_at`, `finished_at`;
- `UNIQUE(kind, subject_id, language)`, which makes it idempotent.

**Transactional.** Jobs are inserted in the same transaction as publication, so either both happen or neither does. The worker runs like the notification dispatcher (interval loop) and takes jobs with `FOR UPDATE SKIP LOCKED`.

**Source of truth.** The HTML is built only from the **result snapshot**, never from live tables, so a PDF always matches its version.

**Storage:**
- `file_objects` gains purposes `report_card`, `routine` and `export`;
- each file keeps its SHA-256;
- files live in the uploads volume, which the existing backups already include.

**Retries:**
- three attempts with backoff;
- failures show in the Publication Center with a Retry button;
- publication itself never fails because a PDF failed.

**Targets** (reference laptop, confirmed in the spike):
- one card available within 10 s of publication;
- a 40-student class within 2 minutes;
- 1,000 students within 20 minutes.

**Languages:**
- the school's default language is generated at publication;
- the other language is generated on first request and then stored.

## 8. Report-card layout (A4 portrait)

**Page:** `@page { size: A4; margin: 12mm 12mm 14mm }`; 210 × 297 mm; body text 9.5–10 pt; tables 9 pt; numbers tabular.

**Content in order:**
1. **Header (≈ 28 mm):**
   - school logo (max 22 mm high) on the left;
   - school name (EN and BN when available), address and contact;
   - document title "Academic Report Card" / "একাডেমিক রিপোর্ট কার্ড" with the term and session;
   - one thin rule in the school's primary colour, the only decorative use of brand colour.
2. **Student block (2 columns):** name (EN and BN), admission number, roll, class, section, group (if any), session.
3. **Summary strip:** total / full, percentage, GPA, grade, result (Passed / Failed / Withheld), class and section position (if the school shows them), attendance summary (optional, from the attendance module).
4. **Subject table:** columns from the blueprint.
   - **Always:** Subject, Full marks, Marks obtained, Grade, Grade point.
   - **When configured:** CT contribution and Term contribution (CT + Final schools), component columns (CQ, MCQ, Practical, Viva), and a "Remarks" column for component failures, Absent and Exempt.
   - **Grouping:** combined subjects are grouped with a subtotal row; the optional subject is shown last and labelled "Optional (points above 2.00 added)".
5. **Annual section** (annual results): term columns and the method label ("Weighted: 25 % · 25 % · 50 %").
6. **Grading key:** the bands used, from the snapshot, so the card explains itself.
7. **Signature areas:** class teacher, head teacher (or principal), guardian; 40 mm lines with labels.
8. **Footer on every page:**
   - publication date, "Version 2 (revised 14 Jan 2027)" when revised, verification code, and a QR code to the verification URL;
   - page *n* of *m*.

**Rules:**
- the subject table may not split a combined subject across pages, and the header row repeats on every page;
- a normal card fits on one page (up to 14 subjects with components);
- a provisional preview (staff only) has the diagonal watermark "PROVISIONAL — NOT PUBLISHED" and no verification code.

**Bangla digits:** a school option; the default is Latin digits, as in the app.

## 9. Routine PDF layout

- **Format:** A4 portrait per class (landscape for the whole-school matrix).
- **Header:** as on the report card, titled "Examination Routine" with the term and version.
- **Table:** Date (with weekday), Sitting and time, Subject and paper, Rooms per section.
- **Footer:** holidays in the period, instructions (a school-editable text), publication date, version and "Amended" note.
- **Student routine:** one page per section with only the papers that section sits, and optional alternatives labelled.

## 10. Verification and immutability

- **Published documents are never overwritten:**
  - a correction creates a new result version and a new PDF;
  - the old file stays linked to its superseded version and is visible to staff only.
- **Verification code:**
  - 10 random characters from an unambiguous alphabet, unique, not derived from student data;
  - `GET /verify/:code` is public, rate-limited and returns only school name, student name, class, term, GPA, outcome, version and status ("current" or "superseded by version 3");
  - no marks, contact details or enumeration (unknown codes and wrong codes look the same).
- **Hash:** the PDF's SHA-256 is stored, so a printed copy can be checked against the stored file by staff.
- **Access:**
  - the student, the linked guardians and staff with `exams:read` can download a PDF;
  - downloads are audited for staff (`exam.report_card_downloaded`), not for families.

## 11. Bulk export

- **Formats:** per class or section, one merged print-ready PDF (ordered by roll) or a ZIP of single PDFs.
- **Size limit:** merged files above the 10 MB `file_objects` limit are split per section.
- **Reuse:** bulk jobs reuse stored single-card PDFs and never re-render published cards.
- **Cleanup:** exports are kept 30 days and then deleted. Single cards and routines are records and are kept.

## 12. Acceptance criteria

**Routine:**
1. A published routine never has a hard conflict, a holiday slot or a slot outside the term.
2. Generation is deterministic and reports every unplaced paper with a reason.
3. An amendment keeps the previous version and PDFs, notifies only affected sections, and shows the diff.
4. Teachers see their exam duties; students see only their section's papers, including their optional subject.

**Report cards and PDFs:**
1. Publishing a class creates one job per student in the same transaction. Every card becomes available without any manual step.
2. A PDF's values equal the result snapshot; regenerating returns the same stored file.
3. A correction produces a new version and PDF, and the old one stays available to staff.
4. The Bangla sample set renders correctly in the PDF.
5. The verification URL reveals no marks and gives the same response for unknown and malformed codes.
6. A provisional preview cannot be mistaken for a published card (watermark, no verification code).
