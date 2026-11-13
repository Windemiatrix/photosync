package plan_test

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Each row lays out one asset from what the files of each remote made of it
// in the run. The expected decisions, statuses, albums and placements are
// written from the rules on deletions, the deletes policy, the first
// placement, the priority of remotes, the modes and the layout of files, not
// from the code, and must not depend on the order of the links, the albums or
// the memberships. The remotes keep the order of the configuration, which is
// the priority while the policies name none.
func TestDecideLayout(t *testing.T) {
	t.Parallel()
	const (
		holidays, trip, party, old, gone domain.AlbumID = 10, 11, 12, 13, 99

		personal, shared                 = domain.LibraryPersonal, domain.LibraryShared
		unchanged, moved, replaced       = domain.ChangeUnchanged, domain.ChangeMoved, domain.ChangeReplaced
		deleted, added, absent           = domain.ChangeDeleted, domain.ChangeNew, domain.ChangeNone
		twoWay, uploadOnly, downloadOnly = domain.ModeTwoWay, domain.ModeUploadOnly, domain.ModeDownloadOnly
		nothing, upload, forget          = domain.ActionNone, domain.ActionUpload, domain.ActionForget
		moveAsset, moveFile, copyFile    = domain.ActionMoveAsset, domain.ActionMoveFile, domain.ActionCopyFile
		deleteAsset, deleteFile          = domain.ActionDeleteAsset, domain.ActionDeleteFile
		replace, toQuarantine            = domain.ActionReplaceAsset, domain.ActionQuarantineFile
		active, detached                 = domain.StatusActive, domain.StatusDetached
		inICloud, inCloud                = domain.StatusQuarantinedICloud, domain.StatusQuarantinedRemote

		atHolidays, atHolidays1 = "2024/05-01 - Holidays/IMG_0001.HEIC", "2024/05-01 - Holidays/IMG_0001 (1).HEIC"
		atTrip, atParty         = "2024/05-02 - Trip/IMG_0001.HEIC", "2024/05-03 - Party/IMG_0001.HEIC"
		atOld, atGone           = "2024/05-04 - Old/IMG_0001.HEIC", "2024/05-05 - Gone/IMG_0001.HEIC"
		atDay                   = "2024/05-01/IMG_0001.HEIC"
	)
	albums := []domain.Album{
		{ID: 1, UUID: "album-1", Kind: domain.KindFolder, Title: "2024"},
		{ID: holidays, UUID: "album-10", Title: "05/01 Holidays", ParentID: 1, FolderTitles: []string{"2024"}},
		{ID: trip, UUID: "album-11", Title: "05/02 Trip", ParentID: 1, FolderTitles: []string{"2024"}},
		{ID: party, UUID: "album-12", Title: "05/03 Party", ParentID: 1, FolderTitles: []string{"2024"}},
		{ID: old, UUID: "album-13", Title: "05/04 Old", ParentID: 1, FolderTitles: []string{"2024"}, InTrash: true},
	}
	asset := func(ids ...domain.AlbumID) domain.Asset {
		a := domain.Asset{
			ID: 7, Key: domain.AssetKey{CloudAssetGUID: "guid-7", UUID: "uuid-7"},
			Library: personal, TargetLibrary: personal, OriginalName: "IMG_0001.HEIC",
			Taken: time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC),
		}
		for _, id := range ids {
			a.Memberships = append(a.Memberships, domain.Membership{AlbumID: id, FirstSeenRun: "run-1"})
		}
		return a
	}
	trashed, hidden := asset(trip, holidays), asset()
	trashed.InTrash, hidden.Hidden = true, true
	file := func(id domain.LinkID, remote string, album domain.AlbumID, at string) domain.Link {
		return domain.Link{
			ID: id, AssetID: 7, Remote: remote, Root: personal, Path: at, Role: domain.RoleOriginal, AlbumID: album,
			Size: 100, MD5: "a1", Level: domain.LevelExact, Status: active,
		}
	}
	place := func(l domain.Link, root domain.LibraryKind, album domain.AlbumID, at string) domain.Link {
		l.Root, l.AlbumID, l.Path = root, album, at
		return l
	}
	a1, a2, inOld := file(1, "alpha", holidays, atHolidays), file(2, "alpha", party, atParty), file(14, "alpha", old, atOld)
	b1, b2, g1 := file(3, "beta", holidays, atHolidays), file(4, "beta", party, atParty), file(5, "gamma", holidays, atHolidays)
	back, daily, stuck := file(6, "alpha", holidays, atHolidays), file(8, "alpha", 0, atDay), file(9, "beta", holidays, atHolidays)
	back.Status, stuck.Status = inICloud, detached
	a1Trip, a1Shared, a1Old := place(a1, personal, trip, atTrip), place(a1, shared, holidays, atHolidays), place(a1, personal, old, atOld)
	b1Trip, b1Gone, b2Here := place(b1, personal, trip, atTrip), place(b1, personal, gone, atGone), place(b2, personal, holidays, atHolidays1)
	dec := func(l domain.Link, icloud, remote domain.ChangeKind, a domain.LinkAction, rule string) domain.LinkDecision {
		return domain.LinkDecision{Link: l, ICloud: icloud, Remote: remote, Action: a, Rule: rule}
	}
	kept := func(l domain.Link) domain.LinkDecision { return dec(l, unchanged, unchanged, nothing, "") }
	decs := func(d ...domain.LinkDecision) []domain.LinkDecision { return d }
	ch := func(id domain.AlbumID, rule string) []domain.AlbumChange {
		return []domain.AlbumChange{{AlbumID: id, Rule: rule}}
	}
	od := func(join, leave []domain.AlbumChange, d ...domain.LinkDecision) domain.ObjectDecision {
		return domain.ObjectDecision{Links: d, Join: join, Leave: leave}
	}
	links := func(l ...domain.Link) []domain.Link { return l }
	spec := func(name string, m domain.Mode) domain.RemoteSpec {
		return domain.RemoteSpec{Name: name, Mode: m, Capabilities: domain.Capabilities{ContentHash: domain.HashMD5}}
	}
	alpha, beta, gamma := spec("alpha", twoWay), spec("beta", twoWay), spec("gamma", twoWay)
	betaUp, gammaDown := spec("beta", uploadOnly), spec("gamma", downloadOnly)
	config := func(r ...domain.RemoteSpec) []domain.RemoteSpec { return r }
	under := func(deletes domain.DeletesPolicy, priority ...string) domain.Policies {
		p := domain.DefaultPolicies()
		p.Deletes, p.RemotePriority = deletes, priority
		return p
	}
	quarantine := under(domain.DeletesQuarantine)
	skipHidden, noAlbums := under(domain.DeletesQuarantine), under(domain.DeletesQuarantine)
	skipHidden.Hidden, noAlbums.AlbumsForPersonal = domain.HiddenSkip, false
	m := func(id domain.AlbumID, run string) domain.Membership {
		return domain.Membership{AlbumID: id, FirstSeenRun: run}
	}
	ms := func(x ...domain.Membership) []domain.Membership { return x }
	pl := func(a domain.LinkAction, l domain.Link, album domain.AlbumID, rule string) domain.Placement {
		return domain.Placement{Action: a, Link: l, AlbumID: album, Rule: rule}
	}
	up := func(album domain.AlbumID, rule string) domain.Placement {
		return domain.Placement{Action: upload, AlbumID: album, Rule: rule}
	}
	pls := func(x ...domain.Placement) []domain.Placement { return x }
	type byRemote = map[string]domain.ObjectDecision
	type decided = map[string][]domain.LinkDecision
	type statuses = map[domain.LinkID]domain.LinkStatus
	type placed = map[string][]domain.Placement
	deletion := byRemote{
		"alpha": od(nil, nil, dec(a1, unchanged, deleted, deleteAsset, "R-LINK-09")), "beta": od(nil, nil, kept(b1)), "gamma": od(nil, nil),
	}
	replacing := byRemote{
		"alpha": od(nil, nil, dec(a1, unchanged, replaced, replace, "R-LINK-06")),
		"beta":  od(nil, nil, dec(b1, unchanged, replaced, replace, "R-LINK-06")),
	}
	type result struct {
		decisions   decided
		status      statuses
		target      domain.Asset
		join, leave []domain.AlbumChange
		placements  placed
	}
	equal := func(x, y result) bool {
		rest := func(a domain.Asset) domain.Asset {
			a.Memberships = nil
			return a
		}
		return maps.EqualFunc(x.decisions, y.decisions, slices.Equal[[]domain.LinkDecision]) && maps.Equal(x.status, y.status) &&
			slices.Equal(x.target.Memberships, y.target.Memberships) && reflect.DeepEqual(rest(x.target), rest(y.target)) &&
			slices.Equal(x.join, y.join) && slices.Equal(x.leave, y.leave) &&
			maps.EqualFunc(x.placements, y.placements, slices.Equal[[]domain.Placement])
	}
	show := func(r result) []string {
		var out []string
		for _, name := range slices.Sorted(maps.Keys(r.decisions)) {
			out = append(out, name+":")
			for _, d := range r.decisions[name] {
				out = append(out, fmt.Sprintf("%s %d %s/%s %s %s", name, d.Link.ID, d.ICloud, d.Remote, d.Action, d.Rule))
			}
		}
		out = append(out, fmt.Sprintf("status %v", r.status),
			fmt.Sprintf("albums %v in %q", r.target.Memberships, r.target.TargetLibrary),
			fmt.Sprintf("join %v leave %v", r.join, r.leave))
		for _, name := range slices.Sorted(maps.Keys(r.placements)) {
			for _, x := range r.placements[name] {
				out = append(out, fmt.Sprintf("%s %s %d #%d %s", name, x.Action, x.Link.ID, x.AlbumID, x.Rule))
			}
		}
		return out
	}
	sameObject := func(x, y domain.ObjectDecision) bool {
		return slices.Equal(x.Links, y.Links) && slices.Equal(x.Join, y.Join) && slices.Equal(x.Leave, y.Leave)
	}
	for _, c := range []struct {
		name          string
		now           domain.Asset
		found, linked bool
		objects       byRemote
		links         []domain.Link
		remotes       []domain.RemoteSpec
		p             domain.Policies
		decisions     decided
		status        statuses
		albums        []domain.Membership
		library       domain.LibraryKind
		join, leave   []domain.AlbumChange
		placements    placed
	}{
		{"nothing changed", asset(holidays), true, true,
			byRemote{"alpha": od(nil, nil, kept(a1))}, links(a1), config(alpha), quarantine,
			decided{"alpha": decs(kept(a1))}, statuses{1: active}, ms(m(holidays, "run-1")), personal, nil, nil, nil},
		{"a move in one remote reaches the others as their modes let them", asset(holidays), true, true,
			byRemote{
				"alpha": od(ch(trip, "R-LINK-05"), ch(holidays, "R-LINK-05"), dec(a1, unchanged, moved, moveAsset, "R-LINK-05")),
				"beta":  od(nil, nil, kept(b1)),
				"gamma": od(nil, nil, kept(g1)),
			},
			links(a1Trip, b1, g1), config(alpha, betaUp, gammaDown), quarantine,
			decided{"alpha": decs(dec(a1, unchanged, moved, moveAsset, "R-LINK-05")), "beta": decs(kept(b1)), "gamma": decs(kept(g1))},
			statuses{1: active, 3: active, 5: active}, ms(m(trip, "run-2")), personal, ch(trip, "R-LINK-05"), ch(holidays, "R-LINK-05"),
			placed{"beta": pls(pl(moveFile, b1, trip, "R-OBJECT-05"))}},
		{"a new asset joins the only event of its day and goes up where the mode lets it", asset(), true, false,
			byRemote{"alpha": od(nil, nil), "beta": od(nil, nil), "gamma": od(nil, nil)}, nil, config(alpha, betaUp, gammaDown), quarantine,
			decided{"alpha": nil, "beta": nil, "gamma": nil}, nil, ms(m(holidays, "run-2")), personal, ch(holidays, "R-LAYOUT-05"), nil,
			placed{"alpha": pls(up(holidays, "R-LINK-01")), "beta": pls(up(holidays, "R-LINK-01"))}},
		{"a new asset in two events leaves the farther one at its first placement", asset(holidays, trip), true, false,
			byRemote{"alpha": od(nil, nil)}, nil, config(alpha), quarantine,
			decided{"alpha": nil}, nil, ms(m(holidays, "run-1")), personal, nil, ch(trip, "R-LAYOUT-04"),
			placed{"alpha": pls(up(holidays, "R-LINK-01"))}},
		{"an asset with a link gets no first placement, though it is in no album", asset(), true, true,
			byRemote{"alpha": od(nil, nil, kept(daily))}, links(daily), config(alpha), quarantine,
			decided{"alpha": decs(kept(daily))}, statuses{8: active}, nil, personal, nil, nil, nil},
		{"an asset gone from the library is not laid out", domain.Asset{}, false, true,
			byRemote{"alpha": od(nil, nil, dec(a1, deleted, unchanged, deleteFile, "R-LINK-08"))}, links(a1), config(alpha), quarantine,
			decided{"alpha": decs(dec(a1, deleted, unchanged, toQuarantine, "R-STATUS-01"))}, statuses{1: inCloud}, nil, "", nil, nil, nil},
		{"an asset in Recently Deleted is not laid out, not even in a remote without it", trashed, true, true,
			byRemote{"alpha": od(nil, nil, dec(a1, deleted, unchanged, deleteFile, "R-LINK-08")), "beta": od(nil, nil)},
			links(a1), config(alpha, beta), quarantine,
			decided{"alpha": decs(dec(a1, deleted, unchanged, toQuarantine, "R-STATUS-01")), "beta": nil}, statuses{1: inCloud},
			ms(m(holidays, "run-1"), m(trip, "run-1")), personal, nil, nil, nil},
		{"a hidden asset that does not sync gets no first placement", hidden, true, false,
			nil, nil, config(alpha), skipHidden,
			nil, nil, nil, personal, nil, nil, nil},
		{"under quarantine a deletion that stands reaches the other remotes and lays out nothing", asset(holidays), true, true,
			deletion, links(b1), config(alpha, beta, gamma), quarantine,
			decided{
				"alpha": decs(dec(a1, unchanged, deleted, deleteAsset, "R-STATUS-02")),
				"beta":  decs(dec(b1, deleted, unchanged, toQuarantine, "R-STATUS-01")),
				"gamma": nil,
			},
			statuses{1: inICloud, 3: inCloud}, ms(m(holidays, "run-1")), personal, nil, nil, nil},
		{"under none a deleted file detaches its remote and the others go on", asset(holidays), true, true,
			deletion, links(b1), config(alpha, beta, gamma), under(domain.DeletesNone),
			decided{"alpha": decs(dec(a1, unchanged, deleted, nothing, "R-STATUS-05")), "beta": decs(kept(b1)), "gamma": nil},
			statuses{1: detached, 3: active}, ms(m(holidays, "run-1")), personal, nil, nil,
			placed{"gamma": pls(up(holidays, "R-LINK-01"))}},
		{"under propagate a deletion that stands leaves no link", asset(holidays), true, true,
			deletion, links(b1), config(alpha, beta, gamma), under(domain.DeletesPropagate),
			decided{
				"alpha": decs(dec(a1, unchanged, deleted, deleteAsset, "R-STATUS-06")),
				"beta":  decs(dec(b1, deleted, unchanged, deleteFile, "R-STATUS-06")),
				"gamma": nil,
			},
			statuses{1: "", 3: ""}, ms(m(holidays, "run-1")), personal, nil, nil, nil},
		{"upload-only: a file that the user deleted does not go up again", asset(holidays), true, true,
			byRemote{"alpha": od(nil, nil, kept(a1)), "beta": od(nil, nil, dec(b1, unchanged, deleted, nothing, "R-MODE-01"))},
			links(a1), config(alpha, betaUp), quarantine,
			decided{"alpha": decs(kept(a1)), "beta": decs(dec(b1, unchanged, deleted, nothing, "R-MODE-01"))},
			statuses{1: active, 3: detached}, ms(m(holidays, "run-1")), personal, nil, nil, nil},
		{"a deletion beaten by a move elsewhere goes up again by R-LINK-10, and to a remote without the asset by R-LINK-01", asset(holidays), true, true,
			byRemote{
				"alpha": od(nil, nil, dec(a1, unchanged, deleted, deleteAsset, "R-LINK-09")),
				"beta":  od(ch(trip, "R-LINK-05"), ch(holidays, "R-LINK-05"), dec(b1, unchanged, moved, moveAsset, "R-LINK-05")),
				"gamma": od(nil, nil),
			},
			links(b1Trip), config(alpha, beta, gamma), quarantine,
			decided{
				"alpha": decs(dec(a1, moved, deleted, upload, "R-LINK-10")),
				"beta":  decs(dec(b1, unchanged, moved, moveAsset, "R-LINK-05")),
				"gamma": nil,
			},
			statuses{1: active, 3: active}, ms(m(trip, "run-2")), personal, ch(trip, "R-LINK-05"), ch(holidays, "R-LINK-05"),
			placed{"alpha": pls(up(trip, "R-LINK-10")), "gamma": pls(up(trip, "R-LINK-01"))}},
		{"a link back from quarantine without its file goes up by R-LINK-01", asset(holidays), true, true,
			byRemote{"alpha": od(nil, nil, dec(back, added, absent, upload, "R-LINK-01"))}, nil, config(alpha), quarantine,
			decided{"alpha": decs(dec(back, added, absent, upload, "R-LINK-01"))}, statuses{6: active},
			ms(m(holidays, "run-1")), personal, nil, nil,
			placed{"alpha": pls(up(holidays, "R-LINK-01"))}},
		{"two remotes replace the asset: the first in the configuration while the policies name no priority", asset(holidays), true, true,
			replacing, links(a1, b1), config(beta, alpha), quarantine,
			decided{"alpha": decs(dec(a1, unchanged, replaced, domain.ActionUpdateBase, "R-OBJECT-09")), "beta": decs(dec(b1, unchanged, replaced, replace, "R-LINK-06"))},
			statuses{1: active, 3: active}, ms(m(holidays, "run-1")), personal, nil, nil, nil},
		{"the priority that the policies name goes before the configuration", asset(holidays), true, true,
			replacing, links(a1, b1), config(beta, alpha), under(domain.DeletesQuarantine, "alpha"),
			decided{"alpha": decs(dec(a1, unchanged, replaced, replace, "R-LINK-06")), "beta": decs(dec(b1, unchanged, replaced, domain.ActionUpdateBase, "R-OBJECT-09"))},
			statuses{1: active, 3: active}, ms(m(holidays, "run-1")), personal, nil, nil, nil},
		{"remotes that disagree on an album: the first in the configuration decides it", asset(holidays, party), true, true,
			byRemote{
				"alpha": od(ch(trip, "R-LINK-05"), ch(holidays, "R-LINK-05"), dec(a1, unchanged, moved, moveAsset, "R-LINK-05"), kept(a2)),
				"beta":  od(ch(holidays, "R-LINK-05"), ch(party, "R-LINK-05"), kept(b1), dec(b2, unchanged, moved, moveAsset, "R-LINK-05")),
			},
			links(a1Trip, a2, b1, b2Here), config(beta, alpha), quarantine,
			decided{
				"alpha": decs(dec(a1, unchanged, moved, moveAsset, "R-LINK-05"), kept(a2)),
				"beta":  decs(kept(b1), dec(b2, unchanged, moved, moveAsset, "R-LINK-05")),
			},
			statuses{1: active, 2: active, 3: active, 4: active}, ms(m(holidays, "run-1"), m(trip, "run-2")), personal,
			ch(trip, "R-LINK-05"), ch(party, "R-LINK-05"),
			placed{"alpha": pls(pl(moveFile, a2, holidays, "R-OBJECT-05")), "beta": pls(pl(copyFile, b2Here, trip, "R-OBJECT-03"))}},
		{"remotes that disagree on an album: the priority that the policies name decides it", asset(holidays, party), true, true,
			byRemote{
				"alpha": od(ch(trip, "R-LINK-05"), ch(holidays, "R-LINK-05"), dec(a1, unchanged, moved, moveAsset, "R-LINK-05"), kept(a2)),
				"beta":  od(ch(holidays, "R-LINK-05"), ch(party, "R-LINK-05"), kept(b1), dec(b2, unchanged, moved, moveAsset, "R-LINK-05")),
			},
			links(a1Trip, a2, b1, b2Here), config(beta, alpha), under(domain.DeletesQuarantine, "alpha"),
			decided{
				"alpha": decs(dec(a1, unchanged, moved, moveAsset, "R-LINK-05"), kept(a2)),
				"beta":  decs(kept(b1), dec(b2, unchanged, moved, moveAsset, "R-LINK-05")),
			},
			statuses{1: active, 2: active, 3: active, 4: active}, ms(m(trip, "run-2")), personal,
			ch(trip, "R-LINK-05"), slices.Concat(ch(holidays, "R-LINK-05"), ch(party, "R-LINK-05")),
			placed{
				"alpha": pls(pl(toQuarantine, a2, 0, "R-OBJECT-04")),
				"beta":  pls(pl(moveFile, b2Here, trip, "R-OBJECT-05"), pl(toQuarantine, b1, 0, "R-OBJECT-04")),
			}},
		{"a file moved into the other root takes the files of the other remotes there", asset(holidays), true, true,
			byRemote{"alpha": od(nil, nil, dec(a1, unchanged, moved, moveAsset, "R-LINK-05")), "beta": od(nil, nil, kept(b1))},
			links(a1Shared, b1), config(alpha, beta), quarantine,
			decided{"alpha": decs(dec(a1, unchanged, moved, moveAsset, "R-LINK-05")), "beta": decs(kept(b1))},
			statuses{1: active, 3: active}, ms(m(holidays, "run-1")), shared, nil, nil,
			placed{"beta": pls(pl(moveFile, b1, holidays, "R-OBJECT-07"))}},
		{"albums_for_personal false: a move of the asset in iCloud still moves its file under the personal root", asset(trip), true, true,
			byRemote{"alpha": od(nil, nil, dec(a1, moved, unchanged, moveFile, "R-LINK-04"))}, links(a1), config(alpha), noAlbums,
			decided{"alpha": decs(dec(a1, moved, unchanged, moveFile, "R-LINK-04"))}, statuses{1: active}, ms(m(trip, "run-1")), personal, nil, nil,
			placed{"alpha": pls(pl(moveFile, a1, trip, "R-OBJECT-05"))}},
		{"a Join of an album in Recently Deleted or gone from the library is dropped", asset(holidays), true, true,
			byRemote{
				"alpha": od(ch(old, "R-LINK-05"), ch(holidays, "R-LINK-05"), dec(a1, unchanged, moved, moveAsset, "R-LINK-05")),
				"beta":  od(ch(gone, "R-LINK-05"), ch(holidays, "R-LINK-05"), dec(b1, unchanged, moved, moveAsset, "R-LINK-05")),
			},
			links(a1Old, b1Gone), config(alpha, beta), quarantine,
			decided{
				"alpha": decs(dec(a1, unchanged, moved, moveAsset, "R-LINK-05")),
				"beta":  decs(dec(b1, unchanged, moved, moveAsset, "R-LINK-05")),
			},
			statuses{1: active, 3: active}, nil, personal, nil, ch(holidays, "R-LINK-05"),
			placed{"alpha": pls(pl(moveFile, a1Old, 0, "R-OBJECT-06")), "beta": pls(pl(moveFile, b1Gone, 0, "R-OBJECT-06"))}},
		{"a Leave of an album in Recently Deleted is dropped", asset(holidays, old), true, true,
			byRemote{"alpha": od(nil, ch(old, "R-OBJECT-02"), kept(a1), dec(inOld, moved, deleted, forget, "R-OBJECT-02"))},
			links(a1), config(alpha), quarantine,
			decided{"alpha": decs(kept(a1), dec(inOld, moved, deleted, forget, "R-OBJECT-02"))}, statuses{1: active, 14: ""},
			ms(m(holidays, "run-1"), m(old, "run-1")), personal, nil, nil, nil},
		{"a remote where the asset is frozen is not laid out", asset(holidays), true, true,
			byRemote{"alpha": od(nil, nil, kept(a1))}, links(a1, stuck), config(alpha, beta), quarantine,
			decided{"alpha": decs(kept(a1))}, statuses{1: active}, ms(m(holidays, "run-1")), personal, nil, nil, nil},
		{"a new asset that only an upload-only remote decides gets no first placement and goes up to its day folder", asset(), true, false,
			byRemote{"beta": od(nil, nil)}, nil, config(alpha, betaUp), quarantine,
			decided{"beta": nil}, nil, nil, personal, nil, nil,
			placed{"beta": pls(up(0, "R-LINK-01"))}},
		{"a new asset in two events that only upload-only remotes decide keeps both and goes up to each", asset(holidays, trip), true, false,
			byRemote{"beta": od(nil, nil)}, nil, config(betaUp), quarantine,
			decided{"beta": nil}, nil, ms(m(holidays, "run-1"), m(trip, "run-1")), personal, nil, nil,
			placed{"beta": pls(up(holidays, "R-LINK-01"), up(trip, "R-LINK-01"))}},
		{"a new asset that only a download-only remote decides still joins the only event of its day", asset(), true, false,
			byRemote{"gamma": od(nil, nil)}, nil, config(gammaDown), quarantine,
			decided{"gamma": nil}, nil, ms(m(holidays, "run-2")), personal, ch(holidays, "R-LAYOUT-05"), nil, nil},
		{"a new asset that no remote decides gets no first placement", asset(), true, false,
			nil, nil, config(alpha), quarantine,
			nil, nil, nil, personal, nil, nil, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			before := byRemote{}
			for name, o := range c.objects {
				before[name] = domain.ObjectDecision{Links: slices.Clone(o.Links), Join: slices.Clone(o.Join), Leave: slices.Clone(o.Leave)}
			}
			ls, all, members, remotes := slices.Clone(c.links), slices.Clone(albums), slices.Clone(c.now.Memberships), slices.Clone(c.remotes)
			priority := slices.Clone(c.p.RemotePriority)
			var got result
			got.decisions, got.status, got.target, got.join, got.leave, got.placements =
				plan.DecideLayout(c.now, c.found, c.linked, c.objects, c.links, c.remotes, albums, c.p, "run-2")
			want := result{c.decisions, c.status, c.now, c.join, c.leave, c.placements}
			want.target.Memberships, want.target.TargetLibrary = c.albums, c.library
			if !equal(got, want) {
				t.Errorf("DecideLayout = %q; want %q", show(got), show(want))
			}
			var rules []string
			for _, ds := range c.decisions {
				for _, d := range ds {
					rules = append(rules, d.Rule)
				}
			}
			for _, x := range slices.Concat(c.join, c.leave) {
				rules = append(rules, x.Rule)
			}
			for _, ps := range c.placements {
				for _, x := range ps {
					rules = append(rules, x.Rule)
				}
			}
			for _, id := range rules {
				if _, ok := domain.Rules[id]; id != "" && !ok {
					t.Errorf("%s is not in domain.Rules", id)
				}
			}
			now, rl, ra := c.now, slices.Clone(c.links), slices.Clone(albums)
			now.Memberships = slices.Clone(c.now.Memberships)
			slices.Reverse(now.Memberships)
			slices.Reverse(rl)
			slices.Reverse(ra)
			var again result
			again.decisions, again.status, again.target, again.join, again.leave, again.placements =
				plan.DecideLayout(now, c.found, c.linked, c.objects, rl, c.remotes, ra, c.p, "run-2")
			if !equal(again, got) {
				t.Errorf("with the links, albums and memberships reversed: %q; want %q", show(again), show(got))
			}
			for i := range got.target.Memberships {
				got.target.Memberships[i].FirstSeenRun = "changed"
			}
			for _, x := range [][]domain.AlbumChange{got.join, got.leave} {
				for i := range x {
					x[i].Rule = "changed"
				}
			}
			if !maps.EqualFunc(c.objects, before, sameObject) || !slices.Equal(c.links, ls) || !reflect.DeepEqual(albums, all) ||
				!slices.Equal(c.now.Memberships, members) || !reflect.DeepEqual(c.remotes, remotes) || !slices.Equal(c.p.RemotePriority, priority) {
				t.Errorf("DecideLayout changed its input or shares a slice with it: objects %v, memberships %v, priority %v",
					c.objects, c.now.Memberships, c.p.RemotePriority)
			}
		})
	}
}
