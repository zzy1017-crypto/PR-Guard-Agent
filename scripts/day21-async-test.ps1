[CmdletBinding()]
param(
    [int]$ProjectID = 6,
    [int]$DiffID = 5,
    [int]$TopK = 5,
    [int]$AlternateTopK = 8,
    [string]$BaseURL = "http://localhost:8080",
    [int]$MockDelayMS = 3000,
    [int]$MaxSubmitMilliseconds = 1000,
    [int]$PollTimeoutSeconds = 120,
    [int]$PollIntervalMilliseconds = 500
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
function Submit([int]$ForTopK) {
    $timer = [System.Diagnostics.Stopwatch]::StartNew()
    $response = API POST "/projects/$ProjectID/diffs/$DiffID/analysis-tasks?top_k=$ForTopK"
    $timer.Stop()
    return @{ Response = $response; ElapsedMS = $timer.ElapsedMilliseconds }
}
function Wait-Terminal([uint64]$TaskID, [ref]$MaxBusy) {
    $deadline = (Get-Date).AddSeconds($PollTimeoutSeconds)
    $lastStatus = ""
    do {
        $task = API GET "/analysis-tasks/$TaskID"
        $status = [string]$task.data.status
        if ($status -ne $lastStatus) {
            Write-Host "[STATE] task_id=$TaskID $lastStatus -> $status; attempt_count=$($task.data.attempt_count)"
            $lastStatus = $status
        }
        $runtime = API GET "/ops/workers"
        $busy = [int]$runtime.data.busy_worker_count
        $configured = [int]$runtime.data.configured_worker_count
        if ($busy -gt $MaxBusy.Value) { $MaxBusy.Value = $busy }
        if ($busy -gt $configured) { throw "busy_worker_count=$busy exceeds worker_count=$configured" }
        if ($status -in @("succeeded", "failed")) { return $task }
        Start-Sleep -Milliseconds $PollIntervalMilliseconds
    } while ((Get-Date) -lt $deadline)
    throw "task $TaskID did not reach a terminal state within $PollTimeoutSeconds seconds"
}

try {
    if ($TopK -eq $AlternateTopK) { throw "AlternateTopK must differ from TopK" }
    Write-Host "[INFO] Configure llm.provider=mock and llm.mock_delay_ms=$MockDelayMS before this test."

    $first = Submit $TopK
    $taskID = [uint64]$first.Response.data.task_id
    Write-Host "[INFO] submit elapsed_ms=$($first.ElapsedMS) task_id=$taskID reused=$($first.Response.data.reused)"
    Assert ($first.ElapsedMS -lt $MaxSubmitMilliseconds) "submit returns before the expected full Mock LLM delay"
    if ($MockDelayMS -gt 0) { Assert ($first.ElapsedMS -lt $MockDelayMS) "submit elapsed time is below mock_delay_ms" }

    $duplicate = Submit $TopK
    Assert ([uint64]$duplicate.Response.data.task_id -eq $taskID) "duplicate submission returns the same task_id"
    Assert ($duplicate.Response.data.reused -eq $true) "duplicate submission returns reused=true"

    $different = Submit $AlternateTopK
    $differentTaskID = [uint64]$different.Response.data.task_id
    Assert ($differentTaskID -ne $taskID) "different top_k maps to a different task_id/task_key"

    $maxBusy = 0
    $terminal = Wait-Terminal $taskID ([ref]$maxBusy)
    Write-Host "[INFO] task_id=$taskID terminal_status=$($terminal.data.status)"
    Assert ($terminal.data.status -eq "succeeded") "first async task succeeds"
    $differentTerminal = Wait-Terminal $differentTaskID ([ref]$maxBusy)
    Write-Host "[INFO] task_id=$differentTaskID terminal_status=$($differentTerminal.data.status)"
    Assert ($differentTerminal.data.status -eq "succeeded") "different top_k async task succeeds"

    $workers = API GET "/ops/workers"
    Assert ([int]$workers.data.registered_worker_count -le [int]$workers.data.configured_worker_count) "registered workers do not exceed configured worker_count"
    Assert ($maxBusy -le [int]$workers.data.configured_worker_count) "observed concurrent running tasks do not exceed worker_count"
    Write-Host "[INFO] maximum observed busy workers=$maxBusy; configured=$($workers.data.configured_worker_count)"
    Write-Host "[PASS] async task verification completed" -ForegroundColor Green
    exit 0
} catch {
    Write-Host "[FAIL] $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}
