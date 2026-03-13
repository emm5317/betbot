#!/usr/bin/env pwsh

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Invoke-CheckedCommand {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Command
    )

    Write-Host ">> $Command"
    Invoke-Expression $Command
    if ($LASTEXITCODE -ne 0) {
        throw "Command failed (exit $LASTEXITCODE): $Command"
    }
}

function Wait-ComposeProjectReady {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ProjectName,
        [string[]]$AllowExitedServices = @(),
        [int]$TimeoutSeconds = 180
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)

    while ((Get-Date) -lt $deadline) {
        $ids = docker ps -a --filter "label=com.docker.compose.project=$ProjectName" --format "{{.ID}}"
        if ($LASTEXITCODE -ne 0) {
            throw "Failed to list containers for compose project '$ProjectName'."
        }
        if (-not $ids) {
            Start-Sleep -Seconds 2
            continue
        }

        $allReady = $true
        foreach ($id in $ids) {
            $inspectJson = docker inspect $id
            if ($LASTEXITCODE -ne 0) {
                throw "Failed to inspect container id '$id'."
            }
            $container = ($inspectJson | ConvertFrom-Json)[0]
            $name = $container.Name.Trim("/")
            $service = $container.Config.Labels.'com.docker.compose.service'
            $status = $container.State.Status
            $exitCode = $container.State.ExitCode
            $health = $null
            if ($container.State.PSObject.Properties.Name -contains "Health" -and $container.State.Health) {
                $health = $container.State.Health.Status
            }

            if ($status -eq "running") {
                if ($health -and $health -ne "healthy") {
                    $allReady = $false
                    break
                }
                continue
            }

            if ($status -eq "exited" -and ($AllowExitedServices -contains $service) -and $exitCode -eq 0) {
                continue
            }

            Write-Host "Container not ready: $name (service=$service status=$status health=$health exit_code=$exitCode)"
            $allReady = $false
            break
        }

        if ($allReady) {
            Write-Host "Project '$ProjectName' is ready."
            return
        }

        Start-Sleep -Seconds 2
    }

    throw "Timed out waiting for compose project '$ProjectName' to become ready."
}

function Initialize-BetbotDatabaseIfNeeded {
    param(
        [Parameter(Mandatory = $true)]
        [string]$BetbotRoot
    )

    $pollRunsExists = docker exec -i betbot-postgres psql -U betbot -d betbot -tAc "SELECT to_regclass('public.poll_runs') IS NOT NULL;"
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to check Betbot schema state."
    }
    $pollRunsExists = $pollRunsExists.Trim().ToLowerInvariant()

    if ($pollRunsExists -ne "t") {
        $migrationFiles = Get-ChildItem (Join-Path $BetbotRoot "migrations\*.up.sql") | Sort-Object Name
        foreach ($migrationFile in $migrationFiles) {
            Write-Host "Applying Betbot migration: $($migrationFile.Name)"
            Get-Content $migrationFile.FullName -Raw | docker exec -i betbot-postgres psql -U betbot -d betbot -v ON_ERROR_STOP=1 -f -
            if ($LASTEXITCODE -ne 0) {
                throw "Failed applying migration $($migrationFile.Name)"
            }
        }
    }

    # Health endpoint requires at least one recent successful poll row.
    docker exec -i betbot-postgres psql -U betbot -d betbot -v ON_ERROR_STOP=1 -c "INSERT INTO poll_runs (source, started_at, finished_at, status, games_seen, snapshots_seen, inserts_count, dedup_skips, error_text) SELECT 'migration-bootstrap', NOW(), NOW(), 'success', 0, 0, 0, 0, '' WHERE NOT EXISTS (SELECT 1 FROM poll_runs);"
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to seed initial poll_runs row for Betbot health readiness."
    }
}

$tradebotRoot = "C:\dev\Tradebot"
$betbotRoot = "C:\dev\betbot"
$tradebotOldCompose = Join-Path $tradebotRoot "docker\docker-compose.yml"
$betbotOldCompose = Join-Path $betbotRoot "deploy\docker\docker-compose.yml"
$tradebotCompose = Join-Path $tradebotRoot "docker-compose.yml"
$betbotCompose = Join-Path $betbotRoot "docker-compose.yml"

if (-not (Test-Path $tradebotCompose)) {
    throw "Missing expected compose file: $tradebotCompose"
}
if (-not (Test-Path $betbotCompose)) {
    throw "Missing expected compose file: $betbotCompose"
}

# Step 1: bring down old shared context ("docker" project from nested compose paths)
if (Test-Path $tradebotOldCompose) {
    Invoke-CheckedCommand "docker compose -p docker -f `"$tradebotOldCompose`" down --remove-orphans"
}
if (Test-Path $betbotOldCompose) {
    Invoke-CheckedCommand "docker compose -p docker -f `"$betbotOldCompose`" down --remove-orphans"
}

# Step 2: bring up isolated Tradebot stack and wait for readiness.
Invoke-CheckedCommand "docker compose -f `"$tradebotCompose`" up -d --build"
Wait-ComposeProjectReady -ProjectName "tradebot" -AllowExitedServices @("migrate") -TimeoutSeconds 300

# Step 3: bring up isolated Betbot stack and wait for readiness.
Invoke-CheckedCommand "docker compose -f `"$betbotCompose`" up -d --build"
Initialize-BetbotDatabaseIfNeeded -BetbotRoot $betbotRoot
Wait-ComposeProjectReady -ProjectName "betbot" -TimeoutSeconds 240

Write-Host "Docker compose migration complete."
