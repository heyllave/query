package bridgejson

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"github.com/heyllave/query/complete"
	"github.com/heyllave/query/parser"
	"github.com/heyllave/query/token"
)

// Completion is the JSON form of a [complete.Completion]. It is also what
// [Offer] reads back, so a client hands the object it was given straight to
// the next call.
type Completion struct {
	Slot    string `json:"slot"`
	Replace Span   `json:"replace"`
	Prefix  string `json:"prefix"`
	Items   []Item `json:"items"`
	Field   string `json:"field,omitempty"`
	// Type is the field's type by name, present when the field is declared.
	Type     string   `json:"type,omitempty"`
	Operator string   `json:"operator,omitempty"`
	Function string   `json:"function,omitempty"`
	Lists    []string `json:"lists,omitempty"`
	Quoted   bool     `json:"quoted,omitempty"`
	// Arithmetic marks a value that is an operand of arithmetic, where only
	// a number, a date or a duration can stand.
	Arithmetic bool `json:"arithmetic,omitempty"`
}

// Span is a stretch of the query: where it starts and how long it is, in
// the unit the call was made in.
type Span struct {
	Offset int `json:"offset"`
	Length int `json:"length"`
}

// Item is the JSON form of a [complete.Item].
type Item struct {
	Label  string `json:"label"`
	Text   string `json:"text"`
	Kind   string `json:"kind"`
	Detail string `json:"detail,omitempty"`
}

// Complete reports what can be written at cursor in query. cursor and the
// span it answers with count bytes, or UTF-16 code units when utf16 is set,
// which is how a JavaScript string is indexed.
//
// A cursor that is not a position in the query (it is negative, past the
// end, or inside a character) is an error: unlike a query that is merely
// unfinished, it is a mistake in the caller.
func Complete(query, fieldsJSON string, cursor int, utf16 bool, o Options) Envelope {
	fields, err := ParseFields(fieldsJSON)
	if err != nil {
		return Fail(err.Error())
	}
	if !utf8.ValidString(query) {
		return Fail("query is not valid UTF-8")
	}
	at := cursor
	if utf16 {
		var ok bool
		if at, ok = utf16ToByte(query, cursor); !ok {
			return Fail(fmt.Sprintf("cursor %d is not a position in the query: it is counted in UTF-16 code units, from 0 to %d, and cannot split a character", cursor, UTF16Len(query)))
		}
	} else if at < 0 || at > len(query) || !utf8.ValidString(query[:at]) {
		return Fail(fmt.Sprintf("cursor %d is not a position in the query: it is counted in bytes, from 0 to %d, and cannot split a character", cursor, len(query)))
	}
	c := complete.At(query, at, fields, o)
	return ok(completionToJSON(c, query, utf16))
}

func completionToJSON(c complete.Completion, query string, utf16 bool) Completion {
	out := Completion{
		Slot:       c.Slot.String(),
		Replace:    spanOf(c.Replace, query, utf16),
		Prefix:     c.Prefix,
		Items:      itemsToJSON(c.Items),
		Field:      c.Field,
		Operator:   c.Operator,
		Function:   c.Function,
		Lists:      c.Lists,
		Quoted:     c.Quoted,
		Arithmetic: c.Arithmetic,
	}
	if c.Known {
		out.Type = c.Type.String()
	}
	return out
}

func itemsToJSON(items []complete.Item) []Item {
	out := make([]Item, len(items))
	for i, it := range items {
		out[i] = Item{Label: it.Label, Text: it.Text, Kind: string(it.Kind), Detail: it.Detail}
	}
	return out
}

func spanOf(p token.Position, query string, utf16 bool) Span {
	if !utf16 {
		return Span{Offset: p.Offset, Length: p.Length}
	}
	start := UTF16Len(query[:p.Offset])
	return Span{Offset: start, Length: UTF16Len(query[p.Offset : p.Offset+p.Length])}
}

// Offer writes the host's candidates as items for a completion [Complete]
// returned: completionJSON is that object, candidatesJSON an array of
// strings. See [complete.Completion.Offer].
func Offer(completionJSON, candidatesJSON string) Envelope {
	var in Completion
	if err := json.Unmarshal([]byte(completionJSON), &in); err != nil {
		return Fail("invalid completion: " + err.Error())
	}
	slot, found := slotNamed(in.Slot)
	if !found {
		return Fail(fmt.Sprintf("invalid completion: unknown slot %q", in.Slot))
	}
	c := complete.Completion{Slot: slot, Field: in.Field, Quoted: in.Quoted, Function: in.Function, Arithmetic: in.Arithmetic}
	if in.Type != "" {
		if err := json.Unmarshal([]byte(fmt.Sprintf("%q", in.Type)), &c.Type); err != nil {
			return Fail("invalid completion: " + err.Error())
		}
		c.Known = true
	}
	if slot == parser.SlotKey && in.Field == "" {
		return Fail("invalid completion: a key slot names no field")
	}
	var candidates []string
	if err := json.Unmarshal([]byte(candidatesJSON), &candidates); err != nil || candidates == nil {
		return Fail("candidates must be an array of strings")
	}
	return ok(itemsToJSON(c.Offer(candidates...)))
}

func slotNamed(name string) (parser.Slot, bool) {
	for s := parser.SlotNone; s <= parser.SlotArgument; s++ {
		if s.String() == name {
			return s, true
		}
	}
	return parser.SlotNone, false
}

// UTF16Len is the length of s in UTF-16 code units, the length JavaScript
// reports for the same string.
func UTF16Len(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r >= 0x10000 {
			n++
		}
	}
	return n
}

// utf16ToByte turns an offset in UTF-16 code units into a byte offset. ok is
// false when the offset is outside s or between the two halves of a
// surrogate pair.
func utf16ToByte(s string, units int) (offset int, ok bool) {
	if units < 0 {
		return 0, false
	}
	n := 0
	for i, r := range s {
		if n == units {
			return i, true
		}
		n++
		if r >= 0x10000 {
			n++
		}
		if n > units {
			return 0, false
		}
	}
	return len(s), n == units
}
