#!/bin/bash

# Fix for "Dirty database version 15" error loop
# This happens because migration 15 failed mid-way previously.
# Since the bot container keeps crashing, we must fix it via the running Database container.

echo "🔄 Attempting to fix dirty database state..."

# 1. Update the schema_migrations table directly in Postgres
# We assume the user is 'postgres' and db is 'remnawave' (standard defaults).
# If these differ, edit the variables below.
DB_USER="postgres"
DB_NAME="remnawave"

# Try to find the DB container name from docker-compose ps or assume standard name
CONTAINER_NAME=$(docker ps --format "{{.Names}}" | grep -E "db|postgres" | head -n 1)

if [ -z "$CONTAINER_NAME" ]; then
  echo "❌ Could not find a running database container."
  echo "Please run: docker-compose exec <db-service> psql -U $DB_USER -d $DB_NAME -c 'UPDATE schema_migrations SET dirty = false, version = 14;'"
  exit 1
fi

echo "✅ Found database container: $CONTAINER_NAME"

# Reset dirty flag and force version back to 14 (so 15 runs fresh)
docker exec "$CONTAINER_NAME" psql -U "$DB_USER" -d "$DB_NAME" -c "UPDATE schema_migrations SET dirty = false, version = 14;"

if [ $? -eq 0 ]; then
  echo "✅ Database state reset to version 14 (clean)."
  echo "🚀 Restarting bot container to apply fixed migration..."
  docker restart $(docker ps --format "{{.Names}}" | grep -E "bot|app" | head -n 1)
else
  echo "❌ Failed to reset database. Check credentials or container name."
fi
