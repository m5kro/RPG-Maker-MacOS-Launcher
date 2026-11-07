$ErrorActionPreference = 'Stop'
$runDir = $null
try {
    Write-Host 'RPG Maker MZ benchmark (Windows x64)'
    Write-Host 'Use only a trusted game you have permission to run. The copy is NOT a sandbox.'
    Write-Host 'Extract this tool OUTSIDE the game folder. Keep the game window focused.'
    $game = (Read-Host 'Game folder containing index.html and js/rmmz_core.js').Trim().Trim('"')
    $runtime = (Read-Host 'Full path to Windows NW.js nw.exe (or the NW.js-based Game.exe)').Trim().Trim('"')
    $mode = Read-Host 'Mode: 1 = fixed map (default), 2 = manual gameplay'
    if ($mode -notin @('', '1', '2')) { throw 'Choose 1 or 2.' }
    $game = (Resolve-Path -LiteralPath $game).Path
    $runtime = (Resolve-Path -LiteralPath $runtime).Path
    $warmup = 10
    if ($mode -eq '2') {
        $warmup = 60
        Write-Host 'Manual mode: reach the target scene during the 60-second warmup.'
        Write-Host 'Timing is automatic, not triggered by reaching that scene. A title screen is not gameplay.'
    }
    $runDir = Join-Path $PSScriptRoot ('results\' + (Get-Date -Format 'yyyyMMdd-HHmmss') + '-' + [guid]::NewGuid().ToString('N').Substring(0, 8))
    $reportDir = Join-Path $runDir 'report'
    New-Item -ItemType Directory -Path $reportDir | Out-Null
    $runInfo = [ordered]@{
        mode = $(if ($mode -eq '2') { 'manual' } else { 'fixed' })
        game_folder_name = Split-Path -Leaf $game
        warmup_seconds = $warmup
        duration_seconds = 30
        recorded_at = (Get-Date).ToString('o')
    }
    $runInfo | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $reportDir 'run-info.json') -Encoding UTF8
    try {
        $machine = [ordered]@{
            os = @(Get-CimInstance Win32_OperatingSystem | Select-Object Caption, Version, OSArchitecture)
            cpu = @(Get-CimInstance Win32_Processor | Select-Object Name, NumberOfCores, NumberOfLogicalProcessors)
            memory = @(Get-CimInstance Win32_ComputerSystem | Select-Object TotalPhysicalMemory)
            gpu = @(Get-CimInstance Win32_VideoController | Select-Object Name, DriverVersion, CurrentHorizontalResolution, CurrentVerticalResolution, CurrentRefreshRate)
        }
        $machine | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $reportDir 'machine.json') -Encoding UTF8
    } catch {
        $_.ToString() | Set-Content -LiteralPath (Join-Path $reportDir 'machine-error.txt') -Encoding UTF8
    }
    $arguments = @('--game', $game, '--runtime', $runtime, '--root', (Join-Path $runDir 'run'), '--warmup', "$warmup", '--duration', '30')
    if ($mode -ne '2') { $arguments += '--fixed' }
    Write-Host 'Copying the game, then measuring. Do not close this console. No progress bar during copying.'
    Write-Host 'The game exits automatically after measurement. Results remain under:' $runDir
    # Windows PowerShell treats native stderr as non-terminating errors, including normal NW.js warnings.
    $ErrorActionPreference = 'Continue'
    & (Join-Path $PSScriptRoot 'benchmark-mz.exe') @arguments > (Join-Path $reportDir 'console.log') 2>&1
    $exitCode = $LASTEXITCODE
    $ErrorActionPreference = 'Stop'
    $result = Join-Path $runDir 'run\result.json'
    if (Test-Path -LiteralPath $result) {
        Copy-Item -LiteralPath $result -Destination (Join-Path $reportDir 'result.json')
    }
    if ($exitCode -ne 0) { throw "Benchmark failed (exit $exitCode). See console.log; keep this report for diagnosis." }
    if (-not (Test-Path -LiteralPath $result)) { throw 'No result.json was produced.' }
    Write-Host 'Measurement completed.'
} catch {
    Write-Host ('ERROR: ' + $_.ToString()) -ForegroundColor Red
    if ($runDir -and (Test-Path -LiteralPath (Join-Path $runDir 'report'))) {
        $_.ToString() | Set-Content -LiteralPath (Join-Path $runDir 'report\error.txt') -Encoding UTF8
    }
} finally {
    if ($runDir -and (Test-Path -LiteralPath (Join-Path $runDir 'report'))) {
        try {
            $zip = Join-Path $runDir 'report.zip'
            Compress-Archive -LiteralPath (Join-Path $runDir 'report') -DestinationPath $zip
            Write-Host 'Send ONLY this small report ZIP, not the copied game/profile:'
            Write-Host $zip -ForegroundColor Green
            Write-Host 'Review console.log before sharing: warnings may contain local paths.'
        } catch {
            Write-Host ('Could not create report ZIP: ' + $_.ToString()) -ForegroundColor Red
        }
    }
    Read-Host 'Press Enter to close' | Out-Null
}
