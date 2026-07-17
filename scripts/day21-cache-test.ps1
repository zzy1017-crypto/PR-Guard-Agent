[CmdletBinding()]
param(
    [int]$ProjectID = 6,
    [int]$DiffID = 5,
    [int]$TopK = 5,
    [int]$AlternateTopK = 8,
    [string]$BaseURL = "http://localhost:8080",
    [string]$RedisContainer = "prguard-redis",
    [switch]$DangerousFlushDB
)

$ErrorActionPreference = "Stop"
$BaseURL = $BaseURL.TrimEnd("/")
$RedisDB = 0

function Write-Pass([string]$Message) { Write-Host "[PASS] $Message" -ForegroundColor Green }
function Fail([string]$Message) { throw $Message }
function Redis([string[]]$Arguments) {
    $output = & docker exec $RedisContainer redis-cli -n $RedisDB @Arguments
    if ($LASTEXITCODE -ne 0) { Fail "redis-cli failed: $($Arguments -join ' ')" }
    return @($output)
}
function Scan-Keys([int]$ForTopK) {
    return @(Redis -Arguments @("--scan", "--pattern", "prguard:report:${ProjectID}:*:topk:$ForTopK") | Where-Object { $_ -and $_ -notmatch '^Warning:' })
}
function Remove-TestKeys([int]$ForTopK) {
    $keys = @(Scan-Keys $ForTopK)
    foreach ($key in $keys) {
        [void](Redis -Arguments @("DEL", $key))
        Write-Host "[INFO] deleted test cache key: $key"
    }
}
function Analyze([int]$ForTopK) {
    return Invoke-RestMethod -Method POST -Uri "$BaseURL/projects/$ProjectID/diffs/$DiffID/analyze?top_k=$ForTopK" -ContentType "application/json"
}
function Get-SingleKey([int]$ForTopK) {
    $keys = @(Scan-Keys $ForTopK)
    if ($keys.Count -ne 1) { Fail "expected exactly one Redis key for project=$ProjectID top_k=$ForTopK, got $($keys.Count): $($keys -join ', ')" }
    return [string]$keys[0]
}

try {
    if ($TopK -eq $AlternateTopK) { Fail "AlternateTopK must differ from TopK" }
    [void](Redis -Arguments @("PING"))
    Write-Pass "connected to Redis DB 0 in $RedisContainer"

    if ($DangerousFlushDB) {
        Write-Warning "DANGEROUS: -DangerousFlushDB explicitly requested; all keys in Redis DB 0 will be deleted."
        $confirmation = Read-Host "Type FLUSHDB to continue"
        if ($confirmation -cne "FLUSHDB") { Fail "FLUSHDB confirmation not provided" }
        [void](Redis -Arguments @("FLUSHDB"))
        Write-Pass "Redis DB 0 flushed by explicit request"
    } else {
        Remove-TestKeys $TopK
        Remove-TestKeys $AlternateTopK
        Write-Pass "removed only matching project/top_k report cache keys"
    }

    $first = Analyze $TopK
    if ($first.data.cached -ne $false) { Fail "first analyze must return cached=false" }
    Write-Pass "first analyze cached=false"

    $keyTopK = Get-SingleKey $TopK
    $ttlTopKOutput = @(Redis -Arguments @("TTL", $keyTopK))
    $ttlTopK = [int]([string]$ttlTopKOutput[0])
    if ($ttlTopK -le 0) { Fail "TTL must be greater than 0 for $keyTopK; got $ttlTopK" }
    $cachedJSON = (Redis -Arguments @("GET", $keyTopK)) -join "`n"
    $cachedObject = $cachedJSON | ConvertFrom-Json
    Write-Host "[INFO] Redis key=$keyTopK TTL=$ttlTopK cached_field=$($cachedObject.cached)"
    Write-Pass "cached=false inside stored JSON is valid and TTL is positive"

    $second = Analyze $TopK
    if ($second.data.cached -ne $true) { Fail "second analyze must return cached=true" }
    Write-Pass "second analyze cached=true"

    $alternate = Analyze $AlternateTopK
    if ($alternate.data.cached -ne $false) { Fail "first alternate top_k analyze must return cached=false" }
    $keyAlternate = Get-SingleKey $AlternateTopK
    $ttlAlternateOutput = @(Redis -Arguments @("TTL", $keyAlternate))
    $ttlAlternate = [int]([string]$ttlAlternateOutput[0])
    Write-Host "[INFO] Redis key=$keyAlternate TTL=$ttlAlternate"
    if ($ttlAlternate -le 0) { Fail "alternate key TTL must be greater than 0" }
    if ($keyTopK -eq $keyAlternate) { Fail "top_k values unexpectedly share one Redis key" }
    Write-Pass "top_k=$TopK and top_k=$AlternateTopK use different Redis keys"
    exit 0
} catch {
    Write-Host "[FAIL] $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}
