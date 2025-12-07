#!/bin/bash
set -euo pipefail

log() {
  printf "[%s] %s\n" "$(date '+%Y-%m-%dT%H:%M:%S%z')" "$*"
}

OUTLINE_URL=${OUTLINE_URL:-http://mdpdf:3000}

log "Starting Redis..."
redis-server --daemonize yes

log "Starting Postgres..."
service postgresql start

log "Waiting for Postgres to be ready..."
until su - postgres -c "pg_isready -h 127.0.0.1" >/dev/null 2>&1; do
  sleep 1
done
chown -R postgres:postgres /var/lib/postgresql

log "Configuring Postgres..."
su - postgres -c "psql -tc \"SELECT 1 FROM pg_roles WHERE rolname = 'user'\" | grep -q 1 || psql -c \"CREATE USER \\\"user\\\" WITH PASSWORD 'pass';\""
su - postgres -c "psql -tc \"SELECT 1 FROM pg_database WHERE datname = 'outline'\" | grep -q 1 || psql -c \"CREATE DATABASE outline OWNER \\\"user\\\" ENCODING 'UTF8';\""
su - postgres -c "psql -c \"GRANT ALL PRIVILEGES ON DATABASE outline TO \\\"user\\\";\""

log "Starting Outline..."

(
  log "Waiting for Outline to be ready..."
  while [ "$(curl -s -o /dev/null -w "%{http_code}" "$OUTLINE_URL")" != "200" ]; do
    sleep 5
  done
  log "Outline is ready. Running install script..."
  python3 /install_outline.py
) &

log "Starting PDF Server..."
gunicorn --bind 0.0.0.0:5000 --chdir / pdfserver:app --daemon

exec "$@"
