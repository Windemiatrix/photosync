package plan_test

import (
	"fmt"
	"path"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// A run takes the sidecars of a photo wherever it takes the photo, named
// after it, and to quarantine or out of the cloud with it under the deletes
// policy, but not to its copies and never onto a file of the user. The
// expected operations follow the rules of the components and of the names,
// not the code, and must not depend on the order of the operations of the
// main files, of the sidecars, of the scan and of the links.
func TestSidecarOps(t *testing.T) {
	t.Parallel()
	const (
		personal, shared                     = domain.LibraryPersonal, domain.LibraryShared
		holidays, trip, cats                 = "2024/05-01 - Holidays", "2024/05-02 - Trip", "Albums/Cats"
		inQuarantine                         = ".photosync/quarantine/2026-11-13/"
		twoWay, upOnly, downOnly             = domain.ModeTwoWay, domain.ModeUploadOnly, domain.ModeDownloadOnly
		quarantine, propagate, keep          = domain.DeletesQuarantine, domain.DeletesPropagate, domain.DeletesNone
		moveFile, parkFile, deleteFile, stay = domain.ActionMoveFile, domain.ActionQuarantineFile, domain.ActionDeleteFile, domain.ActionNone
		held                                 = domain.StatusQuarantinedRemote
	)
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	link := func(id domain.LinkID, remote string, root domain.LibraryKind, p string, album domain.AlbumID) domain.Link {
		return domain.Link{
			ID: id, AssetID: 7, Remote: remote, Root: root, Path: p, Role: domain.RoleOriginal, AlbumID: album,
			Level: domain.LevelExact, Status: domain.StatusActive,
		}
	}
	photo := link(1, "alpha", personal, holidays+"/IMG_0001.HEIC", 10)
	twin := link(3, "alpha", personal, holidays+"/IMG_0001 (1).HEIC", 10)
	spare := link(5, "alpha", shared, "Misc/IMG_0001.HEIC", 0)
	stranger := link(9, "alpha", personal, holidays+"/IMG_0002.HEIC", 10)
	stranger.AssetID = 8
	betaPhoto := link(11, "beta", personal, holidays+"/IMG_0001.HEIC", 10)
	links := []domain.Link{photo, twin, spare, stranger, betaPhoto}
	file := func(root domain.LibraryKind, p string) domain.RemoteEntry {
		return domain.RemoteEntry{Root: root, Path: p, Size: 10, ETag: "e1"}
	}
	h := func(name string) domain.RemoteEntry { return file(personal, holidays+"/"+name) }
	// The scan holds the files of every row: the photo where the base has it
	// and where the user moved or renamed it, the sidecars beside the photo
	// and beside its twin, whose own file the user deleted, an older file of
	// quarantine, files of the user with the names that the sidecars would
	// take, and the copy in the shared root with two sidecars whose names
	// differ in case alone.
	scan := []domain.RemoteEntry{
		h("IMG_0001.HEIC"), h("IMG_0001.AAE"), h("IMG_0001.xmp"), h("IMG_0001 (1).xmp"),
		h("IMG_0001.heic"), h("Beach.HEIC"), h("IMG_0002.HEIC"), h("IMG_0002.xmp"),
		file(personal, inQuarantine+holidays+"/IMG_0001.xmp"),
		file(personal, "Kept/IMG_0001.HEIC"), file(personal, "Kept/IMG_0001.AAE"),
		file(personal, "Mine/IMG_0001.HEIC"), file(personal, "Mine/IMG_0001.AAE"), file(personal, "Mine/IMG_0001.xmp"),
		file(shared, "Misc/IMG_0001.HEIC"), file(shared, "Misc/IMG_0001.xmp"), file(shared, "Misc/img_0001.xmp"),
	}
	found := func(ls ...domain.Link) map[domain.LinkID]domain.RemoteEntry {
		files := map[domain.LinkID]domain.RemoteEntry{}
		for _, l := range ls {
			files[l.ID] = file(l.Root, l.Path)
		}
		return files
	}
	to := func(l domain.Link, root domain.LibraryKind, p string, album domain.AlbumID) domain.Link {
		l.Root, l.Path, l.AlbumID = root, p, album
		return l
	}
	mainOp := func(k domain.OpKind, from, after domain.Link, dst, rule string) domain.Op {
		return domain.Op{
			Kind: k, Remote: "alpha", Root: from.Root, Src: from.Path, DstRoot: after.Root, Dst: dst, Asset: 7, Rule: rule,
			Links: []domain.Link{after},
		}
	}
	move := func(l domain.Link, root domain.LibraryKind, p string, album domain.AlbumID, rule string) domain.Op {
		return mainOp(domain.OpMove, l, to(l, root, p, album), p, rule)
	}
	dup := func(l domain.Link, root domain.LibraryKind, p string, album domain.AlbumID, rule string) domain.Op {
		c := to(l, root, p, album)
		c.ID = 0
		return mainOp(domain.OpCopy, l, c, p, rule)
	}
	park := func(l domain.Link, status domain.LinkStatus, q, rule string) domain.Op {
		after := l
		after.Status, after.QuarantinePath = status, q
		return mainOp(domain.OpQuarantine, l, after, q, rule)
	}
	moved := func(root domain.LibraryKind, src string, dstRoot domain.LibraryKind, dst string) domain.Op {
		return domain.Op{Kind: domain.OpMove, Remote: "alpha", Root: root, Src: src, DstRoot: dstRoot, Dst: dst, Asset: 7, Rule: "R-COMPONENT-04"}
	}
	parked := func(root domain.LibraryKind, src, dst, rule string) domain.Op {
		return domain.Op{Kind: domain.OpQuarantine, Remote: "alpha", Root: root, Src: src, DstRoot: root, Dst: dst, Asset: 7, Rule: rule}
	}
	deleted := func(src string) domain.Op {
		return domain.Op{Kind: domain.OpDelete, Remote: "alpha", Root: personal, Src: src, Asset: 7, Rule: "R-STATUS-06", Irreversible: true}
	}
	taken := func(root domain.LibraryKind, p string) domain.Notice {
		return domain.Notice{Code: "plan.path_taken", Params: []string{"alpha", string(root), p}}
	}
	show := func(ops []domain.Op, notices []domain.Notice) string {
		var out []string
		for _, o := range ops {
			out = append(out, fmt.Sprintf("%s %s %s:%s -> %s:%s asset %d album %d %s irreversible=%t links=%v",
				o.Kind, o.Remote, o.Root, o.Src, o.DstRoot, o.Dst, o.Asset, o.AlbumID, o.Rule, o.Irreversible, o.Links))
		}
		for _, n := range notices {
			out = append(out, n.Code+" "+strings.Join(n.Params, "|"))
		}
		return fmt.Sprintf("%q", out)
	}
	for _, c := range []struct {
		name     string
		mode     domain.Mode
		deletes  domain.DeletesPolicy
		mains    []domain.Op
		fates    map[domain.LinkID]domain.LinkAction
		sidecars map[domain.LinkID][]domain.RemoteEntry
		files    map[domain.LinkID]domain.RemoteEntry
		ops      []domain.Op
		notices  []domain.Notice
	}{
		{"a photo that the plan moves takes its sidecars along, each named after the photo, and its copy takes none", twoWay, quarantine,
			[]domain.Op{
				dup(photo, personal, cats+"/IMG_0001.HEIC", 13, "R-OBJECT-03"),
				move(photo, personal, trip+"/IMG_0001.HEIC", 11, "R-OBJECT-05"),
			},
			map[domain.LinkID]domain.LinkAction{1: moveFile},
			map[domain.LinkID][]domain.RemoteEntry{1: {h("IMG_0001.xmp"), h("IMG_0001.AAE")}},
			found(photo),
			[]domain.Op{
				moved(personal, holidays+"/IMG_0001.AAE", personal, trip+"/IMG_0001.AAE"),
				moved(personal, holidays+"/IMG_0001.xmp", personal, trip+"/IMG_0001.xmp"),
			},
			nil},
		{"a move to the other library takes the sidecar to the other root", twoWay, quarantine,
			[]domain.Op{move(photo, shared, holidays+"/IMG_0001.HEIC", 10, "R-OBJECT-07")},
			map[domain.LinkID]domain.LinkAction{1: moveFile},
			map[domain.LinkID][]domain.RemoteEntry{1: {h("IMG_0001.xmp")}},
			found(photo),
			[]domain.Op{moved(personal, holidays+"/IMG_0001.xmp", shared, holidays+"/IMG_0001.xmp")},
			nil},
		{"the sidecars that the user left behind follow the photo that the user moved, but not onto a file of the user", twoWay, quarantine,
			nil, map[domain.LinkID]domain.LinkAction{1: moveFile},
			map[domain.LinkID][]domain.RemoteEntry{1: {h("IMG_0001.AAE"), h("IMG_0001.xmp")}},
			found(to(photo, personal, "Kept/IMG_0001.HEIC", 0)),
			[]domain.Op{moved(personal, holidays+"/IMG_0001.xmp", personal, "Kept/IMG_0001.xmp")},
			[]domain.Notice{taken(personal, "Kept/IMG_0001.AAE")}},
		{"the sidecars of a photo that the user moved stay where files of the user hold both of their names, each with a notice in the order of the paths",
			twoWay, quarantine,
			nil, map[domain.LinkID]domain.LinkAction{1: moveFile},
			map[domain.LinkID][]domain.RemoteEntry{1: {h("IMG_0001.xmp"), h("IMG_0001.AAE")}},
			found(to(photo, personal, "Mine/IMG_0001.HEIC", 0)),
			nil, []domain.Notice{taken(personal, "Mine/IMG_0001.AAE"), taken(personal, "Mine/IMG_0001.xmp")}},
		{"a photo whose extension the user changed in case alone keeps its sidecar beside it, without a notice", twoWay, quarantine,
			nil, map[domain.LinkID]domain.LinkAction{1: moveFile},
			map[domain.LinkID][]domain.RemoteEntry{1: {h("IMG_0001.xmp")}},
			found(to(photo, personal, holidays+"/IMG_0001.heic", 10)),
			nil, nil},
		{"a sidecar takes the new name that the user gave the photo in its directory", twoWay, quarantine,
			nil, map[domain.LinkID]domain.LinkAction{1: moveFile},
			map[domain.LinkID][]domain.RemoteEntry{1: {h("IMG_0001.xmp")}},
			found(to(photo, personal, holidays+"/Beach.HEIC", 10)),
			[]domain.Op{moved(personal, holidays+"/IMG_0001.xmp", personal, holidays+"/Beach.xmp")},
			nil},
		{"two sidecars whose names differ in case alone take one name in the other root: the first by path follows the file, the other stays with a notice",
			twoWay, quarantine,
			[]domain.Op{move(spare, personal, trip+"/IMG_0001.HEIC", 11, "R-OBJECT-07")},
			map[domain.LinkID]domain.LinkAction{5: moveFile},
			map[domain.LinkID][]domain.RemoteEntry{5: {file(shared, "Misc/img_0001.xmp"), file(shared, "Misc/IMG_0001.xmp")}},
			found(spare),
			[]domain.Op{moved(shared, "Misc/IMG_0001.xmp", personal, trip+"/IMG_0001.xmp")},
			[]domain.Notice{taken(personal, trip+"/IMG_0001.xmp")}},
		{"a needless copy goes to quarantine in its own root, and its sidecar follows it there whatever the deletes policy", twoWay, propagate,
			[]domain.Op{park(spare, "", inQuarantine+"Misc/IMG_0001.HEIC", "R-OBJECT-04")},
			map[domain.LinkID]domain.LinkAction{5: parkFile},
			map[domain.LinkID][]domain.RemoteEntry{5: {file(shared, "Misc/IMG_0001.xmp")}},
			found(spare),
			[]domain.Op{parked(shared, "Misc/IMG_0001.xmp", inQuarantine+"Misc/IMG_0001.xmp", "R-COMPONENT-04")},
			nil},
		{"under quarantine the sidecars of an asset gone from iCloud go to quarantine from where they lie, beside a file gone too, the lower link ID first",
			twoWay, quarantine,
			[]domain.Op{park(photo, held, inQuarantine+holidays+"/IMG_0001.HEIC", "R-STATUS-01")},
			map[domain.LinkID]domain.LinkAction{1: deleteFile, 3: deleteFile},
			map[domain.LinkID][]domain.RemoteEntry{1: {h("IMG_0001.xmp"), h("IMG_0001.AAE")}, 3: {h("IMG_0001 (1).xmp")}},
			found(photo),
			[]domain.Op{
				parked(personal, holidays+"/IMG_0001 (1).xmp", inQuarantine+holidays+"/IMG_0001 (1) (1).xmp", "R-STATUS-01"),
				parked(personal, holidays+"/IMG_0001.AAE", inQuarantine+holidays+"/IMG_0001.AAE", "R-STATUS-01"),
				parked(personal, holidays+"/IMG_0001.xmp", inQuarantine+holidays+"/IMG_0001 (1).xmp", "R-STATUS-01"),
			},
			nil},
		{"under propagate the sidecars of an asset gone from iCloud leave the cloud for good", twoWay, propagate,
			nil, map[domain.LinkID]domain.LinkAction{1: deleteFile, 3: deleteFile},
			map[domain.LinkID][]domain.RemoteEntry{1: {h("IMG_0001.xmp"), h("IMG_0001.AAE")}, 3: {h("IMG_0001 (1).xmp")}},
			found(photo),
			[]domain.Op{deleted(holidays + "/IMG_0001 (1).xmp"), deleted(holidays + "/IMG_0001.AAE"), deleted(holidays + "/IMG_0001.xmp")},
			nil},
		{"under none the sidecars of an asset gone from iCloud stay", twoWay, keep,
			nil, map[domain.LinkID]domain.LinkAction{1: deleteFile, 3: deleteFile},
			map[domain.LinkID][]domain.RemoteEntry{1: {h("IMG_0001.xmp"), h("IMG_0001.AAE")}, 3: {h("IMG_0001 (1).xmp")}},
			found(photo), nil, nil},
		{"upload-only: the sidecar of a photo that the user deleted goes to quarantine", upOnly, quarantine,
			nil, map[domain.LinkID]domain.LinkAction{1: deleteFile},
			map[domain.LinkID][]domain.RemoteEntry{1: {h("IMG_0001.xmp")}},
			nil,
			[]domain.Op{parked(personal, holidays+"/IMG_0001.xmp", inQuarantine+holidays+"/IMG_0001 (1).xmp", "R-STATUS-01")},
			nil},
		{"download-only: nothing goes to the cloud when the user moves the photo and deletes its twin", downOnly, quarantine,
			nil, map[domain.LinkID]domain.LinkAction{1: moveFile, 3: deleteFile},
			map[domain.LinkID][]domain.RemoteEntry{1: {h("IMG_0001.xmp")}, 3: {h("IMG_0001 (1).xmp")}},
			found(to(photo, personal, "Kept/IMG_0001.HEIC", 0)), nil, nil},
		{"a photo that stays, a photo of another remote and a sidecar of another asset give nothing here", twoWay, quarantine,
			nil, map[domain.LinkID]domain.LinkAction{1: stay, 11: moveFile},
			map[domain.LinkID][]domain.RemoteEntry{1: {h("IMG_0001.xmp")}, 9: {h("IMG_0002.xmp")}},
			found(photo, to(betaPhoto, personal, "Kept/IMG_0001.HEIC", 0)), nil, nil},
		{"nothing at all", twoWay, quarantine, nil, nil, nil, nil, nil, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			input := fmt.Sprintf("%+v %+v %+v %+v", c.mains, c.fates, c.sidecars, c.files)
			names := plan.NewNames("alpha", scan, links)
			got, notices := plan.SidecarOps(names, 7, c.mains, c.fates, c.sidecars, c.files, c.mode, c.deletes, now)
			if !reflect.DeepEqual(got, c.ops) || !reflect.DeepEqual(notices, c.notices) {
				t.Errorf("SidecarOps = %s; want %s", show(got, notices), show(c.ops, c.notices))
			}
			for _, o := range c.ops {
				if _, ok := domain.Rules[o.Rule]; !ok {
					t.Errorf("%s is not in domain.Rules", o.Rule)
				}
			}
			// Free numbers a name whose stem the names hold, so a destination
			// that they hold comes back changed; a deletion has none.
			for _, o := range got {
				if o.Kind == domain.OpDelete {
					continue
				}
				if again := names.Free(o.DstRoot, path.Dir(o.Dst), path.Base(o.Dst)); again == o.Dst {
					t.Errorf("the names of the run do not hold %s:%s", o.DstRoot, o.Dst)
				}
			}
			rm, rs, rl := slices.Clone(c.mains), slices.Clone(scan), slices.Clone(links)
			slices.Reverse(rm)
			slices.Reverse(rs)
			slices.Reverse(rl)
			rsc := map[domain.LinkID][]domain.RemoteEntry{}
			for id, es := range c.sidecars {
				rsc[id] = slices.Clone(es)
				slices.Reverse(rsc[id])
			}
			again, more := plan.SidecarOps(plan.NewNames("alpha", rs, rl), 7, rm, c.fates, rsc, c.files, c.mode, c.deletes, now)
			if !reflect.DeepEqual(again, got) || !reflect.DeepEqual(more, notices) {
				t.Errorf("with the operations, the sidecars, the scan and the links reversed: %s; want %s", show(again, more), show(got, notices))
			}
			if after := fmt.Sprintf("%+v %+v %+v %+v", c.mains, c.fates, c.sidecars, c.files); after != input {
				t.Errorf("SidecarOps changed its input:\n%s\nwant\n%s", after, input)
			}
		})
	}
}
