package plan_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Written from the conflict policies, not from the code: the winner of a
// conflict decides whether the asset follows its file or the file follows
// the asset, the choice of the user stands in for the policy, and ask, skip
// or a policy unknown here leave the decisions as they are. Every row runs
// under each policy.
func TestSettleConflict(t *testing.T) {
	t.Parallel()
	const (
		holidays, trip, party         domain.AlbumID = 10, 11, 12
		unchanged, moved                             = domain.ChangeUnchanged, domain.ChangeMoved
		moveAsset, moveFile, conflict                = domain.ActionMoveAsset, domain.ActionMoveFile, domain.ActionConflict
	)
	type row struct {
		link   domain.LinkID
		action domain.LinkAction
		rule   string
	}
	type change struct {
		album domain.AlbumID
		rule  string
	}
	type outcome struct {
		links       []row
		join, leave []change
	}
	project := func(cs []domain.AlbumChange) []change {
		var out []change
		for _, x := range cs {
			out = append(out, change{x.AlbumID, x.Rule})
		}
		return out
	}
	decided := func(id domain.LinkID, icloud domain.ChangeKind) domain.LinkDecision {
		d, _ := plan.DecideLink(icloud, moved, false)
		d.Link = domain.Link{
			ID: id, AssetID: 7, Remote: "disk", Root: domain.LibraryPersonal, Path: "2024/05-01 - Holidays/IMG_0001.HEIC",
			Role: domain.RoleOriginal, AlbumID: holidays, Status: domain.StatusActive,
		}
		return d
	}
	both := domain.ObjectDecision{
		Links: []domain.LinkDecision{decided(1, unchanged), decided(2, moved)},
		Join:  []domain.AlbumChange{{AlbumID: trip, Rule: "R-LINK-05"}, {AlbumID: party, Rule: "R-LINK-07"}},
		Leave: []domain.AlbumChange{{AlbumID: holidays, Rule: "R-LINK-05"}, {AlbumID: holidays, Rule: "R-LINK-07"}},
	}
	one := domain.ObjectDecision{
		Links: []domain.LinkDecision{decided(1, unchanged)},
		Join:  []domain.AlbumChange{{AlbumID: trip, Rule: "R-LINK-05"}},
		Leave: []domain.AlbumChange{{AlbumID: holidays, Rule: "R-LINK-05"}},
	}
	kept := outcome{
		[]row{{1, moveAsset, "R-LINK-05"}, {2, conflict, "R-LINK-07"}},
		[]change{{trip, "R-LINK-05"}, {party, "R-LINK-07"}},
		[]change{{holidays, "R-LINK-05"}, {holidays, "R-LINK-07"}},
	}
	cloudWins := outcome{[]row{{1, moveAsset, "R-LINK-05"}, {2, moveAsset, "R-LINK-07"}}, kept.join, kept.leave}
	icloudWins := outcome{
		[]row{{1, moveAsset, "R-LINK-05"}, {2, moveFile, "R-LINK-07"}},
		[]change{{trip, "R-LINK-05"}},
		[]change{{holidays, "R-LINK-05"}},
	}
	alone := outcome{[]row{{1, moveAsset, "R-LINK-05"}}, []change{{trip, "R-LINK-05"}}, []change{{holidays, "R-LINK-05"}}}
	all := func(o outcome) [5]outcome { return [5]outcome{o, o, o, o, o} }
	policies := [5]domain.ConflictPolicy{domain.ConflictRemoteWins, domain.ConflictICloudWins, domain.ConflictAsk, domain.ConflictSkip, "keep"}
	for _, c := range []struct {
		name       string
		o          domain.ObjectDecision
		resolution domain.ConflictPolicy
		want       [5]outcome // remote_wins, icloud_wins, ask, skip, unknown
	}{
		{"a conflict beside a move of another file", both, "", [5]outcome{cloudWins, icloudWins, kept, kept, kept}}, // covers: R-LINK-07
		{"no conflict", one, "", all(alone)},
		{"the user chose the cloud", both, domain.ConflictRemoteWins, all(cloudWins)},
		{"the user chose iCloud", both, domain.ConflictICloudWins, all(icloudWins)},
		{"a choice unknown here", both, "keep", all(kept)},
		{"nothing to settle", domain.ObjectDecision{}, "", all(outcome{})},
	} {
		for k, policy := range policies {
			links, join, leave := slices.Clone(c.o.Links), slices.Clone(c.o.Join), slices.Clone(c.o.Leave)
			got := plan.SettleConflict(c.o, c.resolution, policy)
			var rows []row
			for i, d := range got.Links {
				rows = append(rows, row{d.Link.ID, d.Action, d.Rule})
				if i >= len(links) || d.Link != links[i].Link || d.ICloud != links[i].ICloud || d.Remote != links[i].Remote {
					t.Errorf("%s under %q: %+v is not the decision of the input at %d", c.name, policy, d, i)
				}
			}
			w := c.want[k]
			if !slices.Equal(rows, w.links) || !slices.Equal(project(got.Join), w.join) || !slices.Equal(project(got.Leave), w.leave) {
				t.Errorf("%s under %q: SettleConflict = %+v, joins %+v, leaves %+v; want %+v, joins %+v, leaves %+v",
					c.name, policy, rows, project(got.Join), project(got.Leave), w.links, w.join, w.leave)
			}
			if !slices.Equal(c.o.Links, links) || !slices.Equal(c.o.Join, join) || !slices.Equal(c.o.Leave, leave) {
				t.Errorf("%s under %q: SettleConflict changed its input to %+v", c.name, policy, c.o)
			}
			var rules []string
			for _, r := range w.links {
				rules = append(rules, r.rule)
			}
			for _, x := range slices.Concat(w.join, w.leave) {
				rules = append(rules, x.rule)
			}
			for _, id := range rules {
				if _, ok := domain.Rules[id]; !ok {
					t.Errorf("%s is not in domain.Rules", id)
				}
			}
		}
	}
}

// Every decision that a mode and the deletes policy leave, with and without
// the same move on both sides, is settled under every conflict policy: a
// winner leaves no conflict, the asset following its file under remote_wins
// and the file following the asset under icloud_wins, and nothing but a
// conflict changes.
func TestSettleConflictTotal(t *testing.T) {
	t.Parallel()
	winners := map[domain.ConflictPolicy]domain.LinkAction{
		domain.ConflictRemoteWins: domain.ActionMoveAsset,
		domain.ConflictICloudWins: domain.ActionMoveFile,
	}
	cells, wrong, first := 0, 0, ""
	for _, m := range []domain.Mode{domain.ModeTwoWay, domain.ModeUploadOnly, domain.ModeDownloadOnly} {
		for _, deletes := range []domain.DeletesPolicy{domain.DeletesNone, domain.DeletesQuarantine, domain.DeletesPropagate} {
			for _, icloud := range changeKinds {
				for _, remote := range changeKinds {
					for _, same := range []bool{false, true} {
						d, ok := plan.ModeDecide(m, icloud, remote, same)
						if !ok {
							continue
						}
						d, _ = plan.StatusDeletes(d, deletes)
						for _, policy := range []domain.ConflictPolicy{domain.ConflictRemoteWins, domain.ConflictICloudWins, domain.ConflictAsk, domain.ConflictSkip, "keep"} {
							cells++
							want := d
							if w, won := winners[policy]; won && d.Action == domain.ActionConflict {
								want.Action = w
							}
							got := plan.SettleConflict(domain.ObjectDecision{Links: []domain.LinkDecision{d}}, "", policy)
							if len(got.Links) != 1 || got.Links[0] != want {
								wrong++
								if first == "" {
									first = fmt.Sprintf("%s, %s, iCloud %s, cloud %s, same %v, %s: %+v; want %+v", m, deletes, icloud, remote, same, policy, got.Links, want)
								}
							}
						}
					}
				}
			}
		}
	}
	if cells != 1350 || wrong != 0 {
		t.Errorf("settled %d cells, %d of them wrong; want 1350 and none: each of the 15 possible pairs, with and without same, in 3 modes under 3 deletes policies and 5 conflict policies; the first wrong: %s", cells, wrong, first)
	}
}

// Each row changes the albums or the library of the asset from one remote or
// several, as their settled decisions say, or places it for the first time.
// The expected target and album changes are written from the rules on
// albums, libraries and the priority of remotes, not from the code, and must
// not depend on the order of the memberships and links.
func TestDecideTarget(t *testing.T) {
	t.Parallel()
	const (
		holidays, trip, party domain.AlbumID = 10, 11, 12
		personal, shared                     = domain.LibraryPersonal, domain.LibraryShared
		unchanged, moved                     = domain.ChangeUnchanged, domain.ChangeMoved
	)
	asset := func(library, target domain.LibraryKind, ids ...domain.AlbumID) domain.Asset {
		a := domain.Asset{
			ID: 7, Key: domain.AssetKey{CloudAssetGUID: "guid-7", UUID: "uuid-7"},
			Library: library, TargetLibrary: target, OriginalName: "IMG_0001.HEIC", Width: 4032, Height: 3024,
		}
		for _, id := range ids {
			a.Memberships = append(a.Memberships, domain.Membership{AlbumID: id, FirstSeenRun: "run-1"})
		}
		return a
	}
	in := func(ids ...domain.AlbumID) domain.Asset { return asset(personal, personal, ids...) }
	file := func(id domain.LinkID, remote string, root domain.LibraryKind, role domain.LinkRole) domain.Link {
		return domain.Link{
			ID: id, AssetID: 7, Remote: remote, Root: root, Path: "2024/05-01 - Holidays/IMG_0001.HEIC",
			Role: role, AlbumID: holidays, Status: domain.StatusActive,
		}
	}
	at := func(l domain.Link, root domain.LibraryKind, album domain.AlbumID) domain.Link {
		l.Root, l.AlbumID = root, album
		return l
	}
	decision := func(l domain.Link, icloud domain.ChangeKind, a domain.LinkAction, rule string) domain.LinkDecision {
		return domain.LinkDecision{Link: l, ICloud: icloud, Remote: moved, Action: a, Rule: rule}
	}
	toAsset := func(l domain.Link) domain.LinkDecision {
		return decision(l, unchanged, domain.ActionMoveAsset, "R-LINK-05")
	}
	ch := func(id domain.AlbumID, rule string) domain.AlbumChange {
		return domain.AlbumChange{AlbumID: id, Rule: rule}
	}
	changes := func(c ...domain.AlbumChange) []domain.AlbumChange { return c }
	od := func(join, leave []domain.AlbumChange, d ...domain.LinkDecision) domain.ObjectDecision {
		return domain.ObjectDecision{Links: d, Join: join, Leave: leave}
	}
	e1 := file(1, "disk", personal, domain.RoleOriginal)
	e2 := file(2, "disk", shared, domain.RoleOriginal)
	live := file(4, "disk", personal, domain.RoleLiveVideo)
	a1 := file(5, "alpha", personal, domain.RoleOriginal)
	b1 := file(6, "beta", personal, domain.RoleOriginal)
	b2 := file(8, "beta", shared, domain.RoleOriginal)

	type member struct {
		album domain.AlbumID
		run   string
	}
	type change struct {
		album domain.AlbumID
		rule  string
	}
	project := func(cs []domain.AlbumChange) []change {
		var out []change
		for _, x := range cs {
			out = append(out, change{x.AlbumID, x.Rule})
		}
		return out
	}
	for _, c := range []struct {
		name        string
		now         domain.Asset
		first       domain.ObjectDecision
		objects     map[string]domain.ObjectDecision
		links       []domain.Link
		priority    []string
		albums      []member
		library     domain.LibraryKind
		join, leave []change
	}{
		{"nothing changes", in(holidays), domain.ObjectDecision{},
			map[string]domain.ObjectDecision{"disk": od(nil, nil)}, []domain.Link{e1}, nil,
			[]member{{holidays, "run-1"}}, personal, nil, nil},
		{"a remote moves the asset to another event", in(holidays), domain.ObjectDecision{},
			map[string]domain.ObjectDecision{"disk": od(changes(ch(trip, "R-LINK-05")), changes(ch(holidays, "R-LINK-05")), toAsset(e1))},
			[]domain.Link{at(e1, personal, trip)}, nil,
			[]member{{trip, "run-2"}}, personal, []change{{trip, "R-LINK-05"}}, []change{{holidays, "R-LINK-05"}}},
		{"the asset is already in the event it joins: only the leave is a change", in(holidays, trip), domain.ObjectDecision{},
			map[string]domain.ObjectDecision{"disk": od(changes(ch(trip, "R-LINK-05")), changes(ch(holidays, "R-LINK-05")), toAsset(e1))},
			[]domain.Link{at(e1, personal, trip)}, nil,
			[]member{{trip, "run-1"}}, personal, nil, []change{{holidays, "R-LINK-05"}}},
		{"a conflict won by the cloud: the asset joins the event of its file and keeps the one it moved to in iCloud", in(trip), domain.ObjectDecision{},
			map[string]domain.ObjectDecision{"disk": od(changes(ch(party, "R-LINK-07")), changes(ch(holidays, "R-LINK-07")),
				decision(e1, moved, domain.ActionMoveAsset, "R-LINK-07"))},
			[]domain.Link{at(e1, personal, party)}, nil,
			[]member{{trip, "run-1"}, {party, "run-2"}}, personal, []change{{party, "R-LINK-07"}}, nil},
		{"two remotes move the asset to two events: it joins both", in(holidays), domain.ObjectDecision{},
			map[string]domain.ObjectDecision{
				"alpha": od(changes(ch(trip, "R-LINK-05")), changes(ch(holidays, "R-LINK-05"))),
				"beta":  od(changes(ch(party, "R-LINK-05")), changes(ch(holidays, "R-OBJECT-02"))),
			}, nil, []string{"alpha", "beta"},
			[]member{{trip, "run-2"}, {party, "run-2"}}, personal, []change{{trip, "R-LINK-05"}, {party, "R-LINK-05"}}, []change{{holidays, "R-LINK-05"}}},
		{"remotes that disagree on an album: the first in priority decides it", in(holidays, party), domain.ObjectDecision{},
			map[string]domain.ObjectDecision{
				"alpha": od(changes(ch(trip, "R-LINK-05")), changes(ch(holidays, "R-LINK-05"))),
				"beta":  od(changes(ch(holidays, "R-LINK-05")), changes(ch(party, "R-LINK-05"))),
			}, nil, []string{"beta", "alpha"},
			[]member{{holidays, "run-1"}, {trip, "run-2"}}, personal, []change{{trip, "R-LINK-05"}}, []change{{party, "R-LINK-05"}}},
		{"the remotes that the priority does not name come after it in the order of their names", in(holidays), domain.ObjectDecision{},
			map[string]domain.ObjectDecision{
				"gamma": od(changes(ch(party, "R-LINK-07")), changes(ch(holidays, "R-LINK-07"))),
				"alpha": od(changes(ch(holidays, "R-LINK-05"), ch(trip, "R-LINK-05")), nil),
				"beta":  od(nil, changes(ch(trip, "R-OBJECT-02"))),
			}, nil, []string{"gamma", "absent"},
			[]member{{trip, "run-2"}, {party, "run-2"}}, personal, []change{{trip, "R-LINK-05"}, {party, "R-LINK-07"}}, []change{{holidays, "R-LINK-07"}}},
		{"the first placement joins the only event of the day", in(), od(changes(ch(holidays, "R-LAYOUT-05")), nil),
			map[string]domain.ObjectDecision{"disk": od(nil, nil)}, nil, nil,
			[]member{{holidays, "run-2"}}, personal, []change{{holidays, "R-LAYOUT-05"}}, nil},
		{"the first placement goes before every remote", in(holidays, trip), od(nil, changes(ch(trip, "R-LAYOUT-04"))),
			map[string]domain.ObjectDecision{"disk": od(changes(ch(trip, "R-LINK-05")), nil)}, nil, nil,
			[]member{{holidays, "run-1"}}, personal, nil, []change{{trip, "R-LAYOUT-04"}}},
		{"a file moved into the other root: the library of that root", in(holidays), domain.ObjectDecision{},
			map[string]domain.ObjectDecision{"disk": od(nil, nil, toAsset(e1))}, []domain.Link{at(e1, shared, holidays)}, nil,
			[]member{{holidays, "run-1"}}, shared, nil, nil},
		{"a Live Photo video moved into the other root: the library stays", in(holidays), domain.ObjectDecision{},
			map[string]domain.ObjectDecision{"disk": od(nil, nil, toAsset(live))}, []domain.Link{e1, at(live, shared, holidays)}, nil,
			[]member{{holidays, "run-1"}}, personal, nil, nil},
		{"a file moved back into the library of an asset waiting for a move", asset(shared, personal, holidays), domain.ObjectDecision{},
			map[string]domain.ObjectDecision{"disk": od(nil, nil, toAsset(e1))}, []domain.Link{at(e1, shared, holidays)}, nil,
			[]member{{holidays, "run-1"}}, shared, nil, nil},
		{"a conflict won by iCloud: the library stays", in(trip), domain.ObjectDecision{},
			map[string]domain.ObjectDecision{"disk": od(nil, nil, decision(e1, moved, domain.ActionMoveFile, "R-LINK-07"))},
			[]domain.Link{at(e1, shared, holidays)}, nil,
			[]member{{trip, "run-1"}}, personal, nil, nil},
		{"a later remote changes the library that the first keeps", in(holidays), domain.ObjectDecision{},
			map[string]domain.ObjectDecision{
				"alpha": od(changes(ch(trip, "R-LINK-05")), changes(ch(holidays, "R-LINK-05")), toAsset(a1)),
				"beta":  od(nil, nil, toAsset(b1)),
			}, []domain.Link{at(a1, personal, trip), at(b1, shared, holidays)}, []string{"alpha", "beta"},
			[]member{{trip, "run-2"}}, shared, []change{{trip, "R-LINK-05"}}, []change{{holidays, "R-LINK-05"}}},
		{"two remotes move files into the two roots: the first in priority decides", in(holidays), domain.ObjectDecision{},
			map[string]domain.ObjectDecision{"alpha": od(nil, nil, toAsset(a1)), "beta": od(nil, nil, toAsset(b2))},
			[]domain.Link{at(a1, shared, holidays), at(b2, personal, holidays)}, []string{"beta", "alpha"},
			[]member{{holidays, "run-1"}}, personal, nil, nil},
		{"two files moved into the two roots in one remote: the first by link ID decides", in(holidays), domain.ObjectDecision{},
			map[string]domain.ObjectDecision{"disk": od(nil, nil, toAsset(e1), toAsset(e2))},
			[]domain.Link{at(e1, shared, holidays), at(e2, personal, holidays)}, nil,
			[]member{{holidays, "run-1"}}, shared, nil, nil},
		{"no remote: the asset as it is, its albums in order", in(trip, holidays), domain.ObjectDecision{}, nil, nil, nil,
			[]member{{holidays, "run-1"}, {trip, "run-1"}}, personal, nil, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			memberships, links := slices.Clone(c.now.Memberships), slices.Clone(c.links)
			got, join, leave := plan.DecideTarget(c.now, c.first, c.objects, c.links, c.priority, "run-2")
			var albums []member
			for _, m := range got.Memberships {
				albums = append(albums, member{m.AlbumID, m.FirstSeenRun})
			}
			rest := got
			rest.Memberships, rest.TargetLibrary = c.now.Memberships, c.now.TargetLibrary
			if !slices.Equal(albums, c.albums) || got.TargetLibrary != c.library || !slices.Equal(project(join), c.join) ||
				!slices.Equal(project(leave), c.leave) || !reflect.DeepEqual(rest, c.now) {
				t.Errorf("DecideTarget = %+v in %q, %+v of %q, joins %+v, leaves %+v; want %+v in %q, joins %+v, leaves %+v",
					albums, got.TargetLibrary, got.Key, got.Library, project(join), project(leave), c.albums, c.library, c.join, c.leave)
			}
			if !slices.Equal(c.now.Memberships, memberships) || !slices.Equal(c.links, links) {
				t.Errorf("DecideTarget changed its input to %+v and %+v", c.now.Memberships, c.links)
			}
			for _, x := range slices.Concat(c.join, c.leave) {
				if _, ok := domain.Rules[x.rule]; !ok {
					t.Errorf("%s is not in domain.Rules", x.rule)
				}
			}
			reversed := c.now
			reversed.Memberships = slices.Clone(memberships)
			slices.Reverse(reversed.Memberships)
			slices.Reverse(links)
			again, j, l := plan.DecideTarget(reversed, c.first, c.objects, links, c.priority, "run-2")
			if !slices.Equal(again.Memberships, got.Memberships) || again.TargetLibrary != got.TargetLibrary || !slices.Equal(j, join) || !slices.Equal(l, leave) {
				t.Errorf("with the memberships and links reversed: %+v in %q, %+v, %+v; want %+v in %q, %+v, %+v",
					again.Memberships, again.TargetLibrary, j, l, got.Memberships, got.TargetLibrary, join, leave)
			}
		})
	}
}

// The target is the asset by which every remote lays out its files, so a
// change that one remote makes reaches the others in the same run, each as
// its mode lets it, and the winner of a conflict decides where the file ends
// up. The decisions are built as a run builds them; the placements are
// written from the layout rules, not from the code.
func TestDecideTargetLayout(t *testing.T) {
	t.Parallel()
	const (
		holidays, trip, party            domain.AlbumID = 10, 11, 12
		personal, shared                                = domain.LibraryPersonal, domain.LibraryShared
		unchanged, moved                                = domain.ChangeUnchanged, domain.ChangeMoved
		twoWay, uploadOnly, downloadOnly                = domain.ModeTwoWay, domain.ModeUploadOnly, domain.ModeDownloadOnly
		atHolidays                                      = "2024/05-01 - Holidays/IMG_0001.HEIC"
		atTrip, atParty                                 = "2024/05-02 - Trip/IMG_0001.HEIC", "2024/05-03 - Party/IMG_0001.HEIC"
	)
	albums := []domain.Album{
		{ID: 1, UUID: "album-1", Kind: domain.KindFolder, Title: "2024"},
		{ID: holidays, UUID: "album-10", Title: "05/01 Holidays", ParentID: 1, FolderTitles: []string{"2024"}},
		{ID: trip, UUID: "album-11", Title: "05/02 Trip", ParentID: 1, FolderTitles: []string{"2024"}},
		{ID: party, UUID: "album-12", Title: "05/03 Party", ParentID: 1, FolderTitles: []string{"2024"}},
	}
	in := func(ids ...domain.AlbumID) domain.Asset {
		a := domain.Asset{
			ID: 7, Key: domain.AssetKey{CloudAssetGUID: "guid-7", UUID: "uuid-7"},
			Library: personal, TargetLibrary: personal, OriginalName: "IMG_0001.HEIC",
		}
		for _, id := range ids {
			a.Memberships = append(a.Memberships, domain.Membership{AlbumID: id, FirstSeenRun: "run-1"})
		}
		return a
	}
	file := func(id domain.LinkID, remote string) domain.Link {
		return domain.Link{
			ID: id, AssetID: 7, Remote: remote, Root: personal, Path: atHolidays,
			Role: domain.RoleOriginal, AlbumID: holidays, Status: domain.StatusActive,
		}
	}
	at := func(l domain.Link, root domain.LibraryKind, album domain.AlbumID, p string) domain.Link {
		l.Root, l.AlbumID, l.Path = root, album, p
		return l
	}
	type side struct {
		remote        string
		mode          domain.Mode
		base, now     domain.Link
		icloud, cloud domain.ChangeKind
	}
	type place struct {
		action domain.LinkAction
		link   domain.LinkID
		album  domain.AlbumID
		rule   string
	}
	for _, c := range []struct {
		name   string
		now    domain.Asset
		policy domain.ConflictPolicy
		sides  []side
		want   map[string][]place
	}{
		{"a move in one remote reaches the others as their modes let it", in(holidays), domain.ConflictRemoteWins, []side{
			{"alpha", twoWay, file(1, "alpha"), at(file(1, "alpha"), personal, trip, atTrip), unchanged, moved},
			{"beta", uploadOnly, file(2, "beta"), file(2, "beta"), unchanged, unchanged},
			{"gamma", downloadOnly, file(3, "gamma"), file(3, "gamma"), unchanged, unchanged},
		}, map[string][]place{"beta": {{domain.ActionMoveFile, 2, trip, "R-OBJECT-05"}}}},
		{"a conflict won by the cloud copies the file into the event the asset moved to in iCloud", in(trip), domain.ConflictRemoteWins, []side{ // covers: R-LINK-07
			{"disk", twoWay, file(1, "disk"), at(file(1, "disk"), personal, party, atParty), moved, moved},
		}, map[string][]place{"disk": {{domain.ActionCopyFile, 1, trip, "R-OBJECT-03"}}}},
		{"a conflict won by iCloud moves the file into the event of the asset", in(trip), domain.ConflictICloudWins, []side{
			{"disk", twoWay, file(1, "disk"), at(file(1, "disk"), personal, party, atParty), moved, moved},
		}, map[string][]place{"disk": {{domain.ActionMoveFile, 1, trip, "R-OBJECT-05"}}}},
		{"a file moved into the other root takes the files of the other remotes there", in(holidays), domain.ConflictRemoteWins, []side{ // covers: R-OBJECT-07
			{"alpha", twoWay, file(1, "alpha"), at(file(1, "alpha"), shared, holidays, atHolidays), unchanged, moved},
			{"beta", twoWay, file(2, "beta"), file(2, "beta"), unchanged, unchanged},
		}, map[string][]place{"beta": {{domain.ActionMoveFile, 2, holidays, "R-OBJECT-07"}}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var links []domain.Link
			for _, s := range c.sides {
				links = append(links, s.now)
			}
			objects := map[string]domain.ObjectDecision{}
			for _, s := range c.sides {
				d, _ := plan.ModeDecide(s.mode, s.icloud, s.cloud, false)
				d.Link = s.base
				o := domain.ObjectDecision{Links: []domain.LinkDecision{d}}
				if s.mode != uploadOnly {
					o = plan.DecideObject(c.now, o.Links, links, s.remote, albums)
				}
				objects[s.remote] = plan.SettleConflict(o, "", c.policy)
			}
			target, _, _ := plan.DecideTarget(c.now, domain.ObjectDecision{}, objects, links, nil, "run-2")
			for _, s := range c.sides {
				var got []place
				for _, p := range plan.DecidePlacement(target, links, s.remote, albums) {
					if plan.ModeAllows(s.mode, p.Action) {
						got = append(got, place{p.Action, p.Link.ID, p.AlbumID, p.Rule})
					}
				}
				if !slices.Equal(got, c.want[s.remote]) {
					t.Errorf("%s lays out %+v; want %+v", s.remote, got, c.want[s.remote])
				}
				for _, p := range c.want[s.remote] {
					if _, ok := domain.Rules[p.rule]; !ok {
						t.Errorf("%s is not in domain.Rules", p.rule)
					}
				}
			}
		})
	}
}
