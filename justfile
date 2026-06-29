set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

default: verify

help:
    @just --list

verify:
    go vet ./...
    go test -race ./...

build:
    go build -o bin/daigate ./cmd/daigate