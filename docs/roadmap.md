# Fly_Birds Roadmap

The [project brief](PROJECT_BRIEF.md) is the source of truth for each milestone's detailed scope. [Progress](PROGRESS.md) records verified acceptance criteria and test evidence.

| Milestone | Scope | Status |
|---|---|---|
| M0 | Foundation, health endpoint, CI, initial documentation | Merged into main; local Compose runtime verified |
| M1 | Authentication and school configuration | Merged into main; 12/12 brief criteria locally verified |
| M2 | Academic structure | 6/6 merged into main |
| M3 | Students and guardians | 6/6 merged into main |
| M4 | Teachers and staff | 4/4 merged into main |
| M5 | Attendance | 5/5 locally verified and merged into main |
| M6 | Routine | 3/3 locally verified and merged into main |
| M7 | Examinations and results | 8/8 locally verified and merged into main |
| M8 | Fees and accounts | 6/6 locally verified and merged into main |
| M9 | Notices and notifications | 7/7 locally verified and merged into main, plus the approved Daily Class Diary extension |
| M10 | Admin control center | 5/5 locally verified and merged into main |
| M11 | Analytics | 5/5 locally verified and merged into main |
| M12 | UI and product polish | 5/5 locally verified and merged into main |
| M13 | Final release | 7/7 locally verified and merged into main (PR #19) |
| M14 | School onboarding: local provisioning (`school-provision`) | Merged into main (PR #24) |
| Payment safety | Online payment off by default; administrator-controlled simulation for test installations only; legacy simulated payments labelled, excluded and voidable | Merged into main (PR #25) |
| E1 | Examination system architecture and UX design (documents only, `docs/exams/`) | Merged into main (PR #26) |
| E2 | Exact result engine (`internal/results/calc`) and the read-only `exam-verify` tool | Merged into main (PR #27) |
| E3 | Mark statuses (absent, exempt, incomplete, withheld) and the sheet workflow (submit, return, approve), versioned per-cell mark writes | Merged into main (PR #28) |
| E4 | Teacher marks workspace: My exam sheets queue, keyboard grid, autosave with conflicts and recovery, paste/CSV preview, history, mobile one-at-a-time | Merged into main (PR #29) |
| E5 | Exam terms, versioned assessment blueprints (CT + final, best-N, components, combined papers, optional subjects, school rules), the term setup wizard, component mark entry, provisional term results, class tests hidden from students, class positions by term; existing exams linked without value changes (migration 1502) | Merged into main (PR #31) |
| E6 | Immutable result versions with snapshots, the result review and publication centre, head-teacher approval bound to the reviewed inputs, corrections that supersede and never delete, withdrawal, term and annual (final only, equal, weighted) results for families; published results recorded as version 1 without value changes (migration 1503) | Merged into main (PR #32) |
| M15 | Enrolment history and year-end promotion ([details](ENROLMENT_AND_PROMOTION.md)). Stage A: one enrolment per student per session with an append-only change log, marks and results tied to the year they belong to, history-safe rosters and reports (QA-06), history API and export (migration 1701). Stage B1: class progression, promotion batches with recommendations from the published annual result, head-teacher approval bound to the inputs, one-transaction apply and revert, repeat/graduate/withdraw/transfer, login follows withdrawal (QA-03), per-session fee concessions with a review, generic moves guarded (migration 1702). Stage B2: screens | Stage A merged (PR #34); stage B1 locally verified on feature/m15-stage-b-promotion; B2 not started |

The original seven-day M1–M13 target is a planning target. Testing, security, acceptance criteria, and milestone approval take priority. Future features outside M0–M13 are listed in the README and will not be implemented during these milestones.
