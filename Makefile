.PHONY: build-frontend run-server run

build-frontend:
	@if [ -f frontend/package.json ]; then \
		(cd frontend && npm run build); \
	else \
		mkdir -p frontend/dist; \
		cp frontend/index.html frontend/dist/index.html; \
		echo "Wrote frontend/dist/index.html"; \
	fi

run-server: build-frontend
	@cd backend && JODATA_FRONTEND_BUILD_DIR=../frontend/dist go run ./cmd/server

run: run-server
