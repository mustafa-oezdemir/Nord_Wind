param(
    [string]$BaseUrl = 'http://127.0.0.1:8080'
)

$ErrorActionPreference = 'Stop'
$base = $BaseUrl.TrimEnd('/')

foreach ($service in @('db', 'api', 'ssr', 'edge')) {
    $containerId = docker compose ps -q $service
    if ($LASTEXITCODE -ne 0 -or -not $containerId) {
        throw "Container $service ist nicht verfügbar."
    }
    $health = ''
    for ($attempt = 0; $attempt -lt 15; $attempt++) {
        $health = docker inspect --format '{{.State.Health.Status}}' $containerId
        if ($LASTEXITCODE -ne 0 -or $health -eq 'unhealthy') { break }
        if ($health -eq 'healthy') { break }
        Start-Sleep -Seconds 2
    }
    if ($LASTEXITCODE -ne 0 -or $health -ne 'healthy') {
        throw "Container $service ist nicht gesund: $health"
    }
}

$homepageResponse = Invoke-WebRequest -Uri "$base/"
if ($homepageResponse.StatusCode -ne 200 -or $homepageResponse.Content -notmatch '<title>[^<]*NordWinds</title>') {
    throw 'Die NordWinds-Startseite antwortet nicht wie erwartet.'
}

$meta = Invoke-RestMethod -Uri "$base/api/v1/meta"
if ($meta.name -cne 'NordWinds' -or $meta.locale -cne 'de-DE' -or $meta.currency -cne 'EUR') {
    throw 'Die API-Metadaten stimmen nicht mit der Projektkonfiguration überein.'
}

$products = Invoke-RestMethod -Uri "$base/api/v1/products?limit=1"
if ($products.data.Count -gt 0) {
    $first = $products.data[0]
    $detail = Invoke-WebRequest -Uri "$base/produkte/$($first.slug)"
    if ($detail.StatusCode -ne 200 -or -not $detail.Content.Contains($first.name) -or
        -not $detail.Content.Contains('rel="canonical"')) {
        throw 'Die Produktseite ist im ersten HTML nicht lesbar oder hat keine kanonische URL.'
    }
}

docker compose exec -T api /usr/local/bin/migrate status | Out-Null
if ($LASTEXITCODE -ne 0) {
    throw 'Die Datenbankmigration wurde nicht bestätigt.'
}

Write-Output 'Smoke-Test bestanden: Startseite, Produkt-SSR, Gin-API, Container-Health und Migration.'
