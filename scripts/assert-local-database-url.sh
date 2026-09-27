#!/usr/bin/env bash
set -euo pipefail

database_url="${DATABASE_URL:-}"

if [[ -z "$database_url" ]]; then
  echo "error: DATABASE_URL is not set. Copy .env.example to .env"
  exit 1
fi

case "$database_url" in
  *supabase.co*|*supabase.com*|*pooler.supabase.com*)
    echo "error: DATABASE_URL points at hosted Supabase. Use local Docker Postgres only."
    exit 1
    ;;
esac

case "$database_url" in
  postgres://*@localhost:*|postgres://*@127.0.0.1:*)
    ;;
  *)
    echo "error: DATABASE_URL must target localhost for local dev (got non-local host)."
    exit 1
    ;;
esac

case "$database_url" in
  *:54323/*|*:54323)
    echo "error: DATABASE_URL points at monaco-postgres-test (port 54323), the durability-off test database."
    echo "Dev data belongs in monaco-postgres on ${POSTGRES_PORT:-54322}. Tests read TEST_DATABASE_URL."
    exit 1
    ;;
esac
