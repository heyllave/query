.PHONY: build test coverage lint fmt vet check clean wasm

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
