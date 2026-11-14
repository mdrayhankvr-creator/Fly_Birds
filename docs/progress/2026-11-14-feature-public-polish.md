# Modern Bold public page polish — 2026-11-14

Branch `feature/public-polish` started at `origin/main` after PR #54. Modern Bold now leaves sign-in closed on the landing page; its header button opens the existing credential form in a focused desktop dialog or phone bottom sheet. `/login` and protected-route redirects open the same card. Escape, close, and browser back dismiss it. Classic and Minimal retain their layouts.

The Modern Bold hero now fills the first screen with school slideshow photos, crossfade, small dot navigation, phone swipe, and hover/focus pause. Reduced motion stops automatic advance. The phone school name is limited to three lines with a smaller inline badge and secondary name. Results links open `/login`.

Verification on a freshly seeded `fbqa-cp1b` throwaway database: frontend typecheck, build, i18n check, and 20 unit tests passed. `suite.mjs` passed 130/130, `phone-login.mjs` 29/29, and `public-design.mjs` 97/97. The focused `narrow-phones.mjs` public crawl passed EN/BN across 10 public routes at 320, 360, and 375px with no sideways scroll or JavaScript errors. Eight review captures at 375px and 1440px, EN/BN, landing/sign-in are indexed in `shots-public-polish/index.md` in the QA environment. The screenshot capture also verified focus wrapping, swipe, and automatic slideshow advance.
