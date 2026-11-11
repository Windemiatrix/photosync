package naming_test

import (
	"path"
	"slices"
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"
	"pgregory.net/rapid"

	"github.com/Windemiatrix/photosync/internal/domain/naming"
)

const maxNameBytes = 255

func TestSanitizePart(t *testing.T) {
	t.Parallel()

	tests := []struct{ in, want string }{
		{"IMG_0001.HEIC", "IMG_0001.HEIC"},

		{"a/b", "a∕b"}, {`a\b`, "a⧵b"}, {"a:b", "a꞉b"},
		{"a*b", "a∗b"}, {"a?b", "a？b"}, {`a"b`, "a＂b"},
		{"a<b", "a‹b"}, {"a>b", "a›b"}, {"a|b", "aǀb"},
		{"a∕b", "a∕b"}, {"a．b", "a．b"},

		{"a\x00b", "ab"}, {"a\tb", "ab"}, {"a\x1fb", "ab"}, {"a\x7fb", "a\x7fb"},
		{"a\xffb", "a�b"},

		{"a.", "a．"}, {"a ", "a "}, {"a..", "a.．"}, {"a.b", "a.b"},
		{" a", " a"}, {".a", "．a"}, {".", "．"}, {"..", "．．"},
		{"...", "．.．"},

		{"CON", "CON_"}, {"con", "con_"}, {"Con.heic", "Con_.heic"},
		{"COM1.tar.gz", "COM1_.tar.gz"}, {"COM0", "COM0_"}, {"lpt9", "lpt9_"},
		{"com¹", "com¹_"}, {"NUL.", "NUL_．"}, {"AUX ", "AUX _"},
		{"COM10", "COM10"}, {"CONSOLE", "CONSOLE"}, {"ICON.png", "ICON.png"},
		{".CON", "．CON"}, {"CON .heic", "CON _.heic"}, {"conin$.heic", "conin$_.heic"},

		{"й", "й"}, {"й", "й"},
		{"ё", "ё"}, {"ё", "ё"},
		{"≮", "≮"}, {"≮", "≮"},
		{"a\x01́", "á"},
		{"क़", "क़"}, {"क़", "क़"},
		{"Ω", "Ω"}, {"Ω", "Ω"},

		{"", "_"}, {"\x00\x1f", "_"}, {"_", "_"},
	}
	for _, tt := range tests {
		got := naming.SanitizePart(tt.in)
		if got != tt.want {
			t.Errorf("SanitizePart(%q) = %q, want %q", tt.in, got, tt.want)
		}
		if len(got) > maxNameBytes {
			t.Errorf("SanitizePart(%q) is %d bytes, want at most %d", tt.in, len(got), maxNameBytes)
		}
	}
}

func TestSanitizePartLimits(t *testing.T) {
	t.Parallel()

	a := func(n int) string { return strings.Repeat("a", n) }
	tests := []struct{ name, in, want string }{
		{"exactly 255 bytes", a(255), a(255)},
		{"256 bytes", a(256), a(255)},
		{"cut on a rune boundary", a(254) + "я", a(254)},
		{"mark stays with its base", a(253) + "е́", a(253)},
		{"final dot lengthens the name, one more step back", a(254) + ".", a(254)},
		{"final dot lengthens the name, two bytes over", a(253) + ".", a(253)},
		{"final dot fits in 255 bytes", a(252) + ".", a(252) + "．"},
		{"final space fits in 255 bytes", a(253) + " ", a(253) + " "},
		{"final space does not fit", a(254) + " ", a(254)},
		{"cut exposes a final dot", a(254) + ".bc", a(254)},
		{"replacements are counted, exactly 255", strings.Repeat("/", 85), strings.Repeat("∕", 85)},
		{"replacements are counted, over the limit", strings.Repeat("/", 86), strings.Repeat("∕", 85)},
		{"reserved name and cut", "CON." + a(300), "CON_." + a(250)},
		{"cut leaves a reserved stem", "CONx" + strings.Repeat("́", 130), "CON_"},
		{"cut leaves no base", strings.Repeat("́", 300), "_"},
		{"leading dot counts in the length", "." + a(300), "．" + a(252)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := naming.SanitizePart(tt.in)
			if got != tt.want {
				t.Errorf("SanitizePart(%d bytes) = %d bytes %q, want %d bytes %q",
					len(tt.in), len(got), got, len(tt.want), tt.want)
			}
			if len(got) > maxNameBytes {
				t.Errorf("SanitizePart(%d bytes) is %d bytes, want at most %d", len(tt.in), len(got), maxNameBytes)
			}
		})
	}
}

func TestSanitizePartProperties(t *testing.T) {
	t.Parallel()

	t.Run("result is valid", func(t *testing.T) {
		t.Parallel()
		rapid.Check(t, func(t *rapid.T) {
			x := nameGen().Draw(t, "x")
			got := naming.SanitizePart(x)
			if !validName(got) {
				t.Fatalf("SanitizePart(%q) = %q, not a valid name", x, got)
			}
		})
	})

	t.Run("idempotent", func(t *testing.T) {
		t.Parallel()
		rapid.Check(t, func(t *rapid.T) {
			x := nameGen().Draw(t, "x")
			once := naming.SanitizePart(x)
			if twice := naming.SanitizePart(once); twice != once {
				t.Fatalf("SanitizePart(%q) = %q, but sanitizing it again gives %q", x, once, twice)
			}
		})
	})

	t.Run("valid name is unchanged", func(t *testing.T) {
		t.Parallel()
		rapid.Check(t, func(t *rapid.T) {
			x := nameGen().Draw(t, "x")
			if got := naming.SanitizePart(x); validName(x) && got != x {
				t.Fatalf("SanitizePart(%q) = %q, want the valid name unchanged", x, got)
			}
		})
	})

	t.Run("NFC and NFD forms give one name", func(t *testing.T) {
		t.Parallel()
		rapid.Check(t, func(t *rapid.T) {
			x := nameGen().Draw(t, "x")
			fromNFC := naming.SanitizePart(norm.NFC.String(x))
			fromNFD := naming.SanitizePart(norm.NFD.String(x))
			if fromNFC != fromNFD {
				t.Fatalf("SanitizePart of NFC form of %q = %q, of NFD form = %q", x, fromNFC, fromNFD)
			}
		})
	})

	t.Run("levels keep the depth of the path", func(t *testing.T) {
		t.Parallel()
		rapid.Check(t, func(t *rapid.T) {
			xs := rapid.SliceOfN(nameGen(), 1, 5).Draw(t, "xs")
			parts := make([]string, len(xs))
			for i, x := range xs {
				parts[i] = naming.SanitizePart(x)
			}
			joined := strings.Join(parts, "/")
			if got := strings.Split(joined, "/"); !slices.Equal(got, parts) {
				t.Fatalf("names %q joined as %q split into %q", xs, joined, got)
			}
			if cleaned := path.Clean(joined); cleaned != joined {
				t.Fatalf("names %q joined as %q, path.Clean gives %q", xs, joined, cleaned)
			}
		})
	})
}

func runeGen() *rapid.Generator[rune] {
	special := []rune{
		'/', '\\', ':', '*', '?', '"', '<', '>', '|', '.', ' ',
		0x00, '\t', 0x1f, 0x7f,
		'и', '̆', 'е', '̈', '́', '̸',
		'क़', 'Ω', '∕', '．', ' ', 'я', '\U0001f600',
		'a', 'C', 'O', 'N', '¹', '_',
	}
	return rapid.OneOf(rapid.SampledFrom(special), rapid.Rune())
}

func reservedGen() *rapid.Generator[string] {
	stems := rapid.SampledFrom([]string{
		"CON", "con", "Con", "PRN", "AUX", "NUL", "COM1", "com9", "LPT0", "COM¹", "lpt³",
	})
	tails := rapid.SampledFrom([]string{"", ".", ".heic", ".tar.gz", " ", "x"})
	return rapid.Custom(func(t *rapid.T) string {
		return stems.Draw(t, "stem") + tails.Draw(t, "tail")
	})
}

func nameGen() *rapid.Generator[string] {
	return rapid.OneOf(rapid.StringOfN(runeGen(), 0, 300, -1), reservedGen())
}
