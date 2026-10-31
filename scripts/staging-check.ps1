param(
    [string]$Project = 'nordwinds-staging',
    [string]$StagingHost = 'staging.localhost',
    [int]$Port = 18443,
    [switch]$SyntheticFlow
)

$ErrorActionPreference = 'Stop'
if ($Project -notmatch '^[a-z][a-z0-9_-]+$' -or $StagingHost -ne 'staging.localhost' -or $Port -lt 1024 -or $Port -gt 65535) {
    throw 'Nur das isolierte lokale Staging-Ziel ist für diesen Test erlaubt.'
}
$base = "https://${StagingHost}:$Port"
$curl = if ($IsWindows) { 'curl.exe' } else { 'curl' }
$curlOptions = @('--silent', '--show-error', '--max-time', '20')
if ($IsWindows) { $curlOptions += '--ssl-no-revoke' }
$temporary = Join-Path ([IO.Path]::GetTempPath()) ('nordwinds-staging-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $temporary | Out-Null
$certificate = Join-Path $temporary 'root.crt'
$headersFile = Join-Path $temporary 'headers.txt'
$bodyFile = Join-Path $temporary 'body.txt'
$cookieFile = Join-Path $temporary 'cookies.txt'
$cookieReady = $false

function Send-StagingRequest {
    param([string]$Path, [string]$Method = 'GET', [string]$Body, [string[]]$Headers = @())
    $arguments = @($curlOptions) + @('--cacert', $certificate, '--resolve', "${StagingHost}:${Port}:127.0.0.1", '--dump-header', $headersFile, '--output', $bodyFile, '--write-out', '%{http_code}', '--request', $Method)
    if ($cookieReady) { $arguments += @('--cookie', $cookieFile, '--cookie-jar', $cookieFile) }
    if ($null -ne $Body) { $arguments += @('--data-raw', $Body, '--header', 'Content-Type: application/json') }
    foreach ($header in $Headers) { $arguments += @('--header', $header) }
    $status = & $curl @arguments ($base + $Path)
    if ($LASTEXITCODE -ne 0 -or $status -notmatch '^\d{3}$') { throw "HTTPS-Anfrage an $Path fehlgeschlagen." }
    return [pscustomobject]@{
        Status = [int]$status
        Body = [IO.File]::ReadAllText($bodyFile)
        Headers = [IO.File]::ReadAllText($headersFile)
    }
}

try {
    $containers = @(docker compose --env-file .env.staging -f compose.yaml -f compose.staging.yaml -p $Project ps -q edge)
    if ($LASTEXITCODE -ne 0 -or $containers.Count -ne 1 -or -not $containers[0]) { throw 'Staging-Edge läuft nicht.' }
    $edge = $containers[0].Trim()
    $pem = @(docker exec $edge cat /tmp/caddy/data/caddy/pki/authorities/local/root.crt)
    if ($LASTEXITCODE -ne 0 -or ($pem -join "`n") -notmatch 'BEGIN CERTIFICATE') { throw 'Lokales TLS-Stammzertifikat fehlt.' }
    [IO.File]::WriteAllText($certificate, (($pem -join "`n") + "`n"), [Text.Encoding]::ASCII)

    $homeResponse = Send-StagingRequest -Path '/'
    if ($homeResponse.Status -ne 200 -or $homeResponse.Body -notmatch '<title>[^<]*NordWinds</title>' -or $homeResponse.Headers -notmatch "script-src 'self' 'nonce-") { throw 'HTTPS-Startseite oder CSP ungültig.' }
    $meta = Send-StagingRequest -Path '/api/v1/meta'
    if ($meta.Status -ne 200 -or ($meta.Body | ConvertFrom-Json).name -cne 'NordWinds' -or $meta.Headers -notmatch 'X-Request-Id: [0-9a-f]{32}') { throw 'API-Metadaten oder Request-ID fehlen.' }
    $products = Send-StagingRequest -Path '/api/v1/products?limit=1'
    $product = ($products.Body | ConvertFrom-Json).data | Select-Object -First 1
    if ($products.Status -ne 200 -or -not $product.slug) { throw 'Für den Staging-Test fehlt ein synthetisches Produkt.' }
    $detail = Send-StagingRequest -Path ("/produkte/" + $product.slug)
    if ($detail.Status -ne 200 -or $detail.Body -notmatch [regex]::Escape($product.name) -or $detail.Body -notmatch '<link rel="canonical"') { throw 'Produkt-SSR über HTTPS fehlgeschlagen.' }
    if ((Send-StagingRequest -Path '/internal/metrics').Status -ne 404) { throw 'Interne Metriken sind extern erreichbar.' }
    if ((Send-StagingRequest -Path '/api/v1/mock-payments/methods').Status -ne 404) { throw 'Mock-Zahlungen sind im Staging aktiv.' }

    if ($SyntheticFlow) {
        $email = 'nw25-' + [guid]::NewGuid().ToString('N') + '@example.invalid'
        $password = [Convert]::ToBase64String([Security.Cryptography.RandomNumberGenerator]::GetBytes(36))
        $origin = "Origin: $base"
        $credentials = @{ email = $email; password = $password } | ConvertTo-Json -Compress
        if ((Send-StagingRequest -Path '/api/v1/auth/register' -Method POST -Body $credentials -Headers @($origin)).Status -ne 201) { throw 'Synthetische Registrierung fehlgeschlagen.' }
        $arguments = @($curlOptions) + @('--cacert', $certificate, '--resolve', "${StagingHost}:${Port}:127.0.0.1", '--dump-header', $headersFile, '--output', $bodyFile, '--write-out', '%{http_code}', '--cookie-jar', $cookieFile, '--header', $origin, '--header', 'Content-Type: application/json', '--data-raw', $credentials, '--request', 'POST')
        $loginStatus = & $curl @arguments ($base + '/api/v1/auth/login')
        if ($LASTEXITCODE -ne 0 -or $loginStatus -ne '200') { throw 'Synthetische Anmeldung fehlgeschlagen.' }
        $csrf = ([IO.File]::ReadAllText($bodyFile) | ConvertFrom-Json).csrf_token
        if (-not $csrf -or -not (Test-Path -LiteralPath $cookieFile)) { throw 'Session oder CSRF-Token fehlen.' }
        $cookieReady = $true
        $variant = (Send-StagingRequest -Path ("/api/v1/products/" + $product.slug)).Body | ConvertFrom-Json
        $variantID = ($variant.variants | Select-Object -First 1).id
        if (-not $variantID) { throw 'Produktvariante fehlt.' }
        $cartBody = @{ variant_id = $variantID; quantity = 1 } | ConvertTo-Json -Compress
        $authHeaders = @($origin, "X-CSRF-Token: $csrf")
        $cart = Send-StagingRequest -Path '/api/v1/cart/items' -Method POST -Body $cartBody -Headers $authHeaders
        if ($cart.Status -ne 200 -or ($cart.Body | ConvertFrom-Json).items.Count -ne 1) { throw 'Synthetischer Warenkorb fehlgeschlagen.' }
        $blocked = Send-StagingRequest -Path '/api/v1/orders' -Method POST -Body '{}' -Headers $authHeaders
        if ($blocked.Status -ne 503 -or ($blocked.Body | ConvertFrom-Json).code -ne 'SERVICE_UNAVAILABLE') { throw 'Checkout-Sperre im Staging fehlt.' }
        $closed = Send-StagingRequest -Path '/api/v1/customer/me' -Method DELETE -Headers $authHeaders
        if ($closed.Status -ne 204) { throw 'Synthetisches Konto konnte nicht geschlossen werden.' }
        Write-Output 'Staging-E2E PASS: Registrierung, Produkt, Warenkorb, gesperrter Checkout und Kontoschließung.'
    }
    Write-Output 'Staging-Smoke PASS: TLS-Zertifikat, Startseite, CSP, API, Produkt-SSR und private Endpunkte.'
}
finally {
    foreach ($file in @($certificate, $headersFile, $bodyFile, $cookieFile)) {
        if (Test-Path -LiteralPath $file) { Remove-Item -LiteralPath $file }
    }
    Remove-Item -LiteralPath $temporary
}
