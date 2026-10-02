# scripts/backup.ps1 — PostgreSQL Database Backup for EMS on Windows
param (
    [string]$BackupDir = ".\backups",
    [int]$RetentionDays = 7
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $BackupDir)) {
    New-Item -ItemType Directory -Path $BackupDir -Force | Out-Null
}

$Timestamp = (Get-Date).ToString("yyyyMMdd_HHmmss")
$BackupFile = Join-Path $BackupDir "ems_backup_$Timestamp.sql"

Write-Host "[+] Starting EMS PostgreSQL database backup at $(Get-Date)..." -ForegroundColor Cyan

docker compose -f compose.prod.yaml exec -T postgres pg_dump -U ems -d ems --clean --if-exists | Out-File -FilePath $BackupFile -Encoding utf8

Write-Host "[+] Backup successfully created: $BackupFile" -ForegroundColor Green

# Purge old backups
$LimitDate = (Get-Date).AddDays(-$RetentionDays)
Get-ChildItem -Path $BackupDir -Filter "ems_backup_*.sql" | Where-Object { $_.LastWriteTime -lt $LimitDate } | Remove-Item -Force
Write-Host "[+] Old backups pruned." -ForegroundColor Cyan
