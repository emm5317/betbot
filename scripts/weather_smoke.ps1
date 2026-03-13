#!/usr/bin/env pwsh

param(
    [string]$DatabaseUrl = "postgres://betbot:betbot-dev-password@127.0.0.1:5432/betbot?sslmode=disable",
    [switch]$EnsurePostgres
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Push-Location $repoRoot

try {
    if ($EnsurePostgres) {
        $existing = docker ps -a --filter "name=^/betbot-postgres$" --format "{{.Names}}"
        if ($LASTEXITCODE -ne 0) {
            exit $LASTEXITCODE
        }

        if (($existing | Select-Object -First 1) -eq "betbot-postgres") {
            docker start betbot-postgres | Out-Null
            if ($LASTEXITCODE -ne 0) {
                exit $LASTEXITCODE
            }
        }
        else {
            docker compose -f deploy/docker/docker-compose.yml up -d postgres
            if ($LASTEXITCODE -ne 0) {
                exit $LASTEXITCODE
            }
        }
    }

    $env:BETBOT_TEST_DATABASE_URL = $DatabaseUrl
    $env:GOCACHE = Join-Path $repoRoot ".gocache"
    $env:GOTELEMETRY = "off"

    Write-Host "Running weather smoke harness against $DatabaseUrl"

    go test ./internal/integration -run '^TestWeatherSyncRiverSmokeHarness$' -count=1 -v
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }

    Write-Host "weather smoke harness: PASS"
}
finally {
    Pop-Location
}
