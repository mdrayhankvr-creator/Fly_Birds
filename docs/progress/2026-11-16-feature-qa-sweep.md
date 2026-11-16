# Whole-site QA sweep — 2026-11-16

Started `feature/qa-sweep` from `origin/main` after the profile-redesign merge (`839a11b`). Rebuilt the isolated fbqa-cp1b stack with a new demo database and checked 880 sidebar page visits: admin, head teacher, teacher, class teacher, accountant, and student at 1440 and 375 pixels in English and Bangla, light and dark. The seeded sibling phone-login flow passed 29 browser checks. No page-level sideways scroll or raw translation keys were found.

Small fixes: optional own staff avatars now return an empty success for users without a staff record and work for linked administrators/accountants; people lists use stored photo versions before fetching an image, including attendance and fee search. Phone app-bar titles wrap, shared tabs wrap on small screens, and staff list names have room to display. The reusable `e2e/qa-sweep.mjs` records network, console, translation, layout, empty-page, and timing findings while keeping one browser and a short default run.

The first sweep found an absent-signature HTTP 404 on the staff self page and student marksheet PDF HTTP 500 responses in both English and Bangla. The PDF failures were assigned to the separate result/PDF/document workstream.

Verification: frontend typecheck, build, i18n check and all 22 unit tests passed; `go vet ./...` and fresh-database attendance, fees and HTTP API tests passed. On the fresh fbqa-cp1b seed, `suite.mjs` passed 130/130, `narrow-phones.mjs` passed 15/15, and `phone-login.mjs` passed 29/29. The full QA matrix completed 880 page visits; 12 visits reported the same absent-signature 404. A safe action probe opened 27 controls without HTTP or JavaScript errors, and a demo notice was created and edited. The matrix's 24 slow diary readings came from hidden loading placeholders inside closed comment panels; the wait was corrected and a 56-visit teacher/student check found no slow pages. One later scoped role-switch check produced two transient empty/slow head-dashboard readings; the full matrix loaded the head dashboard in all eight variants at about 0.6–0.7 seconds.

Verification and screenshot details are in `C:\Users\Laptop Click\Fly_Birds_QA_env\qa-sweep-report.md` and `shots-qa-sweep/`.

## Follow-up after PR #58

Merged `origin/main` at `a3b8d33` into this branch with a normal merge; there were no conflicts. Main includes the component-less marksheet fix. Added an owner-only signature version to staff profile data. The signature card now requests the image only when that version exists; otherwise it shows its empty state and upload action. Uploading or removing a signature refreshes the profile metadata. The owner-only metadata behavior has a fresh-database Go integration test. No result/PDF/document generation code changed on this branch.

On a new fbqa-cp1b demo database, `qa-sweep.mjs` passed 220/220 sidebar visits with zero findings, `suite.mjs` passed 130/130, and `narrow-phones.mjs` passed 15/15. English and Bangla student marksheet PDF downloads produced no HTTP errors. Frontend typecheck, build, i18n and 22 unit tests passed; `go vet ./...`, staff tests and the staff profile HTTP integration test passed against a separate fresh test database.
