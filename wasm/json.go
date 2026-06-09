//go:build wasm

package main

import (
	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/internal/bridgejson"
)

// The JSON contract lives in internal/bridgejson so the WASM and cgo/FFI
// bridges share one source of truth. These thin wrappers keep the local call
// sites unchanged.

func astToJSON(expr ast.Expression) *bridgejson.AST { return bridgejson.AstToJSON(expr) }

func jsonToAST(data string) (ast.Expression, error) { return bridgejson.JSONToAST(data) }
