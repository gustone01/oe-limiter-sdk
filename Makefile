.PHONY: tidy build test example deps

tidy:
	go mod tidy
	cd examples/event_client && go mod tidy

build:
	go build ./...

test:
	go test ./... -v -count=1
	cd examples/event_client && go test ./... -v -count=1

example:
	cd examples/event_client && go run .

# 首次克隆后配置私有模块（Windows PowerShell 见 scripts/setup.ps1）
deps:
	@echo "go env -w GOPRIVATE=192.168.10.236"
	@echo "git config --global url.\"http://192.168.10.236:3000/\".insteadOf \"https://192.168.10.236/\""
