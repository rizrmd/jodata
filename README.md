# jodata

Jodata is a Go-native BI engine with Superset-inspired concepts: ingestion -> semantic datasets -> validated chart intents -> dashboards.

## Repository layout

```text
jodata/
  backend/   # Go services (all current API and engine logic)
    cmd/
    internal/
    go.mod
    go.sum
  frontend/  # Browser frontend (single-page app)
    index.html
```

`backend` is the only service process. The frontend is a static asset package consumed by
`backend` at runtime.

## Build and run backend + SPA

```sh
make run-server
```

Default address: `http://localhost:8080`

The backend serves `/` and deep-link routes from a SPA bundle in `frontend/dist`.

Build commands:

```sh
make build-frontend
```

Equivalent manual flow:

```sh
cd frontend
mkdir -p dist
cp index.html dist/index.html
cd ../backend
JODATA_FRONTEND_BUILD_DIR=../frontend/dist go run ./cmd/server
```

To persist state:

```sh
export JODATA_DATA_FILE=../.data/jodata.json
cd backend
go run ./cmd/server
```

Optional LLM planner:

```sh
export JODATA_AI_PROVIDER=llm
export JODATA_LLM_API_KEY=...
export JODATA_LLM_MODEL=gpt-4.1-mini
cd backend
go run ./cmd/server
```

Optional API-key roles:

```sh
export JODATA_API_KEYS='admin-secret:admin,editor-secret:editor,viewer-secret:viewer'
```

When configured, send `Authorization: Bearer <key>` or `X-API-Key: <key>`.

## Frontend

The full frontend implementation is in [`frontend/index.html`](frontend/index.html), containing:

- file/URL/API upload flows
- intermediary payload import
- parser profile management
- metric/visualization creation
- AI auto-chart + auto-dashboard + auto-build
- chart and dashboard views
- bundle import/export

Backend preference:
- uses `JODATA_FRONTEND_BUILD_DIR` (default `../frontend/dist`) for static assets and `index.html` when serving SPA routes
- uses `JODATA_FRONTEND_INDEX` when set, otherwise attempts `../frontend/dist/index.html`, then `../frontend/index.html` in relation to backend working dir
- falls back to embedded copy for robustness
