param([string]$OutputFile = '.env.staging')

$ErrorActionPreference = 'Stop'
if (Test-Path -LiteralPath $OutputFile) { throw "$OutputFile existiert bereits und wird nicht überschrieben." }
$example = Get-Content -LiteralPath .env.staging.example -Raw
$ownerPassword = [Convert]::ToBase64String([Security.Cryptography.RandomNumberGenerator]::GetBytes(48))
$runtimePassword = [Convert]::ToBase64String([Security.Cryptography.RandomNumberGenerator]::GetBytes(48))
$metricsToken = [Convert]::ToBase64String([Security.Cryptography.RandomNumberGenerator]::GetBytes(48))
$configured = $example.Replace('hier-ein-eigenes-starkes-passwort-setzen', $ownerPassword).
    Replace('hier-ein-anderes-passwort-mit-mindestens-32-zeichen-setzen', $runtimePassword).
    Replace('hier-einen-metrik-token-mit-mindestens-32-zeichen-setzen', $metricsToken)
Set-Content -LiteralPath $OutputFile -Value $configured -NoNewline -Encoding utf8
if (-not (git check-ignore -- $OutputFile)) { throw "$OutputFile ist nicht von Git ausgeschlossen." }
Write-Output "Lokale Staging-Konfiguration erstellt: $OutputFile"
