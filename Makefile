SHELL := /usr/bin/env bash

GO ?= go
PKGS ?= ./...
VERIFY_SCRIPT := scripts/verify.sh

.DEFAULT_GOAL := verify

.PHONY: verify format build test vet race

verify:
	@GO_BIN="$(GO)" PKGS="$(PKGS)" "$(VERIFY_SCRIPT)"

format:
	@GO_BIN="$(GO)" PKGS="$(PKGS)" "$(VERIFY_SCRIPT)" format

build:
	@GO_BIN="$(GO)" PKGS="$(PKGS)" "$(VERIFY_SCRIPT)" build

test:
	@GO_BIN="$(GO)" PKGS="$(PKGS)" "$(VERIFY_SCRIPT)" test

vet:
	@GO_BIN="$(GO)" PKGS="$(PKGS)" "$(VERIFY_SCRIPT)" vet

race:
	@GO_BIN="$(GO)" PKGS="$(PKGS)" "$(VERIFY_SCRIPT)" race
