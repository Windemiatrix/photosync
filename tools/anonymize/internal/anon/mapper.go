package anon

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// ErrUnsupported reports a value outside the domain the mapping preserves
// (section "not supported" of the selection rules). It is fatal when the
// mapping meets it: the unit check runs before the mapping.
var ErrUnsupported = errors.New("unsupported value")

// ErrExhausted reports that no free output was found for a value within the
// attempt limit. Word pseudonyms keep the length over 20 ASCII letters, so
// the space of short ASCII words is small (about 400 two-letter words). The
// build and verifyMaterials skip a unit that runs out of space; the mapping
// stays injective because the registered outputs are kept.
var ErrExhausted = errors.New("mapping space exhausted")

// maxAttempts bounds the attempts to find a free output for one value.
const maxAttempts = 1_000_000

// Purposes of the derived keys.
const (
	purposeWord   = "word"
	purposeDigits = "digits"
	purposeID     = "id"
	purposeSHA1   = "sha1"
	purposeSize   = "size"
	purposeYear   = "year"
	purposePick   = "pick"
)

// Year range the mapping shifts and the epoch years it keeps.
const (
	minShiftedYear = 1971
	maxShiftedYear = 2027
	minYear        = 1969
)

// Output alphabets of the word pseudonyms by the class of a cluster.
var (
	alphaASCII        = []rune("ghijklmnopqrstuvwxyz")
	alphaDecomposable = []rune("йёéèêëáàâäóòôöúü")
	alphaOther        = []rune("абвгдежзиклмнопрстуфхцчшщъыьэюя")
)

// macFunc derives a key: HMAC-SHA-256(salt, purpose ‖ 0x00 ‖ input ‖ counter)
// with counter as 8 big-endian bytes (the attempt in the high 32 bits, the
// block of a longer stream in the low 32 bits).
type macFunc func(purpose string, input []byte, counter uint64) [32]byte

func hmacFunc(salt []byte) macFunc {
	key := append([]byte(nil), salt...)
	return func(purpose string, input []byte, counter uint64) [32]byte {
		h := hmac.New(sha256.New, key)
		h.Write([]byte(purpose))
		h.Write([]byte{0})
		h.Write(input)
		var c [8]byte
		binary.BigEndian.PutUint64(c[:], counter)
		h.Write(c[:])
		var out [32]byte
		copy(out[:], h.Sum(nil))
		return out
	}
}

// registry keeps an injective mapping of keys to outputs.
type registry struct {
	out   map[string]string // key -> output
	owner map[string]string // output -> key
}

func newRegistry() *registry {
	return &registry{out: map[string]string{}, owner: map[string]string{}}
}

// Mapper is the deterministic keyed mapping of one run. The same input always
// gives the same output; outputs of different inputs never coincide. The
// order of calls decides only how collisions are resolved, so callers keep a
// fixed order.
type Mapper struct {
	mac    macFunc
	limit  uint32 // attempts per value; maxAttempts outside tests
	shift  int
	sizeA  int64
	sizeC  int64
	words  *registry
	digits map[int]*registry // by length
	ids    *registry
	hashes *registry
}

// NewMapper returns the mapping keyed by a 32-byte salt.
func NewMapper(salt []byte) (*Mapper, error) {
	if len(salt) != 32 {
		return nil, fmt.Errorf("salt: %d bytes, want 32", len(salt))
	}
	return newMapperMAC(hmacFunc(salt)), nil
}

func newMapperMAC(mac macFunc) *Mapper {
	m := &Mapper{mac: mac, limit: maxAttempts, words: newRegistry(), digits: map[int]*registry{}, ids: newRegistry(), hashes: newRegistry()}
	y := mac(purposeYear, nil, 0)
	m.shift = 4 * (7 + int(y[0])%12)
	s := mac(purposeSize, nil, 0)
	// a >= 1: with a = 0 the differences of sizes would be kept.
	m.sizeA = 1 + int64(binary.BigEndian.Uint16(s[0:2]))
	m.sizeC = 1 + int64(binary.BigEndian.Uint16(s[2:4])%4096)
	return m
}

// YearShift returns the date shift in years: 4k with k in 7..18, that is
// 1461k days, which keeps the calendar date and the time of day in 1901-2099.
func (m *Mapper) YearShift() int { return m.shift }

// pick returns the selection key of a unit.
func (m *Mapper) pick(category, key string) [32]byte {
	return m.mac(purposePick, []byte(category+"\x00"+key), 0)
}

// stream returns n derived bytes for one attempt.
func (m *Mapper) stream(purpose string, input []byte, attempt uint32, n int) []byte {
	out := make([]byte, 0, n+32)
	for block := uint32(0); len(out) < n; block++ {
		s := m.mac(purpose, input, uint64(attempt)<<32|uint64(block))
		out = append(out, s[:]...)
	}
	return out[:n]
}

// unique returns the registered output of key or finds a free one: gen makes
// the candidate of an attempt, accept rejects reserved candidates.
func (m *Mapper) unique(r *registry, key string, gen func(attempt uint32) string, accept func(string) bool) (string, error) {
	if out, ok := r.out[key]; ok {
		return out, nil
	}
	for attempt := uint32(0); attempt < m.limit; attempt++ {
		c := gen(attempt)
		if _, taken := r.owner[c]; taken {
			continue
		}
		if accept != nil && !accept(c) {
			continue
		}
		r.out[key], r.owner[c] = c, key
		return c, nil
	}
	return "", ErrExhausted
}

var (
	uuidRE   = regexp.MustCompile(`[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}`)
	hexRunRE = regexp.MustCompile(`[0-9A-Fa-f]{16,}`)
)

// Text maps free text (a stem, a path segment, an album title): UUIDs and
// long hexadecimal runs become marked identifiers; then the text is cut into
// letter, digit and other runs and each run is mapped by its own rule (the
// structure of names, events and albums is kept because separators, short
// numbers and structural words stay as they are).
func (m *Mapper) Text(s string) (string, error) {
	if rule := textRule(s); rule != "" {
		return "", ErrUnsupported
	}
	var b strings.Builder
	last := 0
	for _, loc := range uuidRE.FindAllStringIndex(s, -1) {
		if err := m.textHex(&b, s[last:loc[0]]); err != nil {
			return "", err
		}
		id, err := m.ID(s[loc[0]:loc[1]])
		if err != nil {
			return "", err
		}
		b.WriteString(id)
		last = loc[1]
	}
	if err := m.textHex(&b, s[last:]); err != nil {
		return "", err
	}
	return b.String(), nil
}

// textHex maps the hexadecimal runs of 16 and more characters with a letter
// and a digit, then the runs of the rest.
func (m *Mapper) textHex(b *strings.Builder, s string) error {
	last := 0
	for _, loc := range hexRunRE.FindAllStringIndex(s, -1) {
		run := s[loc[0]:loc[1]]
		if !strings.ContainsAny(run, "0123456789") || !strings.ContainsAny(run, "abcdefABCDEF") {
			continue
		}
		if err := m.textRuns(b, s[last:loc[0]]); err != nil {
			return err
		}
		h, err := m.hexID(run)
		if err != nil {
			return err
		}
		b.WriteString(h)
		last = loc[1]
	}
	return m.textRuns(b, s[last:])
}

func (m *Mapper) textRuns(b *strings.Builder, s string) error {
	runs := splitRuns(s)
	for i, r := range runs {
		switch r.class {
		case runLetters:
			w, err := m.word(r.text)
			if err != nil {
				return err
			}
			b.WriteString(w)
		case runDigits:
			keptTime := isKeptTime(r.text) && i >= 2 && runs[i-1].text == "_" && runs[i-2].class == runDigits && isKeptDate(runs[i-2].text)
			d, err := m.number(r.text, keptTime)
			if err != nil {
				return err
			}
			b.WriteString(d)
		default:
			b.WriteString(r.text)
		}
	}
	return nil
}

// isKeptTime reports whether a digit run after a kept date is a time the
// mapping keeps: a valid HHMMSS or HHMMSSmmm (the PXL_ and Android names).
// Any other run is mapped, so a number after a date that is not a time of
// day never reaches the output.
func isKeptTime(s string) bool {
	if len(s) != 6 && len(s) != 9 {
		return false
	}
	_, err := time.Parse("150405", s[:6])
	return err == nil
}

// isKeptDate reports whether an 8-digit run is a date the mapping shifts or
// keeps: YYYYMMDD with the year in 1969..2027.
func isKeptDate(s string) bool {
	y, ok := compactDateYear(s)
	return ok && y >= minYear && y <= maxShiftedYear
}

// compactDateYear returns the year of a valid YYYYMMDD date.
func compactDateYear(s string) (int, bool) {
	if len(s) != 8 {
		return 0, false
	}
	d, err := time.Parse("20060102", s)
	if err != nil {
		return 0, false
	}
	return d.Year(), true
}

// number maps an ASCII digit run; keptTime marks the time after a kept date.
func (m *Mapper) number(s string, keptTime bool) (string, error) {
	if len(s) <= 2 || keptTime {
		return s, nil
	}
	if y, ok := compactDateYear(s); ok && y >= minYear && y <= maxShiftedYear {
		if y < minShiftedYear {
			return s, nil
		}
		return strconv.Itoa(y+m.shift) + s[4:], nil
	}
	r := m.digits[len(s)]
	if r == nil {
		r = newRegistry()
		m.digits[len(s)] = r
	}
	gen := func(attempt uint32) string {
		raw := m.stream(purposeDigits, []byte(s), attempt, len(s))
		out := make([]byte, len(s))
		for i, c := range raw {
			out[i] = '0' + c%10
		}
		return string(out)
	}
	// An 8-digit output never looks like a shifted or an epoch date.
	accept := func(c string) bool {
		y, ok := compactDateYear(c)
		if !ok {
			return true
		}
		reserved := y >= 1999 && y <= 2099 || y == 1969 || y == 1970
		return !reserved
	}
	return m.unique(r, s, gen, accept)
}

// clusters cuts a letter run into clusters: a rune and the marks after it.
func clusters(s string) []string {
	var out []string
	start := 0
	for i, r := range s {
		if i > start && !unicode.Is(unicode.M, r) {
			out = append(out, s[start:i])
			start = i
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// word maps a letter run: one cluster and structural words stay, any other
// word becomes a pseudonym of the same length in clusters with the same case
// and form cluster by cluster.
func (m *Mapper) word(w string) (string, error) {
	in := clusters(w)
	if len(in) == 1 || structuralSet[strings.ToLower(w)] {
		return w, nil
	}
	key := lowerNFC(w)
	kc := clusters(key)
	if len(kc) != len(in) {
		return "", ErrUnsupported
	}
	base, err := m.unique(m.words, key, func(attempt uint32) string {
		raw := m.stream(purposeWord, []byte(key), attempt, len(kc))
		var b strings.Builder
		for i, c := range kc {
			alpha := alphaOther
			switch {
			case len(c) == 1 && c[0] < utf8.RuneSelf:
				alpha = alphaASCII
			case decomposable(c):
				alpha = alphaDecomposable
			}
			b.WriteRune(alpha[int(raw[i])%len(alpha)])
		}
		return b.String()
	}, func(c string) bool { return !AllowedWord(c) })
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for i, r := range []rune(base) {
		c := in[i]
		first, _ := utf8.DecodeRuneInString(c)
		if unicode.IsUpper(first) || unicode.IsTitle(first) {
			r = unicode.ToUpper(r)
		}
		if c != nfc(c) {
			b.WriteString(nfd(string(r)))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String(), nil
}

// Words returns the sorted base forms (lower case, NFC) of all generated
// words.
func (m *Mapper) Words() []string {
	out := make([]string, 0, len(m.words.owner))
	for w := range m.words.owner {
		out = append(out, w)
	}
	sort.Strings(out)
	return out
}

// Name maps a file name: the stem by the text rule, the extension as it is
// when it is a known media or sidecar extension, by the text rule otherwise.
func (m *Mapper) Name(s string) (string, error) {
	stem, ext := Splitext(s)
	ms, err := m.Text(stem)
	if err != nil {
		return "", err
	}
	if ext == "" || isKnownExt(ext[1:]) {
		return ms + ext, nil
	}
	me, err := m.Text(ext)
	if err != nil {
		return "", err
	}
	return ms + me, nil
}

func isKnownExt(e string) bool {
	e = strings.ToLower(e)
	for _, k := range knownExts {
		if k == e {
			return true
		}
	}
	return false
}

// year maps a year segment: 1971..2027 shifted, 1969 and 1970 kept.
func (m *Mapper) year(s string) (string, error) {
	y, err := strconv.Atoi(s)
	if err != nil || y < minYear || y > maxShiftedYear {
		return "", ErrUnsupported
	}
	if y < minShiftedYear {
		return s, nil
	}
	return strconv.Itoa(y + m.shift), nil
}

// Path maps a cloud path segment by segment: a first folder of exactly four
// digits is a year, the folder below it is an event (it must not start with
// three digits), the last segment is a file name, the rest is text.
func (m *Mapper) Path(p string) (string, error) {
	seg := strings.Split(p, "/")
	out := make([]string, len(seg))
	yearFolder := false
	for i, s := range seg {
		var err error
		switch {
		case i == len(seg)-1:
			out[i], err = m.Name(s)
		case i == 0 && fourDigits.MatchString(s):
			yearFolder = true
			out[i], err = m.year(s)
		case i == 1 && yearFolder:
			if eventDigitsRE.MatchString(s) {
				return "", ErrUnsupported
			}
			out[i], err = m.Text(s)
		default:
			out[i], err = m.Text(s)
		}
		if err != nil {
			return "", err
		}
	}
	return strings.Join(out, "/"), nil
}

// Title maps an album title by the text rule.
func (m *Mapper) Title(s string) (string, error) { return m.Text(s) }

// Folder maps one element of folder_names: four digits are a year, anything
// else is text.
func (m *Mapper) Folder(s string) (string, error) {
	if fourDigits.MatchString(s) {
		return m.year(s)
	}
	return m.Text(s)
}

// Date maps an ISO 8601 date: the year is shifted (1969 and 1970 are kept),
// every other byte stays. "" stays "".
func (m *Mapper) Date(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	if _, ok := ParseDT(s); !ok {
		return "", ErrUnsupported
	}
	y, err := m.year(s[:4])
	if err != nil {
		return "", err
	}
	return y + s[4:], nil
}

// ID maps an identifier (uuid, burst_key, content_id) case-insensitively. A
// UUID becomes 00000000-XXXX-8XXX-YXXX-XXXXXXXXXXXX, anything else becomes
// 00000000 and 24 hex digits; the case of the hex digits follows the input.
func (m *Mapper) ID(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	key := strings.ToUpper(s)
	isUUID := uuidRE.MatchString(s) && len(s) == 36
	out, err := m.unique(m.ids, key, func(attempt uint32) string {
		raw := m.stream(purposeID, []byte(key), attempt, 16)
		h := strings.ToUpper(hex.EncodeToString(raw))
		if !isUUID {
			return "00000000" + h[:24]
		}
		variant := "89AB"[raw[15]%4]
		return "00000000-" + h[0:4] + "-8" + h[4:7] + "-" + string(variant) + h[7:10] + "-" + h[10:22]
	}, nil)
	if err != nil {
		return "", err
	}
	return matchCase(s, out), nil
}

// hexID maps a long hexadecimal run of a name to 00000000 and hex digits of
// the same total length.
func (m *Mapper) hexID(s string) (string, error) {
	key := "hex\x00" + strings.ToUpper(s)
	out, err := m.unique(m.ids, key, func(attempt uint32) string {
		raw := m.stream(purposeID, []byte(key), attempt, (len(s)-8+1)/2)
		return "00000000" + strings.ToUpper(hex.EncodeToString(raw))[:len(s)-8]
	}, nil)
	if err != nil {
		return "", err
	}
	return matchCase(s, out), nil
}

// matchCase returns out in lower case when the input has a lower-case hex
// letter and no upper-case one.
func matchCase(in, out string) string {
	if strings.ContainsAny(in, "abcdef") && !strings.ContainsAny(in, "ABCDEF") {
		return strings.ToLower(out)
	}
	return out
}

// Hash maps a SHA-1: 00000000 and 32 lower-case hex digits; "" stays "".
func (m *Mapper) Hash(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	key := strings.ToLower(s)
	return m.unique(m.hashes, key, func(attempt uint32) string {
		return "00000000" + hex.EncodeToString(m.stream(purposeSHA1, []byte(key), attempt, 16))
	}, nil)
}

// Size maps a file size: 0 stays 0, any other size n becomes
// n + n·a/2^18 + c with a in [1, 2^16] and c in [1, 4096]. The map is
// strictly increasing, so equality and order are kept while the exact bytes
// and differences are not.
func (m *Mapper) Size(n int64) int64 {
	if n <= 0 {
		return n
	}
	return n + n*m.sizeA>>18 + m.sizeC
}

// Asset maps a catalog record field by field in the order of the struct.
func (m *Mapper) Asset(a Asset) (Asset, error) {
	var err error
	out := a
	out.InTrash = false
	if out.UUID, err = m.ID(a.UUID); err != nil {
		return Asset{}, err
	}
	if out.OriginalFilename, err = m.Name(a.OriginalFilename); err != nil {
		return Asset{}, err
	}
	if out.Filename, err = m.Name(a.Filename); err != nil {
		return Asset{}, err
	}
	if out.Date, err = m.Date(a.Date); err != nil {
		return Asset{}, err
	}
	out.OriginalFilesize = m.Size(a.OriginalFilesize)
	if out.BurstKey, err = m.ID(a.BurstKey); err != nil {
		return Asset{}, err
	}
	out.Albums = make([]AlbumRef, len(a.Albums))
	for i, al := range a.Albums {
		names := make([]string, len(al.FolderNames))
		for j, f := range al.FolderNames {
			if names[j], err = m.Folder(f); err != nil {
				return Asset{}, err
			}
		}
		title, err := m.Title(al.Title)
		if err != nil {
			return Asset{}, err
		}
		out.Albums[i] = AlbumRef{FolderNames: names, Title: title}
	}
	return out, nil
}

// File maps an inventory record field by field in the order of the struct.
func (m *Mapper) File(f File) (File, error) {
	var err error
	out := f
	if out.Path, err = m.Path(f.Path); err != nil {
		return File{}, err
	}
	out.Size = m.Size(f.Size)
	if out.DateTaken, err = m.Date(f.DateTaken); err != nil {
		return File{}, err
	}
	if out.ContentID, err = m.ID(f.ContentID); err != nil {
		return File{}, err
	}
	if out.SHA1, err = m.Hash(f.SHA1); err != nil {
		return File{}, err
	}
	return out, nil
}

// Set maps a set of records in order: assets, then files.
func (m *Mapper) Set(s *Set) (*Set, error) {
	out := &Set{Assets: make([]Asset, len(s.Assets)), Files: make([]File, len(s.Files)), Src: append([]int(nil), s.Src...)}
	for i, a := range s.Assets {
		ma, err := m.Asset(a)
		if err != nil {
			return nil, err
		}
		out.Assets[i] = ma
	}
	for i, f := range s.Files {
		mf, err := m.File(f)
		if err != nil {
			return nil, err
		}
		out.Files[i] = mf
	}
	return out, nil
}
