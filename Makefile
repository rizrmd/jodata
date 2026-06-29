.PHONY: build-frontend run-server run

build-frontend:
	@if [ -f frontend/package.json ] && command -v npm >/dev/null 2>&1; then \
		(cd frontend && npm run build); \
	elif [ -f frontend/package.json ]; then \
		echo "npm not found; falling back to static index.html copy."; \
		test -d frontend/dist || mkdir -p frontend/dist; \
		cp frontend/index.html frontend/dist/index.html; \
		echo "Wrote frontend/dist/index.html"; \
	else \
		mkdir -p frontend/dist; \
		cp frontend/index.html frontend/dist/index.html; \
		echo "Wrote frontend/dist/index.html"; \
	fi

run-server: build-frontend
	@cd backend && JODATA_FRONTEND_BUILD_DIR=../frontend/dist go run ./cmd/server

run: run-server
