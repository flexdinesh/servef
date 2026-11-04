# Makefile
GO ?= go
SRC := $(wildcard *.go)

.PHONY: build test
build: $(SRC)
	$(GO) build -o bin/app ./...

test:
	@echo "testing $@"
	$(GO) test ./... -count=1
