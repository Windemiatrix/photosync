package match_test

import (
	"cmp"
	"path"
	"reflect"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/Windemiatrix/go-libs/osxphotos/export"
	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/match"
)

// place writes an entry as its path, with the root in front unless it is the
// shared one, so that table rows stay short.
func place(e domain.RemoteEntry) string {
	if e.Root == domain.LibraryShared {
		return e.Path
	}
	return string(e.Root) + ":" + e.Path
}

func places(es []domain.RemoteEntry) []string {
	var out []string
	for _, e := range es {
		out = append(out, place(e))
	}
	return out
}

// describe writes each candidate as one string, so that one comparison checks
// the main file, the role, the components and the sidecars together.
func describe(cs []match.Candidate) []string {
	var out []string
	for _, c := range cs {
		s := place(c.Main)
		if c.Role != domain.RoleOriginal {
			s += " role=" + string(c.Role)
		}
		if c.Live != nil {
			s += " live=" + place(*c.Live)
		}
		if c.RAW != nil {
			s += " raw=" + place(*c.RAW)
		}
		for _, sc := range c.Sidecars {
			s += " sidecar=" + place(sc)
		}
		out = append(out, s)
	}
	return out
}

func TestGroup(t *testing.T) {
	t.Parallel()
	if got, unknown := match.Group(nil); got != nil || unknown != nil {
		t.Errorf("Group(nil) = %#v, %#v, want nil, nil", got, unknown)
	}

	files := func(paths ...string) []domain.RemoteEntry {
		var es []domain.RemoteEntry
		for _, p := range paths {
			es = append(es, domain.RemoteEntry{Root: domain.LibraryShared, Path: p})
		}
		return es
	}
	cases := []struct {
		name          string
		in            []domain.RemoteEntry
		want, unknown []string
	}{
		{
			name: "a Live Photo",
			in:   files("a/IMG_0001.HEIC", "a/IMG_0001.MOV"),
			want: []string{"a/IMG_0001.HEIC live=a/IMG_0001.MOV"},
		},
		{
			name: "photosync names the video in lower case",
			in:   files("a/IMG_0001.HEIC", "a/IMG_0001.mov"),
			want: []string{"a/IMG_0001.HEIC live=a/IMG_0001.mov"},
		},
		{
			name: "a JPEG Live Photo",
			in:   files("a/IMG_0001.JPG", "a/IMG_0001.MOV"),
			want: []string{"a/IMG_0001.JPG live=a/IMG_0001.MOV"},
		},
		{
			name: "stems differ in case",
			in:   files("a/IMG_0001.HEIC", "a/img_0001.mov"),
			want: []string{"a/IMG_0001.HEIC live=a/img_0001.mov"},
		},
		{
			name: "NFD and NFC stems",
			in:   files("a/Й.HEIC", "a/Й.MOV"),
			want: []string{"a/Й.HEIC live=a/Й.MOV"},
		},
		{
			name: "a RAW pair",
			in:   files("a/IMG_0001.DNG", "a/IMG_0001.JPG"),
			want: []string{"a/IMG_0001.JPG raw=a/IMG_0001.DNG"},
		},
		{
			name: "a RAW alone",
			in:   files("a/IMG_0001.DNG"),
			want: []string{"a/IMG_0001.DNG"},
		},
		{
			name: "a Live Photo with a RAW",
			in:   files("a/IMG_0001.CR2", "a/IMG_0001.MOV", "a/IMG_0001.JPG"),
			want: []string{"a/IMG_0001.JPG live=a/IMG_0001.MOV raw=a/IMG_0001.CR2"},
		},
		{
			name: "a video alone",
			in:   files("a/IMG_0001.MOV"),
			want: []string{"a/IMG_0001.MOV"},
		},
		{
			name: "an image does not pair",
			in:   files("a/IMG_0001.PNG", "a/IMG_0001.MOV"),
			want: []string{"a/IMG_0001.MOV", "a/IMG_0001.PNG"},
		},
		{
			name: "MP4 is not the video of a Live Photo",
			in:   files("a/IMG_0001.MP4", "a/IMG_0001.HEIC"),
			want: []string{"a/IMG_0001.HEIC", "a/IMG_0001.MP4"},
		},
		{
			name: "two stills",
			in:   files("a/IMG_0001.JPG", "a/IMG_0001.MOV", "a/IMG_0001.HEIC"),
			want: []string{"a/IMG_0001.HEIC", "a/IMG_0001.JPG", "a/IMG_0001.MOV"},
		},
		{
			name: "two MOVs",
			in:   files("a/img_0001.mov", "a/IMG_0001.MOV", "a/IMG_0001.HEIC"),
			want: []string{"a/IMG_0001.HEIC", "a/IMG_0001.MOV", "a/img_0001.mov"},
		},
		{
			name: "two RAWs",
			in:   files("a/IMG_0001.NEF", "a/IMG_0001.JPG", "a/IMG_0001.DNG"),
			want: []string{"a/IMG_0001.DNG", "a/IMG_0001.JPG", "a/IMG_0001.NEF"},
		},
		{
			name: "directories apart",
			in:   files("a/IMG_0001.HEIC", "b/IMG_0001.MOV"),
			want: []string{"a/IMG_0001.HEIC", "b/IMG_0001.MOV"},
		},
		{
			name: "directories that differ in case",
			in:   files("a/IMG_0001.HEIC", "A/IMG_0001.MOV"),
			want: []string{"A/IMG_0001.MOV", "a/IMG_0001.HEIC"},
		},
		{
			name: "roots apart",
			in: []domain.RemoteEntry{
				{Root: domain.LibraryShared, Path: "a/IMG_0001.HEIC"},
				{Root: domain.LibraryPersonal, Path: "a/IMG_0001.MOV"},
			},
			want: []string{"personal:a/IMG_0001.MOV", "a/IMG_0001.HEIC"},
		},
		{
			name: "numbered Live Photos",
			in:   files("a/IMG_0001.HEIC", "a/IMG_0001.MOV", "a/IMG_0001 (1).HEIC", "a/IMG_0001 (1).mov"),
			want: []string{"a/IMG_0001 (1).HEIC live=a/IMG_0001 (1).mov", "a/IMG_0001.HEIC live=a/IMG_0001.MOV"},
		},
		{
			name: "a numbered still and a plain MOV",
			in:   files("a/IMG_0001 (1).HEIC", "a/IMG_0001.MOV"),
			want: []string{"a/IMG_0001 (1).HEIC", "a/IMG_0001.MOV"},
		},
		{
			name: "a RAW with its XMP",
			in:   files("a/IMG_0001.xmp", "a/IMG_0001.DNG"),
			want: []string{"a/IMG_0001.DNG sidecar=a/IMG_0001.xmp"},
		},
		{
			name: "a Live Photo with its sidecars",
			in:   files("a/IMG_0001.xmp", "a/IMG_0001.HEIC", "a/IMG_0001.AAE", "a/IMG_0001.MOV"),
			want: []string{"a/IMG_0001.HEIC live=a/IMG_0001.MOV sidecar=a/IMG_0001.AAE sidecar=a/IMG_0001.xmp"},
		},
		{
			name: "a source with its THM",
			in:   files("a/MVI_0001.THM", "a/MVI_0001.AVI"),
			want: []string{"a/MVI_0001.AVI role=source sidecar=a/MVI_0001.THM"},
		},
		{
			name: "DVD files follow the VOB",
			in:   files("a/VTS_01_0.IFO", "a/VTS_01_0.BUP", "a/VTS_01_0.VOB"),
			want: []string{"a/VTS_01_0.VOB role=source sidecar=a/VTS_01_0.BUP sidecar=a/VTS_01_0.IFO"},
		},
		{
			name: "an MPG alone",
			in:   files("a/MOV_0001.mpg"),
			want: []string{"a/MOV_0001.mpg role=source"},
		},
		{
			name: "sidecars without media",
			in:   files("a/IMG_0002.xmp", "a/archive.zip"),
		},
		{
			name: "sidecars of an ambiguous stem",
			in:   files("a/IMG_0001.xmp", "a/IMG_0001.HEIC", "a/IMG_0001.JPG"),
			want: []string{"a/IMG_0001.HEIC", "a/IMG_0001.JPG"},
		},
		{
			name:    "unknown extensions",
			in:      files("a/README", "a/IMG_0001.psd", "a/IMG_0002.", "a/IMG_0001.HEIC"),
			want:    []string{"a/IMG_0001.HEIC"},
			unknown: []string{"a/IMG_0001.psd", "a/IMG_0002.", "a/README"},
		},
		{
			name: "directories are left out",
			in: []domain.RemoteEntry{
				{Root: domain.LibraryShared, Path: "a/IMG_0001.HEIC", IsDir: true},
				{Root: domain.LibraryShared, Path: "a", IsDir: true},
				{Root: domain.LibraryShared, Path: "a/IMG_0001.MOV"},
			},
			want: []string{"a/IMG_0001.MOV"},
		},
		{
			name: "files at the root",
			in:   files("IMG_0001.HEIC", "IMG_0001.MOV"),
			want: []string{"IMG_0001.HEIC live=IMG_0001.MOV"},
		},
		{
			name: "nothing",
		},
		{
			name:    "a dot in a directory name",
			in:      files("a.v2/README", "a.v2/IMG_0001.HEIC"),
			want:    []string{"a.v2/IMG_0001.HEIC"},
			unknown: []string{"a.v2/README"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, unknown := match.Group(c.in)
			if d := describe(got); !slices.Equal(d, c.want) {
				t.Errorf("%s: candidates = %+q, want %+q", c.name, d, c.want)
			}
			if p := places(unknown); !slices.Equal(p, c.unknown) {
				t.Errorf("%s: unknown = %+q, want %+q", c.name, p, c.unknown)
			}
		})
	}
}

var groupExts = []struct{ ext, class string }{
	{".HEIC", "still"}, {".heic", "still"}, {".JPG", "still"},
	{".png", "image"},
	{".DNG", "raw"}, {".NEF", "raw"},
	{".MOV", "mov"}, {".mov", "mov"},
	{".MP4", "video"},
	{".AVI", "source"}, {".mpg", "source"},
	{".xmp", "sidecar"}, {".AAE", "sidecar"}, {".THM", "sidecar"}, {".zip", "sidecar"},
	{".psd", "unknown"}, {"", "unknown"}, {".", "unknown"},
}

func groupEntry() *rapid.Generator[domain.RemoteEntry] {
	return rapid.Custom(func(t *rapid.T) domain.RemoteEntry {
		root := rapid.SampledFrom([]domain.LibraryKind{domain.LibraryShared, domain.LibraryPersonal}).Draw(t, "root")
		dir := rapid.SampledFrom([]string{"", "a/", "A/", "a/b/"}).Draw(t, "dir")
		stem := rapid.SampledFrom([]string{"IMG_0001", "img_0001", "IMG_0001 (1)", "IMG_0002", "Й", "Й"}).Draw(t, "stem")
		ext := rapid.SampledFrom(groupExts).Draw(t, "ext")
		return domain.RemoteEntry{
			Root:  root,
			Path:  dir + stem + ext.ext,
			IsDir: rapid.IntRange(0, 9).Draw(t, "isDir") == 9,
			Size:  rapid.Int64Range(1, 3).Draw(t, "size"),
		}
	})
}

// covers: INV-9
func TestGroupProperties(t *testing.T) {
	t.Parallel()
	classes := map[string]string{}
	for _, g := range groupExts {
		classes[g.ext] = g.class
	}
	class := func(e domain.RemoteEntry) string { return classes[path.Ext(e.Path)] }
	before := func(a, b domain.RemoteEntry) bool {
		return cmp.Or(cmp.Compare(a.Root, b.Root), strings.Compare(a.Path, b.Path)) < 0
	}
	sameStem := func(a, b domain.RemoteEntry) bool {
		return a.Root == b.Root && path.Dir(a.Path) == path.Dir(b.Path) &&
			export.StemKey(path.Base(a.Path)) == export.StemKey(path.Base(b.Path))
	}

	rapid.Check(t, func(t *rapid.T) {
		in := rapid.SliceOfNDistinct(groupEntry(), 0, 12, place).Draw(t, "in")
		cands, unknown := match.Group(in)

		inputs := map[string]domain.RemoteEntry{}
		for _, e := range in {
			inputs[place(e)] = e
		}
		slots := map[string][]string{}
		found := func(slot string, e domain.RemoteEntry) {
			if want, ok := inputs[place(e)]; !ok || !reflect.DeepEqual(e, want) {
				t.Fatalf("%s %q = %#v is not an entry of the input", slot, place(e), e)
			}
			slots[place(e)] = append(slots[place(e)], slot)
		}
		for _, c := range cands {
			found("main", c.Main)
			if c.Live != nil {
				found("live", *c.Live)
			}
			if c.RAW != nil {
				found("raw", *c.RAW)
			}
			for _, s := range c.Sidecars {
				found("sidecar", s)
			}
		}
		for _, e := range unknown {
			found("unknown", e)
		}
		for _, e := range in {
			got := slots[place(e)]
			var ok bool
			switch {
			case e.IsDir:
				ok = len(got) == 0
			case class(e) == "unknown":
				ok = slices.Equal(got, []string{"unknown"})
			case class(e) == "sidecar":
				ok = len(got) == 0 || slices.Equal(got, []string{"sidecar"})
			default:
				ok = len(got) == 1 && slices.Contains([]string{"main", "live", "raw"}, got[0])
			}
			if !ok {
				t.Fatalf("%q (dir %v, %s) is found as %q", place(e), e.IsDir, class(e), got)
			}
		}

		perm := rapid.Permutation(in).Draw(t, "perm")
		if pc, pu := match.Group(perm); !reflect.DeepEqual(pc, cands) || !reflect.DeepEqual(pu, unknown) {
			t.Fatalf("permuted input gives %q, unknown %q, want %q, unknown %q",
				describe(pc), places(pu), describe(cands), places(unknown))
		}

		for _, c := range cands {
			comps := slices.Clone(c.Sidecars)
			if c.Live != nil {
				comps = append(comps, *c.Live)
			}
			if c.RAW != nil {
				comps = append(comps, *c.RAW)
			}
			for _, x := range comps {
				if !sameStem(x, c.Main) {
					t.Fatalf("%q goes with %q of another place or stem", place(x), place(c.Main))
				}
			}
			if (c.Live != nil && class(*c.Live) != "mov") || (c.RAW != nil && class(*c.RAW) != "raw") {
				t.Fatalf("components of %q are of a wrong kind: %q", place(c.Main), describe([]match.Candidate{c}))
			}
			if (c.Live != nil || c.RAW != nil) && class(c.Main) != "still" {
				t.Fatalf("%q has components but is %s, want still", place(c.Main), class(c.Main))
			}
			role := domain.RoleOriginal
			if class(c.Main) == "source" {
				role = domain.RoleSource
			}
			if c.Role != role {
				t.Fatalf("role of %q = %q, want %q", place(c.Main), c.Role, role)
			}
		}

		for i := 1; i < len(cands); i++ {
			if !before(cands[i-1].Main, cands[i].Main) {
				t.Fatalf("candidates %q are not in order", describe(cands))
			}
		}
		for i := 1; i < len(unknown); i++ {
			if !before(unknown[i-1], unknown[i]) {
				t.Fatalf("unknown %q is not in order", places(unknown))
			}
		}
		for _, c := range cands {
			for i := 1; i < len(c.Sidecars); i++ {
				if strings.Compare(c.Sidecars[i-1].Path, c.Sidecars[i].Path) >= 0 {
					t.Fatalf("sidecars of %q are not in order: %q", place(c.Main), places(c.Sidecars))
				}
			}
		}
	})
}
