package naming_test

import (
	"strings"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain/naming"
)

func FuzzSanitizePart(f *testing.F) {
	seeds := []string{
		"", ".", "..", "CON", "con.heic", "a/b", "й", "≮", "a\x01́", "\xff\xfe",
		strings.Repeat("я", 200),
		strings.Repeat("a", 254) + ".",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		out := naming.SanitizePart(s)
		if !validName(out) {
			t.Errorf("SanitizePart(%q) = %q, not a valid name", s, out)
		}
		if again := naming.SanitizePart(out); again != out {
			t.Errorf("SanitizePart(%q) = %q, but sanitizing it again gives %q", s, out, again)
		}
	})
}
