# Marksheet v2 and automatic admit cards

Branch `feature/marksheet-admit`, from `main` `ec7b94d7` (E8 merged, PR #44).
Migrations 1805 (class teachers, comments, signatures) and 1806 (admit
cards). Both documents use the E7 Typst renderer; the E7 and E8 document
rules (published data only, Bangla digits and dates, school header) apply.

## 1. Marksheet v2 (ফলাফল পত্র / Academic Transcript)

Replaces the E7 marksheet layout. A4 portrait, one page per student,
print-friendly (thin rules, light tints that survive grey printing), in
English or Bangla.

| Part | What it shows |
|---|---|
| Header | Logo (or initials), school name in both languages, EIIN, address and contacts, the verification QR code |
| Title band | "Academic Transcript / ফলাফল পত্র" (the document's language first), the exam (term or annual) and session |
| Student | The uploaded photo (a placeholder when none), name in both languages, father's and mother's names, date of birth, student ID, class, section, roll, session, exam |
| Subjects | Code and name; obtained and full marks per component (CQ, MCQ, practical, or "Exam" for an ordinary paper); the class-test share; total; highest in the class; letter grade; grade point. Combined papers (a shared combined key in the blueprint, such as Bangla 1st and 2nd paper) take one row per paper, and their total, highest mark, grade and point span both rows. The optional (fourth) subject comes last, under a labelled row: "only the grade point above 2.00 is added to the GPA; it never fails the result" (the calculation engine's rule) |
| Summary | Total marks, GPA, GPA without the fourth subject, letter grade, result, failed subjects, class and section positions "out of N", term attendance, publication date |
| Below | Class teacher's comment (two ruled lines to write by hand when there is none) and the school's grading scale |
| Signatures | The class teacher's signature image and name, the head teacher's (the ID-card signature and title, and the head teacher's name from school settings), and a blank guardian line with a date |
| Footer | Version, publication date and document code |

**Attendance:** days present (present or late) out of working days. Working
days run from the term's start to its end (or to today, if earlier), less
the weekly off days and published holidays (the same rules as attendance).
A term without dates starts the day after the previous term's end (or on
the session's first day) and ends on the day the results were published;
the annual result covers the session. With no attendance recorded the field
shows "—".

**Positions:** "3 of 60" counts every published result of the class (and of
the section).

**What is read live:** the photo, parents, date of birth, attendance, the
comment, the class teacher and both signatures are not part of a published
result. They are read when the document is made and folded into its cache
key, so a changed comment, signature or attendance makes a new document. A
published result never changes; a test checks that its versions stay as
they were.

**The old report card page:** the per-exam report card on My results no
longer prints the page. It offers the marksheet PDF (English and Bangla) of
the exam's term result once that result is published (the student's own
download, or the staff document for the administrator), and says when it
is not published yet.

## 2. Class teachers, comments and signatures

- **Class teacher:** one per section (sections belong to one session). The
  administrator assigns them in a new "Class teachers" card on the Sections
  page; everyone with `academics:read` sees who they are and whether they
  have uploaded a signature.
- **Comments** (`exams:comment`, new; administrator, head teacher, teacher):
  one per student per term or annual result. The class teacher chooses an
  editable preset (English and Bangla; six seeded) or writes their own words
  (up to 400 characters). A preset prints in the document's language; own
  words print as written. The head teacher and the administrator see every
  section, can correct a comment (counted as reviewed) and mark the written
  ones reviewed; a teacher's later change needs reviewing again. Another
  teacher, an accountant and students cannot read them. Every change is
  audited. Screen: Academics → Class teacher comments (`/exams/comments`),
  with the preset editor for the head teacher and the administrator.
- **Signatures:** each member of staff uploads their own on My staff
  profile. A PNG keeps its transparency; in a photo or scan (JPEG, or a PNG
  without transparency) the light paper becomes transparent. The ink is
  trimmed, scaled to fit 600 × 200 pixels and stored as PNG (at most 256 KB).
  The marksheet fits it inside the signature slot.

## 3. Admit cards (প্রবেশপত্র)

Made from a term's **published** routine (E8). Two cards per A4 sheet with a
dashed cut line, in English or Bangla:

- the school header, "Admit Card / প্রবেশপত্র", exam and session;
- the photo, name in both languages, student ID, class, section, roll, and
  the room and seat when they are the same for every paper ("see the
  timetable" otherwise);
- the student's own timetable: date, weekday, time, subject and paper, room
  and seat, the optional subject marked;
- the exam rules (the school's own, one per line in each language, or six
  built-in ones), the head teacher's signature and name, the routine version
  and a verification QR code.

**Seats:** numbered within each room and sitting in a fixed order: class
(display order), section, roll, admission number. Two students never share
a seat, and the same routine always gives the same seats. A hall shared by
several classes continues the numbers.

**Who:** the administrator and the head teacher print any section or a whole
class; a teacher prints the sections they are class teacher of
(`routine:admit_cards`, new). A student downloads their own once the routine
is published (My routine → Exam schedule). Families use the student's
account, as for every other student page.

**Fee hold** (off by default; `routine:admit_settings`, administrator): with
it on, a student with a fee due on or before the cutoff date that is still
unpaid gets no card. Class and section prints leave them out and say how
many (`X-Admit-Cards-Held`); the student is told to contact the office. The
office (`routine:admit_release`: the administrator and the accountant) sees
every student with dues by the cutoff (amount, oldest due) and releases one
for the term with an optional note, or holds them again. Audited.

**Verification:** each printed card gets a 22-character code for its routine
version. `/verify/admit/{code}` (public, like the marksheet's) shows whether
the card was printed from the routine now in force or the routine was
amended later, the term, session, class, section, version and print date,
and the school. No name or identifier.

Cards are made on request (like routine PDFs), not stored.

## 4. API

| Method | Path | Permission |
|---|---|---|
| GET | `/academic/class-teachers?session_id=` | `academics:read` |
| PUT | `/academic/sections/{id}/class-teacher` | `academics:manage`, administrator |
| GET, POST | `/exams/comments/presets` | `exams:comment` (POST: head teacher or administrator) |
| PUT | `/exams/comments/presets/{id}` | `exams:comment`, head teacher or administrator |
| GET | `/exams/comments/sections` | `exams:comment` |
| GET, PUT | `/exams/comments` | `exams:comment` (class teacher of the section, head teacher, administrator) |
| POST | `/exams/comments/review` | `exams:comment`, head teacher or administrator |
| GET, PUT, DELETE | `/staff/me/signature` | `staff:read_self` |
| GET | `/routines/admit-cards/settings` | `routine:admit_cards` or `routine:admit_release` |
| PUT | `/routines/admit-cards/settings` | `routine:admit_settings`, administrator |
| GET | `/routines/admit-cards/terms` | `routine:admit_cards` or `routine:admit_release` |
| GET | `/routines/admit-cards/terms/{termId}/students` | `routine:admit_cards` |
| GET | `/routines/admit-cards/terms/{termId}/pdf` | `routine:admit_cards` |
| GET | `/routines/admit-cards/withheld?term_id=` | `routine:admit_release` |
| PUT, DELETE | `/routines/admit-cards/terms/{termId}/releases/{studentId}` | `routine:admit_release` |
| GET | `/routines/admit-cards/me` | `routine:read_self`, student |
| GET | `/routines/admit-cards/me/{termId}/pdf` | `routine:read_self`, student |
| GET | `/public/admit-cards/{token}` | none (routine module on) |

The legacy report card response (`/exams/me/results/{examId}` and the staff
and guardian forms) gains `marksheet_version_id`.

## 5. Limits

- **Guardian logins** get no marksheet or admit card download: families use
  the student's account (as for every student page since checkpoint 4).
- **Seats** are not chosen by hand and rooms keep no seat plan; a seat is
  the student's place in the fixed order above.
- **Comments** have no history beyond the audit log; the head teacher's
  review is a mark, not an approval step that blocks printing.
- **Signature images** are trimmed and resized but not cleaned further;
  a busy photo background may need cropping before upload.
- **Attendance** counts recorded days; a working day with nothing recorded
  for the student counts as not present.

## 6. Also changed

- **Codes in Bangla documents:** subject codes and student IDs that contain
  letters ("BAN-101", "DEMO-001") keep their digits as written; only
  all-digit codes ("১০১") take Bangla digits. Before, "DEMO-001" printed as
  "DEMO-০০১".
- **Dashboard class strip** (found by the E8 browser check in the
  afternoon): finished periods were dimmed to 75 % opacity, below the
  contrast minimum; they keep full colour now (their tint and the "Done"
  chip still mark them). The horizontally scrolling strip can now take
  keyboard focus (axe `scrollable-region-focusable` at 375px).
- **Office navigation:** a user with `routine:admit_release` but no routine
  pages (the accountant) gets an "Admit cards" entry, so the app knows the
  routine module is on for them.
- **Build:** the translations are split into two long-cached files (exam
  and routine texts in their own), so neither passes Vite's 500 kB warning.

## 7. Verification (2026-11-10, disposable resources only)

- **Go:** `go test ./... -p 1 -count=1` on a freshly created PostgreSQL 17
  database: 30 of 30 packages with tests passed; `go vet` clean; gofmt clean
  (276 files, as committed). New tests:
  - `TestPostgresMarksheetV2`: parents and date of birth (English and
    Bangla), combined papers with per-paper rows, the fourth subject in its
    own rows, "1 of 3" positions, failed subjects, attendance without the
    weekly off day and a published two-day holiday (late counts as present),
    the class teacher assigned only by the administrator, comments (another
    teacher refused; a preset and own words not together; the class teacher
    cannot review; the head teacher reviews two), the signatures and their
    images, the cache (same document, no new render), a changed comment and
    a new signature making new documents, a teacher's change undoing the
    review, the published result versions unchanged, no class teacher
    leaving a blank slot.
  - `TestPostgresAdmitCards`: nothing before publication; who may print
    (the class teacher's own section only, a whole class only for the
    administrator and head teacher, no printing by the accountant, settings
    by the administrator only, releases by the office only); card order;
    seats across two classes sharing a hall and in a classroom; the fee hold
    off by default, a cutoff required, the held list with amount and oldest
    due, the class print leaving the held student out; the held student
    refused, released (with a note), held again; a section with everyone
    held; the QR check current, unknown and superseded after an amendment.
  - `TestPostgresAdmitCardData`: Bangla digits, dates and names, room and
    seat on the card or "see the timetable", the school's or the built-in
    rules, QR assets, a card without papers.
  - `TestProcessSignature`: transparency kept, paper removed from PNG and
    JPEG scans, trimming and fitting, blank and broken uploads refused.
- **Frontend:** typecheck and build passed with no large-chunk warning;
  i18n check passed (3,793 English and 3,791 Bangla keys; the two-key gap
  predates this branch); unit tests 18 of 18.
- **Browser,** on `m15b1` rebuilt from this branch with fresh volumes
  (migrations 2.2 s, `up --wait` 125 s with the image build, no restarts),
  demo-seeded, run 14:30–14:41 Dhaka time: `suite.mjs` 117 of 117,
  `result-documents.mjs` 31 of 31, `exam-routine.mjs` 37 of 37, new
  `marksheet-admit.mjs` 35 of 35 (five screens at 1440px and 375px in both
  languages and themes with overflow, contrast and axe; no page errors).
- **Samples** (checked by eye; page images in `images/`):
  `C:\Users\Laptop Click\Fly_Birds_QA_env\pdf-marksheet-admit\`: marksheets
  with photo, fourth subject and combined papers (EN, BN); a failed student
  with a long comment and no photo (EN, BN) and after the comment changed
  (EN); admit cards for the 60-student class (EN, BN, 30 pages each) and the
  student's own (BN). The demo term starts in the future, so these samples
  show attendance as "—"; the Go test covers the counted case.
