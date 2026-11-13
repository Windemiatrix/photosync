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

// Each row decides the Live Photo videos and RAW files of one asset from what
// the run makes of the files that they lie beside. The expected fates,
// decisions, copies and statuses are written from the rules on components,
// the modes and the deletes policy, not from the code, and must not depend on
// the order of the links, the remotes or the decisions of a remote.
func TestDecideComponents(t *testing.T) {
	t.Parallel()
	const (
		holidays, trip, party domain.AlbumID = 10, 11, 12

		personal, shared                           = domain.LibraryPersonal, domain.LibraryShared
		original, source, liveVideo, rawPair       = domain.RoleOriginal, domain.RoleSource, domain.RoleLiveVideo, domain.RoleRAWPair
		unchanged, moved, replaced, deleted, added = domain.ChangeUnchanged, domain.ChangeMoved, domain.ChangeReplaced, domain.ChangeDeleted, domain.ChangeNew
		twoWay, uploadOnly, downloadOnly           = domain.ModeTwoWay, domain.ModeUploadOnly, domain.ModeDownloadOnly
		quarantine, noDeletes, propagate           = domain.DeletesQuarantine, domain.DeletesNone, domain.DeletesPropagate
		nothing, upload, importAsset, link         = domain.ActionNone, domain.ActionUpload, domain.ActionImport, domain.ActionLink
		moveFile, copyFile, moveAsset              = domain.ActionMoveFile, domain.ActionCopyFile, domain.ActionMoveAsset
		replace, moveReplace, takeBase             = domain.ActionReplaceAsset, domain.ActionMoveFileReplaceAsset, domain.ActionUpdateBase
		deleteFile, deleteAsset, toQuarantine      = domain.ActionDeleteFile, domain.ActionDeleteAsset, domain.ActionQuarantineFile
		forget                                     = domain.ActionForget
		active, inCloud, inICloud, detached        = domain.StatusActive, domain.StatusQuarantinedRemote, domain.StatusQuarantinedICloud, domain.StatusDetached

		dir, tripDir, partyDir, misc = "2024/05-01 - Holidays/", "2024/05-02 - Trip/", "2024/05-03 - Party/", "Misc/"
	)
	file := func(id domain.LinkID, remote string, role domain.LinkRole, album domain.AlbumID, p string) domain.Link {
		return domain.Link{
			ID: id, AssetID: 7, Remote: remote, Root: personal, Path: p, Role: role, AlbumID: album,
			Size: 100, MD5: "a1", Level: domain.LevelExact, Status: active,
		}
	}
	m, v, raw := file(1, "alpha", original, holidays, dir+"IMG_0001.HEIC"), file(2, "alpha", liveVideo, holidays, dir+"IMG_0001.MOV"), file(3, "alpha", rawPair, holidays, dir+"IMG_0001.DNG")
	m2, v2 := file(4, "alpha", original, party, partyDir+"IMG_0001.HEIC"), file(5, "alpha", liveVideo, party, partyDir+"IMG_0001.MOV")
	bm, bv := file(6, "beta", original, holidays, dir+"IMG_0001.HEIC"), file(7, "beta", liveVideo, holidays, dir+"IMG_0001.MOV")
	sm, sv, src := file(8, "alpha", original, holidays, dir+"IMG_0001.HEIC"), file(9, "alpha", liveVideo, holidays, dir+"IMG_0001.MOV"), file(10, "alpha", source, holidays, dir+"MVI_0001.AVI")
	sm.Root, sv.Root = shared, shared
	away, held, back := m, v, m2
	away.Status, held.Status, back.Status = inCloud, inCloud, inCloud
	at := func(l domain.Link) domain.RemoteEntry {
		return domain.RemoteEntry{Root: l.Root, Path: l.Path, Size: l.Size, MD5: l.MD5}
	}
	to := func(l domain.Link, root domain.LibraryKind, p string) domain.RemoteEntry {
		return domain.RemoteEntry{Root: root, Path: p, Size: l.Size, MD5: l.MD5}
	}
	edited := func(l domain.Link) domain.RemoteEntry {
		e := at(l)
		e.MD5 = "f0"
		return e
	}
	place := func(l domain.Link, p string) domain.Link {
		l.Path = p
		return l
	}
	dec := func(l domain.Link, icloud, remote domain.ChangeKind, a domain.LinkAction, rule string) domain.LinkDecision {
		return domain.LinkDecision{Link: l, ICloud: icloud, Remote: remote, Action: a, Rule: rule}
	}
	kept := func(l domain.Link) domain.LinkDecision { return dec(l, unchanged, unchanged, nothing, "") }
	decs := func(d ...domain.LinkDecision) []domain.LinkDecision { return d }
	pl := func(a domain.LinkAction, l domain.Link, album domain.AlbumID, rule string) domain.Placement {
		return domain.Placement{Action: a, Link: l, AlbumID: album, Rule: rule}
	}
	pls := func(x ...domain.Placement) []domain.Placement { return x }
	links := func(l ...domain.Link) []domain.Link { return l }
	spec := func(name string, mode domain.Mode) domain.RemoteSpec {
		return domain.RemoteSpec{Name: name, Mode: mode, Capabilities: domain.Capabilities{ContentHash: domain.HashMD5}}
	}
	config := func(r ...domain.RemoteSpec) []domain.RemoteSpec { return r }
	alpha, upOnly, downOnly := config(spec("alpha", twoWay)), config(spec("alpha", uploadOnly)), config(spec("alpha", downloadOnly))
	type decided = map[string][]domain.LinkDecision
	type statuses = map[domain.LinkID]domain.LinkStatus
	type placed = map[string][]domain.Placement
	type found = map[domain.LinkID]domain.RemoteEntry
	type fated = map[domain.LinkID]domain.LinkAction
	type followed = map[domain.LinkID][]domain.LinkDecision
	type copied = map[domain.LinkID][]domain.Placement
	still, steady := decided{"alpha": decs(kept(m))}, statuses{1: active}
	moving := decided{"alpha": decs(dec(m, unchanged, moved, moveAsset, "R-LINK-05"))}
	planned := decided{"alpha": decs(dec(m, moved, unchanged, moveFile, "R-LINK-04"))}
	toTrip, copying := placed{"alpha": pls(pl(moveFile, m, trip, "R-OBJECT-05"))}, placed{"alpha": pls(pl(copyFile, m, trip, "R-OBJECT-03"))}
	type result struct {
		fates      fated
		components followed
		copies     copied
		after      statuses
	}
	equal := func(x, y result) bool {
		return maps.Equal(x.fates, y.fates) && maps.EqualFunc(x.components, y.components, slices.Equal[[]domain.LinkDecision]) &&
			maps.EqualFunc(x.copies, y.copies, slices.Equal[[]domain.Placement]) && maps.Equal(x.after, y.after)
	}
	show := func(r result) []string {
		var out []string
		for _, id := range slices.Sorted(maps.Keys(r.fates)) {
			out = append(out, fmt.Sprintf("fate %d %s", id, r.fates[id]))
		}
		for _, id := range slices.Sorted(maps.Keys(r.components)) {
			for _, d := range r.components[id] {
				out = append(out, fmt.Sprintf("%d: %d %s/%s %s %s", id, d.Link.ID, d.ICloud, d.Remote, d.Action, d.Rule))
			}
		}
		for _, id := range slices.Sorted(maps.Keys(r.copies)) {
			for _, x := range r.copies[id] {
				out = append(out, fmt.Sprintf("%d: %s %d %s #%d %s", id, x.Action, x.Link.ID, x.Link.Path, x.AlbumID, x.Rule))
			}
		}
		return append(out, fmt.Sprintf("status %v", r.after))
	}
	for _, c := range []struct {
		name       string
		decisions  decided
		status     statuses
		placements placed
		links      []domain.Link
		files      found
		remotes    []domain.RemoteSpec
		deletes    domain.DeletesPolicy
		want       result
	}{
		{"nothing changed", still, steady, nil, links(m, v), found{1: at(m), 2: at(v)}, alpha, quarantine,
			result{fated{1: nothing}, followed{1: decs(kept(v))}, nil, statuses{2: active}}},
		{"a video deleted on its own is restored beside its photo", still, steady, nil, links(m, v), found{1: at(m)}, alpha, quarantine,
			result{fated{1: nothing}, followed{1: decs(dec(v, unchanged, deleted, upload, "R-COMPONENT-01"))}, nil, statuses{2: active}}},
		{"a video moved away on its own goes back beside its photo", still, steady, nil, links(m, v),
			found{1: at(m), 2: to(v, personal, tripDir+"IMG_0001.MOV")}, alpha, quarantine,
			result{fated{1: nothing}, followed{1: decs(dec(v, unchanged, moved, moveFile, "R-COMPONENT-02"))}, nil, statuses{2: active}}},
		{"a video with new content replaces the asset", still, steady, nil, links(m, v), found{1: at(m), 2: edited(v)}, alpha, quarantine,
			result{fated{1: nothing}, followed{1: decs(dec(v, unchanged, replaced, replace, "R-COMPONENT-03"))}, nil, statuses{2: active}}},
		{"a photo that the user moved takes its video along", moving, steady, nil, links(m, v),
			found{1: to(m, personal, tripDir+"IMG_0001.HEIC"), 2: at(v)}, alpha, quarantine,
			result{fated{1: moveFile}, followed{1: decs(dec(v, unchanged, unchanged, moveFile, "R-COMPONENT-04"))}, nil, statuses{2: active}}},
		{"a video that the user moved with its photo is taken into the base", moving, steady, nil, links(m, v),
			found{1: to(m, personal, tripDir+"IMG_0001.HEIC"), 2: to(v, personal, tripDir+"IMG_0001.MOV")}, alpha, quarantine,
			result{fated{1: moveFile}, followed{1: decs(dec(v, unchanged, moved, takeBase, "R-COMPONENT-04"))}, nil, statuses{2: active}}},
		{"a photo that the plan moves takes its video and its RAW file along", planned, steady, toTrip, links(raw, v, m),
			found{1: at(m), 2: at(v), 3: at(raw)}, alpha, quarantine,
			result{fated{1: moveFile}, followed{1: decs(dec(v, moved, unchanged, moveFile, "R-COMPONENT-04"), dec(raw, moved, unchanged, moveFile, "R-COMPONENT-04"))},
				nil, statuses{2: active, 3: active}}},
		{"where the plan moves a photo is not known yet, so a video that the user put there moves again beside it", planned, steady, toTrip, links(m, v),
			found{1: at(m), 2: to(v, personal, tripDir+"IMG_0001.MOV")}, alpha, quarantine,
			result{fated{1: moveFile}, followed{1: decs(dec(v, moved, moved, moveFile, "R-COMPONENT-02"))}, nil, statuses{2: active}}},
		{"a photo that the user moved with its video and the plan moves on takes the video along",
			decided{"alpha": decs(dec(m2, unchanged, moved, moveAsset, "R-LINK-05"))}, statuses{4: active},
			placed{"alpha": pls(pl(moveFile, place(m2, misc+"IMG_0001.HEIC"), trip, "R-OBJECT-05"))}, links(m2, v2),
			found{4: to(m2, personal, misc+"IMG_0001.HEIC"), 5: to(v2, personal, misc+"IMG_0001.MOV")}, alpha, quarantine,
			result{fated{4: moveFile}, followed{4: decs(dec(v2, unchanged, moved, moveFile, "R-COMPONENT-02"))}, nil, statuses{5: active}}},
		{"a video with new content follows the photo that the plan moves and replaces the asset", planned, steady, toTrip, links(m, v),
			found{1: at(m), 2: edited(v)}, alpha, quarantine,
			result{fated{1: moveFile}, followed{1: decs(dec(v, moved, replaced, moveReplace, "R-COMPONENT-03"))}, nil, statuses{2: active}}},
		{"a copy of the photo takes a copy of its video", still, steady, copying, links(m, v), found{1: at(m), 2: at(v)}, alpha, quarantine,
			result{fated{1: nothing}, followed{1: decs(kept(v))}, copied{1: pls(pl(copyFile, v, trip, "R-OBJECT-03"))}, statuses{2: active}}},
		{"a video deleted on its own is restored beside its photo and beside the copy of the photo", still, steady, copying, links(m, v),
			found{1: at(m)}, alpha, quarantine,
			result{fated{1: nothing}, followed{1: decs(dec(v, unchanged, deleted, upload, "R-COMPONENT-01"))},
				copied{1: pls(pl(upload, v, trip, "R-COMPONENT-01"))}, statuses{2: active}}},
		{"the copy of a video that the user moved is made from where it lies", still, steady, copying, links(m, v),
			found{1: at(m), 2: to(v, personal, misc+"IMG_0001.MOV")}, alpha, quarantine,
			result{fated{1: nothing}, followed{1: decs(dec(v, unchanged, moved, moveFile, "R-COMPONENT-02"))},
				copied{1: pls(pl(copyFile, place(v, misc+"IMG_0001.MOV"), trip, "R-OBJECT-03"))}, statuses{2: active}}},
		{"a copy of another file of the asset takes no video", decided{"alpha": decs(kept(m), kept(m2))}, statuses{1: active, 4: active},
			placed{"alpha": pls(pl(copyFile, m2, trip, "R-OBJECT-03"))}, links(m, v, m2), found{1: at(m), 2: at(v), 4: at(m2)}, alpha, quarantine,
			result{fated{1: nothing, 4: nothing}, followed{1: decs(kept(v))}, nil, statuses{2: active}}},
		{"a photo that goes to quarantine as a needless copy takes its video along, and neither keeps a link", still, steady,
			placed{"alpha": pls(pl(toQuarantine, m, 0, "R-OBJECT-04"))}, links(m, v), found{1: at(m), 2: at(v)}, alpha, quarantine,
			result{fated{1: toQuarantine}, followed{1: decs(dec(v, unchanged, unchanged, toQuarantine, "R-COMPONENT-04"))}, nil, statuses{2: ""}}},
		{"under quarantine a video goes to quarantine with the photo of an asset deleted in iCloud",
			decided{"alpha": decs(dec(m, deleted, unchanged, toQuarantine, "R-STATUS-01"))}, statuses{1: inCloud}, nil, links(m, v),
			found{1: at(m), 2: at(v)}, alpha, quarantine,
			result{fated{1: deleteFile}, followed{1: decs(dec(v, deleted, unchanged, toQuarantine, "R-STATUS-01"))}, nil, statuses{2: inCloud}}},
		{"under none a video stays, detached, with the photo of an asset deleted in iCloud",
			decided{"alpha": decs(dec(m, deleted, unchanged, nothing, "R-STATUS-05"))}, statuses{1: detached}, nil, links(m, v),
			found{1: at(m), 2: at(v)}, alpha, noDeletes,
			result{fated{1: deleteFile}, followed{1: decs(dec(v, deleted, unchanged, nothing, "R-STATUS-05"))}, nil, statuses{2: detached}}},
		{"under propagate a video is deleted with the photo of an asset deleted in iCloud",
			decided{"alpha": decs(dec(m, deleted, unchanged, deleteFile, "R-STATUS-06"))}, statuses{1: ""}, nil, links(m, v),
			found{1: at(m), 2: at(v)}, alpha, propagate,
			result{fated{1: deleteFile}, followed{1: decs(dec(v, deleted, unchanged, deleteFile, "R-STATUS-06"))}, nil, statuses{2: ""}}},
		{"a video already gone with the photo that the user deleted is forgotten",
			decided{"alpha": decs(dec(m, unchanged, deleted, deleteAsset, "R-STATUS-02"))}, statuses{1: inICloud}, nil, links(m, v), nil, alpha, quarantine,
			result{fated{1: deleteFile}, followed{1: decs(dec(v, unchanged, deleted, forget, "R-COMPONENT-04"))}, nil, statuses{2: ""}}},
		{"a photo that the user deleted and a change elsewhere uploads again leaves its old video to the deletes policy",
			decided{"alpha": decs(dec(m, moved, deleted, upload, "R-LINK-10"))}, steady, placed{"alpha": pls(pl(upload, domain.Link{}, trip, "R-LINK-10"))},
			links(m, v), found{2: at(v)}, alpha, quarantine,
			result{fated{1: deleteFile}, followed{1: decs(dec(v, moved, unchanged, toQuarantine, "R-STATUS-01"))}, nil, statuses{2: inCloud}}},
		{"a photo whose moved file outlives the deletion of its asset in iCloud keeps its video",
			decided{"alpha": decs(dec(m, deleted, moved, importAsset, "R-LINK-11"))}, steady, nil, links(m, v),
			found{1: to(m, personal, tripDir+"IMG_0001.HEIC"), 2: at(v)}, alpha, quarantine,
			result{fated{1: moveFile}, followed{1: decs(dec(v, deleted, unchanged, moveFile, "R-COMPONENT-04"))}, nil, statuses{2: active}}},
		{"upload-only: a video follows a photo that the user deleted under the deletes policy",
			decided{"alpha": decs(dec(m, unchanged, deleted, nothing, "R-MODE-01"))}, statuses{1: detached}, nil, links(m, v),
			found{2: at(v)}, upOnly, quarantine,
			result{fated{1: deleteFile}, followed{1: decs(dec(v, unchanged, unchanged, toQuarantine, "R-STATUS-01"))}, nil, statuses{2: inCloud}}},
		{"upload-only: a video with new content is taken into the base, since the asset is not replaced", still, steady, nil, links(m, v),
			found{1: at(m), 2: edited(v)}, upOnly, quarantine,
			result{fated{1: nothing}, followed{1: decs(dec(v, unchanged, replaced, takeBase, "R-MODE-01"))}, nil, statuses{2: active}}},
		{"upload-only: a video deleted on its own is uploaded again", still, steady, nil, links(m, v), found{1: at(m)}, upOnly, quarantine,
			result{fated{1: nothing}, followed{1: decs(dec(v, unchanged, deleted, upload, "R-COMPONENT-01"))}, nil, statuses{2: active}}},
		{"download-only: a video moved away on its own is taken into the base where it lies", still, steady, nil, links(m, v),
			found{1: at(m), 2: to(v, personal, tripDir+"IMG_0001.MOV")}, downOnly, quarantine,
			result{fated{1: nothing}, followed{1: decs(dec(v, unchanged, moved, takeBase, "R-MODE-02"))}, nil, statuses{2: active}}},
		{"download-only: a video deleted on its own is detached", still, steady, nil, links(m, v), found{1: at(m)}, downOnly, quarantine,
			result{fated{1: nothing}, followed{1: decs(dec(v, unchanged, deleted, nothing, "R-MODE-02"))}, nil, statuses{2: detached}}},
		{"download-only: a video stays where it lies when the user moves its photo", moving, steady, nil, links(m, v),
			found{1: to(m, personal, tripDir+"IMG_0001.HEIC"), 2: at(v)}, downOnly, quarantine,
			result{fated{1: moveFile}, followed{1: decs(dec(v, unchanged, unchanged, nothing, "R-MODE-02"))}, nil, statuses{2: active}}},
		{"download-only: a video stays, detached, beside the photo of an asset deleted in iCloud",
			decided{"alpha": decs(dec(m, deleted, unchanged, nothing, "R-MODE-02"))}, statuses{1: detached}, nil, links(m, v),
			found{1: at(m), 2: at(v)}, downOnly, quarantine,
			result{fated{1: deleteFile}, followed{1: decs(dec(v, deleted, unchanged, nothing, "R-MODE-02"))}, nil, statuses{2: detached}}},
		{"download-only: a video with new content replaces the asset", still, steady, nil, links(m, v), found{1: at(m), 2: edited(v)}, downOnly, quarantine,
			result{fated{1: nothing}, followed{1: decs(dec(v, unchanged, replaced, replace, "R-COMPONENT-03"))}, nil, statuses{2: active}}},
		{"a video beside no photo of the state leaves the cloud under the deletes policy", decided{"alpha": nil}, nil, nil, links(away, v),
			found{2: at(v)}, alpha, quarantine,
			result{nil, followed{0: decs(dec(v, unchanged, unchanged, toQuarantine, "R-STATUS-01"))}, nil, statuses{2: inCloud}}},
		{"a remote where the asset is frozen decides nothing for its video", still, steady, nil, links(m, v, bm, bv),
			found{1: at(m), 2: at(v), 6: at(bm), 7: at(bv)}, config(spec("alpha", twoWay), spec("beta", twoWay)), quarantine,
			result{fated{1: nothing}, followed{1: decs(kept(v))}, nil, statuses{2: active}}},
		{"each remote and each root has its own photos and videos",
			decided{"alpha": decs(kept(m), kept(sm)), "beta": decs(kept(bm))}, statuses{1: active, 6: active, 8: active},
			placed{"alpha": pls(pl(toQuarantine, sm, 0, "R-OBJECT-04")), "beta": pls(pl(moveFile, bm, trip, "R-OBJECT-05"))},
			links(m, v, sm, sv, bm, bv), found{1: at(m), 2: at(v), 6: at(bm), 7: at(bv), 8: at(sm), 9: at(sv)},
			config(spec("alpha", twoWay), spec("beta", twoWay)), quarantine,
			result{fated{1: nothing, 6: moveFile, 8: toQuarantine},
				followed{
					1: decs(kept(v)),
					6: decs(dec(bv, unchanged, unchanged, moveFile, "R-COMPONENT-04")),
					8: decs(dec(sv, unchanged, unchanged, toQuarantine, "R-COMPONENT-04")),
				},
				nil, statuses{2: active, 7: active, 9: ""}}},
		{"only active files of the original or the source role have fates, a video in quarantine has no decision, and a video beside a file without a fate has no main file",
			decided{"alpha": decs(kept(m), dec(back, added, added, link, "R-LINK-03"), kept(src))}, statuses{1: active, 4: active, 10: active}, nil,
			links(m, held, back, v2, src), found{1: at(m), 4: at(back), 5: at(v2), 10: at(src)}, alpha, quarantine,
			result{fated{1: nothing, 10: nothing}, followed{0: decs(dec(v2, unchanged, unchanged, toQuarantine, "R-STATUS-01"))}, nil, statuses{5: inCloud}}},
		{"a video that the user moved to the other root goes back beside its photo", still, steady, nil, links(m, v),
			found{1: at(m), 2: to(v, shared, dir+"IMG_0001.MOV")}, alpha, quarantine,
			result{fated{1: nothing}, followed{1: decs(dec(v, unchanged, moved, moveFile, "R-COMPONENT-02"))}, nil, statuses{2: active}}},
		{"two copies of the photo take copies of its video and its RAW file, by link and then by copy", still, steady,
			placed{"alpha": pls(pl(copyFile, m, trip, "R-OBJECT-03"), pl(copyFile, m, party, "R-OBJECT-03"))}, links(m, v, raw),
			found{1: at(m), 2: at(v), 3: at(raw)}, alpha, quarantine,
			result{fated{1: nothing}, followed{1: decs(kept(v), kept(raw))},
				copied{1: pls(pl(copyFile, v, trip, "R-OBJECT-03"), pl(copyFile, v, party, "R-OBJECT-03"),
					pl(copyFile, raw, trip, "R-OBJECT-03"), pl(copyFile, raw, party, "R-OBJECT-03"))},
				statuses{2: active, 3: active}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			decisions, placements := decided{}, placed{}
			for name, ds := range c.decisions {
				decisions[name] = slices.Clone(ds)
			}
			for name, ps := range c.placements {
				placements[name] = slices.Clone(ps)
			}
			status, ls, fs, rs := maps.Clone(c.status), slices.Clone(c.links), maps.Clone(c.files), slices.Clone(c.remotes)
			var got result
			got.fates, got.components, got.copies, got.after =
				plan.DecideComponents(c.decisions, c.status, c.placements, c.links, c.files, c.remotes, c.deletes)
			if !equal(got, c.want) {
				t.Errorf("DecideComponents = %q; want %q", show(got), show(c.want))
			}
			var rules []string
			for _, ds := range c.want.components {
				for _, d := range ds {
					rules = append(rules, d.Rule)
				}
			}
			for _, ps := range c.want.copies {
				for _, x := range ps {
					rules = append(rules, x.Rule)
				}
			}
			for _, id := range rules {
				if _, ok := domain.Rules[id]; id != "" && !ok {
					t.Errorf("%s is not in domain.Rules", id)
				}
			}
			rd, rl, rr := decided{}, slices.Clone(c.links), slices.Clone(c.remotes)
			for name, ds := range c.decisions {
				rd[name] = slices.Clone(ds)
				slices.Reverse(rd[name])
			}
			slices.Reverse(rl)
			slices.Reverse(rr)
			var again result
			again.fates, again.components, again.copies, again.after =
				plan.DecideComponents(rd, c.status, c.placements, rl, c.files, rr, c.deletes)
			if !equal(again, got) {
				t.Errorf("with the links, remotes and decisions reversed: %q; want %q", show(again), show(got))
			}
			for _, ds := range got.components {
				for i := range ds {
					ds[i].Rule = "changed"
				}
			}
			for _, ps := range got.copies {
				for i := range ps {
					ps[i].Rule = "changed"
				}
			}
			if !maps.EqualFunc(c.decisions, decisions, slices.Equal[[]domain.LinkDecision]) || !maps.Equal(c.status, status) ||
				!maps.EqualFunc(c.placements, placements, slices.Equal[[]domain.Placement]) || !slices.Equal(c.links, ls) ||
				!maps.Equal(c.files, fs) || !reflect.DeepEqual(c.remotes, rs) {
				t.Errorf("DecideComponents changed its input or shares a slice with it")
			}
		})
	}
}
