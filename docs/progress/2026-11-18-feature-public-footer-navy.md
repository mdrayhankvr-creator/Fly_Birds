# Public footer navy redesign — 2026-11-18

Branch: `feature/public-footer-navy` from `origin/main` at `93ac4a3`.

- Gave the public footer the same accessible navy and warm accent palette in light and dark mode, with a four-column desktop layout and two-column tablet layout.
- Kept the school's configured logo, bilingual name, tagline, footer description, EIIN, address, email, map and social links. Bilingual default tagline and description appear only when the school has not configured them. The copyright year follows Asia/Dhaka time, and the requested EN/BN developer credit is small and separate.
- Published only the school contact phone already entered by an administrator in public-page settings. No student, guardian or staff contact data is returned. Empty contact fields and unpublished legal links are hidden; section links appear only when their target section exists.
- Extended the public-page integration test and public-design browser checks for school phone, live footer links, both themes and languages, 320–1440px widths, empty data and axe WCAG A/AA. Updated the release-suite credit assertion.

Verification: frontend typecheck/build, i18n and 23 unit tests passed; Go vet and the focused Postgres public-page integration test on a fresh disposable database passed; `go test ./internal/school` passed. On a disposable `fbqa-cp1b` demo database, public-design passed 130/130, suite passed 130/130, whitelabel passed 23/23, and public-only narrow-phones passed 3/3 (10 pages per language at 320/360/375px). A repeated suite run on a reused demo database failed attendance setup checks; it passed on a fresh seed. The QA app and its original database were restored (readyz 200; 57 migrations). Disposable test databases were retained without deleting Docker data.
