package domain_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// The configuration stores these strings: changing one breaks the
// configurations already written.
func TestPolicyValues(t *testing.T) {
	t.Parallel()
	checkStored(t, []domain.ConflictPolicy{domain.ConflictRemoteWins, domain.ConflictICloudWins, domain.ConflictAsk, domain.ConflictSkip},
		"remote_wins", "icloud_wins", "ask", "skip")
	checkStored(t, []domain.DeletesPolicy{domain.DeletesNone, domain.DeletesQuarantine, domain.DeletesPropagate},
		"none", "quarantine", "propagate")
	checkStored(t, []domain.DerivedCopyPolicy{domain.DerivedReplaceWithOriginal, domain.DerivedKeepBoth, domain.DerivedKeepRemote},
		"replace_with_original", "keep_both", "keep_remote")
	checkStored(t, []domain.MultiAlbumPolicy{domain.MultiAlbumKeepOthers, domain.MultiAlbumRemoveOthers},
		"closest_date_keep_others", "closest_date_remove_others")
	checkStored(t, []domain.UnsupportedFormatsPolicy{domain.UnsupportedConvertMP4, domain.UnsupportedRemoteOnly},
		"convert_mp4", "remote_only")
	checkStored(t, []domain.EditedFilesPolicy{domain.EditedIgnore, domain.EditedQuarantine},
		"ignore", "quarantine")
	checkStored(t, []domain.HiddenPolicy{domain.HiddenSync, domain.HiddenSkip},
		"sync", "skip")
	checkStored(t, []domain.BurstPolicy{domain.BurstAll, domain.BurstSelectedOnly},
		"all", "selected_only")
}

// The oracle spells the values as the configuration stores them, not with
// the constants, so that a wrong constant cannot hide a wrong default.
func TestDefaultPolicies(t *testing.T) {
	t.Parallel()
	want := domain.Policies{
		Conflict:           "remote_wins",
		DerivedCopy:        "replace_with_original",
		MultiAlbum:         "closest_date_remove_others",
		UnsupportedFormats: "convert_mp4",
		EditedFiles:        "ignore",
		Hidden:             "sync",
		Burst:              "all",
		AlbumsForPersonal:  true,
		EditedDirs:         []string{"отредактировано", "edited"},
		SettleMinutes:      10,
		NestedSeparator:    " / ",
	}
	got := domain.DefaultPolicies()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DefaultPolicies() = %+v, want %+v", got, want)
	}
	got.EditedDirs[0] = "changed"
	if next := domain.DefaultPolicies(); next.EditedDirs[0] != "отредактировано" {
		t.Errorf("DefaultPolicies().EditedDirs[0] = %q after a change to an earlier result, want %q", next.EditedDirs[0], "отредактировано")
	}
}

// A configuration that passes Validate must not fail a sync later, and the
// command that checks it names every wrong key at once.
func TestPoliciesValidate(t *testing.T) {
	t.Parallel()
	enums := []struct {
		key          string
		set          func(p *domain.Policies, v string)
		valid, wrong []string
	}{
		{"conflict", func(p *domain.Policies, v string) { p.Conflict = domain.ConflictPolicy(v) },
			[]string{"remote_wins", "icloud_wins", "ask", "skip"},
			[]string{"", "newest_wins", "Remote_Wins", "remote_wins ", "quarantine"}},
		{"deletes", func(p *domain.Policies, v string) { p.Deletes = domain.DeletesPolicy(v) },
			[]string{"none", "quarantine", "propagate"},
			[]string{"", "delete", "None", "skip"}},
		{"derived_copy", func(p *domain.Policies, v string) { p.DerivedCopy = domain.DerivedCopyPolicy(v) },
			[]string{"replace_with_original", "keep_both", "keep_remote"},
			[]string{"", "keep_original", "Keep_Both", "remote_only"}},
		{"multi_album", func(p *domain.Policies, v string) { p.MultiAlbum = domain.MultiAlbumPolicy(v) },
			[]string{"closest_date_keep_others", "closest_date_remove_others"},
			[]string{"", "keep_others", "remove_others", "Closest_Date_Keep_Others"}},
		{"unsupported_formats", func(p *domain.Policies, v string) { p.UnsupportedFormats = domain.UnsupportedFormatsPolicy(v) },
			[]string{"convert_mp4", "remote_only"},
			[]string{"", "convert", "Convert_MP4", "keep_remote"}},
		{"edited_files", func(p *domain.Policies, v string) { p.EditedFiles = domain.EditedFilesPolicy(v) },
			[]string{"ignore", "quarantine"},
			[]string{"", "delete", "Ignore", "none"}},
		{"hidden", func(p *domain.Policies, v string) { p.Hidden = domain.HiddenPolicy(v) },
			[]string{"sync", "skip"},
			[]string{"", "show", "Sync", "all"}},
		{"burst", func(p *domain.Policies, v string) { p.Burst = domain.BurstPolicy(v) },
			[]string{"all", "selected_only"},
			[]string{"", "selected", "All", "sync"}},
	}
	for _, e := range enums {
		for _, v := range e.valid {
			p := validPolicies()
			e.set(&p, v)
			checkPolicyErrors(t, e.key+"="+v, p.Validate())
		}
		for _, v := range e.wrong {
			p := validPolicies()
			e.set(&p, v)
			checkPolicyErrors(t, e.key+"="+v, p.Validate(), domain.PolicyError{Key: e.key, Value: v})
		}
	}

	cases := []struct {
		name string
		set  func(p *domain.Policies)
		want []domain.PolicyError
	}{
		{"settle_minutes=0", func(p *domain.Policies) { p.SettleMinutes = 0 }, nil},
		{"settle_minutes=1", func(p *domain.Policies) { p.SettleMinutes = 1 }, nil},
		{"settle_minutes=-1", func(p *domain.Policies) { p.SettleMinutes = -1 },
			[]domain.PolicyError{{Key: "settle_minutes", Value: "-1"}}},
		{"untitled_event Latin", func(p *domain.Policies) { p.UntitledEvent = "Untitled" }, nil},
		{"untitled_event Cyrillic", func(p *domain.Policies) { p.UntitledEvent = "Без названия" }, nil},
		// Whether a name suits the cloud is for package naming, which imports
		// domain and so cannot be called from here.
		{"untitled_event a/b", func(p *domain.Policies) { p.UntitledEvent = "a/b" }, nil},
		{"untitled_event empty", func(p *domain.Policies) { p.UntitledEvent = "" },
			[]domain.PolicyError{{Key: "untitled_event"}}},
		{"nested_separator \" / \"", func(p *domain.Policies) { p.NestedSeparator = " / " }, nil},
		{"nested_separator empty", func(p *domain.Policies) { p.NestedSeparator = "" },
			[]domain.PolicyError{{Key: "nested_separator"}}},
		{"albums_root Cyrillic", func(p *domain.Policies) { p.AlbumsRoot = "Альбомы" }, nil},
		{"albums_root empty", func(p *domain.Policies) { p.AlbumsRoot = "" },
			[]domain.PolicyError{{Key: "albums_root"}}},
		{"unchecked fields", func(p *domain.Policies) {
			p.DeleteEmptyAlbums = true
			p.AlbumsForPersonal = false
			p.RemotePriority = []string{"", "a", "a"}
			p.EditedDirs = []string{"", "a/b"}
		}, nil},
		{"every key wrong", func(p *domain.Policies) {
			*p = domain.Policies{
				Conflict: "remote-wins", Deletes: "Quarantine", DerivedCopy: "keep-both",
				MultiAlbum: "closest_date", UnsupportedFormats: "convert_mp3", EditedFiles: "Ignore",
				Hidden: "hide", Burst: "selected", SettleMinutes: -10,
			}
		}, []domain.PolicyError{
			{Key: "conflict", Value: "remote-wins"},
			{Key: "deletes", Value: "Quarantine"},
			{Key: "derived_copy", Value: "keep-both"},
			{Key: "multi_album", Value: "closest_date"},
			{Key: "unsupported_formats", Value: "convert_mp3"},
			{Key: "edited_files", Value: "Ignore"},
			{Key: "hidden", Value: "hide"},
			{Key: "burst", Value: "selected"},
			{Key: "settle_minutes", Value: "-10"},
			{Key: "untitled_event"},
			{Key: "nested_separator"},
			{Key: "albums_root"},
		}},
	}
	for _, c := range cases {
		p := validPolicies()
		c.set(&p)
		checkPolicyErrors(t, c.name, p.Validate(), c.want...)
	}

	// A configuration starts from the defaults, so Validate must take the
	// result of DefaultPolicies as it is: a value receiver.
	checkPolicyErrors(t, "DefaultPolicies()", domain.DefaultPolicies().Validate(),
		domain.PolicyError{Key: "deletes"}, domain.PolicyError{Key: "untitled_event"}, domain.PolicyError{Key: "albums_root"})
}

// The text goes to the log as it is, so it names its package like every
// error of domain.
func TestPolicyError(t *testing.T) {
	t.Parallel()
	e := &domain.PolicyError{Key: "deletes", Value: "Quarantine"}
	if got, want := e.Error(), `domain: policy deletes: invalid value "Quarantine"`; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// Spelled as the configuration stores them, so that a constant with a wrong
// value fails here too.
func validPolicies() domain.Policies {
	return domain.Policies{
		Conflict:           "icloud_wins",
		Deletes:            "quarantine",
		DerivedCopy:        "keep_both",
		MultiAlbum:         "closest_date_keep_others",
		UnsupportedFormats: "remote_only",
		EditedFiles:        "quarantine",
		Hidden:             "skip",
		Burst:              "selected_only",
		SettleMinutes:      10,
		UntitledEvent:      "Untitled",
		NestedSeparator:    " > ",
		AlbumsRoot:         "Albums",
	}
}

// errors.As, not a type assertion: errorlint rejects one on an error in tests
// too.
func checkPolicyErrors(t *testing.T, name string, err error, want ...domain.PolicyError) {
	t.Helper()
	if len(want) == 0 {
		if err != nil {
			t.Errorf("%s: Validate() = %v, want nil", name, err)
		}
		return
	}
	var joined interface{ Unwrap() []error }
	if !errors.As(err, &joined) {
		t.Errorf("%s: Validate() = %v (%T), want an errors.Join of %+v", name, err, err, want)
		return
	}
	var got []domain.PolicyError
	for _, e := range joined.Unwrap() {
		var pe *domain.PolicyError
		if !errors.As(e, &pe) {
			t.Errorf("%s: %v (%T) in Validate(), want a *domain.PolicyError", name, e, e)
			return
		}
		got = append(got, *pe)
	}
	if !slices.Equal(got, want) {
		t.Errorf("%s: Validate() = %+v, want %+v", name, got, want)
	}
}
