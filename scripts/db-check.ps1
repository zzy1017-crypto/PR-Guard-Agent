param(
    [string]$Container = "prguard-mysql",
    [string]$Database = "pr_guard",
    [string]$User = "root",
    [string]$Password = "123456",
    [string]$ProjectId = "<PROJECT_ID>"
)

$mysqlArgs = @("exec", $Container, "mysql", "-u$User", "-p$Password", $Database)

Write-Host "Tables"
docker @mysqlArgs -e "SHOW TABLES;"
Write-Host ""

Write-Host "Recent projects"
docker @mysqlArgs -e "SELECT id, name, code_version_hash, create_at FROM projects ORDER BY id DESC LIMIT 5;"
Write-Host ""

Write-Host "Overall counts"
docker @mysqlArgs -e "SELECT COUNT(*) AS project_count FROM projects; SELECT COUNT(*) AS diff_record_count FROM diff_records;"
Write-Host ""

if ($ProjectId -ne "<PROJECT_ID>") {
    Write-Host "Counts for project $ProjectId"
    docker @mysqlArgs -e "SELECT COUNT(*) AS file_count FROM project_files WHERE project_id=$ProjectId; SELECT COUNT(*) AS chunk_count FROM code_chunks WHERE project_id=$ProjectId; SELECT id, project_id, diff_hash, create_at FROM diff_records WHERE project_id=$ProjectId ORDER BY id DESC LIMIT 5;"
} else {
    Write-Host "Skip project-specific checks: pass -ProjectId with a real project id."
}
