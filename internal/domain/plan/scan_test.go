package plan_test

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Each row is a scan of the remote against the links of the state, as the
// user left the cloud since the last applied run. The expected files, new
// entries, edits and notices are written from the rules on links, quarantine,
// detached links, moves by content, edits and the own files of photosync, not
// from the code, and must not depend on the order of the links or the scan.
func TestReadScan(t *testing.T) {
	t.Parallel()
	const (
		personal, shared = domain.LibraryPersonal, domain.LibraryShared
		byMD5, noHash    = domain.HashMD5, domain.HashNone

		atHolidays, atHolidays1 = "2024/05-01 - Holidays/IMG_0001.HEIC", "2024/05-01 - Holidays/IMG_0001 (1).HEIC"
		atTrip, atParty         = "2024/05-02 - Trip/IMG_0001.HEIC", "2024/05-03 - Party/IMG_0002.HEIC"
		liveHolidays, liveTrip  = "2024/05-01 - Holidays/IMG_0001.MOV", "2024/05-02 - Trip/IMG_0001.MOV"
		anEdit, movedToEdits    = "2024/05-01 - Holidays/edited/IMG_0002.HEIC", "2024/05-01 - Holidays/edited/IMG_0001.HEIC"
		inAlbumEdited           = "Albums/Edited/IMG_0001.HEIC"
	)
	editedDirs := []string{"edited"}
	link := func(id domain.LinkID, remote string, role domain.LinkRole, status domain.LinkStatus) domain.Link {
		return domain.Link{
			ID: id, AssetID: 7, Remote: remote, Root: personal, Path: atHolidays, Role: role, AlbumID: 10,
			Size: 100, MD5: "a1", Level: domain.LevelExact, Status: status,
		}
	}
	file := func(root domain.LibraryKind, at, md5 string) domain.RemoteEntry {
		return domain.RemoteEntry{Root: root, Path: at, Size: 100, MD5: md5}
	}
	l1 := link(1, "disk", domain.RoleOriginal, domain.StatusActive)
	live := link(2, "disk", domain.RoleLiveVideo, domain.StatusActive)
	live.Path, live.MD5 = liveHolidays, "a2"
	l3 := link(3, "other", domain.RoleOriginal, domain.StatusActive)
	copyInTrip := link(4, "disk", domain.RoleOriginal, domain.StatusActive)
	copyInTrip.Path, copyInTrip.AlbumID = atTrip, 11
	l5 := link(5, "disk", domain.RoleOriginal, domain.StatusActive)
	l5.Root, l5.Path, l5.AlbumID = shared, atTrip, 11
	inCloud := link(6, "disk", domain.RoleOriginal, domain.StatusQuarantinedRemote)
	inCloud.QuarantinePath = ".photosync/quarantine/2024-06-01/" + atHolidays
	inICloud := link(8, "disk", domain.RoleOriginal, domain.StatusQuarantinedICloud)
	detached := link(9, "disk", domain.RoleOriginal, domain.StatusDetached)
	duplicate := link(11, "disk", domain.RoleOriginal, domain.StatusActive)
	duplicate.AssetID, duplicate.Path = 8, atHolidays1
	newer := link(12, "disk", domain.RoleOriginal, domain.StatusActive)
	newer.AssetID, newer.MD5 = 9, "b2"
	themed := link(13, "disk", domain.RoleOriginal, domain.StatusActive)
	themed.Path, themed.AlbumID = inAlbumEdited, 20
	held, other := file(personal, atHolidays, "a1"), file(personal, atHolidays, "b2")
	inTrip, newInParty := file(personal, atTrip, "a1"), file(personal, atParty, "c3")
	sharedSame, sharedOther := file(shared, atHolidays, "a1"), file(shared, atHolidays, "b2")
	sharedTrip, liveInTrip := file(shared, atTrip, "b2"), file(personal, liveTrip, "a2")
	dirAtHolidays := domain.RemoteEntry{Root: personal, Path: atHolidays, IsDir: true}
	own := []domain.RemoteEntry{
		{Root: personal, Path: ".photosync", IsDir: true},
		file(personal, ".photosync/quarantine/2024-06-01/"+atHolidays, "a1"),
		file(shared, ".photosync/tmp/IMG_0001.HEIC", "a1"),
		newInParty,
	}
	nested := file(personal, "2024/05-01 - Holidays/.photosync/IMG_0002.HEIC", "c4")
	longer := file(personal, ".photosync-old/IMG_0003.HEIC", "c5")
	edit, toEdits, inThemed := file(personal, anEdit, "c6"), file(personal, movedToEdits, "a1"), file(personal, inAlbumEdited, "a1")
	links := func(l ...domain.Link) []domain.Link { return l }
	entries := func(e ...domain.RemoteEntry) []domain.RemoteEntry { return e }
	type found = map[domain.LinkID]domain.RemoteEntry
	ambiguous := func(paths ...string) []domain.Notice {
		var n []domain.Notice
		for _, p := range paths {
			n = append(n, domain.Notice{Code: "plan.ambiguous_move", Params: []string{"personal", p}})
		}
		return n
	}
	where := func(e domain.RemoteEntry) string {
		if e.IsDir {
			return fmt.Sprintf("%s:%s/", e.Root, e.Path)
		}
		return fmt.Sprintf("%s:%s %s", e.Root, e.Path, e.MD5)
	}
	places := func(es []domain.RemoteEntry) []string {
		var out []string
		for _, e := range es {
			out = append(out, where(e))
		}
		return out
	}
	show := func(files found, fresh, edited []domain.RemoteEntry, notices []domain.Notice) string {
		var byID []string
		for _, id := range slices.Sorted(maps.Keys(files)) {
			byID = append(byID, fmt.Sprintf("%d %s", id, where(files[id])))
		}
		return fmt.Sprintf("files %q, fresh %q, edited %q, notices %v", byID, places(fresh), places(edited), notices)
	}
	sameNotice := func(m, n domain.Notice) bool { return m.Code == n.Code && slices.Equal(m.Params, n.Params) }
	for _, c := range []struct {
		name    string
		hash    domain.HashKind
		links   []domain.Link
		scan    []domain.RemoteEntry
		files   found
		fresh   []domain.RemoteEntry
		edited  []domain.RemoteEntry
		notices []domain.Notice
	}{
		{"nothing changed", byMD5, links(l1), entries(held), found{1: held}, nil, nil, nil},
		{"a file replaced in the cloud", byMD5, links(l1), entries(other), found{1: other}, nil, nil, nil},
		{"the old content of a replaced file put elsewhere", byMD5, links(l1), entries(other, inTrip),
			found{1: other}, entries(inTrip), nil, nil},
		{"a file gone from the cloud", byMD5, links(l1), nil, nil, nil, nil, nil},
		{"a file moved to another event", byMD5, links(l1), entries(inTrip), found{1: inTrip}, nil, nil, nil},
		{"a new file", byMD5, nil, entries(newInParty), nil, entries(newInParty), nil, nil},
		{"a file moved into the other root", byMD5, links(l1), entries(sharedSame), found{1: sharedSame}, nil, nil, nil},
		{"one of two copies deleted", byMD5, links(copyInTrip, l1), entries(inTrip), found{4: inTrip}, nil, nil, nil},
		{"the duplicates of two assets and one new file", byMD5, links(l1, duplicate), entries(inTrip),
			nil, entries(inTrip), nil, ambiguous(atHolidays1, atHolidays)},
		{"the video of a Live Photo moved apart from its photo", byMD5, links(live, l1), entries(liveInTrip, held),
			found{1: held, 2: liveInTrip}, nil, nil, nil},
		{"a directory at the path of a link", byMD5, links(l1), entries(dirAtHolidays), found{1: dirAtHolidays}, nil, nil, nil},
		{"a directory at the path of a link, its file moved elsewhere", byMD5, links(l1), entries(dirAtHolidays, inTrip),
			found{1: inTrip}, nil, nil, nil},
		{"a remote without a hash", noHash, links(l1), entries(inTrip), nil, entries(inTrip), nil, nil},
		{"quarantined in iCloud, a file of the same size at its path of a remote without a hash", noHash, links(inICloud),
			entries(other), found{8: other}, nil, nil, nil},
		{"a link of another remote", byMD5, links(l3), entries(held), nil, entries(held), nil, nil},
		{"one path in both roots", byMD5, links(l1), entries(held, sharedOther), found{1: held}, entries(sharedOther), nil, nil},
		{"a file replaced under the shared root", byMD5, links(l5), entries(sharedTrip), found{5: sharedTrip}, nil, nil, nil},
		{"a detached link keeps its path though its file changed", byMD5, links(detached), entries(other),
			found{9: other}, nil, nil, nil},
		{"a detached link without its file, its content elsewhere", byMD5, links(detached), entries(inTrip),
			nil, entries(inTrip), nil, nil},
		{"quarantined in the cloud, the file brought back", byMD5, links(inCloud), entries(held), found{6: held}, nil, nil, nil},
		{"quarantined in iCloud, another file at its path", byMD5, links(inICloud), entries(other),
			found{8: other}, entries(other), nil, nil},
		{"quarantined without its file, its content elsewhere", byMD5, links(inCloud), entries(inTrip),
			nil, entries(inTrip), nil, nil},
		{"a quarantined link and a new active one at one path", byMD5, links(newer, inICloud), entries(other),
			found{8: other, 12: other}, nil, nil, nil},
		{"the own files of photosync count for nothing", byMD5, links(l1), own, nil, entries(newInParty), nil, nil},
		{"a .photosync below the top and a longer name are the user's", byMD5, nil, entries(nested, longer),
			nil, entries(longer, nested), nil, nil},
		{"an edit in an edited directory beside a new file", byMD5, nil, entries(newInParty, edit),
			nil, entries(newInParty), entries(edit), nil},
		{"a linked file in a directory named like the edits", byMD5, links(themed), entries(inThemed),
			found{13: inThemed}, nil, nil, nil},
		{"a file moved into an edited directory", byMD5, links(l1), entries(toEdits), found{1: toEdits}, nil, nil, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := domain.RemoteSpec{Name: "disk", Mode: domain.ModeTwoWay, Capabilities: domain.Capabilities{ContentHash: c.hash}}
			ls, scan := slices.Clone(c.links), slices.Clone(c.scan)
			files, fresh, edited, notices := plan.ReadScan(c.links, r, c.scan, editedDirs)
			if !maps.Equal(files, c.files) || !slices.Equal(fresh, c.fresh) || !slices.Equal(edited, c.edited) ||
				!slices.EqualFunc(notices, c.notices, sameNotice) {
				t.Errorf("ReadScan = %s; want %s", show(files, fresh, edited, notices), show(c.files, c.fresh, c.edited, c.notices))
			}
			if !slices.Equal(c.links, ls) {
				t.Errorf("ReadScan changed its input: links changed")
			}
			if !slices.Equal(c.scan, scan) {
				t.Errorf("ReadScan changed its input: scan = %q; want %q", places(c.scan), places(scan))
			}
			slices.Reverse(ls)
			slices.Reverse(scan)
			againFiles, againFresh, againEdited, againNotices := plan.ReadScan(ls, r, scan, editedDirs)
			if !maps.Equal(againFiles, files) || !slices.Equal(againFresh, fresh) || !slices.Equal(againEdited, edited) ||
				!slices.EqualFunc(againNotices, notices, sameNotice) {
				t.Errorf("with the links and the scan reversed: %s; want %s",
					show(againFiles, againFresh, againEdited, againNotices), show(files, fresh, edited, notices))
			}
		})
	}
}
