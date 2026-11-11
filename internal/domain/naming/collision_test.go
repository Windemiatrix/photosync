package naming_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/Windemiatrix/go-libs/osxphotos/export"
	"github.com/Windemiatrix/photosync/internal/domain/naming"
)

func dirOf(files ...string) map[string]bool {
	taken := map[string]bool{}
	for _, f := range files {
		taken[export.StemKey(f)] = true
	}
	return taken
}

func issueAll(names ...string) []string {
	taken := map[string]bool{}
	got := make([]string, len(names))
	for i, n := range names {
		got[i] = naming.Free(n, taken)
		taken[export.StemKey(got[i])] = true
	}
	return got
}

func TestFree(t *testing.T) {
	t.Parallel()

	tests := []struct {
		desc, name string
		files      []string
		want       string
	}{
		{"free", "IMG_0001.HEIC", nil, "IMG_0001.HEIC"},
		{"taken", "IMG_0001.HEIC", []string{"IMG_0001.HEIC"}, "IMG_0001 (1).HEIC"},
		{"another file, case and extension do not matter", "IMG_0001.HEIC", []string{"img_0001.jpg"}, "IMG_0001 (1).HEIC"},
		{
			"the video of a Live Photo takes the stem",
			"IMG_0001.HEIC", []string{"IMG_0001.HEIC", "IMG_0001 (1).MOV"}, "IMG_0001 (2).HEIC",
		},
		{"numbered names alone do not block the plain one", "IMG_0001.HEIC", []string{"IMG_0001 (1).HEIC"}, "IMG_0001.HEIC"},
		{"sanitized first", "a:b.jpg", nil, "a꞉b.jpg"},
		{"sanitized name is taken", "a:b.jpg", []string{"a꞉b.JPG"}, "a꞉b (1).jpg"},
		{"NFD name, NFC directory", "Café.jpg", []string{"Café.png"}, "Café (1).jpg"},
		{"no extension", "README", []string{"README"}, "README (1)"},
		{"only the last extension is cut", "a.b.jpg", []string{"a.b.png"}, "a.b (1).jpg"},
		{"another stem does not block", "a.b.jpg", []string{"a.png"}, "a.b.jpg"},
		{"empty name", "", nil, "_"},
		{"empty name is taken", "", []string{"_"}, "_ (1)"},
		{"reserved name", "CON.jpg", []string{"CON_.png"}, "CON_ (1).jpg"},
	}
	for _, tt := range tests {
		if !validName(tt.want) {
			t.Errorf("%s: want %q is not a valid name", tt.desc, tt.want)
		}
		if got := naming.Free(tt.name, dirOf(tt.files...)); got != tt.want {
			t.Errorf("%s: Free(%q, %q) = %q, want %q", tt.desc, tt.name, tt.files, got, tt.want)
		}
	}
}

func TestFreeLimits(t *testing.T) {
	t.Parallel()

	a := func(n int) string { return strings.Repeat("a", n) }
	b := func(n int) string { return strings.Repeat("b", n) }

	countFiles := []string{a(251) + ".jpg"}
	for k := 1; k <= 9; k++ {
		countFiles = append(countFiles, fmt.Sprintf("%s (%d).jpg", a(247), k))
	}

	tests := []struct {
		name, in string
		files    []string
		want     string
	}{
		{"fits exactly with the number", a(247) + ".jpg", []string{a(247) + ".jpg"}, a(247) + " (1).jpg"},
		{"one byte over", a(248) + ".jpg", []string{a(248) + ".jpg"}, a(247) + " (1).jpg"},
		{"four bytes over", a(251) + ".jpg", []string{a(251) + ".jpg"}, a(247) + " (1).jpg"},
		{"count does not go back", a(251) + ".jpg", countFiles, a(246) + " (10).jpg"},
		{
			"cut on a rune boundary",
			strings.Repeat("я", 125) + ".jpg", []string{strings.Repeat("я", 125) + ".jpg"},
			strings.Repeat("я", 123) + " (1).jpg",
		},
		{
			"mark stays with its base",
			a(245) + "е́bb.jpg", []string{a(245) + "е́bb.jpg"},
			a(245) + " (1).jpg",
		},
		{
			"only the last extension stays",
			"a.b" + a(248) + ".jpg", []string{"a.b" + a(248) + ".jpg"},
			"a.b" + a(244) + " (1).jpg",
		},
		{
			"no extension",
			strings.Repeat("/", 86), []string{strings.Repeat("∕", 85)},
			strings.Repeat("∕", 83) + " (1)",
		},
		{"long extension", "x." + b(253), []string{"x." + b(253)}, "_ (1)"},
		{"stem cut inside its only letter", "я." + b(249), []string{"я." + b(249)}, "_ (1)"},
		{"fallback name is taken", "x." + b(253), []string{"x." + b(253), "_ (1)"}, "_ (2)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := naming.Free(tt.in, dirOf(tt.files...))
			if got != tt.want {
				t.Errorf("Free(%d bytes %q) = %d bytes %q, want %d bytes %q",
					len(tt.in), tt.in, len(got), got, len(tt.want), tt.want)
			}
			if len(got) > maxNameBytes {
				t.Errorf("Free(%d bytes) is %d bytes, want at most %d", len(tt.in), len(got), maxNameBytes)
			}
		})
	}
}

func TestFreeOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		names []string
		want  []string
	}{
		{
			[]string{"IMG_0001.HEIC", "img_0001.jpg", "IMG_0001.HEIC"},
			[]string{"IMG_0001.HEIC", "img_0001 (1).jpg", "IMG_0001 (2).HEIC"},
		},
		{
			[]string{"img_0001.jpg", "IMG_0001.HEIC", "IMG_0001.HEIC"},
			[]string{"img_0001.jpg", "IMG_0001 (1).HEIC", "IMG_0001 (2).HEIC"},
		},
	}
	for _, tt := range tests {
		if got := issueAll(tt.names...); !slices.Equal(got, tt.want) {
			t.Errorf("names %q issued as %q, want %q", tt.names, got, tt.want)
		}
	}
}

func TestFreeProperties(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		xs := rapid.SliceOfN(nameGen(), 1, 6).Draw(t, "xs")
		taken := map[string]bool{}
		for i, x := range xs {
			k := rapid.IntRange(0, 3).Draw(t, fmt.Sprintf("numbers taken for %d", i))
			for n := range k {
				numbered, _ := export.IncrementName(naming.SanitizePart(x), n, nil)
				taken[export.StemKey(numbered)] = true
			}
		}
		for _, x := range slices.Concat(xs, xs) {
			sanitized := naming.SanitizePart(x)
			wasFree := !taken[export.StemKey(sanitized)]
			got := naming.Free(x, taken)
			if !validName(got) {
				t.Fatalf("Free(%q) = %q, not a valid name", x, got)
			}
			if taken[export.StemKey(got)] {
				t.Fatalf("Free(%q) = %q, its key is taken", x, got)
			}
			if wasFree && got != sanitized {
				t.Fatalf("Free(%q) = %q, want the free name %q unchanged", x, got, sanitized)
			}
			taken[export.StemKey(got)] = true
		}
	})
}
