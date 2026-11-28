package config

import (
	"fmt"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// Config is a configuration file, with the default of each key that it leaves
// out.
type Config struct {
	Library  string `yaml:"library"`
	Lang     string `yaml:"lang"`
	StateDir string `yaml:"state_dir"`
	Sync     Sync   `yaml:"sync"`
}

// Sync is the section sync.
type Sync struct {
	PathTemplate       string    `yaml:"path_template"`
	Exclude            []string  `yaml:"exclude"`
	RemotePriority     []string  `yaml:"remote_priority"`
	SettleMinutes      int       `yaml:"settle_minutes"`
	QuarantineKeepDays int       `yaml:"quarantine_keep_days"`
	EditedDirs         []string  `yaml:"edited_dirs"`
	Templates          Templates `yaml:"templates"`
	Policies           Policies  `yaml:"policies"`
}

// Templates is the section sync.templates.
type Templates struct {
	Photos []string `yaml:"photos"`
	Cloud  []string `yaml:"cloud"`
	Names  string   `yaml:"names"`
}

// Policies is the section sync.policies.
type Policies struct {
	Conflict           domain.ConflictPolicy           `yaml:"conflict"`
	Deletes            domain.DeletesPolicy            `yaml:"deletes"`
	DerivedCopy        domain.DerivedCopyPolicy        `yaml:"derived_copy"`
	MultiAlbum         domain.MultiAlbumPolicy         `yaml:"multi_album"`
	UnsupportedFormats domain.UnsupportedFormatsPolicy `yaml:"unsupported_formats"`
	EditedFiles        domain.EditedFilesPolicy        `yaml:"edited_files"`
	Hidden             domain.HiddenPolicy             `yaml:"hidden"`
	Burst              domain.BurstPolicy              `yaml:"burst"`
	DeleteEmptyAlbums  bool                            `yaml:"delete_empty_albums"`
	AlbumsForPersonal  bool                            `yaml:"albums_for_personal"`
}

// Error is a fault of a configuration file: a key that photosync does not
// take, or the whole file. Key is the path of the key, such as
// "sync.policies.conflict" or "remotes[0].name", "" for the whole file; Code
// is stable across versions. Value is the scalar as written; it is "" for a
// syntax error, a key that photosync does not know, a repeated key, a second
// document, a list, a section and the whole file, so that a stray secret stays
// out of the logs. Line counts from 1, 0 when unknown; for Code "syntax" it is
// the line that the parser names, at or before the fault.
type Error struct {
	Key, Code, Value string
	Line             int
}

func (e *Error) Error() string {
	return fmt.Sprintf("config: %s: key %q, line %d, value %q", e.Code, e.Key, e.Line, e.Value)
}

func defaults() Config {
	p := domain.DefaultPolicies()
	return Config{
		Library:  "auto",
		Lang:     "ru",
		StateDir: "~/Library/Application Support/photosync",
		Sync: Sync{
			PathTemplate:       "{event_path}/{original_name}",
			SettleMinutes:      p.SettleMinutes,
			QuarantineKeepDays: p.QuarantineKeepDays,
			EditedDirs:         p.EditedDirs,
			Templates:          Templates{Photos: p.Templates.Photos, Cloud: p.Templates.Cloud},
			Policies: Policies{
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
