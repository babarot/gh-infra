BINARY_NAME := gh-infra
VERSION := $(shell cat VERSION)
LDFLAGS := "-X main.version=$(VERSION) -X main.revision=$(shell git rev-parse --verify --short HEAD)"

all: build

TEST_EXCLUDE := %/cmd %/cmd/gh-infra
TEST_PKGS := $(filter-out $(TEST_EXCLUDE),$(shell go list ./...))

test:
	go test $(TEST_PKGS) -coverprofile=coverage.out -covermode=count

lint:
	golangci-lint run ./...

build:
	go build -ldflags $(LDFLAGS) -trimpath -o $(BINARY_NAME) ./cmd/gh-infra/

install:
	go install -ldflags $(LDFLAGS) ./cmd/gh-infra/

clean:
	rm -f $(BINARY_NAME)

docs:
	mise exec -- npm run dev --prefix docs

docs-build:
	mise exec -- npm run build --prefix docs

docs-install:
	mise exec -- npm install --prefix docs

demos:
	@if ! docker info > /dev/null 2>&1; then \
		echo "Docker is not running — please start Docker and try again."; \
		exit 1; \
	fi
	@echo "Building gh-infra for Linux..."
	@GOOS=linux GOARCH=amd64 go build -ldflags $(LDFLAGS) -trimpath -o docs/tapes/.gh-infra ./cmd/gh-infra/
	@for tape in docs/tapes/*.tape; do \
		echo "Recording $$(basename $$tape)..."; \
		docker run --rm \
			-v $(CURDIR)/docs/tapes:/data \
			-w /data \
			$(foreach v,$(DEMO_ENV),-e $(v)) \
			ghcr.io/charmbracelet/vhs $$(basename $$tape); \
	done
	@rm -f docs/tapes/.gh-infra
	@echo "Copying assets to docs/public/..."
	@cp docs/tapes/demo.gif docs/tapes/demo-light.gif docs/public/

.PHONY: all test lint build install clean docs docs-build docs-install demos
