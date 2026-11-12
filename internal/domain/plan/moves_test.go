package plan_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Each row is what the user did in the cloud since the last applied run, as
// the links whose file is gone from its path and the files of the scan that
// no link names. The expected moves are written from what the user did, not
// from the code, and must not depend on the order of the input.
func TestMovesByContent(t *testing.T) {
	t.Parallel()
	const (
		byMD5, bySHA256, noHash = domain.HashMD5, domain.HashSHA256, domain.HashNone

		a, b, c domain.AssetID = 7, 8, 9

		holidays  = "2024/05-01 - Holidays/IMG_0001.HEIC"
		holidays1 = "2024/05-01 - Holidays/IMG_0001 (1).HEIC"
		party     = "2024/06-01 - Party/IMG_0001.HEIC"
		beach     = "2024/05-01 - Holidays/Beach.HEIC"
	)
	type content struct {
		size        int64
		md5, sha256 string
	}
	x, y, video := content{100, "a1", "b1"}, content{200, "a2", "b2"}, content{300, "a3", "b3"}
	link := func(id domain.LinkID, asset domain.AssetID, p string, ct content) domain.Link {
		return domain.Link{
			ID: id, AssetID: asset, Remote: "disk", Root: domain.LibraryPersonal, Path: p, Role: domain.RoleOriginal,
			Size: ct.size, MD5: ct.md5, SHA256: ct.sha256, ETag: ct.md5, Status: domain.StatusActive,
		}
	}
	file := func(p string, ct content) domain.RemoteEntry {
		return domain.RemoteEntry{Root: domain.LibraryPersonal, Path: p, Size: ct.size, MD5: ct.md5, SHA256: ct.sha256, ETag: ct.md5}
	}
	l1, l2, l3 := link(1, a, holidays, x), link(2, a, "2024/05-02 - Trip/IMG_0001.HEIC", x), link(3, b, holidays1, x)
	l4 := link(4, a, "2024/05-01 - Holidays/IMG_0001.MOV", video)
	l4.Role = domain.RoleLiveVideo
	l5 := link(5, c, "2024/05-02 - Trip/IMG_0002.HEIC", y)
	l9 := link(9, b, holidays, x)
	l9.Root = domain.LibraryShared
	toParty, toParty1 := file(party, x), file("2024/06-01 - Party/IMG_0001 (1).HEIC", x)
	toWork, toBeach := file("2024/06-02 - Work/IMG_0001.HEIC", x), file(beach, x)
	shaOnly := file(beach, content{100, "a9", "b1"})
	partyY, workY := file("2024/06-01 - Party/IMG_0002.HEIC", y), file("2024/06-02 - Work/IMG_0002.HEIC", y)
	liveParty := file("2024/06-01 - Party/IMG_0001.MOV", video)
	sharedHolidays, sharedParty := file(holidays, x), file(party, x)
	sharedHolidays.Root, sharedParty.Root = domain.LibraryShared, domain.LibraryShared
	links := func(l ...domain.Link) []domain.Link { return l }
	files := func(e ...domain.RemoteEntry) []domain.RemoteEntry { return e }
	to := func(l domain.Link, e domain.RemoteEntry) plan.Move { return plan.Move{Link: l, To: e} }
	moves := func(m ...plan.Move) []plan.Move { return m }
	ambiguous := func(paths ...string) []domain.Notice {
		var n []domain.Notice
		for _, p := range paths {
			n = append(n, domain.Notice{Code: "plan.ambiguous_move", Params: []string{"personal", p}})
		}
		return n
	}
	describe := func(ms []plan.Move) []string {
		var s []string
		for _, m := range ms {
			s = append(s, fmt.Sprintf("%d %s:%s", m.Link.ID, m.To.Root, m.To.Path))
		}
		return s
	}
	sameNotice := func(m, n domain.Notice) bool { return m.Code == n.Code && slices.Equal(m.Params, n.Params) }

	for _, r := range []struct {
		name  string
		hash  domain.HashKind
		gone  []domain.Link
		fresh []domain.RemoteEntry
		want  []plan.Move
		notes []domain.Notice
	}{
		{"a file moved to another folder", byMD5, links(l1), files(toParty), moves(to(l1, toParty)), nil}, // covers: R-OBJECT-08
		{"a file renamed in its folder", byMD5, links(l1), files(toBeach), moves(to(l1, toBeach)), nil},
		{"the same SHA-256 with another MD5", byMD5, links(l1), files(file(party, content{100, "a9", "b1"})), nil, nil},
		{"a renamed file found by its SHA-256 alone", bySHA256, links(l1), files(shaOnly), moves(to(l1, shaOnly)), nil},
		{"without a hash nothing moves", noHash, links(l1), files(toParty), nil, nil},
		{"neither has the MD5", byMD5, links(link(1, a, holidays, content{100, "", "b1"})), files(file(party, content{100, "", "b1"})), nil, nil},
		{"the copies of one asset moved", byMD5, links(l1, l2), files(toWork, toParty), moves(to(l1, toParty), to(l2, toWork)), nil},
		{"the copies of one asset and fewer new files", byMD5, links(l2, l1, l5), files(toParty), moves(to(l1, toParty)), nil},
		{"one link and more new files", byMD5, links(l1), files(toWork, toParty), moves(to(l1, toParty)), nil},
		{"a copy deleted and another put elsewhere, beside a file of other content", byMD5, links(l1), files(partyY, toWork), moves(to(l1, toWork)), nil},
		{"the duplicates of two assets moved", byMD5, links(l1, l3, l5), files(toParty, toParty1), moves(to(l3, toParty1), to(l1, toParty)), nil},
		{"the duplicates of two assets and a third copy", byMD5, links(l1, l3), files(toWork, toParty, toParty1), moves(to(l3, toParty1), to(l1, toParty)), nil},
		{"the duplicates of two assets and one new file", byMD5, links(l1, l3), files(toParty), nil, ambiguous(holidays1, holidays)},
		{"the duplicates of two assets deleted", byMD5, links(l1, l3), nil, nil, nil},
		{"an ambiguous content beside a clear one", byMD5, links(l5, l1, l3), files(workY, toParty), moves(to(l5, workY)), ambiguous(holidays1, holidays)},
		{"the video of a Live Photo moved apart from its photo", byMD5, links(l4), files(liveParty), moves(to(l4, liveParty)), nil},
		{"a file put in the other root", byMD5, links(l1), files(sharedHolidays), moves(to(l1, sharedHolidays)), nil},
		{"one path in both roots", byMD5, links(l9, l1), files(sharedParty, toParty), moves(to(l1, toParty), to(l9, sharedParty)), nil},
		{"duplicates moved within each root", byMD5, links(l9, l1), files(sharedParty, toWork), moves(to(l1, toWork), to(l9, sharedParty)), nil},
	} {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			gone, fresh := slices.Clone(r.gone), slices.Clone(r.fresh)
			got, notes := plan.MovesByContent(r.gone, r.fresh, r.hash)
			if !slices.Equal(got, r.want) {
				t.Errorf("MovesByContent = %v; want %v", describe(got), describe(r.want))
			}
			if !slices.EqualFunc(notes, r.notes, sameNotice) {
				t.Errorf("notices = %q; want %q", notes, r.notes)
			}
			if !slices.Equal(r.gone, gone) || !slices.Equal(r.fresh, fresh) {
				t.Errorf("MovesByContent changed its input")
			}
			slices.Reverse(gone)
			slices.Reverse(fresh)
			again, againNotes := plan.MovesByContent(gone, fresh, r.hash)
			if !slices.Equal(again, got) || !slices.EqualFunc(againNotes, notes, sameNotice) {
				t.Errorf("with the input reversed: %v, %q; want %v, %q", describe(again), againNotes, describe(got), notes)
			}
		})
	}
}
