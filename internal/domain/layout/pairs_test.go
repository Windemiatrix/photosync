package layout_test

import (
	"fmt"
	"path"
	"reflect"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/layout"
)

// covers: R-LAYOUT-07
// covers: R-LAYOUT-10
func TestFirstPairs(t *testing.T) {
	t.Parallel()

	const (
		r07, r10              = "R-LAYOUT-07", "R-LAYOUT-10"
		holidaysDir, otherDir = "2024/05-01 - Holidays", "2024/05-03 - Other"
		aDir, bDir            = "2024/05-04 - A", "2024/05-05 - B"
	)
	std := domain.DefaultPolicies()

	holidays := domain.Album{ID: 1, FolderTitles: []string{"2024"}, Title: "05/01 Holidays"}
	trips := domain.Album{ID: 2, Kind: domain.KindFolder, FolderTitles: []string{"2024"}, Title: "Trips"}
	year := domain.Album{ID: 3, Kind: domain.KindFolder, Title: "2024"}
	pets := domain.Album{ID: 4, Title: "Pets"}
	party := domain.Album{ID: 5, FolderTitles: []string{"2024"}, Title: "05/03 Party"}
	old := domain.Album{ID: 6, FolderTitles: []string{"2024"}, Title: "05/02 Old", InTrash: true}
	colon := domain.Album{ID: 7, FolderTitles: []string{"2024"}, Title: "05/01 a:b"}
	bare := domain.Album{ID: 8, FolderTitles: []string{"2024"}, Title: "05/01"}

	in := func(dir string, n int) []string {
		files := make([]string, n)
		for i := range files {
			files[i] = path.Join(dir, fmt.Sprintf("IMG_%04d.HEIC", i))
		}
		return files
	}
	set := func(dirs ...string) map[string]bool {
		m := map[string]bool{}
		for _, d := range dirs {
			m[d] = true
		}
		return m
	}
	pair := func(id domain.AlbumID, dir string) domain.AlbumPair {
		return domain.AlbumPair{AlbumID: id, DirPath: dir, CreatedBy: domain.OriginRemote}
	}
	existing := func(id domain.AlbumID, dir string, by domain.PairOrigin) []domain.AlbumPair {
		return []domain.AlbumPair{{AlbumID: id, DirPath: dir, CreatedBy: by}}
	}

	cases := []struct {
		rule, name string
		albums     []domain.Album
		pairs      []domain.AlbumPair
		dirs       map[string]bool
		files      map[domain.AlbumID][]string
		want       []domain.AlbumPair
	}{
		{r10, "an album takes the directory of its name", []domain.Album{holidays}, nil, set(holidaysDir), nil,
			[]domain.AlbumPair{pair(1, holidaysDir)}},
		{r10, "a year folder takes the directory of its name", []domain.Album{year}, nil, set("2024"), nil,
			[]domain.AlbumPair{pair(3, "2024")}},
		{r10, "an album without a description takes the bare day", []domain.Album{bare}, nil, set("2024/05-01"), nil,
			[]domain.AlbumPair{pair(8, "2024/05-01")}},
		{r10, "a thematic album takes no directory by name", []domain.Album{pets}, nil, set("Albums/Pets", "Pets"), nil,
			nil},
		{r10, "a folder that the template does not cover takes none", []domain.Album{trips}, nil, set("2024/Trips"), nil,
			nil},
		{r10, "the name compares exactly", []domain.Album{holidays}, nil, set("2024/05-01 - holidays"), nil,
			nil},
		{r10, "the name wins over the files", []domain.Album{holidays}, nil, set(holidaysDir, aDir),
			map[domain.AlbumID][]string{1: in(aDir, 3)},
			[]domain.AlbumPair{pair(1, holidaysDir)}},
		{r10, "51 of 100 files are more than half", []domain.Album{party}, nil, set(otherDir, aDir, bDir),
			map[domain.AlbumID][]string{5: slices.Concat(in(otherDir, 51), in(aDir, 25), in(bDir, 24))},
			[]domain.AlbumPair{pair(5, otherDir)}},
		{r10, "50 of 100 files are not more than half", []domain.Album{party}, nil, set(otherDir, aDir, bDir),
			map[domain.AlbumID][]string{5: slices.Concat(in(otherDir, 50), in(aDir, 25), in(bDir, 25))},
			nil},
		{r10, "49 of 100 files are not more than half", []domain.Album{party}, nil, set(otherDir, aDir, bDir),
			map[domain.AlbumID][]string{5: slices.Concat(in(otherDir, 49), in(aDir, 26), in(bDir, 25))},
			nil},
		{r10, "a directory outside the template takes no pair by files", []domain.Album{party}, nil, set("Archive"),
			map[domain.AlbumID][]string{5: in("Archive", 3)},
			nil},
		{r10, "a year folder takes no pair by files", []domain.Album{party}, nil, set("2024"),
			map[domain.AlbumID][]string{5: in("2024", 3)},
			nil},
		{r10, "the root takes no pair", []domain.Album{party}, nil, nil,
			map[domain.AlbumID][]string{5: in("", 3)},
			nil},
		{r10, "a directory below an event takes no pair by files", []domain.Album{party}, nil, set(holidaysDir + "/raw"),
			map[domain.AlbumID][]string{5: in(holidaysDir+"/raw", 3)},
			nil},
		{r10, "a former day folder pairs by its files", []domain.Album{party}, nil, set("2024/05-01"),
			map[domain.AlbumID][]string{5: in("2024/05-01", 3)},
			[]domain.AlbumPair{pair(5, "2024/05-01")}},
		{r10, "the most files take a directory that several albums claim", []domain.Album{holidays, party}, nil, set(aDir),
			map[domain.AlbumID][]string{1: in(aDir, 2), 5: in(aDir, 3)},
			[]domain.AlbumPair{pair(5, aDir)}},
		{r10, "equal files: the lower ID", []domain.Album{party, holidays}, nil, set(aDir),
			map[domain.AlbumID][]string{1: in(aDir, 2), 5: slices.Concat(in(aDir, 2), in(bDir, 1))},
			[]domain.AlbumPair{pair(1, aDir)}},
		{r10, "an album in the trash gets none", []domain.Album{old}, nil, set("2024/05-02 - Old"),
			map[domain.AlbumID][]string{6: in("2024/05-02 - Old", 3)},
			nil},
		{r07, "a paired album is not paired again", []domain.Album{holidays}, existing(1, aDir, domain.OriginICloud), set(holidaysDir),
			map[domain.AlbumID][]string{1: in(holidaysDir, 3)},
			nil},
		{r07, "a paired directory is not taken by name", []domain.Album{holidays}, existing(9, holidaysDir, domain.OriginICloud), set(holidaysDir), nil,
			nil},
		{r07, "a paired directory is not taken by files", []domain.Album{party}, existing(9, aDir, domain.OriginICloud), set(aDir),
			map[domain.AlbumID][]string{5: in(aDir, 3)},
			nil},
		{r10, "a description that sanitizing changes pairs by the changed name", []domain.Album{colon}, nil, set("2024/05-01 - a꞉b"), nil,
			[]domain.AlbumPair{pair(7, "2024/05-01 - a꞉b")}},
	}
	for _, c := range cases {
		t.Run(c.rule+" "+c.name, func(t *testing.T) {
			t.Parallel()

			if _, ok := domain.Rules[c.rule]; !ok {
				t.Errorf("%s is not in domain.Rules", c.rule)
			}
			got := layout.FirstPairs(c.albums, c.pairs, c.dirs, c.files, std)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("FirstPairs(%+v, %+v, %v) = %+v, want %+v", c.albums, c.pairs, c.dirs, got, c.want)
			}
			reversed := slices.Clone(c.albums)
			slices.Reverse(reversed)
			if again := layout.FirstPairs(reversed, c.pairs, c.dirs, c.files, std); !reflect.DeepEqual(again, got) {
				t.Errorf("FirstPairs with the albums reversed = %+v, want %+v", again, got)
			}
		})
	}
}

// covers: R-LAYOUT-07
// covers: INV-13
func TestFirstPairsProperties(t *testing.T) {
	t.Parallel()

	type world struct {
		albums []domain.Album
		pairs  []domain.AlbumPair
		dirs   map[string]bool
		files  map[domain.AlbumID][]string
	}
	p := domain.DefaultPolicies()
	places := rapid.SampledFrom([][]string{nil, {"2024"}, {"2024", "Trips"}})
	titles := rapid.SampledFrom([]string{"05/01 Holidays", "05/02 Trip", "05/01", "Pets", "2024"})
	vocab := []string{
		"", "2024", "2024/Trips", "2024/05-01", "2024/05-01 - Holidays", "2024/05-02 - Trip",
		"2024/Pets", "2024/05-01 - Holidays/raw", "Albums/Pets", "Archive",
	}
	dirOf := rapid.SampledFrom(vocab)
	bools := rapid.SampledFrom([]bool{false, true})
	trashed := rapid.SampledFrom([]bool{false, false, false, true})
	worlds := rapid.Custom(func(t *rapid.T) world {
		w := world{dirs: map[string]bool{}, files: map[domain.AlbumID][]string{}}
		taken := map[string]bool{}
		for i := range rapid.IntRange(0, 6).Draw(t, "albums") {
			a := domain.Album{
				ID:           domain.AlbumID(i + 1),
				FolderTitles: places.Draw(t, "folders"),
				Title:        titles.Draw(t, "title"),
				InTrash:      trashed.Draw(t, "inTrash"),
			}
			if bools.Draw(t, "isFolder") {
				a.Kind = domain.KindFolder
			}
			w.albums = append(w.albums, a)
			if j := rapid.IntRange(-1, len(vocab)-1).Draw(t, "pair"); j >= 0 && !taken[vocab[j]] {
				taken[vocab[j]] = true
				w.pairs = append(w.pairs, domain.AlbumPair{AlbumID: a.ID, DirPath: vocab[j], CreatedBy: domain.OriginICloud})
			}
			for k, d := range rapid.SliceOfN(dirOf, 0, 6).Draw(t, "files") {
				w.files[a.ID] = append(w.files[a.ID], path.Join(d, fmt.Sprintf("IMG_%d_%d.HEIC", a.ID, k)))
			}
		}
		for _, d := range rapid.SliceOfN(dirOf, 0, 10).Draw(t, "dirs") {
			w.dirs[d] = true
		}
		return w
	})
	show := func(w world) string {
		counts := map[domain.AlbumID]int{}
		for id, f := range w.files {
			counts[id] = len(f)
		}
		return fmt.Sprintf("albums %+v, pairs %+v, dirs %v, files per album %v", w.albums, w.pairs, w.dirs, counts)
	}
	holdsMost := func(files []string, dir string) bool {
		n := 0
		for _, f := range files {
			if path.Dir(f) == dir {
				n++
			}
		}
		return n*2 > len(files)
	}

	t.Run("pairs are new, one to one and in order", func(t *testing.T) {
		t.Parallel()
		rapid.Check(t, func(t *rapid.T) {
			w := worlds.Draw(t, "world")
			got := layout.FirstPairs(w.albums, w.pairs, w.dirs, w.files, p)

			byID := map[domain.AlbumID]domain.Album{}
			for _, a := range w.albums {
				byID[a.ID] = a
			}
			oldAlbums, oldDirs := map[domain.AlbumID]bool{}, map[string]bool{}
			for _, o := range w.pairs {
				oldAlbums[o.AlbumID], oldDirs[o.DirPath] = true, true
			}
			seen := map[string]bool{}
			for i, g := range got {
				if i > 0 && got[i-1].AlbumID >= g.AlbumID {
					t.Fatalf("%s: FirstPairs = %+v: AlbumIDs do not rise strictly", show(w), got)
				}
				if seen[g.DirPath] {
					t.Fatalf("%s: FirstPairs = %+v: %q is paired twice", show(w), got, g.DirPath)
				}
				seen[g.DirPath] = true
				if oldAlbums[g.AlbumID] || oldDirs[g.DirPath] {
					t.Fatalf("%s: FirstPairs = %+v: %+v pairs an album or a directory that has a pair", show(w), got, g)
				}
				a, ok := byID[g.AlbumID]
				if !ok || a.InTrash {
					t.Fatalf("%s: FirstPairs = %+v: %+v pairs no album or an album in the trash", show(w), got, g)
				}
				if g.CreatedBy != domain.OriginRemote || g.Remote != "" || g.Root != "" {
					t.Fatalf("%s: FirstPairs = %+v: %+v, want CreatedBy %q and no Remote or Root", show(w), got, g, domain.OriginRemote)
				}
				byName := g.DirPath == layout.DirFor(a, p) && w.dirs[g.DirPath]
				if !byName && !holdsMost(w.files[g.AlbumID], g.DirPath) {
					t.Fatalf("%s: FirstPairs = %+v: %+v is neither the name of the album nor holds most of its files", show(w), got, g)
				}
			}
		})
	})

	t.Run("a second call adds nothing, in any order", func(t *testing.T) {
		t.Parallel()
		rapid.Check(t, func(t *rapid.T) {
			w := worlds.Draw(t, "world")
			got := layout.FirstPairs(w.albums, w.pairs, w.dirs, w.files, p)

			if again := layout.FirstPairs(w.albums, slices.Concat(w.pairs, got), w.dirs, w.files, p); len(again) != 0 {
				t.Fatalf("%s: FirstPairs with its own pairs %+v = %+v, want none", show(w), got, again)
			}
			albums := slices.Clone(w.albums)
			slices.Reverse(albums)
			pairs := slices.Clone(w.pairs)
			slices.Reverse(pairs)
			if other := layout.FirstPairs(albums, pairs, w.dirs, w.files, p); !reflect.DeepEqual(other, got) {
				t.Fatalf("%s: FirstPairs with the albums and pairs reversed = %+v, want %+v", show(w), other, got)
			}
		})
	})
}
