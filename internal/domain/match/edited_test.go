package match_test

import (
	"cmp"
	"reflect"
	"slices"
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"
	"pgregory.net/rapid"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/match"
)

func TestEdited(t *testing.T) {
	t.Parallel()
	if kept, edited := match.Edited(nil, nil); kept != nil || edited != nil {
		t.Errorf("Edited(nil, nil) = %#v, %#v, want nil, nil", kept, edited)
	}

	nfc := "изменённые"
	nfd := norm.NFD.String(nfc)
	if nfd == nfc {
		t.Fatalf("%+q has no composed letter, so the NFD cases test nothing", nfc)
	}
	defaults := []string{"отредактировано", "edited"}
	files := func(paths ...string) []domain.RemoteEntry {
		var es []domain.RemoteEntry
		for _, p := range paths {
			es = append(es, domain.RemoteEntry{Root: domain.LibraryShared, Path: p})
		}
		return es
	}
	dirEntries := func(paths ...string) []domain.RemoteEntry {
		es := files(paths...)
		for i := range es {
			es[i].IsDir = true
		}
		return es
	}
	cases := []struct {
		name         string
		editedDirs   []string
		in           []domain.RemoteEntry
		kept, edited []string
	}{
		{
			name:       "an original",
			editedDirs: defaults,
			in:         files("a/IMG_0001.HEIC"),
			kept:       []string{"a/IMG_0001.HEIC"},
		},
		{
			name:       "a directory of edits",
			editedDirs: defaults,
			in:         files("a/edited/IMG_0001.HEIC"),
			edited:     []string{"a/edited/IMG_0001.HEIC"},
		},
		{
			name:       "the other default directory",
			editedDirs: defaults,
			in:         files("a/отредактировано/IMG_0001.HEIC"),
			edited:     []string{"a/отредактировано/IMG_0001.HEIC"},
		},
		{
			name:       "a directory in another case",
			editedDirs: defaults,
			in:         files("a/EDITED/IMG_0001.HEIC", "a/Отредактировано/IMG_0002.HEIC"),
			edited:     []string{"a/EDITED/IMG_0001.HEIC", "a/Отредактировано/IMG_0002.HEIC"},
		},
		{
			name:       "a setting in another case",
			editedDirs: []string{"Edited"},
			in:         files("a/edited/IMG_0001.HEIC"),
			edited:     []string{"a/edited/IMG_0001.HEIC"},
		},
		{
			name:       "a directory in NFD",
			editedDirs: []string{nfc},
			in:         files("a/" + nfd + "/IMG_0001.HEIC"),
			edited:     []string{"a/" + nfd + "/IMG_0001.HEIC"},
		},
		{
			name:       "a setting in NFD",
			editedDirs: []string{nfd},
			in:         files("a/" + nfc + "/IMG_0001.HEIC"),
			edited:     []string{"a/" + nfc + "/IMG_0001.HEIC"},
		},
		{
			name:       "a directory of edits at any depth",
			editedDirs: defaults,
			in:         files("2024/edited/a/IMG_0002.HEIC", "edited/2024/05-01 - Trip/IMG_0001.HEIC"),
			edited:     []string{"2024/edited/a/IMG_0002.HEIC", "edited/2024/05-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name:       "names like a directory of edits",
			editedDirs: defaults,
			in:         files("a/edited", "a/edited.jpg", "a/edited_photos/IMG_0001.HEIC", "a/x.edited/IMG_0002.HEIC", "edited"),
			kept:       []string{"a/edited", "a/edited.jpg", "a/edited_photos/IMG_0001.HEIC", "a/x.edited/IMG_0002.HEIC", "edited"},
		},
		{
			name:       "an edited name",
			editedDirs: defaults,
			in:         files("a/IMG_0001_EDITED.JPG", "a/IMG_0001_edited.AAE", "a/IMG_0001_edited.jpeg"),
			edited:     []string{"a/IMG_0001_EDITED.JPG", "a/IMG_0001_edited.AAE", "a/IMG_0001_edited.jpeg"},
		},
		{
			name:       "a numbered edited name",
			editedDirs: defaults,
			in:         files("a/IMG_0001_edited (1).jpeg", "a/IMG_0001_edited (12).jpeg"),
			edited:     []string{"a/IMG_0001_edited (1).jpeg", "a/IMG_0001_edited (12).jpeg"},
		},
		{
			name:       "not a number",
			editedDirs: defaults,
			in: files("a/IMG_0001_edited ().jpeg", "a/IMG_0001_edited (-1).jpeg", "a/IMG_0001_edited (1) (2).jpeg",
				"a/IMG_0001_edited (a).jpeg", "a/IMG_0001_edited(1).jpeg"),
			kept: []string{"a/IMG_0001_edited ().jpeg", "a/IMG_0001_edited (-1).jpeg", "a/IMG_0001_edited (1) (2).jpeg",
				"a/IMG_0001_edited (a).jpeg", "a/IMG_0001_edited(1).jpeg"},
		},
		{
			name:       "edited inside a name",
			editedDirs: defaults,
			in:         files("a/IMG_edited_0001.HEIC", "a/edited_IMG_0001.HEIC"),
			kept:       []string{"a/IMG_edited_0001.HEIC", "a/edited_IMG_0001.HEIC"},
		},
		{
			name:       "any file in a directory of edits",
			editedDirs: defaults,
			in:         files("a/edited/IMG_0001.psd", "a/edited/IMG_0001.xmp", "a/edited/README"),
			edited:     []string{"a/edited/IMG_0001.psd", "a/edited/IMG_0001.xmp", "a/edited/README"},
		},
		{
			name:       "directories are kept",
			editedDirs: defaults,
			in:         dirEntries("a/IMG_0001_edited", "a/edited", "a/edited/b"),
			kept:       []string{"a/IMG_0001_edited", "a/edited", "a/edited/b"},
		},
		{
			name:   "no setting",
			in:     files("a/IMG_0001_edited.jpeg", "a/edited/IMG_0001.HEIC"),
			kept:   []string{"a/edited/IMG_0001.HEIC"},
			edited: []string{"a/IMG_0001_edited.jpeg"},
		},
		{
			name:       "a dot in a directory name",
			editedDirs: defaults,
			in:         files("a.v2/IMG_0001_edited"),
			edited:     []string{"a.v2/IMG_0001_edited"},
		},
		{
			name:       "files at the root",
			editedDirs: defaults,
			in:         files("IMG_0001.HEIC", "IMG_0001_edited.jpeg"),
			kept:       []string{"IMG_0001.HEIC"},
			edited:     []string{"IMG_0001_edited.jpeg"},
		},
		{
			name:       "results in path order",
			editedDirs: defaults,
			in:         files("b/IMG_0002.HEIC", "a/edited/IMG_0003.HEIC", "a/IMG_0001.HEIC", "a/IMG_0001_edited.jpeg"),
			kept:       []string{"a/IMG_0001.HEIC", "b/IMG_0002.HEIC"},
			edited:     []string{"a/IMG_0001_edited.jpeg", "a/edited/IMG_0003.HEIC"},
		},
		{
			name:       "roots apart",
			editedDirs: defaults,
			in: []domain.RemoteEntry{
				{Root: domain.LibraryShared, Path: "a/IMG_0001_edited.jpeg"},
				{Root: domain.LibraryPersonal, Path: "b/IMG_0002_edited.jpeg"},
			},
			edited: []string{"personal:b/IMG_0002_edited.jpeg", "a/IMG_0001_edited.jpeg"},
		},
		{
			name:       "nothing",
			editedDirs: defaults,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			in := slices.Clone(c.in)
			kept, edited := match.Edited(in, c.editedDirs)
			if !reflect.DeepEqual(in, c.in) {
				t.Errorf("%s: Edited changed its input to %+q", c.name, places(in))
			}
			if p := places(kept); !slices.Equal(p, c.kept) {
				t.Errorf("%s: kept = %+q, want %+q", c.name, p, c.kept)
			}
			if p := places(edited); !slices.Equal(p, c.edited) {
				t.Errorf("%s: edited = %+q, want %+q", c.name, p, c.edited)
			}
		})
	}
}

// editDirNames and editFileNames label the parts of a drawn path with whether
// they make a file an edit under the default settings.
var editDirNames = []struct {
	name string
	edit bool
}{
	{"a", false}, {"2024", false}, {"edited", true}, {"EDITED", true},
	{"Отредактировано", true}, {"edited_photos", false}, {"x.edited", false},
}

var editFileNames = []struct {
	name string
	edit bool
}{
	{"IMG_0001.HEIC", false}, {"IMG_0001_edited.jpeg", true}, {"IMG_0001_EDITED (2).JPG", true},
	{"IMG_0001_edited", true}, {"IMG_edited_0001.HEIC", false},
	{"IMG_0001_edited (1) (2).jpeg", false}, {"edited", false}, {"edited.jpg", false},
}

type labeledEntry struct {
	entry domain.RemoteEntry
	edit  bool
}

func editEntry() *rapid.Generator[labeledEntry] {
	return rapid.Custom(func(t *rapid.T) labeledEntry {
		var parts []string
		edit := false
		for range rapid.IntRange(0, 3).Draw(t, "depth") {
			d := rapid.SampledFrom(editDirNames).Draw(t, "dir")
			parts = append(parts, d.name)
			edit = edit || d.edit
		}
		f := rapid.SampledFrom(editFileNames).Draw(t, "name")
		isDir := rapid.IntRange(0, 9).Draw(t, "isDir") == 9
		root := rapid.SampledFrom([]domain.LibraryKind{domain.LibraryShared, domain.LibraryPersonal}).Draw(t, "root")
		return labeledEntry{
			entry: domain.RemoteEntry{Root: root, Path: strings.Join(append(parts, f.name), "/"), IsDir: isDir},
			edit:  !isDir && (edit || f.edit),
		}
	})
}

// covers: INV-9
func TestEditedProperties(t *testing.T) {
	t.Parallel()
	byRootPath := func(a, b domain.RemoteEntry) int {
		return cmp.Or(cmp.Compare(a.Root, b.Root), strings.Compare(a.Path, b.Path))
	}
	rapid.Check(t, func(t *rapid.T) {
		drawn := rapid.SliceOfNDistinct(editEntry(), 0, 12, func(l labeledEntry) string { return place(l.entry) }).Draw(t, "in")
		var in, wantKept, wantEdited []domain.RemoteEntry
		for _, l := range drawn {
			in = append(in, l.entry)
			if l.edit {
				wantEdited = append(wantEdited, l.entry)
			} else {
				wantKept = append(wantKept, l.entry)
			}
		}
		slices.SortFunc(wantKept, byRootPath)
		slices.SortFunc(wantEdited, byRootPath)
		before := slices.Clone(in)
		kept, edited := match.Edited(in, []string{"отредактировано", "edited"})
		if !reflect.DeepEqual(in, before) {
			t.Fatalf("Edited changed its input %+q to %+q", places(before), places(in))
		}
		if !reflect.DeepEqual(kept, wantKept) || !reflect.DeepEqual(edited, wantEdited) {
			t.Fatalf("Edited(%+q) = %+q, %+q; want %+q, %+q",
				places(in), places(kept), places(edited), places(wantKept), places(wantEdited))
		}
	})
}
