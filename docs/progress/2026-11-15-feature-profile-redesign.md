# Profile redesign — 15 November 2026

- Built new student and staff profile headers, summary cards and keyboard-accessible tabs. The layouts use the school colour, adapt to small screens, and support English, Bangla, light and dark themes.
- Student tabs now group personal and guardian details, current subjects and results, a month attendance calendar, fees and available documents. Staff tabs group contact and official fields, teaching assignments and today's routine, attendance, and documents.
- Added scoped student profile and monthly attendance access for assigned teachers. The server removes student personal and login fields from teacher responses and keeps ID cards and fee data outside that view. Heads may read a student's fee statement; teachers cannot. Staff visibility settings remain enforced by the server; employee IDs are limited to administrators, heads and the owner.
- Reused existing analytics, fee, results, admit-card, photo and staff profile APIs. No migration was added.
- Scoped Go tests, `go vet`, frontend typecheck/build, i18n and 20 unit tests passed. The final fresh fbqa-cp1b seed passed `suite.mjs` (125/125), `profiles-safety.mjs` (50/50) and `profile-redesign.mjs` (34/34). Eight screenshots are indexed in the QA folder.
