
# Commands for invdex
default:
  @just --list
# Build invdex with Go
build:
  go build ./...

# Run tests for invdex with Go
test:
  go clean -testcache
  go test ./...