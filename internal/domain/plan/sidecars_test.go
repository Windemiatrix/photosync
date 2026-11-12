package plan_test

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// A sidecar goes with the main file whose link names its directory and stem,
// found by where it lies now: neither its content nor a move or a deletion of
// its own makes it go anywhere. Unless a row says otherwise, the photo
// IMG_0001.HEIC of the Holidays event has an XMP beside it. The order of
// links and scan does not change the result (INV-9).
func TestSidecars(t *testing.T) {
	t.Parallel()
	const (
		dir, trip = "2024/05-01 - Holidays/", "2024/05-02 - Trip/"
		itsXMP    = "1 " + dir + "IMG_0001.xmp"
	)
	photo := domain.Link{
		ID: 1, AssetID: 7, Remote: "disk", Root: domain.LibraryPersonal, Path: dir + "IMG_0001.HEIC",
		Role: domain.RoleOriginal, AlbumID: 10, Status: domain.StatusActive,
	}
	other := func(id domain.LinkID, edit func(l *domain.Link)) domain.Link {
		l := photo
		l.ID = id
		edit(&l)
		return l
	}
	file := func(p string) domain.RemoteEntry {
		return domain.RemoteEntry{Root: domain.LibraryPersonal, Path: p, Size: 4096, ETag: "1"}
	}
	shared := func(e domain.RemoteEntry) domain.RemoteEntry {
		e.Root = domain.LibraryShared
		return e
	}
	heic, xmp := file(dir+"IMG_0001.HEIC"), file(dir+"IMG_0001.xmp")
	listed := func(m map[domain.LinkID][]domain.RemoteEntry) []string {
		var out []string
		for _, id := range slices.Sorted(maps.Keys(m)) {
			for _, e := range m[id] {
				out = append(out, fmt.Sprintf("%d %s", id, e.Path))
			}
		}
		return out
	}
	for _, c := range []struct {
		name  string
		links []domain.Link
		scan  []domain.RemoteEntry
		want  []string
	}{
		{"an XMP beside its photo", []domain.Link{photo}, []domain.RemoteEntry{heic, xmp}, []string{itsXMP}}, // covers: R-COMPONENT-04
		{"two sidecars, in the order of their paths", []domain.Link{photo}, []domain.RemoteEntry{xmp, heic, file(dir + "IMG_0001.AAE")},
			[]string{"1 " + dir + "IMG_0001.AAE", itsXMP}},
		{"the stem in lower case", []domain.Link{photo}, []domain.RemoteEntry{heic, file(dir + "img_0001.xmp")}, []string{"1 " + dir + "img_0001.xmp"}},
		{"a numbered name", []domain.Link{photo}, []domain.RemoteEntry{heic, file(dir + "IMG_0001 (1).xmp")}, nil},
		{"another stem", []domain.Link{photo}, []domain.RemoteEntry{heic, file(dir + "IMG_0002.xmp")}, nil},
		{"a sidecar moved on its own to another directory", []domain.Link{photo}, []domain.RemoteEntry{heic, file(trip + "IMG_0001.xmp")}, nil}, // covers: R-COMPONENT-05
		{"a sidecar with new content", []domain.Link{photo},
			[]domain.RemoteEntry{heic, {Root: domain.LibraryPersonal, Path: dir + "IMG_0001.xmp", Size: 8192, ETag: "2"}}, []string{itsXMP}},
		{"a sidecar deleted on its own", []domain.Link{photo}, []domain.RemoteEntry{heic}, nil},
		{"the photo moved away by the user", []domain.Link{photo}, []domain.RemoteEntry{file(trip + "IMG_0001.HEIC"), xmp}, []string{itsXMP}},
		{"the directory in another case", []domain.Link{photo}, []domain.RemoteEntry{heic, file("2024/05-01 - holidays/IMG_0001.xmp")}, nil},
		{"a directory with a sidecar extension", []domain.Link{photo},
			[]domain.RemoteEntry{heic, {Root: domain.LibraryPersonal, Path: dir + "IMG_0001.zip", IsDir: true}}, nil},
		{"the photo in the root of the remote", []domain.Link{other(1, func(l *domain.Link) { l.Path = "IMG_0001.HEIC" })},
			[]domain.RemoteEntry{file("IMG_0001.HEIC"), file("IMG_0001.xmp")}, []string{"1 IMG_0001.xmp"}},
		{"the same path in both roots", []domain.Link{photo, other(2, func(l *domain.Link) { l.AssetID, l.Root = 8, domain.LibraryShared })},
			[]domain.RemoteEntry{heic, shared(heic), shared(xmp)}, []string{"2 " + dir + "IMG_0001.xmp"}},
		{"the same photo in another remote", []domain.Link{photo, other(2, func(l *domain.Link) { l.Remote = "box" })},
			[]domain.RemoteEntry{heic, xmp}, []string{itsXMP}},
		{"a photo with the video of its Live Photo", []domain.Link{photo, other(2, func(l *domain.Link) { l.Role, l.Path = domain.RoleLiveVideo, dir+"IMG_0001.MOV" })},
			[]domain.RemoteEntry{heic, file(dir + "IMG_0001.MOV"), xmp}, []string{itsXMP}},
		{"a photo with a lesser copy of its stem", []domain.Link{photo, other(2, func(l *domain.Link) { l.Role, l.Path = domain.RoleDerived, dir+"IMG_0001.JPG" })},
			[]domain.RemoteEntry{heic, file(dir + "IMG_0001.JPG"), xmp}, []string{itsXMP}},
		{"the source of a converted video", []domain.Link{other(1, func(l *domain.Link) { l.Role, l.Path = domain.RoleSource, dir+"MVI_0001.AVI" })},
			[]domain.RemoteEntry{file(dir + "MVI_0001.AVI"), file(dir + "MVI_0001.THM")}, []string{"1 " + dir + "MVI_0001.THM"}},
		{"a photo in quarantine", []domain.Link{other(1, func(l *domain.Link) {
			l.Status, l.QuarantinePath = domain.StatusQuarantinedRemote, ".photosync/quarantine/2024-06-01/"+dir+"IMG_0001.HEIC"
		})}, []domain.RemoteEntry{xmp}, nil},
		{"two photos of one stem", []domain.Link{photo, other(2, func(l *domain.Link) { l.AssetID, l.Path = 8, dir+"IMG_0001.JPG" })},
			[]domain.RemoteEntry{heic, file(dir + "IMG_0001.JPG"), xmp}, nil},
	} {
		links, scan := slices.Clone(c.links), slices.Clone(c.scan)
		got := listed(plan.Sidecars(links, "disk", scan))
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: Sidecars = %q; want %q", c.name, got, c.want)
		}
		if !reflect.DeepEqual(links, c.links) || !reflect.DeepEqual(scan, c.scan) {
			t.Errorf("%s: Sidecars changed its input", c.name)
		}
		slices.Reverse(links)
		slices.Reverse(scan)
		if again := listed(plan.Sidecars(links, "disk", scan)); !slices.Equal(again, got) {
			t.Errorf("%s: with the input reversed, Sidecars = %q; want %q as before", c.name, again, got)
		}
	}
}

// A sidecar that follows its main file takes the name of that file where it
// ends up, and keeps its own extension as the cloud wrote it.
func TestSidecarPath(t *testing.T) {
	t.Parallel()
	const dir, trip = "2024/05-01 - Holidays/", "2024/05-02 - Trip/"
	for _, c := range []struct{ main, sidecar, want string }{
		{trip + "IMG_0001.HEIC", dir + "IMG_0001.xmp", trip + "IMG_0001.xmp"}, // covers: R-COMPONENT-04
		{trip + "IMG_0001.HEIC", dir + "IMG_0001.AAE", trip + "IMG_0001.AAE"},
		{trip + "IMG_0001 (1).HEIC", dir + "IMG_0001.xmp", trip + "IMG_0001 (1).xmp"},
		{trip + "IMG_0001.HEIC", dir + "img_0001.xmp", trip + "IMG_0001.xmp"},
		{"2024/05-02 - St. Ives/IMG_0001.HEIC", dir + "IMG_0001.xmp", "2024/05-02 - St. Ives/IMG_0001.xmp"},
		{"IMG_0001.HEIC", dir + "IMG_0001.xmp", "IMG_0001.xmp"},
	} {
		if got := plan.SidecarPath(c.main, c.sidecar); got != c.want {
			t.Errorf("SidecarPath(%q, %q) = %q; want %q", c.main, c.sidecar, got, c.want)
		}
	}
}
