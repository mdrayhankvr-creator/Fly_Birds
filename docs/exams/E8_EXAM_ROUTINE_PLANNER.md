# E8 — Term exam routine planner

Branch `feature/e8-exam-routine`. Migration 1804. It follows the
[routine and PDF spec](E1_ROUTINE_AND_PDF_SPEC.md) (sections 2–5 and 9) and
the E8 row of the [implementation roadmap](E1_IMPLEMENTATION_ROADMAP.md). The
per-section exam routines of migration 0601 and their endpoints are
unchanged.

## 1. What a school does with it

1. **Rooms** (once): each exam room with its kind and, optionally, its seats.
   - A **hall** can seat several classes.
   - A **lab** is needed for practicals.
2. **Start the routine** of an exam term (administrator):
   - version 1 is a draft with two sittings, Morning 10:00–13:00 and
     Afternoon 14:00–17:00;
   - it has one theory paper for each subject of each class that has an exam
     in the term (or of every active class with sections);
   - its dates are the term's, and can be narrowed.
3. **Plan.** The administrator:
   - adds sittings and papers (a practical, a viva, a second paper);
   - places papers by hand (date, sitting, a room for each section);
   - adds invigilation duties;
   - asks for a **draft**, which places everything not yet pinned.
4. **Approve.** The administrator submits. The head teacher approves the
   routine, or returns it with a reason. The administrator can also take it
   back to draft.
5. **Publish.** The administrator publishes:
   - students see their papers and their own room, with the next exam on
     their dashboard;
   - teachers see their invigilation duties, with the next one on their
     dashboard.
6. **Amend.** A published routine is never changed. The administrator amends
   it with a reason:
   - this starts the next version as a copy;
   - the published version stays in force until the amendment goes through
     approval and publication;
   - the planner shows the difference, paper by paper ("Class 7 Mathematics:
     14 Jan → 16 Jan");
   - for 7 days, students see "Amended on …" and the changed papers marked;
   - the old version is kept as *Replaced* for staff.

## 2. Rules

The same check runs on manual placement, on the draft, and at submission,
approval and publication (`backend/internal/routine/planner`).

### Hard rules

These block a manual placement, and submission, approval and publication.

| Rule | Example message |
|---|---|
| A weekly off day (shared with attendance, QA-12) | "2027-01-08 is a weekly off day" |
| A published holiday | "2027-01-12 is Shab-e-Barat (holiday)" |
| A date outside the routine's dates or the term | "… is outside the routine's dates" |
| Two papers of one class in one sitting | "Class 9 already has Physics this sitting (Chemistry)" |
| Students taking both of two optional subjects in one sitting | "14 Class 9 students take both Higher Math and Biology" |
| A room over its seats | "Room 204 holds 40; 78 assigned" |
| Two classes in a room that is not a hall | "Room 101 holds 2 classes but is not a hall" |
| A practical outside a lab | "Class 9 Chemistry practical must be in a lab; Room 101 is not one" |
| A paper longer than its sitting | "Class 9 Physics needs 180 min; Morning is 150 min" |
| One invigilator in two rooms at once | "Mr Karim is in Room 101 and Room 204" |
| A room no longer in use | |
| At submission: a paper with no date, or a section with no room | "Class 6 English has no date" |

A few details:
- **Students** follow their subject choices (E5). A class's optional subjects
  can share a sitting when no student takes both. While nobody in the class
  has chosen, everyone takes every optional subject, as elsewhere.
- **Sections** with nobody sitting a paper need no room.
- **Rooms** may hold two sections of one class when they fit.

### Soft rules (warnings only)

- more than one paper a day for a class;
- a teacher invigilating their own subject;
- a room with no invigilator;
- a duty in a room that holds no paper then.

## 3. The draft

The draft is deterministic and greedy, as the spec describes:
1. **Order:** the papers are ordered by how few sittings can take them, then
   the busiest classes first, then class, subject code, paper number and
   kind.
2. **Placement:** each paper goes to the earliest open sitting with no hard
   conflict, preferring days without another paper of the class.
3. **Rooms:** each section gets a room:
   - first a free room (classrooms, then halls, then labs, by name);
   - then a hall, or a room this class already uses that still has seats;
   - practicals get labs only.
4. **Pinned papers** (placed by hand) never move.
5. **Invigilators:** each room in use gets one invigilator. It is an active
   teacher who is free in that sitting, preferring one who does not teach a
   subject sat there, then the fewest duties.
6. **Report:** papers it could not place are reported with the rule that
   blocked them most often, for example "no sitting is long enough".

It never breaks a hard rule to make a paper fit. The same input gives the
same draft, and drafting again changes nothing. The screen says it can miss
an arrangement a different order would find, and the draft is reviewed and
approved by people.

## 4. Data (migration 1804)

| Table | Holds |
|---|---|
| `exam_rooms` | name (EN and BN), kind (classroom, lab, hall), optional seats, in use |
| `routine_versions` | one routine per term, kept as versions: `draft → submitted → approved → published → superseded`. Also: the reason (required for an amendment), the version it amends, dates, instructions, who submitted, approved and published, and when, and the published content's SHA-256 |
| `routine_sittings` | per version, so a published version's times never change |
| `routine_papers` | class, subject, kind (theory, practical, viva, oral), paper number, length, needs a lab, date and sitting, pinned |
| `routine_paper_rooms` | the room each section sits a paper in |
| `routine_duties` | one invigilator, one room, one sitting; nobody twice in a sitting (unique) |

The database guards the workflow too:
- triggers refuse any change to a version's content once it leaves draft,
  any status move outside the workflow, and deleting anything but a draft;
- at most one version of a term is in progress, and one is published.

Permissions:
- `routine:approve` is new, held by head teachers;
- planning and publishing need `routine:manage` and the administrator role;
- reading needs `routine:read` (administrators and head teachers).

Audit events:
- `routine.version_created`, `routine.version_updated`, `routine.version_deleted`;
- `routine.room_saved`, `routine.sitting_saved`, `routine.sitting_deleted`;
- `routine.paper_added`, `routine.paper_placed`, `routine.paper_deleted`;
- `routine.duty_added`, `routine.duty_deleted`;
- `routine.generated` (placed, unplaced, kept and duties);
- `routine.version_submitted`, `routine.version_returned`,
  `routine.version_approved`, `routine.version_published`;
- `routine.version_amended` (with the reason).

## 5. API

All routes need the routine module.

| Method and path | Who |
|---|---|
| `GET /routines/rooms`, `GET /routines/terms`, `GET /routines/versions/{id}`, `GET /routines/versions/{id}/pdf` | `routine:read` |
| `POST/PATCH /routines/rooms…`, `POST /routines/versions`, `PATCH/DELETE /routines/versions/{id}`, sittings, papers, duties, `generate`, `submit`, `withdraw`, `publish`, `amend` | `routine:manage`, administrator |
| `POST /routines/versions/{id}/approve`, `…/return` | `routine:approve` (head teacher) |
| `GET /routines/terms/me`, `GET /routines/terms/me/{versionId}/pdf` | `routine:read_self`, students |
| `GET /routines/duties/me` | `routine:read_self`, teachers |

How the routes answer when something is refused:
- **A placement** that breaks a rule gets `409 routine_conflict`, and
  `error.issues` lists the rules with their codes and messages.
- **A step** that the check blocks gets `409 routine_not_ready`.
- **A version past draft** gets `409 routine_state`.
- **No renderer:** the PDFs answer `503 documents_unavailable`.

The OpenAPI file documents every route.

## 6. PDFs

The routine PDFs use the E7 Typst renderer and the school header (logo or
initials, names, EIIN, contacts), in English or Bangla with Bangla digits:
- **Class routine:** one page per class. Each row has the date with its
  weekday, the sitting and time, the paper and its code, the length, and the
  room of each section.
- **Room schedule:** one page per room in use. Each row has the classes,
  sections and papers, the students, and the invigilators.
- **Invigilation duties:** the roster, landscape.
- **Every page:** the holidays in the period, the instructions, the amendment
  reason, and the publication date and version. A version that is not
  published carries a draft watermark, and a replaced one says so.
- **The student's routine:** their section's papers and rooms only, from the
  published version.

PDFs are rendered on request (a routine is a few pages), not stored.

## 7. Differences from the spec, and limits

- **Legacy routines.** The per-section exam routines (migration 0601) are not
  converted into term versions. They keep their tables, screens and
  endpoints, and the planner does not read them, so nothing a school has
  published changes. Exams keep their `routine_id`.
- **Slots.** A paper carries its own date, sitting and rooms, so "a paper
  scheduled twice" cannot happen.
- **Soft rules.** These are always on, not switchable per school. "Rest days
  before a heavy subject" and "the day after a long holiday" are not
  implemented.
- **Rooms.** No home rooms: rooms are allocated in name order.
- **Publication** posts no notice and sends no notification. Students see an
  amendment through the "Amended" marks.
- **PDFs** are rendered on request, not stored as files.
- **Guardians.** There is no guardian view: families use the student's
  account (guardian logins are retired).
- **The draft** can fail where another order would succeed. It reports what
  it could not place.
