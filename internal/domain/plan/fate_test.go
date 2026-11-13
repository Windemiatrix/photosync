package plan_test

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Written from the rules on deletions and replacements, not from the code: a
// change of the asset in any remote beats a deletion, a deletion that stands
// reaches every remote through iCloud in the same run unless deletes is none,
// and of several replacements of the asset only the first in priority is
// made. Every row runs under each deletes policy, an unknown one counting as
// quarantine.
func TestDecideFate(t *testing.T) {
	t.Parallel()
	const (
		unchanged, moved, replaced       = domain.ChangeUnchanged, domain.ChangeMoved, domain.ChangeReplaced
		deleted, absent, added           = domain.ChangeDeleted, domain.ChangeNone, domain.ChangeNew
		twoWay, uploadOnly, downloadOnly = domain.ModeTwoWay, domain.ModeUploadOnly, domain.ModeDownloadOnly
		nothing, upload, importFile      = domain.ActionNone, domain.ActionUpload, domain.ActionImport
		deleteAsset, deleteFile, forget  = domain.ActionDeleteAsset, domain.ActionDeleteFile, domain.ActionForget
		moveAsset, moveFile, updateBase  = domain.ActionMoveAsset, domain.ActionMoveFile, domain.ActionUpdateBase
		replace, moveReplace             = domain.ActionReplaceAsset, domain.ActionMoveFileReplaceAsset
	)
	type objects = map[string]domain.ObjectDecision
	file := func(id domain.LinkID, remote string) domain.Link {
		return domain.Link{
			ID: id, AssetID: 7, Remote: remote, Root: domain.LibraryPersonal, Path: "2024/05-01 - Holidays/IMG_0001.HEIC",
			Role: domain.RoleOriginal, AlbumID: 10, Status: domain.StatusActive,
		}
	}
	d1, d2 := file(1, "disk"), file(2, "disk")
	d2.AlbumID, d2.Path = 11, "2024/05-02 - Trip/IMG_0001.HEIC"
	a1, b1, g1, restored := file(3, "alpha"), file(4, "beta"), file(5, "gamma"), file(6, "beta")
	restored.Status = domain.StatusQuarantinedICloud
	decided := func(m domain.Mode, l domain.Link, icloud, remote domain.ChangeKind) domain.LinkDecision {
		d, _ := plan.ModeDecide(m, icloud, remote, false)
		d.Link = l
		return d
	}
	is := func(l domain.Link, icloud, remote domain.ChangeKind, a domain.LinkAction, rule string) domain.LinkDecision {
		return domain.LinkDecision{Link: l, ICloud: icloud, Remote: remote, Action: a, Rule: rule}
	}
	od := func(d ...domain.LinkDecision) domain.ObjectDecision { return domain.ObjectDecision{Links: d} }
	albums := func(o domain.ObjectDecision) domain.ObjectDecision {
		o.Join = []domain.AlbumChange{{AlbumID: 11, Rule: "R-LINK-05"}}
		o.Leave = []domain.AlbumChange{{AlbumID: 10, Rule: "R-LINK-05"}}
		return o
	}
	both := func(o objects) [2]objects { return [2]objects{o, o} }
	all := map[string]domain.Mode{"disk": twoWay, "alpha": twoWay, "beta": twoWay, "gamma": twoWay}
	but := func(name string, m domain.Mode) map[string]domain.Mode {
		modes := maps.Clone(all)
		modes[name] = m
		return modes
	}
	gone, goneDown := od(decided(twoWay, a1, unchanged, deleted)), od(decided(downloadOnly, a1, unchanged, deleted))
	moving := albums(od(decided(twoWay, b1, unchanged, moved)))
	deleting, standing := od(is(a1, unchanged, deleted, deleteAsset, "R-LINK-09")), od(is(a1, moved, deleted, deleteAsset, "R-LINK-09"))
	uploaded, asMoving := od(is(a1, moved, deleted, upload, "R-LINK-10")), albums(od(is(b1, unchanged, moved, moveAsset, "R-LINK-05")))
	same := func(x, y domain.ObjectDecision) bool {
		return slices.Equal(x.Links, y.Links) && slices.Equal(x.Join, y.Join) && slices.Equal(x.Leave, y.Leave)
	}
	show := func(fate objects) map[string][]string {
		out := map[string][]string{}
		for name, o := range fate {
			for _, d := range o.Links {
				out[name] = append(out[name], fmt.Sprintf("%d %s/%s %s %s", d.Link.ID, d.ICloud, d.Remote, d.Action, d.Rule))
			}
			out[name] = append(out[name], fmt.Sprintf("joins %v leaves %v", o.Join, o.Leave))
		}
		return out
	}
	for _, c := range []struct {
		name     string
		modes    map[string]domain.Mode
		priority []string
		in       objects
		want     [2]objects // under none, under any other policy
	}{
		{"every file gone and the asset changed in iCloud: the change beats the deletion", all, nil, // covers: R-LINK-10
			objects{"disk": od(decided(twoWay, d1, unchanged, deleted), decided(twoWay, d2, moved, deleted))},
			both(objects{"disk": od(is(d1, moved, deleted, upload, "R-LINK-10"), is(d2, moved, deleted, upload, "R-LINK-10"))})},
		{"every file gone and nothing changed: the deletion stands", all, nil, // covers: R-OBJECT-01
			objects{"disk": od(decided(twoWay, d1, unchanged, deleted))},
			both(objects{"disk": od(is(d1, unchanged, deleted, deleteAsset, "R-LINK-09"))})},
		{"a deletion reaches the other remotes through iCloud", but("gamma", downloadOnly), nil, // covers: R-LINK-08
			objects{"alpha": gone, "beta": od(decided(twoWay, b1, unchanged, unchanged)), "gamma": od(decided(downloadOnly, g1, unchanged, unchanged))},
			[2]objects{
				{"alpha": deleting, "beta": od(is(b1, unchanged, unchanged, nothing, "")), "gamma": od(is(g1, unchanged, unchanged, nothing, ""))},
				{"alpha": deleting, "beta": od(is(b1, deleted, unchanged, deleteFile, "R-LINK-08")), "gamma": od(is(g1, deleted, unchanged, nothing, "R-MODE-02"))},
			}},
		{"a file moved in another remote beats the deletion", all, nil,
			objects{"alpha": gone, "beta": moving},
			both(objects{"alpha": uploaded, "beta": asMoving})},
		{"new content in another remote beats the deletion", all, nil,
			objects{"alpha": gone, "beta": od(decided(twoWay, b1, unchanged, replaced))},
			both(objects{"alpha": uploaded, "beta": od(is(b1, unchanged, replaced, replace, "R-LINK-06"))})},
		{"an asset moved in iCloud beats the deletion", all, nil,
			objects{"alpha": gone, "beta": od(decided(twoWay, b1, moved, unchanged))},
			both(objects{"alpha": uploaded, "beta": od(is(b1, moved, unchanged, moveFile, "R-LINK-04"))})},
		{"an asset brought back from Recently Deleted beats the deletion", all, nil,
			objects{"alpha": gone, "beta": od(decided(twoWay, restored, added, absent))},
			both(objects{"alpha": uploaded, "beta": od(is(restored, added, absent, upload, "R-LINK-01"))})},
		{"a download-only remote does not see the change: its deletion stands and reaches the others", but("alpha", downloadOnly), nil, // covers: R-LINK-11
			objects{"alpha": goneDown, "beta": moving, "gamma": od(decided(twoWay, g1, unchanged, unchanged))},
			[2]objects{
				{"alpha": standing, "beta": asMoving, "gamma": od(is(g1, unchanged, unchanged, nothing, ""))},
				{"alpha": standing, "beta": albums(od(is(b1, deleted, moved, importFile, "R-LINK-11"))), "gamma": od(is(g1, deleted, unchanged, deleteFile, "R-LINK-08"))},
			}},
		{"a move that an upload-only remote takes into the base is no change", but("beta", uploadOnly), nil,
			objects{"alpha": gone, "beta": od(decided(uploadOnly, b1, unchanged, moved))},
			[2]objects{
				{"alpha": deleting, "beta": od(is(b1, unchanged, moved, updateBase, "R-MODE-01"))},
				{"alpha": deleting, "beta": od(is(b1, deleted, moved, deleteFile, "R-LINK-08"))},
			}},
		{"a move under the personal root that changes no album is no change", all, nil,
			objects{"alpha": gone, "beta": od(is(b1, unchanged, moved, updateBase, "R-MODE-04"))},
			[2]objects{
				{"alpha": deleting, "beta": od(is(b1, unchanged, moved, updateBase, "R-MODE-04"))},
				{"alpha": deleting, "beta": od(is(b1, deleted, moved, deleteFile, "R-LINK-08"))},
			}},
		{"every file gone in two remotes: both deletions stand", all, nil,
			objects{"alpha": gone, "beta": od(decided(twoWay, b1, unchanged, deleted)), "gamma": od(decided(twoWay, g1, unchanged, unchanged))},
			[2]objects{
				{"alpha": deleting, "beta": od(is(b1, unchanged, deleted, deleteAsset, "R-LINK-09")), "gamma": od(is(g1, unchanged, unchanged, nothing, ""))},
				{"alpha": deleting, "beta": od(is(b1, unchanged, deleted, deleteAsset, "R-LINK-09")), "gamma": od(is(g1, deleted, unchanged, deleteFile, "R-LINK-08"))},
			}},
		{"a deletion beaten in a two-way remote stands in a download-only one", but("beta", downloadOnly), nil,
			objects{"alpha": gone, "beta": od(decided(downloadOnly, b1, unchanged, deleted)), "gamma": od(decided(twoWay, g1, moved, unchanged))},
			[2]objects{
				{"alpha": uploaded, "beta": od(is(b1, moved, deleted, deleteAsset, "R-LINK-09")), "gamma": od(is(g1, moved, unchanged, moveFile, "R-LINK-04"))},
				{"alpha": od(is(a1, deleted, deleted, forget, "R-LINK-12")), "beta": od(is(b1, moved, deleted, deleteAsset, "R-LINK-09")), "gamma": od(is(g1, deleted, unchanged, deleteFile, "R-LINK-08"))},
			}},
		{"a link back from quarantine keeps its decision while the deletion stands", but("alpha", downloadOnly), nil,
			objects{"alpha": goneDown, "beta": od(decided(twoWay, restored, added, absent))},
			both(objects{"alpha": standing, "beta": od(is(restored, added, absent, upload, "R-LINK-01"))})},
		{"two remotes replace the asset: the first in priority, the other waits", all, []string{"beta", "alpha"},
			objects{"alpha": od(decided(twoWay, a1, unchanged, replaced)), "beta": od(decided(twoWay, b1, unchanged, replaced))},
			both(objects{"alpha": od(is(a1, unchanged, replaced, nothing, "")), "beta": od(is(b1, unchanged, replaced, replace, "R-LINK-06"))})},
		{"the remotes that the priority does not name come after it in the order of their names", all, []string{"gamma", "delta"},
			objects{
				"alpha": od(decided(twoWay, a1, unchanged, replaced)), "beta": od(decided(twoWay, b1, unchanged, replaced)),
				"gamma": od(decided(twoWay, g1, unchanged, replaced)),
			},
			both(objects{
				"alpha": od(is(a1, unchanged, replaced, nothing, "")), "beta": od(is(b1, unchanged, replaced, nothing, "")),
				"gamma": od(is(g1, unchanged, replaced, replace, "R-LINK-06")),
			})},
		{"a replacement with a move in iCloud, first by name, stands", all, nil,
			objects{"alpha": od(decided(twoWay, a1, moved, replaced)), "beta": od(decided(twoWay, b1, unchanged, replaced))},
			both(objects{"alpha": od(is(a1, moved, replaced, moveReplace, "R-LINK-14")), "beta": od(is(b1, unchanged, replaced, nothing, ""))})},
		{"a replacement with a move in iCloud waits behind the first in priority", all, []string{"beta"},
			objects{"alpha": od(decided(twoWay, a1, moved, replaced)), "beta": od(decided(twoWay, b1, unchanged, replaced))},
			both(objects{"alpha": od(is(a1, moved, replaced, nothing, "")), "beta": od(is(b1, unchanged, replaced, replace, "R-LINK-06"))})},
		{"two files replaced in one remote: the first by link ID", all, nil,
			objects{"disk": od(decided(twoWay, d1, unchanged, replaced), decided(twoWay, d2, unchanged, replaced))},
			both(objects{"disk": od(is(d1, unchanged, replaced, replace, "R-LINK-06"), is(d2, unchanged, replaced, nothing, ""))})},
		{"replacements while a deletion stands: each replaced file is imported", but("alpha", downloadOnly), nil,
			objects{"alpha": goneDown, "beta": od(decided(twoWay, b1, unchanged, replaced)), "gamma": od(decided(twoWay, g1, unchanged, replaced))},
			[2]objects{
				{"alpha": standing, "beta": od(is(b1, unchanged, replaced, replace, "R-LINK-06")), "gamma": od(is(g1, unchanged, replaced, nothing, ""))},
				{"alpha": standing, "beta": od(is(b1, deleted, replaced, importFile, "R-LINK-11")), "gamma": od(is(g1, deleted, replaced, importFile, "R-LINK-11"))},
			}},
		{"nothing deleted or replaced: the decisions and albums as given", but("gamma", downloadOnly), nil,
			objects{"alpha": od(decided(twoWay, a1, moved, unchanged)), "beta": moving, "gamma": od(decided(downloadOnly, g1, unchanged, unchanged))},
			both(objects{"alpha": od(is(a1, moved, unchanged, moveFile, "R-LINK-04")), "beta": asMoving, "gamma": od(is(g1, unchanged, unchanged, nothing, ""))})},
		{"no remote", all, nil, nil, both(objects{})},
		{"no remote in an empty map", all, nil, objects{}, both(objects{})},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			for _, deletes := range []domain.DeletesPolicy{domain.DeletesNone, domain.DeletesQuarantine, domain.DeletesPropagate, "trash"} {
				before := objects{}
				for name, o := range c.in {
					before[name] = domain.ObjectDecision{Links: slices.Clone(o.Links), Join: slices.Clone(o.Join), Leave: slices.Clone(o.Leave)}
				}
				got, want := plan.DecideFate(c.in, c.modes, c.priority, deletes), c.want[1]
				if deletes == domain.DeletesNone {
					want = c.want[0]
				}
				if !maps.EqualFunc(got, want, same) {
					t.Errorf("under %q: DecideFate = %q; want %q", deletes, show(got), show(want))
				}
				if !maps.EqualFunc(c.in, before, same) {
					t.Errorf("under %q: DecideFate changed its input to %q", deletes, show(c.in))
				}
				for _, o := range want {
					for _, d := range o.Links {
						if _, ok := domain.Rules[d.Rule]; d.Rule != "" && !ok {
							t.Errorf("%s is not in domain.Rules", d.Rule)
						}
					}
				}
			}
		})
	}
}

// Every decision that a mode gives the file of an asset found outside
// Recently Deleted, in each of two remotes, under every deletes policy: each
// remote keeps one decision on its link and file that its mode runs under a
// known rule, a change of the asset leaves no deletion in a two-way remote,
// at most one replacement is made, a deletion that stands reaches the other
// remote unless deletes is none, and nothing else changes a decision.
func TestDecideFateTotal(t *testing.T) {
	t.Parallel()
	type side struct {
		mode           domain.Mode
		icloud, remote domain.ChangeKind
	}
	var sides []side
	for _, m := range []domain.Mode{domain.ModeTwoWay, domain.ModeUploadOnly, domain.ModeDownloadOnly} {
		for _, icloud := range []domain.ChangeKind{domain.ChangeUnchanged, domain.ChangeMoved} {
			for _, remote := range []domain.ChangeKind{domain.ChangeUnchanged, domain.ChangeMoved, domain.ChangeReplaced, domain.ChangeDeleted} {
				sides = append(sides, side{m, icloud, remote})
			}
		}
	}
	decide := func(s side, id domain.LinkID, remote string) domain.LinkDecision {
		d, _ := plan.ModeDecide(s.mode, s.icloud, s.remote, false)
		d.Link = domain.Link{ID: id, AssetID: 7, Remote: remote, Role: domain.RoleOriginal, Status: domain.StatusActive}
		return d
	}
	changes := func(d domain.LinkDecision) bool {
		return d.ICloud == domain.ChangeMoved || d.Action == domain.ActionMoveAsset || d.Action == domain.ActionReplaceAsset
	}
	replaces := func(d domain.LinkDecision) bool {
		return d.Action == domain.ActionReplaceAsset || d.Action == domain.ActionMoveFileReplaceAsset
	}
	cells, wrong, first := 0, 0, ""
	for _, a := range sides {
		for _, b := range sides {
			for _, deletes := range []domain.DeletesPolicy{domain.DeletesNone, domain.DeletesQuarantine, domain.DeletesPropagate, "trash"} {
				cells++
				modes := map[string]domain.Mode{"alpha": a.mode, "beta": b.mode}
				in := map[string]domain.LinkDecision{"alpha": decide(a, 1, "alpha"), "beta": decide(b, 2, "beta")}
				got := plan.DecideFate(map[string]domain.ObjectDecision{
					"alpha": {Links: []domain.LinkDecision{in["alpha"]}},
					"beta":  {Links: []domain.LinkDecision{in["beta"]}},
				}, modes, nil, deletes)
				out := map[string]domain.LinkDecision{}
				for name, o := range got {
					if len(o.Links) == 1 {
						out[name] = o.Links[0]
					}
				}
				changed := changes(in["alpha"]) || changes(in["beta"])
				spread := deletes != domain.DeletesNone &&
					(out["alpha"].Action == domain.ActionDeleteAsset || out["beta"].Action == domain.ActionDeleteAsset)
				problem := ""
				switch {
				case len(got) != 2 || len(out) != 2:
					problem = "not one decision in each remote"
				case replaces(out["alpha"]) && replaces(out["beta"]):
					problem = "two replacements"
				}
				for _, name := range []string{"alpha", "beta"} {
					d, o, m := in[name], out[name], modes[name]
					_, known := domain.Rules[o.Rule]
					beaten := d.Action == domain.ActionDeleteAsset && changed
					waits := name == "beta" && replaces(d) && replaces(in["alpha"])
					switch {
					case problem != "":
					case o.Link != d.Link || o.Remote != d.Remote:
						problem = name + " loses its link or its file"
					case !plan.ModeAllows(m, o.Action) || !known && (o.Rule != "" || o.Action != domain.ActionNone):
						problem = name + " gets a decision that its mode does not run under a known rule"
					case changed && m == domain.ModeTwoWay && o.Action == domain.ActionDeleteAsset:
						problem = name + " deletes the changed asset"
					case spread && o.Action != domain.ActionDeleteAsset && o.ICloud != domain.ChangeDeleted:
						problem = name + " misses the deletion"
					case o != d && !beaten && !spread && !waits:
						problem = name + " changes by no rule"
					}
				}
				if problem != "" {
					wrong++
					if first == "" {
						first = fmt.Sprintf("alpha %+v and beta %+v under %s: %s; DecideFate = %+v", a, b, deletes, problem, got)
					}
				}
			}
		}
	}
	if cells != 2304 || wrong != 0 {
		t.Errorf("fated %d cells, %d of them wrong; want 2304 and none: 24 decisions that a mode gives a file in each of two remotes under 4 deletes policies; the first wrong: %s", cells, wrong, first)
	}
}
