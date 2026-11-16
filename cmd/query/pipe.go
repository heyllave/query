package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/eval"
	"github.com/heyllave/query/parser"
	"github.com/heyllave/query/validate"
)

// The filter, eval and map commands read JSON records from stdin, one per
// line or as one top-level array, and write what the query says about each
// of them. They are the jq-shaped way to use the language from a shell: the
// same query string a browser or a server would run, over a stream.

// sampleLimit is how many records are buffered while the fields a query
// names are still unseen. Past it, an unseen field is typed as text.
const sampleLimit = 1000

// pipeOptions are the flags the three commands share.
type pipeOptions struct {
	schemaPath string
	pretty     bool
	raw        bool
}

func pipeFlags(name string, stderr io.Writer, withRaw bool) (*flag.FlagSet, *pipeOptions) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	o := &pipeOptions{}
	fs.StringVar(&o.schemaPath, "schema", "", "path to a JSON schema file; without it, field types are read from the records")
	fs.BoolVar(&o.pretty, "pretty", false, "indent the output")
	if withRaw {
		fs.BoolVar(&o.raw, "raw", false, "write strings without quotes, like jq -r")
		fs.BoolVar(&o.raw, "r", false, "same as --raw")
	}
	return fs, o
}

// runFilter is `query filter "<condition>"`: every record the condition
// matches is written back unchanged.
func runFilter(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs, o := pipeFlags("filter", stderr, false)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() == 0 {
		_, _ = fmt.Fprintln(stderr, "error: missing condition")
		_, _ = fmt.Fprintln(stderr, `usage: query filter "<condition>" [--schema <path>] [--pretty] < records.jsonl`)
		return 1
	}
	q := fs.Arg(0)
	expr, err := parser.Parse(q, 0)
	if err != nil {
		printErrors(q, err, stderr)
		return 1
	}
	return pipe(o, ast.Fields(expr), stdin, stdout, stderr, func(fields []validate.FieldConfig, opts []eval.Option) (applyFn, error) {
		prog, err := eval.Compile(q, fields, opts...)
		if err != nil {
			return nil, err
		}
		return func(rec record) (any, bool, error) {
			// The record's own bytes go back out, so its keys keep their
			// order and its numbers their spelling.
			return rec.raw, prog.Match(rec.data), nil
		}, nil
	})
}

// runEval is `query eval "<value expression>"`: the expression's value for
// each record is written as JSON, or null when it has none.
func runEval(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs, o := pipeFlags("eval", stderr, true)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() == 0 {
		_, _ = fmt.Fprintln(stderr, "error: missing value expression")
		_, _ = fmt.Fprintln(stderr, `usage: query eval "<value expression>" [--schema <path>] [--pretty] [--raw] < records.jsonl`)
		return 1
	}
	q := fs.Arg(0)
	expr, err := parser.ParseValue(q, 0)
	if err != nil {
		printErrors(q, err, stderr)
		return 1
	}
	return pipe(o, ast.Fields(expr), stdin, stdout, stderr, func(fields []validate.FieldConfig, opts []eval.Option) (applyFn, error) {
		prog, err := eval.CompileValue(q, fields, opts...)
		if err != nil {
			return nil, err
		}
		return func(rec record) (any, bool, error) {
			v, err := prog.Eval(rec.data)
			if errors.Is(err, eval.ErrNoValue) {
				return nil, true, nil
			}
			return v, true, err
		}, nil
	})
}

// runMap is `query map "<name=expr, ...>"`: each record becomes an object
// with the named keys, each the value of its expression. A bare field path
// keeps its own name: `id, total=[total]*1.1, team=[labels.team]`.
func runMap(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs, o := pipeFlags("map", stderr, false)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() == 0 {
		_, _ = fmt.Fprintln(stderr, "error: missing projection")
		_, _ = fmt.Fprintln(stderr, `usage: query map "name=<value expression>, ..." [--schema <path>] [--pretty] < records.jsonl`)
		return 1
	}
	entries, err := parseProjection(strings.Join(fs.Args(), ","))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	var names []ast.FieldPath
	exprs := make([]ast.Expression, len(entries))
	for i, e := range entries {
		expr, err := parser.ParseValue(e.expr, 0)
		if err != nil {
			printErrors(e.expr, err, stderr)
			return 1
		}
		exprs[i] = expr
		names = append(names, ast.Fields(expr)...)
	}
	return pipe(o, names, stdin, stdout, stderr, func(fields []validate.FieldConfig, opts []eval.Option) (applyFn, error) {
		progs := make([]*eval.ValueProgram, len(entries))
		for i, e := range entries {
			prog, err := eval.CompileValue(e.expr, fields, opts...)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", e.name, err)
			}
			progs[i] = prog
		}
		return func(rec record) (any, bool, error) {
			out := make(orderedObject, len(entries))
			for i, e := range entries {
				v, err := progs[i].Eval(rec.data)
				if err != nil && !errors.Is(err, eval.ErrNoValue) {
					return nil, false, err
				}
				if err != nil {
					v = nil
				}
				out[i] = member{e.name, v}
			}
			return out, true, nil
		}, nil
	})
}

// projection is one `name=expr` entry of a map.
type projection struct {
	name, expr string
}

// parseProjection splits a projection on its top-level commas. An entry is
// `name=<value expression>`, or a bare field path, which stands for itself:
// `labels.team` is `labels.team=[labels.team]`.
func parseProjection(spec string) ([]projection, error) {
	var out []projection
	for _, part := range splitTopLevel(spec, ',') {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if isFieldPath(part) {
			out = append(out, projection{name: part, expr: "[" + part + "]"})
			continue
		}
		eq := strings.IndexByte(part, '=')
		if eq <= 0 || !isFieldPath(strings.TrimSpace(part[:eq])) {
			return nil, fmt.Errorf("projection entry %q needs a name: name=<value expression>", part)
		}
		name, expr := strings.TrimSpace(part[:eq]), strings.TrimSpace(part[eq+1:])
		if expr == "" {
			return nil, fmt.Errorf("projection entry %q has no expression", part)
		}
		out = append(out, projection{name: name, expr: expr})
	}
	if len(out) == 0 {
		return nil, errors.New("empty projection")
	}
	return out, nil
}

// isFieldPath reports whether s is a bare dotted name: letters, digits, `_`,
// `-` and `.`, starting with a letter or `_`.
func isFieldPath(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
		case i > 0 && (r == '.' || r == '-' || (r >= '0' && r <= '9')):
		default:
			return false
		}
	}
	return true
}

// splitTopLevel splits s on sep outside quotes, parentheses and brackets.
func splitTopLevel(s string, sep byte) []string {
	var parts []string
	depth, start := 0, 0
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inQuote:
			switch c {
			case '\\':
				i++
			case '"':
				inQuote = false
			}
		case c == '"':
			inQuote = true
		case c == '(' || c == '[':
			depth++
		case c == ')' || c == ']':
			depth--
		case c == sep && depth == 0:
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return append(parts, s[start:])
}

// member is one key of an orderedObject.
type member struct {
	key   string
	value any
}

// orderedObject marshals as a JSON object whose keys keep the projection's
// order, which a Go map would not.
type orderedObject []member

func (o orderedObject) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, m := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := json.Marshal(m.key)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(m.value)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// record is one input object: decoded for the evaluator, and as read for
// writing back unchanged.
type record struct {
	data map[string]any
	raw  json.RawMessage
}

// applyFn answers, for one record, the value to write, whether to write it,
// and an error that stops the run.
type applyFn func(record) (any, bool, error)

// compileFn builds the per-record function once the field configs are known.
type compileFn func(fields []validate.FieldConfig, opts []eval.Option) (applyFn, error)

// pipe reads records from stdin and writes what compile's function says
// about each. Without a schema the fields the query names are typed from the
// records: the first records are buffered until every named field has been
// seen with a value, or sampleLimit records have gone by.
func pipe(o *pipeOptions, named []ast.FieldPath, stdin io.Reader, stdout, stderr io.Writer, compile compileFn) int {
	in, err := newRecordReader(stdin)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	out := newRecordWriter(stdout, in.array, o.pretty, o.raw)

	var fields []validate.FieldConfig
	var opts []eval.Option
	sampler := &recordSchema{wanted: named}
	if o.schemaPath != "" {
		fields, err = loadSchema(o.schemaPath)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		sampler = nil
	} else {
		opts = append(opts, eval.WithCustomValidator(sampler))
	}

	var apply applyFn
	var buffered []record
	emit := func(rec record) error {
		v, keep, err := apply(rec)
		if err != nil {
			return err
		}
		if keep {
			return out.write(v)
		}
		return nil
	}
	ready := func() error {
		var err error
		apply, err = compile(fields, opts)
		if err != nil {
			return err
		}
		for _, rec := range buffered {
			if err := emit(rec); err != nil {
				return err
			}
		}
		buffered = nil
		return nil
	}

	for {
		rec, err := in.next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		if apply == nil {
			if sampler != nil && !sampler.saturated() && len(buffered) < sampleLimit {
				sampler.observe(rec.data)
				buffered = append(buffered, rec)
				continue
			}
			if err := ready(); err != nil {
				return reportCompile(err, stderr)
			}
		}
		if err := emit(rec); err != nil {
			_, _ = fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}
	if apply == nil {
		if err := ready(); err != nil {
			return reportCompile(err, stderr)
		}
	}
	if err := out.close(); err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	return 0
}

func reportCompile(err error, stderr io.Writer) int {
	_, _ = fmt.Fprintf(stderr, "error: %s\n", err)
	return 1
}

// recordSchema types the fields a query names from the records it has seen,
// through the validator's field hook. A field never seen is text, so a
// condition on it validates and simply matches nothing, as in jq.
type recordSchema struct {
	wanted  []ast.FieldPath
	samples map[string]any
}

func (s *recordSchema) observe(rec map[string]any) {
	if s.samples == nil {
		s.samples = make(map[string]any)
	}
	for _, fp := range s.wanted {
		name := fp.String()
		if _, ok := s.samples[name]; ok {
			continue
		}
		if v, ok := lookup(rec, name); ok && v != nil {
			s.samples[name] = v
		}
	}
}

// saturated reports whether every named field has a sample.
func (s *recordSchema) saturated() bool {
	for _, fp := range s.wanted {
		if _, ok := s.samples[fp.String()]; !ok {
			return false
		}
	}
	return true
}

// lookup follows a dotted path through nested objects. A path that is not
// found at the top level is looked for inside the elements of list fields,
// which is where a selector's inner condition reads it.
func lookup(rec map[string]any, path string) (any, bool) {
	if v, ok := lookupPath(rec, path); ok {
		return v, true
	}
	for _, v := range rec {
		list, ok := v.([]any)
		if !ok {
			continue
		}
		for _, el := range list {
			if m, ok := el.(map[string]any); ok {
				if v, ok := lookupPath(m, path); ok {
					return v, true
				}
			}
		}
	}
	return nil, false
}

func lookupPath(rec map[string]any, path string) (any, bool) {
	if v, ok := rec[path]; ok {
		return v, true
	}
	head, rest, found := strings.Cut(path, ".")
	if !found {
		return nil, false
	}
	child, ok := rec[head].(map[string]any)
	if !ok {
		return nil, false
	}
	return lookupPath(child, rest)
}

// GetFieldConfig types a field from its sample. A nested object accepts any
// key under it. A field never seen is text that allows every operator, so
// the query validates and the condition matches nothing.
func (s *recordSchema) GetFieldConfig(name string) (validate.FieldConfig, bool) {
	cfg := validate.FieldConfig{Name: name, Type: validate.TypeText}
	sample, seen := s.samples[name]
	switch v := sample.(type) {
	case float64, json.Number:
		cfg.Type = validate.TypeDecimal
	case bool:
		cfg.Type = validate.TypeBoolean
	case string:
		if _, err := parser.ParseDateLiteral(v); err == nil {
			cfg.Type = validate.TypeDatetime
		}
	case map[string]any:
		cfg.Nested = true
	}
	cfg.AllowedOps = append(validate.DefaultOps(cfg.Type), validate.OpPresence)
	if !seen {
		cfg.AllowedOps = allOps
	}
	return cfg, true
}

// allOps is every operator, for a field no record has shown.
var allOps = []validate.Op{
	validate.OpEq, validate.OpNeq, validate.OpGt, validate.OpGte, validate.OpLt, validate.OpLte,
	validate.OpRange, validate.OpWildcard, validate.OpPresence,
}

func (s *recordSchema) ValidateCustomRules(ast.Expression) error { return nil }

// recordReader yields the objects on stdin: one top-level array, or any
// number of objects in sequence (JSON lines).
type recordReader struct {
	dec   *json.Decoder
	array bool
	n     int
}

func newRecordReader(r io.Reader) (*recordReader, error) {
	br := bufio.NewReader(r)
	rr := &recordReader{dec: json.NewDecoder(br)}
	// Skip leading whitespace and peek: an opening bracket means one array
	// of records.
	for {
		b, err := br.ReadByte()
		if errors.Is(err, io.EOF) {
			return rr, nil
		}
		if err != nil {
			return nil, err
		}
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
			continue
		}
		if err := br.UnreadByte(); err != nil {
			return nil, err
		}
		rr.array = b == '['
		break
	}
	if rr.array {
		if _, err := rr.dec.Token(); err != nil { // the '['
			return nil, fmt.Errorf("invalid input: %w", err)
		}
	}
	return rr, nil
}

func (r *recordReader) next() (record, error) {
	if r.array {
		if !r.dec.More() {
			if _, err := r.dec.Token(); err != nil && !errors.Is(err, io.EOF) { // the ']'
				return record{}, fmt.Errorf("invalid input: %w", err)
			}
			return record{}, io.EOF
		}
	}
	var raw json.RawMessage
	if err := r.dec.Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return record{}, io.EOF
		}
		return record{}, fmt.Errorf("record %d: invalid JSON: %w", r.n+1, err)
	}
	r.n++
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil || data == nil {
		return record{}, fmt.Errorf("record %d is not an object", r.n)
	}
	return record{data: data, raw: raw}, nil
}

// recordWriter writes results in the shape the input had: an array stays an
// array, lines stay lines.
type recordWriter struct {
	w      io.Writer
	array  bool
	pretty bool
	raw    bool
	n      int
}

func newRecordWriter(w io.Writer, array, pretty, raw bool) *recordWriter {
	return &recordWriter{w: w, array: array, pretty: pretty, raw: raw}
}

func (w *recordWriter) write(v any) error {
	if w.array && w.n == 0 {
		if _, err := io.WriteString(w.w, "["); err != nil {
			return err
		}
	}
	if w.array && w.n > 0 {
		if _, err := io.WriteString(w.w, ","); err != nil {
			return err
		}
	}
	if w.array && w.pretty {
		if _, err := io.WriteString(w.w, "\n  "); err != nil {
			return err
		}
	}
	w.n++
	if s, ok := v.(string); ok && w.raw {
		_, err := io.WriteString(w.w, s+"\n")
		return err
	}
	indent := ""
	if w.array {
		indent = "  "
	}
	var data []byte
	var err error
	switch {
	case w.pretty:
		data, err = json.MarshalIndent(v, indent, "  ")
	default:
		data, err = json.Marshal(v)
		if raw, ok := v.(json.RawMessage); ok && err == nil {
			var b bytes.Buffer
			err = json.Compact(&b, raw)
			data = b.Bytes()
		}
	}
	if err != nil {
		return err
	}
	if !w.array {
		data = append(data, '\n')
	}
	_, err = w.w.Write(data)
	return err
}

func (w *recordWriter) close() error {
	if !w.array {
		return nil
	}
	if w.n == 0 {
		_, err := io.WriteString(w.w, "[]\n")
		return err
	}
	if w.pretty {
		_, err := io.WriteString(w.w, "\n]\n")
		return err
	}
	_, err := io.WriteString(w.w, "]\n")
	return err
}
