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

// Each row is one asset: the decisions on its files and on their Live Photo
// videos and RAW files, its links where their files lie now, the operations
// that the run gives it before and the entries of the scan. The expected
// replacement follows the rules of a replacement, not the code, and must not
// depend on the order of the input.
func TestReplaceOps(t *testing.T) {
	t.Parallel()
	const (
		personal, shared     = domain.LibraryPersonal, domain.LibraryShared
		original, live, raw  = domain.RoleOriginal, domain.RoleLiveVideo, domain.RoleRAWPair
		holidaysDir, tripDir = "2024/05-01 - Holidays", "2024/05-02 - Trip"

		holidays, trip, binned domain.AlbumID = 10, 11, 12
	)
	type (
		decided   = map[string][]domain.LinkDecision
		followers = map[domain.LinkID][]domain.LinkDecision
		links     = []domain.Link
		files     = map[domain.LinkID]domain.RemoteEntry
		ops       = []domain.Op
		notices   = []domain.Notice
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
	asset := func(in ...domain.AlbumID) domain.Asset {
		a := domain.Asset{
			ID: 7, Key: domain.AssetKey{CloudAssetGUID: "guid-7", UUID: "uuid-7"}, Library: personal, TargetLibrary: personal,
			OriginalName: "IMG_0001.HEIC",
		}
		for _, id := range in {
			a.Memberships = append(a.Memberships, domain.Membership{AlbumID: id, FirstSeenRun: "run-1"})
		}
		return a
	}
	decide := func(action domain.LinkAction, rule string, l domain.Link) domain.LinkDecision {
		return domain.LinkDecision{Link: l, Action: action, Rule: rule}
	}
	at := func(l domain.Link, p string, album domain.AlbumID) domain.Link {
		l.Path, l.AlbumID = p, album
		return l
	}
	moving := func(l, to domain.Link, rule string) domain.Op {
		return domain.Op{Kind: domain.OpMove, Remote: l.Remote, Root: l.Root, Src: l.Path, DstRoot: to.Root, Dst: to.Path, Asset: 7, Rule: rule, Links: links{to}}
	}
	renewed := func(l domain.Link, e domain.RemoteEntry) domain.Link {
		l.Level, l.Size, l.SHA256, l.MD5, l.ETag, l.MTime = domain.LevelImported, e.Size, e.SHA256, e.MD5, e.ETag, e.MTime
		return l
	}
	replacing := func(rule string, ls ...domain.Link) domain.Op {
		return domain.Op{Kind: domain.OpImport, Remote: ls[0].Remote, Root: ls[0].Root, Src: ls[0].Path, Asset: 7, Rule: rule, Links: ls}
	}
	joining := func(album domain.AlbumID, rule string) domain.Op {
		return domain.Op{Kind: domain.OpAlbumAdd, Asset: 7, AlbumID: album, Rule: rule}
	}
	gone := domain.Op{Kind: domain.OpAssetDelete, Asset: 7, Rule: "R-STATUS-08"}
	told := func(code string) notices {
		return notices{{Code: code, Params: []string{"IMG_0001.HEIC", "uuid-7"}}}
	}

	base1, new1 := entry(personal, holidaysDir+"/IMG_0001.HEIC", 100, "a1"), entry(personal, holidaysDir+"/IMG_0001.HEIC", 120, "b1")
	dng1, mov1 := entry(personal, holidaysDir+"/IMG_0001.DNG", 300, "r1"), entry(personal, holidaysDir+"/IMG_0001.MOV", 30, "v1")
	newMov1, mov2 := entry(personal, holidaysDir+"/IMG_0001.MOV", 35, "v2"), entry(personal, holidaysDir+"/IMG_0002.MOV", 30, "v3")
	copy1 := entry(personal, tripDir+"/IMG_0001.HEIC", 100, "a1")
	copied1, raw1, photo1 := link(1, "alpha", copy1, original, trip), link(2, "alpha", dng1, raw, holidays), link(3, "alpha", base1, original, holidays)
	live1, live2 := link(4, "alpha", mov1, live, holidays), link(5, "alpha", mov2, live, holidays)
	base3, new3 := entry(shared, holidaysDir+"/IMG_0003.HEIC", 100, "c3"), entry(shared, holidaysDir+"/IMG_0003.HEIC", 130, "d3")
	mov3 := entry(shared, holidaysDir+"/IMG_0003.MOV", 30, "v4")
	photo6, live7 := link(6, "beta", base3, original, holidays), link(7, "beta", mov3, live, holidays)
	photo6Trip, live7Trip := at(photo6, tripDir+"/IMG_0003.HEIC", trip), at(live7, tripDir+"/IMG_0003.mov", trip)
	photo8, new8 := link(8, "beta", base1, original, holidays), entry(personal, holidaysDir+"/IMG_0001.HEIC", 150, "b8")
	parked, hidden := photo1, asset(holidays)
	parked.Status, parked.QuarantinePath, hidden.Hidden = "", ".photosync/quarantine/2026-11-13/"+photo1.Path, true
	trashed, lost1, lostLive1 := asset(holidays), renewed(photo1, new1), renewed(live1, newMov1)
	trashed.InTrash, lost1.AssetID, lostLive1.AssetID = true, 0, 0

	for _, c := range []struct {
		name       string
		target     domain.Asset
		decisions  decided
		components followers
		links      links
		ops        ops
		files      files
		want       ops
		notices    notices
	}{
		{"nothing at all", asset(holidays), nil, nil, nil, nil, nil, nil, nil},
		{"a photo that the user changed in place replaces its asset with one imported from it with its RAW file and its changed Live Photo video, into the albums of the asset outside Recently Deleted that the run does not join already, and the old asset goes to Recently Deleted",
			asset(trip, holidays, binned),
			decided{"alpha": {decide(domain.ActionReplaceAsset, "R-LINK-06", photo1), decide(domain.ActionNone, "", copied1)}},
			followers{3: {decide(domain.ActionReplaceAsset, "R-COMPONENT-03", live1)}},
			links{live1, copied1, photo1, live2, raw1}, ops{joining(trip, "R-LINK-05")}, files{1: copy1, 2: dng1, 3: new1, 4: newMov1, 5: mov2},
			ops{replacing("R-LINK-06", renewed(photo1, new1), renewed(raw1, dng1), renewed(live1, newMov1)), joining(holidays, "R-LINK-06"), gone}, nil}, // covers: R-LINK-06
		{"a photo that the user changed while its asset moved to another event is imported with its Live Photo video from where the run moves them",
			asset(trip),
			decided{"beta": {decide(domain.ActionMoveFileReplaceAsset, "R-LINK-14", photo6)}},
			followers{6: {decide(domain.ActionMoveFile, "R-COMPONENT-04", live7)}},
			links{live7, photo6}, ops{moving(photo6, photo6Trip, "R-OBJECT-05"), moving(live7, live7Trip, "R-COMPONENT-04")}, files{6: new3, 7: mov3},
			ops{replacing("R-LINK-14", renewed(photo6Trip, new3), renewed(live7Trip, mov3)), joining(trip, "R-LINK-14"), gone}, nil}, // covers: R-LINK-14
		{"a changed Live Photo video replaces the asset with its group, which beats a changed photo of a later link in another remote",
			asset(trip, holidays),
			decided{"alpha": {decide(domain.ActionNone, "", photo1)}, "beta": {decide(domain.ActionReplaceAsset, "R-LINK-06", photo8)}},
			followers{3: {decide(domain.ActionReplaceAsset, "R-COMPONENT-03", live1)}},
			links{photo8, live1, photo1}, nil, files{3: base1, 4: newMov1, 8: new8},
			ops{
				replacing("R-COMPONENT-03", renewed(photo1, base1), renewed(live1, newMov1)),
				joining(holidays, "R-COMPONENT-03"), joining(trip, "R-COMPONENT-03"), gone,
			}, nil}, // covers: R-COMPONENT-03
		{"a hidden asset, which a sync cannot find in Photos, is not replaced, with a notice",
			hidden, decided{"alpha": {decide(domain.ActionReplaceAsset, "R-LINK-06", photo1)}}, nil, links{photo1}, nil, files{3: new1},
			nil, told("plan.hidden_not_replaced")},
		{"a changed file that the run takes to quarantine as a needless copy replaces nothing, with a notice",
			asset(trip), decided{"alpha": {decide(domain.ActionMoveFileReplaceAsset, "R-LINK-14", photo1)}}, nil, links{photo1},
			ops{{Kind: domain.OpQuarantine, Remote: "alpha", Root: personal, Src: photo1.Path, DstRoot: personal, Dst: parked.QuarantinePath, Asset: 7, Rule: "R-OBJECT-04", Links: links{parked}}},
			files{3: new1}, nil, told("plan.quarantined_not_replaced")},
		{"nothing is replaced while a decision deletes the asset",
			asset(holidays),
			decided{"alpha": {decide(domain.ActionDeleteAsset, "R-STATUS-02", photo1)}, "beta": {decide(domain.ActionReplaceAsset, "R-LINK-06", photo8)}},
			nil, links{photo8}, nil, files{8: new8}, nil, nil},
		{"nothing is replaced while a decision imports a file of the asset as a new asset, though its Live Photo video has new content too",
			trashed,
			decided{"alpha": {decide(domain.ActionImport, "R-LINK-11", photo1)}},
			followers{3: {decide(domain.ActionReplaceAsset, "R-COMPONENT-03", live1)}},
			links{live1, photo1},
			ops{{Kind: domain.OpImport, Remote: "alpha", Root: personal, Src: photo1.Path, AlbumID: holidays, Rule: "R-LINK-11", Links: links{lost1, lostLive1}}},
			files{3: new1, 4: newMov1}, nil, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			show := func() string {
				return fmt.Sprintf("%+v %+v %+v %+v %+v %+v %+v", c.target, c.decisions, c.components, c.links, c.ops, c.files, albums)
			}
			input := show()
			got, said := plan.ReplaceOps(c.target, c.decisions, c.components, c.links, c.ops, c.files, albums)
			if !reflect.DeepEqual(got, c.want) || !reflect.DeepEqual(said, c.notices) {
				t.Errorf("ReplaceOps =\n%+v\n%+v\nwant\n%+v\n%+v", got, said, c.want, c.notices)
			}
			for _, o := range c.want {
				if _, ok := domain.Rules[o.Rule]; !ok {
					t.Errorf("%s is not in domain.Rules", o.Rule)
				}
			}
			target, decisions, components := c.target, decided{}, followers{}
			target.Memberships = slices.Clone(c.target.Memberships)
			slices.Reverse(target.Memberships)
			for name, ds := range c.decisions {
				decisions[name] = slices.Clone(ds)
				slices.Reverse(decisions[name])
			}
			for id, ds := range c.components {
				components[id] = slices.Clone(ds)
				slices.Reverse(components[id])
			}
			ls, ps, as := slices.Clone(c.links), slices.Clone(c.ops), slices.Clone(albums)
			slices.Reverse(ls)
			slices.Reverse(ps)
			slices.Reverse(as)
			again, more := plan.ReplaceOps(target, decisions, components, ls, ps, c.files, as)
			if !reflect.DeepEqual(again, got) || !reflect.DeepEqual(more, said) {
				t.Errorf("with the input reversed:\n%+v\n%+v\nwant\n%+v\n%+v", again, more, got, said)
			}
			for _, o := range got {
				for i := range o.Links {
					o.Links[i].Path = "changed"
				}
			}
			if after := show(); after != input {
				t.Errorf("ReplaceOps changed its input or shares a slice with it:\n%s\nwant\n%s", after, input)
			}
		})
	}
}
