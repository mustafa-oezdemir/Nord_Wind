param(
    [string]$Project = 'nordwinds-staging',
    [string]$EnvFile = '.env.staging',
    [string]$BackupDirectory = 'backups-staging'
)

$ErrorActionPreference = 'Stop'
if ($Project -ne 'nordwinds-staging') { throw 'Nur das isolierte Staging-Projekt ist für diesen Dry-Run erlaubt.' }
if (-not (Test-Path -LiteralPath $EnvFile -PathType Leaf)) { throw 'Staging-Umgebung fehlt.' }
$configured = Get-Content -LiteralPath $EnvFile -Raw
if ($configured -notmatch '(?m)^APP_ENV=staging\s*$' -or $configured -notmatch '(?m)^NORDWINDS_DEMO_CHECKOUT_ENABLED=false\s*$') {
    throw 'Staging muss den echten Checkout gesperrt halten.'
}
$compose = @('compose', '--env-file', $EnvFile, '-f', 'compose.yaml', '-f', 'compose.staging.yaml', '-p', $Project)
docker @compose config --quiet
if ($LASTEXITCODE -ne 0) { throw 'Compose-Konfiguration ungültig.' }

docker @compose run --rm --no-deps -e NORDWINDS_BACKUP_ONCE=true backup
if ($LASTEXITCODE -ne 0) { throw 'Sicherung vor dem Migrations-Dry-Run fehlgeschlagen.' }
$archive = Get-ChildItem -LiteralPath $BackupDirectory -Filter 'nordwinds-*.dump' -File |
    Sort-Object LastWriteTimeUtc -Descending | Select-Object -First 1
if (-not $archive) { throw 'Kein Sicherungsarchiv vorhanden.' }
$restoreDatabase = 'nordwinds_restore_nw25_' + [guid]::NewGuid().ToString('N').Substring(0, 12)
pwsh -NoProfile -File scripts/restore-test.ps1 -Project $Project -BackupFile $archive.FullName -RestoreDatabase $restoreDatabase
if ($LASTEXITCODE -ne 0) { throw 'Rücksicherung vor dem Migrations-Dry-Run fehlgeschlagen.' }

docker @compose run --rm --no-deps -e "POSTGRES_DB=$restoreDatabase" migrate up
if ($LASTEXITCODE -ne 0) { throw "Migration auf der Kopie fehlgeschlagen; $restoreDatabase bleibt zur Untersuchung bestehen." }
docker @compose run --rm --no-deps -e "POSTGRES_DB=$restoreDatabase" migrate status
if ($LASTEXITCODE -ne 0) { throw "Migrationsstatus der Kopie fehlgeschlagen; $restoreDatabase bleibt zur Untersuchung bestehen." }

$containers = @(docker ps --filter "label=com.docker.compose.project=$Project" --filter 'label=com.docker.compose.service=db' --format '{{.ID}}')
if ($LASTEXITCODE -ne 0 -or $containers.Count -ne 1 -or -not $containers[0]) { throw 'Staging-Datenbankcontainer nicht eindeutig.' }
$container = $containers[0].Trim()
docker exec $container sh -c 'dropdb -U "$POSTGRES_USER" --if-exists "$1"' sh $restoreDatabase
if ($LASTEXITCODE -ne 0) { throw "Die Kopie $restoreDatabase konnte nicht entfernt werden." }
Write-Output "Staging-Preflight PASS: jüngste Sicherung zurückgespielt, Migration auf Kopie geprüft, Kopie entfernt. Archiv=$($archive.Name)"
