package output

import (
	"fmt"
	"io"

	"github.com/heyllave/query/ast"
)

// errUnformattable reports that a formatter was handed an expression it does not
// cover. [ast.Visit] returns a nil node for such a root — today only the
// value-domain [ast.ValueExpr], which the predicate formatters do not render.
// Without this guard the JSON formatter would encode a bare "null" and the tree
// formatter would dereference the nil root and panic. A formatter calls this
// when Visit yields nil for a non-nil expression.
func errUnformattable(expr ast.Expression) error {
	return fmt.Errorf("output: cannot format %T; the formatters cover predicate expressions, not the value domain — evaluate value expressions with the eval package", expr)
}

// Formatter converts an AST expression into formatted output.
//
// Built-in implementations ([JSONOutput], [TreeOutput]) use [ast.Visitor]
// internally. Custom implementations can use any approach.
type Formatter interface {
	Format(w io.Writer, expr ast.Expression, opts Options) error
}

// Options configures output behavior.
type Options struct {
	Positions bool // include source position spans
}

// Option configures formatting behavior.
type Option func(*Options)

// WithPositions includes source position spans in the output.
func WithPositions() Option {
	return func(o *Options) { o.Positions = true }
}

func buildOpts(opts []Option) Options {
	var o Options
	for _, fn := range opts {
		fn(&o)
	}
	return o
}

// Predefined formatters.
//
// This JSON is for humans — debugging and tooling (CamelCase node kinds,
// snake_case fields, positions). It is not the client wire format: the
// cross-language clients speak the separate, versioned contract in
// internal/bridgejson (lowercase kinds, camelCase fields, derived field lists).
// Keep the two apart on purpose; changing this view does not change the client
// contract and vice versa.
var (
	// JSONOutput formats the AST as indented JSON.
	JSONOutput Formatter = &jsonFormatter{}

	// TreeOutput formats the AST as a tree with box-drawing characters.
	TreeOutput Formatter = &treeFormatter{}
)

// Format writes the AST expression to w using the given formatter.
func Format(w io.Writer, expr ast.Expression, f Formatter, opts ...Option) error {
	return f.Format(w, expr, buildOpts(opts))
}

// AsJSON formats the AST as indented JSON bytes.
func AsJSON(expr ast.Expression, opts ...Option) ([]byte, error) {
	return asBytes(JSONOutput, expr, opts...)
}

// AsTree formats the AST as a tree with box-drawing characters.
func AsTree(expr ast.Expression, opts ...Option) ([]byte, error) {
	return asBytes(TreeOutput, expr, opts...)
}

func asBytes(f Formatter, expr ast.Expression, opts ...Option) ([]byte, error) {
	var buf bytesWriter
	if err := f.Format(&buf, expr, buildOpts(opts)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// bytesWriter is a minimal io.Writer that collects bytes.
type bytesWriter struct {
	data []byte
}

func (b *bytesWriter) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *bytesWriter) Bytes() []byte {
	return b.data
}
