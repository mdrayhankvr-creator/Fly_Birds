# Fly_Birds — Development Progress

## Measurement

Each bullet under M0–M13 in [PROJECT_BRIEF.md](PROJECT_BRIEF.md) is one provisional acceptance criterion. There are 83 roadmap criteria, including four for M0. Count a criterion when its implementation has repository evidence and applicable local verification. Recalculate both denominators if the criteria are refined.

- **Overall completion:** 4 verified criteria ÷ 83 total criteria × 100 = **4.8%** (rounded to one decimal place).
- **Current milestone completion:** 4 verified M0 criteria ÷ 4 M0 criteria × 100 = **100%**.

These percentages reflect verified roadmap criteria in this repository. Remote GitHub Actions execution and Docker runtime remain separately unverified.

## Current Status

| Item | Status |
|---|---|
| Current development phase | M0 foundation implemented and locally verified; awaiting review |
| Current milestone | M0 — Foundation |
| Completed acceptance criteria | All four M0 criteria |
| Remaining acceptance criteria | 79 criteria in M1–M13; none of M0 |
| Features implemented | Go/Gin API, optional GORM/PostgreSQL connection, health endpoint, React shell, Bangla/English toggle, frontend health display, Docker Compose configuration, CI workflow |
| Tests passed | `go vet ./...`, `go test ./...`, `npm.cmd ci`, `npm.cmd run build`, `docker compose config --quiet`, local HTTP smoke checks |
| Tests failed | None in the final local verification run |
| Application runnability | Backend and frontend ran locally; frontend, proxy, and API each returned HTTP 200. Docker stack startup unverified because the Docker daemon was unavailable |
| Next planned milestone | M1 — Authentication and School Configuration, only after explicit `next` instruction |

## M0 Acceptance Criteria and Evidence

| Criterion | Status | Evidence |
|---|---|---|
| Project foundation | Complete | Go and React projects, PostgreSQL Compose service, Dockerfiles, environment examples, and local integrated HTTP smoke check |
| Health endpoint | Complete | `GET /healthz` returned `{"data":{"status":"ok"}}`; backend health and error response tests passed |
| CI | Complete (configured) | `.github/workflows/ci.yml` runs Go vet, Go tests, and frontend build; the same checks passed locally. Remote workflow execution is pending a developer push |
| Initial documentation | Complete | README, roadmap, progress record, project brief, and M0 OpenAPI document |

## Verification Details

- `go vet ./...` passed in `backend/`.
- `go test ./...` passed in `backend/`; the configuration and HTTP API packages contain tests. The command reports no test files for the API entrypoint and database connection packages.
- `npm.cmd ci` (using the local offline cache) and `npm.cmd run build` passed in `frontend/`; the build includes TypeScript checking.
- `docker compose config --quiet` exited successfully. The CLI warned that it could not read the local Docker config file, but configuration validation succeeded.
- A locally running backend returned HTTP 200 from `http://localhost:8080/healthz`. A locally running Vite frontend returned HTTP 200 from `http://localhost:5173/` and proxied `http://localhost:5173/api/healthz` with the same healthy JSON response.

## Known Limitations and Blockers

- The Docker daemon was not running in this environment, so container builds, PostgreSQL connectivity, and full Compose startup were not tested.
- GitHub Actions has not run on this branch because no push was performed.
- M0 has no database schema, users, authentication, or demo credentials. Those are later milestone work.
