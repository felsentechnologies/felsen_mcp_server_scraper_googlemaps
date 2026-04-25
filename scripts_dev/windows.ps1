param(
    [string]$ImageName = "mcp-googlemaps",
    [int]$HostPort = 3000,
    [string]$BearerToken = "",
    [string]$AllowedOrigins = ""
)

$ErrorActionPreference = "Stop"
$ProjectRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$ComposeFile = Join-Path $ProjectRoot "Docker\docker-compose.yaml"
$TokenDir = Join-Path $env:LOCALAPPDATA "FelsenTechnologies\mcp-googlemaps"
$TokenFile = Join-Path $TokenDir "bearer-token.txt"
$GeneratedBearerToken = $false
$LastComposeSucceeded = $false

function Write-Title {
    param([string]$Text)
    Write-Host ""
    Write-Host "=== $Text ===" -ForegroundColor Cyan
}

function Write-FelsenBanner {
    Write-Host "  ______    _                  _____         _                 _             _           " -ForegroundColor Cyan
    Write-Host " |  ____|  | |                |_   _|       | |               | |           (_)          " -ForegroundColor Cyan
    Write-Host " | |__ ___ | |___  ___ _ __     | | ___  ___| |__  _ __   ___ | | ___   __ _ _  ___  ___ " -ForegroundColor Cyan
    Write-Host " |  __/ _ \| / __|/ _ \ '_ \    | |/ _ \/ __| '_ \| '_ \ / _ \| |/ _ \ / _` | |/ _ \/ __|" -ForegroundColor Cyan
    Write-Host " | | |  __/| \__ \  __/ | | |   | |  __/ (__| | | | | | | (_) | | (_) | (_| | |  __/\__ \" -ForegroundColor Cyan
    Write-Host " |_|  \___||_|___/\___|_| |_|   \_/\___|\___|_| |_|_| |_|\___/|_|\___/ \__, |_|\___||___/" -ForegroundColor Cyan
    Write-Host "                                                                        __/ |             " -ForegroundColor Cyan
    Write-Host "                                                                       |___/              " -ForegroundColor Cyan
}

function Test-Docker {
    try {
        & docker version | Out-Null
        if ($LASTEXITCODE -ne 0) {
            Write-Host "Docker nao esta respondendo. Abra o Docker Desktop e tente novamente." -ForegroundColor Red
            return $false
        }
        return $true
    }
    catch {
        Write-Host "Docker nao esta disponivel. Abra o Docker Desktop e tente novamente." -ForegroundColor Red
        return $false
    }
}

function New-BearerToken {
    $bytes = New-Object byte[] 32
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $rng.GetBytes($bytes)
    }
    finally {
        $rng.Dispose()
    }
    return [Convert]::ToBase64String($bytes).TrimEnd("=").Replace("+", "-").Replace("/", "_")
}

function Initialize-BearerToken {
    if ([string]::IsNullOrWhiteSpace($script:BearerToken)) {
        if (Test-Path -LiteralPath $script:TokenFile) {
            $savedToken = (Get-Content -LiteralPath $script:TokenFile -Raw).Trim()
            if (-not [string]::IsNullOrWhiteSpace($savedToken)) {
                $script:BearerToken = $savedToken
                $script:GeneratedBearerToken = $false
                return
            }
        }

        $script:BearerToken = New-BearerToken
        $script:GeneratedBearerToken = $true
        if (-not (Test-Path -LiteralPath $script:TokenDir)) {
            New-Item -ItemType Directory -Path $script:TokenDir -Force | Out-Null
        }
        Set-Content -LiteralPath $script:TokenFile -Value $script:BearerToken -NoNewline
    }
}

function Write-BearerTokenInfo {
    if ([string]::IsNullOrWhiteSpace($script:BearerToken)) { return }

    Write-Host ""
    if ($script:GeneratedBearerToken) {
        Write-Host "HTTP_BEARER_TOKEN gerado automaticamente:" -ForegroundColor Yellow
    }
    else {
        Write-Host "HTTP_BEARER_TOKEN configurado:" -ForegroundColor Yellow
    }
    Write-Host $script:BearerToken -ForegroundColor Green
    Write-Host ""
    Write-Host "Use este valor na OpenAI como authorization/bearer token." -ForegroundColor Yellow
}

function Invoke-Compose {
    param([string[]]$ArgsList)
    $script:LastComposeSucceeded = $false
    if (-not (Test-Docker)) { return }
    Push-Location $ProjectRoot
    try {
        $env:HTTP_PORT = "$HostPort"
        $env:IMAGE_NAME = $ImageName
        $env:HTTP_BEARER_TOKEN = $BearerToken
        $env:MCP_BEARER_TOKEN = $BearerToken
        $env:MCP_ALLOWED_ORIGINS = $AllowedOrigins
        & docker compose -f $ComposeFile @ArgsList
        if ($LASTEXITCODE -ne 0) {
            throw "docker compose falhou com codigo de saida $LASTEXITCODE"
        }
        $script:LastComposeSucceeded = $true
    }
    finally {
        Pop-Location
    }
}

function Invoke-StackBuild {
    Write-Title "Build da stack"
    Invoke-Compose @("build")
}

function Start-Stack {
    Write-Title "Subir stack"
    Initialize-BearerToken
    Invoke-Compose @("up", "-d", "--build")
    if ($script:LastComposeSucceeded) {
        Write-BearerTokenInfo
    }
}

function Stop-Stack {
    Write-Title "Parar stack"
    Invoke-Compose @("down")
}

function Restart-Stack {
    Stop-Stack
    Start-Stack
}

function RecreateStack {
    Write-Title "Recriar stack"
    Initialize-BearerToken
    Invoke-Compose @("up", "-d", "--build", "--force-recreate")
    if ($script:LastComposeSucceeded) {
        Write-BearerTokenInfo
    }
}

function Show-Logs {
    Write-Title "Logs"
    Invoke-Compose @("logs", "-f")
}

function Show-Status {
    Write-Title "Status"
    Invoke-Compose @("ps")
}

function Show-ComposeConfig {
    Write-Title "Config Docker Compose"
    Invoke-Compose @("config")
}

function Test-Health {
    Write-Title "Health check"
    Initialize-BearerToken
    $url = "http://localhost:$HostPort/health"
    try {
        Invoke-RestMethod -Uri $url -Method Get -Headers (Get-AuthHeaders)
    }
    catch {
        Write-Host "Falha ao chamar $url" -ForegroundColor Red
        Write-Host $_.Exception.Message
    }
}

function Test-Scrape {
    Write-Title "Teste POST /scrape"
    Initialize-BearerToken
    $url = "http://localhost:$HostPort/scrape"
    $body = @{
        searchQueries = @("pizzarias em Curitiba")
        maxPlacesPerQuery = 3
        scrapeEmails = $false
        scrapePhones = $false
        language = "pt-BR"
    } | ConvertTo-Json

    try {
        Invoke-RestMethod -Uri $url -Method Post -Headers (Get-AuthHeaders) -ContentType "application/json" -Body $body
    }
    catch {
        Write-Host "Falha ao chamar $url" -ForegroundColor Red
        Write-Host $_.Exception.Message
    }
}

function Test-MCP {
    Write-Title "Teste POST /mcp"
    Initialize-BearerToken
    $url = "http://localhost:$HostPort/mcp"
    $headers = Get-AuthHeaders
    $headers["Accept"] = "application/json, text/event-stream"
    $headers["MCP-Protocol-Version"] = "2025-06-18"

    $body = @{
        jsonrpc = "2.0"
        id = 1
        method = "tools/list"
        params = @{}
    } | ConvertTo-Json -Depth 5

    try {
        Invoke-RestMethod -Uri $url -Method Post -Headers $headers -ContentType "application/json" -Body $body
    }
    catch {
        Write-Host "Falha ao chamar $url" -ForegroundColor Red
        Write-Host $_.Exception.Message
    }
}

function Get-AuthHeaders {
    Initialize-BearerToken
    return @{
        "Authorization" = "Bearer $script:BearerToken"
        "Accept" = "application/json, text/event-stream"
    }
}

function Show-Menu {
    Clear-Host
    Write-FelsenBanner
    Write-Host ""
    Write-Host "MCP Google Maps - Docker Dev" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "Projeto:       $ProjectRoot"
    Write-Host "Compose:       $ComposeFile"
    Write-Host "Imagem:        $ImageName`:latest"
    Write-Host "Porta local:   $HostPort"
    Write-Host "Bearer token:  $(if ($BearerToken) { 'configurado' } else { 'sera gerado ao subir a stack' })"
    Write-Host "Token salvo:   $TokenFile"
    Write-Host ""
    Write-Host "1. Buildar stack"
    Write-Host "2. Subir stack"
    Write-Host "3. Parar stack"
    Write-Host "4. Reiniciar stack"
    Write-Host "5. Recriar stack"
    Write-Host "6. Ver status"
    Write-Host "7. Ver logs"
    Write-Host "8. Health check"
    Write-Host "9. Testar /scrape"
    Write-Host "10. Testar /mcp"
    Write-Host "11. Ver config compose"
    Write-Host "0. Sair"
    Write-Host ""
}

do {
    Show-Menu
    $choice = Read-Host "Escolha uma opcao"

    try {
        switch ($choice) {
            "1" { Invoke-StackBuild }
            "2" { Start-Stack }
            "3" { Stop-Stack }
            "4" { Restart-Stack }
            "5" { RecreateStack }
            "6" { Show-Status }
            "7" { Show-Logs }
            "8" { Test-Health }
            "9" { Test-Scrape }
            "10" { Test-MCP }
            "11" { Show-ComposeConfig }
            "0" { break }
            default { Write-Host "Opcao invalida." -ForegroundColor Yellow }
        }
    }
    catch {
        Write-Host ""
        Write-Host "Erro: $($_.Exception.Message)" -ForegroundColor Red
    }

    if ($choice -ne "0" -and $choice -ne "7") {
        Write-Host ""
        Read-Host "Pressione Enter para continuar"
    }
} while ($choice -ne "0")
