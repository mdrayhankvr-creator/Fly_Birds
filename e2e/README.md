# Browser suites

End-to-end checks that drive a real headless Chrome or Edge over the Chrome
DevTools Protocol against a running Fly_Birds stack. They need Node 22 or
later and no npm packages.

| Script | What it checks |
|---|---|
| `suite.mjs` | Landing and login pages, every role's home dashboard and navigation, the attendance register flow, module screens, Bangla and English, mobile layout, dark theme, uncaught exceptions and Content-Security-Policy violations (106 checks). |
| `whitelabel.mjs` | School branding: name, colours and logo applied across the shell, then restored (23 checks). |
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

## Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `E2E_BASE_URL` | `http://127.0.0.1:5173` | Address of the web app. |
| `DEMO_PASSWORD` | (required) | Demo account password. |
| `E2E_BROWSER` | auto-detected | Path to a Chrome or Edge executable. |
| `E2E_INSECURE_TLS` | unset | `1` accepts a self-signed certificate (local HTTPS only). |

Screenshots and `suite.json` results are written to `e2e/shots/`, which Git
ignores. Each script exits non-zero when any check fails.
