# scripts/restore.ps1 — Restore EMS database from backup file
param (
    [Parameter(Mandatory=$true)]
    [string]$BackupFile
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $BackupFile)) {
    Write-Error "Backup file not found: $BackupFile"
    exit 1
}

$Confirm = Read-Host "WARNING: This will overwrite the current PostgreSQL database. Continue? (y/N)"
if ($Confirm -notmatch '^[Yy]$') {
    Write-Host "Restore cancelled." -ForegroundColor Yellow
    exit 0
}

Write-Host "[+] Restoring EMS database from $BackupFile..." -ForegroundColor Cyan
Get-Content -Path $BackupFile -Raw | docker compose -f compose.prod.yaml exec -T postgres psql -U ems -d ems

Write-Host "[+] Database restore completed successfully." -ForegroundColor Green
