// Package hintwords holds the vocabulary the parser's and the validator's
// hints share: words borrowed from SQL, spelled-out duration units, and what
// counts as a plain number. Both packages meet the same mistakes from
// different sides (name LIKE x fails to parse, name IS NULL parses and fails
// validation), so the words live once, here.
package hintwords

import "strings"

// NotSQL is the hint for a query written as an SQL statement.
const NotSQL = "only conditions are allowed here, such as field=value; " +
	"selecting, sorting and limiting are not part of a query"

// Missing is the hint for IS NULL and IS NOT NULL.
const Missing = "to test for a missing value write NOT field; for a present one write the field alone"

var sql = map[string]string{
	"LIKE":     "there is no LIKE; use = with * as the wildcard: name=john*",
	"ILIKE":    "there is no ILIKE; = already ignores case, and * is the wildcard: name=john*",
	"BETWEEN":  "a range is written with a colon and two dots: total:1..5",
	"IS":       Missing,
	"NULL":     Missing,
	"CONTAINS": `to test for part of a text use a wildcard, name=*john*, or contains(name, "john")`,
	"NOT":      "NOT goes before a condition: NOT state=draft; to compare, use state!=draft",
	"SELECT":   NotSQL,
	"FROM":     NotSQL,
	"WHERE":    NotSQL,
	"ORDER":    NotSQL,
	"BY":       NotSQL,
	"GROUP":    NotSQL,
	"LIMIT":    NotSQL,
	"OFFSET":   NotSQL,
}

// SQL returns what to write instead of word, when word is borrowed from SQL.
// The match ignores case. NOT is in the table for the validator, which meets
// it as a stray field name (state NOT=draft); the parser, for which NOT is a
// keyword, does not look it up.
func SQL(word string) (hint string, ok bool) {
	hint, ok = sql[strings.ToUpper(word)]
	return hint, ok
}

// IsMissingTest reports whether word is IS or NULL, the two words of an SQL
// missing-value test.
func IsMissingTest(word string) bool {
	return strings.EqualFold(word, "IS") || strings.EqualFold(word, "NULL")
}

var units = map[string]string{
	"minute": "m", "minutes": "m", "min": "m", "mins": "m",
	"hour": "h", "hours": "h", "hr": "h", "hrs": "h",
	"day": "d", "days": "d",
	"week": "w", "weeks": "w",
}

// DurationUnit returns the one-letter unit a spelled-out one abbreviates to:
// "d" for days. The match ignores case.
func DurationUnit(word string) (unit string, ok bool) {
	unit, ok = units[strings.ToLower(word)]
	return unit, ok
}

// IsNumber reports whether s is written as a plain unsigned number: digits
// with at most dots between them, such as 42 or 1.5.
func IsNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && r != '.' {
			return false
		}
	}
	return true
}
