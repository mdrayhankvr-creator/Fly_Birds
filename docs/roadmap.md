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
| E3 | Mark statuses (absent, exempt, incomplete, withheld) and the sheet workflow (submit, return, approve), versioned per-cell mark writes | Locally verified on feature/e3-exam-workflow-foundation; push, CI and merge pending |

The original seven-day M1–M13 target is a planning target. Testing, security, acceptance criteria, and milestone approval take priority. Future features outside M0–M13 are listed in the README and will not be implemented during these milestones.
