#!/bin/sh

set -eu

if [ -z "${L2BG_DATABASE_URL:-}" ]; then
  echo "L2BG_DATABASE_URL is required for db-init" >&2
  exit 1
fi

echo "db-init: waiting for PostgreSQL"
until pg_isready -d "$L2BG_DATABASE_URL" >/dev/null 2>&1; do
  sleep 1
done

echo "db-init: applying bootstrap.sql"
psql "$L2BG_DATABASE_URL" -v ON_ERROR_STOP=1 -f /workspace/bootstrap.sql

echo "db-init: validating required tables"
for table_name in schema_bootstrap accounts account_credentials characters character_items gameplay_sessions pvp_combat_events gameplay_event_outbox gameplay_event_receipts; do
  table_exists="$(psql "$L2BG_DATABASE_URL" -tAc "SELECT to_regclass('public.${table_name}') IS NOT NULL;")"
  if [ "$table_exists" != "t" ]; then
    echo "db-init: missing required table ${table_name}" >&2
    exit 1
  fi
done

echo "db-init: schema bootstrap completed successfully"
