VERSION := $(shell git describe --tags --always --dirty="_dev")
LDFLAGS := -ldflags='-X "main.version=$(VERSION)"'
SUBDIRS=$(shell find . -type d -not -path '*/.*' -mindepth 1 -maxdepth 1)

GOTESTFLAGS = -race
GOTAGS = testing

GO ?= $(shell which go)

export GOEXPERIMENT=nocoverageredesign

.PHONY: test
test:
	@for dir in $(SUBDIRS); do \
		cd $$dir && \
		$(GO) test -vet=off -tags='$(GOTAGS)' $(GOTESTFLAGS) -coverpkg="./..." -coverprofile=.coverprofile ./... && \
		grep -v 'cmd' < .coverprofile > .covprof && mv .covprof .coverprofile && \
		$(GO) tool cover -func=.coverprofile && \
		cd .. ; \
	done

.PHONY: coverage
coverage:
	$(GO) tool cover -html=.coverprofile

.PHONY: version
version:
	@echo $(VERSION)

# golangci-lint doesn't transparantly support multiple directories
# instead we get:
#
#     ERRO Running error: context loading failed: no go files to analyze
#
.PHONY: lint
lint: $(GOPATH)/bin/golangci-lint
	@for dir in $(SUBDIRS); do \
		cd $$dir && \
		golangci-lint run --timeout 5m . && \
		cd .. ; \
	done

$(GOPATH)/bin/golangci-lint:
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.51.2

$(GOPATH)/bin/golines:
	$(GO) install github.com/segmentio/golines@latest

.PHONY: fmtcheck
fmtcheck: $(GOPATH)/bin/golines
	exit $(shell golines -m 128 -l . | wc -l)

.PHONY: fmtfix
fmtfix: $(GOPATH)/bin/golines
	golines -m 128 -w .
