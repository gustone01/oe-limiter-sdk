# oe-limiter-sdk 开发环境初始化（Windows PowerShell）
# 用法: .\scripts\setup.ps1

Set-Location $PSScriptRoot\..
Write-Host "go mod tidy..."
go mod tidy
go build ./...
go test ./... -count=1

Write-Host "完成。模块路径: github.com/gustone01/oe-limiter-sdk"
Write-Host "数据库: mysql < sql/schema.sql"
Write-Host "示例: 设置 .env 后 cd examples/event_client && go run ."
