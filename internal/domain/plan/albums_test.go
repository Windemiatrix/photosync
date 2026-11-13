package plan_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Each row gives the albums and pairs of a run, the directories of one root
// of a remote that hold main files, and the files of the albums there. The
// expected pairs and albums are written from the rules on album pairs, the
// layout of the cloud, the modes and the albums of the personal root, not
// from the code, and must not depend on the order of the albums and pairs.
func TestPairDirs(t *testing.T) {
	t.Parallel()
	const (
		year, hobby                                  domain.AlbumID = 1, 2
		holidays, trip, pets, archive, old, printing domain.AlbumID = 10, 11, 12, 13, 14, 15
		next                                         domain.AlbumID = 16

		personal, shared             = domain.LibraryPersonal, domain.LibraryShared
		folder, event, thematic      = domain.KindFolder, domain.KindEvent, domain.KindThematic
		byICloud, byRemote, byImport = domain.OriginICloud, domain.OriginRemote, domain.OriginImport
		twoWay, uploadOnly, downOnly = domain.ModeTwoWay, domain.ModeUploadOnly, domain.ModeDownloadOnly
	)
	album := func(id domain.AlbumID, kind domain.AlbumKind, parent domain.AlbumID, title string, folders ...string) domain.Album {
		return domain.Album{ID: id, Kind: kind, Title: title, ParentID: parent, FolderTitles: folders}
	}
	lib := func(more ...domain.Album) []domain.Album {
		return append([]domain.Album{
			album(year, folder, 0, "2024"),
			album(hobby, folder, 0, "Hobby"),
			album(holidays, event, year, "05/01 Holidays", "2024"),
			album(trip, event, year, "05/02 Trip", "2024"),
			album(pets, thematic, 0, "Pets"),
			album(archive, thematic, 0, "Archive"),
			album(printing, thematic, hobby, "3D printing", "Hobby"),
		}, more...)
	}
	trashed := album(old, event, year, "05/03 Old", "2024")
	trashed.InTrash = true
	pair := func(id domain.AlbumID, remote string, root domain.LibraryKind, dir string, by domain.PairOrigin) domain.AlbumPair {
		return domain.AlbumPair{AlbumID: id, Remote: remote, Root: root, DirPath: dir, CreatedBy: by}
	}
	mine := func(id domain.AlbumID, dir string) domain.AlbumPair { return pair(id, "disk", personal, dir, byRemote) }
	prs := func(x ...domain.AlbumPair) []domain.AlbumPair { return x }
	adds := func(a ...domain.Album) []domain.Album { return a }
	in := func(dirs ...string) map[string]bool {
		set := map[string]bool{}
		for _, d := range dirs {
			set[d] = true
		}
		return set
	}
	type scan = map[domain.AlbumID][]string
	p := domain.DefaultPolicies()
	p.UntitledEvent, p.AlbumsRoot = "Untitled", "Albums"
	noAlbums := p
	noAlbums.AlbumsForPersonal = false
	show := func(made []domain.AlbumPair, added []domain.Album) []string {
		var out []string
		for _, x := range made {
			out = append(out, fmt.Sprintf("pair %d %s %s:%s %s", x.AlbumID, x.Remote, x.Root, x.DirPath, x.CreatedBy))
		}
		for _, a := range added {
			out = append(out, fmt.Sprintf("album %d %s %q in %q parent %d; %+v", a.ID, a.Kind, a.Title, a.FolderTitles, a.ParentID, a))
		}
		return out
	}
	for _, c := range []struct {
		name   string
		albums []domain.Album
		pairs  []domain.AlbumPair
		last   domain.AlbumID
		mode   domain.Mode
		root   domain.LibraryKind
		p      domain.Policies
		dirs   map[string]bool
		files  scan
		made   []domain.AlbumPair
		added  []domain.Album
	}{
		{"a paired directory stays as it is", lib(), prs(pair(holidays, "disk", personal, "2024/05-01 - Holidays", byICloud)), 0, twoWay, personal, p,
			in("2024/05-01 - Holidays"), scan{holidays: {"2024/05-01 - Holidays/IMG_0001.HEIC"}}, nil, nil},
		{"an album pairs with the directory of its name", lib(), nil, 0, twoWay, personal, p,
			in("2024/05-02 - Trip"), nil, prs(mine(trip, "2024/05-02 - Trip")), nil},
		{"an album pairs by name with a directory that holds only subdirectories", lib(), nil, 0, twoWay, personal, p,
			in("2024/05-02 - Trip/raw"), nil,
			prs(mine(trip, "2024/05-02 - Trip"), mine(next, "2024/05-02 - Trip/raw")), adds(album(next, event, year, "05/02 Trip / raw", "2024"))},
		{"an album pairs with the directory that holds most of its files", lib(), nil, 0, twoWay, personal, p,
			in("2024/Holiday trip"), scan{holidays: {"2024/Holiday trip/IMG_0001.HEIC", "2024/Holiday trip/IMG_0002.HEIC"}},
			prs(mine(holidays, "2024/Holiday trip")), nil},
		{"folders get no pair", lib(), nil, 0, twoWay, personal, p,
			in("2024/05-02 - Trip", "Albums/Hobby/3D printing"), nil,
			prs(mine(trip, "2024/05-02 - Trip"), mine(printing, "Albums/Hobby/3D printing")), nil},
		{"the pairs of another remote or root do not count", lib(),
			prs(pair(trip, "other", personal, "2024/05-02 - Trip", byICloud), pair(trip, "disk", shared, "2024/05-02 - Trip", byICloud)), 0, twoWay, personal, p,
			in("2024/05-02 - Trip"), nil, prs(mine(trip, "2024/05-02 - Trip")), nil},
		{"an album paired elsewhere leaves the directory of its name to a new album", lib(),
			prs(pair(holidays, "disk", personal, "2024/Holiday trip", byICloud)), 0, twoWay, personal, p,
			in("2024/05-01 - Holidays"), nil, prs(mine(next, "2024/05-01 - Holidays")), adds(album(next, event, year, "05/01 Holidays", "2024"))},
		{"a new directory in a year folder is a new event", lib(), nil, 0, twoWay, personal, p,
			in("2024/05-04 - Party"), nil, prs(mine(next, "2024/05-04 - Party")), adds(album(next, event, year, "05/04 Party", "2024"))},
		{"a new year gets one folder for its new events", lib(), nil, 0, twoWay, personal, p,
			in("2025/06-01 - Beach", "2025/06-02 - Hike"), nil,
			prs(mine(next+1, "2025/06-01 - Beach"), mine(next+2, "2025/06-02 - Hike")),
			adds(album(next, folder, 0, "2025"), album(next+1, event, next, "06/01 Beach", "2025"), album(next+2, event, next, "06/02 Hike", "2025"))},
		{"a new directory under the albums root is a new album at the top level", lib(), nil, 0, twoWay, personal, p,
			in("Albums/Cats"), nil, prs(mine(next, "Albums/Cats")), adds(album(next, thematic, 0, "Cats"))},
		{"a new album in a folder takes the title of an album outside it", lib(), nil, 0, twoWay, personal, p,
			in("Albums/Hobby/Pets"), nil, prs(mine(next, "Albums/Hobby/Pets")), adds(album(next, thematic, hobby, "Pets", "Hobby"))},
		{"a new folder in a year folder", lib(), nil, 0, twoWay, personal, p,
			in("2024/Trips/06-01 - Kazan"), nil, prs(mine(next+1, "2024/Trips/06-01 - Kazan")),
			adds(album(next, folder, year, "Trips", "2024"), album(next+1, event, next, "06/01 Kazan", "2024", "Trips"))},
		{"a top-level cloud directory pairs with the album of its name", lib(), nil, 0, twoWay, personal, p,
			in("Archive"), nil, prs(mine(archive, "Archive")), nil},
		{"an album in the trash is not taken", lib(trashed), nil, 0, twoWay, personal, p,
			in("2024/05-03 - Old"), nil, prs(mine(next, "2024/05-03 - Old")), adds(album(next, event, year, "05/03 Old", "2024"))},
		{"a folder is not taken for an album", lib(), nil, 0, twoWay, personal, p,
			in("Hobby"), nil, prs(mine(next, "Hobby")), adds(album(next, thematic, 0, "Hobby"))},
		{"a directory below a paired album is a part of its title", lib(),
			prs(pair(holidays, "disk", personal, "2024/Holiday trip", byICloud)), 0, twoWay, personal, p,
			in("2024/Holiday trip/05-02 - Day two/raw"), nil,
			prs(mine(next, "2024/Holiday trip/05-02 - Day two/raw")), adds(album(next, event, year, "05/01 Holidays / 05/02 Day two / raw", "2024"))},
		{"a directory below two paired albums is a part of the nearer one", lib(),
			prs(pair(holidays, "disk", personal, "2024/Holiday trip", byICloud), pair(trip, "disk", personal, "2024/Holiday trip/Sub", byICloud)), 0, twoWay, personal, p,
			in("2024/Holiday trip/Sub/raw"), nil,
			prs(mine(next, "2024/Holiday trip/Sub/raw")), adds(album(next, event, year, "05/02 Trip / raw", "2024"))},
		{"a part below a new album takes its title", lib(), nil, 0, twoWay, personal, p,
			in("2025/06-01 - Beach", "2025/06-01 - Beach/raw"), nil,
			prs(mine(next+1, "2025/06-01 - Beach"), mine(next+2, "2025/06-01 - Beach/raw")),
			adds(album(next, folder, 0, "2025"), album(next+1, event, next, "06/01 Beach", "2025"), album(next+2, event, next, "06/01 Beach / raw", "2025"))},
		{"a folder paired with its directory is no album above the parts below it", lib(),
			prs(pair(year, "disk", personal, "2024", byImport)), 0, twoWay, personal, p,
			in("2024/05-04 - Party"), nil, prs(mine(next, "2024/05-04 - Party")), adds(album(next, event, year, "05/04 Party", "2024"))},
		{"upload-only: albums pair by name and title, none is added", lib(), nil, 0, uploadOnly, personal, p,
			in("2024/05-02 - Trip", "2024/05-04 - Party", "Archive"), nil,
			prs(mine(trip, "2024/05-02 - Trip"), mine(archive, "Archive")), nil},
		{"download-only: a new directory adds an album", lib(), nil, 0, downOnly, personal, p,
			in("2024/05-04 - Party"), nil, prs(mine(next, "2024/05-04 - Party")), adds(album(next, event, year, "05/04 Party", "2024"))},
		{"albums_for_personal false: the personal root adds no album", lib(), nil, 0, twoWay, personal, noAlbums,
			in("2024/05-04 - Party"), nil, nil, nil},
		{"albums_for_personal false: the personal root pairs albums by name and title", lib(), nil, 0, twoWay, personal, noAlbums,
			in("2024/05-02 - Trip", "2024/05-04 - Party", "Archive"), nil,
			prs(mine(trip, "2024/05-02 - Trip"), mine(archive, "Archive")), nil},
		{"albums_for_personal false: the shared root adds albums", lib(), nil, 0, twoWay, shared, noAlbums,
			in("2024/05-04 - Party"), nil,
			prs(pair(next, "disk", shared, "2024/05-04 - Party", byRemote)), adds(album(next, event, year, "05/04 Party", "2024"))},
		{"new IDs go above the last ID of the state", lib(), nil, 30, twoWay, personal, p,
			in("Albums/Cats"), nil, prs(mine(31, "Albums/Cats")), adds(album(31, thematic, 0, "Cats"))},
		{"the root, the albums root, a year folder and a day folder stand for no album", lib(), nil, 0, twoWay, personal, p,
			in(".", "Albums", "2024", "2024/05-01"), nil, nil, nil},
		{"a subdirectory of a day folder is a part of the untitled event of the day", lib(), nil, 0, twoWay, personal, p,
			in("2024/05-01/raw"), nil, prs(mine(next, "2024/05-01/raw")), adds(album(next, event, year, "05/01 / raw", "2024"))},
		{"two directories with one album path get two albums", lib(), nil, 0, twoWay, personal, p,
			in("Albums/Cats", "Cats"), nil,
			prs(mine(next, "Albums/Cats"), mine(next+1, "Cats")), adds(album(next, thematic, 0, "Cats"), album(next+1, thematic, 0, "Cats"))},
		{"of two albums with one title the lower ID takes the directory", append([]domain.Album{album(20, thematic, 0, "Archive")}, lib()...), nil, 0, twoWay, personal, p,
			in("Archive"), nil, prs(mine(archive, "Archive")), nil},
		{"the pairs come in the order of AlbumID", lib(), nil, 0, twoWay, personal, p,
			in("2024/05-02 - Trip", "Albums/Cats", "Archive"), nil,
			prs(mine(trip, "2024/05-02 - Trip"), mine(archive, "Archive"), mine(next, "Albums/Cats")), adds(album(next, thematic, 0, "Cats"))},
		{"nothing in, nothing out", nil, nil, 0, twoWay, personal, p, nil, nil, nil, nil},
		{"an album that a call for another root added pairs here by its name", lib(album(next, folder, 0, "2025"), album(next+1, event, next, "06/01 Beach", "2025")),
			prs(pair(next+1, "disk", personal, "2025/06-01 - Beach", byRemote)), 0, twoWay, shared, p,
			in("2025/06-01 - Beach"), nil, prs(pair(next+1, "disk", shared, "2025/06-01 - Beach", byRemote)), nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := domain.RemoteSpec{Name: "disk", Mode: c.mode}
			input := fmt.Sprintf("%+v %+v %v %v", c.albums, c.pairs, c.dirs, c.files)
			made, added := plan.PairDirs(c.albums, c.pairs, c.last, r, c.root, c.dirs, c.files, c.p)
			if got, want := show(made, added), show(c.made, c.added); !slices.Equal(got, want) {
				t.Errorf("PairDirs = %q; want %q", got, want)
			}
			more, again := plan.PairDirs(slices.Concat(c.albums, added), slices.Concat(c.pairs, made), c.last, r, c.root, c.dirs, c.files, c.p)
			if len(more)+len(again) > 0 {
				t.Errorf("a second call with the albums and pairs of the first = %q; want none", show(more, again))
			}
			albums, pairs := slices.Clone(c.albums), slices.Clone(c.pairs)
			slices.Reverse(albums)
			slices.Reverse(pairs)
			if back, also := plan.PairDirs(albums, pairs, c.last, r, c.root, c.dirs, c.files, c.p); !slices.Equal(show(back, also), show(made, added)) {
				t.Errorf("with the albums and pairs reversed: %q; want %q", show(back, also), show(made, added))
			}
			for _, a := range added {
				for i := range a.FolderTitles {
					a.FolderTitles[i] = "changed"
				}
			}
			if after := fmt.Sprintf("%+v %+v %v %v", c.albums, c.pairs, c.dirs, c.files); after != input {
				t.Errorf("PairDirs changed its input or shares a slice with it:\n%s\nwant\n%s", after, input)
			}
		})
	}
}
