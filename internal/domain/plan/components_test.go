package plan_test

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Every change of a component file and every fate of its main file, with and
// without the component already where the main file ends up, is either
// decided as written below, rule by rule from the rule texts and not from the
// code, or cannot happen at all.
func TestComponentAction(t *testing.T) {
	t.Parallel()
	const (
		unchanged, moved, deleted, replaced = domain.ChangeUnchanged, domain.ChangeMoved, domain.ChangeDeleted, domain.ChangeReplaced

		mainStays, mainMoves, mainCopied = domain.ActionNone, domain.ActionMoveFile, domain.ActionCopyFile
		mainQuarantined, mainDeleted     = domain.ActionQuarantineFile, domain.ActionDeleteFile
	)
	type cell struct {
		remote domain.ChangeKind
		main   domain.LinkAction
	}
	type result struct {
		action domain.LinkAction
		rule   string
	}
	rules := []struct {
		result
		onlySame bool
		cells    []cell
	}{
		{result{domain.ActionNone, ""}, false, []cell{{unchanged, mainStays}}},
		{result{domain.ActionUpload, "R-COMPONENT-01"}, false, []cell{{deleted, mainStays}, {deleted, mainMoves}, {deleted, mainCopied}}}, // covers: R-COMPONENT-01
		{result{domain.ActionMoveFile, "R-COMPONENT-02"}, false, []cell{{moved, mainStays}, {moved, mainMoves}}},                          // covers: R-COMPONENT-02
		{result{domain.ActionReplaceAsset, "R-COMPONENT-03"}, false, []cell{{replaced, mainStays}}},                                       // covers: R-COMPONENT-03
		{result{domain.ActionMoveFile, "R-COMPONENT-04"}, false, []cell{{unchanged, mainMoves}}},                                          // covers: R-COMPONENT-04
		{result{domain.ActionUpdateBase, "R-COMPONENT-04"}, true, []cell{{moved, mainStays}, {moved, mainMoves}}},
		{result{domain.ActionQuarantineFile, "R-COMPONENT-04"}, false, []cell{{unchanged, mainQuarantined}, {moved, mainQuarantined}, {replaced, mainQuarantined}}},
		{result{domain.ActionDeleteFile, "R-COMPONENT-04"}, false, []cell{{unchanged, mainDeleted}, {moved, mainDeleted}, {replaced, mainDeleted}}},
		{result{domain.ActionForget, "R-COMPONENT-04"}, false, []cell{{deleted, mainQuarantined}, {deleted, mainDeleted}}},
		{result{domain.ActionCopyFile, "R-OBJECT-03"}, false, []cell{{unchanged, mainCopied}, {moved, mainCopied}, {replaced, mainCopied}}},
		{result{domain.ActionMoveFileReplaceAsset, "R-COMPONENT-03"}, false, []cell{{replaced, mainMoves}}},
	}
	type key struct {
		cell
		same bool
	}
	want := map[key]result{}
	for _, onlySame := range []bool{false, true} {
		for _, r := range rules {
			for _, c := range r.cells {
				switch {
				case r.onlySame != onlySame:
				case onlySame:
					want[key{c, true}] = r.result
				default:
					want[key{c, false}], want[key{c, true}] = r.result, r.result
				}
			}
		}
	}
	// A component link has a base, so its file is in the cloud or was; its
	// main file stays, moves, is copied, or leaves the cloud.
	impossible := func(c cell) bool {
		return c.remote == domain.ChangeNone || c.remote == domain.ChangeNew ||
			!slices.Contains([]domain.LinkAction{mainStays, mainMoves, mainCopied, mainQuarantined, mainDeleted}, c.main)
	}
	actions := []domain.LinkAction{
		domain.ActionNone, domain.ActionUpload, domain.ActionImport, domain.ActionLink,
		domain.ActionMoveFile, domain.ActionMoveAsset, domain.ActionReplaceAsset,
		domain.ActionMoveFileReplaceAsset, domain.ActionConflict, domain.ActionDeleteFile,
		domain.ActionDeleteAsset, domain.ActionForget, domain.ActionUpdateBase,
		domain.ActionCopyFile, domain.ActionQuarantineFile,
	}
	used := map[string]bool{}
	for _, remote := range changeKinds {
		for _, main := range actions {
			for _, same := range []bool{false, true} {
				c := cell{remote, main}
				action, rule, ok := plan.ComponentAction(remote, main, same)
				w, listed := want[key{c, same}]
				switch {
				case listed == impossible(c):
					t.Errorf("%s, %s: listed %v, impossible %v", remote, main, listed, impossible(c))
				case !listed && (ok || action != "" || rule != ""):
					t.Errorf("ComponentAction(%s, %s, %v) = %s, %q, %v; want no decision", remote, main, same, action, rule, ok)
				case listed && (!ok || (result{action, rule}) != w):
					t.Errorf("ComponentAction(%s, %s, %v) = %s, %q, %v; want %s, %q, true", remote, main, same, action, rule, ok, w.action, w.rule)
				}
				if ok && rule != "" {
					used[rule] = true
				}
			}
		}
	}
	// A sidecar has no link, so no decision here stands for its rule.
	for _, id := range slices.Sorted(maps.Keys(domain.Rules)) {
		if strings.HasPrefix(id, "R-COMPONENT-") && id != "R-COMPONENT-05" && !used[id] {
			t.Errorf("%s decides no cell", id)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(used)) {
		if _, ok := domain.Rules[id]; !ok {
			t.Errorf("%s is not in domain.Rules", id)
		}
	}
}

// Each row changes one thing about the video of a Live Photo that lies beside
// its photo: the files of one shot share a directory and a stem.
func TestComponentBeside(t *testing.T) {
	t.Parallel()
	main := domain.Link{
		ID: 1, AssetID: 7, Remote: "disk", Root: domain.LibraryPersonal, Path: "2024/05-01 - Holidays/IMG_0001.HEIC",
		Role: domain.RoleOriginal, AlbumID: 10, Status: domain.StatusActive,
	}
	for _, c := range []struct {
		name string
		edit func(l *domain.Link)
		want bool
	}{
		{"the video of a Live Photo", func(*domain.Link) {}, true}, // covers: R-COMPONENT-04
		{"a RAW file", func(l *domain.Link) { l.Role, l.Path = domain.RoleRAWPair, "2024/05-01 - Holidays/IMG_0001.DNG" }, true},
		{"the extension in lower case", func(l *domain.Link) { l.Path = "2024/05-01 - Holidays/IMG_0001.mov" }, true},
		{"the stem in lower case", func(l *domain.Link) { l.Path = "2024/05-01 - Holidays/img_0001.MOV" }, true},
		{"a numbered name", func(l *domain.Link) { l.Path = "2024/05-01 - Holidays/IMG_0001 (1).MOV" }, false},
		{"another directory", func(l *domain.Link) { l.Path = "2024/05-02 - Trip/IMG_0001.MOV" }, false},
		{"a subdirectory", func(l *domain.Link) { l.Path = "2024/05-01 - Holidays/Live/IMG_0001.MOV" }, false},
		{"the directory in lower case", func(l *domain.Link) { l.Path = "2024/05-01 - holidays/IMG_0001.MOV" }, false},
		{"another root", func(l *domain.Link) { l.Root = domain.LibraryShared }, false},
		{"another remote", func(l *domain.Link) { l.Remote = "box" }, false},
		{"another asset", func(l *domain.Link) { l.AssetID = 8 }, false},
		{"another file of the asset", func(l *domain.Link) { l.Role, l.Path = domain.RoleOriginal, "2024/05-01 - Holidays/IMG_0001.JPG" }, false},
		{"the source of a converted asset", func(l *domain.Link) { l.Role, l.Path = domain.RoleSource, "2024/05-01 - Holidays/IMG_0001.MPG" }, false},
		{"a lesser copy", func(l *domain.Link) { l.Role, l.Path = domain.RoleDerived, "2024/05-01 - Holidays/IMG_0001.JPG" }, false},
		{"in quarantine", func(l *domain.Link) {
			l.Status, l.QuarantinePath = domain.StatusQuarantinedRemote, ".photosync/quarantine/2024-06-01/2024/05-01 - Holidays/IMG_0001.MOV"
		}, false},
	} {
		l := domain.Link{
			ID: 2, AssetID: 7, Remote: "disk", Root: domain.LibraryPersonal, Path: "2024/05-01 - Holidays/IMG_0001.MOV",
			Role: domain.RoleLiveVideo, AlbumID: 10, Status: domain.StatusActive,
		}
		c.edit(&l)
		if got := plan.ComponentBeside(main, l); got != c.want {
			t.Errorf("%s: ComponentBeside = %v; want %v", c.name, got, c.want)
		}
	}
}

// A component that moves, is copied or is uploaded takes its name from the
// main file where that ends up and from its resource in Photos, not from its
// own name in the cloud.
func TestComponentPath(t *testing.T) {
	t.Parallel()
	asset := domain.Asset{Resources: []domain.Resource{
		{Role: domain.ResourceOriginal, Filename: "IMG_0001.HEIC", Size: 100},
		{Role: domain.ResourceLiveVideo, Filename: "IMG_0001.MOV", Size: 200},
		{Role: domain.ResourceRAWPair, Filename: "IMG_0001.DNG", Size: 300},
	}}
	for _, c := range []struct {
		main string
		role domain.LinkRole
		want string
	}{
		{"2024/05-02 - Trip/IMG_0001.HEIC", domain.RoleLiveVideo, "2024/05-02 - Trip/IMG_0001.mov"}, // covers: R-COMPONENT-04
		{"2024/05-02 - Trip/IMG_0001.HEIC", domain.RoleRAWPair, "2024/05-02 - Trip/IMG_0001.DNG"},
		{"2024/05-02 - Trip/IMG_0001 (1).HEIC", domain.RoleLiveVideo, "2024/05-02 - Trip/IMG_0001 (1).mov"},
		{"2024/05-01/IMG_0042.HEIC", domain.RoleRAWPair, "2024/05-01/IMG_0042.DNG"},
		{"IMG_0001.HEIC", domain.RoleLiveVideo, "IMG_0001.mov"},
	} {
		if got := plan.ComponentPath(c.main, c.role, asset); got != c.want {
			t.Errorf("ComponentPath(%q, %s) = %q; want %q", c.main, c.role, got, c.want)
		}
	}
}
