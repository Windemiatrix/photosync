package dates_test

import (
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain/dates"
)

func TestLocalDay(t *testing.T) {
	t.Parallel()

	z9 := time.FixedZone("", 9*3600)

	cases := []struct {
		label  string
		t      time.Time
		offset *int
		year   int
		month  time.Month
		day    int
	}{
		{"-05:00 evening", time.Date(2024, 1, 1, 4, 50, 0, 0, time.UTC), ptr(-5 * 3600), 2023, time.December, 31},
		{"+5:45 after midnight", time.Date(2023, 12, 31, 18, 20, 0, 0, time.UTC), ptr(5*3600 + 45*60), 2024, time.January, 1},
		{"+5:45 before midnight", time.Date(2023, 12, 31, 18, 10, 0, 0, time.UTC), ptr(5*3600 + 45*60), 2023, time.December, 31},
		{"+12:45", time.Date(2023, 12, 31, 11, 20, 0, 0, time.UTC), ptr(12*3600 + 45*60), 2024, time.January, 1},
		{"+14:00 midnight", time.Date(2023, 12, 31, 10, 0, 0, 0, time.UTC), ptr(14 * 3600), 2024, time.January, 1},
		{"-12:00 last second", time.Date(2024, 1, 1, 11, 59, 59, 0, time.UTC), ptr(-12 * 3600), 2023, time.December, 31},
		{"zero offset", time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC), ptr(0), 2023, time.December, 31},
		{"leap day", time.Date(2024, 2, 28, 23, 30, 0, 0, time.UTC), ptr(3600), 2024, time.February, 29},
		{"nil offset", time.Date(2023, 12, 31, 23, 50, 0, 0, time.UTC), nil, 2023, time.December, 31},
		{"nil offset, other location", time.Date(2023, 12, 31, 20, 0, 0, 0, time.UTC).In(z9), nil, 2023, time.December, 31},
		{"offset, other location", time.Date(2024, 1, 1, 4, 50, 0, 0, time.UTC).In(z9), ptr(-5 * 3600), 2023, time.December, 31},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			year, month, day := dates.LocalDay(tc.t, tc.offset)
			if year != tc.year || month != tc.month || day != tc.day {
				t.Errorf("LocalDay(%v, %v) = %d-%d-%d, want %d-%d-%d",
					tc.t, tc.offset, year, month, day, tc.year, tc.month, tc.day)
			}
		})
	}
}
