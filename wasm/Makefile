.PHONY: build clean setup

GOROOT := $(shell go env GOROOT)

# The Dart web backend and the npm package both load query.wasm + wasm_exec.js
# from the npm client directory, so emit them there.
NPM := ../clients/npm

# -trimpath and -s -w keep build paths and debug tables out of the binary, so
# the same source and Go version produce the same bytes on any machine — which
# is what lets a consumer that vendors query.wasm check it against a rebuild.
# `install` rather than `cp`: a Go installed read-only (Nix) hands out a
# read-only wasm_exec.js that a second `cp` cannot overwrite.
build:
	GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o $(NPM)/query.wasm .
	install -m 0644 "$(GOROOT)/lib/wasm/wasm_exec.js" $(NPM)/src/wasm_exec.js

setup: build
	cd $(NPM) && npm install

clean:
	rm -f $(NPM)/query.wasm $(NPM)/src/wasm_exec.js
	rm -rf $(NPM)/dist $(NPM)/node_modules
