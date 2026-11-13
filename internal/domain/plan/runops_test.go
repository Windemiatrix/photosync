package plan_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/match"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Each row is what a run reads from the scans of its remotes. The expected
// operations are written from the rules on new albums, imports, one import of
// a content and edits, not from the code, and must not depend on the order of
// the input.
func TestRunOps(t *testing.T) {
	t.Parallel()
	const (
		personal, shared = domain.LibraryPersonal, domain.LibraryShared
		holidays, trip   = "2024/05-01 - Holidays", "2024/05-02 - Trip"
		party, edits     = "2025/01-05 - Party", "2024/05-01 - Holidays/edited"
		inQuarantine     = ".photosync/quarantine/2026-11-13/"
	)
	type (
		reads   = map[string]plan.RemoteScan
		news    = []plan.NewFile
		entries = []domain.RemoteEntry
		specs   = []domain.RemoteSpec
	)
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	written := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	ignore := domain.DefaultPolicies()
	quarantine := ignore
	quarantine.EditedFiles = domain.EditedQuarantine

	spec := func(name string, mode domain.Mode, hash domain.HashKind) domain.RemoteSpec {
		return domain.RemoteSpec{Name: name, Mode: mode, Capabilities: domain.Capabilities{ContentHash: hash}}
	}
	alpha, alphaUp := spec("alpha", domain.ModeTwoWay, domain.HashMD5), spec("alpha", domain.ModeUploadOnly, domain.HashMD5)
	beta, betaUp, betaDown := spec("beta", domain.ModeTwoWay, domain.HashMD5), spec("beta", domain.ModeUploadOnly, domain.HashMD5), spec("beta", domain.ModeDownloadOnly, domain.HashMD5)
	plain := spec("beta", domain.ModeTwoWay, domain.HashNone)
	gamma, gammaDown := spec("gamma", domain.ModeTwoWay, domain.HashSHA256), spec("gamma", domain.ModeDownloadOnly, domain.HashMD5)

	event := func(id, parent domain.AlbumID, year, title string) domain.Album {
		return domain.Album{ID: id, Kind: domain.KindEvent, Title: title, ParentID: parent, FolderTitles: []string{year}}
	}
	trashed := event(11, 1, "2024", "05/02 Trip")
	trashed.InTrash = true
	added := []domain.Album{{ID: 21, Kind: domain.KindFolder, Title: "2025"}, event(22, 21, "2025", "01/05 Party")}
	albums := slices.Concat([]domain.Album{{ID: 1, Kind: domain.KindFolder, Title: "2024"}, event(10, 1, "2024", "05/01 Holidays"), trashed}, added)

	file := func(root domain.LibraryKind, at string, size int64, md5 string) domain.RemoteEntry {
		return domain.RemoteEntry{Root: root, Path: at, Size: size, MD5: md5, SHA256: "s" + md5, ETag: "e" + md5, MTime: written}
	}
	e1, e1v, e1r := file(personal, holidays+"/IMG_0001.HEIC", 100, "a1"), file(personal, holidays+"/IMG_0001.MOV", 30, "a2"), file(personal, holidays+"/IMG_0001.DNG", 200, "a3")
	e1v.MTime, e1r.MTime = written.Add(time.Minute), written.Add(2*time.Minute)
	e4, e5 := file(personal, party+"/IMG_0004.HEIC", 400, "d4"), file(shared, trip+"/IMG_0005.HEIC", 500, "e5")
	e6, e7 := file(personal, "2024/05-09 - Gone/IMG_0006.HEIC", 600, "e6"), file(personal, "2024/MVI_0007.AVI", 700, "e7")
	d9a, d9b, d9c := file(personal, holidays+"/IMG_0009.HEIC", 900, "d9"), file(personal, trip+"/IMG_0009.HEIC", 900, "d9"), file(personal, "2024/IMG_0009.HEIC", 900, "d9")
	// A remote may list only the hash of its own kind, so one content must be
	// told by that hash and never by the other.
	d9aMD5, d9bMD5, d9cSHA := d9a, d9b, d9c
	d9aMD5.SHA256, d9bMD5.SHA256, d9cSHA.MD5 = "", "", ""
	f8 := file(personal, trip+"/IMG_0008.HEIC", 800, "f8")
	xa, xb, xs := file(personal, edits+"/x.jpg", 50, "x1"), file(personal, edits+"/x (1).jpg", 60, "x2"), file(shared, edits+"/x.jpg", 70, "x3")
	shot := match.Candidate{Main: e1, Role: domain.RoleOriginal, Live: &e1v, RAW: &e1r, Sidecars: entries{file(personal, holidays+"/IMG_0001.XMP", 5, "a4")}}
	converted := match.Candidate{Main: e7, Role: domain.RoleSource}
	scans := map[string]entries{"alpha": {file(personal, inQuarantine+edits+"/x.jpg", 50, "x1")}}

	alone := func(e domain.RemoteEntry) match.Candidate { return match.Candidate{Main: e, Role: domain.RoleOriginal} }
	newFile := func(c match.Candidate, action domain.LinkAction, rule string, album domain.AlbumID) plan.NewFile {
		return plan.NewFile{
			File:     c,
			Decision: domain.LinkDecision{ICloud: domain.ChangeNone, Remote: domain.ChangeNew, Action: action, Rule: rule},
			AlbumID:  album,
		}
	}
	imported := func(c match.Candidate, album domain.AlbumID) plan.NewFile {
		return newFile(c, domain.ActionImport, "R-LINK-02", album)
	}
	reported := newFile(alone(d9a), domain.ActionNone, "R-MODE-01", 0)

	link := func(remote string, e domain.RemoteEntry, role domain.LinkRole, album domain.AlbumID) domain.Link {
		return domain.Link{
			Remote: remote, Root: e.Root, Path: e.Path, Role: role, AlbumID: album, Size: e.Size,
			SHA256: e.SHA256, MD5: e.MD5, ETag: e.ETag, MTime: e.MTime, Level: domain.LevelImported, Status: domain.StatusActive,
		}
	}
	importOf := func(remote string, album domain.AlbumID, main domain.RemoteEntry, links ...domain.Link) domain.Op {
		return domain.Op{Kind: domain.OpImport, Remote: remote, Root: main.Root, Src: main.Path, AlbumID: album, Rule: "R-LINK-02", Links: links}
	}
	single := func(remote string, album domain.AlbumID, e domain.RemoteEntry) domain.Op {
		return importOf(remote, album, e, link(remote, e, domain.RoleOriginal, album))
	}
	create := func(id domain.AlbumID) domain.Op {
		return domain.Op{Kind: domain.OpAlbumCreate, AlbumID: id, Rule: "R-LAYOUT-06"}
	}
	aside := func(remote string, e domain.RemoteEntry, name string) domain.Op {
		return domain.Op{
			Kind: domain.OpQuarantine, Remote: remote, Root: e.Root, Src: e.Path, DstRoot: e.Root, Dst: inQuarantine + edits + "/" + name,
			Rule: "R-STATUS-10",
		}
	}

	namesOf := func(remotes specs) map[string]*plan.Names {
		names := map[string]*plan.Names{}
		for _, r := range remotes {
			names[r.Name] = plan.NewNames(r.Name, scans[r.Name], nil)
		}
		return names
	}
	show := func(ops []domain.Op) string {
		var out []string
		for _, o := range ops {
			line := fmt.Sprintf("%s %s %s:%s -> %s:%s asset %d album %d %s irreversible=%t",
				o.Kind, o.Remote, o.Root, o.Src, o.DstRoot, o.Dst, o.Asset, o.AlbumID, o.Rule, o.Irreversible)
			for _, l := range o.Links {
				line += fmt.Sprintf(" %+v", l)
			}
			out = append(out, line)
		}
		return fmt.Sprintf("%q", out)
	}

	for _, c := range []struct {
		name    string
		read    reads
		added   []domain.Album
		albums  []domain.Album
		remotes specs
		p       domain.Policies
		want    []domain.Op
	}{
		{"nothing at all", nil, nil, nil, nil, quarantine, nil},
		{"a new file comes in with its Live video and RAW into the album of its directory, and its sidecar stays in the cloud",
			reads{"alpha": {NewFiles: news{imported(shot, 10)}}}, nil, albums, specs{alpha}, ignore,
			[]domain.Op{importOf("alpha", 10, e1,
				link("alpha", e1, domain.RoleOriginal, 10), link("alpha", e1v, domain.RoleLiveVideo, 10), link("alpha", e1r, domain.RoleRAWPair, 10))}},
		{"the albums that a run adds come first, a folder before its album, then the edits leave and the new files come in",
			reads{"alpha": {NewFiles: news{imported(alone(e4), 22)}, Edited: entries{xa}}}, []domain.Album{added[1], added[0]}, albums, specs{alpha}, quarantine,
			[]domain.Op{create(21), create(22), aside("alpha", xa, "x (1).jpg"), single("alpha", 22, e4)}},
		{"a new file goes into no album when its album is in Recently Deleted, gone or none, under either root and as a converted video",
			reads{"alpha": {NewFiles: news{imported(converted, 0), imported(alone(e5), 11), imported(alone(e6), 20)}}}, nil, albums, specs{alpha}, ignore,
			[]domain.Op{single("alpha", 0, e6), importOf("alpha", 0, e7, link("alpha", e7, domain.RoleSource, 0)), single("alpha", 0, e5)}},
		{"upload-only only reports a new file", reads{"alpha": {NewFiles: news{reported}}}, nil, albums, specs{alphaUp}, ignore, nil},
		{"of one content new in two remotes the first by name imports it and the other waits, download-only importing too",
			reads{"alpha": {NewFiles: news{imported(alone(d9aMD5), 10)}}, "beta": {NewFiles: news{imported(alone(f8), 0), imported(alone(d9bMD5), 0)}}},
			nil, albums, specs{betaDown, alpha}, ignore, []domain.Op{single("alpha", 10, d9aMD5), single("beta", 0, f8)}},
		{"a content that the remote first by name only reports comes in from the next one",
			reads{"alpha": {NewFiles: news{reported}}, "beta": {NewFiles: news{imported(alone(d9b), 0)}}},
			nil, albums, specs{beta, alphaUp}, ignore, []domain.Op{single("beta", 0, d9b)}},
		{"one content under hashes of different kinds comes in from each remote",
			reads{"alpha": {NewFiles: news{imported(alone(d9a), 10)}}, "gamma": {NewFiles: news{imported(alone(d9cSHA), 0)}}},
			nil, albums, specs{gamma, alpha}, ignore, []domain.Op{single("alpha", 10, d9a), single("gamma", 0, d9cSHA)}},
		{"without a hash every new file comes in",
			reads{"beta": {NewFiles: news{imported(alone(d9c), 0), imported(alone(d9b), 0)}}},
			nil, albums, specs{plain}, ignore, []domain.Op{single("beta", 0, d9b), single("beta", 0, d9c)}},
		{"under edited_files quarantine the edits of both roots leave without a link, in the order of their paths", // covers: R-STATUS-10
			reads{"alpha": {Edited: entries{xs, xa, xb}}}, nil, albums, specs{alpha}, quarantine,
			[]domain.Op{aside("alpha", xb, "x (1).jpg"), aside("alpha", xa, "x (2).jpg"), aside("alpha", xs, "x.jpg")}},
		{"edits leave two-way and upload-only remotes, each by the names of its own scan, and stay in download-only",
			reads{"alpha": {Edited: entries{xa}}, "beta": {Edited: entries{xa}}, "gamma": {Edited: entries{xa}}},
			nil, albums, specs{gammaDown, betaUp, alpha}, quarantine, []domain.Op{aside("alpha", xa, "x (1).jpg"), aside("beta", xa, "x.jpg")}},
		{"under edited_files ignore the edits stay", reads{"alpha": {Edited: entries{xs, xa, xb}}}, nil, albums, specs{alpha}, ignore, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			input := fmt.Sprintf("%+v %+v %+v %+v %+v %+v %+v", c.read, c.added, c.albums, c.remotes, c.p, *shot.Live, *shot.RAW)
			got := plan.RunOps(c.read, c.added, c.albums, namesOf(c.remotes), c.remotes, c.p, now)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("RunOps = %s; want %s", show(got), show(c.want))
			}
			for _, o := range c.want {
				if _, ok := domain.Rules[o.Rule]; !ok {
					t.Errorf("%s is not in domain.Rules", o.Rule)
				}
			}
			back := reads{}
			for name, s := range c.read {
				s.NewFiles, s.Edited = slices.Clone(s.NewFiles), slices.Clone(s.Edited)
				slices.Reverse(s.NewFiles)
				slices.Reverse(s.Edited)
				back[name] = s
			}
			ra, rl, rm := slices.Clone(c.added), slices.Clone(c.albums), slices.Clone(c.remotes)
			slices.Reverse(ra)
			slices.Reverse(rl)
			slices.Reverse(rm)
			if again := plan.RunOps(back, ra, rl, namesOf(rm), rm, c.p, now); !reflect.DeepEqual(again, got) {
				t.Errorf("with the input reversed: %s; want %s", show(again), show(got))
			}
			for _, o := range got {
				for i := range o.Links {
					o.Links[i].Path = "changed"
				}
			}
			if after := fmt.Sprintf("%+v %+v %+v %+v %+v %+v %+v", c.read, c.added, c.albums, c.remotes, c.p, *shot.Live, *shot.RAW); after != input {
				t.Errorf("RunOps changed its input or shares a slice with it:\n%s\nwant\n%s", after, input)
			}
		})
	}
}
