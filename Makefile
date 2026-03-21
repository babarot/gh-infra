BINARY_NAME := gh-infra
VERSION := $(shell cat VERSION)
LDFLAGS := "-X main.version=$(VERSION) -X main.revision=$(shell git rev-parse --verify --short HEAD)"

all: build

TEST_PKGS := ./internal/repository/ ./internal/fileset/ ./internal/gh/ ./internal/manifest/

test:
	go test $(TEST_PKGS) -coverprofile=coverage.out -covermode=count

build:
	go build -ldflags $(LDFLAGS) -trimpath -o $(BINARY_NAME) ./cmd/gh-infra/

install:
	go install -ldflags $(LDFLAGS) ./cmd/gh-infra/

clean:
	rm -f $(BINARY_NAME)

.PHONY: all test build install clean
