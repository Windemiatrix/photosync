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

// Each row is a scan of the remote as ReadScan reads it against the links of
// the state: the files that no link names and where the linked files lie now.
// The expected links, new files and notices are written from the rules on
// matching, on copies of one file, on assets that do not sync or wait in the
// remote and on Live Photos and RAW pairs, not from the code, and must not
// depend on the order of the input or on the mode of the remote.
func TestMatchFresh(t *testing.T) {
	t.Parallel()
	const (
		personal, shared           = domain.LibraryPersonal, domain.LibraryShared
		byMD5, bySHA, noHash       = domain.HashMD5, domain.HashSHA256, domain.HashNone
		original, source           = domain.RoleOriginal, domain.RoleSource
		liveVideo, rawPair         = domain.RoleLiveVideo, domain.RoleRAWPair
		exact, sameShot, component = domain.LevelExact, domain.LevelSameShot, domain.LevelPairComponent
		pairRule, copyRule         = "R-LINK-03", "R-LAYOUT-08"

		last domain.LinkID = 10

		atHolidays, atHolidays1   = "2024/05-01 - Holidays/IMG_0001.HEIC", "2024/05-01 - Holidays/IMG_0001 (1).HEIC"
		liveHolidays, rawHolidays = "2024/05-01 - Holidays/IMG_0001.MOV", "2024/05-01 - Holidays/IMG_0001.DNG"
		atTrip, atTrip1           = "2024/05-02 - Trip/IMG_0001.HEIC", "2024/05-02 - Trip/IMG_0001 (1).HEIC"
		liveTrip, jpgTrip         = "2024/05-02 - Trip/IMG_0001.MOV", "2024/05-02 - Trip/IMG_0001.HEIC.jpg"
		atParty, liveParty        = "2024/05-03 - Party/IMG_0002.HEIC", "2024/05-03 - Party/IMG_0001.MOV"
	)
	taken, mtime := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC), time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	file := func(root domain.LibraryKind, at string, size int64, md5 string) domain.RemoteEntry {
		return domain.RemoteEntry{Root: root, Path: at, Size: size, MD5: md5, SHA256: "s" + md5, ETag: "e" + md5, MTime: mtime}
	}
	heic, mov, dng := file(personal, atHolidays, 100, "a1"), file(personal, liveHolidays, 300, "c1"), file(personal, rawHolidays, 400, "d1")
	trip, trip1, tripMov := file(personal, atTrip, 100, "a1"), file(personal, atTrip1, 100, "a1"), file(personal, liveTrip, 300, "c1")
	tripNew, tripJpg, partyMov := file(personal, atTrip, 100, "b2"), file(personal, jpgTrip, 200, "e2"), file(personal, liveParty, 300, "c1")
	replaced, dup, sharedHeic := file(personal, atHolidays, 100, "b2"), file(personal, atHolidays1, 100, "a1"), file(shared, atHolidays, 100, "a1")
	party, sharedParty := file(personal, atParty, 200, "e1"), file(shared, atParty, 200, "e1")
	tripBySHA := trip
	tripBySHA.MD5 = "zz"
	tripDng := file(personal, "2024/05-02 - Trip/IMG_0001.DNG", 400, "d2")
	aae, xmp := file(personal, "2024/05-01 - Holidays/IMG_0001.AAE", 10, "f1"), file(personal, "2024/05-03 - Party/IMG_0002.XMP", 10, "g1")
	sidecar := file(personal, "2024/05-01 - Holidays/IMG_0001.XMP", 10, "h1")
	notes, readme := file(personal, "2024/notes.txt", 5, "n1"), file(shared, "Albums/readme.pdf", 5, "n2")
	dir := domain.RemoteEntry{Root: personal, Path: "2024/05-01 - Holidays", IsDir: true}
	avi := file(personal, "2024/05-01 - Holidays/MVI_0001.AVI", 900, "v1")
	avi.Meta.Taken = taken
	photo := func(id domain.AssetID, name string, size int64, roles ...domain.ResourceRole) domain.Asset {
		a := domain.Asset{
			ID: id, Key: domain.AssetKey{CloudAssetGUID: fmt.Sprintf("guid-%d", id), UUID: fmt.Sprintf("uuid-%d", id)},
			Library: personal, TargetLibrary: personal, OriginalName: name, Taken: taken,
			Resources: []domain.Resource{{Role: domain.ResourceOriginal, Filename: name, Size: size}},
		}
		for _, role := range roles {
			a.Resources = append(a.Resources, domain.Resource{Role: role})
		}
		return a
	}
	a7, a9, b8 := photo(7, "IMG_0001.HEIC", 100), photo(9, "IMG_0001.HEIC", 100), photo(8, "IMG_0002.HEIC", 200)
	live7 := photo(7, "IMG_0001.HEIC", 100, domain.ResourceLiveVideo)
	full7 := photo(7, "IMG_0001.HEIC", 100, domain.ResourceLiveVideo, domain.ResourceRAWPair)
	video7 := photo(7, "MVI_0001.MP4", 500)
	video7.IsVideo = true
	hidden7, hidden9, trashed7 := photo(7, "IMG_0001.HEIC", 100), photo(9, "IMG_0001.HEIC", 100), photo(7, "IMG_0001.HEIC", 100)
	hidden7.Hidden, hidden9.Hidden, trashed7.InTrash = true, true, true
	link := func(id domain.LinkID, asset domain.AssetID, remote, at string, role domain.LinkRole, status domain.LinkStatus) domain.Link {
		return domain.Link{
			ID: id, AssetID: asset, Remote: remote, Root: personal, Path: at, Role: role, AlbumID: 10,
			Size: 100, MD5: "a1", Level: exact, Status: status,
		}
	}
	l1, l2 := link(1, 7, "disk", atHolidays, original, domain.StatusActive), link(2, 9, "disk", atHolidays1, original, domain.StatusActive)
	l3 := link(3, 7, "other", atHolidays, original, domain.StatusActive)
	l4 := link(4, 7, "disk", liveHolidays, liveVideo, domain.StatusActive)
	l4.Size, l4.MD5 = 300, "c1"
	detached := link(1, 7, "disk", atHolidays, original, domain.StatusDetached)
	quarantined := link(6, 7, "disk", atHolidays, original, domain.StatusQuarantinedRemote)
	quarantined.QuarantinePath = ".photosync/quarantine/2024-06-01/" + atHolidays
	made := func(id domain.LinkID, asset domain.AssetID, e domain.RemoteEntry, role domain.LinkRole, level domain.MatchLevel, rule string) domain.LinkDecision {
		return domain.LinkDecision{
			Link: domain.Link{
				ID: id, AssetID: asset, Remote: "disk", Root: e.Root, Path: e.Path, Role: role,
				Size: e.Size, SHA256: e.SHA256, MD5: e.MD5, ETag: e.ETag, MTime: e.MTime, Level: level, Status: domain.StatusActive,
			},
			ICloud: domain.ChangeNew, Remote: domain.ChangeNew, Action: domain.ActionLink, Rule: rule,
		}
	}
	alone := func(e domain.RemoteEntry) match.Candidate { return match.Candidate{Main: e, Role: original} }
	unknown := func(root domain.LibraryKind, at string) domain.Notice {
		return domain.Notice{Code: "plan.unknown_file", Params: []string{string(root), at}}
	}
	assets := func(a ...domain.Asset) []domain.Asset { return a }
	links := func(l ...domain.Link) []domain.Link { return l }
	entries := func(e ...domain.RemoteEntry) []domain.RemoteEntry { return e }
	decided := func(d ...domain.LinkDecision) []domain.LinkDecision { return d }
	newFiles := func(c ...match.Candidate) []match.Candidate { return c }
	type found = map[domain.LinkID]domain.RemoteEntry
	p, skip := domain.DefaultPolicies(), domain.DefaultPolicies()
	skip.Hidden = domain.HiddenSkip
	show := func(ds []domain.LinkDecision, cs []match.Candidate, ns []domain.Notice) string {
		var out []string
		for _, d := range ds {
			out = append(out, fmt.Sprintf("%+v %s/%s %s %s", d.Link, d.ICloud, d.Remote, d.Action, d.Rule))
		}
		for _, c := range cs {
			s := fmt.Sprintf("new %s:%s %s %s", c.Main.Root, c.Main.Path, c.Main.MD5, c.Role)
			for _, e := range []*domain.RemoteEntry{c.Live, c.RAW} {
				if e != nil {
					s += " with " + e.Path
				}
			}
			for _, e := range c.Sidecars {
				s += " sidecar " + e.Path
			}
			out = append(out, s)
		}
		for _, n := range ns {
			out = append(out, fmt.Sprintf("notice %s %q", n.Code, n.Params))
		}
		return fmt.Sprintf("%q", out)
	}
	sameNotice := func(m, n domain.Notice) bool { return m.Code == n.Code && slices.Equal(m.Params, n.Params) }
	same := func(ds []domain.LinkDecision, cs []match.Candidate, ns []domain.Notice, wd []domain.LinkDecision, wc []match.Candidate, wn []domain.Notice) bool {
		return slices.Equal(ds, wd) && reflect.DeepEqual(cs, wc) && slices.EqualFunc(ns, wn, sameNotice)
	}
	for _, c := range []struct {
		name      string
		hash      domain.HashKind
		p         domain.Policies
		assets    []domain.Asset
		links     []domain.Link
		files     found
		fresh     []domain.RemoteEntry
		decisions []domain.LinkDecision
		left      []match.Candidate
		notices   []domain.Notice
	}{
		{"nothing new", byMD5, p, assets(a7), nil, nil, nil, nil, nil, nil},
		{"a new asset and its file", byMD5, p, assets(a7), nil, nil, entries(heic),
			decided(made(11, 7, heic, original, exact, pairRule)), nil, nil},
		{"a photo with its Live video and RAW", byMD5, p, assets(full7), nil, nil, entries(dng, mov, heic),
			decided(made(11, 7, heic, original, exact, pairRule), made(12, 7, mov, liveVideo, component, pairRule),
				made(13, 7, dng, rawPair, component, pairRule)), nil, nil},
		{"a Live video that the asset lacks is a new file", byMD5, p, assets(a7), nil, nil, entries(mov, heic),
			decided(made(11, 7, heic, original, exact, pairRule)), newFiles(alone(mov)), nil},
		{"the source file of a converted video", byMD5, p, assets(video7), nil, nil, entries(avi),
			decided(made(11, 7, avi, source, sameShot, pairRule)), nil, nil},
		{"sidecars get no link", byMD5, p, assets(a7), nil, nil, entries(xmp, party, aae, heic),
			decided(made(11, 7, heic, original, exact, pairRule)),
			newFiles(match.Candidate{Main: party, Role: original, Sidecars: entries(xmp)}), nil},
		{"unknown files are reported, the sidecar of a linked file and a directory are not", byMD5, p, assets(a7), links(l1),
			found{1: heic}, entries(readme, sidecar, notes, dir), nil, nil,
			[]domain.Notice{unknown(personal, "2024/notes.txt"), unknown(shared, "Albums/readme.pdf")}},
		{"an asset linked in the remote matches no other file, nor holds a lone copy of its video", byMD5, p, assets(a7),
			links(l1, l4), found{1: heic, 4: mov}, entries(partyMov, tripNew), nil, newFiles(alone(tripNew), alone(partyMov)), nil},
		{"copies of a linked file in another directory and under the other root", byMD5, p, assets(a7), links(l1), found{1: heic}, // covers: R-LAYOUT-08
			entries(sharedHeic, trip), decided(made(11, 7, trip, original, exact, copyRule), made(12, 7, sharedHeic, original, exact, copyRule)),
			nil, nil},
		{"the copy of a replaced file has its new content", byMD5, p, assets(a7), links(l1), found{1: replaced},
			entries(tripNew, trip1), decided(made(11, 7, tripNew, original, exact, copyRule)), newFiles(alone(trip1)), nil},
		{"a file gone from its link is no copy", byMD5, p, assets(a7), links(l1), nil, entries(trip), nil, newFiles(alone(trip)), nil},
		{"a copy of a file that matched", byMD5, p, assets(a7), nil, nil, entries(trip, heic),
			decided(made(11, 7, heic, original, exact, pairRule), made(12, 7, trip, original, exact, copyRule)), nil, nil},
		{"a copy with its Live video", byMD5, p, assets(live7), nil, nil, entries(tripMov, trip, mov, heic),
			decided(made(11, 7, heic, original, exact, pairRule), made(12, 7, mov, liveVideo, component, pairRule),
				made(13, 7, trip, original, exact, copyRule), made(14, 7, tripMov, liveVideo, component, copyRule)), nil, nil},
		{"a copy leaves a video that its asset lacks as a new file, in the order of place", byMD5, p, assets(a7), links(l1),
			found{1: heic}, entries(tripJpg, tripMov, trip), decided(made(11, 7, trip, original, exact, copyRule)),
			newFiles(alone(tripJpg), alone(tripMov)), nil},
		{"a copy leaves a RAW that its asset lacks as a new file, though the asset has a Live video", byMD5, p, assets(live7), links(l1),
			found{1: heic}, entries(tripDng, trip), decided(made(11, 7, trip, original, exact, copyRule)), newFiles(alone(tripDng)), nil},
		{"a copy with its RAW", byMD5, p, assets(full7), links(l1), found{1: heic}, entries(tripDng, trip),
			decided(made(11, 7, trip, original, exact, copyRule), made(12, 7, tripDng, rawPair, component, copyRule)), nil, nil},
		{"no copy on a remote without a hash", noHash, p, assets(a7), links(l1), found{1: heic}, entries(trip),
			nil, newFiles(alone(trip)), nil},
		{"a content that two assets hold goes with the lower AssetID", byMD5, p, assets(a9, a7), links(l1, l2),
			found{1: heic, 2: dup}, entries(trip), decided(made(11, 7, trip, original, exact, copyRule)), nil, nil},
		{"a hidden asset under hidden: skip takes its file and a copy without links", byMD5, skip, assets(hidden7), nil, nil,
			entries(trip, heic), nil, nil, nil},
		{"an asset in Recently Deleted takes its file without a link", byMD5, p, assets(trashed7), nil, nil, entries(heic),
			nil, nil, nil},
		{"an asset with a detached link takes a new file of it without a link", byMD5, p, assets(a7), links(detached), nil,
			entries(trip), nil, nil, nil},
		{"an asset with a quarantined link takes a new file of it without a link", byMD5, p, assets(a7), links(quarantined, l4),
			nil, entries(trip), nil, nil, nil},
		{"a copy of the file of a hidden asset is neither linked nor new", byMD5, skip, assets(hidden7), links(l1), found{1: heic},
			entries(trip), nil, nil, nil},
		{"a copy of the file of an asset gone from Photos is new", byMD5, p, nil, links(l1), found{1: heic}, entries(trip),
			nil, newFiles(alone(trip)), nil},
		{"the links of another remote do not count", byMD5, p, assets(a7), links(l3), nil, entries(heic),
			decided(made(11, 7, heic, original, exact, pairRule)), nil, nil},
		{"a file that matches a new asset goes with it, though another asset holds its content", byMD5, p, assets(a7, a9),
			links(l1), found{1: heic}, entries(trip), decided(made(11, 9, trip, original, exact, pairRule)), nil, nil},
		{"IDs go above the last: pairs, then copies, in both roots", byMD5, p, assets(b8, a7), nil, nil, entries(sharedParty, trip, heic),
			decided(made(11, 7, heic, original, exact, pairRule), made(12, 8, sharedParty, original, exact, pairRule),
				made(13, 7, trip, original, exact, copyRule)), nil, nil},
		{"a copy by the hash of the kind that the remote gives", bySHA, p, assets(a7), links(l1), found{1: heic},
			entries(tripBySHA), decided(made(11, 7, tripBySHA, original, exact, copyRule)), nil, nil},
		{"a file that a hidden asset and a new one both match goes to the new one", byMD5, skip, assets(hidden7, a9), nil, nil,
			entries(heic), decided(made(11, 9, heic, original, exact, pairRule)), nil, nil},
		{"a file that an asset waiting for relink and a new one both match goes to the new one", byMD5, p, assets(a7, a9), links(detached),
			nil, entries(trip), decided(made(11, 9, trip, original, exact, pairRule)), nil, nil},
		{"a second file of the name and size of a matched asset with other content is new", byMD5, p, assets(a7), nil, nil,
			entries(tripNew, heic), decided(made(11, 7, heic, original, exact, pairRule)), newFiles(alone(tripNew)), nil},
		{"a file that a hidden asset matches is no copy of another asset with its content", byMD5, skip, assets(a7, hidden9), links(l1),
			found{1: heic}, entries(trip), nil, nil, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			spec := func(m domain.Mode) domain.RemoteSpec {
				return domain.RemoteSpec{Name: "disk", Mode: m, Capabilities: domain.Capabilities{ContentHash: c.hash}}
			}
			before := fmt.Sprintf("%+v %+v %+v %+v", c.assets, c.links, c.files, c.fresh)
			as, ls, fs := slices.Clone(c.assets), slices.Clone(c.links), slices.Clone(c.fresh)
			ds, cs, ns := plan.MatchFresh(c.assets, c.links, c.files, spec(domain.ModeTwoWay), c.fresh, last, c.p)
			if !same(ds, cs, ns, c.decisions, c.left, c.notices) {
				t.Errorf("MatchFresh = %s; want %s", show(ds, cs, ns), show(c.decisions, c.left, c.notices))
			}
			for _, m := range []domain.Mode{domain.ModeUploadOnly, domain.ModeDownloadOnly} {
				if md, mc, mn := plan.MatchFresh(c.assets, c.links, c.files, spec(m), c.fresh, last, c.p); !same(md, mc, mn, ds, cs, ns) {
					t.Errorf("%s: %s; want %s, as two-way", m, show(md, mc, mn), show(ds, cs, ns))
				}
			}
			slices.Reverse(as)
			slices.Reverse(ls)
			slices.Reverse(fs)
			if rd, rc, rn := plan.MatchFresh(as, ls, c.files, spec(domain.ModeTwoWay), fs, last, c.p); !same(rd, rc, rn, ds, cs, ns) {
				t.Errorf("with the assets, links and scan reversed: %s; want %s", show(rd, rc, rn), show(ds, cs, ns))
			}
			if after := fmt.Sprintf("%+v %+v %+v %+v", c.assets, c.links, c.files, c.fresh); after != before {
				t.Errorf("MatchFresh changed its input:\n%s\nwant\n%s", after, before)
			}
			for _, d := range c.decisions {
				if _, ok := domain.Rules[d.Rule]; !ok {
					t.Errorf("%s is not in domain.Rules", d.Rule)
				}
			}
		})
	}
}
