.PHONY: native web-assets test test-web test-all analyze clean

# Build the cgo c-shared library the native FFI backend loads (into native/).
native:
	$(MAKE) -C ../ffi build

# Stage the assets the browser test server serves: the WASM build, Go's JS glue,
# and a copy of the shared corpus. test/ is where package:test resolves the
# relative URLs the browser test fetches.
web-assets:
	$(MAKE) -C ../../wasm build
	cp ../npm/query.wasm test/query.wasm
	cp ../npm/src/wasm_exec.js test/wasm_exec.js
	cp ../../conformance/corpus.json test/corpus.json

analyze:
	dart pub get
	dart analyze --fatal-infos

# Native FFI: client + conformance on the Dart VM.
test: native
	dart pub get
	dart test -p vm

# Web WASM: conformance through the dart:js_interop backend in a browser.
test-web: web-assets
	dart pub get
	dart test -p chrome

test-all: native web-assets
	dart pub get
	dart test -p vm,chrome

clean:
	rm -f test/query.wasm test/wasm_exec.js test/corpus.json
	rm -rf native .dart_tool
