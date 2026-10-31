param(
    [Parameter(Mandatory = $true)][string]$Project,
    [Parameter(Mandatory = $true)][string]$BackupFile,
    [Parameter(Mandatory = $true)][string]$RestoreDatabase
)

$ErrorActionPreference = 'Stop'
if ($Project -notmatch '^[a-z][a-z0-9_-]+$') {
    throw 'Ungültiger Compose-Projektname.'
}
if ($RestoreDatabase -notmatch '^nordwinds_restore_[a-z0-9_]+$') {
    throw 'Die Zieldatenbank muss mit nordwinds_restore_ beginnen.'
}
$dump = (Resolve-Path -LiteralPath $BackupFile).Path
if ([IO.Path]::GetExtension($dump) -ne '.dump') {
    throw 'Nur ein PostgreSQL-Custom-Archiv mit der Endung .dump ist erlaubt.'
}
$checksumFile = [IO.Path]::ChangeExtension($dump, '.sha256')
if (-not (Test-Path -LiteralPath $checksumFile)) {
    throw 'Die SHA-256-Datei fehlt.'
}
$expected = (Get-Content -LiteralPath $checksumFile -Raw).Split(' ', [StringSplitOptions]::RemoveEmptyEntries)[0].Trim().ToLowerInvariant()
$actual = (Get-FileHash -LiteralPath $dump -Algorithm SHA256).Hash.ToLowerInvariant()
if ($expected -notmatch '^[0-9a-f]{64}$' -or $actual -cne $expected) {
    throw 'Die SHA-256-Prüfsumme stimmt nicht.'
}

$containers = @(docker compose -p $Project ps -q db)
if ($LASTEXITCODE -ne 0 -or $containers.Count -ne 1 -or -not $containers[0]) {
    throw 'Die Datenbank des angegebenen Compose-Projekts läuft nicht.'
}
$container = $containers[0].Trim()
$remote = '/tmp/nordwinds-restore-' + [guid]::NewGuid().ToString('N') + '.dump'
try {
    docker cp -- $dump "${container}:$remote" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Archiv konnte nicht in den Datenbankcontainer kopiert werden.' }
    docker exec $container pg_restore --list $remote | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Das Archiv ist nicht lesbar.' }
    docker exec $container sh -c 'createdb -U "$POSTGRES_USER" "$1"' sh $RestoreDatabase
    if ($LASTEXITCODE -ne 0) { throw 'Die isolierte Zieldatenbank konnte nicht angelegt werden. Sie darf nicht bereits existieren.' }
    docker exec $container sh -c 'pg_restore -U "$POSTGRES_USER" -d "$1" --exit-on-error --no-owner --no-acl "$2"' sh $RestoreDatabase $remote
    if ($LASTEXITCODE -ne 0) { throw 'Die Rücksicherung ist fehlgeschlagen; die isolierte Zieldatenbank zur Untersuchung behalten.' }
    $result = docker exec $container sh -c 'psql -U "$POSTGRES_USER" -d "$1" -Atc "SELECT (SELECT count(*) FROM schema_migrations), (SELECT count(*) FROM customers), (SELECT count(*) FROM orders), (SELECT count(*) FROM products)"' sh $RestoreDatabase
    if ($LASTEXITCODE -ne 0 -or $result -notmatch '^\d+\|\d+\|\d+\|\d+$') {
        throw 'Die Datenprüfung nach der Rücksicherung ist fehlgeschlagen.'
    }
    Write-Output "Restore PASS: Projekt=$Project Datenbank=$RestoreDatabase SHA256=$actual Versionen|Kunden|Bestellungen|Produkte=$result"
}
finally {
    docker exec $container rm -f -- $remote | Out-Null
}
