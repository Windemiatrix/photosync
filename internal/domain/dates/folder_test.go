package dates_test

import (
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain/dates"
)

func TestParseFolder(t *testing.T) {
	t.Parallel()

	positive := []struct {
		label string
		dir   string
		want  time.Time
	}{
		{"day with event", "2024/05-01 - Holidays", time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)},
		{"day only", "2024/05-01", time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)},
		{"day with title", "2024/09-24 Flight home", time.Date(2024, 9, 24, 12, 0, 0, 0, time.UTC)},
		{"day event subfolder", "2024/05-01 - Holidays/sources", time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)},
		{"day subfolder", "2024/05-01/sources", time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)},
		{"month with event", "2024/06 - Sea", time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)},
		{"month only", "2024/06", time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)},
		{"month subfolder", "2024/06/sources", time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)},
		{"leap day", "2024/02-29 - Leap", time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC)},
		{"last day of year", "1999/12-31 - New year", time.Date(1999, 12, 31, 12, 0, 0, 0, time.UTC)},
	}
	for _, tc := range positive {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			got, ok := dates.ParseFolder(tc.dir)
			if !ok {
				t.Fatalf("ParseFolder(%q): ok = false, want true", tc.dir)
			}
			if !got.Equal(tc.want) {
				t.Errorf("ParseFolder(%q) = %v, want %v", tc.dir, got, tc.want)
			}
			if got.Location() != time.UTC {
				t.Errorf("ParseFolder(%q).Location() = %v, want UTC", tc.dir, got.Location())
			}
		})
	}

	negative := []struct {
		label string
		dir   string
	}{
		{"empty", ""},
		{"year only", "2024"},
		{"named event", "2024/Trip"},
		{"unsorted", "2024/unsorted"},
		{"nested event", "2024/Trip/05-01 - Kazan"},
		{"no year", "Albums/05-01 - Holidays"},
		{"leading slash", "/2024/05-01"},
		{"two digit year", "24/05-01"},
		{"five digit year", "20245/05-01"},
		{"fullwidth year", fullwidth("2024") + "/05-01"},
		{"fullwidth month", "2024/" + fullwidth("05-01")},
		{"no separator", "2024/0501"},
		{"one digit day", "2024/05-1 x"},
		{"one digit month", "2024/5-01 x"},
		{"day then dash", "2024/05-01-Holidays"},
		{"month then dash", "2024/06-Sea"},
		{"month then letter", "2024/06Sea"},
		{"leading space", "2024/ 06 - Sea"},
		{"month 13", "2024/13 - X"},
		{"month 00", "2024/00 - X"},
		{"day 32", "2024/05-32 - X"},
		{"day 00", "2024/05-00"},
		{"feb 29 common year", "2023/02-29 - X"},
	}
	for _, tc := range negative {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			if got, ok := dates.ParseFolder(tc.dir); ok {
				t.Errorf("ParseFolder(%q) = %v, true; want no date", tc.dir, got)
			}
		})
	}
}
