# TiandingJi (familyledger) - one-click deploy to FeiNiu (fnOS) NAS
# Double-click to run. Requires: SSH enabled on NAS (fnOS Settings -> SSH).
# English only on purpose: avoids PowerShell 5.1 UTF-8 encoding issues.

$ErrorActionPreference = "Stop"

$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$root = Split-Path -Parent $here
$bin  = Join-Path $root "dist\familyledger"
if (-not (Test-Path $bin)) { $bin = Join-Path $root "dist\familyledger-linux-amd64" }
if (-not (Test-Path $bin)) {
    Write-Host "ERROR: binary not found in $root\dist\" -ForegroundColor Red
    Read-Host "Press Enter to exit"; exit 1
}

Write-Host "=== TiandingJi NAS deploy ===" -ForegroundColor Cyan
Write-Host "Binary: $bin" -ForegroundColor Gray

$ip = Read-Host "NAS LAN IP (e.g. 192.168.110.50)"
$ip = $ip.Trim()
$user = Read-Host "SSH user [default: xuan]"
if ([string]::IsNullOrWhiteSpace($user)) { $user = "xuan" }

Write-Host ""
Write-Host "This script will:" -ForegroundColor Yellow
Write-Host "  1) upload the binary to /vol1/docker/familyledger/bin/"
Write-Host "  2) write docker-compose.yml there"
Write-Host "  3) start the container on port 8080"
Write-Host ""
$tok = Read-Host "Family passcode for the app (6 digits) [default: 123456]"
if ([string]::IsNullOrWhiteSpace($tok)) { $tok = "123456" }

function Run-Remote($cmd) {
    Write-Host "  > $cmd" -ForegroundColor DarkGray
    ssh -o StrictHostKeyChecking=no -o ConnectTimeout=8 "${user}@${ip}" $cmd
    if ($LASTEXITCODE -ne 0) { throw "remote command failed (exit $LASTEXITCODE)" }
}

try {
    Write-Host ""
    Write-Host "[1/5] Testing SSH connection..." -ForegroundColor Cyan
    Run-Remote "uname -a"

    Write-Host ""
    Write-Host "[2/5] Preparing folders on NAS..." -ForegroundColor Cyan
    Run-Remote "sudo mkdir -p /vol1/docker/familyledger/bin /vol1/docker/familyledger/data /vol1/docker/familyledger/backup"

    Write-Host ""
    Write-Host "[3/5] Uploading binary (16MB, may take a while)..." -ForegroundColor Cyan
    scp -o StrictHostKeyChecking=no -o ConnectTimeout=8 "$bin" "${user}@${ip}:/tmp/familyledger"
    if ($LASTEXITCODE -ne 0) { throw "upload failed" }
    Run-Remote "sudo mv /tmp/familyledger /vol1/docker/familyledger/bin/familyledger && sudo chmod 755 /vol1/docker/familyledger/bin/familyledger"

    Write-Host ""
    Write-Host "[4/5] Writing docker-compose.yml..." -ForegroundColor Cyan
    $compose = @"
services:
  familyledger:
    image: alpine:3.20
    container_name: familyledger
    restart: unless-stopped
    command: ["/bin/sh","-c","chmod +x /app/familyledger && exec /app/familyledger"]
    ports:
      - "8080:8080"
    volumes:
      - /vol1/docker/familyledger/bin:/app
      - /vol1/docker/familyledger/data:/data
      - /vol1/docker/familyledger/backup:/backup
    environment:
      - TZ=Asia/Shanghai
      - FL_DB=/data/familyledger.db
      - FL_ADDR=:8080
      - FL_SHARED_TOKEN=$tok
      - FL_BACKUP_DIR=/backup
"@
    $tmpfile = [System.IO.Path]::GetTempFileName()
    [System.IO.File]::WriteAllText($tmpfile, $compose)
    scp -o StrictHostKeyChecking=no $tmpfile "${user}@${ip}:/tmp/td-compose.yml"
    Run-Remote "sudo mv /tmp/td-compose.yml /vol1/docker/familyledger/docker-compose.yml"
    Remove-Item $tmpfile -Force

    Write-Host ""
    Write-Host "[5/5] Starting container..." -ForegroundColor Cyan
    Run-Remote "cd /vol1/docker/familyledger && (sudo docker compose up -d || sudo docker-compose up -d)"

    Start-Sleep -Seconds 4
    Write-Host ""
    Write-Host "=== Container status ===" -ForegroundColor Cyan
    Run-Remote "sudo docker ps --filter name=familyledger --format '{{.Status}} | {{.Ports}}'"

    Write-Host ""
    Write-Host "Done. Open  http://${ip}:8080  in your browser." -ForegroundColor Green
    Write-Host "Passcode: $tok" -ForegroundColor Green
}
catch {
    Write-Host ""
    Write-Host "FAILED: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host ""
    Write-Host "Common causes:" -ForegroundColor Yellow
    Write-Host "  - SSH not enabled: fnOS Settings -> SSH -> enable"
    Write-Host "  - wrong NAS IP"
    Write-Host "  - Docker app not installed yet: open Docker once from fnOS desktop"
}

Read-Host "Press Enter to exit"
