package eval

import (
	"reflect"
	"sync"
	"testing"
	"time"
)

type benchInvoice struct {
	State     string    `query:"state"`
	Total     float64   `query:"total"`
	Items     int       `query:"items"`
	Country   string    `query:"country"`
	CreatedAt time.Time `query:"created_at"`
	Active    bool      `query:"active"`
	Internal  string    // untagged
}

// BenchmarkMatchStruct measures the per-record cost of evaluating a compiled
// query against struct instances — the "compile once, match many" hot path.
func BenchmarkMatchStruct(b *testing.B) {
	prog, err := CompileFor[benchInvoice]("state=draft AND total>50000")
	if err != nil {
		b.Fatal(err)
	}
	inv := benchInvoice{State: "draft", Total: 60000, Items: 3, Country: "us", Active: true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = prog.MatchStruct(inv)
	}
}

// TestStructAccessor_TagIndexCached verifies the per-type tag map is built once
// and reused across values of the same type — the same map instance is returned
// each call. Guards against regressing to a rebuild-per-call StructAccessor.
func TestStructAccessor_TagIndexCached(t *testing.T) {
	typ := reflect.TypeOf(benchInvoice{})
	m1 := tagIndex(typ)
	m2 := tagIndex(typ)
	// Same type must return the identical cached map (compare reflect-level
	// pointers via reflect.ValueOf so we test instance identity, not contents).
	if reflect.ValueOf(m1).Pointer() != reflect.ValueOf(m2).Pointer() {
		t.Error("tagIndex rebuilt the map for the same type; expected a cached instance")
	}

	// Accessors over distinct values resolve their own fields, and untagged
	// fields stay invisible.
	accA := StructAccessor(benchInvoice{State: "draft"})
	accB := StructAccessor(benchInvoice{State: "issued"})
	if v, _ := accA("state"); v != "draft" {
		t.Errorf("accA(state) = %v, want draft", v)
	}
	if v, _ := accB("state"); v != "issued" {
		t.Errorf("accB(state) = %v, want issued", v)
	}
	if _, ok := accA("internal"); ok {
		t.Error("untagged field should not resolve")
	}
}

// TestStructAccessor_ConcurrentSafe exercises the cache under concurrent use,
// to be run with -race.
func TestStructAccessor_ConcurrentSafe(t *testing.T) {
	prog, err := CompileFor[benchInvoice]("state=draft AND total>50000")
	if err != nil {
		t.Fatal(err)
	}
	inv := benchInvoice{State: "draft", Total: 60000}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				if !prog.MatchStruct(inv) {
					t.Error("expected match")
					return
				}
			}
		}()
	}
	wg.Wait()
}
