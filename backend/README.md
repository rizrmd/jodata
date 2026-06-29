# backend

Go backend for jodata.

Run from this directory after building frontend assets (`../frontend/dist`):

```sh
cd backend
JODATA_FRONTEND_BUILD_DIR=../frontend/dist go run ./cmd/server
```

It serves the SPA from `JODATA_FRONTEND_BUILD_DIR` (default `../frontend/dist`), falling back to:
- `JODATA_FRONTEND_INDEX` when set
- `../frontend/dist/index.html`
- embedded `backend/index.html` bundle

Security/runtime knobs:

- `JODATA_CORS_ORIGINS` (comma-separated allow list, or `*`).
- `JODATA_MAX_REQUEST_BYTES` (max request body in bytes; default `33554432`).
