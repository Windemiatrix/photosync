package dates_test

import (
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain/dates"
)

func TestSame(t *testing.T) {
	t.Parallel()

	m := time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC)
	wall545 := time.Date(2024, 1, 1, 5, 44, 59, 0, time.UTC)
	far := time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	z545 := time.FixedZone("", 5*3600+45*60)

	cases := []struct {
		label string
		a, b  time.Time
		want  bool
	}{
		{"equal", m, m, true},
		{"2 s later", m.Add(2 * time.Second), m, true},
		{"2 s earlier", m.Add(-2 * time.Second), m, true},
		{"2 s and 1 ns later", m.Add(2*time.Second + time.Nanosecond), m, false},
		{"3 s later", m.Add(3 * time.Second), m, false},
		{"3 s earlier", m.Add(-3 * time.Second), m, false},
		{"same instant in another zone", m.In(z545), m, true},
		{"wall clock is not the moment", wall545, m, false},
		{"far apart", time.Time{}, far, false},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			if got := dates.Same(tc.a, tc.b); got != tc.want {
				t.Errorf("Same(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
			if got := dates.Same(tc.b, tc.a); got != tc.want {
				t.Errorf("Same(%v, %v) = %v, want %v", tc.b, tc.a, got, tc.want)
			}
		})
	}
}

func TestQuarterHourShift(t *testing.T) {
	t.Parallel()

	m := time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC)
	wall545 := time.Date(2024, 1, 1, 5, 44, 59, 0, time.UTC)
	far := time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	z545 := time.FixedZone("", 5*3600+45*60)

	cases := []struct {
		label string
		a, b  time.Time
		k     int
		ok    bool
	}{
		{"same moment", m, m, 0, true},
		{"+5:45", m.Add(5*time.Hour + 45*time.Minute), m, 23, true},
		{"+5:45 and 2 s", m.Add(5*time.Hour + 45*time.Minute + 2*time.Second), m, 23, true},
		{"+5:45 less 1 s", m.Add(5*time.Hour + 45*time.Minute - time.Second), m, 23, true},
		{"+5:45 and 3 s", m.Add(5*time.Hour + 45*time.Minute + 3*time.Second), m, 0, false},
		{"+12:45", m.Add(12*time.Hour + 45*time.Minute), m, 51, true},
		{"-3:30", m.Add(-(3*time.Hour + 30*time.Minute)), m, -14, true},
		{"+14:00", m.Add(14 * time.Hour), m, 56, true},
		{"-14:00", m.Add(-14 * time.Hour), m, -56, true},
		{"+14:00 and 2 s", m.Add(14*time.Hour + 2*time.Second), m, 56, true},
		{"+14:15", m.Add(14*time.Hour + 15*time.Minute), m, 0, false},
		{"-14:15", m.Add(-(14*time.Hour + 15*time.Minute)), m, 0, false},
		{"half a quarter", m.Add(7*time.Minute + 30*time.Second), m, 0, false},
		{"1:07:30", m.Add(time.Hour + 7*time.Minute + 30*time.Second), m, 0, false},
		{"same instant in another zone", m.In(z545), m, 0, true},
		{"wall clock of +5:45", wall545, m, 23, true},
		{"far apart", time.Time{}, far, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			if k, ok := dates.QuarterHourShift(tc.a, tc.b); k != tc.k || ok != tc.ok {
				t.Errorf("QuarterHourShift(%v, %v) = (%d, %v), want (%d, %v)", tc.a, tc.b, k, ok, tc.k, tc.ok)
			}
			if k, ok := dates.QuarterHourShift(tc.b, tc.a); k != -tc.k || ok != tc.ok {
				t.Errorf("QuarterHourShift(%v, %v) = (%d, %v), want (%d, %v)", tc.b, tc.a, k, ok, -tc.k, tc.ok)
			}
		})
	}
}

// covers: INV-14
func TestQuarterHourShiftEveryK(t *testing.T) {
	t.Parallel()

	bases := []time.Time{
		time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC),
		time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 2, 29, 12, 0, 0, 500_000_000, time.UTC),
	}
	shifts := []time.Duration{
		-3 * time.Second,
		-(2*time.Second + time.Nanosecond),
		-2 * time.Second,
		-time.Second,
		0,
		time.Second,
		2 * time.Second,
		2*time.Second + time.Nanosecond,
		3 * time.Second,
	}

	for _, base := range bases {
		t.Run(base.Format(time.RFC3339Nano), func(t *testing.T) {
			t.Parallel()

			for k := -60; k <= 60; k++ {
				for _, j := range shifts {
					a := base.Add(time.Duration(k)*15*time.Minute + j)

					withinTolerance := -2*time.Second <= j && j <= 2*time.Second
					wantOK := -56 <= k && k <= 56 && withinTolerance
					wantK := 0
					if wantOK {
						wantK = k
					}
					wantSame := k == 0 && withinTolerance

					if gotK, gotOK := dates.QuarterHourShift(a, base); gotK != wantK || gotOK != wantOK {
						t.Fatalf("base %v, k %d, j %v: QuarterHourShift(a, base) = (%d, %v), want (%d, %v)",
							base, k, j, gotK, gotOK, wantK, wantOK)
					}
					if gotK, gotOK := dates.QuarterHourShift(base, a); gotK != -wantK || gotOK != wantOK {
						t.Fatalf("base %v, k %d, j %v: QuarterHourShift(base, a) = (%d, %v), want (%d, %v)",
							base, k, j, gotK, gotOK, -wantK, wantOK)
					}
					if got := dates.Same(a, base); got != wantSame {
						t.Fatalf("base %v, k %d, j %v: Same(a, base) = %v, want %v", base, k, j, got, wantSame)
					}
					if got := dates.Same(base, a); got != wantSame {
						t.Fatalf("base %v, k %d, j %v: Same(base, a) = %v, want %v", base, k, j, got, wantSame)
					}
				}
			}
		})
	}
}
