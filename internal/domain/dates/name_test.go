package dates_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain/dates"
)

func ptr(v int) *int { return &v }

func sameOffset(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// \d in Go regexp is ASCII-only, so fullwidth digits must not parse.
func fullwidth(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r - '0' + 0xFF10
		}
		return r
	}, s)
}

func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var parseNamePositive = []struct {
	label  string
	name   string
	want   time.Time
	offset *int
}{
	{"pxl utc millis", "PXL_20231231_235959123.mp4", time.Date(2023, 12, 31, 23, 59, 59, 123_000_000, time.UTC), ptr(0)},
	{"pxl suffix", "PXL_20231231_235959123~2.jpg", time.Date(2023, 12, 31, 23, 59, 59, 123_000_000, time.UTC), ptr(0)},
	{"pxl no extension", "PXL_20231231_235959123", time.Date(2023, 12, 31, 23, 59, 59, 123_000_000, time.UTC), ptr(0)},
	{"pxl motion photo", "PXL_20240229_000000000.MP.jpg", time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC), ptr(0)},
	{"img", "IMG_20231231_235959.jpg", time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC), nil},
	{"img no extension", "IMG_20231231_235959", time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC), nil},
	{"img underscore counter", "IMG_20231231_235959_1.jpg", time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC), nil},
	{"img copy suffix", "IMG_20231231_235959 (1).jpg", time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC), nil},
	{"img leap day", "IMG_20240229_120000.jpg", time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC), nil},
	{"vid", "VID_20230615_081500.mp4", time.Date(2023, 6, 15, 8, 15, 0, 0, time.UTC), nil},
	{"dashed", "2023-12-31 23.59.59.jpg", time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC), nil},
	{"dashed counter", "2023-12-31 23.59.59-1.jpg", time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC), nil},
	{"dashed copy suffix", "2023-12-31 23.59.59 (1).jpg", time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC), nil},
	{"compact", "20231231_235959.jpg", time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC), nil},
	{"dashed no extension", "2023-12-31 23.59.59", time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC), nil},
	{"compact no extension", "20231231_235959", time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC), nil},
	{"compact bracket suffix", "20231231_235959(0).jpg", time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC), nil},
	{"img epoch", "IMG_19700101_000000.jpg", time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC), nil},
}

var parseNameNegative = []struct {
	label string
	name  string
}{
	{"month 13", "IMG_20231301_120000.jpg"},
	{"month 00", "IMG_20230001_120000.jpg"},
	{"day 00", "IMG_20230100_120000.jpg"},
	{"feb 31", "IMG_20230231_120000.jpg"},
	{"feb 29 common year", "IMG_20230229_120000.jpg"},
	{"hour 24", "IMG_20231231_240000.jpg"},
	{"minute 60", "IMG_20231231_236000.jpg"},
	{"second 60", "IMG_20231231_235960.jpg"},
	{"five digits of time", "IMG_20231231_23595.jpg"},
	{"no time", "IMG_20231231.jpg"},
	{"pxl without millis", "PXL_20231231_235959.mp4"},
	{"pxl two millis digits", "PXL_20231231_23595912.mp4"},
	{"dashed without seconds", "2023-12-31 23.59.jpg"},
	{"img seven time digits", "IMG_20231231_2359590.jpg"},
	{"pxl four millis digits", "PXL_20231231_2359591234.mp4"},
	{"compact seven time digits", "20231231_2359591.jpg"},
	{"dashed three second digits", "2023-12-31 23.59.590.jpg"},
	{"prefix before img", "edited_IMG_20231231_235959.jpg"},
	{"prefix before compact", "x20231231_235959.jpg"},
	{"prefix before pxl", "edited_PXL_20231231_235959123.mp4"},
	{"prefix before dashed", "x2023-12-31 23.59.59.jpg"},
	{"pxl feb 31", "PXL_20230231_120000000.mp4"},
	{"dashed month 13", "2023-13-01 12.00.00.jpg"},
	{"compact hour 24", "20231231_240000.jpg"},
	{"lower case img", "img_20231231_235959.jpg"},
	{"lower case pxl", "pxl_20231231_235959123.mp4"},
	{"sign after pxl seconds", "PXL_20231231_235959+12.mp4"},
	{"img counter", "IMG_1234.HEIC"},
	{"img edited counter", "IMG_E1234.JPG"},
	{"dsc counter", "DSC_0001.JPG"},
	{"messenger name", "IMG-20231231-WA0001.jpg"},
	{"screenshot name", "Screenshot_20231231-235959.png"},
	{"dashed time separators", "2023-12-31 23-59-59.jpg"},
	{"fullwidth digits", fullwidth("IMG_20231231_235959.jpg")},
	{"empty", ""},
}

func TestParseName(t *testing.T) {
	t.Parallel()

	for _, tc := range parseNamePositive {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			got, ok := dates.ParseName(tc.name)
			if !ok {
				t.Fatalf("ParseName(%q): ok = false, want true", tc.name)
			}
			if !got.Time.Equal(tc.want) {
				t.Errorf("ParseName(%q).Time = %v, want %v", tc.name, got.Time, tc.want)
			}
			if got.Time.Location() != time.UTC {
				t.Errorf("ParseName(%q).Time.Location() = %v, want UTC", tc.name, got.Time.Location())
			}
			if !sameOffset(got.Offset, tc.offset) {
				t.Errorf("ParseName(%q).Offset = %v, want %v", tc.name, got.Offset, tc.offset)
			}
		})
	}

	for _, tc := range parseNameNegative {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			if got, ok := dates.ParseName(tc.name); ok {
				t.Errorf("ParseName(%q) = %v, true; want no date", tc.name, got.Time)
			}
		})
	}

	t.Run("offset not shared", func(t *testing.T) {
		t.Parallel()

		const name = "PXL_20231231_235959123.mp4"
		first, ok := dates.ParseName(name)
		if !ok || first.Offset == nil {
			t.Fatalf("ParseName(%q): ok = %v, Offset = %v; want a date with Offset", name, ok, first.Offset)
		}
		*first.Offset = 3600

		second, ok := dates.ParseName(name)
		if !ok || second.Offset == nil {
			t.Fatalf("second ParseName(%q): ok = %v, Offset = %v; want a date with Offset", name, ok, second.Offset)
		}
		if *second.Offset != 0 {
			t.Errorf("second ParseName(%q).Offset = %d, want 0", name, *second.Offset)
		}
	})
}

func FuzzParseName(f *testing.F) {
	for _, tc := range parseNamePositive {
		f.Add(tc.name)
	}
	for _, tc := range parseNameNegative {
		f.Add(tc.name)
	}

	f.Fuzz(func(t *testing.T, name string) {
		got, ok := dates.ParseName(name)
		if !ok {
			return
		}
		if got.Time.Location() != time.UTC {
			t.Fatalf("ParseName(%q).Time.Location() = %v, want UTC", name, got.Time.Location())
		}

		rest, found := strings.CutPrefix(digits(name), got.Time.Format("20060102150405"))
		if !found {
			t.Fatalf("ParseName(%q).Time = %v: not the leading digits of the name", name, got.Time)
		}

		if got.Offset == nil {
			if got.Time.Nanosecond() != 0 {
				t.Fatalf("ParseName(%q): nanoseconds %d without Offset, want 0", name, got.Time.Nanosecond())
			}
			return
		}
		if *got.Offset != 0 {
			t.Fatalf("ParseName(%q).Offset = %d, want 0", name, *got.Offset)
		}
		if got.Time.Nanosecond()%1_000_000 != 0 {
			t.Fatalf("ParseName(%q): %d ns is not whole milliseconds", name, got.Time.Nanosecond())
		}
		wantMillis := fmt.Sprintf("%03d", got.Time.Nanosecond()/1_000_000)
		if !strings.HasPrefix(rest, wantMillis) {
			t.Fatalf("ParseName(%q): milliseconds %s are not the digits after the time (%q)", name, wantMillis, rest)
		}
	})
}
