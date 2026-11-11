package naming_test

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
	"pgregory.net/rapid"

	"github.com/Windemiatrix/photosync/internal/domain/naming"
)

const quarantinePrefix = ".photosync/quarantine/2024-05-01/"

var reservedStems = []string{
	"CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$",
	"COM0", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
	"COM¹", "COM²", "COM³",
	"LPT0", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
	"LPT¹", "LPT²", "LPT³",
}

func validName(s string) bool {
	if s == "" || len(s) > 255 {
		return false
	}
	if !utf8.ValidString(s) || norm.NFC.String(s) != s {
		return false
	}
	if strings.ContainsAny(s, `/\:*?"<>|`) || strings.IndexFunc(s, func(r rune) bool { return r < 0x20 }) >= 0 {
		return false
	}
	if strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") || strings.HasSuffix(s, " ") {
		return false
	}
	stem, _, _ := strings.Cut(s, ".")
	return !slices.Contains(reservedStems, strings.ToUpper(strings.TrimRight(stem, " ")))
}

func TestIsValidPart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want bool
	}{
		{"IMG_0001.HEIC", true},
		{"2024", true},
		{"05-01 - Holidays", true},
		{"Отпуск", true},
		{"a∕b", true},
		{"_", true},
		{"a.b", true},
		{" a", true},
		{strings.Repeat("a", 255), true},
		{"COM10", true},
		{"क़", true},
		{"a\x7fb", true},

		{"", false},
		{".", false},
		{"..", false},
		{".a", false},
		{"a.", false},
		{"a ", false},
		{"a/b", false},
		{`a\b`, false},
		{"a:b", false},
		{"a*b", false},
		{"a?b", false},
		{`a"b`, false},
		{"a<b", false},
		{"a>b", false},
		{"a|b", false},
		{"a\x00b", false},
		{"a\x1fb", false},
		{"й", false},
		{"क़", false},
		{"\xff", false},
		{strings.Repeat("a", 256), false},
		{"CON", false},
		{"con.heic", false},
		{"COM¹", false},
		{"LPT0.txt", false},
		{"CON .heic", false},
		{"CONOUT$", false},
	}
	for _, tt := range tests {
		if got := naming.IsValidPart(tt.in); got != tt.want {
			t.Errorf("IsValidPart(%q) = %v, want %v", tt.in, got, tt.want)
		}
		if got := validName(tt.in); got != tt.want {
			t.Errorf("validName(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestIsValidPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want bool
	}{
		{"a", true},
		{"2024/05-01 - Holidays/IMG_0001.HEIC", true},
		{"Albums/Trips/IMG_0001.HEIC", true},

		{"", false},
		{"/a", false},
		{"a/", false},
		{"a//b", false},
		{"a/./b", false},
		{"a/../b", false},
		{"../a", false},
		{"a/b:c", false},
		{"a/й", false},
		{"a/CON", false},
	}
	for _, tt := range tests {
		if got := naming.IsValidPath(tt.in); got != tt.want {
			t.Errorf("IsValidPath(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestIsValidPathLimit(t *testing.T) {
	t.Parallel()

	level := strings.Repeat("я", 100)
	base := strings.Repeat(level+"/", 8) + level
	tests := []struct {
		lastLevelRunes int
		wantRunes      int
		want           bool
	}{
		{81, 1023, true},
		{82, 1024, true},
		{83, 1025, false},
	}
	for _, tt := range tests {
		p := base + "/" + strings.Repeat("я", tt.lastLevelRunes)
		if got := utf8.RuneCountInString(quarantinePrefix + p); got != tt.wantRunes {
			t.Fatalf("path with a last level of %d runes is %d runes with the prefix, want %d",
				tt.lastLevelRunes, got, tt.wantRunes)
		}
		if got := naming.IsValidPath(p); got != tt.want {
			t.Errorf("IsValidPath of %d runes (%d with the prefix) = %v, want %v",
				utf8.RuneCountInString(p), tt.wantRunes, got, tt.want)
		}
	}
}

func TestValidProperties(t *testing.T) {
	t.Parallel()

	t.Run("part check agrees with the oracle", func(t *testing.T) {
		t.Parallel()
		rapid.Check(t, func(t *rapid.T) {
			x := nameGen().Draw(t, "x")
			if got, want := naming.IsValidPart(x), validName(x); got != want {
				t.Fatalf("IsValidPart(%q) = %v, oracle says %v", x, got, want)
			}
		})
	})

	t.Run("path of sanitized levels is valid within the limit", func(t *testing.T) {
		t.Parallel()
		limit := 1024 - utf8.RuneCountInString(quarantinePrefix)
		rapid.Check(t, func(t *rapid.T) {
			xs := rapid.SliceOfN(nameGen(), 1, 5).Draw(t, "xs")
			parts := make([]string, len(xs))
			for i, x := range xs {
				parts[i] = naming.SanitizePart(x)
			}
			joined := strings.Join(parts, "/")
			want := utf8.RuneCountInString(joined) <= limit
			if got := naming.IsValidPath(joined); got != want {
				t.Fatalf("IsValidPath(%q) = %v, want %v for %d runes", joined, got, want, utf8.RuneCountInString(joined))
			}
		})
	})
}
