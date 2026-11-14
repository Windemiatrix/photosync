package plan_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Each row is one asset: the decisions on its links, its links where their
// files lie now and the entries of the scan. The expected imports follow the
// rules of an import by a link, not the code, and must not depend on the
// order of the input.
func TestImportOps(t *testing.T) {
	t.Parallel()
	const (
		personal, shared            = domain.LibraryPersonal, domain.LibraryShared
		original, live, raw, source = domain.RoleOriginal, domain.RoleLiveVideo, domain.RoleRAWPair, domain.RoleSource
		holidaysDir, tripDir        = "2024/05-01 - Holidays", "2024/05-02 - Trip"

		holidays, trip, binned, gone domain.AlbumID = 10, 11, 12, 99
	)
	type (
		decided = map[string][]domain.LinkDecision
		links   = []domain.Link
		files   = map[domain.LinkID]domain.RemoteEntry
		ops     = []domain.Op
	)
	settled := time.Date(2026, 11, 13, 11, 0, 0, 0, time.UTC)
	albums := []domain.Album{{ID: holidays, Title: "05/01 Holidays"}, {ID: trip, Title: "05/02 Trip"}, {ID: binned, Title: "05/04 Party", InTrash: true}}
	entry := func(root domain.LibraryKind, at string, size int64, md5 string) domain.RemoteEntry {
		return domain.RemoteEntry{Root: root, Path: at, Size: size, MD5: md5, SHA256: "s" + md5, ETag: "e" + md5, MTime: settled}
	}
	link := func(id domain.LinkID, remote string, e domain.RemoteEntry, role domain.LinkRole, album domain.AlbumID) domain.Link {
		return domain.Link{
			ID: id, AssetID: 7, Remote: remote, Root: e.Root, Path: e.Path, Role: role, AlbumID: album,
			Size: e.Size, SHA256: e.SHA256, MD5: e.MD5, ETag: e.ETag, Level: domain.LevelExact, Status: domain.StatusActive,
		}
	}
	at := func(l domain.Link, e domain.RemoteEntry, album domain.AlbumID) domain.Link {
		l.Root, l.Path, l.AlbumID = e.Root, e.Path, album
		return l
	}
	away := func(l domain.Link) domain.Link {
		l.Status, l.QuarantinePath = domain.StatusQuarantinedRemote, ".photosync/quarantine/2026-08-01/"+l.Path
		return l
	}
	decide := func(action domain.LinkAction, rule string, l domain.Link) domain.LinkDecision {
		return domain.LinkDecision{Link: l, Action: action, Rule: rule}
	}
	imported := func(l domain.Link, e domain.RemoteEntry, album domain.AlbumID) domain.Link {
		l.AssetID, l.Root, l.Path, l.AlbumID, l.Level, l.Status, l.QuarantinePath = 0, e.Root, e.Path, album, domain.LevelImported, domain.StatusActive, ""
		l.Size, l.SHA256, l.MD5, l.ETag, l.MTime = e.Size, e.SHA256, e.MD5, e.ETag, e.MTime
		return l
	}
	importing := func(rule string, album domain.AlbumID, ls ...domain.Link) domain.Op {
		return domain.Op{Kind: domain.OpImport, Remote: ls[0].Remote, Root: ls[0].Root, Src: ls[0].Path, AlbumID: album, Rule: rule, Links: ls}
	}

	base1, new1 := entry(personal, holidaysDir+"/IMG_0001.HEIC", 100, "a1"), entry(personal, holidaysDir+"/IMG_0001.HEIC", 120, "b1")
	mov1, dng1 := entry(personal, holidaysDir+"/IMG_0001.MOV", 30, "v1"), entry(personal, holidaysDir+"/IMG_0001.DNG", 300, "r1")
	mov2, copy1 := entry(personal, holidaysDir+"/IMG_0002.MOV", 30, "v2"), entry(personal, tripDir+"/IMG_0001.HEIC", 100, "a1")
	photo1, live1, raw1 := link(1, "alpha", base1, original, holidays), link(2, "alpha", mov1, live, holidays), link(3, "alpha", dng1, raw, holidays)
	live2, copied1 := link(4, "alpha", mov2, live, holidays), link(5, "alpha", copy1, original, trip)
	base3, moved3 := entry(shared, holidaysDir+"/IMG_0003.HEIC", 100, "c3"), entry(shared, tripDir+"/IMG_0003.HEIC", 100, "c3")
	baseMov3, mov3 := entry(shared, holidaysDir+"/IMG_0003.MOV", 30, "v3"), entry(shared, tripDir+"/IMG_0003.MOV", 30, "v3")
	base8, new8 := entry(personal, holidaysDir+"/IMG_0008.HEIC", 100, "a8"), entry(personal, holidaysDir+"/IMG_0008.HEIC", 150, "b8")
	photo3, live3, photo8 := link(6, "beta", base3, original, holidays), link(7, "beta", baseMov3, live, holidays), link(8, "beta", base8, original, holidays)
	e9, m9 := entry(personal, holidaysDir+"/IMG_0009.HEIC", 100, "a9"), entry(personal, holidaysDir+"/IMG_0009.MOV", 30, "v9")
	photo9, live9 := link(9, "alpha", e9, original, holidays), link(10, "alpha", m9, live, holidays)
	e11, e12 := entry(personal, "2024/05-04 - Party/IMG_0011.HEIC", 100, "a11"), entry(personal, "2024/05-05 - Zoo/IMG_0012.AVI", 900, "a12")
	photo11, video12 := link(11, "alpha", e11, original, binned), link(12, "alpha", e12, source, gone)

	for _, c := range []struct {
		name        string
		decisions   decided
		links       links
		files       files
		forPersonal bool
		want        ops
	}{
		{"nothing at all", nil, nil, nil, true, nil},
		{"a photo that the user changed while its asset is gone from iCloud comes back as a new asset in the album of its link, with the Live Photo video and the RAW file beside it, and another file of the asset that goes to quarantine is not imported",
			decided{"alpha": {decide(domain.ActionImport, "R-LINK-11", photo1), decide(domain.ActionQuarantineFile, "R-STATUS-01", copied1)}},
			links{raw1, copied1, photo1, live2, live1}, files{1: new1, 2: mov1, 3: dng1, 4: mov2, 5: copy1}, true,
			ops{importing("R-LINK-11", holidays, imported(photo1, new1, holidays), imported(live1, mov1, holidays), imported(raw1, dng1, holidays))}}, // covers: R-LINK-11
		{"files that the user moved or changed come back from where they lie now, the one under the personal root into no album while that root sets no albums",
			decided{"beta": {decide(domain.ActionImport, "R-LINK-11", photo3), decide(domain.ActionImport, "R-LINK-11", photo8)}},
			links{at(photo3, moved3, trip), at(live3, mov3, trip), photo8}, files{6: moved3, 7: mov3, 8: new8}, false,
			ops{
				importing("R-LINK-11", 0, imported(photo8, new8, 0)),
				importing("R-LINK-11", trip, imported(photo3, moved3, trip), imported(live3, mov3, trip)),
			}}, // covers: R-MODE-04
		{"a photo and its Live Photo video that the user brings back from quarantine come back as one new asset",
			decided{"alpha": {decide(domain.ActionImport, "R-LINK-02", away(photo9)), decide(domain.ActionImport, "R-LINK-02", away(live9))}},
			links{photo9, live9}, files{9: e9, 10: m9}, true,
			ops{importing("R-LINK-02", holidays, imported(photo9, e9, holidays), imported(live9, m9, holidays))}}, // covers: R-STATUS-04
		{"a file brought back from quarantine whose album is in Recently Deleted or gone comes back into no album, and so does a video that Photos does not take",
			decided{"alpha": {decide(domain.ActionImport, "R-LINK-02", away(photo11)), decide(domain.ActionImport, "R-LINK-02", away(video12))}},
			links{photo11, video12}, files{11: e11, 12: e12}, true,
			ops{importing("R-LINK-02", 0, imported(photo11, e11, 0)), importing("R-LINK-02", 0, imported(video12, e12, 0))}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			input := fmt.Sprintf("%+v %+v %+v %+v", c.decisions, c.links, c.files, albums)
			got := plan.ImportOps(c.decisions, c.links, c.files, albums, c.forPersonal)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("ImportOps =\n%+v\nwant\n%+v", got, c.want)
			}
			for _, o := range c.want {
				if _, ok := domain.Rules[o.Rule]; !ok {
					t.Errorf("%s is not in domain.Rules", o.Rule)
				}
			}
			reversed, ls, as := decided{}, slices.Clone(c.links), slices.Clone(albums)
			for name, ds := range c.decisions {
				reversed[name] = slices.Clone(ds)
				slices.Reverse(reversed[name])
			}
			slices.Reverse(ls)
			slices.Reverse(as)
			if again := plan.ImportOps(reversed, ls, c.files, as, c.forPersonal); !reflect.DeepEqual(again, got) {
				t.Errorf("with the input reversed:\n%+v\nwant\n%+v", again, got)
			}
			for _, o := range got {
				for i := range o.Links {
					o.Links[i].Path = "changed"
				}
			}
			if after := fmt.Sprintf("%+v %+v %+v %+v", c.decisions, c.links, c.files, albums); after != input {
				t.Errorf("ImportOps changed its input or shares a slice with it:\n%s\nwant\n%s", after, input)
			}
		})
	}
}
