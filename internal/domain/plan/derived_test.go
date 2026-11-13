package plan_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Each row is what matching gives for one remote once the album pairs of the
// run are known: the new links, the assets that they hold and the scan with
// the frames of the files. The decisions and notices are written from the
// derived copy policy in each mode, not from the code, and must not depend on
// the order of the input. A decision of the policy deletes nothing: under
// every deletes policy its link stays active.
func TestDecideDerived(t *testing.T) {
	t.Parallel()
	const (
		personal, shared              = domain.LibraryPersonal, domain.LibraryShared
		twoWay, upOnly, downOnly      = domain.ModeTwoWay, domain.ModeUploadOnly, domain.ModeDownloadOnly
		replace, both, keep           = domain.DerivedReplaceWithOriginal, domain.DerivedKeepBoth, domain.DerivedKeepRemote
		original, source, liveVideo   = domain.RoleOriginal, domain.RoleSource, domain.RoleLiveVideo
		exact, sameShot, weak         = domain.LevelExact, domain.LevelSameShot, domain.LevelSameShotWeak
		timeSize, probable, component = domain.LevelSameTimeSize, domain.LevelProbable, domain.LevelPairComponent
		pairRule, copyRule            = "R-LINK-03", "R-LAYOUT-08"
	)
	photo := func(id domain.AssetID, name string, width, height int, size int64) domain.Asset {
		return domain.Asset{
			ID: id, Key: domain.AssetKey{UUID: fmt.Sprintf("uuid-%d", id)}, Library: personal, TargetLibrary: personal,
			OriginalName: name, Width: width, Height: height,
			Resources: []domain.Resource{{Role: domain.ResourceOriginal, Filename: name, Size: size}},
		}
	}
	file := func(root domain.LibraryKind, at string, width, height int, size int64, md5 string) domain.RemoteEntry {
		return domain.RemoteEntry{Root: root, Path: at, Size: size, MD5: md5, ETag: "e" + md5, Meta: domain.MediaMeta{Width: width, Height: height}}
	}
	made := func(id domain.LinkID, asset domain.AssetID, e domain.RemoteEntry, role domain.LinkRole, level domain.MatchLevel, rule string) domain.LinkDecision {
		return domain.LinkDecision{
			Link: domain.Link{
				ID: id, AssetID: asset, Remote: "disk", Root: e.Root, Path: e.Path, Role: role,
				Size: e.Size, MD5: e.MD5, ETag: e.ETag, Level: level, Status: domain.StatusActive,
			},
			ICloud: domain.ChangeNew, Remote: domain.ChangeNew, Action: domain.ActionLink, Rule: rule,
		}
	}
	replaced := func(d domain.LinkDecision) domain.LinkDecision {
		d.Action, d.Rule = domain.ActionReplaceFile, "R-OBJECT-10"
		return d
	}
	derived := func(d domain.LinkDecision) domain.LinkDecision {
		d.Link.Role = domain.RoleDerived
		return d
	}
	decs := func(d ...domain.LinkDecision) []domain.LinkDecision { return d }

	a7, a8 := photo(7, "IMG_0001.HEIC", 4032, 3024, 2_000_000), photo(8, "IMG_0002.HEIC", 4032, 3024, 2_000_000)
	a9 := photo(9, "IMG_0003.HEIC", 4032, 3024, 2_000_000)
	smaller := file(personal, "2024/05-01 - Holidays/IMG_0001.jpg", 1600, 1200, 2_500_000, "a1")
	smallerMov := file(personal, "2024/05-01 - Holidays/IMG_0001.MOV", 0, 0, 3_000_000, "b1")
	equal := file(personal, "2024/05-03 - Party/IMG_0003.jpeg", 4032, 3024, 2_000_000, "c1")
	larger := file(shared, "2024/05-02 - Trip/IMG_0002.jpg", 8064, 6048, 1_500_000, "d1")
	twin := file(personal, "2024/05-02 - Trip/IMG_0002.jpg", 640, 480, 300_000, "e1")
	smallerCopy := file(personal, "2024/05-04 - Zoo/IMG_0001.jpg", 1600, 1200, 2_500_000, "a1")
	largerCopy := file(shared, "2024/05-05 - Old/IMG_0002.jpg", 8064, 6048, 1_500_000, "d1")
	d11, d12 := made(11, 7, smaller, original, sameShot, pairRule), made(12, 7, smallerMov, liveVideo, component, pairRule)
	d13, d14 := made(13, 9, equal, original, weak, pairRule), made(14, 8, larger, original, probable, pairRule)
	d15, d16 := made(15, 7, smallerCopy, original, exact, copyRule), made(16, 8, largerCopy, original, exact, copyRule)
	mixed := func() []domain.LinkDecision { return decs(d15, d12, d14, d11, d16, d13) }
	photos := []domain.Asset{a9, a7, a8}
	scan := []domain.RemoteEntry{largerCopy, smallerMov, larger, smaller, equal, smallerCopy, twin}
	matched := decs(d11, d12, d13, d14, d15, d16)
	replacedAll := decs(replaced(d11), replaced(d12), d13, d14, replaced(d15), d16)
	derivedAll := decs(derived(d11), derived(d12), d13, d14, derived(d15), d16)
	cloudBetter := []domain.Notice{{Code: "plan.cloud_better", Params: []string{"shared", "2024/05-02 - Trip/IMG_0002.jpg"}}}

	shot := func(id domain.AssetID) domain.Asset {
		return photo(id, fmt.Sprintf("IMG_%04d.HEIC", id), 4032, 3024, 2_000_000)
	}
	garden := func(id domain.AssetID) domain.RemoteEntry {
		return file(personal, fmt.Sprintf("2024/06-01 - Garden/IMG_%04d.jpg", id), 1600, 1200, 2_500_000, fmt.Sprintf("g%d", id))
	}
	d21, d22 := made(21, 31, garden(31), original, exact, pairRule), made(22, 32, garden(32), original, sameShot, pairRule)
	d23, d24 := made(23, 33, garden(33), original, weak, pairRule), made(24, 34, garden(34), original, timeSize, pairRule)
	d25 := made(25, 35, garden(35), original, probable, pairRule)
	video := photo(20, "MVI_0020.MP4", 1920, 1080, 50_000_000)
	video.IsVideo = true
	avi := file(personal, "2024/05-01 - Holidays/MVI_0020.AVI", 640, 480, 80_000_000, "v1")
	d26 := made(26, 20, avi, source, sameShot, pairRule)
	lake := file(personal, "2024/06-02 - Lake/IMG_0037.jpg", 8064, 6048, 1_500_000, "h37")
	d27 := made(27, 37, lake, original, sameShot, pairRule)

	show := func(ds []domain.LinkDecision, ns []domain.Notice) string {
		var out []string
		for _, d := range ds {
			out = append(out, fmt.Sprintf("%+v %s/%s %s %s", d.Link, d.ICloud, d.Remote, d.Action, d.Rule))
		}
		for _, n := range ns {
			out = append(out, fmt.Sprintf("notice %s %q", n.Code, n.Params))
		}
		return fmt.Sprintf("%q", out)
	}
	same := func(ds []domain.LinkDecision, ns []domain.Notice, wd []domain.LinkDecision, wn []domain.Notice) bool {
		return slices.Equal(ds, wd) && slices.EqualFunc(ns, wn, func(m, n domain.Notice) bool { return m.Code == n.Code && slices.Equal(m.Params, n.Params) })
	}
	for _, c := range []struct {
		name      string
		mode      domain.Mode
		policy    domain.DerivedCopyPolicy
		linked    []domain.LinkDecision
		assets    []domain.Asset
		fresh     []domain.RemoteEntry
		decisions []domain.LinkDecision
		notices   []domain.Notice
	}{
		{"nothing new", twoWay, replace, nil, nil, nil, nil, nil},
		{"replace_with_original, two-way: a lesser copy with its Live video and its copies gives way to the original", // covers: R-OBJECT-10
			twoWay, replace, mixed(), photos, scan, replacedAll, cloudBetter},
		{"replace_with_original, upload-only: as two-way", upOnly, replace, mixed(), photos, scan, replacedAll, cloudBetter},
		{"replace_with_original, download-only: nothing in the cloud is replaced", downOnly, replace, mixed(), photos, scan, matched, cloudBetter},
		{"keep_both, two-way: a lesser copy with its Live video and its copies is kept as derived", twoWay, both, mixed(), photos, scan, derivedAll, cloudBetter},
		{"keep_both, upload-only: as two-way", upOnly, both, mixed(), photos, scan, derivedAll, cloudBetter},
		{"keep_both, download-only: as two-way", downOnly, both, mixed(), photos, scan, derivedAll, cloudBetter},
		{"keep_remote, two-way: the links stay as matched", twoWay, keep, mixed(), photos, scan, matched, cloudBetter},
		{"keep_remote, upload-only: the same", upOnly, keep, mixed(), photos, scan, matched, cloudBetter},
		{"keep_remote, download-only: the same", downOnly, keep, mixed(), photos, scan, matched, cloudBetter},
		{"an unknown policy keeps the copy in the cloud", twoWay, "", mixed(), photos, scan, matched, cloudBetter},
		{"only a file matched by its shot is compared", twoWay, replace, decs(d25, d23, d21, d24, d22),
			[]domain.Asset{shot(35), shot(31), shot(34), shot(32), shot(33)},
			[]domain.RemoteEntry{garden(33), garden(31), garden(35), garden(32), garden(34)},
			decs(d21, replaced(d22), replaced(d23), d24, replaced(d25)), nil},
		{"the source of a converted video is no copy of it", twoWay, replace, decs(d26), []domain.Asset{video},
			[]domain.RemoteEntry{avi}, decs(d26), nil},
		{"a link whose asset is gone from the snapshot is left as it is", twoWay, replace, decs(d12, d11), nil, scan,
			decs(d11, d12), nil},
		{"a link whose file is not in the scan is left as it is", twoWay, replace, decs(d12, d11), photos, nil,
			decs(d11, d12), nil},
		{"the notices follow the order of their links", twoWay, replace, decs(d27, d14), []domain.Asset{shot(37), a8},
			[]domain.RemoteEntry{lake, larger}, decs(d14, d27), []domain.Notice{
				{Code: "plan.cloud_better", Params: []string{"shared", "2024/05-02 - Trip/IMG_0002.jpg"}},
				{Code: "plan.cloud_better", Params: []string{"personal", "2024/06-02 - Lake/IMG_0037.jpg"}},
			}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			linked, assets, fresh := slices.Clone(c.linked), slices.Clone(c.assets), slices.Clone(c.fresh)
			for i := range assets {
				assets[i].Resources = slices.Clone(assets[i].Resources)
			}
			ds, ns := plan.DecideDerived(c.linked, c.assets, c.fresh, c.mode, c.policy)
			if !same(ds, ns, c.decisions, c.notices) {
				t.Errorf("DecideDerived = %s; want %s", show(ds, ns), show(c.decisions, c.notices))
			}
			rl, ra, rf := slices.Clone(linked), slices.Clone(c.assets), slices.Clone(fresh)
			slices.Reverse(rl)
			slices.Reverse(ra)
			slices.Reverse(rf)
			if rd, rn := plan.DecideDerived(rl, ra, rf, c.mode, c.policy); !same(rd, rn, ds, ns) {
				t.Errorf("with the links, assets and scan reversed: %s; want %s", show(rd, rn), show(ds, ns))
			}
			for i := range ds {
				ds[i].Link.Path = "changed"
			}
			if !slices.Equal(c.linked, linked) || !reflect.DeepEqual(c.assets, assets) || !slices.Equal(c.fresh, fresh) {
				t.Error("DecideDerived changed its input or shares a slice with it")
			}
			for _, d := range c.decisions {
				if _, ok := domain.Rules[d.Rule]; !ok {
					t.Errorf("%s is not in domain.Rules", d.Rule)
				}
				for _, deletes := range []domain.DeletesPolicy{domain.DeletesNone, domain.DeletesQuarantine, domain.DeletesPropagate} {
					if got, status := plan.StatusDeletes(d, deletes); got != d || status != domain.StatusActive {
						t.Errorf("StatusDeletes(%s by %s, %s) = %s by %s, link %q; want it as it is, active",
							d.Action, d.Rule, deletes, got.Action, got.Rule, status)
					}
				}
			}
		})
	}
}
