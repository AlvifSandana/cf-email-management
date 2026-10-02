#!/usr/bin/env bash
# scripts/backup.sh — PostgreSQL Database Backup for EMS
set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-./backups}"
TIMESTAMP="$(date +'%Y%m%d_%H%M%S')"
BACKUP_FILE="${BACKUP_DIR}/ems_backup_${TIMESTAMP}.sql.gz"
RETENTION_DAYS="${RETENTION_DAYS:-7}"

mkdir -p "${BACKUP_DIR}"

echo "[+] Starting EMS PostgreSQL database backup at $(date)..."

# Dump database through docker compose container
docker compose -f compose.prod.yaml exec -T postgres pg_dump -U ems -d ems --clean --if-exists | gzip > "${BACKUP_FILE}"

echo "[+] Backup successfully created: ${BACKUP_FILE} ($(du -h "${BACKUP_FILE}" | cut -f1))"

# Prune backups older than RETENTION_DAYS
echo "[+] Purging backups older than ${RETENTION_DAYS} days..."
find "${BACKUP_DIR}" -name "ems_backup_*.sql.gz" -type f -mtime +"${RETENTION_DAYS}" -delete

echo "[+] Backup routine completed successfully."
