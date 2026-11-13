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

// A run takes the Live Photo video and the RAW file of a photo wherever it
// takes the photo, named after it, and never onto a file of the user. The
// expected operations follow the rules of the components and of the names,
// not the code, and must not depend on the order of the operations of the
// main files, of the lists, of the scan and of the links.
func TestComponentOps(t *testing.T) {
	t.Parallel()
	const (
		personal, shared                    = domain.LibraryPersonal, domain.LibraryShared
		holidays, trip, party               = "2024/05-01 - Holidays", "2024/05-02 - Trip", "2024/05-03 - Party"
		cats, zoo, day                      = "Albums/Cats", "2024/05-05 - Zoo", "2024/05-04"
		inQuarantine                        = ".photosync/quarantine/2026-11-13/"
		unchanged, moved, replaced, deleted = domain.ChangeUnchanged, domain.ChangeMoved, domain.ChangeReplaced, domain.ChangeDeleted
		quarantine, held                    = domain.ActionQuarantineFile, domain.StatusQuarantinedRemote
	)
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	original := domain.Resource{Role: domain.ResourceOriginal, Filename: "IMG_0001.HEIC", Size: 100, SHA256: "s7", MD5: "m7"}
	movie := domain.Resource{Role: domain.ResourceLiveVideo, Filename: "IMG_0001.MOV", Size: 30, SHA256: "sv", MD5: "mv"}
	rawFile := domain.Resource{Role: domain.ResourceRAWPair, Filename: "IMG_0001.DNG", Size: 200, SHA256: "sr", MD5: "mr"}
	group := domain.Asset{
		ID: 7, Key: domain.AssetKey{UUID: "uuid-7"}, Library: personal, TargetLibrary: personal,
		Resources: []domain.Resource{original, movie, rawFile},
	}
	inShared, single := group, group
	inShared.TargetLibrary, single.Resources = shared, []domain.Resource{original}
	link := func(id domain.LinkID, remote string, role domain.LinkRole, root domain.LibraryKind, p string, album domain.AlbumID, size int64, md5 string) domain.Link {
		return domain.Link{
			ID: id, AssetID: 7, Remote: remote, Root: root, Path: p, Role: role, AlbumID: album, Size: size,
			SHA256: "s" + md5, MD5: md5, ETag: "e" + md5, MTime: now.Add(-time.Hour), Level: domain.LevelExact, Status: domain.StatusActive,
		}
	}
	photo := link(1, "alpha", domain.RoleOriginal, personal, holidays+"/IMG_0001.HEIC", 10, 100, "m1")
	video := link(2, "alpha", domain.RoleLiveVideo, personal, holidays+"/IMG_0001.MOV", 10, 30, "m2")
	raw := link(3, "alpha", domain.RoleRAWPair, personal, holidays+"/IMG_0001.DNG", 10, 200, "m3")
	stray := link(4, "alpha", domain.RoleLiveVideo, personal, holidays+"/IMG_0001 (1).MOV", 10, 30, "m4")
	needless := link(5, "alpha", domain.RoleOriginal, shared, "Misc/IMG_0001.HEIC", 0, 100, "m1")
	needlessVideo := link(6, "alpha", domain.RoleLiveVideo, shared, "Misc/IMG_0001.MOV", 0, 30, "m2")
	betaPhoto := link(11, "beta", domain.RoleOriginal, personal, holidays+"/IMG_0001.HEIC", 10, 100, "m1")
	betaVideo := link(12, "beta", domain.RoleLiveVideo, personal, holidays+"/IMG_0001.MOV", 10, 30, "m2")
	links := []domain.Link{photo, video, raw, stray, needless, needlessVideo, betaPhoto, betaVideo}
	// Matching makes this link in the run, so the state does not hold it.
	lesser := link(21, "alpha", domain.RoleOriginal, personal, zoo+"/IMG_0001.jpg", 14, 40, "c21")
	// The scan holds the files of every row, where the base has them and where
	// the user put them, an older file of quarantine and files of the user
	// with the names that a video and a RAW file would take. The video of the
	// base in Holidays stays for the row where the user moved it away, its
	// name differing in case alone from the one that a video beside the photo
	// there takes, so that only a check of the exact path finds that one free.
	scan := []domain.RemoteEntry{
		{Root: personal, Path: holidays + "/IMG_0001.HEIC", Size: 100, MD5: "m1"},
		{Root: personal, Path: holidays + "/IMG_0001.MOV", Size: 30, MD5: "m2"},
		{Root: personal, Path: holidays + "/IMG_0001.DNG", Size: 200, MD5: "m3"},
		{Root: personal, Path: holidays + "/IMG_0001 (1).MOV", Size: 30, MD5: "m4"},
		{Root: personal, Path: inQuarantine + holidays + "/IMG_0001.MOV", Size: 30, MD5: "m9"},
		{Root: personal, Path: "Misc/IMG_0001.MOV", Size: 30, MD5: "m2"},
		{Root: personal, Path: "Misc/IMG_0001.DNG", Size: 200, MD5: "m3"},
		{Root: personal, Path: "Kept/IMG_0001.HEIC", Size: 100, MD5: "m1"},
		{Root: personal, Path: "Kept/IMG_0001.mov", Size: 25, MD5: "u1"},
		{Root: personal, Path: "Mine/IMG_0001.HEIC", Size: 100, MD5: "m1"},
		{Root: personal, Path: "Mine/IMG_0001.DNG", Size: 190, MD5: "u2"},
		{Root: personal, Path: "Mine/IMG_0001.mov", Size: 20, MD5: "u3"},
		{Root: personal, Path: party + "/IMG_0001.MOV", Size: 30, MD5: "m2"},
		{Root: personal, Path: zoo + "/IMG_0001.jpg", Size: 40, MD5: "c21"},
		{Root: shared, Path: "Misc/IMG_0001.HEIC", Size: 100, MD5: "m1"},
		{Root: shared, Path: "Misc/IMG_0001.MOV", Size: 30, MD5: "m2"},
	}
	dec := func(l domain.Link, remote domain.ChangeKind, a domain.LinkAction, rule string) domain.LinkDecision {
		return domain.LinkDecision{Link: l, ICloud: unchanged, Remote: remote, Action: a, Rule: rule}
	}
	place := func(a domain.LinkAction, l domain.Link, album domain.AlbumID, rule string) domain.Placement {
		return domain.Placement{Action: a, Link: l, AlbumID: album, Rule: rule}
	}
	filesOf := func(ls ...domain.Link) map[domain.LinkID]domain.RemoteEntry {
		files := map[domain.LinkID]domain.RemoteEntry{}
		for _, l := range ls {
			files[l.ID] = domain.RemoteEntry{Root: l.Root, Path: l.Path, Size: l.Size, MD5: l.MD5}
		}
		return files
	}
	at := func(l domain.Link, id domain.LinkID, root domain.LibraryKind, p string, album domain.AlbumID) domain.Link {
		l.ID, l.Root, l.Path, l.AlbumID = id, root, p, album
		return l
	}
	op := func(k domain.OpKind, root domain.LibraryKind, src string, dstRoot domain.LibraryKind, dst, rule string, l domain.Link) domain.Op {
		return domain.Op{Kind: k, Remote: "alpha", Root: root, Src: src, DstRoot: dstRoot, Dst: dst, Asset: 7, Rule: rule, Links: []domain.Link{l}}
	}
	move := func(l domain.Link, root domain.LibraryKind, p string, album domain.AlbumID, rule string) domain.Op {
		return op(domain.OpMove, l.Root, l.Path, root, p, rule, at(l, l.ID, root, p, album))
	}
	dup := func(l domain.Link, root domain.LibraryKind, p string, album domain.AlbumID, rule string) domain.Op {
		return op(domain.OpCopy, l.Root, l.Path, root, p, rule, at(l, 0, root, p, album))
	}
	upload := func(id domain.LinkID, role domain.LinkRole, res domain.Resource, root domain.LibraryKind, p string, album domain.AlbumID, rule string) domain.Op {
		return op(domain.OpPut, "", "", root, p, rule, domain.Link{
			ID: id, AssetID: 7, Remote: "alpha", Root: root, Path: p, Role: role, AlbumID: album,
			Size: res.Size, SHA256: res.SHA256, MD5: res.MD5, Level: domain.LevelUploaded, Status: domain.StatusActive,
		})
	}
	park := func(l domain.Link, status domain.LinkStatus, q, rule string) domain.Op {
		after := l
		after.Status, after.QuarantinePath = status, q
		return op(domain.OpQuarantine, l.Root, l.Path, l.Root, q, rule, after)
	}
	taken := func(p string) []domain.Notice {
		return []domain.Notice{{Code: "plan.path_taken", Params: []string{"alpha", "personal", p}}}
	}
	show := func(xs []domain.Op, ns []domain.Notice) string {
		var out []string
		for _, o := range xs {
			line := fmt.Sprintf("%s %s %s:%s -> %s:%s asset %d album %d %s irreversible=%t",
				o.Kind, o.Remote, o.Root, o.Src, o.DstRoot, o.Dst, o.Asset, o.AlbumID, o.Rule, o.Irreversible)
			for _, l := range o.Links {
				line += fmt.Sprintf(" [%d %s asset %d %s %s:%s album %d %s %s %d %s %s %s %s q=%s]",
					l.ID, l.Remote, l.AssetID, l.Role, l.Root, l.Path, l.AlbumID, l.Level, l.Status, l.Size, l.SHA256, l.MD5,
					l.ETag, l.MTime.Format(time.RFC3339), l.QuarantinePath)
			}
			out = append(out, line)
		}
		for _, n := range ns {
			out = append(out, n.Code+" "+strings.Join(n.Params, "|"))
		}
		return fmt.Sprintf("%q", out)
	}
	for _, c := range []struct {
		name       string
		target     domain.Asset
		mains      []domain.Op
		components map[domain.LinkID][]domain.LinkDecision
		copies     map[domain.LinkID][]domain.Placement
		after      map[domain.LinkID]domain.LinkStatus
		files      map[domain.LinkID]domain.RemoteEntry
		ops        []domain.Op
		notices    []domain.Notice
	}{
		{"a photo that the plan moves takes its video and its RAW file along, the RAW file with new content too, each named after the photo",
			group, []domain.Op{move(photo, personal, trip+"/IMG_0001.HEIC", 11, "R-OBJECT-05")},
			map[domain.LinkID][]domain.LinkDecision{1: {
				dec(video, unchanged, domain.ActionMoveFile, "R-COMPONENT-04"),
				dec(raw, replaced, domain.ActionMoveFileReplaceAsset, "R-COMPONENT-03"),
			}},
			nil, nil, filesOf(photo, video, raw),
			[]domain.Op{
				move(raw, personal, trip+"/IMG_0001.DNG", 11, "R-COMPONENT-03"),
				move(video, personal, trip+"/IMG_0001.mov", 11, "R-COMPONENT-04"),
			},
			nil},
		{"a move to the other library takes the video along to the other root", inShared,
			[]domain.Op{move(photo, shared, holidays+"/IMG_0001.HEIC", 10, "R-OBJECT-07")},
			map[domain.LinkID][]domain.LinkDecision{1: {dec(video, unchanged, domain.ActionMoveFile, "R-COMPONENT-04")}},
			nil, nil, filesOf(photo, video),
			[]domain.Op{move(video, shared, holidays+"/IMG_0001.mov", 10, "R-COMPONENT-04")},
			nil},
		{"a video that the user moved away goes back beside the photo that stays, and the RAW file beside the photo stays too", group, nil,
			map[domain.LinkID][]domain.LinkDecision{1: {
				dec(video, moved, domain.ActionMoveFile, "R-COMPONENT-02"), dec(raw, unchanged, domain.ActionNone, ""),
			}},
			nil, nil, filesOf(photo, at(video, 2, personal, "Misc/IMG_0001.MOV", 10), raw),
			[]domain.Op{move(at(video, 2, personal, "Misc/IMG_0001.MOV", 10), personal, holidays+"/IMG_0001.mov", 10, "R-COMPONENT-02")},
			nil},
		{"the files beside a photo that the user moved follow it, but not onto a file of the user", group, nil,
			map[domain.LinkID][]domain.LinkDecision{1: {
				dec(video, unchanged, domain.ActionMoveFile, "R-COMPONENT-04"), dec(raw, unchanged, domain.ActionMoveFile, "R-COMPONENT-04"),
			}},
			nil, nil, filesOf(at(photo, 1, personal, "Kept/IMG_0001.HEIC", 0), video, raw),
			[]domain.Op{move(raw, personal, "Kept/IMG_0001.DNG", 10, "R-COMPONENT-04")},
			taken("Kept/IMG_0001.mov")},
		{"a video that the user put where the plan moves the photo takes the number that the photo takes there", group,
			[]domain.Op{move(photo, personal, party+"/IMG_0001 (1).HEIC", 12, "R-OBJECT-05")},
			map[domain.LinkID][]domain.LinkDecision{1: {dec(video, moved, domain.ActionMoveFile, "R-COMPONENT-02")}},
			nil, nil, filesOf(photo, at(video, 2, personal, party+"/IMG_0001.MOV", 10)),
			[]domain.Op{move(at(video, 2, personal, party+"/IMG_0001.MOV", 10), personal, party+"/IMG_0001 (1).mov", 12, "R-COMPONENT-02")},
			nil},
		{"a needless copy goes to quarantine in its own root, and its video follows it there without a link", group,
			[]domain.Op{park(needless, "", inQuarantine+"Misc/IMG_0001.HEIC", "R-OBJECT-04")},
			map[domain.LinkID][]domain.LinkDecision{5: {dec(needlessVideo, unchanged, quarantine, "R-COMPONENT-04")}},
			nil, map[domain.LinkID]domain.LinkStatus{6: ""}, filesOf(needless, needlessVideo),
			[]domain.Op{park(needlessVideo, "", inQuarantine+"Misc/IMG_0001.MOV", "R-COMPONENT-04")},
			nil},
		{"under quarantine the files of an asset gone from iCloud go to quarantine from where they lie, the one beside no photo first",
			domain.Asset{}, []domain.Op{park(photo, held, inQuarantine+holidays+"/IMG_0001.HEIC", "R-STATUS-01")},
			map[domain.LinkID][]domain.LinkDecision{
				1: {dec(video, unchanged, quarantine, "R-STATUS-01"), dec(raw, moved, quarantine, "R-STATUS-01")},
				0: {dec(stray, unchanged, quarantine, "R-STATUS-01")},
			},
			nil, map[domain.LinkID]domain.LinkStatus{2: held, 3: held, 4: held},
			filesOf(photo, video, at(raw, 3, personal, "Misc/IMG_0001.DNG", 10), stray),
			[]domain.Op{
				park(stray, held, inQuarantine+holidays+"/IMG_0001 (1).MOV", "R-STATUS-01"),
				park(video, held, inQuarantine+holidays+"/IMG_0001 (2).MOV", "R-STATUS-01"),
				park(at(raw, 3, personal, "Misc/IMG_0001.DNG", 10), held, inQuarantine+"Misc/IMG_0001.DNG", "R-STATUS-01"),
			},
			nil},
		{"each copy of the photo takes a copy of its video and of its RAW file beside it", group,
			[]domain.Op{dup(photo, personal, trip+"/IMG_0001.HEIC", 11, "R-OBJECT-03"), dup(photo, personal, cats+"/IMG_0001.HEIC", 13, "R-OBJECT-03")},
			map[domain.LinkID][]domain.LinkDecision{1: {dec(video, unchanged, domain.ActionNone, ""), dec(raw, unchanged, domain.ActionNone, "")}},
			map[domain.LinkID][]domain.Placement{1: {
				place(domain.ActionCopyFile, video, 11, "R-OBJECT-03"), place(domain.ActionCopyFile, video, 13, "R-OBJECT-03"),
				place(domain.ActionCopyFile, raw, 11, "R-OBJECT-03"), place(domain.ActionCopyFile, raw, 13, "R-OBJECT-03"),
			}},
			nil, filesOf(photo, video, raw),
			[]domain.Op{
				dup(raw, personal, trip+"/IMG_0001.DNG", 11, "R-OBJECT-03"), dup(raw, personal, cats+"/IMG_0001.DNG", 13, "R-OBJECT-03"),
				dup(video, personal, trip+"/IMG_0001.mov", 11, "R-OBJECT-03"), dup(video, personal, cats+"/IMG_0001.mov", 13, "R-OBJECT-03"),
			},
			nil},
		{"a video deleted on its own is restored beside the photo and beside the copy of the photo", group,
			[]domain.Op{dup(photo, personal, trip+"/IMG_0001.HEIC", 11, "R-OBJECT-03")},
			map[domain.LinkID][]domain.LinkDecision{1: {dec(video, deleted, domain.ActionUpload, "R-COMPONENT-01")}},
			map[domain.LinkID][]domain.Placement{1: {place(domain.ActionUpload, video, 11, "R-COMPONENT-01")}},
			nil, filesOf(photo),
			[]domain.Op{
				upload(2, domain.RoleLiveVideo, movie, personal, holidays+"/IMG_0001.mov", 10, "R-COMPONENT-01"),
				upload(0, domain.RoleLiveVideo, movie, personal, trip+"/IMG_0001.mov", 11, "R-COMPONENT-01"),
			},
			nil},
		{"the files of another remote give nothing here, though they lie at the same paths", group,
			[]domain.Op{dup(photo, personal, trip+"/IMG_0001.HEIC", 11, "R-OBJECT-03")},
			map[domain.LinkID][]domain.LinkDecision{
				1:  {dec(video, unchanged, domain.ActionNone, "")},
				11: {dec(betaVideo, moved, domain.ActionMoveFile, "R-COMPONENT-02")},
			},
			map[domain.LinkID][]domain.Placement{
				1:  {place(domain.ActionCopyFile, video, 11, "R-OBJECT-03")},
				11: {place(domain.ActionCopyFile, at(betaVideo, 12, personal, "Misc/IMG_0001.MOV", 10), 11, "R-OBJECT-03")},
			},
			nil, filesOf(photo, video, betaPhoto, at(betaVideo, 12, personal, "Misc/IMG_0001.MOV", 10)),
			[]domain.Op{dup(video, personal, trip+"/IMG_0001.mov", 11, "R-OBJECT-03")},
			nil},
		{"upload-only: each path that the photo goes up to takes the video and the RAW file of the asset beside it", group,
			[]domain.Op{
				upload(0, domain.RoleOriginal, original, personal, "Holidays/IMG_0001.HEIC", 10, "R-LINK-01"),
				upload(0, domain.RoleOriginal, original, personal, "Trips/IMG_0001.HEIC", 10, "R-LINK-01"),
			},
			nil, nil, nil, nil,
			[]domain.Op{
				upload(0, domain.RoleRAWPair, rawFile, personal, "Holidays/IMG_0001.DNG", 10, "R-LINK-01"),
				upload(0, domain.RoleLiveVideo, movie, personal, "Holidays/IMG_0001.mov", 10, "R-LINK-01"),
				upload(0, domain.RoleRAWPair, rawFile, personal, "Trips/IMG_0001.DNG", 10, "R-LINK-01"),
				upload(0, domain.RoleLiveVideo, movie, personal, "Trips/IMG_0001.mov", 10, "R-LINK-01"),
			},
			nil},
		{"a replacement goes up with its own video, and the original put in place of a copy of the lesser file takes its files anew", group,
			[]domain.Op{
				park(lesser, held, inQuarantine+zoo+"/IMG_0001.jpg", "R-OBJECT-10"),
				upload(0, domain.RoleOriginal, original, personal, trip+"/IMG_0001.HEIC", 11, "R-OBJECT-03"),
				upload(21, domain.RoleOriginal, original, personal, zoo+"/IMG_0001 (1).HEIC", 14, "R-OBJECT-10"),
				upload(22, domain.RoleLiveVideo, movie, personal, zoo+"/IMG_0001 (1).mov", 14, "R-OBJECT-10"),
			},
			nil, nil, nil, nil,
			[]domain.Op{
				upload(0, domain.RoleRAWPair, rawFile, personal, trip+"/IMG_0001.DNG", 11, "R-OBJECT-03"),
				upload(0, domain.RoleLiveVideo, movie, personal, trip+"/IMG_0001.mov", 11, "R-OBJECT-03"),
			},
			nil},
		{"an asset without a Live Photo video or a RAW file puts up its photo alone", single,
			[]domain.Op{upload(0, domain.RoleOriginal, original, personal, day+"/IMG_0001.HEIC", 0, "R-LINK-01")},
			nil, nil, nil, nil, nil, nil},
		{"a replacement, a deletion, a cleared link and a base that takes a file need no operation here", group, nil,
			map[domain.LinkID][]domain.LinkDecision{
				0: {dec(stray, deleted, domain.ActionForget, "R-COMPONENT-04")},
				1: {dec(video, replaced, domain.ActionReplaceAsset, "R-COMPONENT-03"), dec(raw, moved, domain.ActionUpdateBase, "R-COMPONENT-04")},
				5: {dec(needlessVideo, unchanged, domain.ActionDeleteFile, "R-STATUS-06")},
			},
			nil, nil, filesOf(photo, video, raw, needlessVideo), nil, nil},
		{"download-only: nothing goes to the cloud", group, nil,
			map[domain.LinkID][]domain.LinkDecision{
				0: {dec(stray, unchanged, domain.ActionNone, "R-MODE-02")},
				1: {dec(video, moved, domain.ActionUpdateBase, "R-MODE-02"), dec(raw, unchanged, domain.ActionNone, "R-MODE-02")},
			},
			nil, nil, filesOf(photo, at(video, 2, personal, "Misc/IMG_0001.MOV", 10), raw, stray), nil, nil},
		{"nothing at all", domain.Asset{}, nil, nil, nil, nil, nil, nil, nil},
		{"the files beside a photo that the user moved stay where files of the user hold both of their names, each with a notice", group, nil,
			map[domain.LinkID][]domain.LinkDecision{1: {
				dec(video, unchanged, domain.ActionMoveFile, "R-COMPONENT-04"), dec(raw, unchanged, domain.ActionMoveFile, "R-COMPONENT-04"),
			}},
			nil, nil, filesOf(at(photo, 1, personal, "Mine/IMG_0001.HEIC", 0), video, raw),
			nil, append(taken("Mine/IMG_0001.DNG"), taken("Mine/IMG_0001.mov")...)},
		{"a video deleted on its own is restored beside the photo where the plan moves it", group,
			[]domain.Op{move(photo, personal, trip+"/IMG_0001.HEIC", 11, "R-OBJECT-05")},
			map[domain.LinkID][]domain.LinkDecision{1: {dec(video, deleted, domain.ActionUpload, "R-COMPONENT-01")}},
			nil, nil, filesOf(photo),
			[]domain.Op{upload(2, domain.RoleLiveVideo, movie, personal, trip+"/IMG_0001.mov", 11, "R-COMPONENT-01")},
			nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			input := fmt.Sprintf("%+v %+v %+v %+v %+v %+v", c.target, c.mains, c.components, c.copies, c.after, c.files)
			names := plan.NewNames("alpha", scan, links)
			got, notices := plan.ComponentOps(names, c.target, c.mains, c.components, c.copies, c.after, c.files, now)
			if !reflect.DeepEqual(got, c.ops) || !reflect.DeepEqual(notices, c.notices) {
				t.Errorf("ComponentOps = %s; want %s", show(got, notices), show(c.ops, c.notices))
			}
			for _, o := range c.ops {
				if _, ok := domain.Rules[o.Rule]; !ok {
					t.Errorf("%s is not in domain.Rules", o.Rule)
				}
			}
			// Free numbers a name whose stem the names hold, so a destination
			// that they hold comes back changed.
			for _, o := range got {
				if again := names.Free(o.DstRoot, path.Dir(o.Dst), path.Base(o.Dst)); again == o.Dst {
					t.Errorf("the names of the run do not hold %s:%s", o.DstRoot, o.Dst)
				}
			}
			rm, rs, rl := slices.Clone(c.mains), slices.Clone(scan), slices.Clone(links)
			slices.Reverse(rm)
			slices.Reverse(rs)
			slices.Reverse(rl)
			rc, rp := map[domain.LinkID][]domain.LinkDecision{}, map[domain.LinkID][]domain.Placement{}
			for id, ds := range c.components {
				rc[id] = slices.Clone(ds)
				slices.Reverse(rc[id])
			}
			for id, ps := range c.copies {
				rp[id] = slices.Clone(ps)
				slices.Reverse(rp[id])
			}
			again, more := plan.ComponentOps(plan.NewNames("alpha", rs, rl), c.target, rm, rc, rp, c.after, c.files, now)
			if !reflect.DeepEqual(again, got) || !reflect.DeepEqual(more, notices) {
				t.Errorf("with the operations, the lists, the scan and the links reversed: %s; want %s", show(again, more), show(got, notices))
			}
			for _, o := range got {
				for i := range o.Links {
					o.Links[i].Path = "changed"
				}
			}
			if after := fmt.Sprintf("%+v %+v %+v %+v %+v %+v", c.target, c.mains, c.components, c.copies, c.after, c.files); after != input {
				t.Errorf("ComponentOps changed its input or shares a slice with it:\n%s\nwant\n%s", after, input)
			}
		})
	}
}
