//go:build wasm

// Package main is the WASM entry point for the query library.
// It exposes parse, validate, and stringify functions to JavaScript
// via syscall/js.
//
// Build:
//
//	GOOS=js GOARCH=wasm go build -o query.wasm .
//
// Load in JavaScript/TypeScript:
//
//	const go = new Go();
//	const result = await WebAssembly.instantiate(wasmBytes, go.importObject);
//	go.run(result.instance);
//
//	const ast = queryParse("state=draft AND total>50000");
//	const errors = queryValidate(ast, fieldsJSON);
//	const str = queryStringify(ast);
package main

import (
	"syscall/js"
)

func main() {
	js.Global().Set("queryParse", js.FuncOf(jsParse))
	js.Global().Set("queryValidate", js.FuncOf(jsValidate))
	js.Global().Set("queryStringify", js.FuncOf(jsStringify))
	js.Global().Set("queryParseAndValidate", js.FuncOf(jsParseAndValidate))
	js.Global().Set("queryTokens", js.FuncOf(jsTokens))

	// Block forever so the WASM module stays alive
	select {}
}
