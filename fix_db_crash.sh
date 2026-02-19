#!/bin/bash

# Fix for "Dirty database version 15" error loop on VPS
# Confirmed container names from user:
DB_CONTAINER="remnawave-telegram-shop-db"
BOT_CONTAINER="remnawave-shop-bot-1"
DB_USER="postgres"
DB_NAME="remnawave"

echo "🔄 Fixing dirty database state on VPS..."

# 1. Reset dirty flag and force version back to 14
docker exec "$DB_CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" -c "UPDATE schema_migrations SET dirty = false, version = 14;"

if [ $? -eq 0 ]; then
  echo "✅ Database state reset to version 14."
  echo "🚀 Rebuilding and restarting bot to apply fixed migration..."
  # We must rebuild to ensure the fixed 000015 migration file is copied into the container
  docker-compose up -d --build bot
else
  echo "❌ Failed to reset database. Check if container '$DB_CONTAINER' is running."
fi
