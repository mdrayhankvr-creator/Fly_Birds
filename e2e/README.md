# Browser suites

End-to-end checks that drive a real headless Chrome or Edge over the Chrome
DevTools Protocol against a running Fly_Birds stack. They need Node 22 or
later and no npm packages.

| Script | What it checks |
|---|---|
| `suite.mjs` | Landing and login pages, every role's home dashboard and navigation, the attendance register flow, module screens, Bangla and English, mobile layout, dark theme, uncaught exceptions and Content-Security-Policy violations, guardian facilities in the student account, and the school landing page: identity, sign-in form on the page, no vendor marketing, demo quick sign-in only when the server offers it, the school shown once with no header sign-in link, a 375px phone layout (117 checks). |
| `whitelabel.mjs` | School branding: name, colours and logo applied across the shell, then restored (23 checks). |
| `exam-setup.mjs` | E5 exam setup: a term, a class blueprint built in the browser (class tests, components, combined papers, optional subjects, validation, worked sample), the term wizard, subject choices, component entry, provisional term results, who sees what, four screens in three sizes, both languages and themes, contrast and axe (83 checks). Run it after the suites that expect the seeded section sizes. |
| `result-publication.mjs` | E6 result versions: exam publication into versions, the publication centre, head-teacher approval and administrator publication in three clicks, what students and guardians see (no class-test detail), a correction through the exam page with superseded and verified versions, concurrent publication, grading-band and optional-subject changes after publication, withdrawal, withheld results, the annual result, permissions, four screens in three sizes, both languages and themes, contrast and axe (101 checks). It adds a term and a blueprint version to the demo student's class. |
| `marks-workspace.mjs` | E4 teacher marks workspace: keyboard entry, autosave, conflicts, offline and recovery, paste, submission and review, access, three viewports, both languages and themes, contrast and axe, failing saves (49 checks). Prepare the school first with `marks-workspace-setup.mjs`, which adds students to the demo sections, so run the other suites before it; see docs/exams/E4_MARKS_WORKSPACE.md §8. |

## Requirements

- A stack whose database holds a **freshly seeded demo school** (the suite
  expects today's attendance register to be empty). Never point the suites at
  a real school.
- `DEMO_PASSWORD`: the password given to `seed-demo`.

```sh
# Production-style stack on http://localhost:8081 (see docs/DEPLOYMENT.md)
docker compose --env-file deploy/<qa>.env -f deploy/compose.prod.yaml exec -T -e DEMO_PASSWORD backend /app/seed-demo -confirm

DEMO_PASSWORD='...' E2E_BASE_URL=http://localhost:8081 node e2e/suite.mjs
DEMO_PASSWORD='...' E2E_BASE_URL=http://localhost:8081 node e2e/whitelabel.mjs
```

## Demo quick sign-in (local testing only)

The API can offer one-click sign-in for the five seed-demo staff and student
accounts, for the owner's local testing. It is off unless the API starts with
`DEMO_QUICK_LOGIN=true`; `deploy/compose.prod.yaml` never passes that variable
and school provisioning never sets it. Turn it on for a disposable stack with
an override file kept outside the repository:

```yaml
# quick-login.override.yaml
services:
  backend:
    environment:
      DEMO_QUICK_LOGIN: "true"
```

```sh
docker compose -p <qa> --env-file <qa>.env -f deploy/compose.prod.yaml -f quick-login.override.yaml up -d
```

The server signs in only accounts that seed-demo recorded in `demo_records`
at the demo address, only for requests addressed to `localhost` or a loopback
address from a page on such an origin, and audit-logs each sign-in as
`auth.demo_quick_login`. No password reaches the browser.

## Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `E2E_BASE_URL` | `http://127.0.0.1:5173` | Address of the web app. |
| `DEMO_PASSWORD` | (required) | Demo account password. |
| `E2E_BROWSER` | auto-detected | Path to a Chrome or Edge executable. |
| `E2E_INSECURE_TLS` | unset | `1` accepts a self-signed certificate (local HTTPS only). |
| `E2E_BROWSER_TIMEOUT_MS` | `30000` | How long one browser start may take to become ready. |
| `E2E_BROWSER_ATTEMPTS` | `2` | Browser starts to try before the script stops. |

## Browser startup

`cdp.mjs` starts a private browser for each `launch()` and connects only to
that browser:

- **Port.** The browser picks a free DevTools port and records it in its own
  throwaway profile (`DevToolsActivePort`). The `port` that scripts pass is
  accepted but unused, so a leftover browser or another program on a fixed
  port can never answer instead.
- **Readiness and errors.** Readiness is polled up to `E2E_BROWSER_TIMEOUT_MS`.
  A browser that cannot be started, exits during startup or never becomes
  ready fails that attempt at once, with the last lines of its own error
  output. One failed start is retried; if every attempt fails, the script
  stops with an error and runs no checks.
- **Lost connection.** If the browser exits or its connection drops during a
  run, pending and later commands fail with "browser connection closed"
  instead of hanging.
- **Cleanup.** Only the browser process this run started is stopped (on
  `close()`, on a failed start, or when the script exits), and its profile is
  deleted. `E2E_BROWSER` must point to an existing file; a wrong path is an
  error, not a silent fallback to another browser.

`node --test e2e/cdp.test.mjs` checks this behaviour. Most cases use
`testdata/fake-browser.mjs`, a stand-in that can start slowly, crash, hang or
drop its connection; one case starts the real browser three times while
another program holds the old fixed port.

Screenshots and `suite.json` results are written to `e2e/shots/`, which Git
ignores. Each script exits non-zero when any check fails.
