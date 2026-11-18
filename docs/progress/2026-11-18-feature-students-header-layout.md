# Students page header layout

Created `feature/students-header-layout` from `origin/main` at `93ac4a3`
in an isolated Joy worktree. The original checkout and other worktrees were
preserved. The live `fbshow` stack and school data were not accessed.

The Students header displayed five action links. At the `sm` breakpoint the
shared `PageHeader` put its heading and actions in a flex row, while the
action group used `sm:shrink-0`. The action group could consume the available
width, shrinking the title/description to a very narrow column. The header's
`overflow: hidden` then clipped the rightmost actions.

`PageHeader` now accepts an opt-in `stackActions` layout. Only Students uses
it: its heading gets a full row and the five actions wrap within the card.
The default shared layout and all admission, login and setup actions remain
unchanged. Another multi-action page header was checked at tablet width.

Verification: frontend typecheck, production build, i18n check and 23/23
unit tests passed. New `e2e/students-header-layout.mjs` passed 26/26 checks
on a fresh disposable demo database: 1440, 768 and 390px in EN and BN;
heading and description width, title line count, all five visible/clickable
actions, wrapped rows, no page overflow and no browser errors. Screenshots
are in `C:\Users\Laptop Click\Fly_Birds_QA_env\shots-students-header`.

After testing, `fbqa-cp1b` was restored to its original `flybirds` QA
database and app images; readiness returned HTTP 200 and the original DB
still had 57 migrations. The disposable test database was left intact.
