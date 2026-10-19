BINARY_NAME := gh-infra
VERSION := $(shell cat VERSION)
LDFLAGS := "-X main.version=$(VERSION) -X main.revision=$(shell git rev-parse --verify --short HEAD)"

all: build

# cmd is left out of the coverage profile (and of octocov's code ratio), but
# its tests still run.
TEST_EXCLUDE := %/cmd %/cmd/gh-infra
TEST_PKGS := $(filter-out $(TEST_EXCLUDE),$(shell go list ./...))
CMD_PKGS := $(filter $(TEST_EXCLUDE),$(shell go list ./...))

test:
	go test -race $(TEST_PKGS) -coverprofile=coverage.out -covermode=atomic
	go test -race $(CMD_PKGS)

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

# Record the demo GIFs with VHS in Docker and copy them to docs/public:
#   make demo [TAPES="demo demo-apply"]
# TAPES names the tapes to record (docs/tapes/<name>.tape); all when not given.
# Only demo*.gif go to docs/public; the others (showcase) stay in docs/tapes.
demo:
	@if ! docker info > /dev/null 2>&1; then \
		echo "Docker is not running — please start Docker and try again."; \
		exit 1; \
	fi
	@echo "Building gh-infra for Linux..."
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags $(LDFLAGS) -trimpath -o docs/tapes/.gh-infra ./cmd/gh-infra/
	@docs/tapes/vhs.sh $(foreach v,$(DEMO_ENV),-e $(v)) $(TAPES)
	@rm -f docs/tapes/.gh-infra
	@echo "Copying assets to docs/public/..."
	@for t in $(or $(TAPES),$(basename $(notdir $(wildcard docs/tapes/*.tape)))); do \
		case $$t in demo*) cp docs/tapes/$$t.gif docs/public/ ;; esac; \
	done

# Try the terminal output against mock-gh, without GitHub:
#   make playground [SCENARIO=many] [DELAY=<seconds per gh call>]
# MOCK_DELAY works too, on the command line or in the environment; DELAY
# replaces it only when given.
playground:
	@$(if $(DELAY),MOCK_DELAY=$(DELAY) )docs/tapes/playground.sh $(SCENARIO)

.PHONY: all test lint build install clean docs docs-build docs-install demo playground
