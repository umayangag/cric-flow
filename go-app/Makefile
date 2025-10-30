APP_NAME:=cricinfo
API_BIN:=bin/api
SCRAPER_BIN:=bin/scraper
MIGRATE_BIN:=bin/migrate
GOFLAGS:=
LDFLAGS:=-s -w

export POSTGRES_HOST?=localhost
export POSTGRES_PORT?=5432
export POSTGRES_DB?=cricket_data
export POSTGRES_USER?=postgres
export POSTGRES_PASSWORD?=postgres
export POSTGRES_SSLMODE?=disable

.PHONY: all build clean run-api run-scraper docker-build docker-run fmt vet test migrate build-migrate

all: build

build:
	@mkdir -p bin
	GOFLAGS=$(GOFLAGS) CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(API_BIN) ./cmd/api
	GOFLAGS=$(GOFLAGS) CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(SCRAPER_BIN) ./cmd/scraper

build-migrate:
	@mkdir -p bin
	GOFLAGS=$(GOFLAGS) CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(MIGRATE_BIN) ./cmd/tools/migrate

migrate: build-migrate
	POSTGRES_HOST=$(POSTGRES_HOST) POSTGRES_PORT=$(POSTGRES_PORT) POSTGRES_DB=$(POSTGRES_DB) POSTGRES_USER=$(POSTGRES_USER) POSTGRES_PASSWORD=$(POSTGRES_PASSWORD) POSTGRES_SSLMODE=$(POSTGRES_SSLMODE) \
		$(MIGRATE_BIN) -dir=migrations

run-api:
	go run ./cmd/api

run-scraper:
	go run ./cmd/scraper --team="Sri Lanka" --from=2010-01-01 --to=2020-01-01

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test ./...

docker-build:
	docker build -t umayangag/cric-app-api:latest -f Dockerfile.api .
	docker build -t umayangag/cric-app-scraper:latest -f Dockerfile.scraper .

docker-run:
	docker run --rm -e POSTGRES_HOST=$${POSTGRES_HOST} -e POSTGRES_PORT=$${POSTGRES_PORT} -e POSTGRES_DB=$${POSTGRES_DB} -e POSTGRES_USER=$${POSTGRES_USER} -e POSTGRES_PASSWORD=$${POSTGRES_PASSWORD} -p 8080:8080 --name $(APP_NAME)-api umayangag/cric-app-api:latest