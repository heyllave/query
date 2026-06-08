package main

import (
	"testing"
	"time"
)

// TestColombianHolidays2026 pins the full computed national calendar for a year
// so a regression in the Easter computus or the Ley Emiliani shift is caught.
func TestColombianHolidays2026(t *testing.T) {
	want := []string{
		"2026-01-01", // Año Nuevo
		"2026-01-12", // Reyes Magos (moved from Jan 6)
		"2026-03-23", // San José (moved from Mar 19)
		"2026-04-02", // Jueves Santo
		"2026-04-03", // Viernes Santo
		"2026-05-01", // Día del Trabajo
		"2026-05-18", // Ascensión (Easter+39, moved)
		"2026-06-08", // Corpus Christi (Easter+60, moved)
		"2026-06-15", // Sagrado Corazón (Easter+68, moved)
		"2026-06-29", // San Pedro y San Pablo
		"2026-07-20", // Independencia
		"2026-08-07", // Batalla de Boyacá
		"2026-08-17", // Asunción (moved from Aug 15)
		"2026-10-12", // Día de la Raza
		"2026-11-02", // Todos los Santos (moved from Nov 1)
		"2026-11-16", // Independencia de Cartagena (moved from Nov 11)
		"2026-12-08", // Inmaculada Concepción
		"2026-12-25", // Navidad
	}

	got := ColombianHolidays(2026)
	if len(got) != len(want) {
		t.Fatalf("got %d holidays, want %d", len(got), len(want))
	}
	for i, w := range want {
		if g := got[i].Format("2006-01-02"); g != w {
			t.Errorf("holiday %d = %s, want %s", i, g, w)
		}
	}
}

// TestEaster checks the computus against known Western Easter Sundays.
func TestEaster(t *testing.T) {
	cases := map[int]string{
		2024: "2024-03-31",
		2025: "2025-04-20",
		2026: "2026-04-05",
		2027: "2027-03-28",
	}
	for year, want := range cases {
		if got := easter(year).Format("2006-01-02"); got != want {
			t.Errorf("easter(%d) = %s, want %s", year, got, want)
		}
	}
}

// TestColombianHolidays_Invariants verifies the calendar is strictly ascending
// (hence unique) across a span of years. The count is usually 18 but is 17 in
// years where two Ley Emiliani holidays shift onto the same Monday (e.g. 2025).
func TestColombianHolidays_Invariants(t *testing.T) {
	for year := 2020; year <= 2030; year++ {
		hs := ColombianHolidays(year)
		if len(hs) < 17 || len(hs) > 18 {
			t.Errorf("%d: got %d holidays, want 17 or 18", year, len(hs))
		}
		for i := 1; i < len(hs); i++ {
			if !hs[i-1].Before(hs[i]) {
				t.Errorf("%d: holidays not strictly ascending at %d (%s, %s)",
					year, i, hs[i-1].Format("2006-01-02"), hs[i].Format("2006-01-02"))
			}
		}
	}
}

func TestNextMonday(t *testing.T) {
	// A Monday is unchanged; every other day advances to the next Monday.
	mon := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)
	if got := nextMonday(mon); !got.Equal(mon) {
		t.Errorf("nextMonday(Mon) = %s, want unchanged", got.Format("2006-01-02"))
	}
	tue := time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC) // Reyes Magos
	if got := nextMonday(tue); !got.Equal(mon) {
		t.Errorf("nextMonday(Tue Jan 6) = %s, want 2026-01-12", got.Format("2006-01-02"))
	}
}
