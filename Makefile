SHELL := /usr/bin/env bash

GO ?= go
PKGS ?= ./...
APP ?= bgorch
CMD_DIR ?= ./cmd/$(APP)
BIN_DIR ?= ./bin
BIN ?= $(BIN_DIR)/$(APP)
VERIFY_SCRIPT ?= scripts/verify.sh
GOLANGCI_LINT ?= golangci-lint
COVER_FILE ?= coverage.out

.DEFAULT_GOAL := help

.PHONY: help all verify lint format fmt build install intall test vet race test-race cover clean

help:
	@printf '%s\n' \
		'Useful targets:' \
		'  make fmt         - format go files in place (gofmt -w)' \
		'  make format      - check formatting (gofmt -l via verify script)' \
		'  make lint        - run golangci-lint' \
		'  make vet         - run go vet' \
		'  make test        - run unit/integration tests' \
		'  make race        - run tests with race detector (supported platforms only)' \
		'  make build       - build binary at ./bin/$(APP)' \
		'  make install     - go install ./cmd/$(APP)' \
		'  make cover       - test coverage report in $(COVER_FILE)' \
		'  make verify      - run format check + build + test + vet + race' \
		'  make clean       - remove local build/test artifacts'

all: verify

verify:
	@GO_BIN="$(GO)" PKGS="$(PKGS)" "$(VERIFY_SCRIPT)"

lint:
	@command -v "$(GOLANGCI_LINT)" >/dev/null 2>&1 || { \
		echo "$(GOLANGCI_LINT) not found; install it first (https://golangci-lint.run/usage/install/)"; \
		exit 1; \
	}
	@"$(GOLANGCI_LINT)" run $(PKGS)

format:
	@GO_BIN="$(GO)" PKGS="$(PKGS)" "$(VERIFY_SCRIPT)" format

fmt:
	@find . -type f -name '*.go' \
		-not -path './vendor/*' \
		-not -path './.git/*' \
		-print0 | xargs -0 gofmt -w

build:
	@mkdir -p "$(BIN_DIR)"
	@"$(GO)" build -trimpath -o "$(BIN)" "$(CMD_DIR)"

install:
	@"$(GO)" install -trimpath "$(CMD_DIR)"

intall: install

test:
	@GO_BIN="$(GO)" PKGS="$(PKGS)" "$(VERIFY_SCRIPT)" test

vet:
	@GO_BIN="$(GO)" PKGS="$(PKGS)" "$(VERIFY_SCRIPT)" vet

race: test-race

test-race:
	@GO_BIN="$(GO)" PKGS="$(PKGS)" "$(VERIFY_SCRIPT)" race

cover:
	@"$(GO)" test -count=1 -shuffle=off -covermode=atomic -coverprofile="$(COVER_FILE)" $(PKGS)
	@"$(GO)" tool cover -func="$(COVER_FILE)"

clean:
	@rm -rf "$(BIN_DIR)" "$(COVER_FILE)"
