# Synthetic teacher bulk setup

This Admin workflow creates fictional **TEST** teacher profiles and linked sign-in accounts for timetable testing. It targets **15 active teacher profiles total**. Every preview recounts existing active profiles; inactive profiles still reserve their employee codes. Existing teachers and account links are kept as they are.

## Use

1. After migration 1816 is reviewed and deployed, sign in as an Administrator with `staff:manage`, `users:create`, and `permissions:manage`. The Staff and Permissions modules must be enabled.
2. Open **Staff → Synthetic TEST teacher setup** and select **Refresh read-only preview**. Review existing profiles, account link status, proposed names, codes and login IDs, and the fingerprint. Resolve any protected Admin or identifier conflict before continuing.
3. Choose a separate export passphrase of at least 16 characters, confirm it, then select **Create TEST teachers**. Read the confirmation before approving. A changed staff or account state makes the preview stale and requires a new preview. No staff are created on cancellation.
4. Save the automatically downloaded `TEST-teacher-credentials.fbcred` file before leaving the page. It is encrypted in the browser with the chosen passphrase; the passphrase is never sent to the server. Keep file and passphrase separately. The **Open a saved encrypted file** control decrypts locally and downloads a CSV for individual distribution. Protect and remove the unlocked CSV after use.
5. Each new teacher must change their distinct temporary password at first sign-in. The temporary password expires according to the existing authentication policy. A retry with the same request key returns counts only, never passwords. If the file is lost before it is saved, an Administrator must reset the new accounts individually through the supported account workflow.

The candidate email addresses use the reserved `flybirds.test` domain as sign-in IDs. They are not contact addresses. Names in both languages and profile markers explicitly say TEST. The workflow leaves phone, contact email, qualifications, joining date, photo, availability, class teacher duty and subject assignments blank. No school identity, branding, student, result or routine data is changed.

## Before timetable publication

Verify each real staff identity and joining date if replacing TEST data. Collect school-approved qualifications and subject/class assignments. Confirm teacher availability, section assignments, rooms, school hours, period lengths, subject frequency and the 45-minute lunch start. Review the routine planner preview for teacher, section and room conflicts before publishing.

## Safety and recovery

The server locks staff and account tables, rechecks grants, protected Admin state, identifiers and the preview fingerprint, and creates all profiles, accounts, audit events and the retry record in one transaction. A failure rolls the whole run back. The protected Administrator addresses `mdrayhankvr@gmail.com` and `2023331031@student.sust.edu` are checked and cannot become candidates or update targets. This workflow has no bulk undo. If a school decides to remove TEST profiles later, use reviewed per-record procedures after checking routine and attendance references; do not reset the school.

The browser test runner `node e2e/teacher-bulk-setup-local.mjs` starts its own loopback API and Vite server against a newly created `teacher_browser_test_*` database in `m15-testdb`. It disables its temporary database login at the end and leaves the disposable database for inspection. Never aim the browser test at fbshow or another existing school.
