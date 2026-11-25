package plan_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// The plan changes, in two remotes and on iCloud, assets of the delta,
// assets that the delta lacks and no asset at all, with links, conflicts and
// notices beside its operations. The delta holds assets whose original is or
// is not anywhere, and the state links that the plan moves, detaches, brings
// back from quarantine or drops, beside assets to which matching gives new
// files, some of them copies, one of them a file that iCloud replaces. The
// numbers follow what each of them means, counted by hand, not the code, and
// must not depend on the order of the input; the samples stop at fifty.
func TestSummarize(t *testing.T) {
	t.Parallel()
	const (
		personal, shared = domain.LibraryPersonal, domain.LibraryShared
		holidays, trip   = "2024/05-01 - Holidays/", "2024/05-02 - Trip/"
		quarantined      = ".photosync/quarantine/2026-11-13/"
		original, live   = domain.RoleOriginal, domain.RoleLiveVideo
		exact, active    = domain.LevelExact, domain.StatusActive
	)
	type (
		kinds   = map[domain.OpKind]plan.Tally
		levels  = map[domain.MatchLevel]int
		samples = map[domain.MatchLevel][]domain.LinkID
	)
	link := func(id domain.LinkID, asset domain.AssetID, remote string, role domain.LinkRole, level domain.MatchLevel, status domain.LinkStatus) domain.Link {
		return domain.Link{ID: id, AssetID: asset, Remote: remote, Role: role, Level: level, Status: status}
	}
	world := func() (domain.Plan, domain.SnapshotDelta, domain.State) {
		yes, no := true, false
		p := domain.Plan{
			Ops: []domain.Op{
				{Kind: domain.OpAlbumCreate, AlbumID: 23, Rule: "R-LAYOUT-06"},
				{
					Kind: domain.OpDelete, Remote: "alpha", Root: personal, Src: holidays + "IMG_0009.HEIC", Asset: 9,
					Rule: "R-STATUS-06", Irreversible: true, Links: []domain.Link{{Size: 90}},
				},
				{
					Kind: domain.OpMove, Remote: "alpha", Root: personal, Src: holidays + "IMG_0007.HEIC", DstRoot: personal,
					Dst: trip + "IMG_0007.HEIC", Asset: 11, Rule: "R-OBJECT-05",
					Links: []domain.Link{{
						ID: 1, AssetID: 11, Remote: "alpha", Root: personal, Path: trip + "IMG_0007.HEIC", Role: original, Size: 70,
						Level: exact, Status: active,
					}},
				},
				{
					Kind: domain.OpQuarantine, Remote: "alpha", Root: personal, Src: trip + "IMG_0007 (1).HEIC", DstRoot: personal,
					Dst: quarantined + trip + "IMG_0007 (1).HEIC", Asset: 7, Rule: "R-OBJECT-04", Links: []domain.Link{{Size: 70}},
				},
				{
					Kind: domain.OpQuarantine, Remote: "alpha", Root: personal, Src: "edited/IMG_0005.HEIC", DstRoot: personal,
					Dst: quarantined + "edited/IMG_0005.HEIC", Rule: "R-STATUS-10",
				},
				{
					Kind: domain.OpQuarantine, Remote: "alpha", Root: personal, Src: trip + "IMG_0012.HEIC", DstRoot: personal,
					Dst: quarantined + trip + "IMG_0012.HEIC", Asset: 12, Rule: "R-OBJECT-04",
					Links: []domain.Link{link(103, 12, "alpha", original, exact, "")},
				},
				{
					Kind: domain.OpQuarantine, Remote: "alpha", Root: personal, Src: trip + "IMG_0022.HEIC", DstRoot: personal,
					Dst: quarantined + trip + "IMG_0022.HEIC", Asset: 22, Rule: "R-OBJECT-10",
					Links: []domain.Link{link(112, 22, "alpha", original, domain.LevelSameShotWeak, domain.StatusQuarantinedRemote)},
				},
				{
					Kind: domain.OpDelete, Remote: "beta", Root: shared, Src: holidays + "IMG_0002.HEIC", Asset: 2,
					Rule: "R-STATUS-06", Irreversible: true, Links: []domain.Link{{Size: 200}},
				},
				{Kind: domain.OpDelete, Remote: "beta", Root: shared, Src: holidays + "IMG_0002.xmp", Asset: 2, Rule: "R-STATUS-06"},
				{
					Kind: domain.OpCopy, Remote: "alpha", Root: personal, Src: trip + "IMG_0006.HEIC", DstRoot: personal,
					Dst: holidays + "IMG_0006.HEIC", Asset: 6, Rule: "R-OBJECT-03", Links: []domain.Link{{Size: 100}},
				},
				{Kind: domain.OpPut, Remote: "alpha", DstRoot: personal, Dst: trip + "IMG_0006.HEIC", Asset: 6, Rule: "R-LINK-01", Links: []domain.Link{{Size: 100}}},
				{Kind: domain.OpPut, Remote: "alpha", DstRoot: personal, Dst: trip + "IMG_0006.mov", Asset: 6, Rule: "R-LINK-01", Links: []domain.Link{{Size: 30}}},
				{
					Kind: domain.OpPut, Remote: "alpha", DstRoot: personal, Dst: trip + "IMG_0022 (1).HEIC", Asset: 22, Rule: "R-OBJECT-10",
					Links: []domain.Link{{ID: 112, AssetID: 22, Remote: "alpha", Role: original, Size: 20, Level: domain.LevelUploaded, Status: active}},
				},
				{Kind: domain.OpPut, Remote: "beta", DstRoot: personal, Dst: trip + "IMG_0006.HEIC", Asset: 6, Rule: "R-LINK-01", Links: []domain.Link{{Size: 100}}},
				{
					Kind: domain.OpImport, Remote: "alpha", Root: personal, Src: "Inbox/IMG_0011.HEIC", Rule: "R-LINK-02",
					Links: []domain.Link{{Remote: "alpha", Size: 400, Level: domain.LevelImported}, {Remote: "beta", Size: 999, Level: domain.LevelImported}},
				},
				{
					Kind: domain.OpImport, Remote: "alpha", Root: personal, Src: "Inbox/IMG_0012.HEIC", Rule: "R-LINK-02",
					Links: []domain.Link{{Remote: "alpha", Size: 500}, {Remote: "alpha", Size: 50}},
				},
				{
					Kind: domain.OpImport, Remote: "alpha", Root: personal, Src: holidays + "IMG_0001.HEIC", Asset: 1, Rule: "R-LINK-06",
					Links: []domain.Link{{Remote: "alpha", Size: 300}, {Remote: "alpha", Size: 30}},
				},
				{Kind: domain.OpImport, Remote: "alpha", Root: personal, Src: holidays + "IMG_0008.HEIC", Asset: 8, Rule: "R-LINK-06", Links: []domain.Link{{Remote: "alpha", Size: 80}}},
				{Kind: domain.OpImport, Remote: "beta", Root: personal, Src: holidays + "IMG_0003.HEIC", Asset: 3, Rule: "R-COMPONENT-03", Links: []domain.Link{{Remote: "beta", Size: 60}}},
				{Kind: domain.OpAlbumAdd, Asset: 1, AlbumID: 10, Rule: "R-LINK-06"},
				{Kind: domain.OpAlbumAdd, Asset: 6, AlbumID: 23, Rule: "R-LINK-05"},
				{
					Kind: domain.OpAlbumAdd, Asset: 16, AlbumID: 23, Rule: "R-LINK-03",
					Links: []domain.Link{link(108, 16, "alpha", original, domain.LevelSameTimeSize, active), link(111, 16, "beta", original, domain.LevelProbable, active)},
				},
				{
					Kind: domain.OpAlbumRemove, Asset: 7, AlbumID: 10, Rule: "R-LAYOUT-04",
					Links: []domain.Link{link(2, 7, "beta", original, exact, domain.StatusDetached)},
				},
				{Kind: domain.OpAssetDelete, Asset: 1, Rule: "R-STATUS-08"},
				{Kind: domain.OpAssetDelete, Asset: 2, Rule: "R-STATUS-06", Irreversible: true},
				{Kind: domain.OpAssetDelete, Asset: 3, Rule: "R-STATUS-08"},
				{Kind: domain.OpAssetDelete, Asset: 8, Rule: "R-STATUS-08"},
				{Kind: domain.OpAssetDelete, Asset: 10, Rule: "R-STATUS-08"},
				{
					Kind: domain.OpExpire, Remote: "beta", Root: personal, Src: ".photosync/quarantine/2026-08-01/IMG_0004.HEIC",
					Rule: "R-STATUS-09", Irreversible: true,
				},
			},
			Links: []domain.Link{
				link(3, 21, "beta", original, domain.LevelUploaded, domain.StatusDetached),
				link(4, 19, "alpha", original, exact, active),
				link(5, 20, "beta", original, domain.LevelImportedState, active),
				link(6, 10, "beta", original, exact, ""),
				link(101, 12, "alpha", original, domain.LevelSameShot, active),
				link(102, 12, "alpha", live, domain.LevelPairComponent, active),
				link(104, 13, "alpha", domain.RoleSource, exact, active),
				link(105, 13, "alpha", live, domain.LevelPairComponent, active),
				link(106, 14, "alpha", original, exact, active),
				link(107, 15, "alpha", original, domain.LevelSameShotWeak, active),
				link(109, 18, "beta", original, domain.LevelProbable, active),
				link(110, 17, "alpha", original, domain.LevelSameTimeSize, active),
			},
			Conflicts: []domain.Conflict{
				{AssetID: 1, Remote: "beta", Status: domain.ConflictStatusOpen},
				{AssetID: 3, Remote: "alpha", Status: domain.ConflictStatusOpen},
				{AssetID: 6, Remote: "alpha", Status: domain.ConflictStatusResolved},
				{AssetID: 7, Remote: "beta", Status: domain.ConflictStatusSkipped},
				{AssetID: 8, Remote: "beta", Status: domain.ConflictStatusSkipped},
			},
			Notices: []domain.Notice{
				{Code: "plan.doubles", Params: []string{"alpha", "personal", "Inbox/A.JPG", "Inbox/a.jpg"}},
				{Code: "plan.settling", Params: []string{"alpha", "personal", "Inbox/IMG_0013.HEIC"}},
				{Code: "plan.settling", Params: []string{"beta", "personal", "Inbox/IMG_0014.HEIC"}},
			},
		}
		d := domain.SnapshotDelta{Assets: []domain.Asset{
			{ID: 1, Library: personal, TargetLibrary: personal, HasCaptionOrKeywords: true},
			{ID: 2, Library: shared, TargetLibrary: shared},
			{ID: 3, Library: personal, TargetLibrary: shared},
			{ID: 4, Library: personal, TargetLibrary: personal, Missing: true},
			{ID: 5, Library: personal, TargetLibrary: personal, Missing: true, InCloud: &no},
			{ID: 6, Library: personal, TargetLibrary: personal, Missing: true, InCloud: &yes},
			{ID: 7, Library: personal, TargetLibrary: personal, Missing: true, InCloud: &yes},
			{ID: 8, Library: personal, TargetLibrary: personal},
		}}
		st := domain.State{Links: []domain.Link{
			link(1, 11, "alpha", original, exact, active),
			link(2, 7, "beta", original, exact, active),
			link(3, 21, "beta", original, domain.LevelUploaded, active),
			link(4, 19, "alpha", original, exact, domain.StatusQuarantinedRemote),
			link(5, 20, "beta", original, domain.LevelImportedState, domain.StatusQuarantinedICloud),
			link(6, 10, "beta", original, exact, domain.StatusQuarantinedICloud),
			link(7, 13, "alpha", domain.RoleSource, exact, active),
			link(8, 14, "beta", original, exact, active),
			link(9, 15, "alpha", live, domain.LevelPairComponent, active),
		}}
		return p, d, st
	}
	reversed := func() (domain.Plan, domain.SnapshotDelta, domain.State) {
		p, d, st := world()
		slices.Reverse(p.Ops)
		slices.Reverse(p.Links)
		slices.Reverse(p.Conflicts)
		slices.Reverse(p.Notices)
		slices.Reverse(d.Assets)
		slices.Reverse(st.Links)
		return p, d, st
	}
	every := plan.Summary{
		ICloud: kinds{
			domain.OpAlbumCreate: {Count: 1}, domain.OpImport: {Count: 2, Bytes: 950}, domain.OpAlbumAdd: {Count: 3},
			domain.OpAlbumRemove: {Count: 1}, domain.OpAssetDelete: {Count: 5},
		},
		Replaced: plan.Tally{Count: 3, Bytes: 470},
		Remotes: map[string]kinds{
			"alpha": {
				domain.OpDelete: {Count: 1}, domain.OpMove: {Count: 1}, domain.OpQuarantine: {Count: 4}, domain.OpPut: {Count: 3, Bytes: 150},
				domain.OpCopy: {Count: 1},
			},
			"beta": {domain.OpDelete: {Count: 2}, domain.OpPut: {Count: 1, Bytes: 100}, domain.OpExpire: {Count: 1}},
		},
		Assets: 14, Deletes: 3, NoUndo: 8, Mass: 9, Shared: 1, Captions: 1, Unavailable: 2, Unsettled: 4,
		Copies: 2, Detached: 2, Returned: 2, Closed: 1,
		Rules: map[string]int{
			"R-COMPONENT-03": 1, "R-LAYOUT-04": 1, "R-LAYOUT-06": 1, "R-LINK-01": 3, "R-LINK-02": 2, "R-LINK-03": 1, "R-LINK-05": 1,
			"R-LINK-06": 3, "R-OBJECT-03": 1, "R-OBJECT-04": 2, "R-OBJECT-05": 1, "R-OBJECT-10": 2, "R-STATUS-06": 4, "R-STATUS-08": 4,
			"R-STATUS-09": 1, "R-STATUS-10": 1,
		},
		Notices: map[string]int{"plan.doubles": 1, "plan.settling": 2},
		Levels: levels{
			domain.LevelExact: 3, domain.LevelSameShot: 1, domain.LevelSameShotWeak: 2, domain.LevelSameTimeSize: 2, domain.LevelProbable: 2,
			domain.LevelPairComponent: 2,
		},
		Samples: samples{domain.LevelSameTimeSize: {108, 110}, domain.LevelProbable: {109, 111}},
	}
	fresh := func(level domain.MatchLevel, first, n domain.LinkID) []domain.Link {
		var links []domain.Link
		for id := first; id < first+n; id++ {
			links = append(links, link(id, domain.AssetID(id), "alpha", original, level, active))
		}
		return links
	}
	ids := func(first, n domain.LinkID) []domain.LinkID {
		var out []domain.LinkID
		for id := first; id < first+n; id++ {
			out = append(out, id)
		}
		return out
	}
	for _, c := range []struct {
		name  string
		input func() (domain.Plan, domain.SnapshotDelta, domain.State)
		want  plan.Summary
	}{
		{"a plan of every kind of operation", world, every},
		{"the plan, the delta and the state in reverse order", reversed, every},
		{
			"an empty plan", func() (domain.Plan, domain.SnapshotDelta, domain.State) {
				return domain.Plan{}, domain.SnapshotDelta{}, domain.State{}
			},
			plan.Summary{
				ICloud: kinds{}, Remotes: map[string]kinds{}, Rules: map[string]int{}, Notices: map[string]int{},
				Levels: levels{}, Samples: samples{},
			},
		},
		{
			"fifty-one new links of a level to sample and fifty of the other", func() (domain.Plan, domain.SnapshotDelta, domain.State) {
				return domain.Plan{Links: slices.Concat(fresh(domain.LevelSameTimeSize, 101, 51), fresh(domain.LevelProbable, 201, 50))}, domain.SnapshotDelta{}, domain.State{}
			},
			plan.Summary{
				ICloud: kinds{}, Remotes: map[string]kinds{}, Rules: map[string]int{}, Notices: map[string]int{},
				Levels: levels{domain.LevelSameTimeSize: 51, domain.LevelProbable: 50}, Samples: samples{domain.LevelSameTimeSize: ids(101, 50), domain.LevelProbable: ids(201, 50)},
			},
		},
	} {
		p, d, st := c.input()
		if got := plan.Summarize(p, d, st); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: Summarize = %#v, want %#v", c.name, got, c.want)
		}
		if wantP, wantD, wantS := c.input(); !reflect.DeepEqual(p, wantP) || !reflect.DeepEqual(d, wantD) || !reflect.DeepEqual(st, wantS) {
			t.Errorf("%s: Summarize changed its input:\n%+v %+v %+v\nwant\n%+v %+v %+v", c.name, p, d, st, wantP, wantD, wantS)
		}
	}
}
