#!/usr/bin/env bash
# scripts/restore.sh — Restore EMS database from backup archive
set -euo pipefail

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <path_to_backup.sql.gz>"
    exit 1
fi

BACKUP_FILE="$1"

if [ ! -f "${BACKUP_FILE}" ]; then
    echo "Error: Backup file not found: ${BACKUP_FILE}"
    exit 1
fi

read -p "WARNING: This will overwrite the current PostgreSQL database. Continue? [y/N]: " confirm
if [[ ! "${confirm}" =~ ^[Yy]$ ]]; then
    echo "Restore cancelled."
    exit 0
fi

echo "[+] Restoring EMS database from ${BACKUP_FILE}..."
gunzip -c "${BACKUP_FILE}" | docker compose -f compose.prod.yaml exec -T postgres psql -U ems -d ems

echo "[+] Database restore completed successfully."
