.DEFAULT_GOAL := help
.PHONY: lint fmt test test-integration test-examples clean tidy vuln modernize-check audit test/cover help

## lint: run golangci-lint
lint:
	golangci-lint run --new-from-rev="origin/master" ./...

## fmt: format code
fmt:
	golangci-lint fmt ./...

## test: run tests with race detector and coverage
test:
	go test -race -shuffle=on -coverprofile=coverage.out -covermode=atomic ./...

## test-integration: run integration tests (requires LM_API_KEY; optional LM_MODEL)
test-integration:
	go test -race -tags=integration -timeout=300s -v ./integration/...

## test-examples: run example smoke tests with LM Studio-backed output verification
test-examples:
	./scripts/test_examples.sh

## clean: remove build artifacts
clean:
	rm -f coverage.out

## tidy: tidy go modules
tidy:
	go mod tidy

## vuln: run govulncheck
vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

## modernize-check: preview Go modernizations without changing files
modernize-check:
	go fix -n ./...

## audit: run all checks
audit: lint test vuln modernize-check
	go mod tidy -diff
	go mod verify

## test/cover: open HTML coverage report
test/cover: test
	go tool cover -html=coverage.out

## help: show this help
help:
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'
