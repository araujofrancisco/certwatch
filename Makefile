BIN       := certwatch
MODULE    := github.com/certwatch/certwatch
GO        := go
GOFLAGS   := -ldflags="-s -w"
BUILDDIR  := build

.PHONY: all build run test lint clean docker-build docker-run docker-stop docker-logs backup restore

all: lint test build

build:
	@mkdir -p $(BUILDDIR)
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(BUILDDIR)/$(BIN) ./cmd/$(BIN)/

run:
	$(GO) run ./cmd/$(BIN)/

test:
	$(GO) test ./... -v -count=1

LINT_VERSION := v1.59.1

lint:
	@which golangci-lint >/dev/null 2>&1 || (echo "installing golangci-lint"; go install github.com/golangci-lint/golangci-lint/cmd/golangci-lint@$(LINT_VERSION))
	golangci-lint run ./...

tidy:
	$(GO) mod tidy

clean:
	rm -rf $(BUILDDIR)

docker-build:
	docker compose build

docker-run:
	docker compose up -d

docker-stop:
	docker compose down

docker-logs:
	docker compose logs -f

backup:
	@scripts/backup.sh

restore:
	@scripts/restore.sh $(filter-out $@,$(MAKECMDGOALS))

%:
	@:
