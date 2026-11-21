# Religion Education choices and cohort lessons: design review

This is an opt-in school configuration. It does not classify any person's
religion, infer a choice from identity or section, or seed a school curriculum.

## Existing behavior and compatibility

- `academic_class_subjects` maps required and optional subjects to a class.
  `student_subject_choices` stores one optional choice per student/session.
  `student_takes_subject` controls marks and result applicability.
- The current class planner stores draft and published JSON versions and
  publishes ordinary section rows to `class_routines`. Its 45-minute lunch,
  weekly off days, room and teacher validation must remain in force.
- `student_attendance` records ordinary section attendance. A cohort lesson
  needs separate subject attendance; ordinary attendance must remain intact.
- The read-only showcase check on 2026-11-20 found one required generic
  `RELIGION` mapping (code 111) for each of classes 6–10; no class-10 optional
  mapping, no 9–10 student group choices, and no ordinary routines. These are
  observations, not migration defaults. Admins must review them again.

## Implemented data and rules

1. Extend the existing `student_subject_choices` role to `religion`, keeping
   the existing `(student_id, session_id, role)` key. Add an explicit,
   school-controlled `class_religion_subjects` list of eligible required class
   mappings. A religion choice references an academic subject, never a
   religious affiliation. No choice is filled by migration or admission.
2. A configured Religion Education option applies only to the student who
   explicitly chose it. Unconfigured schools retain their present behavior.
   The choice must match the student's enrolment, active class mapping and
   eligible group. A choice or option change after a result version exists
   needs a separate reviewed recalculation workflow, so this milestone blocks
   that edit. Fourth-subject queries must filter `role='optional'`.
3. Add cohort lessons to the existing class planner's draft/version/publish
   workflow. Each lesson has one teacher, subject, time and room plus an
   explicit set of student IDs. Membership is checked against the session,
   class, subject applicability and original section. Parallel lessons may
   share a period only with disjoint students and different teachers/rooms.
   Published lesson rows retain historical versions; ordinary section
   routines are not silently converted into cohort lessons.
4. The planner's current 45-minute lunch, off-day, availability and registered
   room checks apply to cohort lessons. Publishing must recheck all conflicts
   in the locked transaction. Teacher roster access is limited to the lesson's
   assigned employee. Subject attendance is keyed by lesson and student and
   keeps the original section; it never rewrites daily section attendance.

## Operator guide

1. In **Exams > Setup > Religion Education**, review each class's required
   mappings. Create school-approved course subjects and map them as required.
   Review generic code 111 before approving separate options; migration 1817
   does not change that mapping.
2. Mark the approved mappings as Religion Education options and choose a
   course explicitly for each student. No student or section default is
   inferred. Students and linked guardians can request a different approved
   course from the academic profile. The Admin reviews requests in the same
   setup page. A request never changes the student's course by itself.
3. In **Class Routine Planner**, enter school hours, teaching days, period
   length, lunch start, teacher availability and weekly frequencies. Register
   rooms and capacities, and assign teachers to subjects. Add combined lessons
   by choosing the class, subject, teacher, period, room and exact students.
   Parallel lessons need separate student lists, teachers and rooms. A
   Religion Education or fourth-subject frequency is required for a section
   only when at least one current student there takes that subject.
4. Check preview issues and the 45-minute lunch, save a draft, and explicitly
   confirm publication. The planner rechecks roster eligibility and conflicts
   in a locked transaction. Prior cohort lesson versions and their subject
   attendance remain available. Ordinary daily section attendance is separate.

Teachers and students see their assigned or joined combined lessons on their
class routine page. The assigned teacher can select a lesson date and record
subject attendance for its exact roster; students see only lesson summaries.
The protected endpoints are `GET /routines/cohorts/me`, `GET
/routines/cohorts/:id/roster?date=YYYY-MM-DD`, and `PUT
/routines/cohorts/:id/attendance`.

## Deployment and configuration review

- Migration 1817 is in Rayhan's 1801–1849 range, immediately after main's
  migration 1816. It adds five tables and no seeded school rows or choices.
  The clean-slate schema guard and table policy include all five tables.
- An Admin must create school-approved subject records and class mappings,
  then explicitly configure which mappings are Religion Education options.
  Generic code 111 must be reviewed so it is not accidentally taught in
  addition to separate courses. No code or migration guesses its replacement.
- The Admin must collect approved individual choices, assign qualified
  teachers, rooms, availability and school hours, and preview cohorts before
  publishing. An unresolved student cannot join a course cohort; plan
  conflicts block publication. Review unresolved students before publishing
  a complete Religion Education timetable.
- The live showcase, its accounts, routine data, branding and tunnel are
  outside this implementation. Any migration or configuration of that school
  requires a later backup, preflight and owner-approved deployment.
