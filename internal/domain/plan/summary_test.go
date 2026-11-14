package plan_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// The plan changes, in two remotes and on iCloud, assets of the delta,
// assets that the delta lacks and no asset at all, with conflicts and notices
// beside its operations; the delta holds assets whose original is or is not
// anywhere. The numbers follow what each of them means, counted by hand, not
// the code, and must not depend on the order of the input.
func TestSummarize(t *testing.T) {
	t.Parallel()
	const (
		personal, shared = domain.LibraryPersonal, domain.LibraryShared
		holidays, trip   = "2024/05-01 - Holidays/", "2024/05-02 - Trip/"
		quarantined      = ".photosync/quarantine/2026-11-13/"
	)
	type kinds = map[domain.OpKind]plan.Tally
	world := func() (domain.Plan, domain.SnapshotDelta) {
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
					Dst: trip + "IMG_0007.HEIC", Asset: 11, Rule: "R-OBJECT-05", Links: []domain.Link{{Size: 70}},
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
				{Kind: domain.OpPut, Remote: "beta", DstRoot: personal, Dst: trip + "IMG_0006.HEIC", Asset: 6, Rule: "R-LINK-01", Links: []domain.Link{{Size: 100}}},
				{Kind: domain.OpImport, Remote: "alpha", Root: personal, Src: "Inbox/IMG_0011.HEIC", Rule: "R-LINK-02", Links: []domain.Link{{Size: 400}}},
				{
					Kind: domain.OpImport, Remote: "alpha", Root: personal, Src: "Inbox/IMG_0012.HEIC", Rule: "R-LINK-02",
					Links: []domain.Link{{Size: 500}, {Size: 50}},
				},
				{
					Kind: domain.OpImport, Remote: "alpha", Root: personal, Src: holidays + "IMG_0001.HEIC", Asset: 1, Rule: "R-LINK-06",
					Links: []domain.Link{{Size: 300}, {Size: 30}},
				},
				{Kind: domain.OpImport, Remote: "alpha", Root: personal, Src: holidays + "IMG_0008.HEIC", Asset: 8, Rule: "R-LINK-06", Links: []domain.Link{{Size: 80}}},
				{Kind: domain.OpImport, Remote: "beta", Root: personal, Src: holidays + "IMG_0003.HEIC", Asset: 3, Rule: "R-COMPONENT-03", Links: []domain.Link{{Size: 60}}},
				{Kind: domain.OpAlbumAdd, Asset: 1, AlbumID: 10, Rule: "R-LINK-06"},
				{Kind: domain.OpAlbumAdd, Asset: 6, AlbumID: 23, Rule: "R-LINK-05"},
				{Kind: domain.OpAlbumRemove, Asset: 7, AlbumID: 10, Rule: "R-LAYOUT-04"},
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
		return p, d
	}
	reversed := func() (domain.Plan, domain.SnapshotDelta) {
		p, d := world()
		slices.Reverse(p.Ops)
		slices.Reverse(p.Conflicts)
		slices.Reverse(p.Notices)
		slices.Reverse(d.Assets)
		return p, d
	}
	every := plan.Summary{
		ICloud: kinds{
			domain.OpAlbumCreate: {Count: 1}, domain.OpImport: {Count: 2, Bytes: 950}, domain.OpAlbumAdd: {Count: 2},
			domain.OpAlbumRemove: {Count: 1}, domain.OpAssetDelete: {Count: 5},
		},
		Replaced: plan.Tally{Count: 3, Bytes: 470},
		Remotes: map[string]kinds{
			"alpha": {
				domain.OpDelete: {Count: 1}, domain.OpMove: {Count: 1}, domain.OpQuarantine: {Count: 2}, domain.OpPut: {Count: 2, Bytes: 130},
				domain.OpCopy: {Count: 1},
			},
			"beta": {domain.OpDelete: {Count: 2}, domain.OpPut: {Count: 1, Bytes: 100}, domain.OpExpire: {Count: 1}},
		},
		Assets: 11, Irreversible: 3, NoUndo: 8, Mass: 7, Shared: 1, Captions: 1, Unavailable: 2, Unsettled: 4,
		Rules: map[string]int{
			"R-COMPONENT-03": 1, "R-LAYOUT-04": 1, "R-LAYOUT-06": 1, "R-LINK-01": 3, "R-LINK-02": 2, "R-LINK-05": 1,
			"R-LINK-06": 3, "R-OBJECT-03": 1, "R-OBJECT-04": 1, "R-OBJECT-05": 1, "R-STATUS-06": 4, "R-STATUS-08": 4, "R-STATUS-09": 1,
			"R-STATUS-10": 1,
		},
		Notices: map[string]int{"plan.doubles": 1, "plan.settling": 2},
	}
	for _, c := range []struct {
		name  string
		input func() (domain.Plan, domain.SnapshotDelta)
		want  plan.Summary
	}{
		{"a plan of every kind of operation", world, every},
		{"the plan and the delta in reverse order", reversed, every},
		{
			"an empty plan", func() (domain.Plan, domain.SnapshotDelta) { return domain.Plan{}, domain.SnapshotDelta{} },
			plan.Summary{ICloud: kinds{}, Remotes: map[string]kinds{}, Rules: map[string]int{}, Notices: map[string]int{}},
		},
	} {
		p, d := c.input()
		if got := plan.Summarize(p, d); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: Summarize = %#v, want %#v", c.name, got, c.want)
		}
		if wantP, wantD := c.input(); !reflect.DeepEqual(p, wantP) || !reflect.DeepEqual(d, wantD) {
			t.Errorf("%s: Summarize changed its input:\n%+v %+v\nwant\n%+v %+v", c.name, p, d, wantP, wantD)
		}
	}
}
