package naming

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// SanitizePart returns s as one level of a cloud path. The result always
// satisfies IsValidPart, a valid s is returned unchanged, and the NFC and NFD
// forms of a name give the same result. Bytes that are not UTF-8 become
// U+FFFD. The name is cut to 255 bytes without splitting a letter from its
// combining marks. An s with nothing left is "_". There is no inverse: a name
// read from the cloud is used as it is.
func SanitizePart(s string) string {
	// Controls go before NFC so that the marks they separated compose; NFC
	// goes before the look-alikes so that both forms of "≮" give one name.
	s = strings.Map(dropControl, s)
	s = norm.NFC.String(s)
	s = lookalikes.Replace(s)
	s = fit(s)
	if s == "" {
		return "_"
	}
	return s
}

// Windows refuses U+0000–U+001F and accepts U+007F. The U+FFFD for a byte
// that is not UTF-8 comes from strings.Map.
func dropControl(r rune) rune {
	if r < 0x20 {
		return -1
	}
	return r
}

// finish runs on the cut prefix, not before the cut: a cut can expose a final
// dot or leave only a reserved stem. It lengthens the name, so the length is
// checked after it.
func fit(s string) string {
	p := cut(s, maxNameBytes)
	for len(finish(p)) > maxNameBytes {
		p = cut(p, len(p)-1)
	}
	return finish(p)
}

// A letter cut off from its combining marks would read as another letter.
func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && (!utf8.RuneStart(s[n]) || startsWithMark(s[n:])) {
		n--
	}
	return s[:n]
}

func startsWithMark(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsMark(r)
}

// The stem ends at the first dot, so it is checked before the edges change
// that dot.
func finish(s string) string {
	return edges(escapeReserved(s))
}

// Windows drops the spaces at the end of the stem before it looks for a device.
func escapeReserved(s string) string {
	stem, _, _ := strings.Cut(s, ".")
	if slices.Contains(reservedStems, strings.ToUpper(strings.TrimRight(stem, " "))) {
		return stem + "_" + s[len(stem):]
	}
	return s
}

func edges(s string) string {
	if rest, ok := strings.CutPrefix(s, "."); ok {
		s = dotLookalike + rest
	}
	if rest, ok := strings.CutSuffix(s, "."); ok {
		return rest + dotLookalike
	}
	if rest, ok := strings.CutSuffix(s, " "); ok {
		return rest + spaceLookalike
	}
	return s
}
