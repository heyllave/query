.PHONY: build test coverage lint fmt vet check clean wasm fuzz examples sqltest

GO       := go
GOTEST   := $(GO) test
GOFLAGS  := -race
COVERAGE := coverage.out

build:
	$(GO) build ./...

test:
	$(GOTEST) $(GOFLAGS) -count=1 ./...

COVERPKG := ./token/...,./ast/...,./parser/...,./validate/...,./eval/...,./

coverage:
	$(GOTEST) $(GOFLAGS) -coverprofile=$(COVERAGE) -covermode=atomic \
		-coverpkg=$(COVERPKG) \
		./token/... ./ast/... ./parser/... ./validate/... ./eval/... .
	$(GO) tool cover -func=$(COVERAGE) | tail -1

lint:
	golangci-lint run ./token/... ./ast/... ./parser/... ./validate/... ./eval/... ./examples/... .

fmt:
	gofmt -w .
	goimports -w -local github.com/heyllave/query .

vet:
	$(GO) vet ./...

check: fmt vet lint test

clean:
	rm -rf bin/ coverage.out coverage.html wasm/*.wasm

wasm:
	cd wasm && $(MAKE) build

# Fuzz the parser, the value grammar and the evaluator for FUZZTIME each
# (default 30s). Seeds run as ordinary tests under `make test`.
FUZZTIME ?= 30s
fuzz:
	$(GOTEST) -run=^$$ -fuzz=^FuzzParse$$ -fuzztime=$(FUZZTIME) ./parser/
	$(GOTEST) -run=^$$ -fuzz=^FuzzParseValue$$ -fuzztime=$(FUZZTIME) ./parser/
	$(GOTEST) -run=^$$ -fuzz=^FuzzCompileMatch$$ -fuzztime=$(FUZZTIME) ./eval/
	$(GOTEST) -run=^$$ -fuzz=^FuzzCompileValue$$ -fuzztime=$(FUZZTIME) ./eval/

# Run every example program the way the README shows.
examples:
	$(GOTEST) -count=1 ./examples/

# Differential tests: every case in conformance/sql/testdata through the evaluator
# and through a real SQLite database (separate module, pure-Go driver).
sqltest:
	cd conformance/sql && $(GOTEST) $(GOFLAGS) -count=1 ./...
