#!/usr/bin/env bash
# lint-conventions.sh — enforce project conventions that golangci-lint
# cannot express (DDL canonical forms, sqlc named params, migration
# hygiene). Every rule here has a documented reason in CLAUDE.md.
#
# Run locally: make lint-conventions
# Run in CI:   part of `make lint`

set -euo pipefail

fail=0

header() { printf '\n━━ %s ━━\n' "$1"; }

check() {
  local desc="$1" pattern="$2" paths="$3" exclude="${4:-}"
  local cmd=(grep -rEn --color=always "$pattern" $paths)
  if [ -n "$exclude" ]; then
    cmd+=(--exclude-dir="$exclude")
  fi
  local hits
  hits="$("${cmd[@]}" 2>/dev/null || true)"
  if [ -n "$hits" ]; then
    printf '\033[31m✗\033[0m %s\n%s\n\n' "$desc" "$hits"
    fail=1
  else
    printf '\033[32m✓\033[0m %s\n' "$desc"
  fi
}

header "Postgres DDL canonical forms (CLAUDE.md Mandatory Rules)"
# TIMESTAMPTZ / TIMESTAMP shorthand → sqlc override fails silently.
check "no TIMESTAMPTZ (use TIMESTAMP WITH TIME ZONE)" \
  '\bTIMESTAMPTZ\b' \
  'migrations/postgres/'
# DECIMAL without pg_catalog match → sqlc emits pgtype.Numeric.
check "no bare DECIMAL (use NUMERIC)" \
  '\bDECIMAL\s*\(' \
  'migrations/postgres/'
# INT shorthand → inconsistent with INTEGER canonical form.
check "no bare INT column type (use INTEGER)" \
  '^\s*[a-z_]+\s+INT(\s|,|$)' \
  'migrations/postgres/'
# BOOL shorthand.
check "no bare BOOL column type (use BOOLEAN)" \
  '^\s*[a-z_]+\s+BOOL(\s|,|$)' \
  'migrations/postgres/'

header "Go conventions (complementing forbidigo)"
# Migrations must go through CLI — hand-written files get silently ignored.
# This catches files named without the framework's timestamp prefix.
if [ -d migrations/postgres ]; then
  bad_files="$(find migrations/postgres -maxdepth 1 -type f -name '*.sql' -regextype posix-extended ! -regex '.*/[0-9]{14}_[a-z0-9_]+\.(up|down)\.sql$' 2>/dev/null || true)"
  if [ -n "$bad_files" ]; then
    printf '\033[31m✗\033[0m migration files without CLI-generated timestamp prefix\n%s\n\n' "$bad_files"
    fail=1
  else
    printf '\033[32m✓\033[0m migration filename convention\n'
  fi
fi

if [ "$fail" -ne 0 ]; then
  printf '\n\033[31mconvention lint failed\033[0m — fix violations above.\n'
  exit 1
fi
printf '\n\033[32mall conventions satisfied\033[0m\n'
