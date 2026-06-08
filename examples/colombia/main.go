// Command colombia wires an up-to-date Colombian public-holiday calendar into
// the query language's business-day functions.
//
// The calendar is computed from each year's rules — fixed dates, the Ley
// Emiliani Monday shift, and Easter-relative dates — rather than stored as a
// table, so it is correct for any year, past or future, and never goes stale.
//
// Run with:
//
//	go run .
package main

import (
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/heyllave/query/eval"
	"github.com/heyllave/query/validate"
)

// easter returns Western (Gregorian) Easter Sunday for the year, via the
// Anonymous Gregorian algorithm (the Computus). Several Colombian holidays are
// defined relative to it.
func easter(year int) time.Time {
	a := year % 19
	b := year / 100
	c := year % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := ((h + l - 7*m + 114) % 31) + 1
	return date(year, time.Month(month), day)
}

// nextMonday moves t to the following Monday, or returns it unchanged if it is
// already a Monday. This is Colombia's Ley Emiliani ("puente festivo"), which
// shifts many holidays to the nearest following Monday for a long weekend.
func nextMonday(t time.Time) time.Time {
	for t.Weekday() != time.Monday {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

func date(year int, m time.Month, day int) time.Time {
	return time.Date(year, m, day, 0, 0, 0, 0, time.UTC)
}

// ColombianHolidays returns the national public holidays (días festivos)
// observed in Colombia for the given year, sorted ascending. There are normally
// 18; a year is 17 when two Ley Emiliani holidays shift onto the same Monday.
func ColombianHolidays(year int) []time.Time {
	var hs []time.Time

	// Fixed-date holidays — always observed on their calendar date.
	hs = append(hs,
		date(year, time.January, 1),   // Año Nuevo
		date(year, time.May, 1),       // Día del Trabajo
		date(year, time.July, 20),     // Día de la Independencia
		date(year, time.August, 7),    // Batalla de Boyacá
		date(year, time.December, 8),  // Inmaculada Concepción
		date(year, time.December, 25), // Navidad
	)

	// Ley Emiliani holidays — moved to the following Monday.
	for _, h := range []time.Time{
		date(year, time.January, 6),   // Reyes Magos
		date(year, time.March, 19),    // San José
		date(year, time.June, 29),     // San Pedro y San Pablo
		date(year, time.August, 15),   // Asunción de la Virgen
		date(year, time.October, 12),  // Día de la Raza
		date(year, time.November, 1),  // Todos los Santos
		date(year, time.November, 11), // Independencia de Cartagena
	} {
		hs = append(hs, nextMonday(h))
	}

	// Easter-relative holidays. Jueves Santo and Viernes Santo stay on their
	// liturgical days; the rest carry the Ley Emiliani Monday shift.
	e := easter(year)
	hs = append(hs,
		e.AddDate(0, 0, -3),             // Jueves Santo (Maundy Thursday)
		e.AddDate(0, 0, -2),             // Viernes Santo (Good Friday)
		nextMonday(e.AddDate(0, 0, 39)), // Ascensión del Señor
		nextMonday(e.AddDate(0, 0, 60)), // Corpus Christi
		nextMonday(e.AddDate(0, 0, 68)), // Sagrado Corazón
	)

	// Two Ley Emiliani holidays can shift onto the same Monday (e.g. in 2025
	// San Pedro and Sagrado Corazón both land on June 30). A coinciding pair is
	// one non-working day, so return distinct dates.
	return dedupe(hs)
}

// dedupe returns the unique calendar days in hs, sorted ascending.
func dedupe(hs []time.Time) []time.Time {
	seen := make(map[string]bool, len(hs))
	var out []time.Time
	for _, h := range hs {
		key := h.Format("2006-01-02")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

// holidaysFunc returns a query function that yields the Colombian holiday
// calendar. Computing for a span of years keeps queries correct across a
// year boundary (e.g. a due date in early January).
func holidaysFunc(years ...int) eval.Func {
	var all []any
	for _, y := range years {
		for _, h := range ColombianHolidays(y) {
			all = append(all, h)
		}
	}
	return eval.Func{Name: "holidays", Call: func(...any) (any, error) { return all, nil }}
}

func main() {
	year := 2026

	fmt.Printf("# Colombian public holidays %d (computed, Ley Emiliani applied)\n", year)
	for _, h := range ColombianHolidays(year) {
		fmt.Printf("  %s  %s\n", h.Format("2006-01-02"), h.Weekday())
	}

	// Register the calendar as holidays() for the business-day functions. A real
	// service would use the relevant year(s), e.g. time.Now().Year().
	holidays := eval.WithFunctions(holidaysFunc(year))
	fields := []validate.FieldConfig{
		{Name: "due", Type: validate.TypeDate, AllowedOps: validate.DateOps},
	}

	fmt.Println("\n# isBusinessDay(due, holidays())")
	// 2026-01-12 is the Monday that Reyes Magos (Jan 6, a Tuesday) moves to.
	for _, d := range []time.Time{
		date(2026, time.January, 12), // moved holiday → not a business day
		date(2026, time.January, 13), // ordinary Tuesday → business day
		date(2026, time.January, 1),  // New Year → not a business day
	} {
		prog, err := eval.Compile("isBusinessDay(due, holidays())", fields, holidays)
		if err != nil {
			log.Fatalf("compile: %v", err)
		}
		fmt.Printf("  %s (%s) -> %v\n", d.Format("2006-01-02"), d.Weekday(), prog.Match(map[string]any{"due": d}))
	}

	fmt.Println("\n# addBusinessDays(due, 3, holidays()) — SLA three working days out")
	prog, err := eval.CompileValue("addBusinessDays(due, 3, holidays())", fields, holidays)
	if err != nil {
		log.Fatalf("compile: %v", err)
	}
	start := date(2026, time.January, 9) // Friday before the moved Reyes Magos
	v, err := prog.Eval(map[string]any{"due": start})
	if err != nil {
		log.Fatalf("eval: %v", err)
	}
	out := v.(time.Time)
	fmt.Printf("  start %s (%s) + 3 business days -> %s (%s)\n",
		start.Format("2006-01-02"), start.Weekday(), out.Format("2006-01-02"), out.Weekday())
	fmt.Println("  (skips Sat/Sun and Mon Jan 12, the moved Reyes Magos holiday)")
}
