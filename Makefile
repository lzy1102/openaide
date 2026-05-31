# OpenAIDE — root Makefile delegates to backend
.PHONY: all build install run test clean fmt lint help

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "latest")

all: build

build:
	@$(MAKE) -C backend build

install:
	@$(MAKE) -C backend install

run:
	@$(MAKE) -C backend run

test:
	@$(MAKE) -C backend test

clean:
	@$(MAKE) -C backend clean

fmt:
	@$(MAKE) -C backend fmt

lint:
	@$(MAKE) -C backend lint

help:
	@echo "OpenAIDE $(VERSION)"
	@echo ""
	@echo "  make build    Build binaries"
	@echo "  make install  Build + install to PATH"
	@echo "  make run      Start API server"
	@echo "  make test     Run all tests"
	@echo "  make clean    Remove build artifacts"
	@echo "  make fmt      Format code"
	@echo "  make lint     Run linter"
