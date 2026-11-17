# Class routine planner

The class routine planner is an administrator workflow for an academic session. It reuses active sections, class-subject mappings, teacher assignments, the active room registry, weekly off days and the existing `class_routines` table. It does not assign a subject, teacher or room automatically. The planner does not edit school settings or create staff, students or accounts.

## Before planning

The school must decide its start and end times, teaching days, lesson duration, transition gap and lunch start. Lunch is exactly 45 minutes. Administrators must enter the approved number of weekly lessons for each class subject, including an explicit zero when a mapped subject is not to be scheduled. A frequency applies to every active section of that class. Teacher assignments and active rooms must exist. The administrator must review teacher availability and enter recurring unavailable intervals. The planner has no separate teacher-duty assignment feature.

The generated grid contains only complete lessons before and after lunch. A lesson must match a grid period exactly. Weekly off days cannot be chosen. The planner checks a teacher, section and room for overlapping lessons, plus teacher and room use in academic sessions whose dates overlap. It reports missing teachers, rooms, frequencies, availability confirmation and empty or invalid schedules. It never fills a free period by itself.

## Preview and publication

1. Open **Routines → Class planner** as an administrator with `routine:manage` and the routine module enabled. Choose the active session.
2. Enter the approved day and frequency inputs. Click **Preview without saving** to load the current sections, subjects, assignments, rooms, off days and current timetable. This request makes no database changes.
3. Place or adjust lessons manually. Use the section, day and teacher filters to review the timetable. Preview again after any change. The section grid shows each teaching period, lunch and free periods. Resolve every issue before publication.
4. **Save draft** stores the proposed plan and leaves the current timetable intact. Once a planner draft exists, the older direct class-routine edit API is blocked for that session; continue editing in the planner.
5. Review the preview's count of current lessons. **Publish version** requires an explicit confirmation. The server rechecks the saved draft against current school data and the preview digest inside one database transaction. A stale or invalid plan is refused. Publication atomically replaces current class-routine rows. The previous timetable is archived as immutable version zero on first publication when legacy rows exist; each published plan is retained as immutable history.

Published versions are retained for audit. To change a published routine, save and confirm a new version. There is no automatic rollback button; a prior version can be reconstructed as a new reviewed draft. The draft is shared by administrators in a session, so coordinate edits and preview again before saving or publishing.

## API and database safety

The `/routines/class-planner/*` endpoints require authentication, `routine:manage`, the routine module and administrator role. `POST /preview` is read-only. `GET /draft`, `PUT /draft` and `POST /publish` use the same authorization. Publication requires `confirmed: true`, a saved draft ID and the current digest. Draft and publication actions are audited.

Migration `1815_class_routine_plans.sql` adds `class_routine_plan_versions` and guards against direct writes to a session with a draft or published plan. It does not update existing class routines. The table is school-life data and is included in the reviewed clean-slate reset schema count. The pending fourth-subject branch uses migration 1814; after both branches merge, the reset schema guard must be reviewed and updated for the combined migration and table counts.

No teacher duty, student record, subject choice, grading, result or owner-managed school setting is changed by this feature. A deployment and real-school timetable require separate owner review.
