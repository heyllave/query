.PHONY: build clean setup

GOROOT := $(shell go env GOROOT)

# The Dart web backend and the npm package both load query.wasm + wasm_exec.js
# from the npm client directory, so emit them there.
NPM := ../clients/npm

build:
	GOOS=js GOARCH=wasm go build -o $(NPM)/query.wasm .
	cp "$(GOROOT)/lib/wasm/wasm_exec.js" $(NPM)/src/wasm_exec.js

setup: build
	cd $(NPM) && npm install

clean:
	rm -f $(NPM)/query.wasm $(NPM)/src/wasm_exec.js
	rm -rf $(NPM)/dist $(NPM)/node_modules
