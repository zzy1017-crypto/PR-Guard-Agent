[CmdletBinding()]
param(
    [int]$ProjectID = 6,
    [int]$DiffID = 5,
    [int]$TopK = 5,
    [int]$AlternateTopK = 8,
    [string]$BaseURL = "http://localhost:8080",
    [int]$PollIntervalSeconds = 1,
    [int]$PollTimeoutSeconds = 120
)

$ErrorActionPreference = "Stop"
$BaseURL = $BaseURL.TrimEnd("/")

function Write-Pass([string]$Message) { Write-Host "[PASS] $Message" -ForegroundColor Green }
function Write-Fail([string]$Message) { Write-Host "[FAIL] $Message" -ForegroundColor Red; exit 1 }
function Assert-True([bool]$Condition, [string]$Message) { if (-not $Condition) { Write-Fail $Message }; Write-Pass $Message }
function Invoke-API([string]$Method, [string]$Path) {
    try {
        return Invoke-RestMethod -Method $Method -Uri "$BaseURL$Path" -ContentType "application/json"
    } catch {
        $detail = $_.Exception.Message
        if ($_.ErrorDetails.Message) { $detail = "$detail; $($_.ErrorDetails.Message)" }
        throw "$Method $Path failed: $detail"
    }
}

try {
    if ($TopK -lt 1 -or $TopK -gt 20 -or $AlternateTopK -lt 1 -or $AlternateTopK -gt 20) {
        throw "TopK and AlternateTopK must be between 1 and 20"
    }
    if ($TopK -eq $AlternateTopK) { throw "AlternateTopK must differ from TopK" }

    $health = Invoke-API GET "/health"
    Assert-True ($health.code -eq 0) "GET /health"

    $retrieve = Invoke-API POST "/projects/$ProjectID/diffs/$DiffID/retrieve?top_k=$TopK"
    Assert-True ($retrieve.code -eq 0 -and $null -ne $retrieve.data.context_chunks) "RAG retrieve returns context_chunks"

    $first = Invoke-API POST "/projects/$ProjectID/diffs/$DiffID/analyze?top_k=$TopK"
    Assert-True ($first.code -eq 0) "first synchronous analyze succeeds"
    Assert-True ($first.data.cached -eq $false) "first synchronous analyze has cached=false"

    $second = Invoke-API POST "/projects/$ProjectID/diffs/$DiffID/analyze?top_k=$TopK"
    Assert-True ($second.data.cached -eq $true) "second synchronous analyze has cached=true"

    $alternate = Invoke-API POST "/projects/$ProjectID/diffs/$DiffID/analyze?top_k=$AlternateTopK"
    Assert-True ($alternate.data.cached -eq $false) "different top_k does not reuse the first cache entry"

    $submitted = Invoke-API POST "/projects/$ProjectID/diffs/$DiffID/analysis-tasks?top_k=$TopK"
    $taskID = [uint64]$submitted.data.task_id
    Assert-True ($taskID -gt 0) "async task submitted; task_id=$taskID"

    $deadline = (Get-Date).AddSeconds($PollTimeoutSeconds)
    do {
        $task = Invoke-API GET "/analysis-tasks/$taskID"
        $status = [string]$task.data.status
        Write-Host "[INFO] task_id=$taskID status=$status attempt_count=$($task.data.attempt_count)"
        if ($status -in @("succeeded", "failed")) { break }
        Start-Sleep -Seconds $PollIntervalSeconds
    } while ((Get-Date) -lt $deadline)

    Assert-True ($status -eq "succeeded") "async task reaches succeeded"

    $metrics = Invoke-API GET "/ops/analysis-tasks/metrics"
    Assert-True ($metrics.code -eq 0 -and $null -ne $metrics.data.current) "GET /ops/analysis-tasks/metrics"

    $workers = Invoke-API GET "/ops/workers"
    Assert-True ($workers.code -eq 0 -and $null -ne $workers.data.workers) "GET /ops/workers"

    Write-Host "[PASS] DAY21 end-to-end test completed" -ForegroundColor Green
    exit 0
} catch {
    Write-Fail $_.Exception.Message
}
