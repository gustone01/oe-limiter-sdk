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

# 拉取依赖
deps:
	go mod tidy
	cd examples/event_client && go mod tidy
