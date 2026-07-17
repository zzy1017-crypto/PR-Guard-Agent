[CmdletBinding()]
param(
    [int]$ProjectID = 6,
    [int]$DiffID = 5,
    [int]$TopK = 5,
    [uint64]$TaskID = 0,
    [string]$BaseURL = "http://localhost:8080",
    [int]$Requests = 200,
    [int]$Concurrency = 10,
    [string]$Provider = "mock",
    [string]$OutputPath = "docs/benchmark-results.md",
    [switch]$ConfirmPrepared
)

$ErrorActionPreference = "Stop"
$BaseURL = $BaseURL.TrimEnd("/")

function API([string]$Method, [string]$Path) {
    return Invoke-RestMethod -Method $Method -Uri "$BaseURL$Path" -ContentType "application/json"
}
function Parse-Hey([string]$Name, [string]$Method, [string]$URL, [string]$Raw) {
    $total = if ($Raw -match '(?m)^\s*Total:\s+([0-9.]+) secs') { $Matches[1] } else { "not parsed" }
    $rps = if ($Raw -match '(?m)^\s*Requests/sec:\s+([0-9.]+)') { $Matches[1] } else { "not parsed" }
    $average = if ($Raw -match '(?m)^\s*Average:\s+([0-9.]+) secs') { $Matches[1] } else { "not parsed" }
    $percentiles = @{}
    foreach ($p in @(50, 90, 95, 99)) {
        if ($Raw -match "(?m)^\s*$p%\s+in\s+([0-9.]+) secs") { $percentiles[$p] = $Matches[1] } else { $percentiles[$p] = "not parsed" }
    }
    $success = 0
    foreach ($match in [regex]::Matches($Raw, '(?m)^\s*\[(\d{3})\]\s+(\d+) responses')) {
        $status = [int]$match.Groups[1].Value
        if ($status -ge 200 -and $status -lt 300) { $success += [int]$match.Groups[2].Value }
    }
    $failure = $Requests - $success
    return [pscustomobject]@{
        Name = $Name; Method = $Method; URL = $URL; Requests = $Requests; Concurrency = $Concurrency
        Total = $total; RPS = $rps; Average = $average; P50 = $percentiles[50]; P90 = $percentiles[90]
        P95 = $percentiles[95]; P99 = $percentiles[99]; Success = $success; Failure = $failure; Raw = $Raw
    }
}
function Run-Hey([string]$Name, [string]$Method, [string]$Path) {
    $url = "$BaseURL$Path"
    Write-Host "[INFO] benchmarking $Method $url"
    $arguments = @("-n", $Requests, "-c", $Concurrency, "-m", $Method, $url)
    $raw = (& hey @arguments 2>&1 | Out-String)
    if ($LASTEXITCODE -ne 0) { throw "hey failed for $Name`n$raw" }
    return Parse-Hey $Name $Method $url $raw
}

try {
    if (-not (Get-Command hey -ErrorAction SilentlyContinue)) {
        throw "hey is not installed or not on PATH. Install it manually, then rerun; this script never installs external tools."
    }
    Write-Warning "This is a development baseline, not a production performance claim."
    Write-Host "Before continuing: use mock provider; pre-warm Analyze cache; temporarily raise/disable test rate limiting; do not target a real LLM API."
    if (-not $ConfirmPrepared) { throw "review the preparation warning and rerun with -ConfirmPrepared" }
    if ($Provider -ne "mock") { throw "high-concurrency benchmark is restricted to Provider=mock" }

    $warm1 = API POST "/projects/$ProjectID/diffs/$DiffID/analyze?top_k=$TopK"
    $warm2 = API POST "/projects/$ProjectID/diffs/$DiffID/analyze?top_k=$TopK"
    if ($warm2.data.cached -ne $true) { throw "Analyze cache warm-up did not produce cached=true" }
    Write-Host "[PASS] Analyze cache is warm"

    if ($TaskID -eq 0) {
        $submitted = API POST "/projects/$ProjectID/diffs/$DiffID/analysis-tasks?top_k=$TopK"
        $TaskID = [uint64]$submitted.data.task_id
    }

    $results = @(
        Run-Hey "health" "GET" "/health"
        Run-Hey "analyze-cache-hit" "POST" "/projects/$ProjectID/diffs/$DiffID/analyze?top_k=$TopK"
        Run-Hey "analysis-task-get" "GET" "/analysis-tasks/$TaskID"
        Run-Hey "workers" "GET" "/ops/workers"
        Run-Hey "task-metrics" "GET" "/ops/analysis-tasks/metrics"
    )

    $outputDirectory = Split-Path -Parent $OutputPath
    if ($outputDirectory) { New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null }
    $cpu = (Get-CimInstance Win32_Processor -ErrorAction SilentlyContinue | Select-Object -First 1 -ExpandProperty Name)
    $memoryBytes = (Get-CimInstance Win32_ComputerSystem -ErrorAction SilentlyContinue).TotalPhysicalMemory
    $memoryGB = if ($memoryBytes) { [math]::Round($memoryBytes / 1GB, 2) } else { "unknown" }
    $timestamp = (Get-Date).ToString("yyyy-MM-dd HH:mm:ss K")

    $lines = @(
        "# DAY21 benchmark results",
        "",
        "> Development baseline only. These measurements must not be presented as production capacity.",
        "",
        "- Time: $timestamp",
        "- Hardware: CPU=$cpu; RAM=${memoryGB}GB",
        "- Provider: $Provider",
        "- Analyze cache state: warmed; cached=true verified",
        "- Requests per endpoint: $Requests",
        "- Concurrency: $Concurrency",
        "- Base URL: $BaseURL",
        "",
        "| Endpoint | Requests | Concurrency | Total (s) | RPS | Avg (s) | P50 | P90 | P95 | P99 | Success | Failure |",
        "|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|"
    )
    foreach ($result in $results) {
        $lines += "| $($result.Method) $($result.Name) | $($result.Requests) | $($result.Concurrency) | $($result.Total) | $($result.RPS) | $($result.Average) | $($result.P50) | $($result.P90) | $($result.P95) | $($result.P99) | $($result.Success) | $($result.Failure) |"
    }
    $lines += @("", "## Raw hey output", "")
    foreach ($result in $results) {
        $lines += @("### $($result.Method) $($result.URL)", "", '```text', $result.Raw.TrimEnd(), '```', "")
    }
    Set-Content -Path $OutputPath -Value $lines -Encoding UTF8
    Write-Host "[PASS] real benchmark output written to $OutputPath" -ForegroundColor Green
    Write-Warning "Restore the test environment's original rate-limit settings now."
    exit 0
} catch {
    Write-Host "[FAIL] $($_.Exception.Message)" -ForegroundColor Red
    Write-Warning "If you changed rate limiting for this run, restore its original settings."
    exit 1
}
