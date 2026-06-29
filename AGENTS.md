# AGENTS for Jodata

## Project summary
- `jodata` is a monorepo with:
  - `backend/`: Go API/server and business logic
  - `frontend/`: static SPA source artifacts (`index.html` today)
- The backend is the only runtime service and serves the SPA from `frontend/dist` via `JODATA_FRONTEND_BUILD_DIR`.
- Existing command flow is Makefile-driven; avoid introducing alternate ad-hoc startup scripts.

## Canonical commands
- `make build-frontend`  
  Build/copy frontend assets into `frontend/dist`.
- `make run-server`  
  Build frontend then run backend with SPA serving enabled.
- `make run`  
  Alias for `make run-server`.
- `cd backend && go test ./...`  
  Run backend tests.

## Runtime config notes
- API routes are under `/api/*`.
- SPA fallback should always serve `index.html` for non-API routes from build dir when possible.
- Use env vars:
  - `JODATA_FRONTEND_BUILD_DIR` (default `../frontend/dist` for backend process)
  - `JODATA_FRONTEND_INDEX` (optional explicit index override)

## Collaboration defaults for edits
- Keep changes small, deterministic, and scoped.
- Preserve existing architecture unless explicitly asked to rework.
- Prefer editing existing files directly over adding unrelated abstractions.
- Avoid changing unrelated files.
- Do not run destructive git commands unless explicitly requested.

## File safety
- Do not modify unrelated user changes.
- Do not delete files unless requested.
- If uncertain, prefer explicit, minimal diffs over broad refactors.
