package plan_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Each row starts from files that lie where the last applied run left them
// and changes the asset, a file, the status of a link, the mode or the
// policies. The expected decisions and current links are written from the
// rules on links, modes, quarantine, detached links and the albums of the
// personal root, not from the code, and must not depend on the order of the
// links.
func TestDecideRemoteLinks(t *testing.T) {
	t.Parallel()
	const (
		holidays, trip, party domain.AlbumID = 10, 11, 12

		personal, shared       = domain.LibraryPersonal, domain.LibraryShared
		unchanged, moved       = domain.ChangeUnchanged, domain.ChangeMoved
		replaced, deleted      = domain.ChangeReplaced, domain.ChangeDeleted
		none, added            = domain.ChangeNone, domain.ChangeNew
		twoWay, uploadOnly     = domain.ModeTwoWay, domain.ModeUploadOnly
		moveAsset, moveFile    = domain.ActionMoveAsset, domain.ActionMoveFile
		updateBase             = domain.ActionUpdateBase
		atHolidays, atTrip     = "2024/05-01 - Holidays/IMG_0001.HEIC", "2024/05-02 - Trip/IMG_0001.HEIC"
		atParty, atDay         = "2024/05-03 - Party/IMG_0001.HEIC", "2024/05-01/IMG_0001.HEIC"
		liveHolidays, liveTrip = "2024/05-01 - Holidays/IMG_0001.MOV", "2024/05-02 - Trip/IMG_0001.MOV"
	)
	albums := []domain.Album{
		{ID: 1, UUID: "album-1", Kind: domain.KindFolder, Title: "2024"},
		{ID: holidays, UUID: "album-10", Title: "05/01 Holidays", ParentID: 1, FolderTitles: []string{"2024"}},
		{ID: trip, UUID: "album-11", Title: "05/02 Trip", ParentID: 1, FolderTitles: []string{"2024"}},
		{ID: party, UUID: "album-12", Title: "05/03 Party", ParentID: 1, FolderTitles: []string{"2024"}},
	}
	pairs := []domain.AlbumPair{
		{AlbumID: holidays, Remote: "disk", Root: personal, DirPath: "2024/05-01 - Holidays", CreatedBy: domain.OriginICloud},
		{AlbumID: trip, Remote: "disk", Root: personal, DirPath: "2024/05-02 - Trip", CreatedBy: domain.OriginICloud},
		{AlbumID: trip, Remote: "disk", Root: shared, DirPath: "2024/05-02 - Trip", CreatedBy: domain.OriginICloud},
		{AlbumID: party, Remote: "other", Root: personal, DirPath: "2024/05-03 - Party", CreatedBy: domain.OriginICloud},
	}
	asset := func(library, target domain.LibraryKind, ids ...domain.AlbumID) domain.Asset {
		a := domain.Asset{
			ID: 7, Key: domain.AssetKey{CloudAssetGUID: "guid-7", UUID: "uuid-7"},
			Library: library, TargetLibrary: target, OriginalName: "IMG_0001.HEIC",
		}
		for _, id := range ids {
			a.Memberships = append(a.Memberships, domain.Membership{AlbumID: id, FirstSeenRun: "run-1"})
		}
		return a
	}
	in := func(ids ...domain.AlbumID) domain.Asset { return asset(personal, personal, ids...) }
	trashed, hidden := in(holidays), in(holidays)
	trashed.InTrash, hidden.Hidden = true, true
	link := func(id domain.LinkID, remote string, role domain.LinkRole, status domain.LinkStatus) domain.Link {
		return domain.Link{
			ID: id, AssetID: 7, Remote: remote, Root: personal, Path: atHolidays, Role: role, AlbumID: holidays,
			Size: 100, MD5: "a1", Level: domain.LevelExact, Status: status,
		}
	}
	place := func(l domain.Link, root domain.LibraryKind, to string, album domain.AlbumID) domain.Link {
		l.Root, l.Path, l.AlbumID = root, to, album
		return l
	}
	l1 := link(1, "disk", domain.RoleOriginal, domain.StatusActive)
	live := link(2, "disk", domain.RoleLiveVideo, domain.StatusActive)
	live.Path, live.Size, live.MD5 = liveHolidays, 50, "c1"
	l3 := link(3, "other", domain.RoleOriginal, domain.StatusActive)
	source := link(4, "disk", domain.RoleSource, domain.StatusActive)
	l5 := place(link(5, "disk", domain.RoleOriginal, domain.StatusActive), shared, atTrip, trip)
	inCloud := link(6, "disk", domain.RoleOriginal, domain.StatusQuarantinedRemote)
	inCloud.QuarantinePath = ".photosync/quarantine/2024-06-01/" + atHolidays
	inICloud := place(link(8, "disk", domain.RoleOriginal, domain.StatusQuarantinedICloud), personal, atTrip, trip)
	detached := link(1, "disk", domain.RoleOriginal, domain.StatusDetached)
	detachedLive := live
	detachedLive.Status = domain.StatusDetached
	back := inCloud
	back.Status, back.QuarantinePath = domain.StatusActive, ""
	entry := func(l domain.Link, root domain.LibraryKind, to string) domain.RemoteEntry {
		return domain.RemoteEntry{Root: root, Path: to, Size: l.Size, MD5: l.MD5}
	}
	same := func(l domain.Link) domain.RemoteEntry { return entry(l, l.Root, l.Path) }
	other := same(l1)
	other.MD5 = "b2"
	dec := func(l domain.Link, icloud, remote domain.ChangeKind, a domain.LinkAction, rule string) domain.LinkDecision {
		return domain.LinkDecision{Link: l, ICloud: icloud, Remote: remote, Action: a, Rule: rule}
	}
	decs := func(d ...domain.LinkDecision) []domain.LinkDecision { return d }
	links := func(l ...domain.Link) []domain.Link { return l }
	kept := dec(l1, unchanged, unchanged, domain.ActionNone, "")
	type scan = map[domain.LinkID]domain.RemoteEntry
	p := domain.DefaultPolicies()
	skipHidden, noAlbums := p, p
	skipHidden.Hidden, noAlbums.AlbumsForPersonal = domain.HiddenSkip, false
	where := func(l domain.Link) string {
		return fmt.Sprintf("%d %s:%s #%d %s %s", l.ID, l.Root, l.Path, l.AlbumID, l.Status, l.QuarantinePath)
	}
	show := func(ds []domain.LinkDecision, ls []domain.Link) []string {
		var out []string
		for _, d := range ds {
			out = append(out, fmt.Sprintf("%s: %s/%s %s %s", where(d.Link), d.ICloud, d.Remote, d.Action, d.Rule))
		}
		for _, l := range ls {
			out = append(out, "now "+where(l))
		}
		return out
	}
	for _, c := range []struct {
		name      string
		now       domain.Asset
		found     bool
		base      domain.LibraryKind
		links     []domain.Link
		mode      domain.Mode
		files     scan
		p         domain.Policies
		decisions []domain.LinkDecision
		current   []domain.Link
		frozen    bool
	}{
		{"nothing changed", in(holidays), true, personal, links(l1), twoWay, scan{1: same(l1)}, p,
			decs(kept), links(l1), false},
		{"a file replaced in the cloud", in(holidays), true, personal, links(l1), twoWay, scan{1: other}, p,
			decs(dec(l1, unchanged, replaced, domain.ActionReplaceAsset, "R-LINK-06")), links(l1), false},
		{"a file gone from the cloud", in(holidays), true, personal, links(l1), twoWay, nil, p,
			decs(dec(l1, unchanged, deleted, domain.ActionDeleteAsset, "R-LINK-09")), nil, false},
		{"a file moved to another event", in(holidays), true, personal, links(l1), twoWay,
			scan{1: entry(l1, personal, atTrip)}, p,
			decs(dec(l1, unchanged, moved, moveAsset, "R-LINK-05")), links(place(l1, personal, atTrip, trip)), false},
		{"a file moved to a directory without a pair", in(holidays), true, personal, links(l1), twoWay,
			scan{1: entry(l1, personal, atDay)}, p,
			decs(dec(l1, unchanged, moved, moveAsset, "R-LINK-05")), links(place(l1, personal, atDay, 0)), false},
		{"a file moved into the other root, where its directory has no pair", in(holidays), true, personal, links(l1), twoWay,
			scan{1: entry(l1, shared, atHolidays)}, p,
			decs(dec(l1, unchanged, moved, moveAsset, "R-LINK-05")), links(place(l1, shared, atHolidays, 0)), false},
		{"a file moved to a directory that only another remote pairs", in(holidays), true, personal, links(l1), twoWay,
			scan{1: entry(l1, personal, atParty)}, p,
			decs(dec(l1, unchanged, moved, moveAsset, "R-LINK-05")), links(place(l1, personal, atParty, 0)), false},
		{"the asset moved to another event in iCloud", in(trip), true, personal, links(l1), twoWay, scan{1: same(l1)}, p,
			decs(dec(l1, moved, unchanged, moveFile, "R-LINK-04")), links(l1), false},
		{"the asset and its file moved: a conflict", in(trip), true, personal, links(l1), twoWay,
			scan{1: entry(l1, personal, atTrip)}, p,
			decs(dec(l1, moved, moved, domain.ActionConflict, "R-LINK-07")), links(place(l1, personal, atTrip, trip)), false},
		{"the asset moved to the shared library in Photos", asset(shared, shared, holidays), true, personal, links(l1), twoWay,
			scan{1: same(l1)}, p,
			decs(dec(l1, moved, unchanged, moveFile, "R-LINK-04")), links(l1), false},
		{"the asset in Recently Deleted", trashed, true, personal, links(l1), twoWay, scan{1: same(l1)}, p,
			decs(dec(l1, deleted, unchanged, domain.ActionDeleteFile, "R-LINK-08")), links(l1), false},
		{"the asset gone from the library", domain.Asset{}, false, personal, links(l1), twoWay, scan{1: same(l1)}, p,
			decs(dec(l1, deleted, unchanged, domain.ActionDeleteFile, "R-LINK-08")), links(l1), false},
		{"a hidden asset under hidden: skip is frozen", hidden, true, personal, links(l1), twoWay, nil, skipHidden,
			nil, links(l1), true},
		{"a hidden asset gone from the library under hidden: skip", hidden, false, personal, links(l1), twoWay,
			scan{1: same(l1)}, skipHidden,
			decs(dec(l1, deleted, unchanged, domain.ActionDeleteFile, "R-LINK-08")), links(l1), false},
		{"a detached file freezes the asset", in(holidays), true, personal, links(live, detached), twoWay, nil, p,
			nil, links(detached, live), true},
		{"a link of another remote is not decided here", in(holidays), true, personal, links(l3, l1), twoWay,
			scan{1: same(l1)}, p,
			decs(kept), links(l1), false},
		{"a detached Live Photo video freezes nothing", in(holidays), true, personal, links(detachedLive, l1), twoWay,
			scan{1: same(l1)}, p,
			decs(kept), links(l1, detachedLive), false},
		{"a Live Photo video moved: its new place without a decision", in(holidays), true, personal, links(l1, live), twoWay,
			scan{1: same(l1), 2: entry(live, personal, liveTrip)}, p,
			decs(kept), links(l1, place(live, personal, liveTrip, trip)), false},
		{"a Live Photo video gone", in(holidays), true, personal, links(l1, live), twoWay, scan{1: same(l1)}, p,
			decs(kept), links(l1), false},
		{"a source file of a converted asset moved", in(holidays), true, personal, links(source), twoWay,
			scan{4: entry(source, personal, atTrip)}, p,
			decs(dec(source, unchanged, moved, moveAsset, "R-LINK-05")), links(place(source, personal, atTrip, trip)), false},
		{"quarantined in the cloud, the file brought back", trashed, true, personal, links(inCloud), twoWay,
			scan{6: same(inCloud)}, p,
			decs(dec(inCloud, none, added, domain.ActionImport, "R-LINK-02")), links(back), false},
		{"quarantined in iCloud, the asset brought back without its file", in(holidays), true, personal, links(inICloud), twoWay,
			nil, p,
			decs(dec(inICloud, added, none, domain.ActionUpload, "R-LINK-01")), nil, false},
		{"quarantined, nothing brought back", trashed, true, personal, links(inICloud), twoWay, nil, p,
			nil, links(inICloud), false},
		{"quarantined in iCloud, the asset gone for good", domain.Asset{}, false, personal, links(inICloud), twoWay, nil, p,
			decs(dec(inICloud, deleted, deleted, domain.ActionForget, "R-LINK-12")), nil, false},
		{"a quarantined file and an active one", in(holidays), true, personal, links(inICloud, l1), twoWay,
			scan{1: same(l1)}, p,
			decs(kept, dec(inICloud, added, none, domain.ActionUpload, "R-LINK-01")), links(l1), false},
		{"upload-only: the base takes a move of the file", in(holidays), true, personal, links(l1), uploadOnly,
			scan{1: entry(l1, personal, atTrip)}, p,
			decs(dec(l1, unchanged, moved, updateBase, "R-MODE-01")), links(place(l1, personal, atTrip, holidays)), false},
		{"upload-only: a move into the other root", in(holidays), true, personal, links(l1), uploadOnly,
			scan{1: entry(l1, shared, atTrip)}, p,
			decs(dec(l1, unchanged, moved, updateBase, "R-MODE-01")), links(place(l1, shared, atTrip, holidays)), false},
		{"upload-only: the asset and its file moved", in(trip), true, personal, links(l1), uploadOnly,
			scan{1: entry(l1, personal, atTrip)}, p,
			decs(dec(l1, moved, moved, moveFile, "R-LINK-04")), links(place(l1, personal, atTrip, trip)), false},
		{"albums_for_personal false: a move within the personal root", in(holidays), true, personal, links(l1), twoWay,
			scan{1: entry(l1, personal, atTrip)}, noAlbums,
			decs(dec(l1, unchanged, moved, updateBase, "R-MODE-04")), links(place(l1, personal, atTrip, holidays)), false},
		{"albums_for_personal false: a move into the shared root", in(holidays), true, personal, links(l1), twoWay,
			scan{1: entry(l1, shared, atTrip)}, noAlbums,
			decs(dec(l1, unchanged, moved, moveAsset, "R-LINK-05")), links(place(l1, shared, atTrip, trip)), false},
		{"albums_for_personal false: a move from the shared root into the personal one", asset(shared, shared, trip), true, shared,
			links(l5), twoWay, scan{5: entry(l5, personal, atHolidays)}, noAlbums,
			decs(dec(l5, unchanged, moved, moveAsset, "R-LINK-05")), links(place(l5, personal, atHolidays, holidays)), false},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := domain.RemoteSpec{Name: "disk", Mode: c.mode, Capabilities: domain.Capabilities{ContentHash: domain.HashMD5}}
			input := slices.Clone(c.links)
			decisions, current, frozen := plan.DecideRemoteLinks(c.now, c.found, c.base, c.links, r, c.files, pairs, albums, c.p)
			if !slices.Equal(decisions, c.decisions) || !slices.Equal(current, c.current) || frozen != c.frozen {
				t.Errorf("DecideRemoteLinks = %q, frozen %v; want %q, frozen %v",
					show(decisions, current), frozen, show(c.decisions, c.current), c.frozen)
			}
			if !slices.Equal(c.links, input) {
				t.Errorf("DecideRemoteLinks changed its input to %q", show(nil, c.links))
			}
			for _, d := range c.decisions {
				if _, ok := domain.Rules[d.Rule]; d.Rule != "" && !ok {
					t.Errorf("%s is not in domain.Rules", d.Rule)
				}
			}
			slices.Reverse(input)
			again, cur, still := plan.DecideRemoteLinks(c.now, c.found, c.base, input, r, c.files, pairs, albums, c.p)
			if !slices.Equal(again, decisions) || !slices.Equal(cur, current) || still != frozen {
				t.Errorf("with the links reversed: %q, frozen %v; want %q, frozen %v",
					show(again, cur), still, show(decisions, current), frozen)
			}
		})
	}
}

// The sides that DecideRemoteLinks gives a link always meet on a decision:
// an active link has those of ICloudSide against those of RemoteSide or of a
// move by content, and a link back from quarantine those of StatusSides. In
// every mode, with the albums of the personal root kept or not, ModeDecide
// and ModeKeepAlbums decide each such pair, so DecideRemoteLinks needs no
// error for a pair that cannot meet on a link.
func TestDecideRemoteLinksDecidable(t *testing.T) {
	t.Parallel()
	type pair struct{ icloud, remote domain.ChangeKind }
	sides := []pair{
		{domain.ChangeNew, domain.ChangeNone}, {domain.ChangeNone, domain.ChangeNew}, {domain.ChangeNew, domain.ChangeNew},
	}
	for _, icloud := range []domain.ChangeKind{domain.ChangeUnchanged, domain.ChangeMoved, domain.ChangeDeleted} {
		for _, remote := range []domain.ChangeKind{domain.ChangeUnchanged, domain.ChangeMoved, domain.ChangeReplaced, domain.ChangeDeleted} {
			sides = append(sides, pair{icloud, remote})
		}
	}
	cells := 0
	for _, m := range []domain.Mode{domain.ModeTwoWay, domain.ModeUploadOnly, domain.ModeDownloadOnly} {
		for _, s := range sides {
			cells++
			_, ok := plan.ModeDecide(m, s.icloud, s.remote, false)
			_, kept := plan.ModeKeepAlbums(m, s.icloud, s.remote, false)
			if !ok || !kept {
				t.Errorf("%s, iCloud %s, cloud %s: ModeDecide %v, ModeKeepAlbums %v; want both decided", m, s.icloud, s.remote, ok, kept)
			}
		}
	}
	if cells != 45 {
		t.Errorf("checked %d cells; want 45: 15 pairs of sides in 3 modes", cells)
	}
}

// Each row reduces the decisions on the links of an asset in one remote as a
// run makes them. The expected decisions, album changes and conflicts are
// written from the rules on the reduction by asset, the modes, the albums of
// the personal root and the conflict policies, not from the code, and must
// not depend on the order of the current links and albums.
func TestDecideRemoteObject(t *testing.T) {
	t.Parallel()
	const (
		holidays, trip, party domain.AlbumID = 10, 11, 12

		personal, shared   = domain.LibraryPersonal, domain.LibraryShared
		unchanged, moved   = domain.ChangeUnchanged, domain.ChangeMoved
		deleted            = domain.ChangeDeleted
		twoWay, uploadOnly = domain.ModeTwoWay, domain.ModeUploadOnly
		remoteWins, ask    = domain.ConflictRemoteWins, domain.ConflictAsk
		atHolidays, atTrip = "2024/05-01 - Holidays/IMG_0001.HEIC", "2024/05-02 - Trip/IMG_0001.HEIC"
		atParty            = "2024/05-03 - Party/IMG_0001.HEIC"
	)
	albums := []domain.Album{
		{ID: 1, UUID: "album-1", Kind: domain.KindFolder, Title: "2024"},
		{ID: holidays, UUID: "album-10", Title: "05/01 Holidays", ParentID: 1, FolderTitles: []string{"2024"}},
		{ID: trip, UUID: "album-11", Title: "05/02 Trip", ParentID: 1, FolderTitles: []string{"2024"}},
		{ID: party, UUID: "album-12", Title: "05/03 Party", ParentID: 1, FolderTitles: []string{"2024"}},
	}
	asset := func(library, target domain.LibraryKind, ids ...domain.AlbumID) domain.Asset {
		a := domain.Asset{
			ID: 7, Key: domain.AssetKey{CloudAssetGUID: "guid-7", UUID: "uuid-7"},
			Library: library, TargetLibrary: target, OriginalName: "IMG_0001.HEIC",
		}
		for _, id := range ids {
			a.Memberships = append(a.Memberships, domain.Membership{AlbumID: id, FirstSeenRun: "run-1"})
		}
		return a
	}
	in := func(ids ...domain.AlbumID) domain.Asset { return asset(personal, personal, ids...) }
	trashed := in(holidays, trip)
	trashed.InTrash = true
	link := func(id domain.LinkID, to string, album domain.AlbumID) domain.Link {
		return domain.Link{
			ID: id, AssetID: 7, Remote: "disk", Root: personal, Path: to, Role: domain.RoleOriginal, AlbumID: album,
			Size: 100, MD5: "a1", Level: domain.LevelExact, Status: domain.StatusActive,
		}
	}
	place := func(l domain.Link, root domain.LibraryKind, to string, album domain.AlbumID) domain.Link {
		l.Root, l.Path, l.AlbumID = root, to, album
		return l
	}
	l1, l2 := link(1, atHolidays, holidays), link(2, atTrip, trip)
	decided := func(m domain.Mode, l domain.Link, icloud, remote domain.ChangeKind) domain.LinkDecision {
		d, _ := plan.ModeDecide(m, icloud, remote, false)
		d.Link = l
		return d
	}
	dec := func(l domain.Link, icloud, remote domain.ChangeKind, a domain.LinkAction, rule string) domain.LinkDecision {
		return domain.LinkDecision{Link: l, ICloud: icloud, Remote: remote, Action: a, Rule: rule}
	}
	decs := func(d ...domain.LinkDecision) []domain.LinkDecision { return d }
	links := func(l ...domain.Link) []domain.Link { return l }
	ch := func(id domain.AlbumID, rule string) []domain.AlbumChange {
		return []domain.AlbumChange{{AlbumID: id, Rule: rule}}
	}
	od := func(ds []domain.LinkDecision, join, leave []domain.AlbumChange) domain.ObjectDecision {
		return domain.ObjectDecision{Links: ds, Join: join, Leave: leave}
	}
	policies := func(conflict domain.ConflictPolicy, albumsForPersonal bool) domain.Policies {
		p := domain.DefaultPolicies()
		p.Conflict, p.AlbumsForPersonal = conflict, albumsForPersonal
		return p
	}
	recorded := func(status domain.ConflictStatus, resolution domain.ConflictPolicy) domain.Conflict {
		return domain.Conflict{AssetID: 7, Remote: "disk", Status: status, Resolution: resolution}
	}
	gone := decs(decided(twoWay, l1, unchanged, unchanged), decided(twoWay, l2, unchanged, deleted))
	trash := decs(decided(twoWay, l1, deleted, unchanged), decided(twoWay, l2, deleted, deleted))
	fileMoved := decs(decided(twoWay, l1, unchanged, moved))
	both := decs(decided(twoWay, l1, moved, moved))
	show := func(o domain.ObjectDecision) []string {
		var out []string
		for _, d := range o.Links {
			out = append(out, fmt.Sprintf("%d %s/%s %s %s", d.Link.ID, d.ICloud, d.Remote, d.Action, d.Rule))
		}
		for _, x := range o.Join {
			out = append(out, fmt.Sprintf("join %d %s", x.AlbumID, x.Rule))
		}
		for _, x := range o.Leave {
			out = append(out, fmt.Sprintf("leave %d %s", x.AlbumID, x.Rule))
		}
		return out
	}
	for _, c := range []struct {
		name      string
		now       domain.Asset
		found     bool
		mode      domain.Mode
		decisions []domain.LinkDecision
		links     []domain.Link
		stored    domain.Conflict
		p         domain.Policies
		want      domain.ObjectDecision
		conflict  domain.Conflict
		in        bool
	}{
		{"two-way: one of two files gone", in(holidays, trip), true, twoWay, gone, links(l1),
			recorded("", ""), policies(remoteWins, true),
			od(decs(dec(l1, unchanged, unchanged, domain.ActionNone, ""), dec(l2, unchanged, deleted, domain.ActionForget, "R-OBJECT-02")),
				nil, ch(trip, "R-OBJECT-02")),
			recorded("", ""), true},
		{"upload-only: the decisions stand as the mode gives them", in(holidays, trip), true, uploadOnly,
			decs(decided(uploadOnly, l1, unchanged, unchanged), decided(uploadOnly, l2, unchanged, deleted)), links(l1),
			recorded("", ""), policies(remoteWins, true),
			od(decs(dec(l1, unchanged, unchanged, domain.ActionNone, ""), dec(l2, unchanged, deleted, domain.ActionNone, "R-MODE-01")), nil, nil),
			recorded("", ""), true},
		{"an asset in Recently Deleted: the decisions stand", trashed, true, twoWay, trash, links(l1),
			recorded("", ""), policies(remoteWins, true),
			od(decs(dec(l1, deleted, unchanged, domain.ActionDeleteFile, "R-LINK-08"), dec(l2, deleted, deleted, domain.ActionForget, "R-LINK-12")), nil, nil),
			recorded("", ""), true},
		{"an asset gone from the library: the decisions stand", domain.Asset{}, false, twoWay, trash, links(l1),
			recorded("", ""), policies(remoteWins, true),
			od(decs(dec(l1, deleted, unchanged, domain.ActionDeleteFile, "R-LINK-08"), dec(l2, deleted, deleted, domain.ActionForget, "R-LINK-12")), nil, nil),
			recorded("", ""), true},
		{"albums_for_personal false: an asset of the personal root joins and leaves no album", in(holidays), true, twoWay, fileMoved,
			links(place(l1, shared, atTrip, trip)), recorded("", ""), policies(remoteWins, false),
			od(decs(dec(l1, unchanged, moved, domain.ActionMoveAsset, "R-LINK-05")), nil, nil),
			recorded("", ""), true},
		{"albums_for_personal false: an asset waiting to move to the shared library keeps its album changes", asset(personal, shared, holidays), true, twoWay, fileMoved,
			links(place(l1, shared, atTrip, trip)), recorded("", ""), policies(remoteWins, false),
			od(decs(dec(l1, unchanged, moved, domain.ActionMoveAsset, "R-LINK-05")), ch(trip, "R-LINK-05"), ch(holidays, "R-LINK-05")),
			recorded("", ""), true},
		{"a conflict under ask stays out of the plan", in(trip), true, twoWay, both, links(place(l1, personal, atParty, party)),
			recorded("", ""), policies(ask, true), od(nil, nil, nil),
			recorded(domain.ConflictStatusOpen, ""), false},
		{"a conflict under skip stays out of the plan", in(trip), true, twoWay, both, links(place(l1, personal, atParty, party)),
			recorded("", ""), policies(domain.ConflictSkip, true), od(nil, nil, nil),
			recorded(domain.ConflictStatusSkipped, ""), false},
		{"a recorded conflict won by the cloud", in(trip), true, twoWay, both, links(place(l1, personal, atParty, party)),
			recorded(domain.ConflictStatusOpen, ""), policies(remoteWins, true),
			od(decs(dec(l1, moved, moved, domain.ActionMoveAsset, "R-LINK-07")), ch(party, "R-LINK-07"), ch(holidays, "R-LINK-07")),
			recorded(domain.ConflictStatusResolved, ""), true},
		{"the user chose iCloud for a recorded conflict", in(trip), true, twoWay, both, links(place(l1, personal, atParty, party)),
			recorded(domain.ConflictStatusOpen, domain.ConflictICloudWins), policies(ask, true),
			od(decs(dec(l1, moved, moved, domain.ActionMoveFile, "R-LINK-07")), nil, nil),
			recorded(domain.ConflictStatusResolved, domain.ConflictICloudWins), true},
		{"both moved to where the layout keeps the file: no conflict", in(trip), true, twoWay, both, links(place(l1, personal, atTrip, trip)),
			recorded("", ""), policies(ask, true),
			od(decs(dec(l1, moved, moved, domain.ActionUpdateBase, "R-LINK-13")), nil, nil),
			recorded("", ""), true},
		{"no decisions: a recorded conflict closes by itself", in(holidays), true, twoWay, nil, nil,
			recorded(domain.ConflictStatusSkipped, ""), policies(remoteWins, true), od(nil, nil, nil),
			recorded(domain.ConflictStatusResolved, ""), true},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := domain.RemoteSpec{Name: "disk", Mode: c.mode}
			decisions, input := slices.Clone(c.decisions), slices.Clone(c.links)
			o, conflict, ok := plan.DecideRemoteObject(c.now, c.found, c.decisions, c.links, r, albums, c.stored, c.p)
			w := c.want
			if !slices.Equal(o.Links, w.Links) || !slices.Equal(o.Join, w.Join) || !slices.Equal(o.Leave, w.Leave) ||
				conflict != c.conflict || ok != c.in {
				t.Errorf("DecideRemoteObject = %q, %+v, in %v; want %q, %+v, in %v", show(o), conflict, ok, show(w), c.conflict, c.in)
			}
			if !slices.Equal(c.decisions, decisions) || !slices.Equal(c.links, input) {
				t.Errorf("DecideRemoteObject changed its input to %q", show(domain.ObjectDecision{Links: c.decisions}))
			}
			var rules []string
			for _, d := range w.Links {
				rules = append(rules, d.Rule)
			}
			for _, x := range slices.Concat(w.Join, w.Leave) {
				rules = append(rules, x.Rule)
			}
			for _, id := range rules {
				if _, known := domain.Rules[id]; id != "" && !known {
					t.Errorf("%s is not in domain.Rules", id)
				}
			}
			all := slices.Clone(albums)
			slices.Reverse(all)
			slices.Reverse(input)
			again, settled, still := plan.DecideRemoteObject(c.now, c.found, c.decisions, input, r, all, c.stored, c.p)
			if !slices.Equal(again.Links, o.Links) || !slices.Equal(again.Join, o.Join) || !slices.Equal(again.Leave, o.Leave) ||
				settled != conflict || still != ok {
				t.Errorf("with the current links and albums reversed: %q, %+v, in %v; want %q, %+v, in %v",
					show(again), settled, still, show(o), conflict, ok)
			}
		})
	}
}
