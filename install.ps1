# JakLingko CLI & MCP Server Windows Installer (PowerShell)
# Run via: powershell -ExecutionPolicy Bypass -File install.ps1

$ErrorActionPreference = "Stop"

$Repo = "bardiz12/jaklingko-client"
$CliBin = "jaklingko-cli.exe"
$McpBin = "jaklingko-mcp.exe"

Write-Host "==================================================" -ForegroundColor Cyan
Write-Host "    JakLingko CLI & MCP Server Windows Installer  " -ForegroundColor Cyan
Write-Host "==================================================" -ForegroundColor Cyan

# 1. Resolve Target Version
$Tag = $args[0]
if (-not $Tag) {
    Write-Host "[INFO] Fetching latest release tag from GitHub..." -ForegroundColor Blue
    try {
        $ReleaseInfo = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ "Accept" = "application/vnd.github+json" }
        $Tag = $ReleaseInfo.tag_name
    } catch {
        $Tag = "v0.1.0"
        Write-Host "[WARN] Could not resolve latest release dynamically. Falling back to: $Tag" -ForegroundColor Yellow
    }
}

$Version = $Tag.TrimStart("v")
$ArchiveName = "jaklingko-client_${Version}_windows_amd64.zip"
$DownloadUrl = "https://github.com/$Repo/releases/download/$Tag/$ArchiveName"

$TempDir = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $TempDir -Force | Out-Null

try {
    $ZipPath = Join-Path $TempDir $ArchiveName
    Write-Host "[INFO] Downloading $ArchiveName..." -ForegroundColor Blue

    $Downloaded = $false

    # Method 1: Direct public release download
    try {
        Invoke-WebRequest -Uri $DownloadUrl -OutFile $ZipPath -UseBasicParsing -ErrorAction Stop
        $Downloaded = $true
    } catch {}

    # Method 2: gh CLI if installed
    if (-not $Downloaded -and (Get-Command gh -ErrorAction SilentlyContinue)) {
        try {
            gh release download $Tag -p $ArchiveName -R $Repo --dir $TempDir
            if (Test-Path $ZipPath) { $Downloaded = $true }
        } catch {}
    }

    # Method 3: Authenticated API if GITHUB_TOKEN or GH_TOKEN is present
    if (-not $Downloaded) {
        $Token = $env:GITHUB_TOKEN
        if (-not $Token) { $Token = $env:GH_TOKEN }
        if ($Token) {
            try {
                $Headers = @{
                    "Authorization" = "Bearer $Token"
                    "Accept" = "application/vnd.github+json"
                }
                $Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/tags/$Tag" -Headers $Headers
                $Asset = $Release.assets | Where-Object { $_.name -eq $ArchiveName } | Select-Object -First 1
                if ($Asset) {
                    $AssetHeaders = @{
                        "Authorization" = "Bearer $Token"
                        "Accept" = "application/octet-stream"
                    }
                    Invoke-WebRequest -Uri $Asset.url -Headers $AssetHeaders -OutFile $ZipPath -UseBasicParsing
                    $Downloaded = $true
                }
            } catch {}
        }
    }

    if (-not $Downloaded) {
        throw "Failed to download $ArchiveName from $DownloadUrl. Check network or set GITHUB_TOKEN if repository is private."
    }

    Write-Host "[INFO] Extracting binaries..." -ForegroundColor Blue
    Expand-Archive -Path $ZipPath -DestinationPath $TempDir -Force

    # Target Install Directory: $HOME\bin or AppData\Local\Microsoft\WindowsApps
    $InstallDir = Join-Path $env:USERPROFILE "bin"
    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }

    Copy-Item -Path (Join-Path $TempDir $CliBin) -Destination (Join-Path $InstallDir $CliBin) -Force
    Copy-Item -Path (Join-Path $TempDir $McpBin) -Destination (Join-Path $InstallDir $McpBin) -Force

    Write-Host "[OK] Successfully installed:" -ForegroundColor Green
    Write-Host "  - CLI Tool   : $(Join-Path $InstallDir $CliBin)" -ForegroundColor Green
    Write-Host "  - MCP Server : $(Join-Path $InstallDir $McpBin)" -ForegroundColor Green

    # Check PATH
    $UserPath = [System.Environment]::GetEnvironmentVariable("Path", "User")
    if ($UserPath -notlike "*$InstallDir*") {
        Write-Host "[INFO] Adding $InstallDir to User PATH environment variable..." -ForegroundColor Blue
        [System.Environment]::SetEnvironmentVariable("Path", "$UserPath;$InstallDir", "User")
        $env:Path += ";$InstallDir"
        Write-Host "[OK] PATH updated successfully." -ForegroundColor Green
    }

    Write-Host "`nQuick Test:" -ForegroundColor Yellow
    Write-Host "  $CliBin -origin `"Dukuh Atas`" -destination `"Monas`""
    Write-Host "  $McpBin -version`n"
}
finally {
    Remove-Item -Path $TempDir -Recurse -Force -ErrorAction SilentlyContinue
}
