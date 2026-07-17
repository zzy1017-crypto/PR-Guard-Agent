[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("invalid_json", "llm_timeout", "qdrant_unavailable", "redis_fail_open", "retry_exhausted")]
    [string]$Scenario,
    [int]$ProjectID = 6,
    [int]$DiffID = 5,
    [int]$TopK = 9,
    [string]$BaseURL = "http://localhost:8080",
    [string]$QdrantContainer = "prguard-qdrant",
    [string]$RedisContainer = "prguard-redis",
    [string]$ApplicationLogPath = "",
    [int]$PollTimeoutSeconds = 180,
    [switch]$ConfirmDisruption
)

$ErrorActionPreference = "Stop"
$BaseURL = $BaseURL.TrimEnd("/")

function API([string]$Method, [string]$Path) {
    return Invoke-RestMethod -Method $Method -Uri "$BaseURL$Path" -ContentType "application/json"
}
function Assert([bool]$Condition, [string]$Message) {
    if (-not $Condition) { throw $Message }
    Write-Host "[PASS] $Message" -ForegroundColor Green
}
function Assert-NoCacheKey {
    $keys = @(& docker exec $RedisContainer redis-cli -n 0 --scan --pattern "prguard:report:${ProjectID}:*:topk:$TopK")
    if ($LASTEXITCODE -ne 0) { throw "could not scan Redis report cache" }
    Assert ($keys.Count -eq 0) "no formal report cache exists for project=$ProjectID top_k=$TopK"
}
function Submit-NewTask {
    $response = API POST "/projects/$ProjectID/diffs/$DiffID/analysis-tasks?top_k=$TopK"
    Assert ($response.data.reused -eq $false) "fault test created a new task (not an old idempotent result)"
    return [uint64]$response.data.task_id
}
function Poll-Until([uint64]$TaskID, [scriptblock]$Predicate, [string]$Description) {
    $deadline = (Get-Date).AddSeconds($PollTimeoutSeconds)
    do {
        $task = API GET "/analysis-tasks/$TaskID"
        Write-Host "[STATE] task_id=$TaskID status=$($task.data.status) attempts=$($task.data.attempt_count) retry_scheduled=$($task.data.retry_scheduled) error=$($task.data.last_error_code)"
        if (& $Predicate $task) { Write-Host "[PASS] $Description" -ForegroundColor Green; return $task }
        Start-Sleep -Seconds 1
    } while ((Get-Date) -lt $deadline)
    throw "timeout waiting for: $Description"
}
function Require-Disruption([string]$ServiceName) {
    if (-not $ConfirmDisruption) {
        throw "$Scenario temporarily stops $ServiceName. Re-run with -ConfirmDisruption after confirming this is a disposable development environment."
    }
    Write-Warning "$Scenario will temporarily stop $ServiceName and restore it in a finally block."
}
function Docker([string[]]$Arguments) {
    & docker @Arguments
    if ($LASTEXITCODE -ne 0) { throw "docker $($Arguments -join ' ') failed" }
}

try {
    [void](API GET "/health")
    switch ($Scenario) {
        "invalid_json" {
            Write-Host "[PRECONDITION] Restart the app with llm.provider=mock and llm.mock_mode=invalid_json."
            Assert-NoCacheKey
            $taskID = Submit-NewTask
            $task = Poll-Until $taskID { param($value) $value.data.status -in @("succeeded", "failed") } "task reached a terminal state"
            Assert ($task.data.status -eq "succeeded") "invalid JSON produces a succeeded fallback task"
            Assert ($task.data.degraded -eq $true -and $task.data.result.degraded -eq $true) "fallback has degraded=true"
            Assert ($null -eq $task.data.report_id -or [int]$task.data.report_id -eq 0) "fallback report_id is null or 0"
            Assert-NoCacheKey
        }
        "llm_timeout" {
            Write-Host "[PRECONDITION] Restart the app with llm.provider=mock and llm.mock_delay_ms greater than llm.timeout_seconds * 1000."
            Assert-NoCacheKey
            $taskID = Submit-NewTask
            $task = Poll-Until $taskID { param($value) $value.data.status -in @("succeeded", "failed") } "task reached a terminal state"
            Assert ($task.data.status -eq "succeeded") "LLM timeout produces a succeeded fallback task"
            Assert ($task.data.result.degraded -eq $true) "timeout fallback has degraded=true"
            Assert ($task.data.result.degraded_reason -eq "llm_timeout") "degraded_reason=llm_timeout"
            Assert ($null -eq $task.data.report_id -or [int]$task.data.report_id -eq 0) "fallback report_id is null or 0"
            Assert-NoCacheKey
        }
        "qdrant_unavailable" {
            Require-Disruption "Qdrant"
            Assert-NoCacheKey
            try {
                Docker -Arguments @("stop", $QdrantContainer)
                $taskID = Submit-NewTask
                $retry = Poll-Until $taskID { param($value) $value.data.status -eq "pending" -and $value.data.retry_scheduled -eq $true } "Qdrant failure scheduled a retry"
                Assert ($null -ne $retry.data.next_run_at) "retry has next_run_at"
                Assert ($retry.data.last_error_code -eq "qdrant_unavailable") "retry error_code=qdrant_unavailable"
                Docker -Arguments @("start", $QdrantContainer)
                $terminal = Poll-Until $taskID { param($value) $value.data.status -in @("succeeded", "failed") } "restored task reached a terminal state"
                Assert ($terminal.data.status -eq "succeeded") "task succeeds after Qdrant recovery"
            } finally {
                Docker -Arguments @("start", $QdrantContainer)
            }
        }
        "redis_fail_open" {
            Require-Disruption "Redis"
            if ([string]::IsNullOrWhiteSpace($ApplicationLogPath) -or -not (Test-Path $ApplicationLogPath)) {
                throw "redis_fail_open requires -ApplicationLogPath pointing to captured Zap application logs"
            }
            try {
                Docker -Arguments @("stop", $RedisContainer)
                $response = API POST "/projects/$ProjectID/diffs/$DiffID/analyze?top_k=$TopK"
                Assert ($response.code -eq 0) "Redis failure does not directly block analyze when fail_open=true"
                Start-Sleep -Milliseconds 500
                $tail = Get-Content -Path $ApplicationLogPath -Tail 1000 | Out-String
                Assert ($tail -match 'rate_limit_redis_error' -and $tail -match 'fail_open') "Zap log contains rate_limit_redis_error with fail_open"
            } finally {
                Docker -Arguments @("start", $RedisContainer)
            }
        }
        "retry_exhausted" {
            Require-Disruption "Qdrant"
            Assert-NoCacheKey
            try {
                Docker -Arguments @("stop", $QdrantContainer)
                $taskID = Submit-NewTask
                $terminal = Poll-Until $taskID { param($value) $value.data.status -eq "failed" } "task exhausted its retry budget"
                Assert ([int]$terminal.data.attempt_count -eq [int]$terminal.data.max_attempts) "attempt_count reaches max_attempts"
                Assert ($terminal.data.last_error_code -eq "retry_exhausted") "final error_code=retry_exhausted"
            } finally {
                Docker -Arguments @("start", $QdrantContainer)
            }
        }
    }
    Write-Host "[PASS] fault scenario '$Scenario' completed" -ForegroundColor Green
    exit 0
} catch {
    Write-Host "[FAIL] $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}
