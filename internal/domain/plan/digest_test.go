package plan_test

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// The digest of a small plan is the SHA-256 of its JSON as written out here.
// Each row then changes a plan of eleven operations on three assets, two albums
// of the snapshot with one title and an album that the run adds, in one way:
// a run must take the plan for the same when only the IDs that it gives
// assets, albums and links, a cloud GUID, the times of files, the order of the
// input or what the plan keeps beside its operations change, and for another
// when an operation does something else. The expectations follow those rules,
// not the code.
//
// covers: INV-17
func TestDigest(t *testing.T) {
	t.Parallel()
	const (
		personal, shared = domain.LibraryPersonal, domain.LibraryShared
		holidays, trip   = "2024/05-01 - Holidays/", "2024/05-02 - Trip/"
		quarantined      = ".photosync/quarantine/2026-11-13/"
	)
	settled := time.Date(2026, 11, 13, 11, 0, 0, 0, time.UTC)
	digest := func(name string, p domain.Plan, d domain.SnapshotDelta) string {
		t.Helper()
		input := fmt.Sprintf("%+v %+v", p, d)
		got, err := plan.Digest(p, d)
		if err != nil {
			t.Errorf("%s: Digest error %v", name, err)
		}
		if after := fmt.Sprintf("%+v %+v", p, d); after != input {
			t.Errorf("%s: Digest changed its input:\n%s\nwant\n%s", name, after, input)
		}
		return got
	}
	asset := func(id domain.AssetID, name string) domain.Asset {
		return domain.Asset{
			ID: id, Key: domain.AssetKey{UUID: fmt.Sprintf("uuid-%d", id)}, Library: personal, TargetLibrary: personal, OriginalName: name,
		}
	}
	holidaysAlbum := func(id domain.AlbumID, uuid string) domain.Album {
		return domain.Album{ID: id, UUID: uuid, Kind: domain.KindEvent, Title: "05/01 Holidays", FolderTitles: []string{"2024"}}
	}

	small := domain.Plan{Ops: []domain.Op{
		{
			Kind: domain.OpPut, Remote: "alpha", DstRoot: personal, Dst: holidays + "IMG_0001.HEIC", Asset: 7, Rule: "R-LINK-01",
			Links: []domain.Link{{
				ID: 3, AssetID: 7, Remote: "alpha", Root: personal, Path: holidays + "IMG_0001.HEIC", Role: domain.RoleOriginal,
				AlbumID: 10, Size: 100, MTime: settled, Level: domain.LevelUploaded, Status: domain.StatusActive,
			}},
		},
		{Kind: domain.OpAlbumAdd, Asset: 7, AlbumID: 10, Rule: "R-LINK-05"},
	}}
	smallDelta := domain.SnapshotDelta{Assets: []domain.Asset{asset(7, "IMG_0001.HEIC")}, Albums: []domain.Album{holidaysAlbum(10, "u-hol")}}
	canonical := `[{"Op":{"Kind":"put","Remote":"alpha","Root":"","Src":"","DstRoot":"personal",` +
		`"Dst":"2024/05-01 - Holidays/IMG_0001.HEIC","Asset":0,"AlbumID":0,"ParentID":0,"Rule":"R-LINK-01","Irreversible":false,` +
		`"Links":[{"ID":0,"AssetID":0,"Remote":"alpha","Root":"personal","Path":"2024/05-01 - Holidays/IMG_0001.HEIC",` +
		`"Role":"original","AlbumID":0,"Size":100,"SHA256":"","MD5":"","ETag":"","MTime":"0001-01-01T00:00:00Z",` +
		`"Level":"uploaded","Status":"active","QuarantinePath":""}]},"Asset":"uuid-7","Album":"","Parent":""},` +
		`{"Op":{"Kind":"album_add","Remote":"","Root":"","Src":"","DstRoot":"","Dst":"","Asset":0,"AlbumID":0,"ParentID":0,` +
		`"Rule":"R-LINK-05","Irreversible":false,"Links":null},"Asset":"uuid-7","Album":"u-hol","Parent":""}]` + "\n"
	if got, want := digest("the small plan", small, smallDelta), fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(canonical))); got != want {
		t.Errorf("the small plan: Digest = %q, want %q, the SHA-256 of\n%s", got, want, canonical)
	}

	// A plan of its own for each row, so that a row changes nothing that
	// another row reads.
	world := func() (domain.Plan, domain.SnapshotDelta) {
		moved := domain.Link{
			ID: 1, AssetID: 7, Remote: "alpha", Root: personal, Path: trip + "IMG_0001.HEIC", Role: domain.RoleOriginal, AlbumID: 23,
			Size: 100, SHA256: "sa1", MD5: "a1", ETag: "ea1", Level: domain.LevelExact, Status: domain.StatusActive,
		}
		parked := domain.Link{
			ID: 2, AssetID: 8, Remote: "alpha", Root: personal, Path: holidays + "IMG_0002.HEIC", Role: domain.RoleOriginal, AlbumID: 10,
			Size: 200, SHA256: "sa2", MD5: "a2", ETag: "ea2", Level: domain.LevelExact, Status: domain.StatusQuarantinedRemote,
			QuarantinePath: quarantined + holidays + "IMG_0002.HEIC",
		}
		uploaded := domain.Link{
			AssetID: 7, Remote: "beta", Root: personal, Path: holidays + "IMG_0001.HEIC", Role: domain.RoleOriginal, AlbumID: 10,
			Size: 100, Level: domain.LevelUploaded, Status: domain.StatusActive,
		}
		imported := func(name string, role domain.LinkRole, size int64) domain.Link {
			return domain.Link{
				Remote: "alpha", Root: personal, Path: "Inbox/" + name, Role: role, Size: size, MD5: "m" + name,
				Level: domain.LevelImported, Status: domain.StatusActive,
			}
		}
		join := func(id domain.AssetID, album domain.AlbumID) domain.Op {
			return domain.Op{Kind: domain.OpAlbumAdd, Asset: id, AlbumID: album, Rule: "R-LINK-05"}
		}
		p := domain.Plan{
			Ops: []domain.Op{
				{Kind: domain.OpAlbumCreate, AlbumID: 23, Rule: "R-LAYOUT-06"},
				{
					Kind: domain.OpMove, Remote: "alpha", Root: personal, Src: holidays + "IMG_0001.HEIC", DstRoot: personal, Dst: moved.Path,
					Asset: 7, Rule: "R-OBJECT-05", Links: []domain.Link{moved},
				},
				{
					Kind: domain.OpQuarantine, Remote: "alpha", Root: personal, Src: parked.Path, DstRoot: personal, Dst: parked.QuarantinePath,
					Asset: 8, Rule: "R-STATUS-01", Links: []domain.Link{parked},
				},
				{Kind: domain.OpPut, Remote: "beta", DstRoot: personal, Dst: uploaded.Path, Asset: 7, Rule: "R-LINK-01", Links: []domain.Link{uploaded}},
				{
					Kind: domain.OpImport, Remote: "alpha", Root: personal, Src: "Inbox/IMG_0003.HEIC", Rule: "R-LINK-02",
					Links: []domain.Link{imported("IMG_0003.HEIC", domain.RoleOriginal, 300), imported("IMG_0003.mov", domain.RoleLiveVideo, 30)},
				},
				join(7, 10), join(7, 13), join(8, 10),
				{Kind: domain.OpAssetDelete, Asset: 9, Rule: "R-LINK-09"},
				{Kind: domain.OpAlbumRename, AlbumID: 13, Dst: "05/01 Vacation", Rule: "R-LAYOUT-12"},
				{Kind: domain.OpAlbumMove, AlbumID: 10, ParentID: 23, Rule: "R-LAYOUT-13"},
			},
			Albums: []domain.Album{{ID: 23, Kind: domain.KindEvent, Title: "05/02 Trip", FolderTitles: []string{"2024"}}},
			Pairs:  []domain.AlbumPair{{AlbumID: 23, Remote: "alpha", Root: personal, DirPath: "2024/05-02 - Trip", CreatedBy: domain.OriginICloud}},
			Links: []domain.Link{{
				ID: 4, AssetID: 9, Remote: "beta", Root: personal, Path: holidays + "IMG_0009.HEIC", Role: domain.RoleOriginal, AlbumID: 10,
				Size: 90, Level: domain.LevelExact, Status: domain.StatusDetached,
			}},
			Conflicts: []domain.Conflict{{AssetID: 8, Remote: "beta", Status: domain.ConflictStatusOpen}},
			Notices:   []domain.Notice{{Code: "plan.settling", Params: []string{"alpha", "personal", "Inbox/IMG_0005.HEIC"}}},
		}
		d := domain.SnapshotDelta{
			Assets: []domain.Asset{asset(7, "IMG_0001.HEIC"), asset(8, "IMG_0002.HEIC"), asset(9, "IMG_0002.HEIC")},
			Albums: []domain.Album{holidaysAlbum(10, "u-hol"), holidaysAlbum(13, "u-hol2")},
		}
		return p, d
	}
	base, baseDelta := world()
	want := digest("the base plan", base, baseDelta)
	seen := map[string]string{want: "the base plan"}
	for _, c := range []struct {
		name   string
		same   bool
		change func(p *domain.Plan, d *domain.SnapshotDelta)
	}{
		{"the same plan", true, func(*domain.Plan, *domain.SnapshotDelta) {}},
		{"other notices, links of the state, conflicts and pairs beside the operations", true, func(p *domain.Plan, _ *domain.SnapshotDelta) {
			p.Notices = append(p.Notices, domain.Notice{Code: "plan.doubles", Params: []string{"alpha", "personal", "Inbox/A.JPG", "Inbox/a.jpg"}})
			p.Links, p.Pairs = nil, nil
			p.Conflicts[0].Status = domain.ConflictStatusSkipped
		}},
		{"other times of the files", true, func(p *domain.Plan, _ *domain.SnapshotDelta) {
			p.Ops[1].Links[0].MTime, p.Ops[4].Links[1].MTime = settled, settled.Add(time.Hour)
		}},
		{"assets that swap their IDs, one of them with a cloud GUID now", true, func(p *domain.Plan, d *domain.SnapshotDelta) {
			swap := func(id domain.AssetID) domain.AssetID {
				switch id {
				case 7:
					return 8
				case 8:
					return 7
				}
				return id
			}
			for i := range p.Ops {
				p.Ops[i].Asset = swap(p.Ops[i].Asset)
				for j := range p.Ops[i].Links {
					p.Ops[i].Links[j].AssetID = swap(p.Ops[i].Links[j].AssetID)
				}
			}
			for i := range d.Assets {
				d.Assets[i].ID = swap(d.Assets[i].ID)
			}
			d.Assets[1].Key.CloudAssetGUID = "guid-8"
		}},
		{"albums that swap their IDs, and the added album with another ID", true, func(p *domain.Plan, d *domain.SnapshotDelta) {
			renumber := func(id domain.AlbumID) domain.AlbumID {
				switch id {
				case 10:
					return 13
				case 13:
					return 10
				case 23:
					return 24
				}
				return id
			}
			for i := range p.Ops {
				p.Ops[i].AlbumID, p.Ops[i].ParentID = renumber(p.Ops[i].AlbumID), renumber(p.Ops[i].ParentID)
				for j := range p.Ops[i].Links {
					p.Ops[i].Links[j].AlbumID = renumber(p.Ops[i].Links[j].AlbumID)
				}
			}
			for i := range d.Albums {
				d.Albums[i].ID = renumber(d.Albums[i].ID)
			}
			p.Albums[0].ID = 24
		}},
		{"links with other IDs", true, func(p *domain.Plan, _ *domain.SnapshotDelta) {
			p.Ops[1].Links[0].ID, p.Ops[2].Links[0].ID = 5, 6
		}},
		{"the operations in reverse order", true, func(p *domain.Plan, _ *domain.SnapshotDelta) {
			slices.Reverse(p.Ops)
		}},
		{"the delta in reverse order, with another capture time and no original for an asset", true, func(_ *domain.Plan, d *domain.SnapshotDelta) {
			no := false
			d.Assets[2].Taken, d.Assets[2].Missing, d.Assets[2].InCloud = settled, true, &no
			slices.Reverse(d.Assets)
			slices.Reverse(d.Albums)
		}},
		{"another kind", false, func(p *domain.Plan, _ *domain.SnapshotDelta) { p.Ops[2].Kind = domain.OpDelete }},
		{"another remote", false, func(p *domain.Plan, _ *domain.SnapshotDelta) { p.Ops[1].Remote = "beta" }},
		{"another root of the source", false, func(p *domain.Plan, _ *domain.SnapshotDelta) { p.Ops[1].Root = shared }},
		{"another source", false, func(p *domain.Plan, _ *domain.SnapshotDelta) { p.Ops[1].Src = holidays + "IMG_0004.HEIC" }},
		{"another root of the destination", false, func(p *domain.Plan, _ *domain.SnapshotDelta) { p.Ops[1].DstRoot = shared }},
		{"another destination", false, func(p *domain.Plan, _ *domain.SnapshotDelta) { p.Ops[1].Dst = trip + "IMG_0004.HEIC" }},
		{"a quarantine on the next day", false, func(p *domain.Plan, _ *domain.SnapshotDelta) {
			p.Ops[2].Dst = ".photosync/quarantine/2026-11-14/" + holidays + "IMG_0002.HEIC"
			p.Ops[2].Links[0].QuarantinePath = p.Ops[2].Dst
		}},
		{"another rule", false, func(p *domain.Plan, _ *domain.SnapshotDelta) { p.Ops[1].Rule = "R-OBJECT-06" }},
		{"an irreversible deletion", false, func(p *domain.Plan, _ *domain.SnapshotDelta) { p.Ops[8].Irreversible = true }},
		{"another asset of the delta", false, func(p *domain.Plan, _ *domain.SnapshotDelta) { p.Ops[7].Asset = 9 }},
		{"another album of the same title", false, func(p *domain.Plan, _ *domain.SnapshotDelta) { p.Ops[7].AlbumID = 13 }},
		{"an asset that the delta lacks", false, func(p *domain.Plan, _ *domain.SnapshotDelta) { p.Ops[8].Asset = 3 }},
		{"another asset that the delta lacks", false, func(p *domain.Plan, _ *domain.SnapshotDelta) { p.Ops[8].Asset = 4 }},
		{"another size of a link", false, func(p *domain.Plan, _ *domain.SnapshotDelta) { p.Ops[1].Links[0].Size = 101 }},
		{"another hash of a link", false, func(p *domain.Plan, _ *domain.SnapshotDelta) { p.Ops[1].Links[0].MD5 = "b1" }},
		{"another status of a link", false, func(p *domain.Plan, _ *domain.SnapshotDelta) { p.Ops[2].Links[0].Status = "" }},
		{"another quarantine path of a link", false, func(p *domain.Plan, _ *domain.SnapshotDelta) {
			p.Ops[2].Links[0].QuarantinePath = quarantined + "IMG_0002.HEIC"
		}},
		{"another path of a link", false, func(p *domain.Plan, _ *domain.SnapshotDelta) { p.Ops[3].Links[0].Path = holidays + "IMG_0001 (1).HEIC" }},
		{"another role of a link", false, func(p *domain.Plan, _ *domain.SnapshotDelta) { p.Ops[4].Links[1].Role = domain.RoleRAWPair }},
		{"another level of a link", false, func(p *domain.Plan, _ *domain.SnapshotDelta) { p.Ops[1].Links[0].Level = domain.LevelSameShot }},
		{"one more expiry", false, func(p *domain.Plan, _ *domain.SnapshotDelta) {
			p.Ops = append(p.Ops, domain.Op{
				Kind: domain.OpExpire, Remote: "alpha", Root: personal, Src: ".photosync/quarantine/2026-08-01/IMG_0009.HEIC",
				Rule: "R-STATUS-09", Irreversible: true,
			})
		}},
		{"another title of the album that the run adds", false, func(p *domain.Plan, _ *domain.SnapshotDelta) { p.Albums[0].Title = "05/03 Trip" }},
		{"another title of a renamed album", false, func(p *domain.Plan, _ *domain.SnapshotDelta) { p.Ops[9].Dst = "05/01 Beach" }},
		{"another folder of a moved album", false, func(p *domain.Plan, _ *domain.SnapshotDelta) { p.Ops[10].ParentID = 13 }},
		{"one operation less", false, func(p *domain.Plan, _ *domain.SnapshotDelta) { p.Ops = slices.Delete(p.Ops, 6, 7) }},
	} {
		p, d := world()
		c.change(&p, &d)
		got := digest(c.name, p, d)
		switch {
		case c.same && got != want:
			t.Errorf("%s: Digest = %q, want %q, that of the base plan", c.name, got, want)
		case c.same:
		case got == want:
			t.Errorf("%s: Digest = %q, the same as that of the base plan", c.name, got)
		case seen[got] != "":
			t.Errorf("%s: Digest = %q, the same as that of %q", c.name, got, seen[got])
		default:
			seen[got] = c.name
		}
	}
}
