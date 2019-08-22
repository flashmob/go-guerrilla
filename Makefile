GIT ?= git
GO_VARS ?=
GO ?= go
COMMIT := $(shell $(GIT) rev-parse HEAD)
VERSION ?= $(shell $(GIT) describe --tags ${COMMIT} 2> /dev/null || echo "$(COMMIT)")
BUILD_TIME := $(shell LANG=en_US date +"%F_%T_%z")
ROOT := github.com/flashmob/go-guerrilla
LD_FLAGS := -X $(ROOT).Version=$(VERSION) -X $(ROOT).Commit=$(COMMIT) -X $(ROOT).BuildTime=$(BUILD_TIME)

.PHONY: help clean dependencies test
help:
	@echo "Please use \`make <ROOT>' where <ROOT> is one of"
	@echo "  guerrillad   to build the main binary for current platform"
	@echo "  test         to run unittests"

clean:
	rm -f guerrillad

vendor:
	dep ensure

guerrillad:
	$(GO_VARS) $(GO) build -o="guerrillad" -ldflags="$(LD_FLAGS)" $(ROOT)/cmd/guerrillad

guerrilladrace:
	$(GO_VARS) $(GO) build -o="guerrillad" -race -ldflags="$(LD_FLAGS)" $(ROOT)/cmd/guerrillad

test:
	$(GO_VARS) $(GO) test -v .
	$(GO_VARS) $(GO) test -v ./tests
	$(GO_VARS) $(GO) test -v ./cmd/guerrillad
	$(GO_VARS) $(GO) test -v ./response
	$(GO_VARS) $(GO) test -v ./backends
	$(GO_VARS) $(GO) test -v ./mail

testrace:
	$(GO_VARS) $(GO) test -v . -race
	$(GO_VARS) $(GO) test -v ./tests -race
	$(GO_VARS) $(GO) test -v ./cmd/guerrillad -race
	$(GO_VARS) $(GO) test -v ./response -race
	$(GO_VARS) $(GO) test -v ./backends -race