package anon

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// Violation is one broken relation and the number of breaks; it holds no
// value.
type Violation struct {
	Relation string
	Count    int
}

// Relations between a set and its image. Together they cover every
// predicate of the reference matching (order, case-insensitive names and
// stems, sizes, time differences to fractions of a second, the half-hour
// rule, the ±1 s bucket, the year and month of the folder, video, frame
// size, the MOV next to its pair, capture by path, the 1970 fix) and what
// the matching of the application needs on top (NFC, " (N)", albums and
// folders, Live, RAW, bursts, duplicates).
const (
	RelShape      = "shape"       // the image has other records
	RelFields     = "fields"      // 1: extension, kind, frame size, tzoffset, flags, burst_photos count, live_pair
	RelDate       = "date"        // 2: shift, form and fraction of dates and of the 1970 fix
	RelFolderInfo = "folder_info" // 3: year, event month and day of the folder
	RelStrings    = "strings"     // 4: emptiness, ASCII, NFC, "/" count, " (N)" and "_edited" tails
	RelIDs        = "ids"         // 5: uuid (with burst_photos), burst group, path, content_id, sha1 partitions
	RelNames      = "names"       // 6: name and stem partitions of assets and files together
	RelFolders    = "folders"     // 7: folder, (folder, stem) and neighbor key partitions of files
	RelAlbums     = "albums"      // 8: album keys of assets and of file folders together
	RelSize       = "size"        // 9: order and equality of sizes
	RelOrder      = "order"       // 10: relative order of the records
	RelEpochGap   = "epoch_gap"   // 11: no shifted date within 14 h 2 s of an epoch date
)

var relationOrder = []string{RelShape, RelFields, RelDate, RelFolderInfo, RelStrings, RelIDs, RelNames, RelFolders, RelAlbums, RelSize, RelOrder, RelEpochGap}

// epochGap is the largest time difference the reference matching links,
// itself included.
const epochGap = 14*time.Hour + 2*time.Second

// bijection counts the breaks of a one-to-one map between keys of a set and
// keys of its image.
type bijection struct {
	fwd, rev map[string]string
	bad      int
}

func newBijection() *bijection { return &bijection{fwd: map[string]string{}, rev: map[string]string{}} }

func (b *bijection) add(o, i string) {
	if (o == "") != (i == "") {
		b.bad++
		return
	}
	if o == "" {
		return
	}
	if v, ok := b.fwd[o]; ok && v != i {
		b.bad++
	}
	if v, ok := b.rev[i]; ok && v != o {
		b.bad++
	}
	b.fwd[o], b.rev[i] = i, o
}

// CheckRelations compares a set with its image: records are paired by their
// source index, shift is the date shift in years. It returns the broken
// relations in a fixed order, with counts only.
func CheckRelations(orig, img *Set, shift int) []Violation {
	c := map[string]int{}
	if bad := pairShape(orig, img); bad {
		return []Violation{{RelShape, 1}}
	}
	pos := map[int]int{}
	for k, s := range img.Src {
		pos[s] = k
	}
	for k := range orig.Src {
		if img.Src[k] != orig.Src[k] {
			c[RelOrder]++
		}
	}
	nA := len(orig.Assets)
	assets := make([]Asset, nA)
	files := make([]File, len(orig.Files))
	for k, s := range orig.Src {
		p := pos[s]
		if k < nA {
			assets[k] = img.Assets[p]
		} else {
			files[k-nA] = img.Files[p-nA]
		}
	}
	days := time.Duration(shift/4*1461) * 24 * time.Hour
	if shift%4 != 0 {
		days = -1
	}
	r := relations{shift: shift, days: days, c: c}
	r.records(orig, assets, files)
	r.partitions(orig, assets, files)
	r.sizes(orig, assets, files)
	c[RelEpochGap] += epochGaps(orig.Assets, orig.Files) + epochGaps(assets, files)
	var out []Violation
	for _, name := range relationOrder {
		if c[name] > 0 {
			out = append(out, Violation{name, c[name]})
		}
	}
	return out
}

// pairShape reports whether the image has other records than the set.
func pairShape(orig, img *Set) bool {
	nA, n := len(orig.Assets), len(orig.Assets)+len(orig.Files)
	if len(img.Assets) != nA || len(img.Files) != len(orig.Files) || len(orig.Src) != n || len(img.Src) != n {
		return true
	}
	pos := map[int]int{}
	for k, s := range img.Src {
		if _, dup := pos[s]; dup {
			return true
		}
		pos[s] = k
	}
	for k, s := range orig.Src {
		p, ok := pos[s]
		if !ok || (k < nA) != (p < nA) {
			return true
		}
	}
	return false
}

type relations struct {
	shift int
	days  time.Duration
	c     map[string]int
}

func (r relations) records(orig *Set, assets []Asset, files []File) {
	for k, o := range orig.Assets {
		i := assets[k]
		if !extSame(o.OriginalFilename, i.OriginalFilename) || !extSame(o.Filename, i.Filename) ||
			o.OriginalWidth != i.OriginalWidth || o.OriginalHeight != i.OriginalHeight || o.Width != i.Width || o.Height != i.Height ||
			o.TZOffset != i.TZOffset || o.InTrash != i.InTrash || o.HasAdjustments != i.HasAdjustments || o.LivePhoto != i.LivePhoto ||
			o.IsMovie != i.IsMovie || o.HasRaw != i.HasRaw || o.IsRaw != i.IsRaw || o.Burst != i.Burst || o.BurstKeyPhoto != i.BurstKeyPhoto ||
			len(o.BurstPhotos) != len(i.BurstPhotos) || !sameAlbumShape(o.Albums, i.Albums) {
			r.c[RelFields]++
		}
		if !r.dateSame(o.Date, i.Date) || !r.fixSame(o, i) {
			r.c[RelDate]++
		}
		bad := !nameStrSame(o.OriginalFilename, i.OriginalFilename) || !nameStrSame(o.Filename, i.Filename)
		var albumStrs [][2]string // titles and folder_names
		if sameAlbumShape(o.Albums, i.Albums) {
			for a := range o.Albums {
				albumStrs = append(albumStrs, [2]string{o.Albums[a].Title, i.Albums[a].Title})
				for f := range o.Albums[a].FolderNames {
					albumStrs = append(albumStrs, [2]string{o.Albums[a].FolderNames[f], i.Albums[a].FolderNames[f]})
				}
			}
		}
		for _, s := range albumStrs {
			if !strSame(s[0], s[1]) || !tailSame(SuffixN(s[0]), SuffixN(s[1])) {
				bad = true
			}
		}
		if bad {
			r.c[RelStrings]++
		}
	}
	for k, o := range orig.Files {
		i := files[k]
		if !extSame(BaseName(o.Path), BaseName(i.Path)) || o.Kind != i.Kind || o.Width != i.Width || o.Height != i.Height || o.LivePair != i.LivePair {
			r.c[RelFields]++
		}
		if !r.dateSame(o.DateTaken, i.DateTaken) {
			r.c[RelDate]++
		}
		fo, fi := FolderInfoOf(o.Path), FolderInfoOf(i.Path)
		if (fo.Year == "") != (fi.Year == "") || fo.Year != "" && r.yearOf(fo.Year) != fi.Year || fo.EventMonth != fi.EventMonth || fo.EventDay != fi.EventDay {
			r.c[RelFolderInfo]++
		}
		if !strSame(o.Path, i.Path) || !nameStrSame(BaseName(o.Path), BaseName(i.Path)) {
			r.c[RelStrings]++
		}
	}
}

// yearOf returns the expected image of a year string.
func (r relations) yearOf(y string) string {
	n, err := strconv.Atoi(y)
	if err != nil || n == 1969 || n == 1970 {
		return y
	}
	return strconv.Itoa(n + r.shift)
}

func sameAlbumShape(a, b []AlbumRef) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i].FolderNames) != len(b[i].FolderNames) {
			return false
		}
	}
	return true
}

// extSame compares the extensions of two names: a known extension stays as
// it is; another one keeps its case pattern and stays unknown.
func extSame(o, i string) bool {
	_, eo := Splitext(o)
	_, ei := Splitext(i)
	if (eo == "") != (ei == "") {
		return false
	}
	if eo == "" {
		return true
	}
	if isKnownExt(eo[1:]) {
		return eo == ei
	}
	return !isKnownExt(ei[1:]) && runeCaseMask(eo) == runeCaseMask(ei)
}

func runeCaseMask(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r != toLowerRune(r) {
			b.WriteByte('U')
		} else {
			b.WriteByte('l')
		}
	}
	return b.String()
}

func toLowerRune(r rune) rune { return []rune(strings.ToLower(string(r)))[0] }

// strSame compares the properties of a string the mapping keeps.
func strSame(o, i string) bool {
	return (o == "") == (i == "") && isASCII(o) == isASCII(i) && (o == nfc(o)) == (i == nfc(i)) &&
		strings.Count(o, "/") == strings.Count(i, "/")
}

// nameStrSame compares the properties of a file name the mapping keeps,
// including the " (N)" and "_edited" tails of the stem.
func nameStrSame(o, i string) bool {
	so, _ := Splitext(o)
	si, _ := Splitext(i)
	return strSame(o, i) && tailSame(SuffixN(so), SuffixN(si)) &&
		strings.HasSuffix(strings.ToLower(so), "_edited") == strings.HasSuffix(strings.ToLower(si), "_edited")
}

// tailSame compares two " (N)" tails: equal, or numbers of the same length
// over two digits (such numbers are mapped).
func tailSame(o, i string) bool {
	return o == i || o != "" && i != "" && len(o) == len(i) && len(o) > len(" (00)")
}

// dateSame checks a date and its image: both empty or both parse, the same
// form byte by byte apart from digits, and the wall clock moved by exactly
// the shift (or kept for 1969 and 1970).
func (r relations) dateSame(o, i string) bool {
	if (o == "") != (i == "") || len(o) != len(i) {
		return false
	}
	for k := 0; k < len(o); k++ {
		do, di := isDigit(o[k]), isDigit(i[k])
		if do != di || !do && o[k] != i[k] {
			return false
		}
	}
	if o == "" {
		return true
	}
	to, ok1 := ParseDT(o)
	ti, ok2 := ParseDT(i)
	if ok1 != ok2 {
		return false
	}
	return !ok1 || ti.Sub(to) == r.want(to)
}

func (r relations) want(t time.Time) time.Duration {
	if t.Year() == 1969 || t.Year() == 1970 {
		return 0
	}
	return r.days
}

// fixSame checks the 1970 fix of an asset and of its image.
func (r relations) fixSame(o, i Asset) bool {
	to, ok1 := ParseDT(o.Date)
	ti, ok2 := ParseDT(i.Date)
	if !ok1 || !ok2 {
		return ok1 == ok2
	}
	fo, apo, _ := Fix1970(o.Name(), to, o.TZOffset)
	fi, api, _ := Fix1970(i.Name(), ti, i.TZOffset)
	return apo == api && (!apo || fi.Sub(fo) == r.want(fo))
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func (r relations) partitions(orig *Set, assets []Asset, files []File) {
	ids := []*bijection{newBijection(), newBijection(), newBijection(), newBijection(), newBijection()}
	names := []*bijection{newBijection(), newBijection(), newBijection(), newBijection(), newBijection()}
	folders := []*bijection{newBijection(), newBijection(), newBijection()}
	albums := newBijection()
	nameKeys := func(n string) []string {
		stem, _ := Splitext(n)
		s := lowerNFC(stem)
		return []string{strings.ToLower(n), strings.ToLower(stem), n, s, strings.TrimSuffix(s, SuffixN(s))}
	}
	addNames := func(o, i string) {
		if o == "" && i == "" {
			return
		}
		ko, ki := nameKeys(o), nameKeys(i)
		for k := range names {
			names[k].add("n:"+ko[k], "n:"+ki[k])
		}
	}
	for k, o := range orig.Assets {
		i := assets[k]
		ids[0].add(o.UUID, i.UUID)
		ids[1].add(o.BurstGroup(), i.BurstGroup())
		if len(o.BurstPhotos) == len(i.BurstPhotos) {
			// The mapping keeps the order of burst_photos; each element maps
			// as the UUID of that photo does.
			for b := range o.BurstPhotos {
				ids[0].add(o.BurstPhotos[b], i.BurstPhotos[b])
			}
		}
		addNames(o.Name(), i.Name())
		if sameAlbumShape(o.Albums, i.Albums) {
			for a := range o.Albums {
				albums.add("a:"+AlbumKey{o.Albums[a].FolderNames, o.Albums[a].Title}.String(),
					"a:"+AlbumKey{i.Albums[a].FolderNames, i.Albums[a].Title}.String())
			}
		}
	}
	for k, o := range orig.Files {
		i := files[k]
		ids[2].add(o.Path, i.Path)
		ids[3].add(o.ContentID, i.ContentID)
		ids[4].add(o.SHA1, i.SHA1)
		addNames(BaseName(o.Path), BaseName(i.Path))
		fo, fi := FolderOf(o.Path), FolderOf(i.Path)
		so, _ := Splitext(BaseName(o.Path))
		si, _ := Splitext(BaseName(i.Path))
		folders[0].add("f:"+fo, "f:"+fi)
		folders[1].add("f:"+fo+"\x00"+strings.ToLower(so), "f:"+fi+"\x00"+strings.ToLower(si))
		folders[2].add("f:"+NeighborKey(o.Path), "f:"+NeighborKey(i.Path))
		ko, oko := FolderToAlbum(fo)
		ki, oki := FolderToAlbum(fi)
		switch {
		case oko != oki:
			albums.bad++
		case oko:
			albums.add("a:"+ko.String(), "a:"+ki.String())
		}
	}
	for _, b := range ids {
		r.c[RelIDs] += b.bad
	}
	for _, b := range names {
		r.c[RelNames] += b.bad
	}
	for _, b := range folders {
		r.c[RelFolders] += b.bad
	}
	r.c[RelAlbums] += albums.bad
}

func (r relations) sizes(orig *Set, assets []Asset, files []File) {
	type pair struct{ o, i int64 }
	var ps []pair
	for k, o := range orig.Assets {
		ps = append(ps, pair{o.OriginalFilesize, assets[k].OriginalFilesize})
	}
	for k, o := range orig.Files {
		ps = append(ps, pair{o.Size, files[k].Size})
	}
	sort.SliceStable(ps, func(a, b int) bool { return ps[a].o < ps[b].o })
	for k, p := range ps {
		if (p.o == 0) != (p.i == 0) {
			r.c[RelSize]++
		}
		if k == 0 {
			continue
		}
		q := ps[k-1]
		if p.o == q.o && p.i != q.i || p.o > q.o && p.i <= q.i {
			r.c[RelSize]++
		}
	}
}

// epochGaps counts the epoch dates (1969, 1970) within 14 h 2 s of a shifted
// date, the bound included as in the reference: the shift would move one of
// them away from the other.
func epochGaps(assets []Asset, files []File) int {
	var epoch, shifted []time.Time
	add := func(t time.Time) {
		if t.Year() == 1969 || t.Year() == 1970 {
			epoch = append(epoch, t)
		} else {
			shifted = append(shifted, t)
		}
	}
	for _, a := range assets {
		if dt, ok := ParseDT(a.Date); ok {
			if fixed, applies, _ := Fix1970(a.Name(), dt, a.TZOffset); applies {
				dt = fixed
			}
			add(dt)
		}
	}
	for _, f := range files {
		if dt, ok := ParseDT(f.DateTaken); ok {
			add(dt)
		}
	}
	sort.Slice(shifted, func(a, b int) bool { return shifted[a].Before(shifted[b]) })
	bad := 0
	for _, e := range epoch {
		k := sort.Search(len(shifted), func(j int) bool { return !shifted[j].Before(e) })
		near := k < len(shifted) && shifted[k].Sub(e) <= epochGap || k > 0 && e.Sub(shifted[k-1]) <= epochGap
		if near {
			bad++
		}
	}
	return bad
}
