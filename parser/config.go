package parser

// Config selects the grammar a query is parsed under. The zero value is the
// default grammar, the one [Parse] uses.
//
// The default grammar is lenient where a person typing into a search box
// benefits from it. A host that takes queries from scripts, saved links or
// other programs can turn the lenient forms off one by one: each strict
// option only refuses queries, so every query a strict host accepts means
// the same thing under the default grammar.
type Config struct {
	// MaxLength is the longest query accepted, in characters. 0 disables the
	// check; nesting stays bounded by [MaxNesting] either way.
	MaxLength int

	// ExplicitAnd requires AND or OR between conditions. By default a space
	// between two conditions is an implicit AND (`state=draft total>5`), which
	// also means a mistyped query can split into conditions nobody wrote:
	// `labels[team]=x` reads as `labels AND [team]=x`. With ExplicitAnd both
	// are refused with [CodeMissingAnd].
	ExplicitAnd bool

	// StrictValues refuses `=`, `<` and `>` inside an unquoted value. By
	// default `a=1+b=2` compares a with the text "1+b=2"; with StrictValues
	// it is an error, and a value that really holds one of those characters
	// is written in quotes (`url="https://x/?a=b"`).
	StrictValues bool
}
