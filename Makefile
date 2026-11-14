.PHONY: build test coverage lint fmt vet check clean wasm fuzz examples sqltest clib golden

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
	golangci-lint run ./cmd/... ./token/... ./ast/... ./parser/... ./validate/... ./eval/... ./examples/... ./internal/... ./conformance/... .

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

# Build the C shared library and run the shared corpus through it from Python.
clib:
	$(MAKE) -C clients/ffi test

# Rewrite the golden file after an intended change to the grammar, the printed
# form, the JSON tree or an error code. Review the diff: each changed line is a
# query that now means something else.
golden:
	$(GOTEST) -count=1 -run TestGolden ./internal/bridgejson -update
