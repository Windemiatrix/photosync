package plan_test

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/template"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/pathtemplate"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// A run moves, copies, uploads and quarantines the files of an asset in the
// directories of its albums under names that no file of the cloud or of the
// run takes. The expected operations follow the rules of the layout and of
// the names, not the code, and must not depend on the order of the
// decisions, the scan, the links, the albums and the pairs.
func TestFileOps(t *testing.T) {
	t.Parallel()
	const (
		personal, shared          = domain.LibraryPersonal, domain.LibraryShared
		holidays, trip            = "2024/05-01 - Holidays", "2024/05-02 - Trip"
		party, cats, day          = "2024/Party time", "Albums/Cats", "2024/05-04"
		inQuarantine              = ".photosync/quarantine/2026-11-13/"
		event, everyAlbum         = "{event_path}/IMG_0001", "{album}/IMG_0001"
		twoWay                    = domain.ModeTwoWay
		quarantine                = domain.ActionQuarantineFile
		unchanged, moved, deleted = domain.ChangeUnchanged, domain.ChangeMoved, domain.ChangeDeleted
	)
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	p := domain.DefaultPolicies()
	p.Deletes, p.UntitledEvent, p.AlbumsRoot = domain.DeletesQuarantine, "Untitled", "Albums"
	fields := templateFields{"album": {Strings: []string{"Holidays", "Trips"}}}
	asset := domain.Asset{
		ID: 7, Key: domain.AssetKey{UUID: "uuid-7"}, Library: personal, TargetLibrary: personal,
		Taken:     time.Date(2024, 5, 4, 12, 0, 0, 0, time.UTC),
		Resources: []domain.Resource{{Role: domain.ResourceOriginal, Filename: "IMG_0001.HEIC", Size: 100, SHA256: "s7", MD5: "m7"}},
	}
	inShared := asset
	inShared.TargetLibrary = shared
	bare := asset
	bare.Resources = nil
	albums := []domain.Album{
		{ID: 1, Kind: domain.KindFolder, Title: "2024"},
		{ID: 10, Kind: domain.KindEvent, Title: "05/01 Holidays", FolderTitles: []string{"2024"}},
		{ID: 11, Kind: domain.KindEvent, Title: "05/02 Trip", FolderTitles: []string{"2024"}},
		{ID: 12, Kind: domain.KindEvent, Title: "05/03 Party", FolderTitles: []string{"2024"}},
		{ID: 13, Kind: domain.KindThematic, Title: "Cats"},
	}
	pairs := []domain.AlbumPair{
		{AlbumID: 11, Remote: "beta", Root: personal, DirPath: "Elsewhere/Trip", CreatedBy: domain.OriginRemote},
		{AlbumID: 11, Remote: "alpha", Root: shared, DirPath: "Shared/Trip", CreatedBy: domain.OriginRemote},
		{AlbumID: 12, Remote: "alpha", Root: personal, DirPath: party, CreatedBy: domain.OriginRemote},
	}
	link := func(id domain.LinkID, root domain.LibraryKind, at string, album domain.AlbumID, md5 string) domain.Link {
		return domain.Link{
			ID: id, AssetID: 7, Remote: "alpha", Root: root, Path: at, Role: domain.RoleOriginal, AlbumID: album,
			Size: 100, MD5: md5, Level: domain.LevelExact, Status: domain.StatusActive,
		}
	}
	photo := link(1, personal, holidays+"/IMG_0001.HEIC", 10, "m1")
	stray := link(2, personal, "Misc/IMG_0002.HEIC", 0, "m2")
	needless := link(4, shared, "Misc/IMG_0001.HEIC", 0, "m1")
	first := link(5, personal, "Misc/x.jpg", 0, "m5")
	second := link(6, personal, "Misc/x (1).jpg", 0, "m6")
	links := []domain.Link{photo, stray, needless, first, second, {
		ID: 9, AssetID: 9, Remote: "alpha", Root: personal, Path: "Misc/x.jpg", Role: domain.RoleOriginal,
		Status: domain.StatusQuarantinedRemote, QuarantinePath: inQuarantine + "Misc/x.jpg",
	}}
	scan := []domain.RemoteEntry{
		{Root: personal, Path: holidays + "/IMG_0001.HEIC", Size: 100, MD5: "m1"},
		{Root: personal, Path: "Misc/IMG_0002.HEIC", Size: 100, MD5: "m2"},
		{Root: personal, Path: "Misc/x.jpg", Size: 100, MD5: "m5"},
		{Root: personal, Path: "Misc/x (1).jpg", Size: 100, MD5: "m6"},
		{Root: personal, Path: trip + "/IMG_0002.jpg", Size: 50},
		{Root: personal, Path: party + "/IMG_0002.jpg", Size: 50},
		{Root: personal, Path: inQuarantine + holidays + "/IMG_0001.HEIC", Size: 100},
		{Root: personal, Path: inQuarantine + "Misc/IMG_0001.HEIC", Size: 100},
		{Root: shared, Path: "Misc/IMG_0001.HEIC", Size: 100, MD5: "m1"},
		{Root: shared, Path: "Moved/IMG_0001.HEIC", Size: 100, MD5: "m1"},
	}
	dec := func(l domain.Link, icloud, remote domain.ChangeKind, a domain.LinkAction, rule string) domain.LinkDecision {
		return domain.LinkDecision{Link: l, ICloud: icloud, Remote: remote, Action: a, Rule: rule}
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
	relinked := func(l domain.Link, id domain.LinkID, root domain.LibraryKind, at string, album domain.AlbumID) domain.Link {
		l.ID, l.Root, l.Path, l.AlbumID = id, root, at, album
		return l
	}
	held := func(l domain.Link, status domain.LinkStatus, at string) domain.Link {
		l.Status, l.QuarantinePath = status, at
		return l
	}
	uploaded := func(root domain.LibraryKind, at string, album domain.AlbumID) domain.Link {
		return domain.Link{
			AssetID: 7, Remote: "alpha", Root: root, Path: at, Role: domain.RoleOriginal, AlbumID: album,
			Size: 100, SHA256: "s7", MD5: "m7", Level: domain.LevelUploaded, Status: domain.StatusActive,
		}
	}
	op := func(k domain.OpKind, root domain.LibraryKind, src string, dstRoot domain.LibraryKind, dst, rule string, l domain.Link) domain.Op {
		return domain.Op{Kind: k, Remote: "alpha", Root: root, Src: src, DstRoot: dstRoot, Dst: dst, Asset: 7, Rule: rule, Links: []domain.Link{l}}
	}
	paired := func(album domain.AlbumID, root domain.LibraryKind, dir string) domain.AlbumPair {
		return domain.AlbumPair{AlbumID: album, Remote: "alpha", Root: root, DirPath: dir, CreatedBy: domain.OriginICloud}
	}
	show := func(xs []domain.Op, ps []domain.AlbumPair, ns []domain.Notice, err error) string {
		var out []string
		for _, o := range xs {
			line := fmt.Sprintf("%s %s %s:%s -> %s:%s asset %d %s", o.Kind, o.Remote, o.Root, o.Src, o.DstRoot, o.Dst, o.Asset, o.Rule)
			for _, l := range o.Links {
				line += fmt.Sprintf(" [%d %s %s:%s album %d %s %s %d %s %s q=%s]",
					l.ID, l.Role, l.Root, l.Path, l.AlbumID, l.Level, l.Status, l.Size, l.SHA256, l.MD5, l.QuarantinePath)
			}
			out = append(out, line)
		}
		for _, x := range ps {
			out = append(out, fmt.Sprintf("pair %d %s %s:%s %s", x.AlbumID, x.Remote, x.Root, x.DirPath, x.CreatedBy))
		}
		for _, n := range ns {
			out = append(out, n.Code+" "+strings.Join(n.Params, "|"))
		}
		return fmt.Sprintf("%q, %v", out, err)
	}
	for _, c := range []struct {
		name       string
		mode       domain.Mode
		tmpl       string
		target     domain.Asset
		decisions  []domain.LinkDecision
		placements []domain.Placement
		files      map[domain.LinkID]domain.RemoteEntry
		ops        []domain.Op
		made       []domain.AlbumPair
		notice     string // a part of the error that the notice holds, "" for no notice
		err        error
	}{
		{"a decision gives an operation here only to quarantine a file, and the placements carry out the others", twoWay, event, asset,
			[]domain.LinkDecision{dec(photo, unchanged, unchanged, domain.ActionNone, ""), dec(stray, moved, unchanged, domain.ActionMoveFile, "R-LINK-04")},
			nil, filesOf(photo, stray), nil, nil, "", nil},
		{"a file moves into the directory of its event, and the album pairs with it", twoWay, event, asset, nil,
			[]domain.Placement{place(domain.ActionMoveFile, stray, 10, "R-OBJECT-05")}, nil,
			[]domain.Op{op(domain.OpMove, personal, "Misc/IMG_0002.HEIC", personal, holidays+"/IMG_0002.HEIC", "R-OBJECT-05",
				relinked(stray, 2, personal, holidays+"/IMG_0002.HEIC", 10))},
			[]domain.AlbumPair{paired(10, personal, holidays)}, "", nil},
		{"a stem that a file of the user takes gives the next number, and the pairs of another remote and another root do not count",
			twoWay, event, asset, nil,
			[]domain.Placement{place(domain.ActionMoveFile, stray, 11, "R-OBJECT-05")}, nil,
			[]domain.Op{op(domain.OpMove, personal, "Misc/IMG_0002.HEIC", personal, trip+"/IMG_0002 (1).HEIC", "R-OBJECT-05",
				relinked(stray, 2, personal, trip+"/IMG_0002 (1).HEIC", 11))},
			[]domain.AlbumPair{paired(11, personal, trip)}, "", nil},
		{"the pair of an album gives its directory, whose names are taken as any other", twoWay, event, asset, nil,
			[]domain.Placement{place(domain.ActionMoveFile, stray, 12, "R-OBJECT-05")}, nil,
			[]domain.Op{op(domain.OpMove, personal, "Misc/IMG_0002.HEIC", personal, party+"/IMG_0002 (1).HEIC", "R-OBJECT-05",
				relinked(stray, 2, personal, party+"/IMG_0002 (1).HEIC", 12))},
			nil, "", nil},
		{"a move to the other library takes the file to the other root, where its name is free, after a copy of it there",
			twoWay, event, inShared, nil,
			[]domain.Placement{place(domain.ActionMoveFile, photo, 10, "R-OBJECT-07"), place(domain.ActionCopyFile, photo, 11, "R-OBJECT-03")}, nil,
			[]domain.Op{
				op(domain.OpCopy, personal, holidays+"/IMG_0001.HEIC", shared, "Shared/Trip/IMG_0001.HEIC", "R-OBJECT-03",
					relinked(photo, 0, shared, "Shared/Trip/IMG_0001.HEIC", 11)),
				op(domain.OpMove, personal, holidays+"/IMG_0001.HEIC", shared, holidays+"/IMG_0001.HEIC", "R-OBJECT-07",
					relinked(photo, 1, shared, holidays+"/IMG_0001.HEIC", 10)),
			},
			[]domain.AlbumPair{paired(10, shared, holidays)}, "", nil},
		{"an upload puts the original into the directory of each album, with the names of its root", twoWay, event, inShared, nil,
			[]domain.Placement{place(domain.ActionUpload, domain.Link{}, 10, "R-LINK-10"), place(domain.ActionUpload, domain.Link{}, 13, "R-LINK-10")}, nil,
			[]domain.Op{
				op(domain.OpPut, "", "", shared, holidays+"/IMG_0001.HEIC", "R-LINK-10", uploaded(shared, holidays+"/IMG_0001.HEIC", 10)),
				op(domain.OpPut, "", "", shared, cats+"/IMG_0001.HEIC", "R-LINK-10", uploaded(shared, cats+"/IMG_0001.HEIC", 13)),
			},
			[]domain.AlbumPair{paired(10, shared, holidays), paired(13, shared, cats)}, "", nil},
		{"an asset in no album goes up to the day folder of its date, which pairs with no album", twoWay, event, asset, nil,
			[]domain.Placement{place(domain.ActionUpload, domain.Link{}, 0, "R-LINK-01")}, nil,
			[]domain.Op{op(domain.OpPut, "", "", personal, day+"/IMG_0001.HEIC", "R-LINK-01", uploaded(personal, day+"/IMG_0001.HEIC", 0))},
			nil, "", nil},
		{"upload-only: a template of several values puts a file at each of its paths", domain.ModeUploadOnly, everyAlbum, asset, nil,
			[]domain.Placement{place(domain.ActionUpload, domain.Link{}, 10, "R-LINK-01")}, nil,
			[]domain.Op{
				op(domain.OpPut, "", "", personal, "Holidays/IMG_0001.HEIC", "R-LINK-01", uploaded(personal, "Holidays/IMG_0001.HEIC", 10)),
				op(domain.OpPut, "", "", personal, "Trips/IMG_0001.HEIC", "R-LINK-01", uploaded(personal, "Trips/IMG_0001.HEIC", 10)),
			},
			[]domain.AlbumPair{paired(10, personal, holidays)}, "", nil},
		{"two-way: a template that does not fit the mode leaves the asset out with a notice, and the album still pairs with its directory",
			twoWay, everyAlbum, asset, nil,
			[]domain.Placement{place(domain.ActionUpload, domain.Link{}, 10, "R-LINK-01")}, nil,
			nil, []domain.AlbumPair{paired(10, personal, holidays)}, "does not fit the mode", nil},
		{"an unknown field stops the run with no operations and no pairs", twoWay, "{event_path}/{foo}", asset, nil,
			[]domain.Placement{place(domain.ActionUpload, domain.Link{}, 10, "R-LINK-01")}, nil,
			nil, nil, "", pathtemplate.ErrUnknownField},
		{"under quarantine the files of an asset gone from iCloud go to quarantine under free paths, in the order of their links",
			twoWay, event, domain.Asset{},
			[]domain.LinkDecision{
				dec(second, deleted, unchanged, quarantine, "R-STATUS-01"), dec(photo, deleted, unchanged, quarantine, "R-STATUS-01"),
				dec(first, deleted, unchanged, quarantine, "R-STATUS-01"),
			},
			nil, filesOf(second, photo, first),
			[]domain.Op{
				op(domain.OpQuarantine, personal, holidays+"/IMG_0001.HEIC", personal, inQuarantine+holidays+"/IMG_0001 (1).HEIC", "R-STATUS-01",
					held(photo, domain.StatusQuarantinedRemote, inQuarantine+holidays+"/IMG_0001 (1).HEIC")),
				op(domain.OpQuarantine, personal, "Misc/x (1).jpg", personal, inQuarantine+"Misc/x (1) (1).jpg", "R-STATUS-01",
					held(second, domain.StatusQuarantinedRemote, inQuarantine+"Misc/x (1) (1).jpg")),
				op(domain.OpQuarantine, personal, "Misc/x.jpg", personal, inQuarantine+"Misc/x (1).jpg", "R-STATUS-01",
					held(first, domain.StatusQuarantinedRemote, inQuarantine+"Misc/x (1).jpg")),
			},
			nil, "", nil},
		{"a needless copy goes to quarantine in its own root and leaves the state", twoWay, event, asset, nil,
			[]domain.Placement{place(quarantine, needless, 0, "R-OBJECT-04")}, filesOf(needless),
			[]domain.Op{op(domain.OpQuarantine, shared, "Misc/IMG_0001.HEIC", shared, inQuarantine+"Misc/IMG_0001.HEIC", "R-OBJECT-04",
				held(needless, "", inQuarantine+"Misc/IMG_0001.HEIC"))},
			nil, "", nil},
		{"download-only: nothing goes to the cloud", domain.ModeDownloadOnly, event, asset,
			[]domain.LinkDecision{dec(photo, deleted, unchanged, domain.ActionNone, "R-MODE-02")}, nil, filesOf(photo), nil, nil, "", nil},
		{"nothing at all", twoWay, event, domain.Asset{}, nil, nil, nil, nil, nil, "", nil},
		{"upload-only: a file that the user moved goes to quarantine from where it lies, and its link takes that place",
			domain.ModeUploadOnly, event, domain.Asset{},
			[]domain.LinkDecision{dec(photo, deleted, moved, quarantine, "R-STATUS-01")}, nil,
			map[domain.LinkID]domain.RemoteEntry{1: {Root: shared, Path: "Moved/IMG_0001.HEIC", Size: 100, MD5: "m1"}},
			[]domain.Op{op(domain.OpQuarantine, shared, "Moved/IMG_0001.HEIC", shared, inQuarantine+"Moved/IMG_0001.HEIC", "R-STATUS-01",
				held(relinked(photo, 1, shared, "Moved/IMG_0001.HEIC", 10), domain.StatusQuarantinedRemote, inQuarantine+"Moved/IMG_0001.HEIC"))},
			nil, "", nil},
		{"an asset without an original goes up nowhere with a notice, and the album still pairs with its directory", twoWay, event, bare, nil,
			[]domain.Placement{place(domain.ActionUpload, domain.Link{}, 10, "R-LINK-01")}, nil,
			nil, []domain.AlbumPair{paired(10, personal, holidays)}, "has no original file", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			tmpl, err := template.Parse(c.tmpl)
			if err != nil {
				t.Fatalf("template.Parse(%q): %v", c.tmpl, err)
			}
			r := domain.RemoteSpec{Name: "alpha", Mode: c.mode}
			input := fmt.Sprintf("%+v %+v %+v %+v %+v %+v %+v", c.decisions, c.placements, c.files, albums, pairs, scan, links)
			got, made, notices, err := plan.FileOps(plan.NewNames("alpha", scan, links), r, c.target, c.decisions, c.placements,
				c.files, albums, pairs, tmpl, fields, p, now)
			noticed := len(notices) == 0
			if c.notice != "" {
				noticed = len(notices) == 1 && notices[0].Code == "plan.no_path" && len(notices[0].Params) == 3 &&
					notices[0].Params[0] == "alpha" && notices[0].Params[1] == "uuid-7" && strings.Contains(notices[0].Params[2], c.notice)
			}
			if !reflect.DeepEqual(got, c.ops) || !slices.Equal(made, c.made) || !noticed || !errors.Is(err, c.err) {
				t.Errorf("FileOps = %s; want %s with a notice with %q", show(got, made, notices, err), show(c.ops, c.made, nil, c.err), c.notice)
			}
			for _, o := range c.ops {
				if _, ok := domain.Rules[o.Rule]; !ok {
					t.Errorf("%s is not in domain.Rules", o.Rule)
				}
			}
			rd, ra, rp, rs, rl := slices.Clone(c.decisions), slices.Clone(albums), slices.Clone(pairs), slices.Clone(scan), slices.Clone(links)
			slices.Reverse(rd)
			slices.Reverse(ra)
			slices.Reverse(rp)
			slices.Reverse(rs)
			slices.Reverse(rl)
			again, also, more, failed := plan.FileOps(plan.NewNames("alpha", rs, rl), r, c.target, rd, c.placements, c.files, ra, rp, tmpl, fields, p, now)
			if !reflect.DeepEqual(again, got) || show(nil, also, more, failed) != show(nil, made, notices, err) {
				t.Errorf("with the decisions, the scan, the links, the albums and the pairs reversed: %s; want %s",
					show(again, also, more, failed), show(got, made, notices, err))
			}
			for _, o := range got {
				for i := range o.Links {
					o.Links[i].Path = "changed"
				}
			}
			for i := range made {
				made[i].DirPath = "changed"
			}
			if after := fmt.Sprintf("%+v %+v %+v %+v %+v %+v %+v", c.decisions, c.placements, c.files, albums, pairs, scan, links); after != input {
				t.Errorf("FileOps changed its input or shares a slice with it:\n%s\nwant\n%s", after, input)
			}
		})
	}
}
