package config_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Windemiatrix/photosync/internal/config"
	"github.com/Windemiatrix/photosync/internal/domain"
)

// The marker stands for a secret in a file that --config names by mistake; it
// matches no real token format, so that secret scanners stay quiet.
const marker = "not-a-secret-7f3a"

// The keys of the root and of sync are spelled here, while the policies come
// from the domain, which owns them: the package must not be its own oracle.
func wantDefaults() config.Config {
	p := domain.DefaultPolicies()
	return config.Config{
		Library:  "auto",
		Lang:     "ru",
		StateDir: "~/Library/Application Support/photosync",
		Sync: config.Sync{
			PathTemplate:       "{event_path}/{original_name}",
			SettleMinutes:      p.SettleMinutes,
			QuarantineKeepDays: p.QuarantineKeepDays,
			EditedDirs:         p.EditedDirs,
			Templates:          config.Templates{Photos: p.Templates.Photos, Cloud: p.Templates.Cloud},
			Policies: config.Policies{
				Conflict:           p.Conflict,
				DerivedCopy:        p.DerivedCopy,
				MultiAlbum:         p.MultiAlbum,
				UnsupportedFormats: p.UnsupportedFormats,
				EditedFiles:        p.EditedFiles,
				Hidden:             p.Hidden,
				Burst:              p.Burst,
				DeleteEmptyAlbums:  p.DeleteEmptyAlbums,
				AlbumsForPersonal:  p.AlbumsForPersonal,
			},
		},
	}
}

func parse(t *testing.T, data string) config.Config {
	t.Helper()
	cfg, err := config.Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	return cfg
}

func parseErrors(t *testing.T, data string) []config.Error {
	t.Helper()
	cfg, err := config.Parse([]byte(data))
	if err == nil {
		t.Fatal("Parse() error = nil, want one")
	}
	if !reflect.DeepEqual(cfg, config.Config{}) {
		t.Errorf("Parse() Config = %+v, want zero", cfg)
	}
	if strings.Contains(err.Error(), marker) {
		t.Errorf("Parse() error %q holds the value of a key", err)
	}
	errs := []error{err}
	var joined interface{ Unwrap() []error }
	if errors.As(err, &joined) {
		errs = joined.Unwrap()
	}
	got := make([]config.Error, 0, len(errs))
	for _, e := range errs {
		var ce *config.Error
		if !errors.As(e, &ce) {
			t.Fatalf("Parse() error %v is not a *config.Error", e)
		}
		got = append(got, *ce)
	}
	return got
}

// laughs is the billion laughs attack under an unknown key: expanded, its last
// list would hold 9^9 strings.
func laughs() string {
	var b strings.Builder
	b.WriteString("bogus:\n  a: &a [x, x, x, x, x, x, x, x, x]\n")
	for c := 'b'; c <= 'i'; c++ {
		refs := strings.TrimSuffix(strings.Repeat(fmt.Sprintf("*%c, ", c-1), 9), ", ")
		fmt.Fprintf(&b, "  %c: &%c [%s]\n", c, c, refs)
	}
	return b.String()
}

func TestParseFull(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("testdata", "full.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want := config.Config{
		Library:  "/Users/Shared/Example.photoslibrary",
		Lang:     "en",
		StateDir: "/Users/Shared/Example/state",
		Sync: config.Sync{
			PathTemplate:       "{event_path}/originals/{original_name}",
			Exclude:            []string{"Photos/Personal", "*.tmp"},
			RemotePriority:     []string{"backup", "disk"},
			SettleMinutes:      5,
			QuarantineKeepDays: 30,
			EditedDirs:         []string{"retouched"},
			Templates: config.Templates{
				Photos: []string{"{YYYY}", "{MM}-{DD} {Description}"},
				Cloud:  []string{"Events", "{YYYY}-{MM}-{DD} {Description}"},
				Names:  "en",
			},
			Policies: config.Policies{
				Conflict:           domain.ConflictICloudWins,
				Deletes:            domain.DeletesQuarantine,
				DerivedCopy:        domain.DerivedKeepBoth,
				MultiAlbum:         domain.MultiAlbumKeepOthers,
				UnsupportedFormats: domain.UnsupportedRemoteOnly,
				EditedFiles:        domain.EditedQuarantine,
				Hidden:             domain.HiddenSkip,
				Burst:              domain.BurstSelectedOnly,
				DeleteEmptyAlbums:  true,
				AlbumsForPersonal:  false,
			},
		},
	}
	if got := parse(t, string(data)); !reflect.DeepEqual(got, want) {
		t.Errorf("Parse() = %+v, want %+v", got, want)
	}
}

func TestParseDefaults(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, data string }{
		{"empty file", ""},
		{"comment only", "# lang: en\n"},
		{"document start", "---\n"},
		{"null document", "~\n"},
		{"empty section", "sync:\n"},
		{"empty string", "lang:\n"},
		{"null string", "lang: null\n"},
		{"null number", "sync:\n  settle_minutes: ~\n"},
		{"null list", "sync:\n  edited_dirs: ~\n"},
		{"empty inner sections", "sync:\n  templates:\n  policies: ~\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got, want := parse(t, tt.data), wantDefaults(); !reflect.DeepEqual(got, want) {
				t.Errorf("Parse() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestParseOverlay(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, data string
		change     func(*config.Config)
	}{
		{"one policy", "sync:\n  policies: {conflict: skip}\n", func(c *config.Config) {
			c.Sync.Policies.Conflict = domain.ConflictSkip
		}},
		{"empty list", "sync:\n  edited_dirs: []\n", func(c *config.Config) {
			c.Sync.EditedDirs = []string{}
		}},
		{"alias in a list", "sync:\n  exclude: [&x tmp]\n  edited_dirs: [*x]\n", func(c *config.Config) {
			c.Sync.Exclude = []string{"tmp"}
			c.Sync.EditedDirs = []string{"tmp"}
		}},
		{"document start before the only document", "---\nlang: en\n", func(c *config.Config) {
			c.Lang = "en"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			want := wantDefaults()
			tt.change(&want)
			if got := parse(t, tt.data); !reflect.DeepEqual(got, want) {
				t.Errorf("Parse() = %+v, want %+v", got, want)
			}
		})
	}
}

// Repeating a long value through aliases is valid YAML: only the size of the
// file limits it.
func TestParseRepeatedAlias(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 64<<10)
	cfg := parse(t, "sync:\n  exclude: [&x "+long+strings.Repeat(", *x", 1000)+"]\n")
	if len(cfg.Sync.Exclude) != 1001 {
		t.Fatalf("len(Exclude) = %d, want 1001", len(cfg.Sync.Exclude))
	}
	for i, s := range cfg.Sync.Exclude {
		if s != long {
			t.Fatalf("Exclude[%d] has %d bytes, want %d", i, len(s), len(long))
		}
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()
	earlier := "flavor: " + marker + "\n" +
		"full_scan: true\n" +
		"client_id: " + marker + "\n" +
		"client_secret_ref: " + marker + "\n" +
		"token_ref: {account: " + marker + "}\n" +
		"password_ref: {account: " + marker + "}\n" +
		"rest: {url: " + marker + "}\n" +
		"url: https://user:" + marker + "@example.com\n" +
		"endpoint: " + marker + "\n"
	secrets := "client_secret_ref: {account: " + marker + "}\n" +
		"token_ref: " + marker + "\n" +
		"password_ref: " + marker + "\n"
	tests := []struct {
		name, data string
		want       []config.Error
	}{
		{"unknown key in the root", "bogus: 1\n", []config.Error{
			{Key: "bogus", Code: "unknown_key", Line: 1},
		}},
		{"unknown key in sync", "sync:\n  bogus: 1\n", []config.Error{
			{Key: "sync.bogus", Code: "unknown_key", Line: 2},
		}},
		{"unknown key in policies", "sync:\n  policies:\n    bogus: x\n", []config.Error{
			{Key: "sync.policies.bogus", Code: "unknown_key", Line: 3},
		}},
		{"unknown key in templates", "sync:\n  templates:\n    bogus: x\n", []config.Error{
			{Key: "sync.templates.bogus", Code: "unknown_key", Line: 3},
		}},
		{"removed untitled_event", "sync:\n  untitled_event: x\n", []config.Error{
			{Key: "sync.untitled_event", Code: "untitled_event_removed", Line: 2},
		}},
		{"removed albums_root", "sync:\n  albums_root: x\n", []config.Error{
			{Key: "sync.albums_root", Code: "albums_root_removed", Line: 2},
		}},
		{"removed album_template", "sync:\n  album_template: x\n", []config.Error{
			{Key: "sync.album_template", Code: "album_template_removed", Line: 2},
		}},
		{"removed nested_separator", "sync:\n  nested_separator: x\n", []config.Error{
			{Key: "sync.nested_separator", Code: "nested_separator_removed", Line: 2},
		}},
		{"removed key outside its section", "untitled_event: x\n", []config.Error{
			{Key: "untitled_event", Code: "unknown_key", Line: 1},
		}},
		{"keys of earlier designs", earlier, []config.Error{
			{Key: "flavor", Code: "unknown_key", Line: 1},
			{Key: "full_scan", Code: "unknown_key", Line: 2},
			{Key: "client_id", Code: "unknown_key", Line: 3},
			{Key: "client_secret_ref", Code: "unknown_key", Line: 4},
			{Key: "token_ref", Code: "unknown_key", Line: 5},
			{Key: "password_ref", Code: "unknown_key", Line: 6},
			{Key: "rest", Code: "unknown_key", Line: 7},
			{Key: "url", Code: "unknown_key", Line: 8},
			{Key: "endpoint", Code: "unknown_key", Line: 9},
		}},
		{"secrets as scalars and sections", secrets, []config.Error{
			{Key: "client_secret_ref", Code: "unknown_key", Line: 1},
			{Key: "token_ref", Code: "unknown_key", Line: 2},
			{Key: "password_ref", Code: "unknown_key", Line: 3},
		}},
		{"token file", `{"access_token": "` + marker + `", "refresh_token": "` + marker + `"}` + "\n", []config.Error{
			{Key: "access_token", Code: "unknown_key", Line: 1},
			{Key: "refresh_token", Code: "unknown_key", Line: 1},
		}},
		{"repeated key in the root", "lang: ru\nlang: en\n", []config.Error{
			{Key: "lang", Code: "duplicate_key", Line: 2},
		}},
		{"repeated key in sync", "sync:\n  exclude: [a]\n  exclude: [b]\n", []config.Error{
			{Key: "sync.exclude", Code: "duplicate_key", Line: 3},
		}},
		{"word for a number", "sync:\n  settle_minutes: ten\n", []config.Error{
			{Key: "sync.settle_minutes", Code: "not_integer", Value: "ten", Line: 2},
		}},
		{"fraction for a number", "sync:\n  settle_minutes: 1.5\n", []config.Error{
			{Key: "sync.settle_minutes", Code: "not_integer", Value: "1.5", Line: 2},
		}},
		{"word for a boolean", "sync:\n  policies:\n    delete_empty_albums: maybe\n", []config.Error{
			{Key: "sync.policies.delete_empty_albums", Code: "not_boolean", Value: "maybe", Line: 3},
		}},
		{"list for a string", "lang: [ru]\n", []config.Error{
			{Key: "lang", Code: "not_string", Line: 1},
		}},
		{"section for a list", "sync:\n  exclude: {a: 1}\n", []config.Error{
			{Key: "sync.exclude", Code: "not_list", Line: 2},
		}},
		{"scalar for a list", "sync:\n  exclude: tmp\n", []config.Error{
			{Key: "sync.exclude", Code: "not_list", Value: "tmp", Line: 2},
		}},
		{"scalar for a section", "sync: 5\n", []config.Error{
			{Key: "sync", Code: "not_section", Value: "5", Line: 1},
		}},
		{"scalar file", "hello\n", []config.Error{
			{Code: "not_section", Line: 1},
		}},
		{"list file", "- a\n", []config.Error{
			{Code: "not_section", Line: 1},
		}},
		{"env file", "KEY=" + marker + "\nOTHER=x\n", []config.Error{
			{Code: "not_section", Line: 1},
		}},
		{"private key file", "-----BEGIN PRIVATE KEY-----\n" + marker + "\n-----END PRIVATE KEY-----\n", []config.Error{
			{Code: "not_section", Line: 1},
		}},
		{"errors in the order of the file", "bogus: 1\nsync:\n  settle_minutes: ten\nlang: [ru]\n", []config.Error{
			{Key: "bogus", Code: "unknown_key", Line: 1},
			{Key: "sync.settle_minutes", Code: "not_integer", Value: "ten", Line: 3},
			{Key: "lang", Code: "not_string", Line: 4},
		}},
		{"keys through an alias", "sync:\n  templates: &t\n    photos: [x]\n    bogus: 1\n  policies: *t\n", []config.Error{
			{Key: "sync.templates.bogus", Code: "unknown_key", Line: 4},
			{Key: "sync.policies.photos", Code: "unknown_key", Line: 3},
			{Key: "sync.policies.bogus", Code: "unknown_key", Line: 4},
		}},
		{"merge key", "sync:\n  templates: &t\n    photos: [x]\n  policies:\n    <<: *t\n", []config.Error{
			{Key: "sync.policies.<<", Code: "unknown_key", Line: 5},
		}},
		{"alias to its own section", "sync: &s\n  templates: *s\n", []config.Error{
			{Key: "sync.templates.templates", Code: "unknown_key", Line: 2},
		}},
		{"empty key", "\"\": 1\n", []config.Error{
			{Code: "unknown_key", Line: 1},
		}},
		{"complex key", "? [a]\n: 1\n", []config.Error{
			{Code: "unknown_key", Line: 1},
		}},
		{"alias as a key", "library: &lang lang\n*lang : en\n", []config.Error{
			{Key: "lang", Code: "unknown_key", Line: 2},
		}},
		{"null tag on a section", "sync: !!null {bogus: 1}\n", []config.Error{
			{Key: "sync.bogus", Code: "unknown_key", Line: 1},
		}},
		{"billion laughs under an unknown key", laughs(), []config.Error{
			{Key: "bogus", Code: "unknown_key", Line: 1},
		}},
		{"billion laughs in a list", laughs() + "sync:\n  exclude: *i\n", []config.Error{
			{Key: "bogus", Code: "unknown_key", Line: 1},
			{Key: "sync.exclude", Code: "not_list", Line: 12},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := parseErrors(t, tt.data); !slices.Equal(got, tt.want) {
				t.Errorf("Parse() errors = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseWhole(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, data, code string
		line             int
	}{
		{"second document", "lang: ru\n---\nlang: en\n", "multiple_documents", 2},
		{"secret in the second document", "lang: ru\n---\npassword_ref: " + marker + "\n", "multiple_documents", 2},
		{"unclosed list", "sync: [\n", "syntax", 1},
		// The parser names the line where it gave up on the enclosing node, at
		// or before the fault; these rows catch a change of that in the library.
		{"broken indent", "lang: ru\nsync:\n  edited_dirs: []\n settle_minutes: 1\n", "syntax", 3},
		{"unclosed list in the middle", "lang: ru\nexclude: [a\nlang: en\n", "syntax", 1},
		{"tab", "lang: ru\nsync:\n\tlang: x\n", "syntax", 3},
		{"secret before a colon", "lang: ru\na: " + marker + ": b\n", "syntax", 2},
		{"secret in an unclosed list", "exclude: [" + marker + "\n", "syntax", 1},
		{"undefined anchor", "password_ref: *" + marker + "\n", "syntax", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseErrors(t, tt.data)
			if len(got) != 1 {
				t.Fatalf("Parse() errors = %+v, want one", got)
			}
			e := got[0]
			if e.Code != tt.code || e.Key != "" || e.Value != "" {
				t.Errorf("Parse() error = %+v, want Code %q, no Key and no Value", e, tt.code)
			}
			if e.Line != tt.line {
				t.Errorf("Parse() error line = %d, want %d", e.Line, tt.line)
			}
		})
	}
}

func TestErrorString(t *testing.T) {
	t.Parallel()
	err := &config.Error{Key: "sync.settle_minutes", Code: "not_integer", Value: "ten", Line: 3}
	want := `config: not_integer: key "sync.settle_minutes", line 3, value "ten"`
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
