package plan_test

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Each row is what DecideLayout and DecideComponents give one asset in a run
// under the deletes policy and the mode that its name says. The operations
// are written from the rules on deletions and albums, not from the code, and
// must not depend on the order of the input.
func TestAssetOps(t *testing.T) {
	t.Parallel()
	const (
		holidays, trip, party domain.AlbumID = 10, 11, 12

		personal, shared  = domain.LibraryPersonal, domain.LibraryShared
		unchanged, moved  = domain.ChangeUnchanged, domain.ChangeMoved
		replaced, deleted = domain.ChangeReplaced, domain.ChangeDeleted
		active, detached  = domain.StatusActive, domain.StatusDetached
		inTrash, away     = domain.StatusQuarantinedICloud, domain.StatusQuarantinedRemote
		atPhoto, atVideo  = "2024/05-01 - Holidays/IMG_0001.HEIC", "2024/05-01 - Holidays/IMG_0001.MOV"
		atTrip, atRaw     = "2024/05-02 - Trip/IMG_0001.HEIC", "2024/05-01 - Holidays/IMG_0001.DNG"
	)
	type (
		decided  = map[string][]domain.LinkDecision
		statuses = map[domain.LinkID]domain.LinkStatus
		followed = map[domain.LinkID][]domain.LinkDecision
		found    = map[domain.LinkID]domain.RemoteEntry
		changes  = []domain.AlbumChange
	)
	link := func(id domain.LinkID, remote string, root domain.LibraryKind, role domain.LinkRole, p string) domain.Link {
		return domain.Link{
			ID: id, AssetID: 7, Remote: remote, Root: root, Path: p, Role: role, AlbumID: holidays,
			Size: 100, MD5: "m1", Level: domain.LevelExact, Status: active,
		}
	}
	photo := link(1, "alpha", personal, domain.RoleOriginal, atPhoto)
	video := link(2, "alpha", personal, domain.RoleLiveVideo, atVideo)
	other := link(3, "beta", personal, domain.RoleOriginal, atPhoto)
	second := link(4, "alpha", shared, domain.RoleOriginal, atPhoto)
	raw := link(5, "alpha", personal, domain.RoleRAWPair, atRaw)
	dec := func(l domain.Link, icloud, remote domain.ChangeKind, a domain.LinkAction, rule string) domain.LinkDecision {
		return domain.LinkDecision{Link: l, ICloud: icloud, Remote: remote, Action: a, Rule: rule}
	}
	entry := func(root domain.LibraryKind, p string) domain.RemoteEntry {
		return domain.RemoteEntry{Root: root, Path: p, Size: 100, MD5: "m1"}
	}
	at := func(ls ...domain.Link) found {
		f := found{}
		for _, l := range ls {
			f[l.ID] = entry(l.Root, l.Path)
		}
		return f
	}
	with := func(l domain.Link, st domain.LinkStatus) domain.Link {
		l.Status = st
		return l
	}
	del := func(l domain.Link, root domain.LibraryKind, src string) domain.Op {
		return domain.Op{
			Kind: domain.OpDelete, Remote: l.Remote, Root: root, Src: src, Asset: 7,
			Rule: "R-STATUS-06", Irreversible: true, Links: []domain.Link{with(l, "")},
		}
	}
	album := func(k domain.OpKind, id domain.AlbumID, rule string) domain.Op {
		return domain.Op{Kind: k, Asset: 7, AlbumID: id, Rule: rule}
	}
	show := func(ops []domain.Op) []string {
		var lines []string
		for _, o := range ops {
			line := fmt.Sprintf("%s %s %s:%s asset %d album %d %s", o.Kind, o.Remote, o.Root, o.Src, o.Asset, o.AlbumID, o.Rule)
			if o.Irreversible {
				line += " irreversible"
			}
			for _, l := range o.Links {
				line += fmt.Sprintf(" [%d %s:%s %s]", l.ID, l.Root, l.Path, l.Status)
			}
			lines = append(lines, line)
		}
		return lines
	}
	for _, c := range []struct {
		name        string
		decisions   decided
		status      statuses
		join, leave changes
		components  followed
		files       found
		want        []domain.Op
	}{
		{"nothing changed",
			decided{"alpha": {dec(photo, unchanged, unchanged, domain.ActionNone, "")}, "beta": {dec(other, unchanged, unchanged, domain.ActionNone, "")}},
			statuses{1: active, 3: active}, nil, nil,
			followed{1: {dec(video, unchanged, unchanged, domain.ActionNone, "")}}, at(photo, video, other), nil},
		{"under propagate an asset deleted in iCloud leaves every remote for good, with its video",
			decided{
				"alpha": {dec(photo, deleted, unchanged, domain.ActionDeleteFile, "R-STATUS-06")},
				"beta":  {dec(other, deleted, unchanged, domain.ActionDeleteFile, "R-STATUS-06")},
			},
			statuses{1: "", 3: ""}, nil, nil,
			followed{1: {dec(video, deleted, unchanged, domain.ActionDeleteFile, "R-STATUS-06")}}, at(photo, video, other),
			[]domain.Op{del(photo, personal, atPhoto), del(video, personal, atVideo), del(other, personal, atPhoto)}},
		{"a video that the user moved to the other root is deleted where it lies",
			decided{"alpha": {dec(photo, deleted, unchanged, domain.ActionDeleteFile, "R-STATUS-06")}},
			statuses{1: ""}, nil, nil,
			followed{1: {dec(video, deleted, moved, domain.ActionDeleteFile, "R-STATUS-06")}},
			found{1: entry(personal, atPhoto), 2: entry(shared, "Misc/IMG_0001.MOV")},
			[]domain.Op{del(photo, personal, atPhoto), del(video, shared, "Misc/IMG_0001.MOV")}},
		{"under propagate a file deleted in the cloud deletes the asset, and the deletion reaches its video and the other remote",
			decided{
				"alpha": {dec(photo, unchanged, deleted, domain.ActionDeleteAsset, "R-STATUS-06")},
				"beta":  {dec(other, deleted, unchanged, domain.ActionDeleteFile, "R-STATUS-06")},
			},
			statuses{1: "", 3: ""}, nil, nil,
			followed{1: {dec(video, unchanged, unchanged, domain.ActionDeleteFile, "R-STATUS-06")}}, at(video, other),
			[]domain.Op{
				del(video, personal, atVideo), del(other, personal, atPhoto),
				{Kind: domain.OpAssetDelete, Asset: 7, Rule: "R-STATUS-06", Irreversible: true, Links: []domain.Link{with(photo, "")}},
			}},
		{"under quarantine the files gone from two remotes and two roots delete the asset once",
			decided{
				"alpha": {dec(second, unchanged, deleted, domain.ActionDeleteAsset, "R-STATUS-02"), dec(photo, unchanged, deleted, domain.ActionDeleteAsset, "R-STATUS-02")},
				"beta":  {dec(other, unchanged, deleted, domain.ActionDeleteAsset, "R-STATUS-02")},
			},
			statuses{1: inTrash, 3: inTrash, 4: inTrash}, nil, nil,
			followed{1: {dec(video, unchanged, unchanged, domain.ActionQuarantineFile, "R-STATUS-01")}}, at(video),
			[]domain.Op{{Kind: domain.OpAssetDelete, Asset: 7, Rule: "R-STATUS-02", Links: []domain.Link{with(photo, inTrash), with(other, inTrash), with(second, inTrash)}}}},
		{"under quarantine the files of an asset deleted in iCloud wait for their quarantine paths",
			decided{
				"alpha": {dec(photo, deleted, unchanged, domain.ActionQuarantineFile, "R-STATUS-01")},
				"beta":  {dec(other, deleted, unchanged, domain.ActionQuarantineFile, "R-STATUS-01")},
			},
			statuses{1: away, 3: away}, nil, nil,
			followed{1: {dec(video, deleted, unchanged, domain.ActionQuarantineFile, "R-STATUS-01")}}, at(photo, video, other), nil},
		{"under none a deletion detaches its link and deletes nothing",
			decided{"alpha": {dec(photo, unchanged, deleted, domain.ActionNone, "R-STATUS-05")}, "beta": {dec(other, unchanged, unchanged, domain.ActionNone, "")}},
			statuses{1: detached, 3: active}, nil, nil,
			followed{1: {dec(video, unchanged, unchanged, domain.ActionNone, "R-STATUS-05")}}, at(video, other), nil},
		{"upload-only: a file that the user deleted detaches its link and deletes no asset",
			decided{"alpha": {dec(photo, unchanged, deleted, domain.ActionNone, "R-MODE-01")}},
			statuses{1: detached}, nil, nil,
			followed{1: {dec(video, unchanged, unchanged, domain.ActionQuarantineFile, "R-STATUS-01")}}, at(video), nil},
		{"download-only: an asset deleted in iCloud detaches its link and deletes no file",
			decided{"alpha": {dec(photo, deleted, unchanged, domain.ActionNone, "R-MODE-02")}},
			statuses{1: detached}, nil, nil,
			followed{1: {dec(video, deleted, unchanged, domain.ActionNone, "R-MODE-02")}}, at(photo, video), nil},
		{"download-only under propagate: a file deleted in the cloud deletes the asset",
			decided{"alpha": {dec(photo, unchanged, deleted, domain.ActionDeleteAsset, "R-STATUS-06")}},
			statuses{1: ""}, nil, nil,
			followed{1: {dec(video, unchanged, unchanged, domain.ActionNone, "R-MODE-02")}}, at(video),
			[]domain.Op{{Kind: domain.OpAssetDelete, Asset: 7, Rule: "R-STATUS-06", Irreversible: true, Links: []domain.Link{with(photo, "")}}}},
		{"a file that the user moved takes the asset into the album of its new folder and out of the old one",
			decided{"alpha": {dec(photo, unchanged, moved, domain.ActionMoveAsset, "R-LINK-05")}},
			statuses{1: active}, changes{{AlbumID: trip, Rule: "R-LINK-05"}}, changes{{AlbumID: holidays, Rule: "R-LINK-05"}},
			followed{1: {dec(video, unchanged, unchanged, domain.ActionMoveFile, "R-COMPONENT-04")}},
			found{1: entry(personal, atTrip), 2: entry(personal, atVideo)},
			[]domain.Op{album(domain.OpAlbumAdd, trip, "R-LINK-05"), album(domain.OpAlbumRemove, holidays, "R-LINK-05")}},
		{"the first placement of a new asset leaves the farther events in the order of album ID",
			nil, nil, nil, changes{{AlbumID: party, Rule: "R-LAYOUT-04"}, {AlbumID: holidays, Rule: "R-LAYOUT-04"}}, nil, nil,
			[]domain.Op{album(domain.OpAlbumRemove, holidays, "R-LAYOUT-04"), album(domain.OpAlbumRemove, party, "R-LAYOUT-04")}},
		{"the files of both roots of a remote go personal first",
			decided{"alpha": {dec(second, deleted, unchanged, domain.ActionDeleteFile, "R-STATUS-06"), dec(photo, deleted, unchanged, domain.ActionDeleteFile, "R-STATUS-06")}},
			statuses{1: "", 4: ""}, nil, nil, nil, at(photo, second),
			[]domain.Op{del(photo, personal, atPhoto), del(second, shared, atPhoto)}},
		{"under propagate a video and a RAW beside no photo of the state are deleted",
			decided{"alpha": nil}, nil, nil, nil,
			followed{0: {
				dec(video, unchanged, unchanged, domain.ActionDeleteFile, "R-STATUS-06"),
				dec(raw, unchanged, unchanged, domain.ActionDeleteFile, "R-STATUS-06"),
			}}, at(video, raw),
			[]domain.Op{del(raw, personal, atRaw), del(video, personal, atVideo)}},
		{"moves and replacements need new paths or a new asset and give nothing here",
			decided{
				"alpha": {dec(photo, moved, unchanged, domain.ActionMoveFile, "R-LINK-04")},
				"beta":  {dec(other, moved, replaced, domain.ActionMoveFileReplaceAsset, "R-LINK-14")},
			},
			statuses{1: active, 3: active}, nil, nil,
			followed{1: {dec(video, moved, unchanged, domain.ActionMoveFile, "R-COMPONENT-04")}}, at(photo, video, other), nil},
		{"a link cleared from the state and a file imported as a new asset give nothing here",
			decided{"alpha": {dec(photo, deleted, deleted, domain.ActionForget, "R-LINK-12")}, "beta": {dec(other, deleted, moved, domain.ActionImport, "R-LINK-11")}},
			statuses{1: "", 3: active}, nil, nil,
			followed{1: {dec(video, deleted, deleted, domain.ActionForget, "R-COMPONENT-04")}},
			found{3: entry(personal, "2024/05-03 - Party/IMG_0001.HEIC")}, nil},
		{"upload-only: a file that the user moved is taken into the base, with nothing to run here",
			decided{"alpha": {dec(photo, unchanged, moved, domain.ActionUpdateBase, "R-MODE-01")}},
			statuses{1: active}, nil, nil,
			followed{1: {dec(video, unchanged, unchanged, domain.ActionMoveFile, "R-COMPONENT-04")}},
			found{1: entry(personal, atTrip), 2: entry(personal, atVideo)}, nil},
		{"nothing at all", nil, nil, nil, nil, nil, nil, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			decisions, components := decided{}, followed{}
			for name, ds := range c.decisions {
				decisions[name] = slices.Clone(ds)
			}
			for id, ds := range c.components {
				components[id] = slices.Clone(ds)
			}
			status, join, leave, files := maps.Clone(c.status), slices.Clone(c.join), slices.Clone(c.leave), maps.Clone(c.files)
			got := plan.AssetOps(7, c.decisions, c.status, c.join, c.leave, c.components, c.files)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("AssetOps = %q; want %q", show(got), show(c.want))
			}
			for _, o := range c.want {
				if _, ok := domain.Rules[o.Rule]; !ok {
					t.Errorf("%s is not in domain.Rules", o.Rule)
				}
			}
			rd, rc := decided{}, followed{}
			for name, ds := range c.decisions {
				rd[name] = slices.Clone(ds)
				slices.Reverse(rd[name])
			}
			for id, ds := range c.components {
				rc[id] = slices.Clone(ds)
				slices.Reverse(rc[id])
			}
			rj, rl := slices.Clone(c.join), slices.Clone(c.leave)
			slices.Reverse(rj)
			slices.Reverse(rl)
			if again := plan.AssetOps(7, rd, c.status, rj, rl, rc, c.files); !reflect.DeepEqual(again, got) {
				t.Errorf("with the decisions, components and album changes reversed: %q; want %q", show(again), show(got))
			}
			for _, o := range got {
				for i := range o.Links {
					o.Links[i].Path = "changed"
				}
			}
			if !maps.EqualFunc(c.decisions, decisions, slices.Equal[[]domain.LinkDecision]) || !maps.Equal(c.status, status) ||
				!slices.Equal(c.join, join) || !slices.Equal(c.leave, leave) ||
				!maps.EqualFunc(c.components, components, slices.Equal[[]domain.LinkDecision]) || !maps.Equal(c.files, files) {
				t.Errorf("AssetOps changed its input or shares a slice with it")
			}
		})
	}
}
