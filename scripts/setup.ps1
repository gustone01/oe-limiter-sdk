# oe-limiter-sdk 开发环境初始化（Windows PowerShell）
# 用法: .\scripts\setup.ps1

Write-Host "配置 Go 私有模块..."
go env -w GOPRIVATE=192.168.10.236
go env -w GONOSUMDB=192.168.10.236

Write-Host "配置 Git 地址映射 (https -> Gitea:3000)..."
git config --global url."http://192.168.10.236:3000/".insteadOf "https://192.168.10.236/"

Set-Location $PSScriptRoot\..
Write-Host "go mod tidy..."
go mod tidy
go build ./...
go test ./... -count=1

Write-Host "完成。模块路径: 192.168.10.236/gustone/oe-limiter-sdk"
Write-Host "数据库: mysql < schema.sql"
Write-Host "示例: 设置 .env 后 cd examples/event_client && go run ."
