package anon

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	maxCaseRecords = 60
	maxPicks       = 8
	maxStemGroup   = 10
)

// Rules of "not supported": a record that breaks one is excluded together
// with its unit. Each rule keeps a value whose image would either lose a
// relation the reference script depends on or trip the leak scanner.
const (
	RuleDateYear      = "date-year"       // a date year outside 1969..2027
	RuleDateParse     = "date-parse"      // a non-empty date that does not parse
	RuleYearSegment   = "year-segment"    // a year folder or folder_names year outside 1969..2027
	RuleEventDigits   = "event-digits"    // an event folder starting with 3 digits or NN-NNN
	RuleChar          = "char"            // "@", "\", "°" or a control character
	RuleLeadingMark   = "leading-mark"    // a combining mark at the start of a run
	RuleNonASCIIDigit = "non-ascii-digit" // a decimal digit outside 0-9
	RuleRootPath      = "root-path"       // a string starting with "/" or "~"
	RuleDrive         = "drive-letter"    // a string starting with "X:/" or "X:\"
	RuleDecimal       = "decimal"         // a number like 55.7539 outside dates
	RuleFix1970       = "fix-1970"        // an invalid 1970 fix of a PXL_ name, or a fixed year outside 1969..2027
	RuleNegativeSize  = "negative-size"   // a negative file size
	RuleCaseFold      = "case-fold"       // "İ", "Σ", "ς" or "i" + U+0307: lower case differs between Python and Go
	RuleClusters      = "clusters"        // a letter run whose lower NFC form has another cluster count
	RuleEpochGap      = "epoch-gap"       // a 1969-1970 date within 14 h 2 s of 1971, where shifted dates start
)

// Epoch dates from lateEpoch on are not supported, so a case never breaks the
// epoch_gap relation.
var lateEpoch = time.Date(minShiftedYear, 1, 1, 0, 0, 0, 0, time.UTC).Add(-epochGap)

func epochRule(t time.Time) string {
	if (t.Year() == 1969 || t.Year() == 1970) && !t.Before(lateEpoch) {
		return RuleEpochGap
	}
	return ""
}

var (
	eventDigitsRE = regexp.MustCompile(`^(?:[0-9]{3}|[0-9]{2}-[0-9]{3})`)
	decimalRE     = regexp.MustCompile(`[0-9]{1,3}\.[0-9]{2,}`)
	driveRE       = regexp.MustCompile(`^[A-Za-z]:[\\/]`)
)

func textRule(s string) string {
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~") {
		return RuleRootPath
	}
	if driveRE.MatchString(s) {
		return RuleDrive
	}
	prevLetter, prev := false, rune(0)
	for _, r := range s {
		switch {
		case r == '@' || r == '\\' || r == '°' || r < 0x20 || r == 0x7f:
			return RuleChar
		case unicode.Is(unicode.M, r):
			if !prevLetter {
				return RuleLeadingMark
			}
			if r == '̇' && (prev == 'i' || prev == 'I') {
				// The Python lower case of "İ": Go lowers "İ" to "i".
				return RuleCaseFold
			}
			prev = r
			continue
		case unicode.Is(unicode.Nd, r) && (r < '0' || r > '9'):
			return RuleNonASCIIDigit
		case r == 'İ' || r == 'Σ' || r == 'ς':
			// Python lowers "İ" to "i̇" and "Σ" at a word end to "ς"; Go to
			// "i" and "σ". With "ς" not supported either, a Python key with
			// "ς" or "i̇" comes only from strings that are dropped.
			return RuleCaseFold
		}
		prevLetter, prev = unicode.IsLetter(r), r
	}
	if decimalRE.MatchString(s) {
		return RuleDecimal
	}
	for _, run := range letterRuns(s) {
		if len(clusters(lowerNFC(run))) != len(clusters(run)) {
			return RuleClusters
		}
	}
	return ""
}

func dateRule(s string) string {
	if s == "" {
		return ""
	}
	t, ok := ParseDT(s)
	if !ok {
		return RuleDateParse
	}
	if t.Year() < minYear || t.Year() > maxShiftedYear {
		return RuleDateYear
	}
	return ""
}

func yearRule(s string) string {
	if !fourDigits.MatchString(s) {
		return ""
	}
	if y, _ := strconv.Atoi(s); y < minYear || y > maxShiftedYear {
		return RuleYearSegment
	}
	return ""
}

// AssetRule returns the "not supported" rule an asset breaks, or "". A 1970
// fix the mapping does not keep is left to the relation check of the pick.
func AssetRule(a Asset) string {
	if r := cmp.Or(dateRule(a.Date), textRule(a.OriginalFilename), textRule(a.Filename)); r != "" {
		return r
	}
	for _, al := range a.Albums {
		if r := textRule(al.Title); r != "" {
			return r
		}
		for _, f := range al.FolderNames {
			if r := cmp.Or(textRule(f), yearRule(f)); r != "" {
				return r
			}
		}
	}
	if dt, ok := ParseDT(a.Date); ok {
		fixed, applies, invalid := Fix1970(a.Name(), dt, a.TZOffset)
		if invalid || applies && (fixed.Year() < minYear || fixed.Year() > maxShiftedYear) {
			return RuleFix1970
		}
		if applies {
			dt = fixed
		}
		if r := epochRule(dt); r != "" {
			return r
		}
	}
	if a.OriginalFilesize < 0 {
		return RuleNegativeSize
	}
	return ""
}

// FileRule returns the "not supported" rule an inventory row breaks, or "".
func FileRule(f File) string {
	if r := textRule(f.Path); r != "" {
		return r
	}
	seg := strings.Split(f.Path, "/")
	for i, s := range seg {
		if r := textRule(s); r != "" {
			return r
		}
		if i == len(seg)-1 {
			break
		}
		if i == 0 {
			if r := yearRule(s); r != "" {
				return r
			}
		}
		if i == 1 && fourDigits.MatchString(seg[0]) && eventDigitsRE.MatchString(s) {
			return RuleEventDigits
		}
	}
	if r := dateRule(f.DateTaken); r != "" {
		return r
	}
	if dt, ok := ParseDT(f.DateTaken); ok {
		if r := epochRule(dt); r != "" {
			return r
		}
	}
	if f.Size < 0 {
		return RuleNegativeSize
	}
	return ""
}

type pairRef struct {
	file  int
	level string
}

// A component holds an asset with all its linked files, a file with its
// owner and a file with all files of the same neighbor key. A union of
// components is closed for the reference matching: the reference script, run
// on it, gives exactly the links of the full run restricted to it. A file
// linked in the full run enters only with its owner, so the first captures of
// phases 1-2 and the greedy assignment repeat; two records left unlinked by
// the full run do not fit each other under the reference rules (otherwise the
// full run would have linked them); and the "last MOV" of a (folder,
// lower(stem)) key is held by the neighbors.
type graph struct {
	m        *Material
	nA       int
	excluded []bool
	rules    []string
	owner    []int // per file record: the owning asset record or -1
	links    [][]pairRef
	comp     []int // -1 when excluded
	members  [][]int
	plan     planIndex
	// exhausted holds the first records of the units the mapping ran out
	// of space for; they are never picked again.
	exhausted map[int]bool
	// broken holds the first records of the units whose image breaks a
	// relation, with the first broken relation; they are never picked again.
	broken map[int]string
}

// newGraph indexes a material that passed the partition check.
func newGraph(m *Material) *graph {
	nA, n := len(m.Assets), len(m.Assets)+len(m.Files)
	g := &graph{m: m, nA: nA, excluded: make([]bool, n), rules: make([]string, n), owner: make([]int, n),
		links: make([][]pairRef, nA), comp: make([]int, n), plan: newPlanIndex(m.Plan), exhausted: map[int]bool{}, broken: map[int]string{}}
	fileIdx := map[string]int{}
	for j, f := range m.Files {
		i := nA + j
		g.owner[i] = -1
		if f.Kind == KindOther {
			g.excluded[i] = true
			continue
		}
		g.rules[i] = FileRule(f)
		if _, dup := fileIdx[f.Path]; !dup {
			fileIdx[f.Path] = i
		}
	}
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	find := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra < rb {
			parent[rb] = ra
		} else if rb < ra {
			parent[ra] = rb
		}
	}
	for i, a := range m.Assets {
		if a.InTrash {
			g.excluded[i] = true
			continue
		}
		g.rules[i] = AssetRule(a)
		for _, l := range m.Links[a.UUID] {
			fi := fileIdx[l.Path]
			level, _ := LevelOf(l.Kind)
			g.owner[fi] = i
			g.links[i] = append(g.links[i], pairRef{fi, level})
			union(i, fi)
		}
	}
	firstByKey := map[string]int{}
	for j, f := range m.Files {
		i := nA + j
		if g.excluded[i] {
			continue
		}
		k := NeighborKey(f.Path)
		if first, ok := firstByKey[k]; ok {
			union(first, i)
		} else {
			firstByKey[k] = i
		}
	}
	compOfRoot := map[int]int{}
	for i := 0; i < n; i++ {
		if g.excluded[i] {
			g.comp[i] = -1
			continue
		}
		r := find(i)
		c, ok := compOfRoot[r]
		if !ok {
			c = len(g.members)
			compOfRoot[r] = c
			g.members = append(g.members, nil)
		}
		g.comp[i] = c
		g.members[c] = append(g.members[c], i)
	}
	return g
}

func (g *graph) componentsOf(seed []int) []int {
	seen := map[int]bool{}
	var out []int
	for _, i := range seed {
		if c := g.comp[i]; c >= 0 && !seen[c] {
			seen[c] = true
			out = append(out, g.members[c]...)
		}
	}
	slices.Sort(out)
	return out
}

func (g *graph) isAsset(i int) bool { return i < g.nA }
func (g *graph) asset(i int) Asset  { return g.m.Assets[i] }
func (g *graph) file(i int) File    { return g.m.Files[i-g.nA] }

func (g *graph) name(i int) string {
	if g.isAsset(i) {
		return g.asset(i).Name()
	}
	return BaseName(g.file(i).Path)
}

func (g *graph) key(i int) string {
	if g.isAsset(i) {
		return g.asset(i).UUID
	}
	return g.file(i).Path
}

func (g *graph) supported(unit []int) bool {
	if len(unit) == 0 || len(unit) > maxCaseRecords {
		return false
	}
	for _, i := range unit {
		if g.rules[i] != "" {
			return false
		}
	}
	return true
}

// Category is one fixture category: a predicate over a unit, or feature
// groups whose records seed a union unit.
type Category struct {
	Name   string
	pred   func(g *graph, unit []int) bool
	groups func(g *graph) [][]int
}

func (g *graph) candidates(c Category) [][]int {
	var out [][]int
	seen := map[int]bool{}
	add := func(u []int) {
		if g.supported(u) && !seen[u[0]] {
			seen[u[0]] = true
			out = append(out, u)
		}
	}
	if c.pred != nil {
		for _, u := range g.members {
			if c.pred(g, u) {
				add(u)
			}
		}
		return out
	}
	for _, grp := range c.groups(g) {
		add(g.componentsOf(grp))
	}
	return out
}

// The pick order is keyed, neither by date nor by name, and at most half of
// the candidates are taken, so a case never shows how many units of a
// category the library has. A skipped unit keeps the outputs it registered, so
// the mapping stays injective and the mapping of the whole case only repeats
// registered outputs.
func (g *graph) pickUnits(m *Mapper, category string, cands [][]int) ([]int, error) {
	type cand struct {
		key  [32]byte
		unit []int
	}
	cs := make([]cand, len(cands))
	for i, u := range cands {
		cs[i] = cand{m.pick(category, g.key(u[0])), u}
	}
	sort.Slice(cs, func(i, j int) bool {
		if c := bytes.Compare(cs[i].key[:], cs[j].key[:]); c != 0 {
			return c < 0
		}
		return cs[i].unit[0] < cs[j].unit[0]
	})
	n := min(maxPicks, (len(cs)+1)/2)
	chosen := map[int]bool{}
	taken := 0
	for _, c := range cs {
		if taken == n {
			break
		}
		extra := 0
		for _, i := range c.unit {
			if !chosen[i] {
				extra++
			}
		}
		if len(chosen)+extra > maxCaseRecords || g.exhausted[c.unit[0]] || g.broken[c.unit[0]] != "" {
			continue
		}
		sub := g.m.Subset(c.unit)
		img, err := m.Set(sub)
		if errors.Is(err, ErrExhausted) {
			g.exhausted[c.unit[0]] = true
			continue
		}
		if err != nil {
			return nil, err
		}
		if vs := CheckRelations(sub, img, m.YearShift()); len(vs) > 0 {
			g.broken[c.unit[0]] = vs[0].Relation
			continue
		}
		for _, i := range c.unit {
			chosen[i] = true
		}
		taken++
	}
	return slices.Sorted(maps.Keys(chosen)), nil
}

func anyRecord(f func(g *graph, i int) bool) func(*graph, []int) bool {
	return func(g *graph, u []int) bool {
		for _, i := range u {
			if f(g, i) {
				return true
			}
		}
		return false
	}
}

func anyAsset(f func(a Asset) bool) func(*graph, []int) bool {
	return anyRecord(func(g *graph, i int) bool { return g.isAsset(i) && f(g.asset(i)) })
}

func anyFile(f func(fl File) bool) func(*graph, []int) bool {
	return anyRecord(func(g *graph, i int) bool { return !g.isAsset(i) && f(g.file(i)) })
}

func anyPair(f func(g *graph, a Asset, fl File, level string) bool) func(*graph, []int) bool {
	return anyRecord(func(g *graph, i int) bool {
		if !g.isAsset(i) {
			return false
		}
		for _, p := range g.links[i] {
			if f(g, g.asset(i), g.file(p.file), p.level) {
				return true
			}
		}
		return false
	})
}

func hasLevel(level string) func(*graph, []int) bool {
	return anyPair(func(_ *graph, _ Asset, _ File, l string) bool { return l == level })
}

func hasPlan(decision string) func(*graph, []int) bool {
	return anyPair(func(g *graph, a Asset, f File, _ string) bool { return g.plan.decision(a.UUID, f.Path) == decision })
}

func stemLower(name string) string {
	s, _ := Splitext(name)
	return strings.ToLower(s)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func (g *graph) recordStrings(i int) []string {
	if !g.isAsset(i) {
		return []string{g.file(i).Path}
	}
	a := g.asset(i)
	out := []string{a.OriginalFilename, a.Filename}
	for _, al := range a.Albums {
		out = append(out, al.Title)
		out = append(out, al.FolderNames...)
	}
	return out
}

func predLive(g *graph, u []int) bool {
	for _, i := range u {
		if !g.isAsset(i) || !g.asset(i).LivePhoto {
			continue
		}
		for _, p := range g.links[i] {
			if ExtLower(g.file(p.file).Path) == "mov" {
				return true
			}
		}
	}
	return false
}

func predRawPair(g *graph, u []int) bool {
	raw, other := map[string]bool{}, map[string]bool{}
	for _, i := range u {
		if g.isAsset(i) {
			if g.asset(i).HasRaw {
				return true
			}
			continue
		}
		f := g.file(i)
		if !IsMedia(f.Kind) {
			continue
		}
		k := NeighborKey(f.Path)
		if IsRawExt(ExtLower(f.Path)) {
			raw[k] = true
		} else {
			other[k] = true
		}
	}
	for k := range raw {
		if other[k] {
			return true
		}
	}
	return false
}

func predManyFolders(g *graph, u []int) bool {
	for _, i := range u {
		if !g.isAsset(i) {
			continue
		}
		folders := map[string]bool{}
		for _, p := range g.links[i] {
			folders[FolderOf(g.file(p.file).Path)] = true
		}
		if len(folders) >= 2 {
			return true
		}
	}
	return false
}

func predYearBoundary(g *graph, u []int) bool {
	for _, i := range u {
		if !g.isAsset(i) {
			continue
		}
		dt, ok := ParseDT(g.asset(i).Date)
		if !ok {
			continue
		}
		if md := dt.Format("01-02"); md == "12-31" || md == "01-01" {
			return true
		}
		for _, p := range g.links[i] {
			if y := FolderInfoOf(g.file(p.file).Path).Year; y != "" && y != strconv.Itoa(dt.Year()) {
				return true
			}
		}
	}
	return false
}

func groupBy(g *graph, key func(i int) (string, bool)) [][]int {
	idx := map[string]int{}
	var groups [][]int
	for i := range g.comp {
		if g.excluded[i] {
			continue
		}
		k, ok := key(i)
		if !ok {
			continue
		}
		gi, seen := idx[k]
		if !seen {
			gi = len(groups)
			idx[k] = gi
			groups = append(groups, nil)
		}
		groups[gi] = append(groups[gi], i)
	}
	out := groups[:0]
	for _, grp := range groups {
		if len(grp) >= 2 {
			out = append(out, grp)
		}
	}
	return out
}

func groupsBurst(g *graph) [][]int {
	return groupBy(g, func(i int) (string, bool) {
		if !g.isAsset(i) {
			return "", false
		}
		k := g.asset(i).BurstGroup()
		return k, k != ""
	})
}

func groupsSameSecond(g *graph) [][]int {
	return groupBy(g, func(i int) (string, bool) {
		if !g.isAsset(i) {
			return "", false
		}
		dt, ok := ParseDT(g.asset(i).Date)
		return dt.Truncate(time.Second).Format(time.DateTime), ok
	})
}

func groupsStemRepeat(g *graph) [][]int {
	var out [][]int
	for _, grp := range groupBy(g, func(i int) (string, bool) { return stemLower(g.name(i)), true }) {
		if len(grp) > maxStemGroup {
			continue
		}
		years := map[int]bool{}
		for _, i := range grp {
			if g.isAsset(i) {
				if dt, ok := ParseDT(g.asset(i).Date); ok {
					years[dt.Year()] = true
				}
			}
		}
		if len(years) >= 2 {
			out = append(out, grp)
		}
	}
	return out
}

func groupsICloudDuplicate(g *graph) [][]int {
	return groupBy(g, func(i int) (string, bool) {
		if !g.isAsset(i) {
			return "", false
		}
		a := g.asset(i)
		return strings.ToLower(a.Name()) + "\x00" + strconv.FormatInt(a.OriginalFilesize, 10), true
	})
}

func groupsCloudDuplicate(g *graph) [][]int {
	return groupBy(g, func(i int) (string, bool) {
		if g.isAsset(i) {
			return "", false
		}
		k := g.file(i).SHA1
		return k, k != ""
	})
}

// Categories lists the fixture categories in output order.
var Categories = []Category{
	{Name: "level-exact", pred: hasLevel(LevelExact)},
	{Name: "level-same-shot", pred: hasLevel(LevelSameShot)},
	{Name: "level-same-shot-weak", pred: hasLevel(LevelSameShotWeak)},
	{Name: "level-same-time-size", pred: hasLevel(LevelSameTimeSize)},
	{Name: "level-probable", pred: hasLevel(LevelProbable)},
	{Name: "level-pair-component", pred: hasLevel(LevelPairComponent)},
	{Name: "plan-replace", pred: hasPlan(PlanReplaceDecision)},
	{Name: "plan-keep-cloud", pred: hasPlan(PlanKeepCloudDecision)},
	{Name: "unmatched-icloud", pred: anyRecord(func(g *graph, i int) bool { return g.isAsset(i) && len(g.links[i]) == 0 })},
	{Name: "unmatched-cloud", pred: anyRecord(func(g *graph, i int) bool {
		return !g.isAsset(i) && IsMedia(g.file(i).Kind) && g.owner[i] < 0
	})},
	{Name: "live", pred: predLive},
	{Name: "raw-pair", pred: predRawPair},
	{Name: "burst", groups: groupsBurst},
	{Name: "same-second", groups: groupsSameSecond},
	{Name: "stem-repeat", groups: groupsStemRepeat},
	{Name: "suffix-n", pred: anyRecord(func(g *graph, i int) bool {
		s, _ := Splitext(g.name(i))
		return SuffixN(s) != ""
	})},
	{Name: "nfd", pred: anyRecord(func(g *graph, i int) bool {
		for _, s := range g.recordStrings(i) {
			if s != nfc(s) {
				return true
			}
		}
		return false
	})},
	{Name: "non-ascii", pred: anyRecord(func(g *graph, i int) bool { return !isASCII(g.name(i)) })},
	{Name: "ext-case", pred: anyPair(func(_ *graph, a Asset, f File, _ string) bool {
		_, ea := Splitext(a.Name())
		_, ef := Splitext(BaseName(f.Path))
		return ea != ef && strings.EqualFold(ea, ef)
	})},
	{Name: "fraction", pred: anyPair(func(_ *graph, _ Asset, f File, _ string) bool { return strings.Contains(f.DateTaken, ".") })},
	{Name: "tz-shift", pred: anyPair(func(_ *graph, a Asset, f File, l string) bool {
		da, ok1 := ParseDT(a.Date)
		df, ok2 := ParseDT(f.DateTaken)
		d := da.Sub(df)
		return l == LevelSameShot && ok1 && ok2 && (d >= 30*time.Minute || d <= -30*time.Minute)
	})},
	{Name: "date-1970", pred: anyAsset(func(a Asset) bool {
		dt, ok := ParseDT(a.Date)
		return ok && dt.Year() == 1970 && pxlRE.MatchString(a.Name())
	})},
	{Name: "many-folders", pred: predManyFolders},
	{Name: "album-many", pred: anyAsset(func(a Asset) bool {
		keys := map[string]bool{}
		for _, al := range a.Albums {
			keys[AlbumKey{al.FolderNames, al.Title}.String()] = true
		}
		return len(keys) >= 2
	})},
	{Name: "year-boundary", pred: predYearBoundary},
	{Name: "edited", pred: anyRecord(func(g *graph, i int) bool {
		if g.isAsset(i) {
			return g.asset(i).HasAdjustments
		}
		return strings.HasSuffix(stemLower(g.name(i)), "_edited")
	})},
	{Name: "sidecar", pred: anyFile(func(f File) bool { return f.Kind == KindSidecar })},
	{Name: "convert", pred: anyFile(func(f File) bool { return f.Kind == KindConvert })},
	{Name: "icloud-duplicate", groups: groupsICloudDuplicate},
	{Name: "cloud-duplicate", groups: groupsCloudDuplicate},
}

// UnitStats counts the units of a material.
type UnitStats struct {
	Total       int
	Unsupported int
	TooLarge    int // supported units over maxCaseRecords records
	// Exhausted counts the candidate units the mapping found no free output
	// for, by their first record: a union unit and one of its components can
	// both count.
	Exhausted int
}

// CaseResult is one category of a run; Case is nil for an empty category.
type CaseResult struct {
	Category string
	Case     *Case
	Src      []int // material record indices of the case, sorted; never written
	Records  int
	Levels   [6]int // pairs by level in the order of Levels
}

// Result is the outcome of a run: aggregates and the cases.
type Result struct {
	Links [6]int // in the order of Levels
	Units UnitStats
	// Rules counts the unsupported units by the rule of their first
	// unsupported record, and under "relation:<relation>" the candidate
	// units skipped for a broken relation, by their first record.
	Rules map[string]int
	Cases []CaseResult // one per category, in the order of Categories
	Words []string
}

// ErrRelations reports that the image of a case breaks relations; the
// message lists the broken relations with their counts only.
var ErrRelations = errors.New("relations broken")

// Build builds the cases of a material that passed the partition check. It
// fails with ErrRelations when the image of a case breaks a relation.
func Build(m *Material, mp *Mapper) (*Result, error) {
	g := newGraph(m)
	res := &Result{}
	for i := 0; i < g.nA; i++ {
		for _, p := range g.links[i] {
			res.Links[slices.Index(Levels, p.level)]++
		}
	}
	res.Units.Total = len(g.members)
	res.Rules = map[string]int{}
	for _, u := range g.members {
		switch rule := unsupportedRule(g, u); {
		case rule != "":
			res.Units.Unsupported++
			res.Rules[rule]++
		case len(u) > maxCaseRecords:
			res.Units.TooLarge++
		}
	}
	for _, c := range Categories {
		recs, err := g.pickUnits(mp, c.Name, g.candidates(c))
		if err != nil {
			return nil, err
		}
		cr := CaseResult{Category: c.Name}
		if len(recs) > 0 {
			if cr, err = g.buildCase(mp, c.Name, recs); err != nil {
				return nil, err
			}
		}
		res.Cases = append(res.Cases, cr)
	}
	res.Units.Exhausted = len(g.exhausted)
	for _, rel := range g.broken {
		res.Rules["relation:"+rel]++
	}
	res.Words = mp.Words()
	return res, nil
}

func unsupportedRule(g *graph, u []int) string {
	for _, i := range u {
		if g.rules[i] != "" {
			return g.rules[i]
		}
	}
	return ""
}

func (g *graph) buildCase(mp *Mapper, category string, recs []int) (CaseResult, error) {
	orig := g.m.Subset(recs)
	img, err := mp.Set(orig)
	if err != nil {
		return CaseResult{}, err
	}
	if vs := CheckRelations(orig, img, mp.YearShift()); len(vs) > 0 {
		parts := make([]string, len(vs))
		for i, v := range vs {
			parts[i] = fmt.Sprintf("%s=%d", v.Relation, v.Count)
		}
		return CaseResult{}, fmt.Errorf("%w: %s", ErrRelations, strings.Join(parts, " "))
	}
	imgOf := map[int]int{}
	for pos, r := range orig.Src {
		imgOf[r] = pos
	}
	cr := CaseResult{Category: category, Src: orig.Src, Records: len(recs)}
	c := &Case{
		Format:     FormatVersion,
		Provenance: Provenance{Kind: ProvenanceKind, Source: ProvenanceSource, Expected: ProvenanceExpected, Category: category},
		ICloud:     img.Assets,
		Cloud:      img.Files,
		Expected:   []Pair{},
	}
	nAssets := len(orig.Assets)
	for pos, r := range orig.Src[:nAssets] {
		for _, p := range g.links[r] {
			fpos := imgOf[p.file] - nAssets
			c.Expected = append(c.Expected, Pair{
				UUID:  img.Assets[pos].UUID,
				Path:  img.Files[fpos].Path,
				Level: p.level,
				Plan:  g.plan.decision(orig.Assets[pos].UUID, orig.Files[fpos].Path),
			})
			cr.Levels[slices.Index(Levels, p.level)]++
		}
	}
	cr.Case = c
	return cr, nil
}

// SupportedUnits returns every unit without an unsupported record, whatever
// its size, in the order of their first records.
func SupportedUnits(m *Material) [][]int {
	g := newGraph(m)
	var out [][]int
	for _, u := range g.members {
		if unsupportedRule(g, u) == "" {
			out = append(out, u)
		}
	}
	return out
}

// UnsupportedRules lists the "not supported" rules in output order.
var UnsupportedRules = []string{
	RuleDateYear, RuleDateParse, RuleYearSegment, RuleEventDigits, RuleChar, RuleLeadingMark, RuleNonASCIIDigit,
	RuleRootPath, RuleDrive, RuleDecimal, RuleFix1970, RuleNegativeSize, RuleCaseFold, RuleClusters, RuleEpochGap,
}

// Subset returns material records (sorted record indices) as a set.
func (m *Material) Subset(records []int) *Set {
	s := &Set{Assets: []Asset{}, Files: []File{}, Src: append([]int{}, records...)}
	for _, r := range records {
		if r < len(m.Assets) {
			s.Assets = append(s.Assets, m.Assets[r])
		} else {
			s.Files = append(s.Files, m.Files[r-len(m.Assets)])
		}
	}
	return s
}
