.PHONY: db api web gen build test sync runs

API := http://localhost:8090
ADMIN_TOKEN ?= dev
TEST_DB := postgres://crossover:crossover@localhost:5433/crossover_test
j ?= rosters

db: ## start Postgres on localhost:5433
	docker compose up -d db

api: ## run the Go server on :8090
	cd backend && ADMIN_TOKEN=$(ADMIN_TOKEN) go run .

web: ## run the frontend dev server, proxying /api to :8090
	cd frontend && npm run dev

gen: ## regenerate internal/db from migrations/ and queries/
	docker run --rm -v "$(PWD)/backend":/src -w /src sqlc/sqlc:1.30.0 generate

build: ## build the frontend into the Go binary: backend/crossover
	cd frontend && npm run build
	rm -rf backend/internal/web/dist && cp -R frontend/build backend/internal/web/dist
	touch backend/internal/web/dist/.gitkeep
	cd backend && CGO_ENABLED=0 go build -o crossover .

test: ## run all tests, using a separate crossover_test database
	docker compose exec -T db psql -U crossover -tc "select 1 from pg_database where datname = 'crossover_test'" | grep -q 1 \
		|| docker compose exec -T db createdb -U crossover crossover_test
	cd backend && TEST_DATABASE_URL=$(TEST_DB) go test ./...

sync: ## make sync c=nba            (rosters)   or   make sync c=nhl j=prospects
	curl -fsS -X POST -H "Authorization: Bearer $(ADMIN_TOKEN)" $(API)/api/admin/sync/$(c)/$(j)

runs: ## recent sync runs
	curl -fsS -H "Authorization: Bearer $(ADMIN_TOKEN)" $(API)/api/admin/ingest-runs
