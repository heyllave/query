package eval

import (
	"testing"
	"time"

	"github.com/heyllave/query/validate"
)

func TestProgram_AST(t *testing.T) {
	prog, err := Compile("state=draft", testFields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if prog.AST() == nil {
		t.Error("expected non-nil AST")
	}
}

func TestParseOps(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"three ops", "=|!=|>", 3},
		{"single op", "=", 1},
		{"empty", "", 0},
		{"duplicates", "=|=|=", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseOps(tt.input)
			if len(got) != tt.want {
				t.Errorf("got %d ops, want %d", len(got), tt.want)
			}
		})
	}
}

type customTagged struct {
	State string `query:"state,=|!="`
}

func TestFieldsFromStruct(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  int
	}{
		{"custom ops", customTagged{}, 1},
		{"testInvoice value", testInvoice{}, 5},
		{"testInvoice pointer", &testInvoice{}, 5},
		{"non-struct", 42, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FieldsFromStruct(tt.input)
			if len(got) != tt.want {
				t.Errorf("got %d, want %d", len(got), tt.want)
			}
		})
	}
}

type allTypes struct {
	S   string        `query:"s"`
	I   int           `query:"i"`
	I8  int8          `query:"i8"`
	I16 int16         `query:"i16"`
	I32 int32         `query:"i32"`
	I64 int64         `query:"i64"`
	U   uint          `query:"u"`
	U8  uint8         `query:"u8"`
	U16 uint16        `query:"u16"`
	U32 uint32        `query:"u32"`
	U64 uint64        `query:"u64"`
	F32 float32       `query:"f32"`
	F64 float64       `query:"f64"`
	B   bool          `query:"b"`
	T   time.Time     `query:"t"`
	D   time.Duration `query:"d"`
	Ptr *string       `query:"ptr"`
	Any []string      `query:"any"`
}

func TestFieldsFromStruct_AllTypes(t *testing.T) {
	fields := FieldsFromStruct(allTypes{})
	byName := make(map[string]validate.FieldConfig)
	for _, f := range fields {
		byName[f.Name] = f
	}

	tests := []struct {
		name string
		want validate.FieldValueType
	}{
		{"s", validate.TypeText},
		{"i", validate.TypeInteger},
		{"i8", validate.TypeInteger},
		{"u", validate.TypeInteger},
		{"f32", validate.TypeDecimal},
		{"f64", validate.TypeDecimal},
		{"b", validate.TypeBoolean},
		{"t", validate.TypeDate},
		{"d", validate.TypeDuration},
		{"ptr", validate.TypeText},
		{"any", validate.TypeText},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, ok := byName[tt.name]
			if !ok {
				t.Fatal("missing field")
			}
			if f.Type != tt.want {
				t.Errorf("got %v, want %v", f.Type, tt.want)
			}
		})
	}
}

func TestStructAccessor(t *testing.T) {
	inv := &testInvoice{State: "draft"}

	tests := []struct {
		name   string
		input  any
		field  string
		wantOk bool
	}{
		{"non-struct", 42, "anything", false},
		{"pointer struct", inv, "state", true},
		{"struct state", testInvoice{State: "draft"}, "state", true},
		{"unexported field", testInvoice{}, "Internal", false},
		{"nonexistent", testInvoice{}, "nonexistent", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := StructAccessor(tt.input)
			_, ok := get(tt.field)
			if ok != tt.wantOk {
				t.Errorf("ok: got %v, want %v", ok, tt.wantOk)
			}
		})
	}
}

func TestCompileFor_NoTaggedFields(t *testing.T) {
	type empty struct {
		X string
	}
	if _, err := CompileFor[empty]("X=y"); err == nil {
		t.Fatal("expected error")
	}
}

func TestBuiltinFunctions(t *testing.T) {
	fns := BuiltinFunctions()
	d := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		fn   string
		args []any
		want any
	}{
		{"lower", "lower", []any{"HELLO"}, "hello"},
		{"upper", "upper", []any{"hello"}, "HELLO"},
		{"trim", "trim", []any{"  x  "}, "x"},
		{"len", "len", []any{"hello"}, int64(5)},
		{"contains true", "contains", []any{"hello world", "WORLD"}, true},
		{"startsWith true", "startsWith", []any{"hello world", "HELLO"}, true},
		{"endsWith true", "endsWith", []any{"hello world", "WORLD"}, true},
		{"year", "year", []any{d}, int64(2026)},
		{"month", "month", []any{d}, int64(3)},
		{"day", "day", []any{d}, int64(15)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fns[tt.fn].Call(tt.args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}

	// time-returning functions: check type only
	if r, _ := fns["now"].Call(); r == nil {
		t.Error("now returned nil")
	}
	if r, _ := fns["today"].Call(); r == nil {
		t.Error("today returned nil")
	}
	if r, _ := fns["daysAgo"].Call(int64(7)); r == nil {
		t.Error("daysAgo returned nil")
	}
}

func TestBuiltinFunctions_Errors(t *testing.T) {
	fns := BuiltinFunctions()

	tests := []struct {
		name string
		fn   string
		args []any
	}{
		{"lower wrong count", "lower", []any{}},
		{"lower too many", "lower", []any{"a", "b"}},
		{"trim wrong count", "trim", []any{}},
		{"len wrong count", "len", []any{}},
		{"contains missing arg", "contains", []any{"a"}},
		{"startsWith missing", "startsWith", []any{"a"}},
		{"endsWith missing", "endsWith", []any{"a"}},
		{"now with args", "now", []any{"bad"}},
		{"today with args", "today", []any{"bad"}},
		{"year wrong count", "year", []any{}},
		{"month wrong count", "month", []any{}},
		{"day wrong count", "day", []any{}},
		{"daysAgo wrong count", "daysAgo", []any{}},
		{"upper wrong count", "upper", []any{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := fns[tt.fn].Call(tt.args...); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestToInt64(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  int64
	}{
		{"int", int(5), 5},
		{"int32", int32(5), 5},
		{"int64", int64(5), 5},
		{"float32", float32(5.7), 5},
		{"float64", float64(5.7), 5},
		{"string fallback", "bad", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toInt64(tt.input); got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  float64
	}{
		{"int", int(5), 5},
		{"int32", int32(5), 5},
		{"int64", int64(5), 5},
		{"float32", float32(5.5), float64(float32(5.5))},
		{"float64", float64(5.5), 5.5},
		{"string fallback", "bad", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toFloat64(tt.input); got != tt.want {
				t.Errorf("got %f, want %f", got, tt.want)
			}
		})
	}
}

func TestToBool(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  bool
	}{
		{"true", true, true},
		{"false", false, false},
		{"string TRUE", "TRUE", true},
		{"string true", "true", true},
		{"string false", "false", false},
		{"int fallback", 42, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toBool(tt.input); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToTime(t *testing.T) {
	tm := time.Now()

	tests := []struct {
		name    string
		input   any
		wantErr bool
	}{
		{"time.Time", tm, false},
		{"date string", "2026-01-01", false},
		{"RFC3339", "2026-01-01T12:00:00Z", false},
		{"bad string", "bad", true},
		{"int fallback", 42, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toTime(tt.input)
			if tt.wantErr && !got.IsZero() {
				t.Errorf("expected zero time, got %v", got)
			}
			if !tt.wantErr && got.IsZero() {
				t.Errorf("unexpected zero time")
			}
		})
	}
}

func TestToDuration(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  time.Duration
	}{
		{"duration", time.Hour, time.Hour},
		{"string", "1h", time.Hour},
		{"bad string", "bad", 0},
		{"int fallback", 42, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toDuration(tt.input); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompile_AdditionalMatches(t *testing.T) {
	tests := []struct {
		name   string
		query  string
		fields []validate.FieldConfig
		data   map[string]any
		want   bool
	}{
		{
			name:  "float comparison",
			query: "score>3.5",
			fields: []validate.FieldConfig{
				{Name: "score", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
			},
			data: map[string]any{"score": 4.0},
			want: true,
		},
		{
			name:  "string ordering",
			query: "name>alice",
			fields: []validate.FieldConfig{
				{Name: "name", Type: validate.TypeText,
					AllowedOps: []validate.Op{validate.OpEq, validate.OpGt, validate.OpLt}},
			},
			data: map[string]any{"name": "bob"},
			want: true,
		},
		{
			name:  "duration comparison",
			query: "ttl>1h",
			fields: []validate.FieldConfig{
				{Name: "ttl", Type: validate.TypeDuration, AllowedOps: validate.DurationOps},
			},
			data: map[string]any{"ttl": 2 * time.Hour},
			want: true,
		},
		{
			name:  "date equality",
			query: "created_at=2026-01-01",
			fields: []validate.FieldConfig{
				{Name: "created_at", Type: validate.TypeDate, AllowedOps: validate.DateOps},
			},
			data: map[string]any{"created_at": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
			want: true,
		},
		{
			name:  "wildcard field absent",
			query: "name=John*",
			fields: []validate.FieldConfig{
				{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
			},
			data: map[string]any{},
			want: false,
		},
		{
			name:  "range field absent",
			query: "year:2020..2026",
			fields: []validate.FieldConfig{
				{Name: "year", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
			},
			data: map[string]any{},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, err := Compile(tt.query, tt.fields)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := prog.Match(tt.data); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
