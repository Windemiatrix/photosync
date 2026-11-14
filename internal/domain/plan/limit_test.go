package plan_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Each row but the first takes a plan of five assets, its operations in the
// order that a run takes them: asset 7 in three phases, in two remotes and in
// iCloud; asset 5, whose first operation comes after that of asset 7; two
// imports of new assets; and asset 3, whose only operation comes in phase 5.
// Operations of no asset lie between them, and asset 13 has a link and a
// conflict but no operation. The expected plans follow the rule of the limit,
// not the code.
func TestLimit(t *testing.T) {
	t.Parallel()
	const personal, holidays = domain.LibraryPersonal, "2024/05-01 - Holidays/"
	type (
		ops       = []domain.Op
		links     = []domain.Link
		conflicts = []domain.Conflict
	)
	link := func(id domain.LinkID, asset domain.AssetID, role domain.LinkRole, at string) domain.Link {
		return domain.Link{ID: id, AssetID: asset, Remote: "alpha", Root: personal, Path: at, Role: role, Size: 10, MD5: "m1", Status: domain.StatusActive}
	}
	create := domain.Op{Kind: domain.OpAlbumCreate, AlbumID: 31, Rule: "R-LAYOUT-06"}
	move7 := domain.Op{
		Kind: domain.OpMove, Remote: "alpha", Root: personal, Src: "2024/IMG_0007.HEIC", DstRoot: personal, Dst: holidays + "IMG_0007.HEIC",
		Asset: 7, Rule: "R-OBJECT-05", Links: links{link(71, 7, domain.RoleOriginal, holidays+"IMG_0007.HEIC")},
	}
	edited := domain.Op{
		Kind: domain.OpQuarantine, Remote: "alpha", Root: personal, Src: "Edited/IMG_0100.HEIC", DstRoot: personal,
		Dst: ".photosync/quarantine/2026-11-13/Edited/IMG_0100.HEIC", Rule: "R-STATUS-10",
	}
	put5 := domain.Op{Kind: domain.OpPut, Remote: "alpha", DstRoot: personal, Dst: holidays + "IMG_0005.HEIC", Asset: 5, Rule: "R-LINK-01"}
	put7 := domain.Op{Kind: domain.OpPut, Remote: "beta", DstRoot: personal, Dst: holidays + "IMG_0007.HEIC", Asset: 7, Rule: "R-LINK-01"}
	import1 := domain.Op{
		Kind: domain.OpImport, Remote: "alpha", Root: personal, Src: "Inbox/IMG_0201.HEIC", Rule: "R-LINK-02",
		Links: links{link(0, 0, domain.RoleOriginal, "Inbox/IMG_0201.HEIC"), link(0, 0, domain.RoleLiveVideo, "Inbox/IMG_0201.mov")},
	}
	import2 := domain.Op{Kind: domain.OpImport, Remote: "alpha", Root: personal, Src: "Inbox/IMG_0202.HEIC", Rule: "R-LINK-02"}
	join7 := domain.Op{Kind: domain.OpAlbumAdd, Asset: 7, AlbumID: 31, Rule: "R-LINK-05"}
	delete3 := domain.Op{Kind: domain.OpAssetDelete, Asset: 3, Rule: "R-LINK-09"}
	expire := domain.Op{
		Kind: domain.OpExpire, Remote: "alpha", Root: personal, Src: ".photosync/quarantine/2026-08-01/IMG_0009.HEIC",
		Rule: "R-STATUS-09", Irreversible: true,
	}
	link51, link52 := link(51, 5, domain.RoleOriginal, "2024/IMG_0005.HEIC"), link(52, 5, domain.RoleLiveVideo, "2024/IMG_0005.mov")
	link72, link131 := link(72, 7, domain.RoleOriginal, "Trips/IMG_0007.HEIC"), link(131, 13, domain.RoleOriginal, "2024/IMG_0013.HEIC")
	settled5 := domain.Conflict{AssetID: 5, Remote: "alpha", Status: domain.ConflictStatusResolved, Resolution: domain.ConflictRemoteWins}
	open5 := domain.Conflict{AssetID: 5, Remote: "beta", Status: domain.ConflictStatusOpen}
	skipped13 := domain.Conflict{AssetID: 13, Remote: "alpha", Status: domain.ConflictStatusSkipped}
	albums := []domain.Album{{ID: 31, Title: "05/01 Holidays"}}
	pairs := []domain.AlbumPair{{AlbumID: 31, Remote: "alpha", Root: personal, DirPath: "2024/05-01 - Holidays", CreatedBy: domain.OriginRemote}}
	notices := []domain.Notice{{Code: "plan.settling", Params: []string{"alpha", "personal", "Inbox/IMG_0203.HEIC"}}}
	// A plan of its own for each row, so that a change to the input shows in
	// the row that made it.
	whole := func() domain.Plan {
		return domain.Plan{
			Ops:       ops{create, move7, edited, put5, put7, import1, import2, join7, delete3, expire},
			Albums:    albums,
			Pairs:     pairs,
			Links:     links{link51, link52, link72, link131},
			Conflicts: conflicts{settled5, open5, skipped13},
			Notices:   notices,
		}
	}
	for _, c := range []struct {
		name string
		p    domain.Plan
		n    int
		want domain.Plan
	}{
		{"an empty plan", domain.Plan{}, 1, domain.Plan{}},
		{"no limit", whole(), 0, whole()},
		{"a limit below zero is no limit", whole(), -1, whole()},
		{"the first asset keeps its operations in every phase and remote, the others leave with their links and conflicts, and the operations of no asset stay",
			whole(), 1, domain.Plan{
				Ops: ops{create, move7, edited, put7, join7, expire}, Albums: albums, Pairs: pairs,
				Links: links{link72, link131}, Conflicts: conflicts{skipped13}, Notices: notices,
			}},
		{"an import is an asset of its own",
			whole(), 3, domain.Plan{
				Ops: ops{create, move7, edited, put5, put7, import1, join7, expire}, Albums: albums, Pairs: pairs,
				Links: links{link51, link52, link72, link131}, Conflicts: conflicts{settled5, open5, skipped13}, Notices: notices,
			}},
		{"a limit of as many assets as the plan has", whole(), 5, whole()},
		{"a limit above the number of assets", whole(), 6, whole()},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			input := fmt.Sprintf("%+v", c.p)
			if got := plan.Limit(c.p, c.n); !reflect.DeepEqual(got, c.want) {
				t.Errorf("Limit(%d) =\n%+v\nwant\n%+v", c.n, got, c.want)
			}
			if after := fmt.Sprintf("%+v", c.p); after != input {
				t.Errorf("Limit changed its input:\n%s\nwant\n%s", after, input)
			}
		})
	}
}
