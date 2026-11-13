package plan_test

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/match"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Each row is what matching gives for one remote, the links that it makes and
// the files that no asset holds, against the album pairs of the run. The
// albums, joins and decisions are written from the rules on new links and new
// files in each mode and root, not from the code, and must not depend on the
// order of the input.
func TestJoinFresh(t *testing.T) {
	t.Parallel()
	const (
		personal, shared         = domain.LibraryPersonal, domain.LibraryShared
		twoWay, upOnly, downOnly = domain.ModeTwoWay, domain.ModeUploadOnly, domain.ModeDownloadOnly
		byMD5, bySHA, noHash     = domain.HashMD5, domain.HashSHA256, domain.HashNone
		original, source         = domain.RoleOriginal, domain.RoleSource
		liveVideo, rawPair       = domain.RoleLiveVideo, domain.RoleRAWPair
		exact, component         = domain.LevelExact, domain.LevelPairComponent
		pairRule, copyRule       = "R-LINK-03", "R-LAYOUT-08"
		holidays, trip, party    = "2024/05-01 - Holidays", "2024/05-02 - Trip", "2024/05-03 - Party"
		old, zoo                 = "2024/05-04 - Old", "2024/05-05 - Zoo"
	)
	file := func(root domain.LibraryKind, at, md5 string) domain.RemoteEntry {
		return domain.RemoteEntry{Root: root, Path: at, Size: 100, MD5: md5, SHA256: "s" + md5, ETag: "e" + md5}
	}
	made := func(id domain.LinkID, asset domain.AssetID, e domain.RemoteEntry, role domain.LinkRole, level domain.MatchLevel, rule string) domain.LinkDecision {
		return domain.LinkDecision{
			Link: domain.Link{
				ID: id, AssetID: asset, Remote: "disk", Root: e.Root, Path: e.Path, Role: role,
				Size: e.Size, SHA256: e.SHA256, MD5: e.MD5, ETag: e.ETag, Level: level, Status: domain.StatusActive,
			},
			ICloud: domain.ChangeNew, Remote: domain.ChangeNew, Action: domain.ActionLink, Rule: rule,
		}
	}
	in := func(d domain.LinkDecision, album domain.AlbumID) domain.LinkDecision {
		d.Link.AlbumID = album
		return d
	}
	pair := func(album domain.AlbumID, remote string, root domain.LibraryKind, dir string) domain.AlbumPair {
		return domain.AlbumPair{AlbumID: album, Remote: remote, Root: root, DirPath: dir, CreatedBy: domain.OriginRemote}
	}
	pairs := []domain.AlbumPair{
		pair(10, "disk", personal, holidays), pair(11, "disk", personal, trip), pair(31, "other", personal, trip),
		pair(12, "disk", personal, party), pair(20, "disk", shared, holidays), pair(21, "disk", shared, trip),
		pair(22, "disk", shared, party), pair(30, "other", personal, old),
	}
	alone := func(e domain.RemoteEntry) match.Candidate { return match.Candidate{Main: e, Role: original} }
	importing := domain.LinkDecision{ICloud: domain.ChangeNone, Remote: domain.ChangeNew, Action: domain.ActionImport, Rule: "R-LINK-02"}
	reporting := domain.LinkDecision{ICloud: domain.ChangeNone, Remote: domain.ChangeNew, Action: domain.ActionNone, Rule: "R-MODE-01"}
	imported := func(c match.Candidate, album domain.AlbumID) plan.NewFile {
		return plan.NewFile{File: c, Decision: importing, AlbumID: album}
	}
	reported := func(c match.Candidate) plan.NewFile { return plan.NewFile{File: c, Decision: reporting} }
	ch := func(album domain.AlbumID, rule string) domain.AlbumChange {
		return domain.AlbumChange{AlbumID: album, Rule: rule}
	}
	type joins = map[domain.AssetID][]domain.AlbumChange
	decs := func(d ...domain.LinkDecision) []domain.LinkDecision { return d }
	chs := func(c ...domain.AlbumChange) []domain.AlbumChange { return c }
	cands := func(c ...match.Candidate) []match.Candidate { return c }
	nfs := func(n ...plan.NewFile) []plan.NewFile { return n }
	p, noAlbums := domain.DefaultPolicies(), domain.DefaultPolicies()
	noAlbums.AlbumsForPersonal = false

	heic, heic1 := file(personal, holidays+"/IMG_0001.HEIC", "a1"), file(personal, holidays+"/IMG_0001 (1).HEIC", "a1")
	mov, dng := file(personal, holidays+"/IMG_0001.MOV", "c1"), file(personal, holidays+"/IMG_0001.DNG", "d1")
	copyShared, pairShared := file(shared, trip+"/IMG_0001.HEIC", "a1"), file(shared, party+"/IMG_0002.HEIC", "b2")
	copyPersonal := file(personal, trip+"/IMG_0002.HEIC", "b2")
	d11, d12 := made(11, 7, heic, original, exact, pairRule), made(12, 7, copyShared, original, exact, copyRule)
	d13, d14 := made(13, 8, pairShared, original, exact, pairRule), made(14, 8, copyPersonal, original, exact, copyRule)
	linkedAll := decs(in(d11, 10), in(d12, 21), in(d13, 22), in(d14, 11))
	joinedAll := joins{7: chs(ch(10, pairRule), ch(21, copyRule)), 8: chs(ch(11, copyRule), ch(22, pairRule))}
	joinedShared := joins{7: chs(ch(21, copyRule)), 8: chs(ch(22, pairRule))}
	newPersonal, newShared := file(personal, party+"/IMG_0003.HEIC", "c3"), file(shared, holidays+"/IMG_0004.HEIC", "c4")
	inZoo, atTop := file(personal, zoo+"/IMG_0005.HEIC", "e5"), file(personal, "IMG_0007.HEIC", "g7")
	inOld, newOld := file(personal, old+"/IMG_0005.HEIC", "e5"), file(personal, old+"/IMG_0015.HEIC", "m1")
	inBest, newBest := file(personal, holidays+"/Best/IMG_0006.HEIC", "f6"), file(personal, holidays+"/Best/IMG_0016.HEIC", "n1")
	sameHolidays, sameTrip := file(personal, holidays+"/IMG_0008.HEIC", "h8"), file(personal, trip+"/IMG_0008.HEIC", "h8")
	sameParty, sameShared := file(personal, party+"/IMG_0008.HEIC", "h8"), file(shared, holidays+"/IMG_0008.HEIC", "h8")
	otherParty := file(personal, party+"/IMG_0009.HEIC", "i9")
	shaHolidays, shaTrip, shaParty := file(personal, holidays+"/IMG_0010.HEIC", "x1"), file(personal, trip+"/IMG_0012.HEIC", "z1"), file(personal, party+"/IMG_0011.HEIC", "x1")
	shaHolidays.SHA256, shaTrip.SHA256, shaParty.SHA256 = "s1", "s1", "s2"
	still, live, raw := file(personal, holidays+"/IMG_0013.HEIC", "k1"), file(personal, holidays+"/IMG_0013.MOV", "k2"), file(personal, holidays+"/IMG_0013.DNG", "k3")
	xmp, avi := file(personal, holidays+"/IMG_0013.XMP", "k4"), file(personal, holidays+"/MVI_0014.AVI", "k5")
	shot := match.Candidate{Main: still, Role: original, Live: &live, RAW: &raw, Sidecars: []domain.RemoteEntry{xmp}}
	converted := match.Candidate{Main: avi, Role: source}

	show := func(ls []domain.LinkDecision, js map[domain.AssetID][]domain.AlbumChange, ns []plan.NewFile) string {
		var out []string
		for _, d := range ls {
			out = append(out, fmt.Sprintf("%+v %s/%s %s %s", d.Link, d.ICloud, d.Remote, d.Action, d.Rule))
		}
		for _, id := range slices.Sorted(maps.Keys(js)) {
			out = append(out, fmt.Sprintf("join %d %v", id, js[id]))
		}
		for _, n := range ns {
			s := fmt.Sprintf("new %s:%s %s %s #%d %s/%s %s %s", n.File.Main.Root, n.File.Main.Path, n.File.Main.MD5, n.File.Role,
				n.AlbumID, n.Decision.ICloud, n.Decision.Remote, n.Decision.Action, n.Decision.Rule)
			for _, e := range []*domain.RemoteEntry{n.File.Live, n.File.RAW} {
				if e != nil {
					s += " with " + e.Path
				}
			}
			for _, e := range n.File.Sidecars {
				s += " sidecar " + e.Path
			}
			out = append(out, s)
		}
		return fmt.Sprintf("%q", out)
	}
	same := func(ls []domain.LinkDecision, js map[domain.AssetID][]domain.AlbumChange, ns []plan.NewFile, wl []domain.LinkDecision, wj map[domain.AssetID][]domain.AlbumChange, wn []plan.NewFile) bool {
		return slices.Equal(ls, wl) && maps.EqualFunc(js, wj, func(x, y []domain.AlbumChange) bool { return slices.Equal(x, y) }) && reflect.DeepEqual(ns, wn)
	}
	for _, c := range []struct {
		name    string
		mode    domain.Mode
		hash    domain.HashKind
		p       domain.Policies
		matched []domain.LinkDecision
		left    []match.Candidate
		linked  []domain.LinkDecision
		join    joins
		files   []plan.NewFile
	}{
		{"nothing new", twoWay, byMD5, p, nil, nil, nil, nil, nil},
		{"two-way: a link takes the album of its directory, its asset joins it and a new file is imported into it, under either root",
			twoWay, byMD5, p, decs(d13, d11, d14, d12), cands(alone(newShared), alone(newPersonal)),
			linkedAll, joinedAll, nfs(imported(alone(newPersonal), 12), imported(alone(newShared), 20))},
		{"two-way with albums_for_personal false: nothing under the personal root joins an album", // covers: R-MODE-04
			twoWay, byMD5, noAlbums, decs(d13, d11, d14, d12), cands(alone(newShared), alone(newPersonal)),
			linkedAll, joinedShared, nfs(imported(alone(newPersonal), 0), imported(alone(newShared), 20))},
		{"download-only: as two-way",
			downOnly, byMD5, p, decs(d13, d11, d14, d12), cands(alone(newShared), alone(newPersonal)),
			linkedAll, joinedAll, nfs(imported(alone(newPersonal), 12), imported(alone(newShared), 20))},
		{"download-only with albums_for_personal false: as two-way",
			downOnly, byMD5, noAlbums, decs(d13, d11, d14, d12), cands(alone(newShared), alone(newPersonal)),
			linkedAll, joinedShared, nfs(imported(alone(newPersonal), 0), imported(alone(newShared), 20))},
		{"upload-only: links keep the albums of their directories, but nothing joins one and new files are only reported",
			upOnly, byMD5, p, decs(d13, d11, d14, d12), cands(alone(newShared), alone(newPersonal)),
			linkedAll, nil, nfs(reported(alone(newPersonal)), reported(alone(newShared)))},
		{"upload-only with albums_for_personal false: the same",
			upOnly, byMD5, noAlbums, decs(d13, d11, d14, d12), cands(alone(newShared), alone(newPersonal)),
			linkedAll, nil, nfs(reported(alone(newPersonal)), reported(alone(newShared)))},
		{"a directory without a pair and the top of the root give no album", twoWay, byMD5, p,
			decs(made(11, 7, inZoo, original, exact, pairRule)), cands(alone(atTop)),
			decs(made(11, 7, inZoo, original, exact, pairRule)), nil, nfs(imported(alone(atTop), 0))},
		{"a directory that only another remote pairs gives no album", twoWay, byMD5, p,
			decs(made(11, 7, inOld, original, exact, pairRule)), cands(alone(newOld)),
			decs(made(11, 7, inOld, original, exact, pairRule)), nil, nfs(imported(alone(newOld), 0))},
		{"a subdirectory of a paired directory gives no album", twoWay, byMD5, p,
			decs(made(11, 7, inBest, original, exact, pairRule)), cands(alone(newBest)),
			decs(made(11, 7, inBest, original, exact, pairRule)), nil, nfs(imported(alone(newBest), 0))},
		{"a Live video and a RAW lie in the album of their main file, which the asset joins once", twoWay, byMD5, p,
			decs(made(13, 7, dng, rawPair, component, pairRule), made(11, 7, heic, original, exact, pairRule), made(12, 7, mov, liveVideo, component, pairRule)),
			nil,
			decs(in(made(11, 7, heic, original, exact, pairRule), 10), in(made(12, 7, mov, liveVideo, component, pairRule), 10),
				in(made(13, 7, dng, rawPair, component, pairRule), 10)),
			joins{7: chs(ch(10, pairRule))}, nil},
		{"a pair and a copy in one album join it under both rules", twoWay, byMD5, p,
			decs(made(12, 7, heic1, original, exact, copyRule), made(11, 7, heic, original, exact, pairRule)), nil,
			decs(in(made(11, 7, heic, original, exact, pairRule), 10), in(made(12, 7, heic1, original, exact, copyRule), 10)),
			joins{7: chs(ch(10, copyRule), ch(10, pairRule))}, nil},
		{"of new files with one content only the first by path goes, the others wait", twoWay, byMD5, p,
			nil, cands(alone(sameTrip), alone(sameHolidays), alone(otherParty)),
			nil, nil, nfs(imported(alone(sameHolidays), 10), imported(alone(otherParty), 12))},
		{"of new files with one content under both roots the one under the personal root goes", twoWay, byMD5, p,
			nil, cands(alone(sameShared), alone(sameParty)),
			nil, nil, nfs(imported(alone(sameParty), 12))},
		{"upload-only reports one content once", upOnly, byMD5, p,
			nil, cands(alone(sameTrip), alone(sameHolidays)),
			nil, nil, nfs(reported(alone(sameHolidays)))},
		{"without a hash every new file goes", twoWay, noHash, p,
			nil, cands(alone(sameTrip), alone(sameHolidays)),
			nil, nil, nfs(imported(alone(sameHolidays), 10), imported(alone(sameTrip), 11))},
		{"one content is by the hash of the kind that the remote gives", twoWay, bySHA, p,
			nil, cands(alone(shaParty), alone(shaTrip), alone(shaHolidays)),
			nil, nil, nfs(imported(alone(shaHolidays), 10), imported(alone(shaParty), 12))},
		{"a new file goes with its Live video, RAW and sidecars, and a converted video as its source", twoWay, byMD5, p,
			nil, cands(converted, shot),
			nil, nil, nfs(imported(shot, 10), imported(converted, 10))},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := domain.RemoteSpec{Name: "disk", Mode: c.mode, Capabilities: domain.Capabilities{ContentHash: c.hash}}
			matched, left, all := slices.Clone(c.matched), slices.Clone(c.left), slices.Clone(pairs)
			ls, js, ns := plan.JoinFresh(c.matched, c.left, pairs, r, c.p)
			if !same(ls, js, ns, c.linked, c.join, c.files) {
				t.Errorf("JoinFresh = %s; want %s", show(ls, js, ns), show(c.linked, c.join, c.files))
			}
			rm, rl, rp := slices.Clone(matched), slices.Clone(left), slices.Clone(all)
			slices.Reverse(rm)
			slices.Reverse(rl)
			slices.Reverse(rp)
			if rls, rjs, rns := plan.JoinFresh(rm, rl, rp, r, c.p); !same(rls, rjs, rns, ls, js, ns) {
				t.Errorf("with the links, the new files and the pairs reversed: %s; want %s", show(rls, rjs, rns), show(ls, js, ns))
			}
			for i := range ls {
				ls[i].Link.Path = "changed"
			}
			if !slices.Equal(c.matched, matched) || !reflect.DeepEqual(c.left, left) || !slices.Equal(pairs, all) {
				t.Error("JoinFresh changed its input or shares a slice with it")
			}
			var rules []string
			for _, d := range c.linked {
				rules = append(rules, d.Rule)
			}
			for _, cs := range c.join {
				for _, x := range cs {
					rules = append(rules, x.Rule)
				}
			}
			for _, n := range c.files {
				rules = append(rules, n.Decision.Rule)
			}
			for _, id := range rules {
				if _, ok := domain.Rules[id]; !ok {
					t.Errorf("%s is not in domain.Rules", id)
				}
			}
		})
	}
}
