#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export LC_ALL=C
export TZ=UTC
export GO111MODULE=on

GO_BIN="${GO_BIN:-go}"
PKGS="${PKGS:-./...}"

log() {
  printf '[verify] %s\n' "$*"
}

check_format() {
  log "format check (gofmt -l)"

  files=()
  while IFS= read -r file; do
    files+=("$file")
  done < <(find . -type f -name '*.go' \
    -not -path './vendor/*' \
    -not -path './.git/*' | LC_ALL=C sort)

  if ((${#files[@]} == 0)); then
    log "no go files found"
    return 0
  fi

  local unformatted
  unformatted="$(gofmt -l "${files[@]}")"
  if [[ -n "$unformatted" ]]; then
    log "unformatted files detected"
    printf '%s\n' "$unformatted"
    return 1
  fi

  log "format check passed"
}

run_build() {
  log "build"
  "$GO_BIN" build -trimpath $PKGS
}

run_test() {
  log "test"
  "$GO_BIN" test -count=1 -shuffle=off $PKGS
}

run_vet() {
  log "vet"
  "$GO_BIN" vet $PKGS
}

race_supported() {
  local goos goarch cgo
  goos="$("$GO_BIN" env GOOS)"
  goarch="$("$GO_BIN" env GOARCH)"
  cgo="$("$GO_BIN" env CGO_ENABLED)"

  if [[ "$cgo" != "1" ]]; then
    return 1
  fi

  case "${goos}/${goarch}" in
    darwin/amd64|darwin/arm64|linux/amd64|linux/arm64|windows/amd64)
      return 0
      ;;
  esac

  return 1
}

run_race() {
  if ! race_supported; then
    log "race test skipped (unsupported platform or CGO disabled)"
    return 0
  fi

  log "race test"
  "$GO_BIN" test -race -count=1 -shuffle=off $PKGS
}

run_stage() {
  case "$1" in
    format)
      check_format
      ;;
    build)
      run_build
      ;;
    test)
      run_test
      ;;
    vet)
      run_vet
      ;;
    race)
      run_race
      ;;
    *)
      printf 'unknown stage: %s\n' "$1" >&2
      return 2
      ;;
  esac
}

main() {
  if (($# == 0)); then
    run_stage format
    run_stage build
    run_stage test
    run_stage vet
    run_stage race
    log "verify completed"
    return 0
  fi

  local stage
  for stage in "$@"; do
    run_stage "$stage"
  done
}

main "$@"
