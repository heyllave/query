package query_test

import (
	"fmt"
	"strings"

	"github.com/heyllave/query"
	"github.com/heyllave/query/complete"
	"github.com/heyllave/query/parser"
	"github.com/heyllave/query/validate"
)

func completionFields() []validate.FieldConfig {
	return []validate.FieldConfig{
		{Name: "status", Type: validate.TypeText, AllowedOps: validate.TextOps,
			Values: []string{"ready", "not ready", "failed"}},
		{Name: "cluster", Type: validate.TypeText, AllowedOps: validate.TextOps},
		{Name: "labels", Type: validate.TypeText, AllowedOps: validate.TextOps, Nested: true},
		{Name: "replicas", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
	}
}

// The values a field lists in its config are offered after its operator,
// each written so that it reads back as itself.
func ExampleComplete() {
	text := "status="
	c := query.Complete(text, len(text), completionFields(), query.WithStrict())

	fmt.Println("slot:", c.Slot, "field:", c.Field)
	for _, it := range c.Items {
		fmt.Printf("%-10s -> %s\n", it.Label, it.Text)
	}
	// Output:
	// slot: value field: status
	// ready      -> ready
	// not ready  -> "not ready"
	// failed     -> failed
}

// A query written by accepting one suggestion after another.
func ExampleComplete_typing() {
	fields := completionFields()
	for _, text := range []string{"", "st", "status ", "status=re", "status=ready ", "status=ready AND rep"} {
		c := query.Complete(text, len(text), fields, query.WithStrict())
		var texts []string
		for _, it := range c.Items {
			texts = append(texts, it.Text)
		}
		fmt.Printf("%-22q %-10s %s\n", text, c.Slot, strings.Join(texts, " "))
	}
	// Output:
	// ""                     field      status cluster labels. replicas NOT
	// "st"                   field      status cluster
	// "status "              operator   = != IN ( NOT IN ( AND OR
	// "status=re"            value      ready "not ready"
	// "status=ready "        connective AND OR
	// "status=ready AND rep" field      replicas
}

// Values that live in the host's own data: the completion says what is
// being asked for, the host finds the candidates, and Offer writes them the
// way the grammar needs.
func ExampleComplete_hostValues() {
	clusters := []string{"prod", "staging us", "812"}

	text := "cluster="
	c := query.Complete(text, len(text), completionFields(), query.WithStrict())
	if c.Slot == parser.SlotValue && c.Field == "cluster" {
		for _, it := range c.Offer(clusters...) {
			fmt.Printf("%-10s -> %s\n", it.Label, text[:c.Replace.Offset]+it.Text)
		}
	}

	text = "labels.te"
	c = query.Complete(text, len(text), completionFields(), query.WithStrict())
	fmt.Println("slot:", c.Slot, "field:", c.Field, "prefix:", c.Prefix)
	for _, it := range c.Offer("team", "my key") {
		fmt.Printf("%-10s -> %s\n", it.Label, text[:c.Replace.Offset]+it.Text)
	}
	// Output:
	// prod       -> cluster=prod
	// staging us -> cluster="staging us"
	// 812        -> cluster="812"
	// slot: key field: labels prefix: te
	// team       -> labels.team
	// my key     -> ["labels.my key"]
}

// The whole word under the cursor is replaced, not only the part before it.
func ExampleComplete_replace() {
	text := "stat=x"
	c := query.Complete(text, 3, completionFields())
	accepted := text[:c.Replace.Offset] + c.Items[0].Text + text[c.Replace.Offset+c.Replace.Length:]
	fmt.Println(accepted)
	// Output: status=x
}

func ExampleValueText() {
	for _, v := range []string{"prod", "staging us", "812", "a*b", "2026-01-01"} {
		fmt.Println(complete.ValueText(v))
	}
	// Output:
	// prod
	// "staging us"
	// "812"
	// "a*b"
	// "2026-01-01"
}

func ExampleFieldText() {
	for _, name := range []string{"labels.team", "labels.my key", "not"} {
		fmt.Println(complete.FieldText(name))
	}
	// Output:
	// labels.team
	// ["labels.my key"]
	// ["not"]
}
