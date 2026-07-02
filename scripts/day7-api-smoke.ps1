param(
    [string]$BaseUrl = "http://localhost:8080",
    [string]$ProjectName = "<PROJECT_NAME>",
    [string]$ProjectZip = "<GO_PROJECT_ZIP_PATH>",
    [string]$ProjectId = "<PROJECT_ID>",
    [string]$DiffFile = "<DIFF_OR_PATCH_PATH>"
)

Write-Host "GET /health"
curl.exe "$BaseUrl/health"
Write-Host ""

if ($ProjectZip -ne "<GO_PROJECT_ZIP_PATH>" -and (Test-Path $ProjectZip)) {
    Write-Host "POST /projects/upload"
    curl.exe -X POST "$BaseUrl/projects/upload" -F "project_name=$ProjectName" -F "file=@$ProjectZip"
    Write-Host ""
} else {
    Write-Host "Skip upload: pass -ProjectZip with a real .zip path."
    Write-Host ""
}

if ($ProjectId -ne "<PROJECT_ID>") {
    Write-Host "POST /projects/$ProjectId/chunks/ast"
    curl.exe -X POST "$BaseUrl/projects/$ProjectId/chunks/ast"
    Write-Host ""
} else {
    Write-Host "Skip AST chunks: pass -ProjectId after upload."
    Write-Host ""
}

if ($ProjectId -ne "<PROJECT_ID>" -and $DiffFile -ne "<DIFF_OR_PATCH_PATH>" -and (Test-Path $DiffFile)) {
    Write-Host "POST /projects/$ProjectId/diffs"
    curl.exe -X POST "$BaseUrl/projects/$ProjectId/diffs" -F "file=@$DiffFile"
    Write-Host ""
} else {
    Write-Host "Skip diff upload: pass -ProjectId and -DiffFile with real values."
}
